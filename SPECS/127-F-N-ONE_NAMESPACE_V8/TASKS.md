# TASKS 127 · Волна 1 — state v8 (одно пространство имён)

Норма — `contract/docs/ONE_NAMESPACE.md` §1–§2, задача — `SPEC.md` §2–§3, адреса
и ловушки — `CODEMAP.md` (§1, §2, §3, §6, §7). Ловушки CODEMAP §7 — обязательное
чтение перед правкой: 1 (гейт версии), 2 (DNS Marshal), 3 (RawMessage), 4
(`StableRuleID`), 5 (`num` — три типа), 9 (порядок ключей), 14 (два инварианта),
17 (три синтетических State), 18 (srs «всё или ничего»), 20 (`EncodeBody`).

Правила исполнителю: работать ТОЛЬКО в каталоге worktree, который назван в
задании (абсолютные пути); ветки не переключать; коммитов не делать; `bin/`,
`bin/locale/ru.json`, `contract/VERSION`, `contract/schema/*`, `contract/docs/*`
не трогать (контракт — волна 3); тесты — в конце своего этапа, полный прогон
один раз в конце волны; россыпи юнитов не писать (память проекта: только
критичные интеграционные). Свои правки отражать в `CODEMAP.md` (адреса, новые
функции) и отмечать чекбоксы здесь. Тексты UI не меняются.

## 0. Целевая форма записей (state v8) — решения, не обсуждаются

Правило маршрута (`rules[]` и `sections.rules[]`):

```go
type Rule struct {
    Kind    RuleKind          `json:"kind"`
    ID      string            `json:"id,omitempty"`   // необязательные метаданные (ONE_NAMESPACE §1 стр. 67); лаунчер не генерирует, провозит
    Ref     string            `json:"ref,omitempty"`  // только preset
    Name    string            `json:"name,omitempty"` // inline|srs; источник StableRuleID — строка та же, что была в body.name
    Enabled bool              `json:"enabled"`
    Num     *int              `json:"num,omitempty"`  // бывший order_num; nil = не размечено
    Refs    []string          `json:"refs,omitempty"` // только srs: URL наборов, дедуп с сохранением порядка (бывшие srs_url+srs_urls)
    Vars    map[string]string `json:"vars,omitempty"` // только preset
    Body    json.RawMessage   `json:"body,omitempty"` // inline|srs: правило sing-box КАК ЕСТЬ; у preset нет
}
```

- `Body` остаётся `json.RawMessage` (ловушка 3). Внутри — матчеры и **цель в
  форме sing-box**: `"outbound": "<tag>"` | `"action": "reject"` |
  `"action": "reject", "method": "drop"`. Значения `reject`/`drop` из v7-поля
  `outbound` при миграции переводятся ровно как `outboundutil.ApplyOutboundToRule`
  (`internal/outboundutil/outbound.go:22`); прочее — `"outbound": <значение>`.
  Пустой `outbound` v7 → ключа нет. `rule_set` в `body` не хранится (вписывает
  сборка по `Refs`). У srs `body` = `{"outbound": …}` (или `action`).
- Порядок ключей `body` при миграции сохраняется: `match` v7 берётся сырыми
  байтами (`json.RawMessage`), цель дописывается в конец объекта склейкой, а не
  через map (иначе `canonical_roundtrip` и порядок в файле поедут).

DNS-сервер и DNS-правило (`dns.servers[]`, `dns.rules[]`, `sections.dns.*`):

```go
type DNSServer struct {
    Kind    DNSServerKind          `json:"kind"`
    Tag     string                 `json:"tag,omitempty"`  // template|user
    Ref     string                 `json:"ref,omitempty"`  // preset
    Enabled bool                   `json:"enabled"`
    Body    map[string]interface{} `json:"body,omitempty"` // только user; БЕЗ tag/kind/ref/enabled внутри
}
type DNSRule struct {
    Kind    DNSRuleKind            `json:"kind"`
    ID      string                 `json:"id,omitempty"`
    Ref     string                 `json:"ref,omitempty"`
    Name    string                 `json:"name,omitempty"` // необязательно; лаунчер не заполняет, провозит
    Enabled bool                   `json:"enabled"`
    Body    map[string]interface{} `json:"body,omitempty"` // только user: DNS-правило sing-box целиком, server внутри
}
```

- Четыре кастомных `MarshalJSON`/`UnmarshalJSON` (`dns_options.go:141/177/208/228`)
  **удаляются**; сериализация — обычные struct-теги. Плоская форма читается
  только миграцией v7→v8.
- `tag` внутри `Body` в v8 быть не должно; при эмиссии `resolve_dns.go:303-308`
  по-прежнему дописывает `tag` из поля — оставить (ловушка 2).

Корень файла v8 (`diskStateV8`, порядок полей = порядок ключей, ловушка 9):
`meta{version:8, schema:"sources_v8", …как v7}`, `sources`, `directions`,
`rules`, `vars`, **`dns`** (бывший `dns_options`), **`warp`** (бывший
`warp_accounts`). Два переименованных корневых ключа — ради бэкапа 1.0 без
маппера (SPEC §4: `dns{servers,rules,final,strategy}`, `warp[]`). Состав
объектов `dns` и `warp` не меняется. Узел и источник (`sources_v7.go`) — без
изменений формы.

Вид (view) и конструкторы — единственные писатели/читатели `Body`:

