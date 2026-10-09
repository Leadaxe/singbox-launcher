# CODEMAP 161 — что затрагивает окно Service

Срез: develop `9944c4f0`, 10.10.2026. Якоря `файл:строка` — на этот срез; при
расхождении искать по имени символа.

## 0. Неожиданное (влияет на план)

| Факт | Где | Следствие |
|---|---|---|
| `parseCoreBuild` / `compareCoreBuilds` / `leadingDigits` / `compareInts` / `sha256File` живут в файле с тегом `darwin \|\| (windows && !386)` | `core/daemon_service_state.go:1`, `:205–304`, `:560` | на Linux и Win7 их нет; удалённые машины обслуживаются со всех платформ → перенос в нетегированный файл |
| `shellQuote` — только darwin | `core/daemon_manager_darwin.go:190` | ssh-обёртке нужен свой POSIX-quote в нетегированном коде |
| Реестр машин **не** входит ни в бэкап, ни в контракт LxBox | `grep` по `core/backup`, `contract/` — пусто; реестр = `<DataDir>/bin/remote-daemons.json` (`core/services/lxd_remote_registry.go:75–103`) | новые поля без бампа контракта; миграция не нужна (`omitempty`) |
| `SHA256SUMS` есть в каждом релизе форка (проверено: lx.2, lx.11, lx.12, lx.14) | `https://github.com/Leadaxe/sing-box-lx/releases/download/v<ver>/SHA256SUMS` | формат `sha256sum`: `<hex64>␠␠<имя ассета>`; суммы — **по архивам** (`.tar.gz`/`.zip`), не по распакованному бинарю |
| Окна живого лога ядра **для машины нет** | стрим `SubscribeLogLines` зовёт только Debug API (`core/debugapi/remote_observe_endpoints.go:439`) | окно лога машины — новый файл |
| Локально «живой лог» = окно Logs, вкладка Core: в daemon-режиме читает кольцевой буфер `SubscribeLog` | `ui/log_viewer_window.go:79`, `:233–250`; `core/backend.go:565` `DaemonCoreLogLines` | для Local — открыть это окно |
| Демон отдаёт хвост СВОЕГО лога по сети: `GET /admin/logs?tail=N` (гайд §6) | в `internal/lxdclient` метода нет | в рамки 161 не входит (см. PLAN §11) |
| `RemoteHealth` сравнивается `prev != h` в heartbeat | `ui/machine_heartbeat.go:146` | поле, меняющееся каждый тик (uptime), вызвало бы перерисовку строк каждые 5 с |
| `SetStateDir` пишет реестр только при изменении | `core/services/lxd_remote_registry.go:569–592` | кэш паспорта обязан так же: без записи файла на каждый тик |
| Heartbeat не пишет в `errLog`; историю отказов пишет только Connect | `ui/machine_heartbeat.go:114–133`, `ui/machine_list_panel.go:540` | «не отвечает 12 мин, N попыток» требует отметки начала серии в `machineLiveness` |
| Asset-имена форка: `linux-armv7`, `linux-mips-softfloat`, `linux-mipsle-softfloat`; в `SingboxAssetSuffix` mips/mipsle нет | `core/core_downloader.go:304–339` | `SingboxAssetSuffixFor` добавляет mips/mipsle |
| Файлы состояния демона: `<state_dir>/last_good.json`, `candidate.json`, `clients.json`, `daemon.json` | форк `lxd/store.go:32–33,105` | рецепт last-good верен |
| Linux-рецепт форка: procd `/etc/init.d/sing-box-lxd`, state `/etc/sing-box-lxd/state`; systemd state `/var/lib/sing-box-lxd/state`; `--service=install` на Linux только печатает рецепт | форк `lxd/service_linux.go:32,103,109`; гайд §8 | дефолты SPEC §5.2 сходятся с форком |

## 1. Строка удалённой машины — `ui/machine_list_panel.go`

