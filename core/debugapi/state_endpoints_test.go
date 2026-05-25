package debugapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// TestStateFull — GET returns marshalled state including all SPEC 053/056/057/058
// surfaces (Rules, DNS, Connections.Outbounds with Ref/Updates).
func TestStateFull(t *testing.T) {
	st := state.New()
	st.Rules = []state.Rule{
		{Kind: state.RuleKindPreset, Ref: "ru-direct", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
	}
	st.DNS = state.DNSOptions{
		Final: "google_doh",
		Servers: []state.DNSServer{
			{Kind: state.DNSServerKindTemplate, Tag: "google_doh", Enabled: true},
		},
	}
	st.Connections.Outbounds = []configtypes.OutboundConfig{
		{Tag: "proxy-out", Ref: configtypes.RefTemplate},
	}
	ff := &fakeFacade{stateValue: st}
	base, _ := newTestServer(t, ff)

	resp, err := http.DefaultClient.Do(authedReq(t, "GET", base+"/state/full", nil))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var got state.State
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Verify all new surfaces survived round-trip.
	if len(got.Rules) != 1 || got.Rules[0].Ref != "ru-direct" {
		t.Errorf("rules lost: %+v", got.Rules)
	}
	if got.DNS.Final != "google_doh" || len(got.DNS.Servers) != 1 {
		t.Errorf("dns lost: %+v", got.DNS)
	}
	if len(got.Connections.Outbounds) != 1 || got.Connections.Outbounds[0].Ref != configtypes.RefTemplate {
		t.Errorf("outbounds.ref lost: %+v", got.Connections.Outbounds)
	}
}

// TestStateFullNotFound — fresh install (state.ErrNotFound) → 404.
func TestStateFullNotFound(t *testing.T) {
	ff := &fakeFacade{stateLoadErr: state.ErrNotFound}
	base, _ := newTestServer(t, ff)
	resp, err := http.DefaultClient.Do(authedReq(t, "GET", base+"/state/full", nil))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status: want 404, got %d", resp.StatusCode)
	}
}

// TestStateRulesGet — focused read of /state/rules.
func TestStateRulesGet(t *testing.T) {
	st := state.New()
	st.Rules = []state.Rule{
		{Kind: state.RuleKindInline, ID: "01J", Enabled: true, Body: json.RawMessage(`{"name":"x","match":{},"outbound":"direct-out"}`)},
	}
	ff := &fakeFacade{stateValue: st}
	base, _ := newTestServer(t, ff)
	var got struct {
		Rules []state.Rule `json:"rules"`
	}
	status, _ := doJSON(t, authedReq(t, "GET", base+"/state/rules", nil), &got)
	if status != 200 || len(got.Rules) != 1 {
		t.Fatalf("status=%d rules=%+v", status, got.Rules)
	}
}

// TestStateRulesPatch — table-driven mode switching + validation.
func TestStateRulesPatch(t *testing.T) {
	cases := []struct {
		name        string
		mode        string
		rules       []state.Rule
		wantStatus  int
		wantCount   int
		wantInBody  string
	}{
		{
			name: "replace_empty",
			mode: "replace",
			rules: []state.Rule{},
			wantStatus: 200,
			wantCount:  0,
		},
		{
			name: "replace_one_preset",
			mode: "replace",
			rules: []state.Rule{
				{Kind: state.RuleKindPreset, Ref: "ru-direct", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
			},
			wantStatus: 200,
			wantCount:  1,
		},
		{
			name: "append_inline",
			mode: "append",
			rules: []state.Rule{
				{Kind: state.RuleKindInline, ID: "01K", Enabled: true, Body: json.RawMessage(`{"name":"a","match":{},"outbound":"direct-out"}`)},
			},
			wantStatus: 200,
			wantCount:  2, // initial state has 1 below
		},
		{
			name:       "bad_mode",
			mode:       "merge",
			rules:      nil,
			wantStatus: 400,
		},
		{
			name: "bad_rule_kind",
			mode: "replace",
			rules: []state.Rule{
				{Kind: "wat", Body: json.RawMessage(`{}`)},
			},
			wantStatus: 422,
			wantInBody: "unknown rule kind",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Seed initial state with 1 rule so append can verify the
			// 2 = 1 + 1 outcome.
			init := state.New()
			init.Rules = []state.Rule{
				{Kind: state.RuleKindInline, ID: "01J", Enabled: true, Body: json.RawMessage(`{"name":"existing","match":{},"outbound":"direct-out"}`)},
			}
			ff := &fakeFacade{stateValue: init}
			base, _ := newTestServer(t, ff)

			body, _ := json.Marshal(patchRulesReq{Mode: tc.mode, Rules: tc.rules})
			status, raw := doJSON(t, authedReq(t, "PATCH", base+"/state/rules", body), nil)
			if status != tc.wantStatus {
				t.Fatalf("status: want %d, got %d body=%s", tc.wantStatus, status, raw)
			}
			if tc.wantInBody != "" && !strings.Contains(string(raw), tc.wantInBody) {
				t.Errorf("body: want substring %q, got %s", tc.wantInBody, raw)
			}
			if tc.wantStatus == 200 {
				if ff.savedState == nil {
					t.Fatalf("savedState: nil after successful PATCH")
				}
				if len(ff.savedState.Rules) != tc.wantCount {
					t.Errorf("rule count: want %d, got %d", tc.wantCount, len(ff.savedState.Rules))
				}
			}
		})
	}
}

