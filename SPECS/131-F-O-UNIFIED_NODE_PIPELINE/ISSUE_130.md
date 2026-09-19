# Issue #130 — trojan:// + gRPC: `v2ray-grpc: unexpected status: 404 Not Found`

Узел автора: транспорт gRPC, `serviceName` вида `/*****/something/Tun`.
Сервер отвечает 404 — значит путь запроса, который шлёт ядро, не совпадает
с тем, который слушает сервер.

## 1. Диагноз

### Ведущий «/» у Xray — это ДРУГАЯ ФОРМА ЗАПИСИ, а не часть имени

Исходная гипотеза («снять ведущий `/`, как это делает Xray») неверна и, если
бы её реализовали буквально, дала бы неправильный путь. Xray разбирает
`serviceName` двумя разными способами, и ведущий `/` их переключает:

`~/go/pkg/mod/github.com/xtls/xray-core@v1.260327.1-.../transport/internet/grpc/config.go`

- `getServiceName()` — строки 17–33:
  - без ведущего `/` (строка 19): всё имя целиком уходит в
    `url.PathEscape(...)` — внутренние `/` превращаются в `%2F`;
  - с ведущим `/` (строки 24–32): берётся кусок **от первого до ПОСЛЕДНЕГО**
    `/` (`c.ServiceName[1:lastIndex]`), режется на сегменты, каждый
    экранируется по отдельности, и `/` между сегментами остаются живыми.
- `getTunStreamName()` — строки 36–43: при ведущем `/` именем ПОТОКА
  становится **последний сегмент** (`/` … `|` отсекается), а не литерал `Tun`.

Отсюда точное значение строки автора:

| `serviceName` | сервис (Xray) | поток (Xray) | путь на проводе |
|---|---|---|---|
| `/abcde/something/Tun` | `abcde/something` | `Tun` | `/abcde/something/Tun` |
| `abcde/something` | `abcde%2Fsomething` | `Tun` | `/abcde%2Fsomething/Tun` |
| `/abcde/Tun` | `abcde` | `Tun` | `/abcde/Tun` |

Подтверждено тестами самого Xray
(`transport/internet/grpc/config_test.go:30` — `/my/sample/path/a|b` →
`my/sample/path`; `config_test.go:40` — `/foo` → `""`).

То есть `/abcde/something/Tun` — это НЕ имя сервиса со слэшем впереди.
Последний сегмент `Tun` здесь — имя потока, и сервер слушает
`/abcde/something/Tun`.

### Что делает ядро

`sing-box-lx` v1.14.1-lx.7, `transport/v2raygrpclite/client.go:58–59`:

```go
Path:    "/" + options.ServiceName + "/Tun",
RawPath: "/" + url.PathEscape(options.ServiceName) + "/Tun",
```

Имя потока захардкожено (`/Tun`), а `service_name` целиком уходит в
`url.PathEscape`. На проводе `net/http` берёт `RawPath`, поэтому при
`service_name = "/abcde/something/Tun"` запрос уходит на

```
/%2Fabcde%2Fsomething%2FTun/Tun
```

Сервер ждёт `/abcde/something/Tun`, сравнение путей у gRPC-сервера строгое
(`transport/v2raygrpclite/server.go:45` строит `"/" + ServiceName + "/Tun"`,
`server.go:69–70` — `if request.URL.Path != s.path { … StatusNotFound }`),
отсюда ровно тот 404, что в issue. Полная (не lite) реализация ведёт себя так
же: `transport/v2raygrpc/custom_name.go` — `"/"+name+"/Tun"`.

**Вывод: баг на стороне лаунчера — диалект Xray не переводился в диалект
sing-box.** Оба ядра при этом консистентны сами с собой: проблема возникает
только при переносе строки из Xray-клиента (v2rayN и подобные) в sing-box.

## 2. Где значение попадало в конфиг

Ведущий `/` не трогался нигде — строка ехала в `transport.service_name` как есть:

- `core/config/subscription/node_parser_transport.go:214–222` — share-URI,
  приоритет `serviceName` → `service_name` → `path`;
- `core/config/subscription/xray_outbound_convert.go:339–341` — Xray-JSON,
  `grpcSettings.serviceName`;
- `core/config/subscription/node_parser_core.go:822` — ветка vmess-JSON
  (`path` → `service_name`).

