package build

// Выпуск собственного правила-дерева (SPEC 188 волна 3, TEMPLATE_LANG §6.13 и
// §6.11, контракт 1.1.179–1.1.180).
//
// Тело записи inline|srs — правило sing-box, простое или `logical`-дерево;
// ссылки на наборы (`refs`) — ключ простого узла на любом уровне. Обход:
//
//  1. каждая ссылка узла — запись route.rule_set (local из копии | remote по
//     политике правила); одна ссылка дерева — одна запись;
//  2. у узла со ссылками ключи «в набор» таблицы условий реестра уходят в
//     inline-набор узла; у узла без ссылок остаются на нём — выбор лаунчера,
//     config.json таких правил прежний байт в байт;
//  3. rule_set узла = рукописные теги (первыми) + теги ссылок по порядку refs
//     + тег inline-набора последним; один тег — строкой;
//  4. всё или ничего: не доехал хоть один набор дерева — правило целиком не
//     выпускается.
//
// Зеркало DNS строится тем же обходом по выпущенному телу. Имён ключей
// условий здесь нет: что идёт в набор, в зеркало и что доменное — из таблицы
// и списков реестра; код знает только ключи формы записи и логики ядра.

import (
	"encoding/json"
	"strings"

	"singbox-launcher/core/config/registry"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/srstag"
)

// Ключи логики ядра и записи rule_set (не условия — их в таблице нет).
const (
	ruleTreeKeyRuleSet = "rule_set"
	ruleTreeKeyType    = "type"
	ruleTreeKeyMode    = "mode"
	ruleTreeKeyRules   = "rules"
	ruleTreeKeyInvert  = "invert"
	ruleTreeKeyTag     = "tag"
	ruleTreeKeyFormat  = "format"
	ruleTreeKeyPath    = "path"
	ruleTreeLogical    = "logical"
	ruleTreeModeAnd    = "and"
	ruleSetTypeInline  = "inline"
	ruleSetTypeLocal   = "local"
)

// Причины невыпуска правила (Skipped, журнал).
const (
	ruleSkipNotCached      = "srs file not cached"
	ruleSkipDetour         = "download_detour unavailable"
	ruleSkipRefsOnGroup    = "rule set refs on a rule group (refs belong to a simple rule)"
	ruleSkipDanglingPrefix = "rule_set not in the config: "
)

// ruleTreeKeys — списки реестра, по которым идёт обход.
type ruleTreeKeys struct {
	set    map[string]bool // столбец «в набор» таблицы условий
	domain map[string]bool // группа domain таблицы условий
	mirror []string        // rule_dns_mirror_match (с rule_set), порядок списка
	unseen map[string]bool // rule_dns_mirror_unseen
}

func loadRuleTreeKeys() ruleTreeKeys {
	reg, err := registry.Get()
	if err != nil {
		return ruleTreeKeys{}
	}
	unseen := map[string]bool{}
	for _, k := range reg.Allowlist(ruleDNSMirrorUnseenList) {
		unseen[k] = true
	}
	return ruleTreeKeys{
		set:    reg.RuleSetKeys(),
		domain: reg.RuleDomainKeys(),
		mirror: reg.Allowlist(ruleDNSMirrorMatchList),
		unseen: unseen,
	}
}

// userRuleEmit — выпуск одного правила.
type userRuleEmit struct {
	// Body — правило в route.rules; nil — правило не выпускается.
	Body map[string]interface{}
	// RuleSets — записи наборов дерева (ссылки и inline-наборы) в порядке
	// обхода; при невыпуске — Skipped.
	RuleSets []ResolvedRouteRuleSet
	// Mirror — матчер DNS-зеркала по дереву (§6.11); nil — пусто.
	Mirror map[string]interface{}
	// MirrorDomains — доменный ключ или ссылка хоть в одном простом узле.
	MirrorDomains bool
	// Handwritten — рукописные теги rule_set тела (проверяются на висячесть
	// после выпуска всей оси).
	Handwritten []string
	// SkipReason — причина невыпуска ("" — выпущено).
	SkipReason string
}

// ruleTreeTagFn — тег ссылки и inline-набора узла.
type ruleTreeTagFn struct {
	ref    func(ref string) string
	inline func(path string) string
}

