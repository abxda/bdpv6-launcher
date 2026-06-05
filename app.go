package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/abxda/bdpv6-launcher/internal/exercises"
	"github.com/abxda/bdpv6-launcher/internal/hdfsfs"
	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
	"github.com/abxda/bdpv6-launcher/internal/ports"
	"github.com/abxda/bdpv6-launcher/internal/repair"
	"github.com/abxda/bdpv6-launcher/internal/services"
	"github.com/abxda/bdpv6-launcher/internal/setup"
	"github.com/abxda/bdpv6-launcher/internal/state"
	"github.com/abxda/bdpv6-launcher/internal/sysinfo"
	"github.com/abxda/bdpv6-launcher/internal/vmpeer"
)

// App is the Wails-bound type. Methods exported (capitalised) become callable
// from JS as window.go.main.App.<Method>(...). Methods that need to emit
// events to the frontend use the ctx captured during startup.
type App struct {
	ctx context.Context

	paths    *paths.Paths
	state    *state.Store
	registry *services.Registry

	setupSink  *logsink.Sink
	setup      *setup.Orchestrator
	repairSink *logsink.Sink
	repair     *repair.Repairer

	// exercises are discovered lazily on first ListExercises() call and
	// then cached. exerciseRunners holds one Runner+sink per exercise id.
	exMu            sync.Mutex
	exerciseList    []exercises.Exercise
	exerciseRunners map[string]*exerciseSession

	logSubsMu sync.Mutex
	logSubs   map[string]int // service id → logsink sub id, so we can detach on shutdown

	// shuttingDown gates the deferred-shutdown logic in beforeClose so the
	// second close attempt (issued from runtime.Quit at the end of the
	// graceful sequence) is allowed through instead of triggering a new
	// async shutdown run.
	shuttingDown atomic.Bool
}

const AppVersion = "0.2.0"

func NewApp() *App {
	p := paths.Detect()
	st := state.NewStore(p.StateFile)
	setupSink := logsink.New("setup", p.Logs, 2000)
	repairSink := logsink.New("repair", p.Logs, 2000)
	// setup/repair are low-volume diagnostic channels the student (or an agent)
	// may tail live. fsync each line so they're visible immediately even on
	// exFAT, where unsynced writes stay invisible to other processes until the
	// file handle closes. Service sinks stay unsynced (high volume).
	setupSink.SyncEachLine(true)
	repairSink.SyncEachLine(true)
	setupOrc := setup.New(p, st, setupSink)
	a := &App{
		paths:           p,
		state:           st,
		registry:        services.NewRegistry(),
		setupSink:       setupSink,
		setup:           setupOrc,
		repairSink:      repairSink,
		repair:          repair.New(p, st, repairSink, setupOrc),
		logSubs:         map[string]int{},
		exerciseRunners: map[string]*exerciseSession{},
	}
	return a
}

// exerciseSession bundles the runner + its log sink + a once-only event
// pump so each ExerciseTab subscription only fires the line-forward
// goroutine once.
type exerciseSession struct {
	ex     exercises.Exercise
	sink   *logsink.Sink
	runner *exercises.Runner
	pumped bool
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	_ = a.state.Load()
	a.healEnvironment()
	a.bootstrapServices()
	a.attachLogStreams()
	a.applyAlwaysOnTop(a.state.Get().AlwaysOnTop)
	sysinfo.Warm()
	go a.statusTickLoop()
	go a.sysinfoTickLoop()
}

// healEnvironment runs the platform-specific startup heal pass and surfaces
// the result in the setup log. On macOS this restores Unix features that
// exFAT / .zip-via-Archive-Utility strip (exec bits, python wrappers, the
// libsqlite3.dylib alias, com.apple.quarantine on the distro tree) so the
// student experience is "extract and launch" regardless of transport. The
// implementation is in internal/paths/heal_*; on non-darwin hosts it's a
// no-op. Errors are non-fatal — services try to start either way and the
// user can re-run via the repair UI.
func (a *App) healEnvironment() {
	rep := a.paths.HealDarwin()
	for _, line := range rep.Actions {
		a.setupSink.Emit("INFO", "[heal] "+line)
	}
	for _, line := range rep.Errors {
		a.setupSink.Emit("WARN", "[heal] "+line)
	}
}

