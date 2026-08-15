// Package debugapi — SPEC 100 §3.4: наблюдаемость удалённой машины.
//
// Всё ходит через кешируемый gRPC-транспорт (services.TransportPool) либо
// admin REST той же машины. Стримовые источники (status, connections, dns,
// logs) отдаются СНАПШОТАМИ: подписка открывается на время запроса, события
// собираются окном ?duration=&max= и стрим закрывается. SSE — сознательно v2.
package debugapi

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"singbox-launcher/core/services"
)

func (s *Server) handleRemoteGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}
	groups, err := t.Groups()
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

// handleRemoteProxies — GET ?group=: узлы группы. Без group — первая
// selector-группа машины (агенту не нужен второй запрос ради дефолта).
func (s *Server) handleRemoteProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}
	group := strings.TrimSpace(r.URL.Query().Get("group"))
	if group == "" {
		groups, err := t.Groups()
		if err != nil {
			writeRemoteError(w, err)
			return
		}
		if len(groups) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"group": nil, "proxies": []any{}, "selected": nil})
			return
		}
		group = groups[0]
	}
	proxies, selected, err := t.GroupProxies(group)
	if err != nil {
		if services.IsRemoteGroupUnknown(err) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
			return
		}
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"group": group, "proxies": proxies, "selected": selected})
}

