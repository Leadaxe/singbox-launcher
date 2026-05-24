# Wizard state (state.json)

Декларативная модель Configurator: где лежит, как загружается, как сохраняется,
куда уходит при build. Файл переписан под schema v6 (SPEC 053 + SPEC 056-R-N
+ SPEC 057-R-N), v5 описан только в разделе «Миграции».

---

## 1. Файлы и расположение

- **`bin/wizard_states/state.json`** — текущий снимок. Единственный файл,
  читаемый при старте Configurator'а и при headless rebuild config.json.
- **`bin/wizard_states/<id>.json`** — именованные снимки (Save As).
  Структурно идентичны `state.json`; при Read копируются поверх `state.json`.
- **`bin/subscriptions/<source_id>.raw`** — per-source raw body cache подписки
  (atomic .tmp + rename). Read-path парсит .raw напрямую без сети.

ExecDir resolve описан в SPECS/022 (macOS app support directories). На macOS
release-сборке это `~/Library/Application Support/singbox-launcher/bin/...`,
в dev-сборке — рядом с бинарём.

---

## 2. Top-level schema v6 (canonical)

```jsonc
{
  "meta": {
    "version": 6,
    "schema":  "presets_v1",
    "comment": "...",
    "created_at": "RFC3339 UTC",
    "updated_at": "RFC3339 UTC"
  },

  "connections": {
    "sources":   [ ... ],     // per-source subscription / server entries
    "outbounds": [ ... ],     // global outbound selectors / urltests
    "defaults":  { "reload": "4h", "max_nodes": 3000 }
  },

  "rules": [
    { "kind": "preset", "ref": "...",  "enabled": true, "body": { "vars": {} } },
    { "kind": "inline", "id":  "...",  "enabled": true, "body": { "name": "...", "match": {}, "outbound": "..." } },
    { "kind": "srs",    "id":  "...",  "enabled": true, "body": { "name": "...", "srs_url": "...", "outbound": "..." } }
  ],

  "vars": [
    { "name": "tun",          "value": "true" },
    { "name": "dns_strategy", "value": "prefer_ipv4" },
    ...
  ],

  "dns_options": {
    "strategy":                "...",   // optional fallback; source of truth — vars[]
    "final":                   "...",
    "default_domain_resolver": "...",
    "servers": [
      { "kind": "template", "tag": "...",        "enabled": true  },
      { "kind": "preset",   "ref": "<pid>:<tag>", "enabled": true },
      { "kind": "user",     "tag": "...", "type": "...", "server": "...", "enabled": true, ... }
    ],
    "rules": [
      { "kind": "preset", "ref": "<pid>", "enabled": true },
      { "kind": "user",   "enabled": true, ... }
    ]
  }
}
```

Top-level keys, отсутствующие в v6 (vs предыдущих ревизий):
`id` (snapshot-имя живёт в имени файла), `config_params`, `custom_rules`,
`selectable_rule_states`, `rules_library_merged`, `dns_options.independent_cache`.

---

## 3. Детальные схемы по секциям

### 3.1 `connections.sources[i]`

Дискриминатор `type`: `subscription` (URL → пачка нод) или `server` (один URI → один outbound).

| Поле | Тип | Когда | Описание |
|------|-----|-------|----------|
| `id` | string | всегда | ULID (Crockford-base32, 26 символов). Стабильный — переживает Save/Load. Имя файла `bin/subscriptions/<id>.raw`. |
| `type` | string | всегда | `subscription` \| `server`. |
| `enabled` | bool | всегда | Source активен. Disabled → его outbound'ы не попадают в финальный config. |
| `label` | string | опц. | Display name (для server обязательно для UX; для subscription — fallback из `meta.profile_title`). |
| `exclude_from_global` | bool | опц. | Исключить из global `proxy-out` / `auto-proxy-out`. |
| `url` | string | subscription | URL подписки. |
| `skip` | `[]map[string]string` | subscription | Skip-rules (имена нод которые не парсить). |
| `tag` | `{prefix, postfix, mask}` | subscription | Преобразование tag'ов нод (BL: префиксы и т.п.). `mask` overrides prefix+postfix. |
| `outbounds` | `[]OutboundConfig` | subscription | Per-source local outbound'ы (BL:auto / BL:select urltest+selector). |
| `expose_group_tags_to_global` | bool | subscription | Выставлять локальные group-tag'и в global selector. См. SPEC 026. |
| `update` | `{interval_hours, auto_refresh}` | subscription | Per-source override default reload interval. |
| `max_nodes` | int | subscription | Per-source override `defaults.max_nodes`. |
| `meta` | `SubscriptionMeta` | subscription | Runtime данные (см. ниже), заполняется Update'ом. |
| `uri` | string | server | vless:// / vmess:// / wireguard:// / etc. — один сервер. |

