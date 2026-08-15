// Package debugapi — SPEC 100 remote-machines endpoint group.
//
// Обёртка Debug API над реестром удалённых машин (services.RemoteRegistry) и
// их gRPC-транспортами. Каждый вызов адресует машину явно по {id} — API не
// наследует UI-концепцию «активной машины» (remote-override): stateless-адрес
// в пути надёжнее модального состояния, которое агент забыл переключить.
package debugapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/lxdclient"
	"singbox-launcher/internal/platform"
)

// RemoteAPI — зависимости remote-группы. Конкретные типы, а не интерфейс:
// services не импортирует debugapi (цикла нет), а реестр — файловый и
// прекрасно живёт в тестах на temp-каталоге.
type RemoteAPI struct {
	Registry *services.RemoteRegistry
	Pool     *services.TransportPool
	// ExecDir — корень лаунчера; от него считаются пути профилей машин
	// (bin/wizard_states/remote/<id>/…).
	ExecDir string

	// UI-override (SPEC 100 §3.8) — то, что в UI делают кнопки
	// Connect/Disconnect вкладки Remote: перевести вкладку Servers лаунчера
	// на машину. Опциональные: nil (или ErrUIUnavailable из замыкания) =
	// headless-запуск или UI ещё не создан → 503.
	//
	// Отдельно от stateless-адресации: обычные /remote/machines/{id}/*
	// вызовы НЕ трогают выбор в UI — это единственные ручки, которые его
	// меняют.
	UIConnect    func(id string) error
	UIDisconnect func() error
	UIState      func() (id, name string, active bool, err error)
}

// ErrUIUnavailable — UI-override недоступен: лаунчер работает headless или
// окно ещё не создано. Мапится в 503 (временное состояние, не ошибка запроса).
var ErrUIUnavailable = errors.New("UI override is unavailable: launcher is headless or the UI is not initialized yet")

