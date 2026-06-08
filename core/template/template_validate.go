package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// reservedVarNames — имена vars[].name, зарезервированные движком (SPEC 067).
// "runtime" — namespace runtime-globals (@runtime.platform / @runtime.arch);
// объявлять var с таким именем нельзя. Strict lower-case.
var reservedVarNames = map[string]struct{}{
	"runtime": {},
}

// validateOuterIfRefs проверяет элементы outer params[].if / if_or, vars[].if / if_or,
// presets[].if / if_or. Per SPEC 067 Phase 3:
//   - элемент должен начинаться с "@" → strip и lookup в varByName (type=bool).
//   - bare элемент (legacy) → loader error с указанием канонической формы.
//   - runtime globals (`@runtime.platform` / `@runtime.arch`) запрещены в outer if[] (только #if).
func validateOuterIfRefs(ctx string, ifNames, ifOrNames []string, varByName map[string]TemplateVar) error {
	if len(ifNames) > 0 && len(ifOrNames) > 0 {
		return fmt.Errorf("%s: if and if_or cannot both be set", ctx)
	}
	if err := validateOuterIfList(ctx+".if", ifNames, varByName); err != nil {
		return err
	}
	if err := validateOuterIfList(ctx+".if_or", ifOrNames, varByName); err != nil {
		return err
	}
	return nil
}

func validateOuterIfList(ctx string, names []string, varByName map[string]TemplateVar) error {
	for _, raw := range names {
		if !strings.HasPrefix(raw, "@") {
			return fmt.Errorf("template: %s has bare var-ref %q in if[]; use canonical %q form", ctx, raw, "@"+raw)
		}
		name := strings.TrimPrefix(raw, "@")
		if name == "" {
			return fmt.Errorf("%s: empty var-ref after @", ctx)
		}
		if isRuntimeGlobalRef(name) {
			return fmt.Errorf("%s: runtime global %q is not allowed in outer if[]/if_or[] (only inside #if predicates)", ctx, raw)
		}
		vd, ok := varByName[name]
		if !ok {
			return fmt.Errorf("%s: unknown var %q", ctx, raw)
		}
		if vd.Type != "bool" {
			return fmt.Errorf("%s: var %q must be type bool, got %q", ctx, raw, vd.Type)
		}
	}
	return nil
}

// validWizardVarNameRE — лексика vars[].name (SPEC 032).
var validWizardVarNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateWizardTemplate проверяет vars и ссылки @name в config и params (PLAN / TASKS 032).
// Секция vars в сыром JSON на плейсхолдеры @ не сканируется — только config и params.
// SPEC 067: дополнительно валидирует #if constructs внутри params[].value и
// проверяет, что vars[].name не пересекается с runtime-globals (`platform` / `arch`).
func ValidateWizardTemplate(vars []TemplateVar, params []TemplateParam, config json.RawMessage) error {
	names := make(map[string]struct{})
	varByName := make(map[string]TemplateVar, len(vars))
	for i, v := range vars {
		if v.Separator {
			if err := validateVarsSeparator(i, v); err != nil {
				return err
			}
			continue
		}
		nm := strings.TrimSpace(v.Name)
		if nm == "" {
			return fmt.Errorf("vars[%d]: empty name", i)
		}
		if !validWizardVarNameRE.MatchString(nm) {
			return fmt.Errorf("vars[%d]: invalid name %q (expected [A-Za-z_][A-Za-z0-9_]*)", i, nm)
		}
		if _, dup := names[nm]; dup {
			return fmt.Errorf("vars: duplicate name %q", nm)
		}
		if _, reserved := reservedVarNames[nm]; reserved {
			return fmt.Errorf("template: vars[].name %q is reserved (runtime global namespace); rename", nm)
		}
		names[nm] = struct{}{}
		varByName[nm] = v
	}

	for i, v := range vars {
		ctx := fmt.Sprintf("vars[%d]", i)
		if err := validateOuterIfRefs(ctx, v.If, v.IfOr, varByName); err != nil {
			return err
		}
		// vars[].default_value может содержать #if (SPEC 067) — но только с
		// @runtime.* globals (user-var refs запрещены: на этапе resolve дефолтов
		// другие vars ещё не разрешены, порядок не гарантирован).
		if err := validateDefaultValueIf(v.DefaultValue, ctx); err != nil {
			return err
		}
	}

	for i, p := range params {
		ctx := fmt.Sprintf("params[%d]", i)
		if err := validateOuterIfRefs(ctx, p.If, p.IfOr, varByName); err != nil {
			return err
		}
		// Validate #if constructs first — they have more specific errors than
		// the generic placeholder check (e.g. bare "@runtime.platform" in #if predicate).
		if err := validateIfConstruct(p.Value, varByName, ctx+".value"); err != nil {
			return err
		}
		refs, err := collectPlaceholderNamesFromJSON(p.Value)
		if err != nil {
			return fmt.Errorf("params[%d].value: %w", i, err)
		}
		for _, ref := range refs {
			if isRuntimeGlobalRef(ref) {
				continue
			}
			if _, ok := names[ref]; !ok {
				return fmt.Errorf("params[%d]: @%q is not declared in vars", i, ref)
			}
		}
	}

	refs, err := collectPlaceholderNamesFromJSON(config)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	for _, ref := range refs {
		if _, ok := names[ref]; !ok {
			return fmt.Errorf("config: @%q is not declared in vars", ref)
		}
	}
	return nil
}

