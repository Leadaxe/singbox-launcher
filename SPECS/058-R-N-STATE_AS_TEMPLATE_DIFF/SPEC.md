# SPEC 058-R-N — STATE_AS_TEMPLATE_DIFF

**Status:** New (N)
**Type:** Refactor (R) — architectural cleanup, единая модель для template/preset/user outbound'ов
**Depends on:** SPEC 056-R-N (DNS resolver pattern), SPEC 057-R-N (outbounds preset binding via ref+updates)
**Не меняет:** sing-box config.json format; v6 state schema version.

---

## Проблема

После SPEC 057 у нас две разные модели для двух типов outbound'ов в `state.connections.outbounds[]`:

1. **Preset add entries** (`ref="<preset_id>"`) — thin ref, body live из `template.presets[id]`. Sync function поддерживает lifecycle через enable/disable preset. Изменения preset patch'ей в template приходят к юзеру автоматически.

2. **Template-derived + user globals** (`ref=""`) — self-contained full body в state. Snapshot template body на момент последнего save. Если template обновился (новый `addOutbounds`, изменился `comment`, появилось поле) — юзер этого **не получит** до явного Reset на required entry. Diff между template и state не виден.

Дополнительно: USER edits смешаны с template defaults в одном body. Нет способа быстро узнать «что юзер реально поменял», нет автоматического template auto-upgrade.

DNS уже решил эту проблему через SPEC 056 — `kind=template` entries хранят только tag+enabled, body live из template. Outbounds отстают.

## Целевая модель

Outbound entries в `state.connections.outbounds[]` делятся на **два класса**:

- **Прямые (direct):** self-contained body живёт целиком в state. Поле `ref` отсутствует (`omitempty` в JSON). Это юзерские outbounds, которые ни на что не ссылаются — full ownership, full Edit/Del, lifecycle вручную.
- **Ссылочные (referenced):** body живёт снаружи — в `template.parser_config.outbounds[]` или в `template.presets[].outbounds[]`. Поле `ref` одновременно (a) маркирует entry как ссылочный и (b) указывает источник тела.

Для ссылочных entries `ref` принимает одно из двух значений:
- `#TEMPLATE#` — body из `template.parser_config.outbounds[tag]`
- `<preset_id>` — body из `template.presets[id].outbounds` (mode=add)

USER edit поверх ссылочного entry хранится как **field-level diff** в `updates[]` с `ref="#USER#"` — это не отдельный класс entry, а патч поверх referenced.

```jsonc
"outbounds": [
  // (referenced, template) Template-derived без правок: только tag + ref.
  // Body на render/emit берётся из template.parser_config.outbounds[tag].
  { "tag": "auto-proxy-out", "ref": "#TEMPLATE#" },

  // (referenced, template) Template-derived с preset patches и USER patch.
  // Финальный body = template + apply updates[] в order.
  {
    "tag": "proxy-out", "ref": "#TEMPLATE#",
    "updates": [
      { "ref": "russian",   "patch": { "filters": { "tag": "!/(🇷🇺)/i" } } },
      { "ref": "ru-inside", "patch": { "filters": { "tag": "!/(🇷🇺)/i" } } },
      { "ref": "#USER#",    "patch": { "comment": "my custom" } }
    ]
  },

  // (referenced, preset) Preset add, body live из template.presets[russian].outbounds[mode=add].
  // Возможны USER patches поверх (юзер открыл Edit и поправил).
  {
    "tag": "ru VPN 🇷🇺", "ref": "russian",
    "updates": [
      { "ref": "#USER#", "patch": { "addOutbounds": [...] } }
    ]
  },

  // (direct) Self-contained body, не связан с template/preset.
  // Поле ref отсутствует — это и есть признак direct entry.
  { "tag": "myProxy", "type": "selector", "options": {...} }
]
```

### Sentinel constants

Всего две константы — больше ничего не вводим:

```go
// core/config/configtypes/types.go
const (
    RefTemplate = "#TEMPLATE#" // только в state.outbounds[].ref — ссылка на template body
    RefUser     = "#USER#"     // только в state.outbounds[].updates[].ref — user patch
)
```

