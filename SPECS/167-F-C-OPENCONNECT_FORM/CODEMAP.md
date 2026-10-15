# CODEMAP 167 — форма OpenConnect: wildcard `*`, `form`, роль `variant`, второй фактор и SSO

Карта для исполнителей SPEC 167. Строки — на коммит `50deef63` (develop, контракт 1.1.121).
Норма примитивов — `REGISTRY_DESIGN.md`. Здесь только якоря: «как сейчас» и «куда встраивать».

## 0. Что важно знать до начала

- **Формы Add server на реестре нет.** Все формы (SOCKS/HTTP/WireGuard/Tailscale) написаны руками в `ui/configurator/dialogs/add_server_dialog.go` (выбор схемы — `widget.NewSelect`). Блоки правки узла (AWG, Tailscale) — тоже руками, в `ui/configurator/tabs/`. Каждый со своей маленькой моделью. Форма openconnect будет первой, которая раскладывается по реестру (`reg.Form`).
- **Форма Add server живёт в `dialogs/`, а не в `tabs/`.** PLAN называет `tabs/source_openconnect_edit.go`. Конструктор Add server встраивается в `add_server_dialog.go` (новый режим) по образцу `add_server_tailscale.go`. Правка узла — блок в `source_edit_window.go` по образцу `source_tailscale_edit.go`.
- **Формы не запускают `sing-box check`.** До сохранения они проверяют только санитайзером: `nodeflow.Sanitize` → `nodewarn.Summary`. Цикл отказа ядра (core-reject) идёт на сборке или Save конфигуратора.
- **Подсказки и предупреждения UI сейчас не читают `Field.Advisory` и `Field.Secret`.** Коды доходят до UI только как предупреждения санитайзера.
- **UI статуса Tailscale опрашивает кэш, а не получает событие.** Хук `TailscaleStatusCache.OnSnapshot` никем не вызывается. Диалог челленджа так не сделать: нужен свой push или поллинг.
- **Временного loopback-слушателя в коде нет.** Единственный `net.Listen` — debug API.

## 1. Реестр (`core/config/registry/registry.go`, 2010 строк)

### 1.1 Типы

| Что | Где | Как сейчас / куда встраивать |
|---|---|---|
| `Field` | `:25`–`:247` | Поле тела. Неизвестные атрибуты молча игнорируются (`:23-24`). Ключевые поля: `Type`/`Ref`/`Inline` `:26-28`, `Items`/`Order`/`Fields` `:31-33`, `Values` `:36`, `Pattern` `:51`, `ItemPattern` `:65`, `OnItemInvalid` `:69`, `Min`/`Max` `:112-113`, `Required` `:125`, `RequiredUnless` `:128`, `Secret` `:129`, `Managed` `:132`, `Default` `:135`, `OnInvalid` `:136`, `Advisory` `:137`, `Role` `:145`, `CoreRejects`/`Conflicts`/`Requires` `:209-211`, `Level` `:230`, `DescEn`/`DescRu`/`Impl` `:234-236`, `Variants`/`Discriminator` `:240-241` (`json:"-"`). **Сюда:** `Wildcard bool` (`json:"-"`, ставится разбором ключа `"*"`) и `Form *FieldForm` (`json:"form"`). |
| `section` | `:524`–`:561` | Секция на диске. `FieldsUnchecked` `:538-541` — **удалить**. **Сюда:** `Form *FormSection` (`json:"form"`). |
| `BodySchema` | `:628`–`:647` | Разрешённая схема. `FieldsUnchecked` `:642-643` — **удалить**. **Сюда:** `Form *FormSection`. |
| `Condition` | `:435`–`:441`, `UnmarshalJSON` `:446` | Ключи `any_set`, `source_kind`; остальные ключи → `Values` (путь → скаляр или `{in:[…]}`/`{not_in:[…]}`). Это и есть грамматика `visible_when`, без `source_kind`. |
| `RequiredUnless` | `:486` | `{set, absent, code}`. Та же форма у `form.required_unless`. |
| `Relation` | `:493`–`:521` | `with`/`path`/`equals`/`unless_set` `:506`/`when` `:511`/`set` `:517`/`code`/`core_rejects`. |
| `Advisory` | `:318` | `{values, except, when, code}`. Источник предупреждений формы. |
| Роли | `:1613`–`:1620` | `RoleCredential`, `RolePrivateKey`. **Сюда:** `RoleVariant = "variant"`. |
| `protocolFiles` | `:711`–`:715` | Явный список схем. `openconnect` уже есть (1.1.120). |

### 1.2 Загрузка и разрешение

| Функция | Где | Как сейчас / куда |
|---|---|---|
| `Get` / `MustGet` / `Load` | `:725` / `:732` / `:741` | `Load` читает суб-схемы `tls/transports/multiplex/dialer` (`:743-754`), затем идёт по `protocolFiles` (`:770`). Для каждого файла: `readFile` → `resolveSection` → `bodies[scheme]`, `kinds`, `sources`, `singboxTypes`/`schemeByType`, алиасы (имя файла, `singbox_type`). |
| `resolveSection(scheme, sec, subs)` | `:901`–`:962` | Копирует атрибуты секции (`:903-915`); `FieldsUnchecked: sec.FieldsUnchecked` на `:913` — снять. Цикл по `sec.Order` (`:916`) падает, если имени из `order` нет в `fields` (`:918-920`), поэтому `"*"` обязан лежать и в `fields`. Поле не-ref идёт как есть (`:921-925`): **здесь** выставлять `Wildcard` у записи `"*"` (у неё `Type == ""`). Inline-ref (`__dialer`) вливает `sub.Order` плоско (`:931-944`), дубли пропускает. **Сюда:** `"*"` суб-схемы не вливать (сейчас в dialer его нет — это защита на будущее). Вложенные объекты протокола не разрешаются: `"*"` внутри `tls.fields` остаётся записью `fields["*"]`. Флаг на всех уровнях ставить рекурсивным проходом по `Fields`/`Items`. |
| `refAsObject` / `resolveNamedRef` / `mergeRefAttrs` | `:966` / `:1015` / `:1041` | Ссылка → объект (`Order`/`Fields` суб-схемы) или вариант (транспорты). У openconnect `tls` — свой объект, а не ref на `tls.json`. |

### 1.3 Доступ

