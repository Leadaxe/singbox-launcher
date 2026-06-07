package build

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/template"
)

// TestMaterializeSecretsInVars_NilNoop — nil-аргументы не валят.
func TestMaterializeSecretsInVars_NilNoop(t *testing.T) {
	MaterializeSecretsInVars(nil, nil)
	MaterializeSecretsInVars(nil, map[string]string{"x": "1"})
	MaterializeSecretsInVars(&template.TemplateData{}, nil)
}

// TestMaterializeSecretsInVars_NoTemplateVarNoop — если в шаблоне
// нет ни одной type:"secret" var, ничего не делаем.
func TestMaterializeSecretsInVars_NoTemplateVarNoop(t *testing.T) {
	td := &template.TemplateData{
		Vars:        []template.TemplateVar{{Name: "log_level"}},
		RawTemplate: json.RawMessage(`{}`),
	}
	vars := map[string]string{}
	MaterializeSecretsInVars(td, vars)
	if _, ok := vars["clash_secret"]; ok {
		t.Errorf("clash_secret must NOT be added when template has no var: %v", vars)
	}
}

// TestMaterializeSecretsInVars_GeneratesWhenMissing — есть template-var,
// но в vars пусто → генерируется секрет.
func TestMaterializeSecretsInVars_GeneratesWhenMissing(t *testing.T) {
	td := &template.TemplateData{
		Vars:        []template.TemplateVar{{Name: "clash_secret", Type: "secret"}},
		RawTemplate: json.RawMessage(`{}`),
	}
	vars := map[string]string{}
	MaterializeSecretsInVars(td, vars)
	got, ok := vars["clash_secret"]
	if !ok {
		t.Fatalf("clash_secret must be generated, got: %v", vars)
	}
	if len(got) < 8 {
		t.Errorf("generated secret should be reasonably long, got: %q", got)
	}
}

// TestMaterializeSecretsInVars_PreservesExisting — если уже разрешённое
// значение есть, не переписываем.
func TestMaterializeSecretsInVars_PreservesExisting(t *testing.T) {
	td := &template.TemplateData{
		Vars:        []template.TemplateVar{{Name: "clash_secret", Type: "secret"}},
		RawTemplate: json.RawMessage(`{}`),
	}
	vars := map[string]string{"clash_secret": "my-existing-secret-12345"}
	MaterializeSecretsInVars(td, vars)
	if got := vars["clash_secret"]; got != "my-existing-secret-12345" {
		t.Errorf("existing secret must be preserved, got: %q", got)
	}
}

// TestMaterializeSecretsInVars_OverwritesUnresolvedPlaceholder — если в
// vars лежит unresolved-плейсхолдер вида CHANGE_THIS_*, генерируется новый.
// Это `template.SecretUnresolved`-критерий: пусто/whitespace или
// CHANGE_THIS_*-префикс. Произвольная строка (включая `@clash_secret`) не
// считается unresolved и не перезаписывается — это by design, чтобы не терять
// уже установленный пользовательский секрет.
func TestMaterializeSecretsInVars_OverwritesUnresolvedPlaceholder(t *testing.T) {
	td := &template.TemplateData{
		Vars:        []template.TemplateVar{{Name: "clash_secret", Type: "secret"}},
		RawTemplate: json.RawMessage(`{}`),
	}
	vars := map[string]string{"clash_secret": "CHANGE_THIS_PLACEHOLDER"} // unresolved
	MaterializeSecretsInVars(td, vars)
	got := vars["clash_secret"]
	if got == "CHANGE_THIS_PLACEHOLDER" || got == "" {
		t.Errorf("unresolved CHANGE_THIS_* must be replaced; got: %q", got)
	}
}

// TestMaterializeSecretsInVars_PreservesArbitraryString — произвольное
// (не-CHANGE_THIS_*) значение НЕ перезаписывается, даже если оно похоже на
// плейсхолдер. Это защищает от потери пользовательского секрета.
func TestMaterializeSecretsInVars_PreservesArbitraryString(t *testing.T) {
	td := &template.TemplateData{
		Vars:        []template.TemplateVar{{Name: "clash_secret", Type: "secret"}},
		RawTemplate: json.RawMessage(`{}`),
	}
	vars := map[string]string{"clash_secret": "@clash_secret"}
	MaterializeSecretsInVars(td, vars)
	if got := vars["clash_secret"]; got != "@clash_secret" {
		t.Errorf("arbitrary value must be preserved; got: %q", got)
	}
}
