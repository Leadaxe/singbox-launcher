// File dns_user_rules.go — UI для user-defined DNS rules.
//
// Заменяет legacy MultiLineEntry JSON editor на:
//   - Row-list user-defined rules (как DNS servers): brief summary + Edit/Del
//   - [+ Add Rule] button: открывает Form+JSON tabs dialog для создания
//   - [View All DNS Rules] button: popup со скомпилированными rules
//     (bundled preset + user) — read-only preview финального config.json:dns.rules
//
// State: `model.DNSRulesText` остаётся как **сериализованная строка** для
// presenter sync, но UI больше не показывает её как textarea — list rows
// генерируются из parsed `state.dns.extra_rules` (через json.Unmarshal на
// DNSRulesText).
package tabs

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/build"
	wizardtemplate "singbox-launcher/core/template"
	internaldialogs "singbox-launcher/internal/dialogs"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// SPEC 062-F-N: legacy text-based user DNS rule helpers (userDNSRulesParsed,
// userDNSRulesSerialize, setUserDNSRulesText) удалены — UI теперь работает
// с typed model.DNSUserRules. Sync hidden DNSRulesEntry widget живёт в
// dns_unified_rules.go::syncDNSRulesTextToHiddenEntry.

// dnsRuleSummary — human-readable краткое описание rule для tile.
// Берёт match-поля + server. Tooltip полный JSON.
func dnsRuleSummary(rule map[string]interface{}) (title, tooltip string) {
	parts := []string{}
	addList := func(key, prefix string) {
		v, ok := rule[key]
		if !ok {
			return
		}
		switch x := v.(type) {
		case string:
			if x != "" {
				parts = append(parts, prefix+x)
			}
		case []interface{}:
			if len(x) > 0 {
				strs := make([]string, 0, len(x))
				for _, s := range x {
					if str, ok := s.(string); ok {
						strs = append(strs, str)
					}
				}
				if len(strs) > 0 {
					parts = append(parts, prefix+strings.Join(strs, ","))
				}
			}
		}
	}
	addList("domain", "domain=")
	addList("domain_suffix", "suffix=")
	addList("domain_keyword", "keyword=")
	addList("ip_cidr", "cidr=")
	addList("rule_set", "rule_set=")

	// Note: NO `→ ` prefix here — the unified rules list (dns_unified_rules.go)
	// adds its own `→ ` per row to mark user-rule vs `🔗` for preset-rule.
	// Embedding the arrow in the summary led to a double-arrow «→ → server»
	// when both wrappers ran. Keep this string clean; UI decorates.
	if server, ok := rule["server"].(string); ok && server != "" {
		title = server + "  ·  " + strings.Join(parts, " · ")
	} else {
		title = strings.Join(parts, " · ")
	}
	if title == "" {
		title = "(empty rule)"
	}

	full, _ := json.MarshalIndent(rule, "", "  ")
	tooltip = string(full)
	return title, tooltip
}

// SPEC 062-F-N: renderUserDNSRulesRows removed — replaced by
// dns_unified_rules.go::buildSingleDNSUserRuleRow в едином ordered списке.
// dnsRuleSummary остаётся (используется обоими user/preset row builders).

