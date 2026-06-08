# Data flow

Сводные диаграммы load / save / build / preset-toggle для Configurator'а
после SPEC 053 + 056-R-N + 057-R-N + 058-R-N. Дополняет
[WIZARD_STATE.md](WIZARD_STATE.md) и [TEMPLATE_REFERENCE.md](TEMPLATE_REFERENCE.md)
(там — спецификация секций state и template; здесь — как они вместе
двигаются по времени).

---

## 1. Load flow

`state.json` + `wizard_template.json` → `model.WizardModel` (in-memory) → UI.

```
launcher start
     │
     ▼
core/template_migration.InvalidateTemplateIfStale(execDir)
     │   compare Settings.LastTemplateLauncherVersion vs constants.AppVersion
     │   stale → unlink bin/wizard_template.json (dev AppVersion — пропуск)
     │   UI на следующем запуске показывает «Download Template»;
     │   после скачки MarkTemplateInstalled пишет AppVersion в settings.json
     ▼
extractEmbeddedTemplate (if file missing)
     │
     ▼
core/template.LoadTemplateData(execDir)
     │   read JSON
     │   ValidateWizardTemplate (включая #if construct и outer @-only — SPEC 067)
     │   ApplyParams(runtime.GOOS) → effective Config sections
     │   SubstituteVarsInJSON(goos, goarch):
     │     · resolves "@var" placeholders во всём JSON-дереве
     │     · обрабатывает "#if" construct (map-spread + array-element),
     │       runtime globals @runtime.platform / @runtime.arch — SPEC 067
     │   ParsePresets + filter platforms
     ▼
model.TemplateData (immutable for session)
     │
     ▼─── path A: state.json exists ─────────────────────────┐
     │                                                       │
     │   core/state.Load(path)                               │
     │     probe meta.version                                │
     │     parseV6 / parseV5 / parseLegacyAndMigrate         │
     │     legacyDevDNSToOptions (if dev-shape `dns.{...}`)  │
     │     MigrateOutboundsToReferencedShape(state, tpl)    │  ◄── SPEC 058 one-shot
     │       walk outbounds with empty Ref:                  │      empty Ref + tag in template
     │         tag in template.parser_config.outbounds       │      → Ref="#TEMPLATE#" + diff
     │           → Ref="#TEMPLATE#", diff→USER patch,        │      stripped to {tag, ref, updates}
     │             strip body fields                         │      idempotent on re-load
     │         else keep direct (ref="", body inline)        │
     │   → state.State {Connections, Rules, DNS, Vars}        │
     │                                                       │
     │   presenter.LoadState(stateFile)                      │
     │     restoreParserConfig (legacy view)                 │
     │     MigrateSettingsVarsFromConfigParams (one-shot)    │
     │     restoreConfigParams + restoreDNS                  │
     │     ApplyRulesLibraryMigration (idempotent)           │
     │     restoreCustomRules + restorePresetRefs            │
     │     build.SyncOutboundsWithActivePresets             │  ◄── adopt-on-first-sync
     │       (state.Rules, &model.GlobalOutbounds, presets)   │      legacy → preset-bound
     │     RefreshDerivedParserConfig                        │
     │                                                       │
     │   model.WizardModel populated                         │
     │                                                       │
     ▼─── path B: state.json missing (fresh install) ───────┤
     │                                                       │
     │   business.LoadConfigFromFile                         │
     │     prefer config.json @ParserConfig block            │
     │     fallback → template.parser_config                 │
     │   initializeWizardContent                             │
     │     InitializeTemplateState                           │
     │     ApplyWizardDNSTemplate (if DNS empty)             │
     │                                                       │
     ▼───────────────────────────────────────────────────────┘
     │
     ▼
SyncModelToGUI + RefreshOutboundOptions
     │
     ▼
UI renders (Sources / Outbounds / Rules / DNS / Settings tabs)
```

Ключевой момент: `SyncOutboundsWithActivePresets` на Load включает
**adopt-on-first-sync** — pre-SPEC-057 state (где preset-add outbounds
жили как обычные globals) получает корректный `Ref` без юзерского
вмешательства.

