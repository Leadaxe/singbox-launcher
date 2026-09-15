// File disk_v7.go — on-disk-схема state.json v7 (SPEC 118, этап 2).
//
// SPEC 127: v7 — ЛЕГАСИ-формат. Save пишет v8 (disk_v8.go), а v7-файл читает
// миграция по сырому документу (migration_v7_to_v8.go) — своего парсера в
// типы у v7 больше нет, потому что типы записей сменили форму. Здесь
// остаются константы схемы и справочная форма корня.
//
// Корень ПЛОСКИЙ (SPEC Т1): обёртки connections больше нет.
//
//	{
//	  "meta":          { version: 7, schema: "sources_v7", ... },
//	  "sources":       [ {kind, tag, enabled, ...} ],   // юнион по kind
//	  "directions":    [ ... ],                          // configtypes.Direction
//	  "rules":         [ ... ],                          // как v6
//	  "vars":          [ ... ],
//	  "dns_options":   { ... },
//	  "warp_accounts": { ... }
//	}
package state

// SchemaVersionV7 — формат файла state.json эпохи v7 (SPEC 118).
// SPEC 127: только вход миграции; Save пишет SchemaVersionV8.
const SchemaVersionV7 = 7

// SchemaNameV7 — внутренний идентификатор схемы v7 (хранится в meta.schema).
// Он же — «мажор схемы» для remote-гейта (SPEC Т10, волна W7).
const SchemaNameV7 = "sources_v7"

// migrationPurgesLegacy — гейт шага 8 миграции v6→v7 (PLAN §6): снос
// raw-кэша и легаси-ключей после миграции. ВКЛЮЧЁН волной W5: моста больше
// нет, легаси-поля никем не читаются, и держать их в файле значило бы
// хранить материал, который система уже не понимает.
//
// Константа оставлена (а не заинлайнена) как явная точка порядка: снос
// выполняется ТОЛЬКО после успешной записи v7-файла (load_router).
const migrationPurgesLegacy = true

// Корень v7 как справка о ключах (типы записей внутри — v7-форма, её знает
// только migration_v7_to_v8.go):
//
//	meta, sources, directions, rules, vars, dns_options, warp_accounts
