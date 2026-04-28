// Package build — единственная функция-генератор `config.json` из тройки
// (state, outbounds-cache, template).
//
// Это реализация фазы 3.4 + 5.3 SPEC 045 (STATE_CONFIG_DECOUPLING). До
// рефакторинга сборка config.json была размазана по двум write-points
// (`core/config.WriteToConfig` при Update и
// `ui/wizard/business.SaveConfigWithBackup` при Save визарда), причём
// каждый дублировал часть логики. После — `BuildConfig` — единственная
// чистая функция; вызывающий слой пишет результат на диск отдельным шагом.
//
// Архитектурно `core/build` — leaf-пакет: ничего не импортирует из ui/.
// Вызывающий слой (Configurator-presenter / parser-pipeline) собирает
// `BuildContext` из своих моделей и вызывает `BuildConfig`.
//
// Контракт:
//
//	ctx := build.BuildContext{
//	    Template:   td,        // *core/template.TemplateData
//	    Vars:       vars,      // map[string]string (включая dns_*, clash_secret)
//	    Cache:      cache,     // *build.ParsedCache
//	    Stats:      stats,     // PreviewStats для preview-режима
//	    ForPreview: false,     // true для preview, false для save
//	    DNS:        dnsCfg,    // DNSConfig для merge dns секции
//	    Route:      routeCfg,  // RouteConfig для merge route секции
//	}
//	res, err := build.BuildConfig(ctx)
//	if err != nil { ... }
//	atomic_write(configPath, res.ConfigJSON)
//
// `BuildConfig` НЕ:
//   - не пишет в файл (вызывающий делает это сам);
//   - не запускает `sing-box check` (валидация — отдельный шаг
//     вызывающего слоя; pipeline можно настроить так, чтобы check шёл
//     до record/atomic-rename);
//   - не парсит подписки (это слой parser);
//   - не материализует clash_secret (вызывающий делает
//     `MaterializeClashSecretInVars` до сборки контекста).
package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"singbox-launcher/core/template"
)

// BuildContext — все данные, нужные для одной сборки config.json.
//
// Заполняется вызывающим слоем (wizard / Configurator / parser pipeline).
// Все поля семантически required, но nil-tolerant поведение задокументировано
// поэлементно: BuildConfig не паникует на nil/пустых вложенных значениях.
type BuildContext struct {
	// Template — распарсенный шаблон. Содержит RawConfig, Params, Vars,
	// RawTemplate, Config, ConfigOrder. Обязательно — nil → ErrInvalidInputs.
	Template *template.TemplateData

	// Vars — итоговый набор vars для substitution (template defaults + state
	// overrides + DNS scalars + clash_secret). Вызывающий формирует через
	// `ApplyDNSScalarsToVars` + `MaterializeClashSecretInVars`. nil трактуется
	// как пустая map.
	Vars map[string]string

	// Cache — outbounds/endpoints из последнего parser-run. nil или
	// IsEmpty() — секции рендерятся пустыми (sing-box стартанёт без
	// подписочных нод; вызывающий обычно поднимает CacheStale).
	Cache *ParsedCache

	// Stats — preview-render metadata. Учитывается только при ForPreview=true.
	Stats PreviewStats

	// ForPreview — true для UI preview-кадра (сжатые сводки на больших
	// подписках); false для record (между маркерами пусто, parser-update
	// заполнит позже).
	ForPreview bool

	// DNS — параметры merge'а dns-секции. См. DNSConfig.
	DNS DNSConfig

	// Route — параметры merge'а route-секции. См. RouteConfig.
	Route RouteConfig
}

// Result — итог сборки.
type Result struct {
	// ConfigJSON — итоговый JSON-текст config.json. Не nil при err == nil.
	ConfigJSON []byte

	// Validation — non-fatal предупреждения (например, GetEffectiveConfig упал
	// на substitution и мы откатились на template defaults).
	Validation ValidationResult
}