| Сущность | Якорь | Что делает |
|---|---|---|
| тексты строки | `:33–37` | `machinesDeployMissingText`, `powerDeployBodyText`, `machinesDriftTip`, `machinesRolledBackTip` |
| `connectAttempts`, `connectRetryDelay` | `:57–60` | 5 попыток × 3 с |
| `machineListPanel` | `:63–122` | карты `health`, `errLog`, `connectAttempt`, `moreOpen`, `liveness`, `builtSums`, `deployDrift`; `stopHeartbeat` |
| `connectFailure`, `maxConnectFailures` | `:131–143` | история отказов Connect за сессию |
| `CreateMachineListPanel` | `:142` | единственный экземпляр (из `CreateRemoteTab`), стартует heartbeat (`:194`) |
| `buildRow` | `:224` | строка машины |
| — маркер | `:244–276` | `markerFor` → цвет; клик → `OpenMachineWireLogWindow` |
| — `metaRow` (платформа + адрес + Connect) | `:278–301` | Connect/Disconnect/«Connecting… n/5» |
| — `nameRow` (✎ ✕) | `:303–318` | правка/удаление записи |
| — до Connect | `:322–334` | только паспорт, `return` — ни статуса, ни кнопок |
| — Configure + красная точка | `:337–347` | `withCornerDot(..., ColorNameError)` при `health.InterruptedApply` |
| — статус «версия · статус» | `:351–364` | `health.Version + " · " + CoreStatus`, ошибка `unreachable: %s` |
| — Start/Stop, ↻ | `:366–389` | `togglePower`, `restartCore` |
| — ⓘ | `:395–399` | `showHealthDetails(d, health)` |
| — Deploy + оранжевая точка | `:401–417` | `configDrift` → `withCornerDot(..., ColorNameWarning)` |
| — RES | `:423–428` | `OpenMachineResourcesWindow` |
| — `statusRow` | `:434–435` | `HBox(infoBtn, powerBtn, restartBtn)` справа от статуса |
| — «more» | `:440–486` | профайлер, телеметрия; стрелка `moreBtn` `:461`; ряд `HBox(configureObj, deployObj, resBtn)` `:470–474`; раскрытие `newRevealBox` `:481` |
| `connectMachine` | `:490` | гасит окна прежней машины (`:499–506`), повторы, `recordFailure` (`:540`), LAN-диагностика, `p.health[d.ID] = h` (`:569`), `FailStreak = threshold` при провале (`:577`) |
| `showHealthDetails` | `:621–728` | окно «что демон сообщает о себе» (state dir, sha, история отказов) |
| `disconnectMachine` | `:798` | закрывает профайлер/телеметрию |
| `editMachine` | `:825` | `OpenEditMachineWindow(..., reload)` |
| `removeMachine` | `:843` | закрывает окна машины (`:852–855`) |
| `deployTo` | `:972–1007` | читает `GetRemoteConfigPathFor`, `dialog.ShowConfirm` → `registry.Deploy` в горутине |
| `redrawRows` | `:1009` | перестройка всех строк из кэша |
| `builtConfigSHA`, `configDrift` | `:1024`, `:1049` | sha собранного config.json по mtime/размеру |
| `withCornerDot` / `cornerDotLayout` / `dotLayout` | `:1056–1097` | точка в правом верхнем углу кнопки |
| `recordFailure`, `failures` | `:1099`, `:1114` | история отказов под `errMu` |

## 2. Heartbeat и здоровье

| Сущность | Якорь | Что делает |
|---|---|---|
| `heartbeatInterval` 5 с, `heartbeatFailThreshold` 2, `heartbeatTimeout` 4 с | `ui/machine_heartbeat.go:26–56` | период, порог красного, срок опроса |
| `machineLiveness{FailStreak, LastErr, LastOK}` | `ui/machine_heartbeat.go:59–66` | свежесть ответов (отдельно от health) |
| `startHeartbeat` / `pollActive` | `:72`, `:91–157` | опрашивает только активную машину; ошибка — `FailStreak++` без перезаписи health; успех — `p.health[id] = h`, перерисовка при `prev != h` (`:146`) |
| `onCoreStatusChanged` | `:160` | синхронизирует левую колонку |
| `markerState`, `markerFor` | `:174–207` | idle / live / flaky / down |
| `livenessOf`, `healthOf` | `:210–218` | доступ для окна журнала (`livenessSource`) |
| `livenessSource` | `ui/machine_wire_log_window.go:50–53` | интерфейс «панель → окно», образец для окна Service |
| `wireLogRefresh` = 1 с | `ui/machine_wire_log_window.go:36` | окно журнала перечитывает кэш панели тикером, без сети |
| `services.RemoteHealth` | `core/services/lxd_remote_registry.go:809–830` | `Reachable, Err, CoreStatus, LastError, Version, StateDir, ActiveSHA, LastGoodSHA, InterruptedApply` |
| `Health` / `HealthWithin` / `healthCtx` | `:836`, `:848`, `:854–882` | `/admin/status` + best-effort `/admin/info`; кэширует `state_dir` через `SetStateDir` (`:878`) |

