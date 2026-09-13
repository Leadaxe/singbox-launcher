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
