# Protocols and link formats of singbox-launcher

**🌐 Language**: English | [Русский](Protocols.ru.md)

A reference for the **URI formats**: which protocols the launcher understands,
which parameters it reads from each, and what they become in `config.json`.

Configuring the parser itself — sources, filters, directions (`outbounds`),
marker sections, the wizard — lives in a separate document,
[**`ParserConfig.md`**](ParserConfig.md).

## Contents

- [Supported protocols](#supported-protocols) — a table of 14 schemes
- [The xhttp transport and AmneziaWG](#the-xhttp-transport-and-amneziawg)
- [A JSON array of full Xray/V2Ray configs](#a-json-array-of-full-xrayv2ray-configs)
- [Degradation codes on a node](#degradation-codes-on-a-node)
- [Share URIs from an outbound](#share-uris-from-an-outbound-or-a-wireguard-endpoint-back-to-a-link) — the reverse direction
- [URI formats per protocol](#uri-formats-for-direct-links) — the parameters of each scheme

### Supported protocols

| # | URI scheme | sing-box `type` | Config section | Version / build tag | Description |
|---|-----------|-----------------|----------------|--------------------|----------|
| 1 | `vless://` | `vless` | `outbounds[]` | core (+ **`with_xhttp`** for xhttp) | TCP/raw/ws/grpc/http/`httpupgrade`/quic/**`xhttp`** (splithttp), TLS, Reality, Vision flow. xhttp is native on the sing-box-lx core (see below). |
| 2 | `vmess://` | `vmess` | `outbounds[]` | core (+ **`with_xhttp`**) | Base64 JSON or legacy cleartext `method:uuid@host:port`. `net=h2`→`http`+TLS; `net=xhttp`→**`xhttp`**, `net=httpupgrade`→`httpupgrade` (distinct transports). |
| 3 | `trojan://` | `trojan` | `outbounds[]` | core | Same transport/TLS as VLESS. Password in the userinfo. |
| 4 | `ss://` | `shadowsocks` | `outbounds[]` | core | SIP002 + legacy `ss://base64("method:password@host:port")`. Methods are a fixed allow-list (2022-blake3, AEAD GCM, ChaCha20-Poly1305). |
| 5 | `hysteria2://`, `hy2://` | `hysteria2` | `outbounds[]` | core (QUIC) | Multi-port (`mport`/`ports` query, or `host:123,5000-6000` in the authority); obfs is `salamander` or `gecko` (the fork's core supports both). |
| 6 | `ssh://` | `ssh` | `outbounds[]` | core | **A singbox-launcher URI dialect**, not an RFC. Inline key / key path / passphrase / host_key. |
| 7 | `socks5://`, `socks://` | `socks` (version=5) | `outbounds[]` | core | User/pass optional. The `scheme` filter field keeps the original (`socks5` vs `socks`). |
| 8 | `naive+https://`, `naive+quic://` | `naive` | `outbounds[]` | **sing-box ≥ 1.13.0** + build tag **`with_naive_outbound`** (fork core `1.14.0-lx.4+` — all desktop platforms; on Windows `libcronet.dll` is required and the launcher ships it). On a core without support such nodes are **degraded with a warning** instead of breaking the config. | DuckSoft 2020 URI dialect. `extra-headers=` (CRLF-separated pairs). TLS carries `server_name` only. |
| 9 | `wireguard://` | `wireguard` | **`endpoints[]`** | **sing-box ≥ 1.11** (+ **`with_awg`** for AmneziaWG) | One peer; `@ParserSTART_E`/`@ParserEND_E` markers. Default port 51820, mtu 1420. Optional **AmneziaWG 2.0** parameters (jc/jmin/jmax, s1–s4, h1–h4, i1–i5) — see below. |
| 10 | `tuic://` | `tuic` | `outbounds[]` | core (QUIC) | TUIC v5: `uuid:password` in the userinfo. Query: `congestion_control` (cubic/new_reno/bbr), `udp_relay_mode` (native/quic), `alpn`, `sni`, `allow_insecure`, `reduce_rtt`/`zero_rtt_handshake`, `heartbeat`, `fp`. TLS is mandatory (QUIC). |
| 11 | `vpn://` | `wireguard` | **`endpoints[]`** | same as #9 | An **Amnezia** profile (a `.vpn` file: base64url + qCompress + JSON, SPEC 075): the WG/AWG container is imported and converted into a canonical `wireguard://` URI. See the Amnezia (`vpn://`) section below. |
| 12 | `masque://` | `masque` | `outbounds[]` | **fork core `1.14.0-lx.26+`** (the `vhttp`+`tls` schema, core SPEC 062) | **A singbox-launcher URI dialect.** MASQUE / CONNECT-IP (RFC 9484) — whole IP packets over HTTP/3 (`h3`) or HTTP/2 (`h2`), primarily Cloudflare WARP. base64(DER) keys, tunnel addresses in `address=`. Nodes are usually produced by the WARP wizard. See the MASQUE (`masque://`) section below. |
| 13 | `anytls://` | `anytls` | `outbounds[]` | core (core rc.17+: `option/anytls.go`) | Password in the userinfo, mandatory TLS block; session-pool tuning (`idle_session_check_interval`, `idle_session_timeout`, `min_idle_session`). SPEC 091. |
| 14 | `proxy-http://`, `proxy-https://` (aliases `proxy+http://`, `proxy+https://`) | `http` | `outbounds[]` | core | **HTTP(S) CONNECT proxy** (SPEC 103 §9.B6, LxBox convention). Credentials in the userinfo; `proxy-https` adds TLS (default port 443, plain 80). A custom scheme rather than a bare `http(s)://` because those are a **subscription source**, intercepted upstream before they could ever be read as a node. |

Besides URIs, the Add field accepts **raw `[Interface]/[Peer]` text** (a WireGuard/AmneziaWG `.conf`, AWG fields included) — conf blocks are detected before line-by-line parsing and converted into `wireguard://` URIs (SPEC 076); see the `.conf` text section below.

**Not supported** (explicitly, not implemented): **ShadowTLS**, **Mieru**, **Hysteria 1** (v2 only), **ShadowsocksR / SSR**, **Tor**, a bare `http(s)://...` URL as a node (it is always a **subscription source**; the node form is `proxy-http://` / `proxy-https://`, row 14 above). Selectors (`selector`, `urltest`, `direct`, `block`, `dns`) are not URI protocols; they are assembled on the ParserConfig side (see the [`outbounds` section](ParserConfig.md#the-outbounds-section)).

### The xhttp transport and AmneziaWG

The launcher is built against the **[sing-box-lx](https://github.com/Leadaxe/sing-box-lx)** core (upstream sing-box plus exactly two client-side features behind build tags). The launcher's parser/generator/share-URI support both end to end; at runtime they work **only** on a core with the matching tag — on stock sing-box a config carrying these fields is rejected at load time (an explicit error, no silent downgrade).

**✅ The `xhttp` transport — fully supported (build tag `with_xhttp`).** The former degradation into `httpupgrade` is gone. With `type=xhttp` (VLESS/Trojan) or `net=xhttp` (VMess) a real `type:"xhttp"` transport (Xray-compatible splithttp) is built with all its fields, and serialized back into a share URI without loss:

- Fields: `mode` (`auto` \| `packet-up` \| `stream-up` \| `stream-one`; on the fork `auto`=`packet-up`, and `stream-one` has a known downlink-framing bug), `host`, `path`, `headers`, `x_padding_bytes` (a `"min-max"` range, default `100-1000`, carried in the `Referer` header), `no_grpc_header`. Composes with TLS/Reality (not with XTLS-Vision — a protocol limitation).
- `httpupgrade` is now a **separate** transport (HTTP/1.1 Upgrade) — no longer confused with xhttp on either input or share-URI output.
- Details: `SPECS/071-F-N-XHTTP_TRANSPORT/SPEC.md`, `sing-box-lx/docs-lx/lx-config.md`.

**✅ AmneziaWG 2.0 (AWG2) — WireGuard obfuscation (build tag `with_awg`).** A WireGuard endpoint (`wireguard://`) may carry promoted AWG fields: the numbers `jc`/`jmin`/`jmax`, `s1`–`s4`, `h1`–`h4` and the CPS strings `i1`–`i5` (AWG 2.0, case-sensitive tag format). `h1`–`h4` may be a single number **or a range** `lo-hi` (AWG 2.0 header randomization; a core ≥ `1.13.13-lx.6` picks a value per handshake itself — subtask 073.2). Import sources: `wireguard://`/`awg://` URIs, Amnezia `vpn://` profiles (SPEC 075) and pasted `.conf` text (SPEC 076); emitted into `endpoints[]`, round-tripped through share URIs without loss. An endpoint with **no** AWG field at all is plain WireGuard (byte-identical to upstream). Field details are in the [WireGuard](#wireguard-wireguard) section below; `SPECS/073-F-N-AMNEZIAWG_PARAMS/SPEC.md`, `sing-box-lx/docs-lx/lx-config.md`.

Per-scheme details (query parameters, TLS, transport, edge cases) are in [URI formats for direct links](#uri-formats-for-direct-links) below.

### A JSON array of full Xray/V2Ray configs

If the subscription body (plain, or after Base64 decoding) is a **valid JSON array** `[...]` whose elements look like Xray (`outbounds[].protocol`, VLESS with `settings.vnext`), the launcher treats it as a subscription: **one** logical node is extracted from **each element**. Parsing uses the **`outbounds`** field and, when present, **`remarks`**; the element's root-level **`dns`**, **`routing`**, **`inbounds`** and everything else is **not** merged into the launcher's own config.

**How an Xray array is told apart from a sing-box array (016, not implemented)**

| Step | Heuristic |
|-----|-----------|
| Decoder | After trimming, the string starts with **`[`**, **`json.Valid`** passes, and `json.Unmarshal` into an array succeeds — the body is not rejected as "not a subscription" (`DecodeSubscriptionContent`). |
| Parser entry | **`IsXrayJSONArrayBody`**: the same — a `[` prefix, valid JSON, an array of objects. |
| Array element | **`xrayElementHasProtocolOutbounds`**: **`outbounds`** contains at least one object with a **`protocol`** field (a string) — the marker of the **Xray dialect**. Elements carrying only the sing-box **`type`** without **`protocol`** are not considered Xray for this branch and are **skipped** with a `debuglog` line (follow-up **016** is expected). |
| Node | Among VLESS entries with **`settings.vnext`** the main outbound is picked (`xrayBuildVLESSFromOutbound`); with **`dialerProxy`** the hop is parsed as **`socks`** or **`vless`** (`xrayChainHopFromOutbound`; the socks hop via `xrayBuildJumpFromSocksOutbound`); any other hop `protocol` skips the element (`WarnLog`). |

**`remarks` and sing-box tags**

- **`ParsedNode.Label`** receives the full **`remarks`** text (when empty, the fallback is the main Xray outbound's tag or `xray-{index}`).
- **Tags** of the generated sing-box outbounds: when **`remarks`** is non-empty a **slug** is built from it (letters/digits in any script, **regional indicator symbols** for UTF flags, normalization through `textnorm`, length trimming; other punctuation and emoji besides flags do not enter the slug). The **main** outbound gets the tag **`{slug}`**; when chaining through SOCKS the second outbound (the jump) gets **`{slug}_jump_server`**, and the main one carries a **`detour`** to that tag in its JSON. When **`remarks`** is empty — **`xray-{index}`** and **`xray-{index}_jump_server`**. After that, as with ordinary subscriptions, **`tag_prefix` / `tag_postfix` / `tag_mask`**, **`textnorm.NormalizeProxyDisplay`** and **`MakeTagUnique`** are applied (to the jump as well).
- In the generated `config.json` fragment a **comment** `// …` built from **`Label`** (the full `remarks`) is still written above the outbound, since sing-box has no "remarks" field on an outbound.

**The `dialerProxy` chain**

With **`streamSettings.sockopt.dialerProxy`** (or **`dialer`**) pointing at an outbound with the same **`tag`**, hops with **`protocol: socks`** and **`protocol: vless`** are supported; in `config.json` the hop's outbound is generated first, then the main one (VLESS etc.) with a **`detour`** to the hop's tag. If no outbound matches the tag, or the hop's **`protocol`** is neither socks nor vless, the array element yields **no** node (`WarnLog`). Details and the extension to other types: **`SPECS/036-F-C-XRAY_JUMP_ANY_PROTOCOL/SPEC.md`**. An array of configs **in the sing-box format only** (`type` in outbounds without the Xray `protocol`) is **not** parsed in the MVP (follow-up **016**).

**Example and code**

A structure like the public Xray subscriptions (**`dns`**, **`inbounds`**, **`log`**, **`mux`**, **`tcpSettings`**, **`routing`**, **`freedom`/`blackhole`**), with made-up data: **`docs/examples/xray_subscription_array_sample.json`**. The same scenario in tests: **`core/config/subscription/testdata/xray_provider_anon.json`** (`go:embed` in **`xray_json_array_test.go`**). Implementation: **`xray_json_array.go`**, **`xray_outbound_convert.go`**, **`decoder.go`** (`DecodeSubscriptionContent`), **`source_loader.go`** (`LoadNodesFromSource`, **`applyTagsToXrayNode`**), configurator: **`ui/configurator/tabs/source_tab.go`** (`refreshOneSourceFromUI`).

### Degradation codes on a node

A link from a public subscription is routinely wrong in ways that are *not* the
user's fault: a junk `fp=`, an out-of-allowlist `packet_encoding`, an obfuscation
type the core does not know. The parser's rule is **degrade the node, not the
config** — one broken value must never make `sing-box check` reject the whole file
and leave the user without a VPN.

But a degradation that only reaches `debuglog` is invisible: the UI and LxBox show
the node as if nothing happened. So a surviving node carries the machine-readable
codes of everything that was silently adjusted:

- `configtypes.ParsedNode.Warnings []string`, appended via `AddWarning` (deduped).
- The normative dictionary is **`contract/registry/warnings.json`** — shared with
  LxBox, so both apps report the same event by the same name.
- `core/config/subscription/parse_warnings.go` holds the Go constants.

Codes split into two kinds, and the split is deliberate:

| Kind | Severity | Where it lives |
|---|---|---|
| The node **survives**, a value was adjusted | `info` / `warning` | on the node, in `Warnings[]` |
| The node is **dropped** at parse time | `error` | in the drop reason — there is no `ParsedNode` to mark |

Examples of the first kind: `masque_vhttp_invalid` (a `vhttp` outside `{h3,h2}` is
forced to `h3`), `naive_padding_ignored`, `packet_encoding_unknown`,
`ws_early_data_converted` (an Xray `?ed=N` tail split into `max_early_data` +
`early_data_header_name` — the path in the config is deliberately not the one in
the link), `amnezia_container_choice` (a `vpn://` profile held several containers
and the single-node path took one).

Examples of the second: `ss_method_invalid`, `port_invalid`,
`awg_headers_overlap` — the core rejects such an endpoint at load, i.e. the *whole*
config fails, so the node is skipped during parsing instead.

A guard test (`registry_sync_test.go`) keeps the two sides honest: every Go code
must exist in the registry, and a code that is never attached to a node must be
declared `severity: error`.

## Documents and URI-parser source code

| Document / location | Contents |
|------------------|------------|
| **This file** (`docs/ParserConfig.md`) | Direct-link formats in `connections`, share URIs, the ParserConfig structure, the update pipeline. |
| **`contract/registry/protocols/<scheme>.json`** | **The normative field reference** shared with the LxBox mobile app (SPEC 103): per-scheme query parameters, aliases, allowlists, degradation rules, and which side implements what. When this file and the registry disagree, the registry wins. |
| **`contract/docs/CANON.md`, `IDENTITY.md`** | How a parsed node is canonicalized (no defaults, no `tag`/`detour`, sorted keys) and how its identity hash is computed — both shared with LxBox. |
| **`contract/corpus/uri/`** | Conformance fixtures both projects run (`core/config/contract_test.go` here, `test/contract/` there). A parser change that alters behaviour shows up as a corpus diff. |
| **`SPECS/023-F-C-SUBSCRIPTION_TRANSPORT_VLESS_TROJAN/SUBSCRIPTION_PARAMS_REPORT.md`** | Tables: VLESS/Trojan query → sing-box fields; examples from public subscriptions; query keys. |
| **`SPECS/029-Q-С-SUBSCRIPTION_PARSER_CLASH_CONVERTOR_PARITY/SPEC.md`** | Compatibility extensions (029): `type=httpupgrade`, `peer`, `obfsParam`, VMess legacy / `httpupgrade` / `h2`, Hysteria2 TLS; cross-checked against the sing-box schema. |
| **`SPECS/033-F-N-SUBSCRIPTION_XRAY_JSON_ARRAY/SPEC.md`** | A subscription as a JSON array of full Xray configs: `remarks`, slug tags, `dialerProxy` → `detour`, MVP boundaries (a sing-box array is **016**, follow-up). |
| **`SPECS/036-F-C-XRAY_JUMP_ANY_PROTOCOL/SPEC.md`** | `dialerProxy`: a **SOCKS** or **VLESS** hop; other protocols as mappings land (**complete** within the SPEC's scope). |
| The **`core/config/subscription`** package | `ParseNode`, `buildOutbound` — `node_parser_core.go`; VLESS/Trojan transport+TLS — `node_parser_transport.go`; VMess — `node_parser_vmess.go` (`parseVMessDecoded`, `parseVMessJSON`, `parseVMessLegacyCleartext`); Hysteria2 — `node_parser_hysteria2.go`; WireGuard / SSH — `node_parser_wireguard.go`, `node_parser_ssh.go`; share URIs — the `share_uri.go` dispatcher plus the `shareuri_*.go` implementations; the Xray JSON array — `xray_json_array.go`, `xray_outbound_convert.go`, `xray_protocols.go`, `xray_balancer.go`. |

## Share URIs from an outbound or a WireGuard endpoint (back to a link)

The feature spec (right-click on the Servers tab, the context menu, implementation details): **`SPECS/025-F-C-SERVERS_CONTEXT_MENU_SHARE_URI/`** (SPEC, PLAN, IMPLEMENTATION_REPORT).

The parser turns a **subscription string** (`ParseNode` → `buildOutbound`, or for WireGuard an object in `endpoints[]`) into sing-box JSON. The reverse operation is **building a share URI out of an outbound or WireGuard endpoint already written** into `config.json`, so a link can be shared without keeping the original subscription string.

### Principle and format mapping

- **Encoder input:** one element of the `outbounds` array **or** one element of `endpoints[]` with `type: wireguard` (the same field set `parseWireGuardURI` / `GenerateEndpointJSON` produces).
- **Output:** a single URI string in the formats this project can read back: `vless://`, `vmess://` (base64 JSON), `trojan://`, `ss://` (SIP002), `socks5://`, `hysteria2://`, `tuic://`, `ssh://`, **`wireguard://`**.
- **Query / transport / TLS:** for VLESS and Trojan, encoding follows the same conventions as parsing (`uriTransportFromQuery`, `vlessTLSFromNode`, `trojanTLSFromNode` in `node_parser_transport.go`). VMess does not use a standard URI query in its main format (base64 JSON); the legacy form and the JSON fields are in `node_parser_vmess.go`. The detailed VLESS/Trojan reference: **`SUBSCRIPTION_PARAMS_REPORT.md`** (023); the 029 extensions: the **`029-Q-С-…/SPEC.md`** spec and the URI sections below.

### API in the code

| Function | Package | Purpose |
|--------|--------|------------|
| `ShareURIFromOutbound(out map[string]interface{})` | `core/config/subscription` (`share_uri.go`) | Encoding from an outbound JSON object; for `type: wireguard` it delegates to `ShareURIFromWireGuardEndpoint` |
| `ShareURIFromWireGuardEndpoint(ep map[string]interface{})` | `core/config/subscription` (`shareuri_wireguard.go`) | Encoding a `wireguard://` from a single endpoint (one peer in `peers[]`) |
| `GetOutboundMapByTag(configPath, tag)` | `core/config` (`outbound_share.go`) | Looking an outbound up by its `tag` field in `config.json` |
| `GetEndpointMapByTag(configPath, tag)` | `core/config` (`outbound_share.go`) | Looking an endpoint up by its `tag` field in `endpoints[]` |
| `ShareProxyURIForOutboundTag(configPath, tag)` | `core/config` (`outbound_share.go`) | An outbound by tag first, otherwise WireGuard in `endpoints[]` |

The **`ErrShareURINotSupported`** error (`subscription`) means the outbound type cannot be encoded into a single URI, or fields are missing.

### Supported `outbound.type` values

| `type` in JSON | URI scheme | Notes |
|---------------|-----------|-----------|
| `vless` | `vless://` | `encryption=none`, transport/TLS as in subscriptions |
| `vmess` | `vmess://` + base64 | The node's JSON fields match `parseVMessJSON` |
| `trojan` | `trojan://` | Password in the userinfo |
| `shadowsocks` | `ss://` | SIP002, base64(`method:password`) |
| `socks` | `socks5://` | `version` 5; user/password when present |
| `hysteria2` | `hysteria2://` | TLS SNI, `mport`, obfs and so on where possible |
| `tuic` | `tuic://` | `uuid:password`; `congestion_control`, `udp_relay_mode`, `zero_rtt_handshake`, `heartbeat`; `alpn`/`sni`/`insecure` out of TLS |
| `ssh` | `ssh://` | Inline `private_key` is **not** encoded into the URI; the key path and the other fields go into the query, as in the SSH URI docs |
| `naive` | `naive+https://` / `naive+quic://` | HTTP/2 (`naive+https`) or QUIC (`naive+quic`); user/pass in the userinfo; `extra-headers` in the query with `\r\n`-separated pairs (see the **NaïveProxy** section below). Requires sing-box **≥ 1.13.0** with the `with_naive_outbound` build tag (fork core `1.14.0-lx.4+`). |
| `anytls` | `anytls://` | Password in the userinfo; the TLS block is mandatory; session-pool fields when set |
| `masque` | `masque://` | base64(DER) keys in the userinfo/`publickey=`, `ip`/`ipv6` joined into `address=`; `vhttp` and `tls.server_name` → `vhttp=`/`sni=`. Requires core `1.14.0-lx.26+`. |
| `wireguard` | `wireguard://` | The node normally lives in `endpoints[]` only; the format and query are in the **WireGuard** section below. **One URI ↔ one remote peer:** with several entries in `peers[]` encoding is not supported (`ErrShareURINotSupported`). |

**Not encodable into a single share URI:** `selector`, `urltest`, `direct`, `block`, `dns`, `http`, arbitrary utility types; WireGuard with **several** `peers`; an outbound with a non-empty **`detour`** (a chain via a jump from an Xray JSON subscription).

### GUI

The **Servers** tab (the Clash API proxy list): **right-click** a row → `serversProxyContextMenu`: the first line is **`api.ProxyInfo.ContextMenuTypeLine`** (the API's **`type`** field lower-cased, or `servers.menu_context_type_unknown`); then **"Copy link"** (`servers.menu_copy_link`). The top line is not `Disabled` and has `Action: nil` (so its text colour matches a normal menu item). What lands in the clipboard comes from `config.ShareProxyURIForOutboundTag` over the `FileService.ConfigPath` path: an outbound by tag first, otherwise WireGuard in `endpoints[]`. A right-click on the Ping/Switch buttons may not open the menu (Fyne hit-test hierarchy). Status messages: `servers.copy_link_resolving`, `servers.copy_link_done`, `servers.copy_link_not_supported`.

### Tests

Round-trip and selected scenarios: `core/config/subscription/share_uri_encode_test.go`; integration with the config file: `core/config/outbound_share_test.go`.

## URI formats for direct links

The parser accepts direct links in the `connections` array. The format depends on the protocol:

### VLESS (`vless://`)
The standard URI format: `vless://uuid@server:port?params#tag`

**Query → sing-box outbound field mapping** (TLS, [V2Ray transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/), Reality, `security=none`, key normalization): the detailed reference and tables live in `SPECS/023-F-C-SUBSCRIPTION_TRANSPORT_VLESS_TROJAN/SUBSCRIPTION_PARAMS_REPORT.md` (the "Reference" section and § 1a).

**Query-string parameters (the usual ones):**
- `encryption` — often `none` in Xray links; it is not duplicated as a separate field in the VLESS outbound JSON
- `flow` — the VLESS sub-protocol in sing-box (for example `xtls-rprx-vision`), see the [VLESS docs](https://sing-box.sagernet.org/configuration/outbound/vless/). When the link has **no `flow`**, **nothing is substituted** into the outbound (if you want Vision, put `flow=xtls-rprx-vision` in the subscription).
- `security` — `none` | `tls` | `reality`; with `none` no TLS is added to the outbound
- `sni` — the name for SNI / certificate verification → `tls.server_name`; when `sni` is empty, **`peer`** is used (some subscriptions carry the same meaning there)
- `fp`, **`fingerprint`** — the uTLS fingerprint → `tls.utls.fingerprint`. The valid strings are those in the [sing-box docs (TLS, utls, fingerprint)](https://sing-box.sagernet.org/configuration/shared/tls/#outbound): the list there is **lower-case** (`chrome`, `firefox`, `qq`, `random`, `randomized`, …). Values from links, and the field when **generating** `config.json`, are lower-cased, otherwise sing-box may return an `unknown uTLS fingerprint` error for variants like `QQ`.
- `alpn` — a comma-separated list → `tls.alpn`
- `insecure`, `allowInsecure` / `allowinsecure` — with `1` / `true` → `tls.insecure`
- `pbk`, `sid` — Reality → `tls.reality.public_key`, `short_id`
- `type` — the transport: `tcp` / `raw`, `ws`, `grpc`, `http`, **`httpupgrade`**, **`xhttp`**, more rarely `quic`. `xhttp` is built as a native splithttp transport (the sing-box-lx core, build tag `with_xhttp`; see [the xhttp and AmneziaWG section](#the-xhttp-transport-and-amneziawg) at the top), separate from `httpupgrade`
- `path` — the WebSocket / HTTP / httpupgrade path, or the fallback service name for gRPC
- `host` / `Host` — for WS the `Host` header; when neither `host` nor `sni` is in the query, WS falls back to **`obfsParam`**. When `host` or `sni` is present, they win. For HTTP/httpupgrade it is the transport's `host` field (the case of the `Host` key in the query is respected)
- `headerType` — together with `type=raw` or `tcp` and the value `http` it selects an HTTP-type transport (obfuscation), see report 023
- `serviceName` / `service_name` — the gRPC service name → `transport.service_name`
- **The `fp` default:** when neither `fp` nor `fingerprint` is set, VLESS falls back to `random`. Trojan has no such default — there a uTLS block appears only for a recognized `fp` (and the `fingerprint` key is not read).
- `packetEncoding` — the outbound's `packet_encoding` field. **Allow-list:** only `xudp`, `packetaddr`, `none` (an empty value included). Anything else is **dropped with a warning** into `debuglog` — sing-box would not accept unknown values. See the [VLESS docs](https://sing-box.sagernet.org/configuration/outbound/vless/)
- `spx`, `quicSecurity`, `authority` — common in Xray/panel links; they are **not** carried into the documented sing-box client JSON and do not affect parsing
- `mode` and `extra` — these **do** matter, but only with `type=xhttp`: `mode` goes into the transport as is, and `extra` is a URL-encoded JSON from which the same xhttp fields are read (values from `extra` override the flat parameters of the same name). See [the `xhttp` parameters](#the-xhttp-transport-parameters) below

**⚠️ TLS is disabled by port.** If `security` is empty and the port is one of the usual plaintext ports (80, 8080, 8880, 2052, 2082, 2086, 2095), no TLS block is emitted at all — the link is treated as a plain-HTTP node (a common case for Cloudflare subscriptions). To get TLS on such a port anyway, set `security=tls` explicitly.

**⚠️ Early data in the WebSocket path (`?ed=N`).** Xray hides the early-data setting inside the path itself: `path=/ws?ed=2048`. sing-box wants it in separate fields, so the parser strips `ed` out of the path and expands it into `max_early_data` + `early_data_header_name` (`Sec-WebSocket-Protocol`). Without that conversion the node passes `check` but answers 404 at runtime.

**⚠️ Vision over UDP:443 — the port is rewritten.** With `flow=xtls-rprx-vision-udp443` the parser **forces** `server_port=443` (regardless of the port in the URI) and `packet_encoding=xudp`. The flow's semantics are XTLS Vision over UDP traffic to the standard 443. If your server listens for Vision on a non-standard port, use `flow=xtls-rprx-vision` (without the `-udp443` suffix).

**Example:**
```
vless://uuid@server.com:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=example.com&fp=chrome&pbk=...&sid=...&type=tcp#🇳🇱 Netherlands
```

### The `xhttp` transport parameters

With `type=xhttp` (VLESS/Trojan) or `net=xhttp` (VMess) a `{"type":"xhttp", …}` transport is assembled. Values come from two sources: ordinary query parameters and **`extra`** — a URL-encoded JSON; on a key collision `extra` wins. Names are read in both snake_case and camelCase (`session_key` = `sessionKey`).

| Parameter (snake_case / camelCase) | Type in config | Meaning |
|---|---|---|
| `mode` | string | `auto` \| `packet-up` \| `stream-up` \| `stream-one`. On the fork `auto` = `packet-up`; `stream-one` has a known downlink-framing bug |
| `path` | string | The request path; the tail from the first `?` is trimmed (see the warning below) |
| `host` | string | The Host header; when empty, the SNI from TLS is substituted |
| `x_padding_bytes` / `xPaddingBytes` | string | A `"min-max"` range, default `100-1000`; carried in the `Referer` header |
| `no_grpc_header` / `noGRPCHeader` | bool | Drop the gRPC-compatible header |
| `no_sse_header` / `noSSEHeader` | bool | Drop the SSE-compatible header |
| `x_padding_obfs_mode` / `xPaddingObfsMode` | bool | Enable x-padding obfuscation |
| `session_placement`, `session_key` / `sessionPlacement`, `sessionKey` | string | Placement and key name for the session |
| `seq_placement`, `seq_key` / `seqPlacement`, `seqKey` | string | Placement and key name for the sequence |
| `uplink_data_placement`, `uplink_data_key` / `uplinkDataPlacement`, `uplinkDataKey` | string | Placement and key name for the uplink data |
| `uplink_chunk_size` / `uplinkChunkSize` | string | The uplink's chunk size |
| `uplink_http_method` / `uplinkHTTPMethod` | string | The uplink's HTTP method |
| `x_padding_key`, `x_padding_header`, `x_padding_placement`, `x_padding_method` (camelCase: `xPaddingKey`, `xPaddingHeader`, `xPaddingPlacement`, `xPaddingMethod`) | string | Fine tuning of the x-padding obfuscation |
| `sc_max_each_post_bytes` / `scMaxEachPostBytes` | string | The core expects a `"min-max"` string; a bare number (including `30.0` coming from `extra`) is normalized into a string |
| `sc_min_posts_interval_ms` / `scMinPostsIntervalMs` | string | The same rule |
| `sc_stream_up_server_secs` / `scStreamUpServerSecs` | string | The same rule |
| `sc_max_buffered_posts` / `scMaxBufferedPosts` | **number** | The core decodes it as an int, not as a string |

**The `xmux` fields** are written as the same flat parameters — no nested object is needed; the parser assembles `"xmux": {…}` inside the transport itself:

| Parameter (snake_case / camelCase) | Type in config | Meaning |
|---|---|---|
| `max_connections` / `maxConnections` | string | Connection-count limit (a `"min-max"` range is allowed) |
| `max_concurrency` / `maxConcurrency` | string | Concurrency limit (a range is allowed) |
| `c_max_reuse_times` / `cMaxReuseTimes` | string | How many times a connection is reused |
| `h_max_request_times` / `hMaxRequestTimes` | string | Request limit per HTTP connection |
| `h_max_reusable_secs` / `hMaxReusableSecs` | string | How long an HTTP connection stays reusable |
| `h_keep_alive_period` / `hKeepAlivePeriod` | **number** | The core decodes it as an int, not as a string |

**Boolean fields are emitted only when true.** `no_grpc_header`, `no_sse_header` and `x_padding_obfs_mode` are not written at all when `false` — the core's default is the absent field. `1`, `true` and `yes` count as true (case-insensitive).

**An example covering every group** (flat parameters, the recommended form):

```
vless://UUID@example.com:443?encryption=none&security=tls&sni=a.com&type=xhttp&mode=packet-up&path=%2Fgtm.js&host=a.com&xPaddingBytes=100-1000&scMaxEachPostBytes=1000000&scMaxBufferedPosts=30&maxConnections=1&maxConcurrency=16-32&hKeepAlivePeriod=30#node-01
```

yields the transport:

```json
{
  "type": "xhttp",
  "mode": "packet-up",
  "path": "/gtm.js",
  "host": "a.com",
  "x_padding_bytes": "100-1000",
  "sc_max_each_post_bytes": "1000000",
  "sc_max_buffered_posts": 30,
  "xmux": { "max_connections": "1", "max_concurrency": "16-32", "h_keep_alive_period": 30 }
}
```

The same fields may travel in `extra` (URL-encoded JSON) — on a key collision `extra` wins:

```
&extra=%7B%22maxConnections%22%3A1%2C%22scMaxBufferedPosts%22%3A30%7D
```

The nested form `extra={"xmux":{…}}` — the one Xray itself writes — is read as well: its members are flattened into the same fields. It is not needed for your own links; the flat form is shorter and equivalent.

**⚠️ The query tail in `path` is trimmed.** `path=/gtm.js?id-aabbccdd` yields `"path": "/gtm.js"` — everything from the first `?` counts as a query rather than the path (SPEC 002 §4.1; real nodes ship `path=/GaMeOpTiMiZeR?ed=2048`). Nothing else is normalized: a backslash (`\gtm.js`) and residual percent-encoding reach the config as-is — `check` accepts them and the server answers 404.

Values are not validated further — the core parses them. Implementation: `xhttpTransportFromQuery` / `xhttpBuildTransport` in `core/config/subscription/node_parser_transport.go`; specs: `SPECS/071-F-N-XHTTP_TRANSPORT/SPEC.md`, `sing-box-lx` SPEC 002.

### VMess (`vmess://`)
**⚠️ A quirk:** VMess is normally base64(JSON), but a **legacy** string after base64 is supported too: `method:uuid@host:port` with an optional `?query` (as in some clients). The `#tag` fragment is cut off **before** base64 decoding.

Format: `vmess://base64(json)` or `vmess://base64(cleartext)#tag`

The JSON must contain the fields:
- `v` — version (usually `"2"`)
- `ps` — name/tag
- `add` — server address
- `port` — port
- `id` — client UUID
- `aid` — alterId (optional)
- `scy` — the encryption method (optional)
- `net` — the network type (`tcp`, `ws`, `http`, `grpc`, **`httpupgrade`**, **`xhttp`**; **`h2`** → the `http` transport + TLS; **`xhttp`** → the native `xhttp` splithttp transport (the sing-box-lx core, build tag `with_xhttp`), separate from `httpupgrade`; see [the xhttp and AmneziaWG section](#the-xhttp-transport-and-amneziawg) at the top)
- `type` — the header type (for `tcp`)
- `host` — the host (for `ws`/`http`; for WS an empty `host` is filled in with the SNI from TLS, when present)
- `path` — the path (for `ws`/`http`/`grpc`)
- `tls` — whether TLS is used (`"tls"` or absent)
- `sni` — SNI (optional)
- `alpn` — ALPN (optional)
- `fp` — fingerprint (optional)
- `insecure` in the JSON (`"1"`) — insecure TLS, as with VLESS

**Building a VMess outbound with TLS:** `tls.server_name` comes from `sni`; failing that, from the **`peer`** field in the query (when the provider duplicated the name there); otherwise from the **server address** (`add`). The **`insecure` / `allowInsecure` / `allowinsecure`** flags in the query are handled exactly as for VLESS (`tlsInsecureTrue`).

**Legacy cleartext (not JSON):** the parser also accepts `vmess://base64("method:uuid@host:port?query")` — an old format used by some clients (early V2RayN and the like). After base64 decoding it is recognized as a URI with the same query parameters as the URI protocols: `type=ws`, `path`, `tls=1`, `host`, `sni` and so on; they map into `transport` and `tls`. The parser detects the format automatically from the first character of the decoded string: `{` → JSON, otherwise → legacy cleartext.

**Example:**
```
vmess://eyJ2IjoiMiIsInBzIjoiVGVzdCIsImFkZCI6InNlcnZlci5jb20iLCJwb3J0Ijo0NDMsImlkIjoi dXVpZCIsImFpZCI6MCwic2N5IjoiYXV0byIsIm5ldCI6InRjcCIsInR5cGUiOiJub25lIiwidGxzIjoiIn0=
```

### Trojan (`trojan://`)
The standard URI format: `trojan://password@server:port?params#tag`

The same **TLS** and **[V2Ray transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/)** rules as VLESS (including `type=ws`, `path`, `host` / `Host`, `type=httpupgrade`, `type=xhttp` — native splithttp just like VLESS, see [the xhttp and AmneziaWG section](#the-xhttp-transport-and-amneziawg) at the top), see **`SUBSCRIPTION_PARAMS_REPORT.md`** (023) and spec **029**.

**Query-string parameters (the usual ones):**
- `security` — for example `tls` or `none` (no TLS)
- `sni`, `host`, **`peer`** — SNI / certificate name (`sni` wins, then `peer`, then `host`, and finally the server address); for WS it is also the Host header
- `type` — `ws`, `grpc`, `http`, **`httpupgrade`**, **`xhttp`**, `tcp`/`raw` (plus `headerType=http` when needed) — as with VLESS. `xhttp` is the native splithttp transport (the sing-box-lx core, build tag `with_xhttp`), separate from `httpupgrade`
- `path` — the WebSocket path
- `alpn`, `fp`, `insecure` / `allowInsecure` — as with VLESS

**Example:**
```
trojan://password123@server.com:443?security=tls&sni=example.com#🇺🇸 United States
```

### Shadowsocks (`ss://`)

Two formats:

1. **SIP002 (preferred):** `ss://base64(method:password)@server:port#tag` — the userinfo carries base64-encoded method and password, while server/port stay plain.
2. **Legacy non-SIP002:** `ss://base64("method:password@server:port")#tag` — the whole `method:password@server:port` inside one base64 block. The parser detects and supports both automatically.

**Encryption methods (the `isValidShadowsocksMethod` allow-list):**

| Category | Methods |
|-----------|--------|
| Shadowsocks 2022 (recommended) | `2022-blake3-aes-128-gcm`, `2022-blake3-aes-256-gcm`, `2022-blake3-chacha20-poly1305` |
| AEAD GCM | `aes-128-gcm`, `aes-192-gcm`, `aes-256-gcm` |
| AEAD ChaCha20 | `chacha20-ietf-poly1305`, `xchacha20-ietf-poly1305` |
| No encryption | `none` (only if the server is configured accordingly) |

Any other method (legacy streaming RC4/AES-CFB, for instance) is **rejected at parse time** — sing-box would not accept it in an outbound, so creating the node makes no sense.

**Example:**
```
ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@server.com:443#Shadowsocks Server
```

### Hysteria2 (`hysteria2://` or `hy2://`)
**Scheme:** `hysteria2://` or `hy2://` (the official short form)

The standard URI format: `hysteria2://[auth@]hostname[:port]/?[key=value]&[key=value]...`

**Structure:**
- `auth` — authentication credentials (a password, or username:password for userpass)
- `hostname` — the server address
- `port` — the port (443 by default when omitted)
- `#tag` — the tag/comment (optional)

**Multi-port:** Hysteria2 accepts the port list from several sources, in priority order:
1. **The `mport` query** (or `ports`) — the canonical way. The value is a comma-separated list of ports / ranges: `mport=443,5000-6000,8443`.
2. **Authority-style `host:123,5000-6000`** — if the URI's port part contains a comma (which violates the RFC), the parser first rescues the tail through `hysteria2RecoverMultiPortAuthority`: the first port goes into `server_port`, and the remainder (the first one included) moves into the query as `mport`. A URI like `hysteria2://[email protected]:123,5000-6000/?...` is handled correctly.
3. If `mport` is empty and the port holds a single number, it is the simple single-port case.

From there sing-box takes over: with `server_ports` (a list) present, the client may open on any of them.

**Query-string parameters (per the official spec):**
- `obfs` — the obfuscation type: `salamander` or `gecko`. An unknown type degrades the node to obfuscation-off with a warning rather than taking the config down; an obfs without a password keeps working with obfuscation off (`node_parser_hysteria2.go`).
- `obfs-password` — the password for that obfuscation type
- `sni` — Server Name Indication for TLS connections
- `insecure`, **`allowInsecure` / `allowinsecure`** — insecure TLS (as with VLESS: `1` / `true` / `yes`); `skip-cert-verify` is honoured too, but only in the exact forms `true` / `1`
- `fingerprint` / `fp` — the uTLS fingerprint → `tls.utls` in sing-box
- `pinSHA256` — the base64 SHA-256 of the certificate's public key → `tls.certificate_public_key_sha256` in sing-box
- `alpn` — a comma-separated ALPN list (usually `h3` for hysteria2)
- `upmbps` / `downmbps` — bandwidth in Mbps → `up_mbps` / `down_mbps` in sing-box. Non-numeric values are ignored

**⚠️ About bandwidth.** `upmbps`/`downmbps` describe *your* link, not the server's, so strictly speaking they do not belong in a shared subscription (every user's are different). But when they are present in a link, the parser **reads them and carries them over** rather than dropping them. Client modes (HTTP, SOCKS5) are not supported in the URI.

**Examples:**
```
hysteria2://password123@server.com:443?sni=example.com&insecure=1#🇺🇸 United States
hy2://password@server.com:443?obfs=salamander&obfs-password=secret&sni=real.example.com#Server
hysteria2://[email protected]:123,5000-6000/?insecure=1&pinSHA256=deadbeef#Multi-port Server
```

**Official documentation:** [Hysteria 2 URI Scheme](https://v2.hysteria.network/docs/developers/URI-Scheme/)

### SSH (`ssh://`)
**⚠️ A custom format:** the SSH URI format is singbox-launcher's own, not a standard protocol.

The standard URI format: `ssh://user:password@server:port?params#tag`

**Query-string parameters:**
- `password` — the password (it can also go into the userinfo: `user:password@`)
- `private_key` — the private key (inline, URL-encoded)
- `private_key_path` — a path to the private-key file (for example `$HOME/.ssh/id_rsa`)
- `private_key_passphrase` — the passphrase for the private key
- `host_key` — the host key (several can be given, comma-separated, URL-encoded)
- `host_key_algorithms` — host-key algorithms (comma-separated)
- `client_version` — the client version (for example `SSH-2.0-OpenSSH_7.4p1`)

**Default port:** 22 (when omitted). If the user is missing, `root` is used, with a warning.

**Examples:**
```
ssh://root:admin@127.0.0.1:22#Local SSH
ssh://user@server.com:22?private_key_path=$HOME/.ssh/id_rsa#Git Server
ssh://root:password@192.168.1.1:22?private_key_path=/path/to/key&host_key=ecdsa-sha2-nistp256%20AAAA...&client_version=SSH-2.0-OpenSSH_7.4p1#My SSH Server
```

### SOCKS5 (`socks5://` or `socks://`)

Format: `socks5://[user:password@]host[:port]#tag`, or `socks://...` (the short form). In the generated sing-box config this is an outbound of **`type`: `socks`** with **`version`: `5`** (sing-box has no separate `socks5` type). In the parser's filters the **`scheme`** field keeps the original: **`socks5`** for `socks5://` links, **`socks`** for `socks://`.

**Structure:**
- `user:password` — optional authentication (the proxy's login and password)
- `host` — the SOCKS5 server's host or IP (required)
- `port` — the port (**1080** by default when omitted)
- `#tag` — the node's tag/comment (optional)

**Examples:**
```
socks5://myuser:mypass@proxy.example.com:1080#Office SOCKS5
socks5://proxy.example.com:1080
socks://127.0.0.1:1080#Local
```

### NaïveProxy (`naive+https://` / `naive+quic://`)

**Requirement:** sing-box must be built with NaïveProxy support (the `with_naive_outbound` build tag). The `sing-box-lx` fork core supports naive on every desktop platform from **`1.14.0-lx.4`** on: on Windows the outbound loads `libcronet.dll` at runtime (the launcher extracts it from the core's release archive into `bin/` on Download/Reinstall), while on macOS and Linux cronet is linked in statically. Permanent exceptions: `windows-386-legacy-windows-7` and mips (cronet is not built for those).

If the current core has no naive support (the tag is missing, or it is a purego build without libcronet next to the binary), the launcher **degrades naive nodes with a warning** (an Update toast, warnings on rebuild) so that a single node cannot fail the whole `sing-box check` (the probe: `core/core_capabilities.go::CoreSupportsNaive`).

**URI scheme** (de facto, DuckSoft 2020 — [gist](https://gist.github.com/DuckSoft/ca03913b0a26fc77a1da4d01cc6ab2f1)):

```
naive+https://<user>:<pass>@<host>:<port>/?<params>#<label>
naive+quic://<user>:<pass>@<host>:<port>/?<params>#<label>
```

- **Scheme:** `naive+https` is the HTTP/2 transport; `naive+quic` is QUIC (with an automatic `quic_congestion_control: bbr` in the JSON).
- **Userinfo:** `<user>:<pass>`, or just `<pass>` (which then lands in the user slot — as with hysteria2). Anonymous mode means no userinfo at all.
- **Port:** optional, default **443**.
- **Query:**
  - `padding=true|false` — **ignored** with a warning (sing-box has no matching field).
  - `extra-headers=<urlencoded "Header1: Value1\r\nHeader2: Value2">` — extra HTTP headers; invalid pairs (a bad charset in the name, CR/LF/NUL in the value) are skipped with a warning, the rest are kept.
- **Fragment (`#label`):** URL-decoded, UTF-8-fixed — as everywhere.

**Examples:**

```
naive+https://what:happened@test.someone.cf?padding=false#Naive!
naive+https://some.public.rs?padding=true#Public-01
naive+quic://manhole:114514@quic.test.me
naive+https://some.what?extra-headers=X-Username%3Auser%0D%0AX-Password%3Apassword
```

**The resulting outbound JSON** (sing-box ≥ 1.13.0, [doc](https://sing-box.sagernet.org/configuration/outbound/naive/)):

```json
{
  "type": "naive",
  "tag": "…",
  "server": "test.someone.cf",
  "server_port": 443,
  "username": "what",
  "password": "happened",
  "tls": { "enabled": true, "server_name": "test.someone.cf" }
}
```

For `naive+quic://`, `"quic": true` and `"quic_congestion_control": "bbr"` are added. The `extra-headers` block expands into `"extra_headers": {"X-Username": "user", "X-Password": "password"}`.

**The TLS block:** the sing-box naive outbound supports **only** `server_name`, `certificate`, `certificate_path`, `ech` — `alpn / utls / reality / min_version` do not apply to this type and are not emitted by the parser. A custom SNI in the URI is not supported yet (v1); `tls.server_name` = `host`. To override it by hand, edit `config.json` after the wizard's Save.

**Share URI (encoding back)** — `ShareURIFromOutbound` for `type: "naive"`:
- `naive+https://` or `naive+quic://` depending on `quic: true/false`.
- The `extra_headers` map is sorted lexicographically by key (to keep the round-trip deterministic), joined with `\r\n` and encoded into the query.
- `padding` is **not** restored (it is not stored in the outbound).

Implementation: `core/config/subscription/node_parser_naive.go` (helpers), `node_parser_core.go` (dispatch), `shareuri_naive.go` (`shareURIFromNaive`). Spec: [**SPECS/044-F-C-NAIVE_PROXY_PARSER/SPEC.md**](../SPECS/044-F-C-NAIVE_PROXY_PARSER/SPEC.md).

### TUIC (`tuic://`)

TUIC v5 is a proxy over QUIC: authentication by a `uuid` + `password` pair, a UDP relay and optional 0-RTT. The TLS block is mandatory (there is no QUIC without TLS), so the parser always writes `tls.enabled: true`.

**Format:** `tuic://<uuid>:<password>@<host>[:port]?<params>#<tag>`

**Structure:**
- `<uuid>` — the userinfo username → `uuid` in the outbound. Without it the node is still assembled, with the warning `TUIC link missing uuid`.
- `<password>` — the userinfo password → `password`. Also required, otherwise the warning `TUIC link missing password`.
- `port` — **443** by default.
- `#tag` — the tag/comment (optional).

**Query parameters:**

| Parameter | Meaning |
|---|---|
| `congestion_control` | The QUIC congestion controller: `cubic`, `new_reno`, `bbr` (case-insensitive). **⚠️ Any other value is dropped with a warning** — sing-box complains about unknown controllers when loading the config, so the parser would rather stay silent than fail the whole `config.json` |
| `udp_relay_mode` | `native` or `quic`. An unknown value is dropped with a warning |
| `zero_rtt_handshake` | `1`/`true`/`yes` → `zero_rtt_handshake: true`. The alias **`reduce_rtt`** is accepted (v2rayN/Nekobox send it) — either one is enough |
| `heartbeat` | The keepalive interval. **A bare number means seconds**: `heartbeat=10` → `"10s"`; a value with a unit (`10s`, `1m`) passes through as is |
| `sni` | → `tls.server_name`. **⚠️ Fallback:** when `sni` is empty, equals `🔒`, or does not look like a host (no dot and no colon), the server address is substituted |
| `insecure` / `allowInsecure` / `allow_insecure` / `skip-cert-verify` / `skipCertVerify` | `1`/`true`/`yes` → `tls.insecure: true`. All five spellings are checked explicitly: the key lookup is case-insensitive, but not separator-insensitive |
| `fp` / `fingerprint` | The uTLS fingerprint → `tls.utls`. The value is normalized against an allow-list (`NormalizeUTLSFingerprint`); `fp` wins, `fingerprint` is the fallback |
| `alpn` | A comma-separated list → `tls.alpn` (whitespace trimmed). When unset, the core applies its own default |

**Examples:**
```
tuic://b8b1a1e3-0a2c-4d0f-9a3b-1c2d3e4f5a6b:[email protected]:443?congestion_control=bbr&udp_relay_mode=quic&alpn=h3&sni=cdn.example.com#🇩🇪 TUIC DE
tuic://b8b1a1e3-0a2c-4d0f-9a3b-1c2d3e4f5a6b:[email protected]:8443?reduce_rtt=1&heartbeat=10&allow_insecure=1&fp=chrome#TUIC self-signed
```

**Share URI (encoding back)** — the round-trip preserves meaning, not bytes: `insecure` is always emitted in the canonical `insecure=1` form, and `zero_rtt_handshake=1` under its new name (never `reduce_rtt`).

Implementation: `core/config/subscription/node_parser_tuic.go`, `node_parser_core.go` (dispatch), `shareuri_tuic.go`.

### AnyTLS (`anytls://`)

AnyTLS is a proxy over ordinary TLS with a pool of reusable sessions and padding that masks packet sizes. The credential is **a single password**, as with Trojan. TLS is mandatory, so the parser always writes `tls.enabled: true`.

**Format:** `anytls://<password>@<host>[:port]?<params>#<tag>`

**Structure:**
- `<password>` — the userinfo **username** (not the password field!), as with Trojan → `password` in the outbound. Without it: the warning `AnyTLS link missing password (userinfo)`.
- `port` — **443** by default.
- `#tag` — the tag/comment (optional).

**Query parameters:**

| Parameter | Meaning |
|---|---|
| `sni` / `peer` | → `tls.server_name`; `sni` wins, `peer` is the fallback. **⚠️ Fallback:** when both are empty, the value equals `🔒`, or it does not look like a host (no dot and no colon), the server address is substituted |
| `insecure` / `allowInsecure` / `allow_insecure` / `skip-cert-verify` / `skipCertVerify` | `1`/`true`/`yes` → `tls.insecure: true` (all five spellings, as with TUIC) |
| `fp` / `fingerprint` | The uTLS fingerprint → `tls.utls`, normalized against the allow-list; `fp` wins |
| `alpn` | A comma-separated list → `tls.alpn` (whitespace trimmed) |
| `idle_session_check_interval` | How often the pool checks idle sessions. **A bare number means seconds** (`30` → `"30s"`), the same convention as TUIC's `heartbeat` |
| `idle_session_timeout` | How long a session may idle before it is closed; a bare number is seconds here too |
| `min_idle_session` | How many sessions to keep warm, an integer ≥ 0. **⚠️ A non-number or a negative value is dropped with a warning** |

**Examples:**
```
anytls://[email protected]:443?sni=cdn.example.com&alpn=h2,http/1.1&fp=chrome#🇳🇱 AnyTLS NL
anytls://[email protected]:8443?peer=example.com&insecure=1&idle_session_check_interval=30&idle_session_timeout=60&min_idle_session=2#AnyTLS pool
```

**Share URI (encoding back)** — the round-trip preserves meaning: the SNI always comes out as `sni` (never `peer`), insecure TLS as `insecure=1`, and `min_idle_session` is written only when greater than 0.

Implementation: `core/config/subscription/node_parser_anytls.go`, `node_parser_core.go` (dispatch), `shareuri_anytls.go`. Spec: [**SPECS/091-F-C-ANYTLS_PROTOCOL/SPEC.md**](../SPECS/091-F-C-ANYTLS_PROTOCOL/SPEC.md).

### MASQUE (`masque://`)
**⚠️ A custom format:** like SSH, `masque://` is a singbox-launcher URI dialect rather than a standard. It is symmetric with the launcher's own emission: whatever the WARP (MASQUE) wizard produces, the parser reads back without loss.

MASQUE (CONNECT-IP, RFC 9484) tunnels **whole IP packets** over HTTP/3 or HTTP/2 — primarily this is Cloudflare WARP. A core of **`1.14.0-lx.26+`** is required (the `vhttp` + nested `tls` schema, core SPEC 062); nodes are written into `outbounds[]`.

**Format:** `masque://<PRIVATE_KEY_DER>@<SERVER>:<PORT>?publickey=<PUB_DER>&address=<v4,v6>&...#tag`

**Structure:**
- `<PRIVATE_KEY_DER>` — the client's EC private key, **base64(DER)** (SEC1, `x509.ParseECPrivateKey`). It goes into the userinfo; `?private_key=`/`?privatekey=` are accepted too. A `/` inside the base64 is escaped by the parser itself.
- `<SERVER>:<PORT>` — the WARP endpoint (usually an IP), port `443` by default. A non-numeric port is not an error here — 443 is used silently (most other schemes reject such a URI).
- `#tag` — the tag/comment (optional).

**Query parameters:**

| Parameter | Required | Default | Meaning |
|---|---|---|---|
| `publickey` (`public_key`) | **yes** | — | The endpoint's public key, **base64(DER)** PKIX (`x509.ParsePKIXPublicKey`, ECDSA). This is what authenticates the endpoint — which is why the SNI is free |
| `address` | **yes** | — | The local addresses **inside** the tunnel, comma-separated (`172.16.0.2/32,2606:4700:...::/128`). A bare address becomes `/32` (v4) / `/128` (v6). At least one is needed; without it the core rejects the node (`at least one of ip/ipv6 is required`) |
| `vhttp` | no | `h3` | **The HTTP version** carrying CONNECT-IP: `h3` (QUIC) or `h2` (HTTP/2, TCP:443). Not the L4 list! An unrecognized value is forced to `h3` with a warning |
| `profile` | no | `cloudflare` | `cloudflare` (WARP quirks) or `standard` (strict RFC 9484, your own server) |
| `sni` | no | — | The name in the ClientHello → `tls.server_name`. Also accepted as `server_name` |
| `insecure` | no | — | `1`/`true` → `tls.insecure`. Also accepted as `skip_cert_verify`, `allowinsecure` |
| `mtu` | no | `1280` | The userspace stack's MTU. The parser accepts any positive number; the upper bound (16000 on `h2`) is checked by the core |
| `idle_timeout` | no | — | The idle period after which the tunnel is suspended (the next dial brings it back) |
| `keep_alive` (`keep_alive_period`) | no | — | The QUIC keepalive, `h3` only |

**⚠️ `vhttp`, not `network`.** Before core `lx.26` the HTTP version was called `network` — the exact opposite of what `network` means on every other protocol (there it is the tcp/udp list). The core renamed the field (core SPEC 062) and accepts the old one until `lx.30` with a warning. **The launcher's parser understands both forms** (`?network=h2` works — foreign subscriptions still send it), but into `config.json` it always writes the new one: `vhttp` + a nested `tls`. On a conflict (`vhttp=h3&network=h2`) the new name wins.

**⚠️ The default SNI is not the endpoint's address.** Naming the MASQUE endpoint in the ClientHello is exactly what a DPI looks at. The WARP wizard substitutes a neutral domain from its own pool, and with an empty SNI the core falls back to the profile's default. The endpoint is authenticated by pinning `publickey`, so the SNI can be anything.

**Examples:**
```
masque://MHcCAQEEIA...@162.159.198.2:443?publickey=MFkwEwYHKoZI...&address=172.16.0.2/32,2606:4700:110:8142::/128&vhttp=h3&sni=www.microsoft.com#🎭 WARP
masque://MHcCAQEEIA...@162.159.198.2:443?publickey=MFkwEwYHKoZI...&address=172.16.0.2/32&vhttp=h2&mtu=1400#WARP h2
```

**What ends up in `config.json`:**
```jsonc
{
  "type": "masque",
  "tag": "🎭 WARP",
  "server": "162.159.198.2",
  "server_port": 443,
  "profile": "cloudflare",
  "vhttp": "h3",
  "tls": { "server_name": "www.microsoft.com" },
  "private_key": "<base64 DER EC>",
  "public_key":  "<base64 DER PKIX>",
  "ip":   "172.16.0.2/32",
  "ipv6": "2606:4700:110:8142::/128",
  "mtu":  1280
}
```

**Importing a ready-made config.** An outbound in the **old** schema (flat `network`/`sni`/`skip_cert_verify`) is normalized on import: a flat `sni` must not coexist with `tls.server_name` — that is two sources for one name, and on a mismatch the core fails fast. `utls`/`reality` are stripped for masque (it runs over QUIC, and the core ignores them anyway).

**The keys cannot be derived from a URI** — they come from a Cloudflare registration. The easiest way to get a node is the wizard: **Config Wizard → WARP → MASQUE**, which registers an account and assembles the `masque://` link for you.

Implementation: `core/config/subscription/node_parser_masque.go`, `shareuri_masque.go`, `core/warp/masque.go`. Specs: [**SPECS/086-F-O-MASQUE_WARP_INTEGRATION/SPEC.md**](../SPECS/086-F-O-MASQUE_WARP_INTEGRATION/SPEC.md); the field schema is in `sing-box-lx/docs-lx/lx-config.md` §4.

### WireGuard (`wireguard://`)
**⚠️ A quirk:** WireGuard nodes are written into the config's **endpoints** section (not into outbounds). **sing-box 1.11+** is required.

The standard URI format: `wireguard://<PRIVATE_KEY>@<SERVER>:<PORT>?params#tag`

The userinfo holds the client's private key. Special characters in the query must be URL-encoded: `/` → `%2F`, `,` → `%2C`. **A raw `/` inside a base64 key** (common in non-canonical links) is now tolerated — the parser percent-encodes it itself before parsing (see subtask 073.1 in the AmneziaWG spec).

**Query-string parameters:**
- `publickey` — the server's public key (base64, required)
- `address` — the client's address inside the VPN, CIDR (for example `10.10.10.2/32`, required). A bare IP without a mask (`172.16.0.2`, as in AmneziaWG/`.conf` exports) is padded to `/32` (IPv4) / `/128` (IPv6).
- `allowedips` — allowed routes, comma-separated CIDRs (for example `0.0.0.0/0,::/0`, required). Bare IPs get a prefix the same way.
- `mtu` — the MTU (1420 by default)
- `keepalive` — the keepalive interval, in seconds
- `presharedkey` — the PSK (base64)
- `listenport` — the local listen port (when set, `listen_port` is added to the endpoint)
- `name` — the interface name (defaults to `singbox-wg0`)
- `dns` — DNS servers

An invalid `keepalive`/`mtu`/`listenport` value is dropped silently rather than failing the node.

**Example:**
```
wireguard://privatekey-base64@10.0.0.1:51820?publickey=server-pubkey-base64&address=10.10.10.2%2F32&allowedips=0.0.0.0%2F0%2C%3A%3A%2F0&keepalive=25&mtu=1420#My WG
```

**`reserved` (Cloudflare WARP).** The byte triple `reserved=b0,b1,b2` goes into `peers[0].reserved`. The value must be three numbers in 0–255; otherwise the field is skipped silently (the node still works — WARP lives without it on many paths).

**The `ip` / `id` / `ib` masquerade sugar.** A WireSock-style shorthand for AWG's first decoy packet: `ip` is the masquerade protocol (`quic` \| `dns` \| `stun` \| `sip`), `id` is the masquerade domain (LDH-validated; required for `quic`, a pseudo-name is generated for `dns`/`sip`, and `stun` ignores it), `ib` is the browser (`chrome` \| `firefox` \| `curl`, only meaningful with `ip=quic`). The core expands them into `i1` itself, so the sugar is **mutually exclusive** with an explicit `i1`.

**Parsing details:** the private key from the userinfo is decoded via PathUnescape. In `publickey` and `presharedkey` a `+` (part of base64) is preserved during parsing.

**AmneziaWG 2.0 (optional — a sing-box-lx core with `with_awg`):**

The same `wireguard://` links may carry AmneziaWG obfuscation parameters — they are promoted to the root of the WireGuard endpoint, next to `private_key`/`peers`:

- **Numeric** (uint32 → a JSON number): `jc` (the number of junk packets before the handshake), `jmin`/`jmax` (min/max junk size), `s1`/`s2` (junk before the init/response handshake), `s3`/`s4` (junk before the cookie-reply/transport — **AWG 2.x**), `h1`–`h4` (magic headers for the 4 WireGuard message types).
- **String** (case-sensitive tag format): `i1`–`i5` — **AWG 2.0** CPS decoy packets, sent in order before the handshake. Tags: `<b 0xHEX>` static bytes, `<c>` a counter, `<t>` a timestamp, `<r N>` / `<rc N>` / `<rd N>` — random bytes / characters / digits.

The numeric field names are read from the query in any case; `i1`–`i5` are taken verbatim (case preserved). `H1`–`H4` may be given as a **range** `lo-hi` (AWG 2.0 header randomization — that is how Amnezia exports it): the range is passed into the endpoint **as is, as a string** `"h1": "N-M"` (a reversed one is normalized, the bounds are uint32) — a **sing-box-lx ≥ 1.13.13-lx.6** core picks a value inside the range per handshake; single values are emitted as JSON numbers, as before. Ranges are allowed on `h1`–`h4` only (subtask 073.2). An endpoint with **no** AWG field at all is plain WireGuard (byte-identical to upstream). The client and the server must carry **matching** AWG parameters — the I-packets are configuration, not something negotiated over the wire. The mapping is 1:1 with `awg.conf` (awg-quick): `[Interface] Jc/Jmin/Jmax/S1–S4/H1–H4/I1–I5` → the endpoint root, `[Peer] …` → `peers[0]`.

**An AWG endpoint's MTU is clamped down to `1280`** (the AmneziaWG recommendation). `s3`/`s4` add padding to **every** transport packet, so at `mtu=1420` the resulting packet exceeds the path MTU and the OS rejects it (`sendmsg: message too long`): the handshake succeeds, but data silently stalls. The parser's policy for a WireGuard endpoint **with** AWG fields: an `mtu` above 1280 in the URI is lowered to 1280; an explicitly smaller value (say `mtu=1200`) is respected; with no `mtu` at all the default is 1280 (not 1420). Plain WireGuard (no AWG fields) keeps the upstream default of 1420. The ceiling without headroom is `1500 − 28 (UDP/IP) − 32 (WG) − max(s3,s4)`; 1280 is the IPv6 minimum and is safe on PPPoE/mobile/nested paths. **The server must use a symmetric MTU** — otherwise large return packets hit the same path-MTU wall.

**Example (AWG2):**
```
wireguard://privkey-base64@server.example.com:51821?publickey=server-pubkey&address=10.0.0.2%2F32&allowedips=0.0.0.0%2F0%2C%3A%3A%2F0&keepalive=25&jc=10&jmin=50&jmax=100&s1=20&s2=20&s3=60&s4=60&h1=1234567890&h2=1234567891&h3=1234567892&h4=1234567893&i1=%3Cb%200x000100002112a442%3E%3Cr%2012%3E#AWG2
```
(`i1` here is the URL-encoded `<b 0x000100002112a442><r 12>`.) Support is implemented in `applyAWGFields` / `ShareURIFromWireGuardEndpoint` (`core/config/subscription/node_parser_wireguard.go`, `shareuri_wireguard.go`); at runtime it needs a core with `with_awg`. See `SPECS/073-F-N-AMNEZIAWG_PARAMS/SPEC.md` and `sing-box-lx/docs-lx/lx-config.md`.

### Amnezia (`vpn://`)

The **`vpn://…`** links exported by Amnezia VPN / AmneziaWG 2.0 (a `.vpn` file is one such link) are accepted directly: paste the link into Sources or Connections. The format (the reference being `amnezia-vpn/config-decoder`): `vpn://` + base64url without padding, inside it qCompress (4 big-endian length bytes + zlib), and under that the JSON of the whole Amnezia profile.

Only the **WireGuard/AmneziaWG container** is imported from the profile (OpenVPN/Cloak/XRay containers are skipped): `defaultContainer` is tried first, then the rest in order. The `[Interface]/[Peer]` config found there is converted into a canonical `wireguard://` URI (see the parameter table above), so the same rules apply: bare IPs normalized into CIDRs, the AWG fields `Jc`/`Jmin`/`Jmax`/`S1`–`S4`/`H1`–`H4`/`I1`–`I5` promoted to the endpoint root, and **the AWG endpoint's MTU clamped to 1280** — the `MTU = 1420` from an Amnezia config reliably breaks data transfer (`sendmsg: message too long`). The node name comes from the profile's `description`, then `hostName`, then the container name.

Limits: a link up to 512 KB, an unpacked profile up to 8 MB (zlib-bomb protection). A profile with no WG/AWG container yields an error listing the containers it did have. Implementation: `core/config/subscription/node_parser_amnezia.go`; spec: `SPECS/075-F-C-AMNEZIA_VPN_IMPORT/SPEC.md`; a reference decoder for debugging: `scripts/decode_amnezia_vpn.py`.

### Raw `.conf` text (`[Interface]/[Peer]`)

The contents of a WireGuard/AmneziaWG `.conf` file can be pasted into the Add field of the Sources tab **as is** — the classifier picks `[Interface]` blocks out of the pasted text before line-by-line parsing and converts each into a canonical `wireguard://` URI (the URI is what gets stored and shared). Several blocks in one paste produce several nodes; links in the same text keep working. The node name is the host from `Endpoint`. AWG fields and the MTU clamp behave as with `vpn://` above. An invalid block is skipped with a log warning instead of failing the whole paste. Implementation: `core/config/subscription/wgconf_text.go` plus the hook in `classifyInputLines` (`ui/configurator/business/parser.go`); spec: `SPECS/076-F-C-WGCONF_PASTE_IMPORT/SPEC.md`.

### A subscription that serves a `.conf` or a `vpn://` profile

The two forms above describe what a user can **paste**. A subscription URL may
also *serve* them, and until SPEC 103 phase 2 such a body produced zero nodes
without a single message: anything that was not JSON fell through to
line-by-line URI parsing, which found no links.

Both are now recognised as body kinds of their own:

- **wg-quick `.conf`** (`BodyKindWGConf`) — the body is reduced to canonical
  `wireguard://` URIs *before* branching, so AWG fields and the MTU clamp keep
  working through the one parser that already implements them. Several
  `[Interface]` sections in one file give several nodes; a block without a
  `[Peer]` endpoint is skipped with a count rather than voiding the whole
  subscription.
- **Amnezia `vpn://`** (`BodyKindVPNLink`) — detected before the base64
  heuristic (`:` is outside the base64 alphabet). **All** WG/AWG containers of
  the profile are imported, not just the default one: a profile carrying
  several locations is the normal Amnezia case. Order is deterministic
  (default container first) and each node's label carries its container name —
  otherwise `MakeTagUnique` would turn the locations into "…-2"/"…-3" with no
  way to tell them apart. Uncompressed profiles (plain base64 JSON, which
  Amnezia also exports) are accepted alongside the qCompress form.

Implementation: `core/config/subscription/body_classify.go`,
`wgconf_text.go` (`WGConfBodyToURIs`), `node_parser_amnezia.go`
(`ParseAmneziaVPNLinkAll`). Fixtures: `contract/corpus/body/`.


### Add from file

WG/AmneziaWG configs are often handed out as files — the **"Add from file"** button on the Sources tab (next to Get free) opens a **native system dialog** for picking a file (`.conf` / `.vpn` / `.txt`) and runs its contents through the same path as the Add field: `.conf` → a WG/AWG node, `.vpn` → an Amnezia profile, text with links → nodes. The file limit is 1 MB. The native dialog: `osascript` (macOS), PowerShell `OpenFileDialog` (Windows), `zenity`/`kdialog` (Linux); if neither is present on Linux, it falls back to the built-in Fyne dialog. Implementation: `platform.PickOpenFile` (SPEC 082) + `business.ReadSourceFileText` in `ui/configurator/tabs/source_tab.go`; specs: `SPECS/079-F-N-ADD_SOURCE_FROM_FILE/`, `SPECS/082-F-N-NATIVE_FILE_PICKER/`.