Конвейер **строка машины → heartbeat → RemoteHealth**: `buildRow` читает
`p.health[d.ID]`, `p.liveness[d.ID]` → `markerFor`. Connect (`connectMachine`)
и тик (`pollActive`, только активная машина, `GetLxdRemoteOverride`) зовут
`registry.Health/HealthWithin` → `adminClient(id)` → `lxdclient.StatusCtx` +
`InfoCtx` → `RemoteHealth`; успех кладётся в `p.health`, промах — только в
`p.liveness` (health остаётся последним известным); `redrawRows` — на смене
маркера/health/drift.

## 3. Реестр машин — `core/services/lxd_remote_registry.go`

| Сущность | Якорь | Что делает |
|---|---|---|
| `RemoteDaemon` | `:37–73` | `ID, Name, Addr, ServerFingerprint, Secret, GOOS, GOARCH, StateDir, AddedAt` |
| `remoteRegistryFile` = `remote-daemons.json` | `:75–76` | `<DataDir>/bin/remote-daemons.json` (`path()` `:92`) |
| `listLocked` / `saveLocked` | `:112`, `:128–147` | чтение/атомарная запись целиком (tmp + rename) |
| `Get` | `:150` | |
| `PairWithAddr` / `RePair` | `:183`, `:258–323` | re-pair правит `Addr/ServerFingerprint/Secret`, сбрасывает `StateDir` (`:316`), прочие поля сохраняет |
| `Update(id, name, addr)` / `SetAddr` | `:440`, `:472` | |
| `Remove` | `:501` | |
| `SetPlatform` | `:547–567` | GOOS/GOARCH |
| `SetStateDir` | `:569–592` | no-op при пустом/том же значении |
| `ResourceDir`, `TailscaleStateDir`, `Target()` | `:595`, `:617`, `:632` | `Target()` — дефолт `linux/amd64` |
| `ImportFrom` | `:729` | слияние реестра другой папки: копирует запись целиком (новые поля поедут сами) |
| `adminClient` / `Transport` | `:960`, `:989` | REST-клиент / gRPC-транспорт по записи |
| Debug API: `machineView` / `machineViewOf` | `core/debugapi/remote_endpoints.go:211–230` | проекция записи; новых полей нет |
| Debug API: PATCH машины | `core/debugapi/remote_endpoints.go:287–350` | `name, addr, goos, goarch` |
| бэкап / контракт | — | реестр не участвует (проверено grep) |
| импорт из UI | `ui/machine_add_window.go:200–216` | «Import from file…» → `ImportFrom` |

## 4. Окно Edit машины — `ui/machine_edit_window.go`

| Сущность | Якорь | Что делает |
|---|---|---|
| `OpenEditMachineWindow` | `:47–72` | три блока: паспорт, re-pair, copy-from; 560×640 |
| `machineEditPassport` | `:79–118` | форма Name/Address/Platform/Architecture; Save = `Update` + `SetPlatform` |
| `machineEditRePair` | `:129–227` | `CommandRow("…get an invite:", "sudo sing-box lxd client add", false)` `:136`; приглашение; Advanced (address, secret); подтверждение; `registry.RePair` в горутине `:178`; закрывает профайлер/телеметрию активной машины `:189–192` |
| `machineEditCopyProfile` | `:233` | перенос настроек |

## 5. Deploy из UI