// validateVarsSeparator: {"separator": true} — только оформление Settings; без name и плейсхолдеров.
func validateVarsSeparator(i int, v TemplateVar) error {
	ctx := fmt.Sprintf("vars[%d]", i)
	if strings.TrimSpace(v.Name) != "" {
		return fmt.Errorf("%s: separator must not set name", ctx)
	}
	if strings.TrimSpace(v.Type) != "" {
		return fmt.Errorf("%s: separator must not set type", ctx)
	}
	if !v.DefaultValue.IsEmpty() || strings.TrimSpace(v.DefaultNode) != "" {
		return fmt.Errorf("%s: separator must not set default_value or default_node", ctx)
	}
	if len(v.Options) > 0 {
		return fmt.Errorf("%s: separator must not set options", ctx)
	}
	if strings.TrimSpace(v.Title) != "" || strings.TrimSpace(v.Tooltip) != "" {
		return fmt.Errorf("%s: separator must not set title or tooltip", ctx)
	}
	if len(v.If) > 0 || len(v.IfOr) > 0 {
		return fmt.Errorf("%s: separator must not set if or if_or", ctx)
	}
	wu := strings.ToLower(strings.TrimSpace(v.WizardUI))
	if wu != "" && wu != "hidden" {
		return fmt.Errorf("%s: separator wizard_ui must be empty or \"hidden\"", ctx)
	}
	return nil
}

func collectPlaceholderNamesFromJSON(raw json.RawMessage) ([]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var v interface{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var out []string
	walkJSONPlaceholders(v, &out)
	return out, nil
}

func walkJSONPlaceholders(v interface{}, out *[]string) {
	switch x := v.(type) {
	case map[string]interface{}:
		for _, val := range x {
			walkJSONPlaceholders(val, out)
		}
	case []interface{}:
		if len(x) == 1 {
			if s, ok := x[0].(string); ok {
				if name := parseAtVarName(s); name != "" {
					*out = append(*out, name)
					return
				}
			}
		}
		for _, el := range x {
			walkJSONPlaceholders(el, out)
		}
	case string:
		if name := parseAtVarName(x); name != "" {
			*out = append(*out, name)
		}
	case json.Number, bool, nil:
		// skip
	default:
		// float64 from JSON — не плейсхолдеры
	}
}

func parseAtVarName(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "@") {
		return ""
	}
	name := strings.TrimSpace(s[1:])
	if name == "" || strings.Contains(name, "@") {
		return ""
	}
	return name
}