// TestStateDNSGet — read DNS section.
func TestStateDNSGet(t *testing.T) {
	st := state.New()
	st.DNS = state.DNSOptions{
		Final: "google_doh",
		Servers: []state.DNSServer{
			{Kind: state.DNSServerKindUser, Tag: "pihole", Enabled: true, Body: map[string]interface{}{"type": "udp", "server": "192.168.1.5"}},
		},
	}
	ff := &fakeFacade{stateValue: st}
	base, _ := newTestServer(t, ff)
	var got state.DNSOptions
	status, _ := doJSON(t, authedReq(t, "GET", base+"/state/dns", nil), &got)
	if status != 200 {
		t.Fatalf("status: %d", status)
	}
	if got.Final != "google_doh" || len(got.Servers) != 1 {
		t.Errorf("dns: %+v", got)
	}
}

// TestStateDNSPatchValid — replace entire dns section.
func TestStateDNSPatchValid(t *testing.T) {
	ff := &fakeFacade{stateValue: state.New()}
	base, _ := newTestServer(t, ff)
	body := []byte(`{"final":"cloudflare_doh","servers":[{"kind":"template","tag":"cloudflare_doh","enabled":true}]}`)
	status, raw := doJSON(t, authedReq(t, "PATCH", base+"/state/dns", body), nil)
	if status != 200 {
		t.Fatalf("status: %d body=%s", status, raw)
	}
	if ff.savedState == nil || ff.savedState.DNS.Final != "cloudflare_doh" {
		t.Errorf("save: %+v", ff.savedState)
	}
}

// TestStateDNSPatchBadKind — unknown server kind → 422.
func TestStateDNSPatchBadKind(t *testing.T) {
	ff := &fakeFacade{stateValue: state.New()}
	base, _ := newTestServer(t, ff)
	body := []byte(`{"servers":[{"kind":"weird","tag":"x","enabled":true}]}`)
	status, raw := doJSON(t, authedReq(t, "PATCH", base+"/state/dns", body), nil)
	if status != 422 {
		t.Errorf("status: want 422, got %d body=%s", status, raw)
	}
}

// TestStateDNSRulesGet — returns USER rules as wizard-text {"rules":[...]}.
// PRESET rules are filtered out (they don't have a text representation —
// they're toggle-refs into the template).
func TestStateDNSRulesGet(t *testing.T) {
	st := state.New()
	st.DNS = state.DNSOptions{
		Rules: []state.DNSRule{
			{Kind: state.DNSRuleKindPreset, Ref: "russian", Enabled: true},
			{Kind: state.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{"domain": "example.com", "server": "cf"}},
			{Kind: state.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{"domain": "internal.lan", "server": "pihole"}},
		},
	}
	ff := &fakeFacade{stateValue: st}
	base, _ := newTestServer(t, ff)
	var got struct {
		Text string `json:"text"`
	}
	status, _ := doJSON(t, authedReq(t, "GET", base+"/state/dns/rules", nil), &got)
	if status != 200 {
		t.Fatalf("status: %d", status)
	}
	// Sanity: parses back to 2 entries (preset was filtered out).
	var root struct {
		Rules []map[string]interface{} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(got.Text), &root); err != nil {
		t.Fatalf("unmarshal returned text: %v\ntext=%q", err, got.Text)
	}
	if len(root.Rules) != 2 {
		t.Errorf("want 2 user rules in text, got %d: %s", len(root.Rules), got.Text)
	}
}

// TestStateDNSRulesGetEmpty — no rules at all → empty text.
func TestStateDNSRulesGetEmpty(t *testing.T) {
	ff := &fakeFacade{stateValue: state.New()}
	base, _ := newTestServer(t, ff)
	var got struct {
		Text string `json:"text"`
	}
	status, _ := doJSON(t, authedReq(t, "GET", base+"/state/dns/rules", nil), &got)
	if status != 200 || got.Text != "" {
		t.Errorf("status=%d text=%q (want 200 + empty)", status, got.Text)
	}
}

