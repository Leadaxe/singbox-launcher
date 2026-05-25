// File rules_unified_rows.go — единый renderer строк правил для Rules tab.
//
// Обходит model.RuleOrder в порядке слотов; для каждого slot dispatch'ит:
//   - SlotKindCustom → существующий tile builder из rules_tab.go (rules_box.Add)
//     для одного CustomRule (legacy inline/srs)
//   - SlotKindPresetRef → preset-ref tile (см. ниже)
//
// Drag ↑↓ и delete действуют на индексы RuleOrder, не на CustomRules/PresetRefs.
// При delete юзер видит как пропадает конкретный tile; CompactRuleOrderIndices
// поддерживает целостность ссылок на сдвинувшиеся индексы.
package tabs

import (
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core/services"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// buildUnifiedRuleRows — один обход через RuleOrder, рендерит tile per slot.
func buildUnifiedRuleRows(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	availableOutbounds []string,
	showAddRuleDialog ShowAddRuleDialogFunc,
	rulesBox *fyne.Container,
) {
	for slotIdx, slot := range model.RuleOrder {
		switch slot.Kind {
		case wizardmodels.SlotKindCustom:
			if slot.Index < 0 || slot.Index >= len(model.CustomRules) {
				continue
			}
			buildSingleCustomRuleRow(presenter, model, guiState, availableOutbounds, showAddRuleDialog, rulesBox, slot.Index, slotIdx)
		case wizardmodels.SlotKindPresetRef:
			if slot.Index < 0 || slot.Index >= len(model.PresetRefs) {
				continue
			}
			buildSinglePresetRefRow(presenter, model, guiState, availableOutbounds, showAddRuleDialog, rulesBox, slot.Index, slotIdx)
		}
	}
}

// buildSinglePresetRefRow рисует tile для одного preset-ref'а (kind=preset).
// Tile match что у CustomRule tile: drag ↑↓ / enable / label+summary / edit / delete.
// Drag оперирует индексами model.RuleOrder.
//
// availableOutbounds — список outbound tag'ов; если preset имеет **ровно одну**
// var типа "outbound", показываем inline-selector справа (по образу custom rule
// tile). Multi-outbound presets (split-all-traffic) inline-selector не получают —
// только через edit-dialog.
func buildSinglePresetRefRow(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	availableOutbounds []string,
	showAddRuleDialog ShowAddRuleDialogFunc,
	rulesBox *fyne.Container,
	refIdx int,
	slotIdx int,
) {
	pr := model.PresetRefs[refIdx]

	// Lookup template preset for label / vars schema (broken-preset → marker).
	var tplPreset *wizardtemplate.Preset
	if model.TemplateData != nil {
		for i := range model.TemplateData.Presets {
			if model.TemplateData.Presets[i].ID == pr.Ref {
				tplPreset = &model.TemplateData.Presets[i]
				break
			}
		}
	}

	labelText, brokenRef := presetTileLabel(pr, tplPreset)
	srsEntries := presetRefSRSEntries(pr, tplPreset)

	// HoverRow + rowGetter — тот же паттерн что в buildSingleCustomRuleRow
	// (rules_tab.go::247): hover-подсветка всего ряда, label с tooltip,
	// HoverForwardButton'ы прокидывают hover-event'ы из дочерних виджетов на
	// HoverRow, чтобы вся строка подсвечивалась при наведении на любую кнопку.
	var row *fynewidget.HoverRow
	rowGetter := func() *fynewidget.HoverRow { return row }

	label := ttwidget.NewLabel(labelText)
	label.Wrapping = fyne.TextWrapOff
	label.Truncation = fyne.TextTruncateEllipsis
	if tplPreset != nil {
		if d := strings.TrimSpace(tplPreset.Description); d != "" {
			label.SetToolTip(d)
		}
	}

	// Inline outbound selector: показываем если у preset'а **ровно одна** var
	// типа "outbound" (типичный кейс — `out`). Multi-outbound presets
	// (split-all-traffic с out_a + out_b) — только через edit dialog.
	var soloOutVar *wizardtemplate.PresetVar
	if tplPreset != nil {
		outCount := 0
		for i := range tplPreset.Vars {
			if tplPreset.Vars[i].Type == "outbound" {
				outCount++
				if soloOutVar == nil {
					soloOutVar = &tplPreset.Vars[i]
				}
			}
		}
		if outCount != 1 {
			soloOutVar = nil
		}
	}
	var outSel *fynewidget.HoverForwardSelect
	if soloOutVar != nil {
		options := append([]string(nil), availableOutbounds...)
		currentVal := pr.Vars[soloOutVar.Name]
		if currentVal == "" {
			currentVal = soloOutVar.Default
		}
		// Если текущий outbound отсутствует в options (например sentinel "drop"
		// для stop-http3 не входит в обычный proxy-list) — добавляем впереди,
		// чтобы Select мог его отобразить.
		seen := false
		for _, o := range options {
			if o == currentVal {
				seen = true
				break
			}
		}
		if !seen && currentVal != "" {
			options = append([]string{currentVal}, options...)
		}
		// Тот же anti-loop приём что у checkbox'а: создаём с nil callback,
		// ставим Selected напрямую (не SetSelected — он триггерит OnChanged
		// когда value меняется), потом назначаем OnChanged. Без этого initial
		// render каждого preset-ряда триггерил бы MarkAsChanged + (для presetов
		// с outbounds) refreshRulesTabFromPresenter → rebuild → infinite cascade.
		outSel = fynewidget.NewHoverForwardSelect(options, nil, rowGetter)
		outSel.Selected = currentVal
		outSel.OnChanged = func(value string) {
			if pr.Vars == nil {
				pr.Vars = make(map[string]string)
			}
			pr.Vars[soloOutVar.Name] = value
			model.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
		}
	}

	// SRS download-on-enable flow (тот же паттерн что в rules_tab.go::332-360
	// для legacy custom rules):
	//   - srsBtn создаётся ниже (облачко справа), его OnTapped запускает download
	//   - checkbox callback при click ON + SRS missing: взвести флаг + триггернуть
	//     srsBtn.OnTapped() + откатить визуально в OFF
	//   - srsBtn на success колбэке: если флаг взведён → pr.Enabled = true,
	//     refresh tab (rebuild row уже покажет checkbox ON через SetChecked)
	var srsBtn *ttwidget.Button
	enableOnSRSSuccess := false

	// Создаём checkbox БЕЗ OnChanged, потом ставим Checked напрямую через
	// field (не SetChecked — он триггерит OnChanged когда value меняется),
	// потом назначаем OnChanged. Без этой осторожности initial рендер
	// enabled-preset'а триггерил бы callback → MarkAsChanged → (для preset'ов
	// с outbounds) refreshRulesTabFromPresenter → новый checkbox → infinite loop.
	var enableCh *widget.Check
	enableCh = widget.NewCheck("", nil)
	enableCh.Checked = pr.Enabled
	enableCh.OnChanged = func(on bool) {
		if on && len(srsEntries) > 0 && model.ExecDir != "" &&
			!services.AllSRSDownloadedForEntries(model.ExecDir, srsEntries) {
			if srsBtn != nil {
				enableOnSRSSuccess = true
				srsBtn.OnTapped()
			}
			enableCh.SetChecked(false)
			return
		}
		pr.Enabled = on
		presenter.MarkAsChanged()
		model.TemplatePreviewNeedsUpdate = true
		// Sync inline outbound selector enable-state с preset toggle (как в
		// legacy custom rule tile: outboundSelect.Enable/Disable в callback'е).
		if outSel != nil {
			if on {
				outSel.Enable()
			} else {
				outSel.Disable()
			}
		}
		// Единая точка для всех аftermath'ов preset toggle:
		// DNS UI refresh + outbounds eager sync + outbounds UI + available
		// outbounds для Rules/Final selects (см. RefreshAfterPresetToggle docstring).
		presenter.RefreshAfterPresetToggle()
		// Если preset имеет outbounds[] с mode=add — rebuild Rules tab чтобы
		// inline preset-ref selects других rows видели новые/удалённые tag'и.
		// (RefreshAfterPresetToggle уже обновил RefreshOutboundOptions, но
		// rebuild всего таба нужен только когда tag set реально меняется.)
		if presetHasAddOutbounds(tplPreset) {
			refreshRulesTabFromPresenter(presenter, showAddRuleDialog)
		}
	}
	setTooltip(enableCh, locale.T("wizard.rules.tooltip_rule_enabled"))
	if brokenRef {
		enableCh.Disable()
	}
	// Initial disable если preset выключен.
	if outSel != nil && (brokenRef || !pr.Enabled) {
		outSel.Disable()
	}

	editBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.DocumentCreateIcon(), func() {
		showEditPresetRefDialog(presenter, model, guiState, refIdx, showAddRuleDialog)
	}, rowGetter)
	editBtn.Importance = widget.LowImportance
	setTooltip(editBtn, locale.T("wizard.shared.button_edit"))
	if brokenRef {
		editBtn.Disable()
	}

	delBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.DeleteIcon(), func() {
		dialog.ShowConfirm(
			"Confirmation",
			fmt.Sprintf("Delete preset '%s'?", labelText),
			func(ok bool) {
				if !ok {
					return
				}
				deletedIdx := refIdx
				model.PresetRefs = append(model.PresetRefs[:deletedIdx], model.PresetRefs[deletedIdx+1:]...)
				wizardmodels.CompactRuleOrderIndices(model, wizardmodels.SlotKindPresetRef, deletedIdx)
				model.TemplatePreviewNeedsUpdate = true
				presenter.MarkAsChanged()
				refreshRulesTabFromPresenter(presenter, showAddRuleDialog)
			},
			guiState.Window,
		)
	}, rowGetter)
	delBtn.Importance = widget.LowImportance
	setTooltip(delBtn, locale.T("wizard.rules.button_delete"))

	upBtn := fynewidget.NewHoverForwardButton("↑", func() {
		moveSlotUp(presenter, model, slotIdx, showAddRuleDialog)
	}, rowGetter)
	upBtn.Importance = widget.LowImportance
	if slotIdx <= 0 {
		upBtn.Disable()
		setTooltip(upBtn, locale.T("wizard.rules.tooltip_move_up_off"))
	} else {
		setTooltip(upBtn, locale.T("wizard.rules.tooltip_move_up"))
	}

	downBtn := fynewidget.NewHoverForwardButton("↓", func() {
		moveSlotDown(presenter, model, slotIdx, showAddRuleDialog)
	}, rowGetter)
	downBtn.Importance = widget.LowImportance
	if slotIdx >= len(model.RuleOrder)-1 {
		downBtn.Disable()
		setTooltip(downBtn, locale.T("wizard.rules.tooltip_move_down_off"))
	} else {
		setTooltip(downBtn, locale.T("wizard.rules.tooltip_move_down"))
	}

	// SRS-облачко: показываем если preset (с учётом текущих vars) содержит
	// remote rule_set'ы которые ещё не скачаны. На клик — скачивание всех
	// remote rule_set'ов preset'а через services.DownloadSRSGroup.
	// Когда юзер выключит var управляющий remote rule_set (например geoip_enabled)
	// → presetRefSRSEntries вернёт пустой list → облачко исчезнет.
	var srsHF *fynewidget.HoverForwardTTButton
	var srsWarn *ttwidget.Label
	srsMissingEnabled := false
	if len(srsEntries) > 0 && model.ExecDir != "" {
		srsHF = makePresetSRSButton(presenter, model, guiState, srsEntries, showAddRuleDialog, pr, &enableOnSRSSuccess, rowGetter)
		srsBtn = srsHF.TTWidget()
		// SRS-warning badge: preset enabled в state но файлы не скачены →
		// rule фактически не работает (sing-box упадёт на missing .srs).
		// Visual ⚠ + auto-download silently в фоне ниже. Defensive против
		// сценариев: (a) файлы потёрли вручную, (b) template добавил srs_url
		// в уже-enabled preset, (c) load state'а с broken cache.
		if pr.Enabled && !services.AllSRSDownloadedForEntries(model.ExecDir, srsEntries) {
			srsMissingEnabled = true
			srsWarn = ttwidget.NewLabel("⚠")
			srsWarn.Importance = widget.WarningImportance
			srsWarn.SetToolTip(locale.T("wizard.rules.tooltip_srs_missing_enabled"))
		}
	}

	// Клик по label — тоггл checkbox'а (как в legacy custom rule row).
	labelTap := fynewidget.NewTapWrap(label, func() {
		if enableCh.Disabled() {
			return
		}
		enableCh.SetChecked(!enableCh.Checked)
	})

	leftLead := container.NewHBox(upBtn, downBtn, fynewidget.CheckLeadingWrap(enableCh))
	var rightCluster *fyne.Container
	if outSel != nil {
		rightCluster = container.NewHBox(editBtn, delBtn, outSel)
	} else {
		rightCluster = container.NewHBox(editBtn, delBtn)
	}
	var center fyne.CanvasObject = labelTap
	if srsHF != nil {
		// ⚠ slot — между center label'ом и srs download'ом, если SRS
		// missing + preset enabled (см. srsMissingEnabled блок выше).
		var srsCluster fyne.CanvasObject = srsHF
		if srsWarn != nil {
			srsCluster = container.NewHBox(srsWarn, srsHF)
		}
		center = container.NewBorder(nil, nil, nil, srsCluster, labelTap)
	}
	rowInner := container.NewBorder(nil, nil, leftLead, rightCluster, center)
	row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{})
	row.WireTooltipLabelHover(label)
	rulesBox.Add(row)

	// Auto-download silent: SRS missing у enabled preset'а — пробуем
	// тихо скачать сразу. Failure не показывает popup (silent=true) —
	// юзер увидит остающуюся ⚠ + ⬇ srs кнопку для ручного retry.
	// Success path: refreshRulesTabFromPresenter пересоберёт row → srsWarn
	// исчезнет, btn станет "✔️ srs". Cascade safety: success-path не
	// re-kick'ает (после success !AllSRSDownloaded → false → check fails).
	if srsMissingEnabled && srsHF != nil {
		runSRSDownloadAsync(presenter, model, guiState, srsEntries, srsHF.TTWidget(), nil,
			func() {
				model.TemplatePreviewNeedsUpdate = true
				// НЕ вызываем MarkAsChanged — это auto-fix, не user action.
				refreshRulesTabFromPresenter(presenter, showAddRuleDialog)
			},
			true, /* silent */
		)
	}
}