**JSON example — subscription source:**
```jsonc
{
  "id": "01KQCTRQBSSF0CCYFD2WWTVY9R",
  "type": "subscription",
  "enabled": true,
  "exclude_from_global": false,
  "url": "https://example.com/sub.txt",
  "tag": { "prefix": "BL:" },
  "outbounds": [
    {
      "tag": "BL:auto",
      "type": "urltest",
      "options": { "interval": "5m", "tolerance": 100, "url": "https://cp.cloudflare.com/generate_204" }
    },
    {
      "tag": "BL:select",
      "type": "selector",
      "options": { "default": "BL:auto" },
      "addOutbounds": ["BL:auto"]
    }
  ],
  "expose_group_tags_to_global": true,
  "update": { "interval_hours": 4, "auto_refresh": true },
  "max_nodes": 3000,
  "meta": {
    "profile_title": "My VPN Pack",
    "url_at_fetch": "https://example.com/sub.txt",
    "last_fetched_at": "2026-05-24T13:56:25Z",
    "last_status": "ok",
    "http_status_code": 200,
    "raw_body_bytes": 46318,
    "nodes_count_fetched": 148,
    "userinfo": { "upload_bytes": 0, "download_bytes": 1024000, "total_bytes": 107374182400, "expire_unix": 1735689600 },
    "preview_nodes": [ "vless://...", "ss://...", "..." ]
  }
}
```

**JSON example — server source:**
```jsonc
{
  "id": "01KQCXYZ...",
  "type": "server",
  "enabled": true,
  "label": "My direct server",
  "uri": "vless://uuid@host:443?type=tcp&security=reality&pbk=...#MyServer"
}
```

**Drilldown — поле `meta` (subscription runtime данные):**

| Поле | Описание |
|------|----------|
| `profile_title` | Из `subscription-profile-title` header или inline `#profile_title:` в первой строке body. |
| `profile_update_interval_hours`, `support_url`, `profile_web_page_url`, `content_disposition_filename` | Headers (response + inline body). |
| `userinfo` | `{upload_bytes, download_bytes, total_bytes, expire_unix}` — раскрытый `subscription-userinfo` header (V2Board/Xboard). |
| `url_at_fetch`, `last_fetched_at`, `last_status`, `error_count`, `last_error_msg`, `http_status_code`, `raw_body_bytes` | Fetch history. |
| `nodes_count_fetched`, `truncated`, `preview_nodes` | Результат парсинга. `truncated` = обрезали по `max_nodes`. |

### 3.2 `connections.outbounds[i]` — `OutboundConfig`

Top-level entry в global outbounds. Может быть трёх типов:
- **обычный global** (template/user) — без `ref`, без `updates`
- **preset add** — `ref = "<preset_id>"`, body от preset'а, UI read-only
- **target preset update** — без `ref`, но с `updates[]` стеком от preset'ов

| Поле | Тип | Описание |
|------|-----|----------|
| `tag` | string | Display tag, уникальный в рамках global outbounds. |
| `type` | string | sing-box type: `selector` / `urltest` / `direct` / `vless` / etc. |
| `options` | `map[string]interface{}` | sing-box options (default, interrupt_exist_connections, interval, ...) — emit'ятся flat в финальный outbound JSON. |
| `filters` | `map[string]interface{}` | UI/build-only: regex-фильтры по tag нод (`{"tag": "!/(RU)/i"}`). Не попадает в sing-box; используется `filterNodesForSelector` в generator. |
| `addOutbounds` | `[]string` | UI/build-only: дополнительные tag'и которые включить в `selector.outbounds[]` (union с подходящими по filter нодами). |
| `preferredDefault` | `map[string]interface{}` | UI/build-only: метаданные предпочтительного default для selector'а. |
| `comment` | string | UI/build-only: comment-prefix эмитится как `// <comment>\n` перед outbound JSON. |
| `wizard` | `string` \| `{hide: bool}` | UI: hide entry из Wizard (legacy "hide" string или новый object). |
| `ref` | string (omitempty) | **SPEC 057.** Не пусто → entry от preset add. Значение = `preset.id`. Lifecycle: enable preset → entry создаётся, disable → удаляется. UI: read-only (View вместо Edit, без Del). |
| `updates` | `[]OutboundUpdate` (omitempty) | **SPEC 057.** Стек patches от preset'ов через `mode=update`. Финальное body = base + apply patches в order. Lifecycle: enable/disable preset → push/drop entry с этим `ref`. |

