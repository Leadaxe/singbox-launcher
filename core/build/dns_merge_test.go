package build

import (
	"encoding/json"
	"strings"
	"testing"
)

// helper для извлечения key из json-выхода без лишней вёрстки в тестах.
func unmarshalDNS(t *testing.T, raw json.RawMessage) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, raw)
	}
	return m
}

// TestMergeDNSSection_Basic — два сервера + строка rules; результат содержит обе секции.
func TestMergeDNSSection_Basic(t *testing.T) {
	tmpl := json.RawMessage(`{"servers": [], "strategy": "ipv4_only"}`)
	cfg := DNSConfig{
		Servers: []json.RawMessage{
			json.RawMessage(`{"tag":"primary","address":"1.1.1.1"}`),
			json.RawMessage(`{"tag":"secondary","address":"8.8.8.8","description":"google","enabled":true}`),
		},
		RulesText: `[{"domain":"example.com","server":"primary"}]`,
	}

	got, err := MergeDNSSection(tmpl, cfg)
	if err != nil {
		t.Fatalf("MergeDNSSection: %v", err)
	}
	m := unmarshalDNS(t, got)

	servers, _ := m["servers"].([]interface{})
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	// "description" / "enabled" должны быть стрипнуты.
	for _, s := range servers {
		so := s.(map[string]interface{})
		if _, has := so["description"]; has {
			t.Errorf("description must be stripped: %+v", so)
		}
		if _, has := so["enabled"]; has {
			t.Errorf("enabled must be stripped: %+v", so)
		}
	}

	rules, _ := m["rules"].([]interface{})
	if len(rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(rules))
	}

	// Не задавали Final → fallback на первый enabled-server tag.
	if final, _ := m["final"].(string); final != "primary" {
		t.Errorf("final fallback: want primary, got %q", final)
	}

	// Strategy из шаблона сохраняется при пустом cfg.Strategy.
	if s, _ := m["strategy"].(string); s != "ipv4_only" {
		t.Errorf("template strategy must remain: %q", s)
	}
}

// TestMergeDNSSection_DisabledServerSkipped — сервер с enabled=false не идёт в out.
func TestMergeDNSSection_DisabledServerSkipped(t *testing.T) {
	cfg := DNSConfig{
		Servers: []json.RawMessage{
			json.RawMessage(`{"tag":"on","address":"1.1.1.1","enabled":true}`),
			json.RawMessage(`{"tag":"off","address":"2.2.2.2","enabled":false}`),
		},
	}
	got, err := MergeDNSSection(json.RawMessage(`{}`), cfg)
	if err != nil {
		t.Fatalf("MergeDNSSection: %v", err)
	}
	m := unmarshalDNS(t, got)
	servers, _ := m["servers"].([]interface{})
	if len(servers) != 1 {
		t.Fatalf("disabled server must be skipped, got: %v", servers)
	}
	if got := servers[0].(map[string]interface{})["tag"]; got != "on" {
		t.Errorf("only 'on' should remain, got tag=%v", got)
	}
}

// TestMergeDNSSection_FinalOverride — explicit Final выбивает fallback.
func TestMergeDNSSection_FinalOverride(t *testing.T) {
	cfg := DNSConfig{
		Servers: []json.RawMessage{
			json.RawMessage(`{"tag":"primary"}`),
			json.RawMessage(`{"tag":"secondary"}`),
		},
		Final: "secondary",
	}
	got, err := MergeDNSSection(json.RawMessage(`{}`), cfg)
	if err != nil {
		t.Fatalf("MergeDNSSection: %v", err)
	}
	m := unmarshalDNS(t, got)
	if final, _ := m["final"].(string); final != "secondary" {
		t.Errorf("explicit Final must override: got %q", final)
	}
}

// TestMergeDNSSection_NoFinalAvailable — нет ни Final, ни enabled-серверов
// → ключ final удаляется.
func TestMergeDNSSection_NoFinalAvailable(t *testing.T) {
	tmpl := json.RawMessage(`{"final":"old-final"}`)
	cfg := DNSConfig{Servers: []json.RawMessage{
		json.RawMessage(`{"tag":"off","enabled":false}`),
	}}
	got, err := MergeDNSSection(tmpl, cfg)
	if err != nil {
		t.Fatalf("MergeDNSSection: %v", err)
	}
	m := unmarshalDNS(t, got)
	if _, has := m["final"]; has {
		t.Errorf("final must be removed when no enabled server: %v", m["final"])
	}
}

// TestMergeDNSSection_StrategyOverride — explicit Strategy перезаписывает шаблон.
func TestMergeDNSSection_StrategyOverride(t *testing.T) {
	tmpl := json.RawMessage(`{"strategy":"ipv4_only"}`)
	cfg := DNSConfig{Strategy: "prefer_ipv6"}
	got, err := MergeDNSSection(tmpl, cfg)
	if err != nil {
		t.Fatalf("MergeDNSSection: %v", err)
	}
	m := unmarshalDNS(t, got)
	if s, _ := m["strategy"].(string); s != "prefer_ipv6" {
		t.Errorf("Strategy must override: got %q", s)
	}
}

