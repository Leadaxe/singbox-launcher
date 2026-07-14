# SPEC 080-F-N — XDG DATA DIR + PORTABLE-РЕЖИМ

> Связано: [issue #85](https://github.com/Leadaxe/singbox-launcher/issues/85). Расширяет идею из issue (XDG-пути для writable-данных), но добавляет два пункта, которых в issue не было и которые важны для существующих пользователей: **явный portable-режим** (кнопка в настройках + авто-детект) и **безболезненную миграцию** старых данных, а не «пересоздать с нуля».

## Цель

Сейчас всё runtime-состояние пишется рядом с бинарником: `{ExecDir}/logs/`, `{ExecDir}/bin/wizard_states/`, `{ExecDir}/bin/subscriptions/`, `{ExecDir}/bin/config.json`, скачанное ядро и т.д. (см. `internal/platform/platform_common.go`, `core/services/file_service.go:86-96`). На системах с read-only каталогом бинарника (**NixOS**, **Guix**, **Flatpak**, **snap**) старт падает:

```
NewFileService: cannot create directories: mkdir /nix/store/.../bin/logs: read-only file system
```

(буквально `fmt.Errorf` из `core/services/file_service.go:89` + текст ядра).

Нужно:
1. По умолчанию писать writable-данные в **XDG-совместимый путь** (`os.UserConfigDir()/singbox-launcher`), оставив поставляемые read-only ресурсы (locales, встроенный `wizard_template.json`, bundled-ядро) рядом с бинарником.
2. Сохранить **portable-режим** (Windows «с флешки» — исходный сценарий, см. ответ владельца в issue): всё рядом с бинарником, ничего в профиль пользователя.
3. **Не потерять данные** существующих пользователей при обновлении: при первом старте новой версии данные из `ExecDir` переезжают/подхватываются автоматически.

## Контекст и ключевая проблема дизайна

`settings.json` (флаги языка/пинга/подписок — `internal/locale/settings.go`) сам лежит в `{binDir}/settings.json`, т.е. **внутри** writable-области. Значит флаг «portable» нельзя хранить в settings.json: чтобы прочитать settings.json, надо уже знать, портабл мы или нет (где `binDir`). **Курица-яйцо.**

Поэтому режим определяется **до** чтения настроек, по факту наличия данных на диске — ровно как просил владелец: **не по marker-файлу, а по уже существующей папке состояний**, которая есть у всех текущих пользователей.

### Правило выбора режима (детект при старте, в `NewFileService`)

`UserDataDir` вычисляется так:

```
execDir   := <override env | filepath.Dir(os.Executable())>
xdgDir    := os.UserConfigDir()/singbox-launcher

portable  := isPortable(execDir, xdgDir)
dataDir   := execDir  if portable  else  xdgDir
```

`isPortable(execDir, xdgDir)`:

1. **Override env** `SINGBOX_LAUNCHER_PORTABLE=1` → `true`; `=0` → `false` (жёсткий приоритет, для тестов/CI/кастомных сборок).
2. **Существующая папка состояний рядом с бинарником** — `{execDir}/bin/wizard_states/` существует **И** `xdgDir` ещё не инициализирован → `true`.
   Это и есть «по папке состояний»: у текущих пользователей `wizard_states/` уже лежит рядом с exe → они молча остаются portable, ничего не переезжает, обновление незаметно.
3. **`xdgDir` уже инициализирован** (есть `{xdgDir}/bin/wizard_states/` или маркер переезда) → `false` (стационар, данные уже там).
4. Иначе (чистая установка, ни там ни там) → `false` — **новый дефолт XDG**. На read-only Nix это и спасает от падения.

> Итог: **существующие** инсталляции остаются portable без действий пользователя; **новые** — едут в XDG; **Nix/Flatpak** перестают падать. Пользователь может переключиться кнопкой (см. ниже).

### Read-only seed (всегда из `execDir`, не из `dataDir`)

Остаются рядом с бинарником и читаются через новые `*FromExec`-функции:
- встроенный `wizard_template.json` (`core/template/loader.go:226`) — но **скачанный/изменённый** template пишется в `dataDir`;
- `locales/` (`internal/locale/locale.go`, `main.go:68` `LoadExternalLocales`) — каталог переводов поставки;
- bundled-ядро `bin/sing-box` как **источник** установки (`SingboxBundledPath`), при этом **скачанное** ядро — в `dataDir`.

## Объём

### 1. `internal/platform/platform_common.go` — расщепление путей

Сейчас все `Get*` принимают `execDir`. Разделяем на два класса:

**Writable (теперь принимают `dataDir`)** — семантика прежняя, аргумент переименовать в `dataDir`:
`GetConfigPath`, `GetBinDir`, `GetRuleSetsDir`, `GetWizardStatesDir`, `GetWizardStatePath`, `GetOutboundsCachePath`, `GetSubscriptionsDir`, `GetLogsDir`.

**Read-only seed (новые `*FromExec`)**:
- `GetWizardTemplatePathFromExec(execDir)` — встроенный template поставки.
- `GetBinDirFromExec(execDir)` — для locales / bundled-ядра.
- (`GetWizardTemplatePath(dataDir)` остаётся для **скачанного** template под dataDir — template-resolver сначала ищет в dataDir, фолбэк на FromExec; см. §5.)

`EnsureDirectories(dataDir)` — создаёт writable-каталоги под `dataDir` (`logs/`, `bin/`, `bin/rule-sets/`, `bin/wizard_states/`, `bin/subscriptions/`).

Новое:
- `GetUserDataDir() (string, error)` — `os.UserConfigDir()/singbox-launcher` (константа `AppDirName = "singbox-launcher"`).
- `IsPortable(execDir, dataDir) bool` — правило выше.
- `GetMigrationMarkerPath(dataDir)` — `{dataDir}/.migrated` (флаг «переезд из ExecDir выполнен», см. §3).

### 2. `core/services/file_service.go` — `FileService.DataDir`

```go
type FileService struct {
    ExecDir  string // read-only seed: locales, bundled template, bundled core
    DataDir  string // writable: logs, config, states, subs, cache, downloaded core
    Portable bool   // true → DataDir == ExecDir
    ...
}
```

`NewFileService()`:
1. `execDir` (учесть env-override, см. §4).
2. `xdgDir, _ := GetUserDataDir()`.
3. `portable := IsPortable(execDir, xdgDir)`; `dataDir := execDir if portable else xdgDir`.
4. Если **стационар** и переезд ещё не выполнен (`!portable && !markerExists(dataDir)`) → `migrateFromExec(execDir, dataDir)` (§3).
5. `seedShippedData(execDir, dataDir)` — идемпотентно (§5).
6. `EnsureDirectories(dataDir)`.
7. Все writable-пути — от `dataDir`; `SingboxBundledPath` — от `execDir`; `SingboxPath` resolver: сначала `{dataDir}/bin/sing-box` (скачанное), фолбэк `{execDir}/bin/sing-box` (bundled).
8. `OpenLogFiles`/`ReopenChildLogFile` — заменить `fs.ExecDir` → `fs.DataDir` (строки 107/114/122/145).

### 3. Миграция ExecDir → DataDir (первый стационарный старт)

`migrateFromExec(execDir, dataDir)`:
- Срабатывает один раз: страж — маркер `{dataDir}/.migrated`.
- **Копирует** (не перемещает — старые данные остаются как бэкап) из `{execDir}` в `{dataDir}`, если источник существует и приёмник пуст:
  `bin/config.json`, `bin/wizard_states/`, `bin/subscriptions/`, `bin/rule-sets/`, `bin/outbounds.cache.json`, `bin/settings.json`, скачанный `bin/sing-box`, скачанный `bin/wizard_template.json`, `logs/` — опционально (логи можно не тащить).
- По завершении пишет `.migrated` (с датой/версией внутри — для диагностики).
- Идемпотентно и безопасно на read-only `execDir`: чтение источника разрешено, пишем только в `dataDir`. Если `execDir` пуст (Nix-первый-старт) — копировать нечего, просто ставим маркер.

> Это закрывает «дыру» из issue: там было *«state пересоздастся, template перекачается»* — здесь данные пользователя (подписки, состояния визарда, настройки) **сохраняются**.

### 4. Кнопка Portable в настройках + переезд между режимами

**UI** (`ui/settings_tab.go`, новая секция «Storage» рядом с Connection/Language; паттерн как `autoUpdateCheck` на :51 — `widget.NewCheck`):
- Чекбокс **«Portable mode (store data next to the app)»**.
  - Состояние = `fs.Portable`.
  - Disabled + тултип-объяснение, если `execDir` read-only (Nix/Flatpak): portable там физически невозможен — показываем причину.
- Под чекбоксом — read-only строка с текущим `DataDir` и кнопка **«Open data folder»** (как уже открываются папки в `ui/diagnostics_tab.go`).

**Переключение** (по смыслу — «механизм переезда portable ↔ стационар», который просил владелец):
- Тоггл **не** меняет пути в текущем процессе мгновенно (FileService живёт весь сеанс). Вместо этого:
  1. Показать `dialog.ShowConfirm`: «Данные будут перенесены в `<target>`. Приложение перезапустится». При активной VPN-сессии — предупредить/заблокировать (не дёргать живой процесс — см. эксплуатационное правило проекта про sing-box).
  2. Записать **намерение переезда** в файл-флаг, который читается до settings.json: `{targetDataDir}/.pending-mode` или симметрично — потому что settings.json недоступен на этом этапе (см. «курица-яйцо»). Конкретно:
     - **stationary → portable**: скопировать `{xdgDir}` → `{execDir}`, поставить маркер; при следующем старте `IsPortable` вернёт true по правилу 2.
     - **portable → stationary**: скопировать `{execDir}` → `{xdgDir}`, поставить `{xdgDir}/.migrated`; правило 3 уведёт в XDG.
  3. Перезапустить приложение (graceful restart) либо попросить пользователя перезапустить вручную, если авто-рестарт сложен.
- Старые данные при переезде **копируются, не удаляются** (бэкап). Можно показать тост «Old data left at `<src>`, safe to delete».

> Механика «по папке состояний» здесь самосогласована: после копирования в целевой каталог появляется его `bin/wizard_states/`, и детект (§1) сам выберет нужный режим на следующем старте — без хранения булева флага в недоступном settings.json.

### 5. Seeding поставляемых ресурсов

`seedShippedData(execDir, dataDir)` (идемпотентно, только в portable-стационар XDG; в portable — no-op, всё уже на месте):
- если `{dataDir}/bin/wizard_template.json` отсутствует, **не** копировать (template resolver сам фолбэкнётся на bundled FromExec) — копируем только когда появляется *скачанный/изменённый* template.
- locales **не** копируем — читаем напрямую из `execDir` (`LoadExternalLocales(GetLocaleDirFromExec(execDir))`).

Template resolver (новый порядок в `core/template/loader.go` и `core/config/varsubst.go:161`):
1. `{dataDir}/bin/wizard_template.json` (скачанный/кастомный) — если есть;
2. иначе `{execDir}/bin/wizard_template.json` (bundled seed).

### 6. Env-override

```
SINGBOX_LAUNCHER_EXEC_DIR   — переопределяет execDir (как сейчас по факту считается; тесты, кастом).
SINGBOX_LAUNCHER_DATA_DIR   — переопределяет dataDir напрямую (CI, IoT, кастомные установки).
SINGBOX_LAUNCHER_PORTABLE   — 1/0, жёсткий выбор режима (приоритет над авто-детектом).
```

### 7. Callers — замена `ExecDir` → `DataDir` (writable) / `*FromExec` (seed)

`FileService` отдаёт оба пути; адаптер для configurator (`ui/configurator/business/file_service_adapter.go`) получает метод `DataDir() string`, интерфейс `FileServiceInterface` (`ui/configurator/business/interfaces.go:35`) расширяется. `StateStore` (`state_store.go:44`) переходит на `fileService.DataDir()`.

**Writable → `DataDir`:**
| Файл | Что меняется |
|------|-------------|
| `main.go` | `GetWizardStatePath`, `GetBinDir`, `GetLogsDir` → DataDir |
| `core/log_level.go` | `GetWizardStatePath` |
| `core/auto_update.go` | `GetWizardStatePath` |
| `core/config_service.go` | `GetConfigPath`, `GetSubscriptionsDir`, `GetBinDir` |
| `core/rebuild.go` | `GetWizardStatePath`, `GetOutboundsCachePath` |
| `core/rebuild_raw_cache.go` | `GetOutboundsCachePath`, `GetSubscriptionsDir` |
| `core/process_service.go` | `GetBinDir` (workdir), пути логов |
| `core/config_service_subscriptions.go` | `GetSubscriptionsDir`, `GetWizardStatePath`, `GetWizardStatesDir` |
| `core/snapshot/snapshot.go` | config/state/cache/subs → DataDir; bundled template → FromExec |
| `core/debugapi_wiring.go` | `GetWizardStatePath` |
| `ui/core_dashboard_tab.go` / `_status.go` | `GetWizardStatePath`, `GetWizardStatesDir` |
| `ui/configurator/configurator.go` | binDir → DataDir; template load → resolver |
| `ui/configurator/business/state_store.go` | `GetWizardStatesDir(DataDir)` |
| `ui/configurator/presentation/presenter_state.go`, `presenter_save.go` | statesDir → DataDir |
| `ui/configurator/tabs/source_edit_*.go` | `GetSubscriptionsDir(DataDir)` |
| `ui/traffic_bootstrap.go` | `GetLogsDir`, child-bin → DataDir |
| `ui/diagnostics_tab.go` | `GetLogsDir(DataDir)`; locale-листинг → FromExec |
| `ui/clash_api_tab.go`, `ui/settings_tab.go` | binDir для settings.json → DataDir |
| `internal/locale` callers | `settings.json` (`SaveSettings`/`LoadSettings`) → `GetBinDir(DataDir)` |

**Read-only seed → `*FromExec`:**
| Файл | Что меняется |
|------|-------------|
| `main.go:68` | locales: `LoadExternalLocales(GetLocaleDirFromExec(execDir))` |
| `ui/settings_tab.go:189` | скачивание локалей в seed-каталог |
| `core/template/loader.go:226` | template resolver (dataDir → fallback execDir) |
| `core/config/varsubst.go:161` | `GetWizardTemplatePath` через resolver |

### 8. Тесты

- `internal/platform`: `IsPortable` — таблица случаев (env=1/0, есть `wizard_states` в execDir, инициализирован xdg, чистая установка). `GetUserDataDir`, `*FromExec`.
- `core/services`: `migrateFromExec` — копирует подписки/состояния/настройки; идемпотентность по `.migrated`; пустой execDir; read-only execDir (источник readonly, dataDir writable).
- Обновить существующие, где жёстко `ExecDir`: `core/rebuild_raw_cache_test.go`, `core/refresh_meta_test.go`, `core/snapshot/snapshot_test.go`, `core/debugapi/snapshot_test.go`, `core/template_migration_test.go`, `internal/locale/settings_test.go` — через `SINGBOX_LAUNCHER_DATA_DIR`/временный каталог.
- Template resolver: dataDir-приоритет, fallback на bundled.

### 9. Локали + Release notes

- en + ru строки: «Portable mode…», «Open data folder», тултип-причина для read-only, диалоги переезда.
- Release notes: новое поведение по умолчанию (XDG для новых установок), portable сохранён и переключаем, существующие данные мигрируют автоматически.

## Вне объёма

- Перенос **bundled-ядра/template** в XDG — они остаются seed рядом с бинарником; в dataDir едет только *скачанное*.
- Полноценная Flatpak/snap-упаковка (манифесты, sandbox-портал) — здесь только чтобы приложение **не падало** и писало в правильный путь.
- Шифрование/синхронизация dataDir, мульти-профили.
- Авто-удаление старых данных после переезда (оставляем как бэкап; максимум — подсказка).

## Критерии приёмки

1. **Существующий пользователь** (Win/mac/Linux, данные рядом с exe): обновился на новую версию → остаётся portable, все подписки/состояния/настройки на месте, ничего не переехало, поведение прежнее.
2. **Новая установка** на обычной ОС: данные пишутся в `os.UserConfigDir()/singbox-launcher`, рядом с бинарником ничего writable не создаётся (кроме чтения seed).
3. **NixOS/Flatpak/snap** (read-only каталог бинарника): приложение **стартует без падения**, всё пишет в XDG; чекбокс Portable показан disabled с понятной причиной.
4. Кнопка **Portable mode** в настройках: переключение запускает диалог подтверждения, копирует данные в целевой каталог, после перезапуска приложение использует новый режим; старые данные остаются как бэкап.
5. **Open data folder** открывает фактический `DataDir`.
6. `SINGBOX_LAUNCHER_DATA_DIR` / `SINGBOX_LAUNCHER_PORTABLE` переопределяют поведение (для CI/кастома).
7. Скачанный template/ядро берутся из `DataDir`; bundled — из `ExecDir` (resolver с фолбэком).
8. Активная VPN-сессия не нарушается переключением режима (никаких операций над живым sing-box; при необходимости — блок/предупреждение).
9. `go build ./... && go test ./... && go vet ./...` — зелёные.

## Порядок реализации (предлагаемый)

1. `platform_common.go`: новые функции (`GetUserDataDir`, `IsPortable`, `*FromExec`, `EnsureDirectories(dataDir)`), без смены callers — параллельно со старыми.
2. `FileService`: `DataDir`/`Portable`, детект, `migrateFromExec`, seeding, resolver ядра/template.
3. Перевести callers (writable → DataDir, seed → FromExec) пачками по слоям: core → ui → configurator → locale.
4. Template resolver в loader/varsubst.
5. UI-секция Storage + переезд + диалоги + локали.
6. Тесты + release notes.
