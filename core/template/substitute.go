package template

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// isIntCastVar: вары, которые в sing-box JSON ожидаются как числа, а не строки.
// Плейсхолдер "@name" в шаблоне подменяется целым числом (strconv.Atoi).
func isIntCastVar(name string) bool {
	switch name {
	case "tun_mtu", "mixed_listen_port", "proxy_in_listen_port", "urltest_tolerance":
		return true
	}
	return false
}

// runtimeGlobalPrefix — пространство имён runtime-globals в #if predicates (SPEC 067).
// Ссылка вида "@runtime.platform" / "@runtime.arch" резолвится не из vars, а из
// runtime.GOOS / runtime.GOARCH. Namespace расширяемый: новые поля добавляются в
// runtimeGlobalFields + dispatch в lookupVarScalar.
const runtimeGlobalPrefix = "runtime."

// runtimeGlobalFields — известные поля namespace @runtime (без префикса).
var runtimeGlobalFields = map[string]struct{}{
	"platform": {},
	"arch":     {},
}

// isRuntimeGlobalRef true для имён вида "runtime.*" (после strip "@").
func isRuntimeGlobalRef(name string) bool {
	return strings.HasPrefix(name, runtimeGlobalPrefix)
}

// isKnownRuntimeGlobal true только для @runtime.<известное поле>.
func isKnownRuntimeGlobal(name string) bool {
	if !strings.HasPrefix(name, runtimeGlobalPrefix) {
		return false
	}
	_, ok := runtimeGlobalFields[name[len(runtimeGlobalPrefix):]]
	return ok
}

// SubstituteVarsInJSON заменяет литералы "@name" в дереве JSON на разрешённые значения.
// Параметры goos / goarch используются runtime-globals (@runtime.platform / @runtime.arch)
// в predicates #if construct'а (см. SPEC 067).
func SubstituteVarsInJSON(data []byte, vars []TemplateVar, resolved map[string]ResolvedVar, goos, goarch string) ([]byte, error) {
	out, _, err := substituteVarsInJSONInternal(data, vars, resolved, goos, goarch, false)
	return out, err
}

// SubstituteVarsInJSONStrict — то же что SubstituteVarsInJSON, но возвращает
// ошибку (UnresolvedVarError) если в дереве встречена ссылка на @var, отсутствующий
// в `resolved`. Используется preset-substitute path'ом (см. SPEC 067 Phase 8),
// где unresolved @var означает «пропустить preset целиком», а не подставить пустую строку.
func SubstituteVarsInJSONStrict(data []byte, vars []TemplateVar, resolved map[string]ResolvedVar, goos, goarch string) ([]byte, []string, error) {
	return substituteVarsInJSONInternal(data, vars, resolved, goos, goarch, true)
}

// UnresolvedVarError возвращается SubstituteVarsInJSONStrict если в дереве
// встречены неразрешённые @var ссылки.
type UnresolvedVarError struct {
	Names []string
}

func (e *UnresolvedVarError) Error() string {
	return "unresolved @var(s): " + strings.Join(e.Names, ", ")
}

