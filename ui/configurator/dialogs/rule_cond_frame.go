package dialogs

// rule_cond_frame.go — рамка условия в форме правила (SPEC 188 §2.1, §6).
//
// Что в рамке — таблица реестра rule_conditions.json: ключи условия, вид
// поля (`field`), подписи (`label`, `placeholder`), значения enum, `one_of`,
// `conflicts`, `platforms`. Имён ключей здесь нет: особый вид получают поля
// по виду (`refs` — поле наборов, `processes` — выбор процессов), а не по
// имени ключа. Поля одной рамки — «any of», рамки между собой — «AND».

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config/registry"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Виды поля таблицы условий (registry ruleConditionFields).
const (
	condFieldRefs      = "refs"
	condFieldLines     = "lines"
	condFieldCIDRLines = "cidr_lines"
	condFieldPorts     = "ports"
	condFieldBool      = "bool"
	condFieldEnumMulti = "enum_multi"
	condFieldProcesses = "processes"
)

const (
	ruleCondConflictText = "Not available while the condition has rule sets. For “rule sets or private addresses”, add a Rule group with “any of”."
	ruleCondPortErrText  = "Not a port number (0–65535): %s"
)

// ruleCondRegistryTexts — подписи и подсказки таблицы условий реестра
// (rule_conditions.json, `label`, `placeholder`), которые форма лаунчера
// показывает через locale.T. Список только для проверки каталога
// переводов (tools/l10n/l10n_check): ключи приходят из реестра, сканер их
// не видит. Новая подпись в таблице — строка здесь и перевод в ru.json.
var ruleCondRegistryTexts = []string{
	"Destination / Rule sets",                // l10n-key
	"Rule sets — URL or file (one per line)", // l10n-key
	"Domains",                                // l10n-key
	"One per line, e.g. example.com",         // l10n-key
	"Domain suffixes",                        // l10n-key
	"Domain keywords",                        // l10n-key
	"One per line, e.g. google",              // l10n-key
	"Domain regex",                           // l10n-key
	"One per line, e.g. ^.*\\.google\\.com$", // l10n-key
	"IP addresses / CIDR",                    // l10n-key
	"One per line, e.g. 192.168.1.0/24",      // l10n-key
	"Private addresses",                      // l10n-key
	"Port",                                   // l10n-key
	"Ports",                                  // l10n-key
	"e.g. 443, 8443",                         // l10n-key
	"Port ranges",                            // l10n-key
	"One per line, e.g. 1000:2000",           // l10n-key
	"Source",                                 // l10n-key
	"Source IP addresses / CIDR",             // l10n-key
	"Private source addresses",               // l10n-key
	"Source port",                            // l10n-key
	"Source ports",                           // l10n-key
	"Source port ranges",                     // l10n-key
	"Process",                                // l10n-key
	"Name",                                   // l10n-key
	"Path",                                   // l10n-key
	"One per line, e.g. C:\\Program Files\\App\\app.exe", // l10n-key
	"Path regex",                           // l10n-key
	"One per line, e.g. .*steam.*",         // l10n-key
	"Protocol",                             // l10n-key
	"Protocols",                            // l10n-key
	"Network",                              // l10n-key
	"Networks",                             // l10n-key
	"Inbound",                              // l10n-key
	"Inbound tags",                         // l10n-key
	"One per line, e.g. tun-in",            // l10n-key
	"Wi-Fi",                                // l10n-key
	"SSID",                                 // l10n-key
	"One per line",                         // l10n-key
	"BSSID",                                // l10n-key
	"One per line, e.g. 00:11:22:33:44:55", // l10n-key
}

// ruleFormCtx — общее для всех рамок окна правила.
type ruleFormCtx struct {
	table wizardmodels.RuleCondTable
	app   fyne.App
	win   fyne.Window
	// goos — ОС машины, для которой собирается конфиг (цель модели), а не
	// хоста лаунчера: по ней рамка с `platforms` пишет «не действует».
	goos     string
	onChange func()
}

func (c *ruleFormCtx) window() fyne.Window { return c.win }

func (c *ruleFormCtx) changed() {
	if c.onChange != nil {
		c.onChange()
	}
}

