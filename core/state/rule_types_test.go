package state

import (
	"encoding/json"
	"strings"
	"testing"
)

// Сценарии те же, что до state v8, но на целевой форме: имя/номер/наборы/
// переменные — поля записи, `body` — правило sing-box как есть.

// TestRule_RoundTrip_Preset — preset-ref в state.rules[]: vars на уровне записи.
func TestRule_RoundTrip_Preset(t *testing.T) {
	raw := []byte(`{
		"kind": "preset",
		"ref": "ru-direct",
		"enabled": true,
		"vars": {"dns_ip": "77.88.8.7"}
	}`)
	var r Rule
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.Kind != RuleKindPreset || r.Ref != "ru-direct" || !r.Enabled {
		t.Errorf("header mismatch: %+v", r)
	}
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

// TestRule_RoundTrip_PresetEmptyVars — preset-ref без vars (всё дефолтное).
func TestRule_RoundTrip_PresetEmptyVars(t *testing.T) {
	raw := []byte(`{"kind": "preset", "ref": "block-ads", "enabled": false}`)
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

// TestRule_PresetMissingBody — тела у preset нет вовсе, DecodeBody даёт вид с {}.
func TestRule_PresetMissingBody(t *testing.T) {
	r := NewPresetRule("x", nil)
	r.Enabled = true
	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("DecodeBody on missing body: %v", err)
	}
	pb := body.(*PresetBody)
	if pb.Vars == nil {
		t.Error("Vars should be initialized to empty map, not nil")
	}
	if len(r.Body) != 0 {
		t.Errorf("preset rule must carry no body, got %s", r.Body)
	}
}

// TestRule_PresetVarsNotInBody — NewPresetRule кладёт переменные в поле записи,
// а не в тело (единственный писатель).
func TestRule_PresetVarsNotInBody(t *testing.T) {
	r := NewPresetRule("ru-direct", map[string]string{"out": "direct-out"})
	out, _ := json.Marshal(r)
	if !strings.Contains(string(out), `"vars":{"out":"direct-out"}`) {
		t.Errorf("vars must live on the record: %s", out)
	}
	if strings.Contains(string(out), `"body"`) {
		t.Errorf("preset must not emit body: %s", out)
	}
}

// TestRule_RoundTrip_Inline — user inline rule: имя снаружи, тело — правило.
//
// Legacy-ключ "id" из старых файлов больше не молчит: в v8 он — законные
// метаданные записи, лаунчер их провозит.
func TestRule_RoundTrip_Inline(t *testing.T) {
	raw := []byte(`{
		"kind": "inline",
		"id": "01J9X0000000000000000000A",
		"name": "Firefox через VPN",
		"enabled": true,
		"num": 1000,
		"body": {
			"domain_suffix": ["example.com"],
			"package_name": ["org.mozilla.firefox"],
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
	if r.ID != "01J9X0000000000000000000A" {
		t.Errorf("id must be carried through, got %q", r.ID)
	}
	if r.Num == nil || *r.Num != 1000 {
		t.Errorf("num mismatch: %v", r.Num)
	}
	if got := StableRuleID(r); got != "Firefox--VPN" {
		t.Errorf("StableRuleID: %q (want sanitize of name)", got)
	}

	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("DecodeBody: %v", err)
	}
	ib := body.(*InlineBody)
	if ib.Name != "Firefox через VPN" || ib.Outbound != "proxy-out" {
		t.Errorf("view mismatch: %+v", ib)
	}
	if domains, ok := ib.Match["domain_suffix"].([]interface{}); !ok || len(domains) != 1 {
		t.Errorf("match.domain_suffix mismatch: %+v", ib.Match)
	}
	if _, leaked := ib.Match["outbound"]; leaked {
		t.Errorf("Match must not carry the target: %+v", ib.Match)
	}
}

// TestRule_InlineView_RejectAndDrop — цель вида вычисляется из тела sing-box.
func TestRule_InlineView_RejectAndDrop(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{"port":[443],"action":"reject"}`, "reject"},
		{`{"port":[443],"action":"reject","method":"drop"}`, "drop"},
		{`{"port":[443],"outbound":"proxy-out"}`, "proxy-out"},
		{`{"port":[443]}`, ""},
	}
	for _, tc := range cases {
		r := Rule{Kind: RuleKindInline, Name: "x", Body: json.RawMessage(tc.body)}
		body, err := r.DecodeBody()
		if err != nil {
			t.Fatalf("DecodeBody(%s): %v", tc.body, err)
		}
		if got := body.(*InlineBody).Outbound; got != tc.want {
			t.Errorf("body %s → outbound %q, want %q", tc.body, got, tc.want)
		}
	}
}

