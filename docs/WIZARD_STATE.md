# Wizard state (state.json)

**🌐 Language**: English | [Русский](WIZARD_STATE.ru.md)

The Configurator's declarative model: where it lives, how it is loaded, how it is
saved, where it goes at build time.

> **Schema v7 (SPEC 118) is what the launcher writes today.** The per-section
> descriptions below still document **v6** and are kept because v6 files are
> read and migrated forever. The shape changed as follows; the sections marked
> v6 should be read with this in mind:
>
> - `meta.version` is `7`, `meta.schema` is `"sources_v7"`;
> - sources moved to the top level: `sources[]` instead of `connections.sources[]`,
>   and Directions to `directions[]` instead of `connections.direction_outbounds[]`;
> - a source is a flat union with a `kind` discriminator — `server`, `chain`,
>   `auto`, `folder`, `subscription`;
> - a container's nodes are stored explicitly in `nodes[]`, each with its own
>   `tag`, `enabled`, `origin` and ready-to-emit `body`. Subscription bodies are
>   no longer re-parsed at build time, and the raw-response cache
>   (`bin/subscriptions/*.raw`) is gone;
> - per-node on/off lives on the node (`enabled`) instead of a separate
>   `disabled_nodes` map of hashed keys; links (`detour`, chain hops, group
>   members) are `NodeLink` objects — `{folder_id, tag}`;
> - a subscription carries its own request identity: `user_agent`, `hwid`,
>   `send_hwid`, `hash_device_model`, plus `relays_in_directions`.
>
> v6 files migrate automatically on first launch; the previous file is kept
> next to the new one as `state.json.v6.bak`.

---

## 1. Files and layout

**The local machine** (the historical flat layout, unchanged):

- **`bin/wizard_states/state.json`** — the current snapshot. The only file read
  when the Configurator starts and during a headless config.json rebuild.
- **`bin/wizard_states/<id>.json`** — named snapshots (Save As). Structurally
  identical to `state.json`; on Read they are copied over `state.json`.
- **`bin/subscriptions/<source_id>.raw`** — the per-source raw subscription body
  cache (atomic .tmp + rename). The read path parses `.raw` directly, no network.
- **`bin/rule-sets/<tag>.srs`** — downloaded rule-sets.

**A remote machine** (SPEC 098): everything it owns lives under one directory,
where `<id>` is the ID of its entry in the `bin/remote-daemons.json` registry:

- **`bin/wizard_states/remote/<id>/state.json`** — its current snapshot;
- **`bin/wizard_states/remote/<id>/<snap>.json`** — its named snapshots;
- **`bin/wizard_states/remote/<id>/config.json`** — its built config (this is the
  file Deploy in its row ships);
- **`bin/wizard_states/remote/<id>/srs/`**, **`…/subscriptions/`** — its `.srs`
  files and subscription bodies.

Machines share no files: the GC of live tags and Source.IDs is computed from
**that** machine's states and deletes only inside its own directories. Removing a
machine deletes its state directory plus `bin/remote-daemons/<id>/` (the client
keys).

Paths are resolved only through `internal/platform` (`GetWizardStatePathFor`,
`GetRemoteConfigPathFor`, `GetRuleSetsDirFor`, `GetSubscriptionsDirFor`) — do not
assemble them from string literals.

ExecDir resolution is described in SPECS/022 (macOS app support directories). In a
macOS release build that is `~/Library/Application Support/singbox-launcher/bin/...`;
in a dev build it sits next to the binary.

---

## 2. Top-level schema v6 (canonical)

```jsonc
{
  "meta": {
    "version": 6,
    "schema":  "presets_v1",
    "comment": "...",
    "created_at": "RFC3339 UTC",
    "updated_at": "RFC3339 UTC"
  },

  "connections": {
    "sources":   [ ... ],     // per-source subscription / server entries
    "direction_outbounds": [ ... ],  // SPEC 104: Directions — the targets rules point at
                              // (older states keep them under "outbounds"; read forever, never written)
    "defaults":  { "reload": "4h", "max_nodes": 3000 }
  },

  "rules": [
    { "kind": "preset", "ref": "...",  "enabled": true, "body": { "vars": {} } },
    { "kind": "inline", "id":  "...",  "enabled": true, "body": { "name": "...", "match": {}, "outbound": "..." } },
    { "kind": "srs",    "id":  "...",  "enabled": true, "body": { "name": "...", "srs_url": "...", "outbound": "..." } }
  ],

  "vars": [
    { "name": "tun",          "value": "true" },
    { "name": "dns_strategy", "value": "prefer_ipv4" },
    ...
  ],

  "dns_options": {
    "strategy":                "...",   // optional fallback; source of truth — vars[]
    "final":                   "...",
    "default_domain_resolver": "...",
    "servers": [
      { "kind": "template", "tag": "...",        "enabled": true  },
      { "kind": "preset",   "ref": "<pid>:<tag>", "enabled": true },
      { "kind": "user",     "tag": "...", "type": "...", "server": "...", "enabled": true, ... }
    ],
    "rules": [
      { "kind": "preset", "ref": "<pid>", "enabled": true },
      { "kind": "user",   "enabled": true, ... }
    ]
  }
}
```

Top-level keys absent from v6 (compared with earlier revisions):
`id` (the snapshot name lives in the filename), `config_params`, `custom_rules`,
`selectable_rule_states`, `rules_library_merged`, `dns_options.independent_cache`,
`foreign_backup_extensions` (the LX Backup `extensions` carry-through mechanism
was abolished with contract 0.11.0 — see `contract/docs/BACKUP_PRINCIPLES.md`
П3: state after an import is indistinguishable from state configured by hand,
so there is nothing left to store verbatim).

---

## 3. Per-section schemas in detail

### 3.1 `connections.sources[i]`

The `type` discriminator: `subscription` (a URL → a batch of nodes) or `server` (one URI → one outbound).

