# SPEC 118 · Этап 2 — план реализации

Архитектура и порядок волн. Все file:line — из recon-документов SPEC 117
(ветка develop, 2026-08-29); перед правкой сверяться с актуальным кодом.

## 1. Go-типы: дискриминация полиморфного sources[]

### 1.1 Выбор: плоский юнион с дискриминатором `kind`

Три рассматривавшихся варианта:

- **(а) Go-интерфейс `Source` + custom UnmarshalJSON контейнера**
  (буквальный перенос Dart-дерева). Отвергнут: репо-политика «интерфейс —
  только при ≥2 реализациях или моках» тут не выполняется по существу
  (потребителям всё равно нужен switch по kind); custom-анмаршал на каждый
  уровень усложняет byte-stable roundtrip; deep-copy/diff/патчи получают
  type-assertion в каждом callsite.
- **(б) Конверт с вложенными типами** `{kind, server:{...}} | {kind,
  folder:{...}}`. Отвергнут: лишний уровень вложенности против всей
  существующей схемы state (rules[] и dns_options уже используют ПЛОСКИЙ
  kind-дискриминатор — SPEC 053/056), шумные диффы, двойное имя у каждого
  поля.
- **(в) ПРИНЯТО: плоский юнион** — один Go-struct на уровень, дискриминатор
  `kind`, поля чужих kind'ов пустые/omitempty. Ровно паттерн сегодняшних
  `Source.Type` (connections.go:56) и `Rule`/`DNSOptions`. Плюсы:
  стандартный encoding/json (стабильный порядок полей по struct → roundtrip
  байт-в-байт бесплатно), механическая валидация формы, привычные
  deep-copy и диффы. Минус — нелегальные комбинации полей возможны
  синтаксически — закрывается нормализатором на Load (лишние поля
  отбрасываются с warning) и конструкторами.

### 1.2 Типы (core/state, новый файл `sources_v7.go`)

```go
type SourceKind string // "server" | "chain" | "auto" | "folder" | "subscription"

// NodeLink — единая ссылка «через кого» (features/directions.md §6).
type NodeLink struct {
    FolderID string `json:"folder_id,omitempty"` // "" → корневое пространство финальных тегов
    Tag      string `json:"tag"`                 // сырой тег узла папки | финальный тег корня
}

// Origin — происхождение узла. nil = создан руками с нуля.
type Origin struct {
    Kind   string `json:"kind"`              // "uri" | "wg_ini" | "json"; kind=warp НЕ существует (warp-К1)
    Raw    string `json:"raw"`               // байт в байт; правится только Regen и merge-освежением
    SubURL string `json:"sub_url,omitempty"` // URL родной подписки; ВНУТРИ своей подписки пуст
}

// TagPolicy — косметика эмиссии (prefix/postfix, переменные {$tag}… живут).
type TagPolicy struct {
    Prefix  string `json:"prefix,omitempty"`
    Postfix string `json:"postfix,omitempty"`
}

// AutoStrategy = configtypes.DirectionAuto (перенос, не изобретение;
// 9 полей: mode/url/interval/idle_timeout/tolerance-TemplateInt/
// interrupt-трёхзначный/pool/pool_tolerance/sticky_hash).
type AutoStrategy = configtypes.DirectionAuto

// AutoGroup — провайдерская группа (kind=auto): groups-К2/К3 закрыты полями.
type AutoGroup struct {
    GroupType string       `json:"group_type"`        // "selector" | "urltest"; selector остаётся selector'ом
    Default   string       `json:"default,omitempty"` // selector only; сырой тег члена, обязан входить в состав
    Members   []NodeLink   `json:"members"`
    Strategy  AutoStrategy `json:"strategy,omitempty"`
}

// Node — узел: kind ∈ {server, chain, auto}. Живёт в корне sources[]
// (через Source) и в Folder.Nodes.
type Node struct {
    Kind    SourceKind `json:"kind"`
    Tag     string     `json:"tag"`     // СЫРОЙ тег — идентичность в контейнере, до тег-политики
    Enabled bool       `json:"enabled"`
    Origin  *Origin    `json:"origin,omitempty"`

    Body   json.RawMessage `json:"body,omitempty"`   // server: готовый sing-box outbound, чист от detour
    Detour *NodeLink       `json:"detour,omitempty"` // server only; у folder — общий detour (см. Source)
    Hops   []NodeLink      `json:"hops,omitempty"`   // chain: ближний первым
    Group  *AutoGroup      `json:"group,omitempty"`  // auto
}

// Source — элемент sources[]: узел ИЛИ папка/подписка.
// Node встроен — его поля инлайнятся в JSON (у folder заняты kind/enabled/
// detour: общий detour папки едет тем же ключом "detour").
type Source struct {
    Node

    // folder | subscription
    ID        string         `json:"id,omitempty"`   // ULID; ЕДИНСТВЕННАЯ идентификация папки
    Name      string         `json:"name,omitempty"`
    TagPolicy *TagPolicy     `json:"tag_policy,omitempty"`
    Nodes     []Node         `json:"nodes,omitempty"`
    Replace   *FolderReplace `json:"replace,omitempty"`

    // subscription only
    URL          string              `json:"url,omitempty"`
    Skip         []map[string]string `json:"skip,omitempty"`
    MaxNodes     int                 `json:"max_nodes,omitempty"`
    Update       *UpdateSpec         `json:"update,omitempty"`
    Meta         *SubMeta            `json:"meta,omitempty"`
    UpdateStatus *SubUpdateStatus    `json:"update_status,omitempty"`
}

// FolderReplace — свёртка папки: объект, не узел.
type FolderReplace struct {
    Mode     string        `json:"mode"`          // "manual" | "auto" | "both"
    Tag      string        `json:"tag"`           // явный тег замены; both → двойник "<tag>-auto"
    Strategy *AutoStrategy `json:"strategy,omitempty"` // nil при manual
}
```

