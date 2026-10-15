package registry

import (
	"sort"
	"strconv"
	"strings"
)

// Состояние формы узла по реестру (контракт 1.1.122, REGISTRY_DESIGN §3 SPEC
// 167; общие секции, варианты, fixed и массив [0] — контракт 1.1.123, SPEC
// 168 §2.4).
//
// Чистые функции от тела узла: ни Fyne, ни имён схем. Их читают форма
// лаунчера и раннер корпуса contract/corpus/form — одна трактовка атрибутов
// `form` для UI и для сверки с LxBox.
//
// go1.20-совместимо: без slices/maps/min/max.

// FormState — что форма делает с телом: какие поля рисует, какие требует,
// какой вариант групп выбора активен и где значение вне диапазона формы.
//
// Пути — от корня тела ("tls.peer_fingerprint", "peers.0.address",
// "transport.path"), в порядке order реестра.
type FormState struct {
	// Visible — поля с `form`, которые форма рисует (visible_when истинно,
	// и предок не скрыт). Сюда же попадают контейнеры с `form` (объект,
	// массив, ref-поле общей секции) и дискриминатор варианта
	// ("transport.type"). Пути `body.form.fixed` не рисуются и не входят.
	Visible []string
	// Required — видимые поля, обязательность формы которых действует
	// (required и не снята required_unless).
	Required []string
	// Missing — обязательные поля, которых в теле нет (форма не сохранит).
	Missing []string
	// Choices — id группы выбора → id активного варианта.
	Choices map[string]string
	// OutOfRange — видимые заданные поля вне min/max формы.
	OutOfRange []FormRangeViolation
}

// FormRangeViolation — значение поля вне диапазона формы.
type FormRangeViolation struct {
	Path  string
	Value float64
	Min   *float64
	Max   *float64
}

// FormField — поле формы схемы (FormFields): что рисовать, где и чем.
type FormField struct {
	// Path — путь тела от корня. У полей варианта — без имени варианта
	// ("transport.path" у ws и у http один и тот же путь, различает Variant).
	Path string
	// Field — поле реестра (у дискриминатора — синтетический enum).
	Field *Field
	// Block — блок формы: свой `form.block`, иначе блок ref-поля, которое
	// подключило общую секцию; у поля варианта группы выбора — блок группы.
	Block string
	// Variant — имя варианта вариантной секции ("ws"): поле видно, только
	// когда дискриминатор VariantOf равен Variant. Пусто вне вариантов.
	Variant string
	// VariantOf — путь вариантного объекта ("transport"), пусто вне вариантов.
	VariantOf string
	// Discriminator — поле выбора варианта ("transport.type"): пусто =
	// объект не пишется вовсе.
	Discriminator bool
	// ChoiceID/OptionID — путь занимает вариант группы выбора: поле рисует
	// группа, а не блок.
	ChoiceID string
	OptionID string
}

// FormState вычисляет состояние формы схемы по телу; false — у схемы нет
// формы по реестру (секции body.form).
//
// Значения `body.form.fixed` перед вычислением кладутся поверх тела (форма
// пишет их всегда), сами пути fixed не рисуются.
//
// Скрытое поле не рисуется и не валидируется, но из тела не снимается —
// функция тело не меняет вовсе.
func (r *Registry) FormState(scheme string, body map[string]interface{}) (FormState, bool) {
	b, ok := r.bodies[scheme]
	if !ok || b.Form == nil {
		return FormState{}, false
	}
	eval := body
	if eval == nil {
		eval = map[string]interface{}{}
	}
	if len(b.Form.Fixed) > 0 {
		eval = copyFormMap(eval)
		applyFixed(eval, b.Form.Fixed)
	}
	st := FormState{Choices: map[string]string{}}
	w := newFormWalk(b, eval)
	w.visit = func(ff FormField, visible bool) {
		if !visible {
			return
		}
		st.Visible = append(st.Visible, ff.Path)
		if ff.Discriminator {
			return
		}
		f := ff.Field
		v, present := bodyValue(eval, ff.Path)
		if FormRequired(f, ff.Path, eval) {
			st.Required = append(st.Required, ff.Path)
			if !present {
				st.Missing = append(st.Missing, ff.Path)
			}
		}
		if present && (f.Form.Min != nil || f.Form.Max != nil) {
			if n, ok := formNumber(v); ok {
				if (f.Form.Min != nil && n < *f.Form.Min) || (f.Form.Max != nil && n > *f.Form.Max) {
					st.OutOfRange = append(st.OutOfRange, FormRangeViolation{Path: ff.Path, Value: n, Min: f.Form.Min, Max: f.Form.Max})
				}
			}
		}
	}
	w.fields("", b.Order, b.Fields, formCtx{visible: true})
	for _, c := range b.Form.Choices {
		st.Choices[c.ID] = ActiveFormOption(c, eval)
	}
	return st, true
}

