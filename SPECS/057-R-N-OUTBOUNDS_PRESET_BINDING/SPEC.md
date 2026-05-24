# SPEC 057-R-N — OUTBOUNDS_PRESET_BINDING

**Status:** New (N)
**Type:** Refactor (R) — architectural cleanup, no new user-facing features
**Depends on:** SPEC 055 (preset.outbounds), SPEC 056-R-N (resolver pattern для DNS)
**Не меняет:** v6 state schema version; sing-box config.json format

---

## Проблема

После SPEC 055 (preset.outbounds) + SPEC 056-R-N (DNS resolver) outbound'ы — единственная секция, где **preset binding не персистится в state**:

- Preset add outbound (`{tag: "ru VPN 🇷🇺", ref: "russian"}`) — синтетический, materialized at render time через `ApplyPresetOutboundsToParserConfig(empty)`. UI рисует через `collectPresetOutboundRows`. На reload — пересоздаётся, **order теряется**.
- Preset update outbound (`{mode: "update", tag: "proxy-out", filters: {...}}`) — патч применяется на build-time, но в state.outbounds[].filters стоит patched body **без знания** что это от preset. На disable preset patch не откатывается (просто перестаёт применяться при следующем build).

Результат: UI/state/build paths рассинхронизированы, preset reorder требует костыля (display-order map в памяти), revert update'а невозможен.

DNS уже решил это через `kind=preset, ref=...` entries в `state.dns_options.servers[]` + `SyncDNSOptionsWithActivePresets` lifecycle. Outbounds — последняя секция без этой симметрии.

## Целевая модель

`state.connections.outbounds[]` становится **единственным источником истины**. Preset binding выражается через два новых поля:

- **`ref: "<preset_id>"`** — outbound добавлен через `preset.outbounds[mode=add]`. Lifecycle: enable preset → entry добавляется; disable → удаляется.
- **`updates: [{ref, patch}]`** — стек patches от `preset.outbounds[mode=update]`. Lifecycle: enable preset → patch добавляется в стек; disable → удаляется. Финальный body = base + apply updates в order.

```json
"connections": {
  "outbounds": [
    {
      "tag": "proxy-out",
      "type": "selector",
      "options": {"interrupt_exist_connections": true, "default": "auto-proxy-out"},
      "addOutbounds": ["direct-out", "auto-proxy-out"],
      "updates": [
        {"ref": "russian",   "patch": {"filters": {"tag": "!/(🇷🇺)/i"}}},
        {"ref": "ru-inside", "patch": {"filters": {"tag": "!/(🇷🇺)/i"}}}
      ]
    },
    {
      "tag": "ru VPN 🇷🇺",
      "type": "selector",
      "options": {"default": "direct-out", "interrupt_exist_connections": true},
      "filters": {"tag": "/(🇷🇺)/i"},
      "addOutbounds": ["direct-out"],
      "ref": "russian"
    }
  ]
}
```

**Resolver pattern** (по образцу `ResolveDNS`):

```
state.connections.outbounds[] + template
              ↓
       ResolveOutbounds()
              ↓
   ResolvedOutbound{Body merged from base + updates, Ref, Updates[], ...}
              ↓
       ┌──────┴──────┐
       ▼             ▼
   UI render    Build emit
```

UI рендерит base из state directly + знает о ref/updates → kind-aware controls (preset row read-only, updates indicator на globals).
Build эмитит computed merged body в `config.json::outbounds[]`.

## Семантика полей

### `ref: string`

- Только на entries добавленных через `preset.outbounds[mode=add]`.
- Значение = `preset.id` владельца.
- UI: row read-only (lock edit body, no del — управляется через preset toggle).
- На disable parent preset: entry удаляется из state.outbounds[].
- На re-enable: entry создаётся заново с дефолтным positioning (append at end) — user может Up/Down его двигать, position сохраняется в state.outbounds[] slice index.

### `updates: [{ref, patch}]`

- Стек patches от `preset.outbounds[mode=update]`. Order: иммутабельный по времени применения (insertion order).
- На render/build: `merged = base; for each u in updates: merged = applyOutboundUpdate(merged, u.patch)`.
- На disable parent preset: соответствующая entry удаляется из updates[]. Merged пересчитывается → автоматически correct.
- Base (само outbound тело без updates) хранится в обычных полях outbound. updates — отдельный стек.

