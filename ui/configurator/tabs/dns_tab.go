package tabs

import (
	"encoding/json"
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	dnsDialogViewHintParamsText = "This server is declared by the template: its body is fixed. Toggle it on or off with the checkbox in the list; Params below changes what it does let you set."
	dnsPlaceholderRulesText     = "{\n  \"rules\": [\n    {\"rule_set\":\"example\",\"server\":\"tag\"}\n  ]\n}"
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
		return "This row comes from template config.dns: it cannot be edited, removed, or toggled here. The checkbox shows inclusion in the generated DNS config (from saved state / template); when included, dns_options for the same tag can override the skeleton body." // l10n-key
	}
	return "Include this server in the generated sing-box DNS config. Disabled servers stay in the list and in state; rules and Final must use enabled tags only." // l10n-key
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

	// SPEC 173: «Add from template» — попап со всем каталогом шаблона
	// (отсутствующий сервер возвращается, имеющийся копируется своим),
	// «Add all defaults» — все отсутствующие сразу. Доступность кнопок
	// пересчитывает каждая пересборка списка.
	var addFromTemplateBtn, addAllDefaultsBtn *widget.Button
	syncTemplateButtons := func() {
		if addFromTemplateBtn == nil || addAllDefaultsBtn == nil {
			return
		}
		m := presenter.Model()
		for _, b := range []struct {
			btn *widget.Button
			on  bool
		}{
			{addFromTemplateBtn, len(wizardbusiness.DNSTemplateCatalog(m)) > 0},
			{addAllDefaultsBtn, len(wizardbusiness.DNSTemplateServersMissing(m)) > 0},
		} {
			if b.on {
				b.btn.Enable()
			} else {
				b.btn.Disable()
			}
		}
	}

	refreshList := func() {
		serversBox.Objects = serversBox.Objects[:0]
		m := presenter.Model()
		syncTemplateButtons()
		if len(m.DNSServers) == 0 {
			g := components.NewScrollGutter()
			serversBox.Add(container.NewHBox(
				widget.NewLabel(locale.T("No DNS servers.")),
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
		// Карта включённости инвариантна в пределах одной пересборки —
		// считается ДО цикла: внутри него она заново анмаршалила бы весь
		// список серверов на КАЖДУЮ строку (O(n²) на десятках строк).
		// Значения переменных — свои у каждого сервера (SPEC 129): их
		// объявления и выбор живут в записи, и считаются по тегу строки.
		enabledByTag := dnsEnabledByTag(m.DNSServers)
		// Удерживающие ссылки (SPEC 173) — тоже одним расчётом на пересборку:
		// шаблонный сервер, который что-то держит, удалить нельзя.
		holders := presenter.DNSServerHolders()

		for i := range m.DNSServers {
			func(idx int) {
				var row *fynewidget.HoverRow
				rowGetter := func() *fynewidget.HoverRow { return row }

				raw := m.DNSServers[idx]
				var obj map[string]interface{}
				if err := json.Unmarshal(raw, &obj); err != nil {
					obj = nil
				}
				sum := dnsServerSummaryFromObj(obj, dnsVarValuesFor(m, dnsJSONStringField(obj, "tag")))
				if obj == nil && len(raw) > 0 {
					sum = dnsServerSummaryFromInvalidRaw(raw)
				}
				// У группы показываем состав: выключенные участники
				// зачёркнуты — на сборке они из состава выпадут.
				sum += dnsGroupMembersSummary(obj, enabledByTag)
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

				// Метки в конце строки (SPEC 109). Узнать это иначе можно
				// было только открыв запись — а открывают далеко не каждую.
				//
				// 🔒 — запись объявлена шаблоном: тело неизменяемо. Тот же
				// глиф и тот же смысл, что у Направлений и пресетных правил.
				// ⚙ — у записи есть настраиваемые параметры (канал, адрес
				// провайдера, профиль). Тем же символом в приложении помечены
				// настройки в трёх других местах.
				if templateOwned {
					sum += "  🔒"
				}
				if tag != "" && len(dnsServerVarsFor(presenter, tag)) > 0 {
					sum += "  ⚙"
				}
				// 📎 — выключенный сервер нужен DNS-зеркалу правила: сборка
				// включит его принудительно (SPEC 172, TEMPLATE_LANG §6.11).
				if users := ruleDNSServerUsers(m, tag); len(users) > 0 && !wizardbusiness.DNSServerWizardEnabledRaw(raw) {
					sum += "  📎"
					note := fmt.Sprintf(locale.T("Used by rule %s: kept in the config while the rule needs it."), strings.Join(users, ", "))
					if desc != "" {
						desc += "\n"
					}
					desc += note
				}

				// Замок по удерживающим ссылкам (SPEC 173, как у LxBox) — у
				// любого сервера списка, шаблонного и своего.
				usedBy := holders[tag]
				if usedBy != "" {
					if desc != "" {
						desc += "\n"
					}
					desc += dnsUsedByText(usedBy)
				}

				// Не вызывать SyncModelToGUI здесь — он пересобирает весь список и все вкладки; только обновить селекты.
				sumLabel := ttwidget.NewLabel(sum)
				sumLabel.Truncation = fyne.TextTruncateClip
				// Обработчик вешается ПОСЛЕ SetChecked (ловушка
				// SetChecked→OnChanged): конструктор с готовым хэндлером
				// заставлял КАЖДУЮ пересборку списка звать
				// setDNSServerEnabledAt(idx, true) для включённых строк — а
				// с каскадом SPEC 109 это принудительно включало обратно
				// участников группы, которых пользователь выключил, и
				// запускало реентерабельную пересборку из середины цикла.
				cwc := fynewidget.NewCheckWithContent(nil, sumLabel, fynewidget.CheckWithContentConfig{ContentToolTip: desc})
				enCheck := cwc.Check
				enCheck.SetChecked(wizardbusiness.DNSServerWizardEnabledRaw(raw))
				reverting := false
				enCheck.OnChanged = func(checked bool) {
					if reverting {
						return
					}
					// Используемый включённый сервер не выключается (SPEC 173,
					// как у LxBox): галка активна, щелчок возвращает её и
					// говорит, кто держит. Ссылки — на момент щелчка: сервер мог
					// стать используемым после пересборки строки. Выключенный
					// используемый включается как обычно.
					if !checked {
						if by := presenter.DNSServerHolders()[tag]; by != "" && !locked {
							reverting = true
							enCheck.SetChecked(true)
							reverting = false
							showDNSLockedHint(dialogParent(), enCheck, fmt.Sprintf(locale.T("Locked: used by %s. Remove the reference first."), by))
							return
						}
					}
					// Полная пересборка только когда изменились ДРУГИЕ строки
					// (включение группы включает участников): она дороже, и
					// на каждый щелчок галкой её звать незачем.
					if setDNSServerEnabledAt(presenter, idx, checked) {
						presenter.RefreshDNSListAndSelects()
						return
					}
					presenter.RefreshDNSDependentSelectsOnly()
				}
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
					// Тем же окном, что и своя запись: форма заблокирована,
					// вкладка JSON на месте, параметры правятся кнопкой
					// внутри. Отдельный диалог «сырой JSON» убран — он и
					// создавал разницу между своим сервером и шаблонным.
					viewIdx := idx
					viewBtn := fynewidget.NewHoverForwardButtonWithIcon(locale.T("View"), theme.SearchIcon(), func() {
						showDNSServerEditor(presenter, dialogParent(), viewIdx)
					}, rowGetter)
					viewBtn.Importance = widget.LowImportance
					// SPEC 173 (D-139): шаблонный сервер удаляется, как свой, —
					// вернуть его можно «Add from template». Кроме `required`
					// и удерживаемого ссылкой: Del недоступен, подсказка
					// называет, кто держит.
					delBtn := fynewidget.NewHoverForwardTTButton(locale.T("Del"), func() {
						deleteDNSServerAt(presenter, idx)
						presenter.RefreshDNSListAndSelects()
					}, rowGetter)
					delBtn.Icon = theme.DeleteIcon()
					delBtn.Importance = widget.LowImportance
					if usedBy != "" {
						delBtn.Disable()
						delBtn.SetToolTip(dnsUsedByText(usedBy))
					}
					right = container.NewHBox(buildRowEditDelCluster(viewBtn, delBtn), rowGutter)
				} else {
					editBtn := fynewidget.NewHoverForwardButtonWithIcon(locale.TN(1, "Edit"), theme.DocumentCreateIcon(), func() {
						showDNSServerEditor(presenter, dialogParent(), idx)
					}, rowGetter)
					editBtn.Importance = widget.LowImportance
					delBtn := fynewidget.NewHoverForwardTTButton(locale.T("Del"), func() {
						deleteDNSServerAt(presenter, idx)
						presenter.RefreshDNSListAndSelects()
					}, rowGetter)
					delBtn.Icon = theme.DeleteIcon()
					delBtn.Importance = widget.LowImportance
					if usedBy != "" {
						delBtn.Disable()
						delBtn.SetToolTip(dnsUsedByText(usedBy))
					}
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

	addBtn := widget.NewButton(locale.T("Add"), func() {
		showDNSServerAddDialog(presenter, dialogParent())
	})
	addFromTemplateBtn = widget.NewButton(locale.T("Add from template"), nil)
	addFromTemplateBtn.OnTapped = func() {
		showDNSTemplateCatalogPopup(presenter, dialogParent(), addFromTemplateBtn)
	}
	addAllDefaultsBtn = widget.NewButton(locale.T("Add all defaults"), func() {
		added := false
		for _, c := range wizardbusiness.DNSTemplateServersMissing(presenter.Model()) {
			if wizardbusiness.AddDNSTemplateServer(presenter.Model(), c.Tag) {
				added = true
			}
		}
		if added {
			presenter.MarkAsChanged()
			presenter.RefreshDNSListAndSelects()
		}
	})
	setTooltip(addFromTemplateBtn, locale.T("Pick a template DNS server: a deleted one comes back with the template's defaults, one already in the list is added as your own copy."))
	setTooltip(addAllDefaultsBtn, locale.T("Return every deleted template DNS server, with the template's defaults."))
	syncTemplateButtons()

	serversScroll := container.NewVScroll(serversBox)
	// Доля высоты окна, а не константа: при растягивании окна список
	// серверов растёт вместе с ним, а на низком экране не выпихивает
	// кнопки навигации под Dock (тот же механизм, что на других вкладках).
	// Минимум скромный: высоту список берёт растяжением в Border ниже, а
	// это лишь нижняя граница, чтобы на низком окне он не схлопнулся.
	serversScroll.SetMinSize(fyne.NewSize(0, 160))

	serversLabel := widget.NewLabel(locale.T("DNS servers"))
	serversLabel.Importance = widget.MediumImportance
	serversHeader := container.NewHBox(serversLabel, layout.NewSpacer(), addFromTemplateBtn, addAllDefaultsBtn, addBtn)

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
	finalLabel := newTooltipLabel(varTitle(wizardmodels.VarDNSFinal, locale.T("Final DNS:")), finalTip)
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
	resLabel := newTooltipLabel(varTitle(wizardmodels.VarDNSDefaultDomainResolver, locale.T("Default domain resolver:")), resTip)
	setTooltip(guiState.DNSDefaultResolverSelect, varTooltip(wizardmodels.VarDNSDefaultDomainResolver))

	guiState.DNSRulesEntry = widget.NewMultiLineEntry()
	guiState.DNSRulesEntry.SetPlaceHolder(locale.T(dnsPlaceholderRulesText))
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
	// Константа, а не доля окна: доля считалась от высоты окна на момент
	// создания и держала минимум всего Мастера (см. wizardTabScrollMinHeight).
	rulesHeight.SetMinSize(fyne.NewSize(0, wizardTabScrollMinHeight))
	rulesBlock := container.NewStack(rulesHeight, rulesScroll)

	rulesLabel := widget.NewLabel(locale.T("Rules (JSON object with \"rules\" array)"))
	rulesLabel.Importance = widget.MediumImportance

	strategyTip := varTooltip(wizardmodels.VarDNSStrategy)
	strategyLabel := newTooltipLabel(varTitle(wizardmodels.VarDNSStrategy, locale.T("Strategy:")), strategyTip)

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

	cacheBlock := buildDNSCacheSettings(presenter)

	// Final и default_domain_resolver — одна строка: две группы (лейбл+селект), spacer между ними.
	// Плоский HBox с одним Spacer между четырьмя виджетами даёт селектам нулевую ширину в Fyne.
	finalGroup := container.NewHBox(finalLabel, guiState.DNSFinalSelect)
	resolverGroup := container.NewHBox(resLabel, guiState.DNSDefaultResolverSelect)
	finalAndResolverRow := container.NewHBox(finalGroup, layout.NewSpacer(), resolverGroup)

	// SPEC 166 волна 3: галка под резолвером — объектная форма
	// route.default_domain_resolver, поле disable_optimistic_cache. Глобальная
	// переменная с wizard_ui fix: Settings её не рисует, только эта вкладка.
	// Подпись и подсказка — литералы каталога переводов, совпадающие с
	// title/tooltip переменной в шаблоне (как у настроек кэша DNS).
	noStaleProgrammatic := false
	noStaleCheck := ttwidget.NewCheck(locale.T("Never serve stale answers for node hostnames"), func(b bool) {
		if noStaleProgrammatic {
			return
		}
		m := presenter.Model()
		if m == nil {
			return
		}
		v := "false"
		if b {
			v = "true"
		}
		if m.SettingsVars == nil {
			m.SettingsVars = make(map[string]string)
		}
		if cur, ok := m.SettingsVars[wizardmodels.VarDNSResolverNoStale]; ok && cur == v {
			return
		}
		m.SettingsVars[wizardmodels.VarDNSResolverNoStale] = v
		presenter.MarkAsChanged()
	})
	noStaleCheck.SetToolTip(locale.T("Resolve a node's hostname again once its TTL expires instead of dialing a cached address. Keeps nodes alive with providers that rotate addresses."))
	noStaleRow := container.NewHBox(layout.NewSpacer(), noStaleCheck)
	refreshNoStale := func() {
		m := presenter.Model()
		if m == nil || m.TemplateData == nil {
			return
		}
		tdl := m.TemplateData
		if _, ok := wizardtemplate.VarByName(tdl.Vars, wizardmodels.VarDNSResolverNoStale); !ok {
			noStaleRow.Hide()
			return
		}
		noStaleRow.Show()
		val := strings.TrimSpace(wizardtemplate.DisplaySettingValueFor(
			tdl.Vars, m.SettingsVars, tdl.RawTemplate, wizardmodels.VarDNSResolverNoStale, m.Target.Normalized()))
		noStaleProgrammatic = true
		noStaleCheck.SetChecked(val == "true")
		noStaleProgrammatic = false
	}
	refreshNoStale()
	if prevCacheRefresh := guiState.RefreshDNSCacheSettings; prevCacheRefresh != nil {
		guiState.RefreshDNSCacheSettings = func() {
			prevCacheRefresh()
			refreshNoStale()
		}
	} else {
		guiState.RefreshDNSCacheSettings = refreshNoStale
	}

	refreshList()

	// SPEC 062-F-N: единый ordered список DNS rules (preset + user
	// interleaved, drag ↑↓). Вместо двух разделённых секций bundled +
	// user — один VBox dispatch'ит по DNSRuleOrder.
	unifiedRulesBox := container.NewVBox()
	// rawRulesEntry — JSON-редактор для raw-JSON toggle (Phase 3 deferred mode).
	rawRulesEntry := fynewidget.NewJSONEditor("")
	rawRulesScroll := container.NewScroll(rawRulesEntry.Object())
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
		buildUnifiedDNSRuleRows(presenter, m, dialogParent(), unifiedRulesBox, func() {
			if refreshAll != nil {
				refreshAll()
			}
		})
		if len(unifiedRulesBox.Objects) == 0 {
			unifiedRulesBox.Add(widget.NewLabel(locale.T("No DNS rules.")))
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
	setTooltip(toggleBtn, locale.T("Toggle between unified list view and raw JSON editor (user rules only)."))
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
			newRules := wizardmodels.DNSUserRulesFromText(rawRulesEntry.Text())
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
	addRuleBtn := widget.NewButton(locale.T("+ Add Rule"), func() {
		showEditUserDNSRuleDialog(presenter, dialogParent(), -1, refreshAll)
	})
	addRuleBtn.Importance = widget.MediumImportance
	viewAllBtn := widget.NewButton(locale.T("View All DNS Rules"), func() {
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
		cacheBlock,
		widget.NewSeparator(),
		rulesHeader,
		unifiedRulesBox,
		rawRulesBlock,
		rulesButtons,
		widget.NewSeparator(),
		finalAndResolverRow,
		noStaleRow,
	)
	// Снаружи — VScroll: Scroll отдаёт содержимому max(его минимум, размер
	// окна), поэтому на высоком окне Border по-прежнему растягивает список
	// серверов, а минимум вкладки перестаёт быть минимумом всего Мастера.
	// Без него нижний блок (кэш, список правил без прокрутки, кнопки,
	// строка Final) плюс список серверов давали ~650pt, AppTabs брал этот
	// максимум по всем вкладкам, и окно нельзя было сжать ни на одной.
	return container.NewVScroll(container.NewBorder(
		serversHeader,
		bottom,
		nil, nil,
		serversScroll,
	))
}

func dnsServerSummaryFromInvalidRaw(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return locale.T("(invalid server JSON)")
	}
	const max = 64
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max]) + "…"
	}
	return s
}

// dnsVarValuesFor — значения, которые уедут в тело сервера tag (SPEC 129):
// переменные самого сервера — выбор из его записи, иначе умолчание для
// таргета модели; прочие имена — переменные шаблона (правки Settings поверх
// умолчаний). Те же правила, что у сборки (template.DNSServerVarValues):
// строка списка обязана показывать то, что реально уезжает в конфиг, а не
// служебное «@dns_ip».
func dnsVarValuesFor(m *wizardmodels.WizardModel, tag string) map[string]string {
	if m == nil || m.TemplateData == nil {
		return nil
	}
	td := m.TemplateData
	return wizardtemplate.DNSServerVarValues(td.DNSServerVars[tag], m.DNSTemplateVars[tag],
		td.Vars, m.SettingsVars, m.Target.Normalized())
}

// dnsResolvePlaceholder показывает ЗНАЧЕНИЕ переменной вместо её имени.
//
// Тела серверов UI берёт прямо из шаблона, где `@dns_ip` ещё не
// подставлен — подстановка живёт на пути сборки конфига. В строке списка
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
		return locale.T("(invalid server JSON)")
	}
	tag := dnsJSONStringField(obj, "tag")
	typ := dnsJSONStringField(obj, "type")
	server := dnsResolvePlaceholder(dnsJSONStringField(obj, "server"), vars)
	if tag == "" {
		tag = locale.T("(no tag)")
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
// dnsCatalogPopupMaxRows / dnsCatalogPopupWidth — размер попапа каталога:
// строк без прокрутки и ширина (кнопка уже строки «описание · тег»).
const (
	dnsCatalogPopupMaxRows = 12
	dnsCatalogPopupWidth   = 440
)

// showDNSTemplateCatalogPopup — попап «Add from template» под кнопкой
// (SPEC 173): весь каталог шаблона компактными строками «описание · тег».
// Отсутствующий в списке сервер возвращается записью шаблона с его
// умолчаниями; сервер, который уже в списке (справа — «in list»),
// добавляется копией — своим сервером с телом, которое он эмитит сейчас, и
// сразу открывается редактором. Попап — плоский widget.List, как у выбора
// detour.
func showDNSTemplateCatalogPopup(presenter *wizardpresentation.WizardPresenter, win fyne.Window, anchor fyne.CanvasObject) {
	if win == nil {
		return
	}
	choices := wizardbusiness.DNSTemplateCatalog(presenter.Model())
	if len(choices) == 0 {
		return
	}
	var popup *widget.PopUp
	list := widget.NewList(
		func() int { return len(choices) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Truncation = fyne.TextTruncateEllipsis
			mark := widget.NewLabel("")
			mark.Importance = widget.LowImportance
			return container.NewBorder(nil, nil, nil, mark, label)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(choices) {
				return
			}
			c := choices[id]
			row := obj.(*fyne.Container)
			label := row.Objects[0].(*widget.Label)
			mark := row.Objects[1].(*widget.Label)
			text := c.Title
			if text != c.Tag {
				text += " · " + c.Tag
			}
			label.SetText(text)
			if c.Listed {
				mark.SetText(locale.T("in list · copy"))
			} else {
				mark.SetText("")
			}
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		list.Unselect(id)
		if id < 0 || id >= len(choices) {
			return
		}
		popup.Hide()
		c := choices[id]
		m := presenter.Model()
		if !c.Listed {
			if wizardbusiness.AddDNSTemplateServer(m, c.Tag) {
				presenter.MarkAsChanged()
				presenter.RefreshDNSListAndSelects()
			}
			return
		}
		idx, ok := wizardbusiness.DNSTemplateServerCopy(m, c.Tag)
		if !ok {
			return
		}
		presenter.MarkAsChanged()
		presenter.RefreshDNSListAndSelects()
		showDNSServerEditor(presenter, win, idx)
	}

	popup = widget.NewPopUp(list, win.Canvas())
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	pos.Y += anchor.Size().Height
	pad := theme.Padding()
	rowH := list.CreateItem().MinSize().Height + pad
	n := len(choices)
	if n > dnsCatalogPopupMaxRows {
		n = dnsCatalogPopupMaxRows
	}
	h := float32(n)*rowH + pad
	if room := win.Canvas().Size().Height - pos.Y - pad; room > rowH && h > room {
		h = room
	}
	w := float32(dnsCatalogPopupWidth)
	// Попап прижат правым краем к кнопке: она стоит у правого края вкладки.
	if x := pos.X + anchor.Size().Width - w; x > 0 {
		pos.X = x
	}
	popup.Resize(fyne.NewSize(w, h))
	popup.ShowAtPosition(pos)
}

// dnsLockedHintDuration — сколько висит подсказка о запертой галке.
const dnsLockedHintDuration = 3 * time.Second

// showDNSLockedHint — лёгкая подсказка под объектом (не модальная, без
// кнопок, сама прячется): галка используемого сервера не снялась, и
// пользователь должен узнать почему — как SnackBar у LxBox.
func showDNSLockedHint(win fyne.Window, anchor fyne.CanvasObject, text string) {
	if win == nil || anchor == nil {
		return
	}
	label := widget.NewLabel(text)
	pop := widget.NewPopUp(label, win.Canvas())
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	pos.Y += anchor.Size().Height
	pop.ShowAtPosition(pos)
	go func() {
		<-time.After(dnsLockedHintDuration)
		fyne.Do(pop.Hide)
	}()
}

// dnsUsedByText — подсказка «кто держит сервер» по `used_by` удерживающей
// ссылки (state.DNSServerHolders, словарь контракта).
func dnsUsedByText(usedBy string) string {
	kind, src := usedBy, ""
	if i := strings.Index(usedBy, ":"); i >= 0 {
		kind, src = usedBy[:i], usedBy[i+1:]
	}
	switch kind {
	case "required":
		return locale.T("Required by the template: it cannot be deleted.")
	case "final":
		return locale.T("Used by the Final DNS setting: pick another server there to delete this one.")
	case "default_domain_resolver":
		return locale.T("Used by the default domain resolver: pick another server there to delete this one.")
	case "dns_rule":
		return locale.T("Used by a DNS rule: change or remove the rule to delete this server.")
	case "rule":
		return fmt.Sprintf(locale.T("Used by the DNS option of routing rule \"%s\": change it to delete this server."), src)
	case "group":
		return fmt.Sprintf(locale.T("Used by DNS group \"%s\": delete the group first."), src)
	case "server":
		return fmt.Sprintf(locale.T("Used as the domain resolver of DNS server \"%s\": delete that server first."), src)
	case "preset":
		return fmt.Sprintf(locale.T("Used by preset \"%s\": change its DNS server or turn it off to delete this one."), src)
	default:
		return fmt.Sprintf(locale.T("Used by %s."), usedBy)
	}
}

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
	delTag := dnsJSONStringField(deleted, "tag")
	// SPEC 173: шаблонный сервер удаляется, кроме `required` (удерживаемый
	// ссылкой отсекает кнопка: она недоступна).
	fromTemplate := wizardbusiness.DNSTagFromTemplate(m, delTag)
	if fromTemplate && wizardbusiness.DNSTagLocked(m, delTag) {
		return
	}
	m.DNSServers = append(m.DNSServers[:index], m.DNSServers[index+1:]...)
	if fromTemplate {
		wizardbusiness.ForgetDNSTemplateServer(m, delTag)
	}

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

func dnsServerDialogJSONArea(entry fyne.CanvasObject) fyne.CanvasObject {
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
		dialog.ShowError(fmt.Errorf("%s", locale.T("JSON must not be empty.")), w)
		return false
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("Invalid JSON"), err), w)
		return false
	}
	tag := dnsJSONStringField(obj, "tag")
	if tag == "" {
		dialog.ShowError(fmt.Errorf("%s", locale.T("Server must have a non-empty \"tag\".")), w)
		return false
	}
	mod := p.Model()
	if editIndex >= 0 && editIndex < len(mod.DNSServers) {
		var cur map[string]interface{}
		_ = json.Unmarshal(mod.DNSServers[editIndex], &cur)
		if wizardbusiness.DNSTagFromTemplate(mod, dnsJSONStringField(cur, "tag")) {
			dialog.ShowError(fmt.Errorf("%s", locale.T("This DNS server cannot be edited (template config.dns).")), w)
			return false
		}
	}
	// SPEC 173: тег сервера каталога занят и тогда, когда строку удалили, —
	// своя запись с ним сохранилась бы шаблонной (syncDNSServersOnly
	// различает вид по тегу шаблона).
	if wizardbusiness.DNSTagFromTemplate(mod, tag) {
		dialog.ShowError(fmt.Errorf("%s: %s", locale.T("This tag belongs to a template DNS server; return it with Add from template"), tag), w)
		return false
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
			dialog.ShowError(fmt.Errorf("%s: %s", locale.T("Duplicate tag"), tag), w)
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
		locale.T("Add DNS server"), locale.T("A new DNS server. The tag must be unique — rules and the DNS default refer to it."), false)
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
	readOnly bool,
) {
	// Шаблонная запись (readOnly) сохранять нечего — её JSON только
	// читается, поэтому там просмотрщик, а не редактор.
	var jsonEditor fynewidget.JSONEditor
	var jsonViewer fynewidget.JSONView
	var jsonArea fyne.CanvasObject
	if readOnly {
		jsonViewer = fynewidget.NewJSONView("")
		jsonArea = jsonViewer.Object()
	} else {
		jsonEditor = fynewidget.NewJSONEditor("")
		jsonArea = jsonEditor.Object()
	}
	setJSONText := func(s string) {
		if jsonEditor != nil {
			jsonEditor.SetText(s)
		} else {
			jsonViewer.SetText(s)
		}
	}

	formOK := true
	if body != nil {
		formOK = form.Load(body)
	}

	// Текст JSON собирается из ФОРМЫ при переключении на вкладку — но
	// только пока пользователь сам его не правил: снимок формы поверх
	// живых правок JSON молча стирал их («заглянул на Настройки и
	// вернулся» — правки заменены). Правка JSON делает его источником
	// истины до конца сеанса окна.
	jsonDirty := false
	lastSyncedJSON := ""
	if jsonEditor != nil {
		jsonEditor.SetOnChanged(func(s string) {
			if s != lastSyncedJSON {
				jsonDirty = true
			}
		})
	}
	syncJSONFromForm := func() {
		if !formOK || jsonDirty {
			return
		}
		if b, err := json.MarshalIndent(form.Collect(), "", "  "); err == nil {
			lastSyncedJSON = string(b)
			setJSONText(lastSyncedJSON)
		}
	}
	if formOK {
		syncJSONFromForm()
	} else if body != nil {
		if b, err := json.MarshalIndent(body, "", "  "); err == nil {
			lastSyncedJSON = string(b)
			setJSONText(lastSyncedJSON)
		}
	}

	hintLabel := widget.NewLabel(hint)
	hintLabel.Wrapping = fyne.TextWrapWord

	formBody := container.NewVBox(hintLabel)
	// SPEC 109: у шаблонной записи параметры — ЕДИНСТВЕННОЕ, что можно
	// менять. Прятать их за кнопку «Params» значит требовать лишний клик
	// ради того, за чем окно и открывают; тело же, наоборот, только
	// читается. Поэтому параметры идут сверху, до неизменяемых полей.
	if readOnly {
		if tag, _ := body["tag"].(string); tag != "" {
			if rows := dnsTemplateVarRows(p, tag); rows != nil {
				formBody.Add(rows)
				formBody.Add(widget.NewSeparator())
			}
		}
	}
	formBody.Add(form.content)

	formTab := container.NewTabItem(locale.T("Settings"),
		container.NewVScroll(formBody))
	jsonHint := widget.NewLabel(locale.T("The same entry as raw JSON, in sing-box dns.servers form. Types without a form are edited here."))
	jsonHint.Wrapping = fyne.TextWrapWord
	jsonTab := container.NewTabItem(locale.T("JSON"),
		container.NewBorder(jsonHint, nil, nil, nil, dnsServerDialogJSONArea(jsonArea)))

	var tabs *container.AppTabs
	if formOK {
		tabs = container.NewAppTabs(formTab, jsonTab)
	} else {
		// Тип без формы: только JSON, и сразу с пояснением почему.
		note := widget.NewLabel(locale.T("This server type has no form yet — edit it as JSON. The form covers udp, tcp, tls (DoT), https (DoH) and group."))
		note.Wrapping = fyne.TextWrapWord
		tabs = container.NewAppTabs(container.NewTabItem(
			locale.T("JSON"),
			container.NewBorder(note, nil, nil, nil, dnsServerDialogJSONArea(jsonArea))))
	}
	tabs.OnSelected = func(ti *container.TabItem) {
		if ti == jsonTab {
			syncJSONFromForm()
		}
	}

	// Отдельное окно, а не модальный диалог (как у правки DNS-правила,
	// dns_user_rules.go): диалог живёт внутри канвы родителя и не может
	// быть ни выше её, ни сдвинут. У формы сервера десяток полей, у группы
	// ещё и список участников — в модальном окне это упирается в потолок
	// визарда, а рядом с ним подсмотреть список серверов уже нельзя.
	controller := p.Controller()
	if controller == nil || controller.UIService == nil {
		return
	}
	editWin := controller.UIService.Application.NewWindow(title)

	if readOnly {
		form.SetReadOnly()
	}

	save := widget.NewButton(locale.T("Save"), func() {
		// readOnly-окно кнопки «Сохранить» не показывает (см. buttons ниже).
		if jsonEditor == nil {
			return
		}
		text := jsonEditor.Text()
		// С вкладки «Настройки» источник истины — форма: пользователь мог
		// не открывать JSON вовсе, и там лежал бы снимок начального
		// состояния.
		if formOK && tabs.Selected() == formTab {
			b, err := json.Marshal(form.Collect())
			if err != nil {
				dialog.ShowError(err, editWin)
				return
			}
			text = string(b)
		}
		if applyDNSServerJSON(p, editWin, text, editIndex) {
			editWin.Close()
		}
	})
	save.Importance = widget.HighImportance
	cancel := widget.NewButton(locale.T("Cancel"), func() {
		editWin.Close()
	})

	buttons := container.NewHBox(layout.NewSpacer(), cancel, save)
	if readOnly {
		// Шаблонная запись: сохранять нечего — тело живёт в шаблоне.
		// Правятся только её собственные параметры, и кнопка на них здесь
		// же, чтобы не искать её в списке.
		cancel.SetText(locale.T("Close"))
		buttons = container.NewHBox(layout.NewSpacer(), cancel)
	}
	editWin.SetContent(container.NewBorder(nil, buttons, nil, nil, tabs))
	editWin.Resize(fyne.NewSize(660, 620))
	fynewidget.CenterOnScreen(editWin)
	editWin.Show()
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
	// Шаблонная запись открывается ТЕМ ЖЕ окном, только в режиме чтения:
	// раньше она уходила в отдельный диалог с сырым JSON, и один и тот же
	// сервер выглядел формой у себя и текстом у шаблона.
	if wizardbusiness.DNSTagFromTemplate(m, dnsJSONStringField(cur, "tag")) {
		form := newDNSServerForm(p, dnsJSONStringField(cur, "tag"))
		// Подсказка по факту: обещать «Параметры» там, где их нет, значит
		// отправить пользователя искать кнопку, которой не будет.
		hint := locale.T("This server is declared by the template: its body is fixed. Toggle it on or off with the checkbox in the list.")
		if len(dnsServerVarsFor(p, dnsJSONStringField(cur, "tag"))) > 0 {
			hint = locale.T(dnsDialogViewHintParamsText)
		}
		showDNSServerDialog(p, w, form, cur, -1,
			locale.T("DNS server (from template)"), hint, true)
		return
	}
	form := newDNSServerForm(p, dnsJSONStringField(cur, "tag"))
	showDNSServerDialog(p, w, form, cur, index,
		locale.T("Edit DNS server"), locale.T("Edit the DNS server. Its tag is what rules and the DNS default refer to."), false)
}
