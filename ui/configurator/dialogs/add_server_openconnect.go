// File add_server_openconnect.go — вариант «OpenConnect» формы Add server и
// виджеты формы, общие с блоком правки готового узла (SPEC 167 §2.3).
//
// Раскладка и соответствие «поле ↔ путь тела» — из общей модели
// wizardbusiness.OpenConnectFormModel (по reg.Form("openconnect")); здесь
// только Fyne. Блок правки (tabs/source_openconnect_edit.go) собирает ту же
// OpenConnectForm, поэтому конструктор и правка не расходятся.
//
// Форма отдаёт ДОКУМЕНТ узла с одним endpoint в AddServerResult.ConfigJSON,
// как Tailscale: его разбирает тот же AppendManualConfigJSON. Адрес сервера
// форма не разбирает в ParsedNode.Server/Port — у openconnect нет
// server_port, и `https://` ядро дописывает само.
package dialogs

import (
	"encoding/json"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// openConnectClassicNoteText — вторая строка подсказки под паролем в
// классическом режиме: канала челленджа у `sing-box run` нет.
const openConnectClassicNoteText = "This launcher runs the core in classic mode: second-factor prompts are unavailable here"

// OpenConnectForm — виджеты формы узла OpenConnect.
type OpenConnectForm struct {
	model *wizardbusiness.OpenConnectFormModel
	// base — тело, поверх которого форма пишет свои пути (у нового узла
	// пустое, у готового — текущее тело узла).
	base map[string]interface{}

	entries map[string]*widget.Entry
	checks  map[string]*widget.Check
	selects map[string]*widget.Select
	// selectValues — подпись пункта выпадающего списка → значение enum.
	selectValues map[string]map[string]string
	// cells — ячейки строк поля (подпись, контрол, подсказки): прячутся
	// вместе по visible_when.
	cells map[string][]fyne.CanvasObject
	// advisory — строка предупреждения advisory под полем.
	advisory map[string]*widget.Label

	radios map[string]*widget.RadioGroup
	// optionByLabel — группа → подпись варианта → вариант.
	optionByLabel map[string]map[string]wizardbusiness.OpenConnectChoiceOption
	// optionCells — ячейки поля варианта (отпечаток): видны, когда вариант
	// выбран.
	optionCells map[string][]fyne.CanvasObject
	// optionAdvisory — предупреждение выбранного варианта (Do not verify).
	optionAdvisory map[string]*widget.Label

	box      *fyne.Container
	onChange func()
	syncing  bool
}

// NewOpenConnectForm собирает форму. onChange зовётся на каждую правку
// пользователя (не на программную загрузку).
func NewOpenConnectForm(onChange func()) (*OpenConnectForm, error) {
	model, err := wizardbusiness.NewOpenConnectFormModel()
	if err != nil {
		return nil, err
	}
	o := &OpenConnectForm{
		model:          model,
		base:           map[string]interface{}{},
		entries:        map[string]*widget.Entry{},
		checks:         map[string]*widget.Check{},
		selects:        map[string]*widget.Select{},
		selectValues:   map[string]map[string]string{},
		cells:          map[string][]fyne.CanvasObject{},
		advisory:       map[string]*widget.Label{},
		radios:         map[string]*widget.RadioGroup{},
		optionByLabel:  map[string]map[string]wizardbusiness.OpenConnectChoiceOption{},
		optionCells:    map[string][]fyne.CanvasObject{},
		optionAdvisory: map[string]*widget.Label{},
		onChange:       onChange,
	}
	classic := wizardbusiness.OpenConnectClassicBackend()

	var top []fyne.CanvasObject
	var items []*widget.AccordionItem
	openIdx := []int{}
	for _, blk := range model.Blocks {
		grid := container.New(layout.NewFormLayout())
		for _, c := range blk.Choices {
			o.addChoice(grid, c)
		}
		for _, f := range blk.Fields {
			o.addField(grid, f, classic && f.Path == "password")
		}
		if !blk.Collapsible {
			top = append(top, grid)
			continue
		}
		if blk.Expanded {
			openIdx = append(openIdx, len(items))
		}
		items = append(items, widget.NewAccordionItem(locale.T(blk.Title), grid))
	}
	objs := top
	if len(items) > 0 {
		acc := widget.NewAccordion(items...)
		acc.MultiOpen = true
		for _, i := range openIdx {
			acc.Open(i)
		}
		objs = append(objs, acc)
	}
	o.box = container.NewVBox(objs...)

	// Новый узел: первый шлюз списка (макет SPEC §2.3 — FortiGate) и
	// проверка сертификата по системным УЦ.
	o.Load(nil)
	return o, nil
}

// Object — корневой объект формы.
func (o *OpenConnectForm) Object() *fyne.Container { return o.box }

func (o *OpenConnectForm) changed() {
	if o.syncing {
		return
	}
	o.refresh()
	if o.onChange != nil {
		o.onChange()
	}
}

// hintLabel — пояснение под строкой.
func hintLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.LowImportance
	return l
}

