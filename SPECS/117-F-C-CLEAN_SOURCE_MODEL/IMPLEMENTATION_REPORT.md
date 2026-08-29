# SPEC 117 · Этап 1 — отчёт о реализации (W1–W6)

Ветка develop. W1–W4 закоммичены (`f15706e`, `c814e37`, `5feaf65`);
W5–W6 — в рабочей копии (git-операции исполнителю запрещены).

## 1. Изменённые файлы по волнам

### W1+W2 — ревизия модели + read-пути на canonical (commit `f15706e`)

- `ui/configurator/models/wizard_model.go` — поле `Revision uint64` +
  `BumpRevision()`; мемо `AvailableOutboundsMemoKey string` →
  `AvailableOutboundsMemoRev uint64`.
- `ui/configurator/business/parser.go` — вход `AsParserConfig()` вместо
  `json.Unmarshal(ParserConfigJSON)`; stale-guard по ревизии; запись
  `model.ParserConfig = &parserConfig` снята.
- `ui/configurator/business/outbound.go` — теги из
  `GlobalOutbounds`+`Sources[i].Outbounds`; мемо по ревизии.
- `ui/configurator/business/detour.go`, `direction_rename.go`,
  `preview_cache.go`, `create_config.go` — чтения canonical, фоллбэки
  «nil → распарси строку» сняты.
- `ui/configurator/presentation/presenter_async.go`, `presenter_save.go` —
  гейты по `len(model.Sources)`.
- `ui/configurator/tabs/source_chain_tab.go`, `source_chain_hops.go` —
  кандидаты позиций из canonical; `getParserConfigForChain` удалён.
- `main.go`, `ui/core_dashboard_tab.go` — счётчики из `s.Connections`.
- `core/config_service_context.go`, `core/rebuild.go` — мёртвый параметр
  `_ *config.ParserConfig` и его провоз сняты.
- `core/state/state.go` — комментарий: `State.ParserConfig` = read-only
  Load-проекция.
- Точки бампа (8 файлов): `source_tab.go`, `source_edit_window.go`,
  `sources.go`, `sources_json.go`, `presenter_sync.go`,
  `presenter_state_helpers.go`, `configurator.go`, `preview_cache.go`.

### W3 — write-пути и CRUD на canonical (commit `c814e37`)

- `ui/configurator/outbounds_configurator/{configurator,configurator_helpers,edit_dialog}.go`
  — пакет работает на `model.GlobalOutbounds` / `model.Sources[i].Outbounds`;
  `getParserConfig` удалён; достройка пустых `ProxySource{}` снята; каждая
  мутация завершается `BumpRevision()`; превью селектора — одноразовая
  проекция `AsParserConfig()`.
- `ui/configurator/tabs/source_edit_window.go` — окно на deep-copy
  `state.Source` (`cloneSource`); `applyProxyEditToSource` заменён
  присваиванием `m.Sources[i] = *edited` (`applySourceEditToModel`).
- `ui/configurator/tabs/source_fold_tab.go` — Fold-вкладка на `Source.Fold`.
- `ui/configurator/business/detour.go` — сигнатуры `DetourOptions*` на
  `*corestate.Source`.
- `ui/configurator/business/direction_rename.go` — legacy-половины
  переименования удалены (остались `GlobalOutbounds` + `Sources[i].Outbounds`).
- `ui/configurator/presentation/presenter_sync.go`, `presenter_state.go` —
  одиночные `SyncOutboundsWithTemplate`.
- `ui/configurator/tabs/source_tab.go` — `onConfiguratorApply` без обратного
  копирования; `CreateDirectionsTab` без материализации `m.ParserConfig`.

### W4 — снос обратных синков (commit `5feaf65`)

- `core/state/sync_to_connections.go` — УДАЛЁН целиком (+ вызов в `save.go`).
- `core/state/connections_helpers.go` — хелперы обратного направления удалены
  (`buildTagSpecFromLegacy` оставлен — его использует Load-миграция v4).
- `ui/configurator/presentation/presenter_state.go` — `CreateStateFromModel`
  пишет только Connections.
- `ui/configurator/presentation/presenter_sync.go` —
  `ApplyParserConfigFromCurrentJSON` удалён (+ вызов в `configurator.go`).
- `core/backup/import.go` — `ensureSourceID`: импорт бэкапа минтит ULID сам
  (раньше это делал снесённый синк).
- Новые тесты: `core/state/canonical_roundtrip_test.go` +
  `testdata/v6_roundtrip.json`; `business/source_creator_ulid_test.go`;
  `core/backup/import_ulid_test.go`.

### W5 — чистка мёртвого транспорта и полей (рабочая копия)

