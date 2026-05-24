# SPEC 053 — Implementation Plan

Большой semantic refactor: thin-ref пресеты, локальный tag scope, per-preset DNS bundle, conditional fragments через `if`/`if_or`. Делаем фазами с green-tests на каждом этапе.

---

## Фаза 1 — Pure data types (preset + rule header/body)

**Цель:** Завести Go-типы, schema, JSON-сериализацию. Без интеграции с UI/build/migration.

### Файлы

- `core/template/preset_types.go` (новый):
  - `Preset`, `PresetVar`, `OptionEntry`, `PresetRuleSet`, `PresetDNSServer`
  - `PresetTitle`, `Description` поля для bundled DNS (UI vs sing-box)
  - `Options` как `json.RawMessage` + декодер по `Type`
  - `Select` для type=dns_server (`"local"`/`"global"`)
  - `If`/`IfOr` массивы на vars и fragments
- `core/template/preset_types_test.go`:
  - JSON round-trip всех форм preset'а (1-5 из SPEC)
  - `Options` декодер: `[]OptionEntry` (enum) vs `[]string` (dns_server/outbound)
  - omitempty на пустых полях
- `core/state/v6/rule_types.go` (новый):
  - `Rule` header (`Kind`, `Ref`/`ID`, `Enabled`, `Body`)
  - `PresetBody`, `InlineBody`, `SrsBody`
  - Discriminator-based parser dispatcher
- `core/state/v6/rule_types_test.go`:
  - Round-trip всех 3 kind'ов
  - Wrong identifier для kind → strip + warning

### Acceptance

- `go test ./core/template/... ./core/state/v6/...` — green
- Round-trip identity для preset из SPEC §«Real-world example»

---

## Фаза 2 — Template parser + validation

**Цель:** Загрузка `template.presets[]` из `wizard_template.json`, валидация по таблице из SPEC §«Validation».

### Файлы

- `core/template/preset_loader.go` (новый):
  - `LoadPresets(raw json.RawMessage) ([]Preset, []Warning)` — non-fatal warnings
  - Uniqueness check: `preset.id`, `vars[].name`, `rule_set[].tag`, `dns_servers[].tag`
  - Reference check: `rule.rule_set`, `dns_rule.rule_set`, `dns_rule.server` ссылки разрешимы
  - Vars `if`/`if_or` ссылается на bool vars того же preset'а
  - `default` ∈ resolved scope для dns_server/outbound/enum
  - `select` ∈ {"local","global"}, не для type ≠ dns_server
  - Vars scope collision check (preset.vars[i].name vs template.vars[*].Name) → warning
- `core/template/preset_loader_test.go`:
  - Каждый validation case даёт ожидаемый warning
  - Корректный preset проходит без warnings
- `core/template/loader.go` (модификация):
  - Парсинг новой `template.presets[]` секции (рядом с legacy `selectable_rules[]`)
  - Logging warnings через `debuglog`

### Acceptance

- `go test ./core/template/...` — green
- ru-direct из SPEC `Real-world example` парсится без warnings
- Намеренно-сломанный preset (duplicate tag, dangling ref) даёт ожидаемые warnings

---

## Фаза 3 — Preset expansion engine

**Цель:** Pure function `ExpandPreset(preset, varsValues, ctx) (Fragments, []Warning)` — substitute, if-filter, tag-prefix, dns filter, sentinel resolve, dangling ref cleanup.

### Файлы

