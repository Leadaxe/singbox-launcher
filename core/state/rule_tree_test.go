package state_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"singbox-launcher/core/backup"
	"singbox-launcher/core/build"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// ruleRefsV8Fixture — state.json v8 старой формы (SPEC 188 волна 2): ссылки
// наборов — полем записи. Пять случаев: плоское srs с дублем ссылки и DNS,
// inline с порядком ключей «как в файле», srs с корнем logical, ссылки и на
// записи, и в теле, дерево с ссылками в под-правиле.
const ruleRefsV8Fixture = `{
  "meta": {"version": 8, "schema": "sources_v8", "created_at": "2026-11-01T00:00:00Z", "updated_at": "2026-11-01T00:00:00Z"},
  "sources": [],
  "directions": [],
  "rules": [
    {"kind": "srs", "name": "Lists", "enabled": true, "num": 1000,
     "refs": ["https://e.com/a.srs", "https://e.com/b.srs", "https://e.com/a.srs"],
     "srs_mode": "local",
     "body": {"outbound": "direct-out"},
     "dns": {"enabled": true, "serverTag": "cf"}},
    {"kind": "inline", "name": "Plain", "enabled": true, "num": 1010,
     "body": {"port": [443], "domain_suffix": ["x.example"], "outbound": "direct-out"}},
    {"kind": "srs", "name": "Old logical", "enabled": true, "num": 1020,
     "refs": ["https://e.com/c.json"],
     "body": {"type": "logical", "mode": "or", "rules": [{"port": [443]}, {"network": ["udp"]}], "outbound": "direct-out"}},
    {"kind": "srs", "name": "Both", "enabled": true, "num": 1030,
     "refs": ["https://e.com/b.srs"],
     "body": {"refs": ["https://e.com/a.srs"], "outbound": "direct-out"}},
    {"kind": "inline", "name": "Tree", "enabled": true, "num": 1040,
     "body": {"type": "logical", "mode": "and", "rules": [{"source_ip_is_private": true}, {"refs": ["https://e.com/c.json"]}], "outbound": "direct-out"}},
    {"kind": "inline", "name": "Hand", "enabled": true, "num": 1050,
     "body": {"rule_set": ["user:Lists:2"], "port": [80], "outbound": "direct-out"}},
    {"kind": "inline", "name": "Empty refs", "enabled": true, "num": 1060,
     "body": {"port": [22], "refs": [], "outbound": "direct-out"}},
    {"kind": "inline", "name": "Group refs", "enabled": true, "num": 1070,
     "body": {"type": "logical", "mode": "or", "refs": ["https://e.com/g.srs"], "rules": [{"port": [1]}, {"port": [2]}], "outbound": "direct-out"}}
  ],
  "dns": {"rules": [{"kind": "user", "enabled": true, "body": {"rule_set": "user:Lists", "server": "cf"}}]}
}`

