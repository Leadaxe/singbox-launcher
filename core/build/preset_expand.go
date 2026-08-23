// Package build содержит код сборки финального sing-box config.json
// из state.json + template.json.
//
// File preset_expand.go — expansion engine для preset bundles (SPEC 053).
//
// ExpandPreset резолвит template.preset + user varsValues в готовые фрагменты
// config.json (route.rule_set, route.rules, dns.servers, dns.rules).
//
// Алгоритм (см. SPEC §«Build pipeline → Expand preset-ref»):
//  1. Build varsMap из user values + template defaults
//  2. Filter vars/fragments по if/if_or
//  3. Deep-copy fragments, substitute @name
//  4. Prefix local tags `<preset_id>:<tag>`
//  5. Filter bundled dns_servers через @dns_server / literal в dns_rule.server
//  6. Apply outbound sentinels (reject/drop) — через существующий ApplyOutboundToRule
//  7. Clean dangling rule_set refs (после if-filter некоторые tag'и могли отсутствовать)
//  8. Strip detour: "direct-out" в DNS-серверах
//
// Substitute — ТУПАЯ ТЕКСТОВАЯ ЗАМЕНА (no _Dropped sentinel). Опциональность
// достигается через `if`/`if_or` на vars и фрагментах.
package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/core/template"
	"singbox-launcher/internal/outboundutil"
)

// TagSeparator — разделитель в auto-prefixed tag'ах `<preset_id>:<local_tag>`.
// Решено `:` для согласования со subscription prefix scheme (SPEC 052).
const TagSeparator = ":"

// PresetFragments — результат раскрытия одного preset-ref'а.
type PresetFragments struct {
	// RuleSets — определения rule_set с уже префиксованными tag'ами.
	// Пустой если все элементы preset.rule_set имели if=false.
	RuleSets []map[string]interface{}

	// RoutingRules — routing rules (preset.rules после substitute и prefix).
	// Каждая entry эмитится в порядке исходного списка. Empty slice если все
	// rules имеют if=false или после dangling-cleanup стали пустыми.
	//
	// SPEC 067 Phase 9: было одиночное RoutingRule, теперь slice — соответствует
	// Preset.Rules []map (multi-rule presets как split-all-traffic).
	RoutingRules []map[string]interface{}

	// DNSRule — dns rule (preset.dns_rule). nil если нет / if=false / dangling.
	DNSRule map[string]interface{}

	// DNSRules — упорядоченный список dns rules (preset.dns_rules, SPEC 085.1).
	// Разворачиваются в порядке; predefined/action-правила (без server)
	// сохраняются. Пусто если preset их не определяет.
	DNSRules []map[string]interface{}

	// DNSServers — bundled DNS-серверы, отфильтрованные через @dns_server var.
	// Только tag'и упомянутые в emit'ах попадают сюда. С префиксом `<preset_id>:`.
	DNSServers []map[string]interface{}
}

// ExpandWarning — non-fatal предупреждение expansion engine'а.
type ExpandWarning struct {
	PresetID string
	Message  string
}

func (w ExpandWarning) String() string {
	return fmt.Sprintf("preset %q: %s", w.PresetID, w.Message)
}

// ExpandPreset выполняет полное раскрытие preset'а.
//
// userVars — значения переменных из state.rule.body.vars (только diff от
// default'ов; пустые / отсутствующие резолвятся через template.preset.vars[].default).
//
// target — платформа и роль ЦЕЛЕВОЙ машины для runtime globals
// (@runtime.platform / @runtime.arch / @runtime.target) в #if predicates
// (SPEC 067, SPEC 097). Callers передают TargetSpec; тесты — fakes.
//
// Возвращает (fragments, warnings, ok). ok=false если preset нельзя раскрыть
// (например unresolved @var) — в этом случае fragments частично заполнен,
// но caller должен пропустить preset целиком.
func ExpandPreset(preset *template.Preset, userVars map[string]string, target template.TargetSpec) (*PresetFragments, []ExpandWarning, bool) {
	return ExpandPresetWithGlobals(preset, userVars, nil, target)
}

