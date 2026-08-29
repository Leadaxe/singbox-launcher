package state

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRule_RoundTrip_Preset — preset-ref в state.rules[].
func TestRule_RoundTrip_Preset(t *testing.T) {
	raw := []byte(`{
		"kind": "preset",
		"ref": "ru-direct",
		"enabled": true,
		"body": {"vars": {"dns_ip": "77.88.8.7"}}
	}`)
	var r Rule
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.Kind != RuleKindPreset || r.Ref != "ru-direct" || !r.Enabled {
		t.Errorf("header mismatch: %+v", r)
	}
	// SPEC 063: Rule.ID удалено; identity вычисляется через StableRuleID.
	if StableRuleID(r) != "ru-direct" {
		t.Errorf("preset identity should equal Ref, got %q", StableRuleID(r))
	}

	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("DecodeBody: %v", err)
	}
	pb, ok := body.(*PresetBody)
	if !ok {
		t.Fatalf("expected *PresetBody, got %T", body)
	}
	if pb.Vars["dns_ip"] != "77.88.8.7" {
		t.Errorf("vars mismatch: %+v", pb.Vars)
	}
}

// TestRule_RoundTrip_PresetEmptyVars — preset-ref с пустым body.vars (всё дефолтное).
func TestRule_RoundTrip_PresetEmptyVars(t *testing.T) {
	raw := []byte(`{
		"kind": "preset", "ref": "block-ads", "enabled": false, "body": {"vars": {}}
	}`)
	var r Rule
	_ = json.Unmarshal(raw, &r)
	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("DecodeBody: %v", err)
	}
	pb := body.(*PresetBody)
	if pb.Vars == nil || len(pb.Vars) != 0 {
		t.Errorf("expected empty non-nil Vars map, got %v", pb.Vars)
	}
}

// TestRule_PresetMissingBody — body может отсутствовать совсем, DecodeBody возвращает {}.
func TestRule_PresetMissingBody(t *testing.T) {
	r := Rule{Kind: RuleKindPreset, Ref: "x", Enabled: true}
	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("DecodeBody on missing body: %v", err)
	}
	pb := body.(*PresetBody)
	if pb.Vars == nil {
		t.Error("Vars should be initialized to empty map, not nil")
	}
}

// TestRule_RoundTrip_Inline — user inline rule.
//
// SPEC 063: JSON "id" field в payload — legacy. Go unmarshal silently
// игнорирует (Rule struct больше не имеет ID field); identity вычисляется
// через StableRuleID из body.name.
func TestRule_RoundTrip_Inline(t *testing.T) {
	raw := []byte(`{
		"kind": "inline",
		"id": "01J9X0000000000000000000A",
		"enabled": true,
		"body": {
			"name": "Firefox через VPN",
			"match": {"domain_suffix": ["example.com"], "package_name": ["org.mozilla.firefox"]},
			"outbound": "proxy-out"
		}
	}`)
	var r Rule
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.Kind != RuleKindInline {
		t.Errorf("kind mismatch: %+v", r)
	}
	if r.Ref != "" {
		t.Errorf("inline must not have ref, got %q", r.Ref)
	}
	// Identity вычислимая, не зависит от legacy "id" в JSON.
	if got := StableRuleID(r); got != "Firefox--VPN" {
		t.Errorf("StableRuleID: %q (want sanitize of body.name)", got)
	}

	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("DecodeBody: %v", err)
	}
	ib := body.(*InlineBody)
	if ib.Name != "Firefox через VPN" || ib.Outbound != "proxy-out" {
		t.Errorf("body mismatch: %+v", ib)
	}
	if domains, ok := ib.Match["domain_suffix"].([]interface{}); !ok || len(domains) != 1 {
		t.Errorf("match.domain_suffix mismatch: %+v", ib.Match)
	}
}

// TestRule_RoundTrip_Srs — user srs rule с reject outbound.
func TestRule_RoundTrip_Srs(t *testing.T) {
	raw := []byte(`{
		"kind": "srs",
		"id": "01J9X0000000000000000000B",
		"enabled": true,
		"body": {
			"name": "Custom block list",
			"srs_url": "https://example.com/blocklist.srs",
			"outbound": "reject"
		}
	}`)
	var r Rule
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := StableRuleID(r); got != "Custom-block-list" {
		t.Errorf("StableRuleID: %q (want sanitize of body.name)", got)
	}
	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("DecodeBody: %v", err)
	}
	sb := body.(*SrsBody)
	if sb.SrsURL != "https://example.com/blocklist.srs" {
		t.Errorf("srs_url mismatch: %q", sb.SrsURL)
	}
	if sb.Outbound != "reject" {
		t.Errorf("outbound mismatch: %q (expected reject sentinel)", sb.Outbound)
	}
}