- `core/build/preset_expand.go` (новый):
  - `_substitute(obj any, vars map[string]string) any` — рекурсивный JSON walk, **только текстовая замена** (no `_Dropped` sentinel)
  - `evalIf(vars, ifList, ifOrList) bool` — переиспользуем существующую `ParamBoolVarTrue` из `core/template/vars_resolve.go`
  - `filterByIf(fragments, vars)` — выкидывает фрагменты с `if=false`
  - `prefixTags(preset_id, fragment)` — переписывает все локальные tag refs в `<preset_id>:<tag>`
  - `filterDnsServers(preset, varsMap)` — оставляет только использованные (через `@dns_server` var или dns_rule.server литерал)
  - `applyOutboundSentinels` — wraps `ApplyOutboundToRule`
  - `cleanDanglingRuleSetRefs` — удаляет ссылки на отсутствующие rule_set tag'и из `rule.rule_set` / `dns_rule.rule_set`
  - `directOutDetourStrip` — `detour: "direct-out"` → удалить ключ
- `core/build/preset_expand_test.go`:
  - Real-world expansion case 1 (default varsValues) → expected fragments
  - Case 2 (`use_dns_override: false`) → no dns
  - Case 3 (`geoip_enabled: false`) → rule_set[2] dropped, ref clean
  - Case 4 (юзер выбрал yandex_doh) → filter swap
  - Broken cases: unresolved @var, unknown var в varsValues

### Acceptance

- `go test ./core/build/preset_expand_test.go` — green
- Все 4 case'а ru-direct из SPEC дают byte-equal fragments (golden test)

---

## Фаза 4 — State v5 → v6 migration

**Цель:** Загружать v5 state, конвертировать в v6, писать v6. Backup старого файла. Идемпотентно.

### Файлы

- `core/state/v6/migration.go` (новый):
  - `MigrateV5ToV6(old State) (newState, []Warning)` — pure func
  - `custom_rules[]` → `rules[]` с kind discriminator (inline/srs heuristic)
  - `selectable_rule_states` (legacy) → preset-refs если ID совпадает по label
  - `dns_options.servers[]` → `template_servers` override map + `extra_servers`
  - `dns_options.rules[]` → `extra_rules`
- `core/state/v6/migration_test.go`:
  - Real-world v5 fixture → expected v6
  - Идемпотентность: v6 → migrate (noop) → v6
- `core/state/load.go` (модификация):
  - Detection: `meta.version == 5` → migrate, `== 6` → load directly
  - Backup `state.json` → `state.json.v5.bak` при первом upgrade'е
- `core/state/save.go` (модификация):
  - Always write v6

### Acceptance

- Real v5 state.json (из install) загружается, мигрирует, сохраняется
- Backup создаётся ровно один раз
- v6 → load → save → load — idempotent
- `go test ./core/state/...` — green

---

## Фаза 5 — Build pipeline integration

**Цель:** Подключить `preset_expand` в Rebuild pipeline. DNS section собирается из template_defaults + bundled + extras с effective_enabled resolve.

### Файлы

- `core/build/rules_pipeline.go` (новый, или extension в существующий):
  - `BuildRulesAndDNS(template, state, ctx) (RouteSection, DNSSection)`:
    - Для каждого `state.rules[]` элемента:
      - `kind=preset` → `ExpandPreset` → append fragments
      - `kind=inline` → emit headless rule_set + route rule
      - `kind=srs` → emit local rule_set + route rule (или skip если файл missing)
    - Merge стратегия: identical-skip по tag'у, first-wins при conflict
    - DNS: `template.dns_defaults.servers[].filter(effective_enabled) + bundled + extras`
    - Hijack-dns rule в начале route.rules[]
- `core/build/rules_pipeline_test.go`:
  - Mixed state (preset + inline + srs) → корректный merged config.json
  - Identical-skip и first-wins сценарии
  - `state.dns.template_servers[tag].enabled` override резолвится правильно
- `core/rebuild.go` (модификация):
  - Замена legacy `applyCustomRules` + DNS merge на новый pipeline
- `core/build/testdata/golden/preset_*.json` (новые fixtures):
  - `ru-direct-default.json`, `ru-direct-no-dns.json`, `ru-direct-no-geoip.json`
  - `multi-preset-and-user.json`

### Acceptance

