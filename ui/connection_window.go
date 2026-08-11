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
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
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

	win := ac.UIService.Application.NewWindow(locale.T("conn.window_title"))

	localContent := buildLocalEngineTab(ac, win, onChanged)
	remoteContent := buildRemoteDaemonPanel(ac, win, onChanged) // nil вне macOS

	var body fyne.CanvasObject
	if remoteContent == nil {
		// Не macOS: удалённый демон недоступен (lxd-клиент darwin-only),
		// переключать нечего.
		body = localContent
	} else {
		localBox := container.NewVBox(localContent)
		remoteBox := container.NewVBox(remoteContent)
		showScope := func(remote bool) {
			if remote {
				localBox.Hide()
				remoteBox.Show()
			} else {
				remoteBox.Hide()
				localBox.Show()
			}
		}
		scopeLocal := locale.T("conn.scope_local")
		scopeRemote := locale.T("conn.scope_remote")
		scope := widget.NewRadioGroup([]string{scopeLocal, scopeRemote}, func(selected string) {
			showScope(selected == scopeRemote)
		})
		scope.Horizontal = true
		scope.Required = true
		// Стартовое положение — где живёт текущее подключение: daemon-режим
		// с не-loopback адресом = удалённый демон.
		if connectionScopeIsRemote(ac) {
			scope.SetSelected(scopeRemote)
			showScope(true)
		} else {
			scope.SetSelected(scopeLocal)
			showScope(false)
		}
		body = container.NewVBox(scope, widget.NewSeparator(), localBox, remoteBox)
	}

	// Строго вертикальный скролл: по ширине контент ужимается под окно
	// (Label'ы с Wrapping), правая полоса — канонический gutter проекта
	// (components.NewScrollGutter, ширина ScrollbarGutterWidth).
	scrolled := container.NewVScroll(container.NewBorder(nil, nil, nil,
		components.NewScrollGutter(), container.NewPadded(body)))

	// Высота — как у главного окна на момент открытия (его фактический
	// canvas-размер), чтобы окна вставали рядом одинаковыми колонками.
	height := float32(640)
	if ac.UIService.MainWindow != nil {
		if h := ac.UIService.MainWindow.Canvas().Size().Height; h > 400 {
			height = h
		}
	}

	win.SetContent(scrolled)
	win.Resize(fyne.NewSize(560, height))
	win.CenterOnScreen()
	win.SetOnClosed(func() {
		connWindowMu.Lock()
		connWindowOpen = nil
		connWindowMu.Unlock()
	})

	connWindowMu.Lock()
	connWindowOpen = win
	connWindowMu.Unlock()
	win.Show()
}
