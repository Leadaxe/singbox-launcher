// File node_form_model.go — общая модель формы узла по реестру (SPEC 167
// §2.3, обобщена SPEC 168 §2.2): конструктор «Add server» и блок правки у
// готового узла раскладываются по ней одной, для любой схемы с секцией
// `body.form`.
//
// Что берётся из реестра: поля формы и их порядок (reg.FormFields — общие
// секции, варианты транспорта, элемент [0] массива, fixed), блок поля,
// видимость, обязательность, диапазон формы, тип значения, секрет, enum,
// коды advisory, группы выбора, `form.implies`. Вычисление видимости,
// обязательности и активного варианта — функции реестра (FormState,
// ActiveFormOption), запись по пути — SetFormValue/DeleteFormValue/
// ApplyFormImplies/ApplyFormFixed; модель обход реестра не повторяет.
//
// Что своё у стороны — тексты (node_form_labels.go): подписи и подсказки по
// пути тела. Поле с `form`, для которого у стороны нет подписи, форма не
// рисует и не трогает.
//
// Форма меняет ТОЛЬКО свои пути. Остальное тело (сквозные ключи, второй
// пир, поля без подписи) копируется как есть. Скрытое поле из тела не
// снимается: что пришло из JSON, уходит в ядро (REGISTRY_DESIGN §3).
//
// go1.20-совместимо: без slices/maps/min/max.
package business

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"singbox-launcher/core"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/nodewarn"
)

// NodeFormFieldKind — вид поля формы (по типу поля в реестре).
type NodeFormFieldKind string

const (
	NodeFormKindString   NodeFormFieldKind = "string"
	NodeFormKindEnum     NodeFormFieldKind = "enum"
	NodeFormKindBool     NodeFormFieldKind = "bool"
	NodeFormKindInt      NodeFormFieldKind = "int"
	NodeFormKindDuration NodeFormFieldKind = "duration"
	// NodeFormKindList — listable_string / string_array: в форме строка
	// через запятую, в теле массив.
	NodeFormKindList NodeFormFieldKind = "list"
	// NodeFormKindRange — awg_range: число либо строка «min-max».
	NodeFormKindRange NodeFormFieldKind = "range"
)

// NodeFormField — поле формы: путь тела и всё, что нужно для строки.
type NodeFormField struct {
	// Key — уникальный ключ поля в форме: путь, у поля варианта —
	// «вариант/путь» (пути вариантов совпадают: transport.path у ws и http).
	Key string
	// Path — путь от корня тела ("tls.server_name", "peers.0.address").
	Path string
	// Block — id блока из body.form.blocks; у поля варианта группы выбора
	// это блок группы.
	Block string
	// Variant/VariantOf — поле варианта вариантного объекта: видно и
	// пишется, только когда дискриминатор объекта равен Variant.
	Variant   string
	VariantOf string
	// Discriminator — выбор варианта (transport.type): пусто — объект не
	// пишется вовсе.
	Discriminator bool
	Kind          NodeFormFieldKind
	// Label, Hint — английские ключи locale (подпись строки, пояснение под
	// строкой). Placeholder — подсказка в пустом поле: образец значения
	// стороны либо значение по умолчанию из реестра, не ключ locale.
	Label       string
	Hint        string
	Placeholder string
	// Secret — значение маскируется (password entry с глазком).
	Secret bool
	// Values — допустимые непустые значения enum; ValueLabels — подписи
	// значений (без подписи — значение как есть).
	Values      []string
	ValueLabels map[string]string
	// Optional — у выпадающего списка есть пустой пункт (ключ не пишется);
	// EmptyLabel — его подпись (ключ locale), пусто — «Default».
	Optional   bool
	EmptyLabel string
	// Default — строковый дефолт поля в реестре (что ядро берёт без ключа).
	Default string
	// Min/Max — диапазон ФОРМЫ (не ядра).
	Min *float64
	Max *float64
	// ChoiceID/OptionID — поле занимает вариант группы выбора: пишется,
	// только когда вариант активен.
	ChoiceID string
	OptionID string
	// ClassicNote — под полем строка о недоступности второго фактора в
	// классическом режиме ядра.
	ClassicNote bool

	host bool
	reg  *registry.Field
}

