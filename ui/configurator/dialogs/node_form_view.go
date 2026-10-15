// File node_form_view.go — виджет формы узла по реестру (SPEC 167 §2.3,
// обобщён SPEC 168 §2.2): вариант схемы в «Add server» и блок правки у
// готового узла.
//
// Раскладка и соответствие «поле ↔ путь тела» — из общей модели
// wizardbusiness.NodeFormModel (по reg.FormFields схемы); здесь только Fyne.
// Блок правки (tabs/source_form_edit.go) собирает ту же NodeForm, поэтому
// конструктор и правка не расходятся.
//
// Форма отдаёт ДОКУМЕНТ узла (outbounds[] или endpoints[]) в
// AddServerResult.ConfigJSON, как Tailscale: его разбирает тот же
// AppendManualConfigJSON.
package dialogs

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// nodeFormClassicNoteText — вторая строка подсказки под паролем в
// классическом режиме: канала челленджа у `sing-box run` нет.
const nodeFormClassicNoteText = "This launcher runs the core in classic mode: second-factor prompts are unavailable here"

// NodeForm — виджеты формы узла одной схемы.
type NodeForm struct {
	model *wizardbusiness.NodeFormModel
	// base — тело, поверх которого форма пишет свои пути (у нового узла
	// пустое, у готового — текущее тело узла).
	base map[string]interface{}

	// Контролы по ключу поля модели.
	entries map[string]*widget.Entry
	checks  map[string]*widget.Check
	selects map[string]*widget.Select
	// selectValues — подпись пункта выпадающего списка → значение.
	selectValues map[string]map[string]string
	// cells — ячейки строк поля (подпись, контрол, подсказки): прячутся
	// вместе по видимости модели.
	cells map[string][]fyne.CanvasObject
	// advisory — строка предупреждения advisory под полем.
	advisory map[string]*widget.Label

	radios map[string]*widget.RadioGroup
	// optionByLabel — группа → подпись варианта → вариант.
	optionByLabel map[string]map[string]wizardbusiness.NodeFormChoiceOption
	// optionCells — ячейки поля варианта (отпечаток): видны, когда вариант
	// выбран.
	optionCells map[string][]fyne.CanvasObject
	// optionAdvisory — предупреждение выбранного варианта (Do not verify).
	optionAdvisory map[string]*widget.Label

	box      *fyne.Container
	onChange func()
	syncing  bool
}

