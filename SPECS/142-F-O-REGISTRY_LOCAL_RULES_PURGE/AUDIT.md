# Аудит рукописных правил об узлах вне движка реестра

Дата: 25.09.2026. Ветка `develop` (HEAD de39c82a), в рабочей копии лежат чужие незакоммиченные правки (`sanitize.go`, `transports.json`, `shadowsocks.json`, корпус). Их не трогал, номера строк в `nodeflow/sanitize.go` могут сдвинуться.
Режим: только чтение. Тесты не запускались.

Как искал:
1. Грепом по всему Go-коду, кроме `core/config/registry`: литералы схем и типов (`"vless"`…`"tailscale"`, `quic`, `reality`, `utls`), `case`/`==` по `Scheme`/`type`, литералы значений (`chrome`, `h3`, `xtls-rprx-vision`, `salamander`…), обращения `ob["tls"|"transport"|"flow"|…]`, `delete(` по картам узла, префиксы `xxx://`, функции `validate*` в UI.
2. Каждое совпадение читал в контексте и сверял с `contract/registry/**` (body.fields, mappers, relations, allowlists, warnings).
3. Все `refs.go`, `impl` и `warnings.*.go` в реестре проверил скриптом: существует ли файл, не вышла ли строка за конец файла, объявлен ли идентификатор в Go.

Условные обозначения входов: **URI** — ссылка / `.conf` / `awg://`; **Xray** — Xray JSON; **SB** — импорт sing-box JSON (подписка, файл); **MJ** — ручной JSON (вкладка JSON источника, `NodeFromManualConfigJSON`); **BK** — тело из бэкапа или state, которое при пересчёте проходит `materializeBody`; **UI** — формы конфигуратора; **BLD** — сборка config.json.

---

## Группа A. Удаляется сразу: правило уже есть в реестре или движке

### A1. Таблица sing-box type → схема в Go
- **Где:** `core/config/subscription/singbox_import.go:391-422` (`singboxSchemeByType`, `singboxTypeToScheme`, `SchemeFromSingboxType`). Потребители: `singbox_import.go:329`, `manual_config.go:52`, `singbox_sections_extract.go:227`, `core/config/canonical_emit.go:488` (`canonicalSchemeFromType`), `core/config/node_materialize.go:277`.
- **Что делает:** переводит тип ядра в схему лаунчера (`shadowsocks→ss` и т. д.). Тип вне таблицы → «unsupported outbound type».
- **Входы:** SB, MJ, BK (канон → узел), Xray (через `node_materialize`).
- **В реестре:** есть дубль. Поле `singbox_type` в `contract/registry/protocols/*.json`, обратная карта — `registry.SchemeForSingboxType` (`core/config/registry/registry.go:~1043`). Её уже используют `share_uri.go:44,111`. Файловый комментарий сам называет таблицу временной.
- **Замена:** `reg.SchemeForSingboxType(t)` плюс фильтр по `kind` протокола (`outbound|endpoint`). Для фильтра загрузчику нужен аксессор `ProtocolKind(scheme)`, сейчас `kind` не читается.
- **Расхождение, которое надо учесть:** в реестре есть `singbox_type: chain` (`chain.json`), а в Go-таблице chain нет. Тупая замена начнёт принимать `type: chain` из чужого конфига как узел. Нужно явное решение: либо фильтр по kind/ролям, либо реестровый признак «не импортируется как узел».
- **Тесты:** `core/config/singbox_check_import_test.go` (зовёт `SchemeFromSingboxType`) — правка на реестровую функцию.
- **Риск:** med.

### A2. `sanitizeSingboxTLS`: снятие `tls`, если это не объект или пустой объект
- **Где:** `core/config/subscription/singbox_sanitize.go:113-129`, вызов в `:69`.
- **Что делает:** удаляет `tls`, если он не map (WarnLog) или пустой `{}`. Кода на узле нет.
- **Входы:** только SB. На MJ/BK то же самое судит санитайзер реестра.
- **В реестре:** дубль. `tls` имеет тип `object` (`tls.json` body). Объект не того типа в `nodeflow` даёт `onInvalid` → `type_invalid` с кодом. Обоснование в комментарии («ядро отвергнет на разборе, реестр не выразит») неверно: `materializeBody` судит карту до того, как её увидит ядро.
- **Расхождение:** на входе SB правило срабатывает молча, на MJ/BK — с кодом `type_invalid`.
- **Замена:** удалить, работает существующий `type: object` + `on_invalid`. Поведение пустого `tls: {}` сверить с `objectField` (скорее всего эмитится пустым либо снимается `absent_when`). При необходимости добавить `absent_when: {}` или кейс корпуса.
- **Тесты:** `singbox_sanitize_test.go: TestSanitizeSingboxHandlesMalformedBlocks` уходит или переносится в корпус `body/singbox/tls_not_object`.
- **Риск:** low.

### A3. Предпроверка server/server_port и список «безадресных» типов при импорте sing-box
- **Где:** `singbox_import.go:340-349` и `:430` (`singboxTypeIsAddressless`: `wireguard || tailscale`).
- **Что делает:** отбраковывает запись без `server` или с портом вне 1..65535. Для wireguard и tailscale проверку пропускает.
- **Входы:** SB. На MJ/BK/Xray проверки нет, там судит реестр.
- **В реестре:** дубль. `server` имеет `required`, `server_port` — `format: port` в теле каждой адресной схемы. У wireguard и tailscale этих полей в корне нет, это уже записано схемой.
- **Замена:** удалить. Узел получит `Drop` с `field_missing`/`type_invalid` в `materializeBody`.
- **Что проверить:** отбраковка из `materializeBody` должна так же доезжать до `dropped[]` / `kind=unsupported` на своей позиции, как сейчас `result.rejected.addCoded`. Иначе строка пропадёт молча.
- **Тесты:** кейсы импорта «missing server» в `singbox_import*_test.go` (если есть) перейдут на коды реестра.
- **Риск:** med.

### A4. Мёртвый uTLS-allowlist и алиасы HelloChrome_* в Go
- **Где:** `core/config/subscription/node_parser_transport.go:20-84` (`singboxUTLSFingerprints`, `utlsAliasPrefixes`, `NormalizeUTLSFingerprint`, `normalizeUTLSFingerprintEx`).
- **Что делает:** второй allowlist отпечатков и перевод `HelloChrome_120 → chrome`. В рабочем коде не вызывается, только из тестов.
- **Входы:** никакие.
- **В реестре:** дубль. `tls.json` → `utls.fingerprint.values` + `on_invalid coerce chrome`, `allowlists.json`, `tls.json` mapper `utls_xray_hello_names` (в реестре есть `hellochrome`).
- **Замена:** удалить.
- **Тесты уходят:** `node_parser_transport_test.go: TestNormalizeUTLSFingerprint`; `registry_sync_test.go: TestRegistrySyncUTLSFingerprints` и строки 115-118 `TestRegistryAllowlistsRejectOutsiders`. `TestParseNode_VLESS_RawUTLSIdentifierFingerprint` остаётся, он идёт через движок.
- **Риск:** low.

