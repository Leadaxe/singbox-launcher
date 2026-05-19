# SPEC 056 — Implementation Plan

## Phase order

| # | Phase | Estimate | Touches |
|---|---|---|---|
| 0 | Pre-cleanup docs (этот SPEC, PLAN, TASKS; reset 055 TASKS; delete 055 IMPLEMENTATION_REPORT.md) | 0.1 day | SPECS/* |
| 1 | Surgical revert хаоса 055 (preserve P1–P10 + sing-box check) | 0.5 day | core/build, ui/configurator, core/* |
| 2 | Types & loader (`PresetOutbound` + `validatePresetOutbounds`) | 0.3 day | core/template |
| 3 | Pre-patch core: `ApplyPresetOutboundsToParserConfig` | 1 day | core/build/preset_outbounds.go (new) |
| 4 | Wire в build pipeline и preview | 0.3 day | core/build/build.go, core/config_service.go, ui/configurator/business/create_config.go |
| 5 | Route post-pass: `cleanDanglingOutboundRefInRule` | 0.2 day | core/build/preset_outbounds.go |
| 6 | UI: `collectActivePresetOutboundTags` + refresh-on-toggle | 0.3 day | ui/configurator/business/outbound.go, ui/configurator/tabs/rules_unified_rows.go |
| 7 | Template content migration (ru-inside / ru-blocked / russian outbounds) | 0.2 day | bin/wizard_template.json |
| 8 | Tests + golden fixtures + docs | 0.5 day | core/build/*_test.go, docs/release_notes/upcoming.md |

**Total estimate:** ~3.4 рабочих дня.

---

## Phase 0 — Pre-cleanup docs

**Цель:** зафиксировать корректное состояние Spec Kit перед началом
кода. Поведение пользовательского билда не меняется.

### Что меняем

- **`SPECS/056-B-N-OUTBOUNDS_PARSER_RESTORE/SPEC.md`** — обновить
  разделом «Корневая причина», «Финальная архитектура», «Acceptance»,
  «Параллельные правки» (см. `SPEC.md`).
- **`SPECS/056-B-N-OUTBOUNDS_PARSER_RESTORE/PLAN.md`** — этот файл.
- **`SPECS/056-B-N-OUTBOUNDS_PARSER_RESTORE/TASKS.md`** — чек-лист
  фаз 1–8.
- **`SPECS/055-F-N-PRESET_OUTBOUNDS/TASKS.md`** — сбросить все галочки
  в TODO. SPEC.md / PLAN.md 055 остаются как product-level спека
  желаемой семантики (она корректна, неверной была реализация).
- **`SPECS/055-F-N-PRESET_OUTBOUNDS/IMPLEMENTATION_REPORT.md`** —
  удалить (отчёт врёт: build green, тесты зелёные, но фича в проде
  не работает — это лож в документации).

### Acceptance

- `go build ./...` без изменений (нет изменений кода).
- Папка `055-F-N-PRESET_OUTBOUNDS` содержит SPEC.md, PLAN.md, TASKS.md
  (все нужные пункты в TODO).
- Папка `056-B-N-OUTBOUNDS_PARSER_RESTORE` содержит SPEC.md (обновлён),
  PLAN.md, TASKS.md.

---

## Phase 1 — Surgical revert хаоса 055

**Цель:** вернуть кодовую базу к состоянию `f665c27` для 055-related
файлов, **сохраняя** P1–P10 (см. `SPEC.md` таблицу) и mixed-куски из
`15b217c` (sing-box check) / `5e56c0b` (force-flag).

### Стратегия

Не `git revert` целиком (mixed commits), а **точечный откат файла за
файлом** через `git show f665c27:<path>` + ручное переналожение
параллельных правок (P1–P10) поверх.

### Файлы — полное удаление

- `core/build/preset_outbounds_test.go` — создан 055-ом, переписываем
  с нуля в Phase 8.
- `core/template/preset_outbounds_test.go` — то же.

### Файлы — частичный откат до `f665c27`, потом cherry-pick параллельных правок

| Файл | Откатить до `f665c27` | Cherry-pick после отката |
|------|---|---|
| `core/build/build.go` | да | `0c3dce5` (P8) + ничего из 055 |
| `core/build/dns_merge.go` | да | `b03fd5b` (P7) — DNS bare-tag |
| `core/build/preset_expand.go` | да | — (post-053 baseline OK) |
| `core/build/preset_merge.go` | да | — |
| `core/build/rules_pipeline.go` | да | — |
| `core/template/preset_loader.go` | да | — |
| `core/template/preset_types.go` | да | — |
| `core/rebuild.go` | **нет** (mixed) | Применить только sing-box check из `15b217c` (`validateConfigViaSingBox`, `stripANSI`, Step 5.4) + `5e56c0b` (forced flag). НЕ применять 055 куски. |
| `core/config_service.go` | **нет** (mixed) | Сохранить `1019144` (SPEC 054). Снести `AllNodeTags` поле и `collectAllNodeTagsFromCache`. |
| `ui/configurator/business/create_config.go` | **нет** (mixed) | Сохранить `d36a257` (P4) — preview applies preset-refs. Снести `AllNodeTags` и `collectAllNodeTagsFromCacheLocal`. |
| `ui/configurator/business/outbound.go` | да | — (баselineи не было файла; удалить целиком) |
| `ui/configurator/tabs/rules_unified_rows.go` | **нет** (mixed) | Сохранить `dc4cf09` (P5) + `0ecc403` (P6) — anti-loop фиксы. Снести вызов `refreshRulesTabFromPresenter` при toggle outbound preset. |
| `ui/core_dashboard_tab.go` | **нет** | Сохранить `5e56c0b` (P2). |
| `ui/configurator/presentation/presenter_methods.go` | **нет** | Сохранить `842df2c` (P3). |
| `bin/wizard_template.json` | да | — (откатить ru-inside/ru-blocked/russian outbounds-добавки, вернуть proxy-out/auto-proxy-out filters + global ru VPN 🇷🇺) |
| `bin/locale/ru.json` | **нет** | Сохранить `d36a257` локализационные ключи. |
| `internal/locale/en.json` | **нет** | Сохранить ключи source-overview из P9. |
| `internal/textnorm/proxy_display.go` | **нет** | Сохранить P10. |
| `ui/configurator/tabs/source_edit_*.go` | **нет** | Сохранить P9 (полностью). |
| `core/preview_nodes_test.go` | **нет** | Сохранить P1. |
| `docs/release_notes/upcoming.md` | **нет** | Снести 055 entry, сохранить 054 entry. |

### `PresetMergeContext` — что меняем

В `core/build/preset_merge.go` (post-053 baseline) структура
`PresetMergeContext` уже содержит:
```go
type PresetMergeContext struct {
    Presets        []template.Preset
    RulesV6        []v6.Rule
    DNS            v6.DNSConfig
    SrsCachedPaths map[string]string
    ExecDir        string
    // (NEW in Phase 3) — будет добавлено: ParserConfig *configtypes.ParserConfig
}
```
В Phase 3 добавим поле `ParserConfig` для pre-patch. В Phase 1 трогать
не нужно (поле появится в Phase 3 как required-input для новой
функции).

### Acceptance Phase 1

- `go build ./...` зелёный.
- `go vet ./...` зелёный.
- `go test ./...` зелёный (все существующие тесты, включая P1 / P4 /
  P5 / P6 / P7 / P8 регрессионные).
- Поведение в проде: preset.outbounds **временно не работает** (фича
  как у post-053 — выключена). Это намеренно: лучше отсутствие фичи,
  чем сломанная фича. Будет включена обратно в Phase 3+.
- В `bin/wizard_template.json` `ru-inside` / `russian` / `ru-blocked`
  снова имеют `proxy-out` / `auto-proxy-out` с `filters: !RU` и
  global `ru VPN 🇷🇺` — как было в post-053. Эти preset'ы продолжают
  работать как раньше через rule_set + globals.

---

## Phase 2 — Types & loader

**Цель:** вернуть типы `PresetOutbound` и loader-валидацию, но без
изменений в build pipeline.

### `core/template/preset_types.go`

Добавить:
```go
type PresetOutbound struct {
    Mode             string                 `json:"mode,omitempty"`         // "add" (default) | "update"
    Tag              string                 `json:"tag"`
    Type             string                 `json:"type,omitempty"`         // required for "add"
    Options          map[string]interface{} `json:"options,omitempty"`
    Filters          map[string]interface{} `json:"filters,omitempty"`
    AddOutbounds     []string               `json:"addOutbounds,omitempty"`
    PreferredDefault map[string]interface{} `json:"preferredDefault,omitempty"`
    Comment          string                 `json:"comment,omitempty"`
    Wizard           interface{}            `json:"wizard,omitempty"`
    If               []string               `json:"if,omitempty"`
    IfOr             []string               `json:"if_or,omitempty"`
}

// Preset уже существует:
type Preset struct {
    ...existing fields...
    Outbounds []PresetOutbound `json:"outbounds,omitempty"` // NEW
}
```

**Замечание:** структура **зеркально** повторяет
`configtypes.OutboundConfig` плюс control-fields `Mode/If/IfOr`. Это
намеренно — упрощает конверсию в Phase 3.

### `core/template/preset_loader.go::validatePresetOutbounds`

- `mode` ∈ `{"", "add", "update"}` — empty normalized to `"add"`,
  unknown → strip + warning.
- `tag` не пустой → warning + skip entry.
- `mode == "add"` → `type` обязателен; иначе warning + skip.
- `mode == "update"` → `type` warned (нельзя менять; drop в Phase 3).
- Tag uniqueness в пределах одного preset.
- `if` / `if_or` — references на existing bool vars.

### Tests `core/template/preset_outbounds_test.go`

7 test cases (по одному на каждое правило валидации + round-trip
parse/serialize).

### Acceptance Phase 2

- `go build ./...` зелёный.
- `go test ./core/template/...` зелёный (7 новых тестов проходят).
- Поведение в проде: preset.outbounds в template'е парсится, но
  build pipeline их **игнорирует** (Phase 3 ещё не сделан).

---

## Phase 3 — Pre-patch core

**Цель:** реализовать главную функцию архитектуры —
`ApplyPresetOutboundsToParserConfig`.

### Новый файл `core/build/preset_outbounds.go`

```go
package build

import (
    "singbox-launcher/core/config/configtypes"
    "singbox-launcher/core/template"
    "singbox-launcher/core/state/v6"
)

// ApplyPresetOutboundsToParserConfig возвращает копию parserCfg с применёнными
// preset.outbounds[] от всех enabled preset-refs (по RuleOrder).
//
// Mutates: nothing — оригинал parserCfg не тронут (deep-copy внутри).
//
// Algorithm:
//   1. Deep-clone parserCfg.ParserConfig.Outbounds[].
//   2. Walk active preset-refs in RuleOrder.
//   3. For each ref: ExpandPresetOutbounds(preset, vars) → []OutboundConfig + mode.
//   4. For each expanded entry:
//        mode="add":
//          - tag-collision с globals или earlier preset → first wins + warning
//          - identical body → silent skip
//          - else → append to clone
//        mode="update":
//          - lookup target в clone (по Tag)
//          - target отсутствует → warning + skip (no auto-create)
//          - apply field-merge: Filters→replace, AddOutbounds→union,
//            Options.*→per-field replace, Wizard→replace, Type/Tag→drop,
//            Comment/PreferredDefault→replace
//   5. Return new ParserConfig (copy) с patched Outbounds[].
func ApplyPresetOutboundsToParserConfig(
    parserCfg *configtypes.ParserConfig,
    presets []template.Preset,
    refs []v6.PresetRef,
    ruleOrder []v6.RuleOrderEntry,
) (*configtypes.ParserConfig, []string /* warnings */, error) {
    // implementation
}

