package template

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// SecretReader — источник энтропии для GenerateSecret / MaybeGenerateSecrets.
// В тестах можно временно подменить на детерминированный io.Reader.
var SecretReader io.Reader = rand.Reader

const secretPlaceholderPrefix = "CHANGE_THIS_"

// SecretUnresolved true, если значение секрета ещё нужно автогенерировать
// (пусто/пробелы или префикс плейсхолдера шаблона CHANGE_THIS_*). Критерий
// общий для всех type:"secret" var (см. MaybeGenerateSecrets).
func SecretUnresolved(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || strings.HasPrefix(s, secretPlaceholderPrefix)
}

// TemplateVar описывает элемент секции vars шаблона.
type TemplateVar struct {
	// Separator: декоративная горизонтальная линия на вкладке Settings (без name/type/плейсхолдеров).
	Separator    bool            `json:"separator,omitempty"`
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	DefaultValue VarDefaultValue `json:"default_value,omitempty"`
	DefaultNode  string          `json:"default_node,omitempty"`
	// Options — список допустимых значений (substitution uses these).
	// JSON form: ["5m", "30s"]  OR  [{"title":"5m (default)", "value":"5m"}].
	// Raw strings are read into Options as-is; object form populates Options
	// with value and OptionTitles in parallel with title.
	Options []string `json:"-"`
	// OptionTitles — human-readable labels parallel to Options; nil (or
	// shorter-than-Options) means "use value as title". Populated from the
	// `{title,value}` object form. Not serialized back out.
	OptionTitles []string `json:"-"`
	WizardUI     string   `json:"wizard_ui,omitempty"`
	Platforms    []string `json:"platforms,omitempty"`
	// Title подпись строки на вкладке Settings; при пустом используется name.
	Title string `json:"title,omitempty"`
	// Tooltip всплывающая подсказка для строки (виджеты с поддержкой SetToolTip).
	Tooltip string `json:"tooltip,omitempty"`
	// If: строка Settings активна только если все перечисленные bool vars истинны (как params.if).
	If []string `json:"if,omitempty"`
	// IfOr: активна если хотя бы одна bool var истинна (как params.if_or).
	IfOr []string `json:"if_or,omitempty"`
	// Enable — гейт #enable (SPEC 107): условие языка §5.1 любой формы.
	// Для переменной шаблона false означает «строка Settings неактивна»;
	// значение при этом СОХРАНЯЕТСЯ и по-прежнему подставляется в конфиг —
	// UI-гейт, а не emit-гейт (§11.7). У переменной ПРЕСЕТА иначе: имя
	// удаляется из varsMap, и фрагменты с ним выпадают.
	Enable json.RawMessage `json:"#enable,omitempty"`
	// OnChange — side-effect при изменении этой var (паритет с mobile,
	// SPEC 103): `{"set": {"@target": <#if-дерево>, ...}}`. Ключ "set" —
	// единственная форма сегодня (как в эталоне LxBox parser_config.dart).
	// Каждый target пере-вычисляется через EvalIfScalar В КОНТЕКСТЕ уже
	// НОВОГО значения этой var и пишется в целевую переменную; см.
	// ApplyOnChange (on_change.go).
	OnChange map[string]json.RawMessage `json:"on_change,omitempty"`

	// OnChangeHashed — канонический помеченный вариант `#on_change`
	// (SPEC 107). Правило языка: `#` — ключевое слово движка. Легаси
	// `on_change` читается бессрочно; слияние — в UnmarshalJSON.
	OnChangeHashed map[string]json.RawMessage `json:"#on_change,omitempty"`
}

// templateVarAlias avoids infinite recursion in UnmarshalJSON and carries the
// raw options payload so it can be decoded into either string or object form.
type templateVarAlias struct {
	Separator      bool                       `json:"separator,omitempty"`
	Name           string                     `json:"name"`
	Type           string                     `json:"type"`
	DefaultValue   VarDefaultValue            `json:"default_value,omitempty"`
	DefaultNode    string                     `json:"default_node,omitempty"`
	Options        json.RawMessage            `json:"options,omitempty"`
	WizardUI       string                     `json:"wizard_ui,omitempty"`
	Platforms      []string                   `json:"platforms,omitempty"`
	Title          string                     `json:"title,omitempty"`
	Tooltip        string                     `json:"tooltip,omitempty"`
	If             []string                   `json:"if,omitempty"`
	IfOr           []string                   `json:"if_or,omitempty"`
	Enable         json.RawMessage            `json:"#enable,omitempty"`
	OnChange       map[string]json.RawMessage `json:"on_change,omitempty"`
	OnChangeHashed map[string]json.RawMessage `json:"#on_change,omitempty"`
}

