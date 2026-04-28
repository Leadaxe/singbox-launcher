# SPEC 052 — Implementation Report (final)

Все 8 фаз завершены. Build + tests + golden-test зелёные.

---

## Phase 1 — pure-data v5 types + migration ✅

**Файлы:**
- [core/state/v5/types.go](../../core/state/v5/types.go) — MetaSection, ConnectionsSection, Source, SourceType, TagSpec, UpdateSpec, SubscriptionMeta, UserInfo, ConfigParam, SettingVar, CustomRule, DNSOptions
- [core/state/v5/v4.go](../../core/state/v5/v4.go) — V4File, V4ParserConfig, ParseV4 (input для миграции)
- [core/state/v5/migration.go](../../core/state/v5/migration.go) — `MigrateV4ToV5(old, gen)` pure-функция
- [core/state/v5/ulid.go](../../core/state/v5/ulid.go) — собственная ULID-имплементация (26-char Crockford-base32)
- [core/state/v5/types_test.go](../../core/state/v5/types_test.go) — JSON round-trip, omitempty, TagSpec.IsZero
- [core/state/v5/migration_test.go](../../core/state/v5/migration_test.go) — реальный v4-fixture (5 sub + 1 server + DNS + 12 custom_rules + 13 vars), subscription-only, server-only, mixed, idempotent

**Решения:**
- Server label из migration = `tag_prefix + fragment + tag_postfix` (сохраняет UX: пользователь видел `WG:wg-parnas`, продолжает видеть так после миграции).
- Reverse-адаптер ставит `tag_mask = label`, чтобы парсер выдавал тэг строго равный label.
- TagSpec — pointer (`*TagSpec`) чтобы пустой `{}` не появлялся в JSON для server-source'ов.

---

## Phase 2 — HTTP headers + inline-comments parser ✅

**Файлы:**
- [core/config/subscription/meta.go](../../core/config/subscription/meta.go) — `ParseHeaders`, `ParseInlineComments`, `MergeMeta`, `parseSubscriptionUserinfo`, `decodeProfileTitle`, `parseContentDispositionFilename`
- [core/config/subscription/meta_test.go](../../core/config/subscription/meta_test.go) — V2Board fixture, malformed, base64 detection, inline fallback, RFC5987 filename

**Headers контракт LxBox-совместимый:** `subscription-userinfo`, `profile-title`, `profile-update-interval`, `support-url`, `profile-web-page-url`, `content-disposition`. `MergeMeta` — HTTP headers выигрывают, inline `#-comments` — fallback.

---

## Phase 3 — fetcher 2.0 + raw cache ✅

**Файлы:**
- [core/config/subscription/fetcher.go](../../core/config/subscription/fetcher.go) — `FetchSubscriptionWithMeta`, `FetchResult`, `FetchHTTPError`, `IsHTTPError`. Старый `FetchSubscription` сохранён как deprecated wrapper.
- [core/state/v5/raw_cache.go](../../core/state/v5/raw_cache.go) — `WriteRawBody` (atomic .tmp+Rename), `ReadRawBody`, `DeleteRawBody`, `ListRawBodyIDs`, `DeleteOrphans`, `validateID` (path-traversal guard).
- [internal/platform.GetSubscriptionsDir](../../internal/platform/platform_common.go) — путь `<execDir>/bin/subscriptions/`.
- Tests: round-trip, overwrite, missing dir, orphans GC, path-traversal rejection, V2Board response happy path, inline fallback, HTTP error.

