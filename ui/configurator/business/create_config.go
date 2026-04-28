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
	"runtime"
	"strings"

	"singbox-launcher/core/build"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardtemplate "singbox-launcher/core/template"
)

// MaterializeClashSecretIfNeeded гарантирует SettingsVars непустую map'у и
// делегирует материализацию clash_secret в `core/build`. Тонкая обёртка для
// двух callsites — preview build + EffectiveConfigSection.
func MaterializeClashSecretIfNeeded(model *wizardmodels.WizardModel) {
	if model == nil {
		return
	}
	if model.SettingsVars == nil {
		model.SettingsVars = make(map[string]string)
	}
	build.MaterializeClashSecretInVars(model.TemplateData, model.SettingsVars)
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

	// Mutates model.SettingsVars: материализует dns_* + clash_secret.
	SyncDNSModelToSettingsVars(model)
	MaterializeClashSecretIfNeeded(model)

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
			Servers:          model.DNSServers,
			RulesText:        model.DNSRulesText,
			Final:            model.DNSFinal,
			Strategy:         model.DNSStrategy,
			IndependentCache: model.DNSIndependentCache,
		},
		Route: routeConfigFromModel(model),
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

// MergeRouteSection — back-compat шим над `build.MergeRouteSection` для
// существующих generator_test.go тестов wizard-side. Production-код больше
// эту функцию не использует — `core/build.MergeRouteSection` вызывается
// напрямую из orchestrator'а через `build.RouteConfig`.
//
// Deprecated: использовать `build.MergeRouteSection(raw, build.RouteConfig{...})` напрямую.
// Обёртка останется до тех пор, пока тесты не перенесены в `core/build`.
func MergeRouteSection(raw json.RawMessage, customRules []*wizardmodels.RuleState, finalOutbound string, execDir string, defaultDomainResolver string, omitDefaultDomainResolver bool) (json.RawMessage, error) {
	rules := make([]build.RouteRule, 0, len(customRules))
	for _, rs := range customRules {
		rules = append(rules, build.RouteRule{
			Enabled:     rs.Enabled,
			Outbound:    wizardmodels.GetEffectiveOutbound(rs),
			PrimaryRule: rs.Rule.Rule,
			Rules:       rs.Rule.Rules,
			RuleSets:    rs.Rule.RuleSets,
		})
	}
	return build.MergeRouteSection(raw, build.RouteConfig{
		Rules:                     rules,
		FinalOutbound:             finalOutbound,
		ExecDir:                   execDir,
		DefaultDomainResolver:     defaultDomainResolver,
		OmitDefaultDomainResolver: omitDefaultDomainResolver,
	})
}

// effectiveTemplateConfig returns the merged top-level config map (after
// applying params + substituting vars) and the key order. Used by
// `EffectiveConfigSection` для UI-операций, читающих конкретную секцию
// (например, `settings_tun_darwin.go` проверяет experimental.tun).
//
// На неудаче GetEffectiveConfig — fallback на td.Config / td.ConfigOrder
// (template defaults без подставленных vars).
func effectiveTemplateConfig(model *wizardmodels.WizardModel) (map[string]json.RawMessage, []string) {
	if model == nil || model.TemplateData == nil {
		return nil, nil
	}
	td := model.TemplateData
	config, order := td.Config, td.ConfigOrder
	if len(td.RawConfig) > 0 && (len(td.Params) > 0 || len(td.Vars) > 0) {
		effective, ord, err := wizardtemplate.GetEffectiveConfig(
			td.RawConfig,
			td.Params,
			runtime.GOOS,
			td.Vars,
			model.SettingsVars,
			td.RawTemplate,
		)
		if err == nil {
			return effective, ord
		}
		debuglog.WarnLog("effectiveTemplateConfig: GetEffectiveConfig: %v", err)
	}
	return config, order
}

// EffectiveConfigSection returns merged template JSON for one top-level
// config key (e.g. "experimental"). Используется UI-кодом, которому нужно
// прочитать конкретную секцию шаблона с применёнными vars (но без
// перестроения всего config'а через BuildConfig).
func EffectiveConfigSection(model *wizardmodels.WizardModel, sectionKey string) (json.RawMessage, bool, error) {
	if model == nil || model.TemplateData == nil {
		return nil, false, fmt.Errorf("no template data")
	}
	MaterializeClashSecretIfNeeded(model)
	config, _ := effectiveTemplateConfig(model)
	raw, ok := config[sectionKey]
	return raw, ok, nil
}
