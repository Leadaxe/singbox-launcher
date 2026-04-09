# Creating Your Own Config Wizard Template

This guide is for **VPN providers and service administrators** who want to create a custom configuration template (`wizard_template.json`) that users can configure through the built-in wizard interface.

---

## What is the Config Wizard?

The Config Wizard is a visual interface that helps users create their `config.json` file without manually editing JSON. As a provider, you create a template file (`wizard_template.json`) that defines:

- Default settings (DNS, logging, routing rules)
- Which rules users can enable/disable via checkboxes
- How generated proxy outbounds are inserted
- Default proxy groups and selectors
- Platform-specific configurations (Windows, macOS, Linux)

Users simply:
1. Enter their subscription URL or direct proxy links
2. Choose which optional rules to enable
3. Click "Save" to generate their `config.json`

**Wizard `state.json` (not part of the template file):** The launcher saves snapshots under `bin/wizard_states/`. The **root** JSON field **`version`** is the **wizard state format** (currently **4** on new saves; **2–4** when loading). Version **4** marks the **032** era: **`state.vars`**, template **`vars`**, **`@name`** placeholders in **`config`**/**`params`**, and conditional **`if`**/**`if_or`** on **params**. This is **separate** from **`parser_config.version`** in the same file or in **`wizard_template.json`**. See **docs/WIZARD_STATE.md**.

---

## Quick Start

### Step 1: Create the Template File

Create a file named `wizard_template.json` in the `bin/` folder (next to the application executable). **This single file works for all platforms** (Windows, macOS, Linux).

### Step 2: Use This Minimal Template

Copy this skeleton and customize it:

```json
{
  "parser_config": {
    "ParserConfig": {
      "version": 4,
      "proxies": [
        {
          "source": "https://your-subscription-url-here"
        }
      ],
      "outbounds": [
        {
          "tag": "auto-proxy-out",
          "type": "urltest",
          "options": {
            "url": "https://cp.cloudflare.com/generate_204",
            "interval": "5m",
            "tolerance": 100
          },
          "outbounds": {
            "proxies": {}
          }
        },
        {
          "tag": "proxy-out",
          "type": "selector",
          "options": {
            "default": "auto-proxy-out"
          },
          "outbounds": {
            "addOutbounds": ["direct-out", "auto-proxy-out"],
            "proxies": {}
          }
        }
      ]
    }
  },

  "config": {
    "log": {
      "level": "info",
      "timestamp": true
    },
    "dns": {
      "servers": [
        {
          "type": "udp",
          "tag": "direct_dns",
          "server": "9.9.9.9",
          "server_port": 53
        }
      ],
      "final": "direct_dns"
    },
    "inbounds": [],
    "outbounds": [
      { "type": "direct", "tag": "direct-out" }
    ],
    "route": {
      "rule_set": [],
      "rules": [
        { "ip_is_private": true, "outbound": "direct-out" },
        { "domain_suffix": ["local"], "outbound": "direct-out" }
      ],
      "final": "proxy-out"
    },
    "experimental": {
      "clash_api": {
        "external_controller": "127.0.0.1:9090",
        "secret": "CHANGE_THIS_TO_YOUR_SECRET_TOKEN"
      }
    }
  },

  "selectable_rules": [
    {
      "label": "Block Ads",
      "description": "Soft-block ads by rejecting connections",
      "default": true,
      "rule_set": [
        {
          "tag": "ads-all",
          "type": "remote",
          "format": "binary",
          "url": "https://raw.githubusercontent.com/v2fly/domain-list-community/release/geosite-category-ads-all.srs",
          "download_detour": "direct-out",
          "update_interval": "24h"
        }
      ],
      "rule": {
        "rule_set": "ads-all",
        "action": "reject"
      }
    },
    {
      "label": "Route Russian domains directly",
      "description": "Send Russian domain traffic directly",
      "rule": {
        "rule_set": "ru-domains",
        "outbound": "direct-out"
      }
    }
  ],

  "params": [
    {
      "name": "inbounds",
      "platforms": ["windows", "linux"],
      "value": [
        {
          "type": "tun",
          "tag": "tun-in",
          "interface_name": "singbox-tun0",
          "address": ["172.16.0.1/30"],
          "mtu": 1400,
          "auto_route": true,
          "strict_route": false,
          "stack": "system"
        }
      ]
    },
    {
      "name": "route.rules",
      "platforms": ["windows", "linux"],
      "mode": "prepend",
      "value": [
        { "inbound": "tun-in", "action": "resolve", "strategy": "prefer_ipv4" },
        { "inbound": "tun-in", "action": "sniff", "timeout": "1s" }
      ]
    },
    {
      "name": "inbounds",
      "platforms": ["darwin"],
      "value": [
        {
          "type": "mixed",
          "tag": "proxy-in",
          "listen": "127.0.0.1",
          "listen_port": 7890,
          "sniff": true,
          "sniff_override_destination": true,
          "set_system_proxy": true
        }
      ]
    }
  ]
}
```

### Step 3: Customize for Your Service

1. **Update `parser_config`**: Replace `"source": "https://your-subscription-url-here"` with your actual subscription URL (or leave empty if users will enter their own)
2. **Adjust outbound groups**: Modify the `outbounds` array in `parser_config` to match your proxy group structure
3. **Add your rules**: Replace the example `selectable_rules` with your own routing rules
4. **Set default final**: Change `"final": "proxy-out"` in `config.route` to match your default proxy group tag
5. **Configure platform-specific settings**: Add or modify entries in `params` for platform-specific configurations

---

## Understanding the Template Structure

The unified template consists of five main sections:

### 1. `parser_config` Section

**Purpose**: Defines the default subscription parser configuration.

**Structure**:
```json
{
  "parser_config": {
    "ParserConfig": {
      "version": 4,
      "proxies": [
        { "source": "https://example.com/subscription" }
      ],
      "outbounds": [
        {
          "tag": "auto-proxy-out",
          "type": "urltest",
          "options": { "url": "https://cp.cloudflare.com/generate_204", "interval": "5m" },
          "outbounds": { "proxies": {} }
        }
      ],
      "parser": {
        "reload": "1h",
        "last_updated": "2026-01-30T12:00:00Z"
      }
    }
  }
}
```

**Fields**:
- `version`: Always use `4` (current ParserConfig version)
- `proxies`: Array with subscription URLs (users can override these in the wizard)
- `outbounds`: Default proxy groups (urltest, selector, etc.) that will be available to users
- `parser`: Parser settings (reload interval, last updated timestamp)

**Important**: The wizard normalizes this to version 4 format automatically. Make sure all `tag` values match what you reference in your `route` section.

---

### 2. `config` Section

**Purpose**: Contains the main sing-box configuration (platform-independent part).

**Structure**:
```json
{
  "config": {
    "log": { "level": "warn", "timestamp": true },
    "dns": { /* DNS configuration */ },
    "inbounds": [],
    "outbounds": [
      { "type": "direct", "tag": "direct-out" }
    ],
    "route": {
      "rule_set": [],
      "rules": [
        { "ip_is_private": true, "outbound": "direct-out" },
        { "domain_suffix": ["local"], "outbound": "direct-out" }
      ],
      "final": "proxy-out"
    },
    "experimental": { /* experimental features */ }
  }
}
```

