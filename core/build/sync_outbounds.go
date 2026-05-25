// Package build — File sync_outbounds.go (SPEC 057/058-R-N).
//
// Lifecycle функция для preset/template binding в outbounds — единая точка
// добавления/удаления referenced entries в state.connections.outbounds[].
//
// Вызывается:
//   - На load после parseV6 (idempotent — повторный вызов с тем же state не
//     меняет ничего)
//   - На каждый preset toggle в Rules tab (после mutation state.Rules)
//   - Перед marshalDiskV6 (defensive — гарантия что state в правильном shape)
//
// Семантика (SPEC 058-R-N §"Sync semantics"):
//
//  1. Walk state.outbounds, для каждого:
//     - Drop referenced preset entries (ref=<preset_id>) с disabled/missing preset
//     - Drop referenced template entries (ref=#TEMPLATE#) если tag отсутствует в template
//     - Direct entries (ref="") не трогаем (от template-evolution независимы)
//     - Из updates[] drop preset patches от disabled preset; USER patch оставляем
//  2. Add missing referenced preset add entries
//  3. Add missing referenced template entries (seeding из template)
//  4. Add expected preset update patches (mode=update entries)
//  5. Re-order updates[]: preset patches в rule order, USER в конце
//  6. Adopt legacy: existing direct entry + tag совпадает с template/preset →
//     конвертируем в referenced (set ref, strip body)
package build

