# SPEC 073-F-N — ПОДДЕРЖКА ПАРАМЕТРОВ AMNEZIAWG (AWG 2.0)

## Цель

Научить лаунчер сквозному проходу obfuscation-параметров **AmneziaWG 2.0** (далее AWG) — от subscription-URI / вставленного config.json до сгенерированного `config.json` и обратно (share-URI). Параметры (junk-пакеты `jc`/`jmin`/`jmax`, handshake-junk `s1`–`s4`, magic-заголовки `h1`–`h4`, CPS-пакеты `i1`–`i5`) должны парситься, переноситься через outbound-модель **без потерь** и эмититься в WireGuard-endpoint `config.json` ровно в том shape, который понимает форк sing-box-lx (`with_awg`).

Зачем: форк `github.com/Leadaxe/sing-box-lx` (релиз `v1.13.13-lx.1`, ветка `lx`) уже умеет AWG 2.0 — фича live-validated против реального AWG2-сервера (handshake + keepalive + outbound traffic с `Jc=10`/`Jmin=50`/`Jmax=100`, `S1`–`S4`, `H1`–`H4`, `I1`–`I3`). Но лаунчер сейчас никак не знает про эти поля: `parseWireGuardURI` строит endpoint только из канонических WG-полей, а `config.json` со вставленными AWG-параметрами проходит через модель, но AWG-поля не парсятся из URI и не эмитятся обратно в share-URI. Без этой SPEC пользователь форк-ядра не может задать AWG-узел через подписку или wizard — только руками в Raw JSON, что теряется при любом round-trip через парсер.

## Контекст

**Текущее состояние (greenfield по AWG — `grep -ri "awg\|amneziawg" core/ ui/` пусто):**

* WireGuard — особый случай: парсится не в `outbounds[]`, а в sing-box `endpoints[]`. Роутер `ParseNode` (`core/config/subscription/node_parser_core.go:195`) по префиксу `wireguard://` уходит в `parseWireGuardURI` (`core/config/subscription/node_parser_wireguard.go:18`). `IsDirectLink` (`node_parser_core.go:20-31`) перечисляет известные схемы.
* `parseWireGuardURI` строит **flat endpoint map** (`endpoint` — `node_parser_wireguard.go:131-143`) с полями `type`/`tag`/`name`/`system`/`mtu`/`address`/`private_key`/`peers[]` и **отдельный `peer` map** (`node_parser_wireguard.go:114-129`) с `address`/`port`/`public_key`/`allowed_ips`/`persistent_keepalive_interval`/`pre_shared_key`. AWG-полей среди них нет — они молча отбрасываются, даже если присутствуют в URI.
* Эмиссия в `config.json`: `GenerateEndpointJSON` (`core/config/outbound_generator.go:494-517`) делает **generic-копию** `for k, v := range node.Outbound { endpoint[k] = v }` и `json.MarshalIndent`. То есть **любое поле, положенное в `node.Outbound` парсером, эмитится автоматически** — целевой shape AWG (поля промотнуты в корень endpoint) совпадает с этим механизмом. Это ключевая точка переиспользования: генератор править почти не нужно.
* Обратная конвертация: `ShareURIFromWireGuardEndpoint` (`core/config/subscription/shareuri_wireguard.go:17-86`) собирает `wireguard://` из endpoint-объекта. Он эмитит только канонические WG query-params (`publickey`/`address`/`allowedips`/`mtu`/`keepalive`/`presharedkey`/`listenport`/`name`/`dns`) — AWG-поля теряются при share.
* Модель **schema-permissive**: `ParsedNode.Outbound` — `map[string]interface{}`, проброс неизвестных полей возможен. Но конкретно WireGuard-путь **не permissive**: и парсер (явный список ключей), и share-URI (явный список query) перечисляют поля поимённо. Поэтому AWG требует **явного расширения** обоих, не «само пройдёт».
* Edit-диалог outbound'ов (`ui/configurator/outbounds_configurator/edit_dialog.go`) имеет три вкладки (`edit_dialog.go:902-905`): **Settings** (форма: tag/type/comment/filters/options), **Raw** (редактируемый JSON outbound-объекта, `edit_dialog.go:277`), **Preview** (read-only). Per-field форма для transport/endpoint-внутренностей **отсутствует** — AWG-поля сегодня редактируются только через Raw JSON.
* Core-пин: `RequiredCoreVersion = "1.13.12"` (`internal/constants/constants.go:61`) — **upstream SagerNet, без AWG**. Форк с AWG — `v1.13.13-lx.1`.