// TestMergeDNSSection_IndependentCacheTristate — nil/true/false поведение.
func TestMergeDNSSection_IndependentCacheTristate(t *testing.T) {
	// nil → ключ не выставляется
	{
		got, _ := MergeDNSSection(json.RawMessage(`{}`), DNSConfig{})
		m := unmarshalDNS(t, got)
		if _, has := m["independent_cache"]; has {
			t.Errorf("nil → key must not appear: %v", m)
		}
	}
	// true
	{
		v := true
		got, _ := MergeDNSSection(json.RawMessage(`{}`), DNSConfig{IndependentCache: &v})
		m := unmarshalDNS(t, got)
		if b, _ := m["independent_cache"].(bool); !b {
			t.Errorf("true → independent_cache=true: %v", m)
		}
	}
	// false
	{
		v := false
		got, _ := MergeDNSSection(json.RawMessage(`{}`), DNSConfig{IndependentCache: &v})
		m := unmarshalDNS(t, got)
		raw, has := m["independent_cache"]
		if !has {
			t.Errorf("false → key must appear: %v", m)
		}
		if b, _ := raw.(bool); b {
			t.Errorf("false → independent_cache=false")
		}
	}
}

// TestParseDNSRulesText_FullObjectForm — канонический формат {"rules":[...]}.
func TestParseDNSRulesText_FullObjectForm(t *testing.T) {
	rules, err := ParseDNSRulesText(`{"rules":[{"domain":"a"},{"domain":"b"}]}`)
	if err != nil {
		t.Fatalf("ParseDNSRulesText: %v", err)
	}
	if len(rules) != 2 {
		t.Errorf("want 2 rules, got %d", len(rules))
	}
}

// TestParseDNSRulesText_PlainArray — голый массив тоже принимается.
func TestParseDNSRulesText_PlainArray(t *testing.T) {
	rules, err := ParseDNSRulesText(`[{"domain":"a"}]`)
	if err != nil {
		t.Fatalf("ParseDNSRulesText: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("want 1 rule, got %d", len(rules))
	}
}

// TestParseDNSRulesText_SingleObjectIsOneRule — одиночный объект = 1 правило.
func TestParseDNSRulesText_SingleObjectIsOneRule(t *testing.T) {
	rules, err := ParseDNSRulesText(`{"domain":"x"}`)
	if err != nil {
		t.Fatalf("ParseDNSRulesText: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("want 1 rule, got %d", len(rules))
	}
}

// TestParseDNSRulesText_LegacyMultiline — одна JSON-объект на строку + комментарии.
func TestParseDNSRulesText_LegacyMultiline(t *testing.T) {
	in := `# comment line
{"domain":"a"}

# another comment
{"domain":"b"}`
	rules, err := ParseDNSRulesText(in)
	if err != nil {
		t.Fatalf("ParseDNSRulesText: %v", err)
	}
	if len(rules) != 2 {
		t.Errorf("want 2 rules from legacy multiline, got %d", len(rules))
	}
}

// TestParseDNSRulesText_Empty — пустая строка → nil, nil.
func TestParseDNSRulesText_Empty(t *testing.T) {
	rules, err := ParseDNSRulesText("")
	if err != nil || rules != nil {
		t.Errorf("empty input: want (nil, nil), got (%v, %v)", rules, err)
	}
}

// TestParseDNSRulesText_RulesNotArrayError — {"rules": "not-array"} — error.
func TestParseDNSRulesText_RulesNotArrayError(t *testing.T) {
	_, err := ParseDNSRulesText(`{"rules":"not-array"}`)
	if err == nil || !strings.Contains(err.Error(), "rules") {
		t.Errorf("expected 'rules' error, got: %v", err)
	}
}

// TestParseDNSRulesText_NonObjectInArrayError — массив с не-объектом.
func TestParseDNSRulesText_NonObjectInArrayError(t *testing.T) {
	_, err := ParseDNSRulesText(`[42]`)
	if err == nil || !strings.Contains(err.Error(), "expected JSON object") {
		t.Errorf("expected 'expected JSON object' error, got: %v", err)
	}
}

// TestFirstEnabledDNSServerTag_FindsFirst — игнорирует disabled / без тега.
func TestFirstEnabledDNSServerTag_FindsFirst(t *testing.T) {
	servers := []json.RawMessage{
		json.RawMessage(`{"address":"x"}`), // no tag
		json.RawMessage(`{"tag":"off","enabled":false}`),
		json.RawMessage(`{"tag":"second"}`),
		json.RawMessage(`{"tag":"third"}`),
	}
	if got := firstEnabledDNSServerTag(servers); got != "second" {
		t.Errorf("want 'second', got %q", got)
	}
}

// TestFirstEnabledDNSServerTag_NoneEnabled — все disabled или без тега.
func TestFirstEnabledDNSServerTag_NoneEnabled(t *testing.T) {
	servers := []json.RawMessage{
		json.RawMessage(`{"tag":"off","enabled":false}`),
	}
	if got := firstEnabledDNSServerTag(servers); got != "" {
		t.Errorf("want empty, got %q", got)
	}
}
