package debugapi

import (
	"errors"
	"net/http"
	"time"
)

// UIInspector — доступ debug API к живому состоянию окон Fyne. Подключается
// через EnableUI из wiring (core знает Fyne, этот пакет — нет), поэтому
// группа /debug/ui/* и capability "ui" существуют только при подключении.
//
// Зачем: фриз UI при живом процессе. Fyne отдаёт клики верхнему overlay-у
// канваса, и забытый overlay (индикатор перетаскивания, невидимый попап)
// глушит всё окно, хотя Go-часть отвечает. Snapshot показывает, что лежит
// в стеке overlay-ев и где фокус; ClearOverlays снимает их без перезапуска.
type UIInspector interface {
	// Snapshot — окна с их canvas, overlay-ами и фокусом; сериализуется как есть.
	Snapshot() (any, error)
	// ClearOverlays снимает все overlay-и со всех окон; возвращает их число.
	ClearOverlays() (int, error)
}

// ErrUILoopUnresponsive — главный цикл Fyne не ответил за uiCallTimeout:
// значит, завис сам цикл, а не только маршрутизация кликов.
var ErrUILoopUnresponsive = errors.New("ui loop unresponsive")

// EnableUI подключает инспектор UI (SPEC 078: группа документируется в
// манифесте только когда включена).
func (s *Server) EnableUI(u UIInspector) { s.ui = u }

func (s *Server) uiEndpoints() []apiEndpoint {
	return []apiEndpoint{
		{"GET", "/debug/ui", true, "Fyne windows: canvas, overlays stack, focus (frozen-UI triage)", s.handleUISnapshot},
		{"POST", "/debug/ui/overlays/clear", true, "Remove every canvas overlay (unfreeze a window blocked by a stale overlay)", s.handleUIClearOverlays},
	}
}

func (s *Server) handleUISnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	snap, err := s.ui.Snapshot()
	if err != nil {
		writeUIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"captured_at": time.Now().UTC().Format(time.RFC3339), "windows": snap})
}

func (s *Server) handleUIClearOverlays(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	n, err := s.ui.ClearOverlays()
	if err != nil {
		writeUIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": n})
}

func writeUIError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrUILoopUnresponsive) {
		writeJSON(w, http.StatusGatewayTimeout, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
}
