// File chain_emit_test.go — эмиссия источника-цепочки (SPEC 110).
//
// Проверяются инварианты ядра (`protocol/chain/chain.go:85-100`) и форма
// собранного объекта: нарушение любого из них не даёт стартовать ВСЕМУ
// конфигу, а не одной цепочке.
package config

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

func chainOf(hops ...string) *configtypes.SourceChain {
	return &configtypes.SourceChain{Hops: hops}
}

// chainSource — источник-цепочка в ParserConfig.
func chainSource(tag string, c *configtypes.SourceChain) ProxySource {
	return ProxySource{TagMask: tag, Chain: c}
}

// resolveOne прогоняет один источник-цепочку через разрешение и возвращает
// получившийся узел (или nil) с причиной деградации.
func resolveOne(t *testing.T, src ProxySource, poolTags []string, directionTags ...string) (*ParsedNode, string) {
	t.Helper()
	pc := &ParserConfig{}
	pc.ParserConfig.Proxies = []ProxySource{src}
	pool := make([]*ParsedNode, 0, len(poolTags))
	for i, tag := range poolTags {
		pool = append(pool, &ParsedNode{Tag: tag, Scheme: "socks", Server: "10.0.0.1", Port: 1080 + i})
	}
	dirs := make(map[string]bool, len(directionTags))
	for _, d := range directionTags {
		dirs[d] = true
	}
	bySource := map[int][]*ParsedNode{}
	out, broken := ResolveChainSources(pc, pool, bySource, dirs)
	if len(broken) > 0 {
		return nil, broken[0].Reason
	}
	for _, n := range out {
		if n.Scheme == configtypes.ChainOutboundType {
			return n, ""
		}
	}
	return nil, "цепочка не появилась в пуле и не сообщила причину"
}

func TestChainNode_MinimalShape(t *testing.T) {
	node, reason := resolveOne(t, chainSource("double-hop", chainOf("hop-a", "hop-b")),
		[]string{"hop-a", "hop-b"})
	if node == nil {
		t.Fatalf("цепочка не стала узлом: %s", reason)
	}
	// Цепочка эмитится как ручная нода — объект уходит в конфиг как есть.
	if !node.EmitRaw {
		t.Error("узел цепочки не помечен EmitRaw — объект пройдёт через per-scheme switch")
	}
	ob := node.Outbound
	if ob["type"] != "chain" {
		t.Errorf("type = %v, ожидали chain", ob["type"])
	}
	if ob["tag"] != "double-hop" {
		t.Errorf("tag = %v", ob["tag"])
	}
	// Ключ ядра — outbounds, наше поле называется Hops.
	hops, ok := ob["outbounds"].([]interface{})
	if !ok || len(hops) != 2 || hops[0] != "hop-a" || hops[1] != "hop-b" {
		t.Fatalf("outbounds = %v, ожидали [hop-a hop-b] в порядке пакета", ob["outbounds"])
	}
	// Незаданные поля не должны появляться: пустой idle_timeout ядро
	// читает как «0», а это другое поведение, чем умолчание 5m.
	for _, key := range []string{"idle_timeout", "strip_evasion", "strip", "rewrite"} {
		if _, present := ob[key]; present {
			t.Errorf("незаданное поле %q попало в конфиг", key)
		}
	}
}

func TestChainNode_OptionsRoundtrip(t *testing.T) {
	no := false
	c := chainOf("hop-a", "hop-b")
	c.IdleTimeout = "10m"
	c.StripEvasion = &no
	c.Strip = map[string]bool{
		configtypes.ChainStripTLSUTLS:     true,
		configtypes.ChainStripTLSFragment: false,
	}
	c.Rewrite = map[string]interface{}{"vless": map[string]interface{}{"flow": ""}}

	node, reason := resolveOne(t, chainSource("tuned", c), []string{"hop-a", "hop-b"})
	if node == nil {
		t.Fatalf("цепочка не стала узлом: %s", reason)
	}
	ob := node.Outbound
	if ob["idle_timeout"] != "10m" {
		t.Errorf("idle_timeout = %v", ob["idle_timeout"])
	}
	if ob["strip_evasion"] != false {
		t.Errorf("strip_evasion = %v, ожидали false", ob["strip_evasion"])
	}
	strip, ok := ob["strip"].(map[string]interface{})
	if !ok || strip[configtypes.ChainStripTLSUTLS] != true || strip[configtypes.ChainStripTLSFragment] != false {
		t.Errorf("strip = %v", ob["strip"])
	}
	if _, ok := ob["rewrite"].(map[string]interface{}); !ok {
		t.Errorf("rewrite = %v", ob["rewrite"])
	}
}

func TestChainNode_CoreInvariants(t *testing.T) {
	cases := []struct {
		name string
		tag  string
		c    *configtypes.SourceChain
		want string
	}{
		{"одна позиция", "c", chainOf("hop-a"), "at least two"},
		{"пусто", "c", chainOf(), "no positions set"},
		{"пустой тег", "c", chainOf("hop-a", "  "), "position 2 is empty"},
		{"самоссылка", "c", chainOf("hop-a", "c"), "references the chain itself"},
		{"дубль", "c", chainOf("hop-a", "hop-a"), "repeats"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, reason := resolveOne(t, chainSource(tc.tag, tc.c), []string{"hop-a", "c"})
			if node != nil {
				t.Fatalf("цепочка стала узлом, хотя ядро её отвергнет")
			}
			if !strings.Contains(reason, tc.want) {
				t.Errorf("причина %q не содержит %q", reason, tc.want)
			}
		})
	}
}

