package config

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
)

// SPEC 112 — идентичность узла есть его тег в рамках источника.

func vlessNodeForIdentity(tag string) *ParsedNode {
	return &ParsedNode{
		Tag:    tag,
		Scheme: "vless",
		Server: "e.example.com",
		Port:   443,
		UUID:   "b831381d-6324-4d53-ad4f-8cda48b30811",
		Outbound: map[string]interface{}{
			"type":        "vless",
			"tag":         tag,
			"server":      "e.example.com",
			"server_port": 443,
			"uuid":        "b831381d-6324-4d53-ad4f-8cda48b30811",
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "e.example.com",
			},
		},
	}
}

// Идентичность — снятый парсером сырой тег, а не текущий Tag: тег переписывают
// префикс/маска/уникализация конфига, идентичность обязана это пережить.
func TestNodeIdentityIsTheStampedRawTag(t *testing.T) {
	node := vlessNodeForIdentity("NL:🇳🇱 NL-1")
	node.IdentityTag = "🇳🇱 NL-1"

	if got := NodeIdentity(node); got != "🇳🇱 NL-1" {
		t.Fatalf("NodeIdentity = %q, want the raw tag", got)
	}
}

// Смена tag_prefix / tag_mask источника идентичность не трогает: она снята до
// них, а Tag после них.
func TestNodeIdentitySurvivesTagPolicyChange(t *testing.T) {
	plain := vlessNodeForIdentity("node")
	subscription.StampNodeIdentity(plain, map[string]int{})

	prefixed := vlessNodeForIdentity("node")
	subscription.StampNodeIdentity(prefixed, map[string]int{})
	prefixed.Tag = "NL:node" // как если бы источнику задали tag_prefix
	prefixed.Outbound["tag"] = "NL:node"

	if NodeIdentity(plain) != NodeIdentity(prefixed) {
		t.Fatalf("tag policy must not change identity: %q vs %q",
			NodeIdentity(plain), NodeIdentity(prefixed))
	}
}

// Ключевое требование SPEC 112: содержимое узла в идентичность не входит.
// Провайдер вправе поменять сервер под тем же именем — это ТОТ ЖЕ узел.
func TestNodeIdentityIgnoresConnectionFields(t *testing.T) {
	base := vlessNodeForIdentity("node")
	subscription.StampNodeIdentity(base, map[string]int{})
	want := NodeIdentity(base)
	if want == "" {
		t.Fatal("базовая идентичность пуста")
	}

	tests := []struct {
		name   string
		mutate func(n *ParsedNode)
	}{
		{"port", func(n *ParsedNode) { n.Port = 8443; n.Outbound["server_port"] = 8443 }},
		{"server", func(n *ParsedNode) { n.Server = "other.example.com"; n.Outbound["server"] = "other.example.com" }},
		{"uuid", func(n *ParsedNode) {
			n.UUID = "11111111-2222-3333-4444-555555555555"
			n.Outbound["uuid"] = "11111111-2222-3333-4444-555555555555"
		}},
		{"sni", func(n *ParsedNode) {
			n.Outbound["tls"].(map[string]interface{})["server_name"] = "www.microsoft.com"
		}},
		{"transport", func(n *ParsedNode) {
			n.Outbound["transport"] = map[string]interface{}{"type": "ws", "path": "/ws"}
		}},
		{"detour", func(n *ParsedNode) { n.Outbound["detour"] = "some-jump" }},
	}

	for _, tt := range tests {
		t.Run(tt.name+" не меняет идентичность", func(t *testing.T) {
			node := vlessNodeForIdentity("node")
			subscription.StampNodeIdentity(node, map[string]int{})
			tt.mutate(node)
			if got := NodeIdentity(node); got != want {
				t.Fatalf("правка %s увела идентичность: %q → %q", tt.name, want, got)
			}
		})
	}
}

// Критерий приёмки 2: смена формы хранения узла (uri ↔ config_json) не меняет
// идентичность. Именно на этом ломалась ссылка detour в стейте IRA.
func TestNodeIdentitySurvivesStorageFormChange(t *testing.T) {
	const tag = "🔥🎭 WARP (MASQUE)"

	fromURI := vlessNodeForIdentity(tag)
	subscription.StampNodeIdentity(fromURI, map[string]int{})

	// Тот же узел, приехавший ручным config_json: эмиссия у него другая
	// (EmitRaw, лишние поля), а имя — то же.
	fromJSON := vlessNodeForIdentity(tag)
	fromJSON.EmitRaw = true
	fromJSON.Outbound["packet_encoding"] = "xudp"
	fromJSON.Outbound["domain_strategy"] = "prefer_ipv4"
	subscription.StampNodeIdentity(fromJSON, map[string]int{})

	if NodeIdentity(fromURI) != NodeIdentity(fromJSON) {
		t.Fatalf("форма хранения увела идентичность: %q vs %q",
			NodeIdentity(fromURI), NodeIdentity(fromJSON))
	}
	// А legacy-хеш их как раз и разводил — ради этого он и снесён.
	if LegacyNodeIdentityHash(fromURI) == LegacyNodeIdentityHash(fromJSON) {
		t.Skip("legacy-хеши совпали — тест перестал воспроизводить исходную ловушку")
	}
}

