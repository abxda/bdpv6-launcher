package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/abxda/bdpv6-launcher/internal/hdfsfs"
	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
	"github.com/abxda/bdpv6-launcher/internal/ports"
	"github.com/abxda/bdpv6-launcher/internal/repair"
	"github.com/abxda/bdpv6-launcher/internal/services"
	"github.com/abxda/bdpv6-launcher/internal/setup"
	"github.com/abxda/bdpv6-launcher/internal/state"
	"github.com/abxda/bdpv6-launcher/internal/sysinfo"
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

	logSubsMu sync.Mutex
	logSubs   map[string]int // service id → logsink sub id, so we can detach on shutdown
}

const AppVersion = "0.2.0"

func NewApp() *App {
	p := paths.Detect()
	st := state.NewStore(p.StateFile)
	setupSink := logsink.New("setup", p.Logs, 2000)
	repairSink := logsink.New("repair", p.Logs, 2000)
	setupOrc := setup.New(p, st, setupSink)
	a := &App{
		paths:      p,
		state:      st,
		registry:   services.NewRegistry(),
		setupSink:  setupSink,
		setup:      setupOrc,
		repairSink: repairSink,
		repair:     repair.New(p, st, repairSink, setupOrc),
		logSubs:    map[string]int{},
	}
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	_ = a.state.Load()
	a.bootstrapServices()
	a.attachLogStreams()
	a.applyAlwaysOnTop(a.state.Get().AlwaysOnTop)
	sysinfo.Warm()
	go a.statusTickLoop()
	go a.sysinfoTickLoop()
}

func (a *App) domReady(ctx context.Context) {}

// beforeClose is invoked when the user clicks the X button. We do NOT block
// here — shutdown() handles the actual cleanup, and beforeClose is meant for
// "show a confirmation prompt" use cases.
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	return false
}

func (a *App) shutdown(ctx context.Context) {
	stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = a.registry.StopAll(stopCtx)
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
// PyQt5 launcher.
func (a *App) sysinfoTickLoop() {
	sysinfo.Tick(a.ctx, 2*time.Second, func(s sysinfo.Sample) {
		wailsruntime.EventsEmit(a.ctx, "sysinfo:update", s)
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
	SetupCompleted    bool              `json:"setupCompleted"`
	NamenodeFormatted bool              `json:"namenodeFormatted"`
}

func (a *App) GetEnvInfo() EnvInfo {
	ok, missing := a.paths.Validate()
	return EnvInfo{
		AppVersion:        AppVersion,
		OS:                runtime.GOOS,
		Arch:              runtime.GOARCH,
		ScriptDir:         a.paths.ScriptDir,
		DistributionOK:    ok,
		MissingDirs:       missing,
		ServicePaths:      a.paths.ServicePaths(),
		SetupCompleted:    a.state.Get().SetupCompleted,
		NamenodeFormatted: a.paths.NamenodeFormatted(),
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
	return svc.Start(a.ctx)
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
// alert. True when the user has not finished the wizard AND the NameNode is
// not formatted yet.
func (a *App) SetupNeeded() bool {
	if a.state.Get().SetupCompleted {
		return false
	}
	return !a.paths.NamenodeFormatted()
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