| Функция | Где | Как сейчас / куда |
|---|---|---|
| `Body(scheme)` | `:1227` | Схема по имени, алиасу или типу ядра. |
| `SchemeForSingboxType(t)` | `:1241` | Тип ядра → схема (lower + trim). |
| `NodeSchemeForSingboxType(t)` | `:1254` | То же, но только для схем с входом `singbox` (`AcceptsSource` `:1263`). У openconnect `sources: ["singbox"]`. |
| `ProtocolKind(scheme)` | `:1277` | `outbound`/`endpoint`/`group`; константа `KindEndpoint` на `:1273`. Потребитель — `core/config/endpoint_schemes.go:20` `IsEndpointScheme`. |
| `Order(scheme)` | `:1289` | Копия `Order`. Единственный потребитель — `linkmap/plan.go:169` → `PlanSet.BodyOrder` (`plan.go:66`), а его никто не читает. `*` в `Order` не вредит. |
| `Field(scheme, path)` | `:1302`–`:1330` | Путь по `Fields`/`Variants`/`Items.Fields`. **Сюда:** на сегменте `"*"` возвращать `false` (wildcard — не поле). |
| `FieldAllowedOn` | `:1367` | Читает `Conflicts` по готовому телу через `HoldsOn`/`bodyValue`. |
| `WalkPresent` / `walkPresent` | `:1466` / `:1474` | Обход заданных полей по `order`. Ключа `"*"` в теле нет, поэтому запись пропускается сама; явный `continue` по `Wildcard` надёжнее. Потребители: `Level` `:1542`, `YieldsTo` `:1419`, `gate.go:65`. |
| `Level(scheme, body)` | `:1526` | Подпись уровня (`levels`/`level`/`level_mark`). У openconnect `levels` нет → `""`. |
| `FieldWithRole(scheme, role)` | `:1625`–`:1638` | Ищет только на верхнем уровне `b.Order`, берёт первое совпадение. Для `variant` годится как есть. Сейчас потребитель один — `core/config/subscription/share_uri_secret.go:37`; в `ui/` не вызывается. |
| `ExitCapable` | `:1645` | Через `ExitCapableWhen.HoldsOn`. |
| `(*Condition).HoldsOn(body)` | `:1661`–`:1696` | Предикаты `Values`: `in` ложно при отсутствии пути; `not_in` истинно при отсутствии; скаляр — равенство печатных форм, отсутствие = ложь. Затем `any_set`. **`default` поля не подставляется:** `visible_when {"flavor":"fortinet"}` на теле без `flavor` ложно (и это верно: дефолт anyconnect), а `{"flavor":{"not_in":["fortinet"]}}` истинно. Источник истины для видимости в форме. |
| `bodyValue` | `:1699` | Идёт по пути через карты; nil и пустая строка = «не задано». |
| `FieldStrings(scheme, path)` | `:1732` | `values` строками (для `widget.Select`). |
| `Warning` / `WarningText` / `WarningAdvice` | `:1747` / `:1754` / `:1771` | Тексты кодов (для `advisory` в форме). |
| `RuleCoreRejects` / `fieldsAtBodyPath` | `:1874` / `:1913`–`:1940` | Жёсткость кода по пути; `fieldsAtBodyPath` берёт `fields[parts[0]]`. **Сюда:** сегмент `"*"` не находить. |
| **Новое:** `Form(scheme) *FormSection` | — | Положить рядом с `FieldWithRole`. Типы: `FieldForm{Block; VisibleWhen *Condition; Required bool; RequiredUnless *RequiredUnless; Min, Max *float64}`, `FormSection{Blocks []string; Choices []FormChoice}`, `FormChoice{ID, Block; Options []FormOption}`, `FormOption{ID; Paths []string; Set map[string]interface{}}`. |

### 1.4 Схема `contract/schema/registry_body.schema.json` (738 строк)

| Определение | Строки | Заметка |
|---|---|---|
| `relation` (`with`/`path`/`equals`/`when`/`unless_set`/`set`/`code`/`core_rejects`) | `:59`–`:97` | `unless_set` на `:80`. |
| `requiredUnless` | `:111`–`:120` | — |
| `condition` | `:208`–`:244` | `any_set` `:212`, `source_kind` `:217`, предикаты путей в `patternProperties` `:224`, `additionalProperties:false`. `visible_when` — `$ref` сюда. |
| `field` | `:358`–`:627` | `order` `:375`; `fields` `:380` (`additionalProperties: field` `:383` — **сюда** `patternProperties "^\\*$": wildcardField`); `item_pattern` `:402`; `on_item_invalid` `:406`; `min`/`max` `:431-432`; `required_unless` `:454`; `required` `:455`; `secret` `:459`; `role` `:463` (enum `credential`/`private_key` — **добавить** `variant`); `advisory` `:491`; `conflicts` `:577`; `requires` `:582`; `core_rejects` `:361`. `required: ["type"]` на `:595` — поэтому `"*"` нужен отдельный `wildcardField`. `additionalProperties:false` на `:596`. **Сюда:** `form` → `fieldForm`. |
| `bodySection` | `:629`–`:684` | `fields` `:642` (так же `patternProperties`); `fields_unchecked` `:672` — **удалить**; **сюда** `form` → `formSection`. `additionalProperties:false` на `:683`. |
| `variantsSection` | `:685`–`:722` | `fields` вариантов `:706` — `"*"` там не нужен. |

Схема описательная: валидатора JSON Schema в CI нет. Фактический линтер — `TestRegistryBodyStructure` (§3.1).

## 2. Санитайзер и эмиттер (`core/config/nodeflow/`)

### 2.1 `sanitize.go`

| Что | Где | Как сейчас / куда |
|---|---|---|
| `buildManagedKeys` | `:64` | `tag`, `type` — в корне снимаются молча. |
| `type Result` | `:30`–`:53` | `Clean, Warnings, Drop, Implied, Repaired, RepairWarnings, HardPaths`. |
| `type sanitizer` | `:80`–`:155` | `srcRoot` `:100` (исходная карта целиком), `cleanRoot` `:101`, `building` `:112`, `removed` `:121`, `absent` `:128`, `dropped` `:140`. |
| `Sanitize` / `SanitizeFrom` / `SanitizeFromKind` | `:171` / `:185` / `:200` | **Ветку `FieldsUnchecked` `:218`–`:230` удалить** (она копирует все ключи, кроме `buildManagedKeys`, через `deepCopyValue`). Дальше по порядку: `markSchemeForbidden` `:241` → `markAbsentObjects` `:247` → `object` `:248` → `relations` `:252` → отказ по `missingRequired` → `applyDeferred`. |
| `markSchemeForbidden` | `:284` | Цикл по `order` и `fields[name]`, рекурсия через `emitShape`. **Пропускать `*`.** |
| `markAbsentObjects` | `:331` | То же; `Managed` → `absent`. **Пропускать `*`.** |
| `object(prefix, order, fields, src)` | `:570`–`:707` | `known` строится из ключей `fields` (`:582`). Цикл `order` (`:587`): `Managed` → пропуск; затем `absent`, `omitAsUnset`, `allowedForScheme`; при отсутствии — `default_when`/`required`; иначе `value()`. Хвост `:698`–`:705`: всё вне `known` → `warn("unknown_key", …)`. **Сюда:** в цикле `order` — `continue` на `Wildcard`; в хвосте, если есть `fields["*"]`, вместо `unknown_key` писать `out[name] = deepCopyValue(src[name])`. В корне `buildManagedKeys` по-прежнему пропускаются. `"*"` попадёт в `known`, но ключа `"*"` у ядра нет. |
| `minWhenAbsent` | `:1012` | Цикл по `order`. У `*` `MinWhen == nil`, поэтому пропуск идёт сам, но по норме нужен явный. |
| `objectField` | `:1471`–`:1505` | При `len(f.Fields)==0` — свободная карта как есть (`:1479`). С `fields: {"*":…}` объект пойдёт через `object()`; если результат пуст, объект снимается (`:1499`). |
| `variantObject` / `arrayField` | `:1508` / `:1538` | Вызывают `object()` для вариантов и `items`. |
| `pathPresent` → `lookupNonEmpty` → `lookupClean` / `lookupPath(srcRoot)` | `:1667` / `:1732` / `:1720` / `:1778` | Связи ищут путь в чистой карте, потом в исходной; `gone()` (`:1759`) учитывает `removed`/`absent`/`dropped`. Поэтому `unless_set: ["tls.client_key"]` работает и на неописанном пути. |
| `advisory` / `advisoryValueMatches` | `:1805` / `:1829` | Код без правки значения. Форме нужен тот же матч: экспортировать или повторить через реестр. |
| `deepCopyMap` / `deepCopyValue` | `:2519` / `:2527` | Копия карт и срезов. |
| `sortedKeys` | `:2081` | Пригодится эмиттеру для слота `*`. |
| `Repairs` | `:2551` | Починки на замороженном теле. |

