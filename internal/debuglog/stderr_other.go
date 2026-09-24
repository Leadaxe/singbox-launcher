//go:build !windows

package debuglog

// RedirectNativeStderr — no-op вне Windows: там процесс запускается с живым
// stderr, и вывод чужого нативного кода не теряется.
func RedirectNativeStderr(string) error { return nil }

// releaseNativeStderr — no-op вне Windows: stderr не подменялся.
func releaseNativeStderr() {}
