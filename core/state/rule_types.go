// File rule_types.go — Rule, RuleKind, виды тела и конструкторы (SPEC 127, state v8).
//
// # Одно пространство имён (SPEC 127 §2, contract/docs/ONE_NAMESPACE.md §1)
//
// Запись правила = метаданные приложения + `body` = правило sing-box КАК ЕСТЬ:
//
//	{ "kind": "inline", "name": "4pda", "enabled": true, "num": 1000,
//	  "body": { "domain_suffix": ["4pda.to"], "outbound": "proxy-out" } }
//	{ "kind": "srs", "name": "…", "enabled": true, "num": 1010,
//	  "refs": ["https://…/a.srs"], "body": { "outbound": "proxy-out" } }
//	{ "kind": "preset", "ref": "block-ads", "enabled": true, "num": 960,
//	  "vars": { "out": "reject" } }
//
// До v8 тело было разобрано на части (`body{name,match,outbound}`,
// `body{name,srs_url,srs_urls,outbound}`, `body{vars}`), а номер звался
// `order_num`. Миграция по сырому документу — `migration_v7_to_v8.go`.
//
// # Читатель один, писателей — четыре
//
// Тело читает ТОЛЬКО DecodeBody (возвращает ВИД — PresetBody/InlineBody/
// SrsBody, собранный из полей записи и body). Пишут тело ТОЛЬКО
// NewPresetRule / NewInlineRule / NewSrsRule / (*Rule).SetOutbound. Ручной
// `json.Marshal(state.XBody{…})` вне этого файла — ошибка: асимметрия
// «один читатель, дюжина писателей» уже дала два бага (srs-правило узла
// эмитило один набор из трёх; бэкап канонизировал URL в обход DecodeBody).
package state

import (
	"bytes"
	"encoding/json"
	"fmt"

	"singbox-launcher/internal/outboundutil"
)

// RuleKind — дискриминатор типа правила в state.rules[].
type RuleKind string

const (
	// RuleKindPreset — тонкая ссылка на template.presets[].
	// Тела нет: match-поля живут в template, свои значения переменных — в Vars.
	RuleKindPreset RuleKind = "preset"

	// RuleKindInline — user-defined inline rule.
	// Body: правило sing-box целиком (матчеры + outbound|action).
	RuleKindInline RuleKind = "inline"

	// RuleKindSrs — user-defined srs rule.
	// Refs: URL наборов; Body: только цель (`rule_set` вписывает сборка).
	RuleKindSrs RuleKind = "srs"
)

// Rule — единица в state.rules[] и в sections.rules[] (одна форма для обоих).
//
// Сериализация (порядок полей = порядок ключей в файле):
//
//	{
//	  "kind":    "preset" | "inline" | "srs",
//	  "id":      "…",              // необязательные метаданные; лаунчер не генерирует, провозит
//	  "ref":     "<preset_id>",    // только preset
//	  "name":    "…",              // inline|srs; источник StableRuleID
//	  "enabled": true,
//	  "num":     1000,             // позиция на оси; nil = не размечено
//	  "refs":    ["https://…"],    // только srs
//	  "vars":    {"out":"direct"}, // только preset
//	  "body":    { … }             // inline|srs: правило sing-box как есть
//	}
type Rule struct {
	// Kind — discriminator. Required.
	Kind RuleKind `json:"kind"`

	// ID — необязательные метаданные другой стороны (ONE_NAMESPACE §1).
	// Лаунчер его не генерирует и не читает — только провозит без потерь.
	ID string `json:"id,omitempty"`

	// Ref — ссылка на template.presets[].id. Required для kind=preset, иначе пуст.
	Ref string `json:"ref,omitempty"`

	// Name — отображаемое имя (inline|srs). Источник StableRuleID: строка та
	// же, что до v8 лежала в body.name.
	Name string `json:"name,omitempty"`

	// Enabled — общий toggle.
	Enabled bool `json:"enabled"`

	// Num — позиция на разреженной оси порядка (SPEC 106, D-051; до v8 —
	// `order_num`).
	//
	// Авторитетный источник приоритета правила: правила сортируются по
	// возрастанию Num, при равенстве сохраняется взаимный порядок в списке.
	// Позиция в самом слайсе `state.Rules[]` перестаёт быть приоритетом —
	// она лишь тай-брейк.
	//
	// nil = правило ещё не размечено: разметка происходит при первой же
	// загрузке (MarkRuleOrder), отдельного шага миграции нет.
	Num *int `json:"num,omitempty"`

	// Refs — URL наборов srs-правила в порядке ввода (до v8 — body.srs_url +
	// body.srs_urls). Дедуп с сохранением порядка делает NewSrsRule.
	//
	// `rule_set` в Body НЕ хранится: теги наборов вписывает сборка по этому
	// списку (resolve_route.go).
	Refs []string `json:"refs,omitempty"`

	// Vars — значения переменных пресета (до v8 — body.vars). Только diff от
	// template-дефолтов: пустая карта = всё дефолтное.
	Vars map[string]string `json:"vars,omitempty"`

	// Body — правило sing-box КАК ЕСТЬ (inline|srs); у preset тела нет.
	//
	// RawMessage, а не карта: порядок ключей тела в файле сохраняется
	// побайтно (SubstituteSelf работает по сырому JSON ради того же).
	Body json.RawMessage `json:"body,omitempty"`
}