`SubMeta` — сегодняшняя `SubscriptionMeta` МИНУС fetch-история и
PreviewNodes/NodesCountFetched/Truncated: остаются заголовки провайдера
(profile_title, interval, support_url, web_page, content_disposition),
`UserInfo`, `ProviderAnnounce`. `SubUpdateStatus` — новая структура:
`url_at_fetch, last_attempt_at, last_success_at, last_status, error_count,
last_error_msg, last_error_url, http_status_code, raw_body_bytes,
nodes_count_fetched, truncated, warnings []FetchWarning` (per-record
деградации: skip-счётчики, битые записи, потерянные группы-члены —
источник данных отчёта сборки вместо parse-стадии, jsontab-К4).

Замечания:

- **id только у папки/подписки** — узлы идентифицируются тегом (SPEC 112,
  node-identity-is-tag). ULID сохраняется у Folder/Subscription для
  NodeLink.folderId, имён профильных директорий и адресации отчётов.
- **default в AutoGroup, не в AutoStrategy** — default member-зависим
  (strategy-К2), а AutoStrategy переиспользуется твинами/replace, где
  default'а нет.
- **Embedded Node** даёт плоский JSON без ручного маршала; у kind=folder
  Node.Tag/Origin/Body/Hops/Group пусты (нормализатор следит), Detour =
  общий detour папки — одна json-точка "detour" на оба смысла, семантика
  различается по kind (как и всё остальное в юнионе).

### 1.3 Валидация формы и совместимость вперёд/назад

- `normalizeSourceShape(s *Source) []Warning` на Load: по kind обнуляет
  нелегальные поля (с warning), проверяет обязательные (server без body →
  деградация узла с warning, не отказ файла; правило битого фрагмента).
- Неизвестный `kind` → отказ загрузки с версиями в сообщении (файл от
  более нового мажора не должен сюда попасть — но защита обязана быть).
- **Вперёд (минорные добавки в v7)**: новые опциональные поля с omitempty;
  encoding/json игнорирует незнакомые ключи — v7-читатель старой сборки
  переживает файл новой, ПОКА мажор 7. Ломающее изменение формы = version 8.
- **Назад**: v6 и старше читаются только миграцией (§5); писать легаси
  некуда — Save всегда v7.
