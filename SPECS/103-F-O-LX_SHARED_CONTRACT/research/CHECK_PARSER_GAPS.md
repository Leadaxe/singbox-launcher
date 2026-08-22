Все проверки завершены. Итоги по пунктам:

**1. Dart: naive+quic:// — НЕТ**
Диспетчер имеет только ветку `case 'naive+https':` — `/Users/macbook/projects/LxBox/app/lib/services/parser/uri_parsers.dart:59`. Строка `naive+quic` не встречается нигде в `lib/` (grep пуст); `naive_parser.dart` не различает варианты. Go поддерживает обе: `/Users/macbook/projects/singbox-launcher/core/config/subscription/node_parser_core.go:272-281,380`.

**2. Dart: vmess legacy cleartext без base64 — НЕТ (частично: только внутри base64)**
`parseVmess` жёстко требует успешный base64-декод: `/Users/macbook/projects/LxBox/app/lib/services/parser/uri_parsers/vmess_parser.dart:22-23` (`decodeBase64Safe` → null → отказ; `:`/`@` — невалидные base64-символы). Fallback `_vmessLegacy` (vmess_parser.dart:35-37, 97-126) разбирает `method:uuid@host:port`, но только над УЖЕ декодированной строкой, т.е. поддержан `vmess://base64(method:uuid@host:port)`. Замечание: Go ведёт себя так же — тоже требует base64 (node_parser_core.go:115-124, parseVMessLegacyCleartext в node_parser_vmess.go:92), так что паритет полный, а сырой cleartext не поддержан нигде.

**3. Dart: hysteria2 multi-port — НЕТ**
`hysteria2_parser.dart` (весь файл, строки 12-82) читает только sni/fp/alpn/obfs/up_mbps/down_mbps — ни `mport`, ни `ports`. Модель `Hysteria2Spec` не имеет поля портовых диапазонов: `/Users/macbook/projects/LxBox/app/lib/models/node_spec.dart:370-405`. Grep по `mport|server_ports|hop` в `lib/` пуст. Go поддерживает: node_parser_hysteria2.go:26-33 + hysteria2_ports.go:128-163.

**4. Dart: пробел в userinfo — ЧАСТИЧНО (нет явной обработки)**
Явного кода нет: единственный userinfo-фикс — `encodeUserInfoSlashes`, экранирует только `/`: `/Users/macbook/projects/LxBox/app/lib/services/parser/uri_utils.dart:191-202`. Парсеры опираются на `Uri.tryParse` (например vless_parser.dart:12-15 с `Uri.decodeComponent`); Dart-Uri по семантике SDK нормализует невалидный символ в `%20`, а не отвергает URI, поэтому нода, вероятно, выживает — но это неявное поведение SDK, не проверенное тестом (Dart SDK на этой машине нет, запуском не подтверждал; тестов на этот случай в `app/test/parser/` нет). Go имеет явный фикс: `percentEncodeUserinfoSpaces`, node_parser_core.go:42-76, вызов :288-291, тест userinfo_space_test.go:8.

**5. Go: одиночный JSON-объект как тело подписки — ЧАСТИЧНО (расхождение веток)**
- Классификатор ПРИНИМАЕТ: `ClassifySubscriptionBody` → `classifyJSONObjectBody` — `{"type":...}` → BodyKindSingboxOutbound (body_classify.go:134-135), `{"outbounds":[...]}`/`{"endpoints":[...]}` → BodyKindSingboxConfig (:137-138); дальше принят в `LoadNodesFromSourceEx` → `ParseSingboxBody` (source_loader.go:347-354).
- Но сетевой fetch-путь ОТВЕРГАЕТ до классификатора: fetcher.go:293 зовёт `DecodeSubscriptionContent`, и голый `{`-body падает с ошибкой «JSON configuration instead of subscription list» — decoder.go:68-71. Ошибка → LoadNodesFromSourceEx лишь логирует fetch-fail (source_loader.go:327-330), 0 нод.
- Объект ДОХОДИТ до классификатора двумя обходами: (a) base64-обёрнутый объект — decoder.go:33-52 декодирует и пропускает без проверки формы; (b) cache-путь — `buildBodyLookup` при decode-ошибке отдаёт raw как есть (core/rebuild_raw_cache.go:134-137), hook LookupCachedBody (source_loader.go:314-320) минует decoder. Т.е. ровно то расхождение, которое докстрок body_classify.go:10-13 описывает как «до SPEC 094», на fetch-пути живо до сих пор.

**6. Go: детект Clash YAML с подсказкой — НЕТ**
Grep `yaml|clash|proxies:` по core/ и internal/ пуст (единственный хит «clash» — имя переменной коллизии тегов, xray_json_array.go:121). YAML-тело в decoder.go упадёт либо в URI-ветку (:74-77, 0 нод), либо в generic ошибку base64 (:79) — без какого-либо hint. В Dart есть: JsonFlavor.clashYaml — body_decoder.dart:67, детект :201, обработка parse_all.dart:152, subscription_controller.dart:850 + services/parse_hints.dart.

**7. Go: proxy-http:// / proxy+https:// — НЕТ**
Полный набор схем Go: node_parser_core.go:20-36 (IsDirectLink) и switch :84-272 — никаких proxy-http/proxy+http вариантов (grep пуст по всему пакету). Dart имеет все 4 алиаса: uri_parsers.dart:70-74.

**8. Лимиты — ЧАСТИЧНО (расходятся)**
- Go: MaxURILength = 8192 — node_parser_core.go:40, enforcement :96-97; MaxNodesPerSubscription = 3000 — core/config/configtypes/types.go:74, enforcement source_loader.go:368, 410, 453, 575; плюс MaxSubscriptionResponseSize = 10 MB — fetcher.go:116.
- Dart: maxURILength = 65536 (в 8 раз больше Go) — uri_utils.dart:7, enforcement uri_parsers.dart:38 и amnezia_link.dart:22; cap числа нод — НЕТ: parse_all.dart без лимита, grep `maxNodes|take(N)` по lib/ пуст. Лимита размера тела в parser/subscription-сервисах тоже не нашёл.

**9. Go: ECH — ЧАСТИЧНО, всегда молча, warning'ов нет нигде**
- URI-путь: параметр `ech=` не читается ни одним share-URI парсером (grep `"ech"|echconfig|ech_|ech=` по core/config/subscription/ и core/config/ — ноль хитов) → уходит в node.Query и молча теряется на allowlist-эмиттере. Никакого warning.
- sing-box JSON-импорт: `tls.ech` проходит НАСКВОЗЬ нетронутым — `sanitizeSingboxTLS` (singbox_sanitize.go:121-158) на QUIC срезает только utls (:143-145) и reality (:147-149); коммент :47-48 утверждает «utls/reality/ech … снимаем их здесь», но кода удаления ech НЕТ — коммент расходится с кодом.
- Xray JSON-конвертер: ech-полей нет (grep пуст в xray_outbound_convert.go/xray_protocols.go) → отбрасываются молча при пересборке.

**10. Dart: экспорт wgconf INI из WireguardSpec — НЕТ**
`WireguardSpec.toUri()` эмитит только `wg://`-URI: node_spec.dart:777 → `toUriWireguard` node_spec_emit.dart:614. Единственный писатель `[Interface]/[Peer]` в Dart — WARP-специфичный `WarpAccount.toWireguardConf` (`/Users/macbook/projects/LxBox/app/lib/services/warp/warp_account.dart:155-189`), он строит conf из полей WARP-аккаунта, а не из WireguardSpec — обратного экспорта спеки в INI нет.