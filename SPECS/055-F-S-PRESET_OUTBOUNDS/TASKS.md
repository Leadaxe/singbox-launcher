# SPEC 055 — Tasks (consolidated)

**Status:** ✅ Shipped (S). Все фазы выполнены. Этот файл — финальный
checkpoint всей истории; полная implementation history в
`IMPLEMENTATION_REPORT.md`, актуальный план в `IMPLEMENTATION_PLAN.md`,
feature semantics — в `SPEC.md`.

История работы:

```
Изначальный план 055 (post-merge JSON-patch)
    ↓ провалился (launcher fields leak, sing-box 1.12+ FATAL)
Revert 098c5e1
    ↓
Pre-patch parser_config (ex-SPEC 056)
    ↓ shipped
Post-ship QA вскрыл DNS-баги того же класса
    ↓
DNS schema cleanup (ex-SPEC 057, попытка → revert → корректный invariant)
    ↓
Single SPEC 055 (этот) covers всё
```

---

## Phase 1 — Types & loader — [DONE `4756b39`]

- [x] `core/template/preset_types.go`
  - [x] `Outbounds []PresetOutbound` field в `Preset`
  - [x] `PresetOutbound` struct (Mode, Tag, Type, Options, Filters, AddOutbounds, PreferredDefault, Comment, Wizard, If, IfOr)
- [x] `core/template/preset_loader.go::validatePresetOutbounds`:
  - [x] mode ∈ {"", "add", "update"} (empty → "add"; unknown → strip)
  - [x] tag non-empty
  - [x] mode=add → type required
  - [x] mode=update → type stripped + warning
  - [x] tag uniqueness within preset
- [x] `core/template/preset_outbounds_test.go` — 9 unit tests

## Phase 2 — Expand engine — [DONE `2b2e77a`]

- [x] `core/build/preset_outbounds.go::ExpandPresetOutbounds(preset, vars)`
  - [x] Substitute `@var` в options/filters/addOutbounds
  - [x] Filter by if/if_or (preset-level + entry-level)
  - [x] Convert to `configtypes.OutboundConfig` (parser-format)
  - [x] Drop control fields (mode, if, if_or) перед re-unmarshal'ом

## Phase 3 — Pre-patch core — [DONE `2b2e77a`]

- [x] `core/build/preset_outbounds.go::ApplyPresetOutboundsToParserConfig`
  - [x] Deep-clone parserCfg.ParserConfig.Outbounds[] (immutable original)
  - [x] Walk rules in order (Kind=preset && Enabled)
  - [x] mode=add: identical-skip / first-wins (template global vs earlier preset)
  - [x] mode=update: lookup target by tag, applyOutboundUpdate
  - [x] `applyOutboundUpdate(target, patch)` типизированный helper:
    - [x] filters → replace
    - [x] addOutbounds → union (`unionStringList`)
    - [x] options.* → replace per-field
    - [x] wizard → replace
    - [x] type/tag → immutable
    - [x] comment → replace if non-empty
    - [x] preferredDefault → replace

## Phase 4 — Wire pre-patch — [DONE `8fb10f7`]

- [x] `core/rebuild_raw_cache.go::buildSnapshotFromRawCache` — новый `td` param
- [x] `core/rebuild.go` — LoadTemplateData moved before Step 2
- [x] `core/config_service.go::UpdateConfigFromSubscriptions` — inline pre-patch
- [x] `ui/configurator/business/parser.go::ParseAndPreview` — preview pre-patch
- [x] `core/rebuild_raw_cache_test.go` — signatures updated

## Phase 5 — Route post-pass cleanup — [DONE `2d16895`]

- [x] `core/build/preset_outbounds.go::cleanDanglingOutboundRefInRule`
- [x] `core/build/preset_outbounds.go::CleanDanglingOutboundsInRouteRules`
- [x] `core/build/build.go::buildOrderedSections` — precompute `finalOutboundTags`
- [x] `core/build/build.go::buildSection("route")` — cleanup pass после `MergePresetsIntoRoute`
- [x] Skip cleanup в preview (`ctx.ForPreview=true`)
- [x] Sentinel literals (reject/block/drop/direct/dns-out) preserved

## Phase 6 — UI integration — [DONE `c20b24a`]

- [x] `ui/configurator/business/outbound.go::collectActivePresetOutboundTags`
- [x] `ui/configurator/business/outbound.go::GetAvailableOutbounds` augmented
- [x] `ui/configurator/tabs/rules_unified_rows.go::presetHasAddOutbounds` helper
- [x] Toggle callback вызывает `RefreshOutboundOptions` + `refreshRulesTabFromPresenter` когда preset has add-outbounds; anti-loop защита из dc4cf09 + 0ecc403 сохранена

