package template

// Шаблонная часть модели каналов (SPEC 104): описание групп и стартовый набор.
//
// Каналы настраиваются ШАБЛОНОМ, а не зашиты в код: тип группы, состав
// магических опций (direct/block/auto) и параметры urltest живут в
// `group_templates`, а стартовые каналы — в `default_channels`. Так автор
// шаблона задаёт поведение, не трогая приложение, и обе платформы читают
// одно и то же описание.

import (
	"encoding/json"
	"strings"
)

// DefaultChannel — стартовый канал из `default_channels[]`.
//
// Применяется ОДИН раз, при первом появлении секции каналов в состоянии:
// дальше набор принадлежит пользователю, и повторный seed затирал бы его
// правки при каждом обновлении шаблона.
type DefaultChannel struct {
	Tag            string `json:"tag"`
	Label          string `json:"label,omitempty"`
	DefaultEnabled *bool  `json:"default_enabled,omitempty"`
}

// Enabled сообщает, включён ли канал по умолчанию (умолчание — да).
func (d DefaultChannel) IsEnabled() bool {
	return d.DefaultEnabled == nil || *d.DefaultEnabled
}

// ChannelGroupTemplates — секция `group_templates` шаблона.
type ChannelGroupTemplates struct {
	// MagicNodes — служебные опции, которые канал может включить в селектор:
	// auto (генерируемая urltest-группа), direct, block.
	MagicNodes map[string]MagicNode `json:"magic_nodes,omitempty"`
	// Channel — описание группы самого канала (обычно selector).
	Channel ChannelGroupSpec `json:"channel"`
	// Auto — описание парной urltest-группы.
	Auto ChannelGroupSpec `json:"auto"`
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

// ResolveTag возвращает тег опции для канала с данным тегом.
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

// ChannelGroupSpec — описание группы (selector или urltest).
type ChannelGroupSpec struct {
	Type string `json:"type,omitempty"`
	// Include — какие магические опции добавляются в группу по умолчанию.
	Include []string `json:"include,omitempty"`
	// Options — поля группы как они уедут в конфиг; значения могут быть
	// ссылками на переменные шаблона ("@urltest_url").
	Options map[string]json.RawMessage `json:"options,omitempty"`
}

// ChannelTemplates читает секции каналов из шаблона.
//
// Отсутствие секций — не ошибка: шаблон без каналов означает, что модель
// каналов для него не используется, и приложение работает по-старому.
func (td *TemplateData) ChannelTemplates() (ChannelGroupTemplates, bool) {
	var out ChannelGroupTemplates
	if td == nil || len(td.RawTemplate) == 0 {
		return out, false
	}
	var probe struct {
		GroupTemplates *ChannelGroupTemplates `json:"group_templates"`
	}
	if err := json.Unmarshal(td.RawTemplate, &probe); err != nil || probe.GroupTemplates == nil {
		return out, false
	}
	return *probe.GroupTemplates, true
}

// DefaultChannels читает стартовый набор каналов из шаблона.
func (td *TemplateData) DefaultChannels() []DefaultChannel {
	if td == nil || len(td.RawTemplate) == 0 {
		return nil
	}
	var probe struct {
		DefaultChannels []DefaultChannel `json:"default_channels"`
	}
	if err := json.Unmarshal(td.RawTemplate, &probe); err != nil {
		return nil
	}
	return probe.DefaultChannels
}
