# Recon: потребители legacy (ParserConfig / ProxySource / ParserConfigJSON) вне core/config-конвейера

Аудит существующего кода против SPEC 117 DRAFT (чистая модель источников).
Ветка develop, 2026-08-29. Все пути — от корня `/Users/macbook/projects/singbox-launcher`.

## 0. Рамка: три представления одного и того же

1. **Canonical** — `state.Source` / `State.Connections` (`core/state/connections.go`) и
   `WizardModel.Sources []corestate.Source` + `GlobalOutbounds []Direction` + `Defaults`
   (`ui/configurator/models/wizard_model.go:74-83`). По SPEC 052 phase 8 объявлен
   источником истины.
2. **Legacy struct** — `configtypes.ParserConfig` / `ProxySource`
   (`core/config/configtypes/types.go:79,92`), продублирован в двух местах:
   `State.ParserConfig` (`core/state/state.go:84`) и `WizardModel.ParserConfig`
   (`wizard_model.go:98`).
3. **Legacy JSON-строка** — `WizardModel.ParserConfigJSON` (`wizard_model.go:93`),
   кэш сериализации legacy-структуры; используется и как текст JSON-редактора,
   и как fingerprint для stale-detection, и как транспорт «канонический список →
   парсер» через json.Unmarshal.

Конвертации:
- canonical → legacy: `WizardModel.AsParserConfig()` (`wizard_model.go:265`),
  `Source.ToProxySourceV4()` (`core/state/adapter_source.go:20`),
  `syncLegacyFromConnections` (`core/state/sync_to_legacy.go:16`, зовётся на Load
  v5/v6: `load_v5.go:50`, `load_v6.go:81`).
- **legacy → canonical (обратный синк, который DRAFT сносит):**
  `syncConnectionsFromLegacy` (`core/state/sync_to_connections.go:24`, зовётся из
  `core/state/save.go:36` на КАЖДОМ Save), плюс два UI-обратных синка —
  `onConfiguratorApply` (`ui/configurator/tabs/source_tab.go:994-1013`) и
  `applyProxyEditToSource` (`ui/configurator/tabs/source_edit_window.go:232-313`).

Ключевой факт про Save: `CreateStateFromModel` пишет Connections из canonical
(`presenter_state.go:100-106`) **и** заполняет legacy `state.ParserConfig`
проекцией (`presenter_state.go:112-115`), после чего `state.Save` →
`syncConnectionsFromLegacy` **пересобирает Connections из legacy заново**
(матчинг по ID/URL/URI/тегу цепочки, carry-over Meta/Label/Update). То есть
формально canonical — истина, фактически на записи побеждает legacy-проекция:
две конвертации на каждый Save, ровно как констатирует DRAFT.

---

## 1. Реестр потребителей (файл — функция — читает/мутирует — что именно)

### 1.1 ui/configurator/models

