# SPEC 118 · Этап 2 — задачи

Правила: каждая волна завершается зелёным `go build ./...` и прогоном
затронутых пакетов; финально — полный `go test ./... && go vet ./...`.
GUI-пакеты — скрипты `build/`. Git не трогать. `ui/traffic/**`,
`ui/machine_*.go`, `internal/lxdclient/**`,
`core/services/lxd_remote_registry.go`, `ui/machine_list_panel.go` не
трогать (чужие правки параллельной сессии). go1.20-гард: в новых правках
нет `min`/`max`/`clear`/`slices.`/`maps.`/`PathValue`/`errors.Join`.
Контракт `contract/**` не менять.

## W1 — Типы, схема v7, Load/Save, мост совместимости

- [x] `core/state/sources_v7.go`: `SourceKind`, `NodeLink`, `Origin`,
      `TagPolicy`, `AutoStrategy` (= alias `configtypes.DirectionAuto`),
      `AutoGroup`, `Node`, `Source` (плоский юнион, embedded Node),
      `FolderReplace`, `SubMeta` (из SubscriptionMeta минус
      fetch-история/PreviewNodes), `SubUpdateStatus` (+`FetchWarning`) —
      формы по PLAN §1.2.
- [x] `normalizeSourceShape`: по kind обнуляет нелегальные поля с warning;
      неизвестный kind — внятный отказ загрузки.
- [x] `core/state/disk_v7.go`: `diskStateV7` (плоский корень
      `meta/sources/directions/rules/vars/dns_options/warp_accounts`),
      `SchemaVersion = 7`, `SchemaName = "sources_v7"`.
- [x] `parseV7` / `marshalDiskV7`; `save.go` пишет только v7;
      `load_router.go`: 7 → parseV7, ≤6 → легаси-парс + миграция
      (в W1 — структурный перенос-заглушка, см. ниже).
- [x] `State`: `Sources []Source` + `Directions []configtypes.Direction`
      вместо `Connections ConnectionsSection`; механическая правка
      callsite'ов `Connections.Sources`/`Connections.Outbounds`
      (компиляционно-управляемая, семантику не менять).
- [x] Мост (PLAN §6): `adapter_source.go` — проекция v7 → ProxySource с
      ВРЕМЕННОЙ деривацией легаси-полей (DisabledNodes из enabled, Fold из
      replace, TagSpec из TagPolicy, тройня из NodeLink, Outbounds — пусто);
      маркер `// TEMPORARY BRIDGE (SPEC 118 W1-W4)`; build-пути core и
      raw-кэш работают как раньше.
- [x] Load v6 → структурный перенос в v7-формы без семантики (nodes[]
      пустые, легаси-значения доезжают до моста); гейт
      `migrationPurgesLegacy = false`.
- [x] `WizardModel`: `Sources` на новом типе; `Defaults` пока живёт
      (умирает в W5); точки BumpRevision не потеряны.
- [x] `cloneSource`/deep-copy окна источника обновлены под новые поля
      (Nodes, Replace, TagPolicy, NodeLink) — без slices./maps.
- [x] Тесты: `canonical_roundtrip_test.go` → v7-фикстура
      `testdata/v7_roundtrip.json` (папка с узлами, подписка с nodes[],
      chain c NodeLink-хопами, Auto, replace, directions, warp_accounts):
      Load→Save→Load→Save байт-в-байт; ID-стабильность папок.
- [x] Компиляционная правка тестов затронутых пакетов (семантику не
      менять; падающие по существу — пометить и отложить до своих волн).
- [x] `go build ./...` + `go test ./core/state/... ./ui/configurator/...`.

## W2 — Миграция v6 → v7 — СДЕЛАНО

- [x] `core/state/migration_v6_to_v7.go` (+`migration_report.go`,
      `migration_hooks.go`): 8 шагов features/state.md; вход —
      `migrateLegacyStateToV7` из парсеров v6/v5/v2–4 (v5/v2–4 получили и
      разворот WIZARD-маркеров — `adoptWizardMarkerFolds`); материализация
      через хуки `state.MigrationHooks`, реализация —
      `core/config/migrate_materialize.go` (init-подстановка, паттерн
      NodeIdentityFunc).
- [x] Шаг 1: nodes[] из raw-кэша НОВЫМ чистым парсером
      `subscription.ParseSubscriptionBody`
      (`core/config/subscription/parse_body.go`: skip/дедуп/уникализация/
      реальный кап, БЕЗ тег-политики/MakeTagUnique/ApplySourceDetour/
      filterDisabledNodes) — W3 расширяет и переключает fetch; кэша нет →
      nodes[] пуст + warning. Body корневых server (URI/config_json) тоже
      материализованы (Body+Origin). История SubMeta → SubUpdateStatus.
- [x] Шаг 2: DisabledNodes → enabled=false; legacy-64hex через
      LegacyNodeIdentityHash материализации; несматченные — warning;
      мостовая карта переписана на сырые теги (деривация legacyDisabledNodes
      не двоит ключи — согласованность моста проверена тестом).
