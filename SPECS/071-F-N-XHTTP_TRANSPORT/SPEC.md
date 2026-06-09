# SPEC 071-F-N — XHTTP_TRANSPORT

**Status:** New (N)
**Type:** Feature (F) — добавить полноценную поддержку транспорта **XHTTP** (Xray-совместимый `splithttp`) для outbound'ов VLESS/VMess/Trojan: парсинг из подписок, генерация в `config.json`, отображение в UI Outbounds/transport. Опирается на ядро **sing-box-lx** (`with_xhttp`), которое умеет `type: "xhttp"`; стоковый sing-box такого транспорта не имеет.
**Depends on:** **SPEC 072** (миграция ядра на `sing-box-lx` v1.13.13-lx.1) — **обязательная** зависимость. Без `with_xhttp` в ядре конфиг с `transport.type=xhttp` отклоняется на load-time (фича-гейты строгие: ядро без build-тега не «деградирует молча», а **reject'ит** конфиг). Эта SPEC **не должна** landиться до того, как RequiredCoreVersion указывает на fork-сборку с `with_xhttp`.
**Не меняет:** WireGuard/AmneziaWG (отдельная SPEC), DNS-схему, wizard-движок template-expressions (067), Clash-API клиент. Параметры AmneziaWG (`jc/jmin/jmax/s1–s4/h1–h4/i1–i5`) — out of scope, отдельная SPEC.

---

## Цель

Сейчас лаунчер при встрече `xhttp` в подписке **молча деградирует** его до sing-box-транспорта `httpupgrade`. Это другой wire-протокол: `httpupgrade` — это HTTP/1.1 Upgrade (по сути WebSocket без WS-фрейминга), а XHTTP — это Xray `splithttp` поверх обычных HTTP-запросов с режимами (`packet-up` / `stream-up` / `stream-one`) и padding-обфускацией. Узел, который провайдер выдал как `type=xhttp` под Reality, после такой подмены **не подключается** к серверу (сервер ждёт splithttp-семантику, получает Upgrade). Параметры XHTTP (`mode`, `x_padding_bytes`, `no_grpc_header`, extra-headers) сейчас полностью теряются — даже если бы тип сохранялся, генератор их не эмитит.

Цель — после миграции на `sing-box-lx` (SPEC 072) **сквозно** поддержать XHTTP: распарсить `type=xhttp` из URI VLESS/VMess/Trojan в честный sing-box-`xhttp` транспорт со всеми его полями, прокинуть их без потерь через `ParsedNode.Outbound["transport"]`, сэмитить в `config.json` в правильной схеме, корректно отдать обратно в share-URI (round-trip), и показать в Outbounds-редакторе. Узлы XHTTP+Reality из публичных подписок начинают работать вместо тихого слома.

## Контекст

**Текущее поведение (баг, который чинит эта SPEC).** XHTTP в подписках обрабатывается тремя точками, и все три деградируют его в `httpupgrade`:

1. **VLESS / Trojan:** `core/config/subscription/node_parser_transport.go:121-130` — `case "xhttp", "httpupgrade"` → `t := map[string]interface{}{"type": "httpupgrade"}`. Комментарий прямо говорит: «Xray "xhttp" and subscription alias "httpupgrade" → sing-box "httpupgrade"».
2. **VMess:** `core/config/subscription/node_parser_core.go:562-564` — `if network == "xhttp" { network = "httpupgrade" }`; плюс дубль в `core/config/subscription/node_parser_vmess.go:236-237` (`case "xhttp", "httpupgrade": net = "httpupgrade"`).
3. **Эмиссия:** `core/config/outbound_jsonbuilder.go::appendOutboundTransportParts` (строки 37-80) эмитит **только** `type / path / host / service_name / headers`. Поля XHTTP (`mode`, `x_padding_bytes`, `no_grpc_header`) не эмитятся, даже если бы попали в transport-map.
4. **Round-trip обратно в URI:** `core/config/subscription/shareuri_helpers.go:149-150` — `case "httpupgrade": q.Set("type", "xhttp")`. То есть лаунчер берёт sing-box `httpupgrade` и при экспорте обзывает его `xhttp` в URI — закрепляя путаницу `xhttp ⇄ httpupgrade` в обе стороны.

**Почему это работало «достаточно» раньше.** Стоковый sing-box (текущий pin `1.13.12`, `internal/constants/constants.go:61`) **не имеет** транспорта `xhttp` вообще. Поэтому деградация в `httpupgrade` была единственным способом не уронить конфиг на load-time. Часть узлов случайно работала (если сервер на самом деле слушал httpupgrade-совместимый путь), большинство XHTTP+Reality-узлов — нет.

**Возможность из fork'а.** `sing-box-lx` v1.13.13-lx.1 (ветка `lx`, `with_xhttp` включён в релизном `LX_TAGS`) добавляет нативный (lean-native, не вендоренный) транспорт `xhttp` — Xray-совместимый splithttp для VLESS/VMess/Trojan, с режимами `auto|packet-up|stream-up|stream-one`, работает с TLS/Reality. После SPEC 072 ядро будет принимать `transport.type=xhttp`, и деградация в `httpupgrade` становится не нужна — её надо снять.

**Архитектурная зацепка.** Модель transport в лаунчере **schema-permissive**: `ParsedNode.Outbound` — это `map[string]interface{}`, transport лежит в `Outbound["transport"]` как `map[string]interface{}` и проходит насквозь без валидации enum'ов. Единственное узкое место — `appendOutboundTransportParts`, который эмитит избранный whitelist полей. То есть достаточно (а) распознать `xhttp` как самостоятельный тип в трёх парсер-точках и (б) расширить whitelist эмиттера новыми полями XHTTP. Структурно это малоинвазивно.

## Объём / Вне объёма

**В объёме:**

- Парсинг `type=xhttp` (VLESS/Trojan через `uriTransportFromQuery`) и `network=xhttp` (VMess) в honest sing-box `transport.type=xhttp` — **только** при условии, что ядро это поддерживает (см. § Риски про fallback-политику).
- Извлечение и проброс XHTTP-полей в transport-map: `mode`, `host`, `path`, `headers`, `x_padding_bytes`, `no_grpc_header`.
- Эмиссия этих полей в `config.json` через `appendOutboundTransportParts`.
- Корректный round-trip обратно в share-URI: sing-box `xhttp` → URI `type=xhttp` (а не нынешнее `httpupgrade → type=xhttp`).
- Отображение/редактирование XHTTP-транспорта в Outbounds-редакторе через Raw-JSON-таб (минимум) — verify, что схема не теряется при load/save в редакторе.
- Сохранение существующего `httpupgrade` как **отдельного** типа (не схлопывать его в xhttp обратно — это разные транспорты).
- Тесты round-trip (URI → outbound JSON → URI), golden-config diff, unit на эмиттер.

**Вне объёма:**

- Миграция ядра на `sing-box-lx`, bump `RequiredCoreVersion`, Win7/386-вопрос, downloader-URL — всё это **SPEC 072** (эта SPEC только потребитель её результата).
- AmneziaWG 2.0 и любые WireGuard-расширения (`jc/jmin/jmax/...`) — отдельная SPEC.
- Шаблонные presets/`bin/wizard_template.json` с готовыми XHTTP-пресетами — не вводим (узлы приходят из подписок; presets можно добавить позже отдельным template-cleanup).
- Полноформенный UI с отдельными полями transport (mode-dropdown, padding-range и т.п.) в form-табе редактора — out of scope; редактор XHTTP идёт через Raw-JSON-таб (form-таб transport-внутренности не показывает — это нынешнее поведение, см. `ui/configurator/outbounds_configurator/edit_dialog.go`).
- Реальная негоциация `mode=auto` (в fork'е `auto` использует `packet-up`, live-validated против Xray/3x-ui; см. `sing-box-lx/docs/lx-config.md`) — лаунчер просто прокидывает значение как есть.

## Входные данные

**Файлы и функции для правки:**

| Файл | Что |
|---|---|
| `core/config/subscription/node_parser_transport.go` (стр. 121-130) | `uriTransportFromQuery`: разделить `case "xhttp", "httpupgrade"` — `xhttp` строит `{"type":"xhttp", ...}` + поля, `httpupgrade` остаётся как есть. Добавить хелпер `xhttpTransportFromQuery(q)`. |
| `core/config/subscription/node_parser_core.go` (стр. 558-626) | VMess-ветка: убрать `if network == "xhttp" { network = "httpupgrade" }` (стр. 562-564); добавить `case network == "xhttp":` строящий xhttp-transport. |
| `core/config/subscription/node_parser_vmess.go` (стр. 236-237) | Парный дубль `case "xhttp", "httpupgrade": net = "httpupgrade"` — разделить так же. |
| `core/config/outbound_jsonbuilder.go::appendOutboundTransportParts` (стр. 37-80) | Расширить whitelist: эмитить `mode` (string), `x_padding_bytes` (string), `no_grpc_header` (bool) когда `type=="xhttp"`. `headers` сейчас принимает только `map[string]string` (стр. 68) — добавить ветку для `map[string]interface{}` (URI-парсер кладёт именно её). |
| `core/config/subscription/shareuri_helpers.go` (стр. 149-157) | Добавить `case "xhttp":` эмитящий `type=xhttp` + `mode`/`path`/`host`/`x_padding_bytes`/`no_grpc_header` в query. Существующий `case "httpupgrade": q.Set("type","xhttp")` (стр. 150) **исправить** на `q.Set("type","httpupgrade")` — иначе httpupgrade и xhttp неотличимы в URI. |
| `core/config/outbound_generator.go` (стр. 244) | Точка вызова `appendOutboundTransportParts` — изменений не требует (эмиттер сам расширяется). |
| `ui/configurator/outbounds_configurator/edit_dialog.go` | Verify-only: Raw-JSON-таб load/save сохраняет xhttp-transport без потерь. Правок кода не ожидается (схема permissive). |

**Config shape — что генерируем (sing-box-lx `xhttp`):**

```json
"transport": {
  "type": "xhttp",
  "host": "example.com",
  "path": "/xhttp",
  "mode": "stream-one",
  "headers": {"User-Agent": "..."},
  "x_padding_bytes": "100-1000",
  "no_grpc_header": false
}
```

**Полный пример outbound VLESS+XHTTP+Reality (целевой результат парсинга):**

```json
{
  "type": "vless",
  "server": "example.com",
  "server_port": 443,
  "uuid": "...",
  "tls": {
    "enabled": true,
    "server_name": "example.com",
    "utls": {"enabled": true, "fingerprint": "chrome"},
    "reality": {"enabled": true, "public_key": "...", "short_id": "..."}
  },
  "transport": {
    "type": "xhttp",
    "mode": "stream-one",
    "host": "example.com",
    "path": "/xhttp",
    "x_padding_bytes": "100-1000"
  }
}
```

**URI-параметры (что читаем из подписки):** `type=xhttp`, `mode=auto|packet-up|stream-up|stream-one`, `path`, `host`, `xPaddingBytes` / `x_padding_bytes`, `noGRPCHeader` / `no_grpc_header`. Чтение — через существующий `queryGetFold` (case-insensitive, `node_parser_transport.go:12`), нормализация значений — лежит на ядре.

## Фазы

Порядок строгий: парсер → эмиттер → round-trip → snapping в реальную схему → тесты. Каждая фаза имеет конкретный verify-шаг. **Фазы 1-5 не должны вливаться в релиз до закрытия SPEC 072** (см. § Принцип очерёдности) — иначе сгенерированный конфиг с `type=xhttp` уронит стоковое ядро на load.

**Фаза 1 — Парсинг XHTTP из VLESS/Trojan URI.**
Deliverable: в `node_parser_transport.go` отделить `xhttp` от `httpupgrade`. Новый хелпер:
```go
func xhttpTransportFromQuery(q url.Values) map[string]interface{} {
    t := map[string]interface{}{"type": "xhttp"}
    if v := strings.TrimSpace(queryGetFold(q, "mode")); v != "" { t["mode"] = v }
    if p := queryGetFold(q, "path"); p != "" { t["path"] = p }
    if h := queryGetFold(q, "host"); h != "" { t["host"] = h }
    if pad := xhttpPadding(q); pad != "" { t["x_padding_bytes"] = pad } // xPaddingBytes|x_padding_bytes
    if noGRPC(q) { t["no_grpc_header"] = true }                          // noGRPCHeader|no_grpc_header
    return t
}
```
`case "xhttp"` в `uriTransportFromQuery` вызывает его; `case "httpupgrade"` — прежний код. VLESS/Trojan уже зовут `uriTransportFromQuery` (`node_parser_core.go:513`, `:667`) — изменений в вызывающих нет.
Verify: unit-тест на VLESS-URI с `type=xhttp&mode=stream-one&path=/x&host=h&xPaddingBytes=100-1000` → `Outbound["transport"]` содержит все поля с `type==xhttp`.

**Фаза 2 — Парсинг XHTTP из VMess.**
Deliverable: убрать `network=="xhttp" → "httpupgrade"` ремап в `node_parser_core.go:562-564` и парный в `node_parser_vmess.go:236-237`; добавить `case network == "xhttp":` строящий xhttp-transport тем же хелпером. VMess host/path берутся из тех же ключей, что и в соседних ветках (`host`, fallback `sni`).
Verify: unit на VMess-узле (base64-JSON или query-форме) с `network=xhttp` → transport `type==xhttp`, не `httpupgrade`.

**Фаза 3 — Эмиссия XHTTP в config.json.**
Deliverable: в `appendOutboundTransportParts` после `service_name` (стр. 67) добавить:
```go
if mode, ok := transport["mode"].(string); ok && mode != "" {
    transportParts = append(transportParts, fmt.Sprintf(`"mode":%s`, marshalJSONString(mode)))
}
if pad, ok := transport["x_padding_bytes"].(string); ok && pad != "" {
    transportParts = append(transportParts, fmt.Sprintf(`"x_padding_bytes":%s`, marshalJSONString(pad)))
}
if v, ok := transport["no_grpc_header"].(bool); ok && v {
    transportParts = append(transportParts, `"no_grpc_header":true`)
}
```
Плюс расширить ветку `headers` (стр. 68) на `map[string]interface{}` (парсер кладёт interface-map, не `map[string]string`) — иначе headers молча дропаются для xhttp.
Verify: unit на эмиттере — transport-map с xhttp-полями → JSON содержит `"type":"xhttp"`, `"mode":...`, `"x_padding_bytes":...`. И golden: VLESS+XHTTP+Reality URI → `config.json` равен эталону из § Входные данные.

**Фаза 4 — Round-trip обратно в share-URI.**
Deliverable: в `shareuri_helpers.go` исправить `case "httpupgrade"` (стр. 149-150) — он должен ставить `type=httpupgrade`, не `xhttp`; добавить `case "xhttp":` ставящий `type=xhttp` + `mode`/`path`/`host`/`x_padding_bytes`/`no_grpc_header`.
Verify: round-trip тест — URI(`type=xhttp&mode=...`) → outbound → `ShareURIFromOutbound` → URI с теми же query-параметрами (с точностью до порядка). Отдельный тест: `httpupgrade`-outbound → URI `type=httpupgrade` (регресс на исправление путаницы).

**Фаза 5 — Verify Outbounds-редактора (Raw-JSON-таб).**
Deliverable: правок кода не ожидается (`ParsedNode.Outbound`/`OutboundConfig.Options` schema-permissive). Verify: вручную/тестом — открыть outbound с xhttp-transport в `edit_dialog.go`, переключиться на Raw-таб, сохранить без правок → xhttp-transport со всеми полями сохранён байт-в-байт (через `wizardbusiness.ResolveMergedOutbound`, как Preview-таб, см. `edit_dialog.go:69`). Если поля теряются — это баг проброса (фаза 3), а не UI.

**Фаза 6 — Тесты (acceptance).**
- Unit: парсер VLESS/Trojan/VMess (фазы 1-2), эмиттер (фаза 3), round-trip (фаза 4).
- Golden: эталонный `config.json` для VLESS+XHTTP+Reality; diff показывает только корректный xhttp-блок.
- Регресс: существующие `httpupgrade`-узлы по-прежнему парсятся в `type=httpupgrade` (не съехали в xhttp).
- Матрица режимов: `mode ∈ {auto, packet-up, stream-up, stream-one}` парсится и эмитится дословно.
Verify: `go test ./core/config/...` зелёный; существующие subscription-тесты не сломаны.

## Риски и открытые вопросы

1. **Реальное решение мейнтейнера: что делать со стоковым ядром.** После этой SPEC лаунчер генерит `transport.type=xhttp`. Если у юзера всё ещё стоковый sing-box (фича-гейт `with_xhttp` отсутствует), ядро **reject'ит** конфиг целиком на load. Варианты: **(A)** жёстко завязать на SPEC 072 — генерить xhttp только когда `RequiredCoreVersion` указывает на fork-сборку (но версия пинится константой, рантайм-детекта build-тегов у лаунчера нет); **(B)** сохранить деградацию в `httpupgrade` как fallback, переключаемый по версии ядра; **(C)** считать, что после релиза с fork-ядром стокового не остаётся, и не делать fallback. Рекомендация: **(C)** + строгая зависимость от 072 в release-checklist (см. § Принцип очерёдности). Решение за мейнтейнером — задокументировать в release notes.

2. **`mode` семантика — контракт ядра (уточнено по `lx-config.md`).** `auto` использует `packet-up` (live-validated против Xray/3x-ui). `stream-one` имеет известный баг downlink-framing — выбирать только осознанно. Лаунчер прокидывает `mode` как есть и ничего не навязывает. Открытый вопрос: предупреждать ли в UI про `stream-one`. Предложение: нет (out of scope).

3. **Padding wire-format — ПОДТВЕРЖДЁН (закрыто).** Per `lx-config.md`: padding несётся как `x_padding=<zeros>` в заголовке `Referer` (дефолтное размещение Xray), live-validated против реального Xray (3x-ui); сервер валидирует длину `x_padding` (дефолт 100–1000) и отвечает `400` без него. Лаунчер лишь прокидывает `x_padding_bytes` — размещение на стороне ядра. Прежнее опасение про standalone `X-Padding`-заголовок снято. (Версии клиента/сервера Xray желательно держать совместимыми — XHTTP быстро эволюционирует.)

4. **`headers` тип-несовпадение.** URI-парсер кладёт `headers` как `map[string]interface{}`, а нынешний эмиттер (`outbound_jsonbuilder.go:68`) ждёт `map[string]string` — иначе headers молча дропаются. Фаза 3 обязана покрыть обе формы, иначе extra-headers (User-Agent override и т.п.) теряются тихо.

5. **`httpupgrade ⇄ xhttp` путаница — двунаправленный баг.** Сейчас она «самосогласована» (вход и выход оба обзывают `xhttp`↔`httpupgrade`), поэтому round-trip случайно сходится. После разделения типов нужно убедиться, что **не** сломали юзеров, чьи рабочие узлы реально были `httpupgrade`, помеченные `type=xhttp` в исходной подписке. Открытый вопрос: встречаются ли в дикой природе подписки, где `type=xhttp` означает именно httpupgrade-совместимый сервер (старые мисконфиги). Митигейт: регресс-тесты + явный пункт в release notes.

6. **Forward-compat дропа неизвестных transport-полей.** Эмиттер по-прежнему whitelist'ит поля. Будущие XHTTP-поля (если fork добавит) будут молча дропаться. Не чиним сейчас (YAGNI), но стоит добавить debug-лог дропнутых transport-ключей в `appendOutboundTransportParts` — отдельная мелочь, можно в этой же SPEC опционально.

## Принцип очерёдности

Три SPEC'и из fork-интеграции упорядочены по жёсткости зависимостей:

1. **SPEC 072 (миграция ядра на `sing-box-lx` v1.13.13-lx.1) — ПЕРВАЯ, блокирующая обе остальные.** Пока `RequiredCoreVersion` не указывает на fork-сборку с `with_xhttp`, эта SPEC (071) не может быть зарелизена: сгенерированный `xhttp`-конфиг отклоняется стоковым ядром на load-time (фича-гейты строгие). 072 включает bump `RequiredCoreVersion` (`internal/constants/constants.go:61`), смену downloader-URL (`core/core_downloader.go:139`, `SagerNet/sing-box` → `Leadaxe/sing-box-lx`), релаксацию version-regex (`internal/constants/constants_test.go:18`, под суффикс `-lx.1`), и решение по Win7/`windows-386` (в fork-релизе нет 386-ассета).

2. **SPEC 071 (XHTTP, эта) — ПОСЛЕ 072.** Чисто launcher-side: парсер + эмиттер + round-trip. Не трогает ядро, downloader, version-pin. Может разрабатываться параллельно с 072 на ветке, но **вливаться/релизиться только после** того, как 072 в `main` и `RequiredCoreVersion` — fork-версия. Атомарность: фазы 1-5 — один merge; нельзя зарелизить генерацию `type=xhttp` раньше, чем ядро её принимает.

3. **SPEC AmneziaWG (третья, независима от 071) — тоже ПОСЛЕ 072.** Делит с 072 единственную зависимость (fork-ядро с `with_awg`). С 071 не пересекается по коду: XHTTP правит transport-эмиссию outbound'ов, AWG правит WireGuard-endpoints (`endpoints[]`, не `outbounds[]`) — разные парсеры (`node_parser_wireguard.go`), разные генераторы (`GenerateEndpointJSON`). 071 и AWG можно вести и мёржить в любом взаимном порядке после 072.

**Итог зависимостей:** `072 → {071, AWG}`. 072 — критический путь. 071 и AWG между собой не зависят и не конфликтуют по файлам. Release-checklist 071: подтвердить, что в `main` уже fork-ядро (072 закрыта), и приложить live-тест XHTTP против реального Xray-сервера (см. Риск 3).
