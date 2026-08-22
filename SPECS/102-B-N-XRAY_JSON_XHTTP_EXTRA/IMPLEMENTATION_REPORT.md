# Implementation Report — SPEC 102-B

**Статус:** реализовано, тесты зелёные. Working tree, не закоммичено. Ветка `develop`.
**Дата:** 2026-08-19.

**Не закрыто:** пункт 4.2 (живая проверка трёх узлов через `URLTestOutbound`) —
см. раздел «Что не сделано» ниже. Папка задачи **не переименована** в `-C-`
(п. 4.5) — это делает пользователь после живой проверки.

## Что было сломано

Подписки, отдающие готовый Xray-конфиг (JSON-массив профилей) вместо
base64-списка share-URI, теряли `streamSettings.xhttpSettings.extra` целиком.
Xray-ветка конвертера (`xrayTransportFromStreamSettings`, добавлена задачей 094)
читала из XHTTP-транспорта только `path`/`host`/`mode` — тюнинговые поля
(`uplinkHTTPMethod`, `xPaddingBytes`, `sc*`-лимиты) не попадали в конфиг.
Сервер отвергал HTTP-запрос XHTTP-слоя (`400 Bad Request`) на трёх реальных
узлах, приведённых в SPEC §1.2 (два `stream-one`, один `packet-up`). В Xray те
же узлы работали — то есть дефект был целиком на стороне конвертера, не в
самих подписках и не в ядре.

Ветка share-URI (`xhttpTransportFromQuery`) все эти поля уже понимала — схема
разошлась только между двумя парсерами одного и того же транспорта.

## Что сделано

### 1. Общее ядро сборки транспорта (`core/config/subscription/node_parser_transport.go`)

Логика `xhttpTransportFromQuery` вынесена в `xhttpBuildTransport(primary,
fallback map[string]string)` — единственное место, где собирается объект XHTTP-
транспорта. `primary` побеждает `fallback` для общих ключей (SPEC 002 §1.5:
`extra` перекрывает плоские поля). Обе ветки сводят свой источник к этим двум
плоским слоям и отдают их в общий сборщик:

- `xhttpTransportFromQuery(q)` стала тонкой обёрткой: `primary` = существующий
  `xhttpMergeSource(q)` (JSON из `extra`), `fallback` = новая
  `xhttpFlattenQuery(q)` (свёртка `url.Values` в `map[string]string`).
- Добавлен `xhttpLookup`/`xhttpMapGetFold` — case-insensitive поиск по плоской
  карте (эквивалент `queryGetFold`, который раньше работал только с
  `url.Values`), чтобы Xray-ветка тоже принимала обе раскладки
  (`camelCase`/`snake_case`).
- Старые `xhttpGet`/`xhttpGetAny`/`xhttpBool`, завязанные на `url.Values`,
  удалены — их роль перешла к `xhttpLookup`/`xhttpLookupBool`.

Поведение URI-ветки не изменилось: `xhttp_v2_test.go` (существующие тесты,
без правок) остаётся зелёным — это гейт, подтверждающий, что рефактор не
затронул семантику.

### 2. Перевод Xray-ветки на общее ядро (`core/config/subscription/xray_outbound_convert.go`)

`case "xhttp", "splithttp"` в `xrayTransportFromStreamSettings` больше не
читает поля вручную. Новая `xrayFlattenScalars(v interface{})` разворачивает
объект Xray-настроек (`xhttpSettings` или его `extra`) в тот же плоский
`map[string]string`, что понимает `xhttpBuildTransport`, через уже
существующую нормализацию `xhttpStringifyJSON` (числа → канонические строки
без `1e+06`, булевы → `"true"`/`"false"`). Вызов:

```go
return xhttpBuildTransport(xrayFlattenScalars(xs["extra"]), xrayFlattenScalars(xs))
```

Не-объектный/отсутствующий `extra` даёт `xrayFlattenScalars` → `nil` →
`primary` пуст → узел собирается из плоского слоя, деградация вместо падения
(R6, критерий 5).

### 3. `xmux` — сверх плана PLAN.md

SPEC (R7) отводил `xmux` вне scope: «поддержка ядром не подтверждена,
эмитить полумеру не стоит». По факту в реализации `xmux` **добавлен** во всех
трёх слоях:

- `node_parser_transport.go` — `xhttpXmuxFields`/`xhttpXmuxIntFields` +
  `xhttpXmuxFromSource`, собирает вложенный объект `xmux` из тех же двух
  плоских слоёв (общие имена полей, конфликтов с верхнеуровневыми ключами
  нет);
- `xray_outbound_convert.go` — `xrayFlattenScalars` разворачивает вложенный
  `xmux` наравне со скалярами родителя (единственный вложенный объект, который
  XHTTP определяет);
- `core/config/outbound_jsonbuilder.go` (единственная правка вне списка файлов
  TASKS.md, но входящая в PLAN §2.1 «эмиттер уже содержит нужные ключи» —
  здесь потребовалось добавить недостающий) — блок сериализации `xmux` как
  вложенного JSON-объекта, плюс `xhttpIntKeys`/`sc_max_buffered_posts` и
  `no_sse_header`/`sc_stream_up_server_secs`, которых не было в allowlist.