// makePresetSRSButton — облачко скачивания remote rule_set'ов preset'а.
// Текст кнопки: "Download" если не все скачаны, "Downloaded ✓" если все есть.
// Download-flow делегирован в общий runSRSDownloadAsync (см. rules_tab.go).
//
// enableOnSuccess (nil-safe) — указатель на флаг "если успех, включить preset".
// Взводится из callback'а enable-checkbox'а (download-on-enable flow). Сбрасывается
// в onSuccess (одноразовый). На failure runSRSDownloadAsync не вызывает onSuccess —
// флаг остаётся взведённым (тот же паттерн что в legacy custom rules tile).
func makePresetSRSButton(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	entries []services.SRSEntry,
	showAddRuleDialog ShowAddRuleDialogFunc,
	pr *wizardmodels.PresetRefState,
	enableOnSuccess *bool,
	rowGetter fynewidget.RowHoverGetter,
) *fynewidget.HoverForwardTTButton {
	initialText := srsBtnDownload()
	if services.AllSRSDownloadedForEntries(model.ExecDir, entries) {
		initialText = srsBtnDone()
	}
	btn := fynewidget.NewHoverForwardTTButton(initialText, nil, rowGetter)
	btn.Importance = widget.LowImportance
	if tip := srsEntriesTooltip(entries); tip != "" {
		btn.TTWidget().SetToolTip(tip)
	}
	btn.OnTapped = func() {
		runSRSDownloadAsync(presenter, model, guiState, entries, btn.TTWidget(), nil /* no outboundSelect */, func() {
			model.TemplatePreviewNeedsUpdate = true
			if enableOnSuccess != nil && *enableOnSuccess {
				*enableOnSuccess = false
				if pr != nil {
					pr.Enabled = true
					presenter.RefreshDNSListAndSelects()
				}
			}
			presenter.MarkAsChanged()
			refreshRulesTabFromPresenter(presenter, showAddRuleDialog)
		}, false /* manual click — показываем фейл-диалог */)
	}
	return btn
}

