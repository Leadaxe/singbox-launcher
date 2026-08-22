# LxBox/app — карта сборки конфига и хранения состояния

Авторитетные внутренние доки: `/Users/macbook/projects/LxBox/docs/TEMPLATE.md` (шаблон), `/Users/macbook/projects/LxBox/docs/STORAGE.md` (полная схема стораджа, история миграций), `/Users/macbook/projects/LxBox/docs/ARCHITECTURE.md`.

## 1. Сборка конфига

**Шаблон — файл, а не код**: `app/assets/wizard_template.json` (зашит в APK через flutter assets, юзером не изменяется — чистый каталог + скелет конфига).

- Загрузка: `TemplateLoader` (`app/lib/services/template_loader.dart`) — async-синглтон, кэш ключуется тегом локали; display-тексты локализуются overlay'ем `assets/l10n/ru/template.json` (`TemplateOverlay`, §279). Модель: `WizardTemplate` в `app/lib/models/parser_config.dart` (поля: `parserConfig`, `groupTemplates`, `vars`, `config`, `selectableRules`, `dnsOptions`, `pingOptions`, `speedTestOptions`, `varSections`).
- **Единственная точка сборки**: `buildConfig()` в `app/lib/services/builder/build_config.dart` (§3.4). Вход `List<ServerList>` + `BuildSettings{userVars, channels, customRules, routeFinal, tunApps, vpnMode, idleSuspend, passiveCheck…}`; выход `BuildResult{configJson, config, validation, emitWarnings, generatedVars, channelsWithoutNodes}`. Итог пишется в `<docs>/singbox_config.json` (Kotlin `ConfigManager` → libbox).
- **Движок подстановки** — `app/lib/services/builder/if_engine.dart`: `@var`-плейсхолдеры + декларативный `#if` (map-spread и array-element; предикаты `and`, `#in`, `#matches`), типизированный coerce `coerceVarValue(raw, type)` строго по объявленному `WizardVar.type` (`bool`/`int` с clamp uint16, остальное — строка), sentinel `Dropped`. **В комментарии прямо сказано: «Дизайн заимствован у singbox-launcher SPEC 067 (десктоп), адаптирован под Dart. НЕ берём: `params[]`-механику, `@runtime.*` globals»** — это ключевая точка унификации.
- Пайплайн `buildConfig`: merge defaults+userVars → deepCopy `template.config` → `walk()` (подстановка+`#if`) → эмит нод `ServerList.build(ctx)` (`server_list_build.dart`, `NodeSpec.emit`, аллокатор тегов `_BuildCtx.allocateTag`) → каналы `_buildChannelGroups` (selector `vpn-N` + urltest-двойник `vpn-N-auto`) → `normalizeRuleOrder` (`rule_order.dart`, ось `num` §370) → `applyAllCustomRules` + `expandPreset` (`preset_expand.dart` — у пресетов свой substituteVars того же движка) → flush `RuleSetRegistry` в `route.rule_set/rules` → post-steps → `validateConfig` (`validator.dart`).
- Post-steps (`app/lib/services/builder/post_steps/`): `custom_rules.dart`, `dns_servers.dart`/`dns_rules.dart` (`applyCustomDns`), `tls_transforms.dart` (`applyTlsFragment`/`applyMixedCaseSni`), `tun_packages.dart` (split-tunneling, всегда последний), и heal-страховки `heal_dangling_detours`, `heal_dangling_resolve_servers`, `heal_legacy_dns_strategy`, `heal_unknown_utls_fingerprints`, `heal_invalid_reality` (деградация вместо fatal ядра).

