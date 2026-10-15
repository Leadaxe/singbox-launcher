// Package dialogs — диалоги визарда конфигурации.
//
// Файл add_server_dialog.go: форма ручного добавления источника. Две вкладки —
// «Параметры» (форма схемы по реестру, Tailscale либо Source: произвольный
// текст) и «JSON» (во что это превратится в config.json).
//
// Формы схем (VLESS … OpenConnect, SPEC 168) раскладываются по реестру —
// wizardbusiness.NodeFormModel, виджет node_form_view.go — и отдают ДОКУМЕНТ
// узла (тело после санитайзера и эмиттера реестра) в ConfigJSON: его
// разбирает тот же AppendManualConfigJSON, что и вручную набранный документ.
// Своих сборщиков share-URI и своих проверок у формы нет.
//
// Вкладка JSON — не имитация: превью формы — ParseNodeDocument →
// RenderNodeDocument, превью Source — config.EmitNodeJSONs, та же точка
// эмиссии, что и реальная сборка (WYSIWYG, как во вкладке JSON окна Source).
// Правка JSON побеждает: тронул руками — поля больше не перезаписывают текст,
// и в конфиг уходит именно он.
package dialogs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/registry"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	addServerTagNoteText    = "Shown as the server title in the Sources list. If empty, the host or the link fragment is used."
	addServerSourceNoteText = "Paste anything: share-URI (one per line), a sing-box outbound or config JSON, or [Interface]/[Peer] WireGuard conf."
	addServerJSONHintText   = "Preview of what this source unpacks into. Edit it and your version wins — the fields above stop overwriting it."
	addServerJSONDirtyText  = "Edited by hand — the fields no longer overwrite this JSON."
)

// AddServerResult — что форма отдала наружу. Ровно одно из полей непусто.
type AddServerResult struct {
	// Text — вход для общего пути Add: содержимое вкладки Source (URI-список,
	// JSON, INI — всё, что понимает Add).
	Text string
	// ConfigJSON — документ узла из формы (схема, Tailscale) либо
	// отредактированный вручную outbound: тогда его версия и есть истина.
	ConfigJSON []byte
	// Label — тег из верхнего поля; для ConfigJSON становится Label источника.
	Label string
}

// ShowAddServerDialog открывает форму ручного добавления источника. onResult
// получает результат в главном потоке Fyne.
//
// owner сохранён в сигнатуре ради вызывающих, но роли больше не играет:
// форма открывается самостоятельным окном, а не попапом поверх чужой канвы,
// и ошибки показывает на себе.
func ShowAddServerDialog(presenter *wizardpresentation.WizardPresenter, owner fyne.Window, onResult func(AddServerResult)) {
	_ = owner
	guiState := presenter.GUIState()
	if guiState == nil || guiState.Window == nil || onResult == nil {
		return
	}

	f := newAddServerForm()

	// Отдельное окно (Application.NewWindow), а НЕ модальный попап — тот же
	// довод, что у warp_dialog и preset_ref_edit: попап Fyne подтягивает
	// размер до Content.MinSize() и игнорирует Resize() как потолок, поэтому
	// высокая форма либо раздувает окно, либо вылезает за край без скролла.
	// Форма здесь высокая и растёт (вариант Tailscale — полтора десятка
	// строк), так что попап ей не подходит по устройству.
	controller := presenter.Controller()
	if controller == nil || controller.UIService == nil || controller.UIService.Application == nil {
		return
	}
	addWindow := controller.UIService.Application.NewWindow(locale.T("Add server"))

	submit := func() {
		res, err := f.result()
		if err != nil {
			dialog.ShowError(err, addWindow)
			return
		}
		addWindow.Close()
		onResult(res)
	}

	cancelButton := widget.NewButton(locale.T("Cancel"), func() { addWindow.Close() })
	addButton := widget.NewButton(locale.T("Add"), submit)
	addButton.Importance = widget.HighImportance
	buttons := container.NewHBox(layout.NewSpacer(), cancelButton, addButton)

	addWindow.SetContent(container.NewBorder(nil, buttons, nil, nil, f.container))
	addWindow.Resize(fyne.NewSize(640, 680))
	fynewidget.CenterOnScreen(addWindow)
	addWindow.SetCloseIntercept(func() { addWindow.Close() })
	addWindow.Show()
}