// condField — поле одного ключа условия.
type condField interface {
	object() fyne.CanvasObject
	// value — значение для тела; nil — поле пусто. strict — с проверкой
	// (Save), иначе как набрано (живые подсказки, Preview).
	value(strict bool) (interface{}, error)
	setEnabled(on bool)
}

// condFrame — рамка одного условия.
type condFrame struct {
	cond     registry.RuleCondition
	fields   map[string]condField
	rows     map[string]fyne.CanvasObject
	oneOf    *widget.RadioGroup
	oneOfKey map[string]string // подпись способа → ключ
	conflict *widget.Label
	obj      fyne.CanvasObject
}

func newCondFrame(ctx *ruleFormCtx, cond registry.RuleCondition, values map[string]interface{}, onRemove func()) *condFrame {
	f := &condFrame{
		cond:     cond,
		fields:   map[string]condField{},
		rows:     map[string]fyne.CanvasObject{},
		oneOfKey: map[string]string{},
	}
	changed := func() {
		f.syncConflicts()
		ctx.changed()
	}

	title := widget.NewLabel(locale.T(cond.Label))
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Truncation = fyne.TextTruncateEllipsis
	right := container.NewHBox()
	if len(cond.Keys) > 1 && !cond.OneOf {
		anyOf := widget.NewLabel(locale.T("any of"))
		anyOf.Importance = widget.LowImportance
		right.Add(anyOf)
	}
	remove := widget.NewButtonWithIcon("", theme.CancelIcon(), onRemove)
	remove.Importance = widget.LowImportance
	right.Add(remove)
	body := container.NewVBox(container.NewBorder(nil, nil, nil, right, title))

	if len(cond.Platforms) > 0 && !condPlatformHas(cond.Platforms, ctx.goos) {
		note := widget.NewLabel("⚠ " + locale.T("Has no effect on this machine"))
		note.Importance = widget.WarningImportance
		note.Truncation = fyne.TextTruncateEllipsis
		body.Add(note)
	}

	if cond.OneOf {
		var labels []string
		selected := ""
		for _, k := range cond.Keys {
			l := locale.T(k.Label)
			labels = append(labels, l)
			f.oneOfKey[l] = k.Key
			if selected == "" && !wizardmodels.RuleCondValueEmpty(values[k.Key]) {
				selected = l
			}
		}
		if selected == "" && len(labels) > 0 {
			selected = labels[0]
		}
		f.oneOf = widget.NewRadioGroup(labels, nil)
		f.oneOf.Horizontal = true
		f.oneOf.Required = true
		f.oneOf.SetSelected(selected)
		f.oneOf.OnChanged = func(string) {
			f.syncOneOf()
			changed()
		}
		body.Add(f.oneOf)
	}

	for _, k := range cond.Keys {
		fld, row := newCondField(ctx, k, values[k.Key], !cond.OneOf, changed)
		f.fields[k.Key] = fld
		f.rows[k.Key] = row
		body.Add(row)
	}
	f.conflict = widget.NewLabel(locale.T(ruleCondConflictText))
	f.conflict.Wrapping = fyne.TextWrapWord
	f.conflict.Importance = widget.LowImportance
	f.conflict.Hide()
	body.Add(f.conflict)

	f.syncOneOf()
	f.syncConflicts()
	f.obj = fynewidget.NewFramedPanel(body)
	return f
}

// condPlatformHas — ОС goos среди platforms условия.
func condPlatformHas(platforms []string, goos string) bool {
	for _, p := range platforms {
		if strings.EqualFold(p, goos) {
			return true
		}
	}
	return false
}

// selectedKey — ключ выбранного способа у условия `one_of`; иначе "".
func (f *condFrame) selectedKey() string {
	if f.oneOf == nil {
		return ""
	}
	return f.oneOfKey[f.oneOf.Selected]
}

func (f *condFrame) syncOneOf() {
	if f.oneOf == nil {
		return
	}
	sel := f.selectedKey()
	for key, row := range f.rows {
		if key == sel {
			row.Show()
		} else {
			row.Hide()
		}
	}
}

