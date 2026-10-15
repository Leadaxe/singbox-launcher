package template

// Обход узлов в пресете (SPEC 166, контракт 1.1.119, TEMPLATE_LANG §6.5–§6.7)
// и составная строка `#tpl` (§4.8):
//
//   - `for_each_node` — тело пресета повторяется для каждого узла конфига,
//     прошедшего `#filter`; узел виден под фиксированным именем `@node`;
//   - `for_each_node_do` — обход ради сбора значений: `#do` дописывает
//     (`#append`) значения узлов в text_list-переменные пресета, `#final`
//     один раз преобразует собранное (`#unique` → `#sort` → `#limit`), тело
//     раскрывается ОДИН раз без `@node`;
//   - доступ к узлу: `@node` — финальный тег, `@node.<поле записи>` — поле
//     записи из закрытого перечня, `@node.body.<путь>` — поле тела;
//   - `{"#tpl": "…@{имя}…"}` — составная строка на месте значения.
//
// Прежний ключ `for_each` (node_type/as/filter, контракт 1.1.86) — ошибка
// загрузки: легаси-чтения нет, шаблон у каждой стороны свой.
//
// Имена узла не перечисляются заранее (путь тела произволен): обходчик
// считает объявленным всякое имя с префиксом `node.` (canonCtx.dynPrefixes),
// а значения кладёт в resolved плоской картой по всем путям тела.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// NodeLoopVar — фиксированное имя узла в теле и в `#filter` (§6.6). Имя
// `node` и всё под `node.` зарезервированы: переменная с таким именем —
// ошибка загрузки.
const NodeLoopVar = "node"

// Ключи пресета и слова движка обхода.
const (
	NodeLoopKey      = "for_each_node"
	NodeLoopDoKey    = "for_each_node_do"
	legacyForEachKey = "for_each"

	nodeFilterKey = "#filter"
	nodeDoKey     = "#do"
	nodeFinalKey  = "#final"

	nodeDoAppend = "#append"

	nodeFinalUnique = "#unique"
	nodeFinalSort   = "#sort"
	nodeFinalLimit  = "#limit"
)

// WizardUIInternal — `wizard_ui: "internal"` у переменной пресета (§2.5
// SPEC 166): только для сборки — в формах не показывается, значение из
// состояния и бэкапа игнорируется, пишет её только `#do`.
const WizardUIInternal = "internal"

// NodeLoop — ключ пресета `for_each_node`: тело повторяется для каждого
// узла, прошедшего Filter.
type NodeLoop struct {
	// Filter — условие языка #if (форма cond); nil — истина.
	Filter interface{}

	raw     map[string]interface{}
	invalid string
}

// NodeLoopDo — ключ пресета `for_each_node_do`: обход ради сбора значений в
// переменные пресета; тело раскрывается один раз после обхода.
type NodeLoopDo struct {
	// Filter — условие языка #if (форма cond); nil — истина.
	Filter interface{}
	// Do — операции `#do` по переменным, по алфавиту имён.
	Do []NodeDoOp
	// Final — `#final` по имени переменной (без "@").
	Final map[string]NodeFinalOps

	raw     map[string]interface{}
	invalid string
}

// NodeDoOp — одна операция `#do`: `"@<Var>": {"#append": <Append>}` либо,
// под условием (TEMPLATE_LANG §6.7.1, контракт 1.1.125),
// `"@<Var>": {"#if…": [<Cond>, {"#append": <Value>}, {"#append": <Else>}]}`
// — тело `#if` любой формы §4.1.
type NodeDoOp struct {
	// Var — имя переменной пресета без "@".
	Var string
	// Append — правая часть `#append` безусловной операции: строка
	// (`"@node…"`, `"@var"` или литерал) либо `{"#tpl": "…"}`. У условной
	// операции — nil (см. If).
	Append interface{}
	// If — операция условная: Cond — условие (как написано, без обёртки у
	// короткой формы), Value/Else — правые части `#append` веток.
	If      bool
	Cond    interface{}
	Value   interface{}
	Else    interface{}
	HasElse bool
}

