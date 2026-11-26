package build

// DNS-зеркало собственного правила маршрута (SPEC 172, контракт 1.1.130,
// D-137, TEMPLATE_LANG §6.11).
//
// Запись правила (rules[] inline|srs) может нести объект `dns`: «DNS-запросы
// под это правило отвечает такой-то сервер», «AAAA под это правило — пустым
// ответом» и «HTTPS/SVCB под это правило — пустым ответом» (контракт 1.1.173). Зеркала в state.DNS.Rules не хранятся — их выводит сборка из
// ВЫПУЩЕННЫХ маршрутных правил: правило выключено или наборы srs не скачаны —
// зеркала нет, и ссылка на rule_set в нём всегда указывает на выпущенный
// набор.
//
// Матчер — список реестра (allowlists.json `rule_dns_mirror_match`): имён
// полей правила здесь нет. Гейта по портам, протоколам и network нет с
// контракта 1.1.173: такие ключи (`rule_dns_mirror_unseen`) в зеркало просто
// не идут — DNS-опции действуют на домены правила при любом порте.

import (
	"fmt"
	"sort"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// Имена списков реестра для DNS-зеркала правила.
const (
	ruleDNSMirrorMatchList  = "rule_dns_mirror_match"
	ruleDNSMirrorUnseenList = "rule_dns_mirror_unseen"
	ruleDNSBlockHTTPSList   = "rule_dns_block_https_domains"
)

// Типы запросов, которые глушит Block HTTPS (контракт 1.1.173, §6.11).
var ruleDNSBlockHTTPSQueryTypes = []interface{}{"HTTPS", "SVCB"}

// Тело глушилки AAAA: пустой авторитетный ответ, приложение берёт A.
const (
	ruleDNSForceIPv4Version = 6
	ruleDNSForceIPv4Action  = "predefined"
	ruleDNSForceIPv4Rcode   = "NOERROR"
)

// Локальный ответ фиксированными адресами (контракт 1.1.139, §6.11): запись
// с именем `*.` ядро подставляет под имя запроса (rewriteRecords).
const ruleDNSFixedTTL = 300

// RuleDNSMirrorMatch — матчер DNS-зеркала по телу правила (тело записи с
// refs или выпущенное, простое или дерево — §6.11, обход RuleDNSMirrorForBody).
// ok == false — зеркала нет: ни в одном узле дерева нет условия, видимого
// DNS-слою. Окно правила спрашивает то же самое, что выпускает сборка.
func RuleDNSMirrorMatch(routeBody map[string]interface{}) (match map[string]interface{}, ok bool) {
	m, _ := RuleDNSMirrorForBody(routeBody)
	return m, m != nil
}

// RuleDNSBlockHTTPSAvailable — сборка выпустит Block HTTPS у правила с таким
// телом: матчер зеркала не пуст и хоть в одном простом узле дерева есть
// доменный ключ или ссылка на набор (§6.11). srs — UI-вид правила, у которого
// ссылки сняты с тела (наборы — отдельным полем формы).
func RuleDNSBlockHTTPSAvailable(routeBody map[string]interface{}, srs bool) bool {
	m, domains := RuleDNSMirrorForBody(routeBody)
	return m != nil && (srs || domains)
}

// RuleDNSMirrorNarrowed — хоть один узел дерева несёт условие, которого
// DNS-слой не видит (порт, протокол, network — список реестра
// `rule_dns_mirror_unseen`): DNS-опции правила действуют шире маршрутного
// правила, окно правила подсказывает это.
func RuleDNSMirrorNarrowed(routeBody map[string]interface{}) bool {
	keys := loadRuleTreeKeys()
	found := false
	corestate.WalkRuleTree(routeBody, func(_ []int, node map[string]interface{}, logical bool) bool {
		if !logical && nodeHasAnyKey(node, keys.unseen) {
			found = true
			return false
		}
		return true
	})
	return found
}

// ruleDNSMirrorBody — DNS-правило зеркала: матчер M + условие действия cond +
// ключи действия. Ядро у logical не принимает query_type / ip_version, и
// invert простого правила отрицал бы и условие действия, поэтому такой M
// оборачивается в and из M и cond (§6.11); простой M без invert — ключи
// рядом, как до 1.1.179 (config.json плоских правил прежний).
func ruleDNSMirrorBody(match, cond, action map[string]interface{}) map[string]interface{} {
	inv, _ := match[ruleTreeKeyInvert].(bool)
	var body map[string]interface{}
	if len(cond) > 0 && (corestate.IsLogicalRule(match) || inv) {
		body = map[string]interface{}{
			ruleTreeKeyType:  ruleTreeLogical,
			ruleTreeKeyMode:  ruleTreeModeAnd,
			ruleTreeKeyRules: []interface{}{cloneDNSRuleMap(match), cond},
		}
	} else {
		body = cloneDNSRuleMap(match)
		for k, v := range cond {
			body[k] = v
		}
	}
	for k, v := range action {
		body[k] = v
	}
	return body
}

// ruleDNSMirror — DNS-правила одного маршрутного правила в порядке выпуска:
// Block HTTPS, затем глушилка AAAA и зеркало на сервер либо локальный ответ.
type ruleDNSMirror struct {
	Num  int
	Name string
	// BlockHTTPS — тело глушилки HTTPS/SVCB; nil — опции нет или правилу она
	// недоступна. Стоит первой и не зависит от режима ответа.
	BlockHTTPS map[string]interface{}
	// ForceIPv4 — тело глушилки; nil — опция выключена.
	ForceIPv4 map[string]interface{}
	// Fixed — локальный ответ адресами: два тела, A и AAAA; nil — режим не
	// действует. При нём глушилки и зеркала на сервер нет.
	Fixed []map[string]interface{}
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
		match := r.Mirror
		if match == nil {
			continue
		}
		m := ruleDNSMirror{Num: r.Num, Name: r.Name}
		predefined := map[string]interface{}{"action": ruleDNSForceIPv4Action, "rcode": ruleDNSForceIPv4Rcode}
		if r.DNS.BlockHTTPS && r.MirrorDomains {
			m.BlockHTTPS = ruleDNSMirrorBody(match,
				map[string]interface{}{"query_type": append([]interface{}(nil), ruleDNSBlockHTTPSQueryTypes...)}, predefined)
		}
		if r.DNS.FixedActive() {
			v4, v6 := r.DNS.FixedAddrs()
			m.Fixed = []map[string]interface{}{
				ruleDNSFixedBody(match, "A", v4),
				ruleDNSFixedBody(match, "AAAA", v6),
			}
			out = append(out, m)
			continue
		}
		if r.DNS.ForceIPv4 {
			m.ForceIPv4 = ruleDNSMirrorBody(match,
				map[string]interface{}{"ip_version": ruleDNSForceIPv4Version}, predefined)
		}
		if r.DNS.Enabled && r.DNS.ServerTag != "" {
			m.Server = cloneDNSRuleMap(match)
			m.ServerTag = r.DNS.ServerTag
		}
		if m.BlockHTTPS == nil && m.ForceIPv4 == nil && m.Server == nil {
			continue
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Num < out[j].Num })
	return out
}

