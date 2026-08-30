# CODEMAP — этап 3 (папки в UI)

Кэш знаний о коде: карта для точечного чтения, НЕ пересказ. Ветка `develop`,
после кампании SPEC 118 (state v7). Нормативные модели: `SPECS/features/`
{`sources.md`, `directions.md`, `state.md`} — при расхождении карты и модели
прав документ модели.

Проверять карту грепом по имени функции: строки плывут при правках.

---

## 1. РЕЕСТР

### 1.1 state v7 — типы и хранение (`core/state/`)

Все канонические типы дерева источников — `sources_v7.go` (437 строк):

| Сущность | Место | Заметка |
|---|---|---|
| `SourceKind` + константы | `sources_v7.go:23,25` | server/chain/auto/folder/subscription **+ unsupported** (W11: только внутри контейнера — в корне `normalizeSourceShape` отвергает) |
| `NodeLink{FolderID,Tag}` | `sources_v7.go:34` | FolderID=="" → корневое простр-во финальных тегов |
| `Origin{Kind,Raw,SubURL}` | `sources_v7.go:42` | kind: uri/wg_ini/json (`:52`); kind=warp НЕ существует |
| `TagPolicy{Prefix,Postfix}` + `IsZero` | `sources_v7.go:61,67` | маски нет; переменные — на эмиссии |
| `AutoStrategy` (alias) | `sources_v7.go:77` | = `configtypes.DirectionAuto`, 9 полей |
| `AutoGroup{GroupType,Default,Members,Strategy}` | `sources_v7.go:80` | Default — СЫРОЙ тег члена, живёт тут, не в Strategy |
| `Node{Kind,Tag,Enabled,Origin,Body,Detour,Hops,Group,Reason}` | `sources_v7.go:99` | Body чист от detour; Hops — chain only; Group — auto only; **Reason — unsupported only** |
| `(*Node).IsUnsupported` / `NewUnsupportedNode` | `sources_v7.go:152,159` | SPEC 116 W11: неразобранная запись тела как узел контейнера — origin обязателен, enabled всегда false |
| `FolderReplace{Mode,Tag,Strategy}` | `sources_v7.go:130` | режимы `:139` manual/auto/both |
| `Source` (embedded `Node` + ID/Name/TagPolicy/Nodes/Replace + sub-поля) | `sources_v7.go:147` | ID — ULID, единственная идентификация папки |
| `Source.NodeTagOrLabel()` | `sources_v7.go:191` | откат на Label только для доканонических записей |
| `SubMeta` | `sources_v7.go:203` | заголовки/userinfo/announce; БЕЗ истории и превью |
| `FetchWarning` | `sources_v7.go:219` | per-record деградации fetch |
| `SubUpdateStatus` | `sources_v7.go:233` | единственный дом диагностики fetch |
| Конструкторы `NewServerSource`/`NewChainSource`/`NewAutoSource`/`NewFolderSource`/`NewSubscriptionSource` | `sources_v7.go:251–276` | нормальная точка создания |
| `normalizeSourceShape` / `normalizeNodeShape` | `sources_v7.go:292,379` | гасит нелегальные комбинации полей юниона на Load |

Поля папки для этапа 3: `Source.ID/Name/TagPolicy/Nodes/Replace` —
`sources_v7.go:158–162`; общий detour папки едет тем же ключом
`Node.Detour` (`sources_v7.go:119`, семантика по kind).

**Корень состояния**: `State` — `state.go:50`; конструктор `New()`
`state.go:177`; выборки `GetSubscriptionSources` `:190`,
`GetServerSources` `:204`, `FindSource(id)` `:218`.

**Load / Save**:

- `Load(path)` — `load_router.go:30` (бэкап легаси перед миграцией,
  `PersistMigrationReport`, гейт `migrationPurgesLegacy`: Save → purge → Save).
- `Parse(data)` `load_router.go:86` → `parseWithContext` `:90` — роутер версий:
  `meta>=7` → `parseV7`, 6 → `parseV6Legacy`, 5 → `parseV5Legacy`, top 2–4 → legacy.
- `sniffSchemaVersion` `load_router.go:121` — пробник top-level `version` vs `meta.version`.
- `parseV7` / `diskStateV7` — `disk_v7.go:61,46`.
- `Save` `save.go:25`; `MarshalV7` `save.go:101` (ОДНА сериализация с Debug API);
  `marshalDisk` `save.go:123`.
- Нормализация после разбора — `load_normalize.go:11` (`normalizeNilSlices`),
  `sanitizeOutboundRefs` `:45`.

**Миграция — только вход** (не читать целиком, точка входа + формы):

- `migrateLegacyStateToV7` — `migration_v6_to_v7.go:77` (единственный вход);
  `migrationV7` `:45`; `LoadContext` `:34`.
- Легаси-формы чтения: `legacySourceV6` `migration_legacy_source.go:29`,
  `legacyFold` `:96`, `legacyFoldTagPrefix/AutoTag/SelectTag` `:130–141`
  (позиционный дериватив имени свёртки — он же нужен бэкапу).
- Raw-кэш подписок (жив только на миграции): `migration_raw_cache.go:26–58`.
- Шаги видны по методам: `materializeSources:162`, `materializeSubscription:285`,
  `migrateChainHops:456`, `migrateDetours:527`, `migrateFolds:623`,
  `reportLocalDirections:667`, `reportExcludes:683`, `applyRenames:701`,
  `purgeLegacyAfterMigration:834` (шаг 8, снос легаси).

### 1.2 fetch / merge (`core/config/`, `core/state/`, `core/`)

| Функция | Место |
|---|---|
| `MaterializeSubscriptionBody(subID, decodedBody, skip, capN)` | `core/config/fetch_materialize.go:50` |
| `unsupportedNodeFromRecord` (W11, тег из записи — W13) | `core/config/fetch_materialize.go` — имя берётся ИЗ ЗАПИСИ развилкой `nameFromRejectedRecord` (URI → `LabelFromOriginURI`, JSON → `NameFromOriginJSON` по полям `tag`/`ps`/`remarks`/`name`, баннер → сам его текст); позиционный `unsupported-N` — только для безымянной. Уникализация — ТОЙ ЖЕ машиной, что у принятых (`subscription.MakeIdentityUnique` над `uniquifyAgainstCounts`), по ОДНОМУ счётчику `idCounts` на весь материал: два одинаковых баннера дают «X»/«X-2», как два одноимённых сервера. Своё правило «тег занят → позиционный» снято — оно ломало матч второму баннеру при следующем fetch. Потолок тега `maxUnsupportedTagRunes`=120 |
| `RejectReasonProviderBanner` / `isProviderBannerLine` (W13) | `core/config/subscription/parse_body.go` — анонс провайдера («Лучшие сервера») = запись состава, а не сломанный узел: признак узкий (в строке НЕТ `://`), причина своя и хранится АНГЛИЙСКИМ ключом (перевод — на показе, `previewRowReason`). Всё, что схему заявило и не разобралось, остаётся сломанным узлом с технической причиной |
| `SubscriptionFetchMaterial.Supported` (W11) | `core/config/fetch_materialize.go:25` — достоверность ответа считается по СОБРАВШИМСЯ, не по `len(Nodes)` |
| `subscription.RejectedBodyRecord` / `bodyParseState.reject` (W11) | `core/config/subscription/parse_body.go:77,262` — отбракованная запись с позицией (`After` = сколько принято до неё) и исходником; счётчик принятых не трогает. **Единственный** вход материализации unsupported-узлов: ветка формата, не позвавшая `reject`, теряет запись молча |
| `jsonRejectSink` / `newJSONRejectFlusher` / `remapRejectsToKeptNodes` (W11, фикс W13) | `core/config/subscription/json_body_rejects.go` — сток отбраковок JSON-веток (Xray-массив, sing-box-импорт): они отдают наверх готовый СПИСОК узлов, поэтому позиция помнится как «сколько узлов ветки выпущено до записи» и пересчитывается на выживших после каждого фильтра (владение §342, резолв групп), а в нумерацию принятых переводится проигрыванием — арифметикой нельзя, дедуп и кап режут уже в `st.accept`. **Не** `ParseFailureReasons`: тот дедуплицирует, имеет потолок 3 и намеренно не содержит класс «протокол не поддержан» |
| `SubscriptionFetchMaterial` | `core/config/fetch_materialize.go:25` |
| `MergeSubscriptionNodes(sub, res, trusted)` | `core/state/subscription_merge.go:94` |
| `MergeFolderNodesFromSubscription(folder, subURL, res, trusted)` | `core/state/subscription_merge.go:188` |
| `repointFolderAutoMembers(folder, subURL, touched)` (SPEC 116 W7) | `core/state/subscription_merge.go:301` — хвост folder-merge: members приехавшего Auto переуказываются на папку, непопавший член — prune + warning, выпавший `Group.Default` снимается |
| `refreshMergedNode(fresh, old)` — общая половина обоих merge | `core/state/subscription_merge.go:43` — W11: переход unsupported→узел НЕ переносит `enabled` (починенная запись оживает включённой), обратный переход гасит его принудительно |
| `nodeSubURL` / `setNodeSubURL` / `nodeTagTaken` | `core/state/subscription_merge.go:336,353,370` — `setNodeSubURL` САЖАЕТ origin на клон (копия узла делит `*Origin` с материалом вызывающего) |
| `SubFetchMaterial{Nodes,Truncated}` | `core/state/subscription_merge.go:31` |
| Оркестрация одного обновления | `core/config_service_subscriptions.go:113` `refreshOneSubscriptionSource` |
| Все подписки разом | `core/config_service_subscriptions.go:43` `refreshSubscriptionsMetaAndCache` |
| Резолв капа (подписка → настройки) | `core/config_service_subscriptions.go:210` `resolveSubscriptionMaxNodes` |
| Статусы | `failedSubStatus:224`, `successSubStatus:248` (там же) |
| Точки на один источник | `RefreshSourceInPlace:316`, `RefreshSingleSubscription:399` |
| Закрепление результата ↻ на диске (W13) | `persistFetchResultForSource:352` — load → перенос ТОЛЬКО полей fetch'а (nodes/updateStatus/meta/pendingDisabled) в дисковую запись этого id → Save, под `SubscriptionMu`; источник, которого визард ещё не сохранял, НЕ создаётся |

