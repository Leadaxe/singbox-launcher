// File rule_conditions.go — модель формы правила (SPEC 188 §2, §3, §6): тело
// правила ⇄ стопка условий-рамок и групп правил. Без Fyne: её зовут окно
// правила (форма, вкладка JSON, Preview) и раннер корпуса.
//
// Что за условие у ключа, вид поля и подписи — таблица реестра
// rule_conditions.json (core/config/registry); имён ключей условий здесь нет.
// Служебные ключи (логика ядра, цель, рукописные теги) спрашиваются у
// core/state/rule_tree.go.
//
// Хранение (SPEC §3): правило с группами — `logical and`, первым под-правилом
// обычные условия, следом каждая группа; одна группа без других условий —
// сама группа. Тело, которое в эту форму не ложится (ключ вне таблицы, два
// способа Process сразу, цель в под-правиле), открывается вкладкой JSON.
//
// go1.20-совместимо (Win7): без slices/maps/min/max.
package models

import (
	"encoding/json"
	"sort"

	"singbox-launcher/core/config/registry"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/srstag"
)

// RuleCondForm — имя формы лаунчера в поле forms таблицы условий.
const RuleCondForm = "launcher"

// RuleCondTable — таблица условий для формы лаунчера: условия, которые форма
// показывает, в порядке меню, и принадлежность ключа.
type RuleCondTable struct {
	// Conds — условия формы лаунчера по полю order.
	Conds []registry.RuleCondition
	// byKey — ключ тела → индекс условия в Conds.
	byKey map[string]int
	// kept — ключи таблицы, чьих условий форма не показывает (LxBox):
	// хранятся и провозятся без изменений.
	kept map[string]bool
}

// NewRuleCondTable строит таблицу формы из реестра.
func NewRuleCondTable(reg *registry.Registry) RuleCondTable {
	t := RuleCondTable{byKey: map[string]int{}, kept: map[string]bool{}}
	if reg == nil {
		return t
	}
	for _, c := range reg.RuleConditions() {
		if !c.ShownIn(RuleCondForm) {
			for _, k := range c.Keys {
				t.kept[k.Key] = true
			}
			continue
		}
		t.Conds = append(t.Conds, c)
	}
	sort.SliceStable(t.Conds, func(i, j int) bool { return t.Conds[i].Order < t.Conds[j].Order })
	for i, c := range t.Conds {
		for _, k := range c.Keys {
			t.byKey[k.Key] = i
		}
	}
	return t
}

// LauncherRuleCondTable — таблица формы из загруженного реестра.
func LauncherRuleCondTable() (RuleCondTable, error) {
	reg, err := registry.Get()
	if err != nil {
		return RuleCondTable{byKey: map[string]int{}, kept: map[string]bool{}}, err
	}
	return NewRuleCondTable(reg), nil
}

// Cond — условие формы по id.
func (t RuleCondTable) Cond(id string) (registry.RuleCondition, bool) {
	for _, c := range t.Conds {
		if c.ID == id {
			return c, true
		}
	}
	return registry.RuleCondition{}, false
}

// RuleCond — заполненное условие-рамка: значения ключей как в теле.
type RuleCond struct {
	ID     string
	Values map[string]interface{}
}

// RuleCondStack — простое правило формы: условия (между собой «и») и группы
// правил (тоже «и» с условиями).
type RuleCondStack struct {
	Conds  []RuleCond
	Groups []RuleGroup
	// Extra — ключи без поля формы: условия, которых форма лаунчера не
	// показывает, и рукописный `rule_set`. Провозятся как есть.
	Extra map[string]interface{}
	// Not — `invert` простого правила (написан руками). Форма показывает его
	// группой с галкой Not (ForForm).
	Not bool
}

// RuleGroup — группа правил: «all of» (and) | «any of» (or), «Not» (invert).
type RuleGroup struct {
	Any   bool
	Not   bool
	Rules []RuleCondStack
}

// Empty — в стопке нечего писать в тело.
func (t RuleCondTable) Empty(s RuleCondStack) bool { return t.StackBody(s) == nil }

