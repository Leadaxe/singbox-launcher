# SPEC 118 · W5 «Смерть легаси» — отчёт

Мост §6 PLAN больше не существует: `grep -rn "TEMPORARY BRIDGE" --include=*.go .`
→ 0. Сборка идёт ТОЛЬКО из материализованных `nodes[]`; конвейер сборки не
принимает загрузчик тел и не может его позвать по построению.

## 1. Снесённые файлы

| Файл | Что жило | Куда переехало |
|---|---|---|
| `core/state/raw_cache.go` | API raw-кэша подписок (чтение/запись/сироты) | чтение и удаление кэша — только вход миграции (`core/state/migration_raw_cache.go`, приватный) |
| `core/rebuild_raw_cache.go` | сборка снимка разбором `.raw` | `core/rebuild_snapshot.go` — эмиссия из `nodes[]` (`buildSnapshotFromState`) |
| `core/config/configtypes/source_fold.go` | `SourceFold`, `FoldMode*`, `FoldSelectTag/FoldAutoTag`, `EffectiveTagPrefix` | `FolderReplace` (канон) + приватная `legacyFold` во входе миграции + `backup.Fold` на границе контракта |
| `core/config/source_folds.go` | `PrepareSourceFolds` — разворот свёртки на сборке | `PrepareFolderReplaces` (уже был, W4) |
| `ui/configurator/tabs/source_fold_tab.go` | вкладка «Группа» на `SourceFold` | `ui/configurator/tabs/source_replace_tab.go` — те же контролы mode/strategy плюс ЯВНОЕ поле тега |

## 2. Снесённые символы (прод-код)

**Типы состояния (`core/state`).** Из `Source` ушли: `NodeTag`, `URI`,
`ConfigJSON`, `Chain`, `Outbounds`, `ExcludeFromGlobal`,
`ExposeGroupTagsToGlobal`, `Fold`, `DetourTag`, `DetourNodeSourceID`,
`DetourNodeTag`, `DetourNodeHash`, `DetourNodeLabel`, `DisabledNodes`. Из
`TagPolicy` — `Mask`. Из `SubMeta` — вся fetch-история и `PreviewNodes`.
Умерли алиасы `SourceType`/`SourceTypeXxx`, `TagSpec`, `SubscriptionMeta`.
Ключ `legacy_defaults` из v7-файла и `Defaults` из `WizardModel`.

**Вход миграции.** Легаси-формы переехали в приватные сайдкары рядом с
миграцией — они никогда не доезжают ни до сборки, ни до диска:

- `core/state/migration_legacy_source.go` — `legacySourceV6` (все поля выше),
  `legacyFold`, `legacySubMetaHistory`, формулы `legacyFold*Tag`;
- `core/state/migration_legacy_v4_proxy.go` — `legacyProxyV4`: форма
  `parser_config.proxies[]` схем v2–v4 (её раньше держала сборочная
  `configtypes.ProxySource`);
- `core/state/migration_raw_cache.go` — чтение и удаление `.raw`.

**Сборочная форма (`configtypes.ProxySource`).** Ушли `Outbounds`, `TagMask`,
`ExcludeFromGlobal`, `ExposeGroupTagsToGlobal`, `Fold`, вся detour-тройня,
`DisabledNodes`. Появились два **build-only** (`json:"-"`) деривата прохода 0/2:
`LocalGroups` (группы замены свёрнутой папки) и `Chain` (форма ядра для
эмиттера цепочек, собираемая из тела узла и резолвнутых позиций).

**Конвейер сборки.** `GenerateOutboundsFromParserConfig` больше не принимает
`loadNodesFunc` — «сборка полезла разбирать тела» стало невыразимо. Умерли
`resolveNodeDetours`, `migrateLegacyDetourNodeHash`, `sourceIDOfNode`,
`detourHopDisplayName`, хук `RecordParseFailures` в генераторе,
`subscription.LookupCachedBody`, `ApplySourceDetour`, `filterDisabledNodes`,
`migrateLegacyDisabledKeys`, `GCDisabledNodes`/`gcDisabledNodes`,
`disabledNodeTTL`, `singleNodeSourceTag`, `commentHasWizardLocalOutboundMarker`,
`canonicalMaskBridgeWarnings`, `state.syncLegacyDisabledMap`. Параметр `tagMask`
ушёл из `applyTagPrefixPostfix`; маска осталась только в
`ApplyLegacyTagMachine` — она читатель миграции.

