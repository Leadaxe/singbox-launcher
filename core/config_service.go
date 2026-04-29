// Package core provides core application logic including process management,
// configuration parsing, and service orchestration.
package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
	v5 "singbox-launcher/core/state/v5"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// ConfigService encapsulates configuration parsing and update routines.
// It handles fetching subscriptions, parsing proxy nodes, generating JSON outbounds,
// and updating the configuration file. The service maintains separation of concerns
// by isolating all configuration-related operations from the main controller.
type ConfigService struct {
	ac *AppController
}

// NewConfigService constructs a ConfigService bound to the controller.
// The service requires an initialized AppController with valid ConfigPath.
func NewConfigService(ac *AppController) *ConfigService {
	subscription.CreateHTTPClientFunc = CreateHTTPClient
	subscription.IsNetworkErrorFunc = IsNetworkError
	subscription.GetNetworkErrorMessageFunc = GetNetworkErrorMessage
	services.CreateHTTPClientFunc = CreateHTTPClient
	return &ConfigService{ac: ac}
}

// RunParserProcess starts the internal configuration update process.
// Logic migrated from controller-level function without behavior changes.
func (svc *ConfigService) RunParserProcess() {
	ac := svc.ac
	// Проверяем, не запущен ли уже парсинг
	ac.ParserMutex.Lock()
	if ac.ParserRunning {
		ac.ParserMutex.Unlock()
		if ac.UIService != nil && ac.UIService.Application != nil && ac.UIService.MainWindow != nil {
			dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow, "Parser Info", "Configuration update is already in progress.")
		}
		return
	}
	ac.ParserRunning = true
	ac.ParserMutex.Unlock()

	debuglog.InfoLog("RunParser: Starting internal configuration update...")
	// Ensure flag is reset after completion, even if there's an error
	defer func() {
		ac.ParserMutex.Lock()
		ac.ParserRunning = false
		ac.ParserMutex.Unlock()
	}()

	// Call internal parser to update configuration
	result, err := svc.UpdateConfigFromSubscriptions()

	// Обрабатываем результат — финальный статус идёт в новый
	// in-place toast под Exit'ом (SPEC 052 phase 8 polish), а не popup.
	if err != nil {
		debuglog.ErrorLog("RunParser: subscriptions refresh failed: %v", err)
		if ac.UIService != nil && ac.UIService.ShowSubsResultFunc != nil {
			ac.UIService.ShowSubsResultFunc(false, err.Error())
		} else {
			// Fallback: legacy popup (если новый callback не зарегистрирован).
			ac.ShowParserError(fmt.Errorf("refresh subscriptions: %w", err))
		}
		return
	}
	debuglog.InfoLog("RunParser: cache refreshed; config.json will be rebuilt on next Restart/Rebuild")
	if ac.UIService != nil && ac.UIService.ShowSubsResultFunc != nil {
		ac.UIService.ShowSubsResultFunc(true, parserSuccessToastMessage(result))
	} else if ac.UIService != nil && ac.UIService.Application != nil && ac.UIService.MainWindow != nil {
		dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow, "Subscriptions", parserSuccessToastMessage(result))
	}
}

// parserSuccessToastMessage formats the toast shown after a successful
// subscriptions refresh. Toast intentionally talks about cache (the actual
// thing Update produces in SPEC 045) and prompts user to Rebuild for the
// changes to land in `config.json`.
func parserSuccessToastMessage(result *config.OutboundGenerationResult) string {
	if result == nil || result.TotalSources <= 0 {
		return "Subscriptions refreshed. Press Rebuild or Restart to apply."
	}
	if result.FailedSources == 0 {
		return fmt.Sprintf("Subscriptions refreshed (%d sources, %d nodes). Press Rebuild or Restart to apply.",
			result.SucceededSources, result.NodesCount)
	}
	return fmt.Sprintf("Subscriptions partially refreshed: %d/%d sources OK (%d failed). Press Rebuild or Restart to apply.",
		result.SucceededSources, result.TotalSources, result.FailedSources)
}

