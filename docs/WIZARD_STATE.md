# Wizard state (state.json)

Формат файла состояния визарда конфигурации и логика загрузки/сохранения.

## Назначение

Файл `state.json` (и именованные состояния `<id>.json`) хранит полное состояние визарда: выбранные источники прокси, outbounds, правила маршрутизации (в т.ч. пользовательские), параметры конфигурации. При открытии визарда состояние загружается из текущего файла; при сохранении — записывается обратно.

## Версии формата

| meta.version | Schema name | Статус | Где |
|---|---|---|---|
| 2-4 | `version: N` (top-level) | Legacy | parser auto-migrate на Load |
| **5** | top-level `meta.version: 5` | **Текущий релизный** | shipped в `v0.9.5` |
| **6** | `meta.version: 6` + `meta.schema: "presets_v1"` | **Dev** (HEAD-of-develop) | SPEC 053 + SPEC 056-R-N — добавился preset bundles + flat DNS schema |

**Save logic (SPEC 053):** если `state.RulesV6` содержит хоть один `kind=preset`,
пишем v6; иначе v5. v6 → v5 backup создаётся при первом upgrade
(`state.json.v5.bak`). v6 → v6 in-place dev rewrite (SPEC 056-R-N) — backup
конверсия дев-формата на лету (read-старого-shape, write-нового на Save; backup не делаем — v6 не релизился, lossless).

---

## v6 layout (SPEC 053 + SPEC 056-R-N)

Top-level:

```json
{
  "meta":        { "version": 6, "schema": "presets_v1", "created_at": "...", "updated_at": "..." },
  "connections": { "sources": [...], "outbounds": [...], "defaults": {...} },
  "rules":       [ ... ],            // kind discriminator (preset/inline/srs)
  "vars":        [ { "name": "...", "value": "..." }, ... ],   // включая dns_*
  "dns_options": {                                              // SPEC 056-R-N
    "strategy":                "...",   // дублирует state.vars["dns_strategy"] для in-memory
    "final":                   "...",
    // independent_cache УДАЛЕНО — sing-box 1.14 deprecation (cache always per-transport).
    "default_domain_resolver": "...",
    "servers": [ { kind, ref|tag, enabled, ...body }, ... ],
    "rules":   [ { kind, ref|...,  enabled, ...body }, ... ]
  }
}
```

### `rules[]` — kind discriminator (SPEC 053)

| kind   | header fields                         | body schema                         |
|--------|---------------------------------------|--------------------------------------|
| preset | `{ref, enabled}` — ref = `<preset_id>` | `{vars: {<name>: <value>, ...}}` — только diff от template defaults |
| inline | `{id, enabled}`                       | `{name, match, outbound}` — sing-box match-keys + outbound tag |
| srs    | `{id, enabled}`                       | `{name, srs_url, outbound}` |

### `dns_options.servers[]` — SPEC 056-R-N

| kind     | header fields                          | resolve тела                          |
|----------|----------------------------------------|----------------------------------------|
| template | `{tag, enabled}`                       | `template.dns_options.servers[tag]`   |
| preset   | `{ref, enabled}` — ref = `<preset_id>:<local_tag>` | `template.presets[id].dns_servers[local_tag]` + Vars substitute |
| user     | `{tag, enabled, ...body}` flat         | self-contained body (type/server/...) |

### `dns_options.rules[]`

| kind   | header fields                | resolve тела                        |
|--------|------------------------------|--------------------------------------|
| preset | `{ref, enabled}` ref = `<preset_id>` (один dns_rule на preset) | `template.presets[id].dns_rule` |
| user   | `{enabled, ...body}` flat    | self-contained body                  |

### Lifecycle entries kind=preset (SPEC 056-R-N)

**Memory == disk** invariant: state.dns_options.servers[] + .rules[] всегда
содержит ровно тот набор `kind=preset` entries что соответствует активным
preset-ref'ам в state.rules[]. **`SyncDNSOptionsWithActivePresets`** —
единая точка lifecycle:

- **Enable preset** в Rules tab → создаются entries `{kind:preset, ref}` для
  каждого `template.presets[id].dns_servers[]` + если есть dns_rule → entry в
  rules. Default `Enabled=true`.
- **Disable preset** → **все** entries с этим ref удаляются.
- **Per-server toggle** внутри активного preset разрешён (юзер может скрыть
  отдельный сервер из bundle, ставя `Enabled=false` — value preservируется
  при sync). На disable preset все entries удаляются → re-enable = свежие
  дефолты.

### v6 → v6 in-place dev rewrite (SPEC 056-R-N)

v6 никогда не релизился — это HEAD-of-develop формат после SPEC 053.
SPEC 056-R-N переписывает схему в том же v6/`presets_v1` без bump'а:

- **Старый дев-shape (SPEC 053):** `dns.template_servers` (map) +
  `dns.extra_servers` (array) + `dns.extra_rules` (array).
- **Новый shape (SPEC 056-R-N):** `dns_options.servers[]` + `.rules[]`
  с kind discriminator.

`parseV6` детектит старый shape (через `legacyDevDNSToOptions` fallback)
и конвертит в memory в новый shape. На ближайшем `Save` файл перезаписывается
в новом layout'е. Backup не создаётся — конверсия lossless, v6 не релизился.

### Unified resolver pattern (SPEC 056-R-N Phase B)

Build pipeline и UI render оба consume единую pure-func `ResolveDNS()`
(аналогично `ResolveRoute()`) — нет divergence между preview и финальным
config.

