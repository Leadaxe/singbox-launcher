package backup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/core/state"
)

// Поля стороны LxBox (BACKUP.md §2, «Поля стороны LxBox») лаунчер игнорирует
// МОЛЧА: импорт файла с ними даёт ровно то же состояние, что импорт того же
// файла без них, и предупреждений не даёт. Отдельно — ловушка `tag_policy`: у
// лаунчера это поле контейнера, и у корневого сервера оно не должно ни
// попасть в состояние, ни поменять финальный тег.
func TestLxBoxSideFieldsLeaveStateUnchanged(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(backupCorpusRelPath, "v10_lxbox_fields.backup.json"))
	if err != nil {
		t.Skipf("корпус недоступен: %v", err)
	}

	importBytes := func(data []byte) (*state.State, []Warning) {
		t.Helper()
		f, warns, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		dst := &state.State{}
		res, err := ImportFile(dst, f, ImportOptions{KnownOutbounds: []string{"proxy", "direct"}})
		if err != nil {
			t.Fatalf("Import: %v", err)
		}
		return dst, append(warns, res.Warnings...)
	}

	with, warns := importBytes(raw)
	if len(warns) != 0 {
		t.Fatalf("поля стороны LxBox дали предупреждения: %v", warns)
	}
	without, _ := importBytes(stripLxBoxSideFields(t, raw))

	a, err := with.MarshalV8()
	if err != nil {
		t.Fatalf("MarshalV8: %v", err)
	}
	b, err := without.MarshalV8()
	if err != nil {
		t.Fatalf("MarshalV8: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("поля стороны LxBox изменили состояние:\n--- с полями ---\n%s\n--- без них ---\n%s", a, b)
	}
	for i := range with.Sources {
		src := &with.Sources[i]
		if src.Kind == state.SourceKindServer && src.TagPolicy != nil {
			t.Errorf("корневой сервер %q сохранил tag_policy %+v — поле контейнера у узла", src.Tag, src.TagPolicy)
		}
	}
}

// stripLxBoxSideFields — тот же файл без полей стороны LxBox.
//
// Уровни, где снимаются ключи, разбираются в json.RawMessage: тела записей
// едут в состоянии байт в байт (порядок ключей sing-box), и пересборка через
// map[string]interface{} переставила бы их — состояния разошлись бы не из-за
// полей LxBox.
func stripLxBoxSideFields(t *testing.T, raw []byte) []byte {
	t.Helper()
	obj := func(data json.RawMessage) map[string]json.RawMessage {
		var m map[string]json.RawMessage
		if json.Unmarshal(data, &m) != nil {
			return nil
		}
		return m
	}
	arr := func(data json.RawMessage) []json.RawMessage {
		var l []json.RawMessage
		_ = json.Unmarshal(data, &l)
		return l
	}
	enc := func(v interface{}) json.RawMessage {
		out, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("сборка кейса без полей: %v", err)
		}
		return out
	}
	drop := func(m map[string]json.RawMessage, keys ...string) {
		for _, k := range keys {
			delete(m, k)
		}
	}

	doc := obj(raw)
	if doc == nil {
		t.Fatal("кейс — не объект")
	}
	sources := arr(doc["sources"])
	for i, rawSrc := range sources {
		src := obj(rawSrc)
		drop(src, "detour_policy", "import_rules", "import_rules_enabled", "on_update_action",
			"ping_url", "ping_timeout_ms", "label")
		if string(src["kind"]) == `"server"` {
			drop(src, "tag_policy")
		}
		if nodes := arr(src["nodes"]); len(nodes) > 0 {
			for j, rawNode := range nodes {
				node := obj(rawNode)
				if group := obj(node["group"]); group != nil {
					drop(group, "members_rule", "pool_badge")
					node["group"] = enc(group)
				}
				nodes[j] = enc(node)
			}
			src["nodes"] = enc(nodes)
		}
		sources[i] = enc(src)
	}
	doc["sources"] = enc(sources)

	rules := arr(doc["rules"])
	for i, rawRule := range rules {
		rule := obj(rawRule)
		drop(rule, "update_interval_hours", "verbatim")
		rules[i] = enc(rule)
	}
	doc["rules"] = enc(rules)

	if dns := obj(doc["dns"]); dns != nil {
		servers := arr(dns["servers"])
		for i, rawSrv := range servers {
			srv := obj(rawSrv)
			// `vars` у шаблонного сервера с контракта 1.0.2 — поле обеих
			// сторон (D-118), а не поле LxBox: оно едет в состояние.
			drop(srv, "description")
			servers[i] = enc(srv)
		}
		dns["servers"] = enc(servers)
		doc["dns"] = enc(dns)
	}
	return enc(doc)
}
