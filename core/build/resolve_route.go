// Package build — File resolve_route.go (SPEC 056-R-N follow-up).
//
// Unified resolver для route section (rule_set + route.rules) — параллельно
// resolve_dns.go. Один источник истины для UI render'а и build emit'а.
//
// Тот же контракт что у ResolveDNS: pure func, возвращает структурированный
// view с meta-данными (Source/Active/Enabled). Build emit'ит то у чего
// Active && Enabled.
package build

import (
	"encoding/json"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/outboundutil"
)

// RouteSource — discriminator происхождения route entry.
type RouteSource string

const (
	// RouteSourcePreset — pre-set bundled rule_set / routing rule.
	RouteSourcePreset RouteSource = "preset"

	// RouteSourceInline — user-defined inline rule (match-fields).
	RouteSourceInline RouteSource = "inline"

	// RouteSourceSrs — user-defined srs rule (local .srs file).
	RouteSourceSrs RouteSource = "srs"

	// RouteSourceNode — правило, которое узел носит с собой (SPEC 121).
	// rule_set эта ветка не создаёт: наборов правил у секций узла нет.
	RouteSourceNode RouteSource = "node"
)

// ResolvedRouteRuleSet — одна entry финального rule_set list'а.
type ResolvedRouteRuleSet struct {
	// Tag — финальный sing-box rule_set tag (с preset prefix для preset entries,
	// "user:<id>" для srs entries).
	Tag string

	// Body — готовое sing-box rule_set тело: {tag, type, format, rules|path|url}.
	// Для preset remote rule_set, если файл не cached, Body=nil + Skipped=true.
	Body map[string]interface{}

	// Source — preset|srs (inline не создаёт rule_set).
	Source RouteSource

	// PresetID/Label — только для Source=preset.
	PresetID    string
	PresetLabel string

	// SrsID — id user-srs rule (только для Source=srs).
	SrsID string

	// Skipped — true если rule_set не может быть эмитнут (remote .srs не cached
	// или srs cache miss). Build skip'ает; UI показывает с warning'ом.
	Skipped       bool
	SkippedReason string

	// Enabled — состояние правила-владельца (state.rules[i].enabled).
	//
	// Выключенный пресет НЕ должен тащить свои rule_set в конфиг: его
	// routing-правила не эмитятся, ссылаться на набор некому, а вот
	// требование к файлу остаётся — и на машине, где .srs нет, ядро падает
	// с «open …: no such file or directory». Так выключенные block-ads и
	// russian ломали конфиг для роутера.
	Enabled bool
}

// ResolvedRouteRule — одна entry финального route.rules[] list'а.
type ResolvedRouteRule struct {
	// Body — готовое sing-box route rule body после substitute + clean dangling.
	Body map[string]interface{}

	// Source — preset|inline|srs.
	Source RouteSource

	// PresetID/Label — только для Source=preset.
	PresetID    string
	PresetLabel string

	// OrderNum — позиция правила на оси порядка (SPEC 106). Нужна emit-слою:
	// правила пресетов дописываются к тем, что пришли из шаблона, и без
	// номера якорь с num=0 всё равно оказался бы в хвосте.
	OrderNum int

	// InlineID/SrsID — для kind=inline/srs.
	InlineID string
	SrsID    string

	// NodeTag — финальный тег узла-владельца (только Source=node, SPEC 121):
	// кем правило принесено, для диагностики и UI.
	NodeTag string

	// Active — прошёл if/if_or (только для preset; inline/srs всегда true).
	Active bool

	// Enabled — state.Rules[i].Enabled (top-level toggle).
	Enabled bool

	// InactiveReason — UI tooltip для !Active (только preset).
	InactiveReason string
}

// ResolvedRoute — результат ResolveRoute().
type ResolvedRoute struct {
	RuleSets []ResolvedRouteRuleSet
	Rules    []ResolvedRouteRule
}

