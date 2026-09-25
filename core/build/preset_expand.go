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
// Подстановка — канонический обходчик (SPEC 143): объявленное имя без
// значения даёт Dropped ключа (§5.1 TEMPLATE_LANG), после каскада фрагмент
// проверяется гейтами валидности (fragmentGate*), и невалидный выпадает с
// кодом реестра template_fragment_dropped — не молча и не пресетом целиком.
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
//
// Code/Params — код contract/registry/warnings.json и его параметры: такое
// предупреждение сборка кладёт в отчёт видом template_degraded
// (ExpandWarning.TemplateWarning). Без кода — внутренняя диагностика, только
// лог.
type ExpandWarning struct {
	PresetID string
	Message  string
	Code     string
	Params   map[string]string
}

func (w ExpandWarning) String() string {
	if w.Code != "" {
		return fmt.Sprintf("preset %q: %s %v", w.PresetID, w.Code, w.Params)
	}
	return fmt.Sprintf("preset %q: %s", w.PresetID, w.Message)
}

// TemplateWarning — запись для отчёта сборки; ok=false у предупреждения без
// кода реестра.
func (w ExpandWarning) TemplateWarning() (template.TemplateWarning, bool) {
	if w.Code == "" {
		return template.TemplateWarning{}, false
	}
	return template.TemplateWarning{Code: w.Code, Params: w.Params}, true
}

// templateWarning — запись отчёта у предупреждения, которое заведомо с кодом.
func (w ExpandWarning) templateWarning() template.TemplateWarning {
	return template.TemplateWarning{Code: w.Code, Params: w.Params}
}

// WarnTemplateFragmentDropped — фрагмент шаблона (тело пресета или шаблонный
// DNS-сервер) после подстановки остался без обязательного поля и не включён
// в конфиг (SPEC 143 Т3). Параметры: owner — id пресета или "dns_options",
// kind — секция конфига фрагмента, reason — недостающее поле.
const WarnTemplateFragmentDropped = "template_fragment_dropped"

// Секции конфига фрагментов — значения параметра kind.
const (
	fragmentKindRuleSet   = "route.rule_set"
	fragmentKindRule      = "route.rules"
	fragmentKindDNSRule   = "dns.rules"
	fragmentKindDNSServer = "dns.servers"
)

// fragmentDropped — предупреждение о выпавшем фрагменте с кодом реестра.
func fragmentDropped(owner, kind, reason string) ExpandWarning {
	return ExpandWarning{
		PresetID: owner,
		Message:  fmt.Sprintf("%s entry dropped: no %s after substitution", kind, reason),
		Code:     WarnTemplateFragmentDropped,
		Params:   map[string]string{"owner": owner, "kind": kind, "reason": reason},
	}
}

// substitutionWarnings — предупреждения канонического обходчика в форме
// ExpandWarning: один канал для всего, что пресет сообщает сборке.
func substitutionWarnings(presetID string, ws []template.TemplateWarning) []ExpandWarning {
	out := make([]ExpandWarning, 0, len(ws))
	for _, w := range ws {
		out = append(out, ExpandWarning{PresetID: presetID, Message: w.Code, Code: w.Code, Params: w.Params})
	}
	return out
}