- `meta.version = 7` — он же «мажор схемы» для remote-гейта (Т10);
  отдельного minor в файле не заводим (незнакомые поля и так
  игнорируются).

### 1.4 Корень State и WizardModel

```go
type State struct {
    Meta         MetaSection
    Sources      []Source                // было Connections.Sources
    Directions   []configtypes.Direction // было Connections.Outbounds
    Rules        []Rule
    Vars         []SettingVar
    DNSOptions   DNSOptions
    WarpAccounts *WarpAccountsSection
    // + переходно: LegacyProjection для build-моста (§6), умирает в W6
}
```

`ConnectionsSection` умирает (остаётся только как приватный тип парсера
v6 в миграции). `WizardModel`: `Sources []corestate.Source` меняет тип
наполнения, `GlobalOutbounds` остаётся, `Defaults` удаляется (резолв — из
настроек приложения через существующий канал `locale.LoadSettings` /
`bin/settings.json`). Ревизия/BumpRevision — как в этапе 1, точки бампа
дополняются новыми мутациями (fetch-merge, Regen, toggle узла).

## 2. Схема диска v7 и Load/Save

- `core/state/disk_v7.go`: `diskStateV7{Meta, Sources, Directions, Rules,
  Vars, DNSOptions, WarpAccounts}` — ключи
  `meta/sources/directions/rules/vars/dns_options/warp_accounts`.
  `SchemaVersion = 7`, `SchemaName = "sources_v7"`.
- `load_router.go`: version 7 → `parseV7`; version ≤6 → соответствующий
  легаси-парс + миграция §5 (v2–v5 сначала доезжают существующими
  миграциями до v6-формы in-memory, потом v6→v7 — одна точка входа).
- `save.go`: пишет только v7. Атомарная запись — как сегодня (SPEC 041).
- Порядок полей struct = порядок ключей в файле → roundtrip байт-в-байт
  закрывается тестом уровня `canonical_roundtrip_test.go` (перерабатывается
  на v7-фикстуру).

## 3. Fetch/merge: архитектура

### 3.1 Разделение труда

```
config_service_subscriptions (fetch-сервис, сеть, владелец state)
    │ скачал тело, классифицировал заголовки → SubMeta
    ▼
subscription.ParseSubscriptionBody(body, skip, capN)   ← чистый парсер, без сети
    │ → ParseResult{nodes []ParsedMaterial, groups []ParsedGroupMaterial,
    │               truncated bool, warnings []FetchWarning}
    ▼
state.MergeSubscriptionNodes(sub *Source, res, trusted)  ← merge-правила
    │ nodes[] обновлены, updateStatus записан
    ▼
Save + пометка «конфиг устарел» (dirty), БЕЗ автозапуска сборки
```

- **Парсер** — перекомпоновка сегодняшнего `LoadNodesFromSourceEx`
  (source_loader.go:494-943): остаются классификация тела, ветки
  vpn://wg-conf/singbox/xray/uri, skip внутри парсеров, `dedup.accept`
  (подпись без tag/detour) + `collapsedInto`, пер-сорсная уникализация
  сырых тегов (`StampNodeIdentity`/`idCounts`). ВЫРЕЗАЮТСЯ: тег-политика
  (`applyTagPrefixPostfix` — переезжает в эмиссию), глобальный
  `MakeTagUnique` (эмиссия), `ApplySourceDetour` (умирает),
  `filterDisabledNodes` (умирает), `LookupCachedBody` (умирает).
  Кап: параметр capN (резолв: sub.MaxNodes → настройка приложения;
  клэмп константой 3000) проверяется в тех же точках цикла, где сегодня
  константа (source_loader.go:614 и её сёстры).
- **ParsedMaterial**: `{rawTag (уникализированный), body json.RawMessage,
  originKind, originRaw}`. Группы: `{rawTag, groupType, default, memberRawTags,
  options-allowlist}` — резолв members → `[]NodeLink{folderId=sub.ID, tag}`
  делает merge-слой (после collapsedInto-перепривязки).
