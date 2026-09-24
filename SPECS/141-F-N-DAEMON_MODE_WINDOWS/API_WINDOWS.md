# SPEC 141 · API Windows-части для панели Local (контракт UI)

Ветка `spec-141-windows`. Сигнатуры зафиксированы, тела Windows — заглушки
(`errDaemonWindowsPending`) до фазы 2; движок на Windows закрыт
`daemonEngineAvailable`. macOS-реализации операций рабочие (Terminal).

## Состояние для панели (общее, macOS и Windows x64/arm64)

- `ac.DaemonStatusSnapshot() DaemonUIStatus` — из горутины (сеть). Поля как на
  macOS; `Service DaemonServiceCheck` — вердикт.
- `DaemonServiceCheck.State`: `not_installed`, `unsafe`, `stale`,
  `not_running`, `process_stale`, `ok`, `core_too_old` — те же, что на macOS.
  Методы `NeedsInstall()`, `NeedsBootstrap()` (= NotRunning), `CopyUsable()`,
  `InstallSupported()`.
- Новое: `MismatchFile string`, `ExtraFile bool` — Stale по `libcronet.dll`
  или лишнему файлу в каталоге копии (пусто — расходится сам exe).
- `LaunchdState` на Windows — `CurrentState` SCM (`stopped`, `start_pending`…).
- `core.DaemonServiceCoreHint(version)` — подсказка при CoreTooOld.

## Команды для показа и Copy (существующие, на Windows — рендер PowerShell)

`& '<путь>' <args>`, `'` → `''`. Исполняется argv, не строка.

| Строка панели | Функция |
|---|---|
| Install or update | `ac.DaemonInstallCommand() (string, error)` — err = старое ядро |
| Start the service (NotRunning) | `ac.DaemonBootstrapCommand()` — теперь общий; Windows: `& '…\System32\sc.exe' start sing-box-lxd` |
| Fresh invite | `ac.DaemonRepairCommand()` |
| Uninstall (`--keep-copy [--purge]`) | `ac.DaemonUninstallCommand(purge)` |
| Remove all data | `ac.DaemonUninstallHint()` |
| Kickstart | `DaemonKickstartCommand()` — на Windows `""` (строки нет) |
| Show secret | `DaemonShowSecretCommand()` — Windows: фаза 2, пока `""` |

Тип `core.DaemonCommand{Binary, Args}` + `String()` + `IsZero()` — для
`DaemonRunResult.Command`; сборщики на него переходят в фазе 2.

## Операции (общие сигнатуры; блокируют — звать из горутины)

```go
const core.DaemonOpsElevated bool // Windows true: «Run as administrator»; macOS false: «Run in Terminal»
func (ac) DaemonInstallOrUpdate() DaemonRunResult
func (ac) DaemonStartService() DaemonRunResult          // NotRunning
func (ac) DaemonFreshInvite() DaemonRunResult
func (ac) DaemonUninstallService(keepCopy, purge bool) DaemonRunResult // Remove all data: (false, true)
func (ac) DaemonCopyOnly() DaemonRunResult              // classic, службы нет
func core.DaemonRunWaitingText() string                // «Waiting for the administrator command…»
```

`DaemonRunResult`: `Op`, `Command` (для Copy), `InTerminal` (macOS),
`CoreHint`, `Cancelled` (отказ UAC — диалог не закрывать), `Err`, `Exited`,
`ExitCode`, `TimedOut` (120 с, процесс жив), `Paired`, `Warnings
[]DaemonServiceWarning{Code, Text}` (+`DisplayText()`), `StatusChecked`,
`StatusCode` (install код 1 → `--service=status`), `NoInvite` +
`FreshInvite` (следующий шаг — второе окно UAC), `Service` (пересчитанный
вердикт). Методы: `Succeeded()`, `StatusText()` — готовая
локализованная строка §5.3 с предупреждениями сайдкара.

Панель: на время операции — `DaemonRunWaitingText()`, кнопки выключены;
после — `StatusText()`; при `Exited && ExitCode≠0` или ошибке запуска —
поле `Command.String()` с Copy; при `NoInvite` — поле `FreshInvite` с Copy и
Run as administrator (`DaemonFreshInvite`); затем обновить снимок.

## Ключи locale

Уже в `ru.json` (использует core): «Waiting for the administrator command…»,
«Could not run the command as administrator: %s», «The command failed (exit
code %d). Run it in an elevated terminal to see its output:», «The
administrator command is still running. Check the status again in a
minute.», «The service is installed, but no invite was received. Pair it as
a separate step:», «The command succeeded, but pairing failed: %s», текст
`state_dir_foreign_before_install`, «The service was started.», «The service
was removed.», «The core copy was created or updated.»; ранее — «The
administrator prompt was cancelled.», «Paired with the daemon.».

Добавляет UI-агент (с RU): «Run as administrator» — «Выполнить от имени
администратора»; «Start the service» — «Запустить службу»; «Service Control
Manager reports: %s» — «Диспетчер служб сообщает: %s»; «The service copy
differs from the launcher core in %s. Install or update the service to
refresh it.» — «Копия службы расходится с ядром лаунчера в файле %s.
Установите или обновите службу.»; подписи строк без Terminal/sudo для
Windows (Install, Uninstall, Fresh invite, Remove all data).

## Не UI-агента (core, фаза 2)

Кнопка Install service в диалоге «TUN без прав» (`core/elevation.go`),
диалог гейта classic, «Core updated», модальное Unsafe в `main.go`,
системный прокси (`platform.SetUserSystemProxy/ClearUserSystemProxy/
ReadUserSystemProxy`, хук `setDaemonSystemProxy/clearDaemonSystemProxy`,
метка `daemon_system_proxy` в `settings.json`), классификатор SCM/DACL.
