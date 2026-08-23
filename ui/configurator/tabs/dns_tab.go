package tabs

import (
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

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// setTooltip is the package-shared, nil/empty-guarded tooltip helper for the
// configurator tabs — prefer it over bare o.SetToolTip(...) so empty strings
// and nil objects can't panic or render blank tooltips. (SPEC 069 §5.4 noted
// "three setTooltip helpers" but only this one exists; it's already shared
// package-wide. Direct .SetToolTip and fynewidget.SetToolTipSafe calls also
// remain in places that need the unguarded form.)
func setTooltip(o fyne.CanvasObject, text string) {
	if text == "" || o == nil {
		return
	}
	fynewidget.SetToolTipSafe(o, text)
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
			g := components.NewScrollGutter()
			serversBox.Add(container.NewHBox(
				widget.NewLabel(locale.T("wizard.dns.no_servers")),
				layout.NewSpacer(),
				g,
			))
			serversBox.Refresh()
			return
		}
		// Bundled DNS-серверы от активных preset'ов — добавляются В КОНЕЦ
		// единого списка серверов (SPEC 056-R-N: убрали отдельную секцию,
		// 🔒 рядом с tag'ом и так показывает что сервер пришёл из preset).
		bundledRows := renderPresetBundledDNSRows(m, dialogParent(), func() {
			presenter.MarkAsChanged()
		})

		for i := range m.DNSServers {
			func(idx int) {
				var row *fynewidget.HoverRow
				rowGetter := func() *fynewidget.HoverRow { return row }

				raw := m.DNSServers[idx]
				var obj map[string]interface{}
				if err := json.Unmarshal(raw, &obj); err != nil {
					obj = nil
				}
				sum := dnsServerSummaryFromObj(obj, dnsVarValues(m))
				if obj == nil && len(raw) > 0 {
					sum = dnsServerSummaryFromInvalidRaw(raw)
				}
				// У группы показываем состав: выключенные участники
				// зачёркнуты — на сборке они из состава выпадут.
				sum += dnsGroupMembersSummary(obj, dnsEnabledByTag(m.DNSServers))
				tag := ""
				if obj != nil {
					tag = dnsJSONStringField(obj, "tag")
				}
				desc := ""
				if obj != nil {
					desc = strings.TrimSpace(dnsJSONStringField(obj, "description"))
				}
				// SPEC unify: "locked" (toggle block) — только required entries.
				// "templateOwned" (edit/del block) — любая template entry. Юзер
				// своего DNS-сервера → editable + deletable.
				locked := wizardbusiness.DNSTagLocked(m, tag)
				templateOwned := wizardbusiness.DNSTagFromTemplate(m, tag)

				// Не вызывать SyncModelToGUI здесь — он пересобирает весь список и все вкладки; только обновить селекты.
				sumLabel := ttwidget.NewLabel(sum)
				sumLabel.Truncation = fyne.TextTruncateClip
				cwc := fynewidget.NewCheckWithContent(func(checked bool) {
					// Полная пересборка только когда изменились ДРУГИЕ строки
					// (включение группы включает участников): она дороже, и
					// на каждый щелчок галкой её звать незачем.
					if setDNSServerEnabledAt(presenter, idx, checked) {
						presenter.RefreshDNSListAndSelects()
						return
					}
					presenter.RefreshDNSDependentSelectsOnly()
				}, sumLabel, fynewidget.CheckWithContentConfig{ContentToolTip: desc})
				enCheck := cwc.Check
				enCheck.SetChecked(wizardbusiness.DNSServerWizardEnabledRaw(raw))
				if locked {
					// required entry — toggle заблокирован (всегда вкл).
					enCheck.Disable()
				}
				setTooltip(enCheck, locale.T(tooltipForDNSServerCheck(locked)))

				rowGutter := components.NewScrollGutter()

				// SPEC unify: Edit/Del — только для user-added entries.
				// Template entries (read-only) получают View JSON вместо Edit.
				var right *fyne.Container
				if templateOwned {
					bodyCopy := obj
					viewBtn := fynewidget.NewHoverForwardButtonWithIcon("View", theme.SearchIcon(), func() {
						showTemplateDNSDetailsDialog(dialogParent(), bodyCopy, locked)
					}, rowGetter)
					viewBtn.Importance = widget.LowImportance
					// SPEC 109: у записи из шаблона тело править нельзя, но её
					// СОБСТВЕННЫЕ параметры (канал, адрес провайдера, профиль
					// Safe DNS) — можно и нужно: ради них параметризация и
					// переносилась. Кнопка только там, где параметры есть.
					if len(dnsServerVarsFor(presenter, tag)) > 0 {
						varsTag := tag
						varsBtn := fynewidget.NewHoverForwardButtonWithIcon(
							locale.T("wizard.dns.button_params"), theme.SettingsIcon(), func() {
								showDNSTemplateVarsDialog(presenter, dialogParent(), varsTag)
							}, rowGetter)
						varsBtn.Importance = widget.LowImportance
						right = container.NewHBox(varsBtn, viewBtn, rowGutter)
					} else {
						right = container.NewHBox(viewBtn, rowGutter)
					}
				} else {
					editBtn := fynewidget.NewHoverForwardButtonWithIcon(locale.T("wizard.shared.button_edit"), theme.DocumentCreateIcon(), func() {
						showDNSServerEditor(presenter, dialogParent(), idx)
					}, rowGetter)
					editBtn.Importance = widget.LowImportance
					delBtn := fynewidget.NewHoverForwardButtonWithIcon(locale.T("wizard.shared.button_del"), theme.DeleteIcon(), func() {
						deleteDNSServerAt(presenter, idx)
						presenter.RefreshDNSListAndSelects()
					}, rowGetter)
					delBtn.Importance = widget.LowImportance
					right = container.NewHBox(buildRowEditDelCluster(editBtn, delBtn), rowGutter)
				}
				// Border: check left, content center (tap/hover → check via fynewidget), buttons right — avoids zero-width label in HBox-only row.
				// Shared row tail (see row_scaffold.go); DNS servers have no ↑/↓ so
				// the left is the CheckWithContent leading, not buildRowLeftLead.
				row = finalizeRow(serversBox, cwc.CheckLeading, right, cwc.Content, sumLabel)
			}(i)
		}
		// Append bundled-preset rows в общий список (без заголовка — 🔒 в label
		// показывает source).
		for _, r := range bundledRows {
			serversBox.Add(r)
		}
		serversBox.Refresh()
	}
	guiState.RefreshDNSList = refreshList

	addBtn := widget.NewButton(locale.T("wizard.dns.button_add"), func() {
		showDNSServerAddDialog(presenter, dialogParent())
	})

	serversScroll := container.NewVScroll(serversBox)
	// Доля высоты окна, а не константа: при растягивании окна список
	// серверов растёт вместе с ним, а на низком экране не выпихивает
	// кнопки навигации под Dock (тот же механизм, что на других вкладках).
	// Минимум скромный: высоту список берёт растяжением в Border ниже, а
	// это лишь нижняя граница, чтобы на низком окне он не схлопнулся.
	serversScroll.SetMinSize(fyne.NewSize(0, 160))

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
				presenter.MarkAsChanged()
			}
			return
		}
		if mod.DefaultDomainResolver != v || mod.DefaultDomainResolverUnset {
			mod.DefaultDomainResolver = v
			mod.DefaultDomainResolverUnset = false
			presenter.MarkAsChanged()
		}
	}
	markStrategyChanged := func(value string) {
		v := strings.TrimSpace(value)
		if mod.DNSStrategy != v {
			mod.DNSStrategy = v
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
		presenter.MarkAsChanged()
	}
	rulesScroll := container.NewScroll(guiState.DNSRulesEntry)
	rulesScroll.Direction = container.ScrollBoth
	rulesHeight := canvas.NewRectangle(color.Transparent)
	// Тоже доля окна — см. serversScroll выше.
	rulesHeight.SetMinSize(adaptiveScrollSize(guiState, 0.26, 170))
	rulesBlock := container.NewStack(rulesHeight, rulesScroll)

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

	// SPEC: independent_cache deprecated в sing-box 1.14.0 (cache always keys
	// by transport name). UI checkbox удалён, поле не персистится, не эмитится.
	strategyAndCacheRow := container.NewHBox(
		strategyLabel,
		guiState.DNSStrategySelect,
	)

	// Final и default_domain_resolver — одна строка: две группы (лейбл+селект), spacer между ними.
	// Плоский HBox с одним Spacer между четырьмя виджетами даёт селектам нулевую ширину в Fyne.
	finalGroup := container.NewHBox(finalLabel, guiState.DNSFinalSelect)
	resolverGroup := container.NewHBox(resLabel, guiState.DNSDefaultResolverSelect)
	finalAndResolverRow := container.NewHBox(finalGroup, layout.NewSpacer(), resolverGroup)

	refreshList()

	// SPEC 062-F-N: единый ordered список DNS rules (preset + user
	// interleaved, drag ↑↓). Вместо двух разделённых секций bundled +
	// user — один VBox dispatch'ит по DNSRuleOrder.
	unifiedRulesBox := container.NewVBox()
	// rawRulesEntry — viewer для raw-JSON toggle (Phase 3 deferred mode).
	rawRulesEntry := widget.NewMultiLineEntry()
	rawRulesEntry.Wrapping = fyne.TextWrapOff
	rawRulesScroll := container.NewScroll(rawRulesEntry)
	rawRulesScroll.Direction = container.ScrollBoth
	rawRulesHeight := canvas.NewRectangle(color.Transparent)
	rawRulesHeight.SetMinSize(fyne.NewSize(0, 200))
	rawRulesBlock := container.NewStack(rawRulesHeight, rawRulesScroll)
	rawRulesBlock.Hide() // list view by default

	// rawJSONMode — toggle между list view и raw-JSON edit. Reflected
	// in the toggle button icon (DocumentCreateIcon → ViewRestoreIcon).
	rawJSONMode := false

	var refreshAll func()
	rebuildUnified := func() {
		unifiedRulesBox.Objects = unifiedRulesBox.Objects[:0]
		m := presenter.Model()
		// Reconcile defensively — if user added/removed presets in another
		// tab, DNSRuleOrder might have stale or missing slots.
		wizardmodels.ReconcileDNSRuleOrder(m)
		if len(m.DNSRuleOrder) == 0 {
			unifiedRulesBox.Add(widget.NewLabel(locale.T("wizard.dns.no_rules")))
		} else {
			buildUnifiedDNSRuleRows(presenter, m, dialogParent(), unifiedRulesBox, func() {
				if refreshAll != nil {
					refreshAll()
				}
			})
		}
		unifiedRulesBox.Refresh()
	}
	refreshAll = func() {
		if rawJSONMode {
			// Sync entry text from model when re-entering view.
			rawRulesEntry.SetText(wizardmodels.DNSUserRulesToText(presenter.Model().DNSUserRules))
		} else {
			rebuildUnified()
		}
	}

	previousRefresh := guiState.RefreshDNSList
	guiState.RefreshDNSList = func() {
		previousRefresh()
		refreshAll()
	}
	refreshAll()

	// Raw-JSON toggle — choice (b) from the spec: small icon button at the
	// top-right of the rules section. Click → swap unified list for the raw
	// multi-line editor with the current DNSUserRules serialized; click
	// again → parse text, replace DNSUserRules, rebuild DNSRuleOrder.
	toggleBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), nil)
	toggleBtn.Importance = widget.LowImportance
	setTooltip(toggleBtn, locale.T("wizard.dns.tooltip_toggle_raw_rules"))
	toggleBtn.OnTapped = func() {
		m := presenter.Model()
		if !rawJSONMode {
			// Switch to raw view: serialize current DNSUserRules.
			rawRulesEntry.SetText(wizardmodels.DNSUserRulesToText(m.DNSUserRules))
			unifiedRulesBox.Hide()
			rawRulesBlock.Show()
			rawJSONMode = true
			toggleBtn.SetIcon(theme.ViewRestoreIcon())
		} else {
			// Switch to list view: parse text, replace DNSUserRules, rebuild order.
			newRules := wizardmodels.DNSUserRulesFromText(rawRulesEntry.Text)
			m.DNSUserRules = newRules
			m.DNSRulesText = wizardmodels.DNSUserRulesToText(newRules)
			if gs := presenter.GUIState(); gs != nil && gs.DNSRulesEntry != nil {
				gs.DNSRulesEntry.SetText(m.DNSRulesText)
			}
			// Drop existing user slots and re-append in text order; preset
			// slots stay in their current positions. ReconcileDNSRuleOrder
			// won't add user slots because indices already exist — so do it
			// manually: filter out user-slots, then append fresh ones.
			kept := make([]wizardmodels.DNSRuleSlot, 0, len(m.DNSRuleOrder))
			for _, s := range m.DNSRuleOrder {
				if s.Kind != wizardmodels.DNSSlotKindUser {
					kept = append(kept, s)
				}
			}
			for i := range m.DNSUserRules {
				kept = append(kept, wizardmodels.DNSRuleSlot{
					Kind:  wizardmodels.DNSSlotKindUser,
					Index: i,
				})
			}
			m.DNSRuleOrder = kept
			presenter.MarkAsChanged()
			rawRulesBlock.Hide()
			unifiedRulesBox.Show()
			rawJSONMode = false
			toggleBtn.SetIcon(theme.DocumentCreateIcon())
			rebuildUnified()
		}
	}

	// [+ Add Rule] и [View All DNS Rules] кнопки внизу секции Rules.
	addRuleBtn := widget.NewButton("+ Add Rule", func() {
		showEditUserDNSRuleDialog(presenter, dialogParent(), -1, refreshAll)
	})
	addRuleBtn.Importance = widget.MediumImportance
	viewAllBtn := widget.NewButton("View All DNS Rules", func() {
		showViewAllDNSRulesDialog(presenter, dialogParent())
	})
	viewAllBtn.Importance = widget.LowImportance
	rulesButtons := container.NewHBox(addRuleBtn, layout.NewSpacer(), viewAllBtn)

	// Hide legacy MultiLineEntry rulesBlock — kept in code only for presenter
	// sync compat (SyncGUIToModel reads it on every Save). The toggle above
	// replaces it as the user-facing raw editor.
	_ = rulesBlock

	rulesHeader := container.NewHBox(rulesLabel, layout.NewSpacer(), toggleBtn)

	// Border, а не VBox: VBox даёт каждому элементу ровно его минимальную
	// высоту и не растягивает ни один — весь остаток высоты окна уходил в
	// пустоту под последней строкой. Здесь список серверов стоит в центре
	// и забирает всё, что осталось от шапки и нижнего блока, а список
	// правил внутри нижнего блока держит свою долю окна.
	//
	// Разделитель после serversScroll убран: VScroll рисует собственную
	// границу снизу, и своя линия сразу за ней давала две подряд.
	bottom := container.NewVBox(
		strategyAndCacheRow,
		widget.NewSeparator(),
		rulesHeader,
		unifiedRulesBox,
		rawRulesBlock,
		rulesButtons,
		widget.NewSeparator(),
		finalAndResolverRow,
	)
	return container.NewBorder(
		serversHeader,
		bottom,
		nil, nil,
		serversScroll,
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

// dnsVarValues — значения переменных шаблона: правки пользователя поверх
// дефолтов объявлений.
//
// SettingsVars несёт только то, что пользователь МЕНЯЛ; переменную, которую
// он не трогал, там не найти, и без отката на дефолт объявления строка
// показывала бы служебное «@dns_google_dot_dns_ip» вместо адреса.
func dnsVarValues(m *wizardmodels.WizardModel) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m.SettingsVars)+8)
	if m.TemplateData != nil {
		for _, v := range m.TemplateData.Vars {
			if def := v.DefaultValue.Scalar; def != "" {
				out[v.Name] = def
			}
		}
	}
	for k, v := range m.SettingsVars {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

// dnsResolvePlaceholder показывает ЗНАЧЕНИЕ переменной вместо её имени.
//
// Тела серверов UI берёт прямо из шаблона, где `@dns_google_dot_dns_ip` ещё
// не подставлен — подстановка живёт на пути сборки конфига. В строке списка
// пользователь должен видеть адрес, который реально уедет в конфиг, а не
// служебное имя переменной.
func dnsResolvePlaceholder(v string, vars map[string]string) string {
	if !strings.HasPrefix(v, "@") || vars == nil {
		return v
	}
	if val, ok := vars[strings.TrimPrefix(v, "@")]; ok && val != "" {
		return val
	}
	return v
}

func dnsServerSummaryFromObj(obj map[string]interface{}, vars map[string]string) string {
	if obj == nil {
		return locale.T("wizard.dns.invalid_server")
	}
	tag := dnsJSONStringField(obj, "tag")
	typ := dnsJSONStringField(obj, "type")
	server := dnsResolvePlaceholder(dnsJSONStringField(obj, "server"), vars)
	if tag == "" {
		tag = locale.T("wizard.dns.no_tag")
	}
	var sum string
	if server != "" {
		sum = fmt.Sprintf("%s  ·  %s  ·  %s", tag, typ, server)
	} else {
		sum = fmt.Sprintf("%s  ·  %s", tag, typ)
	}
	if det := strings.TrimSpace(dnsResolvePlaceholder(dnsJSONStringField(obj, "detour"), vars)); det != "" {
		sum += " [" + det + "]"
	}
	return sum
}

// dnsGroupMembersSummary — состав группы строкой, где ВЫКЛЮЧЕННЫЕ участники
// зачёркнуты.
//
// Без этого включённая группа выглядит рабочей, а на сборке ужимается до
// тех, кто реально попал в конфиг (pruneDNSGroupMembers): «Shield DNS
// (multi-provider)» из девяти провайдеров молча превращался в одного.
// Зачёркивание показывает это на месте, до сборки.
//
// U+0336 (combining long stroke overlay) — зачёркивание символами, а не
// стилем: строка списка рисуется одним Label, отдельного стиля на кусок
// текста у него нет.
func dnsGroupMembersSummary(obj map[string]interface{}, enabledByTag map[string]bool) string {
	if obj == nil || obj["type"] != "group" {
		return ""
	}
	members, ok := obj["servers"].([]interface{})
	if !ok || len(members) == 0 {
		return ""
	}
	parts := make([]string, 0, len(members))
	for _, m := range members {
		tag, _ := m.(string)
		if tag == "" {
			continue
		}
		if enabledByTag[tag] {
			parts = append(parts, tag)
			continue
		}
		parts = append(parts, strikeThrough(tag))
	}
	if len(parts) == 0 {
		return ""
	}
	// Двоеточие, а не «→»: стрелки нет в шрифте Fyne по умолчанию, и она
	// рисуется квадратами-заглушками (та же ловушка, что с «→» в строках
	// DNS-правил).
	return ":  " + strings.Join(parts, ", ")
}

// strikeThrough зачёркивает строку комбинирующим символом.
func strikeThrough(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(r)
		b.WriteRune('\u0336')
	}
	return b.String()
}

