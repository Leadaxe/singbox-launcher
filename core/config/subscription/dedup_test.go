package subscription

import (
	"fmt"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 112-B часть A — дедуп записей источника по ключу подключения
// (`схема|сервер|порт|креденшл`).
//
// Дедуп — НЕ идентичность: он живёт один разбор, в состояние не пишется и
// на отметки выключения не влияет. Проверяется здесь только он.

// Регресс v1.5.2 в чистом виде: подписка darkline отдаёт 32 байт-одинаковых
// ss://, различающихся ТОЛЬКО подписью. v1.5.1 показывал один узел, v1.5.2 —
// все 32. Побеждает ПЕРВАЯ запись: её имя пользователь и увидит.
func TestDedupCollapses32ByteCopiesIntoOne(t *testing.T) {
	const uri = "ss://YWVzLTI1Ni1nY206c2VjcmV0cGFzcw@DARK-BOT:443"
	names := []string{"Хорватия", "Финляндия"}
	for i := 1; i <= 7; i++ {
		names = append(names, fmt.Sprintf("LTE %d", i))
	}
	lines := make([]string, 0, 32)
	for i := 0; i < 32; i++ {
		lines = append(lines, uri+"#"+names[i%len(names)]+fmt.Sprintf(" %d", i))
	}

	res := loadFromInlineBody(t, strings.Join(lines, "\n"), configtypes.ProxySource{})

	if len(res.Nodes) != 1 {
		got := make([]string, 0, len(res.Nodes))
		for _, n := range res.Nodes {
			got = append(got, n.Tag)
		}
		t.Fatalf("получено %d узлов, ожидался 1 (теги: %v)", len(res.Nodes), got)
	}
	if want := "Хорватия 0"; res.Nodes[0].Tag != want {
		t.Errorf("выжил узел %q, ожидался первый по порядку (%q)", res.Nodes[0].Tag, want)
	}
}

// ЗАФИКСИРОВАННОЕ СЛЕДСТВИЕ (решение пользователя, SPEC 112-B): ключ жёстче
// прежнего контент-хеша. Один сервер + один креденшл с РАЗНЫМИ SNI теперь
// один узел; v1.5.1 держал оба, LxBox всегда схлопывал — семантики двух
// приложений сходятся намеренно. Тест — пин решения, а не наблюдения.
func TestDedupCollapsesSameServerWithDifferentSNI(t *testing.T) {
	body := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=www.microsoft.com#MS",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=www.cloudflare.com#CF",
	}, "\n")

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 1 {
		t.Fatalf("получено %d узлов, ожидался 1 — SNI в ключ подключения не входит", len(res.Nodes))
	}
	if res.Nodes[0].Tag != "MS" {
		t.Errorf("выжил %q, ожидался первый (MS)", res.Nodes[0].Tag)
	}
}

// Разные креды на одном адресе — разные записи: это разные аккаунты, а не
// копии одной строки.
func TestDedupKeepsDistinctCredentials(t *testing.T) {
	body := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com#A",
		"vless://11111111-2222-3333-4444-555555555555@e.com:443?security=tls&sni=e.com#B",
	}, "\n")

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 2 {
		t.Fatalf("получено %d узлов, ожидалось 2 — креды разные", len(res.Nodes))
	}
}

// Разные серверы не схлопываются.
func TestDedupKeepsDistinctServers(t *testing.T) {
	body := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@a.com:443?security=tls&sni=a.com#A",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@b.com:443?security=tls&sni=b.com#B",
	}, "\n")

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 2 {
		t.Fatalf("получено %d узлов, ожидалось 2", len(res.Nodes))
	}
}

// Дедуп работает и для импортированных sing-box конфигов.
func TestDedupCollapsesDuplicatesInSingboxImport(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"first","server":"e.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"second","server":"e.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"other","server":"x.com","server_port":443,"uuid":"u1"}
	  ]
	}`
	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 2 {
		got := make([]string, 0, len(res.Nodes))
		for _, n := range res.Nodes {
			got = append(got, n.Tag)
		}
		t.Fatalf("получено %d узлов, ожидалось 2 (теги: %v)", len(res.Nodes), got)
	}
	if res.Nodes[0].Tag != "first" {
		t.Errorf("выжил %q, ожидался first", res.Nodes[0].Tag)
	}
}

// Дедуп не должен ломать состав узла-группы: если её член схлопнулся,
// группа обязана указывать на выжившего.
func TestDedupKeepsGroupMembershipConsistent(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"e.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"a-dup","server":"e.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"b","server":"x.com","server_port":443,"uuid":"u1"},
	    {"type":"urltest","tag":"auto","outbounds":["a","a-dup","b"]}
	  ]
	}`
	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	groups := groupNodesOf(res.Nodes)
	if len(groups) != 1 {
		t.Fatalf("получено %d узлов-групп, ожидалась 1", len(groups))
	}

	finalTags := map[string]bool{}
	for _, n := range res.Nodes {
		finalTags[n.Tag] = true
	}
	members := groupMembersOf(groups[0])
	if len(members) == 0 {
		t.Fatal("группа осталась без состава после дедупа")
	}
	for _, m := range members {
		if !finalTags[m] {
			t.Fatalf("группа ссылается на %q, которого нет среди узлов (состав: %v)", m, members)
		}
	}
}