// showEditUserDNSRuleDialog — editor user DNS rule в отдельном fyne window.
// Radio (2 options): SRS (existing rule_set) | Inline (match fields).
// Form/JSON tabs. idx == -1 — create new; idx >= 0 — edit existing.
//
// SPEC 062-F-N: работает напрямую с typed model.DNSUserRules. Add path
// также добавляет slot в DNSRuleOrder через addDNSUserRule.
func showEditUserDNSRuleDialog(
	presenter *wizardpresentation.WizardPresenter,
	parent fyne.Window,
	idx int,
	onChanged func(),
) {
	if parent == nil {
		return
	}
	model := presenter.Model()

	// Working copy from typed DNSUserRules (Phase 3) — fallback to legacy
	// DNSRulesText parse only when typed list is empty (defensive for
	// transitional state).
	var working map[string]interface{}
	if idx >= 0 && idx < len(model.DNSUserRules) {
		working = cloneRuleMap(model.DNSUserRules[idx].Body)
	} else {
		working = map[string]interface{}{}
	}

	// Type detection: SRS vs Inline.
	const (
		typeSRS    = "SRS (existing rule_set)"
		typeInline = "Inline (match fields)"
	)
	initialType := typeInline
	if _, ok := working["rule_set"]; ok {
		initialType = typeSRS
	}

	// === SRS section ===
	availableRuleSetTags := collectAllRuleSetTags(model)
	ruleSetSelect := widget.NewSelect(availableRuleSetTags, nil)
	if rs, ok := working["rule_set"].(string); ok && rs != "" {
		ruleSetSelect.SetSelected(rs)
	} else if rsArr, ok := working["rule_set"].([]interface{}); ok && len(rsArr) > 0 {
		if first, ok := rsArr[0].(string); ok {
			ruleSetSelect.SetSelected(first)
		}
	}
	srsSection := container.NewVBox(
		widget.NewLabel("Rule set tag:"),
		ruleSetSelect,
	)

	// === Inline section ===
	domainSuffixEntry := widget.NewMultiLineEntry()
	domainSuffixEntry.SetPlaceHolder("one suffix per line (e.g. example.com)")
	domainSuffixEntry.SetText(joinStringList(working, "domain_suffix"))

	domainEntry := widget.NewMultiLineEntry()
	domainEntry.SetPlaceHolder("one exact domain per line")
	domainEntry.SetText(joinStringList(working, "domain"))

	keywordEntry := widget.NewMultiLineEntry()
	keywordEntry.SetPlaceHolder("one keyword per line")
	keywordEntry.SetText(joinStringList(working, "domain_keyword"))

	ipCIDREntry := widget.NewMultiLineEntry()
	ipCIDREntry.SetPlaceHolder("one CIDR per line (e.g. 10.0.0.0/8)")
	ipCIDREntry.SetText(joinStringList(working, "ip_cidr"))

	inlineSection := container.NewVBox(
		widget.NewLabel("Domain suffix:"),
		domainSuffixEntry,
		widget.NewLabel("Domain (exact):"),
		domainEntry,
		widget.NewLabel("Domain keyword:"),
		keywordEntry,
		widget.NewLabel("IP CIDR:"),
		ipCIDREntry,
	)

	// === Server picker (общий) ===
	serverOptions := wizardbusiness.DNSEnabledTagOptions(model)
	if len(serverOptions) == 0 {
		serverOptions = []string{""}
	}
	serverSelect := widget.NewSelect(serverOptions, nil)
	if cur, ok := working["server"].(string); ok && cur != "" {
		serverSelect.SetSelected(cur)
	}

	// === Type radio ===
	currentType := initialType
	currentRuleType := func() string {
		if currentType == typeSRS {
			return "srs"
		}
		return "inline"
	}
	updateSectionVisibility := func() {
		if currentType == typeSRS {
			srsSection.Show()
			inlineSection.Hide()
		} else {
			srsSection.Hide()
			inlineSection.Show()
		}
	}
	typeRadio := widget.NewRadioGroup(
		[]string{typeSRS, typeInline},
		func(sel string) {
			currentType = sel
			updateSectionVisibility()
		},
	)
	typeRadio.Horizontal = true
	typeRadio.SetSelected(currentType)

	formContent := container.NewVBox(
		widget.NewLabel("Type:"),
		typeRadio,
		widget.NewSeparator(),
		srsSection,
		inlineSection,
		widget.NewSeparator(),
		widget.NewLabel("Server:"),
		serverSelect,
	)
	updateSectionVisibility()

	// === JSON tab ===
	jsonEntry := widget.NewMultiLineEntry()
	jsonEntry.Wrapping = fyne.TextWrapWord
	refreshJSON := func() {
		updateFromForm(working, currentRuleType(), ruleSetSelect, domainSuffixEntry, domainEntry, keywordEntry, ipCIDREntry, serverSelect)
		b, _ := json.MarshalIndent(working, "", "  ")
		jsonEntry.SetText(string(b))
	}
	refreshJSON()

	formTab := container.NewTabItem("Form", container.NewScroll(container.NewPadded(formContent)))
	jsonTab := container.NewTabItem("JSON", container.NewScroll(container.NewPadded(jsonEntry)))
	tabs := container.NewAppTabs(formTab, jsonTab)
	tabs.OnSelected = func(t *container.TabItem) {
		if t == jsonTab {
			refreshJSON()
		}
	}

	titleStr := "Add DNS Rule"
	if idx >= 0 {
		titleStr = "Edit DNS Rule"
	}

	controller := presenter.Controller()
	if controller == nil || controller.UIService == nil {
		return
	}
	editWin := controller.UIService.Application.NewWindow(titleStr)

	cancelBtn := widget.NewButton("Cancel", func() { editWin.Close() })
	saveBtn := widget.NewButton("Save", func() {
		// Final: parse current tab. Если на JSON tab юзер редактировал — берём
		// его JSON. Иначе — собираем из form.
		var finalRule map[string]interface{}
		if tabs.Selected() == jsonTab {
			if err := json.Unmarshal([]byte(jsonEntry.Text), &finalRule); err != nil {
				dialog.ShowError(fmt.Errorf("invalid JSON: %w", err), editWin)
				return
			}
		} else {
			updateFromForm(working, currentRuleType(), ruleSetSelect, domainSuffixEntry, domainEntry, keywordEntry, ipCIDREntry, serverSelect)
			finalRule = working
		}
		if len(finalRule) == 0 {
			dialog.ShowError(fmt.Errorf("rule is empty"), editWin)
			return
		}
		// Strip top-level kind/ref/enabled if user pasted state-shape JSON in raw mode.
		clean := make(map[string]interface{}, len(finalRule))
		for k, v := range finalRule {
			switch k {
			case "kind", "ref", "enabled":
				continue
			}
			clean[k] = v
		}
		m := presenter.Model()
		if idx >= 0 && idx < len(m.DNSUserRules) {
			m.DNSUserRules[idx].Body = clean
		} else {
			addDNSUserRule(m, clean)
		}
		// Keep DNSRulesText synced (raw-JSON toggle reads it).
		m.DNSRulesText = wizardmodels.DNSUserRulesToText(m.DNSUserRules)
		syncDNSRulesTextToHiddenEntry(presenter)
		m.TemplatePreviewNeedsUpdate = true
		presenter.MarkAsChanged()
		editWin.Close()
		if onChanged != nil {
			onChanged()
		}
	})
	saveBtn.Importance = widget.HighImportance

	buttons := container.NewHBox(layout.NewSpacer(), cancelBtn, saveBtn)
	dialogContent := container.NewBorder(nil, buttons, nil, nil, tabs)
	editWin.Resize(fyne.NewSize(500, 600))
	editWin.CenterOnScreen()
	editWin.SetContent(dialogContent)
	editWin.SetCloseIntercept(func() { editWin.Close() })
	editWin.Show()
}