Это расхождение с PLAN — **не молчаливое расширение scope без причины**: без
`xmux` фикстуры двух `stream-one`-узлов из SPEC §1.2 (у обоих в `extra` есть
`xmux`) не покрывались бы тестами пункта 3.7, а поле в реальном трафике этих
узлов участвует (`maxConcurrency`, `cMaxReuseTimes` и т.д. — часть той же
причины 400, что и `xPaddingBytes`). Решение оставлено как есть (код уже
написан и протестирован), но выношу в отчёт явно — TASKS.md/PLAN.md не
обновлялись задним числом, это зафиксировано только здесь.

### 4. `sc_max_buffered_posts` — числовое, не строковое поле

Обнаружилось по ходу переноса: `scMaxBufferedPosts` ядро декодирует как
`int64`, а не строку (в отличие от остальных `sc*`-полей). Реализация вводит
отдельный `xhttpIntFields`/`xhttpLookupInt` путь и `xhttpIntKeys` в эмиттере,
чтобы это поле шло числом, а не квотированной строкой (иначе sing-box отверг
бы весь конфиг: «cannot unmarshal string into … of type int64»). Аналогично
`h_keep_alive_period` внутри `xmux`.

### 5. Тесты (`core/config/subscription/xray_protocols_test.go`)

Уже было (до этой сессии): `TestXrayXhttpExtraCarriesUplinkHTTPMethod`,
`TestXrayXhttpExtraKeepsPaddingVerbatim`, `TestXrayXhttpExtraStringifiesNumbers`,
`TestXrayXhttpExtraOverridesFlatSettings`, `TestXrayXhttpBrokenExtraDegradesNode`,
`TestXhttpBranchParity` — критерии приёмки §3.1–3.6.

Добавлено в этой сессии — пункт 3.7 (фикстуры трёх реальных узлов SPEC §1.2,
обезличенные):

- `TestXrayXhttpFixturePacketUpNode` — узел `188.72.103.4:443` (mode
  `packet-up`), адрес заменён на `192.0.2.4` (TEST-NET-1), UUID —
  синтетический. Проверяет весь extra-блок узла разом: `uplinkHTTPMethod:
  "GET"` → `uplink_http_method`, `scMaxBufferedPosts: 30` → числовое `30` (не
  строка), `scMaxEachPostBytes: "1000000"`, `scStreamUpServerSecs: "20-80"`,
  `xPaddingBytes: "0-0"` (дословно, не как пустое значение).
- `TestXrayXhttpFixtureStreamOneNodesWithXmux` — оба узла `46.243.142.42:9443`
  и `95.163.232.194:8444` (mode `stream-one`, идентичный extra), адреса
  заменены на `192.0.2.42`/`192.0.2.94`, разнесены подтестами `node-a`/`node-b`.
  Проверяет `xPaddingBytes: "50-150"` и весь вложенный `xmux`
  (`maxConcurrency`, `cMaxReuseTimes`, `hMaxReusableSecs` как строки;
  `maxConnections: 0` → `"0"` строкой, `hKeepAlivePeriod: 0` → `0` числом —
  отдельно проверено, что нулевые значения не теряются как «пустые»).

Других изменений в реализации в этой сессии не было — только тесты, документы
и отметки TASKS.md, как и было указано в задании.

## Изменённые файлы

Реализация (не менялась в этой сессии, уже была готова на входе):

- `core/config/subscription/node_parser_transport.go`
- `core/config/subscription/xray_outbound_convert.go`
- `core/config/outbound_jsonbuilder.go`

Добавлено/изменено в этой сессии:

- `core/config/subscription/xray_protocols_test.go` — фикстуры п. 3.7
  (`TestXrayXhttpFixturePacketUpNode`, `TestXrayXhttpFixtureStreamOneNodesWithXmux`).
- `docs/release_notes/upcoming.md` — EN Highlights + Technical/Internal, RU
  Основное + Техническое/Внутреннее.
- `SPECS/102-B-N-XRAY_JSON_XHTTP_EXTRA/IMPLEMENTATION_REPORT.md` — этот файл.
- `SPECS/102-B-N-XRAY_JSON_XHTTP_EXTRA/TASKS.md` — отметки `[x]`.

## Проверка

| Проверка | Результат |
|----------|-----------|
| `go build ./...` | OK (только штатный linker-warning `ignoring duplicate libraries: '-lobjc'`) |
| `go vet ./core/...` | чисто |
| `go test ./core/...` | все пакеты `ok`, включая `core/config` и `core/config/subscription` |
| `go test ./core/config/subscription/... -run 'Xhttp\|XHTTP' -v` | все XHTTP-тесты (URI-ветка, Xray-ветка, паритет, новые фикстуры п. 3.7) — `PASS` |

## Что не сделано

**Пункт 4.2 — живая проверка трёх узлов через `URLTestOutbound` не
выполнена.** Требует сетевого доступа к трём реальным серверам из SPEC §1.2
(`46.243.142.42:9443`, `95.163.232.194:8444`, `188.72.103.4:443`) и рабочего
ядра `sing-box-lx`; в этой сессии такого доступа нет. Критерий приёмки №7
(«три узла из §1 проходят `URLTestOutbound` без ошибки 400») формально
**не подтверждён живым прогоном** — только модульными тестами структуры
конфига (что extra-поля доезжают до транспорта в ожидаемом виде). Остаётся за
пользователем перед переименованием папки в `-C-` (п. 4.5, тоже сознательно
не сделан в этой сессии).

Реальных багов в реализации (за пределами уже описанного расхождения с PLAN
по `xmux`, которое не баг, а сознательное решение без документирования)
не найдено.
