package registry

// Таблица условий простого правила маршрута (contract/registry/rule_conditions.json,
// контракт 1.1.179, SPEC 188): ключ тела → условие формы, вид поля, идёт ли
// в DNS-зеркало, идёт ли в inline-набор правила. Форма и сборка спрашивают
// таблицу, а не держат имена ключей у себя.
//
// Загрузка проверяет согласованность таблицы со списками allowlists.json,
// которые читает действующий код и LxBox (route_rule_conditions,
// rule_dns_mirror_match, rule_dns_block_https_domains): расхождение — ошибка
// загрузки реестра, то есть красная любая сборка в CI, а не тихое разное
// поведение формы и зеркала.

import (
	"fmt"
)

const (
	ruleConditionsFile = "rule_conditions.json"

	// ruleConditionRefsKey — ключ записи-источника наборов внутри тела
	// простого правила. Ключа ядра с таким именем нет: в маршрут и зеркало
	// он уходит тегами rule_set, поэтому в route_rule_conditions и
	// rule_dns_mirror_match его нет и сверка его обходит.
	ruleConditionRefsKey = "refs"

	ruleCondGroupDomain  = "domain"
	ruleCondGroupAddress = "address"
)

// ruleConditionFields — конечный набор видов поля формы. Новый вид — новый
// виджет формы на обеих сторонах, поэтому неизвестное значение — ошибка
// загрузки, а не молчаливое «поле не рисуется».
var ruleConditionFields = map[string]bool{
	"refs": true, "lines": true, "cidr_lines": true, "ports": true,
	"bool": true, "enum_multi": true, "processes": true,
}

// ruleConditionFormsDefault — формы, показывающие условие без поля forms.
var ruleConditionFormsDefault = []string{"launcher", "lxbox"}

// RuleConditionKey — ключ тела внутри условия.
type RuleConditionKey struct {
	Key       string   `json:"key"`
	Field     string   `json:"field"`
	DNS       bool     `json:"dns"`
	Set       bool     `json:"set"`
	Group     string   `json:"group"`
	Conflicts []string `json:"conflicts"`
	Values    []string `json:"values"`
	Open      bool     `json:"open"`
	// Label — подпись поля формы, Placeholder — подсказка в пустом поле
	// (английский текст; у лаунчера — ключ перевода, контракт 1.1.181).
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
}

// RuleCondition — условие простого правила (рамка формы).
type RuleCondition struct {
	ID        string             `json:"id"`
	Label     string             `json:"label"`
	Order     int                `json:"order"`
	OneOf     bool               `json:"one_of"`
	Platforms []string           `json:"platforms"`
	Forms     []string           `json:"forms"`
	Keys      []RuleConditionKey `json:"keys"`
}

// ShownIn — показывает ли форма стороны form ("launcher" | "lxbox") это условие.
func (c RuleCondition) ShownIn(form string) bool {
	for _, f := range c.Forms {
		if f == form {
			return true
		}
	}
	return false
}

type ruleConditionIndex struct {
	cond int
	key  int
}

