package config

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// WireGuard-узел ОБЯЗАН получать detour: ядро (1.14.0-lx.28) принимает
// `detour` у endpoint/wireguard и честно дозванивается через указанный
// outbound — проверено запуском.
//
// Регрессия: раньше резолв снимал detour у всякого wireguard «правилом
// модели», и настройка из формы — личная у узла или общая у папки — молча
// исчезала между состоянием и конфигом. Узлы Proton поверх WARP при этом
// шли напрямую, хотя в форме стоял detour.
func TestWireguardNodeKeepsDetour(t *testing.T) {
	wg := &ParsedNode{
		Tag:             "wg",
		Scheme:          "wireguard",
		Outbound:        map[string]interface{}{"type": "wireguard"},
		CanonicalDetour: &configtypes.NodeLink{Tag: "relay"},
	}
	relay := &ParsedNode{
		Tag:      "relay",
		Scheme:   "masque",
		Outbound: map[string]interface{}{"type": "masque"},
	}

	nodesBySource := map[int][]*ParsedNode{0: {wg, relay}}
	proxies := []ProxySource{{Canonical: &configtypes.CanonicalSource{}}}
	targets := BuildNodeLinkTargets(proxies, nodesBySource, nil)

	if problem := resolveCanonicalDetour(wg, targets, map[*ParsedNode]bool{}); problem != "" {
		t.Fatalf("резолв отказал: %s", problem)
	}
	got, _ := wg.Outbound["detour"].(string)
	if got != "relay" {
		t.Errorf("detour = %q, ожидали \"relay\"", got)
	}
}