// TestRule_NewInlineRule_TargetInBody — конструктор кладёт цель в тело в форме
// sing-box (ApplyOutboundToRule), а матчеры оставляет как переданы.
func TestRule_NewInlineRule_TargetInBody(t *testing.T) {
	r := NewInlineRule("blocked", map[string]interface{}{"domain_suffix": []string{"ads.example"}}, "drop")
	var body map[string]interface{}
	if err := json.Unmarshal(r.Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["action"] != "reject" || body["method"] != "drop" {
		t.Errorf("drop must become action=reject+method=drop: %v", body)
	}
	if _, has := body["outbound"]; has {
		t.Errorf("outbound must be absent for drop: %v", body)
	}
	if r.Name != "blocked" {
		t.Errorf("name must live on the record: %q", r.Name)
	}
}

// TestRule_SetOutbound_KeepsMatcherOrder — перепись цели не трогает ни ключи
// матчеров, ни их порядок (нужно applyRenames и редактору UI).
func TestRule_SetOutbound_KeepsMatcherOrder(t *testing.T) {
	r := Rule{
		Kind: RuleKindInline, Name: "x",
		Body: json.RawMessage(`{"zzz":1,"aaa":2,"outbound":"old-out"}`),
	}
	if err := r.SetOutbound("new-out"); err != nil {
		t.Fatalf("SetOutbound: %v", err)
	}
	if string(r.Body) != `{"zzz":1,"aaa":2,"outbound":"new-out"}` {
		t.Errorf("matcher order or keys changed: %s", r.Body)
	}
	if err := r.SetOutbound("reject"); err != nil {
		t.Fatalf("SetOutbound reject: %v", err)
	}
	if string(r.Body) != `{"zzz":1,"aaa":2,"action":"reject"}` {
		t.Errorf("reject rewrite: %s", r.Body)
	}
	if err := r.SetOutbound("drop"); err != nil {
		t.Fatalf("SetOutbound drop: %v", err)
	}
	if string(r.Body) != `{"zzz":1,"aaa":2,"action":"reject","method":"drop"}` {
		t.Errorf("drop rewrite: %s", r.Body)
	}
}

// TestRule_RoundTrip_Srs — user srs rule с reject: наборы в refs[], цель в body.
func TestRule_RoundTrip_Srs(t *testing.T) {
	raw := []byte(`{
		"kind": "srs",
		"name": "Custom block list",
		"enabled": true,
		"refs": ["https://example.com/blocklist.srs"],
		"body": {"action": "reject"}
	}`)
	var r Rule
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := StableRuleID(r); got != "Custom-block-list" {
		t.Errorf("StableRuleID: %q (want sanitize of name)", got)
	}
	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("DecodeBody: %v", err)
	}
	sb := body.(*SrsBody)
	if len(sb.URLs()) != 1 || sb.URLs()[0] != "https://example.com/blocklist.srs" {
		t.Errorf("refs mismatch: %v", sb.URLs())
	}
	if sb.Outbound != "reject" {
		t.Errorf("outbound mismatch: %q (expected reject sentinel)", sb.Outbound)
	}
}

