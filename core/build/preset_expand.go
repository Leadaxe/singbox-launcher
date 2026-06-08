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
// goos / goarch — для runtime globals @runtime.platform / @runtime.arch в #if predicates
// (SPEC 067). Callers передают runtime.GOOS / runtime.GOARCH; тесты — fakes.
//
// Возвращает (fragments, warnings, ok). ok=false если preset нельзя раскрыть
// (например unresolved @var) — в этом случае fragments частично заполнен,
// но caller должен пропустить preset целиком.
func ExpandPreset(preset *template.Preset, userVars map[string]string, goos, goarch string) (*PresetFragments, []ExpandWarning, bool) {
	if preset == nil {
		return nil, nil, false
	}

	var warnings []ExpandWarning

	// === 1. Build varsMap ===
	varsMap := make(map[string]string, len(preset.Vars))
	for _, v := range preset.Vars {
		if userVal, ok := userVars[v.Name]; ok && userVal != "" {
			varsMap[v.Name] = userVal
		} else {
			varsMap[v.Name] = v.Default
		}
	}

	// === 2. Filter vars by if/if_or (resolve once, may exclude vars from substitute) ===
	activeVars := filterActiveVars(preset.Vars, varsMap)
	// Удаляем неактивные vars из varsMap чтобы substitute @name на них упал → unresolved warning.
	for name := range varsMap {
		if !activeVars[name] {
			delete(varsMap, name)
		}
	}

	frags := &PresetFragments{}

	// === 3. Filter + substitute rule_set ===
	emittedTags := make(map[string]bool) // tag после prefix
	for _, rs := range preset.RuleSet {
		if !evalIf(rs.If, rs.IfOr, varsMap) {
			continue
		}
		raw, err := deepCopy(rs)
		if err != nil {
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("deep copy rule_set %q: %v", rs.Tag, err)})
			continue
		}
		substituted, ok := substitutePresetBody(raw, preset.Vars, varsMap, goos, goarch)
		if !ok {
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("unresolved @var in rule_set %q", rs.Tag)})
			return nil, warnings, false
		}
		m, _ := substituted.(map[string]interface{})
		if m == nil {
			continue
		}
		// Strip if/if_or (уже резолвлено) — не нужны в sing-box config.
		delete(m, "if")
		delete(m, "if_or")
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
		ruleIf, ruleIfOr := extractIfFromMap(ruleMap)
		if !evalIf(ruleIf, ruleIfOr, varsMap) {
			continue
		}
		raw, err := deepCopyMap(ruleMap)
		if err != nil {
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("deep copy rules[%d]: %v", idx, err)})
			continue
		}
		substituted, ok := substitutePresetBody(raw, preset.Vars, varsMap, goos, goarch)
		if !ok {
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("unresolved @var in rules[%d]", idx)})
			return nil, warnings, false
		}
		m, _ := substituted.(map[string]interface{})
		if m == nil {
			continue
		}
		delete(m, "if")
		delete(m, "if_or")
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

	// === 5. Resolve dns_rule ===
	if preset.DNSRule != nil {
		dnsIf, dnsIfOr := extractIfFromMap(preset.DNSRule)
		if evalIf(dnsIf, dnsIfOr, varsMap) {
			raw, err := deepCopyMap(preset.DNSRule)
			if err != nil {
				warnings = append(warnings, ExpandWarning{preset.ID,
					fmt.Sprintf("deep copy dns_rule: %v", err)})
			} else {
				substituted, ok := substitutePresetBody(raw, preset.Vars, varsMap, goos, goarch)
				if !ok {
					warnings = append(warnings, ExpandWarning{preset.ID, "unresolved @var in dns_rule"})
					return nil, warnings, false
				}
				m, _ := substituted.(map[string]interface{})
				delete(m, "if")
				delete(m, "if_or")
				rewriteRuleSetRefs(m, preset.ID, emittedTags)
				// dns_rule.server — может быть локальный bundled tag (без префикса), prefix'ить.
				if srv, ok := m["server"].(string); ok && srv != "" && !strings.HasPrefix(srv, "@") {
					// Check if it matches bundled tag.
					for _, ds := range preset.DNSServers {
						if ds.Tag == srv {
							m["server"] = preset.ID + TagSeparator + srv
							break
						}
					}
				}
				if !isDNSRuleEmpty(m, emittedTags) {
					frags.DNSRule = m
				}
			}
		}
	}

	// === 6. dns_servers — БЕЗ consumption-filter (SPEC 056-R-N follow-up).
	// Все bundled DNS-серверы preset'а (с if/if_or filter) попадают в frags.
	// Per-server enable управляется через state.DNS.Servers[kind=preset].Enabled,
	// который применяется в ResolveDNS → MergePresetsIntoDNS. Здесь только
	// материализуем body + substitute.
	for _, ds := range preset.DNSServers {
		if !evalIf(ds.If, ds.IfOr, varsMap) {
			continue
		}
		raw, err := deepCopy(ds)
		if err != nil {
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("deep copy dns_server %q: %v", ds.Tag, err)})
			continue
		}
		substituted, ok := substitutePresetBody(raw, preset.Vars, varsMap, goos, goarch)
		if !ok {
			warnings = append(warnings, ExpandWarning{preset.ID,
				fmt.Sprintf("unresolved @var in dns_server %q", ds.Tag)})
			return nil, warnings, false
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
func filterActiveVars(vars []template.PresetVar, varsMap map[string]string) map[string]bool {
	out := make(map[string]bool, len(vars))
	// Multi-pass для случая когда if ссылается на var ниже по списку
	// (но since varsMap уже заполнен с default'ами, single-pass достаточно).
	for _, v := range vars {
		if evalIf(v.If, v.IfOr, varsMap) {
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
// goos / goarch — для @runtime.platform / @runtime.arch globals в #if predicates.
func substitutePresetBody(raw interface{}, presetVars []template.PresetVar, varsMap map[string]string, goos, goarch string) (interface{}, bool) {
	if raw == nil {
		return nil, true
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	templateVars := presetVarsToTemplateVars(presetVars)
	resolved := varsMapToResolved(varsMap, presetVars)
	out, _, err := template.SubstituteVarsInJSONStrict(data, templateVars, resolved, goos, goarch)
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
