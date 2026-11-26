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
	"singbox-launcher/internal/paths"
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
)

// ResolvedRouteRuleSet — одна entry финального rule_set list'а.
type ResolvedRouteRuleSet struct {
	// Tag — финальный sing-box rule_set tag (с preset prefix для preset entries;
	// у собственных правил — по имени правила, TEMPLATE_LANG §6.13).
	Tag string

	// Body — готовое sing-box rule_set тело: {tag, type, format, rules|path|url}.
	// Для preset remote rule_set, если файл не cached, Body=nil + Skipped=true.
	Body map[string]interface{}

	// Source — preset | srs | inline (inline-набор правила без ссылок, SPEC 188).
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

	// owner — номер записи-владельца на оси + 1 (0 — пресет): снятие правила
	// по висячему рукописному rule_set снимает и его наборы.
	owner int
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

	// Num — позиция правила на оси порядка (SPEC 106; до v8 звалась order_num). Нужна
	// emit-слою: правила пресетов дописываются к тем, что пришли из шаблона, и
	// без номера якорь с num=0 всё равно оказался бы в хвосте.
	Num int

	// InlineID/SrsID — для kind=inline/srs.
	InlineID string
	SrsID    string

	// Name — имя записи inline/srs (владелец предупреждений DNS-зеркала).
	Name string

	// DNS — DNS-зеркало записи inline/srs (state.Rule.DNS, TEMPLATE_LANG
	// §6.11); nil — опций нет.
	DNS *corestate.RuleDNS

	// Mirror — матчер DNS-зеркала по дереву выпущенного тела (§6.11, SPEC
	// 188); nil — зеркало пустое. Считается при выпуске: только он знает,
	// какие ключи ушли в inline-наборы и есть ли в них домены.
	Mirror map[string]interface{}

	// MirrorDomains — доменный ключ или ссылка на набор хоть в одном простом
	// узле дерева: условие Block HTTPS (§6.11).
	MirrorDomains bool

	// handwritten — рукописные теги rule_set тела (проверка висячих после
	// выпуска всей оси).
	handwritten []string

	// owner — номер записи на оси + 1 (0 — пресет).
	owner int

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

	// Warnings — предупреждения раскрытия включённых пресетов с кодами реестра
	// (SPEC 143): подстановка и выпавшие фрагменты. Сборка кладёт их в отчёт
	// видом template_degraded; порядок и дубли снимает она же.
	Warnings []template.TemplateWarning
}

// ResolveRoute — единая точка резолва route section.
//
// Аргументы:
//   - state         — v6 state (Rules с preset/inline/srs)
//   - td            — TemplateData (presets с rule_set + routing rule)
//   - dataDir       — для резолва local SRS paths (preset remote rule_set)
//   - srsCachedPaths — map[ссылка набора → путь к скачанному файлу] для kind=srs
//     (CollectSrsCachedPaths; копия привязана к ссылке, а не к правилу)
//
// Возвращает ResolvedRoute. RuleSets дедуплицированы по tag (first-wins);
// Rules в порядке state.Rules.
func ResolveRoute(
	state *corestate.State,
	td *template.TemplateData,
	dataDir paths.DataDir,
	srsCachedPaths map[string]string,
	target template.TargetSpec,
) ResolvedRoute {
	return ResolveRouteWithGlobals(state, td, dataDir, srsCachedPaths, target, nil)
}

// ResolveRouteWithGlobals — ResolveRoute с доступом тела пресета к ГЛОБАЛЬНЫМ
// переменным шаблона (SPEC 106, разрыв G3). Пресет может ссылаться на
// настройку со вкладки Settings (@tun, @resolve_strategy), не объявляя её у
// себя; локальная переменная с тем же именем всегда сильнее. Объявления
// глобалей — td.Vars (SPEC 143 Т2).
func ResolveRouteWithGlobals(
	state *corestate.State,
	td *template.TemplateData,
	dataDir paths.DataDir,
	srsCachedPaths map[string]string,
	target template.TargetSpec,
	globalVars map[string]string,
) ResolvedRoute {
	return ResolveRouteChecked(state, td, dataDir, srsCachedPaths, target, globalVars, nil)
}

// ResolveRouteChecked — ResolveRouteWithGlobals со знанием итогового множества
// тегов выходов (SPEC 185 §2.4): набор в режиме remote, чей download_detour
// указывает на недоступную цель, не эмитится. finalTags == nil — проверки нет
// (превью, вызовы без графа). Теги route.rule_set шаблона неизвестны —
// рукописные rule_set правил на висячесть не проверяются (ResolveRouteTagged).
func ResolveRouteChecked(
	state *corestate.State,
	td *template.TemplateData,
	dataDir paths.DataDir,
	srsCachedPaths map[string]string,
	target template.TargetSpec,
	globalVars map[string]string,
	finalTags map[string]bool,
) ResolvedRoute {
	return ResolveRouteTagged(state, td, dataDir, srsCachedPaths, target, globalVars, finalTags, nil)
}