**Форк-возможность (что уже готово в ядре, статус «C» — complete):**

AWG 2.0 в `sing-box-lx` — расширение WireGuard-endpoint'а DPI-evasion-полями, промотнутыми в корень endpoint (не в peer, не вложенный obfs-map). Типы строго: `jc`/`jmin`/`jmax`/`s1`–`s4`/`h1`–`h4` — `uint32` (JSON number); `i1`–`i5` — строки с **case-sensitive** тег-форматом (`<b 0xHEX>`, `<r N>`, `<c>`, `<t>`, `<rc N>`, `<rd N>`). `i1`–`i5` (CPS-пакеты) **не негоциируются** — config-only; mismatch client/server рвёт соединение. Конфиг без build-tag `with_awg` **явно отвергается** ядром при загрузке (не молчаливый downgrade).

## Объём / Вне объёма

**В объёме:**

* Парсинг AWG-полей из `wireguard://`-URI (query-params `jc`/`jmin`/`jmax`/`s1`–`s4`/`h1`–`h4`/`i1`–`i5`) в endpoint-map.
* Поддержка нового алиаса схемы `awg://` (нормализуется в `wireguard://`-путь парсера) — чтобы подписки, помечающие узел как AmneziaWG отдельной схемой, парсились.
* Перенос AWG-полей через `ParsedNode.Outbound` без потерь.
* Эмиссия AWG-полей в `endpoints[]` `config.json` (промотнуты в корень endpoint).
* Round-trip обратно в share-URI (`ShareURIFromWireGuardEndpoint` эмитит AWG query-params).
* Сохранение типов: числа эмитятся как JSON number, `i1`–`i5` — как строки с сохранением регистра тегов.
* Edit-диалог: AWG-поля доступны минимум через **Raw JSON** (already works after parser/generator land); опциональная Settings-секция — см. Фаза 5.
* Unit-тесты парсера, генератора, share-round-trip + golden config.

**Вне объёма:**

* Bump `RequiredCoreVersion` до `1.13.13-lx.1` и миграция core-downloader на форк — это **SPEC 072** (см. § Принцип очерёдности). Без неё AWG-конфиг сгенерится, но bundled-ядро (1.13.12 upstream) **отвергнет** его при загрузке. Тестировать end-to-end можно только после 072 либо с локально подложенным lx-ядром.
* XHTTP-transport (VLESS/VMess/Trojan) — отдельная фича форка, отдельная SPEC. Не путать: текущий `uriTransportFromQuery` уже remap'ит `xhttp`→`httpupgrade` (`node_parser_transport.go:122`) — это pre-fork поведение, его НЕ трогаем здесь.
* Валидация совместимости client/server CPS (`i1`–`i5`) — невозможна на стороне лаунчера (поля не негоциируются). Максимум — UI-warning (см. § Риски).
* Win7 `windows/386` — у форка нет 386-сборки; gap решается в SPEC 072, не здесь.
* Multi-peer AWG-endpoint — `ShareURIFromWireGuardEndpoint` уже отвергает multi-peer (`shareuri_wireguard.go:32-34`); AWG не меняет это ограничение.
* Полноценная per-field форма всех AWG-полей в Settings-вкладке как обязательная — оставлена опциональной (Фаза 5), Raw JSON покрывает функциональность.

