// Package template: direction_groups.go — секция `group_templates` шаблона
// (SPEC 104).
//
// Во что материализуется Направление — описывает ШАБЛОН, а не код: и
// лаунчер, и LxBox читают одно описание, поэтому одинаково настроенные
// направления дают одинаковые группы на обеих платформах.
//
// Исторические имена ключей (`channel`, `magic_nodes`) сохранены: язык
// шаблона общий с LxBox (contract/docs/TEMPLATE_LANG.md §8.4), и
// переименование ключа сломало бы чужие шаблоны ради косметики.
package template

import (
	"encoding/json"
	"strings"
)

// DirectionGroupTemplates — секция `group_templates`.
type DirectionGroupTemplates struct {
	// MagicNodes — служебные опции, которые направление может включить в
	// селектор: auto (парная urltest-группа), direct, block.
	MagicNodes map[string]MagicNode `json:"magic_nodes,omitempty"`

	// Direction — описание группы самого направления (обычно selector).
	// Ключ в JSON исторический — `channel`.
	Direction DirectionGroupSpec `json:"channel"`

	// Auto — описание парной urltest-группы.
	Auto DirectionGroupSpec `json:"auto"`
}

// MagicNode — служебная опция селектора.
type MagicNode struct {
	Title string `json:"title,omitempty"`
	// Source: "generate" — тег строится по шаблону Tpl; "preset" — берётся
	// готовый Tag.
	Source string `json:"source,omitempty"`
	Tag    string `json:"tag,omitempty"`
	Tpl    string `json:"tpl,omitempty"`
}

// ResolveTag возвращает тег опции для направления с данным тегом.
func (m MagicNode) ResolveTag(parentTag string) string {
	switch m.Source {
	case "generate":
		if m.Tpl == "" {
			return ""
		}
		return strings.ReplaceAll(m.Tpl, "{parent_tag}", parentTag)
	default:
		return m.Tag
	}
}

// DirectionGroupSpec — описание группы (selector или urltest).
type DirectionGroupSpec struct {
	Type string `json:"type,omitempty"`
	// Include — какие служебные опции входят в группу по умолчанию.
	Include []string `json:"include,omitempty"`
	// Options — поля группы как они уедут в конфиг. Значения могут быть
	// ссылками на переменные шаблона ("@urltest_url"): подстановка — задача
	// движка шаблонов, и делать её здесь значило бы завести её вторую
	// реализацию.
	Options map[string]json.RawMessage `json:"options,omitempty"`
}

// DirectionGroups читает секцию `group_templates`.
//
// Отсутствие секции — не ошибка: шаблон без неё просто не даёт умолчаний, и
// направления собираются из того, что задал пользователь.
func (td *TemplateData) DirectionGroups() (DirectionGroupTemplates, bool) {
	var out DirectionGroupTemplates
	if td == nil || len(td.RawTemplate) == 0 {
		return out, false
	}
	var probe struct {
		GroupTemplates *DirectionGroupTemplates `json:"group_templates"`
	}
	if err := json.Unmarshal(td.RawTemplate, &probe); err != nil || probe.GroupTemplates == nil {
		return out, false
	}
	return *probe.GroupTemplates, true
}

// DirectionTwinOptions — опции парной urltest-группы как
// map[string]interface{}, пригодный для передачи в генератор.
//
// Отдельный метод, потому что зовущая сторона (core/config) о шаблонных
// типах не знает: ей нужен готовый словарь, а не структура.
func (td *TemplateData) DirectionTwinOptions() map[string]interface{} {
	groups, ok := td.DirectionGroups()
	if !ok || len(groups.Auto.Options) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(groups.Auto.Options))
	for k, raw := range groups.Auto.Options {
		var v interface{}
		if err := json.Unmarshal(raw, &v); err != nil {
			continue
		}
		out[k] = v
	}
	return out
}

// DirectionMagicTag возвращает тег служебной опции (direct/block/auto).
//
// Имена не универсальны — в лаунчере это `direct-out` и `block-out`, — и
// зашивать их в код значило бы сломать чужой шаблон.
func (td *TemplateData) DirectionMagicTag(name, parentTag string) string {
	groups, ok := td.DirectionGroups()
	if !ok {
		return ""
	}
	node, ok := groups.MagicNodes[name]
	if !ok {
		return ""
	}
	return node.ResolveTag(parentTag)
}