`machineListPanel.deployTo` (`ui/machine_list_panel.go:972`) →
`os.ReadFile(platform.GetRemoteConfigPathFor(...))` (`internal/platform/platform_common.go:69`)
→ `dialog.ShowConfirm(powerDeployBodyText)` → горутина
`registry.Deploy(d.ID, config)` (`core/services/lxd_remote_deploy.go:46`:
ресурсы → `/admin/apply`) → `ShowInformation("Config applied on %s.")` + `p.Reload()`.
Тот же `Deploy` зовёт Debug API `handleRemoteDeploy`
(`core/debugapi/remote_endpoints.go:599`, вызов `:621`). Других вызовов нет.

## 6. Локальная панель Local

### 6.1 `ui/connection_window.go`, `ui/connection_local.go`

| Сущность | Якорь | Что делает |
|---|---|---|
| `OpenConnectionWindow` | `ui/connection_window.go:33` | окно «Connection settings» (⚙ на Servers, вызов `ui/core_dashboard_tab.go:374`); VScroll + gutter; без слоя тултипов |
| `buildLocalEngineTab` | `ui/connection_local.go:31–157` | радио Process/Daemon (`:57–58`), `buildDaemonPanel` (`:37`); `nil` вне daemon-платформ → только classic |

### 6.2 `ui/connection_local_daemon.go` (тег `darwin || (windows && !386)`)

| Сущность | Якорь | Что делает |
|---|---|---|
| тексты плашки и Uninstall | `:23–31` | `daemonServiceOtherCoreText`, `…NoBinaryText`, `…MismatchText`, `daemonUninstallAskText` |
| `buildDaemonPanel` | `:61–411` | вся панель |
| — короткая подсказка + «?» | `:66–72` | `daemonHintText` |
| — `status` + `renderStatus` | `:74–80` | `renderDaemonStatusText(ac, snap, true)` |
| — плашка службы `serviceBox` | `:89–139` | иконка + текст `daemonServiceNoticeText`; под ней install/bootstrap-строки по `NeedsInstall`/`NeedsBootstrap` |
| — `refreshStatus` | `:141–153` | горутина `ac.DaemonStatusSnapshot()` → `fyne.Do` |
| — `ops` | `:160–165` | `daemonOps{after: refreshStatus + onPaired}` |
| — `serviceInstallRow` / `serviceBootstrapRow` | `:172`, `:176` | `ops.row(daemonInstallRowLabel, …, DaemonInstallCommand, DaemonInstallOrUpdate)`; `ops.row(daemonStartRowLabel, …, DaemonBootstrapCommand, DaemonStartService)` |
| — приглашение / секрет | `:182–199` | `SetDaemonSecret` на Enter |
| — `secretHelp` | `:201–207` | `showCommandHelpDialog(..., DaemonShowSecretCommand())` |
| — `pairBtn` / `pairHelp` | `:209–239` | `PairDaemonWithInvite`; справка с `DaemonRepairCommand()` |
| — Uninstall: Unpair, `--purge`, команда, Run | `:241–313` | `UnpairDaemon`, `DaemonUninstallCommand(purge)`, Windows — `ShowActions(Keep copy / Remove all)` |
| — адрес демона | `:317–328` | `SetDaemonAddress` на Enter |
| — `Stop VPN when quitting` | `:331–342` | `settings.DaemonStopVPNOnExit` |
| — ↻ `refreshBtn` | `:344–345` | ручной рефреш (ключ «Refresh daemon status») |
| — `installTab` | `:352–366` | шаг install (`installStepRow`) + `installCoreHint` + приглашение/Pair + «Need a fresh invite» (`DaemonRepairCommand` / `DaemonFreshInvite`) |
| — `statusTab` | `:368–379` | статус, плашка, Connection (адрес, секрет), stop-on-exit |
| — `AppTabs` Status/Install/Uninstall | `:382–386` | |
| — автовыбор вкладки | `:392–402` | один раз: Install, если не установлено или не сопряжено |
| `renderDaemonStatusText` | `:416–469` | строки статуса (ядро/служба/сопряжение/достижимость/версия) |
| `daemonServiceNoticeText` | `:474–521` | текст плашки по `DaemonServiceCheck` |
| `coreBuildLabel` | `:523` | версия + короткий sha |
| `showTextHelpDialog` / `showCommandHelpDialog` | `:545`, `:550–578` | справка «текст + команда» |
| `daemonRunAsAdminKey`, `daemonStartServiceKey` | `:582–583` | ключи кнопок Windows |
| `daemonOps` | `:597–743` | `row` `:651` (macOS → `CommandRow(..., true)`; Windows → поле + Copy + «Run as administrator»), `run` `:681`, `showResult` `:702` |
| `daemonPurgeRow` | `:749` | строка в «Remove all data…» |
| стаб | `ui/connection_local_daemon_stub.go:1` (тег `!darwin && (!windows \|\| 386)`) | `buildDaemonPanel` → nil; `daemonPurgeRow` → `CommandRow(..., false)` |
| платформенные подписи | `ui/command_row_darwin.go:17–28`, `ui/command_row_windows.go:9–24` | `daemonInstallRowLabel`, `daemonStartRowLabel`, `…StepLabel`, `daemonPairHelpText`, `daemonServiceManagerText`, … |