// ResolveRouteTagged — ResolveRouteChecked с тегами route.rule_set шаблона
// (SPEC 188, TEMPLATE_LANG §6.13): они заняты до раздачи тегов наборам
// правил и считаются выпущенными при проверке рукописных rule_set.
// templateTags == nil — шаблон неизвестен: теги раздаются без него, проверки
// висячих рукописных ссылок нет. Все вызовы одной сборки обязаны передавать
// одни и те же rules и templateTags — иначе теги наборов у маршрута и
// DNS-зеркала разойдутся.
func ResolveRouteTagged(
	state *corestate.State,
	td *template.TemplateData,
	dataDir paths.DataDir,
	srsCachedPaths map[string]string,
	target template.TargetSpec,
	globalVars map[string]string,
	finalTags map[string]bool,
	templateTags []string,
) ResolvedRoute {
	var out ResolvedRoute
	if state == nil || td == nil {
		return out
	}

	presetByID := make(map[string]*template.Preset, len(td.Presets))
	for i := range td.Presets {
		presetByID[td.Presets[i].ID] = &td.Presets[i]
	}

	emittedTags := make(map[string]bool)

	// SPEC 106: порядок правил задаёт разреженная ось (Num), а не позиция
	// в слайсе. Нормализация здесь же пере-засевает неотчуждаемые пресеты —
	// именно re-seed на каждой сборке, а не флаг в state, делает их
	// неотчуждаемыми (D-050): стёртое из state правило возвращается.
	rules := corestate.NormalizeRuleOrder(state.Rules, template.RuleOrderSpecs(td.Presets))

	// Теги наборов правил — один раз на ось до выпуска (§6.13): заняты теги
	// шаблона и наборов пресетов, базу и производные берут все записи
	// inline|srs, включённые и нет.
	reserved := ReservedRuleSetTags(presetByID, rules, templateTags)
	keys := loadRuleTreeKeys()
	ruleTags := corestate.AllocateRuleTags(rules, func(t string) bool { return reserved[t] }, keys.set)

	for i, rule := range rules {
		switch rule.Kind {
		case corestate.RuleKindPreset:
			resolvePresetRouteRule(&out, presetByID, rule, dataDir, emittedTags, target, globalVars, td.Vars, td.PresetNodes, finalTags)
		case corestate.RuleKindInline, corestate.RuleKindSrs:
			resolveUserRouteRule(&out, rule, i+1, ruleTags[i], srsCachedPaths, emittedTags, globalVars, finalTags, keys)
		}
	}
	if templateTags != nil {
		dropDanglingHandwrittenRuleSets(&out, templateTags)
	}

	return out
}

