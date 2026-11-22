// File rule_tags.go — теги наборов собственных правил (SPEC 188, контракт
// 1.1.179–1.1.180, TEMPLATE_LANG §6.13, D-157).
//
// Одна раздача тегов на всю ось: сборка (все вызовы ResolveRouteChecked) и
// миграция ссылок на старые теги `user:<id>` получают один и тот же результат,
// потому что считают его одной чистой функцией по одним данным — записям оси и
// занятым заранее тегам (шаблон, пресеты).
//
// Схема (§6.13):
//
//  1. заняты заранее теги route.rule_set шаблона и теги наборов всех пресетов,
//     стоящих в rules[];
//  2. базу B (имя с обрезанными краями, пустое — `unnamed`) берёт каждая
//     запись inline|srs в порядке оси — включённая и нет, с наборами и без;
//     B' — первый свободный из B, `B (2)`, `B (3)`, …;
//  3. производные теги раздаются ВСЕМ записям inline|srs в порядке оси, внутри
//     правила — обходом pre-order (ссылки узла по порядку refs, затем его
//     inline-набор): первая ссылка корневого простого правила — B', прочие —
//     `B':<h8>`; inline-набор корня без ссылок — B', со ссылками —
//     `B':inline`, листа — `B':inline:<путь>`. Тег inline-набора узла без
//     ссылок считается выданным, даже если сторона такой набор не выпускает
//     (лаунчер оставляет ключи на узле);
//  4. производный тег, совпавший с занятым, с базой или с уже выданным,
//     разводится тем же суффиксом ` (N)` на этом теге (кроме тега, равного
//     своей B').
//
// h8 — первые 8 hex sha256 ссылки с обрезанными краями; совпадение h8 у двух
// ссылок дерева одного правила (включая первую ссылку корня) — у ссылок с
// хешем в теге 16 hex.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"singbox-launcher/core/config/registry"
)

// Части тегов наборов правила (§6.13).
const (
	ruleTagUnnamed      = "unnamed"
	ruleTagSep          = ":"
	ruleTagInline       = "inline"
	ruleTagHashShort    = 8
	ruleTagHashLong     = 16
	ruleTagPathSep      = "."
	ruleTagDedupOpen    = " ("
	ruleTagDedupClose   = ")"
	ruleTagDedupFirstNo = 2
	// ruleTagDedupLimit — предохранитель развода от занятости «всего» (reserved
	// по префиксу); на живых данных не достигается.
	ruleTagDedupLimit = 100000
)

// RuleTags — теги наборов одного правила.
type RuleTags struct {
	// Base — B': база правила после развода имён.
	Base string
	// refs — ссылка → тег (одна ссылка дерева — один тег).
	refs map[string]string
	// inline — путь узла (RuleNodePath; "" — корень) → тег inline-набора.
	inline map[string]string
}

// Ref — тег записи rule_set для ссылки ref; "" — ссылки в дереве нет.
func (t RuleTags) Ref(ref string) string { return t.refs[ref] }

// Inline — тег inline-набора узла с путём path (RuleNodePath); "" — у узла нет
// ключей «в набор».
func (t RuleTags) Inline(path string) string { return t.inline[path] }

// RuleNodePath — путь узла дерева тела строкой: индексы rules[] от корня через
// точку ("" — корень, "1.0" — первый ребёнок второго ребёнка).
func RuleNodePath(path []int) string {
	parts := make([]string, len(path))
	for i, p := range path {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, ruleTagPathSep)
}

// RefHash — первые n hex sha256 ссылки (UTF-8, края обрезаны, hex в нижнем
// регистре). Сама ссылка в нижний регистр не переводится: пути
// регистрозависимы.
func RefHash(ref string, n int) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(ref)))
	h := hex.EncodeToString(sum[:])
	if n <= 0 || n > len(h) {
		return h
	}
	return h[:n]
}

// RuleTagBase — база B тегов правила: имя с обрезанными краями, пустое —
// `unnamed`.
func RuleTagBase(name string) string {
	if b := strings.TrimSpace(name); b != "" {
		return b
	}
	return ruleTagUnnamed
}

// NodeRefs — ссылки простого узла тела в порядке записи (пустые выброшены).
func NodeRefs(node map[string]interface{}) []string {
	return dedupNonEmpty(nodeRefs(node))
}

// NodeHasSetKeys — у узла есть ненулевой ключ из setKeys (столбец «в набор»
// таблицы условий реестра, registry.RuleSetKeys).
func NodeHasSetKeys(node map[string]interface{}, setKeys map[string]bool) bool {
	for k, v := range node {
		if setKeys[k] && !isZeroRuleValue(v) {
			return true
		}
	}
	return false
}

// isZeroRuleValue — значение условия пустое (как его отсутствие).
func isZeroRuleValue(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []interface{}:
		return len(x) == 0
	case []string:
		return len(x) == 0
	case map[string]interface{}:
		return len(x) == 0
	case bool:
		return !x
	case float64:
		return x == 0
	}
	return false
}

// ruleTagTaken — множество занятых тегов поверх занятых заранее.
type ruleTagTaken struct {
	reserved func(string) bool
	taken    map[string]bool
}

func (t *ruleTagTaken) has(tag string) bool {
	return t.taken[tag] || (t.reserved != nil && t.reserved(tag))
}

// dedup — первый свободный из tag, `tag (2)`, `tag (3)`, …; помечается занятым.
func (t *ruleTagTaken) dedup(tag string) string {
	out := tag
	for n := ruleTagDedupFirstNo; t.has(out) && n < ruleTagDedupLimit; n++ {
		out = tag + ruleTagDedupOpen + strconv.Itoa(n) + ruleTagDedupClose
	}
	t.taken[out] = true
	return out
}