### A5. Мёртвые хелперы WS Early Data
- **Где:** `node_parser_transport.go:91-176` (`wsEarlyDataHeaderName`, `splitWSEarlyData`, `applyWSEarlyData`, `decodeResidualPercent`, `queryGetFold`).
- **Что делает:** разбирает хвост `?ed=N` и пишет в тело `max_early_data` и `early_data_header_name`. Вызывается только из тестов.
- **В реестре:** дубль. `transports.json` ws: `max_early_data` (две формы записи) и `early_data_header_name default "Sec-WebSocket-Protocol"`, mapper `ws_early_data_path_suffix`.
- **Замена:** удалить.
- **Тесты уходят:** `ws_early_data_test.go: TestSplitWSEarlyData, TestApplyWSEarlyData`. Сквозные `TestParseNode_*_EarlyData*` остаются.
- **Риск:** low.

### A6. Мёртвый `outboundHasTransport` (запрет Vision поверх транспорта)
- **Где:** `core/config/outbound_jsonbuilder.go:35-48`.
- **Что делает:** признак «есть транспорт», чтобы гасить `flow`. Вызывается только из теста.
- **В реестре:** дубль. `vless.json` body `flow`: `conflicts … transport`, `unless_set: [encryption]`, код `vision_with_transport`.
- **Замена:** удалить.
- **Тесты:** `flow_transport_test.go: TestOutboundHasTransport` уходит.
- **Риск:** low.

### A7. Мёртвый сборщик тела MASQUE для WARP
- **Где:** `core/warp/masque.go:117-165` (`ToMasqueOutbound`).
- **Что делает:** собирает карту masque-outbound руками (`vhttp` по умолчанию h3, `tls.server_name`, `keep_alive_period`, mtu), в обход реестра. Рабочий путь — `ToMasqueURI` → движок. `ToMasqueOutbound` зовут только тесты.
- **Замена:** удалить.
- **Тесты:** `core/warp/masque_test.go: TestToMasqueOutbound, TestToMasqueOutbound_RequiresKeys` уходят.
- **Риск:** low.

### A8. Список схем «прямой ссылки»
- **Где:** `core/config/subscription/node_parser_core.go:17-45` (`IsDirectLink`). Вызовы: `source_loader.go:514,575`, `ui/configurator/business/parser.go:265`, `ui/configurator/dialogs/add_server_dialog.go:585`.
- **Что делает:** 25 префиксов `xxx://`, по которым строка считается узлом, а не URL подписки.
- **В реестре:** дубль. `scheme` + `aliases` протоколов (`hy`, `hy2`, `socks5/4/4a`, `naive+https|quic`, `proxy-http(s)`/`proxy+http(s)`, `awg`…) и `detect` секций `uri`. Контейнер `vpn://` описан в `containers.json`.
- **Замена:** `linkmap.SelectURI(plans, s)` (или `linkmap.SchemeOfText` + `detect`) плюс проверка контейнеров.
- **Ограничение:** `http://`/`https://` обязаны остаться URL подписки. У `http.json` scheme `http`, поэтому нужен фильтр по `detect`, а не по имени схемы. Новая схема в реестре сейчас не распознаётся как прямая ссылка, пока её не допишут сюда.
- **Тесты:** `IsDirectLink` дёргают `node_parser_test.go`, `awg_test.go`, `node_parser_http_test.go`, `node_parser_amnezia_test.go`, `core/integration_test.go`. Правка не нужна, если сигнатура останется.
- **Риск:** med.

### A9. Таблица «схема → endpoints[]»
- **Где:** `core/config/endpoint_schemes.go:30-40` (`endpointSchemes`: wireguard, tailscale).
- **Что делает:** решает, в какую секцию config.json эмитится узел.
- **В реестре:** дубль. `kind: endpoint` у `wireguard.json` и `tailscale.json`. Значения сейчас совпадают. Шапка файла («реестр в Go не читается») устарела.
- **Замена:** аксессор `registry.ProtocolKind(scheme)` (тот же, что в A1). Masque в реестре `outbound`, как и эмитится.
- **Тесты:** `contract_canon_test.go` и `corpus_singbox_check_test.go` зовут `IsEndpointScheme`. Сигнатура сохраняется.
- **Риск:** low.

### A10. Рукописный Xray socks → sing-box для хопа цепочки и дефолт `version: "5"`
- **Где:**
  - `core/config/subscription/xray_outbound_convert.go:67-110` (`xrayBuildJumpFromSocksOutbound`), вызов `xray_json_array.go:847-863`;
  - `core/config/outbound_generator.go:1525-1545` (`normalizeChainHop`: пустая схема → socks, `version` = "5").
- **Что делает:** собирает socks-хоп из `settings.servers[0]` (адрес, порт, user/pass) мимо движка и явно пишет `version: "5"`.
- **Входы:** Xray (dialerProxy → socks), старое состояние с `Jump`.
- **В реестре:** дубль и расхождение. У `socks.json` есть секция `mappers.xray`. `body.version` = `default: "5"`, а по CANON §2.4 дефолт ядра не материализуется. Тело хопа здесь не совпадает с телом того же socks, пришедшего ссылкой.
- **Замена:** хоп строить `parseXrayElementByEngine` (секция `socks#xray`). Дефолт `version` не дописывать: реестр и ядро трактуют пустое как 5, комментарий «ядро иначе отвергнет» противоречит `socks.json` impl. Проверить `sing-box check` одним кейсом корпуса.
- **Тесты:** Xray-цепочки с socks (`xray_json_array*_test.go`, `detour_chain_emit_test.go`) — возможна правка эталонов (исчезнет `version:"5"`).
- **Риск:** med.

### A11. Форма AWG пишет тело в обход реестра, если санитайзер что-то снял
- **Где:** `ui/configurator/tabs/source_awg_edit.go:300-372` (`writeAWGBody`, `awgPipelineLostFields`).
- **Что делает:** если `nodeflow.Sanitize` снял хоть одно поле (кроме tag/type) или вернул Drop, в узел пишется сырое `json.Marshal`, а не выход конвейера.
- **Входы:** UI.
- **В реестре:** обоснование устарело. Комментарий ссылается на то, что `h1`-`h4` «объявлены строками». Сейчас это `awg_range` с `range_order`, а relations `ranges_disjoint` покрывают заголовки.
- **Замена:** писать только `Emit(res.Clean)`. Drop показывать кодом, тело не подменять.
- **Тесты:** `source_awg_edit_test.go` (ветка отката).
- **Риск:** med. Правит живые узлы пользователя, нужен прогон на нескольких реальных AWG-телах.

