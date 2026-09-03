# SPEC 117 · Этап 1 — план реализации

Архитектура решения и порядок волн. Все file:line — из `recon/consumers.md`
и `recon/jsontab.md` (ветка develop, 2026-08-29); перед правкой каждого
места сверяться с актуальным кодом — строки могли уехать.

## 1. Целевой поток данных

```
UI-обработчики / business
        │  мутация (единственная)
        ▼
WizardModel.Sources / GlobalOutbounds / Defaults     ← canonical, рабочая модель
        │  BumpRevision() после каждой мутации
        ├──────────────► Revision (uint64) ──► stale-guard, мемо-ключи
        │
        │  на входе parse/generate/validate (одноразово, выбрасывается)
        ▼
AsParserConfig() → *config.ParserConfig ──► парсер / генератор / валидатор
        │
        │  Save
        ▼
CreateStateFromModel → State.Connections ──► state.Save → диск v6 (как есть)
                                                  │ Load
                                                  ▼
                              State.Connections + Load-проекция State.ParserConfig
                              (syncLegacyFromConnections; read-only, для
                               build-путей core, перечитывающих state с диска)
```

Обратных стрелок нет. Слово «legacy» в живом коде остаётся только в трёх
ролях: (1) проекция `AsParserConfig`/`ToProxySourceV4`/`syncLegacyFromConnections`,
(2) чтение старых схем в миграциях Load, (3) сигнатуры общих
build-хелперов на `configtypes` (`SyncOutboundsWithTemplate`,
`MergeOutboundUpdatesInPlace`, `UpsertUserPatch` — тип `Direction` общий
для canonical и проекции, их не трогаем).

## 2. Проекция: два берега

### 2.1 UI-берег: `AsParserConfig()`

`wizard_model.go:265-282` остаётся как есть (свежий объект на каждый вызов,
caller может мутировать). Вызывается **только** непосредственно перед
потреблением:

- `business/parser.go` `ParseAndPreview`: вместо
  `json.Unmarshal(model.ParserConfigJSON)` (`parser.go:52-83`) — прямой
  `pc := model.AsParserConfig()`. Двойная конвертация canonical→строка→legacy
  умирает. Migrate/Sync/Merge preset-патчей (`parser.go:109-121`) работают
  на этой локальной копии — legacy-запись `model.ParserConfig = &parserConfig`
  (`parser.go:213`) удаляется, читатели переводятся (волна 2).
- `business/preview_cache.go` `RebuildPreviewCache`: локальная проекция
  вместо `model.ParserConfig` (`preview_cache.go:28-150`); ветка
  «nil → RefreshDerivedParserConfig» умирает.
- `business/validator.go` `ValidateParserConfig`: принимает проекцию
  (вызов из ParseAndPreview) — сама функция не меняется.
- `outbounds_configurator/edit_dialog.go:727-744`: для
  `PreviewGlobalSelectorNodes(allNodes, proxies, cfg)` — единственное
  место конфигуратора, где legacy-форма нужна по сигнатуре core —
  строится одноразовая проекция `model.AsParserConfig().ParserConfig.Proxies`
  на месте вызова (подписи источников — из `model.Sources`).
- `ui/configurator/tabs/source_chain_tab.go:716-729`
  `getParserConfigForChain` → либо локальная проекция, либо (лучше)
  перевод `collectChainHopCandidates` (`source_chain_hops.go:86-115`) на
  canonical `GlobalOutbounds`+`Sources[i].Outbounds` — кандидаты позиций
  это теги Направлений, проекция им не нужна.

### 2.2 Core-берег: Load-проекция `State.ParserConfig`

Диск v6 не меняется. `syncLegacyFromConnections` (`sync_to_legacy.go:16`)
остаётся и зовётся на Load (`load_v5.go:50`, `load_v6.go:81`) — наполняет
`State.ParserConfig` как **read-only Load-проекцию**. Её читатели остаются:

- `core/config_service.go:372-383` `loadParserConfigForUpdate` — безопасно:
  делает свежий `state.Load` на каждую операцию, проекция всегда
  актуальна диску;
