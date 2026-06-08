package core

// config_service_subscriptions.go — SPEC 070 split из config_service.go (pure move).
// Subscription fetch / refresh пайплайн: per-source meta+raw-cache sweep,
// single-source refresh (in-place и через state.json), orphan GC, preview-node
// извлечение.
//
// **Lock boundaries сохранены ровно как были** в config_service.go:
//   - refreshSubscriptionsMetaAndCache НЕ берёт SubscriptionMu сам — его держит
//     caller (UpdateConfigFromSubscriptions, остался в config_service.go);
//   - RefreshSingleSubscription держит SubscriptionMu на весь load→mutate→save;
//   - RefreshSourceInPlace намеренно НЕ берёт SubscriptionMu (не пишет state.json).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// refreshSubscriptionsMetaAndCache — SPEC 052 phase 5: per-source HTTP fetch
// → парсинг metadata (headers + inline #-comments) → запись raw body в
// `bin/subscriptions/<id>.raw`, заполнение `Source.Meta`.
//
// **Concurrency**: caller (`UpdateConfigFromSubscriptions`) держит
// `ac.SubscriptionMu` на весь load→mutate→save цикл, чтобы конкурентные
// per-source Refresh'и из UI не теряли изменения этой sweep'а. См. SPEC 052
// phase 8 race-fix.
//
// Поведение:
//   - Идём по `state.Connections.Sources` (только subscription, enabled, URL ≠ "");
//   - На success: атомарная запись raw + обновлённая Meta (headers, last_status="ok",
//     error_count=0, last_fetched_at, http_status_code, raw_body_bytes,
//     preview_nodes[:50], nodes_count_fetched, truncated);
//   - На failure: keep старого raw (per-source resilience), Meta.error_count++,
//     last_status="err", last_error_msg, http_status_code (если был ответ);
//   - После всех источников — DeleteOrphans: убираем `.raw` файлы id'ов
//     которых больше нет в state;
//   - Persist state.json через `state.Save` (atomic).
func refreshSubscriptionsMetaAndCache(s *state.State, execDir string) {
	if s == nil {
		return
	}
	subsDir := platform.GetSubscriptionsDir(execDir)

	dirty := false

	// Считаем enabled subscriptions для progress reporting.
	enabledCount := 0
	for _, src := range s.Connections.Sources {
		if src.Type == state.SourceTypeSubscription && src.Enabled && src.URL != "" {
			enabledCount++
		}
	}

	ac := GetController()
	progress := func(p float64, msg string) {
		if ac != nil && ac.UIService != nil && ac.UIService.UpdateParserProgressFunc != nil {
			ac.UIService.UpdateParserProgressFunc(p, msg)
		}
	}

	idx := 0
	for i := range s.Connections.Sources {
		src := &s.Connections.Sources[i]
		if src.Type != state.SourceTypeSubscription || !src.Enabled || src.URL == "" {
			continue
		}
		idx++
		// Progress: 0..70% — fetch phase (до старого parser-pipeline'а который покрывает 70..100).
		pct := float64(idx) / float64(enabledCount) * 70.0
		shortURL := src.URL
		if len(shortURL) > 60 {
			shortURL = shortURL[:60] + "…"
		}
		progress(pct, fmt.Sprintf("Fetching %d/%d: %s", idx, enabledCount, shortURL))

		if refreshOneSubscriptionSource(src, s.Connections.Defaults, subsDir) {
			dirty = true
		}
	}

	// Lazy GC: known set = ОБЪЕДИНЕНИЕ Source.ID'ов из ВСЕХ state'ов
	// (active state.json + named snapshots). `.raw` файл шарится между
	// stages если Source с тем же ID присутствует в нескольких — удаляем
	// только когда ID не упомянут НИГДЕ. Это защищает от случая «Update
	// активного state'а сносит данные неактивного stage'а».
	knownIDs := collectAllStageSourceIDs(execDir)
	if _, gcErr := state.DeleteOrphans(subsDir, knownIDs); gcErr != nil {
		debuglog.WarnLog("refreshSubscriptionsMetaAndCache: DeleteOrphans: %v", gcErr)
	}

	// Persist state с обновлённой meta. Best-effort.
	if dirty {
		statePath := platform.GetWizardStatePath(execDir)
		if err := s.Save(statePath); err != nil {
			debuglog.WarnLog("refreshSubscriptionsMetaAndCache: state.Save: %v", err)
		}
	}
}