func (a *App) domReady(ctx context.Context) {}

// beforeClose intercepts the user clicking X. First call: returns
// prevent=true and kicks off the async graceful shutdown in the background.
// While that runs, the frontend renders a full-screen overlay so the user
// knows the launcher is busy (and not frozen). When the shutdown finishes
// it calls wailsruntime.Quit, which triggers a second beforeClose — this
// time shuttingDown is already true, we return prevent=false, and the
// window actually closes.
//
// This lets us spend 30-60 s safely shutting HDFS down (force-checkpoint
// via dfsadmin before the inevitable hard kill) without confusing the user.
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	if a.shuttingDown.Load() {
		return false
	}
	a.shuttingDown.Store(true)
	go a.runGracefulShutdown()
	return true
}

// runGracefulShutdown is the async work kicked off from beforeClose. It
// emits progress events to the frontend, walks every service through
// GracefulPreStop (if implemented) + Stop, and finally calls Quit to let
// the window close.
func (a *App) runGracefulShutdown() {
	// First: cancel any in-flight exercise step. They're user code (not
	// our services) so we kill them outright rather than wait — students
	// don't want to wait 15 min for a stuck YARN retry loop to give up
	// when they close the launcher.
	a.stopAllExercises()

	wailsruntime.EventsEmit(a.ctx, "shutdown:start", map[string]any{
		"total": len(a.registry.All()),
	})

	// 90 s top-level cap. Each individual service has its own internal
	// timeouts (dfsadmin 20 s × 3 + Stop grace 15 s = ~75 s for the
	// worst case which is NameNode).
	stopCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	_ = a.registry.StopAllWithProgress(stopCtx, func(step, total int, name, phase string) {
		wailsruntime.EventsEmit(a.ctx, "shutdown:progress", map[string]any{
			"step":    step,
			"total":   total,
			"service": name,
			"phase":   phase, // "pre-stop" or "stop"
		})
	})

	wailsruntime.EventsEmit(a.ctx, "shutdown:done", nil)
	wailsruntime.Quit(a.ctx)
}

// shutdown runs after the window has actually closed. By the time we get
// here, runGracefulShutdown has already stopped every service. We just
// flush the sinks (so the on-disk log files end cleanly) and persist
// settings one last time.
func (a *App) shutdown(ctx context.Context) {
	for _, svc := range a.registry.All() {
		if sink := svc.Logs(); sink != nil {
			_ = sink.Close()
		}
	}
	_ = a.state.Save()
}

// bootstrapServices instantiates one wrapper per BDP service using the
// detected paths and the user's port/heap overrides from state.
func (a *App) bootstrapServices() {
	st := a.state.Get()
	port := func(id string, defp int) int {
		if v, ok := st.PortOverrides[id]; ok && v > 0 {
			return v
		}
		return defp
	}
	heap := func(id string) string {
		if v, ok := st.JVMHeap[id]; ok {
			return v
		}
		return ""
	}
	// Registration order matters for StopAll: services are stopped in
	// reverse order, so Jupyter goes down first (clean notebook sessions)
	// and HDFS NameNode last (so DataNode can flush blocks first).
	a.registry.Register(services.NewHDFSNameNode(a.paths, port("hdfs_namenode", 9870)))
	a.registry.Register(services.NewHDFSDataNode(a.paths, port("hdfs_datanode", 9864)))
	a.registry.Register(services.NewKafka(a.paths, port("kafka", 9092), heap("kafka")))
	a.registry.Register(services.NewElasticsearch(a.paths, port("elasticsearch", 9200), heap("elasticsearch")))
	a.registry.Register(services.NewJupyter(a.paths, port("jupyter", 8888)))
}

