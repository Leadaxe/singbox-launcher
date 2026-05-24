# SPEC 053-F-N — PRESET_BUNDLES

**Status:** New (N)
**Type:** Feature (F)
**Depends on:** SPEC 045 (state/config decoupling), SPEC 046 (pinned RequiredTemplateRef), SPEC 052 (connections redesign).
**Bump:** state.json v5 → v6. schema = `"presets_v1"`.

---

## Цель

Превратить `selectable_rules[]` из шаблона в **self-contained параметризованные пресеты**: каждый пресет несёт свои `vars`, `rule_set`, `rule`, `dns_servers`, `dns_rule`. Тэги rule_set'ов и DNS-серверов **локальны внутри preset'а** — при build авто-префиксируются `<preset_id>:<tag>`. Глобального namespace тэгов и магических ссылок `"rule_set": "ru-domains"` в шаблоне больше нет.

`state.rules[]` хранит **тонкие ссылки** на пресеты — `{kind: "preset", ref, enabled, body: {vars}}`. Match-поля и rule_set'ы лежат в шаблоне, а не в state. Bump `RequiredTemplateRef` → юзеры автоматически получают обновлённые match-поля (расширенный список TLD, новые блок-листы) без действий с их стороны.

Параллельно с preset-ref'ами поддерживаются user-defined rule'ы (`kind: "inline"`, `kind: "srs"`) — полный контент в state, как сейчас в `CustomRules`.

## Не-цели

- **Не** меняем структуру sing-box `config.json` (`route`, `dns`, `outbounds` остаются sing-box-совместимыми).
- **Не** меняем `SPEC 052 connections` (`connections.sources/outbounds/defaults` без изменений).
- **Не** делаем UI-редактор пресетов — пресеты пишутся в `bin/wizard_template.json`, правится из репозитория.
- **Не** делаем импорт/экспорт пользовательских пресетов как формат — отдельная фича если понадобится.
- **Не** делаем reactive обновление varsValues в runtime — любое изменение → reconfigure sing-box, как сейчас.

---

## Финальный layout `wizard_template.json` (presets section)

```jsonc
{
  "globals":      [ /* template vars: tun, cert_store, route_final, … — без изменений */ ],
  "outbounds":    [ /* selectors, urltests, direct-out — без изменений */ ],

  "dns_defaults": {
    "strategy":                "prefer_ipv4",
    "independent_cache":       true,
    "final":                   "google_doh",
    "default_domain_resolver": "google_doh",
    "servers": [
      { "tag": "google_doh",      "type": "https", "server": "dns.google",
        "server_port": 443, "path": "/dns-query", "default_enabled": true },
      { "tag": "cloudflare_udp",  "type": "udp",   "server": "1.1.1.1",
        "server_port": 53,  "default_enabled": false },
      { "tag": "yandex_doh",      "type": "https", "server": "77.88.8.88",
        "server_port": 443, "path": "/dns-query", "default_enabled": true }
    ]
  },

  "presets": [
    /* массив self-contained preset'ов; пример каждой формы — ниже */
  ]
}
```

### Удалённые/мигрированные секции

