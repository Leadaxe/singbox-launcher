package debugapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	tprof "singbox-launcher/internal/traffic"
)

// resetProfilerSingleton drains any active session and clears completed.
// The singleton is process-wide so previous tests can leave state behind;
// each test that touches it should call this first.
func resetProfilerSingleton(t *testing.T) {
	t.Helper()
	p := tprof.GetInstance()
	if p.ActiveSession() != nil {
		_, _ = p.StopSession()
	}
	p.ClearAll()
}

// newTestServer spins up a server bound to a free port with a fixed
// token. Returns the base URL and a cleanup hook.
func newTestServer(t *testing.T, ff *fakeFacade) (string, *Server) {
	t.Helper()
	port := freeLocalPort(t)
	s, err := New(ff, port, "tok")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Start()
	t.Cleanup(s.Stop)
	return "http://127.0.0.1:" + itoa(port), s
}

// authedReq builds a request with the test bearer token already set.
func authedReq(t *testing.T, method, url string, body []byte) *http.Request {
	t.Helper()
	var br io.Reader
	if body != nil {
		br = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, br)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer tok")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func doJSON(t *testing.T, req *http.Request, out any) (status int, raw []byte) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do %s %s: %v", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	raw, _ = io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("unmarshal %s %s status=%d body=%s: %v", req.Method, req.URL.Path, resp.StatusCode, string(raw), err)
		}
	}
	return resp.StatusCode, raw
}

// TestTrafficStatus_NoActive — empty state, recording=false.
func TestTrafficStatus_NoActive(t *testing.T) {
	resetProfilerSingleton(t)
	base, _ := newTestServer(t, &fakeFacade{})
	var got trafficStatusResp
	status, _ := doJSON(t, authedReq(t, "GET", base+"/traffic/status", nil), &got)
	if status != 200 {
		t.Fatalf("status: %d", status)
	}
	if got.Recording {
		t.Errorf("recording: want false, got true")
	}
}

// TestTrafficStartStop — happy path: start → status active → stop → not active.
func TestTrafficStartStop(t *testing.T) {
	resetProfilerSingleton(t)
	base, _ := newTestServer(t, &fakeFacade{})

	body, _ := json.Marshal(trafficStartReq{Target: "/Applications/Slack.app/.../Slack", Verbose: true})
	var startResp struct {
		Session trafficSessionSummary `json:"session"`
	}
	status, raw := doJSON(t, authedReq(t, "POST", base+"/traffic/start", body), &startResp)
	if status != 200 {
		t.Fatalf("start: %d body=%s", status, raw)
	}
	if !startResp.Session.Active {
		t.Errorf("session.active: want true, got %+v", startResp.Session)
	}
	if startResp.Session.Target != "/Applications/Slack.app/.../Slack" {
		t.Errorf("session.target: got %q", startResp.Session.Target)
	}

	// /traffic/start again → 409.
	status, _ = doJSON(t, authedReq(t, "POST", base+"/traffic/start", body), nil)
	if status != 409 {
		t.Errorf("double-start: want 409, got %d", status)
	}

	// stop → 200.
	status, _ = doJSON(t, authedReq(t, "POST", base+"/traffic/stop", nil), nil)
	if status != 200 {
		t.Errorf("stop: want 200, got %d", status)
	}

	// stop again → 404.
	status, _ = doJSON(t, authedReq(t, "POST", base+"/traffic/stop", nil), nil)
	if status != 404 {
		t.Errorf("double-stop: want 404, got %d", status)
	}
}

