package state

// SPEC 118 (W1) — canonical v7 roundtrip и стабильность идентичности.
//
// Схема v7 (SPEC Т1): плоский корень sources[]/directions[]/rules[]/vars[]/
// dns_options/warp_accounts/meta; Save пишет только v7. Тесты фиксируют:
//
//  1. Load→Save→Load→Save сходится байт-в-байт (modulo meta.updated_at —
//     Save штампует текущее время всегда);
//  2. ULID папок/подписок стабильны через циклы мутаций и Save/Load; узлы
//     идентифицируются тегом (id у узлов нет — у мостовых верхних узлов
//     ULID живёт до W5);
//  3. загрузка v6-состояния (структурный перенос W1) тоже даёт стабильный
//     v7-roundtrip со второго Save.
//
// Фикстура testdata/v7_roundtrip.json — многосекционный v7 state: папка с
// узлами (server + auto) и replace, подписка с материализованными nodes[] и
// update_status, chain с NodeLink-хопами, верхний server с body/origin,
// directions, rules, vars, dns_options, warp_accounts. Регенерация:
// GEN_V7_ROUNDTRIP_FIXTURE=1 go test -run TestGenerateV7RoundtripFixture
// ./core/state/.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

const v7RoundtripFixture = "testdata/v7_roundtrip.json"

// v6RoundtripFixture — старая v6-фикстура; в W1 служит входом структурного
// переноса (полная миграция и её сценарии — волна W2).
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

