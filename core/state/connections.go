// File connections.go — раздел "connections" в state.json (SPEC 052).
// Types: ConnectionsSection, Source, Defaults, SourceType, TagSpec, UpdateSpec,
// SubscriptionMeta, UserInfo.
package state

import (
	"encoding/json"

	"singbox-launcher/core/config/configtypes"
)

// ConnectionsSection — раздел подключений: sources + направления + defaults.
type ConnectionsSection struct {
	Sources []Source `json:"sources"`

	// Outbounds — глобальные Направления (SPEC 104). Имя поля в Go
	// сохранено намеренно: это те же записи, что и раньше, и ~40 callsite'ов
	// `Connections.Outbounds` описывают именно outbound-секцию конфига.
	Outbounds []configtypes.Direction `json:"direction_outbounds"`

	// LegacyOutbounds — прежний ключ `outbounds`. ТОЛЬКО чтение: состояния,
	// записанные до SPEC 104, переносятся в Outbounds при загрузке
	// (`adoptLegacyDirections`) и больше никогда не пишутся. Поле обязано
	// быть пустым к моменту сериализации — иначе state.json получил бы обе
	// секции и следующая загрузка выбрала бы старую.
	LegacyOutbounds []configtypes.Direction `json:"outbounds,omitempty"`

	Defaults Defaults `json:"defaults"`
}

// adoptLegacyDirections переносит прежний ключ `outbounds` в канонический
// `direction_outbounds` (SPEC 104).
//
// Обе секции сразу — это не «слияние», а выбор: канонический ключ
// приоритетнее. Иначе состояние, однажды сохранённое новой версией и потом
// потроганное старой, склеивало бы два набора направлений с дублями тегов.
func (c *ConnectionsSection) adoptLegacyDirections() {
	if c == nil {
		return
	}
	if c.Outbounds == nil && c.LegacyOutbounds != nil {
		c.Outbounds = c.LegacyOutbounds
	}
	c.LegacyOutbounds = nil
}

// Defaults — настройки по умолчанию для всех source'ов (могут переопределяться
// per-source).
type Defaults struct {
	Reload   string `json:"reload,omitempty"`
	MaxNodes int    `json:"max_nodes,omitempty"`
}

// SourceType — дискриминатор: "subscription" (URL → пачка нод) или
// "server" (один URI → один outbound).
type SourceType string

const (
	SourceTypeSubscription SourceType = "subscription"
	SourceTypeServer       SourceType = "server"
	// SourceTypeChain — цепочка хопов (SPEC 110): маршрут через несколько
	// позиций подряд. Третий тип рядом с подпиской и сервером, потому что
	// цепочка это МАРШРУТ; точка выбора между маршрутами — Направление, и
	// цепочка попадает в него узлом, наравне с серверами подписки.
	SourceTypeChain SourceType = "chain"
)