// remoteEndpoints — таблица группы. Регистрируется из Server.endpoints()
// только при включённой группе, поэтому в /help она видна ровно тогда, когда
// реально доступна.
func (s *Server) remoteEndpoints() []apiEndpoint {
	return []apiEndpoint{
		// Реестр машин.
		{"GET/POST", "/remote/machines", true, "List machines / pair a new one (invite)", s.handleRemoteMachines},
		{"GET/PATCH/DELETE", "/remote/machines/{id}", true, "Get / update / remove a machine", s.handleRemoteMachineByID},
		{"POST", "/remote/machines/{id}/repair", true, "Re-pair with a fresh invite (new client key)", s.handleRemoteRepair},
		{"POST", "/remote/machines/{id}/profile/copy-from", true, "Copy wizard profile from another machine", s.handleRemoteProfileCopyFrom},

		// Здоровье, ядро, конфиг, деплой.
		{"GET", "/remote/machines/{id}/health", true, "Reachability + core status + config SHAs", s.handleRemoteHealth},
		{"POST", "/remote/machines/{id}/core/start", true, "Start the machine's core", s.handleRemoteCoreStart},
		{"POST", "/remote/machines/{id}/core/stop", true, "Stop the machine's core (drops its VPN)", s.handleRemoteCoreStop},
		{"POST", "/remote/machines/{id}/core/rollback", true, "Roll back to last-good config", s.handleRemoteCoreRollback},
		{"GET", "/remote/machines/{id}/config/active", true, "Running config fetched from the machine", s.handleRemoteConfigActive},
		{"GET", "/remote/machines/{id}/config/built", true, "Locally built config of the machine", s.handleRemoteConfigBuilt},
		{"POST", "/remote/machines/{id}/deploy", true, "Deploy resources + config to the machine", s.handleRemoteDeploy},

		// Профиль машины (wizard state) — зеркала /state/*.
		{"GET", "/remote/machines/{id}/state/full", true, "Machine's full wizard state JSON", s.handleRemoteStateFull},
		{"GET/PATCH", "/remote/machines/{id}/state/rules", true, "Get / replace|append machine's routing rules", s.handleRemoteStateRules},
		{"GET/PATCH", "/remote/machines/{id}/state/dns", true, "Get / replace machine's dns_options", s.handleRemoteStateDNS},
		{"GET/PATCH", "/remote/machines/{id}/state/dns/rules", true, "Get / replace machine's USER dns rules (text)", s.handleRemoteStateDNSRules},
		{"GET", "/remote/machines/{id}/state/outbounds/resolved", true, "Machine's resolved outbounds", s.handleRemoteStateOutboundsResolved},

		// Наблюдаемость (gRPC StartedService + admin REST хоста).
		{"GET", "/remote/machines/{id}/groups", true, "Selector group tags of the machine's core", s.handleRemoteGroups},
		{"GET", "/remote/machines/{id}/proxies", true, "Proxies of a group (?group=)", s.handleRemoteProxies},
		{"POST", "/remote/machines/{id}/proxies/switch", true, "Select outbound in a group", s.handleRemoteProxySwitch},
		{"POST", "/remote/machines/{id}/proxies/delay", true, "URL-test one outbound on the machine", s.handleRemoteProxyDelay},
		{"GET", "/remote/machines/{id}/pool", true, "Balancer pool of a urltest group (?group=)", s.handleRemotePool},
		{"GET", "/remote/machines/{id}/rules", true, "Routing rules table of the machine's core", s.handleRemoteRulesList},
		{"GET", "/remote/machines/{id}/outbounds", true, "Outbound tags of the machine's core", s.handleRemoteOutbounds},
		{"GET", "/remote/machines/{id}/status", true, "Core status snapshot + uptime", s.handleRemoteStatus},
		{"GET/DELETE", "/remote/machines/{id}/connections", true, "Live connections snapshot / close all", s.handleRemoteConnections},
		{"DELETE", "/remote/machines/{id}/connections/{conn_id}", true, "Close one connection", s.handleRemoteConnectionByID},
		{"GET", "/remote/machines/{id}/dns/queries", true, "DNS queries window (?duration=&max=)", s.handleRemoteDNSQueries},
		{"GET", "/remote/machines/{id}/logs", true, "Core log window (?duration=&max=)", s.handleRemoteLogs},
		{"GET", "/remote/machines/{id}/host", true, "Host telemetry (CPU, memory, disks)", s.handleRemoteHost},
		{"GET", "/remote/machines/{id}/host/interfaces", true, "Host network interfaces + counters", s.handleRemoteHostInterfaces},
		{"GET", "/remote/machines/{id}/clients", true, "LAN clients directory of the machine", s.handleRemoteClients},
		{"PUT/DELETE", "/remote/machines/{id}/clients/{key}/label", true, "Set / delete a client label", s.handleRemoteClientLabel},

		// Ресурс-стор машины.
		{"GET", "/remote/machines/{id}/resources", true, "Resource overview: local vs machine", s.handleRemoteResources},
		{"POST", "/remote/machines/{id}/resources/sync", true, "Sync built-config resources to the machine", s.handleRemoteResourcesSync},
		{"GET/PUT/DELETE", "/remote/machines/{id}/resources/{name}", true, "Fetch / upload / delete one resource", s.handleRemoteResourceByName},
		{"POST", "/remote/machines/{id}/resources/{name}/download", true, "Download resource into the machine's local dir", s.handleRemoteResourceDownload},

		// Произвольные вызовы (passthrough) — туннель к сопряжённому демону.
		{"POST", "/remote/machines/{id}/raw/rest", true, "Raw admin-REST call to the machine's daemon", s.handleRemoteRawREST},
		{"POST", "/remote/machines/{id}/raw/grpc", true, "Raw daemon.* gRPC call to the machine", s.handleRemoteRawGRPC},

		// UI-override: перевод вкладки Servers лаунчера на машину (SPEC 100
		// §3.8) — то же, что кнопки Connect/Disconnect вкладки Remote.
		{"GET", "/remote/ui", true, "Which machine the launcher UI is connected to", s.handleRemoteUIState},
		{"POST", "/remote/machines/{id}/ui/connect", true, "Point the launcher UI (Servers tab) at this machine", s.handleRemoteUIConnect},
		{"POST", "/remote/ui/disconnect", true, "Return the launcher UI to the local core", s.handleRemoteUIDisconnect},
	}
}

// remoteMachineID достаёт {id} и проверяет, что машина есть в реестре.
// Отвечает 404/500 сам; второй результат false = ответ уже написан.
func (s *Server) remoteMachineID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "machine id is empty"})
		return "", false
	}
	_, ok, err := s.remote.Registry.Get(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return "", false
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("unknown machine %q", id)})
		return "", false
	}
	return id, true
}

