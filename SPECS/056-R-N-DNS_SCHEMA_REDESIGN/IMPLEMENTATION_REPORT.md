# SPEC 056-R-N — Implementation Report (FINAL)

**Status:** shipped (uncommitted, на review)
**Дата:** 2026-05-23

Этот SPEC начинался как чистый refactor DNS-схемы (template_servers map →
flat kind-discriminator), но в процессе вырос в **архитектурную унификацию**
DNS + Route emit pipeline через единый resolver-pattern и UI parity с build
pipeline. Финальный объём шире изначального scope'а; см. Phase F — F отдельно
выделены post-original phases.

---

## Что в результате есть

### 1. Один resolver на каждую секцию

```
state.DNS + template + vars
        │
        ▼
   ResolveDNS()  ◄── единая pure func
        │
   ┌────┴────┐
   ▼         ▼
[UI render] [build emit]
```

Аналогично `ResolveRoute()` для route section.

**Принцип:** memory == disk == emit. То что UI показывает — то и эмитится в
config (модулировано `Active && Enabled`).

### 2. Flat schema state.dns_options.servers/rules через kind discriminator

```json
"dns_options": {
  "servers": [
    {"kind":"template", "tag":"cloudflare_udp", "enabled":true},
    {"kind":"preset",   "ref":"russian:yandex_doh", "enabled":true},
    {"kind":"user",     "tag":"my-pihole", "type":"udp", "server":"...", "enabled":true}
  ],
  "rules": [
    {"kind":"preset", "ref":"russian", "enabled":true},
    {"kind":"user",   "rule_set":"ru-domains", "server":"yandex_doh", "enabled":true}
  ]
}
```

- `kind=template` — toggle через chекbox, body в `template.dns_options.servers[]`
- `kind=preset` — auto-add/remove на toggle preset rule в Rules tab; per-server toggle хранится в `PresetRefState.DNSServerEnabled`
- `kind=user` — full edit/delete, full body в state

### 3. Template DNS unified в одном месте

`template.config.dns.servers[]` теперь **пустой массив** `[]`. Все DNS-серверы
(включая обязательные `local_dns_resolver` / `direct_dns_resolver`) живут в
**одном** `template.dns_options.servers[]` с `required: true` маркером для
mandatory entries.

| `required` в template | UI |
|---|---|
| `true` | locked checkbox (always on), View JSON, без edit/del |
| `false` / отсутствует | toggle works, View JSON |

### 4. Required outbounds (proxy-out и т.п.)

`OutboundConfig` не имеет `Required` field — флаг живёт **только в template**.
UI читает live через `templateRequiredTags(model)` на каждый render. State.json
не персистит `required` (иначе нельзя было бы снять флаг в template'е и увидеть
эффект).

UI для required outbound:
- ✅ Up / Down — reorder работает как у любого global
- ✅ Edit — параметры можно править
- 🔄 **Reset** — откатывает body к template defaults (новая кнопка)
- ❌ Del не рендерится — нельзя удалить template-mandated entry

### 5. Preset bundled outbounds в Outbounds tab

`preset.outbounds[]` (SPEC 055) при enabled preset rule показываются в общем
списке Outbounds tab read-only:
- `<tag> (<type>) — 🔒 <preset_label>`
- View JSON для inspect (с helper text про promote semantic), без edit/del
- Dedup vs existing tags (если global уже определяет тот же tag — first-wins,
  preset row не показывается; паритет с `ApplyPresetOutboundsToParserConfig`)

**Preset reorder с сохранением preset binding (E13):**

Реализован через `model.OutboundDisplayOrder []string` (in-memory, `json:"-"`):

```
state + template
        │
        ▼
collectRowsForUI →  [per-source, globals, preset rows]
                        │
                        ▼ reorderByDisplayOrder(model, globals, presetRows)
                        │       (применяет OutboundDisplayOrder ко второй части)
                        ▼
              unified list для UI
                        │
                        ▼
                Up/Down → reorderRowDisplay
                          • global↔global   → swap в pc.ParserConfig.Outbounds[]
                          • preset↔* или
                            global↔preset    → swap в OutboundDisplayOrder list
```

`rowKey` стабильно идентифицирует entry: `"g:<tag>"` для global, `"p:<tag>"`
для preset. Preset row сохраняет `IsPreset=true` после reorder — View JSON
кнопка остаётся, Edit/Del не возвращаются. То есть «row помнит что он из
preset» как user попросил.

