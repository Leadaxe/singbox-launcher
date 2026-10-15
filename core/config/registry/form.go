package registry

import "strings"

// Состояние формы узла по реестру (контракт 1.1.122, REGISTRY_DESIGN §3).
//
// Чистые функции от тела узла: ни Fyne, ни имён схем. Их читают форма
// лаунчера и раннер корпуса contract/corpus/form — одна трактовка атрибутов
// `form` для UI и для сверки с LxBox.
//
// go1.20-совместимо: без slices/maps/min/max.

// FormState — что форма делает с телом: какие поля рисует, какие требует,
// какой вариант групп выбора активен и где значение вне диапазона формы.
//
// Пути — от корня тела ("tls.peer_fingerprint"), в порядке order реестра.
type FormState struct {
	// Visible — поля с `form`, которые форма рисует (visible_when истинно,
	// и предок не скрыт).
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

// FormState вычисляет состояние формы схемы по телу; false — у схемы нет
// формы по реестру (секции body.form).
//
// Скрытое поле не рисуется и не валидируется, но из тела не снимается —
// функция тело не меняет вовсе.
func (r *Registry) FormState(scheme string, body map[string]interface{}) (FormState, bool) {
	b, ok := r.bodies[scheme]
	if !ok || b.Form == nil {
		return FormState{}, false
	}
	if body == nil {
		body = map[string]interface{}{}
	}
	st := FormState{Choices: map[string]string{}}
	walkForm(&st, "", b.Order, b.Fields, body, true)
	for _, c := range b.Form.Choices {
		st.Choices[c.ID] = ActiveFormOption(c, body)
	}
	return st, true
}

// ActiveFormOption — id активного варианта группы: первый вариант, у
// которого задан хоть один путь из paths; если не задан ни один — вариант с
// пустыми paths; иначе пусто.
func ActiveFormOption(c FormChoice, body map[string]interface{}) string {
	fallback := ""
	for _, o := range c.Options {
		if len(o.Paths) == 0 {
			if fallback == "" {
				fallback = o.ID
			}
			continue
		}
		for _, p := range o.Paths {
			if _, present := bodyValue(body, p); present {
				return o.ID
			}
		}
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

func walkForm(st *FormState, prefix string, order []string, fields map[string]*Field, body map[string]interface{}, parentVisible bool) {
	for _, name := range order {
		f := fields[name]
		if f == nil || f.Wildcard || f.Managed {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		visible := parentVisible && FieldVisible(f, body)
		if f.Form != nil && visible {
			st.Visible = append(st.Visible, path)
			v, present := bodyValue(body, path)
			if FormRequired(f, path, body) {
				st.Required = append(st.Required, path)
				if !present {
					st.Missing = append(st.Missing, path)
				}
			}
			if present && (f.Form.Min != nil || f.Form.Max != nil) {
				if n, ok := formNumber(v); ok {
					if (f.Form.Min != nil && n < *f.Form.Min) || (f.Form.Max != nil && n > *f.Form.Max) {
						st.OutOfRange = append(st.OutOfRange, FormRangeViolation{Path: path, Value: n, Min: f.Form.Min, Max: f.Form.Max})
					}
				}
			}
		}
		if f.Type == "object" && len(f.Fields) > 0 {
			walkForm(st, path, f.Order, f.Fields, body, visible)
		}
	}
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
