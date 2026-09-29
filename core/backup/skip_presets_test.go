package backup

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// LxBox §578, контракт 1.1.86: skip_presets едет у своего сервера и у члена
// папки, пишется только true; импорт совпавшего по телу узла берёт true из
// файла и отсутствием поля своё true не сбрасывает.
func TestSkipPresetsRoundTrip(t *testing.T) {
	body := json.RawMessage(`{"type":"tailscale","auth_key":"k"}`)
	src := &state.State{}
	src.Sources = []state.Source{
		{ID: "01SRV0000000000000000000", Node: state.Node{Kind: state.SourceKindServer, Enabled: true, Tag: "home-ts", Body: body, SkipPresets: true}},
		{ID: "01SRV0000000000000000001", Node: state.Node{Kind: state.SourceKindServer, Enabled: true, Tag: "work-ts",
			Body: json.RawMessage(`{"type":"tailscale","auth_key":"w"}`)}},
		{ID: "01FLD0000000000000000000", Name: "F", Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
			Nodes: []state.Node{{Kind: state.SourceKindServer, Enabled: true, Tag: "m", SkipPresets: true,
				Body: json.RawMessage(`{"type":"tailscale","auth_key":"m"}`)}}},
	}
	raw := fixedExport10(t, src)
	if n := strings.Count(string(raw), `"skip_presets": true`); n != 2 {
		t.Fatalf("skip_presets: true в экспорте %d раз, ожидалось 2:\n%s", n, raw)
	}
	if strings.Contains(string(raw), `"skip_presets": false`) {
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
			Body: json.RawMessage(`{"type":"tailscale","auth_key":"w"}`), SkipPresets: true}},
	}
	if _, err := Import10(dst, &b, ImportOptions{}); err != nil {
		t.Fatalf("Import10: %v", err)
	}
	got := map[string]bool{}
	for _, s := range dst.Sources {
		if s.Kind == state.SourceKindServer {
			got[s.Tag] = s.SkipPresets
		}
		for _, n := range s.Nodes {
			got["F/"+n.Tag] = n.SkipPresets
		}
	}
	if !got["home-ts"] || !got["work-ts"] || !got["F/m"] {
		t.Errorf("skip_presets после импорта: %v", got)
	}
}