Вызовы, связывающие конвейер: `config_service_subscriptions.go:159`
(`MaterializeSubscriptionBody`) → `:173` (`MergeSubscriptionNodes`).

**Кто пишет `state.json`** (W13, важнее, чем кажется): визард (`SaveConfig` —
модель ЦЕЛИКОМ), `RefreshSingleSubscription` (авто-обновление / retry — своя
дисковая копия целиком) и `refreshSubscriptionsMetaAndCache` (Update, там же).
Файл каждый раз пишется целиком, поэтому расхождение модели и диска по одному
источнику любая следующая полная запись разрешает в пользу писателя, а не
данных. До W13 путь ↻ строки был единственным входом fetch'а, который на диск
не писал вовсе, — его результат жил только в модели и терялся первым же чужим
Save (баг обкатки: ↻ отработал, разбор в логе, mtime свежий, а
`last_success_at` подписки часовой давности). Новый вход fetch'а обязан
закреплять свой результат, а не рассчитывать на Save визарда.

### 1.3 Эмиссия (`core/config/`)

**Материализованный канон → узлы**:

- `EmitCanonicalSource(ps, sourceIndex, tagCounts)` — `canonical_emit.go:75`;
  узлов kind=unsupported он не видит вовсе: их отсекает ПРОЕКЦИЯ
  (`state.(*Source).canonicalProjection`, `adapter_source.go:109`) — пропуск
  структурный и ДО тег-машины, иначе неразобранная запись съела бы `{$num}` и
  слот уникализации у соседей (SPEC 116 W11);
  результат `CanonicalEmitResult` `:46`.
- Ветки по kind: `buildCanonicalNode:150`, `buildCanonicalServer:177`,
  `buildCanonicalAuto:217`; chain откладывается на 2-й проход
  (`errCanonicalChainDeferred` `:250`).

**Тег-машина** (единственная точка финального тега):

- `applyEmissionTagMachine` — `canonical_emit.go:362`
  (политика → `NormalizeProxyDisplay` → `subscription.MakeTagUnique`).
- `applyCanonicalTagPolicy:377` (prefix + сырой тег + postfix),
  `replaceCanonicalTagVariables:394` (словарь переменных — дословное зеркало
  `subscription.replaceTagVariables`, расходиться им нельзя),
  `canonicalPolicyInput:346`.

**Резолв ссылок** (`nodelink_resolve.go`):

- `NodeLinkTargets` `:47`; `BuildNodeLinkTargets(...)` `:64` — ОДИН словарь
  целей на всю сборку; `allRootLinkTargets:121`, `rootNodeTagsForGuard:157`.
- `(*NodeLinkTargets).Resolve(link)` `:201`; `NodeLinkResolution` `:187`.
- `ApplyCanonicalNodeLinks(...)` `:234` — detour fail-closed + members prune;
  отдаёт `[]EmissionWarning` (W12), адресат берётся у самого узла по
  `SourceIndex` (замыкание `addr` внутри).
- `resolveCanonicalDetour:370`, `detectCanonicalDetourCycles:403` (кольца
  fail-closed), `resolveCanonicalGroup:475` (prune), `canonicalGroupFolder:527`.
- `warnWireguardDanglingDetours:344` — detour не применяется к WG; отдаёт
  `nodeWarning{node,text}` (`:361`), адресата ставит вызывающий.
- `ResolveCanonicalChainHops(parserConfig, targets)` — `canonical_emit.go:262`
  (тоже `[]EmissionWarning`).

**Деградации эмиссии с адресатом** (`emission_warning.go`, SPEC 116 W12 фикс 2+3):

- `EmissionWarning{Text,SourceID,SourceLabel,DirectionTag}` `:59`;
  `EmissionWarningTexts:76` (только фразы — лог, тесты, `sourceParseFailure`).
- `sourceOfNodeTag:91` (тег узла → источник) и `directionOwningTag:108`
  (тег → Направление; у развёрнутого твина адресат — РОДИТЕЛЬ) — адресация
  столкновений гарда; `emissionWarningsFor:131` — пачка фраз одного источника.
- Фразы — АНГЛИЙСКИЕ ключи локали константами `:31–51`; перевод в
  `bin/locale/ru.json`. Ключ обязан совпадать с записью каталога байт-в-байт.

**Свёртки папок** (`folder_replaces.go`):

- `PrepareFolderReplaces(parserConfig, tmplAutoOptions)` `:38` — разворот
  replace в локальные группы (проход 0, после Направлений, до varsubst).
- `buildReplaceGroups:61`, `FolderReplaceTags:132`, `FolderReplacePoolTag:151`,
  `FolderReplaceGroupTags:163`.

**Гард занятости тегов** (`tag_guard.go`):

- `TagGuard` `:44`, `NewTagGuard:68`, `Claim:74`, `Taken:90`, `Owner:99`,
  `Conflicts:107` (отдаёт `[]TagConflict`, не строки), `ConflictTexts:115`,
  `Tags:128`; `TagOwnerKind` `:28` — значения теперь АНГЛИЙСКИЕ ключи локали
  (`Localized():41`), `TagConflict{Tag,Prev,Kind}` `:56` + `Text():63`.
- `BuildTagGuard(directions, proxies, rootNodeTags, systemTags)` `:148`.
  **W12 фикс 1**: гард строится по форме ПОСЛЕ `ExpandDirectionTwins`, где
  обе половины пары лежат отдельными записями. Твин-запись (`TwinOf != ""`)
  claim'ится как `TagOwnerTwin` и НЕ дублируется формулой `d.Tag+twinSuffix`
  (карта `twinTags` `:181` — теги всех записей списка + `TwinTag` родителей).
  Иначе каждое Направление с автовыбором давало ложное «тег занят дважды:
  Направление и авто-группа Направления», и то же самое — шаблонная
  отдельно стоящая `x-auto`, из-за которой твин вовсе не разворачивается.
  Тест — `tag_guard_twin_test.go`.
- `KnownTargetTags(guard, directions)` `:222` — множество для сброса
  осиротевших целей правил (обязано знать ВСЕ виды тегов).

**Генератор / санитайзер** (`outbound_generator.go`):

- `GenerateOutboundsFromParserConfig(...)` `:1094` — главный конвейер.
- `OutboundGenerationResult` `:52`, `NodeOrigin:139`, `SourceExclusion:149`,
  `sourceParseFailure:169`.
- `GenerateNodeJSON:225` / `Bare:242`, `EmitNodeJSONs:1040`,
  `generateGroupNodeJSON:1407`, `generateCanonicalBodyJSON:1511`,
  `GenerateEndpointJSON:999`.