// TestRule_NewSrsRule_DedupKeepsOrder — канонизация наборов переехала из
// NewSrsBody в конструктор записи: дубли и пустые вон, порядок цел.
func TestRule_NewSrsRule_DedupKeepsOrder(t *testing.T) {
	r := NewSrsRule("три набора", []string{"https://b", "", "https://a", "https://b"}, "proxy-out")
	want := []string{"https://b", "https://a"}
	if len(r.Refs) != len(want) {
		t.Fatalf("refs: %v, want %v", r.Refs, want)
	}
	for i := range want {
		if r.Refs[i] != want[i] {
			t.Fatalf("refs: %v, want %v", r.Refs, want)
		}
	}
	if string(r.Body) != `{"outbound":"proxy-out"}` {
		t.Errorf("srs body must carry the target only: %s", r.Body)
	}
}

// TestRule_PresetWithoutRef_Error — kind=preset без ref → ошибка.
func TestRule_PresetWithoutRef_Error(t *testing.T) {
	r := Rule{Kind: RuleKindPreset}
	_, err := r.DecodeBody()
	if err == nil {
		t.Fatal("expected error: kind=preset without ref")
	}
	if !strings.Contains(err.Error(), "requires ref") {
		t.Errorf("error text mismatch: %v", err)
	}
}

// TestRule_InlineWithoutName_Error — kind=inline без name → ошибка.
func TestRule_InlineWithoutName_Error(t *testing.T) {
	r := Rule{
		Kind: RuleKindInline,
		Body: json.RawMessage(`{"port":[443],"outbound":"direct-out"}`),
	}
	_, err := r.DecodeBody()
	if err == nil {
		t.Fatal("expected error: kind=inline without name")
	}
	if !strings.Contains(err.Error(), "requires name") {
		t.Errorf("error text mismatch: %v", err)
	}
}

// TestRule_SrsWithoutName_Error — kind=srs без name → ошибка.
func TestRule_SrsWithoutName_Error(t *testing.T) {
	r := Rule{
		Kind: RuleKindSrs,
		Refs: []string{"https://x"},
		Body: json.RawMessage(`{"action":"reject"}`),
	}
	_, err := r.DecodeBody()
	if err == nil {
		t.Fatal("expected error: kind=srs without name")
	}
	if !strings.Contains(err.Error(), "requires name") {
		t.Errorf("error text mismatch: %v", err)
	}
}

// TestRule_SrsWithoutRefs_Error — kind=srs без единого набора → ошибка.
func TestRule_SrsWithoutRefs_Error(t *testing.T) {
	r := Rule{
		Kind: RuleKindSrs, Name: "x",
		Body: json.RawMessage(`{"action":"reject"}`),
	}
	_, err := r.DecodeBody()
	if err == nil {
		t.Fatal("expected error: kind=srs without refs")
	}
	if !strings.Contains(err.Error(), "requires refs") {
		t.Errorf("error text mismatch: %v", err)
	}
}

