# SPEC 117 · Этап 1 — однонаправленность потока данных модели источников

Статус: N (в плане). Ветка develop. Нормативная база: `SPECS/features/state.md`
(«Память = диск», «Ревизия модели»), `SPECS/features/sources.md`,
`SPECS/features/directions.md`. Фактура кода: `recon/consumers.md` (реестр
потребителей), `recon/jsontab.md` (JSON-слой UI), `recon/tests.md`
(тестовый ландшафт). Решения по конфликтам — `RECON_DECISIONS.md`.

## 1. Проблема

Одни и те же данные живут в трёх представлениях (consumers.md §0):

1. **Canonical** — `state.Source` / `State.Connections`
   (`core/state/connections.go`) и `WizardModel.Sources` +
   `GlobalOutbounds` + `Defaults` (`ui/configurator/models/wizard_model.go:74-83`).
   По SPEC 052 объявлен источником истины.
2. **Legacy-структура** — `configtypes.ParserConfig`/`ProxySource`,
   продублирована в `State.ParserConfig` (`core/state/state.go:84`) и
   `WizardModel.ParserConfig` (`wizard_model.go:98`).
3. **Legacy JSON-строка** — `WizardModel.ParserConfigJSON` (`wizard_model.go:93`):
   одновременно транспорт «canonical → парсер» (сериализация + обратный
   `json.Unmarshal` на каждый Preview, `business/parser.go:52`) и строковый
   fingerprint stale-detection (`parser.go:170`).

Формально истина — canonical; **фактически на записи побеждает legacy**:
`state.Save` → `syncConnectionsFromLegacy` (`core/state/save.go:36`,
`sync_to_connections.go:24`) пересобирает Connections из legacy-проекции,
которую `CreateStateFromModel` заполнил «ради совместимости»
(`presentation/presenter_state.go:112-115`). Обратный поток legacy → canonical
существует в трёх узлах:

- `syncConnectionsFromLegacy` на каждом Save;
- `onConfiguratorApply` (`ui/configurator/tabs/source_tab.go:994-1013`) —
  конфигуратор Направлений мутирует legacy, Apply копирует назад по индексу;
- `applyProxyEditToSource` (`ui/configurator/tabs/source_edit_window.go:232-313`) —
  окно источника живёт на scratch-`ProxySource` и вручную маппит поля обратно.

Плюс `ApplyParserConfigFromCurrentJSON` (`presentation/presenter_sync.go:438-470`)
делает legacy-структуру рабочей моделью конфигуратора, минуя canonical, а
дубли-правки (`RenameDirection` — четыре копии одного переименования,
`direction_rename.go:120-130`; двойные `SyncOutboundsWithTemplate` в
`presenter_sync.go:249-260`, `presenter_state.go:176-185`,
`configurator_helpers.go:260-278`) синхронизируют представления вручную.
Цена — целый класс багов рассинхронизации и две конвертации на каждый Save.

## 2. Цель этапа

Каноническая модель становится **единственной рабочей моделью** (state.md
«Память = диск»): её мутирует UI, её пишет сохранение, её читает сборка.
Legacy `ParserConfig`/`ProxySource` — **одноразовая проекция
«canonical → сборка»**: строится непосредственно перед parse/generate и
выбрасывается. Обратной синхронизации «проекция → модель» не существует
ни в одном месте. Строковый fingerprint заменяется **ревизией модели** —
монотонным счётчиком (state.md «Ревизия модели»).

## 3. Границы (жёсткие)

**В этапе 1:**

- Снос всех трёх обратных синков (§1) и scratch-паттерна окна источника
  (`ToProxySourceV4` → правка копии `state.Source` напрямую).
- Перевод `ui/configurator/outbounds_configurator` (CRUD Направлений) на
  canonical `GlobalOutbounds` / `Sources[i].Outbounds`.
- Ревизия модели + вычистка мёртвого транспорта `UpdateParserConfig(text)`
  (аргумент игнорируется с SPEC 104, `presenter_ui_updater.go:31`).
