// Package backup — переносимый формат LX Backup v1 (SPEC 103, фаза 4).
//
// Назначение: перенести подписки, серверы, правила, DNS и переменные между
// лаунчером и LxBox без потерь. Формат общий, схема нормативна —
// contract/schema/backup.schema.json, семантика — contract/docs/BACKUP.md.
//
// Три инварианта определяют весь дизайн:
//
//  1. Lossless round-trip: import(export(x)) == x в том же приложении.
//     Всё, что не переносится, едет в extensions.<app> и обязано пережить
//     чужой импорт нетронутым — иначе бэкап, прошедший через телефон,
//     возвращается на десктоп обеднённым.
//  2. Default-deny: неизвестный ключ вне extensions не применяется молча —
//     он либо предъявлен пользователю, либо не существует.
//  3. Нет молчаливых потерь: то, что не применилось, названо warning'ом.
package backup

import (
	"encoding/json"

	"singbox-launcher/core/config/configtypes"
)

// FormatVersion — мажор формата (BACKUP.md §6). Импортёр читает свою и
// меньшие версии; бо́льшую отклоняет с понятной ошибкой.
const FormatVersion = 1

// AppLauncher — идентификатор приложения в exported_by.app и в ключах
// extensions.
const AppLauncher = "launcher"

// AppLxBox — вторая сторона контракта; её блоб extensions мы обязаны
// сохранять нетронутым.
const AppLxBox = "lxbox"

// Extensions — непереносимые данные по приложениям: {"launcher": {...},
// "lxbox": {...}}. Чужой блоб хранится как есть и возвращается в следующий
// экспорт.
type Extensions map[string]json.RawMessage

// Backup — корень файла.
type Backup struct {
	LxBackup   int        `json:"lx_backup"`
	ExportedBy ExportedBy `json:"exported_by"`
	ExportedAt string     `json:"exported_at"`

	Subscriptions []Subscription    `json:"subscriptions,omitempty"`
	Servers       []Server          `json:"servers,omitempty"`
	Directions    []Direction       `json:"directions,omitempty"`
	Chains        []Chain           `json:"chains,omitempty"`
	Rules         []Rule            `json:"rules,omitempty"`
	DNS           *DNS              `json:"dns,omitempty"`
	Vars          map[string]string `json:"vars,omitempty"`
	Route         *Route            `json:"route,omitempty"`
	Warp          []json.RawMessage `json:"warp,omitempty"`

	Extensions Extensions `json:"extensions,omitempty"`
}

// ExportedBy — кто и чем создал файл. Нужен для диагностики и для
// предупреждений об односторонних полях.
type ExportedBy struct {
	App      string `json:"app"`
	Version  string `json:"version"`
	Platform string `json:"platform,omitempty"`
}

// Subscription — источник подписки.
type Subscription struct {
	URL      string        `json:"url"`
	Label    string        `json:"label,omitempty"`
	Enabled  *bool         `json:"enabled,omitempty"`
	Skip     *bool         `json:"skip,omitempty"`
	MaxNodes int           `json:"max_nodes,omitempty"`
	Tag      *TagPolicy    `json:"tag,omitempty"`
	Update   *UpdatePolicy `json:"update,omitempty"`
	// Disabled — отметки выключенных нод: identity-хеш → unix seconds.
	// Только по хешу (BACKUP.md §4): тег и подпись у сторон разные.
	Disabled   map[string]int64 `json:"disabled,omitempty"`
	Detour     json.RawMessage  `json:"detour,omitempty"`
	Extensions Extensions       `json:"extensions,omitempty"`
}

// Direction — Направление, цель правил (SPEC 104, схема v1.1).
//
// Каноническая форма контракта (contract/schema/direction.schema.json), а не
// внутренняя структура приложения: у сторон они разные, а переносится
// именно модель. Отбор узлов передаётся ТЕЛОМ регулярки без обёртки — язык
// паттернов у платформ различается, а тело одинаково.
//
// Зачем это в бэкапе: до v1.1 правило, ссылавшееся на `vpn-3` с телефона,
// приезжало на десктоп в никуда и импортировалось выключенным. Теперь
// отсутствующее Направление создаётся, и правило приходит рабочим.
type Direction struct {
	Tag                       string         `json:"tag"`
	Label                     string         `json:"label,omitempty"`
	Enabled                   *bool          `json:"enabled,omitempty"`
	Filter                    string         `json:"filter,omitempty"`
	Invert                    bool           `json:"invert,omitempty"`
	Default                   string         `json:"default,omitempty"`
	IncludeDirect             bool           `json:"include_direct,omitempty"`
	IncludeBlock              bool           `json:"include_block,omitempty"`
	Include                   []string       `json:"include,omitempty"`
	InterruptExistConnections *bool          `json:"interrupt_exist_connections,omitempty"`
	Auto                      *DirectionAuto `json:"auto,omitempty"`
	Extensions                Extensions     `json:"extensions,omitempty"`
}

