# Фаза 1 — карта текущей архитектуры лаунчера

Анализ текущего кода singbox-launcher (Go + Fyne) на предмет связки state ↔ config ↔ reactivity, фаза 1 SPEC 045.

---

## 1. Все writes `config.json`

**Два write-point'а**, оба атомарные (stage + `os.Rename`):

| Call-site                                       | Файл                                          | Путь триггера                                                     |
|-------------------------------------------------|-----------------------------------------------|-------------------------------------------------------------------|
| `WriteToConfig`                                 | `core/config/updater.go:125–186`              | Parser run: `ConfigService.UpdateConfigFromSubscriptions()` (кнопка Update / auto-update) |
| `SaveConfigWithBackup`                          | `ui/wizard/business/saver.go:56–107`          | Wizard Save                                                       |

**Что пишет parser:** outbounds (свежепарсеные ноды) + endpoints (WireGuard) + обновлённый блок `@ParserConfig`.

**Что пишет wizard:** весь шаблон (vars, rules, log_level, …) + немедленно заполняет маркеры `@ParserSTART/@ParserEND` **из памяти** (`WizardModel.GeneratedOutbounds[]`, `GeneratedEndpoints[]` — кэш последнего успешного ParseAndPreview).

Т.е. wizard **пишет исполняемый config.json всегда при Save**, используя последние известные outbounds, без перепарсинга подписок.

## 2. Все writes `state.json`

**Один call-site:** `SaveCurrentState()` в `ui/wizard/presentation/presenter_save.go:411`. Реализация — в `ui/wizard/models/wizard_state_file.go`.

Вызывается **после** успешного wizard Save в `saveStateAndShowSuccessDialog` (`:410–415`). Пишет весь `WizardModel` в JSON: `ParserConfig`, `CustomRules`, `SettingsVars`, `RulesLibraryMerged`.

**Ключевое:** config.json пишется **первым**, state.json — **вторым**. Они не в транзакции, и контракт неявный — сначала успешный sing-box check на config, потом запись state.

Версия state.json — v4 (см. SPEC 002).

## 3. Wizard Save pipeline

`SaveConfig()` в `ui/wizard/presentation/presenter_save.go:52–72`:

```
SaveConfig()
  ├── SyncGUIToModel()
  ├── validateSaveInput()
  └── executeSaveOperation() [goroutine]
      ├── ensureOutboundsParsed()
      │   ├── if AutoParseInProgress → wait (poll 200ms, timeout 60s)
      │   ├── else if GeneratedOutbounds empty → ParseAndPreview() sync
      │   └── → p.model.GeneratedOutbounds / GeneratedEndpoints filled
      ├── buildConfigForSave() → BuildTemplateConfig(model, false)
      ├── saveConfigFile(configText):
      │   ├── populateCheckText = PopulateParserMarkers(text, outbounds, endpoints)
      │   └── SaveConfigWithBackup(fileService, configText, populateCheckText)
      │       ├── prepareConfigText() — JSON normalize + gen clash_api secret
      │       ├── populateCheckText() — заполняет @ParserSTART/@ParserEND
      │       ├── write config-check.json → sing-box check
      │       ├── BackupFile() existing → .backup.NNN
      │       └── atomic: write .swap → Rename → config.json
      └── saveStateAndShowSuccessDialog():
          ├── StateService.SetTemplateDirty(true)
          ├── UIService.UpdateConfigStatusFunc()  ← broadcast
          ├── SaveCurrentState() → state.json
          └── showSaveSuccessDialog()
```

Т.е. один поток управления пишет **config → state → устанавливает dirty → broadcast**. Никакого разделения нет.

## 4. Подписки → outbounds: кэшируются или напрямую в config?

**Кэшируются в памяти, отдельно от шаблона**, но НЕ в state/config — в `WizardModel.GeneratedOutbounds[]`.

Поток:
1. `ParseAndPreview()` (wizard business layer): парсит подписки → nodes → JSON outbounds → **кэш в `WizardModel`**.
2. Wizard Save: **не перепарсивает**, просто берёт кэш и засовывает его в `config.json` через `PopulateParserMarkers`.
3. Parser run (кнопка Update / cron): **перепарсивает** подписки, пишет новые outbounds в `config.json` через `WriteToConfig`.

