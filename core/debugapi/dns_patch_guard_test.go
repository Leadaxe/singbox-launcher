package debugapi

import (
	"net/http"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// A keyless PATCH /state/dns body ({}) must NOT wipe dns_options: respond 422
// and never call SaveState. Regression for the silent-clear bug found on
// v1.1.5-6-g2042705 (PATCH /state/rules already 400s on {}; dns must guard too).
func TestPatchStateDNS_EmptyBodyDoesNotClear(t *testing.T) {
	port := freeLocalPort(t)
	st := state.New()
	st.DNS.Servers = []state.DNSServer{{Kind: state.DNSServerKindUser, Tag: "cf", Enabled: true}}
	ff := &fakeFacade{stateValue: st}
	s, err := New(ff, port, "tok")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Start()
	defer s.Stop()
	base := "http://127.0.0.1:" + itoa(port)

	cases := map[string]string{
		"empty object": "{}",
		"only unknown": `{"foo":1}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			ff.savedState = nil
			req, _ := http.NewRequest("PATCH", base+"/state/dns", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer tok")
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("patch: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422", resp.StatusCode)
			}
			if ff.savedState != nil {
				t.Error("SaveState must NOT be called on a keyless body (state was wiped)")
			}
		})
	}

	// Sanity: a body WITH an explicit empty servers array is a legit clear → 200.
	req, _ := http.NewRequest("PATCH", base+"/state/dns", strings.NewReader(`{"servers":[],"rules":[]}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch explicit: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("explicit empty arrays: status = %d, want 200", resp.StatusCode)
	}
}

// /help advertises method sets that the handlers actually accept: every method
// listed in an endpoint's Method ("GET/PATCH", "GET/DELETE", …) must not return
// 405. Keeps the registry honest about verbs (bugs: user-agent listed PUT but
// serves PATCH; write verbs were missing from the list).
func TestHelpMethodsMatchHandlers(t *testing.T) {
	port := freeLocalPort(t)
	s, err := New(&fakeFacade{stateValue: state.New()}, port, "tok")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Start()
	defer s.Stop()
	base := "http://127.0.0.1:" + itoa(port)

	for _, e := range s.endpoints() {
		if strings.HasSuffix(e.Path, "/") {
			continue // prefix routes (/traffic/sessions/) need an ID; covered elsewhere
		}
		for _, m := range strings.Split(e.Method, "/") {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			req, _ := http.NewRequest(m, base+e.Path, strings.NewReader("{}"))
			req.Header.Set("Authorization", "Bearer tok")
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", m, e.Path, err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusMethodNotAllowed {
				t.Errorf("%s %s advertised in /help but handler returns 405", m, e.Path)
			}
		}
	}
}