// ruleTreeWalk — результат переписывания дерева (общий для сборки и UI).
type ruleTreeWalk struct {
	refs        []string                 // ссылки дерева в порядке первого появления
	inlineSets  []map[string]interface{} // тела inline-наборов в порядке обхода
	setOrder    []string                 // теги в порядке выпуска записей (ссылки, inline)
	addrOnly    map[string]bool          // inline-наборы без доменных ключей
	domains     bool
	handwritten []string
	refsOnGroup bool
}

// rewriteRuleTree переписывает тело на месте: refs → rule_set, ключи «в
// набор» узла со ссылками → inline-набор.
func rewriteRuleTree(body map[string]interface{}, tags ruleTreeTagFn, keys ruleTreeKeys) ruleTreeWalk {
	w := ruleTreeWalk{addrOnly: map[string]bool{}}
	seen := map[string]bool{}
	corestate.WalkRuleTree(body, func(path []int, node map[string]interface{}, logical bool) bool {
		if logical {
			if _, has := node[corestate.RuleKeyRefs]; has {
				if len(corestate.NodeRefs(node)) > 0 {
					w.refsOnGroup = true
				}
				delete(node, corestate.RuleKeyRefs)
			}
			return true
		}
		refs := corestate.NodeRefs(node)
		delete(node, corestate.RuleKeyRefs)
		w.handwritten = append(w.handwritten, ruleSetTags(node[ruleTreeKeyRuleSet])...)
		if len(refs) > 0 || nodeHasAnyKey(node, keys.domain) {
			w.domains = true
		}
		if len(refs) == 0 {
			// Узел без ссылок — как есть (ключи «в набор» остаются на узле).
			return true
		}
		list := ruleSetTags(node[ruleTreeKeyRuleSet])
		for _, u := range refs {
			tag := tags.ref(u)
			list = append(list, tag)
			if !seen[u] {
				seen[u] = true
				w.refs = append(w.refs, u)
				w.setOrder = append(w.setOrder, tag)
			}
		}
		if corestate.NodeHasSetKeys(node, keys.set) {
			tag := tags.inline(corestate.RuleNodePath(path))
			m := map[string]interface{}{}
			hasDomain := false
			for k, v := range node {
				if keys.set[k] && !isZeroJSONValue(v) {
					m[k] = v
					delete(node, k)
					if keys.domain[k] {
						hasDomain = true
					}
				}
			}
			w.inlineSets = append(w.inlineSets, map[string]interface{}{
				ruleTreeKeyTag:   tag,
				ruleTreeKeyType:  ruleSetTypeInline,
				ruleTreeKeyRules: []interface{}{m},
			})
			w.setOrder = append(w.setOrder, tag)
			if !hasDomain {
				w.addrOnly[tag] = true
			}
			list = append(list, tag)
		}
		node[ruleTreeKeyRuleSet] = ruleSetValue(dedupStrings(list))
		return true
	})
	return w
}

// emitUserRule — выпуск правила inline|srs: наборы, тело маршрута, зеркало.
func emitUserRule(rule corestate.Rule, tags corestate.RuleTags, cache map[string]string, policy RuleSetPolicy, finalTags map[string]bool, keys ruleTreeKeys) userRuleEmit {
	var out userRuleEmit
	body, err := rule.BodyMap()
	if err != nil {
		out.SkipReason = "decode body: " + err.Error()
		return out
	}
	tagFn := ruleTreeTagFn{
		ref: func(u string) string {
			if t := tags.Ref(u); t != "" {
				return t
			}
			return u
		},
		inline: func(p string) string {
			if t := tags.Inline(p); t != "" {
				return t
			}
			return tags.Base + ":inline:" + p
		},
	}
	w := rewriteRuleTree(body, tagFn, keys)
	out.Handwritten = w.handwritten
	if w.refsOnGroup {
		out.SkipReason = ruleSkipRefsOnGroup
		return out
	}

	// Записи наборов в порядке обхода: ссылки узла, затем его inline-набор.
	refByTag := map[string]string{}
	for _, u := range w.refs {
		refByTag[tagFn.ref(u)] = u
	}
	inlineByTag := map[string]map[string]interface{}{}
	for _, s := range w.inlineSets {
		inlineByTag[s[ruleTreeKeyTag].(string)] = s
	}
	id := corestate.StableRuleID(rule)
	source := RouteSourceInline
	if rule.Kind == corestate.RuleKindSrs {
		source = RouteSourceSrs
	}
	for _, tag := range w.setOrder {
		entry := ResolvedRouteRuleSet{Tag: tag, Source: source, SrsID: id, Enabled: rule.Enabled}
		if s, ok := inlineByTag[tag]; ok {
			entry.Body = s
		} else if u := refByTag[tag]; policy.Remote {
			rs, ok := remoteRuleSetBody(tag, "", u, policy, finalTags)
			if !ok && out.SkipReason == "" {
				out.SkipReason = ruleSkipDetour
			}
			entry.Body = rs
		} else {
			p := cache[u]
			if p == "" && out.SkipReason == "" {
				out.SkipReason = ruleSkipNotCached
			}
			entry.Body = map[string]interface{}{
				ruleTreeKeyTag:    tag,
				ruleTreeKeyType:   ruleSetTypeLocal,
				ruleTreeKeyFormat: srstag.FormatFromRef(u),
				ruleTreeKeyPath:   p,
			}
		}
		out.RuleSets = append(out.RuleSets, entry)
	}
	if out.SkipReason != "" {
		for i := range out.RuleSets {
			out.RuleSets[i].Body = nil
			out.RuleSets[i].Skipped = true
			out.RuleSets[i].SkippedReason = out.SkipReason
		}
		return out
	}
	// Ключа refs в config.json нет ни на одном узле (ядро его не знает).
	corestate.StripBodyRefs(body)
	out.Body = body
	out.Mirror = ruleTreeMirror(body, keys, w.addrOnly)
	out.MirrorDomains = w.domains
	return out
}

