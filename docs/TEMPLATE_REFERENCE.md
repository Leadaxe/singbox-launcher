# Template reference (wizard_template.json)

**🌐 Language**: English | [Русский](TEMPLATE_REFERENCE.ru.md)

An architectural summary: what lives in `bin/wizard_template.json` and where it
later surfaces in runtime / state / UI. For the template *syntax* reference see
[WIZARD_TEMPLATE.md](WIZARD_TEMPLATE.md). This file is a reference for launcher
developers and for understanding the template ↔ state relationship.

---

## 1. The file

- **`bin/wizard_template.json`** — one template for every OS.
  Platform-specific pieces come from the `params` section plus `if`/`if_or` over
  vars.
- **Pinned ref:** `internal/constants.RequiredTemplateRef` holds the SHA of the
  repository commit the launcher was built against. CI ldflags inject the real
  hash for a release; a dev build uses the source default (the last known-good
  merge commit).
- **Upgrade lifecycle:** `core/template_migration.go::InvalidateTemplateIfStale`
  compares `Settings.LastTemplateLauncherVersion` (written after the last
  successful "Download Template" via `MarkTemplateInstalled`) against
  `constants.AppVersion`. On a mismatch it deletes `bin/wizard_template.json` —
  on the next start the UI shows "Download Template", and after a successful
  download `bin/settings.json` gets a new `last_template_launcher_version`.
  Dev builds (`v-local-test`, `unnamed-dev`, `*-dirty`) skip invalidation —
  otherwise local development would break on every run. See SPEC 046 (the
  mechanism) and SPEC 067 (the breaking template format — `#if` plus `@`-only
  outer `if[]` — triggered by the same `AppVersion` bump).

---

## 2. Top-level shape

```jsonc
{
  "parser_config":   { ... },        // the ParserConfig wrapper for the subscription parser
  "config":          { ... },        // sing-box config skeleton (log/dns/inbounds/outbounds/route/experimental)
  "params":          [ ... ],        // platform-conditional patches over config (replace/prepend/append)
  "dns_options":     {               // dns tab library
    "servers": [ ... ],              // template DNS server entries (+ required:true for the local/direct resolver)
    "rules":   [ ... ]
  },
  "selectable_rules":[ ... ],        // legacy rules library — kept for back-compat, replaced by presets[]
  "presets":         [ ... ],        // SPEC 053 self-contained preset bundles
  "vars":            [ ... ],        // typed template variables (UI Settings tab + @var substitution)
  "group_templates": { ... }         // SPEC 104: group shapes a Direction materializes into
}
```

---

## 3. Per-section storage / usage

| Top-level key | Contains | Where it lands at runtime | UI tab it shows in |
|---------------|----------|--------------------------|------------------|
| `parser_config` | The default ParserConfig skeleton: outbounds (`proxy-out`, `direct-out`, `auto-proxy-out`) with top-level `required: true` markers (see §5). Since SPEC 058 it is the **live source of truth** for the bodies of referenced template outbounds (state holds only the thin `{tag, ref: "#TEMPLATE#"}`). | On a fresh install it goes into `model.ParserConfigJSON`. During LoadState the body of `ref="#TEMPLATE#"` entries is resolved from here on every render/build. | Outbounds tab (renders model.GlobalOutbounds) |
| `config` | The sing-box config skeleton: `log`, `dns`, `inbounds`, `outbounds`, `route`, `experimental`. Contains `@var` placeholders. | After `applyParams(GOOS) + substitute @vars` → `TemplateData.Config` (per section). At build time it is merged with the state-derived sections. | None directly; previewed through the Edit dialog | 
| `params` | Platform-conditional patches (`if`/`if_or` + `replace`/`prepend`/`append`) | Applied in `LoadTemplateData` (GetEffectiveConfig) — they produce the `Config` for the current runtime.GOOS | — |
| `dns_options.servers` | The library of template DNS servers (cloudflare, google, yandex, ...) plus the mandatory `required:true` entries (`local_dns_resolver`, `direct_dns_resolver`) | `TemplateData.DNSOptionsRaw` → used by `ResolveDNS` to resolve the bodies of kind=template entries in state | DNS tab (renders kind=template entries) |
| `dns_options.rules` | Default template DNS rules (optional) | An auxiliary fill for the DNS rules editor when state is empty | DNS tab |
| `selectable_rules` | **Legacy.** The library from the v3+ era, fully superseded by `presets[]`. | Kept for backwards compatibility: loaded into `TemplateData.SelectableRules` and filtered by platform | Not shown (the Library shows only `presets[]`) |
| `presets` | Self-contained preset bundles: vars / rule_set / dns_servers / dns_rule / rules / outbounds (SPEC 053 + 055 + 057; `rules` is a `[]map`, SPEC 067 Phase 9) | `TemplateData.Presets`. Enabling a preset creates `kind=preset` entries in `state.rules`, `state.dns_options.servers/rules` and `state.connections.outbounds` through the Sync* functions | Library dialog (add to Rules) → Rules tab (preset rows) |
| `vars` | Declarations of typed template variables (`name`, `type`, `default_value`, `options`, `wizard_ui`, `title`, `tooltip`, `if`/`if_or`, `platforms`) | `TemplateData.Vars`. The defaults apply when `state.vars[]` has no override. `@var` literals in `config`/`params` are substituted at build time. The outer `if[]`/`if_or[]` accept the **`@`-form only** (SPEC 067). | Settings tab (auto-rendered) + the DNS scalars (`dns_*` hidden vars) |

