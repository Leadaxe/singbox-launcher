# SPEC 117 · Этап 1 — задачи

Правила: каждая волна завершается зелёным `go build ./...` и прогоном
затронутых пакетов (`go test ./core/state/... ./ui/configurator/...`);
финально — полный `go test ./... && go vet ./...`. GUI-пакеты — скрипты
`build/`. Git не трогать. `ui/traffic/**` не трогать. go1.20-гард: в новых
правках нет `min`/`max`/`clear`/`slices`/`maps`/`PathValue`/`errors.Join`.

## W1 — Фундамент: ревизия модели

- [x] `ui/configurator/models/wizard_model.go`: поле `Revision uint64
      `json:"-"`` + метод `BumpRevision()` с doc-комментарием
      (features/state.md «Ревизия модели»; UI-поток, без атомиков).
- [x] Вставить `BumpRevision()` рядом с каждым существующим вызовом
      `RefreshDerivedParserConfig` (Refresh пока не удалять):
      - [x] `ui/configurator/tabs/source_tab.go` (:101, :212, :240,
            `applySourceMutation` :806-826 — порядок с `MarkAsChanged`
            сохранить, риск Р10; :1017)
      - [x] `ui/configurator/tabs/source_edit_window.go` (:329, :663, :1577)
      - [x] `ui/configurator/business/sources.go` (:150-153)
      - [x] `ui/configurator/business/sources_json.go` (:145-148, :163-164)
      - [x] `ui/configurator/presentation/presenter_sync.go`
            (`RefreshAfterPresetToggle` :249-260)
      - [x] `ui/configurator/presentation/presenter_state_helpers.go`
            (`restoreParserConfig` :31-67)
      - [x] `ui/configurator/configurator.go` (сид шаблона :339-361)
      - [x] `ui/configurator/business/preview_cache.go` (ветка
            nil→Refresh :28-40, пока жива)
- [x] `ui/configurator/business/parser.go`: stale-guard `:170` — снапшот
      `rev := model.Revision` на старте, сравнение ревизий вместо строк;
      семантика выброса (`BuildReportGen = 0`) прежняя.
- [x] `ui/configurator/business/outbound.go`: мемо
      `AvailableOutboundsMemoKey string` → `AvailableOutboundsMemoRev uint64`
      (поле в `wizard_model.go:209-211`), сброс в `InvalidatePreviewCache`.
- [x] Тесты: переписать
      `ui/configurator/business/parser_stale_test.go` (:52, :87) на
      ревизию (мутация модели во время генерации → результат выброшен;
      без мутации → применён).
- [x] `go build ./...` + `go test ./ui/configurator/...` зелёные.

## W2 — Read-пути → canonical / одноразовая проекция

- [x] `ui/configurator/business/parser.go`: вход `ParseAndPreview` =
      `model.AsParserConfig()` (снести парс строки :52-83 и size-gate по
      строке); удалить запись `model.ParserConfig = &parserConfig` (:213).
- [x] `ui/configurator/business/outbound.go`: `GetAvailableOutbounds`
      (:69-140) и `AllDirectionTags` (:267-290) — теги из
      `model.GlobalOutbounds` + `model.Sources[i].Outbounds`; фоллбэк-парс
      строки удалить.
- [x] `ui/configurator/business/detour.go`: `localSubscriptionGroupTags`
      (:237-260) — из `model.Sources`; фоллбэк-парс удалить.
- [x] `ui/configurator/business/preview_cache.go`: `RebuildPreviewCache`
      (:28-150) — локальная проекция `AsParserConfig()`;
      `previewDirectionTags` (:151-165) — canonical;
      `applyMigratedDisabledKeys` (:191-203) не трогать (комментарий об
      индексном инварианте Р1 — к `AsParserConfig`).
- [x] Гейты пустоты → `len(model.Sources)`:
      - [x] `ui/configurator/business/create_config.go:77`
      - [x] `ui/configurator/presentation/presenter_async.go:45`
      - [x] `ui/configurator/presentation/presenter_save.go:89-113`
        (валидация «есть хоть один источник» без повторного парса)