Любое **другое** непустое значение `ref` интерпретируется как preset ID (lookup в `template.presets[id]`). Пустой `ref` (поле отсутствует) означает direct entry.

### Раскладка по позиции

| Позиция | Допустимые значения | Семантика |
|---|---|---|
| `state.outbounds[].ref` (entry-level) | `""` (поле отсутствует) | Direct entry — self-contained body |
| | `#TEMPLATE#` | Referenced → template body |
| | `<preset_id>` | Referenced → preset add body |
| `state.outbounds[].updates[].ref` (patch-level) | `<preset_id>` | Preset patch (активен пока preset enabled, иначе stale → drop) |
| | `#USER#` | User patch (один на outbound, всегда последний в order) |

### Validation

- **Template loader:** `preset.id` regex `^[a-z0-9][a-z0-9_-]*$` (lowercase + `-_`). Регекс по своей форме не даёт preset.id совпасть с константами `#TEMPLATE#`/`#USER#` (там `#` и UPPERCASE) — collision невозможна by construction.
- **State load — `state.outbounds[].ref`:** принимаем `""`, `#TEMPLATE#`, или valid preset.id. Reject `#USER#` (это patch-level sentinel, не entry-level) + всё что не matches preset.id regex и не равно одной из двух констант.
- **State load — `state.outbounds[].updates[].ref`:** принимаем `#USER#` или valid preset.id. Reject `""`, `#TEMPLATE#`, остальное.

### Поведение при разрыве ссылки

Ссылочный entry стал невалиден (source исчез):
- `ref=#TEMPLATE#` + tag нет в `template.parser_config.outbounds[]` → silent drop entry (USER patch тоже теряется — acceptable edge case)
- `ref=<preset_id>` + preset disabled или не существует → silent drop (lifecycle через preset toggle / Restore missing)

Direct entries (без `ref`) от template-evolution не зависят — они живут пока юзер их явно не удалит.

### Resolver semantics

`ResolveOutbounds(state, template)`:
```
for each ob in state.outbounds:
  base = lookup_base(ob.ref, ob.tag, template)
    case "":           ob.body как есть (direct entry — body inline в state)
    case "#TEMPLATE#": template.parser_config.outbounds[tag], drop entry if missing
    case preset_id:    template.presets[preset_id].outbounds find by tag (mode=add), drop if missing
  
  merged = base
  for each u in ob.updates (preset entries в rule order):
    if u.ref не active preset && u.ref != "#USER#": skip (stale)
    merged = apply_patch(merged, u.patch)
  
  # USER patch (updates[].ref == "#USER#") применяется всегда последним.
  if exists USER_patch in updates:
    merged = apply_patch(merged, USER_patch.patch)
  
  emit merged
```

### USER patch — diff от merged_base

На Edit Save:
1. `merged_base` = template body + apply all active preset patches (то что юзер видел при открытии Edit)
2. `form_value` = body после редактирования
3. `USER_patch = field_diff(form_value, merged_base)` — точечно по полям
4. Если diff пуст → не записываем USER entry (no-op Save)
5. Если USER entry уже была → **replace**, не append (всегда один USER patch на outbound)

**Field-level diff правила:**
| Поле | Diff поведение |
|---|---|
| `tag`, `type` | Сравнение строк; immutable для template/preset entries (изменение игнорируется) |
| `filters` (map) | Если equal → skip; иначе записываем целиком (replace) |
| `options` (map) | Per-key diff: пишем только изменённые ключи |
| `addOutbounds` (slice) | Slice equal (set-wise) → skip; иначе пишем целиком (replace, не union — иначе нельзя удалять) |
| `preferredDefault` (map) | Replace целиком |
| `wizard` (interface) | Replace целиком |
| `comment` (string) | Сравнение; пустая строка после edit + non-empty в base → пишем `""` явно для override |

### USER patch — всегда в конце updates[]

`apply_patch` order: preset patches в order их `rules[]` index, затем USER. Sync function гарантирует этот порядок при reorganization updates[].

### Template auto-upgrade — автоматический эффект