// TestTrafficSessionsListAndGet — start, stop, list contains it, GET by id works.
func TestTrafficSessionsListAndGet(t *testing.T) {
	resetProfilerSingleton(t)
	base, _ := newTestServer(t, &fakeFacade{})

	// Start + stop one session.
	body, _ := json.Marshal(trafficStartReq{Target: "/usr/bin/curl"})
	doJSON(t, authedReq(t, "POST", base+"/traffic/start", body), nil)
	var stopResp struct {
		Session trafficSessionSummary `json:"session"`
	}
	doJSON(t, authedReq(t, "POST", base+"/traffic/stop", nil), &stopResp)

	// List → contains the stopped one.
	var list struct {
		Sessions []trafficSessionSummary `json:"sessions"`
	}
	status, _ := doJSON(t, authedReq(t, "GET", base+"/traffic/sessions", nil), &list)
	if status != 200 {
		t.Fatalf("list: %d", status)
	}
	if len(list.Sessions) != 1 {
		t.Fatalf("list length: %d", len(list.Sessions))
	}
	id := list.Sessions[0].ID

	// GET by id → 200, has the session_export shape.
	var exp trafficSessionExport
	status, raw := doJSON(t, authedReq(t, "GET", base+"/traffic/sessions/"+id, nil), &exp)
	if status != 200 {
		t.Fatalf("get session: %d body=%s", status, raw)
	}
	if exp.ID != id || exp.Target != "/usr/bin/curl" {
		t.Errorf("session export: id=%q target=%q", exp.ID, exp.Target)
	}

	// DELETE → 200.
	status, _ = doJSON(t, authedReq(t, "DELETE", base+"/traffic/sessions/"+id, nil), nil)
	if status != 200 {
		t.Errorf("delete: %d", status)
	}
	// DELETE again → 404.
	status, _ = doJSON(t, authedReq(t, "DELETE", base+"/traffic/sessions/"+id, nil), nil)
	if status != 404 {
		t.Errorf("re-delete: %d", status)
	}
}

// TestTrafficSessionsDeleteActiveConflict — DELETE on the active session → 409.
func TestTrafficSessionsDeleteActiveConflict(t *testing.T) {
	resetProfilerSingleton(t)
	base, _ := newTestServer(t, &fakeFacade{})

	body, _ := json.Marshal(trafficStartReq{Target: "/x"})
	var startResp struct {
		Session trafficSessionSummary `json:"session"`
	}
	doJSON(t, authedReq(t, "POST", base+"/traffic/start", body), &startResp)
	status, raw := doJSON(t, authedReq(t, "DELETE", base+"/traffic/sessions/"+startResp.Session.ID, nil), nil)
	if status != 409 {
		t.Errorf("delete active: want 409, got %d body=%s", status, raw)
	}
}

// TestTrafficLive_DefaultAndQuery — checks the ?last= parsing branches.
func TestTrafficLive_DefaultAndQuery(t *testing.T) {
	resetProfilerSingleton(t)
	base, _ := newTestServer(t, &fakeFacade{})

	cases := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{"default", "", 200},
		{"valid_30s", "?last=30s", 200},
		{"valid_5m", "?last=5m", 200},
		{"invalid", "?last=notaduration", 400},
		{"negative", "?last=-1s", 400},
		{"oversize_clamped", "?last=1h", 200}, // clamped to 10m, still 200
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := authedReq(t, "GET", base+"/traffic/live"+tc.query, nil)
			status, _ := doJSON(t, req, nil)
			if status != tc.wantStatus {
				t.Errorf("status: want %d, got %d", tc.wantStatus, status)
			}
		})
	}
}

// TestTrafficClear — POST /traffic/clear wipes completed sessions and
// reports the prior count.
func TestTrafficClear(t *testing.T) {
	resetProfilerSingleton(t)
	base, _ := newTestServer(t, &fakeFacade{})
	// Create two completed sessions back-to-back. NewSession ID is to the
	// second, so we need a brief sleep to avoid ID collision.
	for i := 0; i < 2; i++ {
		doJSON(t, authedReq(t, "POST", base+"/traffic/start", []byte(`{"target":"/x"}`)), nil)
		doJSON(t, authedReq(t, "POST", base+"/traffic/stop", nil), nil)
		time.Sleep(1100 * time.Millisecond)
	}
	var clr struct {
		Cleared int `json:"cleared"`
	}
	status, _ := doJSON(t, authedReq(t, "POST", base+"/traffic/clear", nil), &clr)
	if status != 200 {
		t.Fatalf("status: %d", status)
	}
	if clr.Cleared != 2 {
		t.Errorf("cleared: want 2, got %d", clr.Cleared)
	}
}

