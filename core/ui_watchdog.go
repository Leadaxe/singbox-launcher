package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
)

// UI watchdog: a background goroutine pings the Fyne main loop and, when the
// loop stops answering, writes a full goroutine dump to <LogDir>/ui-freeze-*.txt.
// A frozen window leaves no trace in the log (the Go side keeps running, only
// the main thread is stuck), and on Windows there is no SIGQUIT to get stacks.
const (
	uiWatchdogTick = 10 * time.Second
	// uiWatchdogMissedTicks — how many ticks in a row the probe may stay
	// unanswered before the UI counts as frozen (3 × 10 s = 30 s). Ticks, not
	// wall time: a system sleep between two ticks must not look like a freeze.
	uiWatchdogMissedTicks = 3
	uiFreezeDumpMax       = 8 << 20
	uiFreezeDumpKeep      = 3
	uiFreezeDumpPrefix    = "ui-freeze-"
)

// StartUIWatchdog starts the UI-thread watchdog. Call once, after the Fyne
// event loop has started; it stops when the controller context is cancelled
// (GracefulExit).
func (ac *AppController) StartUIWatchdog() {
	if ac.ctx == nil || ac.FileService == nil {
		return
	}
	go runUIWatchdog(ac.ctx, ac.FileService.Layout.Logs.String())
}

func runUIWatchdog(ctx context.Context, logDir string) {
	var lastAlive atomic.Int64 // unix nanos of the last probe run on the UI thread
	var pending atomic.Bool    // a probe is queued and not yet run
	lastAlive.Store(time.Now().UnixNano())

	var probeSentAt time.Time
	missed := 0
	frozen := false

	ticker := time.NewTicker(uiWatchdogTick)
	defer ticker.Stop()
	debuglog.DebugLog("ui watchdog: started (tick %s, freeze after %d missed ticks)", uiWatchdogTick, uiWatchdogMissedTicks)

	for {
		select {
		case <-ctx.Done():
			debuglog.DebugLog("ui watchdog: stopped")
			return
		case <-ticker.C:
		}

		if !pending.Load() {
			if frozen {
				stall := time.Unix(0, lastAlive.Load()).Sub(probeSentAt).Round(time.Second)
				debuglog.InfoLog("ui watchdog: UI thread responsive again after %s", stall)
				frozen = false
			}
			missed = 0
			probeSentAt = time.Now()
			pending.Store(true)
			// fyne.Do only enqueues into the driver's unbounded queue and never
			// blocks, so the watchdog keeps ticking even when the loop is stuck.
			fyne.Do(func() {
				lastAlive.Store(time.Now().UnixNano())
				pending.Store(false)
			})
			continue
		}

		missed++
		if frozen || missed < uiWatchdogMissedTicks {
			continue
		}
		frozen = true
		stall := time.Since(probeSentAt).Round(time.Second)
		// Dump first, log second: if the freeze is a stuck log writer, the
		// WarnLog below blocks too, but the dump is already on disk.
		path, err := writeUIFreezeDump(logDir, time.Unix(0, lastAlive.Load()), stall)
		debuglog.WarnLog("ui watchdog: UI thread unresponsive for %s", stall)
		if err != nil {
			debuglog.ErrorLog("ui watchdog: write goroutine dump: %v", err)
			continue
		}
		debuglog.WarnLog("ui watchdog: goroutine dump written to %s", path)
	}
}

// writeUIFreezeDump writes a header plus the stacks of all goroutines to
// <logDir>/ui-freeze-<yyyymmdd-hhmmss>.txt and keeps only the newest
// uiFreezeDumpKeep such files. Returns the path of the new file.
func writeUIFreezeDump(logDir string, lastAlive time.Time, stall time.Duration) (string, error) {
	now := time.Now()
	var b bytes.Buffer
	fmt.Fprintf(&b, "UI freeze: the Fyne main loop did not answer for %s\n", stall)
	fmt.Fprintf(&b, "launcher: %s\nplatform: %s/%s\ncaptured: %s\nlast UI response: %s\ngoroutines: %d\n\n",
		constants.AppVersion, runtime.GOOS, runtime.GOARCH,
		now.Format(time.RFC3339), lastAlive.Format(time.RFC3339), runtime.NumGoroutine())
	b.Write(allGoroutineStacks())

	path := filepath.Join(logDir, uiFreezeDumpPrefix+now.Format("20060102-150405")+".txt")
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		return "", err
	}
	pruneUIFreezeDumps(logDir)
	return path, nil
}

// allGoroutineStacks returns runtime.Stack(all) output; the buffer doubles
// until the dump fits or reaches uiFreezeDumpMax (then the tail is cut).
func allGoroutineStacks() []byte {
	for size := 1 << 20; ; size *= 2 {
		buf := make([]byte, size)
		n := runtime.Stack(buf, true)
		if n < size || size >= uiFreezeDumpMax {
			return buf[:n]
		}
	}
}

// pruneUIFreezeDumps removes all but the newest uiFreezeDumpKeep dumps. The
// timestamp in the name sorts lexicographically in time order.
func pruneUIFreezeDumps(logDir string) {
	files, err := filepath.Glob(filepath.Join(logDir, uiFreezeDumpPrefix+"*.txt"))
	if err != nil {
		debuglog.WarnLog("ui watchdog: list old dumps: %v", err)
		return
	}
	sort.Strings(files)
	for len(files) > uiFreezeDumpKeep {
		if err := os.Remove(files[0]); err != nil {
			debuglog.WarnLog("ui watchdog: remove old dump %s: %v", files[0], err)
		}
		files = files[1:]
	}
}
