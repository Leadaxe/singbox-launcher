package build

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildOutboundsSection_SaveEmptyMarkers — Save (forPreview=false) и пустой
// шаблон → секция ровно с маркерами, без статики, без динамики между.
func TestBuildOutboundsSection_SaveEmptyMarkers(t *testing.T) {
	got, err := BuildOutboundsSection(json.RawMessage(`[]`), nil, false, PreviewStats{})
	if err != nil {
		t.Fatalf("BuildOutboundsSection: %v", err)
	}
	// END marker без leading indent — legacy quirk PopulateParserMarkers.
	wantPrefix := "[\n    /** @ParserSTART */\n/** @ParserEND */\n  ]"
	if got != wantPrefix {
		t.Fatalf("output mismatch:\n--- want ---\n%s\n--- got ---\n%s", wantPrefix, got)
	}
}

// TestBuildOutboundsSection_SaveStaticOnly — Save с одной статической нодой
// из шаблона → static вставляется ПОСЛЕ @ParserEND.
func TestBuildOutboundsSection_SaveStaticOnly(t *testing.T) {
	tmpl := json.RawMessage(`[{"type":"direct","tag":"direct"}]`)
	got, err := BuildOutboundsSection(tmpl, nil, false, PreviewStats{})
	if err != nil {
		t.Fatalf("BuildOutboundsSection: %v", err)
	}
	if !strings.Contains(got, "/** @ParserSTART */") || !strings.Contains(got, "/** @ParserEND */") {
		t.Fatalf("markers missing: %s", got)
	}
	if !strings.Contains(got, `{"type":"direct","tag":"direct"}`) {
		t.Fatalf("static outbound missing: %s", got)
	}
	// static идёт после @ParserEND.
	endIdx := strings.Index(got, "/** @ParserEND */")
	staticIdx := strings.Index(got, `"direct"`)
	if endIdx < 0 || staticIdx < endIdx {
		t.Fatalf("static must follow @ParserEND, got: %s", got)
	}
}

// TestBuildOutboundsSection_PreviewWithGenerated — preview=true и небольшое
// число нод → они вставляются между маркерами.
func TestBuildOutboundsSection_PreviewWithGenerated(t *testing.T) {
	gens := []string{
		`{"type":"vless","tag":"node-1"}`,
		`{"type":"trojan","tag":"node-2"}`,
	}
	got, err := BuildOutboundsSection(json.RawMessage(`[]`), gens, true, PreviewStats{NodesCount: 2})
	if err != nil {
		t.Fatalf("BuildOutboundsSection: %v", err)
	}
	for _, g := range gens {
		if !strings.Contains(got, g) {
			t.Errorf("generated entry missing: %q\nfull: %s", g, got)
		}
	}
}

// TestBuildOutboundsSection_PreviewLargeIsTruncated — preview > порога
// заменяется на сводный комментарий (без поэлементного вывода).
func TestBuildOutboundsSection_PreviewLargeIsTruncated(t *testing.T) {
	gens := make([]string, 100)
	for i := range gens {
		gens[i] = `{"type":"vless"}`
	}
	stats := PreviewStats{NodesCount: 4000, LocalSelectorsCount: 5, GlobalSelectorsCount: 10}
	got, err := BuildOutboundsSection(json.RawMessage(`[]`), gens, true, stats)
	if err != nil {
		t.Fatalf("BuildOutboundsSection: %v", err)
	}
	// должна быть text-сводка, а не поэлементные ноды
	if !strings.Contains(got, "Generated: 4000 nodes") {
		t.Errorf("preview truncation summary missing: %s", got)
	}
	if !strings.Contains(got, "Total outbounds: 100") {
		t.Errorf("total-outbounds line missing: %s", got)
	}
	// и не должно быть поэлементного вывода
	if strings.Count(got, `"type":"vless"`) > 0 {
		t.Errorf("preview should NOT inline nodes when over threshold: %s", got)
	}
}

