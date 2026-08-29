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

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
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
//   - Идём по `state.Sources` (только subscription, enabled, URL ≠ "");
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
	for _, src := range s.Sources {
		if src.Kind == state.SourceTypeSubscription && src.Enabled && src.URL != "" {
			enabledCount++
		}
	}

	ac := GetController()
	progress := func(p float64, msg string) {
		if ac != nil && ac.UIService != nil && ac.UIService.UpdateParserProgressFunc != nil {
			ac.UIService.UpdateParserProgressFunc(p, msg)
		}
	}

	// Настройки приложения — дефолт капа max_nodes (SPEC 118 Т1); читаются
	// один раз на весь sweep, а не на каждую подписку.
	settings := locale.LoadSettings(platform.GetBinDir(execDir))

	// Фан-аут по подпискам остаётся последовательным (memory:
	// subscription_scale_fanout) — параллельный fetch сотен подписок
	// стампедил бы сетевой стек.
	idx := 0
	for i := range s.Sources {
		src := &s.Sources[i]
		if src.Kind != state.SourceTypeSubscription || !src.Enabled || src.URL == "" {
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

		if refreshOneSubscriptionSource(src, s.Defaults, settings, subsDir) {
			dirty = true
		}
		// Прежний GCDisabledNodes-проход (SPEC 094 D4) здесь умер: карта
		// выключенных теперь согласуется с каноном node.enabled внутри
		// merge на том же пути достоверного обновления
		// (state.syncLegacyDisabledMap); TTL-механика целиком умирает в W5.
	}

	// Lazy GC: known set = ОБЪЕДИНЕНИЕ Source.ID'ов из всех state'ов ЛОКАЛЬНОЙ
	// машины (active state.json + named snapshots). `.raw` файл шарится между
	// stages если Source с тем же ID присутствует в нескольких — удаляем
	// только когда ID не упомянут НИГДЕ. Это защищает от случая «Update
	// активного state'а сносит данные неактивного stage'а».
	//
	// SPEC 098: и множество, и каталог — локальные. У удалённых машин свои
	// каталоги тел подписок внутри их директорий.
	knownIDs := collectAllStageSourceIDs(execDir, constants.ConfigTargetLocal, "")
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

// collectAllStageSourceIDs возвращает объединение Source.ID'ов из state-файлов
// ОДНОЙ машины (её active state.json + её named snapshots).
//
// SPEC 052 phase 8 fix: <subscriptions>/<id>.raw шарится между stages,
// если Source с тем же ID есть в нескольких state-файлах. DeleteOrphans
// должен сравнивать с union ID'ов всех stage'ов, а не только active —
// иначе Update активного state'а удалит .raw файлы, нужные другому
// (неактивному) stage'у.
//
// SPEC 098: union считается В ГРАНИЦАХ МАШИНЫ, потому что каталог тел
// подписок теперь тоже её собственный:
//
//	local          → bin/wizard_states/*.json  → bin/subscriptions/
//	remote + <id>  → …/remote/<id>/*.json      → …/remote/<id>/subscriptions/
//
// До SPEC 098 каталог был общим, и функция обязана была обходить все уровни
// wizard_states/ — иначе Update одной машины сносил тело подписки, которым
// владеет другая. С раздельными каталогами обход чужих состояний стал
// вредным: он удерживал бы от удаления .raw, уже никем в этой машине не
// упомянутый.
//
// Read-only: errors per-file логируются и пропускаются (битый файл одного
// snapshot'а не должен блокировать GC).
func collectAllStageSourceIDs(execDir, target, machineID string) []string {
	statesDir := platform.GetWizardStatesDirFor(execDir, target, machineID)
	entries, err := os.ReadDir(statesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		debuglog.WarnLog("collectAllStageSourceIDs: readdir %s: %v", statesDir, err)
		return nil
	}

	idSet := make(map[string]struct{})
	collectFromState := func(path string) {
		s, loadErr := state.Load(path)
		if loadErr != nil {
			debuglog.DebugLog("collectAllStageSourceIDs: skip %s: %v", path, loadErr)
			return
		}
		for _, src := range s.Sources {
			if src.ID != "" {
				idSet[src.ID] = struct{}{}
			}
		}
	}

	// SPEC 098: как и collectAllStageRuleSetTags — сканируется только свой
	// уровень. Поддиректории (для local это папки машин) пропускаются:
	// каталог тел подписок у каждой машины свой, и чужие состояния не должны
	// влиять на её GC ни в одну сторону.
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		collectFromState(filepath.Join(statesDir, e.Name()))
	}

	out := make([]string, 0, len(idSet))
	for id := range idSet {
		out = append(out, id)
	}
	return out
}