// updateParserProgress safely calls UpdateParserProgressFunc if it's not nil
func updateParserProgress(ac *AppController, progress float64, status string) {
	if ac.UIService != nil && ac.UIService.UpdateParserProgressFunc != nil {
		ac.UIService.UpdateParserProgressFunc(progress, status)
	}
}

// ProcessProxySource delegates to subscription.LoadNodesFromSource
func (svc *ConfigService) ProcessProxySource(proxySource config.ProxySource, tagCounts map[string]int, progressCallback func(float64, string), subscriptionIndex, totalSubscriptions int) ([]*config.ParsedNode, error) {
	return subscription.LoadNodesFromSource(proxySource, tagCounts, progressCallback, subscriptionIndex, totalSubscriptions)
}

// GenerateNodeJSON delegates to config.GenerateNodeJSON
func (svc *ConfigService) GenerateNodeJSON(node *config.ParsedNode) (string, error) {
	return config.GenerateNodeJSON(node)
}

// GenerateOutboundsFromParserConfig delegates to config.GenerateOutboundsFromParserConfig.
// Hotfix v0.8.8.1: resolves `@varname` placeholders in parser_config.outbounds[]
// options (template defaults from wizard_template.json + user overrides from
// state.json settings_vars) before generation. See core/config/varsubst.go.
func (svc *ConfigService) GenerateOutboundsFromParserConfig(
	parserConfig *config.ParserConfig,
	tagCounts map[string]int,
	progressCallback func(float64, string),
) (*config.OutboundGenerationResult, error) {
	subst := config.BuildVarSubstituterFromDisk(svc.ac.FileService.ExecDir)
	config.SubstituteParserConfigPlaceholders(parserConfig, subst)

	loadNodesFunc := func(ps config.ProxySource, tc map[string]int, pc func(float64, string), idx, total int) ([]*config.ParsedNode, error) {
		return svc.ProcessProxySource(ps, tc, pc, idx, total)
	}
	return config.GenerateOutboundsFromParserConfig(parserConfig, tagCounts, progressCallback, loadNodesFunc)
}