**`OutboundUpdate{ref, patch}`** — одна запись в `updates[]`:

| Поле | Тип | Описание |
|------|-----|----------|
| `ref` | string | `preset.id` owner'а patch'а. |
| `patch` | `map[string]interface{}` | Сами поля для merge (filters / options / addOutbounds / preferredDefault / wizard / comment). `tag` и `type` immutable, не в patch'е. |

Merge semantics (см. `core/build/resolve_outbounds.go::applyOutboundUpdate`):
- `filters` — full replace если в patch
- `options.*` — per-key replace (не глубокий merge)
- `addOutbounds` — union с dedup
- `preferredDefault`, `wizard`, `comment` — replace

**Псевдо-поле `required`:**
Не существует в struct `OutboundConfig`. Читается live из `template.parser_config.outbounds[].required` каждый UI render (`templateRequiredTags` в `outbounds_configurator`). State.json НЕ сохраняет — иначе нельзя было бы снять флаг в template и увидеть эффект.

**JSON examples — три типа entry:**
```jsonc
// 1. Обычный user/template global — без preset binding
{
  "tag": "direct-out",
  "type": "direct"
}

// 2. Template global С update-патчами от 2 preset'ов
// Финальное body на emit = base + apply patches в order.
{
  "tag": "proxy-out",
  "type": "selector",
  "options": { "default": "auto-proxy-out", "interrupt_exist_connections": true },
  "addOutbounds": ["direct-out", "auto-proxy-out"],
  "comment": "Proxy group for everything that should go through VPN.",
  "updates": [
    { "ref": "russian",   "patch": { "filters": { "tag": "!/(🇷🇺)/i" } } },
    { "ref": "ru-inside", "patch": { "filters": { "tag": "!/(🇷🇺)/i" }, "addOutbounds": ["ru-inside-out"] } }
  ]
}

// 3. Preset add entry — body от preset'а, ref привязка
// На disable preset "russian" → эта запись Sync удалит.
{
  "tag": "ru VPN 🇷🇺",
  "type": "selector",
  "options": { "default": "direct-out" },
  "filters": { "tag": "/(🇷🇺)/i" },
  "addOutbounds": ["direct-out"],
  "ref": "russian"
}
```

### 3.3 `connections.defaults`

| Поле | Тип | Default | Описание |
|------|-----|---------|----------|
| `reload` | string | `"4h"` | Default reload interval подписок (per-source override через `Source.Update.IntervalHours`). |
| `max_nodes` | int | `3000` | Default cap нод на subscription (per-source override через `Source.MaxNodes`). |

**JSON example:**
```jsonc
{ "reload": "4h", "max_nodes": 3000 }
```

### 3.4 `rules[i]` — `v6.Rule` (SPEC 053)

Дискриминатор `kind`: `preset` / `inline` / `srs`. Один упорядоченный массив; порядок = порядок эмита в `config.json::route.rules[]`.

| Поле | Тип | Когда | Описание |
|------|-----|-------|----------|
| `kind` | string | всегда | Discriminator. |
| `ref` | string | `kind=preset` | Ссылка на `template.presets[].id`. |
| `id` | string | `kind=inline` \| `srs` | ULID. |
| `enabled` | bool | всегда | Общий toggle. |
| `body` | raw JSON | всегда | Kind-specific payload, декодируется через `DecodeBody`. |

**Body schemas:**

| Kind | Body shape |
|------|------------|
| `preset` | `{ vars: { <name>: <value>, ... } }` — **только diff** от template default'ов. Пустой map = всё дефолтное. Bump'нули template → юзер автоматически получает новые дефолты для var'ов которые не трогал. |
| `inline` | `{ name: string, match: { <sing-box match keys> }, outbound: string }` — outbound = tag или зарезервированный литерал (`reject` / `drop`). |
| `srs` | `{ name: string, srs_url: string, outbound: string }` — URL .srs файла + outbound tag/литерал. |

**JSON examples — три kind'а:**
```jsonc
// 1. Preset-ref правило (вся семантика в template.presets["russian"])
{
  "kind": "preset",
  "ref": "russian",
  "enabled": true,
  "body": { "vars": { "out": "proxy-out" } }  // только переопределённые vars
}

// 2. Inline user rule
{
  "kind": "inline",
  "id": "01KQD5XYZ...",
  "enabled": true,
  "body": {
    "name": "BitTorrent direct",
    "match": { "protocol": "bittorrent" },
    "outbound": "direct-out"
  }
}

// 3. SRS rule-set user rule
{
  "kind": "srs",
  "id": "01KQD7ABC...",
  "enabled": true,
  "body": {
    "name": "Block ads (oisd)",
    "srs_url": "https://example.com/oisd.srs",
    "outbound": "reject"
  }
}
```

