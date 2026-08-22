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
//
// Сохранение конфига ждёт/запускает парсинг через presenter_save.ensureOutboundsParsed, не через TriggerParseForPreview.
package presentation

import (
	"strings"

	"singbox-launcher/core"
	"singbox-launcher/internal/debuglog"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// TriggerParseForPreview запускает парсинг конфигурации для preview.
func (p *WizardPresenter) TriggerParseForPreview() {
	if p.model.AutoParseInProgress {
		return
	}
	if !p.model.PreviewNeedsParse && len(p.model.GeneratedOutbounds) > 0 {
		return
	}
	// SPEC 104: прежнее условие требовало ещё и редактор ParserConfig JSON;
	// после его удаления с вкладки Направлений оно молча отключило бы
	// превью целиком.
	if p.guiState.SourceURLEntry == nil {
		return
	}
	p.MergeGUIToModel()
	// Only ParserConfig is required; SourceURLs is not used (sources come from ParserConfig.Proxies).
	if strings.TrimSpace(p.model.ParserConfigJSON) == "" {
		return
	}

	p.model.AutoParseInProgress = true
	// Save остаётся доступной; при нажатии Save ensureOutboundsParsed ждёт окончания AutoParseInProgress.

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
		// Ради этого вызов и остаётся после удаления вкладки Preview:
		// разбор подписок обновляет список outbound'ов, на который
		// опираются Rules и Направления.
		p.RefreshOutboundOptions()
	}()
}
