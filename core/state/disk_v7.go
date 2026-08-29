// File disk_v7.go — on-disk-схема state.json v7 (SPEC 118, этап 2).
//
// Корень ПЛОСКИЙ (SPEC Т1): обёртки connections больше нет.
//
//	{
//	  "meta":          { version: 7, schema: "sources_v7", ... },
//	  "sources":       [ {kind, tag, enabled, ...} ],   // юнион по kind
//	  "directions":    [ ... ],                          // configtypes.Direction
//	  "rules":         [ ... ],                          // как v6
//	  "vars":          [ ... ],
//	  "dns_options":   { ... },
//	  "warp_accounts": { ... }
//	}
//
// Roundtrip Load→Save→Load→Save — байт-в-байт (порядок полей struct =
// порядок ключей файла; canonical_roundtrip_test.go).
package state

import (
	"encoding/json"
	"fmt"
	"time"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// SchemaVersionV7 — формат файла state.json, который пишет Save (SPEC 118).
const SchemaVersionV7 = 7

// SchemaNameV7 — внутренний идентификатор схемы v7 (хранится в meta.schema).
// Он же — «мажор схемы» для remote-гейта (SPEC Т10, волна W7).
const SchemaNameV7 = "sources_v7"

// migrationPurgesLegacy — гейт шага 8 миграции v6→v7 (PLAN §6): снос
// raw-кэша и легаси-ключей после миграции. ВКЛЮЧЁН волной W5: моста больше
// нет, легаси-поля никем не читаются, и держать их в файле значило бы
// хранить материал, который система уже не понимает.
//
// Константа оставлена (а не заинлайнена) как явная точка порядка: снос
// выполняется ТОЛЬКО после успешной записи v7-файла (load_router).
const migrationPurgesLegacy = true

// diskStateV7 — корневая модель на диске v7. Используется ТОЛЬКО внутри
// marshalDisk / parseV7 (порядок полей = порядок ключей файла).
type diskStateV7 struct {
	Meta         MetaSection             `json:"meta"`
	Sources      []Source                `json:"sources"`
	Directions   []configtypes.Direction `json:"directions"`
	Rules        []Rule                  `json:"rules"`
	Vars         []SettingVar            `json:"vars,omitempty"`
	DNSOptions   DNSOptions              `json:"dns_options"`
	WarpAccounts *WarpAccountsSection    `json:"warp_accounts,omitempty"`
}

// parseV7 — прямой read canonical (v7) формата.
//
// Форма каждого источника прогоняется через normalizeSourceShape: лишние для
// kind'а канонические поля отбрасываются с warning (в лог), неизвестный kind —
// внятный отказ загрузки (файл от более нового мажора).
func parseV7(data []byte) (*State, error) {
	var raw diskStateV7
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("state: parse v7 json: %w", err)
	}

	for i := range raw.Sources {
		warns, err := normalizeSourceShape(&raw.Sources[i])
		for _, w := range warns {
			debuglog.DebugLog("state v7: %s", w)
		}
		if err != nil {
			return nil, err
		}
	}

	s := &State{
		Version:            raw.Meta.Version,
		Comment:            raw.Meta.Comment,
		Target:             raw.Meta.Target,
		TargetPlatform:     raw.Meta.TargetPlatform,
		TargetArch:         raw.Meta.TargetArch,
		Sources:            raw.Sources,
		Directions:         raw.Directions,
		Vars:               raw.Vars,
		Rules:              raw.Rules,
		DNS:                raw.DNSOptions,
		WarpAccounts:       raw.WarpAccounts,
		RulesLibraryMerged: true,
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.CreatedAt); err == nil {
		s.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.UpdatedAt); err == nil {
		s.UpdatedAt = t
	}

	// Legacy CustomRules view — как в v6-парсе: UI-код до Phase 6 читает его.
	s.CustomRules = legacyCustomRulesFromV6(raw.Rules)

	syncLegacyFromCanonical(s)
	normalizeNilSlices(s)
	return s, nil
}
