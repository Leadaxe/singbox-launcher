# PLAN 162 — паспорт платформы, команды демона, troubleshooting аккордеоном

Якоря кода — [CODEMAP 161](../161-F-C-MACHINE_SERVICE_WINDOW/CODEMAP.md)
§13–15; срез develop `d486123e`. Решения сверх SPEC помечены **[решение]**.

## 1. Слои

```
internal/lxdclient                core                               ui
──────────────────────            ─────────────────────────────      ──────────────────────────────
client.go  InfoData +5 полей      core_build.go   суффиксы версий    service_model.go    snapshot: Commands, CommandsState
commands.go Commands/Command/     service_recipes.go                 service_commands.go  строка шапки: селектор, Run, лог,
           RunCommand (новый)        ServicePlatform.Platform,                            машина состояний update
                                     ServiceInit openrc,              service_window.go   шапка + Accordion вместо AppTabs
                                     ServiceOrService,                service_tabs.go     секции = прежние вкладки + Suggested
                                     CommandsRecipe()                 service_source_*.go commander, паспорт платформы
                                  core_download_target.go            machine_edit_window.go селекторы неактивны
                                     ассет по Platform               machine_host_window.go mismatch только без паспорта
                                  services/lxd_remote_registry.go
                                     RemotePassport +5, SetPlatform
                                     из паспорта, Commands* методы
                                  daemon_manager.go  паспорт local
```

Принцип 161 сохраняется: всё табличное — в `core` без Fyne; окно раскладывает
снапшот. Команды — отдельный интерфейс источника `serviceCommander`, сетевые
вызовы только в горутинах, результат — через `fyne.Do`.

## 2. `internal/lxdclient`

### 2.1 `InfoData`
Поля `GOOS string json:"goos"`, `GOARCH json:"goarch"`, `Platform
json:"platform"`, `InitSystem json:"init_system"`, `Service bool
json:"service"`. Комментарий: lx.16+ (SPEC 127), у старых ядер пусто.

### 2.2 `commands.go` (новый)
```go
type Command struct {
    Name, Command, State string      // state: idle|running|done|lost
    StartedAt, FinishedAt string     // RFC3339 или ""
    ExitCode *int                    // null пока нет
    Client, Log string               // Log — только в GetCommand
}
var ErrCommandsUnsupported = errors.New("lxdclient: commands: not supported by this daemon") // 404 на списке
var ErrCommandNotFound   = errors.New(...)  // 404 по имени
type ErrCommandRunning struct{ Current Command }  // 409
func (c *Client) CommandsCtx(ctx) ([]Command, error)
func (c *Client) CommandCtx(ctx, name) (Command, error)
func (c *Client) RunCommandCtx(ctx, name) (Command, error) // POST без тела, 202
```
Тело POST — `nil`, Content-Type не ставится. 400 → обычная ошибка (в
лаунчере недостижимо, но не глотать). Имя команды валидируется клиентом по
`^[a-z0-9][a-z0-9_-]{0,31}$` до запроса (защита от подстановки в путь).

## 3. `core`

### 3.1 `core_build.go`
`parseCoreBuild`: после `lx.N` допускается только `-rc<K>` / `-rc.<K>`
(регистр нижний); иначе `ok=false`. Тест `TestCompareCoreVersion`
(существующий файл теста версий) дополняется `1.14.3-lx.15-test128` →
Unknown, `lx.15-rc.2` → Older против `lx.15`.

### 3.2 `service_recipes.go`
- `ServiceInitOpenRC ServiceInit = "openrc"` — только для шапки и `openrc`
  в `commands`-рецепте; `BuildServiceRecipes` для openrc отдаёт пустые
  `Steps` и `Unsupported = true` **[решение]** (UI пишет строку «no recipes
  for openrc»).
- `ServicePlatform` получает `Platform string` (суффикс ассета из паспорта,
  "" — нет) и `ServiceKnown bool` + `Service bool` (паспорт сообщил, под
  init ли процесс).
- `ParseInitSystem(s string) (ServiceInit, bool)` — из строки паспорта:
  procd/systemd/openrc/launchd/scm; иное → false.
- `NormalizeInitChoice` — без изменений; источник вызывает его только когда
  паспорт init не дал.
- `CommandsRecipe(p ServicePlatform, paths ServicePaths) (json string, ok bool)`
  — фрагмент §4.5 SPEC. Чистая функция, тестируется в том же тесте волны.
- Новые `ServiceStep` id: `ServiceStepEditDaemonJSON` (`vi
  <state_dir>/daemon.json`).

### 3.3 `core_download_target.go`, `core_downloader.go`
- `DownloadCoreForTarget(ctx, version, goos, goarch, platform, progress)`:
  если `platform != ""` — суффикс `platform + ".tar.gz"` (`.zip` при
  `goos == "windows"`), иначе `SingboxAssetSuffixFor`. Новая функция
  `assetSuffixFor(goos, goarch, platform string) string` рядом с
  `SingboxAssetSuffixFor`.
