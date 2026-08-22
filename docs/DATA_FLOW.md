# Data flow

**🌐 Language**: English | [Русский](DATA_FLOW.ru.md)

Summary diagrams of the Configurator's load / save / build / preset-toggle flows
after SPEC 053 + 056-R-N + 057-R-N + 058-R-N. A companion to
[WIZARD_STATE.md](WIZARD_STATE.md) and [TEMPLATE_REFERENCE.md](TEMPLATE_REFERENCE.md)
(those specify the state and template sections; this one shows how they move
together over time).

---

## 1. Load flow

`state.json` + `wizard_template.json` → `model.WizardModel` (in-memory) → UI.

```
launcher start
     │
     ▼
core/template_migration.InvalidateTemplateIfStale(execDir)
     │   compare Settings.LastTemplateLauncherVersion vs constants.AppVersion
     │   stale → unlink bin/wizard_template.json (dev AppVersion — skipped)
     │   on the next start the UI shows "Download Template";
     │   after the download MarkTemplateInstalled writes AppVersion to settings.json
     ▼
extractEmbeddedTemplate (if file missing)
     │
     ▼
core/template.LoadTemplateData(execDir)
     │   read JSON
     │   ValidateWizardTemplate (including the #if construct and outer @-only — SPEC 067)
     │   ApplyParams(runtime.GOOS) → effective Config sections
     │   SubstituteVarsInJSON(goos, goarch):
     │     · resolves "@var" placeholders across the whole JSON tree
     │     · handles the "#if" construct (map-spread + array-element),
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
UI renders (Sources / Directions / Rules / DNS / Settings tabs)
```

The key point: `SyncOutboundsWithActivePresets` on Load performs
**adopt-on-first-sync** — a pre-SPEC-057 state (where preset-added outbounds
lived as ordinary globals) gets the correct `Ref` without any user
intervention.

**The SPEC 058 migration** runs on load, before the presenter: a legacy SPEC 057
state stores a template-derived outbound with an empty `ref` and a snapshotted
body — `MigrateOutboundsToReferencedShape` converts such entries into the
referenced shape (`ref="#TEMPLATE#"` plus the diff over template defaults in
`updates[].patch` with `ref="#USER#"`). The migration is idempotent; entries
with no template match (true direct outbounds) are left alone.

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
     │     ×2: state.ParserConfig.ParserConfig.Outbounds   ◄── mandatory!
     │
     ▼
state.State.Save(path)
     │   syncConnectionsFromLegacy             — copies ParserConfig.Outbounds → Connections
     │                                          (synced version wins; does not clobber updates[])
     │   hasReferencedOutbounds(Connections) ? maybeBackupPre058(path) : skip
     │                                          ◄── SPEC 058 one-shot state.json.pre-058.bak
     │                                          (on the first save after the migration)
     │   marshalDisk                          — single canonical (v6) write path
     │                                          (meta.version=6, schema=presets_v1)
     │                                          the dual write path was removed in SPEC 060
     │
     │   atomic write: open .tmp, write+fsync, Rename .tmp → path, fsync(dir)
     ▼
