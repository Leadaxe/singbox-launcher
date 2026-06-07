# `wizard_template.json` syntax

Reference for the wizard template format. A template is a single JSON file describing
how the final sing-box `config.json` is assembled from the subscription and the user's
choices.

- **Location:** `<binary_dir>/bin/wizard_template.json`. For a macOS `.app` —
  `Contents/MacOS/bin/wizard_template.json` inside the bundle (not the file from the repo).
- **Engine internals** (walker, build pipeline, stock-JSON format):
  [`TEMPLATE_REFERENCE.md`](TEMPLATE_REFERENCE.md). This file covers only the syntax for the template author.

---

## 1. Top-level structure

```json
{
  "parser_config": { ... },
  "config":        { ... },
  "dns_options":   { ... },
  "vars":     [ ... ],
  "params":   [ ... ],
  "presets":  [ ... ]
}
```

| Key | Type | Purpose |
|---|---|---|
| `parser_config` | object | Subscription parser settings + static outbound groups (selector/urltest). Subscription nodes are injected here. |
| `config` | object | Base sing-box config skeleton (`route`, `experimental`, `inbounds`, …) into which `@var` are substituted and `params` are applied. |
| `dns_options` | object | DNS-tab defaults: `servers[]` + `rules[]`. |
| `vars` | array | Typed template variables → **Settings** rows, sources for `@`-placeholders. |
| `params` | array | Patches to `config` by dot-path with conditions (`if`) and a merge mode. |
| `presets` | array | Presets for the **Rules** tab (rules, rule_set, DNS, outbounds). |

> Deprecated: the `selectable_rules` key was removed (SPEC 053) — rules go only through `presets[]`.

---

## 2. `@var` placeholders

