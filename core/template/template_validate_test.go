package template

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateWizardTemplate_ok(t *testing.T) {
	vars := []TemplateVar{
		{Name: "tun", Type: "bool"},
		{Name: "x", Type: "text"},
	}
	params := []TemplateParam{
		{Name: "inbounds", Platforms: []string{"darwin"}, If: []string{"@tun"}, Value: json.RawMessage(`[{"listen_port":"@x"}]`)},
	}
	cfg := json.RawMessage(`{"log":{"level":"@x"}}`)
	if err := ValidateWizardTemplate(vars, params, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestValidateWizardTemplate_duplicateVar(t *testing.T) {
	vars := []TemplateVar{
		{Name: "a", Type: "text"},
		{Name: "a", Type: "text"},
	}
	err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateWizardTemplate_ifAndIfOrRejected(t *testing.T) {
	vars := []TemplateVar{{Name: "a", Type: "bool"}, {Name: "b", Type: "bool"}}
	params := []TemplateParam{
		{If: []string{"@a"}, IfOr: []string{"@b"}, Value: json.RawMessage(`[]`)},
	}
	err := ValidateWizardTemplate(vars, params, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateWizardTemplate_ifOrNotBool(t *testing.T) {
	vars := []TemplateVar{{Name: "tun", Type: "text"}}
	params := []TemplateParam{
		{IfOr: []string{"@tun"}, Value: json.RawMessage(`[]`)},
	}
	err := ValidateWizardTemplate(vars, params, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateWizardTemplate_ifNotBool(t *testing.T) {
	vars := []TemplateVar{{Name: "tun", Type: "text"}}
	params := []TemplateParam{
		{If: []string{"@tun"}, Value: json.RawMessage(`[]`)},
	}
	err := ValidateWizardTemplate(vars, params, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateWizardTemplate_unknownPlaceholder(t *testing.T) {
	err := ValidateWizardTemplate([]TemplateVar{{Name: "a", Type: "text"}}, nil, json.RawMessage(`{"k":"@b"}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateWizardTemplate_invalidVarName(t *testing.T) {
	err := ValidateWizardTemplate([]TemplateVar{{Name: "9bad", Type: "text"}}, nil, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateWizardTemplate_varIfAndIfOrRejected(t *testing.T) {
	vars := []TemplateVar{
		{Name: "a", Type: "bool"},
		{Name: "b", Type: "bool", If: []string{"@a"}, IfOr: []string{"@a"}},
	}
	err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateWizardTemplate_separatorOk(t *testing.T) {
	vars := []TemplateVar{
		{Name: "a", Type: "text"},
		{Separator: true},
		{Name: "b", Type: "text"},
	}
	if err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{"k":"@a","m":"@b"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestValidateWizardTemplate_separatorWithNameRejected(t *testing.T) {
	vars := []TemplateVar{{Separator: true, Name: "x"}}
	err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateWizardTemplate_varIfNotBool(t *testing.T) {
	vars := []TemplateVar{
		{Name: "x", Type: "text"},
		{Name: "y", Type: "text", If: []string{"@x"}},
	}
	err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// SPEC 067 Phase 2 — #if construct validation
// ---------------------------------------------------------------------------

// ifValidatorVars returns a stable set of vars used by #if validation tests:
// flag (bool), other_flag (bool), name (text), names (text_list).
func ifValidatorVars() []TemplateVar {
	return []TemplateVar{
		{Name: "flag", Type: "bool"},
		{Name: "other_flag", Type: "bool"},
		{Name: "name", Type: "text"},
		{Name: "names", Type: "text_list"},
	}
}

func ifValidatorParam(valueJSON string) []TemplateParam {
	return []TemplateParam{
		{Name: "inbounds", Value: json.RawMessage(valueJSON)},
	}
}

func TestIf_BothAndOr_LoaderError(t *testing.T) {
	val := `[{
		"type": "mixed",
		"#if": {
			"and": ["@flag"],
			"or":  ["@other_flag"],
			"value": {"users": []}
		}
	}]`
	err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("expected mutual-exclusion error, got %v", err)
	}
}

func TestIf_NeitherAndOr_LoaderError(t *testing.T) {
	val := `[{
		"type": "mixed",
		"#if": {"value": {"users": []}}
	}]`
	err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "must have one of") {
		t.Fatalf("expected neither-and-nor-or error, got %v", err)
	}
}

func TestIf_ValueMissing_LoaderError(t *testing.T) {
	val := `[{
		"type": "mixed",
		"#if": {"and": ["@flag"]}
	}]`
	err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "missing required \"value\"") {
		t.Fatalf("expected missing-value error, got %v", err)
	}
}

func TestIf_NotWithoutInner_LoaderError(t *testing.T) {
	val := `[{
		"#if": {
			"and": [{"#not": null}],
			"value": {"x": 1}
		}
	}]`
	err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "#not") {
		t.Fatalf("expected #not-null error, got %v", err)
	}

	val2 := `[{
		"#if": {
			"and": [{"#not": {}}],
			"value": {"x": 1}
		}
	}]`
	err = ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val2), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "#not") {
		t.Fatalf("expected #not-empty error, got %v", err)
	}
}

func TestIf_UnknownPredicate_LoaderError(t *testing.T) {
	val := `[{
		"#if": {
			"and": [{"@name": "#unknownPredicate"}],
			"value": {"x": 1}
		}
	}]`
	err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "unknown no-arg predicate") {
		t.Fatalf("expected unknown predicate error, got %v", err)
	}
}

func TestIf_MatchesInvalidRegex_LoaderError(t *testing.T) {
	val := `[{
		"#if": {
			"and": [{"@name": {"#matches": "[invalid"}}],
			"value": {"x": 1}
		}
	}]`
	err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "invalid regex") {
		t.Fatalf("expected invalid-regex error, got %v", err)
	}
}

func TestIf_BareBoolOnTextVar_LoaderError(t *testing.T) {
	val := `[{
		"#if": {
			"and": ["@name"],
			"value": {"x": 1}
		}
	}]`
	err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "bare predicate") {
		t.Fatalf("expected bare-on-text error, got %v", err)
	}
}

func TestIf_BarePlatform_LoaderError(t *testing.T) {
	val := `[{
		"#if": {
			"and": ["@runtime.platform"],
			"value": {"x": 1}
		}
	}]`
	err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "runtime global") {
		t.Fatalf("expected runtime-global-bare error, got %v", err)
	}
}

func TestIf_UnknownVarInPredicate_LoaderError(t *testing.T) {
	val := `[{
		"#if": {
			"and": [{"@unknown_var": "literal"}],
			"value": {"x": 1}
		}
	}]`
	err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "unknown var") {
		t.Fatalf("expected unknown-var error, got %v", err)
	}
}

func TestIf_LiteralEqOnTextList_LoaderError(t *testing.T) {
	val := `[{
		"#if": {
			"and": [{"@names": "abc"}],
			"value": {"x": 1}
		}
	}]`
	err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "literal equality not applicable to text_list") {
		t.Fatalf("expected text_list-equality error, got %v", err)
	}
}

func TestIf_PlatformIn_OK(t *testing.T) {
	val := `[{
		"#if": {
			"and": [{"@runtime.platform": {"#in": ["darwin", "linux"]}}],
			"value": {"x": 1}
		}
	}]`
	if err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestIf_Nested_OK(t *testing.T) {
	val := `[{
		"#if": {
			"and": ["@flag"],
			"value": {
				"#if": {
					"or": [{"@name": "#notEmpty"}],
					"value": {"x": 1}
				}
			}
		}
	}]`
	if err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected ok for nested #if, got %v", err)
	}
}

func TestIf_UnknownBangKey_OnlyWarns(t *testing.T) {
	// Unknown #foo control-construct → warn (not error). Forward compat.
	val := `[{"#foo": "anything", "type": "mixed"}]`
	if err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected ok for unknown #*, got %v", err)
	}
}

func TestVars_ReservedName_Runtime_LoaderError(t *testing.T) {
	vars := []TemplateVar{{Name: "runtime", Type: "text"}}
	err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("expected reserved-name error, got %v", err)
	}
}

func TestVars_FormerReservedNames_NowAllowed(t *testing.T) {
	// SPEC 067 namespace rename: runtime globals moved under @runtime.*, so
	// "platform" / "arch" are ordinary var names again — only "runtime" is reserved.
	vars := []TemplateVar{
		{Name: "platform", Type: "text"},
		{Name: "arch", Type: "text"},
	}
	if err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected ok for former-reserved platform/arch, got %v", err)
	}
}

func TestVars_ReservedName_CamelCase_OK(t *testing.T) {
	// Case-sensitive: "Runtime" is NOT reserved (strict lower-case).
	vars := []TemplateVar{{Name: "Runtime", Type: "text"}}
	if err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected ok for camelCase Runtime, got %v", err)
	}
}

// SPEC 067: #if внутри vars[].default_value — только @runtime.* globals.

func mustDefaultValue(t *testing.T, raw string) VarDefaultValue {
	t.Helper()
	var dv VarDefaultValue
	if err := json.Unmarshal([]byte(raw), &dv); err != nil {
		t.Fatalf("default_value unmarshal: %v", err)
	}
	return dv
}

func TestDefaultValueIf_RuntimeGlobal_OK(t *testing.T) {
	vars := []TemplateVar{{
		Name: "tun_stack", Type: "enum",
		DefaultValue: mustDefaultValue(t, `{"#if":{"and":[{"@runtime.platform":"windows"},{"@runtime.arch":"386"}],"value":"gvisor","else":"system"}}`),
	}}
	if err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected ok for @runtime.* default #if, got %v", err)
	}
}

func TestDefaultValueIf_PerPlatformTree_OK(t *testing.T) {
	vars := []TemplateVar{{
		Name: "tun_stack", Type: "enum",
		DefaultValue: mustDefaultValue(t, `{"default":{"#if":{"and":[{"@runtime.platform":"linux"}],"value":"gvisor","else":"system"}}}`),
	}}
	if err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected ok for per-platform default #if, got %v", err)
	}
}

func TestDefaultValueIf_UserVarRef_LoaderError(t *testing.T) {
	// Ссылка на user-var внутри default #if запрещена (только @runtime.*).
	vars := []TemplateVar{
		{Name: "flag", Type: "bool"},
		{Name: "tun_stack", Type: "enum",
			DefaultValue: mustDefaultValue(t, `{"#if":{"and":["@flag"],"value":"gvisor","else":"system"}}`)},
	}
	err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "unknown var") {
		t.Fatalf("expected user-var rejection in default #if, got %v", err)
	}
}

func TestDefaultValueIf_UnknownRuntimeGlobal_LoaderError(t *testing.T) {
	vars := []TemplateVar{{
		Name: "tun_stack", Type: "enum",
		DefaultValue: mustDefaultValue(t, `{"#if":{"and":[{"@runtime.bogus":"x"}],"value":"a","else":"b"}}`),
	}}
	err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "unknown runtime global") {
		t.Fatalf("expected unknown-runtime-global error, got %v", err)
	}
}

func TestOuterIf_BareRef_LoaderError(t *testing.T) {
	// Phase 3: bare "tun" in if[] is a loader error — canonical form is "@tun".
	vars := []TemplateVar{{Name: "tun", Type: "bool"}}
	params := []TemplateParam{
		{Name: "inbounds", If: []string{"tun"}, Value: json.RawMessage(`[]`)},
	}
	err := ValidateWizardTemplate(vars, params, json.RawMessage(`{}`))
	if err == nil {
		t.Fatalf("expected loader error for bare if[] var-ref, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bare var-ref") || !strings.Contains(msg, `"@tun"`) {
		t.Fatalf("expected error mentioning bare var-ref and canonical @tun form, got %q", msg)
	}
}

func TestOuterIf_AtPrefix_Works(t *testing.T) {
	vars := []TemplateVar{{Name: "tun", Type: "bool"}}
	params := []TemplateParam{
		{Name: "inbounds", If: []string{"@tun"}, Value: json.RawMessage(`[]`)},
	}
	if err := ValidateWizardTemplate(vars, params, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected ok for @-prefixed if[], got %v", err)
	}
}

func TestOuterIf_UnknownVar_LoaderError(t *testing.T) {
	vars := []TemplateVar{{Name: "tun", Type: "bool"}}
	params := []TemplateParam{
		{Name: "inbounds", If: []string{"@nonexistent"}, Value: json.RawMessage(`[]`)},
	}
	err := ValidateWizardTemplate(vars, params, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "unknown var") {
		t.Fatalf("expected unknown-var error, got %v", err)
	}
}

func TestOuterIf_PlatformGlobal_LoaderError(t *testing.T) {
	// Runtime globals @runtime.* live only in #if predicates,
	// never in outer if[]/if_or[]. Phase 3 rejects them at load time.
	vars := []TemplateVar{{Name: "tun", Type: "bool"}}
	params := []TemplateParam{
		{Name: "inbounds", If: []string{"@runtime.platform"}, Value: json.RawMessage(`[]`)},
	}
	err := ValidateWizardTemplate(vars, params, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "runtime global") {
		t.Fatalf("expected runtime-global error for @runtime.platform in outer if[], got %v", err)
	}
}

func TestIf_ArrayElementMode_OK(t *testing.T) {
	val := `["always", {"#if": {"and": ["@flag"], "value": "conditional"}}, "regular"]`
	if err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected ok for array-element #if, got %v", err)
	}
}

func TestIf_EmptyAndList_LoaderError(t *testing.T) {
	val := `[{"#if": {"and": [], "value": {"x": 1}}}]`
	err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "non-empty") {
		t.Fatalf("expected empty-and error, got %v", err)
	}
}

func TestIf_NotInUsingTextList_OK(t *testing.T) {
	// {"@name": {"#in": "@names"}} — text scalar in text_list var → OK.
	val := `[{"#if": {"and": [{"@name": {"#in": "@names"}}], "value": {"x": 1}}}]`
	if err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected ok for #in @text_list_var, got %v", err)
	}
}

func TestIf_NotDoubleNeg_OK(t *testing.T) {
	val := `[{"#if": {"and": [{"#not": {"#not": "@flag"}}], "value": {"x": 1}}}]`
	if err := ValidateWizardTemplate(ifValidatorVars(), ifValidatorParam(val), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected ok for double-#not, got %v", err)
	}
}