- **Merge** (`state.MergeSubscriptionNodes`): по сырому тегу;
  совпавший — body/origin.raw освежаются, enabled/detour живут; новый —
  `enabled=true`; исчезнувший — удаляется, НО при `truncated` удаление
  запрещено (113-A); порядок nodes[] = порядок свежего тела (выжившие
  занимают позицию тела, а не старую). `trusted=false` (ошибка/пусто/
  обрыв) → nodes[] не тронуты, пишется только updateStatus.
- Превью «что придёт» перед добавлением источника (Add-диалог) использует
  тот же чистый парсер — это не сборка.

### 3.2 Кто зовёт fetch

- Расписание авто-обновления (существующий планировщик
  config_service_subscriptions.go) — интервал: sub.Update →
  `Meta.ProfileUpdateIntervalHours` → дефолт настроек приложения.
- Ручной Update в UI; добавление подписки (авто-fetch сразу).
- Правка модели fetch НЕ запускает (Т4); fetch по завершении поднимает
  «конфиг устарел» и BumpRevision.
- Фан-аут по многим подпискам — последовательный/ограниченный, как
  сегодня (memory: subscription_scale_fanout).

## 4. Эмиссия, резолв, гард, пул

- **Проекция сборки**: `AsParserConfig`/`ToProxySourceV4` перестраиваются:
  для подписок и папок проекция отдаёт УЖЕ ГОТОВЫЕ узлы (body +
  метаданные), парсер тел в конвейере сборки не вызывается. Практический
  шов: `SourceLoadResult` наполняется из nodes[] (body → ParsedNode.Outbound
  через json.Unmarshal в map — существующий контракт эмиттера
  `generateGroupNodeJSON`/`GenerateNodeJSONBare` сохраняется), теги
  проставляются эмиссионной тег-машиной: `NormalizeProxyDisplay` →
  policy(prefix/tag/postfix + переменные) → глобальный `MakeTagUnique`.
- **Единый резолв NodeLink** (`core/config/nodelink_resolve.go`):
  один вход для detour узлов, hops цепочек, members Auto, replace-целей;
  словарь целей = финальные теги корня + (folderId, сырой тег) узлов
  папок + replace-теги + теги Направлений + системные. Топологический
  порядок и кольца — расширение существующего `detour_topo.go`; политика
  по виду ребра: detour/hops fail-closed, members prune (передаётся
  флагом ребра). Wireguard-исключение — в точке применения detour.
- **Auto-эмиссия** фильтрует `enabled=false` членов сама (groups-К7);
  пустая группа не эмитится + warning; default вне состава — снимается с
  warning (существующее правило санитайзера остаётся последним рубежом).
- **FolderReplace**: `PrepareFolderReplaces` на месте `PrepareSourceFolds`
  — переиспользует `buildTwin` (source_folds.go:95 показывает паттерн);
  тег явный из replace.tag; both → `<tag>-auto` (формула твинов,
  direction_twins.go:31); авто-состав исключает Auto-узлы папки
  (dropGroupNodes); селекторная половина — опции
  `group_templates.channel` + default=двойник. Экспозиция маркерами
  в comment УМИРАЕТ — пул решает правило (ниже).
- **Пул кандидатов Направлений** (`outbound_filter.go` переписывается):
  верхние узлы + узлы папок без replace (финальные теги) + replace-теги;
  `FilterNodesExcludeFromGlobal`/expose-ветки сносятся.
- **Единый гард тегов** (`core/config/tag_guard.go`): множество занятых =
  Направления + твины + replace-теги + `-auto`-двойники + верхние узлы +
  системные теги шаблона; зовётся на сборке (вместо локального гарда
  ExpandDirectionTwins, direction_twins.go:87-103) и из операций
  именования (`DirectionTagTaken` расширяется этим множеством).
- **Реестр ссылок / rule_target_reset**: known-множество = гард +
  addOutbounds; `renameDNSDetour` и санитайзер получают dns.detour-ребро
  (deps-К11). Полные UI-rename-операции replace-тегов — этап 3.
- **Отчёт сборки**: parse-стадии причин больше нет — отчёт «Итога» берёт
  fetch-警-строки из `updateStatus.warnings` (по подпискам) + свои
  эмиссионные причины (chain_failed, naive_degraded, detour fail-closed);
  адресация — folderId/(folderId, tag) вместо SourceID/индексов.

