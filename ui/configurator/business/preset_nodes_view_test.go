package business

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 145 §7 (LxBox §578): визард отбирает узлы для пресета с for_each тем
// же путём, что сборка. Из одной эмиссии: кэш боевой сборки (как его строит
// rebuild_snapshot) и кэш превью визарда дают один список, и пресет tailscale
// обслуживает включённый свой сервер и узел подписки, но не узел со
// skip_presets и не выключенный узел. Узел подписки со skip_presets в
// состоянии всё равно обслуживается: у него нет записи.
func TestPresetNodesForView_MatchesBuildSelection(t *testing.T) {
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

	tsNode := func(tag string, enabled, skip bool) corestate.Node {
		return corestate.Node{
			Kind: corestate.SourceKindServer, Enabled: enabled, Tag: tag,
			Body:        json.RawMessage(`{"type":"tailscale","tag":"` + tag + `","auth_key":"tskey-auth-example"}`),
			SkipPresets: skip,
		}
	}
	m := wizardmodels.NewWizardModel()
	m.TemplateData = td
	m.Sources = []corestate.Source{
		{ID: corestate.MakeULID(), Node: tsNode("ts-own", true, false)},
		{ID: corestate.MakeULID(), Node: tsNode("ts-skip", true, true)},
		{ID: corestate.MakeULID(), Node: tsNode("ts-off", false, false)},
		{
			ID:    corestate.MakeULID(),
			Node:  corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: true},
			URL:   "https://example.com/sub",
			Nodes: []corestate.Node{tsNode("ts-sub", true, true)},
		},
	}

	res, err := config.GenerateOutboundsFromParserConfig(m.AsParserConfig(), map[string]int{}, nil,
		config.DirectionBuildOptions{BlockTag: "block-out", DirectTag: "direct-out"})
	if err != nil {
		t.Fatalf("эмиссия: %v", err)
	}
	// Превью: поля модели, как их заполняет ParseAndPreview.
	m.GeneratedOutbounds = res.OutboundsJSON
	m.GeneratedEndpoints = res.EndpointsJSON
	m.GeneratedSkipPresets = res.SkipPresetsTags

	// Сборка: кэш, как его строит rebuild_snapshot.
	toRaw := func(in []string) []json.RawMessage {
		out := make([]json.RawMessage, 0, len(in))
		for _, s := range in {
			out = append(out, json.RawMessage(strings.TrimSpace(strings.TrimRight(s, ",\n\r\t "))))
		}
		return out
	}
	buildCache := &build.ParsedCache{
		Outbounds:   toRaw(res.OutboundsJSON),
		Endpoints:   toRaw(res.EndpointsJSON),
		SkipPresets: res.SkipPresetsTags,
	}
	buildNodes := build.PresetForEachNodes(ts, nil, PresetGlobalVars(m), m.Target,
		build.CollectPresetNodes(buildCache, td.ConfigOrder))
	var buildTags []string
	for _, n := range buildNodes {
		buildTags = append(buildTags, n.Tag)
	}

	viewTags := PresetServedTags(m, ts, nil)
	if !reflect.DeepEqual(viewTags, buildTags) {
		t.Fatalf("визард %v, сборка %v", viewTags, buildTags)
	}
	joined := strings.Join(viewTags, " ")
	if len(viewTags) != 2 || !strings.Contains(joined, "ts-own") || !strings.Contains(joined, "ts-sub") {
		t.Errorf("обслуживаемые узлы %v, ждали ts-own и узел подписки ts-sub", viewTags)
	}
	for _, bad := range []string{"ts-skip", "ts-off"} {
		if strings.Contains(joined, bad) {
			t.Errorf("пресет обслуживает %s: %v", bad, viewTags)
		}
	}

	// DNS-серверы пресета в визарде — по тем же узлам.
	frags, _, ok := ExpandPresetForView(m, ts, nil)
	if !ok || len(frags.DNSServers) != 2 {
		t.Fatalf("DNS-серверы пресета в визарде: %+v", frags)
	}
}

// Подпись строки пресета с for_each: ноль, один, два узла.
func TestPresetServedNodesLabel(t *testing.T) {
	if got, want := PresetServedNodesLabel(nil), locale.T("No matching nodes"); got != want {
		t.Errorf("0 узлов: %q, ждали %q", got, want)
	}
	if got := PresetServedNodesLabel([]string{"home-ts"}); got != "home-ts" {
		t.Errorf("1 узел: %q", got)
	}
	if got := PresetServedNodesLabel([]string{"home-ts", "work-ts"}); got != "home-ts, work-ts" {
		t.Errorf("2 узла: %q", got)
	}
}