**Important points**:
- `inbounds`: Should be an empty array `[]` — it will be filled from `params` based on the platform
- `outbounds`: Contains only static outbounds (like `direct-out`). Generated proxy outbounds from the parser are automatically inserted at the beginning of this array (between markers `/** @ParserSTART */` and `/** @ParserEND */`).
- `endpoints`: (Optional.) Empty array `[]` by default. WireGuard nodes from sources are written between `/** @ParserSTART_E */` and `/** @ParserEND_E */`. Requires sing-box 1.11+. See [ParserConfig.md](ParserConfig.md) for `wireguard://` links.
- `route.rules`: Contains only basic universal rules (hijack-dns, ip_is_private, local). Per-user routing presets are authored in **`selectable_rules`**; the wizard merges **enabled** copies into `route` from a single saved list **`custom_rules`** (see **docs/WIZARD_STATE.md**), not as a second parallel `route` layer
- `route.rule_set`: Contains only shared rule sets used by multiple rules or DNS rules. Rule sets specific to individual selectable rules are defined within those rules
- `route.default_domain_resolver`: Tag of a DNS server from `config.dns.servers` used to resolve domain names inside the route engine. In the current model this value is controlled by DNS-tab variable **`dns_default_domain_resolver`** (stored in `state.vars`, applied via `@dns_default_domain_resolver`).

---

### 3. `dns_options` Section

**Purpose**: Wizard-only DNS list defaults. In the current model `dns_options` contains only:
- `servers`
- `rules`

Scalars edited on the DNS tab are template vars (with `wizard_ui: "fix"`):
- `dns_strategy` -> `config.dns.strategy`
- `dns_independent_cache` -> `config.dns.independent_cache`
- `dns_final` -> `config.dns.final`
- `dns_default_domain_resolver` -> `config.route.default_domain_resolver`

**Structure**:
```json
{
  "dns_options": {
    "servers": [
      { "type": "udp", "tag": "direct_dns_resolver", "server": "8.8.8.8", "server_port": 53, "enabled": true }
    ],
    "rules": [
      { "server": "direct_dns_resolver" }
    ]
  }
}
```

**Storage**:
- `state.json -> dns_options`: `servers` and `rules` only
- `state.json -> vars`: DNS scalar values (`dns_*`)

Legacy `dns_options` scalar keys (`strategy`, `final`, `independent_cache`, `default_domain_resolver`, `default_domain_resolver_unset`) are migrated on load into `state.vars` and are not written back by new saves.

---

### 4. `selectable_rules` Section

**Purpose**: Preset definitions used by the wizard **Rules** tab: **first-run seed** (entries with `"default": true`) and the **Add from library** dialog. They are **not** a separate on/off row list in `state.json` anymore — the running profile stores one ordered **`custom_rules`** array (see **docs/WIZARD_STATE.md**, spec **027**).

**Structure**:
```json
{
  "selectable_rules": [
    {
      "label": "Block Ads",
      "description": "Soft-block ads by rejecting connections",
      "default": true,
      "rule_set": [
        {
          "tag": "ads-all",
          "type": "remote",
          "format": "binary",
          "url": "https://example.com/rules/ads.srs",
          "download_detour": "direct-out",
          "update_interval": "24h"
        }
      ],
      "rule": {
        "rule_set": "ads-all",
        "action": "reject"
      }
    }
  ]
}
```

**Fields**:
- `label` (required): The checkbox label shown in the wizard UI
- `description` (required): Tooltip text shown when user clicks "?" button
- `default` (optional, boolean): If `true`, the rule is enabled by default when the wizard first opens
- `platforms` (optional, array of strings): Platforms where this rule is available (`"windows"`, `"linux"`, `"darwin"`). If omitted, the rule is available on all platforms
- `rule_set` (optional, array): Rule set definitions needed for this rule. These are added to `config.route.rule_set` **only if the rule is enabled**
- `rule` (optional, object): Single routing rule (mutually exclusive with `rules`)
- `rules` (optional, array): Multiple routing rules (mutually exclusive with `rule`)

**Rule format**: Can be a single rule object or an array of rules:
```json
// Single rule
{
  "label": "Block ads",
  "rule": {
    "rule_set": "ads-all",
    "action": "reject"
  }
}

// Multiple rules (array)
{
  "label": "Gaming rules",
  "rules": [
    { "rule_set": "games", "outbound": "direct-out" },
    { "port": [27000, 27001], "outbound": "direct-out" }
  ]
}
```

**Outbound selection**: If your rule has an `outbound` field, the wizard automatically shows a dropdown so users can choose which proxy group to use. Available options come from:
- Tags defined in `parser_config` outbounds
- Tags from generated proxies
- Always available: `direct-out`, `reject`, `drop`

**Special case - reject rules**: If your rule has `"action": "reject"`, the wizard offers `reject` and `drop` options instead of proxy groups.

**Platform-specific rules**: You can create platform-specific rules with the same label but different `platforms` and rule content:
```json
{
  "label": "Messengers via proxy",
  "platforms": ["windows"],
  "rule": {
    "rule_set": "messengers",
    "process_name": ["Telegram.exe", "Discord.exe"],
    "outbound": "proxy-out"
  }
},
{
  "label": "Messengers via proxy",
  "platforms": ["darwin", "linux"],
  "rule": {
    "rule_set": "messengers",
    "process_name": ["Telegram", "Discord"],
    "outbound": "proxy-out"
  }
}
```

---

### Template variables (`vars`) and `@` placeholders

**Purpose:** Declare values that appear on the wizard **Settings** tab and are stored in `state.json` under **`vars`** as `{ "name", "value" }` strings. In **`config`** and **`params`**, string literals like `"@var_name"` (and single-element arrays `["@var_name"]` where supported) are replaced when building the effective config. Numeric template fields use **`@tun_mtu`** and **`@mixed_listen_port`** (resolved as integers).

**DNS tab uses `vars` (`dns_*`) now:** Wizard DNS scalars are template vars and use `@dns_*` substitution in `config`. `dns_options` remains only for `servers` / `rules`. See **`docs/WIZARD_STATE.md`** and **`SPECS/032-F-C-WIZARD_SETTINGS_TAB/SUB_SPEC_DNS_TAB_VARS.md`**.

**`params.if`:** Optional array of **boolean** variable names, e.g. `"if": ["tun"]`. The param is applied only if **every** listed variable is true **on the current OS** (see **`vars[].platforms`**: if the var is scoped to other platforms, it counts as false here).

**`params.if_or`:** Optional array of boolean variable names, e.g. `"if_or": ["tun_builtin", "tun"]`. The param is applied if **at least one** listed variable is true on the current OS. **`if`** and **`if_or`** cannot be set on the same param.

