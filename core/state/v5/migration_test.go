package v5

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// fixedIDGen возвращает детерминированный счётчик "id-1", "id-2", …
// Только для тестов — production использует MakeULID.
func fixedIDGen() IDGenerator {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("id-%d", n)
	}
}

// TestMigrate_RealFixture — реальный v4 state.json (5 sub + 1 server +
// DNS + custom_rules + vars) → v5. Проверяем форму, не байт-в-байт.
func TestMigrate_RealFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "v4_real.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	old, err := ParseV4(data)
	if err != nil {
		t.Fatalf("parse v4: %v", err)
	}
	if old.Version != 4 {
		t.Fatalf("fixture is not v4 (version=%d)", old.Version)
	}

	state := MigrateV4ToV5(old, fixedIDGen())

	// 1. Meta — version бамплен на 5, timestamps скопированы.
	if state.Meta.Version != SchemaVersion {
		t.Errorf("Meta.Version = %d, want %d", state.Meta.Version, SchemaVersion)
	}
	if state.Meta.CreatedAt != "2026-04-26T19:36:59Z" {
		t.Errorf("Meta.CreatedAt = %q, want fixture timestamp", state.Meta.CreatedAt)
	}

	// 2. Sources: 6 субскрипшенов с source URL + 1 server из connections[]
	//    из 7 ProxySource'ов в фикстуре (один — connections-only WG).
	wantSubCount := 6
	wantSrvCount := 1
	gotSub, gotSrv := 0, 0
	for _, s := range state.Connections.Sources {
		switch s.Type {
		case SourceTypeSubscription:
			gotSub++
		case SourceTypeServer:
			gotSrv++
		}
	}
	if gotSub != wantSubCount || gotSrv != wantSrvCount {
		t.Errorf("source counts: %d sub, %d srv (want %d sub, %d srv)",
			gotSub, gotSrv, wantSubCount, wantSrvCount)
	}

	// 3. Server: правильный label с tag_prefix preservation.
	//    fixture: tag_prefix="WG:", uri ends with "#wg-parnas"
	//    expected label = "WG:wg-parnas"
	for _, s := range state.Connections.Sources {
		if s.Type != SourceTypeServer {
			continue
		}
		if s.Label != "WG:wg-parnas" {
			t.Errorf("server.Label = %q, want %q", s.Label, "WG:wg-parnas")
		}
		if s.URI == "" {
			t.Errorf("server.URI empty")
		}
	}

	// 4. Subscription: TagSpec.Prefix перенесён, ExcludeFromGlobal/Expose флаги
	//    сохранены. Ищем `WL:` источник у которого был exclude_from_global=true.
	var wl *Source
	for i, s := range state.Connections.Sources {
		if s.Tag != nil && s.Tag.Prefix == "WL:" {
			wl = &state.Connections.Sources[i]
			break
		}
	}
	if wl == nil {
		t.Fatalf("no WL: source found in migrated state")
	}
	if !wl.ExcludeFromGlobal {
		t.Errorf("WL: exclude_from_global lost")
	}
	if !wl.ExposeGroupTagsToGlobal {
		t.Errorf("WL: expose_group_tags_to_global lost")
	}
	if wl.Enabled {
		// fixture: disabled=true → enabled=false
		t.Errorf("WL: should be disabled (enabled=%v)", wl.Enabled)
	}

	// 5. Subscription Outbounds (local urltest+selector) перенесены.
	if len(wl.Outbounds) != 2 {
		t.Errorf("WL: expected 2 local outbounds, got %d", len(wl.Outbounds))
	}

	// 6. Connections.Outbounds — глобальные группы перенесены (5 в фикстуре).
	if len(state.Connections.Outbounds) != 5 {
		t.Errorf("global outbounds: got %d, want 5", len(state.Connections.Outbounds))
	}

	// 7. Defaults.Reload скопирован.
	if state.Connections.Defaults.Reload != "4h" {
		t.Errorf("Defaults.Reload = %q, want 4h", state.Connections.Defaults.Reload)
	}
	if state.Connections.Defaults.MaxNodes != DefaultMaxNodes {
		t.Errorf("Defaults.MaxNodes = %d, want %d", state.Connections.Defaults.MaxNodes, DefaultMaxNodes)
	}

	// 8. ConfigParams + CustomRules + Vars + DNSOptions — preserved.
	if len(state.ConfigParams) != 1 || state.ConfigParams[0].Name != "route.final" {
		t.Errorf("ConfigParams lost: %+v", state.ConfigParams)
	}
	if len(state.CustomRules) != 12 {
		t.Errorf("CustomRules count: got %d, want 12", len(state.CustomRules))
	}
	if len(state.Vars) != 13 {
		t.Errorf("Vars count: got %d, want 13", len(state.Vars))
	}
	if state.DNSOptions == nil {
		t.Fatalf("DNSOptions lost")
	}
	if len(state.DNSOptions.Servers) != 9 {
		t.Errorf("DNS servers: got %d, want 9", len(state.DNSOptions.Servers))
	}

	// 9. JSON round-trip — сериализуем + десериализуем без потерь.
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded State
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Connections.Sources) != len(state.Connections.Sources) {
		t.Errorf("round-trip lost sources: %d → %d", len(state.Connections.Sources), len(decoded.Connections.Sources))
	}
}

