//go:build !windows
// +build !windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// RestartSelf запускает новую копию лаунчера с теми же аргументами и
// возвращает управление — завершить текущий процесс обязан вызывающий.
//
// Вне Windows нужен переключателю Portable (SPEC 135 §4.2): раскладка
// считается один раз при старте, и переезд данных применяет только новый
// процесс. Setsid отвязывает потомка от сессии и группы родителя, чтобы он
// пережил наш выход и не получил сигнал, адресованный терминалу родителя.
// Стандартные потоки не наследуются: exec с nil отдаёт /dev/null.
func RestartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("os.Executable: %w", err)
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("restart launcher: %w", err)
	}
	// Ждать потомка некому: процесс вот-вот завершится.
	_ = cmd.Process.Release()
	return nil
}
