//go:build windows
// +build windows

package platform

// Самоперезапуск лаунчера (SPEC 125 §6.2 R2).
//
// Зачем он вообще нужен: OPENGL32.dll стоит в таблице импорта exe, то есть
// загрузчик Windows отображает его при создании процесса — раньше любой строки
// нашего кода. Значит, сменить рендерер внутри уже живущего процесса нельзя в
// принципе: после переименования DLL старт продолжался с уже отображённой Mesa
// и падал (прогон RC 13.09.2026, два падения в native-stderr.log). Единственный
// способ применить переключение — новый процесс.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

// RestartSelf запускает новую копию лаунчера с теми же аргументами и
// возвращает управление — завершить текущий процесс обязан вызывающий.
//
// PrepareCommand здесь неприменим: он прячет окно (HideWindow), а новому
// процессу окно как раз нужно. DETACHED_PROCESS и CREATE_NEW_PROCESS_GROUP
// отвязывают потомка от консоли и группы родителя, чтобы он пережил наш
// os.Exit и не получил Ctrl+C, адресованный нам.
func RestartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("os.Executable: %w", err)
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("restart launcher: %w", err)
	}
	return nil
}