// syncConflicts — ключ с `conflicts` недоступен, пока в рамке заполнен
// конфликтующий ключ (значение сохраняется), с подсказкой про группу правил.
func (f *condFrame) syncConflicts() {
	blockedAny := false
	for _, k := range f.cond.Keys {
		if len(k.Conflicts) == 0 {
			continue
		}
		blocked := false
		for _, other := range k.Conflicts {
			if fld, ok := f.fields[other]; ok {
				if v, _ := fld.value(false); !wizardmodels.RuleCondValueEmpty(v) {
					blocked = true
				}
			}
		}
		f.fields[k.Key].setEnabled(!blocked)
		blockedAny = blockedAny || blocked
	}
	if blockedAny {
		f.conflict.Show()
	} else {
		f.conflict.Hide()
	}
}

// read — условие из полей рамки; у `one_of` — только выбранный способ.
func (f *condFrame) read(strict bool) (wizardmodels.RuleCond, error) {
	out := wizardmodels.RuleCond{ID: f.cond.ID, Values: map[string]interface{}{}}
	sel := f.selectedKey()
	for _, k := range f.cond.Keys {
		if f.oneOf != nil && k.Key != sel {
			continue
		}
		v, err := f.fields[k.Key].value(strict)
		if err != nil {
			return out, err
		}
		if !wizardmodels.RuleCondValueEmpty(v) {
			out.Values[k.Key] = v
		}
	}
	return out, nil
}

// newCondField — поле ключа по его виду и строка формы с подписью.
// withLabel=false — подпись даёт переключатель способа (`one_of`).
func newCondField(ctx *ruleFormCtx, k registry.RuleConditionKey, initial interface{}, withLabel bool, onChange func()) (condField, fyne.CanvasObject) {
	label := locale.T(k.Label)
	labeled := func(obj fyne.CanvasObject) fyne.CanvasObject {
		if !withLabel {
			return obj
		}
		l := widget.NewLabel(label)
		l.Truncation = fyne.TextTruncateEllipsis
		return container.NewVBox(l, obj)
	}
	switch k.Field {
	case condFieldRefs:
		f := &refsValueField{refs: newRefsField(ctx, label, condValueStrings(initial), onChange)}
		return f, f.refs.object()
	case condFieldBool:
		f := &boolField{check: widget.NewCheck(label, nil)}
		b, _ := initial.(bool)
		f.check.SetChecked(b)
		f.check.OnChanged = func(bool) { onChange() }
		return f, f.check
	case condFieldPorts:
		f := &portsField{entry: widget.NewEntry()}
		if k.Placeholder != "" {
			f.entry.SetPlaceHolder(locale.T(k.Placeholder))
		}
		f.entry.SetText(strings.Join(condValueStrings(initial), ", "))
		f.entry.OnChanged = func(string) { onChange() }
		return f, labeled(f.entry)
	case condFieldEnumMulti:
		f := newEnumField(k, condValueStrings(initial), onChange)
		return f, labeled(f.box)
	case condFieldProcesses:
		f := &processesField{list: newProcessListField(ctx.app, condValueStrings(initial), onChange)}
		return f, labeled(f.list.object())
	default: // lines, cidr_lines
		f := &linesField{entry: widget.NewMultiLineEntry()}
		f.entry.Wrapping = fyne.TextWrapWord
		f.entry.SetMinRowsVisible(3)
		if k.Placeholder != "" {
			f.entry.SetPlaceHolder(locale.T(k.Placeholder))
		}
		f.entry.SetText(strings.Join(condValueStrings(initial), "\n"))
		f.entry.OnChanged = func(string) { onChange() }
		return f, labeled(f.entry)
	}
}

// condValueStrings — значение тела строками поля: строка, число или список.
func condValueStrings(v interface{}) []string {
	one := func(x interface{}) (string, bool) {
		switch s := x.(type) {
		case string:
			return s, s != ""
		case float64:
			return strconv.FormatFloat(s, 'f', -1, 64), true
		case int:
			return strconv.Itoa(s), true
		case nil:
			return "", false
		}
		return fmt.Sprint(x), true
	}
	switch x := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, it := range x {
			if s, ok := one(it); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), x...)
	default:
		if s, ok := one(x); ok {
			return []string{s}
		}
	}
	return nil
}

