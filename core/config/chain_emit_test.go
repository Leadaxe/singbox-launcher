// File chain_emit_test.go — эмиссия Направления-цепочки (SPEC 110).
//
// Проверяются инварианты ядра (`protocol/chain/chain.go:85-100`) и форма
// выпущенного outbound'а: нарушение любого из них не даёт стартовать ВСЕМУ
// конфигу, а не одной цепочке.
package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

func chainDirection(hops ...string) configtypes.Direction {
	return configtypes.Direction{
		Tag:   "hop-chain",
		Type:  configtypes.DirectionTypeChain,
		Chain: &configtypes.DirectionChain{Hops: hops},
	}
}

// unwrapChainJSON вытаскивает объект из строки эмиттера (комментарий +
// табуляция + хвостовая запятая).
func unwrapChainJSON(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	idx := strings.Index(s, "{")
	if idx < 0 {
		t.Fatalf("в выпущенной строке нет объекта: %q", s)
	}
	body := strings.TrimSuffix(strings.TrimSpace(s[idx:]), ",")
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("невалидный JSON %q: %v", body, err)
	}
	return m
}

func TestChainEmit_MinimalShape(t *testing.T) {
	out, reason := GenerateChainJSON(chainDirection("hop-a", "hop-b"), nil)
	if reason != "" {
		t.Fatalf("цепочка не выпущена: %s", reason)
	}
	m := unwrapChainJSON(t, out)
	if m["type"] != "chain" {
		t.Errorf("type = %v, ожидали chain", m["type"])
	}
	if m["tag"] != "hop-chain" {
		t.Errorf("tag = %v", m["tag"])
	}
	// Ключ ядра — outbounds, наше поле называется Hops.
	hops, ok := m["outbounds"].([]interface{})
	if !ok || len(hops) != 2 || hops[0] != "hop-a" || hops[1] != "hop-b" {
		t.Fatalf("outbounds = %v, ожидали [hop-a hop-b] в порядке пакета", m["outbounds"])
	}
	// Незаданные поля не должны появляться: пустой idle_timeout ядро
	// читает как «0», а это другое поведение, чем умолчание 5m.
	for _, key := range []string{"idle_timeout", "strip_evasion", "strip", "rewrite"} {
		if _, present := m[key]; present {
			t.Errorf("незаданное поле %q попало в конфиг", key)
		}
	}
}

func TestChainEmit_OptionsRoundtrip(t *testing.T) {
	no := false
	d := chainDirection("hop-a", "hop-b")
	d.Chain.IdleTimeout = "10m"
	d.Chain.StripEvasion = &no
	d.Chain.Strip = map[string]bool{
		configtypes.ChainStripTLSUTLS:     true,
		configtypes.ChainStripTLSFragment: false,
	}
	d.Chain.Rewrite = map[string]interface{}{"vless": map[string]interface{}{"flow": ""}}

	out, reason := GenerateChainJSON(d, nil)
	if reason != "" {
		t.Fatalf("цепочка не выпущена: %s", reason)
	}
	m := unwrapChainJSON(t, out)
	if m["idle_timeout"] != "10m" {
		t.Errorf("idle_timeout = %v", m["idle_timeout"])
	}
	if m["strip_evasion"] != false {
		t.Errorf("strip_evasion = %v, ожидали false", m["strip_evasion"])
	}
	strip, ok := m["strip"].(map[string]interface{})
	if !ok || strip[configtypes.ChainStripTLSUTLS] != true || strip[configtypes.ChainStripTLSFragment] != false {
		t.Errorf("strip = %v", m["strip"])
	}
	if _, ok := m["rewrite"].(map[string]interface{}); !ok {
		t.Errorf("rewrite = %v", m["rewrite"])
	}
}

// Порядок ключей strip берётся из каталога, а не из map range: иначе
// golden-фикстуры конфига мигали бы от прогона к прогону.
func TestChainEmit_StripOrderStable(t *testing.T) {
	d := chainDirection("hop-a", "hop-b")
	d.Chain.Strip = map[string]bool{
		configtypes.ChainStripXHTTPPadding:     true,
		configtypes.ChainStripTLSFragment:      false,
		configtypes.ChainStripMultiplexPadding: true,
	}
	first, _ := GenerateChainJSON(d, nil)
	for i := 0; i < 20; i++ {
		got, _ := GenerateChainJSON(d, nil)
		if got != first {
			t.Fatalf("порядок ключей плавает:\n%s\n%s", first, got)
		}
	}
	if !strings.Contains(first, `"strip":{"tls.fragment":false,"multiplex.padding":true,"xhttp.padding":true}`) {
		t.Errorf("порядок не по каталогу: %s", first)
	}
}

