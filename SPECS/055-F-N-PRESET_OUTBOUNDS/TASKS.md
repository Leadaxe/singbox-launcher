# SPEC 055 — Tasks

## Phase 1 — Types & loader

- [ ] `core/template/preset_types.go`
  - [ ] `Outbounds []PresetOutbound` field в `Preset`
  - [ ] `PresetOutbound` struct (Mode, Tag, Type, Options, Filters, AddOutbounds, Comment, Wizard, If, IfOr)
- [ ] `core/template/preset_loader.go`
  - [ ] `validatePresetOutbounds(preset, warnings)`:
    - [ ] mode ∈ {"", "add", "update"}
    - [ ] tag non-empty
    - [ ] mode=add → type required
    - [ ] mode=update → type warned + dropped
    - [ ] tag uniqueness within preset
    - [ ] if/if_or — vars existence + bool type
- [ ] `core/template/preset_loader_test.go`
  - [ ] Каждый validation case
  - [ ] Round-trip parse/serialize

## Phase 2 — Expand engine

- [ ] `core/build/preset_expand.go`
  - [ ] `Fragments.Outbounds []PresetOutboundExpanded`
  - [ ] `PresetOutboundExpanded{Mode, Tag, Body}`
  - [ ] `_substitute(outbound, vars)` для options/filters
  - [ ] Filter by if/if_or
  - [ ] Type immutability для update (drop field + warning)
- [ ] `core/build/preset_expand_test.go`
  - [ ] Add basic
  - [ ] Update with vars substitution
  - [ ] If-filter drops outbound
  - [ ] Type-on-update dropped

## Phase 3 — Merge pipeline

- [ ] `core/build/preset_merge.go`
  - [ ] `MergePresetsIntoOutbounds(baseOutbounds, presetRefs) []Outbound`
  - [ ] Алгоритм per-preset emit in RuleOrder
  - [ ] mode=add: identical-skip / first-wins
  - [ ] mode=update: lookup target, apply patch
  - [ ] `applyOutboundUpdate(target, patch)` helper:
    - [ ] filters → replace
    - [ ] addOutbounds → union
    - [ ] options.* → replace per-field
    - [ ] wizard.* → replace per-field
    - [ ] type → drop
    - [ ] tag → drop
    - [ ] comment → replace
- [ ] `cleanDanglingOutboundRefInRule(rule, emittedTags, fallback)`:
  - [ ] outbound ref не в emittedTags → fallback or drop rule
- [ ] `core/build/preset_merge_test.go`
  - [ ] Add basic + collision (preset vs preset, preset vs globals)
  - [ ] Update basic (filters replace, addOutbounds union)
  - [ ] Update missing target → warning + skip
  - [ ] Multi-update chain в порядке RuleOrder
  - [ ] Dangling outbound ref → fallback

## Phase 4 — Build integration

- [ ] `core/build/build.go`
  - [ ] Вставить `MergePresetsIntoOutbounds` после base builder в outbounds section
  - [ ] `MergePresetsIntoRoute`: добавить cleanDanglingOutboundRefInRule pass
- [ ] Integration tests:
  - [ ] preset с outbounds + rule использующий @out → корректный config
  - [ ] preset disabled → outbounds исчезли + dangling user-rule fallback

## Phase 5 — UI

- [ ] `ui/configurator/business/wizard_outbound.go`
  - [ ] `collectActivePresetOutboundTags(model) []string`
  - [ ] Расширить `GetAvailableOutbounds` — append preset tags, dedup
- [ ] `ui/configurator/tabs/rules_unified_rows.go`
  - [ ] Refresh outbound selects on preset enable/disable toggle
- [ ] `ui/configurator/presentation/presenter.go`:
  - [ ] `RefreshOutboundSelects()` method (или совместить с RefreshDNSListAndSelects)
- [ ] Tests:
  - [ ] After enable preset с outbound → `GetAvailableOutbounds` содержит новый tag

## Phase 6 — Template content + cleanup

- [ ] `bin/wizard_template.json`:
  - [ ] Удалить `filters: !RU` из global `proxy-out` и `auto-proxy-out`
  - [ ] Удалить `ru VPN 🇷🇺` global selector
  - [ ] Добавить в `ru-inside` preset:
    - [ ] `mode: "update"` для proxy-out + auto-proxy-out → `filters: !RU`
    - [ ] `mode: "add"` для `ru VPN 🇷🇺` selector
  - [ ] Validate: `ru-inside.vars.out.default` остаётся `"ru VPN 🇷🇺"` (preset-emitted)
- [ ] Bump `RequiredTemplateRef`
- [ ] Manual smoke test:
  - [ ] Enable ru-inside → config.json идентичен предыдущему
  - [ ] Disable ru-inside → config.json чище

## Phase 7 — Docs

- [ ] `docs/release_notes/upcoming.md` — SPEC 055 entry (EN + RU)
- [ ] `SPECS/055-F-N-PRESET_OUTBOUNDS/IMPLEMENTATION_REPORT.md` — финальный отчёт
- [ ] Rename SPEC dir: `055-F-N-` → `055-F-C-` (после QA)

## Golden fixtures

- [ ] `core/build/testdata/golden/preset_outbounds_add.json`
- [ ] `core/build/testdata/golden/preset_outbounds_update.json`
- [ ] `core/build/testdata/golden/preset_outbounds_disabled.json`
- [ ] `core/build/testdata/golden/preset_outbounds_multi_update.json`

## Out of scope (future)

- [ ] SPEC 056 — preset.inbounds (per-preset inbound configuration)
- [ ] SPEC 057 — explicit cross-preset dependencies (preset A depends on outbound from preset B)
- [ ] SPEC 058 — `mode: "replace"` (destructive full-replace)
