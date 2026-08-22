// File dns_unified_rules.go — единый renderer строк DNS-правил для DNS tab
// (SPEC 062-F-N WIZARD_DNS_RULES_UNIFIED_ORDER).
//
// Обходит model.DNSRuleOrder в порядке слотов; для каждого slot dispatch'ит:
//   - DNSSlotKindPresetRef → preset DNS rule row (🔗 prefix, read-only body,
//     View JSON, 🔒 на required preset)
//   - DNSSlotKindUser → user DNS rule row (→ prefix, edit/delete, summary)
//
// Drag ↑↓ оперирует индексами DNSRuleOrder, не подлежащими списками. Delete
// для user-rule делает append + CompactDNSRuleOrderIndices; preset-rule
// удалить нельзя (он живёт пока активен preset-ref в Rules tab).
package tabs

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core/build"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// buildUnifiedDNSRuleRows — обходит model.DNSRuleOrder и рендерит per-row
// widget в dnsRulesBox. Drag ↑↓, enable, edit (для user), delete (для user),
// View JSON (для preset) — всё в одной строке через HoverRow.
func buildUnifiedDNSRuleRows(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	parentWindow fyne.Window,
	dnsRulesBox *fyne.Container,
	refreshAll func(),
) {
	// Drag group for this rebuild; discarded together with the rows it tracks
	// (same lifecycle as the Rules tab group — see CreateRulesTab).
	dragGroup := fynewidget.NewDragReorderGroup(func(from, to int) {
		moveDNSSlot(presenter, model, from, to, refreshAll)
	})
	for slotIdx, slot := range model.DNSRuleOrder {
		switch slot.Kind {
		case wizardmodels.DNSSlotKindUser:
			if slot.Index < 0 || slot.Index >= len(model.DNSUserRules) {
				continue
			}
			buildSingleDNSUserRuleRow(presenter, model, parentWindow, dnsRulesBox, slot.Index, slotIdx, refreshAll, dragGroup)
		case wizardmodels.DNSSlotKindPresetRef:
			if slot.Index < 0 || slot.Index >= len(model.PresetRefs) {
				continue
			}
			buildSingleDNSPresetRuleRow(presenter, model, parentWindow, dnsRulesBox, slot.Index, slotIdx, refreshAll, dragGroup)
		}
	}
}

