// File disk_v8.go — on-disk-схема state.json v8/v9 (SPEC 127, одно пространство
// имён; v9 — SPEC 188: `refs` внутри тела правила).
//
// Отличие от v7 — форма ЗАПИСЕЙ и два имени корневых ключей:
//
//	{
//	  "meta":       { version: 8, schema: "sources_v8", ... },
//	  "sources":    [ {kind, tag, enabled, ...} ],  // юнион по kind
//	  "directions": [ ... ],                                   // configtypes.Direction
//	  "rules":      [ {kind, id?, ref?, name?, enabled, num?, vars?, body?} ],  // v9: refs в body
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
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// SchemaVersionV8 — формат файла state.json SPEC 127: ссылки наборов правила
// — поле записи `refs`. Читается тем же разбором, что v9.
const SchemaVersionV8 = 8

// SchemaVersionV9 — формат файла state.json, который пишет Save (SPEC 188,
// контракт 1.1.179): `refs` — ключ простого правила внутри `body`, `kind`
// выводится из дерева. Остальная форма — как у v8. Мажор поднят ради гейта
// переноса состояния машин (CheckSchemaCompatible): сборка, знающая только
// v8, прочитала бы v9 молча и выпустила бы `refs` ядру.
const SchemaVersionV9 = 9

// SchemaNameV8 — внутренний идентификатор схемы v8 (хранится в meta.schema
// строкой).
const SchemaNameV8 = "sources_v8"

// SchemaNameV9 — имя схемы v9 (поле name объекта meta.schema).
const SchemaNameV9 = "sources_v9"

// SchemaMetaV9 — значение meta.schema файла v9: ОБЪЕКТ, а не строка. Сборки до
// v9 (2.4.x и develop до SPEC 188) читают любой meta.version >= 8 разбором v8
// и ссылки srs-правил ищут на записи: v9 они прочли бы молча, с потерей всех
// srs-правил при первом сохранении Конфигуратора. На объекте в строковом поле
// их разбор падает с ошибкой — файл отвергается целиком. min_reader — мажор,
// который файл читать умеет.
func SchemaMetaV9() json.RawMessage {
	return json.RawMessage(`{"name":"` + SchemaNameV9 + `","min_reader":` + strconv.Itoa(SchemaVersionV9) + `}`)
}

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

// parseV8 — прямой read canonical формата v8 и v9 (у v9 отличается только
// место `refs` правила, его выравнивает NormalizeRuleRecords).
//
// Форма каждого источника прогоняется через normalizeSourceShape: лишние для
// kind'а канонические поля отбрасываются с warning (в лог), неизвестный kind —
// внятный отказ загрузки (файл от более нового мажора).
func parseV8(data []byte) (*State, error) {
	var raw diskStateV8
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("state: parse v8 json: %w", err)
	}
	logDroppedNodeSections(data)
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
	// Контракт 1.1.126: ключ записи skip_presets → skip_tailscale_presets.
	// Здесь, а не в Load: путь общий для Load, Parse и мигрированного v7;
	// файл перепишется новым ключом при следующем Save.
	if n := normalizeNodeRecordFlags(raw.Sources); n > 0 {
		debuglog.InfoLog("state v8: %d node records: skip_presets read as skip_tailscale_presets", n)
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
		LatePresetsSeeded:  raw.Meta.LatePresetsSeeded,
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

	// SPEC 188: `refs` записи (v8) → тело корня, `kind` из дерева — в памяти,
	// файл v9 перепишет ближайшее сохранение. До миграции тегов ниже: та
	// читает ссылки правила через RuleRefs, ей форма не важна.
	if n := NormalizeRuleRecords(s.Rules); n > 0 {
		debuglog.InfoLog("state: %d rules: rule set refs moved into the rule body", n)
	}

	// Ссылки на старые теги наборов (`user:<id>`, `user:<id>:N`) переводятся
	// на теги §6.13 (имя правила, SPEC 188) — в памяти, файл перепишет
	// ближайшее сохранение. После нормализации: ссылки правила — из дерева.
	if n := MigrateRuleSetTagRefsV9(s.Rules, s.DNS.Rules); n > 0 {
		debuglog.InfoLog("state: %d rule_set references moved to the rule-name tags", n)
	}

	// Legacy CustomRules view — как в v6/v7-парсе: UI-код до Phase 6 читает его.
	s.CustomRules = legacyCustomRulesFromV6(s.Rules)

	syncLegacyFromCanonical(s)
	normalizeNilSlices(s)
	return s, nil
}

// logDroppedNodeSections — у v8-файла, записанного до контракта 1.1.85, у
// узлов может остаться поле `sections`. Секции узла упразднены (SPEC 144,
// LxBox §575): разбор их не читает, а в журнал ложится строка на запись.
func logDroppedNodeSections(data []byte) {
	if !bytes.Contains(data, []byte(`"sections"`)) {
		return
	}
	type rec struct {
		Tag      string          `json:"tag"`
		Sections json.RawMessage `json:"sections"`
		Nodes    []rec           `json:"nodes"`
	}
	var doc struct {
		Sources []rec `json:"sources"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return
	}
	for i, src := range doc.Sources {
		if len(src.Sections) > 0 && string(src.Sections) != "null" {
			debuglog.WarnLog("state v8: sources[%d] %q: node sections dropped (no longer supported)", i, src.Tag)
		}
		for j, n := range src.Nodes {
			if len(n.Sections) > 0 && string(n.Sections) != "null" {
				debuglog.WarnLog("state v8: sources[%d].nodes[%d] %q: node sections dropped (no longer supported)", i, j, n.Tag)
			}
		}
	}
}