### 6.3 Общие строки команд

| Сущность | Якорь | Что делает |
|---|---|---|
| `NewCopyButton` | `ui/command_row.go:29` | ⧉ с галочкой-фидбеком |
| `CommandRow(win, labelKey, command, withTerminal)` | `ui/command_row.go:49–94` | поле + ⧉ + ▶ (`theme.ComputerIcon`, тултип «Run in Terminal») если `openTerminal != nil` |
| `openTerminal` | `ui/command_row.go:96–98` | хук: macOS → `ac.OpenTerminalWithCommand` (`ui/command_row_darwin.go:9–16`); Windows/Linux — nil |
| `l10n_helpers.json` | `tools/l10n/l10n_helpers.json` | `CommandRow` — позиция 1 = ключ |

Конвейер **Local → DaemonStatusSnapshot → плашка**: `refreshStatus` (открытие окна,
↻, после операции `ops.after`) → горутина `ac.DaemonStatusSnapshot()`
(`core/daemon_manager.go:291`): settings → `CoreSupportsLxd`, `daemonServiceCheck(nil,"")`
(файлы + launchd/SCM) → `Paired` → `lxdclient.Status()` (`:312`; ошибка → только
DebugLog, возврат без `Reachable`) → `Info()` (`:323`) → `DaemonVersion`, `StateDir`,
`addDaemonProcessVerdict` (`:326`) → `fyne.Do`: `status.SetText(renderDaemonStatusText)`,
`applyServiceState` (плашка + install/bootstrap-строка), `onSnapshot` (стартовая
вкладка). Автоматического тика нет.

## 7. Ядро: служба демона — `core/daemon_manager*.go`, `core/daemon_service_state*.go`

