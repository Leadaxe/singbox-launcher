// File daemon_observe_endpoints.go — замеры у своего ядра через gRPC демона:
// URL-тест одного узла, пул балансировщика группы, узел как его исполняет
// ядро. Пара к /remote/machines/{id}/proxies/delay и …/pool удалённой машины;
// нужны тестам целиком скриптом (сценарий impair — SPEC 190).
package debugapi

import (
	"net/http"
	"strings"
	"time"

	"singbox-launcher/api"
	"singbox-launcher/core/services"
	daemonpb "singbox-launcher/internal/daemonpb"
)

// daemonObserveEndpoints — таблица замеров своего ядра (группа демона).
func (s *Server) daemonObserveEndpoints() []apiEndpoint {
	return []apiEndpoint{
		{"POST", "/proxies/delay", true, "URL-test one outbound of the local core: {\"name\", \"url\"?, \"timeout_ms\"?}", s.handleLocalProxyDelay},
		{"GET", "/pool", true, "Balancer pool of a urltest group of the local core (?group=): slots, quarantine, counters, freeze", s.handleLocalPool},
		{"GET", "/outbounds/{node}/config", true, "One node as the local core runs it: original, effective, changes (core SPEC 165)", s.handleLocalOutboundConfig},
	}
}

// handleLocalProxyDelay — POST /proxies/delay.
func (s *Server) handleLocalProxyDelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var req struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		TimeoutMs uint32 `json:"timeout_ms"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
		return
	}
	// Пустые url и timeout — те же, что у пинга в окне Servers, чтобы цифры
	// debug API и UI были сравнимы.
	if strings.TrimSpace(req.URL) == "" {
		req.URL = api.GetPingTestURL()
	}
	if req.TimeoutMs == 0 {
		req.TimeoutMs = uint32(api.GetPingTestTimeoutMs())
	}
	if req.TimeoutMs > probeMaxTimeoutMs {
		req.TimeoutMs = probeMaxTimeoutMs
	}
	client, closeConn, err := s.startedClient()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "daemon grpc: " + err.Error()})
		return
	}
	defer closeConn()
	start := time.Now()
	resp, err := client.URLTestOutbound(r.Context(), &daemonpb.URLTestOutboundRequest{
		OutboundTag: name, Link: req.URL, Timeout: req.TimeoutMs,
	})
	if err != nil {
		writeJSON(w, impairRPCStatus(err), map[string]any{"error": impairRPCMessage(err)})
		return
	}
	out := map[string]any{
		"ok": resp.GetError() == "", "name": name, "url": req.URL,
		"delay_ms": resp.GetDelay(), "elapsed_ms": time.Since(start).Milliseconds(),
	}
	if e := resp.GetError(); e != "" {
		out["error"] = e
	}
	writeJSON(w, http.StatusOK, out)
}

// handleLocalPool — GET /pool?group=.
func (s *Server) handleLocalPool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	group := strings.TrimSpace(r.URL.Query().Get("group"))
	if group == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "group query parameter is required"})
		return
	}
	client, closeConn, err := s.startedClient()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "daemon grpc: " + err.Error()})
		return
	}
	defer closeConn()
	list, err := client.GetPool(r.Context(), &daemonpb.GetPoolRequest{GroupTag: group})
	if err != nil {
		writeJSON(w, impairRPCStatus(err), map[string]any{"error": impairRPCMessage(err)})
		return
	}
	writeJSON(w, http.StatusOK, poolView(group, services.PoolStateFromPB(list)))
}

// poolView — JSON пула; та же форма, что у /remote/machines/{id}/pool.
func poolView(group string, pool services.PoolState) map[string]any {
	slots := make([]map[string]any, 0, len(pool.Slots))
	for _, sl := range pool.Slots {
		slots = append(slots, map[string]any{
			"slot": sl.Slot, "tag": sl.Tag, "delay_ms": sl.Delay,
			"quarantined": sl.Quarantined, "connections": sl.Connections,
			"confirmed_failures": sl.ConfirmedFailures,
		})
	}
	return map[string]any{
		"group": group, "slots": slots,
		"shifted_keys": pool.ShiftedKeys, "marked_keys": pool.MarkedKeys,
		"frozen": pool.Frozen, "frozen_seconds": pool.FrozenSeconds,
	}
}

// handleLocalOutboundConfig — GET /outbounds/{node}/config.
func (s *Server) handleLocalOutboundConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	tag := strings.TrimSpace(pathParam(r, "node"))
	if tag == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "node tag is required in the path"})
		return
	}
	client, closeConn, err := s.startedClient()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "daemon grpc: " + err.Error()})
		return
	}
	defer closeConn()
	resp, err := client.GetOutboundConfig(r.Context(), &daemonpb.GetOutboundConfigRequest{Tag: tag})
	if err != nil {
		writeJSON(w, impairRPCStatus(err), map[string]any{"error": impairRPCMessage(err)})
		return
	}
	cfg, err := services.ParseOutboundConfig(resp.GetContent())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}
