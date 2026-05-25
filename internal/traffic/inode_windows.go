//go:build windows

package traffic

import "os"

// inode on Windows: no real inode number. We use file size + mtime in the
// tailer's rotation check, so returning a constant 0 is fine — the size /
// stat-fails-then-reopen path catches rotation on Windows.
func inode(fi os.FileInfo) uint64 { return 0 }
