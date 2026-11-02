//go:build windows

package platform

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser32ClipWatch             = windows.NewLazySystemDLL("user32.dll")
	procGetClipboardSequenceNumber = modUser32ClipWatch.NewProc("GetClipboardSequenceNumber")
	procRegisterClipboardFormatW   = modUser32ClipWatch.NewProc("RegisterClipboardFormatW")
	procIsClipboardFormatAvailable = modUser32ClipWatch.NewProc("IsClipboardFormatAvailable")
)

// ClipboardChangeCount — счётчик изменений системного буфера (растёт на
// каждое копирование, даже того же текста). ok=false — платформа не умеет.
func ClipboardChangeCount() (uint64, bool) {
	if procGetClipboardSequenceNumber.Find() != nil {
		return 0, false
	}
	n, _, _ := procGetClipboardSequenceNumber.Call()
	return uint64(n), true
}

// ClipboardConcealed — содержимое буфера помечено чувствительным: формат
// ExcludeClipboardContentFromMonitorProcessing ставят менеджеры паролей.
func ClipboardConcealed() bool {
	if procRegisterClipboardFormatW.Find() != nil || procIsClipboardFormatAvailable.Find() != nil {
		return false
	}
	name, err := windows.UTF16PtrFromString("ExcludeClipboardContentFromMonitorProcessing")
	if err != nil {
		return false
	}
	format, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(name)))
	if format == 0 {
		return false
	}
	ok, _, _ := procIsClipboardFormatAvailable.Call(format)
	return ok != 0
}