- Удаление legacy-половин дублей-правок (§1) и полей
  `WizardModel.ParserConfig`/`ParserConfigJSON` вместе с
  `RefreshDerivedParserConfig`.

**НЕ в этапе 1:**

- **Схема диска НЕ меняется**: v6 как есть. `syncLegacyFromConnections`
  на Load (`load_v5.go:50`, `load_v6.go:81`) остаётся построителем
  проекции `State.ParserConfig` — её читают build-пути core
  (`config_service.go:372-383`, `rebuild.go:186`, `rebuild_raw_cache.go:89`),
  которые перезагружают state с диска на каждую операцию.
- Материализация узлов подписки, v7 (плоский корень), NodeLink, папки,
  FolderReplace, смерть DisabledNodes/Fold/.raw-кэша — это следующие этапы.
- **Локальные Направления источника НЕ удаляются как фича** (это v7):
  их правка в этапе 1 идёт напрямую в canonical `Sources[i].Outbounds`.
- Миграции чтения старых схем (`legacy_migration.go`, `load_v2_v3_v4.go`)
  не трогаются — v4-путь строит Connections через `migrateLegacySources`
  и от обратного синка не зависит.
- `ui/traffic/**` не трогать. Git-операции запрещены.
- Контракт `contract/**` не меняется (внешних форм этап не касается).

## 4. Требования

- **Т1. Canonical-only мутации.** Всякая правка данных источников и
  Направлений из UI/business пишет только `model.Sources`,
  `model.GlobalOutbounds`, `model.Defaults` (и для state-уровня —
  `State.Connections`). Ни одна прод-строка не мутирует
  `ParserConfig`/`ProxySource` с намерением «доедет до модели».
- **Т2. Одноразовая проекция.** Legacy-форма создаётся только в двух видах:
  `WizardModel.AsParserConfig()` (+`Source.ToProxySourceV4()`) на UI-берегу —
  вызывается непосредственно на входе parse/generate/валидации и не
  сохраняется в модель; `State.ParserConfig` на core-берегу — наполняется
  только на Load как read-only проекция для build-путей.
- **Т3. Снос обратных синков.** Удаляются: `core/state/sync_to_connections.go`
  целиком + вызов в `save.go:36`; обратное копирование в
  `onConfiguratorApply`; `applyProxyEditToSource` со scratch-паттерном;
  `ApplyParserConfigFromCurrentJSON` + вызов на смене вкладки
  (`configurator.go:643`); заполнение `state.ParserConfig` в
  `CreateStateFromModel` (`presenter_state.go:112-115`).
- **Т4. Окно источника — прямая правка.** Форма Edit Source работает на
  копии `state.Source`; Apply записывает копию в `m.Sources[i]`.
  Вкл/выкл узла (`setNodeEnabled`) и Fold-вкладка правят поля
  `Source.DisabledNodes`/`Source.Fold` canonical-записи (сами поля
  живут до v7).
- **Т5. CRUD Направлений на canonical.** Все операции
  `outbounds_configurator` (reorder, add, edit, смена scope, delete,
  toggle, seed required, `Updates`-патчи) выполняются на
  `model.GlobalOutbounds` и `model.Sources[i].Outbounds`. Ленивая
  материализация `getParserConfig` (`configurator_helpers.go:280-297`)
  и достройка пустых `ProxySource{}` (`configurator.go:184-187,346-349`)
  умирают.
- **Т6. Ревизия модели.** `WizardModel` несёт монотонный счётчик ревизии;
  каждая мутация модели его поднимает. Stale-detection в `ParseAndPreview`
  (`parser.go:170`) и мемо `GetAvailableOutbounds`
  (`AvailableOutboundsMemoKey`) переводятся на ревизию. Строковые
  отпечатки сериализованной модели не используются нигде.
- **Т7. Дубли-правки.** Legacy-половина удаляется в: `RenameDirection`
  (остаются две canonical-правки — `GlobalOutbounds` и
  `Sources[i].Outbounds`), `RefreshAfterPresetToggle`,
  `syncPresetOutboundsForModel`, `CreateStateFromModel`
  (`presenter_state.go:176-185`).