| Сущность | Якорь | Что делает |
|---|---|---|
| `daemonDefaultListen` = `127.0.0.1:19091` | `core/daemon_manager.go:46–54` | |
| `DaemonUIStatus` | `core/daemon_manager.go:261–287` | `CoreSupportsLxd, ServiceInstalled, Paired, Address, Reachable, CoreStatus, LastError, InterruptedApply, DaemonVersion, StateDir, Service` — без `LogPath/Executable/Listen/TLS/Uptime`, без текста ошибки связи |
| `DaemonStatusSnapshot` | `:291–334` | см. конвейер §6 |
| `DaemonServiceCoreHint` | `:377` | подсказка «обновите ядро лаунчера» |
| `PairDaemonWithInvite` / `UnpairDaemon` / `SetDaemonAddress` / `SetDaemonSecret` | `:444`, `:498`, `:519`, `:552` | |
| `DaemonBootstrapCommand` | `:625` | `daemonBootstrapCommand()` платформы |
| `daemonServiceBinaryFor` | `:640` | копия службы, если пригодна, иначе ядро лаунчера |
| `DaemonRepairCommand` | `:651` | `lxd client add --name <client>` |
| `DaemonInstallCommand` / `daemonInstallCommandFor` | `:668`, `:674` | гейт `serviceCoreGate` |
| `DaemonUninstallCommand` / `daemonUninstallCommandFor` | `:686`, `:693` | `--service=uninstall [--keep-copy] [--purge]` |
| `DaemonCommand` / `DaemonServiceOp` / `DaemonRunResult` | `:727`, `:746–763`, `:789` | ops: install, start, fresh_invite, uninstall, copy |
| darwin: `daemonLaunchdLabel` = `com.leadaxe.sing-box-lxd`, `daemonFallbackRuntimeDir` = `/Library/Application Support/sing-box-lxd` | `core/daemon_manager_darwin.go:31–38` | |
| darwin: `daemonSystemPlistPath` | `:54` | `/Library/LaunchDaemons/<label>.plist` |
| darwin: `DaemonShowSecretCommand` | `:123` | `sudo grep '"secret"' '<runtime>/state/daemon.json'` |
| darwin: `daemonServiceCommand` / `daemonBootstrapCommand` / `DaemonKickstartCommand` | `:131`, `:137`, `:143` | `sudo '<bin>' args`; `sudo launchctl bootstrap system '<plist>'`; `sudo launchctl kickstart -k system/<label>` |
| darwin: `OpenTerminalWithCommand` | `:157–178` | osascript → Terminal `do script` |
| darwin: `appleScriptString`, `shellQuote` | `:181`, `:190` | |
| darwin: `DaemonOpsElevated = false`, ops | `:199–232` | всё через `runDaemonOpInTerminal` (`:234`) |
| windows: `daemonServiceName` = `sing-box-lxd` | `core/daemon_manager_windows.go:37` | |
| windows: `DaemonOpsElevated = true` | `:58` | |
| windows: `daemonFallbackStateDir` | `:62` | `<ProgramData>\sing-box-lxd\state` |
| windows: `daemonServiceCommand`, `powerShellQuote` | `:74`, `:88` | PowerShell `& '<bin>' args` |
| windows: `daemonScExe`, `daemonBootstrapCommand` | `:121`, `:125` | `sc.exe start sing-box-lxd` |
| windows: `DaemonKickstartCommand` = "" | `:131` | |
| windows: `DaemonShowSecretCommand` | `:136` | `Select-String … daemon.json` |
| windows: `runDaemonCommandElevated` | `:185` | `platform.RunElevated` + ожидание кода |
| windows: `DaemonInstallOrUpdate`, `DaemonStartService`, `DaemonFreshInvite`, `DaemonUninstallService` | `:298`, `:362`, `:391`, `:420` | |
| `DaemonServiceState` / `DaemonServiceCheck` | `core/daemon_service_state.go:47–76`, `:80–116` | not_installed / unsafe / stale / not_running / process_stale / ok / core_too_old |
| `NeedsInstall` / `NeedsBootstrap` / `CopyUsable` / `InstallSupported` | `:118`, `:128`, `:134`, `:144` | |
| `serviceCoreGate`, `coreSupportsRootOwnedCopy` | `:182`, `:193` | |
| `coreBuild`, `parseCoreBuild`, `leadingDigits`, `compareCoreBuilds`, `compareInts` | `:205–304` | разбор `X.Y.Z-lx.N[-rcK]`, сравнение (тег!) |
| `sha256File` | `:560` | (тег!) |
| darwin: `minCoreForRootOwnedService` = `1.14.1-lx.12`; копия `/Library/PrivilegedHelperTools/sing-box-lxd`; сайдкар `.install.json` | `core/daemon_service_state_darwin.go:49–110` | `daemonServiceCorePath` `:80`, `daemonServiceSidecarPath` `:102`, `systemDaemonServiceLayout` `:106` |
| windows: `minCoreForRootOwnedService` = `1.14.2-lx.2`; копия `platform.PrivilegedCopyDir()\sing-box-lxd.exe`; сайдкар `PrivilegedSidecarName` | `core/daemon_service_state_windows.go:30–44` | |
| `diagnoseReachError` | `core/backend_daemon.go:214–255` | классификация ошибок связи по подстрокам: TLS-handshake / refused / certificate / HTTPS / EOF |
| `DaemonCoreLogLines`, `DaemonLink` | `core/backend.go:565`, `:618` | буфер лога и состояние канала daemon-движка |

## 8. Паспорт демона, лог машины

