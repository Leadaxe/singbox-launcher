//go:build darwin || linux

package traffic

import (
	"os"
	"syscall"
)

// inode returns the unix inode number for the file. We use it to detect
// rotation (file replaced with a fresh one).
func inode(fi os.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Ino)
	}
	return 0
}