// warnLabel — предупреждение advisory под строкой; скрыто, пока пусто.
func warnLabel() *widget.Label {
	l := widget.NewLabel("")
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.WarningImportance
	l.Hide()
	return l
}

// addField добавляет строки поля в сетку блока.
func (o *OpenConnectForm) addField(grid *fyne.Container, f *wizardbusiness.OpenConnectField, classicNote bool) {
	var cells []fyne.CanvasObject
	add := func(left, right fyne.CanvasObject) {
		grid.Add(left)
		grid.Add(right)
		cells = append(cells, left, right)
	}
	switch f.Kind {
	case wizardbusiness.OpenConnectKindBool:
		ch := widget.NewCheck(locale.T(f.Label), func(bool) { o.changed() })
		o.checks[f.Path] = ch
		add(widget.NewLabel(""), ch)
	case wizardbusiness.OpenConnectKindEnum:
		labels := make([]string, 0, len(f.Values))
		byLabel := map[string]string{}
		for _, v := range f.Values {
			l := wizardbusiness.OpenConnectFlavorLabel(v)
			labels = append(labels, l)
			byLabel[l] = v
		}
		sel := widget.NewSelect(labels, func(string) { o.changed() })
		o.selects[f.Path] = sel
		o.selectValues[f.Path] = byLabel
		add(widget.NewLabel(locale.T(f.Label)), sel)
	default:
		var e *widget.Entry
		if f.Secret {
			// Пароль с «глазом»: маскируется только показ, в теле узла он
			// лежит открыто («секреты в state — by design»).
			e = widget.NewPasswordEntry()
		} else {
			e = widget.NewEntry()
		}
		if f.Placeholder != "" {
			e.SetPlaceHolder(f.Placeholder)
		}
		e.OnChanged = func(string) { o.changed() }
		o.entries[f.Path] = e
		add(widget.NewLabel(locale.T(f.Label)), e)
	}
	if f.Hint != "" {
		add(widget.NewLabel(""), hintLabel(locale.T(f.Hint)))
	}
	if classicNote {
		add(widget.NewLabel(""), hintLabel(locale.T(openConnectClassicNoteText)))
	}
	adv := warnLabel()
	o.advisory[f.Path] = adv
	spacer := widget.NewLabel("")
	spacer.Hide()
	grid.Add(spacer)
	grid.Add(adv)
	// Строку advisory прячет refresh по значению, а не visible_when.
	o.cells[f.Path] = cells
}