- `sanitizeNodeDetours:1643` (fail-closed: выбрасывается узел-носитель),
  `pruneNodesBySource:1758`, `groupMemberTags:1530`, `chainOfNode:1560`,
  `normalizeChainHop:1595`.
- Финальный граф-санитайзер — отдельный пакет: `core/build/outbound_graph_sanitize.go`,
  DNS-detour как ребро графа — `core/build/dns_detour_sanitize.go`.

**Пул кандидатов Направлений (UI)**: `ui/configurator/business/node_pool.go` —
`RebuildNodePool:38`, `nodePoolDirectionTags:120`, `nodePoolRootTargets:137`,
`InvalidateNodePool:150`. Пул строится ТЕМИ ЖЕ вызовами, что сборка
(`EmitCanonicalSource:73`, `BuildNodeLinkTargets:97`, `ResolveCanonicalChainHops:98`)
— иначе пул показывает не тот состав, из которого собирается конфиг (баг #91).

### 1.4 Отчёт сборки

- Хранилище/жизненный цикл: `core/config/build_report.go` — `BuildReportEntry:101`,
  `BuildGeneration:123`, `StartBuildReport:161`, `FinishBuildReport:179`,
  `AddBuildReportEntries:199`, `BuildReport:237`, `BuildReportReadyFor:278`,
  `ParseFailedSourceReason:291`, `EmitWarningsForSource:316` (SPEC 116 W12 —
  деградации эмиссии ЭТОГО источника; их бывает несколько, поэтому список, а
  не одна строка), `DroppedNodesForSource:338`, `ResetBuildReport:147`.
- Фиды: `core/build_report_feed.go` — `FeedBuildReportFromParser:33`
  (эмиссионные записи едут с `SourceID`/`SourceLabel` из `EmissionWarning` —
  раньше субъектом стояла строка "emission"),
  `FeedBuildReportFromFetchStatus:116` (диагностика из состояния, не из разбора),
  `FeedBuildReportFromSanitizer:245`.
- Вызовы: `core/rebuild.go:183,187,199`; UI-путь — `business/parser.go:152,193,197`,
  `business/create_config.go:216`.
- UI-модель отчёта: `ui/configurator/tabs/final_report_model.go` —
  `finalReportLines:88` (порядок видов: потери источника целиком → частичные
  `fetch_degraded`/`emit_degraded` → naive), `finalReportEntryText:125`,
  `finalBuildStatusText:168` (**W12 фикс 4** — «Build OK» / «Build OK, N
  warnings» / «Build FAILED: причина» чистой функцией; вёрстка берёт текст и
  признак провала), `saveButtonVisible:64`.
- Вкладка `final_tab.go`: `statusLabel` над списком (`CreateFinalTab`), кнопка
  `copyBtn` — **W12 фикс 5** — `NewButtonWithIcon("Copy config",
  theme.ContentCopyIcon())`, как «Copy token» в `ui/settings_tab.go:594`, и
  копирует СОБРАННЫЙ конфиг (`builtText`), а не текст отчёта. Провал сборки
  больше не рисует красную строку внутри списка — он и есть статус.
- Строка Sources с ⚠ эмиссии — `ui/configurator/tabs/source_tab.go`
  (`emitWarnings := config.EmitWarningsForSource(sourceID)` рядом с
  `parseFailedReason`; рисуется ОТДЕЛЬНОЙ строкой, не в цепочке `else if`:
  потеря члена группы и урезание на последнем рубеже — разные факты).

### 1.5 Бэкап (контракт 0.11)

- Конвертеры границы v7↔0.11: `core/backup/convert_v7.go` —
  `exportFold:35`, `exportDisabledMap:62`, `exportNodeLinkRef:85`,
  `exportHops:104`, `exportChainSpec:120`, `importFold:130`,
  `importNodeLinkRef:153`, `importHops:172`, `importChainBody:189`,
  `importMaskTag:212`, `replaceTagSurvivesExport:232`,
  `legacyFoldPrefix:252`, `foldDerivedDirectionTags:267`,
  `resolveImportedHops:303`.
- `Export(s, opts)` — `core/backup/export.go:50`; `exportSubscription`,
  `exportServer`, `exportChain`, `exportSourceRef`.
- `Import(s, b, opts)` — `core/backup/import.go:149`; `importSubscription`,
  `importServer`, `importChain`, `backupReplaceTag`, `ensureSourceID`.
- Типы контракта — `core/backup/types.go`; UI — `ui/configurator/tabs/settings_backup.go`,
  `settings_backup_report_window.go`.

**Папка в бэкапе (SPEC 116 W9, §O1=А)**: контракт 0.11 папок не знает, и
экспорт их не пишет — но потеря обязана быть НАЗВАНА (критерий A9).

- `backup.Warning` (`core/backup/import.go:21`) несёт, кроме `Code`/`Detail`,
  ещё `Kind` и `Nodes`: код `backup_source_kind_unsupported` общий у папки и
  провайдерской группы, а фразы разные, и объём потери («и её 12 узлов»)
  из строки не вычитаешь. Литералы `Warning{…}` — только КЛЮЧЕВЫЕ: позиционные
  ломались на каждом новом поле.
- `Export` копит такие потери в отдельный `dropped` и кладёт его **в начало**
  общего списка (`export.go:115` + `:129`): «не поехало вовсе» обязано
  читаться раньше, чем «приехало иначе» (`WarnBackupReplaceTagDerived`).
- UI: `showExportReport` (`settings_backup_report_window.go:122`) — то же окно,
  что у импорта, БЕЗ обрезки; текст для буфера — `exportReportText:137`.
  Окно общее: `openBackupReportWindow:75` (импорт заходит через
  `openImportReportWindow:59`). Прежняя модалка резала список на 10 строках и
  отсылала за хвостом в отчёт импорта, которого при экспорте не бывает.
- Фраза потери — `unsupportedSourceWarnText` (`settings_backup.go:302`),
  ветка `warnText:261`. `warnLines` (`:243`) остался только у превью импорта.
- `contract/**` не тронут: секция `folders[]` — отдельный трек (§O1 вариант В).

### 1.6 UI-конфигуратор

**Список источников**: `ui/configurator/tabs/source_tab.go` (1497) —
`CreateSourcesTab:57`, `applySourceMutation:1062`, `showFolderDeleteDialog:1114`,
`showSourcePreviewAllWindow:1202`, `nodeDisplayLine:1304`,
`CreateDirectionsTab:1330`, `refreshOneSourceFromUI:1387`.

Папка в списке (SPEC 116 W3): `addFolderAction:298` (⋮ → «Add folder»,
ПЕРВЫМ пунктом — §O6; зовёт `corestate.NewFolderSource` + `applySourceMutation`,
окно правки НЕ открывает), `showFolderDeleteDialog:1114` (два исхода С7:
«удалить с узлами» / «вынести узлы в корень» через
`business.ExtractFolderNodesToRoot`, затем снос опустевшей папки; папка
адресуется ULID'ом на клике, не индексом строки). Строка папки НЕ
декорирована (§O5=А): отличие читается по отсутствию URL в подстроке и
кнопки обновления. Имя строки — `Source.Name`; у пустой папки счётчик
показывается явным «· 0 nodes», а `parseFailedReason` подавлен
(`isFolder && len(Nodes)==0`) — пустота папки это воля пользователя, а не
сбой источника.

**Правый клик по строке ВЕРХНЕГО узла (W13; полное меню — заход 2)** —
`source_row_node_ops.go`: `sourceRowNodeOpsAllowed` (server/chain/auto — у
контейнеров не один узел, а состав, и он правится строкой состава),
`showSourceRowNodeContextMenu` (Node info…/правка, Copy JSON, Copy tag,
Copy/Move to folder…, Rename…, Delete). Своей механики нет ни у одного
пункта — **принцип «меню = кнопки»**: Node info…/Rename… →
`showSourceEditWindow` (окно источника И ЕСТЬ форма правки верхнего узла;
свой диалог переименования был бы вторым путём БЕЗ `resetRefsAfterNodeRename`
на Save), Delete → `showSourceRowDeleteDialog` (`source_tab.go` — вынесен из
замыкания строки, один вход у корзины и у пункта, вместе с веткой непустой
папки С7), Copy JSON → `sourceRowNodeJSON` (та же `EmitCanonicalSource` +
`previewNodeJSON`, что у списка и сборки). Контекст Move/Copy — тот же
`previewNodeOps` (`win` = главное окно, `reloadScratch`/`refreshPreview` =
nil, рабочей копии у списка нет), диалог и предупреждения — те же
`showMoveOrCopyDialog`/`applyMoveOrCopy` (`preview_node_ops.go:240,286`), а
реестр переписи и перечисление непереносимых ссылок корня — те же
`business.MoveNodeToFolder`/`CopyNodeToFolder` (`rootOnlyRefsToTag`).
Встраивание — `source_tab.go` (~`:820`): `fynewidget.NewSecondaryTapWrap`
СНАРУЖИ `HoverRow`, потому что Fyne отдаёт событие самому ГЛУБОКОМУ
подходящему объекту (`FindObjectAtPositionMatching`) — внутри обёртка
перехватила бы hover и погасила подсветку строки. В `sourcesBox`,
`dragGroup.Register` и `revealedRow` едет внешний объект (`rowOuter`).

Тултип строки (W13): печатаются ТОЛЬКО заполненные поля (`addTip`,
`source_tab.go:~652`); пустой текст = тултипа нет вовсе (контракт
`ttwidget.ToolTipWidget.MouseIn`). Раньше у папки под курсором висели три
пустых двоеточия.

**Вход В КОНТЕЙНЕР прямо в списке (W13; подписки — заход 2)** —
`source_folder_drilldown.go`. Клик по строке ПАПКИ ИЛИ ПОДПИСКИ переключает
ТУ ЖЕ таблицу в режим её состава; новых виджетов, окон и вкладок нет.
Различие папки и подписки — не в экране, а в модели прав: `kind` едет в
`previewNodeOps`, и `nodeOpsAllowed()`/`reorderAllowed()` сами гасят
Move/Rename/Delete/захват у подписки.

| Элемент | Заметка |
|---|---|
| `folderDrillState{folderID}` + `active/enter/leave` | состояние ЭКРАНА в замыкании `CreateSourcesTab`, рядом с `revealedSourceID`; в модель не едет. Имя поля историческое — адрес у обоих контейнеров один |
| `drillContainerKind(kind)` | folder \| subscription: у них СОСТАВ, а не один узел. server/chain/auto сюда не попадают — их узел и есть Source |
| `folderDrillState.nodesAreFree(sources)` | «сюда можно класть узлы руками» = ровно папка; на нём стоят гейты Add (поле, кнопка, ⋮ и тихий возврат на самих путях — диалоги Add асинхронны) |
| `folderDrillIndex(sources, id)` | адрес — ULID, не индекс: пока смотрят состав, порядок `m.Sources` вправе поехать. `-1` у пропавшего контейнера → вкладка сама возвращается в корень |
| `buildFolderDrillRows` → `folderDrillRowsInput{SourceIndex,Kind,Rows,Identities,Name}` | состав ТЕМ ЖЕ `buildPreviewRows` и ТОЙ ЖЕ `config.EmitCanonicalSource` (свой пустой `tagCounts`), что вкладка Preview и сборка — иначе список показывал бы не тот состав (баг #91) |
| `folderDrillBackRow` | первая строка: `theme.NavigateBackIcon()` + U+00A0 + имя. Текстовой «←» (U+2190) НЕТ — глифа нет в шрифте Fyne, на его месте рисовалось «�». Она же заголовок: отдельного «Folder: …» над списком больше нет |
| `folderDrillNodeRow` | `[захват\|распорка][галка] имя/подстрока [карандаш][корзина]`. Вёрстка — ДОСЛОВНО та же, что у строки Preview: `canvas.Text` + `previewTightVBox{gap: previewTitleSubtitleGap}` (не `widget.Label`+`tightVBox` — у Label свой отступ темы, заголовок вставал выше центра чекбокса). Кнопки = пункты меню: карандаш → `showPreviewNodeEditWindow`, корзина → `ops.showDeleteDialog` (у подписки её нет) |
| `folderDrillNodeEnabled` / `folderDrillSetNodeEnabled` | галка пишет `Enabled` ПРЯМО в модель (scratch'а и Save у списка нет) и зовёт `applySourceMutation` — как тумблер источника |
| `renderFolderDrillRows` | наполняет `sourcesBox`, возвращает `kind`; ставит `*reorder` (замыкание на `previewNodeOps.applyReorder` по СЫРЫМ тегам) только у папки. `dragGroup.Total` НЕ ставится: строки живут в VBox, регистрируются все, а ненулевой `Total` разрешил бы бросок в слот, которого в корне нет |
| `applyFolderDrillChrome` | обвязка: подсказка `sourceHintText` ⇄ `folderDrillHintText` ⇄ `subDrillHintText`; поле Add + кнопка «Add» + ⋮ выключаются в режиме подписки; `previewAllBtn` гаснет (она про ВСЕ источники). Заголовок списка НЕ трогается |
| `applyAddedSourcesToFolderNamed` | Add-поле в режиме папки → `business.AppendNodesToFolder` (тот же `parseSourceInput`, W6). Поле НЕ очищается при отказе: отвергнутый текст обязан остаться на экране |

Точки встраивания в `source_tab.go`: `applyAddedSourcesNamed` (развилка
адреса Add + имя из файла + гейт `nodesAreFree`), форма сервера (тот же
гейт), ⋮-меню (в режиме папки остаются только пункты, чей результат — УЗЕЛ:
Add server / Add WARP / Add from file; в режиме подписки меню не
открывается), захват `dragGroup` (в режиме папки переставляет узлы ВНУТРИ
папки), `refreshSourcesList` (ветка режима — до всего остального),
`RevealSource` (`drill.leave()` — переход из отчёта адресует ИСТОЧНИК, а они
в корне), клик по строке контейнера (`drillContainerKind`,
`SecondaryTapWrap.OnPrimary` СНАРУЖИ `HoverRow`; кнопки строки лежат глубже и
свой tap получают сами).
Тест — `source_folder_drilldown_test.go` (адресация ULID'ом, порядок строк,
подписка + `nodesAreFree`).

**Окно ОДНОГО узла контейнера (W13 заход 2)** —
`preview_node_edit_window.go`, `showPreviewNodeEditWindow(row, rawTag, ops)`:
имя (Rename → готовый `previewNodeOps.applyRename`), тело (Apply / Regen from
raw — ТОТ ЖЕ `source_body_edit`-путь, сигнатура сужена до `*Node`), исходник
(`origin.raw`, только чтение). Правится ТОЛЬКО узел папки: у подписки и у
неразобранной записи то же окно read-only (у второй секция «Outbound JSON»
скрыта целиком — тела нет). Мутация НЕМЕДЛЕННАЯ (`applySourceMutation` +
`afterModelMutation`), разыменование Д5/A4 — той же `DereferenceNodeOrigin`.
**Упразднены оба прежних просмотровых окна**: `showPreviewRowInfoWindow`
(W11, клик по строке) и `showPreviewNodeInfoWindow` («Node info…», разбор +
JSON) вместе с `appendPreviewGroupRows`/`previewGroupMemberTags`/
`previewWithScrollGutter`. Три окна вокруг одного узла показывали и ни одно
не правило.

**Окно источника**: `ui/configurator/tabs/source_edit_window.go` (1857) —
`showSourceEditWindow:377` (главный конструктор окна);
`cloneSource:190`, `cloneCanonicalNode:162`, `cloneDirection:264`
(value-snapshot формы), `mergeEditedSourceIntoModel:324`,
`applySourceEditToModel:353` (путь Save), `setNodeEnabled:90`,
`nodeEnabledInSource:107`, `sourceOriginURI:124`/`setSourceOriginURI:136`,
`resetRefsAfterNodeRename:1761`, `showDetourRefsResetDialog:1804`,
`showStaleSelectionDialog:1828`.

Настройки по виду источника (SPEC 116 W4): `rebuildSettingsLayout:847` —
**`switch` по kind, три ветки** (папка / server / всё прочее = подписка);
общие блоки `tagPolicyBlock:828` (prefix+postfix+подсказка переменных) и
`detourBlock:842` делятся подпиской и папкой дословно. У папки: имя
(`nameEntry:520` → `Source.Name`, OnChanged гейтит по kind), тег-политика,
свёртка, detour — **ни URL, ни интервала, ни max_nodes, ни skip** (A8).
Прежнее «не server ⇒ подписка» осталось верным ровно в одном месте —
`syncFoldTabVisible:1641` (условие вычитанием, папку пропускает само);
остальные ветки текстов разведены по kind: заголовок окна `:426`
(«Folder — имя»), подсказка JSON `:1607`, пустой контейнер `:1578`.
Read-only JSON у папки — `:1319` (`isServerSource`), кнопок Apply/Regen нет
(`:1613` — `switch`: server/chain → Apply+Regen, папка → «Copy nodes as JSON»
(W8, `:1624`), подписка → без кнопок).

**Операции над узлом контейнера (SPEC 116 W5)** — `preview_node_ops.go`,
контекст собирается в окне: `nodeOps := &previewNodeOps{…}`
(`source_edit_window.go:928`), в нём же `reloadScratch` (перечитать рабочую
копию из живой записи, `:934`) и `refreshPreview`. Меню строки —
`showPreviewNodeContextMenu` (`preview_node_info.go`), ему передаётся
**сырой** тег (`identities[id]`, `source_edit_window.go:1236`), а не
`node.Tag`. Авторазыменование при правке тела/Regen —
`dereferenceEditedSourceNode` (`preview_node_ops.go:503`) +
`notifyNodeDereferenced` (`:484`), зовутся из Apply JSON
(`source_edit_window.go:1442`) и Regen (`:1470`).

Вкладки окна:

| Вкладка | Файл | Точка входа |
|---|---|---|
| Overview | `source_edit_overview.go` | `buildOverviewTab:25`; `appendStorageRecordSection:258`. **W11**: поузловой `nodeOriginList` удалён (дублировал Preview), диагностика fetch'а (`fetchWarningTexts` + `previewParseReasonsBlock`) переехала сюда со вкладки Preview |
| Overview: счёт и сырой ответ (W11) | `source_edit_raw_body.go` | `sourceNodesHeader:43` («Nodes: 38 + 5 unsupported»), `appendRawBodySection:63` (кнопка Reload — СКАЧИВАЕТ тело заново через `subscription.FetchSubscriptionWithMeta` и только показывает: ни nodes[], ни updateStatus не трогает), `renderRawBodyView:157` (галки base64 → urldecode → pretty-print; неудача шага не ошибка, шаг просто не применяется) |
| Chain | `source_chain_tab.go` | `newChainForm:111`, `Load:178`, `Collect:267`, `CollectLinks:249`, `applyChainFormToSource:862` |
| Chain (кандидаты хопов) | `source_chain_hops.go` | `collectChainHopCandidates:106`, `chainReplaceTags:242`, `chainFolderIDsBySourceIndex:255`, `chainReferencedBy:321` |
| Replace (свёртка) | `source_replace_tab.go` | `newReplaceTab:53`, `Load:113`, `Collect:145`, `defaultReplaceTag:205`, `replaceAutoChoices:219` |
| Body / JSON | `source_body_edit.go` | `applyServerBodyJSON`, `regenServerBodyFromRaw` (Regen). Аргумент — `*Node`, не `*Source` (W13 заход 2): `Body` и `Origin` принадлежат узлу, и та же пара правит узел контейнера из окна узла |
| JSON-рендер | `source_edit_json.go` | `unpackNodesDoc:72` (общий сборщик документа `{outbounds,endpoints}`, `limit<=0` = без обрезки), `renderUnpackedNodes:118` (показ, `limit=previewNodeCap`), `emittedToEditableJSON:38`, `stripEmittedDecorations:21` |
| JSON: «взять всю папку» (W8) | `folder_copy_json.go` | `folderCopyNodesJSON:47` (живая запись + `EmitCanonicalSource` со СВОИМ пустым `tagCounts` → `unpackNodesDoc(…, 0)` → `fynewidget.SetClipboard`), `folderCopyJSONButton:109` (nil у не-папки). Теги **финальные** (§O2 вариант А); второй эмиссии нет — та же `config.EmitNodeJSONs`, что у сборки. Пустого документа в буфер не уходит: 0 узлов → сообщение, причина названа раздельно (все выключены / не собрались) |
| Preview (операции над узлом) | `preview_node_ops.go` | `previewNodeOps:61`, `nodeOpsAllowed:98` (= «kind == folder»), `reorderAllowed:111`, `applyReorder:129`, `folderTargets:181`, `showMoveOrCopyDialog:222`/`applyMoveOrCopy:268`, `showRenameDialog:315`/`applyRename:337`, `showDeleteDialog:394`/`applyDelete:411`, `afterModelMutation:446` |
| Preview (наполнение папки, W6) | `folder_add_nodes.go` | `newFolderAddNodes:76` (nil у не-папки), `button:94`, `showPasteDialog:121`, `addFromFiles:153`/`fyneFileOpen:172`, `applyFiles:208`, `applyInput:243`, `humanError:268`, `addWarp:280`, `addServer:293`, `finish:312`, `folderAddNodesHeader:342` (встраивание — `source_edit_window.go:948`) |
| Preview (заливка подписки, W7) | `folder_fill_from_sub.go` | `showFillFromSubscriptionDialog:56` (селект доноров), `applyFillFromSubscription:123` (немедленная мутация + `applySourceMutation`/`afterModelMutation`), `offerSubscriptionRefresh:153` (зовёт существующий `refreshOneSourceFromUI`, своего fetch'а нет; дозаливки после обновления нет намеренно), `reportFillResult:172`. Пункт меню — в `folderAddNodes.button()` за разделителем |
| Preview (меню строки) | `preview_node_info.go` | `showPreviewNodeContextMenu` (принимает `previewRow` + сырой тег + `*previewNodeOps`; `row.Node == nil` — выключенный узел: пункты про эмитированный JSON/тег гаснут, окно узла и операции остаются). Первый пункт ведёт в то же окно, что карандаш строки — принцип «меню = кнопки». `showPreviewNodeInfoWindow` УПРАЗДНЁН (W13 заход 2); из файла остались только `previewNodeJSON`, `previewSectionHeader`, `previewInfoRow` |
| Preview (модель строки, W11) | `preview_rows.go` | `previewRow:30`, `buildPreviewRows:53` (строки — по составу `nodes[]`, эмитированные узлы подставляются по СЫРОМУ тегу; группы идентичности не имеют и раздаются по порядку), `previewRowsSupported:114`, `previewRowsUnsupported:125` |
| Preview (вид строки, W11; W13) | `preview_row_view.go` | `previewRowTitle`, `previewRowReason` (**W13** — перевод причины ОДНОЙ точкой на все три места, где она видна: в состоянии причина хранится английским ключом), `previewRowSubtitle` (у Unsupported «⚠ причина» вместо протокола), `previewRowToolTip`, `showPreviewRowContextMenu` (`showPreviewRowInfoWindow` УПРАЗДНЁН — W13 заход 2, окно узла одно). **`previewAnnounceBlock` упразднён (W13)** вместе с питающим каналом в Preview: анонсы провайдера — записи тела, каждая своей строкой состава. `meta.provider_announce` (HTTP / `#announce:`) это ДРУГОЕ — сообщение о самой подписке; его дом Overview + отчёт сборки |
| Прочее | `source_edit_misc.go`, `source_meta_format.go`, `source_tag_shift_warning.go` | предупреждение о смене финального тега |

**business/**:

- `node_pool.go` — см. §1.3.
- `tag_guard_model.go` — `ModelTagOwners:30`, `ModelReplaceTags:70`,
  `ModelRootNodeTags:94`, `KnownRuleTargetTags:117` (модельная половина гарда).
- `folder_fill_subscription.go` (W11-правка) — неразобранные записи в папку не
  едут: заливка их пропускает, а материал из одних только них — отказ
  `ErrSubscriptionNotFetched` (пустой merge разыменовал бы все прежние копии).
- `fetch_writeback.go` — `ApplyFetchSnapshot(m, snapshot, revAtStart):37`,
  `applyFetchResultFields:77` (запись результата фонового fetch в живую модель
  под проверку ревизии).
- `node_move.go` (SPEC 116 W2 + W5) — операции над узлом контейнера:
  `CopyNodeToFolder:63`, `MoveNodeToFolder:95`, `ExtractFolderNodesToRoot:227`,
  `DereferenceNodeOrigin:288`; W5 добавила `RepointContainerNodeLinks:312`
  (переименование НА МЕСТЕ: контейнер тот же, меняется вторая половина
  адреса) и `ClearContainerNodeLinks:331` (удаление: цели больше нет —
  ссылка гасится, а не уводится на соседа). Реестр переписи ссылок на УЗЕЛ —
  `repointNodeLinks:416` (detour / hops / members / `Group.Default`) +
  `repointGroupLinks:483`; ссылки корневого пространства, которые в папку
  указать не могут, только ПЕРЕЧИСЛЯЮТСЯ — `rootOnlyRefsToTag:145`.
  Механика: `lookupNodeForMove:513`, `lookupFolderIndex:540`,
  `placeNodeIntoFolder:559`, `removeNodeFromSource:574`, `containerRefOf:592`,
  `rootTagSet:605`, `uniqueTagIn:628`, `cloneCanonicalNodeForMove:645`.
  Побочки (`BumpRevision`/`InvalidateNodePool`/`MarkAsChanged`/диалоги) — на
  вызывающем UI, как у `ResetDetourNodeRefs`.
  **Не путать с `ResetDetourNodeRefs` (`detour_refs.go:35`)**: тот ГАСИТ и
  знает только `Source.Detour` — он путь Save для ВЕРХНЕГО узла; у узла
  контейнера ссылок четыре вида, и при переименовании их надо переписать,
  а не погасить.
- `rule_target_reset` — `ui/configurator/presentation/rule_target_reset.go:35`
  `(*WizardPresenter).resetForeignRuleTargets`.
- `source_input.go` (SPEC 116 W6) — **единственный разбор** «текст → узлы» на
  все пути Add: `parseSourceInput:70` (`carveSingboxJSON` → `classifyInputLines`
  → `config.MaterializeServerNode`), результат `parsedSourceInput:42`
  (`Subscriptions` / `Nodes` / `URIOf` для дедупа корня / `Unnamed` для имени
  из файла). Куда положить — решает вызывающий: корень (`AppendURLsToSources`,
  `sources.go:35`) или папка (`AppendNodesToFolder`). Второго разбора не
  заводить — ловушка «эмиттер и парсер ходят парой» (дыра Д6).
- `folder_fill.go` (SPEC 116 W6) — `AppendNodesToFolder:54` (узлы в хвост
  `Source.Nodes`, тег уникализируется общим `uniqueTagIn` в пределах ЭТОЙ
  папки), `FolderFillResult:106` (`Added` + `SkippedSubscriptions`),
  `ErrSubscriptionInFolder:120` (сентинел: UI подменяет его переведённым
  текстом, сравнения по подстроке нет), `TagFromFileName:132`.
  Подписка в папку не кладётся: вложенных контейнеров нет.
- `folder_fill_subscription.go` (SPEC 116 W7) — заливка подписки в папку:
  `FillFolderFromSubscription:74` (материал = уже материализованные
  `sub.Nodes`, скопированные `cloneCanonicalNodeForMove`; `Truncated` — из
  `sub.UpdateStatus`; звонок в `corestate.MergeFolderNodesFromSubscription`),
  `FolderSubscriptionFillResult:54`, `ErrSubscriptionNotFetched:50` (сентинел
  «подписку ни разу не обновляли» — UI предлагает обновление),
  `FolderFillSubscriptions:143` + `FolderFillSubscriptionChoice:125` (список
  доноров), `lookupSubscriptionIndex:172`. **Второго разбора тела нет и быть
  не может** — тело подписки в модели не хранится вовсе, и «дозаливка через
  повторный fetch» здесь была бы вторым конвейером разбора (ловушка «сборка
  не парсит тела подписок»).
- `sources.go:157` `NextFolderName` (SPEC 116 W3) — свободное «Folder N».
  В отличие от `NextChainLabel:131` это ИМЯ контейнера (`Source.Name`), а не
  тег: занятость считается по именам папок и подписок, ссылок на него нет.
- `detour_refs.go:111` `SourceDisplayName` — как источник зовут пользователю.
  У КОНТЕЙНЕРОВ (папка/подписка) первым читается `Name`, у узловых kind'ов —
  `Label`: порядок обязан совпадать с `corestate.displayName()`
  (`adapter_source.go:230`), иначе одна и та же папка звалась бы в диалогах
  и в списке по-разному (SPEC 116 W4).
- Смежное: `sources.go:115`, `sources_json.go:156,174` (мутации + BumpRevision),
  `clone_source.go`, `detour_refs.go` (`ResetDetourNodeRefs`),
  `direction_rename.go`, `source_node_counts.go` (счётчик из `nodes[]`, НЕ превью-кэш).

**Смежное вне пакета** (SPEC 116 W6):

- `internal/platform/file_dialog.go` — `PickOpenFile:24` (одиночный),
  `PickOpenFiles:48` (**мультивыбор**), `splitPickedPaths:66` (разделитель —
  перевод строки: единственный байт, которого не бывает внутри пути ни на
  одной из трёх ОС). Реализации: `file_dialog_darwin.go`
  (`with multiple selections allowed` + цикл `POSIX path of`),
  `file_dialog_windows.go` (`$d.Multiselect` + `$d.FileNames`),
  `file_dialog_linux.go` (zenity `--multiple --separator=\n`; у **kdialog**
  мультивыбора без разделителя нет — его ветка осознанно одиночная),
  `file_dialog_stub.go`.
- `ui/configurator/dialogs/` — у `ShowAddWarpDialog` и `ShowAddServerDialog`
  появился параметр `owner fyne.Window` (`nil` = главное окно визарда).
  Окно источника — отдельное `app.NewWindow` (`source_edit_window.go:453`), и
  диалог, прибитый к главному окну, всплывал бы за спиной у пользователя.

**Модель**: `ui/configurator/models/wizard_model.go` — `BumpRevision:280`,
проекция `AsParserConfig()` (одноразовая, см. ловушки).

---

## 2. ТАБЛИЦА СВЯЗЕЙ (кто кого зовёт)

### 2.1 fetch → merge → save

```
UI: source_tab.go:1203 refreshOneSourceFromUI
    │  (или фон: core/config_service_subscriptions.go:43 refreshSubscriptionsMetaAndCache)
    ▼
core/config_service_subscriptions.go:113 refreshOneSubscriptionSource
    ├─ HTTP + декод тела
    ├─ :210 resolveSubscriptionMaxNodes   (подписка → настройки приложения)
    ├─ :159 config.MaterializeSubscriptionBody   ← ЕДИНСТВЕННЫЙ разбор тела
    │        (skip[] → дедуп по подписи → уникализация сырых тегов → кап)
    ├─ :173 state.MergeSubscriptionNodes(src, material, trusted)
    │        (merge по СЫРОМУ тегу; trusted=false → nodes[] не трогаются)
    └─ :248 successSubStatus / :224 failedSubStatus → Source.UpdateStatus
    ▼
диск (W13): persistFetchResultForSource:352 — только у пути ↻
    │  (RefreshSourceInPlace); load → поля fetch'а в запись этого id → Save
    ▼
UI-запись: business/fetch_writeback.go:37 ApplyFetchSnapshot(m, snapshot, revAtStart)
    │  вызовы: source_tab.go (refreshOneSourceFromUI:1222),
    │          source_edit_window.go (triggerOneShotFetch)
    ▼
m.BumpRevision() → InvalidateNodePool → State.Save (save.go:25)
```

Снимок для горутины fetch'а — ОДНА реализация на обе точки кнопки:
`deepCopySourceForFetch` (`source_edit_window.go:208`), зовётся из
`refreshOneSourceFromUI` и `triggerOneShotFetch`. Прежде окно копировало
Skip/Nodes/PendingDisabled, а строка — одну Meta, и её горутина уезжала с
backing-массивами живой модели. Новое ссылочное поле Source, которое читает
или пишет fetch, добавлять туда.

### 2.2 загрузка → миграция → v7

```
state.Load (load_router.go:30)
 ├─ sniffSchemaVersion:121            top "version" / "meta.version"
 ├─ writeLegacyBackupOnce:180         бэкап исходника ДО миграции (идемпотентно)
 ├─ parseWithContext:90 ─ роутер:
 │    meta>=7 → parseV7 (disk_v7.go:61)                    ← прямой путь
 │    meta==6 → parseV6Legacy (load_v6.go)   ┐
 │    meta==5 → parseV5Legacy (load_v5.go)   ├→ migrateLegacyStateToV7
 │    top 2-4 → parseLegacyAndMigrate        ┘   (migration_v6_to_v7.go:77)
 │                 ├ materializeSources:162 (raw-кэш → nodes[])
 │                 ├ migrateChainHops:456 / migrateDetours:527
 │                 ├ migrateFolds:623 (replace.tag материализуется явно)
 │                 └ applyRenames:701
 ├─ load_normalize.go:11 normalizeNilSlices / :45 sanitizeOutboundRefs
 ├─ PersistMigrationReport (migration_report_persist) → файл рядом в bin/
 └─ если migrationPurgesLegacy: Save → purgeLegacyAfterMigration:834 → Save
                                 (порядок обязателен: снос только после записи v7)
```

### 2.3 сборка (rebuild → генерация → резолв → гард → санитайзер → отчёт)

```
core/rebuild.go:100 (*AppController).RebuildConfigIfDirty
    ▼
core/config/outbound_generator.go:1094 GenerateOutboundsFromParserConfig
  проход 0:  PrepareDirections            (Направления + твины)
             PrepareFolderReplaces:1109   (свёртки папок → локальные группы)
             SubstituteParserConfigPlaceholders  (@vars — ПОСЛЕ обоих)
  шаг 1:     по источникам: EmitCanonicalSource:1171
             (только если ps.Canonical != nil — тела подписок НЕ парсятся)
               └ applyEmissionTagMachine → политика → normalize → MakeTagUnique
  резолв:    BuildNodeLinkTargets:1279     ← ОДИН словарь целей на всю сборку
             ResolveCanonicalChainHops:1282 (до ResolveChainSources!)
             ResolveChainSources
             ApplyCanonicalNodeLinks:1289   (detour fail-closed / members prune)
  гард:      BuildTagGuard:1301 → Conflicts() → предупреждения сборки
  санитайз:  sanitizeNodeDetours:1317 (кольца/самоссылки; выбрасывается УЗЕЛ)
             pruneNodesBySource
  шаг 2:     генерация JSON узлов (EmitNodeJSONs / generateCanonicalBodyJSON)
    ▼
core/build/outbound_graph_sanitize.go   финальный граф-санитайзер
core/build/dns_detour_sanitize.go       DNS-detour как ребро того же графа
    ▼
отчёт: rebuild.go:183 FeedBuildReportFromParser
       rebuild.go:187 FeedBuildReportFromFetchStatus  (из состояния подписок)
       rebuild.go:199 FeedBuildReportFromSanitizer
```

### 2.4 окно источника → модель → Save → BumpRevision

```
source_tab.go (строка списка) → showSourceEditWindow (source_edit_window.go:377)
   │  на открытии: cloneSource:190 → value-snapshot формы
   │  вкладки правят КОПИЮ: chainForm.Collect / replaceTab.Collect /
   │                        applyServerBodyJSON / regenServerBodyFromRaw
   │  Cancel → копия выбрасывается, модель не тронута
   ▼ Save
applySourceEditToModel:353
   ├─ mergeEditedSourceIntoModel:324
   │    runtime-поля берутся у ЖИВОЙ записи (Meta/Update/MaxNodes/Nodes/
   │    UpdateStatus/PendingDisabled) — фоновый fetch не затирается формой;
   │    enabledEdits накладываются ПОВЕРХ живых узлов
   ├─ m.BumpRevision()            (models/wizard_model.go:280)
   ├─ m.PreviewNeedsParse = true
   ├─ InvalidateNodePool(m)       (business/node_pool.go:150)
   ├─ presenter.RefreshOutboundsConfiguratorList / ScheduleRefresh…Debounced
   └─ presenter.MarkAsChanged + guiState.RefreshSourcesList
   ▼ если сменился тег узла
resetRefsAfterNodeRename:1571 → business.ResetDetourNodeRefs (detour_refs.go)
   → showDetourRefsResetDialog:1614 / showStaleSelectionDialog:1638
     (выбор в кэше ядра лаунчер переписать не может — только предупредить)
   ▼
State.Save (core/state/save.go:25) → MarshalV7:101
```

---

## 3. ВЫЖИМКИ

### 3.1 state v7 / хранение

- Корень плоский: `sources[] / directions[] / rules[] / dns_options / vars /
  warp_accounts / meta`. Обёртки `connections` не существует — её читает
  только вход миграции.
- `id` есть у ВСЕХ Source, но ссылаются по нему только на папки
  (`NodeLink.FolderID`); у узлов ULID остался ради `SourceRef` бэкапа и
  адресации UI-операций — ссылок на узлы по id в модели v7 нет.
- Память = диск: каноническая модель одна. `State.ParserConfig` —
  **read-only Load-проекция**, наполняется только на Load; писать в неё
  запрещено, и после мутаций canonical-полей она врёт (`state.go:50` шапка).
- Ревизия монотонна (`BumpRevision`); все производные (превью, фоновая
  генерация, отчёт) привязываются к ревизии на старте, устаревший результат
  выбрасывается. Строковых отпечатков не бывает.
- Два независимых dirty-признака: «конфиг устарел» (любая правка → пересборка,
  без сети) и «пора обновить подписку» (только расписание/руками → fetch).
  Правка `skip[]` действует со СЛЕДУЮЩЕГО обновления.
- Не хранится: выбор в manual-селекторе (кэш ядра), превью-кэши, raw-кэш
  подписок, цельное тело подписки, TTL-карта выключенных.

### 3.2 Источники / папки

- **Merge по СЫРОМУ тегу** — одно ядро на два контейнера. Совпал → body/origin
  свежие, `enabled`+`detour` живут (перенос пометок — `refreshMergedNode:43`);
  новый → добавлен включённым; исчезнувший → удалён (подписка,
  `MergeSubscriptionNodes:80`) / **разыменован** (папка,
  `MergeFolderNodesFromSubscription:175`).
- **Ключ заливки в папку — пара (`origin.subUrl == subURL`, сырой тег)**: узлы
  папки с другим `subUrl` или без него не участвуют, не двигаются и не
  трогаются. Позицию в папке задаёт пользователь — совпавший остаётся на
  своём месте (порядок тела провайдера НЕ навязывается), новый идёт в хвост.
  Занятый чужим узлом сырой тег → узел тела деградирует с warning, подмены
  и второго узла с тем же тегом не бывает.
- **Auto, приехавший заливкой, переуказывает members на папку**
  (`repointFolderAutoMembers`, SPEC 116 W7). Копией члена считается узел
  папки ЭТОЙ заливки (`origin.subUrl == subURL`), а не любой одноимённый:
  тег, отбитый коллизией, занят ЧУЖИМ узлом, и увести член на него значило бы
  молча подменить состав группы соседним узлом пользователя. Непопавший член —
  prune + warning (не fail-closed: группа живёт); выпавший из состава
  `Group.Default` снимается тем же проходом. Ручной Auto папки и заливку
  другой подписки проход не трогает — они не в `touched`.
- **trusted=false → nodes[] не трогаются вообще**; `Truncated=true` →
  обновлять и добавлять можно, удалять «исчезнувших» НЕЛЬЗЯ (`:115`), и в
  папке при truncated не выполняется разыменование (`:236`).
- `PendingDisabled` — одноразовое поле между импортом бэкапа/миграцией и
  первым достоверным fetch; применяется по сырым тегам и стирается
  (`subscription_merge.go:125`). Не TTL-карта.
- Порядок `nodes[]` после merge = порядок свежего тела; удержанные
  truncated-узлы уходят в хвост в прежнем относительном порядке.
- Тег-политика — косметика эмиссии: её правка не рвёт enabled, NodeLink и
  merge-ключ (все они на сыром теге).
- Папка — территория пользователя: подписка из неё не удаляет; вложенных
  папок нет; списка «подписок папки» не существует (связь только
  `origin.subUrl`).

### 3.3 Направления / эмиссия

- Пул кандидатов = верхние узлы + узлы папок без replace (по финальным тегам)
  + replace-теги свёрнутых папок. Больше ничего; поштучного «спрятать» нет.
- Свёртка убирает узлы ТОЛЬКО из пула Направлений — в `outbounds` они
  остаются и работают целями цепочек, detour и членами Auto.
- **detour/хопы — fail-closed** (носитель деградирует с ⚠, кольца тоже),
  **члены Auto — prune** (выпадают, группа живёт; без членов не эмитится).
  Разница намеренная: `nodelink_resolve.go:370` vs `:475`.
- Гард занятости — один на все виды тегов сразу (Направления + твины,
  replace-теги + их `-auto`, верхние узлы, системные): `tag_guard.go:148`.
  Направление `x` и replace-тег `x` дали бы два `x-auto`. При этом ОДНА
  сущность претендентом дважды не считается: развёрнутый твин и его родитель
  — это пара, а не коллизия (SPEC 116 W12 фикс 1).
- Реестр переписи ссылок обязан знать ВСЕ виды тегов из гарда — иначе первая
  загрузка сбросит живые правила на direct, приняв replace-теги за чужие
  (`KnownTargetTags:222`, `KnownRuleTargetTags` в модели).
- Твины и авто-половина replace исключают групповые кандидаты (Auto-узлы и
  replace-теги): urltest поверх чужой группы мерил бы чужой выбор.
- **Выключенные УЗЛЫ проходят тег-машину, выключенные ИСТОЧНИКИ — нет.**
  Узел: `buildCanonicalNode` отрабатывает, `emittedNum++` и слот уникализации
  потребляются, и только потом узел выбрасывается (`canonical_emit.go:106`) —
  иначе включение соседа обратно сдвигало бы чужие `{$num}` и суффиксы `-2`.
  Про невалидное тело выключенного узла warning НЕ пишется (`:97`): его не
  сломали, его выключили. Источник целиком с `Disabled` пропускается до
  эмиссии (`outbound_generator.go:1154` `continue`) и в `totalSources` не
  считается — его теги в уникализации не участвуют вовсе.
- Цепочка тег-машину не проходит и номера не потребляет: её тег задан прямо
  и уникализации не подлежит (`canonical_emit.go:90`).

### 3.4 Ловушки (проверять на каждой правке)

- **Сборка не парсит тела подписок.** `EmitCanonicalSource` зовётся только
  при `proxySource.Canonical != nil` (`outbound_generator.go:1171`). Канона
  нет = собирать не из чего; это состояние источника, а не вторая ветка
  конвейера. Не добавлять fallback-разбор.
- **Порядок ключей body.** `Node.Body` = ровно то, что эмиттер кладёт в
  config.json, минус `tag`/`detour`. Пересборка через `map` пересортирует
  ключи и сломает байт-эквивалентность эталонов. Только
  `core/config/body_keyorder.go` (`decodeOrderedJSONObject:37`,
  `setFirst:90`, `setLast:97`, `encode:123`).
- **Го1.20-гард.** Win7-джоба CI собирает весь модуль тулчейном go1.20:
  никаких `min`/`max`/`clear`/`slices.`/`maps.`/`PathValue`/`errors.Join`.
  Кампания 118 держит 0 вхождений — греп перед релизом.
- **Эмиттер и парсер ходят парой.** `GenerateNodeJSON` — per-scheme switch:
  новая схема без emit-ветки молча урезается до `{tag,type,server,server_port}`.
  Каждой схеме — эмиссионный тест.
- **Value-snapshot UI.** Окно источника правит КОПИЮ (`cloneSource:190`,
  `cloneCanonicalNode:162`); до `applySourceEditToModel:353` модель не
  тронута. `mergeEditedSourceIntoModel:324` обязан брать runtime-поля
  (`Nodes/Meta/UpdateStatus/PendingDisabled/Update/MaxNodes`) у ЖИВОЙ записи —
  иначе фоновый fetch, доехавший при открытом окне, затрётся снапшотом.
  Новое runtime-поле Source → добавить в этот список.
- **Порядок в проходе 0**: Направления → свёртки → `SubstituteParserConfigPlaceholders`.
  Развернуть replace после varsubst = `@urltest_*` в опциях авто-группы
  замены останутся неподставленными.
- **`ResolveCanonicalChainHops` строго до `ResolveChainSources`** — вторая
  строит узел по строковым тегам и о ссылках не знает
  (`outbound_generator.go:1282` перед `:1284`).
- **Пул и сборка — одни и те же вызовы.** `RebuildNodePool` обязан звать тот
  же `EmitCanonicalSource` + `BuildNodeLinkTargets` + `ResolveCanonicalChainHops`
  (`node_pool.go:73,97,98`); расхождение = баг #91 (фильтр Направления «не
  берёт» цепочку).
- **`replace.tag` в бэкапе.** В контракте 0.11 имя группы свёртки —
  ПОЗИЦИОННЫЙ дериватив (`legacyFoldPrefix:252`). Явный тег, с деривативом не
  совпавший, круг не переживает; расхождение обязан назвать ЭКСПОРТ
  (`replaceTagSurvivesExport:232`), молча подменять имя запрещено.
- **Fyne label min-width** (`showDetourRefsResetDialog:1614`): длинный `Label`
  без `Wrapping` задаёт окну min-width своей строкой и раздувает диалог.
- **Адрес узла = пара (контейнер, сырой тег).** Перенос меняет ОБЕ половины
  разом: `{folderId: A, tag: raw}` → `{folderId: B, tag: raw′}`; у верхнего
  узла `folderId` пуст, а тег финальный (у корня политики нет). Отсюда две
  ловушки `node_move.go`: (1) сравнивать при переписи ОБЕ половины — иначе
  одноимённый узел ЧУЖОЙ папки поедет вместе с переносимым; (2) `move` из
  корня в папку осиротит цели правил / `route.final` / DNS-detour /
  addOutbounds — переписать их нельзя (папку они не адресуют), поэтому
  `rootOnlyRefsToTag:145` их НАЗЫВАЕТ, и UI обязан показать (критерий A3:
  «либо переписана реестром, либо названа в предупреждении»).
- **Copy не двигает ссылки.** Оригинал остался на месте — переписывать
  нечего, и пустой список у `CopyNodeToFolder` не «забыт», а верен. При этом
  `origin.subUrl` копия СОХРАНЯЕТ (она участвует в будущей merge-заливке);
  обнуляет его только `DereferenceNodeOrigin` — от правки содержимого, не от
  переезда.
- **Операции над узлом НЕ буферизуются в scratch** (SPEC 116 W5). Окно
  источника правит value-snapshot, но Move/Copy затрагивают ДВА источника, а
  scratch знает один: буферизация оставила бы узел уже в чужой папке и ещё в
  исходной, а Cancel — два экземпляра с одним сырым тегом. Поэтому
  Move/Copy/Rename/Delete/reorder мутируют модель НЕМЕДЛЕННО (как fetch), и
  окно обязано перечитать scratch (`reloadScratch`,
  `source_edit_window.go:934`) — иначе следующий Save запишет снимок,
  снятый ДО операции, и молча откатит её.
- **Drag в `widget.List` ≠ drag в VBox** (`fynewidget.DragReorderGroup`).
  Список узлов контейнера — первый drag-список на виртуализированном
  `widget.List`, а тот ПЕРЕИСПОЛЬЗУЕТ объекты строк: обычный `Register`
  оставил бы уехавшую за экран строку висеть под прежним индексом, два
  индекса заявили бы одну полосу экрана и бросок ушёл бы в чужой слот.
  Отсюда `RegisterRecycled:163` (карта держится биекцией),
  `DragHandle.SetIndex:396` (захват перепривязывается на каждой привязке
  строки) и `Total`/`slots():193` (кламп по СПИСКУ ДАННЫХ, а не по видимым
  строкам). У VBox-списков `Total == 0` и поведение прежнее.
  **`to` у `OnReorder` отсчитывается по слайсу УЖЕ БЕЗ вырезанного элемента** —
  так его понимает `chainForm.moveHop` (`source_chain_tab.go:669`), и
  `moveNodeWithinSlice` (`preview_node_ops.go`) обязан понимать так же: захват
  один, и две разные семантики под ним — гарантированный баг.
- **Разбор входного текста один на корень и на папку** (SPEC 116 W6).
  `parseSourceInput` (`business/source_input.go:70`) — единственное место, где
  вставленный текст превращается в узлы; корень и папка отличаются ТОЛЬКО
  адресом назначения. Новая ветка «а сюда положим по-другому» обязана звать
  это же ядро, а не заводить свой `carveSingboxJSON`/`classifyInputLines`:
  расхождение двух разборов не даст ни одной ошибки компиляции и вылезет
  схемой, которая работает в Sources и молча урезается в папке (дыра Д6,
  ловушка «эмиттер и парсер ходят парой»).
- **`state.json` пишется ЦЕЛИКОМ, а писателей несколько** (W13). Визард
  сохраняет модель, авто-обновление и Update — свои дисковые копии; каждая
  запись перекрывает файл. Значит результат, доехавший только до одной из
  сторон, обречён: следующая полная запись другой стороны его выбросит, и
  ошибка выглядит как «сохранилось, но старое» (свежий mtime при старом
  `last_success_at`). Отсюда правило: КАЖДЫЙ вход fetch'а закрепляет свой
  результат на диске сам (`persistFetchResultForSource`), перенося только
  поля, которые он родил, — Save визарда не гарантирован ни по времени, ни
  вообще. Список переносимых полей обязан совпадать с
  `business.applyFetchResultFields`: они делят один смысл «что принадлежит
  fetch'у», и разойдясь, дадут разное состояние в памяти и на диске.
- **Ленивый кэш ≠ «данных нет»**: превью-кэша больше нет, счётчики читают
  `nodes[]` напрямую (`source_node_counts.go`). Совпадение имени
  `SourceNodeCounts` со старым кэшем — не воскрешение механики.
