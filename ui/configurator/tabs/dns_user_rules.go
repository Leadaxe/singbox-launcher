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

	"singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	internaldialogs "singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
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

	ed := newDNSRuleEditor(model, working)

	titleStr := locale.T("Add DNS Rule")
	if idx >= 0 {
		titleStr = locale.T("Edit DNS Rule")
	}

	controller := presenter.Controller()
	if controller == nil || controller.UIService == nil {
		return
	}
	editWin := controller.UIService.Application.NewWindow(titleStr)
	ed.win = editWin

	cancelBtn := widget.NewButton(locale.T("Cancel"), func() { editWin.Close() })
	saveBtn := widget.NewButton(locale.T("Save"), func() {
		finalRule, err := ed.rule()
		if err != nil {
			dialog.ShowError(err, editWin)
			return
		}
		if len(finalRule) == 0 {
			dialog.ShowError(fmt.Errorf("%s", locale.T("Rule is empty")), editWin)
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
		presenter.MarkAsChanged()
		editWin.Close()
		if onChanged != nil {
			onChanged()
		}
	})
	saveBtn.Importance = widget.HighImportance

	buttons := container.NewHBox(layout.NewSpacer(), cancelBtn, saveBtn)
	dialogContent := container.NewBorder(nil, buttons, nil, nil, ed.tabs)
	editWin.Resize(fyne.NewSize(500, 600))
	fynewidget.CenterOnScreen(editWin)
	editWin.SetContent(dialogContent)
	editWin.SetCloseIntercept(func() { editWin.Close() })
	editWin.Show()
}

// dnsRuleEditor — содержимое окна DNS-правила: вкладки Form/JSON над рабочей
// копией правила. Окно, кнопки и запись в модель — у showEditUserDNSRuleDialog;
// сам редактор контроллера не требует.
type dnsRuleEditor struct {
	tabs         *container.AppTabs
	formTab      *container.TabItem
	jsonTab      *container.TabItem
	ruleSetCheck *widget.CheckGroup
	typeRadio    *widget.RadioGroup
	jsonEntry    fynewidget.JSONEditor
	win          fyne.Window // родитель для ошибки битого JSON; задаётся после создания окна
	rule         func() (map[string]interface{}, error)
}

