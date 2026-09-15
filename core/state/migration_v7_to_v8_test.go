package state

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Миграция v7 → v8 (SPEC 127 §3). Проверяется ровно то, ради чего она
// написана: форма записей, ничего не теряется, идентичность правил не
// меняется, файл идемпотентен, а исходник остаётся рядом копией.

// TestMigrateV7ToV8_FixtureBytes — миграция v7-фикстуры даёт РОВНО байты
// v8-фикстуры (modulo timestamps, которые Save штампует сам).
func TestMigrateV7ToV8_FixtureBytes(t *testing.T) {
	want, err := os.ReadFile(v8RoundtripFixture)
	if err != nil {
		t.Fatalf("v8 fixture missing (GEN_V8_ROUNDTRIP_FIXTURE=1): %v", err)
	}

	src := legacyFixtureCopy(t, v7RoundtripFixture)
	s, err := Load(src)
	if err != nil {
		t.Fatalf("Load v7: %v", err)
	}
	out := filepath.Join(t.TempDir(), "migrated.json")
	if err := s.Save(out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(normalizeTimestamps(want), normalizeTimestamps(got)) {
		t.Errorf("migrated v7 differs from the v8 fixture\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestMigrateV7ToV8_Idempotent — Parse(v8) → Save даёт те же байты: вторая
// загрузка уже мигрированного состояния ничего не переписывает.
func TestMigrateV7ToV8_Idempotent(t *testing.T) {
	fixture, err := os.ReadFile(v8RoundtripFixture)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Parse(fixture)
	if err != nil {
		t.Fatalf("Parse v8: %v", err)
	}
	if s.Migration != nil {
		t.Errorf("v8-файл не должен мигрировать: %+v", s.Migration)
	}
	out := filepath.Join(t.TempDir(), "again.json")
	if err := s.Save(out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(normalizeTimestamps(fixture), normalizeTimestamps(got)) {
		t.Errorf("Parse(v8)→Save не идемпотентен\n--- fixture ---\n%s\n--- got ---\n%s", fixture, got)
	}
}

// TestMigrateV7ToV8_BackupAndReport — рядом с файлом остаётся `.v7.bak` с
// ИСХОДНЫМИ байтами, а отчёт знает, откуда мигрировали.
func TestMigrateV7ToV8_BackupAndReport(t *testing.T) {
	src := legacyFixtureCopy(t, v7RoundtripFixture)
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}

	s, err := Load(src)
	if err != nil {
		t.Fatalf("Load v7: %v", err)
	}
	if s.Migration == nil {
		t.Fatal("миграция v7→v8 не отмечена отчётом")
	}
	if s.Migration.FromVersion != SchemaVersionV7 {
		t.Errorf("MigrationReport.FromVersion = %d, want %d", s.Migration.FromVersion, SchemaVersionV7)
	}
	bak := src + v7BackupSuffix
	if s.Migration.BackupPath != bak {
		t.Errorf("BackupPath = %q, want %q", s.Migration.BackupPath, bak)
	}
	saved, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("копии исходника нет: %v", err)
	}
	if !bytes.Equal(original, saved) {
		t.Error(".v7.bak не побайтовая копия исходного файла")
	}

	// Load уже переписал файл в v8 — повторная загрузка не мигрирует и копию
	// не трогает (O_EXCL: первая копия и есть исходник).
	again, err := Load(src)
	if err != nil {
		t.Fatalf("Load повторно: %v", err)
	}
	if again.Migration != nil {
		t.Errorf("повторная загрузка мигрирует заново: %+v", again.Migration)
	}
	saved2, _ := os.ReadFile(bak)
	if !bytes.Equal(original, saved2) {
		t.Error(".v7.bak перезаписан повторной загрузкой")
	}
}

// TestMigrateV7ToV8_NothingLost — поимённая сверка: каждое правило, каждый
// DNS-сервер/правило, секции узла, номера, все URL наборов, порядок ключей
// матчеров (инвариант волны 2 SPEC 127).
func TestMigrateV7ToV8_NothingLost(t *testing.T) {
	s, err := Load(legacyFixtureCopy(t, v7RoundtripFixture))
	if err != nil {
		t.Fatalf("Load v7: %v", err)
	}

	if len(s.Rules) != 4 {
		t.Fatalf("правил: %d, ожидалось 4", len(s.Rules))
	}
	// preset: vars наружу, тела нет.
	if s.Rules[0].Kind != RuleKindPreset || s.Rules[0].Vars["out"] != "direct-out" {
		t.Errorf("preset: %+v", s.Rules[0])
	}
	if len(s.Rules[0].Body) != 0 {
		t.Errorf("у preset не должно быть тела: %s", s.Rules[0].Body)
	}
	if s.Rules[0].Num == nil || *s.Rules[0].Num != 960 {
		t.Errorf("order_num → num потерян: %v", s.Rules[0].Num)
	}
	// inline: имя наружу, порядок ключей матчеров цел, цель в теле.
	if s.Rules[1].Name != "X" {
		t.Errorf("имя не переехало наружу: %+v", s.Rules[1])
	}
	if string(s.Rules[1].Body) != `{"port":[443],"domain_suffix":["example.com"],"outbound":"proxy-out"}` {
		t.Errorf("тело inline: %s", s.Rules[1].Body)
	}
	// drop → action=reject + method=drop (ровно как ApplyOutboundToRule).
	if string(s.Rules[2].Body) != `{"domain_suffix":["ads.example"],"action":"reject","method":"drop"}` {
		t.Errorf("drop не переведён в форму sing-box: %s", s.Rules[2].Body)
	}
	if s.Rules[2].Enabled {
		t.Error("enabled=false потерян")
	}
	// srs: оба URL, reject в теле.
	if len(s.Rules[3].Refs) != 2 {
		t.Fatalf("srs_url+srs_urls → refs: %v", s.Rules[3].Refs)
	}
	if s.Rules[3].Refs[0] != "https://example.invalid/a.srs" || s.Rules[3].Refs[1] != "https://example.invalid/b.srs" {
		t.Errorf("порядок наборов сбит: %v", s.Rules[3].Refs)
	}
	if string(s.Rules[3].Body) != `{"action":"reject"}` {
		t.Errorf("тело srs: %s", s.Rules[3].Body)
	}
	// Идентичность правил — та же строка, что до миграции (теги rule_set и
	// имена файлов srs-кэша висят на ней).
	if got := StableRuleID(s.Rules[3]); got != "three-sets" {
		t.Errorf("StableRuleID после миграции: %q", got)
	}

	// DNS: тела в body, теги снаружи, ссылочные записи без тел.
	if len(s.DNS.Servers) != 3 || len(s.DNS.Rules) != 2 {
		t.Fatalf("DNS: %d серверов, %d правил", len(s.DNS.Servers), len(s.DNS.Rules))
	}
	user := s.DNS.Servers[2]
	if user.Kind != DNSServerKindUser || user.Tag != "my-doh" {
		t.Errorf("user-сервер: %+v", user)
	}
	if user.Body["type"] != "https" || user.Body["server"] != "1.1.1.1" || user.Body["detour"] != "proxy-out" {
		t.Errorf("плоское тело не переехало в body: %+v", user.Body)
	}
	if _, leaked := user.Body["tag"]; leaked {
		t.Errorf("tag остался в теле: %+v", user.Body)
	}
	if s.DNS.Servers[0].Body != nil || s.DNS.Servers[1].Body != nil || s.DNS.Rules[0].Body != nil {
		t.Error("у template/preset-записей тела быть не должно")
	}
	if s.DNS.Rules[1].Body["rule_set"] != "ru-domains" || s.DNS.Rules[1].Body["server"] != "my-doh" {
		t.Errorf("тело user-правила: %+v", s.DNS.Rules[1].Body)
	}

	// Секции узла — та же форма, что в корне.
	var sections *NodeSections
	for i := range s.Sources {
		if s.Sources[i].Tag == "🇯🇵 Tokyo" {
			sections = s.Sources[i].Node.Sections
		}
	}
	if sections == nil || len(sections.Rules) != 1 {
		t.Fatalf("секции узла потеряны: %+v", sections)
	}
	sr := sections.Rules[0]
	if sr.Name != "@{self} network" || sr.Num == nil || *sr.Num != 945 {
		t.Errorf("правило секции: %+v", sr)
	}
	if string(sr.Body) != `{"ip_cidr":["100.64.0.0/10"],"outbound":"@self"}` {
		t.Errorf("тело правила секции: %s", sr.Body)
	}
	if len(sections.DNSServers()) != 1 || sections.DNSServers()[0].Tag != "@{self}-dns" {
		t.Errorf("DNS-серверы секции: %+v", sections.DNSServers())
	}
	if sections.DNSServers()[0].Body["endpoint"] != "@self" {
		t.Errorf("тело DNS-сервера секции: %+v", sections.DNSServers()[0].Body)
	}
	if len(sections.DNSRules()) != 1 || sections.DNSRules()[0].Body["server"] != "@{self}-dns" {
		t.Errorf("DNS-правила секции: %+v", sections.DNSRules())
	}

	// warp_accounts → warp без потерь.
	if s.WarpAccounts == nil || s.WarpAccounts.WG == nil || s.WarpAccounts.WG.PrivateKey != "priv" {
		t.Errorf("warp потерян: %+v", s.WarpAccounts)
	}
}

// TestMigrateV7ToV8_UnknownKindKept — запись неизвестного вида проносится как
// есть, с предупреждением в отчёт: чужой материал не исчезает из файла.
//
// Проверка идёт ЧЕРЕЗ ДИСК (Load → Save), а не по промежуточному документу:
// сырая запись проходит ещё и через parseV8, который читает только `num`, —
// оставленный `order_num` там молча терялся бы, а неразмеченная запись
// уезжала бы на оси вперёд соседей (MarkRuleOrder раздаёт с UserRuleNumStart).
func TestMigrateV7ToV8_UnknownKindKept(t *testing.T) {
	doc := []byte(`{
		"meta": {"version": 7, "schema": "sources_v7"},
		"sources": [],
		"directions": [],
		"rules": [
			{"kind": "preset", "ref": "p1", "enabled": true, "order_num": 900, "body": {"vars": {}}},
			{"kind": "geosite", "id": "XYZ", "enabled": true, "order_num": 1500, "body": {"list": "cn"}},
			{"kind": "inline", "enabled": true, "order_num": 2000, "body": {"name": "tail", "match": {"protocol": "dns"}, "outbound": "proxy"}}
		],
		"dns_options": {}
	}`)

	dir := t.TempDir()
	path := filepath.Join(dir, "bin", "wizard_states", "state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, doc, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Rules) != 3 {
		t.Fatalf("правил: %d, ожидалось 3", len(s.Rules))
	}
	if s.Rules[1].Kind != "geosite" {
		t.Fatalf("запись неизвестного вида потеряна: %+v", s.Rules)
	}
	if s.Rules[1].ID != "XYZ" {
		t.Errorf("id записи неизвестного вида потерян: %+v", s.Rules[1])
	}
	if s.Rules[1].Num == nil || *s.Rules[1].Num != 1500 {
		t.Fatalf("номер записи неизвестного вида потерян: %v", s.Rules[1].Num)
	}
	// Порядок на оси не поехал: чужая запись осталась между своими.
	if s.Rules[0].Num == nil || *s.Rules[0].Num != 900 ||
		s.Rules[2].Num == nil || *s.Rules[2].Num != 2000 {
		t.Errorf("номера соседей: %v %v", s.Rules[0].Num, s.Rules[2].Num)
	}

	// Save уже случился внутри Load (миграция персистится) — читаем файл.
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk struct {
		Rules []map[string]interface{} `json:"rules"`
	}
	if err := json.Unmarshal(saved, &onDisk); err != nil {
		t.Fatal(err)
	}
	if len(onDisk.Rules) != 3 || onDisk.Rules[1]["kind"] != "geosite" {
		t.Fatalf("на диске: %v", onDisk.Rules)
	}
	if onDisk.Rules[1]["num"] != float64(1500) {
		t.Errorf("num записи неизвестного вида не дожил до диска: %v", onDisk.Rules[1])
	}
	if _, stale := onDisk.Rules[1]["order_num"]; stale {
		t.Errorf("на диске остался ключ v7 order_num: %v", onDisk.Rules[1])
	}
	if body, _ := onDisk.Rules[1]["body"].(map[string]interface{}); body == nil || body["list"] != "cn" {
		t.Errorf("тело чужой записи изменено: %v", onDisk.Rules[1])
	}
}

// TestMigrateV7ToV8_StandaloneActionKept — `action` внутри `match` при пустом
// `outbound` — не цель, а самостоятельный эффект правила sing-box (`sniff`,
// `hijack-dns`, `resolve`). Такое правило приезжает от второй стороны через
// импорт бэкапа (`match` там — свободный фрагмент sing-box), v7-эмиттер
// копировал `match` дословно, и ключ доезжал до config.json.
func TestMigrateV7ToV8_StandaloneActionKept(t *testing.T) {
	doc := []byte(`{
		"meta": {"version": 7, "schema": "sources_v7"},
		"sources": [],
		"directions": [],
		"rules": [
			{"kind": "inline", "enabled": true, "order_num": 1020, "body": {"name": "sniff", "match": {"inbound": "tun-in", "action": "sniff", "timeout": "1s"}, "outbound": ""}},
			{"kind": "inline", "enabled": true, "order_num": 1030, "body": {"name": "dns hijack", "match": {"protocol": "dns", "action": "hijack-dns"}, "outbound": ""}},
			{"kind": "inline", "enabled": true, "order_num": 1040, "body": {"name": "blocked", "match": {"domain_suffix": ["ads.example"], "action": "reject"}, "outbound": ""}},
			{"kind": "inline", "enabled": true, "order_num": 1050, "body": {"name": "retarget", "match": {"protocol": "quic", "action": "sniff"}, "outbound": "proxy-out"}}
		],
		"dns_options": {}
	}`)
	rep := &MigrationReport{FromVersion: SchemaVersionV7}
	out, err := migrateV7DocToV8(doc, rep)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s, err := parseV8(out)
	if err != nil {
		t.Fatalf("parseV8: %v", err)
	}
	want := []string{
		`{"inbound":"tun-in","action":"sniff","timeout":"1s"}`,
		`{"protocol":"dns","action":"hijack-dns"}`,
		`{"domain_suffix":["ads.example"],"action":"reject"}`,
		// Цель есть → она переписывает эффект, как это делал ApplyOutboundToRule.
		`{"protocol":"quic","outbound":"proxy-out"}`,
	}
	for i := range want {
		if string(s.Rules[i].Body) != want[i] {
			t.Errorf("тело правила %d:\n  got  %s\n  want %s", i, s.Rules[i].Body, want[i])
		}
	}
}

// TestMigrateV7ToV8_DNSRuleMetadataOutsideBody — `id` и `name` DNS-правила —
// метаданные записи (ONE_NAMESPACE §1): наружу уезжают ОБА, внутри `body`
// остаётся объект sing-box и ничего кроме. Асимметрия (один ключ снят, второй
// продублирован) дала бы два источника истины и мусор в config.json.
func TestMigrateV7ToV8_DNSRuleMetadataOutsideBody(t *testing.T) {
	doc := []byte(`{
		"meta": {"version": 7, "schema": "sources_v7"},
		"sources": [],
		"directions": [],
		"rules": [],
		"dns_options": {"rules": [
			{"kind": "user", "enabled": true, "id": "RID", "name": "my rule", "rule_set": "x", "server": "t1"}
		]}
	}`)
	rep := &MigrationReport{FromVersion: SchemaVersionV7}
	out, err := migrateV7DocToV8(doc, rep)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s, err := parseV8(out)
	if err != nil {
		t.Fatalf("parseV8: %v", err)
	}
	if len(s.DNS.Rules) != 1 {
		t.Fatalf("DNS-правил: %d", len(s.DNS.Rules))
	}
	r := s.DNS.Rules[0]
	if r.ID != "RID" || r.Name != "my rule" {
		t.Errorf("метаданные не переехали наружу: %+v", r)
	}
	for _, k := range []string{"id", "name", "kind", "ref", "enabled"} {
		if _, dup := r.Body[k]; dup {
			t.Errorf("ключ %q остался внутри body: %v", k, r.Body)
		}
	}
	if r.Body["rule_set"] != "x" || r.Body["server"] != "t1" {
		t.Errorf("тело правила потеряно: %v", r.Body)
	}
}

// TestV8FileIsNotReadAsV7 — гейт версии: файл v8 не проваливается в ветку v7
// (иначе новая форма молча терялась бы на первом же Save).
func TestV8FileIsNotReadAsV7(t *testing.T) {
	fixture, err := os.ReadFile(v8RoundtripFixture)
	if err != nil {
		t.Fatal(err)
	}
	v, err := SchemaVersionOfBytes(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if v != SchemaVersionV8 {
		t.Fatalf("SchemaVersionOfBytes = %d, want %d", v, SchemaVersionV8)
	}
	s, err := Parse(fixture)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Version != SchemaVersionV8 {
		t.Errorf("Parse прочитал v8 как v%d", s.Version)
	}
	if len(s.Rules) == 0 || s.Rules[len(s.Rules)-1].Kind != RuleKindSrs || len(s.Rules[len(s.Rules)-1].Refs) != 2 {
		t.Errorf("v8-поля потеряны при чтении: %+v", s.Rules)
	}
}

// normalizeTimestamps — created_at/updated_at на плейсхолдер: Save штампует их
// сам, а сравнение здесь про форму, а не про время.
func normalizeTimestamps(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	for i, ln := range lines {
		for _, key := range []string{"created_at", "updated_at"} {
			if bytes.HasPrefix(ln, []byte(`    "`+key+`": "`)) {
				suffix := ""
				if bytes.HasSuffix(ln, []byte(",")) {
					suffix = ","
				}
				lines[i] = []byte(`    "` + key + `": "<normalized>"` + suffix)
			}
		}
	}
	return bytes.Join(lines, []byte("\n"))
}