// ruleDNSFixedBody — DNS-правило локального ответа одного семейства: матч
// зеркала, query_type, predefined NOERROR и записи `*. 300 IN <qtype> <ip>`;
// без адресов — пустой NOERROR (приложение берёт другое семейство).
func ruleDNSFixedBody(match map[string]interface{}, qtype string, addrs []string) map[string]interface{} {
	action := map[string]interface{}{"action": ruleDNSForceIPv4Action, "rcode": ruleDNSForceIPv4Rcode}
	if len(addrs) > 0 {
		answer := make([]interface{}, 0, len(addrs))
		for _, ip := range addrs {
			answer = append(answer, fmt.Sprintf("*. %d IN %s %s", ruleDNSFixedTTL, qtype, ip))
		}
		action["answer"] = answer
	}
	return ruleDNSMirrorBody(match, map[string]interface{}{"query_type": []interface{}{qtype}}, action)
}

// RuleDNSMirror — DNS-правила одного маршрутного правила для показа (вкладка
// Preview окна правила): Block HTTPS, глушилка AAAA, зеркало либо локальный
// ответ — в порядке выпуска.
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
		if m.BlockHTTPS != nil {
			v.Rules = append(v.Rules, m.BlockHTTPS)
		}
		v.Rules = append(v.Rules, m.Fixed...)
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
	return collectRuleDNSMirrors(ResolveRouteTagged(st, &td, ctx.DataDir, ctx.SrsCachedPaths, ctx.Target, ctx.globalVarValues(), ctx.FinalOutboundTags, ctx.TemplateRouteRuleSetTags))
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
