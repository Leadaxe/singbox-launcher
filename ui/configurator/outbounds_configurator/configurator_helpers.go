// configurator_helpers.go holds standalone helpers and the row model for the
// outbounds configurator: row collection / classification, tag gathering,
// reorder swaps, preset/template lookups. The main NewConfiguratorContent
// builder stays in configurator.go.
//
// SPEC 117: пакет работает НАПРЯМУЮ на canonical-модели
// (model.GlobalOutbounds / model.Sources[i].Outbounds) — legacy-проекция
// ParserConfig здесь больше не читается и не пишется.
package outbounds_configurator

import (
	"encoding/json"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	wizardmodels "singbox-launcher/ui/configurator/models"
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
	Outbound     *config.Direction
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

// collectRows builds the flat list of Направления (global outbounds).
// Order matters: lower items can reference upper items (e.g. in addOutbounds), not the other way around.
//
// SPEC 057-R-N: preset entries в state идентифицируются по `ref` field на
// Direction. Если ref != "" → row marked IsPreset (read-only).
// presetTagToLabel параметр legacy (для обратной compat с тестами), но
// state's ref имеет приоритет.
//
// requiredTags — set tag'ов с `required: true` из template (live lookup).
// state.json не обязан персистить этот flag — template источник истины.
//
// SPEC 117: на входе — canonical model.GlobalOutbounds. Указатели в rows
// смотрят прямо в canonical-слайс: in-place правка строки (Edit без смены
// scope, toggle) мутирует модель без промежуточных копий.
func collectRows(outbounds []config.Direction, presetTagToLabel map[string]string, requiredTags map[string]bool) []outboundRow {
	// SPEC 108: группы подписок в списке Направлений не показываются.
	//
	// Раньше сюда попадали `proxies[].outbounds[]` — служебные `AL:select`
	// и `AL:auto`. Их создаёт и правит сама подписка; настраиваются они на
	// её вкладке «Группа», а в списке Направлений выглядели чужеродно и
	// требовали отдельного заголовка секции, чтобы список не читался одной
	// кучей. Свёрнутые подписки вообще не хранят групп в состоянии — они
	// разворачиваются на сборке (config.PrepareSourceFolds).
	var rows []outboundRow
	for i := range outbounds {
		ob := &outbounds[i]
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
func templateGlobalOutbounds(model *wizardmodels.WizardModel) []config.Direction {
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
// Парсит template raw JSON через map (не struct), так как Direction
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

// collectRowsForUI — wrapper над collectRows: добавляет live-lookup'ы
// required-тегов шаблона и подписей пресетов.
//
// SPEC 057-R-N: preset entries живут в canonical outbounds[] напрямую с
// `ref` field. collectRows уже их рендерит правильно (IsPreset=true для
// ref != ""). Synthetic preset rows + OutboundDisplayOrder больше не
// нужны — natural slice order = display order = emit order.
func collectRowsForUI(model *wizardmodels.WizardModel) []outboundRow {
	if model == nil {
		return nil
	}
	requiredTags := templateRequiredTags(model)
	presetLabels := presetLabelsByID(model)
	return collectRows(model.GlobalOutbounds, presetLabels, requiredTags)
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
//
// SPEC 117: читает canonical model.Sources / model.GlobalOutbounds напрямую.
func collectAllTags(model *wizardmodels.WizardModel) []string {
	if model == nil {
		return nil
	}
	var tags []string
	for si := range model.Sources {
		if !model.Sources[si].Enabled {
			continue
		}
		for i := range model.Sources[si].Outbounds {
			tags = append(tags, model.Sources[si].Outbounds[i].Tag)
		}
	}
	for i := range model.GlobalOutbounds {
		tags = append(tags, model.GlobalOutbounds[i].Tag)
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

// syncOutboundsLocal — local sync helper: вызывает SyncOutboundsWithTemplate
// на canonical model.GlobalOutbounds. Используется refreshList после любой
// UI-мутации, чтобы новые entries получали expected preset patches, stale
// entries — дропались. Idempotent — safe для repeated calls.
//
// SPEC 058-R-N: фикс для сценария «удалил template entry → Restore missing →
// новый thin entry не имел preset updates пока юзер не toggle'нул preset».
//
// SPEC 117: запись ровно одна — canonical; второй проход по legacy-виду
// model.ParserConfig снесён вместе с самим двоевластием представлений.
func syncOutboundsLocal(model *wizardmodels.WizardModel) {
	if model == nil || model.TemplateData == nil {
		return
	}
	rulesV6 := wizardmodels.EmitStateRulesInAxisOrder(
		model.RuleOrder, model.PresetRefs, model.CustomRules,
	)
	build.SyncOutboundsWithTemplate(rulesV6, &model.GlobalOutbounds, model.TemplateData.Presets, build.TemplateOutboundTags(model.TemplateData), model.Target)
}