// StackFromBody — стопка формы из тела правила. Цель корня (`outbound`,
// `action`, `method`) пропускается. ok=false — тело форма не выражает и
// открывает его вкладкой JSON.
func (t RuleCondTable) StackFromBody(body map[string]interface{}) (RuleCondStack, bool) {
	match := make(map[string]interface{}, len(body))
	for k, v := range body {
		if !corestate.IsRuleTargetKey(k) {
			match[k] = v
		}
	}
	return t.stackFromNode(match)
}

func (t RuleCondTable) stackFromNode(node map[string]interface{}) (RuleCondStack, bool) {
	if corestate.IsLogicalRule(node) {
		return t.stackFromLogical(node)
	}
	return t.stackFromSimple(node)
}

// stackFromLogical — составной узел: «условия + группы» (форма SPEC §3)
// раскладывается в стопку, всё прочее — одна группа.
func (t RuleCondTable) stackFromLogical(node map[string]interface{}) (RuleCondStack, bool) {
	anyOf, invert, children, ok := corestate.RuleLogicalParts(node)
	if !ok {
		return RuleCondStack{}, false
	}
	if !anyOf && !invert && len(children) >= 2 {
		rest := children[1:]
		allGroups := true
		for _, c := range rest {
			if !corestate.IsLogicalRule(c) {
				allGroups = false
				break
			}
		}
		if allGroups {
			var s RuleCondStack
			from := 0
			if !corestate.IsLogicalRule(children[0]) {
				first, ok := t.stackFromSimple(children[0])
				if !ok {
					return RuleCondStack{}, false
				}
				if !first.Not && (len(first.Conds) > 0 || len(first.Extra) > 0) {
					s.Conds, s.Extra = first.Conds, first.Extra
					from = 1
				} else {
					from = -1
				}
			}
			if from >= 0 {
				for _, c := range children[from:] {
					g, ok := t.groupFromNode(c)
					if !ok {
						return RuleCondStack{}, false
					}
					s.Groups = append(s.Groups, g)
				}
				return s, true
			}
		}
	}
	g, ok := t.groupFromNode(node)
	if !ok {
		return RuleCondStack{}, false
	}
	return RuleCondStack{Groups: []RuleGroup{g}}, true
}

func (t RuleCondTable) groupFromNode(node map[string]interface{}) (RuleGroup, bool) {
	anyOf, invert, children, ok := corestate.RuleLogicalParts(node)
	if !ok {
		return RuleGroup{}, false
	}
	g := RuleGroup{Any: anyOf, Not: invert}
	for _, c := range children {
		s, ok := t.stackFromNode(c)
		if !ok {
			return RuleGroup{}, false
		}
		g.Rules = append(g.Rules, s)
	}
	return g, true
}

func (t RuleCondTable) stackFromSimple(node map[string]interface{}) (RuleCondStack, bool) {
	var s RuleCondStack
	byCond := map[int]map[string]interface{}{}
	for k, v := range node {
		switch {
		case corestate.IsRuleInvertKey(k):
			b, isBool := v.(bool)
			if !isBool {
				return RuleCondStack{}, false
			}
			s.Not = b
		case corestate.IsRuleKeptKey(k) || t.kept[k]:
			if s.Extra == nil {
				s.Extra = map[string]interface{}{}
			}
			s.Extra[k] = v
		default:
			ci, known := t.byKey[k]
			if !known {
				return RuleCondStack{}, false
			}
			if byCond[ci] == nil {
				byCond[ci] = map[string]interface{}{}
			}
			byCond[ci][k] = v
		}
	}
	idx := make([]int, 0, len(byCond))
	for ci := range byCond {
		idx = append(idx, ci)
	}
	sort.Ints(idx)
	for _, ci := range idx {
		c := t.Conds[ci]
		vals := byCond[ci]
		if c.OneOf {
			n := 0
			for _, v := range vals {
				if !RuleCondValueEmpty(v) {
					n++
				}
			}
			if n > 1 {
				// Два способа Process сразу ядро связывает «и» — форма
				// выбирает один; такое тело правится в JSON.
				return RuleCondStack{}, false
			}
		}
		s.Conds = append(s.Conds, RuleCond{ID: c.ID, Values: vals})
	}
	return s, true
}