// ExpandPresetWithGlobals — ExpandPreset с доступом к ГЛОБАЛЬНЫМ переменным
// шаблона (SPEC 106, разрыв G3; модель LxBox `preset_expand.dart:165-167`).
//
// Тело пресета может ссылаться на глобальную переменную (`@tun`,
// `@resolve_strategy`), не объявляя её у себя: настройка живёт на вкладке
// Settings и общая для всего конфига, дублировать её в каждом пресете
// бессмысленно. Глобали подмешиваются ТОЛЬКО там, где нет одноимённой
// локальной — локальная всегда сильнее (putIfAbsent, а не перезапись).
//
// Без этого неотчуждаемый traffic-processing не собирается вовсе: его правила
// ссылаются на @tun/@enable_proxy_in/@resolve_strategy, и все три уходят в
// unresolved → пресет выбрасывается целиком.
func ExpandPresetWithGlobals(
	preset *template.Preset,
	userVars map[string]string,
	globalVars map[string]string,
	target template.TargetSpec,
) (*PresetFragments, []ExpandWarning, bool) {
	if preset == nil {
		return nil, nil, false
	}

	var warnings []ExpandWarning

	// === 1. Build varsMap ===
	varsMap := make(map[string]string, len(preset.Vars)+len(globalVars))
	for _, v := range preset.Vars {
		// SPEC 106 (модель LxBox §265): ref-переменная не хранит значение у
		// себя — оно живёт в глобальных vars. Пресет лишь показывает общую
		// настройку в своих параметрах; копия разъехалась бы с оригиналом при
		// первом же изменении.
		if v.Ref != "" {
			if gv, ok := globalVars[v.Ref]; ok && gv != "" {
				varsMap[v.Name] = gv
			}
			// Пустая глобаль → имени нет в varsMap: фрагмент со ссылкой на
			// него выпадет как optional-var, а не подставит пустоту.
			continue
		}
		if userVal, ok := userVars[v.Name]; ok && userVal != "" {
			varsMap[v.Name] = userVal
			continue
		}
		if v.Default != "" {
			varsMap[v.Name] = v.Default
			continue
		}
		// Пусто и обязательна — фрагменты, которые на неё ссылаются, собрать
		// нечем. Пресет при этом НЕ выбрасывается целиком (D-011): выпадут
		// только те правила, где переменная реально участвует.
		if v.Required {
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("required var %q is empty — fragments using it are dropped", v.Name)})
			continue
		}
		varsMap[v.Name] = v.Default
	}
	// Глобали — только для имён, которых пресет не объявил у себя.
	for name, val := range globalVars {
		if _, local := varsMap[name]; !local {
			varsMap[name] = val
		}
	}

	// === 2. Filter vars by if/if_or (resolve once, may exclude vars from substitute) ===
	activeVars := filterActiveVars(preset.Vars, varsMap, target)
	// Удаляем неактивные vars из varsMap чтобы substitute @name на них упал → unresolved warning.
	//
	// SPEC 106 (G3): фильтр применяется ТОЛЬКО к переменным, объявленным в
	// самом пресете. Глобали шаблона под него не попадают — у них нет
	// локального if/if_or, и они не «неактивны», а просто объявлены в другом
	// месте. Без этой оговорки @tun/@enable_proxy_in вычищались отсюда и
	// доходили до подстановки как unknown var, роняя весь пресет.
	declaredLocally := make(map[string]bool, len(preset.Vars))
	for _, v := range preset.Vars {
		declaredLocally[v.Name] = true
	}
	for name := range varsMap {
		if declaredLocally[name] && !activeVars[name] {
			delete(varsMap, name)
		}
	}

	frags := &PresetFragments{}

	// === 3. Filter + substitute rule_set ===
	emittedTags := make(map[string]bool) // tag после prefix
	for _, rs := range preset.RuleSet {
		if !template.NormalizeGate(rs.EnableRaw(), rs.If, rs.IfOr).SatisfiedVars(varsMap, target) {
			continue
		}
		raw, err := deepCopy(rs)
		if err != nil {
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("deep copy rule_set %q: %v", rs.Tag, err)})
			continue
		}
		substituted, ok := substitutePresetBody(raw, preset.Vars, varsMap, target)
		if !ok {
			// Dropped-каскад (§5.1, preset_types.go Required): выпадает ОДИН
			// фрагмент, а не пресет целиком — пустая обязательная переменная
			// одного правила не должна снимать несвязанные правила и
			// dns_servers того же пресета (у russian это фильтр «исключить
			// 🇷🇺-узлы», и его молчаливая потеря опаснее недостающего rule_set).
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("unresolved @var in rule_set %q — фрагмент выброшен", rs.Tag)})
			continue
		}
		m, _ := substituted.(map[string]interface{})
		if m == nil {
			continue
		}
		// Strip служебные ключи гейта (уже резолвлены) — sing-box их не знает.
		stripGateKeys(m)
		// Prefix tag.
		localTag, _ := m["tag"].(string)
		prefixed := preset.ID + TagSeparator + localTag
		m["tag"] = prefixed
		emittedTags[localTag] = true // ← для cleanDanglingRefs ниже сравниваем по local
		frags.RuleSets = append(frags.RuleSets, m)
	}

	// === 4. Resolve routing rules ===
	// SPEC 067 Phase 9: preset.Rules — slice. Каждая rule имеет свой `if`/`if_or`
	// gate. Эмитятся в порядке исходного списка.
	for idx, ruleMap := range preset.Rules {
		if ruleMap == nil {
			continue
		}
		if !extractGateFromMap(ruleMap).SatisfiedVars(varsMap, target) {
			continue
		}
		raw, err := deepCopyMap(ruleMap)
		if err != nil {
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("deep copy rules[%d]: %v", idx, err)})
			continue
		}
		substituted, ok := substitutePresetBody(raw, preset.Vars, varsMap, target)
		if !ok {
			// Dropped-каскад: выпадает одно правило, остальной пресет живёт.
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("unresolved @var in rules[%d] — правило выброшено", idx)})
			continue
		}
		m, _ := substituted.(map[string]interface{})
		if m == nil {
			continue
		}
		stripGateKeys(m)
		// Rewrite rule_set refs: local → prefixed, filter dangling.
		rewriteRuleSetRefs(m, preset.ID, emittedTags)
		// Apply outbound sentinels (reject/drop) — shared util с UI.
		if outbound, ok := m["outbound"].(string); ok {
			m = outboundutil.ApplyOutboundToRule(m, outbound)
		}
		if !isRuleEmpty(m, emittedTags) {
			frags.RoutingRules = append(frags.RoutingRules, m)
		} else {
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("rules[%d] dropped (no valid rule_set refs after if-filter)", idx)})
		}
	}

	// === 5. Resolve dns_rule (singular) + dns_rules (plural, SPEC 085.1) ===
	if preset.DNSRule != nil {
		if m, ok := expandOnePresetDNSRule(preset, preset.DNSRule, varsMap, emittedTags, target, &warnings); ok {
			frags.DNSRule = m
		}
	}
	for _, dr := range preset.DNSRules {
		if dr == nil {
			continue
		}
		if m, ok := expandOnePresetDNSRule(preset, dr, varsMap, emittedTags, target, &warnings); ok {
			frags.DNSRules = append(frags.DNSRules, m)
		}
	}

	// === 6. dns_servers — БЕЗ consumption-filter (SPEC 056-R-N follow-up).
	// Все bundled DNS-серверы preset'а (с if/if_or filter) попадают в frags.
	// Per-server enable управляется через state.DNS.Servers[kind=preset].Enabled,
	// который применяется в ResolveDNS → MergePresetsIntoDNS. Здесь только
	// материализуем body + substitute.
	for _, ds := range preset.DNSServers {
		if !template.NormalizeGate(ds.EnableRaw(), ds.If, ds.IfOr).SatisfiedVars(varsMap, target) {
			continue
		}
		raw, err := deepCopy(ds)
		if err != nil {
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("deep copy dns_server %q: %v", ds.Tag, err)})
			continue
		}
		substituted, ok := substitutePresetBody(raw, preset.Vars, varsMap, target)
		if !ok {
			// Dropped-каскад: выпадает один сервер, остальной пресет живёт.
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("unresolved @var in dns_server %q — сервер выброшен", ds.Tag)})
			continue
		}
		m, _ := substituted.(map[string]interface{})
		// Strip UI-only / control fields.
		delete(m, "if")
		delete(m, "if_or")
		delete(m, "title")
		// Strip detour=direct-out (sing-box резолвит без forwarding).
		if det, ok := m["detour"].(string); ok && det == "direct-out" {
			delete(m, "detour")
		}
		// Prefix tag.
		localTag, _ := m["tag"].(string)
		m["tag"] = preset.ID + TagSeparator + localTag
		frags.DNSServers = append(frags.DNSServers, m)
	}

	return frags, warnings, true
}

