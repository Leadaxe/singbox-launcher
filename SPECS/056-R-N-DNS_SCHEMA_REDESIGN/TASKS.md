# SPEC 056-R-N — Tasks (FINAL)

**Status:** shipped, uncommitted (on user review).
**Дата завершения:** 2026-05-23.

---

## Phase A — DNS Schema Redesign (original SPEC) ✅

| # | Phase | Файлы |
|---|---|---|
| A1 | Schema types `v6.DNSOptions` + `DNSServer{Kind,Tag,Ref,Enabled,Body}` + `DNSRule{Kind,Ref,Enabled,Body}` + custom flat-JSON Marshal/Unmarshal | `core/state/v6/dns_options.go` (new) |
| A2 | `SyncDNSOptionsWithActivePresets` — единая lifecycle entries kind=preset (на load + toggle, idempotent) | `core/state/v6/sync_dns.go` (new) |
| A3 | In-place dev rewrite: `parseV6` fallback на старый `dns` shape через `legacyDevDNSToOptions` | `core/state/load.go` |
| A4 | Build DNS pipeline: `MergePresetsIntoDNS` → один walk со switch kind | `core/build/preset_merge.go`, `core/build/rules_pipeline.go` |
| A5 | UI sync `SyncDNSFullToStateV6` под flat shape + UI rules tab toggle wire | `ui/configurator/models/preset_ref_sync.go`, `presentation/presenter_state.go` |
| A6 | Per-preset enabled state живёт в `PresetRefState.DNSServerEnabled` / `DNSRuleEnabled` (не в model maps) | `ui/configurator/models/preset_ref_state.go` |
| A7 | Rename `state.State.DNSV6 → state.State.DNS` (53 callsite) | global |
| A8 | Удалены `legacyDNSOptionsFromV6`, `emitTemplateDNSDefaults`, `SyncDNSToStateV6`, `mergeLockedRow`, dead `GetWizardRequired`, dead automatic backup (`maybeBackupDevV6` / `looksLikeLegacyDevV6`) | разное |
| A9 | Tests: `core/state/v6/sync_dns_test.go` + обновлены existing tests; build/UI tests refactored | разное |

## Phase B — Unified Resolver (DNS + Route) ✅

| # | Phase | Файлы |
|---|---|---|
| B1 | Types: `ResolvedDNSServer/Rule/DNS` + `DNSSource` enum | `core/build/resolve_dns.go` (new) |
| B2 | `ResolveDNS()` impl + 7 unit tests | `core/build/resolve_dns.go`, `_test.go` |
| B3 | Build switch DNS: `MergePresetsIntoDNS` → `ResolveDNS` + filter `Active && Enabled` | `core/build/preset_merge.go` |
| B4 | Consumption-filter удалён из `ExpandPreset` (DNS section) — все bundled серверы попадают в frags | `core/build/preset_expand.go` |
| B5 | `ResolveRoute()` (preset/inline/srs), `MergePresetsIntoRoute` thin wrapper | `core/build/resolve_route.go` (new) |
| B6 | UI DNS render через `ResolveDNS`: HoverRow + WireTooltip + disabled-checkbox + InactiveReason tooltip для preset entries | `ui/configurator/tabs/dns_preset_bundled.go` |
| B7 | `PresetOutboundAddTags` экспорт-helper в build (для UI Outbounds tab) | `core/build/preset_outbounds.go` |

## Phase C — Template DNS Unify (single source) ✅

| # | Phase | Файлы |
|---|---|---|
| C1 | `local_dns_resolver` + `direct_dns_resolver` переехали из `template.config.dns.servers[]` в `template.dns_options.servers[]` с `required: true`. `config.dns.servers: []` оставлен пустой для backward-compat parser. | `bin/wizard_template.json` |
| C2 | `TemplateDNSServer` поле `DefaultEnabled` → `Enabled` + добавлено `Required bool` | `core/build/rules_pipeline.go` |
| C3 | `ParseTemplateDNSDefaults` читает `required` + force `enabled:true` при `required:true && enabled:false` (coherence fix) | `core/build/rules_pipeline.go` |
| C4 | `ValidateTemplateDNSServers` (новая) — tag-uniqueness + required-enabled coherence warnings, вызывается из `parseTemplateDNSDefaultsFromTD` | `core/build/rules_pipeline.go`, `core/config_service.go` |
| C5 | `ResolveDNS` CORE step удалён, `coreDNSServersFromTemplate` helper удалён. Template library walk учитывает `Required` → `Locked=true` | `core/build/resolve_dns.go` |
| C6 | `bodyRequired` helper + dedup `DNSSourceCore` → alias на `DNSSourceTemplate` (затем DNSSourceCore удалён) | `core/build/resolve_dns.go` |
| C7 | `model.DNSLockedTags` удалён, `wizardbusiness.DNSTagLocked` читает `required` из template live | `ui/configurator/models/wizard_model.go`, `ui/configurator/business/wizard_dns.go` |
| C8 | `reconcileDNSServers` упрощён (один walk по dns_options + orphan saved tags); `mergeLockedRow` удалён | `ui/configurator/business/wizard_dns.go` |
| C9 | `stripDNSWizardOnlyFields` добавлен `required` в strip-list (не уезжает в финальный sing-box config) | `core/build/dns_merge.go` |

