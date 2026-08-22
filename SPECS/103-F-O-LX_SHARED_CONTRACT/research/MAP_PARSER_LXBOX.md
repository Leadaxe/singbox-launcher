# Карта парсера подписок LxBox (Flutter/Dart, `/Users/macbook/projects/LxBox/app`)

## 1. Поддерживаемые URI-схемы и диспетчеризация

Диспетчер — функция `parseUri(String uri)` в `lib/services/parser/uri_parsers.dart:37`, switch по схеме на строках 43–83. Возвращает `NodeSpec?` (ошибки структуры → `null`-skip, не throw); лимит `maxURILength = 65536` (`uri_utils.dart:7`). Per-protocol парсеры лежат в `lib/services/parser/uri_parsers/` и ре-экспортируются из `uri_parsers.dart`.

| Схема(ы) | Парсер | Файл |
|---|---|---|
| `autogroup` (`kAutoGroupScheme`, синтетическая — узел автовыбора §322) | `parseAutoGroup` | `uri_parsers/auto_group_parser.dart` |
| `vless` | `parseVless` | `uri_parsers/vless_parser.dart` |
| `vmess` | `parseVmess` | `uri_parsers/vmess_parser.dart` |
| `trojan` | `parseTrojan` | `uri_parsers/trojan_parser.dart` |
| `ss` | `parseShadowsocks` | `uri_parsers/shadowsocks_parser.dart` |
| `hysteria2`, `hy2` | `parseHysteria2` | `uri_parsers/hysteria2_parser.dart` |
| `naive+https` | `parseNaive` | `uri_parsers/naive_parser.dart` |
| `anytls` (§269) | `parseAnyTls` | `uri_parsers/anytls_parser.dart` |
| `tuic` | `parseTuic` | `uri_parsers/tuic_parser.dart` |
| `ssh` | `parseSsh` | `uri_parsers/ssh_parser.dart` |
| `socks`, `socks5` | `parseSocks` | `uri_parsers/socks_parser.dart` |
| `proxy-http`, `proxy-https`, `proxy+http`, `proxy+https` (§222/§268) | `parseHttpProxy` | `uri_parsers/http_parser.dart` |
| `wg`, `wireguard`, `awg` (§097 AmneziaWG2-алиас) | `parseWireguardUri` | `uri_parsers/wireguard_parser.dart` |
| `masque` (§130 MASQUE-WARP CONNECT-IP) | `parseMasqueUri` | `uri_parsers/masque_parser.dart` |

Отдельно, ДО `parseUri`: `vpn://` (Amnezia-ссылка) перехватывается на уровне тела в `body_decoder.dart:86` → `decodeAmneziaLink` (`amnezia_link.dart:17`).

## 2. Входные форматы тела и классификация

Классификация тела — `decode(String body)` в `lib/services/parser/body_decoder.dart:80`, результат — sealed `DecodedBody` (`body_decoder.dart:9`): `UriLines | IniConfig | AmneziaConfig | JsonConfig | DecodeFailure`. Алгоритм:

0. `vpn://…` → `decodeAmneziaLink` (`amnezia_link.dart`): base64url от `qCompress(JSON,8)` (4 байта BE длины + zlib, анти-bomb cap 4 MiB) или несжатый base64-JSON; из `containers[]` берутся `awg`/`wireguard` → `last_config.config` (INI) → `AmneziaConfig(iniTexts)`.
1. Попытка **base64** (`_looksLikeBase64` ≥16 симв., `decodeBase64Safe` — все варианты std/url/±padding) + проверка UTF-8 (`_isLikelyUtf8`, <20% control-байтов) + `_isPlausiblePayload` → декодированный текст идёт в ту же `_classifyPlain`.
2. `{`/`[` → `jsonDecode` + `_detectFlavor` (`body_decoder.dart:181`) → `JsonConfig(value, flavor)`.
3. Первая не-comment строка `[interface]` + наличие `[Peer]` → `IniConfig` (**wgconf — да**).
4. Иначе — построчный сплит, скип пустых и комментариев (`#`, `//`, `;`, счётчик `skippedComments`) → `UriLines`.
5. Пусто → `DecodeFailure(reason, sample)`.

