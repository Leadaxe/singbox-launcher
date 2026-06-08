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
	"runtime"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/template"
)

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
				base.Ref = ob.Ref         // preserve ref в merged для UI metadata
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
		entries, _ := ExpandPresetOutbounds(preset, nil, runtime.GOOS, runtime.GOARCH)
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