// UnmarshalJSON decodes a TemplateVar, accepting `options` as either a list of
// raw strings (legacy) or a list of `{title, value}` objects (mobile parity,
// 2026-04-22). A mixed list is also supported — per-element fallback.
func (v *TemplateVar) UnmarshalJSON(data []byte) error {
	var a templateVarAlias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	v.Separator = a.Separator
	v.Name = a.Name
	v.Type = a.Type
	v.DefaultValue = a.DefaultValue
	v.DefaultNode = a.DefaultNode
	v.WizardUI = a.WizardUI
	v.Platforms = a.Platforms
	v.Title = a.Title
	v.Tooltip = a.Tooltip
	v.If = a.If
	v.IfOr = a.IfOr
	v.Enable = a.Enable
	// Канон `#on_change` побеждает легаси `on_change` (SPEC 107).
	v.OnChange = a.OnChange
	if len(a.OnChangeHashed) > 0 {
		v.OnChange = a.OnChangeHashed
	}

	if len(a.Options) == 0 || string(a.Options) == "null" {
		return nil
	}
	// First try the simple `[]string` form — most templates use this.
	var strs []string
	if err := json.Unmarshal(a.Options, &strs); err == nil {
		v.Options = strs
		return nil
	}
	// Then the object / mixed form. Parse each element individually so a
	// string among objects still works.
	var raws []json.RawMessage
	if err := json.Unmarshal(a.Options, &raws); err != nil {
		return err
	}
	values := make([]string, 0, len(raws))
	titles := make([]string, 0, len(raws))
	var anyTitle, anyObjectForm bool
	for _, r := range raws {
		var s string
		if err := json.Unmarshal(r, &s); err == nil {
			values = append(values, s)
			titles = append(titles, s)
			continue
		}
		var obj struct {
			Title string `json:"title"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(r, &obj); err != nil {
			return err
		}
		anyObjectForm = true
		values = append(values, obj.Value)
		if strings.TrimSpace(obj.Title) == "" {
			titles = append(titles, obj.Value)
		} else {
			titles = append(titles, obj.Title)
			anyTitle = true
		}
	}
	v.Options = values
	if anyTitle {
		v.OptionTitles = titles
	}
	// Object-form options (`[{title, value}]`) imply a closed-set semantic by
	// definition — titles are display-only labels, the substituted value is
	// the `value` field. Combining this with `type:"text"` (free-text combo)
	// is unsafe: free-typed text bypasses the title→value mapping and lands
	// in the config as the literal display string. Same risk for any other
	// type. Normalize to `enum` regardless of the declared type so all code
	// paths (renderer, validator, preview, substitute) see one consistent
	// invariant.
	if anyObjectForm {
		v.Type = "enum"
	}
	return nil
}

// OptionTitle returns the user-visible label for the i-th option, falling
// back to the raw value when no explicit title was supplied.
func (v TemplateVar) OptionTitle(i int) string {
	if i < 0 || i >= len(v.Options) {
		return ""
	}
	if i < len(v.OptionTitles) && strings.TrimSpace(v.OptionTitles[i]) != "" {
		return v.OptionTitles[i]
	}
	return v.Options[i]
}

// VarDisplayTitle подпись строки Settings: title (если не пуст после TrimSpace), иначе name.
func VarDisplayTitle(v TemplateVar) string {
	s := strings.TrimSpace(v.Title)
	if s != "" {
		return s
	}
	return strings.TrimSpace(v.Name)
}

// VarDisplayTooltip текст подсказки; пустой — не показывать.
func VarDisplayTooltip(v TemplateVar) string {
	return strings.TrimSpace(v.Tooltip)
}

// VarByName finds a non-separator var by name.
func VarByName(vars []TemplateVar, name string) (TemplateVar, bool) {
	n := strings.TrimSpace(name)
	for _, v := range vars {
		if v.Separator {
			continue
		}
		if strings.TrimSpace(v.Name) == n {
			return v, true
		}
	}
	return TemplateVar{}, false
}

// VarUISatisfied: условие показа/включения строки Settings для этой var (пустые If/IfOr → всегда true).
// Семантика совпадает с params.if / if_or (ParamBoolVarTrue, VarAppliesOnGOOS).
//
// Обёртка над VarUISatisfiedFor для вызывающих без таргета (local).
func VarUISatisfied(v TemplateVar, varByName map[string]TemplateVar, resolved map[string]ResolvedVar, goos string) bool {
	return VarUISatisfiedFor(v, varByName, resolved, TargetSpec{GOOS: goos}.Normalized())
}

// VarUISatisfiedFor — то же для конкретного таргета (SPEC 097).
//
// Записи if/if_or — предикаты того же языка, что и внутри #if: голое имя
// bool-var ("@tun") ИЛИ объектная форма ({"@runtime.target": "local"},
// {"#not": "@gateway_mode"}, {"@x": {"#in": [...]}}). Одна грамматика на
// весь шаблон: не нужно ни отдельного поля targets[], ни специальных
// правил для UI — «clash_api только на local» пишется тем же выражением,
// что и ветка в config-секции.
func VarUISatisfiedFor(v TemplateVar, varByName map[string]TemplateVar, resolved map[string]ResolvedVar, target TargetSpec) bool {
	if !VarAppliesOnGOOS(v.Platforms, target.Normalized().GOOS) {
		return false
	}
	// SPEC 107: #enable + легаси if/if_or сведены в одно условие. Прежнее
	// «if и if_or одновременно → false» больше не нужно: обе части
	// объединяются через and, как и задумано автором такого шаблона.
	gate := v.Gate()
	if gate == nil {
		return true
	}
	varTypes, scoped := scopeToPlatform(varByName, resolved, target)
	return gate.Satisfied(varTypes, scoped, target)
}

// scopeToPlatform готовит контекст вычисления гейта для целевой ОС.
//
// Переменная, объявленная для другой платформы, обязана считаться ЛОЖНОЙ, а не
// браться из состояния: `if_or: [tun_builtin, tun]`, где tun_builtin —
// windows/linux, а tun — darwin, на macOS должен смотреть только на tun.
// Иначе строка останется активной из-за значения, которого на этой ОС не
// существует (поведение старого ParamBoolVarTrue, vars_resolve.go:480).
//
// Реализуется вырезанием таких имён из resolved: предикат не найдёт значение и
// даст false — тот же результат, но одной семантикой движка.
func scopeToPlatform(varByName map[string]TemplateVar, resolved map[string]ResolvedVar, target TargetSpec) (map[string]string, map[string]ResolvedVar) {
	goos := target.Normalized().GOOS
	varTypes := make(map[string]string, len(varByName))
	scoped := make(map[string]ResolvedVar, len(resolved))
	for name, r := range resolved {
		scoped[name] = r
	}
	for name, decl := range varByName {
		varTypes[name] = decl.Type
		if !VarAppliesOnGOOS(decl.Platforms, goos) {
			delete(scoped, name)
		}
	}
	return varTypes, scoped
}

// Gate возвращает нормализованный гейт переменной (#enable + легаси if/if_or)
// или nil, если гейта нет.
func (v TemplateVar) Gate() *GateCond {
	var enableRaw interface{}
	if len(v.Enable) > 0 {
		if err := json.Unmarshal(v.Enable, &enableRaw); err != nil {
			return &GateCond{Invalid: true} // fail-closed
		}
	}
	return NormalizeGate(enableRaw, v.If, v.IfOr)
}

// GateDeps — имена переменных, от которых зависит активность строки
// (SPEC 107 §8.1): вход в реактивный индекс UI.
func (v TemplateVar) GateDeps() []string {
	return v.Gate().Deps()
}

// ResolvedVar — значение переменной после разрешения (state → default).
type ResolvedVar struct {
	Scalar string
	List   []string
}

// GenerateSecret возвращает случайную строку из 16 символов [A-Za-z0-9].
func GenerateSecret() (string, error) {
	return generateSecret(SecretReader)
}

func generateSecret(r io.Reader) (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	const n = 16
	var b strings.Builder
	b.Grow(n)
	for i := 0; i < n; i++ {
		idx, err := rand.Int(r, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(alphabet[idx.Int64()])
	}
	return b.String(), nil
}

// ResolveTemplateVars разрешает все переменные шаблона для локального таргета
// (эта машина). Тонкая обёртка над ResolveTemplateVarsFor — сохранена для
// вызывающих, которым таргет не важен.
func ResolveTemplateVars(vars []TemplateVar, state map[string]string, rawTemplate json.RawMessage) map[string]ResolvedVar {
	return ResolveTemplateVarsFor(vars, state, rawTemplate, LocalTarget())
}

// ResolveTemplateVarsFor разрешает переменные шаблона для указанного таргета
// (SPEC 097). Таргет влияет на per-platform дефолты (default_value) и на
// #if-деревья внутри них через @runtime.* globals.
func ResolveTemplateVarsFor(vars []TemplateVar, state map[string]string, rawTemplate json.RawMessage, target TargetSpec) map[string]ResolvedVar {
	target = target.Normalized()
	out := make(map[string]ResolvedVar, len(vars))
	var root map[string]json.RawMessage
	if len(rawTemplate) > 0 {
		_ = json.Unmarshal(rawTemplate, &root)
	}
	// varTypes нужен предикатам #if внутри default_value (bare bool-форма
	// смотрит на тип). Собираем один раз по ВСЕМ vars; а вот resolved (`out`)
	// заполняется по ходу цикла — поэтому default_value видит только vars,
	// объявленные выше себя (контракт ForTargetIn).
	varTypes := make(map[string]string, len(vars))
	for _, v := range vars {
		if !v.Separator {
			varTypes[v.Name] = v.Type
		}
	}
	for _, v := range vars {
		if v.Separator {
			continue
		}
		out[v.Name] = resolveOneVar(v, state[v.Name], root, target, varTypes, out)
	}
	return out
}

// MaybeGenerateSecrets автогенерирует значение для КАЖДОЙ объявленной
// type:"secret" переменной, если в resolved она пустая/плейсхолдер CHANGE_THIS_*.
// Обобщает прежнее clash_secret-специфичное поведение: секрет всегда
// материализуется (например proxy_in_password). Если шаблон не вставит его —
// это решает #if в шаблоне (proxy_in_password не попадёт в config без
// непустого proxy_in_username), а не отсутствие значения.
func MaybeGenerateSecrets(vars []TemplateVar, resolved map[string]ResolvedVar) {
	for _, v := range vars {
		if v.Separator || !strings.EqualFold(strings.TrimSpace(v.Type), "secret") {
			continue
		}
		if !SecretUnresolved(resolved[v.Name].Scalar) {
			continue
		}
		gen, err := GenerateSecret()
		if err != nil {
			debuglog.WarnLog("MaybeGenerateSecrets %q: %v", v.Name, err)
			continue
		}
		resolved[v.Name] = ResolvedVar{Scalar: gen}
	}
}

func resolveOneVar(v TemplateVar, stateVal string, root map[string]json.RawMessage, target TargetSpec, varTypes map[string]string, resolved map[string]ResolvedVar) ResolvedVar {
	switch v.Type {
	case "text_list":
		if strings.TrimSpace(stateVal) != "" {
			lines := strings.Split(strings.ReplaceAll(stateVal, "\r\n", "\n"), "\n")
			var nonEmpty []string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" {
					nonEmpty = append(nonEmpty, line)
				}
			}
			return ResolvedVar{List: nonEmpty}
		}
		if !v.DefaultValue.IsEmpty() {
			def := v.DefaultValue.ForTargetIn(target, varTypes, resolved)
			if def != "" {
				return resolveOneVar(TemplateVar{Name: v.Name, Type: "text_list"}, def, root, target, varTypes, resolved)
			}
		}
		if v.DefaultNode != "" && root != nil {
			raw := getRawAtPath(root, v.DefaultNode)
			if len(raw) > 0 {
				var arr []string
				if err := json.Unmarshal(raw, &arr); err == nil {
					return ResolvedVar{List: arr}
				}
			}
		}
		return ResolvedVar{List: []string{}}
	default:
		s := strings.TrimSpace(stateVal)
		if s != "" {
			return ResolvedVar{Scalar: s}
		}
		if !v.DefaultValue.IsEmpty() {
			dv := v.DefaultValue.ForTargetIn(target, varTypes, resolved)
			if dv != "" {
				return ResolvedVar{Scalar: dv}
			}
		}
		if v.DefaultNode != "" && root != nil {
			if lit := readJSONLiteralAsString(getRawAtPath(root, v.DefaultNode)); lit != "" {
				return ResolvedVar{Scalar: lit}
			}
		}
		return ResolvedVar{Scalar: ""}
	}
}

func getRawAtPath(root map[string]json.RawMessage, path string) json.RawMessage {
	parts := strings.Split(path, ".")
	cur := root
	for i, p := range parts {
		raw, ok := cur[p]
		if !ok {
			return nil
		}
		if i == len(parts)-1 {
			return raw
		}
		var next map[string]json.RawMessage
		if err := json.Unmarshal(raw, &next); err != nil {
			return nil
		}
		cur = next
	}
	return nil
}

// readJSONLiteralAsString decodes RAW json.RawMessage bytes (string / number
// / bool / null) to a string, returning "" for anything that is not a
// recognized JSON literal — a STRICT reader.
//
// Intentionally distinct from defaultValueStringify (vars_default.go), which
// takes an already-decoded interface{} and has a fmt.Sprint catch-all
// fallback. The input type (raw bytes vs decoded value) and the fallback
// semantics (strict-reject vs best-effort) both differ on purpose. Do NOT
// merge the two (SPEC 069 §5.4 — verified intentional divergence).
func readJSONLiteralAsString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		if b {
			return "true"
		}
		return "false"
	}
	return ""
}

// VarAppliesOnGOOS: пустой platforms — на всех ОС; иначе только совпадение с goos (Win7-сборка — windows/386).
// Если текущая ОС не входит в список, переменная для params.if / if_or считается ложной (см. ParamBoolVarTrue),
// даже если в resolved осталось значение из state с другой платформы.
func VarAppliesOnGOOS(platforms []string, goos string) bool {
	if len(platforms) == 0 {
		return true
	}
	for _, p := range platforms {
		if p == goos {
			return true
		}
	}
	return false
}

// ParamBoolVarTrue: для if / if_or — bool var объявлена в шаблоне, подходит под текущую ОС (VarAppliesOnGOOS)
// и в resolved равна "true". Не подходит под goos → false (как «нет переменной» для условия), без учёта resolved.
//
// SPEC 067 Phase 3: канонический формат имени — "@var". Префикс strip'ается в начале;
// без префикса (legacy) запись считается допустимой только если validator уже её
// пропустил (валидатор ловит missing-`@` на load — см. validateOuterIfList).
func ParamBoolVarTrue(name string, varByName map[string]TemplateVar, resolved map[string]ResolvedVar, goos string) bool {
	name = strings.TrimPrefix(name, "@")
	vd, ok := varByName[name]
	if !ok || vd.Type != "bool" {
		return false
	}
	if !VarAppliesOnGOOS(vd.Platforms, goos) {
		return false
	}
	r, ok := resolved[name]
	if !ok || strings.TrimSpace(r.Scalar) != "true" {
		return false
	}
	return true
}

// ParamIfSatisfied: все предикаты в if истинны на текущей ОС.
func ParamIfSatisfied(ifNames []string, varByName map[string]TemplateVar, resolved map[string]ResolvedVar, goos string) bool {
	return ParamIfSatisfiedFor(ifNames, varByName, resolved, TargetSpec{GOOS: goos}.Normalized())
}

// ParamIfSatisfiedFor — все предикаты истинны для указанного таргета.
func ParamIfSatisfiedFor(ifNames []string, varByName map[string]TemplateVar, resolved map[string]ResolvedVar, target TargetSpec) bool {
	for _, expr := range ifNames {
		if !condEntryTrue(expr, varByName, resolved, target) {
			return false
		}
	}
	return true
}

// ParamIfOrSatisfied: хотя бы один предикат из списка истинен на текущей ОС.
func ParamIfOrSatisfied(ifOrNames []string, varByName map[string]TemplateVar, resolved map[string]ResolvedVar, goos string) bool {
	return ParamIfOrSatisfiedFor(ifOrNames, varByName, resolved, TargetSpec{GOOS: goos}.Normalized())
}

// ParamIfOrSatisfiedFor — хотя бы один предикат истинен для таргета.
func ParamIfOrSatisfiedFor(ifOrNames []string, varByName map[string]TemplateVar, resolved map[string]ResolvedVar, target TargetSpec) bool {
	for _, expr := range ifOrNames {
		if condEntryTrue(expr, varByName, resolved, target) {
			return true
		}
	}
	return false
}

// VarConditionIsTargetOnly — условие var'а зависит ИСКЛЮЧИТЕЛЬНО от
// @runtime.* globals (таргет, платформа, архитектура), без ссылок на другие
// vars.
//
// Такое условие статично для данной сборки: пользователь не может его
// удовлетворить, что-то переключив, — значит поле надо СКРЫТЬ, а не
// показывать выключенным. Условия со ссылками на vars (@tun) наоборот
// гасят строку: их пользователь может выполнить.
func VarConditionIsTargetOnly(v TemplateVar) bool {
	// SPEC 107: вопрос «от чего зависит условие» отвечает CondDeps — одна
	// семантика на любую форму записи (#enable, легаси if/if_or, JSON-предикат
	// строкой). Раньше функция читала только v.If и требовала, чтобы каждый
	// элемент был JSON-объектом; после миграции на #enable она переставала
	// видеть условие вовсе, и строка гасла вместо того, чтобы скрыться.
	deps := v.GateDeps()
	if len(deps) == 0 {
		return false
	}
	for _, name := range deps {
		if !isRuntimeGlobalRef(name) {
			return false // ссылка на обычную переменную — условие не статично
		}
	}
	return true
}

// onlyRuntimeGlobalRefs true, если в JSON-предикате все "@..."-ссылки —
// runtime-globals.
func onlyRuntimeGlobalRefs(jsonExpr string) bool {
	for _, ref := range atRefPattern.FindAllStringSubmatch(jsonExpr, -1) {
		if !isRuntimeGlobalRef(ref[1]) {
			return false
		}
	}
	return true
}

// atRefPattern выбирает "@name" внутри JSON-строк предиката.
var atRefPattern = regexp.MustCompile(`"@([A-Za-z0-9_.]+)"`)

// condEntryTrue вычисляет ОДНУ запись if/if_or.
//
// Голое "@name" сохраняет исторический смысл (bool-var истинна на этой ОС),
// чтобы существующие шаблоны не поехали. Всё, что начинается с "{", —
// JSON-предикат языка #if, вычисляемый ТЕМ ЖЕ evaluatePredicate, что и
// внутри config-секций: одна грамматика, одно место отказа.
func condEntryTrue(expr string, varByName map[string]TemplateVar, resolved map[string]ResolvedVar, target TargetSpec) bool {
	trimmed := strings.TrimSpace(expr)
	if !strings.HasPrefix(trimmed, "{") {
		return ParamBoolVarTrue(trimmed, varByName, resolved, target.Normalized().GOOS)
	}
	var node interface{}
	if err := json.Unmarshal([]byte(trimmed), &node); err != nil {
		debuglog.WarnLog("template: if-предикат %q — невалидный JSON: %v; считаем false", trimmed, err)
		return false
	}
	varTypes := make(map[string]string, len(varByName))
	for name, vd := range varByName {
		varTypes[name] = vd.Type
	}
	return evaluatePredicate(node, varTypes, resolved, target.Normalized())
}

// VarIndex строит map name -> TemplateVar.
func VarIndex(vars []TemplateVar) map[string]TemplateVar {
	m := make(map[string]TemplateVar, len(vars))
	for _, v := range vars {
		if v.Separator {
			continue
		}
		m[v.Name] = v
	}
	return m
}

// DisplaySettingValue строка для UI Settings без генерации clash_secret (плейсхолдер из шаблона).
func DisplaySettingValue(vars []TemplateVar, state map[string]string, rawFull json.RawMessage, name string) string {
	return DisplaySettingValueFor(vars, state, rawFull, name, LocalTarget())
}

// DisplaySettingValueFor — то же для конкретного таргета (SPEC 097).
//
// Таргет обязателен: state даёт override, но при его отсутствии значение
// приходит из default_value, а тот ветвится по @runtime.target
// (tun_interface_name: lxd-tun0 на remote против singbox-tun0 на local).
// Резолв по local-таргету показывал бы в UI не то значение, которое реально
// уедет в конфиг.
func DisplaySettingValueFor(vars []TemplateVar, state map[string]string, rawFull json.RawMessage, name string, target TargetSpec) string {
	r := ResolveTemplateVarsFor(vars, state, rawFull, target)
	rv, ok := r[name]
	if !ok {
		return ""
	}
	if len(rv.List) > 0 {
		return strings.Join(rv.List, "\n")
	}
	return rv.Scalar
}
