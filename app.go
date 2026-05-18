package main

import (
	"context"
	"runtime"

	"github.com/abxda/bdpv6-launcher/internal/paths"
	"github.com/abxda/bdpv6-launcher/internal/state"
)

// App holds the application context and exposes methods bound to the JS
// frontend via Wails. Methods that should be callable from JS must be
// exported (capitalised) and receive serialisable arguments.
type App struct {
	ctx   context.Context
	paths *paths.Paths
	state *state.Store
}

func NewApp() *App {
	p := paths.Detect()
	return &App{
		paths: p,
		state: state.NewStore(p.StateFile),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Best-effort load; if it fails, the in-memory defaults remain.
	_ = a.state.Load()
}

func (a *App) domReady(ctx context.Context) {}

func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	// F2 will gracefully stop running services here. For F1, just allow close.
	return false
}

func (a *App) shutdown(ctx context.Context) {
	_ = a.state.Save()
}

// --- Bound methods (callable from JS as window.go.main.App.*) ---

// EnvInfo describes the host environment and the resolved distribution
// layout. Returned to the frontend on startup so the UI can show the
// detected paths, version, and whether the distribution is well-formed.
type EnvInfo struct {
	AppVersion       string            `json:"appVersion"`
	OS               string            `json:"os"`
	Arch             string            `json:"arch"`
	ScriptDir        string            `json:"scriptDir"`
	DistributionOK   bool              `json:"distributionOK"`
	MissingDirs      []string          `json:"missingDirs"`
	ServicePaths     map[string]string `json:"servicePaths"`
	SetupCompleted   bool              `json:"setupCompleted"`
	NamenodeFormatted bool             `json:"namenodeFormatted"`
}

const AppVersion = "0.1.0"

// GetEnvInfo returns the resolved environment and distribution layout.
// Used by the frontend on first render and after any repair action.
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

// GetState returns the persisted user state (preferences, port overrides, …).
func (a *App) GetState() state.State {
	return a.state.Get()
}

// SetLanguage persists the preferred UI language (es | en).
func (a *App) SetLanguage(lang string) error {
	if lang != "es" && lang != "en" {
		lang = "es"
	}
	return a.state.Update(func(s *state.State) {
		s.Language = lang
	})
}

// SetAlwaysOnTop persists the toggle. The Wails runtime side-effect (actually
// pinning the window) will be wired in F8.
func (a *App) SetAlwaysOnTop(v bool) error {
	return a.state.Update(func(s *state.State) {
		s.AlwaysOnTop = v
	})
}