// NodeFormChoiceOption — вариант группы выбора.
type NodeFormChoiceOption struct {
	ID    string
	Label string // ключ locale
	Paths []string
	Set   map[string]interface{}
	// Advisory — коды advisory путей Set: вариант окрашивается
	// предупреждением (insecure → tls_insecure).
	Advisory []string
}

// NodeFormChoice — группа взаимоисключающих вариантов.
type NodeFormChoice struct {
	ID      string
	Block   string
	Label   string // ключ locale
	Options []NodeFormChoiceOption
}

// NodeFormBlock — блок формы в порядке реестра.
type NodeFormBlock struct {
	ID    string
	Title string // ключ locale; пусто — блок без заголовка
	// Expanded — блок раскрыт при открытии формы.
	Expanded bool
	// Collapsible — блок сворачивается (аккордеон). Первый блок формы —
	// нет: он всегда виден.
	Collapsible bool
	// Choices — группы выбора блока (рисуются первыми).
	Choices []*NodeFormChoice
	// Fields — поля блока, не принадлежащие группам выбора.
	Fields []*NodeFormField
}

// NodeFormOptions — что форма у этого вызывающего не рисует.
type NodeFormOptions struct {
	// SkipBlocks — блоки, поля которых правит другой редактор (обфускация
	// AmneziaWG в окне узла): форма их не рисует и не трогает.
	SkipBlocks []string
}

// NodeFormModel — модель формы схемы.
type NodeFormModel struct {
	Scheme string
	Blocks []NodeFormBlock
	// Fields — все поля формы (и обычные, и поля вариантов).
	Fields []*NodeFormField
	byKey  map[string]*NodeFormField
	// discOf — путь вариантного объекта → путь его дискриминатора.
	discOf map[string]string
	form   *registry.FormSection
	reg    *registry.Registry
}

// NodeFormValues — что набрано в форме.
type NodeFormValues struct {
	// Fields — ключ поля → текст; у bool "true" или пусто, у enum —
	// значение (пусто — пустой пункт).
	Fields map[string]string
	// Choices — id группы → id выбранного варианта.
	Choices map[string]string
}