// addChoice добавляет группу выбора: радиокнопки и поле варианта под ними.
func (o *OpenConnectForm) addChoice(grid *fyne.Container, c *wizardbusiness.OpenConnectChoice) {
	labels := make([]string, 0, len(c.Options))
	byLabel := map[string]wizardbusiness.OpenConnectChoiceOption{}
	for _, opt := range c.Options {
		l := locale.T(opt.Label)
		labels = append(labels, l)
		byLabel[l] = opt
	}
	o.optionByLabel[c.ID] = byLabel
	radio := widget.NewRadioGroup(labels, func(string) { o.changed() })
	radio.Required = true
	o.radios[c.ID] = radio
	grid.Add(widget.NewLabel(locale.T(c.Label)))
	grid.Add(radio)

	for _, opt := range c.Options {
		f := o.model.ChoiceField(c.ID, opt.ID)
		if f == nil {
			continue
		}
		e := widget.NewEntry()
		if f.Placeholder != "" {
			e.SetPlaceHolder(f.Placeholder)
		}
		e.OnChanged = func(string) { o.changed() }
		o.entries[f.Path] = e
		key := c.ID + "/" + opt.ID
		left := widget.NewLabel("")
		grid.Add(left)
		grid.Add(e)
		cells := []fyne.CanvasObject{left, e}
		if f.Hint != "" {
			hl, hr := widget.NewLabel(""), hintLabel(locale.T(f.Hint))
			grid.Add(hl)
			grid.Add(hr)
			cells = append(cells, hl, hr)
		}
		o.optionCells[key] = cells
		adv := warnLabel()
		o.advisory[f.Path] = adv
		spacer := widget.NewLabel("")
		spacer.Hide()
		grid.Add(spacer)
		grid.Add(adv)
	}
	adv := warnLabel()
	o.optionAdvisory[c.ID] = adv
	spacer := widget.NewLabel("")
	spacer.Hide()
	grid.Add(spacer)
	grid.Add(adv)
}

// Load заполняет форму телом узла; nil — новый узел.
func (o *OpenConnectForm) Load(body map[string]interface{}) {
	if body == nil {
		body = map[string]interface{}{}
	}
	o.base = body
	vals := o.model.FormFromBody(body)

	o.syncing = true
	for path, e := range o.entries {
		e.SetText(vals.Fields[path])
	}
	for path, ch := range o.checks {
		ch.SetChecked(vals.Fields[path] == "true")
	}
	for path, sel := range o.selects {
		v := vals.Fields[path]
		if v == "" {
			if len(body) == 0 && len(sel.Options) > 0 {
				// Новый узел: первое значение списка (макет — FortiGate).
				sel.SetSelected(sel.Options[0])
				continue
			}
			// Готовый узел без ключа: ядро берёт дефолт поля (anyconnect) —
			// его форма и показывает.
			if f := o.model.Field(path); f != nil {
				v = f.Default
			}
		}
		label := o.selectLabelOf(path, v)
		if label != "" && !containsString(sel.Options, label) {
			// Значение вне enum (пришло из JSON) — пунктом списка, иначе
			// Select его не покажет, а форма сняла бы ключ с тела.
			sel.Options = append(sel.Options, label)
		}
		sel.SetSelected(label)
	}
	for id, radio := range o.radios {
		radio.SetSelected(o.optionLabel(id, vals.Choices[id]))
	}
	o.syncing = false
	o.refresh()
}

func (o *OpenConnectForm) selectLabelOf(path, value string) string {
	for l, v := range o.selectValues[path] {
		if v == value {
			return l
		}
	}
	if value == "" {
		return ""
	}
	// Значение вне enum (пришло из JSON) показывается как есть.
	return wizardbusiness.OpenConnectFlavorLabel(value)
}

func (o *OpenConnectForm) optionLabel(choiceID, optionID string) string {
	for l, opt := range o.optionByLabel[choiceID] {
		if opt.ID == optionID {
			return l
		}
	}
	return ""
}

// SetBase меняет тело-основу, не трогая набранное: после записи формы в
// узел следующие правки ложатся поверх записанного.
func (o *OpenConnectForm) SetBase(body map[string]interface{}) {
	if body == nil {
		body = map[string]interface{}{}
	}
	o.base = body
}

// Values — что набрано в форме.
func (o *OpenConnectForm) Values() wizardbusiness.OpenConnectFormValues {
	v := wizardbusiness.OpenConnectFormValues{Fields: map[string]string{}, Choices: map[string]string{}}
	for path, e := range o.entries {
		v.Fields[path] = e.Text
	}
	for path, ch := range o.checks {
		if ch.Checked {
			v.Fields[path] = "true"
		}
	}
	for path, sel := range o.selects {
		if val, ok := o.selectValues[path][sel.Selected]; ok {
			v.Fields[path] = val
			continue
		}
		// Значение вне enum из JSON: «label (value)» → value.
		s := sel.Selected
		if i := strings.LastIndex(s, " ("); i >= 0 && strings.HasSuffix(s, ")") {
			s = s[i+2 : len(s)-1]
		}
		v.Fields[path] = s
	}
	for id, radio := range o.radios {
		if opt, ok := o.optionByLabel[id][radio.Selected]; ok {
			v.Choices[id] = opt.ID
		}
	}
	return v
}