---

## Группа B. Существующий примитив + правка данных реестра (или чтение реестра вместо своей копии)

### B1. `sanitizeSingboxMasqueLegacy`: плоские `network`/`sni`/`skip_cert_verify` у masque
- **Где:** `singbox_sanitize.go:84-98`, вызов `:68`.
- **Что делает:** молча удаляет три ключа (DebugLog), без переноса значений (D-078).
- **Входы:** только SB.
- **В реестре:** расхождение. `masque.json` body объявляет все три поля обычными полями с `deprecated: true` (order: `network`, `sni`, `skip_cert_verify`). Движок атрибут `deprecated` не исполняет. На MJ/BK эти ключи проходят санитайзер и уезжают в тело. По комментарию Go плоский `sni` рядом с `tls.server_name` роняет ядро fail-fast. Вдобавок `impl` у `sni`/`skip_cert_verify` пишет «Переносится в tls.server_name / tls.insecure», а это давно неправда (см. D).
- **Замена, вариант 1 (данные, без Go):** убрать три поля из `masque.body.fields`/`order`. Санитайзер снимет их общим `unknown_key` с кодом на всех входах.
- **Замена, вариант 2 (нужен код движка, фактически C):** дать `deprecated` действие «снять с кодом `field_deprecated`».
- **Тесты:** `node_parser_masque_test.go: TestSanitizeSingboxOutboundMap_MasqueLegacyStripped / _MasqueCanonicalSurvives` переходят в кейс корпуса `body/singbox/masque_legacy_flat_keys`.
- **Риск:** med. Живые MJ-узлы с этими ключами сейчас собираются, после правки потеряют их с кодом.

### B2. Анти-DPI-трансформы TLS: свой список «не применять к naive/masque»
- **Где:** `core/build/tls_transforms.go:104-120` (`isFirstHopTLSOutbound`: `case "direct","block","dns","selector","urltest","naive","masque"`), `:124-150`.
- **Что делает:** на сборке пишет `tls.fragment`, `record_fragment`, `fragment_fallback_delay` и мешает регистр SNI у первых хопов. Исключает naive и masque.
- **Входы:** BLD (все узлы).
- **В реестре:** частично есть, частично расходится. `tls.json` `fragment`/`record_fragment`/`fragment_fallback_delay` имеют `forbidden_for: ["naive"]`. Masque там не запрещён: реестр разрешает `tls.fragment` masque, сборка — нет. Служебные типы — не находка.
- **Замена:** трансформ спрашивает реестр: `reg.Field(scheme, "tls.fragment")` + `allowedFor(scheme)`. Для masque, если «ядро только предупреждает, смысла нет», добавить `masque` в `forbidden_for` (с `forbidden_codes` info) или в `advisory`. Тогда и MJ-узел masque с явным `tls.fragment` получит тот же код.
- **Тесты:** `tls_transforms_test.go: TestApplyTLSTransforms_SkipsDetourAndUtility` (кейсы naive/masque).
- **Риск:** low.

### B3. Каталог и дефолты `strip` цепочки в Go
- **Где:** `core/config/configtypes/types.go:1093-1116` (`ChainStripKeys`, `ChainStripDefault`). Потребители: `ui/configurator/tabs/source_chain_tab.go:122-418`, `core/config/chain_validate.go:44-52`.
- **Что делает:** закрытый список ключей `strip` и их дефолты при `strip_evasion`. Неизвестный ключ, по impl реестра, — фатал старта.
- **Входы:** UI, BLD, SB/MJ (тело `chain`).
- **В реестре:** нет. У `chain.json` `strip` — `type: object` без `fields`, каталога нет. Санитайзер не может снять неизвестный ключ, хотя ядро его отвергнет.
- **Замена (существующие примитивы):** `strip.fields` = 4 bool-поля с `default`, плюс атрибут дефолта при `strip_evasion` (нужен `default_when` с `when` по соседу, либо данные в `impl`). Неизвестный ключ снимется `unknown_key`. Go читает каталог из `reg.Field("chain","strip").Order`.
- **Тесты:** `chain_validate_test.go`, тесты формы цепочки.
- **Риск:** low.

### B4. Конфликт «цепочка снимает utls, а на хопе reality»
- **Где:** `core/config/chain_validate.go:17-72` (`NodeUsesReality`, `ChainStripsUTLS`, `ChainRealityConflict`).
- **Что делает:** предупреждает (`chain_strip_utls_on_reality`), если strip `tls.utls` применится к reality-хопу на позиции ≥ 1.
- **В реестре:** связь уже есть. `tls.reality.enabled` → `requires: [{path: tls.utls.enabled, code: field_requires}]`. Go-код держит копию этого знания: «reality без utls не стартует».
- **Замена:** применить к телу хопа каталог strip (B3) и прогнать `nodeflow.Sanitize`. Сработавший `requires` у `tls.reality.enabled` = конфликт. Правило «позиция ≥ 1» — данные `chain.json` (`rewrite`/`strip` «только к позициям ≥ 1»).
- **Тесты:** `chain_validate_test.go: ChainRealityConflict*`.
- **Риск:** low.

### B5. Форма обфускации AWG: списки полей, enum, границы, разрешение конфликтов
- **Где:** `ui/configurator/tabs/source_awg_edit.go`:
  - `:64-75` — `awgFixedIP="quic"`, дефолты jc/jmin/jmax/ib, `awgBrowsers`;
  - `:163-230` — `validateAWGSettings`: jc 0..128, jmin/jmax 0..1280, jmin ≤ jmax, LDH-домен;
  - `:268-272` — удаление i1..i5 при включении;
  - `:392-409` — `clearAWGSettings`: жёсткий список awg-ключей + `AWG3RootKeys()`;
  - `:411-433` — `clearRangedKeepalive`: диапазон → нижняя граница;
  - `:114` — `t == "wireguard"`.
- **Входы:** UI.
- **В реестре:**
  - `ib` enum `["", "chrome", "firefox", "curl"]` — дубль;
  - `ip` enum — дубль;
  - `id`: `format: host, max 253` — близко, но форма строже (требует точку);
  - `i1..i5` имеют `conflicts: [id, ip, ib]` — дубль, форма разрешает конфликт сама;
  - `jc`/`jmin`/`jmax` в реестре `min: 0` без `max` и без связи `jmin ≤ jmax` — форма строже (расхождение);
  - набор awg-полей = поля с `build_tag: with_awg` — дубль списка.
