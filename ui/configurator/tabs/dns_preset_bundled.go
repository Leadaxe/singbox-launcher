// File dns_preset_bundled.go — read-only секция DNS tab для bundled DNS-серверов
// от активных preset-ref правил.
//
// Юзер видит **что preset реально добавит** в config.json::dns.servers[] — это
// важно для понимания: при активации Russian domains preset с use_dns_override
// у тебя в DNS будут yandex_udp/doh/dot (один из них реально попадает в config —
// тот что выбран через @dns_server var).
//
// Меняется через preset edit dialog (var dns_server picker), не через DNS tab.
package tabs

import (
	"encoding/json"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core/build"
	"singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/fynewidget"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// jsonMarshalIndent — thin alias for jsonPrettyMarshal callers (encapsulates std import).
func jsonMarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

// renderPresetBundledDNSRows — собирает DNS server rows для всех активных
// preset-ref'ов через ResolveDNS (SPEC 056-R-N follow-up).
//
// Использует resolver чтобы:
//   - Получить ВСЕ bundled servers (без consumption-фильтра)
//   - Получить Active flag (прошёл if/if_or)
//   - Получить InactiveReason для tooltip когда !Active
//
// onChanged — callback после toggle чекбокса.
// parentWindow — для View JSON dialog'а.
func renderPresetBundledDNSRows(m *wizardmodels.WizardModel, parentWindow fyne.Window, onChanged func()) []fyne.CanvasObject {
	if m == nil || m.TemplateData == nil {
		return nil
	}
	// Build shadow state из model для передачи в ResolveDNS.
	shadowState := buildShadowStateForResolve(m)
	resolved := build.ResolveDNS(shadowState, m.TemplateData, gatherTemplateVars(m))

	presetByID := make(map[string]*wizardtemplate.Preset, len(m.TemplateData.Presets))
	for i := range m.TemplateData.Presets {
		presetByID[m.TemplateData.Presets[i].ID] = &m.TemplateData.Presets[i]
	}

	var rows []fyne.CanvasObject
	for _, srv := range resolved.Servers {
		if srv.Source != build.DNSSourcePreset {
			continue
		}
		tpl := presetByID[srv.PresetID]
		if tpl == nil {
			continue
		}
		srvCopy := srv
		tplCopy := tpl
		// Найти PresetRefState для записи toggle (lazy lookup на каждый toggle —
		// model.PresetRefs может пересоздаваться).
		onToggle := func(v bool) {
			for _, pr := range m.PresetRefs {
				if pr != nil && pr.Ref == srvCopy.PresetID {
					pr.SetDNSServerEnabled(srvCopy.LocalTag, v)
					break
				}
			}
			if onChanged != nil {
				onChanged()
			}
		}
		onView := func() {
			body, _ := jsonPrettyMarshal(srvCopy.Body)
			header := widget.NewLabelWithStyle(
				"🔒  From preset: "+presetDisplayLabel(tplCopy),
				fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
			)
			helpLabel := widget.NewLabelWithStyle(
				"Read-only preset DNS server. Toggle on/off via checkbox.",
				fyne.TextAlignLeading, fyne.TextStyle{Italic: true},
			)
			helpLabel.Wrapping = fyne.TextWrapWord
			showJSONReadOnlyDialog(parentWindow, "DNS server details", header, helpLabel, body)
		}
		row := buildPresetBundledDNSRowFromResolved(tplCopy, srvCopy, onToggle, onView)
		if row != nil {
			rows = append(rows, row)
		}
	}
	return rows
}

// presetDisplayLabel — helper для UI: label или fallback на ID.
func presetDisplayLabel(p *wizardtemplate.Preset) string {
	if p.Label != "" {
		return p.Label
	}
	return p.ID
}

// buildShadowStateForResolve — конструирует временный state.State из model для
// передачи в build.ResolveDNS на render-time. Не полностью equivalent тому
// что напишется на диск — нам нужны только Rules (для preset-ref discovery)
// и DNS.Servers/Rules (для enabled overrides).
//
// SPEC 056-R-N follow-up: enabled читаем из PresetRefState.DNSServerEnabled /
// DNSRuleEnabled (default true). Раньше были отдельные карты в model.
func buildShadowStateForResolve(m *wizardmodels.WizardModel) *state.State {
	if m == nil {
		return nil
	}
	st := &state.State{}
	st.Rules = wizardmodels.SyncPresetRefsToStateRules(m.PresetRefs)
	// Для каждого PresetRefState собираем preset DNS entries с toggle'ами.
	// Render использует это для visualisation (ResolveDNS читает Enabled).
	for _, pr := range m.PresetRefs {
		if pr == nil || pr.Ref == "" {
			continue
		}
		for localTag, enabled := range pr.DNSServerEnabled {
			st.DNS.Servers = append(st.DNS.Servers, state.DNSServer{
				Kind:    state.DNSServerKindPreset,
				Ref:     pr.Ref + ":" + localTag,
				Enabled: enabled,
			})
		}
		if pr.DNSRuleEnabled != nil {
			st.DNS.Rules = append(st.DNS.Rules, state.DNSRule{
				Kind:    state.DNSRuleKindPreset,
				Ref:     pr.Ref,
				Enabled: *pr.DNSRuleEnabled,
			})
		}
	}
	return st
}

// gatherTemplateVars — собирает global template vars из model для substitute
// на render-time. Объединяет SettingsVars и фиксированные dns_* scalars.
func gatherTemplateVars(m *wizardmodels.WizardModel) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m.SettingsVars))
	for k, v := range m.SettingsVars {
		out[k] = v
	}
	return out
}

