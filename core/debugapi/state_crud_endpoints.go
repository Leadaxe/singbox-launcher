// Package debugapi — SPEC 160: правка состояния по одной записи, как кнопки
// Конфигуратора «добавить/удалить».
//
//	GET    /state/servers               — корневые серверы и члены папок
//	POST   /state/servers               — {input, folder?, tag?} → узлы из ссылок/.conf/vpn:///JSON
//	DELETE /state/servers?tag=&folder=  — узел + NodeLink на него + include Направлений
//	POST   /state/rules                 — одна запись правила v8
//	DELETE /state/rules?num=|name=|ref= — ровно одно правило
//	POST   /state/dns/servers           — DNS-сервер kind=user
//	DELETE /state/dns/servers?tag=      — DNS-сервер kind=user (перенос dns_final)
//	POST   /state/dns/rules             — {rule} → DNS-правило kind=user в конец
//	DELETE /state/dns/rules?index=      — DNS-правило kind=user по индексу dns.rules
//
// Смысл операций живёт в core/stateedit; здесь — HTTP-обвязка общего вида:
// гейт мажора схемы → разбор тела → мьютекс → load → операция → нормы записи
// → save → пересборка config.json у локального состояния (как POST
// /backup/import). Идентификаторы DELETE — query-параметрами: маршруты с
// `{…}` на Win7-сборке (go1.20) недостижимы.
//
// Ошибки: 400 — тело/query не разобраны, 404 — цели нет, 409 — схема файла
// новее или селектор неоднозначен, 422 — запрос по смыслу неприменим (с
// `field`).
package debugapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/core/state"
	"singbox-launcher/core/stateedit"
	"singbox-launcher/core/template"
)

// writeStateEditError — отказ операции stateedit в коде ответа.
func writeStateEditError(w http.ResponseWriter, err error) {
	var fe *stateedit.FieldError
	var ae *stateedit.AmbiguousError
	switch {
	case errors.As(err, &fe):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": fe.Msg, "field": fe.Field})
	case errors.As(err, &ae):
		writeJSON(w, http.StatusConflict, map[string]any{"error": ae.Msg, "nums": ae.Nums})
	case errors.Is(err, stateedit.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
}

// editTemplate — шаблон для проверок операции; nil, если не прочитался
// (stateedit тогда проверяет тем, что знает состояние).
func (s *Server) editTemplate() *template.TemplateData {
	td, err := s.facade.LoadTemplate()
	if err != nil {
		return nil
	}
	return td
}

// commitStateEdit — общий хвост мутации: нормы записи (SPEC 129), save и у
// локального состояния пересборка config.json. Ошибка пересборки правку не
// отменяет (она уже на диске) и едет отдельным полем — иначе агент решил бы,
// что состояние не изменилось, и повторил вызов. false — ответ 500 уже
// написан; при true out дополнен `ok` и полями пересборки, пишет вызывающий.
func (s *Server) commitStateEdit(w http.ResponseWriter, acc stateAccess, st *state.State, out map[string]any) bool {
	state.ApplyRecordVars(st, s.recordVarDeclsFor(st))
	if err := acc.save(st); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "save state: " + err.Error()})
		return false
	}
	out["ok"] = true
	out["config_rebuilt"] = false
	if acc.local {
		if err := s.facade.RebuildConfigIfDirty(); err != nil {
			out["config_rebuild_error"] = err.Error()
		} else {
			out["config_rebuilt"] = true
		}
	}
	return true
}

// loadForEdit — load под уже взятым мьютексом; false — ответ уже написан.
func loadForEdit(w http.ResponseWriter, acc stateAccess) (*state.State, bool) {
	st, err := acc.load()
	if err != nil {
		writeJSON(w, stateErrStatus(err), map[string]any{"error": "load state: " + err.Error()})
		return nil, false
	}
	return st, true
}

// ── /state/servers ───────────────────────────────────────────────

// addServersReq — тело POST /state/servers.
type addServersReq struct {
	Input  string `json:"input"`
	Folder string `json:"folder"`
	Tag    string `json:"tag"`
}

func (s *Server) handleStateServers(w http.ResponseWriter, r *http.Request) {
	s.stateServersWith(w, r, s.localStateAccess())
}