import (
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// SyncOutboundsWithActivePresets — синхронизирует preset binding в outbounds.
//
// Параметры:
//   - rules     — state.Rules (для discovery active preset-ref'ов)
//   - outbounds — pointer на state.Connections.Outbounds (мутируется in-place)
//   - presets   — template.Presets для resolve preset.Outbounds + Vars
//
// Idempotent. Безопасно вызывать многократно.
//
// SPEC 058: signature не меняется (template entries lifecycle определяется
// presence в template, через td.GlobalOutbounds() — но sync function его не
// получает напрямую; «add missing template entries» делается отдельно через
// «Restore missing» UI button, а не автоматически в sync, чтобы юзер сам
// контролировал какие template entries у него в state).
func SyncOutboundsWithActivePresets(
	rules []state.Rule,
	outbounds *[]configtypes.OutboundConfig,
	presets []template.Preset,
) {
	if outbounds == nil {
		return
	}

	presetByID := make(map[string]*template.Preset, len(presets))
	for i := range presets {
		presetByID[presets[i].ID] = &presets[i]
	}

	// 1. Compute active preset IDs + expected entries.
	//    activeRulesOrder — slice presetIDs в порядке state.Rules (для deterministic
	//    re-order updates[] patches).
	activePresetIDs := make(map[string]bool)
	activeRulesOrder := make([]string, 0)
	expectedAdds := make(map[string]configtypes.OutboundConfig) // tag → add entry
	expectedUpdates := make(map[string]map[string]configtypes.OutboundUpdate)
	// expectedUpdates[targetTag][presetID] = OutboundUpdate

	for _, rule := range rules {
		if rule.Kind != state.RuleKindPreset || !rule.Enabled || rule.Ref == "" {
			continue
		}
		preset, ok := presetByID[rule.Ref]
		if !ok {
			continue // dangling — preset.outbounds entries будут удалены ниже
		}
		body, err := rule.DecodeBody()
		if err != nil {
			continue
		}
		pb, _ := body.(*state.PresetBody)
		if pb == nil {
			continue
		}
		if !activePresetIDs[rule.Ref] {
			activePresetIDs[rule.Ref] = true
			activeRulesOrder = append(activeRulesOrder, rule.Ref)
		}

		entries, _ := ExpandPresetOutbounds(preset, pb.Vars)
		for _, entry := range entries {
			switch entry.Mode {
			case "add":
				cfg := entry.Config
				cfg.Ref = preset.ID
				// Не перезаписываем если уже есть expected entry с тем же tag
				// (first-wins по аналогии с ApplyPresetOutboundsToParserConfig).
				if _, dup := expectedAdds[cfg.Tag]; !dup {
					expectedAdds[cfg.Tag] = cfg
				}
			case "update":
				if _, ok := expectedUpdates[entry.Config.Tag]; !ok {
					expectedUpdates[entry.Config.Tag] = make(map[string]configtypes.OutboundUpdate)
				}
				expectedUpdates[entry.Config.Tag][preset.ID] = configtypes.OutboundUpdate{
					Ref:   preset.ID,
					Patch: outboundConfigToPatchMap(entry.Config),
				}
			}
		}
	}

	// 2. Pass through existing outbounds. Filter out preset/template entries which
	//    are no longer expected; filter out stale updates; adopt legacy direct
	//    entries как referenced если tag совпадает с expected preset add.
	out := make([]configtypes.OutboundConfig, 0, len(*outbounds))
	existingPresetTags := make(map[string]bool) // tag'и уже-present preset add entries
	// expectedAddRefByTag — для adopt-on-first-sync: какой preset.id owns этот tag.
	expectedAddRefByTag := make(map[string]string, len(expectedAdds))
	for tag, cfg := range expectedAdds {
		expectedAddRefByTag[tag] = cfg.Ref
	}
	for _, ob := range *outbounds {
		// Drop preset add entries для disabled/missing preset.
		// (Template entries ref=#TEMPLATE# не дропаются здесь — они не привязаны
		// к presets; missing-template-tag handled через resolver fallback.)
		if ob.Ref != "" && ob.Ref != configtypes.RefTemplate {
			if _, expected := expectedAdds[ob.Tag]; !expected {
				continue // drop preset entry — owner preset disabled
			}
			existingPresetTags[ob.Tag] = true
			// SPEC 058: strip body fields (referenced entries thin — body live из preset)
			stripReferencedBody(&ob)
		} else if ob.Ref == configtypes.RefTemplate {
			// Template entry — preserve, strip body (thin shape).
			stripReferencedBody(&ob)
		} else if presetID, expected := expectedAddRefByTag[ob.Tag]; expected {
			// Adopt: direct entry с tag совпадающим с expected preset add →
			// конвертируем в referenced preset. Strip body (live из preset).
			ob.Ref = presetID
			stripReferencedBody(&ob)
			existingPresetTags[ob.Tag] = true
		}

		// Update stack: filter stale preset patches; keep USER patch.
		if len(ob.Updates) > 0 {
			expectedForTag := expectedUpdates[ob.Tag]
			keptUpdates := make([]configtypes.OutboundUpdate, 0, len(ob.Updates))
			for _, u := range ob.Updates {
				if u.Ref == configtypes.RefUser {
					keptUpdates = append(keptUpdates, u)
					continue
				}
				if _, ok := expectedForTag[u.Ref]; ok {
					keptUpdates = append(keptUpdates, u)
				}
				// else: preset disabled/missing → drop
			}
			ob.Updates = keptUpdates
			if len(ob.Updates) == 0 {
				ob.Updates = nil
			}
		}

		// Add missing expected updates for this tag.
		if expectedForTag, ok := expectedUpdates[ob.Tag]; ok {
			present := make(map[string]bool, len(ob.Updates))
			for _, u := range ob.Updates {
				present[u.Ref] = true
			}
			for ref, u := range expectedForTag {
				if !present[ref] {
					ob.Updates = append(ob.Updates, u)
				}
			}
		}

		// Re-order updates[]: preset patches в rule order, USER в конце.
		ob.Updates = reorderUpdates(ob.Updates, activeRulesOrder)

		out = append(out, ob)
	}

	// 3. Append missing preset add entries (after existing ones in stable order).
	for _, rule := range rules {
		if rule.Kind != state.RuleKindPreset || !rule.Enabled {
			continue
		}
		preset, ok := presetByID[rule.Ref]
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
		entries, _ := ExpandPresetOutbounds(preset, pb.Vars)
		for _, entry := range entries {
			if entry.Mode != "add" {
				continue
			}
			tag := entry.Config.Tag
			if existingPresetTags[tag] {
				continue // already present
			}
			// Check first-wins with non-preset existing globals.
			if findOutboundByTag(out, tag) >= 0 {
				continue // collision с user/template global → skip (first-wins)
			}
			// SPEC 058: thin entry — только tag + ref (body live из preset).
			cfg := configtypes.OutboundConfig{
				Tag: tag,
				Ref: preset.ID,
			}
			out = append(out, cfg)
			existingPresetTags[tag] = true
		}
	}

	*outbounds = out
}

// stripReferencedBody — для referenced entry (ref != "") очищает body-поля,
// оставляя только tag + ref + updates. Body live из template/preset на render.
// SPEC 058 shape.
func stripReferencedBody(ob *configtypes.OutboundConfig) {
	if ob == nil || ob.Ref == "" {
		return
	}
	ob.Type = ""
	ob.Options = nil
	ob.Filters = nil
	ob.AddOutbounds = nil
	ob.PreferredDefault = nil
	ob.Comment = ""
}

// reorderUpdates — переставляет updates: preset patches в order
// activeRulesOrder (rule order), USER patch (ref=#USER#) — в конце.
// Stale preset patches (ref не в activeRulesOrder) идут после ordered preset
// patches, перед USER (на следующем sync они dropped, но если sync вызвал
// reorder без drop — preserve них order).
func reorderUpdates(updates []configtypes.OutboundUpdate, activeRulesOrder []string) []configtypes.OutboundUpdate {
	if len(updates) == 0 {
		return updates
	}
	byRef := make(map[string]configtypes.OutboundUpdate, len(updates))
	var userPatch *configtypes.OutboundUpdate
	var staleOrder []string
	for i := range updates {
		u := updates[i]
		if u.Ref == configtypes.RefUser {
			cp := u
			userPatch = &cp
			continue
		}
		if _, dup := byRef[u.Ref]; !dup {
			byRef[u.Ref] = u
			if !containsString(activeRulesOrder, u.Ref) {
				staleOrder = append(staleOrder, u.Ref)
			}
		}
	}
	out := make([]configtypes.OutboundUpdate, 0, len(updates))
	for _, ref := range activeRulesOrder {
		if u, ok := byRef[ref]; ok {
			out = append(out, u)
		}
	}
	for _, ref := range staleOrder {
		out = append(out, byRef[ref])
	}
	if userPatch != nil {
		out = append(out, *userPatch)
	}
	return out
}

func containsString(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}

// findOutboundByTag — index в slice по tag, или -1.
func findOutboundByTag(outbounds []configtypes.OutboundConfig, tag string) int {
	for i, ob := range outbounds {
		if ob.Tag == tag {
			return i
		}
	}
	return -1
}

// outboundConfigToPatchMap — конвертирует preset.outbounds[mode=update] entry
// (typed configtypes.OutboundConfig) в map для хранения в OutboundUpdate.Patch.
//
// Включает только не-zero patch-fields (filters/options/addOutbounds/
// preferredDefault/wizard/comment). Tag/Type для update не релевантны
// (immutable, см. applyOutboundUpdate semantics).
func outboundConfigToPatchMap(cfg configtypes.OutboundConfig) map[string]interface{} {
	patch := make(map[string]interface{})
	if cfg.Filters != nil {
		patch["filters"] = cfg.Filters
	}
	if len(cfg.Options) > 0 {
		patch["options"] = cfg.Options
	}
	if len(cfg.AddOutbounds) > 0 {
		// Convert []string → []interface{} для JSON map symmetry.
		arr := make([]interface{}, len(cfg.AddOutbounds))
		for i, s := range cfg.AddOutbounds {
			arr[i] = s
		}
		patch["addOutbounds"] = arr
	}
	if cfg.PreferredDefault != nil {
		patch["preferredDefault"] = cfg.PreferredDefault
	}
	if cfg.Comment != "" {
		patch["comment"] = cfg.Comment
	}
	return patch
}
