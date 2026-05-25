// Package debugapi — SPEC 059 Traffic Profiler endpoints.
//
// These handlers wrap the singleton TrafficProfiler in internal/traffic so
// external automation (CI, MCP agents, regression fixtures, smoke tests)
// can drive a recording session without UI interaction.
//
// All endpoints are guarded by the existing bearer-auth middleware and
// bound to 127.0.0.1 (same posture as the rest of the debug API).
//
// Endpoint summary (full schema in the SPEC 059 user guide):
//
//	GET    /traffic/status               — recording state + last counters
//	GET    /traffic/live?last=60s        — rolling-buffer snapshot
//	GET    /traffic/sessions             — completed + active session list
//	GET    /traffic/sessions/{id}        — full session export (events incl.)
//	DELETE /traffic/sessions/{id}        — drop one completed session
//	GET    /traffic/processes            — processes seen in rolling buffer
//	POST   /traffic/start                — body {target, verbose?}
//	POST   /traffic/stop                 — finalize active session
//	POST   /traffic/clear                — wipe all completed sessions
//	GET    /traffic/verbose              — current log level
//	POST   /traffic/verbose              — body {enabled} → 202 with warning
//
// Verbose toggle proxies through the facade (LoadState / SaveState /
// ApplyLogLevelAndReload) — we extracted those helpers into core/log_level.go
// so this package doesn't need to import ui/. See the commit message for
// the rationale on choosing the "move into core" approach over a function
// pointer hook.
package debugapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	tprof "singbox-launcher/internal/traffic"
)

const (
	// trafficLiveDefaultWindow — fallback for /traffic/live?last= when the
	// query param is missing or empty. Matches the in-app Live view's
	// initial scrollback (Snapshot(60s)).
	trafficLiveDefaultWindow = 60 * time.Second
	// trafficLiveMaxWindow — hard cap. The rolling buffer itself only
	// retains 60s in steady state, but tests / unusual workloads can
	// occasionally exceed it; we clamp at 10min to keep responses small
	// and the JSON encode bounded.
	trafficLiveMaxWindow = 10 * time.Minute
)

// trafficStatusResp — GET /traffic/status response shape.
type trafficStatusResp struct {
	Recording     bool      `json:"recording"`
	Target        string    `json:"target,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	DurationSec   int       `json:"duration_s"`
	Events        int       `json:"events"`
	EventsDropped int       `json:"events_dropped"`
	WasVerbose    bool      `json:"was_verbose"`
}

func (s *Server) handleTrafficStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	p := tprof.GetInstance()
	sess := p.ActiveSession()
	resp := trafficStatusResp{Recording: sess != nil}
	if sess != nil {
		resp.Target = sess.TargetProcess
		resp.StartedAt = sess.StartedAt
		resp.DurationSec = int(sess.Duration().Seconds())
		resp.Events = len(sess.Events())
		resp.EventsDropped = sess.EventsDropped()
		resp.WasVerbose = sess.WasVerbose
	}
	writeJSON(w, http.StatusOK, resp)
}

// trafficLiveResp — GET /traffic/live response shape.
type trafficLiveResp struct {
	Events   []tprof.TrafficEvent `json:"events"`
	CutoffTS time.Time            `json:"cutoff_ts"`
}

func (s *Server) handleTrafficLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	d := trafficLiveDefaultWindow
	if raw := strings.TrimSpace(r.URL.Query().Get("last")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid last=: " + err.Error()})
			return
		}
		if parsed <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "last must be > 0"})
			return
		}
		d = parsed
	}
	if d > trafficLiveMaxWindow {
		d = trafficLiveMaxWindow
	}
	p := tprof.GetInstance()
	cutoff := time.Now().Add(-d)
	writeJSON(w, http.StatusOK, trafficLiveResp{
		Events:   p.Snapshot(d),
		CutoffTS: cutoff,
	})
}

// trafficSessionSummary — one row of the /traffic/sessions list.
type trafficSessionSummary struct {
	ID         string     `json:"id"`
	Target     string     `json:"target"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Events     int        `json:"events"`
	WasVerbose bool       `json:"was_verbose"`
	Active     bool       `json:"active,omitempty"`
}

