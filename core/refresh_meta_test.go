package core

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/core/state"
	v5 "singbox-launcher/core/state/v5"
	"singbox-launcher/internal/platform"
)

// TestRefreshSubscriptionsMetaAndCache_HappyPath — SPEC 052 phase 5:
// после Update должен появиться `bin/subscriptions/<id>.raw` и Meta
// должна быть заполнена в state.json.
func TestRefreshSubscriptionsMetaAndCache_HappyPath(t *testing.T) {
	body := "vless://uuid@host:443#tokyo\nvless://uuid@host:443#fra\nvless://uuid@host:443#sgp\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", "upload=10; download=20; total=100; expire=1717171717")
		w.Header().Set("Profile-Title", "TestSub")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	execDir := t.TempDir()
	// Готовим bin/wizard_states/state.json с одним subscription.
	statesDir := platform.GetWizardStatesDir(execDir)
	if err := os.MkdirAll(statesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	s := state.New()
	s.Connections.Sources = []state.Source{
		{
			ID:      "01TESTID",
			Type:    state.SourceTypeSubscription,
			Enabled: true,
			URL:     srv.URL,
		},
	}
	if err := s.Save(platform.GetWizardStatePath(execDir)); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	// Подменяем HTTP-фабрику subscription пакета (нужно для FetchSubscriptionWithMeta).
	// Можно работать через дефолт (CreateHTTPClientFunc nil → дефолтный клиент).

	loaded, err := state.Load(platform.GetWizardStatePath(execDir))
	if err != nil {
		t.Fatalf("Load state: %v", err)
	}
	refreshSubscriptionsMetaAndCache(loaded, execDir)

	// 1. raw body файл существует.
	rawPath := filepath.Join(platform.GetSubscriptionsDir(execDir), "01TESTID.raw")
	rawData, err := os.ReadFile(rawPath)
	if err != nil {
		t.Fatalf("raw body should exist at %s: %v", rawPath, err)
	}
	if string(rawData) != body {
		t.Errorf("raw body mismatch: got %q, want %q", rawData, body)
	}

	// 2. state перечитан с диска должен содержать meta.
	reloaded, err := state.Load(platform.GetWizardStatePath(execDir))
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if len(reloaded.Connections.Sources) != 1 {
		t.Fatalf("source count: %d", len(reloaded.Connections.Sources))
	}
	src := reloaded.Connections.Sources[0]
	if src.Meta == nil {
		t.Fatalf("Meta should be populated after refresh")
	}
	if src.Meta.LastStatus != "ok" {
		t.Errorf("LastStatus = %q, want ok", src.Meta.LastStatus)
	}
	if src.Meta.ProfileTitle != "TestSub" {
		t.Errorf("ProfileTitle lost: %q", src.Meta.ProfileTitle)
	}
	if src.Meta.UserInfo == nil || src.Meta.UserInfo.TotalBytes != 100 {
		t.Errorf("UserInfo: %+v", src.Meta.UserInfo)
	}
	if src.Meta.NodesCountFetched != 3 {
		t.Errorf("NodesCountFetched = %d, want 3", src.Meta.NodesCountFetched)
	}
	if len(src.Meta.PreviewNodes) != 3 {
		t.Errorf("PreviewNodes count = %d, want 3", len(src.Meta.PreviewNodes))
	}
	if src.Meta.HTTPStatusCode != 200 {
		t.Errorf("HTTPStatusCode = %d", src.Meta.HTTPStatusCode)
	}
	if src.Meta.RawBodyBytes != int64(len(body)) {
		t.Errorf("RawBodyBytes = %d, want %d", src.Meta.RawBodyBytes, len(body))
	}
}

// TestRefreshSubscriptionsMetaAndCache_FailureKeepsOldRaw — на failed fetch
// старый raw файл сохраняется, error_count инкрементится.
func TestRefreshSubscriptionsMetaAndCache_FailureKeepsOldRaw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	execDir := t.TempDir()
	if err := os.MkdirAll(platform.GetWizardStatesDir(execDir), 0o755); err != nil {
		t.Fatal(err)
	}

	// Заранее заводим raw body — proxy "уже один раз успешно сходили".
	subsDir := platform.GetSubscriptionsDir(execDir)
	oldBody := []byte("vless://old#previous\n")
	if err := v5.WriteRawBody(subsDir, "01FAIL", oldBody); err != nil {
		t.Fatal(err)
	}

	s := state.New()
	s.Connections.Sources = []state.Source{
		{
			ID:      "01FAIL",
			Type:    state.SourceTypeSubscription,
			Enabled: true,
			URL:     srv.URL,
			Meta:    &state.SubscriptionMeta{ErrorCount: 0},
		},
	}
	if err := s.Save(platform.GetWizardStatePath(execDir)); err != nil {
		t.Fatal(err)
	}

	loaded, _ := state.Load(platform.GetWizardStatePath(execDir))
	refreshSubscriptionsMetaAndCache(loaded, execDir)

	// 1. Старый raw НЕ повреждён.
	got, err := v5.ReadRawBody(subsDir, "01FAIL")
	if err != nil {
		t.Fatalf("old raw should be preserved: %v", err)
	}
	if string(got) != string(oldBody) {
		t.Errorf("old raw modified: got %q, want %q", got, oldBody)
	}

	// 2. Meta содержит status=err, error_count=1, http_status_code=403.
	reloaded, _ := state.Load(platform.GetWizardStatePath(execDir))
	src := reloaded.Connections.Sources[0]
	if src.Meta == nil || src.Meta.LastStatus != "err" {
		t.Errorf("LastStatus should be err: %+v", src.Meta)
	}
	if src.Meta.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", src.Meta.ErrorCount)
	}
	if src.Meta.HTTPStatusCode != 403 {
		t.Errorf("HTTPStatusCode = %d, want 403", src.Meta.HTTPStatusCode)
	}
}

// TestRefreshSubscriptionsMetaAndCache_DeleteOrphans — после удаления
// source'а из state, его .raw файл подчищается на следующем Update.
func TestRefreshSubscriptionsMetaAndCache_DeleteOrphans(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("vless://uuid@host:443#a\n"))
	}))
	defer srv.Close()

	execDir := t.TempDir()
	if err := os.MkdirAll(platform.GetWizardStatesDir(execDir), 0o755); err != nil {
		t.Fatal(err)
	}
	subsDir := platform.GetSubscriptionsDir(execDir)
	// Orphan, которого нет в state.
	if err := v5.WriteRawBody(subsDir, "01ORPHAN", []byte("orphan")); err != nil {
		t.Fatal(err)
	}

	s := state.New()
	s.Connections.Sources = []state.Source{
		{ID: "01KEEP", Type: state.SourceTypeSubscription, Enabled: true, URL: srv.URL},
	}
	if err := s.Save(platform.GetWizardStatePath(execDir)); err != nil {
		t.Fatal(err)
	}

	loaded, _ := state.Load(platform.GetWizardStatePath(execDir))
	refreshSubscriptionsMetaAndCache(loaded, execDir)

	// orphan должен быть удалён.
	if _, err := v5.ReadRawBody(subsDir, "01ORPHAN"); err == nil {
		t.Errorf("orphan raw should be deleted")
	}
	// keep должен быть на месте.
	if _, err := v5.ReadRawBody(subsDir, "01KEEP"); err != nil {
		t.Errorf("kept raw missing: %v", err)
	}
}
