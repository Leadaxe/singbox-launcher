package debugapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// SPEC 078: DocsURL pins the docs link to a clean release tag, else main.
func TestDocsURL(t *testing.T) {
	cases := map[string]string{
		"v1.1.5":             "https://github.com/Leadaxe/singbox-launcher/blob/v1.1.5/docs/API.md",
		"v10.20.30":          "https://github.com/Leadaxe/singbox-launcher/blob/v10.20.30/docs/API.md",
		"v1.1.5-12-g9a81f17": "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/API.md",
		"v1.1.5-dirty":       "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/API.md",
		"v-local-test":       "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/API.md",
		"":                   "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/API.md",
	}
	for in, want := range cases {
		if got := DocsURL(in); got != want {
			t.Errorf("DocsURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// ConnectionCardJSON must be human-readable: the auth scheme's "<token>" must
// stay literal, not HTML-escaped to "<token>".
func TestConnectionCardJSON_NoHTMLEscape(t *testing.T) {
	card, err := ConnectionCardJSON("http://127.0.0.1:9263", "abc", "v1.1.5", "1.14.0-lx.1-rc.17")
	if err != nil {
		t.Fatalf("card: %v", err)
	}
	if strings.Contains(card, "\\u003c") || strings.Contains(card, "\\u003e") {
		t.Errorf("card must not HTML-escape angle brackets to \\u003c/\\u003e:\n%s", card)
	}
	if !strings.Contains(card, "Authorization: Bearer <token>") {
		t.Errorf("card auth scheme should read literally:\n%s", card)
	}
	if !strings.Contains(card, `"token": "abc"`) || !strings.Contains(card, `"base_url": "http://127.0.0.1:9263"`) {
		t.Errorf("card missing token/base_url:\n%s", card)
	}
}

// Every endpoint advertised by the registry must actually be reachable (not
// 404), and conversely the manifest/help must list what the server serves —
// the single-source-of-truth guarantee.
func TestManifestAndHelp(t *testing.T) {
	port := freeLocalPort(t)
	ff := &fakeFacade{version: "1.14.0-lx.1-rc.17"}
	s, err := New(ff, port, "tok")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Start()
	defer s.Stop()
	base := "http://127.0.0.1:" + itoa(port)

	get := func(path string) (*http.Response, []byte) {
		req, _ := http.NewRequest("GET", base+path, nil)
		req.Header.Set("Authorization", "Bearer tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp, body
	}

	// GET / manifest
	resp, body := get("/")
	if resp.StatusCode != 200 {
		t.Fatalf("GET /: status %d", resp.StatusCode)
	}
	var manifest struct {
		API       string         `json:"api"`
		Spec      string         `json:"spec"`
		Launcher  string         `json:"launcher"`
		Core      string         `json:"core"`
		Docs      string         `json:"docs"`
		Hint      string         `json:"hint"`
		Endpoints []endpointView `json:"endpoints"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatalf("manifest unmarshal: %v\n%s", err, body)
	}
	if manifest.API != APIDisplayName || manifest.Spec != APISpec {
		t.Errorf("manifest identity wrong: %+v", manifest)
	}
	if manifest.Core != "1.14.0-lx.1-rc.17" || manifest.Launcher != "v-test" {
		t.Errorf("manifest versions wrong: launcher=%q core=%q", manifest.Launcher, manifest.Core)
	}
	if !strings.Contains(manifest.Docs, "/docs/API.md") {
		t.Errorf("manifest docs link wrong: %q", manifest.Docs)
	}
	if len(manifest.Endpoints) == 0 {
		t.Fatal("manifest has no endpoints")
	}

	// GET /help → endpoint list, must match the registry.
	resp, body = get("/help")
	if resp.StatusCode != 200 {
		t.Fatalf("GET /help: status %d", resp.StatusCode)
	}
	var help struct {
		Endpoints []endpointView `json:"endpoints"`
	}
	if err := json.Unmarshal(body, &help); err != nil {
		t.Fatalf("help unmarshal: %v", err)
	}
	if len(help.Endpoints) != len(s.endpoints()) {
		t.Errorf("help lists %d endpoints, registry has %d", len(help.Endpoints), len(s.endpoints()))
	}

	// Single-source-of-truth: every advertised endpoint is actually wired
	// (a GET returns something other than 404). POST-only paths are probed
	// with their method; we only assert "not 404".
	for _, e := range help.Endpoints {
		if strings.HasSuffix(e.Path, "/") {
			continue // prefix routes (e.g. /traffic/sessions/) need an ID
		}
		method := "GET"
		if strings.HasPrefix(e.Method, "POST") {
			method = "POST"
		}
		req, _ := http.NewRequest(method, base+e.Path, nil)
		req.Header.Set("Authorization", "Bearer tok")
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("probe %s %s: %v", method, e.Path, err)
		}
		_ = r.Body.Close()
		if r.StatusCode == http.StatusNotFound {
			t.Errorf("advertised endpoint %s %s is not wired (404)", e.Method, e.Path)
		}
	}
}

// Manifest and help require auth; an unknown path returns 404 with a docs pointer.
func TestManifestAuthAndUnknownPath(t *testing.T) {
	port := freeLocalPort(t)
	s, err := New(&fakeFacade{}, port, "tok")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Start()
	defer s.Stop()
	base := "http://127.0.0.1:" + itoa(port)

	// no auth → 401 on both / and /help
	for _, p := range []string{"/", "/help"} {
		resp, err := http.Get(base + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Errorf("GET %s without auth: status %d, want 401", p, resp.StatusCode)
		}
	}

	// authed unknown path → 404 with docs hint
	req, _ := http.NewRequest("GET", base+"/does-not-exist", nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unknown: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("unknown path: status %d, want 404", resp.StatusCode)
	}
	if !strings.Contains(string(body), "/docs/API.md") {
		t.Errorf("404 body should point at docs: %s", body)
	}
}
