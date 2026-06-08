package api

import (
	"fmt"
	"os"
	"sync"

	"singbox-launcher/internal/debuglog"
)

var (
	// apiLogSinkMu guards both apiLogFile and apiLogSink: they are set/cleared
	// from the UI thread (open/close log files, open/close the log viewer) but
	// read from request-handler goroutines in writeLog. Without it, a shutdown
	// SetAPILogFile(nil) races writeLog → data race + write-after-close.
	apiLogSinkMu sync.RWMutex
	apiLogFile   *os.File                     // api.log target; set via SetAPILogFile, nil before close
	apiLogSink   func(debuglog.Level, string) // optional diagnostics log-viewer callback
)

// SetAPILogFile sets the log file for API requests. Call after opening log files, pass nil before closing.
func SetAPILogFile(f *os.File) {
	apiLogSinkMu.Lock()
	defer apiLogSinkMu.Unlock()
	apiLogFile = f
}

// SetAPILogSink sets an optional callback for the diagnostics log viewer (API tab).
// The callback receives (level, line) for each writeLog() and must not block.
// Call ClearAPILogSink when the log viewer window is closed.
func SetAPILogSink(fn func(debuglog.Level, string)) {
	apiLogSinkMu.Lock()
	defer apiLogSinkMu.Unlock()
	apiLogSink = fn
}

// ClearAPILogSink removes the API log sink (e.g. when the log viewer is closed).
func ClearAPILogSink() {
	SetAPILogSink(nil)
}

// writeLog writes to api.log when level <= GlobalLevel (same rule as debuglog.Log).
func writeLog(level debuglog.Level, format string, args ...interface{}) {
	if level > debuglog.GlobalLevel {
		return
	}
	line := fmt.Sprintf(format, args...)
	apiLogSinkMu.RLock()
	f := apiLogFile
	fn := apiLogSink
	apiLogSinkMu.RUnlock()
	if f != nil {
		_, _ = fmt.Fprintf(f, format, args...)
	}
	if fn != nil {
		fn(level, line)
	}
}