// UpdateConfigFromSubscriptions — **pure cache-refresh pipeline**.
//
// SPEC 045 cleanup invariant: Update НЕ пишет config.json. Единственный
// writer config.json — RebuildConfigIfDirty. Update только обновляет
// `outbounds.cache.json` свежими нодами из подписок.
//
//	parser_config (из state.json)
//	  → SubstituteParserConfigPlaceholders (resolve @vars)
//	  → GenerateOutboundsFromParserConfig (network → []ParsedNode → []OutboundJSON)
//	  → bin/subscriptions/<id>.raw (per-source raw body на диск)
//	  → MarkConfigStale (config stale, нужен Rebuild)
//
// Coarse resilience: если ВСЕ источники упали (network down, etc.) И на
// диске уже есть ненулевой cache — оставляем старый cache как есть, чтобы
// Rebuild мог продолжать работать на последних известных нодах.
// Per-source merge (preserve nodes from failed sources, refresh succeeded
// ones) — отдельный TODO; пока — all-or-nothing.
//
// Возвращает per-source result (counts) для toast-сообщения.
func (svc *ConfigService) UpdateConfigFromSubscriptions() (*config.OutboundGenerationResult, error) {
	ac := svc.ac
	execDir := ac.FileService.ExecDir

	parserConfig, stateRef, err := svc.loadParserConfigForUpdate()
	if err != nil {
		updateParserProgress(ac, -1, fmt.Sprintf("Error: %v", err))
		return nil, err
	}

	// SPEC 052: per-source meta refresh + raw body cache. Происходит
	// до парсера; результат сохраняем в state.json (Connections.Sources[i].Meta).
	//
	// **Lock**: SubscriptionMu сериализует с UI per-source Refresh'ами
	// и event-triggered retry'ями (см. controller.go SubscriptionMu).
	ac.SubscriptionMu.Lock()
	refreshSubscriptionsMetaAndCache(stateRef, execDir)
	ac.SubscriptionMu.Unlock()

	subst := config.BuildVarSubstituterFromDisk(execDir)
	config.SubstituteParserConfigPlaceholders(parserConfig, subst)

	updateParserProgress(ac, 5, "Parsed ParserConfig block")

	progressCallback := func(p float64, s string) {
		updateParserProgress(ac, p, s)
	}

	loadNodesFunc := func(ps config.ProxySource, tc map[string]int, pc func(float64, string), idx, total int) ([]*config.ParsedNode, error) {
		return svc.ProcessProxySource(ps, tc, pc, idx, total)
	}
	// SPEC 052 phase 6: parser использует pre-fetched .raw bodies через
	// LookupCachedBody hook. Это устраняет double-fetch — refreshSubscriptionsMetaAndCache
	// уже сходил в сеть и записал bin/subscriptions/<id>.raw; парсер
	// читает оттуда без повторного network call'а.
	subsDir := platform.GetSubscriptionsDir(execDir)
	bodyByURL := buildBodyLookup(stateRef, subsDir)
	prevHook := subscription.LookupCachedBody
	subscription.LookupCachedBody = func(url string) ([]byte, bool) {
		b, ok := bodyByURL[url]
		return b, ok
	}

	tagCounts := make(map[string]int)
	result, err := config.GenerateOutboundsFromParserConfig(parserConfig, tagCounts, progressCallback, loadNodesFunc)
	subscription.LookupCachedBody = prevHook
	if err != nil {
		progressCallback(-1, fmt.Sprintf("Error: %v", err))
		return result, fmt.Errorf("failed to generate outbounds: %w", err)
	}
	subscription.LogDuplicateTagStatistics(tagCounts, "Parser")

	// SPEC 052 phase 6: bin/outbounds.cache.json больше не пишем.
	// Per-source resilience приходит из bin/subscriptions/<id>.raw —
	// failed-fetch source'ы оставляют свой старый .raw нетронутым
	// (см. refreshSubscriptionsMetaAndCache).
	if len(result.OutboundsJSON) == 0 && len(result.EndpointsJSON) == 0 {
		progressCallback(-1, "Error: no nodes parsed from any source")
		return result, fmt.Errorf("no nodes parsed (all sources empty or failed)")
	}

	progressCallback(100, "Subscriptions refreshed; press Rebuild or Restart to apply")

	// Update success → cache свежий относительно state.proxies →
	//   ClearCacheStale (✓ источники только что обновились);
	//   MarkConfigStale (config.json теперь lag за кэшем — нужен Rebuild).
	// UI окрасит 🔄 в синий, кнопка-↻ Update вернётся в нейтральный.
	if ac.StateService != nil {
		ac.StateService.ClearCacheStale()
		ac.StateService.MarkConfigStale()
		ac.StateService.RecordUpdateSuccess()
	}
	if ac.UIService != nil {
		if ac.UIService.UpdateConfigStatusFunc != nil {
			ac.UIService.UpdateConfigStatusFunc()
		}
		if ac.UIService.UpdateCoreStatusFunc != nil {
			ac.UIService.UpdateCoreStatusFunc()
		}
	}

	// AutoRebuildOnChange — если пользователь включил тоггл (right-click
	// на кнопке refresh+rebuild на дашборде), сразу пересобираем config
	// чтобы он не отставал от свежего cache. Best-effort: ошибка rebuild'а
	// не отменяет успех Update'а.
	binDir := platform.GetBinDir(execDir)
	if locale.LoadSettings(binDir).AutoRebuildOnChange {
		if rebuildErr := ac.RebuildConfigIfDirty(); rebuildErr != nil {
			debuglog.WarnLog("UpdateConfigFromSubscriptions: AutoRebuild after refresh failed: %v", rebuildErr)
		}
	}

	ac.resumeAutoUpdate()
	return result, nil
}