// dnsEnabledByTag — карта «тег → включён» по текущему списку серверов.
func dnsEnabledByTag(servers []json.RawMessage) map[string]bool {
	out := make(map[string]bool, len(servers))
	for _, raw := range servers {
		var o map[string]interface{}
		if json.Unmarshal(raw, &o) != nil {
			continue
		}
		tag, _ := o["tag"].(string)
		if tag == "" {
			continue
		}
		en := true
		if v, has := o["enabled"].(bool); has {
			en = v
		}
		out[tag] = en
	}
	return out
}

// dnsJSONStringField reads a string-like value from unmarshaled JSON (tag/type/server are strings in sing-box).

// setDNSServerEnabledAt переключает сервер. Возвращает true, если вместе с
// ним изменились ДРУГИЕ строки списка (включение группы включает её
// участников) — тогда вызывающий обязан перестроить список, а не только
// обновить селекты: иначе галки участников стоят в модели, но не на экране.
func setDNSServerEnabledAt(p *wizardpresentation.WizardPresenter, index int, enabled bool) bool {
	mod := p.Model()
	if index < 0 || index >= len(mod.DNSServers) {
		return false
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(mod.DNSServers[index], &obj); err != nil {
		return false
	}
	obj["enabled"] = enabled
	b, err := json.Marshal(obj)
	if err != nil {
		return false
	}
	mod.DNSServers[index] = json.RawMessage(b)
	p.MarkAsChanged()

	// SPEC 053: параллельно пишем override в model.DNSTemplateOverrides по tag'у.
	// На Save синхронизируется в state.DNS.TemplateServers через SyncDNSToStateV6.
	// Если у юзера есть preset-ref'ы → save переключится на v6, и override повлияет
	// через MergePresetsIntoDNS на финальный config.json::dns.servers[].
	if tag, ok := obj["tag"].(string); ok && tag != "" {
		if mod.DNSTemplateOverrides == nil {
			mod.DNSTemplateOverrides = make(map[string]bool)
		}
		mod.DNSTemplateOverrides[tag] = enabled
	}

	// SPEC 109: включение ГРУППЫ включает её участников.
	//
	// Группа опрашивает участников по тегу, а выключенный сервер в конфиг не
	// эмитится вовсе. Включённая группа с выключенными участниками — это
	// либо отвергнутый конфиг («dependency[x] not found»), либо, после
	// чистки состава, группа из одного случайно включённого сервера. Ни то
	// ни другое пользователь не имел в виду, ставя галку на «Shield DNS
	// (multi-provider)».
	//
	// Выключение группы участников НЕ трогает: они могли быть нужны сами по
	// себе, и снимать их за пользователя — потеря его настройки.
	if enabled && obj["type"] == "group" {
		return enableDNSGroupMembers(p, mod, obj)
	}
	return false
}

// enableDNSGroupMembers включает серверы, перечисленные в составе группы.
func enableDNSGroupMembers(p *wizardpresentation.WizardPresenter, mod *wizardmodels.WizardModel, group map[string]interface{}) bool {
	members, ok := group["servers"].([]interface{})
	if !ok || len(members) == 0 {
		return false
	}
	changed := false
	want := make(map[string]bool, len(members))
	for _, m := range members {
		if tag, ok := m.(string); ok && tag != "" {
			want[tag] = true
		}
	}
	for i, raw := range mod.DNSServers {
		var srv map[string]interface{}
		if json.Unmarshal(raw, &srv) != nil {
			continue
		}
		tag, _ := srv["tag"].(string)
		if tag == "" || !want[tag] {
			continue
		}
		if en, has := srv["enabled"].(bool); has && en {
			continue // уже включён
		}
		srv["enabled"] = true
		if b, err := json.Marshal(srv); err == nil {
			mod.DNSServers[i] = json.RawMessage(b)
		}
		if mod.DNSTemplateOverrides == nil {
			mod.DNSTemplateOverrides = make(map[string]bool)
		}
		mod.DNSTemplateOverrides[tag] = true
		changed = true
	}
	if changed {
		p.MarkAsChanged()
	}
	return changed
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
	if wizardbusiness.DNSTagFromTemplate(m, dnsJSONStringField(deleted, "tag")) {
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
	return container.NewStack(minH, scroll)
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
		if wizardbusiness.DNSTagFromTemplate(mod, dnsJSONStringField(cur, "tag")) {
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
	form := newDNSServerForm(p, "")
	form.tagEntry.SetText(uniqueDNSTag(p))
	showDNSServerDialog(p, w, form, nil, -1,
		locale.T("wizard.dns.dialog_add_title"), locale.T("wizard.dns.dialog_add_hint"))
}

// showDNSServerDialog — общее окно добавления и правки: вкладка «Настройки»
// с формой и вкладка «JSON» под ней.
//
// JSON остаётся запасным путём (SPEC 109): формы покрывают пять типов из
// двенадцати, и без него всё остальное — hosts, fakeip, dhcp, quic/h3 —
// стало бы ненастраиваемым. Для типа без формы вкладка «Настройки» не
// показывается вовсе, чтобы форма не переписала чужое тело своими полями.
func showDNSServerDialog(
	p *wizardpresentation.WizardPresenter,
	w fyne.Window,
	form *dnsServerForm,
	body map[string]interface{},
	editIndex int,
	title, hint string,
) {
	jsonEntry := widget.NewMultiLineEntry()
	jsonEntry.Wrapping = fyne.TextWrapOff

	formOK := true
	if body != nil {
		formOK = form.Load(body)
	}

	// Текст JSON всегда собирается из ФОРМЫ при переключении на вкладку:
	// иначе правки формы и правки JSON расходятся, и Save берёт то, что
	// лежало раньше.
	syncJSONFromForm := func() {
		if !formOK {
			return
		}
		if b, err := json.MarshalIndent(form.Collect(), "", "  "); err == nil {
			jsonEntry.SetText(string(b))
		}
	}
	if formOK {
		syncJSONFromForm()
	} else if body != nil {
		if b, err := json.MarshalIndent(body, "", "  "); err == nil {
			jsonEntry.SetText(string(b))
		}
	}

	hintLabel := widget.NewLabel(hint)
	hintLabel.Wrapping = fyne.TextWrapWord
	formTab := container.NewTabItem(locale.T("wizard.dns.tab_form"),
		container.NewVScroll(container.NewVBox(hintLabel, form.content)))
	jsonHint := widget.NewLabel(locale.T("wizard.dns.json_hint"))
	jsonHint.Wrapping = fyne.TextWrapWord
	jsonTab := container.NewTabItem(locale.T("wizard.dns.tab_json"),
		container.NewBorder(jsonHint, nil, nil, nil, dnsServerDialogJSONArea(jsonEntry)))

	var tabs *container.AppTabs
	if formOK {
		tabs = container.NewAppTabs(formTab, jsonTab)
	} else {
		// Тип без формы: только JSON, и сразу с пояснением почему.
		note := widget.NewLabel(locale.T("wizard.dns.form_unsupported_type"))
		note.Wrapping = fyne.TextWrapWord
		tabs = container.NewAppTabs(container.NewTabItem(
			locale.T("wizard.dns.tab_json"),
			container.NewBorder(note, nil, nil, nil, dnsServerDialogJSONArea(jsonEntry))))
	}
	tabs.OnSelected = func(ti *container.TabItem) {
		if ti == jsonTab {
			syncJSONFromForm()
		}
	}

	var dlg dialog.Dialog
	save := widget.NewButton(locale.T("wizard.dns.dialog_save"), func() {
		text := jsonEntry.Text
		// С вкладки «Настройки» источник истины — форма: пользователь мог
		// не открывать JSON вовсе, и там лежал бы снимок начального
		// состояния.
		if formOK && tabs.Selected() == formTab {
			b, err := json.Marshal(form.Collect())
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			text = string(b)
		}
		if applyDNSServerJSON(p, w, text, editIndex) && dlg != nil {
			dlg.Hide()
		}
	})
	cancel := widget.NewButton(locale.T("wizard.dns.dialog_cancel"), func() {
		if dlg != nil {
			dlg.Hide()
		}
	})

	buttons := container.NewHBox(layout.NewSpacer(), cancel, save)
	dlg = dialogs.NewCustom(title, tabs, buttons, "", w)
	dlg.Resize(fyne.NewSize(600, 560))
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
	if wizardbusiness.DNSTagFromTemplate(m, dnsJSONStringField(cur, "tag")) {
		return
	}
	form := newDNSServerForm(p, dnsJSONStringField(cur, "tag"))
	showDNSServerDialog(p, w, form, cur, index,
		locale.T("wizard.dns.dialog_title"), locale.T("wizard.dns.dialog_hint"))
}
