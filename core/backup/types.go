// Package backup — переносимый формат LX Backup (контракт 0.11.0).
//
// Назначение: перенести подписки, серверы, цепочки, Направления, правила, DNS
// и переменные между лаунчером и LxBox. Формат общий, схема нормативна —
// contract/schema/backup.schema.json, семантика — contract/docs/BACKUP.md,
// идеи и инварианты — contract/docs/BACKUP_PRINCIPLES.md (П1–П7, при
// конфликте побеждают они).
//
// Три инварианта определяют весь дизайн:
//
//  1. Бэкап — сериализация состояния (П1). Экспорт — чистая функция
//     состояния: два неотличимых состояния дают байт-идентичные файлы;
//     состояние после импорта неотличимо от настроенного руками. Механизма
//     extensions нет: провоз непонятого создавал состояние-призрак, которое
//     протухает, когда каноническую часть правят в другом приложении.
//  2. Непонятое отбрасывается с предупреждением (П3), а не применяется молча
//     и не везётся дальше.
//  3. Нет молчаливых потерь (П6): всё, что не применилось, названо
//     пользователю warning'ом.
package backup

import (
	"encoding/json"

	"singbox-launcher/core/config/configtypes"
)

// FormatVersion — мажор формата (BACKUP.md §8). Импортёр читает свою и
// меньшие версии; бо́льшую отклоняет с понятной ошибкой.
const FormatVersion = 1

// AppLauncher — идентификатор приложения в exported_by.app.
const AppLauncher = "launcher"

// AppLxBox — вторая сторона контракта.
const AppLxBox = "lxbox"

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
}

// ExportedBy — кто и чем создал файл. Нужен для диагностики и для
// предупреждений об односторонних полях.
type ExportedBy struct {
	App      string `json:"app"`
	Version  string `json:"version"`
	Platform string `json:"platform,omitempty"`
}

// SourceRef — ссылка источника на цель дозвона, общая для всех типов
// источников (подписка / сервер / цепочка).
//
// Вынесена отдельной структурой, а не переписана трижды: у трёх записей это
// одно и то же понятие, и разъехавшиеся копии полей — это разъехавшийся
// формат. Финальный конфиговый тег сюда не пишется никогда (П5): он
// вычисляется каждой сборкой на принимающей стороне.
type SourceRef struct {
	// DetourTag — тег outbound'а, через который дозваниваются узлы источника.
	DetourTag string `json:"detour_tag,omitempty"`
	// DetourNodeSourceID + DetourNodeTag — ссылка-ОБЪЕКТ на один узел:
	// id источника-цели плюс identity-тег узла внутри него (IDENTITY.md §2.1).
	DetourNodeSourceID string `json:"detour_node_source_id,omitempty"`
	DetourNodeTag      string `json:"detour_node_tag,omitempty"`
	// DetourNodeLabel — снимок подписи узла-цели, только для показа.
	DetourNodeLabel string `json:"detour_node_label,omitempty"`
}

// Subscription — источник подписки.
type Subscription struct {
	// ID — стабильный идентификатор источника: цель ссылок
	// detour_node_source_id. Без него ссылка на узел этого источника приехала
	// бы мёртвой.
	ID       string        `json:"id,omitempty"`
	URL      string        `json:"url"`
	Label    string        `json:"label,omitempty"`
	Enabled  *bool         `json:"enabled,omitempty"`
	MaxNodes int           `json:"max_nodes,omitempty"`
	Tag      *TagPolicy    `json:"tag,omitempty"`
	Update   *UpdatePolicy `json:"update,omitempty"`
	// Disabled — отметки выключенных нод: идентичность узла → unix seconds.
	// Ключ для формата обмена непрозрачен и копируется как есть (BACKUP.md §5).
	Disabled map[string]int64 `json:"disabled,omitempty"`
	// Skip — фильтры отсева узлов подписки. Поддержка — только launcher.
	Skip []map[string]string `json:"skip,omitempty"`
	// Outbounds — локальные Направления источника, в КАНОНИЧЕСКОЙ форме —
	// той же, что directions[] на корне. Внутренняя структура сюда не едет:
	// её поля не объявлены в схеме и не стоят в таблице поддержки
	// BACKUP.md §2 — то есть были бы ровно тем тайным грузом, ради сноса
	// которого убран механизм extensions.
	//
	// Канонизация — с потерями, и цена названа в BACKUP.md §2/§10: не едут
	// ref и updates (привязка к пресету), comment, options кроме
	// interrupt_exist_connections, ключи filters кроме tag, type (импорт
	// принудительно ставит selector) и invert у preferredDefault.
	// addOutbounds и preferredDefault, напротив, переносятся — первый
	// признаками include_direct/include_block/include, второй телом
	// регулярки в default. Поддержка — только launcher.
	Outbounds []Direction `json:"outbounds,omitempty"`
	// Fold — свёртка подписки в группу (SPEC 108). Только launcher.
	//
	// SPEC 118: в модели ей наследует FolderReplace, а в контракте 0.11
	// форма прежняя — конверторы границы (convert_v7.go) переводят одно в
	// другое. Тип локальный: в приложении такого больше нет.
	Fold                    *Fold `json:"fold,omitempty"`
	ExcludeFromGlobal       bool  `json:"exclude_from_global,omitempty"`
	ExposeGroupTagsToGlobal bool  `json:"expose_group_tags_to_global,omitempty"`
	SourceRef
}

