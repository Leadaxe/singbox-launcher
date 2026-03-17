//go:build windows
// +build windows

package platform

// ChmodExecutable — no-op на Windows: исполняемость определяется расширением файла (.exe).
func ChmodExecutable(_ string) error {
	return nil
}