// remoteCallStatus классифицирует ошибку похода к машине.
//
//	422 — демон отклонил конфиг валидацией (ApplyError.Rejected)
//	409 — ресурс занят живой ссылкой (ResourceError.InUse)
//	404 — built-конфига ещё нет (ErrBuiltConfigMissing)
//	504 — таймаут вызова
//	502 — машина недоступна (сеть/пин/отказ канала)
//	500 — всё остальное
func remoteCallStatus(err error) int {
	var applyErr *lxdclient.ApplyError
	if errors.As(err, &applyErr) && applyErr.Rejected() {
		return http.StatusUnprocessableEntity
	}
	var resErr *lxdclient.ResourceError
	if errors.As(err, &resErr) && resErr.InUse() {
		return http.StatusConflict
	}
	if errors.Is(err, services.ErrBuiltConfigMissing) {
		return http.StatusNotFound
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable:
			return http.StatusBadGateway
		case codes.DeadlineExceeded:
			return http.StatusGatewayTimeout
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return http.StatusGatewayTimeout
		}
		return http.StatusBadGateway
	}
	var urlErr *url.Error
	var opErr *net.OpError
	if errors.As(err, &urlErr) || errors.As(err, &opErr) {
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}

// writeRemoteError — единый ответ об ошибке похода к машине.
func writeRemoteError(w http.ResponseWriter, err error) {
	writeJSON(w, remoteCallStatus(err), map[string]any{"error": err.Error()})
}

// remoteTransport берёт транспорт машины из пула. false = ответ уже написан.
func (s *Server) remoteTransport(w http.ResponseWriter, id string) (*services.LxdRemoteTransport, bool) {
	t, err := s.remote.Pool.Get(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return nil, false
	}
	return t, true
}

// machineView — проекция записи реестра для JSON-ответов.
//
// Секреты не маскируются: API loopback-only, локальная машина — trust-boundary
// (та же позиция, что у /state/full и /debug/snapshot).
type machineView struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Addr              string `json:"addr"`
	ServerFingerprint string `json:"server_fingerprint,omitempty"`
	Secret            string `json:"secret,omitempty"`
	GOOS              string `json:"goos,omitempty"`
	GOARCH            string `json:"goarch,omitempty"`
	StateDir          string `json:"state_dir,omitempty"`
	AddedAt           string `json:"added_at,omitempty"`
}

func machineViewOf(d services.RemoteDaemon) machineView {
	return machineView{
		ID: d.ID, Name: d.Name, Addr: d.Addr,
		ServerFingerprint: d.ServerFingerprint, Secret: d.Secret,
		GOOS: d.GOOS, GOARCH: d.GOARCH,
		StateDir: d.StateDir, AddedAt: d.AddedAt,
	}
}

// handleRemoteMachines — GET (список) / POST (сопряжение по приглашению).
func (s *Server) handleRemoteMachines(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.remote.Registry.List()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		out := make([]machineView, 0, len(list))
		for _, d := range list {
			out = append(out, machineViewOf(d))
		}
		writeJSON(w, http.StatusOK, map[string]any{"machines": out})

	case http.MethodPost:
		var req struct {
			Invite string `json:"invite"`
			Name   string `json:"name"`
			Addr   string `json:"addr"`
			Secret string `json:"secret"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		if strings.TrimSpace(req.Invite) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invite is required (addr#fingerprint#code)"})
			return
		}
		// Форматная ошибка приглашения — вина запроса (400), не машины (5xx).
		// Валидируем до enroll: битый invite не должен считаться «недоступной
		// сетью» и не сжигает одноразовый код.
		if _, perr := lxdclient.ParseInvite(req.Invite); perr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": perr.Error()})
			return
		}
		entry, err := s.remote.Registry.PairWithAddr(req.Invite, req.Name, req.Addr, req.Secret)
		if err != nil {
			// Код приглашения одноразовый: неудача сети означает, что он мог
			// сгореть — предупреждаем, чтобы агент не долбил повторами.
			writeJSON(w, remoteCallStatus(err), map[string]any{
				"error":   err.Error(),
				"warning": "invite codes are single-use; if enroll reached the daemon, request a fresh invite before retrying",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "machine": machineViewOf(entry)})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET or POST required"})
	}
}

// handleRemoteMachineByID — GET / PATCH {name,addr,goos,goarch} / DELETE.
func (s *Server) handleRemoteMachineByID(w http.ResponseWriter, r *http.Request) {
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		d, _, err := s.remote.Registry.Get(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, machineViewOf(d))

	case http.MethodPatch:
		var req struct {
			Name   *string `json:"name"`
			Addr   *string `json:"addr"`
			GOOS   *string `json:"goos"`
			GOARCH *string `json:"goarch"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		if req.Name == nil && req.Addr == nil && req.GOOS == nil && req.GOARCH == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "nothing to update: pass name, addr, goos and/or goarch"})
			return
		}
		cur, _, err := s.remote.Registry.Get(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if req.Name != nil || req.Addr != nil {
			name, addr := cur.Name, cur.Addr
			if req.Name != nil {
				name = *req.Name
			}
			if req.Addr != nil {
				addr = *req.Addr
			}
			if err := s.remote.Registry.Update(id, name, addr); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
			if req.Addr != nil {
				// Канал сменился — старый транспорт говорит с прежним адресом.
				s.remote.Pool.Invalidate(id)
			}
		}
		if req.GOOS != nil || req.GOARCH != nil {
			goos, goarch := cur.GOOS, cur.GOARCH
			if req.GOOS != nil {
				goos = *req.GOOS
			}
			if req.GOARCH != nil {
				goarch = *req.GOARCH
			}
			if err := s.remote.Registry.SetPlatform(id, goos, goarch); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
		}
		updated, _, err := s.remote.Registry.Get(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "machine": machineViewOf(updated)})

	case http.MethodDelete:
		s.remote.Pool.Invalidate(id)
		if err := s.remote.Registry.Remove(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true,
			// Честность модели доверия (SPEC 097): мы забыли ключ у себя, но
			// демон по-прежнему доверяет ему.
			"warning": "access is NOT revoked on the daemon side; run `sing-box lxd client remove` there",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET, PATCH or DELETE required"})
	}
}