### Lifecycle: SyncOutboundsWithActivePresets

Аналогично `SyncDNSOptionsWithActivePresets`. Вызывается:
- На load после parseV6
- На каждый preset toggle в Rules tab
- Перед marshalDiskV6 (defensive)

Семантика:
```
для каждого state.RulesV6[kind=preset, enabled=true]:
  presetVars = build из preset.Vars + body.Vars
  для каждого preset.outbounds[i]:
    if active(if/if_or):  ← filter by template-vars
      if mode=add:
        ensure state.outbounds содержит {tag, ref=preset.id, ...body}
        body = substitute(preset.outbounds[i], presetVars)
      if mode=update:
        target = find state.outbounds by tag
        if target found:
          ensure target.updates содержит {ref=preset.id, patch=substituted_body}

для каждого state.outbounds[]:
  remove ref если preset disabled/missing
  remove updates entries с ref от disabled/missing preset
```

Idempotent.

## Schema changes

`core/config/configtypes/types.go::OutboundConfig`:

```go
type OutboundConfig struct {
    Tag              string                 `json:"tag"`
    Type             string                 `json:"type"`
    Options          map[string]interface{} `json:"options,omitempty"`
    Filters          map[string]interface{} `json:"filters,omitempty"`
    AddOutbounds     []string               `json:"addOutbounds,omitempty"`
    PreferredDefault map[string]interface{} `json:"preferredDefault,omitempty"`
    Comment          string                 `json:"comment,omitempty"`
    Wizard           interface{}            `json:"wizard,omitempty"`

    // SPEC 057-R-N: preset binding.
    Ref     string           `json:"ref,omitempty"`     // preset.id (для mode=add entries)
    Updates []OutboundUpdate `json:"updates,omitempty"` // preset.outbounds[mode=update] стек
}

// OutboundUpdate — одна запись в стеке обновлений от preset.outbounds[mode=update].
type OutboundUpdate struct {
    Ref   string                 `json:"ref"`   // preset.id
    Patch map[string]interface{} `json:"patch"` // patch fields (filters, options, addOutbounds, ...)
}
```

`Required` field остаётся отсутствующим (читается live из template как в SPEC 056-R-N Phase E).

## Build emit

`ApplyPresetOutboundsToParserConfig` — **удалить** (или превратить в noop, только warn если template содержит preset.outbounds но state не has matching entries — что значит sync function не отработал).

Outbound emit использует `ResolveOutbounds(state, template).Outbounds[]` — каждое entry с computed merged body. Order = slice index in state.outbounds[] (natural).

## UI render

`outbounds_configurator.collectRowsForUI(model)`:
- Walk state.outbounds[] напрямую. Для каждого:
  - `ref != ""` → row marked `IsPreset=true`, source label = "🔒 from preset X", read-only
  - `updates non-empty` → row marked `HasPresetUpdates=true`, source label = "Global ⚠ modified by N preset(s)", hover показывает refs
  - иначе → обычный global, full edit
- Up/Down → natural slice swap (moveOutboundUp/Down), persists в state
- Удалить: `collectPresetOutboundRows`, `OutboundDisplayOrder`, synthetic rows logic, `reorderRowDisplay` helpers.

## Backward compat

Existing state.json (no ref, no updates fields):
- JSON unmarshal: missing fields → zero value. ref="" → treated as user/template global. updates=nil → no patches.
- На первом Load + active preset rule: `SyncOutboundsWithActivePresets` обнаружит missing ref entries для active preset, **добавит их**. Sync idempotent — повторный вызов с тем же state не меняет ничего.

Pure v5 state path: parseV5 → OutboundConfig parsed without ref/updates → naturally treated as user. Sync function НЕ вызывается на v5 path (нет state.RulesV6 для walk).

## Acceptance

1. `state.connections.outbounds[]` после Save содержит preset-add entries с `ref="<preset_id>"` и preset-update patches в `updates[]`.
2. На disable preset rule в Rules tab → SyncOutboundsWithActivePresets удаляет соответствующие entries и updates. config.json без этих outbound'ов после Update.
3. Reorder preset row через Up/Down → natural slice swap в state, **позиция персистится** при reload.
4. Edit на preset row (`ref != ""`) **заблокирован** (UI не показывает Edit кнопку или показывает disabled).
5. Body merged через resolver: base + apply updates в order. Юзер видит final body в UI.
6. **Удалено:**
   - `collectPresetOutboundRows` (синтетические rows)
   - `model.OutboundDisplayOrder` (display-only map больше не нужен)
   - `reorderRowDisplay` helpers
   - `ApplyPresetOutboundsToParserConfig` (build path не нуждается в runtime apply)
   - `PresetOutboundAddTags` helper (через ref на entry читается)