// TestTrafficVerboseGet — facade returns level, endpoint reports it.
func TestTrafficVerboseGet(t *testing.T) {
	cases := []struct {
		level string
		want  bool
	}{
		{"", false},
		{"warn", false},
		{"info", false},
		{"debug", true},
		{"trace", true},
	}
	for _, tc := range cases {
		t.Run(tc.level, func(t *testing.T) {
			base, _ := newTestServer(t, &fakeFacade{logLevel: tc.level, logLevelSet: tc.level != ""})
			var got trafficVerboseGetResp
			status, _ := doJSON(t, authedReq(t, "GET", base+"/traffic/verbose", nil), &got)
			if status != 200 {
				t.Fatalf("status: %d", status)
			}
			if got.Enabled != tc.want {
				t.Errorf("enabled: want %v, got %v", tc.want, got.Enabled)
			}
			if got.CurrentLevel != tc.level {
				t.Errorf("current_level: want %q, got %q", tc.level, got.CurrentLevel)
			}
		})
	}
}

// TestTrafficVerbosePost — POST flips the facade-tracked level + returns
// 202 with warning. We don't exercise the actual sing-box restart (that's
// the core helper's job).
func TestTrafficVerbosePost(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantLevel   string
		wantStatus  int
		wantWarning bool
	}{
		{"enable", `{"enabled":true}`, "debug", 202, true},
		{"disable", `{"enabled":false}`, "warn", 202, true},
		{"bad_json", `not json`, "", 400, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ff := &fakeFacade{}
			base, _ := newTestServer(t, ff)
			req := authedReq(t, "POST", base+"/traffic/verbose", []byte(tc.body))
			status, raw := doJSON(t, req, nil)
			if status != tc.wantStatus {
				t.Errorf("status: want %d, got %d body=%s", tc.wantStatus, status, raw)
			}
			if tc.wantLevel != "" && ff.appliedLevel != tc.wantLevel {
				t.Errorf("appliedLevel: want %q, got %q", tc.wantLevel, ff.appliedLevel)
			}
			if tc.wantWarning && !strings.Contains(string(raw), "warning") {
				t.Errorf("expected warning in body, got %s", raw)
			}
		})
	}
}

// TestTrafficProcesses — endpoint just walks SeenProcesses. Without any
// events the list is empty, but the contract is the {processes: []} shape.
func TestTrafficProcesses(t *testing.T) {
	resetProfilerSingleton(t)
	base, _ := newTestServer(t, &fakeFacade{})
	var got struct {
		Processes []trafficProcessRow `json:"processes"`
	}
	status, _ := doJSON(t, authedReq(t, "GET", base+"/traffic/processes", nil), &got)
	if status != 200 {
		t.Fatalf("status: %d", status)
	}
	if got.Processes == nil {
		t.Errorf("processes: want non-nil slice, got nil")
	}
}

// TestTrafficAuthGuard — all new endpoints reject missing/wrong bearer
// with 401. Localhost middleware already covers the IP guard.
func TestTrafficAuthGuard(t *testing.T) {
	base, _ := newTestServer(t, &fakeFacade{})
	paths := []struct {
		method, path string
	}{
		{"GET", "/traffic/status"},
		{"GET", "/traffic/live"},
		{"GET", "/traffic/sessions"},
		{"GET", "/traffic/sessions/anything"},
		{"GET", "/traffic/processes"},
		{"POST", "/traffic/start"},
		{"POST", "/traffic/stop"},
		{"POST", "/traffic/clear"},
		{"GET", "/traffic/verbose"},
		{"POST", "/traffic/verbose"},
	}
	for _, p := range paths {
		t.Run(p.method+p.path, func(t *testing.T) {
			req, _ := http.NewRequest(p.method, base+p.path, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != 401 {
				t.Errorf("status: want 401, got %d", resp.StatusCode)
			}
		})
	}
}