"UI tab it shows in" means where the user interacts with that section. Output
into `config.json` always goes through the build pipeline; no template section
reaches config.json directly without passing through `state + resolve*`.

---

## 4. Presets (SPEC 053 + SPEC 055 + SPEC 057-R-N)

A preset is a parameterized, self-contained bundle. Each component has its own
referencing mechanism in state.

### 4.1 `presets[].outbounds[]` — SPEC 055 + SPEC 057-R-N

Entries carry a `mode` discriminator:

| `mode` | Effect on state | Effect on config.json |
|--------|------------------|------------------------|
| `add` (or omitted) | Enabling the preset adds an entry to `state.connections.outbounds[]` with `ref = preset.id`. The body resolves from the entry. | Emitted as an ordinary outbound by `GenerateOutboundsFromParserConfig`. |
| `update` | Enabling the preset pushes an `OutboundUpdate{ref, patch}` into `state.connections.outbounds[<target_tag>].updates[]`. The target tag must exist in state (found by Tag; if missing → a warning and a no-op). | `MergeOutboundUpdatesInPlace` applies the patches before the generator runs (base + patches in order). |

Lifecycle: `core/build/sync_outbounds.go::SyncOutboundsWithActivePresets`.
Adopt-on-first-sync: pre-SPEC-057 globals without a `Ref` whose tag matches an
expected preset add are adopted (the body is preserved and a `Ref` is added).

### 4.2 `presets[].dns_servers[]` — SPEC 053 + SPEC 056-R-N

Bundled DNS server definitions with local tags. Enabling the preset makes
`SyncDNSOptionsWithActivePresets` create entries in `state.dns_options.servers[]`
with `kind=preset, ref="<preset_id>:<local_tag>"`. The body is resolved from the
template with `@var` substitution from `preset.body.vars` on every build/render.

The user can toggle individual servers (preserved in state.entries.Enabled).
Disabling the preset removes the entries (re-enabling gives fresh defaults).

### 4.3 `presets[].dns_rule` — SPEC 053

An optional object (at most one per preset). Enabling the preset adds an entry to
`state.dns_options.rules[]` with `kind=preset, ref="<preset_id>"`. The body is
resolved from the template plus vars plus the tag prefix (a `@dns_server` var may
reference a bundled `dns_servers[].tag`).

### 4.4 `presets[].rules` — SPEC 053 (`[]map`, SPEC 067 Phase 9)

A preset's routing rules — a slice (`[]map[string]interface{}`); every entry is a
separate routing rule with its own `if`/`if_or`. Enabling the preset creates one
entry in `state.rules[]` with `kind=preset, ref="<preset_id>"`. At build time
`MergePresetsIntoRoute` (via `ResolveRoute`) expands the ref into
`template.presets[id].rules`: substitute vars, prefix local rule_set
tags, resolves the sentinels (`reject` / `drop` → `action`) and emits every rule
into `route.rules[]` in order (preset entries keep the order they have in
`state.rules[]`). An empty / nil `rules` means a preset with no routing rule (just
rule_set / dns_servers / outbounds), which is valid.

### 4.5 `presets[].vars[]` — SPEC 053 + SPEC 048

A preset's typed local variables.

| `type` | UI control | Substitution value |
|--------|------------|--------------------|
| `outbound` | Dropdown: outbound tags + `reject` + `drop` (optionally whitelisted via `options`) | A tag string |
| `dns_server` | A grouped dropdown (3 sections) or a whitelist (`options`/`select`) | A tag string (the build prefixes bundled tags during substitution) |
| `enum` | A dropdown over `options[]` (objects `{title, value}`) | The `value` string |
| `text` | A text entry | A string |
| `number` | A numeric entry | A numeric string |
| `bool` | Checkbox | `"true"` / `"false"` |

The substitution mechanism: a build-time recursive walk over the `rules` /
`dns_rule` / `dns_servers` / `rule_set` fragments — every `"@name"` string is
replaced with `varsMap[name]`. If a var was filtered out by `if`/`if_or`,
substituting its literal fails → the preset is skipped with a warning.

State stores **only the diff** against the template defaults in `rule.body.vars`
(an empty `vars: {}` means the preset runs on template defaults).

---