// PresetBody — ВИД записи kind=preset.
//
// Vars хранит ТОЛЬКО diff от template-default'ов. Пустой map = всё дефолтное.
// Bump'нули template → юзер автоматически получает новые дефолты для var'ов
// которые он не трогал.
type PresetBody struct {
	Vars map[string]string `json:"vars"`
}

// InlineBody — ВИД записи kind=inline.
type InlineBody struct {
	// Name — отображаемое имя в UI (поле записи Rule.Name).
	Name string `json:"name"`

	// Match — тело БЕЗ цели: outbound/action/method сняты.
	Match map[string]interface{} `json:"match"`

	// Outbound — outbound tag или зарезервированный литерал "reject" / "drop",
	// вычисленный из тела: action=reject → "reject", +method=drop → "drop".
	Outbound string `json:"outbound"`
}

// SrsBody — ВИД записи kind=srs.
//
// Одно правило может ссылаться на НЕСКОЛЬКО наборов (диалог правила принимает
// список URL, sing-box принимает `rule_set: [..]`). Список живёт полем записи
// `Refs`; URLs() оставлен алиасом ради читателей, написанных до v8.
type SrsBody struct {
	Name     string   `json:"name"`
	Refs     []string `json:"refs"`
	Outbound string   `json:"outbound"` // tag | "reject" | "drop"
}

// URLs — все URL наборов правила в порядке ввода (минимум один у валидной записи).
func (b *SrsBody) URLs() []string {
	if b == nil {
		return nil
	}
	return b.Refs
}

// ── Конструкторы: единственные писатели Body ───────────────────────

// NewPresetRule — запись пресета. Тела нет, переменные — полем записи.
func NewPresetRule(ref string, vars map[string]string) Rule {
	out := Rule{Kind: RuleKindPreset, Ref: ref}
	if len(vars) > 0 {
		cp := make(map[string]string, len(vars))
		for k, v := range vars {
			cp[k] = v
		}
		out.Vars = cp
	}
	return out
}

// NewInlineRule — inline-правило: body = match ∪ цель.
//
// Ключи карты сериализуются json.Marshal'ом (сортировка по алфавиту) — это
// норма для НОВЫХ записей; порядок ключей существующих правил сохраняет
// миграция, которая берёт байты match как есть.
func NewInlineRule(name string, match map[string]interface{}, outbound string) Rule {
	body := make(map[string]interface{}, len(match)+2)
	for k, v := range match {
		if isTargetKey(k, match) {
			continue
		}
		body[k] = v
	}
	outboundutil.ApplyOutboundToRule(body, outbound)
	raw, err := json.Marshal(body)
	if err != nil {
		raw = json.RawMessage(`{}`)
	}
	return Rule{Kind: RuleKindInline, Name: name, Body: raw}
}

