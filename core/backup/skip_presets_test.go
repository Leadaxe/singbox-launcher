package backup

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// LxBox §578, контракт 1.1.86 (имя поля — 1.1.126): skip_tailscale_presets едет у своего сервера и у члена
// папки, пишется только true; импорт совпавшего по телу узла берёт true из
// файла и отсутствием поля своё true не сбрасывает.
func TestSkipPresetsRoundTrip(t *testing.T) {
	body := json.RawMessage(`{"type":"tailscale","auth_key":"k"}`)
	src := &state.State{}
	src.Sources = []state.Source{
		{ID: "01SRV0000000000000000000", Node: state.Node{Kind: state.SourceKindServer, Enabled: true, Tag: "home-ts", Body: body, NodeRecordFlags: configtypes.NodeRecordFlags{SkipTailscalePresets: true}}},
		{ID: "01SRV0000000000000000001", Node: state.Node{Kind: state.SourceKindServer, Enabled: true, Tag: "work-ts",
			Body: json.RawMessage(`{"type":"tailscale","auth_key":"w"}`)}},
		{ID: "01FLD0000000000000000000", Name: "F", Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
			Nodes: []state.Node{{Kind: state.SourceKindServer, Enabled: true, Tag: "m", NodeRecordFlags: configtypes.NodeRecordFlags{SkipTailscalePresets: true},
				Body: json.RawMessage(`{"type":"tailscale","auth_key":"m"}`)}}},
	}
	raw := fixedExport10(t, src)
	if n := strings.Count(string(raw), `"skip_tailscale_presets": true`); n != 2 {
		t.Fatalf("skip_tailscale_presets: true в экспорте %d раз, ожидалось 2:\n%s", n, raw)
	}
	if strings.Contains(string(raw), `"skip_tailscale_presets": false`) {
		t.Fatalf("false записан в экспорт:\n%s", raw)
	}

	var b Backup10
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	// Приёмник: тот же home-ts без поля — импорт ставит true; work-ts с true —
	// файл без поля его не сбрасывает.
	dst := &state.State{}
	dst.Sources = []state.Source{
		{ID: "01DST0000000000000000000", Node: state.Node{Kind: state.SourceKindServer, Enabled: true, Tag: "home-ts", Body: body}},
		{ID: "01DST0000000000000000001", Node: state.Node{Kind: state.SourceKindServer, Enabled: true, Tag: "work-ts",
			Body: json.RawMessage(`{"type":"tailscale","auth_key":"w"}`), NodeRecordFlags: configtypes.NodeRecordFlags{SkipTailscalePresets: true}}},
	}
	if _, err := Import10(dst, &b, ImportOptions{}); err != nil {
		t.Fatalf("Import10: %v", err)
	}
	got := map[string]bool{}
	for _, s := range dst.Sources {
		if s.Kind == state.SourceKindServer {
			got[s.Tag] = s.SkipTailscalePresets
		}
		for _, n := range s.Nodes {
			got["F/"+n.Tag] = n.SkipTailscalePresets
		}
	}
	if !got["home-ts"] || !got["work-ts"] || !got["F/m"] {
		t.Errorf("skip_tailscale_presets после импорта: %v", got)
	}
}

// Контракт 1.1.126 (SPEC 170): файл до переименования несёт `skip_presets`.
// Импорт читает его как skip_tailscale_presets — у нового корневого узла, у
// члена папки и у узла, узнанного по телу; поля allow_* едут рядом как есть.
func TestLoadLegacySkipPresetsKey(t *testing.T) {
	tsBody := json.RawMessage(`{"type":"tailscale","auth_key":"k"}`)
	src := &state.State{}
	src.Sources = []state.Source{
		{ID: "01SRV0000000000000000000", Node: state.Node{Kind: state.SourceKindServer, Enabled: true, Tag: "home-ts", Body: tsBody,
			NodeRecordFlags: configtypes.NodeRecordFlags{SkipTailscalePresets: true}}},
		{ID: "01SRV0000000000000000001", Node: state.Node{Kind: state.SourceKindServer, Enabled: true, Tag: "new-ts",
			Body:            json.RawMessage(`{"type":"tailscale","auth_key":"n"}`),
			NodeRecordFlags: configtypes.NodeRecordFlags{SkipTailscalePresets: true, AllowGateway: true}}},
		{ID: "01FLD0000000000000000000", Name: "F", Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
			Nodes: []state.Node{{Kind: state.SourceKindServer, Enabled: true, Tag: "m",
				NodeRecordFlags: configtypes.NodeRecordFlags{SkipTailscalePresets: true, AllowLan: true},
				Body:            json.RawMessage(`{"type":"tailscale","auth_key":"m"}`)}}},
	}
	raw := []byte(strings.ReplaceAll(string(fixedExport10(t, src)), `"skip_tailscale_presets"`, `"skip_presets"`))
	if n := strings.Count(string(raw), `"skip_presets": true`); n != 3 {
		t.Fatalf("легаси-файл: skip_presets: true %d раз, ожидалось 3:\n%s", n, raw)
	}
	var b Backup10
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	// Приёмник уже знает home-ts по телу, без флага.
	dst := &state.State{}
	dst.Sources = []state.Source{
		{ID: "01DST0000000000000000000", Node: state.Node{Kind: state.SourceKindServer, Enabled: true, Tag: "home-ts", Body: tsBody}},
	}
	if _, err := Import10(dst, &b, ImportOptions{}); err != nil {
		t.Fatalf("Import10: %v", err)
	}
	got := map[string]configtypes.NodeRecordFlags{}
	for _, s := range dst.Sources {
		if s.LegacySkipPresets {
			t.Errorf("%s: легаси-поле осталось в состоянии", s.Tag)
		}
		if s.Kind == state.SourceKindServer {
			got[s.Tag] = s.NodeRecordFlags
		}
		for _, n := range s.Nodes {
			if n.LegacySkipPresets {
				t.Errorf("F/%s: легаси-поле осталось в состоянии", n.Tag)
			}
			got["F/"+n.Tag] = n.NodeRecordFlags
		}
	}
	want := map[string]configtypes.NodeRecordFlags{
		"home-ts": {SkipTailscalePresets: true},
		"new-ts":  {SkipTailscalePresets: true, AllowGateway: true},
		"F/m":     {SkipTailscalePresets: true, AllowLan: true},
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s после импорта: %+v, ожидалось %+v", k, got[k], w)
		}
	}
	if out := string(fixedExport10(t, dst)); strings.Contains(out, `"skip_presets"`) {
		t.Errorf("экспорт после импорта записал легаси-ключ:\n%s", out)
	}
}
