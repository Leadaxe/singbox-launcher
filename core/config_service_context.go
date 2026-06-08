package core

// config_service_context.go — SPEC 070 split из config_service.go (pure move).
// BuildContext-assembly хелперы: сборка build.BuildContext из state + cache +
// template, плюс конвертеры DNS/Route из state в build-структуры.
//
// Инвариант Preset.ExecDir сохранён ровно как был в config_service.go:
// execDir резолвится один раз под nil-guard и кормит и Route.ExecDir, и
// PresetMergeContext.ExecDir / SrsCachedPaths.

import (
	"encoding/json"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
)

// buildContextFromState собирает BuildContext из state + cache + template.
// Если state nil (legacy fallback) — DNS/Route остаются пустыми, шаблонные
// дефолты используются как есть.
//
// Параметр parserConfig оставлен в сигнатуре для backward-compat callsite'ов;
// в SPEC 045 cleanup'е поле `BuildContext.ParserConfigJSON` удалено вместе
// с блоком `@ParserConfig` в config.json. Аргумент игнорируется.
func (ac *AppController) buildContextFromState(s *state.State, cache *build.ParsedCache, td *template.TemplateData, _ *config.ParserConfig) build.BuildContext {
	ctx := build.BuildContext{
		Template:   td,
		Cache:      cache,
		ForPreview: false, // Update path = save mode (full inline rendering, no truncation)
	}

	if s == nil {
		// Legacy fallback — vars из template defaults (применятся в GetEffectiveConfig).
		return ctx
	}

	// State есть: vars + DNS + Route.
	vars := make(map[string]string, len(s.Vars))
	for _, v := range s.Vars {
		vars[v.Name] = v.Value
	}
	// Materialize clash_secret если template объявляет его и в vars нет.
	build.MaterializeSecretsInVars(td, vars)
	ctx.Vars = vars

	// DNS scalars из state (могут жить в DNSOptions или vars; см. dnsConfigFromUpdate).
	ctx.DNS = dnsConfigForUpdate(s)
	ctx.Route = routeConfigForUpdate(s)
	// SPEC 045 фаза 9: execDir нужен MergeRouteSection-у для резолва путей
	// SRS файлов (bin/rule-sets/<tag>.srs). Без этого convertRuleSetToLocalRequired
	// не может проверить наличие файла и валит build с «empty execDir».
	// execDir feeds both Route (SRS path resolution) and Preset (cached SRS
	// lookup). Resolve once under a nil-guard — the SrsCachedPaths line below
	// used to deref ac.FileService unconditionally (SA5011: nil-deref when ac
	// or FileService is nil).
	var execDir string
	if ac != nil && ac.FileService != nil {
		execDir = ac.FileService.ExecDir
	}
	ctx.Route.ExecDir = execDir
	// SPEC 053: preset bundle merge — все правила из state.Rules в порядке.
	// Если state.Rules не пуст, MergePresetsIntoRoute берёт на себя весь emit
	// (preset/inline/srs). Noop когда RulesV6 пуст (legacy v5-only flow).
	ctx.Preset = build.PresetMergeContext{
		Presets:             td.Presets,
		Rules:               s.Rules,
		DNS:                 s.DNS,
		SrsCachedPaths:      build.CollectSrsCachedPaths(s.Rules, execDir),
		TemplateDNSDefaults: parseTemplateDNSDefaultsFromTD(td),
		ExecDir:             execDir,
	}
	return ctx
}

// parseTemplateDNSDefaultsFromTD — извлекает dns_options.servers[] из template
// и парсит в []build.TemplateDNSServer. Используется MergePresetsIntoDNS для
// материализации DNS-библиотеки (без этого юзерский DNS tab override на
// cloudflare_udp/google_doh/yandex_doh ничего не делает — server не в config).
//
// Возвращает nil если td nil / нет dns_options / парс не удался.
func parseTemplateDNSDefaultsFromTD(td *template.TemplateData) []build.TemplateDNSServer {
	if td == nil || len(td.DNSOptionsRaw) == 0 {
		return nil
	}
	var dnsOpt struct {
		Servers []json.RawMessage `json:"servers"`
	}
	if err := json.Unmarshal(td.DNSOptionsRaw, &dnsOpt); err != nil {
		return nil
	}
	parsed := build.ParseTemplateDNSDefaults(dnsOpt.Servers)
	// Validation warnings — non-fatal; logged для debug.
	for _, w := range build.ValidateTemplateDNSServers(parsed) {
		debuglog.WarnLog("template validation: %s", w)
	}
	return parsed
}

