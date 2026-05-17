# SPEC 055 — Implementation Plan

## Phase order

1. **Types & loader** (1 день)
2. **Expand & merge pipeline** (1 день)
3. **Build integration + dangling cleanup** (0.5 дня)
4. **UI: extended GetAvailableOutbounds** (0.5 дня)
5. **Template content migration** (0.25 дня)
6. **Tests + golden fixtures + docs** (1 день)

**Total estimate:** ~4 рабочих дня.

---

## Phase 1 — Types + loader

### `core/template/preset_types.go`
- Add `Outbounds []PresetOutbound` field в `Preset` struct
- New type `PresetOutbound`:
  ```go
  type PresetOutbound struct {
      Mode         string                 `json:"mode,omitempty"`         // "add" (default) | "update"
      Tag          string                 `json:"tag"`
      Type         string                 `json:"type,omitempty"`         // required for "add"
      Options      map[string]interface{} `json:"options,omitempty"`
      Filters      map[string]interface{} `json:"filters,omitempty"`
      AddOutbounds []string               `json:"addOutbounds,omitempty"`
      Comment      string                 `json:"comment,omitempty"`
      Wizard       map[string]interface{} `json:"wizard,omitempty"`
      If           []string               `json:"if,omitempty"`
      IfOr         []string               `json:"if_or,omitempty"`
  }
  ```
- `Options` оставляем как `map[string]interface{}` — поля могут быть очень разные (selector / urltest / direct / shadowsocks ...).

### `core/template/preset_loader.go`
- Внутри `validatePreset`: новый блок `validatePresetOutbounds`:
  - mode ∈ {"", "add", "update"} (пусто = "add")
  - tag непустой
  - mode=add → type обязателен
  - mode=update → type должен быть пустым (или drop с warning)
  - Tag uniqueness в пределах одного preset
  - if/if_or — references на vars existing (bool типа)

### Tests `preset_loader_test.go`
- Каждый validation case — отдельный sub-test
- Round-trip parse/serialize нового preset с outbounds

---

## Phase 2 — Expand + merge

### `core/build/preset_expand.go`
- Расширить `Fragments` структуру:
  ```go
  type Fragments struct {
      RuleSets   []map[string]interface{}
      DNSServers []map[string]interface{}
      Rules      []map[string]interface{}
      DNSRule    map[string]interface{}
      Outbounds  []PresetOutboundExpanded   // NEW
  }
  type PresetOutboundExpanded struct {
      Mode    string
      Tag     string  // НЕ префиксуется preset_id! Outbound tag — user-facing global namespace.
      Body    map[string]interface{}   // финальный JSON-ready outbound (после @var-substitution)
  }
  ```
- В `ExpandPreset`:
  - Применить `_substitute(outbound, vars)` — внутри options/filters могут быть `@var` ссылки
  - Фильтр по `if`/`if_or`
  - Validate type immutability для mode=update — drop поле type если есть
  - Эмит в `Fragments.Outbounds`

### `core/build/preset_merge.go`
- Новая функция `MergePresetsIntoOutbounds(baseOutbounds []map[string]interface{}, presetRefs ...) []map[string]interface{}`
  - Алгоритм согласно SPEC: emit map by tag, iterate presets in RuleOrder
  - mode=add: identical-skip ИЛИ first-wins (с warning)
  - mode=update: lookup target, apply field-level merge rules
  - Return new outbounds list в правильном порядке (preserve original order + append new)