- `TargetCoreFileName` без изменений (имя файла в Downloads по goos/goarch).

### 3.4 `services/lxd_remote_registry.go`
- `RemotePassport` + `GOOS, GOARCH, Platform, InitSystem string; Service *bool`.
  `samePassportFields` учитывает их.
- `healthCtx`: после `InfoCtx` — если `info.GOOS != "" && info.GOARCH != ""`
  и отличаются от записи → `SetPlatform` + `debuglog.InfoLog`. Если
  `info.InitSystem` распознан (`core.ParseInitSystem`) и отличается от
  `d.InitSystem` → `SetInitSystem` (валидатор расширяется на
  openrc/launchd/scm).
- `RemoteHealth` + те же поля (для окна при живом ответе).
- Методы `Commands(id) ([]lxdclient.Command, error)`, `Command(id, name)`,
  `RunCommand(id, name)` — через `adminClient`, блокирующие, из горутин.
  `CommandsWithin(id, timeout)` для heartbeat.
- `PassportPlatformKnown(d RemoteDaemon) bool` — есть ли паспорт с
  goos/goarch (для Edit и host-mismatch).

### 3.5 `daemon_manager.go` (local)
`daemonPassportCache` хранит `InfoData` целиком — поля приезжают сами.
`DaemonUIStatus.Passport` — тот же тип, ничего не добавлять. Доступ к
loopback-клиенту для команд: `ac.DaemonAdminClient() (*lxdclient.Client,
error)` **[решение]** — обёртка над существующим построением клиента в
`daemon_manager.go` (см. `DaemonStatusSnapshot`), только под тегом
`darwin || (windows && !386)`.

## 4. `ui`: модель

### 4.1 `serviceSnapshot`
```go
Platform core.ServicePlatform   // + Platform, ServiceKnown, Service
InitChoice bool                 // false, когда init из паспорта
CommandsState serviceCommandsState // Unknown | Unsupported | Ready | Down
Commands []lxdclient.Command    // отсортированы демоном
```
`serviceSnapshotEqual` сравнивает `Commands` по name/state/exit/finished
(лог не входит — он живёт в виджете).

### 4.2 `serviceSource` + `serviceCommander`
```go
type serviceCommander interface {
    Commands(done func([]lxdclient.Command, error))        // список
    Command(name string, done func(lxdclient.Command, error))
    RunCommand(name string, done func(lxdclient.Command, error))
    Reconnect(done func(ok bool))   // для update: HealthWithin + перечитать паспорт
    Version() string                // текущая версия из кэша
}
```
`serviceSource` получает `Commander() serviceCommander` (nil — команд нет,
например Windows-local). Удалённый — через реестр; локальный — через
`DaemonAdminClient`. Список команд удалённый источник кладёт в кэш панели
при heartbeat (`machineListPanel.commands[id]`, обновляется в
`pollActive` после Health, тем же `heartbeatTimeout`), локальный — в свой
тик после `DaemonStatusSnapshot`. Окно читает кэш без сети.

### 4.3 Диагноз
`serviceDiagnosis` возвращает `map[serviceTab]string` с **полным текстом
заголовка** вместо глифа: `"✖ Not running — connection refused"`. Первая
проблемная секция — как сейчас; нет проблем → `serviceTabNone` (ничего не
раскрывать).

## 5. `ui`: окно

### 5.1 Шапка (`service_window.go`)
Порядок строк: платформа + guides; версия ядра; состояние; `ServiceNote`;
**строка команд** (`service_commands.go`); `Troubleshooting · commands run
via …`. Селектор init — только при `InitChoice`.

### 5.2 Accordion вместо AppTabs
- `v.tabs *container.AppTabs` → `v.acc *widget.Accordion`,
  `v.items map[serviceTab]*widget.AccordionItem`. Содержимое каждой секции —
  `container.NewVScroll` с `SetMinSize(0, serviceTabScrollMinHeight)`, внутри
  `Padded(build(c))`, как сейчас. **[решение]** Высота секции ограничена
  прокруткой окна: `v.root = Border(header, nil, nil, nil, VScroll(acc))`,
  а минимум прокрутки секции — 160, как у вкладок; окно само не растёт.
- `selectTab(t)` → `acc.Open(index)`; `serviceTabNone` → `acc.CloseAll()`.
- Заголовки из §4.3; `acc.Refresh()` после перестройки содержимого; индекс
  раскрытой секции сохраняется до и восстанавливается после (Accordion
  закрывает всё при `Refresh`? — проверить по исходникам Fyne 2.x; если
  закрывает, хранить `openIdx` и звать `Open` снова).
- `goTo(label, t)` текст «Open Core» и т. п.