// Source — единица подключения. Тип определяет, какие поля используются:
//
//   - SourceTypeSubscription: URL/Skip/Tag/Outbounds/Update/MaxNodes/Meta
//   - SourceTypeServer:       URI; Tag/Update/Meta не используются
//   - SourceTypeChain:        Chain; URL/URI/Tag/Update/Meta не используются
//
// Поля identity (ID/Type/Enabled/Label/ExcludeFromGlobal) — общие.
// Тег узла у server/chain — в NodeTag (см. там); Label только отображает.
type Source struct {
	// identity
	ID                string     `json:"id"`
	Type              SourceType `json:"type"`
	Enabled           bool       `json:"enabled"`
	Label             string     `json:"label,omitempty"`
	ExcludeFromGlobal bool       `json:"exclude_from_global,omitempty"`

	// NodeTag — системный тег узла для type=server и type=chain.
	//
	// Заведён отдельно от Label, потому что раньше этих двух ролей у
	// источника не различали: adapter_source.go клал `TagMask: s.Label`
	// («force tag = label»), то есть переименование в списке молча меняло
	// тег, на который ссылаются фильтры Направлений, позиции других цепочек
	// и rules[].outbound. Пользователь правил подпись — и терял маршрут.
	//
	// Теперь роли разведены ровно как у Направления (Direction.Tag /
	// Direction.Label): тег — идентификатор, на него ссылаются; Label —
	// отображаемое имя, правится свободно. Пусто → NodeTagOrLabel
	// откатывается на Label, чем и держится совместимость с состояниями,
	// записанными до этого разделения (миграция их не переписывает: пустой
	// NodeTag читается как «тег = Label», ровно прежнее поведение).
	//
	// Для type=subscription не используется: там именами узлов управляет
	// Tag (*TagSpec) — prefix/postfix/mask.
	NodeTag string `json:"node_tag,omitempty"`

	// type=subscription only
	URL                     string                  `json:"url,omitempty"`
	Skip                    []map[string]string     `json:"skip,omitempty"`
	Tag                     *TagSpec                `json:"tag,omitempty"`
	Outbounds               []configtypes.Direction `json:"outbounds,omitempty"`
	ExposeGroupTagsToGlobal bool                    `json:"expose_group_tags_to_global,omitempty"`

	// Fold — свёртка подписки в группу (SPEC 108). nil = не свёрнута:
	// узлы попадают в Направления по отдельности.
	//
	// Заменяет прежнюю четвёрку флагов. Сами группы (`<PFX>auto`,
	// `<PFX>select`) в Outbounds больше не хранятся — они разворачиваются
	// на сборке (config.PrepareSourceFolds) ровно так же, как парные
	// auto-группы Направлений: пользователь настраивает одну свёртку, а не
	// два объекта, которые обязаны оставаться синхронными.
	Fold     *configtypes.SourceFold `json:"fold,omitempty"`
	Update   *UpdateSpec             `json:"update,omitempty"`
	MaxNodes int                     `json:"max_nodes,omitempty"`
	Meta     *SubscriptionMeta       `json:"meta,omitempty"`

	// type=server only
	URI string `json:"uri,omitempty"`

	// Chain — type=chain only (SPEC 110): позиции маршрута и настройки
	// звеньев. Материализуется в один outbound типа `chain`.
	Chain *configtypes.SourceChain `json:"chain,omitempty"`

	// ConfigJSON — type=server only: ручной sing-box outbound/endpoint объект.
	// Если задан, при сборке конфига он вставляется passthrough (URI не
	// парсится): для нод, которые не выражаются share-URI (нет протокола /
	// парсера / конвертера) и собраны руками. Лаунчер перештамповывает только
	// tag (= Label) и detour; остальные поля уходят в config.json как есть.
	ConfigJSON json.RawMessage `json:"config_json,omitempty"`

	// DetourTag — SPEC 077: tag of another outbound this source's nodes dial
	// through (proxy chain / hop). Empty = direct dial. Applies to both server
	// and subscription sources. Stored by tag (consistent with rules/selectors);
	// a dangling/cyclic/self target is dropped at build time (fail-open), the
	// node then dials directly. Not applied to WireGuard nodes.
	DetourTag string `json:"detour_tag,omitempty"`

	// DetourNodeSourceID / DetourNodeTag — SPEC 112-A: ссылка на ОДИН узел,
	// через который дозваниваются узлы этого источника. Ссылка — ОБЪЕКТ:
	// ULID источника-цели плюс identity-тег узла ВНУТРИ него (SPEC 112).
	//
	// Финальный конфиговый тег в состоянии не хранится намеренно: он —
	// производная от tag_prefix/tag_mask источника-цели и вычисляется на каждой
	// сборке, а хранимый протухал бы от правки этих полей (тот же класс багов,
	// из-за которого до SPEC 112 протухал контент-хеш).
	//
	// Резолв строгий: обе части обязаны сойтись, иначе источник выпадает из
	// конфига fail-closed (трафик не уходит напрямую) — см.
	// config.resolveNodeDetours. Смену идентичности узла (переименование
	// node_tag) UI отрабатывает сам: сбрасывает ссылки и сообщает об этом.
	//
	// Пустой DetourNodeSourceID при непустом DetourNodeTag — переходная форма
	// (dev-состояния между SPEC 112 и 112-A): тег трактуется как ФИНАЛЬНЫЙ и
	// ищется глобально.
	//
	// Взаимоисключимы с DetourTag — пикер ставит одно и гасит другое; при
	// ручной правке, оставившей оба, побеждает ссылка на узел. DetourNodeLabel
	// — снимок подписи узла на момент выбора, только для показа.
	DetourNodeSourceID string `json:"detour_node_source_id,omitempty"`
	DetourNodeTag      string `json:"detour_node_tag,omitempty"`

	// DetourNodeHash — УПРАЗДНЁННАЯ ссылка по контент-хешу (SPEC 101).
	// Читается только ради миграции состояний, записанных до SPEC 112:
	// генератор опознаёт по нему узел и переписывает ссылку в пару
	// DetourNodeSourceID+DetourNodeTag, а не опознав — берёт DetourNodeLabel
	// как тег (уже без source_id). После миграции не пишется.
	DetourNodeHash  string `json:"detour_node_hash,omitempty"`
	DetourNodeLabel string `json:"detour_node_label,omitempty"`

	// DisabledNodes — SPEC 094 D4: per-node off switch, keyed by the node's
	// IDENTITY and valued with the unix time the mark was last confirmed.
	//
	// Identity is the node's raw provider tag, uniquified within the source and
	// taken before this source's tag_prefix / tag_mask (SPEC 112). The tag is
	// the name the provider manages the node by, so the mark survives the
	// provider rotating the server behind that name, and editing the source's
	// tag policy does not move the marks. Keys written before SPEC 112 (64
	// lowercase hex of the abolished content hash) are migrated to tag keys on
	// the first parse. Marks for nodes gone from the subscription longer than
	// the TTL are garbage-collected, otherwise the map would grow forever.
	DisabledNodes map[string]int64 `json:"disabled_nodes,omitempty"`

	// ForeignExtensions — per-entity блобы ЧУЖИХ приложений из LX Backup
	// (BACKUP.md §1: `extensions.lxbox` может лежать и внутри записи
	// подписки/сервера). Хранятся нетронутыми до следующего экспорта и
	// возвращаются в ту же запись: бэкап, побывавший на десктопе, не должен
	// вернуться на телефон без mobile-only полей (import_rules,
	// identity_override, папки).
	ForeignExtensions map[string]json.RawMessage `json:"foreign_extensions,omitempty"`
}