- [x] `ui/configurator/business/direction_rename.go`: `DirectionTagTaken`
      (:44-63) — только canonical (приоритет legacy-вида снести).
- [x] `ui/configurator/tabs/source_tab.go:889-911`: счётчики источников
      из `len(m.Sources)` / `PreviewNodesBySource`.
- [x] `ui/configurator/tabs/source_chain_tab.go:716-729` +
      `source_chain_hops.go:86-115`: кандидаты позиций цепочки из
      canonical (`GlobalOutbounds`, `Sources[i].Outbounds`);
      `getParserConfigForChain` удалить.
- [x] Core-косметика:
      - [x] `main.go:259-272` — счётчики из `s.Connections`
      - [x] `ui/core_dashboard_tab.go:556-574` — то же
      - [x] `core/config_service_context.go:26-28` — мёртвый параметр
            `_ *config.ParserConfig` убрать; `core/rebuild.go:186-187` —
            мёртвый провоз убрать
      - [x] `core/state/state.go:78-84` — комментарий поля: read-only
            Load-проекция, писать запрещено
- [x] НЕ трогать: `business/loader.go` (строка шаблона),
      `loadParserConfigForUpdate`/`rebuild_raw_cache.go` (читают
      Load-проекцию свежезагруженного state — законно).
- [x] `go build ./...` + `go test ./ui/... ./core/...` зелёные.

## W3 — Write-пути и CRUD на canonical

- [x] **`ui/configurator/outbounds_configurator/`** — перевод пакета на
      `model.GlobalOutbounds` / `model.Sources[i].Outbounds`:
      - [x] `configurator_helpers.go`: удалить `getParserConfig`
            (:280-297); `collectRows` (:58-100) и `collectAllTags`
            (:231-244) — чтение canonical;
            `syncPresetOutboundsForModel` (:260-278) — одна запись в
            `GlobalOutbounds`.
      - [x] `configurator.go`: reorder (:70-86), edit+смена scope
            (:164-190, достройку пустых `ProxySource{}` :184-187 снести),
            delete (:211-217), `Updates`-патч (:240-247), toggle
            (:286-302), add (:337-350, снести достройку :346-349), seed
            required (:370-390) — на canonical; каждая мутация →
            `BumpRevision()`.
      - [x] `edit_dialog.go`: открытие (:57-59) без `getParserConfig`;
            превью селектора (:727-744) — одноразовая проекция
            `model.AsParserConfig().ParserConfig.Proxies` на месте вызова.
- [x] `ui/configurator/tabs/source_tab.go`: `CreateDirectionsTab`
      (:981-992) — снести ленивую материализацию `m.ParserConfig`;
      `onConfiguratorApply` (:994-1024) — снести обратное копирование
      (`GlobalOutbounds ←`, `Sources[i].Outbounds ←`), оставить
      `MarkAsChanged`+`BumpRevision`+refresh.
- [x] **`ui/configurator/tabs/source_edit_window.go`** — окно на
      `state.Source`:
      - [x] `cloneSource()` — явная deep-copy (`Fold`, `Outbounds`,
            `DisabledNodes`, `Meta`, слайсы; риск Р4, без `slices.`/`maps.`);
      - [x] `showSourceEditWindow` (:405): scratch = клон
            `m.Sources[i]` вместо `ToProxySourceV4()`;
      - [x] `setNodeEnabled` (:215-230) — правка `DisabledNodes` копии
            Source;
      - [x] Fold-вкладка: `syncFoldFormFromModel` (:554) и
            `foldTagPrefix` (`source_fold_tab.go:206`) — на `Source.Fold`
            / `Source.TagSpec`;
      - [x] `applyProxyEditToSource` (:232-313) удалить; Apply =
            `m.Sources[i] = clone` + `BumpRevision()`;
            `serializeParserAfterSourceEdit` (:315-336) упростить;
      - [x] chain-wiring (:476, :516) — на canonical из W2.
- [x] `ui/configurator/business/detour.go`: сигнатуры
      `DetourOptions`/`DetourOptionsWithNodes` (:47, :122) —
      `*configtypes.ProxySource` → `*corestate.Source`; поправить
      компиляцию тестов-потребителей (семантику не менять, риск Р7).