## Phase 7 — Template content migration — [DONE `ee6e8e4` + `b745f1d`]

- [x] `bin/wizard_template.json::parser_config.outbounds` — снят `filters: !RU` из `proxy-out` и `auto-proxy-out`
- [x] Удалён global `ru VPN 🇷🇺` selector
- [x] `presets[ru-inside].outbounds` — `mode=update` + `mode=add` для `ru VPN 🇷🇺`
- [x] `presets[russian].outbounds` — `mode=update` + `mode=add` ru VPN
- [x] `presets[ru-blocked].outbounds` — только `mode=update` !RU
- [x] `internal/constants/constants.go::RequiredTemplateRef` — bump на `ee6e8e4`

## Phase 8 — Tests + docs — [DONE `23a7b10`]

- [x] `core/template/preset_outbounds_test.go` — 9 unit tests (validatePresetOutbounds)
- [x] `core/build/preset_outbounds_test.go` — 18 unit tests (Apply / applyOutboundUpdate / Clean / Expand)
- [x] `docs/release_notes/upcoming.md` — SPEC entry (EN + RU)
- [x] `IMPLEMENTATION_REPORT.md` — финальный отчёт

## Phase 9 — Post-ship DNS schema cleanup (ex-SPEC 057, merged-in)

QA вскрыл регрессии **того же архитектурного класса** в DNS pipeline.

- [x] **9.1** `9daa3cd` — DNS sanitize unification: `stripDNSWizardOnlyFields`
  single source of truth (description/enabled/title/if/if_or/_*). Применяется
  во ВСЕХ DNS server emit-путях. Plus `cleanDanglingDNSRule` (зеркало route
  Phase 5 для DNS rules).

- [x] **9.2** `c60fd63` — User inline route rules эмитятся напрямую в
  `route.rules[]` без `rule_set` wrapping. Sing-box headless rule_set
  отвергает connection-level match-поля (protocol/inbound/...).

- [x] **9.3** `e96c86a` — Template DNS library материализация:
  `parseTemplateDNSDefaultsFromTD` + populate `ctx.Preset.TemplateDNSDefaults` +
  emit в `MergePresetsIntoDNS`. Раньше library была доступна только тестовому
  `BuildRulesAndDNS`.

- [x] **9.4** `4eb7b7d` — `isV6DNSActive` guard в `dnsConfigForUpdate` против
  double-emit DNS extras когда state v6.

- [x] **9.5 + 9.6** `edd4565` (attempted) + `79b1ce3` (reverted) — попытка
  удалить `ExtraServers`/`ExtraRules` из схемы. Откачено: extras легитимны для
  **genuinely user-defined** содержимого (`my-pihole` 192.168.1.5 — нет в
  template, ссылаться не на что). Корректный invariant: **template tag не
  должен попадать в extras**; для template сущностей — `TemplateServers`
  override и `preset.dns_rule`. Migration держит invariant через
  `templateDefaults` check.

## Final acceptance — все выполнено

- [x] `sing-box check -c config.json` PASSES после Rebuild с реальным state
- [x] Любая ошибка Rebuild показывает popup (наследие `5e56c0b`)
- [x] **Ноль** функций трансформации preset.outbounds → sing-box format
- [x] Все 24 пакета тестов зелёные (+ 27 новых SPEC 055 unit-тестов)
- [x] `ru VPN 🇷🇺` selector реально содержит RU-tagged nodes
- [x] mode=update патч `proxy-out` от `russian`/`ru-inside` реально фильтрует
- [x] Disable preset → effect полностью исчезает
  (`TestApply_OriginalParserCfgImmutable`)
- [x] User runtime: sing-box стартанул, конфиг применился, юзер подтвердил

## Open follow-ups (не блокирующие, в backlog)

- [ ] **UI cleanup**: убрать «Add DNS Server» / «Add DNS Rule» кнопки из
  DNS tab **для template-defined tag'ов** (можно нажать → копия body в
  state → нарушение invariant'а). Genuinely user-add — оставить.

- [ ] **Strict preset Body validator**: на load для `Rule.Kind=preset`
  парсить **только** `{vars}`; любые extra-поля в body silently drop с
  debug log. Forward-compat защита.

- [ ] **Migration validator**: на load detect template tag в extras и
  warning + предлагать конверсию в TemplateServers override (UI prompt).

## Out of scope (next SPECs, free ID = 056)

- [ ] SPEC 056 — explicit cross-preset dependencies (preset A зависит от preset B)
- [ ] SPEC 057 — `mode: "replace"` (destructive full-replace для outbounds)
- [ ] SPEC 058 — preset.inbounds (per-preset inbound configuration)
- [ ] SPEC 059 — Template authoring docs
