package dialogs

// rule_dialog_preview.go — вкладка Preview окна правила: что правило даёт в
// конфиге (route.rule_set, route.rules, DNS-зеркала). Строится кодом сборки
// по оси модели целиком и тегам шаблона (SPEC 188 §6.13: теги наборов
// зависят от всех правил), в показ идёт только правило окна.

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/build"
	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// rulePreviewInput — состояние окна правила для вкладки Preview.
type rulePreviewInput struct {
	label    string
	outbound string
	// body — тело правила с целью (с вкладки JSON или из формы, `refs`
	// внутри); nil — правило не заполнено.
	body map[string]interface{}
	dns  *corestate.RuleDNS
	srs  corestate.RuleSetOptions // настройки наборов (SPEC 185)
	// editing — правило модели, которое правит окно (nil — новое). Теги
	// наборов зависят от всех правил оси и шаблона (SPEC 188, §6.13): превью
	// собирает ось целиком, подставив правило окна на место editing.
	editing *wizardmodels.RuleState
}

// rulePreviewSection — секция конфига в превью; ключи в порядке конфига.
type rulePreviewSection struct {
	RuleSet []interface{} `json:"rule_set,omitempty"`
	Rules   []interface{} `json:"rules,omitempty"`
}

// rulePreviewText — что правило даёт в конфиге: route.rule_set, route.rules и
// DNS-зеркала (SPEC 172), с пояснениями строками `//` над JSON — набор не
// скачан, сервер зеркала выключен или не найден, опция DNS не применима.
// Запись правила собирается так же, как при Save (EmitStateRulesWithoutOrder),
// маршрут и зеркала — кодом сборки.
func rulePreviewText(model *wizardmodels.WizardModel, in rulePreviewInput) string {
	body := in.body
	outbound := in.outbound
	if outbound == "" {
		outbound = wizardmodels.DefaultOutboundTag
	}
	rs := &wizardmodels.RuleState{
		Rule:             wizardtemplate.TemplateSelectableRule{Label: in.label, Rule: body, RuleSets: wizardmodels.RuleSetsFromBody(body), HasOutbound: true},
		Enabled:          true,
		SelectedOutbound: outbound,
		DNS:              in.dns,
		SrsOptions:       in.srs,
	}
	own := wizardmodels.EmitStateRulesWithoutOrder(nil, []*wizardmodels.RuleState{rs})
	if body == nil || len(own) == 0 {
		return "// " + locale.T("Fill in the rule to see what it adds to the config.")
	}
	rules, ownNum := rulePreviewAxis(model, in.editing, rs, own[0])

	srsCached := build.CollectSrsCachedPaths(rules, model.DataDir, model.ResourceDir)
	td := &wizardtemplate.TemplateData{}
	templateTags := []string{}
	if model.TemplateData != nil {
		td = &wizardtemplate.TemplateData{Presets: model.TemplateData.Presets, Vars: model.TemplateData.Vars}
		templateTags = build.TemplateRouteRuleSetTags(model.TemplateData.Config["route"])
	}
	// Общие значения настроек наборов (SPEC 185) — из Settings: без них превью
	// показало бы local там, где сборка выпустит remote. Ось целиком и теги
	// шаблона — как у сборки (SPEC 188): иначе превью показало бы тег `B`
	// там, где сборка выпустит `B (2)`. В показ идёт только правило окна.
	full := build.ResolveRouteTagged(&corestate.State{Rules: rules}, td, model.DataDir, srsCached, model.Target, model.SettingsVars, nil, templateTags)
	route := rulePreviewOwn(full, own[0], ownNum)

	var notes []string
	routeSec := &rulePreviewSection{}
	for _, r := range route.RuleSets {
		if r.Skipped {
			notes = append(notes, locale.T("Rule set not downloaded yet: the rule stays out of the config until it is."))
			continue
		}
		routeSec.RuleSet = append(routeSec.RuleSet, r.Body)
	}
	for _, r := range route.Rules {
		routeSec.Rules = append(routeSec.Rules, r.Body)
	}

	dnsSec := &rulePreviewSection{}
	mirrors := build.RuleDNSMirrors(route)
	if !in.dns.IsZero() && len(route.Rules) > 0 && len(mirrors) == 0 {
		notes = append(notes, locale.T("DNS options do not apply to this rule: it has no condition DNS queries can match, such as domains, apps or a rule set."))
	}
	enabled := make(map[string]bool)
	for _, t := range wizardbusiness.DNSEnabledTagOptions(model) {
		enabled[t] = true
	}
	present := func(tag string) bool {
		for _, raw := range model.DNSServers {
			var m map[string]interface{}
			if json.Unmarshal(raw, &m) == nil && m["tag"] == tag {
				return true
			}
		}
		return false
	}
	for _, m := range mirrors {
		for _, r := range m.Rules {
			if r["server"] != nil && !enabled[m.ServerTag] {
				if !present(m.ServerTag) {
					notes = append(notes, fmt.Sprintf(locale.T("DNS server %s not found: its DNS rule stays out of the config."), m.ServerTag))
					continue
				}
				notes = append(notes, fmt.Sprintf(locale.T("DNS server %s is turned off: it is kept in the config because this rule uses it."), m.ServerTag))
			}
			dnsSec.Rules = append(dnsSec.Rules, r)
		}
	}

	out := struct {
		Route *rulePreviewSection `json:"route,omitempty"`
		DNS   *rulePreviewSection `json:"dns,omitempty"`
	}{}
	if len(routeSec.RuleSet) > 0 || len(routeSec.Rules) > 0 {
		out.Route = routeSec
	}
	if len(dnsSec.Rules) > 0 {
		out.DNS = dnsSec
	}
	if out.Route == nil && out.DNS == nil && len(notes) > 0 {
		return "// " + strings.Join(notes, "\n// ")
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "// " + err.Error()
	}
	text := string(b)
	if len(notes) > 0 {
		text = "// " + strings.Join(notes, "\n// ") + "\n\n" + text
	}
	return text
}

