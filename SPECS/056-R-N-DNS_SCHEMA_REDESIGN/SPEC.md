# SPEC 056-R-N — DNS_SCHEMA_REDESIGN

**Status:** New (N)
**Type:** Refactor (R) — schema cleanup, no new user-facing features
**Depends on:** SPEC 053 (preset bundles — концепт остаётся), SPEC 055 (preset.outbounds — не трогаем)
**Bump:** state.json schema v6 → v7 (`presets_v1` → `presets_v2`)

---

## Что не так

SPEC 053 раздробил DNS-секцию state.json на 3 отдельных коллекции + размазал
скаляры в `state.vars[]` (унаследовано из SPEC 032). Результат: нечитаемо,
требует back-compat хаков (`legacyDNSOptionsFromV6`, `isV6DNSActive` guard,
double-emit cleanup), и UI-код вынужден жонглировать тремя источниками.

Сейчас (v6):

```json
{
  "vars": [
    {"name": "dns_strategy",                "value": "prefer_ipv4"},
    {"name": "dns_final",                   "value": "direct_dns_resolver"},
    {"name": "dns_independent_cache",       "value": "false"},
    {"name": "dns_default_domain_resolver", "value": "cloudflare_udp"}
    // … вперемешку с tun_address, clash_secret, и ещё 25+ переменными
  ],
  "dns": {
    "template_servers": { "<tag>": {"enabled": bool} },   // map
    "extra_servers":    [ { full server body } ],          // array
    "extra_rules":      [ { ... } ]                         // array
  }
}
```

**Проблемы:**

1. **Scalars потеряны.** `strategy`/`final`/`independent_cache`/`default_domain_resolver`
   в общей куче `vars[]` среди ~30 template-переменных. Не сгруппированы, контекст
   потерян, искать неудобно.

2. **DNS-серверы в 3 коллекциях.** template-overrides (map) + user-added (array) +
   bundled-runtime (нигде, эмитится из presets). Каждый источник со своим эмит-путём.

3. **Артефактные имена.** `extra_servers` / `extra_rules` не очевидны —
   "extra" по отношению к чему? Юзер думает «лишние? необязательные?», а на
   самом деле «**сверх** template + preset».

4. **Back-compat хаки.** `legacyDNSOptionsFromV6` материализует v6 в v5 view
   для UI, что создаёт double-emit риск, разрешённый ещё одним хаком
   (`isV6DNSActive` inline guard в `dnsConfigForUpdate`).

5. **Асимметрия с rules.** `state.rules[]` уже использует kind discriminator
   pattern и работает чисто. DNS — нет. Один и тот же продукт, разные принципы.

## Как должно быть

`kind` discriminator + flat layout — тот же паттерн что у `state.rules[]`:

```json
{
  "meta": {"version": 7, "schema": "presets_v2"},

  "rules": [
    // НЕ ТРОГАЕМ — оставляем как есть
    { "kind": "preset", "ref": "russian", "enabled": true, "body": {"vars": {}} },
    { "kind": "inline", "id":  "01J...", "enabled": true, "body": {...} },
    { "kind": "srs",    "id":  "01J...", "enabled": true, "body": {...} }
  ],

  "vars": [
    // dns_* скаляры удалены отсюда; остальные template vars остаются
  ],

  "dns_options": {
    "strategy":                "prefer_ipv4",
    "final":                   "direct_dns_resolver",
    "independent_cache":        false,
    "default_domain_resolver": "cloudflare_udp",

    "servers": [
      { "kind": "template", "tag": "cloudflare_udp", "enabled": true  },
      { "kind": "template", "tag": "google_doh",     "enabled": true  },
      { "kind": "template", "tag": "cloudflare_doh", "enabled": false },
      { "kind": "user",     "tag": "my-pihole", "type": "udp", "server": "192.168.1.5", "server_port": 53 }
    ],

    "rules": [
      { "kind": "user", "rule_set": "ru-domains", "server": "yandex_doh" }
    ]
  }
}
```

**Что меняется:**

| Сейчас | Стало |
|---|---|
| `state.dns.template_servers: {tag: {enabled}}` | `state.dns_options.servers[]` с `kind="template"` + `enabled` |
| `state.dns.extra_servers: [...]` | то же `servers[]` с `kind="user"` + full body |
| `state.dns.extra_rules: [...]` | `state.dns_options.rules[]` с `kind="user"` |
| `state.vars[].name="dns_*"` | `state.dns_options.{strategy,final,independent_cache,default_domain_resolver}` |
| Имя секции `dns` | `dns_options` (зеркалит template-секцию того же имени) |