## 3. Что изменено

Правило выражено **в реестре**, а не в мапперах: вход три, семантика поля одна,
и по SPEC 131 §3–§4 парсер обязан оставаться тупым маппером. Один флаг на поле
чинит все три входа сразу.

- `contract/registry/transports.json` — у `body.variants.grpc.fields.service_name`
  добавлено `"normalize": "grpc_service_name"` + `impl` с обоснованием и
  зафиксированным ограничением ядра.
- `core/config/nodeflow/coerce.go` — новый режим `grpc_service_name` в
  `normalize()` (функция `normalizeGRPCServiceName`). Правило:
  - нет ведущего `/` → значение не трогаем (старый формат);
  - есть ведущий `/` И последний сегмент (до `|`) равен `Tun` → снимаем
    ведущий `/` и хвост `/Tun`, остальное отдаём как `service_name`;
  - последний сегмент — другое имя потока (`/svc/Stream`) → **не трогаем**:
    ядро такой поток не умеет, и молчаливая правка превратила бы узел в
    рабочий-но-чужой путь. Пусть ломается заметно.
  - хвост `|multi` (серверная запись Xray, имя потока `TunMulti`) отбрасывается:
    к клиенту он не относится.
- `contract/schema/registry_body.schema.json` — `grpc_service_name` в enum
  `normalize`. Заодно в enum добавлен `hex_only`, который уже использовался
  (`tls.json:561`), но в схеме отсутствовал — схема расходилась с реестром.
- `core/config/registry_body_test.go` — `grpc_service_name` в словарь
  `registryNormalizeModes` (guard-тест `TestRegistryBodyStructure`).
- `contract/docs/generated/protocols/_transports.md` — регенерация.
- `docs/release_notes/upcoming.md` — строка в Fixes (EN + RU).

**Кода-предупреждения нет намеренно.** Это перевод написания, а не подгонка
значения: путь на проводе не меняется, узел не деградирует. `normalize_code`
ставится там, где normalize ВЫБРАСЫВАЕТ часть значения
(`core/config/nodeflow/sanitize.go:439–462`), — здесь не тот случай.

### Кейсы корпуса

- `contract/corpus/uri/trojan/grpc_service_name_leading_slash.*` — `/abcde/Tun`
  → `service_name: "abcde"` (чинится полностью);
- `contract/corpus/uri/vless/grpc_service_name_leading_slash.*` —
  `/abcde/something/Tun` → `service_name: "abcde/something"` (форма из issue;
  в комментарии зафиксировано ограничение ядра, см. §4);
- `contract/corpus/body/xray/grpc_service_name_leading_slash.*` — тот же
  `serviceName` через Xray-JSON: другой вход, то же реестровое правило.

### Проверки

- `go build ./...` — чисто.
- `go test ./core/config/... -count=1` — `core/config`, `configtypes`,
  `nodeflow`, `subscription` — ok.
- `go generate ./contract/...` — 23 страницы, повторный прогон идемпотентен.
- go1.20/Win7: `go build -modfile=go.win7.mod ./core/config/nodeflow/...
  ./core/config/subscription/... ./core/config/registry/...` — чисто;
  в новом коде только `strings` (`TrimSpace`/`HasPrefix`/`LastIndex`/`IndexByte`),
  ни `min`/`max`, ни `slices`, ни `maps`.

## 4. Что остаётся ограничением ЯДРА (не чинится в лаунчере)

`transport/v2raygrpclite/client.go:59` (и симметрично
`transport/v2raygrpc/custom_name.go`) гонит `service_name` целиком через
`url.PathEscape`, поэтому **`/` ВНУТРИ имени сервиса уезжает на провод как
`%2F`**. Следствие после нашего фикса:

- `/abcde/Tun` → `service_name = "abcde"` → путь `/abcde/Tun` — **совпадает
  с Xray, узел работает**;
- `/abcde/something/Tun` → `service_name = "abcde/something"` → путь
  `/abcde%2Fsomething/Tun`, а сервер ждёт `/abcde/something/Tun` — **404
  остаётся**.

То есть односегментные имена чинятся полностью, многосегментные — нет:
их ядро в принципе не умеет выразить. Это не решается на стороне лаунчера,
и подменять значение, чтобы «обойти» экранирование, нельзя (percent-кодирование
`%2F` в `service_name` ядро экранирует повторно — `%252F`).