func (s *Server) stateServersWith(w http.ResponseWriter, r *http.Request, acc stateAccess) {
	switch r.Method {
	case http.MethodGet:
		st, err := acc.load()
		if err != nil {
			writeJSON(w, stateErrStatus(err), map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"servers": stateedit.ListServers(st)})

	case http.MethodPost:
		if !guardStateSchema(w, acc) {
			return
		}
		var req addServersReq
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		acc.mu.Lock()
		defer acc.mu.Unlock()
		st, ok := loadForEdit(w, acc)
		if !ok {
			return
		}
		res, err := stateedit.AddServers(st, s.editTemplate(), stateedit.AddServersRequest{
			Input: req.Input, Folder: req.Folder, Tag: req.Tag,
		})
		if err != nil {
			writeStateEditError(w, err)
			return
		}
		where := "root"
		if len(res.Added) > 0 && res.Added[0].FolderID != "" {
			where = "folder " + res.Added[0].FolderID
		}
		out := map[string]any{
			"diff_summary": []string{fmt.Sprintf("servers: +%d (%s), %d skipped", len(res.Added), where, len(res.Skipped))},
			"added":        res.Added,
			"skipped":      res.Skipped,
		}
		if len(res.Added) == 0 {
			// Нечего сохранять: состояние не тронуто, конфиг пересобирать незачем.
			out["ok"] = true
			out["config_rebuilt"] = false
			writeJSON(w, http.StatusOK, out)
			return
		}
		if s.commitStateEdit(w, acc, st, out) {
			writeJSON(w, http.StatusOK, out)
		}

	case http.MethodDelete:
		if !guardStateSchema(w, acc) {
			return
		}
		q := r.URL.Query()
		tag := strings.TrimSpace(q.Get("tag"))
		if tag == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "query parameter tag is required (folder optional)"})
			return
		}
		acc.mu.Lock()
		defer acc.mu.Unlock()
		st, ok := loadForEdit(w, acc)
		if !ok {
			return
		}
		res, err := stateedit.DeleteServer(st, tag, q.Get("folder"))
		if err != nil {
			writeStateEditError(w, err)
			return
		}
		where := "root"
		if res.FolderID != "" {
			where = "folder " + res.FolderID
		}
		out := map[string]any{
			"diff_summary": []string{fmt.Sprintf("servers: -1 %q (%s), %d links removed, %d direction includes removed, %d dangling",
				res.Tag, where, len(res.LinksRemoved), len(res.DirectionsUpdated), len(res.Dangling))},
			"deleted": res,
		}
		if !s.commitStateEdit(w, acc, st, out) {
			return
		}
		// Каталог состояния tailnet-узла — после сохранения (узел точно
		// удалён) и только у локального состояния: корень каталогов —
		// локальная машина, у профиля удалённой одноимённый каталог чужой.
		if acc.local && res.TailscaleStateDir != "" {
			config.RemoveTailscaleStateDir(res.TailscaleStateDir)
		}
		writeJSON(w, http.StatusOK, out)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET, POST or DELETE required"})
	}
}

// ── /state/rules (POST / DELETE) ─────────────────────────────────

// addRuleReq — тело POST /state/rules: запись правила v8. Enabled — указатель
// поверх поля записи (неглубокое поле перекрывает встроенное в encoding/json),
// чтобы отличить «не задано» (= true) от явного false.
type addRuleReq struct {
	state.Rule
	Enabled *bool `json:"enabled"`
}

func (s *Server) stateRuleAddWith(w http.ResponseWriter, r *http.Request, acc stateAccess) {
	if !guardStateSchema(w, acc) {
		return
	}
	var req addRuleReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
		return
	}
	rule := req.Rule
	rule.Enabled = req.Enabled == nil || *req.Enabled
	acc.mu.Lock()
	defer acc.mu.Unlock()
	st, ok := loadForEdit(w, acc)
	if !ok {
		return
	}
	num, err := stateedit.AddRule(st, s.editTemplate(), rule)
	if err != nil {
		writeStateEditError(w, err)
		return
	}
	name := rule.Name
	if rule.Kind == state.RuleKindPreset {
		name = rule.Ref
	}
	out := map[string]any{
		"diff_summary": []string{fmt.Sprintf("rules: +1 %s %q at num %d, %d entries", rule.Kind, name, num, len(st.Rules))},
		"num":          num,
	}
	if s.commitStateEdit(w, acc, st, out) {
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Server) stateRuleDeleteWith(w http.ResponseWriter, r *http.Request, acc stateAccess) {
	if !guardStateSchema(w, acc) {
		return
	}
	q := r.URL.Query()
	sel := stateedit.RuleSelector{Name: q.Get("name"), Ref: q.Get("ref")}
	given := 0
	if raw := q.Get("num"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "num must be an integer"})
			return
		}
		sel.Num = &n
		given++
	}
	if sel.Name != "" {
		given++
	}
	if sel.Ref != "" {
		given++
	}
	if given != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "exactly one of query parameters num, name, ref is required"})
		return
	}
	acc.mu.Lock()
	defer acc.mu.Unlock()
	st, ok := loadForEdit(w, acc)
	if !ok {
		return
	}
	removed, err := stateedit.DeleteRule(st, s.editTemplate(), sel)
	if err != nil {
		writeStateEditError(w, err)
		return
	}
	num := 0
	if removed.Num != nil {
		num = *removed.Num
	}
	name := removed.Name
	if removed.Kind == state.RuleKindPreset {
		name = removed.Ref
	}
	out := map[string]any{
		"diff_summary": []string{fmt.Sprintf("rules: -1 %s %q (num %d), %d entries", removed.Kind, name, num, len(st.Rules))},
		"deleted":      removed,
	}
	if s.commitStateEdit(w, acc, st, out) {
		writeJSON(w, http.StatusOK, out)
	}
}

