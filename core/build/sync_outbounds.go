// Package build — File sync_outbounds.go (SPEC 057-R-N).
//
// Lifecycle функция для preset binding в outbounds — единая точка
// добавления/удаления preset entries в state.connections.outbounds[].
//
// Вызывается:
//   - На load после parseV6 (idempotent — повторный вызов с тем же state не
//     меняет ничего)
//   - На каждый preset toggle в Rules tab (после mutation state.RulesV6)
//   - Перед marshalDiskV6 (defensive — гарантия что state в правильном shape)
//
// Семантика (SPEC 057-R-N §"SyncOutboundsWithActivePresets"):
//
//	для каждого state.RulesV6[kind=preset, enabled=true]:
//	  для каждого preset.outbounds[i] (после if/if_or filter + @var substitute):
//	    if mode=add:
//	      ensure state.outbounds содержит entry с tag + ref=preset.id
//	      (если нет — append; если есть — preserve, body уже там)
//	    if mode=update:
//	      target = find state.outbounds by tag
//	      if target found:
//	        ensure target.updates содержит {ref=preset.id, patch=substituted_body}
//	        (если нет — append; если есть — preserve)
//
//	для каждого state.outbounds[]:
//	  если ref != "" и preset не active/missing → удалить entry
//	  для каждой updates[] entry: если ref не active/missing → удалить
package build

import (
	"singbox-launcher/core/config/configtypes"
	v6 "singbox-launcher/core/state/v6"
	"singbox-launcher/core/template"
)

// SyncOutboundsWithActivePresets — синхронизирует preset binding в outbounds.
//
// Параметры:
//   - rules     — state.RulesV6 (для discovery active preset-ref'ов)
//   - outbounds — pointer на state.Connections.Outbounds (мутируется in-place)
//   - presets   — template.Presets для resolve preset.Outbounds + Vars
//
// Idempotent. Безопасно вызывать многократно.
func SyncOutboundsWithActivePresets(
	rules []v6.Rule,
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
	activePresetIDs := make(map[string]bool)
	expectedAdds := make(map[string]configtypes.OutboundConfig) // tag → add entry (full body)
	expectedUpdates := make(map[string]map[string]configtypes.OutboundUpdate)
	// expectedUpdates[targetTag][presetID] = OutboundUpdate

	for _, rule := range rules {
		if rule.Kind != v6.RuleKindPreset || !rule.Enabled || rule.Ref == "" {
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
		pb, _ := body.(*v6.PresetBody)
		if pb == nil {
			continue
		}
		activePresetIDs[rule.Ref] = true

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

	// 2. Pass through existing outbounds. Filter out preset entries which are
	//    no longer expected; filter out stale updates; adopt legacy globals
	//    как preset entries если tag совпадает с expected preset add.
	out := make([]configtypes.OutboundConfig, 0, len(*outbounds))
	existingPresetTags := make(map[string]bool) // tag'и уже-present preset add entries
	// expectedAddRefByTag — для adopt-on-first-sync: какой preset.id owns этот tag.
	expectedAddRefByTag := make(map[string]string, len(expectedAdds))
	for tag, cfg := range expectedAdds {
		expectedAddRefByTag[tag] = cfg.Ref
	}
	for _, ob := range *outbounds {
		// Drop preset add entries для disabled/missing preset.
		if ob.Ref != "" {
			if _, expected := expectedAdds[ob.Tag]; !expected {
				continue // drop
			}
			existingPresetTags[ob.Tag] = true
			// preserve as-is (body уже там, user мог reorder/edit body не редактируется)
		} else if presetID, expected := expectedAddRefByTag[ob.Tag]; expected {
			// Legacy-migration / adopt-on-first-sync: existing global без Ref
			// имеет tag совпадающий с expected preset add. Это либо state от
			// pre-SPEC-057 (старый promote-to-global подход), либо preset
			// activated на пустой state — в обоих случаях правильно adopt'ить
			// existing entry, не дублировать. Body preserved as-is; только
			// Ref проставляется → entry становится preset-bound.
			//
			// Edge: если юзер реально создал custom outbound с conflicting tag,
			// он "потеряет" свой entry (станет preset-locked). На практике юзер
			// не создаёт outbound'ы с тегами вроде "ru VPN 🇷🇺" вручную —
			// это всегда preset territory.
			ob.Ref = presetID
			existingPresetTags[ob.Tag] = true
		}

		// Update stack: filter stale.
		if len(ob.Updates) > 0 {
			expectedForTag := expectedUpdates[ob.Tag]
			keptUpdates := make([]configtypes.OutboundUpdate, 0, len(ob.Updates))
			for _, u := range ob.Updates {
				if _, ok := expectedForTag[u.Ref]; ok {
					keptUpdates = append(keptUpdates, u)
				}
				// else: preset disabled/missing → drop update
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

		out = append(out, ob)
	}

	// 3. Append missing preset add entries (after existing ones in stable order).
	for _, rule := range rules {
		if rule.Kind != v6.RuleKindPreset || !rule.Enabled {
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
		pb, _ := body.(*v6.PresetBody)
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
			cfg := entry.Config
			cfg.Ref = preset.ID
			out = append(out, cfg)
			existingPresetTags[tag] = true
		}
	}

	*outbounds = out
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
	if cfg.Wizard != nil {
		patch["wizard"] = cfg.Wizard
	}
	if cfg.Comment != "" {
		patch["comment"] = cfg.Comment
	}
	return patch
}