**SPEC 058 migration** работает на load до presenter'а: legacy SPEC 057
state хранит template-derived outbound с пустым `ref` и snapshot'нутым
body — `MigrateOutboundsToReferencedShape` переводит такие entries в
referenced shape (`ref="#TEMPLATE#"` + diff поверх template defaults в
`updates[].patch` с `ref="#USER#"`). Migration идемпотентна; entries
без template match (true direct outbounds) остаются как есть.

---

## 2. Save flow

`model.WizardModel` → `state.json` (atomic write).

```
trigger: Save button / autosave hook
     │
     ▼
presenter.CreateStateFromModel(comment, id)
     │   SyncGUIToModel                       — flush GUI widget values into model
     │   build WizardStateFile                — legacy ParserConfig + canonical Connections
     │   extractConfigParams                  — empty in v6 (vars moved to state.vars)
     │
     │   ReconcileRuleOrder(model)            — collapse RuleOrder vs PresetRefs/CustomRules
     │   SyncRulesByOrderToStateRulesV6       — produces state.Rules (preserves UI order; helper name is legacy)
     │
     │   extractTemplateDNSTags(TemplateData)
     │   SyncDNSFullToStateV6(...)            — DNS UI list → flat state.DNS.servers/rules
     │
     │   state.SyncDNSOptionsWithActivePresets — ensure kind=preset DNS entries match active preset-refs
     │     (state.Rules, &state.DNS, presetMap)
     │   applyPresetEnabledOverrides          — UI toggle for kind=preset → entry.Enabled
     │
     │   build.SyncOutboundsWithActivePresets — TWICE: on both views
     │     ×1: state.Connections.Outbounds
     │     ×2: state.ParserConfig.ParserConfig.Outbounds   ◄── обязательно!
     │
     ▼
state.State.Save(path)
     │   syncConnectionsFromLegacy             — copies ParserConfig.Outbounds → Connections
     │                                          (synced version wins; не затирает updates[])
     │   hasReferencedOutbounds(Connections) ? maybeBackupPre058(path) : skip
     │                                          ◄── SPEC 058 one-shot state.json.pre-058.bak
     │                                          (на первом save после migration)
     │   marshalDisk                          — single canonical (v6) write path
     │                                          (meta.version=6, schema=presets_v1)
     │                                          dual write path удалён в SPEC 060
     │
     │   atomic write: open .tmp, write+fsync, Rename .tmp → path, fsync(dir)
     ▼
disk: bin/wizard_states/state.json
```

**Почему Sync на обе view'а?** `state.Save → syncConnectionsFromLegacy`
копирует `ParserConfig.Outbounds → Connections.Outbounds`. Если sync
наложили только на `Connections` — адаптер затрёт sync'нутые `updates[]`.
Решение: sync обе view'а в `CreateStateFromModel`, тогда адаптер копирует
уже-корректную версию.

После SPEC 060 Save всегда пишет canonical (v6) shape. Legacy v5 файлы
читаются `parseV5Legacy` на load и нормализуются в `State`; ближайший
Save перезаписывает их в v6 layout.

---

## 3. Build flow

`state` + `template` → `bin/config.json` (sing-box-compatible).

> **Single-writer invariant (ADR-070-4).** `config.json` has **exactly one writer**:
> `AppController.RebuildConfigIfDirty` (`core/rebuild.go` → `atomicWriteConfig(ConfigPath, …)`).
> `Start()` rebuilds before launching sing-box (pre-start hook, SPEC 068 dirty
> markers); `Update()` auto-rebuilds on cache-refresh success; `RebuildConfigIfDirty`
> noop-skips when clean and not forced. Neither `Start` nor `Update` writes
> `config.json` directly. See [ARCHITECTURE.md §6.3 / §7](ARCHITECTURE.md).