## Phase D — UI cleanup (DNS tab visual parity) ✅

| # | Phase | Файлы |
|---|---|---|
| D1 | DNS Servers + Rules секция — единый widget pattern (`NewCheckWithContent` + `HoverRow` + `WireTooltipLabelHover`) для template/preset/user entries | `ui/configurator/tabs/dns_preset_bundled.go` |
| D2 | Удалена секция-заголовок "from active presets" — bundled rows в общем списке (🔒 в label показывает source) | `ui/configurator/tabs/dns_tab.go` |
| D3 | Template entries: View JSON кнопка (read-only inspect body). Locked → checkbox greyed. | `dns_tab.go`, `dns_preset_bundled.go` |
| D4 | Preset entries: format `<tag> · 🔒 <preset_label>`, чекбокс toggle, View JSON, no edit/del | `dns_preset_bundled.go` |
| D5 | Render walks `template.dns_options.servers[]` напрямую (не filtered через consumption), все bundled servers видны | `dns_preset_bundled.go::renderPresetBundledDNSRows` |

## Phase E — Outbounds Tab Unify ✅

| # | Phase | Файлы |
|---|---|---|
| E1 | Disabled subscription cascade — `RebuildPreviewCache`, `collectRows`, `collectAllTags` пропускают `proxy.Disabled` (UI ↔ build pipeline parity) | `ui/configurator/business/preview_cache.go`, `ui/configurator/outbounds_configurator/configurator.go` |
| E2 | Preset-bundled outbounds показываются в общем Outbounds tab list через `collectPresetOutboundRows` + `build.PresetOutboundAddTags` | `outbounds_configurator/configurator.go` |
| E3 | Dedup preset rows против `existingTags` (global + per-source) — first-wins, паритет с `ApplyPresetOutboundsToParserConfig` | `outbounds_configurator/configurator.go` |
| E4 | `OutboundConfig.Required` field удалён (изначально добавлен → потом убран, чтобы state.json не персистил template-only flag) | `core/config/configtypes/types.go` |
| E5 | `templateRequiredTags` — live lookup в template raw JSON через map (template = единый источник истины для required) | `outbounds_configurator/configurator.go` |
| E6 | UI required outbound: Up/Down работают, Edit работает, **Reset 🔄 кнопка** (откат body к template), Del кнопка НЕ рендерится (visual cleanup) | `outbounds_configurator/configurator.go` |
| E7 | Preset outbound row: read-only, View JSON для inspect | `outbounds_configurator/configurator.go` |
| E8 | **Preset reorder через promote-to-global**: клик ↑ на preset row копирует body в `pc.ParserConfig.Outbounds[]`, после чего ведёт себя как обычный global. Preset row дедуплицируется (collision skip). Lifecycle: удалить promoted global → preset row возвращается автоматически. | `outbounds_configurator/configurator.go::collectPresetOutboundRows + render` |
| E9 | View JSON dialog для preset outbound: pretty JSON + helper text "Click ↑ to promote to global" | `outbounds_configurator/configurator.go` |
| E10 | Reset кнопка получила label "Reset" — визуальная ширина = Edit/Del для row alignment | `outbounds_configurator/configurator.go` |
| E11 | **Bug fix:** `templateRequiredTags` парсил `model.TemplateData.ParserConfig` с json-tag `"parser_config"` (lowercase), но loader оборачивает в `{"ParserConfig": {...}}` (capital P) — `required: true` молча терялся. Tag исправлен на `"ParserConfig"`. | `outbounds_configurator/configurator.go` |
| E12 | **Bug fix:** изначально `IsRequired = ob.Required OR templateRequiredTags[tag]` — позволяло state.json держать stale Required если template убрал флаг. Заменено на `IsRequired = templateRequiredTags[tag] only` (template = единый источник). | `outbounds_configurator/configurator.go` |
| E13 | **Preset reorder с сохранением preset binding (UI-side).** Реализован через `model.OutboundDisplayOrder []string` (in-memory, `json:"-"`) + helpers `rowKey` / `reorderByDisplayOrder` / `reorderRowDisplay`. Unified list = `globals + presetRows`, отсортированный по OutboundDisplayOrder; Up/Down свопает позиции в этом списке. Special case: global↔global swap идёт через `pc.ParserConfig.Outbounds[]` мутацию (не нужен entry в Order). Preset row сохраняет `IsPreset=true` после reorder — promote-to-global больше не нужен. | `outbounds_configurator/configurator.go`, `wizard_model.go` |

