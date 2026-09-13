# SPEC 121 · Секции узла: DNS и маршрут, которые узел носит с собой

Статус: N. Ветка develop. **§10 (волна 3) главнее §2–§5 там, где они расходятся: хранимая форма, инъекция вместо развёртывания, отсутствие якоря `kind=node`.** Карта кода — `CODEMAP.md` рядом (читать первой;
код — только точечно по её адресам; свои правки отражать в ней тем же
куском). Нормативная база: `SPECS/features/sources.md` (узел, тело, свобода
узлов), `contract/docs/BACKUP.md` §1–2 (чужое объявленное игнорируется
молча), D-049…D-053 (модель пресетов и ось порядка).

Номер 120 в комментариях кода уже означает «служебные узлы / релеи BYPASS»
(CODEMAP §10 п.1), поэтому эта задача — 121.

## 1. Проблема

Часть узлов не работает без сопутствующих секций конфига. Endpoint
Tailscale (sing-box ≥ 1.12) бесполезен без DNS-сервера типа `tailscale`,
привязанного к нему, DNS-правила на `*.ts.net` и правила маршрута на
`100.64.0.0/10`. Такие же связки нужны WireGuard-узлу с подсетями за пиром
и любому узлу, к которому пользователь хочет привязать «свой» DNS.

Сегодня узел — только тело outbound/endpoint (`state.Node.Body`,
`core/state/sources_v7.go:135`). Секции `dns` и `route` из вставленного
sing-box JSON импорт выбрасывает (`singboxIgnoredSections`,
`core/config/subscription/singbox_import.go:27`), и пользователь
вынужден собирать связку руками на трёх вкладках, а при выключении узла
— разбирать. Пресет с переменными этой роли не закрывает: у сопутствующих
секций нет ни одного решения пользователя, всё выводится из тега узла, а
пресет существует в одном экземпляре и не привязан к узлу.

## 2. Решение

Узел получает необязательное поле **`sections`** — набор фрагментов
конфига, которые живут и умирают вместе с ним:

```json
{
  "kind": "server",
  "tag": "tailscale-node",
  "body": { "type": "tailscale", "state_directory": "tailscale", "auth_key": "tskey-…" },
  "sections": {
    "dns_servers": [ { "type": "tailscale", "tag": "ts-dns", "endpoint": "@self" } ],
    "dns_rules":   [ { "domain_suffix": [".ts.net"], "server": "ts-dns" } ],
    "rules":       [ { "ip_cidr": ["100.64.0.0/10"], "outbound": "@self" } ]
  }
}
```

При сборке секции разворачиваются как **неявный пресет узла**:

- `@self` — плейсхолдер «финальный тег этого узла». Подставляется в любое
  строковое значение любой секции (`outbound`, `detour`, `endpoint`, что
  угодно). Финальный тег — тот, под которым узел эмитится в конфиг (с
  учётом TagPolicy папки), а не сырой `Node.Tag`.
- Теги DNS-серверов секции префиксуются `<финальный тег узла>:<локальный тег>`
  (та же схема, что у пресетов, `TagSeparator`, D-012). Поле `server` в
  `dns_rules`, равное локальному тегу сервера из этой же секции, получает
  тот же префикс; любое другое значение остаётся как есть (ссылка на
  шаблонный или пресетный сервер).
- Правило маршрута без `outbound` и без `action` получает `"outbound": "@self"`.
  Это делается **при сохранении в редакторе** (пользователь видит результат
  в теле), и повторно, защитно, при сборке.
- Правила маршрута узла встают на **ось порядка** как один якорь с именем
  узла: запись `state.Rule` нового вида `node`. Якорь виден в списке правил,
  перетаскивается, включается и выключается, но не редактируется и не
  удаляется отдельно от узла. Появляется, когда у узла есть непустые
  `rules`; исчезает вместе с ними или с узлом (пересев на каждой
  нормализации, как у `traffic-processing`, D-050).
- DNS-серверы и DNS-правила узла попадают в секцию `dns` следом за
  серверами и правилами пресетов, в порядке узлов. Оси порядка у DNS нет
  (CODEMAP §10 п.9), и эта задача её не заводит.
- Узел выключен (`Enabled=false`) → узел не эмитится → секций в конфиге
  нет, якорь остаётся на оси в неактивном виде.

