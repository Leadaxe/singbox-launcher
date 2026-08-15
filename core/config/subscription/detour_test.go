package subscription

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 077: applySourceDetour stamps detour on eligible nodes only.
func TestApplySourceDetour(t *testing.T) {
	vless := &configtypes.ParsedNode{Tag: "v", Scheme: "vless", Outbound: map[string]interface{}{"type": "vless"}}
	wg := &configtypes.ParsedNode{Tag: "w", Scheme: "wireguard", Outbound: map[string]interface{}{"type": "wireguard"}}
	wgListen := &configtypes.ParsedNode{Tag: "wl", Scheme: "wireguard", Outbound: map[string]interface{}{"type": "wireguard", "listen_port": 51820}}
	withJump := &configtypes.ParsedNode{
		Tag: "j", Scheme: "vless", Outbound: map[string]interface{}{"type": "vless"},
		Jump: &configtypes.ParsedJump{Tag: "j_hop"},
	}
	nilOut := &configtypes.ParsedNode{Tag: "n", Scheme: "trojan"}

	ApplySourceDetour([]*configtypes.ParsedNode{vless, wg, wgListen, withJump, nilOut}, "hop-out")

	if got, _ := vless.Outbound["detour"].(string); got != "hop-out" {
		t.Errorf("vless detour = %q, want hop-out", got)
	}
	// sing-box-lx wireguard endpoints dial peers through detour like any outbound.
	if got, _ := wg.Outbound["detour"].(string); got != "hop-out" {
		t.Errorf("wireguard detour = %q, want hop-out", got)
	}
	// ...except with listen_port: the core rejects detour+listen_port.
	if _, ok := wgListen.Outbound["detour"]; ok {
		t.Error("wireguard with listen_port must not get a detour")
	}
	if _, ok := withJump.Outbound["detour"]; ok {
		t.Error("node with Xray Jump must not get a source detour (Jump wins)")
	}
	// nil Outbound is materialized then stamped.
	if got, _ := nilOut.Outbound["detour"].(string); got != "hop-out" {
		t.Errorf("trojan (nil outbound) detour = %q, want hop-out", got)
	}
}

// Empty detour tag is a no-op (no key added).
func TestApplySourceDetour_EmptyNoop(t *testing.T) {
	n := &configtypes.ParsedNode{Tag: "v", Scheme: "vless", Outbound: map[string]interface{}{"type": "vless"}}
	ApplySourceDetour([]*configtypes.ParsedNode{n}, "  ")
	if _, ok := n.Outbound["detour"]; ok {
		t.Error("empty detour tag must not add a detour key")
	}
}

// End-to-end through the full source loader: a server source with DetourTag
// yields a node whose generated outbound carries "detour".
func TestLoadNodesFromSource_ServerDetour(t *testing.T) {
	ps := configtypes.ProxySource{
		Connections: []string{"vless://e76a0cd2-da71-42bd-90f5-67ae619a8bdc@h.test:443?encryption=none&security=tls&sni=h.test#srv"},
		TagMask:     "srv",
		DetourTag:   "hop-out",
	}
	nodes, err := LoadNodesFromSource(ps, map[string]int{}, nil, 0, 1)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(nodes))
	}
	if got, _ := nodes[0].Outbound["detour"].(string); got != "hop-out" {
		t.Errorf("server node detour = %q, want hop-out", got)
	}
}
