package template

// Язык шаблона, LxBox §578 (контракт 1.1.86, TEMPLATE_LANG §4.8, §6.5–§6.7):
//
//   - `for_each` у пресета — тело повторяется для каждого узла конфига
//     заданного типа;
//   - доступ к узлу: `@<as>` — финальный тег, `@<as>.<поле записи>` —
//     поле записи из закрытого перечня, `@<as>.body.<путь>` — поле тела;
//   - `{"#tpl": "…@{имя}…"}` — составная строка на месте значения.
//
// Имена узла не перечисляются заранее (путь тела произволен): обходчик
// считает объявленным всякое имя с префиксом `<as>.` (canonCtx.dynPrefixes),
// а значения кладёт в resolved плоской картой по всем путям тела.

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// PresetForEach — ключ пресета `for_each`.
type PresetForEach struct {
	// NodeType — значение поля `type` тела узла.
	NodeType string `json:"node_type"`
	// As — имя, под которым узел виден телу пресета.
	As string `json:"as"`
	// Filter — условие языка #if; nil — истина.
	Filter interface{} `json:"filter,omitempty"`
}

// Valid — обязательные ключи на месте.
func (f *PresetForEach) Valid() bool {
	return f != nil && strings.TrimSpace(f.NodeType) != "" && strings.TrimSpace(f.As) != "" &&
		!strings.ContainsAny(f.As, ".@{} ")
}

// PresetNode — узел конфига для `for_each`: финальный тег, тело в конфиге и
// поле записи `skip_presets` (у узла подписки записи нет — false).
type PresetNode struct {
	Tag         string
	Body        map[string]interface{}
	SkipPresets bool
}

// PresetNodeRecordFields — поля записи узла, видимые пресету. Перечень
// закрытый, расширяется контрактом.
var PresetNodeRecordFields = map[string]bool{"skip_presets": true}

// NodeScope — имена одного узла под именем `as` для обходчика.
type NodeScope struct {
	As       string
	Resolved map[string]ResolvedVar
	Types    map[string]string
}

// NewNodeScope раскладывает узел в плоскую карту имён.
//
// Пустая строка и null в теле значения не дают: имя остаётся объявленным без
// значения, ключ снимается Dropped-каскадом (как у необязательной переменной).
func NewNodeScope(as string, n PresetNode) *NodeScope {
	s := &NodeScope{As: as, Resolved: map[string]ResolvedVar{}, Types: map[string]string{}}
	s.Resolved[as] = ResolvedVar{Scalar: n.Tag, Raw: n.Tag}
	s.Types[as] = "text"
	skip := "false"
	if n.SkipPresets {
		skip = "true"
	}
	s.Resolved[as+".skip_presets"] = ResolvedVar{Scalar: skip, Raw: n.SkipPresets}
	s.Types[as+".skip_presets"] = "bool"
	flattenNodeBody(as+".body", n.Body, s.Resolved)
	return s
}

func flattenNodeBody(prefix string, v interface{}, out map[string]ResolvedVar) {
	switch x := v.(type) {
	case nil:
		return
	case string:
		if x == "" {
			return
		}
		out[prefix] = ResolvedVar{Scalar: x, Raw: x}
	case bool:
		out[prefix] = ResolvedVar{Scalar: strconv.FormatBool(x), Raw: x}
	case json.Number:
		out[prefix] = ResolvedVar{Scalar: x.String(), Raw: x}
	case float64:
		out[prefix] = ResolvedVar{Scalar: strconv.FormatFloat(x, 'f', -1, 64), Raw: x}
	case map[string]interface{}:
		out[prefix] = ResolvedVar{Raw: x}
		for k, child := range x {
			flattenNodeBody(prefix+"."+k, child, out)
		}
	case []interface{}:
		out[prefix] = ResolvedVar{Raw: x}
	}
}

// VarsMap — скалярные значения узла для гейтов (#enable), которые считаются
// по плоской карте строк.
func (s *NodeScope) VarsMap(base map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(s.Resolved))
	for k, v := range base {
		out[k] = v
	}
	for k, r := range s.Resolved {
		if _, isObj := r.Raw.(map[string]interface{}); isObj {
			continue
		}
		if _, isList := r.Raw.([]interface{}); isList {
			continue
		}
		out[k] = r.Scalar
	}
	return out
}