// TestBuildOutboundsSection_PreviewWithStatic — preview с одновременно
// generated + static; запятая между ними.
func TestBuildOutboundsSection_PreviewWithStatic(t *testing.T) {
	gens := []string{`{"type":"vless","tag":"v1"}`}
	tmpl := json.RawMessage(`[{"type":"direct","tag":"direct"}]`)
	got, err := BuildOutboundsSection(tmpl, gens, true, PreviewStats{NodesCount: 1})
	if err != nil {
		t.Fatalf("BuildOutboundsSection: %v", err)
	}
	if !strings.Contains(got, `"v1"`) || !strings.Contains(got, `"direct"`) {
		t.Errorf("expected both v1 and direct: %s", got)
	}
}

// TestBuildEndpointsSection_SaveEmptyMarkers — пустой save для endpoints.
func TestBuildEndpointsSection_SaveEmptyMarkers(t *testing.T) {
	got, err := BuildEndpointsSection(json.RawMessage(`[]`), nil, false, PreviewStats{})
	if err != nil {
		t.Fatalf("BuildEndpointsSection: %v", err)
	}
	wantPrefix := "[\n    /** @ParserSTART_E */\n/** @ParserEND_E */\n  ]"
	if got != wantPrefix {
		t.Fatalf("output mismatch:\n--- want ---\n%s\n--- got ---\n%s", wantPrefix, got)
	}
}

// TestBuildEndpointsSection_PreviewLargeIsTruncated — endpoints превышают порог.
func TestBuildEndpointsSection_PreviewLargeIsTruncated(t *testing.T) {
	gens := make([]string, 50)
	for i := range gens {
		gens[i] = `{"type":"wireguard"}`
	}
	got, err := BuildEndpointsSection(json.RawMessage(`[]`), gens, true,
		PreviewStats{EndpointsCount: 200})
	if err != nil {
		t.Fatalf("BuildEndpointsSection: %v", err)
	}
	if !strings.Contains(got, "Generated: 200 endpoints") {
		t.Errorf("truncation summary missing: %s", got)
	}
}

// TestBuildEndpointsSection_PreviewWithGenerated — preview с одним endpoint.
func TestBuildEndpointsSection_PreviewWithGenerated(t *testing.T) {
	gens := []string{`{"type":"wireguard","tag":"wg-home"}`}
	got, err := BuildEndpointsSection(json.RawMessage(`[]`), gens, true,
		PreviewStats{EndpointsCount: 1})
	if err != nil {
		t.Fatalf("BuildEndpointsSection: %v", err)
	}
	if !strings.Contains(got, `"wg-home"`) {
		t.Errorf("generated endpoint missing: %s", got)
	}
}

// TestBuildOutboundsSection_TrailingCommaWhenStaticFollows — динамическая
// нода имеет запятую если за ней static; не имеет если ничего нет.
func TestBuildOutboundsSection_TrailingCommaWhenStaticFollows(t *testing.T) {
	gens := []string{`{"type":"vless","tag":"v1"}`}
	// Без static: запятой после v1 быть не должно.
	got1, _ := BuildOutboundsSection(json.RawMessage(`[]`), gens, true, PreviewStats{NodesCount: 1})
	// Найдём строку c v1 и убедимся что она без trailing-запятой.
	for _, line := range strings.Split(got1, "\n") {
		if strings.Contains(line, `"v1"`) {
			if strings.HasSuffix(strings.TrimRight(line, " "), ",") {
				t.Errorf("no static → no trailing comma; got line: %q", line)
			}
		}
	}
	// Со static: запятая должна быть.
	tmpl := json.RawMessage(`[{"type":"direct"}]`)
	got2, _ := BuildOutboundsSection(tmpl, gens, true, PreviewStats{NodesCount: 1})
	for _, line := range strings.Split(got2, "\n") {
		if strings.Contains(line, `"v1"`) {
			if !strings.HasSuffix(strings.TrimRight(line, " "), ",") {
				t.Errorf("static follows → expect trailing comma; got line: %q", line)
			}
		}
	}
}