- Helper `applyOutboundUpdate(target, patch map[string]interface{}, warningLog func)`:
  - filters → replace whole map
  - addOutbounds → union (use sets to dedupe)
  - options.* → replace per-field
  - wizard.* → replace per-field
  - type → drop + warning
  - tag → drop (can't change)
  - comment → replace

### Tests `preset_merge_test.go`
- Add basic case: 1 preset adds 1 outbound
- Update basic case: 1 preset patches proxy-out filters
- Collision: 2 presets add same tag → first wins
- Collision: globals has tag → preset add ignored
- Update missing target → warning, no-op
- Update with `type` change → field dropped
- Union semantics для addOutbounds
- Multi-update chain: preset A updates → preset B updates → expected result

---

## Phase 3 — Build integration + cleanup

### `core/build/build.go`
- В `BuildContext` уже есть `Preset` (от SPEC 053). Используем.
- В функции эмитящей `config.outbounds[]`:
  - После старого builder'а (resolve filters + emit base) → вставить `MergePresetsIntoOutbounds`

### `core/build/preset_merge.go` — расширить
- `cleanDanglingOutboundRefInRule(rule map[string]interface{}, emittedTags map[string]bool, fallback string) map[string]interface{}`:
  - Если `rule["outbound"]` указывает на tag не в emittedTags → заменить на fallback (`route.final`)
  - Если fallback пустой → удалить rule entirely (return nil)
- В `MergePresetsIntoRoute`: после клина dangling rule_set refs → пройти cleanDanglingOutboundRefInRule

### Tests
- Build integration test: preset с outbounds + rule использующий @out → config.outbounds содержит, route.rules использует
- Dangling: user rule с outbound=ru VPN 🇷🇺, preset ru-inside disabled → rule fallback to route.final

---

## Phase 4 — UI

### `ui/configurator/business/wizard_outbound.go` (новый файл, или existing)
- Расширить `GetAvailableOutbounds(model)`:
  ```go
  func GetAvailableOutbounds(model *WizardModel) []string {
      tags := collectGlobalOutboundTags(model)   // existing
      tags = append(tags, collectActivePresetOutboundTags(model)...)  // NEW
      return uniqSorted(tags)
  }
  func collectActivePresetOutboundTags(model *WizardModel) []string {
      // Walk model.PresetRefs, lookup template preset, for each enabled:
      //   ExpandPreset (lightweight — without rule_set download check)
      //   Collect frags.Outbounds[i].Tag (только mode != "update")
  }
  ```

### `ui/configurator/tabs/rules_unified_rows.go::buildSinglePresetRefRow`
- Inline outbound select уже использует `availableOutbounds` — без изменений в widget code
- Просто на следующем render preset-emitted tags появятся в options

### Reactive refresh
- При toggle enable/disable preset с outbounds — `presenter.RefreshDNSListAndSelects()` уже зовётся (для DNS); добавить также `RefreshOutboundSelects()` или совместить
- На каждый rebuild Rules tab — availableOutbounds пересчитывается

### Tests (UI tests минимальные — preserved scope)

---

## Phase 5 — Template content migration

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
  ...
  {
    "id": "ru-inside",
    ...
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
  }
]
```

Также применить к `ru-blocked` (если он тоже завязан на ru VPN), `russian` если ему нужен.

### Validation
- Bump `RequiredTemplateRef`
- Manual test: open Configurator с включённым ru-inside → config.json идентичен старому
- Disable ru-inside → config.json чище (proxy-out без фильтра, ru VPN исчез)

---

## Phase 6 — Tests + docs

### Golden fixtures
- `core/build/testdata/golden/preset_outbounds_add.json`
- `core/build/testdata/golden/preset_outbounds_update.json`
- `core/build/testdata/golden/preset_outbounds_disabled.json` (нет preset → config без preset.outbounds эффектов)
- `core/build/testdata/golden/preset_outbounds_multi_update.json` (2 preset'а патчат один tag в порядке)

### docs

- `docs/release_notes/upcoming.md` — entry SPEC 055
- `docs/ARCHITECTURE.md` — раздел preset.outbounds (если ARCHITECTURE.md был обновлён для SPEC 053; иначе пропустить)

### IMPLEMENTATION_REPORT.md
- Финальный отчёт после landing

---

## Risk mitigations

| Risk | Mitigation |
|---|---|
| Преждевременная сложность (cross-preset update chains) | Применять updates строго в RuleOrder, не пытаться оптимизировать |
| Breakage у юзеров с активным ru-inside | Template content migration делается так, что итоговый config.json идентичен для тех у кого preset был enabled. Для disabled — мягкая чистка |
| Forgot to update UI getAvailableOutbounds | Test: создать preset, enable, проверить что `out` var dropdown содержит preset-emitted tag |
| `mode: "update"` race с GC orphan rule-sets | Outbounds не имеют отношения к bin/rule-sets/ — risk не пересекается |

---

## Out of scope (отдельные SPEC'и если понадобятся)

- **SPEC 056** preset.inbounds — preset может добавлять inbound (например per-user proxy-in port для split-traffic-case)
- **SPEC 057** preset cross-references — preset A может ссылаться на outbound preset'а B (с явным dependency declaration)
- **SPEC 058** preset.outbounds.mode = "replace" — destructive full-replace для редких случаев

---

## Sequencing

Не блокирует другие текущие задачи (SPEC 053 закрыт; UI tweaks current session — отдельный коммит). Можно начинать как только SPEC 053 stabilize'ируется в продакшене (~1-2 версии).

Рекомендация: **отложить старт на 1-2 недели** после shipping SPEC 053 — соберём feedback по preset bundles в целом, потом продолжим расширением.