```go
// Читатель — один, как сегодня. Возвращает ВИД, собранный из полей записи и body:
func (r *Rule) DecodeBody() (interface{}, error)
type PresetBody struct{ Vars map[string]string }          // Vars никогда не nil
type InlineBody struct{ Name string; Match map[string]interface{}; Outbound string } // Match = body без outbound/action/method; Outbound = "reject" | "drop" | тег | ""
type SrsBody    struct{ Name string; Refs []string; Outbound string }               // URLs() []string оставить как алиас Refs — минимум правок у читателей
// Писатели — только эти (ловушка 20; 12 ручных json.Marshal(XBody) уходят):
func NewPresetRule(ref string, vars map[string]string) Rule
func NewInlineRule(name string, match map[string]interface{}, outbound string) Rule  // body = match ∪ цель через ApplyOutboundToRule; ключи map — как json.Marshal (сортировка), это норма для новых записей
func NewSrsRule(name string, refs []string, outbound string) Rule                    // refs дедуп с сохранением порядка (как dedupNonEmpty), пустые выброшены
func (r *Rule) SetOutbound(outbound string) error  // переписать цель в body, остальные ключи и их порядок не трогать (нужен applyRenames и UI); реализация по сырому JSON или через вид+пересборку — но ключи матчеров не пересортировывать, если можно
```

Вид `Outbound` для `InlineBody`/`SrsBody` вычисляется из body: `action=reject`
без `method` → `"reject"`, `action=reject`+`method=drop` → `"drop"`, иначе
`body.outbound` (или `""`). Это сохраняет семантику UI (`SelectedOutbound`) и
сборки (`ApplyOutboundToRule` на выходе даёт ту же карту, что сегодня).

`StableRuleID` (`rule_identity.go:29`) читает `r.Name` вместо `body.name` —
алгоритм `sanitizeIDPart` не трогать (ловушка 4).

## 1. Этап S1 — пакет `core/state`

- [x] S1.1 `rule_types.go`: тип `Rule` по §0, виды, конструкторы, `SetOutbound`;
      `NewSrsBody`/`SrsBody.normalize` заменяются канонизацией в `NewSrsRule`
      (дедуп, пустые вон). Валидация в `DecodeBody`: preset — `Ref` обязателен;
      inline — `Name` обязателен; srs — `Name` и ≥1 `Refs`. Неизвестный `Kind` —
      ошибка, как сегодня.
- [x] S1.2 `rule_order.go`: `OrderNum` → `Num` везде (`ruleNum`, `placeRuleAt`,
      `MarkRuleOrder`); `SeedRequiredRules` — литерал `{"vars":{}}` → `NewPresetRule`.
- [x] S1.3 `dns_options.go`: структуры по §0, кастомные методы удалить;
      `FindServerByTag/ByRef/FindRuleByRef`, `IsEmpty` — без изменений семантики.
- [x] S1.4 `disk_v8.go`: `SchemaVersionV8 = 8`, `SchemaNameV8 = "sources_v8"`,
      `diskStateV8` (порядок §0), `parseV8`; `save.go`: `Version = SchemaVersionV8`,
      `MarshalV7` → `MarshalV8` (переименовать вызывающих), сериализация из
      `diskStateV8`; `state.go:40` `SchemaVersion = SchemaVersionV8`;
      `schema_gate.go:28` `SchemaMajor = SchemaVersionV8`.
- [x] S1.5 `migration_v7_to_v8.go`: **миграция по сырому документу**
      `migrateV7DocToV8(doc []byte, rep *MigrationReport) ([]byte, error)` —
      верх как `map[string]json.RawMessage`; `rules[]` → записи §0 (локальные
      зеркала `v7Rule{Kind,Ref,Enabled,OrderNum,Body}`, `v7InlineBody{Name string;
      Match json.RawMessage; Outbound string}`, `v7SrsBody{Name,SrsURL,SrsURLs,
      Outbound}`, `v7PresetBody{Vars}`; неизвестный kind — запись проносится как
      есть с предупреждением в отчёт); `dns_options` → `dns` (серверы/правила из
      плоской формы в `body`: `kind/tag/ref/enabled` наружу, остаток в `body`,
      `tag` из остатка выбрасывается (предупреждение, если отличался от верхнего),
      у `template`/`preset` `body` нет); `warp_accounts` → `warp`; `sources[]`:
      `sections` у корневых узлов и у `nodes[]` папок — теми же функциями
      (`sections.rules[]` как `rules[]`, `sections.dns.*` как `dns.*`);
      `meta.version/schema` → 8/`sources_v8`. Тело `Node.body` и прочие ключи
      источников проносятся сырыми байтами. Итог документа тут же читает
      `parseV8`, поэтому порядок ключей промежуточного документа значения не
      имеет — **кроме байтов внутри `body` правил**.
- [x] S1.6 `load_router.go:105-132`: `meta == 7` → `parseV7` = сырой документ →
      `migrateV7DocToV8` → `parseV8` + `MigrationReport{FromVersion: 7}`;
      `meta >= 8` → `parseV8`; ветки 6/5/2–4 — как были, но их выход теперь
      состояние в типах v8 (см. S1.7). `Load` пишет `state.json.v7.bak` один раз
      (`writeLegacyBackupOnce` с суффиксом `.v7.bak`) перед первой записью v8 —
      по образцу `.v6.bak`. `migration_report.go:64` — жёсткое «v7» в строке лога
      заменить на текущую `SchemaVersion`.
- [x] S1.7 Цепочка v5→v6→v7→v8 остаётся рабочей: конструкторы тел в
      `migration_v5_to_v6.go:31/47/71` и `applyRenames`
      (`migration_v6_to_v7.go:741-774`) переходят на `NewXxxRule`/`SetOutbound`;
      `migration_v6_to_v7.go:819-824` (detour в DNS body) — по новой структуре;
      `load_v6.go:204-240` (`legacyCustomRulesFromV6`) — читает `Name`/`Refs`/вид.
      Итог любой миграции — состояние v8; отдельного шага «v7→v8 по типам» нет.
