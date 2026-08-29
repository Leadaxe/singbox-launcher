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

## W3 — Fetch/merge: материализация

- [ ] `subscription.ParseSubscriptionBody(body, skip, capN)` — чистый
      парсер (PLAN §3.1): классификация тел, skip внутри парсеров, дедуп
      по подписи + collapsedInto, уникализация сырых тегов, реальный кап
      capN в точках цикла; БЕЗ тег-политики, БЕЗ MakeTagUnique, БЕЗ
      ApplySourceDetour, БЕЗ filterDisabledNodes; выход — ParsedMaterial
      + группы + truncated + warnings.
- [ ] `state.MergeSubscriptionNodes`: merge по сырому тегу (освежить /
      добавить / удалить), порядок = порядок тела; truncated → удаление
      запрещено; trusted=false → nodes[] не тронуты; Auto-члены →
      `[]NodeLink{folderId: sub.ID}` c prune потерянных (warning);
      вложенная группа — warning.
- [ ] `config_service_subscriptions.go`: fetch-сервис → скачать,
      SubMeta-заголовки, ParseSubscriptionBody, Merge, updateStatus
      (last_attempt/success, ошибки, truncated, nodes_count, warnings),
      Save, BumpRevision/«конфиг устарел»; писатель raw-кэша остаётся до
      W5 (мост).
- [ ] Резолв капа: sub.MaxNodes → настройка приложения → клэмп 3000;
      резолв интервала: sub.Update → profile-update-interval → настройка
      приложения. Чтение дефолтов из `bin/settings.json` (канал
      locale-Settings); поля UI — в W6.
- [ ] Авто-fetch свежедобавленной подписки — на новый конвейер.
- [ ] Тесты SPEC §4.D (1–9), включая регресс v1.5.2 (32 ss:// → 1) и
      «X, X-2, X»; 113-A и truncated-семантика; body без detour-ключа.
- [ ] `go build ./...` + `go test ./core/... `.

## W4 — Эмиссия, резолв, гард, пул

- [ ] Сборка из nodes[]: проекция подписок/папок отдаёт готовые узлы
      (body → ParsedNode), конвейер сборки не вызывает парс тел подписок;
      raw-кэш из сборки не читается (файлы ещё живут для миграции).
- [ ] Эмиссионная тег-машина: NormalizeProxyDisplay → TagPolicy
      (prefix/tag/postfix + переменные) → MakeTagUnique (глобальный);
      порядок применения = старый (эталонная байт-эквивалентность).
- [ ] `core/config/nodelink_resolve.go`: единый резолв NodeLink
      (detour/hops fail-closed + кольца; members prune; словарь целей:
      корень + узлы папок + replace-теги + Направления + системные);
      топология — расширение detour_topo; WG-исключение detour.
- [ ] Папочный общий detour: Server-узлам без личного, пропуская
      Chain/Auto.
- [ ] Auto-эмиссия: фильтр enabled-членов, пустая группа не эмитится +
      warning, default вне состава — снимается с warning; selector
      сохраняет type/default.
- [ ] `PrepareFolderReplaces` вместо `PrepareSourceFolds`: явный tag,
      both → `<tag>-auto` (buildTwin), исключение Auto-узлов из
      авто-состава, селекторная половина из group_templates.channel;
      WIZARD-маркеры не пишутся.
- [ ] Пул кандидатов Направлений (outbound_filter): верхние + папки без
      replace (финальные теги) + replace-теги; твины исключают Auto-узлы
      и replace-теги.
- [ ] `core/config/tag_guard.go`: единое множество занятых (Направления +
      твины + replace + `-auto` + верхние + системные); подключить к
      сборке и `DirectionTagTaken`.
- [ ] `rule_target_reset`: known += replace-теги, `-auto`-двойники,
      верхние узлы (тест §4.B.10 — правила не сброшены на мигрированном
      состоянии).
- [ ] dns.detour — в граф-санитайзер (висячий ловится сборкой) и в
      known-реестр.
- [ ] Отчёт сборки: fetch-строки из updateStatus.warnings; адресация
      folderId/(folderId, tag); эмиссионные причины (chain_failed,
      naive_degraded, detour ⚠) — как раньше.
- [ ] Мост: детур/фолд-деривации из adapter_source сняты (остаётся только
      Load-проекция там, где build-пути ещё читают ProxySource-форму).
- [ ] Тесты SPEC §4.E (1–9); переработка chain/detour/group-семейств
      тестов на NodeLink (семантика резолва прежняя).
- [ ] `go build ./...` + `go test ./core/... ./ui/...`.

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
