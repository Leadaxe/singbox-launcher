// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_save.go реализует Save-pipeline в state-only форме (SPEC 045 фаза 5.B).
//
// SaveConfig выполняет:
//  1. Validate input (ParserConfig непуст, DNS-валидация).
//  2. SyncGUIToModel — слить UI-виджеты в WizardModel.
//  3. SaveCurrentState → state.json (атомарная запись через core/state.Save).
//  4. Поднять dirty-маркеры в StateService:
//     - CacheStale (источники изменились → жми Update)
//     - ConfigStale (шаблон изменился → жми Restart)
//  5. Опубликовать events.StateChanged через EventBus.
//  6. Показать success-диалог.
//
// Save **больше НЕ пишет config.json**. Реальная пересборка config'а
// делается отдельно:
//   - кнопка Update / auto-update → core/build.BuildConfig (фаза 5.A — реализовано)
//   - кнопка Restart / Run после Save → core/build.BuildConfig (фаза 5.C — TODO)
//
// Это ключевое архитектурное разделение SPEC 045: state — декларативное
// «что хочет пользователь», config — производное «что читает sing-box».
// Save мутирует только первое; build/restart — единственные writer'ы второго.
//
// Используется в:
//   - wizard.go — SaveConfig вызывается при нажатии «Save» в визарде.
package presentation

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/config"
	"singbox-launcher/core/events"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SaveConfig сохраняет конфигурацию асинхронно с прогресс-баром.
func (p *WizardPresenter) SaveConfig() {
	p.SyncGUIToModel()

	// Validate input before starting save operation
	if !p.validateSaveInput() {
		return
	}

	// Check if save operation is already in progress
	if !p.checkSaveOperationState() {
		return
	}

	debuglog.InfoLog("SaveConfig: starting save operation")

	// Устанавливаем флаг синхронно ДО запуска горутины, чтобы избежать race condition
	p.guiState.SaveInProgress = true
	p.SetSaveState("", 0.0)

	go p.executeSaveOperation()
}

// validateSaveInput проверяет входные данные перед сохранением.
// Only ParserConfig.ParserConfig.Proxies is the source of truth; at least one proxy must have Source or Connections.
func (p *WizardPresenter) validateSaveInput() bool {
	if strings.TrimSpace(p.model.ParserConfigJSON) == "" {
		debuglog.WarnLog("SaveConfig: ParserConfig is empty")
		dialog.ShowError(errors.New(locale.T("wizard.save.error_config_empty")), p.guiState.Window)
		return false
	}
	if err := wizardbusiness.ValidateDNSModel(p.model); err != nil {
		debuglog.WarnLog("SaveConfig: DNS validation failed: %v", err)
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.dns.error_validation"), err), p.guiState.Window)
		return false
	}
	var pc config.ParserConfig
	if err := json.Unmarshal([]byte(p.model.ParserConfigJSON), &pc); err != nil {
		debuglog.WarnLog("SaveConfig: ParserConfig JSON invalid: %v", err)
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.save.error_config_invalid"), err), p.guiState.Window)
		return false
	}
	for _, px := range pc.ParserConfig.Proxies {
		if strings.TrimSpace(px.Source) != "" || len(px.Connections) > 0 {
			return true
		}
	}
	debuglog.WarnLog("SaveConfig: no proxy with source or connections in ParserConfig")
	dialog.ShowError(errors.New(locale.T("wizard.save.error_no_sources")), p.guiState.Window)
	return false
}

// checkSaveOperationState проверяет состояние операции сохранения.
func (p *WizardPresenter) checkSaveOperationState() bool {
	if p.guiState.SaveInProgress {
		debuglog.WarnLog("SaveConfig: Save operation already in progress")
		dialog.ShowInformation(locale.T("wizard.save.dialog_saving"), locale.T("wizard.save.dialog_in_progress"), p.guiState.Window)
		return false
	}
	return true
}

