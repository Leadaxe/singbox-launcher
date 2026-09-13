//go:build windows

package debuglog

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// nativeStderrFile — файл держится открытым весь сеанс: закрой его, и хендл,
// отданный в SetStdHandle, станет невалидным.
var nativeStderrFile *os.File

// RedirectNativeStderr уводит stderr процесса в файл path.
//
// Зачем: Windows-бинарь собран с -H windowsgui, то есть подсистема WINDOWS и
// консоли нет. STD_ERROR_HANDLE у такого процесса — INVALID_HANDLE_VALUE, и
// всё, что пишет в stderr чужой нативный код (Mesa, драйверы GPU, GLFW),
// пропадает бесследно. Именно поэтому падение под Mesa 08.09.2026 не оставило
// ни строки нигде: паники Go не было, значит SetCrashOutput молчал, а
// диагностика драйвера уходила в никуда.
//
// Это не прикладное логирование (конституция §5): наш код по-прежнему пишет
// через debuglog, здесь подменяется системный хендл для чужого кода.
// Дополняет EnableCrashOutput, который ловит только панику Go.
func RedirectNativeStderr(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("native stderr: mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("native stderr: open: %w", err)
	}
	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd())); err != nil {
		_ = f.Close()
		return fmt.Errorf("native stderr: SetStdHandle: %w", err)
	}
	// os.Stderr кешируется рантаймом при старте, SetStdHandle его не меняет —
	// подменяем и его, иначе Go-код и нативный код писали бы в разные места.
	os.Stderr = f
	nativeStderrFile = f
	return nil
}