- **Т8. Мёртвый код.** Удаляются: `UpdateParserConfig(text)` (сигнатура,
  интерфейс `ui_updater.go:23`, ~15 вызовов с `m.ParserConfigJSON`),
  поля `WizardModel.ParserConfig`/`ParserConfigJSON`,
  `RefreshDerivedParserConfig`, `AvailableOutboundsMemoKey/Tags`,
  `ValidateParserConfigJSON` (прод-вызовов нет), фоллбэки
  «ParserConfig == nil → распарси строку» (parser/outbound/detour/
  chain_tab/create_config/source_tab).
- **Т9. Поведение пользователя не меняется.** Все сценарии конфигуратора,
  окна источника, превью, Save/Load, Update подписок работают как раньше;
  меняется только внутренний поток данных.
- **Т10. Совместимость сборки.** go1.20 (win7-джоба): без
  `min`/`max`/`clear`/`slices`/`maps`/`PathValue`/`errors.Join`.

## 5. Критерии приёмки (проверяемые)

### A. Grep-инварианты (ноль вхождений в прод-коде, `*_test.go` исключены)

| Инвариант | Проверка |
|---|---|
| Обратный синк снесён | файла `core/state/sync_to_connections.go` нет; `grep -rn "syncConnectionsFromLegacy" core ui` → 0 |
| Save не читает legacy | в `core/state/save.go` нет обращений к `s.ParserConfig` |
| Поля-кэши удалены | `grep -rn "ParserConfigJSON\|RefreshDerivedParserConfig" ui core --include=*.go` → 0 (вне миграций чтения старых схем и `TemplateData.ParserConfig` — это строка шаблона, другая сущность) |
| Мёртвый транспорт вычищен | `grep -rn "UpdateParserConfig(" ui` → 0; `grep -rn "ApplyParserConfigFromCurrentJSON\|ValidateParserConfigJSON" ui` → 0 |
| Модель не держит legacy | в `WizardModel` нет полей типа `*config.ParserConfig`/строки его сериализации; `grep -rn "model.ParserConfig\b\|m.ParserConfig\b" ui --include=*.go` → 0 |
| Конфигуратор на canonical | в `ui/configurator/outbounds_configurator` нет упоминаний `ProxySource` кроме одноразовой проекции для `PreviewGlobalSelectorNodes`; функции `getParserConfig` нет |
| Scratch снесён | `grep -rn "ToProxySourceV4" ui` → 0 (в `core/state/adapter_source.go` остаётся — это прямая проекция) |
| Отпечатков нет | в `ui/configurator/business/parser.go` нет сравнения строк как stale-признака |

### B. Roundtrip и стабильность идентичности

- **Load→Save→Load байт-в-байт**: на фикстурах v6 (взять состав
  `v6_integration_test.go` + реальный многосекционный state с подписками,
  server-URI, chain, локальными Outbounds, DisabledNodes, Fold, Meta)
  `Load(f); Save(p1); Load(p1); Save(p2)` → `p1 == p2` побайтно;
  `p1` относительно фикстуры отличается только `updated_at`/`version`.
- **ID-стабильность**: ULID каждого Source после N циклов
  mutate-canonical → Save → Load неизменен; ни один Save не выдаёт новых
  ULID существующим источникам (раньше это гарантировал матчинг
  обратного синка — `config_json_roundtrip_test.go:44-50`; теперь ID
  живёт в canonical и не пересоздаётся вовсе).
- **Chain/Meta carry-over не нужен**: Meta, Label, Update, PreviewNodes
  переживают Save тривиально (никто их больше не пересобирает).

### C. Поведенческие сценарии (интеграционные тесты уровня business/presentation)

