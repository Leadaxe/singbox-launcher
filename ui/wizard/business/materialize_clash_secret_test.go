package business

import (
	"encoding/json"
	"testing"

	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

func TestMaterializeClashSecretIfNeeded_idempotent(t *testing.T) {
	old := wizardtemplate.ClashSecretReader
	defer func() { wizardtemplate.ClashSecretReader = old }()
	wizardtemplate.ClashSecretReader = fixedReaderForClashTest()

	rawFull := json.RawMessage(`{"vars":[{"name":"clash_secret","type":"secret","default_value":"CHANGE_THIS_X"}]}`)
	td := &wizardtemplate.TemplateData{
		Vars: []wizardtemplate.TemplateVar{
			{Name: "clash_secret", Type: "secret", DefaultValue: wizardtemplate.VarDefaultValue{Scalar: "CHANGE_THIS_X"}},
		},
		RawTemplate: rawFull,
	}
	m := &wizardmodels.WizardModel{
		TemplateData: td,
		SettingsVars: make(map[string]string),
	}
	MaterializeClashSecretIfNeeded(m)
	s1 := m.SettingsVars["clash_secret"]
	if len(s1) != 16 {
		t.Fatalf("len %d", len(s1))
	}
	MaterializeClashSecretIfNeeded(m)
	if m.SettingsVars["clash_secret"] != s1 {
		t.Fatalf("secret changed on second call")
	}
}

func TestMaterializeClashSecretIfNeeded_placeholderKeyStabilizes(t *testing.T) {
	old := wizardtemplate.ClashSecretReader
	defer func() { wizardtemplate.ClashSecretReader = old }()
	wizardtemplate.ClashSecretReader = fixedReaderForClashTest()

	rawFull := json.RawMessage(`{"vars":[{"name":"clash_secret","type":"secret","default_value":"CHANGE_THIS_X"}]}`)
	td := &wizardtemplate.TemplateData{
		Vars: []wizardtemplate.TemplateVar{
			{Name: "clash_secret", Type: "secret", DefaultValue: wizardtemplate.VarDefaultValue{Scalar: "CHANGE_THIS_X"}},
		},
		RawTemplate: rawFull,
	}
	m := &wizardmodels.WizardModel{
		TemplateData: td,
		SettingsVars: map[string]string{"clash_secret": "CHANGE_THIS_OLD"},
	}
	MaterializeClashSecretIfNeeded(m)
	s1 := m.SettingsVars["clash_secret"]
	if len(s1) != 16 {
		t.Fatalf("len %d", len(s1))
	}
	MaterializeClashSecretIfNeeded(m)
	if m.SettingsVars["clash_secret"] != s1 {
		t.Fatalf("secret changed when key held placeholder")
	}
}

type repeatByteReader byte

func (b repeatByteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(b)
	}
	return len(p), nil
}

func fixedReaderForClashTest() repeatByteReader {
	return repeatByteReader(0)
}
