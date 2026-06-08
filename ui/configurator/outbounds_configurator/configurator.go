// Package outbounds_configurator provides reusable UI for configuring outbounds in the wizard:
// list of all outbounds (global + per-source), Edit/Delete/Add, and helpers to apply changes back to ParserConfig.
package outbounds_configurator

import (
	"encoding/json"
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardutils "singbox-launcher/ui/configurator/utils"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// OutboundEditPresenter is used to register the Edit/Add window with the wizard overlay (single instance, focus redirect).
type OutboundEditPresenter interface {
	OpenOutboundEditWindow() fyne.Window
	SetOutboundEditWindow(fyne.Window)
	ClearOutboundEditWindow()
	UpdateChildOverlay()
	Model() *wizardmodels.WizardModel
}

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

// NewConfiguratorContent builds a reusable outbounds configurator content for embedding into tabs.
// ParserConfig is taken from the model (editPresenter.Model()) so the configurator always edits the current config.
// onApply is called after each mutation (Edit/Add/Delete/Up/Down) so the caller can serialize and sync.
// editPresenter is required (Model() is used to get ParserConfig); when set, the Edit/Add window is registered for overlay.
// The returned refresh function rebuilds the list from the current model (call after ParserConfig changes outside the list, e.g. Sources → Edit).
func NewConfiguratorContent(parent fyne.Window, editPresenter OutboundEditPresenter, onApply func()) (fyne.CanvasObject, func()) {
	listContent := container.NewVBox()

	var refreshList func()
	refreshList = func() {
		model := editPresenter.Model()
		if getParserConfig(model) == nil {
			listContent.Objects = nil
			listContent.Refresh()
			return
		}
		// SPEC 058-R-N: re-sync на каждый refresh — после любой мутации
		// (Restore missing / Add / Edit / Del / preset toggle) приводит
		// state в правильный shape: новые outbounds получают expected
		// preset update patches; orphan entries дропаются. Idempotent.
		syncOutboundsLocal(model)
		rows := collectRowsForUI(model)
		items := make([]fyne.CanvasObject, 0, len(rows))
		for rowIdx, r := range rows {
			r := r
			rowIdx := rowIdx
			var row *fynewidget.HoverRow
			rowGetter := func() *fynewidget.HoverRow { return row }

			rawLine := r.Outbound.Tag + " (" + r.Outbound.Type + ")"
			if r.SourceLabel != "" {
				rawLine += " — " + r.SourceLabel
			}
			rawLine = strings.ToValidUTF8(rawLine, "")
			displayLine := wizardutils.TruncateStringEllipsis(rawLine, wizardutils.MaxLabelRunes, "...")

			// Add transparent padding on the right so the list scrollbar has a visual strip.
			rightPadding := components.NewScrollGutter()

			nameLabel := ttwidget.NewLabel(displayLine)
			nameLabel.Wrapping = fyne.TextWrapOff
			nameLabel.Truncation = fyne.TextTruncateEllipsis
			nameLabel.SetToolTip(rawLine)

			var leftArrows, rightControls *fyne.Container

			// SPEC 057-R-N: preset rows — natural slice members с ref. Up/Down
			// для всех rows работает через direct swap pc.ParserConfig.Outbounds[]
			// (moveOutboundUp/Down) — preset binding (ref + updates) переезжает
			// вместе с body, потому что мы двигаем целиком элемент.
			canUp := rowIdx > 0 && sameScope(rows[rowIdx], rows[rowIdx-1])
			canDown := rowIdx < len(rows)-1 && sameScope(rows[rowIdx], rows[rowIdx+1])

			upBtn := fynewidget.NewHoverForwardButton("↑", func() {
				pc := getParserConfig(editPresenter.Model())
				if pc == nil {
					return
				}
				moveOutboundUp(pc, r)
				refreshList()
				if onApply != nil {
					onApply()
				}
			}, rowGetter)
			if !canUp {
				upBtn.Disable()
				fynewidget.SetToolTipSafe(upBtn, locale.T("wizard.outbound.reorder_up_off"))
			} else {
				fynewidget.SetToolTipSafe(upBtn, locale.T("wizard.outbound.reorder_up"))
			}

			downBtn := fynewidget.NewHoverForwardButton("↓", func() {
				pc := getParserConfig(editPresenter.Model())
				if pc == nil {
					return
				}
				moveOutboundDown(pc, r)
				refreshList()
				if onApply != nil {
					onApply()
				}
			}, rowGetter)
			if !canDown {
				downBtn.Disable()
				fynewidget.SetToolTipSafe(downBtn, locale.T("wizard.outbound.reorder_down_off"))
			} else {
				fynewidget.SetToolTipSafe(downBtn, locale.T("wizard.outbound.reorder_down"))
			}

			// Edit button — доступен для всех rows включая preset/required.
			// Для preset: scope locked, Ref/Updates preserved (sync-managed
			// metadata, не должны wipe'нуться юзерским body edit).
			editBtn := fynewidget.NewHoverForwardButtonWithIcon(locale.T("wizard.shared.button_edit"), theme.DocumentCreateIcon(), func() {
				rowsNow := collectRowsForUI(editPresenter.Model())
				if rowIdx >= len(rowsNow) {
					return
				}
				r2 := rowsNow[rowIdx]
				tagsForAdd := tagsAbove(rowsNow, rowIdx)
				wasGlobal := r2.IsGlobal
				wasSourceIndex := r2.SourceIndex
				parserConfig := getParserConfig(editPresenter.Model())
				ShowEditDialog(parent, editPresenter, r2.Outbound, r2.IsGlobal, r2.SourceIndex, tagsForAdd, func(updated *config.OutboundConfig, scopeKind string, sourceIndex int) {
					newGlobal := scopeKind == "global" || sourceIndex < 0
					scopeChanged := wasGlobal != newGlobal || (!newGlobal && wasSourceIndex != sourceIndex)
					// Preset entries: scope locked (preset должен оставаться
					// global, иначе Sync создаст дубль при следующем вызове —
					// per-source entry останется, plus новый global появится).
					if r2.IsPreset {
						scopeChanged = false
					}
					if scopeChanged {
						if wasGlobal {
							parserConfig.ParserConfig.Outbounds = append(parserConfig.ParserConfig.Outbounds[:r2.IndexInSlice], parserConfig.ParserConfig.Outbounds[r2.IndexInSlice+1:]...)
						} else {
							prox := &parserConfig.ParserConfig.Proxies[wasSourceIndex]
							prox.Outbounds = append(prox.Outbounds[:r2.IndexInSlice], prox.Outbounds[r2.IndexInSlice+1:]...)
						}
						if newGlobal {
							parserConfig.ParserConfig.Outbounds = append(parserConfig.ParserConfig.Outbounds, *updated)
						} else {
							for sourceIndex >= len(parserConfig.ParserConfig.Proxies) {
								parserConfig.ParserConfig.Proxies = append(parserConfig.ParserConfig.Proxies, config.ProxySource{})
							}
							parserConfig.ParserConfig.Proxies[sourceIndex].Outbounds = append(parserConfig.ParserConfig.Proxies[sourceIndex].Outbounds, *updated)
						}
					} else {
						// In-place body update. SPEC 058-R-N: для referenced
						// entries Edit dialog (Phase 4 applyEditedConfig) уже
						// вычислил USER patch и put его в updated.Updates +
						// strip'нул body. Для direct entries — updated имеет
						// full body inline + сохранённые Updates. В обоих
						// случаях просто перезаписываем r2.Outbound = *updated.
						*r2.Outbound = *updated
					}
					refreshList()
					if onApply != nil {
						onApply()
					}
				})
			}, rowGetter)

			delBtn := fynewidget.NewHoverForwardButtonWithIcon(locale.T("wizard.shared.button_del"), theme.DeleteIcon(), func() {
				rowsNow := collectRowsForUI(editPresenter.Model())
				if rowIdx >= len(rowsNow) || rowsNow[rowIdx].IsPreset || rowsNow[rowIdx].IsRequired {
					return
				}
				r2 := rowsNow[rowIdx]
				pc := getParserConfig(editPresenter.Model())
				if r2.IsGlobal {
					pc.ParserConfig.Outbounds = append(pc.ParserConfig.Outbounds[:r2.IndexInSlice], pc.ParserConfig.Outbounds[r2.IndexInSlice+1:]...)
				} else {
					prox := &pc.ParserConfig.Proxies[r2.SourceIndex]
					prox.Outbounds = append(prox.Outbounds[:r2.IndexInSlice], prox.Outbounds[r2.IndexInSlice+1:]...)
				}
				refreshList()
				if onApply != nil {
					onApply()
				}
			}, rowGetter)

			// Reset button — clear USER patch для referenced entries (SPEC 058-R-N).
			// После Reset body возвращается к live template/preset defaults
			// (без USER override). Создаём всегда для referenced (чтобы row layout
			// не прыгал), но disable если HasUserPatch=false — нечего ресетить.
			// Для direct entries вообще не создаём (нет base для reset).
			var resetBtn *fynewidget.HoverForwardButton
			if r.IsTemplate || r.IsPreset {
				resetBtn = fynewidget.NewHoverForwardButtonWithIcon("Reset", theme.ViewRefreshIcon(), func() {
					rowsNow := collectRowsForUI(editPresenter.Model())
					if rowIdx >= len(rowsNow) {
						return
					}
					r2 := rowsNow[rowIdx]
					if !(r2.IsTemplate || r2.IsPreset) || !r2.IsGlobal || r2.IndexInSlice < 0 {
						return
					}
					pc := getParserConfig(editPresenter.Model())
					if pc == nil || r2.IndexInSlice >= len(pc.ParserConfig.Outbounds) {
						return
					}
					// Strip USER patch из Updates[]; preset patches preserve.
					pc.ParserConfig.Outbounds[r2.IndexInSlice].Updates = build.UpsertUserPatch(
						pc.ParserConfig.Outbounds[r2.IndexInSlice].Updates,
						nil,
					)
					refreshList()
					if onApply != nil {
						onApply()
					}
				}, rowGetter)
				resetBtn.Importance = widget.LowImportance
				fynewidget.SetToolTipSafe(resetBtn, "Reset — clear your changes, revert to defaults")
				if !r.HasUserPatch {
					resetBtn.Disable()
				}
			}

			leftArrows = container.NewHBox(upBtn, downBtn)
			// fixedWidthBtn — обёртка, фиксирующая минимальную ширину кнопки
			// (Reset > Del по тексту; без фиксации колонка действий "прыгает"
			// между rows). Stack комбинирует MinSize: max(sizer, btn).
			fixedWidthBtn := func(btn fyne.CanvasObject) fyne.CanvasObject {
				sizer := canvas.NewRectangle(color.Transparent)
				sizer.SetMinSize(fyne.NewSize(78, 0))
				return container.NewStack(sizer, btn)
			}
			if r.IsPreset || r.IsRequired {
				// Locked rows: Edit + Reset, без Del.
				rightControls = container.NewHBox(editBtn, fixedWidthBtn(resetBtn), rightPadding)
			} else {
				// Regular: Edit + Del.
				rightControls = container.NewHBox(editBtn, fixedWidthBtn(delBtn), rightPadding)
			}

			rowInner := container.NewBorder(nil, nil, leftArrows, rightControls, nameLabel)
			row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{})
			row.WireTooltipLabelHover(nameLabel)
			items = append(items, row)
		}
		listContent.Objects = items
		listContent.Refresh()
	}

	refreshList()

	addBtn := widget.NewButtonWithIcon(locale.T("wizard.outbound.button_add"), theme.ContentAddIcon(), func() {
		parserConfig := getParserConfig(editPresenter.Model())
		if parserConfig == nil {
			return
		}
		existingTags := collectAllTags(parserConfig)
		ShowEditDialog(parent, editPresenter, nil, true, -1, existingTags, func(updated *config.OutboundConfig, scopeKind string, sourceIndex int) {
			if scopeKind == "global" || sourceIndex < 0 {
				parserConfig.ParserConfig.Outbounds = append(parserConfig.ParserConfig.Outbounds, *updated)
			} else {
				for sourceIndex >= len(parserConfig.ParserConfig.Proxies) {
					parserConfig.ParserConfig.Proxies = append(parserConfig.ParserConfig.Proxies, config.ProxySource{})
				}
				parserConfig.ParserConfig.Proxies[sourceIndex].Outbounds = append(parserConfig.ParserConfig.Proxies[sourceIndex].Outbounds, *updated)
			}
			refreshList()
			if onApply != nil {
				onApply()
			}
		})
	})

	// SPEC 057-R-N: Restore missing template outbounds — recovery для случая,
	// когда юзер случайно удалил template-defined entries (auto-proxy-out,
	// vpn ①, vpn ② и т.п.). Walk template.parser_config.outbounds; для
	// каждого tag'а не в current state — append в конец. Required outbound'ы
	// (proxy-out) restore'нутся первыми если отсутствуют.
	// Note: direct-out не в template.parser_config — это sing-box built-in
	// (если нужен — добавится через Add).
	// ttwidget.Button нативно поддерживает SetToolTip (в отличие от
	// fynewidget.HoverForwardButton, который только wraps standard widget.Button).
	// Кнопка standalone (вне row) — hover forwarding не нужен.
	restoreBtn := ttwidget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		model := editPresenter.Model()
		pc := getParserConfig(model)
		if pc == nil {
			return
		}
		existing := make(map[string]bool, len(pc.ParserConfig.Outbounds))
		for _, ob := range pc.ParserConfig.Outbounds {
			existing[ob.Tag] = true
		}
		tmplOutbounds := templateGlobalOutbounds(model)
		added := 0
		for _, tmplOb := range tmplOutbounds {
			if tmplOb.Tag == "" || existing[tmplOb.Tag] {
				continue
			}
			// SPEC 058-R-N: добавляем thin referenced entry (только tag + ref),
			// body live из template на render. Не копируем полный body.
			pc.ParserConfig.Outbounds = append(pc.ParserConfig.Outbounds, config.OutboundConfig{
				Tag: tmplOb.Tag,
				Ref: config.RefTemplate,
			})
			existing[tmplOb.Tag] = true
			added++
		}
		if added > 0 {
			refreshList()
			if onApply != nil {
				onApply()
			}
		}
	})
	restoreBtn.Importance = widget.LowImportance
	restoreBtn.SetToolTip("Restore template-defined outbounds that were deleted (e.g. auto-proxy-out, vpn ①, vpn ②). Existing entries unchanged.")

	scroll := container.NewScroll(listContent)
	scroll.SetMinSize(fyne.NewSize(0, 280))

	rightTopButtons := container.NewHBox(restoreBtn, addBtn)
	top := container.NewBorder(nil, nil, nil, rightTopButtons, widget.NewLabel(locale.T("wizard.outbound.configurator_label")))
	return container.NewBorder(
		top,
		nil,
		nil, nil,
		scroll,
	), refreshList
}