Секции **не** проходят через структуру `template.Preset`: её DNS-сервер
типизирован (`PresetDNSServer`, `core/template/preset_types.go:349-390`) и
потерял бы поле `endpoint` и любое другое незнакомое. Секции хранятся и
разворачиваются как сырые JSON-объекты, а из конвейера пресетов
переиспользуются примитивы: `TagSeparator`, строгая подстановка
`template.SubstituteVarsInJSONStrict` (`core/template/substitute.go:88`) с
единственной переменной `self`, ось порядка `state.Rule.OrderNum`, слияние
в `dns`/`route` в тех же точках, что у пресетов.

### Чего в секциях нет (v1)

- `rule_set` — ни объявления, ни ссылок. Правило с ключом `rule_set`
  отвергается редактором.
- Переменных, кроме `@self`. Другая `@var` — ошибка редактора; на сборке
  строгая подстановка роняет фрагмент с warning (Dropped-каскад: один
  фрагмент, не вся секция).
- Секций `outbounds`/`endpoints`, `inbounds`, `log` и прочих.
- Секций у узлов подписки (они несвободны, `features/sources.md` §Свобода),
  у цепочек, Auto и unsupported. Только `kind=server` в корне и в папке.

## 3. Модель данных

### 3.1 state.json (v7, аддитивно, без смены версии схемы)

`state.Node` (`core/state/sources_v7.go:116-166`) получает поле:

```go
// Sections — сопутствующие фрагменты конфига узла (SPEC 121). nil = нет.
// Только kind=server; у остальных видов поле игнорируется при чтении.
Sections *NodeSections `json:"sections,omitempty"`

type NodeSections struct {
    DNSServers []json.RawMessage `json:"dns_servers,omitempty"`
    DNSRules   []json.RawMessage `json:"dns_rules,omitempty"`
    Rules      []json.RawMessage `json:"rules,omitempty"`
}
```

Тела хранятся байт-в-байт как `json.RawMessage` — порядок ключей значим
(CODEMAP §10 п.21). Метод `IsEmpty()`; пустой набор нормализуется в `nil`
при сохранении.

`state.Rule` (`core/state/rule_types.go:46-72`) получает вид
`RuleKindNode = "node"` с телом:

```go
type NodeRuleBody struct {
    FolderID string `json:"folder_id,omitempty"` // "" = корень
    Tag      string `json:"tag"`                 // сырой тег узла в контейнере
}
```

Идентичность узла для ссылки — `NodeLink{FolderID, Tag}` (SPEC 112,
`core/config/configtypes/types.go:257-262`). `Enabled` и `OrderNum` — как у
других видов. Ref не используется.

### 3.2 Проекции сборки

`configtypes.CanonicalNode` (`types.go:222-247`) и `ParsedNode`
(`types.go:645-736`) проносят `Sections` до эмиссии. В кэше сборки
(`core/build/parsed_cache.go`) появляется:

```go
// NodeSections — секции узлов, дошедших до эмиссии (SPEC 121), в порядке
// эмиссии. Узел, не попавший в конфиг (выключен, отброшен), сюда не входит.
NodeSections []NodeSectionSet

type NodeSectionSet struct {
    FinalTag string
    Link     configtypes.NodeLink // для сопоставления с state.Rule kind=node
    DNSServers, DNSRules, Rules []json.RawMessage // сырые, до подстановки
}
```

Заполняется там же, где `NodeOrigins` (`core/rebuild_snapshot.go:110`,
источник — `OutboundGenerationResult`, `core/config/outbound_generator.go:1405-1432`),
и во всех трёх производителях `ParsedCache` (шапка `parsed_cache.go:1-15`:
rebuild из сети, rebuild из raw, in-memory режим визарда для preview).

### 3.3 Бэкап (контракт)

`servers[]` получает поле `sections` с поддержкой **launcher**:

```json
"sections": {
  "dns_servers": [...], "dns_rules": [...], "rules": [...],
  "rule_num": 945
}
```

`rule_num` — позиция якоря на оси (`state.Rule.OrderNum` соответствующей
записи `kind=node`); отсутствует, если правил нет. Запись `kind=node` в
`rules[]` бэкапа **не пишется** (она производная от узла), и `rules[].kind`
контракта не расширяется — LxBox ничего нового в `rules[]` не увидит.
Поле объявляется в `contract/schema/backup.schema.json` и в таблице
`servers[]` `contract/docs/BACKUP.md:83-94` со значением «launcher»; по
правилу §1 LxBox игнорирует его молча и по возможности провозит.