// handleRemoteRepair — POST {invite, addr?, secret?}: пере-сопряжение с
// перевыпуском клиентской пары; профиль машины сохраняется.
func (s *Server) handleRemoteRepair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	var req struct {
		Invite string `json:"invite"`
		Addr   string `json:"addr"`
		Secret string `json:"secret"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Invite) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invite is required (addr#fingerprint#code)"})
		return
	}
	if _, perr := lxdclient.ParseInvite(req.Invite); perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": perr.Error()})
		return
	}
	// Старый транспорт держит канал со старым пином/ключом — закрываем до
	// пере-сопряжения, чтобы после него не осталось соединения-призрака.
	s.remote.Pool.Invalidate(id)
	entry, err := s.remote.Registry.RePair(id, req.Invite, req.Addr, req.Secret)
	if err != nil {
		writeJSON(w, remoteCallStatus(err), map[string]any{
			"error":   err.Error(),
			"warning": "invite codes are single-use; if enroll reached the daemon, request a fresh invite before retrying",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "machine": machineViewOf(entry)})
}

// handleRemoteProfileCopyFrom — POST {source_id, overwrite?}.
func (s *Server) handleRemoteProfileCopyFrom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	var req struct {
		SourceID  string `json:"source_id"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.SourceID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "source_id is required"})
		return
	}
	// Копия перезаписывает state приёмника целиком — UI спрашивает
	// подтверждение, API требует явного overwrite=true (SPEC 100 §3.1).
	dstPath := s.machineStatePath(id)
	if _, err := os.Stat(dstPath); err == nil && !req.Overwrite {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "target machine already has a wizard state; pass overwrite=true to replace it",
		})
		return
	}
	mu := s.machineMutex(id)
	mu.Lock()
	defer mu.Unlock()
	if err := s.remote.Registry.CopyProfileFrom(req.SourceID, id); err != nil {
		st := http.StatusInternalServerError
		if strings.Contains(err.Error(), "unknown id") {
			st = http.StatusNotFound
		}
		writeJSON(w, st, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleRemoteHealth — GET: reachability, статус ядра, паспорт, SHA конфигов.
func (s *Server) handleRemoteHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	h := s.remote.Registry.Health(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"reachable":         h.Reachable,
		"error":             emptyToNil(h.Err),
		"core_status":       emptyToNil(h.CoreStatus),
		"last_error":        emptyToNil(h.LastError),
		"version":           emptyToNil(h.Version),
		"state_dir":         emptyToNil(h.StateDir),
		"active_sha":        emptyToNil(h.ActiveSHA),
		"last_good_sha":     emptyToNil(h.LastGoodSHA),
		"interrupted_apply": h.InterruptedApply,
	})
}

func emptyToNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Server) handleRemoteCoreStart(w http.ResponseWriter, r *http.Request) {
	s.remoteCoreAction(w, r, s.remote.Registry.StartCore)
}

func (s *Server) handleRemoteCoreStop(w http.ResponseWriter, r *http.Request) {
	s.remoteCoreAction(w, r, s.remote.Registry.StopCore)
}

func (s *Server) handleRemoteCoreRollback(w http.ResponseWriter, r *http.Request) {
	s.remoteCoreAction(w, r, s.remote.Registry.RollbackCore)
}