// renderPresetBundledDNSRulesRows — собирает DNS rule rows через ResolveDNS.
// SPEC 056-R-N follow-up: единая логика с server rendering.
func renderPresetBundledDNSRulesRows(m *wizardmodels.WizardModel, parentWindow fyne.Window, onChanged func()) []fyne.CanvasObject {
	if m == nil || m.TemplateData == nil {
		return nil
	}
	shadowState := buildShadowStateForResolve(m)
	resolved := build.ResolveDNS(shadowState, m.TemplateData, gatherTemplateVars(m))

	presetByID := make(map[string]*wizardtemplate.Preset, len(m.TemplateData.Presets))
	for i := range m.TemplateData.Presets {
		presetByID[m.TemplateData.Presets[i].ID] = &m.TemplateData.Presets[i]
	}

	var rows []fyne.CanvasObject
	for _, dr := range resolved.Rules {
		if dr.Source != build.DNSSourcePreset {
			continue
		}
		tpl := presetByID[dr.PresetID]
		if tpl == nil {
			continue
		}
		ruleCopy := dr.Body
		tplCopy := tpl
		ref := tplCopy.ID
		drCopy := dr
		onToggle := func(v bool) {
			for _, pr := range m.PresetRefs {
				if pr != nil && pr.Ref == ref {
					pr.SetDNSRuleEnabled(v)
					break
				}
			}
			if onChanged != nil {
				onChanged()
			}
		}
		onView := func() { showBundledDNSRuleDetailsDialog(parentWindow, tplCopy, ruleCopy) }
		rows = append(rows, buildPresetBundledDNSRuleRowFromResolved(tplCopy, drCopy, onToggle, onView))
	}
	return rows
}

// buildPresetBundledDNSRuleRowFromResolved — SPEC 056-R-N: row из ResolvedDNSRule.
// Active=false → checkbox disabled + tooltip с InactiveReason.
func buildPresetBundledDNSRuleRowFromResolved(
	tpl *wizardtemplate.Preset,
	dr build.ResolvedDNSRule,
	onToggle func(bool),
	onView func(),
) fyne.CanvasObject {
	// rule_set summary (local tag'и без preset-prefix).
	ruleSetSummary := ""
	switch v := dr.Body["rule_set"].(type) {
	case string:
		ruleSetSummary = stripPresetPrefix(v, tpl.ID)
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				parts = append(parts, stripPresetPrefix(s, tpl.ID))
			}
		}
		ruleSetSummary = joinSep(parts, ", ")
	}

	presetLabel := tpl.Label
	if presetLabel == "" {
		presetLabel = tpl.ID
	}

	rowText := "🔒 " + presetLabel
	if ruleSetSummary != "" {
		rowText += " (" + ruleSetSummary + ")"
	}
	titleLabel := ttwidget.NewLabel(rowText)
	titleLabel.Wrapping = fyne.TextTruncate

	// Tooltip: server + rule_set + inactive reason если применимо.
	server, _ := dr.Body["server"].(string)
	server = stripPresetPrefix(server, tpl.ID)
	tipParts := []string{}
	if server != "" {
		tipParts = append(tipParts, "server="+server)
	}
	if ruleSetSummary != "" {
		tipParts = append(tipParts, "rule_set="+ruleSetSummary)
	}
	if !dr.Active && dr.InactiveReason != "" {
		tipParts = append(tipParts, "inactive ("+dr.InactiveReason+")")
	}
	tip := joinSep(tipParts, " · ")

	var row *fynewidget.HoverRow
	rowGetter := func() *fynewidget.HoverRow { return row }

	cwc := fynewidget.NewCheckWithContent(func(checked bool) {
		if onToggle != nil {
			onToggle(checked)
		}
	}, titleLabel, fynewidget.CheckWithContentConfig{ContentToolTip: tip})
	cwc.Check.SetChecked(dr.Enabled)
	if !dr.Active {
		cwc.Check.Disable()
	}

	var right *fyne.Container
	if onView != nil {
		viewBtn := fynewidget.NewHoverForwardButtonWithIcon("View JSON", theme.SearchIcon(), onView, rowGetter)
		viewBtn.Importance = widget.LowImportance
		right = container.NewHBox(viewBtn)
	} else {
		right = container.NewHBox()
	}

	rowInner := container.NewBorder(nil, nil, cwc.CheckLeading, right, cwc.Content)
	row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{})
	row.WireTooltipLabelHover(titleLabel)
	return row
}

