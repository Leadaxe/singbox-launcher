package platform

import (
	"errors"
	"fmt"
	"time"

	"singbox-launcher/internal/process"
)

// Права по требованию (SPEC 139): общие объявления для Windows и прочих ОС.
// Реализация — elevation_windows.go, заглушки — elevation_other.go.

// ErrElevationCancelled — пользователь отказал в запросе UAC
// (ERROR_CANCELLED от ShellExecuteExW). Это не сбой: вызывающий пишет INFO и
// оставляет диалог открытым со строкой отмены (SPEC 139 §5 п. 4, SPEC 141 §5.3).
var ErrElevationCancelled = errors.New("the administrator prompt was cancelled")

// ErrElevationNotSupported — запуск с повышением есть только на Windows.
var ErrElevationNotSupported = errors.New("elevation is not supported on this platform")

// Как показать окно процесса, запущенного RunElevated (nShow ShellExecuteExW).
const (
	// ElevatedShowNormal — SW_SHOWNORMAL: перезапуск лаунчера (SPEC 139 §5).
	ElevatedShowNormal = 1
	// ElevatedShowHidden — SW_HIDE: консольные команды ядра (SPEC 141 §5.2).
	ElevatedShowHidden = 0
)

// processPollInterval — шаг опроса списка процессов, когда дескриптор
// процесса не открыть (SPEC 139 §5 п. 6).
const processPollInterval = 250 * time.Millisecond

// pollProcessGone ждёт, пока PID пропадёт из списка процессов, не дольше
// timeout. Список процессов (снимок Toolhelp на Windows) виден и для
// процессов другой учётной записи, дескриптор которых не открыть.
func pollProcessGone(pid int, timeout time.Duration) (gone bool, err error) {
	deadline := time.Now().Add(timeout)
	for {
		_, found, err := process.FindProcess(pid)
		if err != nil {
			return false, fmt.Errorf("list processes: %w", err)
		}
		if !found {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(processPollInterval)
	}
}
