// Package core — log_level helper extracted from ui/traffic_verbose.go so
// out-of-process callers (debugapi SPEC 059 /traffic/verbose endpoint) can
// flip the log level without dragging the UI layer in. The UI wrapper in
// ui/traffic_verbose.go is now a thin shim around these helpers.
//
// The reload semantics are unchanged from the original UI implementation:
// mutate state.vars[log_level] → Save → RebuildConfigIfDirty(forced=true)
// → KillSingBoxForRestart so the monitor brings sing-box back up with the
// new level. Active TCP/UDP connections are reset by the restart — callers
// are responsible for warning the user before calling.
package core

import (
	"errors"
	"fmt"
	"strings"

	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// TrafficLogLevelVar — wizard var key that drives sing-box config.log.level.
const TrafficLogLevelVar = "log_level"

// ReadCurrentLogLevelFromState reads vars[log_level] from state.json on
// disk. Returns ("", false, nil) if state is missing or the var is unset
// (caller should treat that as "template default", which is `warn`).
func ReadCurrentLogLevelFromState(ac *AppController) (string, bool, error) {
	if ac == nil || ac.FileService == nil {
		return "", false, errors.New("core: no controller")
	}
	statePath := platform.GetWizardStatePath(ac.FileService.ExecDir)
	s, err := state.Load(statePath)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("load state: %w", err)
	}
	for _, v := range s.Vars {
		if v.Name == TrafficLogLevelVar {
			return strings.TrimSpace(v.Value), true, nil
		}
	}
	return "", false, nil
}

// ApplyLogLevelAndReloadCore mutates state.vars[log_level], saves, rebuilds
// config.json (forced), then restarts sing-box so the new level takes
// effect on the next process cycle.
//
// CRITICAL UX: sing-box restart RESETS active TCP connections. The UI
// wrapper (ConfirmAndApplyLogLevel in ui/traffic_verbose.go) shows a
// confirmation dialog before invoking this. The debugapi endpoint sets
// 202 Accepted with a warning string so machine callers know to expect
// the disruption.
func ApplyLogLevelAndReloadCore(ac *AppController, level string) error {
	if ac == nil || ac.FileService == nil {
		return errors.New("core: no controller")
	}
	statePath := platform.GetWizardStatePath(ac.FileService.ExecDir)
	s, err := state.Load(statePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	found := false
	for i := range s.Vars {
		if s.Vars[i].Name == TrafficLogLevelVar {
			s.Vars[i].Value = level
			found = true
			break
		}
	}
	if !found {
		s.Vars = append(s.Vars, state.SettingVar{Name: TrafficLogLevelVar, Value: level})
	}
	if err := s.Save(statePath); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	if err := ac.RebuildConfigIfDirty(true); err != nil {
		return fmt.Errorf("rebuild config: %w", err)
	}
	if ac.RunningState != nil && ac.RunningState.IsRunning() {
		KillSingBoxForRestart()
		debuglog.InfoLog("core: requested sing-box restart for log_level=%s", level)
	}
	return nil
}
