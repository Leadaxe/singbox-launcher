package core

import (
	"fmt"
	"os"
	"path/filepath"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// buildSnapshotFromRawCache — SPEC 052 phase 6: парсит `bin/subscriptions/*.raw`
// в память и строит ParsedCache, готовый для BuildConfig. **Без network call'ов.**
//
// Контракт:
//   - Для каждого enabled subscription Source в state.Sources
//     ищет matching `.raw` файл по ID;
//   - Server source'ы парсятся напрямую из URI (не нуждаются в .raw);
//   - Если .raw нет НИ У ОДНОЙ enabled subscription — возвращает
//     (nil, ErrRawCacheIncomplete); caller делает auto-Update fallback.
//     Частично отсутствующий кэш — warning + деградация источника, не ошибка.
//
// SPEC 056: параметр td (nil-safe) подаёт template для pre-patch
// parser_config с preset.outbounds[] перед запуском native outbound
// generator'а. td=nil → no preset processing (тесты, legacy fallback);
// non-nil → ApplyPresetOutboundsToParserConfig применяет mode=add/update
// от enabled preset-refs в s.Rules.
//
// SPEC 115 (фикс-раунд): парсерный результат отдаётся ВТОРЫМ возвратом, а не
// уезжает отсюда прямо в отчёт сборки. Разбор кэша идёт ДО noop-развилки
// Rebuild'а, и открытая здесь попытка на холостом вызове оставалась бы вечно
// незавершённой — то есть стирала бы готовый отчёт прошлой полной сборки, ничего
// не дав взамен. Кто попытку открывает, тот её и доводит: RebuildConfigIfDirty
// делает это уже за развилкой, когда известно, что сборка реально идёт.
func buildSnapshotFromRawCache(s *state.State, execDir string, subst config.VarSubstituter, td *template.TemplateData) (*build.ParsedCache, *config.OutboundGenerationResult, error) {
	if s == nil {
		return nil, nil, fmt.Errorf("buildSnapshotFromRawCache: nil state")
	}
	subsDir := platform.GetSubscriptionsDir(execDir)

	// Проверяем completeness: для каждой enabled subscription есть .raw?
	missing := []string{}
	enabledSubs := 0
	for _, src := range s.Sources {
		if src.Kind != state.SourceTypeSubscription || !src.Enabled || src.URL == "" {
			continue
		}
		enabledSubs++
		path := filepath.Join(subsDir, src.ID+".raw")
		if _, err := os.Stat(path); err != nil {
			missing = append(missing, src.URL)
		}
	}
	// ErrRawCacheIncomplete (→ caller делает auto-Update fallback) — только
	// когда кэша нет НИ У ОДНОЙ подписки: первый запуск или чистый профиль,
	// там без Update собирать не из чего. Частично отсутствующий кэш —
	// деградация, а не ошибка: источник, который стабильно падает при fetch
	// (лимит устройств, протухший токен), никогда не получит .raw, и строгая
	// проверка делала конфиг несобираемым навсегда — а в паре с fallback'ом
	// это раскручивало бесконечный Update-цикл. Отсутствующий источник парсер
	// разово попробует по сети (cache-miss → FetchSubscription) и при
	// неудаче деградирует, конфиг соберётся из остальных.
	if len(missing) > 0 && len(missing) == enabledSubs {
		return nil, nil, fmt.Errorf("%w: %d subscription(s) missing raw cache (e.g. %s)",
			ErrRawCacheIncomplete, len(missing), missing[0])
	}
	var partialWarnings []string
	for _, url := range missing {
		debuglog.WarnLog("buildSnapshotFromRawCache: no raw cache for %s — источник деградирован (fetch падает?), конфиг собирается из остальных", url)
		partialWarnings = append(partialWarnings, fmt.Sprintf("subscription %s has no cached nodes (last fetch failed?) — built without it", url))
	}

	// URL → decoded body lookup для парсера.
	bodyByURL := buildBodyLookup(s, subsDir)

	prev := subscription.LookupCachedBody
	subscription.LookupCachedBody = func(url string) ([]byte, bool) {
		b, ok := bodyByURL[url]
		return b, ok
	}
	defer func() { subscription.LookupCachedBody = prev }()

	parserCfg := s.ParserConfig
	if subst != nil {
		config.SubstituteParserConfigPlaceholders(&parserCfg, subst)
	} else {
		// Caller не передал — берём дефолтный (template + state vars с диска).
		def := config.BuildVarSubstituterFromDisk(execDir)
		config.SubstituteParserConfigPlaceholders(&parserCfg, def)
	}

	// SPEC 057-R-N: ensure parserCfg.Outbounds в правильном shape перед emit.
	//   1. Sync приводит slice к "active preset ref entries + Updates[] стеки"
	//      (handles stale state: template changed since last UI save, или
	//      legacy state.json без ref/updates).
	//   2. MergeOutboundUpdatesInPlace flatten'ит Updates[] стеки в финальное
	//      body — generator про эти поля не знает, видит уже merged.
	// td=nil → quiet skip (тесты, legacy fallback path).
	if td != nil {
		// SPEC 058-R-N: migration legacy direct→referenced. Idempotent.
		tgt := build.TargetSpecFromState(s)
		_ = build.MigrateOutboundsToReferencedShape(&parserCfg.ParserConfig.Outbounds, s.Rules, td, tgt)
		build.SyncOutboundsWithTemplate(s.Rules, &parserCfg.ParserConfig.Outbounds, td.Presets, build.TemplateOutboundTags(td), tgt)
		build.MergeOutboundUpdatesInPlace(&parserCfg, td, tgt)
	}

	tagCounts := make(map[string]int)
	loadNodesFunc := func(ps configtypes.ProxySource, tc map[string]int, pc func(float64, string), idx, total int) ([]*configtypes.ParsedNode, error) {
		return subscription.LoadNodesFromSource(ps, tc, pc, idx, total)
	}

	result, err := config.GenerateOutboundsFromParserConfig(&parserCfg, tagCounts, nil, loadNodesFunc,
		directionBuildOptionsFrom(td))
	if err != nil {
		// Кэш вернулся, но узлов из него не набралось. Снимок не построен —
		// зато диагностика по источникам приехала вместе с ошибкой, и её
		// отдаём: она объясняет, ПОЧЕМУ сборка не состоялась. Отбросив её
		// здесь, мы оставили бы сломанный источник без пометки в списке.
		return nil, result, fmt.Errorf("generate outbounds from raw cache: %w", err)
	}

	subscription.LogDuplicateTagStatistics(tagCounts, "Rebuild")

	warnings := partialWarnings
	if result.SkippedNaiveNodes > 0 {
		warnings = append(warnings, fmt.Sprintf("%d naive node(s) skipped: %s",
			result.SkippedNaiveNodes, result.SkippedNaiveReason))
	}

	return &build.ParsedCache{
		Outbounds:   jsonStringsToRawMessages(result.OutboundsJSON),
		Endpoints:   jsonStringsToRawMessages(result.EndpointsJSON),
		Warnings:    warnings,
		NodeOrigins: buildNodeOrigins(result.NodeOrigins),
	}, result, nil
}

// buildNodeOrigins переводит карту происхождения узлов из формы парсера в
// форму сборщика (SPEC 113-B). Два одинаковых типа в разных пакетах — цена
// того, что core/build остаётся leaf-пакетом и о core/config не знает.
func buildNodeOrigins(src map[string]config.NodeOrigin) map[string]build.NodeOrigin {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]build.NodeOrigin, len(src))
	for tag, o := range src {
		out[tag] = build.NodeOrigin{SourceID: o.SourceID, SourceLabel: o.SourceLabel}
	}
	return out
}

// ErrRawCacheIncomplete — sentinel для отсутствующих .raw файлов.
// Rebuild делает auto-Update fallback при этой ошибке.
var ErrRawCacheIncomplete = fmt.Errorf("raw cache incomplete")

// buildBodyLookup — формирует URL → decoded body map для всех subscription
// source'ов в state. Decoded — потому что FetchSubscription возвращает
// уже decoded content (после base64 strip), а LookupCachedBody hook должен
// мимикрировать тот же контракт.
func buildBodyLookup(s *state.State, subsDir string) map[string][]byte {
	out := make(map[string][]byte, len(s.Sources))
	for _, src := range s.Sources {
		if src.Kind != state.SourceTypeSubscription || !src.Enabled || src.URL == "" {
			continue
		}
		raw, err := state.ReadRawBody(subsDir, src.ID)
		if err != nil {
			debuglog.WarnLog("buildBodyLookup: read raw for %s: %v", src.ID, err)
			continue
		}
		// FetchSubscription возвращает decoded — мимикрируем тот же контракт.
		if dec, err := subscription.DecodeSubscriptionContent(raw); err == nil {
			out[src.URL] = dec
		} else {
			out[src.URL] = raw
		}
	}
	return out
}
