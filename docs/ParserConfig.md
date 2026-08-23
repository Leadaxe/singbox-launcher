# singbox-launcher subscription parser reference

**🌐 Language**: English | [Русский](ParserConfig.ru.md)

> **Where this lives (schema v6).** Parser settings live in **`state.json`**, not in `config.json`. Subscriptions are in `connections.sources[]`, shared selectors in `connections.outbounds[]` (see [`WIZARD_STATE.md`](WIZARD_STATE.md)); the per-source `tag_prefix`, `tag_postfix`, `tag_mask`, `skip`, `outbounds`, `disabled` live on the Source.
>
> ⚠️ **The `/** @ParserConfig ... */` block in `config.json` is gone** — removed in SPEC 045, `state.json` is now the single (canonical) store. Into `config.json` the parser writes only the nodes themselves, between the `@ParserSTART`/`@ParserEND` markers (and `@ParserSTART_E`/`@ParserEND_E` for endpoints).
>
> The document below describes the **field format and the parsing logic** — it applies to `connections.sources[]` / `connections.outbounds[]` 1:1.

## Purpose

The parser updates `bin/config.json` by loading subscriptions (see the [«Supported protocols»](#supported-protocols) table below — 13 protocols: VLESS, VMess, Trojan, Shadowsocks, Hysteria2, SSH, SOCKS5, NaïveProxy, WireGuard/AmneziaWG, TUIC, Amnezia (`vpn://`), MASQUE, AnyTLS), filtering them and grouping them into selectors. The result is written between the `/** @ParserSTART */` and `/** @ParserEND */` markers (outbounds); WireGuard nodes go between `/** @ParserSTART_E */` and `/** @ParserEND_E */` (endpoints). The **endpoints** section (WireGuard) is supported by sing-box from version **1.11** on.

### Supported protocols

| # | URI scheme | sing-box `type` | Config section | Version / build tag | Description |
|---|-----------|-----------------|----------------|--------------------|----------|
| 1 | `vless://` | `vless` | `outbounds[]` | core (+ **`with_xhttp`** for xhttp) | TCP/raw/ws/grpc/http/`httpupgrade`/quic/**`xhttp`** (splithttp), TLS, Reality, Vision flow. xhttp is native on the sing-box-lx core (see below). |
| 2 | `vmess://` | `vmess` | `outbounds[]` | core (+ **`with_xhttp`**) | Base64 JSON or legacy cleartext `method:uuid@host:port`. `net=h2`→`http`+TLS; `net=xhttp`→**`xhttp`**, `net=httpupgrade`→`httpupgrade` (distinct transports). |
| 3 | `trojan://` | `trojan` | `outbounds[]` | core | Same transport/TLS as VLESS. Password in the userinfo. |
| 4 | `ss://` | `shadowsocks` | `outbounds[]` | core | SIP002 + legacy `ss://base64("method:password@host:port")`. Methods are a fixed allow-list (2022-blake3, AEAD GCM, ChaCha20-Poly1305). |
| 5 | `hysteria2://`, `hy2://` | `hysteria2` | `outbounds[]` | core (QUIC) | Multi-port (`mport`/`ports` query, or `host:123,5000-6000` in the authority); obfs is `salamander` only. |
| 6 | `ssh://` | `ssh` | `outbounds[]` | core | **A singbox-launcher URI dialect**, not an RFC. Inline key / key path / passphrase / host_key. |
| 7 | `socks5://`, `socks://` | `socks` (version=5) | `outbounds[]` | core | User/pass optional. The `scheme` filter field keeps the original (`socks5` vs `socks`). |
| 8 | `naive+https://`, `naive+quic://` | `naive` | `outbounds[]` | **sing-box ≥ 1.13.0** + build tag **`with_naive_outbound`** (fork core `1.14.0-lx.4+` — all desktop platforms; on Windows `libcronet.dll` is required and the launcher ships it). On a core without support such nodes are **degraded with a warning** instead of breaking the config. | DuckSoft 2020 URI dialect. `extra-headers=` (CRLF-separated pairs). TLS carries `server_name` only. |
| 9 | `wireguard://` | `wireguard` | **`endpoints[]`** | **sing-box ≥ 1.11** (+ **`with_awg`** for AmneziaWG) | One peer; `@ParserSTART_E`/`@ParserEND_E` markers. Default port 51820, mtu 1420. Optional **AmneziaWG 2.0** parameters (jc/jmin/jmax, s1–s4, h1–h4, i1–i5) — see below. |
| 10 | `tuic://` | `tuic` | `outbounds[]` | core (QUIC) | TUIC v5: `uuid:password` in the userinfo. Query: `congestion_control` (cubic/new_reno/bbr), `udp_relay_mode` (native/quic), `alpn`, `sni`, `allow_insecure`, `reduce_rtt`/`zero_rtt_handshake`, `heartbeat`, `fp`. TLS is mandatory (QUIC). |
| 11 | `vpn://` | `wireguard` | **`endpoints[]`** | same as #9 | An **Amnezia** profile (a `.vpn` file: base64url + qCompress + JSON, SPEC 075): the WG/AWG container is imported and converted into a canonical `wireguard://` URI. See the Amnezia (`vpn://`) section below. |
| 12 | `masque://` | `masque` | `outbounds[]` | **fork core `1.14.0-lx.26+`** (the `vhttp`+`tls` schema, core SPEC 062) | **A singbox-launcher URI dialect.** MASQUE / CONNECT-IP (RFC 9484) — whole IP packets over HTTP/3 (`h3`) or HTTP/2 (`h2`), primarily Cloudflare WARP. base64(DER) keys, tunnel addresses in `address=`. Nodes are usually produced by the WARP wizard. See the MASQUE (`masque://`) section below. |
| 13 | `anytls://` | `anytls` | `outbounds[]` | core (core rc.17+: `option/anytls.go`) | Password in the userinfo, mandatory TLS block; session-pool tuning (`idle_session_check_interval`, `idle_session_timeout`, `min_idle_session`). SPEC 091. |

Besides URIs, the Add field accepts **raw `[Interface]/[Peer]` text** (a WireGuard/AmneziaWG `.conf`, AWG fields included) — conf blocks are detected before line-by-line parsing and converted into `wireguard://` URIs (SPEC 076); see the `.conf` text section below.

**Not supported** (explicitly, not implemented): **ShadowTLS**, **Mieru**, **Hysteria 1** (v2 only), **ShadowsocksR / SSR**, **Tor**, a plain HTTP proxy as a node type (an `http(s)://...` URL is always a **subscription source**, never a node). Selectors (`selector`, `urltest`, `direct`, `block`, `dns`) are not URI protocols; they are assembled on the ParserConfig side (see the [`outbounds` section](#the-outbounds-section)).

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

## Configuration versioning

The parser uses a versioning scheme to manage changes in the configuration structure:

- **Version 1** (obsolete): the version sat at the JSON top level
- **Version 2** (obsolete): the version moved inside `ParserConfig`, and a nested `outbounds` object appeared with the `proxies`, `addOutbounds`, `preferredDefault` fields
- **Version 3** (obsolete): a flat structure, with `filters`, `addOutbounds` and `preferredDefault` at the top level of the outbound object
- **Version 4** (current): support for local outbounds in `ProxySource` and for node-tag prefixes/postfixes

**Not to be confused with the `state.json` schema** — that has its own numbering (currently **v6**, see [`WIZARD_STATE.md`](WIZARD_STATE.md)). `version: 4` describes the ParserConfig block format only.

## Configuration format

The structure lives in `state.json` (the `parser_config` key; subscriptions and selectors are also kept in canonical form in `connections.sources[]` / `connections.outbounds[]`). The format:

```json
{
  "version": 4,
  "proxies": [...],
  "outbounds": [...],
  "parser": {
    "reload": "4h",
    "last_updated": "2025-12-16T03:21:19Z"
  }
}
```

⚠️ **`version: 4` is the version of the ParserConfig block, not of the `state.json` schema.** The two numberings are independent: `ParserConfigVersion = 4` (`core/config/configtypes/types.go`) and `SchemaVersionV6 = 6` (`core/state/disk_v6.go`). The example below shows the fields of this structure — it used to be the contents of the `@ParserConfig` block in `config.json`, and is now the contents of `state.json`.

## A full annotated configuration example

The contents of `parser_config` in `state.json` (formerly the `@ParserConfig` block in `config.json`, removed in SPEC 045):

```jsonc
{
  // Configuration version (current: 4)
  "version": 4,
  
  // The list of proxy-server sources
  "proxies": [
    {
      // Subscription URL (Base64, plain text, or a JSON array of Xray configs)
      // Supported: VLESS, VMess, Trojan, Shadowsocks, Hysteria2,
      // TUIC, SSH, SOCKS5, NaïveProxy, WireGuard/AWG, Amnezia, MASQUE, AnyTLS.
      // See the "Supported protocols" table at the top of this document.
      "source": "https://your-subscription-url.com/subscription",
      
      // Direct links to proxy servers (optional)
      // Can be combined with subscriptions
      "connections": [
        "vless://uuid@server.com:443?security=reality&sni=example.com&fp=chrome&pbk=...&sid=...&type=tcp#🇳🇱 Netherlands",
        "vmess://eyJ2IjoiMiIsInBzIjoi...",
        "trojan://password@server.com:443?security=tls&sni=example.com#🇺🇸 United States",
        "hysteria2://password@server.com:443?sni=example.com&insecure=1#🇺🇸 United States",
        "hy2://password@server.com:443?sni=example.com#🇺🇸 United States (short form)",
        "ssh://root:admin@127.0.0.1:22#Local SSH",
        "socks5://user:pass@proxy.example.com:1080#Office SOCKS5",
        "wireguard://privatekey@10.0.0.1:51820?publickey=...&address=10.10.10.2/32&allowedips=0.0.0.0/0,::/0#WireGuard VPN"
      ],
      
      // Filters for excluding nodes (optional)
      // If at least one filter matches, the node is skipped
      "skip": [
        { "tag": "/🇷🇺/i" },  // Exclude every node whose tag contains 🇷🇺
        { "host": "/test\\./i" } // Exclude nodes whose host contains "test."
      ],
      
      // A prefix for every node tag from this source (optional, version 4)
      // Prepended to the node's original tag
      // The wizard adds "1:", "2:", "3:" and so on automatically when there are several subscriptions
      // Supports variables: {$tag}, {$scheme}, {$protocol}, {$server}, {$port}, {$label}, {$comment}, {$num}
      // Example: "tag_prefix": "{$num} {$protocol}:" → "1 vless:", "2 vmess:" and so on
      // Ignored when tag_mask is set
      "tag_prefix": "1:",
      
      // A postfix for every node tag from this source (optional, version 4)
      // Appended after the node's original tag
      // Supports the same variables as tag_prefix
      // Ignored when tag_mask is set
      "tag_postfix": "--xx",
      
      // A mask that replaces the node tag entirely (optional, version 4)
      // When set, it replaces the node tag completely, ignoring tag_prefix and tag_postfix
      // Supports the same variables as tag_prefix/tag_postfix
      // Example: "tag_mask": "{$num} {$protocol} : {$label}" → "1 vless : United States, New York"
      "tag_mask": "",
      
      // Local outbounds for this source (optional, version 4)
      // Applied only to nodes from this source
      // Tags of local outbounds are added automatically to the list of available outbounds
      // on the wizard's second tab (Rules), so they can be used in routing rules
      "outbounds": [
        {
          "tag": "local-selector",
          "type": "selector",
          "filters": { "tag": "/source1-/i" },
          "comment": "Local selector for this source"
        }
      ]
    },
    {
      // Several sources can be added
      "source": "https://another-subscription-url.com/sub",
      "connections": [],
      "skip": []
    }
  ],
  
  // The list of selectors (proxy groups)
  "outbounds": [
    {
      // Selector name (required)
      // Used in the Clash API tab UI for switching proxies
      "tag": "proxy-out",
      
      // Selector type (required)
      // Supported: "selector", "urltest"
      "type": "selector",
      
      // Extra options for the selector (optional)
      // These fields are added as top-level keys of the resulting selector JSON
      "options": {
        "interrupt_exist_connections": true   // Interrupt existing connections on switch
      },

      // SPEC 104: a Direction's display name (optional; empty = the tag is shown)
      "label": "VPN ①",

      // SPEC 104: paired auto-select group <tag>-auto (optional). When present the
      // launcher emits a urltest with the SAME nodes as this Direction, offers it
      // as the first option and makes it the default unless preferredDefault
      // matched something. The twin is never stored — it is expanded on build.
      "auto": {
        "mode": "least_test",              // least_test | round_robin
        "url": "@urltest_url",             // @var references are substituted at build
        "interval": "@urltest_interval",
        "tolerance": "@urltest_tolerance",
        "interrupt_exist_connections": true
      },
      
      // The main filter for picking nodes (version 4, optional)
      // Logic: OR between objects in an array, AND between keys inside one object
      // In version 2 this was called "outbounds.proxies"
      "filters": {
        // Exclude every node whose tag contains 🇷🇺 or 🇺🇸
        "tag": "!/(🇷🇺|🇺🇸)/i"
      },
      
      // Tags prepended to the selector's outbound list (optional)
      // Useful for adding "direct-out", "reject" and other static outbounds
      // In version 2 this was called "outbounds.addOutbounds"
      "addOutbounds": ["direct-out"],
      
      // A filter that picks the default node (optional)
      // The first node matching the filter becomes the selector's "default" value
      // In version 2 this was called "outbounds.preferredDefault"
      "preferredDefault": {
        "tag": "/🇳🇱/i"  // Pick a node whose tag contains 🇳🇱 as the default
      },
      
      // A comment printed above the selector's JSON (optional)
      "comment": "Proxy group for international connections"
    },
  ],
  
  // Parser settings (optional, filled in automatically)
  "parser": {
    "reload": "4h",                    // Auto-update interval (default "4h")
    "last_updated": "2025-12-16T03:21:19Z"  // Time of the last update (RFC3339, UTC, updated automatically)
  }
}
```

## Field reference

### The `proxies` section

An array of objects describing proxy-server sources.

| Field          | Type      | Required | Description |
|---------------|----------|--------------|----------|
| `source`      | string   | Yes           | The subscription URL. All 13 protocols from the [«Supported protocols»](#supported-protocols) table: VLESS, VMess, Trojan, Shadowsocks, Hysteria2, SSH, SOCKS5, NaïveProxy, WireGuard/AmneziaWG, TUIC, Amnezia (`vpn://`), MASQUE, AnyTLS. Base64 and plain text are both accepted, as is a **JSON array** of full Xray configs (`[ {...}, ... ]`), see above. |
| `connections` | array    | No          | An array of direct links. All 13 schemes from the [«Supported protocols»](#supported-protocols) table: `vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://`/`hy2://`, `tuic://`, `ssh://`, `socks5://`/`socks://`, `naive+https://`/`naive+quic://`, `wireguard://`/`awg://`, `vpn://` (Amnezia), `masque://`, `anytls://`. Can be combined with subscriptions. WireGuard nodes land in the config's `endpoints` section (sing-box ≥ 1.11). NaïveProxy requires sing-box ≥ 1.13.0 + the `with_naive_outbound` build tag (fork core `1.14.0-lx.4+`). More in [URI formats for direct links](#uri-formats-for-direct-links). |
| `skip`        | array    | No          | A list of filters. If at least one matches, the node is skipped. |
| `tag_prefix`  | string   | No          | A prefix added to every node tag from this source (version 4). Applied before the original tag. Supports the variables `{$tag}`, `{$scheme}`, `{$protocol}`, `{$server}`, `{$port}`, `{$label}`, `{$comment}`, `{$num}`. Ignored when `tag_mask` is set. |
| `tag_postfix` | string   | No          | A postfix added to every node tag from this source (version 4). Applied after the original tag. Supports the same variables as `tag_prefix`. Ignored when `tag_mask` is set. |
| `tag_mask`    | string   | No          | A mask that replaces the node tag entirely (version 4). When set, it replaces the tag completely, ignoring `tag_prefix` and `tag_postfix`. Supports the same variables as `tag_prefix`/`tag_postfix`. |
| `outbounds`   | array    | No          | Local outbounds for this source (version 4). Applied only to nodes from this source. **As of SPEC 108 they are no longer offered as rule targets** — a subscription's group is a grouping and sugar for a direction, not a target that rules should hold onto: it disappears with the subscription and is renamed with its prefix. The launcher stops writing marked groups here entirely (see `fold`); hand-written entries still generate. |
| `fold` | object | No | **SPEC 108** — folds the subscription into a single group: `{"mode": "select" \| "auto" \| "select_auto", "auto": {…}}`. Absent means not folded. At generation time (pass 0, `config.PrepareSourceFolds`) it materialises the groups `<trim(tag_prefix)>auto` and/or `<trim(tag_prefix)>select` into `proxies[i].outbounds` and sets both flags below itself, so the subscription arrives in the directions as **one** entry instead of all its nodes. `auto` carries the auto-group settings in the same shape as a direction's `outbounds[].auto`. Under `select_auto` the auto group is the selector's first option and its `default`, and it is **not** exposed as a separate direction candidate — otherwise the user would be offered both the subscription and its own auto-select. |
| `exclude_from_global` | bool | No | When `true`, this source's nodes do **not** enter the pool for the **global** `ParserConfig.outbounds` entries during config generation. Local `proxies[i].outbounds` still use this source's nodes only. The field is `omitempty`; it affects generator behaviour only and does not change the global JSON. A `fold` sets it itself; the wizard no longer exposes it. |
| `expose_group_tags_to_global` | bool | No | Set by a `fold` at generation time; the wizard no longer exposes it. When `true`, during generation the tags of the wizard-**marked** local groups (see below) are **added** to the effective outbound list of **every** global `ParserConfig.outbounds` entry. The stored `outbounds[].addOutbounds` array is **not** rewritten. Strings from a user's own `addOutbounds` are still **not** filtered through `filters`; the injected tags do pass the same `filters` as nodes (a synthetic match on `tag`/`comment`). |
| `detour_tag` | string | No | **SPEC 077.** The tag of another outbound through which **all** of this source's nodes are dialled (a proxy chain / hop): the node gets `"detour":"<tag>"` (`A through B`). Empty means a direct connection. Stored by tag (like rules and selectors). Not applied to WireGuard nodes, nor to nodes with their own Xray chain (`dialerProxy` wins). **Fail-open** at generation time: a self-reference and a cycle among nodes (`A.detour=B`, `B.detour=A`) are detected and broken with a warning (the node then works directly) — otherwise the core would reject the config outright. A dangling detour onto a **template/preset group** tag is not dropped (group tags are only known at final assembly). **The UI choice is deliberately narrowed** (`business.DetourOptions`): only **manual Directions** (from the Directions tab) and **active preset groups** are offered; built-in/utility ones (`direct-out`/`reject`/`drop`), paired auto groups (`<tag>-auto`), the subscription's own local groups and individual nodes are excluded. Single servers as a target are not offered yet. |
| `disabled_nodes` | object | No | **SPEC 094 D4.** Marks for nodes the user switched off: `{"<identity hash>": <unix time>}`. The key is `config.NodeIdentityHash`: sha256 over the node's emitted outbound JSON without the `tag` and `detour` fields, with keys sorted recursively. A hash was chosen over a tag or a position because providers rename nodes between updates and reorder them freely, so a tag-based mark would silently move to a different server. The hash covers everything that describes the connection (SNI and transport included), so the mark follows the specific node and survives a subscription update, a restart and a rename. The value is the time of the last confirmation: a mark for a node absent from the subscription for longer than the TTL `clamp(3 × update interval, 24h, 30d)` is dropped, otherwise the map would grow without bound. GC runs **only after a successful network update** — on a cache run the body may be incomplete. |

On the wizard's first tab (**Sources**) the **Edit** button on a source opens a window with the sub-tabs **Settings** (prefix, the fold checkbox), **Group** (visible only while the fold is on: which of the three layouts to fold into, plus the auto-group settings), **Preview** (the list of local `proxies[i].outbounds` and of the subscription's nodes) and **JSON** (read-only: the whole `proxies[i]` object).

Four checkboxes (`Local auto`, `Local select`, `Exclude from global`, `Expose tags`) used to sit on **Settings**; SPEC 108 replaced them with the single fold checkbox. Only one of their eight combinations was meaningful, and expressing it took three checkboxes at once. The old flags are still read and are expanded into `fold` when the state loads.

#### The wizard's local groups (`WIZARD:` in `comment`) and global generation

In `proxies[i].outbounds` the wizard may create entries with these substrings in the **`comment`** field:

- **`WIZARD:auto`** — a local urltest (its tag is usually `trim(tag_prefix)+"auto"`).
- **`WIZARD:select`** or **`WIZARD:selector`** — a local selector with `default` pointing at auto and an `addOutbounds` containing the auto tag.

The **`exclude_from_global`** and **`expose_group_tags_to_global`** fields are independent. **`expose`** only considers outbounds carrying those markers in `comment` with the **`expose_group_tags_to_global`** flag enabled on the same `proxies[]` element.

Since SPEC 108 the launcher itself creates such entries only through `fold`, at generation time and on a copy of the config — the state holds no groups. This is why renaming `tag_prefix` renames the groups automatically: they are rebuilt from the current prefix on every build, rather than being patched in place.

#### Tag prefixes, postfixes and masks (version 4)

The `tag_prefix`, `tag_postfix` and `tag_mask` fields let you rewrite the tags of nodes from a particular source automatically. This is useful for:

- Grouping nodes by source within the tags
- Simplifying selector filtering
- Avoiding tag conflicts between different sources
- Replacing the tag format entirely through `tag_mask`

**Automatic prefixes:**
When using the configuration wizard, if a subscription has no `tag_prefix` yet (a new source, or nothing was saved in the config), the order is:
1. **The URL fragment** — if the subscription link has a part after `#` (for example `https://host/list.json#abvpn`), the wizard derives `tag_prefix` from that fragment: surrounding whitespace and control characters are stripped, percent-decoding is applied when needed; if the string does not end with `:`, one is appended (as with the numeric prefixes `1:`).
2. Otherwise — **an ordinal** in the `"1:"`, `"2:"`, `"3:"` form (numbered across all sources: subscriptions first, then the connections block).

If a `tag_prefix` for this URL was already in the saved `ParserConfig`, it is **restored** and replaced by neither the fragment nor an ordinal.

**Order of application:**
1. The node is parsed with its original tag (for example `"🇷🇺 Moscow"`)
2. If `tag_mask` is set, it replaces the tag entirely with variables substituted (steps 3–4 are skipped)
3. If `tag_mask` is not set:
   - `tag_prefix` is applied (when set) with variables substituted.
   - `tag_postfix` is applied (when set) with variables substituted.
4. The tag is checked for uniqueness (via `MakeTagUnique`), which appends a `-N` suffix on duplicates

**Supported variables:**

These variables can be used in `tag_prefix`, `tag_postfix` and `tag_mask`:

| Variable | Description | Example value |
|------------|----------|-----------------|
| `{$tag}` | The node's original tag | `"🇷🇺 Moscow"` |
| `{$scheme}` or `{$protocol}` | The node's protocol | `"vless"`, `"vmess"`, `"trojan"`, `"ss"`, `"hysteria2"` |
| `{$server}` | The server address | `"example.com"`, `"192.168.1.1"` |
| `{$port}` | The server port (a number) | `"443"`, `"8080"` |
| `{$label}` | The label from the URL (the fragment after `#`) | `"United States, New York"` |
| `{$comment}` | The node's comment | `"United States, New York"` |
| `{$num}` | The node's ordinal (starting at 1) | `"1"`, `"2"`, `"3"` |

**Examples:**

The automatic format (added by the wizard when there are several subscriptions):
```json
{
  "source": "https://example.com/subscription1",
  "tag_prefix": "1:"
},
{
  "source": "https://example.com/subscription2",
  "tag_prefix": "2:"
}
```

A manual format:
```json
{
  "source": "https://example.com/subscription",
  "tag_prefix": "source1-",
  "tag_postfix": "--xx"
}
```

Using variables:
```json
{
  "connections": [
    "vless://uuid@server.com:443#🇷🇺 Moscow",
    "vmess://...",
    "hysteria2://password@server.com:443#🇺🇸 New York"
  ],
  "tag_prefix": "{$num} {$protocol}:"
}
```

The result:
- For the first node (vless): `"1 vless:🇷🇺 Moscow"`
- For the second node (vmess): `"2 vmess:..."`  
- For the third node (hysteria2): `"3 hysteria2:🇺🇸 New York"`

More examples with variables:
```json
{
  "tag_prefix": "[{$protocol}] {$server}:{$port} - ",
  "tag_postfix": " ({$label})"
}
```

If a node had the tag `"🇷🇺 Moscow"`, the server `"example.com"`, port `443` and protocol `"vless"`, the resulting tag would be:
- `"[vless] example.com:443 - 🇷🇺 Moscow (United States, Moscow)"`

**Using tag_mask:**

`tag_mask` replaces the node tag entirely, ignoring `tag_prefix` and `tag_postfix`:

```json
{
  "connections": [
    "vless://uuid@server.com:443#🇷🇺 Moscow",
    "vmess://...",
    "hysteria2://password@server.com:443#🇺🇸 New York"
  ],
  "tag_mask": "{$num} {$protocol} : {$label}"
}
```

The result:
- For the first node (vless): `"1 vless : 🇷🇺 Moscow"`
- For the second node (vmess): `"2 vmess : ..."`  
- For the third node (hysteria2): `"3 hysteria2 : 🇺🇸 New York"`

**Important:** when `tag_mask` is set, `tag_prefix` and `tag_postfix` are ignored completely.

#### Supported filter keys

- `tag` — the tag name (case- and emoji-sensitive)
- `host` — the node's hostname
- `label` — the original string after `#` in the URI
- `scheme` — the protocol scheme (`vless`, `vmess`, `trojan`, `ss`)
- `fragment` — the URI fragment (same as `label`)
- `comment` — the right-hand part of `label` after `|`

#### The `pattern` format in filters

- `"literal"` — a case-sensitive substring match
- `"!literal"` — negation (exclude nodes with such a value)
- `"/regex/i"` — a regular expression with the `i` flag (case-insensitive)
- `"!/regex/i"` — a negated regular expression

**Examples:**
```json
"skip": [
  { "tag": "!/🇷🇺/i" },           // Exclude every node whose tag contains 🇷🇺
  { "host": "/test\\./i" },        // Exclude nodes whose host contains "test."
  { "scheme": "trojan" },          // Exclude every Trojan node
  { "label": "/Netherlands/i" }   // Exclude nodes whose label contains "Netherlands"
]
```

### The `outbounds` section

An array of objects describing selectors (proxy groups).

| Field              | Type      | Required | Description |
|-------------------|----------|--------------|----------|
| `tag`             | string   | Yes           | The selector name. Used in the Clash API tab UI for switching proxies. |
| `type`            | string   | Yes           | The selector type: `"selector"` (manual choice) or `"urltest"` (automatic best-node choice). |
| `options`         | object   | No          | Extra fields, added as top-level keys of the result. |
| `filters`         | object   | No          | The main filter for picking nodes (version 4). OR between objects in an array, AND between keys inside one object. In version 2 this was called `outbounds.proxies`. |
| `addOutbounds`    | array    | No          | Strings prepended to the resulting outbound list (for example `"direct-out"`). In version 2 this was called `outbounds.addOutbounds`. |
| `preferredDefault`| object   | No          | A filter that picks the default node. The first node matching it becomes the selector's `default` value. In version 2 this was called `outbounds.preferredDefault`. |
| `comment`         | string   | No          | A comment printed above the selector's JSON in the resulting file. |
| `required`        | bool     | No          | **Template-only.** `true` marks the tag as mandatory: the wizard will not let you delete such an outbound outright (Del is blocked, Edit and Reset still work), and a missing one is added from the template. See [Config Wizard behaviour](#config-wizard-behaviour). |
| `ref`             | string   | No          | Preset binding (SPEC 057/058): `""` (a plain outbound), `"#TEMPLATE#"` or `<preset_id>`. |
| `updates`         | array    | No          | A patch stack (SPEC 057/058): preset patches in rule order, plus an optional USER patch — always last. |
| `label`           | string   | No          | **SPEC 104.** The Direction's display name; empty means "show the tag". Kept apart from `comment` on purpose: a template entry's comment describes its purpose in a paragraph and does not work as a name. |
| `disabled`        | bool     | No          | **SPEC 104.** The Direction keeps its settings but is neither built nor offered as a rule target. `disabled` rather than `enabled` so a zero value means "on". |
| `auto`            | object   | No          | **SPEC 104.** Parameters of the paired `<tag>-auto` group: `mode` (`least_test` \| `round_robin`), `url`, `interval`, `tolerance`, `idle_timeout`, `interrupt_exist_connections`, and `pool` / `pool_tolerance` / `sticky_hash` for round-robin. Absent means no twin at all. The twin itself is never stored — it is expanded on every build. |
| `chain`           | object   | No          | **SPEC 110.** Turns the Direction into a hop chain instead of a selector: `hops` (positions in packet order), `idle_timeout`, `strip_evasion`, `strip`, `rewrite`. Present means the entry has no composition, no filter and no auto-select at all. Requires a core built with `with_lx_chain`. See [Hop chains](#hop-chains). |
| `wizard`          | object   | No          | **Legacy.** The old wrapper `{"hide": true, "required": 1}`; its `required` is still read as a fallback (the numeric form included). In the current format `required` sits **flat**, without the wrapper. The numeric semantics of "`1` — add, `2` — always rewrite" do not exist in the code. |

Since SPEC 104 these entries are **Directions** — the targets rules point at —
and the wizard edits them with a form on the *Directions* tab rather than by
hand. The tab's shared JSON editor is gone; raw JSON stayed inside a single
Direction's window, where it is still the escape hatch for anything the form
does not cover (extra `filters` keys, unusual `options`).

The form shows a filter as the **body** of a regular expression plus an invert
tick, and always writes the canonical `/body/i`; matching ignores case because
subscription tags arrive in whatever case the provider chose. See
[DIRECTION_FILTERS.md](DIRECTION_FILTERS.md).

#### Hop chains

**SPEC 110.** A Direction whose `type` is `"chain"` describes a multi-hop route
`client → hop 1 → hop 2 → … → target` and is emitted as sing-box-lx's `chain`
outbound. It is still an ordinary Direction from the outside: rules point at
it, selectors include it, urltest measures it.

```json
{
  "tag": "double-hop",
  "type": "chain",
  "chain": {
    "hops": ["vpn-1", "🇳🇱 Amsterdam"],
    "idle_timeout": "10m",
    "strip_evasion": true,
    "strip": { "tls.utls": false }
  }
}
```

**`hops` are in packet order**: the first entry is the hop closest to you, the
last is the address the destination sees. `detour` reads the other way round
("who dials through whom"), so mixing the two up builds a route that works but
is not the one you meant. Any position may be a node, a subscription group,
another Direction or a template's service tag; switching a group on any
position changes the path without a restart.

The core rejects the whole config — not just the chain — when any of its
invariants break, so the launcher checks them before emitting: at least two
positions, none empty, no self-reference, no duplicates, and a nested chain
only at position 0. A position that did not make it into the config removes
the **entire** chain rather than one hop: a route missing a hop is a different
route.

`strip` removes one-way DPI-evasion tricks from the links (positions from the
second one on), which the server never sees inside a tunnel and which only add
latency. The catalogue is closed — an unknown key is a startup error:

| Key | Stripped by default | What it removes |
|---|---|---|
| `tls.fragment` | yes | ClientHello fragmentation |
| `multiplex.padding` | yes | multiplex padding |
| `xhttp.padding` | yes | XHTTP padding |
| `tls.utls` | **no** | ClientHello fingerprint — **must not** be stripped on `reality` nodes, which need it |

`rewrite` is an RFC 7396 merge patch applied to link options per outbound type;
it is edited on the JSON tab only.

**Cores without `with_lx_chain`** do not know the type and reject the entire
config with `unknown outbound type: chain`. The launcher probes the installed
core's build tags first: an unsupported chain is not emitted at all, rules
pointing at it fall back to `route.final`, and the reason is reported in the
log and in the entry's editor.

#### Filtering logic in `filters`

The `filters` filter works as follows:

1. **AND logic inside one object**: every key in the object must match
   ```json
   "filters": {
     "tag": "/🇳🇱/i",      // AND the tag must contain 🇳🇱
     "host": "/example/i"  // AND the host must contain "example"
   }
   ```

2. **OR logic between objects** (when `filters` is an array):
   ```json
   "filters": [
     { "tag": "/🇳🇱/i" },   // OR the tag contains 🇳🇱
     { "tag": "/🇺🇸/i" }    // OR the tag contains 🇺🇸
   ]
   ```

3. **When `filters` is absent**: every node enters the selector (except those excluded via `skip`)

#### `filters` usage examples

```json
// Exclude nodes carrying 🇷🇺 or 🇺🇸
"filters": {
  "tag": "!/(🇷🇺|🇺🇸)/i"
}

// Include only nodes carrying 🇳🇱
"filters": {
  "tag": "/🇳🇱/i"
}

// Include nodes carrying 🇳🇱 AND a host containing "example"
"filters": {
  "tag": "/🇳🇱/i",
  "host": "/example/i"
}

// Include nodes carrying 🇳🇱 OR 🇺🇸 (when filters is an array)
"filters": [
  { "tag": "/🇳🇱/i" },
  { "tag": "/🇺🇸/i" }
]
```

### The `parser` section

Parser settings (optional, filled in automatically).

| Field          | Type      | Required | Description |
|---------------|----------|--------------|----------|
| `reload`      | string   | No          | The auto-update interval. Defaults to `"4h"`. Format: `"1h"`, `"30m"`, `"24h"` and so on. |
| `last_updated`| string   | No          | The time of the last update in RFC3339 (UTC). Updated automatically on every configuration refresh. |

## The configuration update process

When you press **"Update Config"** on the "Core" tab (or use the Config Wizard):

1. **Loading the configuration**
   - Settings are read from **`state.json`** (`loadParserConfigForUpdate` in `core/config_service.go`); if it is missing, or has no `proxies`, the update stops with a hint to open the wizard and save the subscriptions first
   - The ParserConfig block version is determined
   - ⚠️ The `@ParserConfig` block in `config.json` is **not** read — it was removed in SPEC 045

2. **Fetching subscriptions**
   - For every URL in `proxies[].source`:
     - The subscription body is downloaded (Base64 and plain text are both supported)
     - It is decoded and the proxy-server list is parsed
   - For every direct link in `proxies[].connections`:
     - The link is parsed (any scheme from the «Supported protocols» table) and added to the proxy list

3. **Supported protocols** (the full matrix is in the table at the top of this document)
   - ✅ **VLESS** (`vless://`)
   - ✅ **VMess** (`vmess://`)
   - ✅ **Trojan** (`trojan://`)
   - ✅ **Shadowsocks / SS** (`ss://` — SIP002 + legacy)
   - ✅ **Hysteria2** (`hysteria2://` and the short form `hy2://`)
   - ✅ **SSH** (`ssh://` — a singbox-launcher URI dialect)
   - ✅ **SOCKS5** (`socks5://`, `socks://` — outbound type `socks`, version=5)
   - ✅ **NaïveProxy** (`naive+https://`, `naive+quic://` — sing-box ≥ 1.13.0 + the `with_naive_outbound` build tag; fork core `1.14.0-lx.4+`)
   - ✅ **WireGuard / AmneziaWG** (`wireguard://`, `awg://` — the `endpoints[]` section; sing-box ≥ 1.11, AWG needs `with_awg`)
   - ✅ **TUIC** (`tuic://` — TUIC v5)
   - ✅ **Amnezia** (`vpn://` — a `.vpn` profile, converted into `wireguard://`)
   - ✅ **AnyTLS** (`anytls://`)
   - ✅ **MASQUE** (`masque://` — CONNECT-IP / WARP; fork core `1.14.0-lx.26+`)

4. **Extracting information**
   - From every URI the parser takes:
     - **The tag**: the left part of the comment before `|` (for example `🇳🇱Netherlands`)
     - **The comment**: all the text after `#` in the URI
     - **Connection parameters**: server, port, UUID, TLS settings and so on

5. **Filtering nodes**
   - The `skip` filters from `proxies[]` are applied — matching nodes are excluded
   - The `filters` from `outbounds[]` are applied — nodes are picked for each selector
   - Nodes with duplicate tags are renamed automatically (a `-2`, `-3` … suffix is appended)

6. **Generating node JSON**
   - VLESS / VMess / Trojan / SS / Hysteria2 / SSH / SOCKS5 / **NaïveProxy** / TUIC / AnyTLS / **MASQUE** nodes are serialized into `outbounds[]`; WireGuard nodes go into `endpoints[]` (sing-box ≥ 1.11)
   - Comments are derived from `label`
   - Field order is optimized for readability

7. **Generating selectors**
   - Selectors are created according to `outbounds[]`
   - Comments come from the `comment` field
   - Field order is fixed: `tag`, `type`, `outbounds`, `default`, `interrupt_exist_connections`, then the rest
   - `addOutbounds` entries are prepended to the `outbounds` list
   - `preferredDefault` determines the `default` field's value

8. **Writing the result**
   - The block between the `/** @ParserSTART */` and `/** @ParserEND */` markers is replaced with the new content (outbounds)
   - The block between `/** @ParserSTART_E */` and `/** @ParserEND_E */` is replaced with the generated endpoints (WireGuard), when those markers are present in the config
   - The `last_updated` field in the `parser` section is refreshed
   - Everything happens in a single pass (one file read, one file write)

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

| Parameter | Meaning |
|---|---|
| `mode` | `auto` \| `packet-up` \| `stream-up` \| `stream-one`. On the fork `auto` = `packet-up`; `stream-one` has a known downlink-framing bug |
| `path` | The request path |
| `host` | The Host header; when empty, the SNI from TLS is substituted |
| `x_padding_bytes` (`xPaddingBytes`) | A `"min-max"` range, default `100-1000`; carried in the `Referer` header |
| `no_grpc_header` | Drop the gRPC-compatible header |
| `session_placement`, `session_key` | Placement and key name for the session |
| `seq_placement`, `seq_key` | Placement and key name for the sequence |
| `uplink_data_placement`, `uplink_data_key` | Placement and key name for the uplink data |
| `uplink_chunk_size`, `uplink_http_method` | The uplink's chunk size and HTTP method |
| `x_padding_key`, `x_padding_header`, `x_padding_placement`, `x_padding_method` | Fine tuning of the x-padding obfuscation |
| `sc_max_each_post_bytes` (`scMaxEachPostBytes`) | The core expects a `"min-max"` string; a bare number (including `30.0` coming from `extra`) is normalized into a string |
| `sc_min_posts_interval_ms` (`scMinPostsIntervalMs`) | The same rule |

Values are not validated further — the core parses them. Implementation: `xhttpTransportFromQuery` in `core/config/subscription/node_parser_transport.go`; specs: `SPECS/071-F-N-XHTTP_TRANSPORT/SPEC.md`, `sing-box-lx` SPEC 002.

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
- `obfs` — the obfuscation type (currently only `salamander` is supported)
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

## The marker section in `config.json`

The parser rewrites the block between `/** @ParserSTART */` and `/** @ParserEND */`. An example of the result:

```
/** @ParserSTART */
    // 🇳🇱Netherlands
    {"tag":"🇳🇱Netherlands","type":"vless","server":"...","port":443,...},

    // Proxy group for international connections
    {"tag":"proxy-out","type":"selector","outbounds":["direct-out","auto-proxy-out","🇳🇱Netherlands",...],"default":"🇳🇱Netherlands","interrupt_exist_connections":true},
/** @ParserEND */
```

Every line ends with a comma so that additional objects (`direct-out`, `reject` and so on) can be placed after the block.

## Config Wizard behaviour

### Where the wizard reads its settings from

The canonical store is **`state.json`**; the template (`bin/wizard_template.json`) is only needed for a fresh install.

1. **`state.json`** — when it exists, the wizard shows what is in it: subscriptions (`connections.sources[]`), shared selectors (`connections.outbounds[]`) and the rest of the user's settings. It is read through `state.Load` in the presenter.
2. **The template** — the fallback when there is no `state.json` yet (a first run). `LoadConfigFromFile` (`ui/configurator/business/loader.go`) reads **only** the template.
3. ⚠️ **`config.json` is not part of this chain.** Before SPEC 045 the wizard extracted `@ParserConfig` from `config.json`; both that path and the block itself are gone.

### Mandatory outbounds (`required`)

In the template an outbound can be marked `"required": true` — a tag the wizard will not let you delete outright (the Del button is blocked; Edit and Reset still work). The set of such tags is collected by `TemplateData.RequiredOutboundTags()` (`core/template/loader.go`).

- `required: true` — the tag is mandatory; a missing one is added from the template.
- the field absent / `false` — an ordinary outbound the wizard leaves alone.
- **Legacy:** older templates carry the wrapper `"wizard": {"required": 1}` — its `required` is still read as a fallback (the numeric form included), but in the current format `required` sits **flat** on the outbound, without the `wizard` wrapper.

> ⚠️ The numeric semantics of "`required: 1` — add, `required: 2` — always rewrite from the template" do **not** exist in the code: `required` is a boolean flag. If you find that in older documentation, it does not match the behaviour.

### Preset binding (`ref` / `updates`)

Besides `required`, an outbound may carry template/preset binding fields (SPEC 057/058):

- `ref` — the source: `""` (an ordinary outbound), `"#TEMPLATE#"` or `<preset_id>`;
- `updates` — a patch stack: preset patches in rule order, plus an optional USER patch (always last).

The mechanics in detail: [`WIZARD_STATE.md`](WIZARD_STATE.md) §3.2 and §5.

## Notes and tips

- **Stop sing-box before updating**: the Clash API may react to an intermediate file
- **Flag normalization**: if a subscription carries odd flags, `normalizeFlagTag` in `core/config/subscription/node_parser_core.go` can be extended