disk: bin/wizard_states/state.json
```

**Why sync both views?** `state.Save → syncConnectionsFromLegacy` copies
`ParserConfig.Outbounds → Connections.Outbounds`. If the sync were applied to
`Connections` alone, the adapter would overwrite the synced `updates[]`. The
solution: sync both views in `CreateStateFromModel`, so the adapter copies an
already-correct version.

Since SPEC 060, Save always writes the canonical (v6) shape. Legacy v5 files are
read by `parseV5Legacy` on load and normalized into `State`; the next Save
rewrites them in the v6 layout.

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
     │       template → resolve body from template.dns_options.servers[tag]
     │       preset   → resolve body from template.presets[id].dns_servers[local_tag] + substitute vars
     │       user     → body is already flat in the entry
     │     attach metadata: Source / Required / Locked / Active / Enabled
     │
     ├─► ResolveRoute(state, template, vars)      — pure func
     │     walk state.rules[] kind switch
     │       preset → resolve via template.presets[id].rules (expand + tag prefix)
     │       inline → emit body.match + outbound
     │       srs    → emit body.srs_url + outbound (downloaded .srs path)
     │
     ├─► MergeOutboundUpdates(ob, template)       — pure func (SPEC 058)
     │     per-entry resolver (UI preview / dialog Edit); build runtime
     │     calls MergeOutboundUpdatesInPlace below over the whole parserCfg
     │     for each outbound entry: lookup base by Ref (resolveBaseBody)
     │       ref=""           → direct entry, body inline in state
     │       ref="#TEMPLATE#" → template.parser_config.outbounds[tag]
     │       ref=<preset_id>  → template.presets[id].outbounds (mode=add)
     │     applyUpdatesToBase(base, Updates[]) → merged body
     │       preset patches in rule order, the USER patch (ref="#USER#") last
     │     attach metadata: IsDirect / IsTemplate / IsPreset / HasUserPatch /
     │                      HasPresetUpdates / Required / PresetLabel
     │
     ├─► (headless paths only) ────────────────────────────────────
     │   SyncOutboundsWithActivePresets(rules, &parserCfg.Outbounds, presets)
     │     ensures the parserCfg view is in sync (defensive — UI paths
     │     already synced it in CreateStateFromModel)
     │   MergeOutboundUpdatesInPlace(parserCfg, template)
     │     SPEC 058 pipeline: resolves the template body for referenced entries
     │     and takes the inline one for direct; then applies the Updates[] stack
     │     in order (preset patches → USER patch). The generator knows neither
     │     Ref nor Updates.
     │
     ▼
GenerateOutboundsFromParserConfig
     │     consume merged parserCfg.Outbounds[]
     │     resolve filters / addOutbounds / preferredDefault
     │     append per-source proxies (parsed from .raw cache)
     ▼
MergeDNSSection + MergeRouteSection + MergePresetsIntoRoute
     │     emit final dns / route sections in state.rules[] order
     ▼
atomic write: bin/config.json
```

**The resolver pattern** — `ResolveDNS` / `ResolveRoute` (plus
`MergeOutboundUpdates` for outbounds) are pure functions with no I/O. The UI
render and the build emit consume the very same resolved view → no divergence
between the preview and the final config.

**Headless vs UI paths.** In a UI session `CreateStateFromModel` has already
synced the state before Save, and the build only reads. On headless paths
(`rebuild_raw_cache`, `UpdateConfigFromSubscriptions`, `parseAndPreview`) the
state is read from disk, the sync is called defensively, and then
`MergeOutboundUpdatesInPlace` runs for the generator.

---

## 4. Preset toggle flow

The user clicks the checkbox on a preset row in the Rules tab → an eager state
mutation plus a UI refresh, without a full re-render.

```
UI: Rules tab — checkbox toggle on a preset row
     │   handler in rules_unified_rows.go (a one-liner after the refactor)
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
     │     re-render the DNS tab list (if open)
     │     refresh the DNS dropdowns (Final / DefaultDomainResolver / per-rule server)
     │
     ├─► build.SyncOutboundsWithActivePresets — over both views
     │     ×1: model.GlobalOutbounds
     │     ×2: model.ParserConfig.Outbounds (via RefreshDerivedParserConfig)
     │
     ├─► refresh Directions tab UI
     │     collectRowsForUI reads state directly (since SPEC 057)
     │     preset rows are shown with 🔒 + the preset label
     │     globals with updated filters show "⚠ modified by N preset(s)"
     │
     └─► RefreshOutboundOptions
           rebuild the per-rule outbound dropdowns in the Rules tab
           (newly preset-added tags appear; disabled ones vanish)

  ▲
  │
  MarkAsChanged → the Save button becomes enabled
```

Eager sync (rather than lazy, on Save) exists because the user must see the
effect immediately: a DNS server was added to the list, a new outbound appeared,
the rule dropdowns changed. Without it, the DNS and Directions tabs would show a
stale state until Save.