- [x] S1.8 Секции: `node_sections.go` (`cloneRule` копирует `Num`; `dropForeignKinds`
      без изменений семантики); `node_sections_convert.go`: правило секции из
      sing-box-документа — `body` = правило КАК ЕСТЬ после проверок
      (`rule_set` — отказ; `@var` кроме `@self` — отказ; нет ни `outbound`, ни
      `action` → дописать `"outbound":"@self"`), `Name` — как формируется сегодня
      (не менять алгоритм имени), `Num` от `NodeRuleDefaultNum` как сегодня;
      обратный эмиттер `NodeSectionsToSingbox`: inline — `body` как есть; srs —
      `rule_set` из **всех** `Refs` (ловушка 20а — баг «только первый URL»
      закрывается здесь); DNS-сервер — `body` + `tag` из поля. `ReadNodeSections`
      читает форму v8. `sync_dns.go` — по новым полям.
- [x] S1.9 Тесты `core/state` (в конце этапа): `rule_types_test.go` переписать на
      вид/конструкторы, ни один сценарий не выбрасывать; новая фикстура
      `testdata/v8_roundtrip.json` = результат `Parse(v7_roundtrip.json)` → `Save`
      (сгенерировать кодом, проверить глазами: `num`, `name` снаружи, `refs[]`,
      `vars` у preset, `body` с `outbound`, `dns`/`warp` в корне, `sections` в
      той же форме) + тест: миграция v7-фикстуры даёт ровно эти байты, а
      `Parse(v8_roundtrip.json)` → `Save` — те же байты (идемпотентность);
      `canonical_roundtrip_test.go` — на v8; `real_v088_v4.json`, `v6_roundtrip.json`
      и остальные фикстуры — цепочка миграций зелёная; тест на `.v7.bak` и
      `MigrationReport.FromVersion == 7`. Гейт этапа:
      `go build ./core/state/... && go vet ./core/state/... && go test ./core/state/...`.
      `go build ./...` на этом этапе красный — это ожидаемо, S2/S3 чинят.

## 2. Этап S2 — `core/build`, `core/config`, `core/backup`, `core/debugapi`, корень `core/`

- [x] S2.1 `resolve_route.go`: preset — `Vars` из записи; inline — карта правила =
      `body` как есть (без `ApplyOutboundToRule`, цель уже в форме sing-box), плюс
      прежняя обработка; srs — `refs` из записи, семантика «все наборы или ни
      одного» (`:300-312`) и теги `SrsRuleSetTag(id, i)` без изменений; `ruleOrderNum`
      → `Num`; `ResolvedRouteRule.OrderNum` → `Num`. `resolve_dns.go`: серверы
      `user` — `Body` + `tag` из поля (как сегодня), правила `user` — `Body`;
      `tagFromBody` — проверить, кто зовёт, и что при пустом `body.tag` берётся
      поле. `preset_merge.go`: три синтетических `state.State` (`:330/:412/:608`)
      согласованно; `CollectSrsCachedPaths` — по `Refs`, ключ `StableRuleID`.
      `sync_outbounds.go`, `migrate_outbounds_spec058.go` — `Vars`. `parsed_cache.go`:
      `RulesWithSelf` копирует `Num`; `SubstituteSelf` по сырому body как сегодня.
- [x] S2.2 `core/config_service.go:493-509` (orphan-GC .srs) — `Refs`;
      `core/config/node_document.go` (`ParseNodeDocument`/`RenderNodeDocument`) —
      форма v8; `core/config/subscription/singbox_sections_extract.go` — через
      `NodeSectionsFromSingbox`, форма v8.
- [x] S2.3 `core/debugapi/state_endpoints.go`: PATCH правил — валидация через
      `DecodeBody`, раздача `Num`; PUT/PATCH DNS — типы v8 (тело в `body`);
      `GET /state/full` — `MarshalV8`. Грепнуть `order_num|srs_url|dns_options|
      warp_accounts` в `api/`, `docs/`, `tools/`, `scripts/` и привести описания
      к v8 (только там, где это описание формата состояния/API; чужие файлы не
      трогать).
- [x] S2.4 `core/backup` — **файл бэкапа 0.12 не меняется ни на байт** (волна 2
      заведёт 1.0): `exportRule` читает `Name`/`Refs`/`Vars`/вид; `importRule`
      строит через `NewXxxRule`; `dnsRefFrom`/`importDNS` — `Body` новой структуры;
      `renumberImportedRules` → `Num`; `ServerSections.Raw` — пронос как сегодня
      (внутри теперь форма v8 — это временно, до волны 2; `node_sections_roundtrip_test`
      подправить на форму v8, не удаляя сценарии); `corpus_test.go` хелперы —
      `Refs`. Корпус `contract/corpus/backup` — зелёный без правки ожиданий.
- [x] S2.5 Golden: `TestGoldenScenarios` на `real-v088` (state.json остаётся v7 —
      это проверка миграции на живом состоянии) зелёный **без пересчёта**;
      добавить сценарий `real-v088-v8/` (копии `template.json`, `cache.json`,
      `expected.config.json`, а `state.json` — результат `Parse`+`Save` v7-файла,
      т.е. v8) — обе папки зелёные. Эталон: `ETALON_V6MIG=1 go test ./core -run
      TestEtalonV6MigOutboundSnapshot` зелёный без capture.
- [x] S2.6 Тесты `core/...` (в конце этапа) переписать на форму v8, сценарии не
      удалять. Гейт: `go build ./core/... ./internal/... && go vet ./core/... &&
      go test ./core/...` + S2.5.

