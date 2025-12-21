package tabs

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	wizardstate "singbox-launcher/ui/wizard/state"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// ShowAddRuleDialogFunc is a function type for showing the add rule dialog.
type ShowAddRuleDialogFunc func(state *wizardstate.WizardState, editRule *wizardstate.SelectableRuleState, ruleIndex int)

// CreateRulesTab creates the Rules tab UI.
// showAddRuleDialog is a function that will be called to show the add rule dialog.
func CreateRulesTab(state *wizardstate.WizardState, showAddRuleDialog ShowAddRuleDialogFunc) fyne.CanvasObject {
	if state.TemplateData == nil {
		templateFileName := wizardtemplate.GetTemplateFileName()
		return container.NewVBox(
			widget.NewLabel(fmt.Sprintf("Template file bin/%s not found.", templateFileName)),
			widget.NewLabel("Create the template file to enable this tab."),
		)
	}

	state.InitializeTemplateState()

	availableOutbounds := wizardstate.EnsureDefaultAvailableOutbounds(state.GetAvailableOutbounds())

	// Set flag to block callbacks during initialization
	state.UpdatingOutboundOptions = true

	// Initialize CustomRules if needed
	if state.CustomRules == nil {
		state.CustomRules = make([]*wizardstate.SelectableRuleState, 0)
	}

	rulesBox := container.NewVBox()
	if len(state.SelectableRuleStates) == 0 {
		rulesBox.Add(widget.NewLabel("No selectable rules defined in template."))
	} else {
		for i := range state.SelectableRuleStates {
			ruleState := state.SelectableRuleStates[i]
			idx := i

			// Only show outbound selector if rule has "outbound" field
			var outboundSelect *widget.Select
			var outboundRow fyne.CanvasObject
			if ruleState.Rule.HasOutbound {
				wizardstate.EnsureDefaultOutbound(ruleState, availableOutbounds)
				outboundSelect = widget.NewSelect(availableOutbounds, func(value string) {
					// Ignore callback during programmatic update
					if state.UpdatingOutboundOptions {
						return
					}
					state.SelectableRuleStates[idx].SelectedOutbound = value
					state.TemplatePreviewNeedsUpdate = true
				})
				outboundSelect.SetSelected(ruleState.SelectedOutbound)
				if !ruleState.Enabled {
					outboundSelect.Disable()
				}
				outboundRow = container.NewHBox(
					widget.NewLabel("Outbound:"),
					outboundSelect,
				)
			}
			state.SelectableRuleStates[idx].OutboundSelect = outboundSelect

			checkbox := widget.NewCheck(ruleState.Rule.Label, func(val bool) {
				state.SelectableRuleStates[idx].Enabled = val
				state.TemplatePreviewNeedsUpdate = true
				if outboundSelect != nil {
					if val {
						outboundSelect.Enable()
					} else {
						outboundSelect.Disable()
					}
				}
			})
			checkbox.SetChecked(ruleState.Enabled)

			// Create checkbox container with optional info button for description
			checkboxContainer := container.NewHBox(checkbox)
			if ruleState.Rule.Description != "" {
				infoButton := widget.NewButton("?", func() {
					dialog.ShowInformation(ruleState.Rule.Label, ruleState.Rule.Description, state.Window)
				})
				infoButton.Importance = widget.LowImportance
				checkboxContainer.Add(infoButton)
			}

			rowContent := []fyne.CanvasObject{checkboxContainer, layout.NewSpacer()}
			if outboundRow != nil {
				rowContent = append(rowContent, outboundRow)
			}
			rulesBox.Add(container.NewHBox(rowContent...))
		}
	}

	// Display custom rules
	for i := range state.CustomRules {
		customRule := state.CustomRules[i]
		idx := i

		wizardstate.EnsureDefaultOutbound(customRule, availableOutbounds)

		outboundSelect := widget.NewSelect(availableOutbounds, func(value string) {
			if state.UpdatingOutboundOptions {
				return
			}
			state.CustomRules[idx].SelectedOutbound = value
			state.TemplatePreviewNeedsUpdate = true
		})
		outboundSelect.SetSelected(customRule.SelectedOutbound)
		if !customRule.Enabled {
			outboundSelect.Disable()
		}

		// Edit button
		editButton := widget.NewButton("✏️", func() {
			showAddRuleDialog(state, customRule, idx)
		})
		editButton.Importance = widget.LowImportance

		// Delete button
		deleteButton := widget.NewButton("❌", func() {
			// Create copy of index for closure
			deleteIdx := idx
			// Delete rule
			state.CustomRules = append(state.CustomRules[:deleteIdx], state.CustomRules[deleteIdx+1:]...)
			state.TemplatePreviewNeedsUpdate = true
			// Create wrapper for RefreshRulesTab
			refreshWrapper := func(state *wizardstate.WizardState) fyne.CanvasObject {
				return CreateRulesTab(state, showAddRuleDialog)
			}
			state.RefreshRulesTab(refreshWrapper)
		})
		deleteButton.Importance = widget.LowImportance

		checkbox := widget.NewCheck(customRule.Rule.Label, func(val bool) {
			state.CustomRules[idx].Enabled = val
			state.TemplatePreviewNeedsUpdate = true
			if val {
				outboundSelect.Enable()
			} else {
				outboundSelect.Disable()
			}
		})
		checkbox.SetChecked(customRule.Enabled)

		customRule.OutboundSelect = outboundSelect

		rowContent := []fyne.CanvasObject{
			checkbox,
			editButton,
			deleteButton,
			layout.NewSpacer(),
			container.NewHBox(
				widget.NewLabel("Outbound:"),
				outboundSelect,
			),
		}
		rulesBox.Add(container.NewHBox(rowContent...))
	}

	state.EnsureFinalSelected(availableOutbounds)
	finalSelect := widget.NewSelect(availableOutbounds, func(value string) {
		// Ignore callback during programmatic update
		if state.UpdatingOutboundOptions {
			return
		}
		state.SelectedFinalOutbound = value
		state.TemplatePreviewNeedsUpdate = true
	})
	finalSelect.SetSelected(state.SelectedFinalOutbound)
	state.FinalOutboundSelect = finalSelect

	// Add Rule button - add inside rulesBox
	addRuleButton := widget.NewButton("➕ Add Rule", func() {
		showAddRuleDialog(state, nil, -1)
	})
	addRuleButton.Importance = widget.LowImportance
	rulesBox.Add(addRuleButton)

	rulesScroll := CreateRulesScroll(state, rulesBox)

	// Reset flag before refreshOutboundOptions, as it will set it if needed
	state.UpdatingOutboundOptions = false
	state.RefreshOutboundOptions()

	return container.NewVBox(
		widget.NewLabel("Selectable rules"),
		rulesScroll,
		widget.NewSeparator(),
		container.NewHBox(
			widget.NewLabel("Final outbound:"),
			finalSelect,
			layout.NewSpacer(),
		),
	)
}

// CreateRulesScroll creates a scrollable container for rules content.
func CreateRulesScroll(state *wizardstate.WizardState, content fyne.CanvasObject) fyne.CanvasObject {
	maxHeight := state.Window.Canvas().Size().Height * 0.65
	if maxHeight <= 0 {
		maxHeight = 430
	}
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(0, maxHeight))
	return scroll
}

