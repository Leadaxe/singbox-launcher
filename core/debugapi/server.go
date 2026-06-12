// Package debugapi exposes a small, localhost-only HTTP surface for tools
// (scripts, automation, other GUIs) to introspect and nudge the launcher.
// Modeled on LxBox's spec 031 Debug API but trimmed to the most essential
// endpoints — we intentionally omit the CRUD surface for rules/subs/settings
// since the desktop already has a full wizard for those and the extra
// surface is disproportionate to the use case.
//
// Safety posture:
//   - Bind strictly to 127.0.0.1. No 0.0.0.0 / no LAN. Users who want
//     remote access must adb-forward or ssh-tunnel.
//   - Off by default. User explicitly enables in Diagnostics tab. First
//     enable generates a random bearer token; it's shown in the UI with
//     a Copy button and persisted in bin/settings.json.
//   - All endpoints require "Authorization: Bearer <token>". No CORS.
//   - Action endpoints (state-mutating) are POST only.
package debugapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"singbox-launcher/api"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
)

// DefaultPort — desktop debug-API default. Mobile LxBox uses 9269; we
// chose a separate port so a single host can run both in parallel
// without `lsof: address in use`.
const DefaultPort = 9263

// ControllerFacade is the narrow surface the debug-api needs from the
// singleton AppController. Declared here (not in core) to keep debugapi
// import-free of the full controller — no cycles, easier to test.
type ControllerFacade interface {
	IsRunning() bool
	GetProxiesList() []api.ProxyInfo
	GetActiveProxyName() string
	GetSelectedClashGroup() string
	GetSingboxVersion() string
	GetConfigPath() string
	GetLastUpdateSucceededAt() time.Time
	GetLauncherVersion() string
	// GetExecDir — used by /debug/snapshot to resolve canonical wizard
	// file paths via internal/platform helpers (SUB_SPEC_SNAPSHOT.md §2.2).
	GetExecDir() string

	// Actions — may be no-ops if the facade doesn't want to expose them.
	StartSingBox() error
	StopSingBox() error
	UpdateSubscriptions() error
	// PingAllProxies kicks the same ping-all flow as the Servers-tab
	// "test" button. Returns after the sweep completes (may be slow with
	// many nodes — callers should expect seconds).
	PingAllProxies() error
	// RebuildConfigIfDirty — re-emit config.json from current state +
	// outbounds cache + template, without restarting sing-box (SPEC 045
	// invariant: same call as the pre-Start hook in ProcessService).
	// No-op if dirty markers are clean. Useful for scripts/agents that
	// want config to reflect state changes without disturbing the running
	// sing-box process. The file write is atomic; on error nothing is
	// committed.
	RebuildConfigIfDirty() error

	// State / config surface (SPEC 053/056/057/058 endpoints).
	//
	// LoadState reads state.json from canonical path; SaveState writes it
	// atomically (.tmp + Rename, fsync) — matches StateService semantics.
	// LogLevel helpers proxy to core.{Read,Apply}LogLevel* — used by
	// /traffic/verbose. Template loader exposes preset bundles so
	// /state/outbounds/resolved can render merged bodies.
	LoadState() (*state.State, error)
	SaveState(*state.State) error
	LoadTemplate() (*template.TemplateData, error)
	ApplyLogLevelAndReload(level string) error
	ReadCurrentLogLevel() (string, bool, error)
}

// Server owns the listener, shutdown context, and auth config.
type Server struct {
	mu       sync.Mutex
	listener net.Listener
	httpSrv  *http.Server
	token    string
	facade   ControllerFacade
}

// New constructs a Server bound to 127.0.0.1:port.
// token must be non-empty; callers generate/persist it.
func New(facade ControllerFacade, port int, token string) (*Server, error) {
	if facade == nil {
		return nil, errors.New("debugapi: nil facade")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("debugapi: empty token")
	}
	if port <= 0 {
		port = DefaultPort
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("debugapi: listen on %s: %w", addr, err)
	}

	s := &Server{
		listener: ln,
		token:    token,
		facade:   facade,
	}
	s.httpSrv = &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s, nil
}

