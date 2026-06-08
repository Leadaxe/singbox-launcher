// configurator_helpers.go holds standalone helpers and the row model for the
// outbounds configurator: row collection / classification, tag gathering,
// reorder swaps, preset/template lookups, and ParserConfig access. The main
// NewConfiguratorContent builder stays in configurator.go.
package outbounds_configurator

import (
	"encoding/json"
	"strconv"
	"strings"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardutils "singbox-launcher/ui/configurator/utils"
)

// outboundRow identifies one outbound in the list (global or per-source).
//
// SPEC 058-R-N: entry-level shape вычисляется из ob.Ref:
//   - ref="" → direct (full Edit/Del, body inline)
//   - ref="#TEMPLATE#" → referenced template (Edit+Reset+Del, body live)
//   - ref="<preset_id>" → referenced preset (View, Del нет; lifecycle через preset toggle)
type outboundRow struct {
	IsGlobal     bool
	SourceIndex  int
	IndexInSlice int
	Outbound     *config.OutboundConfig
	SourceLabel  string

	// IsPreset — true для referenced preset entries (ref != "" && ref != #TEMPLATE#).
	// Read-only с возможным USER patch'ем сверху; Del запрещён (lifecycle через preset toggle).
	IsPreset    bool
	PresetID    string
	PresetLabel string

	// IsTemplate — true для referenced template entries (ref="#TEMPLATE#").
	// Edit с diff'ом в USER patch; Del разрешён (можно вернуть через «Restore missing»).
	IsTemplate bool

	// HasUserPatch — true если в Updates[] есть USER patch (ref=#USER#).
	// Visual: badge ✏ к SourceLabel. Reset button становится enabled (clear USER patch).
	HasUserPatch bool

	// IsRequired — true для template outbound'ов с `required: true`. UI блокирует Del
	// полностью (button hidden). Edit + Reset работают. Релевантно только для IsTemplate.
	IsRequired bool
}

// collectRows builds the flat list: local outbounds first (per source), then global.
// Order matters: lower items can reference upper items (e.g. in addOutbounds), not the other way around.
//
// Disabled sources skipped — UI должен совпадать с build pipeline, который
// тоже пропускает disabled подписки (GenerateOutboundsFromParserConfig). Без
// этого юзер видит per-source outbound'ы (BL:auto, BL:select) от выключенных
// подписок, хотя в финальном config.json их нет.
//
// SPEC 057-R-N: preset entries в state идентифицируются по `ref` field на
// OutboundConfig. Если ref != "" → row marked IsPreset (read-only).
// presetTagToLabel параметр legacy (для обратной compat с тестами), но
// state's ref имеет приоритет.
//
// requiredTags — set tag'ов с `required: true` из template (live lookup).
// state.json не обязан персистить этот flag — template источник истины.
func collectRows(pc *config.ParserConfig, presetTagToLabel map[string]string, requiredTags map[string]bool) []outboundRow {
	var rows []outboundRow
	for si, proxy := range pc.ParserConfig.Proxies {
		if proxy.Disabled {
			continue
		}
		label := proxy.Source
		if label == "" {
			label = locale.T("wizard.outbound.label_source") + strconv.Itoa(si+1)
		}
		label = wizardutils.TruncateStringEllipsis(label, wizardutils.MaxLabelRunes, "...")
		for i := range proxy.Outbounds {
			rows = append(rows, outboundRow{
				IsGlobal:     false,
				SourceIndex:  si,
				IndexInSlice: i,
				Outbound:     &pc.ParserConfig.Proxies[si].Outbounds[i],
				SourceLabel:  label,
			})
		}
	}
	for i := range pc.ParserConfig.Outbounds {
		ob := &pc.ParserConfig.Outbounds[i]
		// HasUserPatch — есть ли в Updates[] entry с RefUser.
		hasUserPatch := false
		for _, u := range ob.Updates {
			if u.Ref == config.RefUser {
				hasUserPatch = true
				break
			}
		}
		row := outboundRow{
			IsGlobal:     true,
			IndexInSlice: i,
			Outbound:     ob,
			HasUserPatch: hasUserPatch,
		}
		// SPEC 058-R-N: classify по ref.
		//
		// SourceLabel — атрибуция строки для пользователя. Для per-source
		// outbound'ов = название подписки; для global emit'им только то,
		// что несёт смысл: 🔒 (required template / preset-locked), имя
		// пресета, ✏ (USER patch). Плоское «Global» убрано как шум:
		// глобальные строки и так визуально отличаются от per-source
		// отсутствием source-метки, дублировать слово на каждой строке
		// лишнее.
		switch {
		case ob.Ref == "":
			// Direct — full ownership. Без суффикса.
		case ob.Ref == config.RefTemplate:
			// Referenced template. Помечаем только если required (🔒).
			// Non-required template визуально == direct: разница для
			// юзера проявляется через Edit-диалог (live body из шаблона)
			// и кнопку Restore missing.
			row.IsTemplate = true
			row.IsRequired = requiredTags != nil && requiredTags[ob.Tag]
			if row.IsRequired {
				row.SourceLabel = "🔒"
			}
		default:
			// Referenced preset — имя пресета осмысленно (preset_id ↔ origin).
			row.IsPreset = true
			row.PresetID = ob.Ref
			if presetTagToLabel != nil {
				if lbl, ok := presetTagToLabel[ob.Ref]; ok {
					row.PresetLabel = lbl
				}
			}
			if row.PresetLabel == "" {
				row.PresetLabel = ob.Ref // fallback (dangling)
			}
			row.SourceLabel = "🔒 " + row.PresetLabel
		}
		// USER patch badge (✏) — для referenced entries с пользовательской правкой.
		if hasUserPatch && (row.IsTemplate || row.IsPreset) {
			if row.SourceLabel != "" {
				row.SourceLabel += " "
			}
			row.SourceLabel += "✏"
		}
		rows = append(rows, row)
	}
	return rows
}

