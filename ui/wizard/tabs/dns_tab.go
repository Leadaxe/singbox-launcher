package tabs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

func setTooltip(o fyne.CanvasObject, text string) {
	if text == "" || o == nil {
		return
	}
	if tb, ok := interface{}(o).(interface{ SetToolTip(string) }); ok {
		tb.SetToolTip(text)
	}
}

func tooltipForDNSServerCheck(locked bool) string {
	if locked {
		return "wizard.dns.tooltip_server_locked"
	}
	return "wizard.dns.tooltip_server_enabled"
}

func newTooltipLabel(text, tip string) *ttwidget.Label {
	l := ttwidget.NewLabel(text)
	if strings.TrimSpace(tip) != "" {
		l.SetToolTip(tip)
	}
	return l
}

// CreateDNSTab builds the DNS tab: servers list, strategy + cache, rules, then final + default resolver on one row.
func CreateDNSTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	guiState := presenter.GUIState()
	mod := presenter.Model()
	td := mod.TemplateData
	dialogParent := func() fyne.Window {
		if w := presenter.DialogParent(); w != nil {
			return w
		}
		return guiState.Window
	}
	serversBox := container.NewVBox()

	refreshList := func() {
		serversBox.Objects = serversBox.Objects[:0]
		m := presenter.Model()
		if len(m.DNSServers) == 0 {
			g := canvas.NewRectangle(color.Transparent)
			g.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
			serversBox.Add(container.NewHBox(
				widget.NewLabel(locale.T("wizard.dns.no_servers")),
				layout.NewSpacer(),
				g,
			))
			serversBox.Refresh()
			return
		}
		for i := range m.DNSServers {
			func(idx int) {
				var row *fynewidget.HoverRow
				rowGetter := func() *fynewidget.HoverRow { return row }

				raw := m.DNSServers[idx]
				var obj map[string]interface{}
				if err := json.Unmarshal(raw, &obj); err != nil {
					obj = nil
				}
				sum := dnsServerSummaryFromObj(obj)
				if obj == nil && len(raw) > 0 {
					sum = dnsServerSummaryFromInvalidRaw(raw)
				}
				tag := ""
				if obj != nil {
					tag = dnsJSONStringField(obj, "tag")
				}
				desc := ""
				if obj != nil {
					desc = strings.TrimSpace(dnsJSONStringField(obj, "description"))
				}
				locked := wizardbusiness.DNSTagLocked(m, tag)

				// Не вызывать SyncModelToGUI здесь — он пересобирает весь список и все вкладки; только обновить селекты.
				sumLabel := ttwidget.NewLabel(sum)
				sumLabel.Wrapping = fyne.TextTruncate
				cwc := fynewidget.NewCheckWithContent(func(checked bool) {
					setDNSServerEnabledAt(presenter, idx, checked)
					presenter.RefreshDNSDependentSelectsOnly()
				}, sumLabel, fynewidget.CheckWithContentConfig{ContentToolTip: desc})
				enCheck := cwc.Check
				enCheck.SetChecked(wizardbusiness.DNSServerWizardEnabledRaw(raw))
				if locked {
					// Скелет config.dns: строка зафиксирована; галочка только показывает включение в конфиг (из state/шаблона), без переключения в UI.
					enCheck.Disable()
				}
				setTooltip(enCheck, locale.T(tooltipForDNSServerCheck(locked)))

				editBtn := fynewidget.NewHoverForwardButtonWithIcon(locale.T("wizard.shared.button_edit"), theme.DocumentCreateIcon(), func() {
					showDNSServerEditor(presenter, dialogParent(), idx)
				}, rowGetter)
				editBtn.Importance = widget.LowImportance
				delBtn := fynewidget.NewHoverForwardButtonWithIcon(locale.T("wizard.shared.button_del"), theme.DeleteIcon(), func() {
					deleteDNSServerAt(presenter, idx)
					presenter.RefreshDNSListAndSelects()
				}, rowGetter)
				delBtn.Importance = widget.LowImportance
				if locked {
					editBtn.Disable()
					delBtn.Disable()
					hint := locale.T("wizard.dns.tooltip_server_locked")
					setTooltip(editBtn, hint)
					setTooltip(delBtn, hint)
				}

				rowGutter := canvas.NewRectangle(color.Transparent)
				rowGutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
				right := container.NewHBox(editBtn, delBtn, rowGutter)
				// Border: check left, content center (tap/hover → check via fynewidget), buttons right — avoids zero-width label in HBox-only row.
				rowInner := container.NewBorder(nil, nil, cwc.CheckLeading, right, cwc.Content)
				row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{})
				row.WireTooltipLabelHover(sumLabel)
				serversBox.Add(row)
			}(i)
		}
		serversBox.Refresh()
	}
	guiState.RefreshDNSList = refreshList

	addBtn := widget.NewButton(locale.T("wizard.dns.button_add"), func() {
		showDNSServerAddDialog(presenter, dialogParent())
	})

	serversScroll := container.NewVScroll(serversBox)
	serversScroll.SetMinSize(fyne.NewSize(0, 210)) // 1.5× former 140

	serversLabel := widget.NewLabel(locale.T("wizard.dns.label_servers"))
	serversLabel.Importance = widget.MediumImportance
	serversHeader := container.NewHBox(serversLabel, layout.NewSpacer(), addBtn)

	guiState.DNSFinalSelect = widget.NewSelect([]string{}, func(sel string) {
		if guiState.DNSSelectsProgrammatic {
			return
		}
		mod := presenter.Model()
		if mod.DNSFinal != sel {
			mod.DNSFinal = sel
			mod.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
		}
	})
	var templateVars []wizardtemplate.TemplateVar
	if td != nil {
		templateVars = td.Vars
	}
	varTitle := func(name, fallback string) string {
		vd, ok := wizardtemplate.VarByName(templateVars, name)
		if !ok {
			return fallback
		}
		s := strings.TrimSpace(wizardtemplate.VarDisplayTitle(vd))
		if s == "" {
			return fallback
		}
		return s
	}
	varTooltip := func(name string) string {
		vd, ok := wizardtemplate.VarByName(templateVars, name)
		if !ok {
			return ""
		}
		return strings.TrimSpace(wizardtemplate.VarDisplayTooltip(vd))
	}
	finalTip := varTooltip(wizardmodels.VarDNSFinal)
	finalLabel := newTooltipLabel(varTitle(wizardmodels.VarDNSFinal, locale.T("wizard.dns.label_final")), finalTip)
	setTooltip(guiState.DNSFinalSelect, varTooltip(wizardmodels.VarDNSFinal))

	markResolverChanged := func(value string) {
		v := strings.TrimSpace(value)
		if v == "" {
			if mod.DefaultDomainResolver != "" || !mod.DefaultDomainResolverUnset {
				mod.DefaultDomainResolver = ""
				mod.DefaultDomainResolverUnset = true
				mod.TemplatePreviewNeedsUpdate = true
				presenter.MarkAsChanged()
			}
			return
		}
		if mod.DefaultDomainResolver != v || mod.DefaultDomainResolverUnset {
			mod.DefaultDomainResolver = v
			mod.DefaultDomainResolverUnset = false
			mod.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
		}
	}
	markStrategyChanged := func(value string) {
		v := strings.TrimSpace(value)
		if mod.DNSStrategy != v {
			mod.DNSStrategy = v
			mod.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
		}
	}

	guiState.DNSDefaultResolverSelect = widget.NewSelect([]string{}, func(sel string) {
		if guiState.DNSSelectsProgrammatic {
			return
		}
		markResolverChanged(sel)
	})
	resTip := varTooltip(wizardmodels.VarDNSDefaultDomainResolver)
	resLabel := newTooltipLabel(varTitle(wizardmodels.VarDNSDefaultDomainResolver, locale.T("wizard.dns.label_default_resolver")), resTip)
	setTooltip(guiState.DNSDefaultResolverSelect, varTooltip(wizardmodels.VarDNSDefaultDomainResolver))

	guiState.DNSRulesEntry = widget.NewMultiLineEntry()
	guiState.DNSRulesEntry.SetPlaceHolder(locale.T("wizard.dns.placeholder_rules"))
	guiState.DNSRulesEntry.Wrapping = fyne.TextWrapOff
	guiState.DNSRulesEntry.OnChanged = func(string) {
		if guiState.DNSRulesProgrammatic {
			return
		}
		presenter.Model().TemplatePreviewNeedsUpdate = true
		presenter.MarkAsChanged()
	}
	rulesScroll := container.NewScroll(guiState.DNSRulesEntry)
	rulesScroll.Direction = container.ScrollBoth
	rulesHeight := canvas.NewRectangle(color.Transparent)
		rulesHeight.SetMinSize(fyne.NewSize(0, 170)) // was 120; +50 px for rules JSON area
	rulesBlock := container.NewMax(rulesHeight, rulesScroll)

	rulesLabel := widget.NewLabel(locale.T("wizard.dns.label_rules"))
	rulesLabel.Importance = widget.MediumImportance

	strategyTip := varTooltip(wizardmodels.VarDNSStrategy)
	strategyLabel := newTooltipLabel(varTitle(wizardmodels.VarDNSStrategy, locale.T("wizard.dns.label_strategy")), strategyTip)

	guiState.DNSStrategySelect = widget.NewSelect([]string{}, func(sel string) {
		if guiState.DNSSelectsProgrammatic {
			return
		}
		markStrategyChanged(sel)
	})
	setTooltip(guiState.DNSStrategySelect, varTooltip(wizardmodels.VarDNSStrategy))

	cacheLabelText := varTitle(wizardmodels.VarDNSIndependentCache, locale.T("wizard.dns.label_independent_cache"))
	cacheTip := varTooltip(wizardmodels.VarDNSIndependentCache)
	cacheTitleLbl := ttwidget.NewLabel(cacheLabelText)
	indepCacheCWC := fynewidget.NewCheckWithContent(func(checked bool) {
		if guiState.DNSSelectsProgrammatic {
			return
		}
		cur := false
		if mod.DNSIndependentCache != nil {
			cur = *mod.DNSIndependentCache
		}
		if cur != checked {
			nv := checked
			mod.DNSIndependentCache = &nv
			mod.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
		}
	}, cacheTitleLbl, fynewidget.CheckWithContentConfig{ContentToolTip: cacheTip})
	guiState.DNSIndependentCacheCheck = indepCacheCWC.Check

	strategyAndCacheRow := container.NewHBox(
		strategyLabel,
		guiState.DNSStrategySelect,
		layout.NewSpacer(),
		indepCacheCWC.CheckLeading,
		indepCacheCWC.Content,
	)

	// Final и default_domain_resolver — одна строка: две группы (лейбл+селект), spacer между ними.
	// Плоский HBox с одним Spacer между четырьмя виджетами даёт селектам нулевую ширину в Fyne.
	finalGroup := container.NewHBox(finalLabel, guiState.DNSFinalSelect)
	resolverGroup := container.NewHBox(resLabel, guiState.DNSDefaultResolverSelect)
	finalAndResolverRow := container.NewHBox(finalGroup, layout.NewSpacer(), resolverGroup)

	refreshList()

	return container.NewVBox(
		serversHeader,
		serversScroll,
		widget.NewSeparator(),
		strategyAndCacheRow,
		widget.NewSeparator(),
		rulesLabel,
		rulesBlock,
		widget.NewSeparator(),
		finalAndResolverRow,
	)
}

