# SPEC 117 · Этап 1 — задачи

Правила: каждая волна завершается зелёным `go build ./...` и прогоном
затронутых пакетов (`go test ./core/state/... ./ui/configurator/...`);
финально — полный `go test ./... && go vet ./...`. GUI-пакеты — скрипты
`build/`. Git не трогать. `ui/traffic/**` не трогать. go1.20-гард: в новых
правках нет `min`/`max`/`clear`/`slices`/`maps`/`PathValue`/`errors.Join`.

## W1 — Фундамент: ревизия модели

- [ ] `ui/configurator/models/wizard_model.go`: поле `Revision uint64
      `json:"-"`` + метод `BumpRevision()` с doc-комментарием
      (features/state.md «Ревизия модели»; UI-поток, без атомиков).
- [ ] Вставить `BumpRevision()` рядом с каждым существующим вызовом
      `RefreshDerivedParserConfig` (Refresh пока не удалять):
      - [ ] `ui/configurator/tabs/source_tab.go` (:101, :212, :240,
            `applySourceMutation` :806-826 — порядок с `MarkAsChanged`
            сохранить, риск Р10; :1017)
      - [ ] `ui/configurator/tabs/source_edit_window.go` (:329, :663, :1577)
      - [ ] `ui/configurator/business/sources.go` (:150-153)
      - [ ] `ui/configurator/business/sources_json.go` (:145-148, :163-164)
      - [ ] `ui/configurator/presentation/presenter_sync.go`
            (`RefreshAfterPresetToggle` :249-260)
      - [ ] `ui/configurator/presentation/presenter_state_helpers.go`
            (`restoreParserConfig` :31-67)
      - [ ] `ui/configurator/configurator.go` (сид шаблона :339-361)
      - [ ] `ui/configurator/business/preview_cache.go` (ветка
            nil→Refresh :28-40, пока жива)
- [ ] `ui/configurator/business/parser.go`: stale-guard `:170` — снапшот
      `rev := model.Revision` на старте, сравнение ревизий вместо строк;
      семантика выброса (`BuildReportGen = 0`) прежняя.
- [ ] `ui/configurator/business/outbound.go`: мемо
      `AvailableOutboundsMemoKey string` → `AvailableOutboundsMemoRev uint64`
      (поле в `wizard_model.go:209-211`), сброс в `InvalidatePreviewCache`.
- [ ] Тесты: переписать
      `ui/configurator/business/parser_stale_test.go` (:52, :87) на
      ревизию (мутация модели во время генерации → результат выброшен;
      без мутации → применён).
- [ ] `go build ./...` + `go test ./ui/configurator/...` зелёные.

## W2 — Read-пути → canonical / одноразовая проекция

- [ ] `ui/configurator/business/parser.go`: вход `ParseAndPreview` =
      `model.AsParserConfig()` (снести парс строки :52-83 и size-gate по
      строке); удалить запись `model.ParserConfig = &parserConfig` (:213).
- [ ] `ui/configurator/business/outbound.go`: `GetAvailableOutbounds`
      (:69-140) и `AllDirectionTags` (:267-290) — теги из
      `model.GlobalOutbounds` + `model.Sources[i].Outbounds`; фоллбэк-парс
      строки удалить.
- [ ] `ui/configurator/business/detour.go`: `localSubscriptionGroupTags`
      (:237-260) — из `model.Sources`; фоллбэк-парс удалить.
- [ ] `ui/configurator/business/preview_cache.go`: `RebuildPreviewCache`
      (:28-150) — локальная проекция `AsParserConfig()`;
      `previewDirectionTags` (:151-165) — canonical;
      `applyMigratedDisabledKeys` (:191-203) не трогать (комментарий об
      индексном инварианте Р1 — к `AsParserConfig`).
- [ ] Гейты пустоты → `len(model.Sources)`:
      - [ ] `ui/configurator/business/create_config.go:77`
      - [ ] `ui/configurator/presentation/presenter_async.go:45`
      - [ ] `ui/configurator/presentation/presenter_save.go:89-113`
        (валидация «есть хоть один источник» без повторного парса)
- [ ] `ui/configurator/business/direction_rename.go`: `DirectionTagTaken`
      (:44-63) — только canonical (приоритет legacy-вида снести).
- [ ] `ui/configurator/tabs/source_tab.go:889-911`: счётчики источников
      из `len(m.Sources)` / `PreviewNodesBySource`.
- [ ] `ui/configurator/tabs/source_chain_tab.go:716-729` +
      `source_chain_hops.go:86-115`: кандидаты позиций цепочки из
      canonical (`GlobalOutbounds`, `Sources[i].Outbounds`);
      `getParserConfigForChain` удалить.