// ExpandPresetOutbounds разворачивает preset.Outbounds в []OutboundConfig
// с заданными vars (substitution @var) и фильтрацией by if/if_or.
// Контрол-поля (mode, if, if_or) из вывода удаляются — это парсер-формат.
func ExpandPresetOutbounds(
    preset *template.Preset,
    vars map[string]string,
) (entries []presetOutboundEntry, warnings []string) {
    // implementation
}

// presetOutboundEntry — internal: разделяет mode и OutboundConfig.
type presetOutboundEntry struct {
    Mode    string // "add" | "update"
    Config  configtypes.OutboundConfig
    PresetID string // для warning messages
}
```

### Field-merge helper

```go
// applyOutboundUpdate патчит target типизированно. Возвращает новую
// структуру (target не мутируется).
func applyOutboundUpdate(target configtypes.OutboundConfig, patch configtypes.OutboundConfig) configtypes.OutboundConfig {
    out := target
    // Filters — replace целиком (если задано)
    if patch.Filters != nil {
        out.Filters = patch.Filters
    }
    // AddOutbounds — union (preserve order, dedupe)
    if len(patch.AddOutbounds) > 0 {
        out.AddOutbounds = unionStringList(target.AddOutbounds, patch.AddOutbounds)
    }
    // Options.* — per-field replace (только заданные в patch)
    if len(patch.Options) > 0 {
        if out.Options == nil {
            out.Options = map[string]interface{}{}
        } else {
            out.Options = cloneOptions(target.Options)
        }
        for k, v := range patch.Options {
            out.Options[k] = v
        }
    }
    // PreferredDefault — replace
    if patch.PreferredDefault != nil {
        out.PreferredDefault = patch.PreferredDefault
    }
    // Wizard — replace
    if patch.Wizard != nil {
        out.Wizard = patch.Wizard
    }
    // Comment — replace
    if patch.Comment != "" {
        out.Comment = patch.Comment
    }
    // Type, Tag — намеренно НЕ меняем (immutable)
    return out
}
```

### Tests `core/build/preset_outbounds_test.go`

| Case | Что проверяем |
|---|---|
| add-basic | 1 preset, 1 add → outbound появился в parser_config copy |
| add-collision-globals | preset add tag совпадает с global → first wins (global), warning |
| add-collision-preset | 2 preset add same tag → first by RuleOrder wins |
| add-identical | identical body → silent skip без warning |
| add-disabled | disabled preset → не применяется |
| update-basic | mode=update патчит proxy-out filters → patched |
| update-missing | mode=update на несуществующий tag → warning, no-op |
| update-type | mode=update с type → drop type + warning |
| update-multi | 2 preset update one tag → applied in RuleOrder |
| addOutbounds-union | preset.addOutbounds += target.addOutbounds (unique) |
| filters-replace | preset.filters заменяет target.filters целиком |
| options-per-field | preset.options.default заменяет только default, остальное preserved |
| original-immutability | original parserCfg.Outbounds НЕ тронут |
| empty-presets | пустой список refs → возвращает копию без изменений |

### Acceptance Phase 3

- `go build ./...` зелёный.
- `go test ./core/build/...` зелёный (14 новых тестов).
- Поведение в проде: функция готова, но **никто её не зовёт** (Phase 4
  ещё не сделан).

---

## Phase 4 — Wire pre-patch в build pipeline

**Цель:** перенаправить save-path и preview-path на patched ParserConfig.

### `core/build/build.go`

- В `BuildContext` добавить:
  ```go
  ParserConfig *configtypes.ParserConfig // pre-patched (был applied preset.outbounds)
  ```
- В `BuildConfig` (top-level) — если `ctx.ParserConfig != nil` И есть
  active preset-refs с outbounds — pre-patch применяется **до**
  вызова native generator'а.

**Важно:** не модифицируем case `"outbounds"` в `buildSection`. Native
generator продолжает работать с тем `ParserConfig`, который ему
передан. Pre-patch — это вход pipeline.

### `core/config_service.go::buildContextFromState`

```go
func (ac *AppController) buildContextFromState(s *state.State, cache *build.ParsedCache) build.BuildContext {
    ctx := build.BuildContext{...}

    // SPEC 056: pre-patch parser_config от preset.outbounds (mode=add + mode=update).
    // Mutates: ничего — оригинал в template остаётся как был.
    presets, _ := template.LoadPresets(ac.FileService.ExecDir)
    patched, warnings, err := build.ApplyPresetOutboundsToParserConfig(
        templateData.ParserConfig,
        presets,
        s.RulesV6.PresetRefs,
        s.RulesV6.RuleOrder,
    )
    if err == nil {
        ctx.ParserConfig = patched
        for _, w := range warnings {
            debuglog.WarnLog("preset outbounds: %s", w)
        }
    }

    return ctx
}
```

### `ui/configurator/business/create_config.go::BuildPreviewConfig`

То же самое — pre-patch применяется в preview-path для консистентности.

### Acceptance Phase 4

- `go build ./...` зелёный.
- `go test ./...` зелёный (все тесты, включая 14 новых из Phase 3).
- Поведение в проде:
  - preset с `outbounds[]` влияет на финальный `config.outbounds[]`.
  - mode=update на `proxy-out` / `auto-proxy-out` действительно
    меняет их filters.
  - mode=add добавляет новый selector в конец `outbounds[]`.
  - Финальный config проходит `sing-box check` (нет launcher-only
    полей — native generator их не эмитит).

---

## Phase 5 — Route post-pass cleanup

**Цель:** dangling outbound refs в `route.rules[]` → fallback на
`route.final` или drop.

### `core/build/preset_outbounds.go`

```go
// cleanDanglingOutboundRefInRule — если rule.outbound ∉ finalTags,
// replaces with fallback (route.final). Если fallback пуст → returns nil
// (rule должен быть удалён caller'ом). Sentinel reject/drop preserved.
func cleanDanglingOutboundRefInRule(rule map[string]interface{}, finalTags map[string]bool, fallback string) map[string]interface{} {
    // implementation
}