In `config`, `params[].value`, and preset bodies, a string of the form `"@name"` is
replaced with the value of variable `name` from `vars[]` (or a preset var in the preset's scope).

- **Scalar:** `"mtu": "@tun_mtu"` → the value (for an int var — as a number, not a string).
- **List (single-element array):** `["@dns_list"]` where `dns_list` is `text_list` →
  the array expands into a list of strings.
- **Result type** is determined by the var type: `bool` → `true/false`, `text_list` → array,
  numeric vars (`tun_mtu`, `mixed_listen_port`, `proxy_in_listen_port`, `urltest_tolerance`)
  → number, the rest → string.
- The name after `@` contains no spaces; an unknown `@var` in `config` → load error.

`@var` as an object **key** is used only in `#if` predicates (§4); in ordinary JSON keys
are not substituted.

---

## 3. `vars[]` — variables

Each element describes one variable (a row on the **Settings** tab).

| Field | Type | Description |
|---|---|---|
| `name` | string | Identifier (`[A-Za-z_][A-Za-z0-9_]*`). The name `runtime` is reserved. |
| `type` | string | `text` \| `bool` \| `enum` \| `text_list` \| `secret`. |
| `default_value` | see §3.2 | Default value (when absent from state). |
| `default_node` | string | Alternative: dot-path inside the template to take the default from. |
| `options` | array | For `enum`: `["a","b"]` or `[{"title","value"}]`. |
| `wizard_ui` | string | `edit` (default) \| `view` (read-only) \| `hidden` \| `fix` (edited elsewhere in the UI). |
| `platforms` | array | OSes where the variable is active (empty = all). |
| `title` | string | Row label (empty → `name`). |
| `tooltip` | string | Hover hint. |
| `if` / `if_or` | array | Row-activity condition — `@bool_var` (§3.3). |

### 3.1 Types

| Type | Widget | Substitution |
|---|---|---|
| `text` | input field (+ combo if `options` is set) | string |
| `bool` | checkbox | `true` / `false` |
| `enum` | dropdown | `value` of the selected option |
| `text_list` | multiline field | array of strings (by line) |
| `secret` | masked field (dots) + eye + regenerate button; always pre-filled with a random value | string |

The object form of `options` (`[{title,value}]`) automatically makes the variable an `enum`.

### 3.2 `default_value` — three forms

```json
"default_value": "system"                              // 1. scalar (string/number/bool)
"default_value": {"win7": "gvisor", "default": "system"}  // 2. by platform
"default_value": {"#if": { ... }}                      // 3. #if expression (only @runtime.*)
```

**By platform** — key lookup order: `win7` (only `windows`/`386`) → `<goos>`
(`windows`/`linux`/`darwin`) → `default`.

**`#if`** (§4) is evaluated at runtime; inside it **only** `@runtime.*` globals are allowed,
references to other vars are forbidden (load error). It may also appear in a platform key:
`{"default": {"#if": {...}}}`.

### 3.3 `if` / `if_or` (outer condition)

```json
{"name": "tun_address", "type": "text", "if": ["@tun"]}
```

- Each element is **`@bool_var_name`** (the `@` prefix is required; bare `"tun"` → load error).
- `if` — active if **all** listed are `true`; `if_or` — if **at least one** is. Mutually exclusive.
- On the **Settings** tab the row stays visible, but the **fields are disabled** until the condition is met.
- Runtime globals (`@runtime.*`) in an outer `if`/`if_or` are **forbidden** — only in `#if`.

### 3.4 Separator

```json
{"separator": true}
```
A horizontal line between Settings rows. No `name`/`type`/`default_value`/etc.
(only `platforms` and `wizard_ui: "hidden"` are allowed).

---

## 4. `#if` — conditional fields

A control construct: includes/excludes fields or elements by condition. The `#if` key
**does not appear** in the output JSON.

### 4.1 Form

```json
"#if": {
  "and":   [ <predicate>, ... ],   // OR "or" — exactly one of and/or
  "value": <any JSON>,             // substituted when the condition is true
  "else":  <any JSON>              // optional — when false
}
```

- Exactly one of `and` / `or` (both or neither → error). `value` is required.
- `and` — all predicates `true`; `or` — at least one.

### 4.2 Two placement modes

**Map-spread** — `#if` as a key inside an object; when true the branch fields are **merged into the parent**:
```json
{"type": "tun", "tag": "tun-in",
 "#if": {"and": [{"@runtime.platform": {"#in": ["windows", "linux"]}}], "value": {
   "interface_name": "singbox-tun0"
 }}}
```

**Array-element** — a lone `{"#if": {...}}` as an array element; the element is
**replaced** by the branch or **dropped** (if false and there is no `else`):
```json
"inbound": [
  {"#if": {"and": ["@tun"],             "value": "tun-in"}},
  {"#if": {"and": ["@enable_proxy_in"], "value": "proxy-in"}}
]
```

### 4.3 Predicates (elements of `and` / `or`)

| Form | Semantics |
|---|---|
| `"@var"` | bool var is true (`scalar == "true"`) |
| `{"@var": "literal"}` | equality: `trim(scalar) == "literal"` |
| `{"@var": "#notEmpty"}` | non-empty (text: length > 0; text_list: has elements; bool: true) |
| `{"@var": "#isEmpty"}` | inverse of `#notEmpty` |
| `{"@var": {"#in": ["a","b"]}}` | `scalar` is in the list (the list may be `"@text_list_var"`) |
| `{"@var": {"#notIn": ["a","b"]}}` | `scalar` is not in the list |
| `{"@var": {"#matches": "^re$"}}` | `scalar` matches a Go regexp |
| `{"#not": <predicate>}` | negation of any predicate (recursive) |

Arguments (literal, `#in` elements, `#matches` regex) may contain `@var` — substituted before evaluation.

### 4.4 Runtime globals — `@runtime.*`

Pseudo-variables available **only** in `#if` predicates (not declared in `vars`):

| Global | Source | Values |
|---|---|---|
| `@runtime.platform` | `runtime.GOOS` | `"darwin"`, `"windows"`, `"linux"` |
| `@runtime.arch` | `runtime.GOARCH` | `"amd64"`, `"arm64"`, `"386"` |

- Only text predicates (equality, `#in`, `#matches`, …); **bare** `"@runtime.platform"` in a list → error.
- Win7 = `windows`+`386`: `{"and": [{"@runtime.platform": "windows"}, {"@runtime.arch": "386"}]}`.
- The namespace is extensible; the name `runtime` in `vars[].name` is reserved.

### 4.5 Naming discipline

- `#` — control constructs and predicates (`#if`, `#in`, `#not`, `#notEmpty`, …); not emitted to the output.
- `@` — variable references (`@var`) and runtime globals (`@runtime.*`).
- bare — ordinary data keys and inner keys of an `#if` body (`and`/`or`/`value`/`else`).
- An unknown `#*` key → warn + drop (forward-compat for future constructs).

---

## 5. `params[]` — `config` patches

Apply `value` to a section of `config` by dot-path when the condition holds.

```json
{
  "name": "route.rules",
  "if_or": ["@tun", "@enable_proxy_in"],
  "mode": "prepend",
  "value": [ { ... } ]
}
```

| Field | Description |
|---|---|
| `name` | dot-path in `config` (`"inbounds"`, `"route.rules"`, `"dns.servers"`). |
| `value` | the value to apply (any JSON); supports `@var` and `#if`. |
| `mode` | `replace` (default) \| `prepend` \| `append` (for arrays). |
| `platforms` | OSes to apply on (empty = all). |
| `if` / `if_or` | condition on the **whole** entry — `@bool_var` (§3.3). |

Load order: check `platforms` → `if`/`if_or` → merge `value` by `mode`
→ substitute `@var` → walk `#if`.

---

## 6. `presets[]` — Rules-tab presets

Self-contained sets of rules/rule-sets/DNS/outbounds. A preset's variables have a
**local scope** (`@name` inside a preset resolves against its `vars[]`).

| Field | Type | Description |
|---|---|---|
| `id` | string | Stable slug (`[a-z0-9_-]+`), referenced by state. |
| `label` | string | UI name. |
| `description` | string | Description (tooltip / card). |
| `default_enabled` | bool | Recommendation: enabled on a fresh install? |
| `platforms` | array | OS availability (empty = all). |
| `vars` | array | Local variables (`PresetVar`, §6.1). |
| `rules` | array | Routing rules (array of objects; `@var`, `rule_set` references, their own `if`). |
| `rule_set` | array | rule_set definitions (`PresetRuleSet`, §6.2). |
| `dns_servers` | array | Bundled DNS servers (`PresetDNSServer`, §6.3). |
| `dns_rule` | object | The preset's DNS rule (its own `if`). |
| `outbounds` | array | Add/patch outbounds (`PresetOutbound`, §6.4). |

### 6.1 `PresetVar`

| Field | Description |
|---|---|
| `name` | local name; `@name` in preset bodies. |
| `type` | `outbound` \| `dns_server` \| `enum` \| `text` \| `number` \| `bool`. |
| `default` | required default (string; for bool — `"true"`/`"false"`). |
| `title` / `tooltip` | UI. |
| `options` | `enum` → `[{title,value}]`; `outbound`/`dns_server` → `["tag", …]` (whitelist). |
| `select` | for `dns_server`: `"local"` (only the preset's bundled) \| `"global"` (all available, default). |
| `if` / `if_or` | activity condition (`@bool_var`). |

> The `secret` / `text_list` types are **not** supported in preset vars (only top-level `vars`).

### 6.2 `PresetRuleSet`

```json
{"tag": "geoip-ru", "type": "remote", "format": "binary", "url": "https://…/geoip-ru.srs", "if": ["@geoip_enabled"]}
{"tag": "messengers", "type": "inline", "format": "domain_suffix", "rules": [ { ... } ]}
```
`tag` is local (at build → `<preset_id>:<tag>`). `type`: `inline` (with `rules`) or `remote` (with `url`). Own `if`/`if_or`.

### 6.3 `PresetDNSServer`

```json
{"tag": "yandex", "type": "https", "server": "dns.yandex", "detour": "@out", "if": ["@use_dns_override"]}
```
`type`: `udp`/`https`/`tls`/`h3`. Fields `server`, `server_port`, `path`, `tls`, `detour` (may be `@var`),
`description`. Reaches the config only if selected via a `@dns_server` var or referenced in `dns_rule.server`.

### 6.4 `PresetOutbound`

```json
{"mode": "add",    "tag": "auto", "type": "urltest", "options": { ... }, "if": ["@flag"]}
{"mode": "update", "tag": "proxy-out", "options": { ... }}
```
`mode`: `add` (new, requires `type`) \| `update` (patch an existing one by `tag`). Fields mirror a
sing-box outbound (`options`, `filters`, `addOutbounds`, `preferredDefault`, `comment`). Own `if`/`if_or`.

---

## 7. `parser_config` / `config` / `dns_options`

- **`parser_config`** — subscription parser settings (`reload`, …) + static outbound groups
  (`selector`/`urltest` via `addOutbounds`/`filters`). Subscription nodes are injected between markers.
- **`config`** — the sing-box skeleton: `route` (rules, rule_set, final, …), `experimental` (clash_api, cache),
  `inbounds`, `log`, etc. Accepts `@var`, is patched by `params`, is walked for `#if`.
- **`dns_options`** — `servers[]` and `rules[]` (DNS-tab defaults). DNS scalars are set by
  `vars` with `wizard_ui: "fix"` (`dns_strategy` → `config.dns.strategy`, `dns_final` → `config.dns.final`,
  `dns_default_domain_resolver` → `config.route.default_domain_resolver`).

> More: subscription parser — [`ParserConfig.md`](ParserConfig.md); state storage
> (`vars`, `dns_options`, rules) — [`WIZARD_STATE.md`](WIZARD_STATE.md); engine and
> stock-JSON format — [`TEMPLATE_REFERENCE.md`](TEMPLATE_REFERENCE.md).

---

## 8. Formatting style (bundled template)

Layout rules for the **shipped** `bin/wizard_template.json`. Key order and line breaks
**do not affect** loading — this is about readability for maintainers and clean diffs.
Custom templates may ignore §8, but the syntax (§1–§7) is mandatory.

**Principle:** *compact* — literals and small metadata objects; *expanded* —
expressions (`@…`, `#if`, outer `if[]`) and long lists.

### 8.1 `vars[]` and `presets[].vars[]`

| Part | Layout |
|---|---|
| "Header" (`name`, `type`, `wizard_ui`, `title`, `tooltip`, `platforms`, `comment`, `select`, …) | line 1 |
| `default_value` / `default` (in expanded form) | its own line |
| `options[]` | multiline — each element on its own line |
| outer `if` / `if_or` | its own line, **at the end** of the object |
| `{"separator": true}` | one line |
| simple preset var (`out`, …) — whole thing (including `default`) | one line |

```jsonc
{"name": "tun_mtu", "type": "text", "wizard_ui": "edit",
  "title": "TUN MTU", "tooltip": "…",
  "default_value": "1492",
  "if": ["@tun"]
},
```

### 8.2 JSON payload (`config`, `params[].value`, `parser_config`)

| Context | Rule |
|---|---|
| fields with `@` | one field — one line |
| literals (`type`, `tag`, `auto_route`, …) | may go together on one line |
| `options` **with** `@`-fields | multiline object |
| `options` / `filters` / `addOutbounds` **without** `@` | one line |
| small literal structs (≤2–3 fields: `direct-out`, hijack-dns, `mode:update`) | one line |
| large objects (`dns_options.servers[]`, preset `dns_servers[]`, a full `mode:add` outbound) | multiline — one field per line |

```jsonc
"options": {
  "url": "@urltest_url",
  "interval": "@urltest_interval",
  "tolerance": "@urltest_tolerance",
  "interrupt_exist_connections": true
},
```

### 8.3 `#if`

| `value` / `else` | Layout |
|---|---|
| scalar | one line: `"#if": {"and": [...], "value": "…", "else": "…"}` |
| object | condition + `"value": {` on line 1; body below; closing `}}` (with `else` → `}, "else": {` … `}}}`) |

A scalar branch (array-element) — one line:
```jsonc
"inbound": [
  {"#if": {"and": ["@tun"], "value": "tun-in"}},
  {"#if": {"and": ["@enable_proxy_in"], "value": "proxy-in"}}
]
```

An object branch (map-spread) — condition + `"value": {` on the first line, body below:
```jsonc
"#if": {"and": [{"@runtime.platform": {"#in": ["windows", "linux"]}}], "value": {
  "interface_name": "singbox-tun0"
}}
```

With `else` (both branches are objects); value arrays are written **as is**, without column alignment:
```jsonc
{"#if": {"and": [{"@runtime.platform": "windows"}], "value": {
  "process_name": ["Telegram.exe", "Discord.exe", "WhatsApp.exe", "Signal.exe", "Zoom.exe"]
}, "else": {
  "process_name": ["Telegram", "Discord", "WhatsApp", "Signal", "zoom.us"]
}}}
```

### 8.4 Presets

| Section | Layout |
|---|---|
| `rules[]`, `dns_rule` | one line |
| `rule_set[]` (inline and remote) | line 1: metadata (`tag`, `type`, `format`, **`if`/`if_or`**); line 2: `rules` / `url` |
| `rule_set[].rules` inline | one line if it fits; very long suffix lists — an array with line breaks |
| `outbounds[]` `mode:update` | one line per entry |
| `outbounds[]` `mode:add` (full outbound) | multiline object; `options` / `filters` — one line |
| `params[]` route.rules (scalar `#if`) | the whole rule on one line |

> Unlike `vars[]` (where `if` is on its own line at the end), for **condensed** objects with metadata on one line (`rule_set[]`) `if`/`if_or` goes **on that same line**.

```jsonc
{"tag": "geoip-ru", "type": "remote", "format": "binary", "if": ["@geoip_enabled"],
  "url": "https://…/geoip-ru.srs"}
```

### 8.5 Cheat sheet

| | One line | Multiline |
|---|---|---|
| var "header" | ✓ | — |
| simple preset var (incl. `default`) | ✓ | — |
| `default_value` / `default` (expanded form) | — | ✓ |
| `options[]` elements | — | ✓ |
| outer `if[]` in vars | — | ✓ (at the end) |
| `rule_set[]` metadata (incl. `if`) | ✓ | `rules` / `url` below |
| `@` in payload | — | ✓ (per field) |
| small literal struct (≤2–3 fields) | ✓ | — |
| large config object (`dns_servers[]`, `mode:add`) | — | ✓ |
| `#if` + object `value` | condition | body |
| `#if` + scalar | ✓ | — |
| `filters`, literal `options` | ✓ | — |

---

## 9. Load errors (common)

| Symptom | Cause |
|---|---|
| `bare var-ref "x" in if[]; use "@x"` | an `if`/`if_or` element without `@`. |
| `vars[].name "runtime" is reserved` | name taken by the `@runtime.*` namespace. |
| `@x is not declared in vars` | `@var` in `config`/`params` without a declaration in `vars`. |
| `#if has both "and" and "or"` | both present in an `#if` body — only one allowed. |
| `#if missing "value"` | no required `value` in the `#if` body. |
| `predicate key "x" must start with @ or be #not` | wrong predicate form (the var is the `@var` key, the predicate is the value). |
| `unknown runtime global @runtime.x` | a namespace field other than `platform`/`arch`. |

After editing the template in the repo — rebuild/reinstall the app or copy the file into
`Contents/MacOS/bin/`, otherwise the changes will not be picked up.
