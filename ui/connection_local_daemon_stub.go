//go:build !darwin

package ui

import (
	"fyne.io/fyne/v2"

	"singbox-launcher/core"
)

// buildDaemonPanel — daemon-движок (sing-box lxd) доступен только на macOS;
// на остальных платформах вкладка LOCAL показывает только classic-движок.
func buildDaemonPanel(_ *core.AppController, _ fyne.Window, _ func()) fyne.CanvasObject {
	return nil
}