// ── /state/dns/servers ───────────────────────────────────────────

// addDNSServerReq — тело POST /state/dns/servers; Enabled — как у addRuleReq.
type addDNSServerReq struct {
	state.DNSServer
	Enabled *bool `json:"enabled"`
}

func (s *Server) handleStateDNSServers(w http.ResponseWriter, r *http.Request) {
	s.stateDNSServersWith(w, r, s.localStateAccess())
}

func (s *Server) stateDNSServersWith(w http.ResponseWriter, r *http.Request, acc stateAccess) {
	switch r.Method {
	case http.MethodPost:
		if !guardStateSchema(w, acc) {
			return
		}
		var req addDNSServerReq
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
			return
		}
		srv := req.DNSServer
		srv.Enabled = req.Enabled == nil || *req.Enabled
		acc.mu.Lock()
		defer acc.mu.Unlock()
		st, ok := loadForEdit(w, acc)
		if !ok {
			return
		}
		if err := stateedit.AddDNSServer(st, s.editTemplate(), srv); err != nil {
			writeStateEditError(w, err)
			return
		}
		added := st.DNS.Servers[len(st.DNS.Servers)-1]
		out := map[string]any{
			"diff_summary": []string{fmt.Sprintf("dns.servers: +1 %q, %d entries", added.Tag, len(st.DNS.Servers))},
			"tag":          added.Tag,
		}
		if s.commitStateEdit(w, acc, st, out) {
			writeJSON(w, http.StatusOK, out)
		}

	case http.MethodDelete:
		if !guardStateSchema(w, acc) {
			return
		}
		tag := strings.TrimSpace(r.URL.Query().Get("tag"))
		if tag == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "query parameter tag is required"})
			return
		}
		acc.mu.Lock()
		defer acc.mu.Unlock()
		st, ok := loadForEdit(w, acc)
		if !ok {
			return
		}
		res, err := stateedit.DeleteDNSServer(st, tag)
		if err != nil {
			writeStateEditError(w, err)
			return
		}
		out := map[string]any{
			"diff_summary": []string{fmt.Sprintf("dns.servers: -1 %q, %d entries", res.Tag, len(st.DNS.Servers))},
			"deleted":      res,
		}
		if s.commitStateEdit(w, acc, st, out) {
			writeJSON(w, http.StatusOK, out)
		}

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST or DELETE required"})
	}
}

// ── /state/dns/rules (POST / DELETE) ─────────────────────────────

// addDNSRuleReq — тело POST /state/dns/rules: одно DNS-правило sing-box.
type addDNSRuleReq struct {
	Rule map[string]interface{} `json:"rule"`
}

func (s *Server) stateDNSRuleAddWith(w http.ResponseWriter, r *http.Request, acc stateAccess) {
	if !guardStateSchema(w, acc) {
		return
	}
	var req addDNSRuleReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body: " + err.Error()})
		return
	}
	acc.mu.Lock()
	defer acc.mu.Unlock()
	st, ok := loadForEdit(w, acc)
	if !ok {
		return
	}
	idx, err := stateedit.AddDNSRule(st, req.Rule)
	if err != nil {
		writeStateEditError(w, err)
		return
	}
	out := map[string]any{
		"diff_summary": []string{fmt.Sprintf("dns.rules: +1 at #%d, %d entries", idx, len(st.DNS.Rules))},
		"index":        idx,
	}
	if s.commitStateEdit(w, acc, st, out) {
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Server) stateDNSRuleDeleteWith(w http.ResponseWriter, r *http.Request, acc stateAccess) {
	if !guardStateSchema(w, acc) {
		return
	}
	idx, err := strconv.Atoi(r.URL.Query().Get("index"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "query parameter index (integer) is required"})
		return
	}
	acc.mu.Lock()
	defer acc.mu.Unlock()
	st, ok := loadForEdit(w, acc)
	if !ok {
		return
	}
	removed, err := stateedit.DeleteDNSRule(st, idx)
	if err != nil {
		writeStateEditError(w, err)
		return
	}
	out := map[string]any{
		"diff_summary": []string{fmt.Sprintf("dns.rules: -1 #%d, %d entries", idx, len(st.DNS.Rules))},
		"deleted":      removed,
	}
	if s.commitStateEdit(w, acc, st, out) {
		writeJSON(w, http.StatusOK, out)
	}
}