// EvalFilter — значение `filter` для узла; nil-фильтр истинен.
func (f *PresetForEach) EvalFilter(scope *NodeScope, varsMap map[string]string, target TargetSpec) bool {
	if f == nil || f.Filter == nil {
		return true
	}
	resolved := make(map[string]ResolvedVar, len(varsMap)+len(scope.Resolved))
	types := make(map[string]string, len(scope.Types))
	for k, v := range varsMap {
		resolved[k] = ResolvedVar{Scalar: v}
	}
	for k, v := range scope.Resolved {
		resolved[k] = v
	}
	for k, v := range scope.Types {
		types[k] = v
	}
	return evaluateCond(f.Filter, types, resolved, target.Normalized())
}

// tplKey — ключ составной строки.
const tplKey = "#tpl"

// TplKey — экспорт для валидаторов вне пакета.
const TplKey = tplKey

var tplSlot = regexp.MustCompile(`@\{([^{}]+)\}`)

// evalTplCanon вычисляет {"#tpl": "…"}: вставка `@{имя}` заменяется
// скалярным значением имени; `@имя` без скобок внутри строки — литерал.
// Имя неизвестно, значение пустое или не скаляр → droppedValue. Объект с
// `#tpl` и другими ключами либо не строка под ключом — ошибка шаблона: на
// загрузке её отвергает валидатор, здесь — template_unknown_directive и
// droppedValue.
func evalTplCanon(x map[string]interface{}, ctx *canonCtx) interface{} {
	pattern, ok := x[tplKey].(string)
	if len(x) != 1 || !ok {
		ctx.warnDirective(tplKey)
		return droppedValue{}
	}
	dropped := false
	out := tplSlot.ReplaceAllStringFunc(pattern, func(m string) string {
		if dropped {
			return ""
		}
		name := strings.TrimSpace(m[2 : len(m)-1])
		text := tplScalar(name, ctx)
		if text == "" {
			dropped = true
		}
		return text
	})
	if dropped {
		ctx.emptyRefs++
		return droppedValue{}
	}
	return out
}

// tplScalar — скалярный текст имени для вставки; "" — значения нет.
func tplScalar(name string, ctx *canonCtx) string {
	if isRuntimeGlobalRef(name) {
		if s, _, _, found := lookupVarScalar(name, ctx.resolved, ctx.target); found {
			return s
		}
		ctx.warnUndeclared(name)
		return ""
	}
	r, ok := ctx.resolved[name]
	if !ok {
		if !ctx.isDeclared(name) {
			ctx.warnUndeclared(name)
		}
		return ""
	}
	switch v := r.Raw.(type) {
	case nil:
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return "" // объект или массив — не скаляр
	}
	if ctx.varTypes[name] == "text_list" {
		return ""
	}
	return r.Scalar
}

// isDeclared — имя объявлено шаблоном или принадлежит пространству узла.
func (c *canonCtx) isDeclared(name string) bool {
	if c.declared[name] {
		return true
	}
	for _, p := range c.dynPrefixes {
		if name == p || strings.HasPrefix(name, p+".") {
			return true
		}
	}
	return false
}

// deepCopyRaw — копия значения узла для подстановки (дерево не делится).
func deepCopyRaw(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for k, c := range x {
			out[k] = deepCopyRaw(c)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, c := range x {
			out[i] = deepCopyRaw(c)
		}
		return out
	}
	return v
}

// SubstituteVarsInJSONCanonScoped — канонический обход с пространством узла
// `for_each` (nil — без пространства узла). emptyRef — хоть одна ссылка на
// имя (`@имя`, вставка `#tpl`) не дала значения: Dropped или нулевое значение
// JSON. Сборка пресета по нему отличает правило, условия которого пропали из-за
// пустой переменной, от правила без условий по замыслу автора (SPEC 152).
func SubstituteVarsInJSONCanonScoped(data []byte, vars []TemplateVar, resolved map[string]ResolvedVar, target TargetSpec, node *NodeScope) (out json.RawMessage, warnings []TemplateWarning, emptyRef bool, err error) {
	if node == nil {
		ctx, err := substituteCanonCtx(data, vars, resolved, target, nil)
		if err != nil {
			return nil, nil, false, err
		}
		return ctx.out, ctx.warnings, ctx.emptyRefs > 0, nil
	}
	merged := make(map[string]ResolvedVar, len(resolved)+len(node.Resolved))
	for k, v := range resolved {
		merged[k] = v
	}
	for k, v := range node.Resolved {
		merged[k] = v
	}
	decls := append([]TemplateVar(nil), vars...)
	for name, typ := range node.Types {
		decls = append(decls, TemplateVar{Name: name, Type: typ})
	}
	ctx, err := substituteCanonCtx(data, decls, merged, target, []string{node.As})
	if err != nil {
		return nil, nil, false, err
	}
	return ctx.out, ctx.warnings, ctx.emptyRefs > 0, nil
}