// lookupPresetEnabled — default true (preset включает все свои entries по умолчанию).
// Юзер может выключить отдельный через UI → key=false.
func lookupPresetEnabled(m map[string]bool, key string) bool {
	if m == nil {
		return true
	}
	v, has := m[key]
	if !has {
		return true
	}
	return v
}

// buildPresetBundledDNSRuleRow — одна строка для bundled DNS-rule preset'а
// (SPEC 056-R-N kind=preset).
//
// Layout: `[✅] 🔒 <preset-label> (<rule_set_tags>)         [View JSON]`
// Tooltip: `server=<server> · rule_set=<full-list>` (детали в tooltip).
// Используется тот же widget pattern что и обычные DNS-server rows
// (NewCheckWithContent + HoverRow + WireTooltipLabelHover) — clickable
// hover-row, tooltip на hover label.
func buildPresetBundledDNSRuleRow(
	tpl *wizardtemplate.Preset,
	rule map[string]interface{},
	enabled bool,
	onToggle func(bool),
	onView func(),
) fyne.CanvasObject {
	// Соберём rule_set summary (local tag'и без preset-prefix).
	ruleSetSummary := ""
	switch v := rule["rule_set"].(type) {
	case string:
		ruleSetSummary = stripPresetPrefix(v, tpl.ID)
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				parts = append(parts, stripPresetPrefix(s, tpl.ID))
			}
		}
		ruleSetSummary = joinSep(parts, ", ")
	}

	presetLabel := tpl.Label
	if presetLabel == "" {
		presetLabel = tpl.ID
	}

	rowText := "🔒 " + presetLabel
	if ruleSetSummary != "" {
		rowText += " (" + ruleSetSummary + ")"
	}
	titleLabel := ttwidget.NewLabel(rowText)
	titleLabel.Wrapping = fyne.TextTruncate

	// Tooltip: server + полный rule_set list.
	server, _ := rule["server"].(string)
	server = stripPresetPrefix(server, tpl.ID)
	tipParts := []string{}
	if server != "" {
		tipParts = append(tipParts, "server="+server)
	}
	if ruleSetSummary != "" {
		tipParts = append(tipParts, "rule_set="+ruleSetSummary)
	}
	tip := joinSep(tipParts, " · ")

	var row *fynewidget.HoverRow
	rowGetter := func() *fynewidget.HoverRow { return row }

	cwc := fynewidget.NewCheckWithContent(func(checked bool) {
		if onToggle != nil {
			onToggle(checked)
		}
	}, titleLabel, fynewidget.CheckWithContentConfig{ContentToolTip: tip})
	cwc.Check.SetChecked(enabled)

	var right *fyne.Container
	if onView != nil {
		viewBtn := fynewidget.NewHoverForwardButtonWithIcon("View JSON", theme.SearchIcon(), onView, rowGetter)
		viewBtn.Importance = widget.LowImportance
		right = container.NewHBox(viewBtn)
	} else {
		right = container.NewHBox()
	}

	rowInner := container.NewBorder(nil, nil, cwc.CheckLeading, right, cwc.Content)
	row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{})
	row.WireTooltipLabelHover(titleLabel)
	return row
}

