//go:build !windows
// +build !windows

package platform

import "os"

// ChmodExecutable выставляет бит исполняемости на файле (rwxr-xr-x).
func ChmodExecutable(path string) error {
	return os.Chmod(path, 0755)
}
