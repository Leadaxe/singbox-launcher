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

	"singbox-launcher/core/config"
	wizardtemplate "singbox-launcher/core/template"
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
		// Add local outbounds from all ProxySource.
		// Skip disabled subscriptions — UI dropdown должен показывать только теги,
		// которые реально попадут в финальный config.json (build pipeline тоже
		// пропускает disabled подписки). Иначе юзер может выбрать "BL:select"
		// от отключённой подписки → dangling outbound в emit.
		for _, proxySource := range parserCfg.ParserConfig.Proxies {
			if proxySource.Disabled {
				continue
			}
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

	// SPEC 056: добавляем теги от preset.outbounds[] mode=add активных
	// preset-ref'ов (mode=update не вводит новых тегов, только патчит
	// существующие). Без этого UI Rules tab не предложит "ru VPN 🇷🇺" из
	// ru-inside, и пользователь не сможет выбрать его в своих правилах.
	//
	// Bypass memo: preset-refs меняются независимо от ParserConfigJSON, и
	// мемо по jsonKey может прокэшировать stale set. На UI стороне это
	// дёшево (несколько preset'ов, без I/O).
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

// collectActivePresetOutboundTags возвращает outbound-теги от mode="add"
// entries активных (Enabled) preset-ref'ов в model.PresetRefs.
//
// Семантика (SPEC 056):
//   - Только mode="" (default add) и mode="add" вводят новые tag'и;
//     mode="update" патчит existing — не возвращает.
//   - Per-entry if/if_or фильтруется по varsMap (user override + preset.vars[].Default).
//   - @var в Tag-поле резолвится (rare, обычно tag — литерал).
//   - wizard.hide=true → tag НЕ показывается в picker'е (consistent с
//     OutboundConfig.IsWizardHidden() для template-defined outbound'ов).
//
// Дедуп делает caller (sortedOutboundTagSlice). Возвращает nil если нет
// active preset-refs или ни один не имеет preset.outbounds[].
func collectActivePresetOutboundTags(model *wizardmodels.WizardModel) []string {
	if model == nil || model.TemplateData == nil || len(model.PresetRefs) == 0 {
		return nil
	}
	presetByID := make(map[string]*wizardtemplate.Preset, len(model.TemplateData.Presets))
	for i := range model.TemplateData.Presets {
		presetByID[model.TemplateData.Presets[i].ID] = &model.TemplateData.Presets[i]
	}

	var out []string
	for _, ref := range model.PresetRefs {
		if ref == nil || !ref.Enabled {
			continue
		}
		preset, ok := presetByID[ref.Ref]
		if !ok || len(preset.Outbounds) == 0 {
			continue
		}

		// Build varsMap: user override или preset.vars[].Default.
		varsMap := make(map[string]string, len(preset.Vars))
		for _, v := range preset.Vars {
			if val, has := ref.Vars[v.Name]; has && val != "" {
				varsMap[v.Name] = val
			} else {
				varsMap[v.Name] = v.Default
			}
		}

		for _, ob := range preset.Outbounds {
			mode := ob.Mode
			if mode == "" {
				mode = "add"
			}
			if mode != "add" {
				continue
			}
			if !evalPresetOutboundIf(ob.If, ob.IfOr, varsMap) {
				continue
			}
			if isPresetOutboundHidden(ob.Wizard) {
				continue
			}
			tag := ob.Tag
			if strings.HasPrefix(tag, "@") {
				if val, has := varsMap[tag[1:]]; has {
					tag = val
				}
			}
			if tag != "" {
				out = append(out, tag)
			}
		}
	}
	return out
}

// evalPresetOutboundIf — true iff ВСЕ if истинны И (if_or пуст ИЛИ хотя бы один if_or истинен).
// "Истинно" = varsMap[name] == "true" (case-insensitive). Зеркало семантики
// core/build.evalIf, продублировано чтобы UI не импортировал внутренний build.
func evalPresetOutboundIf(ifList, ifOrList []string, varsMap map[string]string) bool {
	for _, name := range ifList {
		if !strings.EqualFold(varsMap[name], "true") {
			return false
		}
	}
	if len(ifOrList) > 0 {
		anyTrue := false
		for _, name := range ifOrList {
			if strings.EqualFold(varsMap[name], "true") {
				anyTrue = true
				break
			}
		}
		if !anyTrue {
			return false
		}
	}
	return true
}

// isPresetOutboundHidden — true если preset.outbound.wizard указывает hide.
// Зеркало OutboundConfig.IsWizardHidden() — поддерживает обе формы
// ("hide" string-shorthand и {"hide":true} map).
func isPresetOutboundHidden(wizard interface{}) bool {
	if wizard == nil {
		return false
	}
	if s, ok := wizard.(string); ok {
		return s == "hide"
	}
	if m, ok := wizard.(map[string]interface{}); ok {
		if hideVal, has := m["hide"]; has {
			if b, ok := hideVal.(bool); ok {
				return b
			}
		}
	}
	return false
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
