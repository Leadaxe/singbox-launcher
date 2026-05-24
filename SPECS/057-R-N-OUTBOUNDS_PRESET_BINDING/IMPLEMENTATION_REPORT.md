# SPEC 057-R-N — Implementation Report (FINAL)

**Status:** shipped (uncommitted, на review для v0.9.6)
**Дата:** 2026-05-24

Этот SPEC закрывает **последний разрыв** между SPEC 053 (preset bundles) / SPEC 055 (preset.outbounds) / SPEC 056-R-N (DNS resolver pattern) и **outbound-секцией** state'а. После реализации все три большие секции конфига — DNS, route, outbounds — следуют одному паттерну: state — single source of truth, preset binding выражается через дискриминатор (`kind` или `ref`), runtime — pure resolver на pre-synced state.

См. также `SPEC.md` § "Deviations from original plan" для отклонений от первоначального плана.

---

## Что в результате есть

### 1. Schema: `Ref` + `Updates[]` на `OutboundConfig`

```go
type OutboundConfig struct {
    // ... existing fields (Tag, Type, Options, Filters, ...)

    // SPEC 057-R-N: preset binding.
    Ref     string           `json:"ref,omitempty"`     // preset.id владельца (только для mode=add)
    Updates []OutboundUpdate `json:"updates,omitempty"` // стек patches от mode=update
}

type OutboundUpdate struct {
    Ref   string                 `json:"ref"`
    Patch map[string]interface{} `json:"patch"` // filters/options/addOutbounds/preferredDefault/wizard/comment
}
```

Backward compat: missing fields → zero value. Existing state.json без `ref`/`updates` грузится как user/template globals; первый Sync с активным preset adopt'ит legacy entries.

### 2. Lifecycle: `SyncOutboundsWithActivePresets`

`core/build/sync_outbounds.go` — единая точка mutation для state.outbounds[].

**Вызывается:**
- На Load после `parseV6` (idempotent)
- На preset toggle в Rules tab (через `RefreshAfterPresetToggle`)
- Перед `marshalDiskV6` (defensive — в `CreateStateFromModel`)
- В runtime build path (headless rebuild / UpdateConfigFromSubscriptions / parseAndPreview)

**Семантика:**
```
для каждого state.RulesV6[kind=preset, enabled=true]:
  для каждого preset.outbounds[i] (после vars-substitute + if/if_or):
    if mode=add:
      ensure state.outbounds содержит {tag, ref=preset.id, ...body}
      adopt-on-first-sync: если existing global с тем же tag без ref — set Ref
    if mode=update:
      ensure target.Updates[] содержит {ref=preset.id, patch=substituted_body}

для каждого state.outbounds[]:
  ref != "" но preset не active/missing → drop entry
  Updates[] entry с ref от disabled/missing preset → drop entry
```

Idempotent (поверженный вызов даёт тот же результат).

### 3. Resolver: `ResolveOutbounds` + `MergeOutboundUpdatesInPlace`

```
state.connections.outbounds[]
              ↓
      ResolveOutbounds()  ◄── единая pure func (по образцу ResolveDNS)
              ↓
   ResolvedOutbound{Body merged from base+updates, Ref, IsPreset, HasPresetUpdates, Required}
              ↓
       ┌──────┴──────┐
       ▼             ▼
   UI render    Build emit
```

`mergeOutboundUpdates(ob)` — pure per-entry: base body + apply each `u.Patch` через `applyOutboundUpdate` в order. Updates[] стек strip'ается из merged body (metadata, не идёт в config.json).

`MergeOutboundUpdatesInPlace(parserCfg)` — runtime helper: walk Outbounds[], replace body на merged. Используется build path'ами **после** Sync — generator (`GenerateOutboundsFromParserConfig`) не знает поле `Updates`.

### 4. Build pipeline switch

`ApplyPresetOutboundsToParserConfig` **удалена** полностью из `core/build/preset_outbounds.go` (~140 строк + 10 тестов). Все 3 runtime call sites переключены на пару `Sync → Merge`:

| Caller | Было | Стало |
|---|---|---|
| `core/rebuild_raw_cache.go:81` | `ApplyPresetOutboundsToParserConfig(td.Presets, s.RulesV6)` | `Sync(...)` + `Merge(...)` |
| `core/config_service.go:186` | то же | то же |
| `ui/configurator/business/parser.go:95` | то же | то же |

`parseAndPreview` дополнительно держит **две копии** parserConfig: `parserConfig` (unmerged, для model.ParserConfig — сохраняет Updates[] стек), `parserConfigForGen` (merged deep-copy — только для generator). Без этого Save писал бы слитый body без Updates[] (см. § Bug 4 ниже).

### 5. UI switch

