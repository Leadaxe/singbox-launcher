# SPEC 052 — Tasks

## Phase 1 — pure-data types
- [ ] `core/state/v5/types.go` — MetaSection, ConnectionsSection, Source, TagSpec, UpdateSpec, SubscriptionMeta, UserInfo, SourceType
- [ ] `core/state/v5/types_test.go` — JSON round-trip, omitempty
- [ ] `core/state/v5/migration.go` — `MigrateV4ToV5(old) → new`
- [ ] `core/state/v5/migration_test.go` — реальный fixture v4 → expected v5
- [ ] golden v4 fixture: real `/Applications/.../state.json`
- [ ] golden v5 expected: ручная transcription того же state в v5

## Phase 2 — headers + inline parser
- [ ] `core/config/subscription/meta.go`
  - [ ] `ParseHeaders(http.Header) MetaFromHeaders`
  - [ ] `ParseInlineComments([]byte) MetaFromHeaders`
  - [ ] `MergeMeta(headers, inline) SubscriptionMeta`
  - [ ] `parseSubscriptionUserinfo(s) UserInfo`
  - [ ] `decodeProfileTitle(s) string` (base64 detection + UTF-8)
  - [ ] `parseContentDispositionFilename(s) string`
- [ ] `core/config/subscription/meta_test.go`
  - [ ] V2Board response fixture
  - [ ] Xboard response fixture
  - [ ] inline-comment variant даёт идентичный результат
  - [ ] malformed input doesn't panic

## Phase 3 — fetcher 2.0 + raw cache
- [ ] `core/config/subscription/fetcher.go`
  - [ ] `FetchResult` struct
  - [ ] `FetchSubscriptionWithMeta(url) (*FetchResult, error)`
  - [ ] preserve old `FetchSubscription` as deprecated wrapper
- [ ] `core/state/v5/raw_cache.go`
  - [ ] `WriteRawBody(execDir, id, body)` — atomic
  - [ ] `ReadRawBody(execDir, id) ([]byte, error)`
  - [ ] `DeleteRawBody(execDir, id)` — для GC
  - [ ] `ListRawBodyIDs(execDir) []string`
  - [ ] `DeleteOrphans(execDir, knownIDs)` — lazy GC
- [ ] `core/state/v5/raw_cache_test.go`
- [ ] `internal/platform.GetSubscriptionsDir(execDir) string` — helper для пути

## Phase 4 — load/save migration
- [ ] `core/state/load.go` — version detection branch (v4 / v5 / unknown)
- [ ] `core/state/save.go` — always write v5
- [ ] `core/state/state.go` — переход на `v5.State`
- [ ] adapter helpers: `s.GetSubscriptionSources()`, `s.GetServerSources()`
- [ ] tests: реальный v4 → load → save → load — идемпотентно
- [ ] tests: v5 → load → save → load — идемпотентно

## Phase 5 — parser adapter
- [ ] `core/state/v5/adapter.go`
  - [ ] `(*Source).ToProxySourceV4() configtypes.ProxySource` — для legacy парсеров
- [ ] `core/config_service.go::loadParserConfigForUpdate`
  - [ ] чтение из new state.connections.sources (только type=subscription)
  - [ ] adapter в `configtypes.ProxySource` для существующего парсера
- [ ] `core/config_service.go::UpdateConfigFromSubscriptions`
  - [ ] заменить `FetchSubscription` на `FetchSubscriptionWithMeta`
  - [ ] заполнить `state.connections.sources[i].meta` после fetch'а
  - [ ] сохранить state через StateService (mutex)
  - [ ] заполнить `meta.preview_nodes` (первые 50)
  - [ ] обновить `meta.{last_fetched_at, last_status, error_count, ...}`
  - [ ] `bin/subscriptions/<id>.raw` write
- [ ] golden test: BuildConfig output идентичен previous (byte-equal на real-v088 fixture)

## Phase 6 — Rebuild on .raw
- [ ] `core/rebuild.go::RebuildConfigIfDirty`
  - [ ] заменить `outboundscache.Load` на чтение `bin/subscriptions/*.raw`
  - [ ] re-parse каждого `.raw` через парсер для получения нод
  - [ ] in-memory cache parsed результата на сессию
  - [ ] auto-Update fallback при отсутствии `.raw`
- [ ] `core/outboundscache/snapshot.go` — mark deprecated, удалить в фазе 8
- [ ] cleanup: удалить `bin/outbounds.cache.json` при v4→v5 миграции (one-shot)
- [ ] tests: rebuild без сети с готовым `.raw` — green
- [ ] tests: rebuild с пустым `bin/subscriptions/` → trigger Update → success

## Phase 7 — UI
- [ ] `ui/configurator/models/wizard_model.go` — переход на `[]Source`
- [ ] `ui/configurator/business/parser.go` — save/load на v5
- [ ] `ui/configurator/business/loader.go` — fallback на template (без изменений по схеме)
- [ ] `ui/configurator/tabs/source_tab.go`:
  - [ ] секция «Subscriptions» — список `type=subscription`
    - [ ] для каждой: label/url + meta-bar (quota, expire, last_fetched, status)
    - [ ] кнопки: Edit / Refresh / Delete
  - [ ] секция «Servers» — список `type=server`
    - [ ] для каждой: label + uri preview
    - [ ] кнопки: Edit / Delete
  - [ ] Add: dialog «Subscription URL» / «Server URI» (две вкладки)
- [ ] i18n keys: `connections.section_subscriptions`, `connections.section_servers`,
  `connections.subscription_status_ok`, `connections.subscription_quota`, ...
- [ ] tests: configurator/business/* — green

## Phase 8 — cleanup + docs
- [~] `core/outboundscache/` — Snapshot type сохранён как in-memory data carrier; disk I/O dead (Save/Load/IsCorrupt unreferenced); package-level removal — отдельный follow-up patch
- [x] `docs/ARCHITECTURE.md` — добавлен раздел «SPEC 052 — Connections redesign»
- [x] `docs/release_notes/upcoming.md` — entry про SPEC 052 (EN + RU)
- [x] `SPECS/052-F-C-CONNECTIONS_REDESIGN/IMPLEMENTATION_REPORT.md` — final
- [x] переименован SPEC dir `052-F-N-` → `052-F-C-`
- [x] CI: `go build ./... && go test ./...` зелёные; `GOLDEN_RUN_REAL=1` golden byte-equal

## Out of scope (отдельные SPEC, не в этом)
- [ ] **SPEC 053 PER_SOURCE_AUTO_UPDATE_TIMER** — per-subscription timer
- [ ] **SPEC 054 SUBSCRIPTION_INSPECT_UI** — Refresh button per-sub, View raw, quota viz
- [ ] **SPEC 055 PER_SOURCE_PARTIAL_MERGE** — partial merge при failed fetch