// stringsValue — список строк значением тела; пустой — nil.
func stringsValue(items []string) interface{} {
	if len(items) == 0 {
		return nil
	}
	out := make([]interface{}, 0, len(items))
	for _, s := range items {
		out = append(out, s)
	}
	return out
}

type linesField struct{ entry *widget.Entry }

func (f *linesField) object() fyne.CanvasObject { return f.entry }
func (f *linesField) value(bool) (interface{}, error) {
	return stringsValue(ParseLines(f.entry.Text, false)), nil
}
func (f *linesField) setEnabled(on bool) { setEntryEnabled(f.entry, on) }

type portsField struct{ entry *widget.Entry }

func (f *portsField) object() fyne.CanvasObject { return f.entry }
func (f *portsField) value(strict bool) (interface{}, error) {
	var out []interface{}
	split := strings.FieldsFunc(f.entry.Text, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	for _, tok := range split {
		n, err := strconv.Atoi(tok)
		if err != nil || n < 0 || n > 65535 {
			if strict {
				return nil, errors.New(locale.Tf(ruleCondPortErrText, tok))
			}
			continue
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
func (f *portsField) setEnabled(on bool) { setEntryEnabled(f.entry, on) }

type boolField struct{ check *widget.Check }

func (f *boolField) object() fyne.CanvasObject { return f.check }
func (f *boolField) value(bool) (interface{}, error) {
	if f.check.Checked {
		return true, nil
	}
	return nil, nil
}
func (f *boolField) setEnabled(on bool) {
	if on {
		f.check.Enable()
	} else {
		f.check.Disable()
	}
}

// enumField — значения из таблицы чипами; `open` — ещё и поле для значений
// вне списка.
type enumField struct {
	chips []*fynewidget.Chip
	vals  []string
	other *widget.Entry
	box   *fyne.Container
}

func newEnumField(k registry.RuleConditionKey, initial []string, onChange func()) *enumField {
	f := &enumField{vals: append([]string(nil), k.Values...)}
	known := map[string]bool{}
	for _, v := range k.Values {
		known[v] = true
	}
	picked := map[string]bool{}
	var extra []string
	for _, v := range initial {
		if known[v] {
			picked[v] = true
		} else {
			extra = append(extra, v)
		}
	}
	objs := make([]fyne.CanvasObject, 0, len(k.Values))
	for _, v := range k.Values {
		c := fynewidget.NewChip(v, picked[v], func(bool) { onChange() })
		f.chips = append(f.chips, c)
		objs = append(objs, c)
	}
	f.box = container.NewVBox(fynewidget.NewFlowBox(objs...))
	if k.Open || len(extra) > 0 {
		f.other = widget.NewEntry()
		f.other.SetPlaceHolder(locale.T("Other values, comma-separated"))
		f.other.SetText(strings.Join(extra, ", "))
		f.other.OnChanged = func(string) { onChange() }
		f.box.Add(f.other)
	}
	return f
}

func (f *enumField) object() fyne.CanvasObject { return f.box }
func (f *enumField) value(bool) (interface{}, error) {
	var out []string
	for i, c := range f.chips {
		if c.Selected() {
			out = append(out, f.vals[i])
		}
	}
	if f.other != nil {
		for _, s := range strings.Split(f.other.Text, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return stringsValue(out), nil
}
func (f *enumField) setEnabled(on bool) {
	if f.other != nil {
		setEntryEnabled(f.other, on)
	}
}

type processesField struct{ list *processListField }

func (f *processesField) object() fyne.CanvasObject { return f.list.object() }
func (f *processesField) value(bool) (interface{}, error) {
	return stringsValue(f.list.values()), nil
}
func (f *processesField) setEnabled(on bool) { f.list.setEnabled(on) }

type refsValueField struct{ refs *refsField }

func (f *refsValueField) object() fyne.CanvasObject { return f.refs.object() }
func (f *refsValueField) value(strict bool) (interface{}, error) {
	refs, err := f.refs.values(strict)
	if err != nil {
		return nil, err
	}
	return stringsValue(refs), nil
}
func (f *refsValueField) setEnabled(on bool) { f.refs.setEnabled(on) }

func setEntryEnabled(e *widget.Entry, on bool) {
	if on {
		e.Enable()
	} else {
		e.Disable()
	}
}
