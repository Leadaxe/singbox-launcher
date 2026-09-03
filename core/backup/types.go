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
	"sort"

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
// тела: UA, идентификатор устройства и режим его отправки (контракт 0.12.0).
//
// Вложенный объект, а не плоские ключи записи: это ОДНА настройка из
// нескольких частей, и вложенность не даёт им расползтись по корню подписки,
// где они смешались бы с полями самого источника.
//
// Все поля — указатели, включая строки: у этой настройки «не задано» и
// «задано пустым» значат разное. Пустой user_agent — это «слать дефолт
// приложения», а отсутствие ключа — «настройки нет вовсе»; принимающая
// сторона, применив пустую строку как значение, затёрла бы свой дефолт. У
// булевых полей то же самое обязательно: nil = «как в системе», false =
// «явно не отправлять».
//
// DeviceOS/VerOS/DeviceModel лаунчер не применяет (per-source их у него нет)
// и не пишет — они в схеме ради LxBox-стороны; на импорте они дают
// backup_source_identity_dropped, как любой неприменённый ключ.
type SubscriptionIdentity struct {
	UserAgent       *string `json:"user_agent,omitempty"`
	SendHWID        *bool   `json:"send_hwid,omitempty"`
	HWID            *string `json:"hwid,omitempty"`
	DeviceOS        *string `json:"device_os,omitempty"`
	VerOS           *string `json:"ver_os,omitempty"`
	DeviceModel     *string `json:"device_model,omitempty"`
	HashDeviceModel *bool   `json:"hash_device_model,omitempty"`

	// presentKeys — какие ключи реально стояли в файле, в порядке объявления
	// в схеме. Нужны ровно для одного: перечислить в предупреждении те, что
	// лаунчер не применил. Без этого списка пришлось бы либо гадать по
	// значениям (не отличив «не было ключа» от «был пустым»), либо разбирать
	// объект вторым проходом по сырому JSON.
	//
	// Общий обход неизвестных ключей (scanUnknown) внутрь identity не
	// спускается намеренно, иначе одна потеря давала бы два предупреждения:
	// своё и backup_unknown_field.
	presentKeys []string
}

// identityKeyOrder — ключи объекта в порядке схемы. Порядок фиксирован, а не
// взят из обхода map: перечень в предупреждении обязан быть воспроизводимым,
// иначе два импорта одного файла дают разный текст.
var identityKeyOrder = []string{
	"user_agent", "send_hwid", "hwid",
	"device_os", "ver_os", "device_model", "hash_device_model",
}

// identityAppliedKeys — то, что лаунчер умеет применить. Остальное (включая
// незнакомое) отбрасывается с backup_source_identity_dropped.
var identityAppliedKeys = map[string]bool{
	"user_agent": true, "send_hwid": true, "hwid": true, "hash_device_model": true,
}

// UnmarshalJSON — обычный разбор плюс запоминание СОСТАВА ключей.
//
// Свой разбор здесь потому, что стандартный теряет разницу между
// отсутствующим ключом и ключом-пустышкой на уровне, который нам нужен для
// текста предупреждения: указатели различают это для четырёх применяемых
// полей, но про неизвестные ключи в структуре не остаётся ничего.
func (i *SubscriptionIdentity) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	type plain SubscriptionIdentity
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*i = SubscriptionIdentity(p)
	// Сначала известные ключи в порядке схемы, затем чужие — в
	// лексикографическом: у map порядка нет, а текст обязан быть стабильным.
	for _, k := range identityKeyOrder {
		if _, ok := raw[k]; ok {
			i.presentKeys = append(i.presentKeys, k)
		}
	}
	var extra []string
	for k := range raw {
		if !identityKeyInSchema(k) {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	i.presentKeys = append(i.presentKeys, extra...)
	return nil
}

// identityKeyInSchema — объявлен ли ключ схемой 0.12.
func identityKeyInSchema(key string) bool {
	for _, k := range identityKeyOrder {
		if k == key {
			return true
		}
	}
	return false
}

// UnappliedKeys — ключи, приехавшие в файле, но лаунчером не применяемые:
// mobile-only тройка device_os/ver_os/device_model и всё незнакомое.
// Порядок — как в presentKeys, то есть воспроизводимый.
func (i *SubscriptionIdentity) UnappliedKeys() []string {
	if i == nil {
		return nil
	}
	var out []string
	for _, k := range i.presentKeys {
		if !identityAppliedKeys[k] {
			out = append(out, k)
		}
	}
	return out
}

// IsEmpty — объект не несёт ни одного заданного ключа: писать его в файл
// незачем (экспорт — чистая функция состояния, П1: пустышка в каждом файле
// была бы шумом, отличающим два одинаковых состояния).
func (i *SubscriptionIdentity) IsEmpty() bool {
	if i == nil {
		return true
	}
	return i.UserAgent == nil && i.SendHWID == nil && i.HWID == nil &&
		i.DeviceOS == nil && i.VerOS == nil && i.DeviceModel == nil &&
		i.HashDeviceModel == nil
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
