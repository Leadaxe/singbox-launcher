# Карта: шаблоны конфига и хранение состояния в singbox-launcher

## 1. Механизм шаблонов (wizard_template.json)

**Файл:** `<execDir>/bin/wizard_template.json` — единственный шаблон для всех платформ.
- Константа: `internal/constants/constants.go:11` (`WizardTemplateFileName`), путь: `internal/platform/platform_common.go:112-117` (`GetWizardTemplatePath`).
- Шаблон скачивается с GitHub по pinned-коммиту `constants.RequiredTemplateRef` (`core/template/loader.go:215-221`, `GetTemplateURL`); инвалидация при апгрейде лаунчера — `core/template_migration.go` (`InvalidateTemplateIfStale`, маркер `LastTemplateLauncherVersion` в settings.json).

### Синтаксис шаблона
Это **не** голый sing-box config, а обёртка с 6 top-level секциями (парсинг: `core/template/loader.go:240-247`, `LoadTemplateData`):

| Секция | Содержимое |
|---|---|
| `parser_config` | конфиг парсера подписок: глобальные outbounds (selector/urltest группы, `required`, `addOutbounds`), `parser.reload` |
| `vars` | объявления переменных (`TemplateVar`, `core/template/vars_resolve.go:30-56`): `name`, `type` (text/bool/text_list/secret/outbound), `default_value` (скаляр или per-platform map `{"windows":…,"darwin":…,"default":…}`), `options` (строки или `{title,value}`), `wizard_ui`, `title`, `tooltip`, `if`/`if_or` (видимость в UI), `platforms`, `separator` |
| `config` | **полный sing-box config с плейсхолдерами** — секции log/certificate/dns/inbounds/endpoints/outbounds/route/experimental |
| `dns_options` | wizard-only дефолты вкладки DNS (не sing-box) |
| `presets` | параметризованные rule-бандлы (`Preset`, `core/template/preset_types.go:22+`): `id`, `label`, `default_enabled`, `platforms`, `vars` (локальный scope), `rule_set`, `rules[]`, `dns_rule`/`dns_rules`, `dns_servers`, `outbounds` (mode add/update). Локальные tag'и префиксуются `<preset_id>:<tag>` при сборке |
| `params` | платформозависимые патчи config'а (`TemplateParam`, loader.go:192-200): `name` (dot-notation, напр. `route.rules`), `platforms`, `value`, `mode` = replace/prepend/append, условия `if`/`if_or` по bool-переменным |

