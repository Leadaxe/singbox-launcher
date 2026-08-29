// File connections.go — раздел "connections" схем v5/v6 (SPEC 052) и общие
// типы Defaults/UpdateSpec/UserInfo.
//
// SPEC 118 (этап 2, W1): ConnectionsSection и sourceV6 — ПРИВАТНАЯ форма
// парсеров старых схем (v2–v6) и будущей миграции W2. Канонический тип
// источника — state.Source (sources_v7.go); загрузка старого состояния
// переносит sourceV6 в v7-форму структурно (adoptConnectionsV6, disk_v7.go),
// легаси-поля доезжают в мостовые деривативы TEMPORARY BRIDGE.
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

// Defaults — настройки по умолчанию для всех source'ов.
//
// TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5: в каноне v7 умолчаний в
// state нет (они переезжают в настройки приложения, миграция W2 / волна W5);
// до переезда значения живут в State.Defaults и пишутся в v7-файл под
// мостовым ключом legacy_defaults.
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
	Tag                     *TagPolicy              `json:"tag,omitempty"`
	Outbounds               []configtypes.Direction `json:"outbounds,omitempty"`
	ExposeGroupTagsToGlobal bool                    `json:"expose_group_tags_to_global,omitempty"`
	Fold                    *configtypes.SourceFold `json:"fold,omitempty"`
	Update                  *UpdateSpec             `json:"update,omitempty"`
	MaxNodes                int                     `json:"max_nodes,omitempty"`
	Meta                    *SubMeta                `json:"meta,omitempty"`

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

// toV7 — структурный перенос sourceV6 в каноническую v7-форму (SPEC 118 W1).
//
// БЕЗ семантической миграции: шаги 1–7 (материализация nodes[], перенос
// отметок/тегов/хопов/тройни/fold) — волна W2. Здесь только раскладка по
// новым домам: type → kind, tag-спека → tag_policy; всё остальное едет как
// есть в мостовые поля TEMPORARY BRIDGE, из которых adapter_source.go
// деривирует прежнюю ProxySource-форму (поведение сборки не меняется).
func (v sourceV6) toV7() Source {
	return Source{
		Node: Node{
			Kind:    v.Type,
			Enabled: v.Enabled,
		},
		ID:        v.ID,
		TagPolicy: v.Tag,
		URL:       v.URL,
		Skip:      v.Skip,
		MaxNodes:  v.MaxNodes,
		Update:    v.Update,
		Meta:      v.Meta,

		// TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5.
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
}

// adoptConnectionsV6 — перенос секции connections старых схем в плоский
// v7-корень State (структурно, см. sourceV6.toV7).
func adoptConnectionsV6(s *State, cs ConnectionsSection) {
	s.Sources = make([]Source, 0, len(cs.Sources))
	for _, src := range cs.Sources {
		s.Sources = append(s.Sources, src.toV7())
	}
	s.Directions = cs.Outbounds
	s.Defaults = cs.Defaults
}

// UpdateSpec — настройки авто-обновления per-subscription. nil → используются
// глобальные умолчания (State.Defaults.Reload; после W5 — настройки
// приложения).
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