// refreshOneSubscriptionSource — атомарный fetch одного source: скачать →
// SubMeta из заголовков → ParseSubscriptionBody → MergeSubscriptionNodes →
// updateStatus (SPEC 118 W3, PLAN §3.1). Возвращает true если что-то
// изменилось (caller должен сохранить state).
//
// Пишет ТОЛЬКО новые slice/pointer'ы (src.Nodes, src.Meta, src.UpdateStatus
// заменяются, не мутируются): UI-пути RefreshSourceInPlace работают со
// value-snapshot'ом источника, и мутация разделяемых объектов утекала бы в
// модель мимо UI-thread.
//
// На failed fetch / недостоверный разбор: nodes[] НЕ трогаются (SPEC 113-A),
// старый .raw остаётся, ошибка — в updateStatus (и мостовую Meta).
// На success: write .raw atomic (мост до W5 — сборка пока читает его),
// merge в nodes[], meta полностью.
func refreshOneSubscriptionSource(src *state.Source, defaults state.Defaults, settings locale.Settings, subsDir string) bool {
	if src == nil || src.Kind != state.SourceTypeSubscription || src.URL == "" {
		return false
	}
	now := time.Now().UTC().Format(time.RFC3339)

	res, fetchErr := subscription.FetchSubscriptionWithMeta(src.URL)

	if fetchErr != nil {
		// Мостовая Meta (TEMPORARY BRIDGE, UI читает её до W6) — новая копия,
		// не мутация разделяемого указателя.
		meta := state.SubscriptionMeta{}
		if src.Meta != nil {
			meta = *src.Meta
		}
		meta.URLAtFetch = src.URL
		meta.LastFetchedAt = now
		meta.LastStatus = "err"
		meta.ErrorCount++
		meta.LastErrorMsg = fetchErr.Error()
		// SPEC 061: surface the structured announce on either error variant
		// so UI can render an actionable dialog with the provider message +
		// clickable URL, not just a flat error label.
		meta.ProviderAnnounce = nil
		meta.LastErrorURL = ""
		if ae, ok := subscription.IsAnnounceError(fetchErr); ok {
			a := ae.Announce
			meta.ProviderAnnounce = &a
			meta.LastErrorURL = a.URL
		}
		if httpErr, ok := subscription.IsHTTPError(fetchErr); ok {
			meta.HTTPStatusCode = httpErr.StatusCode
			if httpErr.Announce != nil && !httpErr.Announce.IsEmpty() {
				meta.ProviderAnnounce = httpErr.Announce
				meta.LastErrorURL = httpErr.Announce.URL
			}
		} else if res != nil {
			meta.HTTPStatusCode = res.HTTPStatus
		}
		src.Meta = &meta
		// Канонический статус: nodes[] не тронуты вообще (SPEC 113-A).
		src.UpdateStatus = failedSubStatus(src.UpdateStatus, src.URL, now, fetchErr, meta.HTTPStatusCode, meta.LastErrorURL)
		debuglog.WarnLog("refreshOneSubscriptionSource: source %s fetch failed: %v", src.ID, fetchErr)
		return true
	}

	// Единственное место разбора тела подписки (SPEC Т3): кап резолвится
	// «настройка подписки → дефолт настроек приложения», аварийный потолок
	// 3000 клэмпится внутри парсера.
	capN := resolveSubscriptionMaxNodes(src.MaxNodes, settings, defaults)
	material, matErr := config.MaterializeSubscriptionBody(src.ID, res.Body, src.Skip, capN)

	// Достоверность (SPEC 113-A): обрыв разбора — недостоверен. Тело, из
	// которого не родилось НИ ОДНОЙ записи при наличии per-record деградаций,
	// тоже: так выглядит HTML-страница вместо подписки, и удалить по ней все
	// узлы значило бы потерять состояние из-за мусорного ответа. Ноль записей
	// БЕЗ деградаций — легально (пользователь skip'ом отсёк всё): merge
	// честно удалит исчезнувших. material == nil при nil-ошибке парсера —
	// дефект контракта ниже по стеку: тоже недостоверно (фикс ревью W3 —
	// прежняя форма выражения разыменовывала бы nil в ветке trusted).
	trusted := matErr == nil && material != nil && (len(material.Nodes) > 0 || len(material.Warnings) == 0)

	var mergeWarns []string
	if trusted {
		// Raw-кэш — TEMPORARY BRIDGE (SPEC 118 W3-W4, умирает в W5): сборка
		// читает его, пока эмиссия не переехала на nodes[]. Пишется ТОЛЬКО
		// после trust-вердикта (фикс ревью W3, симметрия 113-A на мостовую
		// эпоху): недостоверный ответ не трогает ни nodes[], ни .raw — иначе
		// легаси-путь сборки терял бы узлы, которые канон сохранил.
		//
		// Известное окно расхождения .raw↔канон (не чиним, мост умирает в
		// W5): .raw уже записан, а Save state.json у вызывающего упал →
		// канонические nodes[] отстают от .raw до следующего достоверного
		// fetch. Обе стороны при этом самодостаточны, следующий успешный
		// fetch их выравнивает.
		if writeErr := state.WriteRawBody(subsDir, src.ID, res.RawBody); writeErr != nil {
			debuglog.WarnLog("refreshOneSubscriptionSource: WriteRawBody for %s: %v", src.ID, writeErr)
		}
		_, mergeWarns = state.MergeSubscriptionNodes(src, &state.SubFetchMaterial{
			Nodes:     material.Nodes,
			Truncated: material.Truncated,
		}, true)
		src.UpdateStatus = successSubStatus(src.URL, now, res.HTTPStatus, res.RawBodyBytes, material, mergeWarns)
	} else {
		reason := matErr
		if reason == nil {
			if material == nil {
				reason = fmt.Errorf("subscription parser returned no material")
			} else {
				reason = fmt.Errorf("subscription body yielded no nodes (%d record(s) degraded)", len(material.Warnings))
			}
		}
		st := failedSubStatus(src.UpdateStatus, src.URL, now, reason, res.HTTPStatus, "")
		if material != nil {
			// Причины «почему не родилось» — пользователю в диагностику.
			for _, w := range material.Warnings {
				st.Warnings = append(st.Warnings, state.FetchWarning{Kind: "parse", Message: w})
			}
		}
		src.UpdateStatus = st
		debuglog.WarnLog("refreshOneSubscriptionSource: source %s body untrusted: %v — nodes kept", src.ID, reason)
	}

	// Мостовая Meta успеха (TEMPORARY BRIDGE, UI читает её до W6).
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
	// Реальный truncated от парсера, а не прежняя оценка «счёт строк > кап».
	merged.Truncated = material != nil && material.Truncated

	src.Meta = &merged
	return true
}

