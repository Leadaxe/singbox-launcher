// File preset_ref_edit_dialog.go — edit dialog для preset-ref правила.
//
// Дизайн совпадает с add_rule_dialog.go: AppTabs (Form / JSON) внутри
// внешнего модального диалога. Tab Form — типизированные var-контролы из
// preset.vars (универсальный rendering по PresetVar.Type). Tab JSON —
// preview эмитнутого config-fragment'а (route rule + dns rule + rule_set +
// bundled dns_servers) read-only.
//
// Кнопки внизу: Convert to user rule(s) (one-way conversion), Cancel, Save.
package tabs

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/build"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// showEditPresetRefDialog — двух-табовый dialog (Form + JSON) для preset-ref правила.
func showEditPresetRefDialog(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	idx int,
	showAddRuleDialog ShowAddRuleDialogFunc,
) {
	if idx < 0 || idx >= len(model.PresetRefs) {
		return
	}
	pr := model.PresetRefs[idx]

	// Lookup template preset by ref.
	var tplPreset *wizardtemplate.Preset
	for i := range model.TemplateData.Presets {
		if model.TemplateData.Presets[i].ID == pr.Ref {
			tplPreset = &model.TemplateData.Presets[i]
			break
		}
	}
	if tplPreset == nil {
		dialog.ShowConfirm(
			locale.T("wizard.dialog_confirmation"),
			fmt.Sprintf("Preset '%s' not found in current template. Delete this rule?", pr.Ref),
			func(ok bool) {
				if !ok {
					return
				}
				model.PresetRefs = append(model.PresetRefs[:idx], model.PresetRefs[idx+1:]...)
				wizardmodels.CompactRuleOrderIndices(model, wizardmodels.SlotKindPresetRef, idx)
				model.TemplatePreviewNeedsUpdate = true
				presenter.MarkAsChanged()
				refreshRulesTabFromPresenter(presenter, showAddRuleDialog)
			},
			guiState.Window,
		)
		return
	}

	// Working copy varsValues — пишется в pr.Vars только на Save.
	working := make(map[string]string, len(pr.Vars))
	for k, v := range pr.Vars {
		working[k] = v
	}

	availableOutbounds := wizardbusiness.EnsureDefaultAvailableOutbounds(
		wizardbusiness.GetAvailableOutbounds(model),
	)

	// ===== Form tab: vars rendering — vertical layout (Label сверху, control под ним),
	// как в обычном add_rule_dialog. Каждая var → 2 widget'а в VBox: label + control.
	// itemRefs хранит ссылку на ВЕСЬ блок (label+control в обёртке) для show/hide через if.
	itemRefs := make(map[string]*fyne.Container, len(tplPreset.Vars))
	formItems := make([]fyne.CanvasObject, 0, len(tplPreset.Vars)*3)

	isVarVisible := func(v *wizardtemplate.PresetVar) bool {
		for _, ref := range v.If {
			if !strings.EqualFold(getVarOrDefault(working, ref, tplPreset), "true") {
				return false
			}
		}
		if len(v.IfOr) > 0 {
			any := false
			for _, ref := range v.IfOr {
				if strings.EqualFold(getVarOrDefault(working, ref, tplPreset), "true") {
					any = true
					break
				}
			}
			if !any {
				return false
			}
		}
		return true
	}

	// JSON preview — обычный читаемый цвет (RichText), не disabled-grey.
	// Возможность копировать-выделять текст сохраняется. Изменять content юзер
	// не может — RichText по природе display-only. Visual indicator "read-only"
	// — иконка 🔒 справа сверху от scroll (см. ниже).
	jsonRichText := widget.NewRichTextWithText("")
	jsonRichText.Wrapping = fyne.TextWrapWord

	refreshJSON := func() {
		jsonRichText.ParseMarkdown("```json\n" + buildPresetJSONPreview(tplPreset, working) + "\n```")
	}

	var refreshVisibility func()
	refreshVisibility = func() {
		for i := range tplPreset.Vars {
			v := &tplPreset.Vars[i]
			block, ok := itemRefs[v.Name]
			if !ok {
				continue
			}
			if isVarVisible(v) {
				block.Show()
			} else {
				block.Hide()
			}
		}
		refreshJSON()
	}

	for i := range tplPreset.Vars {
		v := &tplPreset.Vars[i]
		title := v.Title
		if title == "" {
			title = v.Name
		}
		curVal := getVarOrDefault(working, v.Name, tplPreset)

		var wid fyne.CanvasObject
		// isBool — флаг для специального layout (checkbox слева от label, в одну линию).
		isBool := false
		switch v.Type {
		case "bool":
			// Checkbox имеет встроенный label справа от себя (стандартный паттерн UI).
			// Не оборачиваем в "Label сверху + control под ним" как для других типов.
			ch := widget.NewCheck(title, func(on bool) {
				if on {
					working[v.Name] = "true"
				} else {
					working[v.Name] = "false"
				}
				refreshVisibility()
			})
			ch.SetChecked(strings.EqualFold(curVal, "true"))
			wid = ch
			isBool = true
		case "enum":
			enum, _, _ := v.DecodeOptions()
			titles := make([]string, 0, len(enum))
			titleToValue := make(map[string]string, len(enum))
			selectedTitle := ""
			for _, e := range enum {
				titles = append(titles, e.Title)
				titleToValue[e.Title] = e.Value
				if e.Value == curVal {
					selectedTitle = e.Title
				}
			}
			sel := widget.NewSelect(titles, func(picked string) {
				if val, ok := titleToValue[picked]; ok {
					working[v.Name] = val
					refreshJSON()
				}
			})
			if selectedTitle != "" {
				sel.SetSelected(selectedTitle)
			}
			wid = sel
		case "outbound":
			_, tags, _ := v.DecodeOptions()
			var opts []string
			if tags != nil {
				opts = append(opts, tags...)
			} else {
				opts = append(opts, availableOutbounds...)
			}
			sel := widget.NewSelect(opts, func(picked string) {
				working[v.Name] = picked
				refreshJSON()
			})
			if curVal != "" {
				sel.SetSelected(curVal)
			} else if len(opts) > 0 {
				sel.SetSelected(opts[0])
			}
			wid = sel
		case "dns_server":
			tags := resolveDNSServerOptions(v, tplPreset, model)
			sel := widget.NewSelect(tags, func(picked string) {
				working[v.Name] = picked
				refreshJSON()
			})
			if curVal != "" {
				sel.SetSelected(curVal)
			} else if len(tags) > 0 {
				sel.SetSelected(tags[0])
			}
			wid = sel
		case "number":
			entry := widget.NewEntry()
			entry.SetText(curVal)
			entry.OnChanged = func(s string) {
				working[v.Name] = s
				refreshJSON()
			}
			entry.Validator = func(s string) error {
				if s == "" {
					return nil
				}
				if _, err := strconv.Atoi(s); err != nil {
					return fmt.Errorf("must be a number")
				}
				return nil
			}
			wid = entry
		default: // text
			entry := widget.NewEntry()
			entry.SetText(curVal)
			entry.OnChanged = func(s string) {
				working[v.Name] = s
				refreshJSON()
			}
			wid = entry
		}

		// Layout:
		//   - bool: [☑] Label  (checkbox со встроенным label справа от себя, одна строка)
		//   - остальные: "Label:" сверху, control под ним (стиль add_rule_dialog).
		// Tooltip — отдельный label под control'ом (мелкий) для обоих вариантов.
		var blockChildren []fyne.CanvasObject
		if isBool {
			blockChildren = []fyne.CanvasObject{wid}
		} else {
			labelWidget := widget.NewLabel(title + ":")
			blockChildren = []fyne.CanvasObject{labelWidget, wid}
		}
		if v.Tooltip != "" {
			// Tooltip — italic для visual differentiation; цвет обычный
			// (LowImportance даёт серый на сером на тёмной теме → нечитаемо).
			hint := widget.NewLabelWithStyle(v.Tooltip, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
			hint.Wrapping = fyne.TextWrapWord
			blockChildren = append(blockChildren, hint)
		}
		block := container.NewVBox(blockChildren...)
		// Spacer-separator между var'ами для визуального дыхания
		formItems = append(formItems, block, widget.NewSeparator())
		itemRefs[v.Name] = block
	}

	formInner := container.NewVBox(formItems...)
	formContent := container.NewVBox(formInner)

	// ===== JSON tab: preview эмитнутого fragment'а =====
	refreshJSON()

	// Lock icon справа сверху от content area — visual indicator что preview
	// read-only (не серый-disabled, текст обычного цвета).
	jsonLockIcon := widget.NewLabel("🔒")
	jsonScroll := container.NewScroll(jsonRichText)
	jsonContent := container.NewBorder(
		container.NewHBox(layout.NewSpacer(), jsonLockIcon),
		nil, nil, nil,
		jsonScroll,
	)

	// ===== AppTabs (Form / JSON) =====
	formScroll := container.NewScroll(formContent)
	formTabItem := container.NewTabItem(locale.T("wizard.add_rule.tab_form"), formScroll)
	jsonTabItem := container.NewTabItem(locale.T("wizard.add_rule.tab_raw"), jsonContent)
	tabs := container.NewAppTabs(formTabItem, jsonTabItem)
	tabs.OnSelected = func(ti *container.TabItem) {
		if ti == jsonTabItem {
			refreshJSON()
		} else {
			refreshVisibility()
		}
	}
	// Initial visibility apply.
	refreshVisibility()

	// ===== Top: имя preset'а как заголовок + описание серым tooltip-стилем =====
	// Имя preset'а — фиксированное (template-side), редактирование невозможно,
	// поэтому показываем просто bold-текст заголовка (без "Rule Name:" + Entry).
	titleLabel := widget.NewLabelWithStyle(
		tplPreset.Label,
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
	)

	topChildren := []fyne.CanvasObject{titleLabel}
	if desc := strings.TrimSpace(tplPreset.Description); desc != "" {
		// Description под заголовком — обычный читаемый цвет (без LowImportance —
		// на тёмной теме оно становится серым на сером, нечитаемо).
		// Italic — visual differentiation от title, не цветом.
		descLabel := widget.NewLabelWithStyle(desc, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
		descLabel.Wrapping = fyne.TextWrapWord
		topChildren = append(topChildren, descLabel)
	}
	topBlock := container.NewVBox(topChildren...)

	// ===== Buttons =====
	cancelButton := widget.NewButton(locale.T("wizard.dialog_cancel"), nil)

	saveButton := widget.NewButton(locale.T("wizard.shared.button_save"), nil)
	saveButton.Importance = widget.HighImportance

	convertButton := widget.NewButton(locale.T("wizard.rules.button_convert_to_user"), nil)
	convertButton.Importance = widget.LowImportance

	buttons := container.NewHBox(
		convertButton,
		layout.NewSpacer(),
		cancelButton,
		saveButton,
	)

	// Border: top = (Rule Name + Description), bottom = buttons, center = tabs (full height).
	// Полностью повторяет layout обычного add_rule_dialog для UI consistency.
	dialogContent := container.NewBorder(topBlock, buttons, nil, nil, tabs)

	// Открываем отдельным окном через Application.NewWindow — identично обычному
	// add_rule_dialog (resize 500×640, центр экрана, ESC закрывает, Close intercept'ится).
	controller := presenter.Controller()
	if controller == nil || controller.UIService == nil {
		return
	}
	editWindow := controller.UIService.Application.NewWindow(locale.T("wizard.add_rule.title_edit"))

	cancelButton.OnTapped = func() {
		editWindow.Close()
	}
	saveButton.OnTapped = func() {
		newVars := make(map[string]string, len(working))
		for _, v := range tplPreset.Vars {
			val := working[v.Name]
			if val != "" && val != v.Default {
				newVars[v.Name] = val
			}
		}
		pr.Vars = newVars
		model.TemplatePreviewNeedsUpdate = true
		presenter.MarkAsChanged()
		editWindow.Close()
		refreshRulesTabFromPresenter(presenter, showAddRuleDialog)
	}
	convertButton.OnTapped = func() {
		mergedVars := make(map[string]string, len(working))
		for _, v := range tplPreset.Vars {
			val := working[v.Name]
			if val != "" {
				mergedVars[v.Name] = val
			} else {
				mergedVars[v.Name] = v.Default
			}
		}
		dialog.ShowConfirm(
			locale.T("wizard.dialog_confirmation"),
			fmt.Sprintf("Convert '%s' to user-defined rule(s)? You will lose the link to the template — future template updates won't apply.", tplPreset.Label),
			func(ok bool) {
				if !ok {
					return
				}
				converted := convertPresetRefToUserRules(model, tplPreset, mergedVars, pr.Enabled)
				if converted == 0 {
					return
				}
				model.PresetRefs = append(model.PresetRefs[:idx], model.PresetRefs[idx+1:]...)
				wizardmodels.CompactRuleOrderIndices(model, wizardmodels.SlotKindPresetRef, idx)
				model.TemplatePreviewNeedsUpdate = true
				presenter.MarkAsChanged()
				editWindow.Close()
				refreshRulesTabFromPresenter(presenter, showAddRuleDialog)
			},
			editWindow,
		)
	}

	editWindow.Resize(fyne.NewSize(500, 640))
	editWindow.CenterOnScreen()
	editWindow.SetContent(dialogContent)
	editWindow.SetCloseIntercept(func() {
		editWindow.Close()
	})
	editWindow.Show()
}

// buildPresetJSONPreview — генерит read-only JSON preview эмитнутых fragment'ов
// для текущих working varsValues. Показывается во вкладке JSON.
func buildPresetJSONPreview(tpl *wizardtemplate.Preset, working map[string]string) string {
	// Build effective varsMap (working + defaults).
	vars := make(map[string]string, len(tpl.Vars))
	for _, v := range tpl.Vars {
		if val, ok := working[v.Name]; ok && val != "" {
			vars[v.Name] = val
		} else {
			vars[v.Name] = v.Default
		}
	}
	frags, warns, ok := build.ExpandPreset(tpl, vars)
	if !ok {
		return "// preset expansion failed:\n// " + warningsAsText(warns)
	}
	preview := map[string]interface{}{}
	if len(frags.RuleSets) > 0 {
		preview["rule_set"] = frags.RuleSets
	}
	if frags.RoutingRule != nil {
		preview["rule"] = frags.RoutingRule
	}
	if frags.DNSRule != nil {
		preview["dns_rule"] = frags.DNSRule
	}
	if len(frags.DNSServers) > 0 {
		preview["dns_servers"] = frags.DNSServers
	}
	out, err := json.MarshalIndent(preview, "", "  ")
	if err != nil {
		return "// marshal error: " + err.Error()
	}
	text := string(out)
	if len(warns) > 0 {
		text = "// warnings:\n// " + warningsAsText(warns) + "\n\n" + text
	}
	return text
}

func warningsAsText(warns []build.ExpandWarning) string {
	parts := make([]string, 0, len(warns))
	for _, w := range warns {
		parts = append(parts, w.String())
	}
	return strings.Join(parts, "\n// ")
}

// getVarOrDefault — текущее значение или template default.
func getVarOrDefault(working map[string]string, name string, tpl *wizardtemplate.Preset) string {
	if v, ok := working[name]; ok && v != "" {
		return v
	}
	for _, vv := range tpl.Vars {
		if vv.Name == name {
			return vv.Default
		}
	}
	return ""
}

// resolveDNSServerOptions — список доступных DNS-серверов для picker'а.
// Учитывает explicit options whitelist, select="local"/"global" shortcut.
func resolveDNSServerOptions(v *wizardtemplate.PresetVar, tpl *wizardtemplate.Preset, model *wizardmodels.WizardModel) []string {
	if _, tags, ok := v.DecodeOptions(); ok && tags != nil {
		return append([]string(nil), tags...)
	}

	var out []string

	if v.Select == "local" {
		for _, ds := range tpl.DNSServers {
			out = append(out, ds.Tag)
		}
		sort.Strings(out)
		return out
	}

	// "global" or omit → bundled ∪ template defaults.
	for _, ds := range tpl.DNSServers {
		out = append(out, ds.Tag)
	}
	for _, raw := range model.DNSServers {
		var srv map[string]interface{}
		if err := json.Unmarshal(raw, &srv); err == nil {
			if tag, ok := srv["tag"].(string); ok && tag != "" {
				out = append(out, tag)
			}
		}
	}
	seen := make(map[string]bool, len(out))
	dedup := make([]string, 0, len(out))
	for _, t := range out {
		if !seen[t] {
			seen[t] = true
			dedup = append(dedup, t)
		}
	}
	sort.Strings(dedup)
	return dedup
}
