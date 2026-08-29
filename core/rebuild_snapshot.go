package core

import (
	"fmt"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
)

// buildSnapshotFromState — эмиссия узлов из МАТЕРИАЛИЗОВАННОГО состояния
// (SPEC 118 Т5): готовый ParsedCache для BuildConfig, **без единого
// сетевого вызова и без разбора тел подписок**.
//
// Пришло на смену buildSnapshotFromRawCache: тела подписок больше не лежат
// отдельным кэшем на диске и не перепарсиваются на каждой сборке — узлы
// разобраны один раз (fetch либо миграция) и живут в `sources[].nodes[]`.
// Сборке остаётся эмиссия из body.
//
// Контракт:
//   - Подписка без единого узла — это состояние «её ещё ни разу не обновляли»
//     (или последний ответ был недостоверен). Если таковы ВСЕ включённые
//     подписки, возвращается ErrNoMaterializedNodes, и вызывающий делает
//     auto-Update: собирать не из чего. Часть без узлов — деградация с
//     предупреждением, а не отказ: подписка, у которой fetch падает стабильно
//     (лимит устройств, протухший токен), не должна делать конфиг
//     несобираемым навсегда.
//
// SPEC 056: параметр td (nil-safe) подаёт template для pre-patch
// parser_config с preset.outbounds[] перед запуском outbound-генератора.
//
// SPEC 115 (фикс-раунд): парсерный результат отдаётся ВТОРЫМ возвратом, а не
// уезжает отсюда прямо в отчёт сборки. Эмиссия идёт ДО noop-развилки
// Rebuild'а, и открытая здесь попытка на холостом вызове оставалась бы вечно
// незавершённой — то есть стирала бы готовый отчёт прошлой полной сборки,
// ничего не дав взамен. Кто попытку открывает, тот её и доводит.
func buildSnapshotFromState(s *state.State, execDir string, subst config.VarSubstituter, td *template.TemplateData) (*build.ParsedCache, *config.OutboundGenerationResult, error) {
	if s == nil {
		return nil, nil, fmt.Errorf("buildSnapshotFromState: nil state")
	}

	// Есть ли материал: у скольких включённых подписок есть хоть один узел.
	empty := []string{}
	enabledSubs := 0
	for i := range s.Sources {
		src := &s.Sources[i]
		if src.Kind != state.SourceKindSubscription || !src.Enabled || src.URL == "" {
			continue
		}
		enabledSubs++
		if len(src.Nodes) == 0 {
			empty = append(empty, src.URL)
		}
	}
	if enabledSubs > 0 && len(empty) == enabledSubs {
		return nil, nil, fmt.Errorf("%w: %d subscription(s) have no materialized nodes (e.g. %s)",
			ErrNoMaterializedNodes, len(empty), empty[0])
	}
	var partialWarnings []string
	for _, url := range empty {
		debuglog.WarnLog("buildSnapshotFromState: подписка %s без узлов — источник деградирован (fetch падает?), конфиг собирается из остальных", url)
		partialWarnings = append(partialWarnings, fmt.Sprintf("subscription %s has no nodes yet (last fetch failed?) — built without it", url))
	}

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
	result, err := config.GenerateOutboundsFromParserConfig(&parserCfg, tagCounts, nil,
		directionBuildOptionsFrom(td))
	if err != nil {
		// Узлов не набралось. Снимок не построен — зато диагностика по
		// источникам приехала вместе с ошибкой, и её отдаём: она объясняет,
		// ПОЧЕМУ сборка не состоялась. Отбросив её здесь, мы оставили бы
		// сломанный источник без пометки в списке.
		return nil, result, fmt.Errorf("generate outbounds from materialized nodes: %w", err)
	}

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

// ErrNoMaterializedNodes — sentinel: ни у одной включённой подписки нет
// материализованных узлов. Rebuild делает по нему auto-Update fallback.
var ErrNoMaterializedNodes = fmt.Errorf("no materialized subscription nodes")