// resolvePresetRouteRule — expand preset → append rule_sets + routing rule.
func resolvePresetRouteRule(
	out *ResolvedRoute,
	presetByID map[string]*template.Preset,
	rule corestate.Rule,
	dataDir paths.DataDir,
	emittedTags map[string]bool,
	target template.TargetSpec,
	globalVars map[string]string,
	globalDecls []template.TemplateVar,
	nodes []template.PresetNode,
	finalTags map[string]bool,
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
	frags, warns, ok := ExpandPresetForNodes(p, pb.Vars, globalVars, globalDecls, target, nodes)
	for _, w := range warns {
		debuglog.WarnLog("route resolve: %s", w.String())
	}
	// Выключенный пресет в конфиг не идёт — и его деградация пользователю
	// ни к чему.
	if rule.Enabled {
		out.Warnings = append(out.Warnings, expandTemplateWarnings(warns)...)
	}
	if !ok {
		return
	}
	presetLabel := p.DisplayLabel()
	policy := RulePolicy(rule, globalVars)

	// RuleSets из preset.RuleSet (после ExpandPreset уже substituted + prefixed).
	for _, rs := range frags.RuleSets {
		tag, _ := rs["tag"].(string)
		if tag == "" {
			continue
		}
		if emittedTags[tag] {
			continue
		}
		var converted map[string]interface{}
		skip, skipReason := false, "remote .srs not cached"
		rsType, _ := rs["type"].(string)
		rsURL, _ := rs["url"].(string)
		if policy.Remote && rsType == "remote" && rsURL != "" {
			// SPEC 185: набор качает ядро — файл у лаунчера не нужен.
			format, _ := rs["format"].(string)
			body, ok := remoteRuleSetBody(tag, format, rsURL, policy, finalTags)
			converted, skip, skipReason = body, !ok, "download_detour unavailable"
		} else {
			converted, skip = convertPresetRuleSetRemoteToLocal(rs, dataDir, target.ResourceDir, target.SrsLocalDir)
		}
		if skip {
			out.RuleSets = append(out.RuleSets, ResolvedRouteRuleSet{
				Tag:           tag,
				Source:        RouteSourcePreset,
				PresetID:      p.ID,
				PresetLabel:   presetLabel,
				Enabled:       rule.Enabled,
				Skipped:       true,
				SkippedReason: skipReason,
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
		cleaned, lost := cleanDanglingRuleSetInRule(rr, emittedTags)
		if lost && rule.Enabled {
			// SPEC 153: наборов правила нет в конфиге (.srs не скачан) —
			// правило выпало целиком, иначе оно ловило бы весь трафик.
			out.Warnings = append(out.Warnings, fragmentDropped(p.ID, fragmentKindRule, "rule_set").templateWarning())
		}
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
			Num:         ruleNum(rule),
		})
	}
}

// resolveUserRouteRule — запись inline|srs: дерево тела → записи rule_set
// (ссылки и inline-наборы) + правило маршрута + матчер DNS-зеркала
// (rule_tree_emit.go, TEMPLATE_LANG §6.13, §6.11). Всё или ничего: не доехал
// хоть один набор дерева — правило не выпускается, его наборы — Skipped.
func resolveUserRouteRule(
	out *ResolvedRoute,
	rule corestate.Rule,
	owner int,
	tags corestate.RuleTags,
	srsCachedPaths map[string]string,
	emittedTags map[string]bool,
	globalVars map[string]string,
	finalTags map[string]bool,
	keys ruleTreeKeys,
) {
	if _, err := rule.DecodeBody(); err != nil {
		debuglog.WarnLog("route resolve: decode %s body: %v", rule.Kind, err)
		return
	}
	policy := RulePolicy(rule, globalVars)
	e := emitUserRule(rule, tags, srsCachedPaths, policy, finalTags, keys)
	for _, rs := range e.RuleSets {
		rs.owner = owner
		if !rs.Skipped && emittedTags[rs.Tag] {
			continue
		}
		out.RuleSets = append(out.RuleSets, rs)
		if !rs.Skipped {
			emittedTags[rs.Tag] = true
		}
	}
	if e.Body == nil {
		if len(e.RuleSets) == 0 && (rule.Kind == corestate.RuleKindSrs || e.SkipReason == ruleSkipRefsOnGroup) {
			out.RuleSets = append(out.RuleSets, ResolvedRouteRuleSet{
				Tag: tags.Base, Source: userRouteSource(rule), SrsID: corestate.StableRuleID(rule),
				Enabled: rule.Enabled, Skipped: true, SkippedReason: e.SkipReason, owner: owner,
			})
		}
		debuglog.WarnLog("route resolve: rule %q skipped: %s", rule.Name, e.SkipReason)
		return
	}
	r := ResolvedRouteRule{
		Body:          e.Body,
		Source:        userRouteSource(rule),
		Name:          rule.Name,
		DNS:           rule.DNS,
		Mirror:        e.Mirror,
		MirrorDomains: e.MirrorDomains,
		Active:        true,
		Enabled:       rule.Enabled,
		Num:           ruleNum(rule),
		handwritten:   e.Handwritten,
		owner:         owner,
	}
	if r.Source == RouteSourceSrs {
		r.SrsID = corestate.StableRuleID(rule)
	} else {
		r.InlineID = corestate.StableRuleID(rule)
	}
	out.Rules = append(out.Rules, r)
}

func userRouteSource(rule corestate.Rule) RouteSource {
	if rule.Kind == corestate.RuleKindSrs {
		return RouteSourceSrs
	}
	return RouteSourceInline
}

// dropDanglingHandwrittenRuleSets — рукописный rule_set собственного правила,
// ссылающийся на тег, которого нет в итоговом route.rule_set (шаблон +
// выпущенные включённые наборы), снимает правило целиком (§6.13), а с ним и
// наборы этого правила (их некому держать). Снятие может сделать висячими
// ссылки других правил — проход повторяется до фикспойнта, как у чистки
// висячих rule_set пресетов (cleanDanglingRuleSetInRule).
func dropDanglingHandwrittenRuleSets(out *ResolvedRoute, templateTags []string) {
	for {
		valid := make(map[string]bool, len(templateTags)+len(out.RuleSets))
		for _, t := range templateTags {
			valid[t] = true
		}
		for _, rs := range out.RuleSets {
			if !rs.Skipped && rs.Enabled {
				valid[rs.Tag] = true
			}
		}
		changed := false
		kept := out.Rules[:0]
		for _, r := range out.Rules {
			if !r.Enabled || len(r.handwritten) == 0 {
				kept = append(kept, r)
				continue
			}
			var dangling []string
			for _, t := range r.handwritten {
				if !valid[t] {
					dangling = append(dangling, t)
				}
			}
			if len(dangling) == 0 {
				kept = append(kept, r)
				continue
			}
			changed = true
			reason := ruleSkipDanglingPrefix + joinTags(dedupStrings(dangling))
			debuglog.WarnLog("route resolve: rule %q skipped: %s", r.Name, reason)
			for i := range out.RuleSets {
				rs := &out.RuleSets[i]
				if r.owner != 0 && rs.owner == r.owner && !rs.Skipped {
					rs.Skipped, rs.SkippedReason, rs.Body = true, reason, nil
				}
			}
		}
		out.Rules = kept
		if !changed {
			return
		}
	}
}

// ── Helper: silence unused json import (will be used by tests). ──
var _ = json.Unmarshal

// ruleNum — номер правила на оси; неразмеченное считается стоящим в
// начале пользовательской зоны (SPEC 106).
func ruleNum(r corestate.Rule) int {
	if r.Num == nil {
		return corestate.DefaultRuleNum
	}
	return *r.Num
}