**UI.** `PreviewNodes`/`PreviewNodesBySource`/`PreviewCacheGeneration`/
`PreviewIgnoredSectionsBySource`/`RebuildPreviewCache`/`InvalidatePreviewCache`
переименованы в `NodePool*` и пересажены с разбора тел на
`config.EmitCanonicalSource` — тот же код, что у сборки. `SourceNodeCounts`
считается прямо из `nodes[]` (ленивого разбора нет — данные в модели есть
всегда). Scope-ветки конфигуратора (local ↔ global) и
`localSubscriptionGroupTags` умерли. Пикер detour работает на одном `NodeLink`.

## 3. Новое (компенсация, не расширение)

- **`Node.Body` у цепочки** — SPEC 118 W5 закрыл дыру плана: настройки
  маршрута (`idle_timeout`/`strip_evasion`/`strip`/`rewrite`) в v7 не имели
  дома вовсе и терялись бы вместе с `Source.Chain`. Теперь они живут в теле
  узла — тот же дом, что у сервера, — а позиции остаются ссылками в `hops`.
  Конвертеры `configtypes.ChainBody` / `ChainFromBody`; features/sources.md и
  features/state.md §миграция шаг 4 поправлены той же правкой.
- **`config.MaterializeServerNode`** — ЕДИНСТВЕННАЯ точка «share-URI или JSON
  → тело узла»: её зовут миграция, добавление источника в UI, вкладка JSON и
  «Regen from raw». Второй реализацией они разъехались бы на первой правке
  эмиттера.
- **`core/backup/convert_v7.go`** — конвертеры границы v7 ↔ контракт 0.11
  (enabled ⇄ disabled-карта, replace ⇄ fold, NodeLink ⇄ тройня, hops ⇄
  строки, TagPolicy ⇄ tag-спека). Контракт и корпус не тронуты.
- **UI Т8-минимум, вытянутый сюда сносом:** вкладка JSON server-узла = прямой
  редактор `body` с «Regen from raw» (ошибка разбора → откат, узел не
  портится); JSON подписки — синхронный read-only рендер тел; Overview:
  блок raw-body заменён счётом узлов и per-node `origin.raw`.

## 4. Хвост ревью W1 — Export switch по Kind

`backup.Export` получил ветки `folder` и `auto`: контракт 0.11 их не знает
(секция `folders[]` — отдельный трек, SPEC 118 §2), и молчаливое выпадение
запрещено. Подпись изменилась на `(*Backup, []Warning, error)` — предупреждения
экспорта показываются пользователю тем же списком, что у импорта
(`settings_backup.go`), код `backup_source_kind_unsupported`.

## 5. Судьба тестов

### Удалены (категория «б» — умер предмет)