**How `vars[].platforms` interacts with `if` / `if_or`:** For each name in **`if`** or **`if_or`**, the launcher checks whether that variable **applies to the current OS** (empty **`platforms`** → all OSes; otherwise **`runtime.GOOS`** must appear in the list). The legacy Win7 launcher binary is **`windows`** with **`386`** — use **`"windows"`** in **`params` / `selectable_rules`**, not a separate **`win7`** tag in **`platforms`**. The stock **`tun_stack`** var uses **`default_value`** as a JSON object, e.g. **`{"win7":"gvisor","default":"system"}`**, so unset stack defaults to **`gvisor`** on that build and **`system`** elsewhere (see **`VarDefaultValue`** in **`ui/wizard/template/vars_default.go`**). If the variable does **not** apply, it contributes **`false`** to the AND/OR — **before** reading the resolved value from wizard state. A saved **`"true"`** in **`state.vars`** for e.g. **`tun`** does **not** make **`if: ["tun"]`** pass on Linux if **`tun`** is **`darwin`**-only in the template. (Implementation: **`VarAppliesOnGOOS`** / **`ParamBoolVarTrue`** in **`ui/wizard/template/vars_resolve.go`**.)

The stock template uses **`"if": ["tun"]`** with **`"platforms": ["darwin"]`** for macOS TUN **inbounds**, and a single **`route.rules`** prepend with **`"if_or": ["tun_builtin", "tun"]`** and **`"platforms": ["windows", "linux", "darwin"]`**. Do not use the legacy label **`darwin-tun`** in **`platforms`**: it never equals **`runtime.GOOS`** on macOS (**`darwin`**). Use **`darwin`** with **`if`** / **`if_or`** for TUN-specific blocks.

**Variable fields (in `vars` array):** `name` (recommended: `[A-Za-z_][A-Za-z0-9_]*` — enforced at load), `type` (`text`, `bool`, `enum`, `text_list`, `secret`), **`default_value`** (JSON string, number, boolean, or **object** of platform key → string — see **Platform keys for `default_value` objects** below), optional `default_node`, `options` (for `enum`), `platforms`, `wizard_ui` (`edit` / `view` read-only / `hidden` / `fix`), **`title`** (short row label on **Settings**; if omitted or blank, **`name`** is shown), **`tooltip`** (optional hover hint on supported widgets). Optional **`if`** / **`if_or`** (same semantics as **`params`**, mutually exclusive): on the **Settings** tab the row stays visible (subject to **`platforms`** / **`wizard_ui`**) but **controls are disabled** until the condition is met — e.g. TUN address/MTU with **`"if_or": ["tun_builtin", "tun"]`** until the platform-relevant TUN flag is on. Changing a bool var that others reference **refreshes** dependent rows. Type **`secret`** is reserved for the built-in pair **`name`: `clash_secret`**, **`type`: `secret`** (Clash API secret: behaves like `text` with a dedicated regenerate control). Use `text` / `enum` / `bool` / `text_list` for other variables. `wizard_ui: "fix"` means the variable is edited on a dedicated tab/flow (not Settings); `hidden` remains the generic "do not show on Settings" mode.

**Separator (layout only):** an entry **`{"separator": true}`** renders a horizontal rule on **Settings** between rows. Do **not** set **`name`**, **`type`**, **`default_value`**, **`default_node`**, **`options`**, **`title`**, **`tooltip`**, **`if`**, or **`if_or`**. Optional **`platforms`** (same as other vars) and **`wizard_ui`**: only **`hidden`** is allowed besides empty — to hide the rule on some OSes. Separators are not variables: no **`@`** placeholders and nothing is stored in wizard state. **macOS `.app`:** the running binary loads **`Contents/MacOS/bin/wizard_template.json`** inside the bundle — not the repo copy. After editing the template in the repository, **rebuild/reinstall** or **copy** the file into the app bundle so separators (and other JSON changes) appear.

#### macOS: turning `tun` off (wizard Settings)

**Platform:** **`darwin`** only. **Variable name:** the logic is tied to **`name`** exactly **`tun`** (stock **`bin/wizard_template.json`**).

When the user **unchecks** that bool (transition from on → off):

1. **Core must be stopped (launcher state):** if **`RunningState.IsRunning()`** is true — same “running” notion as the Core tab — the launcher shows a message and **reverts** the checkbox; the value in the wizard model is **not** changed to off.
2. **No stray sing-box process:** **`ProcessService.IsSingBoxProcessRunningOnSystem()`** (executable-name heuristic). If a process is still found, the checkbox is reverted — otherwise file cleanup could run while sing-box still holds **TUN** and ports (e.g. mixed **7890**), matching “address already in use” on the next start. This covers desync when **Stop** was cancelled at the admin password prompt: **`ProcessService.Stop`** must **not** clear **RunningState** if **`KillPrivilegedProcess`** fails (fixed behaviour).
3. **After the checks pass:** the launcher collects paths that may be **root-owned** after privileged TUN, then runs **one** privileged **`rm -rf`** over those that exist (same **AuthorizationRef** session as other privileged actions):
   - **experimental cache:** from the effective **`experimental`** section (**`ui/wizard/business/create_config.go`**: **`EffectiveConfigSection`**, same merge as preview via **`GetEffectiveConfig`**). **`core/config.ExperimentalCacheFileFromSection`** reads **`experimental.cache_file`**: if **`enabled`** is not **`false`** and **`path`** is non-empty, the path is resolved — relative paths are under the app **`bin/`** directory. **Only** if the resolved path is **inside** **`bin/`** and **exists**, it is added. Paths **outside** **`bin/`** are **skipped** (warn log).
   - **Core logs:** **`logs/sing-box.log`** and **`logs/sing-box.log.old`** (rotation) under **`ExecDir`** next to the launcher binary — fixed names (**`internal/constants`**: **`LogsDirName`**, **`ChildLogFileName`**); each path is added only if it **exists** and stays **under** **`ExecDir`** (safety check).
   If nothing is listed, no privileged call. If privileged removal **fails**, an error dialog is shown but the wizard **still** sets **`tun`** to off (user may fix permissions or delete files manually). If **core log files** were removed, **`FileService.ReopenChildLogFile()`** closes the stale descriptor and recreates/opens **`logs/sing-box.log`** so the next **Start** and the log viewer use the path on disk again.

Other bool **`vars`** names do **not** trigger this path.

#### Platform keys for `default_value` objects

**Scope:** This subsection is **only** for keys inside a **`vars[].default_value`** JSON **object**.

**Supported OS targets for this project** are **Windows**, **macOS**, and **Linux**. In docs and templates you can treat platform strings as:

- **`params[].platforms`**, **`selectable_rules[].platforms`**, **`vars[].platforms`**: **`windows`**, **`linux`**, **`darwin`** (must match **`runtime.GOOS`**). Do **not** use **`win7`** or **`default`** there. Implementation: **`matchesPlatform`** in **`ui/wizard/template/loader.go`**.

- **`default_value` object** (**`VarDefaultValue.ForPlatform`** / **`defaultValueKeyOrder`** in **`vars_default.go`**): keys are normalized to **lowercase**. Meaningful keys for shipped builds:

| Key | Role |
|-----|------|
| **`win7`** | Legacy Win7 x86 binary only (**`windows`/`386`**), checked **before** **`windows`**. |
| **`windows`**, **`linux`**, **`darwin`** | Same meaning as in **`platforms`** (**`GOARCH`** is not part of the key; no **`linux_amd64`**). |
| **`default`** | Fallback if nothing above matched or values were empty. |

**Lookup order:** **`win7`** (if **`windows`/`386`**) → current **`GOOS`** (**`windows`**, **`linux`**, or **`darwin`**) → **`default`**.

Other keys in the JSON (typos, old **`goos_goarch`**) are **ignored** for resolution. The engine still compares against whatever **`runtime.GOOS`** is if you run on another OS; this project’s releases and examples assume **linux / darwin / windows** only.

The stock app template **`bin/wizard_template.json`** includes a full example (`tun`, `tun_address`, `tun_mtu`, `mixed_listen_port`, `log_level`, `clash_api`, `clash_secret`).

At load time the wizard rejects a template if any **`@name`** in **`config`** or **`params[].value`** is missing from **`vars`**, if **`vars`** contains duplicate **`name`**, if **`params[].if`** / **`params[].if_or`** or **`vars[].if`** / **`vars[].if_or`** references a non-boolean variable, or if both **`if`** and **`if_or`** are set on the same **`params`** or **`vars`** entry.

---

### 5. `params` Section

**Purpose**: Defines platform-specific configuration overrides applied to `config` during template loading.

**Structure**:
```json
{
  "params": [
    {
      "name": "inbounds",
      "platforms": ["windows", "linux"],
      "value": [
        {
          "type": "tun",
          "tag": "tun-in",
          "interface_name": "singbox-tun0",
          "address": ["172.16.0.1/30"],
          "mtu": 1400,
          "auto_route": true,
          "strict_route": false,
          "stack": "system"
        }
      ]
    },
    {
      "name": "route.rules",
      "platforms": ["windows", "linux"],
      "mode": "prepend",
      "value": [
        { "inbound": "tun-in", "action": "resolve", "strategy": "prefer_ipv4" },
        { "inbound": "tun-in", "action": "sniff", "timeout": "1s" }
      ]
    }
  ]
}
```

**Fields**:
- `name` (required, string): Path to the config property using dot notation (e.g., `"inbounds"`, `"route.rules"`, `"dns.servers"`)
- `platforms` (required, array of strings): Platforms where this param applies (`"windows"`, `"linux"`, `"darwin"`)
- `value` (required): The value to apply (can be any JSON type: object, array, string, number, boolean)
- `mode` (optional, string): How to apply the value:
  - `"replace"` (default): Replace `config[name]` with `value`
  - `"prepend"`: Insert elements of `value` at the beginning of array `config[name]`
  - `"append"`: Append elements of `value` to the end of array `config[name]`

**Platform mapping**:
- `"windows"` → Windows (`runtime.GOOS == "windows"`)
- `"linux"` → Linux (`runtime.GOOS == "linux"`)
- `"darwin"` → macOS (`runtime.GOOS == "darwin"`)

**How it works**: During template loading, the wizard:
1. Determines the current platform (`runtime.GOOS`)
2. For each param, checks if the current platform is in `platforms`
3. If yes, applies `value` to `config` at path `name` using the specified `mode`

**Example**: To add TUN inbound only on Windows and Linux:
```json
{
  "name": "inbounds",
  "platforms": ["windows", "linux"],
  "value": [
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "singbox-tun0",
      "address": ["172.16.0.1/30"],
      "mtu": 1400,
      "auto_route": true,
      "strict_route": false,
      "stack": "system"
    }
  ]
}
```

To add system proxy inbound only on macOS:
```json
{
  "name": "inbounds",
  "platforms": ["darwin"],
  "value": [
    {
      "type": "mixed",
      "tag": "proxy-in",
      "listen": "127.0.0.1",
      "listen_port": 7890,
      "sniff": true,
      "sniff_override_destination": true,
      "set_system_proxy": true
    }
  ]
}
```

---

## Example Rules with Detailed Explanations

### Example 1: Rule with Outbound Selection

```json
{
  "label": "Gemini via Gemini VPN",
  "description": "Use dedicated Gemini VPN selector for Gemini rule set.",
  "default": true,
  "rule_set": [
    {
      "tag": "gemini",
      "type": "inline",
      "format": "domain_suffix",
      "rules": [
        {
          "domain_suffix": [
            "generativelanguage.googleapis.com",
            "gemini.google.com",
            "ai.google.dev"
          ]
        }
      ]
    }
  ],
  "rule": {
    "rule_set": "gemini",
    "network": ["tcp", "udp"],
    "outbound": "proxy-out"
  }
}
```

**How it works:**
- This rule has an `outbound` field (`"outbound": "proxy-out"`), so the wizard will show a dropdown selector
- Users can choose from:
  - Any outbound tag defined in `parser_config` (e.g., `proxy-out`, `auto-proxy-out`)
  - Any generated proxy tag from subscriptions (e.g., `🇳🇱Netherlands`, `🇺🇸USA`)
  - Always available options: `direct-out`, `reject`
- The `default: true` means this rule is checked/enabled by default when the wizard opens
- The `description` provides helpful context in the tooltip
- The `rule_set` array contains the rule set definition that will be added to `config.route.rule_set` only if this rule is enabled

### Example 2: Rule with Direct Outbound Default

```json
{
  "label": "Games direct",
  "description": "Send gaming rule set traffic directly for lower latency.",
  "default": true,
  "rule_set": [
    {
      "tag": "games",
      "type": "remote",
      "format": "binary",
      "url": "https://example.com/rules/games.srs",
      "download_detour": "direct-out",
      "update_interval": "24h"
    }
  ],
  "rule": {
    "rule_set": "games",
    "network": ["tcp", "udp"],
    "outbound": "direct-out"
  }
}
```

**How it works:**
- Uses `direct-out` by default (traffic bypasses VPN)
- Users can still change it in the wizard to route gaming traffic through VPN if needed
- The `network` field specifies both TCP and UDP protocols (gaming often uses UDP)
- Marked with `default: true` because low-latency direct routing is usually preferred for gaming

### Example 3: Blocking Rule with Reject Option

```json
{
  "label": "Block ads",
  "description": "Block advertising domains.",
  "rule_set": [
    {
      "tag": "ads",
      "type": "remote",
      "format": "binary",
      "url": "https://example.com/rules/ads.srs",
      "download_detour": "direct-out",
      "update_interval": "24h"
    }
  ],
  "rule": {
    "rule_set": "ads",
    "action": "reject",
    "method": "drop"
  }
}
```

**How it works:**
- This rule uses `"action": "reject"` to block ads by default
- The wizard automatically shows an "Outbound:" dropdown with **all available options**:
  - `reject` - Soft block (rejects connections) - selected by default
  - `drop` - Hard block (drops packets)
  - `direct-out` - Traffic goes directly, bypassing VPN (not recommended for ads)
  - Any proxy groups - Route through VPN (usually not needed)
- **Important:** Users can always change the rule behavior in the wizard by selecting any option from the list, regardless of what's specified in the template

