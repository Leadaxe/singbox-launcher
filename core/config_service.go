// Package core provides core application logic including process management,
// configuration parsing, and service orchestration.
package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
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
	// SPEC 061 Phase 2: wire HWID-family request headers. fetcher.go can't
	// import internal/locale (settings live there but subscription is a leaf
	// package), so we surface a thin snapshot via a function variable. Hook
	// lazily-generates + persists HWID on the first fetch after install so
	// users don't get a "Settings tab → Save" detour before first Update.
	subscription.LoadSubscriptionSettingsFunc = func() subscription.SubscriptionRequestSettings {
		if ac == nil || ac.FileService == nil {
			return subscription.SubscriptionRequestSettings{}
		}
		binDir := platform.GetBinDir(ac.FileService.ExecDir)
		s := locale.LoadSettings(binDir)
		// Generate-and-persist HWID on first use so subsequent fetches
		// (and the Settings tab when the user opens it) see the same ID.
		// Failure to persist is non-fatal — we just regenerate next time;
		// nothing depends on stability across restarts until the user opts in.
		if s.HWID == "" {
			s.EnsureHWID()
			if err := locale.SaveSettings(binDir, s); err != nil {
				debuglog.WarnLog("ConfigService: persist generated HWID: %v", err)
			}
		}
		return subscription.SubscriptionRequestSettings{
			HWID:              s.HWID,
			SendHWID:          s.ShouldSendHWID(),
			DeviceModelHashed: s.SubscriptionDeviceModelHashed,
			UserAgent:         s.SubscriptionUserAgent, // "" → fetcher uses default
		}
	}
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
	_, err := svc.UpdateConfigFromSubscriptions()

	// SPEC 045 фаза 9: финальный success-toast эмитит сам
	// UpdateConfigFromSubscriptions (после rebuild). Здесь обрабатываем
	// только ошибку — fallback popup для случая когда новый callback не
	// зарегистрирован.
	if err != nil {
		debuglog.ErrorLog("RunParser: subscriptions refresh failed: %v", err)
		if ac.UIService != nil && ac.UIService.ShowSubsResultFunc != nil {
			ac.UIService.ShowSubsResultFunc(false, err.Error())
		} else {
			ac.ShowParserError(fmt.Errorf("refresh subscriptions: %w", err))
		}
		return
	}
	debuglog.InfoLog("RunParser: cache refreshed; config.json rebuilt by UpdateConfigFromSubscriptions")
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
// per-source кэши `bin/subscriptions/<id>.raw` свежими нодами из подписок.
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

	// SPEC 057-R-N: ensure parser_config.outbounds в правильном shape:
	//   Sync — приводит slice в соответствие с active preset refs
	//          (template might have changed since last UI save).
	//   Merge — flatten Updates[] стеки в финальное body для generator'а.
	// На failure LoadTemplateData (template missing) — warning + skip;
	// Update должен работать даже без template'а (legacy юзеры).
	if td, terr := template.LoadTemplateData(execDir); terr == nil {
		// SPEC 058-R-N: migration legacy direct→referenced. Idempotent.
		_ = build.MigrateOutboundsToReferencedShape(&parserConfig.ParserConfig.Outbounds, stateRef.Rules, td)
		build.SyncOutboundsWithActivePresets(stateRef.Rules, &parserConfig.ParserConfig.Outbounds, td.Presets)
		build.MergeOutboundUpdatesInPlace(parserConfig, td)
	} else {
		debuglog.WarnLog("UpdateConfigFromSubscriptions: LoadTemplateData failed (skip preset.outbounds sync): %v", terr)
	}

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
	// SPEC 047 phase 6 (SPEC 070): config-status refresh теперь приходит через
	// events.ConfigBuilt — RebuildConfigIfDirty ниже его публикует (ConfigStale
	// только что помечен ⇒ rebuild не no-op'нется), а dashboard-подписчик зовёт
	// updateConfigInfo. Финальный ShowSubsResultFunc-toast тоже рефрешит метку
	// (safety-net, если rebuild упал до публикации). Прямой UpdateConfigStatusFunc
	// здесь убран. UpdateCoreStatusFunc оставлен (VpnState-канал, вне scope).
	if ac.UIService != nil {
		if ac.UIService.UpdateCoreStatusFunc != nil {
			ac.UIService.UpdateCoreStatusFunc()
		}
	}

	// SPEC 045 фаза 9: убрали условный AutoRebuildOnChange — Update всегда
	// сопровождается rebuild'ом, чтобы config.json не отставал от свежего
	// cache. Best-effort: ошибка rebuild'а не отменяет успех Update'а, но
	// её сообщение покажем пользователю в финальном toast'е.
	rebuildErr := ac.RebuildConfigIfDirty()
	if rebuildErr != nil {
		debuglog.WarnLog("UpdateConfigFromSubscriptions: auto-rebuild after refresh failed: %v", rebuildErr)
	}

	// SPEC 045 фаза 9: финальный toast эмитим ЗДЕСЬ, не в RunParser-обёртке.
	// Иначе при auto-update fallback'е (RebuildConfigIfDirty → Update) UI
	// зависает на in-progress 100% — RunParser в этом пути не задействован.
	// Сообщение учитывает rebuild error: success Update + failed Rebuild = частичный успех.
	if ac.UIService != nil && ac.UIService.ShowSubsResultFunc != nil {
		if rebuildErr != nil {
			ac.UIService.ShowSubsResultFunc(false,
				fmt.Sprintf("%s (rebuild failed: %v)", parserSuccessToastMessage(result), rebuildErr))
		} else {
			ac.UIService.ShowSubsResultFunc(true, parserSuccessToastMessage(result))
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

// collectAllStageRuleSetTags возвращает объединение rule-set tags из ВСЕХ
// state-файлов в `bin/wizard_states/`. Источники tag'ов:
//   - CustomRule[i].RuleSet[].tag (legacy / user inline/srs правила; и enabled,
//     и disabled — пользователь может перетоггнуть, лишний download раздражает)
//   - RulesV6[i] Kind=preset → lookup template preset → для каждого
//     rule_set type=remote → content-addressed tag (SRSTagFromURL). Без
//     if/if_or-фильтрации (consciously keep more — лучше держать .srs который
//     потенциально нужен под другим var-комбо, чем потом качать снова).
//
// Используется для orphan GC `bin/rule-sets/` после Rebuild: live множество
// = это объединение, всё за пределами — orphan.
//
// Multi-stage safety: тот же принцип что collectAllStageSourceIDs для
// bin/subscriptions/. Без union'а Rebuild активного state'а сметёт .srs
// нужные другому (неактивному) stage'у — переключение обратно требует
// заново открыть Configurator и скачать.
//
// td (nil-safe) — TemplateData для resolve preset.Ref → rule_set[]. Если nil
// или preset не найден — preset-теги пропускаются (тот же fallback что для
// broken preset-ref'а в UI).
//
// Read-only: errors per-file логируются и пропускаются.
func collectAllStageRuleSetTags(execDir string, td *template.TemplateData) []string {
	statesDir := platform.GetWizardStatesDir(execDir)
	entries, err := os.ReadDir(statesDir)
	if err != nil {
		debuglog.WarnLog("collectAllStageRuleSetTags: readdir %s: %v", statesDir, err)
		return nil
	}

	// Pre-build preset lookup map by ID для быстрого resolve.
	var presetByID map[string]*template.Preset
	if td != nil {
		presetByID = make(map[string]*template.Preset, len(td.Presets))
		for i := range td.Presets {
			presetByID[td.Presets[i].ID] = &td.Presets[i]
		}
	}

	tagSet := make(map[string]struct{})
	addTag := func(tag string) {
		if tag != "" {
			tagSet[tag] = struct{}{}
		}
	}

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
			debuglog.DebugLog("collectAllStageRuleSetTags: skip %s: %v", path, loadErr)
			continue
		}
		// Legacy CustomRule rule_set tags.
		for i := range s.CustomRules {
			for _, rs := range s.CustomRules[i].RuleSet {
				var m map[string]interface{}
				if err := json.Unmarshal(rs, &m); err != nil {
					continue
				}
				if tag, ok := m["tag"].(string); ok {
					addTag(tag)
				}
			}
		}
		// SPEC 053: preset-ref bundled remote rule_set'ы. content-addressed tag'и.
		if presetByID != nil {
			for _, r := range s.Rules {
				if r.Kind != "preset" || r.Ref == "" {
					continue
				}
				tpl, ok := presetByID[r.Ref]
				if !ok {
					continue
				}
				for _, rs := range tpl.RuleSet {
					if rs.Type != "remote" || rs.URL == "" {
						continue
					}
					addTag(build.SRSTagFromURL(rs.URL))
				}
			}
		}
		// Issue #77: user-defined kind=srs rule'ы тоже keep'аются. Файл
		// сохраняется downloader'ом под `build.SRSTagFromURL(SrsURL)`;
		// без этой ветки orphan GC удалял бы реально-скачанный файл при
		// каждом save, ломая user-SRS правила.
		for _, r := range s.Rules {
			if r.Kind != state.RuleKindSrs {
				continue
			}
			body, err := r.DecodeBody()
			if err != nil {
				continue
			}
			sb, ok := body.(*state.SrsBody)
			if !ok || sb.SrsURL == "" {
				continue
			}
			addTag(build.SRSTagFromURL(sb.SrsURL))
		}
	}

	out := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		out = append(out, tag)
	}
	return out
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
