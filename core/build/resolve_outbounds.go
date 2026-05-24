// Package build — File resolve_outbounds.go (SPEC 057-R-N).
//
// Resolver для outbounds — параллельно resolve_dns.go / resolve_route.go.
// Pure func: state.connections.outbounds[] → merged view с meta-info
// (Ref, Updates, IsPreset, HasPresetUpdates, Required).
//
// **Принципы (SPEC 057-R-N):**
//   - state.connections.outbounds[] — единственный источник истины.
//   - Preset add entries имеют `Ref="<preset_id>"`. На UI читается напрямую.
//   - Preset update patches хранятся в `Updates[]` стеке на target outbound.
//     Merged body = base + apply Updates в order.
//   - `Required` — template-only flag, читается live (как в SPEC 056-R-N).
//   - Build emit: runtime `ApplyPresetOutboundsToParserConfig` сохраняется как
//     defensive layer (re-derives patches from template; first-wins skips
//     preset add entries уже materialized в state). Sync function (Phase 2)
//     приводит UI/state в правильный shape; runtime re-application —
//     самосогласованный fallback если state не sync-нут (headless rebuild).
package build

import (
	"encoding/json"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/template"
)

// OutboundSource — discriminator происхождения outbound entry.
type OutboundSource string

const (
	// OutboundSourceGlobal — обычный global outbound (template или user-added).
	OutboundSourceGlobal OutboundSource = "global"

	// OutboundSourcePreset — entry создан через preset.outbounds[mode=add].
	// Имеет непустой Ref (= preset.id владельца). Read-only в UI.
	OutboundSourcePreset OutboundSource = "preset"
)

// ResolvedOutbound — одна entry финального outbounds list'а.
//
// Body = merged result (base + apply Updates в order). Для UI display и
// build emit.
type ResolvedOutbound struct {
	// Body — готовое sing-box outbound тело (after merge of updates).
	Body configtypes.OutboundConfig

	// Source — global (template/user) или preset (add).
	Source OutboundSource

	// IndexInSlice — индекс в state.connections.outbounds[]. Stable order.
	IndexInSlice int

	// Ref — preset.id владельца (только для Source=preset). Пусто иначе.
	Ref string

	// PresetLabel — UI label preset'а (только для Source=preset).
	PresetLabel string

	// Updates — снимок Updates стека (для UI hover/inspect). Не используется
	// для emit; только metadata.
	Updates []configtypes.OutboundUpdate

	// HasPresetUpdates — true если на этот outbound применены updates от
	// active preset'ов. UI показывает indicator + tooltip с list of refs.
	HasPresetUpdates bool

	// Required — true если template маркирует этот tag как `required: true`.
	// Live lookup из template (как в SPEC 056-R-N Phase E).
	Required bool
}

// ResolvedOutbounds — результат ResolveOutbounds.
type ResolvedOutbounds struct {
	Globals []ResolvedOutbound // включая preset entries (с Ref != "")
}

// ResolveOutbounds — единая точка резолва outbounds section (SPEC 057-R-N).
//
// Аргументы:
//   - outbounds — state.connections.outbounds[] (state-of-truth)
//   - presets — template.presets[] (для PresetLabel resolve)
//   - requiredTags — set tag'ов с `required: true` в template (live lookup)
//
// Возвращает структурированный view. Merged body вычисляется на лету:
// base + apply each Update в order.
func ResolveOutbounds(
	outbounds []configtypes.OutboundConfig,
	presets []template.Preset,
	requiredTags map[string]bool,
) ResolvedOutbounds {
	presetByID := make(map[string]*template.Preset, len(presets))
	for i := range presets {
		presetByID[presets[i].ID] = &presets[i]
	}

	out := ResolvedOutbounds{Globals: make([]ResolvedOutbound, 0, len(outbounds))}
	for i, ob := range outbounds {
		merged := mergeOutboundUpdates(ob)

		var source OutboundSource = OutboundSourceGlobal
		var presetLabel string
		if ob.Ref != "" {
			source = OutboundSourcePreset
			if p, ok := presetByID[ob.Ref]; ok {
				presetLabel = p.Label
				if presetLabel == "" {
					presetLabel = p.ID
				}
			} else {
				presetLabel = ob.Ref // dangling — preset больше нет в template
			}
		}

		out.Globals = append(out.Globals, ResolvedOutbound{
			Body:             merged,
			Source:           source,
			IndexInSlice:     i,
			Ref:              ob.Ref,
			PresetLabel:      presetLabel,
			Updates:          ob.Updates,
			HasPresetUpdates: len(ob.Updates) > 0,
			Required:         requiredTags != nil && requiredTags[ob.Tag],
		})
	}
	return out
}

// MergeOutboundUpdates — exported wrapper над mergeOutboundUpdates для
// per-entry merge (UI preview, dialog Edit и т.п.). Возвращает копию
// outbound с применёнными Updates[] патчами.
func MergeOutboundUpdates(ob configtypes.OutboundConfig) configtypes.OutboundConfig {
	return mergeOutboundUpdates(ob)
}

// mergeOutboundUpdates — вычисляет merged outbound body: base + apply Updates
// в order. Возвращает копию (immutable input).
//
// Если Updates пуст — возвращает копию ob без изменений.
func mergeOutboundUpdates(ob configtypes.OutboundConfig) configtypes.OutboundConfig {
	// Clean copy ob (без Updates/Ref — это metadata, в body не идёт).
	merged := ob
	merged.Updates = nil
	// Ref оставляем в merged для render-уровня metadata; не идёт в config.json
	// (native generator не знает поле ref, см. core/config/outbound_generator.go
	// который собирает JSON field-by-field).

	for _, u := range ob.Updates {
		merged = applyOutboundUpdatePatch(merged, u.Patch)
	}
	return merged
}

// MergeOutboundUpdatesInPlace — runtime helper: walks parserCfg.Outbounds[]
// и для каждой entry с непустым Updates[] стеком заменяет body на merged
// (base + apply patches в order). Mutates in-place.
//
// Используется build runtime path'ами (rebuild_raw_cache, UpdateConfigFromSubscriptions,
// UI parseAndPreview) ПОСЛЕ SyncOutboundsWithActivePresets — sync кладёт
// patches в state.Outbounds[*].Updates[], этот helper материализует их в
// финальное body для GenerateOutboundsFromParserConfig (который про Updates
// не знает).
//
// Безопасен для outbound'ов без updates — noop.
// Идемпотентен: повторный вызов после первого даёт тот же результат
// (mergeOutboundUpdates strips Updates из возвращаемой копии, так что вторая
// итерация видит пустой стек).
func MergeOutboundUpdatesInPlace(parserCfg *configtypes.ParserConfig) {
	if parserCfg == nil {
		return
	}
	for i := range parserCfg.ParserConfig.Outbounds {
		if len(parserCfg.ParserConfig.Outbounds[i].Updates) == 0 {
			continue
		}
		parserCfg.ParserConfig.Outbounds[i] = mergeOutboundUpdates(parserCfg.ParserConfig.Outbounds[i])
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
