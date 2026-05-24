# SPEC 060-R-N — STATE_NAMESPACE_COLLAPSE

**Status:** New (N)
**Type:** Refactor (R) — collapse `core/state/v5/` + `core/state/v6/` subpackages в единый `core/state/`. Удаление `V6`/`V5` suffix'ов из field/type/func names.
**Depends on:** SPEC 058-R-N (должен быть merge'нут — namespace collapse поверх него уменьшит mixed-concern PR).
**Не меняет:** JSON wire format (`state.json` на диске), config.json emit, поведение runtime.

---

## Проблема

`core/state/` сейчас распадается на три уровня:

```
core/state/
├── state.go        — top-level State (с полями RulesV6 []v6.Rule, Connections v5.ConnectionsSection)
├── load.go         — parseV5 + parseV6 + parseLegacyAndMigrate
├── save.go         — marshalDisk (v5) + marshalDiskV6 + useV6 = hasPresetRefs(...) gate
├── v5/             — 9 файлов: MetaSection, ConnectionsSection, Source, MigrateV4ToV5, V4File, ulid, raw_cache
└── v6/             — 7 файлов: Rule, RuleKind, PresetBody/InlineBody/SrsBody, DNSOptions, DNSServer, DNSRule, MetaSection, IsV6
```

**Проблемы:**

1. **Двойной namespace** — callsite'ы пишут `v5.X` или `v6.Y`, иногда оба в одном файле. 37 файлов в коде импортят `core/state/v5` либо `core/state/v6`. Mental overhead для чтения кода.

2. **`useV6` dual write path** — `save.go` выбирает между v5-форматом и v6 на основе `hasPresetRefs(s.RulesV6)`. После SPEC 053 (preset bundles релизились в 0.9.x) и SPEC 058 (миграция всё мигрирует на v6) — все юзеры уже на v6 пишут. Dual path = dead branch.

3. **`State.RulesV6` field name** — `V6` суффикс в Go-коде неинформативен (это не "Rules v6 format", это просто "Rules"). JSON tag уже `"rules"` (без суффикса). Несогласованность.

4. **`v5.MetaSection` vs `v6.MetaSection`** — два почти-идентичных типа с разными SchemaVersion константами. Унифицируется в один.

5. **Legacy migrations** (`MigrateV4ToV5`, opt. `MigrateV5ToV6`) живут в подпакетах и тянут за собой namespace. Должны быть internal converter helper'ами без namespace exposure.

## Целевая модель

```
core/state/
├── state.go         — State struct (Rules []Rule, Connections ConnectionsSection, DNS DNSOptions, ...)
├── load.go          — Parse(data) → State (auto-detects legacy; нормализует в canonical shape)
├── save.go          — Save(path) — единственный write path (current shape)
├── rule_types.go    — Rule, RuleKind, PresetBody, InlineBody, SrsBody
├── dns_options.go   — DNSOptions, DNSServer, DNSRule
├── connections.go   — ConnectionsSection, Source, Defaults
├── meta.go          — MetaSection (single), SchemaVersion const
├── migration.go     — legacy v2/v3/v4/v5 → canonical (internal helpers)
├── adapter.go       — ParserConfig↔Connections legacy view sync
├── diff.go          — state diff utilities
├── ulid.go          — Source ID generation
└── raw_cache.go     — subscription raw body cache
```

**Wire format:** не меняется. `state.json` на диске продолжает писаться в том же layout что сейчас produced v6 path. `meta.version` остаётся (single canonical value).

## Изменения

### 1. Перемещение файлов

| Источник | Назначение |
|---|---|
| `core/state/v6/state.go::MetaSection` | `core/state/meta.go::MetaSection` |
| `core/state/v6/state.go::SchemaVersion = 6` | `core/state/meta.go::SchemaVersion = 6` |
| `core/state/v6/rule_types.go::*` | `core/state/rule_types.go::*` |
| `core/state/v6/dns_options.go::*` | `core/state/dns_options.go::*` |
| `core/state/v6/sync_dns.go::*` | `core/state/sync_dns.go::*` |
| `core/state/v6/IsV6` | удалить (используется только для legacy detection в Parse) |
| `core/state/v5/types.go::ConnectionsSection,Source,Defaults` | `core/state/connections.go::*` |
| `core/state/v5/types.go::MetaSection` | удалить (заменён унифицированным `core/state/meta.go`) |
| `core/state/v5/migration.go::MigrateV4ToV5` | `core/state/legacy_migration.go::migrateLegacy` (private) |
| `core/state/v5/v4.go::V4File` | `core/state/legacy_migration.go::v4File` (private struct) |
| `core/state/v5/ulid.go` | `core/state/ulid.go` |
| `core/state/v5/raw_cache.go` | `core/state/raw_cache.go` |
| `core/state/v5/adapter.go` (если есть) | `core/state/adapter.go` (мерж с существующим) |

Тесты двигаются параллельно: `v5/types_test.go` → `core/state/types_test.go` и т.д.

### 2. Переименования

| Старое | Новое | Где |
|---|---|---|
| `State.RulesV6 []v6.Rule` | `State.Rules []Rule` | `core/state/state.go` |
| `State.DNS v6.DNSOptions` | `State.DNS DNSOptions` | (тип теряет prefix) |
| `parseV6`, `parseV5`, `parseLegacyAndMigrate` | `parseCurrent`, `parseLegacy` (с auto-detect внутри `Parse`) | `core/state/load.go` |
| `marshalDisk`, `marshalDiskV6` | `marshalDisk` (single) | `core/state/save.go` |
| `legacyDevDNSToOptions`, `legacyCustomRulesFromV6` | без `V6` суффикса в имени | `core/state/load.go` |

JSON-тэги НЕ меняются — wire format stable.

### 3. Drop `useV6` write gate

```go
// Было:
useV6 := hasPresetRefs(s.RulesV6)
if useV6 { data, err = s.marshalDiskV6() } else { data, err = s.marshalDisk() }

// Стало:
data, err = s.marshalDisk()  // single path
```

`hasPresetRefs` функция удаляется (мёртвый код после collapse).

### 4. Drop `IsV6` detection helper

В `Parse(data)` оставляем auto-detect через `meta.version` integer прямо. v5 файлы (`top-level "version"` или `meta.version == 5`) → читаются через legacy parser, нормализуются в canonical State, на следующем Save переписываются в current shape.

### 5. SchemaVersion

Один constant `core/state/meta.go::SchemaVersion = 6` (текущая шкала). Read-side принимает `meta.version >= 5` (с auto-upgrade). Write-side всегда пишет `SchemaVersion`.

Опционально: bump `SchemaVersion = 7` чтобы маркировать "post-collapse + SPEC 058 shape", и старые v6 reader'ы (если бы такие были) знали что shape стал каноничным. Не критично — поведение совместимо.

## Migration

**Не нужна.** State.json на диске не меняется. Юзер не видит разницы.

## Acceptance

1. `grep -rn 'core/state/v5\|core/state/v6' core/ ui/` — ноль результатов.
2. `core/state/v5/` и `core/state/v6/` директории удалены.
3. Build (`go build ./...`) — clean.
4. Tests (`go test ./...`) — all pass без модификации test data.
5. Round-trip: load существующий state.json → save без правок → diff = byte-identical (или семантически identical с different field order но same content).
6. State `RulesV6` field renamed в `Rules` во всех 37+ callsite'ах. JSON tag `"rules"` сохранён.
7. Single `marshalDisk` функция, без dual path.
8. Acceptance: SPEC 053 тесты (preset bundles round-trip) проходят.
9. Acceptance: SPEC 056 тесты (DNS resolver) проходят.
10. Acceptance: SPEC 057/058 тесты (outbounds preset binding) проходят.

## План фаз

1. **Phase 1 — Inventory + plan target names:** список всех типов/функций/констант в v5 и v6, target name каждой. Решить collisions (например, два `MetaSection` мержатся в один).

2. **Phase 2 — Move v6/ → core/state/:** скопировать файлы наверх, удалить `package v6` declaration. Локальные ссылки внутри package остаются работать. Тесты тоже двигаются.

3. **Phase 3 — Move v5/ → core/state/:** аналогично. Resolve type collisions с v6 (MetaSection merge, etc.).

4. **Phase 4 — Update import callsites:** 37 файлов вне `core/state/` импортят v5/v6. Меняем на `core/state`, удаляем aliased imports (`v5 "..."`, `v6 "..."`).

5. **Phase 5 — Drop dual write + V6 suffix:** save.go выбрасывает `useV6`/`hasPresetRefs`. State.RulesV6 → State.Rules с JSON tag preserved. marshalDiskV6 → marshalDisk единственная.

6. **Phase 6 — Cleanup + tests + build + reinstall:** удалить пустые v5/v6 dirs, run all tests + go vet + manual smoke test на existing state.json.

## Риск

**Низкий.** Pure mechanical refactor:
- Wire format не меняется
- Поведение runtime не меняется
- Test coverage уже есть для preserved behaviors (SPEC 053/056/057/058 acceptance tests)

**Mitigation:** делать в отдельной серии коммитов после merge SPEC 058 в main. Каждая фаза = один коммит. Каждый коммит сам по себе buildable + tests pass. Bisect-friendly при regression.

## Не в скоупе

- Schema bump v6→v7 — опциональная косметика, не обязательна.
- Drop legacy v2/v3/v4 read path — отдельная decision, у каких-то self-hosted юзеров могут быть очень старые state'ы.
- Изменение adapter (`syncConnectionsFromLegacy` / `syncLegacyFromConnections`) — отдельная история (см. potential SPEC об удалении legacy ParserConfig view).
- DNS sync architecture — самостоятельный design (см. SPEC 056).

## Объём

- ~24 файла перемещаются (внутри core/state/)
- ~37 файлов с обновлением imports (callsite'ы)
- Несколько тестовых файлов сдвигаются
- Оценка: ~3 часа сфокусированной работы + 30 мин на тесты/install/smoke