### Черновик заявки в форк (lx)

> **gRPC: multi-segment `service_name` cannot express Xray's absolute-path form**
>
> `transport/v2raygrpclite/client.go:58-59` (and `transport/v2raygrpc/custom_name.go`)
> build the request path as `"/" + url.PathEscape(service_name) + "/Tun"`.
> `PathEscape` encodes `/` as `%2F`, so a service name containing a slash can
> never produce a multi-segment path.
>
> Xray supports this via its absolute-path form (`transport/internet/grpc/config.go`,
> `getServiceName`/`getTunStreamName`): a leading `/` means the last segment is
> the *stream* name and the inner `/` separators are preserved per-segment, so
> `/a/b/Tun` reaches the wire as `/a/b/Tun`. Servers configured that way are
> unreachable from sing-box.
>
> Suggested: escape `service_name` per segment (split on `/`, `PathEscape` each,
> re-join) so inner separators survive, matching Xray. The stream name staying
> hardcoded as `Tun` is fine for the common case; an optional stream name would
> additionally cover `/a/b/CustomStream`.

**Решение за командой ядра** — здесь только фиксация и формулировка.

## 5. Черновик ответа автору issue (EN)

> Thanks for the report — reproduced, and the cause is on our side.
>
> In Xray, a `serviceName` that starts with `/` is a different notation, not just
> a slash-prefixed name: the last segment is the gRPC *stream* name, so
> `/xxxxx/something/Tun` means service `xxxxx/something` + stream `Tun`, and the
> request path on the wire is `/xxxxx/something/Tun`. sing-box takes the plain
> service name and always appends `/Tun`, so we were sending the whole string as
> the service name and the server saw a different path — hence the 404.
>
> The launcher now translates that form when importing, so `/<service>/Tun` gives
> the same request path Xray would produce. One caveat: the core percent-encodes
> `/` inside `service_name`, so a name with several segments (like yours) still
> can't be expressed exactly — we've filed that upstream. If you can, a
> single-segment `serviceName` (`/xxxxx/Tun`) will work in the meantime.

---

## 6. Итог: фикс снят, чинит ядро (18.09.2026, контракт 1.1.3)

Всё описанное выше — **история**. Заявка из §4 принята командой ядра:
вышло `sing-box-lx` **1.14.1-lx.8** (SPEC 093 ядра), и `service_name` с
ведущим «/» разбирается теперь как **custom path Xray** — путь уходит на
провод готовым, с **посегментным** экранированием и с отбрасыванием хвоста
`|…`:

- `/x/Tun` → на проводе `/x/Tun`;
- `/a/b/Tun` → на проводе `/a/b/Tun` (**многосегментное имя починено** —
  ограничение из §4 снято);
- `/a/Stream` → на проводе `/a/Stream` (произвольное имя потока);
- `a/b` (без ведущего «/») → прежнее поведение: имя сервиса, ядро
  дописывает `/Tun`, имя экранируется одним сегментом → `/a%2Fb/Tun`.

**Поэтому перевод формы снят целиком** (решение владельца 18.09.2026):
правило реестра `normalize: grpc_service_name`, значение в enum
`normalize` схемы, `registryNormalizeModes` в линтере и функция
`normalizeGRPCServiceName` в `core/config/nodeflow/coerce.go` вместе с её
юнитами — удалены. Значение передаётся ядру **как есть в любой форме**.

Оставлять перевод было нельзя: после lx.8 он стал бы **вредным** — снимал
бы «/» там, где ядро ждёт готовый путь, и возвращал бы 404 на узлах,
которые ядро теперь обслуживает правильно. Гейт `min_core` не вводится
(решение владельца): обе стороны пинят lx.8, а старому ядру такое значение
и раньше не помогало.

Кейсы корпуса из §3 перенормированы на «как есть» и дополнены четырьмя
новыми формами (`/x/Tun`, `/a/b/Tun`, `/a/Stream`, `a/b`): тело без
изменений, `warnings` пусто. Встречная задача LxBox — `TASKS_LXBOX.md`
§24.12; DRIFT §7.21. Черновики §4 и §5 (заявка в форк и ответ автору
issue) остаются как исторический след; в сам issue #130 по этой правке не
писали.
