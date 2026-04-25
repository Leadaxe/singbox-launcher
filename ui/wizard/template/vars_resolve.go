package template

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"math/big"
	"runtime"
	"strconv"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// ClashSecretReader — источник энтропии для GenerateClashSecret / MaybeGenerateClashSecret.
// В тестах можно временно подменить на детерминированный io.Reader.
var ClashSecretReader io.Reader = rand.Reader

const clashSecretPrefix = "CHANGE_THIS_"

// ClashSecretUnresolved true, если значение из state ещё нужно заменить автогенерацией
// (пусто/пробелы или префикс плейсхолдера шаблона). Совпадает с критерием MaybeGenerateClashSecret.
func ClashSecretUnresolved(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || strings.HasPrefix(s, clashSecretPrefix)
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
}

// templateVarAlias avoids infinite recursion in UnmarshalJSON and carries the
// raw options payload so it can be decoded into either string or object form.
type templateVarAlias struct {
	Separator    bool            `json:"separator,omitempty"`
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	DefaultValue VarDefaultValue `json:"default_value,omitempty"`
	DefaultNode  string          `json:"default_node,omitempty"`
	Options      json.RawMessage `json:"options,omitempty"`
	WizardUI     string          `json:"wizard_ui,omitempty"`
	Platforms    []string        `json:"platforms,omitempty"`
	Title        string          `json:"title,omitempty"`
	Tooltip      string          `json:"tooltip,omitempty"`
	If           []string        `json:"if,omitempty"`
	IfOr         []string        `json:"if_or,omitempty"`
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
func VarUISatisfied(v TemplateVar, varByName map[string]TemplateVar, resolved map[string]ResolvedVar, goos string) bool {
	if len(v.If) > 0 && len(v.IfOr) > 0 {
		return false
	}
	if len(v.If) > 0 {
		return ParamIfSatisfied(v.If, varByName, resolved, goos)
	}
	if len(v.IfOr) > 0 {
		return ParamIfOrSatisfied(v.IfOr, varByName, resolved, goos)
	}
	return true
}

// ResolvedVar — значение переменной после разрешения (state → default).
type ResolvedVar struct {
	Scalar string
	List   []string
}

// IsList true для text_list с данными.
func (v ResolvedVar) IsList() bool {
	return v.List != nil
}

// GenerateClashSecret возвращает случайную строку из 16 символов [A-Za-z0-9].
func GenerateClashSecret() (string, error) {
	return generateClashSecret(ClashSecretReader)
}

func generateClashSecret(r io.Reader) (string, error) {
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

// ResolveTemplateVars разрешает все переменные шаблона.
func ResolveTemplateVars(vars []TemplateVar, state map[string]string, rawTemplate json.RawMessage) map[string]ResolvedVar {
	out := make(map[string]ResolvedVar, len(vars))
	var root map[string]json.RawMessage
	if len(rawTemplate) > 0 {
		_ = json.Unmarshal(rawTemplate, &root)
	}
	for _, v := range vars {
		if v.Separator {
			continue
		}
		out[v.Name] = resolveOneVar(v, state[v.Name], root)
	}
	return out
}

// MaybeGenerateClashSecret подставляет случайный секрет, если значение пустое или плейсхолдер CHANGE_THIS_*.
func MaybeGenerateClashSecret(resolved map[string]ResolvedVar) {
	rv, ok := resolved["clash_secret"]
	if !ok {
		return
	}
	s := strings.TrimSpace(rv.Scalar)
	if s != "" && !strings.HasPrefix(s, clashSecretPrefix) {
		return
	}
	gen, err := GenerateClashSecret()
	if err != nil {
		debuglog.WarnLog("MaybeGenerateClashSecret: %v", err)
		return
	}
	resolved["clash_secret"] = ResolvedVar{Scalar: gen}
}

func resolveOneVar(v TemplateVar, stateVal string, root map[string]json.RawMessage) ResolvedVar {
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
			def := v.DefaultValue.ForPlatform(runtime.GOOS, runtime.GOARCH)
			if def != "" {
				return resolveOneVar(TemplateVar{Name: v.Name, Type: "text_list"}, def, root)
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
			dv := v.DefaultValue.ForPlatform(runtime.GOOS, runtime.GOARCH)
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
func ParamBoolVarTrue(name string, varByName map[string]TemplateVar, resolved map[string]ResolvedVar, goos string) bool {
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

// ParamIfSatisfied: все имена в if — bool vars истинны на текущей ОС.
func ParamIfSatisfied(ifNames []string, varByName map[string]TemplateVar, resolved map[string]ResolvedVar, goos string) bool {
	for _, name := range ifNames {
		if !ParamBoolVarTrue(name, varByName, resolved, goos) {
			return false
		}
	}
	return true
}

// ParamIfOrSatisfied: хотя бы одна bool var из списка истинна на текущей ОС.
func ParamIfOrSatisfied(ifOrNames []string, varByName map[string]TemplateVar, resolved map[string]ResolvedVar, goos string) bool {
	for _, name := range ifOrNames {
		if ParamBoolVarTrue(name, varByName, resolved, goos) {
			return true
		}
	}
	return false
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
	r := ResolveTemplateVars(vars, state, rawFull)
	rv, ok := r[name]
	if !ok {
		return ""
	}
	if len(rv.List) > 0 {
		return strings.Join(rv.List, "\n")
	}
	return rv.Scalar
}
