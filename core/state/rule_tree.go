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

// ruleTreeFlatRefs — тело — простое правило, у которого кроме ссылок только
// цель: такое правило выражает нынешняя форма «наборы + выход».
func ruleTreeFlatRefs(body map[string]interface{}) bool {
	if IsLogicalRule(body) || len(nodeRefs(body)) == 0 {
		return false
	}
	for k := range body {
		if k != ruleKeyRefs && !isTargetKey(k, body) {
			return false
		}
	}
	return true
}
