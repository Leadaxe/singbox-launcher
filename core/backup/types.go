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

import "encoding/json"

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