```
state + template + vars
        │
        ▼
   ResolveDNS() / ResolveRoute()  ◄── pure, single source of truth
        │
   ┌────┴────┐
   ▼         ▼
[UI render] [build emit]
показ всех   эмит где Active && Enabled
с badges
```

`ResolvedDNSServer{Kind, Tag, LocalTag, Body, Source, PresetID, PresetLabel,
Active, Enabled, Locked, InactiveReason}` несёт всю информацию для
обоих consumers. Build emit фильтрует `Active && Enabled && !Source=Core`;
UI рендерит **все** entries с visual hint'ом для `!Active` (disabled checkbox
+ tooltip с InactiveReason).

См. `core/build/resolve_dns.go` и `core/build/resolve_route.go`.

### Template DNS unify через `required: true` (SPEC 056-R-N Phase C)

`template.config.dns.servers[]` теперь **пустой массив**. Все DNS-серверы
(включая mandatory `local_dns_resolver` / `direct_dns_resolver`) живут в
**одном** `template.dns_options.servers[]`. Mandatory entries маркируются
полем `required: true` — UI блокирует toggle/edit/del, в config.json всегда
эмитятся.

```json
"dns_options": {
  "servers": [
    {"tag":"local_dns_resolver",  "type":"local",   "required":true, "enabled":true},
    {"tag":"direct_dns_resolver", "type":"udp", ..., "required":true, "enabled":true},
    {"tag":"cloudflare_udp", ...}
  ]
}
```

UI `wizardbusiness.DNSTagLocked(model, tag)` живо смотрит в template'е
без кеширования в `model.DNSLockedTags` (удалено как dead channel).

### Outbound `required: true` (SPEC 056-R-N Phase E)

Аналогично для template-mandated outbound'ов (`proxy-out` и т.п.):
```json
"parser_config": {
  "outbounds": [
    {"tag":"proxy-out", "required":true, "type":"selector", ...}
  ]
}
```

`OutboundConfig` **не имеет** `Required` field в struct — флаг живёт ТОЛЬКО
в template, читается live через `templateRequiredTags(model)` на каждый
UI render. State.json не персистит `required` (иначе нельзя было бы снять
флаг в template и увидеть эффект).

UI для required outbound: Up/Down ✅, Edit ✅, **Reset 🔄** (откат body к
template defaults), Del не рендерится.

### Preset bundled outbounds в Outbounds tab (SPEC 057-R-N — supersedes 056-R-N Phase E)

`preset.outbounds[]` теперь живут **в state** через два новых поля на `OutboundConfig`:

- **`Ref string`** (`json:"ref,omitempty"`) — entry создан через `preset.outbounds[mode=add]`. Значение = `preset.id` владельца. UI: row read-only (View вместо Edit, без Del), 🔒 + preset label.
- **`Updates []OutboundUpdate`** (`json:"updates,omitempty"`) — стек patches от `preset.outbounds[mode=update]`. Каждая запись `{Ref string, Patch map[string]interface{}}`. На emit финальное body = base + apply patches в order.

```json
"outbounds": [
  {
    "tag": "proxy-out", "type": "selector",
    "options": {...}, "addOutbounds": [...],
    "updates": [
      {"ref": "russian",   "patch": {"filters": {"tag": "!/(🇷🇺)/i"}}},
      {"ref": "ru-inside", "patch": {"filters": {"tag": "!/(🇷🇺)/i"}}}
    ]
  },
  {
    "tag": "ru VPN 🇷🇺", "type": "selector",
    "options": {...}, "filters": {...}, "addOutbounds": [...],
    "ref": "russian"
  }
]
```

Lifecycle через `SyncOutboundsWithActivePresets` (idempotent): вызывается на Load, preset toggle (`RefreshAfterPresetToggle`), перед Save (`CreateStateFromModel` — на обе view'а: Connections + ParserConfig), в headless runtime path'ах (rebuild_raw_cache, UpdateConfigFromSubscriptions, parseAndPreview). Adopt-on-first-sync: existing global без `Ref` с tag совпадающим с preset.add → entry adopt'ится (для legacy state).

Runtime emit: `MergeOutboundUpdatesInPlace(parserCfg)` флэттенит `Updates[]` стек в body перед `GenerateOutboundsFromParserConfig` (generator не знает про Updates).

### Disabled subscription cascade

Toggle off подписки в Sources tab → её per-source outbound'ы (BL:auto/select
и т.д.) исчезают из 4 точек:
- Outbounds tab UI (`collectRows`, `collectAllTags`)
- Rules tab outbound dropdowns (`business.GetAvailableOutbounds`)
- Preview cache (`RebuildPreviewCache`)
- Финальный config.json (`GenerateOutboundsFromParserConfig` — reference)

Все 4 точки симметричны.

---

## v5 layout (shipped)


## Резюме по блокам (чтение)

Ниже — **кто главный** при восстановлении модели. «Шаблон» = актуальный **`bin/wizard_template.json`** после **`LoadTemplateData`**. **State** = загруженный снимок (`state.json` или `<id>.json`). Порядок вызовов при **`LoadState`** — в разделе **«Поток чтения»**.

