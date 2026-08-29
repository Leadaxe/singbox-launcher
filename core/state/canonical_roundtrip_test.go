package state

// SPEC 117 §5.B (W4) — canonical roundtrip и стабильность идентичности.
//
// Обратный синк Save (sync_to_connections.go) удалён: Save сериализует ровно
// s.Connections, ничего не пересобирая. Эти тесты фиксируют следствия:
//
//  1. Load→Save→Load→Save сходится байт-в-байт (modulo meta.updated_at —
//     Save штампует текущее время всегда);
//  2. ULID источников не меняются и не пересоздаются ни на одном Save —
//     ID живёт в canonical с момента создания Source (риск Р3).
//
// Фикстура testdata/v6_roundtrip.json — многосекционный v6 state: подписка
// (Skip/Tag/локальные Outbounds/Fold/Update/MaxNodes/Meta/DisabledNodes),
// server-URI с node-detour ссылкой, server с ручным config_json, цепочка,
// глобальные Направления (ref/updates/auto), defaults, rules, vars,
// dns_options. Регенерация: GEN_V6_ROUNDTRIP_FIXTURE=1 go test -run
// TestGenerateV6RoundtripFixture ./core/state/.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

const v6RoundtripFixture = "testdata/v6_roundtrip.json"

// fixedUpdatedAtLine — плейсхолдер для сравнения «всё, кроме updated_at».
const fixedUpdatedAtLine = `    "updated_at": "<normalized>",`

// normalizeUpdatedAt заменяет единственную строку meta.updated_at
// плейсхолдером. Требует ровно одного вхождения — второй ключ updated_at в
// файле означал бы дрейф схемы, и тест обязан упасть.
func normalizeUpdatedAt(t *testing.T, data []byte) []byte {
	t.Helper()
	lines := bytes.Split(data, []byte("\n"))
	found := 0
	for i, ln := range lines {
		if bytes.Contains(ln, []byte(`"updated_at":`)) {
			lines[i] = []byte(fixedUpdatedAtLine)
			found++
		}
	}
	if found != 1 {
		t.Fatalf("expected exactly 1 updated_at line, got %d", found)
	}
	return bytes.Join(lines, []byte("\n"))
}

