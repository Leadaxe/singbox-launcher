package backup

// Поле `sections` в бэкапе (контракт 1.1.85, NODE_SECTIONS.md; LxBox §575):
// экспорт его не пишет, импорт снимает у записи любого вида, непустой набор
// называется кодом backup_section_record_dropped с причиной not_allowed.

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// exportParseImport10 гоняет состояние через ПОЛНЫЙ круг файла 1.0:
// Export10 → Marshal → Parse → Import.
func exportParseImport10(t *testing.T, src, dst *state.State) (*state.State, []Warning) {
	t.Helper()
	b, _, err := Export10(src, ExportOptions{AppVersion: "test", Platform: "darwin"})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal backup: %v", err)
	}
	if strings.Contains(string(raw), `"sections"`) {
		t.Fatalf("экспорт 1.0 написал ключ sections: %s", raw)
	}
	parsed, warns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, w := range warns {
		if w.Code == WarnBackupUnknownField {
			t.Errorf("scanUnknown ругается на свой же файл 1.0: %s", w)
		}
	}
	res, err := ImportFile(dst, parsed, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return dst, append(warns, res.Warnings...)
}

// TestBackupSectionsDroppedFromAnyRecord — непустые секции у сервера, члена
// папки и подписки снимаются с кодом not_allowed; пустой набор молчит; узлы
// импортируются.
func TestBackupSectionsDroppedFromAnyRecord(t *testing.T) {
	const file = `{
  "lx_backup": 2,
  "sources": [
    {"kind": "server", "tag": "wg-home", "enabled": true,
     "body": {"type": "trojan", "server": "192.0.2.1", "server_port": 443, "password": "pw"},
     "sections": {"rules": [{"kind": "inline", "enabled": true, "body": {"ip_cidr": ["10.0.0.0/8"], "outbound": "@self"}}]}},
    {"kind": "server", "tag": "empty-sec", "enabled": true,
     "body": {"type": "trojan", "server": "192.0.2.2", "server_port": 443, "password": "pw"},
     "sections": {}},
    {"kind": "folder", "id": "01F00000000000000000000000", "name": "Home", "enabled": true,
     "nodes": [
       {"kind": "server", "tag": "ts-member", "enabled": true,
        "body": {"type": "tailscale"},
        "sections": {"dns": {"servers": [{"kind": "user", "tag": "@{self}-dns", "enabled": true, "body": {"type": "tailscale", "endpoint": "@self"}}]}}}
     ]},
    {"kind": "subscription", "id": "01S00000000000000000000000", "name": "Prov", "enabled": true,
     "url": "https://example.invalid/sub",
     "sections": {"rules": [{"kind": "inline", "enabled": true, "body": {"domain": ["x"], "outbound": "direct"}}]}}
  ]
}`
	parsed, warns, err := Parse([]byte(file))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	dst := state.New()
	res, err := ImportFile(dst, parsed, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	warns = append(warns, res.Warnings...)

	got := map[string]string{}
	for _, w := range warns {
		if w.Code == WarnBackupUnknownField && strings.Contains(w.Detail, "sections") {
			t.Errorf("sections назван неизвестным полем: %s", w)
		}
		if w.Code != WarnBackupSectionRecordDropped {
			continue
		}
		if w.Reason != SectionDropNotAllowed {
			t.Errorf("reason=%q, ожидался %q: %s", w.Reason, SectionDropNotAllowed, w)
		}
		got[strings.TrimSuffix(w.Detail, ": sections")] = w.Kind
	}
	want := map[string]string{"wg-home": "server", "ts-member": "server", "Prov": "subscription"}
	if len(got) != len(want) {
		t.Fatalf("предупреждения о снятых секциях %v, ожидались %v", got, want)
	}
	for tag, kind := range want {
		if got[tag] != kind {
			t.Errorf("%s: kind=%q, ожидался %q (все: %v)", tag, got[tag], kind, got)
		}
	}

	// Узлы на месте, и повторный экспорт секций не пишет.
	tags := map[string]bool{}
	for _, src := range dst.Sources {
		tags[src.Tag] = true
		for _, n := range src.Nodes {
			tags[n.Tag] = true
		}
	}
	for _, tag := range []string{"wg-home", "empty-sec", "ts-member"} {
		if !tags[tag] {
			t.Errorf("узел %q не импортирован", tag)
		}
	}
	exportParseImport10(t, dst, state.New())
}
