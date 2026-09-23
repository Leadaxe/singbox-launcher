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
		s.facade.GetLayout(),
		s.facade.GetLauncherVersion(),
		s.facade.GetSingboxVersion(),
	)
	writeJSON(w, http.StatusOK, snap)
}

// pathsView — JSON-форма paths.PathsInfo для GET /debug/paths (SPEC 135 §4.1).
// Text — тот же блок, что копирует кнопка Copy paths и печатает -paths.
type pathsView struct {
	Mode           string   `json:"mode"`
	EnvSource      []string `json:"env_source"`
	AppDir         string   `json:"app_dir"`
	DataDir        string   `json:"data_dir"`
	LogDir         string   `json:"log_dir"`
	CorePath       string   `json:"core_path"`
	CoreSource     string   `json:"core_source"`
	CoreVersion    string   `json:"core_version"`
	ShadowedCore   string   `json:"shadowed_core"`
	TemplatePath   string   `json:"template_path"`
	TemplateSource string   `json:"template_source"`
	WintunPath     string   `json:"wintun_path"`
	WintunFound    bool     `json:"wintun_found"`
	Text           string   `json:"text"`
}

// handlePaths — GET /debug/paths: где лежат программа, данные, логи, какое
// ядро и какой шаблон выбраны.
func (s *Server) handlePaths(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	p := s.facade.GetPathsInfo()
	env := p.Layout.EnvSource
	if env == nil {
		env = []string{}
	}
	writeJSON(w, http.StatusOK, pathsView{
		Mode:           string(p.Layout.Mode),
		EnvSource:      env,
		AppDir:         string(p.Layout.App),
		DataDir:        string(p.Layout.Data),
		LogDir:         string(p.Layout.Logs),
		CorePath:       p.CorePath,
		CoreSource:     p.CoreSource,
		CoreVersion:    p.CoreVersion,
		ShadowedCore:   p.ShadowedCore,
		TemplatePath:   p.TemplatePath,
		TemplateSource: p.TemplateSource,
		WintunPath:     p.WintunPath,
		WintunFound:    p.WintunFound,
		Text:           p.Text(),
	})
}
