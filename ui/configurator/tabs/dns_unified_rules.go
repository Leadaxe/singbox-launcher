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
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core/build"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/fynewidget"
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
	for slotIdx, slot := range model.DNSRuleOrder {
		switch slot.Kind {
		case wizardmodels.DNSSlotKindUser:
			if slot.Index < 0 || slot.Index >= len(model.DNSUserRules) {
				continue
			}
			buildSingleDNSUserRuleRow(presenter, model, parentWindow, dnsRulesBox, slot.Index, slotIdx, refreshAll)
		case wizardmodels.DNSSlotKindPresetRef:
			if slot.Index < 0 || slot.Index >= len(model.PresetRefs) {
				continue
			}
			buildSingleDNSPresetRuleRow(presenter, model, parentWindow, dnsRulesBox, slot.Index, slotIdx, refreshAll)
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
) {
	ur := &model.DNSUserRules[userIdx]

	var row *fynewidget.HoverRow
	rowGetter := func() *fynewidget.HoverRow { return row }

	title, tooltip := dnsRuleSummary(ur.Body)
	label := ttwidget.NewLabel("→ " + title)
	label.Wrapping = fyne.TextTruncate
	if tooltip != "" {
		label.SetToolTip(tooltip)
	}

	// Per-row enable toggle. DNSUserRule.Enabled — disabled → skip emit на Save.
	var enableCh *widget.Check
	enableCh = widget.NewCheck("", nil)
	enableCh.Checked = ur.Enabled
	enableCh.OnChanged = func(on bool) {
		ur.Enabled = on
		// Sync DNSRulesText (deprecated derived view) для совместимости с
		// raw-JSON editor toggle.
		model.DNSRulesText = wizardmodels.DNSUserRulesToText(model.DNSUserRules)
		syncDNSRulesTextToHiddenEntry(presenter)
		model.TemplatePreviewNeedsUpdate = true
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
				model.TemplatePreviewNeedsUpdate = true
				presenter.MarkAsChanged()
				if refreshAll != nil {
					refreshAll()
				}
			},
			parentWindow,
		)
	}, rowGetter)
	delBtn.Importance = widget.LowImportance

	upBtn := fynewidget.NewHoverForwardButton("↑", func() {
		moveDNSSlotUp(presenter, model, slotIdx, refreshAll)
	}, rowGetter)
	upBtn.Importance = widget.LowImportance
	if slotIdx <= 0 {
		upBtn.Disable()
	}
	downBtn := fynewidget.NewHoverForwardButton("↓", func() {
		moveDNSSlotDown(presenter, model, slotIdx, refreshAll)
	}, rowGetter)
	downBtn.Importance = widget.LowImportance
	if slotIdx >= len(model.DNSRuleOrder)-1 {
		downBtn.Disable()
	}

	// Shared row scaffolding (see row_scaffold.go).
	leftLead := buildRowLeftLead(upBtn, downBtn, enableCh)
	right := buildRowEditDelCluster(editBtn, delBtn)
	row = finalizeRow(dnsRulesBox, leftLead, right, label, label)
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

	// Resolve dns_rule body для tooltip + View JSON.
	frags, _, ok := build.ExpandPreset(tplPreset, pr.Vars, runtime.GOOS, runtime.GOARCH)
	var ruleBody map[string]interface{}
	if ok && frags.DNSRule != nil {
		ruleBody = frags.DNSRule
	}

	titleLabel := ttwidget.NewLabel(labelText)
	titleLabel.Wrapping = fyne.TextTruncate
	if ruleBody != nil {
		_, tooltip := dnsRuleSummary(ruleBody)
		if tooltip != "" {
			titleLabel.SetToolTip(tooltip)
		}
	}

	// Enable toggle. Pulls from PresetRefState.DNSRuleEnabled (default true).
	// Если pr.Enabled == false на уровне route — preset выключен глобально,
	// dns_rule тоже не активен. В таком случае дизейблим чекбокс с тултипом.
	var enableCh *widget.Check
	enableCh = widget.NewCheck("", nil)
	enableCh.Checked = pr.IsDNSRuleEnabled() && pr.Enabled
	enableCh.OnChanged = func(on bool) {
		pr.SetDNSRuleEnabled(on)
		model.TemplatePreviewNeedsUpdate = true
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

	upBtn := fynewidget.NewHoverForwardButton("↑", func() {
		moveDNSSlotUp(presenter, model, slotIdx, refreshAll)
	}, rowGetter)
	upBtn.Importance = widget.LowImportance
	if slotIdx <= 0 {
		upBtn.Disable()
	}
	downBtn := fynewidget.NewHoverForwardButton("↓", func() {
		moveDNSSlotDown(presenter, model, slotIdx, refreshAll)
	}, rowGetter)
	downBtn.Importance = widget.LowImportance
	if slotIdx >= len(model.DNSRuleOrder)-1 {
		downBtn.Disable()
	}

	// Shared row scaffolding (see row_scaffold.go). View-only row: no edit/del.
	leftLead := buildRowLeftLead(upBtn, downBtn, enableCh)
	right := container.NewHBox(viewBtn)
	row = finalizeRow(dnsRulesBox, leftLead, right, titleLabel, titleLabel)
}

// moveDNSSlotUp / moveDNSSlotDown — swap slots в DNSRuleOrder. Refresh
// rebuild'ит весь список (тот же паттерн что moveSlotUp в rules_unified_rows.go).
func moveDNSSlotUp(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, slotIdx int, refreshAll func()) {
	if slotIdx <= 0 || slotIdx >= len(model.DNSRuleOrder) {
		return
	}
	model.DNSRuleOrder[slotIdx], model.DNSRuleOrder[slotIdx-1] = model.DNSRuleOrder[slotIdx-1], model.DNSRuleOrder[slotIdx]
	model.TemplatePreviewNeedsUpdate = true
	wizardbusiness.InvalidatePreviewCache(model) // drop cached preview so Preview tab reflects new order
	presenter.MarkAsChanged()
	if refreshAll != nil {
		refreshAll()
	}
}

func moveDNSSlotDown(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, slotIdx int, refreshAll func()) {
	if slotIdx < 0 || slotIdx >= len(model.DNSRuleOrder)-1 {
		return
	}
	model.DNSRuleOrder[slotIdx], model.DNSRuleOrder[slotIdx+1] = model.DNSRuleOrder[slotIdx+1], model.DNSRuleOrder[slotIdx]
	model.TemplatePreviewNeedsUpdate = true
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