### 3.5 `vars[i]`

Простой KV-список — overrides для всех template-объявленных vars.

| Поле | Тип | Описание |
|------|-----|----------|
| `name` | string | Имя из `template.vars[].name`. |
| `value` | string | User-overrides (значение всегда строка; типизация — на template-стороне через `vars[].type`). |

Типичные ключи (определяются template'ом):
- `tun` (bool-as-string: `"true"`/`"false"`) — TUN mode toggle
- `route_final` — выбор final outbound в Rules tab
- `dns_strategy`, `dns_final`, `dns_default_domain_resolver` — DNS scalars
- `clash_secret` — автогенерируемый bearer для Debug API
- Любые user-определённые в template

Записи без `name` (template `{separator: true}`) НЕ попадают в state. Сироты (имена не из текущего template) НЕ грузятся в model и НЕ пишутся обратно.

**JSON example:**
```jsonc
[
  { "name": "tun", "value": "true" },
  { "name": "route_final", "value": "proxy-out" },
  { "name": "dns_strategy", "value": "prefer_ipv4" },
  { "name": "dns_final", "value": "cloudflare_udp" },
  { "name": "dns_default_domain_resolver", "value": "cloudflare_udp" },
  { "name": "clash_secret", "value": "<auto-generated bearer>" }
]
```

### 3.6 `dns_options`

| Поле | Тип | Описание |
|------|-----|----------|
| `strategy` | string (omitempty) | Fallback дубль `vars["dns_strategy"]` для in-memory. Source of truth — `vars`. На диск не пишется если == zero. |
| `final` | string (omitempty) | То же, дубль `vars["dns_final"]`. |
| `default_domain_resolver` | string (omitempty) | То же, дубль `vars["dns_default_domain_resolver"]`. |
| `servers` | `[]DNSServer` | Энтрии с `kind` discriminator (см. ниже). |
| `rules` | `[]DNSRule` | Энтрии с `kind` discriminator. |

**`servers[i]` — `v6.DNSServer` (SPEC 056-R-N):**

| Поле | Тип | Описание |
|------|-----|----------|
| `kind` | `DNSServerKind` | `template` \| `preset` \| `user`. |
| `tag` | string | Для `kind=template` (lookup ключ в `template.dns_options.servers[tag]`) и `kind=user` (display tag в финальном `config.dns.servers[].tag`). Пуст для `preset`. |
| `ref` | string | Только для `kind=preset`, формат `"<preset_id>:<local_tag>"`. Пуст для остальных. |
| `enabled` | bool | Toggle. Build pipeline пропускает entry если `false`. |
| `body` | `map[string]interface{}` | Только для `kind=user` — полные DNS-server поля (type / server / server_port / tls / detour / ...). Для `template` / `preset` — nil (body резолвится из template). |

**`rules[i]` — `v6.DNSRule` (SPEC 056-R-N):**

| Поле | Тип | Описание |
|------|-----|----------|
| `kind` | `DNSRuleKind` | `preset` \| `user`. |
| `ref` | string | Только для `kind=preset`, формат `"<preset_id>"` (один dns_rule на preset). |
| `enabled` | bool | Toggle. |
| `body` | `map[string]interface{}` | Только для `kind=user` — полное sing-box dns rule body (rule_set / server / domain_* / ip_cidr / port / network / ...). nil для preset. |

**Удалено:**
- `independent_cache` — deprecated в sing-box 1.14.0 (cache всегда per-transport). Legacy state с этим ключом парсится без ошибок (unknown field ignored), новые saves не пишут.
- `extra_servers[]`, `extra_rules[]`, `template_servers` map — старая dev-схема SPEC 053, заменена flat-list'ом с kind discriminator (SPEC 056-R-N).

**JSON example — полный `dns_options` блок:**
```jsonc
{
  // strategy/final/default_domain_resolver — fallback дубль; source of truth в vars[]
  // (на диск пишутся только если не zero)

  "servers": [
    // template: ссылка на template.dns_options.servers[tag="cloudflare_udp"]
    { "kind": "template", "tag": "cloudflare_udp", "enabled": true },

    // template: required entry от template'а; локально disabled юзером
    { "kind": "template", "tag": "google_doh", "enabled": false },

    // preset: bundled от russian preset, local_tag="yandex_doh"
    { "kind": "preset", "ref": "russian:yandex_doh", "enabled": true },

    // user: полностью user-defined DNS-сервер с inline body
    {
      "kind": "user",
      "tag": "my-pihole",
      "enabled": true,
      "body": { "type": "udp", "server": "192.168.1.10", "server_port": 53 }
    }
  ],

  "rules": [
    // preset: один dns_rule на preset, тело резолвится из template
    { "kind": "preset", "ref": "russian", "enabled": true },

    // user: полное sing-box dns rule body
    {
      "kind": "user",
      "enabled": true,
      "body": {
        "rule_set": "ru-domains",
        "server": "yandex_doh"
      }
    }
  ]
}
```

---

## 4. Per-block storage rules

| Секция | Содержит | Источник истины | Кто пишет | Кто читает |
|--------|----------|-----------------|-----------|------------|
| `connections.sources` | Source entries (subscription URL или server URI), per-source meta (profile_title, userinfo, last_status), update spec | state | UI Sources tab (`source_tab`), Update flow (после fetch) | parser pipeline, UI dashboard, build |
| `connections.outbounds` | Global selectors/urltest entries, в т.ч. preset-bound (`ref`) и preset-patched (`updates[]`) | state | UI Outbounds tab, `SyncOutboundsWithActivePresets`, presenter `CreateStateFromModel` | build (`ResolveOutbounds` + `MergeOutboundUpdatesInPlace`), UI render |
| `connections.defaults` | reload interval, max_nodes per source default | state | UI Settings/Sources | parser pipeline |
| `rules` | Routing rules через kind discriminator (preset/inline/srs) — единый упорядоченный массив | state | UI Rules tab (drag, library add, edit) | build (`MergeRouteSection` + `MergePresetsIntoRoute`), UI render |
| `vars` | Overrides для всех объявленных в template vars: tun, route_final, dns_*, clash_secret, etc. | state (значения) + template (объявления) | UI Settings tab, скрытые синхронизаторы (`SyncDNSModelToSettingsVars`) | build (`@var` substitute) |
| `dns_options.servers` | Entries kind=template / preset / user; body для template/preset резолвится из template, для user — flat в entry | state (что включено) + template (тело) | UI DNS tab, `SyncDNSOptionsWithActivePresets`, presenter | build (`ResolveDNS` → `MergeDNSSection`), UI render |
| `dns_options.rules` | Entries kind=preset / user. preset = thin ref на `template.presets[].dns_rule`, user = flat body | state + template | UI DNS tab, lifecycle sync, presenter | build (`ResolveDNS`), UI render |

«Источник истины» = откуда берётся семантика записи. «Кто пишет» = в каких
точках кода mutates state. «Кто читает» = consumers при build/render.

---

## 5. Outbound preset binding lifecycle (SPEC 057-R-N)

Outbound entries в `connections.outbounds[]` могут быть тремя типами:
обычный global (template или user), **preset-add** (entry создан preset'ом),
и **target of preset-update** (на entry накладывается стек patches).

### 4.1 Schema на `OutboundConfig`

| Поле | Тип | Семантика |
|------|-----|-----------|
| `ref` | `string` (omitempty) | Не пусто → entry создан через `preset.outbounds[mode=add]`. Значение = `preset.id` владельца. UI: row read-only (View вместо Edit, без Del). На disable preset → entry удаляется. |
| `updates` | `[]OutboundUpdate` (omitempty) | Стек patches от `preset.outbounds[mode=update]` от разных preset'ов. Каждая запись — `{ref: <preset_id>, patch: <fields>}`. Финальное body на emit = base + apply patches в order. |
| `required` | (не в struct) | Template-only флаг, читается live из `template.parser_config.outbounds[]` на render. State.json не персистит — иначе нельзя было бы снять флаг в template и увидеть эффект. |

`OutboundUpdate{Ref, Patch map[string]interface{}}` определён рядом
(`core/config/configtypes/types.go`).

### 4.2 Lifecycle: `SyncOutboundsWithActivePresets`

Единая точка добавления/удаления preset entries. Idempotent.

Вызывается:
- На Load после `parseV6` (presenter `LoadState`)
- На каждый preset toggle в Rules tab (через `RefreshAfterPresetToggle`)
- Перед Save в `CreateStateFromModel` — на **обе view'а**
  (`state.Connections.Outbounds` и `state.ParserConfig.ParserConfig.Outbounds`),
  потому что `syncConnectionsFromLegacy` копирует legacy view → canonical,
  иначе sync'нутые `updates[]` затирались бы адаптером
- В headless runtime path'ах: `rebuild_raw_cache`,
  `UpdateConfigFromSubscriptions`, `parseAndPreview`

Семантика: для каждого активного `state.rules[kind=preset, enabled=true]`
прогоняет `preset.outbounds[]`; mode=add → ensure entry с `ref=preset.id`,
mode=update → ensure `OutboundUpdate{ref, patch}` в `updates[]` target'а.
Entries и patches с ref на disabled/missing preset удаляются.

### 4.3 Adopt-on-first-sync (legacy state)

При первом sync на состоянии pre-SPEC-057 (или после ручного promote-to-global)
existing global без `Ref` с tag, совпадающим с expected preset add — adopt'ится:
ему проставляется `Ref = preset.id`, body preservируется. Без этого
шиппнутые юзеры теряли бы preset binding на апгрейде.

### 4.4 Runtime merge: `MergeOutboundUpdatesInPlace`

Native generator (`GenerateOutboundsFromParserConfig`) не знает про
поле `Updates`. Перед его вызовом build pipeline вызывает
`MergeOutboundUpdatesInPlace(parserCfg)` — walks `parserCfg.Outbounds[]`,
для каждой entry с непустым `Updates[]` стеком заменяет body на merged
(base + apply patches в order). Mutates in-place (через deep-copy на
сайт-edge, model не trash'ится).

Места вызова — те же 3 headless path'а из 4.2 (см. `core/build/resolve_outbounds.go`
комментарии). UI-preview flow разделяет unmerged (для model) и merged
(`parserConfigForGen`) — Save пишет правильный state shape с `updates[]`,
generator получает flat'нутую копию.

---

## 6. DNS preset binding lifecycle (SPEC 056-R-N)

Симметрично outbound binding. `dns_options.servers[]` и `dns_options.rules[]`
— flat array с `kind` discriminator.

### 5.1 `dns_options.servers[]` — kind

| `kind` | Identity | Body |
|--------|----------|------|
| `template` | `tag` (ссылка в `template.dns_options.servers[tag]`) | резолвится из template на build/render |
| `preset` | `ref = "<preset_id>:<local_tag>"` (ссылка на `template.presets[id].dns_servers[local_tag]` + `vars` substitute) | резолвится из template + apply preset vars |
| `user` | `tag` + flat body (type/server/server_port/tls/...) — полная sing-box DNS server spec | self-contained |

Toggle `enabled` доступен для всех трёх kind'ов; edit body — только для user;
delete — только для user (template/preset управляются template'ом и preset
toggle'ом).

### 5.2 `dns_options.rules[]` — kind

| `kind` | Identity | Body |
|--------|----------|------|
| `preset` | `ref = "<preset_id>"` (один dns_rule на preset максимум) | резолвится из `template.presets[id].dns_rule` |
| `user` | flat body (rule_set/server/domain_*/ip_cidr/port/network/...) | self-contained |

### 5.3 Lifecycle: `SyncDNSOptionsWithActivePresets`

Единая точка lifecycle для kind=preset entries. Аналогично outbound sync.

Вызывается из presenter'а на тех же триггерах: Load, preset toggle, перед Save.
Семантика: enable preset → создаются entries `{kind:preset, ref}` для каждого
`template.presets[id].dns_servers[]` + (если есть) `dns_rule`. Default
`Enabled=true`. Disable preset → все entries с ref на этот preset удаляются.
Per-server toggle внутри активного preset (юзер может скрыть отдельный
сервер из bundle) преserve'ится при sync.

Реализация: `core/state/v6/sync_dns.go::SyncDNSOptionsWithActivePresets`.

### 5.4 Required entries (template)

`template.dns_options.servers[]` может пометить entry как `"required": true`
(например, `local_dns_resolver` / `direct_dns_resolver`). Render показывает
галку enabled + lock на toggle/edit/del; build всегда эмитит. Флаг — template-only,
state не персистит — читается live на каждый render через
`wizardbusiness.DNSTagLocked(model, tag)`.

### 5.5 Удалённые поля (sing-box 1.14)

`independent_cache` — deprecated в sing-box 1.14 (кэш теперь всегда
per-transport). Legacy state c этим ключом парсится без ошибок (silently
dropped через `_ = raw.IndependentCache` в `legacyDevDNSToOptions`),
новые saves поле не пишут.

---

## 7. Rule preset binding lifecycle (SPEC 053)

`rules[]` — единый упорядоченный массив через `kind` discriminator.

| `kind` | Header | Body |
|--------|--------|------|
| `preset` | `{ref, enabled}` (ref = `<preset_id>`) | `{vars: {<name>: <value>, ...}}` — только diff от template defaults; пустой map = всё дефолтное |
| `inline` | `{id (ULID), enabled}` | `{name, match (sing-box match-объект), outbound (tag|"reject"|"drop")}` |
| `srs` | `{id (ULID), enabled}` | `{name, srs_url, outbound}` |

Order = order рендера в UI Rules tab (включая drag-reordering) = order эмита
в `config.json::route.rules[]`. Сохраняется через
`SyncRulesByOrderToStateRulesV6(model.RuleOrder, ...)` в `CreateStateFromModel`.

Match-поля и rule_set'ы для kind=preset живут **в template** — bump
`RequiredTemplateRef` → юзеры автоматически получают новые match-поля.
Body хранит только diff vars; пустой `vars: {}` = preset на template defaults.

См. `core/state/v6/rule_types.go` (DecodeBody dispatcher) +
`core/build/preset_expand.go` (build-time substitute + tag-prefix).

---

## 8. Data flow

### 7.1 Load: `state.json` → model

```
disk: bin/wizard_states/state.json
        │
        ▼
core/state.Load(path)
        │   probe meta.version  →  parseV6 (или parseV5 / parseLegacy)
        │   legacyDevDNSToOptions if старый dev-shape `dns.{template_servers,extras}`
        ▼
state.State{Connections, RulesV6, DNS, Vars, ...}
        │
        ▼
presenter.LoadState(stateFile)
        │   restoreParserConfig (legacy view)
        │   restoreConfigParams + restoreDNS
        │   ApplyRulesLibraryMigration (legacy v3→v5 idempotent)
        │   restoreCustomRules + restorePresetRefs (kind=preset)
        │   SyncOutboundsWithActivePresets(model.GlobalOutbounds)   ← adopt-on-first-sync
        │   RefreshDerivedParserConfig
        ▼
model.WizardModel  (Sources, GlobalOutbounds, CustomRules, PresetRefs,
                    DNSServers, DNSRulesText, SettingsVars, RuleOrder)
        │
        ▼
SyncModelToGUI + RefreshOutboundOptions
```

### 7.2 Save: model → `state.json`

```
model.WizardModel
        │
        ▼
presenter.CreateStateFromModel(comment, id)
        │   SyncGUIToModel
        │   build WizardStateFile (legacy view + Connections canonical)
        │   ReconcileRuleOrder + SyncRulesByOrderToStateRulesV6  → state.RulesV6
        │   SyncDNSFullToStateV6                                  → state.DNS
        │   v6.SyncDNSOptionsWithActivePresets(state.RulesV6, &state.DNS, presets)
        │   applyPresetEnabledOverrides (UI toggle → entry.Enabled)
        │   build.SyncOutboundsWithActivePresets ×2 view (Connections + ParserConfig)  ◄── обязательно на обе!
        ▼
state.State.Save(path)
        │   syncConnectionsFromLegacy (ParserConfig → Connections; уже sync'нутая версия побеждает)
        │   hasPresetRefs(RulesV6) ? marshalDiskV6 : marshalDisk (v5)
        │   atomic write (.tmp + Rename) + fsync
        ▼
disk: bin/wizard_states/state.json
```

Двойной sync на обе view (`Connections.Outbounds` + `ParserConfig.Outbounds`)
— ключевой момент: без него `syncConnectionsFromLegacy` затирал бы только что
вычисленные `updates[]` стеки.

### 7.3 Build/Emit: state → `bin/config.json`

```
state.State (после Load или после CreateStateFromModel)
        │
        ▼
core/build (entry: BuildAndWriteConfig / ApplyTemplate)
        │   ResolveDNS(state, template, vars)        ◄── pure func
        │   ResolveRoute(state, template, vars)      ◄── pure func
        │   ResolveOutbounds(state, template)        ◄── pure func
        │   MergeOutboundUpdatesInPlace(parserCfg)   ◄── материализует Updates[] в body для generator'а
        │   GenerateOutboundsFromParserConfig
        │   MergeDNSSection + MergeRouteSection
        │   MergePresetsIntoRoute (per-preset expand: substitute + tag prefix)
        ▼
disk: bin/config.json (sing-box-compatible)
```

Resolve* функции — single source of truth для UI и build (нет divergence
между preview и финальным config).

---

## 9. Required vs preset-locked entries

Три класса entries в UI с разной семантикой управления:

| Класс | Где маркер | Толкование | UI controls |
|-------|------------|------------|-------------|
| **Required (template)** | `template.*.entries[].required = true` (live read, в state не персистится) | Mandatory entry — нельзя toggle/del. Body editable. | Reset (откат body к template defaults), Edit. **Del не рендерится.** |
| **Preset-locked** | `entry.ref != ""` (или `kind=preset` для DNS/rules) | Entry создан preset'ом, body резолвится из template + preset vars. | Toggle enabled (юзер может скрыть отдельный bundle item), View (read-only modal). **Edit / Del не рендерятся.** |
| **User / template global** | `ref == ""` + tag отсутствует в required set | Полный контроль. | Toggle, Edit, Up/Down, Del |

«Required» — про **lock на удаление и toggle**; «preset-locked» — про
**lock на edit body** (управляется через preset toggle в Rules tab).

---

## 10. Миграции

| From → To | Что мигрирует | Backup |
|-----------|---------------|--------|
| v2/v3/v4 → v5 | `selectable_rule_states` + `custom_rules` → единый `custom_rules[]` (rules library merge); `parser_config` wrapped → simplified; `enable_tun_macos` → `vars["tun"]`; `route.default_domain_resolver` → `vars["dns_default_domain_resolver"]` | нет (in-memory; пишется v5 при первом Save) |
| v5 → v6 | `custom_rules[]` → `rules[]` (kind=inline/srs derive из rule_set type); `dns_options.servers/rules` legacy → `dns_options.servers/rules` flat kind discriminator; meta bump | **`state.json.v5.bak`** на первом upgrade (когда появляется хотя бы один kind=preset rule) |
| v6 dev-shape → v6 flat | `dns.{template_servers, extra_servers, extra_rules}` (SPEC 053 промежуточный shape) → `dns_options.servers[]/rules[]` flat (SPEC 056-R-N) | нет (lossless, dev-only, не релизился) |
| sing-box 1.14 | `dns_options.independent_cache` silently dropped (legacy state читается, новый не пишется) | нет |

Save выбирает формат автоматически: `hasPresetRefs(RulesV6)` → v6, иначе v5.
Юзеры с pure inline/srs rules остаются на v5 пока не добавят первый preset.

---

## 11. Где лежит реализация

| Файл | Что |
|------|-----|
| `core/state/load.go` | `Load` / `Parse` / `parseV6` / `parseV5` / `parseLegacyAndMigrate` / `legacyDevDNSToOptions` |
| `core/state/save.go` | `Save` / `marshalDisk` (v5) / `marshalDiskV6` / `maybeBackupV5` |
| `core/state/adapter.go` | `syncConnectionsFromLegacy` / `syncLegacyFromConnections` (обмен legacy ParserConfig ↔ canonical Connections) |
| `core/state/v6/state.go` | v6 State struct + MetaSection |
| `core/state/v6/rule_types.go` | Rule + PresetBody/InlineBody/SrsBody + DecodeBody |
| `core/state/v6/dns_options.go` | DNSServer + DNSRule + flat Marshal/Unmarshal |
| `core/state/v6/sync_dns.go` | `SyncDNSOptionsWithActivePresets` |
| `core/state/v6/migration.go` | `MigrateV5ToV6` (pure func) |
| `core/build/sync_outbounds.go` | `SyncOutboundsWithActivePresets` (lifecycle) + `outboundConfigToPatchMap` |
| `core/build/resolve_outbounds.go` | `ResolveOutbounds` + `MergeOutboundUpdatesInPlace` (runtime helper) |
| `core/build/resolve_dns.go` | `ResolveDNS` (pure DNS view для UI + build) |
| `core/build/resolve_route.go` | `ResolveRoute` (pure route view) |
| `core/template/loader.go` | `LoadTemplateData` + `TemplateData` struct |
| `core/template/preset_types.go` | Preset / PresetVar / PresetRuleSet / PresetDNSServer / PresetOutbound |
| `ui/configurator/presentation/presenter_state.go` | `LoadState` + `CreateStateFromModel` (entry points для save/load) |
| `ui/configurator/presentation/presenter_sync.go` | `RefreshAfterPresetToggle` (presenter-level eager sync после Rules toggle) |

См. также: [TEMPLATE_REFERENCE.md](TEMPLATE_REFERENCE.md) — что лежит в
`wizard_template.json` и куда оно попадает в state/runtime/UI.
[DATA_FLOW.md](DATA_FLOW.md) — расширенные диаграммы load/save/build/toggle.
[CREATE_WIZARD_TEMPLATE.md](CREATE_WIZARD_TEMPLATE.md) — туториал для авторов
template'ов (формат preset'ов, vars, substitute, if/if_or).