// ResolveRoute — единая точка резолва route section.
//
// Аргументы:
//   - state         — v6 state (Rules с preset/inline/srs)
//   - td            — TemplateData (presets с rule_set + routing rule)
//   - execDir       — для резолва local SRS paths (preset remote rule_set)
//   - srsCachedPaths — map[user-rule-id → paths] для kind=srs (по пути на каждый URL правила)
//
// Возвращает ResolvedRoute. RuleSets дедуплицированы по tag (first-wins);
// Rules в порядке state.Rules.
func ResolveRoute(
	state *corestate.State,
	td *template.TemplateData,
	execDir string,
	srsCachedPaths map[string][]string,
	target template.TargetSpec,
) ResolvedRoute {
	return ResolveRouteWithGlobals(state, td, execDir, srsCachedPaths, target, nil)
}

// ResolveRouteWithGlobals — ResolveRoute с доступом тела пресета к ГЛОБАЛЬНЫМ
// переменным шаблона (SPEC 106, разрыв G3). Пресет может ссылаться на
// настройку со вкладки Settings (@tun, @resolve_strategy), не объявляя её у
// себя; локальная переменная с тем же именем всегда сильнее.
func ResolveRouteWithGlobals(
	state *corestate.State,
	td *template.TemplateData,
	execDir string,
	srsCachedPaths map[string][]string,
	target template.TargetSpec,
	globalVars map[string]string,
) ResolvedRoute {
	return ResolveRouteWithNodeSections(state, td, execDir, srsCachedPaths, target, globalVars, nil)
}

// ResolveRouteWithNodeSections — ResolveRouteWithGlobals плюс секции узлов
// (SPEC 121). Якорь kind=node находит здесь свой набор по ссылке {FolderID,
// Tag} и эмитит его правила подряд на своей позиции оси.
//
// nodeSections — наборы узлов, ДОШЕДШИХ до эмиссии. Якорь, чьего узла в
// списке нет (узел выключен, не собрался, снят санитайзером), пропускается
// МОЛЧА: это не поломка, а следствие выключенного узла, и warning здесь
// шумел бы на каждой сборке.
func ResolveRouteWithNodeSections(
	state *corestate.State,
	td *template.TemplateData,
	execDir string,
	srsCachedPaths map[string][]string,
	target template.TargetSpec,
	globalVars map[string]string,
	nodeSections []NodeSectionSet,
) ResolvedRoute {
	var out ResolvedRoute
	if state == nil || td == nil {
		return out
	}

	presetByID := make(map[string]*template.Preset, len(td.Presets))
	for i := range td.Presets {
		presetByID[td.Presets[i].ID] = &td.Presets[i]
	}

	sectionsByLink := make(map[NodeLink]*NodeSectionSet, len(nodeSections))
	nodeLinks := make([]corestate.NodeLink, 0, len(nodeSections))
	for i := range nodeSections {
		ns := &nodeSections[i]
		sectionsByLink[ns.Link] = ns
		if len(ns.Rules) > 0 {
			nodeLinks = append(nodeLinks, corestate.NodeLink{FolderID: ns.Link.FolderID, Tag: ns.Link.Tag})
		}
	}

	emittedTags := make(map[string]bool)

	// SPEC 106: порядок правил задаёт разреженная ось (OrderNum), а не позиция
	// в слайсе. Нормализация здесь же пере-засевает неотчуждаемые пресеты —
	// именно re-seed на каждой сборке, а не флаг в state, делает их
	// неотчуждаемыми (D-050): стёртое из state правило возвращается.
	// SPEC 121: тем же проходом сеются якоря правил, которые узлы носят с собой.
	rules := corestate.NormalizeRuleOrder(state.Rules, template.RuleOrderSpecs(td.Presets), nodeLinks)

	for _, rule := range rules {
		switch rule.Kind {
		case corestate.RuleKindPreset:
			resolvePresetRouteRule(&out, presetByID, rule, execDir, emittedTags, target, globalVars)
		case corestate.RuleKindInline:
			resolveInlineRouteRule(&out, rule)
		case corestate.RuleKindSrs:
			resolveSrsRouteRule(&out, rule, srsCachedPaths, emittedTags)
		case corestate.RuleKindNode:
			resolveNodeRouteRule(&out, rule, sectionsByLink)
		}
	}

	return out
}