| Файл:строка | Функция | R/W | Что именно |
|---|---|---|---|
| `models/wizard_model.go:93,98` | поля `ParserConfigJSON`, `ParserConfig` | — | сами деривные поля; жить им в этапе 1 только как одноразовой проекции |
| `models/wizard_model.go:265-282` | `AsParserConfig` | R canonical → строит legacy | проекция Sources+GlobalOutbounds+Defaults → `*config.ParserConfig` через `ToProxySourceV4` |
| `models/wizard_model.go:287-295` | `RefreshDerivedParserConfig` | W legacy | пересобирает оба деривных кэша; вызывается после каждой мутации Sources (≈15 call-site'ов) |

### 1.2 ui/configurator/business

| Файл:строка | Функция | R/W | Что именно |
|---|---|---|---|
| `business/parser.go:52-83` | `ParseAndPreview` | R `model.ParserConfigJSON` | парсит СТРОКУ обратно в `config.ParserConfig` (canonical → JSON → parse → legacy: два лишних шага) |
| `business/parser.go:109-121` | `ParseAndPreview` | W локальной legacy-копии | `MigrateOutboundsToReferencedShape` + `SyncOutboundsWithTemplate` + `MergeOutboundUpdatesInPlace` на `parserConfig.ParserConfig.Outbounds` |
| `business/parser.go:170` | `ParseAndPreview` | R JSON | fingerprint stale-detection: сравнение строк `ParserConfigJSON` |
| `business/parser.go:213` | `ParseAndPreview` | **W `model.ParserConfig`** | кладёт разобранную legacy-структуру в модель (её потом читают preview/детур/цепочки) |
| `business/parser.go:269-287` | `SerializeParserConfig` | R legacy | нормализация + сериализация в JSON-строку |
| `business/validator.go:49-236` | `ValidateParserConfig`, `ValidateParserConfigJSON` | R legacy | валидация Proxies/Outbounds по legacy-форме |
| `business/outbound.go:69-140` | `GetAvailableOutbounds` | R `model.ParserConfig` или парс `ParserConfigJSON` | теги Направлений; мемоизация по trimmed-строке JSON (`AvailableOutboundsMemoKey`) |
| `business/outbound.go:267-290` | `AllDirectionTags` | R legacy (+парс JSON fallback) | список тегов Направлений |
| `business/detour.go:47,122` | `DetourOptions`, `DetourOptionsWithNodes` | R `*configtypes.ProxySource` | принимают legacy scratch-буфер источника (Detour*-поля) |
| `business/detour.go:237-260` | `localSubscriptionGroupTags` | R `model.ParserConfig.Proxies` (+парс JSON fallback) | теги локальных групп подписок |
| `business/preview_cache.go:28-150` | `RebuildPreviewCache` | R `model.ParserConfig.ParserConfig.Proxies` | разбор всех источников для превью; при nil — `RefreshDerivedParserConfig()` (W legacy) |
| `business/preview_cache.go:151-165` | `previewDirectionTags` | R `model.ParserConfig.Outbounds` | теги включённых Направлений для резолва цепочек |
| `business/preview_cache.go:191-203` | `applyMigratedDisabledKeys` | **W canonical** `Sources[i].DisabledNodes` | миграция ключей отметок SPEC 112 — пишет canonical по индексу legacy-прокси (индексная связь 1:1 неявная) |
| `business/direction_rename.go:44-63` | `DirectionTagTaken` | R legacy first | «форма правит legacy-вид» — приоритет `model.ParserConfig.Outbounds` над canonical `GlobalOutbounds` |
| `business/direction_rename.go:120-130` | `RenameDirection` | **W legacy + canonical (дубль)** | переименовывает в `GlobalOutbounds`, в `model.ParserConfig.Outbounds`, в `ParserConfig.Proxies[i].Outbounds` И в `Sources[i].Outbounds` — четыре копии одной правки |
| `business/create_config.go:77` | `buildConfigWithExclusions` | R JSON | только проверка на пустоту `ParserConfigJSON` как gate сборки превью |
| `business/loader.go:39-60` | `LoadConfigFromFile` | R `TemplateData.ParserConfig` (строка) | парсит parser_config шаблона в legacy-структуру, сериализует обратно |
| `business/sources.go:150-153` | `AppendURLsToSources` | W canonical + refresh legacy | мутирует `model.Sources`, затем `RefreshDerivedParserConfig` + `UpdateParserConfig(JSON)` — уже чистый canonical-писатель |
| `business/sources_json.go:145-148,163-164` | `AppendManualConfigJSON`, `RelabelLastSources` | W canonical + refresh legacy | то же |
| `business/detour_refs.go:36-126` | `ResetDetourNodeRefs`, `SourceDisplayName` | R/W canonical | уже чистые (работают только с `model.Sources`) |
| `business/config_service.go:31`, `business/interfaces.go:18` | адаптер/интерфейс | сигнатура | `GenerateOutboundsFromParserConfig(*config.ParserConfig, …)` — контракт UI↔core на legacy-типе |

### 1.3 ui/configurator/tabs

| Файл:строка | Функция | R/W | Что именно |
|---|---|---|---|
| `tabs/source_tab.go:806-826` | `applySourceMutation` | W canonical → refresh legacy | единая цепочка после мутаций Sources (drag/toggle/delete): `RefreshDerivedParserConfig` + `UpdateParserConfig(JSON)` |
| `tabs/source_tab.go:95-105,205-216,225-245` | Add URL / Add JSON / Add chain | W canonical → refresh legacy | тот же паттерн |
| `tabs/source_tab.go:889-911` | `refreshSourcesList` (счётчики) | R `m.ParserConfig.ParserConfig.Proxies` | число источников из legacy-кэша, не из `len(m.Sources)` |
| `tabs/source_tab.go:981-992` | `CreateDirectionsTab` | **W `m.ParserConfig`** | лениво материализует legacy из JSON-строки, чтобы конфигуратору было что мутировать |
| `tabs/source_tab.go:994-1024` | `onConfiguratorApply` | **W canonical из legacy (обратный синк)** | `GlobalOutbounds ← ParserConfig.Outbounds`; `Sources[i].Outbounds ← Proxies[i].Outbounds` по индексу; затем re-derive |
| `tabs/source_edit_window.go:405` | `showSourceEditWindow` | R canonical → legacy scratch | `scratch := m.Sources[i].ToProxySourceV4()` — окно правки живёт на legacy-буфере |
| `tabs/source_edit_window.go:215-230` | `setNodeEnabled` | W scratch `ProxySource.DisabledNodes` | вкл/выкл узла — через legacy-карту с TTL (DRAFT её убивает → `node.enabled`) |
| `tabs/source_edit_window.go:232-313` | `applyProxyEditToSource` | **W canonical из legacy (обратный синк)** | ручной обратный маппинг всех полей ProxySource → Source по трём веткам (chain/subscription/server); Label намеренно не участвует |
| `tabs/source_edit_window.go:315-336` | `serializeParserAfterSourceEdit` | W canonical + refresh legacy | apply + `RefreshDerivedParserConfig` + `UpdateParserConfig` |
| `tabs/source_edit_window.go:476,516` | chain-tab wiring | R legacy | `collectChainHopCandidates(…, getParserConfigForChain(m), …)` |
| `tabs/source_edit_window.go:554` | `syncFoldFormFromModel` | R/W scratch `*ProxySource` | форма свёртки читает/пишет `ps.Fold` (DRAFT: Fold умирает → `Folder.replace`) |
| `tabs/source_edit_window.go:663,1577` | apply-цепочки окна | W refresh legacy | `RefreshDerivedParserConfig` + `UpdateParserConfig` |
| `tabs/source_chain_tab.go:716-729` | `getParserConfigForChain` | R `m.ParserConfig` или парс `ParserConfigJSON` | legacy как источник кандидатов позиций |
| `tabs/source_chain_hops.go:86-115` | `collectChainHopCandidates` | R `parserConfig.ParserConfig.Outbounds` | Направления как кандидаты позиций цепочки |
| `tabs/source_fold_tab.go:206` | `foldTagPrefix` | R scratch `*ProxySource` | TagPrefix для подсказки тега свёртки |

### 1.4 ui/configurator/outbounds_configurator — главный legacy-МУТАТОР

Весь пакет работает с указателем `model.ParserConfig` (legacy) и никогда с canonical;
изменения доезжают до canonical только обратным синком `onConfiguratorApply`.

| Файл:строка | Операция | R/W | Что именно |
|---|---|---|---|
| `outbounds_configurator/configurator_helpers.go:280-297` | `getParserConfig` | **W `model.ParserConfig`** | лениво парсит `ParserConfigJSON` и кэширует в модель |
| `outbounds_configurator/configurator.go:70-86` | reorder ↑/↓ | W legacy | swap в `pc.ParserConfig.Outbounds` |
| `outbounds_configurator/configurator.go:164-190` | Edit + смена scope | W legacy | delete из `Outbounds`/`Proxies[i].Outbounds`, append в другой scope; при нехватке индекса **достраивает пустые `ProxySource{}`** (`:184-187`) |
| `outbounds_configurator/configurator.go:211-217` | Delete | W legacy | вырезание из обоих scope'ов |
| `outbounds_configurator/configurator.go:240-247` | Edit referenced | W legacy | `Outbounds[i].Updates = build.UpsertUserPatch(...)` |
| `outbounds_configurator/configurator.go:286-302` | enable/disable toggle | W legacy | `Outbounds[idx].Disabled = !on` |
| `outbounds_configurator/configurator.go:337-350` | Add | W legacy | append в `Outbounds` или `Proxies[i].Outbounds` (+достройка пустых ProxySource) |
| `outbounds_configurator/configurator.go:370-390` | seed required | W legacy | добирает required-теги шаблона в `Outbounds` |
| `outbounds_configurator/configurator_helpers.go:58-100` | `collectRows` | R legacy | построение списка из `pc.Outbounds` |
| `outbounds_configurator/configurator_helpers.go:231-244` | `collectAllTags` | R legacy | теги из `Proxies[*].Outbounds` + `Outbounds` |
| `outbounds_configurator/configurator_helpers.go:260-278` | `syncPresetOutboundsForModel` | **W дубль canonical+legacy** | `SyncOutboundsWithTemplate` на `GlobalOutbounds` И на `model.ParserConfig.Outbounds` |
| `outbounds_configurator/configurator_helpers.go:138-177` | `templateOutbounds`, requiredTags | R `TemplateData.ParserConfig` (строка) | парс шаблонного parser_config |
| `outbounds_configurator/edit_dialog.go:57-59` | открытие диалога | R legacy | `getParserConfig(editPresenter.Model())` |
| `outbounds_configurator/edit_dialog.go:727-744` | превью селектора | R legacy | `PreviewGlobalSelectorNodes(allNodes, model.ParserConfig.Proxies, …)`; подписи источников из `Proxies[si]` |

### 1.5 ui/configurator/presentation

| Файл:строка | Функция | R/W | Что именно |
|---|---|---|---|
| `presentation/presenter_save.go:89-113` | `SaveConfig` (валидация) | R JSON | парсит `ParserConfigJSON` заново, чтобы проверить «есть хоть один источник» — вместо `len(model.Sources)` |
| `presentation/presenter_async.go:45` | `UpdateTemplatePreviewAsync` gate | R JSON | пустота `ParserConfigJSON` как признак «нечего собирать» |
| `presentation/presenter_sync.go:249-260` | `RefreshAfterPresetToggle` | **W дубль canonical+legacy** | `SyncOutboundsWithTemplate` на `GlobalOutbounds` и на `model.ParserConfig.Outbounds`, затем `RefreshDerivedParserConfig` (который тут же перетирает legacy-вид проекцией canonical — вторая правка избыточна по построению) |
| `presentation/presenter_sync.go:438-470` | `ApplyParserConfigFromCurrentJSON` | **W legacy-only** | парсит JSON → `model.ParserConfig` + нормализованный `ParserConfigJSON`; canonical не трогает (изменения доедут только через обратный синк конфигуратора) |
| `presentation/presenter_state.go:100-115` | `CreateStateFromModel` | W canonical + W legacy | Connections из модели напрямую И `state.ParserConfig = *AsParserConfig()` «ради совместимости» |
| `presentation/presenter_state.go:176-185` | там же | **W дубль** | `SyncOutboundsWithTemplate` на `state.Connections.Outbounds` И на `state.ParserConfig.ParserConfig.Outbounds` — с комментарием, что иначе адаптер Save затрёт (признание, что на Save побеждает legacy) |
| `presentation/presenter_state_helpers.go:31-67` | `restoreParserConfig` | R canonical → W legacy | Load: Connections → model.Sources/GlobalOutbounds/Defaults, heal-empty из шаблона, затем `RefreshDerivedParserConfig` |
| `presentation/presenter_target.go:246-256` | `invalidateParsedNodes` | W legacy | `model.ParserConfig = nil` при смене таргета |
| `presentation/presenter_ui_updater.go:23-40` | `UpdateParserConfig(text)` | W GUI | толкает JSON-строку в entry-виджет, подавляя OnChanged; часть контракта «строка = вид» |
| `presentation/gui_state.go:91-93,118` | поля | — | `RefreshOutboundsConfiguratorList` и остатки «last valid ParserConfig JSON» |
| `configurator.go:339-361` | `loadConfigFromFile` | W JSON + W canonical | `model.ParserConfigJSON = <шаблонный parser_config>`, затем парс той же строки ради сида `GlobalOutbounds` (canonical сидируется ЧЕРЕЗ legacy-строку) |
| `configurator.go:643` | смена вкладки | W legacy | `presenter.ApplyParserConfigFromCurrentJSON()` |

### 1.6 core вне config-конвейера

| Файл:строка | Функция | R/W | Что именно |
|---|---|---|---|
| `core/state/state.go:78-84` | поле `State.ParserConfig` | — | in-memory дубль canonical в самом состоянии |
| `core/state/sync_to_connections.go:24-223` | `syncConnectionsFromLegacy` | **W canonical из legacy** | обратный синк на Save; матчинг ID/URL/URI/chain-tag, carry-over Meta/Label/Update; edge-case «Proxies == nil → не трогаем Connections» |
| `core/state/sync_to_legacy.go:16` | `syncLegacyFromConnections` | W legacy из canonical | прямой синк на Load (v5:50, v6:81) |
| `core/state/adapter_source.go:20-89` | `ToProxySourceV4` | R canonical → legacy | per-source проекция (включая `ProviderAnnounce` из Meta) |
| `core/state/connections_helpers.go:12-79` | `buildTagSpecFromLegacy` и др. | конвертация | вспомогательные маппинги legacy↔canonical |
| `core/state/save.go:36` | `Save` | вызов обратного синка | `syncConnectionsFromLegacy(s)` перед записью v6 |
| `core/state/legacy_migration.go`, `legacy_v4.go`, `load_v2_v3_v4.go` | миграции | R legacy | чтение старых форм — DRAFT это явно разрешает («легаси-ключи — только чтение в миграции») |
| `core/state/diff.go:18-19` | `Diff.ProxiesChanged/OutboundsChanged` | семантика | поля описаны в терминах legacy; в прод-коде сейчас нигде не выставляются (только `ApplyDiff` в `core/services/state_service.go:188` их потребляет) |
| `core/config_service.go:372-383` | `loadParserConfigForUpdate` | **R legacy из state** | путь Update/rebuild читает `s.ParserConfig` (legacy-вид), а не `s.Connections`; gate «Proxies == nil → ошибка» |
| `core/config_service.go:236-276` | `UpdateSubscriptionsAndConfig` | W локальной legacy-копии | Substitute/Migrate/Sync на копии + `GenerateOutboundsFromParserConfig` |
| `core/config_service.go:157-186` | `ProcessProxySource`, `GenerateOutboundsFromParserConfig` | сигнатуры | публичный контракт сервиса на legacy-типах |
| `core/config_service_context.go:26-28` | `buildContextFromState` | мёртвый параметр | `_ *config.ParserConfig` игнорируется с SPEC 045 — рудимент |
| `core/rebuild.go:186-187` | `Rebuild` | R legacy | `parserCfg := s.ParserConfig` → передаётся в игнорирующий её `buildContextFromState` (мертвый провоз) |
| `core/rebuild_raw_cache.go:89-118` | разбор raw-кэша | R legacy + W копии | `parserCfg := s.ParserConfig`, Substitute/Migrate/Sync, `GenerateOutboundsFromParserConfig`; сам .raw-кэш (`core/state/raw_cache.go`) DRAFT убивает вместе с материализацией узлов |
| `core/debugapi/state_endpoints.go:387-397` | `/state/…` merged outbounds | R canonical, обёртка в legacy | читает `st.Connections.Outbounds`, но заворачивает в `configtypes.ParserConfig{}` потому что `MergeOutboundUpdatesInPlace` требует legacy-обёртку |
| `main.go:259-272` | startup-лог | R legacy | счётчики `s.ParserConfig.ParserConfig.{Version,Proxies,Outbounds}` |
| `ui/core_dashboard_tab.go:556-574` | дашборд | R legacy | те же счётчики для UI |
| `core/template/loader.go:46-53,89-135,294-310` | `TemplateData.ParserConfig` | R/строит | parser_config шаблона хранится СТРОКОЙ в legacy-обёртке `{"ParserConfig":{...}}`; `TemplateGlobalOutbounds` лениво декодирует |
| `core/build/{build,sync_outbounds,migrate_outbounds_spec058,resolve_outbounds}.go` | shared-хелперы | R/W `[]Direction`/`*ParserConfig` | `SyncOutboundsWithTemplate`, `MigrateOutboundsToReferencedShape`, `MergeOutboundUpdatesInPlace`, `UpsertUserPatch` — вызываются и с canonical-слайсами, и с legacy-вида; сигнатуры на `configtypes` |

Отдельно: **auto-update уже чистый** — `core/auto_update.go:128,264` и
`core/config_service_subscriptions.go:58-179` ходят по `state.Connections.Sources`
(canonical). Но регенерация конфига после фетча всё равно уходит в
`loadParserConfigForUpdate` (legacy).

---

## 2. Мутации legacy, УЖЕ продублированные мутацией canonical

Это места, где однонаправленность уже «наполовину сделана» — при переходе на
canonical-only одна из двух правок просто удаляется:

1. `business/direction_rename.go:120-130` — `RenameDirection`: правит
   `GlobalOutbounds` (canonical), `model.ParserConfig.Outbounds` + `Proxies[i].Outbounds`
   (legacy) и `Sources[i].Outbounds` (canonical). Четыре копии.
2. `presentation/presenter_sync.go:254-258` — `RefreshAfterPresetToggle`: дважды
   `SyncOutboundsWithTemplate` (canonical + legacy), причём следом идёт
   `RefreshDerivedParserConfig`, который legacy-вид всё равно перетирает
   проекцией canonical — legacy-правка избыточна уже сейчас.
3. `presentation/presenter_state.go:183-184` — двойной `SyncOutboundsWithTemplate`
   на `state.Connections.Outbounds` и `state.ParserConfig...Outbounds` (комментарий
   честно объясняет: иначе Save-адаптер затрёт canonical legacy-видом).
4. `presentation/presenter_state.go:100-115` — Connections пишутся из модели
   напрямую И `state.ParserConfig` заполняется проекцией; на `state.Save`
   `syncConnectionsFromLegacy` пересоберёт Connections из проекции заново.
5. `outbounds_configurator/configurator_helpers.go:275-277` —
   `syncPresetOutboundsForModel`: тот же двойной Sync canonical+legacy.
6. Обратные синки (мутация идёт в legacy, потом целиком копируется в canonical):
   `tabs/source_tab.go:999-1012` (`onConfiguratorApply`) и
   `tabs/source_edit_window.go:232-313` (`applyProxyEditToSource`).
   Плюс сам `core/state/sync_to_connections.go` на каждом Save.
7. `business/preview_cache.go:191-203` — обратный частный случай: разбор идёт
   по legacy-виду, а результат миграции DisabledNodes пишется сразу в canonical
   `Sources[proxyIndex]` (связь — индекс 1:1 из `AsParserConfig`).

---

## 3. Оценка объёма этапа 1 (однонаправленность)

Цель этапа: canonical (`Sources`/`GlobalOutbounds`/`Connections`) — единственная
рабочая модель; legacy — одноразовая проекция непосредственно перед
parse/generate; обратного синка нет.

**Сносится целиком (3 узла обратного синка):**
- `core/state/sync_to_connections.go` + вызов в `save.go:36`;
- `tabs/source_tab.go` `onConfiguratorApply` (строки 994-1013);
- `tabs/source_edit_window.go` `applyProxyEditToSource` + scratch-паттерн
  `ToProxySourceV4` (окно правки переводится на `*state.Source`).

**Два настоящих переписывания (не механическая замена):**
- пакет `ui/configurator/outbounds_configurator` (3 файла, ~10 мутирующих
  операций) — весь CRUD Направлений работает по указателю на legacy и полагается
  на `Proxies[i].Outbounds` для scope «для одного источника»; переводить на
  `GlobalOutbounds`/`Sources[i].Outbounds` напрямую. По DRAFT локальные
  Направления источника вообще умирают → часть кода не переносится, а удаляется.
- `tabs/source_edit_window.go` (~1600 строк) — вся форма источника живёт на
  scratch `ProxySource` (включая Fold-вкладку и DisabledNodes).

**Механические переключения чтений legacy → canonical (по файлам):**
- ui/business: `parser.go` (вход парсера: `AsParserConfig()` вместо
  парса строки; fingerprint — не строка), `outbound.go`, `detour.go`,
  `preview_cache.go`, `direction_rename.go` (убрать legacy-ветки),
  `create_config.go` (gate по `len(Sources)`), `validator.go` (перевести на
  canonical или оставить на проекции), `loader.go` — 7 файлов;
- ui/tabs: `source_tab.go` (счётчики :889-911, applySourceMutation),
  `source_chain_tab.go`, `source_chain_hops.go`, `source_fold_tab.go` — 4 файла;
- ui/presentation: `presenter_save.go`, `presenter_async.go`, `presenter_sync.go`
  (обе функции), `presenter_state.go`, `presenter_state_helpers.go`,
  `presenter_target.go`, `presenter_ui_updater.go` (судьба JSON-редактора),
  `configurator.go` (сид из шаблона мимо строки) — 8 файлов;
- models: `wizard_model.go` (деривные поля → локальная проекция) — 1 файл;
- core: `state/state.go` (снять поле `ParserConfig` или пометить build-only),
  `state/sync_to_legacy.go` (умирает или становится проекцией по требованию),
  `state/adapter_source.go` (остаётся как проекция), `state/save.go`,
  `state/load_v5.go`/`load_v6.go`, `config_service.go`
  (`loadParserConfigForUpdate` → Connections), `rebuild.go`,
  `rebuild_raw_cache.go`, `config_service_context.go` (мёртвый параметр),
  `debugapi/state_endpoints.go` (сигнатура Merge-хелпера), `main.go`,
  `ui/core_dashboard_tab.go` — ~12 файлов.

**Итого: ~35 прод-файлов, ~70+ call-site'ов**; из них два переписывания
(outbounds_configurator, source_edit_window), три сноса обратного синка,
остальное — механика чтений + удаление дублей из §2. Плюс тестовый хвост
(десятки _test-файлов строят модели через `ParserConfigJSON`/legacy-структуры —
`detour_test`, `outbound_test`, `direction_rename_test`, `validator_test` и др.).

---

## 4. Конфликты кода с моделью DRAFT и пробелы (по теме «потребители»)

1. **На Save побеждает legacy, а не canonical.** `state.Save` →
   `syncConnectionsFromLegacy` пересобирает Connections из legacy-проекции
   (`sync_to_connections.go:24`, `save.go:36`), хотя `CreateStateFromModel` уже
   записал canonical напрямую. DRAFT постулирует ровно обратное направление.
2. **Update/rebuild-конвейер читает legacy-вид состояния**, а не Connections:
   `loadParserConfigForUpdate` (`config_service.go:372-383`),
   `rebuild_raw_cache.go:89`, `rebuild.go:186`. При «canonical — рабочая модель»
   это первые кандидаты на проекцию `Connections → ParserConfig` в одной точке.
3. **JSON-строка как транспорт**: canonical сериализуется в `ParserConfigJSON`
   и парсится обратно на каждом Preview (`parser.go:52-72`) и на сиде
   из шаблона (`configurator.go:346-359`). Двойная конвертация из DRAFT —
   буквально здесь.
4. **CRUD Направлений целиком на legacy** (outbounds_configurator) с обратным
   синком по индексам; включая достройку пустых `ProxySource{}` под scope
   (`configurator.go:184-187,346-349`) — конструкция, которой в новой модели
   нет вообще (локальные Направления источника умирают в пользу FolderReplace).
5. **DisabledNodes (карта с TTL)** — форма правки узлов работает через legacy
   `ProxySource.DisabledNodes` (`source_edit_window.go:215-230`), DRAFT заменяет
   на `node.enabled` при материализации узлов.
6. **Fold-вкладка** редактирует `ProxySource.Fold` scratch-буфера
   (`source_edit_window.go:554`, `source_fold_tab.go:206`) — по DRAFT fold-флаг
   умирает, замена — `Folder.replace`.
7. **Detour-тройня** (`DetourTag`+`DetourNodeSourceID`+`DetourNodeTag` +
   рудименты Hash/Label) провозится через все слои (ProxySource ↔ Source ↔
   формы) — DRAFT сводит к одному `NodeLink`.
8. **`TemplateData.ParserConfig` — строка в legacy-обёртке** (`template/loader.go:53`):
   шаблон продолжит порождать legacy-форму даже после чистки рантайма; DRAFT
   этот угол не оговаривает (пробел: во что сидируется новый пустой state —
   в canonical напрямую или через ту же строку).
9. **`Meta.PreviewNodes`** живёт и пишется на fetch
   (`config_service_subscriptions.go:269-276`, `state/connections.go:262`) —
   DRAFT убивает вместе с материализацией узлов; превью edit-окна при этом
   парсит body самостоятельно (`source_edit_window.go:67-101`) — станет чтением
   `nodes[]`.
10. **`Diff.ProxiesChanged/OutboundsChanged`** (`state/diff.go:18-19`) описаны в
    терминах legacy и в прод-коде не выставляются — семантику «что зовёт
    перепарс» придётся переопределять по canonical-секциям (пробел DRAFT:
    что считается CacheStale, когда «сборка больше не парсит подписки»).
11. **Индексная связь 1:1** `Sources[i] ↔ ParserConfig.Proxies[i]` — негласный
    инвариант, на котором висят обратный синк (`source_tab.go:1006-1012`),
    `applyMigratedDisabledKeys` и `PreviewNodesBySource`; при multi-connection
    legacy-записях он ломается уже сейчас (`sync_to_connections.go:156-206`
    разворачивает одну запись в несколько Source). В новой модели ключом
    становится тег/id — все индексные карты превью подлежат пересмотру.