**Rule Requirements:**
- All `selectable_rules` **must** have either an `outbound` field in the rule or an `action: reject` field
- Rules without these fields are not supported by the wizard and will not appear in the interface
- Rules with `action: resolve`, `action: sniff`, and other actions without `outbound` or `action: reject` should be placed in `config.route.rules` as regular rules (not in `selectable_rules`)

---

## DNS Configuration and Traffic Splitting

This section explains how DNS configuration works in templates and how to route DNS queries through different paths.

### Understanding DNS Servers Structure

In the example template, DNS servers are configured as follows:

```json
{
  "dns": {
    "servers": [
      {
        "type": "udp",
        "tag": "direct_dns_resolver",
        "server": "9.9.9.9",
        "server_port": 53
      },
      {
        "type": "https",
        "tag": "google_doh",
        "server": "8.8.8.8",
        "server_port": 443,
        "path": "/dns-query"
      },
      {
        "type": "https",
        "tag": "google_doh_vpn",
        "server": "8.8.8.8",
        "server_port": 443,
        "path": "/dns-query",
        "detour": "proxy-out"
      }
    ],
    "rules": [
      { "rule_set": "ru-domains", "server": "yandex_doh" },
      { "server": "google_doh" }
    ],
    "final": "direct_dns_resolver"
  }
}
```

**DNS Server Types:**

1. **`direct_dns_resolver`** - Standard UDP DNS (9.9.9.9 - Quad9)
   - Direct connection, no encryption
   - Fast and reliable fallback

2. **`google_doh`** - DNS-over-HTTPS (8.8.8.8)
   - Encrypted DNS queries
   - Direct connection (not through VPN)
   - Good for privacy and bypassing DNS censorship

3. **`google_doh_vpn`** - DNS-over-HTTPS through VPN
   - Same as `google_doh` but routed through `proxy-out` (via `detour`)
   - Use when you want DNS queries to appear from VPN location
   - Helps bypass DNS-based geo-blocking

**When to use DNS through VPN:**
- ✅ You want DNS queries to appear from VPN location
- ✅ You need to bypass DNS-based geo-restrictions
- ✅ Your ISP blocks certain DNS servers
- ✅ You want enhanced privacy for DNS queries

**When to use direct DNS:**
- ✅ Faster response times (no VPN overhead)
- ✅ Resolving local domains correctly (e.g., `.local`, `.lan`)
- ✅ Accessing services that require local DNS resolution
- ✅ Reducing VPN load for DNS queries

### Important DNS Settings

**`domain_strategy: "prefer_ipv4"`:**
- Forces IPv4 resolution even if IPv6 is available
- Prevents connection issues with services that prefer IPv4
- Recommended for better compatibility

**`strategy: "ipv4_only"`** (at DNS level):
- Only resolves to IPv4 addresses
- Useful if your network or VPN doesn't support IPv6
- Reduces DNS resolution overhead

**`independent_cache: false`:**
- DNS cache is shared across all DNS servers
- When you switch VPN servers, cached DNS results are still valid
- Set to `true` if you want separate cache per DNS server (usually not needed)

---

## TUN vs System Proxy (SOCKS/HTTP)

This section explains the difference between TUN mode and system proxy mode, and when to use each.

### TUN Mode (Virtual Network Interface)

**What it is:**
TUN creates a virtual network interface that captures **all traffic** from your system.

**Example configuration (via `params`):**
```json
{
  "name": "inbounds",
  "platforms": ["windows", "linux"],
  "value": [
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "singbox-tun0",
      "address": ["172.16.0.1/30"],
      "mtu": 1400,
      "auto_route": true,
      "strict_route": false,
      "stack": "system"
    }
  ]
}
```

**TUN Parameter Description:**

- **`type: "tun"`** - Interface type. **DO NOT CHANGE** - must always be `"tun"`.

- **`tag: "tun-in"`** - Tag for referencing in routing rules.
  - ✅ **Can be changed** to any unique name (e.g., `"my-tun"`)
  - ⚠️ **Important:** If changed, update all references to this tag in rules (`{ "inbound": "tun-in" }`)

- **`interface_name: "singbox-tun0"`** - Network interface name in the system.
  - ✅ **Can be changed** to any unique name (e.g., `"my-vpn"`)
  - ⚠️ **Important:** On Linux must start with `tun` (e.g., `tun0`, `tun1`)
  - 💡 Recommended to leave default to avoid conflicts

- **`address: ["172.16.0.1/30"]`** - IP address and subnet mask for the TUN interface.
  - ✅ **Can be changed** to another private range (e.g., `["10.0.0.1/30"]`, `["192.168.100.1/30"]`)
  - ⚠️ **Important:** Use only private ranges (RFC 1918):
    - `10.0.0.0/8` - `10.0.0.0` to `10.255.255.255`
    - `172.16.0.0/12` - `172.16.0.0` to `172.31.255.255`
    - `192.168.0.0/16` - `192.168.0.0` to `192.168.255.255`
  - `/30` means 4 addresses (usually sufficient)
  - 💡 If changed, ensure it doesn't conflict with your local network

- **`mtu: 1400`** - Maximum Transmission Unit (maximum packet size).
  - ✅ **Can be changed** in range `1280-1500` (recommended `1300-1450`)
  - 💡 Smaller MTU helps avoid packet fragmentation:
    - `1400` - universal option (good for most cases)
    - `1300` - if experiencing fragmentation issues
    - `1280` - minimum for IPv6, if compatibility needed
  - ❌ Don't set larger than `1500` (standard Ethernet MTU)

- **`auto_route: true`** - Automatically add routes for all traffic.
  - ❌ **DO NOT CHANGE** - must be `true` for full VPN functionality
  - If `false`, TUN will not intercept system traffic

- **`strict_route: false`** - Strict routing (force all traffic through TUN).
  - ✅ **Can be changed** to `true` in some cases:
    - `false` (default) - allows route bypass through rules
    - `true` - forcefully sends all traffic through TUN (stricter mode)
  - 💡 Recommended to leave `false` for more flexibility

- **`stack: "system"`** - Network stack for packet processing.
  - ❌ **DO NOT CHANGE** without necessity
  - `"system"` - uses system stack (recommended)
  - `"gvisor"` - uses gVisor (experimental, for special cases)
  - 💡 Leave `"system"` for stable operation

**When to use TUN:**
- ✅ You want **full system-wide VPN** (all applications automatically)
- ✅ You have administrator/root privileges
- ✅ You need transparent proxy for all traffic
- ✅ You want to protect all network activity
- ✅ You need to route traffic that doesn't support proxy settings

**Advantages:**
- ✅ Works with **all applications** (browsers, games, apps without proxy support)
- ✅ Transparent - applications don't need proxy configuration
- ✅ Captures all traffic automatically
- ✅ Best for full VPN experience

**Disadvantages:**
- ❌ Requires administrator/root privileges
- ❌ More complex setup
- ❌ May interfere with some network services
- ❌ Need proper routing rules to avoid breaking local network

### System Proxy (SOCKS/HTTP)

