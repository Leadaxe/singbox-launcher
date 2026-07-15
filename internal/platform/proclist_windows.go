//go:build windows

package platform

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// listProcessesImpl on Windows uses the psapi pair EnumProcesses +
// QueryFullProcessImageNameW. We don't bother with snapshot APIs
// (CreateToolhelp32Snapshot) — psapi has been the official path since
// Vista and is what most Win32 sample code uses.
//
// We skip:
//   - PID 0 and 4 (System Idle, kernel) — they don't have an executable
//     image and OpenProcess fails with access-denied anyway.
//   - Processes owned by other users (ERROR_ACCESS_DENIED). User-level
//     services like svchost.exe will be invisible; that matches macOS
//     /Linux behavior and the SPEC's "best effort".
//
// Icon extraction is intentionally omitted on MVP. The picker falls
// back to a generic file icon; if users complain we'll add
// SHGetFileInfoW with SHGFI_ICON | SHGFI_SMALLICON later.
func listProcessesImpl() ([]ProcessEntry, error) {
	psapi := windows.NewLazySystemDLL("psapi.dll")
	enumProcesses := psapi.NewProc("EnumProcesses")

	// Start with 1024 PIDs and grow if returned size says we filled the
	// buffer (heuristic from MSDN's sample).
	const maxAttempts = 4
	pidBuf := make([]uint32, 1024)
	var bytesReturned uint32
	for attempt := 0; attempt < maxAttempts; attempt++ {
		r1, _, e1 := enumProcesses.Call(
			uintptr(unsafe.Pointer(&pidBuf[0])),
			uintptr(len(pidBuf)*4),
			uintptr(unsafe.Pointer(&bytesReturned)),
		)
		if r1 == 0 {
			return nil, e1
		}
		// If buffer filled exactly, grow and retry.
		if int(bytesReturned) >= len(pidBuf)*4 {
			pidBuf = make([]uint32, len(pidBuf)*2)
			continue
		}
		break
	}
	n := int(bytesReturned) / 4
	pidBuf = pidBuf[:n]

	out := make([]ProcessEntry, 0, n)
	seen := make(map[string]struct{})
	for _, pid := range pidBuf {
		if pid == 0 || pid == 4 {
			continue
		}
		path, err := processImagePath(pid)
		if err != nil || path == "" {
			continue
		}
		if _, dup := seen[path]; dup {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, ProcessEntry{
			PID:         int(pid),
			Path:        path,
			DisplayName: winDisplayName(path),
		})
	}
	return out, nil
}

func processImagePath(pid uint32) (string, error) {
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	h, err := windows.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer func() { _ = windows.CloseHandle(h) }()

	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	qfpin := kernel32.NewProc("QueryFullProcessImageNameW")

	buf := make([]uint16, syscall.MAX_PATH)
	size := uint32(len(buf))
	r1, _, e1 := qfpin.Call(
		uintptr(h),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 {
		return "", e1
	}
	return syscall.UTF16ToString(buf[:size]), nil
}

func winDisplayName(path string) string {
	base := filepath.Base(path)
	// Strip ".exe" for display — looks cleaner in the picker.
	if len(base) > 4 && (base[len(base)-4:] == ".exe" || base[len(base)-4:] == ".EXE") {
		return base[:len(base)-4]
	}
	return base
}