### 4.6 `group_templates` — how a Direction materializes (SPEC 104)

The shape a Direction turns into is described **by the template**, not
hard-coded, so the launcher and LxBox read one description.

```jsonc
"group_templates": {
  "magic_nodes": {                                    // service options a Direction may offer
    "auto":   { "source": "generate", "tpl": "{parent_tag}-auto" },
    "direct": { "source": "preset",   "tag": "direct-out" },
    "block":  { "source": "preset",   "tag": "block-out"  }
  },
  "channel": { "type": "selector", "options": { "interrupt_exist_connections": true } },
  "auto":    { "type": "urltest",  "options": { "url": "@urltest_url", "interval": "@urltest_interval" } }
}
```

- **`magic_nodes`** — tags of the service options. `source: "preset"` takes the
  tag verbatim; `source: "generate"` builds it from `tpl` with `{parent_tag}`
  replaced by the Direction's own tag. The direct/block tags are read from here
  rather than assumed: they are not universal, and hard-coding them would break
  a custom template. The launcher also uses `block` for the fallback an empty
  Direction falls back to.
- **`auto.options`** — defaults of the paired urltest, landing in the emitted
  group as-is, **including `@var` references**: substitution is the template
  engine's job, and doing it here would be a second implementation of it. A
  Direction's own `auto` fields override these.