### 2.2 `emit.go`

| Что | Где | Как сейчас / куда |
|---|---|---|
| `Emit(scheme, clean)` | `:20` | **Ветку `FieldsUnchecked` `:29`–`:39` удалить** (`writeJSON` всей карты = ключи по алфавиту). Дальше `emitObject(body.Order, body.Fields)`. |
| `emitObject` | `:48`–`:71` | Пишет только ключи из `order`, которые есть в карте. **Сюда:** на позиции `"*"` писать ключи карты, которых нет в `fields`, по алфавиту, каждый через `writeString` + `emitValue(nil, v)`/`writeJSON`. |
| `emitValue` | `:73` | Объект → `emitShape`; `nil`-order → `writeJSON` как есть. |
| `emitShape` | `:115`–`:137` | Вариант по дискриминатору; `len(f.Fields)==0` → свободная карта. Используется ещё в `sanitize.go` (`markSchemeForbidden`, `markAbsentObjects`) и в `gate.go:217`. |
| `writeJSON` | `:142` | `SetEscapeHTML(false)`, без перевода строки; карта кодируется по алфавиту. |

### 2.3 `gate.go`, `coerce.go`, `authored.go`

| Что | Где | Заметка |
|---|---|---|
| `NodeCoreRefusal(scheme, body, core)` | `gate.go:50` | Узловой гейт: `OnCoreUnsupported` + `BuildTag`/`MinCore` схемы (`dropsNode` `:85`, `unmet` `:91`), затем поля через `reg.WalkPresent` (`:65`). openconnect без `with_openconnect` → `openconnect_core_unsupported`. |
| `GateForCore` / `gate.object` | `gate.go:121` / `:197` | Обход `order`/`fields` (`:139`). **Пропускать `*`.** |
| `coerce(f, raw)` | `coerce.go:20` | Приведение по `f.Type`. `Fields` не обходит: `*` сюда не попадёт, если его пропускает `object()`. |
| `authored.go` | — | `Fields` не обходит (авторское тело — PARSING_PRINCIPLES §10, работает без изменений). |

### 2.4 `fields_unchecked_test.go`

`TestFieldsUncheckedOpenVPNClientPassesAsIs` `:10` (sanitize + emit, ожидаемые байты `:36`) и `TestFieldsUncheckedOpenVPNClientCoreGate` `:47`. По PLAN — заменить на `TestWildcardNestedStrict` (REGISTRY_DESIGN §6), запуск по имени. Ожидание байтов `:36` стоит сохранить: оно подтверждает эквивалентность `order: ["*"]`.

### 2.5 Где ещё обходятся `Fields`

| Файл | Строки | Что |
|---|---|---|
| `core/config/registry/registry.go` | `:1308-1325` (`Field`), `:1454` (`FieldsWithBuildTag`), `:1474-1515` (`walkPresent`), `:1579-1606` (`collapseRangeForms`), `:1631` (`FieldWithRole`), `:1913-1936` (`fieldsAtBodyPath`) | пропуск `*` |
| `core/config/node_materialize.go` | `:238` `schemeHasRootAddress` (`body.Fields["server"]`) | Когда у openconnect появится `server`, функция вернёт `true`, и `outboundMapOf` (`:205`) начнёт дописывать `server`/`server_port` из `ParsedNode` (см. риски, §7) |
| `core/config/configtypes/types.go` | `:1123`, `:1139` | каталог strip цепочки — не затрагивается |
| `core/config/linkmap/*` | — | `Fields` не обходит. Только `reg.Order` в `plan.go:169` и `reg.Warning` в `exec.go:203` |
| `contract/tools/gendocs/scheme.go` | `renderBodyList` `:365` (цикл `body.Order` `:380`), `writeBodyItems` `:405`, `writeChildren` `:469`, `writeNested` `:484` (`:486-500`), `refPage` `:750` | **сюда:** строка «прочие ключи — насквозь» на `*`, вывод `form`, роль `variant` |
| `contract/tools/gendocs/sections.go` | `renderSchemeWarnings` `:125` (`:134`), `collectNestedUsages` `:267` (`:269-283`), `collectReplacements` `:383` (`:445-455`), `renderDegradation` `:466` (`:475`), `collectDegradationNested` `:589` (`:591-605`); `conditionPhrase` `:24` (пригодится для `visible_when`) | пропуск `*` |
| `contract/tools/gendocs/warnings.go` | `walkSection` `:219` (`:222-228`), `walkField` `:242` (`:307-319`) | пропуск `*` |
| `contract/tools/gendocs/subschema.go` | `:114`, `:143-152`, `writeSection` `:175` | суб-схемы; `*` там нет |
| `contract/tools/gendocs/linkparams.go` | `:64`, `:217`, `:226` | точечный доступ по имени |

Генерация: `contract/doc.go:11` `//go:generate go run ./tools/gendocs` пишет в `contract/docs/generated/`; CI сверяет `git diff --exit-code`. Страница схемы — `contract/docs/generated/protocols/openconnect.md`.

## 3. Тесты и контракт

### 3.1 `core/config/registry_body_test.go`