Слияние (§9): серверы сопоставляются по телу (CODEMAP §10 п.29). При
совпадении `sections` из файла **замещают** локальные (настройки файла
сильнее локальных, как у подписки). При отсутствии поля в файле локальные
секции остаются. Импорт `rules[]` — полная замена (`import.go:235`), после
неё якоря `kind=node` пересеваются из узлов с `rule_num` из файла.

Версия контракта этой задачей **не поднимается**: предложение 0.13.0
фиксируется черновиком D-098 и задачей `## 9` в `contract/TASKS_LXBOX.md`;
поднимать — после ответа LxBox (правило обеих сторон).

## 4. Сборка

Порядок — `buildOrderedSections` (`core/build/build.go:242-279`), новых
секций верхнего уровня не появляется, меняется наполнение `dns` и `route`.

1. **Финальные теги.** `collectAllFinalOutboundTags`
   (`core/build/preset_outbounds.go:373`) без изменений: узлы с секциями —
   обычные outbound/endpoint, их теги уже там. Теги DNS-серверов узла в
   этот набор не входят (это DNS-пространство имён).
2. **Развёртывание** — новая функция в `core/build/`
   (`node_sections_expand.go`): на вход `NodeSectionSet`, на выход
   фрагменты `PresetFragments`-подобной формы (`DNSServers`, `DNSRules`,
   `RoutingRules` как `[]map[string]interface{}`) плюс warnings. Шаги для
   каждого фрагмента: `SubstituteVarsInJSONStrict` с `{"self": FinalTag}`;
   неразрешённая `@var` → фрагмент выпадает с warning вида
   `node %q: unresolved @var in %s — fragment dropped`. Затем префикс тега
   DNS-сервера; префикс `server` в DNS-правиле, если равен локальному тегу;
   `outbound: FinalTag` в правило без `outbound`/`action`. Правило с
   `rule_set` → выпадает с warning (редактор такое не сохраняет, но state
   мог прийти из бэкапа).
3. **`dns`** — в `MergePresetsIntoDNS` (`core/build/preset_merge.go:312`)
   после серверов пресетов (`:344-356`) и после правил пресетов
   (`:369-383`): серверы узлов, потом правила узлов, в порядке
   `Cache.NodeSections`. Dedup по тегу с уже стоящими серверами — как у
   пресетов (`:336-343`), дубль → warning, первый побеждает.
   `pruneDNSGroupMembers`/`repairDanglingDNSRefs` (`:396`, `:402`) должны
   видеть узловые фрагменты — вставка идёт **до** них.
4. **`route`** — в резолве правил (`core/build/resolve_route.go`, ветки
   `:248` preset и `:276` inline) добавляется ветка `RuleKindNode`: по
   `NodeRuleBody` ищется `NodeSectionSet` с тем же `Link`; не найден
   (узел выключен/отброшен) → правило пропускается без warning; найден →
   `RoutingRules` фрагмента эмитятся подряд на `OrderNum` якоря.
   `MergePresetsIntoRoute` (`preset_merge.go:219`): ранний выход `:223`
   обязан учитывать наличие правил `kind=node` (CODEMAP §10 п.26).
   Резолву нужен доступ к `Cache.NodeSections` — пробросить через
   `PresetMergeContext` (`preset_merge.go:155-199`).
