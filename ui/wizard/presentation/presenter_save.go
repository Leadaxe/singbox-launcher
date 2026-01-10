// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_save.go содержит методы для сохранения конфигурации:
//   - SaveConfig - асинхронное сохранение конфигурации с прогресс-баром и проверками
//
// SaveConfig выполняет следующие шаги:
//  1. Проверяет, что ParserConfig и SourceURLs заполнены
//  2. При необходимости запускает парсинг конфигурации (если PreviewNeedsParse)
//  3. Генерирует финальную конфигурацию из шаблона и модели (BuildTemplateConfig)
//  4. Сохраняет конфигурацию в файл с созданием бэкапа (SaveConfigWithBackup)
//  5. Показывает диалог успешного сохранения и закрывает визард
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
//   - business/generator.go - BuildTemplateConfig для генерации конфигурации
//   - business/saver.go - SaveConfigWithBackup для сохранения файла
//   - presenter_async.go - TriggerParseForPreview для парсинга при необходимости
package presentation

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2/dialog"

	wizardbusiness "singbox-launcher/ui/wizard/business"
)

// SaveConfig сохраняет конфигурацию асинхронно с прогресс-баром.
func (p *WizardPresenter) SaveConfig() {
	p.SyncGUIToModel()

	if strings.TrimSpace(p.model.ParserConfigJSON) == "" {
		dialog.ShowError(fmt.Errorf("ParserConfig is empty"), p.guiState.Window)
		return
	}
	if strings.TrimSpace(p.model.SourceURLs) == "" {
		dialog.ShowError(fmt.Errorf("VLESS URL is empty"), p.guiState.Window)
		return
	}
	if p.guiState.SaveInProgress {
		dialog.ShowInformation("Saving", "Save operation already in progress... Please wait.", p.guiState.Window)
		return
	}
	if p.model.AutoParseInProgress {
		dialog.ShowInformation("Parsing", "Parsing in progress... Please wait.", p.guiState.Window)
		return
	}

	p.SetSaveState("", 0.0)
	go func() {
		defer func() {
			p.SetSaveState("Save", -1)
		}()

		// Step 0: Check and wait for parsing if needed (0-40%)
		if p.model.PreviewNeedsParse || p.model.AutoParseInProgress {
			p.UpdateSaveProgress(0.05)

			if !p.model.AutoParseInProgress {
				p.model.AutoParseInProgress = true
				configService := &wizardbusiness.ConfigServiceAdapter{
					CoreConfigService: p.controller.ConfigService,
				}
				go wizardbusiness.ParseAndPreview(p.model, p, configService)
			}

			maxWaitTime := 60 * time.Second
			startTime := time.Now()
			iterations := 0
			for p.model.AutoParseInProgress {
				if time.Since(startTime) > maxWaitTime {
					SafeFyneDo(p.guiState.Window, func() {
						dialog.ShowError(fmt.Errorf("Parsing timeout: operation took too long"), p.guiState.Window)
					})
					return
				}
				time.Sleep(100 * time.Millisecond)
				iterations++
				progressRange := 0.35
				baseProgress := 0.05
				cycleProgress := float64(iterations%40) / 40.0
				currentProgress := baseProgress + cycleProgress*progressRange
				p.UpdateSaveProgress(currentProgress)
			}
			p.UpdateSaveProgress(0.4)
		}

		// Step 1: Build config (40-80%)
		p.UpdateSaveProgress(0.4)
		text, err := wizardbusiness.BuildTemplateConfig(p.model, false)
		if err != nil {
			SafeFyneDo(p.guiState.Window, func() {
				dialog.ShowError(err, p.guiState.Window)
			})
			return
		}
		p.UpdateSaveProgress(0.8)

		// Step 2: Save file (80-95%)
		fileService := &wizardbusiness.FileServiceAdapter{
			FileService: p.controller.FileService,
		}
		path, err := wizardbusiness.SaveConfigWithBackup(fileService, text)
		if err != nil {
			SafeFyneDo(p.guiState.Window, func() {
				dialog.ShowError(err, p.guiState.Window)
			})
			return
		}
		p.UpdateSaveProgress(0.9)

		// Step 3: Validate config with sing-box (90-95%)
		singBoxPath := ""
		if p.controller.FileService != nil {
			singBoxPath = p.controller.FileService.SingboxPath
		}

		validationErr := wizardbusiness.ValidateConfigWithSingBox(path, singBoxPath)
		p.UpdateSaveProgress(0.95)

		// Step 4: Completion (95-100%)
		time.Sleep(100 * time.Millisecond)
		p.UpdateSaveProgress(1.0)
		time.Sleep(200 * time.Millisecond)

		SafeFyneDo(p.guiState.Window, func() {
			// Show result with validation status
			message := fmt.Sprintf("Config written to %s", path)
			if validationErr != nil {
				message += fmt.Sprintf("\n\n⚠️ Validation warning:\n%v\n\nPlease check the config manually.", validationErr)
				dialog.ShowInformation("Config Saved (with warnings)", message, p.guiState.Window)
			} else {
				message += "\n\n✅ Validation: Passed"
				dialog.ShowInformation("Config Saved", message, p.guiState.Window)
			}

			// Update config status in Core Dashboard
			if p.controller.UIService != nil && p.controller.UIService.UpdateConfigStatusFunc != nil {
				p.controller.UIService.UpdateConfigStatusFunc()
			}
			p.guiState.Window.Close()
		})
	}()
}