// Body — тело узла: base с путями формы по набранному. Ошибка — набранное
// не разбирается (MTU не число).
func (o *OpenConnectForm) Body() (map[string]interface{}, error) {
	return o.model.BodyFromForm(o.base, o.Values())
}

// Verdict — проверка тела перед сохранением (обязательные поля, диапазоны,
// санитайзер реестра).
func (o *OpenConnectForm) Verdict(body map[string]interface{}) error {
	return o.model.Verdict(body)
}

// refresh пересчитывает видимость строк по visible_when и предупреждения
// advisory: при смене шлюза, радиокнопки и галок.
func (o *OpenConnectForm) refresh() {
	body, _ := o.Body()
	visible := o.model.Visible(body)
	for path, cells := range o.cells {
		show := visible[path]
		for _, c := range cells {
			if show {
				c.Show()
			} else {
				c.Hide()
			}
		}
	}
	vals := o.Values()
	for path, adv := range o.advisory {
		text := ""
		if f := o.model.Field(path); f != nil && (f.ChoiceID == "" && visible[path] || f.ChoiceID != "" && vals.Choices[f.ChoiceID] == f.OptionID) {
			text = o.model.AdvisoryText(path, vals.Fields[path])
		}
		setWarn(adv, text)
	}
	for id, byLabel := range o.optionByLabel {
		sel := vals.Choices[id]
		for _, opt := range byLabel {
			key := id + "/" + opt.ID
			for _, c := range o.optionCells[key] {
				if opt.ID == sel {
					c.Show()
				} else {
					c.Hide()
				}
			}
			if opt.ID == sel {
				setWarn(o.optionAdvisory[id], o.model.OptionAdvisoryText(opt))
			}
		}
		if sel == "" {
			setWarn(o.optionAdvisory[id], "")
		}
	}
	o.box.Refresh()
}

func setWarn(l *widget.Label, text string) {
	if l == nil {
		return
	}
	l.SetText(text)
	if text == "" {
		l.Hide()
		return
	}
	l.Show()
}

// openConnectDocument собирает документ узла: один endpoint.
func openConnectDocument(tag string, o *OpenConnectForm) ([]byte, string, error) {
	body, err := o.Body()
	if err != nil {
		return nil, "", err
	}
	if err := o.Verdict(body); err != nil {
		return nil, "", err
	}
	server, _ := body["server"].(string)
	tag = strings.TrimSpace(tag)
	if tag == "" {
		tag = wizardbusiness.OpenConnectDefaultTag(server)
	}
	endpoint := make(map[string]interface{}, len(body)+2)
	for k, v := range body {
		endpoint[k] = v
	}
	endpoint["type"] = wizardbusiness.OpenConnectScheme
	endpoint["tag"] = tag
	doc := map[string]interface{}{
		"endpoints": []interface{}{endpoint},
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, "", err
	}
	return raw, tag, nil
}

// previewOpenConnectDocument — превью документа узла через тот же разбор,
// что применит запись (WYSIWYG вкладки JSON).
func (f *addServerForm) previewOpenConnectDocument() (string, string) {
	if f.oc == nil {
		return "", ""
	}
	raw, _, err := openConnectDocument(f.tag.Text, f.oc)
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

// openConnectResult — результат кнопки Add для варианта OpenConnect.
func (f *addServerForm) openConnectResult() (AddServerResult, error) {
	if f.oc == nil {
		return AddServerResult{}, fmt.Errorf("%s", locale.T("OpenConnect form is unavailable"))
	}
	doc, tag, err := openConnectDocument(f.tag.Text, f.oc)
	if err != nil {
		return AddServerResult{}, err
	}
	return AddServerResult{ConfigJSON: doc, Label: tag}, nil
}

func containsString(list []string, s string) bool {
	for _, it := range list {
		if it == s {
			return true
		}
	}
	return false
}