## Phase F — Cleanup ✅

| # | Phase | Файлы |
|---|---|---|
| F1 | `WizardConfig.Required int` + `GetWizardRequired()` method удалены — dead code, нигде не вызывался | `core/config/configtypes/types.go` |
| F2 | Automatic backup `maybeBackupDevV6` / `looksLikeLegacyDevV6` удалены (YAGNI — dev-only lossless conversion, v6 не релизился) | `core/state/load.go` |
| F3 | `model.DNSPresetServerEnabled` / `DNSPresetRuleEnabled` карты удалены, scope перенесён в `PresetRefState` (per-instance lifecycle) | `wizard_model.go`, `preset_ref_state.go`, helpers |
| F4 | `model.DNSLockedTags` удалён (Phase C7 follow-up — был cache из старого config.dns.servers, заменён live lookup из template) | `wizard_model.go` |
| F5 | `model.OutboundDisplayOrder []string` (in-memory, `json:"-"`) — wired в UI render через `reorderByDisplayOrder` (см. E13). В build pipeline (`ApplyPresetOutboundsToParserConfig`) **НЕ** прокинут — config.json по-прежнему ставит preset entries в конец. Persist в state.json тоже не делается — reorder теряется на рестарт app. | `wizard_model.go` |

## Backlog / Not Done

### Перенесено в SPEC 057-R-N-OUTBOUNDS_PRESET_BINDING

Архитектурный fix preset reorder и preset update-stack живут в отдельном SPEC.
SPEC 057 заменяет `OutboundDisplayOrder` (UI-only) на **прямой preset binding
в state**: `OutboundConfig.Ref string` + `OutboundConfig.Updates []OutboundUpdate`
для стека patches от `mode=update` пресетов. См. `SPECS/057-R-N-OUTBOUNDS_PRESET_BINDING/SPEC.md`.

- ~~Preset reorder — persist в state.json~~ → SPEC 057
- ~~Preset reorder — учёт в build pipeline~~ → SPEC 057
- ~~Preset DNS reorder~~ → отдельная задача (паттерн SPEC 057 + ResolveDNS адаптировать)

### Остаётся в backlog SPEC 056

- **Golden tests** — `core/build/golden_test.go` запускается только под `GOLDEN_RUN_REAL=1`. Реальной CI-проверки байт-в-байт паритета финального config.json до/после рефактора не было. Тестировалось ручным запуском sing-box.
- **UX-валидация Reset кнопки** на required outbound при edge cases (template без tag'а после bump'а, broken template и т.п.). Базовый flow работает; защитные ветки в коде есть (silent no-op).
- **Strict preset Body validator** при load state — отбрасывать non-Vars поля если template author случайно скопировал «жирный» body в preset Body. Сейчас просто игнорируется; warning не выводится.
- **Helper для skip-disabled** при walk `pc.ParserConfig.Proxies[]` — сейчас 3 разных места независимо проверяют `proxy.Disabled` (рефакторнуть в общий iterator или хотя бы lint-check).

## Verify

- `go build ./...` — passes
- `go test ./... -count=1` — все 24 пакета зелёные
- Бинарь установлен в `/Applications/singbox-launcher.app/Contents/MacOS/singbox-launcher`
- Template установлен в `/Applications/singbox-launcher.app/Contents/MacOS/bin/wizard_template.json`
- **Не закоммичено** — ждёт review.

## Diff statistics

```
~34 файла, +2346 −1287 = net +1059 LOC
  - Большие удаления: dead code, legacy hacks, дубли
  - Новый код: 2 resolver'а (DNS + Route), Required mechanism, unified UI patterns
```
