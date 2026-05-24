# SPEC 056-R-N — DNS_SCHEMA_REDESIGN

**Status:** New (N)
**Type:** Refactor (R) — schema cleanup, no new user-facing features
**Depends on:** SPEC 053 (preset bundles — концепт остаётся), SPEC 055 (preset.outbounds — не трогаем)
**Schema:** последний релизный формат — **v5** (`v0.9.5`). **v6 / `presets_v1`
никогда не релизился** — это дев-схема, существует только в HEAD-of-develop
после SPEC 053. Этот SPEC **переписывает дев-схему на месте**, БЕЗ инкремента
номера. Когда консолидированный preset-стек уйдёт в релиз, тогда и будет
честный bump v5 → v6 уже с правильным дизайном.

---

## Что не так в v6 (дев-схема, SPEC 053)

```json
"dns": {
  "template_servers": { "<tag>": {"enabled": bool} },   // map
  "extra_servers":    [ { full server body } ],          // array
  "extra_rules":      [ { ... } ]                         // array
}
```

**Проблемы (только в DNS — `state.rules[]` route part сделан правильно через kind):**

1. **DNS-серверы в 3 коллекциях.** template-overrides (map) + user-added (array) +
   bundled-runtime (нигде, эмитится из presets). Каждый источник со своим эмит-путём.

2. **Артефактные имена `extra_*`.** Не очевидны — "extra" по отношению к чему? Юзер
   читает «лишние? необязательные?», а по факту «**сверх** template + preset».

3. **Имя секции `dns` неинформативно.** В template'е та же секция называется
   `dns_options`, а в state — `dns`. Зеркальность ломается, в коде вечная путаница.

4. **Back-compat хаки.** `legacyDNSOptionsFromV6` материализует v6 в v5 view
   для UI back-compat → создаёт double-emit риск → разрешён inline guard'ом
   v5/v6 в `dnsConfigForUpdate` → плодит сущности.

5. **Асимметрия с rules.** `state.rules[]` уже использует kind discriminator
   pattern (preset/inline/srs) и работает чисто. DNS — нет. Один продукт,
   разные принципы.

## Как должно быть

`kind` discriminator + flat layout — тот же паттерн что у `state.rules[]`.
Schema version и schema name **не меняются** (v6 / `presets_v1` остаются как
дев-маркер до релиза):

```json
{
  "meta": { "version": 6, "schema": "presets_v1" },

  "rules": [
    // НЕ ТРОГАЕМ — kind discriminator уже работает
    { "kind": "preset", "ref": "russian", "enabled": true, "body": {"vars": {}} },
    { "kind": "inline", "id":  "01J...",  "enabled": true, "body": {...} },
    { "kind": "srs",    "id":  "01J...",  "enabled": true, "body": {...} }
  ],

  "vars": [
    // НЕ ТРОГАЕМ — единый KV-store для всех template переменных,
    // включая dns_strategy / dns_final / dns_independent_cache /
    // dns_default_domain_resolver. UI группирует по табам, state — flat.
    { "name": "tun", "value": "true" },
    { "name": "dns_strategy", "value": "prefer_ipv4" },
    ...
  ],

  "dns_options": {
    "servers": [
      { "kind": "template", "tag": "cloudflare_udp",      "enabled": true  },
      { "kind": "template", "tag": "google_doh",          "enabled": true  },
      { "kind": "template", "tag": "cloudflare_doh",      "enabled": false },
      { "kind": "preset",   "ref": "russian:yandex_udp",  "enabled": true  },
      { "kind": "preset",   "ref": "russian:yandex_doh",  "enabled": true  },
      { "kind": "preset",   "ref": "russian:yandex_dot",  "enabled": false },
      { "kind": "user",     "tag": "my-pihole", "type": "udp", "server": "192.168.1.5", "server_port": 53 }
    ],
    "rules": [
      { "kind": "preset", "ref": "russian",                                      "enabled": true },
      { "kind": "user",   "rule_set": "ru-domains", "server": "yandex_doh",      "enabled": true }
    ]
  }
}
```

**Что меняется:**

