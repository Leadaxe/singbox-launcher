package config

import (
	"testing"

	"singbox-launcher/core/config/subscription"
)

// SPEC 094 D2 — стабильная идентичность узла.

func vlessNodeForHash(tag string) *ParsedNode {
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

// Переименование провайдером не меняет идентичность.
func TestNodeIdentityHashIgnoresTag(t *testing.T) {
	a := vlessNodeForHash("🇳🇱 NL-1")
	b := vlessNodeForHash("Amsterdam Fast")

	ha, hb := NodeIdentityHash(a), NodeIdentityHash(b)
	if ha == "" || hb == "" {
		t.Fatalf("hash must not be empty (a=%q b=%q)", ha, hb)
	}
	if ha != hb {
		t.Fatalf("renaming a node must not change its hash:\n a=%s\n b=%s", ha, hb)
	}
}

// tag_prefix / tag_mask источника тоже не меняют идентичность: они переписывают
// только tag, а он из хеша исключён.
func TestNodeIdentityHashIgnoresTagPrefix(t *testing.T) {
	plain := vlessNodeForHash("node")

	prefixed := vlessNodeForHash("NL-node")
	prefixed.Tag = "NL-node"
	prefixed.Outbound["tag"] = "NL-node"

	if NodeIdentityHash(plain) != NodeIdentityHash(prefixed) {
		t.Fatal("tag prefix must not change the hash")
	}
}

// detour исключён: смена джампа не отвязывает пользовательский выбор от узла.
func TestNodeIdentityHashIgnoresDetour(t *testing.T) {
	plain := vlessNodeForHash("node")

	chained := vlessNodeForHash("node")
	chained.Outbound["detour"] = "some-jump"

	if NodeIdentityHash(plain) != NodeIdentityHash(chained) {
		t.Fatal("detour must not change the hash")
	}
}

// Всё, что описывает подключение, идентичность меняет.
func TestNodeIdentityHashReactsToConnectionFields(t *testing.T) {
	base := NodeIdentityHash(vlessNodeForHash("node"))
	if base == "" {
		t.Fatal("base hash is empty")
	}

	tests := []struct {
		name   string
		mutate func(n *ParsedNode)
	}{
		{
			name: "port",
			mutate: func(n *ParsedNode) {
				n.Port = 8443
				n.Outbound["server_port"] = 8443
			},
		},
		{
			name: "server",
			mutate: func(n *ParsedNode) {
				n.Server = "other.example.com"
				n.Outbound["server"] = "other.example.com"
			},
		},
		{
			name: "uuid",
			mutate: func(n *ParsedNode) {
				n.UUID = "11111111-2222-3333-4444-555555555555"
				n.Outbound["uuid"] = "11111111-2222-3333-4444-555555555555"
			},
		},
		{
			// Ключевое отличие от LxBox: один сервер с двумя SNI — две ноды.
			name: "sni",
			mutate: func(n *ParsedNode) {
				n.Outbound["tls"].(map[string]interface{})["server_name"] = "www.microsoft.com"
			},
		},
		{
			name: "transport",
			mutate: func(n *ParsedNode) {
				n.Outbound["transport"] = map[string]interface{}{"type": "ws", "path": "/ws"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" changes the hash", func(t *testing.T) {
			node := vlessNodeForHash("node")
			tt.mutate(node)
			if got := NodeIdentityHash(node); got == base {
				t.Fatalf("changing %s must change the hash (still %s)", tt.name, got)
			}
		})
	}
}

// Хеш не зависит от порядка вставки ключей в map.
func TestNodeIdentityHashIsOrderIndependent(t *testing.T) {
	a := vlessNodeForHash("node")
	b := vlessNodeForHash("node")

	// Пересобираем tls-блок в другом порядке вставки.
	b.Outbound["tls"] = map[string]interface{}{}
	tls := b.Outbound["tls"].(map[string]interface{})
	tls["server_name"] = "e.example.com"
	tls["enabled"] = true

	if NodeIdentityHash(a) != NodeIdentityHash(b) {
		t.Fatal("hash must not depend on map insertion order")
	}
}

// Хеш детерминирован между вызовами.
func TestNodeIdentityHashIsStableAcrossCalls(t *testing.T) {
	node := vlessNodeForHash("node")
	first := NodeIdentityHash(node)
	for i := 0; i < 10; i++ {
		if got := NodeIdentityHash(node); got != first {
			t.Fatalf("hash is not deterministic: %s vs %s", first, got)
		}
	}
}

// Label/Comment — display-текст, в объект outbound'а не попадают и на хеш
// не влияют, отдельного исключения не требуют.
func TestNodeIdentityHashIgnoresLabelAndComment(t *testing.T) {
	plain := vlessNodeForHash("node")

	labeled := vlessNodeForHash("node")
	labeled.Label = "Some flashy label"
	labeled.Comment = "user comment"

	if NodeIdentityHash(plain) != NodeIdentityHash(labeled) {
		t.Fatal("label/comment must not change the hash")
	}
}

func TestNodeIdentityHashNilNode(t *testing.T) {
	if got := NodeIdentityHash(nil); got != "" {
		t.Fatalf("nil node hash = %q, want empty", got)
	}
}

// SPEC 101: wireguard endpoints emit through GenerateEndpointJSON, not the
// per-scheme outbound switch — the hash must still see the full endpoint map
// (keys, addresses), or two WG nodes on one server:port collapse to one hash.
func TestNodeIdentityHashWireGuardDistinguishesKeys(t *testing.T) {
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
	h1, h2 := NodeIdentityHash(n1), NodeIdentityHash(n2)
	if h1 == "" || h2 == "" {
		t.Fatalf("empty hash: h1=%q h2=%q", h1, h2)
	}
	if h1 == h2 {
		t.Fatalf("wireguard nodes with different keys/addresses collapsed to one hash %s", h1)
	}
}