**What it is:**
System proxy mode runs a proxy server (SOCKS or HTTP) that applications connect to manually.

**SOCKS Proxy Example (via `params`):**
```json
{
  "name": "inbounds",
  "platforms": ["darwin"],
  "value": [
    {
      "type": "mixed",
      "tag": "proxy-in",
      "listen": "127.0.0.1",
      "listen_port": 7890,
      "sniff": true,
      "sniff_override_destination": true,
      "set_system_proxy": true
    }
  ]
}
```

**When to use System Proxy:**
- ✅ You don't have administrator/root privileges
- ✅ You want **selective proxying** (only specific applications)
- ✅ You need applications to explicitly use proxy
- ✅ You need **separate proxy and different settings for some applications** (different apps use different proxy servers)
- ✅ You want easier setup without TUN driver installation
- ✅ You're testing or need quick proxy setup

**Advantages:**
- ✅ No admin privileges required
- ✅ Selective - choose which apps use proxy
- ✅ Easier to set up and configure
- ✅ Can run multiple proxy instances on different ports
- ✅ Works on systems where TUN is not available

**Disadvantages:**
- ❌ Not all applications support proxy settings
- ❌ Need to configure each application manually
- ❌ Some applications ignore system proxy settings
- ❌ Less transparent - applications must explicitly support proxy
- ❌ DNS queries may not be routed through proxy (unless configured)

### Comparison Table

| Feature | TUN Mode | System Proxy |
|---------|----------|--------------|
| **Scope** | All system traffic | Selected applications only |
| **Setup Complexity** | Higher | Lower |
| **Admin Rights** | Required | Not required |
| **Application Support** | All apps (automatic) | Apps with proxy support only |
| **Transparency** | Full | Partial |
| **DNS Routing** | Automatic | Manual configuration needed |
| **Network Isolation** | Complete | Application-dependent |
| **Best For** | Full VPN experience | Selective proxying |

### Recommendation

- **For most users**: Use **TUN mode** for full VPN protection
- **For developers/testing**: Use **system proxy** for quick setup and testing
- **For restricted environments**: Use **system proxy** if admin rights unavailable

---

## Local Traffic Rules - Critical for Home Networks

This section explains why local traffic rules are essential and what happens without them.

### The Problem

Without proper local traffic rules, VPN users often experience:
- ❌ Cannot access router web interface (192.168.1.1, 192.168.0.1)
- ❌ Cannot access local network devices (NAS, printers, smart home devices)
- ❌ Local domain names don't work (`.local`, `.lan`)
- ❌ Home network file sharing breaks (SMB, DLNA)
- ❌ Smart home devices become unreachable
- ❌ Cannot configure network devices through web interface

### The Solution: Local Traffic Rules

Always include these critical rules in your template's `config.route.rules`:

```json
{
  "config": {
    "route": {
      "rules": [
        { "ip_is_private": true, "outbound": "direct-out" },
        { "domain_suffix": ["local"], "outbound": "direct-out" }
      ]
    }
  }
}
```

### Understanding the Rules

#### Rule 1: Private IP Addresses

```json
{ "ip_is_private": true, "outbound": "direct-out" }
```

**What it covers:**
- All private IPv4 ranges:
  - `192.168.0.0/16` (192.168.0.0 - 192.168.255.255)
  - `10.0.0.0/8` (10.0.0.0 - 10.255.255.255)
  - `172.16.0.0/12` (172.16.0.0 - 172.31.255.255)
- Router interfaces (e.g., 192.168.1.1, 192.168.0.1)
- Local network devices (NAS, printers, IP cameras)
- Other devices on your home network

**Why it's critical:**
- Without this rule, VPN tries to route local IPs through VPN server
- VPN servers typically don't have routes to your local network
- Result: **Cannot access anything on your local network**

#### Rule 2: Local Domain Suffixes

```json
{ "domain_suffix": ["local"], "outbound": "direct-out" }
```

**What it covers:**
- Domain names ending in `.local` (e.g., `printer.local`, `nas.local`)
- Domain names ending in `.lan` (if added: `["local", "lan"]`)
- mDNS (multicast DNS) resolved names
- Local hostnames discovered via network discovery

**Why it's critical:**
- Many devices use `.local` domains for automatic discovery
- Without this rule, `.local` domains may try to resolve through VPN DNS
- Result: **Local devices become unreachable by name**

### What Happens Without These Rules

**Scenario: User tries to access router (192.168.1.1)**

**Without local traffic rules:**
1. Request goes to VPN server
2. VPN server doesn't have route to 192.168.1.1
3. Connection fails or times out
4. ❌ **User cannot configure router**

**With local traffic rules:**
1. Rule matches `ip_is_private: true`
2. Traffic routed to `direct-out` (bypasses VPN)
3. Request goes directly to router on local network
4. ✅ **Router accessible**

### Understanding System Action Rules

At the beginning of the rules array, you typically place system rules with special actions that don't route traffic but perform preprocessing. These should be added via `params` for TUN mode:

```json
{
  "name": "route.rules",
  "platforms": ["windows", "linux"],
  "mode": "prepend",
  "value": [
    { "inbound": "tun-in", "action": "resolve", "strategy": "prefer_ipv4" },
    { "inbound": "tun-in", "action": "sniff", "timeout": "1s" }
  ]
}
```

#### `action: resolve` - DNS Resolution

```json
{ "inbound": "tun-in", "action": "resolve", "strategy": "prefer_ipv4" }
```

**What it does:**
- Performs DNS resolution (converts domain names to IP addresses) for traffic going through the TUN interface
- The `strategy: "prefer_ipv4"` parameter prefers IPv4 addresses even if IPv6 is available
- This allows sing-box to know the actual destination IP addresses **before** applying routing rules

**Why it's needed:**
- Without this rule, sing-box may not know IP addresses, which complicates routing
- Required for proper operation of other rules that work with IP addresses
- Improves compatibility and stability of VPN operation

#### `action: sniff` - Protocol Detection and Metadata Extraction

```json
{ "inbound": "tun-in", "action": "sniff", "timeout": "1s" }
```

**What it does:**
- Analyzes initial bytes of a connection and determines the protocol (HTTP, TLS, DNS, BitTorrent, etc.)
- Extracts metadata from traffic, for example:
  - **SNI (Server Name Indication)** from TLS handshake - the actual domain name even when connecting by IP address
  - Protocol type for proper routing
- The `timeout: "1s"` parameter limits analysis time - if the protocol is not determined within 1 second, the connection is processed without sniffing

**Why it's needed:**
- Allows using domain-based routing rules (`domain`, `domain_suffix`, `domain_keyword`) even when applications connect by IP address
- Without sniffing, sing-box only sees IP addresses in TUN mode, which limits routing capabilities
- Critically important for proper operation of domain-based rules

**Example:** If an application connects to `1.2.3.4`, but that's the IP for `google.com`, sniffing will extract the domain from the TLS handshake, and rules for `google.com` will work correctly.

#### `action: hijack-dns` - DNS Query Hijacking

```json
{ "protocol": "dns", "action": "hijack-dns" }
```

