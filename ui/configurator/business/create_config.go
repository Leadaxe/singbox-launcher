// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл create_config.go генерирует финальную конфигурацию sing-box из единого шаблона и модели визарда.
//
// BuildTemplateConfig собирает конфигурацию:
//  1. Нормализует ParserConfig (версия, last_updated)
//  2. Для каждой секции config из шаблона:
//     - outbounds: вставляет сгенерированные outbounds перед статическими
//     - route: база из шаблона (статические rules/rule_set, final из шаблона), затем правила и rule_set из custom_rules модели
//     - остальные секции: форматирует как есть
//  3. Оборачивает всё в JSONC с блоком @ParserConfig
//
// Используется в:
//   - presenter_save.go — для генерации конфигурации при сохранении
//   - presenter_async.go — для генерации preview конфигурации
package business

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/build"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// MaterializeSecretsIfNeeded гарантирует SettingsVars непустую map'у и
// делегирует материализацию всех type:"secret" var в `core/build`. Тонкая
// обёртка для двух callsites — preview build + EffectiveConfigSection.
func MaterializeSecretsIfNeeded(model *wizardmodels.WizardModel) {
	if model == nil {
		return
	}
	if model.SettingsVars == nil {
		model.SettingsVars = make(map[string]string)
	}
	build.MaterializeSecretsInVars(model.TemplateData, model.SettingsVars)
}

// BuildPreviewConfig собирает config.json для preview-вкладки визарда.
//
// Wizard-only API: конвертит `WizardModel` → `build.BuildContext` и зовёт
// `core/build.BuildConfig` с `ForPreview=true` (включает preview-truncation
// для больших подписок: >30 нод рендерится сводным комментарием вместо inline).
//
// Единственный продуктивный callsite — `presenter_async.UpdateTemplatePreviewAsync`.
// Save / Update / Restart pre-rebuild идут напрямую через `core/build.BuildConfig`
// без участия этой функции (см. SPEC 045 фазы 5.A/5.B/5.C).
func BuildPreviewConfig(model *wizardmodels.WizardModel) (string, error) {
	if model == nil || model.TemplateData == nil {
		return "", fmt.Errorf("template data not available")
	}

	// Mutates model.SettingsVars: материализует dns_* + секреты.
	SyncDNSModelToSettingsVars(model)
	MaterializeSecretsIfNeeded(model)

	if strings.TrimSpace(model.ParserConfigJSON) == "" {
		return "", fmt.Errorf("ParserConfig is empty and no template available")
	}

	ctx := build.BuildContext{
		Template: model.TemplateData,
		Vars:     model.SettingsVars,
		Cache:    inMemoryCacheFromModel(model),
		Stats: build.PreviewStats{
			NodesCount:           model.OutboundStats.NodesCount,
			LocalSelectorsCount:  model.OutboundStats.LocalSelectorsCount,
			GlobalSelectorsCount: model.OutboundStats.GlobalSelectorsCount,
			EndpointsCount:       model.OutboundStats.EndpointsCount,
		},
		ForPreview: true,
		DNS: build.DNSConfig{
			Servers: model.DNSServers,
			// SPEC 062: user DNS rules now flow through ctx.Preset.DNS.Rules
			// (populated below via SyncDNSByOrderToState) so they emit in
			// DNSRuleOrder, NOT before preset rules. The legacy RulesText
			// path of MergeDNSSection would double-emit them (once here,
			// once from MergePresetsIntoDNS) and pin user rules to the top
			// regardless of the user's drag order. Mirrors what the
			// Save→rebuild path does via core/config_service.go::dnsConfigForUpdate
			// — that one also leaves RulesText empty when v6 state is active.
			Final:    model.DNSFinal,
			Strategy: model.DNSStrategy,
			// SPEC: IndependentCache removed (sing-box 1.14 deprecation).
		},
		Route: routeConfigFromModel(model),
	}

	// SPEC 053/055: Preview tab must show the SAME config that Save/Apply
	// would produce — preset-refs (Block Ads, Russian, etc) need to appear
	// in route.rules. Previously ctx.Preset was zero-value → presets ignored
	// in preview → user thought config was broken.
	// Reconcile model.RuleOrder → v6.Rule[] via same Sync helpers that
	// CreateStateFromModel uses on Save.
	wizardmodels.ReconcileRuleOrder(model)
	rulesV6 := wizardmodels.SyncRulesByOrderToStateRulesV6(
		model.RuleOrder, model.PresetRefs, model.CustomRules,
	)
	templateDNSTags := ExtractTemplateDNSTags(model.TemplateData)
	// SPEC 062-F-N: same order-aware DNS sync as CreateStateFromModel so
	// preview matches what Save would emit (preset + user rules interleaved
	// per DNSRuleOrder).
	wizardmodels.ReconcileDNSRuleOrder(model)
	dnsV6 := wizardmodels.SyncDNSByOrderToState(
		model.DNSRuleOrder,
		model.PresetRefs,
		model.DNSUserRules,
		model.DNSServers,
		model.DNSRulesText,
		model.DNSTemplateOverrides,
		templateDNSTags,
	)
	ctx.Preset = build.PresetMergeContext{
		Presets:             model.TemplateData.Presets,
		Rules:               rulesV6,
		DNS:                 dnsV6,
		SrsCachedPaths:      build.CollectSrsCachedPaths(rulesV6, model.ExecDir),
		ExecDir:             model.ExecDir,
		TemplateDNSDefaults: ParseTemplateDNSDefaults(model.TemplateData),
	}

	res, err := build.BuildConfig(ctx)
	if err != nil {
		return "", err
	}
	return string(res.ConfigJSON), nil
}