// presetHasAddOutbounds — true если preset имеет хотя бы один outbounds[]
// entry с mode="add" (или mode="" → дефолт add). mode="update" игнорируется
// (он не вводит новых tag'ов в available-outbounds set).
//
// SPEC 056: используется при toggle preset-ref enable/disable, чтобы решить
// — нужен ли full Rules tab rebuild + RefreshOutboundOptions. При false →
// preset влияет только на route rule (через ExpandPreset) → достаточно
// уже выполненных MarkAsChanged + TemplatePreviewNeedsUpdate.
func presetHasAddOutbounds(tpl *wizardtemplate.Preset) bool {
	if tpl == nil {
		return false
	}
	for _, ob := range tpl.Outbounds {
		mode := ob.Mode
		if mode == "" {
			mode = "add"
		}
		if mode == "add" {
			return true
		}
	}
	return false
}

// presetTileLabel — текст для tile preset-ref'а: 🔗 + label + non-default vars summary.
// Префикс 🔗 — visual marker «правило из библиотеки пресетов» (kind=preset),
// отличает от user-добавленных inline/srs (`+Add Rule`). Origin тривиально
// читается из state: kind=preset → 🔗, остальное — пользовательское.
// Возвращает (text, brokenRef): brokenRef=true когда preset не найден в template.
func presetTileLabel(pr *wizardmodels.PresetRefState, tpl *wizardtemplate.Preset) (string, bool) {
	if tpl == nil {
		return fmt.Sprintf("🔗 ⚠ Broken preset: %s", pr.Ref), true
	}
	labelText := tpl.Label
	if labelText == "" {
		labelText = tpl.ID
	}
	if summary := summarizePresetVarsCompact(pr, tpl); summary != "" {
		labelText += "  ·  " + summary
	}
	return "🔗 " + labelText, false
}

