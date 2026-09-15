// File disk_v8.go — on-disk-схема state.json v8 (SPEC 127, одно пространство имён).
//
// Отличие от v7 — форма ЗАПИСЕЙ и два имени корневых ключей:
//
//	{
//	  "meta":       { version: 8, schema: "sources_v8", ... },
//	  "sources":    [ {kind, tag, enabled, ..., sections} ],  // юнион по kind
//	  "directions": [ ... ],                                   // configtypes.Direction
//	  "rules":      [ {kind, id?, ref?, name?, enabled, num?, refs?, vars?, body?} ],
//	  "vars":       [ ... ],
//	  "dns":        { servers, rules, final, strategy },        // бывший dns_options
//	  "warp":       { ... }                                     // бывший warp_accounts
//	}
//
// `dns` и `warp` переименованы ради бэкапа 1.0, который становится
// сериализацией состояния без маппера (SPEC 127 §4). Состав объектов тот же.
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

// SchemaVersionV8 — формат файла state.json, который пишет Save (SPEC 127).
const SchemaVersionV8 = 8

// SchemaNameV8 — внутренний идентификатор схемы v8 (хранится в meta.schema).
// Он же — «мажор схемы» для remote-гейта.
const SchemaNameV8 = "sources_v8"

// diskStateV8 — корневая модель на диске v8. Используется ТОЛЬКО внутри
// marshalDisk / parseV8 (порядок полей = порядок ключей файла).
type diskStateV8 struct {
	Meta       MetaSection             `json:"meta"`
	Sources    []Source                `json:"sources"`
	Directions []configtypes.Direction `json:"directions"`
	Rules      []Rule                  `json:"rules"`
	Vars       []SettingVar            `json:"vars,omitempty"`
	DNS        DNSOptions              `json:"dns"`
	Warp       *WarpAccountsSection    `json:"warp,omitempty"`
}

// parseV8 — прямой read canonical (v8) формата.
//
// Форма каждого источника прогоняется через normalizeSourceShape: лишние для
// kind'а канонические поля отбрасываются с warning (в лог), неизвестный kind —
// внятный отказ загрузки (файл от более нового мажора).
func parseV8(data []byte) (*State, error) {
	var raw diskStateV8
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("state: parse v8 json: %w", err)
	}
	// Плоская идентичность подписки из v8 dev-сборки волны 1 — в identity
	// (disk_v8_flat_identity.go).
	liftFlatSubscriptionIdentity(data, raw.Sources)

	for i := range raw.Sources {
		warns, err := normalizeSourceShape(&raw.Sources[i])
		for _, w := range warns {
			debuglog.DebugLog("state v8: %s", w)
		}
		if err != nil {
			return nil, err
		}
	}
	// Dev-формы ссылок сборок 1.6.0 до выпуска поднимаются до нормы NodeLink
	// здесь, при чтении (nodelink_normalize.go). Путь общий для v8 и
	// мигрированного v7; правило идемпотентно, поэтому перезапись файла на
	// загрузке не нужна.
	NormalizeNodeLinks(raw.Sources, raw.Directions)

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
		DNS:                raw.DNS,
		WarpAccounts:       raw.Warp,
		RulesLibraryMerged: true,
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.CreatedAt); err == nil {
		s.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.UpdatedAt); err == nil {
		s.UpdatedAt = t
	}

	// Legacy CustomRules view — как в v6/v7-парсе: UI-код до Phase 6 читает его.
	s.CustomRules = legacyCustomRulesFromV6(s.Rules)

	syncLegacyFromCanonical(s)
	normalizeNilSlices(s)
	return s, nil
}
