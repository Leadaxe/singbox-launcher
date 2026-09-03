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
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/internal/locale"
	tprof "singbox-launcher/internal/traffic"
)

// buildWindowToolbar renders the top-of-window toolbar:
//
//	[Verbose checkbox] [Banner if active] ............. [⋮ Overflow]
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
	verboseChk := ttwidget.NewCheck(locale.T("Verbose logs (debug)"), nil)
	verboseChk.SetChecked(isCurrentlyVerbose(deps))
	verboseChk.SetToolTip(
		"Switches sing-box log level between your saved value (off) and " +
			"\"debug\" (on).\n\n" +
			"In this mode the DNS plane is read from sing-box.log, and only " +
			"\"debug\" makes the core write it.\n\n" +
			"OFF — connections and their hosts are still shown (they come " +
			"from the connections snapshot), but the DNS tab stays empty: no " +
			"queries, no CNAME chains, no failures.\n\n" +
			"ON - DNS queries appear as events, with the CNAME -> A/AAAA chain " +
			"per query. Use while diagnosing, then turn off.\n\n" +
			"Cost: more CPU + faster log-file growth. Toggling triggers a " +
			"sing-box restart, so active connections reset (you'll see a " +
			"confirm dialog).",
	)

	verboseHint := widget.NewLabel("")
	verboseHint.Importance = widget.WarningImportance

	refreshHint := func() {
		if verboseChk.Checked {
			verboseHint.SetText(locale.T("Verbose logs active — battery/CPU impact."))
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

	// В окне машины тулбара нет вовсе: галка правит уровень лога ЛОКАЛЬНОГО
	// конфига и перезапускает СВОЁ ядро — к машине это отношения не имеет, её
	// конфиг лежит у неё. Без writer'а она к тому же просто отскакивала бы
	// назад, изображая работу. Одно меню ⋮ не стоит целой пустой строки —
	// оно переезжает в ряд вкладок, к счётчику соединений.
	if deps.RemoteMachine {
		return nil
	}
	// DNS приходит структурным стримом (daemon-режим, SubscribeDNSQueries) —
	// уровень лога к DNS-плоскости больше не имеет отношения: гейт эмита в
	// ядре это наличие подписчика, а не log.level. Галка тогда предлагала бы
	// рестарт ядра ради данных, которые уже есть, причём в лог они приходят
	// беднее — без процесса, rcode и признака «ответ из кэша».
	//
	// Без неё в тулбаре не остаётся ничего: ⋮ живёт в полосе вкладок. Пустая
	// строка с разделителем съедала бы ряд высоты впустую.
	if deps.Profiler != nil && deps.Profiler.DNSFromStream() {
		return nil
	}
	row := container.NewHBox(verboseChk, verboseHint)
	return container.NewVBox(row, widget.NewSeparator())
}

// buildOverflowButton — меню ⋮ (экспорт сессии, справка).
//
// Отдельно от тулбара: в окне машины тулбара нет, и кнопка живёт в ряду
// вкладок.
func buildOverflowButton(deps WindowDeps, win fyne.Window, live *liveView) *widget.Button {
	overflow := widget.NewButtonWithIcon("", theme.MoreVerticalIcon(), nil)
	overflow.Importance = widget.LowImportance
	overflow.OnTapped = func() {
		menu := buildOverflowMenu(deps, win, live)
		pop := widget.NewPopUpMenu(menu, win.Canvas())
		// Anchor under the overflow button.
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(overflow)
		pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+overflow.MinSize().Height))
	}
	return overflow
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

