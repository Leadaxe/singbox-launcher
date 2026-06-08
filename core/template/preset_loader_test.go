package template

import (
	"strings"
	"testing"
)

func TestLoadPresets_Empty(t *testing.T) {
	ps, ws := LoadPresets(nil, nil)
	if len(ps) != 0 || len(ws) != 0 {
		t.Errorf("expected empty result for nil input: ps=%v ws=%v", ps, ws)
	}

	ps, ws = LoadPresets([]byte("null"), nil)
	if len(ps) != 0 || len(ws) != 0 {
		t.Errorf("expected empty for 'null': ps=%v ws=%v", ps, ws)
	}
}

func TestLoadPresets_MalformedJSON(t *testing.T) {
	_, ws := LoadPresets([]byte(`{"not": "array"}`), nil)
	if len(ws) != 1 || ws[0].Action != "skip" {
		t.Errorf("expected skip warning: %+v", ws)
	}
}

func TestLoadPresets_ValidSimple(t *testing.T) {
	raw := []byte(`[
		{"id": "private-ips-direct", "label": "Private IPs direct",
		 "rules": [{"ip_is_private": true, "outbound": "direct-out"}]},
		{"id": "block-ads", "label": "Block ads",
		 "vars": [{"name": "out", "type": "outbound", "default": "reject"}],
		 "rule_set": [{"tag": "ads", "type": "remote", "url": "https://x/ads.srs"}],
		 "rules": [{"rule_set": "ads", "outbound": "@out"}]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 2 {
		t.Fatalf("expected 2 presets, got %d (warnings: %v)", len(ps), ws)
	}
	if len(ws) != 0 {
		t.Errorf("expected no warnings, got: %v", ws)
	}
	if ps[0].ID != "private-ips-direct" || ps[1].ID != "block-ads" {
		t.Errorf("preset order/ids: %v", ps)
	}
}

func TestLoadPresets_DuplicateID(t *testing.T) {
	raw := []byte(`[
		{"id": "dup", "label": "First",  "rules": [{"outbound": "direct-out"}]},
		{"id": "dup", "label": "Second", "rules": [{"outbound": "direct-out"}]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 1 {
		t.Fatalf("expected 1 preset (first), got %d", len(ps))
	}
	if ps[0].Label != "First" {
		t.Errorf("first should win: %v", ps[0])
	}
	found := false
	for _, w := range ws {
		if strings.Contains(w.Message, "duplicate preset id") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate-id warning: %v", ws)
	}
}

func TestLoadPresets_InvalidIDFormat(t *testing.T) {
	raw := []byte(`[
		{"id": "Has Spaces", "label": "X", "rules": [{}]},
		{"id": "good_one",   "label": "Y", "rules": [{}]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 1 || ps[0].ID != "good_one" {
		t.Errorf("expected only good_one: %v", ps)
	}
	hasFormat := false
	for _, w := range ws {
		if strings.Contains(w.Message, "does not match") {
			hasFormat = true
		}
	}
	if !hasFormat {
		t.Errorf("expected id-format warning: %v", ws)
	}
}

func TestLoadPresets_DuplicateRuleSetTag(t *testing.T) {
	raw := []byte(`[
		{"id": "bad", "label": "X",
		 "rule_set": [
			{"tag": "dup", "type": "inline", "rules": []},
			{"tag": "dup", "type": "remote", "url": "https://x"}
		 ],
		 "rules": [{"rule_set": "dup", "outbound": "direct-out"}]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 0 {
		t.Errorf("preset with duplicate rule_set tag should be skipped: %v", ps)
	}
	hasDup := false
	for _, w := range ws {
		if strings.Contains(w.Message, "duplicate rule_set tag") {
			hasDup = true
		}
	}
	if !hasDup {
		t.Errorf("expected dup-tag warning: %v", ws)
	}
}

func TestLoadPresets_DanglingRuleSetRef(t *testing.T) {
	raw := []byte(`[
		{"id": "dangling", "label": "X",
		 "rule_set": [{"tag": "a", "type": "inline"}],
		 "rules": [{"rule_set": "nonexistent", "outbound": "direct-out"}]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	// Preset не skip'ается (dangling ref в expansion вычищается),
	// но warning должен быть.
	if len(ps) != 1 {
		t.Errorf("expected 1 preset (dangling ref is strip-level): %v", ps)
	}
	hasDangling := false
	for _, w := range ws {
		if strings.Contains(w.Message, "rule_set ref") && strings.Contains(w.Message, "nonexistent") {
			hasDangling = true
		}
	}
	if !hasDangling {
		t.Errorf("expected dangling-ref warning: %v", ws)
	}
}

func TestLoadPresets_VarTypeUnknown(t *testing.T) {
	raw := []byte(`[
		{"id": "bad-type", "label": "X",
		 "vars": [{"name": "x", "type": "fancy", "default": "y"}],
		 "rules": [{"outbound": "direct-out"}]}
	]`)
	_, ws := LoadPresets(raw, nil)
	hasUnknown := false
	for _, w := range ws {
		if strings.Contains(w.Message, "unknown type") {
			hasUnknown = true
		}
	}
	if !hasUnknown {
		t.Errorf("expected unknown-type warning: %v", ws)
	}
}

func TestLoadPresets_SelectOnNonDnsServer(t *testing.T) {
	raw := []byte(`[
		{"id": "bad-select", "label": "X",
		 "vars": [{"name": "out", "type": "outbound", "default": "direct-out", "select": "local"}],
		 "rules": [{"outbound": "@out"}]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 1 {
		t.Fatal("preset should be kept (strip-level)")
	}
	if ps[0].Vars[0].Select != "" {
		t.Errorf("select should be stripped, got %q", ps[0].Vars[0].Select)
	}
	hasSelect := false
	for _, w := range ws {
		if strings.Contains(w.Message, "'select'") && strings.Contains(w.Message, "outbound") {
			hasSelect = true
		}
	}
	if !hasSelect {
		t.Errorf("expected select-on-outbound warning: %v", ws)
	}
}

func TestLoadPresets_SelectInvalidValue(t *testing.T) {
	raw := []byte(`[
		{"id": "x", "label": "X",
		 "vars": [{"name": "dns", "type": "dns_server", "default": "yandex_udp", "select": "fancy"}],
		 "dns_servers": [{"tag": "yandex_udp", "type": "udp", "server": "77.88.8.7"}],
		 "rules": [{"outbound": "direct-out"}]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 1 || ps[0].Vars[0].Select != "" {
		t.Errorf("invalid select should be stripped: %+v", ps[0].Vars[0])
	}
	hasInvalid := false
	for _, w := range ws {
		if strings.Contains(w.Message, "invalid select") {
			hasInvalid = true
		}
	}
	if !hasInvalid {
		t.Errorf("expected invalid-select warning: %v", ws)
	}
}

func TestLoadPresets_SelectAndOptionsCollision(t *testing.T) {
	raw := []byte(`[
		{"id": "x", "label": "X",
		 "vars": [{"name": "dns", "type": "dns_server", "default": "yandex_udp",
		           "select": "local", "options": ["yandex_udp"]}],
		 "dns_servers": [{"tag": "yandex_udp", "type": "udp", "server": "77.88.8.7"}],
		 "rules": [{"outbound": "direct-out"}]}
	]`)
	ps, _ := LoadPresets(raw, nil)
	if len(ps) != 1 || ps[0].Vars[0].Select != "" {
		t.Errorf("select should be stripped when options present: %+v", ps[0].Vars[0])
	}
}

func TestLoadPresets_EnumDefaultNotInOptions(t *testing.T) {
	raw := []byte(`[
		{"id": "x", "label": "X",
		 "vars": [{"name": "mode", "type": "enum", "default": "missing",
		           "options": [{"title": "A", "value": "a"}, {"title": "B", "value": "b"}]}],
		 "rules": [{"outbound": "direct-out"}]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 0 {
		t.Errorf("preset should be skipped: enum default not in options: %v", ps)
	}
	hasNotIn := false
	for _, w := range ws {
		if strings.Contains(w.Message, "not in options") {
			hasNotIn = true
		}
	}
	if !hasNotIn {
		t.Errorf("expected enum-default-not-in-options warning: %v", ws)
	}
}

func TestLoadPresets_IfRefUnknownVar(t *testing.T) {
	raw := []byte(`[
		{"id": "x", "label": "X",
		 "vars": [
			{"name": "out", "type": "outbound", "default": "direct-out"},
			{"name": "dns", "type": "dns_server", "default": "yandex_udp",
			 "if": ["nonexistent"]}
		 ],
		 "dns_servers": [{"tag": "yandex_udp", "type": "udp", "server": "77.88.8.7"}],
		 "rules": [{"outbound": "@out"}]}
	]`)
	_, ws := LoadPresets(raw, nil)
	hasIfRef := false
	for _, w := range ws {
		if strings.Contains(w.Message, "if reference") && strings.Contains(w.Message, "nonexistent") {
			hasIfRef = true
		}
	}
	if !hasIfRef {
		t.Errorf("expected unknown-if-ref warning: %v", ws)
	}
}

func TestLoadPresets_IfRefNonBool(t *testing.T) {
	raw := []byte(`[
		{"id": "x", "label": "X",
		 "vars": [
			{"name": "out", "type": "outbound", "default": "direct-out"},
			{"name": "dns", "type": "dns_server", "default": "yandex_udp",
			 "if": ["out"]}
		 ],
		 "dns_servers": [{"tag": "yandex_udp", "type": "udp", "server": "77.88.8.7"}],
		 "rules": [{"outbound": "@out"}]}
	]`)
	_, ws := LoadPresets(raw, nil)
	hasNonBool := false
	for _, w := range ws {
		if strings.Contains(w.Message, "not a bool var") {
			hasNonBool = true
		}
	}
	if !hasNonBool {
		t.Errorf("expected non-bool if-ref warning: %v", ws)
	}
}

func TestLoadPresets_VarShadowsGlobal(t *testing.T) {
	raw := []byte(`[
		{"id": "x", "label": "X",
		 "vars": [{"name": "cert_store", "type": "outbound", "default": "direct-out"}],
		 "rules": [{"outbound": "@cert_store"}]}
	]`)
	globals := map[string]bool{"cert_store": true}
	ps, ws := LoadPresets(raw, globals)
	if len(ps) != 1 {
		t.Fatal("preset should be kept (collision is just warning)")
	}
	hasShadow := false
	for _, w := range ws {
		if strings.Contains(w.Message, "shadows global") {
			hasShadow = true
		}
	}
	if !hasShadow {
		t.Errorf("expected shadow warning: %v", ws)
	}
}

// TestLoadPresets_RuDirectClean — реальный ru-direct из SPEC §«Real-world example»
// должен парситься без warnings.
func TestLoadPresets_RuDirectClean(t *testing.T) {
	raw := []byte(`[{
		"id": "ru-direct",
		"label": "Russian domains & IPs",
		"default_enabled": true,
		"vars": [
			{"name": "out", "type": "outbound", "default": "direct-out"},
			{"name": "use_dns_override", "type": "bool", "default": "true"},
			{"name": "dns_server", "type": "dns_server", "default": "yandex_udp",
			 "if": ["use_dns_override"], "select": "local"},
			{"name": "dns_ip", "type": "enum", "default": "77.88.8.8",
			 "if": ["use_dns_override"], "options": [
				{"title": "Base", "value": "77.88.8.8"},
				{"title": "Safe", "value": "77.88.8.88"}
			 ]},
			{"name": "geoip_enabled", "type": "bool", "default": "true"}
		],
		"rule_set": [
			{"tag": "ru-domains",  "type": "inline", "format": "domain_suffix",
			 "rules": [{"domain_suffix": ["ru","su"]}]},
			{"tag": "ru-services", "type": "inline", "format": "domain_suffix",
			 "rules": [{"domain_suffix": ["yandex.com"]}]},
			{"tag": "geoip-ru", "type": "remote", "format": "binary",
			 "url": "https://example.com/geoip-ru.srs", "if": ["geoip_enabled"]}
		],
		"dns_servers": [
			{"tag": "yandex_udp", "type": "udp", "server": "@dns_ip",
			 "server_port": 53, "detour": "@out", "if": ["use_dns_override"]},
			{"tag": "yandex_doh", "type": "https", "server": "77.88.8.88",
			 "server_port": 443, "path": "/dns-query", "detour": "@out",
			 "if": ["use_dns_override"]}
		],
		"rules": [{"rule_set": ["ru-domains","ru-services","geoip-ru"], "outbound": "@out"}],
		"dns_rule": {"rule_set": ["ru-domains","ru-services"], "server": "@dns_server",
		             "if": ["use_dns_override"]}
	}]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 1 {
		t.Fatalf("expected 1 preset, got %d (warnings: %v)", len(ps), ws)
	}
	if len(ws) != 0 {
		t.Errorf("ru-direct should parse cleanly, got warnings: %v", ws)
	}

	// Sanity: server в dns_rule это "@dns_server" — это var ref, не tag.
	// validateRuleSetRefs не должен ругаться на @-префикс (см. branch
	// `strings.HasPrefix(srv, "@")` в preset_loader.go).
}

// TestPresetWarning_String — sanity для текстового представления.
func TestPresetWarning_String(t *testing.T) {
	w := PresetWarning{PresetID: "ru-direct", Message: "test message", Action: "skip"}
	s := w.String()
	if !strings.Contains(s, "[skip]") || !strings.Contains(s, "ru-direct") || !strings.Contains(s, "test message") {
		t.Errorf("String format: %q", s)
	}

	w2 := PresetWarning{Message: "global warning", Action: "strip"}
	s2 := w2.String()
	if strings.Contains(s2, `preset "`) {
		t.Errorf("PresetID prefix should be absent when empty: %q", s2)
	}
}
