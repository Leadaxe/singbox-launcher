// File pool_window.go — окно пула балансировщика (daemon-режим).
//
// Показывает слоты пула urltest-группы демона: номер слота, тег узла и его
// задержку — lx-наблюдаемость (RPC GetPool), доступная только по gRPC-каналу
// daemon-режима. Обновляется автоматически, пока окно открыто.
package ui

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
)

const poolWindowRefreshInterval = 3 * time.Second

// poolWindowOpen — синглтон открытого окна (как SettingsWindow).
var (
	poolWindowMu   sync.Mutex
	poolWindowOpen fyne.Window
)

// OpenPoolWindow открывает (или фокусирует) окно пула балансировщика.
func OpenPoolWindow(ac *core.AppController) {
	poolWindowMu.Lock()
	if poolWindowOpen != nil {
		w := poolWindowOpen
		poolWindowMu.Unlock()
		w.RequestFocus()
		return
	}
	poolWindowMu.Unlock()

	if !ac.DaemonPoolAvailable() {
		ShowErrorText(ac.UIService.MainWindow, locale.T("pool.window_title"), locale.T("pool.daemon_only"))
		return
	}

	w := ac.UIService.Application.NewWindow(locale.T("pool.window_title"))

	var (
		slotsMu sync.Mutex
		slots   []core.PoolSlotInfo
	)
	status := widget.NewLabel(locale.T("pool.loading"))
	status.Wrapping = fyne.TextWrapWord

	slotList := widget.NewList(
		func() int {
			slotsMu.Lock()
			defer slotsMu.Unlock()
			return len(slots)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("slot")
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			slotsMu.Lock()
			defer slotsMu.Unlock()
			if int(id) >= len(slots) {
				return
			}
			s := slots[id]
			delay := "—"
			if s.Delay > 0 {
				delay = fmt.Sprintf("%d ms", s.Delay)
			}
			o.(*widget.Label).SetText(fmt.Sprintf("#%d  %s  ·  %s", s.Slot, s.Tag, delay))
		},
	)

	groupSelect := widget.NewSelect(nil, nil)
	groupSelect.PlaceHolder = locale.T("pool.group_placeholder")

	// selectedGroup — потокобезопасная копия выбранной группы. Тикер-горутина
	// не имеет права читать groupSelect.Selected (виджет-поле, доступ только
	// из UI-потока — иначе гонка с SetSelected).
	var groupMu sync.Mutex
	selectedGroup := ""

	refreshSlots := func() {
		groupMu.Lock()
		group := selectedGroup
		groupMu.Unlock()
		if group == "" {
			return
		}
		go func() {
			got, err := ac.DaemonPoolSlots(group)
			fyne.Do(func() {
				if err != nil {
					status.SetText(locale.Tf("pool.error", err))
					return
				}
				slotsMu.Lock()
				slots = got
				slotsMu.Unlock()
				if len(got) == 0 {
					status.SetText(locale.T("pool.empty"))
				} else {
					status.SetText(locale.Tf("pool.slots_count", len(got)))
				}
				slotList.Refresh()
			})
		}()
	}
	groupSelect.OnChanged = func(g string) {
		groupMu.Lock()
		selectedGroup = g
		groupMu.Unlock()
		refreshSlots()
	}

	loadGroups := func() {
		go func() {
			groups, err := ac.DaemonPoolGroups()
			fyne.Do(func() {
				if err != nil {
					status.SetText(locale.Tf("pool.error", err))
					return
				}
				if len(groups) == 0 {
					status.SetText(locale.T("pool.no_groups"))
					return
				}
				groupSelect.Options = groups
				groupSelect.Refresh()
				if groupSelect.Selected == "" {
					groupSelect.SetSelected(groups[0]) // → refreshSlots
				} else {
					refreshSlots()
				}
			})
		}()
	}

	refreshBtn := widget.NewButton(locale.T("pool.refresh"), func() {
		loadGroups()
	})

	// Авто-обновление, пока окно открыто.
	stopTicker := make(chan struct{})
	go func() {
		ticker := time.NewTicker(poolWindowRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopTicker:
				return
			case <-ticker.C:
				refreshSlots()
			}
		}
	}()

	w.SetOnClosed(func() {
		close(stopTicker)
		poolWindowMu.Lock()
		poolWindowOpen = nil
		poolWindowMu.Unlock()
		debuglog.DebugLog("pool window closed")
	})

	top := container.NewBorder(nil, nil, widget.NewLabel(locale.T("pool.group_label")), refreshBtn, groupSelect)
	w.SetContent(container.NewBorder(container.NewVBox(top, status), nil, nil, nil, slotList))
	w.Resize(fyne.NewSize(420, 480))

	poolWindowMu.Lock()
	poolWindowOpen = w
	poolWindowMu.Unlock()

	loadGroups()
	w.Show()
}