// templateGlobalOutbounds — все global outbound'ы из template.parser_config,
// в порядке объявления (без сортировки). Используется кнопкой Restore missing
// для возврата случайно удалённых template entries.
//
// Возвращает nil/пустой slice если template не загружен или parser_config пуст.
func templateGlobalOutbounds(model *wizardmodels.WizardModel) []config.OutboundConfig {
	if model == nil || model.TemplateData == nil || model.TemplateData.ParserConfig == "" {
		return nil
	}
	var parsed config.ParserConfig
	if err := json.Unmarshal([]byte(model.TemplateData.ParserConfig), &parsed); err != nil {
		return nil
	}
	return parsed.ParserConfig.Outbounds
}

// SPEC 058-R-N: helpers templateOutboundByTag/presetOutboundByRefTag удалены.
// Новый Reset button не replaceит body — он чистит USER patch из Updates[]
// (build.UpsertUserPatch с nil). Body для referenced entries резолвится из
// template/preset через MergeOutboundUpdatesInPlace на render/build.

// templateRequiredTags — set tag'ов с `required: true` в template.parser_config.
// outbounds[]. Live lookup на каждый render — template **единственный** источник
// истины (state.json НЕ персистит required, чтобы изменение template'а сразу
// отражалось в UI).
//
// Парсит template raw JSON через map (не struct), так как OutboundConfig
// намеренно не имеет Required field — иначе оно бы попало в state.json.
// Возвращает nil если template не загружен.
func templateRequiredTags(model *wizardmodels.WizardModel) map[string]bool {
	if model == nil || model.TemplateData == nil || model.TemplateData.ParserConfig == "" {
		return nil
	}
	// TemplateData.ParserConfig — wrapped как {"ParserConfig": {...}} (capital P),
	// см. core/template/loader.go:207. JSON-tag здесь капитальный.
	var raw struct {
		ParserConfig struct {
			Outbounds []map[string]interface{} `json:"outbounds"`
		} `json:"ParserConfig"`
	}
	if err := json.Unmarshal([]byte(model.TemplateData.ParserConfig), &raw); err != nil {
		return nil
	}
	out := make(map[string]bool, len(raw.ParserConfig.Outbounds))
	for _, ob := range raw.ParserConfig.Outbounds {
		req, _ := ob["required"].(bool)
		tag, _ := ob["tag"].(string)
		if req && tag != "" {
			out[tag] = true
		}
	}
	return out
}

// collectRowsForUI — convenience wrapper: collectRows (без mutation) + append
// синтетических preset rows. Возвращает unified список для UI render.
//
// **Важно:** preset rows синтетические, их IndexInSlice = -1, Outbound указывает
// на local copy (не в model.ParserConfig). Edit/Del handlers ОБЯЗАНЫ проверять
// row.IsPreset и bailout — иначе будут операции на копии которая не сохранится.
//
// Dedup: existingTags (global + per-source) → preset rows для conflicting tag'ов
// не показываются (паритет с build first-wins).
// collectRowsForUI — wrapper над collectRows + dispatch preset rows.
//
// SPEC 057-R-N: preset entries теперь живут в state.connections.outbounds[]
// напрямую с `ref` field. collectRows уже их рендерит правильно (IsPreset=true
// для ref != ""). Synthetic preset rows + OutboundDisplayOrder больше не
// нужны — natural slice order = display order = emit order.
func collectRowsForUI(model *wizardmodels.WizardModel) []outboundRow {
	pc := getParserConfig(model)
	if pc == nil {
		return nil
	}
	requiredTags := templateRequiredTags(model)
	presetLabels := presetLabelsByID(model)
	return collectRows(pc, presetLabels, requiredTags)
}

// presetLabelsByID — map[preset_id]→display_label для UI label preset rows.
// Lookup из template.Presets.
func presetLabelsByID(model *wizardmodels.WizardModel) map[string]string {
	if model == nil || model.TemplateData == nil {
		return nil
	}
	out := make(map[string]string, len(model.TemplateData.Presets))
	for i := range model.TemplateData.Presets {
		p := &model.TemplateData.Presets[i]
		label := p.Label
		if label == "" {
			label = p.ID
		}
		out[p.ID] = label
	}
	return out
}

