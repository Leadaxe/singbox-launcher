# Template reference (wizard_template.json)

Архитектурная сводка: что лежит в `bin/wizard_template.json`, где это потом
всплывает в runtime / state / UI. Справочник по синтаксису template'ов —
[WIZARD_TEMPLATE.md](WIZARD_TEMPLATE.md). Этот файл — reference
для разработчиков лаунчера и для понимания связи template ↔ state.

---

## 1. Файл

- **`bin/wizard_template.json`** — единственный template для всех ОС.
  Платформозависимые куски через секцию `params` + `if`/`if_or` поверх vars.
- **Pinned ref:** `internal/constants.RequiredTemplateRef` хранит SHA коммита
  в репозитории, под который собран launcher. CI ldflags инжектит реальный
  hash на релизе; в dev-сборке используется source-default (последний known-good
  merge commit).
- **Lifecycle на upgrade:** `core/template_migration.go::InvalidateTemplateIfStale`
  сравнивает `Settings.LastTemplateLauncherVersion` (записан после последнего
  успешного «Download Template» через `MarkTemplateInstalled`) с
  `constants.AppVersion`. При несовпадении удаляет `bin/wizard_template.json`
  — на следующем запуске UI показывает «Download Template», после успешной
  скачки `bin/settings.json` получает новый `last_template_launcher_version`.
  Dev-сборки (`v-local-test`, `unnamed-dev`, `*-dirty`) пропускают
  invalidation — иначе локальная разработка ломалась бы на каждом запуске.
  См. SPEC 046 (механизм) и SPEC 067 (breaking template format — `#if` +
  `@`-only outer `if[]` — триггерится тем же bump `AppVersion`).

---

## 2. Top-level shape

```jsonc
{
  "parser_config":   { ... },        // ParserConfig wrapper для subscription parser
  "config":          { ... },        // sing-box config skeleton (log/dns/inbounds/outbounds/route/experimental)
  "params":          [ ... ],        // platform-conditional patches на config (replace/prepend/append)
  "dns_options":     {               // dns tab library
    "servers": [ ... ],              // template DNS server entries (+ required:true для local/direct resolver)
    "rules":   [ ... ]
  },
  "selectable_rules":[ ... ],        // legacy rules library — kept for back-compat, replaced by presets[]
  "presets":         [ ... ],        // SPEC 053 self-contained preset bundles
  "vars":            [ ... ]         // typed template variables (UI Settings tab + @var substitution)
}
```

---

## 3. Per-section storage / usage

