// File chain_validate_test.go — проверки состава цепочки (SPEC 110, фаза 4).
package config

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// realityHop — живой REALITY-узел vless: тело проходит санитайзер реестра,
// иначе проверка «звено требует tls.utls» судила бы не то.
func realityHop(tag string) *ParsedNode {
	return &ParsedNode{Tag: tag, Scheme: "vless", Outbound: map[string]interface{}{
		"type": "vless", "tag": tag,
		"server": "example-1.com", "server_port": 443,
		"uuid": "11111111-1111-1111-1111-111111111111",
		"tls": map[string]interface{}{
			"enabled":     true,
			"server_name": "example-1.com",
			"utls":        map[string]interface{}{"enabled": true, "fingerprint": "chrome"},
			"reality": map[string]interface{}{
				"enabled":    true,
				"public_key": "AwoRGB8mLTQ7QklQV15lbHN6gYiPlp2kq7K5wMfO1dw",
				"short_id":   "ab",
			},
		},
	}}
}

// TestChainUnstripRequired — ключ strip tls.utls у REALITY-звена не
// снимается по правилу реестра (связь requires с set у tls.reality.enabled +
// chain.json on_hop_required): цепочка собирается с патчем tls.utls=false,
// исходная не мутирует; позиция 0 и цепочка без снятия находок не дают.
func TestChainUnstripRequired(t *testing.T) {
	nodes := map[string]*ParsedNode{
		"r1":    realityHop("r1"),
		"r2":    realityHop("r2"),
		"plain": {Tag: "plain", Scheme: "vless"},
	}
	c := &configtypes.SourceChain{
		Hops:  []string{"r1", "plain", "r2"},
		Strip: map[string]bool{configtypes.ChainStripTLSUTLS: true},
	}
	got, notes := ChainUnstripRequired(c, nodes)
	if len(notes) != 1 || notes[0].Key != configtypes.ChainStripTLSUTLS || len(notes[0].Hops) != 1 || notes[0].Hops[0] != "r2" {
		t.Fatalf("находки = %+v, ожидали tls.utls у [r2] (позиция 0 не звено)", notes)
	}
	if notes[0].Code != "chain_strip_utls_on_reality" {
		t.Errorf("код = %q", notes[0].Code)
	}
	if v, ok := got.Strip[configtypes.ChainStripTLSUTLS]; !ok || v {
		t.Errorf("патч цепочки = %v, ожидали tls.utls=false", got.Strip)
	}
	if !c.Strip[configtypes.ChainStripTLSUTLS] {
		t.Error("исходная цепочка мутировала")
	}
	if _, n := ChainUnstripRequired(&configtypes.SourceChain{Hops: c.Hops}, nodes); len(n) != 0 {
		t.Errorf("находки без снятия utls: %+v", n)
	}
}

func TestChainNestedConflict(t *testing.T) {
	chains := map[string]bool{"inner": true}
	// Позиция 0 — единственная разрешённая для вложенной цепочки.
	ok := &configtypes.SourceChain{Hops: []string{"inner", "node-a"}}
	if got := ChainNestedConflict(ok, chains); len(got) != 0 {
		t.Errorf("вложенная цепочка первой позицией признана конфликтом: %v", got)
	}
	bad := &configtypes.SourceChain{Hops: []string{"node-a", "inner"}}
	if got := ChainNestedConflict(bad, chains); len(got) != 1 || got[0] != "inner" {
		t.Errorf("конфликт = %v, ожидали [inner]", got)
	}
}

func TestChainInternalTag(t *testing.T) {
	// Формат ядра — `<tag>#<index>` (protocol/chain/chain.go:135).
	for _, tag := range []string{"my-chain#0", "my-chain#12", "a#1"} {
		if !ChainInternalTag(tag) {
			t.Errorf("%q не опознан как внутренний тег звена", tag)
		}
	}
	for _, tag := range []string{"", "#0", "my-chain#", "my-chain#a", "my-chain", "vpn ①"} {
		if ChainInternalTag(tag) {
			t.Errorf("%q ошибочно опознан как внутренний тег", tag)
		}
	}
}
