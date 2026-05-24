# SPEC 056-R-N — Implementation Plan

> **Status:** ✅ **DONE**. См. `IMPLEMENTATION_REPORT.md` для финального
> ground truth. Этот документ — оригинальный пошаговый план; реализация
> в основном совпала, но scope expand'ился (Phase B-F добавились) — детали
> в REPORT.

---



Цель — переписать дев-схему `state.dns` на flat `state.dns_options.{servers,rules}[]`
с `kind` discriminator (template/preset/user). Schema version и name **не меняются**
(v6 / `presets_v1` остаются дев-маркером).

---

## Phase 1 — Schema (foundation)

### Новые типы

Файл `core/state/v6/dns_options.go` (новый):

```go
type DNSServerKind string
const (
    DNSServerKindTemplate DNSServerKind = "template"
    DNSServerKindPreset   DNSServerKind = "preset"
    DNSServerKindUser     DNSServerKind = "user"
)

type DNSRuleKind string
const (
    DNSRuleKindPreset DNSRuleKind = "preset"
    DNSRuleKindUser   DNSRuleKind = "user"
)

type DNSServer struct {
    Kind    DNSServerKind
    Tag     string                 // kind=template|user
    Ref     string                 // kind=preset (формат "<preset_id>:<local_tag>")
    Enabled bool
    Body    map[string]interface{} // только для kind=user (type/server/...)
}

type DNSRule struct {
    Kind    DNSRuleKind
    Ref     string                 // kind=preset (формат "<preset_id>")
    Enabled bool
    Body    map[string]interface{} // только для kind=user (rule_set/server/...)
}

type DNSOptions struct {
    Strategy              string                       // dns_* scalars остаются в state.vars[], НЕ здесь
    Final                 string                       //  (не трогаем по решению SPEC)
    IndependentCache      bool
    DefaultDomainResolver string

    Servers []DNSServer
    Rules   []DNSRule
}
```

Кастомные MarshalJSON/UnmarshalJSON для `DNSServer` и `DNSRule`:
плоская сериализация (`{kind, ref|tag, enabled, ...body}`).

### Что удаляется

- `v6.DNSConfig` (с полями `TemplateServers/ExtraServers/ExtraRules`) — удаляется полностью.
- `v6.TemplateServerOvr` — удаляется.

### Изменения state.State