- [x] Шаг 3: NodeTagOrLabel → Node.Tag (нормализация + глобальная
      уникализация старой машины — канонический сырой тег корня = прежний
      финальный); mask-шаблон подписки — warning; prefix/postfix живут;
      Label подписки → канонический Name.
- [x] Шаг 4: хопы → []NodeLink через индекс «финальный тег (старая
      тег-машина, общий tagCounts в порядке источников) → (folderId, сырой
      тег)»; Направления/твины/fold-теги легальны; нерезолвнутый →
      `NodeLink{"", тег}` + warning.
- [x] Шаг 5: тройня → NodeLink (подписка → folderId+сырой тег; верхний →
      голый финальный тег, включая уникализированный при коллизии;
      переходная форма без source_id → `NodeLink{"", тег}`; hash-only —
      резолв по хэш-индексу материализации, нерезолвнутый остаётся мосту с
      warning); detour у chain — отброшен с warning (типом не существует).
- [x] Шаг 6: fold → replace c материализованным деривативом
      (`FoldSelectTag`/`FoldAutoTag`, включая позиционный `N:` при пустом
      префиксе); mode both: `<PFX>auto` → `<tag>-auto`, перепись ссылок
      единым проходом (правила, CustomRules, route_final, опции
      Направлений, dns.detour, хопы легаси+канон, детуры) + warning о
      протухании выбора cache.db; произвольные локальные Направления —
      warning. МОСТ: `SourceFold.{Select,Auto}TagOverride` (TEMPORARY
      BRIDGE) — buildFoldGroups эмитит материализованные теги, чтобы
      эмиссия совпадала с переписанными ссылками; UI Fold-вкладка
      синхронизирует Replace (`syncReplaceFromFold`).
- [x] Шаг 7: standalone exclude_from_global — warning + отчёт.
- [x] Шаг 8 ПОД ГЕЙТОМ (`migrationPurgesLegacy=false` до W5): код сноса
      написан и проверен (`purgeLegacyAfterMigration` + wiring в Load
      «только после успешной записи v7»); defaults → `bin/settings.json`
      (`DefaultSubscriptionReload`/`DefaultSubscriptionMaxNodes`, не
      перетирая явное); бэкап-копия `<state>.v6.bak` пишется в Load ПЕРЕД
      миграцией (идемпотентно, O_EXCL).
- [x] `MigrationReport` из Load (`State.Migration`, только память) →
      диалог один раз на процесс (`ui/configurator/
      migration_report_dialog.go`, wrap-label в скролле — ловушка
      min-width Fyne) + лог каждой строки.
- [x] Сид легаси-формы — тем же путём (сценарий 9); remote-профили — общий
      Load (deriveLoadContext знает и remote/<id>/subscriptions).
- [x] ЭТАЛОНЫ старого движка (сняты ДО правок W2, нужны W8) →
      `SPECS/118-F-N-STATE_V7/etalon/`: полный `real-v088.config.json`
      (выхлоп текущего движка на golden-сценарии) + `v6mig/` (v6-фикстура
      с raw-кэшем и снимок эмиссии outbound'ов; харнес —
      `core/etalon_v6mig_capture_test.go`, `ETALON_V6MIG=1|capture`,
      по умолчанию skip). РЕШЕНИЕ по месту: полноформатный config.json —
      real-v088; для v6mig — уровень эмиссии (слой BuildConfig кампания не
      трогает, см. etalon/README.md). Известное расхождение Р2
      (`[P]auto`→`[P]select-auto`) задокументировано там же — кандидат O3.
- [x] Фикстуры и тесты §4.B п.1–9 —
      `core/state/migration_scenarios_test.go` (внешний state_test: хуки
      материализации живут в core/config): материализация+эквивалентность
      body старой эмиссии, отметки (raw+hex+несматченные), теги и
      mask-warning, хопы (подписка/Направление/fold-тег/призрак), тройня
      (оба вида + переходная + коллизия верхних узлов), fold трёх режимов
      + позиционный пустой префикс + перепись both-ссылок, потери
      (exclude + локальные Направления), снос/бэкап/идемпотентность
      (`PurgeLegacyForTest`), сид. Каркасные тесты чистого парсера —
      `parse_body_test.go` (регресс v1.5.2, `X,X-2,X`, кап, skip,
      selector-группы).
- [x] `go build ./...` + `go test -count=1 ./...` + `go vet ./...` —
      зелёные; греп go1.20 по диффу — чисто.
- [x] Попутный фикс (deterministic red в `./ui/configurator/tabs`):
      `ensureRemoteInterfaces` не считал загруженный ответ демона
      закрытым вопросом и повторял REST на каждый вызов —
      `TestLANIfaceCandidatesShareUplinkCache` падал; добавлен guard
      `e.loaded` (failed по-прежнему retry).

## W3 — Fetch/merge: материализация — СДЕЛАНО

- [x] `subscription.ParseSubscriptionBody` (создан в W2) — в W3 расширен:
      per-record деградации sing-box-импорта (потерянные члены групп —
      `SingboxImportResult.Warnings`, singbox_groups.go) текут в общий
      поток warnings разбора, а не только в лог (Т3 «не молча»).
