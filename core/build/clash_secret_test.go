package build

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/template"
)

// TestMaterializeClashSecretInVars_NilNoop — nil-аргументы не валят.
func TestMaterializeClashSecretInVars_NilNoop(t *testing.T) {
	MaterializeClashSecretInVars(nil, nil)
	MaterializeClashSecretInVars(nil, map[string]string{"x": "1"})
	MaterializeClashSecretInVars(&template.TemplateData{}, nil)
}

// TestMaterializeClashSecretInVars_NoTemplateVarNoop — если в шаблоне
// нет var с именем clash_secret, ничего не делаем.
func TestMaterializeClashSecretInVars_NoTemplateVarNoop(t *testing.T) {
	td := &template.TemplateData{
		Vars:        []template.TemplateVar{{Name: "log_level"}},
		RawTemplate: json.RawMessage(`{}`),
	}
	vars := map[string]string{}
	MaterializeClashSecretInVars(td, vars)
	if _, ok := vars["clash_secret"]; ok {
		t.Errorf("clash_secret must NOT be added when template has no var: %v", vars)
	}
}

// TestMaterializeClashSecretInVars_GeneratesWhenMissing — есть template-var,
// но в vars пусто → генерируется секрет.
func TestMaterializeClashSecretInVars_GeneratesWhenMissing(t *testing.T) {
	td := &template.TemplateData{
		Vars:        []template.TemplateVar{{Name: "clash_secret", Type: "text"}},
		RawTemplate: json.RawMessage(`{}`),
	}
	vars := map[string]string{}
	MaterializeClashSecretInVars(td, vars)
	got, ok := vars["clash_secret"]
	if !ok {
		t.Fatalf("clash_secret must be generated, got: %v", vars)
	}
	if len(got) < 8 {
		t.Errorf("generated secret should be reasonably long, got: %q", got)
	}
}

// TestMaterializeClashSecretInVars_PreservesExisting — если уже разрешённое
// значение есть, не переписываем.
func TestMaterializeClashSecretInVars_PreservesExisting(t *testing.T) {
	td := &template.TemplateData{
		Vars:        []template.TemplateVar{{Name: "clash_secret", Type: "text"}},
		RawTemplate: json.RawMessage(`{}`),
	}
	vars := map[string]string{"clash_secret": "my-existing-secret-12345"}
	MaterializeClashSecretInVars(td, vars)
	if got := vars["clash_secret"]; got != "my-existing-secret-12345" {
		t.Errorf("existing secret must be preserved, got: %q", got)
	}
}

// TestMaterializeClashSecretInVars_OverwritesUnresolvedPlaceholder — если в
// vars лежит unresolved-плейсхолдер вида CHANGE_THIS_*, генерируется новый.
// Это `template.ClashSecretUnresolved`-критерий: пусто/whitespace или
// CHANGE_THIS_*-префикс. Произвольная строка (включая `@clash_secret`) не
// считается unresolved и не перезаписывается — это by design, чтобы не терять
// уже установленный пользовательский секрет.
func TestMaterializeClashSecretInVars_OverwritesUnresolvedPlaceholder(t *testing.T) {
	td := &template.TemplateData{
		Vars:        []template.TemplateVar{{Name: "clash_secret", Type: "text"}},
		RawTemplate: json.RawMessage(`{}`),
	}
	vars := map[string]string{"clash_secret": "CHANGE_THIS_PLACEHOLDER"} // unresolved
	MaterializeClashSecretInVars(td, vars)
	got := vars["clash_secret"]
	if got == "CHANGE_THIS_PLACEHOLDER" || got == "" {
		t.Errorf("unresolved CHANGE_THIS_* must be replaced; got: %q", got)
	}
}

// TestMaterializeClashSecretInVars_PreservesArbitraryString — произвольное
// (не-CHANGE_THIS_*) значение НЕ перезаписывается, даже если оно похоже на
// плейсхолдер. Это защищает от потери пользовательского секрета.
func TestMaterializeClashSecretInVars_PreservesArbitraryString(t *testing.T) {
	td := &template.TemplateData{
		Vars:        []template.TemplateVar{{Name: "clash_secret", Type: "text"}},
		RawTemplate: json.RawMessage(`{}`),
	}
	vars := map[string]string{"clash_secret": "@clash_secret"}
	MaterializeClashSecretInVars(td, vars)
	if got := vars["clash_secret"]; got != "@clash_secret" {
		t.Errorf("arbitrary value must be preserved; got: %q", got)
	}
}