// FormFields — поля, которые форма схемы рисует, в порядке order реестра:
// с учётом подключения общих секций (`form` у ref-поля), forbidden_for
// схемы, exclude_fields/exclude_variants и fixed. Поля всех вариантов
// вариантной секции идут подряд за её дискриминатором. Контейнеры (объект,
// массив, ref-поле) не входят — форма их не рисует. Видимость по телу —
// FormState.Visible. nil — у схемы нет формы.
func (r *Registry) FormFields(scheme string) []FormField {
	b, ok := r.bodies[scheme]
	if !ok || b.Form == nil {
		return nil
	}
	option := map[string][2]string{}
	block := map[string]string{}
	for _, c := range b.Form.Choices {
		for _, o := range c.Options {
			for _, p := range o.Paths {
				option[p] = [2]string{c.ID, o.ID}
				block[p] = c.Block
			}
		}
	}
	var out []FormField
	w := newFormWalk(b, nil)
	w.visit = func(ff FormField, _ bool) {
		if !ff.Discriminator && isFormContainer(ff.Field) {
			return
		}
		if ch, ok := option[ff.Path]; ok {
			ff.ChoiceID, ff.OptionID = ch[0], ch[1]
			ff.Block = block[ff.Path]
		}
		out = append(out, ff)
	}
	w.fields("", b.Order, b.Fields, formCtx{visible: true})
	return out
}

// ApplyFormFixed пишет в тело значения `body.form.fixed` схемы (форма
// пишет их всегда). Схема без формы — тело не меняется.
func (r *Registry) ApplyFormFixed(scheme string, body map[string]interface{}) {
	b, ok := r.bodies[scheme]
	if !ok || b.Form == nil || body == nil {
		return
	}
	applyFixed(body, b.Form.Fixed)
}