- [x] `state.MergeSubscriptionNodes` (`core/state/subscription_merge.go`):
      merge по сырому тегу (освежить / добавить / удалить), порядок =
      порядок свежего тела (удержанные truncated-узлы — в хвосте);
      truncated → удаление запрещено; trusted=false → nodes[] не тронуты;
      `pending_disabled` (вердикт O2, новое поле Source) применяется на
      первом достоверном fetch и стирается (при truncated несматченные
      теги выживают); смена вида узла теряет detour с warning; мостовая
      карта DisabledNodes синхронизируется с каноном
      (`syncLegacyDisabledMap`, TEMPORARY BRIDGE — прежний
      GCDisabledNodes-проход в sweep умер, TTL целиком умирает в W5).
- [x] `config_service_subscriptions.go`: refreshOneSubscriptionSource →
      скачать, SubMeta-заголовки (мостовая Meta живёт до W6),
      `config.MaterializeSubscriptionBody` (`core/config/
      fetch_materialize.go`; общий с миграцией конвертер
      canonicalNodeFromEntry — body fetch = body миграции байт-в-байт),
      Merge, канонический updateStatus (last_attempt/success, ошибки,
      truncated, nodes_count, warnings parse+merge); недостоверность
      113-A: ошибка сети / пустое тело / обрыв / «ноль записей при
      per-record деградациях» (HTML вместо тела) → nodes[] нетронуты;
      Save + MarkConfigStale (RefreshSingleSubscription) и BumpRevision
      (UI in-place пути) — без автозапуска сборки; писатель raw-кэша
      остаётся (мост до W5). Мутации только новыми slice/pointer'ами —
      value-snapshot UI не делит объекты с горутиной.
- [x] Резолв капа (`resolveSubscriptionMaxNodes`): sub.MaxNodes →
      `settings.DefaultSubscriptionMaxNodes` → (мост) legacy defaults →
      клэмп 3000 в парсере. Резолв интервала (`effectiveReload`,
      auto_update.go): sub.Update → `profile-update-interval` заголовок →
      `settings.DefaultSubscriptionReload` → (мост) defaults.Reload →
      встроенный 1ч; staleness — по каноническому
      updateStatus.last_attempt_at (фолбэк Meta). Чтение дефолтов —
      `locale.LoadSettings(bin)`; поля UI — в W6.
- [x] Авто-fetch свежедобавленной подписки — как сегодня: пути
      RefreshSourceInPlace/RefreshSingleSubscription целиком на новом
      конвейере (nodes[] наполняются на любом fetch).
- [x] Тесты SPEC §4.D (1–9): `core/subscription_fetch_test.go`
      (httptest-стабы тел, без сети) — skip со следующего fetch, дедуп
      32→1 + перепривязка членов, «X, X-2, X», реальный кап (подписка и
      настройки приложения) + truncated, merge-пометки, 113-A
      (ошибка/пусто/мусор/truncated без удаления), body без detour/tag,
      Auto-материализация (NodeLink на свою подписку, selector
      type+default, вложенный член — warning), pending_disabled,
      «не фетчилось» на fetch-уровне (warning отчёта сборки — читатель
      updateStatus, волна W4). Merge-юниты —
      `core/state/subscription_merge_test.go`.
- [x] `go build ./...` + `go test -count=1 ./...` + `go vet ./...` —
      зелёные; греп go1.20 по правкам — чисто.

## W4 — Эмиссия, резолв, гард, пул — СДЕЛАНО

- [x] Сборка из nodes[]: `Source.canonicalProjection()` →
      `ProxySource.Canonical` (`configtypes.CanonicalSource/CanonicalNode`,
      `json:"-"`); `EmitCanonicalSource` (`core/config/canonical_emit.go`)
      строит узлы из body — конвейер сборки парсер тел не зовёт (тест
      `runCanonicalBuild` падает, если loadNodesFunc сработал). Мостовой
      путь остаётся у источников БЕЗ канона (материализация не прошла) —
      умирает в W5 вместе с raw-кэшем.
- [x] Тело эмитится КАК ЕСТЬ с возвратом `tag`/`detour` на прежние места
      (`core/config/body_keyorder.go`: порядок ключей не теряется).
      `stripTagAndDetour` переписан на тот же ordered-разбор — без этого
      каркасная W2-сортировка ключей ломала байт-равенство на КАЖДОМ узле.
- [x] Эмиссионная тег-машина: политика(prefix/postfix + переменные) →
      NormalizeProxyDisplay → глобальный MakeTagUnique. Вход политики —
      ПРОВАЙДЕРСКИЙ тег из origin.raw (`subscription.ProviderTagFromLabel`),
      а НЕ уникализированный сырой тег канона: старая машина уникализировала
      финальный тег (`[P] NL-1 •-2`), и подмена дала бы `[P] NL-1-2 •`
      (Р1). Подпись/комментарий восстанавливаются из origin
      (`subscription.LabelFromOriginURI`) — для `{$label}`/`{$comment}` и
      фильтров.