| Блок | Резюме при **`LoadState`** (есть state) | Резюме **без** state (первый запуск / Read → New) |
|------|----------------------------------------|---------------------------------------------------|
| **Шаблон целиком** | Всегда читается **до** state: каркас `config`, дефолты DNS/selectable, сырой `dns_options` шаблона, `DefaultFinal` и т.д. State **не** заменяет шаблон целиком — по полям правила разные (строки таблицы ниже). | Тот же шаблон; парсер может прийти из **`config.json`**, если там есть валидный `@ParserConfig`. |
| **`parser_config`** | **Только state.** Шаблонный парсер на этом шаге **не** подмешивается. | **`config.json`** (приоритет) или **шаблон**. |
| **`config_params`** (`route.final`, …) | **State**; если параметра нет — **`DefaultFinal`** из **шаблона**. Устаревший **`enable_tun_macos`** при загрузке мигрирует в **`vars.tun`** (см. **`vars`**). `route.default_domain_resolver` здесь не норма (одноразовая миграция → см. DNS). | Обычно нет файла state → final задаётся из шаблона / **`EnsureFinalSelected`** после инициализации **`custom_rules`**. |
| **`vars`** | **State**: переопределения переменных шаблона (вкладка **Settings**), пары **`name`** / **`value`**. Элементы шаблона **`{"separator": true}`** не имеют **`name`** и **не** сериализуются в **`state.vars`**. Сироты (имена не из текущего шаблона) при **LoadState** не попадают в модель; при **Save** в файл уходят только имена из объявленных переменных шаблона (без разделителей). Повтор **`name`** в массиве JSON — при загрузке побеждает **последняя** запись. После **`restoreConfigParams`** для плейсхолдера **`clash_secret`** вызывается **`MaterializeClashSecretIfNeeded`**: сгенерированный секрет один раз попадает в **`SettingsVars`**, чтобы превью/DNS не меняли значение на каждом обновлении (пока пользователь не нажмёт **Сброс** для этого поля). | Нет ключа → дефолты из **`wizard_template.json`** (`vars[].default_value` / `default_node`). |
| **`dns_options`** | Из **state** в модель попадают **только** **`servers`** и **`rules`**; скаляры вкладки DNS (**strategy**, **cache**, **final**, **default domain resolver**) — в **`state.vars`** как **`dns_*`** (см. **`SUB_SPEC_DNS_TAB_VARS`**). Затем **`ApplyWizardDNSTemplate`** + **`ApplyDNSVarsFromSettingsToModel`**. | Нет снимка → **`ApplyWizardDNSTemplate`** + **`ApplyDNSVarsFromSettingsToModel`** из **шаблона** (если список DNS в модели ещё пуст). |
| **`selectable_rule_states`** | **Только формат до `rules_library_merged` (версия 2 без флага):** до **`LoadState`** миграция **`ApplyRulesLibraryMigration`** переносит записи в начало **`custom_rules`** и очищает selectable. В сохранённом файле **3** ключа обычно нет. | Не используется: первый запуск без state — **`InitializeTemplateState`** засевает **`custom_rules`** из пресетов шаблона с **`default: true`**. |
| **`custom_rules`** | **Единственный список правил маршрута** в модели после миграции: полные объекты, порядок = порядок в `route.rules` при генерации. | См. **`selectable_rule_states`** / засев из шаблона. |

**Итог одной фразой:** для **парсера** и **`custom_rules`** при **`LoadState`** приоритет у **state** (после однократной миграции library selectable→custom); пресеты **`selectable_rules`** в шаблоне — только **библиотека** для кнопки «Add from library», не отдельный слой в модели; для **DNS** — **`dns_options.servers`/`rules` из state**, скаляры из **`vars`**, **сшивка с шаблоном** (**`ApplyWizardDNSTemplate`**) и выравнивание полей UI из **`dns_*`** (**`ApplyDNSVarsFromSettingsToModel`**).

### Механизм переменных шаблона (`vars` и `@…`)

Это **отдельный** контур от вкладки DNS (следующий подраздел).

**Принцип:** в **`wizard_template.json`** в корне объявляется массив **`vars`** (имя, тип, дефолты, `wizard_ui`, …). Пользовательские переопределения попадают в **`state.json` → `vars`** как массив объектов **`{ "name", "value" }`** (строка **`value`**). При сборке эффективного конфига строковые литералы **`"@<name>"`** в разрешённых местах **`config`** и **`params`** заменяются на разрешённое значение (**SPECS/032**, **`docs/CREATE_WIZARD_TEMPLATE.md`**).

**UI:** автогенерируемые строки на вкладке **Settings** (плюс особые случаи вроде **`clash_secret`**). Метаданные переменной (**тип**, подписи) всегда из шаблона; в файле state — только пары **`name` / `value`**.

**DNS и `vars`:** скаляры вкладки **DNS** (**strategy**, **final**, **default domain resolver**) объявляются в шаблоне как скрытые переменные **`dns_*`** с литералами **`@dns_*`** в **`config`**; пользовательские значения — в **`state.vars`**. (Скаляр **`independent_cache`** удалён в связи с deprecation в sing-box 1.14.0.) Подробнее — **`SPECS/032-F-C-WIZARD_SETTINGS_TAB/SUB_SPEC_DNS_TAB_VARS.md`**.

#### `vars` и условия `params.if` / `params.if_or`

При сборке эффективного конфига из шаблона для каждого имени в **`if`** или **`if_or`** проверяется, входит ли текущая ОС в **`vars[].platforms`** (пустой список — на всех ОС; иначе совпадение с **`runtime.GOOS`**, без отдельной метки **`win7`** — Win7-сборка лаунчера это **windows/386**). Если **нет**, переменная для этого условия считается **ложной**, **даже если** в **`state.vars`** сохранено **`"true"`** (например, профиль перенесён с другой ОС). Подробнее — **docs/CREATE_WIZARD_TEMPLATE.md** (раздел про **`vars`** и **`if`**), **`ui/wizard/template/vars_resolve.go`**.

