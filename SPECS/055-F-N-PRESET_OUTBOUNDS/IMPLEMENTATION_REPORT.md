# SPEC 055 — Implementation Report

**Status:** ✅ **COMPLETE** — все 7 фаз landed end-to-end. Build green, все 24 пакета зелёные, 21 новый unit-тест (7 loader + 4 expand + 7 merge + 3 cleanup).

---

## Phases delivered

### Phase 1 — Types + loader ✅
- `core/template/preset_types.go`:
  - `Preset.Outbounds []PresetOutbound` field
  - `PresetOutbound{Mode, Tag, Type, Options, Filters, AddOutbounds, Comment, Wizard, If, IfOr}`
- `core/template/preset_loader.go::validatePresetOutbounds`:
  - mode ∈ {"", "add", "update"} — empty normalized to "add"; unknown → strip
  - tag non-empty required
  - mode=add → type required
  - mode=update → type warned (drop at expand time, stays in struct)
  - Tag uniqueness within preset
- 7 unit-tests в `preset_outbounds_test.go`

### Phase 2 — Expand engine ✅
- `core/build/preset_expand.go`:
  - `PresetFragments.Outbounds []ExpandedOutbound`
  - `ExpandedOutbound{Mode, Tag, Body}`
  - Substitute `@var` в options/filters/addOutbounds
  - Filter by if/if_or
  - Drop control fields (mode, if, if_or) из Body
  - Drop type для mode=update (loader уже warned)
- 4 unit-tests

### Phase 3 — Merge pipeline ✅
- `core/build/preset_merge.go`:
  - `MergePresetsIntoOutbounds(baseOutbounds, ctx) → mergedOutbounds`
  - Iterate active preset-refs in RuleOrder
  - mode=add: identical-skip / first-wins + warning
  - mode=update: lookup target, apply `applyOutboundUpdate`
  - `applyOutboundUpdate(target, patch, presetID)` — field-level merge:
    - filters → replace whole map
    - addOutbounds → `unionStringList`
    - options.*/wizard.* → replace per-field (только заданные в patch)
    - type/tag → drop (нельзя менять)
    - comment + остальные → replace
  - `cleanDanglingOutboundRefInRule(rule, emittedTags, fallback)` — sentinel reject/drop preserved
  - `CleanDanglingOutboundsInRouteRules(routeRaw, emittedTags, routeFinal)` — pass over route.rules
  - `CollectOutboundTagsFromRaw(rawOutbounds)` — helper для extract tags
- 7 unit-tests merge + 4 cleanup

### Phase 4 — Build integration ✅
- `core/build/build.go`:
  - `BuildContext.allOutboundTags map[string]bool` (internal, computed inside buildOrderedSections)
  - `collectAllOutboundTagsForBuild(outboundsRaw, ctx)` — union template static (after merge) + dynamic cache + always `direct-out`
  - В `buildSection`:
    - case "outbounds": apply MergePresetsIntoOutbounds → BuildOutboundsSection
    - case "route": после MergePresetsIntoRoute → CleanDanglingOutboundsInRouteRules (только когда есть active preset-refs — иначе legacy behavior без cleanup'а, чтобы не сломать test-фикстуры с tag'ами не присутствующими в template)

### Phase 5 — UI integration ✅
- `ui/configurator/business/outbound.go`:
  - `GetAvailableOutbounds`: добавлены preset-emitted tag'и через `collectActivePresetOutboundTags`
  - `collectActivePresetOutboundTags(model)`: walks `model.PresetRefs`, для каждого enabled зовёт `build.ExpandPreset`, собирает `frags.Outbounds[].Tag` (только mode="add", т.к. update не вводит новых тегов)
- `ui/configurator/tabs/rules_unified_rows.go`:
  - Checkbox enable callback: если preset имеет outbounds → `refreshRulesTabFromPresenter` для обновления inline outbound selects во всех rows
- Inline outbound select (HoverForwardSelect) — без изменений в коде, automatically picks up new tags на следующем render'е

### Phase 6 — Template content migration ✅
- `bin/wizard_template.json`:
  - `parser_config.outbounds[]::proxy-out` + `auto-proxy-out`: убран `filters: { tag: "!/(🇷🇺)/i" }` (нейтральный default)
  - Удалён глобальный `ru VPN 🇷🇺` selector
  - `ru-inside` preset получил `outbounds[]`:
    - `mode: "update"` на proxy-out + auto-proxy-out → adds `!/(🇷🇺)/i` filter
    - `mode: "add"` для `ru VPN 🇷🇺` selector (с RU-only filter + addOutbounds direct-out)
  - `ru-inside.vars.out.default` теперь `"ru VPN 🇷🇺"` (раньше был тот же тег, но из globals — теперь из preset.outbounds)
  - Description обновлён: упоминает что preset патчит proxy-out и приносит `ru VPN 🇷🇺`

