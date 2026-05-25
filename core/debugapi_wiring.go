package core

import (
	"errors"
	"time"

	"singbox-launcher/api"
	"singbox-launcher/core/debugapi"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/platform"
)

// debugAPIFacade adapts *AppController to debugapi.ControllerFacade.
// Kept in the core package (where the concrete types live) rather than in
// debugapi to avoid an import cycle.
type debugAPIFacade struct {
	ac *AppController
}

func (f *debugAPIFacade) IsRunning() bool {
	return f.ac.RunningState != nil && f.ac.RunningState.IsRunning()
}

func (f *debugAPIFacade) GetProxiesList() []api.ProxyInfo {
	return f.ac.GetProxiesList()
}

func (f *debugAPIFacade) GetActiveProxyName() string {
	return f.ac.GetActiveProxyName()
}

func (f *debugAPIFacade) GetSelectedClashGroup() string {
	if f.ac.APIService == nil {
		return ""
	}
	return f.ac.APIService.GetSelectedClashGroup()
}

func (f *debugAPIFacade) GetSingboxVersion() string {
	v, err := f.ac.GetInstalledCoreVersion()
	if err != nil {
		return ""
	}
	return v
}

func (f *debugAPIFacade) GetConfigPath() string {
	if f.ac.FileService == nil {
		return ""
	}
	return f.ac.FileService.ConfigPath
}

func (f *debugAPIFacade) GetExecDir() string {
	if f.ac.FileService == nil {
		return ""
	}
	return f.ac.FileService.ExecDir
}

func (f *debugAPIFacade) GetLauncherVersion() string {
	return constants.AppVersion
}

func (f *debugAPIFacade) GetLastUpdateSucceededAt() time.Time {
	if f.ac.StateService == nil {
		return time.Time{}
	}
	f.ac.StateService.LastUpdateMutex.RLock()
	defer f.ac.StateService.LastUpdateMutex.RUnlock()
	return f.ac.StateService.LastUpdateSucceededAt
}

func (f *debugAPIFacade) StartSingBox() error {
	StartSingBoxProcess()
	return nil
}

func (f *debugAPIFacade) StopSingBox() error {
	StopSingBoxProcess()
	return nil
}

func (f *debugAPIFacade) PingAllProxies() error {
	// The pingAllProxies implementation is a closure inside clash_api_tab.go
	// — we expose it via the same UIService hook the power-resume path uses.
	if f.ac.UIService == nil || f.ac.UIService.AutoPingAfterConnectFunc == nil {
		return nil
	}
	f.ac.UIService.AutoPingAfterConnectFunc()
	return nil
}

func (f *debugAPIFacade) RebuildConfigIfDirty() error {
	return f.ac.RebuildConfigIfDirty()
}

// LoadState reads state.json from the canonical wizard path. Returns
// state.ErrNotFound if the file isn't present yet (fresh install).
func (f *debugAPIFacade) LoadState() (*state.State, error) {
	if f.ac.FileService == nil {
		return nil, errors.New("FileService not initialized")
	}
	return state.Load(platform.GetWizardStatePath(f.ac.FileService.ExecDir))
}

// SaveState atomically writes state.json (SPEC 050 invariant — single
// .tmp + Rename, fsync). We then mark the StateService dirty markers so
// the next RebuildConfigIfDirty / Restart sees the change.
func (f *debugAPIFacade) SaveState(s *state.State) error {
	if f.ac.FileService == nil {
		return errors.New("FileService not initialized")
	}
	if s == nil {
		return errors.New("nil state")
	}
	path := platform.GetWizardStatePath(f.ac.FileService.ExecDir)
	if err := s.Save(path); err != nil {
		return err
	}
	if f.ac.StateService != nil {
		// SPEC 050 invariant 3: any external mutation flips both dirty
		// markers (better-safe — we don't classify rules-vs-template
		// changes at the API boundary). Each Mark publishes a
		// StateChanged event via the bus internally.
		f.ac.StateService.MarkCacheStale()
		f.ac.StateService.MarkConfigStale()
	}
	return nil
}

// LoadTemplate proxies to the canonical loader so /state/outbounds/resolved
// can resolve referenced bodies.
func (f *debugAPIFacade) LoadTemplate() (*template.TemplateData, error) {
	if f.ac.FileService == nil {
		return nil, errors.New("FileService not initialized")
	}
	return template.LoadTemplateData(f.ac.FileService.ExecDir)
}

// ApplyLogLevelAndReload — proxy to core.ApplyLogLevelAndReloadCore (the
// helper extracted from ui/traffic_verbose.go in this SPEC). Resets active
// connections.
func (f *debugAPIFacade) ApplyLogLevelAndReload(level string) error {
	return ApplyLogLevelAndReloadCore(f.ac, level)
}

// ReadCurrentLogLevel — proxy to core.ReadCurrentLogLevelFromState.
func (f *debugAPIFacade) ReadCurrentLogLevel() (string, bool, error) {
	return ReadCurrentLogLevelFromState(f.ac)
}

func (f *debugAPIFacade) UpdateSubscriptions() error {
	if f.ac.ConfigService == nil {
		return errors.New("config service not initialized")
	}
	// Run synchronously so the HTTP caller learns success/failure in-band.
	// Per-source counts are exposed to humans through toasts, not through
	// the JSON action contract — discard the result here.
	_, err := f.ac.ConfigService.UpdateConfigFromSubscriptions()
	return err
}

// debugAPIState holds a singleton-ish handle so main.go can Start/Stop it.
var (
	debugAPIServer *debugapi.Server
)

// StartDebugAPI binds the debug-API server on 127.0.0.1:port with the given
// bearer token. Safe to call more than once — subsequent calls restart.
func (ac *AppController) StartDebugAPI(port int, token string) error {
	if debugAPIServer != nil {
		debugAPIServer.Stop()
		debugAPIServer = nil
	}
	s, err := debugapi.New(&debugAPIFacade{ac: ac}, port, token)
	if err != nil {
		return err
	}
	debugAPIServer = s
	debugAPIServer.Start()
	return nil
}

// StopDebugAPI shuts the server down if running. No-op otherwise.
func (ac *AppController) StopDebugAPI() {
	if debugAPIServer == nil {
		return
	}
	debugAPIServer.Stop()
	debugAPIServer = nil
}

// DebugAPIAddr returns the bound "127.0.0.1:port" string if running,
// otherwise empty. Useful for the UI to show a copyable example URL.
func (ac *AppController) DebugAPIAddr() string {
	if debugAPIServer == nil {
		return ""
	}
	return debugAPIServer.Addr()
}