// Start runs the HTTP server in a goroutine. Returns immediately.
func (s *Server) Start() {
	go func() {
		if err := s.httpSrv.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			debuglog.WarnLog("debugapi: Serve: %v", err)
		}
	}()
	debuglog.InfoLog("debugapi: listening on %s", s.listener.Addr())
}

// Stop gracefully shuts the server down (5s deadline).
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpSrv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.httpSrv.Shutdown(ctx)
	s.httpSrv = nil
	debuglog.InfoLog("debugapi: stopped")
}

// GenerateToken returns a random URL-safe token suitable for Bearer auth.
// 32 bytes of entropy, base64-std-no-padding.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("debugapi: rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// endpoints is the single source of truth for the debug-API surface (SPEC 078):
// routes() registers handlers from it, and GET / + GET /help document it. A path
// can't be documented without being wired, or wired without showing up in /help.
//
// "/" (the manifest) and "/help" are intentionally NOT in this list — they
// describe the list and would be self-referential; routes() wires them directly.
func (s *Server) endpoints() []apiEndpoint {
	return []apiEndpoint{
		{"GET", "/ping", false, "Liveness probe (no auth)", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		}},
		{"GET", "/version", true, "Launcher, core and API-spec versions", s.handleVersion},
		{"GET", "/state", true, "Core run state + active proxy/group", s.handleState},
		{"GET", "/proxies", true, "Proxy list with latencies", s.handleProxies},
		{"GET", "/debug/snapshot", true, "Diagnostic snapshot (state/config/template)", s.handleSnapshot},
		{"POST", "/action/update-subs", true, "Re-fetch subscriptions and rebuild config", s.handleUpdateSubs},
		{"POST", "/action/start", true, "Start the core", s.handleStart},
		{"POST", "/action/stop", true, "Stop the core", s.handleStop},
		{"POST", "/action/ping-all", true, "Latency-test all proxies", s.handlePingAll},
		{"POST", "/action/rebuild-config", true, "Rebuild config.json from state", s.handleRebuildConfig},

		// SPEC 053/056/057/058: structured state read + targeted mutations.
		{"GET", "/state/full", true, "Full wizard state JSON", s.handleStateFull},
		{"GET", "/state/rules", true, "Routing rules", s.handleStateRules},
		{"GET", "/state/dns", true, "DNS settings", s.handleStateDNS},
		{"GET", "/state/dns/rules", true, "DNS rules", s.handleStateDNSRules},
		{"GET", "/state/outbounds/resolved", true, "Resolved outbound tags", s.handleStateOutboundsResolved},

		// bin/settings.json — launcher-level preferences (subscription UA, etc).
		{"GET/PUT", "/settings/user-agent", true, "Get/set subscription User-Agent", s.handleSettingsUserAgent},

		// SPEC 059: Traffic Profiler.
		{"GET", "/traffic/status", true, "Traffic profiler status", s.handleTrafficStatus},
		{"GET", "/traffic/live", true, "Live traffic counters", s.handleTrafficLive},
		{"GET", "/traffic/sessions", true, "Captured sessions", s.handleTrafficSessions},
		{"GET", "/traffic/sessions/", true, "Session by ID (path suffix)", s.handleTrafficSessionByID},
		{"GET", "/traffic/processes", true, "Per-process traffic", s.handleTrafficProcesses},
		{"POST", "/traffic/start", true, "Start traffic capture", s.handleTrafficStart},
		{"POST", "/traffic/stop", true, "Stop traffic capture", s.handleTrafficStop},
		{"POST", "/traffic/clear", true, "Clear captured traffic", s.handleTrafficClear},
		{"POST", "/traffic/verbose", true, "Toggle verbose capture", s.handleTrafficVerbose},
	}
}

