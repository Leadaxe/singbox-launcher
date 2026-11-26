// File impair_endpoints.go — управление тестовым outbound'ом `impair` на лету
// (SPEC 164 ядра sing-box-lx, ядро >= 1.14.4-lx.37-rc.1).
//
// Только для тестов: в обычном UI лаунчера impair нет и не будет (решение
// владельца 26.11.2026). Маршруты — по образцу LxBox (их задача 762); под
// ними — gRPC StartedService локального демона: SetImpair, ScheduleImpair,
// GetImpairStatus. Запросы и статус — JSON в схеме конфига ядра
// (docs-lx/lx-impair.md, раздел управления на лету), поэтому здесь они не
// разбираются, а передаются как есть.
package debugapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	daemonpb "singbox-launcher/internal/daemonpb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// impairEndpoints — таблица группы /impair/*.
func (s *Server) impairEndpoints() []apiEndpoint {
	return []apiEndpoint{
		{"GET", "/impair/{outbound}", true, "Status of an impair test outbound (link_params, counters, sublinks, schedule)", s.handleImpairStatus},
		{"POST", "/impair/{outbound}/set", true, "Apply one schedule entry to an impair outbound now: {\"link_params\":{…}} or {\"sublinks\":[…]|false}", s.handleImpairSet},
		{"POST", "/impair/{outbound}/schedule", true, "Replace the schedule of an impair outbound: {\"entries\":[…],\"start\":\"\"|\"now\"|\"first_use\",\"repeat\":\"\"|\"60s\"}; [] restores the config", s.handleImpairSchedule},
	}
}

// impairTag — тег outbound'а из пути; пустой — 400.
func impairTag(w http.ResponseWriter, r *http.Request) (string, bool) {
	tag := strings.TrimSpace(pathParam(r, "outbound"))
	if tag == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "impair outbound tag is required in the path"})
		return "", false
	}
	return tag, true
}

// handleImpairStatus — GET /impair/{outbound}.
func (s *Server) handleImpairStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	tag, ok := impairTag(w, r)
	if !ok {
		return
	}
	client, closeConn, err := s.startedClient()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "daemon grpc: " + err.Error()})
		return
	}
	defer closeConn()
	st, err := client.GetImpairStatus(r.Context(), &daemonpb.ImpairStatusRequest{OutboundTag: tag})
	writeImpairResult(w, st, err)
}

// handleImpairSet — POST /impair/{outbound}/set: тело — одна запись
// расписания без "at", JSON-объект.
func (s *Server) handleImpairSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	tag, ok := impairTag(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read body: " + err.Error()})
		return
	}
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(body, &entry); err != nil || entry == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body must be one schedule entry without \"at\", a JSON object: {\"link_params\":{…}} or {\"sublinks\":[…]|false}"})
		return
	}
	compact, err := compactJSON(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON: " + err.Error()})
		return
	}
	client, closeConn, err := s.startedClient()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "daemon grpc: " + err.Error()})
		return
	}
	defer closeConn()
	st, err := client.SetImpair(r.Context(), &daemonpb.SetImpairRequest{OutboundTag: tag, Entry: compact})
	writeImpairResult(w, st, err)
}

// handleImpairSchedule — POST /impair/{outbound}/schedule.
func (s *Server) handleImpairSchedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	tag, ok := impairTag(w, r)
	if !ok {
		return
	}
	var req struct {
		Entries json.RawMessage `json:"entries"`
		Start   string          `json:"start"`
		Repeat  string          `json:"repeat"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
		return
	}
	var entries []json.RawMessage
	if len(req.Entries) == 0 || json.Unmarshal(req.Entries, &entries) != nil || entries == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "\"entries\" must be a JSON array of schedule entries; [] stops the schedule and restores the config's link"})
		return
	}
	switch req.Start {
	case "", "now", "first_use":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "\"start\" must be \"\", \"now\" or \"first_use\""})
		return
	}
	compact, err := compactJSON(req.Entries)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON: " + err.Error()})
		return
	}
	client, closeConn, err := s.startedClient()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "daemon grpc: " + err.Error()})
		return
	}
	defer closeConn()
	st, err := client.ScheduleImpair(r.Context(), &daemonpb.ScheduleImpairRequest{
		OutboundTag: tag,
		Entries:     compact,
		Start:       req.Start,
		Repeat:      strings.TrimSpace(req.Repeat),
	})
	writeImpairResult(w, st, err)
}

// writeImpairResult — статус ядра как есть (JSON) либо ошибка с HTTP-кодом.
func writeImpairResult(w http.ResponseWriter, st *daemonpb.ImpairStatus, err error) {
	if err != nil {
		writeJSON(w, impairRPCStatus(err), map[string]any{"error": impairRPCMessage(err)})
		return
	}
	raw := strings.TrimSpace(st.GetJson())
	if raw == "" || !json.Valid([]byte(raw)) {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "core returned an invalid impair status", "raw": raw})
		return
	}
	writeJSON(w, http.StatusOK, json.RawMessage(raw))
}

// impairRPCStatus — HTTP-код для ошибки gRPC ядра: нет тега — 404, не impair
// или неверный аргумент — 400, сервис не запущен — 409, ядро без нужного RPC —
// 501. Тот же маппинг у замеров своего ядра (daemon_observe_endpoints.go).
func impairRPCStatus(err error) int {
	switch status.Code(err) {
	case codes.NotFound:
		return http.StatusNotFound
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.FailedPrecondition:
		return http.StatusConflict
	}
	return chainRPCStatus(err)
}

// impairRPCMessage — текст ошибки ядра с подсказкой там, где она не очевидна.
func impairRPCMessage(err error) string {
	switch status.Code(err) {
	case codes.Unimplemented:
		return "core lacks impair control: needs sing-box-lx 1.14.4-lx.37-rc.1 or newer built with with_lx_command and with_lx_impair (" + err.Error() + ")"
	case codes.FailedPrecondition:
		return "core is not running: " + status.Convert(err).Message()
	}
	if st, ok := status.FromError(err); ok {
		return st.Message()
	}
	return err.Error()
}

func compactJSON(raw []byte) (string, error) {
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return "", err
	}
	return b.String(), nil
}