**Проблема:** кэш живёт в UI-объекте `WizardModel`. При закрытии окна Wizard он исчезает. Т.е. в новой архитектуре (где wizard пишет только state) outbounds-кэш надо физически персистить (отдельный файл или поле в state), иначе Build Config без парсера не соберёт рабочий конфиг.

## 5. `UpdateConfigStatusFunc` и друзья: карта broadcast

**Функции в `UIService`:**
```go
UpdateConfigStatusFunc   func()    // Core Dashboard: инфо о config + кнопка Update с `*`
UpdateCoreStatusFunc     func()    // Кнопки Start/Stop/Restart + статус процесса
UpdateTrayMenuFunc       func()    // Системный трей
RefreshAPIFunc           func()    // Перечитать список прокси из Clash API
UpdateParserProgressFunc func(float64, string)  // Progress bar парсера
AutoPingAfterConnectFunc func()    // Ping-all после connect
```

**Call-sites `UpdateConfigStatusFunc`:**
- `core/config_service.go:76` — после успешного RunParserProcess (сброс dirty)
- `ui/wizard/presentation/presenter_save.go:405` — после Wizard Save (выставление dirty)
- `ui/core_dashboard_tab.go:607` — при клике «Read Config»
- `ui/wizard/wizard.go:93` — при ошибке загрузки шаблона

Подписчик один: Core Dashboard `updateConfigInfo` — **перечитывает весь `config.json` с диска** и пересчитывает метрики. Дорогая операция, триггерится на каждый чих.

**Вывод:** типизированных событий нет, всё — один coarse-grained broadcast. Каждый Save/Parse триггерит полное re-read + re-render UI независимо от того, что именно поменялось.

## 6. Dirty-маркер: текущая реализация

Один bool-флаг в `core/services/state_service.go:32–36`:
```go
TemplateDirty      bool
TemplateDirtyMutex sync.RWMutex

SetTemplateDirty(bool), IsTemplateDirty() bool
```

- **Set(true)** в `presenter_save.go:401` — после wizard Save.
- **Set(false)** в `config_service.go:72` — после успешного parser run.
- **Read** в `ui/core_dashboard_tab.go:705` — для рисования `*` на Update-кнопке.

**Проблема (уже задокументирована в SPEC 043):** флаг взводится на **любой** wizard Save, даже если менялся только `tun_stack` или DNS (что требует Restart, а не Update). Семантически размазан.

## 7. Sing-box reload: триггеры

**Автоматического reload'а после изменения `config.json` нет.** Единственный способ применить новый config — **Restart** sing-box (kill → watcher стартует заново).

- `core/controller.go:540–545` — `KillSingBoxForRestart()` → `ProcessService.KillForRestart()` + `RestartRequestedByUser = true`.
- Watcher в `process_service.go:240–244, 316–325` — видит флаг, делает `Start(skipRunningCheck: true)`.
- При старте процесса (`process_service.go:85–94`) — перечитываем Clash API конфиг из `config.json` (BaseURL + Token).

Т.е. после wizard Save или parser run sing-box **продолжает работать со старым config**, пока пользователь вручную не нажмёт Restart. Нет никакой «авто-применимости».

## 8. Bootstrap: порядок в `main.go`

1. `main.go:41–44` — parse флагов (`-start`, `-tray`).
2. `main.go:48` — `NewAppController(appIcons)` — инициализация всех services:
   - FileService → RunningState → UIService → APIService (читает `config.json` для Clash API секрета) → StateService → ProcessService → ConfigService.
   - Стартует auto-update loop в goroutine (первый запуск через ~30 сек).
3. `main.go:54–80` — `LoadExternalLocales`, `LoadSettings` (из `bin/settings.json`).
4. `main.go:172–185` — `OnApplicationStarted`: читает `config.json` **один раз** для парсинга `@ParserConfig` блока. Парсер **не запускается**.
5. `main.go:188–194` — автостарт VPN, если `-start`.
6. `main.go:198–209` — скрыть окно при `-tray`.
7. `main.go:213–222` — создание UI (App → MainWindow → tabs).
8. `main.go:254` — `controller.UpdateUI()` — первое обновление.