// collectAllStageSourceIDs возвращает объединение Source.ID'ов из ВСЕХ
// state-файлов в `bin/wizard_states/` (active state.json + named snapshots).
//
// SPEC 052 phase 8 fix: bin/subscriptions/<id>.raw шарится между stages,
// если Source с тем же ID есть в нескольких state-файлах. DeleteOrphans
// должен сравнивать с union ID'ов всех stage'ов, а не только active —
// иначе Update активного state'а удалит .raw файлы, нужные другому
// (неактивному) stage'у.
//
// Read-only: errors per-file логируются и пропускаются (битый файл одного
// snapshot'а не должен блокировать GC).
func collectAllStageSourceIDs(execDir string) []string {
	statesDir := platform.GetWizardStatesDir(execDir)
	entries, err := os.ReadDir(statesDir)
	if err != nil {
		debuglog.WarnLog("collectAllStageSourceIDs: readdir %s: %v", statesDir, err)
		return nil
	}

	idSet := make(map[string]struct{})
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(statesDir, name)
		s, loadErr := state.Load(path)
		if loadErr != nil {
			debuglog.DebugLog("collectAllStageSourceIDs: skip %s: %v", path, loadErr)
			continue
		}
		for _, src := range s.Connections.Sources {
			if src.ID != "" {
				idSet[src.ID] = struct{}{}
			}
		}
	}

	out := make([]string, 0, len(idSet))
	for id := range idSet {
		out = append(out, id)
	}
	return out
}

// refreshOneSubscriptionSource — атомарный fetch+meta+raw-cache для
// одного source. Мутирует src.Meta in-place; возвращает true если
// что-то изменилось (caller должен сохранить state).
//
// На failed fetch: keep старый .raw, error_count++, last_status="err".
// На success: write .raw atomic, fill meta полностью.
func refreshOneSubscriptionSource(src *state.Source, defaults state.Defaults, subsDir string) bool {
	if src == nil || src.Type != state.SourceTypeSubscription || src.URL == "" {
		return false
	}
	now := time.Now().UTC().Format(time.RFC3339)

	res, fetchErr := subscription.FetchSubscriptionWithMeta(src.URL)
	if src.Meta == nil {
		src.Meta = &state.SubscriptionMeta{}
	}

	if fetchErr != nil {
		src.Meta.URLAtFetch = src.URL
		src.Meta.LastFetchedAt = now
		src.Meta.LastStatus = "err"
		src.Meta.ErrorCount++
		src.Meta.LastErrorMsg = fetchErr.Error()
		// SPEC 061: surface the structured announce on either error variant
		// so UI can render an actionable dialog with the provider message +
		// clickable URL, not just a flat error label.
		src.Meta.ProviderAnnounce = nil
		src.Meta.LastErrorURL = ""
		if ae, ok := subscription.IsAnnounceError(fetchErr); ok {
			a := ae.Announce
			src.Meta.ProviderAnnounce = &a
			src.Meta.LastErrorURL = a.URL
		}
		if httpErr, ok := subscription.IsHTTPError(fetchErr); ok {
			src.Meta.HTTPStatusCode = httpErr.StatusCode
			if httpErr.Announce != nil && !httpErr.Announce.IsEmpty() {
				src.Meta.ProviderAnnounce = httpErr.Announce
				src.Meta.LastErrorURL = httpErr.Announce.URL
			}
		} else if res != nil {
			src.Meta.HTTPStatusCode = res.HTTPStatus
		}
		debuglog.WarnLog("refreshOneSubscriptionSource: source %s fetch failed: %v", src.ID, fetchErr)
		return true
	}

	if writeErr := state.WriteRawBody(subsDir, src.ID, res.RawBody); writeErr != nil {
		debuglog.WarnLog("refreshOneSubscriptionSource: WriteRawBody for %s: %v", src.ID, writeErr)
	}

	merged := res.Meta // value-copy
	merged.URLAtFetch = src.URL
	merged.LastFetchedAt = now
	merged.LastStatus = "ok"
	merged.ErrorCount = 0
	merged.LastErrorMsg = ""
	merged.LastErrorURL = ""
	merged.HTTPStatusCode = res.HTTPStatus
	merged.RawBodyBytes = res.RawBodyBytes
	// ProviderAnnounce on success — only when the provider actually sent
	// announce headers (already populated by ParseHeaders / ParseInlineComments
	// into res.Meta). Otherwise stays nil so UI clears the 📢 badge.
	// SPEC 054: для Xray JSON array подписок line-based extractPreviewNodes
	// раздувал preview_nodes в 50 раз (одна "line" = весь JSON body ~1MB).
	// Сначала пробуем формат-aware path через xray JSON parser; fallback на
	// line-based для base64/text-line подписок.
	if subscription.IsXrayJSONArrayBody(string(res.Body)) {
		merged.PreviewNodes, merged.NodesCountFetched = extractXrayJSONPreviewNodes(res.Body, 50)
	} else {
		merged.PreviewNodes = extractPreviewNodes(res.Body, 50)
		merged.NodesCountFetched = countURIs(res.Body)
	}

	effectiveMax := src.MaxNodes
	if effectiveMax == 0 {
		effectiveMax = defaults.MaxNodes
	}
	if effectiveMax == 0 {
		effectiveMax = state.DefaultMaxNodes
	}
	merged.Truncated = merged.NodesCountFetched > effectiveMax

	src.Meta = &merged
	return true
}