JSON-флейворы (enum `JsonFlavor`, `body_decoder.dart:51`):
- `xrayArray` — **Xray JSON поддержан**: массив автономных Xray-конфигов (элементы с `outbounds`, у которых `protocol`);
- `singboxOutbound` — одиночный `{"type":"vless",…}`;
- `singboxArray` — массив outbound'ов;
- `singboxConfig` — полный конфиг (`outbounds` или `endpoints`, sing-box ≥1.11);
- `singboxMulti` — массив автономных sing-box-конфигов (различение Xray↔sing-box по `protocol` vs `type` в `_looksLikeSingboxOutbounds`, `body_decoder.dart:217`);
- `clashYaml` — распознаётся (`proxies` в Map), но **не парсится** (0 узлов), пользователю даётся подсказка через `diagnoseEmptyParse` (`lib/services/parse_hints.dart:7` — HTML-страница / Clash YAML / полный конфиг с `inbounds`/`routing` / plain-text error);
- `unknown`.

Разбор `DecodedBody` → узлы: `parseAll(DecodedBody, {nameHint})` в `lib/services/parser/parse_all.dart:16` (exhaustive switch; `nameHint` = имя файла, идёт только в INI/Amnezia-ветки; для Amnezia — индексные суффиксы `hint 2` по индексу контейнера).

Отдельно от парсера подписок: `lib/config/config_parse.dart` — это НЕ парсер подписок, а канонизация JSON/JSON5/JSONC пользовательского конфига (`canonicalJsonForSingbox`, `prettyJsonForDisplay`, `*Async` через `compute`) на пакете `json5`.

## 3. Центральная модель узла

**`NodeSpec`** — sealed-иерархия в `lib/models/node_spec.dart:37`. 14 вариантов: `VlessSpec` (uuid, flow, tls, transport, packetEncoding, encryption §335 post-quantum), `VmessSpec` (uuid, alterId, security), `TrojanSpec`, `AnyTlsSpec` (idle-поля), `ShadowsocksSpec` (method/password/plugin), `Hysteria2Spec` (obfs `salamander|gecko` §358, up/down Mbps), `NaiveSpec` (extraHeaders), `TuicSpec` (congestionControl, udpRelayMode, zeroRtt), `SshSpec`, `SocksSpec`, `HttpSpec`, `WireguardSpec` (peers: `WireguardPeer` c `reserved` §025 WARP; `awg: Awg?` §097; `rawIni`), `MasqueSpec` (privateKeyDer/publicKeyDer, profile, vhttp), `AutoSelectSpec` (§322, группа-urltest: `isGroup == true`, `server`/`port` пустые).

Общие поля базы: `id`, `tag`, `label`, `server`, `port`, `rawUri`, `chained` (detour-цепочка `NodeSpec?`), mutable `warnings: List<NodeWarning>`, mutable `sourceCompact`/`sourceExtended` (§302 — исходник для UI), `patchedJson`/`ruleTrail` (§302 — REPLACE-патч import-rules; `emit` отдаёт его глубокой копией `deepCopyJson`).

Вспомогательные модели: `TlsSpec` (`lib/models/tls_spec.dart` — `toSingbox()` и `toSingboxForQuic()` §282: на QUIC срезаются `utls`/`reality`), `TransportSpec` sealed (`lib/models/transport_spec.dart`: `WsTransport` c `maxEarlyData`/`earlyDataHeaderName` §303, `GrpcTransport`, `HttpTransport`, `HttpUpgradeTransport`, `XhttpTransport`), результат эмита `SingboxEntry` sealed = `Outbound | Endpoint` (`lib/models/singbox_entry.dart`), `withChained(spec, chained)` — пересборка с detour (`node_spec.dart:957`).

**`ConfigNode`/`ParsedConfig`** (`lib/models/config_node.dart`) — другая модель: НЕ узел подписки, а структурные метаданные outbound/endpoint уже **собранного** config.json (`ParsedConfig.parse(configRaw)` → `Map<tag, ConfigNode>`, detour-цепочки, `transportLabel`/`securityLabel` с деривацией awg/awg1.5/awg2 §148). Используется UI/интроспекцией, не парсером подписок.

Идентичность:
- `nodeIdentityKey(NodeSpec)` (`lib/services/node_identity.dart:16`) — `protocol|server|port|credential` (без транспорта и TLS — решение юзера 30.07.2026), для дедупа §321 P4 и резолва пулов §322; учитывает `patchedJson`. Зеркало в сыром Xray-JSON — `_xrayIdentity` (`json_parsers.dart:479`), инвариант посимвольного совпадения.
- `nodeIdentityHash(NodeSpec)` (`lib/services/node_hash.dart:65`) — sha256 от `deepSortKeys(emit(TemplateVars.empty).map − tag − detour)`; ключ per-node disable, плюс GC отметок `gcDisabledHashes`/`applyRuleMarks`.