// buildSingleDNSUserRuleRow — один tile для DNSUserRules[userIdx].
// → prefix + summary (server + match fields). Edit ✏ открывает диалог,
// Delete 🗑 убирает запись и slot.
func buildSingleDNSUserRuleRow(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	parentWindow fyne.Window,
	dnsRulesBox *fyne.Container,
	userIdx, slotIdx int,
	refreshAll func(),
	dragGroup *fynewidget.DragReorderGroup,
) {
	ur := &model.DNSUserRules[userIdx]

	var row *fynewidget.HoverRow
	rowGetter := func() *fynewidget.HoverRow { return row }

	title, tooltip := dnsRuleSummary(ur.Body)
	// Префикс пользовательского правила — эмодзи, а не «→» (U+2192): в
	// основном шрифте темы Fyne этого глифа нет, и вместо стрелки рисовалась
	// «плитка» U+FFFD. Эмодзи идут отдельным шрифтом и отображаются всегда —
	// ровно так же, как «🔗» у preset-правил в соседней строке.
	label := ttwidget.NewLabel("✏️ " + title)
	label.Truncation = fyne.TextTruncateClip
	if tooltip != "" {
		label.SetToolTip(tooltip)
	}

	// Per-row enable toggle. DNSUserRule.Enabled — disabled → skip emit на Save.
	enableCh := widget.NewCheck("", nil)
	enableCh.Checked = ur.Enabled
	enableCh.OnChanged = func(on bool) {
		ur.Enabled = on
		// Sync DNSRulesText (deprecated derived view) для совместимости с
		// raw-JSON editor toggle.
		model.DNSRulesText = wizardmodels.DNSUserRulesToText(model.DNSUserRules)
		syncDNSRulesTextToHiddenEntry(presenter)
		presenter.MarkAsChanged()
	}

	editBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.DocumentCreateIcon(), func() {
		showEditUserDNSRuleDialog(presenter, parentWindow, userIdx, refreshAll)
	}, rowGetter)
	editBtn.Importance = widget.LowImportance

	delBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.DeleteIcon(), func() {
		dialog.ShowConfirm(
			"Confirmation",
			"Delete this DNS rule?",
			func(ok bool) {
				if !ok {
					return
				}
				deletedIdx := userIdx
				if deletedIdx < 0 || deletedIdx >= len(model.DNSUserRules) {
					return
				}
				model.DNSUserRules = append(model.DNSUserRules[:deletedIdx], model.DNSUserRules[deletedIdx+1:]...)
				wizardmodels.CompactDNSRuleOrderIndices(model, wizardmodels.DNSSlotKindUser, deletedIdx)
				model.DNSRulesText = wizardmodels.DNSUserRulesToText(model.DNSUserRules)
				presenter.MarkAsChanged()
				if refreshAll != nil {
					refreshAll()
				}
			},
			parentWindow,
		)
	}, rowGetter)
	delBtn.Importance = widget.LowImportance

	dragHandle := fynewidget.NewDragHandle(dragGroup, slotIdx, rowGetter)
	setTooltip(dragHandle, locale.T("wizard.rules.tooltip_drag_reorder"))

	// Shared row scaffolding (see row_scaffold.go).
	leftLead := buildRowDragLead(dragHandle, enableCh)
	right := buildRowEditDelCluster(editBtn, delBtn)
	row = finalizeDragRow(dnsRulesBox, dragGroup, slotIdx, leftLead, right, label, label)
}

// buildSingleDNSPresetRuleRow — один tile для preset-ref DNS rule.
// 🔗 prefix + preset label. 🔒 если route-preset был required в template
// (зеркало template.dns_options.servers[].required). Read-only body
// (через View JSON dialog).
func buildSingleDNSPresetRuleRow(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	parentWindow fyne.Window,
	dnsRulesBox *fyne.Container,
	refIdx, slotIdx int,
	refreshAll func(),
	dragGroup *fynewidget.DragReorderGroup,
) {
	pr := model.PresetRefs[refIdx]

	var tplPreset *wizardtemplate.Preset
	if model.TemplateData != nil {
		for i := range model.TemplateData.Presets {
			if model.TemplateData.Presets[i].ID == pr.Ref {
				tplPreset = &model.TemplateData.Presets[i]
				break
			}
		}
	}

	// Skip slot entirely if preset has no dns_rule (template doesn't define one).
	// Это не должно случаться при правильно построенном DNSRuleOrder, но defensive.
	if tplPreset == nil || !tplPreset.PresetHasDNSRule() {
		return
	}

	var row *fynewidget.HoverRow
	rowGetter := func() *fynewidget.HoverRow { return row }

	presetLabel := tplPreset.Label
	if presetLabel == "" {
		presetLabel = tplPreset.ID
	}
	labelText := "🔗 " + presetLabel

	// Resolve dns_rule body для tooltip + View JSON. SPEC 085.1: пресет может
	// нести несколько DNS-правил под одним slot'ом — для summary берём первое
	// доступное (singular DNSRule, иначе первый элемент DNSRules).
	frags, _, ok := build.ExpandPresetWithGlobals(tplPreset, pr.Vars, model.SettingsVars, model.Target)
	var ruleBody map[string]interface{}
	if ok {
		if frags.DNSRule != nil {
			ruleBody = frags.DNSRule
		} else if len(frags.DNSRules) > 0 {
			ruleBody = frags.DNSRules[0]
		}
	}

	titleLabel := ttwidget.NewLabel(labelText)
	titleLabel.Truncation = fyne.TextTruncateClip
	if ruleBody != nil {
		_, tooltip := dnsRuleSummary(ruleBody)
		if tooltip != "" {
			titleLabel.SetToolTip(tooltip)
		}
	}

	// Enable toggle. Pulls from PresetRefState.DNSRuleEnabled (default true).
	// Если pr.Enabled == false на уровне route — preset выключен глобально,
	// dns_rule тоже не активен. В таком случае дизейблим чекбокс с тултипом.
	enableCh := widget.NewCheck("", nil)
	enableCh.Checked = pr.IsDNSRuleEnabled() && pr.Enabled
	enableCh.OnChanged = func(on bool) {
		pr.SetDNSRuleEnabled(on)
		presenter.MarkAsChanged()
	}
	if !pr.Enabled {
		enableCh.Disable()
	}

	// View JSON кнопка (read-only inspect).
	viewBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.SearchIcon(), func() {
		body := ruleBody
		if body == nil {
			body = map[string]interface{}{}
		}
		showBundledDNSRuleDetailsDialog(parentWindow, tplPreset, body)
	}, rowGetter)
	viewBtn.Importance = widget.LowImportance

	dragHandle := fynewidget.NewDragHandle(dragGroup, slotIdx, rowGetter)
	setTooltip(dragHandle, locale.T("wizard.rules.tooltip_drag_reorder"))

	// Shared row scaffolding (see row_scaffold.go). View-only row: no edit/del.
	leftLead := buildRowDragLead(dragHandle, enableCh)
	right := container.NewHBox(viewBtn)
	row = finalizeDragRow(dnsRulesBox, dragGroup, slotIdx, leftLead, right, titleLabel, titleLabel)
}