// ---------------------------------------------------------------------------
// #if construct validation (SPEC 067 Phase 2)
// ---------------------------------------------------------------------------

// validateIfConstruct рекурсивно обходит дерево JSON (params[].value, vars[].default_value,
// presets[].outbounds[].body etc.) и валидирует обнаруженные #if construct'ы. Поддерживает
// два режима:
//   - map-spread: ключ "#if" внутри обычного объекта.
//   - array-element: элемент массива вида {"#if": {...}} — single-key wrapper.
//
// Неизвестные `#*` ключи в map-spread позиции → warn-log без ошибки (forward-compat).
func validateIfConstruct(rawValue json.RawMessage, varByName map[string]TemplateVar, context string) error {
	if len(bytes.TrimSpace(rawValue)) == 0 {
		return nil
	}
	var v interface{}
	dec := json.NewDecoder(bytes.NewReader(rawValue))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		// JSON ошибки разруливаются другими уровнями — здесь молча.
		return nil
	}
	return walkValidateIf(v, varByName, context)
}

// validateDefaultValueIf валидирует #if внутри vars[].default_value. Передаём
// ПУСТОЙ varByName — это запрещает любые ссылки на user-vars (они резолвятся как
// "unknown var"); разрешены только @runtime.* globals. См. SPEC 067.
//
// Два размещения:
//   - top-level: default_value == {"#if": {...}};
//   - per-platform: {"win7": {"#if": {...}}, "default": "..."} — значение ключа = дерево.
func validateDefaultValueIf(dv VarDefaultValue, ctx string) error {
	if len(dv.PerPlatform) == 0 {
		return nil
	}
	emptyVars := map[string]TemplateVar{}
	if body, ok := dv.PerPlatform["#if"]; ok && len(dv.PerPlatform) == 1 {
		return validateIfBody(body, emptyVars, ctx+".default_value.#if")
	}
	for k, node := range dv.PerPlatform {
		tree, ok := node.(map[string]interface{})
		if !ok {
			continue // обычное строковое значение
		}
		if err := walkValidateIf(tree, emptyVars, fmt.Sprintf("%s.default_value[%s]", ctx, k)); err != nil {
			return err
		}
	}
	return nil
}

