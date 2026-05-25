# Template reference (wizard_template.json)

Архитектурная сводка: что лежит в `bin/wizard_template.json`, где это потом
всплывает в runtime / state / UI. Туториал для авторов template'ов —
[CREATE_WIZARD_TEMPLATE.md](CREATE_WIZARD_TEMPLATE.md). Этот файл — reference
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
  сравнивает `RequiredTemplateRef` с последним кешированным значением и при
  несовпадении удаляет `bin/wizard_template.json` — следующий запуск переписывает
  его из embedded copy (см. constants.WizardTemplate).

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
| `vars` | Объявления типизированных template-переменных (name, type, default, options, if/if_or, ui_meta) | `TemplateData.Vars`. Дефолты применяются если в `state.vars[]` нет override. Литералы `@var` в `config`/`params` подставляются на build. | Settings tab (auto-rendered) + DNS scalars (`dns_*` hidden vars) |

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

**Объявление** (template):
```jsonc
"vars": [
  { "name": "tun", "type": "bool", "default": "true",
    "ui_meta": { "tab": "Settings", "title": "Enable TUN" },
    "platforms": ["windows", "darwin"] }
]
```

**Override** (state.json):
```jsonc
"vars": [
  { "name": "tun", "value": "false" }
]
```

**Substitute** (build): литералы `"@tun"` в `config` / `params` / preset
фрагментах заменяются на эффективное значение (state override ИЛИ template
default). Условия `if`/`if_or` на params/presets/vars проверяются по тому
же varsMap.

**Scope**:
- Глобальные template vars (`template.vars[]`) — видны в top-level `config`
  / `params` (НЕ видны внутри preset'а).
- Preset-local vars (`preset.vars[]`) — видны только внутри своего preset'а
  (rule/dns_rule/dns_servers/rule_set). Cross-scope доступ запрещён —
  preset должен быть self-contained.

**Special: DNS scalars.** `dns_strategy`, `dns_final`, `dns_default_domain_resolver`
объявлены как hidden vars `dns_*`. UI DNS tab пишет в `model.SettingsVars`,
`SyncDNSModelToSettingsVars` копирует в `state.vars[]` перед Save. Build
substitute'ит `@dns_*` литералы в `config.dns`.

**Special: `route_final`.** UI Rules tab dropdown «Final outbound» →
`model.SelectedFinalOutbound` → `SettingsVars["route_final"]` →
`state.vars[]`. Template имеет `"final": "@route_final"` в `config.route`.

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
     │   compare RequiredTemplateRef vs cached marker (.template_ref)
     │   mismatch → unlink bin/wizard_template.json + write new marker
     ▼
extractEmbeddedTemplate (if file missing) → copy embedded → bin/wizard_template.json
     ▼
LoadTemplateData
```

Реализация: `core/template_migration.go::InvalidateTemplateIfStale` +
`core/template/loader.go::LoadTemplateData`.

---

## 9. Где лежит реализация

| Файл | Что |
|------|-----|
| `core/template/loader.go` | `LoadTemplateData` (entry point) + `TemplateData` struct |
| `core/template/preset_loader.go` | `LoadPresets` + validation |
| `core/template/preset_types.go` | Preset / PresetVar / PresetRuleSet / PresetDNSServer / PresetOutbound types |
| `core/template/preset_lite.go` | `PresetLite` interface + `PresetLiteMap` (для sync_dns без cyclic deps) |
| `core/template/vars_resolve.go` | varsMap build + if/if_or eval |
| `core/template/substitute.go` | recursive `@var` substitution |
| `core/template/template_validate.go` | template-side validation (uniqueness, refs resolvable) |
| `internal/constants/constants.go` | `RequiredTemplateRef` + `WizardTemplateFileName` |
| `core/template_migration.go` | `InvalidateTemplateIfStale` (stale template invalidation) |
| `core/build/preset_expand.go` | preset expand at build time (substitute + tag prefix + filter) |

См. также: [WIZARD_STATE.md](WIZARD_STATE.md) — как state взаимодействует
с template, формат `state.json` v6, lifecycle Sync*. [DATA_FLOW.md](DATA_FLOW.md)
— расширенные load/save/build/toggle диаграммы. [CREATE_WIZARD_TEMPLATE.md](CREATE_WIZARD_TEMPLATE.md)
— туториал для авторов preset'ов и template-vars.
