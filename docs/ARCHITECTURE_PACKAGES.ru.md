# Инвентарь по пакетам

**🌐 Язык**: [English](ARCHITECTURE_PACKAGES.md) | Русский

> Спутник **[ARCHITECTURE.ru.md](ARCHITECTURE.ru.md)** (§8). По одной строке
> ответственности на пакет плюс ключевые файлы с однострочным назначением,
> сгруппированные по слоям L0–L7. Отражает **текущую** раскладку файлов после
> SPEC 070 (разбиения монолитов уже внедрены). Тестовые файлы (`*_test.go`) в
> пофайловых списках опущены.

Легенда заголовков слоёв совпадает с ARCHITECTURE.ru.md §2.

---

## L0 — platform (`internal/platform`)

**Ответственность:** абстракция ОС за одним интерфейсом — sleep/wake, HWID-информация
об устройстве, перечисление процессов, чистка призрачных WinTun-адаптеров,
канонические геттеры путей файловой системы. Файлы с платформенными тегами
(`*_darwin.go`/`*_linux.go`/`*_windows.go`/`*_stub.go`); импортов вверх нет.

| Файл | Назначение |
|------|---------|
| `platform_common.go` | Общие геттеры путей (config.json, wizard_template.json, `bin/wizard_states`, rule-sets, subscriptions) — единый источник истины о раскладке файловой системы. |
| `platform_darwin.go` / `platform_linux.go` / `platform_windows.go` | Корни путей по ОС, особенности трея и Dock, отправка console-ctrl (Windows). |
| `power_darwin.go` / `power_linux.go` / `power_windows.go` / `power_stub.go` | Подписка на события sleep/wake (IOKit / logind DBus / WM_POWERBROADCAST) плюс состояние `IsSleeping`/`PowerContext`; заглушка — no-op. |
| `device_info_darwin.go` / `_linux.go` / `_windows.go` | Извлечение HWID (sw_vers / `/etc/os-release` / wmic), кешируется на процесс. |
| `proclist.go` + `proclist_darwin.go` / `_linux.go` / `_windows.go` | Кросс-платформенный интерфейс перечисления процессов и `ProcessEntry` (пикер профайлера трафика). |
| `wintun_cleanup.go` / `wintun_cleanup_other.go` | Общие константы режимов чистки призрачных адаптеров и чистый предикат `ghostTunDecision` (платформонезависимый, покрыт юнит-тестами). |
| `wintun_cleanup_windows_device.go` | SPEC 065: перечисление устройств через SetupAPI и `DIF_REMOVE` фантомных адаптеров. |
| `wintun_cleanup_windows_nla_profiles.go` / `_nla_sigs.go` | Чистка сетевых профилей и сигнатур NLA. |
| `wintun_cleanup_windows_syscall.go` | Ленивые привязки DLL и константы GUID, общие для файлов чистки. |
| `fs_unix.go` / `fs_windows.go` | Хелперы атомарной записи и fsync по ОС. |
| `dock_handler.go` / `dock_handler_stub.go` | Скрытие иконки в Dock на macOS; на остальных — заглушка. |
| `privileged_darwin.go` / `privileged_stub.go` | Привилегированные операции на macOS (удаление кеша и логов TUN); на остальных — заглушка. |
| `singbox_exec_path.go` / `singbox_exec_path_linux.go` | Разрешение пути к исполняемому файлу sing-box (на Linux может использоваться `PATH`). |

---

## L1 — shared-internal (листовые утилиты)

Каждый пакет самодостаточен и ни от чего не зависит (либо зависит только от `debuglog`/`constants`).

| Пакет | Ответственность | Ключевые файлы |
|---------|----------------|-----------|
| `internal/constants` | Константы уровня приложения (имена файлов, пины ядра и шаблона, строки UA, лимиты). | `constants.go` |
| `internal/debuglog` | Уровневое логирование (Off/Error/Warn/Info/Verbose/Trace), опциональный in-memory sink для вьюера логов, хелперы замера времени. | `debuglog.go`, `close.go` |
| `internal/locale` | i18n: английский встроен, внешние/удалённые JSON по языкам, поиск `T`/`Tf` с фоллбэком на английский. | `locale.go`, `settings.go` |
| `internal/traffic` | Развязанный профайлер трафика (только stdlib): сшивка Clash-поллера и хвоста лога, кольцевой буфер, запись сессий, атрибуция по процессам. | `profiler.go`, `session.go`, `types.go`, `clash_connections.go`, `logtail.go`, `parser.go`, `http_client.go`, `singleton.go`, `inode_unix.go`/`inode_windows.go` |
| `internal/outboundutil` | Единый источник истины для маппинга литералов `reject`/`drop` → `action`/`method` правила (общий для сборки и UI). | `outbound.go` |
| `internal/srstag` | Контент-адресуемая генерация имён локальных SRS-файлов (`name-<hash8>`) для дедупликации. | `srstag.go` |
| `internal/urlsafe` | Allowlist URL-схем для кликабельных элементов (http/https/tg разрешены; javascript/file/data заблокированы). | `url.go` |
| `internal/urlredact` | Вырезание userinfo и чувствительных query-параметров из URL перед логированием. | `urlredact.go` |
| `internal/textnorm` | Нормализация UTF-8 и отображаемых символов в тегах прокси (например `❯ → >`), удаление ANSI. | `proxy_display.go`, `stripansi.go` |
| `internal/ctxutil` | Хелпер контекста, учитывающий сон системы. | `sleep.go` |
| `internal/process` | Тонкая обёртка над списком процессов для рантайм-проверок. | `process.go` |
| `internal/wizardsync` | Предикаты слияния GUI→модель без Fyne (`GuiTextAwaitingProgrammaticFill`, `FinalOutboundSelectReadLooksStale`) — тестируются без CGO/GL. | `guards.go` |
| `internal/dialogs` | Общие примитивы диалогов, не зависящие от `ui` (кастомный диалог, диалог неудачной загрузки, авто-скрывающееся уведомление). | `dialogs.go` |
| `internal/lxdclient` | mTLS-клиент демона `sing-box lxd` (SPEC 096/097): вызовы admin REST, пиннинг сертификата (никогда не опционален), разбор одноразовых приглашений (`адрес#отпечаток#код`), клиентская идентичность на машину, определение канала, чтение телеметрии хоста и clients-info. Без состояния приложения. | `client.go`, `identity.go`, `invite.go`, `host.go` |