- `core/rebuild.go:186-187`, `core/rebuild_raw_cache.go:89-118` — то же;
- gate «Proxies == nil → ошибка» продолжает работать (Load всегда
  проецирует).

Меняется: комментарий у поля (`state.go:78-84`) — «read-only проекция,
наполняется только на Load; писать в неё запрещено, Save её не читает».
Счётчики `main.go:259-272` и `ui/core_dashboard_tab.go:556-574`
переводятся на `s.Connections` (механика; чтобы поле нельзя было
случайно счесть рабочим). `buildContextFromState`
(`config_service_context_go:26-28`) — мёртвый параметр
`_ *config.ParserConfig` вычищается вместе с провозом в `rebuild.go:186`.

Инвариант (зафиксировать комментарием): код, мутирующий `s.Connections`
в памяти, не имеет права читать `s.ParserConfig` того же экземпляра —
проекция строится один раз на Load. Сегодня нарушителей нет (fetch-пути
ходят по `state.Connections.Sources` — consumers.md §1.6, «auto-update
уже чистый»).

## 3. Ревизия модели

`features/state.md` «Ревизия модели»: монотонный счётчик, растущий при
каждой правке; производные результаты привязываются к ревизии на момент
старта, результат с устаревшей ревизией выбрасывается.

Интерфейс (в `wizard_model.go`):

```go
// Revision — монотонная ревизия модели (features/state.md). Растёт при
// каждой мутации Sources/GlobalOutbounds/Defaults/rules-влияющих полей.
// Не сериализуется. Заменяет строковый fingerprint ParserConfigJSON.
Revision uint64 `json:"-"`

// BumpRevision — пометить модель изменённой. Только UI-поток (модель
// без внутренней синхронизации, как и остальные поля WizardModel).
func (m *WizardModel) BumpRevision() { m.Revision++ }
```

Без атомиков: все мутации модели и так идут на UI-потоке (тот же контракт,
что у остальных полей); фоновая генерация читает снапшот ревизии до
старта и сравнивает после — через `fyne.Do`/существующие обёртки, как
сегодня сравнивалась строка.

**Точки бампа** — ровно сегодняшние call-site'ы
`RefreshDerivedParserConfig` (механическая замена вызова, список из
consumers/jsontab): `source_tab.go:101,212,240,825(applySourceMutation),1017`;
`source_edit_window.go:329,663,1577`; `business/sources.go:150-153`;
`business/sources_json.go:145-148,163-164`; `presenter_sync.go:249-260`;
`presenter_state_helpers.go:31-67` (restore на Load — бамп легален,
производные обязаны перечитаться); `configurator.go:339-361` (сид из
шаблона). Плюс новые canonical-мутации волны 3 (операции
outbounds_configurator, Apply окна источника) — каждая завершается
`BumpRevision()`.

**Потребители:**

- `ParseAndPreview`: `rev := model.Revision` на старте; вместо
  `strings.TrimSpace(model.ParserConfigJSON) != parserConfigJSON`
  (`parser.go:170`) — `model.Revision != rev` → выброс результата,
  `BuildReportGen = 0` (семантика прежняя).
- `GetAvailableOutbounds` (`outbound.go:69-140`): мемо-ключ
  `AvailableOutboundsMemoKey string` → `AvailableOutboundsMemoRev uint64`
  (0 = пусто); сброс в `InvalidatePreviewCache` как раньше.
- `PreviewCacheGeneration` (`wizard_model.go:195-200`) в этапе 1 **не
  трогаем** — своя механика инвалидции превью-кэша, умирает в v7 вместе
  с кэшем. Слияние двух счётчиков — не наша волна.

## 4. Волны

Каждая волна — самостоятельно собираемое состояние: `go build ./...` и
существующая зелень (кроме явно перечисленных тестов волны) — зелёные.

### W1 — Фундамент: ревизия (не ломающая)