| Что | Где | Куда |
|---|---|---|
| Свои структуры разбора: `bodyField` (`Role` `:251`), `bodyCondition` (`UnmarshalJSON` `:313`), `bodySection` `:412`–`:428`, `registryBodyFile` `:430` | — | `bodySection` не знает `fields_unchecked` и `form`. `readRegistryJSON` (`:435`) вызывает `json.Unmarshal` без `DisallowUnknownFields`, поэтому лишние ключи молча теряются. Для проверок `form` поля надо добавить. |
| `registryProtocolSchemes` | `:466`–`:469` | Явный список схем (openconnect в нём есть). |
| `walkFields` | `:505`–`:526` | **Пропускать `*`**, иначе `checkField` споткнётся о запись без `type`. |
| `checkOrderCoversFields` | `:529`–`:565` | Проверяет `order` ↔ `fields` в обе стороны, рекурсивно: `"*"` обязан быть и там, и там. |
| `TestRegistryBodyStructure` | `:568`–`:632` | Для каждой секции: `core` не пуст, `checkOrderCoversFields`, `walkFields` → `checkField` (`:820`; первая проверка — `registryFieldTypes[f.Type]` на `:823`), `checkBodyRelations` (`:759`); затем `checkFieldRoles` и `checkCoreGates` (`:635`). **Сюда:** `*` только с desc/impl и обязательно в `order`; `*` в путях связей и условий запрещён; `fields_unchecked` запрещён; ключи `form` валидны, пути `visible_when`/`choices` описаны, `block` ∈ `blocks`. |
| `registryFieldRoles` / `checkFieldRoles` | `:710` / `:716`–`:757` | Словарь `credential`/`private_key`; тип поля с ролью — только `string`/`listable_string` (`:737`). **Добавить** `variant` и разрешить для него `enum`: у `flavor` тип `enum`, и без правки тест упадёт. |
| `TestRegistryBodyCoversCoreSchema` | `:1239`–`:1347` | Сверка с `SPECS/131-…/core_schema.draft.json` (`:39`). Список схем `schemeFile` (`:1299`–`:1305`) явный; openconnect и openvpn-client в нём нет, сверка их не касается. По REGISTRY_DESIGN: для тел с `*` проверять только «описанное ⊆ структура ядра», если драфт получит тип openconnect. |
| `TestRegistryMapperSection` / `TestRegistryMapperCodesDeclared` / `TestRegistryWarningsHaveCauseAndFix` | `:1423` / `:1517` / `:1588` | У новых кодов обязательны `cause_*` и `fix_*`. |

### 3.2 Корпус

| Раннер | Где | Формат кейса |
|---|---|---|
| `TestContractCorpusBody` | `core/config/contract_body_test.go:365` | `contract/corpus/body/**/<case>.body` (ведущие строки `#` — комментарий, их отрезает `readCorpusBody` `:36`) + `<case>.expected.json` (`{meta.body_kind, nodes[{entry, kind, label, scheme, warnings?}], dropped?, v}`). Регенерация: `go test ./core/config -run TestContractCorpusBody -update`. Без `expected` кейс пропускается (Skip). Образец: `contract/corpus/body/singbox/openconnect_endpoint.{body,expected.json}` (вход в `outbounds[]`, ожидание `kind: endpoint`). |
| `TestContractCorpusAuthored` | `core/config/contract_authored_test.go:23` | `contract/corpus/authored/<case>.body` — голое тело узла (одна строка JSON после комментариев) + `.expected.json` (`meta.container: own`, у мягких правил коды с `applied:false`). Образец: `hard_flow_invalid_removed`. |
| `TestContractCorpusNodeEdit` | `ui/configurator/tabs/contract_node_edit_test.go` (каталог `:19`) | `contract/corpus/node_edit/*.edit.json` — правка JSON узла (контейнер, прежний источник, ввод). Корпуса `contract/corpus/form/` ещё нет; раннер новый (по PLAN — `core/config/contract_form_test.go`). |
| `TestEmitEveryScheme` | `core/config/nodeflow/nodeflow_test.go:426` | Явный список схем (`:427-429`); для пустого и базового тела `Sanitize` + `Emit` должны давать JSON. |

### 3.3 Файлы контракта

| Файл | Где | Формат |
|---|---|---|
| `contract/VERSION` | — | Сейчас `1.1.121` → `1.1.122`. |
| `contract/README.md` | Таблица «Версии» с `:288`; последние строки `:400` (1.1.120) и `:401` (1.1.121) | `\| 1.1.122 \| **Заголовок** (дата, решение владельца). Повод… Что объявлено… Корпус +N: … Встречная задача LxBox — TASKS_LXBOX.md §N \|`. Таблица атрибутов поля — `:167`–`:190` (строки «Роль» и «Только документация»): дописать `form`, `*`, `variant`. |
| `contract/TASKS_LXBOX.md` | `## 117.` на `:9249` (1.1.120), `## 118.` на `:9261` (1.1.121) | **Последний занятый § — 118**, новый — `## 119. Контракт 1.1.122 — …`. SPEC §3 и PLAN ссылаются на §118 — исправить. Формат: дата и повод → «Объявлено:» нумерованным списком → «Что сделать LxBox: …». |
| `contract/registry/warnings.json` | Образец записи `openconnect_core_unsupported` `:2044`–`:2066` | Ключ кода → `{severity (warning\|info), params[], title_en/ru, text_en/ru, cause_en/ru, fix_en[]/fix_ru[], desc, dart, go}`. Info-коды уже есть (`:79`, `:146`). |
| `contract/docs/PARSING_PRINCIPLES.md` | `:337`–`:340` | Абзац про `fields_unchecked` переписать на `*`. |
| `contract/registry/protocols/openconnect.json`, `openvpn-client.json` | `body` `:26`–`:36`, `refs.go` `:18-22`, `note` `:11`, `emit.note` `:15` | `fields_unchecked: true` → `order: […, "*"]`; ссылки на ветки `FieldsUnchecked` в `refs` и тексты `note` переписать. |

## 4. Подпись узла

### 4.1 Функции

| Что | Где | Как сейчас / куда |
|---|---|---|
| `deriveTransport(nodeType, raw)` | `ui/configurator/business/config_node_labels.go:16` | `registry.Get()` → `SchemeForSingboxType(nodeType)` → `TransportLabel`. |
| `TransportLabel(scheme, raw)` | `config_node_labels.go:39` | Реестр берёт сам через `registry.Get()`, снаружи его не передают. Если есть `reg.Field(scheme,"transport")`: `raw.transport.type`, `http`→`h2`, без блока — `tcp`. Иначе, если есть `vhttp`: значение или `Default`. Иначе `""`. **Сюда:** третья ветка `reg.FieldWithRole(scheme, RoleVariant)` → trim значения `raw[path]`, иначе `Default` поля, иначе `""`. |
| `deriveSecurity(nodeType, raw)` | `config_node_labels.go:74` | При `levels` у схемы — `reg.Level`; иначе `tls.enabled` → «TLS», `reality` → «Reality», `+Vision`. У openconnect `tls.enabled` нет → `""`. |
| `ConfigNode` | `ui/configurator/business/config_nodes.go:23`–`:43` | `Tag`, `Type`, `Kind`, `Server`, `ServerPort`, `Transport`, `Security`, `Detour`, `GroupMembers`, `Raw`. |
| `SubtitleParts()` | `config_nodes.go:69` | `[Type, Transport, Security]` без пустых частей. |
| `Endpoints()` | `config_nodes.go:118` | Узлы с `Kind=="endpoint"`. |
| `LoadConfigNodes` / `ParseConfigNodes` / `configNodeFromRaw` | `config_nodes.go:156` / `:216` / `:243` | Читает **сгенерированный config.json** (`outbounds[]` + `endpoints[]`), кэш по (mtime, size); `deriveTransport`/`deriveSecurity` вызываются на `:262-263`. |

### 4.2 Кто рисует

