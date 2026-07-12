package presentation

import (
	"encoding/json"
	"strings"

	"singbox-launcher/core"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// extractConfigParams извлекает параметры конфигурации из модели.
//
// Сейчас config_params не используется ни для одного параметра в новых
// state.json: route.final мигрирован в state.vars["route_final"] (template
// делает substitution через @route_final), DNS-параметры — в state.vars
// тоже (dns_default_domain_resolver и т.п.).
//
// Поле остаётся в state schema для backward-compat чтения старых state.json
// (см. restoreConfigParams) и для возможных будущих параметров, которые не
// уложатся в template-vars модель.
func (p *WizardPresenter) extractConfigParams() []wizardmodels.ConfigParam {
	return []wizardmodels.ConfigParam{}
}

// restoreParserConfig — SPEC 052 phase 8: переносит state.Connections в
// model.{Sources,GlobalOutbounds,Defaults} (canonical) и обновляет derived
// `ParserConfig`/`ParserConfigJSON` для UI/parser callsite'ов.
func (p *WizardPresenter) restoreParserConfig(stateFile *wizardmodels.WizardStateFile) error {
	// Sources canonical из v5 Connections.
	p.model.Sources = append([]wizardmodels.Source(nil), stateFile.Connections.Sources...)
	p.model.GlobalOutbounds = append([]configtypes.OutboundConfig(nil), stateFile.Connections.Outbounds...)
	p.model.Defaults = stateFile.Connections.Defaults

	// Validate: на свежей миграции должна быть хотя бы пустая slice.
	if p.model.Sources == nil {
		p.model.Sources = []wizardmodels.Source{}
	}
	if p.model.GlobalOutbounds == nil {
		p.model.GlobalOutbounds = []configtypes.OutboundConfig{}
	}

	// Heal-on-empty: state.json мог быть сохранён (баг до v0.9.0.1) с пустыми
	// connections.outbounds — Rebuild сгенерирует config.json без proxy-out /
	// auto-proxy-out селекторов, sing-box упадёт FATAL "default outbound not
	// found: proxy-out". Если state пустой, заполняем из template (как делает
	// loader.go LoadConfigFromFile на cold-start). Не трогаем не-пустой
	// state — пользователь мог явно отредактировать список.
	if len(p.model.GlobalOutbounds) == 0 && p.model.TemplateData != nil && p.model.TemplateData.ParserConfig != "" {
		var parsed configtypes.ParserConfig
		if err := json.Unmarshal([]byte(p.model.TemplateData.ParserConfig), &parsed); err != nil {
			debuglog.WarnLog("restoreParserConfig: heal-empty: failed to parse template parser_config: %v", err)
		} else if len(parsed.ParserConfig.Outbounds) > 0 {
			p.model.GlobalOutbounds = append([]configtypes.OutboundConfig(nil), parsed.ParserConfig.Outbounds...)
			debuglog.InfoLog("restoreParserConfig: heal-empty: seeded %d global outbounds from template (state had empty connections.outbounds)", len(p.model.GlobalOutbounds))
		}
	}

	// Refresh derived view (ParserConfig + ParserConfigJSON) для UI.
	p.model.RefreshDerivedParserConfig()
	wizardbusiness.InvalidatePreviewCache(p.model)
	return nil
}

// restoreCustomRules восстанавливает CustomRules из состояния (шаг 6).
func (p *WizardPresenter) restoreCustomRules(persistedRules []wizardmodels.PersistedCustomRule) {
	p.model.CustomRules = make([]*wizardmodels.RuleState, 0, len(persistedRules))
	for i := range persistedRules {
		ruleState := wizardmodels.PersistedCustomRuleToRuleState(&persistedRules[i])
		p.model.CustomRules = append(p.model.CustomRules, ruleState)
	}
}

// restorePresetRefs (SPEC 053) — восстанавливает model.PresetRefs из state.Rules
// и заполняет model.RuleOrder в порядке state.Rules (так чтобы UI Rules tab
// после load показал правила в том же порядке как при save).
//
// Только kind=preset entries попадают в PresetRefs; kind=inline/srs остаются
// в CustomRules через restoreCustomRules + legacy view (см. parseV6).
func (p *WizardPresenter) restorePresetRefs(state *wizardmodels.WizardStateFile) {
	p.model.PresetRefs = wizardmodels.SyncStateRulesToPresetRefs(state.Rules)
	p.model.DNSTemplateOverrides = wizardmodels.SyncStateV6ToDNSOverrides(state.DNS)
	// SPEC 056-R-N follow-up: per-server/rule preset enabled overrides → PresetRefState fields.
	populatePresetEnabledFromState(p.model.PresetRefs, state.DNS)

	// SPEC 093: авто-сид DNS-only пресетов (fakeip) как постоянных DNS-правил.
	// ДО построения RuleOrder/DNSRuleOrder — Reconcile* ниже подхватят slot'ы.
	// Идемпотентно: существующий (в т.ч. выключенный юзером) ref не трогается.
	wizardmodels.EnsureDNSOnlyPresetsSeeded(p.model)

	// Restore RuleOrder из state.Rules (preserve порядок between save/load).
	// Fallback на дефолтную последовательность если state v5 (нет RulesV6).
	order := wizardmodels.RuleOrderFromStateRulesV6(state.Rules, p.model.PresetRefs, p.model.CustomRules)
	if len(order) == 0 {
		wizardmodels.RebuildRuleOrder(p.model)
	} else {
		p.model.RuleOrder = order
		// Reconcile в случае если в model.CustomRules / PresetRefs есть entries
		// которые не попали в order (могут быть после миграции v5→v6).
		wizardmodels.ReconcileRuleOrder(p.model)
	}

	// SPEC 062-F-N: restore DNSRuleOrder + DNSUserRules from state.DNS.Rules.
	// PresetRefs уже выставлены выше — DNSRuleOrderFromStateRules может
	// маппить kind=preset refs → PresetRefs[Index].
	//
	// На legacy state (state.DNS пуст или ещё нет SPEC 062 порядка) — restoreDNS
	// уже заполнил model.DNSRulesText через populateUserDNSFromState; парсим
	// его в DNSUserRules + RebuildDNSRuleOrder для дефолтного порядка.
	dnsOrder, dnsUserRules := wizardmodels.DNSRuleOrderFromStateRules(state.DNS.Rules, p.model.PresetRefs)
	if len(dnsOrder) > 0 {
		p.model.DNSRuleOrder = dnsOrder
		p.model.DNSUserRules = dnsUserRules
		wizardmodels.ReconcileDNSRuleOrder(p.model)
	} else {
		// Legacy / empty: build DNSUserRules from DNSRulesText (populateUserDNSFromState
		// уже его заполнил), then RebuildDNSRuleOrder для дефолтного user-then-preset.
		p.model.DNSUserRules = wizardmodels.DNSUserRulesFromText(p.model.DNSRulesText)
		wizardmodels.RebuildDNSRuleOrder(p.model)
	}
}

// restoreConfigParams восстанавливает config_params и vars в модель.
//
// route.final ищется в порядке приоритета:
//  1. state.vars["route_final"] (canonical, новые state.json)
//  2. state.config_params["route.final"] (legacy, для миграции v0.9.3-)
//  3. template default (fallback, чистый старт)
//
// Это автоматически мигрирует старые state.json: при следующем Save запись
// уйдёт в state.vars, а config_params останется чистым.
func (p *WizardPresenter) restoreConfigParams(stateFile *wizardmodels.WizardStateFile) {
	allowed := make(map[string]struct{})
	if p.model.TemplateData != nil {
		for _, vd := range p.model.TemplateData.Vars {
			if vd.Separator {
				continue
			}
			allowed[vd.Name] = struct{}{}
		}
	}
	p.model.SettingsVars = make(map[string]string)
	for _, x := range stateFile.Vars {
		if _, ok := allowed[x.Name]; !ok {
			continue // сироты: имя не из текущего шаблона (SPEC 032)
		}
		p.model.SettingsVars[x.Name] = x.Value // при дубликатах name в массиве JSON — последняя запись выигрывает
	}

	// route.final: vars[route_final] → legacy config_params[route.final] → template default.
	if v, ok := p.model.SettingsVars["route_final"]; ok && v != "" {
		p.model.SelectedFinalOutbound = v
	} else if legacy := p.findConfigParamValue(stateFile.ConfigParams, "route.final"); legacy != "" {
		p.model.SelectedFinalOutbound = legacy
		// Eager-write into vars so the next save persists in canonical channel
		// and the legacy config_params entry can be dropped.
		p.model.SettingsVars["route_final"] = legacy
	} else {
		p.model.SelectedFinalOutbound = p.getDefaultFinalOutbound()
	}
}

// restoreDNS loads dns_options from state (if any) and merges with the current wizard_template.json.
func (p *WizardPresenter) restoreDNS(sf *wizardmodels.WizardStateFile) {
	if sf == nil {
		return
	}
	if p.model.TemplateData != nil {
		wizardbusiness.MigrateDNSScalarsFromPersistedToSettingsVars(sf.DNSOptions, p.model.SettingsVars, p.model.TemplateData.Vars)
	}
	if sf.DNSOptions != nil && sf.DNSOptions.ResolverUnset {
		p.model.DefaultDomainResolverUnset = true
		p.model.DefaultDomainResolver = ""
	}
	if sf.DNSOptions != nil {
		wizardbusiness.LoadPersistedWizardDNS(p.model, sf.DNSOptions)
	}
	// SPEC 056 phase 7 regression fix: для v6-файлов sf.DNSOptions == nil,
	// поэтому user-added DNS-сервера и user DNS rules (kind=user в sf.DNS)
	// никем не восстанавливались — populateUserDNSFromState закрывает дыру.
	// Идемпотентно: дедуп по tag, DNSRulesText трогаем только если пуст.
	populateUserDNSFromState(p.model, sf.DNS)
	// Старые state.json: тег только в config_params (до dns_* vars).
	if !p.model.DefaultDomainResolverUnset && strings.TrimSpace(p.model.DefaultDomainResolver) == "" {
		if dr := p.findConfigParamValue(sf.ConfigParams, "route.default_domain_resolver"); dr != "" {
			p.model.DefaultDomainResolver = dr
			p.model.DefaultDomainResolverUnset = false
		}
	}
	wizardbusiness.ApplyWizardDNSTemplate(p.model)
	// Overlay saved enabled state for ALL servers (incl. template) from canonical
	// sf.DNS — populateUserDNSFromState only handled kind=user, so template-server
	// enable/disable overrides were otherwise lost on reopen (and reset the selects).
	applyDNSServerEnabledFromState(p.model, sf.DNS)
	wizardbusiness.ApplyDNSVarsFromSettingsToModel(p.model)
}

// findConfigParamValue ищет значение параметра по имени.
// Возвращает пустую строку, если параметр не найден.
func (p *WizardPresenter) findConfigParamValue(configParams []wizardmodels.ConfigParam, name string) string {
	for _, param := range configParams {
		if param.Name == name {
			return param.Value
		}
	}
	return ""
}

// getDefaultFinalOutbound возвращает значение по умолчанию для final outbound из шаблона.
func (p *WizardPresenter) getDefaultFinalOutbound() string {
	if p.model.TemplateData != nil && p.model.TemplateData.DefaultFinal != "" {
		return p.model.TemplateData.DefaultFinal
	}
	return ""
}

// GetStateStore создает новый StateStore для работы с состояниями.
func (p *WizardPresenter) GetStateStore() *wizardbusiness.StateStore {
	ac := core.GetController()
	fileServiceAdapter := &wizardbusiness.FileServiceAdapter{FileService: ac.FileService}
	return wizardbusiness.NewStateStore(fileServiceAdapter)
}
