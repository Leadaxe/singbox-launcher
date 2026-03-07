// Package ui: log viewer window for Diagnostics (Internal, Core, API tabs).

package ui

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
)

const (
	logViewerMaxLines = 300
	coreRefreshSec    = 5
)

var (
	logViewerMu     sync.Mutex
	logViewerWindow fyne.Window
)

type logEntry struct {
	level debuglog.Level
	line  string
}

// parseCoreLevel returns a level hint for Core tab line coloring (by keywords in the line).
func parseCoreLevel(line string) debuglog.Level {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") {
		return debuglog.LevelError
	}
	if strings.Contains(lower, "warn") {
		return debuglog.LevelWarn
	}
	if strings.Contains(lower, "info") {
		return debuglog.LevelInfo
	}
	if strings.Contains(lower, "debug") || strings.Contains(lower, "verbose") {
		return debuglog.LevelVerbose
	}
	if strings.Contains(lower, "trace") {
		return debuglog.LevelTrace
	}
	return debuglog.LevelInfo
}

func levelColor(l debuglog.Level) string {
	switch l {
	case debuglog.LevelError:
		return "[ERROR]"
	case debuglog.LevelWarn:
		return "[WARN]"
	case debuglog.LevelInfo:
		return "[INFO]"
	case debuglog.LevelVerbose:
		return "[DEBUG]"
	case debuglog.LevelTrace:
		return "[TRACE]"
	default:
		return ""
	}
}