// NewNodeForm собирает форму схемы. onChange зовётся на каждую правку
// пользователя (не на программную загрузку).
func NewNodeForm(scheme string, opts wizardbusiness.NodeFormOptions, onChange func()) (*NodeForm, error) {
	model, err := wizardbusiness.NewNodeFormModel(scheme, nil, opts)
	if err != nil {
		return nil, err
	}
	o := &NodeForm{
		model:          model,
		base:           map[string]interface{}{},
		entries:        map[string]*widget.Entry{},
		checks:         map[string]*widget.Check{},
		selects:        map[string]*widget.Select{},
		selectValues:   map[string]map[string]string{},
		cells:          map[string][]fyne.CanvasObject{},
		advisory:       map[string]*widget.Label{},
		radios:         map[string]*widget.RadioGroup{},
		optionByLabel:  map[string]map[string]wizardbusiness.NodeFormChoiceOption{},
		optionCells:    map[string][]fyne.CanvasObject{},
		optionAdvisory: map[string]*widget.Label{},
		onChange:       onChange,
	}
	classic := wizardbusiness.NodeFormClassicBackend()

	var top []fyne.CanvasObject
	var items []*widget.AccordionItem
	openIdx := []int{}
	for _, blk := range model.Blocks {
		grid := container.New(layout.NewFormLayout())
		for _, c := range blk.Choices {
			o.addChoice(grid, c)
		}
		for _, f := range blk.Fields {
			o.addField(grid, f, classic && f.ClassicNote)
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

	o.Load(nil)
	return o, nil
}

// Object — корневой объект формы.
func (o *NodeForm) Object() *fyne.Container { return o.box }

// Model — модель формы.
func (o *NodeForm) Model() *wizardbusiness.NodeFormModel { return o.model }

func (o *NodeForm) changed() {
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

// emptyOptionLabel — подпись пустого пункта выпадающего списка.
func emptyOptionLabel(f *wizardbusiness.NodeFormField) string {
	if f.EmptyLabel != "" {
		return locale.T(f.EmptyLabel)
	}
	if f.Default != "" {
		return locale.Tf("Default (%s)", f.ValueLabel(f.Default))
	}
	return locale.T("Default")
}

// newSelect — выпадающий список enum/дискриминатора: пустой пункт (если
// поле не обязательно) и значения реестра с подписями стороны.
func (o *NodeForm) newSelect(f *wizardbusiness.NodeFormField) *widget.Select {
	labels := make([]string, 0, len(f.Values)+1)
	byLabel := map[string]string{}
	if f.Optional {
		l := emptyOptionLabel(f)
		labels = append(labels, l)
		byLabel[l] = ""
	}
	for _, v := range f.Values {
		l := f.ValueLabel(v)
		labels = append(labels, l)
		byLabel[l] = v
	}
	sel := widget.NewSelect(labels, func(string) { o.changed() })
	o.selects[f.Key] = sel
	o.selectValues[f.Key] = byLabel
	return sel
}

// newEntry — поле ввода: секрет — с «глазом» (маскируется только показ, в
// теле узла он лежит открыто: «секреты в state — by design»).
func (o *NodeForm) newEntry(f *wizardbusiness.NodeFormField) *widget.Entry {
	var e *widget.Entry
	if f.Secret {
		e = widget.NewPasswordEntry()
	} else {
		e = widget.NewEntry()
	}
	if f.Placeholder != "" {
		// Образец значения или дефолт реестра — не текст интерфейса.
		e.SetPlaceHolder(f.Placeholder)
	}
	e.OnChanged = func(string) { o.changed() }
	o.entries[f.Key] = e
	return e
}

// addField добавляет строки поля в сетку блока.
func (o *NodeForm) addField(grid *fyne.Container, f *wizardbusiness.NodeFormField, classicNote bool) {
	var cells []fyne.CanvasObject
	add := func(left, right fyne.CanvasObject) {
		grid.Add(left)
		grid.Add(right)
		cells = append(cells, left, right)
	}
	switch {
	case f.Kind == wizardbusiness.NodeFormKindBool:
		ch := widget.NewCheck(locale.T(f.Label), func(bool) { o.changed() })
		o.checks[f.Key] = ch
		add(widget.NewLabel(""), ch)
	case f.Kind == wizardbusiness.NodeFormKindEnum:
		add(widget.NewLabel(locale.T(f.Label)), o.newSelect(f))
	default:
		add(widget.NewLabel(locale.T(f.Label)), o.newEntry(f))
	}
	if f.Hint != "" {
		add(widget.NewLabel(""), hintLabel(locale.T(f.Hint)))
	}
	if classicNote {
		add(widget.NewLabel(""), hintLabel(locale.T(nodeFormClassicNoteText)))
	}
	adv := warnLabel()
	o.advisory[f.Key] = adv
	spacer := widget.NewLabel("")
	spacer.Hide()
	grid.Add(spacer)
	grid.Add(adv)
	// Строку advisory прячет refresh по значению, а не по видимости поля.
	o.cells[f.Key] = cells
}

// addChoice добавляет группу выбора: радиокнопки и поля вариантов под ними.
func (o *NodeForm) addChoice(grid *fyne.Container, c *wizardbusiness.NodeFormChoice) {
	labels := make([]string, 0, len(c.Options))
	byLabel := map[string]wizardbusiness.NodeFormChoiceOption{}
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
		e := o.newEntry(f)
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
		o.advisory[f.Key] = adv
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
func (o *NodeForm) Load(body map[string]interface{}) {
	if body == nil {
		body = map[string]interface{}{}
	}
	o.base = body
	vals := o.model.FormFromBody(body)

	o.syncing = true
	for key, e := range o.entries {
		e.SetText(vals.Fields[key])
	}
	for key, ch := range o.checks {
		ch.SetChecked(vals.Fields[key] == "true")
	}
	for key, sel := range o.selects {
		f := o.model.Field(key)
		v := vals.Fields[key]
		if v == "" && f != nil && !f.Optional {
			if len(body) == 0 && len(sel.Options) > 0 {
				// Новый узел, обязательный список: первое значение (макет
				// OpenConnect — FortiGate).
				sel.SetSelected(sel.Options[0])
				continue
			}
			// Готовый узел без ключа: ядро берёт дефолт поля (anyconnect) —
			// его форма и показывает.
			v = f.Default
		}
		label := o.selectLabelOf(key, v)
		if label != "" && !containsString(sel.Options, label) {
			// Значение вне enum (пришло из JSON) — пунктом списка, иначе
			// Select его не покажет, а форма сняла бы ключ с тела.
			sel.Options = append(sel.Options, label)
			o.selectValues[key][label] = v
		}
		sel.SetSelected(label)
	}
	for id, radio := range o.radios {
		radio.SetSelected(o.optionLabel(id, vals.Choices[id]))
	}
	o.syncing = false
	o.refresh()
}

func (o *NodeForm) selectLabelOf(key, value string) string {
	for l, v := range o.selectValues[key] {
		if v == value {
			return l
		}
	}
	if value == "" {
		return ""
	}
	// Значение вне enum (пришло из JSON) показывается как есть.
	if f := o.model.Field(key); f != nil {
		return f.ValueLabel(value)
	}
	return value
}

func (o *NodeForm) optionLabel(choiceID, optionID string) string {
	for l, opt := range o.optionByLabel[choiceID] {
		if opt.ID == optionID {
			return l
		}
	}
	return ""
}

// SetBase меняет тело-основу, не трогая набранное: правки ложатся поверх
// текущего тела узла (поля других редакторов, сквозные ключи).
func (o *NodeForm) SetBase(body map[string]interface{}) {
	if body == nil {
		body = map[string]interface{}{}
	}
	o.base = body
}

// Values — что набрано в форме.
func (o *NodeForm) Values() wizardbusiness.NodeFormValues {
	v := wizardbusiness.NodeFormValues{Fields: map[string]string{}, Choices: map[string]string{}}
	for key, e := range o.entries {
		v.Fields[key] = e.Text
	}
	for key, ch := range o.checks {
		if ch.Checked {
			v.Fields[key] = "true"
		}
	}
	for key, sel := range o.selects {
		if val, ok := o.selectValues[key][sel.Selected]; ok {
			v.Fields[key] = val
			continue
		}
		v.Fields[key] = sel.Selected
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
func (o *NodeForm) Body() (map[string]interface{}, error) {
	return o.model.BodyFromForm(o.base, o.Values())
}

// Verdict — проверка тела перед сохранением (обязательные поля, диапазоны,
// санитайзер реестра).
func (o *NodeForm) Verdict(body map[string]interface{}) error {
	return o.model.Verdict(body)
}

// refresh пересчитывает видимость строк и предупреждения advisory: при
// смене списка, радиокнопки и галок.
func (o *NodeForm) refresh() {
	body, _ := o.Body()
	visible := o.model.Visible(body)
	for key, cells := range o.cells {
		show := visible[key]
		for _, c := range cells {
			if show {
				c.Show()
			} else {
				c.Hide()
			}
		}
	}
	vals := o.Values()
	for key, adv := range o.advisory {
		text := ""
		if f := o.model.Field(key); f != nil && (f.ChoiceID == "" && visible[key] || f.ChoiceID != "" && vals.Choices[f.ChoiceID] == f.OptionID) {
			text = o.model.AdvisoryText(key, vals.Fields[key])
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

// nodeFormDocument собирает документ узла формы.
func nodeFormDocument(tag string, o *NodeForm) ([]byte, string, error) {
	body, err := o.Body()
	if err != nil {
		return nil, "", err
	}
	return o.model.Document(tag, body)
}

// previewNodeFormDocument — превью документа узла через тот же разбор, что
// применит запись (WYSIWYG вкладки JSON).
func (f *addServerForm) previewNodeFormDocument() (string, string) {
	form := f.currentNodeForm()
	if form == nil {
		return "", ""
	}
	raw, _, err := nodeFormDocument(f.tag.Text, form)
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

// nodeFormResult — результат кнопки Add для варианта формы схемы.
func (f *addServerForm) nodeFormResult() (AddServerResult, error) {
	form := f.currentNodeForm()
	if form == nil {
		return AddServerResult{}, fmt.Errorf("%s", locale.T("The form for this server type is unavailable"))
	}
	doc, tag, err := nodeFormDocument(f.tag.Text, form)
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