// showBundledDNSRuleDetailsDialog — read-only modal с DNS-rule preset'а.
// Изменения preset-bundled DNS rule НЕВОЗМОЖНЫ — содержимое определяется template'ом
// + значениями vars. Юзер для кастомных DNS-rules использует Extra rules editor
// (внизу DNS tab) — это полностью отдельный механизм.
func showBundledDNSRuleDetailsDialog(parent fyne.Window, tpl *wizardtemplate.Preset, rule map[string]interface{}) {
	body, _ := jsonPrettyMarshal(rule)
	showBundledReadOnlyDetails(parent, tpl, "DNS rule details", body)
}

// showBundledReadOnlyDetails — модал с monospace JSON preview через RichText,
// без редактирования (read-only). Юзер может выделять/копировать текст.
func showBundledReadOnlyDetails(parent fyne.Window, tpl *wizardtemplate.Preset, title, jsonBody string) {
	if parent == nil {
		return
	}
	desc := tpl.Label
	if desc == "" {
		desc = tpl.ID
	}
	header := widget.NewLabelWithStyle(
		"🔒  From preset: "+desc,
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
	)
	helpLabel := widget.NewLabelWithStyle(
		"Read-only. Edit via preset variables (Rules tab → Edit). For custom DNS rules use the Extra rules editor below.",
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true},
	)
	helpLabel.Wrapping = fyne.TextWrapWord
	showJSONReadOnlyDialog(parent, title, header, helpLabel, jsonBody)
}

// showTemplateDNSDetailsDialog — read-only modal для template DNS-сервера
// (entries из template.dns_options.servers[]).
//
// Header показывает tag + 🔒 (или ⛔ если required). Body — pretty JSON.
func showTemplateDNSDetailsDialog(parent fyne.Window, body map[string]interface{}, required bool) {
	if parent == nil || body == nil {
		return
	}
	tag, _ := body["tag"].(string)
	icon := "🔒"
	helpText := "Read-only template DNS server. Toggle on/off via checkbox."
	if required {
		icon = "⛔"
		helpText = "Required template DNS server: always enabled, always emitted."
	}
	header := widget.NewLabelWithStyle(
		icon+"  Template DNS server: "+tag,
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
	)
	helpLabel := widget.NewLabelWithStyle(
		helpText,
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true},
	)
	helpLabel.Wrapping = fyne.TextWrapWord
	jsonBody, _ := jsonPrettyMarshal(body)
	showJSONReadOnlyDialog(parent, "DNS server details", header, helpLabel, jsonBody)
}

// showJSONReadOnlyDialog — общий low-level: title + header + helpLabel + JSON pretty.
// Используется обоими: showBundledReadOnlyDetails (preset) + showTemplateDNSDetailsDialog (template).
func showJSONReadOnlyDialog(parent fyne.Window, title string, header, helpLabel fyne.CanvasObject, jsonBody string) {
	if parent == nil {
		return
	}

	jsonRich := widget.NewRichTextFromMarkdown("```json\n" + jsonBody + "\n```")
	jsonRich.Wrapping = fyne.TextWrapWord
	scroll := container.NewScroll(jsonRich)

	content := container.NewBorder(
		container.NewVBox(header, helpLabel),
		nil, nil, nil,
		scroll,
	)
	d := dialog.NewCustom(title, "Close", content, parent)
	d.Resize(fyne.NewSize(560, 440))
	d.Show()
}