// RefreshSourceInPlace — SPEC 052 phase 7 cold-start path: fetch+raw+meta для
// одного source, переданного по pointer'у из in-memory wizard model. Не делает
// state.Load и не пишет state.json — caller (Wizard) сам решает, когда
// persist'ить через свой Save flow. Это даёт корректный UX в трёх сценариях:
//
//  1. Cold start, state.json ещё нет (свежая инсталляция, шаблон с дефолтными
//     URL'ами в model). Refresh должен работать без принуждения к Save.
//  2. Существующий state, пользователь добавил новый URL и сразу кликнул
//     Refresh — fetch на in-memory URL, не на старый из state.json.
//  3. Пользователь редактирует URL существующего source и кликает Refresh —
//     то же самое, актуальный URL побеждает.
//
// Что трогаем на диске: только bin/subscriptions/<id>.raw (atomic .tmp+Rename).
// Это per-source файл, конфликта с state.json нет.
//
// Concurrency: SubscriptionMu НЕ берётся — мы не модифицируем state.json. Если
// одновременно сработает heartbeat / manual Update, они работают со state.json
// со своей версией Source — наш in-memory pointer им не виден. UI button-state
// блокирует двойной клик по той же row.
//
// Возвращает (changed, err): changed=true если src.Meta изменился (caller
// должен пере-рендерить row); err — fetch/write ошибки.
func (svc *ConfigService) RefreshSourceInPlace(src *state.Source) (bool, error) {
	if src == nil {
		return false, fmt.Errorf("RefreshSourceInPlace: nil source")
	}
	if src.Type != state.SourceTypeSubscription {
		return false, fmt.Errorf("source %s is not a subscription (type=%q)", src.ID, src.Type)
	}
	if src.URL == "" {
		return false, fmt.Errorf("source %s has empty URL", src.ID)
	}
	execDir := svc.ac.FileService.ExecDir
	subsDir := platform.GetSubscriptionsDir(execDir)

	// Defaults для MaxNodes truncation: пытаемся прочитать из state.json,
	// если он есть. Иначе refreshOneSubscriptionSource fallback'нется на
	// state.DefaultMaxNodes — нормально для cold-start.
	var defaults state.Defaults
	if s, err := state.Load(platform.GetWizardStatePath(execDir)); err == nil {
		defaults = s.Connections.Defaults
	}

	changed := refreshOneSubscriptionSource(src, defaults, subsDir)
	return changed, nil
}