- `ui/configurator/models/wizard_model.go` — удалены поля
  `WizardModel.ParserConfig`, `WizardModel.ParserConfigJSON` и метод
  `RefreshDerivedParserConfig`; doc-комментарии модели переписаны.
- `ui/configurator/business/ui_updater.go` — из интерфейса `UIUpdater`
  удалён `UpdateParserConfig(text string)`; добавлен полезный остаток
  `RefreshOutboundsConfiguratorList()`.
- `ui/configurator/presentation/presenter_ui_updater.go` —
  `UpdateParserConfig(text)` → `RefreshOutboundsConfiguratorList()`
  (тело и раньше только пересобирало список конфигуратора).
- Call-site'ы транспорта (11 вызовов `UpdateParserConfig` + 11 вызовов
  `RefreshDerivedParserConfig`, живых на момент W5; каждому соответствует
  `BumpRevision` из W1 — сверено компилятором после удаления Refresh, риск Р2):
  `source_tab.go` (applyAddedSources, addServerAction, addChainAction,
  applySourceMutation, onConfiguratorApply),
  `source_edit_window.go` (applySourceEditToModel, resetRefsAfterNodeRename),
  `sources.go` (AppendURLsToSources), `sources_json.go`
  (AppendManualConfigJSON, RelabelLastSources), `presenter_sync.go`
  (RefreshAfterPresetToggle — вызов снят совсем: шаг 3 той же функции уже
  обновляет список), `presenter_state.go`/`presenter_state_helpers.go`
  (только снятие `RefreshDerivedParserConfig` рядом с бампом).
- `ui/configurator/presentation/presenter_target.go` —
  `invalidateParsedNodes`: `model.ParserConfig = nil` → `BumpRevision()`.
- `ui/configurator/configurator.go` — сид шаблона парсится сразу в canonical
  `GlobalOutbounds`; присваивание `model.ParserConfigJSON` снято.
- `ui/configurator/presentation/gui_state.go` — висячий комментарий
  «Last valid ParserConfig JSON» удалён; комментарий у
  `RefreshOutboundsConfiguratorList` актуализирован.
- `ui/configurator/business/validator.go` — `ValidateParserConfigJSON`
  удалена (прод-вызовов не было).
- `ui/configurator/business/outbound.go` — устаревший doc-header поправлен.
- Тесты, переставшие компилироваться (правка механическая, семантика не
  менялась): `parser_stale_test.go` (стаб интерфейса),
  `preview_dedup_test.go`, `detour_rename_e2e_test.go` (проекция
  `AsParserConfig()` на входе генератора), `preview_cache_chain_test.go`,
  `preview_target_test.go`, `wizard_integration_test.go`,
  `final_build_report_test.go` (мёртвые присваивания `ParserConfigJSON`).

### W6 — приёмка (рабочая копия)

- `ui/configurator/presentation/canonical_crud_test.go` — C1 + C7.
- `ui/configurator/presentation/preset_toggle_canonical_test.go` — C4.
- `ui/configurator/business/empty_model_gates_test.go` — C6.
- `SPECS/117-F-N-CLEAN_SOURCE_MODEL/SPEC.md` — уточнена формулировка
  инварианта «Scratch снесён» (легальные одноразовые проекции Т2, см. §4).
- `SPECS/117-F-N-CLEAN_SOURCE_MODEL/TASKS.md` — чекбоксы W5/W6.

## 2. Снесённые тесты и их причины

| Тест | Судьба | Причина |
|---|---|---|
| `TestConfigJSON_LegacyRoundTrip` (config_json_roundtrip_test.go) | удалён (W4) | предмет — обратный синк на Save — упразднён этапом; замена: `TestCanonical_LoadSaveLoadSave_ByteIdentical` + `TestCanonical_IDStability`; прокидка ConfigJSON закрыта `TestConfigJSON_ToProxySourceV4` |
| `TestSyncConnectionsFromLegacy_KeepsRefAndID` (detour_node_ref_test.go) | удалён (W4) | предмет — сам синк; инвариант ID закрывает `TestCanonical_IDStability` |
| `TestDetourTag_LegacyRoundTrip` (detour_mapping_test.go) | переработан (W4) | осталась только прямая проекция canonical→`ToProxySourceV4` |
| `TestLoad_V4Minimal` (state_test.go) | без изменений | ассертит `s.Connections` + Load-проекцию (она законно остаётся на Load) |
| `TestSave_RoundTrip` (state_test.go) | переработан (W4) | мутирует `s.Connections`; ID задаётся при создании |
| `TestRenameDirectionTouchesBothViews` (direction_rename_test.go) | заменён (W3) | → `TestRenameDirection_CanonicalOnly` (C2): четвёртой копии не существует |
| `TestParseAndPreview_Discards/AppliesWhenJSONChanges…` (parser_stale_test.go) | переписаны (W1) | → `…WhenModelMutatesDuringGeneration` / `…WhenModelUnchangedDuringGeneration` — stale-guard по ревизии (C5) |
| `TestValidateParserConfigJSON` (validator_test.go) | удалён (W5) | удалён вместе с функцией — прод-вызовов нет (JSON-редактор снесён SPEC 104) |
| `TestDetourPersist…` (tabs/detour_persist_test.go) | удалён (W3) | предмет — scratch-паттерн `applyProxyEditToSource`; замена — буферные тесты `source_node_tag_buffer_test.go` (C3) |