## 3. Этап S3 — UI-модели и вызывающие, документация волны

- [x] S3.1 `ui/configurator/models`: `preset_ref_sync.go` — эмиссия правил через
      `NewInlineRule`/`NewSrsRule`/`NewPresetRule` (`:261/:296/:311/:674`),
      `stripOutboundAction` остаётся для карты матчеров UI; `RuleOrderFromAxis`,
      `copyOrderNum` → `Num`; `SyncStateRulesToPresetRefs` — `Vars` из записи;
      `rule_order_axis.go` — `axisProxyRules` строит `state.Rule` v8;
      `node_rule_ref.go` — `nodeRuleDisplayName` через вид; DNS-слоты и
      `buildDNSRulesFromOrder`/`syncDNSServersOnly` — структуры v8. Поле
      `OrderNum *int` у `RuleState`/`PresetRefState`/`NodeRuleRef` переименовать в
      `Num` (одно пространство имён; механически, компилятор ведёт).
- [x] S3.2 `presentation/`, `business/`, `tabs/`, `dialogs/`,
      `outbounds_configurator/` — по таблице CODEMAP §3.4: `preset_ref_helpers.go:199-213`
      (DNS-сервер → плоский wizard-JSON: теперь из `Body` + `tag`), `:223-240`
      (DNS-правила → текст из `Body`), `preset_ref_convert.go` (`Num`),
      `add_rule_dialog.go:816` (`Num`), `dns_tab.go`, `dns_user_rules.go`,
      `dns_unified_rules.go`, `node_sections.go`. Поведение и тексты UI не меняются.
- [x] S3.3 Документация волны: `CODEMAP.md` — адреса и новые функции; SPEC 121
      `SPEC.md` §10 и `CODEMAP.md` §11–§14 — короткая врезка «форма записей с
      state v8 — SPEC 127 §2, примеры ниже устарели»; `docs/release_notes/upcoming.md`
      → Technical/Техническое: state v8 (`num`, `name`, `refs[]`, `vars`, `body`;
      DNS `body`; корень `dns`/`warp`), миграция v7→v8 при первой загрузке с
      резервной копией `.v7.bak`; удалённый режим требует одинаковой мажорной
      схемы 8 у обеих машин. `SPECS/features/*.md` — только если там описана
      форма правил/DNS в JSON (грепнуть `order_num|srs_url`).
- [x] S3.4 Полный прогон один раз: `go build ./... && go vet ./... && go test ./...`
      (UI-пакеты Fyne — если нужен `build/test_darwin.sh nopause`, использовать
      его), `gofmt -l` пусто, golden/эталон по S2.5. Отчёт — с адресами
      `файл:строка` того, что добавлено, и списком тестов, которые переписаны.

## 4. Инварианты волны (проверяют ревьюеры)

1. `config.json` из одного и того же состояния — байт-в-байт прежний (golden
   `real-v088` и `real-v088-v8`, эталон v6mig).
2. Ничего из v7-файла не теряется: каждое правило (в т.ч. с неизвестным kind),
   каждый DNS-сервер/правило (в т.ч. `tag`/`detour`/любые ключи тела), секции
   узлов, `num`, `enabled`, `vars`, все URL наборов (`srs_url` и `srs_urls`),
   `id`/`name`, если были. Порядок записей и ключей `match` сохраняется.
3. `StableRuleID` даёт ту же строку, что до миграции (теги `rule_set`, имена
   файлов srs-кэша).
4. `import(export(x))` бэкапа 0.12 — как раньше; корпус бэкапа зелёный без
   правки ожиданий.
5. Идемпотентность: `Parse(v8)` → `Save` → `Parse` → `Save` байт-в-байт.
6. Файл v8 не читается как v7 (гейт `meta >= 8`), файл v7 читается и мигрирует
   с резервной копией и отчётом.
7. UI: оси, слоты, drag, toggle, конвертация preset→custom, DNS raw-JSON —
   поведение прежнее (по тестам моделей и по прочтению кода), `OrderNum` в коде
   не остаётся (`rg OrderNum` пусто вне миграции v7→v8).

## 5. Этап FIX — правки по ревью волны 1

Адреса и разбор — `CODEMAP.md` §11. Каждая правка проверена откатом: без неё
соответствующий тест краснеет.

- [x] FIX.1 Самостоятельный `action` (`sniff`/`hijack-dns`/`resolve`) — эффект
      правила, а не цель: `isTargetKey` (`rule_types.go:358`) и четыре
      раскладки через неё (`NewInlineRule`, `DecodeBody`, `SetOutbound`,
      `ruleMatchAndOutbound`); миграция снимает из `match` только те ключи,
      которые тут же перепишет цель. Находки ревью 1 и 4.
- [x] FIX.2 Запись неизвестного `kind` не теряет номер: `renameOrderNumKey`
      (`migration_v7_to_v8.go:225`) переименовывает `order_num` → `num` в сырой
      записи. Находка 2.
- [x] FIX.3 `id` и `name` DNS-правила — оба метаданные и оба вне `body`
      (`migrateV8DNSRules:403`). Находки 3, 5, 8.
- [x] FIX.4 Круг визарда не стирает провозимые метаданные: поля `ID`/`Name` у
      `DNSUserRule`, `CarryRuleMetadata`/`CarryDNSRuleMetadata`
      (`rule_identity.go:95`/`:132`) на пути сохранения. Находки 6 и 7.
- [x] FIX.5 Тесты — расширены существующие сценарии (`migration_v7_to_v8_test.go`,
      `backup_test.go`, `spec127_state_v8_roundtrip_test.go`); новых файлов нет.