```
trigger: app start / config dirty / explicit rebuild
     │
     ▼
core.AppController.RebuildConfigIfDirty  (sole config.json writer; noop if clean & not forced)
     │   assembles BuildContext{Template, Vars, Cache, DNS, Route, Preset}
     │   via config_service.buildContextFromState
     ▼
core/build entry (BuildConfig)  — pure function over BuildContext
     │
     ├─► ResolveDNS(state, template, vars)        — pure func
     │     walk state.dns_options.servers[] kind switch
     │       template → resolve body из template.dns_options.servers[tag]
     │       preset   → resolve body из template.presets[id].dns_servers[local_tag] + substitute vars
     │       user     → body уже flat в entry
     │     attach metadata: Source / Required / Locked / Active / Enabled
     │
     ├─► ResolveRoute(state, template, vars)      — pure func
     │     walk state.rules[] kind switch
     │       preset → resolve через template.presets[id].rules (expand + tag prefix)
     │       inline → emit body.match + outbound
     │       srs    → emit body.srs_url + outbound (downloaded .srs path)
     │
     ├─► MergeOutboundUpdates(ob, template)       — pure func (SPEC 058)
     │     per-entry resolver (UI preview / dialog Edit); build runtime
     │     зовёт MergeOutboundUpdatesInPlace ниже на весь parserCfg
     │     для каждой outbound entry: lookup base by Ref (resolveBaseBody)
     │       ref=""           → direct entry, body inline в state
     │       ref="#TEMPLATE#" → template.parser_config.outbounds[tag]
     │       ref=<preset_id>  → template.presets[id].outbounds (mode=add)
     │     applyUpdatesToBase(base, Updates[]) → merged body
     │       preset patches в rule order, USER patch (ref="#USER#") последним
     │     attach metadata: IsDirect / IsTemplate / IsPreset / HasUserPatch /
     │                      HasPresetUpdates / Required / PresetLabel
     │
     ├─► (headless paths only) ────────────────────────────────────
     │   SyncOutboundsWithActivePresets(rules, &parserCfg.Outbounds, presets)
     │     ensures parserCfg view синхронизирована (defensive — UI-paths
     │     уже sync'нули в CreateStateFromModel)
     │   MergeOutboundUpdatesInPlace(parserCfg, template)
     │     SPEC 058 pipeline: для referenced entries резолвит template body,
     │     для direct берёт inline; затем apply Updates[] стек в order
     │     (preset patches → USER patch). Generator не знает ни Ref, ни Updates.
     │
     ▼
GenerateOutboundsFromParserConfig
     │     consume merged parserCfg.Outbounds[]
     │     resolve filters / addOutbounds / preferredDefault
     │     append per-source proxies (parsed from .raw cache)
     ▼
MergeDNSSection + MergeRouteSection + MergePresetsIntoRoute
     │     emit final dns / route sections в порядке state.rules[]
     ▼
atomic write: bin/config.json
```

**Resolver pattern** — `ResolveDNS` / `ResolveRoute` (+ `MergeOutboundUpdates`
для outbounds) — pure funcs без I/O. UI render и build emit consume один и
тот же resolved view → нет divergence между preview и финальным config.

**Headless vs UI paths.** В UI-сессии `CreateStateFromModel` уже sync'нул
state перед Save, и build читает только. В headless path'ах
(`rebuild_raw_cache`, `UpdateConfigFromSubscriptions`, `parseAndPreview`) —
state читается с диска, sync вызывается defensively, потом
`MergeOutboundUpdatesInPlace` для generator'а.

---

## 4. Preset toggle flow

User clicks checkbox на preset row в Rules tab → eager state mutation +
UI refresh без полного re-render.

```
UI: Rules tab — checkbox toggle на preset row
     │   handler в rules_unified_rows.go (one-liner после рефактора)
     ▼
mutate model:
     state.Rules = update Enabled flag
     PresetRefs[i].Enabled = new value
     │
     ▼
presenter.RefreshAfterPresetToggle()
     │
     ├─► RefreshDNSListAndSelects
     │     v6.SyncDNSOptionsWithActivePresets(rules, &state.DNS, presetMap)
     │     re-render DNS tab list (если открыт)
     │     refresh DNS dropdown'ы (Final / DefaultDomainResolver / per-rule server)
     │
     ├─► build.SyncOutboundsWithActivePresets — на обе view
     │     ×1: model.GlobalOutbounds
     │     ×2: model.ParserConfig.Outbounds (через RefreshDerivedParserConfig)
     │
     ├─► refresh Outbounds tab UI
     │     collectRowsForUI читает state directly (после SPEC 057)
     │     preset rows показываются с 🔒 + preset label
     │     globals с обновлённой filters показывают «⚠ modified by N preset(s)»
     │
     └─► RefreshOutboundOptions
           rebuild per-rule outbound dropdown'ы в Rules tab
           (новые preset-add tag'и появляются; disabled — исчезают)

  ▲
  │
  MarkAsChanged → Save кнопка enable
```