- **Замена:** списки полей и enum — из реестра (`build_tag == with_awg`, `Field.Values`). Проверку ввода делать пробным `nodeflow.Sanitize` и показывать его коды. Если `max` и `jmin ≤ jmax` — реальные ограничения ядра, внести в реестр (`max` существует; для `jmin ≤ jmax` нужна relation-kind `ordered`, см. C8). Предварительно проверить ядро.
- **Тесты:** `source_awg_edit_test.go`.
- **Риск:** low (UI).

### B6. Форма «добавить WireGuard» расходится с реестром
- **Где:** `ui/configurator/dialogs/add_server_dialog.go:742-835` (`validateWGKey`, `buildWireGuardURI`).
- **Что делает:**
  - MTU `576..9000`, в реестре `wireguard.body.mtu`: `min 576, max 1500` — **расхождение**: форма пропускает 1501..9000, санитайзер молча-с-кодом снимет mtu;
  - `allowed` по умолчанию `0.0.0.0/0`, в реестре `peers.allowed_ips.default_when` = `["0.0.0.0/0","::/0"]` — **расхождение**: узел из формы без IPv6-маршрута;
  - проверка ключа base64→32 байта — дубль `format: base64_32`;
  - keepalive 0..65535.
- **Входы:** UI → URI.
- **Замена:** не держать своих границ. Собрать URI, прогнать движок и санитайзер, показать коды. Дефолт `allowed` не подставлять, это сделает `default_when`.
- **Тесты:** `add_server_dialog_test.go` (buildWireGuardURI).
- **Риск:** low.

### B7. Форма Tailscale: свои правила полей
- **Где:** `ui/configurator/dialogs/add_server_tailscale.go:143-215, 245-256`.
- **Что делает:**
  - `auth_key` обязателен — **расхождение**: в реестре `tailscale.auth_key` не `required` («без него — интерактивный вход»);
  - `exit_node` и `advertise_exit_node` взаимоисключены — дубль `conflicts`;
  - `exit_node_allow_lan_access` пишется только при `exit_node` — в реестре связи нет;
  - `advertise_routes` проверяется как CIDR (дубль `format: cidr`) и без `0.0.0.0/0` (в реестре только в `impl`, правила нет).
- **Замена:** `requires: [{path: exit_node}]` у `exit_node_allow_lan_access` (существующий примитив). Запрет `0.0.0.0/0`/`::/0` в `advertise_routes` пока не выражается существующим (нужен `item_forbidden_values`, см. C9). Обязательность `auth_key` — решение владельца.
- **Риск:** low.

### B8. UI-подписи узла: свои таблицы протоколов, и они расходятся между собой
- **Где:**
  - `ui/configurator/business/config_node_labels.go:17-19` (`tcpLikeProtocols`: vless/vmess/trojan/anytls), `:21-29` (awg-ключи), `:41-51` (masque `vhttp`/legacy `network`, дефолт `h3`), `:83-137` (уровень AWG по полям);
  - `ui/configurator/tabs/preview_node_subtitle.go:121` (`case "wireguard","masque","hysteria","hysteria2","tuic"` → транспорта нет, остальным "tcp");
  - `ui/servers_node_info.go:488-545` (только чтение, не находка).
- **Что делает:** выводит подписи транспорта и защиты.
- **Расхождение:** два экрана считают «tcp» по-разному. ss/socks/http/ssh/naive в превью → "tcp", в списке → "". Masque без `vhttp` → "h3", а дефолт реестра `vhttp` = `auto`. Legacy `network` у masque (B1) читается, хотя больше не должен существовать.
- **В реестре:** «есть транспорт» = у схемы есть поле `transport` в body (данные есть). Дефолт `vhttp` = `auto`. Уровень AWG совпадает по смыслу с `kind_when` мапперов (`awg`/`awg3`), но на теле его нет (см. C3).
- **Замена:** `reg.Field(scheme,"transport")` и `Field.Default` — существующие данные. Уровень AWG — через C3.
- **Тесты:** `config_nodes_test.go` (`deriveTransport`).
- **Риск:** low.

### B9. Enum-списки в диалоге WARP и в миграции masque
- **Где:**
  - `ui/configurator/dialogs/warp_dialog.go:169-175` (ip: quic/dns/stun/sip, ib: chrome/firefox/curl), `:279` (vhttp: auto/h3/h2);
  - `core/warp/masque.go:236-240` (`ApplyNodeOptions`: иное → h3);
  - `core/state/masque_uri_migration.go:25` (`masqueVHTTPValues`).
- **В реестре:** дубль. `wireguard.ip/ib` enum, `masque.vhttp` enum `["", "h3","h2","auto"]`. Наборы совпадают.
- **Замена:** читать `Field.Values`. Миграцию можно оставить: разовая, привязана к истории.
- **Риск:** low.

### B10. Имя параметра и код отказа транспорта в движке linkmap
- **Где:** `core/config/linkmap/exec.go:354-359` (сравнение с `"transport_header_unsupported"`, параметр `"transport"`), `:1794-1795` (параметр `"transport"`).
- **Что делает:** в движке зашиты имя кода и имя параметра конкретного блока. Это нарушение «в движке нет частных имён», хоть и мелкое.
- **Замена:** имя параметра брать из `warnings.json` `params` кода (`on_invalid.code`). Текст сообщения — из `WarningText` реестра, а не из ветки `if code == …`.
- **Тесты:** `linkmap/no_scheme_names_test.go` — стоит расширить на литералы кодов.
- **Риск:** low.

---

## Группа C. Нужен новый примитив движка

Все примитивы ниже общие, без имён схем.

### C1. Hysteria v1: `obfs` объектом → строка-пароль
- **Где:** `singbox_sanitize.go:137-170` (`sanitizeSingboxHysteriaObfs`), вызов `:70`.
- **Что делает:** `obfs:{type,password}` → `obfs: password`. Без пароля или странная форма → снять. Пустая строка → снять. Только WarnLog, кода нет.
- **Входы:** только SB. На MJ/BK объект упадёт в `type_invalid` и снимется: obfs потерян, узел жив, код есть. **Расхождение входов.**
- **В реестре:** описано, но не исполняется: `hysteria.json` mapper `obfs_object_to_string` (impl указывает на Go). Поле `obfs` — `string` без `on_invalid`.
- **Новый примитив:** `on_invalid: {action: "unwrap", key: "password", code: "obfs_object_flattened"}`. Если значение-объект несёт ключ `key` со скаляром, берётся он (с кодом info). Иначе обычный `drop`. Общий: «значение пришло обёрткой соседнего диалекта».
- **Тесты:** `hysteria_v1_test.go: TestSingboxHysteriaObfsObjectFlattened` переходит в корпус `body/singbox/hysteria_obfs_object`.
- **Риск:** med.

