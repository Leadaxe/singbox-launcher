package v6

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
	if r.ID != "" {
		t.Errorf("preset-ref must not have id, got %q", r.ID)
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
	if r.Kind != RuleKindInline || r.ID == "" {
		t.Errorf("header mismatch: %+v", r)
	}
	if r.Ref != "" {
		t.Errorf("inline must not have ref, got %q", r.Ref)
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

// TestRule_PresetWithID_Error — kind=preset с лишним id → ошибка.
func TestRule_PresetWithID_Error(t *testing.T) {
	r := Rule{
		Kind: RuleKindPreset, Ref: "x", ID: "extra-id",
		Body: json.RawMessage(`{"vars":{}}`),
	}
	_, err := r.DecodeBody()
	if err == nil {
		t.Fatal("expected error: kind=preset with id")
	}
	if !strings.Contains(err.Error(), "must not have id") {
		t.Errorf("error text mismatch: %v", err)
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

// TestRule_InlineWithoutID_Error — kind=inline без id → ошибка.
func TestRule_InlineWithoutID_Error(t *testing.T) {
	r := Rule{
		Kind: RuleKindInline,
		Body: json.RawMessage(`{"name":"x","match":{},"outbound":"direct-out"}`),
	}
	_, err := r.DecodeBody()
	if err == nil {
		t.Fatal("expected error: kind=inline without id")
	}
	if !strings.Contains(err.Error(), "requires id") {
		t.Errorf("error text mismatch: %v", err)
	}
}

// TestRule_InlineWithRef_Error — kind=inline с лишним ref → ошибка.
func TestRule_InlineWithRef_Error(t *testing.T) {
	r := Rule{
		Kind: RuleKindInline, ID: "01J9X", Ref: "leaked",
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

// TestRule_OmitEmpty — пустые ref/id не пишутся в JSON.
func TestRule_OmitEmpty(t *testing.T) {
	r := Rule{
		Kind:    RuleKindInline,
		ID:      "01J9X",
		Enabled: true,
		Body:    json.RawMessage(`{}`),
	}
	out, _ := json.Marshal(r)
	if strings.Contains(string(out), `"ref":`) {
		t.Errorf("ref should be omitted for inline rule: %s", out)
	}

	r2 := Rule{Kind: RuleKindPreset, Ref: "x", Enabled: true, Body: json.RawMessage(`{}`)}
	out2, _ := json.Marshal(r2)
	if strings.Contains(string(out2), `"id":`) {
		t.Errorf("id should be omitted for preset rule: %s", out2)
	}
}

// TestDNSConfig_RoundTrip — DNS section с template_servers overrides.
// SPEC 057: extras удалены; unmarshal источника с `extra_servers`/`extra_rules`
// должен ТИХО проглотить эти поля (Go JSON decode игнорирует unknown fields
// в struct unmarshal — bonus к forward-compat: старые state.json грузятся).
func TestDNSConfig_RoundTrip(t *testing.T) {
	raw := []byte(`{
		"strategy": "prefer_ipv4",
		"independent_cache": true,
		"final": "google_doh",
		"default_domain_resolver": "google_doh",
		"template_servers": {
			"cloudflare_udp": {"enabled": true},
			"yandex_doh":     {"enabled": false}
		},
		"extra_servers": [
			{"tag": "my-pihole", "type": "udp", "server": "192.168.1.5", "server_port": 53}
		],
		"extra_rules": []
	}`)
	var d DNSConfig
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Strategy != "prefer_ipv4" || d.Final != "google_doh" {
		t.Errorf("scalar fields mismatch: %+v", d)
	}
	if len(d.TemplateServers) != 2 {
		t.Errorf("template_servers count: got %d", len(d.TemplateServers))
	}
	if !d.TemplateServers["cloudflare_udp"].Enabled {
		t.Error("cloudflare_udp override should be enabled=true")
	}
	if d.TemplateServers["yandex_doh"].Enabled {
		t.Error("yandex_doh override should be enabled=false")
	}
	// extras в raw — должны быть проглочены без ошибки. SPEC 057: state их
	// не хранит, любые extras от старых сборок тихо теряются на load.
}

// TestDNSConfig_OmitEmpty — пустой TemplateServers не пишется + удалённые
// поля extra_servers / extra_rules не появляются в выводе (SPEC 057).
func TestDNSConfig_OmitEmpty(t *testing.T) {
	d := DNSConfig{Strategy: "prefer_ipv4", Final: "google_doh"}
	out, _ := json.Marshal(d)
	outStr := string(out)
	for _, mustNotContain := range []string{
		`"template_servers"`, `"extra_servers"`, `"extra_rules"`,
	} {
		if strings.Contains(outStr, mustNotContain) {
			t.Errorf("expected omit: %q present in %s", mustNotContain, outStr)
		}
	}
}

// TestSchemaConstants — sanity для констант версии и schema name.
func TestSchemaConstants(t *testing.T) {
	if SchemaVersion != 6 {
		t.Errorf("SchemaVersion should be 6, got %d", SchemaVersion)
	}
	if SchemaName != "presets_v1" {
		t.Errorf("SchemaName mismatch: %q", SchemaName)
	}
}
