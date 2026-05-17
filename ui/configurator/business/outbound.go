// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл outbound.go содержит функции для работы с outbounds:
//   - GetAvailableOutbounds - список доступных outbound тегов (ParserConfig или JSON); мемо по trimmed ParserConfigJSON при ParserConfig == nil
//   - EnsureDefaultAvailableOutbounds - обеспечивает наличие обязательных outbounds (direct-out, reject, drop)
//   - EnsureFinalSelected - обеспечивает выбранный final outbound в модели
//
// Эти функции работают с WizardModel (чистыми данными), без зависимостей от GUI.
// Используются в презентере при обновлении опций outbound для правил маршрутизации.
//
// Используется в:
//   - presentation/presenter_methods.go - RefreshOutboundOptions вызывает GetAvailableOutbounds и EnsureFinalSelected
//   - business/create_config.go - GetAvailableOutbounds используется при генерации конфигурации
package business

import (
	"encoding/json"
	"sort"
	"strings"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// GetAvailableOutbounds возвращает список доступных outbound тегов из модели.
// При model.ParserConfig == nil и непустом ParserConfigJSON результат кэшируется по строке JSON (сброс — InvalidatePreviewCache).
func GetAvailableOutbounds(model *wizardmodels.WizardModel) []string {
	tags := map[string]struct{}{
		wizardmodels.DefaultOutboundTag: {},
		wizardmodels.RejectActionName:   {},
		"drop":                          {}, // Always include "drop" in available options
	}

	if model == nil {
		return sortedOutboundTagSlice(tags)
	}

	jsonKey := strings.TrimSpace(model.ParserConfigJSON)
	if model.ParserConfig == nil && jsonKey != "" {
		if model.AvailableOutboundsMemoKey == jsonKey && len(model.AvailableOutboundsMemoTags) > 0 {
			out := make([]string, len(model.AvailableOutboundsMemoTags))
			copy(out, model.AvailableOutboundsMemoTags)
			return out
		}
	} else if model.ParserConfig != nil {
		model.AvailableOutboundsMemoKey = ""
		model.AvailableOutboundsMemoTags = nil
	}

	var parserCfg *config.ParserConfig
	if model.ParserConfig != nil {
		parserCfg = model.ParserConfig
	} else if jsonKey != "" {
		var parsed config.ParserConfig
		if err := json.Unmarshal([]byte(model.ParserConfigJSON), &parsed); err == nil {
			parserCfg = &parsed
		}
		// Note: silently ignore parse errors - ParserConfigJSON might be invalid or incomplete
		// This is expected behavior when user is typing ParserConfig
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

	// SPEC 055: добавляем preset-emitted outbound tags (mode=add) от active
	// preset-ref'ов. Mode=update не вводит новых тегов — только патчит существующие.
	for _, tag := range collectActivePresetOutboundTags(model) {
		tags[tag] = struct{}{}
	}

	result := sortedOutboundTagSlice(tags)
	if model.ParserConfig == nil && jsonKey != "" {
		model.AvailableOutboundsMemoKey = jsonKey
		model.AvailableOutboundsMemoTags = append([]string(nil), result...)
	}
	return result
}

// collectActivePresetOutboundTags — SPEC 055. Возвращает теги outbound'ов
// которые **активные** preset-ref'ы добавляют (mode=add) в финальный config.
// mode=update пропускается (он только патчит существующие, не добавляет тегов).
//
// Используется для UI outbound-picker'ов: юзер должен видеть `ru VPN 🇷🇺`
// в dropdown'е только когда `ru-inside` preset enabled.
//
// Disabled preset-refs пропускаются. Сами теги НЕ префиксуются preset_id —
// они user-facing (см. ExpandedOutbound.Tag в core/build/preset_expand.go).
func collectActivePresetOutboundTags(model *wizardmodels.WizardModel) []string {
	if model == nil || model.TemplateData == nil || len(model.PresetRefs) == 0 {
		return nil
	}
	presetByID := make(map[string]int, len(model.TemplateData.Presets))
	for i := range model.TemplateData.Presets {
		presetByID[model.TemplateData.Presets[i].ID] = i
	}
	seen := make(map[string]bool)
	var out []string
	for _, pr := range model.PresetRefs {
		if pr == nil || !pr.Enabled || pr.Ref == "" {
			continue
		}
		idx, ok := presetByID[pr.Ref]
		if !ok {
			continue
		}
		tpl := &model.TemplateData.Presets[idx]
		if len(tpl.Outbounds) == 0 {
			continue
		}
		frags, _, ok := build.ExpandPreset(tpl, pr.Vars)
		if !ok {
			continue
		}
		for _, ob := range frags.Outbounds {
			if ob.Mode != "add" || ob.Tag == "" || seen[ob.Tag] {
				continue
			}
			seen[ob.Tag] = true
			out = append(out, ob.Tag)
		}
	}
	return out
}

func sortedOutboundTagSlice(tags map[string]struct{}) []string {
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