### C2. «Главный секрет» узла (`ParsedNode.UUID`): три рукописные копии, дающие разный результат
- **Где:**
  - `singbox_import.go:433-448` (`singboxCredentialFromMap`), вызовы `:374`, `manual_config.go:73`, `xray_element_engine.go:105`;
  - `core/config/canonical_emit.go:494-506` (`canonicalCredential`), вызов `:199`;
  - `core/config/subscription/node_parser_engine.go:221-268` (`credentialFromBody`: `body["uuid"]` литералом, иначе `userinfo.into[0]`).
- **Что делает:** решает, какое поле тела кладётся в UUID (его читают skip-фильтры `outbound_filter.go` и превью).
- **Расхождение по входам:**
  - ссылка ssh/naive/socks/http даёт username (`into[0]`);
  - SB/MJ/Xray/BK для тех же схем дают `""`;
  - skip-фильтр по UUID для ssh-узла срабатывает у ссылки и не срабатывает у JSON.
  - Xray-вход сознательно переиспользует SB-копию.
- **В реестре:** нет. `secret` не годится (у naive секрет — password, а UUID — username), `userinfo.into[0]` не годится (у vmess/ss первым идёт method).
- **Новый примитив:** атрибут протокола `credential: "<путь тела>"` (или флаг поля `credential: true`). Одна функция в движке читает путь из реестра для всех входов.
- **Тесты:** корпусные ожидания `uuid`/credential (если есть), `canonical_emit_test.go`, skip-фильтры.
- **Риск:** med. Затрагивает фильтры пользователей, эталоны корпуса про identity.

### C3. Узловые гейты возможностей ядра: naive, tailscale, AWG3
- **Где:**
  - `core/config/outbound_generator.go:929-992` (`n.Scheme == "naive"`, `== SchemeTailscale`, `== "wireguard" && subscription.HasAWG3Fields(...)`);
  - `core/config/subscription/awg3.go:21-90` (`awg3RangeFields`, `awg3BoolFields`, `awg3HeaderKeyField`, `AWG3RootKeys`, `HasAWG3Fields` + диапазонный keepalive);
  - `core/build_report_feed.go:114-140` (Subject "naive"/"tailscale").
- **Что делает:** если ядро не умеет схему или фичу, узел выбрасывается целиком. Гейт полей по `min_core` (`nodeflow.GateForCore`) тут не годится: он снимает ключ, а узел остаётся.
- **Входы:** BLD (все узлы).
- **В реестре:**
  - есть данные без исполнителя: `naive.json` body `build_tag: with_naive_outbound`, `tailscale.json` body `build_tag: with_tailscale` (загрузчик `section` их не читает);
  - AWG3-поля размечены `min_core: 1.14.0-lx.32` + `build_tag: with_awg`;
  - признак «узел — AWG3» есть только в `kind_when` мапперов (на входе), на теле его нет;
  - список полей в `awg3.go` — дубль (там ещё и мёртвые `Param`-имена после перехода на движок).
- **Новый примитив, из двух частей:**
  1. `body.kinds` — именованные условия рода над телом (формат `Condition`: `any_set` + предикат значения, например `matches: "-"` для диапазона keepalive). Они же питают бейдж уровня AWG (B8) и `default_when.when.source_kind` на входе SB.
  2. В `GateForCore` — `CoreInfo.Capabilities` (результаты проб) и атрибут `on_core_unsupported: drop_node` / `requires_capability: "<имя>"` на уровне body или kind. Узел снимается с кодом реестра (`awg3_core_unsupported`, `tailscale_core_unsupported`, naive-код).
- **Тесты:** `node_parser_amnezia_test.go` (`HasAWG3Fields`), тесты гейтов в `outbound_generator`/`core_capabilities`, `build_report_feed_test.go`.
- **Риск:** high. Сборка всех узлов на старых ядрах, отчёт сборки.

### C4. REALITY: включение uTLS и замена пустого/`random` отпечатка на chrome при сборке
- **Где:** `core/config/subscription/node_parser_transport.go:178-233` (`EnforceRealityFingerprint`, `utlsJunkFallback`), `core/build/tls_transforms.go:197-249` (`HealRealityFingerprints`).
- **Что делает:** у каждого outbound с `tls.reality`:
  - создаёт или включает `tls.utls`;
  - пустой fp → `chrome`;
  - `random` → `chrome`. Только лог, кода на узле нет.
- **Входы:** BLD (все пути).
- **Расхождения:**
  1. Реестр говорит `reality.enabled requires utls.enabled` (действие — **снять** reality.enabled с `field_requires`), а сборка **чинит** — два разных ответа на один вопрос.
  2. Явный `random` из MJ/SB неотличим от «нашего неявного» и тоже молча меняется на chrome. Это противоречит принципу из того же комментария («явный отпечаток — как есть»).
  3. Неявный `random` пишет маппер (`tls.json` mapper `fp_empty_defaults_to_random`), а сборка его стирает — правило в двух местах.
- **Новый или изменённый примитив:**
  - (a) у `requires` — действие `materialize` / `imply`: «при X задать Y=значение» вместо снятия X, с кодом;
  - (b) условный дефолт маппера: `default_when.when.any_set: [tls.reality…]` на уровне маппера — при pbk не писать `random`, а оставлять пусто, дальше `default_when` санитайзера ставит `chrome`.
  - Сборочный хил после этого удаляется.
- **Тесты:** `core/build/reality_fingerprint_test.go: TestRealityFingerprintAllEntryPaths`, `reality_key_share_gate_test.go`, корпус reality.
- **Риск:** high. Все reality-узлы; поведение против Xray ≥ v26.9.8.

### C5. Xray `dialerProxy` → служебный `freedom` с `fragment` → `tls.fragment = true`
- **Где:** `core/config/subscription/xray_json_array.go:757-770, 814-835` (`xrayFreedomFragmentSpec`, `nodeOutboundTLSEnabled`, `applyXrayFreedomFragment`).
- **Что делает:** по ссылке на соседний элемент документа ставит `tls.fragment` в тело узла.
- **Входы:** Xray.
- **В реестре:** нет. Маппер Xray работает по одному элементу, межэлементных ссылок не знает.
- **Новый примитив:** `overlay` по целевому элементу ссылки (`ref: streamSettings.sockopt.dialerProxy`, `when: target.protocol in [...] && target.settings.fragment set`, `sets: {tls.fragment: true}`, `requires: tls.enabled`). Общий: «свойство, заданное соседом по документу».
- **Тесты:** Xray-кейсы с freedom-fragment (`xray_json_array*_test.go`, корпус xray).
- **Риск:** med.

