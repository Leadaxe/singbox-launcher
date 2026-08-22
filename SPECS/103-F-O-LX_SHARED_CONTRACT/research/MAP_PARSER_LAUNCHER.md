# Карта парсера подписок singbox-launcher

## 1. Поддерживаемые URI-схемы и диспетчеризация

Два места диспетчеризации, оба в `core/config/subscription/node_parser_core.go`:

- **`IsDirectLink()`** — `node_parser_core.go:18-37` — детектор «это прямая ссылка»: `vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://`, `hy2://`, `tuic://`, `anytls://`, `ssh://`, `wireguard://`, `awg://`, `masque://`, `vpn://`, `socks5://`, `socks://`, `naive+https://`, `naive+quic://`.
- **`ParseNode(uri, skipFilters)`** — `node_parser_core.go:80` — фактический диспетчер:
  - `vpn://` → `parseAmneziaVPNLink` (`node_parser_core.go:84`, обходит лимит `MaxURILength = 8192`);
  - `masque://` → `parseMasqueURI` (`node_parser_core.go:91`, `node_parser_masque.go:28`);
  - `vmess://` → base64/legacy-декод → `parseVMessDecoded` (`node_parser_core.go:108-135`, `node_parser_vmess.go:43` — внутри: base64(JSON), legacy cleartext `method:uuid@host:port`, `parseVMessJSON`);
  - `wireguard://`, `awg://` → `parseWireGuardURI` (`node_parser_core.go:260-270`; awg нормализуется в wireguard, AWG-поля через `applyAWGFields`, `node_parser_wireguard.go:381`);
  - остальные (`vless`, `trojan`, `ss` c SIP002/legacy-base64, `hysteria2`/`hy2` c опц. base64 и multiport-восстановлением `hysteria2RecoverMultiPortAuthority`, `tuic`, `anytls`, `ssh`, `socks5`/`socks`, `naive+https`/`naive+quic`) — общий путь через `net/url.Parse` в том же switch (`node_parser_core.go:107-286`), затем `buildOutbound(node)` (`node_parser_core.go:578`).

Схема узла хранится в `ParsedNode.Scheme`; маппинг на sing-box type: `ss`→`shadowsocks`, `socks5`/`socks`→`socks` (в `buildOutbound` и `GenerateNodeJSON`).

## 2. Входные форматы целиком и классификация тела

**Классификация — `body_classify.go`**, тип `BodyKind` (`:14`), функция `ClassifySubscriptionBody(body)` (`:74`):

| BodyKind | Признак |
|---|---|
| `BodyKindURIList` (default) | не JSON; построчный список share-URI |
| `BodyKindXrayArray` | JSON-массив, элементы с `outbounds[].protocol` (`xrayElementHasProtocolOutbounds`, `xray_json_array.go:624`) |
| `BodyKindSingboxOutbound` | объект с `"type"` (проверяется РАНЬШЕ `"outbounds"` — selector несёт оба) |
| `BodyKindSingboxOutboundArray` | массив объектов с `"type"` |
| `BodyKindSingboxConfig` | объект с массивом `outbounds`/`endpoints` |
| `BodyKindSingboxConfigArray` | массив таких конфигов |

До классификации: **`DecodeSubscriptionContent`** (`decoder.go:21`) — base64-тело (через `DecodeBase64Multi`, `encoding_utils.go:30` — 4 варианта кодировки), JSON-массив пропускается как есть, plain-text с `://` как есть; одиночный JSON-объект на этом слое отвергается (но `ClassifySubscriptionBody` в `source_loader` работает уже по декодированному телу).

Прочие входные форматы (не через тело подписки, а через paste/ссылку):
- **wgconf text** ([Interface]/[Peer]) — `wgconf_text.go`: `ExtractWGConfBlocks` (`:23`) + `ConvertWGConfText` (`:52`) → канонический `wireguard://` URI; вызывается из UI-классификатора `classifyInputLines` (`ui/configurator/business/parser.go:199`);
- **Amnezia `vpn://`** — `node_parser_amnezia.go`: base64url + qCompress(zlib) профиль → контейнер WG/AWG → INI → `wgConfToURI` (`:269`) → `parseWireGuardURI`;
- **manual config_json** — `manual_config.go:34` `NodeFromManualConfigJSON`: готовый sing-box объект в `ProxySource.ConfigJSON`, узел с `EmitRaw=true`.

## 3. Центральная модель узла

**`configtypes.ParsedNode`** — `core/config/configtypes/types.go:268` (алиас `config.ParsedNode` в `core/config/models.go:12`):

