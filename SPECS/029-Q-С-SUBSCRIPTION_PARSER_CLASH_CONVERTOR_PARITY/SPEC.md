# Предлагаемые доработки парсера подписок (sing-box)

## 1. Цель

Зафиксировать **узкий** набор улучшений для `core/config/subscription`: типичные варианты query и форматов `vmess://`, которые встречаются в подписках и должны корректно превращаться в outbound **sing-box**.

Ориентир — официальная документация sing-box (ниже — сверка по пунктам) и уже принятая у нас логика [**023**](../023-F-C-SUBSCRIPTION_TRANSPORT_VLESS_TROJAN/SPEC.md) (`uriTransportFromQuery`, `vlessTLSFromNode`, VMess JSON → `buildOutbound`). Форматы **Clash/mihomo YAML** не являются целью.

---

## 2. Сверка с документацией sing-box (актуальная схема)

Источники (sing-box.sagernet.org):

| Тема | Документ | Вывод для парсера |
|------|-----------|-------------------|
| Общий транспорт V2Ray | [V2Ray Transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/) | Допустимые значения **`transport.type`**: `http`, `ws`, `quic`, `grpc`, `httpupgrade`. Отдельного типа **`h2` нет** (в отличие от старых подписей VMess с `net=h2`). |
| HTTPUpgrade | тот же раздел, блок **HTTPUpgrade** | Поля: `type`, **`host`** (строка), `path`, `headers`. Соответствует нашему маппингу `type=xhttp` → `httpupgrade`. |
| WebSocket | тот же раздел, блок **WebSocket** | `path`, `headers` — подходит для заполнения `Host` из `host` / `sni` / `obfsParam`. |
| HTTP (plain / HTTP2-клиент в доке) | тот же раздел, блок **HTTP** | `host` — **массив** строк, `path`, `headers`, … TLS задаётся отдельно в `tls` outbound. |
| VLESS | [VLESS outbound](https://sing-box.sagernet.org/configuration/outbound/vless/) | Поле **`transport`** — та же [V2Ray Transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/). **`tls`** — [TLS outbound](https://sing-box.sagernet.org/configuration/shared/tls/#outbound): `server_name`, `insecure`, `utls`, `reality`, … |
| Trojan | [Trojan outbound](https://sing-box.sagernet.org/configuration/outbound/trojan/) | Аналогично: `transport` + `tls` как у VLESS. |
| VMess | [VMess outbound](https://sing-box.sagernet.org/configuration/outbound/vmess/) | **`transport`** — снова [V2Ray Transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/); значит для VMess допустимы те же типы, включая **`httpupgrade`**, а не выдуманный отдельный тип под Clash. |
| Hysteria2 | [Hysteria2 outbound](https://sing-box.sagernet.org/configuration/outbound/hysteria2/) | **`tls`** обязателен ([TLS outbound](https://sing-box.sagernet.org/configuration/shared/tls/#outbound)): поддерживаются **`insecure`**, **`utls.fingerprint`**, при необходимости **`certificate_public_key_sha256`** (закрепление по ключу сертификата, аналог pin в URI). |

При обновлении мажорной версии sing-box страницы выше нужно перепроверить.

---

## 3. Предлагаемые доработки

### P1

**3.1 VLESS и Trojan: `type=httpupgrade` в query**

- **Документация:** [V2Ray Transport — HTTPUpgrade](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/) — тип транспорта **`httpupgrade`**.
- **Смысл:** в подписках встречается и `type=xhttp`, и `type=httpupgrade`; оба должны давать один и тот же объект транспорта sing-box (`type: httpupgrade`, `host`, `path`, при необходимости `headers`).
- **Сейчас:** в [`uriTransportFromQuery`](../../core/config/subscription/node_parser_transport.go) нет ветки для строки `httpupgrade`.
- **Сделать:** обработать `httpupgrade` идентично `xhttp`; unit-тесты по образцу существующих для `xhttp`.
- **Файлы:** `node_parser_transport.go`, тесты.

**3.2 VLESS: псевдоним `peer` для TLS `server_name`**

- **Документация:** [TLS outbound — `server_name`](https://sing-box.sagernet.org/configuration/shared/tls/#outbound).
- **Смысл:** в URI часто дублируют SNI полем `peer`; в конфиг это всё равно уходит в **`tls.server_name`**.
- **Сейчас:** в [`vlessTLSFromNode`](../../core/config/subscription/node_parser_transport.go) fallback на `peer` не используется.
- **Сделать:** если `sni` пусто, брать `queryGetFold(q, "peer")` для будущего `server_name`; согласовать с эвристикой портов без TLS (023).

**3.3 VMess: после base64 — не JSON (альтернативная строка + query)**

- **Документация:** итоговый outbound всё равно [VMess](https://sing-box.sagernet.org/configuration/outbound/vmess/) + при необходимости [V2Ray Transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/) / [TLS](https://sing-box.sagernet.org/configuration/shared/tls/#outbound) — схема та же, меняется только **входной** разбор URI.
- **Смысл:** часть провайдеров кодирует в base64 не JSON, а строку вида `cipher:uuid@host:port` и опционально query.
- **Сейчас:** только `json.Unmarshal` → ошибка.
- **Сделать:** после неудачного JSON — второй путь разбора → `ParsedNode` / `Query` → существующий `buildOutbound`.
- **Файлы:** `node_parser_vmess.go`, ветка `vmess://` в `ParseNode`.

---

### P2

**3.4 VMess JSON: `net` = `httpupgrade` или `xhttp` → транспорт `httpupgrade`**

- **Документация:** [VMess — `transport`](https://sing-box.sagernet.org/configuration/outbound/vmess/) указывает на [V2Ray Transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/), где явно есть **`httpupgrade`** с полями `host` (string), `path`, `headers`.
- **Сейчас:** в [`parseVMessJSON`](../../core/config/subscription/node_parser_vmess.go) `xhttp` сводится к `ws`; в [`buildOutbound`](../../core/config/subscription/node_parser.go) для vmess получается **`ws`**, а не `httpupgrade` — расхождение со схемой sing-box для HTTP Upgrade.
- **Сделать:** для `net` ∈ {`httpupgrade`, `xhttp`} собирать **`transport.type: httpupgrade`** и переносить `path` / `host` из JSON (для `host` в httpupgrade — строка, не массив).

**3.5 VMess JSON: `net=h2`**

- **Документация:** в [списке транспортов](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/) **нет** типа `h2`. У HTTP-транспорта в доке описаны таймауты в контексте **HTTP/2 client** — то есть слой **`http`** используется и в сценариях с HTTP/2 при TLS.
- **Смысл:** устаревшие VMess-ссылки с `net=h2` обычно означают HTTP/2 поверх TLS, не отдельный «магический» тип в sing-box.
- **Сделать (рекомендация):** маппить `net=h2` на **`transport.type: http`** с `host` как **массив** строк (из поля `host` JSON, иначе из `add`/SNI по правилам VMess), `path` из `path`, плюс **`tls`** на outbound (как для существующего VMess с TLS). Если на практике встретятся несовместимые серверы — зафиксировать и при необходимости заменить на **явную ошибку** парсинга вместо немого TCP.
- **Альтернатива:** явный отказ с сообщением «net=h2: конвертируйте подписку или используйте узел вручную», пока нет тестового сервера.

**3.6 VLESS WebSocket: `obfsParam` как запасной Host**

- **Документация:** [WebSocket — `headers`](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/).
- **Сделать:** в `uriTransportFromQuery` для `type=ws`, если нет `host` и `sni`, использовать `obfsParam` для `headers.Host` (`queryGetFold`).

---

### P3

**3.7 Hysteria2: единая семантика insecure и опционально отпечаток / pin TLS**

- **Документация:** [Hysteria2 — поле `tls`](https://sing-box.sagernet.org/configuration/outbound/hysteria2/) → [TLS outbound](https://sing-box.sagernet.org/configuration/shared/tls/#outbound): **`insecure`**, **`utls`** с **`fingerprint`**, **`certificate_public_key_sha256`** (список base64 SHA-256 публичного ключа — для URI с pin / fingerprint в стиле SHA256).
- **Сейчас:** [`buildHysteria2TLS`](../../core/config/subscription/node_parser_hysteria2.go) не учитывает те же ключи, что [`tlsInsecureTrue`](../../core/config/subscription/node_parser_transport.go) (`allowInsecure` / `allowinsecure`).
- **Сделать:**  
  - Пробросить проверку insecure через общий хелпер с VLESS/Trojan **или** дублировать те же ключи query.  
  - При наличии `fp` / `fingerprint` в URI — выставлять `tls.utls.enabled` + `tls.utls.fingerprint` (как уже делается для VLESS через `normalizeUTLSFingerprint`).  
  - Параметр вроде `pinSHA256` в ссылках — по возможности маппить в **`certificate_public_key_sha256`** (массив строк в JSON), если формат в URI совместим с докой sing-box.

---

## 4. Принципы

- Только поля и типы из документации sing-box по ссылкам в §2; не подгонять конфиг под Clash.
- Каждое изменение — с **unit-тестами**; примеры из прод-подписок — обезличенные.
- Правки REALITY/TLS для VLESS не должны ломать контракт [**023**](../023-F-C-SUBSCRIPTION_TRANSPORT_VLESS_TROJAN/SPEC.md).

---

## 5. Критерии готовности (при взятии в работу)

- Закрыты выбранные пункты с тестами в `core/config/subscription`.
- При смене контракта query — `docs/ParserConfig.md` и при необходимости `docs/release_notes/upcoming.md`.
- Регрессий по тестам 023 и пайплайну подписок нет.

---

## 6. Порядок внедрения (рекомендация)

1. **A:** п. 3.1–3.2 (+ при желании 3.6) — `node_parser_transport.go` + тесты.
2. **B:** п. 3.3 — fallback VMess не-JSON.
3. **C:** п. 3.4–3.5 — VMess `httpupgrade`/`xhttp` и `h2` → `http` + TLS по §3.5.
4. **D:** п. 3.7 — Hysteria2 TLS.

---

## 7. Чеклист

- [x] Реализованы п. 3.1–3.7 (код + тесты в `core/config/subscription`).
- [x] `docs/ParserConfig.md`, `docs/release_notes/upcoming.md` обновлены.

---

## 8. Контекст исследования

Первичный просмотр стороннего конвертера URI → YAML ([clash-convertor](https://github.com/DikozImpact/clash-convertor), `script.js`) помог выписать варианты полей в URI; итоговые требования выше приведены к **официальной схеме sing-box** (§2).

---

## 9. Статус реализации

Выполнено в репозитории: **`uriTransportFromQuery`** (`httpupgrade`, `obfsParam`), **`vlessTLSFromNode`** / **`trojanTLSFromNode`** (`peer`), **`parseVMessDecoded`** + legacy + фрагмент до base64, **`parseVMessJSON`** (`httpupgrade`/`h2`), **`buildOutbound`** (vmess `httpupgrade`/`h2`, TLS `peer`/`tlsInsecureTrue`/fallback `server_name`), **`buildHysteria2TLS`** (insecure aliases, `utls`, `pinSHA256`).