Добавить `Revision`/`BumpRevision`; вставить бамп во все точки §3 рядом с
существующим `RefreshDerivedParserConfig` (Refresh пока живёт — читатели
ещё на кэшах); перевести stale-guard `parser.go:170` и мемо
`outbound.go:80-102` на ревизию. Переработать
`parser_stale_test.go` (2 теста) на ревизию.

Файлы: `models/wizard_model.go`, `business/parser.go` (только guard),
`business/outbound.go` (только мемо), точки бампа (8 файлов, по строке).

### W2 — Read-пути → canonical / локальная проекция

Все чтения `model.ParserConfig`/`ParserConfigJSON` переводятся; фоллбэки
«nil → распарси строку» удаляются:

- `business/parser.go`: вход `AsParserConfig()` вместо строки; удалить
  запись `model.ParserConfig = &parserConfig` (`:213`);
- `business/outbound.go:69-140,267-290`: теги Направлений из
  `GlobalOutbounds` + `Sources[i].Outbounds`;
- `business/detour.go:237-260`: `localSubscriptionGroupTags` из
  `model.Sources` (сигнатуры `DetourOptions*` на `*ProxySource` — см.
  W3, окно источника);
- `business/preview_cache.go`: локальная проекция; `previewDirectionTags`
  (`:151-165`) из canonical; `applyMigratedDisabledKeys` (`:191-203`)
  остаётся (уже пишет canonical; индексный инвариант — риск Р1);
- `business/create_config.go:77`, `presentation/presenter_async.go:45`,
  `presenter_save.go:89-113`: гейты по `len(model.Sources)`;
- `business/direction_rename.go:44-63`: `DirectionTagTaken` — только
  canonical;
- `tabs/source_tab.go:889-911`: счётчики из `len(m.Sources)` /
  `PreviewNodesBySource`;
- `tabs/source_chain_tab.go`, `source_chain_hops.go`: кандидаты из
  canonical (§2.1);
- `core`: `main.go:259-272`, `ui/core_dashboard_tab.go:556-574` →
  `s.Connections`; `config_service_context.go` мёртвый параметр +
  `rebuild.go:186` провоз;
- `business/loader.go:39-60` — не трогаем (парсит строку **шаблона**,
  это сид, не модель).

### W3 — Write-пути и CRUD

Два настоящих переписывания + дубли:

1. **`outbounds_configurator`** (configurator.go, configurator_helpers.go,
   edit_dialog.go): пакет получает `*models.WizardModel` и работает на
   `model.GlobalOutbounds` / `model.Sources[i].Outbounds`:
   - `getParserConfig` (`configurator_helpers.go:280-297`) — удалить;
   - reorder (`configurator.go:70-86`), delete (`:211-217`), toggle
     (`:286-302`), `Updates`-патч (`:240-247`), add (`:337-350`), seed
     required (`:370-390`) — на canonical-слайсах;
   - смена scope (`:164-190`): перенос между `GlobalOutbounds` и
     `Sources[i].Outbounds` по индексу источника; достройка пустых
     `ProxySource{}` (`:184-187,346-349`) умирает — у canonical источник
     либо есть, либо scope недоступен;
   - `collectRows` (`:58-100`), `collectAllTags` (`:231-244`) — чтение
     canonical;
   - `syncPresetOutboundsForModel` (`:260-278`) — одна запись
     (`GlobalOutbounds`);
   - `edit_dialog.go` превью — одноразовая проекция (§2.1);
   - каждая мутирующая операция завершает `BumpRevision()`.
   Одновременно в этой же волне `onConfiguratorApply`
   (`source_tab.go:994-1024`) вырождается в
   `BumpRevision` + refresh списков (обратное копирование удаляется —
   конфигуратору больше нечего копировать), а `CreateDirectionsTab`
   (`:981-992`) перестаёт материализовать `m.ParserConfig`.