// collectAllTags returns all outbound tags in display order (local first, then global).
// Skips disabled sources (their tags не доступны для addOutbounds references).
func collectAllTags(pc *config.ParserConfig) []string {
	var tags []string
	for si := range pc.ParserConfig.Proxies {
		if pc.ParserConfig.Proxies[si].Disabled {
			continue
		}
		for i := range pc.ParserConfig.Proxies[si].Outbounds {
			tags = append(tags, pc.ParserConfig.Proxies[si].Outbounds[i].Tag)
		}
	}
	for i := range pc.ParserConfig.Outbounds {
		tags = append(tags, pc.ParserConfig.Outbounds[i].Tag)
	}
	return tags
}

// tagsAbove returns tags of rows that appear before rowIndex (only those can be used in addOutbounds).
func tagsAbove(rows []outboundRow, rowIndex int) []string {
	if rowIndex <= 0 {
		return nil
	}
	tags := make([]string, 0, rowIndex)
	for i := 0; i < rowIndex; i++ {
		tags = append(tags, rows[i].Outbound.Tag)
	}
	return tags
}

// syncOutboundsLocal — local sync helper: вызывает SyncOutboundsWithActivePresets
// на обоих outbound views модели (GlobalOutbounds canonical + ParserConfig
// derived view). Используется refreshList после любой UI-мутации, чтобы новые
// entries получали expected preset patches, stale entries — дропались.
// Idempotent — safe для repeated calls.
//
// SPEC 058-R-N: фикс для сценария «удалил template entry → Restore missing →
// новый thin entry не имел preset updates пока юзер не toggle'нул preset».
func syncOutboundsLocal(model *wizardmodels.WizardModel) {
	if model == nil || model.TemplateData == nil {
		return
	}
	rulesV6 := wizardmodels.SyncRulesByOrderToStateRulesV6(
		model.RuleOrder, model.PresetRefs, model.CustomRules,
	)
	build.SyncOutboundsWithActivePresets(rulesV6, &model.GlobalOutbounds, model.TemplateData.Presets)
	if model.ParserConfig != nil {
		build.SyncOutboundsWithActivePresets(rulesV6, &model.ParserConfig.ParserConfig.Outbounds, model.TemplateData.Presets)
	}
}

// getParserConfig returns the model's ParserConfig, ensuring it is set from ParserConfigJSON when nil.
func getParserConfig(model *wizardmodels.WizardModel) *config.ParserConfig {
	if model == nil {
		return nil
	}
	if model.ParserConfig != nil {
		return model.ParserConfig
	}
	raw := strings.TrimSpace(model.ParserConfigJSON)
	if raw == "" {
		return nil
	}
	var pc config.ParserConfig
	if err := json.Unmarshal([]byte(raw), &pc); err != nil {
		return nil
	}
	model.ParserConfig = &pc
	return model.ParserConfig
}

// sameScope returns true if both rows are in the same scope (same source or both global).
func sameScope(a, b outboundRow) bool {
	if a.IsGlobal && b.IsGlobal {
		return true
	}
	return !a.IsGlobal && !b.IsGlobal && a.SourceIndex == b.SourceIndex
}

// moveOutboundUp swaps the outbound with the previous one in the same scope.
func moveOutboundUp(parserConfig *config.ParserConfig, r outboundRow) {
	if r.IsGlobal {
		if r.IndexInSlice <= 0 {
			return
		}
		s := parserConfig.ParserConfig.Outbounds
		s[r.IndexInSlice], s[r.IndexInSlice-1] = s[r.IndexInSlice-1], s[r.IndexInSlice]
	} else {
		prox := &parserConfig.ParserConfig.Proxies[r.SourceIndex]
		if r.IndexInSlice <= 0 {
			return
		}
		prox.Outbounds[r.IndexInSlice], prox.Outbounds[r.IndexInSlice-1] = prox.Outbounds[r.IndexInSlice-1], prox.Outbounds[r.IndexInSlice]
	}
}

// moveOutboundDown swaps the outbound with the next one in the same scope.
func moveOutboundDown(parserConfig *config.ParserConfig, r outboundRow) {
	if r.IsGlobal {
		s := parserConfig.ParserConfig.Outbounds
		if r.IndexInSlice >= len(s)-1 {
			return
		}
		s[r.IndexInSlice], s[r.IndexInSlice+1] = s[r.IndexInSlice+1], s[r.IndexInSlice]
	} else {
		prox := &parserConfig.ParserConfig.Proxies[r.SourceIndex]
		if r.IndexInSlice >= len(prox.Outbounds)-1 {
			return
		}
		prox.Outbounds[r.IndexInSlice], prox.Outbounds[r.IndexInSlice+1] = prox.Outbounds[r.IndexInSlice+1], prox.Outbounds[r.IndexInSlice]
	}
}