```go
type ParsedNode struct {
    Tag      string                 // итоговый тег (после prefix/mask/uniquify)
    Scheme   string                 // "vless"|"vmess"|...|"wireguard"|SchemeGroup("group")
    Server   string
    Port     int
    UUID     string                 // «главный секрет» (для trojan/ss/hy2 — пароль)
    Flow     string
    Label    string                 // человекочитаемое имя (fragment)
    Comment  string
    Query    url.Values             // сырые query-параметры URI
    Outbound map[string]interface{} // готовая sing-box outbound/endpoint map
    Jump     *ParsedJump            // deprecated, = Chain[0] (SPEC 094 B1)
    SourceTag string                // исходный тег из sing-box импорта (для групп)
    Chain    []*ParsedNode          // detour-цепочка, ближний хоп первым, cap 8
    SourceIndex int                 // индекс в ParserConfig.proxies (UnsetSourceIndex=-1)
    EmitRaw  bool                   // manual config_json → сериализация map как есть
}
```

Рядом: `ParsedJump` (`:255`), `SchemeGroup = "group"` + `GroupMembersKey = "outbounds"` (`:247-251`), `MaxNodesPerSubscription = 3000` (`:74`), `ProxySource` (`:91` — skip, tag_prefix/postfix/mask, detour_tag/detour_node_hash, disabled_nodes, config_json).

## 4. Пайплайн: от сырого тела до outbound JSON

1. **Fetch** — `FetchSubscriptionWithMeta` (`fetcher.go:226`): HTTP GET (таймаут 30s, лимит 10MB `MaxSubscriptionResponseSize`), заголовки `applySubscriptionRequestHeaders` (`:84`, UA `LxBox/...` + X-Hwid-семейство), → `ParseHeaders`/`ParseInlineComments`/`MergeMeta` (`meta.go:118/163/249`), → `DecodeSubscriptionContent`. Кеш-хук `LookupCachedBody` (`source_loader.go:27`).
2. **Decode** — `DecodeSubscriptionContent` (`decoder.go:21`) — base64 → plain URIs / JSON passthrough.
3. **Classify** — `ClassifySubscriptionBody` (`body_classify.go:74`) внутри `LoadNodesFromSourceEx` (`source_loader.go:347`).
4. **Parse** — три ветки в `LoadNodesFromSourceEx` (`source_loader.go:280`):
   - sing-box JSON → `ParseSingboxBody` (`singbox_import.go:45`) → `normalizeSingboxBodyToConfigs` → `ParseNodesFromSingboxConfigs` (`:106`) → `parseSingboxEntry` (`:248`: копия map + `SanitizeSingboxOutboundMap`, `singbox_sanitize.go:57`) + группы `singboxGroupToNode` (`singbox_groups.go:48`) + цепочки `analyzeSingboxDetour`/`attachChain` (`detour_chain.go:51/141`);
   - Xray-массив → `ParseNodesFromXrayJSONArray` (`xray_json_array.go:30`): два прохода владения идентичностью (`computeXrayIdentityOwners`), `parseXrayJSONArrayElementNodes` → `xrayNodeFromOutbound` (`xray_protocols.go:37`; протоколы: vless, vmess, trojan, shadowsocks, hysteria2; служебные freedom/blackhole/dns/loopback пропускаются), dialerProxy/sockopt-цепочки `attachXrayDialerChain` (`:533`), балансировщик → группа `xrayBalancerFromElement` (`xray_balancer.go:39`);
   - URI-list → построчно `NormalizeSubscriptionTextLine` + `ParseNode`.
5. **Dedup / tags / фильтры** — `dedupNodesByIdentity` (`source_loader.go:154`) через хук `NodeIdentityHashFunc` (= `config.NodeIdentityHash`, `node_hash.go:71`, ставится в `init()` `node_hash.go:36`); `applyTagPrefixPostfix` (`:817`) → `MakeTagUnique` (`:207`); `rebindImportedGroupNodes` (`:701`); skip-фильтры — `shouldSkipNode` (`node_parser_core.go:561`).
6. **Detour / disabled** — `ApplySourceDetour` (`source_loader.go:759`), `filterDisabledNodes` (`:74`, по identity-хешу, TTL-GC `GCDisabledNodes`).
7. **Emit** — `GenerateOutboundsFromParserConfig` (`core/config/outbound_generator.go:753`): узлы через `EmitNodeJSONs` (`:704`) → `GenerateNodeJSON` (`:85`) / `GenerateEndpointJSON` (`:672`, wireguard → массив `endpoints`), затем трёхпроходная валидация селекторов (`outbound_validity.go`) и вставка в config.json.

Промежуточная outbound-map строится ещё на этапе парса: `buildOutbound` (`node_parser_core.go:578`) + транспорт/TLS-хелперы `uriTransportFromQuery`, `vlessTLSFromNode`, `trojanTLSFromNode`, `xhttpTransportFromQuery`, `applyWSEarlyData` (`node_parser_transport.go:114/625/714/267/528`).

## 5. Обратное направление