### 5.3 `service_commands.go` (новый)
Виджет `serviceCommandBar`, живёт в `serviceView` один на окно:
```go
type serviceCommandBar struct {
    v        *serviceView
    sel      *widget.Select
    run      *widget.Button
    cmdText  *widget.Label      // текст команды
    outcome  *widget.Label      // исход / подсказка
    logBox   *widget.Entry      // read-only, моно, скрыт пока нет прогона
    toggle   *widget.Button     // ▴/▾ лог
    active   string             // имя команды под наблюдением
    stop     chan struct{}      // остановка опроса
    update   *serviceUpdateRun  // nil вне прогона update
}
func (b *serviceCommandBar) apply(s serviceSnapshot) // перерисовать по снапшоту
func (b *serviceCommandBar) onRun()
func (b *serviceCommandBar) watch(name string)       // GET раз в секунду до done|lost
```
- `apply`: при `CommandsState != Ready` — селектор с одной записью
  `not available`, Run `Disable()`, outcome — подсказка. Иначе список имён;
  выбранное имя сохраняется между тиками; если выбранное исчезло — первое.
- `onRun`: `update` → `confirmUpdate` → `runUpdate`; иначе
  `RunCommand` → 202/409 → `watch`.
- `watch`: горутина, тикер 1 с, `Command(name)`; каждый ответ → `fyne.Do`:
  лог в `logBox`, исход в `outcome`; `done|lost` → стоп. Ошибка GET вне
  update → стоп, outcome `✖ <err>`.
- `check-update`: `outcome` = строка лога, начинающаяся с `update:
  installed` (регулярка `installed (\S+).*available (\S+)`), иначе общий вид.

### 5.4 `serviceUpdateRun` (там же)
Состояния `posted → polling → reconnecting → verifying → finished`.
- `polling`: как `watch`; ошибка GET или `Reachable=false` в снапшоте →
  `reconnecting`, `since = now`.
- `reconnecting`: цикл `Commander().Reconnect` с задержками 2, 5, 10, 10… с
  до 90 с; строка `Restarting the daemon — reconnecting… %d s` обновляется
  каждую секунду. Панель машин ничего не знает об ожидании: строка списка
  краснеет как при любом обрыве (решение владельца).
- `verifying`: паспорт перечитан (`Reconnect` зовёт `HealthWithin`, который
  обновляет кэш), `Command("update")` → исход по SPEC §4.3; 404 (список) →
  `CommandsState = Unsupported`, строка «Updated to <ver>. Commands are not
  available…».
- Таймаут → `finished` с ошибкой и `selectTab(serviceTabNotRunning)`.
- Закрытие окна (`dispose`) останавливает все горутины через `stop`.

### 5.5 Секции (`service_tabs.go`)
- Core: первой строкой подсказка про `update`, если
  `CommandsState == Ready` и команда есть.
- `service:false`: Not running — шаг «Install the service» первым (переиспользовать
  содержимое `core_scratch`), остальные шаги после.
- openrc: Not running и Core — одна строка `No recipes for openrc yet — see
  the lxd guide` + guides.
- Новая `buildSuggestedCommandsTab`: `CommandsRecipe` в многострочном
  read-only Entry, ⧉, шаги `ServiceStepEditDaemonJSON` и `ServiceStepRestart`,
  подпись.

### 5.6 Add/Edit, host window
- `machine_edit_window.go`: при `PassportPlatformKnown(d)` селекторы
  `Disable()`, под ними label `reported by the daemon`; Save платформу не
  пишет.
- `machine_host_window.go:673`: `hostPlatformMismatch` только если
  `!PassportPlatformKnown(d)`.
- `machine_list_panel.go`: строка платформы читает запись (уже так).

### 5.7 Heartbeat
`pollActive`: после успешного `HealthWithin` — `CommandsWithin(id,
heartbeatTimeout)` → `p.commands[id]` (+ состояние Unsupported/Ready). Ошибка
транспорта → состояние не меняется (кэш старый). Делается только когда окно
Service этой машины открыто **[решение]** (`serviceWindows[key]` есть) —
иначе лишний запрос каждые 5 с на всех машинах.

## 6. Локаль
Новые ключи — все через `locale.T`/`Tf`, длинные в `const` файла. `ru.json`
правится в конце волны по `go run ./tools/l10n/l10n_check`. Имена команд и
JSON — `l10n-exempt`.

## 7. Тесты
Один именованный: расширение существующего теста версий ядра
(`TestCompareCoreVersion`) + `TestCommandsRecipe` в том же пакете `core`
(табличный: procd/systemd/launchd/openrc/scm, с паспортом и без). Для
`lxdclient.commands.go` — `httptest` на 404/409/202 в одном тесте
`TestClientCommands` (data-критично: разбор `exit_code: null`).

## 8. Вне рамок
Debug API для команд; история прогонов; `/admin/host`; Windows-служба как
исполнитель команд; изменение `RequiredCoreVersion`; контракт LxBox (реестр
машин в него не входит).