7. Existing state.json без новых полей загружается без warning'ов; preset entries добавляются автоматом при первом sync.
8. Round-trip тесты: enable preset → save → load → state idempotent. Disable preset → save → load → state не содержит удалённых entries.

## Не в скоупе

- v5 state path (только v6)
- SPEC 053 preset bundles концепт (остаётся как есть)
- SPEC 056-R-N DNS schema (остаётся как есть; symmetric pattern)
- UI display-order persistence для **обычных globals** — они уже persist через slice index в state.outbounds[]

## План фаз

1. **Phase 1 — Schema:** добавить `Ref` + `Updates []OutboundUpdate` в `OutboundConfig`. Helper `applyOutboundUpdatePatch(base, patch)` для merge одного patch. `ResolveOutbound(base, updates) → merged`.

2. **Phase 2 — `SyncOutboundsWithActivePresets`:** lifecycle функция в `core/state/v6/sync_outbounds.go` (new). Параметры: state.RulesV6 + state.Connections.Outbounds + template.Presets. Mutates state.Connections.Outbounds in-place.

3. **Phase 3 — `ResolveOutbounds`:** в `core/build/resolve_outbounds.go` (new). Pure func: state+template → `[]ResolvedOutbound{Body merged, Ref, Updates, IsPreset, HasPresetUpdates, Required}`.

4. **Phase 4 — Build switch:** `ApplyPresetOutboundsToParserConfig` удалена. Outbound emit использует merged body из state. Тесты обновляются.

5. **Phase 5 — UI switch:** `collectRowsForUI` упрощается — читает state directly через `ResolveOutbounds`. Удалить `collectPresetOutboundRows`, `OutboundDisplayOrder`, `reorderRowDisplay`. Preset row UI: ref → read-only badges.

6. **Phase 6 — Migration:** `SyncOutboundsWithActivePresets` вызывается из presenter на Load + Rules tab toggle.

7. **Phase 7 — Cleanup:** удалить `PresetOutboundAddTags`, `templateRequiredTags` runtime overlay (заменяется на `Required` атрибут через resolver), other dead helpers.

8. **Phase 8 — Tests + reinstall.**

## Deviations from original plan (post-implementation notes)

Реальная реализация отклонилась от первоначального плана в нескольких местах. Зафиксировано здесь для следующих SPEC'ов и для понимания текущей кодовой базы.

### 1. `ApplyPresetOutboundsToParserConfig` **не удалена** (см. Phase 4)

План: удалить функцию, runtime использует merged body из state.
Реально: функция удалена из runtime callers (3 точки: `rebuild_raw_cache.go`, `config_service.go`, `ui/configurator/business/parser.go`), но **исходный код функции выпилен полностью** в `core/build/preset_outbounds.go`. Runtime использует **новый helper** `MergeOutboundUpdatesInPlace` (флэттенит `Updates[]` стек в body) после `SyncOutboundsWithActivePresets`.

Почему: generator (`GenerateOutboundsFromParserConfig`) не знает поле `Updates`. Чтобы patches от preset mode=update применились в финальный config.json, нужно их материализовать в base body до передачи в generator. `mergeOutboundUpdates` уже был в SPEC через `ResolveOutbounds` per-entry; вынесли в `MergeOutboundUpdatesInPlace` как mass-mutate helper для slice.

### 2. `templateRequiredTags` **сохранена** (см. Phase 7)