⚠ **Что НЕ дотянули в SPEC 056 — перенесено в SPEC 057-R-N:**
- `OutboundDisplayOrder` имеет `json:"-"` → reorder теряется на рестарт app
- `ApplyPresetOutboundsToParserConfig` в build pipeline всё ещё append'ит
  preset entries в конец `outbounds[]` — config.json по-прежнему имеет
  preset entries последними, независимо от UI порядка

**Архитектурный fix живёт в SPEC 057-R-N-OUTBOUNDS_PRESET_BINDING.** Там
вместо UI-only OutboundDisplayOrder заходят прямые fields на самом
`OutboundConfig`:
- `Ref string` — id preset'а владельца для preset-add entries
- `Updates []OutboundUpdate` — стек patches от mode=update пресетов

Lifecycle через новый `SyncOutboundsWithActivePresets` (зеркало
SyncDNSOptionsWithActivePresets) — preset binding живёт в state.json,
persistent, build pipeline учитывает порядок entries в `pc.ParserConfig.
Outbounds[]` напрямую (preset rows перестают быть синтетическими).

⚠ **Историческая заметка (Phase E8/E13):** прошли через 3 итерации —
- E8: promote-to-global (preset row копировался → терял binding) → отбита
- E13: OutboundDisplayOrder UI-only (binding сохранён, но не персистится
  и не учитывается в emit) → текущее состояние SPEC 056
- SPEC 057: проперный binding через state field — финальная архитектура

### 6. Disabled subscription cascade

Когда юзер выключает чекбокс подписки в Sources tab:
- ✅ Preview cache (`RebuildPreviewCache`) — пропускает disabled
- ✅ Outbounds tab `collectRows` — пропускает per-source outbounds (BL:auto, BL:select)
- ✅ Outbounds tab `collectAllTags` — disabled-source tags не в dropdown для addOutbounds
- ✅ Build pipeline `GenerateOutboundsFromParserConfig` — уже пропускал (это reference)

Все 4 точки теперь паритетны.

### 7. UI parity по widget patterns

DNS Servers + DNS Rules секция используют общий widget pattern
`NewCheckWithContent` + `HoverRow` + `WireTooltipLabelHover` для всех 3 kind'ов
(template/preset/user). Hover row clickable, tooltip wired через label,
visual consistency.

---

## Что было удалено

### Legacy hacks

- `legacyDNSOptionsFromV6` — больше нет двух DNS views (state.DNSV6 = single
  source; UI читает напрямую)
- `inline v5/v6 guard` в `dnsConfigForUpdate` — упрощено через прямой check
  `len(s.RulesV6) > 0 || len(s.DNS.Servers/Rules) > 0`
- `coreDNSServersFromTemplate` + CORE step в ResolveDNS — единый walk по
  `template.dns_options.servers[]` (включая required)
- `emitTemplateDNSDefaults(defaults, overrides)` — каждое `kind=template`
  entry знает свой Enabled напрямую (no override map)
- Consumption-filter в `ExpandPreset` для DNS — все bundled серверы попадают,
  фильтрация по Enabled

### Dead code

- `WizardConfig.Required int` + `GetWizardRequired()` method — нигде не
  вызывался, был задуман для config-health validator но не доведён
- `model.DNSLockedTags map[string]struct{}` + `DNSTagLocked` map-based lookup —
  заменено live lookup'ом из template `required` field
- `mergeLockedRow` (wizard_dns.go) — tag-collision между config.dns.servers
  и dns_options.servers больше невозможен (config.dns.servers пуст)
- `model.DNSPresetServerEnabled` + `DNSPresetRuleEnabled` карты — перенесены
  в `PresetRefState` (естественный per-instance scope)
- `OutboundConfig.Required bool` field (изначально добавлен → потом убран,
  template = единый источник истины)
- `maybeBackupDevV6` / `looksLikeLegacyDevV6` — automatic backup на load,
  выкинут как YAGNI (lossless conversion, v6 не релизился, dev-only state)

### Удалённые UI элементы

- DNS section header "from active presets" — bundled rows в общем списке
- Edit/Del кнопки на template DNS entries — заменены View
- Edit/Del кнопки на preset DNS entries — заменены View
- Edit/Del кнопки на preset Outbound entries — заменены View
- Del кнопка на required Outbound entries — не рендерится (заменена Reset)
- Disabled subscription per-source outbounds (BL:auto и т.д.) — больше не
  показываются