Если template обновляется (новый `addOutbounds`, изменился `comment`, новое поле в `options`):
- `ref="#TEMPLATE#"` без USER patch → юзер получает новые defaults автоматически
- `ref="#TEMPLATE#"` + USER patch только на `filters` → новые `addOutbounds`/`comment` приходят, юзерский `filters` сохраняется
- `ref="<preset_id>"` — то же самое от preset definition

Это и есть **основной выигрыш** от SPEC 058.

## Schema changes

`core/config/configtypes/types.go::OutboundConfig` — поля те же что в SPEC 057, изменяется только семантика `Ref`:

```go
type OutboundConfig struct {
    Tag     string                 `json:"tag"`
    Type    string                 `json:"type,omitempty"`     // omitempty — referenced entries body не пишут
    Options map[string]interface{} `json:"options,omitempty"`  // только в direct entries / USER patch
    Filters map[string]interface{} `json:"filters,omitempty"`
    // ... остальные body-поля — omitempty (отсутствуют у referenced entries)

    // Ref маркирует entry как referenced и указывает источник body.
    // ""           — direct entry (поле отсутствует в JSON, body inline)
    // "#TEMPLATE#" — referenced на template.parser_config.outbounds[tag]
    // "<preset_id>"— referenced на template.presets[id].outbounds (mode=add)
    Ref     string           `json:"ref,omitempty"`
    Updates []OutboundUpdate `json:"updates,omitempty"` // стек patches
}
```

**Backward compat:** `Ref=""` всегда валиден — означает direct entry. Legacy state SPEC 057 хранит template-derived entries с пустым `Ref` и snapshot'нутым body — migration переведёт их в referenced shape (см. ниже): если `tag` совпадает с template'овским — `Ref="#TEMPLATE#"` + diff в USER patch; иначе оставляем direct.

Validation см. выше в «Sentinel constants» (Template loader regex + state load позиционные правила).

## Migration legacy state

Существующие state.json (SPEC 057 shape) хранят template-derived entries с пустым `ref` и snapshot'нутым body — их надо перевести в referenced shape. Migration однопроходная на первом load:

```
for each ob in state.outbounds with ref="":
  template_match = template.parser_config.outbounds.find_by_tag(ob.tag)
  if template_match != nil:
    # tag найден в template — переводим из direct в referenced.
    diff = field_diff(ob.body, template_match.body)
    ob.ref = "#TEMPLATE#"
    if diff non-empty:
      append USER_patch (или replace existing USER patch) с diff
    strip body fields (остаётся только tag + ref + updates)
  else:
    # tag не в template — это настоящий direct entry, оставляем как есть.
    keep ob.ref = "", keep body as-is
```

Referenced preset entries (`ref=<preset_id>`) — уже в правильной форме, migration их не трогает.

**Идемпотентно:** повторный load не меняет state (всё уже в новом shape).

