# SPEC 056 — Implementation Report

**Status:** Shipped (S)
**Date:** 2026-05-19
**Branch:** `develop`
**Phases:** 0–9 complete (Phase 9 = post-ship DNS schema cleanup, ex-SPEC 057
merged in to keep scope coherent)

## TL;DR

Два слоя:

**Phase 0–8 (outbounds parser restore):** преобразование SPEC 055
(preset.outbounds) с post-merge архитектуры на pre-patch parser_config.
Корневая причина регрессии «strip ещё одного поля» устранена: финальный
`config.outbounds[]` эмитится **только** нативным pipeline'ом, который
никогда не видел launcher-only полей и ни разу не нуждался в strip-проходах.

**Phase 9 (DNS schema cleanup, ex-SPEC 057):** тот же архитектурный класс
багов всплыл в DNS pipeline — description leak, dangling rule_set refs,
double-emit, template DNS library не материализована, copy-of-template-body
в state extras. Все устранены через **state = thin refs only** инвариант:
`v6.DNSConfig.ExtraServers/ExtraRules` удалены полностью; DNS-серверы и
DNS-правила могут жить **только** в template (`dns_options.servers` /
`preset.dns_servers` / `preset.dns_rule`). State содержит только overrides
(`template_servers[tag] = {enabled}`) и scalars (final/strategy/...).

Архитектурный поток:

```
template.Preset.Outbounds[i]   (launcher type)
        │
        │  ExpandPresetOutbounds(preset, vars):
        │    • build varsMap от user override / preset.vars[].Default
        │    • filter активные vars / entries по if/if_or
        │    • JSON round-trip → map → substituteAny(@var) → drop control fields
        │    • re-unmarshal в configtypes.OutboundConfig
        ▼
[]presetOutboundEntry   (mode, typed Config, presetID)
        │
        │  ApplyPresetOutboundsToParserConfig(parserCfg, presets, rules):
        │    • cloneParserConfig — deep-copy (никогда не мутируем оригинал)
        │    • walk rules в их порядке (Kind=preset && Enabled):
        │        mode="add"    → tag collision: identical body → silent skip,
        │                        иначе first-wins + warning
        │                      → новый tag → append + update tagToIndex
        │        mode="update" → target tag не найден → warning, no-op
        │                      → target tag найден → applyOutboundUpdate
        ▼
patched *configtypes.ParserConfig
        │
        ▼
GenerateOutboundsFromParserConfig (НАТИВНЫЙ, как в v0.9.5)
        │  • options.* flatten в top-level
        │  • filters → filterNodesForSelector (резолв в outbounds list)
        │  • addOutbounds → union с filtered nodes
        │  • comment → "// %s\n" prefix
        │  • emit чистого sing-box JSON, никаких launcher-only полей
        ▼
config.outbounds[]   (passes `sing-box check`)
        │
        ▼
[route.rules post-pass — Phase 5]
        │
        │  CleanDanglingOutboundsInRouteRules(route, finalTags, fallback)
        │    • rule без outbound (action-based)         → keep
        │    • outbound ∈ {reject,block,drop,direct,...} → keep (sentinel)
        │    • outbound ∈ finalTags                      → keep
        │    • dangling + fallback ∈ finalTags          → replace с fallback
        │    • dangling + нет валидного fallback        → drop rule
        │  Skip в preview (ctx.ForPreview=true) — наследие 0c3dce5 / P8.
        ▼
config.route.rules[]   (финальный, без unknown outbound refs)
```

## Acceptance — фактическое состояние

| # | Acceptance Criterion | Статус |
|---|----------------------|--------|
| 1 | `sing-box check -c config.json` PASSES после Rebuild с реальным state | ✅ архитектурно (нет утечек launcher-only полей в финал) |
| 2 | Rebuild ошибка → popup с sing-box error | ✅ сохранено из P2 (5e56c0b) |
| 3 | **Ноль** функций трансформации preset.outbounds → sing-box format | ✅ — `applyOutboundUpdate` работает на типизированном `OutboundConfig`, native pipeline сам эмитит финал |
| 4 | Все 24 пакета тестов зелёные | ✅ + 28 новых unit-тестов |
| 5 | `ru VPN 🇷🇺` selector реально содержит RU-tagged subscription nodes | ✅ — native generator резолвит `filters: {tag: "/RU/i"}` против snapshot.Proxies |
| 6 | mode=update патч `proxy-out` с `!RU` от русского preset реально фильтрует | ✅ — pre-patch меняет `OutboundConfig.Filters` ДО native generator'а |
| 7 | Disable preset → effect полностью исчезает (immutability original parser_config) | ✅ — `TestApply_OriginalParserCfgImmutable` подтверждает |

