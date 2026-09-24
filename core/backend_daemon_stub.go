//go:build !darwin && (!windows || 386)

package core

import "fmt"

// newDaemonBackend — заглушка вне daemon-платформ (Linux, Win7 windows/386):
// daemon-режим (sing-box lxd как служба ОС) есть на macOS и Windows x64/arm64
// (SPEC 141); здесь форк не предоставляет установку службы (`--service`
// возвращает "not implemented").
func newDaemonBackend(_ *AppController) (CoreBackend, error) {
	return nil, fmt.Errorf("daemon mode is only available on macOS")
}

// notifyDaemonServiceAfterCoreUpdate — no-op вне daemon-платформ (службы демона нет).
func (ac *AppController) notifyDaemonServiceAfterCoreUpdate() {}

// DaemonUnsafeServiceNotice — службы демона вне daemon-платформ нет (SPEC 136).
func (ac *AppController) DaemonUnsafeServiceNotice() (servicePath, command, coreHint string, due bool) {
	return "", "", "", false
}