| Top-level key | Содержит | Куда попадает в runtime | UI tab где видно |
|---------------|----------|--------------------------|------------------|
| `parser_config` | Default ParserConfig skeleton: outbounds (`proxy-out`, `direct-out`, `auto-proxy-out`) с top-level `required: true` маркерами (см. §5). После SPEC 058 — **live source-of-truth** для body referenced template outbound'ов (state хранит только thin `{tag, ref: "#TEMPLATE#"}`). | На fresh install — в `model.ParserConfigJSON`. При LoadState body для `ref="#TEMPLATE#"` entries резолвится отсюда на каждый render/build. | Outbounds tab (renders model.GlobalOutbounds) |
| `config` | Sing-box config skeleton: `log`, `dns`, `inbounds`, `outbounds`, `route`, `experimental`. Содержит `@var` плейсхолдеры. | После `applyParams(GOOS) + substitute @vars` → `TemplateData.Config` (по секциям). При build merge'ится с state-derived sections. | Никакая напрямую; преview через Edit dialog | 
| `params` | Platform-conditional patches (`if`/`if_or` + `replace`/`prepend`/`append`) | Применяются в `LoadTemplateData` (GetEffectiveConfig) — продьюсят `Config` под текущий runtime.GOOS | — |
| `dns_options.servers` | Library template DNS servers (cloudflare, google, yandex, ...) + mandatory `required:true` entries (`local_dns_resolver`, `direct_dns_resolver`) | `TemplateData.DNSOptionsRaw` → используется `ResolveDNS` для резолва body kind=template entries в state | DNS tab (renders kind=template entries) |
| `dns_options.rules` | Default template DNS rules (опционально) | Auxiliary fill для DNS rules editor если state пустой | DNS tab |
| `selectable_rules` | **Legacy.** Library из v3+ времён. Полностью заменён `presets[]`. | Сохранено для back-compat: загружается в `TemplateData.SelectableRules`, фильтруется по platforms | Не показывается (Library показывает только `presets[]`) |
| `presets` | Self-contained preset bundles: vars / rule_set / dns_servers / dns_rule / rule / outbounds (SPEC 053 + 055 + 057) | `TemplateData.Presets`. На enable preset → создаются `kind=preset` entries в `state.rules`, `state.dns_options.servers/rules`, `state.connections.outbounds` через Sync* функции | Library dialog (add to Rules) → Rules tab (preset rows) |
| `vars` | Объявления типизированных template-переменных (`name`, `type`, `default_value`, `options`, `wizard_ui`, `title`, `tooltip`, `if`/`if_or`, `platforms`) | `TemplateData.Vars`. Дефолты применяются если в `state.vars[]` нет override. Литералы `@var` в `config`/`params` подставляются на build. Outer `if[]`/`if_or[]` — **только `@`-form** (SPEC 067). | Settings tab (auto-rendered) + DNS scalars (`dns_*` hidden vars) |

«UI tab где видно» — где юзер взаимодействует с этой секцией. Output в
`config.json` — это всегда build pipeline; ни одна секция template не идёт
в config.json напрямую без прохождения через `state + resolve*`.

---

## 4. Presets (SPEC 053 + SPEC 055 + SPEC 057-R-N)

Preset — параметризованный self-contained bundle. Каждый компонент имеет
свой ref-механизм в state.

### 4.1 `presets[].outbounds[]` — SPEC 055 + SPEC 057-R-N

Entries с `mode` discriminator:

| `mode` | Эффект на state | Эффект на config.json |
|--------|------------------|------------------------|
| `add` (или omit) | На enable preset → entry в `state.connections.outbounds[]` с `ref = preset.id`. Body резолвится из entry. | Эмитится как обычный outbound через `GenerateOutboundsFromParserConfig`. |
| `update` | На enable preset → `OutboundUpdate{ref, patch}` push в `state.connections.outbounds[<target_tag>].updates[]`. Target tag должен существовать в state (find by Tag; не найден → warning, no-op). | `MergeOutboundUpdatesInPlace` применяет patches до generator'а (base + apply patches в order). |

Lifecycle: `core/build/sync_outbounds.go::SyncOutboundsWithActivePresets`.
Adopt-on-first-sync: pre-SPEC-057 globals без `Ref`, совпадающие по tag с
expected preset add — adopt'ятся (preserve body, add `Ref`).

### 4.2 `presets[].dns_servers[]` — SPEC 053 + SPEC 056-R-N

Bundled DNS server defs с локальными tag'ами. На enable preset →
`SyncDNSOptionsWithActivePresets` создаёт entries в `state.dns_options.servers[]`
с `kind=preset, ref="<preset_id>:<local_tag>"`. Body резолвится из template
+ `@var` substitute из `preset.body.vars` каждый раз на build/render.