func substituteVarsInJSONInternal(data []byte, vars []TemplateVar, resolved map[string]ResolvedVar, goos, goarch string, strict bool) ([]byte, []string, error) {
	varTypes := make(map[string]string, len(vars))
	for _, v := range vars {
		if v.Separator {
			continue
		}
		varTypes[v.Name] = v.Type
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var root interface{}
	if err := dec.Decode(&root); err != nil {
		return nil, nil, err
	}
	var unresolved []string
	var unresolvedSink *[]string
	if strict {
		unresolvedSink = &unresolved
	}
	substituteWalkCtx(&root, varTypes, resolved, goos, goarch, unresolvedSink)
	if strict && len(unresolved) > 0 {
		return nil, unresolved, &UnresolvedVarError{Names: unresolved}
	}
	out, err := json.Marshal(root)
	return out, unresolved, err
}

// substituteWalkCtx — internal walker с опциональным sink'ом для unresolved @var
// (используется SubstituteVarsInJSONStrict, SPEC 067 Phase 8). nil sink ==
// legacy lenient behavior (empty string + warn log).
func substituteWalkCtx(v *interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, goos, goarch string, unresolvedSink *[]string) {
	switch x := (*v).(type) {
	case map[string]interface{}:
		// Pre-pass: control-constructs (keys starting with "#").
		// Collect first to avoid mutating during iteration.
		var ctrlKeys []string
		for k := range x {
			if strings.HasPrefix(k, "#") {
				ctrlKeys = append(ctrlKeys, k)
			}
		}
		for _, k := range ctrlKeys {
			raw := x[k]
			switch k {
			case "#if":
				handleIfMapSpreadCtx(x, raw, varTypes, resolved, goos, goarch, unresolvedSink)
				// handleIfMapSpreadCtx always deletes "#if" itself.
			default:
				debuglog.WarnLog("substitute: unknown control-construct %q — dropping", k)
				delete(x, k)
			}
		}
		// Normal field walk.
		for k, val := range x {
			substituteWalkCtx(&val, varTypes, resolved, goos, goarch, unresolvedSink)
			x[k] = val
		}
	case []interface{}:
		// Pre-pass for #if array-element wrappers. We need to filter/replace
		// elements before falling into the legacy single-element collapse.
		hasIfWrapper := false
		for _, elem := range x {
			if m, ok := elem.(map[string]interface{}); ok && len(m) == 1 {
				if _, ok := m["#if"]; ok {
					hasIfWrapper = true
					break
				}
			}
		}
		if hasIfWrapper {
			out := make([]interface{}, 0, len(x))
			for _, elem := range x {
				if m, ok := elem.(map[string]interface{}); ok && len(m) == 1 {
					if body, ok := m["#if"].(map[string]interface{}); ok {
						branch, take := handleIfArrayElementCtx(body, varTypes, resolved, goos, goarch, unresolvedSink)
						if take {
							out = append(out, branch)
						}
						continue
					}
				}
				substituteWalkCtx(&elem, varTypes, resolved, goos, goarch, unresolvedSink)
				out = append(out, elem)
			}
			*v = out
			return
		}
		// Legacy single-element ["@text_list_var"] collapse.
		if len(x) == 1 {
			if s, ok := x[0].(string); ok && strings.HasPrefix(s, "@") {
				name := s[1:]
				if name != "" && !strings.Contains(name, "@") {
					if rep := replacementForPlaceholderCtx(name, varTypes, resolved, unresolvedSink); rep != nil {
						*v = rep
						return
					}
				}
			}
		}
		for i := range x {
			substituteWalkCtx(&x[i], varTypes, resolved, goos, goarch, unresolvedSink)
		}
	case string:
		if strings.HasPrefix(x, "@") {
			name := x[1:]
			if name != "" && !strings.Contains(name, "@") {
				if rep := replacementForPlaceholderCtx(name, varTypes, resolved, unresolvedSink); rep != nil {
					*v = rep
				}
			}
		}
	}
}

func replacementForPlaceholder(name string, varTypes map[string]string, resolved map[string]ResolvedVar) interface{} {
	return replacementForPlaceholderCtx(name, varTypes, resolved, nil)
}

func replacementForPlaceholderCtx(name string, varTypes map[string]string, resolved map[string]ResolvedVar, unresolvedSink *[]string) interface{} {
	r, ok := resolved[name]
	if !ok {
		if unresolvedSink != nil {
			*unresolvedSink = append(*unresolvedSink, name)
		}
		debuglog.WarnLog("substitute: unresolved @%s", name)
		return ""
	}
	typ := varTypes[name]
	if typ == "text_list" {
		if len(r.List) == 0 {
			debuglog.WarnLog("substitute: empty text_list @%s", name)
			return []interface{}{}
		}
		out := make([]interface{}, len(r.List))
		for i, s := range r.List {
			out[i] = s
		}
		return out
	}
	s := strings.TrimSpace(r.Scalar)
	if typ == "bool" {
		if s == "" {
			return false
		}
		return strings.EqualFold(s, "true")
	}
	if s == "" {
		debuglog.WarnLog("substitute: empty scalar @%s", name)
		if isIntCastVar(name) {
			return 0
		}
		return ""
	}
	if isIntCastVar(name) {
		n, err := strconv.Atoi(s)
		if err != nil {
			debuglog.WarnLog("substitute: invalid int @%s: %v", name, err)
			return 0
		}
		return n
	}
	return s
}

// ---------------------------------------------------------------------------
// #if construct (SPEC 067)
// ---------------------------------------------------------------------------

// handleIfMapSpreadCtx evaluates the #if construct in map-spread mode and merges
// the selected branch's fields into parent. Always deletes the "#if" key.
func handleIfMapSpreadCtx(parent map[string]interface{}, rawBody interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, goos, goarch string, unresolvedSink *[]string) {
	defer delete(parent, "#if")
	body, ok := rawBody.(map[string]interface{})
	if !ok {
		debuglog.WarnLog("substitute: #if body is not an object — skipping")
		return
	}
	branch, take := selectIfBranch(body, varTypes, resolved, goos, goarch)
	if !take {
		return
	}
	// Substitute placeholders inside selected branch first.
	substituteWalkCtx(&branch, varTypes, resolved, goos, goarch, unresolvedSink)
	branchMap, ok := branch.(map[string]interface{})
	if !ok {
		debuglog.WarnLog("substitute: #if branch in map-spread context is not an object — skipping merge")
		return
	}
	for k, v := range branchMap {
		parent[k] = v
	}
}

// handleIfArrayElementCtx evaluates the #if construct in array-element mode.
// take=false means drop element from array; take=true means include branch
// (substituted) at this index.
func handleIfArrayElementCtx(body map[string]interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, goos, goarch string, unresolvedSink *[]string) (interface{}, bool) {
	branch, take := selectIfBranch(body, varTypes, resolved, goos, goarch)
	if !take {
		return nil, false
	}
	substituteWalkCtx(&branch, varTypes, resolved, goos, goarch, unresolvedSink)
	return branch, true
}

// selectIfBranch evaluates the condition and picks the value/else branch.
// Returns (branch, take=true) when a branch was selected; (nil, false) when
// condition is false and no else is present.
func selectIfBranch(body map[string]interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, goos, goarch string) (interface{}, bool) {
	cond := evaluateIfCondition(body, varTypes, resolved, goos, goarch)
	if cond {
		val, hasVal := body["value"]
		if !hasVal {
			debuglog.WarnLog("substitute: #if missing required \"value\" field — skipping")
			return nil, false
		}
		return val, true
	}
	if elseVal, hasElse := body["else"]; hasElse {
		return elseVal, true
	}
	return nil, false
}

// evaluateIfCondition extracts and/or and evaluates predicate list.
// Defensive on both-present (warn, false) and neither-present (warn, true).
// Empty `and` → vacuously true; empty `or` → vacuously false.
func evaluateIfCondition(body map[string]interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, goos, goarch string) bool {
	andRaw, hasAnd := body["and"]
	orRaw, hasOr := body["or"]
	if hasAnd && hasOr {
		debuglog.WarnLog("substitute: #if has both \"and\" and \"or\" — treating as false")
		return false
	}
	if !hasAnd && !hasOr {
		debuglog.WarnLog("substitute: #if has neither \"and\" nor \"or\" — treating as true")
		return true
	}
	if hasAnd {
		list, ok := andRaw.([]interface{})
		if !ok {
			debuglog.WarnLog("substitute: #if.and is not an array — treating as false")
			return false
		}
		return evaluatePredicateList(list, true, varTypes, resolved, goos, goarch)
	}
	list, ok := orRaw.([]interface{})
	if !ok {
		debuglog.WarnLog("substitute: #if.or is not an array — treating as false")
		return false
	}
	return evaluatePredicateList(list, false, varTypes, resolved, goos, goarch)
}

// evaluatePredicateList short-circuits AND/OR over predicates.
func evaluatePredicateList(list []interface{}, isAnd bool, varTypes map[string]string, resolved map[string]ResolvedVar, goos, goarch string) bool {
	if isAnd {
		// Empty AND → vacuously true.
		for _, p := range list {
			if !evaluatePredicate(p, varTypes, resolved, goos, goarch) {
				return false
			}
		}
		return true
	}
	// Empty OR → vacuously false.
	for _, p := range list {
		if evaluatePredicate(p, varTypes, resolved, goos, goarch) {
			return true
		}
	}
	return false
}

// evaluatePredicate dispatches the 8 predicate forms (see SPEC 067).
// Recurses for #not.
func evaluatePredicate(p interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, goos, goarch string) bool {
	switch pv := p.(type) {
	case string:
		// Bare "@var" → bool template var → scalar == "true".
		if !strings.HasPrefix(pv, "@") {
			debuglog.WarnLog("substitute: #if predicate bare string %q must start with @ — treating as false", pv)
			return false
		}
		name := strings.TrimPrefix(pv, "@")
		// Runtime globals not allowed in bare form (per SPEC).
		if isRuntimeGlobalRef(name) {
			debuglog.WarnLog("substitute: #if bare predicate %q is not allowed for runtime globals — treating as false", pv)
			return false
		}
		scalar, _, _, found := lookupVarScalar(name, resolved, goos, goarch)
		if !found {
			debuglog.WarnLog("substitute: #if predicate references unknown var @%s — treating as false", name)
			return false
		}
		return strings.EqualFold(strings.TrimSpace(scalar), "true")
	case map[string]interface{}:
		if len(pv) != 1 {
			debuglog.WarnLog("substitute: #if predicate object must have exactly one key — treating as false")
			return false
		}
		for k, v := range pv {
			if k == "#not" {
				return !evaluatePredicate(v, varTypes, resolved, goos, goarch)
			}
			if strings.HasPrefix(k, "@") {
				name := strings.TrimPrefix(k, "@")
				return evaluateVarPredicate(name, v, varTypes, resolved, goos, goarch)
			}
			debuglog.WarnLog("substitute: #if predicate has unknown key %q — treating as false", k)
			return false
		}
	}
	debuglog.WarnLog("substitute: #if predicate has invalid shape — treating as false")
	return false
}

// evaluateVarPredicate dispatches RHS forms for {"@var": ...} predicates.
func evaluateVarPredicate(varName string, rhs interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, goos, goarch string) bool {
	scalar, isList, list, found := lookupVarScalar(varName, resolved, goos, goarch)
	if !found {
		debuglog.WarnLog("substitute: #if predicate references unknown var @%s — treating as false", varName)
		return false
	}
	switch r := rhs.(type) {
	case string:
		// "#notEmpty" / "#isEmpty" — no-arg predicate.
		if r == "#notEmpty" {
			return checkNotEmpty(scalar, isList, list, varName, varTypes)
		}
		if r == "#isEmpty" {
			return !checkNotEmpty(scalar, isList, list, varName, varTypes)
		}
		if strings.HasPrefix(r, "#") {
			debuglog.WarnLog("substitute: #if predicate has unknown no-arg form %q — treating as false", r)
			return false
		}
		// Literal equality — substitute @var if present.
		lit := substituteSimpleString(r, varTypes, resolved)
		return strings.TrimSpace(scalar) == lit
	case map[string]interface{}:
		if len(r) != 1 {
			debuglog.WarnLog("substitute: #if predicate RHS object must have exactly one key — treating as false")
			return false
		}
		for k, arg := range r {
			switch k {
			case "#in":
				return checkInList(scalar, arg, varTypes, resolved)
			case "#notIn":
				return !checkInList(scalar, arg, varTypes, resolved)
			case "#matches":
				return checkMatches(scalar, arg, varTypes, resolved)
			default:
				debuglog.WarnLog("substitute: #if predicate has unknown arg-form %q — treating as false", k)
				return false
			}
		}
	}
	debuglog.WarnLog("substitute: #if predicate RHS has invalid shape — treating as false")
	return false
}

// lookupVarScalar resolves @runtime.* globals (case-sensitive lower-case) and
// otherwise looks up the name in `resolved`. Returns (scalar, isList, list,
// found). Unknown @runtime.<field> → not found (defensive).
func lookupVarScalar(name string, resolved map[string]ResolvedVar, goos, goarch string) (string, bool, []string, bool) {
	if isRuntimeGlobalRef(name) {
		switch name[len(runtimeGlobalPrefix):] {
		case "platform":
			return goos, false, nil, true
		case "arch":
			return goarch, false, nil, true
		default:
			debuglog.WarnLog("substitute: unknown runtime global @%s — treating as not found", name)
			return "", false, nil, false
		}
	}
	r, ok := resolved[name]
	if !ok {
		return "", false, nil, false
	}
	if len(r.List) > 0 || (r.Scalar == "" && r.List != nil) {
		return r.Scalar, true, r.List, true
	}
	return r.Scalar, false, nil, true
}

// checkNotEmpty applies the #notEmpty predicate semantics:
// text → len(trim(scalar)) > 0; text_list → len(list) > 0; bool → scalar == "true".
func checkNotEmpty(scalar string, isList bool, list []string, varName string, varTypes map[string]string) bool {
	if isList {
		return len(list) > 0
	}
	if typ, ok := varTypes[varName]; ok && typ == "bool" {
		return strings.EqualFold(strings.TrimSpace(scalar), "true")
	}
	if typ, ok := varTypes[varName]; ok && typ == "text_list" {
		return len(list) > 0
	}
	return len(strings.TrimSpace(scalar)) > 0
}

// checkInList tests whether scalar is in the args list. argsRaw may be either
// a JSON array of strings or a single "@text_list_var" reference.
func checkInList(scalar string, argsRaw interface{}, varTypes map[string]string, resolved map[string]ResolvedVar) bool {
	trimmed := strings.TrimSpace(scalar)
	// Single "@text_list_var" string — resolve to list.
	if s, ok := argsRaw.(string); ok {
		if strings.HasPrefix(s, "@") {
			name := strings.TrimPrefix(s, "@")
			if typ, exists := varTypes[name]; exists && typ == "text_list" {
				if r, rOK := resolved[name]; rOK {
					for _, item := range r.List {
						if item == trimmed {
							return true
						}
					}
					return false
				}
			}
		}
		debuglog.WarnLog("substitute: #if #in arg as string %q is not a @text_list_var — treating as false", s)
		return false
	}
	// Array form.
	list, ok := argsRaw.([]interface{})
	if !ok {
		debuglog.WarnLog("substitute: #if #in arg is not an array or @text_list_var — treating as false")
		return false
	}
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			continue
		}
		lit := substituteSimpleString(s, varTypes, resolved)
		if lit == trimmed {
			return true
		}
	}
	return false
}