// CleanDanglingOutboundsInRouteRules проходит по route.rules[] и
// чистит dangling refs. Возвращает изменённый raw route + warnings.
func CleanDanglingOutboundsInRouteRules(routeRaw json.RawMessage, finalTags map[string]bool, fallback string) (json.RawMessage, []string, error) {
    // implementation
}
```

### Wire в `build.go::buildSection` case "route"

```go
case "route":
    merged, err := MergePresetsIntoRoute(...)  // existing (053)
    if err != nil { return "", err }

    // SPEC 056: clean dangling outbound refs. Срабатывает только когда
    // в state есть active preset-ref'ы (real scenario: preset removed/
    // disabled, остались dangling refs). Скип в preview (cache может
    // быть неполный — наследуем 0c3dce5).
    if !ctx.ForPreview && hasAnyV6Rule(ctx.Preset.RulesV6) && len(ctx.allOutboundTags) > 0 {
        merged, _ = CleanDanglingOutboundsInRouteRules(merged, ctx.allOutboundTags, ctx.Route.FinalOutbound)
    }
    return FormatSectionJSON(merged, 2)
```

### `ctx.allOutboundTags` populated

В `buildContextFromState` собирается из **patched** parser_config (т.е.
включая preset.outbounds add'ы) после Phase 4 pre-patch.

### Tests

3 new test cases в `core/build/preset_outbounds_test.go`:
- `dangling-fallback` — rule references missing outbound, есть route.final → fallback applied
- `dangling-drop` — rule references missing outbound, route.final пуст → rule dropped
- `sentinel-preserved` — rule.outbound ∈ {"reject", "block"} sentinel-теги не удаляются даже если их нет в outbounds[]

### Acceptance Phase 5

- `go build ./...` зелёный.
- `go test ./...` зелёный (17 новых тестов).
- Поведение: user сделал rule с `outbound: "ru VPN 🇷🇺"`, потом disable
  `ru-inside` → rule сохраняется (на reload) и при build получает
  fallback на route.final. В preview скип (cache может быть stale).

---

## Phase 6 — UI integration

**Цель:** outbound picker (Rules tab) видит preset-emitted tags.

### `ui/configurator/business/outbound.go` (новый файл)

```go
package business