1. **CRUD Направления**: add → reorder → edit (включая смену scope
   global↔local) → toggle → delete через операции конфигуратора; после
   каждой операции `model.GlobalOutbounds`/`model.Sources[i].Outbounds`
   отражают её немедленно, без какого-либо Apply-шага;
   `CreateStateFromModel` → `Save` → `Load` возвращает ровно это.
2. **RenameDirection**: переименование правит тег в `GlobalOutbounds`, в
   `Sources[i].Outbounds` и во всех ссылках (детуры/цепочки — как сегодня);
   никакая четвёртая копия не существует (нет и объекта для неё).
3. **Окно источника**: правка URL/skip/detour/тега, toggle узла,
   Fold-настройки → Apply → `m.Sources[i]` содержит правку; повторное
   открытие окна показывает её без Save на диск.
4. **Preset toggle**: `RefreshAfterPresetToggle` производит одну запись —
   в `GlobalOutbounds`; повторный toggle идемпотентен.
5. **Stale-guard**: генерация, начатая на ревизии R, при любой мутации
   модели (ревизия R+1) до завершения выбрасывает результат
   (`BuildReportGen = 0`, `GeneratedOutbounds` не перезаписаны).
6. **Пустая модель**: гейты «нечего собирать» (`create_config.go`,
   `presenter_async.go`, валидация `SaveConfig`) срабатывают по
   `len(model.Sources) == 0`, а не по пустоте строки.
7. **Update/rebuild-конвейер** (`loadParserConfigForUpdate` → generate)
   работает от свежезагруженного state: Load-проекция содержит все
   источники, сохранённые UI canonical-путём.

### D. Сборка и тесты

- `go build ./...`, `go test ./...`, `go vet ./...` — чистые.
- GUI-пакеты — через скрипты `build/` как обычно.
- Тесты категории (а) из `recon/tests.md` переработаны/удалены по
  таблице §6; **вся остальная зелень остаётся зелёной** — категории (б)
  и (в) этап не трогает.
- Грепы go1.20-несовместимости по diff-файлам → 0.

## 6. Судьба тестов категории (а) (recon/tests.md)

| Тест | Судьба |
|---|---|
| `TestConfigJSON_LegacyRoundTrip` (core/state/config_json_roundtrip_test.go:17) | удалить; заменить roundtrip-тестом B (canonical load→save байт-в-байт + ID-стабильность) |
| `TestDetourTag_LegacyRoundTrip` (detour_mapping_test.go:20) | переработать: только прямая проекция canonical→`ToProxySourceV4` (detour-поля доезжают до проекции) |
| `TestSyncConnectionsFromLegacy_KeepsRefAndID` (detour_node_ref_test.go:76) | удалить (предмет — сам синк); инвариант ID закрывает тест B |
| `TestLoad_V4Minimal` (state_test.go:52,77) | переработать: ассерты на `s.Connections`; ассерт Load-проекции `s.ParserConfig` допустим (она остаётся на Load) |
| `TestSaveLoadRoundTrip` (state_test.go:148) | переработать: мутировать `s.Connections`, не legacy-view |
| `TestRenameDirectionTouchesBothViews` (direction_rename_test.go:105) | заменить на `TestRenameDirection_CanonicalOnly` (сценарий C2) |
| `TestParseAndPreview_Discards/AppliesWhenJSONChanges…` (parser_stale_test.go:52,87) | переписать на ревизию модели (сценарий C5) |
| `TestValidateParserConfigJSON` (validator_test.go:417) | удалить вместе с функцией — прод-вызовов нет (JSON-редактор снесён SPEC 104) |

Каждое удаление фиксируется строкой в IMPLEMENTATION_REPORT.md с причиной
(«предмет теста упразднён этим этапом»).

## 7. Зачем именно сейчас

Этап 1 — фундамент всей чистой модели (DRAFT, v7): пока рабочих моделей
две, любая новая фича обязана править обе и держать три обратных синка в
голове (класс закрытых багов: remote-override-is-global, tag-vs-label
#91, двойные записи preset-toggle). Однонаправленность делает v7
(материализацию, NodeLink, папки) изменением одной модели вместо трёх.