func (s *Server) handleTrafficSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	p := tprof.GetInstance()
	completed := p.CompletedSessions()
	out := make([]trafficSessionSummary, 0, len(completed)+1)
	for _, sess := range completed {
		out = append(out, summaryFromSession(sess, false))
	}
	if active := p.ActiveSession(); active != nil {
		out = append(out, summaryFromSession(active, true))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func summaryFromSession(sess *tprof.Session, active bool) trafficSessionSummary {
	return trafficSessionSummary{
		ID:         sess.ID,
		Target:     sess.TargetProcess,
		StartedAt:  sess.StartedAt,
		FinishedAt: sess.FinishedAt,
		Events:     len(sess.Events()),
		WasVerbose: sess.WasVerbose,
		Active:     active,
	}
}

// trafficSessionExport — full session payload for GET /traffic/sessions/{id}.
// Matches the `sessionExport` struct in ui/traffic/toolbar.go so users get
// the same shape whether they hit the UI's Copy/Export button or the API.
type trafficSessionExport struct {
	ID         string               `json:"id"`
	Target     string               `json:"target_process"`
	StartedAt  time.Time            `json:"started_at"`
	FinishedAt *time.Time           `json:"finished_at,omitempty"`
	WasVerbose bool                 `json:"was_verbose"`
	Events     []tprof.TrafficEvent `json:"events"`
}

func (s *Server) handleTrafficSessionByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/traffic/sessions/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "session id required"})
		return
	}
	p := tprof.GetInstance()
	switch r.Method {
	case http.MethodGet:
		sess := findSessionByID(p, id)
		if sess == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "session not found", "id": id})
			return
		}
		writeJSON(w, http.StatusOK, trafficSessionExport{
			ID:         sess.ID,
			Target:     sess.TargetProcess,
			StartedAt:  sess.StartedAt,
			FinishedAt: sess.FinishedAt,
			WasVerbose: sess.WasVerbose,
			Events:     sess.Events(),
		})
	case http.MethodDelete:
		// Don't allow deleting the active session — caller must Stop first.
		if active := p.ActiveSession(); active != nil && active.ID == id {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "cannot delete active session — POST /traffic/stop first"})
			return
		}
		// DeleteSession is a no-op for unknown ids; detect missing by
		// snapshotting count before/after.
		before := len(p.CompletedSessions())
		p.DeleteSession(id)
		after := len(p.CompletedSessions())
		if before == after {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "session not found", "id": id})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET or DELETE required"})
	}
}

func findSessionByID(p *tprof.TrafficProfiler, id string) *tprof.Session {
	if active := p.ActiveSession(); active != nil && active.ID == id {
		return active
	}
	for _, sess := range p.CompletedSessions() {
		if sess.ID == id {
			return sess
		}
	}
	return nil
}

// trafficProcessRow — one row of GET /traffic/processes. Derived from
// the profiler's SeenProcesses() helper (rolling-buffer scan).
type trafficProcessRow struct {
	Path        string    `json:"path"`
	DisplayName string    `json:"display_name,omitempty"`
	Events      int       `json:"events"`
	LastSeen    time.Time `json:"last_seen"`
}

func (s *Server) handleTrafficProcesses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	p := tprof.GetInstance()
	seen := p.SeenProcesses()
	out := make([]trafficProcessRow, 0, len(seen))
	for _, ps := range seen {
		out = append(out, trafficProcessRow{
			Path:        ps.Path,
			DisplayName: ps.DisplayName,
			Events:      ps.Events,
			LastSeen:    ps.LastSeen,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"processes": out})
}

// trafficStartReq — POST /traffic/start request body.
type trafficStartReq struct {
	Target  string `json:"target"`
	Verbose bool   `json:"verbose,omitempty"`
}

func (s *Server) handleTrafficStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var req trafficStartReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
		return
	}
	req.Target = strings.TrimSpace(req.Target)
	// target="" is supported by the profiler (system-wide capture) but
	// rarely useful through the API; we still accept it.
	p := tprof.GetInstance()
	sess, err := p.StartSession(req.Target, req.Verbose)
	if err != nil {
		if errors.Is(err, tprof.ErrSessionAlreadyActive) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": summaryFromSession(sess, true)})
}

func (s *Server) handleTrafficStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	p := tprof.GetInstance()
	sess, err := p.StopSession()
	if err != nil {
		// Profiler.StopSession returns a plain error "no active session"
		// — map to 404 since the resource (active recording) doesn't exist.
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": summaryFromSession(sess, false)})
}

func (s *Server) handleTrafficClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	p := tprof.GetInstance()
	before := len(p.CompletedSessions())
	p.ClearAll()
	writeJSON(w, http.StatusOK, map[string]any{"cleared": before})
}

// trafficVerboseGetResp / trafficVerboseSetReq — GET / POST /traffic/verbose.
type trafficVerboseGetResp struct {
	Enabled      bool   `json:"enabled"`
	CurrentLevel string `json:"current_level"`
}

type trafficVerboseSetReq struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleTrafficVerbose(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		level, _, err := s.facade.ReadCurrentLogLevel()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, trafficVerboseGetResp{
			Enabled:      level == "debug" || level == "trace",
			CurrentLevel: level,
		})
	case http.MethodPost:
		var req trafficVerboseSetReq
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		target := "warn"
		if req.Enabled {
			target = "debug"
		}
		if err := s.facade.ApplyLogLevelAndReload(target); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		// 202 because the restart is sync-applied here but the actual log
		// emission switch happens once the sing-box monitor re-launches
		// the process. Callers polling /traffic/status will see the change
		// reflect within a few hundred ms.
		writeJSON(w, http.StatusAccepted, map[string]any{
			"ok":      true,
			"level":   target,
			"warning": "active connections reset",
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET or POST required"})
	}
}

// decodeJSONBody — small helper that tolerates empty bodies (treats as {})
// and bounds request size to 1 MiB so a runaway script can't OOM the
// launcher.
func decodeJSONBody(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		// Empty body is acceptable for endpoints with no required fields.
		if errors.Is(err, errEmptyBody{}) || strings.Contains(err.Error(), "EOF") {
			return nil
		}
		return err
	}
	return nil
}

// errEmptyBody — placeholder for the io.EOF check above (kept as a typed
// sentinel so a future refactor can switch on it).
type errEmptyBody struct{}

func (errEmptyBody) Error() string { return "empty body" }
