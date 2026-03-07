// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_save.go содержит методы для сохранения конфигурации:
//   - SaveConfig - асинхронное сохранение конфигурации с прогресс-баром и проверками
//
// SaveConfig выполняет следующие шаги:
//  1. Проверяет, что ParserConfig заполнен (хотя бы один proxy с Source или Connections)
//  2. Генерирует конфигурацию из текущей модели (BuildTemplateConfig; без ожидания парсинга outbounds)
//  3. Сохраняет конфигурацию в файл с созданием бэкапа (SaveConfigWithBackup)
//  4. Показывает диалог успешного сохранения; после сохранения в фоне запускается RunParserProcess (update from subscriptions)
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
//   - core.RunParserProcess - обновление конфига из подписок после сохранения (в фоне)
package presentation

import (
	"encoding/json"
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
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
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
		dialog.ShowError(fmt.Errorf("ParserConfig is empty"), p.guiState.Window)
		return false
	}
	var pc config.ParserConfig
	if err := json.Unmarshal([]byte(p.model.ParserConfigJSON), &pc); err != nil {
		debuglog.WarnLog("SaveConfig: ParserConfig JSON invalid: %v", err)
		dialog.ShowError(fmt.Errorf("ParserConfig is invalid: %w", err), p.guiState.Window)
		return false
	}
	for _, px := range pc.ParserConfig.Proxies {
		if strings.TrimSpace(px.Source) != "" || len(px.Connections) > 0 {
			return true
		}
	}
	debuglog.WarnLog("SaveConfig: no proxy with source or connections in ParserConfig")
	dialog.ShowError(fmt.Errorf("Add at least one source: use the Sources tab (Add) or add proxies in ParserConfig on the Outbounds tab."), p.guiState.Window)
	return false
}

// checkSaveOperationState проверяет состояние операции сохранения.
func (p *WizardPresenter) checkSaveOperationState() bool {
	if p.guiState.SaveInProgress {
		debuglog.WarnLog("SaveConfig: Save operation already in progress")
		dialog.ShowInformation("Saving", "Save operation already in progress... Please wait.", p.guiState.Window)
		return false
	}
	return true
}

