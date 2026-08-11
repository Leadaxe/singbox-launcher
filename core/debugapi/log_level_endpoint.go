// Package debugapi — sing-box log.level as a first-class endpoint.
//
// Until now the only way to move the log level over HTTP was POST
// /traffic/verbose, which is a boolean: it can reach "debug" and "warn"
// and nothing else. Reaching "trace" for a protocol-level capture, or
// dropping to "error" to quieten a noisy box, meant editing state.json by
// hand. This endpoint exposes the underlying var directly.
//
// Endpoints:
//
//	GET   /state/log-level  → {level, is_set, default, effective, allowed}
//	PATCH /state/log-level  → body {level}; restarts the core
//
// The write path is core.ApplyLogLevelAndReloadCore — the same helper
// /traffic/verbose uses — so the semantics are identical: mutate
// state.vars[log_level] → save → forced config rebuild → restart sing-box
// if it is running. That restart RESETS active connections, hence 202
// Accepted plus an explicit warning field rather than a bare 200.
//
// Validation lives here rather than in core: ApplyLogLevelAndReloadCore
// writes whatever string it is handed, and its only other caller
// (/traffic/verbose) passes hardcoded "warn"/"debug", so an invalid level
// could never reach state.json before. This endpoint accepts a caller
// string, so it must reject anything sing-box would choke on — a bad
// level lands in config.log.level and fails the next core start.
package debugapi

import (
	"net/http"
	"strings"
)

// logLevelDefault — what sing-box uses when vars[log_level] is unset. Kept
// in sync with the "log_level" var's default_value in wizard_template.json.
const logLevelDefault = "warn"

// logLevels — the accepted values, mirroring the "log_level" enum options
// in wizard_template.json (which in turn mirror sing-box's log.level).
// Order is loudest → quietest; it is the order reported in "allowed".
var logLevels = []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}

func isValidLogLevel(level string) bool {
	for _, l := range logLevels {
		if l == level {
			return true
		}
	}
	return false
}

// handleStateLogLevel — GET/PATCH /state/log-level.
//
// GET response shape:
//
//	{
//	  "level":     "debug",   // raw stored value ("" when unset)
//	  "is_set":    true,      // whether vars[log_level] exists at all
//	  "default":   "warn",    // template default, used when unset
//	  "effective": "debug",   // what the next core start will actually use
//	  "allowed":   [...]      // valid values for PATCH
//	}
//
// PATCH body:
//
//	{ "level": "trace" }
//
// The field is required: an empty or missing level is rejected rather than
// silently resetting to default, so a truncated request can't quietly
// change logging behaviour. Callers wanting the default set it explicitly.
func (s *Server) handleStateLogLevel(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		level, isSet, err := s.facade.ReadCurrentLogLevel()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		effective := level
		if effective == "" {
			effective = logLevelDefault
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"level":     level,
			"is_set":    isSet,
			"default":   logLevelDefault,
			"effective": effective,
			"allowed":   logLevels,
		})

	case http.MethodPatch:
		// *string so "field omitted" is distinguishable from an explicit
		// empty value — both are errors here, but they get different
		// messages, which matters when debugging a client by hand.
		var req struct {
			Level *string `json:"level"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		if req.Level == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":   "missing 'level' field",
				"allowed": logLevels,
			})
			return
		}
		level := strings.TrimSpace(*req.Level)
		if !isValidLogLevel(level) {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":   "invalid log level: " + level,
				"allowed": logLevels,
			})
			return
		}
		if err := s.facade.ApplyLogLevelAndReload(level); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		// 202, matching /traffic/verbose: state.json and config.json are
		// written synchronously, but the level only takes hold once the
		// monitor brings sing-box back up.
		writeJSON(w, http.StatusAccepted, map[string]any{
			"ok":      true,
			"level":   level,
			"warning": "active connections reset",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET or PATCH required"})
	}
}