Eager sync (а не lazy на Save) — потому что юзеру нужно сразу видеть
эффект: добавился DNS-сервер в список, появился новый outbound, выпадайки
правил обновились. Без eager sync DNS tab и Outbounds tab показывали бы
устаревшее состояние до Save.

---

## 5. Edit dialog flow (SPEC 058)

Outbound Edit dialog с SPEC 058 учитывает три класса entries (direct /
referenced template / referenced preset) и хранит USER edit как
field-level diff поверх merged base.

```
Open Edit dialog (Outbounds tab → Edit button)
     │
     ▼
ResolveMergedOutbound(state, template, tag)
     │   case ref="":          merged_base = body inline в state
     │   case ref="#TEMPLATE#": merged_base = template.parser_config.outbounds[tag]
     │                                       + apply все active preset patches
     │   case ref=<preset_id>: merged_base = template.presets[id].outbounds(tag)
     │                                       + apply все active preset patches
     │   displayBody = merged_base + apply existing USER patch (если есть)
     ▼
populate form fields из displayBody
     │
     │   юзер правит filters / options / addOutbounds / ...
     │
     ▼
[Settings tab ↔ JSON tab переключение]
     │   syncFormToRaw(): показывает save-shape (thin для referenced —
     │     только diff-ные поля; full body для direct)
     │   syncRawToForm(): берёт raw JSON, re-merge с template body для
     │     referenced entries → form populate показывает merged view
     │
     ▼
Save → applyEditedConfig
     │   form_value = собранный body из формы
     │   case referenced (ref != ""):
     │     USER_patch = field_diff(form_value, merged_base)
     │     if diff пуст → drop existing USER patch (no-op Save)
     │     else replace USER patch в updates[] (всегда один, всегда последний)
     │   case direct (ref=""):
     │     body перезаписывается напрямую (нет diff, нет USER patch)
     │
     ▼
MarkAsChanged → Save кнопка enable
```

`syncFormToRaw` / `syncRawToForm` критичны для two-tab UX: state хранит
thin shape, но юзер в Settings tab видит merged view. Re-merge на
переключение гарантирует, что form всегда показывает то, что попадёт в
emit, а не stale snapshot.

---

## 6. Cross-references

| Аспект | Документ |
|--------|----------|
| Что лежит в state.json, какие kind'ы, schema v6 | [WIZARD_STATE.md](WIZARD_STATE.md) |
| Что лежит в wizard_template.json, presets / vars / required | [TEMPLATE_REFERENCE.md](TEMPLATE_REFERENCE.md) |
| Справочник по синтаксису — preset / template var | [WIZARD_TEMPLATE.md](WIZARD_TEMPLATE.md) |
| Общая архитектура приложения (слои, события, ADR) | [ARCHITECTURE.md](ARCHITECTURE.md) |
| Per-package / per-file инвентарь (по слоям L0–L7) | [ARCHITECTURE_PACKAGES.md](ARCHITECTURE_PACKAGES.md) |
| Release notes v0.9.6 (терминология preset binding) | [release_notes/0-9-6.md](release_notes/0-9-6.md) |

| Source SPEC | Что покрывает |
|-------------|---------------|
| SPECS/052-F-C-CONNECTIONS_REDESIGN | v5 connections layout (sources / outbounds / defaults) |
| SPECS/053-F-N-PRESET_BUNDLES | Preset bundles, `kind` discriminator на rules, RequiredTemplateRef integration |
| SPECS/055-F-S-PRESET_OUTBOUNDS | `preset.outbounds[]` design (add/update modes) |
| SPECS/056-R-N-DNS_SCHEMA_REDESIGN | Flat `dns_options.servers/rules[]` kind discriminator + Resolver pattern |
| SPECS/057-R-N-OUTBOUNDS_PRESET_BINDING | Outbound `Ref` + `Updates[]` schema + lifecycle Sync |
| SPECS/058-R-N-STATE_AS_TEMPLATE_DIFF | State outbounds — thin refs (`#TEMPLATE#`/preset_id) + USER patch (`#USER#`); migration + auto-upgrade |
| SPECS/067-F-N-TEMPLATE_EXPRESSIONS | `#if` construct (map-spread + array-element) + expression language predicates + runtime globals `@runtime.platform`/`@runtime.arch` + strict `@`-only var-ref в outer `if[]` |