- [x] `core/config/nodelink_resolve.go`: `NodeLinkTargets` (узлы папок по
      сырым тегам + корневое пространство финальных тегов + Направления +
      твины + replace-теги + системные), `ApplyCanonicalNodeLinks` —
      detour fail-closed с каскадом до фикспойнта и кольцами,
      members prune; `ResolveCanonicalChainHops` — позиции цепочек до
      ResolveChainSources (нерезолвнутая уезжает сырой и роняет цепочку
      целиком). WG-исключение detour — в точке применения.
- [x] Папочный общий detour: Server-узлам без личного, пропуская
      Chain/Auto (`applyCanonicalDetourLink`).
- [x] Auto-эмиссия: выключенные члены не рождаются вовсе, битые — prune с
      warning; пустая группа не эмитится + warning; default только у
      selector и только из состава.
- [x] `PrepareFolderReplaces` (`core/config/folder_replaces.go`): явный
      tag, both → `<tag>-auto` через `buildTwin`, `NoGroupMembers` (новый
      build-only флаг Direction) исключает Auto-узлы папки из авто-состава,
      селекторная половина — опции шаблонных групп. WIZARD-маркеры не
      пишутся; `PrepareSourceFolds` пропускает канонические источники.
- [x] Пул кандидатов (`FilterNodesExcludeFromGlobal` +
      `collectExposeTagCandidates`): верхние узлы + узлы папок без replace
      + replace-тег свёрнутой (итог свёртки, не её внутренность). Твины
      уже исключали Auto-узлы (`dropGroupNodes`) и expose-кандидатов
      (`TwinOf != ""`).
- [x] `core/config/tag_guard.go`: `TagGuard`/`BuildTagGuard`/
      `KnownTargetTags`; вызывается на сборке (конфликты → EmissionWarnings
      + WARN). Модельная сторона — `ui/configurator/business/
      tag_guard_model.go` (`ModelTagOwners`/`ModelReplaceTags`/
      `ModelRootNodeTags`/`KnownRuleTargetTags`), подключена к
      `DirectionTagTaken`.
- [x] `rule_target_reset`: known = `KnownRuleTargetTags` (replace-теги,
      `-auto`-двойники, верхние узлы, выключенные Направления) + прежние
      доступные цели.
- [x] dns.detour — ребро outbound-графа: `core/build/
      dns_detour_sanitize.go` (`SanitizeDNSDetours` в `buildSection` для
      секции dns, `DNSDetourTags` для реестра). Ключ снимается, сервер
      живёт: у DNS-сервера detour это «через какой канал резолвить», а не
      анонимность (в отличие от detour узла — там выбрасывается носитель).
- [x] Отчёт сборки: новые виды `fetch_degraded` (из
      `update_status.warnings` — `FeedBuildReportFromFetchStatus`,
      адресация «источник → сырой тег», плюс «ни разу не обновлялась») и
      `emit_degraded` (`OutboundGenerationResult.EmissionWarnings`).
      Зовётся из обеих точек — боевой Rebuild и «Итог» визарда.
- [x] Мост сужен: `narrowBridgeForCanonical` снимает у канонического
      источника детур-тройню, Fold, DisabledNodes и exclude/expose —
      иначе те же рёбра резолвились бы дважды с разной строгостью.
- [x] Тесты §4.E 1–9 (`core/config/canonical_emit_test.go`), §4.E.8
      (`core/build/dns_detour_sanitize_test.go`), §4.B.10
      (`TestMigrationScenario10RuleTargetsSurviveEmission` +
      `ui/configurator/business/tag_guard_model_test.go`), Р8
      (`core/build_report_fetch_status_test.go`). Три W2-теста
      мостовых деривации переписаны на канонические проекции (предмет
      деривации W4 упраздняет).
- [x] Самопроверка эталонами: `ETALON_V6MIG=1` даёт РОВНО одно
      задекларированное расхождение Р2 (`[P]auto` → `[P]select-auto`),
      других нет. `real-v088` — golden берёт готовый `cache.json` и слой
      эмиссии не трогает; `go test ./core/build` зелёный.
- [x] `go build ./...` + `go vet ./...` + `go test -count=1 ./...` —
      зелёные; греп go1.20 по диффу — чисто.

### Фикс-раунд W4 (по REJECT ревью)

- [x] БЛОКЕР: выключенный узел ПРОХОДИТ тег-машину и отбрасывается ПОСЛЕ
      неё (`EmitCanonicalSource`) — потребляет и `{$num}`, и слот
      глобальной уникализации, ровно как старый движок
      (`filterDisabledNodes` шёл после `applyURINodeTags`). Иначе у
      соседей сдвигались финальные теги: [A, выкл. B, B] дал бы третьему
      `B` вместо `B-2`, протухнув выборами cache.db и ссылками. Тесты —
      `TestEmitDisabledNodeStillConsumesUniqueSlot`,
      `TestEmitDisabledNodeConsumesNumVariable`.