func GetAvailableOutbounds(model *wizardmodels.WizardModel) []string {
    tags := collectGlobalOutboundTags(model)
    tags = append(tags, collectActivePresetOutboundTags(model)...)
    return uniqSorted(tags)
}

// collectActivePresetOutboundTags — теги от mode="add" entries активных
// preset-refs. mode="update" не вводит новых тегов.
func collectActivePresetOutboundTags(model *wizardmodels.WizardModel) []string {
    // Walk model.PresetRefs, lookup template preset, для каждого enabled:
    //   ExpandPresetOutbounds(preset, vars) → entries
    //   collect entries[].Config.Tag для mode="add"
}
```

### `ui/configurator/tabs/rules_unified_rows.go`

В checkbox enable-callback: если preset имеет outbounds → вызвать
`refreshRulesTabFromPresenter` для обновления inline outbound selects
во всех rows.

**Внимание:** этот callback уже **частично** существует от 055 (был
снесён в Phase 1). Восстанавливается с anti-loop защитой из `dc4cf09` /
`0ecc403` — checkbox создаётся с `nil` OnChanged, потом ставится
`Checked` field напрямую, затем назначается OnChanged.

### Acceptance Phase 6

- `go build ./...` зелёный.
- `go test ./...` зелёный.
- Поведение: user enables `ru-inside` preset → `ru VPN 🇷🇺` появляется
  в outbound dropdown'е на Rules tab. Disable → исчезает.

---

## Phase 7 — Template content migration

**Цель:** перенести `!RU` фильтры из template-globals в preset'ы.

### `bin/wizard_template.json`

```diff
"parser_config": {
  "outbounds": [
    {
      "tag": "auto-proxy-out",
      "type": "urltest",
-     "filters": { "tag": "!/(🇷🇺)/i" },
      ...
    },
    {
      "tag": "proxy-out",
      "type": "selector",
-     "filters": { "tag": "!/(🇷🇺)/i" },
      ...
    },
-   {
-     "tag": "ru VPN 🇷🇺",
-     "type": "selector",
-     "options": { "default": "direct-out", "interrupt_exist_connections": true },
-     "filters": { "tag": "/(🇷🇺)/i" },
-     "addOutbounds": ["direct-out"],
-     "comment": "Proxy group for russian VPN"
-   },
    ...
  ]
},

