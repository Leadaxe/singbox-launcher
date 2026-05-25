package traffic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	tprof "singbox-launcher/internal/traffic"
)

// buildWindowToolbar renders the top-of-window toolbar:
//
//   [Verbose checkbox] [Banner if active] ............. [⋮ Overflow]
//
// The verbose checkbox toggles vars[log_level] between the user's saved
// value and "debug", invoking ConfigConfirmApply (which shows the
// "active connections will reset" warning dialog).
//
// The overflow menu provides:
//   - Copy current session JSON to clipboard
//   - Export current session JSON to a file
//   - Clear all completed sessions
//   - Help (opens SPEC excerpt in a dialog)
func buildWindowToolbar(deps WindowDeps, win fyne.Window) fyne.CanvasObject {
	// Use ttwidget.Check so the toggle can carry an explanatory tooltip.
	// Plain widget.Check has no tooltip support; users couldn't tell what
	// the checkbox does until they tried it (and got the «active
	// connections will reset» confirm).
	verboseChk := ttwidget.NewCheck("Verbose logs (debug)", nil)
	verboseChk.SetChecked(isCurrentlyVerbose(deps))
	verboseChk.SetToolTip(
		"Switches sing-box log level between your saved value (off) and " +
			"\"debug\" (on).\n\n" +
			"OFF — only warnings/errors + basic connection events in " +
			"sing-box.log. DNS-by-IP attribution may be incomplete; some " +
			"TCP/UDP events lack the originating domain.\n\n" +
			"ON — full DNS chain logged (CNAME → A → IP per query), " +
			"protocol/fragment details, and the profiler can attribute " +
			"every IP back to its originating domain. Use while diagnosing, " +
			"then turn off.\n\n" +
			"Cost: more CPU + faster log-file growth. Toggling triggers a " +
			"sing-box restart, so active connections reset (you'll see a " +
			"confirm dialog).",
	)

	verboseHint := widget.NewLabel("")
	verboseHint.Importance = widget.WarningImportance

	refreshHint := func() {
		if verboseChk.Checked {
			verboseHint.SetText("Verbose logs active — battery/CPU impact.")
		} else {
			verboseHint.SetText("")
		}
	}
	refreshHint()

	verboseChk.OnChanged = func(checked bool) {
		if deps.ConfigConfirmApply == nil || deps.ConfigReader == nil {
			// Toggle disabled without a writer — just bounce.
			verboseChk.SetChecked(!checked)
			return
		}
		target := "debug"
		if !checked {
			target = "warn" // template default per wizard_template.json
		}
		// Snapshot the *desired* state for the confirm path; if user
		// cancels, revert the checkbox.
		deps.ConfigConfirmApply(target, win, func() {
			fyne.Do(refreshHint)
		})
		// If confirm dialog cancels, the level didn't change. Re-derive
		// the checkbox from the actual state.
		fyne.Do(func() {
			actual := isCurrentlyVerbose(deps)
			if actual != verboseChk.Checked {
				verboseChk.SetChecked(actual)
			}
			refreshHint()
		})
	}

	overflow := widget.NewButtonWithIcon("", theme.MoreVerticalIcon(), nil)
	overflow.OnTapped = func() {
		menu := buildOverflowMenu(deps, win)
		pop := widget.NewPopUpMenu(menu, win.Canvas())
		// Anchor under the overflow button.
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(overflow)
		pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+overflow.MinSize().Height))
	}

	left := container.NewHBox(verboseChk, verboseHint)
	row := container.NewBorder(nil, nil, left, overflow, nil)
	return container.NewVBox(row, widget.NewSeparator())
}

func isCurrentlyVerbose(deps WindowDeps) bool {
	if deps.ConfigReader == nil {
		return false
	}
	level, ok := deps.ConfigReader()
	if !ok {
		return false
	}
	return level == "debug" || level == "trace"
}