## 5. Миграция v6 → v7

`core/state/migration_v6_to_v7.go` + `migration_report.go`. Вход —
in-memory v6-форма (ConnectionsSection и её легаси-поля становятся
приватными типами этого файла). Порядок — 8 шагов features/state.md
(SPEC Т7). Инженерные решения:

- **Шаг 1 (материализация)**: единственный легальный вызов старого
  raw-конвейера — миграция читает `bin/subscriptions/<id>.raw`
  (`state.ReadRawBody`) и гонит НОВЫЙ чистый парсер §3.1 (skip/дедуп/
  уникализация/кап) → nodes[] + Auto-узлы. Так body мигрированных узлов
  и body свежего fetch — один код (иначе первый fetch после апгрейда дал
  бы массовый «body изменился»).
- **Шаг 2**: карта DisabledNodes применяется по identity-ключам =
  сырым тегам (совместимость подтверждена deps §3); legacy-64hex —
  портированный `migrateLegacyDisabledKeys`-матчинг (единственное второе
  место, где он живёт; из прод-парсера удаляется).
- **Шаг 4 (хопы)**: индекс «финальный тег → (folderId, сырой тег)»
  строится по СТАРОЙ тег-машине (policy+mask+uniquify в порядке
  источников v6) — только так строковые хопы резолвятся честно;
  нерезолвнутое → `NodeLink{"", тег}` + warning.
- **Шаг 5**: та же таблица для тройни; переходная форма «тег без
  source_id» → `NodeLink{"", тег}` напрямую (deps-К6 — формы совпадают).
- **Шаг 6 (fold)**: `replace.tag` = материализованный
  `FoldSelectTag`/`FoldAutoTag` с индексом источника НА МОМЕНТ миграции
  (source_fold.go:86-100); для mode both tag = селекторный дериватив,
  двойник получит `<tag>-auto` — здесь дериватив меняется
  (`N:select`+`N:auto` → `N:select`+`N:select-auto`): выбор в cache.db у
  авто-двойника протухнет — фиксируется warning'ом миграции (кандидат в
  список O3, если решим держать старые теги — но старая пара тегов не
  выражается моделью `<tag>-auto`).
- **Шаг 8**: перед миграцией — бэкап-копия `state.json.v6.bak` рядом;
  снос raw-файлов и легаси-ключей только после успешной записи v7.
  `defaults.reload/max_nodes` при первом переносе пишутся в настройки
  приложения, если там ещё пусто (не перетирают явно выставленные).
- **Отчёт**: `MigrationReport{Warnings []string}` возвращается из Load;
  презентер показывает один диалог со списком; отчёт также пишется в лог.
- **Сид**: `configurator.go` сид легаси-формы проходит через тот же
  `migrateV6ToV7` (features/state.md: отдельного нового сида не требуется).
- **Remote-профили**: миграция срабатывает в общем Load — каждый профиль
  (`bin/wizard_states/remote/<id>/`) мигрирует при первом открытии своей
  машины.
- **Идемпотентность**: v7-файл роутится мимо миграции; повторного прохода
  не существует по построению.

## 6. Мост совместимости между волнами

Смена типа `state.Source` задевает почти все пакеты. Чтобы каждая волна
была собираемой и dev-сборки оставались рабочими (юзер ставит их руками —
memory: core-binary-hands-off), вводится ВРЕМЕННЫЙ мост, умирающий в W6:

- W1–W3: build-пути core (`config_service.go`, `rebuild.go`) продолжают
  ходить через legacy-проекцию `ProxySource`; `adapter_source.go`
  переписывается «v7 → ProxySource» и ВРЕМЕННО ДЕРИВИРУЕТ легаси-поля:
  `DisabledNodes` из `enabled=false` узлов, `Fold` из `replace`,
  `TagSpec{Prefix,Postfix}` из TagPolicy, тройню из NodeLink. Raw-кэш и
  старый парс при этом работают как раньше — снос raw-кэша миграцией
  (шаг 8) ГЕЙТИТСЯ до W5 (константа `migrationPurgesLegacy=false`),
  чтобы мигрированное состояние сосуществовало со старым build-путём.