"presets": [
  {
    "id": "ru-inside",
+   "outbounds": [
+     { "mode": "update", "tag": "proxy-out",      "filters": { "tag": "!/(🇷🇺)/i" } },
+     { "mode": "update", "tag": "auto-proxy-out", "filters": { "tag": "!/(🇷🇺)/i" } },
+     {
+       "tag": "ru VPN 🇷🇺",
+       "type": "selector",
+       "options": { "default": "direct-out", "interrupt_exist_connections": true },
+       "filters": { "tag": "/(🇷🇺)/i" },
+       "addOutbounds": ["direct-out"]
+     }
+   ],
    ...
  },
  {
    "id": "russian",
+   "outbounds": [
+     { "mode": "update", "tag": "proxy-out",      "filters": { "tag": "!/(🇷🇺)/i" } },
+     { "mode": "update", "tag": "auto-proxy-out", "filters": { "tag": "!/(🇷🇺)/i" } }
+   ],
    ...
  },
  {
    "id": "ru-blocked",
+   "outbounds": [
+     { "mode": "update", "tag": "proxy-out",      "filters": { "tag": "!/(🇷🇺)/i" } },
+     { "mode": "update", "tag": "auto-proxy-out", "filters": { "tag": "!/(🇷🇺)/i" } }
+   ],
    ...
  }
]
```

### `RequiredTemplateRef` bump

В `internal/constants/constants.go` — bump на новую template-версию
(чтобы старые юзерские config'и подтянули обновление).

### Manual QA

1. Открыть Configurator с включённым `ru-inside` preset.
2. Сохранить → config.json должен пройти `sing-box check`.
3. `config.outbounds[]::proxy-out::outbounds` НЕ содержит RU-tagged
   nodes (фильтр работает через mode=update в preset'е).
4. `config.outbounds[]` содержит `ru VPN 🇷🇺` selector с RU-tagged
   nodes + `direct-out`.
5. Disable `ru-inside` → save → config.json: `proxy-out` снова имеет
   все ноды, `ru VPN 🇷🇺` исчез.

### Acceptance Phase 7

- `go build ./...` зелёный.
- `go test ./...` зелёный.
- Manual QA шаги 1–5 проходят.

---

## Phase 8 — Tests + golden + docs

### Golden fixtures

- `core/build/testdata/golden/preset_outbounds_add.json`
- `core/build/testdata/golden/preset_outbounds_update.json`
- `core/build/testdata/golden/preset_outbounds_disabled.json`
- `core/build/testdata/golden/preset_outbounds_multi_update.json`

Каждый fixture проверяется через **дополнительный** запуск
`sing-box check` в CI (если binary доступен) — acceptance #1.

### Docs

- `docs/release_notes/upcoming.md` — entry SPEC 056 (EN + RU):
  «Fix outbound emit regression introduced in SPEC 055 (preset.outbounds
  now correctly resolves filters/addOutbounds via native parser
  pipeline)».
- `docs/ARCHITECTURE.md` — раздел про pre-patch parser_config (если
  было раздело про SPEC 053 — добавить дополнение).
- `SPECS/056-B-N-OUTBOUNDS_PARSER_RESTORE/IMPLEMENTATION_REPORT.md` —
  финальный отчёт.

### Acceptance Phase 8

- `go build ./...`, `go vet ./...`, `go test ./...` все зелёные.
- Все 5 acceptance criteria из SPEC 056 выполняются.
- IMPLEMENTATION_REPORT.md написан.

---

## Risk mitigations

| Risk | Mitigation |
|---|---|
| ParserConfig deep-clone забыт где-то → mutation оригинала | Тест `original-immutability` в Phase 3 |
| Preview path и save path расходятся (разные ctx.ParserConfig) | Один и тот же `ApplyPresetOutboundsToParserConfig` вызывается в обоих местах |
| Performance regression — deep-clone parser_config на каждый rebuild | parser_config небольшой (~10-30 outbounds), clone-cost микросекунды. Profile if needed. |
| User имел state с rule references на preset-tag после downgrade | Phase 5 fallback ловит. Preview скипает (защищено P8). |
| Тег с emoji (`ru VPN 🇷🇺`) в filters regex | sing-box поддерживает UTF-8 в tag'ах; в native pipeline это уже работает. Тест в Phase 3 |
| Backward compat: старый template без preset.outbounds[] | `Outbounds []PresetOutbound` с omitempty → no-op pre-patch |

---

## Out of scope (future)

- **SPEC 057** preset cross-references (explicit dependency между preset'ами)
- **SPEC 058** preset.outbounds.mode = "replace" (destructive full-replace)
- **SPEC 059** preset.inbounds (per-preset inbound configuration)
- **Template authoring docs** — описать что можно/нельзя в preset.outbounds[]
