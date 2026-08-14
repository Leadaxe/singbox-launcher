package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// SPEC 097: meta.target переживает round-trip Save→Load. Без этого визард
// после перезапуска забывал бы, что состояние в remote/ готовится для
// удалённой машины, и собрал бы ей local-конфиг (с clash_api и find_process).
func TestTargetMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{
		Version:        SchemaVersionV6,
		Target:         "remote",
		TargetPlatform: "linux",
		TargetArch:     "arm64",
	}
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Поля должны лечь именно в meta — там их ищет читатель.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		Meta map[string]interface{} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatal(err)
	}
	if probe.Meta["target"] != "remote" || probe.Meta["target_platform"] != "linux" || probe.Meta["target_arch"] != "arm64" {
		t.Fatalf("meta target fields wrong: %v", probe.Meta)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Target != "remote" || got.TargetPlatform != "linux" || got.TargetArch != "arm64" {
		t.Fatalf("round-trip lost target: %+v", got)
	}
}

// Состояния, записанные до SPEC 097, не имеют meta.target — они должны
// читаться как local без всякой миграции.
func TestLegacyStateHasEmptyTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{Version: SchemaVersionV6}
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, _ := os.ReadFile(path)
	var probe struct {
		Meta map[string]interface{} `json:"meta"`
	}
	_ = json.Unmarshal(raw, &probe)
	// omitempty: пустой таргет не засоряет файл.
	if _, ok := probe.Meta["target"]; ok {
		t.Errorf("empty target must be omitted from meta, got %v", probe.Meta)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Target != "" {
		t.Errorf("legacy state must load with empty target, got %q", got.Target)
	}
}