**Что где захардкожено**:
- В шаблоне (декларативно, с `@var` и `#if`): `log`, `certificate`, скелет `dns` (пустые servers/rules + `@dns_final`/`@dns_strategy`), `inbounds` (tun-in и mixed-in гейтятся `#if @vpn_mode`), базовые `outbounds` (`direct-out`, `block`), `route` (rules пустой — sniff/hijack-dns/resolve переехали в залоченный пресет `traffic-processing` c `num:0`), `experimental.cache_file`, `services:[oom-killer]`. Плюс каталоги: `group_templates`+`default_channels` (§267, сборка каналов), `sections[].vars` (36 переменных, 8 секций), `selectable_rules` (пресеты), `dns_options.servers` (шаблонные DNS), `ping_options`, `speed_test_options`, `parser_config{version:5, reload}`.
- В коде билдера: сборка selector/urltest-групп каналов, деградация `route.final`→`vpn-1`, `route.lx_idle_suspend*`, `urltest.passive_check`/`balancer` (форк-поля ядра), все heal-шаги.

## 2. Состояние (`SettingsStorage`)

**Один JSON-файл, не SharedPreferences**: `lxbox_settings.json` в `getApplicationDocumentsDirectory()`. Класс `SettingsStorage` (`app/lib/services/settings_storage.dart`, ~50K) разнесён `part`-файлами по `app/lib/services/settings_storage/`: `io.dart` (атомарная запись: uniq `.tmp` → rename, `.bak`-recovery §072, in-memory `_cache`, `flushToDisk` §107), `vars.dart`, `sources_rules.dart`, `network.dart`, `channels.dart`, `backup_tun.dart`, `vpn_mode.dart`, `native_prefs.dart`, `warp.dart`. Формат — pretty-JSON (`JsonEncoder.withIndent`). Дополнительно: флаг `configDirty` (§113) и import-allowlist default-deny (§159: `allowedTopLevelKeys` + `allowedVarKeys`).

Top-level ключи (полный закрытый список `allowedTopLevelKeys`, строки 131–158):

| Ключ | Сущность / модель |
|---|---|
| `vars` | плоский `Map<String,String>`: template-vars (log_level, dns_final, tun_*, …) **и** app-флаги (`_appFeatureFlagVars`: auto_update_subs, debug_token, app_language, subscription_user_agent/hwid…) в одной мапе |
| `server_lists[]` | sealed `ServerList` (`app/lib/models/server_list.dart`): `SubscriptionServers` (url, `meta`, `disabled_hashes` {identity-hash: lastSeen} §283, `identity` §289, `import_rules` §302, update-статусы) / `UserServer` (`raw_body`) / `FolderServers` (`members[{raw,enabled,detour}]` §234) + общий `DetourPolicy`. **Ноды не хранятся** — репарсятся при старте из raw_body/HTTP-кэша |
| `custom_rules[]` | sealed `CustomRule` (`app/lib/models/custom_rule.dart`), дискриминатор `kind`: `CustomRuleInline` / `CustomRuleSrs` / `CustomRulePreset` (`presetId`+`varsValues`) / `CustomRuleJson`; общая ось порядка `num` (§370); вложенные `RuleDns` (§117/§256) и `RuleResolve` (§247) |
| `channels[]` + `channels_migrated` | `Channel`/`ChannelAuto` (`app/lib/models/channel.dart`): tag `vpn-1..10` (immutable id), label, node_filter(+invert), default_filter, include_direct/block, `auto{url,interval,tolerance,mode,balancer}` §125/§208 |
| `dns_options` | sealed `DnsServerRef` {template/preset/inline} и `DnsRuleRef` (`app/lib/models/dns_ref.dart`, §294 — on-disk shape байт-совместим со старым) |
| `route_final`, `route_idle_suspend(_reachable)`, `urltest_passive_check`, `ping_options`, `tun_apps`, `vpn_mode`, `warp_account`, `masque_account`, `node_sort_mode`, `node_manual_order`, `native_prefs` | скалярные/объектные настройки (см. STORAGE.md) |

