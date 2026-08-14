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
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/config"
	"singbox-launcher/core/events"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/configurator/business"
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

	// SPEC 097: для remote-таргета Save дополнительно материализует конфиг
	// в bin/remote-config.json и НЕ трогает локальные маркеры — локальное
	// ядро к этому состоянию отношения не имеет.
	if p.ConfigTarget() == constants.ConfigTargetRemote {
		p.exportRemoteConfig()
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

		// Auto-rebuild после Save — теперь безусловно (SPEC 045 фаза 9).
		// Старый AutoRebuildOnChange toggle был артефактом времён, когда
		// rebuild считался дорогой опциональной операцией. С SPEC 052 cache
		// + SPEC 045 build pipeline rebuild оффлайн и быстрый — нет смысла
		// требовать от пользователя ещё одного клика после Save.
		// Best-effort: fail rebuild'а не отменяет успех Save (state.json
		// уже на диске). Build error (например missing SRS) залогируется,
		// dirty маркеры на Update/Restart останутся горящими, пользователь
		// разрулит через UI.
		go func() {
			if err := ac.RebuildConfigIfDirty(); err != nil {
				debuglog.WarnLog("Save: auto-rebuild after Save failed: %v", err)
			}
		}()

		// Step 3: success dialog — только если визард-окно ещё живо. Если
		// пользователь закрыл окно во время Save (Close → CancelSaveOperation
		// сбросил SaveInProgress=false), диалог парентился бы к уже закрытому
		// окну (audit BUG3). Dirty-маркеры + auto-rebuild выше уже отработали —
		// это и есть результат Save, его отменять не нужно.
		if p.guiState.SaveInProgress && p.guiState.Window != nil {
			p.showSaveSuccessDialog(p.statePathForLog())
		}
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
	// SPEC 097/098: путь зависит от таргета и машины (remote-состояние живёт
	// в её подпапке); иначе лог показывал бы local-путь, пока запись шла в
	// remote/<id>/.
	return platform.GetWizardStatePathFor(ac.FileService.ExecDir, p.ConfigTarget(), p.ConfigMachineID())
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
	statePath := platform.GetWizardStatePathFor(ac.FileService.ExecDir, p.ConfigTarget(), p.ConfigMachineID())

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

// exportRemoteConfig собирает config для remote-таргета и пишет его в
// директорию ЭТОЙ машины: bin/wizard_states/remote/<id>/config.json
// (SPEC 097, machine-scoped в SPEC 098 §2.4).
//
// Почему не bin/config.json: последний принадлежит ЛОКАЛЬНОМУ ядру — его
// перезаписывает Update/Rebuild и с него стартует sing-box на этой машине.
// Конфиг, собранный для linux-роутера, там был бы либо затёрт, либо (хуже)
// запущен локально.
//
// Почему не общий bin/remote-config.json (как было до SPEC 098): файл был
// один на все машины, поэтому вторая настроенная машина молча затирала конфиг
// первой, а Deploy отправлял то, что оказалось записано последним. Теперь у
// каждой машины свой файл, и Deploy из её строки физически не может взять
// чужой.
//
// Локальные dirty-маркеры (MarkCacheStale / MarkConfigStale) намеренно НЕ
// поднимаются: они означают «локальный config.json устарел», а remote-Save
// его не касается.
func (p *WizardPresenter) exportRemoteConfig() {
	ac := core.GetController()
	if ac == nil || ac.FileService == nil {
		debuglog.WarnLog("exportRemoteConfig: controller/FileService unavailable")
		return
	}
	// Ноды разбирает парсер подписок; после смены таргета (или до первого
	// разбора) их ещё нет, и конфиг ушёл бы с ПУСТЫМИ секциями между
	// парсер-маркерами — валидный по check, но без единой прокси-ноды.
	// Лучше честно сказать, чем отдать пустышку под видом готового конфига.
	if p.model.PreviewNeedsParse || len(p.model.GeneratedOutbounds) == 0 {
		debuglog.WarnLog("exportRemoteConfig: nodes not parsed yet (needsParse=%v, outbounds=%d)",
			p.model.PreviewNeedsParse, len(p.model.GeneratedOutbounds))
		p.UpdateUI(func() {
			dialog.ShowError(errors.New(locale.T("wizard.save.remote_needs_parse")), p.guiState.Window)
		})
		return
	}
	// Страховка на случай, если визард для машины открыли в обход строки
	// списка: без state_dir пути .srs указывали бы в файловую систему
	// лаунчера, и ядро на той стороне не нашло бы наборы. Молча собрать такой
	// конфиг хуже, чем отказаться: сбой всплыл бы только после Deploy.
	if p.model.ResourceDir == "" && p.modelHasRuleSetFiles() {
		debuglog.WarnLog("exportRemoteConfig: no resource dir for machine %q — connect first", p.ConfigMachineID())
		p.UpdateUI(func() {
			dialog.ShowError(errors.New(locale.T("wizard.save.remote_needs_connect")), p.guiState.Window)
		})
		return
	}
	configText, err := wizardbusiness.BuildRemoteConfig(p.model)
	if err != nil {
		debuglog.ErrorLog("exportRemoteConfig: build failed: %v", err)
		p.UpdateUI(func() {
			dialog.ShowError(err, p.guiState.Window)
		})
		return
	}
	outPath := platform.GetRemoteConfigPathFor(ac.FileService.ExecDir, p.ConfigMachineID())
	// Директория машины могла ещё не существовать: состояние сохраняется
	// StateStore'ом, который создаёт её сам, но порядок вызовов тут не
	// гарантирован, а WriteFile каталоги не создаёт.
	if err := os.MkdirAll(filepath.Dir(outPath), platform.DefaultDirMode); err != nil {
		debuglog.ErrorLog("exportRemoteConfig: mkdir %s: %v", filepath.Dir(outPath), err)
		p.UpdateUI(func() {
			dialog.ShowError(err, p.guiState.Window)
		})
		return
	}
	if err := os.WriteFile(outPath, []byte(configText), platform.DefaultFileMode); err != nil {
		debuglog.ErrorLog("exportRemoteConfig: write %s: %v", outPath, err)
		p.UpdateUI(func() {
			dialog.ShowError(err, p.guiState.Window)
		})
		return
	}
	debuglog.InfoLog("exportRemoteConfig: wrote %s (%d bytes)", outPath, len(configText))
	p.UpdateUI(func() {
		p.showRemoteExportDialog(outPath)
	})
}

// showRemoteExportDialog — диалог успешного экспорта remote-конфига.
//
// Своя реализация вместо dialog.ShowInformation по той же причине, что и у
// local-пути (showSaveSuccessDialog): OK должен закрывать И диалог, И окно
// визарда. У ShowInformation кнопка только прячет сам диалог, поэтому после
// Save окно оставалось открытым — поведение расходилось с local-сохранением.
func (p *WizardPresenter) showRemoteExportDialog(outPath string) {
	var d dialog.Dialog
	okButton := widget.NewButton(locale.T("dialog.ok"), func() {
		if d != nil {
			d.Hide()
		}
		if p.guiState.Window != nil {
			p.guiState.Window.Close()
		}
	})
	okButton.Importance = widget.HighImportance
	buttonsRow := container.NewHBox(layout.NewSpacer(), okButton)

	// Пояснение переносится по словам, а вот путь — отдельным полем без
	// переноса: Label с TextWrapWord ломает длинный путь по СИМВОЛАМ
	// («/Applications/singbox-lau / ncher.app/...»), что нечитаемо и
	// невозможно скопировать. Entry ещё и выделяется мышью.
	messageLabel := widget.NewLabel(locale.T("wizard.save.remote_exported_body"))
	messageLabel.Wrapping = fyne.TextWrapWord

	pathField := widget.NewEntry()
	pathField.SetText(outPath)
	pathField.Wrapping = fyne.TextWrapOff

	body := container.NewVBox(messageLabel, pathField)

	d = dialogs.NewCustom(locale.T("wizard.save.remote_exported_title"),
		body, buttonsRow, "", p.guiState.Window)
	// Ширину задаём явно: Fyne считает её от min-size контента, а у
	// wrapping-Label он равен одной строке — диалог схлопывался в узкую
	// колонку (та же ловушка, что с длинными Label в других окнах проекта).
	d.Resize(fyne.NewSize(720, 260))
	d.Show()
}

// modelHasRuleSetFiles — есть ли в правилах наборы, которые лежат ФАЙЛОМ
// (скачанный .srs), а не inline-списком.
//
// Только они требуют ресурс-стора на удалённой машине: inline уезжает внутри
// самого конфига и никаких путей не просит.
func (p *WizardPresenter) modelHasRuleSetFiles() bool {
	if p.model == nil {
		return false
	}
	for _, rs := range p.model.CustomRules {
		if rs == nil || !rs.Enabled {
			continue
		}
		for _, raw := range rs.Rule.RuleSets {
			var m map[string]interface{}
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			if typ, _ := m["type"].(string); typ == "remote" || typ == "local" {
				return true
			}
		}
	}
	return false
}
