package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 112-B часть A — дедуп записей источника по подписи содержимого
// (полная эмиссия без tag/detour; вердикт 26.08: разные SNI/транспорты —
// разные записи).
//
// Дедуп — НЕ идентичность: он живёт один разбор, в состояние не пишется и
// на отметки выключения не влияет. Проверяется здесь только он.

// withContentSignatureHook подставляет полноконтентную подпись: в проде хук
// ставит init пакета config (sha256 от эмиссии), тестам пакета subscription
// config недоступен (цикл импорта). Стаб обязан различать SNI и транспорт —
// на этом держится вердикт — поэтому хеширует ВЕСЬ узел без Tag/IdentityTag,
// а не тройку scheme|server|uuid, как стаб миграционных тестов.
func withContentSignatureHook(t *testing.T) {
	t.Helper()
	prev := LegacyNodeIdentityHashFunc
	LegacyNodeIdentityHashFunc = func(node *configtypes.ParsedNode) string {
		if node == nil || node.Scheme == configtypes.SchemeGroup {
			return ""
		}
		if strings.TrimSpace(node.Server) == "" {
			return "" // приближение «эмиссия не удалась» — подписи нет
		}
		clone := *node
		// Всё именное — вне подписи, как tag/detour вне эмиссии.
		clone.Tag = ""
		clone.IdentityTag = ""
		clone.SourceTag = ""
		clone.Label = ""
		clone.Comment = ""
		if node.Outbound != nil {
			m := make(map[string]interface{}, len(node.Outbound))
			for k, v := range node.Outbound {
				if k == "tag" || k == "detour" {
					continue
				}
				m[k] = v
			}
			clone.Outbound = m
		}
		b, err := json.Marshal(&clone)
		if err != nil {
			return ""
		}
		sum := sha256.Sum256(b)
		return hex.EncodeToString(sum[:])
	}
	t.Cleanup(func() { LegacyNodeIdentityHashFunc = prev })
}

// Регресс v1.5.2 в чистом виде: мок реальной подписки — 32 байт-одинаковых
// ss://, различающихся ТОЛЬКО подписью. v1.5.1 показывал один узел, v1.5.2 —
// все 32. Побеждает ПЕРВАЯ запись: её имя пользователь и увидит.
func TestDedupCollapses32ByteCopiesIntoOne(t *testing.T) {
	withContentSignatureHook(t)
	const uri = "ss://YWVzLTI1Ni1nY206c2VjcmV0cGFzcw@dup-pool.example:443"
	names := []string{"Страна А", "Страна Б"}
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
	if want := "Страна А 0"; res.Nodes[0].Tag != want {
		t.Errorf("выжил узел %q, ожидался первый по порядку (%q)", res.Nodes[0].Tag, want)
	}
}

// Пин вердикта пользователя (SPEC 112-B, уточнение 26.08): разные SNI одного
// сервера с одним кредом — РАЗНЫЕ записи. Разный SNI = разный способ пройти
// фильтрацию; первая редакция дедупа по кредам их склеивала — откатились к
// семантике v1.5.1 (подпись = полная эмиссия).
func TestDedupKeepsSameServerWithDifferentSNI(t *testing.T) {
	withContentSignatureHook(t)
	body := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=www.microsoft.com#MS",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=www.cloudflare.com#CF",
	}, "\n")

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 2 {
		t.Fatalf("получено %d узлов, ожидалось 2 — разные SNI это разные схемы обхода", len(res.Nodes))
	}
}

// Тот же вердикт для транспортов: grpc- и xhttp-варианты одного сервера с
// одним кредом (реальный кейс из подписки пользователя) — два узла, не один.
func TestDedupKeepsSameServerWithDifferentTransport(t *testing.T) {
	withContentSignatureHook(t)
	body := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com&type=grpc&serviceName=svc#GRPC",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com&type=xhttp&path=%2Fx#XHTTP",
	}, "\n")

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 2 {
		t.Fatalf("получено %d узлов, ожидалось 2 — разные транспорты это разные соединения", len(res.Nodes))
	}
}