// ruleTreeMirror — матчер DNS-зеркала по выпущенному дереву (§6.11).
func ruleTreeMirror(node map[string]interface{}, keys ruleTreeKeys, addrOnly map[string]bool) map[string]interface{} {
	invert, _ := node[ruleTreeKeyInvert].(bool)
	if corestate.IsLogicalRule(node) {
		subs, _ := node[ruleTreeKeyRules].([]interface{})
		kids := make([]interface{}, 0, len(subs))
		for _, sub := range subs {
			sm, ok := sub.(map[string]interface{})
			if !ok {
				continue
			}
			if m := ruleTreeMirror(sm, keys, addrOnly); m != nil {
				kids = append(kids, m)
			}
		}
		switch len(kids) {
		case 0:
			return nil
		case 1:
			// Обёртка с одним ребёнком снимается, её invert переходит на ребёнка.
			kid := kids[0].(map[string]interface{})
			if invert {
				if inv, _ := kid[ruleTreeKeyInvert].(bool); inv {
					delete(kid, ruleTreeKeyInvert)
				} else {
					kid[ruleTreeKeyInvert] = true
				}
			}
			return kid
		}
		out := map[string]interface{}{
			ruleTreeKeyType:  ruleTreeLogical,
			ruleTreeKeyMode:  node[ruleTreeKeyMode],
			ruleTreeKeyRules: kids,
		}
		if invert {
			out[ruleTreeKeyInvert] = true
		}
		return out
	}
	out := map[string]interface{}{}
	for _, k := range keys.mirror {
		v, has := node[k]
		if !has || isZeroJSONValue(v) {
			continue
		}
		if k == ruleTreeKeyRuleSet {
			v = mirrorRuleSetValue(v, addrOnly)
			if v == nil {
				continue
			}
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	if invert {
		out[ruleTreeKeyInvert] = true
	}
	return out
}

// mirrorRuleSetValue — rule_set узла без inline-наборов без доменных ключей
// (ядро не стартует на DNS-правиле с адресным набором). Ничего не исключено —
// значение как было.
func mirrorRuleSetValue(v interface{}, addrOnly map[string]bool) interface{} {
	tags := ruleSetTags(v)
	kept := make([]string, 0, len(tags))
	for _, t := range tags {
		if !addrOnly[t] {
			kept = append(kept, t)
		}
	}
	if len(kept) == len(tags) {
		return v
	}
	if len(kept) == 0 {
		return nil
	}
	return ruleSetValue(kept)
}

// RuleDNSMirrorForBody — матчер DNS-зеркала по телу записи правила (с refs, без
// кэша наборов) и признак «доменный ключ или ссылка в дереве» — для формы:
// пустое зеркало и доступность Block HTTPS не зависят от кэша. Теги ссылок в
// матчере — сами ссылки (настоящие теги считает сборка).
func RuleDNSMirrorForBody(body map[string]interface{}) (mirror map[string]interface{}, domains bool) {
	if body == nil {
		return nil, false
	}
	cp, err := deepCopyMap(body)
	if err != nil || cp == nil {
		return nil, false
	}
	keys := loadRuleTreeKeys()
	w := rewriteRuleTree(cp, ruleTreeTagFn{
		ref:    func(u string) string { return u },
		inline: func(p string) string { return ":inline:" + p },
	}, keys)
	if w.refsOnGroup {
		return nil, w.domains
	}
	corestate.StripBodyRefs(cp)
	return ruleTreeMirror(cp, keys, w.addrOnly), w.domains
}

// RuleDNSMirrorEmpty — DNS-опция правила включена, а зеркало пустое: ⚠ «DNS
// options have no effect: the rule has no condition DNS can see» (§6.11).
func RuleDNSMirrorEmpty(body map[string]interface{}, dns *corestate.RuleDNS) bool {
	if dns.IsZero() || !ruleDNSOptionOn(dns) {
		return false
	}
	m, _ := RuleDNSMirrorForBody(body)
	return m == nil
}

// ruleDNSOptionOn — объект dns включает хоть одну опцию (§6.11).
func ruleDNSOptionOn(d *corestate.RuleDNS) bool {
	if d == nil {
		return false
	}
	return (d.Enabled && d.ServerTag != "") || d.ForceIPv4 || d.BlockHTTPS || d.FixedActive()
}

// ruleSetTags — теги значения rule_set (строка или список).
func ruleSetTags(v interface{}) []string {
	switch x := v.(type) {
	case string:
		if x != "" {
			return []string{x}
		}
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, it := range x {
			if s, ok := it.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), x...)
	}
	return nil
}

// ruleSetValue — один тег строкой, несколько — списком (§6.13 п. 3).
func ruleSetValue(tags []string) interface{} {
	if len(tags) == 1 {
		return tags[0]
	}
	out := make([]interface{}, len(tags))
	for i, t := range tags {
		out[i] = t
	}
	return out
}

func dedupStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func nodeHasAnyKey(node map[string]interface{}, keys map[string]bool) bool {
	for k, v := range node {
		if keys[k] && !isZeroJSONValue(v) {
			return true
		}
	}
	return false
}

// treeRuleSetTags — все теги rule_set тела и его под-правил.
func treeRuleSetTags(body map[string]interface{}) []string {
	var out []string
	corestate.WalkRuleTree(body, func(_ []int, node map[string]interface{}, _ bool) bool {
		out = append(out, ruleSetTags(node[ruleTreeKeyRuleSet])...)
		return true
	})
	return out
}

// ReservedRuleSetTags — теги, занятые до раздачи тегов правилам (§6.13 п. 1):
// route.rule_set шаблона и наборы всех пресетов, стоящих в rules[]
// (включённых и нет, без раскрытия #if) — `<preset_id>:<tag>`.
func ReservedRuleSetTags(presetByID map[string]*template.Preset, rules []corestate.Rule, templateTags []string) map[string]bool {
	out := make(map[string]bool, len(templateTags))
	for _, t := range templateTags {
		if t != "" {
			out[t] = true
		}
	}
	for _, r := range rules {
		if r.Kind != corestate.RuleKindPreset {
			continue
		}
		p, ok := presetByID[r.Ref]
		if !ok {
			continue
		}
		for _, rs := range p.RuleSet {
			if rs.Tag != "" {
				out[p.ID+TagSeparator+rs.Tag] = true
			}
		}
	}
	return out
}

// templateRouteRuleSetTagsFromRaw — теги route.rule_set секции шаблона.
func templateRouteRuleSetTagsFromRaw(route map[string]interface{}) []string {
	arr, _ := route[ruleTreeKeyRuleSet].([]interface{})
	out := make([]string, 0, len(arr))
	for _, rs := range arr {
		if m, ok := rs.(map[string]interface{}); ok {
			if t, _ := m[ruleTreeKeyTag].(string); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

// joinTags — теги через запятую (журнал, причина Skipped).
func joinTags(tags []string) string { return strings.Join(tags, ", ") }

// TemplateRouteRuleSetTags — теги route.rule_set секции route шаблона (сырой
// JSON); секции нет или она не разбирается — пустой список (не nil: шаблон
// известен, тегов в нём нет).
func TemplateRouteRuleSetTags(routeRaw []byte) []string {
	var route map[string]interface{}
	if len(routeRaw) == 0 || json.Unmarshal(routeRaw, &route) != nil {
		return []string{}
	}
	return templateRouteRuleSetTagsFromRaw(route)
}