План: удалить как redundant overlay, replaced by `Required` атрибут через resolver.
Реально: оставлена. `ResolveOutbounds` принимает `requiredTags map[string]bool` параметром, а не вычисляет внутри — это позволяет источнику истины (template) меняться без миграции state. UI читает `templateRequiredTags(model)` live на каждый render. State.json `required` не персистит (если бы персистил, снять флаг в template'е после Save было бы невозможно). Аналогично SPEC 056-R-N Phase E подходу для DNS.

### 3. Adopt-on-first-sync для legacy state

План: не предусмотрен.
Реально: добавлен в `SyncOutboundsWithActivePresets`. Если existing global без `Ref` имеет tag совпадающий с preset.add — entry adopt'ится (`Ref` присваивается). Без этого юзеры pre-SPEC-057 с `ru VPN 🇷🇺` промоутнутым в global теряли preset binding при обновлении.

**Edge case:** юзер создал custom outbound с tag, совпадающим с preset add → entry станет preset-locked. На практике маловероятно (теги типа `ru VPN 🇷🇺` — preset territory). Если потребуется strict — добавить `_user_owned: true` маркер.

### 4. `parseAndPreview` — отдельный `parserConfigForGen` (не из плана)

Bug: `parseAndPreview` мутировал `model.ParserConfig` мерженной (flat) копией после `MergeOutboundUpdatesInPlace`. Save писал в state body со слитыми patch'ами, без `updates[]` стека.

Fix: разделили `parserConfig` (unmerged, для model — сохраняет Updates[]) и `parserConfigForGen` (merged deep-copy — только для generator'а). Model.ParserConfig получает unmerged → Save пишет правильный state shape.

### 5. `syncConnectionsFromLegacy` adapter conflict (не из плана)

Bug: `core/state/save.go:39` вызывает `syncConnectionsFromLegacy`, которая копирует `state.ParserConfig.Outbounds` → `state.Connections.Outbounds`, затирая Sync'нутые `updates[]`.

Fix: в `CreateStateFromModel` теперь Sync вызывается на **обе view'а** (`state.Connections.Outbounds` и `state.ParserConfig.ParserConfig.Outbounds`). Adapter копирует уже синхронизированную версию.

### 6. `RefreshAfterPresetToggle` consolidation (post-Phase 6)

План Phase 6: hook на Rules tab toggle. Реальный hook вызывал sync-логику inline (~25 строк).

Рефактор: единый presenter-метод `RefreshAfterPresetToggle()` рядом с `RefreshDNSListAndSelects` в `presenter_sync.go`. Делает все 4 шага: DNS UI refresh, outbounds eager sync (GlobalOutbounds + ParserConfig.Outbounds + UpdateParserConfig), outbounds tab UI refresh, RefreshOutboundOptions. Toggle handler в `rules_unified_rows.go` теперь однострочный.

### 7. Disabled subscription cascade — 4-я точка фикса (не из плана)

`business/outbound.go::GetAvailableOutbounds` (Rules tab outbound dropdown) walk'ал `parserCfg.ParserConfig.Proxies` без проверки `Disabled`. Юзер видел `BL:auto`, `BL:select` от выключенных подписок в dropdown'ах правил → dangling outbound на эмите.

Fix: добавлен `if proxySource.Disabled { continue }`. Теперь все 4 точки фильтруют disabled подписки одинаково: `RebuildPreviewCache`, `collectRows`, `collectAllTags`, `GetAvailableOutbounds`.

### 8. UI niceties (вне SPEC scope, но в одной session)

- **Restore missing outbounds** кнопка в Outbounds tab (слева от Add) — восстанавливает удалённые template entries. Иконка 🔄, без текста, tooltip.
- **Add** кнопка с иконкой ➕ для визуальной симметрии.
- **Reset / Del** одинаковая ширина через `container.NewStack` с прозрачным sizer'ом 78×0px (раньше колонка "прыгала" между rows).
- **Library Rules dialog** — `already added` checkbox теперь pre-checked + disabled (как required DNS), а не пустой + disabled. Визуальный паритет с DNS tab.

## Риск

Средний. Существующие state.json юзеров могут содержать preset-modified outbounds (filters/addOutbounds patched в-местe template's body). На первом Sync с активным preset:
- Если patched body совпадает с computed `base + preset.update.patch` → noop, sync just adds updates[] entry
- Если расходится → юзер изменил руками. Sync уважает текущий body как base, добавляет updates[] entry. Может быть сюрприз для юзера, но не data loss.

Mitigation: на Save после первого sync writes new state shape; backup `.pre-057.bak` создаётся one-shot если detected legacy state (по аналогии с .v5.bak).

Build emit ДОЛЖЕН быть byte-identical до и после рефактора для существующего state. Golden test обязателен.