## 3. Изменённые сигнатуры

| Было | Стало | Волна |
|---|---|---|
| `UIUpdater.UpdateParserConfig(text string)` | `UIUpdater.RefreshOutboundsConfiguratorList()` | W5 |
| `(*WizardPresenter).UpdateParserConfig(text string)` | `(*WizardPresenter).RefreshOutboundsConfiguratorList()` | W5 |
| `WizardModel.ParserConfig *config.ParserConfig`, `ParserConfigJSON string`, `RefreshDerivedParserConfig()` | удалены; проекция — только `AsParserConfig()` | W5 |
| `WizardModel.AvailableOutboundsMemoKey string` | `AvailableOutboundsMemoRev uint64` (+`AvailableOutboundsMemoTags` — сам кэш, живёт под ключом-ревизией) | W1 |
| `DetourOptions(model, source *configtypes.ProxySource, …)` | `DetourOptions(model, source *corestate.Source, …)` | W3 |
| `DetourOptionsWithNodes(model, source *configtypes.ProxySource, …)` | `…(model, source *corestate.Source, …)` | W3 |
| `buildContextFromState(s, cache, td, _ *config.ParserConfig)` | `buildContextFromState(s, cache, td)` | W2 |
| `ValidateParserConfigJSON(jsonText string) error` | удалена | W5 |
| `NewConfiguratorContent(parent, editPresenter, onApply)` | сигнатура не менялась (внутренний `getParserConfig` удалён) | W3 |

## 4. Grep-инварианты SPEC §5.A (прогон 2026-08-29, прод-код, `*_test.go` исключены)

| Инвариант | Результат |
|---|---|
| `core/state/sync_to_connections.go` не существует | ✅ файла нет |
| `grep -rn "syncConnectionsFromLegacy" core ui` | ✅ 0 |
| `core/state/save.go` не читает `s.ParserConfig` | ✅ 0 обращений в коде (1 хит — комментарий, объясняющий что Save её НЕ читает) |
| `grep -rn "ParserConfigJSON\|RefreshDerivedParserConfig" ui core` | ✅ 0 в коде (5 хитов — исторические комментарии: `configurator.go:351`, `outbound.go` — поправлен, `parser.go:57`, `core/config_service_context.go:25`, `core/build/build.go:199` — все описывают снесённое) |
| `grep -rn "UpdateParserConfig(" ui` | ✅ 0 в коде (2 хита — комментарии «остаток снесённого транспорта») |
| `grep -rn "ApplyParserConfigFromCurrentJSON\|ValidateParserConfigJSON" ui` | ✅ 0 |
| `grep -rnE "model\.ParserConfig\b\|m\.ParserConfig\b" ui` | ✅ 0 в коде (2 хита — комментарии «проекция не трогается / снесён») |
| `outbounds_configurator` без `ProxySource`, `getParserConfig` нет | ✅ 0 в коде (2 хита — комментарии «достройки больше нет»); `getParserConfig` → 0 |
| `grep -rn "ToProxySourceV4" ui` | ✅ 2 вызова — обе одноразовые проекции Т2 «на входе parse/generate» (SPEC §5.A уточнён правкой этой волны): `models/wizard_model.go:275` (`AsParserConfig`) и `tabs/source_edit_window.go:1350` (`refreshServerJSONTab`, превью JSON — проекция выбрасывается сразу после вызова). Мутирующих модель вхождений 0. `core/state/adapter_source.go` — прямая проекция, остаётся |
| `parser.go` без строкового stale-сравнения | ✅ guard — `model.Revision != revAtStart`; `TrimSpace`-хиты — разбор входных строк, не отпечатки |

## 5. Roundtrip и стабильность идентичности (SPEC §5.B)

- `TestCanonical_LoadSaveLoadSave_ByteIdentical` (`core/state/canonical_roundtrip_test.go`):
  на фикстуре `testdata/v6_roundtrip.json` (подписка + server-URI +
  config_json-server + chain + локальные Outbounds + DisabledNodes + Fold +
  Meta + rules/vars/dns) `Load→Save(p1)→Load(p1)→Save(p2)`: `p1 == p2`
  побайтно; `p1` отличается от фикстуры только `meta.updated_at` — ✅.
