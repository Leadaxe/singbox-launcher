// File rule_tree.go — тело правила как дерево условий (SPEC 188, контракт
// 1.1.179, ONE_NAMESPACE §1, BACKUP.md таблица rules[]).
//
// Тело записи inline|srs — правило sing-box: простое (набор условий) или
// составное (`type: logical`, `mode`, `rules[]`, `invert`), элементы `rules[]`
// — снова простые или составные. Ссылки на наборы (`refs`) — ключ ПРОСТОГО
// правила внутри тела на любом уровне дерева; у узла `logical` его нет.
//
//	{ "kind": "srs", "name": "Streaming", "enabled": true,
//	  "body": { "refs": ["https://…/netflix.srs"], "port": [443], "outbound": "direct" } }
//
// Старая форма (state v8, lx_backup 2) держала `refs` полем записи, а тело —
// только целью. Её читают все входы: NormalizeRuleRecord переносит ссылки
// записи в корень тела и выводит `kind` из дерева (есть `refs` — srs, нет —
// inline). `refs` и на записи, и в дереве — главнее дерево.
//
// Один читатель тела: обход, подъём `refs` и вывод `kind` живут только здесь.
// Сборка, бэкап, Debug API и UI берут ссылки правила через RuleRefs, а не
// разбирают тело сами. Имён ключей условий здесь нет — только ключи формы
// записи и логики ядра.
package state

import (
	"bytes"
	"encoding/json"

	"singbox-launcher/internal/debuglog"
)

// Ключи формы записи и логики ядра — единственные имена ключей тела, которые
// знает код (условия — в таблице реестра rule_conditions.json).
const (
	ruleKeyRefs     = "refs"
	ruleKeyType     = "type"
	ruleKeyMode     = "mode"
	ruleKeyRules    = "rules"
	ruleKeyInvert   = "invert"
	ruleTypeLogical = "logical"
	ruleModeAnd     = "and"
)

// RuleKeyRefs — ключ ссылок на наборы в простом правиле тела. Нужен сборке,
// чтобы снять его с тела, уходящего в config.json (ядро ключа не знает).
const RuleKeyRefs = ruleKeyRefs

// ruleKeyRuleSet — рукописные теги наборов в простом правиле (служебный ключ
// формы: хранится и провозится как есть).
const ruleKeyRuleSet = "rule_set"

// ruleModeOr — режим «любое из» узла `logical`.
const ruleModeOr = "or"

// IsRuleTargetKey — ключ цели правила (`outbound`, `action`, `method`): цель
// стоит только у корня тела (SPEC 188 §2).
func IsRuleTargetKey(key string) bool {
	switch key {
	case "outbound", "action", "method":
		return true
	}
	return false
}

// IsRuleKeptKey — служебный ключ простого правила, который форма хранит и
// провозит без поля: рукописные теги наборов (`rule_set`).
func IsRuleKeptKey(key string) bool { return key == ruleKeyRuleSet }

// IsRuleInvertKey — ключ отрицания простого правила (`invert`).
func IsRuleInvertKey(key string) bool { return key == ruleKeyInvert }

// RuleNodeInvert — у узла стоит `invert: true`.
func RuleNodeInvert(node map[string]interface{}) bool {
	inv, _ := node[ruleKeyInvert].(bool)
	return inv
}

// RuleLogicalParts разбирает узел `logical`: режим «любое из» (`mode: or`),
// отрицание и под-правила. ok=false — узел не составной, под-правило не
// объект, режим не and/or или у узла есть ключи сверх type/mode/rules/invert
// (цель корня вызывающий снимает сам).
func RuleLogicalParts(node map[string]interface{}) (anyOf, invert bool, children []map[string]interface{}, ok bool) {
	if !IsLogicalRule(node) {
		return false, false, nil, false
	}
	for k := range node {
		switch k {
		case ruleKeyType, ruleKeyMode, ruleKeyRules, ruleKeyInvert:
		default:
			return false, false, nil, false
		}
	}
	switch mode, _ := node[ruleKeyMode].(string); mode {
	case ruleModeAnd:
	case ruleModeOr:
		anyOf = true
	default:
		return false, false, nil, false
	}
	if v, has := node[ruleKeyInvert]; has {
		b, isBool := v.(bool)
		if !isBool {
			return false, false, nil, false
		}
		invert = b
	}
	subs, isList := node[ruleKeyRules].([]interface{})
	if !isList {
		return false, false, nil, false
	}
	for _, sub := range subs {
		m, isMap := sub.(map[string]interface{})
		if !isMap {
			return false, false, nil, false
		}
		children = append(children, m)
	}
	return anyOf, invert, children, true
}