// ValidationResult — структура для накопления fatal/warning'ов.
type ValidationResult struct {
	// Errors — fatal: build не должен считаться валидным.
	Errors []string
	// Warnings — non-fatal: пользователь должен знать, но конфиг применим.
	Warnings []string
}

// HasErrors — true, если есть хотя бы один fatal.
func (v ValidationResult) HasErrors() bool { return len(v.Errors) > 0 }

// ErrInvalidInputs — структурно неправильный BuildContext.
type ErrInvalidInputs struct{ Reason string }

func (e *ErrInvalidInputs) Error() string {
	return "build: invalid inputs: " + e.Reason
}

// BuildConfig собирает итоговый config.json из BuildContext.
//
// Шаги:
//  1. Validate ctx.Template (обязателен).
//  2. Эффективный конфиг через template.GetEffectiveConfig
//     (применяет Params + substitute vars c type-cast int/bool +
//     условия if/if_or). При ошибке fallback на td.Config / td.ConfigOrder.
//  3. Per-section build в порядке td.ConfigOrder:
//     - "outbounds" → BuildOutboundsSection (cache + static + markers)
//     - "endpoints" → BuildEndpointsSection
//     - "dns"       → MergeDNSSection + FormatSectionJSON
//     - "route"     → MergeRouteSection + FormatSectionJSON
//     - default     → FormatSectionJSON
//  4. Concat: { + sections joined ",\n" + }
//     (раньше тут ещё был `/** @ParserConfig */` блок; удалён в SPEC 045
//     cleanup'е — state.json теперь canonical, дубль parser_config'а
//     в config.json не нужен).
//
// Pure: I/O только через template/MergeRouteSection (filesystem-проверка
// SRS-файлов в convertRuleSetToLocalIfNeeded).
func BuildConfig(ctx BuildContext) (Result, error) {
	if ctx.Template == nil {
		return Result{}, &ErrInvalidInputs{Reason: "Template is nil"}
	}

	res := Result{}

	// Шаг 1: эффективный конфиг через GetEffectiveConfig.
	cfg, order := effectiveConfig(ctx.Template, ctx.Vars, &res)

	// Шаг 2: build sections.
	sections, err := buildOrderedSections(ctx, cfg, order)
	if err != nil {
		return Result{}, err
	}

	// Шаг 3: финальная конкатенация. Раньше тут ещё писался блок-комментарий
	// /** @ParserConfig ... */ с дублем parser_config — удалён в SPEC 045
	// cleanup'е, потому что state.json теперь canonical, а блок никто не
	// читает (4 readers смигрированы на state.Load). Само поле
	// `ctx.ParserConfigJSON` тоже выпилено вместе с блоком.
	var b strings.Builder
	b.WriteString("{\n")
	b.WriteString(strings.Join(sections, ",\n"))
	b.WriteString("\n}\n")

	res.ConfigJSON = []byte(b.String())
	return res, nil
}

// effectiveConfig возвращает эффективные секции и их порядок. При неудаче
// GetEffectiveConfig (например, неразрешимая var в `if`) — fallback на
// предкэшированные td.Config / td.ConfigOrder + warning в Validation.
func effectiveConfig(td *template.TemplateData, vars map[string]string, res *Result) (map[string]json.RawMessage, []string) {
	// Если у шаблона нет ни Params ни Vars — нечего применять, отдаём прекеш.
	if len(td.RawConfig) == 0 || (len(td.Params) == 0 && len(td.Vars) == 0) {
		return td.Config, td.ConfigOrder
	}
	effective, ord, err := template.GetEffectiveConfig(
		td.RawConfig,
		td.Params,
		runtime.GOOS,
		td.Vars,
		vars,
		td.RawTemplate,
	)
	if err != nil {
		res.Validation.Warnings = append(res.Validation.Warnings,
			fmt.Sprintf("template.GetEffectiveConfig failed (%v); falling back to template defaults", err))
		return td.Config, td.ConfigOrder
	}
	return effective, ord
}