**What it does:**
- Intercepts DNS queries and routes them to DNS servers configured in the `dns` section
- Forces all DNS queries to use the configured DNS setup instead of system DNS
- The rule only triggers for traffic with the `dns` protocol

**Why it's needed:**
- Ensures all DNS queries are processed through configured DNS servers (DoH, DoT, specific servers for different domains)
- Without this rule, applications may use system DNS, bypassing your configuration
- Allows applying DNS rules from the `dns.rules` section for traffic splitting

**Important:** This rule should be placed **before** local traffic rules to ensure DNS queries are processed correctly.

### Recommendations for VPN Providers

⚠️ **CRITICAL:** Always include local traffic rules in your templates

1. **Never skip these rules** - Users will have broken home networks without them
2. **Place them early** in the rules array for reliable matching
3. **Document them** - Explain to users why these rules are important
4. **Test thoroughly** - Verify router access and local device connectivity

**Example template snippet:**
```json
{
  "config": {
    "route": {
      "rules": [
        { "protocol": "dns", "action": "hijack-dns" },
        { "ip_is_private": true, "outbound": "direct-out" },
        { "domain_suffix": ["local"], "outbound": "direct-out" }
      ]
    }
  },
  "params": [
    {
      "name": "route.rules",
      "platforms": ["windows", "linux"],
      "mode": "prepend",
      "value": [
        { "inbound": "tun-in", "action": "resolve", "strategy": "prefer_ipv4" },
        { "inbound": "tun-in", "action": "sniff", "timeout": "1s" }
      ]
    }
  ]
}
```

---

## Complete Example: Real-World Template

Here's a more complete example showing best practices:

```json
{
  "parser_config": {
    "ParserConfig": {
      "version": 4,
      "proxies": [
        {
          "source": "https://your-vpn-service.com/api/subscription?token=USER_TOKEN"
        }
      ],
      "outbounds": [
        {
          "tag": "auto-proxy-out",
          "type": "urltest",
          "options": {
            "url": "https://cp.cloudflare.com/generate_204",
            "interval": "5m",
            "tolerance": 100,
            "interrupt_exist_connections": true
          },
          "outbounds": {
            "proxies": {}
          },
          "comment": "Auto-select fastest proxy"
        },
        {
          "tag": "proxy-out",
          "type": "selector",
          "options": {
            "interrupt_exist_connections": true,
            "default": "auto-proxy-out"
          },
          "outbounds": {
            "addOutbounds": ["direct-out", "auto-proxy-out"],
            "proxies": {}
          },
          "comment": "Main proxy selector"
        }
      ],
      "parser": {
        "reload": "1h",
        "last_updated": "2026-01-30T12:00:00Z"
      }
    }
  },

  "config": {
    "log": {
      "level": "warn",
      "timestamp": true
    },
    "dns": {
      "servers": [
        {
          "type": "udp",
          "tag": "direct_dns",
          "server": "9.9.9.9",
          "server_port": 53
        },
        {
          "type": "https",
          "tag": "cloudflare_doh",
          "server": "1.1.1.1",
          "server_port": 443,
          "path": "/dns-query"
        }
      ],
      "rules": [
        { "server": "cloudflare_doh" }
      ],
      "final": "direct_dns"
    },
    "inbounds": [],
    "outbounds": [
      { "type": "direct", "tag": "direct-out" }
    ],
    "route": {
      "rule_set": [
        {
          "tag": "ru-domains",
          "type": "inline",
          "format": "domain_suffix",
          "rules": [
            { "domain_suffix": ["ru", "xn--p1ai"] }
          ]
        }
      ],
      "rules": [
        { "protocol": "dns", "action": "hijack-dns" },
        { "ip_is_private": true, "outbound": "direct-out" },
        { "domain_suffix": ["local"], "outbound": "direct-out" }
      ],
      "final": "proxy-out",
      "auto_detect_interface": true
    },
    "experimental": {
      "clash_api": {
        "external_controller": "127.0.0.1:9090",
        "secret": "CHANGE_THIS_TO_YOUR_SECRET_TOKEN"
      }
    }
  },

  "selectable_rules": [
    {
      "label": "Block Ads",
      "description": "Soft-block ads by rejecting connections (recommended)",
      "default": true,
      "rule_set": [
        {
          "tag": "ads-all",
          "type": "remote",
          "format": "binary",
          "url": "https://raw.githubusercontent.com/v2fly/domain-list-community/release/geosite-category-ads-all.srs",
          "download_detour": "direct-out",
          "update_interval": "24h"
        }
      ],
      "rule": {
        "rule_set": "ads-all",
        "action": "reject"
      }
    },
    {
      "label": "Russian domains direct",
      "description": "Route Russian domains directly (faster for local services)",
      "rule": {
        "rule_set": "ru-domains",
        "outbound": "direct-out"
      }
    },
    {
      "label": "Gaming traffic direct",
      "description": "Route gaming traffic directly for lower latency",
      "default": true,
      "rule_set": [
        {
          "tag": "games",
          "type": "remote",
          "format": "binary",
          "url": "https://example.com/rules/games.srs",
          "download_detour": "direct-out",
          "update_interval": "24h"
        }
      ],
      "rule": {
        "rule_set": "games",
        "outbound": "direct-out"
      }
    }
  ],

  "params": [
    {
      "name": "inbounds",
      "platforms": ["windows", "linux"],
      "value": [
        {
          "type": "tun",
          "tag": "tun-in",
          "interface_name": "singbox-tun0",
          "address": ["172.16.0.1/30"],
          "mtu": 1400,
          "auto_route": true,
          "strict_route": false,
          "stack": "system"
        }
      ]
    },
    {
      "name": "route.rules",
      "platforms": ["windows", "linux"],
      "mode": "prepend",
      "value": [
        { "inbound": "tun-in", "action": "resolve", "strategy": "prefer_ipv4" },
        { "inbound": "tun-in", "action": "sniff", "timeout": "1s" }
      ]
    },
    {
      "name": "inbounds",
      "platforms": ["darwin"],
      "value": [
        {
          "type": "mixed",
          "tag": "proxy-in",
          "listen": "127.0.0.1",
          "listen_port": 7890,
          "sniff": true,
          "sniff_override_destination": true,
          "set_system_proxy": true
        }
      ]
    }
  ]
}
```

---

## Best Practices

### 1. Always Include Static Outbounds

Keep at least `direct-out` in `config.outbounds`. Users need it for local traffic and as a fallback option. Generated proxy outbounds are automatically inserted at the beginning of this array.

### 2. Use Clear Labels and Descriptions

Good labels help users understand what each rule does:
- ✅ `"label": "Block Ads"` 
- ❌ `"label": "Rule 1"`

Good descriptions explain the impact:
- ✅ `"description": "Soft-block ads by rejecting connections instead of dropping packets"`
- ❌ `"description": "Blocks ads"`

### 3. Set Sensible Defaults

Use `"default": true` for rules that most users should enable:
- Ad blocking
- Common geo-routing rules
- Performance optimizations