## 4. Пайплайн: от сырого тела до sing-box outbound JSON

```
SubscriptionSource
  → parseFromSource() (lib/services/subscription/sources.dart:91)
      _fetch → _inlineHeaders (# profile-title: … fallback) → _metaFromHeaders
  → decode(body)                 body_decoder.dart:80  → DecodedBody
  → parseAll(decoded)            parse_all.dart:16     → List<NodeSpec>
      UriLines   → parseUri(line)          uri_parsers.dart:37 (+ sourceCompact = строка)
      IniConfig  → parseWireguardIni       ini_parser.dart:12  (INI → синтетический wireguard:// URI → parseWireguardUri)
      AmneziaConfig → parseWireguardIni на каждый контейнер
      JsonConfig:
        xrayArray → два прохода §342 (priming-сортировка «одиночные раньше пулов» + боевой в порядке файла,
                    дедуп seen §321 P4, synonyms §321 P6, ownedBy)
                    → parseXrayElement (json_parsers.dart:36)
                        _xrayToSpec (json_parsers.dart:531): vless|trojan|vmess|shadowsocks|hysteria(→Hysteria2Spec)
                        dialerProxy → detour-звено (_xrayDetourToSpec), балансеры → AutoSelectSpec (§322)
        singbox* (4 формы) → нормализация к массиву конфигов → parseSingboxConfigs (singbox_config.dart:47)
                    → _parseOne: outbounds+endpoints, разрыв detour-колец (_findCycleEdges),
                      parseSingboxEntry (json_parsers.dart:797): vless|vmess|trojan|anytls|shadowsocks|hysteria2|
                      naive|tuic|ssh|socks|http|wireguard|masque; selector/urltest → AutoSelectSpec;
                      _buildChain → withChained (лимит kMaxDetourDepth = 8)
  → [в контроллере] import-rules §302 → patchedJson
  → NodeSpec.emit(TemplateVars)  node_spec.dart:106 → emitRaw → node_spec_emit.dart (emitVless/…/emitMasque)
      транспорт: TransportSpec.toSingbox(vars) (+warnings), TLS: TlsSpec.toSingbox()/toSingboxForQuic()
  → NodeSpec.getEntries(EmitContext) → NodeEntries(main + detours)
  → builder (lib/services/builder/…: build_config/server_list_build) — префиксы тегов, detour-политика,
    раскладка Outbound → config.outbounds[], Endpoint → config.endpoints[]
```

Нормализация «мусора подписок» по пути парсинга: `normalizeUtlsFingerprint` (allowlist `kUtlsFingerprints`, xray-алиасы `hellochrome_*` → `chrome`, `utls_fingerprint.dart`), `normalizeHysteria2Obfs` (`hysteria2_obfs.dart`), WS early data `?ed=N` → `max_early_data`/`early_data_header_name` (`splitEarlyDataPath`/`decodeResidualPercent` в `transport.dart`), reality pbk/sid валидация (`isValidRealityPublicKey`/`normalizeRealityShortId` в `uri_utils.dart`), гашение `flow=xtls-rprx-vision` при транспорте (§115, `emitVless`).

## 5. Обратное направление: эмиссия share-URI

Да. `NodeSpec.toUri()` — контракт round-trip §4 (`parseUri(spec.toUri()) ≈ spec`), реализации в `lib/models/node_spec_emit.dart`:

- `toUriVless`, `toUriTrojan`, `toUriAnyTls` (с reality pbk/sid), `toUriHysteria2` (включая собственные ключи `obfs-min/max-packet-size` для gecko), `toUriTuic`, `toUriSsh`, `toUriSocks` (`socks5://`), `toUriHttp` (`proxy-http`/`proxy-https` по TLS), `toUriNaive` (`naive+https://`, порт 443 опускается), `toUriShadowsocks` (SIP002 `ss://base64(method:pass)@host:port#frag`), `toUriVmess` (v2rayN `vmess://base64(JSON)`), `toUriWireguard` (`wireguard://` + AWG-query `Awg.writeQuery`, `reserved`), `toUriMasque` (`masque://`).
- `AutoSelectSpec.toUri()` → синтетический `autogroup://` (`autoGroupToUri`, `auto_select.dart:117`) — только для хранения в папке, не переносимая ссылка.

Единственный «lossy» эмиттер — VMess (legacy base64-JSON, кастомная каноническая форма).

## 6. Warnings / деградация битых нод