// attachLogStreams subscribes the App to every registered Sink and forwards
// each new line as a Wails event the frontend can listen to.
func (a *App) attachLogStreams() {
	for _, svc := range a.registry.All() {
		svc := svc // capture
		sink := svc.Logs()
		if sink == nil {
			continue
		}
		subID, ch := sink.Subscribe()
		a.logSubsMu.Lock()
		a.logSubs[svc.ID()] = subID
		a.logSubsMu.Unlock()
		go func() {
			eventName := "service:" + svc.ID() + ":log"
			for line := range ch {
				wailsruntime.EventsEmit(a.ctx, eventName, line)
			}
		}()
	}

	// Wizard / repair sink (id "setup") goes out under the same naming
	// convention as services for frontend symmetry.
	if a.setupSink != nil {
		_, ch := a.setupSink.Subscribe()
		go func() {
			for line := range ch {
				wailsruntime.EventsEmit(a.ctx, "service:setup:log", line)
			}
		}()
	}
	if a.repairSink != nil {
		_, ch := a.repairSink.Subscribe()
		go func() {
			for line := range ch {
				wailsruntime.EventsEmit(a.ctx, "service:repair:log", line)
			}
		}()
	}
}

// statusTickLoop periodically pushes the full status map to the frontend so
// it can rerender badges and PIDs without polling. Runs until the context
// (Wails ctx) is cancelled on shutdown.
func (a *App) statusTickLoop() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			wailsruntime.EventsEmit(a.ctx, "status:tick", a.registry.SortedStatuses())
		}
	}
}

// sysinfoTickLoop samples CPU/RAM and pushes "sysinfo:update" events the
// dashboard renders as progress bars. 2 s cadence matches the legacy
// PyQt5 launcher. It piggybacks an "env:tick" event on the same cadence
// carrying a fresh EnvInfo so the dashboard's setup-state badges reflect
// reality without making the user reload.
func (a *App) sysinfoTickLoop() {
	sysinfo.Tick(a.ctx, 2*time.Second, func(s sysinfo.Sample) {
		wailsruntime.EventsEmit(a.ctx, "sysinfo:update", s)
		wailsruntime.EventsEmit(a.ctx, "env:tick", a.GetEnvInfo())
	})
}

// applyAlwaysOnTop pins / unpins the Wails window. Safe to call before
// the window is fully realised — the runtime no-ops in that case.
func (a *App) applyAlwaysOnTop(v bool) {
	if a.ctx == nil {
		return
	}
	if v {
		wailsruntime.WindowSetAlwaysOnTop(a.ctx, true)
	} else {
		wailsruntime.WindowSetAlwaysOnTop(a.ctx, false)
	}
}

// ============================================================================
// Bound methods (callable from JS)
// ============================================================================

type EnvInfo struct {
	AppVersion        string            `json:"appVersion"`
	OS                string            `json:"os"`
	Arch              string            `json:"arch"`
	ScriptDir         string            `json:"scriptDir"`
	DistributionOK    bool              `json:"distributionOK"`
	MissingDirs       []string          `json:"missingDirs"`
	ServicePaths      map[string]string `json:"servicePaths"`

	// Setup state inferred from the filesystem (not from a persisted flag).
	// All four conditions must be true for SetupNeeded to be false on the
	// dashboard: Hadoop XMLs generated, NameNode formatted (not just empty,
	// not corrupted), Kafka formatted.
	HadoopConfigGenerated bool                `json:"hadoopConfigGenerated"`
	NamenodeFormatted     bool                `json:"namenodeFormatted"`
	NamenodeState         paths.NamenodeState `json:"namenodeState"`
	KafkaFormatted        bool                `json:"kafkaFormatted"`
	SetupNeeded           bool                `json:"setupNeeded"`

	// Legacy: the persisted state.SetupCompleted flag (still surfaced so
	// callers that need to know "did the wizard run to completion in some
	// past session" can read it; the UI no longer relies on it).
	SetupCompleted bool `json:"setupCompleted"`
}

