package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// TestPersistFetchResultTargetsConfiguratorState — SPEC 165: результат Refresh
// закрепляется в файле состояния ТОЙ цели, чей Конфигуратор открыт. Два
// профиля с общим ID источника (машина — копия Local): персист для машины
// меняет только её файл, персист для Local — только локальный.
func TestPersistFetchResultTargetsConfiguratorState(t *testing.T) {
	tmp := t.TempDir()
	dataDir := paths.DataDir(tmp)
	ac := &AppController{FileService: &services.FileService{Layout: paths.Layout{Data: dataDir}}}
	svc := &ConfigService{ac: ac}

	const sharedID = "01SHAREDSOURCEID0000000000"
	node := func(tag string) state.Node {
		return state.Node{Kind: state.SourceKindServer, Tag: tag,
			Body: json.RawMessage(`{"type":"vless","tag":"` + tag + `"}`)}
	}
	write := func(path, tag string) {
		s := &state.State{Sources: []state.Source{{
			Node: state.Node{Kind: state.SourceKindSubscription}, ID: sharedID,
			URL: "https://example.invalid/sub", Nodes: []state.Node{node(tag)},
		}}}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := s.Save(path); err != nil {
			t.Fatalf("save %s: %v", path, err)
		}
	}
	firstTag := func(path string) string {
		s, err := state.Load(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		src := s.FindSource(sharedID)
		if src == nil || len(src.Nodes) == 0 {
			t.Fatalf("%s: source %s missing or empty", path, sharedID)
		}
		return src.Nodes[0].Tag
	}

	localPath := platform.GetWizardStatePath(dataDir)
	machinePath := platform.GetWizardStatePathFor(dataDir, constants.ConfigTargetRemote, "m1")
	write(localPath, "local-old")
	write(machinePath, "machine-old")

	fetched := &state.Source{Node: state.Node{Kind: state.SourceKindSubscription}, ID: sharedID,
		URL: "https://example.invalid/sub", Nodes: []state.Node{node("machine-new")}}
	svc.persistFetchResultForSource(fetched, dataDir, constants.ConfigTargetRemote, "m1")
	if got := firstTag(machinePath); got != "machine-new" {
		t.Fatalf("machine state not updated: %q", got)
	}
	if got := firstTag(localPath); got != "local-old" {
		t.Fatalf("local state overwritten by a machine refresh: %q", got)
	}

	fetched.Nodes = []state.Node{node("local-new")}
	svc.persistFetchResultForSource(fetched, dataDir, constants.ConfigTargetLocal, "")
	if got := firstTag(localPath); got != "local-new" {
		t.Fatalf("local state not updated: %q", got)
	}
	if got := firstTag(machinePath); got != "machine-new" {
		t.Fatalf("machine state touched by a local refresh: %q", got)
	}
}
