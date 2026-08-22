# 102-B — XHTTP `extra` теряется в Xray-JSON ветке парсера

**Тип:** Bug · **Статус:** New · **Область:** `core/config/subscription`

---

## 1. Проблема

Подписки, отдающие **готовый Xray-конфиг (JSON-массив профилей)** вместо
base64-списка share-URI, теряют весь блок `streamSettings.xhttpSettings.extra`.
Узлы с XHTTP собираются в урезанный транспорт и не работают: сервер отвергает
HTTP-запрос XHTTP-слоя.

Ошибки воспроизводятся стабильно (100 % прогонов, `URLTestOutbound`):

| Узел | Ошибка ядра |
|------|-------------|
| `46.243.142.42:9443` (mode `stream-one`) | `v2ray-xhttp: unexpected status: 400 Bad Request` |
| `95.163.232.194:8444` (mode `stream-one`) | `v2ray-xhttp: unexpected status: 400 Bad Request` |
| `188.72.103.4:443` (mode `packet-up`) | `v2ray-xhttp: unexpected upload status: 400 Bad Request` |

TCP-порт открыт, REALITY/TLS-рукопожатие проходит — отвергается именно
HTTP-запрос XHTTP. В Xray те же узлы работают.

### 1.1 Причина

В репозитории **два независимых пути** разбора XHTTP, и они разошлись:

| Путь | Файл | Что умеет |
|------|------|-----------|
| share-URI (`vless://…?extra=…`) | `node_parser_transport.go:236` `xhttpTransportFromQuery` | полный набор SPEC 002 v2: `extra`-JSON, `uplink_http_method`, `x_padding_bytes`, sc-поля, placement/key-поля |
| Xray-JSON | `xray_outbound_convert.go:282` `xrayTransportFromStreamSettings` | **только** `path`, `host`, `mode` |

Xray-ветку добавила задача **094 (C5)** — она закрыла «транспорт вообще не
распознаётся», но перенесла лишь базовое трио. `extra` не читается.

Эмиттер и ядро при этом поля **не теряют**: `uplink_http_method` уже в
allowlist `xhttpV2StringKeys` (`outbound_jsonbuilder.go:20`), `x_padding_bytes`
эмитится отдельно (`:135`). То есть дефект целиком на стороне конвертера.

### 1.2 Ключевые расхождения на реальных узлах

**`188.72.103.4` (`packet-up`)** — причина 400 установлена точно:

```json
"extra": { "uplinkHTTPMethod": "GET", "scMaxBufferedPosts": 30,
           "scMaxEachPostBytes": "1000000", "scStreamUpServerSecs": "20-80",
           "xPaddingBytes": "0-0" }
```

Сервер ждёт uplink методом **GET**; sing-box в `packet-up` шлёт **POST** →
`unexpected upload status: 400`.

**`46.243.142.42` / `95.163.232.194` (`stream-one`)**:

```json
"extra": { "xPaddingBytes": "50-150",
           "xmux": { "maxConcurrency": "16-32", "cMaxReuseTimes": "256-512",
                     "maxConnections": 0, "hKeepAlivePeriod": 0,
                     "hMaxReusableSecs": "900-1500" } }
```

Теряется `x_padding_bytes` — вероятная причина 400 (ядро подставляет свой
padding, которого сервер не ждёт).

Отдельно показателен `xPaddingBytes: "0-0"` — сервер требует padding
**отключить**; выразить это в текущем конфиге нечем.

---

## 2. Требования

**R1.** Ветка `xhttp`/`splithttp` в `xrayTransportFromStreamSettings` читает
`xhttpSettings.extra` и переносит его поля в транспорт под теми же snake_case
именами, что уже понимают эмиттер и ядро.

**R2.** Набор полей и их имена — **тот же**, что в URI-ветке (единый источник
истины). Расхождение схем между двумя ветками не допускается: добавление поля
в одну ветку обязано покрывать обе.

**R3.** Нормализация значений — как в URI-ветке. В Xray-JSON часть значений
приходит числами (`scMaxBufferedPosts: 30`, `uplinkChunkSize: 0`), а эмиттер
ждёт строки; sc-диапазоны нормализуются к форме `"min-max"`.

**R4.** Плоские поля `xhttpSettings` (вне `extra`) читаются тоже — Xray
допускает обе раскладки. При конфликте **`extra` выигрывает** (SPEC 002 §1.5,
поведение URI-ветки).

**R5.** `xPaddingBytes: "0-0"` доходит до конфига дословно — это осмысленное
значение (отключение padding), а не «пусто».

**R6.** Деградация вместо поломки: нераспознанный или битый `extra` не роняет
узел и не пишет мусор в конфиг — узел собирается без этих полей
(поведение по аналогии с `broken-list-pbk-junk`).

**R7.** `xmux` — вне scope: в allowlist эмиттера его нет, поддержка ядром не
подтверждена. Явно зафиксировать как отложенное, не эмитить полумеру.

---

## 3. Критерии приёмки

1. Xray-элемент с `xhttpSettings.extra.uplinkHTTPMethod: "GET"` даёт транспорт
   с `"uplink_http_method": "GET"`.
2. `xPaddingBytes: "50-150"` и `"0-0"` доходят до `x_padding_bytes` дословно.
3. Числовые значения из `extra` (`scMaxBufferedPosts: 30`) попадают в конфиг
   строками (`"30"`), без `1e+06`-нотации.
4. Плоское поле `xhttpSettings` подхватывается; одноимённое поле в `extra`
   его перекрывает.
5. Битый / не-объектный `extra` не роняет разбор узла.
6. Набор поддержанных ключей в Xray-ветке совпадает с URI-веткой —
   зафиксировано тестом, падающим при расхождении.
7. Три узла из §1 проходят `URLTestOutbound` без ошибки 400.
8. `go build ./...`, `go test ./...`, `go vet ./...` — чисто.

---

## 4. Вне scope

- `xmux` (R7) — отдельная задача после подтверждения поддержки ядром.
- Прочие транспорты Xray-ветки (`ws`/`grpc`/`http`) — не трогаем.
- Поведение ping-all / массового тестирования — отдельный дефект,
  обнаружен в той же диагностике, здесь не решается.
