//go:build !darwin

package core

import "fmt"

// newDaemonBackend — заглушка: daemon-режим (sing-box lxd + launchd) пока
// реализован только на macOS; на остальных платформах форк не предоставляет
// установку службы (`--service` возвращает "not implemented").
func newDaemonBackend(_ *AppController) (CoreBackend, error) {
	return nil, fmt.Errorf("daemon mode is only available on macOS")
}

// notifyDaemonServiceAfterCoreUpdate — no-op вне macOS (службы демона нет).
func (ac *AppController) notifyDaemonServiceAfterCoreUpdate() {}