// OpenLogViewerWindow opens a separate window with Internal, Core, and API log tabs.
// If the window is already open, focuses it instead of opening a duplicate.
// Registers sinks on show and clears them on close. Core tab uses ChildLogRelativePath from FileService.
func OpenLogViewerWindow(ac *core.AppController) {
	logViewerMu.Lock()
	if logViewerWindow != nil {
		w := logViewerWindow
		logViewerMu.Unlock()
		w.RequestFocus()
		return
	}
	logViewerMu.Unlock()

	app := ac.UIService.Application
	win := app.NewWindow("Logs")
	win.Resize(fyne.NewSize(700, 500))

	var (
		internalMu     sync.Mutex
		internalLines   []logEntry
		internalList    *widget.List
		internalLevel  debuglog.Level
		apiMu          sync.Mutex
		apiLines       []logEntry
		apiList        *widget.List
		apiLevel       debuglog.Level
		coreLines      []string
		coreList       *widget.List
		corePath     = filepath.Join(ac.FileService.ExecDir, ac.FileService.ChildLogRelativePath)
		internalCh   = make(chan logEntry, 64)
		apiCh        = make(chan logEntry, 64)
		coreTickStop func()
		coreTabIndex   int
		coreRefreshBtn *widget.Button
	)

	trimToMax := func(entries []logEntry) []logEntry {
		if len(entries) <= logViewerMaxLines {
			return entries
		}
		return entries[len(entries)-logViewerMaxLines:]
	}

	// Internal tab: store all entries; filter by level for display
	internalLevel = debuglog.LevelTrace
	internalList = widget.NewList(
		func() int {
			internalMu.Lock()
			n := 0
			for _, e := range internalLines {
				if e.level <= internalLevel {
					n++
				}
			}
			internalMu.Unlock()
			return n
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			internalMu.Lock()
			var e logEntry
			n := 0
			for _, ent := range internalLines {
				if ent.level <= internalLevel {
					n++
				}
			}
			// id 0 = newest (last filtered), id 1 = second newest, ...
			displayIdx := n - 1 - int(id)
			idx := 0
			for i := range internalLines {
				if internalLines[i].level <= internalLevel {
					if idx == displayIdx {
						e = internalLines[i]
						break
					}
					idx++
				}
			}
			internalMu.Unlock()
			lbl := o.(*widget.Label)
			// Line from debuglog already contains level prefix (e.g. [DEBUG], [ERROR])
			lbl.SetText(e.line)
		},
	)
	levelNames := []string{"Error", "Warn", "Info", "Verbose", "Trace"}
	levelByIndex := []debuglog.Level{debuglog.LevelError, debuglog.LevelWarn, debuglog.LevelInfo, debuglog.LevelVerbose, debuglog.LevelTrace}
	internalSelect := widget.NewSelect(levelNames, func(s string) {
		for i, n := range levelNames {
			if n == s {
				internalLevel = levelByIndex[i]
				break
			}
		}
		internalList.Refresh()
	})
	internalSelect.SetSelected("Trace")
	internalTop := container.NewHBox(widget.NewLabel("Level:"), internalSelect)
	internalContent := container.NewBorder(internalTop, nil, nil, nil, internalList)

	// API tab: store all; filter by level for display
	apiLevel = debuglog.LevelTrace
	apiList = widget.NewList(
		func() int {
			apiMu.Lock()
			n := 0
			for _, e := range apiLines {
				if e.level <= apiLevel {
					n++
				}
			}
			apiMu.Unlock()
			return n
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			apiMu.Lock()
			var e logEntry
			n := 0
			for _, ent := range apiLines {
				if ent.level <= apiLevel {
					n++
				}
			}
			// id 0 = newest (last filtered), id 1 = second newest, ...
			displayIdx := n - 1 - int(id)
			idx := 0
			for i := range apiLines {
				if apiLines[i].level <= apiLevel {
					if idx == displayIdx {
						e = apiLines[i]
						break
					}
					idx++
				}
			}
			apiMu.Unlock()
			lbl := o.(*widget.Label)
			lbl.SetText(levelColor(e.level) + " " + e.line)
		},
	)
	apiSelect := widget.NewSelect(levelNames, func(s string) {
		for i, n := range levelNames {
			if n == s {
				apiLevel = levelByIndex[i]
				break
			}
		}
		apiList.Refresh()
	})
	apiSelect.SetSelected("Trace")
	apiTop := container.NewHBox(widget.NewLabel("Level:"), apiSelect)
	apiContent := container.NewBorder(apiTop, nil, nil, nil, apiList)

	// Core tab: load from file
	loadCore := func() {
		lines, err := services.ReadLastLines(corePath, logViewerMaxLines)
		if err != nil {
			debuglog.WarnLog("logViewer: Core read failed: %v", err)
			fyne.Do(func() {
				coreLines = []string{"Log file not available."}
				if coreList != nil {
					coreList.Refresh()
				}
			})
			return
		}
		if lines == nil {
			fyne.Do(func() {
				coreLines = []string{"Log file not available."}
				if coreList != nil {
					coreList.Refresh()
				}
			})
			return
		}
		fyne.Do(func() {
			coreLines = lines
			if coreList != nil {
				coreList.Refresh()
			}
		})
	}
	coreRefreshBtn = widget.NewButton("Refresh", func() {
		loadCore()
	})
	coreList = widget.NewList(
		func() int {
			return len(coreLines)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			// id 0 = newest (last line from file), id 1 = second newest, ...
			idx := len(coreLines) - 1 - int(id)
			if idx >= 0 {
				line := coreLines[idx]
				lbl := o.(*widget.Label)
				lvl := parseCoreLevel(line)
				lbl.SetText(levelColor(lvl) + " " + line)
			}
		},
	)
	coreTop := container.NewHBox(coreRefreshBtn)
	coreContent := container.NewBorder(coreTop, nil, nil, nil, coreList)

	// Tabs
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Internal", theme.DocumentIcon(), internalContent),
		container.NewTabItemWithIcon("Core", theme.ViewRefreshIcon(), coreContent),
		container.NewTabItemWithIcon("API", theme.MailComposeIcon(), apiContent),
	)
	coreTabIndex = 1

	tabs.OnSelected = func(t *container.TabItem) {
		idx := tabs.SelectedIndex()
		if idx == coreTabIndex {
			if coreTickStop != nil {
				coreTickStop()
			}
			ticker := time.NewTicker(coreRefreshSec * time.Second)
			done := make(chan struct{})
			coreTickStop = func() {
				close(done)
				ticker.Stop()
				coreTickStop = nil
			}
			go func() {
				for {
					select {
					case <-done:
						return
					case <-ticker.C:
						loadCore()
					}
				}
			}()
			loadCore()
		} else {
			if coreTickStop != nil {
				coreTickStop()
				coreTickStop = nil
			}
		}
	}

	win.SetContent(tabs)

	// Sink callbacks: non-blocking send
	setSinks := func() {
		debuglog.SetInternalLogSink(func(level debuglog.Level, line string) {
			select {
			case internalCh <- logEntry{level, line}:
			default:
			}
		})
		api.SetAPILogSink(func(level debuglog.Level, line string) {
			select {
			case apiCh <- logEntry{level, line}:
			default:
			}
		})
	}
	clearSinks := func() {
		debuglog.ClearInternalLogSink()
		api.ClearAPILogSink()
	}

	// Goroutines that consume channels and update UI (store all; filter by level in list callbacks)
	go func() {
		for e := range internalCh {
			internalMu.Lock()
			internalLines = trimToMax(append(internalLines, e))
			internalMu.Unlock()
			fyne.Do(func() {
				internalList.Refresh()
			})
		}
	}()
	go func() {
		for e := range apiCh {
			apiMu.Lock()
			apiLines = trimToMax(append(apiLines, e))
			apiMu.Unlock()
			fyne.Do(func() {
				apiList.Refresh()
			})
		}
	}()

	setSinks()
	debuglog.DebugLog("logViewer: Logs window opened, sinks registered")

	win.SetOnClosed(func() {
		logViewerMu.Lock()
		if logViewerWindow == win {
			logViewerWindow = nil
		}
		logViewerMu.Unlock()
		if coreTickStop != nil {
			coreTickStop()
		}
		clearSinks()
		close(internalCh)
		close(apiCh)
	})

	logViewerMu.Lock()
	logViewerWindow = win
	logViewerMu.Unlock()
	win.Show()
}
