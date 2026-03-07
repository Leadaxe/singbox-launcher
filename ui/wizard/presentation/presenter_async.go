// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_async.go содержит методы для асинхронных операций презентера:
//   - TriggerParseForPreview - запускает парсинг конфигурации для preview в отдельной горутине
//   - UpdateTemplatePreviewAsync - обновляет preview шаблона асинхронно в отдельной горутине
//
// Эти методы координируют вызовы бизнес-логики (parser.go, create_config.go) и обновление GUI
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

	"singbox-launcher/core"
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
	// Only ParserConfig is required; SourceURLs is not used (sources come from ParserConfig.Proxies).
	if strings.TrimSpace(p.model.ParserConfigJSON) == "" {
		return
	}

	p.model.AutoParseInProgress = true
	// Keep Save button visible; if user clicks Save during parse, save flow will wait (waitForParsingIfNeeded)
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
		ac := core.GetController()
		configService := &wizardbusiness.ConfigServiceAdapter{
			CoreConfigService: ac.ConfigService,
		}
		if err := wizardbusiness.ParseAndPreview(p, configService); err != nil {
			debuglog.ErrorLog("TriggerParseForPreview: ParseAndPreview failed: %v", err)
			return
		}
		p.RefreshOutboundOptions()
	}()
}

// UpdateTemplatePreviewAsync обновляет preview шаблона асинхронно.
func (p *WizardPresenter) UpdateTemplatePreviewAsync() {
	timing := debuglog.StartTiming("UpdateTemplatePreviewAsync")
	defer timing.EndWithDefer()

	if p.model.PreviewGenerationInProgress {
		debuglog.DebugLog("UpdateTemplatePreviewAsync: Preview generation already in progress, skipping")
		return
	}

	if p.model.TemplateData == nil || p.guiState.TemplatePreviewEntry == nil {
		debuglog.DebugLog("UpdateTemplatePreviewAsync: TemplateData or TemplatePreviewEntry is nil, returning early")
		return
	}

	p.model.PreviewGenerationInProgress = true
	p.SetTemplatePreviewText("Building preview...")
	if p.guiState.TemplatePreviewStatusLabel != nil {
		p.guiState.TemplatePreviewStatusLabel.SetText("⏳ Building preview configuration...")
	}

	go func() {
		goroutineTiming := debuglog.StartTiming("UpdateTemplatePreviewAsync: Goroutine")
		defer func() {
			goroutineTiming.End()
			p.model.PreviewGenerationInProgress = false
			SafeFyneDo(p.guiState.Window, func() {
			if p.guiState.ShowPreviewButton != nil {
				p.guiState.ShowPreviewButton.Enable()
			}
			})
		}()

		SafeFyneDo(p.guiState.Window, func() {
		if p.guiState.TemplatePreviewStatusLabel != nil {
			p.guiState.TemplatePreviewStatusLabel.SetText("⏳ Parsing ParserConfig...")
		}
		})

		buildStartTime := time.Now()
		debuglog.DebugLog("UpdateTemplatePreviewAsync: Calling BuildTemplateConfig")
		text, err := wizardbusiness.BuildTemplateConfig(p.model, true)
		buildDuration := time.Since(buildStartTime)
		if err != nil {
			goroutineTiming.LogTiming("BuildTemplateConfig", buildDuration)
			debuglog.ErrorLog("UpdateTemplatePreviewAsync: BuildTemplateConfig failed: %v", err)
			errorText := fmt.Sprintf("Preview error: %v", err)
			p.SetTemplatePreviewText(errorText)
			p.model.TemplatePreviewNeedsUpdate = false
			SafeFyneDo(p.guiState.Window, func() {
			if p.guiState.TemplatePreviewStatusLabel != nil {
				p.guiState.TemplatePreviewStatusLabel.SetText(fmt.Sprintf("❌ Error: %v", err))
			}
			})
			return
		}
		goroutineTiming.LogTiming("BuildTemplateConfig", buildDuration)
		debuglog.DebugLog("UpdateTemplatePreviewAsync: BuildTemplateConfig completed (result size: %d bytes)", len(text))

		isLargeText := len(text) > 50000
		p.SetTemplatePreviewText(text)

		if !isLargeText {
			SafeFyneDo(p.guiState.Window, func() {
			if p.guiState.TemplatePreviewStatusLabel != nil {
				p.guiState.TemplatePreviewStatusLabel.SetText("✅ Preview ready")
			}
			if p.guiState.ShowPreviewButton != nil {
				p.guiState.ShowPreviewButton.Enable()
			}
			})
			debuglog.DebugLog("UpdateTemplatePreviewAsync: Preview text inserted")
		} else {
			debuglog.DebugLog("UpdateTemplatePreviewAsync: Large text insertion started (status will update when complete)")
		}
	}()
}