func dnsServerSummaryFromInvalidRaw(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return locale.T("wizard.dns.invalid_server")
	}
	const max = 64
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max]) + "…"
	}
	return s
}

func dnsServerSummaryFromObj(obj map[string]interface{}) string {
	if obj == nil {
		return locale.T("wizard.dns.invalid_server")
	}
	tag := dnsJSONStringField(obj, "tag")
	typ := dnsJSONStringField(obj, "type")
	server := dnsJSONStringField(obj, "server")
	if tag == "" {
		tag = locale.T("wizard.dns.no_tag")
	}
	var sum string
	if server != "" {
		sum = fmt.Sprintf("%s  ·  %s  ·  %s", tag, typ, server)
	} else {
		sum = fmt.Sprintf("%s  ·  %s", tag, typ)
	}
	if det := strings.TrimSpace(dnsJSONStringField(obj, "detour")); det != "" {
		sum += " [" + det + "]"
	}
	return sum
}

// dnsJSONStringField reads a string-like value from unmarshaled JSON (tag/type/server are strings in sing-box).

func setDNSServerEnabledAt(p *wizardpresentation.WizardPresenter, index int, enabled bool) {
	mod := p.Model()
	if index < 0 || index >= len(mod.DNSServers) {
		return
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(mod.DNSServers[index], &obj); err != nil {
		return
	}
	obj["enabled"] = enabled
	b, err := json.Marshal(obj)
	if err != nil {
		return
	}
	mod.DNSServers[index] = json.RawMessage(b)
	mod.TemplatePreviewNeedsUpdate = true
	p.MarkAsChanged()
}

func dnsJSONStringField(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case fmt.Stringer:
		return strings.TrimSpace(t.String())
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func uniqueDNSTag(presenter *wizardpresentation.WizardPresenter) string {
	used := make(map[string]struct{})
	for _, raw := range presenter.Model().DNSServers {
		var o map[string]interface{}
		if json.Unmarshal(raw, &o) == nil {
			if t, ok := o["tag"].(string); ok {
				used[strings.TrimSpace(t)] = struct{}{}
			}
		}
	}
	for n := 1; n < 1000; n++ {
		candidate := fmt.Sprintf("dns_%d", n)
		if _, ok := used[candidate]; !ok {
			return candidate
		}
	}
	return "dns_new"
}

func deleteDNSServerAt(p *wizardpresentation.WizardPresenter, index int) {
	m := p.Model()
	if index < 0 || index >= len(m.DNSServers) {
		return
	}
	var deleted map[string]interface{}
	_ = json.Unmarshal(m.DNSServers[index], &deleted)
	if wizardbusiness.DNSTagLocked(m, dnsJSONStringField(deleted, "tag")) {
		return
	}
	delTag, _ := deleted["tag"].(string)
	m.DNSServers = append(m.DNSServers[:index], m.DNSServers[index+1:]...)

	tags := wizardbusiness.DNSEnabledTagOptions(m)
	if delTag == m.DNSFinal && len(tags) > 0 {
		m.DNSFinal = tags[0]
	} else if len(tags) == 0 {
		m.DNSFinal = ""
	}
	if delTag == m.DefaultDomainResolver {
		m.DefaultDomainResolver = ""
		m.DefaultDomainResolverUnset = true
	}
	m.TemplatePreviewNeedsUpdate = true
	p.MarkAsChanged()
}

// applyDNSServerJSON parses JSON, validates tag and uniqueness, then replaces editIndex or appends (editIndex < 0).
// dnsServerDialogEntryMinHeight is the minimum height for the JSON editor in Add/Edit DNS server dialogs.
const dnsServerDialogEntryMinHeight = 240

func dnsServerDialogJSONArea(entry *widget.Entry) fyne.CanvasObject {
	scroll := container.NewScroll(entry)
	scroll.Direction = container.ScrollBoth
	minH := canvas.NewRectangle(color.Transparent)
	minH.SetMinSize(fyne.NewSize(0, dnsServerDialogEntryMinHeight))
	return container.NewMax(minH, scroll)
}

func applyDNSServerJSON(p *wizardpresentation.WizardPresenter, w fyne.Window, text string, editIndex int) bool {
	if w == nil {
		w = p.DialogParent()
	}
	text = strings.TrimSpace(text)
	if text == "" {
		dialog.ShowError(fmt.Errorf("%s", locale.T("wizard.dns.error_empty_json")), w)
		return false
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.dns.error_invalid_json"), err), w)
		return false
	}
	tag := dnsJSONStringField(obj, "tag")
	if tag == "" {
		dialog.ShowError(fmt.Errorf("%s", locale.T("wizard.dns.error_missing_tag")), w)
		return false
	}
	mod := p.Model()
	if editIndex >= 0 && editIndex < len(mod.DNSServers) {
		var cur map[string]interface{}
		_ = json.Unmarshal(mod.DNSServers[editIndex], &cur)
		if wizardbusiness.DNSTagLocked(mod, dnsJSONStringField(cur, "tag")) {
			dialog.ShowError(fmt.Errorf("%s", locale.T("wizard.dns.error_locked_edit")), w)
			return false
		}
	}
	for i, raw := range mod.DNSServers {
		if editIndex >= 0 && i == editIndex {
			continue
		}
		var o map[string]interface{}
		if json.Unmarshal(raw, &o) != nil {
			continue
		}
		if dnsJSONStringField(o, "tag") == tag {
			dialog.ShowError(fmt.Errorf("%s: %s", locale.T("wizard.dns.error_dup_tag"), tag), w)
			return false
		}
	}
	compact, err := json.Marshal(obj)
	if err != nil {
		dialog.ShowError(err, w)
		return false
	}
	if editIndex >= 0 {
		mod.DNSServers[editIndex] = json.RawMessage(compact)
	} else {
		mod.DNSServers = append(mod.DNSServers, json.RawMessage(compact))
	}
	mod.TemplatePreviewNeedsUpdate = true
	p.MarkAsChanged()
	p.RefreshDNSListAndSelects()
	return true
}

func showDNSServerAddDialog(p *wizardpresentation.WizardPresenter, w fyne.Window) {
	if w == nil {
		w = p.DialogParent()
	}
	if w == nil {
		return
	}
	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapOff
	tag := uniqueDNSTag(p)
	stub := map[string]interface{}{
		"type":        "udp",
		"tag":         tag,
		"server":      "1.1.1.1",
		"server_port": 53,
		"enabled":     true,
	}
	if b, err := json.MarshalIndent(stub, "", "  "); err == nil {
		entry.SetText(string(b))
	}

	var dlg dialog.Dialog
	save := widget.NewButton(locale.T("wizard.dns.dialog_save"), func() {
		if applyDNSServerJSON(p, w, entry.Text, -1) && dlg != nil {
			dlg.Hide()
		}
	})
	cancel := widget.NewButton(locale.T("wizard.dns.dialog_cancel"), func() {
		if dlg != nil {
			dlg.Hide()
		}
	})

	main := container.NewVBox(
		widget.NewLabel(locale.T("wizard.dns.dialog_add_hint")),
		dnsServerDialogJSONArea(entry),
	)
	buttons := container.NewHBox(layout.NewSpacer(), cancel, save)
	dlg = dialogs.NewCustom(locale.T("wizard.dns.dialog_add_title"), main, buttons, "", w)
	dlg.Resize(fyne.NewSize(520, 520))
	dlg.Show()
}

func showDNSServerEditor(p *wizardpresentation.WizardPresenter, w fyne.Window, index int) {
	if w == nil {
		w = p.DialogParent()
	}
	if w == nil {
		return
	}
	m := p.Model()
	if index < 0 || index >= len(m.DNSServers) {
		return
	}
	var cur map[string]interface{}
	_ = json.Unmarshal(m.DNSServers[index], &cur)
	if wizardbusiness.DNSTagLocked(m, dnsJSONStringField(cur, "tag")) {
		return
	}
	entry := widget.NewMultiLineEntry()
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, m.DNSServers[index], "", "  "); err != nil {
		entry.SetText(string(m.DNSServers[index]))
	} else {
		entry.SetText(pretty.String())
	}
	entry.Wrapping = fyne.TextWrapOff

	var dlg dialog.Dialog
	save := widget.NewButton(locale.T("wizard.dns.dialog_save"), func() {
		if applyDNSServerJSON(p, w, entry.Text, index) && dlg != nil {
			dlg.Hide()
		}
	})
	cancel := widget.NewButton(locale.T("wizard.dns.dialog_cancel"), func() {
		if dlg != nil {
			dlg.Hide()
		}
	})

	main := container.NewVBox(
		widget.NewLabel(locale.T("wizard.dns.dialog_hint")),
		dnsServerDialogJSONArea(entry),
	)
	buttons := container.NewHBox(layout.NewSpacer(), cancel, save)
	dlg = dialogs.NewCustom(locale.T("wizard.dns.dialog_title"), main, buttons, "", w)
	dlg.Resize(fyne.NewSize(520, 520))
	dlg.Show()
}