// TestStateDNSRulesPatch — table-driven: text replaces USER rules;
// PRESET rules are preserved across the PATCH.
func TestStateDNSRulesPatch(t *testing.T) {
	cases := []struct {
		name           string
		text           string
		wantStatus     int
		wantUserCount  int // expected USER-kind rules after save
		wantPresetKept bool
		wantInBody     string
	}{
		{
			name:           "replace_two_objects",
			text:           `{"rules":[{"domain":"a"},{"domain":"b"}]}`,
			wantStatus:     200,
			wantUserCount:  2,
			wantPresetKept: true,
		},
		{
			name:           "empty_clears_user",
			text:           "",
			wantStatus:     200,
			wantUserCount:  0,
			wantPresetKept: true,
		},
		{
			name:           "legacy_multiline",
			text:           "{\"domain\":\"a\"}\n# comment\n{\"domain\":\"b\"}",
			wantStatus:     200,
			wantUserCount:  2,
			wantPresetKept: true,
		},
		{
			name:           "single_object",
			text:           `{"domain":"only"}`,
			wantStatus:     200,
			wantUserCount:  1,
			wantPresetKept: true,
		},
		{
			name:       "bad_text_not_object",
			text:       `"just a string"`,
			wantStatus: 422,
			wantInBody: "expected JSON object",
		},
		{
			name:       "bad_rules_field_type",
			text:       `{"rules":"oops"}`,
			wantStatus: 422,
			wantInBody: "expected JSON array",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Seed: 1 preset + 1 user. After PATCH, preset should remain.
			init := state.New()
			init.DNS = state.DNSOptions{
				Rules: []state.DNSRule{
					{Kind: state.DNSRuleKindPreset, Ref: "russian", Enabled: true},
					{Kind: state.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{"domain": "stale"}},
				},
			}
			ff := &fakeFacade{stateValue: init}
			base, _ := newTestServer(t, ff)

			body, _ := json.Marshal(patchDNSRulesReq{Text: tc.text})
			status, raw := doJSON(t, authedReq(t, "PATCH", base+"/state/dns/rules", body), nil)
			if status != tc.wantStatus {
				t.Fatalf("status: want %d, got %d body=%s", tc.wantStatus, status, raw)
			}
			if tc.wantInBody != "" && !strings.Contains(string(raw), tc.wantInBody) {
				t.Errorf("body: want substring %q, got %s", tc.wantInBody, raw)
			}
			if tc.wantStatus != 200 {
				return
			}
			if ff.savedState == nil {
				t.Fatalf("savedState: nil after 200")
			}
			userN := 0
			presetN := 0
			for _, r := range ff.savedState.DNS.Rules {
				switch r.Kind {
				case state.DNSRuleKindUser:
					userN++
				case state.DNSRuleKindPreset:
					presetN++
				}
			}
			if userN != tc.wantUserCount {
				t.Errorf("user count: want %d, got %d", tc.wantUserCount, userN)
			}
			if tc.wantPresetKept && presetN != 1 {
				t.Errorf("preset kept: want 1, got %d", presetN)
			}
		})
	}
}

// TestStateDNSRulesPatchBadJSON — malformed wrapper body (not {text:...}) → 400.
func TestStateDNSRulesPatchBadJSON(t *testing.T) {
	ff := &fakeFacade{stateValue: state.New()}
	base, _ := newTestServer(t, ff)
	status, raw := doJSON(t, authedReq(t, "PATCH", base+"/state/dns/rules", []byte(`{not json`)), nil)
	if status != 400 {
		t.Errorf("status: want 400, got %d body=%s", status, raw)
	}
}

// TestStateOutboundsResolved — direct entry passes through; we don't
// exercise the full template lookup here (the resolver itself has its own
// tests under core/build/), just verify the endpoint shape.
func TestStateOutboundsResolved(t *testing.T) {
	st := state.New()
	st.Connections.Outbounds = []configtypes.OutboundConfig{
		{Tag: "my-direct", Type: "direct"},
	}
	ff := &fakeFacade{stateValue: st, templateValue: &template.TemplateData{}}
	base, _ := newTestServer(t, ff)
	var got struct {
		Outbounds []configtypes.OutboundConfig `json:"outbounds"`
	}
	status, _ := doJSON(t, authedReq(t, "GET", base+"/state/outbounds/resolved", nil), &got)
	if status != 200 {
		t.Fatalf("status: %d", status)
	}
	if len(got.Outbounds) != 1 || got.Outbounds[0].Tag != "my-direct" {
		t.Errorf("outbounds: %+v", got.Outbounds)
	}
}

// TestStateOutboundsResolvedTemplateErr — propagates template load
// failure as 500.
func TestStateOutboundsResolvedTemplateErr(t *testing.T) {
	ff := &fakeFacade{stateValue: state.New(), templateErr: errors.New("template missing")}
	base, _ := newTestServer(t, ff)
	resp, err := http.DefaultClient.Do(authedReq(t, "GET", base+"/state/outbounds/resolved", nil))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Errorf("status: want 500, got %d", resp.StatusCode)
	}
}

// TestStateAuthGuard — all new state endpoints reject missing bearer.
func TestStateAuthGuard(t *testing.T) {
	base, _ := newTestServer(t, &fakeFacade{stateValue: state.New()})
	for _, p := range []struct{ method, path string }{
		{"GET", "/state/full"},
		{"GET", "/state/rules"},
		{"PATCH", "/state/rules"},
		{"GET", "/state/dns"},
		{"PATCH", "/state/dns"},
		{"GET", "/state/dns/rules"},
		{"PATCH", "/state/dns/rules"},
		{"GET", "/state/outbounds/resolved"},
	} {
		t.Run(p.method+p.path, func(t *testing.T) {
			req, _ := http.NewRequest(p.method, base+p.path, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != 401 {
				t.Errorf("status: %d", resp.StatusCode)
			}
		})
	}
}