// executeSaveOperation выполняет операцию сохранения в отдельной горутине.
// Save does not wait for outbounds parsing: it writes current model state (existing GeneratedOutbounds or empty).
// After save, config update from subscriptions is triggered in background (RunParserProcess).
func (p *WizardPresenter) executeSaveOperation() {
	defer p.finalizeSaveOperation()

	// Step 1: Build config from current model (0.2-0.4)
	configText, err := p.buildConfigForSave()
	if err != nil {
		return
	}

	// Step 2: Save file (0.4-0.5)
	configPath, err := p.saveConfigFile(configText)
	if err != nil {
		return
	}

	// Step 3: Validate config with sing-box (0.5-0.6)
	validationErr := p.validateConfigFile(configPath)

	// Step 4: Save state.json and show success dialog (0.6-0.9)
	p.saveStateAndShowSuccessDialog(configPath, validationErr)

	// Step 5: Completion (0.9-1.0)
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

// buildConfigForSave строит конфигурацию из шаблона и модели.
// Возвращает текст конфигурации или ошибку.
func (p *WizardPresenter) buildConfigForSave() (string, error) {
	p.UpdateSaveStatusText("Building config...")
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
	p.UpdateSaveStatusText("Saving file...")
	p.UpdateSaveProgress(0.4)
	return text, nil
}

// saveConfigFile сохраняет конфигурацию в файл с созданием бэкапа.
// Возвращает путь к сохраненному файлу или ошибку.
func (p *WizardPresenter) saveConfigFile(configText string) (string, error) {
	// Check if save operation was cancelled
	if !p.guiState.SaveInProgress {
		debuglog.DebugLog("presenter_save: Save operation cancelled before saving file")
		return "", fmt.Errorf("save operation cancelled")
	}

	ac := core.GetController()
	fileService := &wizardbusiness.FileServiceAdapter{
		FileService: ac.FileService,
	}
	debuglog.InfoLog("SaveConfig: saving config file")
	path, err := wizardbusiness.SaveConfigWithBackup(fileService, configText)
	if err != nil {
		debuglog.ErrorLog("SaveConfig: SaveConfigWithBackup failed: %v", err)
		p.UpdateUI(func() {
			dialog.ShowError(err, p.guiState.Window)
		})
		return "", err
	}

	debuglog.InfoLog("SaveConfig: config saved to %s", path)
	p.UpdateSaveStatusText("Validating...")
	p.UpdateSaveProgress(0.5)
	return path, nil
}

// validateConfigFile валидирует сохраненный конфиг с помощью sing-box.
// Возвращает ошибку валидации, если она есть.
func (p *WizardPresenter) validateConfigFile(configPath string) error {
	// Check if save operation was cancelled
	if !p.guiState.SaveInProgress {
		debuglog.DebugLog("presenter_save: Save operation cancelled before validation")
		return fmt.Errorf("save operation cancelled")
	}

	ac := core.GetController()
	singBoxPath := ""
	if ac != nil && ac.FileService != nil {
		singBoxPath = ac.FileService.SingboxPath
	}

	validationErr := wizardbusiness.ValidateConfigWithSingBox(configPath, singBoxPath)
	p.UpdateSaveStatusText("Saving state...")
	p.UpdateSaveProgress(0.6)
	return validationErr
}

// saveStateAndShowSuccessDialog сохраняет state.json и показывает диалог успешного сохранения.
func (p *WizardPresenter) saveStateAndShowSuccessDialog(configPath string, validationErr error) {
	// Check if save operation was cancelled
	if !p.guiState.SaveInProgress {
		debuglog.DebugLog("presenter_save: Save operation cancelled before saving state")
		return
	}
	p.UpdateSaveStatusText("Saving state...")
	p.UpdateSaveProgress(0.7)

	ac := core.GetController()
	// Получаем путь к state.json для логирования
	statesDir := filepath.Join(ac.FileService.ExecDir, "bin", wizardbusiness.WizardStatesDir)
	statePath := filepath.Join(statesDir, wizardmodels.StateFileName)

	p.UpdateUI(func() {
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

		// Перезапускаем сервер, если он запущен
		ac := core.GetController()
		if ac != nil && ac.RunningState != nil && ac.RunningState.IsRunning() {
			debuglog.InfoLog("SaveConfig: restarting sing-box server after config save")
			// Останавливаем сервер
			core.StopSingBoxProcess()
			// Ждем немного, чтобы процесс корректно остановился
			go func() {
				<-time.After(500 * time.Millisecond)
				// Проверяем, что сервер остановился
				ticker := time.NewTicker(100 * time.Millisecond)
				defer ticker.Stop()
				timeout := time.After(2 * time.Second)
				for {
					select {
					case <-timeout:
						debuglog.WarnLog("SaveConfig: timeout waiting for sing-box to stop")
						return
					case <-ticker.C:
						if !ac.RunningState.IsRunning() {
							// Сервер остановлен, запускаем заново
							debuglog.InfoLog("SaveConfig: sing-box stopped, starting again")
							core.StartSingBoxProcess(true) // skipRunningCheck = true
							return
						}
					}
				}
			}()
		}

		// Show success dialog
		p.showSaveSuccessDialog(configPath, validationErr)
	})
	p.UpdateSaveStatusText("Done")
	p.UpdateSaveProgress(0.9)
}

// showSaveSuccessDialog показывает диалог успешного сохранения.
func (p *WizardPresenter) showSaveSuccessDialog(configPath string, validationErr error) {
	// Build message with validation status
	message := fmt.Sprintf("Config written to %s", configPath)
	if validationErr != nil {
		message += fmt.Sprintf("\n\n⚠️ Validation warning:\n%v\n\nPlease check the config manually.", validationErr)
	} else {
		message += "\n\n✅ Validation: Passed"
	}

	// Determine dialog title
	title := "Config Saved"
	if validationErr != nil {
		title = "Config Saved (with warnings)"
	}

	// Create dialog with OK button that closes both dialog and wizard
	var d dialog.Dialog
	okButton := widget.NewButton("OK", func() {
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
// Triggers config update from subscriptions in background (same as "Update" on main tab).
func (p *WizardPresenter) completeSaveOperation() {
	debuglog.InfoLog("SaveConfig: triggering config update from subscriptions (background)")
	go core.RunParserProcess()
	<-time.After(100 * time.Millisecond)
	p.UpdateSaveProgress(1.0)
	<-time.After(200 * time.Millisecond)
}