| Файл | Причина |
|---|---|
| `core/config/subscription/e2e_disabled_flow_test.go` | сквозной контракт disabled-карты отменён осознанно: отметка живёт полем `node.enabled`, карты нет |
| `core/config/subscription/disabled_nodes_test.go` | `filterDisabledNodes` + TTL/GC снесены |
| `core/config/subscription/disabled_migration_test.go` | миграция legacy-hex-ключей карты уехала в миграцию v6→v7 (покрыта `migration_scenarios_test.go` §4.B.2) |
| `core/config/subscription/detour_test.go` | `ApplySourceDetour` и `TagMask` снесены: detour — ребро модели, не парсера |
| `core/config/subscription/trusted_parse_test.go` | `trustedParse` жил ради чисток карты; достоверность разбора теперь решает fetch-сервис (`core/subscription_fetch_test.go`, 113-A) |
| `core/config/expose_exclude_test.go` | `ExcludeFromGlobal`/`ExposeGroupTagsToGlobal` снесены; правило пула — `canonical_emit_test.go` §4.E.6 |
| `core/config/detour_cascade_test.go`, `core/config/detour_node_ref_test.go` | резолв detour-тройни (`resolveNodeDetours`) снесён; каскад/кольца/fail-closed — `ApplyCanonicalNodeLinks` (`canonical_emit_test.go` §4.E.2) |
| `core/config/ira_state_migration_test.go` | миграция `detour_node_hash` снесена вместе с полем |
| `core/config/excluded_sources_test.go` | `excludedSources` генератора умер вместе с тройнёй |
| `core/state/raw_cache_test.go`, `core/rebuild_raw_cache_test.go` | raw-кэш снесён |
| `core/state/disabled_nodes_roundtrip_test.go` | карта не сериализуется — нечего roundtrip'ить |
| `core/state/detour_mapping_test.go`, `core/state/detour_node_ref_test.go` | проекция тройни в сборочную форму снесена |
| `core/state/config_json_roundtrip_test.go` | `Source.ConfigJSON` снесён (тело живёт в `Node.Body`) |
| `core/debugapi/disabled_nodes_api_test.go` | эндпоинт отдавал `disabled_nodes` из состояния; поля нет |
| `core/preview_nodes_test.go`, `core/refresh_meta_test.go` | `Meta.PreviewNodes` и мостовая Meta-история снесены |
| `core/build_report_generation_test.go` | строился на `state.WriteRawBody` |
| `ui/configurator/tabs/disabled_node_toggle_test.go` | тумблер писал карту; поведение тумблера — `source_edit_apply_test.go` |
| `ui/configurator/tabs/source_node_tag_buffer_test.go` | буферизовал `NodeTag`/`Fold`/`DisabledNodes` — все три поля снесены |
| `ui/configurator/business/detour_test.go` | пикер строился на тройне; новый — `detour_refs_test.go` на `NodeLink` |
| `ui/configurator/business/preview_cache_chain_test.go`, `preview_dedup_test.go`, `source_node_counts_test.go` | кэш превью разбирал тела; пул строится эмиссией, счётчики — из `nodes[]` |

### Переработаны (предмет жив, форма другая)

`core/config/canonical_emit_test.go` (снята пара тестов про упразднённую
маску — поля больше нет, warning невыразим), `chain_emit_test.go`,
`contract_direction_test.go`, `direction_twins_test.go`,
`group_node_contract_test.go`, `naive_degrade_test.go` (+ общий помощник
`seedCanonicalNodes`/`generateWithCanonicalNodes`), `manual_config_emit_test.go`,
`source_parse_failed_test.go` (причина «почему пусто» теперь эмиссионная),
`varsubst_test.go`, `uniquify_collision_test.go`,
`singbox_import_e2e_test.go` (тело подаётся httptest-стабом в base64 — хука
кэша больше нет), `core/state/*` (сайдкар-формы, `pending_disabled`),
`core/backup/*` (v7-формы + новая подпись Export),
`ui/configurator/business/detour_refs_test.go`, `detour_rename_e2e_test.go`,
`direction_rename_test.go`, `ui/configurator/presentation/*`,
`ui/configurator/tabs/source_edit_apply_test.go`, `source_chain_save_test.go`.

### Добавлено

`core/state/legacy_fixture_copy_test.go` — **обязательная** обвязка: с
`migrationPurgesLegacy=true` загрузка легаси-состояния переписывает файл на
месте, и чтение фикстуры по её месту в репозитории уничтожало бы её первым же
прогоном. Тесты работают с копией во временном каталоге.

## 6. Таблица grep-инвариантов SPEC §4.A

Счёт — по прод-коду (`--include=*.go`, `*_test.go` исключены), за вычетом
санкционированного исключения: файлы входа миграции v6→v7
(`core/state/migration_legacy_source.go`, `migration_legacy_v4_proxy.go`,
`migration_raw_cache.go`, `migration_v6_to_v7.go`, `connections.go`,
`source_fold_migrate.go`, `legacy_migration.go`, `migration_hooks.go`,
`load_v5.go`, `load_v6.go`, `disk_v6.go`) и конвертеры границы бэкапа
(`core/backup/**`).