| Где | Что |
|---|---|
| `ui/servers_node_subtitle.go:229` `serversNodeSubtitle(ac, proxyInfo, scope)` | Список серверов (Local и Remote). Группа → `groupSubtitle` `:324`; затем предупреждение конвейера (`nodewarn.Subtitle`, `:248`) побеждает; иначе `strings.Join(node.SubtitleParts(),"·")` (`:251`). Особый случай Tailscale — `tailscaleSubtitle` `:274`. Вызовы: `ui/proxy_list_panel.go:943`, `:987`. **Сюда же** — место для состояния openconnect (`auth-pending`/`error`) по образцу `tailscaleSubtitle`. |
| `ui/proxy_list_panel_render.go:32` `effectiveNodeConfigPath(ac, scope)` | Удалённые машины идут **тем же** путём подписи: при `ScopeRemote` с активным override читается `platform.GetRemoteConfigPathFor(...)`. Второй функции подписи нет. |
| `ui/core_runtime_window.go:259-260` | Строки окна Core runtime (вкладка Endpoints — `:102-103`, `renderCoreRuntimeEndpoints` `:200`). |
| `ui/servers_filter.go:514`, `:532` | Фасеты фильтра: `Protocol=Type`, `Variants=parts[1:]`. С `variant` openconnect получит фасет «fortinet». |
| `ui/servers_node_info.go:525` | `memberRow` (члены группы в окне Info). |
| `ui/configurator/tabs/preview_node_subtitle.go:35` `previewNodeSubtitle(node *ParsedNode)` | Превью и списки источников конфигуратора (разобранные узлы, а не config.json): `Scheme · TransportLabel(node.Scheme, node.Outbound)` (`:119`) `· previewSecurityName` (`:126` — своя копия TLS/Reality, без `levels`). `variant` придёт сюда через `TransportLabel` сам. Потребители: `preview_row_view.go:115`, `source_folder_drilldown.go:607`, `source_edit_window.go:1861`, `source_node_row.go:154-173`. |

Итого: правка одного `TransportLabel` покрывает и config.json, и превью.

## 5. Формы узлов

### 5.1 Добавление (Add server)

| Что | Где | Как сейчас |
|---|---|---|
| Меню источников | `ui/configurator/tabs/source_tab.go:250` `addServerAction`, пункты меню `:422` (папка) и `:443` (корень) | `wizarddialogs.ShowAddServerDialog(presenter, nil, cb)`. Результат: в папке → `applyAddedSourcesToFolderNamed` (`source_folder_drilldown.go:1019`) → `AppendNodesToFolder` (`business/folder_fill.go:61`); в корне `res.ConfigJSON` → `AppendManualConfigJSON` (`business/sources_json.go:142`), иначе `AppendURLsToSources` (`business/sources.go:39`); затем `RelabelLastSources` (`sources_json.go:211`), refresh, `MarkAsChanged`. |
| Добавление в папку | `ui/configurator/tabs/folder_add_nodes.go:293` `(f *folderAddNodes) addServer()` | Тот же диалог; `ConfigJSON`/`Text` → `applyInput` (`:243`) → `AppendNodesToFolder` → `finish` (`:312`) → `applySourceMutation` (`source_tab.go:1275`). |
| Диалог | `ui/configurator/dialogs/add_server_dialog.go:80` `ShowAddServerDialog(presenter, owner, onResult)` | Отдельное окно 640×680, `owner` не используется. `AddServerResult{Text, ConfigJSON, Label}` (`:63`). Состояние `addServerForm` `:124`, режимы `addServerMode` `:174-184` (`modeSocks/HTTP/WireGuard/Tailscale/Source`), выбор схемы — `widget.NewSelect` (`:294`, подпись «Server type» `:402`), видимость — `applyModeVisibility` `:358`, вкладки «Parameters» + «JSON» (`newAddServerForm` `:186`), прокрутка `components.WrapInScrollWithGutter` (`:201`). **Сюда:** режим `modeOpenConnect`, поля в своём файле `add_server_openconnect.go`, по образцу Tailscale. |
| Образец endpoint-формы | `ui/configurator/dialogs/add_server_tailscale.go` | `tailscaleFields` `:52`, `buildTailscaleFields(onChange)` `:71` (`widget.NewPasswordEntry` `:77`, `widget.NewCheck` `:94-104`, взаимоисключение `syncExitRole` `:147`). `tailscaleDocument(tag, t)` `:156` строит `map[string]interface{}` с `putIfNotEmpty` (`:251`) → `tailscaleVerdict` (`:219`: `nodeflow.Sanitize` → `nodewarn.Summary(FromParsed)`; не пусто — ошибка формы) → `{"endpoints":[endpoint]}` → `ConfigJSON`. |
| JSON-вкладка | `add_server_dialog.go`: `refreshJSON` `:480`, `previewJSON` `:492`, `parseAddServerInput` `:556`, ручная правка `jsonDirty` `:418` → `manualJSONResult` `:655` | Превью через `config.EmitNodeJSONs` / `ParseNodeDocument` + `RenderNodeDocument(body, NodeBodyGoesToEndpoints(body))` (`:537-551`). |
| Помощники вёрстки | `ui/configurator/dialogs/warp_dialog.go:489` `labeledRow`, `:481` `numEntry`, `:502`/`:511` `registryEnum`/`registryDefault` | `labeledRow` = `container.NewBorder(nil,nil,Label,nil,control)`. |

### 5.2 Правка узла (окно источника)

| Что | Где | Как сейчас |
|---|---|---|
| Раскладка блоков | `ui/configurator/tabs/source_edit_window.go` `rebuildSettingsLayout`, ветка сервера `~:1343-1380`; блоки создаются на `:1251` (`newAWGBlock`) и `:1262` (`newTailscaleBlock`); вставка `:1364` (`tailscaleEditableNode`), `:1369` (`awgVisibleFor`) | Блок получает `nodeRef func() *wizardmodels.Node` и `onApplied`. Правки идут в рабочую копию `scratch` и попадают в модель только по Save. **Сюда:** `newOpenConnectBlock` и условие по схеме узла. |
| Save | `source_edit_window.go:2420` → `applySourceEditToModel` `:402` / `applyNodeEditToModel` `:441` → `afterSourceEditApplied` `:457` | `BumpRevision`, `PreviewNeedsParse`, `InvalidateNodePool`, refresh, `MarkAsChanged`. |
| Блок Tailscale | `ui/configurator/tabs/source_tailscale_edit.go`: `tailscaleRole` `:82`, `readTailscaleRole` `:121`, `applyTailscaleRole` `:148` (сначала delete, потом set), `writeTailscaleBody` `:184`, `newTailscaleBlock(nodeRef, win, onApplied)` `:246`, `NodeSchemeForSingboxType` `:98` | Отказ санитайзера → `dialog.ShowError` + `b.load(node)`. Заголовок — `sectionHeader` (`source_edit_overview.go:319`). |
| Блок AWG | `ui/configurator/tabs/source_awg_edit.go`: `awgSettings` `:109`, `readAWGSettings` `:155`, `validateAWGSettings` `:204`, `applyAWGSettings` `:232`, **`writeAWGBody(node, ob, keep)` `:279`**, `awgRejectedInput` `:318`, `newAWGBlock` `:398`, Select из `reg.FieldStrings(scheme,"ib")` `:90` | Конвейер записи тела, повторять его: `nodeflow.Sanitize` (`Drop` → откат с ошибкой) → `awgRejectedInput` (санитайзер снял введённый ключ → ошибка с заголовком кода) → `nodeflow.Emit` → `config.StampBodyType` → `node.Body` → `node.ReplaceDerivedWarnings(nodewarn.FromParsed(res.Warnings))` → `node.RevalidateCoreVerdictAfterBodyChange(bodyBefore)`. Автозапись по `OnChanged` с флагом `syncing`. |
| JSON узла | `ui/configurator/tabs/source_body_edit.go:51` `applyServerBodyJSON` (→ `config.MaterializeServerNode`/`MaterializeEditedBody`), `:185` `regenServerBodyFromRaw`; вызовы `source_edit_window.go:2072`, `:2134`; помощники `source_edit_json.go` (`stripEmittedDecorations` `:20`, `withPendingDetour` `:44`) | Путь ручного JSON, тот же для всех схем. |