- [x] Отчёт сборки: подписка с `UpdateStatus.LastStatus=="err"` и узлами
      прошлого успеха видна (`lastFetchFailedReason` в
      `FeedBuildReportFromFetchStatus`) — причина провала + дата узлов,
      из которых собран конфиг; без успеха в истории строка остаётся
      одна («ни разу не обновлялась»). Тесты —
      `core/build_report_fetch_status_test.go`.
- [x] `TagPolicy.Mask` больше не игнорируется молча:
      `canonicalMaskBridgeWarnings` кладёт warning в `EmissionWarnings`
      (только у КОНТЕЙНЕРА — у server/chain/auto мостовой `TagMask` несёт
      тег узла), поле формы задизейблено с той же подписью
      (`source_edit_window.go`, вкладку снимает W6).
- [x] WG-detour с висячей целью — `warnWireguardDanglingDetours` (один
      раз, до фикспойнта); носитель не роняется (detour к WG неприменим),
      но две неработающие настройки сразу названы вслух.
- [x] Пересамопроверка: `ETALON_V6MIG=1` — по-прежнему РОВНО одно
      расхождение Р2; выхлоп `real-v088` байт-в-байт равен эталону W2
      (`SPECS/118-F-N-STATE_V7/etalon/real-v088.config.json`), остаточное
      падение `GOLDEN_RUN_REAL` — предсуществующий
      `dns.independent_cache`. Полный прогон зелёный.

## W5 — Смерть легаси — СДЕЛАНО

Отчёт: `SPECS/118-F-N-STATE_V7/reports/W5_REPORT.md` (снесённые
файлы/символы, судьба тестов с причинами, таблица grep-инвариантов,
результаты приёмки).

- [x] Включён снос миграции (шаг 8): `migrationPurgesLegacy = true`.
- [x] Удалены: `core/state/raw_cache.go`, `core/rebuild_raw_cache.go`,
      `LookupCachedBody`, писатель raw-кэша в fetch-сервисе. Чтение и
      удаление кэша живут ТОЛЬКО во входе миграции
      (`core/state/migration_raw_cache.go`, приватный). Снимок сборки —
      `core/rebuild_snapshot.go` (`buildSnapshotFromState`): эмиссия из
      материализованных `nodes[]`, без сети и без разбора тел.
- [x] Удалена disabled-машинерия: карта в типах состояния и в сборочной
      форме, `filterDisabledNodes`, `migrateLegacyDisabledKeys`,
      `GCDisabledNodes`, TTL, `syncLegacyDisabledMap`, UI-кэши отметок.
      debugapi disabled-nodes API умер вместе с полем (сериализация
      состояния — enabled узла); UI-тумблер пишет `node.enabled`.
- [x] Удалён fold: `SourceFold`, `PrepareSourceFolds`, `EffectiveTagPrefix`,
      WIZARD-маркеры. Fold-вкладка окна источника → вкладка Replace
      (`source_replace_tab.go`): те же контролы mode/strategy плюс явное
      поле tag; новых глифов не введено.
- [x] Удалены `ExcludeFromGlobal`/`ExposeGroupTagsToGlobal` и ветки
      `outbound_filter` (`FilterNodesExcludeFromGlobal` →
      `FilterDirectionCandidatePool`: решает ПРАВИЛО пула, не флаг).
- [x] Удалены локальные Направления: поле состояния, scope конфигуратора,
      `localSubscriptionGroupTags`-цели detour. Группы ЗАМЕНЫ свёрнутой
      папки разворачиваются проходом 0 в build-only поле
      `ProxySource.LocalGroups` (`json:"-"`, на диск не едет).
- [x] Удалены `TagSpec.Mask` (тип `TagSpec` умер целиком — остался
      `TagPolicy{prefix,postfix}`; маска читается только миграцией и
      бэкап-границей), `Meta.PreviewNodes`, превью-кэши
      (`RebuildPreviewCache`/`PreviewNodesBySource`/`SourceNodeCounts`/
      `PreviewCacheGeneration` → `NodePool*`, источник — эмиссия канона,
      а счётчики читаются прямо из `nodes[]`), `Defaults` из state-файла
      (ключ `legacy_defaults`) и `WizardModel.Defaults`.
- [x] Удалена detour-тройня из прод-типов (`resolveNodeDetours`,
      `migrateLegacyDetourNodeHash` и вся её обвязка) и `Chain.Hops`
      []string-форма: цепочка стала узлом канона (`hops []NodeLink`).
- [x] Удалён мост §6 целиком: `grep -rn "TEMPORARY BRIDGE"` → 0.
      `GenerateOutboundsFromParserConfig` больше не принимает
      `loadNodesFunc` — «сборка полезла разбирать тела» невыразимо по
      построению.
- [x] Тесты категории (б) удалены с причинами (таблица §5 отчёта:
      `e2e_disabled_flow_test.go`, `expose_exclude_test.go`,
      `raw_cache_test.go`, `disabled_nodes_*`, `preview_nodes_test.go`,
      `rebuild_raw_cache_test.go` и остатки — 22 файла); переработка —
      §5 отчёта.
- [x] Grep-инварианты SPEC §4.A прогнаны — таблица в §6 отчёта; кода по
      всем инвариантам ноль (остаточные вхождения — комментарии и
      санкционированные читатели миграции).
