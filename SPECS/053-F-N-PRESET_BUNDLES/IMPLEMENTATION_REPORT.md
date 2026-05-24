# SPEC 053 — Implementation Report

**Status:** ✅ **COMPLETE** — все 8 фаз + интеграция в основной build pipeline + UI handlers (DNS overrides, Convert to user rule) + template content (6 ready-to-use preset'ов). Build green, все 23 пакета тестов зелёные.

---

## Phases delivered

### Phase 1 — Pure data types ✅
- `core/template/preset_types.go` (232 LOC) — Preset, PresetVar, OptionEntry, PresetRuleSet, PresetDNSServer
- `core/state/v6/rule_types.go` (205 LOC) — Rule header/body, DNSConfig with TemplateServers/ExtraServers/ExtraRules
- `core/state/v6/state.go` (34 LOC) — v6 State with MetaSection (schema: "presets_v1")
- 22 unit-tests

### Phase 2 — Template parser + validation ✅
- `core/template/preset_loader.go` (485 LOC) — LoadPresets with full validation taxonomy
- Loader integration в `core/template/loader.go` — TemplateData получает Presets + PresetWarnings
- 17 unit-tests

### Phase 3 — Expansion engine ✅
- `core/build/preset_expand.go` (520 LOC) — substitute / if-filter / tag-prefix / filter dns_servers / outbound sentinels / dangling refs cleanup
- 12 unit-tests включая 4 golden cases для ru-direct

### Phase 4 — Migration + load/save integration ✅
- `core/state/v6/migration.go` (268 LOC) — MigrateV5ToV6 с heuristic kind detection
- `core/state/load.go` — parseV6 ветка (read v5 + v6), legacyCustomRulesFromV6, legacyDNSOptionsFromV6 для backward-compat UI
- `core/state/save.go` — marshalDiskV6, hasPresetRefs, maybeBackupV5 (atomic `.v5.bak` создаётся при первом v5→v6 upgrade'е)
- `State.RulesV6 / DNSV6` — новые поля параллельно legacy
- 11 + 7 unit-tests (migration_test + v6_integration_test)

### Phase 5 — Build pipeline ✅
- `core/build/rules_pipeline.go` (310 LOC) — BuildRulesAndDNS orchestrator: preset expansion + user inline/srs + DNS merge с effective_enabled
- 14 unit-tests

### Phase 6 — UI Rules tab refactor ✅
- `ui/configurator/models/preset_ref_state.go` (38 LOC) — PresetRefState
- `ui/configurator/models/preset_ref_sync.go` (90 LOC) — Sync helpers UI ↔ v6
- `ui/configurator/models/wizard_model.go` — WizardModel.PresetRefs + DNSTemplateOverrides
- `ui/configurator/presentation/preset_ref_helpers.go` (40 LOC) — PresetRefForUI
- `ui/configurator/presentation/presenter_state.go` — Save (CreateStateFromModel) пишет state.RulesV6/DNSV6 из model; Load (LoadStateFromFile) восстанавливает model.PresetRefs/DNSTemplateOverrides
- `ui/configurator/tabs/library_rules_dialog.go` — Library показывает обе секции (SelectableRules + Presets), Add создаёт preset-ref
- `ui/configurator/tabs/preset_ref_rows.go` (170 LOC) — buildPresetRefRows: drag/enable/edit/delete tiles в Rules tab
- `ui/configurator/tabs/preset_ref_edit_dialog.go` (280 LOC) — showEditPresetRefDialog: универсальный rendering form по PresetVar.Type, broken preset handling

### Phase 7 — DNS tab (partial) ⚠
- DNS save path использует существующий legacy DNSServers; sync с template_servers overrides в state.DNSV6 пишется через `SyncDNSToStateV6` (пока пуст — UI handler для checkbox'а template-сервера не подключён).
- При наличии preset-ref'ов save переключается на v6 формат, но DNS section в v6 эмитит template_servers только если model.DNSTemplateOverrides не пуст. Юзер пока **не редактирует** template_servers через UI — это TODO для отдельной сессии.

### Phase 8 — Template content ✅
- `bin/wizard_template.json` получил секцию `"presets": [...]` рядом с legacy `"selectable_rules": [...]`
- Два первых preset'а: `private-ips-direct` (Form 1: inline match) + `ru-direct-preset` (Form 2: vars + bundled DNS + if)
- Юзер видит их в Library dialog → "Add" → появляются в Rules tab → редактируются через новый dialog → сохраняются в state.json как v6

---

## State.json format flow

```
Fresh install / pure inline/srs rules        → v5 format (current behavior)
                                                                ↓
Юзер добавил preset-ref через Library dialog → next Save: v6 format
                                                + state.json.v5.bak (one-time backup)
                                                                ↓
Subsequent edits → v6 format (backup НЕ перезаписывается, idempotent)
                                                                ↓
Юзер удалил все preset-ref'ы → next Save: v5 format снова
                                  (backup .v5.bak остаётся)
```

---

## Test coverage

| Layer | Tests | Status |
|---|---|---|
| core/template/preset_types | 8 | ✅ |
| core/template/preset_loader | 17 | ✅ |
| core/state/v6/rule_types | 14 | ✅ |
| core/state/v6/migration | 11 | ✅ |
| core/state/v6_integration (load/save/backup) | 7 | ✅ |
| core/build/preset_expand | 12 | ✅ |
| core/build/rules_pipeline | 14 | ✅ |
| **Total new tests** | **83** | **all green** |
| Existing tests (regression) | all packages | ✅ |

`go build ./...` ✅
`go test ./...` ✅ (23 packages green)

---

## Build pipeline integration ✅

- `core/build/preset_merge.go` (NEW, ~270 LOC) — `MergePresetsIntoRoute` + `MergePresetsIntoDNS`: дополнительный pass поверх legacy `MergeRouteSection` / `MergeDNSSection`. **Не ломает** старый код — если preset-ref'ов нет в state, pipeline noop.
- `core/build/build.go` — `BuildContext.Preset` поле + вызов из `buildSection` для секций `dns` и `route`.
- `core/config_service.go` — `buildContextFromState` теперь заполняет `ctx.Preset` из `td.Presets + s.RulesV6 + s.DNSV6`.
- `core/build/preset_merge_test.go` (NEW, ~190 LOC) — 8 unit-тестов: noop, append rule, disabled skip, broken ref, bundled DNS, override disable, extra servers, helper.

## UI handlers (Phase 7) ✅

- `ui/configurator/tabs/dns_tab.go::setDNSServerEnabledAt` — параллельно с legacy DNSServers пишет в `model.DNSTemplateOverrides[tag] = enabled`. На Save синхронизируется в `state.DNSV6.TemplateServers` через `SyncDNSToStateV6`. Build pipeline (`MergePresetsIntoDNS`) применяет override при наличии preset-ref'ов.

## Convert to user rule ✅

- `ui/configurator/tabs/preset_ref_edit_dialog.go` — кнопка "Convert to user rule(s)" в edit dialog'е preset-ref'а.
- `ui/configurator/tabs/preset_ref_convert.go` (NEW, ~130 LOC) — `convertPresetRefToUserRules`:
  - inline rule_set → `CustomRule(kind=inline)` с match'ом из rule_set.rules[0]
  - remote rule_set → `CustomRule(kind=srs)` (юзер должен повторно скачать .srs)
  - без rule_set'ов (inline match в preset.rule) → один inline rule
- Удаляет preset-ref после конверсии. Confirmation dialog предупреждает о потере связи с template.

## Template content ✅

`bin/wizard_template.json` теперь содержит 6 ready-to-use preset'ов:
1. `private-ips-direct` — Form 1 (inline match)
2. `local-lan-domains` — Form 2 (single inline rule_set)
3. `bittorrent-direct` — Form 1 (protocol match)
4. `block-ads` — Form 5 (remote SRS + reject sentinel)
5. `ru-direct-preset` — Form 2 full (vars + bundled DNS + conditional fragment)
6. `ru-inside` — Form 2 (remote SRS)

Юзер видит их в Library dialog → "Add" → preset-ref в Rules tab → edit through dedicated dialog → save в state.json v6.

## Known follow-ups (low priority)

1. **Portable selectable_rules → presets full migration** — 17 текущих legacy `selectable_rules[]` остаются как deep-clone в `CustomRules`. Постепенный портинг по мере накопления feedback.
2. **Per-platform presets** — текущий формат `Preset` не имеет `platforms[]` поля. Если потребуется условный preset по ОС — расширить тип (analog `TemplateSelectableRule.Platforms`).
3. **JSON import/export user preset bundles** — отдельная фича.
4. **More var types** — `process_name` / `package_name` picker'ы из running apps. Out of current scope.

---

## Architecture invariants preserved

1. **Self-contained presets** — все ссылки локальны (validator + auto-prefix `<preset_id>:<tag>`)
2. **Thin state** — preset-ref хранит только `{ref, vars}`; match-поля в template
3. **Substitute = тупая текстовая замена** (no `_Dropped` sentinel'ов)
4. **Опциональность через `if`/`if_or`** (никаких optional/required: false flag'ов)
5. **`reject`/`drop` literal sentinels** в outbound (без sigil'а)
6. **All vars required** в template
7. **Backward compat:** юзеры без preset-ref'ов продолжают работать на v5 без изменений; v5→v6 upgrade автоматический + idempotent backup

---

## Files changed

```
SPECS/053-F-N-PRESET_BUNDLES/
├── SPEC.md                         (1083 LOC — финальная спецификация)
├── PLAN.md                         (314 LOC — фазы / sequencing / риски)
├── TASKS.md                        (190 LOC — атомарные задачи)
└── IMPLEMENTATION_REPORT.md        (this file)

core/template/
├── preset_types.go                 (NEW, 232 LOC)
├── preset_types_test.go            (NEW, 329 LOC)
├── preset_loader.go                (NEW, 485 LOC)
├── preset_loader_test.go           (NEW, 371 LOC)
└── loader.go                       (MODIFIED — TemplateData.Presets + LoadPresets call)

core/state/v6/                      (NEW package)
├── state.go                        (34 LOC)
├── rule_types.go                   (205 LOC)
├── rule_types_test.go              (284 LOC)
├── migration.go                    (268 LOC)
└── migration_test.go               (333 LOC)

core/state/
├── state.go                        (MODIFIED — RulesV6 + DNSV6 fields)
├── load.go                         (MODIFIED — parseV6 + legacy view generators)
├── save.go                         (MODIFIED — marshalDiskV6 + maybeBackupV5)
└── v6_integration_test.go          (NEW, 230 LOC)

core/build/
├── preset_expand.go                (NEW, 520 LOC)
├── preset_expand_test.go           (NEW, 418 LOC)
├── rules_pipeline.go               (NEW, 310 LOC)
└── rules_pipeline_test.go          (NEW, 343 LOC)

ui/configurator/models/
├── preset_ref_state.go             (NEW, 38 LOC)
├── preset_ref_sync.go              (NEW, 90 LOC)
└── wizard_model.go                 (MODIFIED — PresetRefs + DNSTemplateOverrides)

ui/configurator/presentation/
├── preset_ref_helpers.go           (NEW, 40 LOC)
└── presenter_state.go              (MODIFIED — Sync on Save + Restore on Load)

ui/configurator/tabs/
├── library_rules_dialog.go         (MODIFIED — Presets section + Add preset-ref)
├── preset_ref_rows.go              (NEW, 170 LOC)
├── preset_ref_edit_dialog.go       (NEW, 280 LOC)
├── preset_ref_edit_dialog_util.go  (NEW, 7 LOC)
└── rules_tab.go                    (MODIFIED — call buildPresetRefRows)

bin/wizard_template.json            (MODIFIED — added "presets": [...] section)

docs/release_notes/upcoming.md      (MODIFIED — SPEC 053 entry)
```

**Total new code: ~6700 LOC** (incl. tests + docs).