### Плейсхолдеры/маркеры
1. **`"@name"`** — строковый литерал в JSON заменяется значением переменной. Код: `core/template/substitute.go:56` (`SubstituteVarsInJSON`), walker `substituteWalkCtx` (:109-189). Типовые касты: int для `tun_mtu`, `mixed_listen_port`, `proxy_in_listen_port`, `urltest_tolerance` (:15-21); bool → true/false; `text_list` — одноэлементный массив `["@var"]` схлопывается в список строк (:166-177). Unresolved → `""` + warn (strict-вариант `SubstituteVarsInJSONStrict` для presets — пропуск preset'а целиком).
2. **`{"#if": {"and"/"or": [...], "value": ..., "else": ...}}`** — условная конструкция (SPEC 067): map-spread (поля ветки вливаются в родителя) и array-element (элемент включается/выкидывается). Предикаты: bare `"@boolvar"`, `{"@var": "literal"}`, `#notEmpty`/`#isEmpty`, `#in`/`#notIn`, `#matches`, `#not` (:241-538). Runtime-глобалы `@runtime.platform` / `@runtime.arch` / `@runtime.target` (local|remote) — из `TargetSpec` (:443-465), позволяют один шаблон на локальную машину и remote-роутер.
3. **`/** @ParserSTART */` … `/** @ParserEND */`** (outbounds) и **`@ParserSTART_E`/`@ParserEND_E`** (endpoints) — маркеры в итоговом config.json, между которыми вставляются сгенерированные из подписок ноды: `core/build/sections.go:66-144`.

### Пайплайн сборки config.json из шаблона + нод
1. `LoadTemplateData` (`core/template/loader.go:225`) → `TemplateData` (RawConfig, Params, Vars, Presets, Config/ConfigOrder — секции с сохранением порядка ключей).
2. Резолв переменных: template defaults → overrides из state (`State.Vars`) → `MaybeGenerateSecrets` (type:"secret", плейсхолдер `CHANGE_THIS_*` — `vars_resolve.go:24`). Применение params (`applyParamsFiltered`, loader.go:354; dot-notation `applyParam` :395) + подстановка `@vars` — всё в `ApplyTemplateWithVarsFor` (loader.go:339).
3. Парсер подписок генерирует per-node JSON (`core/config/outbound_generator.go`, `GenerateNodeJSON`) → `build.ParsedCache{Outbounds, Endpoints []json.RawMessage}` (`core/build/parsed_cache.go:19`).
4. **Единая точка сборки:** `core/build/build.go:163` `BuildConfig(BuildContext)` — per-section диспетчер (`buildSection` :254): `outbounds` → вставка нод из кэша между маркерами перед статическими (+ анти-DPI TLS-трансформации, + drop dangling detours); `dns` → `MergeDNSSection` + `MergePresetsIntoDNS`; `route` → `MergeRouteSection` (custom rules + rule_set) + `MergePresetsIntoRoute` + чистка dangling outbound-ссылок. Результат — текст config.json; запись на диск делает вызывающий (atomic write).
5. Wizard preview/remote: `ui/configurator/business/create_config.go:57` (`buildConfigFromModel`) конвертит `WizardModel` → `BuildContext`; Save/Update идут через `core/config_service.go` → тот же `build.BuildConfig`.
6. Hotfix-слой: `core/config/varsubst.go` — подстановка `@vars` в `parser_config.outbounds[].options` перед выдачей парсеру.

## 2. Состояние: state.json

**Путь:** `bin/wizard_states/state.json` (текущее) + именованные снапшоты `bin/wizard_states/<id>.json`; для remote-таргетов — `bin/wizard_states/remote/<machineID>/state.json` (`ui/configurator/business/state_store.go:35-76`, `internal/platform/platform_common.go:156-167`).

**In-memory тип:** `State` — `core/state/state.go:48-144` (wizard-алиас `WizardStateFile = corestate.State`, `ui/configurator/models/wizard_state_file.go:27`). Поля:
- Meta: `Version`, `Comment`, `CreatedAt/UpdatedAt`, `Target`/`TargetPlatform`/`TargetArch` (SPEC 097: local|remote + GOOS/GOARCH цели); `ID` — legacy, не сериализуется.
- `Connections ConnectionsSection` — **canonical**: `Sources []Source` + глобальные `Outbounds []configtypes.OutboundConfig` + `Defaults{Reload, MaxNodes}` (`core/state/connections.go:13-24`).
- `ParserConfig configtypes.ParserConfig` — деривная legacy-view (заполняется reverse-адаптером на Load, синхронизируется `syncConnectionsFromLegacy` на Save).
- `Rules []Rule` — v6-правила с дискриминатором `kind: preset|inline|srs` (`core/state/rule_types.go`; preset = тонкая ссылка `{kind, ref: <preset_id>, enabled, body.vars}`).
- `Vars []SettingVar{Name, Value}` — переопределения template-переменных (`core/state/legacy_types.go:28`).
- `DNS DNSOptions` — v6 DNS (servers/rules с kind: template|preset|user, `core/state/dns_options.go:128`).
- `WarpAccounts *WarpAccountsSection` — кэш Cloudflare WARP-регистраций (WG + MASQUE ключи, `core/state/disk_v6.go:73`).
- Legacy in-memory (не пишутся): `ConfigParams []ConfigParam{Name,Value}` (route.final и т.п.), `CustomRules []CustomRule`, `SelectableRuleStates`, `DNSOptions *LegacyDNSOptionsV5`.

**On-disk формат v6** (`diskStateV6`, `core/state/disk_v6.go:52-59`): JSON `{meta{version:6, schema:"presets_v1", comment, created_at, updated_at, target, target_platform, target_arch}, connections, rules, vars, dns_options, warp_accounts}`. Времена — RFC3339.

**Сериализация:** `State.Save` (`core/state/save.go:26`) — atomic write (`.tmp` + fsync + rename), всегда пишет v6; одноразовый бэкап `.pre-058.bak` при миграции outbounds (SPEC 058). Рекомендованный лимит 256 KB (warn-only, state_store.go:31).

**Миграции:** `core/state/load_router.go:30-71` (`Parse`): v6 → `parseCurrent` (load_v6.go), v5 → `parseV5Legacy` (load_v5.go), v2–v4 → `parseLegacyAndMigrate` (load_v2_v3_v4.go, legacy_migration.go); v5→v6 конверсия правил — migration_v5_to_v6.go. История версий — комментарий `state.go:27-38`.

## 3. Подписки, ноды, выбор

- **Определение подписки** — `Source` (`core/state/connections.go:41-94`) внутри state.json: `type` = `subscription` (URL → пачка нод) | `server` (один URI или ручной `config_json` passthrough); `tag TagSpec{prefix,postfix,mask}`, `skip`, `update UpdateSpec{interval_hours, auto_refresh}`, `max_nodes`, per-source `outbounds` (группы), `detour_tag` / `detour_node_hash`+`detour_node_label` (цепочки, SPEC 077/101), `exclude_from_global`.
- **Метаданные fetch'а** — `Source.Meta *SubscriptionMeta` (connections.go:128-162): headers провайдера (profile_title, userinfo upload/download/total/expire), история (`last_fetched_at`, `last_status`, `error_count`, `http_status_code`, `raw_body_bytes`), `nodes_count_fetched`, `truncated`, **`preview_nodes []string`** (сырые share-URI нод — by design, см. memory «секреты в state.json»), `provider_announce`.
- **Кэш тел подписок** — `bin/subscriptions/<source-id>.raw` (remote: `bin/wizard_states/remote/<id>/subscriptions/`): `core/state/raw_cache.go` — `WriteRawBody` (atomic, пустое тело не кэшируется), `ReadRawBody`, `DeleteOrphans` (lazy GC).
- **Отключённые ноды** — `Source.DisabledNodes map[string]int64` (connections.go:93): ключ = identity-hash ноды (`core/config/node_hash.go`, стабилен к переименованиям провайдера), значение = unix-время подтверждения, с TTL-GC.
- **Выбранная нода** (активный выбор в selector'е) — **не в state.json**: её персистит само ядро sing-box в `cache.db` (`experimental.cache_file` в шаблоне, wizard_template.json:233); лаунчер читает/пишет через Clash API. В state живут только выбор outbound'а per-rule (`CustomRule.SelectedOutbound`, `Rule.body`) и route.final (`ConfigParams`).
- Распарсенный кэш нод существует только in-memory (`build.ParsedCache`); файл `bin/outbounds.cache.json` упразднён (SPEC 052 phase 8, комментарий `core/build/parsed_cache.go:8`), константа осталась в constants.go:24.

## 4. Прочие персистентные файлы

| Файл | Назначение |
|---|---|
| `bin/config.json` | итоговый sing-box config (результат BuildConfig) |
| `bin/settings.json` | настройки лаунчера, тип `Settings` — `internal/locale/settings.go:16`: lang, ping-настройки, HWID + приватность подписочных заголовков, debug-API (enabled/token/port), `last_template_launcher_version`, daemon-режим (`core_backend_mode`, `daemon_address`, fingerprint) |
| `bin/wizard_template.json` | шаблон (см. §1) |
| `bin/wizard_states/*.json` | state.json + именованные снапшоты; `remote/<id>/` — state, config.json, subscriptions/, srs/ удалённых машин (SPEC 097/098) |
| `bin/subscriptions/*.raw` | кэш тел подписок |
| `bin/rule-sets/` | скачанные .srs rule-set'ы (remote: `srs/`) |
| `bin/cache.db` | файл ядра sing-box (selector-выбор, fakeip) — управляется ядром |
| `bin/get_free.json` | community-список бесплатных серверов (`ui/configurator/dialogs/get_free_dialog.go`, обновляется с pinned-ref) |
| `bin/locale/` | переводы UI |
| `logs/` | singbox-launcher.log, sing-box.log, parser.log, api.log (constants.go:63-66) |
| `daemon.json` | **не лаунчера**: state-dir демона `sing-box lxd` (macOS), listen/tls/secret (`core/daemon_manager_darwin.go:329`) |
| `bin/remote-config.json` | legacy до SPEC 098, читается только миграцией (constants.go:18) |

**Ключевое для унификации с мобильным приложением:** синтаксис шаблона = `@var`-плейсхолдеры + `#if`-конструкция + params(replace/prepend/append) + presets с локальным неймспейсом тегов; state = v6-схема `presets_v1` с thin preset-refs и kind-дискриминаторами; вся подстановка сосредоточена в `core/template/substitute.go` + `core/template/loader.go`, сборка — в `core/build/build.go`, схема диска — в `core/state/disk_v6.go`.