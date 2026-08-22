# Сравнение тестовых корпусов парсеров подписок

## 1. Лаунчер (Go): `core/config/subscription/`

### testdata/ — всего 2 файла
| Файл | Формат |
|---|---|
| `testdata/singbox_full_config.json` | Полный sing-box конфиг (log/dns/inbounds/outbounds: vless+reality, urltest-группа и т.д.) — вход для импорта sing-box-тел (`singbox_import_fixture_test.go`, через `//go:embed`) |
| `testdata/xray_provider_anon.json` | Анонимизированный Xray JSON **массив** конфигов в стиле реального провайдера (socks/http inbound'ы, vless vnext, dialerProxy) — вход для `xray_json_array_test.go` |

Весь остальной корпус — **inline-строки внутри 36 файлов `*_test.go`**:

- **URI-парсеры по протоколам**: `node_parser_test.go` (vless/vmess/trojan/ss/hysteria2/ssh/socks5/wireguard + real-world примеры), `node_parser_naive_test.go`, `node_parser_tuic_test.go`, `node_parser_anytls_test.go`, `node_parser_masque_test.go`, `node_parser_amnezia_test.go` (vpn:// контейнеры)
- **Фичи/регрессии**: `ws_early_data_test.go` (?ed=N, issue #96), `xhttp_v2_test.go` (полный набор XHTTP-полей + `extra`), `node_parser_transport_test.go` (uTLS fp allowlist, reality pbk-мусор, short_id-нормализация), `hysteria2_ports_test.go` (mport → server_ports), `userinfo_space_test.go`, `awg_test.go` + `awg_range_test.go` (AWG-поля, MTU clamp 1280, ranged H1–H4), `warp_reserved_test.go` (reserved=, masquerade ip/id/ib), `wgconf_text_test.go` (INI .conf текстом), `wireguard_robustness_test.go` (`/` в ключе, bare IP → CIDR)
- **Импорт тел**: `body_classify_test.go`, `decoder_test.go`, `singbox_import_test.go` / `_e2e_test.go` / `_fixture_test.go`, `singbox_sanitize_test.go` (fp/reality/flow/obfs-санитайзы, включая gecko), `xray_json_array_test.go`, `xray_protocols_test.go` (все протоколы Xray-массива, SPEC 102 xhttp `extra`), `xray_ownership_test.go` (владение именами, балансировщик → группа)
- **Пост-обработка**: `dedup_test.go`, `detour_test.go`, `detour_chain_test.go`, `disabled_nodes_test.go`, `e2e_disabled_flow_test.go`
- **Обратная эмиссия**: `share_uri_encode_test.go` (round-trip outbound → share-URI)
- **Мета/сеть**: `meta_test.go`, `fetcher_meta_test.go`, `manual_config_test.go`, `live_subscriptions_test.go` (build-tag `live`, реальные публичные списки)

## 2. LxBox (Dart): `app/test/`

### fixtures/ — файловый корпус (README описывает конвенции Parser v2)
- `vless/`: `reality_xtls.uri`, `ws_tls.uri`, `grpc_tls.uri`, `http2_tls.uri`, `edge_fallback_xhttp.uri`, `xhttp_obfs_full.uri` (golden §127 — все 15 XHTTP-полей)
- `vmess/`: `modern_ws_tls.uri`, `modern_tcp_no_tls.uri`, `grpc_tls.uri`, `edge_fallback_xhttp.uri`
- `trojan/`: `tls_tcp.uri`, `ws_tls.uri`, `grpc_tls.uri`
- `shadowsocks/`: `sip002_aead.uri`, `sip002_chacha.uri`, `ss2022_blake3.uri`, `legacy_base64.uri`
- `hysteria2/`: `basic.uri`, `hy2_alias.uri`, `obfs_salamander.uri`
- `tuic/`: `v5_basic.uri`, `v5_cubic.uri`, `v5_zero_rtt.uri`
- `ssh/`: `password.uri`, `host_key.uri`, `non_standard_port.uri`
- `socks/`: `no_auth.uri`, `auth.uri`, `socks_alias.uri`
- `wireguard/`: `uri_basic.uri`, `uri_psk_keepalive.uri`, `ini_basic.conf`
- `json/`: `singbox_vless_outbound.json`, `singbox_wg_endpoint.json`, `xray_array_reality.json`
- `base64/plain_uri_list.txt` (base64-обёртка URI-списка); `subscriptions/` — пустой (только README, задел под parity-тела); `clash_api/connections_oneway_live.json` — не про парсер

Данные синтетические (`example-N.com`, UUID `1111…`), комментарии через `#`. **Задуманных `*.expected.json` (golden) пока нет** — README называет их «после Фазы 2», в дереве не найдено ни одного.

### Тесты (основной корпус — тоже inline-строки)
- `parser/fixtures_parse_test.dart` — единственный потребитель URI-фикстур: smoke «парсится + emit + round-trip» по всем 9 протоколам
- `parser/`: `vless_test.dart` (матрица flow §115), `xhttp_test.dart` (§127 golden, `extra`, GET+packet-up), `ws_early_data_test.dart` + `ws_query_early_data_test.dart` (?ed= в пути **и** плоские ed/eh + `wsSettings.ed/.eh`), `utls_fingerprint_test.dart`, `reality_short_id_test.dart`, `path_double_encoding_test.dart`, `ech_import_test.dart`, `awg_test.dart` (MTU clamp), `amnezia_link_test.dart` (vpn://, ranged headers §112), `ini_parser_test.dart`, `wireguard_edge_test.dart`, `tuic_test.dart`, `uri_naive_test.dart`, `anytls_test.dart`, `http_proxy_test.dart`, `body_decoder_test.dart`, `json_parsers_test.dart` (sing-box entry + Xray outbound, masque §393, hysteria2 gecko §358, WARP reserved §219), `singbox_config_test.dart`, `xray_multiprotocol_test.dart`, `xray_dedup_test.dart`, `xray_auto_select_test.dart`, `round_trip_test.dart`
- `subscription/` — сетевой/жизненный цикл (fetch, cache, user-agent, `content_disposition_test.dart`, `wg_import_filename_test.dart` и т.д.), к корпусу парсера относится косвенно
- `builder/` — постобработка при сборке конфига: `heal_invalid_reality_test.dart`, `heal_unknown_utls_fingerprints_test.dart`, `heal_dangling_detours_test.dart`, `masque_tls_fragment_test.dart`, `detour_*`

## 3. Пересечения и различия (по фичам)

### Покрыто в обоих
| Фича | Go | Dart |
|---|---|---|
| WS early data `?ed=N` | `ws_early_data_test.go` | `ws_early_data_test.dart` |
| uTLS fp allowlist (HelloChrome_120 → chrome) | `node_parser_transport_test.go`, `singbox_sanitize_test.go` | `utls_fingerprint_test.dart`, `builder/heal_unknown_utls_fingerprints_test.dart` |
| Reality pbk-мусор (`pbk=enabled` на tls) | `node_parser_transport_test.go` | `builder/heal_invalid_reality_test.dart` |
| Reality short_id нечётной длины | `TestNormalizeRealityShortID` там же | `reality_short_id_test.dart` (§343) |
| AWG-поля + MTU clamp ≤1280 | `awg_test.go` | `awg_test.dart` |
| AWG 2.0 ranged headers H1–H4 | `awg_range_test.go` | `amnezia_link_test.dart` (§112) |
| vpn:// (Amnezia контейнер) | `node_parser_amnezia_test.go` | `amnezia_link_test.dart` |
| wgconf INI текстом | `wgconf_text_test.go` | `ini_parser_test.dart` + `fixtures/wireguard/ini_basic.conf` |
| WARP reserved | `warp_reserved_test.go` | `json_parsers_test.dart` (§219) |
| `/` в WG private key, bare IP→CIDR | `wireguard_robustness_test.go` | `wireguard_edge_test.dart` (§106) |
| Xray JSON array (все протоколы, dialerProxy, порядок) | `xray_json_array_test.go`, `xray_protocols_test.go` + `testdata/xray_provider_anon.json` | `json_parsers_test.dart`, `xray_multiprotocol_test.dart` + `fixtures/json/xray_array_reality.json` |
| Владение именами / балансировщик → группа | `xray_ownership_test.go` | `xray_auto_select_test.dart`, `xray_dedup_test.dart` |
| XHTTP расширенные поля + `extra` + отличие от httpupgrade | `xhttp_v2_test.go`, `share_uri_encode_test.go`, SPEC 102 в `xray_protocols_test.go` | `xhttp_test.dart` (§127) + `fixtures/vless/xhttp_obfs_full.uri` |
| Импорт sing-box конфига (outbounds/endpoints/группы/санитайз) | `singbox_import*_test.go`, `singbox_sanitize_test.go` + `testdata/singbox_full_config.json` | `singbox_config_test.dart` (§368), `json_parsers_test.dart` + `fixtures/json/singbox_*.json` |
| hysteria2 obfs gecko / unknown obfs | `singbox_sanitize_test.go` | `round_trip_test.dart`, `json_parsers_test.dart` (§358) |
| masque legacy→new ключи | `node_parser_masque_test.go` | `json_parsers_test.dart` (§393) |
| naive, tuic, anytls | `node_parser_{naive,tuic,anytls}_test.go` | `uri_naive_test.dart`, `tuic_test.dart`, `anytls_test.dart` (кейсы явно портированы, почти 1:1) |
| Классификация/декод тела (base64, CRLF, URL-safe) | `body_classify_test.go`, `decoder_test.go` | `body_decoder_test.dart` |
| Дедуп по identity | `dedup_test.go` | `xray_dedup_test.dart` |
| Flow-матрица vision (vision+transport гасится) | `node_parser_test.go` (vision-udp443 и т.д.) | `vless_test.dart` (§115, полнее) |

### Только в лаунчере (Go)
- **hysteria2 multi-port / mport → server_ports** — `hysteria2_ports_test.go`; в LxBox упоминаний mport нет вообще
- **Пробел в userinfo** — `userinfo_space_test.go`
- **masque как share-URI** (парсер схемы, а не только JSON) — `node_parser_masque_test.go`
- **Обратная эмиссия share-URI из произвольного outbound** — `share_uri_encode_test.go` (у LxBox только `spec.toUri()` round-trip)
- **detour-цепочки на уровне парсера** — `detour_chain_test.go` (в LxBox аналог живёт в `builder/`)
- **disabled-ноды с TTL** — `disabled_nodes_test.go`, `e2e_disabled_flow_test.go`
- **live-тест реальных публичных подписок** — `live_subscriptions_test.go` (тег `live`)
- **HWID-заголовки fetch'а** — `fetcher_meta_test.go`

### Только в LxBox (Dart)
- **ECH-параметры игнорируются с warning** — `ech_import_test.dart` (в Go тестов на ech нет)
- **Плоские query-параметры `ed`/`eh` и `wsSettings.ed/.eh`** — `ws_query_early_data_test.dart` (Go режет только хвост пути)
- **Двойное percent-кодирование пути / ALPN `http%252F1.1`** — `path_double_encoding_test.dart`, `round_trip_test.dart` (§151)
- **proxy-http/proxy+https схемы** — `http_proxy_test.dart` (в Go таких схем нет)
- **Xray `users[0].encryption` (§335), warning на неподдержанный протокол (§321)** — `json_parsers_test.dart`, `xray_multiprotocol_test.dart`
- **Файловый смоук-корпус по всем протоколам** — `fixtures_parse_test.dart` (в Go файловых URI-фикстур нет совсем)

## 4. Формат тестов и извлекаемость общего корпуса

**Go**: почти на 100% inline table-driven тесты — URI-строка и ожидания зашиты в `[]struct{...}` с ручными `if`-проверками полей `ParsedNode` / transport-map. Golden input→output файлов нет; `testdata/` — только 2 JSON-тела для импорта, и даже их ожидания (количество нод, теги) зашиты в код. Round-trip проверяется программно (`parse → emit → parse`).

**Dart**: гибрид. Есть файловый корпус `test/fixtures/**` (33 `.uri`/`.conf`/`.json`/`.txt`), но он используется только смоук-тестом `fixtures_parse_test.dart` и парой точечных тестов (`ini_parser_test.dart`, `json_parsers_test.dart`). Вся содержательная проверка — тоже inline-строки с `expect()` по полям `NodeSpec`/emit-map. Задекларированный в `fixtures/README.md` golden-формат `case_name.uri` + `case_name.expected.json` **не реализован** (ни одного `.expected.json` в дереве).

**Извлечь общий языконезависимый корпус — реально**, и LxBox уже спроектирован под него. Формат: каталог `corpus/<protocol>/<case>.uri` (или `.body` для целых тел + `.headers.json`) + `<case>.expected.json` с каноническим sing-box-узлом (outbound/endpoint map + опционально `warnings[]`).

Что нужно поменять:
- **Оба**: договориться о канонизации выхода (сортировка ключей, отсутствие дефолтных полей, представление warning'ов и «нода деградировала/отброшена» — например `"nodes": []` + `"warnings": ["..."]`). Расхождения уже известны: WG-дефолт MTU (Go честит URI, Dart ставит 1408), naming/tag-политика, порядок узлов Xray-массива.
- **Лаунчер**: написать один runner `corpus_test.go`, который читает каталог, гонит `ParseNode`/`ParseNodesFrom*` + `BuildOutbound` и сравнивает JSON с `expected` (плюс `-update` флаг для регенерации). Затем постепенно вынести inline-кейсы из `node_parser_*_test.go`, `ws_early_data_test.go`, `xhttp_v2_test.go` и т.д. в файлы; логические тесты чистых функций (`splitWSEarlyData`, `TestAWGHeaderOverlap`) оставить inline — они не корпусные.
- **LxBox**: доделать «Фазу 2» из собственного README — сгенерировать `*.expected.json` и заменить смоук в `fixtures_parse_test.dart` на строгое сравнение emit-map с golden. Указать путь к общему корпусу (git submodule / скопированный каталог) вместо локального `test/fixtures`.
- **Перенос дыр покрытия** станет бесплатным: Go-only кейсы (hysteria2 mport, userinfo space, masque-URI) и Dart-only (ech, плоские ed/eh, двойное кодирование, §335 encryption) превращаются в файлы, и второй проект сразу видит, что падает.

Ограничение: корпус закрывает только слой «вход → канонический узел». Пост-обработка (dedup, ownership, detour-цепочки, disabled, авто-группы) в обоих проектах завязана на инфраструктуру (hooks в Go, controllers в Dart) и в общий корпус выносится плохо — её оставить нативными тестами.