| Сущность | Якорь | Что делает |
|---|---|---|
| `lxdclient.StatusInfo` / `Status` / `StatusCtx` | `internal/lxdclient/client.go:116`, `:194`, `:203` | `/admin/status` |
| `lxdclient.InfoData` | `:539–555` | `Version, StateDir, Listen, TLS, Fingerprint, PID, UptimeSeconds, LogPath, Executable, ExecutableSHA256` |
| `Info` / `InfoCtx` | `:558`, `:563` | `/admin/info` |
| `Enroll` | `:491` | сопряжение по коду |
| `services.LogLine`, `SubscribeLogLines` | `core/services/lxd_remote_transport_extra.go:51`, `:61` | gRPC `SubscribeLog` машины, отмена через `cancel` |
| `lxdOverrideTransportForID` | `ui/lxd_remote_override.go:171` | транспорт активной машины (иначе нет) |
| `handleRemoteLogs` | `core/debugapi/remote_observe_endpoints.go:417` | единственный потребитель стрима машины |
| `OpenLogViewerWindow` | `ui/log_viewer_window.go:79` | окно Logs (Internal / Core / API), вызывается из Diagnostics (`ui/diagnostics_tab.go:291`) |

## 9. Загрузчик ядра — `core/core_downloader.go`

| Сущность | Якорь | Что делает |
|---|---|---|
| `coreReleaseRepo` | `:31` | `constants.SingboxCoreRepo` |
| `DownloadProgress{Progress, Message, Status, Error}` | `:55–60` | статусы downloading / extracting / done / error |
| `DownloadCore` | `:68–171` | ядро ЛАУНЧЕРА: release info → asset → temp → `downloadFile` → `extractArchive` → `installBinary` → `ResolveCore` → уведомления службы |
| `directAssetName` / `DirectAssetURL` | `:206`, `:216` | имя/URL по текущей платформе (`runtime`) |
| `SingboxAssetSuffix` | `:304–339` | только `runtime.GOOS/GOARCH`; нет mips/mipsle |
| `findPlatformAsset` | `:342` | |
| `downloadFile` / `downloadFileFromURL` | `:379`, `:412` | оригинал → зеркала `GitHubDownloadMirrors`; HTML-заглушка отсекается |
| `extractArchive` / `extractZip` / `extractTarGz` | `:574`, `:584`, `:638` | имя бинаря — `platform.GetExecutableNames()` текущей ОС (tar ещё принимает суффикс `sing-box`) |
| `installBinary` | `:704` | |
| UI: прогресс | `ui/core_dashboard_tab.go:984–1033` `handleDownload` | канал `DownloadProgress` (буфер 10) → `fyne.Do` → `setSingboxState(..., progress)`; done → `ShowInfo`; error → `ShowDownloadFailedManualWithReason` |
| UI: подсказка имени ассета | `ui/core_dashboard_tab.go:532` | `core.SingboxAssetSuffix()` |
| `downloadFailureReason` | `ui/core_dashboard_tab.go:1038` | текст для rate-limit |

SHA256SUMS: `https://github.com/Leadaxe/sing-box-lx/releases/download/v1.14.3-lx.14/SHA256SUMS`,
строки `<sha256 архива>␠␠sing-box-1.14.3-lx.14-linux-arm64.tar.gz` (по всем 12
ассетам + `.aar`). Ассеты v1.14.3-lx.14: `darwin-{amd64,arm64}.tar.gz`,
`linux-{amd64,arm64,armv7}.tar.gz`, `linux-{mips,mipsle}-softfloat.tar.gz`,
`windows-{amd64,arm64}.zip`, `windows-386-legacy-windows-7.zip`.

## 10. Платформа, константы, локаль, диалоги, виджеты

