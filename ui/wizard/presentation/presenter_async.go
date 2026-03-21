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
// Ошибки парсинга в TriggerParseForPreview пишутся в лог; UpdateTemplatePreviewAsync может отразить ошибку в тексте preview.
//
// Используется в:
//   - wizard.go — TriggerParseForPreview при смене вкладок; UpdateTemplatePreviewAsync при необходимости обновить preview
//   - tabs/source_tab.go — UpdateTemplatePreviewAsync после успешного парсинга
// Сохранение конфига ждёт/запускает парсинг через presenter_save.ensureOutboundsParsed, не через TriggerParseForPreview.
package presentation

import (
	"strings"
	"time"

	"singbox-launcher/core"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
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
	p.MergeGUIToModel()
	// Only ParserConfig is required; SourceURLs is not used (sources come from ParserConfig.Proxies).
	if strings.TrimSpace(p.model.ParserConfigJSON) == "" {
		return
	}

	p.model.AutoParseInProgress = true
	// Save остаётся доступной; при нажатии Save ensureOutboundsParsed ждёт окончания AutoParseInProgress.
	if p.guiState.TemplatePreviewStatusLabel != nil {
		p.guiState.TemplatePreviewStatusLabel.SetText(locale.T("wizard.preview.status_parsing"))
	}
	if p.guiState.TemplatePreviewEntry != nil {
		p.SetTemplatePreviewText(locale.T("wizard.preview.text_parsing"))
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
	p.SetTemplatePreviewText(locale.T("wizard.preview.text_building"))
	if p.guiState.TemplatePreviewStatusLabel != nil {
		p.guiState.TemplatePreviewStatusLabel.SetText(locale.T("wizard.preview.status_building"))
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
			p.guiState.TemplatePreviewStatusLabel.SetText(locale.T("wizard.preview.status_parsing_config"))
		}
		})

		buildStartTime := time.Now()
		debuglog.DebugLog("UpdateTemplatePreviewAsync: Calling BuildTemplateConfig")
		text, err := wizardbusiness.BuildTemplateConfig(p.model, true)
		buildDuration := time.Since(buildStartTime)
		if err != nil {
			goroutineTiming.LogTiming("BuildTemplateConfig", buildDuration)
			debuglog.ErrorLog("UpdateTemplatePreviewAsync: BuildTemplateConfig failed: %v", err)
			errorText := locale.Tf("wizard.preview.error", err)
			p.SetTemplatePreviewText(errorText)
			p.model.TemplatePreviewNeedsUpdate = false
			SafeFyneDo(p.guiState.Window, func() {
			if p.guiState.TemplatePreviewStatusLabel != nil {
				p.guiState.TemplatePreviewStatusLabel.SetText(locale.Tf("wizard.preview.status_error", err))
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
				p.guiState.TemplatePreviewStatusLabel.SetText(locale.T("wizard.preview.status_ready"))
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
