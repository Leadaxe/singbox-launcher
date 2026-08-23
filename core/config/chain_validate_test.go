// File chain_validate_test.go — проверки состава цепочки (SPEC 110, фаза 4).
package config

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

func realityNode(tag string) *ParsedNode {
	return &ParsedNode{Tag: tag, Outbound: map[string]interface{}{
		"tls": map[string]interface{}{
			"reality": map[string]interface{}{"enabled": true},
		},
	}}
}

func TestNodeUsesReality(t *testing.T) {
	if !NodeUsesReality(realityNode("r")) {
		t.Error("reality-узел не опознан")
	}
	// reality: {enabled: false} — это НЕ reality: узел объявил секцию и
	// выключил её, utls ему не обязателен.
	off := &ParsedNode{Tag: "x", Outbound: map[string]interface{}{
		"tls": map[string]interface{}{
			"reality": map[string]interface{}{"enabled": false},
		},
	}}
	for _, n := range []*ParsedNode{nil, {Tag: "plain"}, off} {
		if NodeUsesReality(n) {
			t.Errorf("узел без reality опознан как reality: %+v", n)
		}
	}
}

func TestChainStripsUTLS(t *testing.T) {
	// Умолчание ядра: tls.utls НЕ снимается.
	if ChainStripsUTLS(&configtypes.SourceChain{}) {
		t.Error("utls снимается по умолчанию — расходится с каталогом ядра")
	}
	// Точечный патч перекрывает умолчание в обе стороны.
	on := &configtypes.SourceChain{Strip: map[string]bool{configtypes.ChainStripTLSUTLS: true}}
	if !ChainStripsUTLS(on) {
		t.Error("явное strip[tls.utls]=true не учтено")
	}
	yes := true
	offEvasion := &configtypes.SourceChain{
		StripEvasion: &yes,
		Strip:        map[string]bool{configtypes.ChainStripTLSUTLS: false},
	}
	if ChainStripsUTLS(offEvasion) {
		t.Error("явное strip[tls.utls]=false не перекрыло strip_evasion")
	}
}

func TestChainRealityConflict(t *testing.T) {
	nodes := map[string]*ParsedNode{
		"r1":    realityNode("r1"),
		"r2":    realityNode("r2"),
		"plain": {Tag: "plain"},
	}
	c := &configtypes.SourceChain{
		Hops:  []string{"r1", "plain", "r2"},
		Strip: map[string]bool{configtypes.ChainStripTLSUTLS: true},
	}
	got := ChainRealityConflict(c, nodes)
	// Позиция 0 идёт в сеть как есть — strip её не касается, r1 не конфликт.
	if len(got) != 1 || got[0] != "r2" {
		t.Fatalf("конфликт = %v, ожидали [r2] (позиция 0 не звено)", got)
	}
	// Без снятия utls конфликта нет вовсе.
	if got := ChainRealityConflict(&configtypes.SourceChain{Hops: c.Hops}, nodes); len(got) != 0 {
		t.Errorf("конфликт без снятия utls: %v", got)
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
