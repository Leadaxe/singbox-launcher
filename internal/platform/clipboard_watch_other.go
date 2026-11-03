//go:build !darwin && !windows

package platform

// ClipboardChangeCount — на этой платформе счётчика изменений буфера нет.
func ClipboardChangeCount() (uint64, bool) { return 0, false }

// ClipboardConcealed — пометки чувствительного содержимого здесь не знаем.
func ClipboardConcealed() bool { return false }