### 5.3 Состояние и материализация

| Что | Где |
|---|---|
| `corestate.Origin` / `Node` / `Source` | `core/state/sources_v7.go:48` / `:199` / `:360`; алиасы UI — `ui/configurator/models/wizard_state_file.go:45-49` |
| `AppendManualConfigJSON` | `ui/configurator/business/sources_json.go:142`: `IsNodeDocument` → `ParseNodeDocument`, иначе `NodeFromManualConfigJSON` (`core/config/subscription/manual_config.go:36`, схема через `NodeSchemeForSingboxType` `:53`, `Port` из `server_port` `:68`) → `config.MaterializeServerNode` (`core/config/migrate_materialize.go:429`) → `corestate.Source{Node{Kind: server, Body, Origin, Warnings}}` |
| Раздел конфига | `config.IsEndpointScheme` (`core/config/endpoint_schemes.go:20`), `NodeBodyGoesToEndpoints` (`core/config/node_document.go:230`) |
| Импорт sing-box | `core/config/subscription/singbox_import.go:340` (схема), `:351-352` (`Server`/`Port` из `server`/`server_port`) |
| Подстановка адреса | `core/config/node_materialize.go:205` `outboundMapOf`, `:238` `schemeHasRootAddress` |
| `nodeflow.Result` | `core/config/nodeflow/sanitize.go:30` (см. §2.1); секреты в `Warning.Value` маскируются `***` |

### 5.4 Проверка перед сохранением, предупреждения, секреты

| Что | Где | Заметка |
|---|---|---|
| Проверка формы | `tailscaleVerdict` (`add_server_tailscale.go:219`), `engineVerdict` (`add_server_dialog.go:798`), `writeAWGBody` | Только санитайзер. `sing-box check` формы не вызывают. |
| Цикл отказа ядра | `core/rebuild_corereject.go`: `coreRejectCheck` `:447` (→ `validateConfigViaSingBox` `core/rebuild.go:30`), `coreRejectLoop.run` `:307`, `RunRejectLoop` `:570`, `DisableNodeNamedByCore` `:635`; разбор — `core/corereject/parse.go:118`, `decode.go:72` | UI: `presentation/draft_reject.go:22` `RunDraftRejectPreview` (вызов `tabs/final_tab.go:531`), `:47` `runDraftRejectLoop` (Save удалённой машины, `presenter_save.go:419`); вердикт снимает `RevalidateCoreVerdictAfterBodyChange` (`core/state/node_enabled.go:335`). Пункт SPEC §2.3 «сохранение — sing-box check кандидата» в формах сегодня не исполняется — см. §7. |
| Показ кодов | `internal/nodewarn/nodewarn.go`: `FromParsed` `:400`, `Summary` `:253`, `Subtitle` `:289`, `Describe` `:159`; `internal/nodewarn/section.go:117` `Section` (аккордеон по коду `:243-245`); окно узла — `tabs/source_node_warnings.go:147-159` | Тексты: `reg.WarningText(code, locale.GetLang(), params)`. `Field.Advisory` в `ui/` не читается — показ рядом с полем делать заново. |
| Секреты | `widget.NewPasswordEntry()` со встроенным «глазом»: `add_server_dialog.go:249`, `add_server_tailscale.go:77`, `settings_tab.go:992` | `Field.Secret` UI не читает. |

### 5.5 Конвенции Fyne

| Что | Где |
|---|---|
| `locale.T` / `Tf` / `TN` / `GetLang` | `internal/locale/locale.go:119` / `:138` / `:126` / `:199`. Ключ — английский текст. `bin/locale/ru.json`: `"Add server": { "value": "Добавить сервер" }` (`:3287`). Новые ключи — сразу с переводом. |
| Аккордеон | `widget.NewAccordion(widget.NewAccordionItem(title, content))`: `dialogs/warp_dialog.go:232` («Advanced (all fields)» — ближайший аналог в форме добавления), `ui/machine_add_window.go:96`, `ui/machine_edit_window.go:248,254,347`, `ui/service_window.go:170,459,499`. Своего раскрывашки нет; второй приём — показ/скрытие строк по чекбоксу (`awgBlock.setRowsVisible` `source_awg_edit.go:557`). |
| Select | `add_server_dialog.go:294`; по реестру — `source_awg_edit.go:409`, `warp_dialog.go:166,172,177,282`; `widget.NewSelectEntry` — `warp_dialog.go:174,288` |
| Check | `widget.NewCheck(locale.T(...), cb)`: `add_server_tailscale.go:94-104`, `source_awg_edit.go:447`, `warp_dialog.go:86,151,191` |
| Radio (для выбора сертификата) | `widget.NewRadioGroup`: `warp_dialog.go:61`, `dialogs/add_rule_dialog.go:139,310`, `dns_user_rules.go:205`, `ui/connection_local.go:60` |
| Заметки под полем | `widget.Label` с `Wrapping = TextWrapWord`, `Importance = LowImportance`; предупреждение — `WarningImportance` (`source_awg_edit.go`) |

## 6. Диалоги под демоном (второй фактор и SSO)

### 6.1 Режим backend

| Что | Где |
|---|---|
| `BackendMode`, `BackendClassic`, `BackendDaemon` | `core/backend.go:15`, `:21`, `:25`; `CoreBackend.Mode()` `:36` (`backend_daemon.go:260`, `backend_legacy.go:17`) |
| Выбор при старте | `initBackendFromSettings` `core/backend.go:576` (вызов `controller.go:395`; настройка `core_backend_mode`, `internal/locale/settings.go:131`); дефолт classic — `controller.go:306`; переключение — `SwitchBackendMode` `:137` |
| Проверка в UI | `ac.BackendMode() == core.BackendDaemon` (`core/backend.go:127`): `ui/proxy_list_panel.go:510`, `:1910`, `ui/connection_local.go:83/108/144`. Удалённые машины ходят по gRPC и в classic (`proxy_list_panel.go:510`: `scope == ScopeRemote && lxdConnected`). |
| Цель local/remote | `CoreTarget{Scope; MachineID}` `core/backend.go:300`; `machineTransport` `:315`; UI — `nodeWindowTarget(scope)` `ui/clash_remote.go:250` |