// resolveNodeRouteRule — якорь kind=node → правила маршрута, которые узел
// носит с собой (SPEC 121 §4 п. 4).
//
// Все правила набора эмитятся ПОДРЯД на позиции якоря: у якоря один номер на
// оси, и раздавать вложенным правилам свои значило бы завести вторую ось
// внутри одной позиции.
func resolveNodeRouteRule(out *ResolvedRoute, rule corestate.Rule, sectionsByLink map[NodeLink]*NodeSectionSet) {
	body, err := rule.DecodeBody()
	if err != nil {
		debuglog.WarnLog("route resolve: decode node body: %v", err)
		return
	}
	nb := body.(*corestate.NodeRuleBody)
	set, ok := sectionsByLink[NodeLink{FolderID: nb.FolderID, Tag: nb.Tag}]
	if !ok {
		// Узла в конфиге нет — молча: якорь остаётся на оси, фрагментов нет.
		return
	}
	frags, warns := ExpandNodeSections(*set)
	for _, w := range warns {
		debuglog.WarnLog("route resolve: %s", w)
	}
	for _, rr := range frags.RoutingRules {
		out.Rules = append(out.Rules, ResolvedRouteRule{
			Body:     rr,
			Source:   RouteSourceNode,
			NodeTag:  set.FinalTag,
			Active:   true,
			Enabled:  rule.Enabled,
			OrderNum: ruleOrderNum(rule),
		})
	}
}

// resolvePresetRouteRule — expand preset → append rule_sets + routing rule.
func resolvePresetRouteRule(
	out *ResolvedRoute,
	presetByID map[string]*template.Preset,
	rule corestate.Rule,
	execDir string,
	emittedTags map[string]bool,
	target template.TargetSpec,
	globalVars map[string]string,
) {
	p, ok := presetByID[rule.Ref]
	if !ok {
		debuglog.WarnLog("route resolve: preset ref %q not found in template", rule.Ref)
		return
	}
	body, err := rule.DecodeBody()
	if err != nil {
		debuglog.WarnLog("route resolve: decode preset body for %q: %v", rule.Ref, err)
		return
	}
	pb := body.(*corestate.PresetBody)
	frags, warns, ok := ExpandPresetWithGlobals(p, pb.Vars, globalVars, target)
	for _, w := range warns {
		debuglog.WarnLog("route resolve: %s", w.String())
	}
	if !ok {
		return
	}
	presetLabel := p.DisplayLabel()

	// RuleSets из preset.RuleSet (после ExpandPreset уже substituted + prefixed).
	for _, rs := range frags.RuleSets {
		tag, _ := rs["tag"].(string)
		if tag == "" {
			continue
		}
		if emittedTags[tag] {
			continue
		}
		converted, skip := convertPresetRuleSetRemoteToLocal(rs, execDir, target.ResourceDir, target.SrsLocalDir)
		if skip {
			out.RuleSets = append(out.RuleSets, ResolvedRouteRuleSet{
				Tag:           tag,
				Source:        RouteSourcePreset,
				PresetID:      p.ID,
				PresetLabel:   presetLabel,
				Enabled:       rule.Enabled,
				Skipped:       true,
				SkippedReason: "remote .srs not cached",
			})
			continue
		}
		out.RuleSets = append(out.RuleSets, ResolvedRouteRuleSet{
			Tag:         tag,
			Body:        converted,
			Source:      RouteSourcePreset,
			PresetID:    p.ID,
			PresetLabel: presetLabel,
			Enabled:     rule.Enabled,
		})
		emittedTags[tag] = true
	}

	// Routing rules. Cleanup dangling refs (если remote rule_set skipped).
	// SPEC 067 Phase 9: каждая rule из frags.RoutingRules эмитится отдельной
	// ResolvedRouteRule (in-order).
	for _, rr := range frags.RoutingRules {
		cleaned := cleanDanglingRuleSetInRule(rr, emittedTags)
		if cleaned == nil {
			continue
		}
		out.Rules = append(out.Rules, ResolvedRouteRule{
			Body:        cleaned,
			Source:      RouteSourcePreset,
			PresetID:    p.ID,
			PresetLabel: presetLabel,
			Active:      true, // ExpandPreset уже отфильтровал по if/if_or
			Enabled:     rule.Enabled,
			OrderNum:    ruleOrderNum(rule),
		})
	}
}

