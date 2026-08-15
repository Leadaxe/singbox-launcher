//go:build !windows
// +build !windows

package platform

import "os"

// RunGLProbeChild — тело подпроцесса `-gl-probe`. Пробник нужен только на
// Windows (issue #105, RDP без аппаратного OpenGL); на остальных платформах
// флаг не используется, но обработчик обязан завершить процесс, а не
// продолжить обычный запуск.
func RunGLProbeChild() {
	os.Exit(0)
}

// EnsureDesktopOpenGL — no-op вне Windows: на macOS/Linux проблемы
// «GDI Generic 1.1 в RDP-сессии» не существует.
func EnsureDesktopOpenGL(_ string) {}
