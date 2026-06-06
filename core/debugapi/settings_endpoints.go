// Package debugapi — settings-level launcher knobs exposed via HTTP.
//
// Distinct from state_endpoints.go (which surfaces SPEC 053/056/057/058
// state.json sections): this file deals with bin/settings.json — UI-level
// launcher preferences such as the subscription User-Agent override, HWID
// identification toggles, etc.
//
// Settings live in `internal/locale` (legacy package name from when it
// only held translations) and are loaded/saved via locale.LoadSettings /
// locale.SaveSettings. The path is `<execDir>/bin/settings.json`,
// resolved through ControllerFacade.GetExecDir + platform.GetBinDir.
//
// Endpoints:
//
//	GET   /settings/user-agent  → {user_agent, default, effective}
//	PATCH /settings/user-agent  → body {user_agent}; "" resets to default
//
// All mutations are write-then-respond — no sing-box restart needed
// because the fetcher reads settings lazily via
// subscription.LoadSubscriptionSettingsFunc on every request. Next
// subscription fetch picks up the new UA automatically.
package debugapi

import (
	"net/http"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// handleSettingsUserAgent — GET/PATCH /settings/user-agent.
//
// GET response shape:
//
//	{
//	  "user_agent": "v2rayN/7.5.0",                         // raw stored value (may be empty)
//	  "default":    "singbox-launcher/0.9.9 (macOS arm64)",   // what gets sent when user_agent is empty
//	  "effective":  "v2rayN/7.5.0"                          // what the next fetch will actually send
//	}
//
// PATCH body:
//
//	{ "user_agent": "v2rayN/7.5.0" }   // set custom
//	{ "user_agent": "" }               // reset to default
//
// Empty body / missing field is rejected — explicit "" is needed to reset,
// so PATCH stays idempotent and protects against accidental wipes from
// truncated requests.
func (s *Server) handleSettingsUserAgent(w http.ResponseWriter, r *http.Request) {
	binDir := platform.GetBinDir(s.facade.GetExecDir())

	switch r.Method {
	case http.MethodGet:
		st := locale.LoadSettings(binDir)
		defaultUA := configtypes.BuildSubscriptionUserAgent()
		effective := strings.TrimSpace(st.SubscriptionUserAgent)
		if effective == "" {
			effective = defaultUA
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"user_agent": st.SubscriptionUserAgent,
			"default":    defaultUA,
			"effective":  effective,
		})

	case http.MethodPatch:
		// Use a *string so we can distinguish "field omitted" from
		// "field present with empty value (= reset)". json.Unmarshal
		// leaves a nil *string for missing fields; "" → &"".
		var req struct {
			UserAgent *string `json:"user_agent"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		if req.UserAgent == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "missing 'user_agent' field (use \"\" to reset to default)",
			})
			return
		}
		cur := locale.LoadSettings(binDir)
		cur.SubscriptionUserAgent = strings.TrimSpace(*req.UserAgent)
		if err := locale.SaveSettings(binDir, cur); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		defaultUA := configtypes.BuildSubscriptionUserAgent()
		effective := cur.SubscriptionUserAgent
		if effective == "" {
			effective = defaultUA
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"user_agent": cur.SubscriptionUserAgent,
			"default":    defaultUA,
			"effective":  effective,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET or PATCH required"})
	}
}