## 6. Волна 2 — бэкап 1.0 (экспорт = состояние v8, legacy-вход 0.x, два писателя)

Норма — SPEC.md §4, ONE_NAMESPACE.md §1 (таблицы «Узел», «Контейнеры»,
«Правило», «DNS») и §2. Адреса — CODEMAP.md §4 (бэкап), ловушки §7: 6, 7, 10,
11, 16, 21, 22, 23, 26. Правила исполнителю — как в шапке файла; дополнительно
разрешено править `contract/registry/backup_warnings.json` (добавить код) —
схему и документы контракта НЕ трогать (волна 3).

### 6.0 Форма файла 1.0 — решения

Корень (`core/backup/backup10.go`, тип `Backup10`; порядок полей = порядок ключей):

```json
{
  "lx_backup": 2,                       // int-маркер формата, как сегодня (1 = 0.x); 2 = контракт 1.0
  "exported_by": { "app": "launcher", "version": "…", "platform": "…" },
  "exported_at": "RFC3339",
  "sources":    [ …state.Source… ],     // union по kind: server | folder | subscription | chain
  "directions": [ …backup.Direction… ], // форма 0.12 (direction.schema.json) — норма её не трогала, LxBox-поля label/ping_* живут там
  "rules":      [ …state.Rule… ],       // форма v8
  "dns":        { …state.DNSOptions… }, // strategy, final, default_domain_resolver?, servers[], rules[] — форма v8
  "vars":       { … },                  // только переносимые (IsPortableVar), как сегодня
  "route":      { "final": "…" },       // как сегодня
  "warp":       [ … ]                   // сырой JSON, как сегодня
}
```

`sources[]` — **сериализация `state.Source` теми же типами** с тонким слоем:

- `server` (корневой): `kind, id, tag, enabled, origin?, body, detour?, sections?`
  — ровно `state.Source` (encoding/json по struct-тегам). `service`/`reason`
  у корневых узлов не бывают.
- `folder`: `kind, id, name, tag_policy?, fold?, detour?, nodes[]` — узлы
  всех видов как есть (`state.Node`), включая `chain`/`auto` члены и их
  `sections`. Настройки папки едут (у них теперь есть дом в схеме 1.0);
  сторона, которая их не применяет, игнорирует молча (BACKUP.md §1).
- `subscription`: `kind, id, name, enabled, url, identity{user_agent?, hwid?,
  send_hwid?, hash_device_model?}, tag_policy?, fold?, skip?, max_nodes?,
  update{interval_hours, auto_refresh}?, relays_in_directions?,
  disabled{тег: unix seconds}?` — **без** `nodes[]`, `meta`, `update_status`
  (кэш и рантайм) и без `pending_disabled` (договорённость с LxBox 14.09:
  у них отметка = `{идентичность: unix seconds}` с TTL-очисткой, форма 0.12
  остаётся). `disabled` в файле = `exportDisabledMap` как сегодня (сырые теги
  выключенных узлов кэша ∪ `pending_disabled`, значение 0); импорт —
  `mergeDisabledMarks` как сегодня. `fold` — форма 0.12 (`source_fold.schema.json`,
  `exportFold`/`importFold`), а не state-ный `replace`: у контракта уже есть имя.
- `folder`: `fold` там же по той же причине (если у папки есть `replace`).
- `chain`: `kind, id, tag, enabled, body, hops[{folder_id?, tag}]` — как
  `state.Source`.
- **Исключения из «бэкап = состояние» (тонкий слой, зафиксировано с LxBox
  14.09.2026):** `directions[]` (форма 0.12, `direction.schema.json`), `fold`
  (`source_fold.schema.json`), `disabled{}` (0.12), `identity{}` (объект — и в
  состоянии v8 тоже, см. ниже), `vars` (только переносимые), `route{final}`,
  `warp[]` (сырой). Всё остальное — struct-теги состояния без переименований.
- Ссылки `detour{folder_id, tag}` и `hops[].folder_id` — ULID папки
  **машины-экспортёра**; импорт переписывает их по карте «id из файла → id
  локальной папки» (совпавшая по имени папка держит локальный id, новая —
  id из файла; сегодня это делает `freshIDIfTaken`/`resolveImportedHops`
  для 0.12 — та же механика, только ключ теперь `folder_id`, а не тег).
  Ссылка, чью папку в файле не нашли, — как сегодня у 0.12 (`detour` снимается
  с предупреждением прежним кодом).

**Состояние v8 дополняется (без смены номера схемы — v8 ещё не выпущен):**
`Source.Identity *SubscriptionIdentity{UserAgent, HWID, SendHWID, HashDeviceModel}`
(`json:"identity,omitempty"`) вместо четырёх плоских полей `user_agent`/`hwid`/
`send_hwid`/`hash_device_model`; миграция v7→v8 (`migration_v7_to_v8.go`)
собирает объект из плоских ключей; фикстура `v8_roundtrip.json` и golden
`real-v088-v8/state.json` перегенерируются (конфиг — байт-в-байт прежний).
Читатели четырёх полей (fetcher подписок, UI подписки, `exportSourceIdentity`)
переходят на `src.Identity`. Тип `SubscriptionIdentity` переезжает из
`core/backup/types.go` в `core/state` (с `UnmarshalJSON`, который различает
«ключа нет» и `null`, и `UnappliedKeys` для mobile-only ключей — сегодняшняя
логика, только дом другой); `core/backup` использует его через `state.`.

**Сделано (этап B1, ветка `spec127/backup-10`):** объект `identity` в
состоянии и миграции, публичные читатели/сеттеры, переведены все читатели
четырёх полей; фикстуры перегенерации не потребовали (identity в них нет —
проверено регенерацией, diff пуст). Адреса — CODEMAP §12.1.