### Вкладка DNS: `dns_options` (servers/rules) и скаляры в `vars` (`dns_*`)

Вкладка **DNS** управляет списком серверов и **`rules`** через **`dns_options`** в **`state.json`** и **шаблон**; **strategy**, **`dns.final`**, **`route.default_domain_resolver`** — через скрытые переменные **`dns_*`** в **`wizard_template.json` → `vars`** и переопределения в **`state.vars`** (литералы **`@dns_*`** в **`config`**). Флаг **«резолвер не задан»** остаётся в модели (**`DefaultDomainResolverUnset`**), не кодируется одной пустой строкой в **`vars`**. (Раньше тут был ещё **`independent_cache`** — удалён в связи с deprecation в sing-box 1.14.0; legacy state с этим полем парсится без ошибок, новые saves поле не пишут.)

**Принцип работы:** **`LoadPersistedWizardDNS`** копирует только **`servers`** и **`rules`**; устаревшие ключи в старом **`dns_options`** при **`LoadState`** однократно мигрируют в **`state.vars`** (**`MigrateDNSScalarsFromPersistedToSettingsVars`**). Далее **`ApplyWizardDNSTemplate`**, затем **`ApplyDNSVarsFromSettingsToModel`**. Перед сохранением и сборкой конфига **`SyncDNSModelToSettingsVars`** синхронизирует модель → **`SettingsVars`**. Итоговый **`config.json`**: **`MergeDNSSection`** / **`MergeRouteSection`**.

### macOS: снятие `tun` в визарде (не про формат JSON)

К **`state.vars`** это не добавляет полей: речь о UI на вкладке **Settings**. Пока лаунчер считает ядро **запущенным** (**`RunningState`**, как кнопки Start/Stop на вкладке Core), переменную **`tun`** (**`name`** в шаблоне) **нельзя** переключить в off — показывается сообщение, галка остаётся включённой. После **Stop**, при переходе TUN off, при необходимости одним привилегированным **`rm -rf`** удаляются **`experimental.cache_file.path`** внутри **`bin/`** (если настроено в шаблоне и файл есть) и логи ядра **`logs/sing-box.log`** / **`logs/sing-box.log.old`** под **`ExecDir`**, если они существуют (см. подраздел **macOS: выключение `tun`** в **docs/CREATE_WIZARD_TEMPLATE.md** / **_RU.md**).

## Версия формата

- **`meta.version`** (внутри top-level объекта **`meta`**): целое число. Чтение поддерживает **`2`**, **`3`**, **`4`** (legacy top-level **`version`**) и **`5`** (новый layout с **`meta`** / **`connections`**); **новые сохранения** пишут **`5`** (**SPEC 052**). Старые версии auto-migrate в v5 при первом Load.
- **v5 (текущий)** — **SPEC 052 CONNECTIONS_REDESIGN**:
  - Top-level `meta { version, comment, created_at, updated_at }` вместо разбросанных полей.
  - Top-level `connections { sources, outbounds, defaults }` вместо `parser_config` обёртки.
  - `connections.sources[]` — first-class объект Source с дискриминатором `type=subscription|server`.
  - Per-source `meta { profile_title, userinfo, last_status, last_fetched_at, ... }` — заполняется при Update; используется UI (квота / expire / status badge).
  - `bin/subscriptions/<source.id>.raw` — per-source raw body cache (atomic .tmp+Rename); Rebuild парсит .raw напрямую без сети.
  - `bin/outbounds.cache.json` удалён (Phase 6 cleanup).
  - Top-level `id` удалён (snapshot-имена живут в имени файла `bin/wizard_states/<name>.json`).
  - `rules_library_merged` / `selectable_rule_states` не сериализуются в v5 (всегда `true` после миграции).
- **v4** — SPEC 032: модель переменных шаблона (`vars` массив, `@<name>` подстановки в `config`/`params`, `if`/`if_or` условия).
- **v3** — rules library (единый `custom_rules`, `rules_library_merged`).
- **v2** — самый старый поддерживаемый формат.
- **`rules_library_merged`** (legacy v3-v4): маршрут собирается **только** из **`custom_rules`**. В v5 не сериализуется (всегда true). Пресеты шаблона **`selectable_rules`** остаются **библиотекой** в UI («Add from library»), а не отдельным слоем в state. При первом открытии файла версии **2** без флага выполняется однократная миграция: содержимое **`selectable_rule_states`** сливается в начало **`custom_rules`**, затем state перезаписывается на диск (v5).

## Структура JSON (v5)

Корневой объект содержит:

| Поле | Тип | Описание |
|------|-----|----------|
| `meta` | object | Версия / comment / timestamps (обязательное в v5) — см. ниже |
| `connections` | object | `sources[]`, `outbounds[]`, `defaults{}` — first-class модель подключений (SPEC 052) |
| `config_params` | array | Параметры (в первую очередь **`route.final`**). Устаревшие `enable_tun_macos` / `route.default_domain_resolver` — однократная миграция в **`vars`**. |
| `vars` | array | Переопределения шаблонных переменных (Settings + скрытые `dns_*`). Объекты `{name, value}`. |
| `dns_options` | object | Состояние вкладки DNS визарда: `servers`, `rules` (только; скаляры — в `vars`). |
| `custom_rules` | array | Все правила маршрута, порядок = порядок в `route.rules`. |

### `meta`

| Поле | Тип | Описание |
|------|-----|----------|
| `version` | int | **`5`** в актуальных файлах. |
| `comment` | string | Опционально. |
| `created_at` | string | RFC3339 UTC. |
| `updated_at` | string | RFC3339 UTC. |

