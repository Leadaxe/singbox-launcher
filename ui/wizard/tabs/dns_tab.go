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
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
)

func setTooltip(o fyne.CanvasObject, text string) {
	if text == "" || o == nil {
		return
	}
	if tb, ok := interface{}(o).(interface{ SetToolTip(string) }); ok {
		tb.SetToolTip(text)
	}
}

const dnsIndependentCacheDocURL = "https://sing-box.sagernet.org/configuration/dns/#independent_cache"

func tooltipForDNSServerCheck(locked bool) string {
	if locked {
		return "wizard.dns.tooltip_server_locked"
	}
	return "wizard.dns.tooltip_server_enabled"
}

// CreateDNSTab builds the DNS tab: servers list, strategy + cache, rules, then final + default resolver on one row.
func CreateDNSTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	guiState := presenter.GUIState()
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
				// ttwidget.Label — SetToolTip; обычный widget.Label тултип не показывает.
				label := ttwidget.NewLabel(sum)
				label.Wrapping = fyne.TextTruncate
				if desc != "" {
					label.SetToolTip(desc)
				}

				locked := wizardbusiness.DNSTagLocked(m, tag)

				// Не вызывать SyncModelToGUI здесь — он пересобирает весь список и все вкладки; только обновить селекты.
				enCheck := widget.NewCheck("", func(checked bool) {
					setDNSServerEnabledAt(presenter, idx, checked)
					presenter.RefreshDNSDependentSelectsOnly()
				})
				enCheck.SetChecked(wizardbusiness.DNSServerWizardEnabledRaw(raw))
				if locked {
					// Скелет config.dns: строка зафиксирована; галочка только показывает включение в конфиг (из state/шаблона), без переключения в UI.
					enCheck.Disable()
				}
				setTooltip(enCheck, locale.T(tooltipForDNSServerCheck(locked)))

				editBtn := widget.NewButtonWithIcon(locale.T("wizard.shared.button_edit"), theme.DocumentCreateIcon(), func() {
					showDNSServerEditor(presenter, dialogParent(), idx)
				})
				editBtn.Importance = widget.LowImportance
				delBtn := widget.NewButtonWithIcon(locale.T("wizard.shared.button_del"), theme.DeleteIcon(), func() {
					deleteDNSServerAt(presenter, idx)
					presenter.RefreshDNSListAndSelects()
				})
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
				// Border: check left, label center (gets width for TextTruncate), buttons right — avoids zero-width label in HBox-only row.
				row := container.NewBorder(nil, nil, enCheck, right, label)
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
	finalLabel := widget.NewLabel(locale.T("wizard.dns.label_final"))

	guiState.DNSDefaultResolverSelect = widget.NewSelect([]string{}, func(sel string) {
		if guiState.DNSSelectsProgrammatic {
			return
		}
		mod := presenter.Model()
		notSet := locale.T("wizard.dns.resolver_not_set")
		if sel == notSet {
			if mod.DefaultDomainResolver != "" || !mod.DefaultDomainResolverUnset {
				mod.DefaultDomainResolver = ""
				mod.DefaultDomainResolverUnset = true
				mod.TemplatePreviewNeedsUpdate = true
				presenter.MarkAsChanged()
			}
			return
		}
		if mod.DefaultDomainResolver != sel || mod.DefaultDomainResolverUnset {
			mod.DefaultDomainResolver = sel
			mod.DefaultDomainResolverUnset = false
			mod.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
		}
	})
	resLabel := widget.NewLabel(locale.T("wizard.dns.label_default_resolver"))

	guiState.DNSRulesEntry = widget.NewMultiLineEntry()
	guiState.DNSRulesEntry.SetPlaceHolder(locale.T("wizard.dns.placeholder_rules"))
	guiState.DNSRulesEntry.Wrapping = fyne.TextWrapOff
	guiState.DNSRulesEntry.OnChanged = func(string) {
		presenter.Model().TemplatePreviewNeedsUpdate = true
		presenter.MarkAsChanged()
	}
	rulesScroll := container.NewScroll(guiState.DNSRulesEntry)
	rulesScroll.Direction = container.ScrollBoth
	rulesHeight := canvas.NewRectangle(color.Transparent)
	rulesHeight.SetMinSize(fyne.NewSize(0, 120))
	rulesBlock := container.NewMax(rulesHeight, rulesScroll)

	rulesLabel := widget.NewLabel(locale.T("wizard.dns.label_rules"))
	rulesLabel.Importance = widget.MediumImportance

	guiState.DNSStrategySelect = widget.NewSelect([]string{}, func(sel string) {
		if guiState.DNSSelectsProgrammatic {
			return
		}
		mod := presenter.Model()
		def := locale.T("wizard.dns.strategy_default")
		newStr := ""
		if sel != def {
			newStr = sel
		}
		if mod.DNSStrategy != newStr {
			mod.DNSStrategy = newStr
			mod.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
		}
	})
	strategyLabel := widget.NewLabel(locale.T("wizard.dns.label_strategy"))

	independentCacheLabel := widget.NewLabel(locale.T("wizard.dns.label_independent_cache"))
	guiState.DNSIndependentCacheCheck = widget.NewCheck("", func(checked bool) {
		if guiState.DNSSelectsProgrammatic {
			return
		}
		mod := presenter.Model()
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
	})
	independentCacheHelp := widget.NewButton(locale.T("wizard.rules.button_info"), func() {
		if err := platform.OpenURL(dnsIndependentCacheDocURL); err != nil {
			dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.outbounds.error_open_docs"), err), dialogParent())
		}
	})
	independentCacheHelp.Importance = widget.LowImportance
	independentCacheRow := container.NewHBox(independentCacheLabel, independentCacheHelp, guiState.DNSIndependentCacheCheck)

	strategyAndCacheRow := container.NewHBox(
		strategyLabel,
		guiState.DNSStrategySelect,
		layout.NewSpacer(),
		independentCacheRow,
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
	if server != "" {
		return fmt.Sprintf("%s  ·  %s  ·  %s", tag, typ, server)
	}
	return fmt.Sprintf("%s  ·  %s", tag, typ)
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
// dnsServerDialogEntryMinHeight is the minimum height for the JSON editor in Add/Edit DNS server dialogs
// (2× the rules text block min height on the DNS tab).
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
