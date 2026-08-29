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

## W5 — Смерть легаси

- [ ] Включить снос миграции (шаг 8): `migrationPurgesLegacy = true`.
- [ ] Удалить: `core/state/raw_cache.go`, `core/rebuild_raw_cache.go`,
      `LookupCachedBody`, писатель raw-кэша в fetch-сервисе.
- [ ] Удалить disabled-машинерию: карта в типах, `filterDisabledNodes`,
      `GCDisabledNodes`, TTL, debugapi disabled-nodes API (заменить на
      enabled-мутацию узла), UI-кэши отметок.
- [ ] Удалить fold: `SourceFold`, `PrepareSourceFolds`,
      `EffectiveTagPrefix`, WIZARD-маркеры, Fold-вкладка окна источника →
      вкладка Replace (минимальная правка формы: те же контролы
      mode/strategy + явное поле tag; новых глифов не вводить).
- [ ] Удалить `ExcludeFromGlobal`/`ExposeGroupTagsToGlobal` и ветки
      `outbound_filter`.
- [ ] Удалить локальные Направления: поле, scope конфигуратора, эмиссию
      local selectors, `localSubscriptionGroupTags`-цели detour.
- [ ] Удалить `TagSpec.Mask` (тип TagSpec → используется только
      бэкап-границей/миграцией), `Meta.PreviewNodes`,
      превью-кэши (`RebuildPreviewCache`, `PreviewNodesBySource`,
      `SourceNodeCounts`, `PreviewCacheGeneration`), `Defaults` из
      state и `WizardModel.Defaults`.
- [ ] Удалить detour-тройню из прод-типов (читает только миграция) и
      `Chain.Hops []string`-форму.
- [ ] Удалить мост §6 целиком (TEMPORARY BRIDGE не существует).
- [ ] Тесты категории (б) recon/tests.md: смерть предмета — удалить с
      причиной в отчёте (в т.ч. `e2e_disabled_flow_test.go` — контракт
      отменён осознанно; `expose_exclude_test.go`; `raw_cache_test.go`;
      `disabled_nodes_*`; `preview_nodes_test.go`;
      `rebuild_raw_cache_test.go`); переработка — по своим волнам уже
      сделана, добить остатки.
- [ ] Прогнать grep-инварианты SPEC §4.A — все по нулям.
- [ ] `go build ./...` + `go test ./...` + `go vet ./...`.

## W6 — UI-компенсация

- [ ] Счётчики и превью источников из nodes[] (`source_tab.go`);
      состояние «не фетчилось» — из updateStatus.
- [ ] Окно источника: JSON-вкладка server = редактор body; «Regen from
      raw» (ошибка разбора → откат, узел не портится); JSON подписки —
      синхронный body-рендер, read-only.
- [ ] Overview: raw-body-блок → счёт узлов + per-node origin.raw;
      Storage record = v7-запись.
- [ ] Пул хопов цепочек из nodes[] (+ Направления, replace-теги, верхние);
      выбор хранится NodeLink.
- [ ] Settings приложения: Default update interval + Default max nodes →
      `bin/settings.json`; двухступенчатый резолв виден в подсказках
      существующими средствами (без новых виджетов-вопросов).
- [ ] Предупреждение о протухании выбора cache.db при операциях смены
      финального тега (правка TagPolicy, replace.tag) — существующим
      механизмом предупреждений.
- [ ] Поведенческие тесты Т8 (Regen-откат, счётчики, кандидаты хопов);
      без ассертов на форматирование строк.
- [ ] `go build ./...` + GUI-скрипты `build/`.

## W7 — Бэкап-конвертеры и remote-гейт

- [ ] `core/backup/convert_v7.go`: экспорт v7 → 0.11 (enabled →
      disabled-карта по сырым тегам; replace → fold; NodeLink → тройня;
      hops → строковые финальные теги; TagPolicy → tag{prefix,postfix};
      NodeTag = Node.tag; nodes[] не экспортируются).