func (s *Server) remoteCoreAction(w http.ResponseWriter, r *http.Request, action func(string) error) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	if err := action(id); err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleRemoteConfigActive — GET: работающий конфиг С машины.
func (s *Server) handleRemoteConfigActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	raw, err := s.remote.Registry.ActiveConfig(id)
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// handleRemoteConfigBuilt — GET: локально собранный config.json машины.
func (s *Server) handleRemoteConfigBuilt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	raw, err := os.ReadFile(platform.GetRemoteConfigPathFor(s.remote.ExecDir, id))
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error": "built config does not exist yet — configure the machine first (wizard Save)",
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// handleRemoteDeploy — POST: та же цепочка, что кнопка Deploy (ресурсы →
// конфиг). Body опционален: {config: {…}} деплоит произвольный конфиг вместо
// собранного.
func (s *Server) handleRemoteDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	var config []byte
	if r.ContentLength != 0 {
		var req struct {
			Config json.RawMessage `json:"config"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		if len(req.Config) > 0 && string(req.Config) != "null" {
			config = []byte(req.Config)
		}
	}
	res, err := s.remote.Registry.Deploy(id, config)
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                 true,
		"resources_uploaded": res.ResourcesUploaded,
		"config_sha":         res.ConfigSHA,
	})
}

// --- UI-override (SPEC 100 §3.8) -----------------------------------------

// uiOverrideStatus мапит ошибку UI-хука: недоступный UI — 503 (временное
// состояние процесса), остальное — обычная классификация.
func uiOverrideStatus(err error) int {
	if errors.Is(err, ErrUIUnavailable) {
		return http.StatusServiceUnavailable
	}
	return remoteCallStatus(err)
}

// handleRemoteUIState — GET /remote/ui: на какую машину смотрит вкладка
// Servers лаунчера ({connected:false} = локальное ядро).
func (s *Server) handleRemoteUIState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	if s.remote.UIState == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": ErrUIUnavailable.Error()})
		return
	}
	id, name, active, err := s.remote.UIState()
	if err != nil {
		writeJSON(w, uiOverrideStatus(err), map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connected":    active,
		"machine_id":   emptyToNil(id),
		"machine_name": emptyToNil(name),
	})
}

// handleRemoteUIConnect — POST /remote/machines/{id}/ui/connect: перевести
// вкладку Servers на машину (кнопка Connect).
//
// Health-гейт: кнопка UI после переключения крутит до пяти попыток опроса и
// показывает красный маркер; у API вызов один, поэтому доступность
// проверяется ДО переключения — недоступная машина отвечает 502, а override
// остаётся как был. Ядро в idle — не отказ: подключиться к машине с
// остановленным ядром можно и в UI (узлов просто нет до Start).
func (s *Server) handleRemoteUIConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	if s.remote.UIConnect == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": ErrUIUnavailable.Error()})
		return
	}
	h := s.remote.Registry.Health(id)
	if !h.Reachable {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": fmt.Sprintf("machine is unreachable, UI override not switched: %s", h.Err),
		})
		return
	}
	if err := s.remote.UIConnect(id); err != nil {
		writeJSON(w, uiOverrideStatus(err), map[string]any{"error": err.Error()})
		return
	}
	out := map[string]any{"ok": true, "machine_id": id, "core_status": h.CoreStatus}
	if h.CoreStatus != "started" {
		out["warning"] = "core is not started on the machine — the Servers tab will show no nodes until you start it"
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRemoteUIDisconnect — POST /remote/ui/disconnect: вернуть вкладку
// Servers к локальному ядру (кнопка Disconnect). Идемпотентен: отключать
// нечего — тоже ok.
func (s *Server) handleRemoteUIDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	if s.remote.UIDisconnect == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": ErrUIUnavailable.Error()})
		return
	}
	if err := s.remote.UIDisconnect(); err != nil {
		writeJSON(w, uiOverrideStatus(err), map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// windowParams — параметры сборочного окна стримовых endpoint'ов
// (?duration=&max=): стрим собирается в снапшот, SSE сознательно отложен
// (SPEC 100 §7).
func windowParams(r *http.Request) (time.Duration, int, error) {
	duration := 3 * time.Second
	if v := r.URL.Query().Get("duration"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, 0, fmt.Errorf("duration: %w", err)
		}
		if d <= 0 {
			return 0, 0, errors.New("duration must be positive")
		}
		if d > time.Minute {
			d = time.Minute
		}
		duration = d
	}
	maxEvents := 200
	if v := r.URL.Query().Get("max"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, 0, fmt.Errorf("max: %w", err)
		}
		if n <= 0 {
			return 0, 0, errors.New("max must be positive")
		}
		if n > 5000 {
			n = 5000
		}
		maxEvents = n
	}
	return duration, maxEvents, nil
}