### `connections`

| Поле | Тип | Описание |
|------|-----|----------|
| `sources` | array | Список подключений. Каждый Source — `subscription` (URL → пачка нод) или `server` (один URI). |
| `outbounds` | array | Глобальные группы (`selector` / `urltest`) — отображаются в `outbounds[]` config.json. |
| `defaults` | object | `reload` (string, по умолчанию `4h`), `max_nodes` (int, default 3000). |

#### `connections.sources[i]`

| Поле | Тип | Описание |
|------|-----|----------|
| `id` | string | ULID (Crockford-base32, 26 символов). Стабильный — переживает Save/Load. Имя файла `bin/subscriptions/<id>.raw`. |
| `type` | string | `subscription` или `server`. |
| `enabled` | bool | Источник активен. |
| `label` | string | Опционально для server (человекочитаемое имя; для subscription заполняется из `meta.profile_title`). |
| `exclude_from_global` | bool | Исключить из global outbounds. |
| `url` | string | Только для type=subscription. |
| `skip` | array | Только для subscription: skip-rules (см. ParserConfig). |
| `tag` | object | `prefix`/`postfix`/`mask` (только для subscription). |
| `outbounds` | array | Local outbounds подписки (urltest/selector с тэгами `BL:auto` etc.). |
| `expose_group_tags_to_global` | bool | См. SPEC 026. |
| `update` | object | `interval_hours`, `auto_refresh` — per-source overrides defaults.reload. |
| `max_nodes` | int | Per-source override defaults.max_nodes. |
| `meta` | object | Runtime: profile_title, userinfo (квота), last_status, last_fetched_at, error_count, http_status_code, raw_body_bytes, nodes_count_fetched, truncated, preview_nodes[]. |
| `uri` | string | Только для type=server (vless://, vmess://, wireguard://, ...). |

Краткие резюме по ключам JSON (детали — в разделах ниже и в **«Резюме по блокам»**):

- **`parser_config`** — при `LoadState`: вся правда в этом объекте из файла.
- **`config_params`** — в т.ч. **`route.final`**; резолвер DNS сюда не кладём.
- **`vars`** — пользовательские значения для **`wizard_template.json`** → **`vars`** (Settings); TUN macOS — **`tun`**.
- **`dns_options`** — **servers**/**rules** вкладки DNS + сшивка с шаблоном; скаляры — **`vars`** (**`dns_*`**).
- **`selectable_rule_states`** — устаревший слой (v2); при отсутствии **`rules_library_merged`** сливается в **`custom_rules`** при загрузке.
- **`rules_library_merged`** — после **`true`** в файле и модели нет отдельного списка selectable-state; в **`custom_rules`** лежат все правила маршрута.
- **`custom_rules`** — полный список **пользовательских** правил маршрута; при генерации конфига **`MergeRouteSection`** дописывает включённые записи к **базовому** `route` из шаблона (статические `rules` / `rule_set` в шаблоне остаются первыми). Подробнее — **`docs/ARCHITECTURE.md`**, **`create_config.go`**.

## dns_options (объект в state.json)

> **Резюме (чтение):** в **новых** снимках в **`dns_options`** только **`servers`** и **`rules`**. Старые файлы могут содержать **`strategy`**, **`final`**, **`default_domain_resolver`**, **`default_domain_resolver_unset`** — они при **`LoadState`** мигрируют в **`state.vars`** (**`dns_*`**) и при следующем сохранении из **`dns_options`** исчезают. Ключ **`independent_cache`** (если есть в старых файлах) silently дропается — поле снято в связи с deprecation в sing-box 1.14.0. Далее **`ApplyWizardDNSTemplate`** + **`ApplyDNSVarsFromSettingsToModel`**.

Корневой ключ **`dns_options`** — снимок списка серверов и правил DNS визарда (то же имя, что у секции дефолтов в шаблоне). Правила — массив **`rules`**; в редакторе — построчный текст; при сохранении state текст **парсится** в **`rules`**. Ключ **`rules_text`** в старых `state.json` **не читается**.

| Поле | Тип | Описание |
|------|-----|----------|
| `servers` | array | Список объектов DNS-сервера (sing-box + **`description`**, **`enabled`** для визарда). |
| `rules` | array | Правила DNS (как `dns.rules` в sing-box). |
| *устаревшие* | | **`final`**, **`strategy`**, **`default_domain_resolver`**, **`default_domain_resolver_unset`** — читаются для миграции в **`vars`**, в новых сохранениях не пишутся. |
| *удалено* | | **`independent_cache`** — поле снято в связи с deprecation в sing-box 1.14.0. Legacy state с этим ключом парсится без ошибок (silently dropped). |

**`config_params`:** **`route.default_domain_resolver`** не используется как постоянное хранилище; старые файлы — одноразовый подхват в **`restoreDNS`**, если в **`vars`** ещё нет **`dns_default_domain_resolver`**.

Дефолты скаляров — из **`wizard_template.json`**: **`vars[].default_value`** для **`dns_*`** и литералы **`@dns_*`** в **`config`**. Секция шаблона **`dns_options`** содержит только **`servers`** и **`rules`** (плюс поля внутри объектов серверов).

**Порядок при `LoadState`:** **`restoreConfigParams`** → **`MigrateDNSScalarsFromPersistedToSettingsVars`** → при **`default_domain_resolver_unset`** в старом снимке выставляется флаг модели → **`LoadPersistedWizardDNS`** (только **servers**/**rules**) → при необходимости подхват резолвера из **`config_params`** → **`ApplyWizardDNSTemplate`** → **`ApplyDNSVarsFromSettingsToModel`**.

