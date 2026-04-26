# TASKS 045 — STATE_CONFIG_DECOUPLING

Каждый блок — самостоятельный коммит. После каждого `go build ./... && go test ./... && go vet ./...` должны быть зелёными. Все блоки идут в одной фича-ветке `feat/state-config-decoupling`.

## ✅ Фаза 0 — LxBox deep-dive
- [x] Прочитать `/Users/macbook/projects/LxBox/`, написать отчёт в `LXBOX_NOTES.md`.

## ✅ Фаза 1 — карта текущей архитектуры лаунчера
- [x] Перечислить все writes `config.json` / `state.json`, callsites broadcast, dirty-маркер. Отчёт в `CURRENT_ARCH_NOTES.md`.

## ✅ Фаза 2 — финальный PLAN
- [x] PLAN.md с архитектурными решениями.

## Фаза 3 — Фундамент (новые независимые пакеты)

### 3.1 `core/events` — typed event-bus (SPEC 047)
- [ ] SPEC 047 заведён.
- [ ] Пакет `core/events/`: `Bus` интерфейс, `MemoryBus` реализация (sync dispatch), типы событий, `Cancel` token.
- [ ] Тесты: subscribe/publish, multiple handlers, cancel, panic-isolation handler'а.
- [ ] Зелёный `go build ./core/events && go test ./core/events`.

### 3.2 `core/state` — модель state + миграции
- [ ] Пакет `core/state/`: тип `State` (почти 1:1 с текущим `WizardStateFile`, но без UI-зависимостей).
- [ ] `Load(path) → *State, error`: читает v3/v4/v5, мигрирует в v5 в памяти.
- [ ] `Save(path) → error`: атомарная запись v5 (`.tmp` → rename).
- [ ] `Diff(old, new) → Diff{Proxies, Tun, DNS, Rules, Vars, ...}` для dirty-логики.
- [ ] `testdata/` с примерами v3, v4, v5.
- [ ] Тесты: round-trip, миграция v3→v5, миграция v4→v5, Diff на типичных правках.

### 3.3 `core/outboundscache` — snapshot outbounds
- [ ] Пакет `core/outboundscache/`: `Snapshot` (outbounds, endpoints, source stats, updated_at).
- [ ] `Load(path)`, `Save(path)` — атомарно.
- [ ] `IsEmpty()`.
- [ ] Тесты: round-trip, missing-file → empty snapshot, corrupt-file → error с понятным сообщением.

### 3.4 `core/build` — BuildConfig
- [ ] Пакет `core/build/`: функция `BuildConfig(Inputs) (Result, error)`.
- [ ] Логика: смерджить vars (template defaults + state.Vars), substitute placeholders, разместить outbounds из cache в `@ParserSTART/@ParserEND`, validate через sing-box check.
- [ ] Унаследовать существующую логику из `core/config/updater.go` + `ui/wizard/business/saver.go` (теперь в одном месте).
- [ ] Тесты: пустой cache → конфиг без outbounds (но с заглушкой); полный cache → ноды на местах; некорректный template → fatal validation; warnings — на сценарии WARNS.

## Фаза 4 — Расширение StateService

### 4.1 Два dirty-флага
- [ ] Добавить `UpdateDirty`, `RestartDirty` (mutex-protected) рядом с текущим `TemplateDirty`.
- [ ] Setter'ы: `MarkUpdateDirty()`, `MarkRestartDirty()`, `ClearUpdateDirty()`, `ClearRestartDirty()`.
- [ ] Getter'ы: `IsUpdateDirty()`, `IsRestartDirty()`.
- [ ] `TemplateDirty` оставить deprecated, но ещё использующимся, чтобы не ломать UI до фазы 6.
- [ ] Тесты на concurrency (как у существующего `TemplateDirty`).

### 4.2 Публикация `StateChanged` events через bus
- [ ] StateService держит ссылку на `events.Bus`.
- [ ] При `MarkUpdate/Restart` дополнительно `bus.Publish(StateChanged{Diff})`.
- [ ] AppController передаёт bus в StateService при создании.

## Фаза 5 — Перевод pipeline'ов

### 5.1 `config_service.RunUpdate` через новый pipeline
- [ ] `RunParserProcess` теперь: `state.Load` → `parser.FetchSources(state.ParserConfig)` → `outboundscache.Save(snapshot)` → `build.BuildConfig({state, cache, template})` → `atomic_write(config.json)` → `state_service.ClearUpdateDirty()` → `bus.Publish(ConfigBuilt)`.
- [ ] Старые `WriteToConfig` / `PopulateParserMarkers` помечены deprecated, не вызываются из новых путей.
- [ ] Сохранена обратная совместимость по UI-callbackам (`UpdateConfigStatusFunc` ещё дёргается, чтобы не ломать UI до фазы 6).