| Сейчас (v6) | Стало |
|---|---|
| `state.dns.template_servers: {tag: {enabled}}` (map) | `state.dns_options.servers[]` с `kind="template"` + `enabled` |
| `state.dns.extra_servers: [...]` (array, full body) | то же `servers[]` с `kind="user"` + full body |
| Preset bundled DNS-серверы — runtime-only, **в state нет** | в `state.dns_options.servers[]` с `kind="preset"` + `ref` (memory == disk) |
| Preset bundled DNS rule — runtime-only, **в state нет** | в `state.dns_options.rules[]` с `kind="preset"` + `ref` (memory == disk) |
| `state.dns.extra_rules: [...]` | `state.dns_options.rules[]` с `kind="user"` |
| Имя секции `dns` | `dns_options` (зеркалит template-секцию того же имени) |
| Имена `extra_*` в коде | `kind="user"` / `kind="preset"` discriminator |

## Lifecycle entries `kind=preset` — memory == disk

Когда юзер toggle'ит preset в Rules tab, launcher **синхронизирует** state.dns_options
с активным набором preset'ов. Никакого runtime materialization — то что в state
есть то и эмитится в config.

**Enable preset** (например `russian`):
1. `state.rules[]` ← добавляется `{kind:preset, ref:"russian", enabled:true, body:{vars:{}}}`
2. `state.dns_options.servers[]` ← добавляются entries по одной на каждый
   `template.presets.russian.dns_servers[i]`:
   ```json
   {"kind":"preset", "ref":"russian:yandex_udp", "enabled":true}
   {"kind":"preset", "ref":"russian:yandex_doh", "enabled":true}
   {"kind":"preset", "ref":"russian:yandex_dot", "enabled":true}
   ```
3. `state.dns_options.rules[]` ← добавляется (если preset имеет dns_rule):
   ```json
   {"kind":"preset", "ref":"russian", "enabled":true}
   ```

**Disable preset:**
- ВСЕ entries с ref на disabled preset удаляются:
  - `state.rules[]`: где `ref == "<preset_id>"`
  - `state.dns_options.servers[]`: где `ref` начинается с `"<preset_id>:"`
  - `state.dns_options.rules[]`: где `ref == "<preset_id>"`

**Что можно / нельзя с entries:**

| Действие | `kind="template"` | `kind="preset"` | `kind="user"` |
|---|---|---|---|
| Toggle `enabled` (включить/выключить эмит этой entry в config) | ✅ | ✅ | ✅ |
| Edit body (type/server/port/...) | ❌ (тело в `template.dns_options.servers[tag]`) | ❌ (тело в `template.presets[<id>].dns_servers[<local_tag>]`) | ✅ |
| Delete entry | ❌ (template-defined) | ❌ (управляется только preset enable/disable) | ✅ |

Например `russian` preset принёс три DNS-сервера. Юзер хочет только `yandex_udp` —
ставит чекбоксы на остальных в OFF (`enabled:false`). Entries остаются в списке UI
пока preset 'russian' enabled, но не эмитятся в config. Disable preset 'russian' →
все три entries удаляются автоматом.

**Инварианты:**
1. **Memory == disk.** state.dns_options.{servers,rules}[] всегда содержит ровно то
   что юзер видит в DNS tab и что эмитится в config. Никакого runtime materialization
   разрыва.
2. **No broken refs.** preset disabled → entries удалены. Если в state ref на
   preset/tag которого нет в template (template обновился, отпало) → загрузка
   чистит entry с warning'ом.
3. **Immutable body** для `kind=template` и `kind=preset`: на диске только
   `{kind, ref/tag, enabled}` — тело подсасывается из template на build/render.
4. **Source of truth для UI:** один walk по state.dns_options.servers[] +
   render по `kind`.

## Что НЕ трогаем

- `state.rules[]` — уже kind-based, работает чисто
- `state.vars[]` — единый KV-store, включая `dns_*` scalars (UI группирует
  по таб'ам, state.json — flat; перенос в `dns_options.vars[]` был бы
  синтаксическим сахаром без логического выигрыша)
- SPEC 055 preset.outbounds — отдельная фича, своя архитектура (pre-patch parser_config)
- `template.presets[]` концепт целиком — preset bundles остаются
- `template.dns_options.servers[]` как библиотека template DNS-серверов
- `template.config.dns.servers[]` (минимум: local_dns_resolver, direct_dns_resolver)
- Build pipeline для outbounds, route, parser_config — не касается

## Acceptance

1. `state.json` после Save имеет секцию `dns_options.servers[]`/`rules[]`
   через `kind` discriminator. Секции `dns.template_servers`,
   `dns.extra_servers`, `dns.extra_rules` отсутствуют.