**`ApplyWizardDNSTemplate`** пересобирает список серверов (как раньше: скелет **`config.dns`**, шаблонный **`dns_options.servers`**, осиротевшие теги). Пустые **правила** и прочие поля, для которых в шаблоне **нет** объявлений **`dns_*`**, по-прежнему добираются из шаблона (**`fillDNSAuxiliaryIfEmpty`**).

### Поток DNS (шаблон → модель → state → config.json)

> **Резюме:** **`dns_options`** (**servers**/**rules**) + **`vars`** (**`dns_*`**) → модель → **`MergeDNSSection`** / **`MergeRouteSection`**.

1. **Шаблон** (`LoadTemplateData`): эффективный **`config`** (с подстановкой **`@dns_*`**), сырой **`dns_options`** (**servers**/**rules**).
2. **State:** миграция скаляров из старого **`dns_options`** в **`vars`**; загрузка **servers**/**rules**; **`ApplyWizardDNSTemplate`**; **`ApplyDNSVarsFromSettingsToModel`**.
3. **Модель** и **UI** — без смены сценариев вкладки DNS; **`SyncDNSModelToSettingsVars`** / **`SyncGUIToModel`** поддерживают **`state.vars`**.
4. **Сохранение state:** **`dns_options`** — только **servers** и **rules**; скаляры — в **`vars`**.
5. **Сборка `config.json`:** **`MergeDNSSection`** / **`MergeRouteSection`**. При первом запуске: **`initializeWizardContent`** вызывает **`ApplyWizardDNSTemplate`** и **`ApplyDNSVarsFromSettingsToModel`**. Спецификация: **024** + **SUB_SPEC_DNS_TAB_VARS**.

## `parser_config` и `config_params` (корень state.json)

> **Резюме (`parser_config`):** при **`LoadState`** в модель попадает **только** содержимое из файла state (**`restoreParserConfig`**). Шаблонный парсер на этом шаге **не** смешивается.

> **Резюме (`config_params`):** из state читаются **`route.final`** и остальные пары `name`/`value`; если **`route.final`** в state нет — **`DefaultFinal`** из шаблона. **`enable_tun_macos`** не используется как источник истины: до **`restoreConfigParams`** выполняется миграция в **`vars.tun`**. **`route.default_domain_resolver`** в `config_params` — устаревший дубль; подхватывается **один раз** в **`restoreDNS`**, если после **`dns_options`** резолвер в модели пуст и не режим unset.

Схема **`parser_config`** в JSON и миграции — **SPECS/002-F-C-WIZARD_STATE/WIZARD_STATE_JSON_SCHEMA.md**, **`WizardStateFile.UnmarshalJSON`**.

## `selectable_rule_states` (корень state.json)

> **Резюме (актуальный формат, `version` 3–4):** в норме **отсутствует**. Если файл ещё в старом виде (**`rules_library_merged`** ложь / отсутствует), **`ApplyRulesLibraryMigration`** (в **`LoadState`**, до **`restoreCustomRules`**) строит единый **`custom_rules`**: сначала правила из шаблона в порядке **`selectable_rules`** с учётом сохранённых **`enabled` / selected_outbound** по **`label`**, затем хвост прежних **`custom_rules`**; выставляет **`rules_library_merged`**, очищает **`selectable_rule_states`** в объекте, который уйдёт в **`restoreCustomRules`**.

> **Исторически (до миграции):** источник структуры — шаблон; в state были только **`label`**, **`enabled`**, **`selected_outbound`** по совпадению с **`TemplateData.SelectableRules`**.

## custom_rules (PersistedCustomRule)

> **Резюме (чтение):** при **`LoadState`** правила берутся **только** из массива `custom_rules` в файле state. Шаблон их не определяет и не накладывает. Миграции формата — при **`UnmarshalJSON`** (`MigrateCustomRules`, вывод `type` из `rule` при необходимости).

Каждый элемент — объект с полями:

| Поле | Тип | Описание |
|------|-----|----------|
| `label` | string | Название правила |
| `type` | string | Тип: только `ips`, `urls`, `processes`, `srs`, `raw` |
| `enabled` | bool | Включено ли правило |
| `selected_outbound` | string | Выбранный outbound |
| `description` | string | Описание (опционально) |
| `rule` | object | JSON объекта правила маршрутизации (ip_cidr, domain, rule_set и т.д.) |
| `default_outbound` | string | Outbound по умолчанию |
| `has_outbound` | bool | Есть ли outbound в правиле |
| `params` | object | Состояние UI по типу (опционально; в конфиг не попадает) |
| `rule_set` | array | Определения rule-set'ов для типа `srs` (опционально) |

### type — константы

В state и в коде используются только значения: `ips`, `urls`, `processes`, `srs`, `raw`. При загрузке, если `type` отсутствует или имеет старый формат (например `"Domains/URLs"`), тип выводится из содержимого `rule` функцией **DetermineRuleType(rule)**. При сохранении всегда записываются только эти константы.

### params

Объект для восстановления состояния интерфейса по типу правила:

- **processes:** `match_by_path` (bool), `path_mode` ("Simple"|"Regex") — переключатель «Match by path» и режим Simple/Regex.
- **urls:** `domain_regex` (bool) — состояние галочки «Regex».
- Типы `ips`, `srs`, `raw` могут не использовать params.

### rule_set (для типа srs)

Массив определений rule-set'ов в формате как в `bin/wizard_template.json`: элементы с полями `tag`, `type`, `format`, `url`. При загрузке восстанавливаются в `Rule.RuleSets`; при сохранении записываются из `Rule.RuleSets`.

## Поток чтения: `wizard_template.json`, текущий `state.json` и другой снимок

Ниже — как **собирается модель визарда** из шаблона и из файлов состояния. Код: `ui/wizard/wizard.go` (старт), `ui/wizard/presentation/presenter_state.go` (`LoadState`), `ui/wizard/business/loader.go` (`LoadConfigFromFile`), `ui/wizard/template/loader.go` (`LoadTemplateData`), `ui/wizard/business/state_store.go`, `ui/wizard/models/wizard_state_file.go` (`UnmarshalJSON`, миграции).

### 1. Шаблон всегда загружается первым

При открытии визарда **`LoadTemplateData(ExecDir)`** читает **`bin/wizard_template.json`** и заполняет **`model.TemplateData`**:

| Часть шаблона | Куда попадает | Примечание |
|---------------|----------------|------------|
| **`parser_config`** | `TemplateData.ParserConfig` (строка JSON с обёрткой `ParserConfig` для UI) | Используется, если нет state и нет валидного блока в `config.json` |
| **`config` + `params`** | После **`applyParams`** под текущий **GOOS** (и на darwin с учётом TUN — см. **`GetEffectiveConfig`**) → **`TemplateData.Config`** (секции по ключам), **`ConfigOrder`**, **`RawConfig`**, **`Params`** | Эффективный **`config.dns`** — скелет для DNS; **`route`** — для дефолтов и генерации |
| **`dns_options`** | **`TemplateData.DNSOptionsRaw`** (сырой JSON) | Дефолты вкладки DNS, не отдельный объект sing-box |
| **`selectable_rules`** | **`TemplateData.SelectableRules`** | После фильтра по **`platforms`** под текущую ОС |
| Агрегаты | **`DefaultFinal`**, **`DefaultDomainResolver`** | Извлекаются из `config.route` / `dns_options` шаблона в загрузчике |

Шаблон **не перезагружается** при смене снимка state: остаётся тот же файл в `ExecDir`. Имеет смысл держать шаблон актуальным; при несовпадении версии шаблона и старого state возможны пропуски правил (selectable без совпадения по `label`).

### 2. Старт визарда при **наличии** `state.json`

> **Резюме:** файл state → миграции при разборе JSON → **`LoadState`**: парсер и правила маршрута **из state** (после **`ApplyRulesLibraryMigration`** — только **`custom_rules`**); **config_params** из state (с fallback шаблона для final); DNS — **state + ApplyWizardDNSTemplate**.

1. **`StateStore.LoadCurrentState()`** читает **`bin/wizard_states/state.json`**. Десериализация в **`WizardStateFile`**: кастомный **`UnmarshalJSON`** (миграции **`MigrateSelectableRuleStates`**, **`MigrateCustomRules`**, упрощённый **`parser_config`**).
2. **`presenter.LoadState(stateFile)`** (порядок шагов в коде):
   - **`restoreParserConfig`** — **`parser_config` целиком из state** перезаписывает модель (`ParserConfig`, `ParserConfigJSON`); шаблонный парсер здесь не используется.
   - **`SourceURLs = ""`** — поле ввода URL только для добавления; список источников из **`ParserConfig.Proxies`**.
   - **`restoreConfigParams`** — из **`config_params`**: `route.final` → **`SelectedFinalOutbound`**; если `route.final` нет — **`DefaultFinal`** из шаблона. Из **`vars`** в state — в **`model.SettingsVars`** (TUN macOS — ключ **`tun`**). Затем **`MaterializeClashSecretIfNeeded`** — стабилизация автогенерации **`clash_secret`** в модели (см. строку таблицы про **`vars`**). **`route.default_domain_resolver` в `config_params`** на этом шаге не читается (только миграция в **`restoreDNS`**).
   - **`restoreDNS`** — см. раздел **dns_options** и **Поток DNS** выше: **`LoadPersistedWizardDNS`** (если в state есть **`dns_options`**) копирует в модель **весь** снимок DNS из файла; при необходимости подхват старого резолвера из **`config_params`**; затем **`ApplyWizardDNSTemplate`** (слияние списка серверов с **текущим** шаблоном + подстановка **пустых** полей из шаблона).
   - **`ApplyRulesLibraryMigration(stateFile, TemplateData, ExecDir)`** — если миграция library ещё не выполнена: объединение selectable+template order и существующих **`custom_rules`** в один список в **`stateFile.CustomRules`**, **`RulesLibraryMerged = true`**, **`SelectableRuleStates = nil`**.
   - **`model.RulesLibraryMerged`**, **`model.SelectableRuleStates = nil`**, затем **`restoreCustomRules(stateFile.CustomRules)`** — единственный источник правил маршрута в модели.
   - **`PreviewNeedsParse = true`**, **`SyncModelToGUI`**, **`RefreshOutboundOptions`**. Если миграция только что записала флаг merged — **`SaveWizardState`** текущего файла (идемпотентность при повторном открытии) и **`MarkAsSaved`**; иначе **`MarkAsSaved`**.

Итог: при **LoadState** источники правды — **state** для парсера, **config_params** для final, **`vars`** для настроек шаблона (в т.ч. TUN), **dns_options + шаблон** для DNS (см. DNS-раздел), **`custom_rules` (после миграции)** для маршрута.

### 3. Старт визарда **без** `state.json`

> **Резюме:** парсер из **`config.json`** или шаблона; правила маршрута и DNS — из **шаблона** (`InitializeTemplateState`, при пустом списке DNS — `ApplyWizardDNSTemplate`). **`LoadState` не вызывается.**

1. **`LoadConfigFromFile`** — приоритет **`config.json`**: извлекается блок **`@ParserConfig`**; иначе парсер из **шаблона**. Опционально **`EnsureRequiredOutbounds`**. В модель: **`ParserConfigJSON`**, **`SourceURLs`** (строка из источников в конфиге).
2. **`initializeWizardContent`** → **`InitializeTemplateState`**: **`SelectableRuleStates` всегда сбрасывается**; если **`!RulesLibraryMerged`** и **`CustomRules` пуст** — в **`CustomRules`** добавляются клоны пресетов **`selectable_rules`** с **`IsDefault`** (и SRS-проверкой), затем **`RulesLibraryMerged = true`**; для каждой записи — **`EnsureDefaultOutbound`**; **`EnsureFinalSelected`** для **`SelectedFinalOutbound`**.
3. Если **`len(DNSServers) == 0`** — **`ApplyWizardDNSTemplate`** (только шаблон, без предварительного **`LoadPersistedWizardDNS`**).

**`LoadState` не вызывается.**

### 4. Кнопка **Read** — текущий или **другой** снимок

> **Резюме:** тот же **`LoadState`**, что при старте с `state.json`. Именованный снимок перед этим **копируется** в `state.json`. **New** в диалоге = сценарий без state (п.3).

- Выбор **`state.json`** → **`LoadCurrentState()`** → тот же **`LoadState`**, что в п.2.
- Выбор **именованного** `<id>.json` → **`LoadWizardState(id)`**; при успехе снимок **копируется** в **`state.json`** (**`SaveCurrentState`**), затем **`LoadState`**. Логика восстановления модели **та же**, что при старте с текущим файлом.
- **New** в диалоге: без **`LoadState`** — снова **`LoadConfigFromFile`** + **`InitializeTemplateState`** + **`SyncModelToGUI`** (как «чистый» сценарий без сохранённого state).

### 5. Сводная таблица: что откуда при **`LoadState`**

> **Резюме:** дублирует таблицу **«Резюме по блокам»** в виде трёх колонок для быстрого сопоставления с кодом.

| Область | Основной источник | Роль шаблона |
|---------|-------------------|--------------|
| Парсер, источники, outbounds в JSON | **`parser_config` в state** | Не подмешивается при LoadState |
| Поле URL на Sources | Пустое; список из **Proxies** | — |
| **`route.final`** | **`config_params` state** | Fallback **`DefaultFinal`** шаблона, если параметра нет |
| **Переменные шаблона / TUN macOS** | **`vars` state** (в т.ч. `tun`) | Дефолты из **`wizard_template.json`** (`vars`) |
| Вкладка DNS | **`dns_options` state** + **`ApplyWizardDNSTemplate`** | Скелет **`config.dns`**, сырой **`dns_options`**, блокировки тегов |
| Правила маршрута (`custom_rules`) | **`custom_rules` state** (после миграции — единственный список) | Первый запуск: засев из **`selectable_rules`** с **`default: true`**; шаблон **`selectable_rules`** — библиотека для UI |

### 6. Десериализация файла state (до `LoadState`)

> **Резюме:** сырой JSON → **`WizardStateFile.UnmarshalJSON`** (миграции selectable/custom, форма `parser_config`) → затем п.2.

1. Чтение байтов с диска.
2. **`json.Unmarshal` → `WizardStateFile`**: миграции **`selectable_rule_states`** и **`custom_rules`**, нормализация **`parser_config`**.
3. Далее — **`LoadState`** по п.2.

Подробнее о схеме полей и v1→v2: **SPECS/002-F-C-WIZARD_STATE/WIZARD_STATE_JSON_SCHEMA.md**.

## Где хранится state

- **Текущее состояние:** `bin/wizard_states/state.json` (относительно ExecDir).
- **Именованные состояния:** `bin/wizard_states/<id>.json`.

Чтение/запись выполняет слой бизнес-логики (state_store); презентер создаёт состояние из модели (CreateStateFromModel) и восстанавливает модель из загруженного файла (LoadState).

## Миграции

- **v1 → v2:** `selectable_rule_states` и `custom_rules` приводятся к новому формату (см. WIZARD_STATE_JSON_SCHEMA.md). Поле `type` в custom_rules при загрузке может быть в старом виде — тогда тип выводится из `rule`.
- **v2 → v3 (rules library):** при **`LoadState`**, если **`rules_library_merged`** ещё не установлен, **`ApplyRulesLibraryMigration`** переносит selectable-слой в **`custom_rules`**, выставляет флаг и очищает **`selectable_rule_states`** в памяти; при успешной записи **`state.json`** повторная миграция не дублирует правила.
- **Корневой `version` 3 → 4:** **`4`** соответствует продуктовой линии с **vars** / **`@…`** / **`if`**/**`if_or`** (**032**); **`3`** — снимки только с rules library (переменные шаблона в шаблоне/state могли ещё не использоваться). Обязательного переписывания всех **`3`→`4`** нет. Новые сохранения пишут **`4`** (см. **«Версия формата»** выше).

См. также: **docs/ARCHITECTURE.md** (раздел про загрузку state), **SPECS/002-F-C-WIZARD_STATE/WIZARD_STATE_JSON_SCHEMA.md**. Краткая сводка приоритетов — раздел **«Резюме по блокам (чтение)»** в начале этого файла.