// NewRuleLogical — узел `logical` из под-правил: `mode` and | or, `invert`
// пишется только включённым.
func NewRuleLogical(anyOf, invert bool, children []map[string]interface{}) map[string]interface{} {
	mode := ruleModeAnd
	if anyOf {
		mode = ruleModeOr
	}
	subs := make([]interface{}, 0, len(children))
	for _, c := range children {
		subs = append(subs, c)
	}
	node := map[string]interface{}{ruleKeyType: ruleTypeLogical, ruleKeyMode: mode, ruleKeyRules: subs}
	if invert {
		node[ruleKeyInvert] = true
	}
	return node
}

// SetRuleNodeInvert ставит `invert: true` простому узлу.
func SetRuleNodeInvert(node map[string]interface{}) { node[ruleKeyInvert] = true }

// IsLogicalRule — узел тела составной (`type: logical`).
func IsLogicalRule(node map[string]interface{}) bool {
	t, _ := node[ruleKeyType].(string)
	return t == ruleTypeLogical
}

// WalkRuleTree обходит дерево тела в прямом порядке (pre-order): узел, затем
// под-правила `rules[]` составного узла по порядку. path — индексы `rules[]`
// от корня (у корня пустой). fn вернул false — обход прекращается.
//
// Узлы отдаются картами тела как есть, без копии: правка узла в fn правит
// тело.
func WalkRuleTree(body map[string]interface{}, fn func(path []int, node map[string]interface{}, logical bool) bool) {
	if body == nil || fn == nil {
		return
	}
	walkRuleNode(body, nil, fn)
}

func walkRuleNode(node map[string]interface{}, path []int, fn func([]int, map[string]interface{}, bool) bool) bool {
	logical := IsLogicalRule(node)
	if !fn(path, node, logical) {
		return false
	}
	if !logical {
		return true
	}
	subs, _ := node[ruleKeyRules].([]interface{})
	for i, sub := range subs {
		m, ok := sub.(map[string]interface{})
		if !ok {
			continue
		}
		childPath := make([]int, len(path)+1)
		copy(childPath, path)
		childPath[len(path)] = i
		if !walkRuleNode(m, childPath, fn) {
			return false
		}
	}
	return true
}

// nodeRefs — ссылки простого узла в порядке записи; пустые выброшены.
// Строка вместо списка читается как одна ссылка (терпимость к рукописному).
func nodeRefs(node map[string]interface{}) []string {
	switch v := node[ruleKeyRefs].(type) {
	case string:
		if v != "" {
			return []string{v}
		}
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), v...)
	}
	return nil
}

// BodyRefs — все ссылки простых узлов дерева тела: порядок обхода (pre-order),
// внутри узла — порядок записи, без повторов и пустых.
func BodyRefs(body map[string]interface{}) []string {
	var all []string
	WalkRuleTree(body, func(_ []int, node map[string]interface{}, logical bool) bool {
		if !logical {
			all = append(all, nodeRefs(node)...)
		}
		return true
	})
	return dedupNonEmpty(all)
}

// BodyRefsNested — ссылки есть не только в корневом простом правиле: корень
// составной, а `refs` стоит в его под-правилах.
func BodyRefsNested(body map[string]interface{}) bool {
	nested := false
	WalkRuleTree(body, func(path []int, node map[string]interface{}, logical bool) bool {
		if len(path) > 0 && !logical && len(nodeRefs(node)) > 0 {
			nested = true
			return false
		}
		return true
	})
	return nested
}