// resolveSubscriptionMaxNodes — резолв капа принятых узлов (SPEC Т1,
// двухступенчатый — провайдерского заголовка у max_nodes не существует):
// настройка подписки → дефолт настроек приложения → аварийный
// потолок-константа (клэмп 3000 внутри ParseSubscriptionBody).
func resolveSubscriptionMaxNodes(subMax int, settings locale.Settings, defaults state.Defaults) int {
	if subMax > 0 {
		return subMax
	}
	if settings.DefaultSubscriptionMaxNodes > 0 {
		return settings.DefaultSubscriptionMaxNodes
	}
	// TEMPORARY BRIDGE (SPEC 118 W3-W4), удаляется в W5: до включения сноса
	// миграции (шаг 8) прежние defaults ещё не переехали в настройки
	// приложения — без этой ступени пользователь с defaults.max_nodes в
	// state.json потерял бы свой кап до W5.
	if defaults.MaxNodes > 0 {
		return defaults.MaxNodes
	}
	return 0 // → configtypes.MaxNodesPerSubscription в парсере
}

// failedSubStatus — канонический updateStatus неудачной попытки: ошибка и
// счётчики свежие, память о последнем успехе (даты, счёт узлов, warnings
// живых nodes[]) сохраняется — отчёт сборки и UI продолжают видеть
// диагностику материала, на котором подписка реально живёт.
func failedSubStatus(prev *state.SubUpdateStatus, url, now string, err error, httpCode int, errURL string) *state.SubUpdateStatus {
	st := &state.SubUpdateStatus{
		URLAtFetch:     url,
		LastAttemptAt:  now,
		LastStatus:     "err",
		ErrorCount:     1,
		LastErrorMsg:   err.Error(),
		LastErrorURL:   errURL,
		HTTPStatusCode: httpCode,
	}
	if prev != nil {
		st.ErrorCount = prev.ErrorCount + 1
		st.LastSuccessAt = prev.LastSuccessAt
		st.RawBodyBytes = prev.RawBodyBytes
		st.NodesCountFetched = prev.NodesCountFetched
		st.Truncated = prev.Truncated
		st.Warnings = append([]state.FetchWarning(nil), prev.Warnings...)
	}
	return st
}

