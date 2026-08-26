# singbox-launcher subscription parser reference

**🌐 Language**: English | [Русский](ParserConfig.ru.md)

> **Where this lives (schema v6).** Parser settings live in **`state.json`**, not in `config.json`. Subscriptions are in `connections.sources[]`, shared selectors in `connections.outbounds[]` (see [`WIZARD_STATE.md`](WIZARD_STATE.md)); the per-source `tag_prefix`, `tag_postfix`, `tag_mask`, `skip`, `outbounds`, `disabled` live on the Source.
>
> ⚠️ **The `/** @ParserConfig ... */` block in `config.json` is gone** — removed in SPEC 045, `state.json` is now the single (canonical) store. Into `config.json` the parser writes only the nodes themselves, between the `@ParserSTART`/`@ParserEND` markers (and `@ParserSTART_E`/`@ParserEND_E` for endpoints).
>
> The document below describes the **field format and the parsing logic** — it applies to `connections.sources[]` / `connections.outbounds[]` 1:1.

## Purpose

The parser updates `bin/config.json` by loading subscriptions (see the [«Supported protocols»](Protocols.md#supported-protocols) table below — 14 protocols: VLESS, VMess, Trojan, Shadowsocks, Hysteria2, SSH, SOCKS5, NaïveProxy, WireGuard/AmneziaWG, TUIC, Amnezia (`vpn://`), MASQUE, AnyTLS, HTTP(S) CONNECT proxy), filtering them and grouping them into selectors. The result is written between the `/** @ParserSTART */` and `/** @ParserEND */` markers (outbounds); WireGuard nodes go between `/** @ParserSTART_E */` and `/** @ParserEND_E */` (endpoints). The **endpoints** section (WireGuard) is supported by sing-box from version **1.11** on.

### Supported protocols

The launcher understands **14 URI schemes** — VLESS, VMess, Trojan, Shadowsocks,
Hysteria2, SSH, SOCKS5, NaiveProxy, WireGuard/AmneziaWG, TUIC, Amnezia
(`vpn://`), MASQUE, AnyTLS, HTTP(S) CONNECT proxies — plus bare WireGuard
`.conf` text and JSON arrays of Xray/V2Ray configs.

📄 **The table of schemes, the parameters of each protocol and the reverse
share-URI generation live in a separate document,
[`Protocols.md`](Protocols.md).** It also covers the xhttp transport, the
AmneziaWG fields, the node degradation codes and the list of what is **not**
supported (ShadowTLS, Mieru, Hysteria 1, SSR, Tor).

The document below describes **configuring the parser**: sources, filters,
directions (`outbounds`), marker sections and the wizard.


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
      // TUIC, SSH, SOCKS5, NaïveProxy, WireGuard/AWG, Amnezia, MASQUE, AnyTLS, HTTP(S) CONNECT proxy.
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
| `source`      | string   | Yes           | The subscription URL. All 14 protocols from the [«Supported protocols»](Protocols.md#supported-protocols) table: VLESS, VMess, Trojan, Shadowsocks, Hysteria2, SSH, SOCKS5, NaïveProxy, WireGuard/AmneziaWG, TUIC, Amnezia (`vpn://`), MASQUE, AnyTLS, HTTP(S) CONNECT proxy. Base64 and plain text are both accepted, as is a **JSON array** of full Xray configs (`[ {...}, ... ]`), see above. |
| `connections` | array    | No          | An array of direct links. All 13 schemes from the [«Supported protocols»](Protocols.md#supported-protocols) table: `vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://`/`hy2://`, `tuic://`, `ssh://`, `socks5://`/`socks://`, `naive+https://`/`naive+quic://`, `wireguard://`/`awg://`, `vpn://` (Amnezia), `masque://`, `anytls://`. Can be combined with subscriptions. WireGuard nodes land in the config's `endpoints` section (sing-box ≥ 1.11). NaïveProxy requires sing-box ≥ 1.13.0 + the `with_naive_outbound` build tag (fork core `1.14.0-lx.4+`). More in [URI formats for direct links](#uri-formats-for-direct-links). |
| `skip`        | array    | No          | A list of filters. If at least one matches, the node is skipped. |
| `chain`       | object   | No          | **SPEC 110.** Makes this source a hop chain instead of a subscription or a server: `hops` (positions in packet order), `idle_timeout`, `strip_evasion`, `strip`, `rewrite`. Such a source has no `source` and no `connections` — it materializes into a single `chain` outbound whose tag comes from `tag_mask`. Requires a core built with `with_lx_chain`. See [Hop chains](#hop-chains). |
| `tag_prefix`  | string   | No          | A prefix added to every node tag from this source (version 4). Applied before the original tag. Supports the variables `{$tag}`, `{$scheme}`, `{$protocol}`, `{$server}`, `{$port}`, `{$label}`, `{$comment}`, `{$num}`. Ignored when `tag_mask` is set. |
| `tag_postfix` | string   | No          | A postfix added to every node tag from this source (version 4). Applied after the original tag. Supports the same variables as `tag_prefix`. Ignored when `tag_mask` is set. |
| `tag_mask`    | string   | No          | A mask that replaces the node tag entirely (version 4). When set, it replaces the tag completely, ignoring `tag_prefix` and `tag_postfix`. Supports the same variables as `tag_prefix`/`tag_postfix`. |
| `outbounds`   | array    | No          | Local outbounds for this source (version 4). Applied only to nodes from this source. **As of SPEC 108 they are no longer offered as rule targets** — a subscription's group is a grouping and sugar for a direction, not a target that rules should hold onto: it disappears with the subscription and is renamed with its prefix. The launcher stops writing marked groups here entirely (see `fold`); hand-written entries still generate. |
| `fold` | object | No | **SPEC 108** — folds the subscription into a single group: `{"mode": "select" \| "auto" \| "select_auto", "auto": {…}}`. Absent means not folded. At generation time (pass 0, `config.PrepareSourceFolds`) it materialises the groups `<trim(tag_prefix)>auto` and/or `<trim(tag_prefix)>select` into `proxies[i].outbounds` and sets both flags below itself, so the subscription arrives in the directions as **one** entry instead of all its nodes. `auto` carries the auto-group settings in the same shape as a direction's `outbounds[].auto`. Under `select_auto` the auto group is the selector's first option and its `default`, and it is **not** exposed as a separate direction candidate — otherwise the user would be offered both the subscription and its own auto-select. |
| `exclude_from_global` | bool | No | When `true`, this source's nodes do **not** enter the pool for the **global** `ParserConfig.outbounds` entries during config generation. Local `proxies[i].outbounds` still use this source's nodes only. The field is `omitempty`; it affects generator behaviour only and does not change the global JSON. A `fold` sets it itself; the wizard no longer exposes it. |
| `expose_group_tags_to_global` | bool | No | Set by a `fold` at generation time; the wizard no longer exposes it. When `true`, during generation the tags of the wizard-**marked** local groups (see below) are **added** to the effective outbound list of **every** global `ParserConfig.outbounds` entry. The stored `outbounds[].addOutbounds` array is **not** rewritten. Strings from a user's own `addOutbounds` are still **not** filtered through `filters`; the injected tags do pass the same `filters` as nodes (a synthetic match on `tag`/`comment`). |
| `detour_tag` | string | No | **SPEC 077.** The tag of another outbound through which **all** of this source's nodes are dialled (a proxy chain / hop): the node gets `"detour":"<tag>"` (`A through B`). Empty means a direct connection. Stored by tag (like rules and selectors). Not applied to WireGuard nodes, nor to nodes with their own Xray chain (`dialerProxy` wins). **Fail-open** at generation time: a self-reference and a cycle among nodes (`A.detour=B`, `B.detour=A`) are detected and broken with a warning (the node then works directly) — otherwise the core would reject the config outright. A dangling detour onto a **template/preset group** tag is not dropped (group tags are only known at final assembly). **The UI choice is deliberately narrowed** (`business.DetourOptions`): only **manual Directions** (from the Directions tab) and **active preset groups** are offered; built-in/utility ones (`direct-out`/`reject`/`drop`), paired auto groups (`<tag>-auto`), the subscription's own local groups and individual nodes are excluded. Single servers as a target are not offered yet. |
| `disabled_nodes` | object | No | **SPEC 094 D4, key per SPEC 112.** Marks for nodes the user switched off: `{"<identity>": <unix time>}`. The key is `config.NodeIdentity`: the node's raw provider tag, uniquified within THIS source (first `X`, next `X-2`), taken **before** `tag_prefix` / `tag_postfix` / `tag_mask` are applied. Node content (server, port, credentials, SNI, transport, mtu) is not part of the identity: the tag is the name the provider manages the node by, and rotating the server behind that name is the SAME node. So editing the source's tag policy does not drop the marks, and a provider-side IP rotation does not reset them. The flip side: a provider renaming the node loses the mark (it lives out its TTL and expires) — but it never silently moves to a different server. Before SPEC 112 the key was a sha256 over the emitted JSON; keys of that shape (64 hex chars) are rewritten to tags on the first parse of the source. The value is the time of the last confirmation: a mark for a node absent from the subscription for longer than the TTL `clamp(3 × update interval, 24h, 30d)` is dropped, otherwise the map would grow without bound. GC runs **only after a successful network update** — on a cache run the body may be incomplete. |

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

#### Hop chains

**SPEC 110.** A hop chain describes a multi-hop route `client → hop 1 →
hop 2 → … → target`. It is a **source**, not a Direction — it describes a
*route*, while a Direction is the point where you *choose between* routes.
It lives in `proxies[]` next to subscriptions and servers:

```json
{
  "tag_mask": "double-hop",
  "chain": {
    "hops": ["vpn-1", "🇳🇱 Amsterdam"],
    "idle_timeout": "10m",
    "strip_evasion": true,
    "strip": { "tls.utls": false }
  }
}
```

For the rest of the launcher a chain is a **node**: it joins the pool, is
picked up by Direction filters like any subscription server, and shows up in
the Clash API as a switchable option. That is the point of it being a
source — as a Direction it could neither be selected inside `proxy-out` nor
measured by an auto-select group.

**`hops` are in packet order**: the first entry is the hop closest to you,
the last is the address the destination sees. `detour` reads the other way
round ("who dials through whom"), so mixing the two up builds a route that
works but is not the one you meant. Any position may be a node, a
subscription group, a Direction, a template service tag or another chain;
switching a group on any position changes the path without a restart.

The core rejects the whole config — not just the chain — when any of its
invariants break, so the launcher checks them before emitting: at least two
positions, none empty, no self-reference, no duplicates. A position that did
not make it into the config removes the **entire** chain rather than one
hop: a route missing a hop is a different route.

Two kinds of cycle are ruled out:

- **chain → chain**: a chain may only reference one declared *above* it in
  `proxies[]`, so forward references are rejected — cycles are impossible by
  construction;
- **chain → group → chain**: a Direction does not take into its members a
  chain that runs *through that same Direction* (checked transitively). The
  most common setup hits this immediately — a chain `[proxy-out, exit]`
  while `proxy-out` filters "all nodes" would otherwise contain the very
  chain that dials through it.

`strip` removes one-way DPI-evasion tricks from the links (positions from
the second one on), which the server never sees inside a tunnel and which
only add latency. The catalogue is closed — an unknown key is a startup
error:

| Key | Stripped by default | What it removes |
|---|---|---|
| `tls.fragment` | yes | ClientHello fragmentation |
| `multiplex.padding` | yes | multiplex padding |
| `xhttp.padding` | yes | XHTTP padding |
| `tls.utls` | **no** | ClientHello fingerprint — **must not** be stripped on `reality` nodes, which need it |

Note that `sing-box check` does **not** catch the `tls.utls` + `reality`
clash: the check passes and `run` fails at startup. The launcher's form
refuses that combination for exactly this reason.

`rewrite` is an RFC 7396 merge patch applied to link options per outbound
type; it is edited on the JSON tab only.

**Cores without `with_lx_chain`** do not know the type and reject the entire
config with `unknown outbound type: chain`. The launcher probes the
installed core's build tags first: an unsupported chain never becomes a
node, so it is simply absent from the pool — exactly like a disabled source
— and the reason is reported in the log and in the source's row.

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

The parser accepts direct links in the `connections` array — the format depends
on the protocol.

📄 The parameters of each scheme now live in [`Protocols.md`](Protocols.md):
[VLESS](Protocols.md#vless-vless), [VMess](Protocols.md#vmess-vmess),
[Trojan](Protocols.md#trojan-trojan), [Shadowsocks](Protocols.md#shadowsocks-ss),
[Hysteria2](Protocols.md#hysteria2-hysteria2-or-hy2), [SSH](Protocols.md#ssh-ssh),
[SOCKS5](Protocols.md#socks5-socks5-or-socks), [NaiveProxy](Protocols.md#naïveproxy-naivehttps--naivequic),
[TUIC](Protocols.md#tuic-tuic), [AnyTLS](Protocols.md#anytls-anytls),
[MASQUE](Protocols.md#masque-masque), [WireGuard](Protocols.md#wireguard-wireguard),
[Amnezia](Protocols.md#amnezia-vpn), [`.conf` text](Protocols.md#raw-conf-text-interfacepeer).


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