Don't use `default: true` for:
- Experimental features
- Rules that might break common services
- Region-specific rules (unless you're targeting that region)

### 4. Match Tag Names

Ensure all `tag` values referenced in `route.rules` exist in:
- Your `parser_config` outbounds, OR
- Static outbounds in `config.outbounds`, OR
- Will be generated from subscriptions

### 5. Validate Your JSON

Your template must be valid JSON. Test it:
```bash
# Using jq (if installed)
cat wizard_template.json | jq . > /dev/null

# Or use an online JSON validator
```

### 6. Keep Section Order Logical

The wizard preserves your section order. Organize logically:
1. `parser_config` - Parser configuration
2. `config` - Main sing-box configuration
3. `dns_options` - Wizard DNS list defaults (`servers` / `rules`, optional)
4. `selectable_rules` - User-selectable rules
5. `params` - Platform-specific parameters

### 7. Use `rule_set` Wisely

- Include `rule_set` definitions in `selectable_rules` only if they're specific to that rule
- Place shared rule sets (used by multiple rules or DNS) in `config.route.rule_set`
- This ensures rule sets are only loaded when their rules are enabled

### 8. Platform-Specific Rules

Use the `platforms` field in `selectable_rules` to create platform-specific rules:
- Same `label` with different `platforms` and rule content
- Example: Windows uses `.exe` process names, macOS/Linux use process names without extension

---

## Testing Your Template

### Step 1: Place the File

Put `wizard_template.json` in the `bin/` folder next to the executable.

### Step 2: Launch the Wizard

1. Start the application
2. Open the Config Wizard (usually from the main menu or Tools tab)
3. Verify the template loads without errors

### Step 3: Test User Flow

1. **Tab 1 (Sources & Parser)**:
   - Enter a test subscription URL or direct link
   - Click "Check" - should validate successfully
   - Click "Parse" - should generate outbounds preview

2. **Tab 2 (Rules)**:
   - After first launch, presets with `"default": true` appear as rows; **Add from library** lists all `selectable_rules` presets for appending copies
   - Check outbound dropdowns and per-rule enable checkbox
   - Reorder with ↑/↓ if needed
   - Verify platform-filtered presets behave as expected on each OS

3. **Tab 3 (Preview)**:
   - Click "Show Preview"
   - Verify the generated config looks correct
   - Check that selected rules are included
   - Verify generated proxy outbounds are inserted correctly
   - Verify platform-specific `params` are applied correctly

4. **Save**:
   - Click "Save"
   - Verify `config.json` is created
   - Check that old config was backed up (if existed)
   - Verify `experimental.clash_api.secret` was generated

---

## Troubleshooting

### Template Not Loading

**Problem**: Wizard shows "Template file not found" or JSON errors.

**Solutions**:
- Ensure file is named exactly `wizard_template.json` (case-sensitive)
- Ensure file is in `bin/` folder
- Validate JSON syntax (use `jq` or online validator)
- Check for trailing commas or syntax errors
- Ensure all required sections (`parser_config`, `config`, `selectable_rules`, `params`) are present; optional `dns_options` may provide DNS list defaults (`servers` / `rules`)

### Generated Outbounds Not Appearing

**Problem**: After parsing, no outbounds show in preview.

**Solutions**:
- Verify subscription URL is accessible
- Verify subscription format is supported (vless://, vmess://, trojan://, ss://)
- Check logs in `logs/` folder for parsing errors
- Verify `parser_config` structure is correct

### Rules Not Showing in Wizard

**Problem**: Expected presets do not appear on the Rules tab or in **Add from library**.

**Solutions**:
- Verify JSON structure is valid
- Ensure `label` and `description` fields are present
- Check that rule has either `outbound` field or `action: reject`
- Verify `platforms` field (if used) matches current platform
- Check logs for parsing errors

### Outbound Tags Not Found

**Problem**: Wizard shows "outbound not found" errors.

**Solutions**:
- Verify all referenced tags exist in `parser_config` outbounds
- Ensure `final` tag exists in `config.route.final`
- Check that generated proxies will have matching tags (if using tag filters)
- Add missing outbounds to static section in `config.outbounds`

### Platform-Specific Settings Not Applied

**Problem**: Platform-specific `params` are not being applied.

**Solutions**:
- Verify `platforms` array uses correct values: `"windows"`, `"linux"`, `"darwin"`
- Check that `name` field uses correct dot notation path
- Verify `mode` is correct (`"replace"`, `"prepend"`, `"append"`)
- Ensure JSON structure is valid

---

## Advanced Tips

### Dynamic Subscription URLs

If users need to enter their own subscription URL, leave `source` empty or use a placeholder:
```json
{
  "parser_config": {
    "ParserConfig": {
      "proxies": [
        { "source": "" }
      ]
    }
  }
}
```

Users will enter their URL in the wizard's first tab.

### Multiple Subscription Sources

Support multiple subscriptions:
```json
{
  "parser_config": {
    "ParserConfig": {
      "proxies": [
        { "source": "https://provider1.com/subscription" },
        { "source": "https://provider2.com/subscription" }
      ]
    }
  }
}
```

Users can add more in the wizard interface.

### Conditional Rules Based on Outbound Selection

When a rule has `outbound`, users can choose from available tags. Make sure your `parser_config` defines all options users might need.

### Custom Rule Sets

Reference remote rule sets in `selectable_rules`:
```json
{
  "label": "Block ads",
  "rule_set": [
    {
      "tag": "ads-all",
      "type": "remote",
      "format": "binary",
      "url": "https://example.com/rules/ads.srs",
      "download_detour": "direct-out",
      "update_interval": "24h"
    }
  ],
  "rule": {
    "rule_set": "ads-all",
    "action": "reject"
  }
}
```

Rule sets are only loaded when the rule is enabled.

### Shared Rule Sets

If multiple rules or DNS rules use the same rule set, define it in `config.route.rule_set`:
```json
{
  "config": {
    "route": {
      "rule_set": [
        {
          "tag": "ru-domains",
          "type": "inline",
          "format": "domain_suffix",
          "rules": [
            { "domain_suffix": ["ru", "xn--p1ai"] }
          ]
        }
      ]
    }
  }
}
```

Then reference it in `selectable_rules` without defining it again:
```json
{
  "label": "Russian domains direct",
  "rule": {
    "rule_set": "ru-domains",
    "outbound": "direct-out"
  }
}
```

---

## Distribution

When distributing your customized launcher:

1. Include `wizard_template.json` in your release package
2. Place it in `bin/` folder
3. Users will automatically use it when opening the Config Wizard
4. Consider documenting your specific rules and options in a separate README

---

## Need Help?

- **Template syntax issues**: Check this guide's examples
- **sing-box configuration**: See [official sing-box docs](https://sing-box.sagernet.org/configuration/)
- **ParserConfig format**: See `ParserConfig.md` in this repository
- **Report bugs**: Open an issue on [GitHub](https://github.com/Leadaxe/singbox-launcher/issues)

---

**Note**: This wizard template system is designed to make configuration easier for end users. As a provider, you maintain full control over the default configuration while giving users flexibility to customize their setup. The unified template structure eliminates platform-specific files and comment-based directives, making templates easier to maintain and validate.


