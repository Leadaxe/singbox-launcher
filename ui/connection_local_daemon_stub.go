//go:build !darwin && (!windows || 386)

package ui

import (
	"fyne.io/fyne/v2"

	"singbox-launcher/core"
)

// buildDaemonPanel — daemon-движок (sing-box lxd) есть только на
// daemon-платформах (macOS, Windows x64/arm64); на остальных (Linux, Win7)
// вкладка LOCAL показывает только classic-движок.
func buildDaemonPanel(_ *core.AppController, _ fyne.Window, _ func()) fyne.CanvasObject {
	return nil
}

// daemonPurgeRow — службы демона здесь нет (DaemonUninstallHint всегда
// пуст), строка не показывается; копия без запуска — на всякий случай.
func daemonPurgeRow(_ *core.AppController, win fyne.Window, _ bool, command func() (string, error)) fyne.CanvasObject {
	return CommandRow(win, "", command, false)
}