// rulePreviewAxis — ось правил модели, в которой правило окна стоит на месте
// editing (новое — в конце пользовательской зоны); второй результат — номер
// правила окна на оси.
func rulePreviewAxis(model *wizardmodels.WizardModel, editing, rs *wizardmodels.RuleState, own corestate.Rule) ([]corestate.Rule, int) {
	customs := make([]*wizardmodels.RuleState, 0, len(model.CustomRules)+1)
	found := false
	for _, cr := range model.CustomRules {
		if editing != nil && cr == editing {
			cp := *rs
			cp.Num = editing.Num
			customs = append(customs, &cp)
			found = true
			continue
		}
		customs = append(customs, cr)
	}
	rules := wizardmodels.EmitStateRulesInAxisOrder(model.RuleOrder, model.PresetRefs, customs)
	if found {
		if editing.Num != nil {
			return rules, *editing.Num
		}
		return rules, corestate.DefaultRuleNum
	}
	num := corestate.NextUserRuleNum(rules)
	own.Num = &num
	return append(rules, own), num
}

// rulePreviewOwn — из маршрута всей оси только правило окна: его правило
// маршрута (имя и номер оси) и его наборы (id правила).
func rulePreviewOwn(full build.ResolvedRoute, own corestate.Rule, num int) build.ResolvedRoute {
	var out build.ResolvedRoute
	id := corestate.StableRuleID(own)
	for _, r := range full.Rules {
		if (r.Source == build.RouteSourceInline || r.Source == build.RouteSourceSrs) && r.Name == own.Name && r.Num == num {
			out.Rules = append(out.Rules, r)
		}
	}
	for _, rs := range full.RuleSets {
		if (rs.Source == build.RouteSourceInline || rs.Source == build.RouteSourceSrs) && rs.SrsID == id {
			out.RuleSets = append(out.RuleSets, rs)
		}
	}
	return out
}
