package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
)

// Журнал фоновых ошибок: то, что случилось без действия пользователя
// (перепроверка Clash API после сна, обрыв связи с машиной), не открывает
// диалог. После пробуждения из трея такие диалоги копились стопкой — по
// одному на каждое пробуждение. Здесь они ложатся в журнал, а смотрятся
// из Diagnostics → «Errors».
const errorJournalMax = 100

// Темы журнала: по ним строка статуса панели открывает журнал с фильтром.
const (
	errorTopicAll      = ""
	errorTopicClashAPI = "clash"
	errorTopicDaemon   = "daemon" // gRPC-транспорт и связь с машиной lxd
)

type errorJournalEntry struct {
	first, last time.Time
	topic       string
	source      string
	message     string
	count       int
}

var (
	errorJournalMu        sync.Mutex
	errorJournal          []errorJournalEntry
	errorJournalListeners []func()
	errorJournalWindow    fyne.Window
)

// RecordBackgroundError кладёт ошибку в журнал и во внутренний лог вместо
// диалога. Повтор той же ошибки подряд не плодит строки — растёт счётчик.
func RecordBackgroundError(topic, source string, err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	debuglog.ErrorLog("%s: %s", source, msg)
	now := time.Now()
	errorJournalMu.Lock()
	if n := len(errorJournal); n > 0 && errorJournal[n-1].topic == topic && errorJournal[n-1].source == source && errorJournal[n-1].message == msg {
		errorJournal[n-1].count++
		errorJournal[n-1].last = now
	} else {
		errorJournal = append(errorJournal, errorJournalEntry{first: now, last: now, topic: topic, source: source, message: msg, count: 1})
		if len(errorJournal) > errorJournalMax {
			errorJournal = errorJournal[len(errorJournal)-errorJournalMax:]
		}
	}
	listeners := append([]func(){}, errorJournalListeners...)
	errorJournalMu.Unlock()
	fyne.Do(func() {
		for _, fn := range listeners {
			fn()
		}
	})
}

func errorJournalSnapshot() []errorJournalEntry {
	errorJournalMu.Lock()
	defer errorJournalMu.Unlock()
	return append([]errorJournalEntry(nil), errorJournal...)
}

// errorJournalByTopic — записи темы (errorTopicAll — все), старые первыми.
func errorJournalByTopic(topic string) []errorJournalEntry {
	all := errorJournalSnapshot()
	if topic == errorTopicAll {
		return all
	}
	out := all[:0]
	for _, e := range all {
		if e.topic == topic {
			out = append(out, e)
		}
	}
	return out
}

// onErrorJournalChange подписывает UI на изменения журнала (вызов в UI-потоке).
func onErrorJournalChange(fn func()) {
	errorJournalMu.Lock()
	errorJournalListeners = append(errorJournalListeners, fn)
	errorJournalMu.Unlock()
}

func formatErrorJournalEntry(e errorJournalEntry) string {
	ts := e.last.Format("2006-01-02 15:04:05")
	if e.count > 1 {
		return fmt.Sprintf("%s ×%d (%s) · %s\n%s", ts, e.count, locale.Tf("since %s", e.first.Format("15:04:05")), e.source, e.message)
	}
	return fmt.Sprintf("%s · %s\n%s", ts, e.source, e.message)
}

// newErrorJournalButton — кнопка Diagnostics: «Errors (N)», открывает журнал.
func newErrorJournalButton(ac *core.AppController) *widget.Button {
	btn := widget.NewButtonWithIcon("", theme.ErrorIcon(), func() {
		openErrorJournalWindow(ac, errorTopicAll)
	})
	refresh := func() {
		n := len(errorJournalSnapshot())
		if n == 0 {
			btn.SetText(locale.T("Errors"))
			btn.Importance = widget.MediumImportance
		} else {
			btn.SetText(locale.Tf("Errors (%d)", n))
			btn.Importance = widget.DangerImportance
		}
		btn.Refresh()
	}
	refresh()
	onErrorJournalChange(refresh)
	return btn
}

// errorJournalSetTopic переключает фильтр уже открытого окна.
var errorJournalSetTopic func(topic string)