// StripBodyRefs снимает ключ `refs` со всех узлов дерева (тело правит на
// месте). Тело в config.json уходит без него: наборы туда едут тегами
// `rule_set`, а ключа `refs` ядро не знает и не стартует.
func StripBodyRefs(body map[string]interface{}) {
	WalkRuleTree(body, func(_ []int, node map[string]interface{}, _ bool) bool {
		delete(node, ruleKeyRefs)
		return true
	})
}

// RuleRefs — упорядоченный список уникальных ссылок на наборы правила: обход
// дерева тела (BodyRefs). Дерево без ссылок — ссылки записи (старая форма до
// нормализации). У preset — nil.
//
// Единственный способ спросить «какие наборы у правила»: сборка, загрузчик,
// автообновление, сборщик мусора, форма, бэкап и Debug API зовут его, а не
// читают поле записи.
func RuleRefs(r Rule) []string {
	if r.Kind == RuleKindPreset {
		return nil
	}
	if body, err := decodeRuleBodyMap(r.Body); err == nil {
		if refs := BodyRefs(body); len(refs) > 0 {
			return refs
		}
	}
	return dedupNonEmpty(r.Refs)
}

// NormalizeRuleRecord приводит запись inline|srs к норме 1.1.179:
//
//   - ссылки записи (старая форма) переносятся в тело, если в дереве ссылок
//     нет: у простого корня — ключом `refs` первым, остальные байты тела не
//     трогаются; у составного корня — обёрткой `logical and`, первым
//     под-правилом `{"refs": […]}`, вторым — прежнее дерево, цель и прочие
//     ключи действия — на обёртке;
//   - ссылки и на записи, и в дереве — главнее дерево, ссылки записи
//     отбрасываются;
//   - поле записи `refs` очищается;
//   - `kind` выводится из дерева: есть `refs` — srs, нет — inline. Запись srs
//     без ссылок и без единого условия (тело — только цель) остаётся srs:
//     как inline она ловила бы весь трафик, а так сборка пропускает её, как и
//     прежде («requires refs»).
//
// Preset и неизвестный kind не трогаются. Тело, которое не разбирается,
// оставляется как есть — о нём скажет DecodeBody. Идемпотентна; changed —
// запись изменилась.
func NormalizeRuleRecord(r *Rule) (changed bool) {
	if r == nil || (r.Kind != RuleKindInline && r.Kind != RuleKindSrs) {
		return false
	}
	body, err := decodeRuleBodyMap(r.Body)
	if err != nil {
		return false
	}
	// Пустой `refs` (`[]`, `""`) — не ссылки: ключ снимается, иначе он уехал
	// бы ядру в config.json (неизвестное поле — ядро не стартует).
	if stripped, ok := stripEmptyRefs(r.Body, body); ok {
		r.Body = stripped
		changed = true
	}
	treeRefs := BodyRefs(body)
	recRefs := dedupNonEmpty(r.Refs)
	switch {
	case len(recRefs) > 0 && len(treeRefs) == 0:
		lifted, err := liftRefsIntoBody(r.Body, body, recRefs)
		if err != nil {
			debuglog.WarnLog("state: rule %q: refs kept on the record, body not rewritten: %v", r.Name, err)
			return false
		}
		r.Body = lifted
		treeRefs = recRefs
		changed = true
	case len(recRefs) > 0:
		debuglog.DebugLog("state: rule %q: refs on the record ignored, the body tree has its own", r.Name)
	}
	if r.Refs != nil {
		r.Refs = nil
		changed = true
	}

	kind := r.Kind
	switch {
	case len(treeRefs) > 0:
		kind = RuleKindSrs
	case bodyHasConditions(body):
		kind = RuleKindInline
	}
	if kind != r.Kind {
		r.Kind = kind
		changed = true
	}
	return changed
}

