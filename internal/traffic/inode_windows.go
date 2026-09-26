//go:build windows

package traffic

import (
	"os"

	"golang.org/x/sys/windows"
)

// inode on Windows: no real inode number. We use file size + mtime in the
// tailer's rotation check, so returning a constant 0 is fine — the size /
// stat-fails-then-reopen path catches rotation on Windows.
func inode(fi os.FileInfo) uint64 { return 0 }

// openShared — чтение с FILE_SHARE_DELETE: os.Open его не ставит, и пока
// тейлер держит лог ядра, rename-ротация (classic.log → .old) падает
// «used by another process» и валит старт ядра.
func openShared(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