- [x] `go build ./...` + `go vet ./...` + `go test -count=1 ./...` —
      зелёные. `ETALON_V6MIG=1` — РОВНО одно задекларированное
      расхождение Р2 (`[P]auto` → `[P]select-auto`), других нет;
      `go test ./core/build` зелёный.

### Компенсации, вытянутые в W5 сносом (не расширение объёма)

- [x] ДЫРА ПЛАНА: у цепочки в v7 не было дома для настроек маршрута
      (`idle_timeout`/`strip_evasion`/`strip`/`rewrite`) — они умерли бы
      вместе с `Source.Chain`. Настройки переехали в `Node.Body` (тот же
      дом, что у тела сервера), позиции остались ссылками в `hops`;
      конвертеры `configtypes.ChainBody`/`ChainFromBody`. Живые фичи
      поправлены той же правкой: `features/sources.md` (устройство Chain),
      `features/state.md` (шаг 4 миграции). Эталон подтверждает: цепочка
      сохранила `idle_timeout: 2m`.
- [x] `config.MaterializeServerNode` — единственная точка «share-URI или
      JSON → тело узла» для миграции, добавления источника, вкладки JSON
      и Regen (emitter-parser-pairing).
- [x] `core/backup/convert_v7.go` — конвертеры границы v7 ↔ контракт 0.11
      (W7 достраивает roundtrip-тесты и remote-гейт). Контракт и корпус
      не тронуты.
- [x] Минимум Т8, без которого снос ломал окно источника: вкладка JSON
      server-узла = редактор `body` + «Regen from raw» (ошибка разбора →
      откат), JSON подписки — read-only рендер тел, Overview — счёт узлов
      и per-node `origin.raw` вместо блока raw body.
- [x] `core/state/legacy_fixture_copy_test.go`: с включённым сносом Load
      переписывает файл на месте — тесты обязаны читать КОПИЮ фикстуры,
      иначе первый же прогон уничтожает testdata.

## W6 — UI-компенсация

- [x] Счётчики и превью источников из nodes[] (`source_tab.go`);
      состояние «не фетчилось» — из updateStatus.
- [x] Окно источника: JSON-вкладка server = редактор body; «Regen from
      raw» (ошибка разбора → откат, узел не портится); JSON подписки —
      синхронный body-рендер, read-only.
- [x] Overview: raw-body-блок → счёт узлов + per-node origin.raw;
      Storage record = v7-запись.
- [x] Пул хопов цепочек из nodes[] (+ Направления, replace-теги, верхние);
      выбор хранится NodeLink.
- [x] Settings приложения: Default update interval + Default max nodes →
      `bin/settings.json`; двухступенчатый резолв виден в подсказках
      существующими средствами (без новых виджетов-вопросов).
- [x] Предупреждение о протухании выбора cache.db при операциях смены
      финального тега (правка TagPolicy, replace.tag) — существующим
      механизмом предупреждений.
- [x] Поведенческие тесты Т8 (Regen-откат, счётчики, кандидаты хопов);
      без ассертов на форматирование строк.
- [x] Хвост W2: `MigrationReport` персистится в `bin/migration_report.txt`
      (Load), показывается при первом открытии конфигуратора вместе с
      отчётом из памяти и снимается после показа; отчёты профилей
      дописываются.
- [x] Хвост W3: write-back one-shot fetch (окно источника и строка Sources)
      сверяет ревизию модели — при правке во время полёта заносятся только
      поля результата fetch'а, а не снимок целиком
      (`business/fetch_writeback.go`).
- [x] Хвост W3: противоречие «мостовая Meta штампует ok при недостоверном
      теле» проверено мёртвым — статус живёт только в `SubUpdateStatus`
      (пишется на достоверном merge), заголовки — в `SubMeta`; `sourceDiag`
      держит обе половины раздельно.
- [x] `go build ./...` + GUI-скрипты `build/`.

## W7 — Бэкап-конвертеры и remote-гейт — СДЕЛАНО

Отчёт: `SPECS/118-F-N-STATE_V7/reports/W7_REPORT.md` (найденная дыра с тегом
замены, устройство гейта, форма Debug API, таблица приёмки).

- [x] `core/backup/convert_v7.go`: экспорт v7 → 0.11 (enabled →
      disabled-карта по сырым тегам; replace → fold; NodeLink → тройня;
      hops → строковые финальные теги; TagPolicy → tag{prefix,postfix};
      NodeTag = Node.tag; nodes[] не экспортируются). Конвертеры написаны
      в W5 как компенсация сноса; W7 достроил тесты и НАШЁЛ дыру:
      `replace.tag` в контракте дома не имеет (в 0.11 имя группы было
      позиционным деривативом префикса), и явный тег, с деривативом не
      совпавший, круг не переживает — правила приёмника уехали бы в
      никуда. Добавлены `replaceTagSurvivesExport` + код
      `backup_replace_tag_derived`: расхождение называет ЭКСПОРТ, где ещё
      видны оба имени. Формула свёрнута в общую `legacyFoldPrefix`
      (воспроизводит старый движок байт-в-байт, включая TrimSpace).