// TestRule_InlineWithRef_Error — kind=inline с лишним ref → ошибка.
func TestRule_InlineWithRef_Error(t *testing.T) {
	r := Rule{
		Kind: RuleKindInline, Ref: "leaked", Name: "x",
		Body: json.RawMessage(`{"outbound":"direct-out"}`),
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

// TestRule_OmitEmpty — пустые поля записи не пишутся.
func TestRule_OmitEmpty(t *testing.T) {
	r := Rule{Kind: RuleKindInline, Name: "x", Enabled: true, Body: json.RawMessage(`{}`)}
	out, _ := json.Marshal(r)
	for _, mustNotContain := range []string{`"ref":`, `"id":`, `"refs":`, `"vars":`, `"num":`} {
		if strings.Contains(string(out), mustNotContain) {
			t.Errorf("%s should be omitted for an inline rule: %s", mustNotContain, out)
		}
	}

	r2 := NewPresetRule("x", nil)
	r2.Enabled = true
	out2, _ := json.Marshal(r2)
	for _, mustNotContain := range []string{`"id":`, `"name":`, `"refs":`, `"body":`} {
		if strings.Contains(string(out2), mustNotContain) {
			t.Errorf("%s should be omitted for a preset rule: %s", mustNotContain, out2)
		}
	}
}

// TestRule_KeyOrder — порядок полей структуры = порядок ключей файла.
func TestRule_KeyOrder(t *testing.T) {
	num := 1010
	r := Rule{
		Kind: RuleKindSrs, ID: "i", Name: "n", Enabled: true, Num: &num,
		Refs: []string{"https://a"}, Body: json.RawMessage(`{"outbound":"o"}`),
	}
	out, _ := json.Marshal(r)
	want := `{"kind":"srs","id":"i","name":"n","enabled":true,"num":1010,"refs":["https://a"],"body":{"outbound":"o"}}`
	if string(out) != want {
		t.Errorf("key order drifted:\n got %s\nwant %s", out, want)
	}
}

// TestDNSOptions_RoundTrip — форма v8: метаданные снаружи, тело в body.
func TestDNSOptions_RoundTrip(t *testing.T) {
	// independent_cache в payload — legacy/forward-compat поле, JSON
	// unmarshal должен silently игнорировать (поле снято из DNSOptions).
	raw := []byte(`{
		"strategy": "prefer_ipv4",
		"independent_cache": true,
		"final": "google_doh",
		"default_domain_resolver": "google_doh",
		"servers": [
			{"kind":"template", "tag":"cloudflare_udp", "enabled":true},
			{"kind":"template", "tag":"yandex_doh", "enabled":false},
			{"kind":"preset",   "ref":"russian:yandex_udp", "enabled":true},
			{"kind":"user",     "tag":"my-pihole", "enabled":true,
			 "body":{"type":"udp","server":"192.168.1.5","server_port":53}}
		],
		"rules": [
			{"kind":"preset", "ref":"russian", "enabled":true},
			{"kind":"user",   "enabled":true,
			 "body":{"rule_set":"ru-domains","server":"yandex_doh"}}
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
	if d.Servers[1].Enabled {
		t.Errorf("template entry 1 must stay disabled: %+v", d.Servers[1])
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
	if _, leaked := d.Servers[3].Body["tag"]; leaked {
		t.Errorf("tag must stay out of the body: %+v", d.Servers[3].Body)
	}
	if d.Rules[0].Kind != DNSRuleKindPreset || d.Rules[0].Ref != "russian" {
		t.Errorf("preset rule: %+v", d.Rules[0])
	}
	if d.Rules[1].Body["rule_set"] != "ru-domains" {
		t.Errorf("user rule body lost: %+v", d.Rules[1].Body)
	}
	// Тела у ссылочных записей нет — это проверяет сборка.
	if d.Servers[0].Body != nil || d.Servers[2].Body != nil || d.Rules[0].Body != nil {
		t.Errorf("template/preset entries must carry no body: %+v", d)
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
	if SchemaVersionV7 != 7 {
		t.Errorf("SchemaVersionV7 should be 7, got %d", SchemaVersionV7)
	}
	if SchemaNameV7 != "sources_v7" {
		t.Errorf("SchemaNameV7 mismatch: %q", SchemaNameV7)
	}
	if SchemaVersionV8 != 8 || SchemaVersion != SchemaVersionV8 {
		t.Errorf("SchemaVersion should be v8 (SPEC 127), got %d", SchemaVersion)
	}
	if SchemaNameV8 != "sources_v8" {
		t.Errorf("SchemaNameV8 mismatch: %q", SchemaNameV8)
	}
	if SchemaMajor != SchemaVersionV8 {
		t.Errorf("SchemaMajor must follow the write format, got %d", SchemaMajor)
	}
}
