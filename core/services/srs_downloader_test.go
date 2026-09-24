package services

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/paths"
)

// fillRuleSetsDir создаёт bin/rule-sets/ внутри execDir и пишет в него
// stub-файлы под перечисленные имена (как есть, без .srs автоматического
// дополнения — позволяет тестировать как валидные `<tag>.srs`, так и
// мусор `notes.txt`).
func fillRuleSetsDir(t *testing.T, execDir string, names []string) {
	t.Helper()
	dir := filepath.Join(execDir, constants.BinDirName, constants.RuleSetsDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("stub"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

// listRuleSetsDir возвращает отсортированный список имён файлов в
// bin/rule-sets/. Subdirectories пропускаются.
func listRuleSetsDir(t *testing.T, execDir string) []string {
	t.Helper()
	dir := filepath.Join(execDir, constants.BinDirName, constants.RuleSetsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestDeleteOrphanRuleSets_RemovesUnknown(t *testing.T) {
	execDir := t.TempDir()
	fillRuleSetsDir(t, execDir, []string{"ru-blocked-main.srs", "ads-all.srs", "stale.srs"})

	known := []string{"ru-blocked-main", "ads-all"}
	deleted, err := DeleteOrphanRuleSets(paths.DataDir(execDir), known)
	if err != nil {
		t.Fatalf("DeleteOrphanRuleSets: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != "stale.srs" {
		t.Errorf("expected only stale.srs deleted, got %v", deleted)
	}
	got := listRuleSetsDir(t, execDir)
	want := []string{"ads-all.srs", "ru-blocked-main.srs"}
	if !equalSlices(got, want) {
		t.Errorf("after GC want %v, got %v", want, got)
	}
}

// TestDeleteOrphanRuleSets_RemovesNonSRSGarbage — папка launcher-managed,
// файлы без `.srs` тоже считаются orphan'ами и удаляются.
func TestDeleteOrphanRuleSets_RemovesNonSRSGarbage(t *testing.T) {
	execDir := t.TempDir()
	fillRuleSetsDir(t, execDir, []string{"ru-inside.srs", "notes.txt", "junk", "левый-файл"})

	known := []string{"ru-inside"}
	deleted, err := DeleteOrphanRuleSets(paths.DataDir(execDir), known)
	if err != nil {
		t.Fatalf("DeleteOrphanRuleSets: %v", err)
	}
	if len(deleted) != 3 {
		t.Errorf("expected 3 garbage files removed, got %v", deleted)
	}
	got := listRuleSetsDir(t, execDir)
	if len(got) != 1 || got[0] != "ru-inside.srs" {
		t.Errorf("after GC want only ru-inside.srs, got %v", got)
	}
}

// TestDeleteOrphanRuleSets_PreservesAllKnown — пустой deleted на full match.
func TestDeleteOrphanRuleSets_PreservesAllKnown(t *testing.T) {
	execDir := t.TempDir()
	fillRuleSetsDir(t, execDir, []string{"a.srs", "b.srs", "c.srs"})

	deleted, err := DeleteOrphanRuleSets(paths.DataDir(execDir), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("DeleteOrphanRuleSets: %v", err)
	}
	if len(deleted) != 0 {
		t.Errorf("nothing should be deleted, got %v", deleted)
	}
	if got := listRuleSetsDir(t, execDir); len(got) != 3 {
		t.Errorf("all 3 files should remain, got %v", got)
	}
}

// TestDeleteOrphanRuleSets_MissingDirIsNoOp — если `bin/rule-sets/` не
// существует (свежая инсталляция, ни один rule не enable'нут) — функция
// возвращает (nil, nil) без ошибки. Используется как идемпотентный no-op.
func TestDeleteOrphanRuleSets_MissingDirIsNoOp(t *testing.T) {
	execDir := t.TempDir() // bin/rule-sets/ ещё не создан
	deleted, err := DeleteOrphanRuleSets(paths.DataDir(execDir), []string{"x"})
	if err != nil {
		t.Errorf("missing dir should be no-op, got error: %v", err)
	}
	if len(deleted) != 0 {
		t.Errorf("missing dir → no deletes, got %v", deleted)
	}
}

// TestDeleteOrphanRuleSets_EmptyKnownClearsAll — если пользователь
// disable'нул все rules, на следующем Rebuild папка чистится полностью.
func TestDeleteOrphanRuleSets_EmptyKnownClearsAll(t *testing.T) {
	execDir := t.TempDir()
	fillRuleSetsDir(t, execDir, []string{"a.srs", "b.srs"})

	deleted, err := DeleteOrphanRuleSets(paths.DataDir(execDir), []string{})
	if err != nil {
		t.Fatalf("DeleteOrphanRuleSets: %v", err)
	}
	if len(deleted) != 2 {
		t.Errorf("expected 2 files deleted, got %v", deleted)
	}
	if got := listRuleSetsDir(t, execDir); len(got) != 0 {
		t.Errorf("all files should be gone, got %v", got)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
