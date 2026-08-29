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

- [ ] `core/state/sources_v7.go`: `SourceKind`, `NodeLink`, `Origin`,
      `TagPolicy`, `AutoStrategy` (= alias `configtypes.DirectionAuto`),
      `AutoGroup`, `Node`, `Source` (плоский юнион, embedded Node),
      `FolderReplace`, `SubMeta` (из SubscriptionMeta минус
      fetch-история/PreviewNodes), `SubUpdateStatus` (+`FetchWarning`) —
      формы по PLAN §1.2.
- [ ] `normalizeSourceShape`: по kind обнуляет нелегальные поля с warning;
      неизвестный kind — внятный отказ загрузки.
- [ ] `core/state/disk_v7.go`: `diskStateV7` (плоский корень
      `meta/sources/directions/rules/vars/dns_options/warp_accounts`),
      `SchemaVersion = 7`, `SchemaName = "sources_v7"`.
- [ ] `parseV7` / `marshalDiskV7`; `save.go` пишет только v7;
      `load_router.go`: 7 → parseV7, ≤6 → легаси-парс + миграция
      (в W1 — структурный перенос-заглушка, см. ниже).
- [ ] `State`: `Sources []Source` + `Directions []configtypes.Direction`
      вместо `Connections ConnectionsSection`; механическая правка
      callsite'ов `Connections.Sources`/`Connections.Outbounds`
      (компиляционно-управляемая, семантику не менять).
- [ ] Мост (PLAN §6): `adapter_source.go` — проекция v7 → ProxySource с
      ВРЕМЕННОЙ деривацией легаси-полей (DisabledNodes из enabled, Fold из
      replace, TagSpec из TagPolicy, тройня из NodeLink, Outbounds — пусто);
      маркер `// TEMPORARY BRIDGE (SPEC 118 W1-W4)`; build-пути core и
      raw-кэш работают как раньше.
- [ ] Load v6 → структурный перенос в v7-формы без семантики (nodes[]
      пустые, легаси-значения доезжают до моста); гейт
      `migrationPurgesLegacy = false`.
- [ ] `WizardModel`: `Sources` на новом типе; `Defaults` пока живёт
      (умирает в W5); точки BumpRevision не потеряны.
- [ ] `cloneSource`/deep-copy окна источника обновлены под новые поля
      (Nodes, Replace, TagPolicy, NodeLink) — без slices./maps.
- [ ] Тесты: `canonical_roundtrip_test.go` → v7-фикстура
      `testdata/v7_roundtrip.json` (папка с узлами, подписка с nodes[],
      chain c NodeLink-хопами, Auto, replace, directions, warp_accounts):
      Load→Save→Load→Save байт-в-байт; ID-стабильность папок.
- [ ] Компиляционная правка тестов затронутых пакетов (семантику не
      менять; падающие по существу — пометить и отложить до своих волн).
- [ ] `go build ./...` + `go test ./core/state/... ./ui/configurator/...`.

## W2 — Миграция v6 → v7

- [ ] `core/state/migration_v6_to_v7.go`: 8 шагов features/state.md
      (SPEC Т7, PLAN §5); `ConnectionsSection` и легаси-поля — приватные
      типы миграции.
- [ ] Шаг 1: материализация nodes[] из `bin/subscriptions/<id>.raw`
      НОВЫМ чистым парсером (зависимость: минимальный вход парсера W3 —
      допустимо реализовать `ParseSubscriptionBody` здесь и расширить в
      W3); кэша нет → nodes[] пуст + warning.
- [ ] Шаг 2: DisabledNodes → enabled=false; legacy-64hex-матчинг
      (перенос `migrateLegacyDisabledKeys` в миграцию); несматченные —
      warning.
- [ ] Шаг 3: NodeTagOrLabel/mask server/chain → Node.tag; mask-шаблон
      подписки — warning; prefix/postfix с переменными — в TagPolicy.
- [ ] Шаг 4: `Chain.Hops []string` → `[]NodeLink` через индекс
      «финальный тег (старая тег-машина) → (folderId, сырой тег)»;
      нерезолвнутый → `NodeLink{"", тег}` + warning.
- [ ] Шаг 5: detour-тройня → NodeLink (подписка → folderId; верхний →
      голый тег; переходная форма без source_id → `NodeLink{"", тег}`);
      коллизии верхних тегов: уникализация + перепись ссылок + warning.
- [ ] Шаг 6: fold → replace с материализацией сегодняшнего деривативного
      тега (включая `<index+1>:` при пустом префиксе); mode both:
      перепись ссылок с `N:auto` на `<tag>-auto` + warning о протухании
      выбора cache.db; локальные Направления: fold-производные → replace,
      произвольные — warning.
- [ ] Шаг 7: standalone exclude_from_global — warning + отчёт.
- [ ] Шаг 8 (под гейтом до W5): снос raw-файлов/карт/легаси-ключей после
      успешной записи v7; `defaults` → настройки приложения (не перетирая
      явно выставленные); бэкап-копия `state.json.v6.bak` ПЕРЕД миграцией.
- [ ] `MigrationReport` → единый диалог предупреждений (один раз) + лог.
- [ ] Сид шаблона легаси-формы — через тот же путь; remote-профили —
      мигрируют своим Load.
- [ ] Снять ЭТАЛОННЫЕ config.json со всех миграционных фикстур текущим
      (старым) движком → `SPECS/118-F-N-STATE_V7/etalon/` или
      `core/build/testdata/` (нужны W8; снять до W4!).
- [ ] Фикстуры и тесты SPEC §4.B п.1–9 (п.10 — в W4): fold всех режимов,
      локальные Направления обоих видов, legacy-hex disabled, mask,
      тройня, хопы, exclude, отсутствие кэша, идемпотентность,
      бэкап-копия, сид.
- [ ] `go build ./...` + `go test ./core/state/...`.

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