`lib/models/node_warning.dart` — sealed `NodeWarning` с `severity` (`info|warning|error`), локализуемым `message()`/пиненным английским `renderEn()`, равенством по `runtimeType + props` (dedup, независимый от локали §279). Копятся в mutable `NodeSpec.warnings` при парсинге И при emit'е (fallback'и).

Подклассы: `UnsupportedTransportWarning`, `UnsupportedProtocolWarning`, `MissingFieldWarning`, `DeprecatedFlowWarning`, `VisionWithTransportWarning` (§115), `InsecureTlsWarning` (info — часто намеренно), `NaiveBuildTagWarning`, `UnknownFingerprintWarning` (§281), `XhttpParamResetWarning` + enum `XhttpResetReason` (§217), `EchIgnoredWarning` (§320), `UnknownObfsWarning`/`MissingObfsPasswordWarning` (§358), и группа §368 sing-box-импорта: `DetourCycleBrokenWarning`, `DetourTargetMissingWarning`, `DetourToGroupWarning`, `DetourChainTooDeepWarning`, `SelectorAsAutoWarning`, `GroupMemberMissingWarning`.

Философия деградации (сквозная, §169/§281/§358): значение, которое ядро sing-box отвергнет fatal'ом на ВЕСЬ конфиг, отбрасывается/канонизируется с warning'ом на ноде — нода выживает; битая нода → `null`-skip — подписка выживает; битый контейнер/элемент — try/catch на гранулярности узла, потерянный протокол виден через P5-warning `unsupported`.

## 7. Тестовые корпуса

- **`test/parser/`** — 25 файлов: per-протокол (`vless_test` 36 тестов, `anytls_test`, `tuic_test`, `http_proxy_test`, `uri_naive_test`, `awg_test`, `wireguard_edge_test`), форматы (`body_decoder_test`, `ini_parser_test`, `amnezia_link_test` — энкодер qCompress прямо в тесте), Xray-ветка (`json_parsers_test`, `xray_multiprotocol_test`, `xray_dedup_test`, `xray_auto_select_test` — 47 тестов, `auto_select_rules_test`), sing-box-импорт (`singbox_config_test` — 45), edge-кейсы (`ws_early_data_test`, `ws_query_early_data_test`, `path_double_encoding_test`, `reality_short_id_test`, `ech_import_test`, `utls_fingerprint_test`, `xhttp_test`), `round_trip_test` (URI→Spec→URI→Spec) и `fixtures_parse_test` (смоук по всем `.uri`-фикстурам: parse + emit + round-trip).
- **`test/fixtures/`** — per-протокол каталоги `.uri`-файлов: `vless/` (reality_xtls, ws/grpc/http2_tls, xhttp_obfs_full, edge_fallback_xhttp), `vmess/`, `trojan/`, `shadowsocks/` (legacy base64, SIP002, ss2022_blake3), `hysteria2/`, `tuic/`, `ssh/`, `socks/`, `wireguard/` (uri + `ini_basic.conf`), `json/` (xray_array_reality.json, singbox_vless_outbound.json, singbox_wg_endpoint.json), `base64/plain_uri_list.txt`; `subscriptions/` — пока только README (задел под анонимизированные реальные тела `real_<provider>_<date>.txt` + опциональные `.headers.json`).
- **`test/subscription/`** — пайплайн: `sources_test`, `singbox_config_import_test`, `import_rules_test`, `http_cache_test`, `inline_headers_test`, `content_disposition_test`, `user_agent_test`, `file_subscription_test`, `wg_import_filename_test`, `subscription_identity_test`, `folder_test`, `auto_updater_test` и др.
- **Golden-файлов (flutter golden images / snapshot-файлов) нет** — слово «golden» встречается только как имя константы-эталона внутри `xhttp_test.dart` (golden round-trip 15 camelCase→snake_case полей). Каталог `test/parity/`, упомянутый в комментарии `node_spec_emit.dart`, не существует (v1-parity тесты удалены/не завезены). Плюс `test/pipeline_e2e_test.dart` и `test/models/node_spec_test.dart`, `node_warning_test.dart`, `naive_emit_test.dart`, `masque_spec_test.dart` — эмиссия и модель.

Замечание для кросс-платформенного дизайна: `lib/models/parser_config.dart` — это модель wizard-template (`WizardTemplate`/`ParserConfigBlock` с `version`/`reload`), к разбору подписок отношения почти не имеет; настоящая связка для переноса — `body_decoder → parse_all → uri_parsers|json_parsers|singbox_config|ini_parser → NodeSpec → node_spec_emit`, плюс инвариант пары «эмиттер↔парсер» на каждую схему (см. память `emitter-parser-pairing`).