// GetEnvInfo returns a fresh snapshot of the distribution state. Cheap
// enough to call on every dashboard tick (a few stat() syscalls).
func (a *App) GetEnvInfo() EnvInfo {
	ok, missing := a.paths.Validate()
	hadoopOK := a.paths.HadoopConfigGenerated()
	nnState := a.paths.NamenodeStateOf()
	nnOK := nnState == paths.NamenodeFormattedOK
	kafkaOK := a.paths.KafkaFormatted()
	return EnvInfo{
		AppVersion:            AppVersion,
		OS:                    runtime.GOOS,
		Arch:                  runtime.GOARCH,
		ScriptDir:             a.paths.ScriptDir,
		DistributionOK:        ok,
		MissingDirs:           missing,
		ServicePaths:          a.paths.ServicePaths(),
		HadoopConfigGenerated: hadoopOK,
		NamenodeFormatted:     nnOK,
		NamenodeState:         nnState,
		KafkaFormatted:        kafkaOK,
		SetupNeeded:           !(hadoopOK && nnOK && kafkaOK),
		SetupCompleted:        a.state.Get().SetupCompleted,
	}
}

func (a *App) GetState() state.State { return a.state.Get() }

func (a *App) SetLanguage(lang string) error {
	if lang != "es" && lang != "en" {
		lang = "es"
	}
	return a.state.Update(func(s *state.State) { s.Language = lang })
}

func (a *App) SetAlwaysOnTop(v bool) error {
	if err := a.state.Update(func(s *state.State) { s.AlwaysOnTop = v }); err != nil {
		return err
	}
	a.applyAlwaysOnTop(v)
	return nil
}

// SetAutoStartJupyter persists the toggle. Honoured by F9 startup behaviour.
func (a *App) SetAutoStartJupyter(v bool) error {
	return a.state.Update(func(s *state.State) { s.AutoStartJupyter = v })
}

// SetJVMHeap stores a JVM heap override (e.g. "1g", "2g") for a service.
// The new value takes effect on the next service restart.
func (a *App) SetJVMHeap(serviceID, heap string) error {
	return a.state.Update(func(s *state.State) {
		if s.JVMHeap == nil {
			s.JVMHeap = map[string]string{}
		}
		s.JVMHeap[serviceID] = heap
	})
}

// GetSysInfo returns one synchronous sample — useful for the initial
// dashboard render before the first tick event arrives.
func (a *App) GetSysInfo() sysinfo.Sample { return sysinfo.Now() }

// --- service control --------------------------------------------------------

// ServiceMeta is the static (non-runtime) view of a service. Used by the UI
// to render the Services tab even when nothing has started yet.
type ServiceMeta struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Port int    `json:"port"`
}

func (a *App) ListServices() []ServiceMeta {
	out := []ServiceMeta{}
	for _, svc := range a.registry.All() {
		out = append(out, ServiceMeta{ID: svc.ID(), Name: svc.Name(), Port: svc.Port()})
	}
	return out
}

func (a *App) GetStatuses() []services.Status { return a.registry.SortedStatuses() }

func (a *App) StartService(id string) error {
	svc, ok := a.registry.Get(id)
	if !ok {
		return fmt.Errorf("servicio desconocido: %s", id)
	}
	if err := a.preflightPorts(svc); err != nil {
		return err
	}
	return svc.Start(a.ctx)
}