### C6. Признак «ссылка несёт приватный ключ»
- **Где:** `core/config/subscription/share_uri_secret.go:16-52` (`shareURISecretCarrierFields`: ssh/wireguard/masque → `private_key`).
- **Что делает:** перед выдачей share-URI спрашивает подтверждение.
- **Входы:** UI (копирование ссылки).
- **В реестре:** нет. `secret: true` стоит и на паролях/uuid, которые под правило не подпадают (решение владельца).
- **Новый примитив:** уровень секрета у поля: `secret: "private_key"` (или флаг `share_confirm: true`). Предикат обходит `Field` схемы.
- **Тесты:** `share_uri_secret_test.go`.
- **Риск:** low.

### C7. Роль узла «годится выходом» (tailscale без `exit_node`)
- **Где:** `core/config/configtypes/types.go:915-945` (`IsExitCapable`: `n.Scheme != SchemeTailscale` → true, иначе по `exit_node`).
- **Что делает:** исключает узел из пулов Направлений.
- **Входы:** все (модель).
- **В реестре:** нет. Есть только `exit_node conflicts advertise_exit_node`.
- **Новый примитив:** атрибут протокола `exit_capable_when: {any_set: [exit_node]}` (по умолчанию «всегда»). Движок — одна общая функция по `Condition`.
- **Риск:** low.

### C8. Упорядоченность пары полей (`jmin ≤ jmax`)
- Сейчас правило живёт только в UI (B5). В реестре `jmin requires jmax`, порядка нет.
- **Новый примитив:** `relations.kind: "ordered"` (`paths: [a, b]`, `action`, `code`), рядом с `ranges_disjoint`.
- Сначала проверить в ядре, падает ли оно при `jmin > jmax`.
- **Риск:** low.

### C9. Запрещённые элементы списка (`advertise_routes` без `0.0.0.0/0`, `::/0`)
- UI-правило (B7). В реестре только текст в `impl`.
- **Новый примитив:** `item_forbidden_values: [...]` + `on_item_invalid` (существующий). Через `item_pattern` без lookaround не выражается.
- **Риск:** low.

### C10. Контейнер Amnezia: подъём MTU из `last_config` и подстановка `$PRIMARY_DNS`
- **Где:** `core/config/subscription/node_parser_amnezia.go:393-414` (knownKeys config/last_config/awg/wireguard), `:419-490` (`amneziaPrepareConf`, `amneziaMTUValue`).
- **Что делает:** правит INI до движка: дописывает `MTU` из соседнего ключа профиля, заменяет плейсхолдеры DNS на `dns1`/`dns2`, выбрасывает неразрешённые.
- **Входы:** URI (`vpn://`).
- **В реестре:** только описание прозой в `containers.json → amnezia_vpn_link` (go/dart), примитивов контейнера нет.
- **Новый примитив:** у контейнера:
  - `lift: [{from: "$sibling.mtu", to: "ini.Interface.MTU", unless_set: true}]`;
  - `substitute: {"$PRIMARY_DNS": "$root.dns1", …, on_unresolved: "drop_item"}`.
- **Риск:** med (мало кейсов корпуса). Приоритет низкий.

---

## Группа D. Устаревшие `impl` / `refs.go` / `go` в реестре

### D1. `refs.go` протоколов почти целиком устарели
Проверено скриптом, 144 записи:
- **38** указывают на несуществующие файлы. Все рукописные парсеры и эмиттеры удалены SPEC 133:
  - `node_parser_{anytls,http,hysteria,hysteria2,masque,naive,ss,socks,ssh,tuic,vmess,wireguard}.go`;
  - `shareuri_*.go` (15 файлов), `shareuri_helpers.go`;
  - `core/config/outbound_tls_emit.go`.
- **27** ссылаются на строки за концом файла:
  - `node_parser_core.go` (292 строки, ссылки до :773);
  - `node_parser_transport.go` (233, ссылки до :744);
  - `singbox_sanitize.go` (190, ссылки :221-263);
  - `hysteria2_ports.go` (172, :75-179);
  - `outbound_jsonbuilder.go` (48, :84-206).
- **66** «существуют по номеру строки», но файлы многократно переписаны. Содержимое под номерами не проверено, считать устаревшими.
- Затронуты все протоколы, `tls.json`, `transports.json`, `containers.json`.

**Предложение:**
- убрать `refs.go` с номерами строк или перевести на имена функций/пакетов движка (`core/config/linkmap`, `nodeflow`);
- добавить в `contract/tools` (gendocs/lint) проверку существования файла и идентификатора — ровно та, что делалась для этого аудита.

Риск правки — low (документация), но это правка `contract/registry`: нужен бамп `contract/VERSION`, changelog и параграф `TASKS_LXBOX.md`.

### D2. `mapper[]`-записи (id/from/to/kind/impl) с мёртвыми ссылками
| Файл / id | impl указывает на | Статус |
|---|---|---|
| `hysteria.json` `quic_has_no_utls_or_reality` | `singbox_sanitize.go`, набор `quicOutboundTypes` | набора нет (0 вхождений). Правило теперь `tls.json` utls/reality `forbidden_for` + `forbidden_codes` → `tls_not_applicable_quic` |
| `hysteria.json` `mbps_unit_suffix` | `node_parser_hysteria.go:94 hysteriaMbps` | файла и функции нет |
| `hysteria.json` `obfs_object_to_string` | `singbox_sanitize.go` | пока верно, уйдёт с C1 |
| `hysteria2.json` `mport_range_spec` | `node_parser_hysteria2.go:24` | файла нет (функция `hysteria2MportSpecToSingBoxServerPorts` жива в `hysteria2_ports.go`) |
| `masque.json` `vhttp_empty_defaults_to_h3` | `node_parser_masque.go:104-106` | файла нет. Вдобавок дефолт `vhttp` в body = `auto`, а id говорит h3 — сверить |
| `masque.json` `singbox_flat_fields_stripped` | `sanitizeSingboxMasqueLegacy` | верно, уйдёт с B1 |
| `naive.json` `broken_header_pair_skipped` | `node_parser_naive.go` | файла нет |
| `socks.json` `socks_scheme_is_version` | `node_parser_socks.go`, `socksVersionByScheme`/`ForScheme`/`SchemeForVersion` | ничего нет |
| `tuic.json` `heartbeat_bare_number` | `node_parser_tuic.go:10 normalizeTuicHeartbeat` | нет |
| `vless.json` `vision_udp443_is_a_compound_name`, `packet_encoding_none_means_absent` | `node_parser_core.go:730, :737-745` | строк нет (файл 292 строки) |
| `vmess.json` `legacy_cleartext_fallback` | `node_parser_vmess.go` | нет |
| `wireguard.json` `wg_key_spelling_to_std_base64`, `bare_ip_gets_prefix` | `node_parser_wireguard.go:normalizeWGKey` | нет |
| `tls.json` mapper[0..7] | `vlessTLSFromNode`, `trojanTLSFromNode`, `shouldVLESSSkipTLSForPort`, `applyTLSQueryExtras`, `tlsServerNameFromQuery`, `noteECHIgnored`, `utlsFingerprintOrDefault`, `node_parser_anytls.go` | ничего нет. `normalizeUTLSFingerprintEx`/`utlsAliasPrefixes` существуют, но мёртвые (A4) |
| `transports.json` mapper[0..1] | `splitWSEarlyData`/`applyWSEarlyData` (мёртвые, A5), `uriTransportFromQuery` | последней нет |
| `group.json` mapper[0..3] | `singbox_groups.go`, `xray_balancer.go` (без строк) | файлы есть — ok |

