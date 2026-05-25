// Package build — File resolve_outbounds.go (SPEC 057/058-R-N).
//
// Resolver для outbounds — параллельно resolve_dns.go / resolve_route.go.
// Pure func: state.connections.outbounds[] + template → merged view с meta-info.
//
// **Принципы (SPEC 058-R-N STATE_AS_TEMPLATE_DIFF):**
//
//	state.connections.outbounds[] entries делятся на:
//	  - **Direct** (Ref="")          — self-contained body живёт inline в state.
//	  - **Referenced template** (Ref="#TEMPLATE#") — body live из
//	    template.parser_config.outbounds[tag].
//	  - **Referenced preset** (Ref="<preset_id>") — body live из
//	    template.presets[id].outbounds (mode=add) для этого tag.
//
//	Updates[] стек применяется поверх resolved base в order:
//	  - preset patches (mode=update) — в rule order
//	  - USER patch (ref="#USER#") — всегда последним
//
// **Build emit:** runtime path вызывает SyncOutboundsWithActivePresets →
// MergeOutboundUpdatesInPlace до GenerateOutboundsFromParserConfig. Sync
// поддерживает state shape (template entries thin); Merge резолвит body из
// template для referenced entries и flatten'ит Updates[] стек в финальный body.
package build

import (
	"encoding/json"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/template"
)

// OutboundSource — discriminator происхождения outbound entry.
type OutboundSource string

const (
	// OutboundSourceDirect — direct entry (Ref=""), self-contained body inline.
	// Создаётся юзером через Add. Не связан с template/preset.
	OutboundSourceDirect OutboundSource = "direct"

	// OutboundSourceTemplate — referenced template entry (Ref="#TEMPLATE#").
	// Body live из template.parser_config.outbounds[tag].
	OutboundSourceTemplate OutboundSource = "template"

	// OutboundSourcePreset — referenced preset entry (Ref="<preset_id>").
	// Body live из template.presets[id].outbounds (mode=add).
	OutboundSourcePreset OutboundSource = "preset"
)

// ResolvedOutbound — одна entry финального outbounds list'а.
//
// Body = merged result (template/preset/inline base + apply Updates в order).
// Для UI display и build emit.
type ResolvedOutbound struct {
	// Body — готовое sing-box outbound тело (after resolve base + merge updates).
	Body configtypes.OutboundConfig

	// Source — direct / template / preset.
	Source OutboundSource

	// IndexInSlice — индекс в state.connections.outbounds[]. Stable order.
	IndexInSlice int

	// Ref — копия ob.Ref из state ("" | "#TEMPLATE#" | "<preset_id>").
	Ref string

	// PresetLabel — UI label preset'а (только для Source=preset). Пусто иначе.
	PresetLabel string

	// Updates — снимок Updates стека (для UI hover/inspect). Не используется
	// для emit; только metadata.
	Updates []configtypes.OutboundUpdate

	// HasPresetUpdates — true если на этот outbound применены updates от
	// active preset'ов (хотя бы один update.ref ≠ RefUser).
	HasPresetUpdates bool

	// HasUserPatch — true если в Updates[] есть entry с ref=RefUser
	// (пользовательский field-level diff поверх resolved base, SPEC 058).
	HasUserPatch bool

	// Required — true если template маркирует этот tag как `required: true`.
	// Live lookup из template (как в SPEC 056-R-N Phase E). Релевантно только
	// для Source=template — для direct/preset всегда false.
	Required bool

	// Resolved — true если base body удалось найти (для referenced entries).
	// Для direct всегда true. Для referenced с broken ref (preset disabled,
	// template tag исчез) — false; entry должен быть dropped через sync.
	Resolved bool
}

// ResolvedOutbounds — результат ResolveOutbounds.
type ResolvedOutbounds struct {
	Globals []ResolvedOutbound // включая referenced (template + preset) entries
}