- [ ] Импорт 0.11 → v7: обратные конвертации; mask server/chain →
      Node.tag, подписки — warning; локальные Направления:
      fold-производные → replace, прочие — warning; хопы: резолв по
      живому индексу, нерезолвнутые → `NodeLink{"", тег}` + warning;
      disabled-карта — по вердикту O2 (до вердикта: warning-потеря,
      место применения выделено функцией).
- [ ] Corpus-тесты `contract/corpus/backup/` зелёные без правки фикстур;
      roundtrip-тест экспорт→импорт (SPEC §4.F.2); импорт фикстуры
      v1.5.x (§4.F.3).
- [ ] Remote-гейт: `/profile/copy-from`, `PATCH /state/rules`,
      `PATCH /state/dns` (state_endpoints.go + remote_endpoints.go) —
      сверка meta.version, отказ с версиями в сообщении; тест §4.G.
- [ ] Debug API: snapshot/state-эндпоинты отдают v7-форму (актуализация
      сериализации, без обязательств совместимости).
- [ ] `go build ./...` + `go test ./core/backup/... ./core/debugapi/...`.

## W8 — Голдены и приёмка

- [ ] Golden `real-v088`: миграция фикстуры; сравнение config.json нового
      движка с эталоном W2 — байт-в-байт; расхождения (если есть) —
      поимённый список на вердикт O3, НЕ подгонять молча.
- [ ] То же на `v6_roundtrip.json`-эталоне.
- [ ] Перезафиксировать golden-снимок (state в v7-форме), снять SKIP по
      умолчанию в `core/build/golden_test.go`.
- [ ] Сценарий §4.B.10 на мигрированном real-v088 (правила не сброшены).
- [ ] Полный прогон: `go build ./...`, `go test -count=1 ./...`,
      `go vet ./...`; GUI-пакеты — `build/`-скрипты.
- [ ] Греп go1.20 по диффу → 0; греп-инварианты §4.A повторно.
- [ ] `docs/release_notes/upcoming.md`: миграция состояния, отчёт потерь,
      смена умолчаний (defaults → настройки приложения), реальный кап
      max_nodes.
- [ ] IMPLEMENTATION_REPORT.md: файлы по волнам, судьба тестов категории
      (б) с причинами, изменённые сигнатуры, таблица grep-инвариантов,
      результат байт-эквивалентности, статус открытых вопросов O1–O3.

## Хвосты ревью W1 (обязательны в последующих волнах)

- [x] W2: cloneCanonicalNode / clone Replace.Strategy / clone Fold.Auto —
      deep-copy через новые `configtypes.(*TemplateInt).Clone` и
      `(*DirectionAuto).Clone` (разделяемых указателей больше нет).
- [ ] Волна, включающая создание folder/auto (W5/этап 3): backup Export
      switch по Kind обязан получить кейсы folder/auto (или явный
      warning) — иначе молчаливое выпадение из экспорта (ловушка
      этапа 0 SPEC 116).
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
- [ ] W6: MigrationReport персистить в файл (bin/migration_report.txt)
      и показывать при первом открытии конфигуратора — headless-первый
      Load сейчас оставляет отчёт только в логе (LOW/UX-находка).

## Хвосты ревью W3 (фикс-раунд принят)

- [ ] W6: write-back one-shot fetch в окне источника
      (source_edit_window.go, m.Sources[i] = snapshot) — сверять ревизию
      модели перед записью: Save во время полёта fetch'а сейчас может
      быть откачен снимком (pre-existing гонка, не W3).
- [ ] W6: мостовая Meta при недостоверном теле штампует LastStatus="ok"
      с PreviewNodes из мусора, расходясь с UpdateStatus="err" — UI
      мостовой эпохи показывает противоречивый успех; чинится смертью
      мостовой Meta в W6 (проверить при сносе).
- [ ] W7: импорт бэкапа переводится с DisabledNodes-карты на
      PendingDisabled (комментарий в sources_v7.go актуализирован).
