// File connection_local.go — вкладка LOCAL окна подключения: движок
// локального ядра.
//
// Радио выбирает движок: Process (classic) или Daemon (lxd, только macOS).
// Радио — это намерение пользователя: выбор «Daemon» показывает панель
// демона даже когда режим ещё не включён (службу только предстоит установить
// и сопрячь — кнопки для этого как раз на панели). Фактический движок
// переключается, когда это возможно (VPN остановлен, сопряжение есть), и
// отражается в статусе панели.
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// buildLocalEngineTab собирает вкладку LOCAL. daemon-панель приходит из
// платформенного builder'а (nil вне macOS — тогда вкладка описывает только
// classic-режим без переключателя).
func buildLocalEngineTab(ac *core.AppController, win fyne.Window, onChanged func()) fyne.CanvasObject {
	processHint := widget.NewLabel(locale.T("conn.process_hint"))
	processHint.Wrapping = fyne.TextWrapWord

	var trySwitchToDaemon func()
	daemonPanel := buildDaemonPanel(ac, win, func() {
		// Успешное сопряжение: если пользователь уже выбрал daemon-движок,
		// доводим переключение до конца (при первом выборе оно могло
		// упасть на «not paired»).
		if trySwitchToDaemon != nil {
			trySwitchToDaemon()
		}
	})
	if daemonPanel == nil {
		// Не macOS: движок один, переключать нечего.
		return container.NewVBox(
			sectionHeader(locale.T("conn.engine_process")),
			processHint,
		)
	}

	processLabel := locale.T("conn.engine_process")
	daemonLabel := locale.T("conn.engine_daemon")

	processBox := container.NewVBox(processHint)

	var updatingRadio bool
	radio := widget.NewRadioGroup([]string{processLabel, daemonLabel}, nil)
	radio.Required = true

	persistMode := func(mode core.BackendMode) {
		binDir := platform.GetBinDir(ac.FileService.ExecDir)
		st := locale.LoadSettings(binDir)
		st.CoreBackendMode = string(mode)
		if err := locale.SaveSettings(binDir, st); err != nil {
			debuglog.WarnLog("conn.local: save core_backend_mode: %v", err)
		}
	}

	showPanels := func(daemonSelected bool) {
		if daemonSelected {
			processBox.Hide()
			daemonPanel.Show()
		} else {
			daemonPanel.Hide()
			processBox.Show()
		}
	}

	trySwitchToDaemon = func() {
		if ac.BackendMode() == core.BackendDaemon {
			return
		}
		if ac.RunningState.IsRunning() {
			return // переключимся после остановки VPN — статус панели объяснит
		}
		if err := ac.SwitchBackendMode(core.BackendDaemon); err != nil {
			// Не сопряжён/не установлен: панель с командами уже на экране,
			// статус подскажет следующий шаг. Режим остаётся classic.
			debuglog.InfoLog("conn.local: daemon engine not active yet: %v", err)
			return
		}
		persistMode(core.BackendDaemon)
		if onChanged != nil {
			onChanged()
		}
	}

	radio.OnChanged = func(selected string) {
		if updatingRadio {
			return
		}
		daemonSelected := selected == daemonLabel
		if !daemonSelected {
			// Process: переключение обязано пройти (гейт — работающий VPN).
			if ac.BackendMode() != core.BackendClassic {
				if ac.RunningState.IsRunning() {
					updatingRadio = true
					radio.SetSelected(daemonLabel)
					updatingRadio = false
					ShowErrorText(win, locale.T("conn.window_title"), locale.T("settings.daemon_stop_vpn_first"))
					return
				}
				if err := ac.SwitchBackendMode(core.BackendClassic); err != nil {
					updatingRadio = true
					radio.SetSelected(daemonLabel)
					updatingRadio = false
					ShowError(win, err)
					return
				}
				if onChanged != nil {
					onChanged()
				}
			}
			persistMode(core.BackendClassic)
			showPanels(false)
			return
		}
		// Daemon: показываем панель сразу (намерение), включаем режим когда
		// возможно. Ошибка «not paired» не откатывает выбор — команды
		// установки и сопряжения именно на этой панели.
		showPanels(true)
		if ac.RunningState.IsRunning() && ac.BackendMode() != core.BackendDaemon {
			ShowErrorText(win, locale.T("conn.window_title"), locale.T("settings.daemon_stop_vpn_first"))
			return
		}
		trySwitchToDaemon()
	}

	// Стартовое состояние — фактический движок.
	updatingRadio = true
	if ac.BackendMode() == core.BackendDaemon {
		radio.SetSelected(daemonLabel)
		showPanels(true)
	} else {
		radio.SetSelected(processLabel)
		showPanels(false)
	}
	updatingRadio = false

	return container.NewVBox(
		widget.NewLabelWithStyle(locale.T("conn.engine_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		radio,
		widget.NewSeparator(),
		processBox,
		daemonPanel,
	)
}
