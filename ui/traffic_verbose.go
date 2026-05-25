package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"singbox-launcher/core"
)

// ReadCurrentLogLevel — thin shim around core.ReadCurrentLogLevelFromState.
// Kept here as the canonical UI-facing accessor; the implementation moved
// to core/log_level.go so SPEC 059 debugapi /traffic/verbose can read the
// level without importing ui.
func ReadCurrentLogLevel(ac *core.AppController) (string, bool, error) {
	return core.ReadCurrentLogLevelFromState(ac)
}

// ApplyLogLevelAndReload — thin shim around core.ApplyLogLevelAndReloadCore.
// Active TCP/UDP connections will reset when sing-box restarts; UI callers
// should go through ConfirmAndApplyLogLevel below to show the confirm dialog.
func ApplyLogLevelAndReload(ac *core.AppController, level string) error {
	return core.ApplyLogLevelAndReloadCore(ac, level)
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
