// Package build — File migrate_outbounds_spec058.go.
//
// One-shot migration legacy state (SPEC 057 shape) → SPEC 058 referenced shape.
//
// Семантика:
//
//	для каждого ob in outbounds with ref="":
//	  if tag совпадает с template.parser_config.outbounds[tag]:
//	    merged_base = template_body + apply каждого активного preset mode=update patch
//	                  для этого tag (preset патчи которые УЖЕ были применены в body
//	                  legacy state'а через ApplyPresetOutboundsToParserConfig).
//	    diff = OutboundFieldDiff(ob, merged_base)
//	    ob.ref = "#TEMPLATE#"
//	    if diff non-empty: USER patch в updates[]
//	    strip body
//	  else if tag совпадает с preset.outbounds[mode=add]:
//	    merged_base = preset_add_body (mode=add патчи не аугментируются другими)
//	    diff = OutboundFieldDiff(ob, merged_base)
//	    ob.ref = "<preset_id>"
//	    if diff non-empty: USER patch
//	    strip body
//	  else:
//	    keep as direct (ref="", body inline)
//
// **Diff против merged_base, не против чистого template:** в SPEC 057 state preset
// patches были materialized в body через ApplyPresetOutboundsToParserConfig. Diff
// против чистого template ошибочно атрибутировал бы эти patches как USER edits.
// Diff против (template + active preset patches) даёт корректную семантику: USER
// patch = только реально юзерские правки.
//
// Идемпотентна: повторный запуск на уже мигрированном state — no-op (все
// matching entries уже referenced, в loop попадают только реально direct).
//
// Lossless: каждое изменение либо matchится с merged_base 1:1 (нет diff —
// strip body, body придёт из template + preset patches на render), либо
// diff → USER patch.
package build

import (
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// MigrateOutboundsToReferencedShape — конвертирует direct entries в referenced
// shape там где tag совпадает с template/preset. Mutates outbounds in-place.
//
// Returns true если был хоть один change (caller может использовать как сигнал
// для backup `.pre-058.bak` на следующем save).
//
// td nil → no-op (без template нечем сравнивать). rules используются для
// computing merged_base — какие preset patches должны быть применены к
// template_body чтобы корректно diff'нуть юзерский USER patch.
func MigrateOutboundsToReferencedShape(
	outbounds *[]configtypes.OutboundConfig,
	rules []state.Rule,
	td *template.TemplateData,
) bool {
	if outbounds == nil || td == nil {
		return false
	}
	tmplOutbounds := td.GlobalOutbounds()
	tmplByTag := make(map[string]configtypes.OutboundConfig, len(tmplOutbounds))
	for _, t := range tmplOutbounds {
		tmplByTag[t.Tag] = t
	}

	// Preset lookup tables.
	presetByID := make(map[string]*template.Preset, len(td.Presets))
	for i := range td.Presets {
		presetByID[td.Presets[i].ID] = &td.Presets[i]
	}

	// Compute active preset patches per tag (mode=update).
	// presetUpdatesByTag[tag] = []OutboundUpdate в rule order.
	presetUpdatesByTag := make(map[string][]configtypes.OutboundUpdate)
	// Preset add bodies by tag (mode=add) — для preset_id classify.
	type presetAdd struct {
		ID   string
		Body configtypes.OutboundConfig
	}
	presetAddByTag := make(map[string]presetAdd)

	for _, rule := range rules {
		if rule.Kind != state.RuleKindPreset || !rule.Enabled || rule.Ref == "" {
			continue
		}
		p, ok := presetByID[rule.Ref]
		if !ok {
			continue
		}
		body, err := rule.DecodeBody()
		if err != nil {
			continue
		}
		pb, _ := body.(*state.PresetBody)
		if pb == nil {
			continue
		}
		entries, _ := ExpandPresetOutbounds(p, pb.Vars)
		for _, entry := range entries {
			switch entry.Mode {
			case "add":
				if _, dup := presetAddByTag[entry.Config.Tag]; !dup {
					presetAddByTag[entry.Config.Tag] = presetAdd{ID: p.ID, Body: entry.Config}
				}
			case "update":
				patch := outboundConfigToPatchMap(entry.Config)
				presetUpdatesByTag[entry.Config.Tag] = append(
					presetUpdatesByTag[entry.Config.Tag],
					configtypes.OutboundUpdate{Ref: p.ID, Patch: patch},
				)
			}
		}
	}

	changed := false
	for i := range *outbounds {
		ob := &(*outbounds)[i]
		if ob.Ref != "" {
			continue // already referenced
		}
		// Try template global match.
		if tmplBody, ok := tmplByTag[ob.Tag]; ok {
			// merged_base = template body + apply active preset patches (mode=update)
			// для этого tag. Эти patches УЖЕ были применены в body legacy state'а
			// через ApplyPresetOutboundsToParserConfig; чтобы diff показал только
			// реально юзерские правки, сравниваем против merged_base а не tmplBody.
			mergedBase := applyUpdatesToBase(tmplBody, presetUpdatesByTag[ob.Tag])
			diff := OutboundFieldDiff(*ob, mergedBase)
			ob.Ref = configtypes.RefTemplate
			ob.Updates = UpsertUserPatch(ob.Updates, diff)
			stripReferencedBody(ob)
			changed = true
			continue
		}
		// Try preset add match.
		if pa, ok := presetAddByTag[ob.Tag]; ok {
			// Для preset add'ов merged_base = просто add body (preset add'ы не
			// получают patches от других presetов в текущей модели).
			diff := OutboundFieldDiff(*ob, pa.Body)
			ob.Ref = pa.ID
			ob.Updates = UpsertUserPatch(ob.Updates, diff)
			stripReferencedBody(ob)
			changed = true
			continue
		}
		// No match — настоящий direct entry, leave as-is.
	}
	return changed
}
