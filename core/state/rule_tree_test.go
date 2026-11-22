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
     "body": {"type": "logical", "mode": "and", "rules": [{"source_ip_is_private": true}, {"refs": ["https://e.com/c.json"]}], "outbound": "direct-out"}}
  ],
  "dns": {}
}`

// TestRuleRefsLiftedIntoBody — `refs` правила живёт в теле (контракт 1.1.179):
// запись старой формы читается, сохраняется новой; ссылки и на записи, и в
// дереве — главнее дерево; дерево со ссылками в под-правиле — srs и не роняет
// сборку; плоское правило собирается в прежний вид; экспорт/импорт бэкапа
// пишет lx_backup 3 и переносит форму без потерь.
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
	}
	wantKinds := map[string]state.RuleKind{
		"Lists": state.RuleKindSrs, "Plain": state.RuleKindInline, "Old logical": state.RuleKindSrs,
		"Both": state.RuleKindSrs, "Tree": state.RuleKindSrs,
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
	var listsRule, treeRule map[string]interface{}
	for _, rr := range route.Rules {
		switch rr.Name {
		case "Lists":
			listsRule = rr.Body
		case "Tree":
			treeRule = rr.Body
		}
	}
	wantLists := map[string]interface{}{
		"rule_set": []string{"user:Lists", "user:Lists:2"},
		"outbound": "direct-out",
	}
	if !reflect.DeepEqual(listsRule, wantLists) {
		t.Errorf("плоское srs-правило собирается иначе: %v, ожидалось %v", listsRule, wantLists)
	}
	if treeRule != nil {
		t.Errorf("дерево со ссылками в под-правиле эта волна не выпускает: %v", treeRule)
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
