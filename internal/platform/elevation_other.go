//go:build !windows

package platform

import (
	"os"
	"time"
)

// IsElevated — процесс работает от root. Вне Windows в гейтах SPEC 139 не
// участвует: они включаются только на Windows.
func IsElevated() bool { return os.Geteuid() == 0 }

// ElevationAsksOtherAccount — только Windows.
func ElevationAsksOtherAccount() bool { return false }

// ElevatedViaUAC — только Windows.
func ElevatedViaUAC() bool { return false }

// AdminCleanupTasks — очистки старта, которым нужны права администратора;
// вне Windows таких нет.
func AdminCleanupTasks() []string { return nil }

// ElevatedProcess — процесс, запущенный RunElevated; вне Windows не
// создаётся.
type ElevatedProcess struct {
	Pid int
}

// Wait — только Windows.
func (p *ElevatedProcess) Wait(timeout time.Duration) (exitCode uint32, exited bool, err error) {
	_ = timeout
	return 0, false, ErrElevationNotSupported
}

// Close — только Windows.
func (p *ElevatedProcess) Close() error { return nil }

// RunElevated — только Windows.
func RunElevated(exe string, args []string, dir string, show int) (*ElevatedProcess, error) {
	_, _, _, _ = exe, args, dir, show
	return nil, ErrElevationNotSupported
}

// WaitForProcessExit ждёт выхода процесса pid не дольше timeout. Вне
// Windows -handoff не передаётся (перезапуск с повышением — только Windows),
// поэтому здесь только опрос списка процессов, без сверки образа.
func WaitForProcessExit(pid int, exe string, timeout time.Duration) (exited bool, err error) {
	_ = exe
	if pid <= 0 {
		return true, nil
	}
	return pollProcessGone(pid, timeout)
}