**Backup:** на первом save после migration пишем `state.json.pre-058.bak` (по аналогии с SPEC 053's `.v5.bak`). Lossless rollback гарантирован.

## Sync semantics — обновлённый

`SyncOutboundsWithActivePresets`:
1. Walk state.outbounds, для каждого:
   - Drop referenced preset entries (`ref=<preset_id>`) с disabled/missing preset
   - Drop referenced template entries (`ref=#TEMPLATE#`) если tag отсутствует в template (silently, USER patch на dropped tag тоже теряется)
   - Direct entries (`ref=""`) не трогаем (они не зависят от template)
   - Из `updates[]` drop preset patches от disabled preset; USER patch оставляем
2. Add missing referenced preset add entries (как сейчас в SPEC 057)
3. Add missing referenced template entries: для каждого `template.parser_config.outbounds[]` не представленного в state — append `{tag, ref: "#TEMPLATE#"}`. Используется для первого «засеивания» state из template; в обычном flow юзер сам нажимает «Restore missing».
4. Add expected preset update patches (mode=update entries) — как сейчас
5. Re-order updates[]: preset entries в rule order, USER в конце
6. **Adopt legacy:** existing direct entry + tag совпадает с preset add → конвертируем в referenced (`ref=preset_id`, strip body) (как сейчас в SPEC 057)

## Build pipeline

`MergeOutboundUpdatesInPlace(parserCfg)` обновляется:
- Для referenced entries (`#TEMPLATE#` / preset_id) резолвит base через template lookup
- Для direct entries (`ref=""`) base = body как есть в state
- Применяет updates[] стек (preset patches + USER) в order
- Финальный body — в parserCfg.Outbounds для GenerateOutboundsFromParserConfig

Generator (`GenerateOutboundsFromParserConfig`) не меняется — получает уже merged body.

## UI changes

**Outbounds tab → collectRows:**
- Referenced template (`ref=#TEMPLATE#`) без USER patch → "Global" label, no badge
- Referenced template с USER patch → "Global ✏" badge + tooltip "modified from template defaults"
- Referenced preset (`ref=<preset_id>`) → 🔒 + preset label
- Referenced preset с USER patch → 🔒 ✏ + preset label + tooltip
- Direct (`ref=""`) → "Global" no badge, full edit

**Edit dialog:**
- Для referenced entries открывается с `merged_base` view (template/preset body + active preset patches). На Save вычисляет diff, обновляет USER patch в updates[].
- Для direct entries открывается с inline body, на Save body перезаписывается напрямую (никакого diff, никакого USER patch — direct ему не нужен).

**Кнопки в row:**
- Referenced template (`ref=#TEMPLATE#`): Edit, Reset (clear USER patch, disabled если USER patch отсутствует), Del (удаляет entry из state — восстанавливается через «Restore missing»)
- Referenced preset (`ref=<preset_id>`): View (по умолчанию), Edit (опционально, кладёт USER patch); Del нет (lifecycle через preset toggle)
- Direct (`ref=""`): Edit, Del (full ownership)

**Required outbound special case:**
- `template.parser_config.outbounds[tag].required=true` → state хранит `{tag, ref: "#TEMPLATE#"}`. Del **полностью disabled** (не просто removable→restore, а button hidden). Edit + Reset работают.

**«Restore missing»:**
- Walk template.parser_config.outbounds, для каждого tag не в state → append `{tag, ref: "#TEMPLATE#"}`. Существующие entries не трогаем (юзерские правки сохраняются).

## UI render data flow

```
state.connections.outbounds[]  ─── direct (body inline) + referenced (ref+updates only)
       +
template.parser_config.outbounds[]  ──┐
template.presets[].outbounds[]      ──┤  ResolveOutbounds()  — резолвит referenced
                                       ├──→  []ResolvedOutbound
state.RulesV6 (active presets)      ──┘     {merged Body, Ref, IsDirect, IsTemplate, IsPreset, HasUserPatch}
                                                │
                                       ┌────────┴────────┐
                                       ▼                 ▼
                                   UI render        Build emit (после MergeOutboundUpdatesInPlace)
                                   row badges       config.json::outbounds[]
```

## Acceptance

1. Referenced template entries хранятся как `{tag, ref: "#TEMPLATE#"}` без duplicated body. Referenced preset entries — `{tag, ref: "<preset_id>"}`. Direct entries — без поля `ref`, body inline.
2. Template auto-upgrade: изменение `addOutbounds` / `comment` в `template.parser_config.outbounds[tag]` после релиза → юзер получает изменения автоматически на следующем render/build для referenced template entries (без manual Reset).
3. USER edit на referenced entry → diff против merged_base → USER patch в updates[] с минимальным содержанием.
4. No-op Save на referenced (юзер открыл Edit и закрыл без правок) → USER entry не создаётся / удаляется (если diff пуст).
5. Один USER patch на referenced outbound (не стек) — replace при каждом Save.
6. USER patch применяется **последним** в updates[] order независимо от positional insertion.
7. Direct entries (`ref=""`) — self-contained body, не трогаются template-evolution, не имеют USER patch (правки идут напрямую в body).
8. Referenced template entry с tag, отсутствующим в template → silently drop (с USER patch включительно).
9. Referenced preset entry с `ref=disabled/missing preset` → drop.
10. Migration legacy state (SPEC 057 shape с пустым `ref` и snapshot body) → идемпотентна, lossless backup.
11. Build emit byte-identical до и после migration для unchanged state.

## План фаз

1. **Phase 1 — Schema validation + sentinel constants:** добавить две константы в `configtypes`:
   - `RefTemplate = "#TEMPLATE#"` — только в `state.outbounds[].ref` (referenced template)
   - `RefUser = "#USER#"` — только в `state.outbounds[].updates[].ref` (user patch)
   - Direct entries — пустой `ref` (omitempty), без константы (отсутствие поля = семантика).
   - Template loader: оставляем существующий `preset.id` regex `^[a-z0-9][a-z0-9_-]*$` — он по форме не пересекается с двумя константами (UPPERCASE + `#`). Менять regex не нужно.
   - State loader валидирует допустимость значений `ref` по позиции (entry vs updates).

2. **Phase 2 — Resolver expansion:** `ResolveOutbounds` learns to lookup base by `Ref`: `#TEMPLATE#` → `template.parser_config.outbounds[tag]`, `<preset_id>` → `template.presets[ref].outbounds`, `""` (empty) → direct entry, body inline. Update `mergeOutboundUpdates` / `MergeOutboundUpdatesInPlace` accordingly. Tests на каждый source (direct + 2 referenced).

3. **Phase 3 — Sync function update:** lifecycle для template entries (drop on missing tag, add missing on demand). Reorder updates[]: preset в rule order, USER в конце.

4. **Phase 4 — Edit dialog diff logic:** на Save вычисляем `field_diff(form_value, merged_base)`; replace USER entry в updates[]; no-op skip. Helper `OutboundFieldDiff(a, b configtypes.OutboundConfig) map[string]interface{}`.

5. **Phase 5 — Migration:** одноразовая на load — empty `Ref` + tag в template → конвертируем в referenced (`Ref="#TEMPLATE#"` + diff в USER patch + strip body); иначе оставляем direct (keep empty `Ref` + body). Backup `.pre-058.bak` на первом save.

6. **Phase 6 — UI:** `collectRows` различает 3 source-типа + has_user_patch badge; Edit / Reset / Del button visibility per source. «Restore missing» обновляется для нового layout (просто tag+ref).

7. **Phase 7 — Cleanup:** удалить `presetOutboundByRefTag` helper если становится redundant с resolver expansion; удалить `templateOutboundByTag` (заменяется resolver lookup).

8. **Phase 8 — Tests + build + reinstall.**

## Риск

**Средний.** Migration касается всех user'ских state.json — но идемпотентна и lossless благодаря backup'у. Тесты на:
- byte-identical build emit до/после migration на reference state
- round-trip: edit → save → load → edit unchanged → save → diff пуст
- template auto-upgrade сценарий: предzаписать stale body в state, поменять template, load → state мигрирует, build emit включает новые template fields
- conflict: preset patch + USER patch на одно поле → USER побеждает (последний в order)

**Mitigation:**
- Backup `.pre-058.bak` создаётся на первом save после migration — lossless rollback по необходимости (юзер копирует `.bak` → `state.json`, ставит предыдущий build, всё работает).
- Migration идемпотентна — повторный запуск нового build на уже мигрированном state не делает ничего.
- Acceptance criterion 11 (byte-identical build emit для unchanged state) — golden test на reference state ловит регрессию до релиза.
- Ручной dogfooding на maintainer state.json перед merge в main.

Никакого feature flag не вводим — это лишняя ветвь кода + 2x тест-матрица. Backup + golden тесты — достаточная safety net.

## Не в скоупе

- Per-source outbound'ы (`parser_config.proxies[i].outbounds[]`) — генерятся из subscription parsing, не template-derived. Schema не меняется.
- DNS секция — уже в правильной модели (SPEC 056); SPEC 058 только outbounds.
- Route rules — отдельная история (SPEC 053 preset bundles), будет аналогичная унификация в SPEC 059 если решим.
- Migration v5 → v6 schema bump — не нужен, SPEC 058 in-place dev rewrite в том же v6/`presets_v1`.