---

## Acceptance check

| # | Item | Статус |
|---|---|---|
| 1 | `state.json` Save в новом shape (flat `dns_options.servers/rules` через kind) | ✅ |
| 2 | `state.vars[]` неизменно — `dns_*` scalars остаются | ✅ |
| 3 | Удалены legacy DNS hacks (см. список выше) | ✅ |
| 4 | Один emit-путь через `ResolveDNS()` | ✅ |
| 5 | UI render kind-aware (template/preset/user разные controls + tooltips) | ✅ |
| 6 | In-place dev rewrite старого dns shape → новый (без backup, lossless) | ✅ |
| 7 | `docs/WIZARD_STATE.md` обновлён | ✅ |
| 8 | Unified resolver (Phase B) — DNS + Route, single pattern | ✅ |
| 9 | Template DNS unify (Phase C) — `required: true` маркер | ✅ |
| 10 | Outbounds tab parity (Phase E) — preset rows, disabled subs cascade, required Reset | ✅ |
| 11 | UI widget pattern consistency (Phase D) | ✅ |
| 12 | Dead code cleanup (Phase F) | ✅ |

---

## Что не сделано (backlog)

### Перенесено в SPEC 057-R-N-OUTBOUNDS_PRESET_BINDING

Архитектурный fix preset reorder + preset update-stack — отдельный SPEC.
SPEC 057 заменяет UI-only `OutboundDisplayOrder` на прямой preset binding в
state.json (`OutboundConfig.Ref` + `Updates[]`). Cм. `SPECS/057-R-N-OUTBOUNDS_PRESET_BINDING/SPEC.md`.

- Preset reorder с binding preservation (persist) → SPEC 057
- Build pipeline учёт порядка preset entries → SPEC 057
- Preset DNS reorder → отдельная задача (тот же паттерн адаптировать через ResolveDNS)

### Остаётся в backlog SPEC 056

- **Golden test для byte-identical config.json до/после рефактора.** Golden
  test файлы (`core/build/golden_test.go`) запускаются под флагом
  `GOLDEN_RUN_REAL=1`; реального CI-сравнения не было. Тестировалось
  ручным запуском sing-box после каждой фазы.
- **Полная UX-валидация Reset кнопки** на required outbound при edge cases
  (template без tag'а после bump'а, broken template и т.п.). Базовый flow
  работает; защитные ветки в коде есть (silent no-op).
- **Strict preset Body validator** при load state — отбрасывать non-Vars
  поля если template author случайно положил «жирный» body в `preset.body`.
  Сейчас просто игнорируется на decode; warning не выводится.
- **Helper для skip-disabled** при walk `pc.ParserConfig.Proxies[]` —
  сейчас 3 разных места независимо проверяют `proxy.Disabled` (рефакторнуть
  в общий iterator или хотя бы lint-check).

---

## Объём изменений

```
34 файла:       +2346 строк   −1287 строк    net +1059 LOC
```

Большие удаления (legacy hacks + dead code) компенсируются:
- Двумя новыми resolver'ами (`resolve_dns.go` + `resolve_route.go`) — ~700 LOC
  с тестами
- `Required` mechanism + `PresetOutboundAddTags` + `templateRequiredTags`
- Unified UI patterns в DNS preset bundled + Outbounds tab

---

## Что в git status

Все изменения **uncommitted** в working tree:

```
M  бин 22 файла (core, ui)
A  4 новых файла (resolve_dns.go, resolve_dns_test.go, resolve_route.go,
                  preset_ref_state.go доп. поля)
M  bin/wizard_template.json (DNS unify + required: true на proxy-out)
M  docs/WIZARD_STATE.md
M  SPECS/056-R-N-DNS_SCHEMA_REDESIGN/*.md
```

Тесты: `go test ./...` — все 24 пакета зелёные.

Билд + установка: `/Applications/singbox-launcher.app/Contents/MacOS/singbox-launcher`
(May 23, 22:35 build).

---

## Lessons / Notes

1. **Resolver pattern как single source of truth работает.** Когда UI и build
   расходятся (preview vs final config), а в логике несколько фильтров (если/
   substitute/consumption/enabled), резолвер с metadata (`Active`/`Enabled`/
   `Locked`/`InactiveReason`) даёт явную точку истины и одну точку для
   debug.

2. **Template = source of truth для template-author flags** (`required`).
   Персистить такие флаги в state.json — bait для stale data. Live lookup на
   render — копейки времени, корректность гарантирована.