func TestChainEmit_CoreInvariants(t *testing.T) {
	self := chainDirection("hop-a", "hop-chain")
	cases := []struct {
		name string
		d    configtypes.Direction
		want string
	}{
		{"одна позиция", chainDirection("hop-a"), "минимум две"},
		{"пусто", chainDirection(), "не задано ни одной позиции"},
		{"пустой тег", chainDirection("hop-a", "  "), "позиция 2 пуста"},
		{"самоссылка", self, "ссылается на саму цепочку"},
		{"дубль", chainDirection("hop-a", "hop-a"), "повторяет"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, reason := GenerateChainJSON(tc.d, nil)
			if out != "" {
				t.Fatalf("цепочка выпущена, хотя ядро её отвергнет: %s", out)
			}
			if !strings.Contains(reason, tc.want) {
				t.Errorf("причина %q не содержит %q", reason, tc.want)
			}
		})
	}
}

// Неизвестный ключ strip ядро считает ошибкой старта — конфиг не соберётся
// целиком, поэтому такую цепочку не выпускаем.
func TestChainEmit_UnknownStripKey(t *testing.T) {
	d := chainDirection("hop-a", "hop-b")
	d.Chain.Strip = map[string]bool{"tls.fragmnet": true}
	out, reason := GenerateChainJSON(d, nil)
	if out != "" {
		t.Fatalf("выпущена цепочка с неизвестным ключом strip: %s", out)
	}
	if !strings.Contains(reason, "неизвестный ключ") {
		t.Errorf("причина = %q", reason)
	}
}

// Позиция, не дошедшая до конфига, — ссылка в никуда: ядро не стартует.
// Маршрут без хопа это ДРУГОЙ маршрут, поэтому выпадает цепочка целиком, а
// не одна позиция.
func TestChainEmit_MissingHopDropsWholeChain(t *testing.T) {
	valid := map[string]bool{"hop-a": true}
	out, reason := GenerateChainJSON(chainDirection("hop-a", "hop-b"), valid)
	if out != "" {
		t.Fatalf("выпущена цепочка со ссылкой в никуда: %s", out)
	}
	if !strings.Contains(reason, "hop-b") {
		t.Errorf("причина не называет потерянную позицию: %q", reason)
	}
}

// Без поддержки ядром цепочка не эмитится вовсе (T1): неизвестный тип
// outbound'а отвергает ВЕСЬ конфиг, то есть лишает пользователя VPN.
func TestChainEmit_DegradesWithoutCoreSupport(t *testing.T) {
	prev := ChainSupportProbe
	defer func() { ChainSupportProbe = prev }()
	ChainSupportProbe = func() (bool, string) { return false, "ядро собрано без with_lx_chain" }

	info := map[string]*outboundInfo{
		"hop-a": {isValid: true},
		"hop-b": {isValid: true},
	}
	out, reason := emitChainDirection(chainDirection("hop-a", "hop-b"), info)
	if out != "" {
		t.Fatalf("цепочка выпущена на ядро без поддержки: %s", out)
	}
	if !strings.Contains(reason, "with_lx_chain") {
		t.Errorf("причина не называет тег сборки: %q", reason)
	}
}

// nil-проба (тесты, standalone) — считаем, что ядро умеет: деградировать на
// догадке нельзя.
func TestChainEmit_NilProbeAssumesSupported(t *testing.T) {
	prev := ChainSupportProbe
	defer func() { ChainSupportProbe = prev }()
	ChainSupportProbe = nil

	info := map[string]*outboundInfo{
		"hop-a": {isValid: true},
		"hop-b": {isValid: true},
	}
	out, reason := emitChainDirection(chainDirection("hop-a", "hop-b"), info)
	if out == "" {
		t.Fatalf("цепочка не выпущена при неизвестной поддержке: %s", reason)
	}
}

// Константа шаблона (direct-out) генератором не создаётся и в outboundsInfo
// не попадает — ровно как в addOutbounds селектора. Считать её потерянной
// позицией значило бы запретить цепочку через встроенные outbound'ы.
func TestChainEmit_TemplateConstantHopAllowed(t *testing.T) {
	info := map[string]*outboundInfo{"hop-a": {isValid: true}}
	out, reason := emitChainDirection(chainDirection("hop-a", "direct-out"), info)
	if out == "" {
		t.Fatalf("цепочка через константу шаблона не выпущена: %s", reason)
	}
}