// resolveInlineRouteRule — kind=inline → direct route rule, no rule_set.
func resolveInlineRouteRule(out *ResolvedRoute, rule corestate.Rule) {
	body, err := rule.DecodeBody()
	if err != nil {
		debuglog.WarnLog("route resolve: decode inline body: %v", err)
		return
	}
	ib := body.(*corestate.InlineBody)
	match := ib.Match
	if match == nil {
		match = map[string]interface{}{}
	}
	routeRule := make(map[string]interface{}, len(match)+1)
	for k, v := range match {
		routeRule[k] = v
	}
	routeRule = outboundutil.ApplyOutboundToRule(routeRule, ib.Outbound)
	out.Rules = append(out.Rules, ResolvedRouteRule{
		Body:     routeRule,
		Source:   RouteSourceInline,
		InlineID: corestate.StableRuleID(rule),
		Active:   true,
		Enabled:  rule.Enabled,
		OrderNum: ruleOrderNum(rule),
	})
}

// resolveSrsRouteRule — kind=srs → local rule_set (from cache) + route rule.
func resolveSrsRouteRule(
	out *ResolvedRoute,
	rule corestate.Rule,
	srsCachedPaths map[string][]string,
	emittedTags map[string]bool,
) {
	body, err := rule.DecodeBody()
	if err != nil {
		debuglog.WarnLog("route resolve: decode srs body: %v", err)
		return
	}
	sb := body.(*corestate.SrsBody)
	id := corestate.StableRuleID(rule)
	urls := sb.URLs()
	paths, hasCache := srsCachedPaths[id]
	// Правило — одна единица: либо закэшированы все его наборы, либо оно
	// пропускается целиком (частичный набор молча менял бы смысл правила).
	if !hasCache || len(paths) != len(urls) {
		out.RuleSets = append(out.RuleSets, ResolvedRouteRuleSet{
			Tag:           corestate.SrsRuleSetTag(id, 0),
			Source:        RouteSourceSrs,
			SrsID:         id,
			Enabled:       rule.Enabled,
			Skipped:       true,
			SkippedReason: "srs file not cached",
		})
		debuglog.WarnLog("route resolve: srs rule %q skipped: no cached file", sb.Name)
		return
	}
	// Запись rule_set на каждый набор правила (репорт 1.5.5: из трёх srs в
	// одном правиле эмитился первый). Теги — corestate.SrsRuleSetTag: первый
	// набор держит исторический "user:<id>", остальные "user:<id>:N".
	tags := make([]string, 0, len(paths))
	for i, path := range paths {
		tag := corestate.SrsRuleSetTag(id, i)
		tags = append(tags, tag)
		if emittedTags[tag] {
			continue
		}
		rs := map[string]interface{}{
			"tag":    tag,
			"type":   "local",
			"format": "binary",
			"path":   path,
		}
		out.RuleSets = append(out.RuleSets, ResolvedRouteRuleSet{
			Tag:     tag,
			Body:    rs,
			Source:  RouteSourceSrs,
			SrsID:   id,
			Enabled: rule.Enabled,
		})
		emittedTags[tag] = true
	}
	var ruleSetRef interface{} = tags[0]
	if len(tags) > 1 {
		ruleSetRef = tags
	}
	routeRule := map[string]interface{}{"rule_set": ruleSetRef}
	routeRule = outboundutil.ApplyOutboundToRule(routeRule, sb.Outbound)
	out.Rules = append(out.Rules, ResolvedRouteRule{
		Body:    routeRule,
		Source:  RouteSourceSrs,
		SrsID:   id,
		Active:  true,
		Enabled: rule.Enabled,
	})
}

// ── Helper: silence unused json import (will be used by tests). ──
var _ = json.Unmarshal

// ruleOrderNum — номер правила на оси; неразмеченное считается стоящим в
// начале пользовательской зоны (SPEC 106).
func ruleOrderNum(r corestate.Rule) int {
	if r.OrderNum == nil {
		return corestate.DefaultRuleNum
	}
	return *r.OrderNum
}
