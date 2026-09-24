//go:build !darwin && (!windows || 386)

package core

import (
	"errors"
	"os"

	"fyne.io/fyne/v2"

	"singbox-launcher/internal/dialogs"
)

// privilegedCoreCopyGate — заглушка вне daemon-платформ (Linux, Win7):
// привилегированный старт classic есть только на macOS (SPEC 137); сюда не
// доходят: Start зовёт его под GOOS == darwin.
func (ac *AppController) privilegedCoreCopyGate() (string, error) {
	return "", errors.New("privileged start is only available on macOS")
}

// notifyPrivilegedCopyAfterCoreUpdate — копии для старта с TUN вне macOS нет.
func (ac *AppController) notifyPrivilegedCopyAfterCoreUpdate() {}

// classicElevatedUsesCopy — защищённой копии вне macOS и Windows x64/arm64 нет.
func classicElevatedUsesCopy() bool { return false }

// elevatedClassicStart — только Windows x64/arm64 (SPEC 141 §8).
func (ac *AppController) elevatedClassicStart() (string, *os.File, error) {
	return "", nil, errors.New("elevated classic start is not available on this platform")
}

// tunInstallServiceAction — службы нет (Linux, Win7): кнопки Install service нет.
func (ac *AppController) tunInstallServiceAction() (dialogs.Action, bool) {
	return dialogs.Action{}, false
}

// ShowDaemonUnsafeNoticeElevated — службы нет: показывать нечего.
func (ac *AppController) ShowDaemonUnsafeNoticeElevated(_ fyne.Window, _, _, _ string) bool {
	return false
}