// expandTemplateWarnings — записи отчёта из предупреждений раскрытия: только
// с кодом реестра; остальные остаются в логе.
func expandTemplateWarnings(ws []ExpandWarning) []template.TemplateWarning {
	var out []template.TemplateWarning
	for _, w := range ws {
		if tw, ok := w.TemplateWarning(); ok {
			out = append(out, tw)
		}
	}
	return out
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
	return ExpandPresetWithGlobals(preset, userVars, nil, nil, target)
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
// globalDecls — ОБЪЯВЛЕНИЯ переменных шаблона (td.Vars, SPEC 143 Т2):
// обходчику объявляются все они, а не только имена со значением. Глобаль с
// пустым значением VarValuesFor в globalVars не кладёт; объявленная, она даёт
// Dropped ключа (`"strategy": "@resolve_strategy"` → правило без strategy), а
// не литерал "@resolve_strategy" в конфиге. nil — объявлены только пресет и
// имена из globalVars (тип text).
func ExpandPresetWithGlobals(
	preset *template.Preset,
	userVars map[string]string,
	globalVars map[string]string,
	globalDecls []template.TemplateVar,
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
		// Пусто и обязательна — значения нет. Пресет при этом НЕ
		// выбрасывается целиком (D-011): ключи со ссылкой на неё выпадают
		// (Dropped, §5.1), а фрагмент, оставшийся без обязательного поля,
		// снимает гейт валидности со своим кодом.
		if v.Required {
			warnings = append(warnings, ExpandWarning{PresetID: preset.ID,
				Message: fmt.Sprintf("required var %q is empty — keys using it are dropped", v.Name)})
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
			warnings = append(warnings, ExpandWarning{PresetID: preset.ID,
				Message: fmt.Sprintf("deep copy rule_set %q: %v", rs.Tag, err)})
			continue
		}
		substituted, subWarns, ok := substitutePresetBody(raw, preset.Vars, globalDecls, varsMap, target)
		warnings = append(warnings, substitutionWarnings(preset.ID, subWarns)...)
		if !ok {
			warnings = append(warnings, ExpandWarning{PresetID: preset.ID,
				Message: fmt.Sprintf("substitution in rule_set %q failed — fragment dropped", rs.Tag)})
			continue
		}
		m, _ := substituted.(map[string]interface{})
		if m == nil {
			continue
		}
		// Strip служебные ключи гейта (уже резолвлены) — sing-box их не знает.
		stripGateKeys(m)
		// Гейт после Dropped-каскада: набор без источника правил ядро не
		// загрузит. Выпадает ОДИН фрагмент, а не пресет целиком (§5.1).
		if reason := ruleSetMissingSource(m); reason != "" {
			warnings = append(warnings, fragmentDropped(preset.ID, fragmentKindRuleSet, reason))
			continue
		}
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
			warnings = append(warnings, ExpandWarning{PresetID: preset.ID,
				Message: fmt.Sprintf("deep copy rules[%d]: %v", idx, err)})
			continue
		}
		substituted, subWarns, ok := substitutePresetBody(raw, preset.Vars, globalDecls, varsMap, target)
		warnings = append(warnings, substitutionWarnings(preset.ID, subWarns)...)
		if !ok {
			warnings = append(warnings, ExpandWarning{PresetID: preset.ID,
				Message: fmt.Sprintf("substitution in rules[%d] failed — rule dropped", idx)})
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
		// Гейты после Dropped-каскада (§5.1): без цели правило ядру не
		// нужно, без условий — матчило бы весь трафик.
		switch {
		case len(m) == 0:
			// Всё правило — ветка #if с ложным условием: его выключил автор
			// шаблона, это не деградация (кода нет, только лог).
			warnings = append(warnings, ExpandWarning{PresetID: preset.ID,
				Message: fmt.Sprintf("rules[%d] is empty after #if — skipped", idx)})
		case isRuleUnusable(m):
			warnings = append(warnings, fragmentDropped(preset.ID, fragmentKindRule, "outbound/action"))
		case isRuleEmpty(m, emittedTags):
			warnings = append(warnings, fragmentDropped(preset.ID, fragmentKindRule, "rule_set"))
		default:
			frags.RoutingRules = append(frags.RoutingRules, m)
		}
	}

	// === 5. Resolve dns_rule (singular) + dns_rules (plural, SPEC 085.1) ===
	if preset.DNSRule != nil {
		if m, ok := expandOnePresetDNSRule(preset, preset.DNSRule, globalDecls, varsMap, emittedTags, target, &warnings); ok {
			frags.DNSRule = m
		}
	}
	for _, dr := range preset.DNSRules {
		if dr == nil {
			continue
		}
		if m, ok := expandOnePresetDNSRule(preset, dr, globalDecls, varsMap, emittedTags, target, &warnings); ok {
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
			warnings = append(warnings, ExpandWarning{PresetID: preset.ID,
				Message: fmt.Sprintf("deep copy dns_server %q: %v", ds.Tag, err)})
			continue
		}
		substituted, subWarns, ok := substitutePresetBody(raw, preset.Vars, globalDecls, varsMap, target)
		warnings = append(warnings, substitutionWarnings(preset.ID, subWarns)...)
		if !ok {
			warnings = append(warnings, ExpandWarning{PresetID: preset.ID,
				Message: fmt.Sprintf("substitution in dns_server %q failed — server dropped", ds.Tag)})
			continue
		}
		m, _ := substituted.(map[string]interface{})
		if m == nil {
			continue
		}
		// Гейт после Dropped-каскада: сервер без адреса ядро отвергнет.
		if dnsServerMissingAddress(m) {
			warnings = append(warnings, fragmentDropped(preset.ID, fragmentKindDNSServer, "server"))
			continue
		}
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

// substitutePresetBody — подстановка в тело фрагмента пресета каноническим
// обходчиком (SPEC 143; до него строгий режим ронял фрагмент целиком на
// любом unresolved).
//
// raw — фрагмент (map / slice / scalar — типичные decoded JSON shapes).
// presetVars — объявления пресета; globalDecls — объявления шаблона (td.Vars);
// varsMap — значения после filterActiveVars (пресет + глобали).
//
// Объявленное имя без значения даёт Dropped ключа или элемента (§5.1),
// необъявленное остаётся плейсхолдером с warning template_var_undeclared.
// Возвращает дерево, предупреждения обходчика с параметрами и ok=false только
// на сломанном marshal/unmarshal.
//
// target — для @runtime.* globals (SPEC 067, SPEC 097).
func substitutePresetBody(raw interface{}, presetVars []template.PresetVar, globalDecls []template.TemplateVar, varsMap map[string]string, target template.TargetSpec) (interface{}, []template.TemplateWarning, bool) {
	if raw == nil {
		return nil, nil, true
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, nil, false
	}
	decls, resolved := presetSubstitutionScope(presetVars, globalDecls, varsMap)
	out, warns, err := template.SubstituteVarsInJSONCanonWarnings(data, decls, resolved, target)
	if err != nil {
		return nil, nil, false
	}
	// Decode back via UseNumber to preserve int precision (обходчик внутри
	// читает так же).
	dec := json.NewDecoder(bytes.NewReader(out))
	dec.UseNumber()
	var result interface{}
	if err := dec.Decode(&result); err != nil {
		return nil, nil, false
	}
	return result, warns, true
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

// presetSubstitutionScope — объявления и значения для обходчика тела пресета.
//
// Объявления (SPEC 143 Т2): переменные пресета, затем ВСЕ переменные шаблона
// (globalDecls) — одноимённая переменная пресета сильнее, — затем имена из
// varsMap, не объявленные ни там, ни там (глобали без объявлений у
// вызывающего, тип text). Полный список нужен ради Dropped: глобаль с пустым
// значением в varsMap не попадает, и необъявленной она осталась бы
// плейсхолдером "@name" в конфиге.
//
// Значения — только из varsMap. text_list пресета хранится строкой через
// запятую (как его default), глобали — через перевод строки (VarValuesFor).
func presetSubstitutionScope(presetVars []template.PresetVar, globalDecls []template.TemplateVar, varsMap map[string]string) ([]template.TemplateVar, map[string]template.ResolvedVar) {
	decls := presetVarsToTemplateVars(presetVars)
	local := make(map[string]bool, len(decls))
	for _, v := range decls {
		local[v.Name] = true
	}
	declared := make(map[string]bool, len(decls)+len(globalDecls))
	for name := range local {
		declared[name] = true
	}
	for _, g := range globalDecls {
		if g.Separator || g.Name == "" || declared[g.Name] {
			continue
		}
		declared[g.Name] = true
		decls = append(decls, template.TemplateVar{Name: g.Name, Type: g.Type})
	}
	extras := make([]string, 0, len(varsMap))
	for name := range varsMap {
		if !declared[name] {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras) // детерминированный порядок объявлений
	for _, name := range extras {
		decls = append(decls, template.TemplateVar{Name: name, Type: "text"})
	}

	typeByName := make(map[string]string, len(decls))
	for _, v := range decls {
		typeByName[v.Name] = v.Type
	}
	resolved := make(map[string]template.ResolvedVar, len(varsMap))
	for name, scalar := range varsMap {
		rv := template.ResolvedVar{Scalar: scalar}
		if typeByName[name] == "text_list" {
			if local[name] {
				rv.List = splitTextList(scalar)
			} else {
				rv.List = splitTextListLines(scalar)
			}
		}
		resolved[name] = rv
	}
	return decls, resolved
}

// splitTextListLines — text_list глобали: значение строкой через перевод
// строки, как его отдаёт VarValuesFor; пустые строки отбрасываются.
func splitTextListLines(scalar string) []string {
	out := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(scalar, "\r\n", "\n"), "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
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

// ── Гейты валидности фрагмента после Dropped-каскада (SPEC 143 Т3,
// TEMPLATE_LANG §5.1). Подстановка выбрасывает ключ, значения которому нет;
// фрагмент без обязательного поля ядро отвергает целиком, поэтому такой
// фрагмент выпадает сам — с кодом template_fragment_dropped. Одно место для
// всех четырёх видов: правила маршрута, DNS-правила, наборы правил и
// DNS-серверы (пресетные и шаблонные).

// nonEmptyString — ключ есть и несёт непустую строку.
func nonEmptyString(m map[string]interface{}, key string) bool {
	s, _ := m[key].(string)
	return strings.TrimSpace(s) != ""
}

// isRuleUnusable — правило маршрута без outbound и без action: ядру некуда
// направить совпавший трафик.
func isRuleUnusable(m map[string]interface{}) bool {
	return !nonEmptyString(m, "outbound") && !nonEmptyString(m, "action")
}

// isDNSRuleUnusable — DNS-правило без server и без action (serverless-action
// вроде predefined/reject сервер не требует).
func isDNSRuleUnusable(m map[string]interface{}) bool {
	return !nonEmptyString(m, "server") && !nonEmptyString(m, "action")
}

// ruleSetMissingSource — поле-источник, которого набору правил не хватает;
// "" — набор годен. remote берёт правила по url, local — из path, inline —
// из rules; набор без type обязан нести хоть один источник.
func ruleSetMissingSource(m map[string]interface{}) string {
	switch t, _ := m["type"].(string); t {
	case "remote":
		if !nonEmptyString(m, "url") {
			return "url"
		}
	case "local":
		if !nonEmptyString(m, "path") {
			return "path"
		}
	case "inline":
		if _, ok := m["rules"]; !ok {
			return "rules"
		}
	default:
		_, hasRules := m["rules"]
		if !hasRules && !nonEmptyString(m, "url") && !nonEmptyString(m, "path") {
			return "url/path"
		}
	}
	return ""
}

// dnsServerAddressTypes — типы DNS-серверов sing-box, которым нужен адрес
// (`server`). local/fakeip/dhcp/hosts/resolved/tailscale работают без него.
var dnsServerAddressTypes = map[string]bool{
	"udp": true, "tcp": true, "tls": true, "https": true, "quic": true, "h3": true,
}

// dnsServerMissingAddress — сервер без адреса после подстановки. Легаси-форма
// без type адресуется полем address.
func dnsServerMissingAddress(m map[string]interface{}) bool {
	t, _ := m["type"].(string)
	if t == "" {
		return !nonEmptyString(m, "address")
	}
	return dnsServerAddressTypes[t] && !nonEmptyString(m, "server")
}

// expandOnePresetDNSRule resolves one preset DNS rule map: evaluates its `if`,
// deep-copies, substitutes @vars, strips if/if_or, rewrites rule_set refs, and
// prefixes a bundled server tag. Returns (rule, ok): ok=false when the rule is
// gated off, empty, or hit an unresolved @var — правило выпадает с warning, а
// пресет продолжает собираться (Dropped-каскад §5.1). Shared by the singular
// dns_rule and the plural dns_rules.
func expandOnePresetDNSRule(preset *template.Preset, src map[string]interface{}, globalDecls []template.TemplateVar, varsMap map[string]string, emittedTags map[string]bool, target template.TargetSpec, warnings *[]ExpandWarning) (map[string]interface{}, bool) {
	if !extractGateFromMap(src).SatisfiedVars(varsMap, target) {
		return nil, false
	}
	raw, err := deepCopyMap(src)
	if err != nil {
		*warnings = append(*warnings, ExpandWarning{PresetID: preset.ID, Message: fmt.Sprintf("deep copy dns_rule: %v", err)})
		return nil, false
	}
	substituted, subWarns, ok := substitutePresetBody(raw, preset.Vars, globalDecls, varsMap, target)
	*warnings = append(*warnings, substitutionWarnings(preset.ID, subWarns)...)
	if !ok {
		*warnings = append(*warnings, ExpandWarning{PresetID: preset.ID, Message: "substitution in dns_rule failed — rule dropped"})
		return nil, false
	}
	m, _ := substituted.(map[string]interface{})
	if m == nil {
		return nil, false
	}
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
	// Правило целиком — ветка #if с ложным условием: выключено автором
	// шаблона, не деградация.
	if len(m) == 0 {
		return nil, false
	}
	// Гейты после Dropped-каскада (§5.1): без сервера и action правило
	// ядру не нужно, без условий — перехватывало бы все запросы.
	if isDNSRuleUnusable(m) {
		*warnings = append(*warnings, fragmentDropped(preset.ID, fragmentKindDNSRule, "server/action"))
		return nil, false
	}
	if isDNSRuleEmpty(m, emittedTags) {
		*warnings = append(*warnings, fragmentDropped(preset.ID, fragmentKindDNSRule, "rule_set"))
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
