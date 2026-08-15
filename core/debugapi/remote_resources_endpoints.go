// Package debugapi — SPEC 100 §3.5: ресурс-стор удалённой машины.
//
// Ресурсы — файлы, на которые ссылается конфиг машины (.srs rule-set'ы):
// живут в `<state_dir>/resources/` на её стороне и в srs/-каталоге её профиля
// на нашей. Endpoint'ы сводят обе стороны и гоняют файлы между ними.
package debugapi

import (
	"net/http"
	"os"
	"strings"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/platform"
)

// resourceEntryView — проекция services.ResourceEntry с json-тегами.
type resourceEntryView struct {
	Name       string `json:"name"`
	State      string `json:"state"` // match | missing | differs | orphan
	LocalSize  int64  `json:"local_size,omitempty"`
	ServerSize int64  `json:"server_size,omitempty"`
	LocalSHA   string `json:"local_sha,omitempty"`
	ServerSHA  string `json:"server_sha,omitempty"`
	InUse      bool   `json:"in_use"`
}

// handleRemoteResources — GET: сводка local vs machine.
func (s *Server) handleRemoteResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	entries, err := s.remote.Registry.ResourceOverview(id)
	if err != nil {
		writeRemoteError(w, err)
		return
	}
	out := make([]resourceEntryView, 0, len(entries))
	for _, e := range entries {
		out = append(out, resourceEntryView{
			Name: e.Name, State: string(e.State),
			LocalSize: e.LocalSize, ServerSize: e.ServerSize,
			LocalSHA: e.LocalSHA, ServerSHA: e.ServerSHA,
			InUse: e.InUse,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"resources": out})
}

// handleRemoteResourcesSync — POST: залить на машину всё, на что ссылается её
// собранный конфиг (без деплоя самого конфига).
func (s *Server) handleRemoteResourcesSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	config, err := os.ReadFile(platform.GetRemoteConfigPathFor(s.remote.ExecDir, id))
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error": "built config does not exist yet — resources to sync are derived from it",
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	files, err := services.CollectDeployResources(s.remote.ExecDir, id, config)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := s.remote.Registry.SyncResources(id, files); err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "considered": len(files)})
}

// handleRemoteResourceByName — GET (содержимое с машины) / PUT (залить
// локальный файл машины) / DELETE (удалить на машине).
//
// PUT намеренно без тела: он проталкивает файл из srs/-каталога ЭТОЙ машины —
// источник правды для её рулсетов. Произвольное содержимое кладите через
// raw REST passthrough (`PUT /admin/resources/{name}`).
func (s *Server) handleRemoteResourceByName(w http.ResponseWriter, r *http.Request) {
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" || strings.Contains(name, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid resource name"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		status, body, contentType, err := s.remote.Registry.AdminDo(id,
			http.MethodGet, "/admin/resources/"+name+"/content", nil, "")
		if err != nil {
			writeRemoteError(w, err)
			return
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write(body)

	case http.MethodPut:
		if err := s.remote.Registry.UploadResource(id, name); err != nil {
			if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
				writeJSON(w, http.StatusNotFound, map[string]any{
					"error": err.Error(),
					"hint":  "PUT pushes the machine's LOCAL srs file; for arbitrary content use raw REST (PUT /admin/resources/{name})",
				})
				return
			}
			writeRemoteError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case http.MethodDelete:
		if err := s.remote.Registry.DeleteRemoteResource(id, name); err != nil {
			writeRemoteError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET, PUT or DELETE required"})
	}
}

// handleRemoteResourceDownload — POST: забрать файл с машины в её локальный
// srs/-каталог (обратное направление к PUT).
func (s *Server) handleRemoteResourceDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	id, ok := s.remoteMachineID(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" || strings.Contains(name, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid resource name"})
		return
	}
	if err := s.remote.Registry.DownloadResource(id, name); err != nil {
		writeRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