func buildOverflowMenu(deps WindowDeps, win fyne.Window, live *liveView) *fyne.Menu {
	items := []*fyne.MenuItem{}
	// Буфер вкладки Live — то, на что пользователь смотрит прямо сейчас.
	// Раньше в меню были только пункты записи, и на полном экране событий
	// экспорт отвечал «no session to export»: сессия — это запись по
	// процессу со вкладки By client, к живому потоку отношения не имеющая.
	if live != nil {
		items = append(items,
			fyne.NewMenuItem("Copy live buffer JSON", func() { copyLiveJSON(live, win) }),
			fyne.NewMenuItem("Export live buffer JSON…", func() { exportLiveJSON(live, win) }),
			fyne.NewMenuItemSeparator(),
		)
	}
	items = append(items,
		fyne.NewMenuItem("Copy session JSON", func() { copySessionJSON(deps, win) }),
		fyne.NewMenuItem("Export session JSON…", func() { exportSessionJSON(deps, win) }),
		fyne.NewMenuItem("Clear completed sessions", func() {
			dialog.ShowConfirm("Clear sessions?", locale.T("Delete all completed recording sessions? Active session is preserved."), func(yes bool) {
				if yes {
					deps.Profiler.ClearAll()
				}
			}, win)
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Help / about", func() { showHelpDialog(deps, win) }),
	)
	return fyne.NewMenu("", items...)
}

// sessionExport is the JSON payload — small, no schema version (in-memory
// only feature per SPEC §"Final decisions" #5).
type sessionExport struct {
	Target     string               `json:"target_process"`
	StartedAt  time.Time            `json:"started_at"`
	FinishedAt *time.Time           `json:"finished_at,omitempty"`
	WasVerbose bool                 `json:"was_verbose"`
	Events     []tprof.TrafficEvent `json:"events"`
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
	dialog.ShowInformation(locale.T("Copied"), fmt.Sprintf("Session JSON copied (%d events).", len(exp.Events)), win)
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

// showHelpDialog. Абзац про DNS зависит от источника: где он структурный,
// галки в окне нет вовсе, и советовать её значило бы отправлять пользователя
// искать несуществующий переключатель.
func showHelpDialog(deps WindowDeps, win fyne.Window) {
	dnsPara := "Verbose logs (debug) fills the DNS tab: without it you still\n" +
		"see connections and their hosts, but no DNS queries, CNAME chains\n" +
		"or failures. Toggling restarts sing-box, so active connections\n" +
		"reset — use it while diagnosing, then turn it back off.\n"
	if deps.Profiler != nil && deps.Profiler.DNSFromStream() {
		// Структурный поток (SubscribeDNSQueries): DNS приходит независимо от
		// уровня лога, и в событии есть то, чего в логе нет вовсе.
		dnsPara = "DNS events arrive as a structured stream from the core, so\n" +
			"they don't depend on the log level and cost no restart. Each query\n" +
			"carries its CNAME chain, the server that answered, the process and\n" +
			"the failure reason.\n"
	}
	body := widget.NewLabel(
		"Traffic Profiler shows live DNS / TCP / UDP events from sing-box.\n" +
			"\n" +
			"Live tab — system-wide event stream (all processes).\n" +
			"Per-process tab — pick one process and record a session.\n" +
			"\n" +
			dnsPara +
			"\n" +
			"Sessions are in-memory only — they wipe on app quit. Use Export to\n" +
			"save one to a file.",
	)
	body.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustom("Traffic Profiler", "Close", body, win)
	d.Resize(fyne.NewSize(440, 360))
	d.Show()
}

// liveExport — полезная нагрузка выгрузки живого буфера. Отдельный тип от
// sessionExport: у буфера нет ни цели, ни границ записи, зато есть признак
// «выгружено с фильтром» — без него отчёт с 30 строками из 5000 выглядел бы
// потерей данных.
type liveExport struct {
	Kind       string               `json:"kind"`
	CapturedAt time.Time            `json:"captured_at"`
	Filtered   bool                 `json:"filtered"`
	Events     []tprof.TrafficEvent `json:"events"`
}

func marshalLive(live *liveView, onlyFiltered bool) ([]byte, int, error) {
	ev := live.ExportSnapshot(onlyFiltered)
	data, err := json.MarshalIndent(liveExport{
		Kind:       "live-buffer",
		CapturedAt: time.Now(),
		Filtered:   onlyFiltered,
		Events:     ev,
	}, "", "  ")
	return data, len(ev), err
}

// askLiveScope спрашивает про охват только когда фильтр что-то прячет:
// при чистом фильтре «видимое» и «весь буфер» — одно и то же, и вопрос был
// бы лишним кликом на пути к файлу.
//
// Не ShowCustomConfirm: у confirm-диалога Esc неотличим от dismiss-кнопки,
// и «Whole buffer» на её месте превращал бы отмену в молчаливую выгрузку
// всего буфера. Свои три кнопки, закрытие = отмена.
func askLiveScope(live *liveView, win fyne.Window, done func(onlyFiltered bool)) {
	if !live.FilterActive() {
		done(false)
		return
	}
	var d *dialog.CustomDialog
	pick := func(onlyFiltered bool) func() {
		return func() {
			d.Hide()
			done(onlyFiltered)
		}
	}
	visibleBtn := widget.NewButton(locale.T("Visible rows only"), pick(true))
	visibleBtn.Importance = widget.HighImportance
	wholeBtn := widget.NewButton(locale.T("Whole buffer"), pick(false))
	cancelBtn := widget.NewButton(locale.T("Cancel"), func() { d.Hide() })
	content := container.NewVBox(
		widget.NewLabel(locale.T("A filter is active. Export what's on screen, or every event in the buffer?")),
		container.NewHBox(layout.NewSpacer(), cancelBtn, wholeBtn, visibleBtn),
	)
	d = dialog.NewCustomWithoutButtons(locale.T("Export live buffer"), content, win)
	d.Show()
}

func copyLiveJSON(live *liveView, win fyne.Window) {
	askLiveScope(live, win, func(onlyFiltered bool) {
		data, n, err := marshalLive(live, onlyFiltered)
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
			app.Clipboard().SetContent(string(data))
		}
		dialog.ShowInformation(locale.T("Copied"), fmt.Sprintf("Live buffer JSON copied (%d events).", n), win)
	})
}

func exportLiveJSON(live *liveView, win fyne.Window) {
	askLiveScope(live, win, func(onlyFiltered bool) {
		data, _, err := marshalLive(live, onlyFiltered)
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
			}
		}, win)
		fd.SetFileName(fmt.Sprintf("traffic-live-%s.json", time.Now().Format("20060102T150405")))
		if home, err := os.UserHomeDir(); err == nil {
			if uri, lerr := storage.ListerForURI(storage.NewFileURI(filepath.Clean(home))); lerr == nil {
				fd.SetLocation(uri)
			}
		}
		fd.Show()
	})
}