### Phase 7 — Tests + docs ✅
- 21 unit-tests across 2 файла:
  - `core/template/preset_outbounds_test.go` — 7
  - `core/build/preset_outbounds_test.go` — 14
- `docs/release_notes/upcoming.md` — SPEC 055 entry (EN + RU)
- `SPECS/055-F-N-PRESET_OUTBOUNDS/IMPLEMENTATION_REPORT.md` (this file)

---

## Backward compatibility

- **State.json** — без изменений (v6 schema as-is, preset-refs хранят только `{ref, vars}`)
- **Preset без `outbounds[]`** — работает identical как раньше (SPEC 053 behavior)
- **Old template без preset.outbounds** — нормально, loader просто пропустит
- **User config с enabled ru-inside**: до обновления template имели `proxy-out` с !RU + global `ru VPN 🇷🇺` selector. После обновления template получают то же самое, но теперь через preset (mode=update + mode=add). Идентичный config.json на выходе.
- **Без preset'ов вообще**: cleanup для dangling outbound refs **не срабатывает** (защита от false-positives в legacy tests/fixtures где outbound ссылается на тег не в template).

---

## Test coverage

| Layer | Tests | Status |
|---|---|---|
| core/template/preset_outbounds (loader validation) | 7 | ✅ |
| core/build/preset_outbounds (expand + merge + cleanup) | 14 | ✅ |
| Existing regression tests (24 packages) | all | ✅ |

`go build ./...` ✅
`go test ./...` ✅

---

## Architecture invariants preserved

1. **Self-contained presets** — outbounds bundle вместе с rule_set/dns_servers/dns_rule
2. **Tag scope** — outbound tag'и НЕ префиксуются preset_id (user-facing global namespace), в отличие от rule_set/dns_servers
3. **Explicit mode** — никаких implicit override; mode="update" обязателен для патча
4. **Conservative cleanup** — dangling outbound cleanup только при наличии active preset-refs (защита от ложно-положительных срабатываний)
5. **First-wins для collisions** — детерминированный (по RuleOrder), warning в логи; idempotent для identical bodies

---

## Files changed

```
SPECS/055-F-N-PRESET_OUTBOUNDS/
├── SPEC.md                         (285 LOC)
├── PLAN.md                         (253 LOC)
├── TASKS.md                        (110 LOC — updated)
└── IMPLEMENTATION_REPORT.md        (this file)

core/template/
├── preset_types.go                 (+62 LOC — Outbounds field + PresetOutbound struct)
├── preset_loader.go                (+87 LOC — validatePresetOutbounds)
└── preset_outbounds_test.go        (NEW, 157 LOC, 7 tests)

core/build/
├── preset_expand.go                (+57 LOC — outbounds emit в ExpandPreset)
├── preset_merge.go                 (+213 LOC — MergePresetsIntoOutbounds + applyOutboundUpdate + unionStringList + outboundBodiesIdentical + cleanDanglingOutboundRefInRule + CleanDanglingOutboundsInRouteRules + CollectOutboundTagsFromRaw)
├── build.go                        (+45 LOC — outbounds case wire + collectAllOutboundTagsForBuild + route case cleanup)
└── preset_outbounds_test.go        (NEW, 296 LOC, 14 tests)

ui/configurator/business/
└── outbound.go                     (+50 LOC — collectActivePresetOutboundTags + GetAvailableOutbounds integration)

ui/configurator/tabs/
└── rules_unified_rows.go           (+6 LOC — refresh tab on outbound-affecting preset toggle)

bin/
└── wizard_template.json            (modified — RU filters moved into ru-inside preset)

docs/release_notes/
└── upcoming.md                     (modified — SPEC 055 EN + RU entries)
```

**Total new code: ~1180 LOC** (incl. tests + docs + SPEC).

---

## Known follow-ups (out of scope)

- **SPEC 056** preset.inbounds — preset может добавлять inbound (например per-user proxy-in port)
- **SPEC 057** preset cross-references — explicit dependency между preset'ами
- **SPEC 058** mode="replace" — destructive full-replace (если когда-то понадобится)
- **UI debug view**: chain of updates по tag'у (`tag X: updated by ru-inside (filters); updated by ru-blocked (addOutbounds)`)
- **Template content**: продумать `ru-blocked` / `russian` — возможно тоже надо `update proxy-out`-эффект?
