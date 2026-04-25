package core

import (
	"errors"
	"time"

	"singbox-launcher/api"
	"singbox-launcher/core/debugapi"
	"singbox-launcher/internal/constants"
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
