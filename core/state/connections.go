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
type Source struct {
	// identity
	ID                string     `json:"id"`
	Type              SourceType `json:"type"`
	Enabled           bool       `json:"enabled"`
	Label             string     `json:"label,omitempty"`
	ExcludeFromGlobal bool       `json:"exclude_from_global,omitempty"`

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

	// DetourNodeHash / DetourNodeLabel — SPEC 101: chain through one concrete
	// node addressed by its identity hash (stable across provider renames and
	// reorders, like DisabledNodes keys). Mutually exclusive with DetourTag —
	// the picker sets one and clears the other; hash wins if both survive a
	// hand edit. The label is a display snapshot of the picked node's tag,
	// never used for resolution. Unresolved hash at build time drops this
	// source's nodes (fail-closed), see config.resolveNodeHashDetours.
	DetourNodeHash  string `json:"detour_node_hash,omitempty"`
	DetourNodeLabel string `json:"detour_node_label,omitempty"`

	// DisabledNodes — SPEC 094 D4: per-node off switch, keyed by the node's
	// identity hash and valued with the unix time the mark was last confirmed.
	//
	// Keyed by hash rather than tag or position: providers rename nodes between
	// refreshes and reorder them freely, so a tag-keyed mark would silently move
	// to a different server. Marks for nodes gone from the subscription longer
	// than the TTL are garbage-collected, otherwise the map would grow forever.
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
