package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 094 B3 — эмиссия detour-цепочек произвольной глубины.
//
// Парная сторона к core/config/subscription/detour_chain_test.go: там
// проверяется разбор, здесь — что из модели получается корректный JSON.

func hopNode(tag, scheme, server string, port int) *ParsedNode {
	return &ParsedNode{
		Tag:    tag,
		Scheme: scheme,
		Server: server,
		Port:   port,
		Outbound: map[string]interface{}{
			"type":        scheme,
			"tag":         tag,
			"server":      server,
			"server_port": port,
		},
	}
}

// chainOfNode отдаёт Chain как есть, когда он заполнен.
func TestChainOfNodeUsesChain(t *testing.T) {
	node := &ParsedNode{
		Tag:    "main",
		Scheme: "vless",
		Chain: []*ParsedNode{
			hopNode("hop1", "socks", "h1.com", 1080),
			hopNode("hop2", "vless", "h2.com", 443),
		},
	}

	chain := chainOfNode(node)
	if len(chain) != 2 {
		t.Fatalf("chain length = %d, want 2", len(chain))
	}
	if chain[0].Tag != "hop1" || chain[1].Tag != "hop2" {
		t.Fatalf("chain order = %q, %q; want hop1, hop2", chain[0].Tag, chain[1].Tag)
	}
}

// Legacy Jump без Chain продолжает работать (обратная совместимость).
func TestChainOfNodePromotesLegacyJump(t *testing.T) {
	node := &ParsedNode{
		Tag:    "main",
		Scheme: "vless",
		Jump: &configtypes.ParsedJump{
			Tag:      "legacy",
			Server:   "j.com",
			Port:     1080,
			Outbound: map[string]interface{}{"type": "socks", "tag": "legacy"},
		},
	}

	chain := chainOfNode(node)
	if len(chain) != 1 {
		t.Fatalf("chain length = %d, want 1", len(chain))
	}
	if chain[0].Tag != "legacy" {
		t.Fatalf("hop tag = %q, want legacy", chain[0].Tag)
	}
	// Пустая схема исторически означает socks.
	if chain[0].Scheme != "socks" {
		t.Fatalf("empty scheme must default to socks, got %q", chain[0].Scheme)
	}
	// SOCKS без явной версии ядро отвергает.
	if chain[0].Outbound["version"] != "5" {
		t.Fatalf("socks hop must default to version 5, got %v", chain[0].Outbound["version"])
	}
}

func TestChainOfNodeEmptyForPlainNode(t *testing.T) {
	node := &ParsedNode{Tag: "main", Scheme: "vless"}
	if chain := chainOfNode(node); len(chain) != 0 {
		t.Fatalf("plain node must have no chain, got %d hops", len(chain))
	}
	if chain := chainOfNode(nil); chain != nil {
		t.Fatal("nil node must yield nil chain")
	}
}

// Каждый хоп эмитится отдельным outbound'ом и остаётся валидным JSON.
func TestChainHopsEmitValidJSON(t *testing.T) {
	hops := []*ParsedNode{
		hopNode("hop1", "socks", "h1.com", 1080),
		hopNode("hop2", "trojan", "h2.com", 443),
	}
	hops[1].UUID = "secret"

	for _, hop := range hops {
		out, err := GenerateNodeJSON(hop)
		if err != nil {
			t.Fatalf("GenerateNodeJSON(%s) error: %v", hop.Tag, err)
		}
		assertValidOutboundJSON(t, out)
		if !strings.Contains(out, hop.Tag) {
			t.Errorf("emitted JSON for %s does not carry its tag: %s", hop.Tag, out)
		}
	}
}

// assertValidOutboundJSON проверяет, что строка из GenerateNodeJSON —
// синтаксически валидный JSON-объект после снятия обрамления генератора
// (ведущий комментарий и хвостовая запятая).
func assertValidOutboundJSON(t *testing.T, emitted string) {
	t.Helper()

	body := emitted
	if idx := strings.Index(body, "{"); idx >= 0 {
		body = body[idx:]
	}
	body = strings.TrimSpace(body)
	body = strings.TrimSuffix(body, ",")

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		t.Fatalf("emitted outbound is not valid JSON: %v\n%s", err, emitted)
	}
	if _, ok := obj["type"]; !ok {
		t.Fatalf("emitted outbound has no type: %s", emitted)
	}
}