// newDNSRuleEditor — редактор над working (меняет его на месте).
func newDNSRuleEditor(model *wizardmodels.WizardModel, working map[string]interface{}) *dnsRuleEditor {
	const (
		typeSRS    = "SRS (existing rule_set)"
		typeInline = "Inline (match fields)"
	)

	// Виджеты формы создаются пустыми и заполняются из working в loadForm —
	// при открытии и при переходе JSON → Form. Значения, которых нет среди
	// вариантов (выключенное правило, сервер, ручной ввод в JSON), добавляются
	// в варианты, а не теряются: форма не должна молча урезать правило (#140).

	// === SRS section ===
	// Мульти-выбор: rule_set в sing-box — список, правило может ссылаться
	// на несколько наборов.
	availableRuleSetTags := collectAllRuleSetTags(model)
	ruleSetCheck := widget.NewCheckGroup(availableRuleSetTags, nil)
	srsSection := container.NewVBox(
		widget.NewLabel(locale.T("Rule set tag:")),
		ruleSetCheck,
	)

	// === Inline section ===
	domainSuffixEntry := widget.NewMultiLineEntry()
	domainSuffixEntry.SetPlaceHolder(locale.T("one suffix per line (e.g. example.com)"))

	domainEntry := widget.NewMultiLineEntry()
	domainEntry.SetPlaceHolder(locale.T("one exact domain per line"))

	keywordEntry := widget.NewMultiLineEntry()
	keywordEntry.SetPlaceHolder(locale.T("one keyword per line"))

	ipCIDREntry := widget.NewMultiLineEntry()
	ipCIDREntry.SetPlaceHolder(locale.T("one CIDR per line (e.g. 10.0.0.0/8)"))

	inlineSection := container.NewVBox(
		widget.NewLabel(locale.T("Domain suffix:")),
		domainSuffixEntry,
		widget.NewLabel(locale.T("Domain (exact):")),
		domainEntry,
		widget.NewLabel(locale.T("Domain keyword:")),
		keywordEntry,
		widget.NewLabel(locale.T("IP CIDR:")),
		ipCIDREntry,
	)

	// === Server picker (общий) ===
	serverOptions := wizardbusiness.DNSEnabledTagOptions(model)
	if len(serverOptions) == 0 {
		serverOptions = []string{""}
	}
	serverSelect := widget.NewSelect(serverOptions, nil)

	// === Type radio ===
	currentType := typeInline
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
	// Поля другого режима вычищаются только при ручной смене типа: правило
	// из JSON может сочетать rule_set с domain_suffix, и открытие формы
	// его не урезает.
	loading := false
	typeRadio := widget.NewRadioGroup(
		[]string{typeSRS, typeInline},
		func(sel string) {
			if sel == "" {
				return
			}
			currentType = sel
			updateSectionVisibility()
			if loading {
				return
			}
			if currentType == typeSRS {
				for _, k := range dnsInlineMatchKeys {
					delete(working, k)
				}
			} else {
				delete(working, "rule_set")
			}
		},
	)
	typeRadio.Horizontal = true

	loadForm := func() {
		loading = true
		defer func() { loading = false }()
		rs := stringListValues(working, "rule_set")
		ruleSetCheck.Options = withMissingOptions(availableRuleSetTags, rs)
		ruleSetCheck.SetSelected(rs)
		domainSuffixEntry.SetText(joinStringList(working, "domain_suffix"))
		domainEntry.SetText(joinStringList(working, "domain"))
		keywordEntry.SetText(joinStringList(working, "domain_keyword"))
		ipCIDREntry.SetText(joinStringList(working, "ip_cidr"))
		server, _ := working["server"].(string)
		if server != "" {
			serverSelect.Options = withMissingOptions(serverOptions, []string{server})
		}
		serverSelect.SetSelected(server)
		currentType = typeInline
		if _, ok := working["rule_set"]; ok {
			currentType = typeSRS
		}
		typeRadio.SetSelected(currentType)
		updateSectionVisibility()
	}
	loadForm()

	formContent := container.NewVBox(
		widget.NewLabel(locale.T("Type:")),
		typeRadio,
		widget.NewSeparator(),
		srsSection,
		inlineSection,
		widget.NewSeparator(),
		widget.NewLabel(locale.T("Server:")),
		serverSelect,
	)

	// === JSON tab ===
	jsonEntry := fynewidget.NewJSONEditor("")
	refreshJSON := func() {
		updateFromForm(working, currentRuleType(), ruleSetCheck, domainSuffixEntry, domainEntry, keywordEntry, ipCIDREntry, serverSelect)
		b, _ := json.MarshalIndent(working, "", "  ")
		jsonEntry.SetText(string(b))
	}
	refreshJSON()

	formTab := container.NewTabItem(locale.T("Form"), container.NewScroll(container.NewPadded(formContent)))
	jsonTab := container.NewTabItem(locale.T("JSON"), container.NewScroll(container.NewPadded(jsonEntry.Object())))
	tabs := container.NewAppTabs(formTab, jsonTab)
	// JSON → Form: правка в JSON переносится в форму, а не затирается ею
	// при следующем возврате на вкладку JSON. Битый JSON оставляем на месте.
	ed := &dnsRuleEditor{ruleSetCheck: ruleSetCheck, typeRadio: typeRadio, jsonEntry: jsonEntry, formTab: formTab, jsonTab: jsonTab, tabs: tabs}
	keepJSON := false
	tabs.OnSelected = func(t *container.TabItem) {
		switch t {
		case jsonTab:
			if keepJSON {
				keepJSON = false
				return
			}
			refreshJSON()
		case formTab:
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(jsonEntry.Text()), &parsed); err != nil {
				keepJSON = true
				tabs.Select(jsonTab)
				if ed.win != nil {
					dialog.ShowError(fmt.Errorf("%s: %w", locale.T("Invalid JSON"), err), ed.win)
				}
				return
			}
			if parsed == nil {
				parsed = map[string]interface{}{}
			}
			working = parsed
			loadForm()
		}
	}

	// Итог по текущей вкладке: на JSON — её текст, на форме — форма.
	ed.rule = func() (map[string]interface{}, error) {
		if tabs.Selected() == jsonTab {
			var finalRule map[string]interface{}
			if err := json.Unmarshal([]byte(jsonEntry.Text()), &finalRule); err != nil {
				return nil, fmt.Errorf("%s: %w", locale.T("Invalid JSON"), err)
			}
			return finalRule, nil
		}
		updateFromForm(working, currentRuleType(), ruleSetCheck, domainSuffixEntry, domainEntry, keywordEntry, ipCIDREntry, serverSelect)
		return working, nil
	}
	return ed
}