// loadRuleConditions читает таблицу и сверяет её со списками allowlists.json.
// Зовётся из Load после чтения списков.
func (r *Registry) loadRuleConditions() error {
	var file struct {
		Conditions []RuleCondition `json:"conditions"`
	}
	if err := readJSON(ruleConditionsFile, &file); err != nil {
		return err
	}
	inList := func(name string) map[string]bool {
		set := map[string]bool{}
		for _, v := range r.lists[name] {
			set[v] = true
		}
		return set
	}
	routeConds := inList("route_rule_conditions")
	mirror := inList("rule_dns_mirror_match")
	blockHTTPS := inList("rule_dns_block_https_domains")
	if len(routeConds) == 0 || len(mirror) == 0 || len(blockHTTPS) == 0 {
		return fmt.Errorf("registry: %s: нет списков route_rule_conditions / rule_dns_mirror_match / rule_dns_block_https_domains для сверки", ruleConditionsFile)
	}

	fail := func(format string, a ...interface{}) error {
		return fmt.Errorf("registry: %s: "+format, append([]interface{}{ruleConditionsFile}, a...)...)
	}
	idx := map[string]ruleConditionIndex{}
	ids := map[string]bool{}
	for ci := range file.Conditions {
		c := &file.Conditions[ci]
		if c.ID == "" || ids[c.ID] {
			return fail("условие %d: пустой или повторный id %q", ci, c.ID)
		}
		ids[c.ID] = true
		if len(c.Keys) == 0 {
			return fail("условие %q без ключей", c.ID)
		}
		if len(c.Forms) == 0 {
			c.Forms = append([]string(nil), ruleConditionFormsDefault...)
		}
		// Форма лаунчера рисует подписи из таблицы: условие и каждый его
		// ключ без подписи — ошибка загрузки, а не пустая строка в окне.
		if c.ShownIn("launcher") && c.Label == "" {
			return fail("условие %q без label", c.ID)
		}
		for ki, k := range c.Keys {
			if k.Key == "" {
				return fail("условие %q: ключ %d без имени", c.ID, ki)
			}
			if _, dup := idx[k.Key]; dup {
				return fail("ключ %q встречается в таблице дважды", k.Key)
			}
			idx[k.Key] = ruleConditionIndex{cond: ci, key: ki}
			if c.ShownIn("launcher") && k.Label == "" {
				return fail("ключ %q без label", k.Key)
			}
			if !ruleConditionFields[k.Field] {
				return fail("ключ %q: неизвестный вид поля %q", k.Key, k.Field)
			}
			grouped := k.Group == ruleCondGroupDomain || k.Group == ruleCondGroupAddress
			if k.Group != "" && !grouped {
				return fail("ключ %q: неизвестная группа %q", k.Key, k.Group)
			}
			if k.Set != grouped {
				return fail("ключ %q: set=%v, а группа %q (в набор идут ровно ключи групп domain и address)", k.Key, k.Set, k.Group)
			}
			if (k.Group == ruleCondGroupDomain) != blockHTTPS[k.Key] {
				return fail("ключ %q: группа domain расходится с rule_dns_block_https_domains", k.Key)
			}
			if k.Key == ruleConditionRefsKey {
				continue
			}
			if !routeConds[k.Key] {
				return fail("ключ %q нет в route_rule_conditions", k.Key)
			}
			if k.DNS != mirror[k.Key] {
				return fail("ключ %q: dns=%v расходится с rule_dns_mirror_match", k.Key, k.DNS)
			}
		}
	}
	for _, c := range file.Conditions {
		for _, k := range c.Keys {
			for _, other := range k.Conflicts {
				if _, ok := idx[other]; !ok {
					return fail("ключ %q: conflicts ссылается на ключ %q вне таблицы", k.Key, other)
				}
			}
		}
	}
	for key := range blockHTTPS {
		if _, ok := idx[key]; !ok {
			return fail("ключ %q списка rule_dns_block_https_domains вне таблицы", key)
		}
	}
	r.ruleConds = file.Conditions
	r.ruleCondIdx = idx
	return nil
}

// RuleConditions — условия таблицы в порядке файла (копия).
func (r *Registry) RuleConditions() []RuleCondition {
	out := make([]RuleCondition, len(r.ruleConds))
	copy(out, r.ruleConds)
	return out
}

// RuleConditionOfKey — условие и описание ключа тела; ok=false — ключа в
// таблице нет (форма открывает такое тело как JSON).
func (r *Registry) RuleConditionOfKey(key string) (RuleCondition, RuleConditionKey, bool) {
	i, ok := r.ruleCondIdx[key]
	if !ok {
		return RuleCondition{}, RuleConditionKey{}, false
	}
	c := r.ruleConds[i.cond]
	return c, c.Keys[i.key], true
}

// RuleSetKeys — ключи, которые у простого правила с refs уходят в его
// inline-набор (TEMPLATE_LANG §6.13).
func (r *Registry) RuleSetKeys() map[string]bool {
	return r.ruleCondKeys(func(k RuleConditionKey) bool { return k.Set })
}

// RuleDNSKeys — ключи, которые DNS-зеркало переносит из простого узла
// (TEMPLATE_LANG §6.11); refs входит — в зеркало он идёт тегами rule_set.
func (r *Registry) RuleDNSKeys() map[string]bool {
	return r.ruleCondKeys(func(k RuleConditionKey) bool { return k.DNS })
}

// RuleDomainKeys — доменные ключи (группа domain): Block HTTPS и признак
// «inline-набор с доменами».
func (r *Registry) RuleDomainKeys() map[string]bool {
	return r.ruleCondKeys(func(k RuleConditionKey) bool { return k.Group == ruleCondGroupDomain })
}

func (r *Registry) ruleCondKeys(pick func(RuleConditionKey) bool) map[string]bool {
	out := map[string]bool{}
	for _, c := range r.ruleConds {
		for _, k := range c.Keys {
			if pick(k) {
				out[k.Key] = true
			}
		}
	}
	return out
}
