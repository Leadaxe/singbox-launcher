package ui

import (
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"singbox-launcher/core"
	"singbox-launcher/core/state"
	v5 "singbox-launcher/core/state/v5"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// trafficLogLevelVar is the wizard var key that drives sing-box
// config.log.level.
const trafficLogLevelVar = "log_level"

// ReadCurrentLogLevel reads vars[log_level] from state.json. Returns
// empty string if state is missing or var not set (caller treats as
// "use template default", which is `warn` per wizard_template.json).
func ReadCurrentLogLevel(ac *core.AppController) (string, bool, error) {
	if ac == nil || ac.FileService == nil {
		return "", false, errors.New("ui/traffic: no controller")
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
		if v.Name == trafficLogLevelVar {
			return strings.TrimSpace(v.Value), true, nil
		}
	}
	return "", false, nil
}

// ApplyLogLevelAndReload mutates state.vars[log_level], saves, rebuilds
// config.json, then restarts sing-box so the new level takes effect.
//
// CRITICAL UX: sing-box restart RESETS active TCP connections. Caller
// MUST show a confirmation dialog before invoking this (the user is
// going to lose any open Slack call / WireGuard tunnel etc.). The
// dialog is implemented in ConfirmAndApplyLogLevel below — call that
// instead of this function directly.
func ApplyLogLevelAndReload(ac *core.AppController, level string) error {
	if ac == nil || ac.FileService == nil {
		return errors.New("ui/traffic: no controller")
	}
	statePath := platform.GetWizardStatePath(ac.FileService.ExecDir)
	s, err := state.Load(statePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	// Update or insert.
	found := false
	for i := range s.Vars {
		if s.Vars[i].Name == trafficLogLevelVar {
			s.Vars[i].Value = level
			found = true
			break
		}
	}
	if !found {
		s.Vars = append(s.Vars, v5.SettingVar{Name: trafficLogLevelVar, Value: level})
	}
	if err := s.Save(statePath); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	// Rebuild config.json from template + state (forced so the new var
	// definitely makes it in).
	if err := ac.RebuildConfigIfDirty(true); err != nil {
		return fmt.Errorf("rebuild config: %w", err)
	}
	// Restart so sing-box picks up the new log level. We use
	// KillSingBoxForRestart which preserves StoppedByUser=false so the
	// monitor brings it back up.
	if ac.RunningState != nil && ac.RunningState.IsRunning() {
		core.KillSingBoxForRestart()
		debuglog.InfoLog("ui/traffic: requested sing-box restart for log_level=%s", level)
	}
	return nil
}

// ConfirmAndApplyLogLevel asks for user confirmation (one-line modal
// explaining that active connections will reset), then applies. onDone
// fires on success only. Errors are surfaced via dialog.
func ConfirmAndApplyLogLevel(ac *core.AppController, parent fyne.Window, level string, onDone func()) {
	if parent == nil {
		return
	}
	msg := fmt.Sprintf("Switching sing-box log level to %q will reload the engine — active TCP/UDP connections will be reset.\n\nContinue?", level)
	dialog.ShowConfirm("Reload sing-box?", msg, func(yes bool) {
		if !yes {
			return
		}
		if err := ApplyLogLevelAndReload(ac, level); err != nil {
			dialog.ShowError(err, parent)
			return
		}
		if onDone != nil {
			onDone()
		}
	}, parent)
}