// NewNodeFormModel строит модель формы схемы по реестру.
func NewNodeFormModel(scheme string, reg *registry.Registry, opts NodeFormOptions) (*NodeFormModel, error) {
	if reg == nil {
		var err error
		if reg, err = registry.Get(); err != nil {
			return nil, err
		}
	}
	form := reg.Form(scheme)
	if form == nil {
		return nil, fmt.Errorf("registry: scheme %s has no form", scheme)
	}
	skip := map[string]bool{}
	for _, b := range opts.SkipBlocks {
		skip[b] = true
	}
	m := &NodeFormModel{
		Scheme: scheme,
		byKey:  map[string]*NodeFormField{},
		discOf: map[string]string{},
		form:   form,
		reg:    reg,
	}

	// Пути, которые пишет `set` варианта группы: своего контрола у них нет.
	bySet := map[string]bool{}
	for _, c := range form.Choices {
		for _, o := range c.Options {
			for p := range o.Set {
				bySet[p] = true
			}
		}
	}

	all := reg.FormFields(scheme)
	type ranked struct {
		f    *NodeFormField
		rank int
	}
	var fields []ranked
	for i, ff := range all {
		if ff.Discriminator {
			m.discOf[parentPath(ff.Path)] = ff.Path
		}
		if bySet[ff.Path] || skip[ff.Block] {
			continue
		}
		side, rank := nodeFormSideOf(scheme, ff.Path)
		if side.label == "" {
			// У стороны нет подписи — поле не рисуется и не трогается.
			continue
		}
		f := newNodeFormField(ff, side)
		if rank < 0 {
			rank = 1000 + i
		}
		fields = append(fields, ranked{f: f, rank: rank})
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].rank < fields[j].rank })
	for _, r := range fields {
		m.Fields = append(m.Fields, r.f)
		m.byKey[r.f.Key] = r.f
	}

	first := true
	for _, id := range form.Blocks {
		if skip[id] {
			continue
		}
		bs := nodeFormBlockSides[id]
		blk := NodeFormBlock{ID: id, Title: bs.title, Expanded: !bs.collapsed, Collapsible: !first}
		if blk.Collapsible && blk.Title == "" {
			blk.Title = id
		}
		for _, c := range form.Choices {
			if c.Block != id {
				continue
			}
			cs := nodeFormChoiceSides[c.ID]
			nc := &NodeFormChoice{ID: c.ID, Block: c.Block, Label: cs.label}
			if nc.Label == "" {
				nc.Label = c.ID
			}
			for _, o := range c.Options {
				opt := NodeFormChoiceOption{ID: o.ID, Paths: o.Paths, Set: o.Set, Label: cs.optionLabel(o.ID)}
				if opt.Label == "" {
					opt.Label = o.ID
				}
				for _, p := range sortedKeys(o.Set) {
					if rf := formFieldAt(all, p); rf != nil {
						opt.Advisory = append(opt.Advisory, advisoryCodes(rf, o.Set[p])...)
					}
				}
				nc.Options = append(nc.Options, opt)
			}
			blk.Choices = append(blk.Choices, nc)
		}
		for _, f := range m.Fields {
			if f.Block == id && f.ChoiceID == "" {
				blk.Fields = append(blk.Fields, f)
			}
		}
		if len(blk.Choices) == 0 && len(blk.Fields) == 0 {
			continue
		}
		m.Blocks = append(m.Blocks, blk)
		first = false
	}
	return m, nil
}

func newNodeFormField(ff registry.FormField, side nodeFormSide) *NodeFormField {
	rf := ff.Field
	f := &NodeFormField{
		Key:           ff.Path,
		Path:          ff.Path,
		Block:         ff.Block,
		Variant:       ff.Variant,
		VariantOf:     ff.VariantOf,
		Discriminator: ff.Discriminator,
		Kind:          nodeFormKindOf(rf),
		Label:         side.label,
		Hint:          side.hint,
		EmptyLabel:    side.emptyLabel,
		ValueLabels:   side.values,
		Secret:        rf.Secret,
		ChoiceID:      ff.ChoiceID,
		OptionID:      ff.OptionID,
		ClassicNote:   side.classicNote,
		host:          side.host,
		reg:           rf,
	}
	if ff.Variant != "" {
		f.Key = ff.Variant + "/" + ff.Path
	}
	if rf.Form != nil {
		f.Min, f.Max = rf.Form.Min, rf.Form.Max
	}
	if f.Kind == NodeFormKindEnum {
		hasEmpty := false
		for _, v := range rf.Values {
			s := fmt.Sprint(v)
			if s == "" {
				hasEmpty = true
				continue
			}
			f.Values = append(f.Values, s)
		}
		required := rf.Form != nil && rf.Form.Required
		f.Optional = hasEmpty || !required
	}
	switch d := rf.Default.(type) {
	case string:
		f.Default = d
	case float64:
		f.Default = strconv.FormatFloat(d, 'f', -1, 64)
	}
	if side.placeholder != "" {
		f.Placeholder = side.placeholder
	} else if f.Kind != NodeFormKindBool && f.Kind != NodeFormKindEnum {
		// Подсказка — значение, которое ядро или реестр возьмут без ключа:
		// default_when без условия (allowed_ips), иначе default.
		v := rf.Default
		if dw := rf.DefaultWhen; dw != nil && dw.Absent && dw.When == nil && dw.Value != nil {
			v = dw.Value
		}
		f.Placeholder = placeholderText(v)
	}
	return f
}

