## IMPLEMENTATION REPORT — 023-F-C-SUBSCRIPTION_TRANSPORT_VLESS_TROJAN

- **Status:** Completed  
- **Date completed:** 2026-03-20  

### 1. Кратко

Исправлена генерация sing-box outbound для типовых ссылок из подписок: у **VLESS** добавлены **`transport`** (ws / http / grpc / xhttp→httpupgrade) и **условный TLS** (`security=none` без TLS, Reality по `pbk`, иначе TLS с alpn/insecure/utls). У **Trojan** — transport (например WS) и **tls** из query. У **VMess gRPC** в транспорт попадает **`service_name`** из JSON-поля `path`. Превью подписки в визарде дедуплицирует теги через **`MakeTagUnique`**. После сверки с [документацией sing-box](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/) уточнёны поля: для `http` transport — **`host` как массив**; для `httpupgrade` не передаётся Xray-поле **`mode`**; из JSON не эмитится недокументированный **`authority`**.

### 2. Что сделано и как

**Новый модуль `core/config/subscription/node_parser_transport.go`**

- `uriTransportFromQuery(q)` — по `type` из query строит map транспорта sing-box:
  - `ws` — `path`, `headers.Host`;
  - `http` — `path`, **`host: []string{…}`** (как в доке HTTP transport);
  - `grpc` — `service_name` из `serviceName` или `path`;
  - `xhttp` — **`type: httpupgrade`**, только `host` (string), `path` (без `mode`).
- `vlessTLSFromNode` — при `security=none` возвращает «без TLS»; при непустом `pbk` — TLS + **reality**; при `security=reality` без ключа — обычный TLS; при пустом `security` и «plaintext» портах — без TLS; иначе TLS + `applyTLSQueryExtras` (alpn, insecure/allowInsecure).
- `trojanTLSFromNode` — `security=none` → `tls.enabled: false`; иначе TLS с `server_name` (sni/host/server), опционально `utls` по `fp`.

**`core/config/subscription/node_parser.go`**

- Ветка **VLESS**: после flow — заполнение `transport` и при необходимости `tls` через функции выше (вместо «TLS всегда on»).
- Ветка **Trojan**: пароль + `transport` + `tls`.
- Ветка **VMess** `buildOutbound`: для `grpc` — `service_name` из query `path`; для `http` — `host` как `[]string`; для `ws` — по-прежнему `headers` с Host.

**`core/config/outbound_generator.go`**

- Функция **`appendOutboundTransportParts`**: сериализует `type`, `path`, `host` (string **или** `[]string` через `json.Marshal`), `service_name`, `headers`.
- Вызов после `flow` / `packet_encoding`, до блока `tls`.
- Если в map TLS **`enabled: false`** — в JSON выводится компактный объект без лишних полей.
- `server_name` в tls не пишется, если пустая строка.

**`ui/wizard/tabs/source_tab.go`**

- В **`fetchAndParseSource`** для каждой успешно распарсенной ноды: `node.Tag = subscription.MakeTagUnique(...)`, общий `tagCounts` на один вызов.

**Тесты**

- `TestParseNode_VLESS_TransportAndTLS` (ws без tls, grpc+tls, xhttp→httpupgrade без mode).
- `TestParseNode_Trojan_WebSocket`.
- `TestParseNode_VLESS_TransportAndTLS` — подкейс `type=http` с host-list.
- `TestGenerateNodeJSON_VLESS_WSTransportNoTLS` в `generator_test.go`.

**Документация:** `docs/release_notes/upcoming.md` (EN/RU).

### 3. Проверки

- [x] `go test ./core/config/subscription/...`
- [x] `go test ./core/config/...` (включая GenerateNodeJSON)
- [x] `go vet` по затронутым пакетам

### 4. Ограничения

- Полный паритет с Xray по всем query-параметрам не заявляется.
- **`mode`** из Xray xhttp в sing-box **httpupgrade** по официальной схеме не переносится.
- **`spx`** и аналогичные Xray-Reality поля в документированный outbound TLS sing-box не переносятся.
- **gRPC**: в документации sing-box указана возможность сборки без standard gRPC — на стороне пользователя должна быть подходящая сборка sing-box.

### 5. Дополнение (нормализация query и отчёт по подпискам)

- Количественные результаты сравнения **до/после**, таблицы по четырём публичным спискам и определение «верного» outbound — в **`SUBSCRIPTION_PARAMS_REPORT.md`**, § **4. Результаты: до / после и объёмы по четырём спискам**.
- Справочник **полей sing-box** (определения из официальной доки) и таблица **URI → JSON** — в **`SUBSCRIPTION_PARAMS_REPORT.md`**, § **Справочник**; **`docs/ParserConfig.md`** (разделы VLESS/Trojan) обновлены со ссылкой на отчёт и расширенным списком query-параметров.
- **`queryGetFold`**: чтение `allowinsecure`, `alpn`, `fp`, `sni`, `security`, `pbk`, `sid` без учёта регистра ключа (как в igareck WHITE).
- **`normalizePercentDecodeLoop`** для `alpn` (цепочки `http%25252F1.1` → `http/1.1`).
- **`normalizeUTLSFingerprint`**: `QQ` → `qq` и т.п.
- **`tcp`/`raw` + `headerType=http`**: transport `http` с `host` как `[]string` (строки из goida).
- **`packetEncoding`** из query → `packet_encoding` в outbound VLESS.
- Документ **`SUBSCRIPTION_PARAMS_REPORT.md`**: примеры из abvpn, goida, igareck и ожидаемый sing-box; в **SPEC.md** — таблицы всех полей транспортов/TLS по официальной доке.
- Тесты: дополнительные подкейсы в `TestParseNode_VLESS_TransportAndTLS`.

### 6. Заметка по имени папки

Папка: **`023-F-C-SUBSCRIPTION_TRANSPORT_VLESS_TROJAN`** — статус **C (Complete)**.