| Было в template | Стало |
|---|---|
| `route.rule_set[]` (массив глобальных определений с tag'ами `ru-domains`, `ads-all`) | Удалено. rule_set'ы живут только внутри `presets[i].rule_set[]`. |
| `dns_options.servers[].enabled` | Переименовано в `default_enabled` (template-рекомендация). Реальное состояние enable/disable хранится в `state.dns.template_servers[tag].enabled` (override). |
| `dns_options.rules` | Удалено. DNS-rules собираются из active presets `dns_rule` + `state.dns.extra_rules`. |
| `selectable_rules[]` (примитивные с label/description/rule) | Заменено на `presets[]` (self-contained с vars/rule_set/dns_servers/dns_rule). |

---

## Preset structure

### Go-репрезентация

```go
type Preset struct {
    ID             string         `json:"id"`              // stable slug, e.g. "ru-direct"
    Label          string         `json:"label"`
    Description    string         `json:"description,omitempty"`
    DefaultEnabled bool           `json:"default_enabled,omitempty"`

    Vars        []PresetVar       `json:"vars,omitempty"`
    RuleSet     []PresetRuleSet   `json:"rule_set,omitempty"`
    DNSServers  []PresetDNSServer `json:"dns_servers,omitempty"`

    Rule        map[string]any    `json:"rule,omitempty"`     // routing rule
    DNSRule     map[string]any    `json:"dns_rule,omitempty"` // dns rule
}

type PresetVar struct {
    Name     string         `json:"name"`               // identifier
    Type     string         `json:"type"`               // "outbound"|"dns_server"|"enum"|"text"|"number"|"bool"
    Default  string         `json:"default"`            // always required
    Title    string         `json:"title,omitempty"`    // UI label
    Tooltip  string         `json:"tooltip,omitempty"`

    // For type=enum — list of {title, value} pairs (object form mandatory).
    // For type=dns_server — list of tag strings whitelisting picker contents.
    // For type=outbound — list of outbound tag strings whitelisting picker.
    // Schema: []OptionEntry (enum) or []string (dns_server/outbound).
    Options  json.RawMessage `json:"options,omitempty"`

    // Shortcut for typical scopes (only for type=dns_server).
    //   "local"  — only tags bundled in this preset (preset.dns_servers[].tag).
    //   "global" — all available (bundled ∪ template effective_enabled ∪ extras).
    //   default (omit) → "global".
    // Mutually exclusive with `options`. If both set — options wins + warning.
    // For type=outbound `select` is ignored (no concept of "local" outbounds
    // in a preset — outbounds are always template-global).
    Select   string         `json:"select,omitempty"`

    If       []string       `json:"if,omitempty"`       // var visible/active iff ALL listed bool vars are true
    IfOr     []string       `json:"if_or,omitempty"`    // var visible/active iff AT LEAST ONE listed bool var is true
}

type OptionEntry struct {
    Title string `json:"title"`
    Value string `json:"value"`
}

type PresetRuleSet struct {
    Tag    string `json:"tag"`                     // local tag, прицепится <preset_id>:<tag>
    Type   string `json:"type"`                    // "inline" | "remote"
    Format string `json:"format,omitempty"`        // "domain_suffix" | "binary"
    Rules  []map[string]any `json:"rules,omitempty"` // for type=inline
    URL    string `json:"url,omitempty"`           // for type=remote

    If     []string `json:"if,omitempty"`
    IfOr   []string `json:"if_or,omitempty"`
}

type PresetDNSServer struct {
    Tag         string `json:"tag"`                // local, prefixes to <preset_id>:<tag>
    Type        string `json:"type"`               // "udp" | "https" | "tls" | "h3"
    Server      string `json:"server"`
    ServerPort  int    `json:"server_port,omitempty"`
    Path        string `json:"path,omitempty"`
    TLS         map[string]any `json:"tls,omitempty"`
    Detour      string `json:"detour,omitempty"`   // outbound tag or @varname

    // UI-only label, stripped at emit. Used in dns_server picker dropdown.
    // Fallback to Tag if empty.
    Title       string `json:"title,omitempty"`

    // Passed through to config.json — valid sing-box DNS server field.
    // Shows up in debug views / config inspection / logs.
    Description string `json:"description,omitempty"`

    If   []string `json:"if,omitempty"`
    IfOr []string `json:"if_or,omitempty"`
}
```

### Var types

| `type` | UI control | Substitution value | `options` поле | Constraints |
|---|---|---|---|---|
| `outbound` | dropdown: outbound-теги + `reject` + `drop` | tag-строка | `[]string` whitelist; пропущен → все available + reject/drop | `reject`/`drop` — зарезервированные литералы |
| `dns_server` | grouped dropdown (3 секции) или plain whitelist | tag-строка (локальная, build префиксует bundled) | `[]string` whitelist; пропущен → bundled ∪ template (effective_enabled) ∪ extras | label = `title` bundled DNS, fallback на `tag` |
| `enum` | dropdown с `{title, value}` | `value`-строка | `[]OptionEntry`, обязателен | default ∈ options.value |
| `text` | text entry | строка | — | — |
| `number` | numeric entry | строка-число | — | — |
| `bool` | checkbox | `"true"` / `"false"` | — | — |

**`options` для `dns_server` / `outbound`:**
- если **пропущен** → picker показывает все available (grouped для dns_server, flat для outbound)
- если **задан** → picker показывает только перечисленные tag'и, в порядке `options`, без секционных заголовков
- `default` обязан быть ∈ `options` если последний задан

**`select` shortcut только для `dns_server`** (mutually exclusive с `options`):
- `"global"` (или omit) → все available
- `"local"` → только bundled из preset'а (`preset.dns_servers[].tag`)
- При конфликте `select + options` → `options` wins + warning
- Для `type: outbound` поле `select` игнорируется + warning — у preset'а нет concept'а "local outbounds" (outbound'ы всегда template-global)

**Все vars required.** Поля `optional` / `required: false` нет. Опциональность достигается через `if` / `if_or` на vars и фрагментах.

### `if` / `if_or` семантика

Семантика **идентична** существующей `TemplateVar.If` / `IfOr` (core/template/vars_resolve.go):

- `if: ["use_yandex_dns"]` — активно если **все** перечисленные bool vars `true` (AND)
- `if_or: ["tun", "tun_builtin"]` — активно если **хотя бы одна** bool var `true` (OR)
- Обе пустые / отсутствуют → всегда активно

**Где можно использовать:**

| Место | `if` поддерживается? |
|---|---|
| `presets[i].vars[j]` | да — скрывает var-row в UI edit dialog, исключает var из varsMap при substitute |
| `presets[i].rule_set[j]` | да — фрагмент не эмитится при false |
| `presets[i].dns_servers[j]` | да — фрагмент не эмитится при false |
| `presets[i].dns_rule` | да — целиком не эмитится при false |
| `presets[i].rule` | да — целиком не эмитится при false (preset функционально disabled при текущих vars) |

Имена в `if` / `if_or` — это имена **bool vars** из `preset.vars[]` (локальная область). Глобальные template-vars (tun, log_level и т.д.) в preset scope **не доступны** — пресет должен быть self-contained.

### Vars scope — preset vs global

Глобальные template vars (`template.vars[]` — `cert_store`, `tun`, `tun_mtu`, `route_final`, ...) и preset vars (`preset.vars[]`) — **раздельные пространства имён**.

| Контекст substitute | Видимый scope |
|---|---|
| Top-level template (`route`, `dns`, `outbounds`, ...) | только `template.vars[]` |
| Внутри preset (`preset.rule_set/rule/dns_rule/dns_servers`) | только `preset.vars[]` |

**Cross-scope доступ запрещён.** В preset нельзя сослаться `@cert_store` (глобальная) — это намеренно: preset должен быть self-contained.

### Коллизия имён

Если автор preset'а назвал свою var так же как глобальная (например `preset.vars` содержит `name: "tun"`):

- Load: **warning** `"preset '<id>': var '<name>' shadows global template var; consider renaming"`
- Substitute продолжает работать корректно: внутри preset `@tun` резолвится по локальному, не по глобальному
- На глобальном уровне `@tun` продолжает работать как раньше (preset не влияет)

Warning — диагностика автору preset'а, не error. Preset остаётся валидным.

### Tag scoping

`tag` внутри `preset.rule_set[]` и `preset.dns_servers[]` **локален** в preset'е. Допускается одноимённые tag'и в разных пресетах (`messengers.telegram`, `social.telegram`).

При build все локальные tag'и переименовываются `<preset_id>:<local_tag>`. Ссылки в `rule.rule_set` / `dns_rule.rule_set` / `dns_rule.server` / `dns_server.detour` через **локальные имена** — build переписывает на префиксованные.

Префикс автоматический, юзер его никогда не пишет вручную.

---

## Формы preset'а — примеры

### Форма 1: inline match, без rule_set'ов

```jsonc
{
  "id": "private-ips-direct",
  "label": "Private IPs direct",
  "description": "Route LAN traffic directly.",
  "default_enabled": true,
  "vars": [
    { "name": "out", "type": "outbound", "default": "direct-out", "title": "Outbound" }
  ],
  "rule": { "ip_is_private": true, "outbound": "@out" }
}
```

### Форма 2: один rule_set + bundled DNS под условием

Учебный пример. Демонстрирует: один rule_set, `type: bool` toggle, `type: enum`, `if` на var и фрагменте, один bundled DNS-сервер.

```jsonc
{
  "id": "ru-direct-mini",
  "label": "Russian domains direct (mini)",
  "default_enabled": true,
  "vars": [
    { "name": "out", "type": "outbound", "default": "direct-out", "title": "Outbound" },
    { "name": "use_yandex_dns", "type": "bool", "default": "true",
      "title": "Use Yandex DNS for these domains" },
    { "name": "dns_ip", "type": "enum", "default": "77.88.8.88",
      "if": ["use_yandex_dns"], "title": "UDP server IP",
      "options": [
        { "title": "Safe (77.88.8.88)",  "value": "77.88.8.88" },
        { "title": "Family (77.88.8.7)", "value": "77.88.8.7"  }
      ] }
  ],
  "rule_set": [
    { "tag": "domains", "type": "inline", "format": "domain_suffix",
      "rules": [{ "domain_suffix": ["ru","xn--p1ai","su","moscow"] }] }
  ],
  "dns_servers": [
    { "tag": "yandex_udp", "type": "udp", "server": "@dns_ip",
      "server_port": 53, "detour": "@out",
      "if": ["use_yandex_dns"] }
  ],
  "rule":     { "rule_set": "domains", "outbound": "@out" },
  "dns_rule": { "rule_set": "domains", "server": "yandex_udp",
                "if": ["use_yandex_dns"] }
}
```

### Форма 3: multi rule_set, OR-композиция

```jsonc
{
  "id": "ru-blocked",
  "label": "Russian blocked resources",
  "default_enabled": false,
  "vars": [
    { "name": "out", "type": "outbound", "default": "proxy-out", "title": "Outbound" }
  ],
  "rule_set": [
    { "tag": "main",      "type": "remote", "format": "binary",
      "url": "https://.../geoip-ru-blocked.srs" },
    { "tag": "community", "type": "remote", "format": "binary",
      "url": "https://.../geoip-ru-blocked-community.srs" }
  ],
  "rule": { "rule_set": ["main", "community"],
            "network": ["tcp", "udp"], "outbound": "@out" }
}
```

### Форма 4: selective — rule использует все rule_set'ы, dns_rule только часть

```jsonc
{
  "id": "messengers",
  "label": "Messengers",
  "vars": [
    { "name": "out", "type": "outbound", "default": "proxy-out" }
  ],
  "rule_set": [
    { "tag": "telegram", "type": "remote", "url": "..." },
    { "tag": "whatsapp", "type": "remote", "url": "..." },
    { "tag": "signal",   "type": "remote", "url": "..." }
  ],
  "rule":     { "rule_set": ["telegram","whatsapp","signal"], "outbound": "@out" },
  "dns_rule": { "rule_set": ["telegram","whatsapp"],          "server": "google_doh" }
}
```

### Форма 5: reject через зарезервированный литерал

```jsonc
{
  "id": "block-ads",
  "label": "Block ads",
  "default_enabled": true,
  "vars": [
    { "name": "out", "type": "outbound", "default": "reject", "title": "Action" }
  ],
  "rule_set": [
    { "tag": "ads", "type": "remote", "format": "binary",
      "url": "https://.../geosite-category-ads-all.srs" }
  ],
  "rule": { "rule_set": "ads", "outbound": "@out" }
}
```

---

## Real-world example — `ru-direct`

Адаптация LxBox `ru-direct` preset'а в наш формат. Этот пример **обязательная reference-имплементация**: любой expansion engine должен корректно резолвить ru-direct во всех описанных ниже режимах. Тесты должны включать его как golden fixture.

Демонстрирует одновременно: multi rule_set (3 шт.) · `if` на var, rule_set элементе, dns_server, dns_rule · `type: dns_server` с `select: "local"` · `type: enum` с {title, value} · `type: bool` с tooltip'ом · `@var` substitute в server/detour/outbound · selective rule_set ref (rule использует 3 set'а, dns_rule только 2 — у IP нет DNS) · `description` для DNS picker'а · filter bundled DNS-серверов через `@dns_server`.

### Preset definition

```jsonc
{
  "id": "ru-direct",
  "label": "Russian domains & IPs",
  "description": "Route Russian TLDs (.ru/.su/IDN), service CDNs (Yandex/VK/Avito/2GIS/banks/etc) and Russian IP ranges directly.",
  "default_enabled": true,

  "vars": [
    { "name": "out", "type": "outbound", "default": "direct-out",
      "title":   "Outbound",
      "tooltip": "Where to route traffic for Russian domains and IPs." },

    { "name": "use_dns_override", "type": "bool", "default": "true",
      "title":   "Use Yandex DNS for these domains",
      "tooltip": "Override DNS for matched domains. Disable to resolve via global DNS." },

    { "name": "dns_server", "type": "dns_server", "default": "yandex_udp",
      "if": ["use_dns_override"],
      "select": "local",
      "title":   "DNS server",
      "tooltip": "Recommended: UDP — most stable. DoH/DoT may be filtered by ISPs." },

    { "name": "dns_ip", "type": "enum", "default": "77.88.8.8",
      "if": ["use_dns_override"],
      "title":   "UDP server IP",
      "tooltip": "Applies to UDP only. DoH and DoT are pinned to 77.88.8.88 with SNI safe.dot.dns.yandex.net.",
      "options": [
        { "title": "77.88.8.8 · Base",                     "value": "77.88.8.8" },
        { "title": "77.88.8.1 · Base alt",                 "value": "77.88.8.1" },
        { "title": "2a02:6b8::feed:0ff · Base v6",         "value": "2a02:6b8::feed:0ff" },
        { "title": "77.88.8.88 · Safe",                    "value": "77.88.8.88" },
        { "title": "77.88.8.2 · Safe alt",                 "value": "77.88.8.2" },
        { "title": "2a02:6b8::feed:bad · Safe v6",         "value": "2a02:6b8::feed:bad" },
        { "title": "2a02:6b8:0:1::feed:bad · Safe v6 alt", "value": "2a02:6b8:0:1::feed:bad" },
        { "title": "77.88.8.7 · Family",                   "value": "77.88.8.7" },
        { "title": "77.88.8.3 · Family alt",               "value": "77.88.8.3" },
        { "title": "2a02:6b8::feed:a11 · Family v6",       "value": "2a02:6b8::feed:a11" }
      ] },

    { "name": "geoip_enabled", "type": "bool", "default": "true",
      "title":   "GeoIP IP-range fallback",
      "tooltip": "Match Russian-AS IPs as well as domains — catches CDN/QUIC/ECH cases where domain detection fails. Requires downloading geoip-ru.srs (~150 KB) via the cloud icon next to the rule. Disable to avoid the download." }
  ],

  "rule_set": [
    { "tag": "ru-domains", "type": "inline", "format": "domain_suffix",
      "rules": [{ "domain_suffix": [
        "ru", "su", "xn--p1ai", "xn--p1acf", "xn--80adxhks", "moscow",
        "tatar", "xn--d1acj3b", "xn--80asehdb", "xn--80aswg",
        "xn--c1avg", "xn--j1aef"
      ] }] },

    { "tag": "ru-services", "type": "inline", "format": "domain_suffix",
      "rules": [{ "domain_suffix": [
        "userapi.com", "avito.st", "yandex.net", "yandex.com", "yastatic.net",
        "2gis.com", "okko.tv", "premier.one", "lenta.com", "vk.com",
        "vk-portal.net", "gismeteo.com", "lmru.tech", "mradx.net",
        "wbstatic.net", "wildberries.by", "trbcdn.net", "sberbank.com"
      ] }] },

    { "tag": "geoip-ru", "type": "remote", "format": "binary",
      "url": "https://raw.githubusercontent.com/runetfreedom/russia-v2ray-rules-dat/release/sing-box/rule-set-geoip/geoip-ru.srs",
      "if": ["geoip_enabled"] }
  ],

  "dns_servers": [
    { "tag": "yandex_udp", "type": "udp",   "server": "@dns_ip",
      "server_port": 53,  "detour": "@out",
      "title":       "Yandex UDP",
      "description": "Yandex public DNS over UDP (port 53). IP from var dns_ip.",
      "if": ["use_dns_override"] },

    { "tag": "yandex_doh", "type": "https", "server": "77.88.8.88",
      "server_port": 443, "path": "/dns-query",
      "tls": { "enabled": true, "server_name": "safe.dot.dns.yandex.net" },
      "detour": "@out",
      "title":       "Yandex DoH",
      "description": "Yandex Safe DoH (77.88.8.88, SNI safe.dot.dns.yandex.net).",
      "if": ["use_dns_override"] },

    { "tag": "yandex_dot", "type": "tls",   "server": "77.88.8.88",
      "server_port": 853,
      "tls": { "enabled": true, "server_name": "safe.dot.dns.yandex.net" },
      "detour": "@out",
      "title":       "Yandex DoT",
      "description": "Yandex Safe DoT (77.88.8.88:853, SNI safe.dot.dns.yandex.net).",
      "if": ["use_dns_override"] }
  ],

  "rule":     { "rule_set": ["ru-domains", "ru-services", "geoip-ru"],
                "outbound": "@out" },

  "dns_rule": { "rule_set": ["ru-domains", "ru-services"],
                "server":   "@dns_server",
                "if": ["use_dns_override"] }
}
```

### Expansion case 1 — default varsValues

State: `{kind: "preset", ref: "ru-direct", enabled: true, body: {vars: {}}}`.

varsMap (всё дефолты):
```
out               = "direct-out"
use_dns_override  = "true"   → все if=["use_dns_override"] активны
dns_server        = "yandex_udp"
dns_ip            = "77.88.8.8"
geoip_enabled     = "false"  → if=["geoip_enabled"] неактивен — geoip-ru НЕ эмитится
                              (opt-in: юзер сам включает чтобы скачать ~150KB .srs)
```

Emit в `config.json`:

```jsonc
// route.rule_set[] — только 2 inline set'а; geoip-ru отброшен (if false)
{ "tag": "ru-direct:ru-domains",  "type": "inline", "format": "domain_suffix",
  "rules": [{ "domain_suffix": ["ru","su",...] }] },
{ "tag": "ru-direct:ru-services", "type": "inline", "format": "domain_suffix",
  "rules": [{ "domain_suffix": ["userapi.com","avito.st",...] }] },

// route.rules[] — ссылка на geoip-ru удалена из массива (dangling ref clean)
{ "rule_set": ["ru-direct:ru-domains","ru-direct:ru-services"],
  "outbound": "direct-out" },

// dns.servers[] — filter оставил только yandex_udp (выбран через @dns_server)
//                detour удалён т.к. @out резолвится в direct-out
//                title стрипнут, description прокинут как есть
{ "tag": "ru-direct:yandex_udp", "type": "udp",
  "server": "77.88.8.8", "server_port": 53,
  "description": "Yandex public DNS over UDP (port 53). IP from var dns_ip." },

// dns.rules[]
{ "rule_set": ["ru-direct:ru-domains","ru-direct:ru-services"],
  "server":   "ru-direct:yandex_udp" }
```

Yandex DoH/DoT — bundled, но не выбраны через `@dns_server` → **не эмитятся** (filter шаг 8 expand'а).

### Expansion case 2 — `use_dns_override: false`

State: `{kind: "preset", ref: "ru-direct", enabled: true, body: {vars: {"use_dns_override": "false"}}}`.

varsMap:
```
use_dns_override = "false"  → all if=["use_dns_override"] всё false
geoip_enabled    = "false"  → geoip-ru тоже отключён по умолчанию
```

Что выкидывается:
- vars `dns_server`, `dns_ip` — отфильтрованы (не входят в varsMap, не используются при substitute)
- все три `dns_servers[*]` с `if=["use_dns_override"]` — отброшены
- `dns_rule` — целиком отброшен
- `rule_set[2]` (geoip-ru с `if=["geoip_enabled"]`) — отброшен (см. case 1 rationale)

Emit:

```jsonc
// route.rule_set[] — только 2 inline set'а (geoip-ru off by default)
// route.rules[]    — один, ссылается на 2 set'а
{ "rule_set": ["ru-direct:ru-domains","ru-direct:ru-services"],
  "outbound": "direct-out" }

// dns.servers[] — preset не добавил, только template defaults
// dns.rules[]   — preset не добавил
```

DNS этих доменов резолвится через `state.dns.final` (global).

### Expansion case 3 — `geoip_enabled: true` (opt-in IP fallback)

State: `{... body: {vars: {"geoip_enabled": "true"}}}` (остальное дефолт).

varsMap:
```
geoip_enabled = "true"   → if=["geoip_enabled"] true → geoip-ru эмитится
use_dns_override = "true" → DNS-bundle включён
```

Юзер явно включил GeoIP fallback — launcher должен скачать `geoip-ru.srs` (~150 KB) и
эмитить rule_set с `type: "local"` + path к скачанному файлу.

Emit:

```jsonc
// route.rule_set[] — 3 set'а
{ "tag": "ru-direct:ru-domains",  "type": "inline", ... },
{ "tag": "ru-direct:ru-services", "type": "inline", ... },
{ "tag": "ru-direct:geoip-ru",    "type": "local",  "format": "binary",
  "path": "<execDir>/bin/rule-sets/<sha-tag>.srs" }

// route.rules[]
{ "rule_set": ["ru-direct:ru-domains","ru-direct:ru-services","ru-direct:geoip-ru"],
  "outbound": "direct-out" }

// dns.servers[] / dns.rules[] — как в case 1
```

### Expansion case 4 — юзер выбрал `yandex_doh`

State: `{... body: {vars: {"dns_server": "yandex_doh"}}}` (остальное дефолт).

Меняется только filter dns_servers:
```jsonc
// dns.servers[] — теперь yandex_doh вместо yandex_udp
{ "tag": "ru-direct:yandex_doh", "type": "https",
  "server": "77.88.8.88", "server_port": 443, "path": "/dns-query",
  "tls": { "enabled": true, "server_name": "safe.dot.dns.yandex.net" } },

// dns.rules[]
{ "rule_set": ["ru-direct:ru-domains","ru-direct:ru-services"],
  "server":   "ru-direct:yandex_doh" }
```

`yandex_udp` и `yandex_dot` — bundled, не выбраны → не эмитятся.

---

## Финальный layout `state.json` (rules section)

```jsonc
{
  "meta": { "version": 6, "schema": "presets_v1", "created_at": "...", "updated_at": "..." },
  "connections": { /* SPEC 052 — без изменений */ },

  "rules": [
    { "kind": "preset",
      "ref": "ru-direct",
      "enabled": true,
      "body": { "vars": { "dns_ip": "77.88.8.7" } } },

    { "kind": "preset",
      "ref": "block-ads",
      "enabled": false,
      "body": { "vars": {} } },

    { "kind": "inline",
      "id": "01J9X...A",
      "enabled": true,
      "body": {
        "name":  "Firefox через VPN",
        "match": { "domain_suffix": ["example.com"],
                   "package_name":  ["org.mozilla.firefox"] },
        "outbound": "proxy-out"
      } },

    { "kind": "srs",
      "id": "01J9X...B",
      "enabled": true,
      "body": {
        "name":     "Custom block list",
        "srs_url":  "https://example.com/blocklist.srs",
        "outbound": "reject"
      } }
  ],

  "dns": {
    "strategy":                "prefer_ipv4",
    "independent_cache":       true,
    "final":                   "google_doh",
    "default_domain_resolver": "google_doh",

    "template_servers": {
      "cloudflare_udp": { "enabled": true  },
      "yandex_doh":     { "enabled": false }
    },

    "extra_servers": [
      { "tag": "my-pihole", "type": "udp", "server": "192.168.1.5", "server_port": 53 }
    ],
    "extra_rules": []
  }
}
```

**`template_servers`** — overrides для template-defined DNS-серверов. Хранятся **только tag'и где юзер изменил дефолт**. Отсутствие ключа = берём `default_enabled` из template.

### Удалено из v5

| Поле | Куда делось |
|---|---|
| `custom_rules` | Переименовано в `rules`; формат расширен kind discriminator'ом |
| `selectable_rule_states` | Не существует — preset-ref сам в `rules[]` |
| `config_params` | Удалено — все vars живут в `preset.body.vars` (per-preset) или `state.dns.*` (DNS-global) |
| `vars` (top-level) | Глобальные template vars — без изменений (тот же массив, но называется не как rules) |
| `dns_options` (null или legacy) | Заменено `dns` с extra_servers/extra_rules |

### Go-репрезентация

```go
type Rule struct {
    Kind    string          `json:"kind"`              // "preset" | "inline" | "srs"
    Ref     string          `json:"ref,omitempty"`     // kind=preset
    ID      string          `json:"id,omitempty"`      // kind=inline|srs (ULID)
    Enabled bool            `json:"enabled"`
    Body    json.RawMessage `json:"body"`              // decoded по Kind switch
}

type PresetBody struct {
    Vars map[string]string `json:"vars"`               // diff от template defaults
}

type InlineBody struct {
    Name     string         `json:"name"`
    Match    map[string]any `json:"match"`             // sing-box match-объект
    Outbound string         `json:"outbound"`          // tag | "reject" | "drop"
}

type SrsBody struct {
    Name     string `json:"name"`
    SrsURL   string `json:"srs_url"`
    Outbound string `json:"outbound"`                  // tag | "reject" | "drop"
}

type DNSConfig struct {
    Strategy              string                       `json:"strategy"`
    IndependentCache      bool                         `json:"independent_cache"`
    Final                 string                       `json:"final"`
    DefaultDomainResolver string                       `json:"default_domain_resolver"`
    TemplateServers       map[string]TemplateServerOvr `json:"template_servers,omitempty"`
    ExtraServers          []map[string]any             `json:"extra_servers"`
    ExtraRules            []map[string]any             `json:"extra_rules"`
}

type TemplateServerOvr struct {
    Enabled bool `json:"enabled"`
}
```

### Header / Body разделение

| Header | Body |
|---|---|
| `kind` (discriminator, required) | kind-specific payload |
| identifier (`ref` или `id` по kind) | без identifier'а |
| `enabled` (общий toggle) | — |

**Парсер dispatcher:**

```go
func DecodeRule(raw json.RawMessage) (Rule, error) {
    var h RuleHeader
    json.Unmarshal(raw, &h)
    switch h.Kind {
    case "preset": h.decodeBody(&PresetBody{})
    case "inline": h.decodeBody(&InlineBody{})
    case "srs":    h.decodeBody(&SrsBody{})
    default:       return Rule{}, errSkipWithWarning("unknown kind")
    }
}
```

Жёсткий контракт. Unknown kind → skip + warning. Missing required body field → skip + warning. Extra unknown body field → strip + warning.

---

## Build pipeline

### Общая схема

```
template.presets[]  +  state.rules[]  +  state.dns.*  ──────►  config.json
                                                                │
        ┌───────────────────────────────────────────────────────┼──────────────────────────────────┐
        │                                                       │                                  │
  route.rule_set[]                                        route.rules[]                          dns.*
  ─────────────────                                       ─────────────────                       ────────
  {bundled из active preset-refs}                         {hardcoded {protocol: dns,             servers = dns_defaults.servers
  +                                                        action: hijack-dns} в начале}                  + {bundled dns_servers active presets}
  {user inline headless rule_sets}                        +                                                + state.dns.extra_servers
  +                                                       {по одному rule из каждого
  {user srs local-binary rule_sets                         active preset-ref в                  rules = {bundled dns_rules active presets}
   (path к скачанным .srs файлам)}                         порядке state.rules}                          + state.dns.extra_rules
                                                          +
                                                          {по одному rule из каждого            strategy/final/default_domain_resolver
                                                           active user rule}                              = state.dns.*
```

### Expand preset-ref (`kind: preset`)

1. **Lookup.** Найти `template.presets[]` по `ref`. Не нашли → **broken-preset**: skip + warning `"preset '<ref>' not found in template"`. В UI tile показывает marker.
2. **Build varsMap.** Для каждой `template.preset.vars[j]`:
   - значение из `state.rule.body.vars[name]` если есть и не пустое — используется
   - иначе `default` из template
3. **Filter vars by `if`/`if_or`.** Проверить каждую var против `varsMap`. Var с false-условием удаляется из `varsMap` — substitute её плейсхолдера ниже фейлится (см. validate).
4. **Deep copy fragments.** `rule_set`, `rule`, `dns_rule`, `dns_servers` — deep copy, чтобы не мутировать template.
5. **Filter fragments by `if`/`if_or`.** Каждый элемент `rule_set[]`/`dns_servers[]` и сами `rule`/`dns_rule` проверяются. False-условие → фрагмент выкидывается до substitute.
6. **Substitute `@varname`.** Рекурсивный обход JSON-tree. Каждая строка `"@name"` заменяется на `varsMap[name]`. Если `name` не в varsMap (отфильтрована шагом 3 или опечатка) → build error, preset skip + warning `"unresolved var '@<name>'"`.
7. **Prefix local tags.** Все `rule_set[i].tag` и `dns_servers[i].tag` → `<preset_id>:<local_tag>`. Ссылки в `rule.rule_set` / `dns_rule.rule_set` / `dns_rule.server` / `dns_servers[i].detour` (если ссылка на bundled DNS) переписать.
8. **Filter dns_servers** (критично для `type: dns_server` var'ы).
   Bundled DNS-сервер попадает в emit **только если**:
   - его tag (после префиксования) равен резолву `@dns_server`-var'ы preset'а, **или**
   - его tag упомянут литералом в `preset.dns_rule.server` (без `@`)
   Остальные bundled DNS — мёртвый код, **не эмитятся**.

   Резолв var типа `dns_server`:
   ```
   varValue = varsMap["dns_server"]      // локальное имя в state
   if varValue ∈ preset.dns_servers[].tag:
       resolved = "<preset_id>." + varValue   // bundled → prefix
   else:
       resolved = varValue                     // template default / extra → as is
   ```
9. **Resolve outbound sentinels.** В `rule.outbound`:
   - `"reject"` → `rule.action = "reject"`, удалить `outbound`
   - `"drop"` → `rule.action = "reject"`, `rule.method = "drop"`, удалить `outbound`
   - tag → оставить как `outbound`
   Реализуется через существующий `ApplyOutboundToRule` (`ui/configurator/business/rule_utils.go`).
10. **Emit fragments** в общие массивы `config.json`.

### Expand user inline (`kind: inline`)

1. **Headless rule_set:** emit `{tag: "user.<id>", type: "inline", rules: [body.match]}` в `route.rule_set[]`.
2. **Route rule:** `{rule_set: "user.<id>", outbound: body.outbound}` в `route.rules[]`. Apply outbound sentinels.

### Expand user srs (`kind: srs`)

1. **Lookup cached path.** `RuleSetDownloader.cachedPath(rule.id)`. Не скачано → skip + warning `"srs rule '<name>' skipped: no cached file"`, UI tile показывает «Download required».
2. **Local rule_set:** `{tag: "user.<id>", type: "local", format: "binary", path: "<execDir>/bin/rule-sets/<id>.srs"}` в `route.rule_set[]`.
3. **Route rule:** `{rule_set: "user.<id>", outbound: body.outbound}`. Apply outbound sentinels.

### Merge стратегия

- **rule_set** по emitted tag'у:
  - identical-skip (один tag + deep-equal content) — тихо
  - real conflict (один tag + разный content) — first-wins + warning
- **dns_servers** по emitted tag'у: те же правила.
- **dns_rules / route_rules**: append в порядке `state.rules[]`. Tag'ов нет.

### Порядок в final `config.json`

- `route.rules[]` = `[{protocol: "dns", action: "hijack-dns"}]` (template-hardcoded) + emitted rules в порядке state.rules[] + template-hardcoded fallback (если есть)
- `route.rule_set[]` = emitted rule_set'ы (порядок неважен, sing-box матчит по tag)
- `dns.servers[]` = `dns_defaults.servers[].filter(effective_enabled)` + bundled active + `state.dns.extra_servers`
- `dns.rules[]` = bundled active dns_rules + `state.dns.extra_rules`

### `effective_enabled(tag)` resolve

```
effective_enabled(tag) = state.dns.template_servers[tag]?.enabled
                     ?? template.dns_defaults.servers[tag].default_enabled
                     ?? true
```

При emit поле `default_enabled` из template-сервера **strip'ается** (это не sing-box поле).

---

## UI

### Rules tab — список

```
┌─ Rules ───────────────────────────────────────────────────────┐
│ [+ Add rule]  [+ From library]                       Outbound  │
│ ─────────────────────────────────────────────────────────────  │
│ ↑↓ ☑ Private IPs direct                            [Direct ▾] │
│ ↑↓ ☑ Russian domains direct · Yandex DNS · Family             │
│                                                    [Direct ▾] │
│ ↑↓ ☐ Russian blocked resources · 2 sets            [proxy-out▾]│
│ ↑↓ ☑ Block ads                                     [Reject ▾] │
│ ↑↓ ☑ Firefox через VPN · .example.com              [proxy-out▾]│
│ ↑↓ ☑ Custom block list · srs                       [Reject ▾] │
│                                                                │
│ Final outbound:  [proxy-out ▾]                                │
└───────────────────────────────────────────────────────────────┘
```

- Tile: drag, enable, label, summary (только non-default varsValues / match-resume), outbound picker.
- Outbound picker всегда показан. Литералы `reject`/`drop` отображаются как «Reject» / «Drop», остальное — tag.
- Multi rule_set counter — `· 2 sets`.
- srs marker — `· srs` (плюс кнопка ⬇ download если файл не скачан).
- Tap на tile → edit dialog.

### Edit dialog — preset-ref (`kind: preset`)

```
┌─ Russian domains direct ─────────────────┐
│ 📎 From template (preset: ru-direct)     │
│ Route Russian & Cyrillic TLDs directly.  │
│                                          │
│ Outbound:                       [Direct▾]│
│ ☑ Use Yandex DNS for these domains       │
│ UDP server IP:          [Family 77.88.8.7▾]│   ← виден only when ☑ выше
│                                          │
│ ▾ Preview                                │
│   route.rule_set:   [...]                │
│   route.rules:      [...]                │
│   dns.servers:      [...]                │
│   dns.rules:        [...]                │
│                                          │
│ [Delete rule]                  [Cancel] [Save]  │
└──────────────────────────────────────────┘
```

- Match-поля **не показаны и не редактируются**. Match-контент живёт в template.
- Vars рендерятся универсально по `type`. `if`-условия скрывают зависимые row'ы.
- **Broken preset** (ref отсутствует в текущем template) → vars скрыты, warning баннер + кнопка `[Delete]`. Конверсии нет (snapshot match-полей в preset-ref не хранится).

### Edit dialog — user inline (`kind: inline`)

Полная форма match-полей / outbound — как сейчас в `dialogs/add_rule_dialog.go`. По сути без изменений, но без legacy `selectable_rule` импорта (он теперь preset-ref'ом).

### Edit dialog — user srs (`kind: srs`)

Форма URL + download кнопка + outbound — как сейчас.

### DNS tab

```
┌─ DNS ─────────────────────────────────────────────────────────┐
│ Strategy:                [prefer_ipv4 ▾]                       │
│ Independent cache:       [✓]                                   │
│ Final:                   [google_doh ▾]                        │
│ Default domain resolver: [google_doh ▾]                        │
│                                                                │
│ ─── Default DNS servers (from template) ───                    │
│   [☑] google_doh      · https · dns.google                     │
│   [☑] cloudflare_udp  · udp   · 1.1.1.1     (overridden)       │
│   [☐] yandex_doh      · https · 77.88.8.88  (overridden)       │
│                                                                │
│ ─── From active presets (read-only) ───                        │
│   ru-direct:yandex_doh · https · 77.88.8.88                    │
│   ru-direct:yandex_udp · udp   · 77.88.8.7  (from var dns_ip)  │
│                                                                │
│ ─── Extra servers (user-defined) ───  [+ Add server]           │
│   my-pihole · udp · 192.168.1.5    [edit] [delete]             │
│                                                                │
│ ─── Extra rules (user-defined JSON) ───                        │
│   [JSON multi-line editor]                                     │
└────────────────────────────────────────────────────────────────┘
```

- Источник каждого DNS-сервера виден: template / active preset / user extra.
- Чекбоксы у template-серверов пишут в `state.dns.template_servers[tag].enabled`. Marker `(overridden)` — если значение чекбокса отличается от template-default'а.
- `final` / `default_domain_resolver` — dropdown по union'у всех эффективно-enabled tag'ов.

### Library dialog

Каталог `template.presets[]`. Для каждого:
- Label + description
- Кнопка **Add to Rules** (или «Already added» disabled, если ref уже в `state.rules[]`)
- При добавлении создаётся `{kind: "preset", ref, enabled: default_enabled, body: {vars: {}}}` — varsValues пустой, дефолты приходят из template

---

## Validation

### Template (`wizard_template.json`) — на load

| Проверка | Действие при ошибке |
|---|---|
| `preset.id` уникален в `presets[]` | duplicate → ignore second + warning |
| `preset.id` ∈ `[a-z0-9-]+` | invalid → ignore preset + warning |
| `preset.vars[].name` уникальны в preset | duplicate → ignore second + warning |
| `preset.rule_set[].tag` уникальны в preset | duplicate → ignore preset + warning |
| `preset.dns_servers[].tag` уникальны в preset | duplicate → ignore preset + warning |
| `preset.vars[].default` ∈ `options.value` для type=enum | mismatch → ignore preset + warning |
| `preset.vars[].default` ∈ resolved scope (options или select=local или global) для type=dns_server/outbound | mismatch → ignore preset + warning |
| `preset.vars[].select` ∈ {"local", "global"} | unknown → strip select + warning, fall back to global |
| `preset.vars[].select` задан для type ≠ dns_server | strip + warning (только dns_server поддерживает select) |
| `preset.vars[].select` + `options` оба заданы | options wins + warning |
| `preset.rule.rule_set` ссылается на существующие local tags | unknown ref → ignore preset + warning |
| `preset.dns_rule.rule_set` / `dns_rule.server` ссылки разрешимы | unknown → ignore preset + warning |
| `preset.vars[].if/if_or` ссылается на существующие vars типа `bool` | unknown / wrong type → ignore preset + warning |

### State (`state.json`) — на load

| Проверка | Действие |
|---|---|
| `rule.kind` ∈ {preset, inline, srs} | unknown → skip + warning |
| `kind=preset` имеет `ref`, не имеет `id` | violation → strip extras + warning |
| `kind=inline/srs` имеет `id`, не имеет `ref` | violation → strip extras + warning |
| `id` уникален среди `kind=inline/srs` | duplicate → keep first + warning |
| `ref` уникален среди `kind=preset` | duplicate → keep first + warning |
| `body.vars` keys ∈ `template.presets[ref].vars[].name` | unknown → strip + warning |
| `body.outbound` (inline/srs) ∈ available outbounds ∪ {reject, drop} | unknown → reset to `direct-out` + warning |
| `state.dns.final` ∈ union эффективно-enabled DNS tags | unknown → reset to template `dns_defaults.final` + warning |
| `state.dns.template_servers[tag]` — `tag` существует в `template.dns_defaults.servers[]` | unknown tag → strip + warning (template-server удалён в новой версии) |

### Build-time (per active rule)

| Проверка | Действие |
|---|---|
| preset-ref: `ref` находится в template | not found → broken-preset, skip rule + warning |
| preset-ref: substitute `@name` находит var в varsMap | unresolved → skip preset + warning |
| user srs: cached file существует | missing → skip rule + warning |
| outbound tag (sentinels учтены) существует в `config.outbounds` | unknown → skip rule + warning |

---

## Edge cases

### `@out == "direct-out"` в `dns_servers[i].detour`

`direct-out` — direct outbound, не требует detour'а (sing-box резолвит без forwarding'а через default_domain_resolver). Build после substitute удаляет ключ `detour` из dns_server'а, если значение резолвится в `"direct-out"`.

### Bundled DNS-сервер сконфликтовал с `state.dns.extra_servers` по tag'у

Bundled tag всегда префиксован `<preset_id>:<tag>`. User extra_servers tag валидируется по `[a-z0-9_-]+` (символ `:` запрещён). Конфликт **невозможен по построению**.

### Цикл в `@var`

Невозможен — vars резолвятся в **примитивные значения** (string/number/bool), не в другие vars. Substitute one-level only.

### Dangling rule_set reference

Когда `if`-filter убрал rule_set элемент (например `geoip-ru` при `geoip_enabled=false`), а `rule.rule_set` / `dns_rule.rule_set` ссылается на его tag — после tag-prefix step ссылка проверяется на наличие в emitted set'ах. Отсутствующие tag'и **удаляются из массива** (filter, не error).

- Массив `rule_set` стал пустым → `rule` целиком отбрасывается + warning.
- Хотя бы один tag остался → rule эмитится с урезанным `rule_set` массивом.

Это позволяет писать `rule: { rule_set: ["main", "optional"], outbound: "@out" }` где `optional` под `if`, и rule корректно работает в обоих режимах.

### Шаблон обновился, vars изменились

После bump'а template:
- Var удалена → юзерский `varsValues[name]` стрипается на следующем load с warning.
- Var переименована → юзерский `varsValues[old_name]` стрипается, новая var получает default'ное значение.
- Var изменила type → `varsValues[name]` стрипается + warning, новая var получает default.
- Default изменился → если юзер не трогал var (нет ключа в `varsValues`), автоматически получает новый default. Если трогал — его значение сохраняется (это явный intent).

Это **естественное поведение** thin-ref модели: bump = подтянули новое, но юзерский explicit choice уважается.

### Broken preset (ref не существует в текущем template)

- В Rules tab tile показан с marker'ом `⚠ Broken preset`. Outbound picker disabled, кнопка enable disabled.
- В config.json правило **автоматически отбрасывается** (skip + warning в build log).
- Edit dialog: vars скрыты, warning баннер, единственная кнопка `[Delete]`.
- **Конверсии нет.** preset-ref не хранит snapshot match-полей — нечего конвертировать. Если template вернётся с тем же `ref` (например пользователь обновил template на корректную версию) — правило снова работает.

### Двойной active rule одного preset_id

Запрещено валидацией state: дубликат `ref` → keep first + warning. См. SPEC выше «`ref` уникален среди `kind=preset`».

### User-edited `state.dns.extra_rules`

Эмитятся **в дополнение** к bundled dns_rules. Не отменяют их. Юзер видит в DNS tab «From active presets (read-only)» и «Extra rules (user-defined)» — две явные секции, никаких пересечений.

---

## Migration (state.json v5 → v6)

### Одноразовая на первом load v6-launcher'а

1. **Read** `state.json` с `meta.version == 5`.
2. **Map** `custom_rules[]` → `rules[]`:
   - `kind` derive из текущего `custom_rules[i].rule` структуры:
     - Имеет `srs_url` или `rule_set[0].type == "remote"` → `kind: "srs"`
     - Иначе → `kind: "inline"`
   - `id` = generate new ULID
   - `body.name` = current `label`
   - `body.match` = current `rule` (без `outbound`)
   - `body.outbound` = current `selected_outbound`
   - `body.srs_url` = first `rule_set[0].url` для srs
3. **Map** `selectable_rule_states[]` (если есть legacy после SPEC 027) → `kind: "preset"` ref'ы:
   - Совпадение по preset.label (text-match с template) → создать `{kind: "preset", ref: <preset_id>, enabled, body: {vars: {}}}`
   - Не нашли соответствия → конвертировать как `kind: "inline"` с snapshot'ом match-полей
4. **Map** legacy `dns_options.servers`:
   - tag совпадает с template-defined → если `enabled` отличается от template `default_enabled` → пишем override в `state.dns.template_servers[tag] = {enabled}`
   - tag не совпадает (user-added сервер) → копируется в `state.dns.extra_servers`
5. **Bump** `meta.version = 6`, `meta.schema = "presets_v1"`, `meta.updated_at = now`.
6. **Backup** old `state.json` → `state.json.v5.bak` рядом.

Миграция **идемпотентна**: повторный запуск на уже v6-state — noop. Backup создаётся только при первом upgrade'е.

### `RequiredTemplateRef` bump в release pipeline

После merge нового template в `main` — обновить `internal/constants/RequiredTemplateRef` через CI ldflags (SPEC 046 §5.2) на новый commit hash. Юзерские preset-ref правила **автоматически** подтянут обновлённые match-поля на следующем rebuild'е.

---

## Тестирование

### Unit tests

| Файл | Что проверяет |
|---|---|
| `core/build/preset_expand_test.go` | substitute с разными varsMap'ами, if/if_or filtering, tag prefixing, dns_servers filter, outbound sentinels |
| `core/build/preset_expand_test.go` (cont.) | broken preset (unknown ref), unresolved @var, required-var missing |
| `core/build/merge_test.go` | identical-skip / real-conflict для rule_set и dns_servers по tag'у |
| `core/template/preset_validate_test.go` | template-side validation (uniqueness, refs resolvable) |
| `ui/configurator/models/rule_test.go` | header/body разделение, JSON roundtrip для трёх kind'ов |
| `core/state/migrate_v5_v6_test.go` | миграция: custom_rules → rules[], selectable_rule_states → preset-refs |

### Golden tests (`core/build/testdata/`)

- `preset-ru-direct.json` — состояние state с ru-direct preset → ожидаемый config.json
- `preset-multi-rule-set.json` — ru-blocked с двумя rule_set'ами → emit
- `preset-if-fragment-dropped.json` — `use_yandex_dns: false` → dns_rule/dns_servers выкинуты
- `preset-broken-ref.json` — ref на несуществующий preset → правило не эмитится
- `mixed-preset-and-user.json` — преcет-ref + user inline + user srs в одном state → правильный merge

### Integration

| Сценарий | Ожидание |
|---|---|
| Bump template (расширили `domain_suffix`) + перезапуск launcher'а | preset-ref правила автоматически получили новые match-поля в config; varsValues юзера не потеряны |
| Disable preset-ref в UI | tile показывает disabled, в config.json правило отсутствует |
| Bump template, preset переименовали (ref сменился) | старая preset-ref становится broken; UI показывает warning + кнопка `[Delete]`; правило автоматически skip в emit'е до удаления юзером |
| Юзер задал var via UI, потом дефолт в template поменялся | varsValues юзера сохраняются (явный choice) |

---

## Acceptance

- [ ] `state.json` v6 формат, миграция v5 → v6 идемпотентна, backup сохранён
- [ ] `template.presets[]` парсится, validation reject'ит broken entries
- [ ] Build pipeline эмитит правильный `config.json` для всех 5 форм preset'а (golden tests)
- [ ] `if`/`if_or` работает на vars и фрагментах (rule_set, dns_servers, dns_rule, rule)
- [ ] Tag prefixing автоматический, коллизий между preset'ами нет
- [ ] Rules tab показывает все три kind'а; outbound picker в строке tile
- [ ] Edit dialog preset-ref не показывает match-поля; vars рендерятся по type
- [ ] DNS tab показывает источник каждого сервера (template / preset / extra); template-серверы переключаются через checkbox, override пишется в state.dns.template_servers
- [ ] Broken preset — UI marker + warning, правило автоматически skip в emit'е, единственная UI-кнопка `[Delete]`
- [ ] `reject` / `drop` в outbound работают (apply через существующий `ApplyOutboundToRule`)
- [ ] sing-box принимает эмитнутый config (no unknown fields, no orphan tag refs)
- [ ] RELEASE_PROCESS.md §5.2 описывает bump `RequiredTemplateRef` после merge новых preset'ов
- [ ] 30+ unit-тестов зелёные; golden testdata покрывает 5 форм

---

## Not in scope

- **UI-редактор пресетов в самом launcher'е** — пресеты пишутся в репозитории, не из приложения. Юзер видит каталог в Library dialog, но не правит preset.body.
- **Импорт/экспорт user preset bundles** (json-файл) — отдельная фича если когда-то понадобится.
- **Reactive обновление vars без reconfigure** — любое изменение → reconfigure sing-box, как сейчас.
- **Конверсия preset-ref ↔ user inline/srs** — не поддерживается ни в одном направлении. Юзер либо использует preset-ref (тонкая ссылка на template), либо создаёт user-defined rule с нуля. Это явный выбор при `[+ Add rule]` / `[+ From library]`.
- **Namespacing tag'ов user-defined правил** (сейчас `user.<id>`) — достаточно текущего префикса.
- **Globals → preset vars cross-reference** — preset должен быть self-contained, globals в `if` не доступны.
- **Per-preset cache invalidation на template bump** — все preset-ref'ы инвалидируются вместе с config.json через `RebuildConfigIfDirty` после load v6-state.

---

## Memory / invariants

- **Tag namespace локален в preset'е** — глобальных tag'ов не существует. Юзер никогда не пишет `<preset_id>:<tag>` руками.
- **state.rules[] kind=preset строго `{kind, ref, enabled, body: {vars}}`** — никаких лишних полей. Любые extras при load → strip + warning.
- **Template — immutable, pin'ится через `RequiredTemplateRef`** (SPEC 046). Юзер никогда не правит шаблон через UI.
- **Bump template → расширение у всех юзеров автоматически** — это и есть главная ценность thin-ref модели.
- **Substitute = тупая текстовая замена** — никаких `_Dropped` sentinel'ов, никакого cascade сброса. Опциональность через `if`/`if_or`.
- **Все vars required** — никаких `optional` флагов. Default — обязателен.
- **`if`/`if_or` ссылаются только на bool vars из того же preset'а** — глобальные template-vars не доступны в preset scope.

---

## Execution — fix UI integration (2026-05-13)

Первая попытка интеграции (зафиксирована в первом раунде IMPLEMENTATION_REPORT) была кривой:

### Что было сделано неправильно

1. **Две секции в Library** — рендерил legacy `selectable_rules[]` и новые `presets[]` как **отдельные** секции с separator'ом и header'ом «Parametrized presets (SPEC 053)». Юзер видит дубликаты по смыслу (одни и те же Block Ads / Russian domains в двух местах) и внутренний термин «SPEC 053» в UI.

2. **Параллельные структуры в model** — `model.CustomRules []*RuleState` (legacy для inline/srs) и `model.PresetRefs []*PresetRefState` (новый для kind=preset) живут как два независимых массива. Это значит:
   - В Rules tab они рендерятся **раздельно** (`buildCustomRuleRows` + `buildPresetRefRows`), с UI-разделителем и header'ом «Preset-based rules (SPEC 053)»
   - Drag ↑↓ работает только **внутри** своей секции — нельзя поднять preset-rule над inline-rule
   - **Нет единого порядка** правил — он состоит из двух последовательностей, не из одной

3. **Build pipeline эмитит fragments в конце** — `MergePresetsIntoRoute` делает append поверх результата `MergeRouteSection`, игнорируя любую попытку упорядочить правила между типами. В config.json `route.rules[]` всегда: hijack-dns → legacy CustomRules → preset fragments. Юзер не контролирует this.

4. **Параллельное хранение `selectable_rules[]` и `presets[]` в template** — оба раздела сосуществуют. Юзер сказал прямо: «удалить selectable_rules[] полностью, оставить только presets[]». Не выполнил.

5. **«SPEC 053» / «Parametrized presets» в UI labels** — внутренние термины разработки на user-facing экране.

### Что должно быть (target после fix'а)

**Принцип:** preset-ref и user inline/srs — это **варианты одного rule** в едином упорядоченном списке. Юзер не различает «откуда rule пришло» — он видит просто Rules / Library как раньше.

| Аспект | Как должно быть |
|---|---|
| `template.*` | **Только** `presets[]`. `selectable_rules[]` удалено целиком (legacy preset'ы портированы в `presets[]`). |
| Library dialog | Одна секция без подзаголовков. Каждый item — preset. На Add создаётся `kind: preset` ref в state. |
| `model.Rules` | Один упорядоченный массив `[]*Rule` где `Rule` имеет поле `Kind` (preset / inline / srs). Параллельные `CustomRules` + `PresetRefs` удалены. |
| Rules tab | Один renderer для всех kind'ов. Tile одинаковый: drag ↑↓ / enable / label / summary / outbound picker / edit / delete. Drag перемещает свободно **между всеми типами**. |
| Edit dialog | Dispatch по `rule.Kind`: preset-ref → vars-only form; inline → full match form; srs → URL + download. UI tile один. |
| Build pipeline | Обход `state.rules[]` **по порядку**. Для каждого rule по `Kind` диспатч на expand preset / emit inline / emit srs. Fragments эмитятся в `route.rules[]` строго в порядке state. |
| UI labels | Никаких «SPEC 053», «Parametrized presets», «Preset bundles». Только нейтральное «Rules», «Library». |

### Атомарные задачи fix'а

1. **Model unification** (`ui/configurator/models/`):
   - Удалить `PresetRefs []*PresetRefState` и `DNSTemplateOverrides` как параллельные поля
   - Сделать `Rule` interface (или struct с Kind discriminator) объединяющий 3 формы
   - `model.CustomRules` остаётся, но содержит все 3 kind'а (или переименовать в `model.Rules`)
   - Sync helpers переписать на единый array

2. **UI Rules tab** (`ui/configurator/tabs/rules_tab.go`):
   - Один `buildRuleRows` который dispatch'ит на kind-specific renderer внутри
   - Drag ↑↓ работает на индексах единого массива
   - Удалить `preset_ref_rows.go` (или переписать как helper для kind=preset под общим builder'ом)

3. **Library dialog** (`ui/configurator/tabs/library_rules_dialog.go`):
   - Удалить разделение на две секции
   - Удалить legacy `SelectableRules` импорт — работать только с `template.Presets`
   - Удалить «Parametrized presets (SPEC 053)» header

4. **Edit dispatch** (`ui/configurator/dialogs/`):
   - Tap на preset-ref tile открывает `showEditPresetRefDialog` (как сейчас)
   - Tap на inline/srs tile открывает `add_rule_dialog` (как сейчас)
   - Dispatcher в одной точке, не в раздельных tile-builder'ах

5. **Build pipeline** (`core/build/preset_merge.go` → должен быть переписан):
   - Удалить отдельный `MergePresetsIntoRoute` append-в-конец
   - Расширить `MergeRouteSection` чтобы обходил `state.rules[]` по порядку и dispatch'ил kind=preset через expand, kind=inline/srs через старый код
   - Один проход — один порядок

6. **Template** (`bin/wizard_template.json`):
   - Удалить секцию `selectable_rules[]` целиком
   - Перенести **все** правила оттуда в `presets[]` (включая те которые мы пока не портировали — все 17 entries)
   - Убедиться что эквивалентность сохранена

7. **Template parser** (`core/template/loader.go`):
   - Удалить парсинг `selectable_rules[]` (или оставить как warning-only legacy для шаблонов из старых сборок)
   - `TemplateData.SelectableRules` — удалить поле; всё что было на нём — переделать на `TemplateData.Presets`

8. **State migration** — preserve порядок:
   - При v5→v6 миграции legacy CustomRules идут в `state.rules[]` в их текущем порядке (без сегрегации по kind)
   - При load v6 → model.Rules заполняется в порядке из state.rules[]

9. **Удалить все UI labels с «SPEC 053» / «Preset bundles» / «Parametrized presets»** — `grep -r` и почистить.

### Acceptance fix'а

- [ ] `bin/wizard_template.json` не содержит `selectable_rules[]` секции
- [ ] Library показывает один список без headers и separator'ов
- [ ] Rules tab показывает все правила одним списком, drag ↑↓ работает между любыми двумя rule'ами
- [ ] `config.json::route.rules[]` эмитится в порядке `state.rules[]` (можно проверить переставив preset-rule выше inline и увидев его выше в config)
- [ ] Grep по UI не находит «SPEC 053» / «Parametrized presets» / «Preset bundles» / «Preset-based rules»
- [ ] Все тесты зелёные, build green
- [ ] Round-trip load v6 → save v6 сохраняет порядок правил