// buildV7RoundtripFixture — State для v7-фикстуры (используется генератором).
func buildV7RoundtripFixture() *State {
	boolFalse := false
	s := New()
	s.Comment = "SPEC 118 v7 roundtrip fixture"
	s.Sources = []Source{
		{
			// Верхний server-узел: canonical body + origin, NodeLink-detour
			// на узел подписки.
			Node: Node{
				Kind:    SourceKindServer,
				Tag:     "🇯🇵 Tokyo",
				Enabled: true,
				Origin: &Origin{
					Kind: OriginKindURI,
					Raw:  "vless://uuid@h.example:443?security=reality&pbk=k#Tokyo",
				},
				Body:   json.RawMessage(`{"type":"vless","server":"h.example","server_port":443}`),
				Detour: &NodeLink{FolderID: "01J00000000000000000000SUB", Tag: "NL-1"},
			},
			ID: "01J00000000000000000000SRV",
		},
		{
			// Папка с узлами (server + auto), тег-политикой и свёрткой both.
			Node:      Node{Kind: SourceKindFolder, Enabled: true},
			ID:        "01J00000000000000000000FLD",
			Name:      "Личные",
			TagPolicy: &TagPolicy{Prefix: "[F] "},
			Nodes: []Node{
				{
					Kind:    SourceKindServer,
					Tag:     "DE-1",
					Enabled: true,
					Origin:  &Origin{Kind: OriginKindURI, Raw: "ss://Y2hhY2hh@de.example:8388#DE-1"},
					Body:    json.RawMessage(`{"type":"shadowsocks","server":"de.example","server_port":8388}`),
				},
				{
					Kind:    SourceKindAuto,
					Tag:     "быстрые",
					Enabled: true,
					Group: &AutoGroup{
						GroupType: AutoGroupURLTest,
						Members:   []NodeLink{{Tag: "DE-1"}},
						Strategy:  AutoStrategy{Interval: "5m"},
					},
				},
			},
			Replace: &FolderReplace{
				Mode:     FolderReplaceBoth,
				Tag:      "личные",
				Strategy: &AutoStrategy{Mode: configtypes.AutoModeLeastTest},
			},
		},
		{
			// Подписка с материализованными nodes[] и update_status.
			Node:      Node{Kind: SourceKindSubscription, Enabled: true},
			ID:        "01J00000000000000000000SUB",
			Name:      "Proton NL",
			TagPolicy: &TagPolicy{Prefix: "[P] ", Postfix: " •"},
			Nodes: []Node{
				{
					Kind:    SourceKindServer,
					Tag:     "NL-1",
					Enabled: true,
					Origin:  &Origin{Kind: OriginKindURI, Raw: "vless://uuid@nl.example:443#NL-1"},
					Body:    json.RawMessage(`{"type":"vless","server":"nl.example","server_port":443}`),
				},
				{
					Kind:    SourceKindServer,
					Tag:     "DE-2",
					Enabled: false,
					Origin:  &Origin{Kind: OriginKindURI, Raw: "vless://uuid@de2.example:443#DE-2"},
					Body:    json.RawMessage(`{"type":"vless","server":"de2.example","server_port":443}`),
				},
				{
					Kind:    SourceKindAuto,
					Tag:     "Auto",
					Enabled: true,
					Origin:  &Origin{Kind: OriginKindJSON, Raw: `{"type":"urltest","tag":"Auto"}`},
					Group: &AutoGroup{
						GroupType: AutoGroupURLTest,
						Members:   []NodeLink{{Tag: "NL-1"}, {Tag: "DE-2"}},
					},
				},
			},
			URL:      "https://example.invalid/sub?token=abc&kind=all",
			Skip:     []map[string]string{{"tag": "/(RU)/i"}},
			MaxNodes: 500,
			Update:   &UpdateSpec{IntervalHours: 6, AutoRefresh: &boolFalse},
			Meta: &SubMeta{
				ProfileTitle: "Proton",
				UserInfo:     &UserInfo{UploadBytes: 10, DownloadBytes: 20, TotalBytes: 100},
			},
			UpdateStatus: &SubUpdateStatus{
				URLAtFetch:        "https://example.invalid/sub?token=abc&kind=all",
				LastAttemptAt:     "2026-08-01T00:00:00Z",
				LastSuccessAt:     "2026-08-01T00:00:00Z",
				LastStatus:        "ok",
				NodesCountFetched: 3,
				Warnings: []FetchWarning{
					{Kind: "skip", Count: 2},
				},
			},
		},
		{
			// Цепочка с NodeLink-хопами: ближний хоп первым.
			Node: Node{
				Kind:    SourceKindChain,
				Tag:     "chain-1",
				Enabled: true,
				Hops: []NodeLink{
					{Tag: "🇯🇵 Tokyo"},
					{FolderID: "01J00000000000000000000SUB", Tag: "NL-1"},
				},
			},
			ID: "01J0000000000000000000CHN0",
		},
	}
	s.Directions = []configtypes.Direction{
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
	s.Defaults = Defaults{Reload: "4h", MaxNodes: DefaultMaxNodes}
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
	s.WarpAccounts = &WarpAccountsSection{
		WG: &WarpWGAccount{
			PrivateKey: "priv",
			PeerPublic: "pub",
			ClientV4:   "172.16.0.2",
			ClientV6:   "fd00::2",
			CreatedAt:  "2026-08-01T00:00:00Z",
		},
	}
	return s
}

// TestGenerateV7RoundtripFixture — генератор фикстуры; запускается только
// вручную (GEN_V7_ROUNDTRIP_FIXTURE=1). Штампует фиксированный updated_at,
// чтобы файл в testdata не дрейфовал.
func TestGenerateV7RoundtripFixture(t *testing.T) {
	if os.Getenv("GEN_V7_ROUNDTRIP_FIXTURE") != "1" {
		t.Skip("generator: set GEN_V7_ROUNDTRIP_FIXTURE=1 to (re)write the fixture")
	}
	dir := t.TempDir()
	tmp := filepath.Join(dir, "state.json")
	if err := buildV7RoundtripFixture().Save(tmp); err != nil {
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
	if err := os.WriteFile(v7RoundtripFixture, bytes.Join(lines, []byte("\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCanonical_LoadSaveLoadSave_ByteIdentical — SPEC 118 Т1 / §4.H:
// Load(f) → Save(p1) → Load(p1) → Save(p2): p1 == p2 байт-в-байт (modulo
// meta.updated_at), а p1 относительно фикстуры отличается ТОЛЬКО updated_at.
func TestCanonical_LoadSaveLoadSave_ByteIdentical(t *testing.T) {
	fixtureBytes, err := os.ReadFile(v7RoundtripFixture)
	if err != nil {
		t.Fatalf("fixture missing (regenerate with GEN_V7_ROUNDTRIP_FIXTURE=1): %v", err)
	}

	dir := t.TempDir()
	p1 := filepath.Join(dir, "p1.json")
	p2 := filepath.Join(dir, "p2.json")

	s1, err := Load(v7RoundtripFixture)
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

// TestCanonical_V6StructuralTransfer_RoundtripStable — SPEC 118 W1: загрузка
// v6-состояния (структурный перенос, без семантической миграции) даёт
// стабильный v7-roundtrip: Save(p1) → Load(p1) → Save(p2), p1 == p2. Легаси-
// поля (fold, disabled_nodes, тройня, локальные outbounds, defaults) не
// теряются — они едут в мостовых деривативах TEMPORARY BRIDGE до волны W2.
func TestCanonical_V6StructuralTransfer_RoundtripStable(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "p1.json")
	p2 := filepath.Join(dir, "p2.json")

	s1, err := Load(v6RoundtripFixture)
	if err != nil {
		t.Fatalf("Load v6 fixture: %v", err)
	}
	if err := s1.Save(p1); err != nil {
		t.Fatalf("Save p1: %v", err)
	}
	s2, err := Load(p1)
	if err != nil {
		t.Fatalf("Load p1: %v", err)
	}
	if s2.Version != SchemaVersionV7 {
		t.Fatalf("после Save версия обязана быть v7, got %d", s2.Version)
	}
	if err := s2.Save(p2); err != nil {
		t.Fatalf("Save p2: %v", err)
	}
	p1b, _ := os.ReadFile(p1)
	p2b, _ := os.ReadFile(p2)
	if !bytes.Equal(normalizeUpdatedAt(t, p1b), normalizeUpdatedAt(t, p2b)) {
		t.Errorf("v6→v7 transfer: p1 != p2 (beyond updated_at)\n--- p1 ---\n%s\n--- p2 ---\n%s", p1b, p2b)
	}

	// Структурный перенос без потерь: мостовые поля на месте.
	var sub *Source
	for i := range s2.Sources {
		if s2.Sources[i].Kind == SourceKindSubscription {
			sub = &s2.Sources[i]
		}
	}
	if sub == nil {
		t.Fatal("подписка не доехала до v7-формы")
	}
	if sub.Fold == nil || len(sub.DisabledNodes) != 2 || sub.TagPolicy == nil || len(sub.Outbounds) != 1 {
		t.Errorf("легаси-поля подписки потеряны структурным переносом: %+v", sub)
	}
	if s2.Defaults.Reload != "4h" {
		t.Errorf("defaults не доехали: %+v", s2.Defaults)
	}
}

// TestCanonical_IDStability — §4.H: ULID папок/подписок (и мостовых верхних
// узлов) неизменен через циклы mutate → Save → Load; ни один Save не выдаёт
// новых ULID существующим источникам.
func TestCanonical_IDStability(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	src, err := os.ReadFile(v7RoundtripFixture)
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
	for _, x := range s.Sources {
		if x.ID == "" {
			t.Fatalf("fixture source without ULID: %+v", x)
		}
		wantIDs[x.ID] = true
	}
	wantCount := len(s.Sources)

	// Мутации canonical по циклам: правка URL/имени, toggle узла подписки,
	// правка replace, reorder. Каждый цикл — Save → Load с диска.
	for cycle := 0; cycle < 4; cycle++ {
		for i := range s.Sources {
			x := &s.Sources[i]
			switch x.Kind {
			case SourceKindSubscription:
				// Смена адреса подписки не должна выдавать новый ULID.
				x.URL = "https://example.invalid/sub-v" + string(rune('2'+cycle))
				x.Name = "renamed"
				if len(x.Nodes) > 0 {
					x.Nodes[0].Enabled = cycle%2 == 0
				}
			case SourceKindServer:
				x.Enabled = cycle%2 == 0
			case SourceKindFolder:
				if x.Replace != nil {
					x.Replace.Mode = FolderReplaceManual
					x.Replace.Strategy = nil
				}
			case SourceKindChain:
				x.Hops = append([]NodeLink(nil), x.Hops...)
			}
		}
		if cycle == 2 {
			// Reorder: порядок — пользовательская правка, идентичность
			// не должна от него зависеть.
			n := len(s.Sources)
			rev := make([]Source, 0, n)
			for i := n - 1; i >= 0; i-- {
				rev = append(rev, s.Sources[i])
			}
			s.Sources = rev
		}
		if err := s.Save(path); err != nil {
			t.Fatalf("Save cycle %d: %v", cycle, err)
		}
		s, err = Load(path)
		if err != nil {
			t.Fatalf("Load cycle %d: %v", cycle, err)
		}

		if len(s.Sources) != wantCount {
			t.Fatalf("cycle %d: source count drifted: %d → %d", cycle, wantCount, len(s.Sources))
		}
		got := map[string]bool{}
		for _, x := range s.Sources {
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
	for _, x := range s.Sources {
		if x.Kind == SourceKindSubscription && x.Name != "renamed" {
			t.Errorf("subscription mutation lost after cycles: %+v", x)
		}
	}
}