**Share-URI** — `share_uri.go:17` `ShareURIFromOutbound(out map[string]interface{})`, per-type диспетчер по файлам `shareuri_*.go`: **vless, vmess, trojan, shadowsocks (ss), socks, hysteria2, tuic, anytls, ssh, naive, wireguard (`ShareURIFromWireGuardEndpoint`, `shareuri_wireguard.go`), masque**. Не кодируются: selector/urltest/direct/block/dns/http и multi-peer WG → `ErrShareURINotSupported` (`share_uri.go:13`). Хелперы — `shareuri_helpers.go`.

**Эмиссия outbound JSON** — `core/config/outbound_generator.go`:
- `GenerateNodeJSON` (`:85`) — ручная сборка JSON per-scheme: vless, vmess, trojan, shadowsocks, hysteria2, tuic, naive, masque, anytls, ssh, socks; спец-ветки `generateGroupNodeJSON` (`:949`, SchemeGroup) и `generateRawNodeJSON` (`:1010`, EmitRaw);
- `GenerateEndpointJSON` (`:672`) — wireguard endpoint через `json.MarshalIndent` map как есть;
- `EmitNodeJSONs` (`:704`) — единая точка (chain-хопы + detour), общая для генерации и JSON-вкладки Source-редактора.

⚠️ Известная ловушка (из MEMORY): новая схема без ветки в per-scheme switch `GenerateNodeJSON` молча урезается до `{tag,type,server,server_port}` — каждой схеме нужен эмиссионный тест.

## 6. Warnings / деградация битых нод

Структурированного накопителя warnings **нет** — политика «деградируй ноду, не конфиг», реализуется через `debuglog.WarnLog/ErrorLog` + возврат ошибки из per-node парсера (нода дропается, цикл в `LoadNodesFromSourceEx` продолжает):
- allowlist-валидация значений: `isValidShadowsocksMethod` (`node_parser_ss.go:6`), `NormalizeUTLSFingerprint` (`node_parser_transport.go:84`), `isValidRealityPublicKey`/`normalizeRealityShortID` (`:596/:567`), `packet_encoding` только xudp/packetaddr (`node_parser_core.go:609-624`), `isValidTuicCongestionControl`, `isValidHysteria2ObfsType`; невалидный порт/UTF-8 — ошибка ноды;
- sing-box импорт: `singbox_sanitize.go` (TLS/uTLS/reality/flow/packet_encoding/hy2-obfs/legacy-masque чинятся или вычищаются с WarnLog);
- поля без эквивалента дропаются с warning (naive `padding`, `node_parser_core.go:379`);
- generation-time: `NaiveSupportProbe` → `SkippedNaiveNodes/SkippedNaiveReason` (`outbound_generator.go:66-79`), `sanitizeNodeDetours` (`:1241`, dangling/cycle/self), `resolveNodeHashDetours` (`:1162`, fail-closed — нерезолвленный hash-detour дропает ноды источника);
- provider-level: типизированные ошибки `FetchHTTPError` / `FetchAnnounceError` (`fetcher.go:142/183`) для actionable-диалогов UI.

## 7. Тестовые корпуса

`testdata/` содержит две embed-фикстуры (единственные «golden»-файлы, остальное — inline table-tests):
- `testdata/singbox_full_config.json` → `singbox_import_fixture_test.go` (полный конфиг: группы, цепочка, endpoints, ignored-секции, REALITY);
- `testdata/xray_provider_anon.json` → `xray_json_array_test.go` (реальная анонимизированная Xray-подписка, дедуп/владение идентичностью).

Ключевые `*_test.go`: `node_parser_test.go` (2057 строк, основной корпус URI), `xray_protocols_test.go`, `xray_ownership_test.go`, `ws_early_data_test.go` (issue #96), `xhttp_v2_test.go`, `awg_test.go` + `awg_range_test.go` + `warp_reserved_test.go` + `wireguard_robustness_test.go`, `node_parser_amnezia_test.go`, `node_parser_masque_test.go`, `node_parser_naive_test.go`, `node_parser_tuic_test.go`, `node_parser_anytls_test.go`, `node_parser_transport_test.go`, `singbox_import_test.go` + `singbox_import_e2e_test.go` + `singbox_sanitize_test.go`, `dedup_test.go` + `disabled_nodes_test.go` + `e2e_disabled_flow_test.go`, `detour_chain_test.go`, `share_uri_encode_test.go` (roundtrip URI↔outbound), `body_classify_test.go`, `decoder_test.go`, `meta_test.go` + `fetcher_meta_test.go`, `hysteria2_ports_test.go`, `userinfo_space_test.go`, `wgconf_text_test.go`, `manual_config_test.go`, `live_subscriptions_test.go` (сетевые, opt-in).

Абсолютные пути: всё в `/Users/macbook/projects/singbox-launcher/core/config/subscription/`, эмиссия — `/Users/macbook/projects/singbox-launcher/core/config/outbound_generator.go`, модель — `/Users/macbook/projects/singbox-launcher/core/config/configtypes/types.go`, хеш идентичности — `/Users/macbook/projects/singbox-launcher/core/config/node_hash.go`.