2. **`source_edit_window.go`** (~1600 строк): scratch
   `ps := m.Sources[i].ToProxySourceV4()` (`:405`) → рабочая копия
   `src := m.Sources[i]` (deep-copy: слайсы/карты/указатели `Fold`,
   `Outbounds`, `DisabledNodes`, `Meta` — копировать явно). Формы
   (detour-поля, skip, tag, Fold-вкладка `:554`, `foldTagPrefix`
   `source_fold_tab.go:206`, `setNodeEnabled` `:215-230`) читают/пишут
   копию `state.Source`; `applyProxyEditToSource` (`:232-313`)
   заменяется присваиванием `m.Sources[i] = copy` + `BumpRevision()`
   (`serializeParserAfterSourceEdit` `:315-336` упрощается).
   `DetourOptions*` (`business/detour.go:47,122`) меняют сигнатуру
   `*configtypes.ProxySource` → `*corestate.Source`.
3. **Дубли**: `RenameDirection` (`direction_rename.go:120-130`) — две
   legacy-правки удаляются; `RefreshAfterPresetToggle`
   (`presenter_sync.go:249-260`) — одна запись + `BumpRevision`;
   `presenter_state.go:176-185` — одиночный
   `SyncOutboundsWithTemplate(state.Connections.Outbounds)`.

### W4 — Снос обратных синков (core) и legacy-заполнения Save

Одной волной (взаимные страховки перестают быть нужны одновременно):

- удалить `core/state/sync_to_connections.go` + вызов `save.go:36`;
- `presenter_state.go:100-115`: `CreateStateFromModel` пишет только
  Connections (заполнение `state.ParserConfig` проекцией удаляется);
- `presenter_sync.go:438-470`: `ApplyParserConfigFromCurrentJSON`
  удалить + вызов `configurator.go:643`;
- `connections_helpers.go:12-79`: `buildTagSpecFromLegacy` и хелперы
  обратного направления — удалить неиспользуемое (прямые остаются);
- переработать/удалить тесты категории (а) core/state (SPEC §6):
  roundtrip-тест B, ID-стабильность.

### W5 — Чистка мёртвого транспорта и полей

- `UpdateParserConfig(text)`: удалить из интерфейса
  (`business/ui_updater.go:23`), презентера
  (`presenter_ui_updater.go:23-40` → оставить/переименовать
  `RefreshOutboundsConfiguratorList`-вызов там, где он был полезной
  частью), убрать ~15 вызовов с `m.ParserConfigJSON`;
- удалить поля `WizardModel.ParserConfig`, `ParserConfigJSON`,
  `AvailableOutboundsMemoKey/Tags` (заменён на Rev в W1),
  `RefreshDerivedParserConfig`; `invalidateParsedNodes`
  (`presenter_target.go:246-256`) — строку `model.ParserConfig = nil`
  заменить бампом/инвалидацией превью;
- `configurator.go:339-361`: сид из шаблона — парс строки шаблона сразу
  в canonical (`GlobalOutbounds`), без промежуточного
  `model.ParserConfigJSON`;
- `gui_state.go:91-93,118` — остатки «last valid ParserConfig JSON»;
- `ValidateParserConfigJSON` (`validator.go:220-236`) + тест — удалить;
- пройти grep-инварианты SPEC §5.A.

### W6 — Тесты, приёмка, отчёт

- новые тесты: roundtrip B (если не в W4), сценарии C1–C7 SPEC §5.C
  (business-уровень, без UI-формат-ассертов — правило
  no-ui-format-tests);
- полный прогон `go build ./... && go test ./... && go vet ./...`;
  GUI-пакеты — `build/`-скрипты;
- греп go1.20 по диффу (`min(|max(|clear(|slices\.|maps\.|PathValue|errors\.Join`);
- IMPLEMENTATION_REPORT.md: список снесённых тестов с причинами,
  изменённые сигнатуры (DetourOptions, конструктор конфигуратора),
  таблица grep-инвариантов.

## 5. Риски и ловушки

