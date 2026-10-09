// File connection_window.go — окно «Настройки подключения» (⚙ на Servers).
//
// Один горизонтальный переключатель области вместо вкладок:
//   - LOCAL — ядро на этой машине: движок Process (classic) / Daemon (lxd),
//     установка службы, обслуживание, сопряжение, удаление.
//   - REMOTE — демон на другой машине: ТОЛЬКО подключение (приглашение,
//     адрес, секрет) — установку и удаление службы делает оператор того
//     хоста, здесь этим кнопкам не место.
//
// Окно отдельное (Application.NewWindow), НЕ модальный попап: форма высокая,
// а Fyne раздувает высокий попап на весь экран (см. память проекта).
package ui

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"singbox-launcher/core"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
)

var (
	connWindowMu   sync.Mutex
	connWindowOpen fyne.Window
)

// OpenConnectionWindow открывает (или фокусирует) окно настроек подключения.
// onChanged — колбэк после смены движка/сопряжения (рефреш Servers).
func OpenConnectionWindow(ac *core.AppController, onChanged func()) {
	connWindowMu.Lock()
	if connWindowOpen != nil {
		w := connWindowOpen
		connWindowMu.Unlock()
		w.RequestFocus()
		return
	}
	connWindowMu.Unlock()

	win := ac.UIService.Application.NewWindow(locale.T("Connection settings"))

	// SPEC 098: окно осталось ТОЛЬКО про локальное ядро — движок
	// (classic/daemon) и сопряжение со своим демоном.
	//
	// Переключатель Local/Remote и список чужих машин отсюда убраны: ими
	// целиком заведует правая колонка вкладки Remote, где у каждой машины своя
	// строка с Connect, Configure и Deploy. Две точки управления одними и теми
	// же машинами — это два места, где можно ошибиться, и ровно оттуда росла
	// путаница «сопрягся с роутером, а затёрся адрес своего демона».
	body, dispose := buildLocalEngineTab(ac, win, onChanged)

	// Прокрутки нет на уровне окна: панель демона — окно Service, у каждой
	// его вкладки своя вертикальная прокрутка (SPEC 161 §4.2). Слой тултипов
	// — для подсказок ▶ и шагов окна Service.
	content := fynetooltip.AddWindowToolTipLayer(container.NewPadded(body), win.Canvas())

	// Высота — как у главного окна на момент открытия (его фактический
	// canvas-размер), чтобы окна вставали рядом одинаковыми колонками.
	height := float32(640)
	if ac.UIService.MainWindow != nil {
		if h := ac.UIService.MainWindow.Canvas().Size().Height; h > 400 {
			height = h
		}
	}

	win.SetContent(content)
	// Ширина — как у окна Service машины: шапка и команды те же.
	win.Resize(fyne.NewSize(serviceWindowWidth, height))
	fynewidget.CenterOnScreen(win)
	win.SetOnClosed(func() {
		if dispose != nil {
			dispose()
		}
		fynetooltip.DestroyWindowToolTipLayer(win.Canvas())
		connWindowMu.Lock()
		connWindowOpen = nil
		connWindowMu.Unlock()
	})

	connWindowMu.Lock()
	connWindowOpen = win
	connWindowMu.Unlock()
	win.Show()
}
