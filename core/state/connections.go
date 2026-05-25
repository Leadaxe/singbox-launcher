// File connections.go — раздел "connections" в state.json (SPEC 052).
// Types: ConnectionsSection, Source, Defaults, SourceType, TagSpec, UpdateSpec,
// SubscriptionMeta, UserInfo.
package state

import (
	"singbox-launcher/core/config/configtypes"
)

// ConnectionsSection — раздел подключений: sources + global outbounds + defaults.
type ConnectionsSection struct {
	Sources   []Source                     `json:"sources"`
	Outbounds []configtypes.OutboundConfig `json:"outbounds"`
	Defaults  Defaults                     `json:"defaults"`
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
)

// Source — единица подключения. Тип определяет, какие поля используются:
//
//   - SourceTypeSubscription: URL/Skip/Tag/Outbounds/Update/MaxNodes/Meta
//   - SourceTypeServer:       URI; Tag/Update/Meta не используются
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
	URL                     string                       `json:"url,omitempty"`
	Skip                    []map[string]string          `json:"skip,omitempty"`
	Tag                     *TagSpec                     `json:"tag,omitempty"`
	Outbounds               []configtypes.OutboundConfig `json:"outbounds,omitempty"`
	ExposeGroupTagsToGlobal bool                         `json:"expose_group_tags_to_global,omitempty"`
	Update                  *UpdateSpec                  `json:"update,omitempty"`
	MaxNodes                int                          `json:"max_nodes,omitempty"`
	Meta                    *SubscriptionMeta            `json:"meta,omitempty"`

	// type=server only
	URI string `json:"uri,omitempty"`
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