- **Р1. Индексный инвариант `Sources[i] ↔ Proxies[i]`** (consumers §4.11).
  `AsParserConfig` строит проекцию 1:1 по индексу — на нём висят
  `applyMigratedDisabledKeys` (`preview_cache.go:191-203`, пишет canonical
  по индексу legacy-прокси) и карты превью
  `PreviewNodesBySource`/`SourceNodeCounts` (`map[int]`). После этапа 1
  инвариант **локализуется** (единственный производитель проекции —
  `AsParserConfig`) — зафиксировать комментарием у него; multi-connection
  legacy-записи больше не возникают (их разворачивал умерший
  `sync_to_connections.go:156-206` на старых формах — Load-миграции
  разворачивают на Load, дальше индекс стабилен).
- **Р2. Покрытие BumpRevision.** Пропущенный call-site = «правка не
  протухает превью» — регресс тише, чем краш. Страховка: бамп ставится
  строго на место каждого `RefreshDerivedParserConfig` (компилятор найдёт
  всех после удаления Refresh в W5) + каждая новая canonical-мутация W3.
  Тест C5 покрывает основной сценарий гонки.
- **Р3. ID-стабильность без синка.** Матчинг `sync_to_connections.go`
  сохранял ULID при Save; теперь ID обязан рождаться в момент создания
  Source. Проверить всех создателей: `AppendURLsToSources`
  (`sources.go:150`), `AppendManualConfigJSON` (`sources_json.go:145`),
  Add chain (`source_tab.go:225-245`), клонирование — каждый выдаёт ULID
  сразу. Тест B ловит регресс.
- **Р4. Deep-copy `state.Source` в окне источника.** Поверхностная копия
  разделит `Fold`/`Outbounds`/`DisabledNodes`/`Meta` с моделью → правка
  в форме утечёт в модель до Apply (и переживёт Cancel). Написать
  явный `cloneSource()` (или использовать существующий, если есть) и
  тест на Cancel-без-следов.
- **Р5. Scope-инвалидация конфигуратора.** Сегодня смена scope
  достраивала пустые `ProxySource{}` под индекс (`configurator.go:184-187`)
  — маскировка рассинхрона длины. На canonical индекс источника обязан
  существовать; UI не должен предлагать scope на несуществующий индекс
  (источник выбирается из живого списка — проверить выпадашку scope).
- **Р6. `syncConnectionsFromLegacy` edge-case «Proxies == nil»**
  (`sync_to_connections.go:27-38`) сегодня защищает callsite'ы, пишущие
  сразу в Connections. После W4 защита не нужна (все пишут в
  Connections), но порядок волн важен: **не сносить синк раньше W3** —
  пока конфигуратор мутирует legacy, Save без синка молча потеряет его
  правки.
- **Р7. Сигнатурная волна.** Смена `DetourOptions*` и конструктора
  `outbounds_configurator` заденет тесты категории (в) компиляционно
  (`detour_test.go` 11 функций, `configurator_helpers_test.go`) —
  механическая правка билдеров тестовых моделей, семантику не менять.
- **Р8. `TemplateData.ParserConfig` — не путать с моделью.** Строка
  parser_config **шаблона** (`template/loader.go:46-53`) остаётся как
  есть — это сид, легально парсится в legacy-форму на входе
  (`business/loader.go`, `configurator_helpers.go:138-177`); grep-инварианты
  SPEC §5.A её исключают явно.
- **Р9. go1.20.** Не вводить `slices.Clone` для deep-copy (Р4) —
  `append([]T(nil), ...)` и ручные копии карт.
- **Р10. Порядок в `applySourceMutation`** (`source_tab.go:806-826`):
  `MarkAsChanged` зовётся первым намеренно (комментарий :807-809) —
  сохранить порядок при замене Refresh на Bump.

## 6. Что осознанно НЕ делаем (границы SPEC §3)

Материализация nodes[], v7-схема, NodeLink, FolderReplace, смерть
DisabledNodes/Fold/.raw/PreviewNodes, слияние
`PreviewCacheGeneration`+`Revision`, судьба `Diff.ProxiesChanged`
(семантика legacy, в прод-коде не выставляется — оставить как есть),
бэкап-контракт, `ui/traffic/**`, Git.