// handleRemoteProxySwitch — POST {group, name}.
func (s *Server) handleRemoteProxySwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	var req struct {
		Group string `json:"group"`
		Name  string `json:"name"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Group) == "" || strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "group and name are required"})
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}
	if err := t.SwitchProxy(req.Group, req.Name); err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleRemoteProxyDelay — POST {name}: URL-тест узла НА СТОРОНЕ машины
// (меряется её канал, не наш).
func (s *Server) handleRemoteProxyDelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}
	delay, err := t.Delay(req.Name)
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": req.Name, "delay_ms": delay})
}

// handleRemotePool — GET ?group=: пул балансировщика urltest-группы.
func (s *Server) handleRemotePool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	group := strings.TrimSpace(r.URL.Query().Get("group"))
	if group == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "group query parameter is required"})
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}
	slots, err := t.PoolSlots(group)
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(slots))
	for _, sl := range slots {
		out = append(out, map[string]any{"slot": sl.Slot, "tag": sl.Tag, "delay_ms": sl.Delay})
	}
	writeJSON(w, http.StatusOK, map[string]any{"group": group, "slots": out})
}

func (s *Server) handleRemoteRulesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}
	rules, err := t.Rules()
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rules))
	for _, rl := range rules {
		out = append(out, map[string]any{"type": rl.Type, "payload": rl.Payload, "action": rl.Action})
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": out})
}

func (s *Server) handleRemoteOutbounds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}
	tags, err := t.Outbounds()
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"outbounds": tags})
}

// handleRemoteStatus — GET: первый кадр SubscribeStatus + StartedAt.
//
// Кадра может не быть (ядро остановлено): тогда status=null, а started_at
// отвечает на вопрос «жило ли оно вообще». Таймаут ожидания кадра короткий —
// это снапшот, а не подписка.
func (s *Server) handleRemoteStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}

	statusCh := make(chan services.RemoteStatus, 1)
	var once sync.Once
	cancel, err := t.SubscribeStatus(func(st services.RemoteStatus) {
		once.Do(func() { statusCh <- st })
	})
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	defer cancel()

	out := map[string]any{"status": nil, "started_at": nil, "uptime_seconds": nil}
	select {
	case st := <-statusCh:
		out["status"] = map[string]any{
			"memory":          st.Memory,
			"goroutines":      st.Goroutines,
			"connections_in":  st.ConnectionsIn,
			"connections_out": st.ConnectionsOut,
			"uplink":          st.Uplink,
			"downlink":        st.Downlink,
			"uplink_total":    st.UplinkTotal,
			"downlink_total":  st.DownlinkTotal,
		}
	case <-time.After(3 * time.Second):
	}
	if startedAt, saErr := t.StartedAt(); saErr == nil && !startedAt.IsZero() {
		out["started_at"] = startedAt.UTC().Format(time.RFC3339)
		out["uptime_seconds"] = int64(time.Since(startedAt).Seconds())
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRemoteConnections — GET снапшот соединений / DELETE обрыв всех.
func (s *Server) handleRemoteConnections(w http.ResponseWriter, r *http.Request) {
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		snapshot, cancel, err := t.SubscribeConnections()
		if err != nil {
			writeRemoteError(w, err)
			return
		}
		defer cancel()
		// Первый кадр стрима приходит асинхронно; ждём готовности снимка
		// коротким поллом. Пустая карта после готовности — честный ответ
		// «соединений нет».
		deadline := time.Now().Add(3 * time.Second)
		for {
			conns, ready := snapshot(context.Background())
			if ready {
				writeJSON(w, http.StatusOK, map[string]any{"connections": conns})
				return
			}
			if time.Now().After(deadline) {
				writeJSON(w, http.StatusOK, map[string]any{
					"connections": map[string]any{},
					"warning":     "no snapshot frame arrived within 3s (core stopped or stream slow)",
				})
				return
			}
			time.Sleep(100 * time.Millisecond)
		}

	case http.MethodDelete:
		if err := t.CloseAllConnections(); err != nil {
			writeRemoteError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET or DELETE required"})
	}
}

// handleRemoteConnectionByID — DELETE: обрыв одного соединения по UUID.
func (s *Server) handleRemoteConnectionByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "DELETE required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	connID := strings.TrimSpace(pathParam(r, "conn_id"))
	if connID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "conn_id is empty"})
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}
	if err := t.CloseConnection(connID); err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleRemoteDNSQueries — GET ?duration=&max=: окно DNS-запросов машины.
func (s *Server) handleRemoteDNSQueries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	duration, maxEvents, err := windowParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}

	var mu sync.Mutex
	events := make([]map[string]any, 0, 64)
	full := make(chan struct{}, 1)
	cancel, err := t.SubscribeDNSQueries(func(q services.DNSQuery) {
		mu.Lock()
		defer mu.Unlock()
		if len(events) >= maxEvents {
			return
		}
		events = append(events, map[string]any{
			"domain":       q.Domain,
			"failed":       q.Failed,
			"error":        emptyToNil(q.Error),
			"answers":      q.Answers,
			"cnames":       q.CNAMEs,
			"dns_server":   emptyToNil(q.DNSServer),
			"process_path": emptyToNil(q.ProcessPath),
		})
		if len(events) >= maxEvents {
			select {
			case full <- struct{}{}:
			default:
			}
		}
	})
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	truncated := false
	select {
	case <-full:
		truncated = true
	case <-time.After(duration):
	}
	cancel()
	mu.Lock()
	defer mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "truncated": truncated})
}

// handleRemoteLogs — GET ?duration=&max=: окно лога ядра машины.
//
// Демон отдаёт кольцевой буфер при подписке, поэтому даже короткое окно
// приносит хвост недавних строк — для «что там происходит» этого достаточно.
func (s *Server) handleRemoteLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	duration, maxEvents, err := windowParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}

	var mu sync.Mutex
	lines := make([]map[string]any, 0, 128)
	full := make(chan struct{}, 1)
	cancel, err := t.SubscribeLogLines(func(l services.LogLine) {
		mu.Lock()
		defer mu.Unlock()
		if len(lines) >= maxEvents {
			return
		}
		lines = append(lines, map[string]any{"level": l.Level, "message": l.Message})
		if len(lines) >= maxEvents {
			select {
			case full <- struct{}{}:
			default:
			}
		}
	}, nil)
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	truncated := false
	select {
	case <-full:
		truncated = true
	case <-time.After(duration):
	}
	cancel()
	mu.Lock()
	defer mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines, "truncated": truncated})
}

// handleRemoteHost — GET: телеметрия хоста машины (admin REST /admin/host).
func (s *Server) handleRemoteHost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}
	info, err := t.HostInfo()
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleRemoteHostInterfaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}
	ifaces, err := t.HostInterfaces()
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ifaces)
}

// handleRemoteClients — GET: справочник устройств сети машины.
//
// В обход кеша транспорта: API-вызов разовый, и отдать просроченный кеш с
// пометкой «обновляется в фоне» агенту не поможет — он не вернётся за
// обновлением. Идём в REST напрямую через реестр.
func (s *Server) handleRemoteClients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	status, body, contentType, err := s.remote.Registry.AdminDo(id, http.MethodGet, "/admin/clients-info", nil, "")
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// handleRemoteClientLabel — PUT {name} / DELETE: собственное имя устройства.
func (s *Server) handleRemoteClientLabel(w http.ResponseWriter, r *http.Request) {
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	key := strings.TrimSpace(pathParam(r, "key"))
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "client key is empty"})
		return
	}
	t, ok := s.remoteTransport(w, id)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req struct {
			Name string `json:"name"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		if err := t.SetClientLabel(key, req.Name); err != nil {
			writeRemoteError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case http.MethodDelete:
		if err := t.DeleteClientLabel(key); err != nil {
			writeRemoteError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "PUT or DELETE required"})
	}
}
