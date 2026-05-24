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

	// RoutingRule — routing rule (preset.rule после substitute и prefix).
	// nil если rule имеет if=false или после dangling-cleanup стал пустым.
	RoutingRule map[string]interface{}

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
// Возвращает (fragments, warnings, ok). ok=false если preset нельзя раскрыть
// (например unresolved @var) — в этом случае fragments частично заполнен,
// но caller должен пропустить preset целиком.
func ExpandPreset(preset *template.Preset, userVars map[string]string) (*PresetFragments, []ExpandWarning, bool) {
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
		substituted, ok := substituteAny(raw, varsMap)
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

	// === 4. Resolve routing rule ===
	if preset.Rule != nil {
		ruleIf, ruleIfOr := extractIfFromMap(preset.Rule)
		if evalIf(ruleIf, ruleIfOr, varsMap) {
			raw, err := deepCopyMap(preset.Rule)
			if err != nil {
				warnings = append(warnings, ExpandWarning{preset.ID,
					fmt.Sprintf("deep copy rule: %v", err)})
			} else {
				substituted, ok := substituteAny(raw, varsMap)
				if !ok {
					warnings = append(warnings, ExpandWarning{preset.ID, "unresolved @var in rule"})
					return nil, warnings, false
				}
				m, _ := substituted.(map[string]interface{})
				delete(m, "if")
				delete(m, "if_or")
				// Rewrite rule_set refs: local → prefixed, filter dangling.
				rewriteRuleSetRefs(m, preset.ID, emittedTags)
				// Apply outbound sentinels (reject/drop) — shared util с UI.
				if outbound, ok := m["outbound"].(string); ok {
					m = outboundutil.ApplyOutboundToRule(m, outbound)
				}
				if !isRuleEmpty(m, emittedTags) {
					frags.RoutingRule = m
				} else {
					warnings = append(warnings, ExpandWarning{preset.ID,
						"routing rule dropped (no valid rule_set refs after if-filter)"})
				}
			}
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
				substituted, ok := substituteAny(raw, varsMap)
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
		substituted, ok := substituteAny(raw, varsMap)
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
func evalIf(ifList, ifOrList []string, varsMap map[string]string) bool {
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

// substituteAny рекурсивно заменяет строки `@name` на varsMap[name].
// Возвращает (result, ok). ok=false если найдена строка `@name` для name,
// которого нет в varsMap (unresolved → skip preset).
//
// Тупой текстовый замен: только полное равенство "@name". Embed'ы в подстроку
// типа "prefix@name" не поддерживаются (YAGNI).
func substituteAny(obj interface{}, vars map[string]string) (interface{}, bool) {
	switch v := obj.(type) {
	case string:
		if !strings.HasPrefix(v, "@") {
			return v, true
		}
		name := v[1:]
		if name == "" {
			return v, true
		}
		val, exists := vars[name]
		if !exists {
			return nil, false
		}
		return val, true

	case map[string]interface{}:
		for k, val := range v {
			rep, ok := substituteAny(val, vars)
			if !ok {
				return nil, false
			}
			v[k] = rep
		}
		return v, true

	case []interface{}:
		for i, val := range v {
			rep, ok := substituteAny(val, vars)
			if !ok {
				return nil, false
			}
			v[i] = rep
		}
		return v, true

	default:
		return obj, true
	}
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

// collectConsumedBundledDNSTags — УДАЛЕНА в SPEC 056-R-N follow-up.
// Consumption-filter заменён per-server enable toggle в state.DNS.Servers
// (см. ResolveDNS + MergePresetsIntoDNS).

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
