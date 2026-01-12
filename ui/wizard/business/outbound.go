// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл outbound.go содержит функции для работы с outbounds:
//   - GetAvailableOutbounds - получение списка доступных outbound тегов из модели (ParserConfig, GeneratedOutbounds)
//   - EnsureDefaultAvailableOutbounds - обеспечивает наличие обязательных outbounds (direct-out, reject, drop)
//   - EnsureFinalSelected - обеспечивает выбранный final outbound в модели
//
// Эти функции работают с WizardModel (чистыми данными), без зависимостей от GUI.
// Используются в презентере при обновлении опций outbound для правил маршрутизации.
//
// Используется в:
//   - presentation/presenter_methods.go - RefreshOutboundOptions вызывает GetAvailableOutbounds и EnsureFinalSelected
//   - business/generator.go - GetAvailableOutbounds используется при генерации конфигурации
package business

import (
	"encoding/json"
	"sort"

	"singbox-launcher/core/config"
	wizardmodels "singbox-launcher/ui/wizard/models"
)

// GetAvailableOutbounds возвращает список доступных outbound тегов из модели.
func GetAvailableOutbounds(model *wizardmodels.WizardModel) []string {
	tags := map[string]struct{}{
		wizardmodels.DefaultOutboundTag: {},
		wizardmodels.RejectActionName:   {},
		"drop":                          {}, // Always include "drop" in available options
	}

	var parserCfg *config.ParserConfig
	if model.ParserConfig != nil {
		parserCfg = model.ParserConfig
	} else if model.ParserConfigJSON != "" {
		var parsed config.ParserConfig
		if err := json.Unmarshal([]byte(model.ParserConfigJSON), &parsed); err == nil {
			parserCfg = &parsed
		}
	}

	if parserCfg != nil {
		// Add global outbounds
		for _, outbound := range parserCfg.ParserConfig.Outbounds {
			if outbound.IsWizardHidden() {
				continue
			}
			if outbound.Tag != "" {
				tags[outbound.Tag] = struct{}{}
			}
			for _, extra := range outbound.AddOutbounds {
				tags[extra] = struct{}{}
			}
		}
		// Add local outbounds from all ProxySource
		for _, proxySource := range parserCfg.ParserConfig.Proxies {
			for _, outbound := range proxySource.Outbounds {
				if outbound.IsWizardHidden() {
					continue
				}
				if outbound.Tag != "" {
					tags[outbound.Tag] = struct{}{}
				}
				for _, extra := range outbound.AddOutbounds {
					tags[extra] = struct{}{}
				}
			}
		}
	}

	result := make([]string, 0, len(tags))
	for tag := range tags {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

// EnsureDefaultAvailableOutbounds обеспечивает наличие дефолтных outbounds в списке.
func EnsureDefaultAvailableOutbounds(outbounds []string) []string {
	if len(outbounds) == 0 {
		return []string{wizardmodels.DefaultOutboundTag, wizardmodels.RejectActionName}
	}
	return outbounds
}

// EnsureFinalSelected обеспечивает, что final outbound выбран из доступных опций.
func EnsureFinalSelected(model *wizardmodels.WizardModel, options []string) {
	options = EnsureDefaultAvailableOutbounds(options)
	preferred := model.SelectedFinalOutbound
	if preferred == "" && model.TemplateData != nil && model.TemplateData.DefaultFinal != "" {
		preferred = model.TemplateData.DefaultFinal
	}
	if preferred == "" {
		preferred = wizardmodels.DefaultOutboundTag
	}
	if !containsString(options, preferred) {
		if model.TemplateData != nil && model.TemplateData.DefaultFinal != "" && containsString(options, model.TemplateData.DefaultFinal) {
			preferred = model.TemplateData.DefaultFinal
		} else if containsString(options, wizardmodels.DefaultOutboundTag) {
			preferred = wizardmodels.DefaultOutboundTag
		} else {
			preferred = options[0]
		}
	}
	model.SelectedFinalOutbound = preferred
}

// containsString проверяет, содержит ли слайс строк целевую строку.
func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