// successSubStatus — канонический updateStatus успешного fetch+merge:
// per-record деградации парсера и merge персистятся здесь — отчёт сборки и
// UI читают их из состояния, ничего не перепарсивая (jsontab-К4).
func successSubStatus(url, now string, httpStatus int, rawBytes int64, material *config.SubscriptionFetchMaterial, mergeWarns []string) *state.SubUpdateStatus {
	st := &state.SubUpdateStatus{
		URLAtFetch:     url,
		LastAttemptAt:  now,
		LastSuccessAt:  now,
		LastStatus:     "ok",
		HTTPStatusCode: httpStatus,
		RawBodyBytes:   rawBytes,
	}
	if material != nil {
		st.NodesCountFetched = len(material.Nodes)
		st.Truncated = material.Truncated
		for _, w := range material.Warnings {
			st.Warnings = append(st.Warnings, state.FetchWarning{Kind: "parse", Message: w})
		}
	}
	for _, w := range mergeWarns {
		st.Warnings = append(st.Warnings, state.FetchWarning{Kind: "merge", Message: w})
	}
	return st
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
	if src.Kind != state.SourceTypeSubscription {
		return false, fmt.Errorf("source %s is not a subscription (type=%q)", src.ID, src.Kind)
	}
	if src.URL == "" {
		return false, fmt.Errorf("source %s has empty URL", src.ID)
	}
	execDir := svc.ac.FileService.ExecDir
	subsDir := platform.GetSubscriptionsDir(execDir)

	// Defaults капа: настройки приложения (SPEC Т1) + мостовые defaults из
	// state.json, если он есть (TEMPORARY BRIDGE до W5) — нормально для
	// cold-start, когда нет ни того, ни другого: парсер клэмпит потолком.
	settings := locale.LoadSettings(platform.GetBinDir(execDir))
	var defaults state.Defaults
	if s, err := state.Load(platform.GetWizardStatePath(execDir)); err == nil {
		defaults = s.Defaults
	}

	changed := refreshOneSubscriptionSource(src, defaults, settings, subsDir)
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
	if src.Kind != state.SourceTypeSubscription {
		return nil, fmt.Errorf("source %s is not a subscription (type=%q)", sourceID, src.Kind)
	}

	subsDir := platform.GetSubscriptionsDir(execDir)
	settings := locale.LoadSettings(platform.GetBinDir(execDir))
	dirty := refreshOneSubscriptionSource(src, s.Defaults, settings, subsDir)
	if dirty {
		if err := s.Save(statePath); err != nil {
			return src, fmt.Errorf("save state after refresh: %w", err)
		}
		// «Конфиг устарел» (SPEC Т4): fetch-merge поднимает признак, но
		// НИКОГДА не запускает пересборку сам — это решение пользователя
		// (Rebuild рядом либо AutoRebuildOnChange).
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
		// SPEC 094 A5: узел-группа не соединение — server/port у него нет, и
		// общий формат дал бы бессмысленное «group://:0#tag». Показываем
		// состав: это единственное, что о группе стоит знать в превью.
		if node.Scheme == configtypes.SchemeGroup {
			out = append(out, formatGroupPreview(node))
			continue
		}
		// URI-like preview: `<scheme>://<server>:<port>#<tag>` (~50-150 байт).
		// Server/Port дают связь с реальной нодой, tag — human-readable label.
		// UUID/Flow намеренно не включаем — это секреты, в preview не место.
		out = append(out, fmt.Sprintf("%s://%s:%d#%s", node.Scheme, node.Server, node.Port, node.Tag))
	}
	return out, total
}