// addServerForm — состояние формы.
type addServerForm struct {
	container *fyne.Container

	tag *widget.Entry

	// Вкладка «Параметры».
	proto *widget.Select

	// Формы схем по реестру (SPEC 168): строятся при первом выборе схемы и
	// живут до закрытия окна — переключение типа набранное не теряет.
	forms  map[string]*NodeForm
	scheme string
	// formBox — место формы выбранной схемы.
	formBox *fyne.Container

	// Поля Tailscale.
	ts *tailscaleFields

	// formsScroll — прокручиваемая область с формой схемы или Tailscale.
	formsScroll *container.Scroll

	// source — многострочный ввод варианта Source.
	source    *widget.Entry
	sourceBox *fyne.Container

	// Вкладка «JSON».
	jsonView   fynewidget.JSONEditor
	jsonStatus *widget.Label

	mode addServerMode
	// jsonDirty — JSON тронут руками; поля больше его не перезаписывают.
	jsonDirty bool
	// syncing — идёт программная запись в jsonView, OnChanged не считать правкой.
	syncing bool
}

// addServerMode — что выбрано в «Параметрах».
type addServerMode int

const (
	// modeForm — форма схемы по реестру (addServerForm.scheme), отдаёт
	// ДОКУМЕНТ узла.
	modeForm addServerMode = iota
	// SPEC 122: узел tailnet — тоже ДОКУМЕНТ узла (тело + секции), см.
	// add_server_tailscale.go.
	modeTailscale
	modeSource
)