## Входные данные

**Файлы и функции под правку:**

| Файл | Функция / место | Что делаем |
|---|---|---|
| `core/config/subscription/node_parser_wireguard.go` | `parseWireGuardURI` (стр. 18-181); endpoint-map (стр. 131-143) | Извлечь AWG query-params, положить в endpoint root |
| `core/config/subscription/node_parser_core.go` | `IsDirectLink` (стр. 20-31); `ParseNode` switch (стр. 195) | Добавить `awg://` → нормализация в WG-путь |
| `core/config/subscription/shareuri_wireguard.go` | `ShareURIFromWireGuardEndpoint` (стр. 17-86) | Эмитить AWG query-params |
| `core/config/outbound_generator.go` | `GenerateEndpointJSON` (стр. 494-517) | Проверить, что generic-копия проносит AWG (правки минимальны/нет) |
| `core/config/subscription/node_parser_wireguard.go` | новый helper | Парсинг + валидация AWG-полей (числовые / строковые) |
| `ui/configurator/outbounds_configurator/edit_dialog.go` | Raw-вкладка (стр. 277) / опц. Settings | AWG через Raw уже работает; опц. секция — Фаза 5 |

**Точный список AWG-полей (из research, единственный источник истины):**

| Поле | Тип | JSON | Семантика |
|---|---|---|---|
| `jc` | uint32 | number | количество junk-пакетов |
| `jmin` | uint32 | number | мин. размер junk-пакета |
| `jmax` | uint32 | number | макс. размер junk-пакета |
| `s1` | uint32 | number | handshake junk size 1 |
| `s2` | uint32 | number | handshake junk size 2 |
| `s3` | uint32 | number | handshake junk size 3 |
| `s4` | uint32 | number | handshake junk size 4 |
| `h1` | uint32 | number | magic header 1 |
| `h2` | uint32 | number | magic header 2 |
| `h3` | uint32 | number | magic header 3 |
| `h4` | uint32 | number | magic header 4 |
| `i1` | string | string | CPS packet 1 (case-sensitive tags) |
| `i2` | string | string | CPS packet 2 |
| `i3` | string | string | CPS packet 3 |
| `i4` | string | string | CPS packet 4 |
| `i5` | string | string | CPS packet 5 |

