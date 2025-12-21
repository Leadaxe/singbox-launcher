package state

import (
	"fyne.io/fyne/v2"
	"singbox-launcher/internal/debuglog"
)

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

