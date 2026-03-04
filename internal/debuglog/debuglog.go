// Package debuglog provides a centralized logging system with configurable log levels.
//
// The package supports multiple log levels (Off, Error, Warn, Info, Verbose/Debug, Trace).
// Level is set at build time: release (tag) builds use Warn, dev (GH prerelease/local) builds use Debug.
//
// Usage:
//
//	debuglog.DebugLog("Processing item %d", 42)
//	debuglog.InfoLog("Operation completed successfully")
//	debuglog.WarnLog("Deprecated function used")
//	debuglog.ErrorLog("Failed to process: %v", err)
//
//	// Timing operations
//	timing := debuglog.StartTiming("processData")
//	defer timing.EndWithDefer()
//
//	// Log large text fragments with automatic truncation
//	debuglog.LogTextFragment("Parser", debuglog.LevelVerbose, "Config content", configText, 500)
//
// Optional internal log sink: SetInternalLogSink sets a callback that receives (level, line) for every
// Log() call; used by the diagnostics log viewer window. The callback is invoked from any goroutine
// and must not block (e.g. push to a channel; UI updates via fyne.Async). The viewer filters by level.
package debuglog

import (
	"fmt"
	"log"
	"sync"
	"time"

	"singbox-launcher/internal/constants"
)

// Level represents the log level threshold.
// Higher values mean more verbose logging.
type Level uint8

const (
	// LevelOff (0) disables all logging.
	LevelOff Level = iota

	// LevelError (1) shows only error messages.
	LevelError

	// LevelWarn (2) shows warnings and errors.
	LevelWarn

	// LevelInfo (3) shows informational messages, warnings, and errors.
	LevelInfo

	// LevelVerbose (4, also known as Debug) shows detailed debug information.
	LevelVerbose

	// LevelTrace (5) shows the most detailed information including trace logs.
	LevelTrace
)

var (
	// GlobalLevel is the global log level threshold.
	// Release (tag) builds: LevelWarn. Dev (GH prerelease/local) builds: LevelVerbose (debug).
	GlobalLevel = defaultLevelByBuild()

	internalLogSinkMu sync.RWMutex
	internalLogSink   func(Level, string)
)

// defaultLevelByBuild returns LevelWarn for release builds (version from tag, no "-"),
// LevelVerbose (debug) for dev builds (prerelease, local, version with "-").
func defaultLevelByBuild() Level {
	if constants.GetMyBranch() == "main" {
		return LevelWarn
	}
	return LevelVerbose
}

// Log writes a log message with the specified prefix and level.
// The message is only logged if the level is less than or equal to GlobalLevel.
//
// Parameters:
//   - prefix: log prefix (e.g., "DEBUG", "ERROR"). If empty, no prefix is added.
//   - level: log level threshold for this message.
//   - format: format string (same as fmt.Printf).
//   - args: arguments for the format string.
func Log(prefix string, level Level, format string, args ...interface{}) {
	if level > GlobalLevel {
		return
	}
	message := fmt.Sprintf(format, args...)
	var line string
	if prefix != "" {
		line = fmt.Sprintf("[%s] %s", prefix, message)
		log.Printf("[%s] %s", prefix, message)
	} else {
		line = message
		log.Print(message)
	}
	internalLogSinkMu.RLock()
	fn := internalLogSink
	internalLogSinkMu.RUnlock()
	if fn != nil {
		// Include timestamp for the log viewer (file log uses log package's default format)
		lineWithTime := fmt.Sprintf("%s %s", time.Now().Format("2006-01-02 15:04:05"), line)
		fn(level, lineWithTime)
	}
}

// SetInternalLogSink sets an optional callback for the diagnostics log viewer.
// The callback receives (level, line) for every Log() call and must not block.
// Call ClearInternalLogSink when the viewer window is closed.
func SetInternalLogSink(fn func(Level, string)) {
	internalLogSinkMu.Lock()
	defer internalLogSinkMu.Unlock()
	internalLogSink = fn
}