// Переименование провайдером — это смена имени, а значит и идентичности.
// Отметка выключения при этом честно теряется (по TTL), но не переезжает
// молча на чужой узел.
func TestNodeIdentityChangesOnProviderRename(t *testing.T) {
	a := vlessNodeForIdentity("🇳🇱 NL-1")
	subscription.StampNodeIdentity(a, map[string]int{})
	b := vlessNodeForIdentity("Amsterdam Fast")
	subscription.StampNodeIdentity(b, map[string]int{})

	if NodeIdentity(a) == NodeIdentity(b) {
		t.Fatal("узлы с разными именами обязаны иметь разные идентичности")
	}
}

// Дубли имён внутри источника разводятся тем же правилом, что теги конфига.
func TestNodeIdentityUniquifiesDuplicatesWithinSource(t *testing.T) {
	idCounts := map[string]int{}
	first := vlessNodeForIdentity("🇳🇱 NL")
	second := vlessNodeForIdentity("🇳🇱 NL")
	third := vlessNodeForIdentity("🇳🇱 NL")
	subscription.StampNodeIdentity(first, idCounts)
	subscription.StampNodeIdentity(second, idCounts)
	subscription.StampNodeIdentity(third, idCounts)

	want := []string{"🇳🇱 NL", "🇳🇱 NL-2", "🇳🇱 NL-3"}
	got := []string{NodeIdentity(first), NodeIdentity(second), NodeIdentity(third)}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("идентичность #%d = %q, ожидалась %q (все: %v)", i+1, got[i], want[i], got)
		}
	}
}

// Узел-группа идентичности не имеет: цепляться через selector — задача
// DetourTag (SPEC 077), отметок выключения у групп нет.
func TestNodeIdentityGroupHasNone(t *testing.T) {
	group := &ParsedNode{Tag: "🚀 Авто", Scheme: configtypes.SchemeGroup}
	if got := NodeIdentity(group); got != "" {
		t.Fatalf("идентичность группы = %q, ожидалась пустая", got)
	}
	if got := subscription.StampNodeIdentity(group, map[string]int{}); got != "" {
		t.Fatalf("StampNodeIdentity проштамповал группу: %q", got)
	}
}

func TestNodeIdentityNilNode(t *testing.T) {
	if got := NodeIdentity(nil); got != "" {
		t.Fatalf("идентичность nil-узла = %q, ожидалась пустая", got)
	}
}

// Legacy-хеш обязан остаться воспроизводимым — на нём держится миграция
// (SPEC 112, пункт 7). Тест страхует от «почистили заодно с эмиттером».
func TestLegacyNodeIdentityHashStillReproducible(t *testing.T) {
	node := vlessNodeForIdentity("node")
	first := LegacyNodeIdentityHash(node)
	if len(first) != 64 {
		t.Fatalf("legacy-хеш = %q, ожидались 64 hex-символа", first)
	}
	// Тег в хеш не входил — миграция обязана опознать узел после
	// переименования конфиговых тегов префиксом.
	renamed := vlessNodeForIdentity("NL:node")
	if LegacyNodeIdentityHash(renamed) != first {
		t.Fatal("legacy-хеш стал зависеть от тега — миграция перестанет опознавать узлы")
	}
	if got := LegacyNodeIdentityHash(nil); got != "" {
		t.Fatalf("legacy-хеш nil-узла = %q, ожидался пустой", got)
	}
}

// SPEC 101: wireguard-узлы эмитятся через GenerateEndpointJSON — legacy-хеш
// обязан видеть полную карту endpoint'а, иначе миграция отметок на WG
// перепутает узлы одного server:port.
func TestLegacyNodeIdentityHashWireGuardDistinguishesKeys(t *testing.T) {
	uri1 := "wireguard://UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=@1.2.3.4:51820?publickey=QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU=&address=10.0.0.2/32&allowedips=0.0.0.0/0#a"
	uri2 := "wireguard://UFNLMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=@1.2.3.4:51820?publickey=QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU=&address=10.9.9.9/32&allowedips=0.0.0.0/0#b"
	n1, err := subscription.ParseNode(uri1, nil)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := subscription.ParseNode(uri2, nil)
	if err != nil {
		t.Fatal(err)
	}
	h1, h2 := LegacyNodeIdentityHash(n1), LegacyNodeIdentityHash(n2)
	if h1 == "" || h2 == "" {
		t.Fatalf("пустой legacy-хеш: h1=%q h2=%q", h1, h2)
	}
	if h1 == h2 {
		t.Fatalf("WG-узлы с разными ключами схлопнулись в один legacy-хеш %s", h1)
	}
}
