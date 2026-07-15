// Package debugapi — SPEC 053/056/057/058 state endpoints.
//
// These expose the post-SPEC-060 State surfaces that landed after the
// original SPEC 050 debug-API contract was drafted. The intent is parity
// with the in-process StateService: external callers (CI scripts, MCP
// agents, regression fixtures) can read structured slices of state and
// patch the new sections atomically.
//
//	GET   /state/full                  — full marshalled State as JSON
//	GET   /state/rules                 — SPEC 053 rules[] only
//	PATCH /state/rules                 — body {mode: replace|append, rules}
//	GET   /state/dns                   — SPEC 056 dns_options section
//	PATCH /state/dns                   — replace whole dns_options
//	GET   /state/dns/rules             — wizard-text view of USER rules only
//	PATCH /state/dns/rules             — body {text} → replace USER rules
//	GET   /state/outbounds/resolved    — SPEC 057+058 merged outbounds view
//
// All mutations follow the SPEC 050 contract: validate → Save → respond.
// We do NOT touch config.json — callers wanting the rebuilt file should
// follow up with POST /action/rebuild-config (existing endpoint).
package debugapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// handleStateFull — GET /state/full. Returns the full in-memory State as
// JSON. We marshal via State directly so the response includes ALL
// post-SPEC-060 fields (Rules, DNS, Connections.Outbounds with Ref/Updates).
func (s *Server) handleStateFull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	st, err := s.facade.LoadState()
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "state.json not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// patchRulesReq — body for PATCH /state/rules.
//
// mode=replace overwrites Rules[] wholesale; mode=append concatenates.
// We intentionally don't support per-rule indexing — callers wanting
// fine-grained edits should read /state/rules, mutate locally, then
// PATCH replace. Keeps the mutation surface narrow and easy to reason
// about; mirrors SPEC 050's mode="replace|append" pattern for
// /state/custom_rules.
type patchRulesReq struct {
	Mode  string       `json:"mode"`
	Rules []state.Rule `json:"rules"`
}

func (s *Server) handleStateRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		st, err := s.facade.LoadState()
		if err != nil {
			writeJSON(w, stateErrStatus(err), map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rules": st.Rules})

	case http.MethodPatch:
		var req patchRulesReq
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		if req.Mode != "replace" && req.Mode != "append" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "mode must be 'replace' or 'append'"})
			return
		}
		// Per-rule validation: kind discriminator + body decode round-trip.
		// Bad shape → 422 (SPEC 050 semantic-error code).
		for i := range req.Rules {
			if _, err := (&req.Rules[i]).DecodeBody(); err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error": fmt.Sprintf("rules[%d]: %s", i, err.Error()),
					"field": fmt.Sprintf("rules[%d]", i),
				})
				return
			}
		}
		s.stateMu.Lock()
		defer s.stateMu.Unlock()
		st, err := s.facade.LoadState()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "load state: " + err.Error()})
			return
		}
		before := len(st.Rules)
		switch req.Mode {
		case "replace":
			st.Rules = req.Rules
		case "append":
			st.Rules = append(st.Rules, req.Rules...)
		}
		if err := s.facade.SaveState(st); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "save state: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":           true,
			"diff_summary": []string{fmt.Sprintf("rules: %s, %d → %d entries", req.Mode, before, len(st.Rules))},
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET or PATCH required"})
	}
}

// handleStateDNS — GET /state/dns / PATCH /state/dns.
//
// PATCH replaces the entire dns_options section (SPEC 056 flat shape).
// We don't merge — mirrors PUT /state/dns/servers semantics from SPEC 050.
// Callers wanting field-level edits should GET → mutate → PATCH.
func (s *Server) handleStateDNS(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		st, err := s.facade.LoadState()
		if err != nil {
			writeJSON(w, stateErrStatus(err), map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, st.DNS)

	case http.MethodPatch:
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read body: " + err.Error()})
			return
		}
		// Guard against silent wipe: PATCH replaces the WHOLE dns_options, so a
		// bare `{}` (or a truncated request) would clear every DNS server/rule
		// and still return 200. Require the body to actually carry servers
		// and/or rules; a keyless object → 422, state untouched. (DNSOptions has
		// a custom Unmarshal, so probe the raw keys rather than trusting nil
		// slices.)
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(body, &probe); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		_, hasServers := probe["servers"]
		_, hasRules := probe["rules"]
		if !hasServers && !hasRules {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": `body must include "servers" and/or "rules"; refusing to clear dns_options`,
				"field": "dns",
			})
			return
		}
		var req state.DNSOptions
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		// Light validation: kinds must be in the known set.
		for i, srv := range req.Servers {
			switch srv.Kind {
			case state.DNSServerKindTemplate, state.DNSServerKindPreset, state.DNSServerKindUser:
			default:
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error": fmt.Sprintf("servers[%d]: unknown kind %q", i, srv.Kind),
					"field": fmt.Sprintf("dns.servers[%d].kind", i),
				})
				return
			}
		}
		for i, rl := range req.Rules {
			switch rl.Kind {
			case state.DNSRuleKindPreset, state.DNSRuleKindUser:
			default:
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error": fmt.Sprintf("rules[%d]: unknown kind %q", i, rl.Kind),
					"field": fmt.Sprintf("dns.rules[%d].kind", i),
				})
				return
			}
		}
		s.stateMu.Lock()
		defer s.stateMu.Unlock()
		st, err := s.facade.LoadState()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "load state: " + err.Error()})
			return
		}
		st.DNS = req
		if err := s.facade.SaveState(st); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "save state: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true,
			"diff_summary": []string{fmt.Sprintf("dns: replace, %d servers / %d rules",
				len(req.Servers), len(req.Rules))},
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET or PATCH required"})
	}
}

