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
	"singbox-launcher/internal/debuglog"
)

// DefaultPort matches LxBox spec 031 so scripts already written against
// mobile work unchanged when pointed at the desktop.
const DefaultPort = 9269

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

	// Actions — may be no-ops if the facade doesn't want to expose them.
	StartSingBox() error
	StopSingBox() error
	UpdateSubscriptions() error
	// PingAllProxies kicks the same ping-all flow as the Servers-tab
	// "test" button. Returns after the sweep completes (may be slow with
	// many nodes — callers should expect seconds).
	PingAllProxies() error
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

// routes wires the endpoints. auth middleware guards everything except /ping
// (which is still bound to 127.0.0.1 so it's not a real leak vector).
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	protected := http.NewServeMux()
	protected.HandleFunc("/version", s.handleVersion)
	protected.HandleFunc("/state", s.handleState)
	protected.HandleFunc("/proxies", s.handleProxies)
	protected.HandleFunc("/action/update-subs", s.handleUpdateSubs)
	protected.HandleFunc("/action/start", s.handleStart)
	protected.HandleFunc("/action/stop", s.handleStop)
	protected.HandleFunc("/action/ping-all", s.handlePingAll)

	mux.Handle("/", s.authMiddleware(protected))
	return mux
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
		"launcher":  s.facade.GetLauncherVersion(),
		"singbox":   s.facade.GetSingboxVersion(),
		"api":       "debugapi/v1",
	})
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{
		"running":                 s.facade.IsRunning(),
		"active_proxy":            s.facade.GetActiveProxyName(),
		"selected_group":          s.facade.GetSelectedClashGroup(),
		"singbox_version":         s.facade.GetSingboxVersion(),
		"subs_last_updated_unix":  unixOrNull(s.facade.GetLastUpdateSucceededAt()),
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