func buildOverflowMenu(deps WindowDeps, win fyne.Window) *fyne.Menu {
	items := []*fyne.MenuItem{
		fyne.NewMenuItem("Copy session JSON", func() { copySessionJSON(deps, win) }),
		fyne.NewMenuItem("Export session JSON…", func() { exportSessionJSON(deps, win) }),
		fyne.NewMenuItem("Clear completed sessions", func() {
			dialog.ShowConfirm("Clear sessions?", "Delete all completed recording sessions? Active session is preserved.", func(yes bool) {
				if yes {
					deps.Profiler.ClearAll()
				}
			}, win)
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Help / about", func() { showHelpDialog(win) }),
	}
	return fyne.NewMenu("", items...)
}

// sessionExport is the JSON payload — small, no schema version (in-memory
// only feature per SPEC §"Final decisions" #5).
type sessionExport struct {
	Target     string                `json:"target_process"`
	StartedAt  time.Time             `json:"started_at"`
	FinishedAt *time.Time            `json:"finished_at,omitempty"`
	WasVerbose bool                  `json:"was_verbose"`
	Events     []tprof.TrafficEvent  `json:"events"`
}

func currentExport(deps WindowDeps) (*sessionExport, error) {
	s := deps.Profiler.ActiveSession()
	if s == nil {
		// No active — pick newest completed.
		comp := deps.Profiler.CompletedSessions()
		if len(comp) == 0 {
			return nil, fmt.Errorf("no session to export — start one first")
		}
		s = comp[len(comp)-1]
	}
	return &sessionExport{
		Target:     s.TargetProcess,
		StartedAt:  s.StartedAt,
		FinishedAt: s.FinishedAt,
		WasVerbose: s.WasVerbose,
		Events:     s.Events(),
	}, nil
}

func copySessionJSON(deps WindowDeps, win fyne.Window) {
	exp, err := currentExport(deps)
	if err != nil {
		dialog.ShowError(err, win)
		return
	}
	data, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		dialog.ShowError(err, win)
		return
	}
	if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
		app.Clipboard().SetContent(string(data))
	}
	dialog.ShowInformation("Copied", fmt.Sprintf("Session JSON copied (%d events).", len(exp.Events)), win)
}

func exportSessionJSON(deps WindowDeps, win fyne.Window) {
	exp, err := currentExport(deps)
	if err != nil {
		dialog.ShowError(err, win)
		return
	}
	data, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		dialog.ShowError(err, win)
		return
	}
	fd := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		if uc == nil {
			return
		}
		defer func() { _ = uc.Close() }()
		if _, werr := uc.Write(data); werr != nil {
			dialog.ShowError(werr, win)
			return
		}
	}, win)
	// Suggest a filename like "traffic-Slack-20260524T123415.json".
	target := shortPath(exp.Target)
	if target == "" {
		target = "session"
	}
	suggested := fmt.Sprintf("traffic-%s-%s.json", target, exp.StartedAt.Format("20060102T150405"))
	fd.SetFileName(suggested)
	// Default to user home — Fyne won't accept a string path, only a
	// URI; we shell out for the home dir.
	if home, err := os.UserHomeDir(); err == nil {
		if uri, lerr := storage.ListerForURI(storage.NewFileURI(filepath.Clean(home))); lerr == nil {
			fd.SetLocation(uri)
		}
	}
	fd.Show()
}

func showHelpDialog(win fyne.Window) {
	body := widget.NewLabel(
		"Traffic Profiler shows live DNS / TCP / UDP events from sing-box.\n" +
			"\n" +
			"Live tab — system-wide event stream (all processes).\n" +
			"Per-process tab — pick one process and record a session.\n" +
			"\n" +
			"Verbose logs (debug) gives full DNS visibility but resets active\n" +
			"connections when toggling. Use it when diagnosing a specific issue,\n" +
			"then turn it back off.\n" +
			"\n" +
			"Sessions are in-memory only — they wipe on app quit. Use Export to\n" +
			"save one to a file.",
	)
	body.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustom("Traffic Profiler", "Close", body, win)
	d.Resize(fyne.NewSize(440, 300))
	d.Show()
}
