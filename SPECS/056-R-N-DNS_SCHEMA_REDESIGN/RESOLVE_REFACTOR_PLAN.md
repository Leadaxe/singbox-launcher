# SPEC 056-R-N follow-up — Unified DNS/Route resolver

**Status:** ✅ **DONE** (shipped, uncommitted). См. `IMPLEMENTATION_REPORT.md` Phase B.

> Этот документ — оригинальный plan для unified resolver рефактора. Реальная
> реализация совпала с планом (DNS + Route resolvers через pure-func, UI parity,
> build cleanup). См. финальные note'ы в IMPLEMENTATION_REPORT и список deltas
> в TASKS.md.

---

**Status (original):** in-progress (uncommitted)
**Trigger:** обнаружен mismatch UI ↔ state ↔ emit для preset DNS-серверов
(render показывал 1 yandex_*, state содержит 3, config эмитит 1 после
consumption-фильтра в ExpandPreset). После SPEC 056-R-N инвариант
**state == memory == emit** не держится — половина if/if_or логики живёт
в build path и не доступна UI.

## Цель

Один **resolver** (pure func) — единая точка истины для «что есть в DNS
после применения template + state + vars». UI и build pipeline оба
consume один результат.

```
state + template + vars
        │
        ▼
   ResolveDNS()  ◄── pure, единственная функция-резолвер
        │
        ▼
   ResolvedDNS{ Servers, Rules, Scalars }
        │
   ┌────┴────┐
   ▼         ▼
[UI render] [build emit]
показ ВСЕХ  Active && Enabled → config.dns
с badges
```

Аналогично `ResolveRoute()` для route section.

## Принципы

1. **Memory == disk == emit.** То что в state — то и в config, что в UI —
   то и в config. Никаких скрытых фильтров.
2. **Consumption-filter удалён.** Все bundled DNS-серверы preset'а попадают
   в config (а не только @dns_server-picked). Юзер управляет per-server
   чекбоксом. Dangling `dns_rule.server` ловится отдельным валидатором.
3. **`evalIf`/`substitute` в одном месте.** UI и build используют одни и
   те же primitives через resolver.
4. **Inactive — first-class UI state.** Если server `Active=false` (не прошёл
   if/if_or), checkbox disabled, tooltip объясняет причину.

## Структуры (Phase 1)

```go
type DNSSource string
const (
    DNSSourceCore     DNSSource = "core"     // template.config.dns.servers (минимум)
    DNSSourceTemplate DNSSource = "template" // template.dns_options.servers (library)
    DNSSourcePreset   DNSSource = "preset"   // template.presets[X].dns_servers
    DNSSourceUser     DNSSource = "user"     // state.DNS.Servers[kind=user]
)

type ResolvedDNSServer struct {
    Tag         string                 // финальный (с preset prefix если применимо)
    LocalTag    string                 // для UI без preset prefix
    Body        map[string]interface{} // готовое тело после substitute (sing-box-valid)
    Source      DNSSource
    PresetID    string                 // только для Source=preset
    PresetLabel string

    Active         bool   // прошёл if/if_or
    Enabled        bool   // юзерский toggle (state)
    Locked         bool   // core minimum — нельзя toggle
    InactiveReason string // "if=use_dns_override" для UI tooltip
}

type ResolvedDNSRule struct {
    Body        map[string]interface{}
    Source      DNSSource              // preset/user (template/core нет для rules)
    PresetID    string
    PresetLabel string

    Active         bool
    Enabled        bool
    InactiveReason string
}

type ResolvedDNS struct {
    Servers []ResolvedDNSServer
    Rules   []ResolvedDNSRule
    // Scalars: Strategy, Final, IndependentCache, DefaultDomainResolver
    // (из state.Vars[dns_*] с fallback на template default)
}
```

## Алгоритм `ResolveDNS()`

```
INPUT: state, template, varsMap (template vars + dns_* scalars)

1. CORE: template.config.dns.servers[]
   → Source=core, Locked=true, Active=true, Enabled=true

2. TEMPLATE LIBRARY: template.dns_options.servers[]
   → Source=template, Active=true (нет if на этом уровне),
     Enabled = state.DNS.Servers[kind=template, tag].Enabled
              ?? template default_enabled

3. PRESETS: для каждого state.Rules[kind=preset, enabled=true]:
   - presetVars = build из preset.Vars + PresetBody.Vars
   - для каждого preset.DNSServers[i]:
     → active, reason = evalIfWithReason(ds.If, ds.IfOr, presetVars)
     → body = substitute(ds_struct → map, presetVars)
     → tag = preset.ID + ":" + ds.Tag
     → enabled = state.DNS.Servers[kind=preset, ref=tag].Enabled
                ?? true
     → Source=preset, PresetID, PresetLabel

4. USER: state.DNS.Servers[kind=user]
   → Source=user, Active=true, Enabled=srv.Enabled, Body=srv.Body

5. То же для Rules (template/core нет, только preset/user)

OUTPUT: ResolvedDNS
```

## Дюngling-rule warning

Если `dns_rule.server="russian:yandex_doh"` ссылается на server который
`Enabled=false` (или `Active=false`) — build emit'ит warning и **skip'ает
эту rule** (или весь rule_set). Защита от FATAL у sing-box.

## Этапы (Phase 1-8)

| # | Phase | Файлы |
|---|---|---|
| 1 | Types only (struct + enum, no impl) | `core/build/resolve_dns.go` |
| 2 | `ResolveDNS()` impl + unit tests | `core/build/resolve_dns.go` + `_test.go` |
| 3 | Build switch DNS: `MergePresetsIntoDNS` → `ResolveDNS` + filter. Удалить consumption-фильтр из `ExpandPreset`. Dangling warning. | `core/build/preset_merge.go`, `core/build/preset_expand.go` |
| 4 | `ResolveRoute()` + route build switch | `core/build/resolve_route.go` (new), `core/build/preset_merge.go` |
| 5 | UI DNS render: `dns_tab.go` + `dns_preset_bundled.go` → один проход по `ResolveDNS()`. Disabled checkbox + tooltip для !Active. | `ui/configurator/tabs/dns_tab.go`, `dns_preset_bundled.go` |
| 6 | Drop `model.DNSPresetServerEnabled` + `DNSPresetRuleEnabled`. UI toggle пишет прямо в state. | `wizard_model.go`, `preset_ref_helpers.go`, `presenter_state.go` |
| 7 | Rename `state.State.DNSV6` → `state.State.DNS` (~19 callsites) | глобально |
| 8 | Build + tests + reinstall, leave uncommitted | — |

## Что выкидываем

- `consumedBundled` + `collectConsumedBundledDNSTags` в `ExpandPreset`
- DNS section в `ExpandPreset.frags.DNSServers/DNSRule` (логика переезжает в `ResolveDNS`)
- `resolvePresetDNSServer` / `resolvePresetDNSRule` (был костыль вокруг ExpandPreset)
- `model.DNSPresetServerEnabled` / `DNSPresetRuleEnabled`
- `applyPresetEnabledOverrides` / `populatePresetEnabledFromState` helpers
- `renderPresetBundledDNSRows` (заменяется единым render'ом)

## Что НЕ трогаем

- Schema version v6 / `presets_v1` — не меняется
- v5 read path (`parseV5`, `s.DNSOptions` legacy указатель)
- `parseV6::legacyDevDNSToOptions` fallback (read-старого-shape для миграции)
- `SyncDNSOptionsWithActivePresets` — остаётся как точка lifecycle entries
  (sync создаёт/удаляет kind=preset records; resolver работает поверх)