// buildV6RoundtripFixture — State для фикстуры (используется генератором).
func buildV6RoundtripFixture() *State {
	boolFalse := false
	s := New()
	s.Comment = "SPEC 117 roundtrip fixture"
	s.Connections.Sources = []Source{
		{
			ID:      "01J00000000000000000000SUB",
			Type:    SourceTypeSubscription,
			Enabled: true,
			Label:   "Proton NL",
			URL:     "https://example.invalid/sub?token=abc&kind=all",
			Skip:    []map[string]string{{"tag": "/(RU)/i"}},
			Tag:     &TagSpec{Prefix: "[P] ", Postfix: " •"},
			Outbounds: []configtypes.Direction{
				{
					Tag:     "local-video",
					Type:    "selector",
					Filters: map[string]interface{}{"tag": "/(NL|DE)/"},
					Auto:    &configtypes.DirectionAuto{Mode: configtypes.AutoModeLeastTest},
				},
			},
			// ExposeGroupTagsToGlobal при живом Fold не пишется: миграция
			// SPEC 108 выражает флаг свёрткой и гасит его на Load.
			Fold: &configtypes.SourceFold{
				Mode: configtypes.FoldModeSelectAuto,
				Auto: &configtypes.DirectionAuto{Interval: "5m"},
			},
			Update:   &UpdateSpec{IntervalHours: 6, AutoRefresh: &boolFalse},
			MaxNodes: 500,
			Meta: &SubscriptionMeta{
				ProfileTitle:      "Proton",
				LastFetchedAt:     "2026-08-01T00:00:00Z",
				LastStatus:        "ok",
				NodesCountFetched: 42,
				PreviewNodes:      []string{"🇳🇱 NL-1", "🇩🇪 DE-2"},
				UserInfo:          &UserInfo{UploadBytes: 10, DownloadBytes: 20, TotalBytes: 100},
			},
			DisabledNodes: map[string]int64{
				"🇩🇪 DE-2":  1750000000,
				"🇳🇱 NL-11": 1750000001,
			},
		},
		{
			ID:                 "01J00000000000000000000SRV",
			Type:               SourceTypeServer,
			Enabled:            true,
			Label:              "Tokyo",
			NodeTag:            "🇯🇵 Tokyo",
			URI:                "vless://uuid@h.example:443?security=reality&pbk=k#Tokyo",
			DetourNodeSourceID: "01J00000000000000000000SUB",
			DetourNodeTag:      "NL-1",
			DetourNodeLabel:    "🇳🇱 NL-1",
		},
		{
			ID:         "01J0000000000000000000JSON",
			Type:       SourceTypeServer,
			Enabled:    false,
			Label:      "manual-node",
			NodeTag:    "manual-node",
			ConfigJSON: json.RawMessage(`{"type":"someproto","server":"10.0.0.1"}`),
		},
		{
			ID:      "01J0000000000000000000CHN0",
			Type:    SourceTypeChain,
			Enabled: true,
			Label:   "Двойной хоп",
			NodeTag: "chain-1",
			Chain: &configtypes.SourceChain{
				Hops:         []string{"🇯🇵 Tokyo", "NL-1"},
				IdleTimeout:  "2m",
				StripEvasion: &boolFalse,
				Strip:        map[string]bool{configtypes.ChainStripTLSUTLS: false},
			},
		},
	}
	s.Connections.Outbounds = []configtypes.Direction{
		{
			Tag:  "proxy-out",
			Ref:  configtypes.RefTemplate,
			Auto: &configtypes.DirectionAuto{Mode: configtypes.AutoModeLeastTest},
			Updates: []configtypes.OutboundUpdate{
				{Ref: configtypes.RefUser, Patch: map[string]interface{}{
					"filters": map[string]interface{}{"tag": "!/(🇷🇺)/i"},
				}, Explicit: true},
			},
		},
		{
			Tag:     "video-out",
			Type:    "selector",
			Filters: map[string]interface{}{"tag": "/(NL)/"},
		},
	}
	s.Connections.Defaults = Defaults{Reload: "4h", MaxNodes: DefaultMaxNodes}
	s.Rules = []Rule{
		{Kind: RuleKindPreset, Ref: "ru-direct", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
		{Kind: RuleKindInline, Enabled: true,
			Body: json.RawMessage(`{"name":"X","match":{"port":[443]},"outbound":"proxy-out"}`)},
	}
	s.Vars = []SettingVar{{Name: "log_level", Value: "warn"}}
	s.DNS = DNSOptions{
		Strategy: "prefer_ipv4",
		Final:    "google_doh",
		Servers: []DNSServer{
			{Kind: DNSServerKindTemplate, Tag: "cloudflare_udp", Enabled: true},
		},
	}
	return s
}

// TestGenerateV6RoundtripFixture — генератор фикстуры; запускается только
// вручную (GEN_V6_ROUNDTRIP_FIXTURE=1). Штампует фиксированный updated_at,
// чтобы файл в testdata не дрейфовал.
func TestGenerateV6RoundtripFixture(t *testing.T) {
	if os.Getenv("GEN_V6_ROUNDTRIP_FIXTURE") != "1" {
		t.Skip("generator: set GEN_V6_ROUNDTRIP_FIXTURE=1 to (re)write the fixture")
	}
	dir := t.TempDir()
	tmp := filepath.Join(dir, "state.json")
	if err := buildV6RoundtripFixture().Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	// Фиксированные timestamps: created_at выставляет Save (zero → now),
	// updated_at Save штампует всегда — нормализуем оба на константу,
	// сохраняя хвост строки (запятая есть/нет — решает позиция ключа).
	fixLine := func(ln []byte, key string) []byte {
		prefix := `    "` + key + `": "`
		if !bytes.HasPrefix(ln, []byte(prefix)) {
			return ln
		}
		suffix := ""
		if bytes.HasSuffix(ln, []byte(",")) {
			suffix = ","
		}
		return []byte(prefix + "2026-08-01T00:00:00Z\"" + suffix)
	}
	lines := bytes.Split(data, []byte("\n"))
	for i, ln := range lines {
		lines[i] = fixLine(fixLine(ln, "created_at"), "updated_at")
	}
	if err := os.WriteFile(v6RoundtripFixture, bytes.Join(lines, []byte("\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCanonical_LoadSaveLoadSave_ByteIdentical — SPEC 117 §5.B:
// Load(f) → Save(p1) → Load(p1) → Save(p2): p1 == p2 байт-в-байт (modulo
// meta.updated_at), а p1 относительно фикстуры отличается ТОЛЬКО updated_at.
// Никакой пересборки Connections на Save больше нет — состав, порядок,
// Meta/Fold/DisabledNodes/локальные Outbounds доезжают как есть.
func TestCanonical_LoadSaveLoadSave_ByteIdentical(t *testing.T) {
	fixtureBytes, err := os.ReadFile(v6RoundtripFixture)
	if err != nil {
		t.Fatalf("fixture missing (regenerate with GEN_V6_ROUNDTRIP_FIXTURE=1): %v", err)
	}

	dir := t.TempDir()
	p1 := filepath.Join(dir, "p1.json")
	p2 := filepath.Join(dir, "p2.json")

	s1, err := Load(v6RoundtripFixture)
	if err != nil {
		t.Fatalf("Load fixture: %v", err)
	}
	if err := s1.Save(p1); err != nil {
		t.Fatalf("Save p1: %v", err)
	}
	s2, err := Load(p1)
	if err != nil {
		t.Fatalf("Load p1: %v", err)
	}
	if err := s2.Save(p2); err != nil {
		t.Fatalf("Save p2: %v", err)
	}

	p1b, err := os.ReadFile(p1)
	if err != nil {
		t.Fatal(err)
	}
	p2b, err := os.ReadFile(p2)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(normalizeUpdatedAt(t, p1b), normalizeUpdatedAt(t, p2b)) {
		t.Errorf("p1 != p2 (beyond updated_at): save is not idempotent\n--- p1 ---\n%s\n--- p2 ---\n%s", p1b, p2b)
	}
	if !bytes.Equal(normalizeUpdatedAt(t, fixtureBytes), normalizeUpdatedAt(t, p1b)) {
		t.Errorf("p1 differs from fixture beyond updated_at\n--- fixture ---\n%s\n--- p1 ---\n%s", fixtureBytes, p1b)
	}
}

// TestCanonical_IDStability — SPEC 117 §5.B / риск Р3: ULID каждого Source
// неизменен через циклы mutate-canonical → Save → Load; ни один Save не
// выдаёт новых ULID существующим источникам (раньше это гарантировал матчинг
// обратного синка; теперь ID живёт в canonical и не пересоздаётся вовсе).
func TestCanonical_IDStability(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	src, err := os.ReadFile(v6RoundtripFixture)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantIDs := map[string]bool{}
	for _, x := range s.Connections.Sources {
		if x.ID == "" {
			t.Fatalf("fixture source without ULID: %+v", x)
		}
		wantIDs[x.ID] = true
	}
	wantCount := len(s.Connections.Sources)

	// Мутации canonical по циклам: правка URL/подписи, toggle, правка
	// цепочки, reorder. Каждый цикл — Save → Load с диска.
	for cycle := 0; cycle < 4; cycle++ {
		for i := range s.Connections.Sources {
			x := &s.Connections.Sources[i]
			switch x.Type {
			case SourceTypeSubscription:
				// Смена адреса подписки не должна выдавать новый ULID
				// (до SPEC 112 это ломал матчинг по URL).
				x.URL = "https://example.invalid/sub-v" + string(rune('2'+cycle))
				x.Label = "renamed"
			case SourceTypeServer:
				x.Enabled = cycle%2 == 0
			case SourceTypeChain:
				x.Chain.IdleTimeout = "3m"
			}
		}
		if cycle == 2 {
			// Reorder: порядок — пользовательская правка, идентичность
			// не должна от него зависеть.
			n := len(s.Connections.Sources)
			rev := make([]Source, 0, n)
			for i := n - 1; i >= 0; i-- {
				rev = append(rev, s.Connections.Sources[i])
			}
			s.Connections.Sources = rev
		}
		if err := s.Save(path); err != nil {
			t.Fatalf("Save cycle %d: %v", cycle, err)
		}
		s, err = Load(path)
		if err != nil {
			t.Fatalf("Load cycle %d: %v", cycle, err)
		}

		if len(s.Connections.Sources) != wantCount {
			t.Fatalf("cycle %d: source count drifted: %d → %d", cycle, wantCount, len(s.Connections.Sources))
		}
		got := map[string]bool{}
		for _, x := range s.Connections.Sources {
			got[x.ID] = true
		}
		for id := range wantIDs {
			if !got[id] {
				t.Errorf("cycle %d: ULID %s lost", cycle, id)
			}
		}
		for id := range got {
			if !wantIDs[id] {
				t.Errorf("cycle %d: Save issued a NEW ULID %s to an existing source", cycle, id)
			}
		}
	}

	// Мутации доехали (Save действительно сохраняет canonical-правки).
	for _, x := range s.Connections.Sources {
		if x.Type == SourceTypeSubscription && x.Label != "renamed" {
			t.Errorf("subscription mutation lost after cycles: %+v", x)
		}
	}
}