// formatGroupPreview описывает узел-группу одной строкой превью.
//
// Формат «group://<type>?<параметры>#<tag>» держится общей формы соседних
// строк (schema://…#tag), но вместо адреса несёт всё, что у группы есть:
// размер пула, цель и период проверки, выбор по умолчанию. Иначе по снимку не
// понять, чем одна группа отличается от другой.
//
// Состав перечисляется тегами: без него не видно, ЧТО именно в пуле, а это
// главный вопрос к группе. Список ограничен — пул провайдера бывает на 15+
// узлов, и полный перечень раздул бы строку до нечитаемости.
func formatGroupPreview(node *configtypes.ParsedNode) string {
	groupType := "group"
	params := make([]string, 0, 5)

	if node.Outbound != nil {
		if t, ok := node.Outbound["type"].(string); ok && t != "" {
			groupType = t
		}

		members := groupPreviewMembers(node.Outbound)
		params = append(params, fmt.Sprintf("members=%d", len(members)))

		// Каждый член отдельным ключом «outbounds[]=»: тег провайдера может
		// содержать запятую («🇩🇪 Berlin, DE»), и склейка через разделитель
		// сделала бы список неразбираемым — граница между тегами исчезает.
		//
		// Состав пишется ЦЕЛИКОМ. Соблазн урезать его понятен — SPEC 054
		// боролся с раздутым state.json, — но цена измерена: пул на 50 узлов
		// со всеми членами занимает ~1.7 КБ, то есть 0.6% от порога в 256 КБ.
		// Там раздувала одна строка на 983 КБ, а не сотня байт; экономить
		// здесь значит показывать пользователю неполный состав без причины.
		for _, tag := range members {
			params = append(params, "outbounds[]="+tag)
		}

		// Параметры проверки — по ним видно, как группа выбирает узел.
		for _, key := range []string{"url", "interval", "tolerance", "default"} {
			if v, ok := node.Outbound[key]; ok && v != nil {
				params = append(params, fmt.Sprintf("%s=%v", key, v))
			}
		}
	}

	return fmt.Sprintf("group://%s?%s#%s", groupType, strings.Join(params, "&"), node.Tag)
}

// groupPreviewMembers достаёт теги членов группы.
func groupPreviewMembers(outbound map[string]interface{}) []string {
	raw, ok := outbound[configtypes.GroupMembersKey].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if tag, ok := item.(string); ok && tag != "" {
			out = append(out, tag)
		}
	}
	return out
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