**Выбранная нода канала в стораджe приложения НЕ хранится** — её помнит само ядро через `experimental.cache_file` (`cache.db`); в `lxbox_settings.json` есть только `route_final` (какой канал — финальный). `native_prefs` (§189) — JSON-зеркало Android `SharedPreferences boxvpn_boot.*` («JSON = истина, native = рабочая копия для Dart-less моментов»); в SharedPreferences остаются только Flutter-prefs типа `app_theme_mode`.

## 3. Тела подписок / кэш

- **`HttpCache`** (`app/lib/services/subscription/http_cache.dart`): `getApplicationSupportDirectory()/sub_cache/<url.hashCode.toRadix(16)>` — сырое тело, `<hash>.headers` — JSON заголовков; атомарная запись tmp→rename. Используется для offline-rehydrate нод при старте (`SubscriptionController._rehydrateFromCache`) и вкладки Source. Файловые подписки хранятся как `url: "file:<uuid>"` со снапшотом в этом же кэше (§129). ⚠ STORAGE.md пишет `http_cache/<sha1>` — код правдивее: `sub_cache/<hashCode>`.
- **`sources.dart`** (`app/lib/services/subscription/sources.dart`): sealed `SubscriptionSource` (`UrlSource`/`FileSource`/`ClipboardSource`/`InlineSource`), fetch с 3 ретраями и exp backoff; UA-бренд `LxBox-android/<ver>` (`user_agent.dart`).
- **Метаданные**: `SubscriptionMeta` (`app/lib/models/subscription_meta.dart`) — из HTTP-заголовков `subscription-userinfo`, `profile-title`, `profile-update-interval`, `profile-web-page-url`; персистится внутри `server_lists[].meta`. Идентичность фетча: глобальный static-holder `SubscriptionIdentity` (`app/lib/services/subscription/subscription_identity.dart`, ключи `subscription_*` в `vars`) + per-sub `SubscriptionIdentityOverride` (`server_list.dart`, §289).
- **`.srs`-кэш**: `RuleSetDownloader` (`app/lib/services/rule_set_downloader.dart`) → `$docs/rule_sets/<id>.srs`.

## 4. Миграции состояния

**Глобального номера версии стораджа нет.** Механизм — четыре слоя:

1. **One-shot boolean-guards** в самом `lxbox_settings.json`: `channels_migrated` (§125 — `_migrateChannelsIfNeeded` в `settings_storage/channels.dart`, seed каналов из `group_templates`+`default_channels` c учётом legacy `enabled_groups`), `presets_migrated` (§159 — seed дефолтных пресетов), `preset_ids_remapped` (§228 — сама миграция удалена в §229, ключ оставлен инертным, имя не переиспользовать).
2. **Толерантные `fromJson`**: отсутствующий `kind` → `inline`, `target`→`outbound` читается под обоими именами, отсутствующий `vpn_mode` → `mode=vpn`, legacy flat `import_rules` конвертируется на чтении, первое сохранение переписывает в новый формат.
3. **Lazy-нумерация/нормализация вместо версионированных шагов**: ось `num` правил размечается при первом заходе на Routing (`markRuleOrder`) и нормализуется на каждом билде (`normalizeRuleOrder`, §370); heal-on-load битых detour-ссылок каналов (`sources_rules.dart`, тесты `test/migration/channel_heal_refs_test.dart`, `detour_channel_heal_test.dart`).
4. **Allowlist на импорте бэкапа** (§159, default-deny) — единственная чистка мусорных ключей; legacy-ключи (`proxy_sources`, `app_rules`, `enabled_groups`, `excluded_nodes`) на диске безвредны и никем не читаются.

`test/migration/` покрывает seed каналов (fresh install, идемпотентность, нерезолвенные `@var` в auto), и heal-проходы. Шаблон версионируется только полем `parser_config.version` (сейчас 5, схема парсер-пайплайна §026) — в отличие от десктопного лаунчера шаблон зашит в APK и на диске не живёт, поэтому проблемы «протухшего wizard_template.json» здесь нет.