// moveDNSSlot — переносит slot в DNSRuleOrder с позиции from на to
// (drag-and-drop). Refresh rebuild'ит весь список — тот же паттерн, что
// moveSlot в rules_unified_rows.go.
func moveDNSSlot(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, from, to int, refreshAll func()) {
	if !wizardmodels.MoveDNSRuleSlot(model, from, to) {
		return
	}
	wizardbusiness.InvalidatePreviewCache(model) // drop cached preview so Preview tab reflects new order
	presenter.MarkAsChanged()
	if refreshAll != nil {
		refreshAll()
	}
}

// addDNSUserRule — append new DNSUserRule + add slot to DNSRuleOrder.
// Используется кнопкой "+ Add Rule" в DNS tab. After save dialog, the dialog
// caller invokes this to persist.
func addDNSUserRule(model *wizardmodels.WizardModel, body map[string]interface{}) {
	if model == nil {
		return
	}
	newIdx := len(model.DNSUserRules)
	model.DNSUserRules = append(model.DNSUserRules, wizardmodels.DNSUserRule{
		Enabled: true,
		Body:    body,
	})
	model.DNSRuleOrder = append(model.DNSRuleOrder, wizardmodels.DNSRuleSlot{
		Kind:  wizardmodels.DNSSlotKindUser,
		Index: newIdx,
	})
	model.DNSRulesText = wizardmodels.DNSUserRulesToText(model.DNSUserRules)
}

// syncDNSRulesTextToHiddenEntry — пишет model.DNSRulesText в hidden
// DNSRulesEntry widget. Hidden widget остался от legacy editor mode и
// читается syncGUIToModelDNS на Save — без этого SyncGUIToModel перетёр
// бы model.DNSRulesText пустой строкой widget.Text.
func syncDNSRulesTextToHiddenEntry(presenter *wizardpresentation.WizardPresenter) {
	if presenter == nil {
		return
	}
	if gs := presenter.GUIState(); gs != nil && gs.DNSRulesEntry != nil {
		m := presenter.Model()
		if m != nil && gs.DNSRulesEntry.Text != m.DNSRulesText {
			gs.DNSRulesEntry.SetText(m.DNSRulesText)
		}
	}
}