// dnsConfigForUpdate — извлекает DNS-related данные из state в build.DNSConfig.
//
// Schema distinction:
//   - v6 state — `state.DNS` (state.DNSOptions) — flat servers[]/rules[] через
//     kind discriminator. Servers/Rules эмитятся через `ctx.Preset.DNS`
//     в MergePresetsIntoDNS. Здесь читаем только scalars (Final/Strategy)
//     — но они в state живут в Vars[].
//   - pure-v5 state — DNSOptions единственный источник данных, читаем
//     cfg.Servers/RulesText.
//
// v6 active iff len(s.Rules) > 0 OR len(s.DNS.Servers/Rules) > 0.
//
// dns_* scalars из state.Vars[] всегда побеждают (SPEC 056-R-N: единый
// KV-store для всех wizard vars, включая dns_*).
//
// SPEC: independent_cache УДАЛЕНО — deprecated в sing-box 1.14.0; legacy
// state.Vars[dns_independent_cache] и DNSOptions.IndependentCache игнорируются.
func dnsConfigForUpdate(s *state.State) build.DNSConfig {
	cfg := build.DNSConfig{}

	v6Active := s != nil &&
		(len(s.Rules) > 0 ||
			len(s.DNS.Servers) > 0 ||
			len(s.DNS.Rules) > 0)

	if v6Active {
		// v6 path: scalars из DNSV6; servers/rules идут через ctx.Preset.DNS.
		cfg.Final = s.DNS.Final
		cfg.Strategy = s.DNS.Strategy
	} else if s.DNSOptions != nil {
		// pure-v5 path
		cfg.Final = s.DNSOptions.Final
		cfg.Strategy = s.DNSOptions.Strategy
		cfg.Servers = s.DNSOptions.Servers
		if len(s.DNSOptions.Rules) > 0 {
			raw, err := json.Marshal(map[string]interface{}{"rules": s.DNSOptions.Rules})
			if err == nil {
				cfg.RulesText = string(raw)
			}
		}
	}

	// dns_* scalars из vars[] (источник истины; SPEC 032 + SPEC 056-R-N).
	for _, v := range s.Vars {
		switch v.Name {
		case "dns_final":
			cfg.Final = v.Value
		case "dns_strategy":
			cfg.Strategy = v.Value
		}
	}
	return cfg
}

// routeConfigForUpdate — конвертит state.CustomRules в build.RouteConfig.
//
// SPEC 053: если state.Rules содержит правила — legacy CustomRules emit
// **скипается** (RouteConfig.Rules = nil). Все правила (preset/inline/srs)
// эмитятся через MergePresetsIntoRoute в правильном порядке из state.Rules.
// Это избегает double-emit (правило не появится дважды в route.rules[]).
//
// `route.final` НЕ читается здесь: он подставляется на этапе
// template-substitution через `@route_final` (state.vars["route_final"] →
// template substituter → финальный config.json). MergeRouteSection видит
// пустой FinalOutbound и оставляет уже-substituted шаблонное значение.
func routeConfigForUpdate(s *state.State) build.RouteConfig {
	if len(s.Rules) > 0 {
		// v6 path: rules эмитятся через MergePresetsIntoRoute в правильном порядке.
		return build.RouteConfig{}
	}
	rules := make([]build.RouteRule, 0, len(s.CustomRules))
	for _, cr := range s.CustomRules {
		outbound := cr.SelectedOutbound
		if outbound == "" {
			outbound = cr.DefaultOutbound
		}
		rules = append(rules, build.RouteRule{
			Enabled:     cr.Enabled,
			Outbound:    outbound,
			PrimaryRule: cr.Rule,
			RuleSets:    cr.RuleSet,
		})
	}
	return build.RouteConfig{
		Rules: rules,
	}
}