// NewSrsRule — srs-правило: наборы полем записи (дедуп с сохранением порядка,
// пустые выброшены), body = только цель.
func NewSrsRule(name string, refs []string, outbound string) Rule {
	body := map[string]interface{}{}
	outboundutil.ApplyOutboundToRule(body, outbound)
	raw, err := json.Marshal(body)
	if err != nil {
		raw = json.RawMessage(`{}`)
	}
	return Rule{Kind: RuleKindSrs, Name: name, Refs: dedupNonEmpty(refs), Body: raw}
}

// SetOutbound переписывает цель в теле правила, не трогая остальные ключи и
// их порядок (нужен applyRenames и редактору UI).
//
// Работа по сырому JSON, а не через карту: пересортировка ключей матчеров
// сделала бы файл другим на ровном месте, а golden — красным.
func (r *Rule) SetOutbound(outbound string) error {
	if r == nil {
		return fmt.Errorf("state: SetOutbound on nil rule")
	}
	switch r.Kind {
	case RuleKindInline, RuleKindSrs:
	default:
		return fmt.Errorf("state: rule kind=%q has no outbound", string(r.Kind))
	}
	body := r.Body
	if len(bytes.TrimSpace(body)) == 0 {
		body = json.RawMessage(`{}`)
	}
	var keys []string
	var vals map[string]json.RawMessage
	var err error
	if keys, vals, err = decodeObjectOrdered(body); err != nil {
		return fmt.Errorf("state: rule body: %w", err)
	}

	// Целевые ключи снимаем, остальные оставляем на местах: самостоятельный
	// `action` (не `reject`) — эффект правила, а не цель, и переживает смену
	// цели вместе с матчерами.
	bodyMap, _ := decodeRuleBodyMap(body)
	kept := keys[:0]
	for _, k := range keys {
		if isTargetKey(k, bodyMap) {
			continue
		}
		kept = append(kept, k)
	}
	target := map[string]interface{}{}
	outboundutil.ApplyOutboundToRule(target, outbound)

	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	writeKV := func(k string, raw json.RawMessage) {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(raw)
	}
	for _, k := range kept {
		writeKV(k, vals[k])
	}
	// Цель дописывается в конец в фиксированном порядке ApplyOutboundToRule.
	for _, k := range []string{"outbound", "action", "method"} {
		v, ok := target[k]
		if !ok {
			continue
		}
		vb, err := json.Marshal(v)
		if err != nil {
			return err
		}
		writeKV(k, vb)
	}
	buf.WriteByte('}')
	r.Body = json.RawMessage(buf.Bytes())
	return nil
}

// decodeObjectOrdered — ключи JSON-объекта в порядке их появления плюс сырые
// значения. Нужен всюду, где порядок ключей тела обязан пережить правку.
func decodeObjectOrdered(raw json.RawMessage) ([]string, map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, nil, fmt.Errorf("expected an object")
	}
	var keys []string
	vals := map[string]json.RawMessage{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		key, ok := kt.(string)
		if !ok {
			return nil, nil, fmt.Errorf("expected an object key")
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, nil, err
		}
		if _, seen := vals[key]; !seen {
			keys = append(keys, key)
		}
		vals[key] = val
	}
	if _, err := dec.Token(); err != nil {
		return nil, nil, err
	}
	return keys, vals, nil
}