// dnsInlineMatchKeys — поля режима Inline формы DNS-правила.
var dnsInlineMatchKeys = []string{"domain_suffix", "domain", "domain_keyword", "ip_cidr"}

// updateFromForm — read form widgets → write into working map.
// ruleType: "srs" → используется только ruleSetSel (rule_set field в working).
// Один набор пишется строкой, несколько — списком.
//
//	"inline" → используются domain_suffix / domain / keyword / ip_cidr entry'и.
//
// Server общий для обоих режимов.
//
// Поля противоположного режима не трогает — их вычищает ручная смена типа
// в форме (#140).
func updateFromForm(
	working map[string]interface{},
	ruleType string,
	ruleSetSel *widget.CheckGroup,
	dsEntry, dEntry, kEntry, ipEntry *widget.Entry,
	serverSel *widget.Select,
) {
	if ruleType == "srs" {
		switch sel := ruleSetSel.Selected; len(sel) {
		case 0:
			delete(working, "rule_set")
		case 1:
			working["rule_set"] = sel[0]
		default:
			list := make([]interface{}, len(sel))
			for i, rs := range sel {
				list[i] = rs
			}
			working["rule_set"] = list
		}
	} else {
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

// stringListValues — значения map[key] (string или []string) списком.
func stringListValues(m map[string]interface{}, key string) []string {
	switch x := m[key].(type) {
	case string:
		if x != "" {
			return []string{x}
		}
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, v := range x {
			if str, ok := v.(string); ok && str != "" {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

// withMissingOptions — options плюс значения из values, которых в них нет.
func withMissingOptions(options, values []string) []string {
	out := append([]string(nil), options...)
	for _, v := range values {
		found := false
		for _, o := range out {
			if o == v {
				found = true
				break
			}
		}
		if !found {
			out = append(out, v)
		}
	}
	return out
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
//   - наборы пользовательских srs-правил (user:<id>, user:<id>:N)
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
			frags, _, ok := wizardbusiness.ExpandPresetForView(m, tpl, pr.Vars)
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

	// Наборы пользовательских srs-правил — те же теги, что эмитит сборка
	// (resolve_route.go): `user:<id>`, `user:<id>:2`, … по одному на URL.
	// Inline-правила rule_set не порождают. Ссылку на набор, который не попал
	// в route.rule_set (правило выключено, .srs не скачан), сборка вычищает
	// из DNS-правила сама (SPEC 153). Issue #140.
	for _, r := range wizardmodels.EmitStateRulesWithoutOrder(nil, m.CustomRules) {
		if r.Kind != state.RuleKindSrs {
			continue
		}
		id := state.StableRuleID(r)
		for i := range r.Refs {
			add(state.SrsRuleSetTag(id, i))
		}
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
			frags, _, ok := wizardbusiness.ExpandPresetForView(m, tpl, pr.Vars)
			if !ok {
				continue
			}
			// SPEC 085.1: a preset may bundle several DNS rules (singular
			// DNSRule + plural DNSRules) behind one slot; materialize all.
			if frags.DNSRule != nil {
				allRules = append(allRules, frags.DNSRule)
			}
			allRules = append(allRules, frags.DNSRules...)
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
		locale.T("Read-only preview of final config.json::dns.rules in user order (preset and user rules interleaved per DNSRuleOrder)."),
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
