package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// updateBinaryStatus проверяет наличие бинарника и обновляет статус
func (tab *CoreDashboardTab) updateBinaryStatus() {
	// Проверяем, существует ли бинарник
	if _, err := tab.controller.GetInstalledCoreVersion(); err != nil {
		tab.statusLabel.SetText(locale.T("core.status_error_not_found"))
		tab.statusLabel.Importance = widget.MediumImportance // Текст всегда черный
		// UpdateUI will be called automatically by RunningState.Set() or other state changes
		// Don't call UpdateUI() here to avoid infinite loop
		return
	}
	// Если бинарник найден, обновляем статус запуска
	tab.updateRunningStatus()
	// UpdateUI will be called automatically by RunningState.Set() or other state changes
	// Don't call UpdateUI() here to avoid infinite loop
}

// updateRunningStatus обновляет статус Running/Stopped на основе RunningState
func (tab *CoreDashboardTab) updateRunningStatus() {
	// Get button state from centralized function (same logic as Tray Menu)
	buttonState := tab.controller.GetVPNButtonState()

	// Update status label based on state
	restartInfo := ""
	if tab.controller.ConsecutiveCrashAttempts > 0 {
		restartInfo = fmt.Sprintf(" [restart %d/%d]", tab.controller.ConsecutiveCrashAttempts, 3)
	}

	if !buttonState.BinaryExists {
		tab.statusLabel.SetText(locale.T("core.status_error_not_found") + restartInfo)
		tab.statusLabel.Importance = widget.MediumImportance // Текст всегда черный
	} else if buttonState.IsRunning {
		tab.statusLabel.SetText(locale.T("core.status_running") + restartInfo)
		tab.statusLabel.Importance = widget.MediumImportance // Текст всегда черный
	} else {
		tab.statusLabel.SetText(locale.T("core.status_stopped") + restartInfo)
		tab.statusLabel.Importance = widget.MediumImportance // Текст всегда черный
	}

	// Update buttons based on centralized state
	if tab.startButton != nil {
		if buttonState.StartEnabled {
			tab.startButton.Enable()
			tab.startButton.Importance = widget.HighImportance // Синяя кнопка, когда доступна
			tab.startButton.Refresh()
		} else {
			tab.startButton.Disable()
			tab.startButton.Importance = widget.MediumImportance // Обычная, когда недоступна
			tab.startButton.Refresh()
		}
	}
	if tab.stopButton != nil {
		if buttonState.StopEnabled {
			tab.stopButton.Enable()
			tab.stopButton.Importance = widget.HighImportance
			tab.stopButton.Refresh()
		} else {
			tab.stopButton.Disable()
			tab.stopButton.Importance = widget.MediumImportance
			tab.stopButton.Refresh()
		}
	}
	if tab.restartButton != nil {
		// Кнопка 🔄 — split-control «Rebuild …». Имеет смысл только когда
		// есть `state.json`, потому что **оба** пункта меню в семантике
		// SPEC 045 включают rebuild (state → config). Без state rebuild =
		// no-op, а «start без rebuild» — это уже отдельная кнопка Start
		// слева, дублировать не надо. Поэтому условие enable:
		//   binary есть AND state.json есть
		hasState := false
		if tab.controller != nil && tab.controller.FileService != nil {
			if _, err := os.Stat(platform.GetWizardStatePath(tab.controller.FileService.ExecDir)); err == nil {
				hasState = true
			}
		}
		if buttonState.BinaryExists && hasState {
			tab.restartButton.Enable()
		} else {
			tab.restartButton.Disable()
		}
		// Dirty marker: state edited → нужно перезапустить sing-box чтобы
		// применить. HighImportance (синий) даёт явный визуальный сигнал.
		//
		// Намеренно НЕ guard'им через IsRunning: если новый launcher не сам
		// запустил sing-box (например, sing-box крутится из другой установки
		// лаунчера), Enable/Disable кнопки управляется отдельно через
		// `buttonState.StopEnabled`. Цвет dirty-маркера должен ставиться
		// независимо — даже у disabled-кнопки видно что state ждёт рестарта.
		// Сбрасывается ProcessService.Start после RebuildConfigIfDirty
		// (см. core/rebuild.go).
		restartTooltip := fmt.Sprintf(locale.T("core.button_restart_tooltip"), platform.ShortcutModifierLabel())
		tab.restartButton.SetText("🔄")
		if tab.controller.StateService != nil && tab.controller.StateService.IsConfigStale() {
			tab.restartButton.Importance = widget.HighImportance
			tab.restartButton.SetToolTip(locale.T("core.restart_dirty_tooltip") + " — " + restartTooltip)
		} else {
			tab.restartButton.Importance = widget.MediumImportance
			tab.restartButton.SetToolTip(restartTooltip)
		}
		tab.restartButton.Refresh()
	}
}

