package state

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Контракт 1.1.126 (SPEC 170, D-134): ключ записи `skip_presets` переименован
// в `skip_tailscale_presets`. Старый state.json читается терпимо — true у
// корневого сервера и у члена папки переезжает в новое поле, поля allow_*
// читаются как есть, а сохранение пишет только новые ключи.
func TestLoadLegacySkipPresetsKey(t *testing.T) {
	const file = `{
  "meta": {"version": 8, "schema": "sources_v8", "created_at": "2026-10-01T00:00:00Z", "updated_at": "2026-10-01T00:00:00Z"},
  "sources": [
    {"kind": "server", "id": "01J00000000000000000000TS1", "tag": "home-ts", "enabled": true,
     "body": {"type": "tailscale", "tag": "home-ts", "auth_key": "tskey-auth-example"}, "skip_presets": true},
    {"kind": "server", "id": "01J00000000000000000000WG1", "tag": "wg-home", "enabled": true,
     "body": {"type": "wireguard", "tag": "wg-home", "address": ["10.8.0.2/32"], "private_key": "k"},
     "allow_lan": true, "allow_gateway": true},
    {"kind": "folder", "id": "01J000000000000000000FOLDR", "enabled": true, "name": "F",
     "nodes": [{"kind": "server", "tag": "m", "enabled": true,
       "body": {"type": "tailscale", "tag": "m", "auth_key": "tskey-auth-m"}, "skip_presets": true, "allow_local": true}]}
  ],
  "directions": [],
  "rules": [],
  "dns": {}
}`
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	check := func(s *State, stage string) {
		t.Helper()
		var ts, wg, m *Node
		for i := range s.Sources {
			switch s.Sources[i].Tag {
			case "home-ts":
				ts = &s.Sources[i].Node
			case "wg-home":
				wg = &s.Sources[i].Node
			}
			if s.Sources[i].Kind == SourceKindFolder && len(s.Sources[i].Nodes) == 1 {
				m = &s.Sources[i].Nodes[0]
			}
		}
		if ts == nil || wg == nil || m == nil {
			t.Fatalf("%s: узлы потеряны", stage)
		}
		if !ts.SkipTailscalePresets || ts.LegacySkipPresets {
			t.Errorf("%s: корневой сервер: skip_tailscale_presets=%v, легаси=%v", stage, ts.SkipTailscalePresets, ts.LegacySkipPresets)
		}
		if !m.SkipTailscalePresets || m.LegacySkipPresets || !m.AllowLocal {
			t.Errorf("%s: член папки: %+v, легаси=%v", stage, m.NodeRecordFlags, m.LegacySkipPresets)
		}
		if wg.SkipTailscalePresets || wg.AllowLocal || !wg.AllowLan || !wg.AllowGateway {
			t.Errorf("%s: wg-home: %+v", stage, wg.NodeRecordFlags)
		}
	}

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	check(s, "Load")
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(saved, []byte(`"skip_presets"`)) {
		t.Errorf("сохранение записало легаси-ключ skip_presets:\n%s", saved)
	}
	if n := bytes.Count(saved, []byte(`"skip_tailscale_presets": true`)); n != 2 {
		t.Errorf("skip_tailscale_presets: true в файле %d раз, ожидалось 2:\n%s", n, saved)
	}
	if bytes.Contains(saved, []byte(`"allow_local": false`)) || bytes.Contains(saved, []byte(`"skip_tailscale_presets": false`)) {
		t.Errorf("false у поля записи записан в файл:\n%s", saved)
	}
	s2, err := Load(path)
	if err != nil {
		t.Fatalf("повторный Load: %v", err)
	}
	check(s2, "Save→Load")
}