// StackBody — тело правила без цели (SPEC §3); nil — писать нечего. Пустые
// поля и условия не пишутся, пустые правила и группы выбрасываются.
func (t RuleCondTable) StackBody(s RuleCondStack) map[string]interface{} {
	simple := map[string]interface{}{}
	for _, c := range s.Conds {
		for k, v := range c.Values {
			if !RuleCondValueEmpty(v) {
				simple[k] = v
			}
		}
	}
	for k, v := range s.Extra {
		simple[k] = v
	}
	var groups []map[string]interface{}
	for _, g := range s.Groups {
		if b := t.groupBody(g); b != nil {
			groups = append(groups, b)
		}
	}
	if len(groups) == 0 {
		if len(simple) == 0 {
			return nil
		}
		if s.Not {
			corestate.SetRuleNodeInvert(simple)
		}
		return simple
	}
	var out map[string]interface{}
	if len(simple) == 0 && len(groups) == 1 {
		out = groups[0]
	} else {
		children := make([]map[string]interface{}, 0, len(groups)+1)
		if len(simple) > 0 {
			children = append(children, simple)
		}
		children = append(children, groups...)
		out = corestate.NewRuleLogical(false, false, children)
	}
	if s.Not {
		out = corestate.NewRuleLogical(false, true, []map[string]interface{}{out})
	}
	return out
}

func (t RuleCondTable) groupBody(g RuleGroup) map[string]interface{} {
	var children []map[string]interface{}
	for _, r := range g.Rules {
		if b := t.StackBody(r); b != nil {
			children = append(children, b)
		}
	}
	if len(children) == 0 {
		return nil
	}
	return corestate.NewRuleLogical(g.Any, g.Not, children)
}

// ForForm — стопка, какой её рисует форма: `invert` простого правила (у
// формы нет галки Not у правила) становится группой «all of» с Not вокруг
// него. Тело при этом то же по смыслу; окно сравнивает снимки формы, так что
// неизменённое правило сохраняется исходным телом.
func (s RuleCondStack) ForForm() RuleCondStack {
	out := RuleCondStack{Conds: s.Conds, Extra: s.Extra}
	for _, g := range s.Groups {
		ng := RuleGroup{Any: g.Any, Not: g.Not}
		for _, r := range g.Rules {
			ng.Rules = append(ng.Rules, r.ForForm())
		}
		out.Groups = append(out.Groups, ng)
	}
	if !s.Not {
		return out
	}
	return RuleCondStack{Groups: []RuleGroup{{Not: true, Rules: []RuleCondStack{out}}}}
}

// RuleCondValueEmpty — значение поля пусто: не пишется в тело.
func RuleCondValueEmpty(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case bool:
		return !x
	case []interface{}:
		return len(x) == 0
	case []string:
		return len(x) == 0
	case []int:
		return len(x) == 0
	}
	return false
}

// RuleCondBodyJSON — тело стопки каноническим JSON (ключи по алфавиту):
// снимок для сравнения «правило не меняли».
func (t RuleCondTable) RuleCondBodyJSON(s RuleCondStack) string {
	b, err := json.Marshal(t.StackBody(s))
	if err != nil {
		return ""
	}
	return string(b)
}

// RuleSetsFromBody — записи наборов `remote` по ссылкам тела (`refs` в
// дереве) для кнопки скачивания и гейта: тег — имя файла копии от ссылки
// (srstag.TagFromURL), как у загрузчика и сборщика мусора.
func RuleSetsFromBody(body map[string]interface{}) []json.RawMessage {
	refs := corestate.BodyRefs(body)
	if len(refs) == 0 {
		return nil
	}
	out := make([]json.RawMessage, 0, len(refs))
	for _, u := range refs {
		raw, err := json.Marshal(map[string]interface{}{
			"tag":    srstag.TagFromURL(u),
			"type":   "remote",
			"format": srstag.FormatFromRef(u),
			"url":    u,
		})
		if err == nil {
			out = append(out, raw)
		}
	}
	return out
}

// RuleStateRuleSets — наборы правила Конфигуратора: из `refs` тела, а у
// legacy-вида без них — поле RuleSets.
func RuleStateRuleSets(rs *RuleState) []json.RawMessage {
	if rs == nil {
		return nil
	}
	if sets := RuleSetsFromBody(rs.Rule.Rule); len(sets) > 0 {
		return sets
	}
	return rs.Rule.RuleSets
}