// dedupNonEmpty снимает дубли и пустые строки, сохраняя порядок.
func dedupNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, u := range in {
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// outboundFromBody — цель правила в форме UI из тела sing-box:
// action=reject без method → "reject", action=reject+method=drop → "drop",
// иначе body.outbound (или "").
func outboundFromBody(body map[string]interface{}) string {
	if body == nil {
		return ""
	}
	if action, _ := body["action"].(string); action == "reject" {
		if method, _ := body["method"].(string); method == "drop" {
			return "drop"
		}
		return "reject"
	}
	out, _ := body["outbound"].(string)
	return out
}

// isTargetKey — ключ тела принадлежит ЦЕЛИ правила, а не матчерам.
//
// `outbound` и `method` — всегда цель. `action` — цель ТОЛЬКО со значением
// `reject`: остальные значения (`sniff`, `hijack-dns`, `resolve`, `route`…) —
// самостоятельный эффект правила sing-box, который обязан доехать до
// config.json и пережить круг «состояние → бэкап → состояние». Снятие такого
// `action` вместе с целью превращало рабочее правило в матчер без эффекта.
func isTargetKey(key string, body map[string]interface{}) bool {
	switch key {
	case "outbound", "method":
		return true
	case "action":
		action, _ := body["action"].(string)
		return action == "reject"
	}
	return false
}

// DecodeBody — единственный читатель тела: возвращает ВИД записи, собранный
// из полей записи и body. Один из {*PresetBody, *InlineBody, *SrsBody}.
//
// Валидация по kind:
//
//	preset → r.Ref required
//	inline → r.Ref empty, r.Name required
//	srs    → r.Ref empty, r.Name required, хотя бы один Refs
//
// Ошибки:
//   - kind=preset без ref → semantic error
//   - kind=inline/srs с ref / без name (srs ещё без refs) → semantic error
//   - kind unknown → error
//   - JSON unmarshal failed → error
func (r *Rule) DecodeBody() (interface{}, error) {
	switch r.Kind {
	case RuleKindPreset:
		if r.Ref == "" {
			return nil, fmt.Errorf("rule kind=preset requires ref")
		}
		vars := make(map[string]string, len(r.Vars))
		for k, v := range r.Vars {
			vars[k] = v
		}
		return &PresetBody{Vars: vars}, nil

	case RuleKindInline:
		if r.Ref != "" {
			return nil, fmt.Errorf("rule kind=inline must not have ref")
		}
		if r.Name == "" {
			return nil, fmt.Errorf("rule kind=inline requires name")
		}
		body, err := decodeRuleBodyMap(r.Body)
		if err != nil {
			return nil, fmt.Errorf("decode inline body: %w", err)
		}
		match := make(map[string]interface{}, len(body))
		for k, v := range body {
			if isTargetKey(k, body) {
				continue
			}
			match[k] = v
		}
		return &InlineBody{Name: r.Name, Match: match, Outbound: outboundFromBody(body)}, nil

	case RuleKindSrs:
		if r.Ref != "" {
			return nil, fmt.Errorf("rule kind=srs must not have ref")
		}
		if r.Name == "" {
			return nil, fmt.Errorf("rule kind=srs requires name")
		}
		refs := dedupNonEmpty(r.Refs)
		if len(refs) == 0 {
			return nil, fmt.Errorf("rule kind=srs requires refs")
		}
		body, err := decodeRuleBodyMap(r.Body)
		if err != nil {
			return nil, fmt.Errorf("decode srs body: %w", err)
		}
		return &SrsBody{Name: r.Name, Refs: refs, Outbound: outboundFromBody(body)}, nil

	default:
		return nil, fmt.Errorf("unknown rule kind: %q", r.Kind)
	}
}

// BodyMap — тело правила картой: правило sing-box КАК ЕСТЬ, вместе с целью
// (`outbound` | `action`[+`method`]). Нужен сборке и UI там, где тело едет в
// конфиг целиком и разбирать его на матчеры и цель незачем.
//
// Возвращается копия: карта разбирается из сырых байтов на каждый вызов,
// поэтому мутировать её безопасно. Пустое тело — пустая карта.
func (r *Rule) BodyMap() (map[string]interface{}, error) {
	if r == nil {
		return map[string]interface{}{}, nil
	}
	return decodeRuleBodyMap(r.Body)
}

// decodeRuleBodyMap — тело правила картой; пустое тело — пустая карта.
func decodeRuleBodyMap(raw json.RawMessage) (map[string]interface{}, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]interface{}{}, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	return out, nil
}
