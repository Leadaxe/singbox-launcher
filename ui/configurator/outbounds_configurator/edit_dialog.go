// edit_dialog.go provides the Add/Edit outbound dialog for the configurator.
// The dialog is shown as a separate window (like the Add Rule dialog).
package outbounds_configurator

import (
	"encoding/json"
	"fmt"
	"strings"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	fynetooltip "github.com/dweymouth/fyne-tooltip"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/textnorm"
	"singbox-launcher/ui/configurator/autogroupform"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardutils "singbox-launcher/ui/configurator/utils"
)

// ShowEditDialog opens a separate window to add or edit an outbound. existing may be nil for add.
// ParserConfig is taken from the model (editPresenter.Model()) so the dialog always uses current sources.
// onSave is called with the new config, scopeKind ("global" or "source") and sourceIndex (when scope is source).
// editPresenter is required (Model() is used to get ParserConfig); when set, only one Edit/Add window is allowed.
func ShowEditDialog(
	parent fyne.Window,
	editPresenter OutboundEditPresenter,
	existing *config.Direction,
	isGlobal bool,
	sourceIndex int,
	existingTags []string,
	onSave func(updated *config.Direction, scopeKind string, sourceIndex int),
) {
	if editPresenter != nil {
		if w := editPresenter.OpenOutboundEditWindow(); w != nil {
			w.RequestFocus()
			return
		}
	}
	parserConfig := getParserConfig(editPresenter.Model())
	if parserConfig == nil {
		dialog.ShowError(fmt.Errorf("%s", locale.T("wizard.outbound.error_config")), parent)
		return
	}
	isAdd := existing == nil
	dialogTitle := locale.T("wizard.outbound.title_edit")
	if isAdd {
		dialogTitle = locale.T("wizard.outbound.title_add")
	}

	// SPEC 058-R-N: для referenced entries (ref != "") body live из template/preset.
	// displayBody — это merged view (template body + active preset patches + USER patch
	// если есть). Используем для populate формы. Для direct entries — это просто
	// existing as-is.
	displayBody := existing
	if existing != nil && existing.Ref != "" && editPresenter != nil {
		// SPEC 058-R-N: для referenced entries (ref != "") body live из template/preset.
		// Используем тот же pipeline что Preview tab — wizardbusiness.ResolveMergedOutbound
		// сначала прогоняет sync + MergeOutboundUpdatesInPlace на копии parserConfig
		// (как parseAndPreview делает для emit), затем возвращает merged entry по tag.
		// Это устраняет дублирование merge-логики и гарантирует что dialog показывает
		// то же что увидит build pipeline.
		if merged := wizardbusiness.ResolveMergedOutbound(editPresenter.Model(), existing.Tag); merged != nil {
			displayBody = merged
		}
	}

	tagEntry := widget.NewEntry()
	if displayBody != nil {
		tagEntry.SetText(displayBody.Tag)
	}
	tagEntry.SetPlaceHolder(locale.T("wizard.outbound.placeholder_tag"))

	// SPEC 104: имя Направления. Отдельно от Comment намеренно — у
	// шаблонных записей комментарий описывает назначение абзацем, именем
	// его не сделать.
	labelEntry := widget.NewEntry()
	if displayBody != nil {
		labelEntry.SetText(displayBody.Label)
	}
	labelEntry.SetPlaceHolder(locale.T("wizard.outbound.placeholder_label"))

	// Новое Направление получает свободный тег и имя по умолчанию: тег —
	// цель правил, и придумывать его вручную незачем; имя пользователь
	// поправит, если захочет.
	if isAdd {
		nextTag := configtypes.NextDirectionTag(existingTags)
		tagEntry.SetText(nextTag)
		if n, ok := configtypes.DirectionNumber(nextTag); ok {
			labelEntry.SetPlaceHolder(configtypes.DefaultDirectionLabel(n))
		}
	} else {
		// Тег — цель правил: переименование сломало бы ссылки. Правится
		// только при создании.
		tagEntry.Disable()
	}

	// SPEC 088 load-balancing: mode/balancer парсим ЗАРАНЕЕ — он определяет, какой
	// из трёх типов показать (round_robin = отдельный пункт "loadbalance", хотя на
	// проводе это тот же type:urltest + mode:round_robin).

	// Type: три пункта. manual(selector) | auto(urltest, least_test) |
	// loadbalance(urltest + round_robin). Последние два — один wire-type urltest,
	// различаются наличием mode:round_robin.
	// SPEC 104: автовыбор — СВОЙСТВО Направления, а не его тип: галка
	// включает парную группу `<tag>-auto`, настройки которой живут на
	// отдельной вкладке. Смешивать это с типом записи было бы неверно —
	// у Направления тип всегда selector.
	autoTwinCheck := widget.NewCheck(locale.T("wizard.outbound.auto_twin_check"), nil)
	autoTwinCheck.SetChecked(displayBody != nil && displayBody.Auto != nil)

	// SPEC 104: Направление с автогруппой — та же настройка urltest, только
	// применяется к двойнику `<tag>-auto`, а не к самой записи.
	hasAutoTwin := func() bool { return autoTwinCheck.Checked }

	// Filters: fixed key "tag", value editable. Flag-picker button (🌐) opens
	// emoji picker dialog with live regex preview + match-count.
	filterKeyLabel := widget.NewLabel(locale.T("wizard.outbound.label_tag"))
	filterValEntry := widget.NewEntry()
	filterValEntry.SetPlaceHolder(locale.T("wizard.outbound.placeholder_filter"))

	// SPEC 104: пользователь вводит ТЕЛО регулярки, а не `/…/i` — писать
	// обёртку руками лишнее знание, а флаг регистра не выбор, а свойство:
	// имена узлов приходят из подписок в произвольном регистре.
	// Инверсия — переключаемая иконка «!» ПЕРЕД полем: она меняет смысл
	// всего выражения, и читается как знак перед ним, а не как действие
	// после. Сам Check остаётся носителем состояния (его читают сборка
	// cfg, пикер и сброс формы) — просто не показывается.
	filterInvertCheck := widget.NewCheck("", nil)
	filterInvertBtn := ttwidget.NewButton("!", nil)
	filterInvertBtn.SetToolTip(locale.T("wizard.outbound.filter_invert"))
	syncInvertBtn := func() {
		if filterInvertCheck.Checked {
			// Включённая инверсия меняет смысл всего отбора на обратный —
			// это должно быть видно, а не угадываться по подсказке.
			filterInvertBtn.Importance = widget.HighImportance
		} else {
			filterInvertBtn.Importance = widget.LowImportance
		}
		filterInvertBtn.Refresh()
	}
	syncInvertBtn()
	filterInvertBtn.OnTapped = func() {
		filterInvertCheck.SetChecked(!filterInvertCheck.Checked)
		syncInvertBtn()
	}
	filterPickerBtn := widget.NewButton("🌐", func() {
		var nodes []*config.ParsedNode
		if editPresenter != nil {
			if m := editPresenter.Model(); m != nil {
				// Same path as Preview tab: rebuild the preview cache before
				// reading PreviewNodes. Без этого кэш пуст, если юзер ещё не
				// открывал Preview tab, и picker показывает 0 нод.
				// best-effort: ошибка ребилда не блокирует picker, просто
				// возможно nodes окажется stale/empty (юзер увидит чипы 0
				// или пустой список).
				_, _ = wizardbusiness.RebuildPreviewCache(m)
				nodes = m.PreviewNodes
			}
		}
		showFlagPickerPopup(parent, nodes, filterValEntry.Text, filterInvertCheck.Checked,
			func(body string, invert bool) {
				filterValEntry.SetText(body)
				filterInvertCheck.SetChecked(invert)
				syncInvertBtn()
			})
	})
	filterPickerBtn.Importance = widget.LowImportance
	// Compose: [entry stretches] [button 30px].
	// SPEC 104: ссылка на справку по регуляркам с примерами — форма
	// принимает тело выражения, и пользователю негде узнать синтаксис.
	filterHelpBtn := ttwidget.NewButton("?", func() {
		if err := platform.OpenURL(directionFilterDocURL); err != nil {
			dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.outbound.error_open_docs"), err), parent)
		}
	})
	filterHelpBtn.Importance = widget.LowImportance
	filterHelpBtn.SetToolTip(locale.T("wizard.outbound.filter_help"))

	// Border, а не GridWithColumns: сетка делит ширину поровну, и между
	// узкой подписью «tag» и полем зияла половина диалога.
	filterValBox := container.NewBorder(nil, nil,
		container.NewHBox(filterKeyLabel, filterInvertBtn),
		container.NewHBox(filterPickerBtn, filterHelpBtn), filterValEntry)
	if displayBody != nil && displayBody.Filters != nil {
		body, invert := configtypes.DirectionFilterTag(displayBody.Filters)
		if body == "" {
			// Ключа `tag` нет — показываем первое строковое значение, чтобы
			// чужой фильтр (host, scheme) не выглядел пустым. Сохранение
			// перепишет его в канонический вид ключа `tag`, остальные ключи
			// останутся нетронутыми.
			for _, v := range displayBody.Filters {
				if s, ok := v.(string); ok {
					body, invert, _ = configtypes.DirectionFilterBody(s)
					break
				}
			}
		}
		filterValEntry.SetText(body)
		filterInvertCheck.SetChecked(invert)
		syncInvertBtn()
	}

	// Preferred default: fixed key "tag", value editable
	defKeyLabel := widget.NewLabel(locale.T("wizard.outbound.label_tag"))
	defValEntry := widget.NewEntry()
	defValEntry.SetPlaceHolder(locale.T("wizard.outbound.placeholder_preferred"))
	if displayBody != nil && displayBody.PreferredDefault != nil {
		body, _ := configtypes.DirectionFilterTag(displayBody.PreferredDefault)
		if body == "" {
			for _, v := range displayBody.PreferredDefault {
				if s, ok := v.(string); ok {
					body, _, _ = configtypes.DirectionFilterBody(s)
					break
				}
			}
		}
		defValEntry.SetText(body)
	}

	// AddOutbounds: direct-out, reject checkboxes + checkboxes for other tags
	directCheck := widget.NewCheck("direct-out", nil)
	// SPEC 104: вместо `reject` предлагаем тег блокировки из шаблона.
	// `reject` — это ACTION правила sing-box, а не outbound: положить его в
	// outbounds[] значит сослаться на несуществующий тег. Имя тега берём из
	// шаблона (`magic_nodes.block`) — оно не универсально.
	blockTag := directionBlockTag(editPresenter)
	blockCheck := widget.NewCheck(blockTag, nil)
	otherTagChecks := make([]*widget.Check, 0, len(existingTags))
	otherTagsMap := make(map[string]*widget.Check)
	for _, tag := range existingTags {
		c := widget.NewCheck(tag, nil)
		otherTagChecks = append(otherTagChecks, c)
		otherTagsMap[tag] = c
	}
	if displayBody != nil && len(displayBody.AddOutbounds) > 0 {
		for _, t := range displayBody.AddOutbounds {
			if t == "direct-out" {
				directCheck.SetChecked(true)
			} else if t == blockTag {
				blockCheck.SetChecked(true)
			} else if c, ok := otherTagsMap[t]; ok {
				c.SetChecked(true)
			}
		}
	}

	otherTagsBox := container.NewVBox()
	for _, c := range otherTagChecks {
		otherTagsBox.Add(c)
	}
	scrollOther := container.NewScroll(otherTagsBox)
	scrollOther.SetMinSize(fyne.NewSize(0, 80))

	// Raw tab: editable JSON (valid outbound object)
	initialConfig := existing
	if initialConfig == nil {
		initialConfig = &config.Direction{
			Tag:          "",
			Type:         "selector",
			Comment:      "",
			Options:      map[string]interface{}{"interrupt_exist_connections": true},
			AddOutbounds: nil,
		}
	}
	rawJSONBytes, _ := json.MarshalIndent(initialConfig, "", "  ")
	rawEntry := widget.NewMultiLineEntry()
	rawEntry.SetText(string(rawJSONBytes))
	rawEntry.Wrapping = fyne.TextWrapOff
	rawEntry.SetMinRowsVisible(16)
	rawScroll := container.NewScroll(rawEntry)
	rawScroll.SetMinSize(fyne.NewSize(400, 360))

	// Raw documentation button (opens ParserConfig.md "Секция outbounds")
	rawDocButton := widget.NewButton(locale.T("wizard.outbound.button_docs"), func() {
		docURL := "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/ParserConfig.md#%D1%81%D0%B5%D0%BA%D1%86%D0%B8%D1%8F-outbounds"
		if err := platform.OpenURL(docURL); err != nil {
			dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.outbound.error_open_docs"), err), parent)
		}
	})
	rawHeader := container.NewHBox(
		widget.NewLabel(locale.T("wizard.outbound.label_raw_json")),
		layout.NewSpacer(),
		rawDocButton,
	)
	rawContainer := container.NewBorder(
		rawHeader,
		nil,
		nil,
		nil,
		rawScroll,
	)

	// editSource — where the authoritative content currently lives:
	// "settings" or "raw". Preview tab is read-only and never updates this.
	//
	// Routing read/sync by `editSource` (not visible-tab) fixes scenarios
	// like Settings → Preview → Raw and Raw → Preview → Save: a stale form
	// must not overwrite raw, and Save from Preview must use whatever the
	// user typed last, not always the form path.
	var editSource string = "settings"

	var dialogWin fyne.Window

	// SPEC 108: Направление всегда глобальное. Выбор скоупа («для всех» /
	// «для подписки») из формы убран: локальная группа подписки создаётся
	// её собственной свёрткой, а не здесь, — иначе тем же диалогом можно
	// было бы завести группу старого формата, которую никто уже не
	// показывает и не настраивает. Сигнатура onSave сохранена: у неё
	// четыре точки вызова снаружи.
	getScopeFromForm := func() (scopeKind string, idx int) { return "global", -1 }
	// Списки допустимых значений для полей автогруппы. Берутся из
	// переменных шаблона (`@urltest_*`), чтобы пользователь мог выбрать
	// «наследовать из Settings», а не только конкретное число.
	curInterval, curTolerance, curURL := "", "", ""
	if displayBody != nil && displayBody.Auto != nil {
		a := displayBody.Auto
		curInterval = a.Interval
		if v := a.Tolerance.Value(); v != nil {
			curTolerance = fmt.Sprintf("%v", v)
		}
		curURL = a.URL
	}
	intervalLabels, intervalLabelToValue := templateVarChoices(editPresenter, "urltest_interval", curInterval)
	toleranceLabels, toleranceLabelToValue := templateVarChoices(editPresenter, "urltest_tolerance", curTolerance)
	urlLabels, _ := templateVarChoices(editPresenter, "urltest_url", curURL)

	// SPEC 104/108: вкладка «Автовыбор» — настройки парной группы
	// `<tag>-auto`. Виджеты и разметка живут в autogroupform: та же форма
	// нужна вкладке «Группа» свёрнутой подписки, и вторая её реализация
	// разъехалась бы с этой на первой же правке.
	autoModeLabel := ttwidget.NewLabel(locale.T("wizard.outbound.label_auto_mode"))
	autoModeLabel.SetToolTip(locale.T("wizard.outbound.auto_mode_tooltip"))

	autoForm := autogroupform.New(autogroupform.Choices{
		Interval:  autogroupform.VarChoices{Labels: intervalLabels, LabelToValue: intervalLabelToValue},
		Tolerance: autogroupform.VarChoices{Labels: toleranceLabels, LabelToValue: toleranceLabelToValue},
		URL:       autogroupform.VarChoices{Labels: urlLabels},
	})
	if displayBody != nil {
		autoForm.Load(displayBody.Auto)
	}

	// Подсказка ОБЯЗАНА переноситься: у Label без Wrapping минимальная
	// ширина равна длине строки, и она растягивает всё содержимое — поля
	// уезжают за правый край окна.
	autoHint := widget.NewLabel(locale.T("wizard.outbound.auto_tab_hint"))
	autoHint.Wrapping = fyne.TextWrapWord

	autoTabContent := autoForm.Content(autoHint, autoModeLabel)

	// buildConfigForPreview builds a config.Direction snapshot based on
	// the authoritative source (settings form or raw JSON). Routes by
	// `editSource`, not `currentTab` — preview tab itself doesn't host edits,
	// so when called from Preview we read from wherever the user last typed.
	//
	// `requireTag=true`: empty tag → error (save() needs a real tag).
	// `requireTag=false`: empty tag → autoinjected "_preview_" placeholder so
	// preview tab + syncFormToRaw work before the user has typed a name.
	buildConfigForPreview := func(requireTag bool) (*config.Direction, error) {
		if editSource == "raw" {
			var cfg config.Direction
			if err := json.Unmarshal([]byte(rawEntry.Text), &cfg); err != nil {
				return nil, fmt.Errorf("%s: %w", locale.T("wizard.outbound.error_invalid_json"), err)
			}
			if strings.TrimSpace(cfg.Tag) == "" {
				if requireTag {
					return nil, fmt.Errorf("%s", locale.T("wizard.outbound.error_tag_required"))
				}
				cfg.Tag = "_preview_"
			}
			return &cfg, nil
		}

		tag := strings.TrimSpace(tagEntry.Text)
		if tag == "" {
			if requireTag {
				return nil, fmt.Errorf("%s", locale.T("wizard.outbound.error_tag_required"))
			}
			tag = "_preview_"
		}
		// SPEC 104: Направление — всегда selector. Автовыбор задаётся полем
		// Auto (вкладка «Автовыбор») и разворачивается в парный urltest на
		// сборке; отдельного типа записи для него нет.
		const obType = "selector"

		cfg := &config.Direction{
			Tag:   tag,
			Type:  obType,
			Label: strings.TrimSpace(labelEntry.Text),
		}
		// SPEC 104: комментарий формой не правится (его роль взяло имя), но
		// и не теряется: у шаблонных записей это осмысленный текст, который
		// уедет в конфиг, если имя не задано.
		if displayBody != nil {
			cfg.Comment = displayBody.Comment
		}
		// SPEC 104: выключение — свойство записи, форма его не меняет
		// (переключатель живёт в списке), поэтому переносим как есть.
		if displayBody != nil {
			cfg.Disabled = displayBody.Disabled
		}
		if displayBody != nil && displayBody.Options != nil {
			cfg.Options = make(map[string]interface{})
			for k, v := range displayBody.Options {
				cfg.Options[k] = v
			}
			// A selector must not carry urltest-only keys. If the user switched
			// an edited urltest → selector, the wholesale copy above would leak
			// url/interval/tolerance into the selector and produce an invalid
			// config.json (sing-box rejects unknown selector fields).
			if obType == "selector" {
				delete(cfg.Options, "url")
				delete(cfg.Options, "interval")
				delete(cfg.Options, "tolerance")
			}
		} else {
			cfg.Options = map[string]interface{}{"interrupt_exist_connections": true}
		}

		// SPEC 104: Направление с автогруппой. Настройки берутся из тех же
		// виджетов, что и у самостоятельного urltest, но уезжают в Auto —
		// двойник разворачивается на сборке, в состоянии его нет.
		if hasAutoTwin() {
			cfg.Auto = autoForm.Collect()
		}

		// SPEC 104: форма отдаёт тело регулярки, на диск уезжает
		// канонический паттерн `/тело/i` (или `!/тело/i`). Чужие ключи
		// фильтра (host, scheme) сохраняются нетронутыми — их правят на
		// вкладке JSON.
		var baseFilters map[string]interface{}
		if displayBody != nil {
			baseFilters = displayBody.Filters
		}
		cfg.Filters = configtypes.SetDirectionFilterTag(
			baseFilters, filterValEntry.Text, filterInvertCheck.Checked)

		var baseDefault map[string]interface{}
		if displayBody != nil {
			baseDefault = displayBody.PreferredDefault
		}
		cfg.PreferredDefault = configtypes.SetDirectionFilterTag(
			baseDefault, defValEntry.Text, false)

		var addOb []string
		if directCheck.Checked {
			addOb = append(addOb, "direct-out")
		}
		if blockCheck.Checked {
			addOb = append(addOb, blockCheck.Text)
		}
		for _, c := range otherTagChecks {
			if c.Checked {
				addOb = append(addOb, c.Text)
			}
		}
		cfg.AddOutbounds = addOb

		return cfg, nil
	}

	// SPEC 058-R-N: applyEditedConfig.
	// Для direct entries (existing.Ref=="") — body inline, copy existing's Updates
	// (если есть юзерские правки накопленные — preserve).
	// Для referenced entries (existing.Ref!="") — вычисляем diff cfg → merged_base
	// и обновляем USER patch в updates[]. Body fields в cfg не идут в save (referenced
	// entries thin — body live из template/preset).
	applyEditedConfig := func(cfg *config.Direction) {
		if existing == nil {
			return
		}
		cfg.Ref = existing.Ref
		if cfg.Ref == "" {
			// Direct entry: preserve existing Updates (на случай legacy с USER patch).
			if len(existing.Updates) > 0 {
				cfg.Updates = append([]config.OutboundUpdate(nil), existing.Updates...)
			}
			return
		}
		// Referenced entry: diff cfg против merged_base без USER patch.
		var td *template.TemplateData
		var tgt template.TargetSpec
		if editPresenter != nil {
			if m := editPresenter.Model(); m != nil {
				td = m.TemplateData
				tgt = m.Target
			}
		}
		// merged_base = resolved template/preset body + active preset patches
		// (без USER patch — он и есть результат этого edit).
		baseEntry := *existing
		baseEntry.Updates = filterOutUserPatch(existing.Updates)
		mergedBase := build.MergeOutboundUpdates(baseEntry, td, tgt)
		// Referenced entry резолвится из template/preset ПО ТЕГУ. Переименование
		// рвёт эту связь: на следующей сборке lookup по новому тегу не найдёт
		// тело, и в config.json уедет заглушка с пустым type (sing-box бракует
		// весь конфиг). То же — уже осиротевшая ссылка (тег исчез из шаблона):
		// mergedBase.Type пуст, diff'ить не от чего. В обоих случаях
		// материализуем запись в direct: тело формы (то, что юзер видит и
		// сохраняет) становится inline, ссылка и patch-стек отбрасываются.
		if cfg.Tag != existing.Tag || (td != nil && strings.TrimSpace(mergedBase.Type) == "") {
			cfg.Ref = ""
			cfg.Updates = nil
			return
		}
		diff := build.OutboundFieldDiff(*cfg, mergedBase)
		// updates[] = existing preset patches + новый USER patch (или без него если diff пуст).
		cfg.Updates = build.UpsertUserPatch(
			append([]config.OutboundUpdate(nil), baseEntry.Updates...),
			diff,
		)
		// Strip body fields — referenced entries thin.
		stripDirectBodyForReferenced(cfg)
	}

	save := func() {
		// Route by editSource (where user actually typed) instead of currentTab.
		// Save from Preview tab must use whatever was last edited, not always
		// the form path.
		if editSource == "raw" {
			var cfg config.Direction
			if err := json.Unmarshal([]byte(rawEntry.Text), &cfg); err != nil {
				dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.outbound.error_invalid_json"), err), dialogWin)
				return
			}
			if strings.TrimSpace(cfg.Tag) == "" {
				dialog.ShowError(fmt.Errorf("%s", locale.T("wizard.outbound.error_tag_required")), dialogWin)
				return
			}
			scopeKind, idx := getScopeFromForm()
			// SPEC 057-R-N: Raw tab показывает ref/updates юзеру (они в JSON),
			// но юзерский edit мог их случайно изменить/удалить. Преимущество
			// state-managed полей: оверрайдим тем что в state, игнорируем raw edit.
			applyEditedConfig(&cfg)
			onSave(&cfg, scopeKind, idx)
			if dialogWin != nil {
				dialogWin.Close()
			}
			return
		}

		// Save → requireTag=true: explicit error if tag is empty.
		cfg, err := buildConfigForPreview(true)
		if err != nil {
			dialog.ShowError(err, dialogWin)
			return
		}
		scopeKind, idx := getScopeFromForm()

		// SPEC 057-R-N: preserve preset binding (Form tab их не показывает,
		// но они должны "пережить" Form-edit).
		applyEditedConfig(cfg)
		onSave(cfg, scopeKind, idx)
		if dialogWin != nil {
			dialogWin.Close()
		}
	}

	form := container.NewVBox(
		widget.NewLabel(locale.T("wizard.outbound.label_name")),
		labelEntry,
		widget.NewLabel(locale.T("wizard.outbound.label_tag_field")),
		tagEntry,
		autoTwinCheck,
		widget.NewLabel(locale.T("wizard.outbound.label_filters")),
		filterValBox,
		widget.NewLabel(locale.T("wizard.outbound.label_preferred")),
		container.NewBorder(nil, nil, defKeyLabel, nil, defValEntry),
		widget.NewLabel(locale.T("wizard.outbound.label_add_outbounds")),
		container.NewHBox(directCheck, blockCheck),
		scrollOther,
	)
	// Right margin inside scroll so the scrollbar does not overlap form elements
	const scrollbarGap = 20
	rightGap := canvas.NewRectangle(color.Transparent)
	rightGap.SetMinSize(fyne.NewSize(scrollbarGap, 0))
	formWithGap := container.NewBorder(nil, nil, nil, rightGap, form)
	widthSpacer := canvas.NewRectangle(color.Transparent)
	widthSpacer.SetMinSize(fyne.NewSize(400, 0))
	scrollContent := container.NewStack(widthSpacer, formWithGap)
	dialogScroll := container.NewScroll(scrollContent)
	dialogScroll.SetMinSize(fyne.NewSize(400, 400))

	// Preview tab: uses preview cache from the wizard model (via editPresenter.Model()).
	previewStatusLabel := widget.NewLabel(locale.T("wizard.outbound.preview_switch"))
	type previewRow struct {
		text  string
		color color.Color
	}
	var previewRows []previewRow
	previewList := widget.NewList(
		func() int { return len(previewRows) },
		func() fyne.CanvasObject { return canvas.NewText("", color.White) },
		func(id int, o fyne.CanvasObject) {
			if id < 0 || id >= len(previewRows) {
				return
			}
			if txt, ok := o.(*canvas.Text); ok {
				txt.Text = previewRows[id].text
				txt.Color = previewRows[id].color
			}
		},
	)
	previewListScroll := container.NewScroll(previewList)
	previewListScroll.SetMinSize(fyne.NewSize(400, 320))
	previewContent := container.NewBorder(
		previewStatusLabel,
		nil,
		nil,
		nil,
		previewListScroll,
	)

	buildPreview := func() {
		previewRows = nil
		previewList.Refresh()

		if editPresenter == nil {
			previewStatusLabel.SetText(locale.T("wizard.outbound.preview_no_presenter"))
			return
		}
		model := editPresenter.Model()
		if model == nil {
			previewStatusLabel.SetText(locale.T("wizard.outbound.preview_model_nil"))
			return
		}

		// Preview → requireTag=false: empty tag is fine, we substitute a
		// placeholder so the filter pipeline still runs and the user can see
		// which nodes match before naming the outbound.
		cfg, err := buildConfigForPreview(false)
		if err != nil {
			previewStatusLabel.SetText(locale.T("wizard.outbound.preview_invalid_json"))
			return
		}

		// SPEC 057-R-N: preview должен показывать final emit. Form/Raw отдают
		// base body (без Updates[] стека), но emit применяет patches от preset'ов.
		// Подмешиваем Updates от existing → merge → preview через final body.
		// Без этого preview proxy-out не отфильтрует RU ноды (filters лежат в
		// Updates[].patch, а cfg.Filters пуст), хотя в config.json фильтр сработает.
		if existing != nil && len(existing.Updates) > 0 {
			cfg.Updates = append([]config.OutboundUpdate(nil), existing.Updates...)
			var td *template.TemplateData
			var tgt template.TargetSpec
			if editPresenter != nil {
				if m := editPresenter.Model(); m != nil {
					td = m.TemplateData
					tgt = m.Target
				}
			}
			merged := build.MergeOutboundUpdates(*cfg, td, tgt)
			cfg = &merged
		}

		// Ensure preview cache is up to date.
		errorCount, err := wizardbusiness.RebuildPreviewCache(model)
		if err != nil {
			previewStatusLabel.SetText(locale.Tf("wizard.outbound.preview_cache_failed", err))
			return
		}
		allNodes := model.PreviewNodes
		if len(allNodes) == 0 {
			previewStatusLabel.SetText(locale.T("wizard.outbound.preview_no_nodes"))
			return
		}

		var filteredNodes []*config.ParsedNode
		var defaultTag string
		if model.ParserConfig != nil {
			filteredNodes, defaultTag = config.PreviewGlobalSelectorNodes(allNodes, model.ParserConfig.ParserConfig.Proxies, *cfg)
		} else {
			filteredNodes, defaultTag = config.PreviewSelectorNodes(allNodes, *cfg)
		}
		filteredSet := make(map[*config.ParsedNode]bool, len(filteredNodes))
		for _, n := range filteredNodes {
			filteredSet[n] = true
		}

		// Map node pointer to source label using PreviewNodesBySource and ParserConfig.
		sourceLabels := make(map[*config.ParsedNode]string)
		if model.ParserConfig != nil && model.PreviewNodesBySource != nil {
			for si, nodes := range model.PreviewNodesBySource {
				if si < 0 || si >= len(model.ParserConfig.ParserConfig.Proxies) {
					continue
				}
				proxy := model.ParserConfig.ParserConfig.Proxies[si]
				label := proxy.Source
				if label == "" {
					label = locale.T("wizard.outbound.label_source") + fmt.Sprintf("%d", si+1)
				}
				label = wizardutils.TruncateStringEllipsis(label, wizardutils.MaxLabelRunes, "...")
				for _, n := range nodes {
					sourceLabels[n] = label
				}
			}
		}

		// Build rows: default node first, then the rest in original allNodes order.
		defaultRows := make([]previewRow, 0)
		otherRows := make([]previewRow, 0, len(allNodes))

		for _, node := range allNodes {
			inSelector := filteredSet[node]
			isDefault := inSelector && node.Tag == defaultTag

			src := sourceLabels[node]
			if src == "" {
				src = locale.T("wizard.outbound.preview_unknown_source")
			}
			text := node.Tag
			if text == "" {
				if node.Label != "" {
					text = node.Label
				} else if node.Server != "" {
					text = fmt.Sprintf("%s:%d", node.Server, node.Port)
				} else {
					text = node.Scheme
				}
			}
			text = textnorm.NormalizeProxyDisplay(text)
			text = fmt.Sprintf("%s — %s", text, src)
			if isDefault {
				text = "[default] " + text
			}

			var rowColor color.Color
			switch {
			case isDefault:
				rowColor = color.RGBA{R: 0, G: 128, B: 255, A: 255} // blue
			case inSelector:
				rowColor = color.RGBA{R: 0, G: 160, B: 0, A: 255} // green
			default:
				rowColor = color.RGBA{R: 200, G: 0, B: 0, A: 255} // red
			}

			row := previewRow{text: text, color: rowColor}
			if isDefault {
				defaultRows = append(defaultRows, row)
			} else {
				otherRows = append(otherRows, row)
			}
		}

		previewRows = append(defaultRows, otherRows...)
		previewList.Refresh()

		status := locale.Tf("wizard.outbound.preview_status", len(allNodes), len(filteredNodes))
		if defaultTag != "" {
			status += locale.Tf("wizard.outbound.preview_default", defaultTag)
		}
		if len(cfg.AddOutbounds) > 0 {
			status += locale.Tf("wizard.outbound.preview_also_includes", strings.Join(cfg.AddOutbounds, ", "))
		}
		if errorCount > 0 {
			status += locale.Tf("wizard.outbound.preview_source_errors", errorCount)
		}
		previewStatusLabel.SetText(status)
	}

	// syncRawToForm parses the Raw tab JSON and updates Settings form fields (tag, type, comment, filters, etc.).
	// Called when user switches from Raw to Settings so the form reflects the raw JSON.
	//
	// SPEC 058-R-N: для referenced entries (cfg.Ref != "") Raw содержит thin
	// shape (tag+ref+updates без body) — populate из этого даст пустую форму.
	// Re-merge с template: build.MergeOutboundUpdates резолвит base body и
	// applies updates → получаем full merged view для populate.
	syncRawToForm := func() {
		var cfg config.Direction
		if err := json.Unmarshal([]byte(rawEntry.Text), &cfg); err != nil {
			return // invalid JSON: leave form as is
		}
		if strings.TrimSpace(cfg.Tag) == "" {
			return
		}
		// Re-merge для referenced entries — иначе форма обнуляется.
		display := cfg
		if cfg.Ref != "" && editPresenter != nil {
			if m := editPresenter.Model(); m != nil {
				display = build.MergeOutboundUpdates(cfg, m.TemplateData, m.Target)
			}
		}
		tagEntry.SetText(display.Tag)
		labelEntry.SetText(display.Label)

		// SPEC 104: в форму кладём ТЕЛО регулярки и признак инверсии.
		filterBody, filterInvert := configtypes.DirectionFilterTag(display.Filters)
		filterValEntry.SetText(filterBody)
		filterInvertCheck.SetChecked(filterInvert)
		syncInvertBtn()

		defBody, _ := configtypes.DirectionFilterTag(display.PreferredDefault)
		defValEntry.SetText(defBody)
		directCheck.SetChecked(false)
		blockCheck.SetChecked(false)
		for _, c := range otherTagChecks {
			c.SetChecked(false)
		}
		if len(display.AddOutbounds) > 0 {
			for _, t := range display.AddOutbounds {
				if t == "direct-out" {
					directCheck.SetChecked(true)
				} else if t == blockTag {
					blockCheck.SetChecked(true)
				} else if c, ok := otherTagsMap[t]; ok {
					c.SetChecked(true)
				}
			}
		}
		// SPEC 104: параметры автогруппы живут в Auto, а не в Options самой
		// записи, и правятся на вкладке «Автовыбор».
		autoTwinCheck.SetChecked(display.Auto != nil)
		autoForm.Load(display.Auto)
	}

	// syncFormToRaw — собирает Direction из текущего состояния формы
	// и кладёт его JSON в rawEntry. Вызывается при переключении Settings → Raw.
	//
	// SPEC 058-R-N: Raw view показывает SAVE-shape (что реально попадёт в state),
	// не resolved/merged body. Для referenced entries (ref != "") это означает:
	// thin tag+ref + Updates с USER patch (diff формы vs merged_base). Юзер видит
	// то же что и save(), без иллюзии full body.
	syncFormToRaw := func() {
		// Guard by editSource: if user's last edits were in raw (and now they
		// just returned to it via preview), preserve raw — don't overwrite
		// with a stale form snapshot. If editSource was "settings", form is
		// authoritative → push it into raw.
		if editSource != "settings" {
			return
		}
		// Settings → Raw sync: requireTag=false so an empty-tag form still
		// materializes a skeleton JSON in Raw (user can keep editing there).
		cfg, err := buildConfigForPreview(false)
		if err != nil || cfg == nil {
			return
		}
		// applyEditedConfig делает: для referenced — diff vs merged_base + USER
		// patch + strip body; для direct — preserve Updates + full body.
		applyEditedConfig(cfg)
		b, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return
		}
		rawEntry.SetText(string(b))
	}

	autoScroll := container.NewScroll(autoTabContent)
	autoScroll.SetMinSize(fyne.NewSize(400, 400))
	autoTabItem := container.NewTabItem(locale.T("wizard.outbound.tab_auto"), autoScroll)

	tabs := container.NewAppTabs(
		container.NewTabItem(locale.T("wizard.outbound.tab_settings"), dialogScroll),
		container.NewTabItem(locale.T("wizard.outbound.tab_raw"), rawContainer),
		container.NewTabItem(locale.T("wizard.outbound.tab_preview"), previewContent),
	)
	// SPEC 104: вкладка «Автовыбор» существует ровно пока стоит галка.
	// Вставляем второй — сразу после Settings, чтобы настройки двойника
	// были рядом с настройками самого Направления.
	syncAutoTab := func() {
		has := false
		for _, it := range tabs.Items {
			if it == autoTabItem {
				has = true
				break
			}
		}
		switch {
		case autoTwinCheck.Checked && !has:
			items := append([]*container.TabItem{tabs.Items[0], autoTabItem}, tabs.Items[1:]...)
			tabs.SetItems(items)
		case !autoTwinCheck.Checked && has:
			tabs.Remove(autoTabItem)
		}
	}
	syncAutoTab()
	// Обработчик ПОСЛЕ начального SetChecked — иначе сработал бы на нём.
	autoTwinCheck.OnChanged = func(bool) { syncAutoTab() }
	tabs.OnSelected = func(t *container.TabItem) {
		switch t.Text {
		case locale.T("wizard.outbound.tab_raw"):
			// Going TO raw. If editSource is "settings" → push form into raw
			// (syncFormToRaw has its own editSource=="settings" guard).
			// If editSource was already "raw" (returning via Preview), keep
			// raw as user left it.
			syncFormToRaw()
			editSource = "raw"
		case locale.T("wizard.outbound.tab_preview"):
			// Preview is read-only. Don't touch editSource. buildPreview
			// uses buildConfigForPreview, which routes by editSource.
			buildPreview()
		default:
			// Going TO settings. If editSource was "raw" → re-parse raw into
			// the form. If editSource was "settings" (returning via Preview),
			// the form is already correct — don't overwrite with possibly
			// stale rawEntry.
			if editSource == "raw" {
				syncRawToForm()
			}
			editSource = "settings"
		}
	}

	cancelBtn := widget.NewButton(locale.T("wizard.outbound.button_cancel"), func() {
		if dialogWin != nil {
			dialogWin.Close()
		}
	})
	saveBtn := widget.NewButton(locale.T("wizard.outbound.button_save"), func() { save() })

	buttonsContainer := container.NewHBox(
		layout.NewSpacer(),
		cancelBtn,
		saveBtn,
	)
	mainContent := container.NewBorder(
		nil,
		buttonsContainer,
		nil,
		nil,
		tabs,
	)

	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	dialogWin = app.NewWindow(dialogTitle)
	if editPresenter != nil {
		editPresenter.SetOutboundEditWindow(dialogWin)
		dialogWin.SetOnClosed(func() {
			fynetooltip.DestroyWindowToolTipLayer(dialogWin.Canvas())
			editPresenter.ClearOutboundEditWindow()
			editPresenter.UpdateChildOverlay()
		})
	}
	dialogWin.Resize(fyne.NewSize(440, 560))
	dialogWin.CenterOnScreen()
	// fynetooltip layer обязателен для tooltips на ttwidget виджетах в
	// отдельном окне — без него fyne-tooltip пишет "no tool tip layer
	// created for current overlay" и tooltips не показываются.
	dialogWin.SetContent(fynetooltip.AddWindowToolTipLayer(mainContent, dialogWin.Canvas()))
	dialogWin.Show()
	if editPresenter != nil {
		editPresenter.UpdateChildOverlay()
	}
}