- [ ] Core-косметика:
      - [ ] `main.go:259-272` — счётчики из `s.Connections`
      - [ ] `ui/core_dashboard_tab.go:556-574` — то же
      - [ ] `core/config_service_context.go:26-28` — мёртвый параметр
            `_ *config.ParserConfig` убрать; `core/rebuild.go:186-187` —
            мёртвый провоз убрать
      - [ ] `core/state/state.go:78-84` — комментарий поля: read-only
            Load-проекция, писать запрещено
- [ ] НЕ трогать: `business/loader.go` (строка шаблона),
      `loadParserConfigForUpdate`/`rebuild_raw_cache.go` (читают
      Load-проекцию свежезагруженного state — законно).
- [ ] `go build ./...` + `go test ./ui/... ./core/...` зелёные.

## W3 — Write-пути и CRUD на canonical

- [ ] **`ui/configurator/outbounds_configurator/`** — перевод пакета на
      `model.GlobalOutbounds` / `model.Sources[i].Outbounds`:
      - [ ] `configurator_helpers.go`: удалить `getParserConfig`
            (:280-297); `collectRows` (:58-100) и `collectAllTags`
            (:231-244) — чтение canonical;
            `syncPresetOutboundsForModel` (:260-278) — одна запись в
            `GlobalOutbounds`.
      - [ ] `configurator.go`: reorder (:70-86), edit+смена scope
            (:164-190, достройку пустых `ProxySource{}` :184-187 снести),
            delete (:211-217), `Updates`-патч (:240-247), toggle
            (:286-302), add (:337-350, снести достройку :346-349), seed
            required (:370-390) — на canonical; каждая мутация →
            `BumpRevision()`.
      - [ ] `edit_dialog.go`: открытие (:57-59) без `getParserConfig`;
            превью селектора (:727-744) — одноразовая проекция
            `model.AsParserConfig().ParserConfig.Proxies` на месте вызова.
- [ ] `ui/configurator/tabs/source_tab.go`: `CreateDirectionsTab`
      (:981-992) — снести ленивую материализацию `m.ParserConfig`;
      `onConfiguratorApply` (:994-1024) — снести обратное копирование
      (`GlobalOutbounds ←`, `Sources[i].Outbounds ←`), оставить
      `MarkAsChanged`+`BumpRevision`+refresh.
- [ ] **`ui/configurator/tabs/source_edit_window.go`** — окно на
      `state.Source`:
      - [ ] `cloneSource()` — явная deep-copy (`Fold`, `Outbounds`,
            `DisabledNodes`, `Meta`, слайсы; риск Р4, без `slices.`/`maps.`);
      - [ ] `showSourceEditWindow` (:405): scratch = клон
            `m.Sources[i]` вместо `ToProxySourceV4()`;
      - [ ] `setNodeEnabled` (:215-230) — правка `DisabledNodes` копии
            Source;
      - [ ] Fold-вкладка: `syncFoldFormFromModel` (:554) и
            `foldTagPrefix` (`source_fold_tab.go:206`) — на `Source.Fold`
            / `Source.TagSpec`;
      - [ ] `applyProxyEditToSource` (:232-313) удалить; Apply =
            `m.Sources[i] = clone` + `BumpRevision()`;
            `serializeParserAfterSourceEdit` (:315-336) упростить;
      - [ ] chain-wiring (:476, :516) — на canonical из W2.
- [ ] `ui/configurator/business/detour.go`: сигнатуры
      `DetourOptions`/`DetourOptionsWithNodes` (:47, :122) —
      `*configtypes.ProxySource` → `*corestate.Source`; поправить
      компиляцию тестов-потребителей (семантику не менять, риск Р7).
- [ ] Дубли-правки:
      - [ ] `business/direction_rename.go:120-130` — legacy-правки
            (`model.ParserConfig.Outbounds`, `Proxies[i].Outbounds`)
            удалить; остаются `GlobalOutbounds` + `Sources[i].Outbounds`.
      - [ ] `presentation/presenter_sync.go:249-260` — одиночный
            `SyncOutboundsWithTemplate(GlobalOutbounds)`.
      - [ ] `presentation/presenter_state.go:176-185` — одиночный
            `SyncOutboundsWithTemplate(state.Connections.Outbounds)`.
- [ ] Тесты: `TestRenameDirectionTouchesBothViews`
      (`direction_rename_test.go:105`) → `TestRenameDirection_CanonicalOnly`;
      компиляционная правка `detour_test.go`,
      `configurator_helpers_test.go`.
- [ ] `go build ./...` + `go test ./ui/...` зелёные.

## W4 — Снос обратных синков

- [ ] Удалить `core/state/sync_to_connections.go` целиком + вызов в
      `core/state/save.go:36`.
- [ ] `core/state/connections_helpers.go:12-79`: удалить хелперы, жившие
      только ради обратного синка (`buildTagSpecFromLegacy` и т.п.);
      прямые (canonical→legacy) оставить.