// TestMigrate_SubscriptionOnly — простая subscription без connections.
func TestMigrate_SubscriptionOnly(t *testing.T) {
	old := &V4File{
		Version: 4,
		ParserConfig: V4ParserConfig{
			Proxies: []configtypes.ProxySource{{
				Source:    "https://example.com/sub",
				TagPrefix: "S:",
				Disabled:  false,
			}},
		},
	}
	state := MigrateV4ToV5(old, fixedIDGen())
	if len(state.Connections.Sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(state.Connections.Sources))
	}
	s := state.Connections.Sources[0]
	if s.Type != SourceTypeSubscription || !s.Enabled || s.URL != "https://example.com/sub" {
		t.Errorf("subscription mismatch: %+v", s)
	}
	if s.Tag == nil || s.Tag.Prefix != "S:" {
		t.Errorf("tag prefix lost")
	}
}

// TestMigrate_ServerOnly — connections без source = массив server-источников.
func TestMigrate_ServerOnly(t *testing.T) {
	old := &V4File{
		Version: 4,
		ParserConfig: V4ParserConfig{
			Proxies: []configtypes.ProxySource{{
				Connections: []string{
					"vless://uuid@host:443#node-A",
					"vless://uuid@host:444",
				},
				TagPrefix: "S:",
				Disabled:  true,
			}},
		},
	}
	state := MigrateV4ToV5(old, fixedIDGen())
	if len(state.Connections.Sources) != 2 {
		t.Fatalf("got %d sources, want 2", len(state.Connections.Sources))
	}
	got0, got1 := state.Connections.Sources[0], state.Connections.Sources[1]
	if got0.Type != SourceTypeServer || got1.Type != SourceTypeServer {
		t.Errorf("both should be server type")
	}
	if got0.Label != "S:node-A" {
		t.Errorf("label0 = %q, want S:node-A", got0.Label)
	}
	if got1.Label != "S:server-2" {
		t.Errorf("label1 = %q, want S:server-2 (no fragment fallback)", got1.Label)
	}
	if got0.Enabled || got1.Enabled {
		t.Errorf("disabled=true should map to enabled=false")
	}
}

// TestMigrate_Mixed — source + connections в одном ProxySource → emit оба.
func TestMigrate_Mixed(t *testing.T) {
	old := &V4File{
		Version: 4,
		ParserConfig: V4ParserConfig{
			Proxies: []configtypes.ProxySource{{
				Source:      "https://example.com/sub",
				Connections: []string{"vless://uuid@host:443#manual"},
				TagPrefix:   "M:",
			}},
		},
	}
	state := MigrateV4ToV5(old, fixedIDGen())
	if len(state.Connections.Sources) != 2 {
		t.Fatalf("got %d, want 2 (subscription + server)", len(state.Connections.Sources))
	}
	if state.Connections.Sources[0].Type != SourceTypeSubscription {
		t.Errorf("[0] should be subscription")
	}
	if state.Connections.Sources[1].Type != SourceTypeServer {
		t.Errorf("[1] should be server")
	}
}

// TestMigrate_Idempotent — миграция одной и той же v4-фикстуры дважды
// (с одинаковым gen) даёт идентичный output.
func TestMigrate_Idempotent(t *testing.T) {
	old := &V4File{
		Version: 4,
		ParserConfig: V4ParserConfig{
			Proxies: []configtypes.ProxySource{{Source: "https://example.com/sub"}},
		},
	}
	a := MigrateV4ToV5(old, fixedIDGen())
	b := MigrateV4ToV5(old, fixedIDGen())

	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Errorf("non-deterministic:\n%s\n!=\n%s", ja, jb)
	}
}

// TestMigrate_NilInput — nil input → nil output, no panic.
func TestMigrate_NilInput(t *testing.T) {
	if got := MigrateV4ToV5(nil, fixedIDGen()); got != nil {
		t.Errorf("MigrateV4ToV5(nil) = %+v, want nil", got)
	}
}

// TestExtractFragment — corner cases для парсера URL fragment.
func TestExtractFragment(t *testing.T) {
	cases := map[string]string{
		"vless://uuid@host:443#hello":           "hello",
		"wireguard://x@y:51820#wg-parnas":       "wg-parnas",
		"vless://uuid@host:443#hello%20world":   "hello world",
		"vless://uuid@host:443":                 "",
		"plain":                                 "",
		"#":                                     "",
		"":                                      "",
	}
	for in, want := range cases {
		got := extractFragment(in)
		if got != want {
			t.Errorf("extractFragment(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMakeULID_FormatAndUnique — sanity-проверка формата и уникальности.
func TestMakeULID_FormatAndUnique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := MakeULID()
		if len(id) != ulidLen {
			t.Fatalf("len(MakeULID) = %d, want %d (%q)", len(id), ulidLen, id)
		}
		// Все символы — Crockford alphabet.
		for _, ch := range id {
			if !isCrockfordChar(byte(ch)) {
				t.Fatalf("non-crockford char %q in %q", ch, id)
			}
		}
		if seen[id] {
			t.Fatalf("collision at iter %d: %q", i, id)
		}
		seen[id] = true
	}
}

func isCrockfordChar(b byte) bool {
	for i := 0; i < len(crockfordAlphabet); i++ {
		if crockfordAlphabet[i] == b {
			return true
		}
	}
	return false
}