### 6.2 Локальный демон (`core/backend_daemon.go`, build tag `darwin \|\| (windows && !386)`, заглушка `backend_daemon_stub.go:11`)

| Что | Где | Как сейчас |
|---|---|---|
| `DaemonBackend` | `:35`–`:81` | `ctx`/`cancel` (`:39-40`) — жизнь всех supervise-циклов; `conn` (`:42-43`); кэш `tailscale services.TailscaleStatusCache` (`:69`). **Сюда:** кэш openconnect. |
| `newDaemonBackend` | `:160`–`:185` | Запускает `superviseStatus/Logs/Connections/Tailscale` (`:180-183`). **Сюда:** `go b.superviseOpenConnect()`. |
| `grpcConn` / `grpcClient` | `:538` / `:553` | Ленивое общее соединение; `daemonpb.NewStartedServiceClient(conn)`. |
| `isActive` | `:255` | Фрейм старого backend состояние не трогает. |
| `refreshUI` | `:530` | `ac.UIService.UpdateCoreStatusFunc()`. |
| `Close` | `:512` | `cancel()` — все циклы выходят. |
| `daemonProxyTransport.rpc()` | `:847`, `:853-860` | Unary-вызов с таймаутом 15 с. |
| `isUnimplemented` | `core/backend_daemon_dns.go:21` | Ядро без реализации RPC отличается от лежащего демона; Unimplemented повтором не лечится. |

Паттерн supervise (`superviseStatus` `:570-607`, `superviseLogs` `:680-706`): `grpcClient()` → `client.SubscribeX(b.ctx, req)` → `consumeX(stream)` до ошибки `Recv`; бэкофф 1 с ×2 до 10 с; сброс только после первого фрейма.

`core/backend_daemon_tailscale.go`: `superviseTailscale` `:29-55` (`SubscribeTailscaleStatus` `:38`), `consumeTailscaleStream` `:60-75` (`defer MarkDead`, `isActive`, `cache.Apply(upd)` — фрейм заменяет весь снимок), `TailscaleStatus(tag)` `:78`, `TailscaleLive` `:83`, unary `TailscaleSetExitNode` `:93` / `StatusNow` `:111` / `Logout` `:122` (таймаут 10 с, `:88`), поток с колбэком `TailscalePing` `:138`. **Образец для `core/backend_daemon_openconnect.go`.**

Кэш: `core/services/tailscale_status.go:266-314` (`Apply` `:275`, `MarkDead` `:288`, `Get` `:295`, `Live` `:303`, `OnSnapshot` `:310` — **не вызывается**).

### 6.3 Удалённые машины (`core/services/`)

| Что | Где | Как сейчас |
|---|---|---|
| `LxdRemoteTransport` | `lxd_remote_transport.go:42-60` | `conn`, `ts tailscaleStream` (`:59`). **Сюда:** поле `oc openConnectStream`. |
| `streamConn` | `:76` | Ленивый dial для подписок. |
| `runResilientStream(ctx, name, attempt, onDrop)` | `:100-118` | Цикл attempt → onDrop → пауза 2 с (`:72`). |
| `rpc()` | `:156-171` | Unary, таймаут 25 с (`:129`), контекст от `Background`. |
| `Close` | `:136` | Сначала `stopTailscaleStream()`. **Сюда:** остановка стрима openconnect. |
| `tailscaleStream` / `ensureTailscaleStream` / `stopTailscaleStream` | `lxd_remote_tailscale.go:29-34` / `:40-75` / `:78` | Запуск лениво, не чаще одного раза, из аксессоров `TailscaleStatus` (`:88`) и `TailscaleLive` (`:94`). Команды через `t.rpc()` (`:102`, `:118`, `:128`). **Образец для `lxd_remote_openconnect.go`.** |
| Unimplemented на удалённой стороне | `lxd_remote_transport.go:459`, `:751` | Образец обработки старого демона. |

### 6.4 Фасад выбора транспорта (`core/tailscale_status.go`)

`TailscaleTarget = CoreTarget` (`:44`), `TailscaleIn` (`:47`), `tailscaleSource` interface (`:36`), `(ac).tailscaleSource(t)` (`:57-71`): Remote → `ac.machineTransport(t)` (→ `UIService.LxdMachineTransportFunc`, зарегистрирован `ui/lxd_remote_override.go:262`, реализация `:267`) с утверждением типа; иначе `ac.Backend().(tailscaleSource)`. Контроллер (`:116`, `tailscaleControl` `:133`), публичные `TailscaleAvailable` `:75` … `TailscalePing` `:177`. **Образец для `core/openconnect_status.go`.**

### 6.5 gRPC (`internal/daemonpb/`, синк `SYNC_REV` = `729f1acf…`, `.proto` в репозитории нет)

Клиент (`started_service_grpc.pb.go`):
- `:116` `SubscribeOpenConnectStatus(ctx, *emptypb.Empty, ...) (grpc.ServerStreamingClient[OpenConnectStatusUpdate], error)`
- `:117` `SubmitOpenConnectAuthResponse(ctx, *OpenConnectAuthResponseSubmission, ...) (*emptypb.Empty, error)`
- `:118` `CancelOpenConnectAuthChallenge(ctx, *OpenConnectAuthChallengeCancel, ...) (*emptypb.Empty, error)`

Сообщения (`started_service.pb.go`; `State`/`Kind` — строки, enum нет):

| Тип | Строка | Поля |
|---|---|---|
| `OpenConnectStatusUpdate` | 5893 | `Endpoints []*OpenConnectEndpointStatus` |
| `OpenConnectEndpointStatus` | 5937 | `EndpointTag`, `State`, `StateText`, `AuthChallenge *OpenConnectAuthChallenge`, `Error`, `TunnelInfo *OpenConnectTunnelInfo` |
| `OpenConnectTunnelInfo` | 6021 | `Server`, `Flavor`, `Transport`, `Ipv4/Ipv6/Dns []string`, `Mtu uint32`, `ConnectedSince int64` |
| `OpenConnectAuthChallenge` | 6121 | `Id`, `Banner`, `Message`, `Error`, oneof `Challenge`: `_Form{Form}` (6223) / `_Browser{Browser}` (6227); `GetForm`/`GetBrowser` ~6200-6216 |
| `OpenConnectAuthForm` | 6235 | `Fields []*OpenConnectAuthFormField` |
| `OpenConnectAuthFormField` | 6279 | `SubmissionKey`, `Name`, `Label`, `Kind`, `Value`, `Options []*OpenConnectAuthFormChoice` (6363: `Value`, `Label`) |
| `OpenConnectBrowserRequest` | 6415 | `Url`, `FinalURL`, `CookieNames`, `HeaderNames`, `CallbackURLPrefixes`, `EarlyCookieNames []string`, `CacheID` |
| `OpenConnectAuthFormResponse` | 6611 | `Values map[string]string` |
| `OpenConnectBrowserResult` | 6655 | `FinalURL`, `Cookies []*OpenConnectBrowserCookie` (6507), `Headers []*OpenConnectBrowserHeader` (6559) |
| `OpenConnectAuthResponseSubmission` | 6715 | `EndpointTag`, `ChallengeID`, oneof `Response`: `_Form{Form}` (6801) / `_Browser{Browser}` (6805) |
| `OpenConnectAuthChallengeCancel` | 6813 | `EndpointTag`, `ChallengeID` |