// filterActiveVars — оценивает if/if_or каждой var'ы. Возвращает set активных имён.
func filterActiveVars(vars []template.PresetVar, varsMap map[string]string, target template.TargetSpec) map[string]bool {
	out := make(map[string]bool, len(vars))
	// Multi-pass для случая когда if ссылается на var ниже по списку
	// (но since varsMap уже заполнен с default'ами, single-pass достаточно).
	for _, v := range vars {
		// SPEC 107: гейт переменной пресета — #enable + легаси if/if_or.
		if template.NormalizeGate(v.EnableRaw(), v.If, v.IfOr).SatisfiedVars(varsMap, target) {
			out[v.Name] = true
		}
	}
	return out
}

// evalIf — true iff ВСЕ ifList истинны И (ifOr пуст ИЛИ хотя бы одна ifOr истинна).
// Сам факт «var истинна» = varsMap[name] == "true" (case-insensitive).
//
// Пустые ifList+ifOrList → true (фрагмент всегда активен).
//
// SPEC 067 Phase 3: канонический формат имени — "@var" (loader validation требует
// `@`-префикс). Префикс strip'ается перед lookup; bare имена (legacy) тоже
// работают — но валидатор их отвергает на load.
// evalIf — boolean if/if_or evaluation. Single source of truth is
// evalIfWithReason (resolve_dns.go); evalIf just drops the reason string.
func evalIf(ifList, ifOrList []string, varsMap map[string]string) bool {
	ok, _ := evalIfWithReason(ifList, ifOrList, varsMap)
	return ok
}