Юзер может toggle per-server (preserve'ится в state.entries.Enabled).
На disable preset → entries удаляются (re-enable → свежие дефолты).

### 4.3 `presets[].dns_rule` — SPEC 053

Опциональный объект (один на preset). На enable preset → entry в
`state.dns_options.rules[]` с `kind=preset, ref="<preset_id>"`. Body
резолвится из template + vars + tag-prefix (`@dns_server` var может
ссылаться на bundled `dns_servers[].tag`).

### 4.4 `presets[].rule` — SPEC 053

Routing rule preset'а. На enable → entry в `state.rules[]` с
`kind=preset, ref="<preset_id>"`. На build `MergePresetsIntoRoute`
expand'ит ref в `template.presets[id].rule`: substitute vars, prefix
local rule_set tag'и, resolve sentinels (`reject` / `drop` → `action`),
эмитит в `route.rules[]` в том же порядке, как entry в `state.rules[]`.

### 4.5 `presets[].vars[]` — SPEC 053 + SPEC 048

Типизированные локальные переменные preset'а.

| `type` | UI control | Substitution value |
|--------|------------|--------------------|
| `outbound` | Dropdown: outbound tags + `reject` + `drop` (опц. whitelist через `options`) | Tag-строка |
| `dns_server` | Grouped dropdown (3 секции) или whitelist (`options`/`select`) | Tag-строка (build prefix'ует bundled tag'и при substitute) |
| `enum` | Dropdown по `options[]` (object `{title, value}`) | `value`-строка |
| `text` | Text entry | Строка |
| `number` | Numeric entry | Строка-число |
| `bool` | Checkbox | `"true"` / `"false"` |

Substitute механизм: build-time recursive walk по `rule` / `dns_rule` /
`dns_servers` / `rule_set` фрагментам — каждая строка `"@name"`
заменяется на `varsMap[name]`. Если var отфильтрована через `if`/`if_or`
— substitute её литерала фейлится → preset skip + warning.

State хранит **только diff** от template defaults в `rule.body.vars`
(пустой `vars: {}` = preset на template defaults).

---

## 5. Template-owned vs user-editable

Маркер `"required": true` (SPEC 056-R-N Phase C/E) — template-only флаг,
state не персистит. Применим к:

| Где | Эффект в UI |
|-----|-------------|
| `parser_config.outbounds[].required` | Outbounds tab: Up/Down + Edit + Reset; Del не рендерится |
| `dns_options.servers[].required` | DNS tab: enabled+lock (toggle blocked); Edit/Del заблокированы |

**Shape (после SPEC 058):** `required` — top-level поле прямо на outbound
entry, не вложенное в обёртку:

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

**DEPRECATED:** старая форма `{ "wizard": { "required": 1 } }` всё ещё
парсится через legacy fallback в `td.RequiredOutboundTags()` —
исключительно для обратной совместимости со старыми template-форками.
Новые template'ы должны использовать top-level `required: true`.

Read live на каждый UI render через helpers:
- `wizardbusiness.DNSTagLocked(model, tag)` — для DNS
- `templateRequiredTags(model)` → используется `ResolveOutbounds` для outbound

Если template author снимает `required:true` в новой версии template'а —
эффект мгновенный (state не помнит stale значение).

---

## 6. Data flow

```
bin/wizard_template.json (pinned via RequiredTemplateRef)
         │
         ▼
LoadTemplateData(execDir)
         │   read JSON
         │   ApplyParams(runtime.GOOS) → effective Config
         │   Substitute @vars в Config (через TemplateData.Vars defaults)
         │   ParsePresets → []Preset (фильтр по platforms)
         │   ParseSelectableRules → []SelectableRule (legacy, фильтр platforms)
         ▼
model.TemplateData (in-memory, immutable)
         │
         ├──► UI render (Library dialog, DNS tab, Settings tab, Outbounds tab)
         │
         ├──► build pipeline:
         │     ResolveDNS(state, template, vars)
         │     ResolveRoute(state, template, vars)
         │     ResolveOutbounds(state, template)
         │
         └──► presenter Sync* (на каждый preset toggle):
                SyncDNSOptionsWithActivePresets(rules, &state.DNS, presets)
                SyncOutboundsWithActivePresets(rules, &state.outbounds, presets)
```

TemplateData immutable после load; модификация template requires app restart.

**SPEC 058 — template body как live source.** Outbound entries в
`state.connections.outbounds[]` хранятся как **thin refs**
(`{tag, ref: "#TEMPLATE#", updates: [...]}`) — body отсутствует. На
каждый render/build body резолвится из `template.parser_config.outbounds[tag]`
через `ResolveOutbounds`. Template-author эффект: правка
`parser_config.outbounds[].options` / `addOutbounds` / `comment` в новом
билде доезжает до юзера автоматически (без manual Reset на каждой
референсной entry). User edits хранятся как field-level diff в
`updates[].patch` с `ref="#USER#"`. См. SPEC 058 + [DATA_FLOW.md](DATA_FLOW.md)
для подробностей resolver pipeline'а.

---

## 7. `vars` mechanism

**Объявление** (template) — канонический вид из stock `bin/wizard_template.json`:

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
    "if": ["@tun"]   // outer if — только @-prefixed bool var (§9.6)
  },
  {"separator": true}
]
```

**Preset-local vars** используют поле **`default`** (не `default_value`); та же дисциплина
`if`/`if_or` и оформление — §10.

**Override** (state.json):
```jsonc
"vars": [
  { "name": "tun", "value": "false" }
]
```

**Substitute** (build): литералы `"@tun"` в `config` / `params` / preset
фрагментах заменяются на эффективное значение (state override ИЛИ template
default). Условия **`if`/`if_or`** на params/presets/vars проверяются по тому
же varsMap; каждый элемент списка **обязан** быть `@var` (bare `"tun"` → loader
error, §9.6).

**Scope**:
- Глобальные template vars (`template.vars[]`) — видны в top-level `config`
  / `params` (НЕ видны внутри preset'а).
- Preset-local vars (`preset.vars[]`) — видны только внутри своего preset'а
  (rule/dns_rule/dns_servers/rule_set). Cross-scope доступ запрещён —
  preset должен быть self-contained.

**Reserved names:** `vars[].name` **`runtime`** зарезервировано (namespace
runtime-globals `@runtime.*` в `#if` predicates only — §9.5; имена `platform` /
`arch` снова свободны).

