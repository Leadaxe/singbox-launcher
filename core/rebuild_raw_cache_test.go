package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/core/state"
	v5 "singbox-launcher/core/state/v5"
	"singbox-launcher/internal/platform"
)

// TestBuildSnapshotFromRawCache_HappyPath — полностью offline-сценарий:
// state с одним subscription + готовый bin/subscriptions/<id>.raw
// → snapshot строится без network call'ов.
func TestBuildSnapshotFromRawCache_HappyPath(t *testing.T) {
	execDir := t.TempDir()
	subsDir := platform.GetSubscriptionsDir(execDir)
	if err := os.MkdirAll(subsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Реальный VLESS URI чтобы парсер действительно вернул ноду.
	body := []byte("vless://12345678-1234-1234-1234-123456789abc@example.com:443?encryption=none&security=tls&type=tcp#tokyo\n")
	if err := v5.WriteRawBody(subsDir, "01TESTRAW", body); err != nil {
		t.Fatal(err)
	}

	s := &state.State{
		Connections: state.ConnectionsSection{
			Sources: []state.Source{
				{
					ID:      "01TESTRAW",
					Type:    state.SourceTypeSubscription,
					Enabled: true,
					URL:     "https://test/sub",
				},
			},
		},
	}
	// Заполняем legacy view (как делает Load).
	if err := os.MkdirAll(platform.GetWizardStatesDir(execDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(filepath.Join(platform.GetWizardStatesDir(execDir), "state.json")); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.Load(filepath.Join(platform.GetWizardStatesDir(execDir), "state.json"))
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}

	snap, err := buildSnapshotFromRawCache(loaded, execDir, nil)
	if err != nil {
		t.Fatalf("buildSnapshotFromRawCache: %v", err)
	}
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if len(snap.Outbounds) == 0 {
		t.Errorf("expected at least 1 outbound parsed from raw, got 0")
	}
}

// TestBuildSnapshotFromRawCache_IncompleteCache — отсутствие .raw для
// enabled subscription → ErrRawCacheIncomplete.
func TestBuildSnapshotFromRawCache_IncompleteCache(t *testing.T) {
	execDir := t.TempDir()
	if err := os.MkdirAll(platform.GetWizardStatesDir(execDir), 0o755); err != nil {
		t.Fatal(err)
	}

	s := &state.State{
		Connections: state.ConnectionsSection{
			Sources: []state.Source{
				{
					ID:      "01MISSING",
					Type:    state.SourceTypeSubscription,
					Enabled: true,
					URL:     "https://test/sub",
				},
			},
		},
	}
	if err := s.Save(platform.GetWizardStatePath(execDir)); err != nil {
		t.Fatal(err)
	}
	loaded, _ := state.Load(platform.GetWizardStatePath(execDir))

	_, err := buildSnapshotFromRawCache(loaded, execDir, nil)
	if !errors.Is(err, ErrRawCacheIncomplete) {
		t.Errorf("expected ErrRawCacheIncomplete, got %v", err)
	}
}

// TestBuildSnapshotFromRawCache_DisabledSubscriptionsIgnored — disabled
// subscription без .raw НЕ должна блокировать rebuild. Микс enabled+disabled:
// enabled парсится, disabled пропускается, completeness-check тоже пропускает
// disabled (не требует .raw).
func TestBuildSnapshotFromRawCache_DisabledSubscriptionsIgnored(t *testing.T) {
	execDir := t.TempDir()
	subsDir := platform.GetSubscriptionsDir(execDir)
	if err := os.MkdirAll(subsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(platform.GetWizardStatesDir(execDir), 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("vless://12345678-1234-1234-1234-123456789abc@example.com:443?encryption=none&security=tls&type=tcp#tokyo\n")
	if err := v5.WriteRawBody(subsDir, "01ENABLED", body); err != nil {
		t.Fatal(err)
	}

	s := &state.State{
		Connections: state.ConnectionsSection{
			Sources: []state.Source{
				{ID: "01ENABLED", Type: state.SourceTypeSubscription, Enabled: true, URL: "https://test/sub-a"},
				{ID: "01DISABLED", Type: state.SourceTypeSubscription, Enabled: false, URL: "https://test/sub-b"},
			},
		},
	}
	if err := s.Save(platform.GetWizardStatePath(execDir)); err != nil {
		t.Fatal(err)
	}
	loaded, _ := state.Load(platform.GetWizardStatePath(execDir))

	snap, err := buildSnapshotFromRawCache(loaded, execDir, nil)
	if err != nil {
		t.Fatalf("rebuild with mix enabled/disabled: %v", err)
	}
	if snap == nil || len(snap.Outbounds) == 0 {
		t.Errorf("expected outbounds from enabled source, got empty: %+v", snap)
	}
}

// TestCleanupLegacyOutboundsCache — bin/outbounds.cache.json удаляется.
func TestCleanupLegacyOutboundsCache(t *testing.T) {
	execDir := t.TempDir()
	cachePath := platform.GetOutboundsCachePath(execDir)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanupLegacyOutboundsCache(execDir)

	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Errorf("expected outbounds.cache.json to be removed, stat=%v", err)
	}

	// Idempotent: повторный вызов не падает.
	cleanupLegacyOutboundsCache(execDir)
}