// TestRuleRefsLiftedIntoBody — `refs` правила живёт в теле (контракт 1.1.179):
// запись старой формы читается, сохраняется новой; ссылки и на записи, и в
// дереве — главнее дерево; дерево со ссылками в под-правиле — srs и не роняет
// сборку; плоское правило собирается в прежний вид; экспорт/импорт бэкапа
// пишет lx_backup 3 и переносит форму без потерь. Волна 3: рукописные ссылки
// на старые теги user:<id> (тело правила, своё DNS-правило) переписываются на
// теги §6.13 при загрузке; дерево со ссылками в под-правиле выпускается.
func TestRuleRefsLiftedIntoBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte(ruleRefsV8Fixture), 0o644); err != nil {
		t.Fatal(err)
	}

	// ── Загрузка: подъём ссылок, вывод вида ──
	s, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load v8: %v", err)
	}
	byName := func(st *state.State, name string) state.Rule {
		t.Helper()
		for _, r := range st.Rules {
			if r.Name == name {
				return r
			}
		}
		t.Fatalf("правило %q не найдено", name)
		return state.Rule{}
	}
	wantBodies := map[string]string{
		"Lists": `{"refs":["https://e.com/a.srs","https://e.com/b.srs"],"outbound":"direct-out"}`,
		// inline без ссылок — байт-в-байт как в файле.
		"Plain": `{"port":[443],"domain_suffix":["x.example"],"outbound":"direct-out"}`,
		// составной корень — обёртка and: ссылки первым под-правилом.
		"Old logical": `{"type":"logical","mode":"and","rules":[{"refs":["https://e.com/c.json"]},{"type":"logical","mode":"or","rules":[{"port":[443]},{"network":["udp"]}]}],"outbound":"direct-out"}`,
		// ссылки и на записи, и в дереве — главнее дерево.
		"Both": `{"refs":["https://e.com/a.srs"],"outbound":"direct-out"}`,
		"Tree": `{"type":"logical","mode":"and","rules":[{"source_ip_is_private":true},{"refs":["https://e.com/c.json"]}],"outbound":"direct-out"}`,
		// рукописная ссылка на старый тег user:<id>:2 — на тег §6.13 (волна 3).
		"Hand": `{"rule_set":["Lists:589a1aec"],"port":[80],"outbound":"direct-out"}`,
		// пустой refs снимается при нормализации (порядок ключей прежний).
		"Empty refs": `{"port":[22],"outbound":"direct-out"}`,
		// refs на узле logical — запись хранится как есть (сборка не выпускает).
		"Group refs": `{"type":"logical","mode":"or","refs":["https://e.com/g.srs"],"rules":[{"port":[1]},{"port":[2]}],"outbound":"direct-out"}`,
	}
	wantKinds := map[string]state.RuleKind{
		"Lists": state.RuleKindSrs, "Plain": state.RuleKindInline, "Old logical": state.RuleKindSrs,
		"Both": state.RuleKindSrs, "Tree": state.RuleKindSrs, "Hand": state.RuleKindInline,
		"Empty refs": state.RuleKindInline, "Group refs": state.RuleKindInline,
	}
	wantRefs := map[string][]string{
		"Lists":       {"https://e.com/a.srs", "https://e.com/b.srs"},
		"Old logical": {"https://e.com/c.json"},
		"Both":        {"https://e.com/a.srs"},
		"Tree":        {"https://e.com/c.json"},
	}
	checkRules := func(stage string, st *state.State) {
		t.Helper()
		for name, body := range wantBodies {
			r := byName(st, name)
			// Сжатое сравнение сохраняет порядок ключей: файл пишется с
			// отступами, а норма — про ключи и их порядок, не про пробелы.
			var compact bytes.Buffer
			if err := json.Compact(&compact, r.Body); err != nil || compact.String() != body {
				t.Errorf("%s: %q тело\n got %s\nwant %s", stage, name, r.Body, body)
			}
			if r.Refs != nil {
				t.Errorf("%s: %q: ссылки остались на записи: %v", stage, name, r.Refs)
			}
			if r.Kind != wantKinds[name] {
				t.Errorf("%s: %q: kind %q, ожидался %q", stage, name, r.Kind, wantKinds[name])
			}
			if got := state.RuleRefs(r); !reflect.DeepEqual(got, wantRefs[name]) {
				t.Errorf("%s: %q: RuleRefs %v, ожидалось %v", stage, name, got, wantRefs[name])
			}
		}
		lists := byName(st, "Lists")
		if lists.DNS == nil || lists.DNS.ServerTag != "cf" || lists.SrsMode != state.SrsModeLocal {
			t.Errorf("%s: dns и srs_mode остаются на записи: %+v", stage, lists)
		}
		// Своё DNS-правило со ссылкой на старый тег user:<id> — на тег §6.13.
		if len(st.DNS.Rules) != 1 || st.DNS.Rules[0].Body["rule_set"] != "Lists" {
			t.Errorf("%s: DNS-правило со старым тегом не переписано: %+v", stage, st.DNS.Rules)
		}
	}
	checkRules("load", s)

	// ── Сохранение: v9, ключа refs у записей нет ──
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk struct {
		Meta  struct{ Version int } `json:"meta"`
		Rules []map[string]json.RawMessage
	}
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	// Сборка до v9 читает meta.schema строкой: на v9 её разбор обязан упасть
	// (файл новее приложения отвергается, а не читается с потерей srs-правил).
	var oldReader struct {
		Meta struct {
			Version int    `json:"version"`
			Schema  string `json:"schema"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &oldReader); err == nil {
		t.Errorf("разбор v8 (meta.schema строкой) прочёл файл v9 без ошибки")
	}
	if disk.Meta.Version != state.SchemaVersionV9 {
		t.Errorf("Save пишет meta.version %d, ожидалась %d", disk.Meta.Version, state.SchemaVersionV9)
	}
	for i, rec := range disk.Rules {
		if _, ok := rec["refs"]; ok {
			t.Errorf("rules[%d]: ключ refs на записи после сохранения", i)
		}
	}

	// ── Повторная загрузка ничего не меняет ──
	s2, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load v9: %v", err)
	}
	checkRules("reload", s2)
	if err := s2.Save(path); err != nil {
		t.Fatal(err)
	}
	raw2, _ := os.ReadFile(path)
	if !bytes.Equal(dropUpdatedAt(raw), dropUpdatedAt(raw2)) {
		t.Errorf("второе сохранение изменило файл:\n%s\n---\n%s", raw, raw2)
	}

	// ── Сборка: плоское правило — прежний вид, дерево не роняет сборку ──
	cache := build.CollectSrsCachedPaths(s2.Rules, "/data", "")
	for _, ref := range []string{"https://e.com/a.srs", "https://e.com/b.srs", "https://e.com/c.json"} {
		if cache[ref] == "" {
			t.Errorf("кэш по ссылке: нет пути для %s (%v)", ref, cache)
		}
	}
	route := build.ResolveRouteWithGlobals(s2, &template.TemplateData{}, "/data", cache, template.TargetSpec{}, nil)
	bodies := map[string]string{}
	for _, rr := range route.Rules {
		b, _ := json.Marshal(rr.Body)
		bodies[rr.Name] = string(b)
	}
	// Теги наборов — имя правила (§6.13, волна 3); дерево выпускается.
	wantBuilt := map[string]string{
		"Lists": `{"outbound":"direct-out","rule_set":["Lists","Lists:589a1aec"]}`,
		"Tree":  `{"mode":"and","outbound":"direct-out","rules":[{"source_ip_is_private":true},{"rule_set":"Tree:86c39bbd"}],"type":"logical"}`,
	}
	for name, want := range wantBuilt {
		if bodies[name] != want {
			t.Errorf("правило %q собирается иначе:\n got %s\nwant %s", name, bodies[name], want)
		}
	}
	// Ключ refs ядру не уходит ни в одной ветке; refs на узле logical —
	// правило не выпускается, причина — в Skipped.
	for name, b := range bodies {
		if bytes.Contains([]byte(b), []byte(`"refs"`)) {
			t.Errorf("правило %q уходит в config.json с ключом refs: %s", name, b)
		}
	}
	if _, ok := bodies["Group refs"]; ok {
		t.Errorf("правило с refs на узле logical выпущено: %s", bodies["Group refs"])
	}
	groupSkipped := false
	for _, rs := range route.RuleSets {
		if rs.Skipped && rs.Tag == "Group refs" && rs.SkippedReason != "" {
			groupSkipped = true
		}
	}
	if !groupSkipped {
		t.Errorf("нет записи Skipped с причиной для правила с refs на узле logical: %+v", route.RuleSets)
	}
	if bodies["Empty refs"] != `{"outbound":"direct-out","port":[22]}` {
		t.Errorf("правило с пустым refs: %s", bodies["Empty refs"])
	}

	// ── Бэкап: экспорт пишет 3, импорт переносит форму ──
	// Запись старой формы в памяти (в обход загрузки) экспортируется новой.
	mem := *s2
	mem.Rules = append(append([]state.Rule(nil), s2.Rules...), state.Rule{
		Kind: state.RuleKindSrs, Name: "In memory", Enabled: true,
		Refs: []string{"https://e.com/d.srs"}, Body: json.RawMessage(`{"outbound":"direct-out"}`),
	})
	b, _, err := backup.Export10(&mem, backup.ExportOptions{})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	if b.LxBackup != backup.FormatVersion11 {
		t.Errorf("экспорт пишет lx_backup %d, ожидалось %d", b.LxBackup, backup.FormatVersion11)
	}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	file, _, err := backup.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	dst := &state.State{}
	if _, err := backup.ImportFile(dst, file, backup.ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	checkRules("backup", dst)
	if got := byName(dst, "In memory"); string(got.Body) != `{"refs":["https://e.com/d.srs"],"outbound":"direct-out"}` || got.Refs != nil {
		t.Errorf("запись старой формы экспортирована не в новой: %s (refs записи %v)", got.Body, got.Refs)
	}

	// ── Формы до v8: srs_url / srs_urls (state v6, v7), ref / refs (бэкап 0.12) ──
	// Ссылки поднимаются в тело на каждом пути, порядок и дедуп сохраняются;
	// legacy-вид (CustomRules) видит srs-правило со всеми наборами.
	const legacyRules = `[
	  {"kind": "srs", "enabled": true, "order_num": 1000, "body": {"name": "One", "srs_url": "https://e.com/a.srs", "outbound": "direct-out"}},
	  {"kind": "srs", "enabled": true, "order_num": 1010, "body": {"name": "Many", "srs_url": "https://e.com/a.srs",
	   "srs_urls": ["https://e.com/a.srs", "https://e.com/b.srs", "https://e.com/a.srs", "https://e.com/c.srs"], "outbound": "direct-out"}}
	]`
	legacyWant := map[string]string{
		"One":  `{"refs":["https://e.com/a.srs"],"outbound":"direct-out"}`,
		"Many": `{"refs":["https://e.com/a.srs","https://e.com/b.srs","https://e.com/c.srs"],"outbound":"direct-out"}`,
	}
	checkLegacy := func(stage string, st *state.State, view bool) {
		t.Helper()
		for name, want := range legacyWant {
			r := byName(st, name)
			if string(r.Body) != want || r.Refs != nil || r.Kind != state.RuleKindSrs {
				t.Errorf("%s: %q: тело %s, refs записи %v, kind %q; ожидалось тело %s", stage, name, r.Body, r.Refs, r.Kind, want)
			}
			if !view {
				continue
			}
			found := false
			for _, cr := range st.CustomRules {
				if cr.Label != name {
					continue
				}
				found = true
				if cr.Type != state.RuleTypeSRS || len(cr.RuleSet) != len(state.RuleRefs(r)) {
					t.Errorf("%s: %q: legacy-вид Type %q, наборов %d, ожидалось srs и %d", stage, name, cr.Type, len(cr.RuleSet), len(state.RuleRefs(r)))
				}
			}
			if !found {
				t.Errorf("%s: %q нет в legacy-виде", stage, name)
			}
		}
	}
	for _, tc := range []struct{ stage, doc string }{
		{"v6", `{"meta": {"version": 6}, "connections": {"sources": [], "outbounds": [], "defaults": {}}, "rules": ` + legacyRules + `}`},
		{"v7", `{"meta": {"version": 7, "schema": "sources_v7"}, "sources": [], "directions": [], "rules": ` + legacyRules + `}`},
	} {
		st, err := state.Parse([]byte(tc.doc))
		if err != nil {
			t.Fatalf("%s: Parse: %v", tc.stage, err)
		}
		checkLegacy(tc.stage, st, true)
	}
	file012, _, err := backup.Parse([]byte(`{
  "lx_backup": 1, "exported_by": {"app": "launcher", "version": "1.5.9", "platform": "darwin"}, "exported_at": "2025-06-15T15:06:40Z",
  "rules": [
    {"kind": "srs", "name": "One", "enabled": true, "ref": "https://e.com/a.srs", "outbound": "direct-out"},
    {"kind": "srs", "name": "Many", "enabled": true, "ref": "https://e.com/a.srs",
     "refs": ["https://e.com/a.srs", "https://e.com/b.srs", "https://e.com/a.srs", "https://e.com/c.srs"], "outbound": "direct-out"}
  ]
}`))
	if err != nil {
		t.Fatalf("0.12: Parse: %v", err)
	}
	dst012 := state.New()
	if _, err := backup.ImportFile(dst012, file012, backup.ImportOptions{}); err != nil {
		t.Fatalf("0.12: Import: %v", err)
	}
	checkLegacy("0.12", dst012, false)
}

// dropUpdatedAt — файл без строки meta.updated_at (её штампует Save).
func dropUpdatedAt(data []byte) []byte {
	var out [][]byte
	for _, ln := range bytes.Split(data, []byte("\n")) {
		if !bytes.Contains(ln, []byte(`"updated_at"`)) {
			out = append(out, ln)
		}
	}
	return bytes.Join(out, []byte("\n"))
}