| Сущность | Якорь | Что делает |
|---|---|---|
| `platform.OpenURL` | `internal/platform/platform_darwin.go:30`, `platform_windows.go:34`, `platform_linux.go:33` | открыть в браузере |
| образец гиперссылки | `ui/machine_add_window.go:110–117` | `widget.NewHyperlink` + `SetURLFromString` + `OnTapped → platform.OpenURL` |
| прочие `NewHyperlink` | `ui/core_dashboard_tab.go:541,1063,1176`, `ui/help_tab.go:85,94` | |
| `SingboxReleasesURL`, `RemoteDaemonDocsURL`, `ContractWarningsDocBaseURL` | `internal/constants/constants.go:142–166` | URL-константы |
| `SingboxCoreRepo` | `:184` | `Leadaxe/sing-box-lx` |
| `RequiredCoreVersion` = `1.14.3-lx.14` | `:191` | |
| `RemoteDaemonsDirName` | `:78` | |
| `locale.T` / `TN` / `Tf` / `GetLang` | `internal/locale/locale.go:119`, `:126`, `:138`, `:199` | ключ = английский текст |
| каталог | `bin/locale/ru.json` | `{"<en>": {"value": "<ru>"}}` |
| l10n-гарды | `tools/l10n/README.md`; `go run ./tools/l10n/l10n_check --strict`, `go run ./tools/l10n/hardcoded_check --strict` | missing/orphan роняют CI; литерал с буквами в display-позиции без `locale.*` — тоже |
| Win7-страж | `tools/win7guard/main.go` | `go run ./tools/win7guard` |
| `dialogs.NewCustom` | `internal/dialogs/dialogs.go:25` | диалог с произвольным содержимым и кнопками |
| `dialogs.Action` / `ActionsDialog` / `ShowActions` | `:326`, `:339`, `:396` | список действий, без чекбокса |
| `ShowError`, `ShowErrorText`, `ShowConfirm` | `ui/dialogs.go:11`, `:18`, `:39` | |
| `ttwidget` | `github.com/dweymouth/fyne-tooltip/widget` | кнопки с тултипом; окну нужен `fynetooltip.AddWindowToolTipLayer` (образец `ui/machine_resources_window.go:131`), снятие — `DestroyWindowToolTipLayer` (`ui/servers_filter_window.go:528`) |
| `components.WrapInScrollWithGutter`, `NewScrollGutter` | `ui/components/scroll_gutter.go:105`, `:85` | |
| минимум прокрутки константой | `ui/configurator/tabs/scroll_height.go:15` (`wizardTabScrollMinHeight = 160`) | образец лечения «AppTabs берёт максимум по вкладкам» |

## 11. Debug API про службу и машины

| Сущность | Якорь | Что отдаёт |
|---|---|---|
| `DaemonStatus` | `core/debugapi/daemon_endpoints.go:19–39` | без log_path/executable/listen/tls/uptime, без ошибки связи |
| `DaemonCommands` | `:44–51` | `install, uninstall, uninstall_purge, repair, kickstart, show_secret` |
| маршруты `/daemon/*` | `:81–88` | status, pair, unpair, settings, engine, commands, raw/rest, raw/grpc |
| `handleDaemonStatus` / `handleDaemonCommands` | `:92`, `:210` | |
| проводка | `core/debugapi_wiring_daemon.go:32–49` (`Status`), `:61–76` (`Commands`) | из `DaemonStatusSnapshot` и `Daemon*Command` |
| маршруты `/remote/machines/*` | `core/debugapi/remote_endpoints.go:65–121` | CRUD, repair, health, core start/stop/rollback, deploy, logs, host… |
| `handleRemoteHealth` | `:492–513` | `reachable, error, core_status, last_error, version, state_dir, active_sha, last_good_sha, interrupted_apply` |

Рецептов службы (status/restart/log/last-good/ssh) в API нет; `kickstart` на
Windows — пустая строка.

## 12. Документация

| Документ | Якорь | Что там сейчас |
|---|---|---|
| `docs/TROUBLESHOOTING.md` / `.ru.md` | разделы `All platforms`, `Linux`, `Windows`, `Remote machines` (`:117` / `:114`) | про демон ничего |
| `docs/DAEMON_AND_REMOTE.md` / `.ru.md` | §2 установка (`.ru:65`), §3 сопряжение (`:185`), §4.1 вкладка Remote (`:218`), §4.2 запись реестра (`:239`), §4.4 Deploy (`:280`) | |
| `docs/API.md` / `.ru.md` | «Remote machines (SPEC 100)» (`API.md:348`) | |
| `docs/ARCHITECTURE.md` | L6 (`:85`), §11 (`:1029`) | список окон машины, швы движков |
| гайды форка | `docs-lx/lxd-daemon(.ru).md` §3 (daemon.json), §6 (логи), §7/§7a/§8/§9/§11; `docs-lx/openwrt-vpn-ssid(.ru).md` | ветка `lx` |
| `docs/release_notes/upcoming.md` | `## EN / ### Highlights`, `## RU / ### Основное` | |