// TagSpec — правила преобразования тэгов нод подписки.
//
//	tag = mask           если mask != ""
//	tag = prefix + tag + postfix  иначе
//
// Поддерживаются переменные (`{$tag}`, `{$server}`, ...) — обрабатываются
// в core/config/subscription.applyTagPrefixPostfix.
type TagSpec struct {
	Prefix  string `json:"prefix,omitempty"`
	Postfix string `json:"postfix,omitempty"`
	Mask    string `json:"mask,omitempty"`
}

// IsZero возвращает true, если все три поля пустые (нечего применять).
func (t *TagSpec) IsZero() bool {
	if t == nil {
		return true
	}
	return t.Prefix == "" && t.Postfix == "" && t.Mask == ""
}

// NodeTagOrLabel — системный тег узла источника (type=server / type=chain).
//
// Откат на Label при пустом NodeTag — не удобство, а миграция: состояния,
// записанные до разделения ролей, несут тег именно в Label, и переписывать
// их на загрузке нельзя (файл делят с более старыми сборками). Пустой
// NodeTag поэтому читается как «тег равен Label» — прежнее поведение слово
// в слово, — а заполненный побеждает.
func (s Source) NodeTagOrLabel() string {
	if s.NodeTag != "" {
		return s.NodeTag
	}
	return s.Label
}

// UpdateSpec — настройки авто-обновления per-subscription. nil → используются
// global defaults (Connections.Defaults.Reload).
type UpdateSpec struct {
	IntervalHours int   `json:"interval_hours,omitempty"`
	AutoRefresh   *bool `json:"auto_refresh,omitempty"` // nil → true (default включён)
}

// SubscriptionMeta — runtime-данные подписки, заполняются Update'ом.
//
// Headers parsed из HTTP response + inline "#header: value" в первых строках
// тела (LxBox-совместимый контракт; см. SPEC 052 §"Headers контракт").
type SubscriptionMeta struct {
	// headers (HTTP response + inline #-comments в body первой строкой)
	ProfileTitle               string    `json:"profile_title,omitempty"`
	ProfileUpdateIntervalHours int       `json:"profile_update_interval_hours,omitempty"`
	SupportURL                 string    `json:"support_url,omitempty"`
	ProfileWebPageURL          string    `json:"profile_web_page_url,omitempty"`
	ContentDispositionFilename string    `json:"content_disposition_filename,omitempty"`
	UserInfo                   *UserInfo `json:"userinfo,omitempty"`

	// fetch history
	URLAtFetch     string `json:"url_at_fetch,omitempty"`    // URL на момент fetch'а
	LastFetchedAt  string `json:"last_fetched_at,omitempty"` // RFC3339 UTC
	LastStatus     string `json:"last_status,omitempty"`     // "ok" | "err"
	ErrorCount     int    `json:"error_count,omitempty"`     // подряд (resets на success)
	LastErrorMsg   string `json:"last_error_msg,omitempty"`
	HTTPStatusCode int    `json:"http_status_code,omitempty"`
	RawBodyBytes   int64  `json:"raw_body_bytes,omitempty"`

	// nodes
	NodesCountFetched int      `json:"nodes_count_fetched,omitempty"`
	Truncated         bool     `json:"truncated,omitempty"` // обрезали по max_nodes
	PreviewNodes      []string `json:"preview_nodes,omitempty"`

	// SPEC 061: provider sent announce headers (success **or** failure).
	// nil → no announce on last fetch. UI renders ⚠ icon when LastStatus="err"
	// and 📢 icon when LastStatus="ok" but this field is non-nil. Cleared on
	// a clean successful refresh with no announce headers.
	ProviderAnnounce *ProviderAnnounce `json:"provider_announce,omitempty"`

	// LastErrorURL — convenience snapshot of ProviderAnnounce.URL (or other
	// actionable URL on the last error). Surfaces in simpler UI affordances
	// (status tooltip, log message) without having to dereference
	// ProviderAnnounce. Empty on success or when provider gave no URL.
	LastErrorURL string `json:"last_error_url,omitempty"`
}

// UserInfo — раскрытый subscription-userinfo header (V2Board / Xboard).
//
//	"upload=N; download=N; total=N; expire=UNIX"
type UserInfo struct {
	UploadBytes   int64 `json:"upload_bytes,omitempty"`
	DownloadBytes int64 `json:"download_bytes,omitempty"`
	TotalBytes    int64 `json:"total_bytes,omitempty"`
	ExpireUnix    int64 `json:"expire_unix,omitempty"`
}
