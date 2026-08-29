// File connections.go — раздел "connections" схем v5/v6 (SPEC 052) и общие
// типы Defaults/UpdateSpec/UserInfo.
//
// SPEC 118 (этап 2): ConnectionsSection и sourceV6 — ПРИВАТНАЯ форма
// парсеров старых схем (v2–v6) и миграции. Канонический тип источника —
// state.Source (sources_v7.go); загрузка старого состояния переносит
// sourceV6 в v7-форму (adoptConnectionsV6), а легаси-поля уезжают в сайдкар
// миграции (legacySourceV6, migration_legacy_source.go) и умирают с ней.
package state

import (
	"encoding/json"

	"singbox-launcher/core/config/configtypes"
)

// ConnectionsSection — раздел подключений старых схем: sources + Направления +
// defaults. Живёт только в парсерах v2–v6 и миграции; State её больше не несёт.
type ConnectionsSection struct {
	Sources []sourceV6 `json:"sources"`

	// Outbounds — глобальные Направления (SPEC 104).
	Outbounds []configtypes.Direction `json:"direction_outbounds"`

	// LegacyOutbounds — прежний ключ `outbounds`. ТОЛЬКО чтение: состояния,
	// записанные до SPEC 104, переносятся в Outbounds при загрузке
	// (`adoptLegacyDirections`) и больше никогда не пишутся.
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

// Defaults — прежние умолчания подключений схем v2–v6.
//
// В каноне v7 умолчаний в состоянии НЕТ (SPEC Т1): они переехали в настройки
// приложения (bin/settings.json). Тип читается только парсерами старых схем
// и шагом 8 миграции, который их туда и перекладывает.
type Defaults struct {
	Reload   string `json:"reload,omitempty"`
	MaxNodes int    `json:"max_nodes,omitempty"`
}

// sourceV6 — единица подключения схем v5/v6 (бывший экспортируемый Source).
// Точная on-disk-форма старых файлов; используется ТОЛЬКО парсерами старых
// схем и миграцией. Семантику полей см. в истории (SPEC 052/077/094/108/
// 110/112); канонические собратья — в sources_v7.go.
type sourceV6 struct {
	// identity
	ID                string     `json:"id"`
	Type              SourceKind `json:"type"`
	Enabled           bool       `json:"enabled"`
	Label             string     `json:"label,omitempty"`
	ExcludeFromGlobal bool       `json:"exclude_from_global,omitempty"`

	// NodeTag — системный тег узла для type=server и type=chain
	// (пусто → тег = Label; см. Source.NodeTagOrLabel).
	NodeTag string `json:"node_tag,omitempty"`

	// type=subscription only
	URL                     string                  `json:"url,omitempty"`
	Skip                    []map[string]string     `json:"skip,omitempty"`
	Tag                     *legacyTagSpec          `json:"tag,omitempty"`
	Outbounds               []configtypes.Direction `json:"outbounds,omitempty"`
	ExposeGroupTagsToGlobal bool                    `json:"expose_group_tags_to_global,omitempty"`
	Fold                    *legacyFold             `json:"fold,omitempty"`
	Update                  *UpdateSpec             `json:"update,omitempty"`
	MaxNodes                int                     `json:"max_nodes,omitempty"`
	Meta                    *legacySubMeta          `json:"meta,omitempty"`

	// type=server only
	URI        string          `json:"uri,omitempty"`
	ConfigJSON json.RawMessage `json:"config_json,omitempty"`

	// type=chain only (SPEC 110)
	Chain *configtypes.SourceChain `json:"chain,omitempty"`

	// detour (SPEC 077 / 101 / 112 / 112-A)
	DetourTag          string `json:"detour_tag,omitempty"`
	DetourNodeSourceID string `json:"detour_node_source_id,omitempty"`
	DetourNodeTag      string `json:"detour_node_tag,omitempty"`
	DetourNodeHash     string `json:"detour_node_hash,omitempty"`
	DetourNodeLabel    string `json:"detour_node_label,omitempty"`

	// DisabledNodes — SPEC 094 D4: карта выключенных узлов по identity-ключам.
	DisabledNodes map[string]int64 `json:"disabled_nodes,omitempty"`
}

// legacyTagSpec — прежняя «tag»-спека v6: prefix/postfix плюс УПРАЗДНЁННАЯ
// маска. Канон v7 несёт только TagPolicy{prefix,postfix} (SPEC Т2); mask у
// server/chain хранила тег узла (переезжает в Node.Tag), у подписки была
// шаблоном (упраздняется с warning).
type legacyTagSpec struct {
	Prefix  string `json:"prefix,omitempty"`
	Postfix string `json:"postfix,omitempty"`
	Mask    string `json:"mask,omitempty"`
}

// legacySubMeta — прежние метаданные подписки v6: канонические заголовки
// плюс fetch-история и превью, которых в v7-мете нет (история → SubUpdateStatus,
// превью упразднено).
type legacySubMeta struct {
	ProfileTitle               string    `json:"profile_title,omitempty"`
	ProfileUpdateIntervalHours int       `json:"profile_update_interval_hours,omitempty"`
	SupportURL                 string    `json:"support_url,omitempty"`
	ProfileWebPageURL          string    `json:"profile_web_page_url,omitempty"`
	ContentDispositionFilename string    `json:"content_disposition_filename,omitempty"`
	UserInfo                   *UserInfo `json:"userinfo,omitempty"`

	URLAtFetch     string `json:"url_at_fetch,omitempty"`
	LastFetchedAt  string `json:"last_fetched_at,omitempty"`
	LastStatus     string `json:"last_status,omitempty"`
	ErrorCount     int    `json:"error_count,omitempty"`
	LastErrorMsg   string `json:"last_error_msg,omitempty"`
	HTTPStatusCode int    `json:"http_status_code,omitempty"`
	RawBodyBytes   int64  `json:"raw_body_bytes,omitempty"`

	NodesCountFetched int      `json:"nodes_count_fetched,omitempty"`
	Truncated         bool     `json:"truncated,omitempty"`
	NodePool      []string `json:"preview_nodes,omitempty"`

	ProviderAnnounce *ProviderAnnounce `json:"provider_announce,omitempty"`

	LastErrorURL string `json:"last_error_url,omitempty"`
}

// toV7 — структурный перенос sourceV6 в каноническую v7-форму: раскладка по
// новым домам (type → kind, tag-спека → tag_policy, канонические поля меты).
// Семантическая миграция (материализация nodes[], отметки, теги, хопы,
// тройня, fold → replace) идёт следом, из сайдкара legacySourceV6.
func (v sourceV6) toV7() (Source, legacySourceV6) {
	out := Source{
		Node: Node{
			Kind:    v.Type,
			Enabled: v.Enabled,
		},
		ID:       v.ID,
		URL:      v.URL,
		Skip:     v.Skip,
		MaxNodes: v.MaxNodes,
		Update:   v.Update,
	}
	if v.Tag != nil && (v.Tag.Prefix != "" || v.Tag.Postfix != "") {
		out.TagPolicy = &TagPolicy{Prefix: v.Tag.Prefix, Postfix: v.Tag.Postfix}
	}

	legacy := legacySourceV6{
		Label:                   v.Label,
		NodeTag:                 v.NodeTag,
		URI:                     v.URI,
		ConfigJSON:              v.ConfigJSON,
		Chain:                   v.Chain,
		Outbounds:               v.Outbounds,
		ExcludeFromGlobal:       v.ExcludeFromGlobal,
		ExposeGroupTagsToGlobal: v.ExposeGroupTagsToGlobal,
		Fold:                    v.Fold,
		DetourTag:               v.DetourTag,
		DetourNodeSourceID:      v.DetourNodeSourceID,
		DetourNodeTag:           v.DetourNodeTag,
		DetourNodeHash:          v.DetourNodeHash,
		DetourNodeLabel:         v.DetourNodeLabel,
		DisabledNodes:           v.DisabledNodes,
	}
	if v.Tag != nil {
		legacy.TagMask = v.Tag.Mask
	}
	if v.Meta != nil {
		out.Meta = &SubMeta{
			ProfileTitle:               v.Meta.ProfileTitle,
			ProfileUpdateIntervalHours: v.Meta.ProfileUpdateIntervalHours,
			SupportURL:                 v.Meta.SupportURL,
			ProfileWebPageURL:          v.Meta.ProfileWebPageURL,
			ContentDispositionFilename: v.Meta.ContentDispositionFilename,
			UserInfo:                   v.Meta.UserInfo,
			ProviderAnnounce:           v.Meta.ProviderAnnounce,
		}
		legacy.MetaHistory = legacySubMetaHistory{
			URLAtFetch:        v.Meta.URLAtFetch,
			LastFetchedAt:     v.Meta.LastFetchedAt,
			LastStatus:        v.Meta.LastStatus,
			ErrorCount:        v.Meta.ErrorCount,
			LastErrorMsg:      v.Meta.LastErrorMsg,
			LastErrorURL:      v.Meta.LastErrorURL,
			HTTPStatusCode:    v.Meta.HTTPStatusCode,
			RawBodyBytes:      v.Meta.RawBodyBytes,
			NodesCountFetched: v.Meta.NodesCountFetched,
			Truncated:         v.Meta.Truncated,
		}
	}
	return out, legacy
}

// adoptConnectionsV6 — перенос секции connections старых схем в плоский
// v7-корень State. Возвращает сайдкар легаси-полей (вход миграции), который
// живёт ровно до конца Load.
func adoptConnectionsV6(s *State, cs ConnectionsSection) []legacySourceV6 {
	s.Sources = make([]Source, 0, len(cs.Sources))
	legacy := make([]legacySourceV6, 0, len(cs.Sources))
	for _, src := range cs.Sources {
		v7, leg := src.toV7()
		s.Sources = append(s.Sources, v7)
		legacy = append(legacy, leg)
	}
	s.Directions = cs.Outbounds
	s.Defaults = cs.Defaults
	return legacy
}

// UpdateSpec — настройки авто-обновления per-subscription. nil → используются
// умолчания настроек приложения (bin/settings.json).
type UpdateSpec struct {
	IntervalHours int   `json:"interval_hours,omitempty"`
	AutoRefresh   *bool `json:"auto_refresh,omitempty"` // nil → true (default включён)
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
