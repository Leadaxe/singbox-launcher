package business

import (
	"encoding/json"
	"strings"
	"testing"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 097: превью обязано рендерить конфиг ТАРГЕТА модели. Регрессия, от
// которой этот тест защищает: BuildPreviewConfig собирал BuildContext без
// Target — remote-модель показывала в превью local-конфиг (clash_api на
// месте, find_process=true), а на роутер уехало бы другое.
func TestBuildPreviewConfig_RespectsRemoteTarget(t *testing.T) {
	execDir := findProjectRoot(t)
	templateData, err := wizardtemplate.LoadTemplateData(execDir)
	if err != nil {
		t.Fatalf("load template: %v", err)
	}

	newModel := func(target wizardtemplate.TargetSpec) *wizardmodels.WizardModel {
		model := wizardmodels.NewWizardModel()
		model.TemplateData = templateData
		model.ExecDir = execDir
		model.ParserConfigJSON = strings.TrimSpace(templateData.ParserConfig)
		model.RulesLibraryMerged = true
		model.Target = target
		ApplyWizardDNSTemplate(model)
		ApplyDNSVarsFromSettingsToModel(model)
		return model
	}

	parsePreview := func(t *testing.T, text string) map[string]interface{} {
		t.Helper()
		// Preview — JSONC с маркер-комментариями; для проверки секций
		// вырезаем строки-комментарии (в тесте это достаточно: маркеры
		// живут на отдельных строках).
		var b strings.Builder
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") || strings.HasPrefix(strings.TrimSpace(line), "/*") {
				continue
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(b.String()), &m); err != nil {
			t.Fatalf("preview is not parseable after comment strip: %v", err)
		}
		return m
	}

	local := newModel(wizardtemplate.LocalTarget())
	localText, err := BuildPreviewConfig(local)
	if err != nil {
		t.Fatalf("local preview: %v", err)
	}
	localCfg := parsePreview(t, localText)
	if _, ok := localCfg["experimental"].(map[string]interface{})["clash_api"]; !ok {
		t.Errorf("local preview must keep clash_api")
	}

	remote := newModel(wizardtemplate.TargetSpec{
		GOOS: "linux", GOARCH: "arm64", Target: constants.ConfigTargetRemote,
	})
	remoteText, err := BuildPreviewConfig(remote)
	if err != nil {
		t.Fatalf("remote preview: %v", err)
	}
	remoteCfg := parsePreview(t, remoteText)
	if exp, ok := remoteCfg["experimental"].(map[string]interface{}); ok {
		if _, has := exp["clash_api"]; has {
			t.Errorf("remote preview must NOT contain clash_api")
		}
	}
	// find_process ветвится по РОЛИ, не по таргету: remote-сервер сохраняет
	// true, gateway выключает.
	if route, ok := remoteCfg["route"].(map[string]interface{}); ok {
		if route["find_process"] != true {
			t.Errorf("remote (non-gateway) preview find_process: want true, got %#v", route["find_process"])
		}
	}

	gwModel := newModel(wizardtemplate.TargetSpec{
		GOOS: "linux", GOARCH: "arm64", Target: constants.ConfigTargetRemote,
	})
	gwModel.SettingsVars["gateway_mode"] = "true"
	gwText, err := BuildPreviewConfig(gwModel)
	if err != nil {
		t.Fatalf("gateway preview: %v", err)
	}
	gwCfg := parsePreview(t, gwText)
	if route, ok := gwCfg["route"].(map[string]interface{}); ok {
		if route["find_process"] != false {
			t.Errorf("gateway preview find_process: want false, got %#v", route["find_process"])
		}
	}
}