// ResolveOutbounds — единая точка резолва outbounds section (SPEC 057/058-R-N).
//
// Аргументы:
//   - outbounds — state.connections.outbounds[] (state-of-truth)
//   - td — template data; nil для legacy fallback (referenced entries дают
//     Resolved=false, body = состояние ob как есть — обычно почти пустой для
//     referenced template entries; UI render для них degraded)
//
// Возвращает структурированный view. Merged body вычисляется на лету:
// resolve base (template/preset/inline) + apply each Update в order.
func ResolveOutbounds(
	outbounds []configtypes.OutboundConfig,
	td *template.TemplateData,
) ResolvedOutbounds {
	tmplOutbounds := td.GlobalOutbounds() // nil-safe
	presetByID := make(map[string]*template.Preset)
	if td != nil {
		for i := range td.Presets {
			presetByID[td.Presets[i].ID] = &td.Presets[i]
		}
	}
	requiredTags := td.RequiredOutboundTags() // nil-safe

	out := ResolvedOutbounds{Globals: make([]ResolvedOutbound, 0, len(outbounds))}
	for i, ob := range outbounds {
		base, resolved := resolveBaseBody(ob, tmplOutbounds, presetByID)
		merged := applyUpdatesToBase(base, ob.Updates)

		source := classifySource(ob.Ref)

		var presetLabel string
		if source == OutboundSourcePreset {
			if p, ok := presetByID[ob.Ref]; ok {
				presetLabel = p.Label
				if presetLabel == "" {
					presetLabel = p.ID
				}
			} else {
				presetLabel = ob.Ref // dangling
			}
		}

		hasPresetUpd, hasUserPatch := summarizeUpdates(ob.Updates)

		out.Globals = append(out.Globals, ResolvedOutbound{
			Body:             merged,
			Source:           source,
			IndexInSlice:     i,
			Ref:              ob.Ref,
			PresetLabel:      presetLabel,
			Updates:          ob.Updates,
			HasPresetUpdates: hasPresetUpd,
			HasUserPatch:     hasUserPatch,
			Required:         requiredTags[ob.Tag],
			Resolved:         resolved,
		})
	}
	return out
}

// classifySource — переводит Ref в OutboundSource enum.
func classifySource(ref string) OutboundSource {
	switch {
	case ref == "":
		return OutboundSourceDirect
	case ref == configtypes.RefTemplate:
		return OutboundSourceTemplate
	default:
		return OutboundSourcePreset
	}
}

// summarizeUpdates — возвращает (hasPresetPatch, hasUserPatch).
func summarizeUpdates(updates []configtypes.OutboundUpdate) (bool, bool) {
	var hasPreset, hasUser bool
	for _, u := range updates {
		if u.Ref == configtypes.RefUser {
			hasUser = true
		} else if u.Ref != "" {
			hasPreset = true
		}
	}
	return hasPreset, hasUser
}

// resolveBaseBody — для referenced entry возвращает base body из template/preset.
// Для direct entry возвращает ob как есть.
//
// Returns: (base, resolved). resolved=false означает referenced entry с broken
// ref (template tag исчез или preset disabled/missing); caller обычно дропает
// такие entries через sync. Body в этом случае = ob (degraded view для UI).
func resolveBaseBody(
	ob configtypes.OutboundConfig,
	tmplOutbounds []configtypes.OutboundConfig,
	presetByID map[string]*template.Preset,
) (configtypes.OutboundConfig, bool) {
	switch ob.Ref {
	case "":
		// Direct entry — body inline.
		return ob, true
	case configtypes.RefTemplate:
		// Referenced template — lookup body из template.parser_config.outbounds[tag].
		for _, t := range tmplOutbounds {
			if t.Tag == ob.Tag {
				base := t
				base.Ref = ob.Ref       // preserve ref в merged для UI metadata
				base.Updates = ob.Updates // preserve updates stack (will be applied)
				return base, true
			}
		}
		return ob, false
	default:
		// Referenced preset — lookup body из template.presets[ref].outbounds[mode=add, tag].
		preset, ok := presetByID[ob.Ref]
		if !ok {
			return ob, false
		}
		// Expand с дефолтными vars (нам нужен только outbound shape; vars
		// substitution для emit делает sync function).
		entries, _ := ExpandPresetOutbounds(preset, nil)
		for _, entry := range entries {
			if entry.Mode == "add" && entry.Config.Tag == ob.Tag {
				base := entry.Config
				base.Ref = ob.Ref
				base.Updates = ob.Updates
				return base, true
			}
		}
		return ob, false
	}
}

