# SPEC: NaïveProxy — parser, outbound generator, share-URI encoder

Задача: поддержать протокол **NaïveProxy** как полноправный tier-1 протокол в парсере подписок, генераторе sing-box outbound'ов и share-URI-энкодере. Пользователь добавляет в подписку URI формата `naive+https://…` или `naive+quic://…`, лаунчер генерирует sing-box outbound `type: "naive"`, а при правом клике «Copy link» на ноде получает обратно валидный URI.

**Статус:** не реализовано (планирование). После реализации — обновить раздел Статус, написать IMPLEMENTATION_REPORT.

**Связанные:**
- `docs/ParserConfig.md` — описание подписочных URI (раздел про naive добавить).
- `core/config/subscription/node_parser.go` — главный dispatcher URI-схем.
- `core/config/subscription/share_uri_encode.go` — обратный путь.

---

## 1. Проблема

### 1.1 До изменений

Лаунчер не знает о NaïveProxy ни на одном уровне:

- `IsDirectLink` → `false` для `naive+https://...` → URI отклоняется в Wizard → Sources.
- `ParseNode` → `unsupported scheme` → любая подписка со строкой `naive+https://...` логирует ошибку и дропает узел.
- Share-URI encoder → не знает `type: "naive"` → «Copy link» вернёт `ErrShareURINotSupported`.

Пользователи, у которых сервер на NaïveProxy (популярно в обходе DPI через Chrome-подобный TLS-fingerprint, плюс HTTPS/2 / HTTP/3 transport), вынуждены руками редактировать `config.json`.

### 1.2 Цель

- URI `naive+https://user:pass@host:443/?padding=true#Label` парсится в `ParsedNode{Scheme: "naive", ...}`.
- `GenerateNodeJSON` для naive-ноды собирает валидный sing-box outbound (`type: "naive"`), который sing-box 1.13.0+ принимает.
- Right-click → Copy link на naive-ноде возвращает исходный (или эквивалентный) URI.
- Unit-тесты покрывают форматы из de-facto спецификации (DuckSoft, 2020) + edge cases.

---

## 2. URI-формат (входной)

De-facto стандарт без официального endorsement'а от `klzgrad` (автор NaïveProxy), но поддерживается клиентами NaiveGUI / v2rayN / NekoRay / sing-box-client-GUIs. Источник: https://gist.github.com/DuckSoft/ca03913b0a26fc77a1da4d01cc6ab2f1.

### 2.1 Синтаксис

```
naive+https://<username>:<password>@<host>:<port>/?<params>#<label>
naive+quic://<username>:<password>@<host>:<port>/?<params>#<label>
```

- **Обязательно:** схема (`naive+https` или `naive+quic`), `host`. `port` опционален → default `443` для обеих схем.
- **Username/password:** опциональны; если только username — считается password (как у hysteria2).
- **Query params:**
  - `padding=true|false` — default `false`. Не имеет прямого поля в sing-box naive outbound; **игнорируем с warning** в первой версии (см. §5.3).
  - `extra-headers=<encoded>` — дополнительные HTTP-заголовки. Формат до URL-encoding: `Header1: Value1\r\nHeader2: Value2`. В URI `\r\n` → `%0D%0A`, `:` → `%3A`.
- **Fragment (`#label`):** human-readable имя ноды. URL-decode + UTF-8 validate (как делается для vless/vmess).

### 2.2 Примеры

Canonical (из спеки DuckSoft):

```
naive+https://what:happened@test.someone.cf?padding=false#Naive!
naive+https://some.public.rs?padding=true#Public-01
naive+quic://manhole:114514@quic.test.me
naive+https://some.what?extra-headers=X-Username%3Auser%0D%0AX-Password%3Apassword
```

Дополнительные (лаунчер должен корректно обработать):

```
naive+https://a:b@example.com:10443/?extra-headers=X-Forwarded-Proto%3Ahttps%0D%0AX-Custom%3Atest#%E2%9C%85%20JP-01
naive+quic://secret@quic.example.com:8443/
naive+https://u:p@host.tld            # без /, без query, без fragment
```

### 2.3 Что НЕ поддерживаем в v1