- [ ] `ui/configurator/presentation/presenter_state.go:100-115`:
      `CreateStateFromModel` пишет только Connections — заполнение
      `state.ParserConfig` проекцией удалить.
- [ ] `ui/configurator/presentation/presenter_sync.go:438-470`:
      `ApplyParserConfigFromCurrentJSON` удалить + вызов
      `configurator.go:643`.
- [ ] Тесты категории (а), core/state (фиксация в отчёте):
      - [ ] `TestConfigJSON_LegacyRoundTrip`
            (`config_json_roundtrip_test.go:17`) — удалить;
      - [ ] `TestSyncConnectionsFromLegacy_KeepsRefAndID`
            (`detour_node_ref_test.go:76`) — удалить;
      - [ ] `TestDetourTag_LegacyRoundTrip` (`detour_mapping_test.go:20`)
            — переработать: только прямая проекция;
      - [ ] `TestLoad_V4Minimal` (`state_test.go:52,77`) — ассерты на
            `s.Connections` (+ допустимо на Load-проекцию);
      - [ ] `TestSaveLoadRoundTrip` (`state_test.go:148`) — мутирует
            `s.Connections`.
- [ ] Новые тесты (SPEC §5.B):
      - [ ] roundtrip `Load→Save→Load→Save` байт-в-байт на фикстурах v6
            (подписки + server + chain + локальные Outbounds +
            DisabledNodes + Fold + Meta);
      - [ ] ID-стабильность: ULID неизменны через циклы Save/Load, новые
            не выдаются (риск Р3 — проверить, что все создатели Source
            выдают ULID при создании).
- [ ] `go build ./...` + `go test ./core/state/... ./ui/...` зелёные.

## W5 — Чистка мёртвого транспорта и полей

- [ ] `UpdateParserConfig(text)`: удалить из
      `business/ui_updater.go:23` (интерфейс),
      `presentation/presenter_ui_updater.go:23-40` (полезный остаток —
      `RefreshOutboundsConfiguratorList` — вызывать напрямую где нужно);
      убрать все ~15 вызовов (`source_tab.go:101,212,240,825,1017`;
      `source_edit_window.go:329,663,1577`; `sources.go:153`;
      `sources_json.go:148,164`; `presenter_sync.go:259`).
- [ ] `models/wizard_model.go`: удалить поля `ParserConfig`,
      `ParserConfigJSON`, метод `RefreshDerivedParserConfig` (компилятор
      выдаст пропущенные call-site'ы — каждому уже должен соответствовать
      `BumpRevision` из W1; риск Р2); `AsParserConfig` остаётся с
      комментарием об индексном инварианте 1:1 (Р1).
- [ ] `presentation/presenter_target.go:246-256`:
      `invalidateParsedNodes` — вместо `model.ParserConfig = nil` —
      `BumpRevision` + инвалидция превью.
- [ ] `configurator.go:339-361`: сид из шаблона — парс строки шаблона
      сразу в canonical (`GlobalOutbounds`/`Defaults`), без
      `model.ParserConfigJSON`.
- [ ] `presentation/gui_state.go:91-93,118`: остатки «last valid
      ParserConfig JSON» удалить.
- [ ] `business/validator.go:220-236`: `ValidateParserConfigJSON` +
      `TestValidateParserConfigJSON` (`validator_test.go:417`) удалить
      (прод-вызовов нет).
- [ ] Прогнать grep-инварианты SPEC §5.A — все по нулям.
- [ ] `go build ./...` + `go test ./...` зелёные.

## W6 — Приёмка

- [ ] Поведенческие тесты SPEC §5.C (business/presentation-уровень, без
      ассертов на форматирование строк — правило no-ui-format-tests):
      - [ ] C1 CRUD Направления сквозь Save/Load;
      - [ ] C2 rename canonical-only;
      - [ ] C3 окно источника: Apply → `m.Sources[i]`; Cancel — без
            следов в модели (deep-copy, Р4);
      - [ ] C4 preset toggle — одна запись, идемпотентность;
      - [ ] C5 stale-guard по ревизии (если не закрыт W1);
      - [ ] C6 гейты пустой модели;
      - [ ] C7 update-конвейер от свежезагруженного state.
- [ ] Полный прогон: `go build ./...`, `go test ./...`, `go vet ./...`;
      GUI-пакеты — `build/`-скрипты.
- [ ] Греп go1.20 по диффу:
      `git diff --name-only | xargs grep -nE "\b(min|max|clear)\(|slices\.|maps\.|PathValue|errors\.Join"`
      (только чтение диффа, без git-мутаций) → 0 в новых правках.
- [ ] `ui/traffic/**` не затронут (`git status` — только чтение).
- [ ] IMPLEMENTATION_REPORT.md: изменённые файлы, снесённые тесты с
      причинами, изменённые сигнатуры, таблица grep-инвариантов,
      результаты roundtrip.