**Special: DNS scalars.** `dns_strategy`, `dns_final`, `dns_default_domain_resolver`
объявлены как hidden vars `dns_*`. UI DNS tab пишет в `model.SettingsVars`,
`SyncDNSModelToSettingsVars` копирует в `state.vars[]` перед Save. Build
substitute'ит `@dns_*` литералы в `config.dns`.

**Special: `route_final`.** UI Rules tab dropdown «Final outbound» →
`model.SelectedFinalOutbound` → `SettingsVars["route_final"]` →
`state.vars[]`. Template имеет `"final": "@route_final"` в `config.route`.

**Оформление JSON** bundled template — §10 (editorial style, не контракт loader'а).

Реализация: `core/template/vars_resolve.go` + `core/template/substitute.go`.

---

## 8. Pinned templates

Template вшит в репозиторий → embedded в бинарь → распакован в `bin/` на
first run. Каждая релизная сборка пиннит конкретный commit:

| Source | Когда используется | Where |
|--------|---------------------|-------|
| **CI inject** | Release сборка: GitHub Actions подставляет SHA merge commit'а через `-ldflags '-X singbox-launcher/internal/constants.RequiredTemplateRef=<sha>'` | `.github/workflows/release.yml` |
| **Source default** | Dev-сборка (`go build` без ldflags) | `internal/constants/constants.go::RequiredTemplateRef` константа |

**Bump процесс** (на каждом релизе):
1. Merge `develop` → `main` (создаётся merge commit с обновлённым `bin/wizard_template.json`)
2. Tag merge commit (`vX.Y.Z`)
3. На `develop` обновить source-default `RequiredTemplateRef` на SHA merge commit'а

**Lifecycle на launch**:
```
launcher start
     │
     ▼
InvalidateTemplateIfStale(execDir)
     │   compare Settings.LastTemplateLauncherVersion vs constants.AppVersion
     │   stale (LastTemplateLauncherVersion < AppVersion) → unlink bin/wizard_template.json
     │   (dev AppVersion skip: v-local-test / unnamed-dev / *-dirty)
     ▼
UI shows «Download Template» (если файл отсутствует)
     │   юзер кликает → скачивается с raw.githubusercontent.com под pinned ref
     │   MarkTemplateInstalled → bin/settings.json::last_template_launcher_version = AppVersion
     ▼
LoadTemplateData
```

Реализация: `core/template_migration.go::InvalidateTemplateIfStale` +
`internal/locale/settings.go::LastTemplateLauncherVersion` /
`MarkTemplateInstalled` + `core/template/loader.go::LoadTemplateData`.

Breaking template format changes (например SPEC 067 — `#if` + `@`-only outer
`if[]`) триггерятся этим же механизмом: после bump `AppVersion` на первом
запуске старый кеш удаляется → юзер скачивает новый шаблон одним кликом.

---

## 9. `#if` construct (SPEC 067) — desktop only

Template expressions v1 — declarative conditional field inclusion прямо в
шаблоне, без post-substitute Go-хуков. Реализован в
`core/template/substitute.go::SubstituteVarsInJSON` (walker) и
`core/template/template_validate.go::validateIfConstruct` (load-time
validation). Покрывает кейсы вида «одно поле внутри уже эмиченного объекта
зависит от bool var / runtime platform».

> **Mobile parity:** все `#*` constructs (`#if`, потенциальные
> `#for_each` / `#include`) — **desktop only** до подтяжки реализации в
> LxBox. Шаблоны, шарящиеся между лаунчерами, должны helmet'ить
> платформы которых поддержка ещё нет.

### 9.1 Naming discipline — `#` vs bare vs `@`

| Префикс | Где | Зачем маркер |
|---------|-----|--------------|
| `#` | Construct gateway (`#if`) + predicates в `and`/`or` (`#in`, `#not`, `#notEmpty`, …) | Scope-switch: walker отличает control-key от data-key в произвольном объекте; predicate-имя от string literal в predicate list |
| bare | Inner keys тела `#if` (`and`, `or`, `value`, `else`) + outer legacy keys (`params[].if`, `params[].if_or`, `params[].value`, `params[].mode`) | Walker уже в known scope, маркер избыточен |
| `@` | Var-ref (только имя из `vars[]`; bare `"var"` → loader error) + runtime globals `@runtime.platform` / `@runtime.arch` (только в `#if` predicates) | Унифицированная нотация var-ref'ов везде; неоднозначность «literal vs var name» устранена |

Forward compatibility: неизвестный ключ начинающийся с `#` → walker
логирует warn и удаляет (graceful degradation). Это позволяет добавлять
новые constructs (`#for_each`, `#include`, …) без breaking change для
старых лаунчеров.

### 9.2 Форма

```jsonc
"#if": {
  "and":   [<predicate>, <predicate>, ...],  // mutually exclusive с `or`
  "or":    [<predicate>, <predicate>, ...],  // mutually exclusive с `and`
  "value": <any JSON>,                        // обязателен, then-ветка
  "else":  <any JSON>                         // опциональный else-ветка
}
```

Правила (validation на load):
* Ровно один из `and` / `or` непустым списком. Нет / оба / пустой list → loader error.
* `value` обязателен (не nil).
* `else` опционален; null в `value`/`else` → error в map-spread (нельзя merge), legal в array-element.

### 9.3 Два режима размещения

**Map-spread mode** — `#if` как ключ внутри объекта:

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

* condition true → `value` обязан быть объектом; его поля мерджатся в
  родительский объект (collision → branch overrides). Ключ `#if` удаляется.
* condition false: при наличии `else` мерджатся его поля; без `else`
  ключ просто удаляется (parent unchanged).

**Array-element mode** — `#if` как единственный ключ объекта-элемента
массива:

```jsonc
"options": [
  "always",
  {"#if": {"and": ["@dark_mode"], "value": "extra-dark", "else": "extra-light"}},
  "regular"
]
```

* condition true → элемент заменяется на `value` (любой тип).
* condition false: при наличии `else` — заменяется на `else`; без `else`
  — элемент **удаляется** из массива (длина -1).

Detection rule: элемент — `#if` wrapper, если это объект из РОВНО одного
ключа `#if`. Иначе обычный элемент (с возможным spread-mode `#if` внутри).

### 9.4 Expression language — predicates

Каждый элемент `and` / `or` — predicate. Восемь форм:

| Форма | Семантика |
|---|---|
| `"@var"` | bool template var → `scalar == "true"` (только bool var; **не** `@runtime.platform` / `@runtime.arch`) |
| `{"@var": "literal"}` | equality: `trim(scalar) == "literal"` (literal **не** начинается с `#`) |
| `{"@var": "#notEmpty"}` | text → `len(trim(scalar)) > 0`; text_list → `len(list) > 0`; bool → `scalar == "true"` |
| `{"@var": "#isEmpty"}` | инверсия `#notEmpty` |
| `{"@var": {"#in":      ["a","b","c"]}}` | `trim(scalar)` присутствует в списке (`["..."]` или `@text_list_var`) |
| `{"@var": {"#notIn":   ["a","b","c"]}}` | `trim(scalar)` отсутствует в списке |
| `{"@var": {"#matches": "^[a-z]+$"}}` | `trim(scalar)` match'ит Go-regexp |
| `{"#not": <predicate>}` | унарная негация (recursive inner predicate) |

Substitution внутри predicate args: literal в equality, элементы `#in` /
`#notIn`, regex pattern в `#matches` могут содержать `@var` — walker
substitute'ит их **до** оценки predicate. ИСКЛЮЧЕНИЕ: bare `"@var"` в
predicate list и ключ `"@var"` в single-key object'е walker не
substitute'ит (иначе var-reference потеряется).

Пример:

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

Namespace `@runtime.*` — pseudo-var'ы, доступные **только** в `#if.and` /
`#if.or` predicates (расширяемый — новые поля добавляются под `@runtime.`):

| Global | Runtime source | Значения |
|---|---|---|
| `@runtime.platform` | `runtime.GOOS` | `"darwin"`, `"windows"`, `"linux"` |
| `@runtime.arch` | `runtime.GOARCH` | `"amd64"`, `"arm64"`, `"386"` |

Семантика — те же predicate-формы, что у text-var (equality, `#in`,
`#notIn`, `#matches`, `#notEmpty` / `#isEmpty`). Bare `"@runtime.platform"` /
`"@runtime.arch"` в predicate list (bool-form) → validation error: они не bool.

Case-sensitive lower-case (как `runtime.GOOS` / `runtime.GOARCH`).
**Reserved:** `vars[].name == "runtime"` → loader error (collision с namespace
`@runtime.*`; `platform` / `arch` снова свободны как имена vars). **Outer
`if` / `if_or`** runtime globals
**не принимают** — там только bool template vars; platform-gate на уровне
param по-прежнему через `params[].platforms[]`.

Win7-сборка (`windows/386`): `{"@runtime.platform": "windows"}` + `{"@runtime.arch": "386"}`
в одном `and` — эквивалент «только win7-bin».

### 9.6 Outer `if` / `if_or` — канонический `@`-only

`params[].if` / `params[].if_or`, `vars[].if` / `vars[].if_or`,
`presets[].if` / `presets[].if_or` принимают **только** `@`-prefixed
var-ref'ы. Bare `"tun"` → loader error на template load:

```
template: params[N].if has bare var-ref "tun" in if[]; use canonical "@tun" form
```

Var должна существовать в `vars[]` и иметь `type: "bool"`. Runtime globals
(`@runtime.platform` / `@runtime.arch`) в outer `if[]` **запрещены** — только в `#if`
predicates.

### 9.7 Реальный пример — TUN inbound без дублирования

Было (две `params[].name="inbounds"` entries, различающиеся **только**
наличием `interface_name`):

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

Стало (одна entry, platform-conditional поле инкапсулировано в map-spread
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

**route.rules** с динамическим `inbound[]` (array-element `#if`, скалярные `value`):

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

Подробности и edge cases — `SPECS/067-F-N-TEMPLATE_EXPRESSIONS/SPEC.md`.

---

### 9.8 `default_value` поддерживает `#if` (runtime-only)

`vars[].default_value` может быть `#if`-выражением — дефолт вычисляется в runtime
по `@runtime.*` globals. Обобщает per-platform ключи (`win7` / `<goos>` /
`default`): вместо именованных ключей — условия (`and`/`or`, `#in`, `#matches`, …).

**Только `@runtime.*`:** ссылки на другие `vars[]` внутри `default_value`-`#if`
**запрещены** (loader error) — на этапе resolve дефолтов остальные vars ещё не
разрешены, порядок не гарантирован. Globals от порядка резолва не зависят.

Две формы:

```jsonc
// top-level: всё default_value — это #if
"default_value": {"#if": {"and": [{"@runtime.platform": "windows"}, {"@runtime.arch": "386"}],
                          "value": "gvisor", "else": "system"}}

// per-platform: значение ключа платформы — #if-дерево (можно мешать со строками)
"default_value": {"default": {"#if": {"and": [{"@runtime.platform": "windows"}, {"@runtime.arch": "386"}],
                                      "value": "gvisor", "else": "system"}}}
```

Выбранная ветвь рекурсивно разрешается до скаляра (строка / число / bool →
строка); condition false без `else` → пустой дефолт. Реализация —
`VarDefaultValue.ForPlatform` (`core/template/vars_default.go`), валидация —
`validateDefaultValueIf` (пустой `varByName` → любой user-var ref = «unknown var»).

## 10. Formatting style (stock `bin/wizard_template.json`)

Editorial conventions for the **bundled** template. Порядок ключей и переносы
**не влияют** на loader — это readability для maintainers. Кастомные шаблоны
могут игнорировать §10, но semantic rules (§9) обязательны.

### 10.1 Общий принцип

**Компактно** — литералы и мелкие metadata-объекты. **Развёрнуто** — выражения
(`@…`, `#if`, outer `if[]`) и длинные списки.

### 10.2 Top-level `vars[]` и `presets[].vars[]`

| Часть | Оформление |
|-------|------------|
| «Шапка» | **Строка 1:** `name`, `type`, `wizard_ui`, `title`, `tooltip`, `platforms`, `comment`, `select`, … |
| `default_value` / `default` | **Отдельная строка** с отступом |
| `options[]` | **Multiline:** каждый элемент на своей строке (`{title,value}` — по элементу) |
| outer `if` / `if_or` | **Отдельная строка**, **в конце** объекта (после metadata) |
| `{"separator": true}` | **Одна строка** |
| Простые preset vars (`out`, …) | **Одна строка** целиком |

Пример conditional var:

```jsonc
{"name": "tun_mtu", "type": "text", "wizard_ui": "edit",
  "title": "TUN MTU", "tooltip": "…",
  "default_value": "1492",
  "if": ["@tun"]
},
```

### 10.3 JSON payload (`config`, `params[].value`, `parser_config`)

| Контекст | Правило |
|----------|---------|
| Поля с `@` | **Одно поле — одна строка** |
| Литералы (`type`, `tag`, `auto_route`, …) | Можно на одной строке между собой |
| `options` **с** `@`-полями | **Multiline object** (не одна строка) |
| `options` / `filters` / `addOutbounds` **без** `@` | **Одна строка** |
| Мелкие struct'ы из литералов (≤2–3 поля) | **Одна строка** (`direct-out`, hijack-dns, `mode:update`) |
| Крупные объекты (`dns_options.servers[]`, preset `dns_servers[]`, полный `mode:add`) | **Multiline** — одно поле на строку |

Пример urltest `options`:

```jsonc
"options": {
  "url": "@urltest_url",
  "interval": "@urltest_interval",
  "tolerance": "@urltest_tolerance",
  "interrupt_exist_connections": true
},
```

### 10.4 `#if` construct

| `value` / `else` | Оформление |
|------------------|------------|
| **Скаляр** | `"#if": {"and": [...], "value": "…"}` — одна строка |
| **Объект** | условие + `"value": {` на строке 1; тело объекта ниже; закрытие `}}` |

Пример map-spread:

```jsonc
"#if": {"and": [{"@runtime.platform": {"#in": ["windows", "linux"]}}], "value": {
  "interface_name": "singbox-tun0"
}}
```

### 10.5 Presets

| Секция | Оформление |
|--------|------------|
| `rules`, `dns_rule`, простые `vars[]` | одна строка |
| `rule_set[]` (inline и remote) | строка 1: metadata (`tag`/`type`/`format`/**`if`/`if_or`**); строка 2: `rules` / `url` |
| `rule_set[]` inline, длинные suffix | одна строка если влезает; очень длинные — переносы в массивах |
| `outbounds[]` `mode:update` | одна строка на entry |
| `outbounds[]` `mode:add` (полный) | multiline-объект; `options` / `filters` — одна строка |
| `params[]` route.rules (скалярные `#if`) | правило целиком в одну строку |

> У **condensed**-объектов с metadata в одну строку (`rule_set[]`) `if`/`if_or` ставится **на той же строке** (в отличие от `vars[]`, где `if` — отдельной строкой в конце).

### 10.6 Шпаргалка

| | Одна строка | Multiline |
|---|-------------|-----------|
| var metadata (строка 1) | ✓ | — |
| `default_value` / `default` | — | ✓ |
| `options[]` elements | — | ✓ |
| outer `if[]` в vars | — | ✓ (в конце) |
| `@` в payload | — | ✓ (по полю) |
| `#if` + object `value` | условие | тело |
| `#if` + scalar | ✓ | — |
| `filters`, literal `options` | ✓ | — |

---

## 11. Где лежит реализация

| Файл | Что |
|------|-----|
| `core/template/loader.go` | `LoadTemplateData` (entry point) + `TemplateData` struct |
| `core/template/preset_loader.go` | `LoadPresets` + validation |
| `core/template/preset_types.go` | Preset / PresetVar / PresetRuleSet / PresetDNSServer / PresetOutbound types |
| `core/template/preset_lite.go` | `PresetLite` interface + `PresetLiteMap` (для sync_dns без cyclic deps) |
| `core/template/vars_resolve.go` | varsMap build + outer `if`/`if_or` eval (strict `@`-prefix, SPEC 067) |
| `core/template/substitute.go` | recursive `@var` substitution + `#if` walker / predicate engine / runtime globals `@runtime.platform`/`@runtime.arch` (SPEC 067) |
| `core/template/template_validate.go` | template-side validation (uniqueness, refs resolvable, `#if` construct + outer `@`-only refs — SPEC 067) |
| `internal/constants/constants.go` | `RequiredTemplateRef` + `WizardTemplateFileName` |
| `core/template_migration.go` | `InvalidateTemplateIfStale` (stale template invalidation) |
| `core/build/preset_expand.go` | preset expand at build time (substitute + tag prefix + filter) |

См. также: [WIZARD_STATE.md](WIZARD_STATE.md) — как state взаимодействует
с template, формат `state.json` v6, lifecycle Sync*. [DATA_FLOW.md](DATA_FLOW.md)
— расширенные load/save/build/toggle диаграммы. [WIZARD_TEMPLATE.md](WIZARD_TEMPLATE.md)
— туториал для авторов preset'ов и template-vars (§10 здесь — editorial style
для maintainers bundled template).