// jsonPrettyMarshal — JSON pretty-print для bundled DNS server / rule deтail dialog'а.
func jsonPrettyMarshal(v interface{}) (string, error) {
	b, err := jsonMarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// buildPresetBundledDNSRowFromResolved — кладёт ResolvedDNSServer в widget row.
// SPEC 056-R-N: Active=false → checkbox disabled + tooltip с InactiveReason.
// View JSON кнопка справа — read-only inspect body (нет Edit/Del у preset entries).
func buildPresetBundledDNSRowFromResolved(
	tpl *wizardtemplate.Preset,
	srv build.ResolvedDNSServer,
	onToggle func(bool),
	onView func(),
) fyne.CanvasObject {
	presetLabel := tpl.Label
	if presetLabel == "" {
		presetLabel = tpl.ID
	}

	rowText := srv.LocalTag
	if rowText == "" {
		rowText = srv.Tag
	}
	rowText += " · 🔒 " + presetLabel

	titleLabel := ttwidget.NewLabel(rowText)
	titleLabel.Wrapping = fyne.TextTruncate

	tipParts := []string{}
	if typ, ok := srv.Body["type"].(string); ok && typ != "" {
		tipParts = append(tipParts, typ)
	}
	if s, ok := srv.Body["server"].(string); ok && s != "" {
		tipParts = append(tipParts, s)
	}
	if desc, ok := srv.Body["description"].(string); ok && desc != "" {
		tipParts = append(tipParts, desc)
	}
	if !srv.Active && srv.InactiveReason != "" {
		tipParts = append(tipParts, "inactive ("+srv.InactiveReason+")")
	}
	tip := joinSep(tipParts, " · ")

	var row *fynewidget.HoverRow
	rowGetter := func() *fynewidget.HoverRow { return row }

	cwc := fynewidget.NewCheckWithContent(func(checked bool) {
		if onToggle != nil {
			onToggle(checked)
		}
	}, titleLabel, fynewidget.CheckWithContentConfig{ContentToolTip: tip})
	cwc.Check.SetChecked(srv.Enabled)
	if !srv.Active {
		cwc.Check.Disable()
	}

	// rowGutter — reserved space под scrollbar (visual паритет с template-rows).
	rowGutter := canvas.NewRectangle(color.Transparent)
	rowGutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))

	var right *fyne.Container
	if onView != nil {
		viewBtn := fynewidget.NewHoverForwardButtonWithIcon("View", theme.SearchIcon(), onView, rowGetter)
		viewBtn.Importance = widget.LowImportance
		right = container.NewHBox(viewBtn, rowGutter)
	} else {
		right = container.NewHBox(rowGutter)
	}

	rowInner := container.NewBorder(nil, nil, cwc.CheckLeading, right, cwc.Content)
	row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{})
	row.WireTooltipLabelHover(titleLabel)
	return row
}

// buildPresetBundledDNSRow — legacy entry point (используется renderPresetBundledDNSRulesRows
// и тестами). Будет удалён вместе с Phase 6.
//
// SPEC 056-R-N kind=preset.
//
// Layout: `[✅] <tag> · 🔒 <preset-label>`
// Tooltip: `type · server[:port] · description` (детали в tooltip).
// Используется тот же widget pattern что и обычные DNS-server rows.
func buildPresetBundledDNSRow(
	tpl *wizardtemplate.Preset,
	ds map[string]interface{},
	enabled bool,
	onToggle func(bool),
) fyne.CanvasObject {
	tag, _ := ds["tag"].(string)
	typ, _ := ds["type"].(string)
	server, _ := ds["server"].(string)
	desc, _ := ds["description"].(string)

	localTag := stripPresetPrefix(tag, tpl.ID)
	presetLabel := tpl.Label
	if presetLabel == "" {
		presetLabel = tpl.ID
	}

	rowText := localTag
	if rowText == "" {
		rowText = tag
	}
	rowText += " · 🔒 " + presetLabel

	titleLabel := ttwidget.NewLabel(rowText)
	titleLabel.Wrapping = fyne.TextTruncate

	tipParts := []string{}
	if typ != "" {
		tipParts = append(tipParts, typ)
	}
	if server != "" {
		tipParts = append(tipParts, server)
	}
	if desc != "" {
		tipParts = append(tipParts, desc)
	}
	tip := joinSep(tipParts, " · ")

	var row *fynewidget.HoverRow

	cwc := fynewidget.NewCheckWithContent(func(checked bool) {
		if onToggle != nil {
			onToggle(checked)
		}
	}, titleLabel, fynewidget.CheckWithContentConfig{ContentToolTip: tip})
	cwc.Check.SetChecked(enabled)

	rowInner := container.NewBorder(nil, nil, cwc.CheckLeading, nil, cwc.Content)
	row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{})
	row.WireTooltipLabelHover(titleLabel)
	return row
}

// stripPresetPrefix — убирает `<preset_id>:` префикс из tag'а если он там есть.
// `"ru-direct:yandex_udp"` → `"yandex_udp"`. Если префикса нет — возвращает as is.
func stripPresetPrefix(tag, presetID string) string {
	prefix := presetID + ":"
	if presetID != "" && len(tag) > len(prefix) && tag[:len(prefix)] == prefix {
		return tag[len(prefix):]
	}
	return tag
}

// joinSep — простой join без strings package (минимизировать imports в UI файле).
func joinSep(parts []string, sep string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += p
	}
	return out
}