func walkValidateIf(v interface{}, varByName map[string]TemplateVar, context string) error {
	switch x := v.(type) {
	case map[string]interface{}:
		// Validate control-construct keys.
		for k, raw := range x {
			if !strings.HasPrefix(k, "#") {
				continue
			}
			switch k {
			case "#if":
				if err := validateIfBody(raw, varByName, context+".#if"); err != nil {
					return err
				}
			default:
				debuglog.WarnLog("template: %s has unknown control-construct %q — ignored (forward-compat)", context, k)
			}
		}
		// Recurse into all values.
		for k, val := range x {
			if err := walkValidateIf(val, varByName, context+"."+k); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, elem := range x {
			subCtx := fmt.Sprintf("%s[%d]", context, i)
			// Array-element mode: single-key {"#if": {...}} wrapper.
			if m, ok := elem.(map[string]interface{}); ok && len(m) == 1 {
				if body, ok := m["#if"]; ok {
					if err := validateIfBody(body, varByName, subCtx+".#if"); err != nil {
						return err
					}
					continue
				}
			}
			if err := walkValidateIf(elem, varByName, subCtx); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateIfBody проверяет тело #if construct (and/or/value/else).
func validateIfBody(raw interface{}, varByName map[string]TemplateVar, ctx string) error {
	body, ok := raw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("%s: body must be an object", ctx)
	}
	andRaw, hasAnd := body["and"]
	orRaw, hasOr := body["or"]
	if hasAnd && hasOr {
		return fmt.Errorf("%s: must have exactly one of \"and\" or \"or\" (not both)", ctx)
	}
	if !hasAnd && !hasOr {
		return fmt.Errorf("%s: must have one of \"and\" or \"or\"", ctx)
	}
	var list []interface{}
	var listCtx string
	if hasAnd {
		list, ok = andRaw.([]interface{})
		if !ok {
			return fmt.Errorf("%s.and: must be an array", ctx)
		}
		listCtx = ctx + ".and"
	} else {
		list, ok = orRaw.([]interface{})
		if !ok {
			return fmt.Errorf("%s.or: must be an array", ctx)
		}
		listCtx = ctx + ".or"
	}
	if len(list) == 0 {
		return fmt.Errorf("%s: predicate list must be non-empty", listCtx)
	}
	for i, p := range list {
		if err := validateIfPredicate(p, varByName, fmt.Sprintf("%s[%d]", listCtx, i)); err != nil {
			return err
		}
	}
	// value required, not nil.
	valField, hasVal := body["value"]
	if !hasVal {
		return fmt.Errorf("%s: missing required \"value\" field", ctx)
	}
	if valField == nil {
		return fmt.Errorf("%s.value: must not be null", ctx)
	}
	// Recurse into value (it may itself contain nested #if).
	if err := walkValidateIf(valField, varByName, ctx+".value"); err != nil {
		return err
	}
	if elseField, hasElse := body["else"]; hasElse {
		if elseField == nil {
			return fmt.Errorf("%s.else: must not be null", ctx)
		}
		if err := walkValidateIf(elseField, varByName, ctx+".else"); err != nil {
			return err
		}
	}
	return nil
}

// validateIfPredicate валидирует одну форму predicate (см. SPEC 067 §Expression language).
func validateIfPredicate(p interface{}, varByName map[string]TemplateVar, ctx string) error {
	switch pv := p.(type) {
	case string:
		// Bare "@var" — bool template var; @runtime.* не разрешены в bare form.
		if !strings.HasPrefix(pv, "@") {
			return fmt.Errorf("%s: bare predicate %q must start with @", ctx, pv)
		}
		name := strings.TrimPrefix(pv, "@")
		if name == "" {
			return fmt.Errorf("%s: empty var name after @", ctx)
		}
		if isRuntimeGlobalRef(name) {
			return fmt.Errorf("%s: runtime global %q is not allowed in bare form (not bool); use {%q: ...}", ctx, pv, pv)
		}
		vd, ok := varByName[name]
		if !ok {
			return fmt.Errorf("%s: references unknown var %q", ctx, pv)
		}
		if vd.Type != "bool" {
			return fmt.Errorf("%s: bare predicate %q requires bool var, got type %q", ctx, pv, vd.Type)
		}
		return nil
	case map[string]interface{}:
		if len(pv) != 1 {
			return fmt.Errorf("%s: predicate object must have exactly one key", ctx)
		}
		for k, rhs := range pv {
			if k == "#not" {
				if rhs == nil {
					return fmt.Errorf("%s.#not: requires inner predicate (got null)", ctx)
				}
				if m, ok := rhs.(map[string]interface{}); ok && len(m) == 0 {
					return fmt.Errorf("%s.#not: requires inner predicate (got empty object)", ctx)
				}
				return validateIfPredicate(rhs, varByName, ctx+".#not")
			}
			if !strings.HasPrefix(k, "@") {
				return fmt.Errorf("%s: predicate key %q must start with @ or be #not", ctx, k)
			}
			name := strings.TrimPrefix(k, "@")
			if name == "" {
				return fmt.Errorf("%s: empty var name after @", ctx)
			}
			return validateVarPredicateRHS(name, rhs, varByName, ctx+"."+k)
		}
	}
	return fmt.Errorf("%s: predicate has invalid shape", ctx)
}

// validateVarPredicateRHS — dispatch по RHS для {"@var": <rhs>} predicates.
func validateVarPredicateRHS(varName string, rhs interface{}, varByName map[string]TemplateVar, ctx string) error {
	// Resolve var type. Globals @runtime.* are always text-like.
	var varType string
	isRuntimeGlobal := false
	if isRuntimeGlobalRef(varName) {
		if !isKnownRuntimeGlobal(varName) {
			return fmt.Errorf("%s: unknown runtime global @%s (known: @runtime.platform, @runtime.arch)", ctx, varName)
		}
		isRuntimeGlobal = true
		varType = "text"
	} else {
		vd, ok := varByName[varName]
		if !ok {
			return fmt.Errorf("%s: references unknown var @%s", ctx, varName)
		}
		varType = vd.Type
	}
	switch r := rhs.(type) {
	case string:
		// "#notEmpty" / "#isEmpty" — no-arg predicate (any of text/text_list/bool).
		if r == "#notEmpty" || r == "#isEmpty" {
			switch varType {
			case "text", "text_list", "bool":
				return nil
			default:
				return fmt.Errorf("%s: %s not applicable to var type %q", ctx, r, varType)
			}
		}
		if strings.HasPrefix(r, "#") {
			return fmt.Errorf("%s: unknown no-arg predicate %q", ctx, r)
		}
		// Literal equality — var type must be text (text_list equality invalid).
		if isRuntimeGlobal {
			return nil
		}
		switch varType {
		case "text":
			return nil
		case "text_list":
			return fmt.Errorf("%s: literal equality not applicable to text_list var (use {#in: [...]})", ctx)
		default:
			return fmt.Errorf("%s: literal equality not applicable to var type %q (requires text)", ctx, varType)
		}
	case map[string]interface{}:
		if len(r) != 1 {
			return fmt.Errorf("%s: predicate RHS object must have exactly one key", ctx)
		}
		for k, arg := range r {
			switch k {
			case "#in", "#notIn":
				return validateInArg(varType, isRuntimeGlobal, arg, varByName, ctx+"."+k)
			case "#matches":
				return validateMatchesArg(varType, isRuntimeGlobal, arg, ctx+"."+k)
			default:
				return fmt.Errorf("%s: unknown arg-taking predicate %q", ctx, k)
			}
		}
	}
	return fmt.Errorf("%s: predicate RHS has invalid shape", ctx)
}

// validateInArg — args для #in/#notIn: либо []string, либо "@text_list_var" string.
func validateInArg(varType string, isRuntimeGlobal bool, arg interface{}, varByName map[string]TemplateVar, ctx string) error {
	if !isRuntimeGlobal && varType != "text" && varType != "text_list" {
		return fmt.Errorf("%s: #in/#notIn not applicable to var type %q", ctx, varType)
	}
	switch a := arg.(type) {
	case string:
		// Must be "@text_list_var".
		if !strings.HasPrefix(a, "@") {
			return fmt.Errorf("%s: string arg %q must be @text_list_var reference", ctx, a)
		}
		name := strings.TrimPrefix(a, "@")
		vd, ok := varByName[name]
		if !ok {
			return fmt.Errorf("%s: references unknown var %q", ctx, a)
		}
		if vd.Type != "text_list" {
			return fmt.Errorf("%s: arg var %q must be type text_list, got %q", ctx, a, vd.Type)
		}
		return nil
	case []interface{}:
		if len(a) == 0 {
			return fmt.Errorf("%s: list must be non-empty", ctx)
		}
		for i, item := range a {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("%s[%d]: list element must be string", ctx, i)
			}
		}
		return nil
	}
	return fmt.Errorf("%s: arg must be []string or @text_list_var reference", ctx)
}

// validateMatchesArg — arg для #matches: string, который компилируется как Go-regexp.
func validateMatchesArg(varType string, isRuntimeGlobal bool, arg interface{}, ctx string) error {
	if !isRuntimeGlobal && varType != "text" {
		return fmt.Errorf("%s: #matches not applicable to var type %q (requires text)", ctx, varType)
	}
	pat, ok := arg.(string)
	if !ok {
		return fmt.Errorf("%s: pattern must be a string", ctx)
	}
	// If pattern is a placeholder (@var) — skip compilation (resolved at runtime).
	if strings.HasPrefix(pat, "@") {
		return nil
	}
	if _, err := regexp.Compile(pat); err != nil {
		return fmt.Errorf("%s: invalid regex %q: %w", ctx, pat, err)
	}
	return nil
}