`state.json` при старте **не читается** — лениво загружается только когда пользователь открывает Wizard.

## 9. Ссылки на ключевые файлы

| Компонент                         | Файл                                                              | Строки    |
|-----------------------------------|-------------------------------------------------------------------|-----------|
| `SaveConfigWithBackup`            | `ui/wizard/business/saver.go`                                     | 56–107    |
| `WriteToConfig`                   | `core/config/updater.go`                                          | 125–186   |
| `PopulateParserMarkers`           | `core/config/updater.go`                                          | 88–119    |
| `SaveConfig` (wizard entry)       | `ui/wizard/presentation/presenter_save.go`                        | 52–72     |
| `executeSaveOperation`            | `ui/wizard/presentation/presenter_save.go`                        | 116–148   |
| `ensureOutboundsParsed`           | `ui/wizard/presentation/presenter_save.go`                        | 171–211   |
| `SaveCurrentState`                | `ui/wizard/presentation/presenter_*.go`                           | 411       |
| `RunParserProcess`                | `core/config_service.go`                                          | 36–82     |
| `UpdateConfigFromSubscriptions`   | `core/config_service.go`                                          | 115+      |
| `StateService.SetTemplateDirty`   | `core/services/state_service.go`                                  | 79–83     |
| `updateConfigInfo`                | `ui/core_dashboard_tab.go`                                        | 698–750   |
| `NewAppController` (bootstrap)    | `core/controller.go`                                              | 158–271   |
| `OnApplicationStarted`            | `main.go`                                                         | 172–185   |

## 10. Болевые точки для split'а

1. **Кэш `GeneratedOutbounds` живёт в `WizardModel` (UI-слой).** При закрытии окна Wizard — теряется. В новой архитектуре cache должен переехать либо в `StateService` (в памяти), либо в отдельный файл (`outbounds.cache.json`). Иначе `BuildConfig` без парсера не соберёт валидный конфиг.

2. **`ensureOutboundsParsed()` блокирующе poll'ит 60 секунд.** Этот паттерн «wizard Save ждёт parser» плохо переносится в мир асинхронных событий. Нужен явный контракт: либо wizard Save **всегда** триггерит parser, либо wizard Save **никогда** не ждёт — и пользователь явно видит «build нужен».

3. **`UpdateConfigStatusFunc` — одна точка для всего.** Разделение даст минимум три разных сигнала (StateChanged, ConfigBuilt, ProcessStateChanged). Существующий код ожидает, что одна callback'а решает всё — придётся переписывать подписчиков.

4. **Миграция `state.json` v4 → v5.** Нужен dry-run: на старом файле проверяется загрузка, новая запись ≥ v5, чтение старого — без падений.

5. **Wizard косвенно владеет config.json.** Сейчас wizard — единственный компонент, знающий шаблон template + как вставить outbounds в маркеры. При разделении `BuildConfig` становится отдельным пакетом вне `ui/wizard/`, а wizard только даёт state.

6. **Parser ездит в `config.json` через `@ParserConfig` блок.** `UpdateConfigFromSubscriptions` читает parser-конфиг прямо из config.json, фетчит по нему, дозаписывает обратно outbounds. После split'а parser должен читать из `state.json` (или из отдельного контракта), а писать в `outbounds.cache.*` + триггерить `BuildConfig`, который уже собирает итоговый config.

7. **Транзакционность `SetTemplateDirty` + `SaveCurrentState`.** Сейчас dirty-флаг ставится до записи state-файла. В асинхронной модели нужно гарантировать, что dirty становится видимым только когда state-файл физически на диске и reload-сигнал отправлен.

---

**Итог фазы 1:** архитектура сейчас = монолит «wizard Save пишет config + state + триггерит broadcast». Нет независимого `BuildConfig`, нет отдельного outbounds-кэша, нет типизированных событий. Именно это и предстоит расщепить в SPEC 045.
