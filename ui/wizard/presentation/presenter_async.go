// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_async.go содержит методы для асинхронных операций презентера:
//   - TriggerParseForPreview - запускает парсинг конфигурации для preview в отдельной горутине
//   - UpdateTemplatePreviewAsync - обновляет preview шаблона асинхронно в отдельной горутине
//
// Эти методы координируют вызовы бизнес-логики (parser.go, generator.go) и обновление GUI
// через UIUpdater, обеспечивая безопасное обновление GUI из других горутин через SafeFyneDo.
//
// Асинхронные операции имеют отдельную ответственность от синхронных методов.
// Содержат сложную логику управления состоянием прогресса и блокировками.
// Обрабатывают ошибки асинхронных операций и показывают диалоги пользователю.
//
// Используется в:
//   - wizard.go - UpdateTemplatePreviewAsync вызывается при изменении данных, требующих обновления preview
//   - presenter_save.go - TriggerParseForPreview вызывается при сохранении, если нужен парсинг
//   - tabs/source_tab.go - UpdateTemplatePreviewAsync вызывается после успешного парсинга
package presentation

import (
	"fmt"
	"strings"
	"time"

	"singbox-launcher/internal/debuglog"
	wizardbusiness "singbox-launcher/ui/wizard/business"
)

// TriggerParseForPreview запускает парсинг конфигурации для preview.
func (p *WizardPresenter) TriggerParseForPreview() {
	if p.model.AutoParseInProgress {
		return
	}
	if !p.model.PreviewNeedsParse && len(p.model.GeneratedOutbounds) > 0 {
		return
	}
	if p.guiState.SourceURLEntry == nil || p.guiState.ParserConfigEntry == nil {
		return
	}
	p.SyncGUIToModel()
	if strings.TrimSpace(p.model.SourceURLs) == "" || strings.TrimSpace(p.model.ParserConfigJSON) == "" {
		return
	}

	p.model.AutoParseInProgress = true
	p.UpdateSaveButtonText("")
	if p.guiState.TemplatePreviewStatusLabel != nil {
		p.guiState.TemplatePreviewStatusLabel.SetText("⏳ Parsing subscriptions and generating outbounds...")
	}
	if p.guiState.TemplatePreviewEntry != nil {
		p.SetTemplatePreviewText("Parsing configuration... Please wait.")
	}

	go func() {
		defer func() {
			p.model.AutoParseInProgress = false
		}()
		configService := &wizardbusiness.ConfigServiceAdapter{
			CoreConfigService: p.controller.ConfigService,
		}
		if err := wizardbusiness.ParseAndPreview(p.model, p, configService); err != nil {
			debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, "TriggerParseForPreview: ParseAndPreview failed: %v", err)
			return
		}
		p.RefreshOutboundOptions()
	}()
}

// UpdateTemplatePreviewAsync обновляет preview шаблона асинхронно.
func (p *WizardPresenter) UpdateTemplatePreviewAsync() {
	startTime := time.Now()
	debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "UpdateTemplatePreviewAsync: START at %s", startTime.Format("15:04:05.000"))

	if p.model.PreviewGenerationInProgress {
		debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "UpdateTemplatePreviewAsync: Preview generation already in progress, skipping")
		return
	}

	if p.model.TemplateData == nil || p.guiState.TemplatePreviewEntry == nil {
		debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "UpdateTemplatePreviewAsync: TemplateData or TemplatePreviewEntry is nil, returning early")
		return
	}

	p.model.PreviewGenerationInProgress = true
	p.SetTemplatePreviewText("Building preview...")
	if p.guiState.TemplatePreviewStatusLabel != nil {
		p.guiState.TemplatePreviewStatusLabel.SetText("⏳ Building preview configuration...")
	}
	p.UpdateSaveButtonText("")

	go func() {
		goroutineStartTime := time.Now()
		debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "UpdateTemplatePreviewAsync: Goroutine START at %s", goroutineStartTime.Format("15:04:05.000"))

		defer func() {
			totalDuration := time.Since(goroutineStartTime)
			debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "UpdateTemplatePreviewAsync: Goroutine END (duration: %v)", totalDuration)
			p.model.PreviewGenerationInProgress = false
			p.UpdateSaveButtonText("Save")
			if p.guiState.ShowPreviewButton != nil {
				p.guiState.ShowPreviewButton.Enable()
			}
		}()

		if p.guiState.TemplatePreviewStatusLabel != nil {
			p.guiState.TemplatePreviewStatusLabel.SetText("⏳ Parsing ParserConfig...")
		}

		buildStartTime := time.Now()
		debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "UpdateTemplatePreviewAsync: Calling BuildTemplateConfig")
		text, err := wizardbusiness.BuildTemplateConfig(p.model, true)
		buildDuration := time.Since(buildStartTime)
		if err != nil {
			debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "UpdateTemplatePreviewAsync: BuildTemplateConfig failed (took %v): %v", buildDuration, err)
			errorText := fmt.Sprintf("Preview error: %v", err)
			p.SetTemplatePreviewText(errorText)
			p.model.TemplatePreviewNeedsUpdate = false
			if p.guiState.TemplatePreviewStatusLabel != nil {
				p.guiState.TemplatePreviewStatusLabel.SetText(fmt.Sprintf("❌ Error: %v", err))
			}
			return
		}
		debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "UpdateTemplatePreviewAsync: BuildTemplateConfig completed in %v (result size: %d bytes)",
			buildDuration, len(text))

		isLargeText := len(text) > 50000
		p.SetTemplatePreviewText(text)

		if !isLargeText {
			if p.guiState.TemplatePreviewStatusLabel != nil {
				p.guiState.TemplatePreviewStatusLabel.SetText("✅ Preview ready")
			}
			if p.guiState.ShowPreviewButton != nil {
				p.guiState.ShowPreviewButton.Enable()
			}
			debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "UpdateTemplatePreviewAsync: Preview text inserted")
		} else {
			debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "UpdateTemplatePreviewAsync: Large text insertion started (status will update when complete)")
		}
	}()
}