`outbounds_configurator/configurator.go`:
- `collectRows` определяет preset rows через `ob.Ref != ""` (не через synthetic-row append)
- Удалены: `rowKey`, `reorderByDisplayOrder`, `reorderRowDisplay`, `indexOfString`, `collectPresetOutboundRows` (~170 строк)
- Up/Down handlers — direct `moveOutboundUp/Down` на slice (preset binding переезжает с элементом)
- Удалено: `model.OutboundDisplayOrder []string` (in-memory display-order карта)
- Visual для preset row: 🔒 + preset label, кнопки View + Up/Down (no Edit/Del)
- Visual для required row: 🔒 + Global, кнопки Edit + Reset + Up/Down (no Del)
- **Restore missing** кнопка слева от Add (иконка 🔄) — восстанавливает удалённые template outbound'ы

### 6. Unified preset-toggle hook

`presenter_sync.go::RefreshAfterPresetToggle()` — единая точка для всех аftermath'ов preset enable/disable. 4 шага:
1. DNS UI refresh (через существующую `RefreshDNSListAndSelects`)
2. Outbounds eager Sync (`model.GlobalOutbounds` + `model.ParserConfig.Outbounds` + `RefreshDerivedParserConfig` + `UpdateParserConfig`)
3. Outbounds tab UI refresh (`guiState.RefreshOutboundsConfiguratorList`)
4. `RefreshOutboundOptions` (Rules/Final dropdowns)

Toggle handler в `rules_unified_rows.go` — однострочник `presenter.RefreshAfterPresetToggle()`.

### 7. Disabled subscription cascade (4-я точка фикса)

`business/outbound.go::GetAvailableOutbounds` теперь пропускает `proxy.Disabled` подписки. До фикса юзер видел `BL:auto/select` (от выключенных подписок) в outbound dropdown'ах Rules tab → dangling refs на эмите. Все 4 кодовые точки теперь фильтруют единообразно:
- `core.RebuildPreviewCache`
- `outbounds_configurator.collectRows`
- `outbounds_configurator.collectAllTags`
- `business.GetAvailableOutbounds`

---

## State.json — финальная форма

```jsonc
{
  "version": 6,
  "connections": {
    "outbounds": [
      // template global без preset binding
      { "tag": "direct-out", "type": "direct" },

      // template global С update-патчами от 2 пресетов
      {
        "tag": "proxy-out",
        "type": "selector",
        "options": { "default": "auto-proxy-out", "interrupt_exist_connections": true },
        "addOutbounds": ["direct-out", "auto-proxy-out"],
        "comment": "Proxy group for everything that should go through VPN.",
        "updates": [
          { "ref": "russian",   "patch": { "filters": { "tag": "!/(🇷🇺)/i" } } },
          { "ref": "ru-inside", "patch": { "filters": { "tag": "!/(🇷🇺)/i" } } }
        ]
      },

      // preset add entry — ref + body
      {
        "tag": "ru VPN 🇷🇺",
        "type": "selector",
        "options": { "default": "direct-out" },
        "filters": { "tag": "/(🇷🇺)/i" },
        "addOutbounds": ["direct-out"],
        "ref": "russian"
      }
    ]
  },

  "rules": [
    { "kind": "preset", "ref": "russian",   "enabled": true,  "body": {"vars": {}} },
    { "kind": "preset", "ref": "ru-inside", "enabled": true,  "body": {"vars": {}} }
  ]
}
```

| Поле | Кто пишет | Кто читает | Когда меняется |
|---|---|---|---|
| `outbounds[].ref` | Sync на enable preset | UI (lock row), build emit (метаданные, не идёт в config.json) | enable/disable preset |
| `outbounds[].updates[]` | Sync на enable preset с mode=update | merger при render/emit (`mergeOutboundUpdates`) | enable/disable preset |
| `outbounds[].tag/type/options/filters/addOutbounds/...` | юзер через Edit (для не-ref entries) или Sync (для ref entries) | всё | юзер правит globals; preset правит ref entries |

---

## Архитектурные баги, найденные и пофикшенные в процессе

### Bug 1: Adopt-on-first-sync для legacy state

Симптом: `ru VPN 🇷🇺` промоутнутый старым "promote-to-global" подходом до SPEC 057 после установки 057 не получал `ref` — UI рендерил как обычный Global без 🔒.
Корень: Sync искал missing preset entries по tag в expectedAdds, но `findOutboundByTag(out, tag) >= 0` возвращало `true` для existing global → first-wins → preset add skip.
Фикс: добавлен `expectedAddRefByTag` map (tag → presetID). Если existing global без `Ref` имеет совпадающий tag — adopt (set Ref).

### Bug 2: `parseAndPreview` затирал Updates[] стек в model

Симптом: после Save state имел base body со слитыми filters/addOutbounds от preset patch'а, но `updates[]` стека НЕ было.
Корень: `parseAndPreview` делал `model.ParserConfig = &parserConfig` после `MergeOutboundUpdatesInPlace(&parserConfig)`. Merged (flat) копия попадала в model → Save сериализовал её в state.
Фикс: разделены `parserConfig` (unmerged, для model) и `parserConfigForGen` (merged deep-copy, только для generator). Model получает unmerged → state shape правильный.

### Bug 3: `syncConnectionsFromLegacy` adapter перезатирал Sync

