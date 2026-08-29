package business

// SPEC 117 §5.C — сценарий C6: гейты «нечего собирать» срабатывают по
// canonical `len(model.Sources) == 0`, а не по пустоте строкового кэша
// (которого больше нет). Проверяются business-гейты:
//   - buildConfigWithExclusions (create_config.go) — сборка/превью;
//   - ParseAndPreview (parser.go) — парсерная стадия.
// Гейты presentation (TriggerParseForPreview, validateSaveInput) построены
// на том же выражении len(model.Sources) == 0 и требуют живых виджетов —
// они этим тестом не дублируются.

import (
	"strings"
	"testing"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

func TestEmptyModelGate_BuildPreviewConfig(t *testing.T) {
	execDir := findProjectRoot(t)
	td, err := wizardtemplate.LoadTemplateData(execDir)
	if err != nil {
		t.Fatalf("load template: %v", err)
	}

	model := wizardmodels.NewWizardModel()
	model.TemplateData = td
	model.ExecDir = execDir
	model.RulesLibraryMerged = true
	ApplyWizardDNSTemplate(model)
	ApplyDNSVarsFromSettingsToModel(model)

	// Пустая canonical-модель — гейт обязан отказать.
	if _, err := BuildPreviewConfig(model); err == nil {
		t.Fatal("пустая модель: превью обязано отказать по len(Sources) == 0")
	}

	// SourceURLs — поле ввода, не источник: гейт не должен на него смотреть.
	model.SourceURLs = "https://example.com/sub"
	if _, err := BuildPreviewConfig(model); err == nil {
		t.Fatal("текст в поле ввода не источник: гейт обязан отказать")
	}

	// Один canonical-источник открывает сборку.
	model.Sources = append(model.Sources, wizardmodels.Source{
		ID:      "01C6SUB00000000000000000",
		Type:    wizardmodels.SourceTypeSubscription,
		Enabled: true,
		URL:     "https://example.com/sub",
	})
	if _, err := BuildPreviewConfig(model); err != nil {
		t.Fatalf("модель с источником: превью обязано собраться, получено: %v", err)
	}
}

func TestEmptyModelGate_ParseAndPreview(t *testing.T) {
	model := wizardmodels.NewWizardModel()
	ctx := stubStaleUIUpdater{model: model}

	// Пустая модель: гейт отрабатывает до обращения к генератору —
	// nil ConfigService это фиксирует (вызов уронил бы тест паникой).
	err := ParseAndPreview(ctx, nil)
	if err == nil {
		t.Fatal("пустая модель: ParseAndPreview обязан отказать")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("ожидался отказ гейта пустоты, получено: %v", err)
	}

	// Наличие источника выводит из гейта (дальше падение уже не про пустоту).
	model.Sources = append(model.Sources, corestate.Source{
		ID:      "01C6SRV00000000000000000",
		Type:    corestate.SourceTypeServer,
		Enabled: true,
		Label:   "srv",
		URI:     "vless://uuid@host:443",
	})
	model.BumpRevision()
	gen := &blockingGenMock{proceed: make(chan struct{}), out: &config.OutboundGenerationResult{}}
	close(gen.proceed)
	if err := ParseAndPreview(ctx, gen); err != nil && strings.Contains(err.Error(), "empty") {
		t.Fatalf("модель с источником не должна упираться в гейт пустоты: %v", err)
	}
}
