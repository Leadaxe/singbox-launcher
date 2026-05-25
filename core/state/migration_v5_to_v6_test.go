package state

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMigrate_BumpsVersionAndSchema(t *testing.T) {
	old := diskStateV5{Meta: metaSectionV5{Version: 5, CreatedAt: "2026-01-01T00:00:00Z"}}
	new, _ := migrateV5ToV6(old, nil, nil)
	if new.Meta.Version != 6 {
		t.Errorf("Version: got %d want 6", new.Meta.Version)
	}
	if new.Meta.Schema != "presets_v1" {
		t.Errorf("Schema: got %q want presets_v1", new.Meta.Schema)
	}
	if new.Meta.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("CreatedAt should be preserved: %q", new.Meta.CreatedAt)
	}
	if new.Meta.UpdatedAt == "" {
		t.Error("UpdatedAt should be set")
	}
}

func TestMigrate_PreservesConnectionsAndVars(t *testing.T) {
	old := diskStateV5{
		Connections: ConnectionsSection{
			Sources:  []Source{{ID: "abc", Type: SourceTypeSubscription, URL: "https://x"}},
			Defaults: Defaults{MaxNodes: 100},
		},
		Vars: []SettingVar{{Name: "cert_store", Value: "mozilla"}},
	}
	new, _ := migrateV5ToV6(old, nil, nil)
	if len(new.Connections.Sources) != 1 || new.Connections.Sources[0].ID != "abc" {
		t.Errorf("connections.sources lost: %+v", new.Connections.Sources)
	}
	if new.Connections.Defaults.MaxNodes != 100 {
		t.Errorf("defaults lost: %+v", new.Connections.Defaults)
	}
	if len(new.Vars) != 1 || new.Vars[0].Name != "cert_store" {
		t.Errorf("vars lost: %+v", new.Vars)
	}
}

// TestMigrate_CustomRule_Inline — простое inline правило → kind=inline.
func TestMigrate_CustomRule_Inline(t *testing.T) {
	old := diskStateV5{
		CustomRules: []CustomRule{
			{
				Label:            "Firefox через VPN",
				Enabled:          true,
				SelectedOutbound: "proxy-out",
				Rule: map[string]interface{}{
					"domain_suffix": []interface{}{"example.com"},
					"package_name":  []interface{}{"org.mozilla.firefox"},
					"outbound":      "proxy-out",
				},
			},
		},
	}
	new, _ := migrateV5ToV6(old, nil, nil)
	if len(new.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(new.Rules))
	}
	r := new.Rules[0]
	if r.Kind != RuleKindInline {
		t.Errorf("kind: %q want inline", r.Kind)
	}
	if r.ID == "" {
		t.Error("ID should be generated for inline")
	}
	if r.Ref != "" {
		t.Errorf("Ref should be empty for inline: %q", r.Ref)
	}
	if !r.Enabled {
		t.Error("Enabled should be preserved")
	}
	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("DecodeBody: %v", err)
	}
	ib := body.(*InlineBody)
	if ib.Name != "Firefox через VPN" {
		t.Errorf("name: %q", ib.Name)
	}
	if ib.Outbound != "proxy-out" {
		t.Errorf("outbound: %q", ib.Outbound)
	}
	// outbound должен быть stripped из match
	if _, has := ib.Match["outbound"]; has {
		t.Errorf("match should not contain outbound key: %+v", ib.Match)
	}
	if _, has := ib.Match["domain_suffix"]; !has {
		t.Errorf("match should contain domain_suffix: %+v", ib.Match)
	}
}

// TestMigrate_CustomRule_Srs — правило с remote rule_set → kind=srs.
func TestMigrate_CustomRule_Srs(t *testing.T) {
	old := diskStateV5{
		CustomRules: []CustomRule{
			{
				Label:            "Custom block list",
				Enabled:          true,
				SelectedOutbound: "reject",
				RuleSet: []json.RawMessage{
					json.RawMessage(`{"type": "remote", "url": "https://example.com/list.srs"}`),
				},
				Rule: map[string]interface{}{"rule_set": "custom-list", "outbound": "reject"},
			},
		},
	}
	new, _ := migrateV5ToV6(old, nil, nil)
	if len(new.Rules) != 1 {
		t.Fatalf("expected 1 rule")
	}
	r := new.Rules[0]
	if r.Kind != RuleKindSrs {
		t.Errorf("kind: %q want srs", r.Kind)
	}
	body, _ := r.DecodeBody()
	sb := body.(*SrsBody)
	if sb.SrsURL != "https://example.com/list.srs" {
		t.Errorf("srs_url: %q", sb.SrsURL)
	}
	if sb.Outbound != "reject" {
		t.Errorf("outbound sentinel preserved: %q", sb.Outbound)
	}
}