| Инвариант | Проверка | Результат |
|---|---|---|
| raw-кэш снесён | файлов `core/state/raw_cache.go`, `core/rebuild_raw_cache.go` нет; `LookupCachedBody\|ReadRawBody\|WriteRawBody` | файлов нет; **0** |
| disabled-карта снесена | `DisabledNodes\|GCDisabledNodes\|filterDisabledNodes` | **0 кода** (3 упоминания в комментариях-объяснениях) |
| fold снесён | `SourceFold\|PrepareSourceFolds\|WIZARD:auto\|WIZARD:selector\|EffectiveTagPrefix` | **0 кода** (3 в комментариях `folder_replaces.go` «чем отличается от умершего») |
| exclude/expose снесены | `ExcludeFromGlobal\|ExposeGroupTagsToGlobal` | **0 кода** (1 в комментарии) |
| mask снесён | `TagMask` | **0 кода**; 1 вызов — `ApplyLegacyTagMachine` (читатель миграции), 1 комментарий |
| локальных Направлений нет | у v7 `Source` нет поля `Outbounds`; `Sources[i].Outbounds\|localSubscriptionGroupTags` | поля нет; **0 кода** (8 в комментариях) |
| detour-тройня снесена | `DetourNodeSourceID\|DetourNodeHash\|DetourNodeLabel` | **0** |
| PreviewNodes снесены | `PreviewNodes\|PreviewCacheGeneration` | **0** |
| defaults ушли | в state-типах нет `Defaults` у модели; `Connections.Defaults\|model.Defaults` | **0** (`state.Defaults` остался ТОЛЬКО как форма чтения легаси-схем, шаг 8 её и переносит) |
| body чист от detour | `ApplySourceDetour` | **0 кода** (2 комментария) |
| сборка не парсит подписки | сигнатура `GenerateOutboundsFromParserConfig` не принимает загрузчик | по построению |
| обёртки `connections` нет | `ConnectionsSection\|"connections"` в `core/state` | только миграция |

## 7. Приёмка

| Проверка | Результат |
|---|---|
| `go build ./...` | зелёный |
| `go vet ./...` | зелёный |
| `go test -count=1 ./...` | зелёный |
| `ETALON_V6MIG=1 go test ./core -run TestEtalonV6Mig` | **РОВНО одно** задекларированное расхождение Р2: `[P]auto` → `[P]select-auto` (пара тегов свёртки both стала `<tag>` + `<tag>-auto`). Других расхождений нет — в т.ч. цепочка сохранила `idle_timeout: 2m`, то есть переезд настроек маршрута в `body` прошёл без потерь |
| `go test ./core/build` | зелёный |
| греп go1.20 по диффу | чисто (`min`/`max`/`clear`/`slices.`/`maps.`/`PathValue`/`errors.Join` — 0) |

### Одно осознанное отступление в корпусе

`contract/corpus/direction/fold_select_auto` помечен `t.Skip` с явной
причиной (`corpusDivergence` в `contract_direction_test.go`): его ожидания
описывают УПРАЗДНЁННУЮ пару тегов `<PFX>select` + `<PFX>auto`, а модель v7
эмитит `<tag>` + `<tag>-auto` — то же расхождение Р2. Фикстура **не тронута**
(контракт и корпус в этой кампании не меняются, SPEC 118 §2); кейс вернётся,
когда контракт догонит модель (этап 4). Остальные 20+ кейсов корпуса зелёные.

## 8. Замечания для следующих волн

- **W6.** Т8 закрыт частично и вынужденно (JSON-вкладка, Overview, счётчики,
  пул хопов). Не сделано: поля умолчаний на вкладке Settings приложения
  (значения читаются из `bin/settings.json`, но UI-поля нет);
  предупреждение о протухании выбора `cache.db` при правке `replace.tag`;
  персист `MigrationReport` в файл. Мостовая Meta мертва — хвост ревью W3
  про «противоречивый успех» закрыт вместе с ней.
- **W7.** Конвертеры бэкапа написаны и покрыты корпусом, но контракт-роундтрип
  §4.F.2/§4.F.3 и remote-гейт по мажору — не делались. Импорт кладёт отметки
  в `pending_disabled` (вердикт O2) — хвост ревью W3 закрыт.
- **Открыто (капитану).** Тег замены при импорте бэкапа материализуется
  позиционным деривативом `<PFX>select` (`backupReplaceTag`): контракт 0.11
  тега не несёт, а в v7 он обязателен. Формула та же, что у миграции, — на
  ней держатся ссылки правил из того же файла.
