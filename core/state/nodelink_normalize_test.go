package state

import (
	"bytes"
	"testing"
)

// devFormsStateV8 — состояние, записанное сборкой 1.6.0 до выпуска: ссылки
// в dev-формах, которые норма NodeLink не принимает (NODE_LINK.md §7.3).
//
//   - член провайдерской группы подписки без folder_id (S1);
//   - позиция цепочки на группу ФИНАЛЬНЫМ тегом (`nl:Best`) — так её писала
//     форма (S3);
//   - позиция на группу в папке с переменной в политике: финальный тег не
//     угадать, ссылка обязана остаться как есть.
const devFormsStateV8 = `{
  "meta": {"version": 8, "schema": "sources_v8", "created_at": "2026-09-01T00:00:00Z", "updated_at": "2026-09-01T00:00:00Z"},
  "sources": [
    {
      "kind": "subscription", "id": "01SUBNL0000000000000000000", "name": "NL", "enabled": true,
      "url": "https://example.invalid/nl", "tag_policy": {"prefix": "nl:"},
      "nodes": [
        {"kind": "server", "tag": "US-1", "enabled": true, "body": {"type": "trojan", "server": "us1.example", "server_port": 443}},
        {"kind": "server", "tag": "US-2", "enabled": false, "body": {"type": "trojan", "server": "us2.example", "server_port": 443}},
        {"kind": "auto", "tag": "Best", "enabled": true, "group": {"group_type": "urltest", "members": [{"tag": "US-1"}, {"tag": "US-2"}]}}
      ]
    },
    {
      "kind": "folder", "id": "01FLDVARS00000000000000000", "name": "Vars", "enabled": true,
      "tag_policy": {"prefix": "{$num} "},
      "nodes": [
        {"kind": "server", "tag": "DE-1", "enabled": true, "body": {"type": "trojan", "server": "de1.example", "server_port": 443}},
        {"kind": "auto", "tag": "Pick", "enabled": true, "group": {"group_type": "urltest", "members": [{"folder_id": "01FLDVARS00000000000000000", "tag": "DE-1"}]}}
      ]
    },
    {
      "kind": "chain", "tag": "via-best", "enabled": true, "body": {"type": "chain"},
      "hops": [{"folder_id": "01SUBNL0000000000000000000", "tag": "nl:Best"}, {"folder_id": "01FLDVARS00000000000000000", "tag": "2 Pick"}]
    }
  ],
  "directions": [],
  "rules": [],
  "dns": {}
}`

// Чтение состояния поднимает dev-формы ссылок до нормы, и повторное чтение
// ничего не меняет — перезаписи файла на загрузке поэтому нет.
func TestNodeLinkDevFormsLiftedOnRead(t *testing.T) {
	s, err := Parse([]byte(devFormsStateV8))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	const sub = "01SUBNL0000000000000000000"
	const vars = "01FLDVARS00000000000000000"

	best := s.Sources[0].Nodes[2].Group
	want := []NodeLink{{FolderID: sub, Tag: "US-1"}, {FolderID: sub, Tag: "US-2"}}
	if len(best.Members) != len(want) || best.Members[0] != want[0] || best.Members[1] != want[1] {
		t.Errorf("S1: члены группы подписки = %+v, want %+v", best.Members, want)
	}

	hops := s.Sources[2].Hops
	if hops[0] != (NodeLink{FolderID: sub, Tag: "Best"}) {
		t.Errorf("S3: позиция на группу финальным тегом = %+v, want {%s Best}", hops[0], sub)
	}
	if hops[1] != (NodeLink{FolderID: vars, Tag: "2 Pick"}) {
		t.Errorf("политика с переменной: финальный тег не угадывается, ссылка обязана остаться как есть, got %+v", hops[1])
	}

	first, err := s.MarshalV8()
	if err != nil {
		t.Fatalf("MarshalV8: %v", err)
	}
	again, err := Parse(first)
	if err != nil {
		t.Fatalf("Parse(второй круг): %v", err)
	}
	second, err := again.MarshalV8()
	if err != nil {
		t.Fatalf("MarshalV8(второй круг): %v", err)
	}
	if !bytes.Equal(normalizeTimestamps(first), normalizeTimestamps(second)) {
		t.Errorf("нормализация не идемпотентна:\n--- первый круг ---\n%s\n--- второй ---\n%s", first, second)
	}
}
