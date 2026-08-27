package business

import (
	"testing"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 113-B (M3) — реестр исключений переписывается КАЖДОЙ сборкой, включая
// preview-сборку Мастера.
//
// Путь Мастера был третьим из трёх и единственным, кто результат
// GenerateOutboundsFromParserConfig выбрасывал: ⚠ в строке источника
// показывал итог предыдущей сборки. Пользователь чинил хоп, жал Preview,
// узлы возвращались — а пометка висела до полного Rebuild, и наоборот:
// сломанный только что хоп preview никак не отмечал.

type registryGenMock struct {
	out *config.OutboundGenerationResult
}

func (m *registryGenMock) GenerateOutboundsFromParserConfig(
	*config.ParserConfig,
	map[string]int,
	func(float64, string),
) (*config.OutboundGenerationResult, error) {
	return m.out, nil
}

func (m *registryGenMock) RefreshSingleSubscription(string) (*corestate.Source, error) {
	return nil, nil
}

func (m *registryGenMock) RefreshSourceInPlace(*corestate.Source) (bool, error) {
	return false, nil
}

const registryTestJSON = `{"ParserConfig":{"version":1,"proxies":[{"source":"https://example.com/a"}],"outbounds":[]}}`

func runPreview(t *testing.T, res *config.OutboundGenerationResult) {
	t.Helper()
	model := wizardmodels.NewWizardModel()
	model.ParserConfigJSON = registryTestJSON
	if err := ParseAndPreview(stubStaleUIUpdater{model: model}, &registryGenMock{out: res}); err != nil {
		t.Fatalf("ParseAndPreview: %v", err)
	}
}

func TestParseAndPreview_WritesExclusionRegistry(t *testing.T) {
	t.Cleanup(func() { config.SetExcludedSources(nil) })

	// Preview со сломанным хопом обязана поставить пометку.
	runPreview(t, &config.OutboundGenerationResult{
		OutboundsJSON: []string{`{"type":"direct","tag":"kept"}`},
		ExcludedSources: []config.SourceExclusion{
			{SourceID: "01SUB", SourceLabel: "Proton NL", Reason: "хоп не найден"},
		},
	})
	if got := config.ExcludedSourceReason("01SUB"); got == "" {
		t.Fatal("preview-сборка не записала исключение — строка источника не покажет ⚠")
	}

	// Чистая preview обязана пометку СНЯТЬ: реестр — итог последней сборки,
	// а не накопитель.
	runPreview(t, &config.OutboundGenerationResult{
		OutboundsJSON: []string{`{"type":"direct","tag":"kept"}`},
	})
	if got := config.ExcludedSourceReason("01SUB"); got != "" {
		t.Fatalf("пометка пережила чистую preview-сборку: %q", got)
	}
}

// Отброшенный результат (ParserConfigJSON изменился во время генерации) реестр
// трогать не имеет права: сборки, чей итог выброшен, не было.
func TestParseAndPreview_DiscardedResultLeavesRegistryAlone(t *testing.T) {
	t.Cleanup(func() { config.SetExcludedSources(nil) })

	config.SetExcludedSources([]config.SourceExclusion{
		{SourceID: "01SUB", SourceLabel: "Proton NL", Reason: "хоп не найден"},
	})

	entered := make(chan struct{})
	proceed := make(chan struct{})
	mock := &blockingGenMock{
		entered: entered,
		proceed: proceed,
		out:     &config.OutboundGenerationResult{OutboundsJSON: []string{`{"type":"direct","tag":"x"}`}},
	}

	model := wizardmodels.NewWizardModel()
	model.ParserConfigJSON = registryTestJSON
	errCh := make(chan error, 1)
	go func() { errCh <- ParseAndPreview(stubStaleUIUpdater{model: model}, mock) }()

	<-entered
	model.ParserConfigJSON = staleTestJSONB
	close(proceed)
	if err := <-errCh; err != nil {
		t.Fatalf("ParseAndPreview: %v", err)
	}

	if got := config.ExcludedSourceReason("01SUB"); got == "" {
		t.Fatal("отброшенная сборка стёрла реестр — ⚠ исчезла без причины")
	}
}