// TestRule_PresetWithoutRef_Error — kind=preset без ref → ошибка.
func TestRule_PresetWithoutRef_Error(t *testing.T) {
	r := Rule{Kind: RuleKindPreset, Body: json.RawMessage(`{"vars":{}}`)}
	_, err := r.DecodeBody()
	if err == nil {
		t.Fatal("expected error: kind=preset without ref")
	}
	if !strings.Contains(err.Error(), "requires ref") {
		t.Errorf("error text mismatch: %v", err)
	}
}

// TestRule_InlineWithoutName_Error — SPEC 063: kind=inline без body.name → ошибка.
func TestRule_InlineWithoutName_Error(t *testing.T) {
	r := Rule{
		Kind: RuleKindInline,
		Body: json.RawMessage(`{"match":{"port":[443]},"outbound":"direct-out"}`),
	}
	_, err := r.DecodeBody()
	if err == nil {
		t.Fatal("expected error: kind=inline without body.name")
	}
	if !strings.Contains(err.Error(), "requires body.name") {
		t.Errorf("error text mismatch: %v", err)
	}
}

// TestRule_SrsWithoutName_Error — SPEC 063: kind=srs без body.name → ошибка.
func TestRule_SrsWithoutName_Error(t *testing.T) {
	r := Rule{
		Kind: RuleKindSrs,
		Body: json.RawMessage(`{"srs_url":"https://x","outbound":"reject"}`),
	}
	_, err := r.DecodeBody()
	if err == nil {
		t.Fatal("expected error: kind=srs without body.name")
	}
	if !strings.Contains(err.Error(), "requires body.name") {
		t.Errorf("error text mismatch: %v", err)
	}
}

// TestRule_SrsWithoutURL_Error — SPEC 063: kind=srs без body.srs_url → ошибка.
func TestRule_SrsWithoutURL_Error(t *testing.T) {
	r := Rule{
		Kind: RuleKindSrs,
		Body: json.RawMessage(`{"name":"x","outbound":"reject"}`),
	}
	_, err := r.DecodeBody()
	if err == nil {
		t.Fatal("expected error: kind=srs without body.srs_url")
	}
	if !strings.Contains(err.Error(), "requires body.srs_url") {
		t.Errorf("error text mismatch: %v", err)
	}
}

// TestRule_InlineWithRef_Error — kind=inline с лишним ref → ошибка.
func TestRule_InlineWithRef_Error(t *testing.T) {
	r := Rule{
		Kind: RuleKindInline, Ref: "leaked",
		Body: json.RawMessage(`{"name":"x","match":{},"outbound":"direct-out"}`),
	}
	_, err := r.DecodeBody()
	if err == nil {
		t.Fatal("expected error: kind=inline with ref")
	}
	if !strings.Contains(err.Error(), "must not have ref") {
		t.Errorf("error text mismatch: %v", err)
	}
}

// TestRule_UnknownKind_Error — unknown kind → ошибка.
func TestRule_UnknownKind_Error(t *testing.T) {
	r := Rule{Kind: "geosite", Body: json.RawMessage(`{}`)}
	_, err := r.DecodeBody()
	if err == nil {
		t.Fatal("expected error on unknown kind")
	}
	if !strings.Contains(err.Error(), "unknown rule kind") {
		t.Errorf("error text mismatch: %v", err)
	}
}

// TestRule_OmitEmpty — пустой ref не пишется для kind=inline; id больше нет
// в struct (SPEC 063) — Marshal никогда не эмитит "id" field вообще.
func TestRule_OmitEmpty(t *testing.T) {
	r := Rule{
		Kind:    RuleKindInline,
		Enabled: true,
		Body:    json.RawMessage(`{}`),
	}
	out, _ := json.Marshal(r)
	if strings.Contains(string(out), `"ref":`) {
		t.Errorf("ref should be omitted for inline rule: %s", out)
	}
	if strings.Contains(string(out), `"id":`) {
		t.Errorf("id should NOT be emitted (SPEC 063 drop): %s", out)
	}

	r2 := Rule{Kind: RuleKindPreset, Ref: "x", Enabled: true, Body: json.RawMessage(`{}`)}
	out2, _ := json.Marshal(r2)
	if strings.Contains(string(out2), `"id":`) {
		t.Errorf("id should NOT be emitted (SPEC 063 drop): %s", out2)
	}
}

// TestRule_LegacyIDIgnoredOnLoad — SPEC 063: state.json с legacy "id" поле
// загружается без error; identity вычисляется из body.name.
func TestRule_LegacyIDIgnoredOnLoad(t *testing.T) {
	raw := []byte(`{
		"kind": "srs",
		"id": "rule-YT",
		"enabled": true,
		"body": {"name": "YT", "srs_url": "https://x/y.srs", "outbound": "direct-out"}
	}`)
	var r Rule
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := StableRuleID(r); got != "YT" {
		t.Errorf("StableRuleID after load: got %q, want \"YT\" (from body.name, not legacy id)", got)
	}
}