// preflightPorts checks every port the service needs is free (or held by
// the service itself, in case of a Start-while-Running). Without this,
// a stale JVM zombie from a previous run would let the service Start
// succeed up to the point where Java tries to bind, then crash with a
// cryptic "Address already in use" deep in a Hadoop stack trace.
// Surfacing it here gives the student a clear, actionable error.
func (a *App) preflightPorts(svc services.Service) error {
	ourPID := svc.Status().PID
	type portRequirer interface{ RequiredPorts() []int }
	required := []int{svc.Port()}
	if pr, ok := svc.(portRequirer); ok {
		required = pr.RequiredPorts()
	}
	for _, port := range required {
		own := ports.WhoOwns(port)
		if own.PID == 0 {
			continue // free
		}
		if own.PID == ourPID {
			continue // it's already us
		}
		// Someone else owns it. Surface a clear actionable error.
		name := own.Name
		if name == "" {
			name = "proceso desconocido"
		}
		// Caso especial: el ocupante es la VM de la Edición Vagrant. En vez del
		// error técnico, damos un mensaje claro y accionable (la UI ofrece
		// apagarla con el botón que llama a ShutdownVagrantPeer).
		if vmpeer.OwnerIsVagrantVM(name) {
			return fmt.Errorf(
				"VAGRANT_VM_RUNNING|El puerto %d lo está usando tu laboratorio Vagrant (la máquina virtual). "+
					"Las dos ediciones usan los mismos puertos y no pueden correr a la vez. "+
					"Apaga la máquina virtual de Vagrant para usar el Portable.",
				port,
			)
		}
		return fmt.Errorf(
			"puerto %d ya está ocupado por %s (PID %d). "+
				"Abre el tab Puertos para liberarlo, o termínalo desde Administrador de tareas.",
			port, name, own.PID,
		)
	}
	return nil
}

// VagrantVMRunning reporta si la VM de la Edición Vagrant está encendida
// (ocupa los mismos puertos que el Portable). La UI lo usa para mostrar el
// botón "Apagar laboratorio Vagrant".
func (a *App) VagrantVMRunning() bool {
	return vmpeer.Running()
}

// ShutdownVagrantPeer apaga limpiamente la VM de la Edición Vagrant (ACPI por
// nombre), liberando los puertos para que el Portable pueda arrancar. No
// necesita saber desde qué carpeta se levantó la VM.
func (a *App) ShutdownVagrantPeer() error {
	if !vmpeer.Running() {
		return nil
	}
	if vmpeer.Shutdown(90 * time.Second) {
		return nil
	}
	return fmt.Errorf("no confirmé el apagado de la máquina virtual a tiempo; inténtalo de nuevo o apágala desde VirtualBox")
}

// KillPortHolder terminates whichever process is currently bound to the
// given local port. Refuses to kill one of the launcher's own services
// (use the Stop button for those). Returns a clear error message that
// the UI surfaces verbatim.
func (a *App) KillPortHolder(port int) error {
	own := ports.WhoOwns(port)
	if own.PID == 0 {
		return fmt.Errorf("el puerto %d ya está libre", port)
	}
	// Refuse to kill one of our own services through this path.
	for _, svc := range a.registry.All() {
		if svc.Status().PID == own.PID {
			return fmt.Errorf("ese puerto pertenece al servicio %q — usa el botón Detener en su lugar para que el shutdown sea gracioso", svc.Name())
		}
	}
	if err := ports.KillByPID(own.PID); err != nil {
		return fmt.Errorf("no pude matar PID %d (%s): %w", own.PID, own.Name, err)
	}
	return nil
}

func (a *App) StopService(id string) error {
	svc, ok := a.registry.Get(id)
	if !ok {
		return fmt.Errorf("servicio desconocido: %s", id)
	}
	return svc.Stop(a.ctx)
}

// GetServiceLogs returns the ring buffer snapshot for a given service. The
// frontend uses this on console open, then subscribes to live updates via
// the runtime events.
func (a *App) GetServiceLogs(id string) []logsink.Line {
	svc, ok := a.registry.Get(id)
	if !ok || svc.Logs() == nil {
		return nil
	}
	return svc.Logs().Snapshot()
}

func (a *App) ClearServiceLogs(id string) {
	if svc, ok := a.registry.Get(id); ok && svc.Logs() != nil {
		svc.Logs().Clear()
	}
}

// JupyterURL returns the captured token-bearing URL (empty if Jupyter has
// not printed it yet or is not running).
func (a *App) JupyterURL() string {
	if svc, ok := a.registry.Get("jupyter"); ok {
		if j, ok := svc.(*services.Jupyter); ok {
			return j.URL()
		}
	}
	return ""
}

// OpenJupyter asks the OS to open the captured Jupyter URL in the default
// browser. No-op if the URL is not yet known.
func (a *App) OpenJupyter() {
	url := a.JupyterURL()
	if url == "" {
		return
	}
	wailsruntime.BrowserOpenURL(a.ctx, url)
}