### 5.2 Configurator Save → state-only
- [ ] `presenter_save.go`: убрать `SaveConfigWithBackup`, `populateCheckText`, `buildConfigForSave`.
- [ ] `ensureOutboundsParsed` остаётся только для UI preview (генерирует данные для предпросмотра, не для записи).
- [ ] `executeSaveOperation` сводится к: `SyncGUIToModel` → `state.Save` → `state_service.MarkUpdateDirty/MarkRestartDirty(diff)` → `bus.Publish(StateChanged)` → success dialog.
- [ ] Удалить блокирующее ожидание `AutoParseInProgress`.
- [ ] Тесты: Save в UI больше не пишет config.json (юнит-тест с временной директорией: до Save и после Save mtime config.json одинаковый).

## Фаза 6 — UI два маркера

### 6.1 Кнопка Update в Core Dashboard
- [ ] Маркер `*` показывается, если `IsUpdateDirty()` AND есть подписки.
- [ ] `updateConfigInfo` пересчитывает по событиям (подписка на `ConfigBuilt`, `StateChanged`), а не по callback `UpdateConfigStatusFunc`.

### 6.2 Маркер на Restart
- [ ] Новый виджет / индикатор рядом с кнопкой Restart.
- [ ] Показывается, если `IsRestartDirty()` AND `RunningState.IsRunning()`.
- [ ] Tooltip / hover-текст: «Шаблон изменился, перезапустите sing-box чтобы применить».
- [ ] Сбрасывается на successful Restart.
- [ ] i18n-ключи: `core_dashboard.restart_dirty_marker`, `core_dashboard.restart_dirty_tooltip` (en+ru).

### 6.3 Удалить `TemplateDirty` deprecated
- [ ] Все callsites переведены на `Update/RestartDirty`. Убрать поле и API из `StateService`.
- [ ] Обновить `state_service_test.go`.

## Фаза 7 — Rename Wizard → Configurator

### 7.1 Переименование папок и пакетов
- [ ] `git mv ui/wizard ui/configurator`.
- [ ] Все `package wizard` → `package configurator` (внутренние подпакеты — `business`, `models`, `presentation` остаются, переезжают вместе с родителем).
- [ ] Все impоrt пути → `singbox-launcher/ui/configurator/...`.
- [ ] Структуры: `WizardModel` → `ConfiguratorModel`, `WizardPresenter` → `ConfiguratorPresenter`, `WizardController` → `ConfiguratorController`, и т.д. (replace_all по проекту).

### 7.2 i18n
- [ ] Все ключи `wizard.*` → `configurator.*` в `internal/locale/en.json`, `internal/locale/ru.json`.
- [ ] Тесты `TestAllKeysPresent` проходят.

### 7.3 Документация
- [ ] `docs/CREATE_WIZARD_TEMPLATE.md` → `docs/CREATE_CONFIGURATOR_TEMPLATE.md` (+`_RU`).
- [ ] `docs/WIZARD_CHILD_WINDOWS.md`, `docs/WIZARD_STATE.md` → переименовать аналогично.
- [ ] Обновить README.md + README_RU.md (упоминания «Config Wizard»).
- [ ] Файл `bin/wizard_template.json` **остаётся** (формат шаблона не меняется; только UI-обёртка переименована).

## Фаза 8 — Документация и release

### 8.1 ARCHITECTURE
- [ ] Обновить `docs/ARCHITECTURE.md`: новый слой state, BuildConfig, outboundscache, events bus, два маркера.

### 8.2 Release notes
- [ ] `docs/release_notes/upcoming.md` (EN + RU): «Configurator (renamed from Wizard) + state/config decoupling: Save в Configurator больше не мутирует config.json; новый шаг Build...».
- [ ] Migration notes: state.json v4 → v5 автоматическая; на чистой установке без подписок sing-box стартанёт после первого Update.

### 8.3 SPEC closure
- [ ] `IMPLEMENTATION_REPORT.md` со сводкой фактических решений и отклонений от PLAN.
- [ ] Переименовать папку `SPECS/045-F-N-STATE_CONFIG_DECOUPLING` → `SPECS/045-F-C-STATE_CONFIG_DECOUPLING`.
- [ ] SPEC 047 (events) — закрытие отдельным IMPLEMENTATION_REPORT в его папке.

## Гейты качества (на каждом коммите)

- [ ] `go build ./...` — зелёный.
- [ ] `go test ./...` — зелёный.
- [ ] `go vet ./...` — чисто.
- [ ] `golangci-lint run` — чисто на изменённых пакетах.
- [ ] Зелёный `go test -race` хотя бы на `core/state`, `core/build`, `core/events`, `core/services`.