// NodeFinalOps — глаголы `#final` одной переменной. Порядок применения
// фиксирован нормой: Unique → Sort → Limit (не порядок ключей JSON).
type NodeFinalOps struct {
	Unique bool
	Sort   bool
	// Limit — первые N элементов; 0 — без ограничения.
	Limit int
}

// UnmarshalJSON читает ключ как есть; ошибка формы запоминается и
// называется валидатором загрузки с id пресета (ValidateNodeLoops), а не
// роняет разбор всего пресета безымянной ошибкой JSON.
func (l *NodeLoop) UnmarshalJSON(data []byte) error {
	raw, invalid := decodeLoopObject(data, NodeLoopKey)
	*l = NodeLoop{raw: raw, invalid: invalid}
	if invalid != "" {
		return nil
	}
	for _, k := range sortedKeys(raw) {
		switch k {
		case nodeFilterKey:
			l.Filter = raw[k]
		case nodeDoKey, nodeFinalKey:
			l.invalid = fmt.Sprintf("%s: %s is only allowed in %s (the body of %s sees @node directly)", NodeLoopKey, k, NodeLoopDoKey, NodeLoopKey)
			return nil
		default:
			l.invalid = unknownLoopKey(NodeLoopKey, k)
			return nil
		}
	}
	return nil
}

// MarshalJSON — ключ в форме шаблона.
func (l NodeLoop) MarshalJSON() ([]byte, error) {
	return marshalLoopRaw(l.raw)
}

// UnmarshalJSON — разбор `#filter`/`#do`/`#final`; ошибка формы
// запоминается для валидатора (см. NodeLoop.UnmarshalJSON).
func (l *NodeLoopDo) UnmarshalJSON(data []byte) error {
	raw, invalid := decodeLoopObject(data, NodeLoopDoKey)
	*l = NodeLoopDo{raw: raw, invalid: invalid}
	if invalid != "" {
		return nil
	}
	hasDo := false
	for _, k := range sortedKeys(raw) {
		switch k {
		case nodeFilterKey:
			l.Filter = raw[k]
		case nodeDoKey:
			hasDo = true
			ops, msg := parseNodeDo(raw[k])
			if msg != "" {
				l.invalid = msg
				return nil
			}
			l.Do = ops
		case nodeFinalKey:
			final, msg := parseNodeFinal(raw[k])
			if msg != "" {
				l.invalid = msg
				return nil
			}
			l.Final = final
		default:
			l.invalid = unknownLoopKey(NodeLoopDoKey, k)
			return nil
		}
	}
	if !hasDo {
		l.invalid = fmt.Sprintf("%s: %s is required", NodeLoopDoKey, nodeDoKey)
		return nil
	}
	for name := range l.Final {
		if !l.hasDo(name) {
			l.invalid = fmt.Sprintf("%s: %s names @%s, which has no operation in %s", NodeLoopDoKey, nodeFinalKey, name, nodeDoKey)
			return nil
		}
	}
	return nil
}

// MarshalJSON — ключ в форме шаблона.
func (l NodeLoopDo) MarshalJSON() ([]byte, error) {
	return marshalLoopRaw(l.raw)
}

func (l *NodeLoopDo) hasDo(name string) bool {
	for _, op := range l.Do {
		if op.Var == name {
			return true
		}
	}
	return false
}

// decodeLoopObject — значение ключа обхода объектом (числа — json.Number).
func decodeLoopObject(data []byte, key string) (map[string]interface{}, string) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Sprintf("%s: %v", key, err)
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Sprintf("%s must be an object", key)
	}
	return m, ""
}

func marshalLoopRaw(raw map[string]interface{}) ([]byte, error) {
	if raw == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(raw)
}

// unknownLoopKey — сообщение о ключе вне §2.2–§2.4; у снятых ключей прежнего
// for_each — с подсказкой замены.
func unknownLoopKey(key, k string) string {
	switch k {
	case "node_type":
		return fmt.Sprintf("%s: key node_type is removed (contract 1.1.119) — select nodes with #filter, e.g. {\"@node.body.type\": \"tailscale\"}", key)
	case "as":
		return fmt.Sprintf("%s: key as is removed (contract 1.1.119) — the node is always @node", key)
	case "filter":
		return fmt.Sprintf("%s: key filter is removed (contract 1.1.119) — use #filter", key)
	}
	return fmt.Sprintf("%s: unknown key %q (allowed: %s)", key, k, allowedLoopKeys(key))
}