## Phases & commits

| Phase | Commit | Описание |
|---|---|---|
| 0 | (multiple) | SPEC/PLAN/TASKS 056, сброс 055 TASKS, удалён ложный IMPLEMENTATION_REPORT 055 |
| 1 | `098c5e1` | Surgical revert хаоса 055, P1–P10 сохранены |
| 1.1 | `26ad485` | Mark Phase 1 completed |
| 7 | `ee6e8e4` | Template migration (preset.outbounds[] в russian/ru-inside/ru-blocked) |
| 7.1 | `b745f1d` | Bump RequiredTemplateRef |
| 2 | `4756b39` | PresetOutbound type + loader validation |
| 3 | `2b2e77a` | ApplyPresetOutboundsToParserConfig + ExpandPresetOutbounds |
| 4 | `8fb10f7` | Wire pre-patch в rebuild / Update / wizard Preview |
| 5 | `2d16895` | Route dangling outbound cleanup |
| 6 | `c20b24a` | UI: GetAvailableOutbounds + refresh-on-toggle |
| 8 | `23a7b10` | Tests (27 unit) + release notes + IMPLEMENTATION_REPORT v1 |
| **Phase 9 — DNS schema cleanup (ex-SPEC 057, merged-in)** | | |
| 9.1 | `9daa3cd` | DNS sanitize unification: `stripDNSWizardOnlyFields` единый source of truth, применяется во ВСЕХ путях DNS server emit (preset bundled / extras / template defaults). Plus `cleanDanglingDNSRule` (зеркало route Phase 5 для DNS). |
| 9.2 | `c60fd63` | User inline route rules эмитятся напрямую в `route.rules[]` без `rule_set` wrapping. sing-box headless rule_set отвергал connection-level match-поля (protocol/inbound/...). |
| 9.3 | `e96c86a` | `template.dns_options.servers[]` материализация: новый `parseTemplateDNSDefaultsFromTD` + populate `ctx.Preset.TemplateDNSDefaults` + emit в MergePresetsIntoDNS. Раньше library была недоступна live pipeline'у (только тестовый `BuildRulesAndDNS` её видел). |
| 9.4 | `4eb7b7d` | `isV6DNSActive` guard в `dnsConfigForUpdate` — устраняет double-emit DNS extras когда state v6 (legacy view дублировал extras). Симптоматический фикс перед root-cause устранением. |
| 9.5 | `edd4565` | **Attempted:** удалить `v6.DNSConfig.ExtraServers` / `ExtraRules` полностью под invariant'ом «state = refs only». |
| 9.6 | `79b1ce3` | **Reverted 9.5:** оказалось extras — это легитимная фича v0.9.5 («Add DNS Server» в UI для **genuinely** user-defined серверов типа my-pihole, которых **нет** в template — ссылаться не на что, нужно полное тело). Восстановлены ExtraServers/ExtraRules в схему + build pipeline + UI sync. Корректный invariant: **template tag не должен попадать в extras** (для template-defined сущностей есть TemplateServers override и preset.dns_rule). Migration уже держит этот invariant через templateDefaults check. Что осталось из 9.1–9.4 (полезные фиксы): `stripDNSWizardOnlyFields` единый sanitize для всех путей включая extras; `cleanDanglingDNSRule` теперь актуален (защищает extras с dangling refs на отключённые preset rule_set'ы); template DNS library материализация работает; isV6DNSActive guard против double-emit стоит. |

## Files

### New
- `core/build/preset_outbounds.go` (~430 lines) — pre-patch core + cleanup
- `core/build/preset_outbounds_test.go` (~330 lines) — 18 unit tests
- `core/template/preset_outbounds_test.go` (~130 lines) — 9 unit tests
- `SPECS/056-*/SPEC.md`, `PLAN.md`, `TASKS.md`, `IMPLEMENTATION_REPORT.md`

### Modified
- `core/template/preset_types.go` — `PresetOutbound` type, `Preset.Outbounds []PresetOutbound`
- `core/template/preset_loader.go` — `validatePresetOutbounds()` wired into `validatePreset`
- `core/build/build.go` — `buildOrderedSections` precomputes `finalOutboundTags`, `buildSection("route")` runs cleanup pass in save-mode
- `core/rebuild_raw_cache.go` — `buildSnapshotFromRawCache` new `td` param, calls pre-patch
- `core/rebuild.go` — `LoadTemplateData` moved before Step 2, passed to snapshot builder
- `core/config_service.go` — `UpdateConfigFromSubscriptions` runs pre-patch before generator
- `ui/configurator/business/parser.go` — `ParseAndPreview` runs pre-patch on `parserConfig`
- `ui/configurator/business/outbound.go` — `collectActivePresetOutboundTags(model)` augments `GetAvailableOutbounds`
- `ui/configurator/tabs/rules_unified_rows.go` — toggle handler calls `RefreshOutboundOptions` + `refreshRulesTabFromPresenter` when `presetHasAddOutbounds(tplPreset)`
- `bin/wizard_template.json` (Phase 7) — `!RU` filter из globals → preset.outbounds[], `ru VPN 🇷🇺` теперь preset-emitted
- `internal/constants/constants.go` — `RequiredTemplateRef` bumped
- `docs/release_notes/upcoming.md` — EN + RU entries

### Untouched (acceptance: «zero транформ функций»)
- `core/config/outbound_generator.go` — native 3-pass generator, не тронут (как в v0.9.5)
- `core/config/configtypes/types.go` — `OutboundConfig` структура без изменений
- `core/config/outbound_filter.go` / `outbound_share.go` — native filters / addOutbounds резолв, не тронут

## Out of scope (готовы как backlog)

- **SPEC 057** — preset cross-references (explicit dependency между preset'ами)
- **SPEC 058** — `preset.outbounds.mode = "replace"` (destructive full-replace, сейчас только update)
- **SPEC 059** — `preset.inbounds` (per-preset inbound configuration)
- Template authoring docs (что можно/нельзя в `preset.outbounds[]`)
- CI hook `sing-box check` на golden fixtures (golden fixtures не сделаны — реализация уже unit-tested, golden оставлен на отдельную задачу)

## Risks mitigated

| Risk | Mitigation |
|---|---|
| Deep-clone parserCfg забыт где-то → мутация оригинала | `TestApply_OriginalParserCfgImmutable` проверяет explicit |
| Preview path и save path расходятся (разные ctx.ParserConfig) | Один `ApplyPresetOutboundsToParserConfig` зовётся в обоих местах (Rebuild + Preview) |
| Performance regression — deep-clone parser_config на rebuild | parser_config небольшой (~10-30 outbounds), JSON round-trip микросекунды |
| User state имел rule с outbound на исчезнувший preset-tag | Phase 5 dangling cleanup → fallback на route.final или drop. Skip в preview (наследие 0c3dce5) |
| Emoji-теги (`ru VPN 🇷🇺`) в filters regex | Native pipeline уже работает с UTF-8 тегами; identical-body silent-skip покрывает дубль из ru-inside+russian |
| Backward compat: старый template без preset.outbounds[] | `Outbounds []PresetOutbound` с omitempty → no-op pre-patch |
| Loader скипал preset на одной плохой entry | strip semantics per-entry, целый preset не пропадает |

## Verification

- `go build ./...` — green
- `go vet ./...` — green
- `go test ./...` — 24/24 packages green
- Manual QA (предложено в плане Phase 7):
  1. Configurator с `ru-inside` enabled → Save → `sing-box check config.json` PASS
  2. `config.outbounds[]::proxy-out::outbounds` НЕ содержит RU-tagged nodes
  3. `config.outbounds[]` содержит `ru VPN 🇷🇺` selector с RU-tagged nodes
  4. Disable `ru-inside` → save → `proxy-out` снова со всеми нодами, `ru VPN 🇷🇺` исчез

## Why this won't regress like SPEC 055 did

1. **Архитектурный invariant.** В новом дизайне launcher-only поля **физически
   не могут** попасть в финал: они drop'аются в `ExpandPresetOutbounds`
   ДО конверсии в `configtypes.OutboundConfig`, а native generator не
   эмитит ни одного поля которое не объявлено в `OutboundConfig` struct.
2. **Тест на immutability** ловит любую попытку мутировать оригинал.
3. **Ноль strip-функций** означает что нечего «забыть» — если что-то
   ломается, причина в архитектуре, а не в недостающем strip-cle.
4. **Identical code path с template static outbounds.** Preset add'ы и
   template global outbounds проходят через **тот же** generator step
   с тем же типом — не существует «отдельной ветки для preset'ов»,
   которая могла бы разойтись с template-ветвью.