// extractGateFromMap — гейт map-фрагмента (rules[], dns_rule, dns_rules[]):
// #enable плюс легаси if/if_or, сведённые в одно условие (SPEC 107 §7).
func extractGateFromMap(m map[string]interface{}) *template.GateCond {
	ifList, ifOrList := extractIfFromMap(m)
	return template.NormalizeGate(m[template.GateKey], ifList, ifOrList)
}

// stripGateKeys убирает служебные ключи гейта из КОПИИ фрагмента — до
// подстановки. Иначе обходчик выбросит #enable как неизвестную директиву с
// warning-шумом на каждое правило (ловушка SPEC 107 §11.12).
func stripGateKeys(m map[string]interface{}) {
	delete(m, "if")
	delete(m, "if_or")
	delete(m, template.GateKey)
}

// extractIfFromMap — достаёт if/if_or из map[string]interface{} (для rule/dns_rule).
func extractIfFromMap(m map[string]interface{}) (ifList, ifOrList []string) {
	if raw, ok := m["if"].([]interface{}); ok {
		for _, x := range raw {
			if s, ok := x.(string); ok {
				ifList = append(ifList, s)
			}
		}
	}
	if raw, ok := m["if_or"].([]interface{}); ok {
		for _, x := range raw {
			if s, ok := x.(string); ok {
				ifOrList = append(ifOrList, s)
			}
		}
	}
	return ifList, ifOrList
}

