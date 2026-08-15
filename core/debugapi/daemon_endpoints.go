// Package debugapi — SPEC 100 §3.6: локальный демон `sing-box lxd`.
//
// Интерфейс кросс-платформенный, но фасад существует только там, где есть
// демонный движок (darwin): wiring других платформ просто не зовёт
// EnableDaemon, и группа не регистрируется — её нет ни в роутинге, ни в /help,
// а capabilities.daemon в манифесте стоит false.
package debugapi

import (
	"net/http"
	"strings"

	"google.golang.org/grpc"
)

// DaemonStatus — снимок состояния локального демона (зеркало
// core.DaemonUIStatus; свой тип, потому что core импортирует debugapi и
// обратная ссылка замкнула бы цикл).
type DaemonStatus struct {
	CoreSupportsLxd  bool   `json:"core_supports_lxd"`
	ServiceInstalled bool   `json:"service_installed"`
	Paired           bool   `json:"paired"`
	Address          string `json:"address"`
	Reachable        bool   `json:"reachable"`
	CoreStatus       string `json:"core_status,omitempty"`
	LastError        string `json:"last_error,omitempty"`
	InterruptedApply bool   `json:"interrupted_apply"`
	DaemonVersion    string `json:"daemon_version,omitempty"`
	StateDir         string `json:"state_dir,omitempty"`
}

// DaemonCommands — готовые sudo-команды для терминала оператора. API их
// ТОЛЬКО отдаёт: принцип «sudo только в вашем терминале» (CONSTITUTION)
// неизменен, лаунчер не исполняет привилегированных операций.
type DaemonCommands struct {
	Install        string `json:"install"`
	Uninstall      string `json:"uninstall"`
	UninstallPurge string `json:"uninstall_purge"`
	Repair         string `json:"repair"`
	Kickstart      string `json:"kickstart"`
	ShowSecret     string `json:"show_secret"`
}

// DaemonFacade — что debugapi нужно от управления локальным демоном.
// Реализуется в core (darwin wiring).
type DaemonFacade interface {
	Status() DaemonStatus
	Pair(invite, secret string) error
	Unpair() error
	SetAddress(addr string) error
	SetSecret(secret string) error
	Commands() DaemonCommands

	// EngineMode — активный движок ядра: "classic" | "daemon".
	EngineMode() string
	// SwitchEngine переключает движок и персистит выбор. Ошибка, если VPN
	// запущен (движок меняется только на остановленном ядре) или демон не
	// сопряжён.
	SwitchEngine(mode string) error

	// AdminDo — произвольный admin-REST вызов к локальному демону.
	AdminDo(method, path string, body []byte, contentType string) (int, []byte, string, error)
	// GRPCConn — соединение к gRPC-плоскости локального демона. Соединением
	// владеет вызывающий: raw-handler закрывает его после вызова (loopback,
	// дешёвый dial — пул тут не оправдан).
	GRPCConn() (*grpc.ClientConn, error)
}

// daemonEndpoints — таблица группы /daemon/*.
func (s *Server) daemonEndpoints() []apiEndpoint {
	return []apiEndpoint{
		{"GET", "/daemon/status", true, "Local daemon: pairing, service, core status", s.handleDaemonStatus},
		{"POST", "/daemon/pair", true, "Pair with the local daemon (invite)", s.handleDaemonPair},
		{"POST", "/daemon/unpair", true, "Forget local pairing (keys, pin, secret)", s.handleDaemonUnpair},
		{"PATCH", "/daemon/settings", true, "Set daemon address and/or secret", s.handleDaemonSettings},
		{"GET/POST", "/daemon/engine", true, "Get / switch core engine (classic|daemon)", s.handleDaemonEngine},
		{"GET", "/daemon/commands", true, "Ready-to-run sudo commands (API never executes them)", s.handleDaemonCommands},
		{"POST", "/daemon/raw/rest", true, "Raw admin-REST call to the local daemon", s.handleDaemonRawREST},
		{"POST", "/daemon/raw/grpc", true, "Raw daemon.* gRPC call to the local daemon", s.handleDaemonRawGRPC},
	}
}

func (s *Server) handleDaemonStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	writeJSON(w, http.StatusOK, s.daemon.Status())
}

func (s *Server) handleDaemonPair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var req struct {
		Invite string `json:"invite"`
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
	if err := s.daemon.Pair(req.Invite, req.Secret); err != nil {
		writeJSON(w, remoteCallStatus(err), map[string]any{
			"error":   err.Error(),
			"warning": "invite codes are single-use; if enroll reached the daemon, request a fresh invite before retrying",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": s.daemon.Status()})
}

func (s *Server) handleDaemonUnpair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	if err := s.daemon.Unpair(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"warning": "the daemon still trusts the old client cert; revoke it there with `sing-box lxd client remove`",
	})
}

// handleDaemonSettings — PATCH {addr?, secret?}.
func (s *Server) handleDaemonSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "PATCH required"})
		return
	}
	var req struct {
		Addr   *string `json:"addr"`
		Secret *string `json:"secret"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
		return
	}
	if req.Addr == nil && req.Secret == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "nothing to update: pass addr and/or secret"})
		return
	}
	if req.Addr != nil {
		if err := s.daemon.SetAddress(*req.Addr); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}
	if req.Secret != nil {
		if err := s.daemon.SetSecret(*req.Secret); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": s.daemon.Status()})
}

// handleDaemonEngine — GET текущий движок / POST {mode} переключение.
func (s *Server) handleDaemonEngine(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"mode": s.daemon.EngineMode()})

	case http.MethodPost:
		var req struct {
			Mode string `json:"mode"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		mode := strings.TrimSpace(req.Mode)
		if mode != "classic" && mode != "daemon" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": `mode must be "classic" or "daemon"`})
			return
		}
		if err := s.daemon.SwitchEngine(mode); err != nil {
			// Работающий VPN — конфликт состояния, а не внутренняя ошибка.
			st := http.StatusConflict
			if !strings.Contains(err.Error(), "stop the VPN") {
				st = http.StatusInternalServerError
			}
			writeJSON(w, st, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mode": s.daemon.EngineMode()})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET or POST required"})
	}
}

func (s *Server) handleDaemonCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"commands": s.daemon.Commands(),
		"note":     "run these yourself in a terminal; the launcher and this API never execute privileged commands",
	})
}

// handleDaemonRawREST — POST /daemon/raw/rest.
func (s *Server) handleDaemonRawREST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	req, payload, err := decodeRawRESTRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	status, body, contentType, err := s.daemon.AdminDo(req.Method, req.Path, payload, req.ContentType)
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	writeRawRESTResponse(w, status, body, contentType)
}

// handleDaemonRawGRPC — POST /daemon/raw/grpc.
func (s *Server) handleDaemonRawGRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var conn *grpc.ClientConn
	s.serveRawGRPC(w, r, func() (grpc.ClientConnInterface, error) {
		c, err := s.daemon.GRPCConn()
		if err != nil {
			return nil, err
		}
		conn = c
		return c, nil
	})
	if conn != nil {
		_ = conn.Close()
	}
}