- `TestCanonical_IDStability`: ULID всех Source неизменны через 4 цикла
  canonical-мутаций (URL/label/toggle/chain/reorder) + Save/Load; новых ULID
  не выдаётся — ✅.
- Создатели Source минтят ULID при создании:
  `business/source_creator_ulid_test.go` (UI-пути),
  `core/backup/import_ulid_test.go` (импорт бэкапа, `ensureSourceID`) — ✅.
- Meta/Label/Update/PreviewNodes переживают Save тривиально — пересборщика
  больше не существует.

## 6. Поведенческие сценарии C1–C7 (SPEC §5.C)

| Сценарий | Тест | Волна |
|---|---|---|
| C1 CRUD Направлений сквозь Save/Load | `presentation/canonical_crud_test.go` `TestDirectionsCRUD_CanonicalThroughSaveLoad` | W6 |
| C2 rename canonical-only | `business/direction_rename_test.go` `TestRenameDirection_CanonicalOnly` | W3 |
| C3 окно источника (Apply → `m.Sources[i]`, Cancel без следов) | `tabs/source_node_tag_buffer_test.go` (4 теста, включая deep-copy Р4) + `tabs/disabled_node_toggle_test.go` | W3 |
| C4 preset toggle — одна запись, идемпотентность | `presentation/preset_toggle_canonical_test.go` `TestPresetToggle_SingleCanonicalWriteAndIdempotent` | W6 |
| C5 stale-guard по ревизии | `business/parser_stale_test.go` (2 теста) | W1 |
| C6 гейты пустой модели | `business/empty_model_gates_test.go` (2 теста) | W6 |
| C7 update-конвейер от свежезагруженного state | Load-проекция в `TestDirectionsCRUD_CanonicalThroughSaveLoad` + `core/state` `TestSave_RoundTrip` | W6/W4 |

Примечание к C1: сами CRUD-операции конфигуратора живут в замыканиях Fyne
(`outbounds_configurator/configurator.go`) и headless не вызываются; тест
исполняет ровно те же canonical-мутации, что и замыкания, и фиксирует их
контракт — немедленное отражение в модели без Apply-шага, рост ревизии,
`CreateStateFromModel → Save → Load` возвращает ровно этот состав.
Примечание к C6: гейты presentation (`TriggerParseForPreview`,
`validateSaveInput`) построены на том же выражении `len(model.Sources) == 0`,
но требуют живых виджетов/диалогов — покрыты business-гейтами.
Без ассертов на форматирование строк (правило no-ui-format-tests).

## 7. Сборка и тесты (финальный прогон, 2026-08-29)

- `go build ./...` — чисто (ld-warning `duplicate libraries '-lobjc'` —
  системный, преждесуществующий).
- `go vet ./...` — чисто.
- `go test -count=1 ./...` — все 37 пакетов ok, 0 FAIL.
- Греп go1.20 по файлам диффа (`\b(min|max|clear)\(|slices\.|maps\.|PathValue|errors\.Join`)
  — 0 в коде (единственный хит — комментарий «без slices./maps.» в
  `source_edit_window.go:229`).
- Известный преждесуществующий флейк LAN-теста: в этом прогоне не
  воспроизвёлся — пакеты `ui` и `core/netiface` прогнаны дважды
  (`go test -count=1`), оба раза зелёные.

## 8. Отклонения и примечания

- `AvailableOutboundsMemoTags` оставлен (SPEC Т8 упоминает
  «AvailableOutboundsMemoKey/Tags»): ключ-отпечаток (`MemoKey`) заменён
  ревизией (`MemoRev`, W1 по PLAN §3), а `MemoTags` — это само
  мемоизированное значение; его удаление уничтожило бы мемо целиком,
  чего Т6 не требует.
- В `RefreshAfterPresetToggle` вызов `UpdateParserConfig` удалён без замены:
  шаг 3 той же функции уже пересобирает список конфигуратора через
  `UpdateUI` — двойной refresh был дубликатом.
- Сид шаблона (`configurator.go::loadConfigFromFile`) сохраняет прежний
  двойной парс (loader сериализует, сид парсит обратно) — минимальная правка:
  снято только промежуточное поле модели; упрощение сигнатуры
  `LoadConfigFromFile` — вне границ этапа.
- В рабочей копии есть незакоммиченные правки `ui/traffic/{live_view,toolbar,window}.go`
  ПАРАЛЛЕЛЬНОЙ сессии (ExportSnapshot тулбара) — волны SPEC 117 их не
  касались (`ui/traffic/**` не трогать — соблюдено).
- SPEC §5.A «Scratch снесён» уточнён правкой W6 (легальные одноразовые
  проекции Т2) — правка SPEC вместе с волной, по поручению.
