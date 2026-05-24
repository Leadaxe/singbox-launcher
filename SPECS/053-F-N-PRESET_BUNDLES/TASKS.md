# SPEC 053 — Tasks

**Status:** ✅ Все 8 фаз shipped + интеграция в build pipeline + UI handlers
(DNS overrides, Convert to user rule, лок-иконка для bundled DNS rows,
row-list DNS rules с Add/View All) + template content (6 preset'ов).
Подробности — `IMPLEMENTATION_REPORT.md`.

## Phase 1 — Pure data types

### Template-side
- [x] `core/template/preset_types.go`
  - [x] `Preset` struct (ID, Label, Description, DefaultEnabled, Vars, RuleSet, DNSServers, Rule, DNSRule)
  - [x] `PresetVar` (Name, Type, Default, Title, Tooltip, Options json.RawMessage, Select, If, IfOr)
  - [x] `OptionEntry` (Title, Value) — для enum
  - [x] `PresetRuleSet` (Tag, Type, Format, Rules, URL, If, IfOr)
  - [x] `PresetDNSServer` (Tag, Type, Server, ServerPort, Path, TLS, Detour, Title, Description, If, IfOr)
  - [x] Custom UnmarshalJSON для Options (object form для enum, []string для dns_server/outbound)
- [x] `core/template/preset_types_test.go` — round-trip + decoder tests (8 cases)

### State-side
- [x] `core/state/v6/rule_types.go` — Rule header + InlineBody/SrsBody/PresetBody + DNSConfig + DecodeRule
- [x] `core/state/v6/rule_types_test.go` — 14 round-trip + error cases

## Phase 2 — Template parser + validation

- [x] `core/template/preset_loader.go` — LoadPresets + full validator taxonomy
- [x] `core/template/preset_loader_test.go` — 17 validation cases
- [x] `core/template/loader.go` — Parse `presets[]`, log warnings, keep legacy `selectable_rules[]` parser

## Phase 3 — Preset expansion engine

- [x] `core/build/preset_expand.go` — substitute / evalIf / filterByIf / prefixTags / filterDnsServers / applyOutboundSentinels / cleanDanglingRuleSetRefs / directOutDetourStrip / ExpandPreset
- [x] `core/build/preset_expand_test.go` — 12 cases incl. 4 golden для ru-direct

## Phase 4 — State v5 → v6 migration

- [x] `core/state/v6/migration.go` — MigrateV5ToV6 (pure)
- [x] `core/state/v6/migration_test.go` — real-fixture + idempotency + round-trip (11 cases)
- [x] `core/state/load.go` — parseV6 ветка (read v5 + v6) + legacyCustomRulesFromV6 / legacyDNSOptionsFromV6 views
- [x] `core/state/save.go` — marshalDiskV6 + atomic `state.json.v5.bak` (one-time, idempotent)
- [x] `core/state/v6_integration_test.go` — 7 load/save/backup tests

## Phase 5 — Build pipeline integration

- [x] `core/build/rules_pipeline.go` — BuildRulesAndDNS orchestrator (preset expansion + user inline/srs + DNS merge с effective_enabled)
- [x] `core/build/rules_pipeline_test.go` — 14 cases (mixed/identical-skip/first-wins/effective_enabled)
- [x] `core/build/preset_merge.go` (NEW) — `MergePresetsIntoRoute` + `MergePresetsIntoDNS` дополнительный pass поверх legacy merge'а
- [x] `core/build/preset_merge_test.go` — 8 unit-tests
- [x] `core/build/build.go` — `BuildContext.Preset` поле + вызов из `buildSection` для секций `dns` и `route`
- [x] `core/config_service.go` — `buildContextFromState` заполняет `ctx.Preset` из `td.Presets + s.RulesV6 + s.DNSV6`
- [x] `internal/outboundutil/outbound.go` (NEW) — shared reject/drop sentinel utility (используется в `preset_expand.go` и legacy `applyCustomRules`)

## Phase 6 — UI: Rules tab refactor

- [x] `ui/configurator/models/preset_ref_state.go` (NEW) — PresetRefState
- [x] `ui/configurator/models/preset_ref_sync.go` (NEW) — Sync UI ↔ v6
- [x] `ui/configurator/models/wizard_model.go` — `PresetRefs` + `DNSTemplateOverrides` поля
- [x] `ui/configurator/models/rule_slot.go` (NEW) — Unified `RuleSlot` ordered list (custom + preset-refs одной колонкой) + `RuleOrder`
- [x] `ui/configurator/presentation/preset_ref_helpers.go` (NEW)
- [x] `ui/configurator/presentation/presenter_state.go` — Save пишет state.RulesV6/DNSV6, Load восстанавливает model.PresetRefs/DNSTemplateOverrides
- [x] `ui/configurator/tabs/library_rules_dialog.go` — Library показывает обе секции (SelectableRules + Presets), Add создаёт preset-ref
- [x] `ui/configurator/tabs/rules_unified_rows.go` (NEW) — preset-ref tile с lock icon, enable/disable, vars edit dialog, SRS cloud download button. **Заменяет** старый `preset_ref_rows.go` (объединение в один список с `RuleSlot`).
- [x] `ui/configurator/tabs/preset_ref_edit_dialog.go` (NEW) — showEditPresetRefDialog: универсальный rendering form по PresetVar.Type, broken preset handling, Convert to user rule
- [x] `ui/configurator/tabs/preset_ref_convert.go` (NEW) — convertPresetRefToUserRules helper
- [x] `ui/configurator/tabs/preset_ref_srs.go` (NEW) — SRS cloud download для bundled rule_set
- [x] `ui/configurator/dialogs/add_rule_dialog.go` — 5 NewCheck заменены на `widget.NewRadioGroup` (Rule Type radio) + conditional domain mode / match-by-path rows

## Phase 7 — UI: DNS tab refactor

- [x] `ui/configurator/business/wizard_dns.go` — `DNSEnabledTagOptions` для server picker'ов
- [x] `ui/configurator/business/preset_bundled_dns.go` (NEW) — resolver bundled DNS-серверов от active preset-ref'ов
- [x] `ui/configurator/tabs/dns_tab.go`:
  - [x] DNS tab moved **после** Rules tab
  - [x] Layout: serversHeader → serversScroll → strategyAndCacheRow → rulesLabel → bundledRulesBox → userRulesBox → rulesButtons → finalAndResolverRow
  - [x] Legacy `rulesBlock` MultiLineEntry скрыт из UI (`_ = rulesBlock`); состояние читается/пишется через row-list
  - [x] Checkbox handler `setDNSServerEnabledAt` пишет в `model.DNSTemplateOverrides[tag] = enabled` + legacy DNSServers
  - [x] `[+ Add Rule]` + spacer + `[View All DNS Rules]` кнопки под user rules
- [x] `ui/configurator/tabs/dns_user_rules.go` (NEW):
  - [x] Row-list user-defined DNS rules (заменяет JSON MultiLineEntry)
  - [x] `[+ Add Rule]` → отдельное fyne window 500×600 с Form/JSON tabs, radio SRS/Inline, server picker
  - [x] `[View All DNS Rules]` popup — compiled bundled+user preview (RichText JSON, read-only)
  - [x] dnsRuleSummary helper для строкового описания
- [x] `ui/configurator/tabs/dns_preset_bundled.go` (NEW):
  - [x] Read-only rows для bundled DNS servers + DNS rules от active preset-ref'ов
  - [x] 🔒 lock icon в позиции checkbox'а (через `lockLeading()` placeholder)
  - [x] Single-line title `<preset-label> (<tag>)` + tooltip с деталями
  - [x] `[View JSON]` button открывает RichTextFromMarkdown popup
  - [x] Italic-стиль для help-текста (вместо серого LowImportance)

## Phase 8 — Content + cleanup + docs

- [x] `bin/wizard_template.json`:
  - [x] Добавлена `presets[]` секция с 6 preset'ами (private-ips-direct, local-lan-domains, bittorrent-direct, block-ads, ru-direct-preset, ru-inside)
  - [x] ru-direct: 5 vars (out, use_dns_override, dns_server select=local, dns_ip с 10 IP options, geoip_enabled), 3 rule_set, 3 dns_servers (yandex_udp/doh/dot c if=use_dns_override), 3 rule refs + 2 dns_rule refs
- [x] `internal/locale/en.json` + 9 локалей: новые ключи `wizard.dns.section_from_active_presets`, `wizard.rules.button_convert_to_user`, `wizard.rules.dialog_edit_rule_title`, `wizard.shared.button_save`, `wizard.add_rule.domain_mode_label`; rename `tab_raw` "Raw" → "JSON"
- [x] `docs/release_notes/upcoming.md` — SPEC 053 entry (EN + RU)
- [x] `SPECS/053-F-N-PRESET_BUNDLES/IMPLEMENTATION_REPORT.md`
- [ ] `docs/ARCHITECTURE.md` — Раздел "SPEC 053 — Preset bundles" (TODO)
- [ ] `docs/RELEASE_PROCESS.md` §5.2 — заметка про bump RequiredTemplateRef критично для distribution preset content (TODO, low priority)
- [ ] Rename SPEC dir: `053-F-N-` → `053-F-C-` (после QA-теста)

## Misc fixes shipped в той же серии

- [x] `ui/clash_api_tab.go` — guard `onLoadAndRefreshProxies` за `IsRunning()`, чтобы не показывать "Clash API disabled" popup при закрытии Configurator
- [x] SPEC 054 created: `SPECS/054-B-N-XRAY_JSON_PREVIEW_NODES_BLOAT/SPEC.md` — найденный bug в Xray JSON-array subscription parser'е (preview_nodes раздувают state.json)

## Out of scope (отдельные SPEC'и в будущем)

- [ ] **SPEC 055 PRESET_IMPORT_EXPORT** — JSON import/export пользовательских пресетов
- [ ] **SPEC 056 LIVE_VARS_RECONFIGURE** — изменение varsValues без полного reconfigure sing-box (требует sing-box runtime API)
- [ ] **SPEC 057 PRESET_DIAGNOSTICS_UI** — debug view раскрытого preset'а с expand trace
- [ ] **More var types** — `process_name` / `package_name` picker'ы из running apps
- [ ] **Per-platform presets** — `platforms[]` поле в Preset (analog `TemplateSelectableRule.Platforms`)