5. **Санитайзер.** `SanitizeDNSDetours` (`core/build/dns_detour_sanitize.go:48`)
   читает только `detour` (`:56-77`). Добавить ребро `dns.servers[].endpoint`:
   висячая ссылка → **сервер выбрасывается целиком** с WarnLog (снятие
   ключа здесь невозможно — сервер без endpoint'а невалиден), после чего
   DNS-правила, ссылавшиеся на выброшенный сервер, чинятся
   `repairDanglingDNSRefs`. Внимание на порядок: сегодня repair идёт внутри
   `MergePresetsIntoDNS` (`:402`), а санитайзер — после него
   (`build.go:322`); выброс сервера санитайзером обязан сопровождаться
   повторной починкой ссылок в той же точке.
   `route.rules[].outbound` покрыт `CleanDanglingOutboundsInRouteRules`
   (`build.go:342`, политика fallback) — без изменений.
6. **Пересев якорей.** `NormalizeRuleOrder` (`core/state/rule_order.go:210`)
   получает шаг `SeedNodeRules(rules, links []NodeLink)`: для каждого узла
   `kind=server` с непустыми `Sections.Rules` — запись `kind=node`, если
   её нет, с `OrderNum = NodeRuleDefaultNum`; записи `kind=node`, у которых
   узла нет или `Sections.Rules` пуст, удаляются. Список `links` собирает
   вызывающая сторона из `state.Sources` (корень + папки). Вызовы:
   `core/build/resolve_route.go:150-154` (сборка) и UI-модель при загрузке
   и при любой правке узла (§5). `NodeRuleDefaultNum = 945`: перед
   якорем `private-ips` (950) — подсеть tailnet и подсети за пиром должны
   матчиться раньше общих правил; пользователь двигает дальше сам.
7. **Инвариант.** Состояние без единого узла с секциями даёт
   **байт-в-байт** тот же конфиг, что сегодня. Это охраняет
   `TestGoldenScenarios` (`core/build/golden_test.go:31`) — эталон
   `real-v088` не пересчитывать.

## 5. UI

### 5.1 Вкладка JSON узла (`ui/configurator/tabs/source_edit_window.go:1840-2060`)

Сегодня вкладка принимает объект с непустым `type` (`:1893-1904`). Она
начинает принимать **две формы**:

- прежнюю — объект outbound/endpoint (тело узла, секции не трогаются);
- **документ** — объект без `type`, но с ключами из набора
  `outbounds`, `endpoints`, `dns`, `route`:
  - ровно одна запись суммарно в `outbounds[]`+`endpoints[]` → тело узла;
    ноль или больше одной → ошибка «document must carry exactly one node»;
  - `dns.servers[]` → `sections.dns_servers`, `dns.rules[]` →
    `sections.dns_rules`, `route.rules[]` → `sections.rules`; остальные
    ключи внутри `dns`/`route` (`final`, `strategy`,
    `default_domain_resolver`, …) отвергаются с перечислением;
  - любой другой верхний ключ (`log`, `inbounds`, `experimental`, …)
    отвергается с перечислением — документ не «конфиг целиком», а
    фрагмент узла;
  - ссылки на тег узла внутри секций: значение, равное тегу записи из
    `outbounds`/`endpoints`, переписывается в `@self` (пользователь
    вставил готовый конфиг с реальным тегом — связка должна пережить
    переименование узла);
  - правило маршрута без `outbound`/`action` получает `"outbound": "@self"`;
  - правило с `rule_set` → ошибка; `@var`, кроме `@self`, → ошибка.

Отрисовка (`:2056-2060`): узел без секций показывается как раньше (голое
тело); узел с секциями — документом `{"outbounds"|"endpoints": [тело],
"dns": {...}, "route": {...}}` с `@self` как есть. Секция `endpoints`
выбирается по той же схеме, что при эмиссии (`outbound_generator.go:1086`).
Грязный флаг (`:2126`) и откат при ошибке — без изменений. Для узлов
подписки вкладка остаётся read-only.

Распаковка состава источника (`source_edit_json.go:108-114`,
`unpackedDoc{Outbounds, Endpoints}`) секции не показывает — это превью
состава, не редактор; без изменений.

### 5.2 Список правил (`ui/configurator/tabs/rules_unified_rows.go`)

Новый вид слота — якорь узла. Модель: `NodeRefState{FolderID, Tag,
Enabled, OrderNum}` рядом с `PresetRefState`
(`ui/configurator/models/preset_ref_state.go`), слот `SlotKindNodeRef` в
`rule_slot.go`, синхронизация с `state.Rule kind=node` по образцу
`preset_ref_sync.go:263`. Строка — по образцу `buildSinglePresetRefRow`
(`:72-349`):

- подпись `🔗 <тег узла>` — глиф тот же, что у пресетных якорей, новых
  глифов не вводить; tooltip: `Route rules carried by node "<tag>" (N)`;
- тумблер — рабочий (`Enabled` записи), ручка перетаскивания — есть,
  шестерёнки и удаления — нет (правится узел, `buildRowEditDelCluster(nil, nil)`);
- узел выключен → строка приглушена, tooltip дополняется `node is disabled`;
- клик по подписи = тоггл, как у остальных.

Ось: `NodeRuleDefaultNum` при первом появлении, дальше — `PlaceRuleAfter`
как у всех сортируемых. Пересев (`SeedNodeRules`) — при загрузке модели и
после каждого сохранения узла в редакторе источников; удаление узла или
его `rules` убирает якорь тем же пересевом.

`GetAvailableOutbounds` (`ui/configurator/business/outbound.go:69`) не
меняется: цели внутри секций — `@self`, чужим правилам тег узла в этой
задаче не предлагается.

### 5.3 Вкладка DNS (`ui/configurator/tabs/dns_tab.go`)

Серверы узлов показываются в конце списка серверов read-only, как bundled
серверы пресетов (`:89-91`, `:211-213`), подпись `🔗 <тег узла>:<локальный
тег>`; правила узлов — в конце объединённого списка правил read-only по
образцу пресетной строки (`dns_unified_rules.go:145-236`), только View
JSON. Порядка у них нет, тумблера нет (управляет узел).

## 6. Импорт из тела источника (вторая фаза)

Пользователь вставляет как источник **целый** sing-box конфиг с одним
узлом и его связкой (ровно тот JSON, что приходит в issue). Сегодня
`route`/`dns` выбрасываются (`singbox_import.go:27`, `:134-149`).

Правило извлечения — только когда в `outbounds[]`+`endpoints[]` конфига
**ровно одна** запись, не являющаяся группой (`IsSingboxGroupType`) и не
служебная (`IsSingboxServiceType`), и она принята парсером как `server`:

- `dns.servers[]`, у которых `detour` или `endpoint` равен тегу узла →
  `sections.dns_servers` (ссылка → `@self`);
- `dns.rules[]`, у которых `server` равен тегу одного из взятых серверов →
  `sections.dns_rules`;
- `route.rules[]`, у которых `outbound` равен тегу узла → `sections.rules`
  (`@self`); правило с `rule_set` пропускается с warning.

Всё остальное в `dns`/`route` игнорируется как сегодня. В
`SingboxImportResult` появляется счётчик извлечённых фрагментов; лог —
InfoLog. Узел `kind=unsupported` секций не получает.

Извлечение — чистая функция в `core/config/subscription/`
(`singbox_sections_extract.go`), без сети и состояния (инвариант парсера,
CONSTITUTION §архитектура).

## 7. Что не входит

- Сам Tailscale: схема `tailscale` в `singboxSchemeByType`, безадресность,
  гейт endpoint'а (`outbound_generator.go:1086`, `:1054`), проба
  `with_tailscale` и деградация с warning, тип `tailscale` в форме
  DNS-сервера, исключение из селекторов Направлений — **SPEC 122**. Эта
  задача проверяется на любом принятом узле (WireGuard-endpoint, обычный
  outbound).
- Несколько якорей на один узел, `rule_set` в секциях, переменные кроме
  `@self`, секции у подписок/цепочек/Auto.
- Ось порядка для DNS.
- Тег узла как цель для чужих правил.

## 8. Приёмка

1. Узел `kind=server` в корне с секциями из §2 (тело — `wireguard` или
   `vless`) → в `config.json`: `dns.servers` содержит сервер с тегом
   `<тег>:ts-dns` и `endpoint: <тег>`; `dns.rules` — правило с
   `server: <тег>:ts-dns`; `route.rules` — правило с `outbound: <тег>` на
   позиции якоря (по умолчанию перед `private-ips`); `outbounds`/`endpoints`
   — без изменений против узла без секций.
2. Тот же узел в папке с TagPolicy-префиксом → все подстановки и префиксы
   используют **финальный** тег.
3. Узел выключен → ни одного фрагмента в конфиге; якорь в списке остаётся,
   приглушён. Узел удалён → якоря нет.
4. Состояние без секций → конфиг байт-в-байт как до задачи (golden).
5. Бэкап: экспорт пишет `servers[].sections` с `rule_num`, в `rules[]`
   записи `kind=node` нет; импорт в пустое состояние восстанавливает
   секции и позицию якоря; импорт поверх состояния с тем же узлом —
   секции файла замещают локальные; файл без `sections` локальные не
   трогает; `scanUnknown` на новое поле не ругается.
6. Вкладка JSON: вставка документа с одним endpoint'ом, `dns` и `route` →
   узел с секциями, реальный тег в ссылках заменён на `@self`, правило без
   `outbound` получило `@self`; вставка документа с двумя узлами или с
   `inbounds` → ошибка с перечислением, узел не тронут.
7. Источник с целым конфигом из одного узла (§6) → узел с секциями; с
   двумя узлами → секции игнорируются как сегодня.
8. Санитайзер: `dns.servers[].endpoint` на несуществующий тег → сервер
   выброшен, правило на него починено, WarnLog назван.
9. Тесты — data-критичные интеграционные, в конце: сборка (п.1–4 одним
   тестом на таблице сценариев), бэкап round-trip (п.5), извлечение из
   тела (п.7). Юнитов на UI и форматирование не писать.
   `go build ./... && go vet ./... && go test ./...` зелёные.

## 9. Синхронизация документов

- `SPECS/features/sources.md`: в §Дерево источников у Server — абзац про
  `sections`; в §Свобода — секции только у свободных узлов.
- `contract/docs/BACKUP.md`: строка в таблице `servers[]`, абзац в §9 про
  замещение секций; `contract/schema/backup.schema.json`.
- `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md`: D-098 (черновик до
  ответа LxBox). `contract/TASKS_LXBOX.md`: `## 9`.
- `docs/release_notes/upcoming.md`; `docs/ARCHITECTURE.md`, если там описан
  конвейер пресетов.
- `CODEMAP.md` этой папки — правки исполнителя по мере работы.

## 10. Пересмотр модели (волна 3, решение 2026-09-05)

> **Форма записей изменена state v8 — SPEC 127 §2 (волна 1, 14.09.2026).**
> Модель раздела в силе (секции = фрагмент состояния, сборка их дописывает),
> но примеры JSON ниже показывают форму v7 и устарели: у правила `order_num`
> стал `num`, `name` и `refs[]` поднялись из тела полями записи, а `body` —
> правило sing-box целиком (матчеры и цель, без обёртки `match`); у DNS-сервера
> тело лежит в `body`, тег — в поле `tag`. Действующая форма — SPEC 127 §2 и
> `contract/docs/ONE_NAMESPACE.md` §1–§2.

Первые две волны хранили секции как сырые фрагменты sing-box JSON с
неявной префиксацией тегов и отдельным якорем `kind=node` в `rules[]`.
Решение владельца: **секции узла — фрагмент состояния в формате самого
лаунчера**, а сборка их не «разворачивает», а **дописывает** к основным
спискам перед обычным резолвом. Ссылки на узел из состояния исчезают: у
записи узла нечего адресовать, она сама знает, где стоит.

### 10.1 Хранение

```json
"sections": {
  "rules": [
    { "kind": "inline", "enabled": true, "order_num": 945,
      "body": { "name": "@{self} network", "match": { "ip_cidr": ["100.64.0.0/10"] }, "outbound": "@self" } }
  ],
  "dns": {
    "servers": [ { "kind": "user", "enabled": true, "tag": "@{self}-dns", "type": "tailscale", "endpoint": "@self" } ],
    "rules":   [ { "kind": "user", "enabled": true, "domain_suffix": [".ts.net"], "server": "@{self}-dns" } ]
  }
}
```

- `sections.rules[]` — записи `state.Rule` видов `inline` | `srs` со своими
  `enabled` и `order_num`. Вид `preset` и вид `node` в секциях недопустимы:
  запись отбрасывается при чтении с WarnLog. **`RuleKindNode` упраздняется**
  вместе с `NodeRuleBody`, `SeedNodeRules` и параметром `nodeLinks` у
  `NormalizeRuleOrder`.
- `sections.dns.servers[]` / `sections.dns.rules[]` — записи `state.DNSServer`
  / `state.DNSRule` вида `user` (сериализация та же, что у корневых: `kind`,
  `enabled`, поля тела плоско). Другие виды отбрасываются с WarnLog.
- Плейсхолдер: `@self` **целой строкой** и `@{self}` **внутри строки**
  (`@{self}-dns`, `@{self}_dns`, `@{self} network`). Подставляется во все
  строковые значения записей секции, включая `tag`, `name`, `server`,
  `outbound`, `endpoint`, `detour`. Неявной префиксации тегов и переписи
  `server` по локальному тегу **больше нет**: что написано, то и уходит.
  Реализация — свой обход JSON-строк (`core/config/selfvar.go`,
  `SubstituteSelf(raw []byte, finalTag string) []byte`), не пресетный
  движок `@var`.
- Несколько правил у узла — несколько записей, каждая со своей позицией на
  оси.

### 10.2 Сборка = инъекция

`ParsedCache.NodeSections[i]` несёт `FinalTag`, `Link` и записи секции как
есть. В точке, где для резолва строится временное `state.State{Rules, DNS}`
(`core/build/preset_merge.go`, `MergePresetsIntoRoute`/`MergePresetsIntoDNS`,
`CollectEmittedRouteRuleSetTags`), записи узлов после `SubstituteSelf`
**конкатенируются**: `Rules = state.Rules ++ узловые`, затем
`SortRulesByNum`; `DNS.Servers = … ++ узловые`, `DNS.Rules = … ++ узловые`
(после пользовательских, оси у DNS нет). Дальше работает существующий
конвейер без узловых веток: резолв inline/srs, `enabled`, dedup DNS-тегов,
`repairDanglingDNSRefs`, `SanitizeDNSDetours` с ребром `endpoint` (волна 1
сохраняется), `CleanDanglingOutboundsInRouteRules`. Файл
`node_sections_expand.go` и `resolveNodeRouteRule` упраздняются.
Инвариант §4 п.7 (байт-в-байт без секций, golden без пересчёта) остаётся.

### 10.3 Старая форма волн 1–2

Старая форма волн 1–2 в релизы не попала, конвертера нет (решение владельца
2026-09-05).

### 10.4 UI

- **Rules.** По строке на каждую запись `sections.rules[]`: подпись
  `🔗 <name> · <тег узла>`, tooltip называет узел; тумблер пишет `enabled`
  записи, перетаскивание — `order_num` (та же ось, тот же `PlaceRuleAfter`,
  ленивый сдвиг соседей пишется тем же проходом); редактирования и удаления
  нет. Обратный указатель строки — в памяти модели:
  `NodeRuleRef{Link state.NodeLink, Index int}` вместо `NodeRefState`.
  Синхронизация при Save раскладывает `order_num`/`enabled` по домам:
  корневые в `rules[]`, узловые в `sections.rules[Index]` своего узла.
- **DNS.** Read-only показ серверов и правил узлов остаётся, подписи — после
  `SubstituteSelf` финальным тегом.
- **Вкладка JSON узла.** Показывает `{ "endpoints"|"outbounds": [тело],
  "sections": { …хранимая форма… } }`. Принимает три входа: голый объект с
  `type` (тело, секции не трогаются); документ с `sections` в хранимой
  форме; sing-box-документ с `dns`/`route` (волна 2), который переводится в
  хранимую форму конвертером sing-box-документа (правило → `inline`,
  сервер/правило → `user`). `ParseNodeDocument` возвращает `*state.NodeSections` новой формы.
- **Перерисовка.** Rules и DNS перестраиваются при заходе на вкладку
  (`tabs.OnSelected` в `ui/configurator/configurator.go:628`), если
  `model.Revision` изменилась с последней отрисовки вкладки; правка узла
  бампает ревизию через существующую инвалидацию пула.
- **Конструктор Tailscale и извлечение из целого конфига** отдают/получают
  sing-box-документ, как сейчас; в хранимую форму их переводит тот же
  конвертер — второй реализации правил перевода быть не должно.

### 10.5 Бэкап и контракт

`servers[].sections` — объект §10.1 без переименований.
`contract/schema/backup.schema.json`, `contract/docs/BACKUP.md`
(строка таблицы, §9 — «файл замещает локальные» сохраняется),
`SPECS/features/sources.md`, `contract/TASKS_LXBOX.md` `## 9` — переписать
под новую форму. В `DECISIONS.md` — новая запись D-099, уточняющая D-098
(решения не переписываются). `contract/VERSION` не поднимать.

### 10.6 Что обязано сохраниться (чеклист «ничего не потерять»)

Все сценарии приёмки §8 в новой форме; конструктор «Add server → Tailscale»
и папочный путь; извлечение связки из целого конфига (SPEC 122 §4 п.1);
гейт ядра, `IsExitCapable`, форма DNS-сервера типа `tailscale`,
`state_directory` только в конфиг; ребро `endpoint` в санитайзере;
тесты волн 1–2 и SPEC 122 переписаны на новую форму,
ни один сценарий не удалён; golden `real-v088` без пересчёта.