Других следов OpenConnect в Go-коде, кроме pb и реестра, нет.

### 6.6 UI: ссылка входа, браузер, диалоги

| Что | Где | Как сейчас |
|---|---|---|
| Кнопка входа Tailscale | `ui/servers_node_network_tab.go:291-300` в `buildTailscaleNetwork` (`:259`) | `NeedsLogin && AuthURL != ""` → `widget.NewButton(locale.T("Sign in"), …platform.OpenURL(url))`, HighImportance в строке Status (`:318`). Проверки `urlsafe` нет. |
| Вкладка и поллинг | `tailscaleNetworkTab(ac, win, tag, cfgPath, scope)` `:96` (строится в `ui/servers_node_info.go:298`); поллинг раз в 1 с `:171-181`, перерисовка в `fyne.Do` (`:160`), стоп по `!windowOpen(win)` (`:176`, помощник `:246`) | Образец живого статуса. |
| Открыть URL | `platform.OpenURL(url) error`: `internal/platform/platform_darwin.go:30` (`open`), `platform_windows.go:34`, `platform_linux.go:33` | `fyne.App.OpenURL` в проекте не используется. Фильтр схем — `internal/urlsafe/url.go:28` `IsSafeAnnounceURL` (http/https/tg). |
| Loopback-слушатель | только `core/debugapi/server.go:184` (`net.Listen("tcp","127.0.0.1:<port>")`, `http.Server{ReadHeaderTimeout:5s}` `:197`, `Serve` `:217`) | Образец кода для `core/openconnect_sso_callback.go`; временного слушателя нет. |
| Родительское окно | «Servers» — вкладки главного окна (`ui/app.go:75-76`), родитель — `ac.UIService.MainWindow` (`core/uiservice/ui_service.go:28`); окно узла — `showNodeInfoWindow` `ui/servers_node_info.go:42`, `NewWindow` `:55` | Отдельного окна Servers нет. |
| Обёртки диалогов | `internal/dialogs/dialogs.go:25` `NewCustom(title, content, buttons, dismissText, parent)` (Esc закрывает), `ShowActions` `:396` (`SetBusy`/`SetStatus`), `ShowCustom` `:454`, `ShowConfirm` `:461`, `ShowError` `:165`; `ui/dialogs.go` (`ShowError` `:11`, …, всё через `fyne.Do`) | — |
| Образцы | `ui/servers_node_network_tab.go:588-591` (диалог Ping: `dialog.NewCustom` + `SetOnClosed(cancel)` + поток через `fyne.Do` — ближайший аналог живого диалога челленджа); `ui/proxy_list_panel.go:1853-1882` (`NewCustomConfirm` над вкладкой Servers); `ui/machine_list_panel.go:1138` (`dialogs.NewCustom`); `ui/configurator/dialogs/save_state_dialog.go:167` (форма полей на обёртке) | — |

## 7. Риски реализации

1. **`server_port` протечёт сквозь `*`.** Как только у openconnect появится описанное поле `server`, `schemeHasRootAddress` (`node_materialize.go:238`) вернёт `true`. Тогда `outboundMapOf` (`:205`) допишет `server_port` из `ParsedNode.Port`, если тот > 0. Под `*` ключ уйдёт в ядро, у ядра такого поля нет, и упадёт декодирование всего конфига. Импорт sing-box и ручной JSON ставят `Port` только из `server_port` (то есть 0), но форма не должна заполнять `ParsedNode.Port` разбором `host:port` из `server`.
2. **`checkFieldRoles` (`registry_body_test.go:737`) пускает роль только на `string`/`listable_string`.** `flavor` — `enum`, без правки линтера тест упадёт.
3. **`HoldsOn` не подставляет `default`.** Правила `visible_when` надо писать в расчёте на отсутствие ключа: для «не fortinet» — `not_in`, а не равенство с `anyconnect`.
4. **Форма из `dialogs/` и блок из `tabs/` — два места.** Общая модель «тело ↔ поля по `reg.Form`» должна жить в одном файле (например, `ui/configurator/business/openconnect_form.go`), иначе Add и правка разойдутся. Этот файл не назван в PLAN.
5. **Supervise-стрим openconnect должен останавливаться на `codes.Unimplemented`** (ядро без `with_openconnect` или старый удалённый демон), иначе будет вечная переподписка каждые 1–10 с. Образец — `isUnimplemented` (`backend_daemon_dns.go:21`).
6. **Push в UI нет.** `OnSnapshot` никто не зовёт; чтобы диалог всплывал по новому `challenge.id`, нужен колбэк из `consume…` в `fyne.Do` (или поллинг главного окна, но он не привязан к открытому окну узла).
7. **Открывать URL браузера ядра через `urlsafe.IsSafeAnnounceURL`.** Кнопка Tailscale этого не делает; ссылку из удалённого демона без проверки открывать нельзя.

## 8. Приложения

### 8.1 Все места `FieldsUnchecked` / `fields_unchecked` (снять)

| Файл | Строки |
|---|---|
| `core/config/registry/registry.go` | `:538-541` (`section`), `:642-643` (`BodySchema`), `:913` (`resolveSection`) |
| `core/config/nodeflow/sanitize.go` | `:218-230` (`SanitizeFromKind`) |
| `core/config/nodeflow/emit.go` | `:29-39` (`Emit`) |
| `core/config/nodeflow/fields_unchecked_test.go` | `:8`, `:10`, `:47` (файл целиком → `TestWildcardNestedStrict`) |
| `contract/schema/registry_body.schema.json` | `:672-675` |
| `contract/registry/protocols/openconnect.json` | `:11` (note), `:15` (emit.note), `:19-20` (refs.go), `:31` (атрибут) |
| `contract/registry/protocols/openvpn-client.json` | `:11`, `:15`, `:19-20`, `:31` |
| `contract/docs/PARSING_PRINCIPLES.md` | `:337-340` |
| `contract/corpus/body/singbox/openconnect_endpoint.body` | `:6` (комментарий) |
| `contract/corpus/body/singbox/openvpn_client_endpoint.body` | `:3` (комментарий) |
| `contract/README.md` | `:380` (1.1.99), `:400` (1.1.120) — история, не править; новая строка 1.1.122 сообщает о снятии |
| `contract/TASKS_LXBOX.md` | `:8829`, `:9255` — история, не править |
| `contract/docs/generated/**` | пересобирается `go generate ./contract/...` |

### 8.2 Явные списки схем реестра

| Файл | Строки |
|---|---|
| `core/config/registry/registry.go` | `:711-715` `protocolFiles` |
| `core/config/registry_body_test.go` | `:466-469` `registryProtocolSchemes`; `:1299-1305` `schemeFile` (без openconnect/openvpn-client); `:1429` (`+ "group"`) |
| `core/config/nodeflow/nodeflow_test.go` | `:427-429` `TestEmitEveryScheme` |

openconnect во всех списках, кроме `schemeFile`, уже есть. Новых схем SPEC 167 не добавляет.
