// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_save.go содержит методы для сохранения конфигурации:
//   - SaveConfig - асинхронное сохранение конфигурации с прогресс-баром и проверками
//
// SaveConfig выполняет следующие шаги:
//  1. Проверяет, что ParserConfig заполнен (хотя бы один proxy с Source или Connections)
//  2. Гарантирует наличие outbounds (ensureOutboundsParsed: ждёт текущий парсинг или запускает ParseAndPreview)
//  3. Генерирует конфигурацию из текущей модели (BuildTemplateConfig с пустыми @ParserSTART/@ParserEND маркерами)
//  4. Заполняет маркеры outbounds из памяти (PopulateParserMarkers), валидирует через sing-box, пишет config.json
//
// Все операции выполняются асинхронно в отдельной горутине с обновлением прогресс-бара.
//
// Сохранение конфигурации - это отдельная ответственность с сложной логикой.
// Содержит координацию нескольких бизнес-операций (парсинг, генерация, сохранение).
// Управляет прогресс-баром и диалогами на разных этапах сохранения.
//
// Используется в:
//   - wizard.go - SaveConfig вызывается при нажатии кнопки "Save" в визарде
//
// Использует:
//   - business/create_config.go - BuildTemplateConfig для генерации конфигурации
//   - business/saver.go - SaveConfigWithBackup для сохранения файла
//   - core/config.PopulateParserMarkers - заполнение маркеров @ParserSTART/@ParserEND из памяти
package presentation

