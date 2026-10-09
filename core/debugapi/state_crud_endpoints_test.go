package debugapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// TestStateCRUDEndpoints — сценарий инцидента через API (SPEC 160): узел
// добавляется точкой /state/servers, правила при этом целы и config.json
// пересобран; неизвестная цель правила — 422; правило снимается по num.
func TestStateCRUDEndpoints(t *testing.T) {
	st := state.New()
	st.Directions = []configtypes.Direction{{Tag: "proxy-out", Type: "selector", AddOutbounds: []string{"direct-out"}}}
	n1000, n1001 := 1000, 1001
	st.Rules = []state.Rule{
		{Kind: state.RuleKindInline, Name: "r1", Enabled: true, Num: &n1000, Body: json.RawMessage(`{"domain_suffix":["a.com"],"outbound":"proxy-out"}`)},
		{Kind: state.RuleKindInline, Name: "r2", Enabled: true, Num: &n1001, Body: json.RawMessage(`{"domain_suffix":["b.com"],"outbound":"direct-out"}`)},
	}
	ff := &fakeFacade{stateValue: st}
	base, _ := newTestServer(t, ff)

	body, _ := json.Marshal(map[string]string{
		"input": "vless://2ee2a715-d541-416a-8713-d66567448c2e@91.98.155.240:443?encryption=none&security=none&type=grpc#de-1",
	})
	var added struct {
		OK            bool `json:"ok"`
		ConfigRebuilt bool `json:"config_rebuilt"`
		Added         []struct {
			Tag string `json:"tag"`
		} `json:"added"`
	}
	status, raw := doJSON(t, authedReq(t, http.MethodPost, base+"/state/servers", body), &added)
	if status != http.StatusOK || len(added.Added) != 1 || added.Added[0].Tag != "de-1" {
		t.Fatalf("POST /state/servers: %d %s", status, raw)
	}
	if ff.savedState == nil || len(ff.savedState.Rules) != 2 {
		t.Fatalf("rules after adding a server: %+v", ff.savedState)
	}
	if !added.ConfigRebuilt || ff.rebuilds != 1 {
		t.Errorf("config not rebuilt: config_rebuilt=%v rebuilds=%d", added.ConfigRebuilt, ff.rebuilds)
	}

	if status, raw := doJSON(t, authedReq(t, http.MethodDelete, base+"/state/servers?tag=nope", nil), nil); status != http.StatusNotFound {
		t.Errorf("DELETE unknown server: %d %s, want 404", status, raw)
	}

	ff.savedState = nil
	bad := []byte(`{"kind":"inline","name":"x","body":{"domain":["x.com"],"outbound":"nowhere"}}`)
	var fieldResp struct {
		Field string `json:"field"`
	}
	status, raw = doJSON(t, authedReq(t, http.MethodPost, base+"/state/rules", bad), &fieldResp)
	if status != http.StatusUnprocessableEntity || fieldResp.Field != "body.outbound" {
		t.Errorf("POST /state/rules unknown target: %d %s, want 422 body.outbound", status, raw)
	}
	if ff.savedState != nil {
		t.Error("rejected rule must not save state")
	}

	status, raw = doJSON(t, authedReq(t, http.MethodDelete, base+"/state/rules?num=1000", nil), nil)
	if status != http.StatusOK {
		t.Fatalf("DELETE /state/rules?num=1000: %d %s", status, raw)
	}
	if got := ff.savedState.Rules; len(got) != 1 || got[0].Name != "r2" {
		t.Errorf("rules after delete: %+v, want only r2", got)
	}
}