> Замечание: и `internal/dialogs`, и `internal/fynewidget` зависят от Fyne.
> `dialogs` отнесён сюда как листовая утилита (без внутренних перекрёстных
> зависимостей); `fynewidget` отнесён к L7 как пакет виджетного яруса. Оба
> переиспользуются во всех слоях UI.

---

## L2 — core-domain (состояние + сборка + конфиг + шаблон)

Чистый домен. Ни Fyne, ни `AppController`.

### `core/state` — схема состояния, загрузка/сохранение, миграции

**Ответственность:** канонический `State` v6 (Connections / Rules / DNS / Vars),
загрузка v2–v6 с миграцией вперёд, атомарное сохранение, адаптеры
легаси↔канонический, ULID и идентичность, raw-кеш тела на источник.

| Файл | Назначение |
|------|---------|
| `state.go` | Корневая структура `State` (идентичность + легаси-представление `ParserConfig` + канонические `Connections`/`Rules`/`DNS`) и хелперы доступа. |
| `rule_order.go` | Числовая ось порядка правил (SPEC 106): ленивый сдвиг до первой дырки, re-seed несортируемых, нормализация состояний, записанных до появления оси. |
| `save.go` | Память→диск: `syncConnectionsFromLegacy`, `marshalDisk` (раскладка v6), атомарные fsync+rename, бэкап SPEC 058. |
| `load_router.go` | `Load`/`Parse`: определение схемы (top-level против `meta.version`), маршрутизация в парсеры v6/v5/v2-v4. |
| `load_v6.go` | `parseCurrent` (канонический v6), фоллбэк `legacyDevDNSToOptions` и `legacyCustomRulesFromV6` (вывод легаси-представления). |
| `load_v5.go` | `parseV5Legacy` и `deriveV6FromLegacy` (добивание BUG1 для headless-путей). |
| `load_v2_v3_v4.go` | `parseLegacyAndMigrate` (v2/v3/v4 → v5 → канонический). |
| `load_normalize.go` | Общий `normalizeAfterLoad` (`syncLegacyFromConnections`, нормализация nil-срезов, санитайз ссылок outbound'ов), вызывается всеми путями разбора. |
| `sync_to_connections.go` | `syncConnectionsFromLegacy` (путь Save: ParserConfig→Connections, сопоставление по URL/URI, сохранение ID/Meta, ULID для новых источников). |
| `sync_to_legacy.go` | `syncLegacyFromConnections` (путь Load: заполняет ParserConfig из Connections для UI обратной совместимости). |
| `connections.go` | Каноническая схема v5: `ConnectionsSection`, `Source` (подписка/сервер), `SubscriptionMeta`. |
| `connections_helpers.go` | Вынесенные общие хелперы `buildTagSpec`, меток сервера и фрагментов URI (дедуплицированы из адаптера и миграции). |
| `rule_types.go` | `Rule` v6 (kind = preset/inline/srs), тела и валидация `DecodeBody`. |
| `rule_identity.go` | Чистая функция `StableRuleID` (preset→Ref, inline/srs→очищенное имя); идентичность вычисляется, а не хранится. |
| `dns_options.go` | Плоская схема DNS v6 (дискриминатор kind template/preset/user) и собственные Marshal/Unmarshal. |
| `sync_dns.go` | `SyncDNSOptionsWithActivePresets` (идемпотентное добавление и удаление DNS-записей пресета при переключении правила). |
| `migration_v5_to_v6.go` | Хелперы v5→v6 (эвристика kind в `migrateCustomRule`, `migrateDNS`). |
| `legacy_migration.go` | Миграция v2/v3/v4→v5 (`migrateV4ToV5`, `migrateLegacySources`). |
| `legacy_types.go` / `legacy_v4.go` | Легаси-типы v4 в памяти и на диске (прослойки только на чтение). |
| `diff.go` | Обнаружение изменений: `CacheStale` (влияние на парсер) / `ConfigStale` (влияние на шаблон). |
| `ulid.go` | Монотонный генератор ULID в Crockford-base32. |
| `raw_cache.go` | `WriteRawBody` (атомарно) / `ReadRawBody` / `DeleteOrphans` для `bin/subscriptions/<id>.raw`. |
| `provider_announce.go` | Разобранные announce-заголовки (SPEC 061): статус HWID-привязки, лимит устройств. |
| `adapter_source.go` | Конвертер `Source → ProxySource` для легаси-кода парсера. |
| `disk_v6.go` | Константы дисковой схемы v6 и приватная структура `diskStateV6`. |

### `core/snapshot`

**Ответственность:** сборка «только на чтение» шаблона + состояния + кеша + конфига в один JSON для диагностики и bug-report'ов.
- `snapshot.go` — `Snapshot.Build`: обход спецификаций файлов, обработка отсутствующего и битого JSON, отчёт `Files`/`Missing`/`Errors`.

### `core/build` — пайплайн config-to-JSON (чистый)

**Ответственность:** `BuildConfig` оркеструет отрисовку по секциям через чистые
обработчики секций и резолверы `ResolveDNS`/`ResolveRoute`/`ExpandPreset`.

| Файл | Назначение |
|------|---------|
| `build.go` | `BuildConfig`-оркестратор: валидация шаблона, `GetEffectiveConfig`, диспетчеризация по секциям, конкатенация финального JSON (чистая). |
| `sections.go` | `BuildOutboundsSection`/`BuildEndpointsSection` — отрисовка outbound'ов и endpoint'ов с маркерами парсера и обрезкой для предпросмотра. |
| `format.go` | Хелперы отступов и форматирования JSON (`Indent`, `FormatSectionJSON`). |
| `dns_merge.go` | `MergeDNSSection` — наложение DNS-серверов, правил, final и strategy поверх DNS шаблона; вырезание полей, нужных только визарду. |
| `route_merge.go` | `MergeRouteSection` — добавление включённых пользовательских правил и SRS rule_set'ов; конвертация remote→local rule_set. |
| `preset_merge.go` | `MergePresetsIntoRoute`/`MergePresetsIntoDNS` (SPEC 053) — второй проход, добавляющий фрагменты из пресетов через резолверы; дедупликация по тегу. |
| `preset_expand.go` | `ExpandPreset` — подстановка `@vars`, вычисление `if`/`if_or`, префиксы тегов, чистка висячих ссылок rule_set → `PresetFragments`. (Единый `evalIf` — стадия 3b SPEC 070.) |
| `preset_outbounds.go` | Раскрытие outbound'ов пресета (режимы add/update, SPEC 055/057). |
| `resolve_dns.go` | Чистый резолвер `ResolveDNS`: сведение template/preset/user DNS с метаданными Active/Enabled/Locked. |
| `resolve_route.go` | Чистый резолвер `ResolveRoute`: сведение маршрутизации preset/inline/srs из `state.Rules`. |
| `resolve_outbounds.go` | Резолвер outbound'а по записи (слияние `Ref`/`Updates`, SPEC 057/058). |
| `rules_pipeline.go` | Хелперы пайплайна порядка правил, общие для разрешения маршрутов. |
| `sync_outbounds.go` | `SyncOutboundsWithActivePresets` (adopt-on-first-sync; жизненный цикл outbound'ов, привязанных к пресетам). |
| `migrate_outbounds_spec058.go` | `MigrateOutboundsToReferencedShape` — одноразовая миграция outbound'ов v5→v6. |
| `outbound_diff.go` | Хелпер `OutboundFieldDiff` (вычисление USER-патча для диалога правки). |
| `parsed_cache.go` | Кеш разобранных узлов в памяти, используемый сборкой. |
| `secrets.go` | Материализация и редакция секретных полей во время сборки. |

### `core/config` — генерация подписка→outbound'ы и чтение конфига

**Ответственность:** оркестрация пайплайна подписка→разбор→JSON outbound'ов (трёхпроходная проверка валидности и фильтрация селекторов); чтение живого `config.json`.

| Файл | Назначение |
|------|---------|
| `outbound_generator.go` | Оркестратор `GenerateOutboundsFromParserConfig` плюс `GenerateNodeJSON`/`GenerateEndpointJSON`/`GenerateSelectorWithFilteredAddOutbounds` (утончён после разбиений SPEC 070). |
| `outbound_validity.go` | Трёхпроходный алгоритм: `buildOutboundsInfo` → `computeOutboundValidity` (топологическая сортировка, детект циклов) → `generateSelectorJSONs`. |
| `outbound_jsonbuilder.go` | `JSONBuilder{parts}` с безопасным по порядку вставки `AppendField` (заменяет связку `fmt.Sprintf`+`strings.Join`). |
| `outbound_filter.go` | Фильтрация узлов для селекторов (`filterNodesForSelector`, `FilterNodesExcludeFromGlobal`, синтетический узел expose, хелперы предпросмотра). |
| `outbound_share.go` | Поиск share-URI по записанному `config.json` (`GetOutboundMapByTag`, `ShareProxyURIForOutboundTag`). |
| `config_loader.go` | Чтение `config.json` (с поддержкой JSONC): группы селекторов, имя TUN-интерфейса, `experimental.cache_file`. |
| `varsubst.go` | `SubstituteParserConfigPlaceholders` — разрешение плейсхолдеров `@name` в опциях outbound'а (дефолты шаблона + override состояния). |
| `models.go` | Псевдонимы типов → `configtypes` для обратной совместимости (`config.ParsedNode` и т. п.). |

### `core/config/configtypes`

**Ответственность:** общие типы парсера в отдельном пакете, чтобы разорвать цикл импортов (`subscription` импортирует `configtypes`, а не `config`).
- `types.go` — `ParserConfig`, `ProxySource`, `OutboundConfig`, `ParsedNode`, `NormalizeParserConfig`.
- `matcher.go` — pattern/filter matching (`MatchesPattern`).

### `core/config/parser`

**Ответственность:** извлечение и нормализация блока `ParserConfig` из `config.json`.
- `factory.go` — `ExtractParserConfig`, `NormalizeParserConfig`, duplicate-tag stats.

### `core/config/subscription` — парсеры протоколов, загрузка и кодирование

**Ответственность:** парсеры URI по протоколам и кодировщики share-URI (VLESS/VMess/Trojan/SS/Hysteria2/TUIC/SSH/SOCKS/Naive/WireGuard, плюс профили Amnezia `vpn://`) и транспорт подписок (загрузка, декодирование, метаданные).

| Файл | Назначение |
|------|---------|
| `source_loader.go` | Точка входа `LoadNodesFromSource`: загрузка → определение формата → разбор → префикс/постфикс/маска тегов, skip-фильтр и дедупликация; офлайн-хук `LookupCachedBody`. |
| `fetcher.go` | HTTP-загрузка `FetchSubscriptionWithMeta` (заголовки HWID/UA, лимит 10 МБ) и декодирование announce-заголовков; устаревшая обёртка `FetchSubscription`. |
| `meta.go` | Разбор метаданных из заголовков и inline-`#comment` (Profile-Title, Subscription-Userinfo, интервал обновления), announce провайдера при пустом теле. |
| `decoder.go` | `DecodeSubscriptionContent` (определение base64 / JSON-массива Xray). |
| `node_parser_core.go` | Диспетчер `ParseNode` и общие хелперы (`extractTagAndComment`, `generateDefaultTag`, `buildOutbound`, `IsDirectLink`). |
| `node_parser_transport.go` | Транспорт и TLS для VLESS/Trojan из query URI (`uriTransportFromQuery`, `vlessTLSFromNode`, `trojanTLSFromNode`, `queryGetFold`). |
| `node_parser_vmess.go` | Полезная нагрузка VMess (JSON и легаси в открытом виде) и транспорты. |
| `node_parser_ss.go` | Shadowsocks (SIP002 и легаси). |
| `node_parser_ssh.go` | SSH. |
| `node_parser_hysteria2.go` | Hysteria2 (плюс `hysteria2_ports.go` для диапазонов mport). |
| `node_parser_naive.go` | Naive. |
| `node_parser_tuic.go` | TUIC v5 (SPEC 074). |
| `node_parser_wireguard.go` | WireGuard и поднятые поля AmneziaWG 2.0 (`applyAWGFields`, диапазонные `h1`–`h4`, предупреждение о пересечении, кламп MTU до 1280). |
| `node_parser_amnezia.go` | Импорт профиля Amnezia `vpn://`: декодирование base64url + qCompress → контейнер WG/AWG (`last_config`) → канонический URI `wireguard://` (SPEC 075). |
| `wgconf_text.go` | Вставленный текст конфига `[Interface]/[Peer]` → URI `wireguard://` (`ExtractWGConfBlocks` / `ConvertWGConfText`, SPEC 076). |
| `xray_json_array.go` / `xray_outbound_convert.go` | Разбор JSON-массива Xray: элемент → `ParsedNode` (плюс jump-хоп), `remarks`→Label, slug-теги; streamSettings→транспорт/TLS. |
| `share_uri.go` | Диспетчер `ShareURIFromOutbound` (обратный к `ParseNode`). |
| `shareuri_vless.go` / `shareuri_vmess.go` / `shareuri_trojan.go` / `shareuri_ss.go` / `shareuri_socks.go` / `shareuri_hysteria2.go` / `shareuri_ssh.go` / `shareuri_tuic.go` / `shareuri_naive.go` / `shareuri_wireguard.go` | Кодировщики outbound→share-URI по протоколам. |
| `shareuri_helpers.go` | Общие хелперы кодирования (`mapGet*`, `transportToQuery`, TLS в query, ALPN/insecure). |
| `utf8_utils.go` | Сведённые воедино валидация и починка UTF-8 (`FixUTF8*`, `HasControlChars`) — дедупликация SPEC 070. |
| `encoding_utils.go` | Сведённое воедино декодирование base64 в нескольких вариантах — дедупликация SPEC 070. |

### `core/backup` — LX Backup v1 (переносимый обмен настройками)

**Ответственность:** экспорт и импорт настроек в формате, общем с мобильным
приложением LxBox (SPEC 103, фаза 4). Схема — `contract/schema/backup.schema.json`,
семантика — `contract/docs/BACKUP.md`.

| Файл | Назначение |
|------|------------|
| `types.go` | Форма файла: подписки, серверы, правила, DNS, переносимые переменные и блобы `extensions` по приложениям. |
| `export.go` | `State` → бэкап. Непереносимые поля источника (skip-фильтры, локальные outbound'ы, detour, id) уезжают в `extensions.launcher`: без этого цикл экспорт-импорт на ТОЙ ЖЕ машине терял бы настройки. |
| `import.go` | Бэкап → `State`. Правило, чья цель здесь не существует, импортируется **выключенным**, а не теряется и не включается: включённое правило с мёртвой целью заставляет ядро отвергнуть весь конфиг. Чужие блобы `extensions` хранятся нетронутыми до следующего экспорта. |
| `portable_vars.go` | Сгенерирован из `contract/registry/vars.json` (portable=true) и сверяется с ним тестом: разъехавшийся список либо потеряет настройку, либо утащит на чужую машину значение, которое там значит другое. |
| `file.go` | Атомарная запись с правами 0600 (в файле лежат секреты открытым текстом), потолок размера и default-deny отчёт о неизвестных ключах корня. |

---

### `core/template` — загрузка шаблона, пресеты и язык выражений

**Ответственность:** загрузка `wizard_template.json`, применение платформенных params, извлечение пресетов (SPEC 053), подстановка `@var`/`#if` (SPEC 067), валидация.

| Файл | Назначение |
|------|---------|
| `loader.go` | `LoadTemplateData`: чтение и валидация шаблона, применение params по GOOS, извлечение пресетов, возврат `TemplateData`. |
| `template_validate.go` | `ValidateWizardTemplate` (уникальность, ссылки, тело `#if`, внешний `@`-only). |
| `substitute.go` | `SubstituteVarsInJSON`: рекурсивная подстановка `@var` и walker `#if` (map-spread / array-element), runtime-глобалы `@runtime.platform`/`@runtime.arch`. |
| `ifexpr.go` | Формы вычисления предикатов `#if` (равенство, `#in`/`#matches`/`#not`, короткое замыкание AND/OR). |
| `vars_resolve.go` / `vars_default.go` | Разрешение переменных (`VarAppliesOnGOOS`, `ParamBoolVarTrue`) и выбор объектного `default_value` (GOOS/win7/default). |
| `preset_loader.go` / `preset_types.go` / `preset_outbounds.go` | Разбор пресетов и типы (rules / dns / outbounds / vars). |
| `cond_deps.go` | Статическое извлечение зависимостей условия (SPEC 107) — индекс, на котором стоит построчное обновление Settings; сам по себе часть общего контракта. |
| `preset_lite.go` | Адаптер, реализующий интерфейс `state.PresetLite` (сам интерфейс живёт в `core/state`, чтобы разорвать цикл импортов), и `PresetLiteMap` для `state.SyncDNSOptionsWithActivePresets`. |
| `rule_utils.go` | Хелперы правил (`HasOutbound`, `GetDefaultOutbound`, `CloneRuleRaw`). |

---

## L3 — сервисы и жизненный цикл

### `core/services`

**Ответственность:** реализации сервисов с состоянием, развязанные с `AppController` и не зависящие от Fyne.

| Файл | Назначение |
|------|---------|
| `file_service.go` | Пути (`ExecDir`, `ConfigPath`, `SingboxPath`, `WintunPath`), открытие/закрытие/ротация логов, бэкапы. |
| `api_service.go` | Интеграция с Clash API, состояние списка прокси и активного прокси, хранение последней ошибки пинга. |
| `state_service.go` | Dirty-маркеры (`MarkConfigStale`/`ClearCacheStale`), настройки (включено ли авто-обновление, кешированная версия лаунчера); публикует `StateChanged`. |
| `srs_downloader.go` | HTTP-загрузка удалённых rule-set'ов (.srs) и групповая загрузка для вкладки Rules; пути с учётом таргета (локальные против `srs/` машины). |
| `proxy_transport.go` | Шов `ProxyTransport`: операции с группами прокси (список, выбор, тест задержки, пул балансировщика) резолвятся **по области** — Clash HTTP для локального ядра, gRPC для демона или выбранной машины. |
| `lxd_remote_registry.go` | Реестр машин (`bin/remote-daemons.json`): записи `RemoteDaemon` (id / имя / адрес / отпечаток / GOOS / GOARCH), каталоги клиентской идентичности на машину, добавление/правка/удаление с чисткой директорий. |
| `lxd_remote_transport.go` | gRPC-транспорт к выбранной машине (подключение/отключение, стримы, admin-вызовы). |
| `lxd_remote_resources.go` | `CollectDeployResources` — собирает локальные rule-set'ы и тела подписок, на которые ссылается конфиг машины, чтобы Deploy отправил их вместе с конфигом. Без виджетов, поэтому покрыт юнит-тестами. |
| `lxd_remote_migration.go` | Одноразовая миграция singleton-профиля до SPEC 098 в директорию машины-владельца; при нескольких сопряжённых машинах отказывается работать с предупреждением. |

### `core/uiservice`

**Ответственность:** состояние UI и контейнер колбэков в отдельном пакете, чтобы `core/services` не импортировал Fyne.
- `ui_service.go` — singleton windows (wizard, settings), tray-menu state, icon resources, callback fields (`UpdateCoreStatusFunc`, `UpdateConfigStatusFunc`, `RefreshAPIFunc`, `ShowSubsResultFunc`, …, `FocusOpenChildWindows`); no callback *implementations*.

### `core/events`

**Ответственность:** типизированная синхронная шина событий (SPEC 047) — см. ARCHITECTURE.ru.md §4.
- `events.go` — `EventKind` enum (3 kinds) + `Event`/`Handler`/`Cancel` + `Bus` interface.
- `payloads.go` — `StateChangedPayload`, `ConfigBuiltPayload`, `VpnStateChangedPayload`.
- `memory_bus.go` — `MemoryBus`: `RWMutex`-guarded handler map, panic-isolated sync `Publish`.

### `core` (жизненный цикл приложения, процесса и конфига)

**Ответственность:** оркестрация жизненного цикла приложения, супервизия процесса, пайплайн обновления конфига, загрузчики. Владелец DI-разводки и EventBus.

| Файл | Назначение |
|------|---------|
| `controller.go` | Синглтон `AppController`: `NewAppController` (теперь **единственный** конструктор — полусобранный фоллбэк `GetController` удалён) плюс `GetController`/`GetControllerOrPanic`; держит сервисы, EventBus и UI-колбэки; публикует `VpnStateChanged`; идемпотентный `GracefulExit` (`sync.Once`). (Всё ещё ~827 строк; извлечение полей и блокировок отложено — ADR-070-7.) |
| `backend.go` | `CoreBackend` — шов движка, которым пользуются все вызывающие сверху вместо прямого обращения к процесс-менеджеру или Clash-клиенту. |
| `backend_legacy.go` | `LegacyBackend` — классический движок: спавн и супервизия `sing-box run`, управляющая плоскость Clash HTTP. |
| `backend_daemon_darwin.go` (+ `_dns_`, `_traffic_`) | `DaemonBackend` — движок lxd для macOS: управляющая плоскость gRPC, структурные стримы DNS и соединений для профайлера. Только под darwin build-тегами. |
| `backend_daemon_stub.go` | Заглушка для не-darwin, чтобы остальной код компилировался без gRPC (именно она держит сборку под Win7 чистой). |
| `daemon_manager_darwin.go` | Жизненный цикл демона со стороны лаунчера: строки sudo-команд, которые он отдаёт пользователю (`--service=install` / `=uninstall [--purge]` / `lxd client add`), сопряжение, паспорт демона (`GET /admin/info`). Сам ничего привилегированного не запускает. |
| `process_service.go` | `ProcessService`: `Start`/`Stop`/`Monitor`, машина состояний crash/restart, обработка выхода привилегированного скрипта, чистка TUN и фантомных адаптеров перед стартом (SPEC 065). |
| `config_service.go` (+ `_context.go`, `_subscriptions.go`) | `ConfigService`: `RunParserProcess`, `UpdateConfigFromSubscriptions` (пайплайн обновления кеша), `buildContextFromState`, обновление по источникам. Разбит с 1066 до ~538 строк; поднятие отпочковавшихся файлов до настоящих швов `SubscriptionFetcher` / `ConfigContextBuilder` пока отложено. |
| `rebuild.go` | `RebuildConfigIfDirty` — **единственный писатель `config.json`** (ADR-070-4); валидация через `sing-box check`; публикует `ConfigBuilt`; `cleanupLegacyOutboundsCache`. |
| `rebuild_raw_cache.go` | `buildSnapshotFromRawCache` — пересборка из `.raw`-тел без сети. |
| `auto_update.go` | Событийное авто-обновление по источникам (SPEC 052): цикл heartbeat, таймеры повторов, подписка на `VpnStateChanged`. |
| `log_level.go` | Headless-применение уровня логов (Load→мутация→Save). |
| `core_downloader.go` / `core_version.go` | Загрузка sing-box и версия (пин через `constants.RequiredCoreVersion`); проверка самообновления лаунчера. |
| `wintun_downloader.go` | Загрузка wintun.dll (Windows). |
| `template_migration.go` | `InvalidateTemplateIfStale` — удаление локального шаблона при апгрейде лаунчера. |
| `tray_menu.go` | Построение меню в системном трее. |
| `network_utils.go` | Общий HTTP-клиент, классификация сетевых ошибок и редакция URL. |
| `error_handler.go` | Единая точка вывода ошибок в UI. |
| `debugapi_wiring.go` | Разводка `ControllerFacade` для Debug API к `AppController`. |
| `main.go` | Точка входа: `NewAppController`, проверка устаревания шаблона, загрузка локали, инициализация UI, регистрация power-resume. |

---

## L4 — api / удалённое управление

### `api` — клиент Clash API

**Ответственность:** исходящий HTTP-клиент Clash API (список прокси / переключение / задержка). Разбит по зонам ответственности в SPEC 070.

| Файл | Назначение |
|------|---------|
| `clash_config.go` | `LoadClashAPIConfig` (базовый URL и токен из конфига). |
| `clash_transport.go` | Жизненный цикл HTTP-клиента и `ResetClashHTTPTransport` (после выхода из сна). |
| `clash_log.go` | Sink и файл `api.log`, простановка временных меток в `writeLog`. |
| `clash_error.go` | `classifyRequestError`/`normalizeRequestError` (сетевая ошибка → сообщение пользователю, с учётом сна). |
| `clash_proxy.go` | `GetProxiesInGroup` и `ProxyInfo` (сырое `Name` против нормализованного `DisplayName`, `DisplayOrName`). |
| `clash_switch.go` | `SwitchProxy` (PUT, экранированная в пути группа и имя в JSON). |
| `clash_delay.go` | `GetDelay`, `TestAPIConnection` и настройки URL и параллелизма пинг-тестов. |
| `clash.go` | Оставшиеся общие объявления и разводка пакета. |

### `core/debugapi` — входящий Debug HTTP API

**Ответственность:** интроспекция и управление приложением через интерфейс `ControllerFacade` (без конкретного импорта вверх).

| Файл | Назначение |
|------|---------|
| `server.go` | Каркас HTTP-сервера: Bearer-аутентификация, таблица маршрутов, фасад к `AppController`. |
| `state_endpoints.go` | Эндпоинты `/state/*` на чтение и атомарные мутации (правила, DNS-правила, уровень логов). |
| `settings_endpoints.go` | Чтение и запись `/settings/*`. |
| `traffic_endpoints.go` | `/traffic/*` (статус/live/сессии) поверх `internal/traffic` (SPEC 059). |
| `snapshot.go` | Эндпоинт `/snapshot` поверх `core/snapshot.Build`. |

---

## L5 — ui-presentation (MVP конфигуратора)

### `ui/configurator/models` — чистые данные

**Ответственность:** `WizardModel` (канонические Sources/GlobalOutbounds/Defaults), порядок слотов правил и DNS, состояние preset-ссылок и схема сохраняемого файла состояния. Без зависимостей от GUI.

| Файл | Назначение |
|------|---------|
| `wizard_model.go` | Центральная модель: Sources (канонические v5), GlobalOutbounds, Defaults; производные представления `ParserConfig`/JSON; `TemplateData`, `RuleOrder`, `DNSRuleOrder`. |
| `wizard_state_file.go` | Схема сохраняемого состояния v6 и миграции (`MigrateCustomRules`, `MigrateSelectableRuleStates`), `ValidateStateID` и `NewWizardStateFile`. |
| `rule_slot.go` / `dns_rule_slot.go` | Контейнеры порядка правил и DNS-правил (чередование строк пресетов и пользователя по порядку). |
| `rule_state.go` / `rule_state_utils.go` | Состояние правил маршрутизации и хелперы эффективного outbound'а. |
| `preset_ref_state.go` / `preset_ref_sync.go` | Жизненный цикл preset-ссылок (`SyncDNSByOrderToState`, `ReconcileDNSRuleOrder`, kind=preset против user). |
| `dns_state.go` / `dns_user_rule.go` / `dns_vars.go` | Состояние модели DNS, типизированная структура пользовательского правила (текст↔JSON) и DNS-переменные. |
| `wizard_settings_migrate.go` | Миграция переменных настроек из легаси `ConfigParams`. |

### `ui/configurator/business` — чистая логика (без Fyne)

**Ответственность:** разбор ParserConfig, схема DNS и outbound'ов, интерполяция шаблона, жизненный цикл состояния, валидация — за интерфейсом `UIUpdater`; никогда не импортирует Fyne или presentation.

| Файл | Назначение |
|------|---------|
| `parser.go` | `ParseAndPreview`, `ApplyURLToParserConfig` (классификация строк, построение ProxySource, восстановление префикса/постфикса тега). |
| `create_config.go` | `BuildPreviewConfig`/`BuildTemplateConfig`, конфиг маршрутов из модели, материализация секретов (предпросмотр против продакшена). |
| `wizard_dns.go` | Публичный API DNS (`ApplyWizardDNSTemplate`, `LoadPersistedWizardDNS`, `DNSTagLocked`, `DNSTagFromTemplate`). |
| `reconcilers.go` | Согласование DNS-серверов (вынесено из `wizard_dns.go`, стадия F SPEC 070). |
| `fillers.go` | Вспомогательное заполнение DNS и выбор дефолтов (вынесено, стадия F SPEC 070). |
| `validators.go` | Валидация DNS (`ValidateDNSModel`) (вынесено, стадия F SPEC 070). |
| `dns_helpers.go` | Вынесенные `parseTemplateDNSOptions`/`extractTemplateDNSTags` (убирают дубль из-за цикла импортов). |
| `template_helpers.go` | Объединённый `effectiveTemplate` (были `effectiveWizardConfig` + `effectiveTemplateConfig`). |
| `dns_settings_vars.go` / `preset_bundled_dns.go` | Скалярные DNS-переменные (strategy/final/resolver) и материализация DNS из пресетов. |
| `outbound.go` | `GetAvailableOutbounds` (с мемоизацией), `ResolveMergedOutbound`, принудительная простановка дефолтного тега. |
| `source_local_wizard.go` | Локальные групповые маркеры `WIZARD:` (auto/selector), флаги expose, переименование тегов при смене префикса. |
| `sources.go` | Хелперы списка источников. |
| `validator.go` | Валидация ввода (ParserConfig/URL/URI/outbound'ы/размер JSON). |
| `loader.go` | `LoadTemplateData`: чтение и валидация шаблона, применение params по GOOS, извлечение пресетов, возврат `TemplateData`. |
| `state_store.go` | Тонкая обёртка над `core/state.Save`/`Load` (атомарная). |
| `preview_cache.go` | Кеш outbound'ов предпросмотра и присвоение `SourceIndex`. |
| `interfaces.go` / `ui_updater.go` | Интерфейс `UIUpdater` (мост business↔presentation). |
| `config_service.go` / `template_loader.go` / `file_service_adapter.go` | Адаптеры к сервисам `core` (`ConfigService`, `TemplateLoader`, `FileService`). |

### `ui/configurator/presentation` — презентер MVP (оркестрация)

**Ответственность:** синхронизация GUI ↔ модель через `WizardPresenter`; диспетчеризация `fyne.Do`; оркестрация save/load и асинхронных операций.

| Файл | Назначение |
|------|---------|
| `presenter.go` | База `WizardPresenter`, `SafeFyneDo`, слоты дочерних окон и DI для пересоздания вкладки правил. |
| `gui_state.go` | Чистый контейнер состояния GUI (только виджеты Fyne), `ChildWindowsOverlay` и `RuleWidget`. |
| `presenter_sync.go` | Двунаправленная синхронизация модель↔GUI (`SyncModelToGUI`/`SyncGUIToModel`/`MergeGUIToModel`) с защитой от устаревших чтений. |
| `presenter_state.go` | `CreateStateFromModel`, `SaveCurrentState`/`SaveStateAs`, `LoadState` (восстановление в 9 шагов), синхронизация порядка правил и DNS. |
| `presenter_state_helpers.go` | Хелперы восстановления, вынесенные из `presenter_state.go` (стадия F SPEC 070). |
| `presenter_save.go` | Пайплайн сохранения (после SPEC 045 — только состояние): валидация → GUI→модель → `state.Save` → публикация `StateChanged` → авто-пересборка → диалог успеха. |
| `presenter_methods.go` | `SetSaveState`, `RefreshOutboundOptions` (с дебаунсом ~300 мс), `InitializeTemplateState`. |
| `presenter_async.go` | `TriggerParseForPreview`, `UpdateTemplatePreviewAsync`. |
| `presenter_rules.go` | Обновление вкладки правил (включая пересоздание после `LoadState` через DI). |
| `presenter_ui_updater.go` | Реализация `UIUpdater` (мост со стороны business). |
| `preset_ref_helpers.go` | `extractTemplateDNSTags` (различение DNS-тегов шаблона для `CreateStateFromModel`). |

### `ui/configurator/configurator.go` + `ui/configurator/utils`

- `configurator.go` — `ShowConfigWizard` entry: singleton check, model/GUI/presenter creation, template load, state.json→config.json fallback, window lifecycle.
- `utils/comparison.go`, `utils/constants.go`, `utils/truncate.go` — struct comparison, timeout/limit constants, text truncation.

---

## L6 — ui-views (вкладки / диалоги / корень)

### `ui` (корневые вкладки и основные представления)

**Ответственность:** корневая полоса вкладок и главные вкладки (Локально, Удалённые, диагностика, настройки, справка), резолвер удалённого Clash API и bootstrap профайлера трафика.

| Файл | Назначение |
|------|---------|
| `app.go` | Корневой контейнер вкладок (Локально / Удалённые / Диагностика / Настройки / Справка); подписан на `VpnStateChanged`. |
| `local_remote_tabs.go` | Двухколоночная композиция, общая для обеих главных вкладок: слева список прокси, справа управление (SPEC 098). |
| `core_dashboard_tab.go` | Дашборд ядра: статус sing-box, блоки версии/загрузки/wintun, статус конфига, селектор состояния, панель тостов подписок. |
| `core_dashboard_tab_helpers.go` / `_status.go` / `core_dashboard_subs_status.go` | Суб-билдеры дашборда, обновление статусов и панель статуса подписок (разбиение SPEC 070). |
| `clash_api_tab.go` | Список прокси, общий для обеих вкладок: сортировка/фильтрация/пинг, отслеживание активного прокси, дропдаун селектора, экспорт share-URI, пул балансировщика. (Всё ещё ~1701 строка; декомпозиция отложена.) |
| `clash_api_tab_helpers.go` / `_render.go` / `_autorefresh.go` | Хелперы списка прокси, отрисовка строк, цикл авто-обновления. |
| `clash_remote.go` | Резолвер эндпоинта удалённого Clash API (SPEC 064). |
| `lxd_remote_override.go` | Резолвинг remote-override с учётом области, чтобы Локально продолжало говорить с локальным ядром, а Удалённые следовали за выбранной машиной. |
| `machine_list_panel.go` | Правая колонка вкладки Удалённые: по строке на машину (имя, платформа, адрес, состояние ядра) с кнопками «Настроить» / Start-Stop / Deploy / правка / удаление и блоком «Ещё». |
| `machine_add_window.go` | Окно добавления машины (вставка приглашения, сопряжение). |
| `connection_window.go` / `connection_local.go` / `connection_local_daemon_darwin.go` / `_stub.go` | Окно настроек подключения: вкладка **Remote** — Clash-override SPEC 064, вкладка **Local** — движок ядра (радио Process / Daemon, команды установки и сопряжения). |
| `command_row.go` (+ `_darwin.go`) | Строка, отдающая привилегированную команду пользователю — копировать или открыть в Терминале, без собственного привилегированного запуска. |
| `machine_profiler.go` | Экземпляр профайлера трафика и окно на машину (SPEC 099); по одному на машину, гаснет вместе с каналом. |
| `machine_host_window.go` / `machine_host_format.go` | Окно телеметрии хоста (CPU, load, память, температура, файловые дескрипторы, диски, интерфейсы) с табличным форматированием фиксированной ширины. |
| `machine_resources_window.go` | Ресурсное хранилище машины (rule-set'ы, тела подписок). |
| `servers_node_subtitle.go` / `servers_node_info.go` / `servers_row_layout.go` | Подзаголовок узла, окно информации и раскладка строки (SPEC 095). |
| `diagnostics_tab.go` | Тесты STUN/DNS, аварийное завершение sing-box, сохранение настроек. |
| `settings_tab.go` / `settings_window.go` | UI настроек (язык, уровень логов, …) в отдельном окне. |
| `help_tab.go` | Вкладка справки. |
| `log_viewer_window.go` | Встроенный вьюер логов (читает sink debuglog). |
| `dialogs.go` | Общие диалоги (`ShowError`/`ShowInfo`/`ShowConfirm`/`ShowCustom`) с `fyne.Do`. |
| `traffic_bootstrap.go` / `traffic_verbose.go` | Разводка и открытие окна профайлера трафика плюс тумблер verbose. |
| `wizard_overlay.go` | Константа `wizardOverlayEnabled` и переключение кликового оверлея главного окна. |

### `ui/configurator/tabs`

**Ответственность:** представления вкладок визарда (Target / Sources / Outbounds / Rules / DNS / Settings / Preview) и окно правки источника.

| Файл | Назначение |
|------|---------|
| `source_tab.go` | Вкладка источников: ввод URL, список источников, запуск окна предпросмотра всех узлов. |
| `source_edit_window.go` + `source_edit_overview.go` / `_raw.go` / `_misc.go` | Окно правки источника (настройки / предпросмотр / сырой JSON; маркеры exclude и expose). |
| `source_meta_format.go` / `source_support_link.go` / `source_error_dialog.go` | Форматирование метаданных источника, ссылка на поддержку и веб-страницу, диалог ошибки. |
| `rules_tab.go` / `rules_unified_rows.go` | Список правил маршрутизации (добавление/правка/удаление, авто-загрузка SRS, выбор outbound'а на правило). |
| `dns_tab.go` / `dns_unified_rules.go` / `dns_user_rules.go` / `dns_preset_bundled.go` | DNS-серверы и единый редактор правил (пресетные и пользовательские). |
| `settings_tab.go` + `settings_tun_darwin.go` / `settings_tun_stub.go` | Настройки переменных шаблона; привилегированная чистка при выключении TUN на darwin. |
| `preset_ref_edit_dialog.go` / `preset_ref_convert.go` / `preset_ref_srs.go` | Правка, конвертация и обработка SRS для preset-ссылок. |
| `library_rules_dialog.go` | Пикер библиотеки пресетов шаблона (Add selected → CustomRules). |
| `preview_tab.go` | Вкладка предпросмотра конфига. |
| `tight_vbox.go` / `tight_hbox.go` | Компактные хелперы раскладки vbox/hbox (`tight_hbox.go` пакует иконки строки с отрицательным зазором `rowIconGap`). |
| `row_scaffold.go` | Общий каркас строки для переупорядочиваемых списков Rules/DNS/Sources: `buildRowLeftLead` (↑/↓ и чекбокс), `buildRowEditDelCluster` (правка/удаление). |

### `ui/configurator/dialogs`

| Файл | Назначение |
|------|---------|
| `add_rule_dialog.go` | Модальное окно добавления и правки правила маршрутизации (форма и вкладка сырого JSON, пикер процессов, список SRS-URL). (Всё ещё ~1154 строки; декомпозиция отложена.) |
| `rule_dialog.go` / `rule_type_selection.go` | Пикер типа правила и хелперы. |
| `load_state_dialog.go` / `save_state_dialog.go` | Диалоги загрузки и сохранения состояния визарда. |
| `get_free_dialog.go` | Диалог импорта бесплатного VPN-конфига. |
| `srs_tag.go` | Хелпер SRS-тегов для диалогов правил. |

### `ui/configurator/outbounds_configurator`

| Файл | Назначение |
|------|---------|
| `configurator.go` / `configurator_helpers.go` | UI списка outbound'ов (добавление/правка/удаление/сброс, глобальная область против источника, значок preset-ссылки). |
| `edit_dialog.go` / `edit_dialog_helpers.go` | Окно создания и правки outbound'а (тег/тип/комментарий, опции urltest, селектор области, слияние ref и пресета). (Всё ещё ~1095 строк; декомпозиция отложена.) |
| `flag_picker.go` | Пикер флагов стран. |

### `ui/traffic`

| Файл | Назначение |
|------|---------|
| `window.go` | Координатор окна профайлера трафика (раскладка, вкладки live и по процессам, тулбар). |
| `live_view.go` | Список пакетов (пауза/продолжение, сортировка, клик к деталям). |
| `per_process_view.go` | Агрегация с группировкой по процессам. |
| `event_detail.go` | Развёрнутая панель деталей события. |
| `process_picker.go` / `toolbar.go` | Выбор процесса, фильтры и сортировка тулбара. |

---

## L7 — ui-виджеты / ассеты

### `internal/fynewidget`

**Ответственность:** переиспользуемые самодостаточные строительные блоки Fyne. Без внутренних зависимостей.

| Файл | Назначение |
|------|---------|
| `hover_row.go` | `HoverRow`: фон при наведении и опциональная подсветка выбора; `WireTooltipLabelHover`. |
| `check_with_content.go` | `CheckWithContent`: чекбокс и контент, переключающий его по нажатию. |
| `hover_forward.go` | `HoverForwardButton`/`Select`/`TTButton`: проброс наведения с дочернего элемента на родительскую строку. |
| `tap_wrap.go` / `secondary_tap_wrap.go` | Обёртки обычного и контекстного нажатия с захватом модификаторов. |
| `tooltip.go` | Хелпер `SetToolTipSafe` (сведённое воедино приведение типа тултипа, дедупликация SPEC 070). |
| `doc.go` | Документация пакета: правила встраивания, паттерны проброса наведения. |

### `ui/icons`

**Ответственность:** встроенные ресурсы SVG-иконок.
- `icons.go` — `bolt.svg` (Core tab), `telegram.svg`, `link.svg` as Fyne static resources.

### `ui/components`

**Ответственность:** общие компоненты UI.
- `scroll_gutter.go` — `NewScrollGutter` / `WrapInScrollWithGutter` (scrollbar spacing).
- `click_redirect.go` — оверлей `ClickRedirect`, пробрасывающий клики в окно визарда для подъёма фокуса. Принимает `*uiservice.UIService` (листовой пакет) — нарушение слоистости V1 устранено.