import (
	"encoding/json"
	"errors"
	"fmt"
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
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/textnorm"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardmodels "singbox-launcher/ui/wizard/models"
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
// Before building config, ensures outbounds are parsed (waits for in-progress parse or runs ParseAndPreview).
// Then builds config, validates via sing-box check, and writes config.json with populated outbounds.
func (p *WizardPresenter) executeSaveOperation() {
	defer p.finalizeSaveOperation()

	// Step 0: Ensure outbounds are parsed (0.0-0.15)
	if err := p.ensureOutboundsParsed(); err != nil {
		debuglog.ErrorLog("SaveConfig: ensureOutboundsParsed failed: %v", err)
		p.UpdateUI(func() {
			dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.save.error_parse_failed"), err), p.guiState.Window)
		})
		return
	}

	// После долгого ensureOutboundsParsed снова сливаем виджеты в модель на UI-потоке (правки во время ожидания).
	p.MergeGUIToModelFromMainThread()

	// Step 1: Build config from current model (0.2-0.4)
	configText, err := p.buildConfigForSave()
	if err != nil {
		return
	}

	// Step 2: Validate (по временному файлу) и запись config.json (0.4-0.6)
	configPath, err := p.saveConfigFile(configText)
	if err != nil {
		return
	}

	// Step 3: Save state.json and show success dialog (0.6-0.9)
	p.saveStateAndShowSuccessDialog(configPath)

	// Step 4: Completion (0.9-1.0)
	p.completeSaveOperation()
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

const (
	parseWaitTimeout      = 60 * time.Second
	parseWaitPollInterval = 200 * time.Millisecond
)

// ensureOutboundsParsed guarantees GeneratedOutbounds are populated before save.
// If parsing is already in progress (user triggered preview), waits for completion.
// If outbounds are still empty, runs ParseAndPreview synchronously.
// Called from executeSaveOperation which already runs in a goroutine.
func (p *WizardPresenter) ensureOutboundsParsed() error {
	if p.model.AutoParseInProgress {
		debuglog.InfoLog("SaveConfig: waiting for in-progress parsing to complete")
		p.UpdateSaveStatusText(locale.T("wizard.save.status_waiting"))
		p.UpdateSaveProgress(0.05)

		deadline := time.Now().Add(parseWaitTimeout)
		for p.model.AutoParseInProgress {
			if time.Now().After(deadline) {
				return fmt.Errorf("subscription parsing timed out")
			}
			time.Sleep(parseWaitPollInterval)
		}
		debuglog.InfoLog("SaveConfig: in-progress parsing completed, outbounds: %d, endpoints: %d",
			len(p.model.GeneratedOutbounds), len(p.model.GeneratedEndpoints))
	}

	if len(p.model.GeneratedOutbounds) > 0 || len(p.model.GeneratedEndpoints) > 0 {
		return nil
	}

	debuglog.InfoLog("SaveConfig: no outbounds generated yet, running ParseAndPreview before save")
	p.UpdateSaveStatusText(locale.T("wizard.save.status_parsing"))
	p.UpdateSaveProgress(0.05)

	p.model.AutoParseInProgress = true
	configService := p.ConfigServiceAdapter()
	if configService == nil {
		p.model.AutoParseInProgress = false
		return fmt.Errorf("config service not available")
	}
	if err := wizardbusiness.ParseAndPreview(p, configService); err != nil {
		return fmt.Errorf("subscription parsing failed: %w", err)
	}
	p.RefreshOutboundOptions()

	debuglog.InfoLog("SaveConfig: ParseAndPreview completed, outbounds: %d, endpoints: %d",
		len(p.model.GeneratedOutbounds), len(p.model.GeneratedEndpoints))
	p.UpdateSaveProgress(0.15)
	return nil
}

// buildConfigForSave строит конфигурацию из шаблона и модели.
// Возвращает текст конфигурации или ошибку.
func (p *WizardPresenter) buildConfigForSave() (string, error) {
	p.UpdateSaveStatusText(locale.T("wizard.save.status_building"))
	p.UpdateSaveProgress(0.2)

	// Check if save operation was cancelled
	if !p.guiState.SaveInProgress {
		debuglog.DebugLog("presenter_save: Save operation cancelled before building config")
		return "", fmt.Errorf("save operation cancelled")
	}

	debuglog.InfoLog("SaveConfig: building template config")
	text, err := wizardbusiness.BuildTemplateConfig(p.model, false)
	if err != nil {
		debuglog.ErrorLog("SaveConfig: BuildTemplateConfig failed: %v", err)
		p.UpdateUI(func() {
			dialog.ShowError(err, p.guiState.Window)
		})
		return "", err
	}

	debuglog.InfoLog("SaveConfig: template config built successfully, length: %d", len(text))
	p.UpdateSaveStatusText(locale.T("wizard.save.status_preparing"))
	p.UpdateSaveProgress(0.4)
	return text, nil
}

// saveConfigFile валидирует конфиг через sing-box (по временному файлу) и при успехе пишет config.json с бэкапом.
// Возвращает путь к сохранённому файлу или ошибку (в т.ч. ошибку валидации).
func (p *WizardPresenter) saveConfigFile(configText string) (string, error) {
	if !p.guiState.SaveInProgress {
		debuglog.DebugLog("presenter_save: Save operation cancelled before saving file")
		return "", fmt.Errorf("save operation cancelled")
	}

	p.UpdateSaveStatusText(locale.T("wizard.save.status_validating"))
	p.UpdateSaveProgress(0.45)

	ac := core.GetController()
	if ac == nil || ac.FileService == nil {
		debuglog.WarnLog("SaveConfig: controller or FileService not available")
		p.UpdateUI(func() {
			dialog.ShowError(errors.New(locale.T("wizard.save.error_controller")), p.guiState.Window)
		})
		return "", fmt.Errorf("controller not available")
	}

	fileService := &wizardbusiness.FileServiceAdapter{FileService: ac.FileService}

	populateCheckText := func(text string) (string, error) {
		outboundsContent := strings.Join(p.model.GeneratedOutbounds, "\n")
		endpointsContent := strings.Join(p.model.GeneratedEndpoints, ",\n")
		if outboundsContent == "" && endpointsContent == "" {
			return text, nil
		}
		return config.PopulateParserMarkers(text, outboundsContent, endpointsContent)
	}

	debuglog.InfoLog("SaveConfig: validating then saving config file")
	path, err := wizardbusiness.SaveConfigWithBackup(fileService, configText, populateCheckText)
	if err != nil {
		debuglog.ErrorLog("SaveConfig: SaveConfigWithBackup failed: %v", err)
		p.UpdateUI(func() {
			p.showSaveErrorDialog(err)
		})
		return "", err
	}

	debuglog.InfoLog("SaveConfig: config saved to %s", path)
	p.UpdateSaveStatusText(locale.T("wizard.save.status_saving_state"))
	p.UpdateSaveProgress(0.6)
	return path, nil
}

// showSaveErrorDialog показывает ошибку сохранения; при ValidationError — диалог с кнопкой «Копировать конфиг».
func (p *WizardPresenter) showSaveErrorDialog(err error) {
	var valErr *wizardbusiness.ValidationError
	if errors.As(err, &valErr) && valErr.ConfigText != "" {
		p.showValidationErrorDialog(valErr)
		return
	}
	dialog.ShowError(err, p.guiState.Window)
}

// showValidationErrorDialog показывает ошибку валидации и кнопку «Копировать конфиг» в буфер обмена.
func (p *WizardPresenter) showValidationErrorDialog(valErr *wizardbusiness.ValidationError) {
	if p.guiState.Window == nil {
		return
	}
	msg := textnorm.StripANSI(valErr.Error())
	messageLabel := widget.NewLabel(msg)
	messageLabel.Wrapping = fyne.TextWrapWord

	scroll := container.NewScroll(messageLabel)
	// Message area: capped so the dialog stays usable without dominating the wizard.
	const maxScrollW, maxScrollH = float32(500), float32(180)
	const minScrollW, minScrollH = float32(130), float32(50)
	scrollW := maxScrollW
	scrollH := maxScrollH
	if p.guiState.Window != nil {
		sz := p.guiState.Window.Canvas().Size()
		if sz.Width > 100 {
			availW := sz.Width - 72
			if availW < scrollW {
				scrollW = availW
			}
		}
		if sz.Height > 160 {
			availH := sz.Height - 140
			if availH < scrollH {
				scrollH = availH
			}
		}
	}
	if scrollW < minScrollW {
		scrollW = minScrollW
	}
	if scrollH < minScrollH {
		scrollH = minScrollH
	}
	if scrollW > maxScrollW {
		scrollW = maxScrollW
	}
	if scrollH > maxScrollH {
		scrollH = maxScrollH
	}
	scroll.SetMinSize(fyne.NewSize(scrollW, scrollH))
	mainArea := container.NewPadded(scroll)

	var d dialog.Dialog
	copyBtn := widget.NewButton(locale.T("wizard.save.button_copy"), func() {
		if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
			app.Clipboard().SetContent(valErr.ConfigText)
			if p.guiState.Window != nil {
				dialogs.ShowAutoHideInfo(app, p.guiState.Window, locale.T("wizard.save.dialog_copied_title"), locale.T("wizard.save.dialog_copied_message"))
			}
		}
	})
	copyBtn.Importance = widget.MediumImportance
	closeBtn := widget.NewButton(locale.T("dialog.close"), func() {
		if d != nil {
			d.Hide()
		}
	})
	closeBtn.Importance = widget.HighImportance

	buttons := container.NewHBox(layout.NewSpacer(), copyBtn, closeBtn)
	d = dialogs.NewCustom(locale.T("wizard.save.dialog_validation_failed"), mainArea, buttons, "", p.guiState.Window)
	d.Show()
	// Whole popup size (title + padding + button row); content min alone is sometimes under-applied until shown.
	if p.guiState.Window != nil {
		ws := p.guiState.Window.Canvas().Size()
		const chromeW, chromeH = float32(32), float32(32)
		rw := scrollW + chromeW
		rh := scrollH + chromeH
		if rw > ws.Width-24 {
			rw = ws.Width - 24
		}
		if rh > ws.Height-24 {
			rh = ws.Height - 24
		}
		if rw >= 100 && rh >= 65 {
			d.Resize(fyne.NewSize(rw, rh))
		}
	}
}

// saveStateAndShowSuccessDialog сохраняет state.json и показывает диалог успешного сохранения.
// Вызывается только после успешной валидации и записи config.json, поэтому диалог всегда с «Validation: Passed».
func (p *WizardPresenter) saveStateAndShowSuccessDialog(configPath string) {
	// Check if save operation was cancelled
	if !p.guiState.SaveInProgress {
		debuglog.DebugLog("presenter_save: Save operation cancelled before saving state")
		return
	}
	p.UpdateSaveStatusText(locale.T("wizard.save.status_saving_state"))
	p.UpdateSaveProgress(0.7)

	ac := core.GetController()
	// Получаем путь к state.json для логирования
	statesDir := filepath.Join(ac.FileService.ExecDir, "bin", wizardbusiness.WizardStatesDir)
	statePath := filepath.Join(statesDir, wizardmodels.StateFileName)

	p.UpdateUI(func() {
		// Wizard committed new template/state. Mark config dirty so the Update
		// button shows an asterisk until the next parser run reflects the change.
		if ac.StateService != nil {
			ac.StateService.SetTemplateDirty(true)
		}
		// Update config status in Core Dashboard
		if ac.UIService != nil && ac.UIService.UpdateConfigStatusFunc != nil {
			ac.UIService.UpdateConfigStatusFunc()
		}

		// Сохраняем текущее состояние в state.json после успешного сохранения конфигурации
		// Сохранение происходит всегда, независимо от hasChanges
		debuglog.InfoLog("SaveConfig: saving state.json to %s", statePath)
		if err := p.SaveCurrentState(); err != nil {
			debuglog.WarnLog("presenter_save: failed to save state after config save: %v", err)
		} else {
			debuglog.InfoLog("SaveConfig: state.json saved successfully to %s", statePath)
		}

		// Логируем итоговую информацию о сохранении
		debuglog.InfoLog("SaveConfig: completed - config.json=%s, state.json=%s", configPath, statePath)

		p.showSaveSuccessDialog(configPath)
	})
	p.UpdateSaveStatusText(locale.T("wizard.save.status_done"))
	p.UpdateSaveProgress(0.9)
}

// showSaveSuccessDialog показывает диалог успешного сохранения (вызывается только после успешной валидации).
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
