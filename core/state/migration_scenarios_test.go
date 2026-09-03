// Миграционные сценарии SPEC 118 §4.B (1–9; п.10 — волна W4).
//
// Внешний пакет state_test: материализация nodes[] требует хуков
// state.MigrationHooks, которые подставляет init пакета core/config —
// внутренний тест пакета state не может его импортировать (цикл).
package state_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
)

// rawBody — тело подписки фикстур: дубль тега (NL-1 дважды → NL-1-2),
// байтовый дубль (NL-1-copy схлопывается дедупом), RU-1 отсекается skip.
const rawBody = `vless://11111111-1111-4111-8111-111111111111@nl.example:443?security=tls&sni=nl.example&type=ws&path=/ws#NL-1
vless://22222222-2222-4222-8222-222222222222@de2.example:443?security=tls&sni=de2.example#DE-2
ss://YWVzLTEyOC1nY206cGFzc3dvcmQ=@nl3.example:8388#NL-1
ss://YWVzLTEyOC1nY206cGFzc3dvcmQ=@nl3.example:8388#NL-1-copy
trojan://pw@ru.example:443?sni=ru.example#RU-1
`

const subID = "01J00000000000000000000SUB"
const sub2ID = "01J00000000000000000000SU2"

const nl1URI = "vless://11111111-1111-4111-8111-111111111111@nl.example:443?security=tls&sni=nl.example&type=ws&path=/ws#NL-1"
const tokyoURI = "vless://33333333-3333-4333-8333-333333333333@h.example:443?security=tls&sni=h.example#Tokyo"

// mainFixture — основная v6-фикстура: подписка с политикой тегов, skip,
// fold select_auto, disabled-картой (сырой тег + legacy-hex %HEX%), вторая
// подписка без кэша с exclude, server с URI и тройней на узел подписки,
// chain с хопами на верхний узел / узел подписки / Направление / fold-тег /
// призрак, произвольное локальное Направление, правила и ссылки на
// fold-теги.
const mainFixture = `{
  "meta": {"version": 6, "schema": "presets_v1", "created_at": "2026-08-01T00:00:00Z", "updated_at": "2026-08-01T00:00:00Z"},
  "connections": {
    "sources": [
      {
        "id": "` + subID + `",
        "type": "subscription",
        "enabled": true,
        "label": "Proton NL",
        "url": "https://example.invalid/sub",
        "skip": [{"tag": "/(RU)/i"}],
        "tag": {"prefix": "[P] ", "postfix": " •"},
        "outbounds": [{"tag": "local-video", "type": "selector", "filters": {"tag": "/(NL)/"}}],
        "fold": {"mode": "select_auto", "auto": {"interval": "5m"}},
        "max_nodes": 500,
        "meta": {"profile_title": "Proton", "last_fetched_at": "2026-08-01T00:00:00Z", "last_status": "ok", "nodes_count_fetched": 3},
        "disabled_nodes": {"DE-2": 1750000000, "%HEX%": 1750000001, "ghost-mark": 1750000002}
      },
      {
        "id": "` + sub2ID + `",
        "type": "subscription",
        "enabled": true,
        "label": "No cache",
        "url": "https://example.invalid/sub2",
        "exclude_from_global": true,
        "disabled_nodes": {"orphan": 1750000000}
      },
      {
        "id": "01J00000000000000000000SRV",
        "type": "server",
        "enabled": true,
        "label": "Tokyo",
        "node_tag": "Tokyo",
        "uri": "` + tokyoURI + `",
        "detour_node_source_id": "` + subID + `",
        "detour_node_tag": "NL-1"
      },
      {
        "id": "01J0000000000000000000TRNS",
        "type": "server",
        "enabled": true,
        "label": "Transitional",
        "node_tag": "Transitional",
        "uri": "vless://44444444-4444-4444-8444-444444444444@t.example:443?security=tls&sni=t.example#T",
        "detour_node_tag": "Tokyo"
      },
      {
        "id": "01J0000000000000000000CHN0",
        "type": "chain",
        "enabled": true,
        "label": "Двойной хоп",
        "node_tag": "chain-1",
        "chain": {"hops": ["Tokyo", "[P] NL-1 •", "video-out", "[P]auto", "ghost"], "idle_timeout": "2m"}
      }
    ],
    "direction_outbounds": [
      {"tag": "video-out", "type": "selector", "filters": {"tag": "/(NL)/"}}
    ],
    "defaults": {"reload": "4h", "max_nodes": 700}
  },
  "rules": [
    {"kind": "inline", "enabled": true, "body": {"name": "fold-target", "match": {"port": [8443]}, "outbound": "[P]select"}},
    {"kind": "inline", "enabled": true, "body": {"name": "fold-auto-target", "match": {"port": [8444]}, "outbound": "[P]auto"}}
  ],
  "vars": [
    {"name": "log_level", "value": "warn"},
    {"name": "route_final", "value": "[P]auto"}
  ],
  "dns_options": {
    "strategy": "prefer_ipv4",
    "servers": [
      {"kind": "user", "enabled": true, "tag": "spec-dns", "type": "udp", "server": "1.1.1.1", "detour": "[P]auto"}
    ]
  }
}`