// substitutePresetBody — SPEC 067 Phase 8: единый substitute path для preset
// bodies через template.SubstituteVarsInJSONStrict. Заменил substituteAny —
// устаревший «тупой текстовый» substitute path, не знающий о #if/predicates.
//
// raw — preset fragment (map / slice / scalar — типичные decoded JSON shapes).
// presetVars — описание preset.Vars (для varTypes map).
// varsMap — varsMap{name: scalar string} после filterActiveVars.
//
// Возвращает (substituted, ok). ok=false если в дереве были unresolved @var,
// либо если marshal/unmarshal сломался — caller должен пропустить preset
// целиком (legacy substituteAny semantics).
//
// target — для @runtime.* globals в #if predicates (SPEC 067, SPEC 097).
func substitutePresetBody(raw interface{}, presetVars []template.PresetVar, varsMap map[string]string, target template.TargetSpec) (interface{}, bool) {
	if raw == nil {
		return nil, true
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	// SPEC 106 (G3): walker'у нужны ОБЪЯВЛЕНИЯ всех имён, которые могут
	// встретиться в теле — включая глобальные переменные шаблона,
	// подмешанные в varsMap. Без объявления имя считается неизвестным, и
	// strict-режим роняет весь пресет: именно так traffic-processing падал
	// на @resolve_strategy.
	templateVars := presetVarsToTemplateVarsWithExtras(presetVars, varsMap)
	resolved := varsMapToResolved(varsMap, presetVars)
	out, _, err := template.SubstituteVarsInJSONStrict(data, templateVars, resolved, target)
	if err != nil {
		// UnresolvedVarError → ok=false (callers warn + skip preset).
		return nil, false
	}
	// Decode back via UseNumber to preserve int precision (substitute walker
	// already uses UseNumber internally; here we round-trip via json.Decoder
	// to match that behavior).
	dec := json.NewDecoder(bytes.NewReader(out))
	dec.UseNumber()
	var result interface{}
	if err := dec.Decode(&result); err != nil {
		return nil, false
	}
	return result, true
}

// presetVarsToTemplateVars — converts PresetVar list to TemplateVar list,
// preserving Name and Type so the walker can apply type-based semantics
// (#notEmpty, text_list → list, etc.).
func presetVarsToTemplateVars(vars []template.PresetVar) []template.TemplateVar {
	if len(vars) == 0 {
		return nil
	}
	out := make([]template.TemplateVar, 0, len(vars))
	for _, v := range vars {
		out = append(out, template.TemplateVar{
			Name: v.Name,
			Type: v.Type,
			If:   v.If,
			IfOr: v.IfOr,
		})
	}
	return out
}

// presetVarsToTemplateVarsWithExtras — объявления переменных пресета плюс
// объявления имён, попавших в varsMap извне (глобали шаблона, SPEC 106 G3).
// Тип у внешних имён неизвестен и трактуется как text — для подстановки этого
// достаточно: типизированная семантика (bool/int/text_list) нужна только
// переменным, объявленным в самом пресете.
func presetVarsToTemplateVarsWithExtras(vars []template.PresetVar, varsMap map[string]string) []template.TemplateVar {
	out := presetVarsToTemplateVars(vars)
	declared := make(map[string]bool, len(out))
	for _, v := range out {
		declared[v.Name] = true
	}
	extras := make([]string, 0, len(varsMap))
	for name := range varsMap {
		if !declared[name] {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras) // детерминированный порядок объявлений
	for _, name := range extras {
		out = append(out, template.TemplateVar{Name: name, Type: "text"})
	}
	return out
}

// varsMapToResolved — adapt preset varsMap (name → scalar string) into the
// ResolvedVar form SubstituteVarsInJSON expects. For text_list type, the scalar
// is parsed into r.List (comma/newline-split; mirrors what loader/state does
// for text_list values stored as strings).
//
// Preset vars don't natively store list values — preset.Default is a string
// regardless of type — but text_list values arriving via state may be
// comma-separated. We split on commas; if there are no commas, single-element
// list.
func varsMapToResolved(varsMap map[string]string, presetVars []template.PresetVar) map[string]template.ResolvedVar {
	typeByName := make(map[string]string, len(presetVars))
	for _, v := range presetVars {
		typeByName[v.Name] = v.Type
	}
	out := make(map[string]template.ResolvedVar, len(varsMap))
	for name, scalar := range varsMap {
		rv := template.ResolvedVar{Scalar: scalar}
		if typeByName[name] == "text_list" {
			rv.List = splitTextList(scalar)
		}
		out[name] = rv
	}
	return out
}

// splitTextList — split text_list scalar on commas (trimmed). Empty input →
// empty list (not nil — distinguish "absent" from "explicitly empty list").
func splitTextList(scalar string) []string {
	s := strings.TrimSpace(scalar)
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// rewriteRuleSetRefs — переписывает rule_set refs:
//   - string "local_tag" → "<preset_id>:<local_tag>" если local_tag в validTags;
//     если local_tag НЕ в validTags (dangling после if-filter) — НИЧЕГО не делаем
//     с этим string'ом (caller сам решит что rule пустой — см. isRuleEmpty)
//   - []interface{} с локальными именами → filter+prefix; dangling выкидываются
func rewriteRuleSetRefs(m map[string]interface{}, presetID string, validTags map[string]bool) {
	ref, ok := m["rule_set"]
	if !ok {
		return
	}
	switch v := ref.(type) {
	case string:
		if v == "" {
			return
		}
		if validTags[v] {
			m["rule_set"] = presetID + TagSeparator + v
		} else {
			// Dangling — удалить ключ (isRuleEmpty проверит).
			delete(m, "rule_set")
		}
	case []interface{}:
		out := make([]interface{}, 0, len(v))
		for _, x := range v {
			s, _ := x.(string)
			if s == "" {
				continue
			}
			if validTags[s] {
				out = append(out, presetID+TagSeparator+s)
			}
			// dangling — skip
		}
		if len(out) > 0 {
			m["rule_set"] = out
		} else {
			delete(m, "rule_set")
		}
	}
}

// isRuleEmpty — rule пустой если нет ни rule_set, ни других match-полей.
// Под "другими match-полями" подразумеваются sing-box match-keys (ip_is_private,
// domain_suffix, и т.п.) — то есть всё кроме action/outbound/method/network/if/if_or.
func isRuleEmpty(m map[string]interface{}, _ map[string]bool) bool {
	if m == nil {
		return true
	}
	nonMatchKeys := map[string]bool{
		"outbound": true, "action": true, "method": true,
		"if": true, "if_or": true,
	}
	for k := range m {
		if !nonMatchKeys[k] {
			return false
		}
	}
	return true
}

// isDNSRuleEmpty — dns_rule пустой если нет server или нет rule_set + других match-полей.
func isDNSRuleEmpty(m map[string]interface{}, _ map[string]bool) bool {
	if m == nil {
		return true
	}
	// SPEC 085.1: an action rule (predefined / reject / route-options) is valid
	// WITHOUT a server — e.g. FakeIP's HTTPS/SVCB predefined block. A rule
	// carrying an `action` or a `query_type` matcher is never "empty".
	if _, hasAction := m["action"]; hasAction {
		return false
	}
	if _, hasQT := m["query_type"]; hasQT {
		return false
	}
	if _, ok := m["server"]; !ok {
		return true
	}
	matchFields := 0
	for k := range m {
		if k == "server" || k == "if" || k == "if_or" {
			continue
		}
		matchFields++
	}
	return matchFields == 0
}

// expandOnePresetDNSRule resolves one preset DNS rule map: evaluates its `if`,
// deep-copies, substitutes @vars, strips if/if_or, rewrites rule_set refs, and
// prefixes a bundled server tag. Returns (rule, ok): ok=false when the rule is
// gated off, empty, or hit an unresolved @var — правило выпадает с warning, а
// пресет продолжает собираться (Dropped-каскад §5.1). Shared by the singular
// dns_rule and the plural dns_rules.
func expandOnePresetDNSRule(preset *template.Preset, src map[string]interface{}, varsMap map[string]string, emittedTags map[string]bool, target template.TargetSpec, warnings *[]ExpandWarning) (map[string]interface{}, bool) {
	if !extractGateFromMap(src).SatisfiedVars(varsMap, target) {
		return nil, false
	}
	raw, err := deepCopyMap(src)
	if err != nil {
		*warnings = append(*warnings, ExpandWarning{preset.ID, fmt.Sprintf("deep copy dns_rule: %v", err)})
		return nil, false
	}
	substituted, ok := substitutePresetBody(raw, preset.Vars, varsMap, target)
	if !ok {
		*warnings = append(*warnings, ExpandWarning{preset.ID, "unresolved @var in dns_rule — правило выброшено"})
		return nil, false
	}
	m, _ := substituted.(map[string]interface{})
	delete(m, "if")
	delete(m, "if_or")
	rewriteRuleSetRefs(m, preset.ID, emittedTags)
	// dns_rule.server — может быть локальный bundled tag (без префикса), prefix'ить.
	if srv, ok := m["server"].(string); ok && srv != "" && !strings.HasPrefix(srv, "@") {
		for _, ds := range preset.DNSServers {
			if ds.Tag == srv {
				m["server"] = preset.ID + TagSeparator + srv
				break
			}
		}
	}
	if isDNSRuleEmpty(m, emittedTags) {
		return nil, false
	}
	return m, true
}

// deepCopy — JSON round-trip копия любой структуры.
func deepCopy(in interface{}) (interface{}, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// deepCopyMap — то же что deepCopy но возвращает map[string]interface{}.
func deepCopyMap(in map[string]interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