// ClearInternalLogSink removes the internal log sink (e.g. when the log viewer is closed).
func ClearInternalLogSink() {
	SetInternalLogSink(nil)
}

// ShouldLog checks if a message with the given level would be logged.
// Returns true if level <= GlobalLevel.
func ShouldLog(level Level) bool {
	return level <= GlobalLevel
}

// LogTextFragment логирует фрагмент текста с автоматической обрезкой для читаемости.
// Для больших текстов показывает начало и конец, избегая захламления логов.
//
// Параметры:
//   - prefix: префикс модуля для логов
//   - level: уровень логирования
//   - description: описание фрагмента
//   - text: текст для логирования
//   - maxChars: максимум символов для показа (рекомендуется 500-1000)
func LogTextFragment(prefix string, level Level, description, text string, maxChars int) {
	if !ShouldLog(level) {
		return
	}

	textLen := len(text)

	// Если текст короткий, показываем полностью
	if textLen <= maxChars*2 {
		Log(prefix, level, "%s (len=%d): %s", description, textLen, text)
		return
	}

	// Для длинных текстов показываем начало и конец
	Log(prefix, level, "%s (len=%d): first %d chars: %s",
		description, textLen, maxChars, text[:maxChars])
	Log(prefix, level, "%s (len=%d): last %d chars: %s",
		description, textLen, maxChars, text[textLen-maxChars:])
}

// DebugLog logs a debug message (LevelVerbose) with "DEBUG" prefix.
func DebugLog(format string, args ...interface{}) {
	Log("DEBUG", LevelVerbose, format, args...)
}

// InfoLog logs an info message (LevelInfo) with "INFO" prefix.
func InfoLog(format string, args ...interface{}) {
	Log("INFO", LevelInfo, format, args...)
}

// ErrorLog logs an error message (LevelError) with "ERROR" prefix.
func ErrorLog(format string, args ...interface{}) {
	Log("ERROR", LevelError, format, args...)
}

// WarnLog logs a warning message (LevelWarn) with "WARN" prefix.
func WarnLog(format string, args ...interface{}) {
	Log("WARN", LevelWarn, format, args...)
}

// TimingContext tracks timing for a function execution.
// Use StartTiming to create a new context, then call End() or use EndWithDefer() for automatic logging.
//
// Example:
//
//	timing := debuglog.StartTiming("processData")
//	defer timing.EndWithDefer()
//	// ... your code ...
type TimingContext struct {
	startTime time.Time
	funcName  string
}

// StartTiming creates a new timing context and logs the start time.
// Returns a TimingContext that can be used to measure and log execution duration.
//
// Example:
//
//	timing := debuglog.StartTiming("myFunction")
//	defer timing.EndWithDefer()
func StartTiming(funcName string) *TimingContext {
	startTime := time.Now()
	DebugLog("%s: START at %s", funcName, startTime.Format("15:04:05.000"))
	return &TimingContext{
		startTime: startTime,
		funcName:  funcName,
	}
}

// End logs the total duration since StartTiming was called and returns the duration.
// This method should be called when the operation completes.
func (tc *TimingContext) End() time.Duration {
	duration := time.Since(tc.startTime)
	DebugLog("%s: END (total duration: %v)", tc.funcName, duration)
	return duration
}

// EndWithDefer returns a defer function for automatic logging.
// Use this with defer to automatically log timing when the function returns.
//
// Example:
//
//	timing := debuglog.StartTiming("myFunction")
//	defer timing.EndWithDefer()
func (tc *TimingContext) EndWithDefer() func() {
	return func() {
		tc.End()
	}
}

// LogTiming logs elapsed time for a specific operation within the timing context.
// Useful for logging intermediate operations while tracking overall execution time.
//
// Example:
//
//	timing := debuglog.StartTiming("processData")
//	defer timing.EndWithDefer()
//
//	start := time.Now()
//	doSomething()
//	timing.LogTiming("doSomething", time.Since(start))
func (tc *TimingContext) LogTiming(operation string, duration time.Duration) {
	DebugLog("%s: %s took %v", tc.funcName, operation, duration)
}
