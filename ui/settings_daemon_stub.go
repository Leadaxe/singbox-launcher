//go:build !darwin

package ui

import (
	"fyne.io/fyne/v2"

	"singbox-launcher/core"
)

// buildDaemonSection — daemon-режим (sing-box lxd) доступен только на macOS;
// на остальных платформах секция не отображается.
func buildDaemonSection(_ *core.AppController) fyne.CanvasObject {
	return nil
}
