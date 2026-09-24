//go:build !darwin && (!windows || 386)

package core

import "errors"

// privilegedCoreCopyGate — заглушка вне daemon-платформ (Linux, Win7):
// привилегированный старт classic есть только на macOS (SPEC 137); сюда не
// доходят: Start зовёт его под GOOS == darwin.
func (ac *AppController) privilegedCoreCopyGate() (string, error) {
	return "", errors.New("privileged start is only available on macOS")
}

// notifyPrivilegedCopyAfterCoreUpdate — копии для старта с TUN вне macOS нет.
func (ac *AppController) notifyPrivilegedCopyAfterCoreUpdate() {}