// checkMatches compiles the regex pattern (after @var substitution) and tests it
// against trimmed scalar.
func checkMatches(scalar string, patternRaw interface{}, varTypes map[string]string, resolved map[string]ResolvedVar) bool {
	pat, ok := patternRaw.(string)
	if !ok {
		debuglog.WarnLog("substitute: #if #matches pattern is not a string — treating as false")
		return false
	}
	pat = substituteSimpleString(pat, varTypes, resolved)
	re, err := regexp.Compile(pat)
	if err != nil {
		debuglog.WarnLog("substitute: #if #matches invalid regex %q: %v — treating as false", pat, err)
		return false
	}
	return re.MatchString(strings.TrimSpace(scalar))
}

// substituteSimpleString is used for controlled @var substitution inside
// predicate args (literal equality, #matches pattern, individual #in elements).
// If s == "@varname", resolves via replacementForPlaceholder and converts to
// string. Otherwise returns s as-is.
func substituteSimpleString(s string, varTypes map[string]string, resolved map[string]ResolvedVar) string {
	if !strings.HasPrefix(s, "@") {
		return s
	}
	name := strings.TrimPrefix(s, "@")
	if name == "" || strings.Contains(name, "@") {
		return s
	}
	rep := replacementForPlaceholder(name, varTypes, resolved)
	if rep == nil {
		return s
	}
	switch v := rep.(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(v)
	}
	return s
}
