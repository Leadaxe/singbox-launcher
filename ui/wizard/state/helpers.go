package state

import (
	"fyne.io/fyne/v2"
	"singbox-launcher/internal/debuglog"
)

// LogLevel represents the logging level.
type LogLevel = debuglog.Level

// Log level constants
const (
	LogLevelOff     = debuglog.LevelOff
	LogLevelError   = debuglog.LevelError
	LogLevelWarn    = debuglog.LevelWarn
	LogLevelInfo    = debuglog.LevelInfo
	LogLevelVerbose = debuglog.LevelVerbose
	LogLevelTrace   = debuglog.LevelTrace
)

// SetLogLevel sets the global logging level.
func SetLogLevel(level LogLevel) {
	debuglog.GlobalLevel = level
}

// GetLogLevel returns the current global logging level.
func GetLogLevel() LogLevel {
	return debuglog.GlobalLevel
}

// SafeFyneDo safely calls fyne.Do only if window is still valid.
func SafeFyneDo(window fyne.Window, fn func()) {
	if window != nil {
		fyne.Do(fn)
	}
}

// DebugLog logs a debug message using the debuglog subsystem.
func DebugLog(format string, args ...interface{}) {
	debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, format, args...)
}

// InfoLog logs an info message using the debuglog subsystem.
func InfoLog(format string, args ...interface{}) {
	debuglog.Log("INFO", debuglog.LevelInfo, debuglog.UseGlobal, format, args...)
}

// ErrorLog logs an error message using the debuglog subsystem.
func ErrorLog(format string, args ...interface{}) {
	debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, format, args...)
}

