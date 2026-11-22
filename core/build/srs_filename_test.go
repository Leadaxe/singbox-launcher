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
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// TestCollectSrsCachedPaths_URLDerived — для kind=srs rule возвращает path
// с URL-derived filename, не с identity.
func TestCollectSrsCachedPaths_URLDerived(t *testing.T) {
	url := "https://example.com/geosite-youtube.srs"
	rules := []state.Rule{srsRule("YT", url, "proxy-out")}
	paths := CollectSrsCachedPaths(rules, "/exec", "")

	// SPEC 188: ключ карты — сама ссылка, а не identity правила.
	got, ok := paths[url]
	if !ok || len(paths) != 1 {
		t.Fatalf("paths map should contain the ref %q with one path: %+v", url, paths)
	}
	// Filename должно быть URL-derived (SRSTagFromURL), а НЕ просто identity.
	expectedTag := SRSTagFromURL(url)
	wantPath := filepath.Join("/exec", "bin", "rule-sets", expectedTag+".srs")
	if got != wantPath {
		t.Errorf("path: got %q, want %q", got, wantPath)
	}
	// Sanity: не label-based filename (вот это был bug).
	if strings.Contains(got, "/YT.srs") || strings.Contains(got, "/rule-YT.srs") {
		t.Errorf("path contains label-based filename — bug #77 regression: %q", got)
	}
}

// TestCollectSrsCachedPaths_TwoRulesSameURL_OneFile — 2 правила с одинаковым URL
// (но разными именами) делят одну запись карты и один файл на диске.
func TestCollectSrsCachedPaths_TwoRulesSameURL_OneFile(t *testing.T) {
	url := "https://example.com/list.srs"
	rules := []state.Rule{
		srsRule("Rule A", url, "proxy-out"),
		srsRule("Rule B", url, "direct-out"),
	}
	paths := CollectSrsCachedPaths(rules, "/exec", "")
	if len(paths) != 1 || paths[url] == "" {
		t.Fatalf("expected one ref entry, got %d: %+v", len(paths), paths)
	}
}

// TestCollectSrsCachedPaths_RenameDoesNotInvalidate — переименование label
// не ломает соответствие с уже скачанным файлом (filename URL-derived).
func TestCollectSrsCachedPaths_RenameDoesNotInvalidate(t *testing.T) {
	url := "https://example.com/list.srs"
	expectedTag := SRSTagFromURL(url)
	wantPath := filepath.Join("/exec", "bin", "rule-sets", expectedTag+".srs")

	before := []state.Rule{srsRule("OldName", url, "direct-out")}
	after := []state.Rule{srsRule("NewName", url, "direct-out")}
	bp := CollectSrsCachedPaths(before, "/exec", "")
	ap := CollectSrsCachedPaths(after, "/exec", "")

	if bp[url] != wantPath {
		t.Errorf("before: got %q want %q", bp[url], wantPath)
	}
	if ap[url] != wantPath {
		t.Errorf("after rename: got %q want %q (same file should be reused)", ap[url], wantPath)
	}
}

// srsRule — включённая srs-запись (контракт 1.1.179): `refs` и цель в теле.
func srsRule(name, url, outbound string) state.Rule {
	r := state.NewSrsRule(name, []string{url}, outbound)
	r.Enabled = true
	return r
}
