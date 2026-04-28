package template

import (
	"encoding/json"
	"testing"
)

func TestValidateWizardTemplate_ok(t *testing.T) {
	vars := []TemplateVar{
		{Name: "tun", Type: "bool"},
		{Name: "x", Type: "text"},
	}
	params := []TemplateParam{
		{Name: "inbounds", Platforms: []string{"darwin"}, If: []string{"tun"}, Value: json.RawMessage(`[{"listen_port":"@x"}]`)},
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
		{If: []string{"a"}, IfOr: []string{"b"}, Value: json.RawMessage(`[]`)},
	}
	err := ValidateWizardTemplate(vars, params, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateWizardTemplate_ifOrNotBool(t *testing.T) {
	vars := []TemplateVar{{Name: "tun", Type: "text"}}
	params := []TemplateParam{
		{IfOr: []string{"tun"}, Value: json.RawMessage(`[]`)},
	}
	err := ValidateWizardTemplate(vars, params, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateWizardTemplate_ifNotBool(t *testing.T) {
	vars := []TemplateVar{{Name: "tun", Type: "text"}}
	params := []TemplateParam{
		{If: []string{"tun"}, Value: json.RawMessage(`[]`)},
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
		{Name: "b", Type: "bool", If: []string{"a"}, IfOr: []string{"a"}},
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
		{Name: "y", Type: "text", If: []string{"x"}},
	}
	err := ValidateWizardTemplate(vars, nil, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}