3. **Dedup при unified emit важен.** Build pipeline first-wins для preset
   collision; UI должен зеркалить это иначе будут визуальные дубли. То же
   для disabled subscription cascade — каждое место где UI читает
   parser_config должно проверять `Disabled`.

4. **Cleanup'ы дают больше LOC reduction чем кажется.** `mergeLockedRow`,
   `legacyDNSOptionsFromV6`, model DNS maps — каждый кусок был оправдан в
   моменте, но накопление дублирующейся семантики делает кодовую базу
   trickier. Unified resolver + scope-bound state (PresetRefState вместо
   глобальных карт) — это и упрощение, и invariant enforcement.

5. **Backup-механика для dev-only schema rewrite — over-engineering.** Когда
   conversion lossless и формат не релизился, automatic backup только
   усложняет код (silent failures, не atomic write, dead-после-первого-save).
   Удалить и оставить ручной `cp` — честнее.

---

## Pitfalls встреченные в процессе

Несколько багов из-под-капота — задокументированы как warning для будущего.

### P1. `OR` semantic vs «single source of truth»

**Симптом:** добавил `OutboundConfig.Required bool` field; при IsRequired flag
делал `IsRequired = ob.Required || templateRequiredTags[tag]`. Работает в
момент. Но если template author СНИМЕТ `required: true` в v2, юзер с уже
сохранённым state.json продолжит видеть row как required (state имеет stale
true).

**Правильно:** template = единый источник. IsRequired = ТОЛЬКО
templateRequiredTags[tag]. State не персистит template-author flags. Field
вообще удалён из `OutboundConfig` чтобы он физически не попал в state.json.

**Урок:** для любого template-author concern (типа `required`/`hide`/locked) —
read-only live из template на каждый render. Никогда не персистить.

### P2. Wrapped JSON «capital P» в TemplateData.ParserConfig

**Симптом:** `templateRequiredTags` молча возвращал пустой set. Reset кнопка
не появлялась.

**Корень:** `core/template/loader.go:207` оборачивает template's
parser_config в `{"ParserConfig": {...}}` (capital P, legacy from old format).
А я парсил с json tag `"parser_config"` (snake_case — sing-box convention).
Field тихо терялся, no error.

**Урок:** wrapped raw JSON — pitfall. JSON-tag должен **в точности** совпадать
с тем что писал loader. Если меняешь struct → проверь callsite парсера.

### P3. Consumption-filter в ExpandPreset выглядел оптимизацией, оказался багом

**Симптом:** UI показывал 1 yandex_* в DNS-серверах вместо 3.

**Корень:** `collectConsumedBundledDNSTags` фильтровал bundled DNS-сервера
preset'а — оставлял только тех что упомянуты через `@dns_server` var ИЛИ
литерал в `dns_rule.server`. Идея: «зачем эмитить server которого никто не
использует». Но новый SPEC 056-R-N даёт юзеру per-server enable toggle —
филтр противоречит этой семантике.

**Урок:** «оптимизация» которая теряет данные ради экономии нескольких
строк config.json — почти всегда баг в долгую. sing-box игнорирует unused
servers — config bloat in 4 entries vs 1 не стоит UI mismatch.

### P4. UI render и build pipeline должны проверять `Disabled` симметрично

**Симптом:** юзер выключил подписку, ноды и outbound'ы продолжали
отображаться в Outbounds tab.

**Корень:** build pipeline (`GenerateOutboundsFromParserConfig`) пропускал
disabled с самого начала, но UI walked all proxies (без disabled check) в
3 разных местах: `RebuildPreviewCache`, `collectRows`, `collectAllTags`.

**Урок:** любой код который читает `pc.ParserConfig.Proxies[]` должен
делать `if proxy.Disabled { continue }`. Лучше сразу wrap'ить через helper
который пропускает (не было — потенциальный future refactor).

### P5. Dedup при unified emit (preset row показывался дважды)

**Симптом:** preset add `ru VPN 🇷🇺` и global уже-сохранённый `ru VPN 🇷🇺` оба
отображались в UI.

**Корень:** `collectRows` walked globals; `collectPresetOutboundRows`
walked preset adds **без collision check** против globals. Build pipeline
делал first-wins (preset add skip'ался), но UI этого не знал.

**Урок:** при unified эмите из разных источников — collision policy должна
быть зеркальной между UI и build. Дедуп через `existingTags` set'ы.