// TestRule_RoundTripDropsID — SPEC 063: save → load → save должен убрать legacy "id".
func TestRule_RoundTripDropsID(t *testing.T) {
	raw := []byte(`{
		"kind": "inline",
		"id": "rule-Old",
		"enabled": true,
		"body": {"name": "Old", "match": {"port": [443]}, "outbound": "direct-out"}
	}`)
	var r Rule
	_ = json.Unmarshal(raw, &r)
	out, _ := json.Marshal(r)
	if strings.Contains(string(out), `"id":`) {
		t.Errorf("re-emit must drop legacy id: %s", out)
	}
}

// TestDNSOptions_RoundTrip — SPEC 056-R-N: flat servers[] через kind discriminator.
func TestDNSOptions_RoundTrip(t *testing.T) {
	// SPEC: independent_cache в payload — legacy/forward-compat поле,
	// JSON unmarshal должен silently игнорировать (поле снято из DNSOptions).
	raw := []byte(`{
		"strategy": "prefer_ipv4",
		"independent_cache": true,
		"final": "google_doh",
		"default_domain_resolver": "google_doh",
		"servers": [
			{"kind":"template", "tag":"cloudflare_udp", "enabled":true},
			{"kind":"template", "tag":"yandex_doh", "enabled":false},
			{"kind":"preset",   "ref":"russian:yandex_udp", "enabled":true},
			{"kind":"user",     "tag":"my-pihole", "type":"udp", "server":"192.168.1.5", "server_port":53, "enabled":true}
		],
		"rules": [
			{"kind":"preset", "ref":"russian", "enabled":true},
			{"kind":"user",   "rule_set":"ru-domains", "server":"yandex_doh", "enabled":true}
		]
	}`)
	var d DNSOptions
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Strategy != "prefer_ipv4" || d.Final != "google_doh" {
		t.Errorf("scalar fields mismatch: %+v", d)
	}
	if len(d.Servers) != 4 {
		t.Fatalf("servers count: %d", len(d.Servers))
	}

	// Round-trip: marshal → unmarshal → identical structure.
	roundtrip, _ := json.Marshal(d)
	var d2 DNSOptions
	if err := json.Unmarshal(roundtrip, &d2); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if len(d2.Servers) != 4 || len(d2.Rules) != 2 {
		t.Errorf("round-trip lost entries: %+v", d2)
	}

	// Spot-check каждого kind'а.
	if d.Servers[0].Kind != DNSServerKindTemplate || d.Servers[0].Tag != "cloudflare_udp" || !d.Servers[0].Enabled {
		t.Errorf("template entry 0: %+v", d.Servers[0])
	}
	if d.Servers[2].Kind != DNSServerKindPreset || d.Servers[2].Ref != "russian:yandex_udp" {
		t.Errorf("preset entry: %+v", d.Servers[2])
	}
	if d.Servers[3].Kind != DNSServerKindUser || d.Servers[3].Tag != "my-pihole" {
		t.Errorf("user entry: %+v", d.Servers[3])
	}
	if d.Servers[3].Body["server"] != "192.168.1.5" {
		t.Errorf("user body lost: %+v", d.Servers[3].Body)
	}
	if d.Rules[0].Kind != DNSRuleKindPreset || d.Rules[0].Ref != "russian" {
		t.Errorf("preset rule: %+v", d.Rules[0])
	}
	if d.Rules[1].Body["rule_set"] != "ru-domains" {
		t.Errorf("user rule body lost: %+v", d.Rules[1].Body)
	}
}

// TestDNSOptions_OmitEmpty — пустые Servers/Rules не пишутся.
func TestDNSOptions_OmitEmpty(t *testing.T) {
	d := DNSOptions{Strategy: "prefer_ipv4", Final: "google_doh"}
	out, _ := json.Marshal(d)
	outStr := string(out)
	for _, mustNotContain := range []string{
		`"servers"`, `"rules"`,
	} {
		if strings.Contains(outStr, mustNotContain) {
			t.Errorf("expected omit: %q present in %s", mustNotContain, outStr)
		}
	}
}

// TestSchemaConstants — sanity для констант версии и schema name.
func TestSchemaConstants(t *testing.T) {
	if SchemaVersionV6 != 6 {
		t.Errorf("SchemaVersionV6 should be 6, got %d", SchemaVersionV6)
	}
	if SchemaNameV6 != "presets_v1" {
		t.Errorf("SchemaNameV6 mismatch: %q", SchemaNameV6)
	}
	if SchemaVersionV7 != 7 || SchemaVersion != SchemaVersionV7 {
		t.Errorf("SchemaVersion should be v7 (SPEC 118), got %d", SchemaVersion)
	}
	if SchemaNameV7 != "sources_v7" {
		t.Errorf("SchemaNameV7 mismatch: %q", SchemaNameV7)
	}
}