// stripEmptyRefs снимает ключ `refs` без единой ссылки со всех узлов дерева
// (body правится на месте). Только у корня — сырой JSON без этого ключа в
// порядке файла; ниже корня — тело заново (порядок ключей — алфавит: такое
// тело пишет только рука). ok=false — снимать нечего. `refs` со ссылками на
// узле `logical` не трогается: запись хранится как есть, сборка такое правило
// не выпускает.
func stripEmptyRefs(raw json.RawMessage, body map[string]interface{}) (json.RawMessage, bool) {
	rootOnly, found := true, false
	WalkRuleTree(body, func(path []int, node map[string]interface{}, _ bool) bool {
		if _, has := node[ruleKeyRefs]; has && len(nodeRefs(node)) == 0 {
			delete(node, ruleKeyRefs)
			found = true
			if len(path) > 0 {
				rootOnly = false
			}
		}
		return true
	})
	if !found {
		return nil, false
	}
	if rootOnly {
		if keys, vals, err := decodeObjectOrdered(bytes.TrimSpace(raw)); err == nil {
			w := newOrderedObject()
			for _, k := range keys {
				if k != ruleKeyRefs {
					w.put(k, vals[k])
				}
			}
			return w.bytes(), true
		}
	}
	out, err := json.Marshal(body)
	if err != nil {
		return nil, false
	}
	return out, true
}

// NormalizeRuleRecords — NormalizeRuleRecord по всем записям; число
// изменённых.
func NormalizeRuleRecords(rules []Rule) int {
	n := 0
	for i := range rules {
		if NormalizeRuleRecord(&rules[i]) {
			n++
		}
	}
	return n
}

// bodyHasConditions — у корня тела есть хоть один ключ, кроме цели.
func bodyHasConditions(body map[string]interface{}) bool {
	for k := range body {
		if !isTargetKey(k, body) {
			return true
		}
	}
	return false
}

// liftRefsIntoBody — сырое тело со ссылками refs в корне (форма 1.1.179).
// Байты ключей тела сохраняются в порядке файла: inline-правила без ссылок
// сюда не попадают, а у srs меняется ровно то, что переносится.
func liftRefsIntoBody(raw json.RawMessage, body map[string]interface{}, refs []string) (json.RawMessage, error) {
	refsJSON, err := json.Marshal(refs)
	if err != nil {
		return nil, err
	}
	var keys []string
	vals := map[string]json.RawMessage{}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		if keys, vals, err = decodeObjectOrdered(trimmed); err != nil {
			return nil, err
		}
	}

	if !IsLogicalRule(body) {
		w := newOrderedObject()
		w.put(ruleKeyRefs, refsJSON)
		for _, k := range keys {
			if k == ruleKeyRefs {
				continue
			}
			w.put(k, vals[k])
		}
		return w.bytes(), nil
	}

	// Составной корень: ключи логики уходят в под-правило, всё остальное
	// (цель и ключи действия) остаётся на обёртке — у `logical` ядро других
	// ключей не принимает, значит прочие и есть действие правила.
	inner := newOrderedObject()
	outer := newOrderedObject()
	for _, k := range keys {
		switch k {
		case ruleKeyType, ruleKeyMode, ruleKeyRules, ruleKeyInvert:
			inner.put(k, vals[k])
		case ruleKeyRefs:
		default:
			outer.put(k, vals[k])
		}
	}
	leaf := newOrderedObject()
	leaf.put(ruleKeyRefs, refsJSON)
	var subs bytes.Buffer
	subs.WriteByte('[')
	subs.Write(leaf.bytes())
	subs.WriteByte(',')
	subs.Write(inner.bytes())
	subs.WriteByte(']')

	typeJSON, _ := json.Marshal(ruleTypeLogical)
	modeJSON, _ := json.Marshal(ruleModeAnd)
	w := newOrderedObject()
	w.put(ruleKeyType, typeJSON)
	w.put(ruleKeyMode, modeJSON)
	w.put(ruleKeyRules, subs.Bytes())
	for _, k := range outer.keys {
		w.put(k, outer.vals[k])
	}
	return w.bytes(), nil
}

// orderedObject — JSON-объект из сырых значений в порядке добавления.
type orderedObject struct {
	keys []string
	vals map[string]json.RawMessage
}

func newOrderedObject() *orderedObject {
	return &orderedObject{vals: map[string]json.RawMessage{}}
}

func (o *orderedObject) put(k string, v json.RawMessage) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *orderedObject) bytes() json.RawMessage {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(o.vals[k])
	}
	buf.WriteByte('}')
	return json.RawMessage(buf.Bytes())
}