### D3. Поля `impl` у полей body/uri с мёртвыми идентификаторами (выборка)
- **`hysteria.json`:**
  - `/uri/query/upmbps/impl` и `/body/fields/up_mbps/impl` — `hysteriaBandwidthOrDefault`, `sanitizeSingboxHysteriaBandwidth` («три копии») — их нет, работает `default_when`;
  - `/refs.go` — `singbox_sanitize.go (…обязательная полоса)` — полосы там нет.
- **`masque.json` body:** `sni.impl` «Переносится в tls.server_name», `skip_cert_verify.impl` «Переносится в tls.insecure» — неправда с D-078 (см. B1).
- **`vless.json` `flow.impl`** (uri): «Go — молча в sanitize» — теперь реестр с кодом `vision_with_transport`.
- **`wireguard.json`:**
  - `/uri/query/mtu/impl` → `awgClampMtu`;
  - `/body/fields/mtu/impl` → `awgMaxMTU`, `hasAWG3Params` (сам текст признаёт перенос);
  - `/body/fields/s1/impl` → `validateAWG3`.
- **`shadowsocks.json` `/uri/query/method/impl`** → `isValidShadowsocksMethod`.
- **`hysteria2.json` `/uri/query/fp/impl`** → `toSingboxForQuic`.
- **`anytls.json` / `tls.json`** → `utlsFingerprintOrDefault`, `buildAnyTLSTLS`, `vlessTLSFromNode`, `sanitizeSingboxReality`.
- **`transports.json` `/blocks/xray/xhttp/uplinkDataPlacement/impl`** → `noteXHTTPPlacementGuard`.
- **`tailscale.json` `refs.go`:**
  - `singbox_import.go:344-359/371-379` и `outbound_generator.go:1054-1086` — строки уехали;
  - `endpoint_schemes.go:1` — таблица уйдёт с A9.

Всего в `impl` найдено ~70 lowerCamel-идентификаторов, не объявленных в Go. Часть — имена параметров Xray (`headerType`, `serviceName`, `multiMode`), это законно. Остальные — имена удалённых функций. Полный сырой список: `scratchpad/refs.txt` и `scratchpad/refsgo.txt`.

### D4. `warnings.json` → поле `go` у кодов: 15 из 80 устарели
| Код | Мёртвые ссылки |
|---|---|
| `reality_fp_not_chrome` | `vlessTLSFromNode`, `noteRealityFingerprint`, `buildAnyTLSTLS`, `node_parser_anytls.go` |
| `reality_short_id_invalid` | `node_parser_transport.go:908`, `normalizeRealityShortID`, `sanitizeSingboxReality` |
| `reality_key_share_invalid` | `vlessTLSFromNode`, `buildAnyTLSTLS`, `sanitizeSingboxReality`, `outbound_tls_emit.go` |
| `awg3_field_invalid` | `awg3.go:applyAWG3Fields` |
| `awg3_header_key_invalid`, `awg3_padding_too_short` | `awg3.go:validateAWG3`, `parseWireGuardURI` |
| `awg3_random_trailers_wide_headers` | `awg3.go:awg3RandomTrailersWithWideHeaders` (теперь relation `cooccurrence` в `wireguard.json`) |
| `awg3_core_unsupported` / `tailscale_core_unsupported` | локальные `awg3Supported` / `tailscaleSupported` (переменные, не функции; формально живы в `outbound_generator.go:871-887`) |
| `template_var_undeclared` / `template_unknown_directive` | `core/template/substitute_canon.go: warnVarUndeclared / warnUnknownDirective` — функций нет |
| `transport_header_unsupported`, `xray_domain_strategy_ignored`, `xray_cert_chain_pin_unsupported`, `grpc_multi_mode_ignored` | ссылки на реестр — ложные срабатывания проверки, ok |

Реально устарели 11-13.

---

## Не находки (для полноты)
- `IsSingboxServiceType` / `IsSingboxGroupType` (`singbox_sanitize.go:20-42`), `xrayServiceProtocols` (`xray_protocols.go:31`), `graphEntry.isGroup/isChain` (`core/build/outbound_graph_sanitize.go:66-77`), `direction_twins.go`, `nodelink_resolve.go` — служебные типы и группы в роли «не узел».
- `core/config/tailscale_state_dir.go`, `core/daemon_manager.go:237-245` — платформенная раскладка каталога состояния, полей узла не судит.
- `core/warp/obfuscation.go`, `core/warp/account.go` — генерация значений аккаунта WARP (эмуляция провайдера). Выход идёт ссылкой через движок.
- `ui/servers_node_info.go:480-545` — только отображение полей.
- Движок `nodeflow` чист от имён схем (сравнения со `s.scheme` идут только через `allowed_for`/`forbidden_for`). В `linkmap` одна мелочь — B10.

---

## Сводная таблица