// executeSaveOperation выполняет операцию сохранения в отдельной горутине.
//
// SPEC 045 фаза 5.2 — Save → state-only:
// больше НЕ пишет config.json. Старая последовательность
// (ensureOutboundsParsed → buildConfigForSave → saveConfigFile) удалена.
// Save теперь:
//  1. сохраняет state.json (декларативное состояние пользователя);
//  2. поднимает dirty-флаги (CacheStale / ConfigStale) в StateService —
//     UI рисует `*` маркеры, пользователь явно жмёт Update / Restart;
//  3. публикует StateChanged event для подписчиков;
//  4. показывает диалог успеха.
//
// Восстановление работающего sing-box config'а делается отдельным шагом:
// `Update` пересобирает config через `core/build.BuildConfig` (фаза 5.1),
// `Restart` пересобирает + kill+start процесса (фаза 5.3).
func (p *WizardPresenter) executeSaveOperation() {
	defer p.finalizeSaveOperation()

	// Save no longer needs outbounds parsing — state.json is purely declarative.
	// Старый `ensureOutboundsParsed` (60-сек poll!) удалён.

	p.UpdateSaveStatusText(locale.T("wizard.save.status_saving_state"))
	p.UpdateSaveProgress(0.5)

	// Step 1: persist state.json.
	statePath := p.saveStateOnly()
	if statePath == "" {
		// SaveCurrentState уже отлогировал ошибку. Прекращаем; finalize вернёт UI.
		return
	}

	// Step 2: signal UI / dirty markers / event-bus.
	p.UpdateUI(func() {
		ac := core.GetController()
		if ac == nil {
			return
		}
		if ac.StateService != nil {
			// Save means: state может быть сильно изменён. До интеграции
			// state.Diff (фаза 4.1 завершена, но calling-side ещё не передаёт
			// previous-state) — поднимаем оба маркера, чтобы пользователь явно
			// решил, что нужно: только Update (источники) или Restart (шаблон).
			//
			// Превышение по точности — better-safe; пользователь увидит два
			// маркера и нажмёт оба. Регрессия мелкая и осознанная; чистая
			// per-domain logic появится в следующей итерации (Diff vs initial state).
			ac.StateService.MarkCacheStale()
			ac.StateService.MarkConfigStale()
		}
		// Old broadcast-callback — оставлен до фазы 6 (UI listens via events).
		// Update marker → updateConfigInfo, Restart marker → updateRunningStatus.
		if ac.UIService != nil {
			if ac.UIService.UpdateConfigStatusFunc != nil {
				ac.UIService.UpdateConfigStatusFunc()
			}
			if ac.UIService.UpdateCoreStatusFunc != nil {
				ac.UIService.UpdateCoreStatusFunc()
			}
		}
		// Event-bus: подписчики UI/diagnostics могут реагировать точечно.
		if ac.EventBus != nil {
			ac.EventBus.Publish(events.Event{
				Kind: events.StateChanged,
				Payload: events.StateChangedPayload{
					Changed: []string{"saved"},
				},
			})
		}

		// AutoRebuildOnChange — если пользователь включил тоггл,
		// сразу пересобираем config.json из свежего state. Best-effort:
		// fail rebuild'а не отменяет успех Save (state.json уже на диске).
		if ac.FileService != nil {
			binDir := platform.GetBinDir(ac.FileService.ExecDir)
			if locale.LoadSettings(binDir).AutoRebuildOnChange {
				if err := ac.RebuildConfigIfDirty(); err != nil {
					debuglog.WarnLog("Save: AutoRebuild after Save failed: %v", err)
				}
			}
		}

		// Step 3: success dialog. Передаём путь к state.json — это то, что
		// мы только что записали; config.json пересоберётся при Update/Restart.
		p.showSaveSuccessDialog(p.statePathForLog())
	})

	p.UpdateSaveStatusText(locale.T("wizard.save.status_done"))
	p.UpdateSaveProgress(0.95)

	p.completeSaveOperation()
}

