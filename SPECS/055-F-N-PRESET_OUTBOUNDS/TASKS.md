# SPEC 055 — Tasks

**Status:** ✅ Все 7 фаз shipped end-to-end в одной сессии. Подробности — `IMPLEMENTATION_REPORT.md`.

## Phase 1 — Types & loader

- [x] `core/template/preset_types.go`
  - [x] `Outbounds []PresetOutbound` field в `Preset`
  - [x] `PresetOutbound` struct (Mode, Tag, Type, Options, Filters, AddOutbounds, Comment, Wizard, If, IfOr)
- [x] `core/template/preset_loader.go::validatePresetOutbounds`:
  - [x] mode ∈ {"", "add", "update"} (empty → "add"; unknown → strip)
  - [x] tag non-empty
  - [x] mode=add → type required
  - [x] mode=update → type warned
  - [x] tag uniqueness within preset
- [x] `core/template/preset_outbounds_test.go` — 7 cases

## Phase 2 — Expand engine

- [x] `core/build/preset_expand.go`
  - [x] `Fragments.Outbounds []ExpandedOutbound`
  - [x] `ExpandedOutbound{Mode, Tag, Body}`
  - [x] `_substitute(outbound, vars)` для options/filters/addOutbounds
  - [x] Filter by if/if_or
  - [x] Type immutability для update (drop field)
- [x] `core/build/preset_outbounds_test.go` — 4 expand cases

## Phase 3 — Merge pipeline

- [x] `core/build/preset_merge.go`
  - [x] `MergePresetsIntoOutbounds(baseOutbounds, presetRefs) []Outbound`
  - [x] Алгоритм per-preset emit in RuleOrder
  - [x] mode=add: identical-skip / first-wins
  - [x] mode=update: lookup target, apply patch
  - [x] `applyOutboundUpdate(target, patch)` helper:
    - [x] filters → replace
    - [x] addOutbounds → union (via `unionStringList`)
    - [x] options.* → replace per-field
    - [x] wizard.* → replace per-field
    - [x] type → drop
    - [x] tag → drop
    - [x] comment → replace
- [x] `cleanDanglingOutboundRefInRule(rule, emittedTags, fallback)`:
  - [x] sentinel reject/drop preserved
  - [x] outbound ref не в emittedTags → fallback или drop
- [x] `CleanDanglingOutboundsInRouteRules` + `CollectOutboundTagsFromRaw`
- [x] 7 merge tests + 4 cleanup tests

## Phase 4 — Build integration

- [x] `core/build/build.go`
  - [x] Вставить `MergePresetsIntoOutbounds` в outbounds case перед `BuildOutboundsSection`
  - [x] route case: `CleanDanglingOutboundsInRouteRules` после `MergePresetsIntoRoute` (только при active preset-refs)
  - [x] `BuildContext.allOutboundTags` precomputed + `collectAllOutboundTagsForBuild`
- [x] Integration через existing test suite — все 24 пакета зелёные

## Phase 5 — UI

- [x] `ui/configurator/business/outbound.go`
  - [x] `collectActivePresetOutboundTags(model) []string`
  - [x] Расширить `GetAvailableOutbounds` — append preset tags
- [x] `ui/configurator/tabs/rules_unified_rows.go`
  - [x] Refresh tab on outbound-affecting preset enable/disable toggle

## Phase 6 — Template content + cleanup

- [x] `bin/wizard_template.json`:
  - [x] Удалить `filters: !RU` из global `proxy-out` и `auto-proxy-out`
  - [x] Удалить `ru VPN 🇷🇺` global selector
  - [x] Добавить в `ru-inside` preset:
    - [x] `mode: "update"` для proxy-out + auto-proxy-out → `filters: !RU`
    - [x] `mode: "add"` для `ru VPN 🇷🇺` selector
  - [x] `ru-inside.vars.out.default` остаётся `"ru VPN 🇷🇺"` (preset-emitted)

## Phase 7 — Docs

- [x] `docs/release_notes/upcoming.md` — SPEC 055 entry (EN + RU)
- [x] `SPECS/055-F-N-PRESET_OUTBOUNDS/IMPLEMENTATION_REPORT.md` — финальный отчёт
- [ ] Rename SPEC dir: `055-F-N-` → `055-F-C-` (после QA-теста — отложено)

## Out of scope (future)

- [ ] SPEC 056 — preset.inbounds (per-preset inbound configuration)
- [ ] SPEC 057 — explicit cross-preset dependencies
- [ ] SPEC 058 — `mode: "replace"` (destructive full-replace)
- [ ] Template content: продумать аналогичный паттерн для `ru-blocked` / `russian`
