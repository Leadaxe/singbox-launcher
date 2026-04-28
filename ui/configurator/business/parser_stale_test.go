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

func (s stubStaleUIUpdater) Model() *wizardmodels.WizardModel         { return s.model }
func (stubStaleUIUpdater) UpdateParserConfig(string)                  {}
func (stubStaleUIUpdater) UpdateTemplatePreview(string)               {}
func (stubStaleUIUpdater) UpdateSaveProgress(float64)                 {}
func (stubStaleUIUpdater) UpdateSaveButtonText(string)                {}

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

const staleTestJSONA = `{"ParserConfig":{"version":1,"proxies":[{"source":"https://example.com/a"}],"outbounds":[]}}`
const staleTestJSONB = `{"ParserConfig":{"version":1,"proxies":[{"source":"https://example.org/b"}],"outbounds":[]}}`

func TestParseAndPreview_DiscardsWhenJSONChangesDuringGeneration(t *testing.T) {
	entered := make(chan struct{})
	proceed := make(chan struct{})
	mock := &blockingGenMock{
		entered: entered,
		proceed: proceed,
		out: &config.OutboundGenerationResult{
			OutboundsJSON: []string{`{"type":"direct","tag":"from-snapshot"}`},
		},
	}

	model := wizardmodels.NewWizardModel()
	model.ParserConfigJSON = staleTestJSONA
	up := stubStaleUIUpdater{model: model}

	errCh := make(chan error, 1)
	go func() {
		errCh <- ParseAndPreview(up, mock)
	}()

	<-entered
	model.ParserConfigJSON = staleTestJSONB
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
}

func TestParseAndPreview_AppliesWhenJSONUnchangedDuringGeneration(t *testing.T) {
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

	model := wizardmodels.NewWizardModel()
	model.ParserConfigJSON = staleTestJSONA
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