// ActiveFormOption — id активного варианта группы по «самому длинному
// совпадению» (контракт 1.1.123): среди вариантов, у которых задан хоть
// один путь из paths, побеждает тот, у кого задано больше путей из
// paths ∪ ключей set; при равенстве — первый. Если не задан ни один —
// вариант с пустыми paths; иначе пусто.
//
// У групп с непересекающимися вариантами (openconnect) это то же правило
// «первый вариант с заданным путём» контракта 1.1.122.
func ActiveFormOption(c FormChoice, body map[string]interface{}) string {
	fallback, best, bestScore := "", "", 0
	for _, o := range c.Options {
		if len(o.Paths) == 0 {
			if fallback == "" {
				fallback = o.ID
			}
			continue
		}
		hit := false
		for _, p := range o.Paths {
			if _, present := bodyValue(body, p); present {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		score := 0
		seen := map[string]bool{}
		count := func(p string) {
			if seen[p] {
				return
			}
			seen[p] = true
			if _, present := bodyValue(body, p); present {
				score++
			}
		}
		for _, p := range o.Paths {
			count(p)
		}
		for _, p := range sortedSetKeys(o.Set) {
			count(p)
		}
		if score > bestScore {
			best, bestScore = o.ID, score
		}
	}
	if best != "" {
		return best
	}
	return fallback
}

// FieldVisible — истинно ли visible_when поля на теле (без учёта предков).
func FieldVisible(f *Field, body map[string]interface{}) bool {
	if f == nil || f.Form == nil || f.Form.VisibleWhen == nil {
		return true
	}
	return f.Form.VisibleWhen.HoldsOn(body)
}

// FormRequired — действует ли обязательность формы поля по пути path:
// required задан и не снят required_unless (любой путь из set задан либо
// в том же объекте не задан любой сосед из absent).
func FormRequired(f *Field, path string, body map[string]interface{}) bool {
	if f == nil || f.Form == nil || !f.Form.Required {
		return false
	}
	ru := f.Form.RequiredUnless
	if ru == nil {
		return true
	}
	for _, p := range ru.Set {
		if _, present := bodyValue(body, p); present {
			return false
		}
	}
	prefix := ""
	if i := strings.LastIndex(path, "."); i >= 0 {
		prefix = path[:i+1]
	}
	for _, name := range ru.Absent {
		if _, present := bodyValue(body, prefix+name); !present {
			return false
		}
	}
	return true
}

// FormValue — значение по пути тела формы ("peers.0.address"); пустая
// строка и nil — «не задано».
func FormValue(body map[string]interface{}, path string) (interface{}, bool) {
	return bodyValue(body, path)
}

// SetFormValue пишет значение по пути тела формы. Недостающие объекты
// создаются; числовой сегмент — элемент массива: нет массива — создаётся,
// короче индекса — дополняется пустыми объектами (форма правит [0]).
// Значение на пути, которое не объект (не массив), заменяется объектом.
func SetFormValue(body map[string]interface{}, path string, v interface{}) {
	if body == nil || path == "" {
		return
	}
	setFormPath(body, strings.Split(path, "."), v)
}

func setFormPath(m map[string]interface{}, parts []string, v interface{}) {
	key := parts[0]
	if len(parts) == 1 {
		m[key] = v
		return
	}
	if isIndexSegment(parts[1]) {
		list, _ := m[key].([]interface{})
		idx, _ := strconv.Atoi(parts[1])
		for len(list) <= idx {
			list = append(list, map[string]interface{}{})
		}
		m[key] = list
		if len(parts) == 2 {
			list[idx] = v
			return
		}
		elem, ok := list[idx].(map[string]interface{})
		if !ok {
			elem = map[string]interface{}{}
			list[idx] = elem
		}
		setFormPath(elem, parts[2:], v)
		return
	}
	child, ok := m[key].(map[string]interface{})
	if !ok {
		child = map[string]interface{}{}
		m[key] = child
	}
	setFormPath(child, parts[1:], v)
}

// DeleteFormValue снимает ключ по пути тела формы и чистит ставших пустыми
// предков: пустой объект снимается, массив из одного пустого элемента —
// тоже (форма правит только [0]). Непустые предки не трогаются.
func DeleteFormValue(body map[string]interface{}, path string) {
	if body == nil || path == "" {
		return
	}
	deleteFormPath(body, strings.Split(path, "."))
}

// deleteFormPath — true, если узел стал пустым и родитель вправе его снять.
func deleteFormPath(node interface{}, parts []string) bool {
	switch n := node.(type) {
	case map[string]interface{}:
		key := parts[0]
		if len(parts) == 1 {
			delete(n, key)
			return len(n) == 0
		}
		child, ok := n[key]
		if !ok {
			return false
		}
		if deleteFormPath(child, parts[1:]) {
			delete(n, key)
		}
		return len(n) == 0
	case []interface{}:
		idx, err := strconv.Atoi(parts[0])
		if err != nil || !isIndexSegment(parts[0]) || idx >= len(n) {
			return false
		}
		if len(parts) == 1 {
			// Элемент целиком форма не снимает: индексы соседей сдвинулись бы.
			return false
		}
		if deleteFormPath(n[idx], parts[1:]) {
			if m, ok := n[idx].(map[string]interface{}); ok && len(m) == 0 && len(n) == 1 {
				return true
			}
		}
		return false
	}
	return false
}

// formCtx — состояние обхода формы на уровне объекта.
type formCtx struct {
	visible   bool
	block     string // блок ref-поля, подключившего общую секцию
	variant   string
	variantOf string
}

// formWalk — общий обход полей формы схемы: FormFields (body == nil — все
// варианты, видимость не судится) и FormState (тело — один вариант по
// дискриминатору, видимость по visible_when).
type formWalk struct {
	scheme   string
	fixed    map[string]interface{}
	excluded map[string]bool
	body     map[string]interface{}
	visit    func(ff FormField, visible bool)
}

func newFormWalk(b *BodySchema, body map[string]interface{}) *formWalk {
	return &formWalk{scheme: b.Scheme, fixed: b.Form.Fixed, excluded: map[string]bool{}, body: body}
}

func (w *formWalk) fields(prefix string, order []string, fields map[string]*Field, ctx formCtx) {
	for _, name := range order {
		f := fields[name]
		if f == nil || f.Wildcard || f.Managed {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if w.excluded[path] || !f.AllowedForScheme(w.scheme) {
			// Поле, запрещённое схеме (reality у hysteria2), отпадает вместе
			// с поддеревом — форма его не предлагает.
			continue
		}
		w.field(path, f, ctx)
	}
}

func (w *formWalk) field(path string, f *Field, ctx formCtx) {
	shared := f.Ref != "" && f.Type == "object"
	if shared && f.Form == nil {
		// Общая секция без `form` у ref-поля в форму схемы не входит.
		return
	}
	visible := ctx.visible && (w.body == nil || FieldVisible(f, w.body))
	if shared {
		if f.Form.Block != "" {
			ctx.block = f.Form.Block
		}
		for _, x := range f.Form.ExcludeFields {
			w.excluded[path+"."+x] = true
		}
	}
	if f.Form != nil {
		if _, isFixed := w.fixed[path]; !isFixed {
			block := f.Form.Block
			if block == "" {
				block = ctx.block
			}
			w.visit(FormField{Path: path, Field: f, Block: block, Variant: ctx.variant, VariantOf: ctx.variantOf}, visible)
		}
	}
	inner := formCtx{visible: visible, block: ctx.block, variant: ctx.variant, variantOf: ctx.variantOf}
	switch {
	case len(f.Variants) > 0:
		if f.Form == nil {
			return
		}
		dpath := path + "." + f.Discriminator
		if _, isFixed := w.fixed[dpath]; !isFixed && f.DiscriminatorField != nil {
			w.visit(FormField{Path: dpath, Field: f.DiscriminatorField, Block: ctx.block, Discriminator: true}, visible)
		}
		excluded := map[string]bool{}
		for _, v := range f.Form.ExcludeVariants {
			excluded[v] = true
		}
		names := f.VariantValues
		if w.body != nil {
			names = nil
			if raw, ok := bodyValue(w.body, dpath); ok {
				if s, isStr := raw.(string); isStr {
					names = []string{strings.TrimSpace(s)}
				}
			}
		}
		for _, name := range names {
			v := f.Variants[name]
			if v == nil || excluded[name] {
				continue
			}
			vctx := inner
			vctx.variant, vctx.variantOf = name, path
			w.fields(path, v.Order, v.Fields, vctx)
		}
	case f.Type == "object" && len(f.Fields) > 0:
		w.fields(path, f.Order, f.Fields, inner)
	case f.Type == "array" && f.Form != nil && f.Items != nil && len(f.Items.Fields) > 0:
		// Массив объектов с `form`: форма правит элемент [0].
		w.fields(path+".0", f.Items.Order, f.Items.Fields, inner)
	}
}

// isFormContainer — поле-контейнер (объект, массив объектов, вариантный
// объект): форма его не рисует, рисует его поля.
func isFormContainer(f *Field) bool {
	if f == nil {
		return false
	}
	return f.Type == "object" || f.Type == "array" || len(f.Variants) > 0
}

func applyFixed(body map[string]interface{}, fixed map[string]interface{}) {
	for _, p := range sortedSetKeys(fixed) {
		SetFormValue(body, p, fixed[p])
	}
}

func sortedSetKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// copyFormMap — глубокая копия тела (карты и списки), чтобы наложение
// fixed не меняло тело вызывающего.
func copyFormMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = copyFormValue(v)
	}
	return out
}

func copyFormValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		return copyFormMap(t)
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, it := range t {
			out[i] = copyFormValue(it)
		}
		return out
	}
	return v
}

func formNumber(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	}
	return 0, false
}
