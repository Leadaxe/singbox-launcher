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

// buildRemoteDaemonPanel — удалённый демон управляется darwin-only клиентом.
func buildRemoteDaemonPanel(_ *core.AppController, _ fyne.Window, _ func()) fyne.CanvasObject {
	return nil
}

// connectionScopeIsRemote — вне macOS удалённого демона нет.
func connectionScopeIsRemote(_ *core.AppController) bool { return false }