- Multi-remote URI (одна строка, несколько хостов) — нет в спеке DuckSoft, нет в клиентах.
- `insecure_concurrency` — нет в URI; sing-box это concurrency-hint на стороне outbound'а, не чат-с-подпиской.
- QUIC-специфичные параметры (`quic_congestion_control`) — нет в URI; выставляем `bbr` как sing-box default.

---

## 3. Маппинг URI → sing-box outbound JSON

sing-box 1.13.0+, [официальная документация](https://sing-box.sagernet.org/configuration/outbound/naive/).

### 3.1 Выходной JSON (каноничная форма)

```json
{
  "type": "naive",
  "tag": "<tag>",
  "server": "<host>",
  "server_port": <port>,
  "username": "<user>",
  "password": "<pass>",
  "tls": {
    "enabled": true,
    "server_name": "<host>"
  }
}
```

Для `naive+quic` добавляется:
```json
  "quic": true,
  "quic_congestion_control": "bbr"
```

Для `extra-headers`:
```json
  "extra_headers": {
    "Header1": "Value1",
    "Header2": "Value2"
  }
```

### 3.2 Правила

1. `tls.enabled: true` ставим **всегда** (naive без TLS не имеет смысла даже в `quic`-режиме — QUIC подразумевает TLS).
2. `tls.server_name` = `server` (host из URI). Пользователь может переопределить через `extra-headers`? Нет — в de-facto спеке нет параметра `sni`, только `extra-headers`. Значит для кастомного SNI пользователь правит JSON после wizard-save. OK для v1.
3. sing-box naive outbound поддерживает **только** эти TLS-поля: `server_name`, `certificate`, `certificate_path`, `ech`. Никаких `alpn`, `utls`, `reality`, `min_version` — при попытке добавить их sing-box отклонит конфиг. Наш генератор не добавляет их в TLS-блок naive outbound'а (в отличие от vless/trojan, где мы их собираем из query).
4. `insecure` из query (если вдруг появится в URI, хотя в спеке нет) — игнорируем, `tls.insecure` в naive outbound не поддерживается.
5. Если только username (`naive+https://pass@host`) — считаем password, `username` остаётся пустым, в JSON не включаем.
6. URL-decode и UTF-8-fixup для label (как для других протоколов).

### 3.3 Parsing extra-headers

URL-decoded значение query-param'а `extra-headers` — строка вида `Header1: Value1\r\nHeader2: Value2`. Парсим:

1. Split по `\r\n`.
2. Для каждой строки split по первому `:`.
3. Trim обе половины.
4. Если header name содержит символы вне спеки (см. [DuckSoft §Extra Headers](https://gist.github.com/DuckSoft/ca03913b0a26fc77a1da4d01cc6ab2f1#extra-headers-encoding)) — WARN и skip этой пары (остальные сохраняем).
5. Если split по `:` не дал 2 частей — WARN и skip.

Валидный header-name charset (из спеки):
```
!#$%&'*+-.0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ\^_`abcdefghijklmnopqrstuvwxyz|~
```

Регулярка:
```regexp
^[!#$%&'*+\-.0-9A-Z\\^_`a-z|~]+$
```

Значения: любой байт кроме CR, LF, NUL.

---

## 4. Маппинг outbound JSON → URI (обратный, share-URI encoder)

### 4.1 Правила

1. Если `quic: true` → scheme = `naive+quic`, иначе `naive+https`.
2. `user = username`, `pass = password`. Если оба пустые — URI без userinfo. Если только `password` — ставим как username (в URI).
3. `host:port` из `server` и `server_port`.
4. `#label` из `tag` (URL-escape фрагмента через `url.PathEscape` как делают vless/vmess encoder'ы).
5. Query:
   - `extra-headers` = encoded сериализация map'а `extra_headers` (строки соединяются через `\r\n`, пара `Header: Value`, весь результат `url.QueryEscape`).
   - `padding` — **не включаем в URI** (sing-box не знает про `padding`, в input'е мы его молча игнорим — было бы неверно выдумывать обратно).
6. Если `tls.server_name` отличается от `server` — **warning в log**, но URI собираем (в v2 можно добавить кастомный param; для v1 sing-box игнорит SNI на входе URI).
7. Если `extra_headers` содержит пару, которая нарушает charset спеки — в энкодере такие **skip с warning** (на уровне write'а конфига мы не должны их были пустить, но defensive-encode).

### 4.2 Canonical vs round-trip

Round-trip **не гарантируется** для некоторых форм:
- `naive+https://pass@host` (только password без username): после round-trip получим `naive+https://pass@host` тоже, без разницы. ОК.
- Если на входе был `padding=true` — после round-trip потеряем. **Это by design** (sing-box не хранит padding).
- Порядок header'ов в `extra-headers`: input-order не сохраняется; map'ы в Go не ordered. Для round-trip'а отсортируем ключи лексикографически. **Тест должен использовать лексикографический порядок ключей** в expected выводе.

---

## 5. Валидация / graceful degradation

### 5.1 Обязательные поля

- `host` непустой → иначе ошибка парсинга.
- `port` в диапазоне 1-65535 (default 443 если не указан).
- `scheme` ровно `naive+https` или `naive+quic` (case-sensitive).

### 5.2 Опциональные

- User/password: если оба пустые — anonymous режим (sing-box примет — это будет просто без auth). Без ошибки.
- Label: пустой → сгенерируется из tag-префикса (как hysteria2 делает сейчас).

### 5.3 Игнорируемые с warning

- `padding=true|false` — логируем `[parser] naive: 'padding' parameter has no sing-box equivalent, ignoring`.
- Неизвестные query-ключи (напр., `sni=...`, `alpn=...`, etc.) — `[parser] naive: unknown query param 'X', ignoring`.
- Invalid extra-headers pair — `[parser] naive: invalid extra-headers entry 'X', skipping`.

### 5.4 sing-box binary feature-probe

**Важный подводный камень:** sing-box docs говорят «Only available on Apple, Android, Windows, and select Linux builds» для naive outbound. У наших официальных SagerNet-релизов по платформам это проверим на запуске:

- При первой генерации naive outbound'а — вызываем `sing-box check` на временной конфиге.
- Если `check` падает с `unknown outbound type "naive"` или аналогичным — **WARN в UI** (диалог на вкладке Rules / в sources-status-row): «Your sing-box build does not include NaiveProxy support. Visit <link to docs> or rebuild with `with_naive_proxy` tag.»
- Нода всё равно попадает в `config.json` (пользователь может потом подложить свой бинарник).
- Флаг «naive not supported» кэшируется в `StateService.SingboxFeaturesSupported` на время сессии.

**V1 можно отложить probe** — просто документировать требование в release-notes, пользователь сам разберётся. **V2 добавить probe.**

---

## 6. Архитектура

### 6.1 Файлы

**Новые:**
- `core/config/subscription/node_parser_naive.go` — parser helpers (парсинг extra-headers, валидация charset'а имён).
- `core/config/subscription/node_parser_naive_test.go` — unit-тесты парсера (+ round-trip).
- `SPECS/044-F-C-NAIVE_PROXY_PARSER/*` — эта спека.

**Изменённые:**
- `core/config/subscription/node_parser.go`:
  - `IsDirectLink` — добавить `naive+https://` / `naive+quic://`.
  - `ParseNode` — добавить `case strings.HasPrefix(uri, "naive+https://")`, то же для `naive+quic://`.
  - `buildOutbound` (function mapping `ParsedNode → map`): добавить ветку `else if node.Scheme == "naive"`.
- `core/config/subscription/share_uri_encode.go`:
  - `ShareURIFromOutbound` switch — добавить `case "naive"`.
  - Новая функция `shareURIFromNaive(out)`.
- `core/config/subscription/share_uri_encode_test.go` — новые тесты.

**Не трогаем:**
- `wizard_template.json` — naive не требует шаблонных vars.
- `core/config/outbound_generator.go` — работает с мапом `outbound`, ничего специфичного для naive не нужно.
- `core/config/configtypes/types.go` — `ParsedNode` подходит как есть (`Scheme`, `Server`, `Port`, `UUID=user`, `Query` для password + extra-headers).

### 6.2 Маппинг ParsedNode

- `Scheme`: `"naive"` (именно так, не `"naive+https"` — в sing-box JSON `"type": "naive"`).
- `Server`: host.
- `Port`: port (default 443).
- `UUID`: **username** (как у других протоколов, где нужен основной credential — username сюда кладём).
- `Query`:
  - `password`: extracted password (из userinfo).
  - `quic`: `"true"` если schema была `naive+quic`, иначе отсутствует.
  - `extra-headers`: сырая (но уже URL-decoded) строка `Header1: Value1\r\nHeader2: Value2`, буду парсить на generator'е.
- `Label`: URL-decoded + UTF-8-fixed fragment.

---

## 7. Тесты

### 7.1 Parser (`node_parser_naive_test.go`)

```
TestParseNode_Naive_Canonical       — 4 примера из спеки
TestParseNode_Naive_DefaultPort     — без :port → port=443
TestParseNode_Naive_OnlyPassword    — naive+https://pass@host (нет username)
TestParseNode_Naive_NoAuth          — naive+https://host (anonymous)
TestParseNode_Naive_QUIC            — naive+quic:// — quic=true в Query
TestParseNode_Naive_ExtraHeaders    — parse и re-emit в generator'е
TestParseNode_Naive_PaddingIgnored  — padding=true в URI → не в outbound
TestParseNode_Naive_InvalidScheme   — naive:// (без +https) → ошибка
TestParseNode_Naive_EmptyHost       — naive+https://:443 → ошибка
TestParseNode_Naive_FragmentUTF8    — %E2%9C%85 → ✅ в label
TestParseNode_Naive_MaxURILength    — uri > 8 KB → ошибка
```

### 7.2 Outbound generator (в том же файле через `TestBuildOutbound_Naive_*`)

```
TestBuildOutbound_Naive_HTTPS       — базовый JSON
TestBuildOutbound_Naive_QUIC        — +quic:true +congestion
TestBuildOutbound_Naive_WithHeaders — extra_headers map'ой
TestBuildOutbound_Naive_NoAuth      — без username/password
TestBuildOutbound_Naive_TLSBlock    — tls.enabled=true, tls.server_name=host
```

### 7.3 Share URI encoder (`share_uri_encode_test.go`)

```
TestShareURIFromNaive_HTTPS         — JSON → URI
TestShareURIFromNaive_QUIC          — quic variant
TestShareURIFromNaive_ExtraHeaders  — sorted keys, %0D%0A separator
TestShareURIFromNaive_NoAuth        — без userinfo в URI
TestShareURIFromNaive_EmptyServer   — ErrShareURINotSupported
TestShareURIRoundtrip_Naive         — URI → ParseNode → buildOutbound → ShareURI → тот же URI (с оговорками: padding теряется, headers — sorted)
```

### 7.4 IsDirectLink / dispatch

```
TestIsDirectLink_Naive              — naive+https, naive+quic → true; naive:// → false
```

### 7.5 Live tests — **не добавляем** (naive-ноды не бывают публичными для анонимного тестирования, и мы не хотим добавлять зависимость от внешнего сервера).

---

## 8. Документация

- `docs/ParserConfig.md` — добавить раздел «NaïveProxy» со схемой URI и примерами. Обновляется в том же коммите.
- `docs/release_notes/upcoming.md` — запись в секциях EN + RU: «NaïveProxy URI parser/generator/share-encoder».
- `README.md` / `README_RU.md` — в списке поддерживаемых протоколов добавить «NaïveProxy (naive+https, naive+quic)».

---

## 9. Open questions (для v2 / будущих итераций)

1. **sing-box feature-probe на startup** (§5.4) — есть ли у текущего бинарника компилированная поддержка naive. Сейчас — не делаем.
2. **Кастомный SNI** — v2 добавить `sni=...` как query-param в URI, писать в `tls.server_name`.
3. **`insecure_concurrency`** — если в community появится `?concurrency=N` — замапим. Пока нет.
4. **ALPN / ECH** — если community-спек расширят URI-синтаксис — добавим.
5. **Multi-remote URI** — если когда-нибудь появится в спеке — отдельный spec.
6. **Jump / detour для naive** (как у Xray-JSON-array) — в v1 naive не участвует в chain'ах; пользователь, который хочет naive как hop, кладёт `detour`-тэг вручную в JSON после save.

---

## 10. Не-цели

- Не пишем свой naive-клиент. sing-box делает heavy lifting.
- Не валидируем TLS-сертификаты серверов — это runtime-область sing-box.
- Не реализуем `naive-server` direction (только клиентский outbound).
