# SPEC 056 — Tasks

Все статусы — TODO. Реализация по фазам из `PLAN.md`. Параллельные
правки P1–P10 (см. `SPEC.md`) НЕ трогать.

## Phase 0 — Pre-cleanup docs

- [x] `SPECS/056-B-N-OUTBOUNDS_PARSER_RESTORE/SPEC.md` — добавлены разделы «Корневая причина», «Финальная архитектура», «Acceptance», «Параллельные правки»
- [x] `SPECS/056-B-N-OUTBOUNDS_PARSER_RESTORE/PLAN.md` — создан
- [x] `SPECS/056-B-N-OUTBOUNDS_PARSER_RESTORE/TASKS.md` — этот файл
- [x] `SPECS/055-F-N-PRESET_OUTBOUNDS/TASKS.md` — все статусы сброшены в TODO
- [x] `SPECS/055-F-N-PRESET_OUTBOUNDS/IMPLEMENTATION_REPORT.md` — удалён

## Phase 1 — Surgical revert хаоса 055 — [COMPLETED `098c5e1`]

### Удаление файлов (созданы 055)

- [x] `core/build/preset_outbounds_test.go` — удалён
- [x] `core/template/preset_outbounds_test.go` — удалён

### Откат до `f665c27`

- [x] `core/build/build.go` ← `f665c27` (cherry-pick P8 отложен — restored в Phase 5)
- [x] `core/build/dns_merge.go` ← `f665c27`, поверх применён `b03fd5b` (P7)
- [x] `core/build/preset_expand.go` ← `f665c27`
- [x] `core/build/preset_merge.go` ← `f665c27`
- [x] `core/build/rules_pipeline.go` ← `f665c27`
- [x] `core/template/preset_loader.go` ← `f665c27`
- [x] `core/template/preset_types.go` ← `f665c27`
- [x] `ui/configurator/business/outbound.go` ← `f665c27`
- [x] `bin/wizard_template.json` ← `f665c27`

### Частичный откат (mixed commits — рукой)

- [x] `core/rebuild.go` — никаких 055-кусков нет (только `15b217c` sing-box check + `5e56c0b` forced flag — оба сохранены)
- [x] `core/config_service.go` — снесён `AllNodeTags` + `collectAllNodeTagsFromCache`; сохранены SPEC 054 (P1) и BuildContext init
- [x] `ui/configurator/business/create_config.go` — снесён `AllNodeTags` + `collectAllNodeTagsFromCacheLocal`; сохранены `d36a257` (P4) preset-ref sync
- [x] `ui/configurator/tabs/rules_unified_rows.go` — снесён `refreshRulesTabFromPresenter` при toggle outbound preset; сохранены `dc4cf09` (P5) + `0ecc403` (P6) anti-loop
- [x] `docs/release_notes/upcoming.md` — снесён 055 entry; сохранён 054 entry

### Verify P1–P10 untouched (через `git diff f665c27..HEAD`)

- [x] `core/preview_nodes_test.go` — 186 lines (P1)
- [x] `ui/core_dashboard_tab.go` — 17 lines (P2)
- [x] `ui/configurator/presentation/presenter_methods.go` — 52 lines (P3)
- [x] `bin/locale/ru.json` — 15 lines (P4)
- [x] `internal/locale/en.json` — 15 lines (P9)
- [x] `internal/textnorm/proxy_display.go` — 35 lines (P10)
- [x] `ui/configurator/tabs/source_edit_*.go` — 396 lines (P9)

### Acceptance Phase 1

- [x] `go build ./...` зелёный
- [x] `go vet ./...` зелёный
- [x] `go test ./...` зелёный (24/24 packages)
- [ ] Manual sanity: запустить app, рестарт connect, убедиться что preset bundles работают как в `f665c27`

## Phase 2 — Types & loader

- [ ] `core/template/preset_types.go` — добавить `Preset.Outbounds []PresetOutbound`
- [ ] `core/template/preset_types.go` — добавить тип `PresetOutbound{Mode, Tag, Type, Options, Filters, AddOutbounds, PreferredDefault, Comment, Wizard, If, IfOr}`
- [ ] `core/template/preset_loader.go::validatePresetOutbounds`:
  - [ ] `mode ∈ {"", "add", "update"}` (empty → "add"; unknown → strip)
  - [ ] `tag` non-empty
  - [ ] `mode=add` → `type` required
  - [ ] `mode=update` → `type` warned (drop at Phase 3 expand)
  - [ ] tag uniqueness в пределах preset
  - [ ] `if`/`if_or` references на existing bool vars
- [ ] `core/template/preset_outbounds_test.go` — 7 unit tests

## Phase 3 — Pre-patch core

### `core/build/preset_outbounds.go` (NEW file)

- [ ] `ApplyPresetOutboundsToParserConfig(parserCfg, presets, refs, ruleOrder) (*ParserConfig, []string, error)`
- [ ] `ExpandPresetOutbounds(preset, vars) (entries, warnings)`
- [ ] `presetOutboundEntry{Mode, Config, PresetID}` internal type
- [ ] `applyOutboundUpdate(target, patch) OutboundConfig` (типизированный field-merge)
- [ ] `unionStringList(a, b []string) []string` helper
- [ ] `cloneOptions(m map[string]interface{}) map[string]interface{}` helper
- [ ] `substitutePresetVars(value interface{}, vars map[string]string) interface{}` (для @var)
- [ ] `deepCloneOutbounds(orig []OutboundConfig) []OutboundConfig`