// statePathForLog возвращает каноничный путь state.json для логирования
// и success-диалога (без I/O — просто составляет путь по execDir).
func (p *WizardPresenter) statePathForLog() string {
	ac := core.GetController()
	if ac == nil || ac.FileService == nil {
		return ""
	}
	return filepath.Join(ac.FileService.ExecDir, "bin", wizardbusiness.WizardStatesDir, wizardmodels.StateFileName)
}

// saveStateOnly persist state.json и возвращает его путь (или "" при ошибке).
// SPEC 045 фаза 5.2 — единственный физический I/O на Save-пути.
func (p *WizardPresenter) saveStateOnly() string {
	ac := core.GetController()
	if ac == nil || ac.FileService == nil {
		debuglog.WarnLog("SaveConfig: controller or FileService not available")
		p.UpdateUI(func() {
			dialog.ShowError(errors.New(locale.T("wizard.save.error_controller")), p.guiState.Window)
		})
		return ""
	}
	statesDir := filepath.Join(ac.FileService.ExecDir, "bin", wizardbusiness.WizardStatesDir)
	statePath := filepath.Join(statesDir, wizardmodels.StateFileName)

	debuglog.InfoLog("SaveConfig: saving state.json to %s", statePath)
	if err := p.SaveCurrentState(); err != nil {
		debuglog.ErrorLog("SaveConfig: failed to save state.json: %v", err)
		p.UpdateUI(func() {
			dialog.ShowError(err, p.guiState.Window)
		})
		return ""
	}
	debuglog.InfoLog("SaveConfig: state.json saved successfully to %s", statePath)
	return statePath
}

// finalizeSaveOperation завершает операцию сохранения и восстанавливает UI.
func (p *WizardPresenter) finalizeSaveOperation() {
	debuglog.InfoLog("SaveConfig: save operation completed (or failed)")
	p.UpdateSaveStatusText("")
	// Всегда восстанавливаем кнопку Save, даже при ошибке
	p.SetSaveState("Save", -1)
	// Сбрасываем флаг парсинга на случай, если он завис
	if p.model.AutoParseInProgress {
		p.model.AutoParseInProgress = false
	}
}

// showSaveSuccessDialog показывает диалог успешного сохранения state.json.
//
// SPEC 045 фаза 5.B — Save теперь только пишет state.json; реальный config.json
// будет пересобран при ближайшем Update / Restart. Сообщение указывает путь
// к **config.json** (для совместимости i18n-ключей `wizard.save.dialog_*`),
// но фактическая запись config'а отложена.
func (p *WizardPresenter) showSaveSuccessDialog(configPath string) {
	message := locale.Tf("wizard.save.dialog_success_message", configPath)
	title := locale.T("wizard.save.dialog_success_title")

	// Create dialog with OK button that closes both dialog and wizard
	var d dialog.Dialog
	okButton := widget.NewButton(locale.T("dialog.ok"), func() {
		// Close dialog first
		if d != nil {
			d.Hide()
		}
		// Close wizard window only (not the main application)
		if p.guiState.Window != nil {
			p.guiState.Window.Close()
		}
	})
	okButton.Importance = widget.HighImportance

	buttonsRow := container.NewHBox(
		layout.NewSpacer(),
		okButton,
	)

	messageLabel := widget.NewLabel(message)

	d = dialogs.NewCustom(title, messageLabel, buttonsRow, "", p.guiState.Window)
	d.Show()
}

// completeSaveOperation завершает операцию сохранения с небольшой задержкой.
// Config.json already contains outbounds populated via PopulateParserMarkers —
// no immediate parser run needed. Subscriptions will refresh on the next auto-update cycle.
func (p *WizardPresenter) completeSaveOperation() {
	debuglog.InfoLog("SaveConfig: save complete, config.json contains populated outbounds")
	<-time.After(100 * time.Millisecond)
	p.UpdateSaveProgress(1.0)
	<-time.After(200 * time.Millisecond)
}