func (tab *CoreDashboardTab) updateConfigInfo() {
	// Обновляем статусы sing-box и wintun.dll
	_ = tab.updateVersionInfo()
	if runtime.GOOS == "windows" {
		tab.updateWintunStatus()
	}

	// State selector — пере-сканить sources, новые "Save As" из визарда
	// должны появляться в dropdown'е без перезапуска.
	tab.refreshStateSelector()

	if tab.configStatusLabel == nil {
		return
	}
	configPath := tab.controller.FileService.ConfigPath
	configExists := false
	if info, err := os.Stat(configPath); err == nil {
		modTime := info.ModTime().Format("2006-01-02")
		// If we have a successful-update timestamp from this session, append a
		// relative "Xm ago / Xh ago" hint so users can see the subscription
		// freshness at a glance without digging for the pill.
		label := locale.Tf("core.status_config_ok", filepath.Base(configPath), modTime)
		if tab.controller.StateService != nil {
			tab.controller.StateService.LastUpdateMutex.RLock()
			succAt := tab.controller.StateService.LastUpdateSucceededAt
			tab.controller.StateService.LastUpdateMutex.RUnlock()
			if !succAt.IsZero() {
				label += "  " + formatRelativeAge(time.Since(succAt))
			}
		}
		tab.configStatusLabel.SetText(label)
		configExists = true
	} else if os.IsNotExist(err) {
		tab.configStatusLabel.SetText(locale.Tf("core.status_config_not_found", filepath.Base(configPath)))
		configExists = false
	} else {
		tab.configStatusLabel.SetText(locale.Tf("core.status_config_error", err))
		configExists = false
	}

	templateFileName := wizardtemplate.GetTemplateFileName()
	templatePath := filepath.Join(tab.controller.FileService.ExecDir, "bin", templateFileName)
	if _, err := os.Stat(templatePath); err != nil {
		// Template not found — show download button, hide configurator + update.
		if tab.templateDownloadButton != nil {
			tab.templateDownloadButton.Show()
			tab.templateDownloadButton.Enable()
			tab.templateDownloadButton.Importance = widget.HighImportance
		}
		if tab.wizardButton != nil {
			tab.wizardButton.Hide()
		}
		if tab.updateConfigButton != nil {
			tab.updateConfigButton.Disable()
		}
	} else {
		// Template found — show configurator, hide download button.
		if tab.templateDownloadButton != nil {
			tab.templateDownloadButton.Hide()
		}
		if tab.wizardButton != nil {
			tab.wizardButton.Show()
			// Configurator-кнопка синеет когда нет config.json (свежий
			// install, надо пройти конфигуратор и Save'нуть).
			if !configExists {
				tab.wizardButton.Importance = widget.HighImportance
			} else {
				tab.wizardButton.Importance = widget.MediumImportance
			}
			tab.wizardButton.Refresh()
		}
		// Update icon: enabled когда есть откуда читать parser_config
		// (state.json — canonical) и парсер сейчас не работает.
		// Синяя при IsCacheStale (state менялся → жми чтобы fetchнуть).
		if tab.updateConfigButton != nil {
			tab.controller.ParserMutex.Lock()
			parserRunning := tab.controller.ParserRunning
			tab.controller.ParserMutex.Unlock()
			hasState := false
			if tab.controller.FileService != nil {
				if _, err := os.Stat(platform.GetWizardStatePath(tab.controller.FileService.ExecDir)); err == nil {
					hasState = true
				}
			}
			if hasState && !parserRunning {
				tab.updateConfigButton.Enable()
			} else {
				tab.updateConfigButton.Disable()
			}
			if tab.controller.StateService != nil && tab.controller.StateService.IsCacheStale() {
				tab.updateConfigButton.Importance = widget.HighImportance
			} else {
				tab.updateConfigButton.Importance = widget.MediumImportance
			}
			tab.updateConfigButton.Refresh()
		}
	}

	// Обновляем статус кнопок Start/Stop, так как они зависят от наличия конфига
	tab.updateRunningStatus()
}

// updateVersionInfo обновляет информацию о версии sing-box и подпись кнопки
// Download/Reinstall по сравнению с pinned `constants.RequiredCoreVersion`
// (SPEC 046).
//
// `GetInstalledCoreVersion()` может долго выполняться (запуск
// `sing-box version` на медленной системе), поэтому вызов вынесен в
// горутину; UI обновляется через fyne.Do. Никаких сетевых походов отсюда
// не делается — версия pinned, не «свежайшая из GitHub».
func (tab *CoreDashboardTab) updateVersionInfo() error {
	go func() {
		installedVersion, err := tab.controller.GetInstalledCoreVersion()
		required := constants.RequiredCoreVersion
		fyne.Do(func() {
			tab.singboxStatusLabel.Importance = widget.MediumImportance
			switch {
			case err != nil:
				// Бинарника нет — синяя «Download vX.Y.Z», подталкиваем к
				// первичной установке.
				tab.downloadButton.Importance = widget.HighImportance
				tab.setSingboxState(
					locale.T("core.singbox_status_not_found"),
					locale.Tf("core.button_download_version", required),
					-1,
				)
			case installedVersion != required:
				// Стоит другая версия — нейтральная «Reinstall vX.Y.Z», без
				// подталкивания: пользователь мог поставить вручную.
				tab.downloadButton.Importance = widget.MediumImportance
				tab.setSingboxState(
					installedVersion,
					locale.Tf("core.button_reinstall_version", required),
					-1,
				)
			default:
				// Версия совпадает — кнопка скрыта.
				tab.setSingboxState(installedVersion, "", -1)
			}
		})
	}()
	return nil
}

// updateWintunStatus обновляет статус wintun.dll
func (tab *CoreDashboardTab) updateWintunStatus() {
	if runtime.GOOS != "windows" {
		return // wintun нужен только на Windows
	}

	exists, err := tab.controller.CheckWintunDLL()
	if err != nil {
		tab.wintunStatusLabel.Importance = widget.MediumImportance
		tab.setWintunState(locale.T("core.wintun_status_error"), "", -1)
		return
	}

	if exists {
		tab.wintunStatusLabel.Importance = widget.MediumImportance
		tab.setWintunState(locale.T("core.wintun_status_ok"), "", -1)
	} else {
		tab.wintunStatusLabel.Importance = widget.MediumImportance
		tab.wintunDownloadButton.Importance = widget.HighImportance
		tab.setWintunState(locale.T("core.wintun_status_not_found"), locale.T("core.button_download"), -1)
	}

	// Обновляем статус кнопок Start/Stop, так как они зависят от наличия wintun.dll
	tab.updateRunningStatus()
}