Числовые поля промотнуты в корень endpoint (не в peer). `i1`–`i5` — тоже корень endpoint, строки, тег-формат `<b 0xHEX>`, `<r N>`, `<c>`, `<t>`, `<rc N>`, `<rd N>` — **регистр сохраняется как есть** (не lower-case'ить).

**Целевой shape AWG-endpoint в `config.json` (`endpoints[]`):**

```json
{
  "type": "wireguard",
  "tag": "awg-server",
  "name": "singbox-wg0",
  "system": false,
  "mtu": 1408,
  "address": ["10.0.0.2/32"],
  "private_key": "...",
  "jc": 10,
  "jmin": 50,
  "jmax": 100,
  "s1": 20,
  "s2": 20,
  "s3": 60,
  "s4": 60,
  "h1": 1234567890,
  "h2": 1234567891,
  "h3": 1234567892,
  "h4": 1234567893,
  "i1": "<b 0x000100002112a442><r 12>",
  "i2": "<b 0x010100002112a442><r 12>",
  "i3": "<r 24>",
  "i4": "",
  "i5": "",
  "peers": [{
    "address": "server.example.com",
    "port": 51821,
    "public_key": "...",
    "pre_shared_key": "...",
    "allowed_ips": ["0.0.0.0/0", "::/0"],
    "persistent_keepalive_interval": 25
  }]
}
```

**Целевой URI-shape (`wireguard://` или алиас `awg://`):**

```
wireguard://<PRIVATE_KEY>@<SERVER>:<PORT>?publickey=...&address=10.0.0.2%2F32
  &allowedips=0.0.0.0%2F0%2C%3A%3A%2F0&mtu=1408&keepalive=25
  &jc=10&jmin=50&jmax=100&s1=20&s2=20&s3=60&s4=60
  &h1=1234567890&h2=1234567891&h3=1234567892&h4=1234567893
  &i1=%3Cb+0x000100002112a442%3E%3Cr+12%3E&i2=...&i3=%3Cr+24%3E#awg-server
```

`i1`–`i5` URL-эскейпятся; при парсе декодируются через `url.QueryUnescape` (регистр тегов сохраняется). Пустые `i4`/`i5` в URI не эмитятся.

## Фазы

Порядок: парсер → генератор/share → схема URI → шаблон/UI → тесты. Каждая фаза самодостаточна и не оставляет бранч в красном.

### Фаза 1 — Парсинг AWG-полей в `parseWireGuardURI`

**Deliverable:** `parseWireGuardURI` (`node_parser_wireguard.go`) после сборки канонического `endpoint`-map (стр. 131-143) извлекает AWG query-params и кладёт их в **корень endpoint** (не в peer).

* Новый helper `applyAWGFields(endpoint map[string]interface{}, q url.Values)`:
  * Числовые поля (`jc`,`jmin`,`jmax`,`s1`–`s4`,`h1`–`h4`) — через `strconv.ParseUint(v, 10, 32)`; при успехе кладём как `uint32`/`int` (см. ниже про тип, чтобы MarshalIndent дал JSON number, не строку). Невалидное число → debug-log + пропуск поля (forward-compat, не fail парса).
  * Строковые поля (`i1`–`i5`) — `url.QueryUnescape`, кладём как string **только если непустая** (`i4`/`i5` пустые не добавляем). Регистр НЕ трогаем.
  * Если **ни одного** AWG-поля нет — endpoint остаётся чистым WG (никаких пустых AWG-ключей не добавляем; backward-compatible с обычными WG-узлами).
* Вызвать helper в `parseWireGuardURI` после строки 143 (до сборки `node`).
* Тип числовых: класть Go-значение, которое `json.MarshalIndent` сериализует как number (например `uint32` или `int`); НЕ `string`. Подтвердить тестом (см. Фаза 6, golden — `"jc": 10`, не `"jc": "10"`).

**Verification:** unit-тест `TestParseWireGuardURI_AWGFields` — URI со всеми 11 числовыми + 3 непустыми `i*` → `node.Outbound["jc"] == uint32(10)` (или эквивалент), `node.Outbound["i1"] == "<b 0x000100002112a442><r 12>"`, `i4`/`i5` отсутствуют. Контрольный тест `TestParseWireGuardURI_NoAWG_StaysClean` — обычный WG-URI не приобретает AWG-ключей.

### Фаза 2 — Алиас схемы `awg://`

**Deliverable:** подписки/ссылки с `awg://` парсятся как AmneziaWG (та же endpoint-логика, что `wireguard://`).

* `IsDirectLink` (`node_parser_core.go:20-31`) — добавить `strings.HasPrefix(trimmed, "awg://")`.
* `ParseNode` switch (`node_parser_core.go`, рядом со стр. 195) — добавить case:
  ```go
  case strings.HasPrefix(uri, "awg://"):
      normalized := strings.Replace(uri, "awg://", "wireguard://", 1)
      return parseWireGuardURI(normalized, skipFilters)
  ```
  (повторяем паттерн `hy2://`→`hysteria2://`, `node_parser_core.go:160-164`).
* Решить (см. § Риски): `node.Scheme` для `awg://` — оставить `"wireguard"` (чтобы `GenerateEndpointJSON`-guard `node.Scheme != "wireguard"`, `outbound_generator.go:495`, продолжил работать) **или** ввести `"awg"` и расширить guard. **Рекомендация:** оставить `"wireguard"` — AWG это надмножество WG-endpoint'а, отдельная схема Scheme усложнит generator/share без выгоды.

**Verification:** `TestParseNode_AWGScheme_RoutesToWireguard` — `awg://...?jc=10` парсится, `node.Scheme == "wireguard"`, AWG-поля на месте. `TestIsDirectLink_AWG` — `awg://x` → true.

### Фаза 3 — Эмиссия AWG в `config.json`

**Deliverable:** `GenerateEndpointJSON` эмитит AWG-поля из `node.Outbound` в корень endpoint с корректными типами.

* `GenerateEndpointJSON` (`outbound_generator.go:494-517`) уже копирует **все** ключи `node.Outbound` generic-циклом (стр. 500-503) → AWG-поля, положенные в Фазе 1, эмитятся автоматически. **Ожидаемая правка — нулевая или near-zero.**
* Единственный риск — порядок/тип: `json.MarshalIndent` сортирует ключи алфавитно (детерминированно) и сериализует `uint32`/`int` как number. Проверить тестом, что `jc` → `10`, `i1` → `"<b ...>"`.
* Если по результату Фазы 1 числа случайно окажутся строками — фиксить **в Фазе 1** (тип хранения), не в генераторе.

**Verification:** `TestGenerateEndpointJSON_AWG` — node с AWG-полями → JSON-строка содержит `"jc": 10` (number) и `"i1": "<b 0x..."` (string); `json.Unmarshal` обратно даёт те же значения и типы.

### Фаза 4 — Round-trip обратно в share-URI

**Deliverable:** `ShareURIFromWireGuardEndpoint` эмитит AWG query-params, так что `endpoint → URI → endpoint` сохраняет AWG-поля.

* `ShareURIFromWireGuardEndpoint` (`shareuri_wireguard.go:17-86`) — после блока канонических query (стр. 56-77) добавить эмиссию AWG:
  * Числовые: `if v := mapGetInt(ep, "jc"); v > 0 { q.Set("jc", strconv.Itoa(v)) }` (и аналогично для остальных 10). **Решение по нулям:** `0` — валидное значение AWG (например `jc=0` = junk off); но share-URI-семантика «эмитить только заданное». **Рекомендация:** эмитить число, если ключ **присутствует** в map (а не `>0`), чтобы не терять явный `0`. Завести helper `mapHasNumericField` / использовать `_, ok := ep["jc"]`.
  * Строковые `i1`–`i5`: `if s := mapGetString(ep, "i1"); s != "" { q.Set("i1", s) }` — `url.Values.Encode` сам эскейпит. Пустые пропускаем.
* `i1`–`i5` могут содержать `<`/`>`/пробелы — `url.Values.Encode` корректно их эскейпит; при обратном парсе (Фаза 1) `url.QueryUnescape` восстановит.

**Verification:** `TestShareURI_AWG_RoundTrip` — собрать endpoint-map с полным AWG-набором → `ShareURIFromWireGuardEndpoint` → `parseWireGuardURI` → сравнить AWG-поля 1:1 (числа равны, строки идентичны включая регистр). `TestShareURI_AWG_ZeroJc_Preserved` — `jc:0` присутствует в map → в URI есть `jc=0` → после round-trip `jc == 0`.

### Фаза 5 — Edit-диалог (опционально: Settings-секция)

**Deliverable:** AWG-поля редактируемы. **Baseline:** Raw JSON-вкладка (`edit_dialog.go:277`) уже их несёт после Фаз 1–4 — функционально достаточно.

* **Минимум (обязательно):** ничего не правим в UI — подтверждаем тестом/ручной проверкой, что AWG-endpoint, отредактированный в Raw, проходит save без потерь (Raw → form-sync `syncRawToForm`, `edit_dialog.go:791`, не должен стирать неизвестные endpoint-поля).
* **Опционально (если время):** collapsible-секция «AmneziaWG (obfuscation)» в Settings-вкладке для wireguard-узлов — 11 number-полей + 5 text-полей с tooltip'ами и предупреждением про CPS-mismatch (`i1`–`i5`). Pre-fill из parsed-endpoint. Это чисто UX-сахар поверх рабочего Raw-пути.

**Verification:** ручная — открыть AWG-узел в Edit, вкладка Raw показывает все AWG-поля; правка `jc` в Raw → Save → перечитать config → значение сохранилось. Если делается опциональная секция — `TestEditDialog_AWGSection_*` на pure-helpers (парс/валидация числовых полей формы), без Fyne-рендера.

### Фаза 6 — Тесты + golden config

**Deliverable:** автотесты зелёные, golden-config зафиксирован.

* Unit (перечислены по фазам выше): парсер, scheme-алиас, генератор, share round-trip — все в `core/config/subscription/*_test.go` и `core/config/generator_test.go`.
* **Golden config:** добавить fixture-узел AWG в существующий golden-набор (`core/build/testdata/golden/`) или новый mini-fixture: subscription c одним `awg://`-узлом → сгенерённый `config.json` сравнивается с `expected.config.json`. Diff должен показывать **только** новый AWG-endpoint в `endpoints[]` с корректными типами (числа — number, `i*` — string).
* Регрессия: существующие WireGuard-тесты (`TestGenerateEndpointJSON_CommentSanitized`, `generator_test.go:435`; share-тесты `outbound_share_test.go`) НЕ ломаются — обычный WG-узел без AWG-полей эмитит идентичный прежнему JSON.
* **Type-fidelity тест** — отдельно проверить, что `jc:10` сериализуется как `10`, а не `"10"`, и `json.Unmarshal` в `map[string]interface{}` даёт `float64` (а не string) — защита от регрессии хранения-типа из Фазы 1.

**Verification:** `go test ./core/...` зелёный; golden-diff ревьюится глазами на типы и shape.

## Риски и открытые вопросы

* **Зависимость от core (блокер end-to-end):** сгенерённый AWG-конфиг ядро **отвергнет**, пока bundled sing-box — 1.13.12 upstream (нет `with_awg`). Полноценный live-тест возможен только после **SPEC 072** (миграция на `v1.13.13-lx.1`) либо с локально подложенным lx-ядром. Сама эта SPEC (парс/ген/share/тесты) НЕ блокируется 072 — можно мержить, фича «спит» до бампа ядра.
* **Решение мейнтейнера — `node.Scheme` для `awg://`:** оставить `"wireguard"` (рекомендация — минимум правок в generator-guard, `outbound_generator.go:495`) или ввести `"awg"`. Если когда-нибудь захочется отличать «чистый WG» от AWG в UI/логах — понадобится отдельная Scheme; сейчас YAGNI.
* **Решение — семантика нуля в share-URI:** `jc=0` (junk off) — валидное явное значение. Эмитить по `присутствию ключа` (рекомендация) или по `>0` (потеряет явный `0`). Влияет на round-trip fidelity. Зафиксировать в Фазе 4.
* **CPS-mismatch (`i1`–`i5`) — неустранимо на стороне клиента:** поля **не негоциируются**; рассинхрон client/server рвёт соединение. Лаунчер не может валидировать. Максимум — UI-warning в опциональной Settings-секции (Фаза 5). Документировать в release notes: «значения `i1`–`i5` должны точно совпадать с серверными».
* **Тип хранения числовых полей:** если положить как `string`, `MarshalIndent` даст `"jc":"10"` — ядро ждёт number. Жёстко тестируется (Фаза 6 type-fidelity). Источник риска — `url.Values.Get` возвращает string, конверсия обязательна.
* **`i1`–`i5` case-sensitivity:** тег-формат регистрозависимый (`<b 0xHEX>` lower-case hex, но теги как заданы). НЕ применять `ToLower` нигде в парс/share-пути. Тест round-trip с mixed-case тегами.
* **Schema-permissive иллюзия:** WireGuard-путь, в отличие от outbound-transport, перечисляет поля поимённо и в парсере, и в share — поэтому AWG требует **двух** явных правок (парс + share), generic-проброс есть только в `GenerateEndpointJSON`.
* **Multi-peer:** AWG-поля на endpoint root, не per-peer — multi-peer share уже запрещён (`shareuri_wireguard.go:32`), AWG не меняет. Если появится спрос на multi-peer AWG в config.json (минуя share) — generic-генератор это пронесёт, но subscription-round-trip останется single-peer.
* **Forward-compat при невалидном AWG в URI:** битое число (`jc=abc`) — пропускаем поле + debug-log, не валим парс всего узла (как mtu/keepalive сейчас, `node_parser_wireguard.go:95-105`). Иначе один битый параметр убьёт весь узел подписки.

## Принцип очерёдности

**Зависимости относительно двух других SPEC форк-интеграции:**

1. **SPEC 072 (lx-core: миграция core-downloader + bump `RequiredCoreVersion` на `1.13.13-lx.1`)** — **soft-зависимость / предшествует для end-to-end.** Эта SPEC (073) реализует генерацию AWG-конфига; но без 072 bundled-ядро (1.13.12 upstream) отвергнет конфиг при загрузке. Порядок: **072 раньше или одновременно** для пользовательской ценности. Код 073 (парс/ген/share/тесты) можно мержить **независимо** — он не вызывает core и не зависит от версии; фича «спит» до бампа ядра в 072. Рекомендация: land 073 за 072 (или в одном релизе), чтобы AWG-узлы работали сразу.

2. **SPEC XHTTP (transport для VLESS/VMess/Trojan, форк-фича `with_xhttp`)** — **независима / параллельна.** Другой код-путь (`uriTransportFromQuery`, `appendOutboundTransportParts`), не пересекается с WireGuard-endpoint-путём этой SPEC. Единственная общая точка — обе ждут lx-ядра (через 072). Можно разрабатывать параллельно; конфликтов мерджа с 073 нет.

**Внутренний порядок фаз (этой SPEC):** Фаза 1 (парс) → Фаза 2 (scheme-алиас) → Фаза 3 (генератор, near-zero) → Фаза 4 (share round-trip) → Фаза 5 (UI, опционально) → Фаза 6 (тесты + golden). Фазы 1–4 — обязательное ядро фичи; 1 → 4 строго последовательны (share-тест в Фазе 4 опирается на парс из Фазы 1). Фаза 6 финализирует. Каждый коммит оставляет `go test ./core/...` зелёным.

---

## Сабтаска 073.1 — robustness-фиксы парсинга (v1.1.2)

> **Статус:** ✅ сделано (v1.1.2, коммит `8f97a51`). Два бага парсинга WG/AWG-ссылок, найденные на проде после релиза AWG. Оба — в `core/config/subscription/node_parser_wireguard.go`, покрыты `wireguard_robustness_test.go`.

**Баг A — сырой `/` в base64-приватном ключе.**
- *Симптом:* ссылка `wireguard://<key с />…@host` (стандартный base64 содержит `/` примерно в половине ключей) → узел добавляется в Sources, но **пропадает из Preview**.
- *Корень:* `url.Parse` принимает `/` в authority-части за начало пути → `parsedURL.User == nil` → парсер возвращает «missing private key».
- *Фикс:* перед `url.Parse` percent-энкодим сырой `/` в userinfo (между `://` и `@`) → `%2F`; `PathUnescape` восстанавливает ключ. Уже-`%2F`-закодированные ссылки не затрагиваются (helper `percentEncodeWGUserinfoSlashes`).

**Баг B — голый адрес без CIDR-префикса.**
- *Симптом:* `address`/`allowed_ips` = голый IP (`172.16.0.2`, частый случай в экспортах AmneziaWG/`.conf`) → ядро **не стартует**: `endpoints[0].address … netip.ParsePrefix("172.16.0.2"): no '/'`.
- *Корень:* sing-box ждёт `netip.Prefix` (CIDR); парсер клал адрес как есть, без маски.
- *Фикс:* голому IPv4 дописываем `/32`, IPv6 — `/128`; применено к `address` и `allowed_ips` (helper `normalizeWGPrefixes`).

**Связь с AWG:** оба бага особенно бьют по AmneziaWG-узлам — их часто импортируют как стандартные amnezia-конфиги (`.conf`-стиль с голым `Address`), а приватные ключи — обычный std-base64 с `/`. Поэтому фиксы документируются здесь, в спеке AWG, а не только в WireGuard.

## Сабтаска 073.2 — диапазоны H1–H4 (AWG 2.0 header randomization)

> **Статус:** ✅ сделано (после v1.1.3). Найдено на проде: реальный `.conf` из AmneziaWG 2.0 нёс `H1 = 43613244-384550127` (диапазон, а не число) — `applyAWGFields` молча скипал все четыре заголовка, узел добавлялся без них, и handshake с сервером не сходился.

- *Симптом:* импорт `.conf`/`awg://` с `H1–H4 = lo-hi` → узел есть, magic-заголовков в endpoint нет → соединение не устанавливается.
- *Корень:* AWG 2.0 рандомизирует заголовки: клиент выбирает значение внутри диапазона, сервер принимает весь диапазон. Парсер ждал одиночный uint32 (`strconv.ParseUint`) и по политике forward-compat пропускал «битое» поле.
- *Фикс (финальный, ядро ≥ `1.13.13-lx.6`):* `parseAWGHeaderRange` (`node_parser_wireguard.go`): одиночное число — int64 как раньше; `lo-hi` (оба uint32) — **пробрасывается в endpoint строкой** `"h1": "N-M"` (нормализованной: границы упорядочиваются) — ядро само выбирает значение внутри диапазона на каждый handshake. Диапазоны — только для `h1`–`h4`; на остальных числовых полях `lo-hi` скипается. Невалидное значение — прежняя политика: скип поля + debug-log, узел живёт. Share-URI отдаёт диапазон без правок (`awgNumericString` уже принимал строки) — round-trip без потерь. `RequiredCoreVersion` забампен на `1.13.13-lx.6`: ядра старше отвергают строковую форму.
- *Промежуточный вариант* (один коммит, `e7ed77e`): диапазон схлопывался в начало — корректно для сервера, но без per-handshake-рандомизации; заменён проброс-вариантом, как только ядро lx.6 научилось диапазонам.
- *Диагностика при импорте:* мусор в `h1`–`h4` — **WarnLog** (молча выброшенный заголовок = WG-дефолт у ядра = несовпадение с сервером, исходный баг 073.2); пересечение эффективных диапазонов четырёх заголовков (`awgHeaderOverlap`, незаданный = WG-дефолт `1`–`4`, одиночный = `[v,v]`) — **WarnLog** с парой полей: ядро отвергнет такой endpoint с `headers must not overlap`, ошибка ядра остаётся источником истины.
- *Приёмка на ядре:* `sing-box 1.13.13-lx.6` (`darwin-arm64`, SHA256 сверен с релизом): `check` принял и диапазонный конфиг из реального awg2-экспорта (полный пайплайн: `.conf`-текст → парсер → `endpoints[]`), и регрессионный с одиночными `H`.
- *Тесты:* `awg_range_test.go` — диапазоны в URI (прямой/перевёрнутый/мусор/диапазон на не-h-поле), share-round-trip + JSON-fidelity (диапазон — строка, одиночное — число) и полный AWG2-`.conf` с диапазонами и пустыми `I2–I5`.