// TestMigrate_CustomRule_PresetMatch — label совпадает с template-preset → kind=preset.
func TestMigrate_CustomRule_PresetMatch(t *testing.T) {
	old := diskStateV5{
		CustomRules: []CustomRule{
			{
				Label:            "Russian domains direct",
				Enabled:          true,
				SelectedOutbound: "direct-out",
				Rule: map[string]interface{}{
					"rule_set": "ru-domains",
					"outbound": "direct-out",
				},
			},
		},
	}
	presetMap := map[string]string{
		"Russian domains direct": "ru-direct",
	}
	new, _ := migrateV5ToV6(old, nil, presetMap)
	if len(new.Rules) != 1 {
		t.Fatalf("expected 1 rule")
	}
	r := new.Rules[0]
	if r.Kind != RuleKindPreset || r.Ref != "ru-direct" {
		t.Errorf("expected preset-ref to ru-direct: %+v", r)
	}
	if r.ID != "" {
		t.Errorf("preset-ref should not have ID: %q", r.ID)
	}
}

// TestMigrate_CustomRule_NoMatchFields — rule только с outbound → skip + warning.
func TestMigrate_CustomRule_NoMatchFields(t *testing.T) {
	old := diskStateV5{
		CustomRules: []CustomRule{
			{
				Label:            "Bare",
				Enabled:          true,
				SelectedOutbound: "direct-out",
				Rule:             map[string]interface{}{"outbound": "direct-out"},
			},
		},
	}
	new, warns := migrateV5ToV6(old, nil, nil)
	if len(new.Rules) != 0 {
		t.Errorf("expected 0 rules after skipping bare: %+v", new.Rules)
	}
	if len(warns) == 0 {
		t.Error("expected warning")
	}
}

// TestMigrate_DNS_Split — SPEC 056-R-N: template-defined серверы → kind=template,
// user-added → kind=user в flat DNSOptions.Servers[].
func TestMigrate_DNS_Split(t *testing.T) {
	// SPEC: IndependentCache УДАЛЕНО — sing-box 1.14 deprecation; legacy v5
	// поле игнорируется на миграции (его не должно быть в v6 state).
	indep := true
	old := diskStateV5{
		DNSOptions: &LegacyDNSOptionsV5{
			Strategy:              "prefer_ipv4",
			Final:                 "google_doh",
			DefaultDomainResolver: "google_doh",
			IndependentCache:      &indep, // legacy — игнорируется
			Servers: []json.RawMessage{
				json.RawMessage(`{"tag": "google_doh", "type": "https", "server": "dns.google", "enabled": true}`),
				json.RawMessage(`{"tag": "cloudflare_udp", "type": "udp", "server": "1.1.1.1", "enabled": false}`),
				json.RawMessage(`{"tag": "my-pihole", "type": "udp", "server": "192.168.1.5", "server_port": 53, "enabled": true}`),
			},
			Rules: []json.RawMessage{
				json.RawMessage(`{"server": "google_doh"}`),
			},
		},
	}
	templateDefaults := map[string]bool{
		"google_doh":     true,
		"cloudflare_udp": true,
	}
	new, _ := migrateV5ToV6(old, templateDefaults, nil)

	if new.DNSOptions.Strategy != "prefer_ipv4" || new.DNSOptions.Final != "google_doh" {
		t.Errorf("DNS scalars: %+v", new.DNSOptions)
	}

	// Servers: 2 template + 1 user.
	var (
		templateGoogle, templateCF, userPiHole *DNSServer
	)
	for i := range new.DNSOptions.Servers {
		s := &new.DNSOptions.Servers[i]
		switch {
		case s.Kind == DNSServerKindTemplate && s.Tag == "google_doh":
			templateGoogle = s
		case s.Kind == DNSServerKindTemplate && s.Tag == "cloudflare_udp":
			templateCF = s
		case s.Kind == DNSServerKindUser && s.Tag == "my-pihole":
			userPiHole = s
		}
	}
	if templateGoogle == nil || !templateGoogle.Enabled {
		t.Errorf("google_doh template entry should be enabled=true: %+v", templateGoogle)
	}
	if templateCF == nil || templateCF.Enabled {
		t.Errorf("cloudflare_udp template entry should be enabled=false: %+v", templateCF)
	}
	if userPiHole == nil {
		t.Fatalf("my-pihole user entry missing: %+v", new.DNSOptions.Servers)
	}
	if _, has := userPiHole.Body["enabled"]; has {
		t.Errorf("user body should not contain enabled: %+v", userPiHole.Body)
	}
	if userPiHole.Body["server"] != "192.168.1.5" {
		t.Errorf("user body lost server: %+v", userPiHole.Body)
	}

	// Rules → kind=user
	if len(new.DNSOptions.Rules) != 1 {
		t.Errorf("rules count: %+v", new.DNSOptions.Rules)
	}
	if new.DNSOptions.Rules[0].Kind != DNSRuleKindUser {
		t.Errorf("rule kind: %v", new.DNSOptions.Rules[0].Kind)
	}
}

