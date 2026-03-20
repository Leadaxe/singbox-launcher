# SPEC: Подписки — VLESS/Trojan transport и TLS по схеме sing-box

Задача: довести разбор типовых ссылок из подписок (VLESS с WS/gRPC/HTTP/xhttp, Trojan с WS, VMess gRPC) до **корректной генерации** outbound и `transport`/`tls` в формате [sing-box](https://sing-box.sagernet.org/), а не только успешного `ParseNode`.

---

## 1. Проблема

### 1.1 Текущее состояние (до работы)

- `ParseNode` извлекал host, port, query и строил `buildOutbound` для VLESS с **TLS всегда включённым**, без учёта `security=none`.
- Для VLESS **не собирался** блок `transport` из параметров `type`, `path`, `host`, `serviceName` — в `GenerateNodeJSON` секция `transport` добавлялась **только для VMess**.
- Для Trojan в outbound попадал в основном пароль; **WebSocket и TLS из query** не маппились в JSON.
- VMess с `net=grpc`: путь в JSON трактовался как `path` транспорта, тогда как в sing-box для gRPC ожидается **`service_name`**.
- Дублирующиеся фрагменты `#tag` давали одинаковые теги; в основном пайплайне уже был `MakeTagUnique`, но **превью подписки в визарде** — нет.

### 1.2 Цель

- Подписочные ссылки в стиле Xray/V2Ray (query-параметры) должны превращаться в конфиг sing-box, **совместимый с документацией**: [V2Ray Transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/), [TLS outbound](https://sing-box.sagernet.org/configuration/shared/tls/#outbound), outbounds [VLESS](https://sing-box.sagernet.org/configuration/outbound/vless/) / [Trojan](https://sing-box.sagernet.org/configuration/outbound/trojan/).

---

## 2. Требования

### 2.1 VLESS

- Собирать `transport` из query: `ws`, `http`, `grpc`, `xhttp` → для sing-box тип `httpupgrade` (без недокументированных полей вроде Xray-only `mode` в httpupgrade).
- `security=none` — **не** добавлять объект `tls` (или не включать шифрование там, где протокол без TLS).
- `pbk` (и связанные поля) — блок **Reality** в TLS; иначе обычный TLS с `sni`, `utls`, `alpn`, `insecure`/`allowInsecure` по query.
- Эвристика для пустого `security`: типичные «plaintext» порты (80, 8080, …) без TLS.

### 2.2 Trojan

- При `type=ws` (и при необходимости других транспортов из того же набора) — `transport` + `tls` из query (`sni`, `host`, `fp`, ALPN, insecure).

### 2.3 VMess

- Транспорт `grpc`: в JSON sing-box использовать **`service_name`**, заполняя из поля `path` VMess JSON.

### 2.4 Сериализация JSON

- `GenerateNodeJSON`: единая сборка `transport` для **vless, vmess, trojan**; для HTTP-транспорта поле `host` — **массив строк**, как в доке sing-box.

### 2.5 Визард

- При разборе подписки для превью применять **`MakeTagUnique`** к тегам нод (как в `LoadNodesFromSource`).

### 2.6 Критерии приёмки

- Тесты: VLESS ws + `security=none` без `tls` в итоговом JSON; gRPC с `service_name`; xhttp → `httpupgrade`; Trojan + WS + TLS.
- `go test` для пакетов `core/config/subscription`, `core/config`.
- Обновить `docs/release_notes/upcoming.md`.

### 2.7 Поля транспортов sing-box (документация)

Официально: [V2Ray Transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/). Ниже — **все поля** схемы по типу и **краткое назначение**; откуда в URI подписки — если маппинг есть в парсере 023.

| Тип | Назначение (по доке) | Поля схемы | Источник в подписке (VLESS/Trojan URI) |
|-----|----------------------|------------|----------------------------------------|
| **HTTP** | Plain HTTP 1.1 поверх TCP; TLS задаётся отдельно в `tls` outbound. | `type`, `host` ([]string), `path`, `method`, `headers`, `idle_timeout`, `ping_timeout` | `type=http` или **`type=raw`/`tcp` + `headerType=http`**: `path`, `host`; остальное не из URI |
| **WebSocket** | Транспорт WebSocket: путь и заголовки запроса. | `type`, `path`, `headers`, `max_early_data`, `early_data_header_name` | `type=ws`: `path`, `host` → `headers.Host` |
| **QUIC** | QUIC (без доп. полей в схеме). | `type` | `type=quic` (редко) |
| **gRPC** | gRPC; **service name** сервиса. | `type`, `service_name`, `idle_timeout`, `ping_timeout`, `permit_without_stream` | `type=grpc`: `serviceName` или `path` → `service_name` |
| **HTTPUpgrade** | HTTP Upgrade; в Xray в ссылках часто как `xhttp`. | `type`, `host` (string), `path`, `headers` | `type=xhttp`: `host`, `path`; **`mode` не переносится** (нет в схеме) |

**Расшифровка полей outbound VLESS** (`flow`, `packet_encoding`, `transport`, `tls` и т.д.) и **Trojan** — в **`SUBSCRIPTION_PARAMS_REPORT.md`**, § **Справочник**.

### 2.8 TLS outbound (клиент) — поля по доке

Официально: [TLS outbound](https://sing-box.sagernet.org/configuration/shared/tls/#outbound).

Полный перечень в доке включает: `enabled`, `disable_sni`, `server_name`, `insecure`, `alpn`, версии, шифры, кривые, сертификаты, `fragment`, `ech`, **`utls`**, **`reality`** и др.

**Кратко по используемым в подписках полям:**

| Поле | Назначение (по доке) |
|------|----------------------|
| `server_name` | Проверка имени на сертификате; в ClientHello для виртуального хостинга (если не IP). |
| `insecure` | Клиент: принимать любой сертификат сервера. |
| `alpn` | Список протоколов прикладного уровня по приоритету (ALPN). |
| `utls` | uTLS: имитация отпечатка ClientHello (`fingerprint`). |
| `reality` | Клиент Reality: `public_key`, `short_id` (и `enabled`). |

**Из URI подписок маппятся:** `sni` → `server_name`; `fp` → `utls.fingerprint` (нормализация регистра, напр. `QQ` → `qq`); `alpn` (в т.ч. после повторного URL-decode); `insecure` / `allowInsecure` / `allowinsecure` → `insecure`; `pbk` + `sid` → `reality.public_key`, `reality.short_id`.

**Не маппятся в JSON sing-box (нет в доке outbound reality для клиента):** `spx` и прочие Xray-расширения.

### 2.9 Отчёт по реальным подпискам

Сводная таблица примеров и ожидаемого JSON, **справочник полей sing-box с определениями из официальной доки**, полный перечень ключей query, а также **§ 4 — количественные результаты** (до/после, 415 строк по четырём URL, ~189 URI с исправленной семантикой outbound): **`SUBSCRIPTION_PARAMS_REPORT.md`** в этой папке.

---

## 3. Вне скоупа

- Перенос **Xray-only** параметров без аналога в sing-box (`spx`, `mode` для httpupgrade, `extra`, и т.д.) — URI по-прежнему **должен разбираться** без ошибки.
- Гарантия работы узла на конкретном сервере без учёта версии/сборки sing-box (например gRPC и build tags).

---

## 4. Ссылки

- [V2Ray Transport — sing-box](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/)
- [TLS (outbound) — sing-box](https://sing-box.sagernet.org/configuration/shared/tls/#outbound)