| # | Файл:строка | Правило | Входы | Реестр | Примитив | Тесты уходят/правятся | Риск |
|---|---|---|---|---|---|---|---|
| A1 | subscription/singbox_import.go:391-422 (+5 потребителей) | type→scheme | SB, MJ, BK, Xray | дубль (`singbox_type`) | `SchemeForSingboxType` + `ProtocolKind` (loader) | singbox_check_import_test | med |
| A2 | subscription/singbox_sanitize.go:113-129 | tls не объект / пуст → снять | SB | дубль (`type: object`) | существующий `on_invalid` | TestSanitizeSingboxHandlesMalformedBlocks | low |
| A3 | subscription/singbox_import.go:340-349, 430 | server/port обязательны, wg/ts безадресные | SB | дубль (`required`, `format: port`) | существующий | импорт-тесты «missing server» | med |
| A4 | subscription/node_parser_transport.go:20-84 | uTLS allowlist + HelloX-алиасы (мёртвый) | — | дубль | — | TestNormalizeUTLSFingerprint, TestRegistrySyncUTLSFingerprints | low |
| A5 | subscription/node_parser_transport.go:91-176 | WS `?ed=` (мёртвый) | — | дубль | — | TestSplitWSEarlyData, TestApplyWSEarlyData | low |
| A6 | config/outbound_jsonbuilder.go:35-48 | Vision vs transport (мёртвый) | — | дубль | — | TestOutboundHasTransport | low |
| A7 | warp/masque.go:117-165 | тело masque руками (мёртвый) | — | обход | — | TestToMasqueOutbound* | low |
| A8 | subscription/node_parser_core.go:17-45 | список схем прямой ссылки | URI, UI | дубль (`aliases`/`detect`) | `linkmap.SelectURI` | — | med |
| A9 | config/endpoint_schemes.go:30-40 | схема → endpoints[] | BLD | дубль (`kind`) | `ProtocolKind` | — | low |
| A10 | subscription/xray_outbound_convert.go:67-110; config/outbound_generator.go:1525-1545 | socks-хоп Xray руками, `version:"5"` | Xray, BK | дубль + расхождение (дефолт материализуется) | движок `socks#xray` | эталоны цепочек | med |
| A11 | ui/.../source_awg_edit.go:300-372 | запись тела мимо санитайзера при «потере» | UI | обход | — | source_awg_edit_test | med |
| B1 | subscription/singbox_sanitize.go:84-98 | masque legacy-ключи молча | SB (MJ/BK — нет!) | расхождение (`deprecated` не исполняется) | убрать поля из body → `unknown_key` | TestSanitizeSingboxOutboundMap_Masque* | med |
| B2 | build/tls_transforms.go:104-120 | fragment не для naive/masque | BLD | частично (`forbidden_for: naive`) | `forbidden_for` + чтение реестра | TestApplyTLSTransforms_SkipsDetourAndUtility | low |
| B3 | configtypes/types.go:1093-1116 | каталог strip цепочки | UI, BLD, SB/MJ | нет | `strip.fields` + `default` | chain_validate_test | low |
| B4 | config/chain_validate.go:17-72 | strip utls на reality-хопе | BLD, UI | связь `requires` есть | прогон Sanitize | chain_validate_test | low |
| B5 | ui/.../source_awg_edit.go:64-433 | enum/границы/конфликты AWG | UI | дубль + UI строже | `Values`, `build_tag`, пробный Sanitize | source_awg_edit_test | low |
| B6 | ui/.../add_server_dialog.go:742-835 | MTU 576..9000, allowed 0.0.0.0/0 | UI | **расхождение** (max 1500, `::/0`) | пробный Sanitize | add_server_dialog_test | low |
| B7 | ui/.../add_server_tailscale.go:143-256 | auth_key обязателен, LAN только с exit | UI | расхождение / нет | `requires` (+C9) | — | low |
| B8 | ui/.../config_node_labels.go:17-137; tabs/preview_node_subtitle.go:121 | подписи транспорта и уровня AWG | UI | данные есть | `Field("transport")`, `Default` (+C3) | config_nodes_test | low |
| B9 | ui/.../warp_dialog.go:169-279; warp/masque.go:236; state/masque_uri_migration.go:25 | enum ip/ib/vhttp | UI, миграция | дубль | `Field.Values` | — | low |
| B10 | linkmap/exec.go:354-359, 1794 | код/параметр транспорта в движке | URI, Xray | `warnings.params` | `params` из реестра | no_scheme_names_test (расширить) | low |
| C1 | subscription/singbox_sanitize.go:137-170 | hysteria obfs объект → строка | SB (MJ — иначе) | описано, не исполняется | `on_invalid: unwrap{key}` | TestSingboxHysteriaObfsObjectFlattened | med |
| C2 | singbox_import.go:433-448; canonical_emit.go:494-506; node_parser_engine.go:221-268 | credential → UUID | все | нет | `credential: <путь>` | canonical_emit_test, фильтры | med |
| C3 | outbound_generator.go:929-992; subscription/awg3.go:21-90 | узловые гейты naive/tailscale/AWG3 | BLD | данные частично (`build_tag`, `min_core`) | `body.kinds` + `requires_capability`/`on_core_unsupported` | amnezia_test, build_report_feed_test | **high** |
| C4 | subscription/node_parser_transport.go:178-233; build/tls_transforms.go:197-249 | reality → utls + fp chrome | BLD | **расхождение** (`requires` снимает vs хил) | `requires.action: imply` + условный дефолт маппера | reality_fingerprint_test | **high** |
| C5 | subscription/xray_json_array.go:757-835 | freedom-fragment → tls.fragment | Xray | нет | overlay по ссылке dialerProxy | xray-кейсы | med |
| C6 | subscription/share_uri_secret.go:16-52 | приватный ключ в ссылке | UI | нет | `secret: "private_key"` | share_uri_secret_test | low |
| C7 | configtypes/types.go:915-945 | tailscale без exit_node ≠ выход | модель | нет | `exit_capable_when` | — | low |
| C8 | (UI B5) | jmin ≤ jmax | UI | нет | relation `ordered` | — | low |
| C9 | (UI B7) | запрет 0.0.0.0/0 в advertise_routes | UI | только impl | `item_forbidden_values` | — | low |
| C10 | subscription/node_parser_amnezia.go:393-490 | MTU из last_config, $PRIMARY_DNS | URI (vpn://) | проза в containers.json | `lift`/`substitute` контейнера | amnezia_test | med |
| D1 | registry/**/refs.go | 38 мёртвых файлов, 27 строк за концом | — | — | lint в contract/tools | — | low |
| D2 | protocols/*.json mapper[] impl (≥20 записей), в т. ч. `quic_has_no_utls_or_reality` | мёртвые ссылки | — | — | правка данных | — | low |
| D3 | impl полей (masque sni/skip_cert_verify, hysteria up_mbps, vless flow, wireguard mtu…) | мёртвые/ложные | — | — | правка данных | — | low |
| D4 | warnings.json `go` (11-13 из 80) | мёртвые функции | — | — | правка данных | — | low |

**Итого:** A — 11, B — 10, C — 10, D — 4 блока (≈150 отдельных мёртвых ссылок).

**Порядок работ:**
1. A4-A7 — мёртвый код, можно удалять сразу.
2. A2, A9, A1, A3, A8, A10.
3. B1 и C1 — закрывают остаток `SanitizeSingboxOutboundMap` целиком.
4. C2.
5. C4 и C3 — отдельными SPEC, они крупные и с высоким риском.
6. D — одной правкой контракта, вместе с lint-проверкой.
