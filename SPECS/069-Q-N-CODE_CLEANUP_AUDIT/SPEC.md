# Аудит уборки кода — singbox-launcher

**Дата:** 2026-06-08
**Проект:** singbox-launcher
**Ветка:** develop
**Метод:** трёхпроходный анализ (module-level чтение + статический анализ staticcheck/deadcode/gofmt + поиск дубликатов), затем ручная верификация каждой находки. Опровергнутые отброшены.

---

## 1. Резюме

Кодовая база в целом здоровая: бизнес-логика рабочая, тесты присутствуют, критических багов почти нет. Аудит подтвердил **304 находки** (223 confirmed + 81 needs-judgment) после дедупликации пересечений между module/static/duplication-проходами. Доминирующая тема — **накопленный долг от миграций** (SPEC 045/047/053/055/056/057/058/060/063/064/067/068): мёртвый код от удалённых пайплайнов, осиротевшие test-only функции, tombstone-комментарии для удалённых символов и устаревшие doc-комментарии/доки, ссылающиеся на несуществующие файлы и функции. Вторая крупная тема — **дублирование** (base64-декодеры ×3, network-error mapping ×4, varsMap-каскад ×3, shallow-map-clone ×4, presetDisplayLabel/evalIf/SRS-tag в нескольких слоях), частично уже разошедшееся (флаг `flow` в селекторах, fetch-пайплайн `FetchSubscription`). Severity почти везде **low**; есть несколько **medium** (data race на `apiLogFile`, leak флага `AutoLoadInProgress`, silent-accept усечённых JSON-тел, утечка urltest-ключей в selector-конфиг, nil-deref в config_service) и одна **high** (дублированный SRS-cache-key, риск рассинхрона ключей кэша). Основная масса — дешёвые быстрые победы: удаление мёртвого кода, чистка комментариев и `gofmt`.

---

## 2. Сводная таблица

### По dimension (confirmed)

| Dimension | Кол-во |
|---|---:|
| dead-code | 78 |
| code-smell | 50 |
| divergent-logic | 30 |
| redundant-comment | 24 |
| doc-mismatch | 20 |
| unnecessary-check | 8 |
| bottleneck | 6 |
| premature-opt | 5 |
| other | 2 |
| **Итого confirmed** | **223** |

> Примечание: ряд static-analysis находок (`module: static-analysis`/`duplication`) перекрывает module-level находки (BuildRulesAndDNS, дубль-импорт core/state, ShowErrorBanner, comparison.go и т.д.) — в разделах ниже они сведены в одну запись.

### По severity (confirmed)

| Severity | Кол-во |
|---|---:|
| high | 1 |
| medium | 24 |
| low | 198 |

### Needs-judgment

| Severity | Кол-во |
|---|---:|
| medium | 7 |
| low | 74 |
| **Итого** | **81** |

### Quick wins (effort ∈ {trivial, small} И confidence = high)

Подавляющее большинство confirmed-находок — quick wins (~190). Самые ценные (severity ≥ medium, effort ≤ small, conf=high):

| Location | Суть | Sev |
|---|---|---|
| core/config/subscription/decoder.go (+srs_tag.go) | Дублированный SRS-cache-key — риск cache-miss | high |
| api/clash.go:194,203,226 | data race на `apiLogFile` при shutdown | medium |
| core/services/api_service.go:315-318 | leak `AutoLoadInProgress` → авто-загрузка навсегда стопится | medium |
| core/debugapi/traffic_endpoints.go:375 | усечённый JSON-body молча принимается как пустой | medium |
| ui/configurator/outbounds_configurator/edit_dialog.go:387-400 | urltest-ключи протекают в selector → невалидный config.json | medium |
| core/config_service.go:378 | nil-deref `ac.FileService` (SA5011) | medium |
| core/services/api_service.go:257 | lockless чтение `apiSvc.Enabled` (data race) | medium |
| ui/configurator/tabs/source_tab.go:281-294 | toggle/delete источника не зажигает Save | medium |
| api/clash.go:280-575 | network-error mapping ×4 | medium |
| internal/outboundutil + ui rule_utils.go | `ApplyOutboundToRule` дублирован | medium |
| core/config/outbound_filter.go:102-119 | селектор-фильтры игнорируют `flow` | medium |

---

## 3. Приоритизированный план уборки

Порядок: высокий риск × низкая стоимость → структурный дубль-долг → косметика оптом.

**Этап 1 — корректность/безопасность (medium+, trivial/small). Брать первым.**
1. **SRS-cache-key (high)** — вынести `internal/srstag.TagFromURL`, заменить обе копии (`preset_merge.go`, `srs_tag.go`). Единственная high-находка; влияет на on-disk ключи кэша.
2. **api/clash.go `apiLogFile` race** — взять под `apiLogSinkMu` в `SetAPILogFile`/`writeLog`.
3. **`AutoLoadInProgress` leak** — обернуть тело горутины в `defer`-сброс флага под мьютексом.
4. **decodeJSONBody EOF-swallow** — заменить `strings.Contains(err,"EOF")` на `errors.Is(err, io.EOF)`.
5. **edit_dialog urltest→selector** — дропать type-несовместимые ключи (url/interval/tolerance) при копировании Options.
6. **config_service nil-deref (SA5011)** — вычислить execDir в локальную под существующим nil-guard.
7. **`apiSvc.Enabled` lockless read** — читать через `GetClashAPIConfig()`.
8. **source_tab toggle/delete не зажигает Save** — добавить `MarkAsChanged()` (и в delete-handler).

**Этап 2 — крупные мёртвые кластеры (medium, medium-effort).** Удалить целиком, аккуратно сохранив живых соседей:
- **`BuildRulesAndDNS` + helpers** (rules_pipeline.go) — релокировать `ParseTemplateDNSDefaults`/`ValidateTemplateDNSServers`/`TemplateDNSServer` в новый файл, удалить остальное + тесты. Это закрывает сразу несколько находок (dead-code, divergent-logic «два эмиттера DNS/route», `SanitizeServerForEmit`).
- **`migrator.go` v1→v4 chain + `ExtractParserConfig/Block` + factory** — весь кластер мёртв вместе.
- **`core/state/diff.go` + `legacy_v4.go` + v5→v6 helpers + `state.New`** — осиротели после SPEC 060.
- **`ResolveOutbounds` + helpers** (resolve_outbounds.go) — test-only.

**Этап 3 — медиум-дубли (small).** network-error mapping ×4; `ApplyOutboundToRule`/`applyRouteOutbound`; `FetchSubscription` → тонкая обёртка; flow-case в селектор-фильтрах; legacy helper-пары adapter.go↔legacy_migration.go.