- Golden tests byte-equal для всех ru-direct case'ов
- sing-box принимает эмитнутый config (orphan-tag check + parse)
- `state.dns.template_servers` override корректно влияет на dns.servers[]

---

## Фаза 6 — UI: Rules tab refactor

**Цель:** Rules tab показывает все 3 kind'а, edit dialog'и kind-specific, library — add as preset-ref.

### Файлы

- `ui/configurator/models/rule_state.go` (модификация):
  - Заменить старый `RuleState{Rule: TemplateSelectableRule, ...}` на новый `Rule{Kind, Ref/ID, Enabled, Body}`
  - Body — typed (PresetBody / InlineBody / SrsBody)
- `ui/configurator/tabs/rules_tab.go` (модификация):
  - Tile показывает все 3 kind'а
  - Summary для preset-ref: краткие non-default varsValues
  - Summary для inline: краткий match-resume
  - Summary для srs: `· srs` marker
  - Broken preset marker `⚠ Broken preset`
- `ui/configurator/dialogs/edit_preset_rule_dialog.go` (новый):
  - Только vars rendering (универсальный по type)
  - Outbound picker
  - DNS server grouped picker
  - Enum dropdown с {title, value}
  - Bool checkbox c if/if_or зависимостями
  - Preview section (показывает emit'нутые fragments)
  - Broken preset state: warning баннер + Delete only
- `ui/configurator/dialogs/add_rule_dialog.go` (модификация):
  - Только для inline/srs (preset edit отдельный dialog)
  - Без legacy selectable_rule импорта
- `ui/configurator/tabs/library_rules_dialog.go` (модификация):
  - "Add to Rules" создаёт `{kind: "preset", ref, body: {vars: {}}}`
  - Disabled если ref уже в state.rules[]

### Acceptance

- Открыл Configurator → видит свои правила (preset + inline + srs)
- Tap на preset-ref → vars-only dialog
- Tap на inline/srs → полный match-форм dialog
- Add from library → preset-ref добавляется, library button disabled
- Broken preset (template без ref'а) → warning + Delete only

---

## Фаза 7 — UI: DNS tab refactor

**Цель:** DNS tab показывает три источника серверов (template / active presets / extras), checkbox'ы пишут в state.dns.template_servers overrides.

### Файлы

- `ui/configurator/tabs/dns_tab.go` (модификация):
  - Секция "Default DNS servers (from template)" — список template.dns_defaults.servers с checkbox'ами
  - effective_enabled resolve через `state.dns.template_servers[tag].enabled ?? default_enabled`
  - `(overridden)` marker если значение != default_enabled
  - Секция "From active presets (read-only)" — список bundled DNS-серверов с source-marker'ом
  - Секция "Extra servers (user-defined)" — `state.dns.extra_servers[]`, add/edit/delete
  - "Extra rules" JSON editor — `state.dns.extra_rules[]`
- `ui/configurator/business/dns_servers.go` (новый/extension):
  - `ResolveDNSServers(template, state) [](tag, source, effectiveEnabled)` — для UI рендера
  - source ∈ {template, preset:<id>, extra}

### Acceptance

- DNS tab корректно показывает источник каждого сервера
- Checkbox у template-сервера пишет override в state, marker `(overridden)` появляется
- Bundled DNS из активного preset'а виден read-only, исчезает при disable preset'а

---

## Фаза 8 — Content: portable ru-direct preset + cleanup

**Цель:** Создать первый параметризованный preset в `bin/wizard_template.json`. Cleanup legacy. Docs.

### Файлы

- `bin/wizard_template.json`:
  - Добавить `presets[]` секцию с ru-direct (real-world example из SPEC)
  - Migrate `selectable_rules[]` → `presets[]` (для существующих простых пресетов: private-ips-direct, block-ads и т.д.)
  - Удалить `route.rule_set[]` (rule_set'ы переехали в preset'ы)
  - Заменить `dns_options.servers[].enabled` → `default_enabled`
  - Удалить `dns_options.rules[]`
- `core/template/loader.go`:
  - Deprecated handling старой `selectable_rules` секции (фаза legacy → один-два релиза)
- `docs/RELEASE_PROCESS.md`:
  - §5.2 обновление `RequiredTemplateRef` теперь критично для preset content
- `docs/ARCHITECTURE.md`:
  - Раздел про preset bundles
- `docs/release_notes/upcoming.md`:
  - SPEC 053 entry (EN + RU)
- `SPECS/053-F-N-PRESET_BUNDLES/IMPLEMENTATION_REPORT.md`:
  - Final report
- Переименование `SPECS/053-F-N-` → `053-F-C-` (close-out)

### Acceptance

- Юзер апгрейдит → migration работает, ru-direct появляется как preset-ref
- Изменение default value у var в template (bump) → у юзера автоматически новый default (если он не override'ил)
- CI: `go build ./... && go test ./...` зелёные

---

## Тестовая стратегия

| Уровень | Что | Где |
|---|---|---|
| Unit | Pure data types, JSON round-trip | `core/template/preset_types_test.go`, `core/state/v6/rule_types_test.go` |
| Unit | Template parser + validation | `core/template/preset_loader_test.go` |
| Unit | Expansion engine — substitute, if, prefix, dns filter | `core/build/preset_expand_test.go` |
| Unit | Migration v5 → v6 | `core/state/v6/migration_test.go` |
| Golden | BuildConfig output для ru-direct (4 case'а) | `core/build/testdata/golden/preset_*.json` |
| Golden | Mixed (preset + inline + srs) → config | `core/build/testdata/golden/mixed_*.json` |
| Integration | Real v5 state.json → migrate → load → save → load | `core/state/migration_e2e_test.go` |
| E2E | Configurator → Edit preset-ref → Save → Restart → sing-box stays up | manual |

---

## Sequencing constraints

```
Фаза 1 (data types)
   │
   ├──→ Фаза 2 (template parser)
   │
   └──→ Фаза 3 (expansion engine)
            │
            ├──→ Фаза 4 (migration v5→v6)
            │       │
            └──→ Фаза 5 (build pipeline)
                    │
                    ├──→ Фаза 6 (UI Rules tab)
                    │
                    └──→ Фаза 7 (UI DNS tab)
                            │
                            └──→ Фаза 8 (content + cleanup)
```

Каждая фаза самодостаточна: build green, tests green, no UX regression до Фазы 6 (старый UI продолжает работать на новом data model через адаптеры).

---

## Риски и mitigations

| Риск | Mitigation |
|---|---|
| Substitute engine баг → пресет ломает config | Phase 3 golden tests на ru-direct (4 case'а) перед Phase 5 |
| Migration v5→v6 destructive на real юзерах | Backup `state.json.v5.bak` перед перезаписью; идемпотентность; runtime detection version |
| Broken preset после template bump → юзер потерял правило | UI marker + auto-skip; правило **не удаляется** автоматически — может вернуться когда template снова добавит ref |
| DNS dual-storage (bundled + extras + template overrides) → конфликты | Identical-skip + first-wins + warning по tag'у; явные секции в UI с source-marker'ом |
| Coordinate preset.vars[name] ↔ template.vars[name] collision | Phase 2 warning на load; preset-local scope wins; no cross-scope substitute |
| LxBox-portable пресет содержит `update_interval` / `enabled` поля | Phase 2 strip unknown fields + warning; preserve known sing-box fields |

---

## Не-цели этого SPEC'а

- UI-редактор пресетов в самом launcher'е (только в template repo)
- Импорт/экспорт user preset bundles как JSON
- Reactive обновление vars без reconfigure sing-box
- Конверсия preset-ref ↔ user inline/srs в обе стороны
- Namespacing tag'ов user-defined правил (сейчас префикс `user:<id>`)