func newAddServerForm() *addServerForm {
	f := &addServerForm{}

	// Тег — общий для всех вариантов, поэтому стоит над вкладками.
	f.tag = widget.NewEntry()
	f.tag.SetPlaceHolder(locale.T("optional"))
	f.tag.OnChanged = func(string) { f.refreshJSON() }
	tagNote := widget.NewLabel(locale.T(addServerTagNoteText))
	tagNote.Wrapping = fyne.TextWrapWord

	f.buildParamsTab()
	// Gutter под вертикальной полосой — канонический паттерн проекта
	// (components.WrapInScrollWithGutter): Fyne рисует полосу ПОВЕРХ вьюпорта,
	// и без зарезервированной полосы справа она наезжает на правый край полей.
	// Заметнее всего на длинных строках варианта Tailscale.
	f.formsScroll = components.WrapInScrollWithGutter(container.NewVBox(f.formBox, f.ts.box))
	f.applyModeVisibility()
	f.buildJSONTab()

	tabs := container.NewAppTabs(
		container.NewTabItem(locale.T("Parameters"), f.paramsContent()),
		container.NewTabItem(locale.T("JSON"), f.jsonContent()),
	)
	tabs.OnSelected = func(ti *container.TabItem) {
		if ti.Text == locale.T("JSON") {
			f.refreshJSON()
		}
	}

	f.container = container.NewBorder(
		container.NewVBox(
			labeledRow(locale.T("Tag"), f.tag),
			tagNote,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		tabs,
	)
	f.refreshJSON()
	return f
}

// buildParamsTab собирает виджеты вкладки «Параметры».
//
// Список типов — SPEC 168 §2.1: формы схем в порядке
// wizardbusiness.NodeFormSchemes, затем Tailscale и Source. Схема, у которой
// реестр формы не описал, в список не попадает.
func (f *addServerForm) buildParamsTab() {
	tsLabel := locale.T("Tailscale")
	sourceLabel := locale.T("Source")

	// Source: многострочный ввод чего угодно, что понимает общий путь Add.
	f.source = widget.NewMultiLineEntry()
	f.source.Wrapping = fyne.TextWrapOff
	// Образцы того, что можно вставить, — сами схемы и ключи (vless://,
	// JSON-тело sing-box, заголовок секции wg-quick). Это синтаксис входных
	// форматов, переводить в нём нечего. l10n-exempt
	f.source.SetPlaceHolder("vless://…\n{\"type\":\"vless\",…}\n[Interface]…")
	f.source.OnChanged = func(string) { f.refreshJSON() }
	sourceNote := widget.NewLabel(locale.T(addServerSourceNoteText))
	sourceNote.Wrapping = fyne.TextWrapWord
	// Border со скроллом в центре: поле растягивается на всю высоту вкладки,
	// как на вкладке JSON. Без него MultiLineEntry внутри VBox схлопывается
	// до одной строки — VBox отдаёт ровно минимальную высоту.
	f.sourceBox = container.NewBorder(
		nil, sourceNote, nil, nil,
		container.NewScroll(f.source),
	)
	f.sourceBox.Hide()

	f.ts = buildTailscaleFields(func() { f.refreshJSON() })
	f.forms = map[string]*NodeForm{}
	f.formBox = container.NewVBox()

	reg, regErr := registry.Get()
	types := []string{}
	schemeByLabel := map[string]string{}
	for _, s := range wizardbusiness.NodeFormSchemes {
		if regErr != nil || reg.Form(s.Scheme) == nil {
			continue
		}
		l := locale.T(s.Label)
		types = append(types, l)
		schemeByLabel[l] = s.Scheme
	}
	types = append(types, tsLabel, sourceLabel)

	f.proto = widget.NewSelect(types, nil)
	f.proto.OnChanged = func(sel string) {
		switch sel {
		case tsLabel:
			f.mode = modeTailscale
		case sourceLabel:
			f.mode = modeSource
		default:
			f.mode = modeForm
			f.scheme = schemeByLabel[sel]
		}
		f.applyModeVisibility()
		f.refreshJSON()
	}
	// Дефолт — первый тип списка (VLESS).
	f.proto.SetSelected(types[0])
}

// currentNodeForm — форма выбранной схемы; строится при первом выборе. nil —
// у реестра нет формы схемы.
func (f *addServerForm) currentNodeForm() *NodeForm {
	if f.mode != modeForm || f.scheme == "" {
		return nil
	}
	if form, ok := f.forms[f.scheme]; ok {
		return form
	}
	form, err := NewNodeForm(f.scheme, wizardbusiness.NodeFormOptions{}, func() { f.refreshJSON() })
	if err != nil {
		f.forms[f.scheme] = nil
		return nil
	}
	f.forms[f.scheme] = form
	return form
}

// applyModeVisibility показывает блок, отвечающий выбранному варианту.
func (f *addServerForm) applyModeVisibility() {
	if f.formsScroll == nil {
		return // первый SetSelected до сборки скролла
	}
	f.sourceBox.Hide()
	f.ts.box.Hide()
	f.formBox.Objects = nil

	switch f.mode {
	case modeForm:
		f.formsScroll.Show()
		if form := f.currentNodeForm(); form != nil {
			f.formBox.Objects = []fyne.CanvasObject{form.Object()}
		}
		f.formBox.Refresh()
		f.formsScroll.ScrollToTop()
	case modeTailscale:
		// Адреса у узла tailnet нет — в tailnet он входит сам, по auth_key.
		f.formsScroll.Show()
		f.ts.box.Show()
	case modeSource:
		f.formsScroll.Hide()
		f.sourceBox.Show()
	}
}

func (f *addServerForm) paramsContent() fyne.CanvasObject {
	// Селектор закреплён сверху, всё остальное — внутри скролла.
	//
	// Формы в top у Border были ошибкой: эта область берёт полную высоту
	// содержимого и не сжимается, поэтому длинная форма (WireGuard — десять
	// строк) уезжала под кнопки диалога, и добраться до нижних полей было
	// нечем — скроллить в top нечего.
	//
	// Source живёт своей веткой: у него внутри уже есть скролл поля, и второй
	// поверх первого дал бы вложенную прокрутку с непредсказуемым захватом
	// колеса. Поэтому многострочник растягивается на центр напрямую.
	return container.NewBorder(
		container.NewVBox(
			labeledRow(locale.T("Server type"), f.proto),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		container.NewStack(f.formsScroll, f.sourceBox),
	)
}

// buildJSONTab собирает виджеты вкладки «JSON».
func (f *addServerForm) buildJSONTab() {
	f.jsonView = fynewidget.NewJSONEditor("")
	f.jsonView.SetOnChanged(func(text string) {
		// Программная синхронизация — не правка человека.
		if f.syncing {
			return
		}
		f.jsonDirty = true
		// Документ узла с `dns`/`route`/`sections`: сохранится только узел
		// (секции узла упразднены, контракт 1.1.85) — сказать это до Add.
		if dropped := manualDocDroppedKeys(text); len(dropped) > 0 {
			f.jsonStatus.SetText(wizardbusiness.NodeInputDroppedMessage())
			return
		}
		f.jsonStatus.SetText(locale.T(addServerJSONDirtyText))
	})
	f.jsonStatus = widget.NewLabel(locale.T(addServerJSONHintText))
	f.jsonStatus.Wrapping = fyne.TextWrapWord
}

func (f *addServerForm) jsonContent() fyne.CanvasObject {
	// Кнопка возврата к автогенерации: без неё ручная правка — дорога в один
	// конец, и человеку пришлось бы закрывать диалог, чтобы начать заново.
	reset := widget.NewButton(locale.T("Rebuild from fields"), func() {
		f.jsonDirty = false
		f.refreshJSON()
	})
	return container.NewBorder(
		nil,
		container.NewVBox(f.jsonStatus, reset),
		nil, nil,
		container.NewScroll(f.jsonView.Object()),
	)
}

// refreshJSON пересчитывает превью. Ручную правку не трогает — она победила.
func (f *addServerForm) refreshJSON() {
	if f.jsonDirty || f.jsonView == nil {
		return
	}
	text, status := f.previewJSON()
	f.syncing = true
	f.jsonView.SetText(text)
	f.syncing = false
	f.jsonStatus.SetText(status)
}

// previewJSON строит превью через ту же эмиссию, что и реальная сборка.
func (f *addServerForm) previewJSON() (string, string) {
	// SPEC 122: у Tailscale превью — ДОКУМЕНТ узла, тот же RenderNodeDocument,
	// которым документ рисует вкладка JSON окна источника.
	if f.mode == modeTailscale {
		return f.previewTailscaleDocument()
	}
	if f.mode == modeForm {
		return f.previewNodeFormDocument()
	}

	input := f.source.Text
	if strings.TrimSpace(input) == "" {
		return "", locale.T(addServerJSONHintText)
	}

	nodes := parseAddServerInput(input)
	if len(nodes) == 0 {
		return "", locale.T("Nothing recognized yet.")
	}

	docs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		outs, ep, eerr := config.EmitNodeJSONs(n)
		if eerr != nil {
			continue
		}
		if ep != "" {
			docs = append(docs, indentJSON(stripEmitted(ep)))
			continue
		}
		for _, o := range outs {
			docs = append(docs, indentJSON(stripEmitted(o)))
		}
	}
	if len(docs) == 0 {
		return "", locale.T("Nothing recognized yet.")
	}
	return strings.Join(docs, ",\n"), locale.Tf("Unpacked nodes: %d", len(docs))
}

// previewTailscaleDocument — превью документа узла tailnet.
//
// Документ прогоняется через ParseNodeDocument → RenderNodeDocument, то есть
// через тот же разбор, который применит запись: превью показывает не то, что
// форма написала, а то, что из него получится (WYSIWYG вкладки JSON).
func (f *addServerForm) previewTailscaleDocument() (string, string) {
	raw, err := tailscaleDocument(f.tag.Text, f.ts)
	if err != nil {
		return "", err.Error()
	}
	body, _, perr := config.ParseNodeDocument(raw)
	if perr != nil {
		return "", perr.Error()
	}
	text, rerr := config.RenderNodeDocument(body, config.NodeBodyGoesToEndpoints(body))
	if rerr != nil {
		return "", rerr.Error()
	}
	return text, locale.Tf("Unpacked nodes: %d", 1)
}

// parseAddServerInput разбирает вход превью: сначала как sing-box JSON, потом
// построчно как share-URI. Ошибки глотаются — это превью, оно обновляется на
// каждый символ и не должно кричать на недонабранную строку.
func parseAddServerInput(input string) []*config.ParsedNode {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil
	}

	if trimmed[0] == '{' || trimmed[0] == '[' {
		kind := subscription.ClassifySubscriptionBody(trimmed)
		switch kind {
		case subscription.BodyKindSingboxOutbound:
			if n, err := subscription.NodeFromManualConfigJSON([]byte(trimmed)); err == nil {
				return []*config.ParsedNode{n}
			}
			return nil
		case subscription.BodyKindSingboxOutboundArray,
			subscription.BodyKindSingboxConfig,
			subscription.BodyKindSingboxConfigArray:
			if res, err := subscription.ParseSingboxBody(trimmed, kind, nil); err == nil {
				return res.Nodes
			}
			return nil
		}
	}

	// WG-conf и share-URI: тот же порядок, что у общего пути Add.
	rest, blocks := subscription.ExtractWGConfBlocks(input)
	nodes := make([]*config.ParsedNode, 0, 4)
	for _, b := range blocks {
		// `.conf` разбирает секция реестра напрямую (SPEC 133).
		if n, err, known := subscription.ParseWGConfByEngine(b, nil); known && err == nil && n != nil {
			nodes = append(nodes, n)
		}
	}
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !subscription.IsDirectLink(line) {
			continue
		}
		// `vpn://` — все контейнеры профиля, как у Add (контракт 1.1.80).
		if subscription.IsAmneziaVPNLink(line) {
			if all, _, err := subscription.ParseAmneziaVPNLinkAll(line, nil); err == nil {
				nodes = append(nodes, all...)
			}
			continue
		}
		if n, err := subscription.ParseNode(line, nil); err == nil {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// result — что уходит наружу по кнопке Add.
func (f *addServerForm) result() (AddServerResult, error) {
	label := strings.TrimSpace(f.tag.Text)

	// Ручная правка JSON побеждает: в конфиг уходит она, а не поля.
	if f.jsonDirty {
		return manualJSONResult(f.jsonView.Text(), label)
	}

	// SPEC 122: Tailscale отдаёт документ узла — его разберёт тот же
	// AppendManualConfigJSON, что и вручную набранный документ.
	if f.mode == modeTailscale {
		doc, derr := tailscaleDocument(f.tag.Text, f.ts)
		if derr != nil {
			return AddServerResult{}, derr
		}
		return AddServerResult{ConfigJSON: doc, Label: tailscaleTag(label)}, nil
	}
	if f.mode == modeForm {
		return f.nodeFormResult()
	}

	text := strings.TrimSpace(f.source.Text)
	if text == "" {
		return AddServerResult{}, fmt.Errorf("%s", locale.T("Input is empty"))
	}
	return AddServerResult{Text: text, Label: label}, nil
}

// manualJSONResult решает, чем стал отредактированный вручную JSON.
//
// Одиночный объект уходит как ConfigJSON — так он сохраняется побайтово,
// включая поля, которых наш парсер не знает. Всё прочее (массив outbound'ов,
// целый конфиг) отдаётся общему пути Add: он это уже умеет.
func manualJSONResult(raw, label string) (AddServerResult, error) {
	body := strings.TrimSpace(raw)
	if body == "" {
		return AddServerResult{}, fmt.Errorf("%s", locale.T("JSON is empty"))
	}
	if strings.HasPrefix(body, "{") {
		// SPEC 121/122: документ узла с `dns`/`route`/`sections` — законная
		// форма ручной правки; его разберёт AppendManualConfigJSON тем же
		// ParseNodeDocument и возьмёт только узел (контракт 1.1.85).
		// Проверка здесь только на разбираемость, чтобы ошибка называлась на
		// кнопке Add, а не ниже по течению.
		//
		// Голый `{"outbounds":[…]}` формально документ тоже, но это давняя
		// многоузловая форма, и её по-прежнему разбирает общий путь Add.
		if config.IsNodeDocument([]byte(body)) && manualDocCarriesSections(body) {
			if _, _, err := config.ParseNodeDocument([]byte(body)); err != nil {
				return AddServerResult{}, err
			}
			var buf bytes.Buffer
			if err := json.Compact(&buf, []byte(body)); err != nil {
				return AddServerResult{}, err
			}
			return AddServerResult{ConfigJSON: buf.Bytes(), Label: label}, nil
		}
		kind := subscription.ClassifySubscriptionBody(body)
		// Целый конфиг ({"outbounds":[…]}) — законная многоузловая форма,
		// её разберёт общий путь Add. А одиночный объект, не признанный
		// outbound'ом, это чаще всего забытый "type": отвергаем здесь, иначе
		// он упадёт ниже по течению с куда менее внятным сообщением.
		if kind != subscription.BodyKindSingboxConfig {
			if _, err := subscription.NodeFromManualConfigJSON([]byte(body)); err != nil {
				return AddServerResult{}, err
			}
			var buf bytes.Buffer
			if err := json.Compact(&buf, []byte(body)); err != nil {
				return AddServerResult{}, err
			}
			return AddServerResult{ConfigJSON: buf.Bytes(), Label: label}, nil
		}
	}
	return AddServerResult{Text: body, Label: label}, nil
}

// manualDocCarriesSections — есть ли в документе ключ `sections`, `dns` или
// `route`. Такой документ разбирается как документ ОДНОГО узла (остальное
// отбрасывается); голый `{"outbounds":[…]}` остаётся давней многоузловой
// формой: её по-прежнему разбирает общий путь Add.
func manualDocCarriesSections(body string) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &probe); err != nil {
		return false
	}
	for _, key := range []string{"sections", "dns", "route"} {
		if _, has := probe[key]; has {
			return true
		}
	}
	return false
}

// manualDocDroppedKeys — ключи документа узла, содержимое которых не будет
// сохранено (`dns`, `route`, `sections` с непустым значением); nil, если текст
// не документ узла или не разбирается.
func manualDocDroppedKeys(text string) []string {
	body := strings.TrimSpace(text)
	if !strings.HasPrefix(body, "{") || !config.IsNodeDocument([]byte(body)) {
		return nil
	}
	_, dropped, err := config.ParseNodeDocument([]byte(body))
	if err != nil {
		return nil
	}
	return dropped
}

// stripEmitted снимает с эмитированной строки то, что делает её фрагментом
// конфига: строки-комментарии, ведущие табы и хвостовую запятую.
func stripEmitted(s string) string {
	lines := strings.Split(s, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "//") {
			continue
		}
		kept = append(kept, ln)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	return strings.TrimSuffix(out, ",")
}

// indentJSON приводит фрагмент к pretty-виду, сохраняя порядок полей эмиттера
// (json.Indent работает на токенах, в отличие от Unmarshal→MarshalIndent).
func indentJSON(s string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(s), "", "  "); err != nil {
		return s
	}
	return buf.String()
}