func placeholderText(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, it := range t {
			parts = append(parts, fmt.Sprint(it))
		}
		return strings.Join(parts, ", ")
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case string:
		return t
	}
	return fmt.Sprint(v)
}

// formFieldAt — поле реестра формы вне вариантов по пути (у путей set).
func formFieldAt(all []registry.FormField, path string) *registry.Field {
	for _, ff := range all {
		if ff.Path == path && ff.Variant == "" {
			return ff.Field
		}
	}
	return nil
}

func parentPath(p string) string {
	if i := strings.LastIndex(p, "."); i >= 0 {
		return p[:i]
	}
	return ""
}

func sortedKeys(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Field — поле модели по ключу.
func (m *NodeFormModel) Field(key string) *NodeFormField {
	return m.byKey[key]
}

// ChoiceField — поле, которое занимает вариант группы (nil — вариант полей
// не несёт).
func (m *NodeFormModel) ChoiceField(choiceID, optionID string) *NodeFormField {
	for _, f := range m.Fields {
		if f.ChoiceID == choiceID && f.OptionID == optionID {
			return f
		}
	}
	return nil
}

// ValueLabel — подпись значения enum в выпадающем списке.
func (f *NodeFormField) ValueLabel(v string) string {
	if l, ok := f.ValueLabels[v]; ok {
		return l
	}
	return v
}

func nodeFormKindOf(f *registry.Field) NodeFormFieldKind {
	switch f.Type {
	case "enum":
		return NodeFormKindEnum
	case "bool":
		return NodeFormKindBool
	case "int", "uint8", "uint16", "uint32", "uint64":
		return NodeFormKindInt
	case "duration":
		return NodeFormKindDuration
	case "listable_string", "string_array":
		return NodeFormKindList
	case "awg_range":
		return NodeFormKindRange
	}
	return NodeFormKindString
}

// advisoryCodes — коды advisory поля, под которые попадает значение v.
func advisoryCodes(f *registry.Field, v interface{}) []string {
	if f == nil || v == nil {
		return nil
	}
	got := fmt.Sprintf("%v", v)
	var out []string
	for _, a := range f.Advisory {
		if a.When != nil {
			continue // условные advisory судит санитайзер, не форма
		}
		hit := len(a.Values) == 0
		for _, w := range a.Values {
			if fmt.Sprintf("%v", w) == got {
				hit = true
				break
			}
		}
		for _, w := range a.Except {
			if fmt.Sprintf("%v", w) == got {
				hit = false
				break
			}
		}
		if hit && a.Code != "" {
			out = append(out, a.Code)
		}
	}
	return out
}

// AdvisoryText — заголовок первого кода advisory, под который попадает
// набранное значение поля (пусто — предупреждать не о чем).
func (m *NodeFormModel) AdvisoryText(key, text string) string {
	f := m.byKey[key]
	if f == nil {
		return ""
	}
	v, ok, _ := f.parse(text)
	if !ok {
		return ""
	}
	return m.advisoryTitle(advisoryCodes(f.reg, v))
}

// OptionAdvisoryText — заголовок кода advisory варианта группы выбора.
func (m *NodeFormModel) OptionAdvisoryText(opt NodeFormChoiceOption) string {
	return m.advisoryTitle(opt.Advisory)
}

func (m *NodeFormModel) advisoryTitle(codes []string) string {
	for _, c := range codes {
		if title, _, ok := m.reg.WarningText(c, locale.GetLang(), nil); ok && title != "" {
			return title
		}
		return c
	}
	return ""
}

// parse переводит текст поля в значение тела. ok=false — поле пустое (ключ
// не пишется); err — текст не разбирается.
func (f *NodeFormField) parse(text string) (interface{}, bool, error) {
	trimmed := strings.TrimSpace(text)
	switch f.Kind {
	case NodeFormKindBool:
		on := trimmed == "true"
		def, _ := f.reg.Default.(bool)
		if on == def {
			// Значение совпало с дефолтом ядра — ключ не пишется.
			return nil, false, nil
		}
		return on, true, nil
	case NodeFormKindInt:
		if trimmed == "" {
			return nil, false, nil
		}
		n, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, false, fmt.Errorf("%s", locale.Tf("%s must be a whole number", locale.T(f.Label)))
		}
		return n, true, nil
	case NodeFormKindRange:
		if trimmed == "" {
			return nil, false, nil
		}
		if n, err := strconv.Atoi(trimmed); err == nil {
			return n, true, nil
		}
		return trimmed, true, nil
	case NodeFormKindList:
		parts := strings.Split(text, ",")
		out := make([]interface{}, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		if len(out) == 0 {
			return nil, false, nil
		}
		return out, true, nil
	}
	if trimmed == "" {
		return nil, false, nil
	}
	if f.Secret {
		// Секрет не обрезается: пробелы в пароле легальны.
		return text, true, nil
	}
	return trimmed, true, nil
}

// activeVariant — текущий вариант вариантного объекта в теле.
func (m *NodeFormModel) activeVariant(body map[string]interface{}, variantOf string) string {
	dpath, ok := m.discOf[variantOf]
	if !ok {
		return ""
	}
	raw, ok := registry.FormValue(body, dpath)
	if !ok {
		return ""
	}
	s, _ := raw.(string)
	return strings.TrimSpace(s)
}

// FormFromBody — значения полей формы из тела узла.
func (m *NodeFormModel) FormFromBody(body map[string]interface{}) NodeFormValues {
	v := NodeFormValues{Fields: map[string]string{}, Choices: map[string]string{}}
	for _, f := range m.Fields {
		if f.Variant != "" && m.activeVariant(body, f.VariantOf) != f.Variant {
			continue
		}
		raw, ok := registry.FormValue(body, f.Path)
		if !ok {
			continue
		}
		v.Fields[f.Key] = formTextOf(raw)
	}
	for _, c := range m.form.Choices {
		v.Choices[c.ID] = registry.ActiveFormOption(c, body)
	}
	return v
}

func formTextOf(raw interface{}) string {
	switch t := raw.(type) {
	case bool:
		if t {
			return "true"
		}
		return ""
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, it := range t {
			parts = append(parts, fmt.Sprint(it))
		}
		return strings.Join(parts, ", ")
	case []string:
		return strings.Join(t, ", ")
	case string:
		return t
	}
	return fmt.Sprint(raw)
}

// BodyFromForm — тело узла: base (текущее тело, у нового узла пустое) с
// путями формы, переписанными по values.
//
// Пустое поле ключ не пишет, bool, равный дефолту ядра, тоже; поле с
// `form.implies` пишет их пути вместе с собой и снимает, когда поле очищено
// (было задано в base). Смена варианта
// транспорта снимает вариантный объект целиком (ключи прежнего варианта
// новому не нужны), пустой дискриминатор — тоже. Смена варианта группы
// выбора чистит пути остальных вариантов и пишет `set` выбранного. Поле,
// скрытое visible_when на итоговом теле, остаётся таким, каким было в base.
// Значения `body.form.fixed` пишутся всегда. Прочие ключи base копируются
// как есть.
func (m *NodeFormModel) BodyFromForm(base map[string]interface{}, values NodeFormValues) (map[string]interface{}, error) {
	out := deepCopyMap(base)
	if out == nil {
		out = map[string]interface{}{}
	}
	if base == nil {
		base = map[string]interface{}{}
	}
	var firstErr error

	// Дискриминаторы: смена или снятие варианта — объект заново.
	selected := map[string]string{}
	for _, f := range m.Fields {
		if !f.Discriminator {
			continue
		}
		obj := parentPath(f.Path)
		sel := strings.TrimSpace(values.Fields[f.Key])
		selected[obj] = sel
		if sel == "" || sel != m.activeVariant(base, obj) {
			registry.DeleteFormValue(out, obj)
		}
		if sel != "" {
			registry.SetFormValue(out, f.Path, sel)
		}
	}

	for _, f := range m.Fields {
		if f.Discriminator {
			continue
		}
		if f.Variant != "" && selected[f.VariantOf] != f.Variant {
			continue // поле другого варианта: путь общий, не трогаем
		}
		// had — поле было задано в base: только его очистка снимает пути
		// implies (uTLS без отпечатка, пришедший из JSON, форма не трогает).
		_, had := registry.FormValue(base, f.Path)
		registry.DeleteFormValue(out, f.Path)
		if f.ChoiceID != "" && values.Choices[f.ChoiceID] != f.OptionID {
			if had {
				registry.ApplyFormImplies(f.reg, out, false)
			}
			continue // вариант не выбран — его путь снят
		}
		v, ok, err := f.parse(values.Fields[f.Key])
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if ok {
			registry.SetFormValue(out, f.Path, v)
		}
		if ok || had {
			registry.ApplyFormImplies(f.reg, out, ok)
		}
	}

	for _, c := range m.form.Choices {
		sel := values.Choices[c.ID]
		for _, o := range c.Options {
			if o.ID == sel {
				continue
			}
			for _, p := range o.Paths {
				registry.DeleteFormValue(out, p)
			}
		}
		for _, o := range c.Options {
			if o.ID != sel {
				continue
			}
			for _, p := range sortedKeys(o.Set) {
				registry.SetFormValue(out, p, o.Set[p])
			}
		}
	}
	m.reg.ApplyFormFixed(m.Scheme, out)

	// Скрытое поле не правится формой: значение base (и его implies)
	// возвращается на место.
	if st, ok := m.reg.FormState(m.Scheme, out); ok {
		visible := map[string]bool{}
		for _, p := range st.Visible {
			visible[p] = true
		}
		for _, f := range m.Fields {
			if f.ChoiceID != "" || f.Discriminator || visible[f.Path] {
				continue
			}
			if f.Variant != "" && selected[f.VariantOf] != f.Variant {
				continue
			}
			restoreFormPath(out, base, f.Path)
			if f.reg.Form != nil {
				for _, p := range sortedKeys(f.reg.Form.Implies) {
					restoreFormPath(out, base, p)
				}
			}
		}
	}
	return out, firstErr
}

// restoreFormPath возвращает в out значение пути из base (нет в base —
// путь снимается).
func restoreFormPath(out, base map[string]interface{}, path string) {
	registry.DeleteFormValue(out, path)
	if old, had := registry.FormValue(base, path); had {
		registry.SetFormValue(out, path, deepCopyValue(old))
	}
}

// Visible — какие поля формы (по ключу) видны на теле: visible_when
// реестра, у поля варианта — ещё и текущий вариант.
func (m *NodeFormModel) Visible(body map[string]interface{}) map[string]bool {
	out := map[string]bool{}
	st, ok := m.reg.FormState(m.Scheme, body)
	if !ok {
		return out
	}
	visible := map[string]bool{}
	for _, p := range st.Visible {
		visible[p] = true
	}
	for _, f := range m.Fields {
		if !visible[f.Path] {
			continue
		}
		if f.Variant != "" && m.activeVariant(body, f.VariantOf) != f.Variant {
			continue
		}
		out[f.Key] = true
	}
	return out
}

// fieldAt — поле модели по пути тела: у полей вариантов — поле текущего
// варианта.
func (m *NodeFormModel) fieldAt(body map[string]interface{}, path string) *NodeFormField {
	for _, f := range m.Fields {
		if f.Path != path {
			continue
		}
		if f.Variant != "" && m.activeVariant(body, f.VariantOf) != f.Variant {
			continue
		}
		return f
	}
	return nil
}

// Verdict — проверка тела перед сохранением: обязательные поля формы,
// диапазоны формы и санитайзер реестра (nodeflow.Sanitize →
// nodewarn.Summary). Пусто — тело годно.
//
// Санитайзер судит тело целиком: отказ уровня узла (Drop) — ошибка; снятый
// им путь формы — ошибка с текстом кода. Коды на путях вне формы (ключи из
// JSON) форму не останавливают — их покажет узел.
func (m *NodeFormModel) Verdict(body map[string]interface{}) error {
	if st, ok := m.reg.FormState(m.Scheme, body); ok {
		var missing []string
		for _, p := range st.Missing {
			if f := m.fieldAt(body, p); f != nil {
				missing = append(missing, locale.T(f.Label))
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s", locale.Tf("Required: %s", strings.Join(missing, ", ")))
		}
		for _, r := range st.OutOfRange {
			f := m.fieldAt(body, r.Path)
			if f == nil {
				continue
			}
			lo, hi := "", ""
			if r.Min != nil {
				lo = strconv.FormatFloat(*r.Min, 'f', -1, 64)
			}
			if r.Max != nil {
				hi = strconv.FormatFloat(*r.Max, 'f', -1, 64)
			}
			return fmt.Errorf("%s", locale.Tf("%s must be within %s…%s", locale.T(f.Label), lo, hi))
		}
	}

	res := nodeflow.Sanitize(m.Scheme, body)
	if res.Drop != nil {
		if msg := nodewarn.Summary(nodewarn.FromParsed([]nodeflow.Warning{*res.Drop})); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("%s", res.Drop.Code)
	}
	for _, f := range m.Fields {
		if f.Variant != "" && m.activeVariant(body, f.VariantOf) != f.Variant {
			continue
		}
		sent, has := registry.FormValue(body, f.Path)
		if !has {
			continue
		}
		kept, still := registry.FormValue(res.Clean, f.Path)
		if still && sameListLen(sent, kept) {
			continue
		}
		wpath := sanitizerPath(f.Path)
		for _, w := range res.Warnings {
			if w.Path != wpath && !strings.HasPrefix(w.Path, wpath+"[") && !strings.HasPrefix(w.Path, wpath+".") {
				continue
			}
			if msg := nodewarn.Summary(nodewarn.FromParsed([]nodeflow.Warning{w})); msg != "" {
				return fmt.Errorf("%s", msg)
			}
			return fmt.Errorf("%s", w.Code)
		}
		if !still {
			return fmt.Errorf("%s", locale.Tf("%s is invalid", locale.T(f.Label)))
		}
	}
	return nil
}

// sanitizerPath — путь формы в записи санитайзера: элемент массива в
// скобках ("peers.0.address" → "peers[0].address").
func sanitizerPath(p string) string {
	parts := strings.Split(p, ".")
	var b strings.Builder
	for i, s := range parts {
		if _, err := strconv.Atoi(s); err == nil && i > 0 {
			b.WriteString("[" + s + "]")
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s)
	}
	return b.String()
}

// sameListLen — у списка санитайзер не снял ни одного элемента (снятый
// элемент — тоже отказ формы: человек его набрал).
func sameListLen(a, b interface{}) bool {
	la, okA := a.([]interface{})
	if !okA {
		return true
	}
	switch lb := b.(type) {
	case []interface{}:
		return len(la) == len(lb)
	case []string:
		return len(la) == len(lb)
	}
	return true
}

// DefaultTag — тег нового узла, когда Name пуст: хост сервера (без схемы,
// порта и пути), иначе имя схемы.
func (m *NodeFormModel) DefaultTag(body map[string]interface{}) string {
	for _, f := range m.Fields {
		if !f.host {
			continue
		}
		if raw, ok := registry.FormValue(body, f.Path); ok {
			if s, isStr := raw.(string); isStr {
				if host := hostOfServer(s); host != "" {
					return host
				}
			}
		}
	}
	return m.Scheme
}

// hostOfServer — хост из адреса сервера: без схемы, порта и пути.
func hostOfServer(server string) string {
	s := strings.TrimSpace(server)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if strings.HasPrefix(s, "[") {
		if i := strings.Index(s, "]"); i > 0 {
			s = s[1:i]
		}
	} else if i := strings.LastIndex(s, ":"); i >= 0 && strings.Count(s, ":") == 1 {
		s = s[:i]
	}
	return s
}

// Document — документ узла из тела формы: один узел в `outbounds[]` или
// `endpoints[]` (по схеме, как при эмиссии). Тело перед этим проверяется
// (Verdict) и проходит санитайзер и эмиттер реестра — тот же конвейер, что
// у правки готового узла формой: тело формы не авторское, и дефолты реестра
// (default_when — allowed_ips пира, mtu 1280 у AmneziaWG) в нём
// материализуются (контракт 1.1.123, TASKS_LXBOX §120 п. 11). Пустой tag —
// DefaultTag.
func (m *NodeFormModel) Document(tag string, body map[string]interface{}) ([]byte, string, error) {
	if err := m.Verdict(body); err != nil {
		return nil, "", err
	}
	stamped, err := m.CleanBody(body)
	if err != nil {
		return nil, "", err
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		tag = m.DefaultTag(body)
	}
	tagJSON, err := json.Marshal(tag)
	if err != nil {
		return nil, "", err
	}
	// tag первым ключом, порядок остальных — эмиттера.
	node := string(tagJSON)
	inner := strings.TrimSpace(string(stamped))
	inner = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(inner, "{"), "}"))
	entry := `{"tag":` + node
	if inner != "" {
		entry += "," + inner
	}
	entry += "}"
	doc := struct {
		Outbounds []json.RawMessage `json:"outbounds,omitempty"`
		Endpoints []json.RawMessage `json:"endpoints,omitempty"`
	}{}
	if config.IsEndpointScheme(m.Scheme) {
		doc.Endpoints = []json.RawMessage{json.RawMessage(entry)}
	} else {
		doc.Outbounds = []json.RawMessage{json.RawMessage(entry)}
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, "", err
	}
	return raw, tag, nil
}

