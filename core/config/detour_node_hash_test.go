package config

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// SPEC 101 — hash-addressed source detour: resolveNodeHashDetours.

const hashDetourHopURI = "masque://cHJpdmF0ZQ==@1.1.1.1:443?publickey=cHVi&sni=example.com&address=172.16.0.2%2F32#hop"

func hashDetourParserConfig(t *testing.T, sources ...ProxySource) *ParserConfig {
	t.Helper()
	pc := &ParserConfig{}
	pc.ParserConfig.Proxies = sources
	return pc
}

func parseNodeForHashDetour(t *testing.T, uri string) *ParsedNode {
	t.Helper()
	n, err := subscription.ParseNode(uri, nil)
	if err != nil {
		t.Fatalf("parse %q: %v", uri, err)
	}
	return n
}

// The happy path: a source chained through a hop node by hash gets the hop's
// FINAL tag stamped on every node, whatever renames/prefixes the hop received.
func TestResolveNodeHashDetours_StampsFinalTag(t *testing.T) {
	hop := parseNodeForHashDetour(t, hashDetourHopURI)
	hash := NodeIdentityHash(hop)
	if hash == "" {
		t.Fatal("hop hash must not be empty")
	}
	hop.Tag = "renamed-by-prefix:hop-7" // final tag differs from URI fragment
	hop.Outbound["tag"] = hop.Tag
	hop.SourceIndex = 0

	chained := parseNodeForHashDetour(t,
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@p.example.com:443?encryption=none&security=tls&sni=p.example.com#proton")
	chained.SourceIndex = 1

	pc := hashDetourParserConfig(t,
		ProxySource{Connections: []string{hashDetourHopURI}},
		ProxySource{Connections: []string{"..."}, DetourNodeHash: hash},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all := resolveNodeHashDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 2 {
		t.Fatalf("no nodes may be dropped, got %d", len(all))
	}
	if got, _ := chained.Outbound["detour"].(string); got != hop.Tag {
		t.Errorf("detour = %q, want final hop tag %q", got, hop.Tag)
	}
	if _, ok := hop.Outbound["detour"]; ok {
		t.Error("hop itself must keep dialing directly")
	}
}

// Fail-closed: an unresolvable hash drops the dependent source's nodes rather
// than letting its traffic dial direct.
func TestResolveNodeHashDetours_UnresolvedDropsSource(t *testing.T) {
	hop := parseNodeForHashDetour(t, hashDetourHopURI)
	hop.SourceIndex = 0
	chained := parseNodeForHashDetour(t,
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@p.example.com:443?encryption=none&security=tls&sni=p.example.com#proton")
	chained.SourceIndex = 1

	pc := hashDetourParserConfig(t,
		ProxySource{Connections: []string{hashDetourHopURI}},
		ProxySource{Connections: []string{"..."}, DetourNodeHash: strings.Repeat("f", 64), DetourNodeLabel: "gone-hop"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {chained}}
	all := resolveNodeHashDetours(pc, nodesBySource, []*ParsedNode{hop, chained})

	if len(all) != 1 || all[0] != hop {
		t.Fatalf("chained source must be dropped, hop kept; got %d node(s)", len(all))
	}
	if _, ok := nodesBySource[1]; ok {
		t.Error("dropped source must leave nodesBySource (selectors would reference ghosts)")
	}
}

// A wireguard endpoint can be the chained side too (SPEC 101 rides on the
// endpoint detour support) — but listen_port nodes stay unstamped: the core
// rejects detour+listen_port and one such node would kill the whole config.
func TestResolveNodeHashDetours_WireGuardChained(t *testing.T) {
	hop := parseNodeForHashDetour(t, hashDetourHopURI)
	hash := NodeIdentityHash(hop)
	hop.SourceIndex = 0

	wg := parseNodeForHashDetour(t,
		"wireguard://UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=@1.2.3.4:51820?publickey=QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU=&address=10.0.0.2/32&allowedips=0.0.0.0/0#wg")
	wg.SourceIndex = 1
	wgListen := parseNodeForHashDetour(t,
		"wireguard://UFNLMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=@1.2.3.4:51820?publickey=QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU=&address=10.0.0.3/32&allowedips=0.0.0.0/0&listenport=51999#wgl")
	wgListen.SourceIndex = 1

	pc := hashDetourParserConfig(t,
		ProxySource{Connections: []string{hashDetourHopURI}},
		ProxySource{Connections: []string{"..."}, DetourNodeHash: hash},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hop}, 1: {wg, wgListen}}
	all := resolveNodeHashDetours(pc, nodesBySource, []*ParsedNode{hop, wg, wgListen})

	if len(all) != 3 {
		t.Fatalf("nothing may be dropped, got %d", len(all))
	}
	if got, _ := wg.Outbound["detour"].(string); got != hop.Tag {
		t.Errorf("wireguard node detour = %q, want %q", got, hop.Tag)
	}
	if _, ok := wgListen.Outbound["detour"]; ok {
		t.Error("wireguard node with listen_port must stay unstamped")
	}
}
