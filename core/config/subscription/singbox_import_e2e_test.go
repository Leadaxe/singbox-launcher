package subscription

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 094 A5 — сквозной проход тела источника через рабочий разбор.
//
// Отличие от singbox_import_test.go: там проверяется ядро разбора sing-box
// JSON, здесь — весь разбор тела подписки целиком (ParseSubscriptionBody:
// классификация, skip, дедуп, уникализация сырых тегов, перепривязка состава
// групп). Это тот же вызов, которым тело разбирает fetch
// (config.MaterializeSubscriptionBody). Тег-политика (префикс/постфикс) в
// разбор не входит — её применяет эмиссия; сквозные проверки с префиксом
// живут в пакете config (canonical_emit_test.go).

// parseInlineBody прогоняет ДЕКОДИРОВАННОЕ тело через ParseSubscriptionBody с
// дефолтным капом — как fetch после декодера.
func parseInlineBody(t *testing.T, body string, skip []map[string]string) *ParsedBody {
	t.Helper()
	pb, err := ParseSubscriptionBody([]byte(body), skip, 0)
	if err != nil {
		t.Fatalf("ParseSubscriptionBody() error: %v", err)
	}
	if pb == nil {
		t.Fatal("ParseSubscriptionBody() returned nil result")
	}
	return pb
}

// entryNodes — узлы принятых записей в порядке тела (группы включительно).
func entryNodes(pb *ParsedBody) []*configtypes.ParsedNode {
	out := make([]*configtypes.ParsedNode, 0, len(pb.Entries))
	for _, e := range pb.Entries {
		if e != nil && e.Node != nil {
			out = append(out, e.Node)
		}
	}
	return out
}

// rawTagsOf — сырые (уникализированные) теги принятых записей.
func rawTagsOf(pb *ParsedBody) []string {
	out := make([]string, 0, len(pb.Entries))
	for _, e := range pb.Entries {
		if e != nil {
			out = append(out, e.RawTag)
		}
	}
	return out
}

// groupEntriesOf отбирает записи-группы.
func groupEntriesOf(pb *ParsedBody) []*ParsedBodyEntry {
	out := make([]*ParsedBodyEntry, 0)
	for _, e := range pb.Entries {
		if e != nil && e.Node != nil && e.Node.Scheme == configtypes.SchemeGroup {
			out = append(out, e)
		}
	}
	return out
}

// groupNodesOf отбирает узлы-группы (SchemeGroup) из общего списка.
func groupNodesOf(nodes []*configtypes.ParsedNode) []*configtypes.ParsedNode {
	out := make([]*configtypes.ParsedNode, 0)
	for _, n := range nodes {
		if n != nil && n.Scheme == configtypes.SchemeGroup {
			out = append(out, n)
		}
	}
	return out
}

// groupMembersOf возвращает состав узла-группы.
func groupMembersOf(node *configtypes.ParsedNode) []string {
	if node == nil || node.Outbound == nil {
		return nil
	}
	raw, _ := node.Outbound[configtypes.GroupMembersKey].([]interface{})
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// Обычная URI-подписка не затронута: групп нет, узлы разбираются как раньше.
func TestLoadSourceURIListUnaffected(t *testing.T) {
	body := "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com#node-one"
	res := parseInlineBody(t, body, nil)

	if len(res.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(res.Entries))
	}
	if got := len(groupEntriesOf(res)); got != 0 {
		t.Fatalf("URI list must not produce group nodes, got %d", got)
	}
	if len(res.IgnoredSections) != 0 {
		t.Fatalf("URI list must not report ignored sections, got %v", res.IgnoredSections)
	}
}

// skip-фильтр применяется и к импортированным узлам, и группа теряет
// отфильтрованного члена, оставаясь валидной.
func TestLoadSourceSkipFilterShrinksImportedGroup(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"keep","server":"good.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"drop","server":"bad.com","server_port":443,"uuid":"u2"},
	    {"type":"urltest","tag":"auto","outbounds":["keep","drop"]}
	  ]
	}`
	res := parseInlineBody(t, body, []map[string]string{{"tag": "drop"}})

	// Всего две записи: обычный "keep" и узел-группа "auto".
	if got := rawTagsOf(res); len(got) != 2 {
		t.Fatalf("entries = %v, want [keep auto]", got)
	}
	if res.Entries[0].RawTag != "keep" {
		t.Fatalf("first entry = %q, want keep", res.Entries[0].RawTag)
	}

	groups := groupEntriesOf(res)
	if len(groups) != 1 {
		t.Fatalf("got %d group entries, want 1", len(groups))
	}
	if members := groups[0].MemberRawTags; len(members) != 1 || members[0] != "keep" {
		t.Fatalf("group members = %v, want [keep]", members)
	}
}