// legacyHashForURI — упразднённый контент-хэш узла из URI (адрес отметок
// выключения SPEC 094/101).
func legacyHashForURI(t *testing.T, uri string) string {
	t.Helper()
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("parse uri: %v", err)
	}
	h := config.LegacyNodeIdentityHash(node)
	if h == "" {
		t.Fatal("empty legacy hash")
	}
	return h
}

// writeStateWithRaw разворачивает execDir-макет и возвращает путь state.json.
func writeStateWithRaw(t *testing.T, stateJSON string, raws map[string]string) string {
	t.Helper()
	execDir := t.TempDir()
	statesDir := filepath.Join(execDir, "bin", "wizard_states")
	subsDir := filepath.Join(execDir, "bin", "subscriptions")
	for _, d := range []string{statesDir, subsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	statePath := filepath.Join(statesDir, "state.json")
	if err := os.WriteFile(statePath, []byte(stateJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	for id, body := range raws {
		if err := os.WriteFile(filepath.Join(subsDir, id+".raw"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return statePath
}

func loadMainFixture(t *testing.T) (*state.State, string) {
	t.Helper()
	fixture := strings.Replace(mainFixture, "%HEX%", legacyHashForURI(t, nl1URI), 1)
	statePath := writeStateWithRaw(t, fixture, map[string]string{subID: rawBody})
	s, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Migration == nil {
		t.Fatal("migration report missing on legacy load")
	}
	return s, statePath
}

func findSource(t *testing.T, s *state.State, id string) *state.Source {
	t.Helper()
	src := s.FindSource(id)
	if src == nil {
		t.Fatalf("source %s not found", id)
	}
	return src
}

func hasWarningContaining(rep *state.MigrationReport, substr string) bool {
	for _, w := range rep.Warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// §4.B.1 — материализация: nodes[] из raw-кэша, body эквивалентен эмиссии
// старого парса; подписка без кэша — nodes[] пуст + warning.
func TestMigrationScenario1Materialization(t *testing.T) {
	s, _ := loadMainFixture(t)
	sub := findSource(t, s, subID)

	var tags []string
	for _, n := range sub.Nodes {
		tags = append(tags, n.Tag)
	}
	want := []string{"NL-1", "DE-2", "NL-1-2"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("materialized raw tags = %v, want %v (skip/дедуп/уникализация)", tags, want)
	}

	// Body первого узла эквивалентен эмиссии старого парса (минус tag/detour).
	node, err := subscription.ParseNode(nl1URI, nil)
	if err != nil {
		t.Fatal(err)
	}
	emitted, err := config.GenerateNodeJSONBare(node)
	if err != nil {
		t.Fatal(err)
	}
	var wantBody, gotBody map[string]interface{}
	if err := json.Unmarshal([]byte(emitted), &wantBody); err != nil {
		t.Fatal(err)
	}
	delete(wantBody, "tag")
	delete(wantBody, "detour")
	if err := json.Unmarshal(sub.Nodes[0].Body, &gotBody); err != nil {
		t.Fatalf("node body not json: %v", err)
	}
	if !reflect.DeepEqual(gotBody, wantBody) {
		t.Fatalf("materialized body != old-parser emission\n got: %v\nwant: %v", gotBody, wantBody)
	}
	if sub.Nodes[0].Origin == nil || sub.Nodes[0].Origin.Kind != state.OriginKindURI || sub.Nodes[0].Origin.Raw != nl1URI {
		t.Fatalf("origin потерян: %+v", sub.Nodes[0].Origin)
	}

	// Подписка без кэша: nodes[] пуст + предупреждение.
	sub2 := findSource(t, s, sub2ID)
	if len(sub2.Nodes) != 0 {
		t.Fatalf("sub2 nodes = %d, want 0", len(sub2.Nodes))
	}
	if !hasWarningContaining(s.Migration, "No cache") {
		t.Fatalf("нет предупреждения об отсутствии кэша: %v", s.Migration.Warnings)
	}
	// Отметка выключения материализовать было не к чему, но выбрасывать её
	// нельзя: узлы появятся первым fetch'ем, и отметка обязана дожить до него
	// в PendingDisabled (вердикт O2) — ровно как у импорта бэкапа.
	if !containsString(sub2.PendingDisabled, "orphan") {
		t.Fatalf("отметка потеряна вместо PendingDisabled: %v", sub2.PendingDisabled)
	}
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// §4.B.2 — отметки: enabled=false по сырым тегам и legacy-hex; несматченные
// ключи — в отчёте; мостовая карта не двоит ключей.
func TestMigrationScenario2DisabledMarks(t *testing.T) {
	s, _ := loadMainFixture(t)
	sub := findSource(t, s, subID)

	enabledByTag := map[string]bool{}
	for _, n := range sub.Nodes {
		enabledByTag[n.Tag] = n.Enabled
	}
	if enabledByTag["DE-2"] {
		t.Error("DE-2: отметка по сырому тегу не применена")
	}
	if enabledByTag["NL-1"] {
		t.Error("NL-1: отметка по legacy-hex не докручена")
	}
	if !enabledByTag["NL-1-2"] {
		t.Error("NL-1-2 выключен без отметки")
	}
	if !hasWarningContaining(s.Migration, "ghost-mark") {
		t.Errorf("несматченный ключ не в отчёте: %v", s.Migration.Warnings)
	}

	// SPEC 118 W5: сборочная форма несёт отметки ТОЛЬКО каноном
	// (nodes[].enabled) — второго представления не существует.
	ps := sub.ToProxySourceV4()
	if ps.Canonical == nil {
		t.Fatal("канонической проекции нет — собирать источник не из чего")
	}
	canonEnabled := map[string]bool{}
	for _, n := range ps.Canonical.Nodes {
		canonEnabled[n.Tag] = n.Enabled
	}
	if canonEnabled["DE-2"] {
		t.Error("проекция потеряла отметку DE-2")
	}
	if canonEnabled["NL-1"] {
		t.Error("проекция не получила переписанный hex → NL-1")
	}
	if !canonEnabled["NL-1-2"] {
		t.Error("NL-1-2 выключен в проекции без отметки")
	}
}

// §4.B.3 — теги: NodeTag → Node.Tag; mask-шаблон подписки — warning;
// переменные prefix/postfix живут.
func TestMigrationScenario3Tags(t *testing.T) {
	s, _ := loadMainFixture(t)
	if got := findSource(t, s, "01J00000000000000000000SRV").Node.Tag; got != "Tokyo" {
		t.Errorf("server Node.Tag = %q, want Tokyo", got)
	}
	if got := findSource(t, s, "01J0000000000000000000CHN0").Node.Tag; got != "chain-1" {
		t.Errorf("chain Node.Tag = %q, want chain-1", got)
	}
	sub := findSource(t, s, subID)
	if sub.TagPolicy == nil || sub.TagPolicy.Prefix != "[P] " || sub.TagPolicy.Postfix != " •" {
		t.Errorf("tag policy потеряна: %+v", sub.TagPolicy)
	}

	// Маска-шаблон подписки — предупреждение (фича is gone).
	maskFixture := `{
	  "meta": {"version": 6, "created_at": "2026-08-01T00:00:00Z", "updated_at": "2026-08-01T00:00:00Z"},
	  "connections": {"sources": [
	    {"id": "` + subID + `", "type": "subscription", "enabled": true, "label": "Masked",
	     "url": "https://example.invalid/sub", "tag": {"mask": "{$num} {$server}"}}
	  ]},
	  "rules": []
	}`
	statePath := writeStateWithRaw(t, maskFixture, map[string]string{subID: rawBody})
	ms, err := state.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !ms.Migration.HasWarnings() || !hasWarningContaining(ms.Migration, "tag mask") {
		t.Errorf("mask-шаблон подписки без предупреждения: %v", ms.Migration.Warnings)
	}
}

// §4.B.4 — хопы: NodeLink с folderId для узлов подписки; Направление и
// fold-тег легальны; нерезолвнутый — NodeLink{"", тег} + warning.
func TestMigrationScenario4ChainHops(t *testing.T) {
	s, _ := loadMainFixture(t)
	chain := findSource(t, s, "01J0000000000000000000CHN0")
	want := []state.NodeLink{
		{Tag: "Tokyo"},
		{FolderID: subID, Tag: "NL-1"},
		{Tag: "video-out"},
		{Tag: "[P]select-auto"}, // "[P]auto" переписан миграцией fold both (Р2)
		{Tag: "ghost"},
	}
	if !reflect.DeepEqual(chain.Node.Hops, want) {
		t.Fatalf("hops = %+v, want %+v", chain.Node.Hops, want)
	}
	if !hasWarningContaining(s.Migration, "ghost") {
		t.Errorf("висячий хоп без предупреждения: %v", s.Migration.Warnings)
	}
}

// §4.B.5 — detour: тройня обоих видов + переходная форма; NodeLink в каноне,
// мост отдаёт прежнюю пару.
func TestMigrationScenario5Detour(t *testing.T) {
	s, _ := loadMainFixture(t)

	srv := findSource(t, s, "01J00000000000000000000SRV")
	if srv.Node.Detour == nil || srv.Node.Detour.FolderID != subID || srv.Node.Detour.Tag != "NL-1" {
		t.Fatalf("тройня на узел подписки → %+v", srv.Node.Detour)
	}
	// SPEC 118 W5: ссылку в сборку везёт ТОЛЬКО канон (NodeLink) — тройни
	// в сборочной форме больше нет.
	ps := srv.ToProxySourceV4()
	if ps.Canonical == nil || len(ps.Canonical.Nodes) != 1 {
		t.Fatalf("канонической проекции узла нет: %+v", ps.Canonical)
	}
	if d := ps.Canonical.Nodes[0].Detour; d == nil || d.FolderID != subID || d.Tag != "NL-1" {
		t.Errorf("проекция потеряла ссылку detour: %+v", d)
	}

	trans := findSource(t, s, "01J0000000000000000000TRNS")
	if trans.Node.Detour == nil || trans.Node.Detour.FolderID != "" || trans.Node.Detour.Tag != "Tokyo" {
		t.Fatalf("переходная форма без source_id → %+v", trans.Node.Detour)
	}

	// Коллизия тегов верхних узлов: уникализация + перепись ссылки по
	// source_id + предупреждение.
	collisionFixture := `{
	  "meta": {"version": 6, "created_at": "2026-08-01T00:00:00Z", "updated_at": "2026-08-01T00:00:00Z"},
	  "connections": {"sources": [
	    {"id": "01J000000000000000000000X1", "type": "server", "enabled": true, "label": "X",
	     "uri": "vless://55555555-5555-4555-8555-555555555555@x1.example:443?security=tls&sni=x1.example#X"},
	    {"id": "01J000000000000000000000X2", "type": "server", "enabled": true, "label": "X",
	     "uri": "vless://66666666-6666-4666-8666-666666666666@x2.example:443?security=tls&sni=x2.example#X"},
	    {"id": "01J000000000000000000000SR3", "type": "server", "enabled": true, "label": "Y",
	     "uri": "vless://77777777-7777-4777-8777-777777777777@y.example:443?security=tls&sni=y.example#Y",
	     "detour_node_source_id": "01J000000000000000000000X2", "detour_node_tag": "X"}
	  ]},
	  "rules": []
	}`
	statePath := writeStateWithRaw(t, collisionFixture, nil)
	cs, err := state.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := findSource(t, cs, "01J000000000000000000000X2").Node.Tag; got != "X-2" {
		t.Fatalf("коллизия верхних узлов не уникализирована: %q", got)
	}
	y := findSource(t, cs, "01J000000000000000000000SR3")
	if y.Node.Detour == nil || y.Node.Detour.Tag != "X-2" || y.Node.Detour.FolderID != "" {
		t.Fatalf("ссылка по source_id не переписана на уникализированный тег: %+v", y.Node.Detour)
	}
	if !cs.Migration.HasWarnings() {
		t.Error("коллизия без предупреждения")
	}
}

// §4.B.6 — fold → replace: режимы 1:1, материализованный дериватив (включая
// позиционный `1:...` при пустом префиксе); правила на `[P]select` живут;
// both-ссылки переписаны на `<tag>-auto`.
func TestMigrationScenario6FoldToReplace(t *testing.T) {
	s, _ := loadMainFixture(t)
	sub := findSource(t, s, subID)
	if sub.Replace == nil || sub.Replace.Mode != state.FolderReplaceBoth || sub.Replace.Tag != "[P]select" {
		t.Fatalf("select_auto → %+v, want both/[P]select", sub.Replace)
	}
	if sub.Replace.Strategy == nil || sub.Replace.Strategy.Interval != "5m" {
		t.Fatalf("strategy потеряна: %+v", sub.Replace.Strategy)
	}

	// Ссылки на "[P]select" живы, "[P]auto" переписан.
	var foldTarget, foldAutoTarget string
	for _, r := range s.Rules {
		body, err := r.DecodeBody()
		if err != nil {
			continue
		}
		ib, ok := body.(*state.InlineBody)
		if !ok {
			continue
		}
		switch ib.Name {
		case "fold-target":
			foldTarget = ib.Outbound
		case "fold-auto-target":
			foldAutoTarget = ib.Outbound
		}
	}
	if foldTarget != "[P]select" {
		t.Errorf("правило на селектор сброшено: %q", foldTarget)
	}
	if foldAutoTarget != "[P]select-auto" {
		t.Errorf("правило на авто-группу не переписано: %q", foldAutoTarget)
	}
	for _, v := range s.Vars {
		if v.Name == "route_final" && v.Value != "[P]select-auto" {
			t.Errorf("route_final не переписан: %q", v.Value)
		}
	}
	for _, srv := range s.DNS.Servers {
		if srv.Tag == "spec-dns" {
			if det, _ := srv.Body["detour"].(string); det != "[P]select-auto" {
				t.Errorf("dns.detour не переписан: %q", det)
			}
		}
	}
	// SPEC 118 W5: свёртку в сборку везёт ТОЛЬКО канон (Replace с явным
	// тегом) — мостового Fold в сборочной форме больше нет.
	ps := sub.ToProxySourceV4()
	if ps.Canonical == nil || ps.Canonical.Replace == nil {
		t.Fatalf("канонической свёртки нет: %+v", ps.Canonical)
	}
	if ps.Canonical.Replace.Mode != configtypes.FolderReplaceBoth || ps.Canonical.Replace.Tag != "[P]select" {
		t.Errorf("проекция свёртки: %+v", ps.Canonical.Replace)
	}

	// Позиционный дериватив при пустом префиксе: подписка №1 → "1:select".
	positional := `{
	  "meta": {"version": 6, "created_at": "2026-08-01T00:00:00Z", "updated_at": "2026-08-01T00:00:00Z"},
	  "connections": {"sources": [
	    {"id": "` + subID + `", "type": "subscription", "enabled": true, "label": "P1",
	     "url": "https://example.invalid/sub", "fold": {"mode": "select"}},
	    {"id": "` + sub2ID + `", "type": "subscription", "enabled": true, "label": "P2",
	     "url": "https://example.invalid/sub2", "fold": {"mode": "auto", "auto": {"interval": "7m"}}}
	  ]},
	  "rules": [
	    {"kind": "inline", "enabled": true, "body": {"name": "pos", "match": {"port": [1]}, "outbound": "1:select"}}
	  ]
	}`
	statePath := writeStateWithRaw(t, positional, map[string]string{subID: rawBody, sub2ID: rawBody})
	psn, err := state.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	p1 := findSource(t, psn, subID)
	if p1.Replace == nil || p1.Replace.Mode != state.FolderReplaceManual || p1.Replace.Tag != "1:select" {
		t.Fatalf("позиционный селектор → %+v, want manual/1:select", p1.Replace)
	}
	p2 := findSource(t, psn, sub2ID)
	if p2.Replace == nil || p2.Replace.Mode != state.FolderReplaceAuto || p2.Replace.Tag != "2:auto" {
		t.Fatalf("позиционная авто-группа → %+v, want auto/2:auto", p2.Replace)
	}
	// Правило на "1:select" не тронуто (manual не переименовывает).
	body, _ := psn.Rules[0].DecodeBody()
	if ib, ok := body.(*state.InlineBody); !ok || ib.Outbound != "1:select" {
		t.Fatalf("правило на позиционный тег сброшено: %+v", body)
	}
}

// §4.B.7 — потери: exclude_from_global и произвольные локальные Направления
// — в едином списке предупреждений.
func TestMigrationScenario7Losses(t *testing.T) {
	s, _ := loadMainFixture(t)
	if !hasWarningContaining(s.Migration, "local-video") {
		t.Errorf("произвольное локальное Направление не в отчёте: %v", s.Migration.Warnings)
	}
	if !hasWarningContaining(s.Migration, "exclude from the global list") {
		t.Errorf("exclude_from_global не в отчёте: %v", s.Migration.Warnings)
	}
}

// §4.B.8 — снос и идемпотентность: файл переписан на v7 (шаг 8 включён в W5,
// migrationPurgesLegacy), бэкап-копия исходника лежит рядом, v7
// Save→Load→Save байт-в-байт, raw-кэш удалён, defaults уехали в настройки
// приложения, не перетирая явное.
func TestMigrationScenario8PurgeAndIdempotency(t *testing.T) {
	fixture := strings.Replace(mainFixture, "%HEX%", legacyHashForURI(t, nl1URI), 1)
	statePath := writeStateWithRaw(t, fixture, map[string]string{subID: rawBody})
	original, _ := os.ReadFile(statePath)

	s, err := state.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}

	// Файл переписан миграцией (шаг 8), а страховкой служит бэкап-копия
	// рядом: она обязана быть байт-в-байт исходником.
	after, _ := os.ReadFile(statePath)
	if bytes.Equal(original, after) {
		t.Fatal("миграция не переписала файл — состояние осталось легаси")
	}
	if !bytes.Contains(after, []byte(`"schema": "sources_v7"`)) {
		t.Fatalf("файл после миграции не в схеме v7:\n%s", after)
	}
	bak, err := os.ReadFile(statePath + ".v6.bak")
	if err != nil {
		t.Fatalf("бэкап-копия не записана: %v", err)
	}
	if !bytes.Equal(bak, original) {
		t.Fatal("бэкап-копия не равна исходнику")
	}

	// Повторный Load v7 — no-op: Save байт-в-байт (кроме meta.updated_at,
	// поэтому сравниваем два цикла Save→Load→Save).
	p1 := filepath.Join(t.TempDir(), "v7.json")
	if err := s.Save(p1); err != nil {
		t.Fatal(err)
	}
	s2, err := state.Load(p1)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Migration != nil {
		t.Fatal("v7-файл прошёл через миграцию повторно")
	}
	p2 := filepath.Join(filepath.Dir(p1), "v7-2.json")
	if err := s2.Save(p2); err != nil {
		t.Fatal(err)
	}
	b1, _ := os.ReadFile(p1)
	b2, _ := os.ReadFile(p2)
	norm := func(b []byte) []byte {
		var m map[string]json.RawMessage
		_ = json.Unmarshal(b, &m)
		var meta map[string]json.RawMessage
		_ = json.Unmarshal(m["meta"], &meta)
		delete(meta, "updated_at")
		mm, _ := json.Marshal(meta)
		m["meta"] = mm
		out, _ := json.Marshal(m)
		return out
	}
	if !bytes.Equal(norm(b1), norm(b2)) {
		t.Fatal("v7 roundtrip не байт-в-байт (сверх meta.updated_at)")
	}

	// Снос (шаг 8): raw-файл удалён, defaults уехали в настройки приложения.
	// Легаси-ПОЛЕЙ сносить нечего: их нет в типе Source (SPEC 118 W5) — они
	// живут только сайдкаром миграции и умирают вместе с ней.
	lc := state.DeriveLoadContextForTest(statePath)
	state.PurgeLegacyForTest(s, lc)
	if _, err := os.Stat(filepath.Join(lc.SubsDir, subID+".raw")); !os.IsNotExist(err) {
		t.Error("raw-кэш не удалён сносом")
	}
	settings := locale.LoadSettings(lc.BinDir)
	if settings.DefaultSubscriptionReload != "4h" || settings.DefaultSubscriptionMaxNodes != 700 {
		t.Errorf("defaults не переехали в настройки: %+v", settings)
	}

	// «Не перетирая явное»: второй перенос не перебивает выставленное.
	s.Defaults = state.Defaults{Reload: "9h", MaxNodes: 111}
	state.PurgeLegacyForTest(s, lc)
	settings = locale.LoadSettings(lc.BinDir)
	if settings.DefaultSubscriptionReload != "4h" || settings.DefaultSubscriptionMaxNodes != 700 {
		t.Errorf("явные настройки перетёрты повторным переносом: %+v", settings)
	}
}

// Шаг 8, обратная сторона: подписке, у которой материализация не дала ни
// одного узла, raw-кэш НЕ сносят. Тело больше взять неоткуда (URL мог
// умереть), и снос превратил бы «узлы появятся после обновления» в
// безвозвратную потерю.
func TestMigrationKeepsRawCacheWhenNothingMaterialized(t *testing.T) {
	fixture := strings.Replace(mainFixture, "%HEX%", legacyHashForURI(t, nl1URI), 1)
	statePath := writeStateWithRaw(t, fixture, map[string]string{
		subID: rawBody,
		// Тело, из которого парсер не соберёт ни одного узла: ни одной
		// известной схемы, ни base64-обёртки.
		sub2ID: "не ссылка и не конфиг\nтоже не ссылка\n",
	})
	s, err := state.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if sub2 := findSource(t, s, sub2ID); len(sub2.Nodes) != 0 {
		t.Fatalf("фикстура протухла: парсер собрал %d узлов из мусора", len(sub2.Nodes))
	}

	lc := state.DeriveLoadContextForTest(statePath)
	state.PurgeLegacyForTest(s, lc)
	if _, err := os.Stat(filepath.Join(lc.SubsDir, sub2ID+".raw")); err != nil {
		t.Errorf("raw-кэш неразобранной подписки снесён — тело не восстановить: %v", err)
	}
	// У материализованной подписки снос при этом отработал как прежде.
	if _, err := os.Stat(filepath.Join(lc.SubsDir, subID+".raw")); !os.IsNotExist(err) {
		t.Error("raw-кэш материализованной подписки не удалён")
	}
}

// §4.B.9 — сид: легаси-сид шаблона (пустые sources, направления шаблона)
// проходит тем же путём и даёт валидную v7-модель без потерь.
func TestMigrationScenario9Seed(t *testing.T) {
	seed := `{
	  "meta": {"version": 6, "created_at": "2026-08-01T00:00:00Z", "updated_at": "2026-08-01T00:00:00Z"},
	  "connections": {
	    "sources": [],
	    "direction_outbounds": [
	      {"tag": "proxy-out", "auto": {"mode": "least_test"}}
	    ]
	  },
	  "rules": []
	}`
	statePath := writeStateWithRaw(t, seed, nil)
	s, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("seed load: %v", err)
	}
	if s.Migration.HasWarnings() {
		t.Fatalf("сид без легаси-фич не должен терять ничего: %v", s.Migration.Warnings)
	}
	if len(s.Directions) != 1 || s.Directions[0].Tag != "proxy-out" {
		t.Fatalf("направления сида потеряны: %+v", s.Directions)
	}
	p := filepath.Join(t.TempDir(), "seed-v7.json")
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Load(p); err != nil {
		t.Fatalf("v7-модель сида не читается: %v", err)
	}
}

// §4.B.10 — rule-reset не стреляет: на МИГРИРОВАННОМ состоянии сборка эмитит
// ровно те теги, на которые смотрят переписанные ссылки, а известные цели
// содержат все виды из гарда занятости.
//
// Проверяется на реальной эмиссии, а не на списке строк: сброс осиротевших
// целей сравнивает цель правила с тем, что реально попадёт в config.json, и
// разойдись эти два множества — правило ушло бы в direct на первой загрузке
// (deps-К2).
func TestMigrationScenario10RuleTargetsSurviveEmission(t *testing.T) {
	s, _ := loadMainFixture(t)

	pc := s.ParserConfig
	res, err := config.GenerateOutboundsFromParserConfig(&pc, map[string]int{}, nil, config.DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("эмиссия мигрированного состояния: %v", err)
	}

	emitted := map[string]bool{}
	for _, line := range res.OutboundsJSON {
		body := line
		if i := strings.Index(body, "{"); i >= 0 {
			body = body[i:]
		}
		body = strings.TrimRight(strings.TrimSpace(body), ",")
		var m map[string]interface{}
		if json.Unmarshal([]byte(body), &m) != nil {
			continue
		}
		if tag, _ := m["tag"].(string); tag != "" {
			emitted[tag] = true
		}
	}

	// Цели правил, route_final и dns.detour после переписи миграции.
	for _, want := range []string{"[P]select", "[P]select-auto"} {
		if !emitted[want] {
			t.Errorf("тег %q не эмитирован — правило на него сбросилось бы на direct; emitted=%v", want, emitted)
		}
	}

	// Гард знает все виды тегов: замены, твины, верхние узлы, системные.
	guard := config.BuildTagGuard(pc.ParserConfig.Outbounds, pc.ParserConfig.Proxies,
		[]string{"Tokyo", "Transitional", "chain-1"}, []string{"direct-out", "block-out"})
	for _, want := range []string{"[P]select", "[P]select-auto", "Tokyo", "video-out", "direct-out"} {
		if !guard.Taken(want) {
			t.Errorf("гард не знает тега %q — переименование смогло бы его занять", want)
		}
	}
	known := config.KnownTargetTags(guard, pc.ParserConfig.Outbounds)
	for _, want := range []string{"[P]select", "[P]select-auto", "Tokyo"} {
		found := false
		for _, k := range known {
			if k == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("известные цели не содержат %q: %v", want, known)
		}
	}
}

// §Т7 хвост W2 (SPEC 118 W6): Load кладёт отчёт миграции на диск.
//
// В памяти State отчёт до конфигуратора не доживает: мигрирует ПЕРВЫЙ, кто
// откроет состояние, — на старте лаунчера это фоновая загрузка без окна, и к
// открытию мастера файл уже v7. Дистанцию между двумя моментами переживает
// только файл рядом.
func TestMigrationReportPersistedByLoad(t *testing.T) {
	s, statePath := loadMainFixture(t)
	if !s.Migration.HasWarnings() {
		t.Fatal("фикстура не дала ни одного предупреждения — проверять нечего")
	}
	binDir := filepath.Dir(filepath.Dir(statePath))

	saved := state.ReadMigrationReport(binDir)
	if saved == "" {
		t.Fatal("Load не сохранил отчёт — на headless-старте он пропал бы")
	}
	for _, w := range s.Migration.Warnings {
		if !strings.Contains(saved, w) {
			t.Errorf("предупреждение не доехало до файла: %q", w)
		}
	}
}

// §4.B.10 на РЕАЛЬНОЙ фикстуре — мигрированный golden real-v088
// (`core/state/testdata/real_v088_v4.json`, копия входа golden-сценария
// `core/build/testdata/golden/real-v088/` до перезафиксации в v7).
//
// Синтетическая фикстура выше проверяет ВИДЫ тегов; эта — что на реальном
// состоянии пользователя (5 подписок со свёрткой, 5 Направлений, 12 правил,
// верхний WireGuard-узел) миграция не оставила ни одной цели, которую
// `resetForeignRuleTargets` принял бы за осиротевшую. Стреляет он один раз и
// необратимо: сброшенное на direct правило пользователь восстанавливает
// руками.
func TestMigrationScenario10RealV088RuleTargetsNotReset(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "real_v088_v4.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	statePath := writeStateWithRaw(t, string(raw), nil)
	s, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("load real-v088: %v", err)
	}
	if s.Migration == nil {
		t.Fatal("real-v088 — легаси-состояние, отчёт миграции обязан быть")
	}

	// Множество известных целей строится ровно так, как его строит гард
	// перед сбросом: Направления + замены свёрнутых папок + верхние узлы +
	// системные теги шаблона.
	var rootTags []string
	for i := range s.Sources {
		src := &s.Sources[i]
		if src.Kind != state.SourceKindSubscription && src.Tag != "" {
			rootTags = append(rootTags, src.Tag)
		}
	}
	pc := s.ParserConfig
	guard := config.BuildTagGuard(pc.ParserConfig.Outbounds, pc.ParserConfig.Proxies,
		rootTags, []string{"direct-out", "block-out", "reject"})

	// Каждая живая цель правила обязана быть известной. Пустая цель и
	// системные теги шаблона проверяются тем же множеством — на них сброс
	// стреляет так же.
	checked := 0
	for _, r := range s.Rules {
		body, derr := r.DecodeBody()
		if derr != nil {
			continue
		}
		inline, ok := body.(*state.InlineBody)
		if !ok || inline.Outbound == "" {
			continue
		}
		checked++
		if !guard.Taken(inline.Outbound) {
			t.Errorf("цель правила %q не известна гарду — правило сбросилось бы на direct", inline.Outbound)
		}
	}
	if checked == 0 {
		t.Fatal("ни одного правила с целью — фикстура не проверяет предмет")
	}

	// Теги замены свёрнутых папок: правила v6 целились в fold-теги, и
	// миграция переписала их на replace-теги. Не знай их гард — reset увёл
	// бы их в direct на первой же загрузке.
	for i := range s.Sources {
		src := &s.Sources[i]
		if src.Replace == nil || src.Replace.Tag == "" {
			continue
		}
		if !guard.Taken(src.Replace.Tag) {
			t.Errorf("тег замены %q не известен гарду", src.Replace.Tag)
		}
	}

	// Направления и их твины.
	for _, d := range s.Directions {
		if !guard.Taken(d.Tag) {
			t.Errorf("Направление %q не известно гарду", d.Tag)
		}
	}
}