// Fold — свёртка подписки контракта 0.11 (прежний configtypes.SourceFold).
//
// Живёт ЗДЕСЬ, на границе бэкапа: контракт 0.11 не меняется (SPEC 118 §2), а
// в модели v7 свёртки в этой форме нет — её место занял FolderReplace.
type Fold struct {
	// Mode: "select" | "auto" | "select_auto"; пустое читается как select.
	Mode string `json:"mode,omitempty"`
	// Auto — параметры автогруппы (режимы auto | select_auto).
	Auto *configtypes.DirectionAuto `json:"auto,omitempty"`
}

// Direction — Направление, цель правил (SPEC 104).
//
// Каноническая форма контракта (contract/schema/direction.schema.json), а не
// внутренняя структура приложения: у сторон они разные, а переносится
// именно модель. Отбор узлов передаётся ТЕЛОМ регулярки без обёртки — язык
// паттернов у платформ различается, а тело одинаково.
type Direction struct {
	// Tag — единственное имя Направления (контракт 0.9.0): отдельного
	// отображаемого имени нет. Ключ label из чужого/старого файла — обычное
	// неизвестное поле: отбрасывается с warning (П3), а не подставляется.
	Tag                       string         `json:"tag"`
	Enabled                   *bool          `json:"enabled,omitempty"`
	Filter                    string         `json:"filter,omitempty"`
	Invert                    bool           `json:"invert,omitempty"`
	Default                   string         `json:"default,omitempty"`
	IncludeDirect             bool           `json:"include_direct,omitempty"`
	IncludeBlock              bool           `json:"include_block,omitempty"`
	Include                   []string       `json:"include,omitempty"`
	InterruptExistConnections *bool          `json:"interrupt_exist_connections,omitempty"`
	Auto                      *DirectionAuto `json:"auto,omitempty"`
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

// Chain — цепочка хопов (SPEC 110).
//
// Идентичность и merge — по Tag: это тег будущего outbound'а, на него
// ссылаются rules[].outbound, route.final, фильтры Направлений и позиции
// других цепочек. ID переносится ради ссылок detour_node_source_id, но
// идентичностью при merge не является (BACKUP.md §4).
//
// Порядок записей нормативен — вложенная цепочка объявляется раньше
// использующей; секция не сортируется ни на экспорте, ни на импорте.
type Chain struct {
	ID      string `json:"id,omitempty"`
	Tag     string `json:"tag"`
	Label   string `json:"label,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
	// Chain — канон цепочки (contract/schema/source_chain.schema.json).
	// Общая форма с configtypes.SourceChain: вторая копия канона была бы
	// расхождением, ждущим своего случая.
	Chain             *configtypes.SourceChain `json:"chain"`
	ExcludeFromGlobal bool                     `json:"exclude_from_global,omitempty"`
	SourceRef
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
	ID         string          `json:"id,omitempty"`
	URI        string          `json:"uri,omitempty"`
	ConfigJSON json.RawMessage `json:"config_json,omitempty"`
	Label      string          `json:"label,omitempty"`
	// NodeTag — ТЕГ узла, а не подпись: на него ссылаются rules[].outbound,
	// фильтры Направлений и позиции цепочек. Отдельно от Label, потому что
	// переименование в списке не должно уводить тег из-под ссылок.
	NodeTag           string `json:"node_tag,omitempty"`
	Enabled           *bool  `json:"enabled,omitempty"`
	ExcludeFromGlobal bool   `json:"exclude_from_global,omitempty"`
	SourceRef
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
	Outbound string            `json:"outbound,omitempty"`
	Ref      string            `json:"ref,omitempty"`
	Vars     map[string]string `json:"vars,omitempty"`
	Match    json.RawMessage   `json:"match,omitempty"`
	DNS      json.RawMessage   `json:"dns,omitempty"`
	Resolve  json.RawMessage   `json:"resolve,omitempty"`
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
	Kind    string            `json:"kind,omitempty"`
	Name    string            `json:"name,omitempty"`
	Enabled *bool             `json:"enabled,omitempty"`
	Num     *float64          `json:"num,omitempty"`
	Ref     string            `json:"ref,omitempty"`
	Vars    map[string]string `json:"vars,omitempty"`
	Value   json.RawMessage   `json:"value,omitempty"`
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