- [x] Импорт 0.11 → v7: обратные конвертации; mask server/chain →
      Node.tag (потери нет — warning не ставится), подписки — warning;
      локальные Направления: fold-производные → replace, прочие —
      warning; хопы: `importHops` + второй проход `resolveImportedHops`
      по живому индексу, нерезолвнутые остаются `NodeLink{"", тег}`
      (fail-closed на сборке, без ложного warning'а — обоснование в
      convert_v7.go).
- [x] ХВОСТ W3: disabled-карта импорта → `PendingDisabled` (вердикт O2) —
      закрыто ещё в W5 (`importSubscription`), W7 покрыл тестом.
- [x] Corpus-тесты `contract/corpus/backup/` (14 кейсов) зелёные без
      правки фикстур; §4.F.2 — `TestRoundTripV7ModelEquivalent` (все
      четыре конвертации + названные цены: nodes[] не едут, настройки
      маршрута цепочки живут в Node.Body) и
      `TestRoundTripV7ResolvesHopIntoContainer` (резолв не ВЫДУМЫВАЕТ
      адрес); §4.F.3 — `TestImportLegacy15xBackup` (fold+outbounds+
      disabled+mask одним файлом) и
      `TestImportLegacyServerMaskArrivesAsNodeTag`.
- [x] Remote-гейт: `core/state/schema_gate.go` — версия спрашивается у
      ФАЙЛА, а не у загруженного состояния (Load мигрирует и разбирает
      всё «мажор и выше» как v7 — после него гейтить нечем). Отказ 409 с
      обеими версиями в тексте и в полях `schema_found`/`schema_supported`.
      Направление значимо: файл СТАРШЕ пропускается (миграция), из
      БУДУЩЕГО — отвергается. GET не гейтуется (диагностика обязана
      работать в момент расхождения); copy-from гейтуется по ИСХОДНИКУ.
      Точки: `state_endpoints.go` (`stateAccess.path` + `guardStateSchema`
      в PATCH-ветках rules/dns), `remote_state_endpoints.go` (path
      machine-доступа), `remote_endpoints.go` (copy-from).
      `lxd_remote_registry.go` не тронут. Тесты §4.G —
      `core/debugapi/schema_gate_test.go` (6 шт., включая локальный PATCH
      и «файл не тронут»); острота каждого гейта проверена временным
      отключением.
- [x] Debug API: `/state/full` (и remote-близнец) отдавал Go-структуру
      `state.State` БЕЗ json-тегов — наружу текли PascalCase-ключи и
      мёртвые поля загрузчика (`Defaults`, `SelectableRuleStates`,
      `RulesLibraryMerged`, `ParserConfig`), а `dns` терялся. Введена
      `(*State).MarshalV7` (обёртка чистой `marshalDisk`): сериализация у
      endpoint'а и у файла теперь ОДНА по построению. `TestStateFull`
      переписан под форму — ответ разбирается тем же `state.Parse`.
      `/debug/snapshot` правки не потребовал (кладёт файл как есть).
      Секреты не маскируются — by design.
- [x] `go build ./...` + `go vet ./...` + `go test -count=1 ./...` —
      зелёные (весь модуль); греп go1.20 по диффу — чисто; `ETALON_V6MIG=1`
      — по-прежнему РОВНО одно задекларированное расхождение Р2.
- [x] Живая фича `SPECS/features/state.md` поправлена той же правкой:
      абзац о теге замены в разделе бэкапа; в «Внешних поверхностях» —
      единая сериализация Debug API и устройство гейта (почему у файла,
      направление расхождения, copy-from по исходнику, GET не гейтуется).

## W8 — Голдены и приёмка — СДЕЛАНО (O3 закрыт вердиктом капитана 2026-08-29)

Отчёт: `SPECS/118-F-N-STATE_V7/IMPLEMENTATION_REPORT.md` (файлы по волнам,
судьба тестов, сигнатуры, grep-инварианты, §7 — вердикт байт-эквивалентности
с поимённым списком расхождений).

- [x] Golden `real-v088`: фикстура мигрирована в v7; сравнение с эталоном
      W2 — **эмиссия (outbounds/endpoints/route/inbounds/log/experimental)
      байт-в-байт**; секция `dns` — ДВА расхождения, поимённо в
      IMPLEMENTATION_REPORT §7.3; вердикт O3 по ним — §7.6 (Р-DNS-1
      принят, Р-DNS-2 починен). Оба — в коде, которого
      кампания не касалась (весь DNS-конвейер: `git diff 39ab397` пуст);
      проявились от починки самого раннера, читавшего DNS через легаси-
      зеркало v5 мимо прод-пути. Ни код под эталон, ни эталон под код не
      подгонялись.
- [x] То же на эталоне класса `v6_roundtrip` (`etalon/v6mig`, `ETALON_V6MIG=1`)
      — РОВНО одно задекларированное расхождение Р2 (`[P]auto` →
      `[P]select-auto`), других нет.