- [x] Дубли-правки:
      - [x] `business/direction_rename.go:120-130` — legacy-правки
            (`model.ParserConfig.Outbounds`, `Proxies[i].Outbounds`)
            удалить; остаются `GlobalOutbounds` + `Sources[i].Outbounds`.
      - [x] `presentation/presenter_sync.go:249-260` — одиночный
            `SyncOutboundsWithTemplate(GlobalOutbounds)`.
      - [x] `presentation/presenter_state.go:176-185` — одиночный
            `SyncOutboundsWithTemplate(state.Connections.Outbounds)`.
- [x] Тесты: `TestRenameDirectionTouchesBothViews`
      (`direction_rename_test.go:105`) → `TestRenameDirection_CanonicalOnly`;
      компиляционная правка `detour_test.go`,
      `configurator_helpers_test.go`.
- [x] `go build ./...` + `go test ./ui/...` зелёные.

## W4 — Снос обратных синков

- [x] Удалить `core/state/sync_to_connections.go` целиком + вызов в
      `core/state/save.go:36`.
- [x] `core/state/connections_helpers.go:12-79`: удалить хелперы, жившие
      только ради обратного синка (`serverLabelFromLegacy`,
      `extractURIFragment`, `sprintfServerN`, `serverConfigJSONKey`);
      `buildTagSpecFromLegacy` оставлен — его использует Load-миграция v4
      (`legacy_migration.go`), это прямое направление.
- [x] `ui/configurator/presentation/presenter_state.go:100-115`:
      `CreateStateFromModel` пишет только Connections — заполнение
      `state.ParserConfig` проекцией удалено (вместе с W3-страховкой
      выравнивания `state.ParserConfig.ParserConfig.Outbounds`).
- [x] `ui/configurator/presentation/presenter_sync.go:438-470`:
      `ApplyParserConfigFromCurrentJSON` удалён + вызов
      `configurator.go:643` (Refresh списка Направлений остался).
- [x] Тесты категории (а), core/state (фиксация в отчёте):
      - [x] `TestConfigJSON_LegacyRoundTrip`
            (`config_json_roundtrip_test.go:17`) — удалён (предмет —
            обратный синк — упразднён этапом);
      - [x] `TestSyncConnectionsFromLegacy_KeepsRefAndID`
            (`detour_node_ref_test.go:76`) — удалён (предмет — сам синк);
      - [x] `TestDetourTag_LegacyRoundTrip` (`detour_mapping_test.go:20`)
            — переработан: только прямая проекция;
      - [x] `TestLoad_V4Minimal` (`state_test.go:52,77`) — уже ассертит
            `s.Connections` + Load-проекцию (она остаётся на Load), без
            изменений;
      - [x] `TestSave_RoundTrip` (`state_test.go`) — переработан: мутирует
            `s.Connections`, ID задаётся при создании и не пересоздаётся.
- [x] Новые тесты (SPEC §5.B) — `core/state/canonical_roundtrip_test.go`
      + фикстура `testdata/v6_roundtrip.json`:
      - [x] roundtrip `Load→Save→Load→Save` байт-в-байт (modulo
            meta.updated_at — Save штампует время всегда) на фикстуре v6
            (подписка + server + config_json-server + chain + локальные
            Outbounds + DisabledNodes + Fold + Meta + rules/vars/dns);
      - [x] ID-стабильность: ULID неизменны через 4 цикла мутаций
            canonical (URL/label/toggle/chain/reorder) + Save/Load; новые
            не выдаются. Аудит создателей Source (Р3): UI-создатели
            (`sources.go`, `sources_json.go`, `source_tab.go` add chain)
            минтят ULID при создании — закреплено
            `business/source_creator_ulid_test.go`; импорт бэкапа минтил
            через снесённый синк → добавлен `ensureSourceID` в
            `core/backup/import.go` + `import_ulid_test.go`.
- [x] `go build ./...` + `go test ./core/state/... ./ui/...` зелёные
      (полный `go test ./...` + `go vet ./...` — тоже).