// --- ports & collisions -----------------------------------------------------

// ScanPorts returns one row per known service port plus an "extras" set of
// non-service ports often relevant for diagnostics (e.g. HDFS RPC 9000,
// Kafka controller 9093). Used by the Ports tab.
func (a *App) ScanPorts() []ports.Probe {
	probes := []ports.Probe{}
	for _, svc := range a.registry.All() {
		probes = append(probes, ports.Probe{
			ServiceID:   svc.ID(),
			ServiceName: svc.Name(),
			Port:        svc.Port(),
		})
	}
	// Extra well-known ports the student often needs to know about even if
	// the service does not expose a top-level Port() for them.
	extras := []ports.Probe{
		{ServiceID: "hdfs_rpc",  ServiceName: "HDFS RPC",         Port: 9000},
		{ServiceID: "kafka_ctl", ServiceName: "Kafka Controller", Port: 9093},
	}
	probes = append(probes, extras...)

	// Build the "PIDs we own" set so the scanner can flag a listening port
	// as "ours" vs "someone else's".
	ours := map[int]bool{}
	for _, svc := range a.registry.All() {
		if pid := svc.Status().PID; pid > 0 {
			ours[pid] = true
		}
	}
	return ports.Scan(probes, ours)
}

// SuggestFreePort returns the first free TCP port at-or-above the requested
// value. 0 means "could not find one in a sensible search window".
func (a *App) SuggestFreePort(start int) int {
	return ports.SuggestFree(start)
}

// SetPortOverride persists a custom port for a service. The new port takes
// effect on the next launcher restart (or service Stop+Start once F8 wires
// in the live reconfiguration path).
func (a *App) SetPortOverride(serviceID string, port int) error {
	if port <= 0 || port >= 65535 {
		return fmt.Errorf("puerto fuera de rango (1-65534): %d", port)
	}
	return a.state.Update(func(s *state.State) {
		if s.PortOverrides == nil {
			s.PortOverrides = map[string]int{}
		}
		s.PortOverrides[serviceID] = port
	})
}

// ClearPortOverride drops a custom port (back to the service default).
func (a *App) ClearPortOverride(serviceID string) error {
	return a.state.Update(func(s *state.State) {
		if s.PortOverrides != nil {
			delete(s.PortOverrides, serviceID)
		}
	})
}

// --- first-run wizard ------------------------------------------------------

// SetupNeeded returns whether the dashboard should show the "configure now"
// alert. Inferred purely from the filesystem so that setup actions taken
// outside the wizard (legacy setup_first_run.bat, manual Repair runs,
// cleanup followed by re-format) reflect immediately on next dashboard
// tick. The persisted state.SetupCompleted flag is no longer consulted —
// it was brittle because the Orchestrator's step state resets per session
// and never tripped the flag when only one step was re-run via Repair.
func (a *App) SetupNeeded() bool {
	return !(a.paths.HadoopConfigGenerated() &&
		a.paths.NamenodeFormatted() &&
		a.paths.KafkaFormatted())
}

// GetSetupSteps returns the wizard steps with their current status.
func (a *App) GetSetupSteps() []setup.Step { return a.setup.Steps() }

// RunSetupStep executes one wizard step. Output streams via the
// service:setup:log event, identical pattern to services.
func (a *App) RunSetupStep(id string) error {
	return a.setup.RunStep(a.ctx, id)
}

// RunSetupAll runs every wizard step in order, stopping on first failure.
func (a *App) RunSetupAll() error { return a.setup.RunAll(a.ctx) }

// GetSetupLogs returns the cached setup output (ring buffer snapshot).
func (a *App) GetSetupLogs() []logsink.Line {
	if a.setupSink == nil {
		return nil
	}
	return a.setupSink.Snapshot()
}

// --- repair actions --------------------------------------------------------

// RepairAction identifies an action by name. Defined as constants so the
// frontend code does not have to know the string values.
type RepairAction = repair.Action

