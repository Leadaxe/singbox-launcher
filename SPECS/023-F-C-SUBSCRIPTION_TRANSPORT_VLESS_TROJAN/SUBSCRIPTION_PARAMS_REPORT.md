# Отчёт: параметры URI подписок → sing-box (задача 023)

Источники примеров (публичные списки, на момент анализа):

- [abvpn JSON](https://xray.abvpn.ru/vless/f4294d89-874b-4d9b-ab85-ddbc29bd87e2/126309188.json#abvpn) — VLESS: `tcp`+Reality, `ws`+TLS, `grpc`+Reality, `xhttp`+TLS.
- [goida-vpn-configs / 22.txt](https://raw.githubusercontent.com/AvenCores/goida-vpn-configs/refs/heads/main/githubmirror/22.txt) — VLESS/VMess/Trojan, в т.ч. `type=raw&headerType=http&security=none`.
- [igareck — WHITE](https://raw.githubusercontent.com/igareck/vpn-configs-for-russia/main/Vless-Reality-White-Lists-Rus-Mobile.txt) — Reality+TCP, xhttp+Reality, WS+TLS, `allowinsecure=0`, `fp=qq`, двойное кодирование `alpn`, `packetEncoding=xudp`, `?&security=tls`.
- [igareck — BLACK](https://raw.githubusercontent.com/igareck/vpn-configs-for-russia/main/BLACK_VLESS_RUS_mobile.txt) — xhttp+Reality, TCP+Reality+`spx`, WS+TLS с `?&` в query.

Ниже **§ Справочник** — краткие определения полей по [документации sing-box](https://sing-box.sagernet.org/); **§ 1** — как это стыкуется с реальными подписками.

---

## Дополнения после спецификации 029 (реализовано в парсере)

Сводка расширений под sing-box; детали и ссылки на доку sing-box — в **`SPECS/029-Q-С-SUBSCRIPTION_PARSER_CLASH_CONVERTOR_PARITY/SPEC.md`**. Обзор URI, Share URI и пайплайн ParserConfig — **`docs/ParserConfig.md`**.

| Тема | Поведение |
|------|-----------|
| VLESS/Trojan `type` | **`httpupgrade`** в query — синоним **`xhttp`**, в outbound transport `type: httpupgrade`. |
| VLESS/Trojan TLS `server_name` | VLESS: `sni` → **`peer`** → адрес сервера. Trojan: `sni` → **`peer`** → **`host`** → адрес сервера. |
| VLESS WS `Host` | После `host` и `sni` — fallback на **`obfsParam`**. |
| VMess base64 | Сначала JSON; иначе legacy **`cipher:uuid@host:port`** + опциональный `?query`; **`#fragment`** отрезается до base64. |
| VMess JSON `net` | **`xhttp`/`httpupgrade`** → transport `httpupgrade`; **`h2`** → transport `http` + TLS; при `h2` без `tls` в JSON TLS включается по умолчанию. |
| VMess TLS в outbound | `server_name`: `sni` → **`peer`** → адрес сервера (`add`); insecure как у VLESS. |
| Hysteria2 `tls` | **`allowInsecure`/`allowinsecure`**; **`fingerprint`/`fp`** → `utls`; **`pinSHA256`** → `certificate_public_key_sha256`. |

---

## Справочник: поля sing-box и связь с URI подписки (Xray/V2Ray style)

Источники определений: [VLESS outbound](https://sing-box.sagernet.org/configuration/outbound/vless/), [Trojan outbound](https://sing-box.sagernet.org/configuration/outbound/trojan/), [TLS (outbound)](https://sing-box.sagernet.org/configuration/shared/tls/#outbound), [V2Ray Transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/).

### Outbound VLESS

| Поле JSON | По доке sing-box | Откуда в типичной подписке (`vless://…`) |
|-----------|-------------------|------------------------------------------|
| `server` | Адрес сервера (обязательно). | Host в authority (`user@host:port`). |
| `server_port` | Порт (обязательно). | Порт в authority; иначе 443. |
| `uuid` | VLESS user id (обязательно). | Username в authority (UUID). |
| `flow` | Подпротокол VLESS; для клиента в т.ч. `xtls-rprx-vision`. | Query `flow`. |
| `network` | Доступные сети: `tcp` / `udp`; по умолчанию включены оба. | В URI подписок обычно не задаётся. |
| `tls` | Конфигурация TLS, см. общий блок TLS. | Собирается из `security`, `sni`, `fp`, `alpn`, `pbk`, `sid`, `insecure` / `allowInsecure` и т.д. |
| `packet_encoding` | Кодирование UDP: `(none)` — выкл.; `packetaddr` — v2ray 5+; `xudp` — xray. | Query `packetEncoding` (лаунчер переносит в это поле). |
| `transport` | Объект [V2Ray Transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/) поверх TCP. | Query `type`, `path`, `host`, `serviceName`, `headerType` и др. |

Поля `multiplex`, dial-опции и т.п. в стандартной share-link подписке **не задаются** URI — при необходимости остаются пустыми / по умолчанию.

### Outbound Trojan

| Поле JSON | По доке sing-box | Откуда в подписке (`trojan://…`) |
|-----------|-------------------|-----------------------------------|
| `server` / `server_port` | Адрес и порт (обязательно). | Authority. |
| `password` | Пароль Trojan (обязательно). | Username или пароль в userinfo (как принято в ссылке). |
| `network` | `tcp` / `udp`. | Редко в query. |
| `tls` | TLS outbound. | `security`, `sni`, `host`, `fp`, `alpn`, `insecure` и т.д. |
| `transport` | V2Ray Transport. | Те же правила, что для VLESS (`type=ws` и др.). |

### TLS (outbound, клиент) — используемые поля

| Поле | По доке (смысл) | Типичный query в подписке |
|------|------------------|---------------------------|
| `enabled` | Включение TLS. | Логика: `security=none` → TLS не добавляем; иначе `true` (Trojan при `security=none` — явный `enabled: false`). |
| `server_name` | Имя хоста для проверки сертификата и SNI (если не IP). | VLESS: `sni` → **`peer`** → адрес сервера. Trojan: `sni` → **`peer`** → **`host`** → адрес сервера. VMess (при TLS): см. **`docs/ParserConfig.md`** (`sni` → `peer` → `add`). |
| `insecure` | Принимать любой сертификат сервера. | `insecure` / `allowInsecure` / `allowinsecure` = `1` / `true` / `yes`. |
| `alpn` | Список ALPN, порядок предпочтений. | `alpn` (список через запятую; в лаунчере — нормализация percent-encoding). |
| `utls` | Клиентский uTLS: `fingerprint` имитирует ClientHello. | `fp` → `utls.fingerprint` (значения: `chrome`, `firefox`, `qq`, `random`, … — см. [доку TLS](https://sing-box.sagernet.org/configuration/shared/tls/#outbound)). |
| `reality` | `enabled`, `public_key`, `short_id` (клиент Reality). | `pbk`, `sid` при наличии `pbk`; иначе при `security=reality` без ключа — только обычный TLS без блока reality (краевой случай). |

Поля вроде `fragment`, `ech`, `min_version`, сертификаты и т.д. **в подписках по URI обычно не передаются** — не маппятся парсером 023.

### V2Ray Transport — назначение полей по типам

Сводка по [официальной таблице типов](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/):

| `transport.type` | Назначение (по доке) | Основные поля схемы |
|------------------|----------------------|----------------------|
| `http` | Plain HTTP 1.1 поверх TCP; TLS не обязателен (задаётся отдельно в `tls`). | `host` ([]), `path`, `method`, `headers`, `idle_timeout`, `ping_timeout`. |
| `ws` | WebSocket: путь запроса и заголовки. | `path`, `headers`, `max_early_data`, `early_data_header_name`. |
| `grpc` | gRPC transport; имя сервиса. | `service_name`, `idle_timeout`, `ping_timeout`, `permit_without_stream`. |
| `httpupgrade` | HTTP Upgrade (в подписках Xray часто подписан как `xhttp`). | `host` (строка), `path`, `headers`. |
| `quic` | QUIC (без доп. полей в схеме). | Только `type`. |

В подписке Xray поля **`type=xhttp`** и **`type=httpupgrade`** маппятся в sing-box в **`httpupgrade`**; параметр **`mode`** из Xray **отсутствует** в документированной схеме `httpupgrade` и **не переносится**.

### Параметры URI без поля в sing-box (Xray-специфика)

Используются генераторами ссылок, но в [TLS outbound](https://sing-box.sagernet.org/configuration/shared/tls/#outbound) / transport **нет** соответствующего поля для клиента: `spx` (Reality), `mode` (xhttp), `extra`, `quicSecurity`, `authority` и т.п. — сохраняются в `ParsedNode.Query`, в генерируемый JSON outbound **не попадают**.

---

## 1. Комбинации query-параметров (VLESS) из подписок


| Набор                                                                           | Пример из подписки | Ожидаемый результат (sing-box outbound)                                                                                                                      |
| ------------------------------------------------------------------------------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `type=ws`, `path`, `host`, `security=tls`, `sni`, `fp`                          | abvpn, igareck     | `transport.type=ws`, `headers.Host`, `tls` с `server_name`, `utls.fingerprint`                                                                               |
| `type=ws`, `security=none`, порт 80                                             | goida              | `transport` ws, блок `tls` отсутствует                                                                                                                       |
| `type=grpc`, `serviceName`, `security=reality`, `pbk`, `sid`, `sni`, `fp`       | abvpn              | `transport.type=grpc`, `service_name`; `tls.reality` + `utls`                                                                                                |
| `type=xhttp`, `path`, `host`, `mode=…`, `security=tls`                          | abvpn              | `transport.type=httpupgrade`, `host`/`path`; без поля `mode` (нет в [доке httpupgrade](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/)) |
| `type=xhttp`, `security=reality`, `pbk`, `sid`, `sni`                           | igareck BLACK      | `httpupgrade` + `tls` с `reality`                                                                                                                            |
| `type=tcp` или `raw`, `security=reality`, `flow=xtls-rprx-vision`, `pbk`, `sid` | все списки         | без `transport` (plain TCP); `tls` + reality                                                                                                                 |
| `type=raw`, `headerType=http`, `host`, `path`, `security=none`                  | goida              | `transport.type=http`, `host: []string{…}`, `path`; без `tls`                                                                                                |
| `allowinsecure=0` (нижний регистр)                                              | igareck WHITE      | `tls.insecure` **не** `true`                                                                                                                                 |
| `insecure=1` на WS+TLS                                                          | igareck BLACK      | `tls.insecure: true`                                                                                                                                         |
| `alpn=http%25252F1.1` (многослойный percent-encode)                             | igareck WHITE      | после нормализации `alpn: ["http/1.1"]`                                                                                                                      |
| `fp=qq` / `fp=QQ`                                                               | igareck, abvpn     | `utls.fingerprint: "qq"` (нижний регистр, [список utls](https://sing-box.sagernet.org/configuration/shared/tls/#outbound))                                   |
| `packetEncoding=xudp`                                                           | igareck WHITE      | `packet_encoding: "xudp"` в outbound                                                                                                                         |
| query вида `?&security=tls&…`                                                   | igareck BLACK      | корректный разбор; присутствуют `transport` и `tls`                                                                                                          |


Параметры, **встречающиеся в подписках**, но **без переноса в JSON sing-box** (остаются только в `node.Query`, разбор URI не ломают): `spx`, `extra`, `mode` (xhttp), `quicSecurity`, `authority` (gRPC в Xray/Clash), пустые служебные значения. **`packetEncoding`** наоборот **маппится** в `packet_encoding` outbound.

---

## 1а. Все уникальные ключи query в четырёх источниках (VLESS/Trojan)

Сводка по объединению ключей из abvpn JSON, goida `22.txt`, igareck WHITE и BLACK (регистр как в файлах; парсер для транспорта и TLS использует **регистронезависимый** поиск там, где это важно: `host`/`Host`, `allowInsecure`/`allowinsecure`, и т.д.). Подробные определения целевых полей sing-box — в **§ Справочник** выше.

| Ключ (варианты) | Куда в лаунчере / sing-box |
|-----------------|----------------------------|
| `type` | транспорт (`ws`, `http`, `grpc`, `httpupgrade`, `tcp`/`raw` без transport) |
| `path` | `path` или `service_name` (grpc fallback) |
| `host`, `Host` | заголовок Host / список host для `http` / `httpupgrade` |
| `headerType` | вместе с `type=raw`/`tcp` → транспорт `http` |
| `serviceName`, `service_name` | `grpc.service_name` |
| `security` | TLS: `none` / TLS / reality (с `pbk`) |
| `sni` | `tls.server_name` |
| `fp` | `tls.utls.fingerprint` (нормализация регистра) |
| `alpn` | `tls.alpn[]` (с нормализацией percent-encoding) |
| `pbk`, `sid` | `tls.reality.public_key`, `short_id` |
| `insecure`, `allowInsecure`, `allowinsecure` | `tls.insecure` при истинном значении |
| `flow` | `outbound.flow` (поле VLESS) |
| `encryption` | только в `node.Query` (типично `none` для VLESS) |
| `packetEncoding` | `outbound.packet_encoding` |
| `allowInsecure` (см. выше) | дублирует семантику `insecure` для TLS |
| `mode` | не переносится (Xray xhttp; в схеме sing-box `httpupgrade` нет) |
| `spx` | не переносится (Xray Reality; в outbound TLS sing-box нет поля) |
| `extra` | не переносится |
| `quicSecurity` | не переносится (артефакт генераторов ссылок) |
| `authority` | не переносится (в JSON sing-box для клиента не используем) |

Это **не** «все возможные ключи во всём интернете» — только то, что встретилось в указанных четырёх подписках. Другие панели могут добавлять свои параметры; они сохранятся в `ParsedNode.Query`, но пока не маппятся в outbound.

---

## 2. Trojan / VMess (кратко)

- **Trojan:** `type=ws` + `security=tls` — как VLESS ws + TLS (`transport` + `tls`), `sni`/`host`/`fp`/`alpn`/`insecure` через те же правила нормализации ключей.
- **VMess:** не URI-query, а base64 JSON; для `net=grpc` путь в JSON мапится в `service_name` (см. SPEC).

---

## 3. Тесты в коде

Покрытие соответствует строкам из таблицы: `TestParseNode_VLESS_TransportAndTLS` (подкейсы: `allowinsecure=0`+`fp=qq`, multiply-encoded `alpn`, `packetEncoding`, `raw`+`headerType=http`, `?&security=tls`, abvpn-style grpc+reality, xhttp+reality).

Полный прогон **всех** строк `vless`/`trojan`/`vmess` из четырёх URL выше (без коммита в CI по умолчанию):

`go test -tags=live ./core/config/subscription/... -run TestLiveParsePublicSubscriptionFiles -count=1`

На момент проверки live-тест даёт **415** строк протоколов (**0** ошибок `ParseNode`) по четырём URL выше.

---

## 4. Результаты: до / после и объёмы по четырём спискам

### 4.1. Ошибки `ParseNode` (буквальный «не парсится»)

На коммите **до** появления `node_parser_transport.go` и правок `buildOutbound` (например `3d5c51b`) те же четыре URL прогонялись счётчиком: для каждой строки `vless://` / `trojan://` / `vmess://` вызывался `ParseNode`.

| Метрика | Значение |
|--------|----------|
| Всего строк протоколов | **415** |
| Ошибок разбора URI | **0** |

Вывод: в этих подписках узлы **и раньше создавались**; проблема была в **семантике outbound** для sing-box (и в части случаев — в потере значений из‑за регистра ключей в query), а не в падении парсера.

### 4.2. Что было неверно до правок (outbound)

| Ситуация | До правок | После правок |
|----------|-----------|--------------|
| VLESS `security=none` | Всё равно собирался `tls` | Блок `tls` отсутствует |
| VLESS `type=ws` / `grpc` / `xhttp` / `http` | `transport` из query **не** собирался | `transport` по [доке](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/) |
| VLESS `raw`/`tcp` + `headerType=http` | Обрабатывалось как plain TCP без HTTP-транспорта | `transport.type=http`, `host` списком |
| Trojan + `type=ws` | Только `password` | `transport` + `tls` |
| `allowinsecure` / `Host` / и т.п. | `url.Values.Get` — регистрозависимо | `queryGetFold` где нужно |
| Многослойный percent в `alpn` | Некорректное значение в TLS | Нормализация decode |
| `fp=QQ` | Как есть | → `qq` для utls |
| `packetEncoding` | Не в outbound | → `packet_encoding` |

Параметры без аналога в документированном клиентском TLS/transport (`spx`, `mode`, `quicSecurity`, `authority`, …) по-прежнему **не попадают в JSON**, но **не мешают** успешному `ParseNode`.

### 4.3. Объёмы по файлам (категории по query; суммы по файлам, строки могут входить в несколько категорий)

Подсчёт по **скачанным** копиям четырёх источников (скрипт классификации по `type` / `security` / `headerType`).

| Источник | Всего строк `vless`+`trojan`+`vmess` | `vmess` | VLESS `security=none` | VLESS с `type` ∈ {ws, grpc, xhttp, http} | VLESS `raw`/`tcp` + `headerType=http` | Trojan `type=ws` |
|----------|-------------------------------------|---------|------------------------|------------------------------------------|----------------------------------------|------------------|
| abvpn JSON | 18 | 0 | 0 | 13 | 0 | 0 |
| goida `22.txt` | 257 | 59 | 29 | 79 | 4 | 3 |
| igareck WHITE | 70 | 0 | 0 | 34 | 0 | 2 |
| igareck BLACK | 70 | 0 | 0 | 49 | 0 | 2 |
| **Σ** | **415** | **59** | **29** | **175** | **4** | **7** |

### 4.4. Уникальные URI и «верные» outbound

- **Глобально уникальных VLESS**, для которых нужны новые правила (**`security=none`** **или** явный транспорт в query **или** `raw`/`tcp`+`headerType=http`): **182** (дедупликация между четырьмя файлами).
- **Уникальных Trojan + WebSocket:** **7**.

Итого порядка **189** прокси-строк, у которых **до** правок конфигурация для sing-box была **системно неполной или неверной** (transport/TLS/plain). Остальные VLESS в основном «чистый TCP + TLS/Reality» — старый код был ближе к целевому виду, но без нормализации ключей/`alpn`/`fp`.

**VMess** в срезе goida `22.txt`: **59** строк; распределение `net`: **ws** и **raw**, **`net=grpc` в этом снимке не встретился** — правка `service_name` для gRPC на этих четырёх файлах **не проявляется** по счётчику, но остаётся в SPEC для других подписок.

### 4.5. Что считать «верным соединением»

Здесь «верно» означает: **для URI собран outbound с ожидаемыми полями `transport`/`tls` по документации sing-box**, а не гарантию живого сервера или успешного рукопожатия (это сеть, политика, версия sing-box).

---

## 5. Ссылки на документацию sing-box и проект

- [V2Ray Transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/)
- [TLS outbound](https://sing-box.sagernet.org/configuration/shared/tls/#outbound)
- [VLESS outbound](https://sing-box.sagernet.org/configuration/outbound/vless/)
- [Trojan outbound](https://sing-box.sagernet.org/configuration/outbound/trojan/)
- [VMess outbound](https://sing-box.sagernet.org/configuration/outbound/vmess/)
- **`docs/ParserConfig.md`** — прямые ссылки, Share URI, маркеры ParserConfig
- **`SPECS/029-Q-С-SUBSCRIPTION_PARSER_CLASH_CONVERTOR_PARITY/SPEC.md`** — расширения парсера 029