2. `state.vars[]` неизменно — `dns_*` scalars остаются как раньше.
3. **Удалены** функции:
   - `legacyDNSOptionsFromV6` (не нужна — нет двух views)
   - inline v5/v6 guard в `dnsConfigForUpdate` (всегда читаем из единого места)
   - `collectRuleSetTagsFromPresets` + `cleanDanglingDNSRule` — пересматриваем:
     остаются если нужны для `dns_options.rules[kind=user]` с `rule_set` ссылками
     на preset rule_set'ы (дроп dangling при disable preset)
4. **Единый emit-путь** для DNS в build: `MergeDNSSection` + `MergePresetsIntoDNS`
   сливаются — один walk по `dns_options.servers[]` со `switch entry.Kind` +
   merge с template-эмитнутыми + preset-bundled.
5. UI рендерит DNS tab из flat `dns_options.servers[]` со kind-aware tile'ами:
   - `kind="template"` → toggle enable, edit/delete заблокированы
   - `kind="preset"` → toggle enable (юзер может скрыть отдельный сервер из preset'а),
     edit/delete заблокированы; пометка «from preset \<id\>» рядом с ref
   - `kind="user"` → полный edit + delete
6. **Dev state переписывается на месте.** Schema version и name НЕ меняются
   (v6 / `presets_v1` остаются маркером дев-стека до релиза). На первой
   загрузке после деплоя старого-дев-v6 формата: read со старого shape
   (`dns.template_servers`/`extra_servers`/`extra_rules`) через
   `legacyDevDNSToOptions` fallback, write в новом shape на ближайшем Save.
   Backup НЕ делаем — конверсия lossless (round-trip тест), v6 не релизился
   (dev-only state у одного разработчика). Кто хочет страховку — `cp` руками.
   Шиппнутые юзеры на v5 — не затрагиваются (parseV5 продолжает работать
   как раньше; в v6-shape переходят только при первом Save с preset-ref'ом
   как обычно, но уже в **правильном** layout'е).
7. `docs/WIZARD_STATE.md` переписан под актуальную схему (попутно —
   раз уж doc всё равно отстал).

## Не в скоупе

- Перенос `dns_*` scalars из `state.vars[]` в `dns_options` (обсуждалось,
  отбито — это был бы синтаксический сахар без логического выигрыша; единый
  KV-store `state.vars[]` остаётся)
- Откат SPEC 053 целиком — preset bundles концепт остаётся, route rules `kind`
  pattern остаётся
- Откат SPEC 055 — preset.outbounds работает, не трогаем
- Изменения UI кроме DNS tab
- Изменения в template structure

## План фаз (детально — в `IMPLEMENTATION_PLAN.md` после approval)

1. **Phase 1 — Schema:** новый `v6.DNSOptions` struct: `Servers []DNSServer{Kind,
   Tag, Ref, Enabled, Body map[string]interface{}}` + `Rules []DNSRule{Kind, Ref,
   Enabled, Body map[string]interface{}}`. Тело `Body` непустое только для
   `kind=user`. `Tag` для template, `Ref` для preset (формат `"<preset_id>:<local_tag>"`
   для серверов, `"<preset_id>"` для rules). Schema version и name **не меняются**.
   Старый `v6.DNSConfig` (template_servers/extra_servers/extra_rules) удаляется.

2. **Phase 2 — Preset toggle sync:** новая функция `SyncDNSOptionsWithActivePresets(
   state, presets) error`. Вызывается при каждом toggle preset enable/disable в
   Rules tab И на загрузке state'а (idempotent). Семантика:
   - Для каждого `state.rules[].kind=preset && enabled=true`: ensure entries
     `{kind:preset, ref:<id>:<local>}` присутствуют в `state.dns_options.servers[]`
     по числу `template.presets[id].dns_servers[]`. Default `enabled` берётся из
     template или из существующей entry если она уже была (preserve user toggle).
   - То же для `state.dns_options.rules[]` если preset имеет `dns_rule`.
   - Для disabled или удалённых preset'ов: удалить entries с соответствующим
     prefix'ом ref.

3. **Phase 3 — In-place dev rewrite:** на parseV6 — если встречаем старый
   shape (`dns.template_servers` / `dns.extra_servers` / `dns.extra_rules`),
   читаем по-старому через `legacyDevDNSToOptions`, конвертим в in-memory
   `v6.DNSOptions`. **Дополнительно** материализуем `kind=preset` entries для
   всех active `state.rules[].kind=preset` (вызов SyncDNSOptionsWithActivePresets
   из Phase 2 — происходит на сторонне презентера при load). На ближайшем
   Save файл перезаписывается в новом layout'е. **Backup НЕ делаем** —
   YAGNI: dev-only, lossless conversion (round-trip тест покрывает).

4. **Phase 4 — Build pipeline:** переписать `MergeDNSSection` + `MergePresetsIntoDNS`
   под единый walk с kind switch (template/preset/user). Удалить
   `legacyDNSOptionsFromV6`, inline v5/v6 guard в `dnsConfigForUpdate`,
   старый runtime materialization preset DNS bundled в `ExpandPreset`.
   `cleanDanglingDNSRule` остаётся для `kind=user` DNS rules с `rule_set`
   ссылками на preset rule_set'ы (защита от disable race).

5. **Phase 5 — UI sync:** переписать `SyncDNSFullToStateV6` под flat shape
   с kind. Preset toggle handler в Rules tab вызывает `SyncDNSOptionsWithActivePresets`
   после изменения. UI рендерит DNS tab из единого `state.dns_options.servers[]`
   с kind-aware tile'ами (template: toggle only; preset: toggle only + «from preset
   X» label; user: full edit + delete).

6. **Phase 6 — Tests + docs:** unit'ы под flat shape, lifecycle тесты
   (enable→sync→disable→sync = entries up/down), `docs/WIZARD_STATE.md`
   переписан, `IMPLEMENTATION_REPORT.md` финал.

## Объём работы

~100 LOC чистого прироста (~170 нового кода − ~70 удалений legacy хаков и
runtime materialization). +`SyncDNSOptionsWithActivePresets` ~50 LOC новой
функции. ~1.5 дня focused work. Риск низкий-средний — дев-only schema rewrite,
шипп v5 не затрагивается; новая lifecycle логика покрыта unit-тестами.

## Риск / rollback

Дев-only schema rewrite. Шиппнутая v5 не затрагивается (parseV5 работает
как раньше). Дев-юзеры (только разработчик) получают `legacyDevDNSToOptions`
fallback read для совместимости при первой загрузке — на ближайшем Save
файл перезаписывается в новом layout'е. Backup автоматический НЕ делается
(YAGNI; кто хочет страховку — `cp state.json state.json.bak` руками перед
апгрейдом). Тесты на round-trip старый-shape → новый-shape → emit
(данные сохраняются 1:1 по семантике).

На любом этапе можно остановиться — старый read-path (`legacyDevDNSToOptions`)
остаётся в коде до того момента как **все** dev-state'ы перешли на новый
shape (~один release-cycle), потом удаляется отдельным cleanup-commit'ом.

---

## Scope expansion (post-original)

Изначальный scope (Phase 1-6 выше) был выполнен. В процессе обнаружились
смежные техдолги, которые исправлены в той же итерации — см. **IMPLEMENTATION_REPORT.md**
(Phase A через F):

- **Phase B — Unified Resolver pattern (DNS + Route).** `ResolveDNS()` +
  `ResolveRoute()` как pure-func single source of truth для UI и build emit.
  Build pipeline (`MergePresetsIntoDNS`/`Route`) переписаны как thin wrappers.
- **Phase C — Template DNS unify через `required: true` маркер.** Merge
  `config.dns.servers[]` (минимум) и `dns_options.servers[]` (библиотека) в
  один list. `local_dns_resolver`/`direct_dns_resolver` теперь в dns_options
  с `required: true`.
- **Phase D — UI widget pattern consistency.** DNS Servers + Rules tab
  единый widget pattern (HoverRow + WireTooltip) для всех 3 kind'ов.
- **Phase E — Outbounds tab unify.** Preset bundled outbounds показываются
  в общем списке (dedup vs globals). Disabled subscription cascade — preview
  cache / collectRows / collectAllTags паритетны build pipeline'у. Required
  outbound mechanism с Reset кнопкой.
- **Phase F — Cleanup.** Dead code (`GetWizardRequired`, `maybeBackupDevV6`,
  `model.DNSPresetServerEnabled` карты, `WizardConfig.Required`) удалены.

**Полный список изменений + diff stats:** см. `IMPLEMENTATION_REPORT.md` и `TASKS.md`.

## Follow-up backlog (не сделано)

- **Preset reorder в Outbounds tab** (юзер попросил, но требует архитектурного
  изменения build pipeline — persistent global+preset ordering).
- **Preset DNS reorder** аналогично.
- **Golden test для byte-identical config.json** до/после — есть скелет
  (`core/build/golden_test.go`), но скрыт за `GOLDEN_RUN_REAL=1`. CI parity
  не проверена; тестировалось ручным запуском.