func allowedLoopKeys(key string) string {
	if key == NodeLoopKey {
		return nodeFilterKey
	}
	return nodeFilterKey + ", " + nodeDoKey + ", " + nodeFinalKey
}

// parseNodeDo — `#do`: объект `"@имя" → операция`. Операция — объект ровно с
// одним глаголом либо объект с единственным ключом `#if…`, обе ветки
// которого — объекты ровно с одним глаголом (§6.7.1).
func parseNodeDo(v interface{}) ([]NodeDoOp, string) {
	m, ok := v.(map[string]interface{})
	if !ok || len(m) == 0 {
		return nil, fmt.Sprintf("%s: %s must be a non-empty object {\"@var\": {\"%s\": …}}", NodeLoopDoKey, nodeDoKey, nodeDoAppend)
	}
	ops := make([]NodeDoOp, 0, len(m))
	for _, k := range sortedKeys(m) {
		name, msg := loopVarKey(nodeDoKey, k)
		if msg != "" {
			return nil, msg
		}
		op, ok := m[k].(map[string]interface{})
		if !ok {
			return nil, fmt.Sprintf("%s: %s.%s must be an object with exactly one verb", NodeLoopDoKey, nodeDoKey, k)
		}
		ctx := nodeDoKey + "." + k
		ifKeys := ifKeysSorted(op)
		if len(ifKeys) == 0 {
			rhs, msg := parseDoVerb(op, ctx)
			if msg != "" {
				return nil, msg
			}
			ops = append(ops, NodeDoOp{Var: name, Append: rhs})
			continue
		}
		// Условная операция: ровно один ключ #if… и ничего рядом — условие
		// без веток (плоский гейт) и ветки без условия не смешиваются.
		if len(ifKeys) > 1 {
			return nil, fmt.Sprintf("%s: %s: an operation allows exactly one %q key, got %d", NodeLoopDoKey, ctx, "#if…", len(ifKeys))
		}
		if len(op) != 1 {
			return nil, fmt.Sprintf("%s: %s: no other keys are allowed next to %s (put the verb inside its branches)", NodeLoopDoKey, ctx, ifKeys[0])
		}
		f, msg := parseIfForm(op[ifKeys[0]])
		if msg != "" {
			return nil, fmt.Sprintf("%s: %s.%s: %s", NodeLoopDoKey, ctx, ifKeys[0], msg)
		}
		cond := NodeDoOp{Var: name, If: true, Cond: f.Cond, HasElse: f.HasElse}
		if cond.Value, msg = parseDoBranch(f.Value, ctx+"."+ifKeys[0]+" value"); msg != "" {
			return nil, msg
		}
		if f.HasElse {
			if cond.Else, msg = parseDoBranch(f.Else, ctx+"."+ifKeys[0]+" else"); msg != "" {
				return nil, msg
			}
		}
		ops = append(ops, cond)
	}
	return ops, ""
}

// parseDoBranch — ветка условной операции: объект ровно с одним глаголом.
func parseDoBranch(v interface{}, ctx string) (interface{}, string) {
	op, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Sprintf("%s: %s must be an object with exactly one verb", NodeLoopDoKey, ctx)
	}
	return parseDoVerb(op, ctx)
}

// parseDoVerb — объект операции без условия: ровно один глагол из закрытого
// списка (`#append`) с валидной правой частью.
func parseDoVerb(op map[string]interface{}, ctx string) (interface{}, string) {
	if len(op) != 1 {
		return nil, fmt.Sprintf("%s: %s must be an object with exactly one verb", NodeLoopDoKey, ctx)
	}
	for verb, rhs := range op {
		if verb != nodeDoAppend {
			return nil, fmt.Sprintf("%s: %s: unknown verb %q (allowed: %s)", NodeLoopDoKey, ctx, verb, nodeDoAppend)
		}
		if msg := validAppendValue(rhs); msg != "" {
			return nil, fmt.Sprintf("%s: %s.%s: %s", NodeLoopDoKey, ctx, nodeDoAppend, msg)
		}
		return rhs, ""
	}
	return nil, ""
}