// loadParserConfigForUpdate берёт parser_config из state.json — canonical
// и единственный источник с SPEC 045 cleanup'а. Если state.json нет (или
// в нём нет proxies) — это явная ошибка пользовательского flow: надо
// сначала открыть Wizard, заполнить и Save'нуть.
//
// Раньше тут был fallback на `parser.ExtractParserConfig(configPath)` для
// чтения `/** @ParserConfig */` блока из старого config.json (legacy
// migration v0.8.8.x → SPEC 045). Удалён вместе с самим блоком в build.go:
// теперь нет места откуда читать в обход state.
//
// Возвращает копию parser_config (Substitute мутирует in-place — не хочется
// чтобы он касался state.ParserConfig) и *state.State для DNS/Route/Vars
// в BuildContext.
func (svc *ConfigService) loadParserConfigForUpdate() (*config.ParserConfig, *state.State, error) {
	statePath := platform.GetWizardStatePath(svc.ac.FileService.ExecDir)
	s, err := state.Load(statePath)
	if err != nil {
		return nil, nil, fmt.Errorf("update requires state.json — open Wizard, fill in subscriptions and Save first (load state failed: %w)", err)
	}
	if s.ParserConfig.ParserConfig.Proxies == nil {
		return nil, nil, fmt.Errorf("update requires state.json with proxies — open Wizard, add subscription URL and Save first")
	}
	pcCopy := s.ParserConfig
	return &pcCopy, s, nil
}

// buildContextFromState собирает BuildContext из state + cache + template.
// Если state nil (legacy fallback) — DNS/Route остаются пустыми, шаблонные
// дефолты используются как есть.
//
// Параметр parserConfig оставлен в сигнатуре для backward-compat callsite'ов;
// в SPEC 045 cleanup'е поле `BuildContext.ParserConfigJSON` удалено вместе
// с блоком `@ParserConfig` в config.json. Аргумент игнорируется.
func buildContextFromState(s *state.State, cache *build.ParsedCache, td *template.TemplateData, _ *config.ParserConfig) build.BuildContext {
	ctx := build.BuildContext{
		Template:   td,
		Cache:      cache,
		ForPreview: false, // Update path = save mode (full inline rendering, no truncation)
	}

	if s == nil {
		// Legacy fallback — vars из template defaults (применятся в GetEffectiveConfig).
		return ctx
	}

	// State есть: vars + DNS + Route.
	vars := make(map[string]string, len(s.Vars))
	for _, v := range s.Vars {
		vars[v.Name] = v.Value
	}
	// Materialize clash_secret если template объявляет его и в vars нет.
	build.MaterializeClashSecretInVars(td, vars)
	ctx.Vars = vars

	// DNS scalars из state (могут жить в DNSOptions или vars; см. dnsConfigFromUpdate).
	ctx.DNS = dnsConfigForUpdate(s)
	ctx.Route = routeConfigForUpdate(s)
	return ctx
}

// dnsConfigForUpdate — извлекает DNS-related данные из state в build.DNSConfig.
// state.DNSOptions содержит servers/rules; final/strategy/independent_cache
// исторически живут в state.Vars (dns_*) после миграции SPEC 032.
func dnsConfigForUpdate(s *state.State) build.DNSConfig {
	cfg := build.DNSConfig{}
	if s.DNSOptions != nil {
		cfg.Servers = s.DNSOptions.Servers
		cfg.Final = s.DNSOptions.Final
		cfg.Strategy = s.DNSOptions.Strategy
		cfg.IndependentCache = s.DNSOptions.IndependentCache
		if len(s.DNSOptions.Rules) > 0 {
			raw, err := json.Marshal(map[string]interface{}{"rules": s.DNSOptions.Rules})
			if err == nil {
				cfg.RulesText = string(raw)
			}
		}
	}
	for _, v := range s.Vars {
		switch v.Name {
		case "dns_final":
			cfg.Final = v.Value
		case "dns_strategy":
			cfg.Strategy = v.Value
		case "dns_independent_cache":
			b := v.Value == "true"
			cfg.IndependentCache = &b
		}
	}
	return cfg
}

