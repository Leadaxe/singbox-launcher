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
//   - RefreshSourceInPlace качает БЕЗ мьютекса (сеть на in-memory URL), а
//     закрепление результата на диске (persistFetchResultForSource, SPEC 116
//     W13) берёт тот же SubscriptionMu на свой короткий load→mutate→save.

import (
	"fmt"
	"strings"
	"time"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
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
//   - На success: материализованные nodes[] (merge по сырому тегу),
//     обновлённые заголовки провайдера в Meta и канонический updateStatus;
//   - На failure: nodes[] не тронуты (SPEC 113-A), ошибка — в updateStatus;
//   - Persist state.json через `state.Save` (atomic).
func refreshSubscriptionsMetaAndCache(s *state.State, execDir string) {
	if s == nil {
		return
	}

	dirty := false

	// Считаем enabled subscriptions для progress reporting.
	enabledCount := 0
	for _, src := range s.Sources {
		if src.Kind == state.SourceKindSubscription && src.Enabled && src.URL != "" {
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
		if src.Kind != state.SourceKindSubscription || !src.Enabled || src.URL == "" {
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

		if refreshOneSubscriptionSource(src, settings) {
			dirty = true
		}
	}

	// Persist state с обновлённой meta. Best-effort.
	if dirty {
		statePath := platform.GetWizardStatePath(execDir)
		if err := s.Save(statePath); err != nil {
			debuglog.WarnLog("refreshSubscriptionsMetaAndCache: state.Save: %v", err)
		}
	}
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
// ошибка — в updateStatus. На success: merge в nodes[], заголовки в Meta.
func refreshOneSubscriptionSource(src *state.Source, settings locale.Settings) bool {
	if src == nil || src.Kind != state.SourceKindSubscription || src.URL == "" {
		return false
	}
	now := time.Now().UTC().Format(time.RFC3339)

	res, fetchErr := subscription.FetchSubscriptionWithMeta(src.URL)

	if fetchErr != nil {
		// Заголовки провайдера — новая копия, не мутация разделяемого
		// указателя: их читает UI со своего value-snapshot'а.
		meta := state.SubMeta{}
		if src.Meta != nil {
			meta = *src.Meta
		}
		// SPEC 061: структурированный announce нужен обеим формам ошибки —
		// UI рисует по нему диалог с текстом провайдера и кликабельным URL,
		// а не плоскую строку ошибки.
		meta.ProviderAnnounce = nil
		errURL := ""
		httpCode := 0
		if ae, ok := subscription.IsAnnounceError(fetchErr); ok {
			a := ae.Announce
			meta.ProviderAnnounce = &a
			errURL = a.URL
		}
		if httpErr, ok := subscription.IsHTTPError(fetchErr); ok {
			httpCode = httpErr.StatusCode
			if httpErr.Announce != nil && !httpErr.Announce.IsEmpty() {
				meta.ProviderAnnounce = httpErr.Announce
				errURL = httpErr.Announce.URL
			}
		} else if res != nil {
			httpCode = res.HTTPStatus
		}
		src.Meta = &meta
		// Канонический статус: nodes[] не тронуты вообще (SPEC 113-A).
		src.UpdateStatus = failedSubStatus(src.UpdateStatus, src.URL, now, fetchErr, httpCode, errURL)
		debuglog.WarnLog("refreshOneSubscriptionSource: source %s fetch failed: %v", src.ID, fetchErr)
		return true
	}

	// Единственное место разбора тела подписки (SPEC Т3): кап резолвится
	// «настройка подписки → дефолт настроек приложения», аварийный потолок
	// 3000 клэмпится внутри парсера.
	capN := resolveSubscriptionMaxNodes(src.MaxNodes, settings)
	material, matErr := config.MaterializeSubscriptionBody(src.ID, res.Body, src.Skip, capN)

	// Достоверность (SPEC 113-A): обрыв разбора — недостоверен. Тело, из
	// которого не родилось НИ ОДНОЙ записи при наличии per-record деградаций,
	// тоже: так выглядит HTML-страница вместо подписки, и удалить по ней все
	// узлы значило бы потерять состояние из-за мусорного ответа. Ноль записей
	// БЕЗ деградаций — легально (пользователь skip'ом отсёк всё): merge
	// честно удалит исчезнувших. material == nil при nil-ошибке парсера —
	// дефект контракта ниже по стеку: тоже недостоверно (фикс ревью W3 —
	// прежняя форма выражения разыменовывала бы nil в ветке trusted).
	//
	// SPEC 116 W11: считается по СОБРАВШИМСЯ узлам (material.Supported), а не
	// по длине Nodes — в ней теперь живут и неразобранные записи
	// (kind=unsupported). HTML-страница вместо подписки даёт полный список
	// отбракованных строк, и посчитай мы их узлами, ответ объявился бы
	// достоверным и снёс бы весь состав.
	trusted := matErr == nil && material != nil && (material.Supported > 0 || len(material.Warnings) == 0)

	var mergeWarns []string
	if trusted {
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

	// Заголовки провайдера успешного ответа: канонический дом SubMeta.
	// Диагностика (даты, коды, счёт узлов, truncated) живёт в updateStatus —
	// второго её экземпляра в состоянии нет.
	merged := res.Meta // value-copy
	src.Meta = &merged
	return true
}

// resolveSubscriptionMaxNodes — резолв капа принятых узлов (SPEC Т1,
// двухступенчатый — провайдерского заголовка у max_nodes не существует):
// настройка подписки → дефолт настроек приложения → аварийный
// потолок-константа (клэмп 3000 внутри ParseSubscriptionBody).
func resolveSubscriptionMaxNodes(subMax int, settings locale.Settings) int {
	if subMax > 0 {
		return subMax
	}
	if settings.DefaultSubscriptionMaxNodes > 0 {
		return settings.DefaultSubscriptionMaxNodes
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
		// Счёт узлов — СОБРАВШИХСЯ: неразобранные записи (SPEC 116 W11) ездят
		// в том же Nodes, но узлами подписки не являются — их число UI
		// показывает отдельным слагаемым («38 + 5 unsupported»).
		st.NodesCountFetched = material.Supported
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
// одного source, переданного по pointer'у из in-memory wizard model. Fetch
// идёт на in-memory URL — это и есть смысл пути, и он даёт корректный UX в
// трёх сценариях:
//
//  1. Cold start, state.json ещё нет (свежая инсталляция, шаблон с дефолтными
//     URL'ами в model). Refresh должен работать без принуждения к Save.
//  2. Существующий state, пользователь добавил новый URL и сразу кликнул
//     Refresh — fetch на in-memory URL, не на старый из state.json.
//  3. Пользователь редактирует URL существующего source и кликает Refresh —
//     то же самое, актуальный URL побеждает.
//
// SPEC 116 W13 (баг обкатки): результат ДОПОЛНИТЕЛЬНО закрепляется в
// state.json — для ЭТОГО источника и только для полей, которые родил fetch
// (persistFetchResultForSource). Прежде путь не писал на диск вовсе, и это
// был единственный вход fetch'а с таким свойством: у ↻ строки результат жил
// только в памяти визарда, а state.json тем временем переписывали ДРУГИЕ
// писатели (heartbeat, VPN-event retry, Update) — каждый со своей копии,
// загруженной с диска, и целым файлом. Разойтись двум копиям достаточно было
// одного клика: дальше любая полная запись — хоть Save визарда, хоть
// авто-обновление — молча выбрасывала результат другой стороны. На живой
// машине это выглядело так: ↻ отработал, лог показал разбор, а в state.json
// у подписки остались last_success_at и состав узлов часовой давности.
//
// Записываются ТОЛЬКО поля fetch'а (nodes/updateStatus/meta/pendingDisabled)
// — остальным в записи владеет визард, и переносить сюда его правки нельзя:
// снимок в горутине старше живой модели ровно настолько, сколько летел fetch.
//
// Concurrency: запись идёт под SubscriptionMu — тем же, которым сериализуются
// heartbeat и Update. UI button-state блокирует двойной клик по той же row.
//
// Возвращает (changed, err): changed=true если результат fetch'а изменил
// запись (caller должен пере-рендерить row); err — ошибки fetch'а. Неудача
// закрепления на диске не отменяет успех обновления в памяти: визард сохранит
// его своим Save.
func (svc *ConfigService) RefreshSourceInPlace(src *state.Source) (bool, error) {
	if src == nil {
		return false, fmt.Errorf("RefreshSourceInPlace: nil source")
	}
	if src.Kind != state.SourceKindSubscription {
		return false, fmt.Errorf("source %s is not a subscription (type=%q)", src.ID, src.Kind)
	}
	if src.URL == "" {
		return false, fmt.Errorf("source %s has empty URL", src.ID)
	}
	execDir := svc.ac.FileService.ExecDir

	// Дефолт капа — настройки приложения (SPEC Т1); их отсутствие на
	// cold-start нормально: парсер клэмпит своим потолком.
	settings := locale.LoadSettings(platform.GetBinDir(execDir))

	changed := refreshOneSubscriptionSource(src, settings)
	if changed {
		svc.persistFetchResultForSource(src, execDir)
	}
	return changed, nil
}

// persistFetchResultForSource закрепляет в state.json результат fetch'а ОДНОГО
// источника (SPEC 116 W13).
//
// Почему не Save целой модели: модель принадлежит визарду, и её запись — его
// Save. Здесь на руках только снимок одного источника, снятый до fetch'а;
// всё, что в нём НЕ результат сети (url, skip, тег-политика, замена, подпись,
// enabled), к моменту приземления могло устареть на любую правку пользователя.
// Поэтому в дисковую запись переносятся ровно те же поля, что переносит
// business.applyFetchResultFields в модель — список обязан совпадать с ним.
//
// Источник, которого на диске нет (визард ещё не сохранял его — сценарий 1
// cold-start), НЕ создаётся: state.json пишет визард, и родить там половину
// записи значило бы подсунуть ему источник без url и подписи.
func (svc *ConfigService) persistFetchResultForSource(src *state.Source, execDir string) {
	if src == nil || src.ID == "" || svc == nil || svc.ac == nil {
		return
	}
	statePath := platform.GetWizardStatePath(execDir)

	svc.ac.SubscriptionMu.Lock()
	defer svc.ac.SubscriptionMu.Unlock()

	s, err := state.Load(statePath)
	if err != nil {
		// Cold start (state.json ещё нет) — не ошибка пути: результат живёт в
		// модели, визард сохранит его своим Save.
		debuglog.DebugLog("persistFetchResultForSource: state.Load: %v — результат остаётся в памяти", err)
		return
	}
	disk := s.FindSource(src.ID)
	if disk == nil || disk.Kind != state.SourceKindSubscription {
		debuglog.DebugLog("persistFetchResultForSource: источник %s ещё не сохранён визардом — пропуск", src.ID)
		return
	}
	disk.Nodes = src.Nodes
	disk.UpdateStatus = src.UpdateStatus
	disk.Meta = src.Meta
	disk.PendingDisabled = src.PendingDisabled

	if err := s.Save(statePath); err != nil {
		debuglog.WarnLog("persistFetchResultForSource: state.Save: %v", err)
		return
	}
	// «Конфиг устарел» (SPEC Т4): состав узлов на диске сменился, но сборку
	// отсюда не запускаем — это решение пользователя, как и у
	// RefreshSingleSubscription.
	if svc.ac.StateService != nil {
		svc.ac.StateService.MarkConfigStale()
	}
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
	if src.Kind != state.SourceKindSubscription {
		return nil, fmt.Errorf("source %s is not a subscription (type=%q)", sourceID, src.Kind)
	}

	settings := locale.LoadSettings(platform.GetBinDir(execDir))
	dirty := refreshOneSubscriptionSource(src, settings)
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

// extractXrayJSONNodePool — SPEC 054. Для Xray JSON array подписок:
// парсит body через subscription.ParseNodesFromXrayJSONArray и эмитит первые
// `limit` нод в URI-like формате `<scheme>://<server>:<port>#<tag>`.
//
// Возвращает (previewNodes, totalCount). totalCount — реальное количество
// нод в JSON array (для meta.nodes_count_fetched).
//
// На parse-error → возвращает (nil, 0) — caller должен решить fallback (но
// caller сначала вызывает IsXrayJSONArrayBody, так что path должен совпадать).
func extractXrayJSONNodePool(body []byte, limit int) ([]string, int) {
	nodes, err := subscription.ParseNodesFromXrayJSONArray(string(body), nil)
	if err != nil {
		debuglog.WarnLog("extractXrayJSONNodePool: parse failed: %v", err)
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

// extractNodePool — первые `limit` URI-like строк из decoded body.
// «URI-like» = содержит "://", не пустая, не комментарий.
func extractNodePool(body []byte, limit int) []string {
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