// updateFromForm — read form widgets → write into working map.
// ruleType: "srs" → используется только ruleSetSel (rule_set field в working).
//           "inline" → используются domain_suffix / domain / keyword / ip_cidr entry'и.
// Server общий для обоих режимов.
//
// При переключении типа поля противоположного режима **очищаются** из working,
// чтобы JSON не содержал смешанных полей.
func updateFromForm(
	working map[string]interface{},
	ruleType string,
	ruleSetSel *widget.Select,
	dsEntry, dEntry, kEntry, ipEntry *widget.Entry,
	serverSel *widget.Select,
) {
	if ruleType == "srs" {
		// Очищаем inline поля.
		delete(working, "domain")
		delete(working, "domain_suffix")
		delete(working, "domain_keyword")
		delete(working, "ip_cidr")
		if rs := strings.TrimSpace(ruleSetSel.Selected); rs != "" {
			working["rule_set"] = rs
		} else {
			delete(working, "rule_set")
		}
	} else {
		// Очищаем rule_set, заполняем inline.
		delete(working, "rule_set")
		setStringList(working, "domain_suffix", dsEntry.Text)
		setStringList(working, "domain", dEntry.Text)
		setStringList(working, "domain_keyword", kEntry.Text)
		setStringList(working, "ip_cidr", ipEntry.Text)
	}
	if s := strings.TrimSpace(serverSel.Selected); s != "" {
		working["server"] = s
	} else {
		delete(working, "server")
	}
}