// applyUpdatesToBase — applies Updates[] stack к resolved base.
// Returns копию (не мутирует input).
func applyUpdatesToBase(base configtypes.OutboundConfig, updates []configtypes.OutboundUpdate) configtypes.OutboundConfig {
	merged := base
	merged.Updates = nil // metadata, не пишется в config.json

	for _, u := range updates {
		merged = applyOutboundUpdatePatch(merged, u.Patch)
	}
	return merged
}

// MergeOutboundUpdates — exported wrapper для per-entry merge (UI preview,
// dialog Edit и т.п.). Возвращает копию outbound с resolved body и
// применёнными Updates[] патчами.
//
// td может быть nil — тогда referenced entries будут degraded (body = ob.body
// без template lookup). Direct entries работают всегда.
func MergeOutboundUpdates(ob configtypes.OutboundConfig, td *template.TemplateData) configtypes.OutboundConfig {
	return mergeOutboundUpdates(ob, td)
}

// mergeOutboundUpdates — вычисляет merged outbound body: resolve base
// (template/preset/inline) + apply Updates в order. Возвращает копию.
func mergeOutboundUpdates(ob configtypes.OutboundConfig, td *template.TemplateData) configtypes.OutboundConfig {
	tmplOutbounds := td.GlobalOutbounds()
	presetByID := make(map[string]*template.Preset)
	if td != nil {
		for i := range td.Presets {
			presetByID[td.Presets[i].ID] = &td.Presets[i]
		}
	}
	base, _ := resolveBaseBody(ob, tmplOutbounds, presetByID)
	return applyUpdatesToBase(base, ob.Updates)
}

// MergeOutboundUpdatesInPlace — runtime helper: walks parserCfg.Outbounds[] и
// для каждой entry резолвит base (template/preset/inline) + flatten'ит
// Updates[] стек в финальное body. Mutates in-place.
//
// Используется build runtime path'ами (rebuild_raw_cache,
// UpdateConfigFromSubscriptions, UI parseAndPreview) ПОСЛЕ
// SyncOutboundsWithActivePresets — sync кладёт thin referenced entries с
// Ref+Updates, этот helper resolves bodies и материализует patches в финальный
// body для GenerateOutboundsFromParserConfig (который про Ref/Updates не знает).
//
// td может быть nil — fallback на existing body (SPEC 057 shape). Это нужно для
// legacy state без миграции и для тестов.
//
// Idempotent: повторный вызов после первого даёт тот же результат.
func MergeOutboundUpdatesInPlace(parserCfg *configtypes.ParserConfig, td *template.TemplateData) {
	if parserCfg == nil {
		return
	}
	tmplOutbounds := td.GlobalOutbounds()
	presetByID := make(map[string]*template.Preset)
	if td != nil {
		for i := range td.Presets {
			presetByID[td.Presets[i].ID] = &td.Presets[i]
		}
	}
	for i := range parserCfg.ParserConfig.Outbounds {
		ob := parserCfg.ParserConfig.Outbounds[i]
		if ob.Ref == "" && len(ob.Updates) == 0 {
			continue // direct без patches — nothing to do
		}
		base, _ := resolveBaseBody(ob, tmplOutbounds, presetByID)
		parserCfg.ParserConfig.Outbounds[i] = applyUpdatesToBase(base, ob.Updates)
	}
}

// applyOutboundUpdatePatch — применяет один patch (map) к target outbound.
// Тонкая обёртка вокруг applyOutboundUpdate(target, patch OutboundConfig)
// для удобства работы с map-форматом из OutboundUpdate.Patch.
//
// Конвертирует map → OutboundConfig (через JSON marshal/unmarshal на patch
// keys) → вызывает existing applyOutboundUpdate → возвращает результат.
//
// Если patch не парсится — возвращает target без изменений (safe noop).
func applyOutboundUpdatePatch(target configtypes.OutboundConfig, patch map[string]interface{}) configtypes.OutboundConfig {
	if len(patch) == 0 {
		return target
	}
	patchJSON, err := json.Marshal(patch)
	if err != nil {
		return target
	}
	var patchOC configtypes.OutboundConfig
	if err := json.Unmarshal(patchJSON, &patchOC); err != nil {
		return target
	}
	return applyOutboundUpdate(target, patchOC)
}