// RunRepair invokes one of the destructive maintenance actions. Streams via
// the service:repair:log event. The caller (frontend) is responsible for
// obtaining explicit user confirmation before invoking.
func (a *App) RunRepair(action string) error {
	if action == "" {
		return fmt.Errorf("acción vacía")
	}
	return a.repair.Run(a.ctx, repair.Action(action))
}

// GetRepairLogs returns the ring buffer snapshot from the repair sink.
func (a *App) GetRepairLogs() []logsink.Line {
	if a.repairSink == nil {
		return nil
	}
	return a.repairSink.Snapshot()
}

// --- exercises --------------------------------------------------------------

// ListExercises returns the playable exercises discovered next to the BDP
// distribution. Cheap to call on every Exercises tab open.
func (a *App) ListExercises() []exercises.Exercise {
	a.exMu.Lock()
	defer a.exMu.Unlock()
	if a.exerciseList == nil {
		a.exerciseList = exercises.Discover(a.paths)
	}
	return a.exerciseList
}

// sessionFor lazily builds (or returns the cached) per-exercise session.
// First call also starts the goroutine that forwards sink lines to the
// "exercise:<id>:log" Wails event and registers an OnStateChange so the
// UI knows when to swap Run/Stop buttons.
func (a *App) sessionFor(id string) *exerciseSession {
	a.exMu.Lock()
	defer a.exMu.Unlock()
	if s, ok := a.exerciseRunners[id]; ok {
		return s
	}
	// Ensure the master list is loaded.
	if a.exerciseList == nil {
		a.exerciseList = exercises.Discover(a.paths)
	}
	var ex *exercises.Exercise
	for i := range a.exerciseList {
		if a.exerciseList[i].ID == id {
			ex = &a.exerciseList[i]
			break
		}
	}
	if ex == nil {
		return nil
	}
	sink := logsink.New("ex-"+id, a.paths.Logs, 4000)
	runner := exercises.NewRunner(*ex, a.paths, sink)
	s := &exerciseSession{ex: *ex, sink: sink, runner: runner}
	a.exerciseRunners[id] = s

	// One subscription per session, forwards every line as an event.
	_, ch := sink.Subscribe()
	logEvent := "exercise:" + id + ":log"
	go func() {
		for line := range ch {
			wailsruntime.EventsEmit(a.ctx, logEvent, line)
		}
	}()

	// State change → fires "exercise:<id>:state" with {running: bool}
	// so the UI can swap the [▶ Run] / [⏹ Stop] buttons.
	stateEvent := "exercise:" + id + ":state"
	runner.OnStateChange(func(running bool) {
		wailsruntime.EventsEmit(a.ctx, stateEvent, map[string]any{"running": running})
	})
	return s
}

// RunExerciseStep executes one step (0-indexed) of the given exercise.
func (a *App) RunExerciseStep(exerciseID string, stepIdx int) error {
	s := a.sessionFor(exerciseID)
	if s == nil {
		return fmt.Errorf("ejercicio desconocido: %s", exerciseID)
	}
	return s.runner.RunStep(a.ctx, stepIdx)
}

// RunAllExerciseSteps runs every step in order, stopping on the first failure.
func (a *App) RunAllExerciseSteps(exerciseID string) error {
	s := a.sessionFor(exerciseID)
	if s == nil {
		return fmt.Errorf("ejercicio desconocido: %s", exerciseID)
	}
	return s.runner.RunAll(a.ctx)
}

// GetExerciseLogs returns the ring-buffer snapshot for the exercise console.
func (a *App) GetExerciseLogs(exerciseID string) []logsink.Line {
	s := a.sessionFor(exerciseID)
	if s == nil {
		return nil
	}
	return s.sink.Snapshot()
}

// ClearExerciseLogs wipes the per-exercise ring buffer (just the UI cache;
// the on-disk log file under logs/ex-<id>.log is preserved).
func (a *App) ClearExerciseLogs(exerciseID string) {
	if s := a.sessionFor(exerciseID); s != nil {
		s.sink.Clear()
	}
}