// inMemoryCacheFromModel конвертит model.GeneratedOutbounds/.GeneratedEndpoints
// (legacy []string format с `\t`-префиксом и trailing `,`) в build.ParsedCache.
// Используется только preview-путём (Save не строит config из этих полей).
func inMemoryCacheFromModel(model *wizardmodels.WizardModel) *build.ParsedCache {
	pc := &build.ParsedCache{}
	for _, s := range model.GeneratedOutbounds {
		cleaned := strings.TrimSpace(strings.TrimRight(s, ",\n\r\t "))
		if cleaned == "" {
			continue
		}
		pc.Outbounds = append(pc.Outbounds, json.RawMessage(cleaned))
	}
	for _, s := range model.GeneratedEndpoints {
		cleaned := strings.TrimSpace(strings.TrimRight(s, ",\n\r\t "))
		if cleaned == "" {
			continue
		}
		pc.Endpoints = append(pc.Endpoints, json.RawMessage(cleaned))
	}
	return pc
}

// routeConfigFromModel — конвертит CustomRules WizardModel в build.RouteConfig.
func routeConfigFromModel(model *wizardmodels.WizardModel) build.RouteConfig {
	rules := make([]build.RouteRule, 0, len(model.CustomRules))
	for _, rs := range model.CustomRules {
		outbound := wizardmodels.GetEffectiveOutbound(rs)
		rules = append(rules, build.RouteRule{
			Enabled:     rs.Enabled,
			Outbound:    outbound,
			PrimaryRule: rs.Rule.Rule,
			Rules:       rs.Rule.Rules,
			RuleSets:    rs.Rule.RuleSets,
		})
	}
	return build.RouteConfig{
		Rules:                     rules,
		FinalOutbound:             model.SelectedFinalOutbound,
		ExecDir:                   model.ExecDir,
		DefaultDomainResolver:     model.DefaultDomainResolver,
		OmitDefaultDomainResolver: model.DefaultDomainResolverUnset,
	}
}

// EffectiveConfigSection returns merged template JSON for one top-level
// config key (e.g. "experimental"). Используется UI-кодом, которому нужно
// прочитать конкретную секцию шаблона с применёнными vars (но без
// перестроения всего config'а через BuildConfig).
func EffectiveConfigSection(model *wizardmodels.WizardModel, sectionKey string) (json.RawMessage, bool, error) {
	if model == nil || model.TemplateData == nil {
		return nil, false, fmt.Errorf("no template data")
	}
	MaterializeSecretsIfNeeded(model)
	// secrets уже материализованы выше — effectiveTemplate(..., false).
	config, _ := effectiveTemplate(model, false)
	raw, ok := config[sectionKey]
	return raw, ok, nil
}