// Пустой ключ (нет сервера или креденшла) не схлопывает: иначе все безымянные
// по секрету записи сложились бы в одну.
func TestDedupKeepsNodesWithoutConnKey(t *testing.T) {
	nodes := []*configtypes.ParsedNode{
		{Tag: "a", Scheme: "vless"},                             // нет сервера
		{Tag: "b", Scheme: "vless"},                             // нет сервера
		{Tag: "c", Scheme: "vless", Server: "e.com", Port: 443}, // нет креденшла
		{Tag: "d", Scheme: "vless", Server: "e.com", Port: 443},
	}
	if got := dedupParsedNodes(nodes); len(got) != 4 {
		t.Fatalf("получено %d узлов, ожидалось 4 — пустой ключ не схлопывает", len(got))
	}
}

// Узлы-группы ключа не имеют и через дедуп проходят всегда.
func TestDedupPassesGroupNodes(t *testing.T) {
	nodes := []*configtypes.ParsedNode{
		{Tag: "auto", Scheme: configtypes.SchemeGroup},
		{Tag: "auto2", Scheme: configtypes.SchemeGroup},
	}
	if got := dedupParsedNodes(nodes); len(got) != 2 {
		t.Fatalf("получено %d узлов, ожидалось 2 — группы дедуп не трогает", len(got))
	}
}

// Дедуп per-source: два ИСТОЧНИКА с одинаковой записью дают по узлу каждый.
// Ключ живёт один разбор — иначе вторая подписка выглядела бы пустой.
func TestDedupIsPerSource(t *testing.T) {
	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com#NL"

	tagCounts := map[string]int{}
	first := loadFromInlineBodyWithCounts(t, uri, configtypes.ProxySource{}, tagCounts)
	second := loadFromInlineBodyWithCounts(t, uri, configtypes.ProxySource{}, tagCounts)

	if len(first.Nodes) != 1 || len(second.Nodes) != 1 {
		t.Fatalf("получено %d и %d узлов, ожидалось по 1 — дедуп не должен переживать источник",
			len(first.Nodes), len(second.Nodes))
	}
}

// serverConnKey — то же семейство, что xrayServerKey: креденшл берётся из
// UUID, иначе из первого непустого password/uuid/private_key/auth_str.
func TestServerConnKeyPicksCredential(t *testing.T) {
	cases := []struct {
		name string
		node *configtypes.ParsedNode
		want string
	}{
		{
			name: "uuid",
			node: &configtypes.ParsedNode{Scheme: "VLESS", Server: "e.com", Port: 443, UUID: "u1"},
			want: "vless|e.com|443|u1",
		},
		{
			name: "password",
			node: &configtypes.ParsedNode{Scheme: "ss", Server: "e.com", Port: 443,
				Outbound: map[string]interface{}{"password": "p1"}},
			want: "ss|e.com|443|p1",
		},
		{
			name: "no server",
			node: &configtypes.ParsedNode{Scheme: "vless", UUID: "u1"},
			want: "",
		},
		{
			name: "no credential",
			node: &configtypes.ParsedNode{Scheme: "vless", Server: "e.com", Port: 443},
			want: "",
		},
		{
			name: "group",
			node: &configtypes.ParsedNode{Scheme: configtypes.SchemeGroup, Server: "e.com", Port: 443, UUID: "u1"},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serverConnKey(tc.node); got != tc.want {
				t.Fatalf("serverConnKey() = %q, ожидалось %q", got, tc.want)
			}
		})
	}
}

// xray-ownership на обобщение ключа не отреагировал: узел без опознанного
// секрета по-прежнему закрепляется за элементом (там ключ решает «чей адрес»,
// а не «та же ли это запись»).
func TestXrayServerKeyStillOwnsCredlessNodes(t *testing.T) {
	node := &configtypes.ParsedNode{Scheme: "vless", Server: "e.com", Port: 443}
	if got := xrayServerKey(node); got != "vless|e.com|443|" {
		t.Fatalf("xrayServerKey() = %q — ownership безымянного по секрету узла изменилась", got)
	}
}
