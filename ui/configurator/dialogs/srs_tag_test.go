package dialogs

import (
	"strings"
	"testing"
)

// TestSrsTagFromURL_FormatStability — детерминизм: same URL → same tag.
// Хеш-часть стабильна через runs (SHA-256 от полной URL).
func TestSrsTagFromURL_FormatStability(t *testing.T) {
	url := "https://example.com/path/blocklist.srs"
	tag1 := SRSTagFromURL(url)
	tag2 := SRSTagFromURL(url)
	if tag1 != tag2 {
		t.Errorf("same URL must produce same tag: %q vs %q", tag1, tag2)
	}
	// Должно быть "blocklist-<hash8>".
	if !strings.HasPrefix(tag1, "blocklist-") {
		t.Errorf("expected blocklist- prefix: %q", tag1)
	}
	hash := strings.TrimPrefix(tag1, "blocklist-")
	if len(hash) != 8 {
		t.Errorf("hash part must be 8 hex chars, got %d (%q)", len(hash), hash)
	}
}

// TestSrsTagFromURL_DifferentURLsDifferentTags — две URL с одинаковым
// filename'ом, но разным path / host → разные tags. Гарантия что
// файлы не сталкиваются в bin/rule-sets/.
func TestSrsTagFromURL_DifferentURLsDifferentTags(t *testing.T) {
	tagA := SRSTagFromURL("https://example.com/path1/blocklist.srs")
	tagB := SRSTagFromURL("https://example.com/path2/blocklist.srs")
	tagC := SRSTagFromURL("https://other.com/path1/blocklist.srs")

	if tagA == tagB {
		t.Errorf("different paths must produce different tags: %q vs %q", tagA, tagB)
	}
	if tagA == tagC {
		t.Errorf("different hosts must produce different tags: %q vs %q", tagA, tagC)
	}
	if tagB == tagC {
		t.Errorf("all three must be distinct: A=%q B=%q C=%q", tagA, tagB, tagC)
	}
}

// TestSrsTagFromURL_FilenameStripsSrsSuffix — `.srs` отрезается из filename
// перед hash-attachment'ом.
func TestSrsTagFromURL_FilenameStripsSrsSuffix(t *testing.T) {
	tag := SRSTagFromURL("https://example.com/x/myblock.srs")
	if !strings.HasPrefix(tag, "myblock-") {
		t.Errorf("expected myblock- prefix without .srs: %q", tag)
	}
	if strings.Contains(tag, ".srs") {
		t.Errorf(".srs must not appear in tag: %q", tag)
	}
}

// TestSrsTagFromURL_EmptyFilenameFallsBackToSrs — URL без полезного filename
// (только schema://host/) → tag = "srs-<hash>". Иначе получился бы tag
// с ведущим тире.
func TestSrsTagFromURL_EmptyFilenameFallsBackToSrs(t *testing.T) {
	tag := SRSTagFromURL("https://example.com/")
	if !strings.HasPrefix(tag, "srs-") {
		t.Errorf("empty filename → fallback prefix 'srs-', got %q", tag)
	}
	if strings.HasPrefix(tag, "-") {
		t.Errorf("tag must not start with '-': %q", tag)
	}
}

// TestSrsTagFromURL_LongPathTakesLastSegment — для multi-segment path берётся
// только последний сегмент (имя файла).
func TestSrsTagFromURL_LongPathTakesLastSegment(t *testing.T) {
	tag := SRSTagFromURL("https://example.com/a/b/c/d/finalname.srs")
	if !strings.HasPrefix(tag, "finalname-") {
		t.Errorf("expected last segment as filename: %q", tag)
	}
}

// TestSrsTagFromURL_HashIsHex — hash8 это валидный hex (a-f0-9).
func TestSrsTagFromURL_HashIsHex(t *testing.T) {
	tag := SRSTagFromURL("https://x/y.srs")
	parts := strings.Split(tag, "-")
	if len(parts) < 2 {
		t.Fatalf("tag must have hash suffix: %q", tag)
	}
	hash := parts[len(parts)-1]
	for _, r := range hash {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("hash must be lowercase hex, got %q (rune %q)", hash, r)
		}
	}
}
