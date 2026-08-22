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
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
)

// resolveBaseBody — для referenced entry возвращает base body из template/preset.
// Для direct entry возвращает ob как есть.
//
// Returns: (base, resolved). resolved=false означает referenced entry с broken
// ref (template tag исчез или preset disabled/missing); caller обычно дропает
// такие entries через sync. Body в этом случае = ob (degraded view для UI).
func resolveBaseBody(
	ob configtypes.Direction,
	tmplOutbounds []configtypes.Direction,
	presetByID map[string]*template.Preset,
	target template.TargetSpec,
) (configtypes.Direction, bool) {
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
		entries, _ := ExpandPresetOutbounds(preset, nil, target)
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
func applyUpdatesToBase(base configtypes.Direction, updates []configtypes.OutboundUpdate) configtypes.Direction {
	merged := base
	merged.Updates = nil // metadata, не пишется в config.json

	for _, u := range updates {
		merged = applyOutboundUpdatePatch(merged, u.Patch, u.Ref == configtypes.RefUser)
	}
	return merged
}

// MergeOutboundUpdates — exported wrapper для per-entry merge (UI preview,
// dialog Edit и т.п.). Возвращает копию outbound с resolved body и
// применёнными Updates[] патчами.
//
// td может быть nil — тогда referenced entries будут degraded (body = ob.body
// без template lookup). Direct entries работают всегда.
func MergeOutboundUpdates(ob configtypes.Direction, td *template.TemplateData, target template.TargetSpec) configtypes.Direction {
	return mergeOutboundUpdates(ob, td, target)
}

// mergeOutboundUpdates — вычисляет merged outbound body: resolve base
// (template/preset/inline) + apply Updates в order. Возвращает копию.
func mergeOutboundUpdates(ob configtypes.Direction, td *template.TemplateData, target template.TargetSpec) configtypes.Direction {
	tmplOutbounds := td.GlobalOutbounds()
	presetByID := make(map[string]*template.Preset)
	if td != nil {
		for i := range td.Presets {
			presetByID[td.Presets[i].ID] = &td.Presets[i]
		}
	}
	base, _ := resolveBaseBody(ob, tmplOutbounds, presetByID, target)
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
func MergeOutboundUpdatesInPlace(parserCfg *configtypes.ParserConfig, td *template.TemplateData, target template.TargetSpec) {
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
	kept := parserCfg.ParserConfig.Outbounds[:0]
	for i := range parserCfg.ParserConfig.Outbounds {
		ob := parserCfg.ParserConfig.Outbounds[i]
		if ob.Ref == "" && len(ob.Updates) == 0 {
			kept = append(kept, ob) // direct без patches — nothing to do
			continue
		}
		base, resolved := resolveBaseBody(ob, tmplOutbounds, presetByID, target)
		merged := applyUpdatesToBase(base, ob.Updates)
		// Осиротевшая ссылка (тег исчез из template/preset, например после
		// переименования записи) даёт degraded thin body без type. Эмитить его
		// нельзя: sing-box отвергает ВЕСЬ конфиг («unknown outbound type: ""»),
		// один битый огрызок кладёт остальные 40+ рабочих outbound'ов. Дропаем
		// с warning'ом. td==nil (legacy SPEC 057) сюда не попадает: там body
		// inline и type непустой.
		if !resolved && strings.TrimSpace(merged.Type) == "" {
			debuglog.WarnLog("outbound resolve: %q references missing template/preset body (ref=%q) — dropped from build", ob.Tag, ob.Ref)
			continue
		}
		kept = append(kept, merged)
	}
	parserCfg.ParserConfig.Outbounds = kept
}

// applyOutboundUpdatePatch — применяет один patch (map) к target outbound.
// Тонкая обёртка вокруг applyOutboundUpdate(target, patch Direction)
// для удобства работы с map-форматом из OutboundUpdate.Patch.
//
// Конвертирует map → Direction (через JSON marshal/unmarshal на patch
// keys) → вызывает existing applyOutboundUpdate → возвращает результат.
//
// userPatch=true (ref=#USER#): addOutbounds в патче — это полный список из
// формы (OutboundFieldDiff: "replace целиком"), а не добавка; union из
// applyOutboundUpdate тут доливал бы снятые юзером теги обратно из базы, и
// чекбокс (например proxy-out) было бы невозможно выключить. Поэтому при
// наличии ключа "addOutbounds" список заменяется как есть, включая пустой [].
// Preset-патчи (userPatch=false) сохраняют union — они добавляют теги, не зная
// базового списка.
//
// Если patch не парсится — возвращает target без изменений (safe noop).
func applyOutboundUpdatePatch(target configtypes.Direction, patch map[string]interface{}, userPatch bool) configtypes.Direction {
	if len(patch) == 0 {
		return target
	}
	patchJSON, err := json.Marshal(patch)
	if err != nil {
		return target
	}
	var patchOC configtypes.Direction
	if err := json.Unmarshal(patchJSON, &patchOC); err != nil {
		return target
	}
	out := applyOutboundUpdate(target, patchOC)
	if userPatch {
		if _, ok := patch["addOutbounds"]; ok {
			out.AddOutbounds = append([]string(nil), patchOC.AddOutbounds...)
		}
	}
	return out
}
