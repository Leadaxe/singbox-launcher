package state

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// SPEC 188: прежняя версия приложения v9 не читает и после отказа может
// перезаписать состояние шаблоном. Первая запись v8 → v9 обязана оставить
// рядом побайтную копию исходника; повторная — не трогать её; файл, уже
// бывший v9, копии не порождает.
func TestStateV9FirstSaveKeepsPreviousFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	v8 := []byte(`{
  "meta": { "version": 8, "schema": "sources_v8", "created_at": "2026-08-01T00:00:00Z", "updated_at": "2026-08-01T00:00:00Z" },
  "sources": [],
  "directions": [],
  "rules": [
    { "kind": "srs", "name": "Streaming", "enabled": true,
      "refs": ["https://example.com/netflix.srs"],
      "body": { "outbound": "direct" } }
  ]
}`)
	if err := os.WriteFile(path, v8, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatalf("load v8: %v", err)
	}
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	bak := path + ".v8.bak"
	got, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("copy of the v8 file missing: %v", err)
	}
	if !bytes.Equal(got, v8) {
		t.Fatalf("copy differs from the original v8 file:\n%s", got)
	}
	cur, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, meta, _ := sniffSchemaVersion(cur); meta != SchemaVersionV9 {
		t.Fatalf("state.json not rewritten as v9: meta.version=%d", meta)
	}

	// Повторное сохранение копию не трогает.
	info1, _ := os.Stat(bak)
	if err := s.Save(path); err != nil {
		t.Fatalf("second save: %v", err)
	}
	got2, err := os.ReadFile(bak)
	if err != nil || !bytes.Equal(got2, v8) {
		t.Fatalf("second save changed the copy: %v", err)
	}
	info2, _ := os.Stat(bak)
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Fatalf("second save rewrote the copy")
	}

	// Файл, уже бывший v9, копии не порождает.
	dir9 := t.TempDir()
	path9 := filepath.Join(dir9, "state.json")
	if err := os.WriteFile(path9, cur, 0o644); err != nil {
		t.Fatal(err)
	}
	s9, err := Load(path9)
	if err != nil {
		t.Fatalf("load v9: %v", err)
	}
	if err := s9.Save(path9); err != nil {
		t.Fatalf("save v9: %v", err)
	}
	entries, err := os.ReadDir(dir9)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "state.json" {
			t.Fatalf("v9 save left an extra file: %s", e.Name())
		}
	}
}