- W4 переключает сборку на эмиссию из nodes[]; W5 включает снос и
  удаляет мост целиком (grep-инварианты SPEC §4.A его не переживают).
- Мост — единственное место, где легаси-поля дериватся; помечен
  `// TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5`.

## 7. Бэкап-конвертеры (граница 0.11)

`core/backup/convert_v7.go`: экспорт `state.Source(v7)` → форма
`backup.Source(0.11)` и обратно. Таблица соответствий — SPEC Т9.
Существующие `export.go`/`import.go` перестраиваются на конвертеры;
`types.go` (форма 0.11) НЕ меняется; corpus-раннер зелёный на нетронутых
фикстурах. Импорт-ветки с потерями (mask подписки, произвольные локальные
Направления) используют существующий канал warning'ов импорта.
Disabled-карта импорта — по решению O2 (реализация ждёт вердикта; до
него — вариант Б с warning, чтобы волна не блокировалась).

## 8. UI-компенсация

- Превью/счётчики: `source_tab.go` счётчики = `len(sub.Nodes)` /
  подсчёт enabled; превью-строки — из nodes[] (формат существующих
  подписей, без новых глифов). `RebuildPreviewCache`/`SourceNodeCounts`
  машинерия удаляется вместе с W5.
- Окно источника (`source_edit_window.go`): вкладка JSON server-узла —
  редактор body напрямую; «Regen from raw» вместо «Reset to URI»
  (parse(origin.raw) → body, ошибка → откат + диалог); вкладка JSON
  подписки — синхронный рендер body из nodes[] (read-only); горутина
  ReadRawBody/parsePreviewNodesFromBody умирает.
- Overview (`source_edit_overview.go`): raw-body-блок → счёт узлов +
  per-node origin.raw; Storage record = v7-запись.
- Хопы цепочек (`source_chain_hops.go`): кандидаты из nodes[] подписок
  и папок + верхние узлы + Направления + replace-теги; выбранное
  хранится NodeLink'ом.
- Settings-вкладка приложения: поля Default update interval и Default
  max nodes (персист `bin/settings.json` через существующий
  locale-Settings канал).
- Никаких Test* на форматирование подписей (no-ui-format-tests).

## 9. Волны

Каждая волна: `go build ./...` зелёный, затронутые пакеты — `go test`;
финал волны — фиксация в TASKS. Крупные волны отдаются исполнителю
целиком (orchestrate-opus-subagents).

- **W1 — Типы + схема + Load/Save + мост.** `sources_v7.go`, `disk_v7.go`,
  parseV7/marshalDiskV7, SchemaVersion=7, load_router; State.Sources/
  Directions вместо Connections; WizardModel на новом типе; мост §6
  (адаптер v7→ProxySource с деривацией легаси-полей); механическая правка
  всех callsite'ов полей (компиляционно-управляемая). Load v6 на этой
  волне — структурный перенос без семантической миграции (гейт
  `migrationPurgesLegacy=false`, шаги 1–7 — заглушки, поля переносятся
  как есть в мостовые деривативы). Roundtrip-тест v7.
- **W2 — Миграция v6→v7.** Полные 8 шагов (§5), отчёт миграции + диалог,
  бэкап-копия, сид, фикстуры и сценарии SPEC §4.B (кроме п.10 —
  known-расширение приходит в W4); снос (шаг 8) под гейтом.
- **W3 — Fetch/merge.** Чистый парсер §3.1 (skip/дедуп/уникализация/
  реальный кап), MergeSubscriptionNodes, updateStatus, 113-A/truncated,
  Auto-материализация с NodeLink-членами, авто-fetch новой подписки,
  резолв умолчаний из настроек приложения; поведенческие тесты SPEC §4.D.
  Fetch уже пишет nodes[]; сборка пока ходит мостом.
- **W4 — Эмиссия + резолв + гард + пул.** Сборка из nodes[] (без парса
  подписок), тег-политика на эмиссии, единый NodeLink-резолв,
  PrepareFolderReplaces, пул Направлений, единый гард, known-расширение
  rule_target_reset, dns.detour в санитайзер, отчёт сборки из
  updateStatus; тесты SPEC §4.E. Мост сужается до Load-проекции
  build-путей (детур-деривации умирают).
