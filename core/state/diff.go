package state

import (
	"encoding/json"
	"reflect"

	"singbox-launcher/core/config/configtypes"
)

// Diff описывает «что изменилось» между двумя State.
//
// Логика разделения на CacheStale / ConfigStale:
//   - **CacheStale** — изменения, которые требуют **перепарса подписок**
//     и **перегенерации config.json** (выполняется кнопкой Update либо
//     auto-update'ом). Это правки в parser-конфиге: список / URL'ы /
//     skip / tag_prefix подписок, локальные ноды.
//   - **ConfigStale** — изменения, которые требуют **перезапуска sing-box**
//     (config.json пересобирается, но работающий процесс должен прочитать
//     новый файл). Это правки шаблонной части: tun, dns, rules,
//     log_level, vars шаблона.
//
// На один Save оба флага могут подняться независимо.
type Diff struct {
	// Поля → CacheStale
	ProxiesChanged    bool // ParserConfig.Proxies список изменился (added/removed/modified)
	OutboundsChanged  bool // ParserConfig.Outbounds (локальные ноды) изменились

	// Поля → ConfigStale
	VarsChanged          bool // SettingsVars изменились (tun_stack, log_level, dns_*, urltest_*…)
	ConfigParamsChanged  bool // route.final и т.п.
	SelectableRulesChanged bool
	CustomRulesChanged   bool
	DNSOptionsChanged    bool

	// Метаданные правок (не влияют на dirty-флаги)
	IDChanged      bool
	CommentChanged bool
}

// IsEmpty — никаких реальных изменений не зафиксировано.
func (d Diff) IsEmpty() bool {
	return !d.ProxiesChanged &&
		!d.OutboundsChanged &&
		!d.VarsChanged &&
		!d.ConfigParamsChanged &&
		!d.SelectableRulesChanged &&
		!d.CustomRulesChanged &&
		!d.DNSOptionsChanged &&
		!d.IDChanged &&
		!d.CommentChanged
}

// AffectsParser — true, если есть изменения, требующие повторного
// фетча подписок и пересборки config.json.
func (d Diff) AffectsParser() bool {
	return d.ProxiesChanged || d.OutboundsChanged
}

// AffectsTemplate — true, если есть изменения шаблонной части,
// требующие перезапуска работающего sing-box.
func (d Diff) AffectsTemplate() bool {
	return d.VarsChanged ||
		d.ConfigParamsChanged ||
		d.SelectableRulesChanged ||
		d.CustomRulesChanged ||
		d.DNSOptionsChanged
}

// DiffStates сравнивает prev и cur и возвращает Diff с поднятыми флагами
// для тех областей, что изменились. Семантика:
//   - prev == nil трактуется как «до этого state'а ничего не было»
//     (всё, что есть в cur непустое — считается изменением).
//   - cur == nil — программная ошибка вызывающего; возвращается пустой Diff.
//
// Сравнение полей делается по семантике, а не побайтно: порядок proxies в
// слайсе не важен (сравниваются как множества по Tag); порядок vars/rules
// аналогично — по Name/Label.
func DiffStates(prev, cur *State) Diff {
	if cur == nil {
		return Diff{}
	}
	var p State
	if prev != nil {
		p = *prev
	}

	d := Diff{
		IDChanged:              p.ID != cur.ID,
		CommentChanged:         p.Comment != cur.Comment,
		ProxiesChanged:         !sameProxies(p.ParserConfig.ParserConfig.Proxies, cur.ParserConfig.ParserConfig.Proxies),
		OutboundsChanged:       !sameOutboundConfigs(p.ParserConfig.ParserConfig.Outbounds, cur.ParserConfig.ParserConfig.Outbounds),
		VarsChanged:            !sameVars(p.Vars, cur.Vars),
		ConfigParamsChanged:    !sameConfigParams(p.ConfigParams, cur.ConfigParams),
		SelectableRulesChanged: !sameSelectableRules(p.SelectableRuleStates, cur.SelectableRuleStates),
		CustomRulesChanged:     !sameCustomRules(p.CustomRules, cur.CustomRules),
		DNSOptionsChanged:      !sameDNSOptions(p.DNSOptions, cur.DNSOptions),
	}
	return d
}

// --- helpers: сравнение по «множествам» (порядок неважен), где это корректно --