- [x] Golden-снимок перезафиксирован: `state.json` сценария — v7-форма
      (входной v4 сохранён как `core/state/testdata/real_v088_v4.json`).
      Раннер `golden_test.go` приведён к прод-пути (`ctx.Preset`,
      канонический `state.DNS`, `TargetSpecFromState`, presets/DNS-библиотека
      шаблона); `actual.config.json` переведён в `.gitignore` как артефакт
      отладки.
- [x] СНЯТИЕ SKIP у `real-*` — **сделано по вердикту капитана O3
      (2026-08-29)**. Порядок был такой: сперва починен Р-DNS-2 —
      предсуществующий прод-баг неполного множества валидных
      `rule_set`-тегов (`CollectEmittedRouteRuleSetTags` в
      `core/build/preset_merge.go`, точка вызова в `core/build/build.go`,
      тест `core/build/dns_ruleset_dangling_test.go`; чистка висячих
      ссылок сохранена); затем `expected.config.json` сценария
      перезафиксирован честным выхлопом починенного раннера; затем снят
      SKIP вместе с переменной `GOLDEN_RUN_REAL` — real-сценарии идут в
      обычном `go test ./core/build`, сравнение осталось строгим
      байт-в-байт. Против эталона W2 остался один принятый класс Р-DNS-1
      (порядок `dns.servers`, множества идентичны). Разбор —
      IMPLEMENTATION_REPORT §7.6, декларация — `etalon/README.md`.
- [x] Сценарий §4.B.10 на мигрированном real-v088:
      `TestMigrationScenario10RealV088RuleTargetsNotReset`
      (`core/state/migration_scenarios_test.go`) — ни одна цель правила,
      тег замены или Направление не считаются осиротевшими; проверено на
      «кусачесть» временным сужением множества системных тегов.
- [x] Полный прогон: `go build ./...`, `go test -count=1 ./...`,
      `go vet ./...` — зелёные; GUI — `bash build/build_darwin.sh` зелёный.
- [x] Греп go1.20 по диффу кампании → 0 (три совпадения — комментарии,
      объясняющие сам гард); греп-инварианты §4.A прогнаны повторно —
      таблица в IMPLEMENTATION_REPORT §5, кода по всем инвариантам ноль
      (оговорка про имя `SourceNodeCounts` — там же).
- [x] `docs/release_notes/upcoming.md`: миграция состояния (+`.v6.bak`),
      отчёт потерь, defaults → настройки приложения, реальный кап
      max_nodes (потолок 3000) — EN + RU.
- [x] IMPLEMENTATION_REPORT.md: файлы по волнам, судьба тестов категории
      (б) с причинами, изменённые сигнатуры, таблица grep-инвариантов,
      результат байт-эквивалентности, статус открытых вопросов O1–O3.

## Хвосты ревью W1 (обязательны в последующих волнах)

- [x] W2: cloneCanonicalNode / clone Replace.Strategy / clone Fold.Auto —
      deep-copy через новые `configtypes.(*TemplateInt).Clone` и
      `(*DirectionAuto).Clone` (разделяемых указателей больше нет).
- [x] W5: backup Export switch по Kind получил кейсы folder/auto —
      предупреждение `backup_source_kind_unsupported` (контракт 0.11 их
      не знает, секция `folders[]` — отдельный трек SPEC 118 §2).
      Подпись стала `Export(...) (*Backup, []Warning, error)`;
      предупреждения экспорта показываются пользователю тем же списком,
      что у импорта (`settings_backup.go`).
- [x] Semantic-фиксация ревью: Load-проекция теперь несёт DisabledNodes
      (устранено расхождение projection/wizard в сторону намерения
      пользователя); folder/auto/unknown в проекции — выключенный
      плейсхолдер (индексный инвариант жив).

## Хвосты ревью W2

- [x] applyRenames: перепись fold-тегов в PresetBody.Vars (пропуск —
      LOW-находка ревью; закрыто оркестратором после W2).
- [x] reportLocalDirections: «источник», не «подписка» (COSMETIC).
- [x] etalon/README: честная формулировка про dns.independent_cache
      (раннер его НЕ нормализует; предсуществующее падение
      GOLDEN_RUN_REAL — не предмет W8).
- [x] W6: MigrationReport персистить в файл (bin/migration_report.txt)
      и показывать при первом открытии конфигуратора — headless-первый
      Load сейчас оставляет отчёт только в логе (LOW/UX-находка).

## Хвосты ревью W3 (фикс-раунд принят)

- [x] W6: write-back one-shot fetch в окне источника
      (source_edit_window.go, m.Sources[i] = snapshot) — сверять ревизию
      модели перед записью: Save во время полёта fetch'а сейчас может
      быть откачен снимком (pre-existing гонка, не W3).
- [x] W5: мостовая Meta умерла — история fetch живёт ТОЛЬКО в
      `UpdateStatus`, а `SubMeta` несёт лишь заголовки провайдера.
      Противоречивого успеха больше нет по построению.
- [x] W5: импорт бэкапа переведён с DisabledNodes-карты на
      `PendingDisabled` (вердикт O2); туда же миграция кладёт
      несматченные ключи. Комментарий в sources_v7.go актуализирован.
