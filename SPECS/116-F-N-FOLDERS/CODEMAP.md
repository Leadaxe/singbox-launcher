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
| `SourceKind` + константы | `sources_v7.go:23,25` | server/chain/auto/folder/subscription |
| `NodeLink{FolderID,Tag}` | `sources_v7.go:34` | FolderID=="" → корневое простр-во финальных тегов |
| `Origin{Kind,Raw,SubURL}` | `sources_v7.go:42` | kind: uri/wg_ini/json (`:52`); kind=warp НЕ существует |
| `TagPolicy{Prefix,Postfix}` + `IsZero` | `sources_v7.go:61,67` | маски нет; переменные — на эмиссии |
| `AutoStrategy` (alias) | `sources_v7.go:77` | = `configtypes.DirectionAuto`, 9 полей |
| `AutoGroup{GroupType,Default,Members,Strategy}` | `sources_v7.go:80` | Default — СЫРОЙ тег члена, живёт тут, не в Strategy |
| `Node{Kind,Tag,Enabled,Origin,Body,Detour,Hops,Group}` | `sources_v7.go:99` | Body чист от detour; Hops — chain only; Group — auto only |
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
| `MaterializeSubscriptionBody(subID, decodedBody, skip, capN)` | `core/config/fetch_materialize.go:42` |
| `SubscriptionFetchMaterial` | `core/config/fetch_materialize.go:24` |
| `MergeSubscriptionNodes(sub, res, trusted)` | `core/state/subscription_merge.go:50` |
| `SubFetchMaterial{Nodes,Truncated}` | `core/state/subscription_merge.go:20` |
| Оркестрация одного обновления | `core/config_service_subscriptions.go:113` `refreshOneSubscriptionSource` |
| Все подписки разом | `core/config_service_subscriptions.go:43` `refreshSubscriptionsMetaAndCache` |
| Резолв капа (подписка → настройки) | `core/config_service_subscriptions.go:210` `resolveSubscriptionMaxNodes` |
| Статусы | `failedSubStatus:224`, `successSubStatus:248` (там же) |
| Точки на один источник | `RefreshSourceInPlace:292`, `RefreshSingleSubscription:321` |

Вызовы, связывающие конвейер: `config_service_subscriptions.go:159`
(`MaterializeSubscriptionBody`) → `:173` (`MergeSubscriptionNodes`).

### 1.3 Эмиссия (`core/config/`)

**Материализованный канон → узлы**:

- `EmitCanonicalSource(ps, sourceIndex, tagCounts)` — `canonical_emit.go:75`;
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
- `ApplyCanonicalNodeLinks(...)` `:234` — detour fail-closed + members prune.
- `resolveCanonicalDetour:349`, `detectCanonicalDetourCycles:383` (кольца
  fail-closed), `resolveCanonicalGroup:455` (prune), `canonicalGroupFolder:507`.
- `warnWireguardDanglingDetours:329` — detour не применяется к WG (`:318`).
- `ResolveCanonicalChainHops(parserConfig, targets)` — `canonical_emit.go:262`.

**Свёртки папок** (`folder_replaces.go`):

- `PrepareFolderReplaces(parserConfig, tmplAutoOptions)` `:38` — разворот
  replace в локальные группы (проход 0, после Направлений, до varsubst).
- `buildReplaceGroups:61`, `FolderReplaceTags:132`, `FolderReplacePoolTag:151`,
  `FolderReplaceGroupTags:163`.

**Гард занятости тегов** (`tag_guard.go`):

- `TagGuard` `:38`, `NewTagGuard:46`, `Claim:52`, `Taken:68`, `Owner:77`,
  `Conflicts:85`, `Tags:94`; `TagOwnerKind` `:27`.
- `BuildTagGuard(directions, proxies, rootNodeTags, systemTags)` `:114`.
- `KnownTargetTags(guard, directions)` `:156` — множество для сброса
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
  `ParseFailedSourceReason:291`, `DroppedNodesForSource:312`, `ResetBuildReport:147`.
- Фиды: `core/build_report_feed.go` — `FeedBuildReportFromParser:33`,
  `FeedBuildReportFromFetchStatus:116` (диагностика из состояния, не из разбора),
  `FeedBuildReportFromSanitizer:245`.
- Вызовы: `core/rebuild.go:183,187,199`; UI-путь — `business/parser.go:152,193,197`,
  `business/create_config.go:216`.
- UI-модель отчёта: `ui/configurator/tabs/final_report_model.go`, вкладка
  `final_tab.go`.

### 1.5 Бэкап (контракт 0.11)

- Конвертеры границы v7↔0.11: `core/backup/convert_v7.go` —
  `exportFold:35`, `exportDisabledMap:62`, `exportNodeLinkRef:85`,
  `exportHops:104`, `exportChainSpec:120`, `importFold:130`,
  `importNodeLinkRef:153`, `importHops:172`, `importChainBody:189`,
  `importMaskTag:212`, `replaceTagSurvivesExport:232`,
  `legacyFoldPrefix:252`, `foldDerivedDirectionTags:267`,
  `resolveImportedHops:303`.
- `Export(s, opts)` — `core/backup/export.go:48`; `exportSubscription:226`,
  `exportServer:252`, `exportChain:178`, `exportSourceRef:155`.
- `Import(s, b, opts)` — `core/backup/import.go:134`; `importSubscription:296`,
  `importServer:371`, `importChain:397`, `backupReplaceTag:358`,
  `ensureSourceID:279`.