// Неизвестный ключ strip ядро считает ошибкой старта — конфиг не соберётся
// целиком, поэтому такую цепочку не выпускаем.
func TestChainNode_UnknownStripKey(t *testing.T) {
	c := chainOf("hop-a", "hop-b")
	c.Strip = map[string]bool{"tls.fragmnet": true}
	node, reason := resolveOne(t, chainSource("c", c), []string{"hop-a", "hop-b"})
	if node != nil {
		t.Fatal("выпущена цепочка с неизвестным ключом strip")
	}
	if !strings.Contains(reason, "unknown key") {
		t.Errorf("причина = %q", reason)
	}
}

// Позиция, которой нет в пуле, — ссылка в никуда: ядро не стартует. Маршрут
// без хопа это ДРУГОЙ маршрут, поэтому выпадает цепочка целиком.
func TestChainNode_MissingHopDropsWholeChain(t *testing.T) {
	node, reason := resolveOne(t, chainSource("c", chainOf("hop-a", "hop-b")), []string{"hop-a"})
	if node != nil {
		t.Fatal("выпущена цепочка со ссылкой в никуда")
	}
	if !strings.Contains(reason, "hop-b") {
		t.Errorf("причина не называет потерянную позицию: %q", reason)
	}
}

// Направление — законная позиция: цепочка через группу это главное, чего
// не умеет detour.
func TestChainNode_DirectionHopAllowed(t *testing.T) {
	node, reason := resolveOne(t, chainSource("c", chainOf("proxy-out", "hop-b")),
		[]string{"hop-b"}, "proxy-out")
	if node == nil {
		t.Fatalf("цепочка через Направление не собралась: %s", reason)
	}
}

// Без поддержки ядром цепочка не становится узлом вовсе (T1): неизвестный
// тип outbound'а отвергает ВЕСЬ конфиг, то есть лишает пользователя VPN.
func TestChainNode_DegradesWithoutCoreSupport(t *testing.T) {
	prev := ChainSupportProbe
	defer func() { ChainSupportProbe = prev }()
	ChainSupportProbe = func() (bool, string) { return false, "ядро собрано без with_lx_chain" }

	node, reason := resolveOne(t, chainSource("c", chainOf("hop-a", "hop-b")),
		[]string{"hop-a", "hop-b"})
	if node != nil {
		t.Fatal("цепочка собрана на ядре без поддержки")
	}
	if !strings.Contains(reason, "with_lx_chain") {
		t.Errorf("причина не называет тег сборки: %q", reason)
	}
}

// nil-проба (тесты, standalone) — считаем, что ядро умеет: деградировать на
// догадке нельзя.
func TestChainNode_NilProbeAssumesSupported(t *testing.T) {
	prev := ChainSupportProbe
	defer func() { ChainSupportProbe = prev }()
	ChainSupportProbe = nil

	node, reason := resolveOne(t, chainSource("c", chainOf("hop-a", "hop-b")),
		[]string{"hop-a", "hop-b"})
	if node == nil {
		t.Fatalf("цепочка не собралась при неизвестной поддержке: %s", reason)
	}
}

// Цепочка вправе сослаться на цепочку, объявленную ВЫШЕ по списку: так
// вложенность выразима, а циклы невозможны по построению.
func TestChainNode_NestedChainResolvesInOrder(t *testing.T) {
	pc := &ParserConfig{}
	pc.ParserConfig.Proxies = []ProxySource{
		chainSource("inner", chainOf("hop-a", "hop-b")),
		chainSource("outer", chainOf("inner", "hop-c")),
	}
	pool := []*ParsedNode{
		{Tag: "hop-a", Scheme: "socks"}, {Tag: "hop-b", Scheme: "socks"}, {Tag: "hop-c", Scheme: "socks"},
	}
	out, broken := ResolveChainSources(pc, pool, map[int][]*ParsedNode{}, nil)
	if len(broken) > 0 {
		t.Fatalf("деградация: %+v", broken)
	}
	got := 0
	for _, n := range out {
		if n.Scheme == configtypes.ChainOutboundType {
			got++
		}
	}
	if got != 2 {
		t.Fatalf("собрано цепочек: %d, ожидали 2", got)
	}
}

// Обратный порядок — ссылка вперёд не разрешается: это и есть механизм,
// которым исключены циклы между цепочками.
func TestChainNode_ForwardReferenceRejected(t *testing.T) {
	pc := &ParserConfig{}
	pc.ParserConfig.Proxies = []ProxySource{
		chainSource("outer", chainOf("inner", "hop-c")),
		chainSource("inner", chainOf("hop-a", "hop-b")),
	}
	pool := []*ParsedNode{
		{Tag: "hop-a", Scheme: "socks"}, {Tag: "hop-b", Scheme: "socks"}, {Tag: "hop-c", Scheme: "socks"},
	}
	_, broken := ResolveChainSources(pc, pool, map[int][]*ParsedNode{}, nil)
	if len(broken) != 1 || !strings.Contains(broken[0].Reason, "inner") {
		t.Fatalf("ссылка вперёд принята: %+v", broken)
	}
}