- **`channel`** — the key name is historical (the model was called "channels"
  during SPEC 104's first draft). The template language is shared with LxBox,
  and renaming the key would break templates already in the wild for no gain.

A template without this section still works: Directions are built from what
the user configured, just without template defaults.

Mobile-only: `default_channels` (the phone seeds its starting set from it; the
launcher seeds from `parser_config.outbounds`).

---

## 5. Template-owned vs user-editable

The `"required": true` marker (SPEC 056-R-N Phase C/E) is a template-only flag
that state never persists. It applies to:

| Where | Effect in the UI |
|-----|-------------|
| `parser_config.outbounds[].required` | Outbounds tab: Up/Down + Edit + Reset; Delete is not rendered |
| `dns_options.servers[].required` | DNS tab: enabled + locked (the toggle is blocked); Edit/Delete are blocked |

**Shape (since SPEC 058):** `required` is a top-level field directly on the
outbound entry, not nested inside a wrapper:

```jsonc
"parser_config": {
  "outbounds": [
    { "tag": "auto-proxy-out", "type": "urltest", "required": true,
      "options": { "url": "@urltest_url", "interval": "@urltest_interval" } },
    { "tag": "proxy-out", "type": "selector", "required": true,
      "options": { "outbounds": ["auto-proxy-out", "direct-out"] } },
    { "tag": "direct-out", "type": "direct" }
  ]
}
```

**DEPRECATED:** the old `{ "wizard": { "required": 1 } }` form is still parsed by
a legacy fallback in `td.RequiredOutboundTags()`, purely for backwards
compatibility with older template forks. New templates must use the top-level
`required: true`.

Read live on every UI render through helpers:
- `wizardbusiness.DNSTagLocked(model, tag)` — for DNS
- `templateRequiredTags(model)` → used by the UI render path (`collectRowsForUI` /
  `collectRows` in `ui/configurator/outbounds_configurator/configurator.go`) for
  outbounds

If a template author drops `required:true` in a new template version, the effect
is immediate (state remembers no stale value).

---

## 6. Data flow

```
bin/wizard_template.json (pinned via RequiredTemplateRef)
         │
         ▼
LoadTemplateData(execDir)
         │   read JSON
         │   ApplyParams(runtime.GOOS) → effective Config
         │   Substitute @vars in Config (using the TemplateData.Vars defaults)
         │   ParsePresets → []Preset (filtered by platform)
         │   ParseSelectableRules → []SelectableRule (legacy, filtered by platform)
         ▼
model.TemplateData (in-memory, immutable)
         │
         ├──► UI render (Library dialog, DNS tab, Settings tab, Outbounds tab)
         │
         ├──► build pipeline:
         │     ResolveDNS(state, template, vars)
         │     ResolveRoute(state, template, vars)
         │     MergeOutboundUpdatesInPlace(parserCfg, template)
         │
         └──► presenter Sync* (on every preset toggle):
                SyncDNSOptionsWithActivePresets(rules, &state.DNS, presets)
                SyncOutboundsWithActivePresets(rules, &state.outbounds, presets)
```

TemplateData is immutable after load; modifying the template requires an app restart.

**SPEC 058 — the template body as a live source.** Outbound entries in
`state.connections.outbounds[]` are stored as **thin refs**
(`{tag, ref: "#TEMPLATE#", updates: [...]}`) with no body. On every render/build
the body is resolved from `template.parser_config.outbounds[tag]` through
`MergeOutboundUpdates` / `MergeOutboundUpdatesInPlace`. The effect for a template
author: an edit to `parser_config.outbounds[].options` / `addOutbounds` /
`comment` in a new build reaches users automatically (no manual Reset on every
referenced entry). User edits are stored as a field-level diff in
`updates[].patch` with `ref="#USER#"`. See SPEC 058 and
[DATA_FLOW.md](DATA_FLOW.md) for the resolver pipeline in detail.

---

## 7. `vars` mechanism

**Declaration** (template) — the canonical form from the stock `bin/wizard_template.json`:

```jsonc
"vars": [
  {"name": "tun", "type": "bool", "wizard_ui": "edit",
    "platforms": ["windows", "linux", "darwin"],
    "title": "Enable TUN", "tooltip": "…",
    "default_value": {"windows": "true", "linux": "true", "darwin": "true", "default": "false"}
  },
  {"name": "tun_address", "type": "text", "wizard_ui": "edit",
    "title": "TUN interface address", "tooltip": "…",
    "default_value": "172.16.0.1/30",
    "if": ["@tun"]   // the outer if takes only an @-prefixed bool var (§9.6)
  },
  {"separator": true}
]
```

**Preset-local vars** use the **`default`** field (not `default_value`); the same
`if`/`if_or` discipline and formatting apply — §10.

**Override** (state.json):
```jsonc
"vars": [
  { "name": "tun", "value": "false" }
]
```

**Substitution** (build): the `"@tun"` literals in `config` / `params` / preset
fragments are replaced with the effective value (the state override OR the
template default). The **`if`/`if_or`** conditions on params/presets/vars are
evaluated against the same varsMap; every element of the list **must** be an
`@var` (a bare `"tun"` → the loader
error, §9.6).

**Scope**:
- Global template vars (`template.vars[]`) are visible in the top-level `config`
  / `params` (NOT inside a preset).
- Preset-local vars (`preset.vars[]`) are visible only inside their own preset
  (rule/dns_rule/dns_servers/rule_set). Cross-scope access is forbidden — a preset
  must be self-contained.

**Reserved names:** the `vars[].name` **`runtime`** is reserved (the namespace of
the `@runtime.*` runtime globals, usable in `#if` predicates only — §9.5; the
names `platform` / `arch` are free again).

**Special: DNS scalars.** `dns_strategy`, `dns_final`, `dns_default_domain_resolver`
are declared as hidden `dns_*` vars. The UI DNS tab writes into
`model.SettingsVars`, `SyncDNSModelToSettingsVars` copies them into `state.vars[]`
before Save, and the build substitutes the `@dns_*` literals in `config.dns`.

**Special: `route_final`.** UI Rules tab dropdown «Final outbound» →
`model.SelectedFinalOutbound` → `SettingsVars["route_final"]` →
`state.vars[]`. The template carries `"final": "@route_final"` in `config.route`.

**JSON formatting** of the bundled template — §10 (editorial style, not a loader contract).

Implementation: `core/template/vars_resolve.go` + `core/template/substitute.go`.

---

## 8. Pinned templates

The template lives in the repository → is embedded into the binary → is extracted
into `bin/` on first run. Every release build pins one specific commit:

| Source | When it is used | Where |
|--------|---------------------|-------|
| **CI injection** | Release builds: GitHub Actions substitutes the merge commit's SHA via `-ldflags '-X singbox-launcher/internal/constants.RequiredTemplateRef=<sha>'` | `.github/workflows/release.yml` |
| **Source default** | Dev builds (`go build` without ldflags) | The `internal/constants/constants.go::RequiredTemplateRef` constant |

**The bump procedure** (on every release):
1. Merge `develop` → `main` (this creates a merge commit with the updated `bin/wizard_template.json`)
2. Tag merge commit (`vX.Y.Z`)
3. On `develop`, update the `RequiredTemplateRef` source default to the merge commit's SHA

**Lifecycle at launch**:
```
launcher start
     │
     ▼
InvalidateTemplateIfStale(execDir)
     │   compare Settings.LastTemplateLauncherVersion vs constants.AppVersion
     │   stale (LastTemplateLauncherVersion < AppVersion) → unlink bin/wizard_template.json
     │   (dev AppVersion skip: v-local-test / unnamed-dev / *-dirty)
     ▼
UI shows "Download Template" (when the file is absent)
     │   the user clicks → it is downloaded from raw.githubusercontent.com at the pinned ref
     │   MarkTemplateInstalled → bin/settings.json::last_template_launcher_version = AppVersion
     ▼
LoadTemplateData
```

Implementation: `core/template_migration.go::InvalidateTemplateIfStale` +
`internal/locale/settings.go::LastTemplateLauncherVersion` /
`MarkTemplateInstalled` + `core/template/loader.go::LoadTemplateData`.

Breaking template format changes (SPEC 067's `#if` + `@`-only outer `if[]`, for
instance) ride the same mechanism: after an `AppVersion` bump the stale cache is
deleted on first start → the user downloads the new template in one click.

---

## 9. `#if` construct (SPEC 067)

Template expressions v1 — declarative conditional field inclusion right inside the
template, with no post-substitution Go hooks. Implemented in
`core/template/substitute.go::SubstituteVarsInJSON` (the walker) and
`core/template/template_validate.go::validateIfConstruct` (load-time
validation). It covers cases of the form "one field inside an already-emitted
object depends on a bool var / the runtime platform".

> **Mobile parity (SPEC 103):** `#if` is implemented on both sides — the
> launcher (`core/template/substitute.go`) and LxBox
> (`app/lib/services/builder/if_engine.dart`) — and the normative description of
> the shared language lives in `contract/docs/TEMPLATE_LANG.md`. Where this
> reference and the contract disagree, the contract wins. Other `#*` constructs
> (the potential `#for_each` / `#include`) remain desktop-only until they land in
> LxBox as well.

**Named conditions.** A condition key may carry any suffix: `"#if"`, `"#if1"`,
`"#if 2"`, `"#if tun-only"` are all equivalent. This exists because JSON keeps
only the last of two identical keys — a second plain `"#if"` on the same object
would silently overwrite the first — so a suffix is what lets one object carry
several independent conditions, and it doubles as a readable label. Several
`#if…` keys on one object are applied in sorted key order (identical in both
engines). An array element (§9.4) and a scalar `#if` still need exactly one such
key — with any suffix.

### 9.1 Naming discipline — `#` vs `@` vs data

**The rule in one sentence (SPEC 107, D-073):**

> `#` marks an engine keyword. `@` marks a variable reference. Everything else is data.

| Prefix | What it is | Examples |
|--------|-----------|----------|
| `#` | engine keyword: constructs, condition operators, branches | `#if`, `#enable`, `#and`, `#or`, `#not`, `#value`, `#else`, `#in`, `#notIn`, `#matches`, `#notEmpty`, `#isEmpty`, `#on_change`, `#set` |
| `@` | reference to a `vars[]` entry or a runtime global | `@tun`, `@tun_mtu`, `@runtime.platform` |
| no prefix | data — sing-box fields and values | `outbound`, `rule_set`, `server`, `options[].value` |

The first character of a key is self-describing: the parser needs no context,
and the template author needs not remember which construct they are inside.

**Legacy spellings are read indefinitely.** Before SPEC 107 the rule held only
half the time: `#not`/`#in`/`#matches` were marked, while `and`/`or`/`value`/
`else`/`on_change`/`set` were not — although the same engine interprets them.
Unmarked forms stay valid, existing templates keep working, and a mixed
spelling (canonical outside, legacy inside) is valid too.

Forward compatibility: an unknown key starting with `#` is logged and dropped
(graceful degradation), so new constructs can be added without breaking older
launchers. A side benefit of the marked canon: a typo such as `#adn` is now
caught by that same mechanism.

### 9.2 Shape

```jsonc
"#if": {
  "#or":    [<cond>, <cond>, ...],   // mutually exclusive with "#and"
  "#and":   [<cond>, <cond>, ...],   // mutually exclusive with "#or"
  "#value": <any JSON>,               // required, then-branch
  "#else":  <any JSON>                // optional, else-branch
}
```

An element of `#and`/`#or` is a predicate **or a nested cond-obj**; depth is
unlimited (SPEC 107 lifted the earlier restriction):

```jsonc
"#if": {
  "#or": [
    "@tun",
    {"#and": [{"@runtime.platform": "windows"}, "@enable_proxy_in"]}
  ],
  "#value": { "...": "..." }
}
```

`#not` negates any condition, including `#and`/`#or`, not just a predicate.

Load-time validation:
* Exactly one of `#and` / `#or`. Neither or both → load error.
* `#value` is required (not nil).
* `#else` is optional; null in `#value`/`#else` is an error in map-spread
  (nothing to merge), legal in array-element position.
* An invalid shape evaluates to **false** plus a warning at runtime
  (fail-closed): a typo in a condition must never silently enable a node.

The same keys spelled without `#` (`and`, `or`, `value`, `else`) are supported
indefinitely.

### 9.2.1 `#enable` — node existence gate

A flat key among the node's own fields: it substitutes nothing and instead
decides whether the node exists at all.

```jsonc
{
  "tag": "geoip-ru",
  "type": "remote",
  "#enable": ["@geoip_enabled"],
  "url": "https://example.com/geoip-ru.srs"
}
```

* The value is any condition (§9.2), including the list shorthand:
  `["@a", "@b"]` ≡ `{"#and": ["@a", "@b"]}`.
* false → the node drops out: the key disappears from its object, the element
  from its array. Its contents are **not evaluated at all**.
* Evaluated **first**, before any `#if` on the same node.
* Carriers: objects in the `config` tree, preset fragments (`rule_set[]`,
  `rules[]`, `dns_servers[]`, `dns_rule(s)`, `outbounds[]`), variable
  declarations (greys out the Settings row), `params[]` (block not applied).
* Legacy equivalents: `if` (≡ `#and` list), `if_or` (≡ `#or` list), and in
  LxBox `enabled: "@var"` — all read indefinitely.

Difference from `#if`: `#if` substitutes one of the `#value`/`#else` branches;
`#enable` is a boolean gate with no branches.

### 9.3 The two placement modes

**Map-spread mode** — `#if` as a key inside an object:

```jsonc
{
  "type": "mixed", "tag": "proxy-in",
  "listen": "@proxy_in_listen",
  "listen_port": "@proxy_in_listen_port",
  "set_system_proxy": "@proxy_in_set_system_proxy",
  "#if": {"and": ["@proxy_in_auth_enabled"], "value": {
    "users": [{
      "username": "@proxy_in_username",
      "password": "@proxy_in_password"
    }]
  }}
}
```

* condition true → `value` must be an object; its fields are merged into the
  parent object (on a collision the branch wins). The `#if` key is removed.
* condition false: with an `else`, its fields are merged; without one the key is
  simply removed (the parent is unchanged).

**Array-element mode** — `#if` as the sole key of an object that is an array
element:

```jsonc
"options": [
  "always",
  {"#if": {"and": ["@dark_mode"], "value": "extra-dark", "else": "extra-light"}},
  "regular"
]
```

* condition true → the element is replaced with `value` (of any type).
* condition false: with an `else` it is replaced with `else`; without one the
  element is **removed** from the array (length −1).

Detection rule: an element is an `#if` wrapper when it is an object with EXACTLY
one key, `#if`. Otherwise it is an ordinary element (possibly containing a
spread-mode `#if` inside).

### 9.4 Expression language — predicates

Every element of `and` / `or` is a predicate. Eight forms:

| Form | Semantics |
|---|---|
| `"@var"` | A bool template var → `scalar == "true"` (bool vars only; **not** `@runtime.platform` / `@runtime.arch`) |
| `{"@var": "literal"}` | Equality: `trim(scalar) == "literal"` (the literal must **not** start with `#`) |
| `{"@var": "#notEmpty"}` | text → `len(trim(scalar)) > 0`; text_list → `len(list) > 0`; bool → `scalar == "true"` |
| `{"@var": "#isEmpty"}` | The inverse of `#notEmpty` |
| `{"@var": {"#in":      ["a","b","c"]}}` | `trim(scalar)` is present in the list (`["..."]` or `@text_list_var`) |
| `{"@var": {"#notIn":   ["a","b","c"]}}` | `trim(scalar)` is absent from the list |
| `{"@var": {"#matches": "^[a-z]+$"}}` | `trim(scalar)` matches the Go regexp |
| `{"#not": <predicate>}` | Unary negation (a recursive inner predicate) |

Substitution inside predicate arguments: the literal in an equality, the elements
of `#in` / `#notIn`, and the regex pattern in `#matches` may contain `@var` — the
walker substitutes them **before** evaluating the predicate. EXCEPTION: a bare
`"@var"` in a predicate list and the `"@var"` key of a single-key object are not
substituted (that would lose the var reference).

Example:

```jsonc
"and": [
  "@flag_a",                                       // bool true
  {"#not": "@flag_b"},                             // bool false
  {"@runtime.platform": {"#in": ["darwin", "linux"]}},     // runtime GOOS
  {"@runtime.arch": "amd64"},                              // runtime GOARCH
  {"@protocol": {"#in": ["vless", "trojan"]}},
  {"#not": {"@hostname": {"#matches": "^test-"}}}
]
```

### 9.5 Runtime globals — namespace `@runtime.*`

The `@runtime.*` namespace holds pseudo-vars available **only** in `#if.and` /
`#if.or` predicates (it is extensible — new fields are added under `@runtime.`):

| Global | Runtime source | Values |
|---|---|---|
| `@runtime.platform` | `runtime.GOOS` | `"darwin"`, `"windows"`, `"linux"` |
| `@runtime.arch` | `runtime.GOARCH` | `"amd64"`, `"arm64"`, `"386"` |

The semantics are the same predicate forms as for a text var (equality, `#in`,
`#notIn`, `#matches`, `#notEmpty` / `#isEmpty`). Bare `"@runtime.platform"` /
`"@runtime.arch"` in a predicate list (the bool form) → a validation error: they are not bools.

Case-sensitive lower case (like `runtime.GOOS` / `runtime.GOARCH`).
**Reserved:** `vars[].name == "runtime"` → a loader error (it collides with the
`@runtime.*` namespace; `platform` / `arch` are free again as var names). **The outer
`if` / `if_or`** runtime globals
**do not accept them** — only bool template vars there; platform gating at the
param level still goes through `params[].platforms[]`.

The Win7 build (`windows/386`): `{"@runtime.platform": "windows"}` +
`{"@runtime.arch": "386"}` in one `and` is the equivalent of "win7 binary only".

### 9.6 The outer `if` / `if_or` — canonically `@`-only

`params[].if` / `params[].if_or`, `vars[].if` / `vars[].if_or`,
`presets[].if` / `presets[].if_or` accept **only** `@`-prefixed var refs. A bare
`"tun"` → a loader error at template load:

```
template: params[N].if has bare var-ref "tun" in if[]; use canonical "@tun" form
```

The var must exist in `vars[]` and have `type: "bool"`. The runtime globals
(`@runtime.platform` / `@runtime.arch`) are **forbidden** in the outer `if[]` — they
belong in `#if`
predicates.

### 9.7 A real example — the TUN inbound without duplication

Before (two `params[].name="inbounds"` entries differing **only** in whether
`interface_name` is present):

```jsonc
{ "name": "inbounds", "platforms": ["windows", "linux"], "if": ["@tun"],
  "value": [{ "type": "tun", "tag": "tun-in", "interface_name": "singbox-tun0",
              "address": ["@tun_address"], "mtu": "@tun_mtu",
              "auto_route": true, "strict_route": "@strict_route",
              "stack": "@tun_stack" }] },
{ "name": "inbounds", "platforms": ["darwin"], "if": ["@tun"],
  "value": [{ "type": "tun", "tag": "tun-in",
              "address": ["@tun_address"], "mtu": "@tun_mtu",
              "auto_route": true, "strict_route": "@strict_route",
              "stack": "@tun_stack" }] }
```

After (a single entry, with the platform-conditional field encapsulated in a map-spread
`#if`):

```jsonc
{
  "name": "inbounds",
  "if": ["@tun"],
  "value": [{
    "type": "tun", "tag": "tun-in", "auto_route": true,
    "address": ["@tun_address"],
    "mtu": "@tun_mtu",
    "strict_route": "@strict_route",
    "stack": "@tun_stack",
    "#if": {"and": [{"@runtime.platform": {"#in": ["windows", "linux"]}}], "value": {
      "interface_name": "singbox-tun0"
    }}
  }]
}
```

**route.rules** with a dynamic `inbound[]` (an array-element `#if` with scalar `value`s):

```jsonc
{
  "name": "route.rules",
  "if_or": ["@tun", "@enable_proxy_in"],
  "mode": "prepend",
  "value": [
    {"inbound": [{"#if": {"and": ["@tun"], "value": "tun-in"}}, {"#if": {"and": ["@enable_proxy_in"], "value": "proxy-in"}}], "action": "resolve", "strategy": "@resolve_strategy"}
  ]
}
```

Details and edge cases — `SPECS/067-F-N-TEMPLATE_EXPRESSIONS/SPEC.md`.

---

### 9.8 `default_value` supports `#if` (runtime-only)

`vars[].default_value` may be an `#if` expression — the default is computed at
runtime from the `@runtime.*` globals. It generalizes the per-platform keys
(`win7` / `<goos>` / `default`): conditions (`and`/`or`, `#in`, `#matches`, …)
instead of named keys.

**`@runtime.*` only:** referencing other `vars[]` inside a `default_value` `#if`
is **forbidden** (a loader error) — while defaults are being resolved the other
vars are not resolved yet and the order is not guaranteed. The globals do not
depend on resolution order.

Two forms:

```jsonc
// top-level: the whole default_value is an #if
"default_value": {"#if": {"and": [{"@runtime.platform": "windows"}, {"@runtime.arch": "386"}],
                          "value": "gvisor", "else": "system"}}

// per-platform: a platform key's value is an #if tree (mixable with plain strings)
"default_value": {"default": {"#if": {"and": [{"@runtime.platform": "windows"}, {"@runtime.arch": "386"}],
                                      "value": "gvisor", "else": "system"}}}
```

The chosen branch is resolved recursively down to a scalar (string / number /
bool → string); a false condition without an `else` yields an empty default.
Implementation — `VarDefaultValue.ForPlatform` (`core/template/vars_default.go`);
validation — `validateDefaultValueIf` (with an empty `varByName`, any user-var ref
counts as an "unknown var").

## 10. Formatting style (stock `bin/wizard_template.json`)

Editorial conventions for the **bundled** template. Key order and line breaks
**do not affect** the loader — they are readability for maintainers. Custom
templates may ignore §10, but the semantic rules (§9) are mandatory.

### 10.1 The general principle

**Compact** — literals and small metadata objects. **Expanded** — expressions
(`@…`, `#if`, the outer `if[]`) and long lists.

### 10.2 Top-level `vars[]` and `presets[].vars[]`

| Part | Formatting |
|-------|------------|
| The "header" | **Line 1:** `name`, `type`, `wizard_ui`, `title`, `tooltip`, `platforms`, `comment`, `select`, … |
| `default_value` / `default` | **Its own line**, indented |
| `options[]` | **Multiline:** one element per line (`{title,value}` per element) |
| The outer `if` / `if_or` | **Its own line**, **at the end** of the object (after the metadata) |
| `{"separator": true}` | **A single line** |
| Simple preset vars (`out`, …) | **A single line** in full |

An example of a conditional var:

```jsonc
{"name": "tun_mtu", "type": "text", "wizard_ui": "edit",
  "title": "TUN MTU", "tooltip": "…",
  "default_value": "1492",
  "if": ["@tun"]
},
```

### 10.3 JSON payload (`config`, `params[].value`, `parser_config`)

| Context | Rule |
|----------|---------|
| Fields containing `@` | **One field per line** |
| Literals (`type`, `tag`, `auto_route`, …) | May share one line |
| `options` **with** `@` fields | **A multiline object** (not one line) |
| `options` / `filters` / `addOutbounds` **without** `@` | **A single line** |
| Small literal-only structs (≤2–3 fields) | **A single line** (`direct-out`, hijack-dns, `mode:update`) |
| Large objects (`dns_options.servers[]`, preset `dns_servers[]`, a full `mode:add`) | **Multiline** — one field per line |

An example of urltest `options`:

```jsonc
"options": {
  "url": "@urltest_url",
  "interval": "@urltest_interval",
  "tolerance": "@urltest_tolerance",
  "interrupt_exist_connections": true
},
```

### 10.4 `#if` construct

| `value` / `else` | Formatting |
|------------------|------------|
| **A scalar** | `"#if": {"and": [...], "value": "…"}` — a single line |
| **An object** | the condition + `"value": {` on line 1; the object body below; closing with `}}` |

A map-spread example:

```jsonc
"#if": {"and": [{"@runtime.platform": {"#in": ["windows", "linux"]}}], "value": {
  "interface_name": "singbox-tun0"
}}
```

### 10.5 Presets

| Section | Formatting |
|--------|------------|
| `rules`, `dns_rule`, simple `vars[]` | a single line |
| `rule_set[]` (inline and remote) | line 1: the metadata (`tag`/`type`/`format`/**`if`/`if_or`**); line 2: `rules` / `url` |
| `rule_set[]` inline with long suffixes | a single line when it fits; very long ones wrap inside the arrays |
| `outbounds[]` `mode:update` | one line per entry |
| `outbounds[]` `mode:add` (full) | a multiline object; `options` / `filters` on a single line |
| `params[]` route.rules (scalar `#if`s) | the whole rule on one line |

> In **condensed** objects whose metadata sits on one line (`rule_set[]`), `if`/`if_or` goes **on that same line** — unlike `vars[]`, where `if` takes its own line at the end.

### 10.6 Cheat sheet

| | Single line | Multiline |
|---|-------------|-----------|
| var metadata (line 1) | ✓ | — |
| `default_value` / `default` | — | ✓ |
| `options[]` elements | — | ✓ |
| the outer `if[]` in vars | — | ✓ (at the end) |
| `@` in a payload | — | ✓ (per field) |
| `#if` + an object `value` | the condition | the body |
| `#if` + scalar | ✓ | — |
| `filters`, literal `options` | ✓ | — |

---

## 11. Where the implementation lives

| File | What |
|------|-----|
| `core/template/loader.go` | `LoadTemplateData` (entry point) + `TemplateData` struct |
| `core/template/preset_loader.go` | `LoadPresets` + validation |
| `core/template/preset_types.go` | Preset / PresetVar / PresetRuleSet / PresetDNSServer / PresetOutbound types |
| `core/template/preset_lite.go` | The `PresetLite` interface + `PresetLiteMap` (for sync_dns without cyclic deps) |
| `core/template/vars_resolve.go` | varsMap build + outer `if`/`if_or` eval (strict `@`-prefix, SPEC 067) |
| `core/template/substitute.go` | recursive `@var` substitution + `#if` walker / predicate engine / runtime globals `@runtime.platform`/`@runtime.arch` (SPEC 067) |
| `core/template/template_validate.go` | template-side validation (uniqueness, refs resolvable, `#if` construct + outer `@`-only refs — SPEC 067) |
| `internal/constants/constants.go` | `RequiredTemplateRef` + `WizardTemplateFileName` |
| `core/template_migration.go` | `InvalidateTemplateIfStale` (stale template invalidation) |
| `core/build/preset_expand.go` | preset expand at build time (substitute + tag prefix + filter) |

See also: [WIZARD_STATE.md](WIZARD_STATE.md) — how state interacts with the
template, the `state.json` v6 format, the Sync* lifecycle.
[DATA_FLOW.md](DATA_FLOW.md) — extended load/save/build/toggle diagrams.
[WIZARD_TEMPLATE.md](WIZARD_TEMPLATE.md) — a tutorial for preset and template-var
authors (§10 here is the editorial style for maintainers of the bundled
template).
