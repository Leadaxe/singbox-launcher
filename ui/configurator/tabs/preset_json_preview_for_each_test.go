package tabs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Вкладка JSON пресета с `for_each` (SPEC 145 §7): узлы есть — раскрытие по
// отобранным узлам; узлов нет — одна строка-пояснение вместо пустых фрагментов.
func TestBuildPresetJSONPreview_ForEachNodes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "bin", "wizard_template.json"))
	if err != nil {
		t.Skipf("боевой шаблон недоступен: %v", err)
	}
	td, err := wizardtemplate.ParseTemplateData(raw)
	if err != nil {
		t.Fatalf("шаблон не загрузился: %v", err)
	}
	var ts *wizardtemplate.Preset
	for i := range td.Presets {
		if td.Presets[i].ID == "tailscale" {
			ts = &td.Presets[i]
		}
	}
	if ts == nil || ts.ForEach == nil {
		t.Fatal("пресета tailscale с for_each нет в шаблоне")
	}
	prev := config.TailscaleStateDirRoot()
	config.SetTailscaleStateDirRoot(t.TempDir())
	t.Cleanup(func() { config.SetTailscaleStateDirRoot(prev) })

	m := wizardmodels.NewWizardModel()
	m.TemplateData = td

	t.Run("no nodes", func(t *testing.T) {
		got := buildPresetJSONPreview(m, ts, map[string]string{})
		if got != "// "+locale.T("No matching nodes.") {
			t.Fatalf("без узлов ждали строку-пояснение, получили:\n%s", got)
		}
	})

	t.Run("with node", func(t *testing.T) {
		m.Sources = []corestate.Source{{ID: corestate.MakeULID(), Node: corestate.Node{
			Kind: corestate.SourceKindServer, Enabled: true, Tag: "ts-own",
			Body: json.RawMessage(`{"type":"tailscale","tag":"ts-own","auth_key":"tskey-auth-example"}`),
		}}}
		res, err := config.GenerateOutboundsFromParserConfig(m.AsParserConfig(), map[string]int{}, nil,
			config.DirectionBuildOptions{BlockTag: "block-out", DirectTag: "direct-out"})
		if err != nil {
			t.Fatalf("эмиссия: %v", err)
		}
		m.GeneratedOutbounds = res.OutboundsJSON
		m.GeneratedEndpoints = res.EndpointsJSON
		m.GeneratedSkipPresets = res.SkipPresetsTags

		got := buildPresetJSONPreview(m, ts, map[string]string{})
		if strings.Contains(got, locale.T("No matching nodes.")) || strings.HasPrefix(got, "// preset expansion failed") {
			t.Fatalf("с узлом ждали раскрытие, получили:\n%s", got)
		}
		if !strings.Contains(got, "ts-own") || got == "{}" {
			t.Fatalf("раскрытие не несёт узел ts-own:\n%s", got)
		}
	})
}