func openErrorJournalWindow(ac *core.AppController, topic string) {
	if errorJournalWindow != nil {
		if errorJournalSetTopic != nil {
			errorJournalSetTopic(topic)
		}
		errorJournalWindow.RequestFocus()
		return
	}
	win := ac.UIService.Application.NewWindow(locale.T("Errors"))
	win.Resize(fyne.NewSize(700, 450))

	// VBox в скролле, а не widget.List: у списка высота строки одна на
	// всех, и перенесённое длинное сообщение обрезалось бы.
	box := container.NewVBox()
	topics := []string{errorTopicAll, errorTopicClashAPI, errorTopicDaemon}
	topicNames := []string{locale.T("All"), locale.T("Clash API only"), locale.T("gRPC and daemon only")}
	current := topic
	reload := func() {
		entries := errorJournalByTopic(current)
		objs := make([]fyne.CanvasObject, 0, len(entries)*2)
		if len(entries) == 0 {
			objs = append(objs, widget.NewLabel(locale.T("No errors recorded.")))
		}
		// Новые сверху.
		for i := len(entries) - 1; i >= 0; i-- {
			l := widget.NewLabel(formatErrorJournalEntry(entries[i]))
			l.Wrapping = fyne.TextWrapWord
			l.Selectable = true
			objs = append(objs, l, widget.NewSeparator())
		}
		box.Objects = objs
		box.Refresh()
	}
	filter := widget.NewSelect(topicNames, func(name string) {
		for i, n := range topicNames {
			if n == name {
				current = topics[i]
			}
		}
		reload()
	})
	setTopic := func(t string) {
		for i, x := range topics {
			if x == t {
				filter.SetSelectedIndex(i) // вызывает reload
				return
			}
		}
	}
	setTopic(topic)
	onErrorJournalChange(func() {
		if errorJournalWindow == win {
			reload()
		}
	})

	copyBtn := widget.NewButtonWithIcon(locale.T("Copy all"), theme.ContentCopyIcon(), func() {
		snap := errorJournalByTopic(current)
		var b strings.Builder
		for i := len(snap) - 1; i >= 0; i-- {
			b.WriteString(formatErrorJournalEntry(snap[i]))
			b.WriteString("\n\n")
		}
		win.Clipboard().SetContent(b.String())
	})
	clearBtn := widget.NewButtonWithIcon(locale.T("Clear"), theme.DeleteIcon(), func() {
		errorJournalMu.Lock()
		errorJournal = nil
		listeners := append([]func(){}, errorJournalListeners...)
		errorJournalMu.Unlock()
		for _, fn := range listeners {
			fn()
		}
	})
	top := container.NewHBox(filter, copyBtn, clearBtn)
	win.SetContent(container.NewBorder(top, nil, nil, nil, container.NewVScroll(box)))
	win.SetOnClosed(func() {
		errorJournalWindow = nil
		errorJournalSetTopic = nil
	})
	errorJournalWindow = win
	errorJournalSetTopic = setTopic
	win.Show()
}

// errorStatusLine — строка под списком узлов: последняя фоновая ошибка темы
// панели. Видна только пока ошибка актуальна; клик открывает журнал с
// фильтром по этой теме.
type errorStatusLine struct {
	btn   *ttwidget.Button
	topic string
}

func newErrorStatusLine(ac *core.AppController) *errorStatusLine {
	l := &errorStatusLine{}
	l.btn = ttwidget.NewButton("", func() {
		openErrorJournalWindow(ac, l.topic)
	})
	l.btn.Alignment = widget.ButtonAlignLeading
	l.btn.Importance = widget.LowImportance
	l.btn.SetToolTip(locale.T("Show these errors"))
	l.btn.Hide()
	onErrorJournalChange(l.refresh)
	return l
}

// Show показывает строку по теме (вызов в UI-потоке).
func (l *errorStatusLine) Show(topic string) {
	l.topic = topic
	l.btn.Show()
	l.refresh()
}

// Hide прячет строку: проверка прошла или состояние сброшено.
func (l *errorStatusLine) Hide() {
	l.topic = errorTopicAll
	l.btn.Hide()
}

func (l *errorStatusLine) refresh() {
	if !l.btn.Visible() {
		return
	}
	entries := errorJournalByTopic(l.topic)
	if len(entries) == 0 {
		l.btn.Hide()
		return
	}
	e := entries[len(entries)-1]
	text := "❌ " + locale.Tf("%s: no connection", e.source)
	if e.count > 1 {
		text += fmt.Sprintf(" ×%d", e.count)
	}
	text += " · " + e.last.Format("15:04") + "  ›"
	l.btn.SetText(text)
}
