package business

import (
	"testing"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

type stubStaleUIUpdater struct {
	model *wizardmodels.WizardModel
}

func (s stubStaleUIUpdater) Model() *wizardmodels.WizardModel { return s.model }
func (stubStaleUIUpdater) RefreshOutboundsConfiguratorList()  {}
func (stubStaleUIUpdater) UpdateTemplatePreview(string)       {}
func (stubStaleUIUpdater) UpdateSaveProgress(float64)         {}
func (stubStaleUIUpdater) UpdateSaveButtonText(string)        {}

type blockingGenMock struct {
	entered chan struct{}
	proceed chan struct{}
	out     *config.OutboundGenerationResult
}

func (m *blockingGenMock) GenerateOutboundsFromParserConfig(
	*config.ParserConfig,
	map[string]int,
	func(float64, string),
) (*config.OutboundGenerationResult, error) {
	if m.entered != nil {
		m.entered <- struct{}{}
	}
	<-m.proceed
	return m.out, nil
}

// RefreshSingleSubscription — no-op stub (тесты parser_stale не используют per-source refresh).
func (m *blockingGenMock) RefreshSingleSubscription(string) (*corestate.Source, error) {
	return nil, nil
}

// RefreshSourceInPlace — no-op stub (тесты parser_stale не используют per-source refresh).
func (m *blockingGenMock) RefreshSourceInPlace(*corestate.Source) (bool, error) {
	return false, nil
}

// newStaleTestModel — модель с одним источником-подпиской: минимум, при
// котором ParseAndPreview доходит до генерации.
func newStaleTestModel() *wizardmodels.WizardModel {
	model := wizardmodels.NewWizardModel()
	model.Sources = append(model.Sources, corestate.Source{
		ID:   corestate.MakeULID(),
		Node: corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: true},
		URL:  "https://example.com/a",
	})
	model.BumpRevision()
	return model
}

// SPEC 117 (сценарий C5): генерация, начатая на ревизии R, при любой мутации
// модели (ревизия R+1) до завершения выбрасывает результат.
func TestParseAndPreview_DiscardsWhenModelMutatesDuringGeneration(t *testing.T) {
	entered := make(chan struct{})
	proceed := make(chan struct{})
	mock := &blockingGenMock{
		entered: entered,
		proceed: proceed,
		out: &config.OutboundGenerationResult{
			OutboundsJSON: []string{`{"type":"direct","tag":"from-snapshot"}`},
		},
	}

	model := newStaleTestModel()
	up := stubStaleUIUpdater{model: model}

	errCh := make(chan error, 1)
	go func() {
		errCh <- ParseAndPreview(up, mock)
	}()

	<-entered
	// Любая canonical-мутация поднимает ревизию — этого достаточно, чтобы
	// результат стартовавшей раньше генерации протух.
	model.BumpRevision()
	close(proceed)

	if err := <-errCh; err != nil {
		t.Fatalf("ParseAndPreview: %v", err)
	}
	if len(model.GeneratedOutbounds) != 0 {
		t.Fatalf("expected empty GeneratedOutbounds, got %#v", model.GeneratedOutbounds)
	}
	if !model.PreviewNeedsParse {
		t.Fatal("expected PreviewNeedsParse after stale discard")
	}
	if model.BuildReportGen != 0 {
		t.Fatalf("expected BuildReportGen reset to 0 after discard, got %v", model.BuildReportGen)
	}
}

func TestParseAndPreview_AppliesWhenModelUnchangedDuringGeneration(t *testing.T) {
	entered := make(chan struct{})
	proceed := make(chan struct{})
	wantLine := `{"type":"direct","tag":"kept"}`
	mock := &blockingGenMock{
		entered: entered,
		proceed: proceed,
		out: &config.OutboundGenerationResult{
			OutboundsJSON: []string{wantLine},
		},
	}

	model := newStaleTestModel()
	up := stubStaleUIUpdater{model: model}

	errCh := make(chan error, 1)
	go func() {
		errCh <- ParseAndPreview(up, mock)
	}()

	<-entered
	close(proceed)

	if err := <-errCh; err != nil {
		t.Fatalf("ParseAndPreview: %v", err)
	}
	if len(model.GeneratedOutbounds) != 1 || model.GeneratedOutbounds[0] != wantLine {
		t.Fatalf("expected applied outbounds, got %#v", model.GeneratedOutbounds)
	}
	if model.PreviewNeedsParse {
		t.Fatal("did not expect PreviewNeedsParse after successful apply")
	}
}