- **W5 — Смерть легаси.** Включить снос миграции (шаг 8); удалить
  raw-кэш-код, disabled-карту+TTL+GC, fold-машинерию, exclude/expose,
  локальные Направления, mask, PreviewNodes/превью-кэши, Defaults,
  detour-тройню, мост §6 целиком; переработка/удаление тестов категории
  (б) по recon/tests.md; grep-инварианты SPEC §4.A.
- **W6 — UI-компенсация.** §8 целиком; поведенческие проверки Т8
  (Regen-откат, счётчики из nodes[], хопы из nodes[]).
- **W7 — Бэкап-конвертеры + remote-гейт.** §7, тесты SPEC §4.F;
  version-гейт `/profile/copy-from` и `PATCH /state/*` обеих сторон +
  тест §4.G; Debug API формы v7 (актуализация snapshot/state_endpoints).
- **W8 — Голдены + приёмка.** Байт-эквивалентность §4.C на real-v088 и
  v6_roundtrip (сравнение старого движка — зафиксированный эталонный
  config.json СНЯТЫЙ ДО W4 — с новым); перезафиксировать golden, снять
  SKIP; полный прогон build/test/vet + GUI-скрипты; греп go1.20;
  IMPLEMENTATION_REPORT (судьба тестов, расхождения golden — если есть,
  на вердикт O3).

Порядок жёсткий: W4 раньше W5 (сборка не может потерять raw-кэш до
эмиссии из nodes[]); эталон для W8 снимается ДО W4 (пока старый движок
жив) — задача W2 включает снятие эталонных config.json с фикстур.

## 10. Риски

- **Р1. Golden не сойдётся байт-в-байт.** Порядкозависимые механики
  (MakeTagUnique по порядку источников, сортировка ключей опций групп,
  materialized fold-теги, отсутствие ApplySourceDetour-штампа может
  менять порядок ключей в outbound). Смягчение: эмиссионная тег-машина
  воспроизводит старый порядок применения; эталоны сняты заранее (W2);
  расхождения — поимённо на вердикт (O3), не подгонять молча.
- **Р2. Смена пары fold-тегов при mode both** (`N:auto` →
  `<tag>-auto`-дериватив) протухает выбор cache.db и ссылки на `N:auto`.
  Смягчение: миграция переписывает ссылки на новый дериватив через
  реестр + явный warning; зафиксировано в отчёте миграции.
- **Р3. Объём компиляционной волны W1.** Смена типа Source задевает
  десятки файлов; риск смысловых правок под видом механических.
  Смягчение: W1 меняет только форму (мост держит семантику), все
  поведенческие изменения — в своих волнах; ревью диффа W1 отдельно.
- **Р4. Мост утекает в релиз.** Деривация DisabledNodes/Fold из нового
  канона — временная; если W5 не добьёт, легаси-поля оживут. Смягчение:
  grep-инварианты §4.A прямо запрещают мостовые символы; маркер-коммент
  TEMPORARY BRIDGE.
- **Р5. Миграция необратима после шага 8.** Ошибка после сноса raw-кэша
  теряет материал. Смягчение: снос строго после успешной записи v7;
  бэкап-копия state; гейт сноса до W5.
- **Р6. Регресс дедупа/уникализации при переносе парсера.** Порядок
  стадий (дедуп ДО тегов) обязателен (source_loader.go:513-517).
  Смягчение: контрактные body-фикстуры (категория (в)) гоняются через
  новый вход парсера; регресс-кейс v1.5.2 — обязательный тест.
- **Р7. go1.20.** Большой новый код (deep-copy, миграция) без
  slices/maps/min/max; греп в каждой волне.
- **Р8. Двухфазный отчёт «Итога»** (jsontab-К4): перенос parse-причин в
  updateStatus меняет тайминг появления строк отчёта. Смягчение:
  final_tab-конвейер читает state синхронно — поведенческий тест на
  «источник с битой записью виден в Итоге после fetch, без пересборки».