### 6.1 Экспорт — два писателя

- [x] W2.1 `core/backup/export10.go`: `Export10(s *state.State, opts) (*Backup10,
      []Warning, error)` — чистая функция состояния (П1): `sources[]` по правилам
      §6.0 (копии, не общие указатели; `Nodes=nil`/`Meta=nil`/`UpdateStatus=nil`
      у подписок; `PendingDisabled` не пишется, вместо него `disabled` из
      `exportDisabledMap`; `fold` из `exportFold`; `identity` — объект состояния как есть), `directions` через сегодняшний
      `exportDirection`, `rules`/`dns` — **срезы состояния как есть** (никаких
      `exportRule`/`dnsRefFrom`), `vars` через `exportVars`, `route`, `warp` через
      `exportWarp`. Предупреждения экспорта: `WarnBackupReplaceTagDerived` как
      сегодня; `backup_local_only_dropped` в 1.0 **не эмитится** (полей без дома
      больше нет).
- [x] W2.2 `core/backup/legacy_write_012.go`: сегодняшние `Export`, `exportRule`,
      `dnsRefFrom`, `exportServer*`, `exportFolder`, `exportSubscription`,
      `exportChain`, `droppedLocalOnlyFields` переезжают сюда как `Export012`
      (файл 0.12 **байт-в-байт прежний**, включая порядок ключей и warnings;
      секции узлов в 0.12 **не пишутся** — норма §4 ONE_NAMESPACE: `ServerSections`
      и пронос `Raw` из 0.12-писателя снимаются, `backup_test`/`node_sections_roundtrip_test`
      переводятся на 1.0).
- [x] W2.3 Точка выбора: `type ExportFormat int` (`ExportFormat012`, `ExportFormat10`),
      `const BackupExportFormatDefault = ExportFormat012` (одна константа, SPEC
      §4), `ExportOptions.Format`; `ExportFile(path, s, opts)`/`WriteFile` пишут
      выбранный формат. UI: в диалоге экспорта чекбокс «Backup format 1.0 (new;
      requires LxBox with 1.0 import)» через `locale.T` (ключ английский; ru.json
      не трогать), по умолчанию по константе; grep `backup.Export(` — все
      вызывающие (UI, debug API, tools) получают `Format`.

### 6.2 Импорт — один путь слияния, два входа

- [x] W2.4 Разделить сегодняшний `Import` на (а) **декодирование записей файла в
      типы состояния** и (б) **слияние** (`merge.go` + `importDNS`/`importWarp`/
      `renumberImportedRules` + правила §9 BACKUP.md без изменений: подписки по
      `url` байт-в-байт, серверы по телу, папки по имени, цепочки/Направления по
      тегу, DNS «своё сильнее», `rules[]` — единственная полная замена, `vars`
      переносимые, `route.final` при известной цели, `warp` добавлением).
      Вход 1.0 (`import10.go`): `Backup10` → записи состояния напрямую (копии),
      `identity` объектом, `disabled{}` → `mergeDisabledMarks`, `fold` → `importFold`,
      переписка `folder_id` у `detour`/`hops` по карте id. Вход 0.x
      (`legacy_read_0x.go`): сегодняшние `importSubscription`/`importServer`/
      `importChain`/`importRule`/`importDNS`-разбор → те же записи состояния
      (через `NewXxxRule`, `state.DNSServer{Body}`), `node_tag`/`config_json`/
      `uri`/`label` → `tag`/`body`/`origin`, `folder` → сборка папки по имени
      (как сегодня в `mergeServers`), `disabled` → `mergeDisabledMarks`,
      `chain: SourceChain` → `body`+`hops`. После декодирования — **один** код
      слияния для обоих входов.
- [x] W2.5 Секции при импорте: «файл замещает секции узла» (как сегодня
      `applyImportedSections`); записи чужого `kind` внутри секций отбрасываются
      с кодом **`backup_section_record_dropped`** (side=import, params
      `["node", "kind"]`) — добавить в `contract/registry/backup_warnings.json`
      по формату соседей и в словарь кодов пакета; sync-тест словаря зелёный.
      **Перенумерация оси** (`renumberImportedRules`) идёт по объединённой оси
      `rules[]` ∪ `sections.rules[]` всех приехавших узлов с сохранением
      относительного порядка (SPEC 126 L2): узловые правила получают номера в
      том же проходе, а не абсолютные из файла.
- [x] W2.6 `file.go`: `Parse` различает `lx_backup` 1 и 2 (иное — отказ с
      прежним кодом); `scanUnknown` — два набора списков ключей: для 1.0 —
      ключи struct-тегов состояния (генерировать из типов рефлексией, чтобы
      списки не расходились с кодом), для 0.x — прежние списки без изменений
      (ловушка 26). `decodeTolerant` — оба формата.
- [x] W2.7 Тесты (в конце волны): (1) round-trip на **полном** состоянии v8
      (подписка с identity/skip/disabled/fold, папка с tag_policy и узлами
      трёх видов, корневой сервер с секциями Tailscale, цепочка с hops в папку,
      Направления, правила всех трёх видов с `action`, DNS всех видов, warp):
      `Export10` → `Parse` → импорт в пустое состояние → `Export10` — **файл
      байт-в-байт** (П1, чистота) и состояние эквивалентно (ids подписок/папок
      сохраняются, секции на месте, номера оси относительный порядок); (2) все
      существующие кейсы `contract/corpus/backup/*.backup.json` (0.12) проходят
      legacy-входом с теми же `expected` — ожидания **не править**; (3) 0.12-писатель:
      `Export012` на фикстуре даёт тот же файл, что до волны (снять эталон до
      правок: `go test`-хелпером сохранить вывод старого `Export` в
      `core/backup/testdata/export012_*.json` ПЕРЕД рефакторингом, потом
      сравнивать); (4) `schema_test` — 0.12-писатель против `backup.schema.json`
      как сегодня; проверка 1.0 против схемы — волна 3. Полный `go test ./...`
      один раз; golden/эталон без пересчёта.