// RefreshSingleSubscription — SPEC 052 phase 7: per-source manual refresh,
// триггеренный из UI (кнопка Refresh per row). Делает fetch+meta+raw для
// одного source, обновляет state.json (atomic).
//
// Не запускает Rebuild — это решение пользователя (Rebuild button рядом
// либо AutoRebuildOnChange). Не трогает другие source'ы.
//
// Возвращает обновлённый Source (его Meta) для отображения в UI без
// повторного Load.
func (svc *ConfigService) RefreshSingleSubscription(sourceID string) (*state.Source, error) {
	if sourceID == "" {
		return nil, fmt.Errorf("RefreshSingleSubscription: empty source id")
	}
	execDir := svc.ac.FileService.ExecDir
	statePath := platform.GetWizardStatePath(execDir)

	// SPEC 052 phase 8 race-fix: load+mutate+save сериализуем через
	// SubscriptionMu — параллельный heartbeat/manual Update обновляющий
	// другие source'ы не должен потеряться от этой single-source save'ы.
	svc.ac.SubscriptionMu.Lock()
	defer svc.ac.SubscriptionMu.Unlock()

	s, err := state.Load(statePath)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	src := s.FindSource(sourceID)
	if src == nil {
		return nil, fmt.Errorf("source not found: %s", sourceID)
	}
	if src.Type != state.SourceTypeSubscription {
		return nil, fmt.Errorf("source %s is not a subscription (type=%q)", sourceID, src.Type)
	}

	subsDir := platform.GetSubscriptionsDir(execDir)
	dirty := refreshOneSubscriptionSource(src, s.Connections.Defaults, subsDir)
	if dirty {
		if err := s.Save(statePath); err != nil {
			return src, fmt.Errorf("save state after refresh: %w", err)
		}
		// Mark cache stale так, чтобы Rebuild подхватил свежий .raw,
		// и mark config stale — UI должен напомнить про Rebuild/Restart.
		if svc.ac.StateService != nil {
			svc.ac.StateService.MarkConfigStale()
		}
	}
	return src, nil
}

// extractXrayJSONPreviewNodes — SPEC 054. Для Xray JSON array подписок:
// парсит body через subscription.ParseNodesFromXrayJSONArray и эмитит первые
// `limit` нод в URI-like формате `<scheme>://<server>:<port>#<tag>`.
//
// Возвращает (previewNodes, totalCount). totalCount — реальное количество
// нод в JSON array (для meta.nodes_count_fetched).
//
// На parse-error → возвращает (nil, 0) — caller должен решить fallback (но
// caller сначала вызывает IsXrayJSONArrayBody, так что path должен совпадать).
func extractXrayJSONPreviewNodes(body []byte, limit int) ([]string, int) {
	nodes, err := subscription.ParseNodesFromXrayJSONArray(string(body), nil)
	if err != nil {
		debuglog.WarnLog("extractXrayJSONPreviewNodes: parse failed: %v", err)
		return nil, 0
	}
	total := len(nodes)
	if total == 0 {
		return nil, 0
	}
	n := limit
	if n > total {
		n = total
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		node := nodes[i]
		if node == nil {
			continue
		}
		// URI-like preview: `<scheme>://<server>:<port>#<tag>` (~50-150 байт).
		// Server/Port дают связь с реальной нодой, tag — human-readable label.
		// UUID/Flow намеренно не включаем — это секреты, в preview не место.
		out = append(out, fmt.Sprintf("%s://%s:%d#%s", node.Scheme, node.Server, node.Port, node.Tag))
	}
	return out, total
}

// extractPreviewNodes — первые `limit` URI-like строк из decoded body.
// «URI-like» = содержит "://", не пустая, не комментарий.
func extractPreviewNodes(body []byte, limit int) []string {
	if len(body) == 0 || limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	lines := strings.Split(string(body), "\n")
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		if !strings.Contains(ln, "://") {
			continue
		}
		out = append(out, ln)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// countURIs — общее число URI-like строк (не нодовый-парсинг, грубая оценка
// для meta.nodes_count_fetched).
func countURIs(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	n := 0
	for _, ln := range strings.Split(string(body), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		if strings.Contains(ln, "://") {
			n++
		}
	}
	return n
}
