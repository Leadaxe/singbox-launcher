// Package v5 — финальная (SPEC 052) on-disk-схема state.json.
//
// Этот пакет — leaf: импортирует только configtypes (для OutboundConfig)
// и стандартную библиотеку. core/state будет импортировать v5 и
// re-export'ить алиасы для backward-compat callsite'ов (Phase 4).
//
// Top-level layout:
//
//	{
//	  "meta":         { version, comment, created_at, updated_at },
//	  "connections":  { sources, outbounds, defaults },
//	  "config_params": [...],
//	  "custom_rules":  [...],
//	  "vars":          [...],
//	  "dns_options":   {...}
//	}
package v5

import (
	"encoding/json"

	"singbox-launcher/core/config/configtypes"
)

// SchemaVersion — формат файла state.json, который пишет v5.
const SchemaVersion = 5

// DefaultMaxNodes — дефолтный потолок числа нод per-source. Зеркалит
// configtypes.MaxNodesPerSubscription (3000), но живёт здесь чтобы
// migration v4→v5 не зависела от парсера.
const DefaultMaxNodes = 3000

// State — корневая модель v5.
//
// Любые изменения требуют bump'а SchemaVersion и явной миграции.
type State struct {
	Meta         MetaSection        `json:"meta"`
	Connections  ConnectionsSection `json:"connections"`
	ConfigParams []ConfigParam      `json:"config_params"`
	CustomRules  []CustomRule       `json:"custom_rules"`
	Vars         []SettingVar       `json:"vars,omitempty"`
	DNSOptions   *DNSOptions        `json:"dns_options"`
}

// MetaSection — метаинформация state'а (version + timestamps).
type MetaSection struct {
	Version   int    `json:"version"`
	Comment   string `json:"comment,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

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
	URLAtFetch     string `json:"url_at_fetch,omitempty"`     // URL на момент fetch'а
	LastFetchedAt  string `json:"last_fetched_at,omitempty"`  // RFC3339 UTC
	LastStatus     string `json:"last_status,omitempty"`      // "ok" | "err"
	ErrorCount     int    `json:"error_count,omitempty"`      // подряд (resets на success)
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

// ──────────────────────────────────────────────────────────────────
// User-rule типы (config_params, custom_rules, vars, dns_options)
//
// Дублируют состав core/state.* с идентичными JSON-тегами. В Phase 4
// core/state будет импортировать v5 и re-export'ить эти типы как
// алиасы; пока они живут здесь чтобы пакет v5 был самодостаточным
// для миграции и golden-тестов.
// ──────────────────────────────────────────────────────────────────

// ConfigParam — параметр маршрутизации (например, route.final).
type ConfigParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SettingVar — переопределение переменной шаблона (вкладка Settings).
type SettingVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CustomRule — пользовательское правило с полным определением.
type CustomRule struct {
	Label            string                 `json:"label"`
	Type             string                 `json:"type,omitempty"`
	Enabled          bool                   `json:"enabled"`
	SelectedOutbound string                 `json:"selected_outbound"`
	Description      string                 `json:"description,omitempty"`
	Rule             map[string]interface{} `json:"rule,omitempty"`
	DefaultOutbound  string                 `json:"default_outbound,omitempty"`
	HasOutbound      bool                   `json:"has_outbound"`
	Params           map[string]interface{} `json:"params,omitempty"`
	RuleSet          []json.RawMessage      `json:"rule_set,omitempty"`
}

// DNSOptions — снимок вкладки DNS визарда (servers + rules + skalary).
type DNSOptions struct {
	Servers               []json.RawMessage `json:"servers"`
	Rules                 []json.RawMessage `json:"rules,omitempty"`
	Final                 string            `json:"final,omitempty"`
	Strategy              string            `json:"strategy,omitempty"`
	IndependentCache      *bool             `json:"independent_cache,omitempty"`
	DefaultDomainResolver string            `json:"default_domain_resolver,omitempty"`
	ResolverUnset         bool              `json:"default_domain_resolver_unset,omitempty"`
}

// ──────────────────────────────────────────────────────────────────
// Известные константы типов правил (зеркало core/state.RuleType*).
// ──────────────────────────────────────────────────────────────────

const (
	RuleTypeIPS       = "ips"
	RuleTypeURLs      = "urls"
	RuleTypeProcesses = "processes"
	RuleTypeSRS       = "srs"
	RuleTypeRaw       = "raw"
)

// IsKnownRuleType возвращает true, если s — одна из актуальных констант типов.
func IsKnownRuleType(s string) bool {
	switch s {
	case RuleTypeIPS, RuleTypeURLs, RuleTypeProcesses, RuleTypeSRS, RuleTypeRaw:
		return true
	default:
		return false
	}
}
