# PLAN 045 — STATE_CONFIG_DECOUPLING

**Решения по итогам фаз 0 (LxBox deep-dive) + 1 (карта текущего лаунчера):**

| # | Вопрос | Решение |
|---|--------|---------|
| 1 | Имя контейнера UI после переименования | **Configurator** (`ui/configurator/`); вкладки внутри остаются (Settings, DNS, Rules, Sources) |
| 2 | Кэш outbounds | **Файл** `bin/outbounds.cache.json` (атомарная запись `.tmp` → rename) |
| 3 | Переименование Wizard → Configurator | Делаем **сразу**, в этом же SPEC'е |
| 4 | Typed events (SPEC 047) | Заводим **сразу**, ставим как зависимость 045 |
| 5 | Темп поставки | **Один большой шаг** (одна фича-ветка / один PR), но внутри чистая многоуровневая декомпозиция (см. ниже) |

---

## Архитектурный target

```
bin/
├── settings.json           — settings (как сейчас, не трогаем)
├── state.json              — v5: декларативное состояние Configurator
├── outbounds.cache.json    — НОВОЕ: outbounds, спарсенные из подписок
└── config.json             — производная: BuildConfig(state, outboundsCache, template)

core/
├── state/                  — НОВОЕ: чистая модель state, Load/Save, миграции v4 → v5
├── build/                  — НОВОЕ: BuildConfig — единственная функция-генератор config.json
├── outboundscache/         — НОВОЕ: чтение/запись outbounds.cache.json
├── events/                 — НОВОЕ: типизированный event-bus (SPEC 047)
├── services/
│   └── state_service.go    — расширен: два независимых dirty-флага
├── config/                 — упрощён: WriteToConfig переезжает в build/
└── config_service.go       — Update path = parser → cache → BuildConfig

ui/
├── configurator/           — переимен. из ui/wizard/
└── ...
```

### Контракт `BuildConfig`

```go
package build

type Inputs struct {
    State    *state.State
    Cache    *outboundscache.Snapshot   // outbounds + endpoints из последнего парсинга
    Template *template.Template          // wizard_template.json
}

type Result struct {
    ConfigJSON     []byte                 // готовый, провалидированный sing-box check
    Validation     ValidationResult       // fatal + warnings
    GeneratedVars  map[string]string      // случайные secret'ы и т.п.
}

func BuildConfig(in Inputs) (Result, error)
```

`BuildConfig` — **единственная** функция, формирующая итоговый JSON конфига. Не пишет в файл сама; вызывающий слой (в `config_service.UpdateConfigFromSubscriptions` и при initial bootstrap) — отдельным шагом записывает атомарно.

### Контракт `state`

```go
package state

type State struct {
    Version       int
    ID            string
    Comment       string
    CreatedAt     time.Time
    UpdatedAt     time.Time
    ConfigParams  []ConfigParam
    Vars          []SettingVar
    SelectableRules []SelectableRuleState
    CustomRules     []CustomRule
    DNSOptions    *DNSOptions
    ParserConfig  parserconfig.ParserConfig    // proxies + outbounds metadata
    RulesLibraryMerged bool
}

func Load(path string) (*State, error)
func (s *State) Save(path string) error           // атомарная запись
func (s *State) Diff(prev *State) Diff            // для dirty-маркеров
```

Миграция: `Load` поддерживает чтение v3, v4, v5; `Save` всегда пишет v5.

### Контракт `outboundscache`

```go
package outboundscache

type Snapshot struct {
    Version      int
    UpdatedAt    time.Time
    Outbounds    []json.RawMessage    // готовые JSON-блоки sing-box outbound
    Endpoints    []json.RawMessage
    SourceStats  map[string]SourceStat
}

func Load(path string) (*Snapshot, error)
func (s *Snapshot) Save(path string) error
func (s *Snapshot) IsEmpty() bool
```

Парсер пишет cache → BuildConfig читает. `WizardModel.GeneratedOutbounds[]` становится **read-only зеркалом** этого snapshot'а для UI preview, не источником правды.

### Контракт `events` (SPEC 047)

```go
package events

type Bus interface {
    Publish(Event)
    Subscribe(EventKind, Handler) Cancel
}

type EventKind int
const (
    StateChanged EventKind = iota
    ConfigBuilt
    SubscriptionUpdated
    VpnStateChanged
    ProxyActiveChanged
    PowerResume
)
```

Реализация — sync dispatch (handlers вызываются в той же goroutine, что публикует). Подробнее — SPEC 047.

### Два dirty-маркера в `StateService`

```go
// UpdateDirty — список источников / proxies / skip изменился, надо перепарсить и пересобрать.
func (s *StateService) IsUpdateDirty() bool
func (s *StateService) ClearUpdateDirty()  // вызывается после успешного парсера+build

// RestartDirty — поля шаблона (tun, dns, rules, log_level) изменились,
// config пересобран, но sing-box работает со старым.
func (s *StateService) IsRestartDirty() bool
func (s *StateService) ClearRestartDirty()  // вызывается после Restart sing-box
```

Setter'ы — внутри `state.Diff`-логики (см. ниже). UI не вызывает их напрямую, только читает через `state.OnSaved` event.

