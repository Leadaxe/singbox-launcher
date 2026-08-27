package subscription

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 113-A §4 — группа не теряет схлопнутых членов (находка аудита M1).
//
// Дедуп выбрасывает байт-копию ДО простановки тегов, а состав группы
// перечисляет членов исходными тегами. Раньше выброшенный член просто не
// находился в карте «исходный → итоговый» и выпадал из состава; группа,
// собранная целиком из копий, теряла ВСЕХ членов и удалялась с warning'ом,
// хотя её узлы живы под другими именами.

// Группа перечисляет только копии — обязана выжить с составом из выживших
// оригиналов.
func TestGroupOfCollapsedMembersSurvivesWithSurvivors(t *testing.T) {
	withContentSignatureHook(t)

	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"a.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"b","server":"b.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"a2","server":"a.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"b2","server":"b.com","server_port":443,"uuid":"u1"},
	    {"type":"urltest","tag":"auto","outbounds":["a2","b2"]}
	  ]
	}`

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	groups := groupNodesOf(res.Nodes)
	if len(groups) != 1 {
		t.Fatalf("получено %d групп, ожидалась 1 — группа из одних копий не должна умирать (узлы: %v)",
			len(groups), tagsOfNodes(res.Nodes))
	}
	members := groupMembersOf(groups[0])
	if len(members) != 2 || members[0] != "a" || members[1] != "b" {
		t.Fatalf("состав группы = %v, ожидался [a b] — теги выживших копий", members)
	}
}

// Повтор члена после перепривязки схлопывается: и оригинал, и его копия
// указывают на один выживший тег, а дубль тега в outbounds ядро отвергает.
func TestGroupMembersDeduplicatedAfterRebind(t *testing.T) {
	withContentSignatureHook(t)

	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"a.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"a2","server":"a.com","server_port":443,"uuid":"u1"},
	    {"type":"urltest","tag":"auto","outbounds":["a","a2"]}
	  ]
	}`

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	groups := groupNodesOf(res.Nodes)
	if len(groups) != 1 {
		t.Fatalf("получено %d групп, ожидалась 1", len(groups))
	}
	members := groupMembersOf(groups[0])
	if len(members) != 1 || members[0] != "a" {
		t.Fatalf("состав группы = %v, ожидался [a] без повтора", members)
	}
}

// Превью ≡ боевой разбор: DedupParsedNodes (его зовёт вкладка Preview окна
// источника) обязан перепривязывать состав так же, а не показывать группу,
// ссылающуюся в пустоту.
func TestDedupParsedNodesRebindsGroupMembers(t *testing.T) {
	withContentSignatureHook(t)

	node := func(tag, server string) *configtypes.ParsedNode {
		return &configtypes.ParsedNode{
			Tag: tag, Scheme: "vless", Server: server, Port: 443, UUID: "u1",
			Outbound: map[string]interface{}{"type": "vless", "tag": tag, "server": server},
		}
	}
	group := &configtypes.ParsedNode{
		Tag: "auto", Scheme: configtypes.SchemeGroup,
		Outbound: map[string]interface{}{
			"type":                      "urltest",
			configtypes.GroupMembersKey: []interface{}{"a2", "b"},
		},
	}
	got := DedupParsedNodes([]*configtypes.ParsedNode{
		node("a", "a.com"), node("b", "b.com"), node("a2", "a.com"), group,
	})

	if len(got) != 3 {
		t.Fatalf("получено %d узлов, ожидалось 3 (a, b, группа): %v", len(got), tagsOfNodes(got))
	}
	members := groupMembersOf(group)
	if len(members) != 2 || members[0] != "a" || members[1] != "b" {
		t.Fatalf("состав превью = %v, ожидался [a b]", members)
	}
}

// default группы, указывающий на схлопнутую копию, тоже переезжает на
// выжившего: снятый default молча меняет поведение группы.
func TestGroupDefaultFollowsCollapsedMember(t *testing.T) {
	withContentSignatureHook(t)

	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"a.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"a2","server":"a.com","server_port":443,"uuid":"u1"},
	    {"type":"selector","tag":"pick","outbounds":["a2"],"default":"a2"}
	  ]
	}`

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	groups := groupNodesOf(res.Nodes)
	if len(groups) != 1 {
		t.Fatalf("получено %d групп, ожидалась 1", len(groups))
	}
	def, _ := groups[0].Outbound["default"].(string)
	if def != "a" {
		t.Fatalf("default = %q, ожидался тег выжившей копии %q", def, "a")
	}
}