- Типы контракта — `core/backup/types.go`; UI — `ui/configurator/tabs/settings_backup.go`,
  `settings_backup_report_window.go`.

### 1.6 UI-конфигуратор

**Список источников**: `ui/configurator/tabs/source_tab.go` (1139) —
`CreateSourcesTab:57`, `applySourceMutation:816`, `showSourcePreviewAllWindow:848`,
`nodeDisplayLine:950`, `CreateDirectionsTab:976`, `refreshOneSourceFromUI:1033`.

**Окно источника**: `ui/configurator/tabs/source_edit_window.go` (1667) —
`showSourceEditWindow:377` (главный конструктор окна);
`cloneSource:190`, `cloneCanonicalNode:162`, `cloneDirection:264`
(value-snapshot формы), `mergeEditedSourceIntoModel:324`,
`applySourceEditToModel:353` (путь Save), `setNodeEnabled:90`,
`nodeEnabledInSource:107`, `sourceOriginURI:124`/`setSourceOriginURI:136`,
`resetRefsAfterNodeRename:1571`, `showDetourRefsResetDialog:1614`,
`showStaleSelectionDialog:1638`.

Вкладки окна:

| Вкладка | Файл | Точка входа |
|---|---|---|
| Overview | `source_edit_overview.go` | `buildOverviewTab:26`; `nodeOriginList:289`, `appendStorageRecordSection:235` |
| Chain | `source_chain_tab.go` | `newChainForm:111`, `Load:178`, `Collect:267`, `CollectLinks:249`, `applyChainFormToSource:862` |
| Chain (кандидаты хопов) | `source_chain_hops.go` | `collectChainHopCandidates:106`, `chainReplaceTags:242`, `chainFolderIDsBySourceIndex:255`, `chainReferencedBy:321` |
| Replace (свёртка) | `source_replace_tab.go` | `newReplaceTab:53`, `Load:113`, `Collect:145`, `defaultReplaceTag:205`, `replaceAutoChoices:219` |
| Body / JSON | `source_body_edit.go` | `applyServerBodyJSON:40`, `regenServerBodyFromRaw:71` (Regen) |
| JSON-рендер | `source_edit_json.go` | `renderUnpackedNodes:53`, `emittedToEditableJSON:38` |
| Прочее | `source_edit_misc.go`, `source_meta_format.go`, `source_tag_shift_warning.go` | предупреждение о смене финального тега |

**business/**:

- `node_pool.go` — см. §1.3.
- `tag_guard_model.go` — `ModelTagOwners:30`, `ModelReplaceTags:70`,
  `ModelRootNodeTags:94`, `KnownRuleTargetTags:117` (модельная половина гарда).
- `fetch_writeback.go` — `ApplyFetchSnapshot(m, snapshot, revAtStart):37`,
  `applyFetchResultFields:77` (запись результата фонового fetch в живую модель
  под проверку ревизии).
- `rule_target_reset` — `ui/configurator/presentation/rule_target_reset.go:35`
  `(*WizardPresenter).resetForeignRuleTargets`.
- Смежное: `sources.go:165`, `sources_json.go:156,174` (мутации + BumpRevision),
  `clone_source.go`, `detour_refs.go` (`ResetDetourNodeRefs`),
  `direction_rename.go`, `source_node_counts.go` (счётчик из `nodes[]`, НЕ превью-кэш).

**Модель**: `ui/configurator/models/wizard_model.go` — `BumpRevision:280`,
проекция `AsParserConfig()` (одноразовая, см. ловушки).

---

## 2. ТАБЛИЦА СВЯЗЕЙ (кто кого зовёт)

### 2.1 fetch → merge → save

```
UI: source_tab.go:1033 refreshOneSourceFromUI
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
UI-запись: business/fetch_writeback.go:37 ApplyFetchSnapshot(m, snapshot, revAtStart)
    │  вызовы: source_tab.go:1080, source_edit_window.go:919
    ▼
m.BumpRevision() → InvalidateNodePool → State.Save (save.go:25)
```

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

- **Merge по СЫРОМУ тегу** — `subscription_merge.go:50`. Совпал → body/origin
  свежие, `enabled`+`detour` живут; новый → добавлен включённым; исчезнувший →
  удалён (в подписке) / разыменован (в папке).
- **trusted=false → nodes[] не трогаются вообще**; `Truncated=true` →
  обновлять и добавлять можно, удалять «исчезнувших» НЕЛЬЗЯ (`:83`).
- `PendingDisabled` — одноразовое поле между импортом бэкапа/миграцией и
  первым достоверным fetch; применяется по сырым тегам и стирается
  (`subscription_merge.go:98`). Не TTL-карта.
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
  Разница намеренная: `nodelink_resolve.go:349` vs `:455`.
- Гард занятости — один на все виды тегов сразу (Направления + твины,
  replace-теги + их `-auto`, верхние узлы, системные): `tag_guard.go:114`.
  Направление `x` и replace-тег `x` дали бы два `x-auto`.
- Реестр переписи ссылок обязан знать ВСЕ виды тегов из гарда — иначе первая
  загрузка сбросит живые правила на direct, приняв replace-теги за чужие
  (`KnownTargetTags:156`, `KnownRuleTargetTags` в модели).
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
- **Ленивый кэш ≠ «данных нет»**: превью-кэша больше нет, счётчики читают
  `nodes[]` напрямую (`source_node_counts.go`). Совпадение имени
  `SourceNodeCounts` со старым кэшем — не воскрешение механики.
