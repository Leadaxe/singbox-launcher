// Package backup — переносимый формат LX Backup (контракт 0.12.0).
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
	"singbox-launcher/core/state"
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
	ID  string `json:"id,omitempty"`
	URL string `json:"url"`
	// Label — имя ИСТОЧНИКА, а не узла: у подписки ссылочного тега в паре с
	// подписью нет, поэтому снос label из servers[]/chains[] (контракт 0.12)
	// её не касается — переименовать источник и ничего не сломать можно.
	Label string `json:"label,omitempty"`
	// Identity — чем подписка представляется провайдеру (контракт 0.12.0).
	// Указатель: отсутствие объекта и объект со всеми пустыми ключами — это
	// разные вещи, и экспорт не должен писать пустышку в каждый файл.
	Identity *SubscriptionIdentity `json:"identity,omitempty"`
	Enabled  *bool                 `json:"enabled,omitempty"`
	MaxNodes int                   `json:"max_nodes,omitempty"`
	Tag      *TagPolicy            `json:"tag,omitempty"`
	Update   *UpdatePolicy         `json:"update,omitempty"`
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

// SubscriptionIdentity — чем подписка представляется провайдеру при запросе
// тела: UA, идентификатор устройства и режим его отправки.
//
// Псевдоним типа состояния, а не своя копия (SPEC 127 §6.0): одно
// пространство имён — у настройки один дом, и файл бэкапа (обоих форматов)
// сериализует ровно то, что лежит в состоянии. Форма объекта и её мотивы —
// core/state/subscription_identity.go; JSON-теги те же, поэтому запись 0.12
// не меняется ни на байт.
type SubscriptionIdentity = state.SubscriptionIdentity

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
	// Tag — имя Направления у лаунчера (контракт 0.9.0) и цель правил у
	// обеих сторон: отдельного отображаемого имени здесь нет.
	Tag string `json:"tag"`
	// Label — Поддержка: LxBox, лаунчер игнорирует. С контракта 0.12.4
	// (D-094) подпись Направления объявлена в схеме: LxBox её пишет и
	// читает, лаунчер зовёт Направление тегом и приехавшее значение МОЛЧА
	// отбрасывает (BACKUP.md §1). Поле объявлено ради этого молчания —
	// иначе общий разбор неизвестных ключей давал бы backup_unknown_field
	// на каждом Направлении чужого файла. Экспорт его не пишет: `json:"-"`
	// тут не годится, потому что снял бы и чтение.
	Label string `json:"label,omitempty"`
	// PingURL и PingTimeoutMs — Поддержка: LxBox (контракт 0.12.6, D-096):
	// бюджет замера узлов Направления в приложении. Лаунчер такой настройки
	// не имеет: читает молча, на экспорте не пишет. Объявлены по той же
	// причине, что Label — иначе backup_unknown_field на каждом Направлении
	// файла LxBox. Не путать с DirectionAuto.URL/IdleTimeout (urltest ядра).
	PingURL                   string         `json:"ping_url,omitempty"`
	PingTimeoutMs             int            `json:"ping_timeout_ms,omitempty"`
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
	ID  string `json:"id,omitempty"`
	Tag string `json:"tag"`
	// Label — Поддержка: LxBox, лаунчер игнорирует. С контракта 0.12.4
	// (D-094) поле объявлено в схеме: LxBox подпись цепочки пишет и читает,
	// у лаунчера имя одно — тег (SPEC 112), поэтому приехавшее значение он
	// не применяет и МОЛЧА отбрасывает (BACKUP.md §1), а на экспорте не
	// пишет. Поле объявлено здесь затем, чтобы не считаться неизвестным
	// ключом и не шуметь warning'ом на каждом импорте файла LxBox.
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
	// Label — LEGACY-ВХОД: в схеме 0.12 поля нет; читается для файлов 0.11 и
	// раньше. У сервера без node_tag подпись становится тегом, иначе —
	// warning backup_label_dropped. Экспорт его не пишет.
	Label string `json:"label,omitempty"`
	// NodeTag — ТЕГ узла, а не подпись: на него ссылаются rules[].outbound,
	// фильтры Направлений и позиции цепочек. Отдельно от Label, потому что
	// переименование в списке не должно уводить тег из-под ссылок.
	NodeTag string `json:"node_tag,omitempty"`
	// Folder — имя папки, в которой лежит запись; пусто = корень списка.
	// Папка не имеет отдельной секции: собственных данных, кроме имени, у
	// неё нет, а вторая секция потребовала бы держать два места в согласии.
	// Обе стороны собирают её по этому имени, порядок членов = порядок
	// записей в файле.
	Folder            string `json:"folder,omitempty"`
	Enabled           *bool  `json:"enabled,omitempty"`
	ExcludeFromGlobal bool   `json:"exclude_from_global,omitempty"`
	// Sections — фрагменты конфига, которые узел носит с собой (SPEC 121).
	// Поле ЛАУНЧЕРА (BACKUP.md §2, «Поддержка: launcher»): LxBox игнорирует
	// его молча и по возможности провозит.
	Sections *ServerSections `json:"sections,omitempty"`
	SourceRef
}

// ServerSections — секции узла в бэкапе (SPEC 121 §10.5).
//
// Форма та же, что на диске: записи правил и DNS лаунчера со своими `enabled`
// и `order_num`. Своего типа у бэкапа нет намеренно — переименование полей
// завело бы вторую схему одних и тех же данных и потребовало бы держать её в
// согласии с первой.
//
// Тело едет непрозрачным блоком: разбирает его state, а не контракт. Так
// незнакомое поле DNS-сервера (`endpoint` у tailscale) переживает round-trip.
type ServerSections struct {
	// Raw — объект `sections` как он лежит в файле.
	Raw json.RawMessage
}

// MarshalJSON / UnmarshalJSON — секции едут блоком как есть.
func (s ServerSections) MarshalJSON() ([]byte, error) {
	if len(s.Raw) == 0 {
		return []byte("null"), nil
	}
	return s.Raw, nil
}

func (s *ServerSections) UnmarshalJSON(data []byte) error {
	s.Raw = append(json.RawMessage(nil), data...)
	return nil
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
	Outbound string `json:"outbound,omitempty"`
	Ref      string `json:"ref,omitempty"`
	// Refs — kind=srs: ВСЕ URL наборов правила по порядку, `ref` = `refs[0]`.
	// Пишется только при двух и более. Черновое поле контракта (D-100, по
	// образцу `sections`): сторона без поддержки читает `ref` и получает
	// первый набор — ровно то, что было до поля.
	Refs    []string          `json:"refs,omitempty"`
	Vars    map[string]string `json:"vars,omitempty"`
	Match   json.RawMessage   `json:"match,omitempty"`
	DNS     json.RawMessage   `json:"dns,omitempty"`
	Resolve json.RawMessage   `json:"resolve,omitempty"`
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