**Bundled DNS-серверы от preset'ов:** runtime-only, в state НЕ хранятся (это
осталось хорошо в v6, не меняем). Эмитятся при build из `preset.dns_servers`.
UI может показывать их с пометкой «bundled by preset X», но в state.json не
сохраняет — disable preset → исчезают автоматически.

## Что НЕ трогаем

- `state.rules[]` — уже kind-based, работает чисто
- SPEC 055 preset.outbounds — отдельная фича, своя архитектура (pre-patch parser_config)
- `template.presets[]` концепт целиком — preset bundles остаются
- `template.dns_options.servers[]` как библиотека template DNS-серверов
- `template.config.dns.servers[]` (минимум: local_dns_resolver, direct_dns_resolver)
- Build pipeline для outbounds, route, parser_config — не касается

## Acceptance

1. `state.json` после Save имеет `meta.version: 7`, `schema: "presets_v2"`,
   секцию `dns_options` с скалярами и `servers[]`/`rules[]` через `kind`.
2. `vars[]` НЕ содержит `dns_*` элементов (миграция в `dns_options`).
3. **Удалены** функции:
   - `legacyDNSOptionsFromV6` (не нужна — нет двух views)
   - inline v5/v6 guard в `dnsConfigForUpdate` (всегда читаем из единого места)
   - `isV6DNSActive` (уже инлайнен в 9.7; после рефактора весь этот if тоже исчезает)
4. **Единый emit-путь** для DNS в build: `MergeDNSSection` + `MergePresetsIntoDNS`
   сливаются (или второй съедает первый) — один walk по `dns_options.servers[]`
   со `switch entry.Kind`.
5. UI рендерит DNS tab из flat `dns_options.servers[]` со kind-aware tile'ами
   (template = можно только toggle enable; user = можно edit/delete; bundled
   преfix `<preset>:` = read-only с пометкой preset'а-источника).
6. Migration v6 → v7 idempotent с backup `state.json.v6.bak`. Существующие
   v6 state'ы (мой dev + любые hypothetical tester'ские) грузятся, конвертятся
   на первом Save, ничего не теряют.
7. `docs/WIZARD_STATE.md` переписан под новую схему (попутно — раз уж doc
   всё равно отстал).

## Не в скоупе

- Откат SPEC 053 целиком — preset bundles концепт остаётся, route rules `kind`
  pattern остаётся
- Откат SPEC 055 — preset.outbounds работает, не трогаем
- Изменения UI кроме DNS tab
- Изменения в template structure

## План фаз (детально — в `IMPLEMENTATION_PLAN.md` после approval)

1. **Phase 1 — Schema:** новый `v7.DNSOptions` struct с `Strategy/Final/...` +
   `Servers []DNSServer{Kind, Tag, Enabled, Body}` + `Rules []DNSRule{Kind, Body}`.
   Bump `SchemaVersion=7`, `SchemaName="presets_v2"`.

2. **Phase 2 — Migration:** `MigrateV6ToV7` — конвертит v6 в v7 с backup'ом
   `state.json.v6.bak`. Walk:
   - `dns.template_servers` → `dns_options.servers[]` с kind="template"
   - `dns.extra_servers` → `dns_options.servers[]` с kind="user"
   - `dns.extra_rules` → `dns_options.rules[]` с kind="user"
   - `vars[dns_*]` → `dns_options.{strategy,final,independent_cache,default_domain_resolver}`

3. **Phase 3 — Build pipeline:** переписать `MergeDNSSection` под единый walk
   с kind switch. Удалить `MergePresetsIntoDNS` extras loops (объединить в
   единый emit). Удалить `legacyDNSOptionsFromV6`. Удалить inline v5/v6 guard
   в `dnsConfigForUpdate`.

4. **Phase 4 — UI sync:** переписать `SyncDNSFullToStateV6` под flat shape с kind.

5. **Phase 5 — UI rendering:** DNS tab tile'ы становятся kind-aware:
   template → toggle only, user → edit/delete, bundled → read-only с preset hint.

6. **Phase 6 — Tests + docs:** unit'ы под flat shape, `docs/WIZARD_STATE.md`
   переписан, `IMPLEMENTATION_REPORT.md` финал.

## Риск / rollback

Schema change — нужна аккуратная миграция. Idempotent backup'ы (state.json.v6.bak),
тесты на round-trip v6→v7→v6 (потеря только в имени поля + структуре, данные
сохраняются 1:1). На любом этапе можно остановиться — v6 read-path остаётся
до того момента как Phase 2 migration отработает на конкретном state'е.
