# SPEC 055-F-S — PRESET_OUTBOUNDS (включая ex-SPEC 056 OUTBOUNDS_PARSER_RESTORE)

**Status:** Shipped (S)
**Type:** Feature (F) — feature semantics + implementation + post-ship DNS schema cleanup
**Depends on:** SPEC 053 (preset bundles), SPEC 052 (connections redesign).
**Bump:** Не требуется (state.json остаётся v6 — `presets_v1` schema без изменений; шаблонная семантика расширяется).

---

## Структура SPEC'а

Этот SPEC объединяет три исторически разделённых документа в одно место:

| Бывшая папка | Что было | Куда переехало |
|---|---|---|
| **055** (этот) | Feature spec (что должен делать preset.outbounds) | Раздел «Feature semantics» ниже |
| **ex-056** OUTBOUNDS_PARSER_RESTORE | Implementation rewrite после провала первой попытки 055 | `IMPLEMENTATION_PLAN.md` + `IMPLEMENTATION_REPORT.md` в этой же папке |
| **ex-057** STATE_REFS_ONLY (никогда не существовал как папка) | Post-ship DNS schema cleanup того же класса | Раздел «Post-ship: DNS schema cleanup» ниже + соответствующие фазы в plan/report |

Принцип консолидации: **одна фича, один SPEC**. Первая попытка реализации (исходный 055-план — post-merge JSON-patch) провалилась → второй заход (ex-056 — pre-patch parser_config) сработал → выявил родственные DNS-баги → почистили схему. Всё про preset.outbounds — здесь.

---

## Инвариант (объединённый, post-cleanup)

```
В state.json лежат ТОЛЬКО:
  • thin refs на template entities (preset_id, server_tag, rule_set_tag)
    + diff/override (vars, enabled)
  • полные тела ТОЛЬКО для того, чего нет в template
    (genuinely user-add: kind=inline/srs route rules; DNS extra_servers/
    extra_rules для user-added DNS — НЕ template-tag копии)

НИКОГДА не копировать template body в state.
Копия = дырка во времени: template меняется → копия отстаёт → dangling/конфликт.
```

Все DNS-server emit-пути проходят через **единую** `stripDNSWizardOnlyFields`
(single source of truth для cleanup: `description/enabled/title/if/if_or/_*`).
Все outbound-emit пути проходят через **единый** native generator
(`GenerateOutboundsFromParserConfig`) — preset.outbounds типизированно
pre-patch'ат `parserCfg.ParserConfig.Outbounds[]` ДО его запуска.

---

## Цель

Расширить SPEC 053 `presets[]` четвёртой self-contained секцией — `outbounds`. Preset получает возможность:

1. **`mode: "add"`** (дефолт) — объявить новый outbound (типично `selector`/`urltest`), который появляется в `config.json` **только когда preset enabled**.
2. **`mode: "update"`** — точечно патчить существующий outbound (из `parser_config.outbounds[]` или ранее-эмитнутый preset'ом) — например добавить `filters` или дополнить `addOutbounds`.

Use case: убрать с глобального `proxy-out` фильтр `!/(🇷🇺)/i` (он создаёт лишнюю sticky-логику для юзеров без ru-flow), а нужный фильтр + отдельный `ru VPN 🇷🇺` селектор привезти **внутрь** `ru-inside` preset'а. Включил preset → появилась RU-секция и proxy-out исключил RU-прокси; выключил → всё чисто.

Принцип self-contained presets из SPEC 053 раскрывается до конца: preset несёт `vars`/`rule_set`/`rule`/`dns_servers`/`dns_rule` **и** теперь `outbounds`.

## Не-цели

- **Не** меняем `parser_config.outbounds[]` структуру — поля и семантика как сейчас (`tag`, `type`, `options`, `filters`, `addOutbounds`, `wizard.required`).
- **Не** меняем sing-box `config.json` — preset outbounds эмитятся в той же `outbounds[]` array.
- **Не** делаем UI-редактор preset.outbounds — содержимое пресета редактируется в `wizard_template.json`. UI только видит результирующий список outbound тэгов для outbound-picker'ов.
- **Не** делаем cross-preset references на уровне outbound (`addOutbounds` другого preset'а) — пока. Только globals + outbounds того же preset'а.
- **Не** меняем `mode` семантику в params (уже есть `replace`/`prepend`/`append` — это independent space; preset.outbounds.mode переиспользует только название поля, не values).

---

## Финальный layout preset.outbounds

```jsonc
{
  "id": "ru-inside",
  "label": "Russia-only services",
  "description": "...",
  "vars": [
    { "name": "out", "type": "outbound", "default": "ru VPN 🇷🇺", "title": "Outbound" }
  ],
  "rule_set": [
    { "tag": "ru-inside", "type": "remote", "format": "binary",
      "url": "https://..." }
  ],
  "outbounds": [
    // mode: "update" — патч существующего proxy-out
    {
      "mode": "update",
      "tag": "proxy-out",
      "filters": { "tag": "!/(🇷🇺)/i" }
    },
    // mode: "add" (default) — новый selector с RU-only фильтром
    {
      "tag": "ru VPN 🇷🇺",
      "type": "selector",
      "options": { "default": "direct-out", "interrupt_exist_connections": true },
      "filters": { "tag": "/(🇷🇺)/i" },
      "addOutbounds": ["direct-out"]
    }
  ],
  "rule": { "rule_set": "ru-inside", "outbound": "@out" }
}
```

---

## Mode семантика

| `mode` | Значение | Семантика |
|---|---|---|
| **`"add"`** | дефолт когда поле не указано | Создать новый outbound с этим `tag`. Tag-collision (с globals или earlier preset) → **первый wins + warning**. Identical-body → silent skip. |
| **`"update"`** | explicit | Найти outbound с этим `tag` в `parser_config.outbounds[] + earlier preset emits`; пропатчить указанные поля. Target отсутствует → **skip + warning**, не auto-create. |

Других значений нет. Любое неизвестное `mode` → skip + warning «unknown mode».

### Field-level merge rules для `mode: "update"`

| Поле | Семантика merge | Почему |
|---|---|---|
| `filters` | **Replace** (всё или ничего) | Деep-merge regex'ов опасен (две конкурирующие политики дают непредсказуемый результат); replace явный |
| `addOutbounds` | **Union** (append unique entries) | Естественная аддитивность — preset расширяет доступные опции, не теряет существующие |
| `options.default` | Replace per-field | Скаляр, однозначно |
| `options.interrupt_exist_connections` | Replace per-field | Скаляр |
| `options.*` (любые другие) | Replace per-field только тех что заданы | Не трогаем поля которые update не упомянул |
| `wizard` block (`wizard.required`) | Replace per-field | Низкоуровневые launcher-флаги |
| `type` | **Запрещено менять** | Меняет фундаментальную семантику outbound'а; drop этот field + warning |
| `tag` | **Запрещено менять** | Это идентификатор для targeting'а |
| `comment` | Replace | Юзерский комментарий |

### Edge cases

| Случай | Поведение |
|---|---|
| `mode: "update"` на несуществующий tag | Skip + warning `preset X: update target 'tag' not found in current outbounds list` |
| Два preset'а делают `update` одного tag | Применяются по `model.RuleOrder` (порядок в Rules tab); каждый layer'ом поверх предыдущего |
| `mode: "update"` пытается заменить `type` | Drop поле `type` из patch'а + warning `preset X: cannot change 'type' via update; field dropped` |
| `mode: "update"` + ещё один `mode: "add"` для одного `tag` в **одном** preset | Warning, оставить только `add` (более «полное» определение) |
| Два preset'а `mode: "add"` на одинаковый `tag` | Первый (по RuleOrder) wins + warning `preset Y: tag T already added by preset X; this add ignored`. Identical body — silent skip без warning. |
| `mode: "add"` на tag совпадающий с globals | Первый (globals) wins + warning. Юзер должен использовать `mode: "update"` явно если хочет менять. |
| Preset disabled → его outbounds (add **и** update-patches) **не** применяются | Identical поведение что для `rule_set`/`dns_servers`/`dns_rule` |
| Dangling reference: user rule использует `outbound: "ru VPN 🇷🇺"`, потом disable `ru-inside` | Build pipeline cleanup: rule'у с unknown outbound → fallback на `route.final` + warning. Аналогично `cleanDanglingRuleSetInRule`, только для `outbound:` поля. |

---

## Build pipeline integration

Заходим в существующий pipeline `core/build/build.go`:

```
parser_config.outbounds[]
   ↓
[1] Старый builder: resolve filters против snapshot.Proxies, эмит base config.outbounds[]
   ↓
[2] NEW: MergePresetsIntoOutbounds — обходит active preset-refs (по RuleOrder), применяет outbounds[]
   ↓
config.outbounds[] (финальный)
   ↓
[3] Существующий: route.rules → cleanDanglingOutboundRefInRule (новая функция, аналог cleanDanglingRuleSetInRule)
   ↓
config.route.rules[] (финальный)
```

### Stage [2] алгоритм

```
emitted = clone(base_outbounds)  // tag → outbound
emittedOrder = list of tags in original order

for each active preset-ref (in RuleOrder):
    frags = ExpandPreset(template_preset, vars)  // includes frags.Outbounds
    for each ob in frags.Outbounds:
        mode = ob.mode or "add"
        switch mode:
            case "add":
                if emitted[ob.tag] exists:
                    if identical(ob, emitted[ob.tag]):
                        skip silently
                    else:
                        warning "tag T already exists (from {globals|preset X})"
                        skip
                else:
                    emitted[ob.tag] = ob
                    emittedOrder.append(ob.tag)
            case "update":
                if emitted[ob.tag] not exists:
                    warning "update target T not found"
                    skip
                else:
                    target = emitted[ob.tag]
                    for each field in ob (except tag, mode, type):
                        applyMergeRule(target, field, ob[field])
                    emitted[ob.tag] = target  // patched
            default:
                warning "unknown mode"

return [emitted[tag] for tag in emittedOrder]
```

### Stage [3] cleanDanglingOutboundRefInRule

После того как все merged, проходим `config.route.rules[]`:
- Если `rule.outbound` указывает на tag НЕ в финальном `emitted` set → log warning, заменить на `route.final` (если она есть) ИЛИ drop весь rule (если final не задан).

---

## State / migration

**Никаких изменений в state.json.** v6 schema (`presets_v1`) уже хранит preset-ref как `{kind: "preset", ref, enabled, body: {vars}}`. Содержимое preset.outbounds лежит в template и не персистится в state.

Юзеру не нужна миграция — на свежем template'е с обновлёнными preset'ами enabled='ru-inside' юзеры просто увидят новые outbounds в config'е.

**Backward compat:** preset без `outbounds[]` секции работает как раньше. Старые state'ы без preset.outbounds — без изменений.

---

## UI integration

### Outbound picker (Rules tab inline + edit dialog)

`wizardbusiness.GetAvailableOutbounds(model)` сейчас читает `model.GlobalOutbounds` (snapshot of parser_config.outbounds после ContrastResolve). Нужно расширить:

```go
func GetAvailableOutbounds(model *WizardModel) []string {
    tags := collectGlobalOutboundTags(model)
    // NEW: добавляем tags от active preset-refs (с учётом vars / if-filter)
    tags = append(tags, collectActivePresetOutboundTags(model)...)
    return uniqSorted(tags)
}
```

Inline outbound select (см. `rules_unified_rows.go::buildSinglePresetRefRow`) — без изменений в коде, просто options-list будет содержать preset-emitted tags. Если preset с `out.default = "ru VPN 🇷🇺"` enabled — selector сразу видит этот tag.

### Library dialog

Без изменений. Library показывает preset'ы из template — содержимое `outbounds[]` в UI не отображается (как сейчас не отображается `rule_set[]`/`dns_servers[]`). Detail-view (если когда-то понадобится) может показать full preview.

---

## Template content migration

После shipping SPEC 055 — отдельным template-патчем:

1. **Убрать `filters: { tag: "!/(🇷🇺)/i" }`** из `parser_config.outbounds[]::proxy-out` и `auto-proxy-out` (нейтральный default)
2. **Убрать `ru VPN 🇷🇺` selector** из `parser_config.outbounds[]`
3. **Внести в `ru-inside` preset** (`presets[]`):
   ```jsonc
   "outbounds": [
     { "mode": "update", "tag": "proxy-out",      "filters": { "tag": "!/(🇷🇺)/i" } },
     { "mode": "update", "tag": "auto-proxy-out", "filters": { "tag": "!/(🇷🇺)/i" } },
     { "tag": "ru VPN 🇷🇺", "type": "selector", ... }
   ]
   ```
4. **Изменить `ru-inside.vars.out.default`** с `"ru VPN 🇷🇺"` (вшитый в globals) на `"ru VPN 🇷🇺"` (теперь preset-emitted — тег тот же)

`RequiredTemplateRef` bump → все юзеры подтянут изменённый template; для тех у кого `ru-inside` enabled — конфиг будет идентичен текущему (proxy-out отфильтрован + ru VPN присутствует); для disabled — proxy-out чище.

---

## Validation в loader

Расширить `core/template/preset_loader.go`:

```go
// Внутри LoadPresets, per preset:
for i, ob := range preset.Outbounds:
    mode := ob.Mode
    if mode == "":
        mode = "add"
    if mode != "add" and mode != "update":
        warning "preset X.outbounds[i]: unknown mode '%s'"
        continue
    if ob.Tag == "":
        warning "preset X.outbounds[i]: missing tag"; continue
    if mode == "update" and (ob.Type != "" or hasExplicitTypeOverride(ob)):
        warning "preset X.outbounds[i]: mode=update cannot change type; field will be dropped at build"
    if mode == "add" and ob.Type == "":
        warning "preset X.outbounds[i]: mode=add requires type"; continue
    // tag uniqueness check ВНУТРИ preset (нельзя дважды объявить один tag в одном preset)
    if seenTags[ob.Tag]:
        warning "preset X.outbounds[i]: tag %s duplicated in same preset"
    seenTags[ob.Tag] = true
```

Cross-preset / preset-vs-globals tag collision **НЕ** валидируется в loader'е (loader не знает порядок enablement) — это runtime check в Stage [2].

---

## Edge / opinionated

### Почему не разрешаем `mode: "replace"` (полная замена)?

`replace` имел бы семантику: «удалить существующий outbound с этим tag целиком, поставить новый». Это редкий use case и опасный (можно случайно убрать `addOutbounds` от другого preset'а). `update` покрывает 95% случаев. Если когда-то понадобится — добавить отдельно с явным флагом, как explicit destructive opt-in.

### Почему первый-wins а не последний-wins?

Globals в template — это «база». Presets ADD сверху. Если юзер хочет реально override (поменять semantics globals'а) — он должен сделать это явно через `mode: "update"`, а не через silent `add` с тем же tag'ом. Первый-wins для `add` — это защита от случайной перезаписи.

### Почему filters — replace, а addOutbounds — union?

`filters` это **политика выбора** (regex select); две независимые политики не композируются логически. `addOutbounds` это **список доступных опций**; естественная семантика union.

### Multi-update ordering

`ru-inside` обновляет `proxy-out` фильтром !RU. `ru-blocked` ТОЖЕ обновляет `proxy-out` (например задаёт `default: auto-ru-blocked`). Если оба preset'а enabled — порядок применения = порядок в Rules tab (RuleOrder). Юзер видит детерминизм через drag ↑↓.

---

## Risks

| Риск | Мера |
|---|---|
| Юзер enable preset'а с update'ом → меняется поведение глобального proxy-out → ломается другой preset/правило который полагался на старый proxy-out | Документация в `description` preset'а: «UPDATES global proxy-out: adds RU-exclude filter». UI может показать ⚠ marker. |
| `mode: "update"` на `wizard.required` outbound (proxy-out/auto-proxy-out) → если update снимает `wizard.required` или меняет `type` → нарушение invariant | Loader валидация: `type` нельзя менять, `wizard.required` нельзя снижать ниже текущего. |
| Cross-preset update'ы создают сложные mental-stack'и (preset A update poison'ит для preset B) | Логируем при каждом update «applied update from preset X to tag T», debug-view может показать «chain of updates» в IMPLEMENTATION_REPORT future. |
| Dangling outbound ref на user-defined rule после disable preset | Build cleanup fallback → route.final, warning в log. Юзеру в UI можно подсветить affected rule. |
| Tag со спец-символами (эмодзи `🇷🇺`) в `addOutbounds` или `rule.outbound` ссылка | sing-box поддерживает любые UTF-8 strings в tag'ах. Проверить на edge case с emoji в regex'ах `filters.tag`. |

---

## Migration plan

1. **SPEC 055 ship** (PRs + tests + template content update в одном бампе template ref).
2. **`docs/release_notes/upcoming.md`** entry (EN + RU): «preset.outbounds — self-contained outbound groups; ru-inside preset now ships its own RU-selector».
3. **No state migration needed** (см. State / migration выше).
4. **Existing user configs** с enabled `ru-inside` — config.json после update будет identical (proxy-out отфильтрован + ru VPN 🇷🇺 присутствует). Без `ru-inside` enabled — proxy-out чище, потеря: lost `ru VPN 🇷🇺` из selector dropdown'ов (если юзер вручную выбрал его на каком-то правиле — `cleanDanglingOutboundRefInRule` его пересадит на route.final).

---

# Implementation history

## Phase 0 — первая попытка (abandoned)

Изначальный план SPEC 055 (см. `PLAN.md` оригинал в git history до коммита `098c5e1`) — **post-merge JSON-патч**: native pipeline эмитит чистый sing-box outbounds[], потом отдельная функция накладывает preset.outbounds сверху (mutate JSON).

Это не сработало: preset.outbounds — это **parser-формат** (зеркало `configtypes.OutboundConfig` с `options/filters/addOutbounds/comment/wizard`), а финал — **sing-box формат** (с flatten'нутыми options, резолвнутыми filters, без launcher-only полей). Post-merge тащил launcher-only поля в финал → sing-box 1.12+ FATAL на каждом Rebuild. Каждый strip-фикс ловил один симптом, следующий запускался при следующем поле. 5 итераций «добавим ещё один strip» → код-хаос, фича так и не заработала.

Revert: `098c5e1 revert(spec-055): surgical revert of preset.outbounds implementation chaos`.

## Phases 1–8 — pre-patch parser_config (ex-SPEC 056)

Архитектурный сдвиг: preset.outbounds **типизированно** конвертится в `configtypes.OutboundConfig` и **pre-patch'ится** в deep-clone `parserCfg.ParserConfig.Outbounds[]` **до** запуска native `GenerateOutboundsFromParserConfig`. Native pipeline (без изменений с v0.9.5) сам делает options-flatten / filters→`filterNodesForSelector` / addOutbounds union / comment-as-prefix. **Ноль strip-функций** для outbound JSON.

Полный план в `IMPLEMENTATION_PLAN.md`. Полный отчёт в `IMPLEMENTATION_REPORT.md`. Кратко:

| Phase | Commit | Что |
|---|---|---|
| 1 | `098c5e1` | Surgical revert хаоса первой попытки |
| 2 | `4756b39` | `PresetOutbound` type + loader validation |
| 3 | `2b2e77a` | `ApplyPresetOutboundsToParserConfig` + `ExpandPresetOutbounds` |
| 4 | `8fb10f7` | Wire pre-patch в rebuild / Update / wizard Preview |
| 5 | `2d16895` | Route dangling outbound cleanup |
| 6 | `c20b24a` | UI: `collectActivePresetOutboundTags` + refresh-on-toggle |
| 7 | `ee6e8e4` + `b745f1d` | Template migration (RU presets) + RequiredTemplateRef bump |
| 8 | `23a7b10` | 27 unit-тестов + release notes |

## Post-ship: DNS schema cleanup (ex-SPEC 057, merged-in)

После Phase 8 manual-QA вскрылись 4 регрессии **того же класса** в DNS pipeline:

1. **`description` течёт в финал** для preset-bundled DNS-серверов (sing-box 1.12+ rejects) — `9daa3cd` ввёл `stripDNSWizardOnlyFields` как **single source of truth** sanitize-функцию для всех DNS server emit-путей (preset bundled, extra_servers, template defaults).
2. **User inline route rule с `protocol: bittorrent`** крашился — sing-box headless rule_set отвергает connection-level match-поля. `c60fd63` — emit user inline match **напрямую в route.rules[]** без `rule_set` обёртки.
3. **Template DNS library не материализуется** — `cloudflare_udp` enabled в DNS tab, но в config'е его нет, `default_domain_resolver: cloudflare_udp` → FATAL. `e96c86a` — populate `ctx.Preset.TemplateDNSDefaults` в build pipeline.
4. **Stale extras от старой template-версии** (`extra_rules` ссылается на `ru-domains` который теперь `russian:ru-domains`) — `9daa3cd` ввёл `cleanDanglingDNSRule` (зеркало route Phase 5 для DNS rules) + `collectRuleSetTagsFromPresets`.

**Инвариант (post-cleanup):** см. начало этого SPEC'а. `extra_servers` / `extra_rules` оставлены в `v6.DNSConfig` для **genuinely user-defined** содержимого (например `my-pihole` 192.168.1.5) — НЕ для копий template-тегов. Loader/migration валидируют: template tag в extras → конвертить в `template_servers` override.

См. `IMPLEMENTATION_REPORT.md::Phase 9` для полного списка commits и диффа.