- [x] W2.9 **Debug API — паритет с UI (просьба владельца 14.09):**
      `GET /backup/export?format=1.0|0.12` (без параметра — `BackupExportFormatDefault`;
      тело ответа — файл бэкапа как есть, `Content-Disposition` с именем из
      `SuggestFileName`; предупреждения экспорта — в заголовке `X-Backup-Warnings`
      JSON-массивом кодов или отдельным полем при `?envelope=1`), `POST /backup/import`
      (тело — файл бэкапа любого читаемого формата; ответ — `ImportResult`
      с warnings и счётчиками; после импорта — `Save` состояния и пересборка
      конфига тем же путём, что UI-импорт), `GET /backup/formats` (какие форматы
      читает/пишет эта сборка и дефолт). Зарегистрировать в `/help`; те же три
      маршрута под `/remote/machines/{id}/…` — только если удалённые машины
      уже проксируют `/state/*` общим механизмом (иначе не заводить). Поднять
      версию API-спеки (`/version`, `api/` — если там есть описание маршрутов,
      дописать). Тест эндпоинтов — в существующем стиле `core/debugapi/*_test.go`
      (один интеграционный: export 1.0 → import в пустое состояние → export
      байт-в-байт).
- [x] W2.8 `CODEMAP.md` §4 — новые файлы/функции; `TASKS.md` чекбоксы;
      `docs/release_notes/upcoming.md` — «формат бэкапа 1.0 (пока выключен по
      умолчанию, чекбокс в экспорте), импорт читает оба».

- [x] W2.10 **Правки по РЕАЛЬНЫМ данным владельца (после волны 2).** Два
      дефекта слияния, найденные импортом живого состояния, а не ревью; оба
      воспроизведены до правки и проверены откатом. Разбор и адреса —
      `CODEMAP.md` §16.
      (1) **`importDNS` схлопывал preset-серверы:** ключ был `kind`+`tag`, а у
      `kind: preset` тега нет вовсе (идентичность — `ref` вида
      `russian:yandex_doh`), поэтому ВСЕ preset-записи давали один ключ
      `preset\x00` и после первой отбрасывались как «своё сильнее» — молча. На
      живом состоянии из 17 DNS-серверов после импорта в пустое оставалось 15
      (пропадали `russian:yandex_doh`, `russian:yandex_dot`). Ключ стал единым
      `kind`+`tag`+`ref` (`core/backup/import.go:584`); норма §9 п. 5 не
      меняется — «по `kind`+`tag`» для preset читается как `kind`+`ref`.
      Проверено грепом, что этот ключ строится ровно в одном месте.
      (2) **Папки-тёзки при импорте 1.0:** норма §9 п. 3 («папка по имени»)
      писалась под 0.12, где у папки нет `id`; в 1.0 он есть, а UI допускает
      две папки с одним именем (у владельца две «Folder 1», 4 и 6 узлов).
      Карта «имя → папка» видела первую, и состав второй папки ФАЙЛА
      дописывался в первую ЛОКАЛЬНУЮ — состояние росло на каждом импорте
      собственного экспорта. Введён `folderIndex` (`core/backup/merge.go:529`)
      с порядком «сперва `id`, затем имя»; вход 0.x зовёт `lookup("", name)` и
      не затронут. **Хвост того же дефекта (проверка владельца на реальных
      данных):** импорт того же файла в ПУСТОЕ состояние давал 17 источников
      вместо 18 — вторая папка файла (id B) по имени попадала в только что
      заведённую из файла первую (id A). По имени теперь матчатся только
      папки, существовавшие в состоянии ДО импорта: `folderIndex` помечает
      собственные создания (`addCreated:551` — запись 1.0, по имени НЕ находится;
      `addExisting:544` и `addCreated0x:559` — находятся). Отдельный метод для
      0.x обязателен: там папка собирается из плоского `servers[]` по имени, и
      пометка `fresh` завела бы по папке на запись (мутация красит корпус). Подпискам и корневым серверам то же не нужно: там ключ —
      `url` байт в байт и ТЕЛО, настоящая идентичность записи, и менять их
      значило бы менять норму §9 пп. 1–2 (разбор — CODEMAP §16.2).
      Тесты — `merge_test.go`: `TestMergeDNSPresetServersSurviveByRef`,
      `TestMergeDNSPresetServerNotOverwrittenByFile`,
      `TestImport10TwinFoldersMatchByID`,
      `TestImport10FolderFromOtherMachineMatchesByName`. Корпус
      `contract/corpus/backup`, эталоны `export012_*.json` и писатель 0.12 не
      тронуты; полный гейт зелёный.

**Сделано (этап B2, ветка `spec127/backup-10`):** W2.4–W2.6. Импорт разрезан
на декодирование (`legacy_read_0x.go` для 0.x, `import10.go` для 1.0 — оба
отдают `decodedFile`) и ЕДИНОЕ слияние (`applyDecoded` + `mergeSources`);
правила §9 не менялись, корпус зелёный без правки ожиданий. Код
`backup_section_record_dropped` заведён в реестре, в словаре пакета и в
тексте UI; sync-тест словаря добавлен. `Parse` различает `lx_backup` 1 и 2,
`scanUnknown` — два набора списков (1.0 генерируется рефлексией по
struct-тегам состояния). Из W2.7 сделаны пп. 1–3 (круг 1.0, корпус, эталоны
0.12); п. 4 не потребовал правок. Адреса — CODEMAP §13.