func sameProxies(a, b []configtypes.ProxySource) bool {
	// ProxySource не имеет уникального ключа (Source может быть пустым у
	// Connections-only источников, Connections — слайс URL'ов). Сравниваем
	// как упорядоченную последовательность: reorder тоже считается
	// изменением. False positive «Update горит после простого drag&drop» —
	// допустим, гасится одним нажатием Update.
	return reflect.DeepEqual(a, b)
}

func sameOutboundConfigs(a, b []configtypes.OutboundConfig) bool {
	// Порядок локальных нод значим (определяет порядок в config.json),
	// поэтому сравниваем как упорядоченную последовательность.
	return reflect.DeepEqual(a, b)
}

func sameVars(a, b []SettingVar) bool {
	if len(a) != len(b) {
		return false
	}
	ma := make(map[string]string, len(a))
	for _, v := range a {
		ma[v.Name] = v.Value
	}
	for _, v := range b {
		x, ok := ma[v.Name]
		if !ok || x != v.Value {
			return false
		}
	}
	return true
}

func sameConfigParams(a, b []ConfigParam) bool {
	if len(a) != len(b) {
		return false
	}
	ma := make(map[string]string, len(a))
	for _, x := range a {
		ma[x.Name] = x.Value
	}
	for _, x := range b {
		v, ok := ma[x.Name]
		if !ok || v != x.Value {
			return false
		}
	}
	return true
}

func sameSelectableRules(a, b []SelectableRuleState) bool {
	if len(a) != len(b) {
		return false
	}
	ma := make(map[string]SelectableRuleState, len(a))
	for _, x := range a {
		ma[x.Label] = x
	}
	for _, y := range b {
		x, ok := ma[y.Label]
		if !ok || x != y {
			return false
		}
	}
	return true
}

func sameCustomRules(a, b []CustomRule) bool {
	if len(a) != len(b) {
		return false
	}
	// CustomRule содержит map'ы → reflect.DeepEqual надо использовать поэлементно
	// после индексации по Label. Label у custom rule уникален (валидируется UI).
	ma := make(map[string]CustomRule, len(a))
	for _, x := range a {
		ma[x.Label] = x
	}
	for _, y := range b {
		x, ok := ma[y.Label]
		if !ok {
			return false
		}
		if !customRuleEqual(x, y) {
			return false
		}
	}
	return true
}

// customRuleEqual — глубокое сравнение через reflect.DeepEqual, но с
// детерминизацией порядка ключей в RuleSet (json.RawMessage сравнивается
// побайтно — порядок ключей внутри JSON важен; тут хотим того же поведения,
// что у MarshalIndent на исходных данных).
func customRuleEqual(a, b CustomRule) bool {
	// быстрая проверка простых полей
	if a.Label != b.Label || a.Type != b.Type || a.Enabled != b.Enabled ||
		a.SelectedOutbound != b.SelectedOutbound || a.Description != b.Description ||
		a.DefaultOutbound != b.DefaultOutbound || a.HasOutbound != b.HasOutbound {
		return false
	}
	if !reflect.DeepEqual(a.Rule, b.Rule) {
		return false
	}
	if !reflect.DeepEqual(a.Params, b.Params) {
		return false
	}
	return sameRuleSet(a.RuleSet, b.RuleSet)
}

func sameRuleSet(a, b []json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		// Индексированное сравнение: порядок rule_set внутри custom rule —
		// часть его определения (так это сейчас и пишется в state.json).
		ai, bi := string(a[i]), string(b[i])
		if ai != bi {
			return false
		}
	}
	return true
}

func sameDNSOptions(a, b *LegacyDNSOptionsV5) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Final != b.Final || a.Strategy != b.Strategy ||
		a.DefaultDomainResolver != b.DefaultDomainResolver || a.ResolverUnset != b.ResolverUnset {
		return false
	}
	// SPEC: IndependentCache diff УДАЛЁН — sing-box 1.14 deprecation.
	if !sameRawMessageSlice(a.Servers, b.Servers) {
		return false
	}
	return sameRawMessageSlice(a.Rules, b.Rules)
}

// boolPtrEqual УДАЛЁН — был нужен только для IndependentCache diff
// (sing-box 1.14 deprecation).

func sameRawMessageSlice(a, b []json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	// Порядок dns servers значим в sing-box (первый matching по rules);
	// сравниваем как упорядоченную последовательность.
	for i := range a {
		if string(a[i]) != string(b[i]) {
			return false
		}
	}
	return true
}