// CleanBody — тело после санитайзера и эмиттера реестра с "type" первым
// ключом: так его пишет и правка узла формой. Отказ уровня узла — ошибка.
func (m *NodeFormModel) CleanBody(body map[string]interface{}) (json.RawMessage, error) {
	res := nodeflow.Sanitize(m.Scheme, body)
	if res.Drop != nil {
		if msg := nodewarn.Summary(nodewarn.FromParsed([]nodeflow.Warning{*res.Drop})); msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, fmt.Errorf("%s", res.Drop.Code)
	}
	emitted, err := nodeflow.Emit(m.Scheme, res.Clean)
	if err != nil {
		return nil, err
	}
	// Emit снимает managed-ключ "type" — вернуть его обязан пишущий тело.
	return config.StampBodyType(m.Scheme, emitted, body)
}

// NodeFormClassicBackend — ядро запущено лаунчером в классическом режиме
// (`sing-box run`): канала челленджа второго фактора нет (SPEC 167 §2.5),
// форма добавляет об этом строку под полем с ClassicNote.
func NodeFormClassicBackend() bool {
	ac := core.GetController()
	return ac != nil && ac.BackendMode() == core.BackendClassic
}

func deepCopyMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = deepCopyValue(v)
	}
	return out
}

func deepCopyValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		return deepCopyMap(t)
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, it := range t {
			out[i] = deepCopyValue(it)
		}
		return out
	}
	return v
}