// routeConfigForUpdate — конвертит state.CustomRules в build.RouteConfig.
// SelectedFinalOutbound и DefaultDomainResolver — позже, требуют доступа
// к UI-state (model.SelectedFinalOutbound и проч.); для Update path шаблонные
// дефолты обычно в порядке.
func routeConfigForUpdate(s *state.State) build.RouteConfig {
	rules := make([]build.RouteRule, 0, len(s.CustomRules))
	for _, cr := range s.CustomRules {
		outbound := cr.SelectedOutbound
		if outbound == "" {
			outbound = cr.DefaultOutbound
		}
		rules = append(rules, build.RouteRule{
			Enabled:     cr.Enabled,
			Outbound:    outbound,
			PrimaryRule: cr.Rule,
			RuleSets:    cr.RuleSet,
		})
	}
	return build.RouteConfig{
		Rules: rules,
	}
}

// atomicWriteConfig — атомарная запись config.json через .tmp + os.Rename.
// Защищает работающий sing-box: в худшем случае (crash, power loss) старый
// config.json остаётся целым.
func atomicWriteConfig(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, platform.DefaultFileMode); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}



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
	if _, gcErr := v5.DeleteOrphans(subsDir, knownIDs); gcErr != nil {
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
		if httpErr, ok := subscription.IsHTTPError(fetchErr); ok {
			src.Meta.HTTPStatusCode = httpErr.StatusCode
		} else if res != nil {
			src.Meta.HTTPStatusCode = res.HTTPStatus
		}
		debuglog.WarnLog("refreshOneSubscriptionSource: source %s fetch failed: %v", src.ID, fetchErr)
		return true
	}

	if writeErr := v5.WriteRawBody(subsDir, src.ID, res.RawBody); writeErr != nil {
		debuglog.WarnLog("refreshOneSubscriptionSource: WriteRawBody for %s: %v", src.ID, writeErr)
	}

	merged := res.Meta // value-copy
	merged.URLAtFetch = src.URL
	merged.LastFetchedAt = now
	merged.LastStatus = "ok"
	merged.ErrorCount = 0
	merged.LastErrorMsg = ""
	merged.HTTPStatusCode = res.HTTPStatus
	merged.RawBodyBytes = res.RawBodyBytes
	merged.PreviewNodes = extractPreviewNodes(res.Body, 50)
	merged.NodesCountFetched = countURIs(res.Body)

	effectiveMax := src.MaxNodes
	if effectiveMax == 0 {
		effectiveMax = defaults.MaxNodes
	}
	if effectiveMax == 0 {
		effectiveMax = v5.DefaultMaxNodes
	}
	merged.Truncated = merged.NodesCountFetched > effectiveMax

	src.Meta = &merged
	return true
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

// jsonStringsToRawMessages конвертирует []string (как возвращает
// config.OutboundGenerationResult) в []json.RawMessage для build.ParsedCache.
//
// На входе — legacy-формат:
//
//	"\t// <human-readable label>\n\t{...JSON...},"
//
// (см. core/config/outbound_generator.go::GenerateNodeJSON). Нужно отрезать:
//  1. ведущий `\t` отступ;
//  2. line-comment `// label\n` (его не парсит strict JSON);
//  3. хвостовую `,` (разделитель в массиве outbounds в config.json).
//
// Простой `TrimSpace` не справится с `// ...` посредине. Используем
// `jsonc.ToJSON` — он стрипит комментарии, оставляя чистый JSON-объект.
func jsonStringsToRawMessages(in []string) []json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]json.RawMessage, 0, len(in))
	for _, s := range in {
		// 1. Снять трейлинг-комму и whitespace.
		cleaned := strings.TrimSpace(strings.TrimRight(s, ",\n\r\t "))
		if cleaned == "" {
			continue
		}
		// 2. Прогнать через jsonc → снимутся `//` и `/* */` комментарии.
		canonical := jsonc.ToJSON([]byte(cleaned))
		canonical = []byte(strings.TrimSpace(string(canonical)))
		if len(canonical) == 0 {
			continue
		}
		// 3. Strict-валидация: гарантия что в кэш попадает только валидный JSON.
		if !json.Valid(canonical) {
			debuglog.WarnLog("jsonStringsToRawMessages: skipping invalid JSON: %.80q", cleaned)
			continue
		}
		out = append(out, json.RawMessage(canonical))
	}
	return out
}