// DirectionAuto — параметры парной группы автовыбора.
type DirectionAuto struct {
	Mode                      string   `json:"mode,omitempty"`
	URL                       string   `json:"url,omitempty"`
	Interval                  string   `json:"interval,omitempty"`
	Tolerance                 int      `json:"tolerance,omitempty"`
	IdleTimeout               string   `json:"idle_timeout,omitempty"`
	InterruptExistConnections *bool    `json:"interrupt_exist_connections,omitempty"`
	Pool                      int      `json:"pool,omitempty"`
	PoolTolerance             int      `json:"pool_tolerance,omitempty"`
	StickyHash                []string `json:"sticky_hash,omitempty"`
}

// Chain — цепочка хопов (SPEC 110, схема v1.2).
//
// Идентичность и merge — по Tag: это тег будущего outbound'а, на него
// ссылаются rules[].outbound, route.final, фильтры Направлений и позиции
// других цепочек. У лаунчера тег хранится именем источника (Label ≡ TagMask,
// adapter_source.go), отдельного отображаемого имени у цепочки нет: Label
// здесь не пишется, а чужое значение провозится непонятым полем записи
// (backupFieldsKey) до следующего экспорта.
//
// Порядок записей нормативен — вложенная цепочка объявляется раньше
// использующей; секция не сортируется ни на экспорте, ни на импорте.
type Chain struct {
	Tag     string `json:"tag"`
	Label   string `json:"label,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
	// Chain — канон цепочки (contract/schema/source_chain.schema.json).
	// Общая форма с configtypes.SourceChain: вторая копия канона была бы
	// расхождением, ждущим своего случая.
	Chain      *configtypes.SourceChain `json:"chain"`
	Extensions Extensions               `json:"extensions,omitempty"`
}

// TagPolicy — правила именования нод источника.
type TagPolicy struct {
	Prefix  string `json:"prefix,omitempty"`
	Postfix string `json:"postfix,omitempty"`
	Mask    string `json:"mask,omitempty"`
}

// UpdatePolicy — автообновление источника.
type UpdatePolicy struct {
	IntervalHours int   `json:"interval_hours,omitempty"`
	Auto          *bool `json:"auto,omitempty"`
}

// Server — одиночный узел: ровно одно из URI / ConfigJSON.
type Server struct {
	URI        string          `json:"uri,omitempty"`
	ConfigJSON json.RawMessage `json:"config_json,omitempty"`
	Label      string          `json:"label,omitempty"`
	Enabled    *bool           `json:"enabled,omitempty"`
	Detour     json.RawMessage `json:"detour,omitempty"`
	Extensions Extensions      `json:"extensions,omitempty"`
}

// RuleKind — вид правила.
type RuleKind string

const (
	RuleInline RuleKind = "inline"
	RuleSRS    RuleKind = "srs"
	RulePreset RuleKind = "preset"
	RuleJSON   RuleKind = "json"
)

// Rule — правило маршрутизации.
type Rule struct {
	Kind    RuleKind `json:"kind"`
	Name    string   `json:"name,omitempty"`
	Enabled *bool    `json:"enabled,omitempty"`
	// Num — общая ось порядка (модель LxBox). Импортёр перенумеровывает,
	// сохраняя ОТНОСИТЕЛЬНЫЙ порядок: абсолютные номера у сторон свои.
	Num *float64 `json:"num,omitempty"`
	// Outbound — символическая ссылка на цель. Несуществующая цель не
	// повод терять правило: импортируется выключенным с warning.
	Outbound   string            `json:"outbound,omitempty"`
	Ref        string            `json:"ref,omitempty"`
	Vars       map[string]string `json:"vars,omitempty"`
	Match      json.RawMessage   `json:"match,omitempty"`
	DNS        json.RawMessage   `json:"dns,omitempty"`
	Resolve    json.RawMessage   `json:"resolve,omitempty"`
	Extensions Extensions        `json:"extensions,omitempty"`
}

// DNS — секция DNS.
type DNS struct {
	Servers  []DNSRef `json:"servers,omitempty"`
	Rules    []DNSRef `json:"rules,omitempty"`
	Final    string   `json:"final,omitempty"`
	Strategy string   `json:"strategy,omitempty"`
}

// DNSRef — запись DNS с дискриминатором происхождения.
type DNSRef struct {
	Kind       string            `json:"kind,omitempty"`
	Name       string            `json:"name,omitempty"`
	Enabled    *bool             `json:"enabled,omitempty"`
	Num        *float64          `json:"num,omitempty"`
	Ref        string            `json:"ref,omitempty"`
	Vars       map[string]string `json:"vars,omitempty"`
	Value      json.RawMessage   `json:"value,omitempty"`
	Extensions Extensions        `json:"extensions,omitempty"`
}

// Route — маршрутные умолчания.
type Route struct {
	Final string `json:"final,omitempty"`
}

// boolPtr — helper для полей с умолчанием true: писать значение нужно
// только когда оно отличается от умолчания схемы.
func boolPtr(v bool) *bool { return &v }

// f64Ptr — helper для номера оси порядка.
func f64Ptr(v float64) *float64 { return &v }