// AllocateRuleTags — теги наборов всех записей inline|srs оси (§6.13). rules —
// в порядке оси (NormalizeRuleOrder у сборки, SortRulesByNum у загрузки
// состояния); reserved — занятые заранее теги (шаблон и пресеты); setKeys —
// ключи «в набор» таблицы условий. Ключ результата — индекс записи в rules.
func AllocateRuleTags(rules []Rule, reserved func(tag string) bool, setKeys map[string]bool) map[int]RuleTags {
	taken := &ruleTagTaken{reserved: reserved, taken: map[string]bool{}}
	out := make(map[int]RuleTags)

	// Базы — всем записям до производных тегов (п. 5 §6.13).
	for i, r := range rules {
		if r.Kind != RuleKindInline && r.Kind != RuleKindSrs {
			continue
		}
		out[i] = RuleTags{
			Base:   taken.dedup(RuleTagBase(r.Name)),
			refs:   map[string]string{},
			inline: map[string]string{},
		}
	}

	// Производные — в порядке оси, всем записям (включённым и нет).
	for i, r := range rules {
		tags, ok := out[i]
		if !ok {
			continue
		}
		body, err := decodeRuleBodyMap(r.Body)
		if err != nil {
			continue
		}
		if len(BodyRefs(body)) == 0 && len(r.Refs) > 0 {
			// Старая форма до нормализации: ссылки записи — ссылки корня.
			body = cloneShallow(body)
			refs := make([]interface{}, 0, len(r.Refs))
			for _, u := range dedupNonEmpty(r.Refs) {
				refs = append(refs, u)
			}
			if IsLogicalRule(body) {
				body = map[string]interface{}{ruleKeyType: ruleTypeLogical, ruleKeyRules: []interface{}{
					map[string]interface{}{ruleKeyRefs: refs}, body,
				}}
			} else {
				body[ruleKeyRefs] = refs
			}
		}
		allocateDerivedTags(body, tags, taken, setKeys)
	}
	return out
}

// allocateDerivedTags — производные теги одного правила (п. 4–5 §6.13).
func allocateDerivedTags(body map[string]interface{}, tags RuleTags, taken *ruleTagTaken, setKeys map[string]bool) {
	all := BodyRefs(body)
	// Длина хеша: совпадение h8 у двух ссылок дерева правила (включая первую
	// ссылку корня) — у ссылок с хешем в теге 16 hex.
	byShort := make(map[string]int, len(all))
	for _, u := range all {
		byShort[RefHash(u, ruleTagHashShort)]++
	}
	firstRoot := ""
	if !IsLogicalRule(body) {
		if refs := NodeRefs(body); len(refs) > 0 {
			firstRoot = refs[0]
		}
	}
	WalkRuleTree(body, func(path []int, node map[string]interface{}, logical bool) bool {
		if logical {
			return true
		}
		refs := NodeRefs(node)
		for _, u := range refs {
			if _, done := tags.refs[u]; done {
				continue
			}
			if u == firstRoot {
				tags.refs[u] = tags.Base
				continue
			}
			n := ruleTagHashShort
			if byShort[RefHash(u, ruleTagHashShort)] > 1 {
				n = ruleTagHashLong
			}
			tags.refs[u] = taken.dedup(tags.Base + ruleTagSep + RefHash(u, n))
		}
		if !NodeHasSetKeys(node, setKeys) {
			return true
		}
		p := RuleNodePath(path)
		switch {
		case len(path) == 0 && len(refs) == 0:
			tags.inline[p] = tags.Base
		case len(path) == 0:
			tags.inline[p] = taken.dedup(tags.Base + ruleTagSep + ruleTagInline)
		default:
			tags.inline[p] = taken.dedup(tags.Base + ruleTagSep + ruleTagInline + ruleTagSep + p)
		}
		return true
	})
}

func cloneShallow(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// RuleSetKeysFromRegistry — ключи «в набор» таблицы условий реестра; реестр не
// загрузился — nil (inline-наборы тогда не считаются, теги ссылок те же).
func RuleSetKeysFromRegistry() map[string]bool {
	reg, err := registry.Get()
	if err != nil {
		return nil
	}
	return reg.RuleSetKeys()
}

// StateRuleTags — теги наборов правил состояния без шаблона (загрузка,
// миграция ссылок, UI-вид): ось — SortRulesByNum, заняты заранее теги с
// префиксом `<preset_id>:` пресетов из rules[] (точного списка наборов
// пресета без шаблона нет; расхождение со сборкой — только у правила, чьё имя
// начинается на `<preset_id>:`). Результат выровнен по индексам rules.
func StateRuleTags(rules []Rule) []RuleTags {
	order := make([]int, len(rules))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return ruleNum(rules[order[a]]) < ruleNum(rules[order[b]]) })
	sorted := make([]Rule, len(rules))
	var prefixes []string
	for i, idx := range order {
		sorted[i] = rules[idx]
		if rules[idx].Kind == RuleKindPreset && rules[idx].Ref != "" {
			prefixes = append(prefixes, rules[idx].Ref+ruleTagSep)
		}
	}
	// Тег с суффиксом развода ` (N)` префиксом пресета не занят: локальные
	// теги пресетов таких суффиксов не носят, а без этого развод имени
	// `<preset_id>:…` не нашёл бы свободного тега.
	reserved := func(tag string) bool {
		for _, p := range prefixes {
			if strings.HasPrefix(tag, p) && !strings.Contains(tag[len(p):], ruleTagDedupOpen) {
				return true
			}
		}
		return false
	}
	byAxis := AllocateRuleTags(sorted, reserved, RuleSetKeysFromRegistry())
	out := make([]RuleTags, len(rules))
	for i, idx := range order {
		out[idx] = byAxis[i]
	}
	return out
}