## W5 — Чистка мёртвого транспорта и полей

- [x] `UpdateParserConfig(text)`: удалить из
      `business/ui_updater.go:23` (интерфейс),
      `presentation/presenter_ui_updater.go:23-40` (полезный остаток —
      `RefreshOutboundsConfiguratorList` — вызывать напрямую где нужно);
      убрать все ~15 вызовов (`source_tab.go:101,212,240,825,1017`;
      `source_edit_window.go:329,663,1577`; `sources.go:153`;
      `sources_json.go:148,164`; `presenter_sync.go:259`).
- [x] `models/wizard_model.go`: удалить поля `ParserConfig`,
      `ParserConfigJSON`, метод `RefreshDerivedParserConfig` (компилятор
      выдаст пропущенные call-site'ы — каждому уже должен соответствовать
      `BumpRevision` из W1; риск Р2); `AsParserConfig` остаётся с
      комментарием об индексном инварианте 1:1 (Р1).
- [x] `presentation/presenter_target.go:246-256`:
      `invalidateParsedNodes` — вместо `model.ParserConfig = nil` —
      `BumpRevision` + инвалидция превью.
- [x] `configurator.go:339-361`: сид из шаблона — парс строки шаблона
      сразу в canonical (`GlobalOutbounds`/`Defaults`), без
      `model.ParserConfigJSON`.
- [x] `presentation/gui_state.go:91-93,118`: остатки «last valid
      ParserConfig JSON» удалить.
- [x] `business/validator.go:220-236`: `ValidateParserConfigJSON` +
      `TestValidateParserConfigJSON` (`validator_test.go:417`) удалить
      (прод-вызовов нет).
- [x] Прогнать grep-инварианты SPEC §5.A — все по нулям.
- [x] `go build ./...` + `go test ./...` зелёные.

## W6 — Приёмка

- [x] Поведенческие тесты SPEC §5.C (business/presentation-уровень, без
      ассертов на форматирование строк — правило no-ui-format-tests):
      - [x] C1 CRUD Направления сквозь Save/Load —
            `presentation/canonical_crud_test.go`;
      - [x] C2 rename canonical-only — закрыт W3
            (`TestRenameDirection_CanonicalOnly`);
      - [x] C3 окно источника: Apply → `m.Sources[i]`; Cancel — без
            следов в модели (deep-copy, Р4) — закрыт W3
            (`tabs/source_node_tag_buffer_test.go`:
            TestNodeTagEditIsBufferedUntilSave / …ReachesModelOnSave /
            TestCloneSourceIsDeeplyIndependent / …ChainIndependent +
            `disabled_node_toggle_test.go`);
      - [x] C4 preset toggle — одна запись, идемпотентность —
            `presentation/preset_toggle_canonical_test.go`;
      - [x] C5 stale-guard по ревизии — закрыт W1
            (`business/parser_stale_test.go`);
      - [x] C6 гейты пустой модели —
            `business/empty_model_gates_test.go`;
      - [x] C7 update-конвейер от свежезагруженного state —
            Load-проекция в `presentation/canonical_crud_test.go`
            (+ `core/state` TestSave_RoundTrip).
- [x] Полный прогон: `go build ./...`, `go test ./...`, `go vet ./...`;
      GUI-пакеты — `build/`-скрипты.
- [x] Греп go1.20 по диффу:
      `git diff --name-only | xargs grep -nE "\b(min|max|clear)\(|slices\.|maps\.|PathValue|errors\.Join"`
      (только чтение диффа, без git-мутаций) → 0 в новых правках.
- [x] `ui/traffic/**` не затронут волнами SPEC 117 (`git status` — только
      чтение; в рабочей копии есть незакоммиченные правки `ui/traffic/*`
      ПАРАЛЛЕЛЬНОЙ сессии — ExportSnapshot тулбара, к этапу не относятся).
- [x] IMPLEMENTATION_REPORT.md: изменённые файлы, снесённые тесты с
      причинами, изменённые сигнатуры, таблица grep-инвариантов,
      результаты roundtrip.