**Order.** Entries are stored in `sources[]` array order — the same order they appear in on the Sources tab. The ↑/↓ buttons in a row reorder the slice elements (the schema doesn't change and `id` is preserved); the new order goes into `state.json` on the next Save.

| Field | Type | When | Description |
|------|-----|-------|----------|
| `id` | string | always | A ULID (Crockford base32, 26 chars). Stable — survives Save/Load. The filename is `bin/subscriptions/<id>.raw`. |
| `type` | string | always | `subscription` \| `server` \| `chain` (SPEC 110). |
| `enabled` | bool | always | The source is active. Disabled → its outbounds never reach the final config. |
| `label` | string | opt. | Display name (effectively required for a server; for a subscription it falls back to `meta.profile_title`). |
| `exclude_from_global` | bool | opt. | Keep this source's nodes out of the directions pool. **Read-only as of SPEC 108:** a fold sets it itself at build time and the UI no longer exposes it. The field survives for states where nodes bypassed the shared list with no groups at all — a fold cannot express that. |
| `url` | string | subscription | The subscription URL. |
| `skip` | `[]map[string]string` | subscription | Skip rules (names of nodes not to parse). |
| `tag` | `{prefix, postfix, mask}` | subscription | Node tag transformation (`BL:` prefixes and the like). `mask` overrides prefix+postfix. |
| `outbounds` | `[]OutboundConfig` | subscription | Former per-source groups. **No longer written as of SPEC 108:** a folded subscription's groups are expanded at build time from `fold`, and entries carrying the old `WIZARD:*` markers are dropped on load. |
| `expose_group_tags_to_global` | bool | subscription | The former SPEC 026 flag. **Read-only as of SPEC 108:** it is expanded into `fold` on load and never written back. |
| `fold` | `{mode, auto}` | subscription | **SPEC 108** — folds the subscription into a single group. Absent means not folded: its nodes enter the directions individually. `mode`: `select` \| `auto` \| `select_auto`. `auto` holds the auto-group settings, in the same canonical shape as a direction's `outbounds[].auto`. The groups themselves (`<prefix>auto`, `<prefix>select`) are NOT stored: they are expanded on every build, so renaming the subscription's prefix renames them automatically. |
| `chain` | `{hops, …}` | chain | **SPEC 110** — a hop chain: `hops` (positions in PACKET order), `idle_timeout`, `strip_evasion`, `strip`, `rewrite`. Only for `type: chain`; such a source has neither `url` nor `uri`. It materializes into ONE outbound of type `chain` whose tag comes from `label`. A chain describes a ROUTE while a Direction is the choice *between* routes, hence a source rather than a Direction; it enters Directions as a node, like any subscription server. |
| `update` | `{interval_hours, auto_refresh}` | subscription | Per-source override of the default reload interval. |
| `max_nodes` | int | subscription | Per-source override `defaults.max_nodes`. |
| `meta` | `SubscriptionMeta` | subscription | Runtime data (see below), filled in by Update. |
| `uri` | string | server | vless:// / vmess:// / wireguard:// / etc. — a single server. |

**JSON example — subscription source:**
```jsonc
{
  "id": "01KQCTRQBSSF0CCYFD2WWTVY9R",
  "type": "subscription",
  "enabled": true,
  "url": "https://example.com/sub.txt",
  "tag": { "prefix": "BL:" },
  // Folded into a selector with an auto-group: the directions list gets one
  // BL:select entry whose default is BL:auto. Both groups are created at build
  // time — the state holds neither.
  "fold": {
    "mode": "select_auto",
    "auto": { "interval": "5m", "tolerance": 100 }
  },
  "update": { "interval_hours": 4, "auto_refresh": true },
  "max_nodes": 3000,
  "meta": {
    "profile_title": "My VPN Pack",
    "url_at_fetch": "https://example.com/sub.txt",
    "last_fetched_at": "2026-05-24T13:56:25Z",
    "last_status": "ok",
    "http_status_code": 200,
    "raw_body_bytes": 46318,
    "nodes_count_fetched": 148,
    "userinfo": { "upload_bytes": 0, "download_bytes": 1024000, "total_bytes": 107374182400, "expire_unix": 1735689600 },
    "preview_nodes": [ "vless://...", "ss://...", "..." ]
  }
}
```

**JSON example — server source:**
```jsonc
{
  "id": "01KQCXYZ...",
  "type": "server",
  "enabled": true,
  "label": "My direct server",
  "uri": "vless://uuid@host:443?type=tcp&security=reality&pbk=...#MyServer"
}
```

**Drilldown — the `meta` field (subscription runtime data):**

| Field | Description |
|------|----------|
| `profile_title` | From the `subscription-profile-title` header or an inline `#profile_title:` on the body's first line. |
| `profile_update_interval_hours`, `support_url`, `profile_web_page_url`, `content_disposition_filename` | Headers (response + inline body). |
| `userinfo` | `{upload_bytes, download_bytes, total_bytes, expire_unix}` — the parsed `subscription-userinfo` header (V2Board/Xboard). |
| `url_at_fetch`, `last_fetched_at`, `last_status`, `error_count`, `last_error_msg`, `http_status_code`, `raw_body_bytes` | Fetch history. |
| `nodes_count_fetched`, `truncated`, `preview_nodes` | The parse result. `truncated` means it was cut off at `max_nodes`. |

**Hop chains (`type: chain`, SPEC 110).** `hops` lists positions in PACKET
order: the first is the hop closest to you, the last is the address the
destination sees. `detour` reads the other way round ("who dials through
whom"), and mixing them up builds a route that works but is not the one you
meant. A position may be a node, a subscription group, a Direction, a
template service tag or another chain.

A chain becomes a node AFTER every source is loaded: its positions reference
tags that are only final by then (subscription prefixes, duplicate
uniquification). For the rest of the launcher it is an ordinary node — it
joins the pool, is caught by Direction filters, and switches in the Clash
API.

The core rejects the whole config when a chain breaks its rules, so they are
checked before emitting: at least two positions, none empty, no
self-reference, no duplicates. A position not found among nodes and
Directions removes the **entire** chain: a route missing a hop is a
different route.

Cycles are ruled out two different ways: a chain may only reference a chain
declared ABOVE it in the source list (forward references are rejected), and
a Direction does not take into its members a chain that runs through that
same Direction — checked transitively.

`strip` removes one-way DPI-evasion tricks from links; the catalogue is
closed (`tls.fragment`, `multiplex.padding`, `xhttp.padding`, `tls.utls`)
and an unknown key is a startup error. `tls.utls` is not stripped by default
and must not be stripped on `reality` nodes: `sing-box check` does not catch
this — the config passes the check and fails at startup.

A core built without `with_lx_chain` does not know the type. The launcher
probes the build tags first: an unsupported chain never becomes a node — it
is simply absent from the pool, like a disabled source — and the reason
lands in the log and in the source's row.

---

### 3.2 `connections.outbounds[i]` — `OutboundConfig`

A top-level entry in the global outbounds. **SPEC 058-R-N** splits entries into two
classes by storage shape:

- **Direct** — a self-contained body lives entirely in state. The `ref` field is
  absent (`omitempty`). These are user outbounds that reference nothing — full
  ownership: Edit writes the body directly, with no USER patch.
- **Referenced** — the body lives elsewhere (in the template or a preset), and the
  state holds only `tag + ref + updates[]`. Body fields (`type`, `options`,
  `filters`, `addOutbounds`, …) are **not written** — all of them are `omitempty`.
  The sync function strips the body on every pass to maintain the thin-shape
  invariant.

For referenced entries `ref` takes one of:
- `#TEMPLATE#` (the `configtypes.RefTemplate` constant) — the body lives in
  `template.parser_config.outbounds[tag]`
- `<preset_id>` — the body lives in `template.presets[id].outbounds[mode=add]`

A USER edit on top of a referenced entry is stored as a **field-level diff** in
`updates[]` with `ref="#USER#"` (the `configtypes.RefUser` constant) — it is a
patch over the reference, not a separate class of entry. One per outbound, always
last in `updates[]`, replace-not-append on every Save.

#### Sentinel constants

```go
// core/config/configtypes/types.go
const (
    RefTemplate = "#TEMPLATE#" // only in state.outbounds[].ref
    RefUser     = "#USER#"     // only in state.outbounds[].updates[].ref
)
```

Any other non-empty `ref` is interpreted as a preset ID. Validation is
positional:

| Position | Allowed | Rejected |
|---|---|---|
| `outbounds[].ref` (entry level) | `""` / `#TEMPLATE#` / `<preset_id>` | `#USER#` (patch level), junk |
| `outbounds[].updates[].ref` (patch level) | `#USER#` / `<preset_id>` | `""`, `#TEMPLATE#` |

The `^[a-z0-9_-]+$` regex for Preset.id cannot by construction overlap with the
UPPERCASE+`#` constants — a collision is impossible. `sanitizeOutboundRefs` in
`core/state/load.go` drops entries with an invalid `ref` at load time.

#### Schema

| Field | Type | Description |
|------|-----|----------|
| `tag` | string | Display tag, unique within the global outbounds. |
| `type` | string (**omitempty**) | The sing-box type. Direct entries only — empty for referenced ones (the body lives in the template/preset). |
| `options` | `map[string]interface{}` (omitempty) | sing-box options. Direct entries / USER patch only. |
| `filters` | `map[string]interface{}` (omitempty) | UI/build only: regex filters over node tags. |
| `addOutbounds` | `[]string` (omitempty) | UI/build only: unioned with the nodes matching the filter. |
| `preferredDefault` | `map[string]interface{}` (omitempty) | UI/build only: default metadata. |
| `comment` | string (omitempty) | UI/build only: the `// <comment>\n` comment prefix. |
| `required` | bool (omitempty) | **SPEC 058.** A top-level flag (formerly inside `wizard.required`). A template-level marker — the UI locks Delete. The template is the source of truth; it reaches state through the legacy `wizard.required` migration. |
| `ref` | string (omitempty) | **SPEC 058.** `""` (direct) / `#TEMPLATE#` (referenced template) / `<preset_id>` (referenced preset add). |
| `updates` | `[]OutboundUpdate` (omitempty) | The patch stack: preset patches in rule order + an optional USER patch (always last). |

**Removed by SPEC 058:** the `wizard interface{}` field (which used to hold the
`{hide}` / `{required}` map). `required` became a separate top-level `bool`;
`hide` is not used in the state shape (template only).

**`OutboundUpdate{ref, patch}`** — a single entry in `updates[]`:

| Field | Type | Description |
|------|-----|----------|
| `ref` | string | `<preset_id>` (a preset patch) or `#USER#` (the user diff). |
| `patch` | `map[string]interface{}` | The fields to merge (filters / options / addOutbounds / preferredDefault / comment). `tag` and `type` are immutable and never in a patch. |

Merge semantics (`core/build/resolve_outbounds.go::applyOutboundUpdatePatch`
→ `core/build/preset_outbounds.go::applyOutboundUpdate`; diff —
`core/build/outbound_diff.go::OutboundFieldDiff`):
- `filters` — a full replace when present in the patch
- `options.*` — per-key replace (not a deep merge)
- `addOutbounds` — unioned with the base (`unionStringList`), only when the patch
  is non-empty
- `preferredDefault`, `comment` — replace

**The `required` pseudo-field vs the real one:**
- In the **template** it is the source of truth, read live on every UI render.
- In **state** it is now persisted (see the table above), but it is populated from
  the template during migration/sync. A change in the template is reflected
  correctly in the UI on the next load.

#### JSON examples — direct vs referenced

```jsonc
// 1. (direct) Self-contained — fields inline, no ref.
{
  "tag": "myProxy",
  "type": "selector",
  "options": { "default": "direct-out" },
  "addOutbounds": ["direct-out"]
}

// 2. (referenced, template) Purely template-derived; the user changed nothing.
// At render time the body comes from template.parser_config.outbounds["auto-proxy-out"].
{ "tag": "auto-proxy-out", "ref": "#TEMPLATE#" }

// 3. (referenced, template) Template-derived with preset patches + a USER patch.
// The final body = template + updates[] applied in order (USER always last).
{
  "tag": "proxy-out",
  "ref": "#TEMPLATE#",
  "updates": [
    { "ref": "russian",   "patch": { "filters": { "tag": "!/(🇷🇺)/i" } } },
    { "ref": "ru-inside", "patch": { "filters": { "tag": "!/(🇷🇺)/i" }, "addOutbounds": ["ru-inside-out"] } },
    { "ref": "#USER#",    "patch": { "comment": "my custom" } }
  ]
}

// 4. (referenced, preset) A preset add — the body lives in template.presets["russian"].outbounds[mode=add].
// Disabling the "russian" preset → Sync removes the entry. No USER patch.
{ "tag": "ru VPN 🇷🇺", "ref": "russian" }

// 5. (referenced, preset) A preset add with a USER patch — the user opened Edit and changed addOutbounds.
{
  "tag": "ru VPN 🇷🇺",
  "ref": "russian",
  "updates": [
    { "ref": "#USER#", "patch": { "addOutbounds": ["direct-out", "myProxy"] } }
  ]
}
```

### 3.3 `connections.defaults`

| Field | Type | Default | Description |
|------|-----|---------|----------|
| `reload` | string | `"4h"` | The default subscription reload interval (per-source override via `Source.Update.IntervalHours`). |
| `max_nodes` | int | `3000` | The default node cap per subscription (per-source override via `Source.MaxNodes`). |

**JSON example:**
```jsonc
{ "reload": "4h", "max_nodes": 3000 }
```

### 3.4 `rules[i]` — `v6.Rule` (SPEC 053)

The `kind` discriminator: `preset` / `inline` / `srs`. A single ordered array; the order is the emission order in `config.json::route.rules[]`.

| Field | Type | When | Description |
|------|-----|-------|----------|
| `kind` | string | always | Discriminator. |
| `ref` | string | `kind=preset` | A reference to `template.presets[].id`. |
| `id` | string | `kind=inline` \| `srs` | ULID. |
| `enabled` | bool | always | The common toggle. |
| `body` | raw JSON | always | Kind-specific payload, decoded via `DecodeBody`. |

**Body schemas:**

| Kind | Body shape |
|------|------------|
| `preset` | `{ vars: { <name>: <value>, ... } }` — **the diff only** against the template defaults. An empty map means everything is default. Bump the template → the user automatically gets the new defaults for vars they never touched. |
| `inline` | `{ name: string, match: { <sing-box match keys> }, outbound: string }` — outbound is a tag or a reserved literal (`reject` / `drop`). |
| `srs` | `{ name: string, srs_url: string, outbound: string }` — the URL of an .srs file + an outbound tag/literal. |

**JSON examples — the three kinds:**
```jsonc
// 1. A preset-ref rule (all the semantics live in template.presets["russian"])
{
  "kind": "preset",
  "ref": "russian",
  "enabled": true,
  "body": { "vars": { "out": "proxy-out" } }  // only the overridden vars
}

// 2. Inline user rule
{
  "kind": "inline",
  "id": "01KQD5XYZ...",
  "enabled": true,
  "body": {
    "name": "BitTorrent direct",
    "match": { "protocol": "bittorrent" },
    "outbound": "direct-out"
  }
}

// 3. SRS rule-set user rule
{
  "kind": "srs",
  "id": "01KQD7ABC...",
  "enabled": true,
  "body": {
    "name": "Block ads (oisd)",
    "srs_url": "https://example.com/oisd.srs",
    "outbound": "reject"
  }
}
```

### 3.5 `vars[i]`

A simple KV list — overrides for every var declared by the template.

| Field | Type | Description |
|------|-----|----------|
| `name` | string | The name from `template.vars[].name`. |
| `value` | string | The user override (always a string; typing lives on the template side via `vars[].type`). |

Typical keys (defined by the template):
- `tun` (bool-as-string: `"true"`/`"false"`) — the TUN mode toggle
- `route_final` — the final-outbound choice on the Rules tab
- `dns_strategy`, `dns_final`, `dns_default_domain_resolver` — DNS scalars
- `clash_secret` — an auto-generated bearer for the Debug API
- anything else the template defines

Entries without a `name` (template `{separator: true}`) do NOT reach state. Orphans
(names absent from the current template) are NOT loaded into the model and are NOT
written back.

**JSON example:**
```jsonc
[
  { "name": "tun", "value": "true" },
  { "name": "route_final", "value": "proxy-out" },
  { "name": "dns_strategy", "value": "prefer_ipv4" },
  { "name": "dns_final", "value": "cloudflare_udp" },
  { "name": "dns_default_domain_resolver", "value": "cloudflare_udp" },
  { "name": "clash_secret", "value": "<auto-generated bearer>" }
]
```

### 3.6 `dns_options`

| Field | Type | Description |
|------|-----|----------|
| `strategy` | string (omitempty) | An in-memory fallback duplicate of `vars["dns_strategy"]`. `vars` is the source of truth. Not written to disk when zero. |
| `final` | string (omitempty) | The same, duplicating `vars["dns_final"]`. |
| `default_domain_resolver` | string (omitempty) | The same, duplicating `vars["dns_default_domain_resolver"]`. |
| `servers` | `[]DNSServer` | Entries with a `kind` discriminator (see below). |
| `rules` | `[]DNSRule` | Entries with a `kind` discriminator. |

**`servers[i]` — `v6.DNSServer` (SPEC 056-R-N):**

| Field | Type | Description |
|------|-----|----------|
| `kind` | `DNSServerKind` | `template` \| `preset` \| `user`. |
| `tag` | string | For `kind=template` (the lookup key in `template.dns_options.servers[tag]`) and `kind=user` (the display tag in the final `config.dns.servers[].tag`). Empty for `preset`. |
| `ref` | string | For `kind=preset` only, shaped `"<preset_id>:<local_tag>"`. Empty otherwise. |
| `enabled` | bool | Toggle. The build pipeline skips the entry when `false`. |
| `body` | `map[string]interface{}` | For `kind=user` only — the full DNS server fields (type / server / server_port / tls / detour / ...). nil for `template` / `preset` (the body resolves from the template). |

**`rules[i]` — `v6.DNSRule` (SPEC 056-R-N):**

| Field | Type | Description |
|------|-----|----------|
| `kind` | `DNSRuleKind` | `preset` \| `user`. |
| `ref` | string | For `kind=preset` only, shaped `"<preset_id>"` (one dns_rule per preset). |
| `enabled` | bool | Toggle. |
| `body` | `map[string]interface{}` | For `kind=user` only — the full sing-box dns rule body (rule_set / server / domain_* / ip_cidr / port / network / ...). nil for preset. |

**Template entries: two shapes (SPEC 109).** `template.dns_options.servers[]`
accepts an entry in two forms — flat (ours) and nested
`{description, enabled, vars[], server{}}` (LxBox). The nested one is expanded
into the flat one when the template loads (`template.NormalizeDNSOptions`), so
in `state.json` and everywhere downstream an entry is always flat.

The `vars[]` an entry declares become template variables named
`dns_<tag>_<var>`, sharing one namespace with every other `@placeholder`. The
prefix is required: without it `outbound` from `google_dot` would overwrite
`outbound` from `cloudflare_dot`. They are hidden from the Settings tab
(`wizard_ui: hidden`) — their place is the server's own window, otherwise the
settings list would grow by two dozen indistinguishable "Outbound" rows.

Such a variable's value is stored in the state's `vars[]` like any other
setting; the server body stays in the template and is never copied into state.

**A group's members are pruned at build time.** A member missing from the final
server list — switched off by the user, or declared by an inactive preset — is
dropped from the group, and a group left with no members is not emitted at all.
A reference to a tag that was not emitted brings down the WHOLE config
(`dependency[x] not found for server[group]`), so "leave it and hope" is not an
option; neither is "enable the missing ones for them" — traffic would then go
through servers they never picked.

**Removed:**
- `independent_cache` — deprecated in sing-box 1.14.0 (the cache is always per-transport). A legacy state carrying this key still parses (the unknown field is ignored); new saves don't write it.
- `extra_servers[]`, `extra_rules[]`, the `template_servers` map — the old SPEC 053 dev schema, replaced by a flat list with a kind discriminator (SPEC 056-R-N).

**JSON example — a complete `dns_options` block:**
```jsonc
{
  // strategy/final/default_domain_resolver — a fallback duplicate; vars[] is the source of truth
  // (written to disk only when non-zero)

  "servers": [
    // template: a reference to template.dns_options.servers[tag="cloudflare_udp"]
    { "kind": "template", "tag": "cloudflare_udp", "enabled": true },

    // template: an entry required by the template; disabled locally by the user
    { "kind": "template", "tag": "google_doh", "enabled": false },

    // preset: bundled by the russian preset, local_tag="yandex_doh"
    { "kind": "preset", "ref": "russian:yandex_doh", "enabled": true },

    // user: a fully user-defined DNS server with an inline body
    {
      "kind": "user",
      "tag": "my-pihole",
      "enabled": true,
      "body": { "type": "udp", "server": "192.168.1.10", "server_port": 53 }
    }
  ],

  "rules": [
    // preset: one dns_rule per preset; the body resolves from the template
    { "kind": "preset", "ref": "russian", "enabled": true },

    // user: a full sing-box dns rule body
    {
      "kind": "user",
      "enabled": true,
      "body": {
        "rule_set": "ru-domains",
        "server": "yandex_doh"
      }
    }
  ]
}
```

---

### 3.7 `connections.direction_outbounds[i]` — Directions (SPEC 104)

A **Direction** is a named routing target rules point at. Rules cannot point
at a subscription node directly: node tags are regenerated on every refresh,
so such a rule would fall apart on its own. On build a Direction materializes
into a `selector` plus, when auto-select is on, a paired `urltest` named
`<tag>-auto`.

| Field | Type | Meaning |
|-------|------|---------|
| `tag` | string | The identifier rules reference. Immutable once created — renaming would break every rule pointing at it. Auto-issued ones look like `vpn-1`; template and preset ones are arbitrary (`proxy-out`, `ru VPN 🇷🇺`). |
| `label` | string | Display name. Empty means "show the tag". Free to change: nothing references a name. |
| `disabled` | bool | Not built, not offered as a rule target. **`disabled`, not `enabled`** — a bool's zero value has to mean "on", or an entry written without the key would read as switched off. |
| `type` | string | `selector` for a Direction; `urltest` for the template's standalone auto groups (`auto-proxy-out`) |
| `filters` | object | Node filter in the shared pattern language (`/re/i`, `!/re/i`). The form only ever shows the **body** and an invert tick; the `i` flag is always written. |
| `preferredDefault` | object | Same language; the first matching node becomes the selector's `default`. |
| `addOutbounds` | array | Extra options: `direct-out`, `block-out`, and Directions **above** in the list. Never a `<tag>-auto` twin — that is an option only inside its own Direction. |
| `auto` | object \| null | Twin parameters: `mode` (`least_test` \| `round_robin`), `url`, `interval`, `tolerance`, `idle_timeout`, `interrupt_exist_connections`, plus `pool` / `pool_tolerance` / `sticky_hash` for round-robin. **null means no twin at all.** |
| `options`, `comment`, `required`, `ref`, `updates` | | As before (SPEC 057/058): template/preset binding and the patch stack. |

**The twin is not stored.** `<tag>-auto` is expanded on every build from
`auto`. Keeping it in state would mean two objects a user has to keep in sync
by hand.

**The old key is read forever.** State written before SPEC 104 keeps its
Directions under `connections.outbounds`; it is adopted on load and never
written back. When both keys are present the canonical one wins — otherwise
state touched by an older version after a newer one would glue two sets
together with duplicate tags.

Shape: `core/config/configtypes/types.go`. Materialization:
`core/config/direction_twins.go` + the three-pass generator. Filter helpers:
`core/config/configtypes/direction_filter.go`.

---

## 4. Per-block storage rules

| Section | Contains | Source of truth | Who writes | Who reads |
|--------|----------|-----------------|-----------|------------|
| `connections.sources` | Source entries (a subscription URL or a server URI), per-source meta (profile_title, userinfo, last_status), the update spec | state | The UI Sources tab (`source_tab`), the Update flow (after a fetch) | the parser pipeline, the UI dashboard, build |
| `connections.direction_outbounds` | Directions — selector/urltest entries, including preset-bound (`ref`) and preset-patched (`updates[]`) ones. The pre-SPEC-104 key `connections.outbounds` is still read. | state | The UI Directions tab, `SyncOutboundsWithTemplate`, the presenter's `CreateStateFromModel` | build (`MergeOutboundUpdatesInPlace`; UI preview — `MergeOutboundUpdates`), UI render |
| `connections.defaults` | The reload interval and the per-source max_nodes default | state | The UI Settings/Sources tabs | the parser pipeline |
| `rules` | Routing rules behind a kind discriminator (preset/inline/srs) — a single ordered array | state | The UI Rules tab (drag, library add, edit) | build (`MergeRouteSection` + `MergePresetsIntoRoute`), UI render |
| `vars` | Overrides for every var the template declares: tun, route_final, dns_*, clash_secret, etc. | state (the values) + template (the declarations) | The UI Settings tab, the hidden synchronizers (`SyncDNSModelToSettingsVars`) | build (`@var` substitution) |
| `dns_options.servers` | Entries of kind=template / preset / user; for template/preset the body resolves from the template, for user it is flat in the entry | state (what is enabled) + template (the body) | The UI DNS tab, `SyncDNSOptionsWithActivePresets`, the presenter | build (`ResolveDNS` → `MergeDNSSection`), UI render |
| `dns_options.rules` | Entries of kind=preset / user. preset is a thin ref to `template.presets[].dns_rule`, user is a flat body | state + template | The UI DNS tab, the lifecycle sync, the presenter | build (`ResolveDNS`), UI render |

"Source of truth" means where an entry's semantics come from. "Who writes" means
the places in the code that mutate state. "Who reads" means the consumers at
build/render time.

---

## 5. Outbound preset/template binding lifecycle (SPEC 057-R-N + SPEC 058-R-N)

Outbound entries in `connections.outbounds[]` exist in one of two shapes (see
§3.2): **direct** (body inline, empty ref) or **referenced** (external body; the
state holds only `tag + ref + updates[]`). SPEC 057 introduced preset binding via
`ref=<preset_id>`; SPEC 058 extended the same model to template entries via
`ref=#TEMPLATE#` and formalized the USER edit as a field-level diff in `updates[]`
with `ref=#USER#`.

### 5.1 Schema (see §3.2 for the full table)

What matters for the lifecycle:
- `ref=""` → direct; the lifecycle is manual (Edit / Delete — full ownership)
- `ref=#TEMPLATE#` → referenced template; the lifecycle runs through "Restore missing" / drop on a missing tag
- `ref=<preset_id>` → referenced preset add; the lifecycle runs through the preset toggle on the Rules tab
- `updates[].ref=<preset_id>` → a preset mode=update patch; lifecycle via the preset toggle
- `updates[].ref=#USER#` → the USER diff: one per outbound, replaced on every Save, always last

### 5.2 Lifecycle: `SyncOutboundsWithActivePresets`

The single place where referenced entries are added and removed. Idempotent.

Called from:
- On Load after `parseV6` (the presenter's `LoadState`)
- On every preset toggle in the Rules tab (via `RefreshAfterPresetToggle`)
- Before Save in `CreateStateFromModel` — over **both views**
  (`state.Connections.Outbounds` and `state.ParserConfig.ParserConfig.Outbounds`),
  because `syncConnectionsFromLegacy` copies the legacy view → canonical, so
  otherwise the synced `updates[]` would be clobbered by the adapter
- On headless runtime paths: `rebuild_raw_cache`,
  `UpdateConfigFromSubscriptions`, `parseAndPreview`

The semantics (see `core/build/sync_outbounds.go`):

1. Walk `state.outbounds`; for each entry:
   - Drop preset entries (`ref=<preset_id>`) when the owning preset is disabled/missing
   - Strip the body (`type`/`options`/`filters`/`addOutbounds`/`preferredDefault`/`comment`)
     from every referenced entry — the **thin-shape invariant**
   - Leave direct entries (`ref=""`) alone
   - Drop preset patches from disabled presets out of `updates[]`; keep the USER patch
2. Append missing preset add entries as thin `{tag, ref: preset.id}` (the body lives in the preset)
3. Append the expected preset update patches (`mode=update`) into the target's `updates[]`
4. **Re-order `updates[]`:** preset patches in rule order (by `activeRulesOrder`),
   stale preset patches (preset disabled but the patch is still there) after the
   ordered ones, and the USER patch always last. Implemented by `reorderUpdates`
   in the same file.
5. **Adopt-on-first-sync:** a direct entry whose tag matches an expected preset add
   is converted into a referenced preset (`ref=preset.id`, body stripped).

Template entries (`ref=#TEMPLATE#`) are **not dropped** by the sync function on a
missing tag — that is handled by the resolver fallback at render/build time (a
silent drop). Adding template entries happens separately, through the UI's
"Restore missing" button (not automatically during sync, so the user controls
which template tags live in their state).

### 5.3 Migration legacy state — `MigrateOutboundsToReferencedShape` (SPEC 058)

A one-shot on the first load after the upgrade: SPEC 057 stored template-derived
entries with an empty `ref` and a snapshotted body — the migration converts them
into the referenced shape. See `core/build/migrate_outbounds_spec058.go`.

For each direct entry (`ref=""`):

- If `tag` matches `template.parser_config.outbounds[tag]`:
  1. `merged_base` = the template body + the active preset `mode=update` patches
     for that tag (this matters: those patches were ALREADY materialized into the
     legacy body by `ApplyPresetOutboundsToParserConfig`, so the diff must be taken
     against them, not against the bare template — otherwise preset patches get
     attributed as USER edits)
  2. `diff = OutboundFieldDiff(ob, merged_base)`
  3. Set `ob.ref = "#TEMPLATE#"`, upsert the USER patch with `diff` (when non-empty), strip the body
- Otherwise, if `tag` matches a preset add — set `ref=<preset_id>` + diff against
  the preset body → USER patch + strip the body
- Otherwise → a genuine direct entry, left as-is

**Idempotent:** running it again on an already-migrated state is a no-op (the loop
skips entries that are already referenced).

**Backup:** `state.json.pre-058.bak` on the first save after the migration, via
`maybeBackupSPEC058` in `core/state/save.go` (mirroring SPEC 053's `.v5.bak`).
Lossless rollback is guaranteed.

### 5.4 Runtime merge: `MergeOutboundUpdatesInPlace`

The native generator (`GenerateOutboundsFromParserConfig`) knows nothing about
`Updates` and `Ref`. Before calling it, the build pipeline runs
`MergeOutboundUpdatesInPlace(parserCfg)`, which walks `parserCfg.Outbounds[]` and
for each entry:
- **Referenced (`ref != ""`):** looks up the base body in the template/preset,
  applies the `updates[]` stack (preset patches + USER) in order, and writes the
  merged result into the entry
- **Direct (`ref == ""`):** applies `updates[]` (if any) to the inline body

It mutates in place (via a deep copy at the call edge, so the model is not
trashed). The UI preview flow keeps the unmerged form (for the model save) apart
from the merged one (`parserConfigForGen` — the generator gets a flattened copy).

---

## 6. DNS preset binding lifecycle (SPEC 056-R-N)

Symmetrical to the outbound binding. `dns_options.servers[]` and
`dns_options.rules[]` are flat arrays with a `kind` discriminator.

### 6.1 `dns_options.servers[]` — kind

| `kind` | Identity | Body |
|--------|----------|------|
| `template` | `tag` (a reference into `template.dns_options.servers[tag]`) | resolved from the template at build/render time |
| `preset` | `ref = "<preset_id>:<local_tag>"` (a reference to `template.presets[id].dns_servers[local_tag]` + `vars` substitution) | resolved from the template with the preset vars applied |
| `user` | `tag` + a flat body (type/server/server_port/tls/...) — a complete sing-box DNS server spec | self-contained |

The `enabled` toggle is available for all three kinds; editing the body is
user-only; deleting is user-only as well (template/preset entries are governed by
the template and the preset toggle).

### 6.2 `dns_options.rules[]` — kind

| `kind` | Identity | Body |
|--------|----------|------|
| `preset` | `ref = "<preset_id>"` (at most one dns_rule per preset) | resolved from `template.presets[id].dns_rule` |
| `user` | flat body (rule_set/server/domain_*/ip_cidr/port/network/...) | self-contained |

### 6.3 Lifecycle: `SyncDNSOptionsWithActivePresets`

The single lifecycle point for kind=preset entries, mirroring the outbound sync.

Called from the presenter on the same triggers: Load, preset toggle, before Save.
The semantics: enabling a preset creates `{kind:preset, ref}` entries for every
`template.presets[id].dns_servers[]` plus its `dns_rule` (when present), defaulting
to `Enabled=true`. Disabling a preset removes every entry referencing it. A
per-server toggle inside an active preset (the user may hide one server from a
bundle) is preserved across syncs.

Implementation: `core/state/sync_dns.go::SyncDNSOptionsWithActivePresets`.

### 6.4 Required entries (template)

`template.dns_options.servers[]` may mark an entry `"required": true` (for
example `local_dns_resolver` / `direct_dns_resolver`). The render shows it ticked
and locks toggle/edit/delete; the build always emits it. The flag is template-only
and is never persisted in state — it is read live on every render via
`wizardbusiness.DNSTagLocked(model, tag)`.

### 6.5 Removed fields (sing-box 1.14)

`independent_cache` — deprecated in sing-box 1.14 (the cache is always
per-transport now). A legacy state carrying this key still parses (it is silently
dropped via `_ = raw.IndependentCache` in `legacyDevDNSToOptions`); new saves do
not write the field.

---

## 7. Rule preset binding lifecycle (SPEC 053)

`rules[]` is a single ordered array behind a `kind` discriminator.

| `kind` | Header | Body |
|--------|--------|------|
| `preset` | `{ref, enabled}` (ref = `<preset_id>`) | `{vars: {<name>: <value>, ...}}` — the diff against the template defaults only; an empty map means everything is default |
| `inline` | `{id (ULID), enabled}` | `{name, match (a sing-box match object), outbound (tag|"reject"|"drop")}` |
| `srs` | `{id (ULID), enabled}` | `{name, srs_url, outbound}` |

The order is the render order in the UI Rules tab (drag-reordering included),
which is also the emission order in `config.json::route.rules[]`. It is persisted
by `SyncRulesByOrderToStateRulesV6(model.RuleOrder, ...)` inside
`CreateStateFromModel` (the function name is legacy; the result goes into
`state.Rules`).

The match fields and rule_sets for kind=preset live **in the template** — bump
`RequiredTemplateRef` and users automatically get the new match fields. The body
stores only the diffed vars; an empty `vars: {}` means the preset runs on template
defaults.

See `core/state/rule_types.go` (the DecodeBody dispatcher) +
`core/build/preset_expand.go` (build-time substitution + tag prefixing).

---

## 8. Data flow

### 8.1 Load: `state.json` → model

```
disk: bin/wizard_states/state.json
        │
        ▼
core/state.Load(path)
        │   probe meta.version  →  parseV6 (or parseV5 / parseLegacy)
        │   legacyDevDNSToOptions if the old dev shape `dns.{template_servers,extras}`
        │   sanitizeOutboundRefs (SPEC 058: rejects refs invalid for their position)
        ▼
state.State{Connections, Rules, DNS, Vars, ...}
        │
        ▼
presenter.LoadState(stateFile)
        │   restoreParserConfig (legacy view)
        │   restoreConfigParams + restoreDNS
        │   ApplyRulesLibraryMigration (legacy v3→v5 idempotent)
        │   restoreCustomRules + restorePresetRefs (kind=preset)
        │   MigrateOutboundsToReferencedShape (SPEC 058: direct→referenced + USER patch, idempotent)
        │   SyncOutboundsWithActivePresets(model.GlobalOutbounds)   ← adopt-on-first-sync + strip referenced body
        │   RefreshDerivedParserConfig
        ▼
model.WizardModel  (Sources, GlobalOutbounds, CustomRules, PresetRefs,
                    DNSServers, DNSRulesText, SettingsVars, RuleOrder)
        │
        ▼
SyncModelToGUI + RefreshOutboundOptions
```

### 8.2 Save: model → `state.json`

```
model.WizardModel
        │
        ▼
presenter.CreateStateFromModel(comment, id)
        │   SyncGUIToModel
        │   build WizardStateFile (legacy view + Connections canonical)
        │   ReconcileRuleOrder + SyncRulesByOrderToStateRulesV6  → state.Rules
        │   SyncDNSFullToStateV6                                  → state.DNS
        │   state.SyncDNSOptionsWithActivePresets(state.Rules, &state.DNS, presets)
        │   applyPresetEnabledOverrides (UI toggle → entry.Enabled)
        │   build.SyncOutboundsWithActivePresets ×2 views (Connections + ParserConfig)  ◄── both are mandatory!
        ▼
state.State.Save(path)
        │   maybeBackupSPEC058 (SPEC 058: .pre-058.bak on the first save after the migration)
        │   syncConnectionsFromLegacy (ParserConfig → Connections; the already-synced version wins)
        │   marshalDisk (a single canonical-v6 path since SPEC 060; the dual write is gone)
        │   atomic write (.tmp + Rename) + fsync
        ▼
disk: bin/wizard_states/state.json
```

Syncing both views (`Connections.Outbounds` + `ParserConfig.Outbounds`) is the
crux: without it, `syncConnectionsFromLegacy` would clobber the `updates[]` stacks
that were just computed.

### 8.3 Build/Emit: state → `bin/config.json`

```
state.State (after Load or after CreateStateFromModel)
        │
        ▼
core/build (entry: BuildConfig)
        │   ResolveDNS(state, template, vars)        ◄── pure func
        │   ResolveRoute(state, template, vars)      ◄── pure func
        │   MergeOutboundUpdatesInPlace(parserCfg)   ◄── materializes Updates[] (preset + USER patches) into the body for the generator
        │   GenerateOutboundsFromParserConfig
        │   MergeDNSSection + MergeRouteSection
        │   MergePresetsIntoRoute (per-preset expand: substitute + tag prefix)
        ▼
disk: bin/config.json (sing-box-compatible)
```

The Resolve* functions are the single source of truth for both the UI and the
build (no divergence between the preview and the final config).

---

## 9. Required vs preset-locked entries

Three classes of entry in the UI, with different management semantics:

| Class | Where the marker is | Meaning | UI controls |
|-------|------------|------------|-------------|
| **Required (template)** | `template.*.entries[].required = true`. For outbounds: the top-level `required bool` in state (SPEC 058, populated from the template on load/sync); for DNS it is read live from the template at render time. | A mandatory entry — it cannot be toggled or deleted. The body is editable through a USER patch (referenced) or inline (direct). | Reset (clears the USER patch, reverting to template defaults), Edit. **Delete is not rendered.** |
| **Referenced template** (SPEC 058) | `ref == "#TEMPLATE#"` | The body lives in `template.parser_config.outbounds[tag]`. The user can layer a USER patch on top through Edit (a field-level diff in `updates[]`). | Edit (opens the merged_base view; Save computes the diff), Reset (clears the USER patch — disabled when there is none), Delete (removes the entry — restorable via "Restore missing") |
| **Preset-locked** | `entry.ref` = `<preset_id>` (for outbounds) or `kind=preset` (for DNS/rules) | The entry was created by a preset; the body resolves from the template/preset. | Toggle enabled (the user can hide an individual bundle item), View (a read-only modal) or Edit with a USER patch. **Delete is not rendered** — the lifecycle runs through the preset toggle on the Rules tab. |
| **Direct** | `ref == ""` (the field is absent) and the tag is absent from the template/presets | Full control, a self-contained body. | Toggle, Edit (writes the body directly, no USER patch), Up/Down, Delete |

"Required" is about a **delete lock**; "preset-locked" is about an **edit-body
lock**; "referenced template" is the promoted class (SPEC 058) where the edit goes
through a USER patch over the template body, which is what gives template
auto-upgrade for free.

---

## 10. Migrations

| From → To | What migrates | Backup |
|-----------|---------------|--------|
| v2/v3/v4 → v5 | `selectable_rule_states` + `custom_rules` → a single `custom_rules[]` (rules library merge); wrapped `parser_config` → simplified; `enable_tun_macos` → `vars["tun"]`; `route.default_domain_resolver` → `vars["dns_default_domain_resolver"]` | none (in-memory; v5 is written on the first Save) |
| v5 → v6 | `custom_rules[]` → `rules[]` (kind=inline/srs derived from the rule_set type); legacy `dns_options.servers/rules` → the flat kind-discriminated `dns_options.servers/rules`; meta bump | **`state.json.v5.bak`** on the first upgrade (once at least one kind=preset rule appears) |
| v6 dev shape → v6 flat | `dns.{template_servers, extra_servers, extra_rules}` (the intermediate SPEC 053 shape) → flat `dns_options.servers[]/rules[]` (SPEC 056-R-N) | none (lossless, dev-only, never released) |
| SPEC 057 outbounds → SPEC 058 | Direct entries with a full body whose `tag` matches the template/a preset → thin referenced entries (`ref=#TEMPLATE#` / `ref=<preset_id>`) plus a USER patch holding the field-level diff against merged_base. Idempotent and lossless. Also: the legacy `wizard.required` map → a top-level `required bool`; the `wizard interface{}` field was removed from the struct. | **`state.json.pre-058.bak`** on the first save after the migration |
| sing-box 1.14 | `dns_options.independent_cache` is silently dropped (a legacy state still reads, new ones don't write it) | none |

Save always writes the canonical (v6) shape (SPEC 060 removed the dual write
path). Legacy v5 files are still read by `parseV5Legacy` and normalized into
`State` at load time; the next Save rewrites them in the v6 layout. Users with
purely inline/srs rules stay on v5 until they add their first preset.

---

## 11. Where the implementation lives

| File | What |
|------|-----|
| `core/state/load.go` | `Load` / `Parse` / `parseCurrent` / `parseV5Legacy` / `parseLegacyAndMigrate` / `legacyDevDNSToOptions` + `sanitizeOutboundRefs` (SPEC 058: drops entries whose `ref` is invalid for its position) |
| `core/state/save.go` | `Save` / `marshalDisk` (a single canonical-v6 write path since SPEC 060) / `maybeBackupSPEC058` (SPEC 058: `.pre-058.bak` on the first save after the referenced-shape migration) |
| `core/state/adapter.go` | `syncConnectionsFromLegacy` / `syncLegacyFromConnections` (the legacy ParserConfig ↔ canonical Connections exchange) |
| `core/state/disk_v6.go` | `diskStateV6` (private write-shape) + `MetaSection` + `SchemaVersionV6` |
| `core/state/rule_types.go` | `Rule` + `PresetBody`/`InlineBody`/`SrsBody` + `DecodeBody` |
| `core/state/dns_options.go` | `DNSServer` + `DNSRule` + flat `MarshalJSON`/`UnmarshalJSON` |
| `core/state/sync_dns.go` | `SyncDNSOptionsWithActivePresets` |
| `core/state/migration_v5_to_v6.go` | `migrateV5ToV6` (private helper) + `isV5`/`isV6` detection |
| `core/state/legacy_migration.go` | `migrateV4ToV5` (private) + `IDGenerator` |
| `core/state/legacy_v4.go` | `v4File` (private) + `parseV4File` |
| `core/state/legacy_types.go` | `LegacyDNSOptionsV5` (for the backward-compat parse path and the UI's `PersistedDNSState`) |
| `core/state/connections.go` | `ConnectionsSection`/`Source`/`Defaults`/`TagSpec`/`SubscriptionMeta`/`UserInfo` |
| `core/state/raw_cache.go` | `WriteRawBody`/`ReadRawBody`/`DeleteOrphans` |
| `core/state/ulid.go` | `MakeULID` |
| `core/build/sync_outbounds.go` | `SyncOutboundsWithActivePresets` (lifecycle) + `stripReferencedBody` + `reorderUpdates` + `outboundConfigToPatchMap` |
| `core/build/migrate_outbounds_spec058.go` | **SPEC 058.** `MigrateOutboundsToReferencedShape` — the one-shot direct→referenced conversion + USER patch on the first load |
| `core/build/outbound_diff.go` | **SPEC 058.** `OutboundFieldDiff` (a field-level diff against merged_base) + `UpsertUserPatch` |
| `core/build/resolve_outbounds.go` | `resolveBaseBody` (honours `ref` for the base lookup) + `MergeOutboundUpdates` / `MergeOutboundUpdatesInPlace` (runtime helpers) + `applyUpdatesToBase` + `applyOutboundUpdatePatch` (map patch → `preset_outbounds.go::applyOutboundUpdate`) |
| `core/build/resolve_dns.go` | `ResolveDNS` (the pure DNS view for both UI and build) |
| `core/build/resolve_route.go` | `ResolveRoute` (pure route view) |
| `core/template/loader.go` | `LoadTemplateData` + `TemplateData` struct |
| `core/template/preset_types.go` | Preset / PresetVar / PresetRuleSet / PresetDNSServer / PresetOutbound |
| `ui/configurator/presentation/presenter_state.go` | `LoadState` + `CreateStateFromModel` (the save/load entry points) |
| `ui/configurator/presentation/presenter_sync.go` | `RefreshAfterPresetToggle` (the presenter-level eager sync after a Rules toggle) |

See also: [TEMPLATE_REFERENCE.md](TEMPLATE_REFERENCE.md) — what lives in
`wizard_template.json` and where it ends up in state/runtime/UI.
[DATA_FLOW.md](DATA_FLOW.md) — extended load/save/build/toggle diagrams.
[WIZARD_TEMPLATE.md](WIZARD_TEMPLATE.md) — the template syntax reference
(the preset format, vars, substitution, if/if_or).