### Tests `core/build/preset_outbounds_test.go`

- [ ] add-basic
- [ ] add-collision-globals (first wins)
- [ ] add-collision-preset (first wins by RuleOrder)
- [ ] add-identical (silent skip)
- [ ] add-disabled (no-op)
- [ ] update-basic (proxy-out filters patched)
- [ ] update-missing (warning, no-op)
- [ ] update-type-immutable (drop type + warning)
- [ ] update-multi (2 presets update same tag in RuleOrder)
- [ ] addOutbounds-union
- [ ] filters-replace
- [ ] options-per-field
- [ ] original-immutability
- [ ] empty-presets

## Phase 4 — Wire pre-patch

- [ ] `core/build/build.go::BuildContext` — добавить `ParserConfig *configtypes.ParserConfig`
- [ ] `core/build/build.go` — в `BuildConfig` использовать `ctx.ParserConfig` если задан, иначе fallback на template
- [ ] `core/config_service.go::buildContextFromState` — вызвать `ApplyPresetOutboundsToParserConfig` и положить в `ctx.ParserConfig`
- [ ] `ui/configurator/business/create_config.go::BuildPreviewConfig` — то же для preview path
- [ ] Тест интеграции: build с preset.outbounds → finalconfig.outbounds[] содержит patched tags

### Verify pipeline cleanliness

- [ ] `config.outbounds[]` не содержит полей `options/filters/addOutbounds/comment/wizard` (native эмит)
- [ ] `sing-box check -c config.json` PASSES при включённом `ru-inside` (manual)

## Phase 5 — Route post-pass cleanup

- [ ] `core/build/preset_outbounds.go::cleanDanglingOutboundRefInRule(rule, finalTags, fallback)`
- [ ] `core/build/preset_outbounds.go::CleanDanglingOutboundsInRouteRules(routeRaw, finalTags, fallback)`
- [ ] `core/build/build.go::buildSection` case "route" — добавить cleanup pass после `MergePresetsIntoRoute`
- [ ] Skip cleanup в preview (`ctx.ForPreview=true`) — наследуем `0c3dce5` (P8)
- [ ] Tests: dangling-fallback, dangling-drop, sentinel-preserved

## Phase 6 — UI integration

- [ ] `ui/configurator/business/outbound.go` — `GetAvailableOutbounds(model)` + `collectActivePresetOutboundTags(model)`
- [ ] `ui/configurator/tabs/rules_unified_rows.go` — refresh rules tab при toggle preset c outbounds (с anti-loop защитой из dc4cf09/0ecc403)
- [ ] Manual: enable preset с mode=add → outbound dropdown получает new tag

## Phase 7 — Template content migration

- [ ] `bin/wizard_template.json::parser_config.outbounds` — снять `filters: !RU` из `proxy-out` и `auto-proxy-out`
- [ ] `bin/wizard_template.json::parser_config.outbounds` — удалить global `ru VPN 🇷🇺` selector
- [ ] `bin/wizard_template.json::presets[ru-inside].outbounds` — добавить `mode=update` для proxy-out + auto-proxy-out + `mode=add` для `ru VPN 🇷🇺`
- [ ] `bin/wizard_template.json::presets[russian].outbounds` — добавить `mode=update` для proxy-out + auto-proxy-out
- [ ] `bin/wizard_template.json::presets[ru-blocked].outbounds` — то же
- [ ] `internal/constants/constants.go::RequiredTemplateRef` — bump
- [ ] Manual QA 1–5 из PLAN.md Phase 7

## Phase 8 — Golden fixtures + docs

- [ ] `core/build/testdata/golden/preset_outbounds_add.json`
- [ ] `core/build/testdata/golden/preset_outbounds_update.json`
- [ ] `core/build/testdata/golden/preset_outbounds_disabled.json`
- [ ] `core/build/testdata/golden/preset_outbounds_multi_update.json`
- [ ] CI hook: `sing-box check` на каждом fixture (если binary доступен)
- [ ] `docs/release_notes/upcoming.md` — SPEC 056 entry (EN + RU)
- [ ] `docs/ARCHITECTURE.md` — pre-patch parser_config (если есть SPEC 053 раздел)
- [ ] `SPECS/056-B-N-OUTBOUNDS_PARSER_RESTORE/IMPLEMENTATION_REPORT.md`

## Final acceptance (из SPEC 056)

- [ ] `sing-box check -c config.json` PASSES после `Rebuild` с реальным user state'ом
- [ ] Любая ошибка `Rebuild` показывает popup (наследие 5e56c0b + sing-box check)
- [ ] **Ноль** функций трансформирующих preset.outbounds в sing-box format
- [ ] Все 24 пакета тестов зелёные
- [ ] `ru VPN 🇷🇺` selector реально содержит RU-tagged subscription nodes
- [ ] mode=update на `proxy-out` от `russian`/`ru-inside` действительно фильтрует RU-ноды
- [ ] Disable preset → effect полностью исчезает (original parser_config не тронут)

## Out of scope (НЕ делать)

- [ ] SPEC 057 — preset cross-references
- [ ] SPEC 058 — preset.outbounds.mode="replace"
- [ ] SPEC 059 — preset.inbounds
- [ ] Template authoring docs (отдельная задача)