**Сделано (этап B3, ветка `spec127/backup-10`):** W2.3 UI-часть закрыта ещё
B1 (чекбокс через `locale.T`, дефолт по константе, все вызывающие через
`ExportFile`/`Format` — проверено грепом). W2.9: `core/debugapi/backup_endpoints.go` —
`GET /backup/export` (тело = файл, `Content-Disposition`, коды потерь в
`X-Backup-Warnings`, `?envelope=1`), `POST /backup/import` (любой читаемый
формат, гейт мажора схемы, `Save` + `RebuildConfigIfDirty`),
`GET /backup/formats`; зарегистрированы в `/help`; зеркала машин под
`/remote/machines/{id}/backup/*` — заведены, потому что `/state/*` у машин уже
проксируется общим `stateAccess`. Версия API-спеки НЕ поднята: поверхность
аддитивная (SPEC 100 §254). W2.7 п. 1 доведён до полноты §6.0: `richState10`
дополнен srs-правилом, правилом-эффектом и правилом-отказом, ссылочными
DNS-записями; добавлена сверка СОСТОЯНИЯ после круга (`assertStateEquivalent10`),
плюс круг через HTTP в `core/debugapi`. W2.8: `docs/API.md`/`API.ru.md`,
`docs/release_notes/upcoming.md`, CODEMAP §14. Полный прогон волны
(`gofmt -l .`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`) —
зелёный, 38 пакетов `ok`, ни одного `FAIL`. Адреса — CODEMAP §14.

### 6.3 Инварианты волны (ревьюеры)

1. Файл 0.12 из `Export012` — байт-в-байт как до волны (эталоны W2.7 п.3).
2. Импорт 0.12 — прежняя семантика: корпус зелёный без правки ожиданий.
3. `import(export10(x))` в пустое состояние → `export10` даёт тот же файл; ни
   одно поле §6.0 не теряется (identity, skip, tag_policy, fold, disabled, hops с
   folder_id, detour, sections всех трёх списков, num, enabled, vars, refs).
4. Секции: файл замещает; чужой kind — код; ось перенумерована совместно.
5. Ссылки на папки переживают импорт на другой машине (id из файла ≠ локальные).
6. Дефолт формата — константа; UI-чекбокс переключает; оба входа читаются
   всегда; `lx_backup` ≠ 1/2 — отказ.

## 7. Этап FIX — правки по ревью волны 2

Адреса и разбор — `CODEMAP.md` §15. Шестнадцать находок ревью = семь дефектов
(часть находок описывала один дефект с разных сторон). Каждая правка проверена
откатом: без неё соответствующий тест краснеет. Все семь жили ИСКЛЮЧИТЕЛЬНО в
пути 1.0 — писатель 0.12, его эталоны, корпус 0.12 и правила слияния §9 не
тронуты ни на строку.

- [x] FIX.1 Имя группы свёртки едет ЯВНО: `Source10.FoldTag` (`fold_tag`)
      рядом с объектом `fold` формы контракта 0.11, где поля тега нет вовсе.
      Дериватив остался запасным ходом для чужого файла. Снял сразу четыре
      следствия: подмену явного имени, ОДИН тег у папки и первой свёрнутой
      подписки (позиционная формула определена для `subscriptions[]`), потерю
      `route.final` и молчание экспорта о папке. `WarnBackupReplaceTagDerived`
      в 1.0 больше не эмитится — потери нет. Находки 1, 2, 3, 7, 11.
- [x] FIX.2 Проверка целей правила в 1.0 была МЕРТВА: `DecodeBody` возвращает
      указатели, а `ruleTarget10` матчил значения — правило с несуществующей
      целью приезжало включённым и роняло `config.json`. Находка 10.
- [x] FIX.3 Настройки СОВПАВШЕЙ записи берутся из файла: `applyFolderSettings`
      для папки и `relays_in_directions` у подписки, под флагом
      `decodedSource.FullSettings` (у 0.12 этих полей нет, и применять их
      «ноль» значило бы стирать локальное импортом старого файла). Счётчик
      `UpdatedFolders`. Находки 4, 8, 14, 15.
- [x] FIX.4 `dns.default_domain_resolver` переживает круг: поле заведено в
      `decodedDNS` и применяется в `importDNS` (писатель клал его в файл с
      самого начала, читателя не было). Находки 5, 6, 12.
- [x] FIX.5 Ссылочные члены папки (chain/auto) ключуются ТЕГОМ, как в корне:
      `folderMemberKey`. Повторный импорт больше не дописывает копию. Находка 13.
- [x] FIX.6 Поле `sections` у узла, которому оно не положено, снимается с
      кодом `backup_section_record_dropped` — ровно как обещает реестр.
      Находка 16.
- [x] FIX.7 Mobile-only ключи identity вход 1.0 больше не складывает в
      состояние молча: `importIdentity10` собирает объект из применяемой
      четвёрки и называет остальное, как вход 0.x. Находка 9.
- [x] FIX.8 Тесты — расширены существующие сценарии (`purity_test.go`,
      `merge_test.go`, `backup_test.go`, `identity_test.go`,
      `node_sections_roundtrip_test.go`); новых файлов нет. Каждый проверен
      мутацией. Полный прогон один раз: `gofmt -l .` пусто, `go build ./...`,
      `go vet ./...`, `go test -count=1 ./...` — зелёные, 38 пакетов `ok`.