// routes wires the endpoints. auth middleware guards everything except /ping
// (which is still bound to 127.0.0.1 so it's not a real leak vector). The
// manifest (/) and /help discovery endpoints are auth-guarded too.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	protected := http.NewServeMux()

	for _, e := range s.endpoints() {
		if e.Auth {
			protected.HandleFunc(e.Path, e.handler)
		} else {
			mux.HandleFunc(e.Path, e.handler)
		}
	}

	// SPEC 078: self-description. "/" → manifest, "/help" → endpoint list.
	// "/" is an exact-match catch-all in protected (ServeMux routes any unknown
	// path to "/"); handleManifest distinguishes the real root from 404s.
	protected.HandleFunc("/help", s.handleHelp)
	protected.HandleFunc("/", s.handleManifest)

	mux.Handle("/", s.authMiddleware(protected))
	return mux
}

// handleManifest serves the GET / self-description manifest (SPEC 078): API
// identity, versions, auth scheme, a version-pinned docs link, and the endpoint
// list. Because "/" is the ServeMux catch-all, any unknown path lands here too —
// those get a 404 with the same docs pointer so an agent isn't left guessing.
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	launcher := s.facade.GetLauncherVersion()
	if r.URL.Path != "/" {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": "unknown endpoint",
			"path":  r.URL.Path,
			"docs":  DocsURL(launcher),
			"hint":  "GET / for the manifest, GET /help for the endpoint list.",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"api":       APIDisplayName,
		"spec":      APISpec,
		"launcher":  launcher,
		"core":      s.facade.GetSingboxVersion(),
		"auth":      APIAuthScheme,
		"docs":      DocsURL(launcher),
		"hint":      APIHint,
		"endpoints": endpointViews(s.endpoints()),
	})
}

// handleHelp serves GET /help — just the endpoint list, for an agent to
// discover the surface and take it from there.
func (s *Server) handleHelp(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoints": endpointViews(s.endpoints()),
	})
}

// authMiddleware requires "Authorization: Bearer <token>" on every protected
// route. 401 with a JSON body, not HTML — this API is for machine callers.
// Comparison is constant-time so an attacker on the same host can't learn
// token bytes by timing the 401 response. On a real loopback interface this
// leak is theoretical, but ConstantTimeCompare costs nothing and removes the
// class of bug outright.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(h, prefix) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		got := strings.TrimSpace(h[len(prefix):])
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"launcher": s.facade.GetLauncherVersion(),
		"singbox":  s.facade.GetSingboxVersion(),
		"api":      "debugapi/v1",
	})
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{
		"running":                s.facade.IsRunning(),
		"active_proxy":           s.facade.GetActiveProxyName(),
		"selected_group":         s.facade.GetSelectedClashGroup(),
		"singbox_version":        s.facade.GetSingboxVersion(),
		"subs_last_updated_unix": unixOrNull(s.facade.GetLastUpdateSucceededAt()),
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleProxies(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.facade.GetProxiesList())
}

func (s *Server) handleUpdateSubs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	if err := s.facade.UpdateSubscriptions(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	if err := s.facade.StartSingBox(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleRebuildConfig — POST /action/rebuild-config
//
// Дёргает RebuildConfigIfDirty в обход restart sing-box: пересобирает
// config.json из current state + outbounds cache + template, если
// CacheStale или ConfigStale взведён. Если оба чистые — no-op (200 OK,
// `{"ok":true,"rebuilt":false}`); иначе пересборка + 200 OK с
// `{"ok":true,"rebuilt":true}`. Атомарная запись через .tmp + Rename.
//
// Useful for scripts/agents которые хотят увидеть новый config.json на
// диске, не дёргая running sing-box. После rebuild маркеры дырти
// сбрасываются (как и в обычном pre-Start path), чтобы UI знал что
// state и config теперь согласованы.
func (s *Server) handleRebuildConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	if err := s.facade.RebuildConfigIfDirty(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePingAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	if err := s.facade.PingAllProxies(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	if err := s.facade.StopSingBox(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Addr returns the literal "127.0.0.1:N" the server is bound to — useful
// for building a pastable example URL in the UI.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func unixOrNull(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Unix()
}