### Поток данных «Save в Configurator»

```
User: clicks Save in Configurator
  → ConfiguratorPresenter.Save():
      → SyncGUIToModel()                  # как сейчас
      → state.Save(path)                  # атомарно
      → eventbus.Publish(StateChanged{kind: ...})
      → showSuccessDialog()
```

Configurator **не пишет config.json**, **не зовёт parser**, **не дёргает sing-box**.

### Поток данных «Update»

```
User: clicks Update OR cron auto-update OR initial bootstrap (config.json missing)
  → config_service.RunUpdate():
      → state.Load()
      → parser.FetchSources(state.ParserConfig)  # сеть
      → outboundscache.Save(snapshot)            # на диск
      → build.BuildConfig({state, cache, template})  → result
      → atomic_write(config.json, result.ConfigJSON)
      → state_service.ClearUpdateDirty()
      → eventbus.Publish(ConfigBuilt{...})
```

### Поток данных при bootstrap

```
main.go startup:
  → state.Load()      # v4-файлы прозрачно мигрируются в v5 при первом Save
  → outboundscache.Load()  # пусто при первом запуске
  → if config.json absent OR state.UpdatedAt > config.json mtime:
       → trigger config_service.RunUpdate() lazy (через первую auto-update итерацию)
```

Т.е. lazy rebuild — не блокируем UI на старте. Если `config.json` отсутствует совсем — sing-box не стартанёт до первого Update; это норма для чистой установки.

### Diff и dirty-маркеры

```go
// state.Diff возвращает структуру с булями:
type Diff struct {
    ProxiesChanged       bool   // → UpdateDirty
    SourceListChanged    bool   // → UpdateDirty
    SkipChanged          bool   // → UpdateDirty
    TunChanged           bool   // → RestartDirty
    DNSChanged           bool   // → RestartDirty
    RulesChanged         bool   // → RestartDirty
    LogLevelChanged      bool   // → RestartDirty
    VarsChanged          bool   // depends on var: некоторые → restart, некоторые → update
}
```

При Save сравниваем загруженный state со старым (in-memory копией от первого Load), вычисляем Diff, выставляем флаги.

### Переименование Wizard → Configurator

Площадь:
- Папка `ui/wizard/` → `ui/configurator/`.
- Все `package wizard` → `package configurator`.
- i18n-ключи `wizard.*` → `configurator.*` (с миграцией старых файлов локалей в коде).
- Имя вкладки в UI — рендерится из i18n, переедет автоматически при ключах.
- `WizardModel` → `ConfiguratorModel`, `WizardPresenter` → `ConfiguratorPresenter`, и т.д.
- Имя файла — **`state.json` остаётся** (это файл данных, не UI).
- Документация: `docs/CREATE_WIZARD_TEMPLATE.md` → `CREATE_CONFIGURATOR_TEMPLATE.md` (но шаблон остаётся `wizard_template.json` для совместимости с уже выпущенными конфигами).

В release-notes явный пункт: "**Wizard → Configurator** rename, поведение неизменно, кроме разделения Save и Build (см. ниже)."

## Порядок реализации (внутри одного PR)

Шаги независимы и каждый компилится зелёным:

1. **Фундамент 1 — `core/events`** (зависимостей ноль; реализация SPEC 047 stub).
2. **Фундамент 2 — `core/state`** (чистая модель state, миграции v3/v4/v5; параллельная с существующим `WizardStateFile`, ещё не подключённая).
3. **Фундамент 3 — `core/outboundscache`** (snapshot reader/writer).
4. **Фундамент 4 — `core/build`** (BuildConfig как чистая функция; принимает на вход state + cache + template, отдаёт JSON; тесты на нескольких сценариях).
5. **Bridge — расширить `StateService`** двумя dirty-флагами (`UpdateDirty`, `RestartDirty`) и публикацией `StateChanged` событий.
6. **Wire — `config_service.RunUpdate`** перевести на новый pipeline (parser → cache → BuildConfig → write).
7. **Wire — Wizard Save** урезать: убрать `SaveConfigWithBackup`, `populateCheckText`, `ensureOutboundsParsed` блокирующее ожидание; оставить только `state.Save` + `eventbus.Publish(StateChanged)`.
8. **UI — два маркера** в Core Dashboard: `*` на Update + отдельный маркер на Restart.
9. **UI rename — Wizard → Configurator** (большой рефакторинг по импортам и i18n; делается в конце, когда поведение стабильно).
10. **Удаление мёртвого кода** — старый `WizardStateFile` (если читается полностью через новый `core/state`), `populateCheckText`, кэш `WizardModel.GeneratedOutbounds[]` как источник правды (остаётся как preview-зеркало).
11. **Документация** — `docs/ARCHITECTURE.md`, `docs/release_notes/upcoming.md`, переход к `state.json v5`.
12. **IMPLEMENTATION_REPORT.md** + переименование папки SPEC `045-F-N-` → `045-F-C-`.

После шага 4 уже есть полный новый pipeline в виде «параллельно живущей» библиотеки — можно даже unit-тестировать end-to-end через testdata. После шага 6 — старый код не используется. После шага 9 — финальный clean-up.