// buildOrderedSections итерирует order и форматирует каждую секцию.
// Для каждой секции — указанный обработчик из orchestrator-маппинга
// (см. BuildConfig godoc); неизвестные ключи идут через `FormatSectionJSON`.
func buildOrderedSections(ctx BuildContext, cfg map[string]json.RawMessage, order []string) ([]string, error) {
	out := make([]string, 0, len(order))
	for _, key := range order {
		raw, ok := cfg[key]
		if !ok {
			continue
		}
		formatted, err := buildSection(ctx, key, raw)
		if err != nil {
			return nil, fmt.Errorf("build: section %q: %w", key, err)
		}
		out = append(out, fmt.Sprintf(`  "%s": %s`, key, formatted))
	}
	return out, nil
}

// buildSection — диспетчер для одной секции. Pure: state хранится только
// внутри ctx (никаких side effects вне результата).
func buildSection(ctx BuildContext, key string, raw json.RawMessage) (string, error) {
	switch key {
	case "outbounds":
		return BuildOutboundsSection(raw, cacheOutboundsAsStrings(ctx.Cache), ctx.ForPreview, ctx.Stats)
	case "endpoints":
		return BuildEndpointsSection(raw, cacheEndpointsAsStrings(ctx.Cache), ctx.ForPreview, ctx.Stats)
	case "dns":
		merged, err := MergeDNSSection(raw, ctx.DNS)
		if err != nil {
			return "", err
		}
		return FormatSectionJSON(merged, 2)
	case "route":
		merged, err := MergeRouteSection(raw, ctx.Route)
		if err != nil {
			return "", err
		}
		return FormatSectionJSON(merged, 2)
	default:
		formatted, err := FormatSectionJSON(raw, 2)
		if err != nil {
			// Если форматирование упало — fallback на raw, как делал legacy.
			return string(raw), nil
		}
		return formatted, nil
	}
}

// cacheOutboundsAsStrings конвертит []json.RawMessage cache в []string,
// который ожидает BuildOutboundsSection. Нормализует форматирование:
// outbounds в кэше хранятся compact (одна строка на entry), как у wizard
// `model.GeneratedOutbounds`. nil cache → nil []string.
func cacheOutboundsAsStrings(c *ParsedCache) []string {
	if c == nil || len(c.Outbounds) == 0 {
		return nil
	}
	return normalizeCacheEntries(c.Outbounds, true)
}

// cacheEndpointsAsStrings — аналогично для endpoints, но pretty-printed
// (multi-line c 2-space indent) — соответствует legacy
// `wizard.model.GeneratedEndpoints` формату для wireguard'ов.
func cacheEndpointsAsStrings(c *ParsedCache) []string {
	if c == nil || len(c.Endpoints) == 0 {
		return nil
	}
	return normalizeCacheEntries(c.Endpoints, false)
}

// normalizeCacheEntries приводит entries к ожидаемому форматированию:
//   - compact=true → одна строка на entry (json.Compact);
//   - compact=false → pretty-printed multi-line с IndentBase отступом.
//
// Cache-entries хранятся как clean JSON — без `\t`-префикса или дополнительной
// indent'ации (это была quirk legacy `GenerateNodeJSON`-генератора). Indent
// добавляется уже в `BuildOutboundsSection`/`BuildEndpointsSection` при
// формировании финальной секции.
func normalizeCacheEntries(entries []json.RawMessage, compact bool) []string {
	out := make([]string, 0, len(entries))
	for _, raw := range entries {
		if compact {
			b := &bytes.Buffer{}
			if err := json.Compact(b, raw); err != nil {
				out = append(out, string(raw))
				continue
			}
			out = append(out, b.String())
		} else {
			b := &bytes.Buffer{}
			if err := json.Indent(b, raw, "", IndentBase); err != nil {
				out = append(out, string(raw))
				continue
			}
			out = append(out, b.String())
		}
	}
	return out
}