- `state.State.DNSV6` — тип меняется с `v6.DNSConfig` на `v6.DNSOptions`.
  Имя поля сохраняем (минимизируем churn по 19 callsite'ам); меняется лишь
  JSON-тег при сериализации.

### Изменения save/load

- `state.marshalDiskV6` — JSON-ключ `"dns"` → `"dns_options"`. Сериализация v6.DNSOptions.
- `state.parseV6` — читает `"dns_options"` (новый shape). С fallback на `"dns"` (старый
  dev-shape) — см. Phase 3.

### LOC: ~+180, −20.

---

## Phase 2 — SyncDNSOptionsWithActivePresets

Файл `core/state/v6/sync_dns.go` (новый).

```go
// SyncDNSOptionsWithActivePresets — единая точка lifecycle entries kind=preset.
//
// Вызывается:
//   - на parseV6 после load (idempotent)
//   - на каждый toggle preset enable/disable в Rules tab
//   - перед marshalDiskV6 (defensive)
//
// Семантика:
//   - Для каждого state.Rules[Kind=preset && Enabled=true]:
//     ensure entries {Kind:preset, Ref:<id>:<local_tag>} в state.DNSV6.Servers
//     по числу template.presets[id].dns_servers[].
//     Default Enabled=true. Если entry уже была — preserve её Enabled.
//   - Для preset с dns_rule: ensure entry {Kind:preset, Ref:<id>} в state.DNSV6.Rules.
//   - Drop entries с Ref на disabled/missing preset.
func SyncDNSOptionsWithActivePresets(
    rules []Rule,
    dns *DNSOptions,
    presetByID map[string]*template.Preset,
)
```

LOC: ~80.

---

## Phase 3 — In-place dev rewrite

Файл `core/state/load.go`.

### parseV6

1. Попробовать декод нового shape (`dns_options`).
2. Если `dns_options` отсутствует, попробовать старый (`dns` с template_servers/extra_*).
3. Если попался старый shape — конвертировать в `v6.DNSOptions` (см. ниже).
   На ближайшем Save файл перезапишется в новом layout'е (normal save flow).
4. Вызвать `SyncDNSOptionsWithActivePresets` (idempotent — добавит preset entries
   если их нет).

**Backup НЕ делаем** — YAGNI: v6 не релизился (только мой dev-state), конверсия
lossless (round-trip тест покрывает). Кто хочет страховку — `cp state.json
state.json.bak` руками перед апгрейдом.

### Migration функция

`core/state/v6/migration.go` — обновить или удалить `migrateDNS`:
старый `v5 → v6 DNSConfig` уже не нужен (v6 был дев-only). Но **v5 → v6.DNSOptions**
нужен: при первом Save после деплоя SPEC 056 поверх v5 (юзер добавил preset-ref)
state переходит в v6, и DNS должны попасть в новый shape.

Старый `migrateDNS(*v5.DNSOptions, templateDefaults)` → новый
`migrateDNSToOptions(*v5.DNSOptions, templateDefaults) v6.DNSOptions`:
- servers[] split:
  - tag ∈ templateDefaults → `{Kind:template, Tag, Enabled}`
  - tag ∉ templateDefaults → `{Kind:user, Tag, Enabled, Body: full body минус enabled}`
- rules[] → `{Kind:user, Body}` каждое.

### legacyDNSOptionsFromV6 — УДАЛИТЬ

В parseV6 больше **не материализуем** legacy view. Заменяем на:
- UI рендерит DNS tab прямо из `s.DNSV6.Servers/Rules` через kind switch.
- Build pipeline читает `s.DNSV6` напрямую (через `ctx.Preset.DNS`).

Callsite'ы которые ещё на `s.DNSOptions` (legacy):
- `dnsConfigForUpdate` — переписать на чтение из `s.DNSV6` (см. Phase 4).

LOC: −80 (delete legacyDNSOptionsFromV6) + ~50 (migrateDNSToOptions + parseV6 logic).

---

## Phase 4 — Build pipeline

Файлы `core/build/preset_merge.go`, `core/build/rules_pipeline.go`, `core/config_service.go`.

### Один walk по dns_options.servers[]

`MergePresetsIntoDNS` переписывается. Старая логика трёх отдельных проходов
(template overrides filter / bundled append из ExpandPreset / extras) сливается в
**один цикл** по `ctx.DNS.Servers`:

```go
for _, srv := range ctx.DNS.Servers {
    if !srv.Enabled { continue }
    switch srv.Kind {
    case DNSServerKindTemplate:
        body := lookupTemplateDNSServer(templateDNSDefaults, srv.Tag)
        if body == nil { warn; continue }
        servers = append(servers, stripDNSWizardOnlyFields(body))
    case DNSServerKindPreset:
        body := lookupPresetDNSServer(presetByID, srv.Ref)
        if body == nil { warn (dangling — sync должен был почистить); continue }
        // Apply preset Vars substitute. Strip. Prefix tag.
        servers = append(servers, stripDNSWizardOnlyFields(resolved))
    case DNSServerKindUser:
        m := bodyToMap(srv)
        servers = append(servers, stripDNSWizardOnlyFields(m))
    }
}
```

То же для `ctx.DNS.Rules` (только preset/user kind).

### Удаления

- `legacyDNSOptionsFromV6` — уже удалена в Phase 3.
- inline v5/v6 guard в `dnsConfigForUpdate` — упрощается: всегда `s.DNSV6` для v6
  схемы, `s.DNSOptions` только для pure-v5 (когда `len(s.RulesV6)==0` и
  `len(s.DNSV6.Servers)==0`).
- runtime materialize bundled DNS в `MergePresetsIntoDNS` (старый цикл по
  `ctx.RulesV6 → ExpandPreset → frags.DNSServers`) — теперь это происходит
  через `kind=preset` lookup в новом walk'е, а entries уже сами в state.

### Что остаётся

- `cleanDanglingDNSRule` — да, оставить (защита от race условий между sync и build).
- `stripDNSWizardOnlyFields` — single source of truth, остаётся как было.
- `ExpandPreset` — `frags.DNSServers` / `frags.DNSRule` всё ещё нужны для
  ExpandPreset тестов и для извлечения тел; но в `MergePresetsIntoDNS` мы их
  больше не использует (используем lookup body напрямую через PresetDNSServer
  struct).
- `MergePresetsIntoRoute` — НЕ ТРОГАЕМ (route остаётся как есть).

LOC: ~−150 (упрощение), ~+80 (новый walk).

### config_service.go

```go
func dnsConfigForUpdate(s *state.State) build.DNSConfig {
    cfg := build.DNSConfig{}
    if len(s.RulesV6) > 0 || len(s.DNSV6.Servers) > 0 {
        // v6 path: scalars из DNSOptionsV6; servers/rules идут через ctx.Preset.DNS
        cfg.Final = s.DNSV6.Final
        cfg.Strategy = s.DNSV6.Strategy
        if s.DNSV6.IndependentCache {
            t := true
            cfg.IndependentCache = &t
        }
    } else if s.DNSOptions != nil {
        // pure-v5 path
        cfg.Final = s.DNSOptions.Final
        cfg.Strategy = s.DNSOptions.Strategy
        cfg.IndependentCache = s.DNSOptions.IndependentCache
        cfg.Servers = s.DNSOptions.Servers
        if len(s.DNSOptions.Rules) > 0 { ... }
    }
    // dns_* scalars из vars (override)
    for _, v := range s.Vars {
        switch v.Name {
        case "dns_final":              cfg.Final = v.Value
        case "dns_strategy":           cfg.Strategy = v.Value
        case "dns_independent_cache":  b := v.Value == "true"; cfg.IndependentCache = &b
        }
    }
    return cfg
}
```

LOC: ~−10, ~+10.

---

## Phase 5 — UI sync

Файлы `ui/configurator/models/preset_ref_sync.go`,
`ui/configurator/presentation/presenter_state.go`,
`ui/configurator/tabs/dns_tab.go`.

### SyncDNSFullToStateV6 → переписать

Старая сигнатура:
```go
SyncDNSFullToStateV6(dnsServers, dnsRulesText, templateOverrides, templateDNSTags) v6.DNSConfig
```

Новая:
```go
SyncDNSOptionsFromUI(uiServers, dnsRulesText, templateDNSTags, presetByID, activePresetIDs) v6.DNSOptions
```

Алгоритм:
- Для каждого uiServer:
  - tag ∈ templateDNSTags → `{Kind:template, Tag, Enabled}`
  - tag похож на `<preset_id>:<local>` && preset активен → `{Kind:preset, Ref, Enabled}`
  - иначе → `{Kind:user, Tag, Enabled, Body}`
- rulesText парсится: `kind=user` entries.
- Затем `SyncDNSOptionsWithActivePresets` материализует preset-entries для active presets.

### Rules tab hook

Где `presenter.TogglePresetRef` или эквивалент — после обновления `state.RulesV6`
вызвать `SyncDNSOptionsWithActivePresets(state.RulesV6, &state.DNSV6, presetByID)`.

### UI рендеринг DNS tab — Phase 5b (опционально не в этом проходе)

`dns_tab.go` — рендер из `state.DNSV6.Servers` с kind-aware tile'ами:
- template: только toggle
- preset: только toggle + label "from preset X"
- user: edit/delete

LOC: ~−80 (legacy sync), ~+100 (new sync), UI rendering — ~50.

---

## Phase 6 — Tests + docs

### Тесты

- `core/state/v6/dns_options_test.go` — JSON round-trip каждого kind.
- `core/state/v6/sync_dns_test.go` — lifecycle: enable preset → sync → entries
  appeared; disable → entries gone; preserve user toggle на re-sync.
- `core/state/v6_integration_test.go` — обновить под новый shape.
- `core/state/v6/migration_test.go` — обновить TestMigrateDNS*: v5→v6.DNSOptions.
- `core/build/preset_merge_test.go` — переписать DNS merge tests под new walk.
- `core/build/rules_pipeline_test.go` — обновить ExtraServers/ExtraRules тесты.
- `ui/configurator/models/preset_ref_sync_test.go` — обновить SyncDNSFullToStateV6.
- `ui/configurator/models/dns_state_test.go` — round-trip flat shape.

### Docs

- `docs/WIZARD_STATE.md` — полная DNS-секция переписана под новый shape
  (раз уж doc всё равно отстал — освежаем целиком).
- `SPECS/056-R-N-DNS_SCHEMA_REDESIGN/IMPLEMENTATION_REPORT.md` — финал.
- `SPECS/056-R-N-DNS_SCHEMA_REDESIGN/TASKS.md` — checklist завершения.

LOC: ~+300 тестов, ~−150 устаревших.

---

## Объём

- Code: чистый delta ~+30 LOC (большие удаления компенсируют новый kind switch).
- Tests: ~+150 LOC.
- Docs: ~+200 строк.

Реализация фаз 1-5 — 1.5 дня focused. Тесты + докс — ещё 0.5 дня.

## Риск

- Дев-only schema rewrite. parseV5 не затрагивается.
- v6 был только в HEAD-of-develop — backup механика покрывает мой single dev state.
- Round-trip тесты + lifecycle тесты должны поймать все edge case'ы.

## Откат

- Phase 1-2 можно влить без активации; код compile'ится, но не используется.
- Phase 3 (parseV6) — после этого state переходит в новый shape; откат =
  `git revert` + ручной `cp state.json.<backup_made_by_user>.bak state.json`.
- Phase 4+ — без Phase 3 несовместимо (build path требует new DNSOptions).