// validAppendValue — правая часть `#append`: строка или `{"#tpl": строка}`.
func validAppendValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		if strings.TrimSpace(x) == "" {
			return "empty value"
		}
		return ""
	case map[string]interface{}:
		if s, ok := x[tplKey].(string); ok && len(x) == 1 && s != "" {
			return ""
		}
		return "an object is allowed only as {\"#tpl\": \"…\"}"
	}
	return "must be a reference \"@…\", a string or {\"#tpl\": \"…\"} (objects and arrays are not allowed)"
}

// parseNodeFinal — `#final`: объект `"@имя" → {глаголы}`.
func parseNodeFinal(v interface{}) (map[string]NodeFinalOps, string) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Sprintf("%s: %s must be an object {\"@var\": {…}}", NodeLoopDoKey, nodeFinalKey)
	}
	out := make(map[string]NodeFinalOps, len(m))
	for _, k := range sortedKeys(m) {
		name, msg := loopVarKey(nodeFinalKey, k)
		if msg != "" {
			return nil, msg
		}
		verbs, ok := m[k].(map[string]interface{})
		if !ok || len(verbs) == 0 {
			return nil, fmt.Sprintf("%s: %s.%s must be a non-empty object of verbs", NodeLoopDoKey, nodeFinalKey, k)
		}
		var ops NodeFinalOps
		for _, verb := range sortedKeys(verbs) {
			val := verbs[verb]
			switch verb {
			case nodeFinalUnique, nodeFinalSort:
				b, ok := val.(bool)
				if !ok {
					return nil, fmt.Sprintf("%s: %s.%s.%s must be a bool", NodeLoopDoKey, nodeFinalKey, k, verb)
				}
				if verb == nodeFinalUnique {
					ops.Unique = b
				} else {
					ops.Sort = b
				}
			case nodeFinalLimit:
				n, ok := positiveInt(val)
				if !ok {
					return nil, fmt.Sprintf("%s: %s.%s.%s must be an integer > 0", NodeLoopDoKey, nodeFinalKey, k, verb)
				}
				ops.Limit = n
			default:
				return nil, fmt.Sprintf("%s: %s.%s: unknown verb %q (allowed: %s, %s, %s)", NodeLoopDoKey, nodeFinalKey, k, verb, nodeFinalUnique, nodeFinalSort, nodeFinalLimit)
			}
		}
		out[name] = ops
	}
	return out, ""
}

// loopVarKey — ключ `"@имя"` в `#do`/`#final`; возвращает имя без "@".
func loopVarKey(section, k string) (string, string) {
	name := strings.TrimPrefix(k, "@")
	if !strings.HasPrefix(k, "@") || name == "" || strings.ContainsAny(name, "@.{} ") {
		return "", fmt.Sprintf("%s: %s key %q must be a preset var reference \"@name\"", NodeLoopDoKey, section, k)
	}
	return name, ""
}

