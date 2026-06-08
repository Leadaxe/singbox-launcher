// File srs_filename_test.go — SPEC 063: integration tests для issue #77 fix.
//
// Проверяем что user-SRS filename во всех 3 точках единого источника правды:
//  1. CollectSrsCachedPaths   — value path использует URL-derived tag
//  2. ResolveRoute            — emit'ит local rule_set с тем же path
//  3. collectAllStageRuleSetTags (косвенно через CollectSrsCachedPaths logic)
//
// Issue #77 reproducer: user добавляет SRS rule, файл скачивается под
// SRSTagFromURL(URL); сборщик ранее искал под `<r.ID>.srs` — mismatch →
// "no cached file" warning, orphan GC удалял реально-скачанный файл.
package build

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// TestCollectSrsCachedPaths_URLDerived — для kind=srs rule возвращает path
// с URL-derived filename, не с identity.
func TestCollectSrsCachedPaths_URLDerived(t *testing.T) {
	url := "https://example.com/geosite-youtube.srs"
	body, _ := json.Marshal(state.SrsBody{Name: "YT", SrsURL: url, Outbound: "proxy-out"})
	rules := []state.Rule{
		{Kind: state.RuleKindSrs, Enabled: true, Body: body},
	}
	paths := CollectSrsCachedPaths(rules, "/exec")

	id := state.StableRuleID(rules[0])
	if id != "YT" {
		t.Fatalf("StableRuleID baseline: got %q", id)
	}
	got, ok := paths[id]
	if !ok {
		t.Fatalf("paths map should contain identity %q: %+v", id, paths)
	}
	// Filename должно быть URL-derived (SRSTagFromURL), а НЕ просто identity.
	expectedTag := SRSTagFromURL(url)
	wantPath := "/exec/bin/rule-sets/" + expectedTag + ".srs"
	if got != wantPath {
		t.Errorf("path: got %q, want %q", got, wantPath)
	}
	// Sanity: не label-based filename (вот это был bug).
	if strings.Contains(got, "/YT.srs") || strings.Contains(got, "/rule-YT.srs") {
		t.Errorf("path contains label-based filename — bug #77 regression: %q", got)
	}
}

// TestCollectSrsCachedPaths_TwoRulesSameURL_OneFile — 2 правила с одинаковым URL
// (но разными именами) дедуплицируются в один файл на диске.
func TestCollectSrsCachedPaths_TwoRulesSameURL_OneFile(t *testing.T) {
	url := "https://example.com/list.srs"
	rules := []state.Rule{
		{Kind: state.RuleKindSrs, Enabled: true,
			Body: mustMarshalSRS("Rule A", url, "proxy-out")},
		{Kind: state.RuleKindSrs, Enabled: true,
			Body: mustMarshalSRS("Rule B", url, "direct-out")},
	}
	paths := CollectSrsCachedPaths(rules, "/exec")
	if len(paths) != 2 {
		t.Fatalf("expected 2 identity entries, got %d: %+v", len(paths), paths)
	}
	// Оба identity → один и тот же путь.
	pathA := paths["Rule-A"]
	pathB := paths["Rule-B"]
	if pathA == "" || pathB == "" {
		t.Fatalf("missing identity in map: %+v", paths)
	}
	if pathA != pathB {
		t.Errorf("dedup expected: same URL → same path, got A=%q B=%q", pathA, pathB)
	}
}

// TestCollectSrsCachedPaths_RenameDoesNotInvalidate — переименование label
// не ломает соответствие с уже скачанным файлом (filename URL-derived).
func TestCollectSrsCachedPaths_RenameDoesNotInvalidate(t *testing.T) {
	url := "https://example.com/list.srs"
	expectedTag := SRSTagFromURL(url)
	wantPath := "/exec/bin/rule-sets/" + expectedTag + ".srs"

	before := []state.Rule{
		{Kind: state.RuleKindSrs, Enabled: true,
			Body: mustMarshalSRS("OldName", url, "direct-out")},
	}
	after := []state.Rule{
		{Kind: state.RuleKindSrs, Enabled: true,
			Body: mustMarshalSRS("NewName", url, "direct-out")},
	}
	bp := CollectSrsCachedPaths(before, "/exec")
	ap := CollectSrsCachedPaths(after, "/exec")

	if bp["OldName"] != wantPath {
		t.Errorf("before: got %q want %q", bp["OldName"], wantPath)
	}
	if ap["NewName"] != wantPath {
		t.Errorf("after rename: got %q want %q (same file should be reused)", ap["NewName"], wantPath)
	}
}

func mustMarshalSRS(name, url, outbound string) json.RawMessage {
	b, _ := json.Marshal(state.SrsBody{Name: name, SrsURL: url, Outbound: outbound})
	return b
}