Симптом: после Reset на required outbound и Save — Updates[] стек не появлялся в state, хотя preset был enabled.
Корень: `core/state/save.go:39` → `syncConnectionsFromLegacy` → `s.Connections.Outbounds = copy of s.ParserConfig.ParserConfig.Outbounds` (legacy view → canonical). Мой Sync работал только на `state.Connections.Outbounds`, адаптер затирал.
Фикс: в `CreateStateFromModel` Sync вызывается на **обе** view'а (Connections + ParserConfig.Outbounds). Адаптер копирует уже синхронизированную ParserConfig.

### Bug 4: preset toggle не триггерил Sync (Phase 6 не был реально wired)

Симптом: disable russian в Rules tab → `ru VPN 🇷🇺` остаётся в Outbounds tab. Только после Save выпадает.
Корень: SPEC 057 Phase 6 task list был помечен completed, но факти hook на toggle не существовал — Sync вызывался только в Save/Load.
Фикс: handler в `rules_unified_rows.go::buildPresetRefRow` теперь вызывает `presenter.RefreshAfterPresetToggle()`.

---

## Code metrics

| Изменение | Linеs delta |
|---|---|
| Новый код: `sync_outbounds.go`, `sync_outbounds_test.go`, `resolve_outbounds.go`, `resolve_outbounds_test.go` | +700 |
| Удалено: `ApplyPresetOutboundsToParserConfig` + tests | −350 |
| Удалено: UI synthetic preset rows + display-order helpers | −170 |
| Удалено: `model.OutboundDisplayOrder` + связь | −5 |
| Удалено: `cloneParserConfig`, `outboundsIdentical`, `PresetOutboundAddTags` | −50 |
| Изменено: 3 runtime callers, presenter (sync + refresh), 4 UI fix points | ~150 modified |
| **Чистый delta** | **~+150 (за счёт нового lifecycle/resolver) — но удаленного кода больше чем нового на UI стороне** |

Тесты: 14 новых unit-тестов (`sync_outbounds_test.go` 7 + `resolve_outbounds_test.go` 7). Все 23 пакета зелёные.

---

## Что осталось как **дефенсивный** оверхед (не критично)

1. **Двойной Sync в `CreateStateFromModel`** — на Connections и ParserConfig.Outbounds. Можно было бы починить адаптер, но это touch shared core code → risk. Двойной Sync дёшев (микросекунды).

2. **Legacy state с merged body в base** — у юзеров после первого Save с SPEC 057 base body имеет filters/addOutbounds от старого merge'а + Updates[] стек поверх. Merge идемпотентен (filters replace = same, addOutbounds union = dedup), эмит правильный. Но shape "избыточный". Чистка вручную в state.json возможна (убрать filters/addOutbounds из base) — Sync положит их в updates[] на следующем Save.

3. **`templateRequiredTags` parsing на каждый render** — JSON unmarshal раз в UI render. Микросекунды, OK; если станет bottleneck — можно мемоизировать по `model.TemplateData.ParserConfig` hash.

---

## Acceptance — статус

| # | Acceptance criterion | Статус |
|---|---|---|
| 1 | state.connections.outbounds[] после Save содержит preset-add entries с ref + Updates[] стеки | ✅ verified |
| 2 | На disable preset → entries и updates автоматически чистятся | ✅ verified |
| 3 | Reorder preset row через Up/Down — позиция персистится | ✅ verified |
| 4 | Edit на preset row заблокирован | ✅ (View вместо Edit) |
| 5 | Body merged через resolver: base + apply updates в order | ✅ verified |
| 6 | `collectPresetOutboundRows`, `OutboundDisplayOrder`, `reorderRowDisplay` удалены | ✅ |
|   | `ApplyPresetOutboundsToParserConfig` удалена | ✅ (с заменой на Sync+Merge) |
|   | `PresetOutboundAddTags` helper удалён | ✅ |
| 7 | Existing state.json без новых полей загружается без warning'ов | ✅ verified + adopt-on-sync |
| 8 | Round-trip: enable → save → load → state idempotent | ✅ verified |

---

## Зачем это нужно (one-paragraph elevator pitch)

После v0.9.5 outbounds — единственная секция конфига, в которой preset binding **не персистится в state**. Каждый рендер UI и каждый build пересоздавали preset entries из template + active rules. Reorder preset row требовал отдельной in-memory display-order карты (не персистила). Юзер не видел "это от preset X" в state.json. Disable preset не откатывал mode=update патчи — они просто переставали применяться на следующем build.

SPEC 057 переводит outbounds на ту же модель что SPEC 053 (preset bundles) + SPEC 056-R-N (DNS resolver): **state — single source of truth**, preset binding выражается через `ref` (для add) и `updates[]` стек (для update). Один lifecycle helper (`SyncOutboundsWithActivePresets`) приводит state в согласованный shape. Один resolver (`ResolveOutbounds`) дает merged view для UI и build. UI читает state directly, не пересоздавая preset entries на render.

Теперь DNS / route / outbounds — три параллельные ветки одного архитектурного pattern'а. Следующий SPEC может либо unify lifecycle helpers в один meta-pattern, либо двигаться к конкретным фичам (например, preset cascade — preset A enables preset B).