func positiveInt(v interface{}) (int, bool) {
	switch x := v.(type) {
	case json.Number:
		n, err := strconv.ParseInt(x.String(), 10, 32)
		if err != nil || n <= 0 {
			return 0, false
		}
		return int(n), true
	case float64:
		if x <= 0 || x != float64(int(x)) {
			return 0, false
		}
		return int(x), true
	}
	return 0, false
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ValidateNodeLoops — отказы загрузки пресета по SPEC 166 §2.1–§2.5:
// прежний `for_each`, оба ключа обхода сразу, форма ключа, переменные `#do`
// (объявлены в этом пресете, тип text_list), резерв имени `node`, `unique`
// не у text_list. nil — пресет годен.
func ValidateNodeLoops(p *Preset) error {
	if p == nil {
		return nil
	}
	if p.legacyForEach {
		return fmt.Errorf("key %s is removed (contract 1.1.119) — use %s or %s", legacyForEachKey, NodeLoopKey, NodeLoopDoKey)
	}
	if p.ForEachNode != nil && p.ForEachNodeDo != nil {
		return fmt.Errorf("%s and %s are mutually exclusive — use one of them", NodeLoopKey, NodeLoopDoKey)
	}
	for _, v := range p.Vars {
		if v.Name == NodeLoopVar || strings.HasPrefix(v.Name, NodeLoopVar+".") {
			return fmt.Errorf("var name %q is reserved for the node of %s/%s", v.Name, NodeLoopKey, NodeLoopDoKey)
		}
		if v.Unique && v.Type != "text_list" {
			return fmt.Errorf("var %q: unique is allowed only for type text_list, got %q", v.Name, v.Type)
		}
	}
	if l := p.ForEachNode; l != nil && l.invalid != "" {
		return fmt.Errorf("%s", l.invalid)
	}
	l := p.ForEachNodeDo
	if l == nil {
		return nil
	}
	if l.invalid != "" {
		return fmt.Errorf("%s", l.invalid)
	}
	for _, op := range l.Do {
		v := p.presetVar(op.Var)
		if v == nil || v.Ref != "" {
			return fmt.Errorf("%s: %s writes @%s, which is not declared in this preset's vars", NodeLoopDoKey, nodeDoKey, op.Var)
		}
		if v.Type != "text_list" {
			return fmt.Errorf("%s: %s on @%s needs type text_list, got %q", NodeLoopDoKey, nodeDoAppend, op.Var, v.Type)
		}
	}
	return nil
}

func (p *Preset) presetVar(name string) *PresetVar {
	for i := range p.Vars {
		if p.Vars[i].Name == name {
			return &p.Vars[i]
		}
	}
	return nil
}

// IsNodeLoop — у пресета есть обход узлов (`for_each_node` или
// `for_each_node_do`): ему нужны узлы конфига.
func (p *Preset) IsNodeLoop() bool {
	return p != nil && (p.ForEachNode != nil || p.ForEachNodeDo != nil)
}

// NodeFilter — `#filter` обхода пресета; ok=false — у пресета обхода нет.
func (p *Preset) NodeFilter() (filter interface{}, ok bool) {
	switch {
	case p == nil:
		return nil, false
	case p.ForEachNode != nil:
		return p.ForEachNode.Filter, true
	case p.ForEachNodeDo != nil:
		return p.ForEachNodeDo.Filter, true
	}
	return nil, false
}

// PresetNode — узел конфига для обхода: финальный тег, тело в конфиге и
// поле записи `skip_presets` (у узла подписки записи нет — false).
type PresetNode struct {
	Tag         string
	Body        map[string]interface{}
	SkipPresets bool
}

// PresetNodeRecordFields — поля записи узла, видимые пресету. Перечень
// закрытый, расширяется контрактом.
var PresetNodeRecordFields = map[string]bool{"skip_presets": true}

// NodeScope — имена одного узла под именем `node` для обходчика.
type NodeScope struct {
	As       string
	Resolved map[string]ResolvedVar
	Types    map[string]string
}

// NewNodeScope раскладывает узел в плоскую карту имён под `node`.
//
// Пустая строка и null в теле значения не дают: имя остаётся объявленным без
// значения, ключ снимается Dropped-каскадом (как у необязательной переменной).
func NewNodeScope(n PresetNode) *NodeScope {
	as := NodeLoopVar
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

// EvalNodeFilter — значение `#filter` для узла; nil-фильтр истинен. Видны
// имена узла, переменные пресета, глобали (varsMap) и `@runtime.*`;
// отсутствующее значение — ложь без предупреждения.
func EvalNodeFilter(filter interface{}, scope *NodeScope, varsMap map[string]string, target TargetSpec) bool {
	if filter == nil {
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
	return evaluateCond(filter, types, resolved, target.Normalized())
}

// NodeDoValue — значение операции `#do` на одном узле (TEMPLATE_LANG §6.7.1):
// "" — узел пропускается молча. Безусловная операция — NodeAppendValue;
// условная прогоняется тем же обходчиком как map-spread `#if` объекта
// операции с пространством узла: `{"#if": [cond, {"#append": V}, {"#append":
// E}]}` даёт `{"#append": …}` (операция выполняется) либо `{}` (условие
// ложно без else — узел пропущен без предупреждения). Правые части веток
// читаются как у безусловной операции (ссылка — вставкой `#tpl`).
func NodeDoValue(op NodeDoOp, vars []TemplateVar, resolved map[string]ResolvedVar, target TargetSpec, node *NodeScope) (string, []TemplateWarning) {
	if !op.If {
		return NodeAppendValue(op.Append, vars, resolved, target, node)
	}
	// Глагол ветки в обёртке — под нейтральным ключом: ключ на "#" обходчик
	// выбросил бы как неизвестную директиву, а глагол уже разобран на загрузке.
	const verb = "append"
	body := []interface{}{op.Cond, map[string]interface{}{verb: appendAsTpl(op.Value)}}
	if op.HasElse {
		body = append(body, map[string]interface{}{verb: appendAsTpl(op.Else)})
	}
	wrapped, warns := substituteWrapped(map[string]interface{}{"#if": body}, vars, resolved, target, node)
	result, _ := wrapped.(map[string]interface{})
	rhs, present := result[verb]
	if !present {
		return "", warns
	}
	return appendScalarText(rhs), warns
}

// NodeAppendValue — значение правой части `#append` на одном узле (TEMPLATE_LANG
// §6.7.1): подстановка тем же обходчиком, что у тела, по правилу вставки
// `#tpl` (§4.8) — скаляр (строка, число, bool) идёт текстом. Пустое значение
// (нет пути, null, "", Dropped), объект и список — "": узел пропускается
// молча.
func NodeAppendValue(rhs interface{}, vars []TemplateVar, resolved map[string]ResolvedVar, target TargetSpec, node *NodeScope) (string, []TemplateWarning) {
	out, warns := substituteWrapped(appendAsTpl(rhs), vars, resolved, target, node)
	return appendScalarText(out), warns
}

// appendAsTpl — ссылка "@имя" в правой части `#append` вычисляется как
// вставка `@{имя}`: необъявленное имя снимает значение (с
// template_var_undeclared), а не оставляет плейсхолдер элементом списка.
func appendAsTpl(rhs interface{}) interface{} {
	if ref, ok := rhs.(string); ok && strings.HasPrefix(ref, "@") && len(ref) > 1 {
		return map[string]interface{}{tplKey: "@{" + ref[1:] + "}"}
	}
	return rhs
}

// substituteWrapped прогоняет одно значение через канонический обход с
// пространством узла и возвращает результат (nil — значение снято или
// подстановка не удалась).
func substituteWrapped(v interface{}, vars []TemplateVar, resolved map[string]ResolvedVar, target TargetSpec, node *NodeScope) (interface{}, []TemplateWarning) {
	data, err := json.Marshal(map[string]interface{}{"v": v})
	if err != nil {
		return nil, nil
	}
	out, warns, _, err := SubstituteVarsInJSONCanonScoped(data, vars, resolved, target, node)
	if err != nil {
		return nil, warns
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	dec.UseNumber()
	var wrapped map[string]interface{}
	if err := dec.Decode(&wrapped); err != nil {
		return nil, warns
	}
	return wrapped["v"], warns
}

// appendScalarText — текст скаляра для text_list; "" — не скаляр или пусто.
// Перевод строки внутри значения разрезал бы элемент при хранении строкой —
// он сводится к пробелу.
func appendScalarText(v interface{}) string {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case json.Number:
		s = x.String()
	case bool:
		s = strconv.FormatBool(x)
	case float64:
		s = strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return ""
	}
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")
	return strings.TrimSpace(s)
}

// Apply — `#final` над списком: Unique → Sort → Limit (порядок нормы).
func (f NodeFinalOps) Apply(list []string) []string {
	if f.Unique {
		list = UniqueStrings(list)
	}
	if f.Sort {
		sorted := append([]string(nil), list...)
		sort.Strings(sorted)
		list = sorted
	}
	if f.Limit > 0 && len(list) > f.Limit {
		list = list[:f.Limit]
	}
	return list
}

// UniqueStrings — список без дубликатов, порядок первого появления.
func UniqueStrings(list []string) []string {
	seen := make(map[string]bool, len(list))
	out := make([]string, 0, len(list))
	for _, s := range list {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
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
// обхода `for_each_node` / `#do` (nil — без пространства узла). emptyRef — хоть одна ссылка на
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