**Этап 4 — косметика оптом (trivial).**
- `gofmt -w` по всему дереву (~25 файлов; CI сейчас не гейтит, но единоразово).
- Снос tombstone-комментариев и компилятор-guard'ов (`var _ = …`).
- Чистка осиротевших doc-комментариев и docs/*.md рассинхронов.
- Удаление мелких dead-полей/констант/обёрток (substitute.go wrappers, дубль-импорт core/state, и т.д.).

---

## 4. Находки по dimension

### 4.1 dead-code

| Location | Суть | Cleanup | Sev | Conf | Effort |
|---|---|---|---|---|---|
| core/config_service.go:144-147 | `ConfigService.GenerateNodeJSON` — обёртка без вызовов | Удалить метод + doc | low | high | trivial |
| core/core_version.go:54-62 | `AppController.GetCoreBinaryPath` неиспользуем | Удалить | low | high | trivial |
| core/controller.go:391-419 | `AppController.RunHidden` неиспользуем (+stale childLogFileName ветка) | Удалить | low | high | trivial |
| core/network_utils.go:19-20 | `NetworkLongTimeout` const без читателей | Удалить или подключить к download | low | high | trivial |
| core/build/rules_pipeline.go:42-277,300-336 | `BuildRulesAndDNS` + `mergeRuleSets`/`mapsEqual`/`normalizeMatch` — test-only параллельный DNS/route пайплайн | Удалить; релокировать живые `ParseTemplateDNSDefaults`/`ValidateTemplateDNSServers`/`TemplateDNSServer` | medium | high | medium |
| core/build/preset_outbounds.go:60-79 | `PresetOutboundAddByTag` экспортирован, без вызовов | Удалить | low | high | trivial |
| core/build/resolve_route.go:12,292-293 | `encoding/json` только ради `var _ = json.Unmarshal` | Удалить blank-assign + import | low | high | trivial |
| core/build/sections.go:159-165 | `appendStaticEntries` параметр `hasDynamic` игнорируется (`_ = hasDynamic`) | Дропнуть параметр + 2 call-site | low | high | trivial |
| core/build/preset_merge.go:518-526 | `hasAnyPresetRef` «для совместимости» — только тесты | Удалить, тесты свернуть в `hasAnyV6Rule` | low | high | trivial |
| core/build/rules_pipeline.go:412-423 | `SanitizeServerForEmit` — test-only, stale godoc | Удалить + тест | low | high | trivial |
| core/state/migration_v5_to_v6.go:34-66,14-26 | `migrateV5ToV6` + `migrateWarning` — test-only | Удалить, переписать тесты на прямые вызовы | medium | high | small |
| core/state/migration_v5_to_v6.go:239-270 | `isV6`/`isV5`/`isLikelyLegacyLabel` без prod-вызовов | Удалить `isLikelyLegacyLabel`; `isV5/isV6` → test-helper | low | high | small |
| core/state/dns_options.go:258-294 | `FindServerByTag`/`FindServerByRef`/`FindRuleByRef` — мёртвый экспорт | Удалить (есть unexported аналоги в sync_dns.go) | low | high | trivial |
| core/state/state.go:150-177 | `GetSubscriptionSources`/`GetServerSources` без вызовов | Удалить; `FindSource` оставить | low | high | trivial |
| core/state/adapter.go:189-192 | no-op `if frag=="" { frag="" }` | Удалить | low | high | trivial |
| core/state/legacy_migration.go:93-95 | no-op `if … == nil { … = nil }` | Удалить блок | low | high | trivial |
| core/template/substitute.go:102-104,247-251,276-281 | `substituteWalk`/`handleIfMapSpread`/`handleIfArrayElement` — nil-sink обёртки без вызовов (живут `*Ctx`-варианты) | Удалить три обёртки | low | high | trivial |
| core/template/vars_resolve.go:215-218 | `ResolvedVar.IsList()` неиспользуем (эвристика расходится — НЕ роутить) | Удалить | low | high | trivial |
| core/template/loader.go:74-76,321 | `TemplateData.PresetWarnings` пишется, не читается (warnings уже логируются 305-307) | Дропнуть поле + doc | low | high | small |
| core/template/loader.go:164-168 | `TemplateSelectableRule.Platforms`/`.IsDefault` мертвы | Удалить поля + write на add_rule_dialog.go:825 | low | high | small |
| core/config/outbound_share.go:53-62 | `GetEndpointMapByTag` — только свой тест | Удалить (или unexport) | low | high | trivial |
| core/debugapi/traffic_endpoints.go:375,383-387 | `errEmptyBody` sentinel никогда не возвращается — `errors.Is` всегда false | Удалить тип+метод+ветку; детектить `io.EOF` | low | high | trivial |
| core/debugapi/server.go:52, core/debugapi_wiring.go:49-54 | `ControllerFacade.GetConfigPath` объявлен/реализован, без вызовов | Удалить из iface+impl+fake | low | high | trivial |
| core/events/events.go:52,69,63; payloads.go:39,57 | `SubscriptionUpdated`/`AutoUpdateStatus`/`PowerResume` — не публикуются/не подписаны (SPEC 047 scaffolding) | Удалить kinds+payloads (или подключить) | low | high | small |
| core/uiservice/ui_service.go:37,51,52 | `TrayIcon`/`ParserProgressBar`/`ParserStatusLabel` — dead-поля | Удалить три поля | low | high | trivial |
| core/services/srs_downloader.go:143-146 | `AllSRSDownloaded` обёртка без вызовов (живёт `…ForEntries`) | Удалить | low | high | trivial |
| ui/clash_remote_ui.go:226-230 | dead scratch в Connect (portRaw/portInt парсят не те поля и отбрасываются; SA4006) | Удалить строки | medium | high | trivial |
| ui/clash_remote_ui.go:302-304 | `var _ = api.TestAPIConnection` anchor — dead import | Удалить var + import | low | high | trivial |
| ui/core_dashboard_tab.go:65,360-363,376-377 | `parserStatusLabel` создаётся/драйвится, но не добавлен ни в один контейнер | Удалить поле+alloc+Show/SetText | low | high | small |
| ui/dialogs.go:48-56 | `ShowErrorBanner` без вызовов (SPEC 068 убрал ErrorBanner) | Удалить | low | high | trivial |
| ui/configurator/tabs/source_tab.go:260,269,277-279,414-416 | shadowing-баг: внешний `prefixLabel` всегда nil → dimming/tooltip-hover мёртвы | Поднять одну `var`, присваивать `=` внутри | medium | high | trivial |
| ui/configurator/tabs/dns_user_rules.go:444-453 | `collectAllRuleSetTags` CustomRules-цикл ничего не делает (`_ = cr`) | Удалить цикл + поправить doc | low | high | trivial |
| ui/configurator/tabs/dns_user_rules.go:509 | `_ = sort.SliceStable` при уже используемом `sort.Strings` | Удалить строку | low | high | trivial |
| ui/configurator/tabs/source_edit_overview.go:405-406 | `var _ = strings.Builder{}` при живом `strings` | Удалить строки | low | high | trivial |
| ui/configurator/tabs/source_meta_format.go:97-108 | `formatNodesCount` truncated-ветка недостижима (вызов всегда `,0`) | Дропнуть параметр + ветку | low | high | trivial |
| ui/configurator/business/loader.go:60-116 | `CloneOutbound`+`deepCopyValue` неиспользуемы | Удалить | low | high | trivial |
| ui/configurator/business/rule_utils.go:52-61 | `FormatRuleAsJSON` неиспользуем | Удалить + строку doc | low | high | trivial |
| ui/configurator/business/sources.go:139-143 | `ClassifyInput` экспорт без вызовов; комментарий называет несуществующую функцию | Удалить | low | high | trivial |
| ui/configurator/business/state_store.go:183-186,264-280 | dead if-блок (перетёрт безусловным `md.ID=id`); `parseMetadataAny.ID` не наблюдается | Удалить if-блок; опц. дропнуть ID-probe | low | high | trivial |
| ui/configurator/presentation/presenter_ui_updater.go:50-75, business/ui_updater.go:24-25 | `UpdateTemplatePreview` (метод iface+impl) без prod-вызовов | Удалить из impl/iface + test-stub | medium | high | trivial |
| ui/configurator/presentation/presenter_sync.go:328-341 | `MergeGUIToModelFromMainThread` — сирота удалённого `ensureOutboundsParsed` | Удалить | medium | high | trivial |
| ui/configurator/presentation/presenter_state.go:196-208 | `extractConfigParams` всегда возвращает пустой слайс | Инлайнить + удалить метод | low | high | trivial |
| ui/configurator/dialogs/rule_dialog.go:34-40 | `RuleType*Label` константы неиспользуемы | Удалить блок + упоминание в doc | low | high | trivial |
| ui/configurator/dialogs/rule_dialog.go:60-74 | `ParseLines` `preserveOriginal=true` ветка недостижима (6 вызовов = false) | Дропнуть параметр+ветку, обновить вызовы | low | high | small |
| ui/configurator/outbounds_configurator/configurator.go:53,146 | `outboundRow.PresetID` пишется, не читается | Удалить поле + assign | low | high | trivial |
| ui/icons/icons.go:1, bolt.svg | весь пакет `ui/icons` не импортирован | Удалить пакет + SVG | low | high | trivial |
| ui/configurator/utils/comparison.go:26,58,71,88 | весь файл (`OutboundsMatchStrict`/`StringSlicesEqual`/`MapsEqual`/`ValuesEqual`) мёртв | Удалить файл | low | high | trivial |
| ui/configurator/utils/constants.go:23-77 | 13 неиспользуемых констант (5 живых оставить) | Удалить 13 + поправить doc | low | high | small |
| ui/configurator/models/wizard_model.go:94,179 (+presenter writes) | `SelectableRuleStates` write-only | Дропнуть поле + make() + 3×`=nil` | low | high | small |
| ui/configurator/models/wizard_state_file.go:34,129-136 | `ToPersistedSelectableRuleState` + alias `PersistedSelectableRuleState` мертвы | Удалить функцию + alias | low | high | trivial |
| ui/configurator/models/preset_ref_sync.go:256-265 | `SyncDNSFullToStateV6` только из тестов | Удалить, тест на реальный путь | low | high | small |
| internal/platform/power_*.go (4 файла) | `RegisterSleepCallback` + sleep-callback fan-out мертвы (sleepingFlag/cancel — живы) | Дропнуть callback-механику, СОХРАНИТЬ sleepingFlag/cancel | low | high | small |
| internal/platform/platform_{windows,darwin,linux}.go | `GetBuildFlags` экспортирован ×3, без вызовов | Удалить ×3 | low | high | trivial |
| internal/platform/wintun_cleanup_windows.go:93,108 | `digcfPresent`/`spdrpFriendlyName` объявлены, не используются | Удалить, текст в комментарий у call-site | low | high | trivial |
| internal/platform/power_windows.go:160 | `_ = windows.HWND(hwnd)` no-op | Удалить строку | low | high | trivial |
| internal/traffic/clash_connections.go:68-88,205-211 + profiler.go:149-153 | `ConnDelta.Bytes`/`ClashConnBytesDelta` считаются, не потребляются (только len() в логе) | Дропнуть тип+вычисление, убрать count из лога | low | high | small |
| internal/traffic/parser.go:60-62,147-152 + profiler.go:304-307 | `EventRouterMatch`/`LogLine.Outbound` парсятся, не используются; stale-комментарий | Удалить парс-путь+поле (или подключить) | medium | high | small |
| internal/traffic/parser.go:64-66,155-162 + profiler.go:308-311 | `reInboundOut` вывод (Port/IP) не потребляется (default→nil) | Удалить или превратить в enrichment | low | high | small |
| internal/traffic/session.go:25 | `Session.VerboseToggleTimes` без записей/чтений | Удалить поле | low | high | trivial |
| internal/traffic/clash_connections.go:33 | `ClashConnMeta.Type` декодится, не читается | Удалить поле | low | high | trivial |
| internal/traffic/clash_connections.go:25 | `ClashConn.RulePayload` декодится, не читается | Удалить (или surface в detail) | low | high | trivial |
| internal/traffic/clash_connections.go:124-129 | `ConnPoller.SetInterval` без вызовов | Удалить | low | high | trivial |
| ui/traffic/window.go:35-38, ui/traffic_bootstrap.go:102-104 | `WindowDeps.ConfigWriter` подключён, не читается | Удалить поле + assign | low | high | trivial |
| ui/traffic/live_view.go:47,299 | `liveFilter.Process` читается в `passes()`, нигде не присваивается | Удалить поле+ветку, поправить doc | low | high | trivial |
| ui/traffic/window.go:19,281-282 | `var _ = widget.NewLabel` + единственный держатель import | Удалить var+comment, затем import | low | high | trivial |
| internal/constants/constants.go:8,10,37,42,43 | 5 неиспользуемых констант | Удалить | low | high | trivial |
| internal/textnorm/stripansi.go:15 | `StripANSI` (+2 regexp) test-only | Удалить файл+тест (или подключить) | low | high | trivial |
| internal/debuglog/debuglog.go:146 | `LogTextFragment` без вызовов | Удалить + doc-пример | low | high | trivial |
| core/config/parser/migrator.go (v1→v4) + block_extractor + factory.ExtractParserConfig | весь migrator/extractor/factory кластер недостижим (deadcode) | Удалить функции + осиротевшие тесты | medium | high | medium |
| core/state/diff.go + legacy_v4.go + migration_v5_to_v6 helpers + state.New | `DiffStates` семейство, `parseV4File`, `state.New` недостижимы (deadcode) | Удалить файлы/функции + тесты | medium | high | medium |
| core/build/resolve_outbounds.go:108,159,171 | `ResolveOutbounds`/`classifySource`/`summarizeUpdates` test-only | Удалить + тесты | medium | high | medium |
| assorted (build.go:111, parsed_cache.go:29, file_service.go:209/284, raw_cache.go:136/149, traffic profiler.go:112 …) | `HasErrors`/`IsEmpty`/`BackupPath`/`BackupFile`/`DeleteRawBody`/`ListRawBodyIDs`/`TrafficProfiler.Stop` и др. недостижимы (deadcode) | Удалить (Stop ≠ wiring: живёт StopSession) | low | high | medium |
| ui (app.go:213/228/233, validator.go family, CreateSourceTab, comparison.go, ClassifyInput …) | крупнейший dead-кластер UI-хелперов (deadcode) | Удалить подтверждённо-мёртвые | medium | high | medium |

### 4.2 divergent-logic

| Location | Суть | Cleanup | Sev | Conf | Effort |
|---|---|---|---|---|---|
| core/core_downloader.go:197-241,243-280 | platform→asset-suffix матрица дублирована в `buildSourceForgeAssets` и `SingboxAssetSuffix` | `buildSourceForgeAssets` строит имя через `SingboxAssetSuffix()` | low | high | small |
| core/build/rules_pipeline.go:78-274 vs preset_merge/resolve_dns/route | два эмиттера preset DNS/route — **= dead-code находка rules_pipeline**, не отдельная работа | Решается удалением `BuildRulesAndDNS` | low | high | medium |
| core/build/preset_merge.go:446-460 vs preset_expand.go:466-482 | DNS-match whitelist (cleanDangling) vs blacklist (isDNSRuleEmpty) — расходятся на новых ключах | Извлечь один `isDNSRuleMatchEmpty(m)` | low | medium | small |
| core/config/subscription/decoder.go:15-37, node_parser.go:379-397, meta.go:381-404 | три base64 multi-variant декодера | `decodeBase64Any`; делегировать; meta как validating-wrapper | low | high | small |
| core/config/subscription/fetcher.go:343-419 vs 226-299 | deprecated `FetchSubscription` дублирует весь fetch-пайплайн `…WithMeta`; уже разошлись (announce) | Сделать тонкой обёрткой или мигрировать source_loader.go:123 | medium | high | small |
| core/config/subscription/fetcher.go:348-358 | inline client вместо `newHTTPClient()` | `client := newHTTPClient()` | low | high | trivial |
| core/config/subscription/node_parser_vmess.go:254-278,280-302 | два почти-идентичных TLS-блока (tls vs h2), даже стиль расходится | Один helper для sni/alpn/fp/insecure | low | high | small |
| core/config/outbound_filter.go:102-119 vs node_parser.go:502-521 | селектор-`getNodeValue` без `case "flow"` (skip-фильтры имеют) → `flow`-фильтр исключает всё | Добавить `case "flow"`; лучше — общий helper в configtypes | medium | high | small |
| core/config/outbound_filter.go:122-155 vs node_parser.go:524-557 | `matchesPattern` дублирован byte-for-byte (+перекомпиляция regexp) | Перенести в configtypes, делегировать | low | high | small |
| core/state/adapter.go:177-228 vs legacy_migration.go:138-186 | три legacy helper-пары (extractFragment/buildTagSpec/serverLabel) | Оставить по одной копии, удалить дубль adapter.go | medium | high | small |
| ui/log_viewer_window.go:120-177,179-233 | internal/api-log List ~95 строк copy-paste | `buildLevelFilteredLogList(...)` ×2 | medium | high | medium |
| ui/configurator/business/parser.go:189-214 vs sources.go:145-162 | `classifyInputLines` дублирован как `classifyInputLinesV2` (самоназван «копия») | Свернуть; после удаления `ClassifyInput` — `V2` тоже мёртв | low | high | small |
| ui/configurator/business/create_config.go:51-71 vs presentation/preset_ref_helpers.go:18-37 | `extractTemplateDNSTagsLocal` дублирует `extractTemplateDNSTags` | Вынести в общий нижний слой | low | high | small |
| ui/configurator/dialogs/load_state_dialog.go:66-93 vs 137-158 | sort+filename-building дублированы (setup vs refresh) | `sortAndFormatStates(states)` | low | high | small |
| ui/configurator/dialogs/add_rule_dialog.go:242-260 vs 1060-1075 | domain/process extraction дублирован edit-load vs syncRawToForm; edit читает params, raw — нет | `applyDomain/ProcessFieldsFromRule` + params-override | medium | high | medium |
| ui/configurator/outbounds_configurator/edit_dialog.go:387-400 | wholesale-копия `displayBody.Options` протекает urltest-ключами в selector → невалидный config.json | Дропать type-несовместимые ключи | medium | high | small |
| internal/outboundutil/outbound.go:22 vs ui/.../rule_utils.go:24 | `ApplyOutboundToRule` дублирован (single-source-of-truth пакет существует) | UI-копия → clone+делегат | medium | high | small |
| internal/traffic/event_detail.go:24-37 vs per_process_view.go:369-382 | два «find Traffic Profiler window» helper'а, расходятся на untitled | Один общий helper | low | high | small |
| ui/traffic/per_process_view.go:185-191 vs 241-248 | sort-by-bytes дублирован (saved-open vs refresh) | `sortByBytes`/`applyAggregates` helper | low | high | small |
| api/clash.go:280-289,369-378,514-523,566-575 | network-error mapping блок ×4 byte-for-byte | `classifyRequestError(err, fallbackMsg)` | medium | high | small |
| core/build/route_merge.go:158-175 vs outboundutil:22-43 | `applyRouteOutbound` дублирует `ApplyOutboundToRule` | Заменить на `outboundutil.ApplyOutboundToRule` | medium | high | trivial |
| core/build/resolve_dns.go:542; ui outbound.go:222; preset_ref_srs.go:63; preset_ref_edit_dialog.go:90 | `evalIf` (if/if_or предикат) дублирован ×4 | Экспортировать предикат в leaf-пакет | medium | high | small |
| core/build/preset_expand.go:88-104, preset_outbounds.go:104-120, resolve_dns.go:461-477 | varsMap-каскад (defaults+overrides+filter) дублирован ×3 (`buildPresetVarsMap` уже извлечён) | Вызывать `buildPresetVarsMap` из двух остальных | low | high | small |
| core/state/load.go:298; rule_utils.go:65; dns_user_rules.go:389; route_merge.go:257 | 4 идентичных shallow-map-clone (+ десятки inline) | `maps.Clone` / один helper | low | high | medium |
| core/build/resolve_dns.go:573; dns_preset_bundled.go:104 (+2 inline) | `presetDisplayLabel` (Label-or-ID) дублирован build/UI | `Preset.DisplayLabel()` метод | low | high | trivial |

### 4.3 redundant-comment

| Location | Суть | Cleanup | Sev | Conf | Effort |
|---|---|---|---|---|---|
| core/build/rules_pipeline.go:412 + dns_merge.go:156 | `SanitizeServerForEmit` (узкий, dead) дублирует `stripDNSWizardOnlyFields` (живой) | Удалить `SanitizeServerForEmit` + тест | low | high | trivial |
| core/config_service.go:933-938 | комментарий обещает MarkCacheStale, код вызывает только MarkConfigStale | Поправить комментарий | low | high | trivial |
| core/build/rebuild_raw_cache.go:28-32 | doc ссылается на удалённую `ApplyPresetOutboundsToParserConfig` | Описать текущий Migrate/Sync/Merge поток | low | high | trivial |
| core/build (resolve_dns:368, rules_pipeline:292, preset_expand:484, dns_merge:32/105, sync_dns:14/24/60) | tombstone-комментарии «УДАЛЕНА/удалён» | Снести; оставить 1 строку у живых defensive `delete()` | low | high | trivial |
| core/config/subscription/node_parser.go:375-378 | doc «uses shared tryDecodeBase64 from core» — на деле локальная копия, core-версии нет | Удалить/поправить (свернуть в дедуп декодеров) | low | high | trivial |
| core/template/loader.go:74-76 | doc `PresetWarnings` «логируются callsite'ом» — на деле в-loader 305-307 | Поправить (или resolve удалением поля) | low | high | trivial |
| core/state/diff.go:244-245, load.go:287-295, migration_v5_to_v6.go:234-236 | tombstone-блоки (boolPtrEqual/legacyDNSOptionsFromV6/generateMigrationULID) | Снести; макс 1 строка против re-intro | low | high | trivial |
| ui/help_tab.go:68-75 | обе ветки if/else вызывают одно и то же; else-return = no-op | Один вызов `updateLauncherVersionInfo()` | low | high | trivial |
| ui/configurator/tabs/dns_tab.go:471,564-565 | оторванные/смещённые doc-комментарии | Передвинуть к реальным декларациям | low | high | trivial |
| ui/configurator/business/wizard_dns.go:240-241,633-635,155 | tombstone `mergeLockedRow` + stale `см. mergeLockedRow` + ignored `dnsObj` | Снести, опц. дропнуть `dnsObj` | low | high | trivial |
| ui/configurator/business/parser.go:184 | закомментированная dead-строка `// go UpdateTemplatePreviewAsync` | Удалить | low | high | trivial |
| ui/configurator/presentation/presenter_async.go:17,48; presenter_sync.go:329 | stale-ссылки на удалённую `ensureOutboundsParsed` | Поправить (sync:329 уйдёт с методом) | low | high | trivial |
| ui/configurator/presentation/presenter_save.go:296-304 | doc/log описывают config.json+`PopulateParserMarkers` (удалён, Save=state-only) | Переписать на state-only | low | high | trivial |
| ui/configurator/outbounds_configurator/configurator.go:223-237 | два header-блока: stale design vs SPEC 057-R-N (первый противоречит коду) | Удалить 223-231, оставить 232-237 | low | high | trivial |
| ui/configurator/models/preset_ref_sync.go:631 | tombstone `SyncDNSToStateV6` → указывает на test-only преемника | Удалить | low | high | trivial |
| ui/components/click_redirect.go:13-31,32-38 | два соседних doc-блока (lowercase + exported) на один тип | Слить в один | low | high | trivial |
| internal/traffic/session.go:30-32 | struct-комментарий описывает несуществующий кэш агрегатов | Удалить (или реализовать кэш) | low | high | trivial |
| internal/traffic/http_client.go:5-11 | doc `asStdHTTP` про несуществующий import-cycle | Убрать спекулятивную фразу | low | medium | small |
| internal/misc process_service.go:560 | `_ = pInfo // …` restate-кода (связан с FindProcess-находкой) | Удалить вместе со slimming FindProcess | low | high | trivial |
| api/clash.go:77 | `// Increased to 20 seconds…` — историческая правка, не смысл | Описать значение или удалить | low | high | trivial |
| docs/TEMPLATE_REFERENCE.md:104-110,60,125 | §4.4 описывает singular `rule`, после SPEC 067 Phase 9 — `rules []map` | Переименовать §4.4, обновить список/walk | medium | high | small |
| docs/WIZARD_STATE.md:231,786-787 | `applyUpdatesToBase`/`applyOutboundUpdate` приписаны не тем файлам | Поправить ссылки (resolve_outbounds/preset_outbounds) | low | high | trivial |
| docs/TEMPLATE_REFERENCE.md:167 | `templateRequiredTags` приписан `ResolveOutbounds` (UI-хелпер ≠ core) | Разделить: core=`RequiredOutboundTags()`, UI=`templateRequiredTags` | low | high | trivial |

### 4.4 doc-mismatch

| Location | Суть | Cleanup | Sev | Conf | Effort |
|---|---|---|---|---|---|
| core/config_service.go:933-938 | см. redundant-comment (комментарий ≠ MarkCacheStale) | — | low | high | trivial |
| core/config/subscription/node_parser_transport.go:43-51 | `NormalizeUTLSFingerprint` doc обещает variant-mapping, делает только lowercase | Переформулировать на lowercase/trim | low | high | trivial |
| core/state/dns_options.go:24-26, sync_dns.go:8-12, load.go:154 | doc «sync вызывается на load», но headless Load его не зовёт (только presenter_state.go:143) | Поправить: sync на открытие Configurator/toggle | medium | high | small |
| core/debugapi/server.go:286-302 | doc обещает `{"rebuilt":bool}`, handler пишет только `{"ok":true}` | Убрать promise или протянуть bool | low | high | small |
| core/services/api_service.go:245,274 (+controller.go:686) | doc «(1,3,7,13,17s)», реально 14-attempt {1..15} | Обновить оба комментария | low | high | trivial |
| core/services/file_service.go:22 | doc-путь `ui/wizard/business/saver.go` (файла нет) | Указать реальный saver (state_store/config_service) | low | high | trivial |
| internal/outboundutil/outbound.go:4-11 | doc «single source of truth, used by rule_utils.go» — rule_utils не импортирует пакет | Унифицировать форк или убрать строку | low | high | trivial |
| ui/configurator/models/dns_state.go:18 | doc alias `=DNSOptions`, реальный `=LegacyDNSOptionsV5` (тип НЕ мёртв) | Поправить комментарий, файл не удалять | low | high | trivial |
| ui/configurator/configurator.go:1-30 | header «Package wizard / wizard.go», реально `package configurator` | Обновить header | low | high | trivial |
| ui/configurator/models/rule_slot.go:43 | `см. ApplyRuleOrderFromV6Rules` (нет такой); реальная `RuleOrderFromStateRulesV6` | Поправить ссылку | low | high | trivial |
| docs/TEMPLATE_REFERENCE.md:104-110,60,125 | см. redundant-comment (singular `rule` → `rules`) | — | medium | high | small |
| docs/WIZARD_STATE.md:231,786-787 | см. redundant-comment (мис-атрибуция функций) | — | low | high | trivial |
| docs/DATA_FLOW.md:161; WIZARD_STATE.md:715 | несуществующий entry `BuildAndWriteConfig`/`ApplyTemplate` (реальный `BuildConfig`) | Заменить на `core/build.BuildConfig` | medium | high | trivial |
| docs/ARCHITECTURE.md:197-200 | дерево ссылается на удалённый `core/config/updater.go` | Удалить узел, перенести функции | medium | high | small |
| docs/ARCHITECTURE.md:286-288 | дерево ссылается на удалённый `ui/error_banner.go` | Удалить узел (или указать `ShowErrorBanner`) | low | high | trivial |
| docs/PRODUCT_ANALYSIS.md:107 | путь `ui/wizard/business/create_config.go` не существует; функция в core/build | Поправить на `core/build/route_merge.go` | low | high | trivial |
| docs/PRODUCT_ANALYSIS.md:15,259 vs API.md:3 | «28 endpoints/5 групп» vs «24/6» (реально 24 protected + /ping) | Свести к 24 (+/ping), единый group-count | low | high | trivial |
| docs/TEMPLATE_REFERENCE.md:167 | см. redundant-comment (templateRequiredTags ≠ ResolveOutbounds) | — | low | high | trivial |
| docs/ARCHITECTURE.md:388-395 | дерево называет переименованные rules_tab.go хелперы | Регенерировать дерево или урезать до package-map | low | high | medium |

### 4.5 code-smell

| Location | Суть | Cleanup | Sev | Conf | Effort |
|---|---|---|---|---|---|
| core/network_utils.go:34-37, controller.go:72 | не gofmt-clean (выравнивание) | `gofmt -w` | low | high | trivial |
| core/build/rules_pipeline.go:285-290 | `TemplateDNSServer` json-теги никогда не (un)marshal'ятся | Удалить теги (при релокации типа) | low | high | trivial |
| core/build/preset_expand.go:450-464,466-482 | `isRuleEmpty`/`isDNSRuleEmpty` принимают игнорируемый `_ map[string]bool` | Дропнуть параметр + 2 call-site | low | high | trivial |
| core/build/resolve_dns.go:461-477 + preset_expand/preset_outbounds | varsMap-каскад дублирован — см. divergent-logic | — | low | high | small |
| core/config/subscription/decoder.go:46,76-88 | повторный `TrimSpace` на уже trimmed `contentStr` (4 no-op) | Работать с `contentStr` напрямую | low | high | trivial |
| core/config/subscription/share_uri_encode.go:179,262,363,388,533,545,609,618 | `mapGetString(map{"v":x},"v")` — throwaway-map ×9 для coerce | Извлечь `ifaceToString(v)`, заменить сайты | low | high | small |
| core/config/outbound_share.go:65-71 | fallback на endpoints через `strings.Contains(err,"not found")` | sentinel-ошибка + `errors.Is` | low | medium | small |
| core/debugapi/traffic_endpoints.go:375 | усечённые тела молча проглатываются (`Contains(err,"EOF")`) — деструктивный clear на PATCH /state/dns | `errors.Is(err, io.EOF)`, остальное → 400 | medium | high | trivial |
| core/debugapi/server.go:236-248 | не gofmt-clean (map-литералы) | `gofmt -w` | low | high | trivial |
| core/services/api_service.go:315-318 | `AutoLoadInProgress` течёт при пустом group → авто-загрузка стопится навсегда | `defer`-сброс под мьютексом | medium | high | trivial |
| core/services/api_service.go:257 | lockless чтение `apiSvc.Enabled` (race с ReloadClashAPIConfig) | Читать через `GetClashAPIConfig()` | medium | high | trivial |
| core/services/api_service.go:388 | `fmt.Errorf("Clash API is disabled")` — capitalized, без verbs (ST1005) | `errors.New("clash API is disabled")` | low | high | trivial |
| core/services/srs_downloader.go:77-79 | timeout-ветка теряет cause (`Errorf("connection timeout")`) | `…: %w` | low | high | trivial |
| core/services/file_service.go:22 + uiservice naming | dead doc-путь (см. doc-mismatch); Wizard*-нейминг — intentional | Поправить только путь | low | high | trivial |
| core/template/preset_loader.go:338-348 | global-collision warning эмитит `Action:"strip"`, но не strip'ит (inert) | advisory Action или документировать | low | high | small |
| core/template/loader.go:312-324, preset_loader.go:56-60 | не gofmt-clean | `gofmt -w` | low | high | trivial |
| ui/diagnostics_tab.go:183,186 | STUN-строки hardcoded English, мимо locale | locale-ключи через `locale.Tf` | low | high | small |
| ui/traffic_verbose.go:34-35 | confirm-диалог hardcoded English | locale.T/Tf | low | high | small |
| ui/diagnostics_tab.go:262-272 | индекс через label-scan вместо `SelectedIndex()` | `ipServiceSelect.SelectedIndex()` | low | high | trivial |
| ui/core_dashboard_tab.go:1199-1244 | 4 fyne.Do download-failed блока copy-paste | local `fail := func(reason)` | low | high | small |
| ui/core_dashboard_tab.go:639 | magic `3` в restart-label, decoupled от `restartAttempts` | Экспортировать лимит / constants | low | high | trivial |
| ui/configurator/tabs/dns_tab.go:266-267 | лишний таб (gofmt flags файл) | `gofmt -w` | low | high | trivial |
| ui/configurator/business/sources.go:118 | `time.Since(time.Now())` всегда ~0 | Удалить строку (EndWithDefer покрывает) | low | high | trivial |
| ui/configurator/business/sources.go:15,164-166 | `configtypes` импортирован только под `var _` guard | Удалить import+guard | low | high | trivial |
| ui/configurator/dialogs/add_rule_dialog.go:80,82,87 | двойной `UpdateChildOverlay` без изменения состояния | Снять прямой вызов :80 | low | high | trivial |
| ui/configurator/dialogs/save_state_dialog.go:135,150 | дубль `Resize(400,300)` | Удалить второй | low | high | trivial |
| ui/configurator/dialogs/add_rule_dialog.go:212 | stale `O(1) instead of O(n)` over-justification | Переформулировать/инлайнить | low | medium | trivial |
| ui/configurator/dialogs/add_rule_dialog.go:559,648,668,724,796,609-626 | path_mode/domain_mode логика из локализованных строк (хрупко, ×5+) | Стабильные const/enum, сравнивать с ними | medium | high | medium |
| ui/configurator/dialogs/get_free_dialog.go:165 | спекулятивный `_ = presenter` + typo-комментарий | Дропнуть параметр | low | medium | trivial |
| ui/configurator/outbounds_configurator/configurator.go:284-294,482 | `tagsAbove` без sameScope-фильтра (cross-scope кандидаты молча дропаются build'ом) | Фильтровать sameScope или смягчить doc | low | high | small |
| ui/configurator/outbounds_configurator/edit_dialog.go (literals) | magic-литералы типов/scope/edit-source/pseudo-tags (settings×8, raw×7, …) | Package-level consts | low | high | small |
| ui/configurator/outbounds_configurator/flag_picker.go:235-245 | `rebuildFromChips` перетирает hand-typed regex при пустом выборе | Skip SetText когда picked пуст | low | high | trivial |
| internal/platform/wintun_cleanup_windows.go:335-367,478-527 | header «no-op на non-Win7», но NLA-cleanup кросс-версионный | Уточнить header (Win7-only = только device-node) | low | high | trivial |
| internal/platform/power_windows.go:120-135 | `WNDCLASSEXW` анонимный литерал дублирован для `sizeof` (cbSize-footgun) | Именованный тип `wndClassExW` | medium | high | small |
| internal/fynewidget/hover_row.go:149-151 | `& 0xff` после `uint8`-cast — no-op | Удалить маску | low | high | trivial |
| internal/process/process.go:29 + process_service.go:558-560 | `FindProcess` возвращает `ProcessInfo`, который caller отбрасывает | Сменить на `(bool,error)` / убрать `_ =` | low | high | small |
| internal/debuglog/debuglog.go:98-114 | сообщение форматируется до 3 раз; `line` строится зря без viewer | `log.Print(line)`, gate на viewerWants | low | high | trivial |
| api/clash.go:259…606 (~23) | per-call timestamp+writeLog boilerplate (magic layout literal) | Адаптировать `logMsg`-closure | low | high | small |
| api/clash.go:194,202-204,226-227 | `apiLogFile` пишется/читается без синхронизации (race + use-after-close при shutdown) | Под `apiLogSinkMu`: snapshot *os.File в writeLog | medium | high | trivial |
| main.go:117-184 | ~67-строчный `updateTrayMenu` closure (debounce/magic-delays/recover) в main() | Перенести в метод `UIService` | low | high | medium |
| main.go:294-336 | вложенные `AfterFunc(3s)→AfterFunc(2s)` power-resume (~5 уровней, magic delays) | Извлечь named-метод + const | low | medium | medium |
| core/config_service.go:378,381 | nil-deref `ac.FileService` до nil-check (SA5011) | execDir в локальную под guard 368-369 | medium | high | trivial |
| static (network_utils:81, ui_service:119/121, hover_row:61, settings_tab:426, configurator:344/352/434, dns_tab:116, source_tab:403, migration test:342) | deprecated stdlib/Fyne API (NewMax×23, SelectTab, OnChanged, TextTruncate, Dark/LightTheme, Clipboard, PlaceHolderColor, strings.Title) — SA1019 | Механический sweep на новые API | low | high | medium |
| static (rules_pipeline:106, session:181/271, clash_api_tab:564, configurator:277, dns_tab:355, dns_unified_rules:82/207, preset_ref_edit_dialog:122, rules_unified_rows:178, source_edit_window:264, help_tab:63, api_service:388, core_dashboard_tab:1463, controller:185) | мелкие staticcheck (S1011/S1021/S1000/ST1005/ST1006) | gofmt-уровня правки | low | high | small |
| core/state/migration_v5_to_v6_test.go:342 | deprecated `strings.Title` в тест-фикстуре (SA1019) | `cases.Title` или inline-helper | low | high | trivial |

### 4.6 unnecessary-check

| Location | Суть | Cleanup | Sev | Conf | Effort |
|---|---|---|---|---|---|
| core/tray_menu.go:40-42,151-154,165 | дублирующие `runtime.GOOS` guard'ы (caller уже darwin-gated) | Снять entry-guard 152-154 и inner 165 | low | high | trivial |
| core/config_service.go:428-463,341-363 | `dnsConfigForUpdate` nil-guard (`s != nil`) misleading half-guard (caller гарантирует non-nil, 440/454 деref) | Дропнуть guard или сделать полностью nil-safe | low | high | trivial |
| core/config/subscription/node_parser.go:469 | `r >= 0` всегда true для range-rune | `if r <= 0x1F` | low | high | trivial |
| internal/traffic/parser.go:177-180 | `case "i/o timeout"` поглощён `case "timeout"` ниже | Удалить узкий case | low | high | trivial |
| internal/traffic/profiler.go:203 (callers 157,182) | `eventFromConn` имеет всегда-игнорируемый bool `_` | Дропнуть параметр + 2 аргумента | low | high | trivial |
| ui/configurator/dialogs/add_rule_dialog.go:203-210 | redundant empty-fallback после `EnsureDefaultAvailableOutbounds` (всегда non-empty) | Убрать len==0 fallback + >0 guard | low | high | trivial |
| core/template/substitute.go:171,185,561 | dead nil-check на `replacementForPlaceholder` (никогда не nil-iface; SA4023) | Снять guard'ы или вернуть nil на !ok | low | high | small |
| ui/configurator/business/sources.go:17-18 | дубль-импорт core/state (`state` + `corestate`; ST1019) | Один alias, `corestate.MakeULID` на 72/98 | low | high | trivial |

### 4.7 bottleneck

| Location | Суть | Cleanup | Sev | Conf | Effort |
|---|---|---|---|---|---|
| ui/log_viewer_window.go:137-162,196-220 | O(n)-скан на строку при рендере (×2 прохода) — bounded line-cap'ом | Кэшировать filtered-slice per Refresh | low | high | small |
| ui/traffic/live_view.go:80-118,265-274 | `filteredIndices()` O(n) per row → O(rows×n) per Live refresh (ring 5000) | Кэш per-refresh под `v.mu`, инвалидация на filter/events | medium | high | small |
| ui/traffic/per_process_view.go:223-282,298-304 | per-event re-aggregate всей сессии без coalescing (Events() ×4 + 3 sort + 5 Refresh) | atomic dirty-flag, агрегировать только по 1s-тику | medium | high | small |
| ui/traffic/per_process_view.go:284-296,257-272 | 1s-тик re-aggregate даже idle / read-only saved | Gate на `ActiveSession() != nil` | low | high | small |
| internal/traffic/session.go:150-280 | агрегация ре-сканирует до 50k events на каждый UI-тик (4× copy + 3 rebuild, O(n·k)) | Кэш по event-count / инкрементальные индексы | medium | medium | medium |
| main.go:201-214 | startup парсит state.json целиком ради 1 info-лога (dimension завышен — cold-path) | Логировать из уже загруженного state | low | high | small |

### 4.8 premature-opt

| Location | Суть | Cleanup | Sev | Conf | Effort |
|---|---|---|---|---|---|
| core/build/preset_merge.go:353-367 + resolve_dns.go:373-384 | marshal→unmarshal мост для уже структурированных TemplateDNSDefaults | Дать ResolveDNS типизированный вход | low | medium | small |
| ui/configurator/tabs/dns_preset_bundled.go:30-33,248 | `jsonMarshalIndent` — однострочный алиас вокруг `json.MarshalIndent` | Инлайнить, удалить | low | high | trivial |
| ui/configurator/business/outbound.go:261-262 + presenter/rules_tab callers | двойной `EnsureDefaultAvailableOutbounds` (callers pre-apply, потом ещё раз) | Выбрать одного владельца инварианта | low | high | small |
| ui/configurator/utils/comparison.go:88-99 | `ValuesEqual` json.Marshal per comparison (в мёртвом коде) | Удаляется вместе с comparison.go | low | medium | trivial |
| ui/traffic/live_view.go:317-330 | hand-rolled `itoa` «avoid strconv import bloat» (ложно) | `strconv.Itoa`, добавить import | low | high | trivial |

### 4.9 other

| Location | Суть | Cleanup | Sev | Conf | Effort |
|---|---|---|---|---|---|
| api/clash.go:194,202-227 (writers main.go:387, controller.go:207/381) | `apiLogFile` data race + use-after-close (см. также 4.5) | Под `apiLogSinkMu` | medium | high | trivial |

---

## 5. Needs-judgment (решение владельца)

Находки, где правка меняет поведение, упирается в архитектурное решение, или это вопрос вкуса/языка комментариев. Сгруппировано по теме.

### 5.1 Архитектура / рефакторинг (medium-effort, design-call)

| Location | Суть | Sev | Conf |
|---|---|---|---|
| core/process_service.go:305-369,372-478 | crash/restart state-machine дублирован (Monitor vs onPrivilegedScriptExited); грани реально расходятся (PID-check, err==nil) | medium | high |
| core/controller.go:140-167 | `GetController` fallback ре-имплементит подмножество `NewAppController` wiring (omits UIService/APIService) | low | high |
| core/config_service.go:37-71 | `NewConfigService` мутирует package-global func-vars как side-effect конструктора | low | high |
| ui/configurator/presentation/presenter_state.go:254-340 | `CreateStateFromModel`/`LoadState` тянут build/migration в presentation-слой; dual-view adapter workaround | low (large) | medium |
| ui/configurator/dialogs/add_rule_dialog.go:60-1146 | `ShowAddRuleDialog` ~1086-строчный монолит, 7 вложенных closures, без unit-тестов | medium (large) | high |
| ui/configurator/outbounds_configurator/configurator.go:384-690; edit_dialog.go:36-966 | две длинные глубоко-вложенные функции-конструкторы держат всю dialog/list-логику | low (large) | high |
| ui/configurator/outbounds_configurator (весь пакет) | ноль тестов при сложной diff/scope/sync-логике (есть pure helpers под тесты) | medium | high |
| internal/platform/power_darwin.go vs power_linux.go | near-identical state-machine (cgo vs godbus); фикс надо дублировать | low | high |
| core/config/outbound_generator.go:242-489,496-659 | `GenerateNodeJSON` строит JSON конкатенацией строк (field-order/comment rationale реален; golden-tests) | low (large) | medium |
| core/services/srs_downloader.go:42 + locale.go:43 + fetcher.go:50 | три идентичных `CreateHTTPClientFunc` injection-seam | low | medium |

### 5.2 Удалить vs оставить (intentional/forward-compat)

| Location | Суть | Sev | Conf |
|---|---|---|---|
| internal/platform/platform_*.go (SendCtrlBreak ×3) | ноль вызовов, НО документировано в release-notes как намеренно-сохранённый graceful-shutdown API (coupled с CREATE_NO_WINDOW) — не удалять молча | low | high |
| core/events/events.go:60 (ProxyActiveChanged) | подписан (auto_update.go:75), никогда не публикуется (switch идёт через callback) — wire или delete | medium | high |
| core/events/events.go:48 (ConfigBuilt) | публикуется, нет prod-подписчиков (возможный seam под SPEC 047) | low | high |
| core/events/memory_bus.go:96-122 (SubscribeAll) | только тесты; документирован как debug-seam | low | medium |
| core/state/disk_v6.go (MetaSection.Schema/SchemaName) | write-only forward-compat (probe ключит на version) — намеренно | low | medium |
| ui/configurator/business/validator.go:169-237 | `ValidateRule`/`ValidateParserConfigJSON`/`ValidateHTTPResponseSize` без prod-вызовов — public API? | low | high |
| ui/configurator/models/wizard_state_file.go:40,45-50 | 5 alias мертвы; НО `PersistedDNSState` живой (3 callsite) — НЕ удалять | low | high |
| ui/app.go:254-260 (updateClashAPITabState) | сведён к no-op (SPEC 064), но комментарий держит как future-gating stub | low | medium |

### 5.3 Поведенческие / UX-решения

| Location | Суть | Sev | Conf |
|---|---|---|---|
| core/network_utils.go:81 | `IsNetworkError` через deprecated `net.Error.Temporary()` — удаление меняет классификацию | low | high |
| core/build/dns_merge.go:92-100,174-190 | `dns.final` fallback может выбрать отфильтрованный tag — практически недостижимо (producer удалён) | low | medium |
| core/config/parser/factory.go:34-37 | redundant size re-check после full read (marginal TOCTOU) | low | high |
| ui/configurator/tabs/source_tab.go:281-294 | toggle/delete источника не вызывает `MarkAsChanged` → Save не зажигается (traced bug, но control обычно сопровождается др. правками) | medium | high |
| ui/configurator/outbounds_configurator/configurator.go:482,617 | Add vs Edit берут addOutbounds-кандидатов из разных источников (бьёт только Edit non-last-row) | low | high |
| ui/configurator/outbounds_configurator/configurator.go:526-543 | redundant Del-guard (IsRequired уже не рендерит delBtn) — defensive против index-staleness | low | high |
| ui/traffic/toolbar.go:146-163 | export/copy берёт active-или-newest, decoupled от отображаемой сессии | low | medium |
| ui/traffic/per_process_view.go:223-282 | refresh мутирует слайсы без lock (но всё UI-thread-confined через fyne.Do → не race, lone mutex misleading) | low | medium |
| ui/traffic/live_view vs per_process_view | system-wide Live имеет filter/search/pause, per-process — нет (возможно intentional) | low | medium |
| internal/traffic/profiler.go:350-357 | dnsByIP sweep использует `e.TS` как now (skew при backfill); gated за 2048-threshold | low | medium |
| internal/traffic/profiler.go:332-335,495-505 | rolling-GC предполагает TS-sorted, но два clock-источника (poll wall vs log TS) | low | medium |
| internal/traffic/profiler.go:218-220 | `MatchedVia="router_log"` для Clash-API-attribution (debug-only label) | low | medium |
| core/debugapi/traffic_endpoints.go:370 | `MaxBytesReader(nil, …)` — теряется proactive close-on-overflow | low | medium |
| core/debugapi/server.go:286-302 + API.md:151 | doc-comment обещает `rebuilt`, handler не отдаёт (API.md уже footnote'ит) | low | high |
| ui/configurator/dialogs/get_free_dialog.go:106 | `f.ReadFrom` без size-guard — но endpoint = first-party GitHub (threat overstated) | low | medium |
| api/clash.go:156 + 263/352/495/548 | тройной timeout (Client.Timeout 20s + ctx 20s) — harmless redundancy | low | high |
| api/clash.go:316-325,444-448 | `DisplayOrName` fallback недостижим для prod-данных, но exported public-API guard | low | medium |

### 5.4 Дубликация (опциональная консолидация, не баг)

| Location | Суть | Sev | Conf |
|---|---|---|---|
| core/template/loader.go:476-486 vs vars_resolve.go:378-388 | `matchesPlatform` == `VarAppliesOnGOOS` byte-identical | low | high |
| core/template/vars_default.go:151-169 vs vars_resolve.go:349-373 | два scalar-stringify (разные входы: decoded vs raw bytes) | low | medium |
| ui/.../tabs (setTooltip ×3 + 5 inline) | три идентичных `SetToolTip` helper + inline-копии | low | high |
| ui/.../source_tab.go + source_edit_window.go | mutate→RefreshDerived→InvalidatePreview→UpdateParser последовательность дублирована (и расходится по MarkAsChanged) | low | high |
| ui/.../business create_config.go + wizard_dns.go | три DNSOptionsRaw.servers парсера; `effectiveWizardConfig` vs `effectiveTemplateConfig` (расходятся по MaterializeSecrets) | low | high |
| ui/.../business/outbound.go:219-241 | `evalPresetOutboundIf` зеркалит unexported `build.evalIf` (нужен экспорт) | low | high |
| core/state/diff.go + sync_dns.go | не gofmt-clean (но CI не гейтит gofmt) | low | high |

### 5.5 Косметика комментариев / язык (taste)

| Location | Суть | Sev | Conf |
|---|---|---|---|
| ui/configurator/tabs/dns_user_rules.go и др. | hardcoded English в DNS-rule диалогах vs соседний `locale.T` | low | high |
| ui/.../tabs source_edit_window.go:60-62,80-82 | magic `200` preview-cap ×2 | low | medium |
| ui/.../tabs source_edit_overview.go:34-268 | `buildOverviewTab` ~234-строчный монолит (timing-логи verbose-gated, не prod) | low | medium |
| core/config/varsubst.go:1-27,236-247 | тяжёлые исторические/hotfix narrative-комментарии («misreading on my part») | low | medium |
| core/services state_service/api_service | mixed RU/EN doc-блоки (DefaultAutoPingMaxProxies rationale — НЕ трогать) | low | low |
| ui/clash_api_tab.go, core_dashboard_tab.go | большие RU-rationale (restart-guard rationale — load-bearing, не трогать) | low | low |
| ui/traffic window.go/live_view/per_process_view | повторённый deadlock/UnselectAll rationale (correct, defensive) | low | medium |
| ui/configurator/outbounds_configurator (весь) | mixed RU/EN комментарии (как и весь repo) | low | high |
| core/state/ulid.go:66-84 | hand-rolled base32 bit-twiddling (корректно, протестировано) | low | low |
| core/build/preset_merge.go:29-33 | trivial single-call stdlib обёртки (osStatLocal/urlParseLocal/sha256SumLocal) | low | medium |

### 5.6 Прочие мелкие (taste/edge)

| Location | Суть | Sev | Conf |
|---|---|---|---|
| core/config/config_loader.go:48-60 | manual dedup-loop где set чище (tiny counts) | low | medium |
| core/config/subscription (xray/share_uri int/int64 cases) | недостижимые int/int64 arm'ы в JSON-coercer'ах (general helpers) | low | medium |
| core/config/subscription/source_loader.go:83-312 | ~230-строчный `LoadNodesFromSource`, tag-pipeline ×3 | low | medium |
| core/config/subscription/node_parser.go:646-706 | vmess re-implements ws/grpc/http transport (fallback-цепочки расходятся) | low | medium |
| core/config/subscription/decoder.go:99 | misleading «failed to decode base64» при общем формат-fail | low | medium |
| core/process_service.go:556-567,658-683 | `_ = pInfo` no-op; `parseCSVLine` over-clever (корректен) | low | high/low |
| core/state/adapter.go:28-41 | confusing nil-Proxies edge (load-bearing на «один Save») | low | medium |
| core/build/build.go:236-246 | DNS-секция parse+marshal дважды (cold-path, не bottleneck) | low | medium |
| core/core_downloader.go:186-194 | `getReleaseInfoFromSourceForge` игнорит ctx (symmetry) | low | high |
| ui/diagnostics_tab.go:67-100 | `checkSTUN` shadows outer err (корректно, хрупко) | low | medium |
| ui/configurator/dialogs/rule_dialog.go:94-98 | redundant `regexp.Compile` в `SimplePatternToRegex` (cheap guard, тест опирается) | low | medium |
| ui/configurator/dialogs/get_free_dialog.go:188-239 | mutable shared dialog-state через `*dialog.Dialog` (ordering dance) | low | medium |
| ui/configurator/business/source_local_wizard.go:126-131,149 | package-global map aliased в auto-outbounds (latent, текущий corruptor отсутствует) | low | high |
| ui/configurator/presentation presenter_methods/ui_updater | `SetTemplatePreviewText` оборачивает async `UpdateUI` в goroutine | low | medium |
| ui/configurator/presentation/presenter_save.go:68-69,299-304 | redundant `SaveInProgress` write; blocking sleeps для скрываемого прогресс-бара | low | high/medium |
| ui/configurator/presentation (presenter*.go) | длинные doc-блоки доминируют (часть load-bearing) | low | medium |
| ui/configurator/configurator.go:276-290 | два recursive var-closures (circular-import workaround) | low | medium |
| ui/configurator/models/wizard_state_file.go (Defaults alias) | `Defaults` alias тоже 0-ref | low | high |
| core/services/api_service.go:277-323 | inconsistent indentation (gofmt-clean, стиль) | low | medium |
| internal/platform/power_windows.go:182-187 | `ERROR_SUCCESS` guard на Translate/DispatchMessageW (теоретический spurious-warn) | low | medium |
| internal/platform/power_darwin.go:233-234 | два discard cgo-accessor (удаление coupled с C-statics) | low | high |
| internal/platform/device_info_windows.go:8-9 | wmic «present in Win10+» комментарий устарел (Win11 24H2 FoD); graceful fallback | low | medium |
| internal/traffic/profiler.go:218-285 | per-event множество мелких lock-секций (single-producer → корректно) | low | medium |
| internal/traffic/profiler.go:334,337 + session.go:57,62 | front-slice eviction (`s[1:]`) удерживает backing-head (bounded caps) | low | medium |
| internal/dialogs/dialogs.go:56-83 | `NewCustom` подменяет canvas-global `OnTypedKey` (хрупко при stacked dialogs; modal → edge) | low | medium |

---

## 6. Что НЕ трогать (intentional / уже опровергнуто)

Чтобы эти пункты не всплывали в следующих аудитах:

- **`Wizard*`-нейминг** в `core/uiservice` (WizardWindow, FocusOpenChildWindows «wizard child windows») — намеренно сохранён per architecture-note. Трогать только мёртвый doc-путь в `file_service.go:22`.
- **`SendCtrlBreak` (internal/platform ×3)** — НЕ мёртвый leftover: документировано в release-notes 0-9-8/0-8-2 как намеренно-сохранённый graceful-shutdown API, coupled с решением держать CREATE_NO_WINDOW выключенным. Удаление — только осознанное coupled-решение владельца.
- **`MetaSection.Schema` / `SchemaName`** — намеренный write-only forward-compat (probe ключит на version).
- **`PersistedDNSState` alias (wizard_state_file.go:40)** — живой (3 prod-callsite). Аудиторская заметка о его «мёртвости» опровергнута. Аналогично — НЕ удалять как «dead doc-target» в dns_state.go.
- **`DefaultAutoPingMaxProxies` rationale (state_service.go:11-19)** — load-bearing field-report (Windows network-storm на 500+ нодах), НЕ урезать как «verbose accessor».
- **restart-guard rationale (core_dashboard_tab.go:677-714)** — non-obvious design (почему restart НЕ IsRunning-gated) — load-bearing, не урезать.
- **`cloneOptions` (preset_outbounds.go:248)** — это JSON round-trip DEEP-copy, легитимно отличается от shallow-clone helper'ов; НЕ включать в shallow-clone дедуп.
- **`firstEnabledDNSServerTag` асимметрия (dns_merge.go)** — producer body-less маркеров (`legacyDNSOptionsFromV6`) удалён в SPEC 056-R-N; практически недостижимо. Latent, не активный баг.
- **`source_local_wizard.go` shared map** — заявленный corruptor (`RenameWizardLocalOutboundTags`) мутирует select-, а не auto-outbound; corruption-путь отсутствует. Latent only.
- **`parseCSVLine` (process_service.go:658-683)** — корректен для tasklist-CSV; «subtly wrong» опровергнуто. Только over-clever (taste).
- **`per_process_view.refresh` без lock** — всё UI-thread-confined через `fyne.Do`; реального data-race нет (lone mutex лишь misleading).
- **`asStdHTTP` interface seam** — комментарий про import-cycle ложный (убрать фразу), но сам seam служит для test-mock — не выпиливать абстракцию целиком без нужды.
- **gofmt CI-gate** — `.golangci.yaml` НЕ включает gofmt/gofumpt/gci; «CI lint risk» от не-gofmt-clean файлов опровергнут (кроме возможного whitespace-линтера на trailing blank line). Это косметика, не gate-failure.
- **main.go:201-214 «bottleneck»** — это one-time startup-диагностика в горутине, dimension завышен; не perf-критично.