// Разные креды на одном адресе — разные записи: это разные аккаунты, а не
// копии одной строки.
func TestDedupKeepsDistinctCredentials(t *testing.T) {
	withContentSignatureHook(t)
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
	withContentSignatureHook(t)
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
	withContentSignatureHook(t)
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
	withContentSignatureHook(t)
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
// по секрету записи сложились бы в одну. А вот одинаковые ПО СОДЕРЖИМОМУ
// записи без креденшла — честные дубли (семантика v1.5.1): подпись — полная
// эмиссия, креденшл ей не обязателен.
func TestDedupKeepsNodesWithoutSignature(t *testing.T) {
	withContentSignatureHook(t)
	nodes := []*configtypes.ParsedNode{
		{Tag: "a", Scheme: "vless"},                             // нет сервера → подписи нет
		{Tag: "b", Scheme: "vless"},                             // нет сервера → подписи нет
		{Tag: "c", Scheme: "vless", Server: "e.com", Port: 443}, // без креденшла, но контент одинаковый…
		{Tag: "d", Scheme: "vless", Server: "e.com", Port: 443}, // …значит дубль — схлопывается
	}
	if got := DedupParsedNodes(nodes); len(got) != 3 {
		t.Fatalf("получено %d узлов, ожидалось 3 — без подписи не схлопываем, контент-дубли схлопываем", len(got))
	}
}

// Узлы-группы ключа не имеют и через дедуп проходят всегда.
func TestDedupPassesGroupNodes(t *testing.T) {
	withContentSignatureHook(t)
	nodes := []*configtypes.ParsedNode{
		{Tag: "auto", Scheme: configtypes.SchemeGroup},
		{Tag: "auto2", Scheme: configtypes.SchemeGroup},
	}
	if got := DedupParsedNodes(nodes); len(got) != 2 {
		t.Fatalf("получено %d узлов, ожидалось 2 — группы дедуп не трогает", len(got))
	}
}

// Дедуп per-source: два ИСТОЧНИКА с одинаковой записью дают по узлу каждый.
// Ключ живёт один разбор — иначе вторая подписка выглядела бы пустой.
func TestDedupIsPerSource(t *testing.T) {
	withContentSignatureHook(t)
	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com#NL"

	tagCounts := map[string]int{}
	first := loadFromInlineBodyWithCounts(t, uri, configtypes.ProxySource{}, tagCounts)
	second := loadFromInlineBodyWithCounts(t, uri, configtypes.ProxySource{}, tagCounts)

	if len(first.Nodes) != 1 || len(second.Nodes) != 1 {
		t.Fatalf("получено %d и %d узлов, ожидалось по 1 — дедуп не должен переживать источник",
			len(first.Nodes), len(second.Nodes))
	}
}

// SPEC 113-A §2: ключ по кредам упразднён — во всём парсере остался ОДИН ключ,
// подпись содержимого. xray-ownership считает её же, значит группы и записи
// схлопываются по одному правилу.
func TestXrayServerKeyUsesTheSameSignatureAsDedup(t *testing.T) {
	withContentSignatureHook(t)

	node := &configtypes.ParsedNode{Scheme: "vless", Server: "e.com", Port: 443, UUID: "u1"}
	if got, want := xrayServerKey(node), dedupSignature(node); got != want {
		t.Fatalf("xrayServerKey() = %q, дедуп считает %q — ключ во всём парсере обязан быть один", got, want)
	}

	// Узел-группа подписи не имеет ни там, ни там.
	group := &configtypes.ParsedNode{Scheme: configtypes.SchemeGroup, Server: "e.com", Port: 443, UUID: "u1"}
	if got := xrayServerKey(group); got != "" {
		t.Fatalf("узел-группа получил подпись %q", got)
	}

	// Разные креды на одном адресе — разные подписи (это разные аккаунты).
	other := &configtypes.ParsedNode{Scheme: "vless", Server: "e.com", Port: 443, UUID: "u2"}
	if xrayServerKey(node) == xrayServerKey(other) {
		t.Fatal("узлы с разными кредами получили одну подпись")
	}
}