// StopExerciseStep cancels whatever step is currently running for the
// given exercise. No-op if nothing is running.
func (a *App) StopExerciseStep(exerciseID string) {
	if s := a.sessionFor(exerciseID); s != nil {
		s.runner.Stop()
	}
}

// OpenExerciseBash spawns a new git-bash.exe window pre-configured with
// JDK / Hadoop / Python on PATH, cwd at the exercise dir, and a teaching
// cheat-sheet of useful commands printed on first prompt. Lets students
// experiment freely without leaving the launcher ecosystem.
func (a *App) OpenExerciseBash(exerciseID string) error {
	s := a.sessionFor(exerciseID)
	if s == nil {
		return fmt.Errorf("ejercicio desconocido: %s", exerciseID)
	}
	return exercises.OpenBashSession(a.paths, s.ex)
}

// IsExerciseRunning lets the UI initialise the Run/Stop button state on
// view enter (the state event covers subsequent changes).
func (a *App) IsExerciseRunning(exerciseID string) bool {
	s := a.sessionFor(exerciseID)
	if s == nil {
		return false
	}
	return s.runner.Running()
}

// stopAllExercises is called during graceful shutdown so a long-running
// or hung step does not block the launcher window from closing.
func (a *App) stopAllExercises() {
	a.exMu.Lock()
	sessions := make([]*exerciseSession, 0, len(a.exerciseRunners))
	for _, s := range a.exerciseRunners {
		sessions = append(sessions, s)
	}
	a.exMu.Unlock()
	for _, s := range sessions {
		s.runner.Stop()
	}
}

// --- HDFS explorer ---------------------------------------------------------

// ListHDFS returns the WebHDFS LISTSTATUS for the given path (use "/" for
// root). Lazy-loaded by the frontend tree on click.
func (a *App) ListHDFS(path string) ([]hdfsfs.Entry, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 4*time.Second)
	defer cancel()
	port := 9870
	if svc, ok := a.registry.Get("hdfs_namenode"); ok {
		port = svc.Port()
	}
	c := hdfsfs.New(fmt.Sprintf("http://127.0.0.1:%d", port), "")
	return c.List(ctx, path)
}

// --- Notebooks tab --------------------------------------------------------

// NotebookFile is one entry shown in the Notebooks tab.
type NotebookFile struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	ModTime  int64  `json:"modTime"` // unix millis
	IsDir    bool   `json:"isDir"`
}

// ListNotebooks returns the contents of paths.Notebooks (non-recursive).
// .ipynb_checkpoints and dotfiles are filtered out to keep the list tidy.
func (a *App) ListNotebooks() ([]NotebookFile, error) {
	dir := a.paths.Notebooks
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Returning an empty slice is friendlier than an error for the UI;
		// the user just sees "no notebooks".
		return []NotebookFile{}, nil
	}
	out := make([]NotebookFile, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, NotebookFile{
			Name: name, Size: info.Size(),
			ModTime: info.ModTime().UnixMilli(), IsDir: e.IsDir(),
		})
	}
	return out, nil
}

// OpenNotebook builds the Jupyter URL for a notebook file (relative to
// paths.Notebooks) and asks the system browser to open it. Requires
// Jupyter to be running (so we have its token).
func (a *App) OpenNotebook(name string) error {
	jurl := a.JupyterURL()
	if jurl == "" {
		return fmt.Errorf("Jupyter no está corriendo o aún no expone token")
	}
	// jurl is e.g. http://127.0.0.1:8888/lab?token=abc
	// We want http://127.0.0.1:8888/lab/tree/<name>?token=abc so the file opens.
	u, err := url.Parse(jurl)
	if err != nil {
		return err
	}
	u.Path = "/lab/tree/" + name
	wailsruntime.BrowserOpenURL(a.ctx, u.String())
	return nil
}

// OpenNotebooksFolder abre la carpeta de notebooks (la misma que usa Jupyter)
// en el explorador de archivos del SO. No requiere que Jupyter esté corriendo.
func (a *App) OpenNotebooksFolder() error {
	return openFolder(a.paths.Notebooks)
}