---

## 5. Edit dialog flow (SPEC 058)

Since SPEC 058 the outbound Edit dialog handles three classes of entry (direct /
referenced template / referenced preset) and stores the USER edit as a
field-level diff over the merged base.

```
Open Edit dialog (Directions tab → Edit button)
     │
     ▼
ResolveMergedOutbound(state, template, tag)
     │   case ref="":          merged_base = the body inline in state
     │   case ref="#TEMPLATE#": merged_base = template.parser_config.outbounds[tag]
     │                                       + apply every active preset patch
     │   case ref=<preset_id>: merged_base = template.presets[id].outbounds(tag)
     │                                       + apply every active preset patch
     │   displayBody = merged_base + apply the existing USER patch (if any)
     ▼
populate the form fields from displayBody
     │
     │   the user edits filters / options / addOutbounds / ...
     │
     ▼
[switching between the Settings tab ↔ JSON tab]
     │   syncFormToRaw(): shows the save shape (thin for referenced —
     │     only the diffed fields; the full body for direct)
     │   syncRawToForm(): takes the raw JSON and re-merges it with the template
     │     body for referenced entries → the populated form shows the merged view
     │
     ▼
Save → applyEditedConfig
     │   form_value = the body assembled from the form
     │   case referenced (ref != ""):
     │     USER_patch = field_diff(form_value, merged_base)
     │     if the diff is empty → drop the existing USER patch (a no-op Save)
     │     else replace the USER patch in updates[] (always one, always last)
     │   case direct (ref=""):
     │     the body is overwritten directly (no diff, no USER patch)
     │
     ▼
MarkAsChanged → the Save button becomes enabled
```

`syncFormToRaw` / `syncRawToForm` are critical to the two-tab UX: the state
stores the thin shape, yet the Settings tab shows the user a merged view. The
re-merge on every switch guarantees the form always shows what will actually be
emitted, not a stale snapshot.

---

## 6. Cross-references

| Aspect | Document |
|--------|----------|
| What lives in state.json, which kinds, schema v6 | [WIZARD_STATE.md](WIZARD_STATE.md) |
| What lives in wizard_template.json, presets / vars / required | [TEMPLATE_REFERENCE.md](TEMPLATE_REFERENCE.md) |
| Syntax reference — preset / template var | [WIZARD_TEMPLATE.md](WIZARD_TEMPLATE.md) |
| Overall application architecture (layers, events, ADRs) | [ARCHITECTURE.md](ARCHITECTURE.md) |
| Per-package / per-file inventory (by layers L0–L7) | [ARCHITECTURE_PACKAGES.md](ARCHITECTURE_PACKAGES.md) |
| Release notes v0.9.6 (preset-binding terminology) | [release_notes/0-9-6.md](release_notes/0-9-6.md) |

| Source SPEC | What it covers |
|-------------|---------------|
| SPECS/052-F-C-CONNECTIONS_REDESIGN | v5 connections layout (sources / outbounds / defaults) |
| SPECS/053-F-N-PRESET_BUNDLES | Preset bundles, the `kind` discriminator on rules, RequiredTemplateRef integration |
| SPECS/055-F-S-PRESET_OUTBOUNDS | `preset.outbounds[]` design (add/update modes) |
| SPECS/056-R-N-DNS_SCHEMA_REDESIGN | Flat `dns_options.servers/rules[]` kind discriminator + Resolver pattern |
| SPECS/057-R-N-OUTBOUNDS_PRESET_BINDING | Outbound `Ref` + `Updates[]` schema + lifecycle Sync |
| SPECS/058-R-N-STATE_AS_TEMPLATE_DIFF | State outbounds — thin refs (`#TEMPLATE#`/preset_id) + USER patch (`#USER#`); migration + auto-upgrade |
| SPECS/067-F-N-TEMPLATE_EXPRESSIONS | The `#if` construct (map-spread + array-element) + expression-language predicates + runtime globals `@runtime.platform`/`@runtime.arch` + strict `@`-only var refs in the outer `if[]` |
