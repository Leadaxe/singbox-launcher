//go:build !windows
// +build !windows

package platform

import "os"

// RunGLProbeChild — тело подпроцесса `-gl-probe` / `-gl-probe-local`. Пробник
// нужен только на Windows (issue #105, RDP без аппаратного OpenGL); на
// остальных платформах флаги не используются, но обработчик обязан завершить
// процесс, а не продолжить обычный запуск.
func RunGLProbeChild(_ bool) {
	os.Exit(0)
}

// EnsureDesktopOpenGL — no-op вне Windows: на macOS/Linux проблемы
// «GDI Generic 1.1 в RDP-сессии» не существует, а значит нет ни Mesa3D,
// ни состояния гейта.
func EnsureDesktopOpenGL(_ string, _ bool) {}
