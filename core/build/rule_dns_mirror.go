package build

// DNS-зеркало собственного правила маршрута (SPEC 172, контракт 1.1.130,
// D-137, TEMPLATE_LANG §6.11).
//
// Запись правила (rules[] inline|srs) может нести объект `dns`: «DNS-запросы
// под это правило отвечает такой-то сервер» и «AAAA под это правило — пустым
// ответом». Зеркала в state.DNS.Rules не хранятся — их выводит сборка из
// ВЫПУЩЕННЫХ маршрутных правил: правило выключено или наборы srs не скачаны —
// зеркала нет, и ссылка на rule_set в нём всегда указывает на выпущенный
// набор.
//
// Матчер и гейт — списки реестра (allowlists.json `rule_dns_mirror_match`,
// `rule_dns_mirror_gate`): имён полей правила и видов записи здесь нет.

import (
	"sort"

	"singbox-launcher/core/config/registry"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// Имена списков реестра для DNS-зеркала правила.
const (
	ruleDNSMirrorMatchList = "rule_dns_mirror_match"
	ruleDNSMirrorGateList  = "rule_dns_mirror_gate"
)

// Тело глушилки AAAA: пустой авторитетный ответ, приложение берёт A.
const (
	ruleDNSForceIPv4Version = 6
	ruleDNSForceIPv4Action  = "predefined"
	ruleDNSForceIPv4Rcode   = "NOERROR"
)

// RuleDNSMirrorMatch — матчер DNS-зеркала по телу маршрутного правила (у srs —
// с вписанным сборкой rule_set). ok == false — зеркала нет: тело несёт ключ
// гейта или ни одного ключа матчера. Окно правила спрашивает то же самое,
// чтобы опция была недоступна ровно там, где сборка её не выпустит.
func RuleDNSMirrorMatch(routeBody map[string]interface{}) (match map[string]interface{}, ok bool) {
	reg, err := registry.Get()
	if err != nil || routeBody == nil {
		return nil, false
	}
	for _, k := range reg.Allowlist(ruleDNSMirrorGateList) {
		if v, has := routeBody[k]; has && !isZeroJSONValue(v) {
			return nil, false
		}
	}
	match = make(map[string]interface{})
	for _, k := range reg.Allowlist(ruleDNSMirrorMatchList) {
		if v, has := routeBody[k]; has && !isZeroJSONValue(v) {
			match[k] = v
		}
	}
	if len(match) == 0 {
		return nil, false
	}
	return match, true
}

// ruleDNSMirror — DNS-правила одного маршрутного правила: глушилка AAAA и/или
// зеркало на сервер, в этом порядке.
type ruleDNSMirror struct {
	Num  int
	Name string
	// ForceIPv4 — тело глушилки; nil — опция выключена.
	ForceIPv4 map[string]interface{}
	// Server — тело зеркала БЕЗ server; nil — опция выключена.
	Server    map[string]interface{}
	ServerTag string
}

// collectRuleDNSMirrors — зеркала выпущенных маршрутных правил в порядке оси.
func collectRuleDNSMirrors(route ResolvedRoute) []ruleDNSMirror {
	var out []ruleDNSMirror
	for _, r := range route.Rules {
		if r.DNS.IsZero() || !r.Active || !r.Enabled {
			continue
		}
		if r.Source != RouteSourceInline && r.Source != RouteSourceSrs {
			continue
		}
		match, ok := RuleDNSMirrorMatch(r.Body)
		if !ok {
			continue
		}
		m := ruleDNSMirror{Num: r.Num, Name: r.Name}
		if r.DNS.ForceIPv4 {
			m.ForceIPv4 = cloneDNSRuleMap(match)
			m.ForceIPv4["ip_version"] = ruleDNSForceIPv4Version
			m.ForceIPv4["action"] = ruleDNSForceIPv4Action
			m.ForceIPv4["rcode"] = ruleDNSForceIPv4Rcode
		}
		if r.DNS.Enabled && r.DNS.ServerTag != "" {
			m.Server = cloneDNSRuleMap(match)
			m.ServerTag = r.DNS.ServerTag
		}
		if m.ForceIPv4 == nil && m.Server == nil {
			continue
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Num < out[j].Num })
	return out
}

// RuleDNSMirror — DNS-правила одного маршрутного правила для показа (вкладка
// Preview окна правила): глушилка AAAA и/или зеркало, в порядке выпуска.
// ServerTag — тег сервера зеркала; есть ли он в конфиге, решает сборка.
type RuleDNSMirror struct {
	Rules     []map[string]interface{}
	ServerTag string
}

// RuleDNSMirrors — зеркала выпущенных правил разрешённого маршрута тем же
// кодом, что у сборки (collectRuleDNSMirrors), в порядке оси.
func RuleDNSMirrors(route ResolvedRoute) []RuleDNSMirror {
	var out []RuleDNSMirror
	for _, m := range collectRuleDNSMirrors(route) {
		var v RuleDNSMirror
		if m.ForceIPv4 != nil {
			v.Rules = append(v.Rules, m.ForceIPv4)
		}
		if m.Server != nil {
			body := cloneDNSRuleMap(m.Server)
			body["server"] = m.ServerTag
			v.Rules = append(v.Rules, body)
			v.ServerTag = m.ServerTag
		}
		out = append(out, v)
	}
	return out
}

// ruleDNSMirrorsForMerge — зеркала правил контекста сборки. Маршрут
// разрешается здесь же (как в MergePresetsIntoRoute) только когда хоть одно
// правило несёт опцию: без неё сборка DNS остаётся прежней.
func ruleDNSMirrorsForMerge(ctx PresetMergeContext) []ruleDNSMirror {
	has := false
	for i := range ctx.Rules {
		if !ctx.Rules[i].DNS.IsZero() {
			has = true
			break
		}
	}
	if !has {
		return nil
	}
	st := &corestate.State{Rules: ctx.Rules, DNS: ctx.DNS}
	td := template.TemplateData{Presets: ctx.Presets, Vars: ctx.TemplateVars, PresetNodes: ctx.PresetNodes}
	return collectRuleDNSMirrors(ResolveRouteWithGlobals(st, &td, ctx.DataDir, ctx.SrsCachedPaths, ctx.Target, ctx.globalVarValues()))
}

// ruleDNSMirrorServers — теги серверов, на которые ссылаются зеркала.
func ruleDNSMirrorServers(mirrors []ruleDNSMirror) map[string]bool {
	out := make(map[string]bool)
	for _, m := range mirrors {
		if m.Server != nil {
			out[m.ServerTag] = true
		}
	}
	return out
}

// RuleDNSMirrorIndex — место пары DNS-правил зеркала с номером num среди n
// DNS-правил (TEMPLATE_LANG §6.11): перед первым слотом пресета, чей номер
// больше num; такого нет — сразу после последнего слота пресета; слотов нет —
// n. slotNum(i) — номер слота, если правило i начинает слот пресета;
// inSlot(i) — правило i принадлежит слоту пресета.
func RuleDNSMirrorIndex(n, num int, slotNum func(i int) (int, bool), inSlot func(i int) bool) int {
	after := -1
	for i := 0; i < n; i++ {
		if got, ok := slotNum(i); ok && got > num {
			return i
		}
		if inSlot(i) {
			after = i + 1
		}
	}
	if after >= 0 {
		return after
	}
	return n
}

// insertRuleDNSMirrors — разрешённые DNS-правила с зеркалами на их местах.
// Слот пресета — подряд идущие правила одного PresetID; его номер — номер
// маршрутного правила пресета (presetNums).
func insertRuleDNSMirrors(rules []ResolvedDNSRule, mirrors []ResolvedDNSRule, mirrorNums []int, presetNums map[string]int) []ResolvedDNSRule {
	if len(mirrors) == 0 {
		return rules
	}
	slotNum := func(i int) (int, bool) {
		r := rules[i]
		if r.Source != DNSSourcePreset {
			return 0, false
		}
		if i > 0 && rules[i-1].Source == DNSSourcePreset && rules[i-1].PresetID == r.PresetID {
			return 0, false
		}
		if n, ok := presetNums[r.PresetID]; ok {
			return n, true
		}
		return corestate.DefaultRuleNum, true
	}
	inSlot := func(i int) bool { return rules[i].Source == DNSSourcePreset }

	at := make(map[int][]ResolvedDNSRule)
	for i, m := range mirrors {
		pos := RuleDNSMirrorIndex(len(rules), mirrorNums[i], slotNum, inSlot)
		at[pos] = append(at[pos], m)
	}
	out := make([]ResolvedDNSRule, 0, len(rules)+len(mirrors))
	for i := 0; i <= len(rules); i++ {
		out = append(out, at[i]...)
		if i < len(rules) {
			out = append(out, rules[i])
		}
	}
	return out
}
