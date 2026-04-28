package debugapi

import (
	"net/http"

	"singbox-launcher/core/snapshot"
)

// handleSnapshot — HTTP-обёртка над core/snapshot.Build.
//
// Вся логика чтения и компоновки — в пакете core/snapshot (используется также
// UI-кнопкой «Copy snapshot» в Diagnostics tab; обе ветки потребляют один
// и тот же Source of Truth).
//
// Контракт endpoint'а: см. SPECS/038-F-C-DEBUG_API/SUB_SPEC_SNAPSHOT.md.
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	snap := snapshot.Build(
		s.facade.GetExecDir(),
		s.facade.GetLauncherVersion(),
		s.facade.GetSingboxVersion(),
	)
	writeJSON(w, http.StatusOK, snap)
}
