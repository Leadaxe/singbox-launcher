package ui

import (
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
)

// Окно живого лога ядра удалённой машины (SPEC 161 §5.5, PLAN §6.7): стрим
// gRPC SubscribeLog по каналу подключённой машины. Раньше этот стрим читал
// только Debug API — из лаунчера лог машины был не виден вовсе.
//
// Свежие строки сверху, как в журнале обмена (machine_wire_log_window.go):
// окно читают «что происходит сейчас», и поле не нужно прокручивать вниз на
// каждой пачке строк.

const (
	// machineCoreLogMax — сколько строк держит окно (кольцо).
	machineCoreLogMax = 1000
	// machineCoreLogRedraw — период перерисовки: строки идут пачками, и
	// SetText на каждую строку загружал бы UI-поток впустую.
	machineCoreLogRedraw = 250 * time.Millisecond
)

var (
	machineCoreLogMu      sync.Mutex
	machineCoreLogWindows = map[string]fyne.Window{}
)

// OpenMachineCoreLogWindow открывает живой лог ядра машины. Только для
// подключённой машины: стрим идёт по её транспорту.
func OpenMachineCoreLogWindow(ac *core.AppController, d services.RemoteDaemon) {
	if ac == nil || ac.UIService == nil || ac.UIService.Application == nil {
		return
	}
	machineCoreLogMu.Lock()
	if win, ok := machineCoreLogWindows[d.ID]; ok {
		machineCoreLogMu.Unlock()
		win.Show()
		win.RequestFocus()
		return
	}
	machineCoreLogMu.Unlock()

	transport, ok := lxdOverrideTransportForID(d.ID)
	if !ok {
		ShowErrorText(ac.UIService.MainWindow, locale.T("Live log"), locale.Tf("Connect to %s first: the log streams over its channel.", d.Name))
		return
	}

	var (
		mu    sync.Mutex
		lines []string
		dirty bool
	)
	push := func(line string) {
		mu.Lock()
		lines = append(lines, line)
		if len(lines) > machineCoreLogMax {
			lines = lines[len(lines)-machineCoreLogMax:]
		}
		dirty = true
		mu.Unlock()
	}
	cancel, err := transport.SubscribeLogLines(func(l services.LogLine) {
		push(strings.TrimRight(l.Message, "\n"))
	}, func() {
		push(locale.T("— the core restarted —"))
	})
	if err != nil {
		debuglog.WarnLog("machine core log: subscribe %q: %v", d.ID, err)
		ShowError(ac.UIService.MainWindow, err)
		return
	}

	win := ac.UIService.Application.NewWindow(locale.Tf("%s — core log", d.Name))
	body := widget.NewMultiLineEntry()
	body.Wrapping = fyne.TextWrapWord
	body.SetPlaceHolder(locale.T("Waiting for log lines…"))

	render := func() string {
		mu.Lock()
		defer mu.Unlock()
		var b strings.Builder
		for i := len(lines) - 1; i >= 0; i-- {
			b.WriteString(lines[i])
			b.WriteByte('\n')
		}
		return b.String()
	}

	copyBtn := widget.NewButton(locale.T("Copy"), func() { setClipboard(render()) })
	closeBtn := widget.NewButton(locale.T("Close"), func() { win.Close() })
	closeBtn.Importance = widget.HighImportance
	win.SetContent(container.NewPadded(container.NewBorder(nil,
		container.NewBorder(nil, nil, copyBtn, closeBtn), nil, nil, body)))
	win.Resize(fyne.NewSize(820, 540))
	fynewidget.CenterOnScreen(win)

	stop := make(chan struct{})
	win.SetOnClosed(func() {
		close(stop)
		cancel()
		machineCoreLogMu.Lock()
		delete(machineCoreLogWindows, d.ID)
		machineCoreLogMu.Unlock()
	})
	go func() {
		t := time.NewTicker(machineCoreLogRedraw)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				mu.Lock()
				changed := dirty
				dirty = false
				mu.Unlock()
				if !changed {
					continue
				}
				text := render()
				fyne.Do(func() { body.SetText(text) })
			}
		}
	}()

	machineCoreLogMu.Lock()
	machineCoreLogWindows[d.ID] = win
	machineCoreLogMu.Unlock()
	win.Show()
}

// CloseMachineCoreLogWindow закрывает живой лог машины: канал к ней рвётся
// (Disconnect, Connect другой машины, re-pair) или её удалили. UI-поток.
func CloseMachineCoreLogWindow(id string) {
	machineCoreLogMu.Lock()
	win, ok := machineCoreLogWindows[id]
	machineCoreLogMu.Unlock()
	if ok {
		win.Close()
	}
}
