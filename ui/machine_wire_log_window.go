package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/lxdclient"
)

// Окно журнала обмена с машиной: что лаунчер спросил и что ему ответили.
//
// Отвечает на вопрос, который до него было негде задать: «зелёная кнопка
// горит — а разговор-то идёт?». Раньше единственным признаком жизни канала
// был сам маркер, и когда он врал (состояние бралось из кеша последнего
// действия), проверить его было нечем, кроме общего лога лаунчера, где
// события всех машин лежат вперемешку со всем остальным.
//
// Отдельное окно, а не диалог: строк много, они длинные, и у высокого
// модального попапа в Fyne разъезжается размер ([[fyne-label-minwidth-trap]]).

// wireLogRefresh — период перерисовки открытого окна.
//
// Быстрее heartbeat незачем: новые события в журнале появляются в его ритме,
// плюс редкие рабочие вызовы. Секунда даёт ощущение живого потока и не
// перерисовывает окно впустую.
const wireLogRefresh = time.Second

// wireLogRows — сколько последних событий показываем.
//
// Журнал хранит больше, но окно про «что происходит сейчас»: длинная простыня
// заставляла бы прокручивать вниз к свежему на каждом обновлении.
const wireLogRows = 80

var (
	wireLogMu      sync.Mutex
	wireLogWindows = map[string]fyne.Window{}
)

// livenessSource — откуда окно берёт сводку состояния. Реализует панель машин.
type livenessSource interface {
	livenessOf(id string) machineLiveness
	healthOf(id string) (services.RemoteHealth, bool)
}

// OpenMachineWireLogWindow открывает журнал обмена с машиной.
//
// Повторный вызов не плодит окна: показывает уже открытое. Иначе клик по
// маркеру при каждой перерисовке строки оставлял бы стопку одинаковых окон.
func OpenMachineWireLogWindow(ac *core.AppController, d services.RemoteDaemon, src livenessSource) {
	wireLogMu.Lock()
	if win, ok := wireLogWindows[d.ID]; ok {
		wireLogMu.Unlock()
		win.Show()
		win.RequestFocus()
		return
	}
	wireLogMu.Unlock()

	win := ac.UIService.Application.NewWindow(locale.Tf("%s — exchange log", d.Name))

	summary := widget.NewLabel("")
	summary.Wrapping = fyne.TextWrapWord

	// Entry, а не Label: строки нужно уметь выделить и скопировать в тикет.
	// Правки в нём никуда не уходят — текст перезаписывается на каждом
	// обновлении, как в соседнем окне статуса демона.
	body := widget.NewMultiLineEntry()
	body.Wrapping = fyne.TextWrapOff

	render := func() (string, string) {
		events := lxdclient.WireLog(d.ID)
		if len(events) > wireLogRows {
			events = events[len(events)-wireLogRows:]
		}
		var b strings.Builder
		for i := len(events) - 1; i >= 0; i-- { // свежие сверху
			e := events[i]
			mark := "ok"
			if !e.OK() {
				mark = "FAIL"
			}
			fmt.Fprintf(&b, "%s  %-6s %-4s %-28s %6s",
				e.When.Format("15:04:05"), e.Kind, mark, e.Op, e.Took.Round(time.Millisecond))
			if e.Status != 0 {
				fmt.Fprintf(&b, "  %d", e.Status)
			}
			if e.Err != "" {
				fmt.Fprintf(&b, "  %s", strings.ReplaceAll(e.Err, "\n", " "))
			}
			b.WriteByte('\n')
		}
		if b.Len() == 0 {
			// Пустой журнал — не ошибка и не «связи нет»: до Connect лаунчер
			// с машиной не разговаривает вовсе. Говорим это прямо, иначе
			// пустое поле читается как поломка окна.
			b.WriteString(locale.T("No exchange with this machine yet — press Connect."))
		}
		return wireLogSummary(d, src), b.String()
	}

	head, text := render()
	summary.SetText(head)
	body.SetText(text)

	copyBtn := widget.NewButton(locale.T("Copy"), func() {
		h, t := render()
		setClipboard(h + "\n\n" + t)
	})
	closeBtn := widget.NewButton(locale.T("Close"), func() { win.Close() })
	closeBtn.Importance = widget.HighImportance

	win.SetContent(container.NewPadded(container.NewBorder(
		container.NewVBox(summary, widget.NewSeparator()),
		container.NewBorder(nil, nil, copyBtn, closeBtn),
		nil, nil,
		body,
	)))
	win.Resize(fyne.NewSize(760, 520))
	win.CenterOnScreen()

	stop := make(chan struct{})
	win.SetOnClosed(func() {
		close(stop)
		wireLogMu.Lock()
		delete(wireLogWindows, d.ID)
		wireLogMu.Unlock()
	})
	go func() {
		t := time.NewTicker(wireLogRefresh)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				h, txt := render()
				fyne.Do(func() {
					summary.SetText(h)
					// CursorRow/выделение сбрасывать не жалко: окно читают
					// сверху, куда и приходят свежие строки.
					body.SetText(txt)
				})
			}
		}
	}()

	wireLogMu.Lock()
	wireLogWindows[d.ID] = win
	wireLogMu.Unlock()
	win.Show()
}

// wireLogSummary — шапка окна: состояние машины словами.
//
// Без неё журнал отвечает только на «что происходило», но не на «и что это
// значит»: череда FAIL под зелёным маркером выглядела бы противоречием.
func wireLogSummary(d services.RemoteDaemon, src livenessSource) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s   %s", d.Name, d.Addr)
	if src == nil {
		return b.String()
	}
	health, connected := src.healthOf(d.ID)
	live := src.livenessOf(d.ID)
	switch markerFor(connected, health, live) {
	case markerLive:
		b.WriteString(locale.T("\nAnswering."))
	case markerFlaky:
		b.WriteString(locale.Tf("\nNo answer to the last %d poll(s) — retrying.", live.FailStreak))
	case markerDown:
		b.WriteString(locale.T("\nNot answering."))
	default:
		b.WriteString(locale.T("\nNot connected."))
	}
	if !live.LastOK.IsZero() {
		b.WriteString(locale.Tf("  Last answer at %s.", live.LastOK.Format("15:04:05")))
	}
	if live.LastErr != "" {
		fmt.Fprintf(&b, "\n%s", strings.ReplaceAll(live.LastErr, "\n", " "))
	}
	if connected && health.CoreStatus != "" {
		fmt.Fprintf(&b, "\n%s: %s", locale.T("Core"), health.CoreStatus)
		if health.LastError != "" {
			fmt.Fprintf(&b, "  (%s)", health.LastError)
		}
	}
	return b.String()
}

// CloseMachineWireLogWindow закрывает журнал машины (её удалили из реестра).
func CloseMachineWireLogWindow(id string) {
	wireLogMu.Lock()
	win, ok := wireLogWindows[id]
	wireLogMu.Unlock()
	if ok {
		fyne.Do(win.Close)
	}
}