// patchDNSRulesReq — body for PATCH /state/dns/rules.
//
// Тот же текст-формат, что принимает редактор визарда (`build.ParseDNSRulesText`):
// {"rules":[...]}, голый массив, одиночный объект или legacy-многострочный.
// Пустая строка → стереть все USER rules.
type patchDNSRulesReq struct {
	Text string `json:"text"`
}

// handleStateDNSRules — GET/PATCH /state/dns/rules.
//
// Это **текстовый вид только USER-правил** (kind=user) — тот же, что
// показывает визард. PRESET-правила (toggle-ссылки на template.presets[*].dns_rule)
// сюда не попадают и при PATCH сохраняются как есть. Этим эндпоинт
// отличается от `PATCH /state/dns`, который заменяет всю секцию.
//
// SPEC 050 контракт: bad JSON → 400, парсинг текста → 422, save → 200.
func (s *Server) handleStateDNSRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		st, err := s.facade.LoadState()
		if err != nil {
			writeJSON(w, stateErrStatus(err), map[string]any{"error": err.Error()})
			return
		}
		// Соберём текст из USER-правил. Body — map[string]interface{},
		// именно его принимает DNSRulesToText.
		userRules := make([]interface{}, 0)
		for _, rl := range st.DNS.Rules {
			if rl.Kind == state.DNSRuleKindUser && rl.Body != nil {
				userRules = append(userRules, rl.Body)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"text": build.DNSRulesToText(userRules)})

	case http.MethodPatch:
		var req patchDNSRulesReq
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		parsed, perr := build.ParseDNSRulesText(req.Text)
		if perr != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": "dns rules text: " + perr.Error(),
				"field": "text",
			})
			return
		}
		s.stateMu.Lock()
		defer s.stateMu.Unlock()
		st, err := s.facade.LoadState()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "load state: " + err.Error()})
			return
		}
		// Считаем сколько USER-правил было до — для diff_summary.
		beforeUser := 0
		kept := make([]state.DNSRule, 0, len(st.DNS.Rules))
		for _, rl := range st.DNS.Rules {
			if rl.Kind == state.DNSRuleKindUser {
				beforeUser++
				continue
			}
			kept = append(kept, rl)
		}
		// USER-правила из parsed становятся новыми DNSRule с enabled=true
		// (text-формат не несёт enabled-флаг — это совместимо с визардом,
		// где переключатели у user-правил отсутствуют).
		for _, p := range parsed {
			m, ok := p.(map[string]interface{})
			if !ok {
				// build.ParseDNSRulesText гарантирует, что элементы — объекты,
				// но защитимся от регресса.
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error": "dns rules text: non-object entry after parse",
					"field": "text",
				})
				return
			}
			kept = append(kept, state.DNSRule{
				Kind:    state.DNSRuleKindUser,
				Enabled: true,
				Body:    m,
			})
		}
		st.DNS.Rules = kept
		if err := s.facade.SaveState(st); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "save state: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true,
			"diff_summary": []string{fmt.Sprintf("dns.rules[user]: replace, %d → %d entries",
				beforeUser, len(parsed))},
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET or PATCH required"})
	}
}

// handleStateOutboundsResolved — GET /state/outbounds/resolved.
//
// Returns the merged outbound bodies (post-MergeOutboundUpdatesInPlace +
// resolveBaseBody for #TEMPLATE# / preset refs) so consumers see exactly
// what the build emit will produce. Useful for fixtures and asserting
// that a SPEC 058 USER patch lands correctly.
//
// We don't write anywhere — purely a read endpoint over the resolved
// view. Template load failures map to 500 (no usable template = nothing
// meaningful to resolve against).
func (s *Server) handleStateOutboundsResolved(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	st, err := s.facade.LoadState()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "load state: " + err.Error()})
		return
	}
	td, err := s.facade.LoadTemplate()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "load template: " + err.Error()})
		return
	}
	// MergeOutboundUpdatesInPlace mutates a ParserConfig in place; copy
	// the outbound slice into a fresh ParserConfig so we don't affect
	// any state cached by the facade.
	pc := configtypes.ParserConfig{}
	pc.ParserConfig.Outbounds = append([]configtypes.OutboundConfig(nil), st.Connections.Outbounds...)
	build.MergeOutboundUpdatesInPlace(&pc, td)
	writeJSON(w, http.StatusOK, map[string]any{"outbounds": pc.ParserConfig.Outbounds})
}

// stateErrStatus maps state.ErrNotFound to 404 (same contract as /state/full);
// everything else stays 500.
func stateErrStatus(err error) int {
	if errors.Is(err, state.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