// joinStringList — формирует multi-line string из map[key] (string или []string).
func joinStringList(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []interface{}:
		parts := make([]string, 0, len(x))
		for _, s := range x {
			if str, ok := s.(string); ok {
				parts = append(parts, str)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// setStringList — парсит multi-line text → []string и пишет в map[key].
// Пустые строки игнорируются. Если в результате пусто — ключ удаляется.
func setStringList(m map[string]interface{}, key, text string) {
	lines := strings.Split(text, "\n")
	out := make([]interface{}, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		delete(m, key)
	} else {
		m[key] = out
	}
}

func cloneRuleMap(r map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}

// collectAllRuleSetTags — список всех доступных rule_set tag'ов для DNS-rule:
//   - bundled rule_set tag'и от active preset-ref'ов (ru-direct:ru-domains, ...)
//   - user inline/srs правил из state.CustomRules (user:<id>)
//
// Возвращает sorted unique list.
func collectAllRuleSetTags(m *wizardmodels.WizardModel) []string {
	if m == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	add := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			return
		}
		seen[tag] = true
		out = append(out, tag)
	}

	// Bundled rule_set tags from active preset-refs.
	if m.TemplateData != nil {
		presetByID := make(map[string]*wizardtemplate.Preset)
		for i := range m.TemplateData.Presets {
			presetByID[m.TemplateData.Presets[i].ID] = &m.TemplateData.Presets[i]
		}
		for _, pr := range m.PresetRefs {
			if pr == nil || !pr.Enabled {
				continue
			}
			tpl := presetByID[pr.Ref]
			if tpl == nil {
				continue
			}
			frags, _, ok := build.ExpandPreset(tpl, pr.Vars)
			if !ok {
				continue
			}
			for _, rs := range frags.RuleSets {
				if tag, ok := rs["tag"].(string); ok {
					add(tag)
				}
			}
		}
	}

	// User inline/srs rules tag'и (id-based — формат `user:<id>`).
	for _, cr := range m.CustomRules {
		if cr == nil {
			continue
		}
		// CustomRule имеет rule_set (для srs) или генерированный inline tag.
		// Используем generated tag pattern из MergeRulesAndDNS — `user:<id>`.
		// Здесь у нас нет ID, поэтому используем label-based heuristic.
		// Реально юзер ссылается на rule_set от user-rule редко — fallback на пустой.
		_ = cr
	}

	sort.Strings(out)
	return out
}

// showViewAllDNSRulesDialog — popup со всеми скомпилированными DNS rules
// (bundled от active presets + user state.dns.extra_rules). Read-only preview.
func showViewAllDNSRulesDialog(presenter *wizardpresentation.WizardPresenter, parent fyne.Window) {
	if parent == nil {
		return
	}
	m := presenter.Model()
	if m == nil {
		return
	}

	// SPEC 062-F-N: обходим DNSRuleOrder — preview совпадает с тем, что
	// эмитится в state.DNS.Rules (а оттуда — в config.json).
	var allRules []map[string]interface{}
	presetByID := make(map[string]*wizardtemplate.Preset)
	if m.TemplateData != nil {
		for i := range m.TemplateData.Presets {
			presetByID[m.TemplateData.Presets[i].ID] = &m.TemplateData.Presets[i]
		}
	}
	for _, slot := range m.DNSRuleOrder {
		switch slot.Kind {
		case wizardmodels.DNSSlotKindPresetRef:
			if slot.Index < 0 || slot.Index >= len(m.PresetRefs) {
				continue
			}
			pr := m.PresetRefs[slot.Index]
			if pr == nil || !pr.Enabled || !pr.IsDNSRuleEnabled() {
				continue
			}
			tpl := presetByID[pr.Ref]
			if tpl == nil {
				continue
			}
			frags, _, ok := build.ExpandPreset(tpl, pr.Vars)
			if !ok || frags.DNSRule == nil {
				continue
			}
			allRules = append(allRules, frags.DNSRule)
		case wizardmodels.DNSSlotKindUser:
			if slot.Index < 0 || slot.Index >= len(m.DNSUserRules) {
				continue
			}
			ur := m.DNSUserRules[slot.Index]
			if !ur.Enabled || len(ur.Body) == 0 {
				continue
			}
			allRules = append(allRules, ur.Body)
		}
	}
	_ = sort.SliceStable // kept for legacy stub; not used.

	// 4. Pretty JSON
	wrapper := map[string]interface{}{"rules": allRules}
	body, _ := json.MarshalIndent(wrapper, "", "  ")

	header := widget.NewLabelWithStyle(
		fmt.Sprintf("All DNS rules (%d total)", len(allRules)),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
	)
	help := widget.NewLabelWithStyle(
		"Read-only preview of final config.json::dns.rules in user order (preset and user rules interleaved per DNSRuleOrder).",
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true},
	)
	help.Wrapping = fyne.TextWrapWord

	rich := widget.NewRichTextFromMarkdown("```json\n" + string(body) + "\n```")
	rich.Wrapping = fyne.TextWrapWord
	scroll := container.NewScroll(rich)

	content := container.NewBorder(
		container.NewVBox(header, help),
		nil, nil, nil,
		scroll,
	)
	d := internaldialogs.NewCustom("DNS Rules Preview", content, nil, "Close", parent)
	d.Resize(fyne.NewSize(620, 520))
	d.Show()
}