// TestMigrate_NoDNSOptions — отсутствие DNSOptions → пустой DNSOptions без паники.
func TestMigrate_NoDNSOptions(t *testing.T) {
	old := diskStateV5{}
	new, _ := migrateV5ToV6(old, nil, nil)
	if len(new.DNSOptions.Servers) != 0 || len(new.DNSOptions.Rules) != 0 {
		t.Errorf("DNS should be empty: %+v", new.DNSOptions)
	}
}

// TestMigrate_RoundTrip_JSON — migrate → marshal → unmarshal → identical.
func TestMigrate_RoundTrip_JSON(t *testing.T) {
	old := diskStateV5{
		Meta: metaSectionV5{Version: 5, CreatedAt: "2026-01-01T00:00:00Z"},
		CustomRules: []CustomRule{
			{
				Label:            "X",
				Enabled:          true,
				SelectedOutbound: "direct-out",
				Rule:             map[string]interface{}{"ip_is_private": true, "outbound": "direct-out"},
			},
		},
	}
	state1, _ := migrateV5ToV6(old, nil, nil)
	raw, err := json.Marshal(state1)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var state2 diskStateV6
	if err := json.Unmarshal(raw, &state2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if state1.Meta.Version != state2.Meta.Version || len(state1.Rules) != len(state2.Rules) {
		t.Errorf("round-trip mismatch")
	}
}

// TestIsV6_IsV5 — schema detection.
func TestIsV6_IsV5(t *testing.T) {
	v5Raw := []byte(`{"meta":{"version":5}}`)
	v6Raw := []byte(`{"meta":{"version":6}}`)
	bad := []byte(`{not json`)

	if !isV5(v5Raw) || isV5(v6Raw) || isV5(bad) {
		t.Errorf("isV5 misbehaving")
	}
	if !isV6(v6Raw) || isV6(v5Raw) || isV6(bad) {
		t.Errorf("isV6 misbehaving")
	}
}

func TestMigrate_IdempotentRules(t *testing.T) {
	// Идемпотентность тут означает: один и тот же v5 inputs → одинаковая
	// структура rules (минус ULID'ы которые генерятся). Просто sanity что
	// функция детерминированна по schema/header.
	old := diskStateV5{
		Meta:        metaSectionV5{Version: 5, CreatedAt: "2026-01-01T00:00:00Z"},
		CustomRules: []CustomRule{{Label: "A", Enabled: true, SelectedOutbound: "direct-out", Rule: map[string]interface{}{"ip_is_private": true}}},
	}
	new1, _ := migrateV5ToV6(old, nil, nil)
	new2, _ := migrateV5ToV6(old, nil, nil)
	if new1.Meta.Version != new2.Meta.Version {
		t.Error("Version differs")
	}
	if len(new1.Rules) != len(new2.Rules) {
		t.Error("Rules count differs")
	}
	// IDs будут разные (timestamp+counter) — это OK для миграции, но не для round-trip identity.
}

func TestIsLikelyLegacyLabel(t *testing.T) {
	cases := map[string]bool{
		"Russian domains direct": true,
		"Block Ads":              true,
		"Private IPs direct":     true,
		"Custom user rule":       false,
		"":                       false,
	}
	for label, want := range cases {
		if got := isLikelyLegacyLabel(label); got != want {
			t.Errorf("%q: got %v want %v", label, got, want)
		}
	}

	// Просто чтобы поле использовалось — иначе unused warning.
	_ = strings.Title
}