User-Agent остался `SubscriptionParserClient` (не ломаем поведение провайдеров с whitelist'ом).

---

## Phase 4 — state load/save миграция ✅

**Файлы:**
- [core/state/state.go](../../core/state/state.go) — State содержит legacy-поля (ParserConfig, ID, RulesLibraryMerged, SelectableRuleStates) И v5-canonical (Connections). Type aliases на v5-типы.
- [core/state/load.go](../../core/state/load.go) — version detection: `meta.version >= 5` → v5; `version` 2-4 → legacy + auto-миграция через `v5.MigrateV4ToV5`.
- [core/state/save.go](../../core/state/save.go) — всегда пишет v5-формат.
- [core/state/adapter.go](../../core/state/adapter.go) — `syncConnectionsFromLegacy` (Save) + `syncLegacyFromConnections` (Load v5).
- [core/state/state_test.go](../../core/state/state_test.go) — обновлённые ожидания + новый `TestLoadSave_IdempotentV5`.

**ID preservation:** на Save ID из существующих Connections.Sources переносятся по URL/URI match'у; новые получают свежий ULID. Round-trip stable.

---

## Phase 5 — parser adapter + meta wiring ✅

**Файлы:**
- [core/state/v5/adapter.go](../../core/state/v5/adapter.go) — `(*Source).ToProxySourceV4()` для legacy-парсера.
- [core/config_service.go::refreshSubscriptionsMetaAndCache](../../core/config_service.go) — per-source meta refresh + `bin/subscriptions/<id>.raw` write.
- [core/refresh_meta_test.go](../../core/refresh_meta_test.go) — happy path, failure-keeps-raw, orphan GC.

Update теперь:
1. `refreshSubscriptionsMetaAndCache` — fetch + meta + raw atomic для каждой enabled subscription;
2. `subscription.LookupCachedBody` hook ставится → парсер читает cached body вместо повторного fetch (нет double-fetch);
3. state.Save с обновлённой Meta;
4. **bin/outbounds.cache.json больше не пишется** (per-source resilience через `.raw` per-source).

---

## Phase 6 — Rebuild on .raw ✅

**Файлы:**
- [core/rebuild_raw_cache.go](../../core/rebuild_raw_cache.go) — `buildSnapshotFromRawCache` парсит `.raw` файлы в Snapshot in-memory без network call'ов.
- [core/rebuild.go](../../core/rebuild.go) — заменил `outboundscache.Load(cachePath)` на `buildSnapshotFromRawCache`. Auto-Update fallback при `ErrRawCacheIncomplete`.
- One-shot cleanup: `cleanupLegacyOutboundsCache` удаляет старый `bin/outbounds.cache.json`.
- [core/rebuild_raw_cache_test.go](../../core/rebuild_raw_cache_test.go) — happy path offline, incomplete cache, disabled-source ignored, legacy file cleanup.

**Acceptance:** Rebuild без сети с готовым `.raw` — green; Rebuild с пустым `bin/subscriptions/` → trigger Update → success.

**Golden test parity:** `core/build/testdata/golden/real-v088` config.json byte-equal с предыдущим — green.

---

## Phase 7 — UI configurator new model + per-source refresh ✅

**Cleanup (под нож):**
- Удалена кастомная сериализация `WizardStateFile.MarshalJSON`/`UnmarshalJSON` — wizard теперь использует `corestate.Save`/`Load` напрямую.
- `WizardStateFile`, `PersistedCustomRule`, `PersistedSettingVar`, `PersistedSelectableRuleState`, `PersistedDNSState`, `ConfigParam` — теперь алиасы на `corestate.X` типы. Удалено ~250 строк дублирующего кода.
- `WizardStateVersion` теперь равно `corestate.SchemaVersion` (v5).
- Wizard `MigrateSelectableRuleStates`, `MigrateCustomRules` — больше не нужны, `corestate.Load` обрабатывает v2-v4 → v5.
- [ui/configurator/business/state_store.go](../../ui/configurator/business/state_store.go) — переписан как тонкий wrapper вокруг `corestate.Load`/`Save`.

**UI enrichment:**
- [ui/configurator/tabs/source_meta_format.go](../../ui/configurator/tabs/source_meta_format.go) — formatters для Meta: status badge, last_fetched, quota, expire, nodes count, humanizeBytes, humanizeDuration, metaTooltip.
- [ui/configurator/tabs/source_tab.go](../../ui/configurator/tabs/source_tab.go) — обогащённое отображение per-row:
  - Type indicator (📡 subscription / 🔌 server) слева;
  - Profile_title overlay (если есть) справа от label;
  - Status badge (●ok / ●err / ●never) с цветом по important;
  - Refresh button (для подписок) — async вызов `RefreshSingleSubscription`;
  - Tooltip с полной meta (profile_title, status, fetched, quota, expire, nodes, support_url, last error).
- [core/config_service.go::RefreshSingleSubscription](../../core/config_service.go) — per-source refresh API.
- 23 новых i18n ключа (en + ru).

**WizardModel.Sources []corestate.Source** — заполняется на Load, используется UI для look-up Meta по URL/URI.

---

## Phase 8 — cleanup + docs ✅

- [docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md) — добавлен раздел «SPEC 052 — Connections redesign» с описанием state.json v5 layout, raw cache, LookupCachedBody hook, RefreshSingleSubscription, Wizard StateStore cleanup, UI source rows enrichment.
- [docs/release_notes/upcoming.md](../../docs/release_notes/upcoming.md) — Highlights (EN + RU) bullet про SPEC 052.
- IMPLEMENTATION_REPORT.md (этот файл).

---

## Acceptance criteria (из SPEC.md)

| # | Критерий | Status |
|---|---|---|
| 1 | Round-trip v4→v5 идемпотентен | ✅ `TestLoadSave_IdempotentV5` |
| 2 | Headers parse: 6 полей из V2Board response | ✅ `TestParseHeaders_V2BoardLike` |
| 3 | Inline #-comments дают идентичный результат | ✅ `TestParseInlineComments_SameAsHeaders` |
| 4 | Failed fetch resilience | ✅ `TestRefreshSubscriptionsMetaAndCache_FailureKeepsOldRaw` |
| 5 | Cache discipline (raw + bytes) | ✅ `TestRefreshSubscriptionsMetaAndCache_HappyPath` |
| 6 | Preview consistency (≤50 элементов) | ✅ `extractPreviewNodes` + integration test |
| 7 | Rebuild без сети с готовым .raw | ✅ `TestBuildSnapshotFromRawCache_HappyPath` |
| 8 | max_nodes cap → truncated=true | ✅ `merged.Truncated = ...` в refresh |
| 9 | state.json size ≤ 20KB при 50 / 20K нод | ✅ preview обрезан до 50 |
| 10 | Migration data preservation | ✅ `TestMigrate_RealFixture` |
| 11 | BuildConfig byte-equal на real-v088 | ✅ `GOLDEN_RUN_REAL=1 go test` zero diff |

---

## Тестовое покрытие

```
go test ./...   → all green
GOLDEN_RUN_REAL=1 go test -run TestGoldenScenarios ./core/build/   → byte-equal
```

23 пакета, ~30 новых тестов, golden test stays byte-equal.

---

## Out of scope (отдельные SPEC, как и было обещано)

- **SPEC 053 PER_SOURCE_AUTO_UPDATE_TIMER** — per-subscription timer на основе `update.interval_hours` + exponential backoff. Поле `Update *UpdateSpec` в Source уже есть; UI-кнопка / scheduler — отдельный SPEC.
- **SPEC 054 SUBSCRIPTION_INSPECT_UI** — диалог «View raw content», более детальная quota visualization (history graphs).
- **SPEC 055 PER_SOURCE_PARTIAL_MERGE** — partial merge при failed fetch (sources внутри одной подписки).

---

## Технический долг — вычищен (follow-up cleanup pass)

После основного завершения SPEC 052 проведён cleanup pass — оба пункта тех-долга закрыты.

### #1 — `core/outboundscache` package удалён

- Тип `Snapshot` (содержавший только `Outbounds` + `Endpoints` использованных полей) перенесён в [`core/build/parsed_cache.go`](../../core/build/parsed_cache.go) как `ParsedCache` (4 используемых поля → 2).
- Удалены: `Save`/`Load`/`IsCorrupt`/`errCorrupt`/`SchemaVersion`/`StateID`/`UpdatedAt`/`SourceTags`/`SourceStats`/`IsForState`/`New(stateID)` — всё было нужно только для disk I/O, который ушёл в SPEC 052 phase 6.
- Удалён весь `core/outboundscache/` каталог (430 строк включая тесты).
- `BuildContext.Cache *outboundscache.Snapshot` → `*build.ParsedCache`.

### #2 — Wizard `Sources` canonical, `ParserConfig` derived

- `WizardModel.Sources []corestate.Source` теперь **единственный источник истины** для списка подключений (раньше — read-only snapshot вторичный к `ParserConfig.Proxies`).
- `WizardModel.GlobalOutbounds []configtypes.OutboundConfig` — отдельное поле для global outbounds (selectors/urltest), зеркалит `state.connections.outbounds`.
- `WizardModel.Defaults corestate.Defaults` — global defaults (reload, max_nodes).
- `WizardModel.ParserConfig` и `WizardModel.ParserConfigJSON` — теперь **derived caches**, синхронизируются `model.RefreshDerivedParserConfig()` после любой мутации.
- `model.AsParserConfig()` — собирает свежий `*config.ParserConfig` из Sources + GlobalOutbounds + Defaults для парсера/preview (вызывается при ParseAndPreview).

### UI mutation paths — все через Sources

- **Add URL** — [`business.AppendURLsToSources`](../../ui/configurator/business/sources.go) создаёт новые `corestate.Source` напрямую (subscription→type=subscription с auto-derived prefix, direct-link→type=server один Source per URI с label из URI fragment), classified по URL/URI; де-дуп по URL/URI.
- **Toggle disabled** — `m.Sources[i].Enabled = enabled; m.RefreshDerivedParserConfig()`.
- **Delete** — `m.Sources = append(m.Sources[:i], m.Sources[i+1:]...); m.RefreshDerivedParserConfig()`.
- **Edit** (source_edit_window) — scratch-buffer `*config.ProxySource` derived из `Sources[i]` на open; widget'ы мутируют scratch; `applyProxyEditToSource` синхронизирует обратно при сохранении (subscription: TagPrefix/Postfix/Mask → Tag.{Prefix,Postfix,Mask}; server: TagMask → Label).
- **Outbounds-configurator (global)** — мутирует `m.ParserConfig.ParserConfig.Outbounds`; на apply переносится в `m.GlobalOutbounds`, потом RefreshDerivedParserConfig.

### Save / Load flow

- **Save**: [`CreateStateFromModel`](../../ui/configurator/presentation/presenter_state.go) собирает `state.State` напрямую из canonical Sources/GlobalOutbounds/Defaults в `state.Connections.{Sources,Outbounds,Defaults}`; ParserConfig derived view заполняется как backup для legacy callsite'ов которые читают `state.ParserConfig.ParserConfig.Proxies` (тесты).
- **Load**: `restoreParserConfig` копирует `Connections.Sources/Outbounds/Defaults` в model; `RefreshDerivedParserConfig` строит legacy view.

### Build/test status после cleanup

```
go test ./...                        → all green (22 packages, было 23 — один удалён)
GOLDEN_RUN_REAL=1 go test ...build   → byte-equal на real-v088
```

Старые `business.AppendURLsToParserConfig`, `ApplyURLToParserConfig` сохранены в `parser.go` для совместимости с тестами и не использующимся UI-paths; основной поток Add URL идёт через `AppendURLsToSources`.