func summarizePresetVarsCompact(pr *wizardmodels.PresetRefState, tpl *wizardtemplate.Preset) string {
	if pr == nil || tpl == nil || len(pr.Vars) == 0 {
		return ""
	}
	defaults := make(map[string]string, len(tpl.Vars))
	for _, v := range tpl.Vars {
		defaults[v.Name] = v.Default
	}
	keys := make([]string, 0, len(pr.Vars))
	for k := range pr.Vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := pr.Vars[k]
		if v == "" || v == defaults[k] {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ", ")
}

// moveSlotUp / moveSlotDown — swap slots в RuleOrder.
func moveSlotUp(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, slotIdx int, showAddRuleDialog ShowAddRuleDialogFunc) {
	if slotIdx <= 0 || slotIdx >= len(model.RuleOrder) {
		return
	}
	model.RuleOrder[slotIdx], model.RuleOrder[slotIdx-1] = model.RuleOrder[slotIdx-1], model.RuleOrder[slotIdx]
	model.TemplatePreviewNeedsUpdate = true
	presenter.MarkAsChanged()
	refreshRulesTabFromPresenter(presenter, showAddRuleDialog)
}

func moveSlotDown(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, slotIdx int, showAddRuleDialog ShowAddRuleDialogFunc) {
	if slotIdx < 0 || slotIdx >= len(model.RuleOrder)-1 {
		return
	}
	model.RuleOrder[slotIdx], model.RuleOrder[slotIdx+1] = model.RuleOrder[slotIdx+1], model.RuleOrder[slotIdx]
	model.TemplatePreviewNeedsUpdate = true
	presenter.MarkAsChanged()
	refreshRulesTabFromPresenter(presenter, showAddRuleDialog)
}
