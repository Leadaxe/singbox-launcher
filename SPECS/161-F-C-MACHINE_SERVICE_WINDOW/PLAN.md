# PLAN 161 — окно Service

Карта затрагиваемого кода с якорями — [CODEMAP.md](CODEMAP.md). Здесь — как
строим и какие решения приняты. Решения, принятые в плане сверх SPEC,
помечены **[решение]**.

## 1. Слои

```
core (без Fyne, без тегов)              ui (Fyne)
─────────────────────────────           ──────────────────────────────────────
core_build.go         версии ядра  ──┐  service_model.go     serviceSource, snapshot, вкладки
service_recipes.go    рецепты, ssh,  ├─▶ service_window.go   окно/встраиваемый вид, шапка, ре-рендер
                      классификатор  │  service_tabs.go      Not running / Core / Pairing / Reference
core_download_target.go ядро под   ──┘  service_step_row.go  строка «шаг: команда [⧉][▶]»
                      платформу         service_guides.go    ссылки на гайды по локали
services/lxd_remote_registry.go         service_source_remote.go   источник «машина» + вердикт строки
                      поля записи       service_source_local.go    источник «локальный демон» (тег)
daemon_manager*.go    паспорт, restart  service_core_download.go   шаг 1 вкладки Core (удалённо)
                                        machine_core_log_window.go живой лог машины
```

Принцип: всё, что можно проверить таблицей (команды по платформе, ssh-обёртка,
сравнение версий, имя ассета, классификация ошибки связи), — чистые функции в
`core` без Fyne и без build-тегов; UI только раскладывает готовые
`core.ServiceStep`. Локальный и удалённый случаи различаются **источником**, а
не окном.

## 2. core: версии ядра — `core/core_build.go` (новый, без тегов)

- **Перенос** (не копия) из `core/daemon_service_state.go:205–304`:
  `coreBuild`, `parseCoreBuild`, `leadingDigits`, `compareCoreBuilds`,
  `compareInts`; и `sha256File` (`:560`). В `daemon_service_state.go` они
  удаляются; его пользователи (`coreSupportsRootOwnedCopy`, кэши) продолжают
  звать их из того же пакета. Причина: файл под тегом
  `darwin || (windows && !386)`, а сравнение нужно и на Linux/Win7 (машины
  обслуживаются с любой платформы).
- Экспорт:
  - `type CoreVersionVerdict int` — `CoreVersionUnknown | CoreVersionOlder | CoreVersionCurrent | CoreVersionNewer`.
  - `CompareCoreVersion(running, required string) CoreVersionVerdict` —
    `Unknown`, если любая сторона не разбирается (`unknown`, апстрим без
    `-lx.N`, пусто). Пре-релиз сравнивается как есть (`lx.14-rc1 < lx.14`).
  - `CoreBuildShort(v string) string` — `"lx.11"`; если у сравниваемых
    версий одинаковый `lx`, но разная база — полная версия
    (`CoreVersionPairLabels(running, required) (a, b string)`), чтобы строка
    «lx.14 older than lx.14» не появилась.
- **[решение]** «Неизвестная» версия (dev-сборка) **не** даёт ⚠: точка и
  строка только при доказанном `Older`; окно пишет «cannot compare».

## 3. core: рецепты — `core/service_recipes.go` (новый, без тегов)

Чистые функции, без `AppController`, без сети.

```go
type ServiceInit string // "procd" | "systemd" | "launchd" | "scm"
const ServiceName = "sing-box-lxd"; const ServiceLaunchdLabel = "com.leadaxe.sing-box-lxd"

type ServicePlatform struct{ GOOS, GOARCH string; Init ServiceInit }
func DefaultServiceInit(goos, goarch string) ServiceInit
//   linux: arm, arm64, mips, mipsle → procd; прочее → systemd; darwin → launchd; windows → scm
func NormalizeInitChoice(stored string, goos, goarch string) ServiceInit // "" → дефолт

type ServicePath struct{ Value string; Default bool } // Default = не сообщено демоном
type ServicePaths struct {
    Executable, StateDir, LogPath, ServiceFile, Listen ServicePath
    TLS *bool        // nil — не сообщено
    // локально дополнительно (заполняет источник):
    ServiceBinary, InstallRecord ServicePath
}
func DefaultServicePaths(p ServicePlatform) ServicePaths
//   procd:   /usr/bin/sing-box, /etc/sing-box-lxd/state, /tmp/lxd.log, /etc/init.d/sing-box-lxd
//   systemd: /usr/local/bin/sing-box, /var/lib/sing-box-lxd/state, /var/lib/sing-box-lxd/lxd.log,
//            /etc/systemd/system/sing-box-lxd.service
//   launchd: /Library/PrivilegedHelperTools/sing-box-lxd, /Library/Application Support/sing-box-lxd/state,
//            /Library/Application Support/sing-box-lxd/lxd.log, /Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist
//   scm:     C:\Program Files\sing-box-lxd\sing-box-lxd.exe, C:\ProgramData\sing-box-lxd\state,
//            C:\ProgramData\sing-box-lxd\logs\lxd.log, "sing-box-lxd" (SCM)
//   (локальный источник подменяет launchd/scm-значения точными из core/daemon_*_<os>.go)
func MergeServicePaths(reported ServicePassport, def ServicePaths) ServicePaths
//   непустое из паспорта → Default=false; иначе дефолт с Default=true

type ServicePassport struct{ Version, StateDir, Executable, LogPath, Listen string; TLS *bool }

type ServiceStep struct {
    ID          string // "status","restart","start","stop","log_tail","log_follow","last_good",
                       // "core_upload","core_check","core_swap","core_rollback","scratch_enable",
                       // "remove_service","remove_state","client_add","client_list","client_remove",
                       // "show_daemon_json","port_owner","bootstrap"
    Command     string // ровно то, что копирует ⧉ (без ssh-обёртки)
    RunsLocally bool   // выполняется на ЭТОМ компьютере (upload): ▶ без обёртки
    NeedsRoot   bool   // команде нужен root на машине (для sudo-префикса не-root ssh)
    UsesDefault bool   // в команде есть путь с Default=true → подпись «default»
    Interactive bool   // foreground (last-good): ▶ всё равно можно, подпись про Ctrl-C
}
type ServiceRecipeInput struct {
    Platform  ServicePlatform; Paths ServicePaths
    SSHUser   string     // для sudo-префикса: не root → "sudo "
    Running   string     // версия ядра на машине (для имени бэкапа)
    Uploaded  string     // локальный путь скачанного бинаря (для core_upload); "" — плейсхолдер
    Today     time.Time  // имя бэкапа при неизвестной версии
}
type ServiceRecipes struct{ Steps map[string]ServiceStep; ScratchScript string; ScratchPath string }
func BuildServiceRecipes(in ServiceRecipeInput) ServiceRecipes
```

Команды — по таблице SPEC §5.2/§5.3 с решениями:

| ID | procd | systemd | launchd (локально и по ssh) | scm |
|---|---|---|---|---|
| status | `/etc/init.d/sing-box-lxd status` | `systemctl status sing-box-lxd --no-pager` | `launchctl print system/com.leadaxe.sing-box-lxd` | `sc.exe query sing-box-lxd` |
| restart | `… restart` | `systemctl restart sing-box-lxd` | `sudo launchctl kickstart -k system/…` | **[решение]** `Restart-Service -Name sing-box-lxd -Force` |
| bootstrap | — | — | `sudo launchctl bootstrap system '<plist>'` | `sc.exe start sing-box-lxd` |
| stop | `… stop` | `systemctl stop …` | `sudo launchctl bootout system/…` | `sc.exe stop sing-box-lxd` |
| log_tail | `tail -n 100 <log>` | то же | `sudo tail -n 100 '<log>'` | `Get-Content -Tail 100 '<log>'` |
| log_follow | `tail -f <log>` | то же | `sudo tail -f '<log>'` | `Get-Content -Wait -Tail 50 '<log>'` |
| last_good | `stop && <exe> lxd --state-dir <s> --config-force <s>/last_good.json` | то же | `sudo launchctl bootout … ; sudo '<exe>' lxd …` | `sc.exe stop …; & '<exe>' lxd …` |
| show_daemon_json | `cat <s>/daemon.json` | то же | `sudo cat '<s>/daemon.json'` | `Get-Content '<s>\daemon.json'` |
| port_owner | `netstat -lnp \| grep <port>` | `ss -ltnp \| grep <port>` | `sudo lsof -nP -i :<port>` | `netstat -ano \| findstr :<port>` |
| client_add / list / remove | `<exe> lxd client add --name singbox-launcher --state-dir <s>` / `client list …` / `client remove <name> …` | то же | под `sudo` | PowerShell `& '<exe>' lxd client …` |

- **[решение] Restart на Windows** — `Restart-Service … -Force` вместо пары
  `sc.exe stop` + `sc.exe start`: `sc.exe stop` асинхронен, и немедленный
  `start` падает с 1056 «already running». Одно окно UAC.
- Ядро на машине (Linux):
  - `core_upload` (`RunsLocally`): `ssh [-p N] <user@host> 'cat > /tmp/sing-box.new' < '<путь скачанного>'`.
  - `core_check`: `chmod +x /tmp/sing-box.new && sha256sum /tmp/sing-box.new && /tmp/sing-box.new version && /tmp/sing-box.new check -c <s>/last_good.json`
    (**[решение]** `sha256sum` всегда — сверка с показанным в шаге 1 sha бинаря;
    busybox на OpenWrt его имеет).
  - `core_swap`: `cp <exe> <bak> && <init stop> && mv /tmp/sing-box.new <exe> && chmod 755 <exe> && <init start>`;
    `<bak>` = procd `/root/sing-box.<версия>.bak`, systemd `<exe>.<версия>.bak`;
    версия неизвестна → `YYYYMMDD` от `Today`.
  - `core_rollback`: `cp <bak> <exe> && <init restart>`.
  - `scratch_enable`: procd `chmod +x /etc/init.d/sing-box-lxd && /etc/init.d/sing-box-lxd enable && /etc/init.d/sing-box-lxd start`;
    systemd `systemctl daemon-reload && systemctl enable --now sing-box-lxd`.
  - `ScratchScript`: init-скрипт procd / unit systemd из гайда форка §8.2/§8.3
    с подставленными `<exe>` и `<s>`; для procd — напоминание про
    `/etc/sysupgrade.conf` (строка подсказки в UI, не в скрипте).
  - `remove_service`: procd `/etc/init.d/sing-box-lxd disable && /etc/init.d/sing-box-lxd stop && rm /etc/init.d/sing-box-lxd`;
    systemd `systemctl disable --now sing-box-lxd && rm /etc/systemd/system/sing-box-lxd.service && systemctl daemon-reload`.
  - `remove_state`: `rm -r <s>` (отдельный шаг, красная подпись).
- Удалённые darwin/windows: шаги 2–4 Core не строятся (**[решение]**:
  установка там — `lxd --service=install` на самой машине, гайд §7/§7a); остальные
  шаги строятся по launchd/scm. Для scm по ssh ▶ не предлагается (§6).
- Пути в командах POSIX-квотятся только если содержат что-то кроме
  `[A-Za-z0-9_./-]` (`/tmp/lxd.log` остаётся читаемым, `/Library/Application Support/…` — в кавычках).

ssh-цель — в `core/services/ssh_target.go` (**[решение]**: её валидирует
`RemoteRegistry.SetSSH`, а `core/services` не может импортировать `core`);
обёртка и классификация — в `core/service_recipes.go`:

```go
// core/services/ssh_target.go
type SSHTarget struct{ User, Host string; Port int } // Port 0 = по умолчанию
func ParseSSHTarget(s string) (SSHTarget, error)       // user@host, user@host:port, host, [v6]:port
func DefaultSSHTarget(daemonAddr string) SSHTarget    // root@<хост адреса демона>
func (t SSHTarget) String() string                    // как ввёл бы человек: root@192.168.10.1[:2222]

// core/service_recipes.go
func WrapSSH(t services.SSHTarget, remoteCmd string, tty bool) string
//   ssh [-t] [-p N] user@host '<cmd с экранированными одинарными кавычками>'
//   tty=true, когда в команде есть sudo (пароль в Terminal)
func PosixQuote(s string) string

type DaemonReachKind int // ReachOK | ReachDown | ReachCertChanged | ReachNotTrusted | ReachChannelMismatch | ReachUnknown
func ClassifyDaemonReachError(err string) DaemonReachKind
//   down: "connection refused", "no route to host", "i/o timeout", "no such host", "dial tcp", "deadline exceeded"
//   cert changed: "fingerprint", "certificate" (кроме "bad certificate")
//   not trusted: "bad certificate", "unknown certificate authority", "403", "not paired"
//   channel mismatch: "first record does not look like a TLS handshake", "HTTP request to an HTTPS server"
```

- **[решение]** `diagnoseReachError` (`core/backend_daemon.go:214`) переводится
  на `ClassifyDaemonReachError` для выбора ветки (тексты советов и
  «следование за plain-каналом» не меняются) — подстроки живут в одном месте.

Тест — один табличный `TestServiceRecipes` в `core/service_recipes_test.go`
(раздел 12).

## 4. core: ядро под платформу машины

`core/core_downloader.go`:
- `SingboxAssetSuffixFor(goos, goarch string) string` — текущая таблица +
  `linux/mips` → `linux-mips-softfloat.tar.gz`, `linux/mipsle` →
  `linux-mipsle-softfloat.tar.gz`; `SingboxAssetSuffix()` становится
  `SingboxAssetSuffixFor(runtime.GOOS, runtime.GOARCH)`.
- `directAssetNameFor(version, goos, goarch)`, `DirectAssetURLFor(...)`;
  старые функции — обёртки.
- `extractZip`/`extractTarGz` получают имя бинаря параметром
  (`extractArchiveNamed(archive, dest, binName)`); `extractArchive` — обёртка с
  `platform.GetExecutableNames()`.

`core/core_download_target.go` (новый, без тегов):

```go
type TargetCoreDownload struct {
    Path, SidecarPath string // ~/Downloads/sing-box-<ver>-<goos>-<goarch>[.exe], <Path>.sha256
    Asset, ArchiveSHA, BinarySHA string
    SumsVerified bool        // архив сверен с SHA256SUMS
    SumsMissing  bool        // SHA256SUMS в релизе нет / не скачался
}
func TargetCoreFileName(version, goos, goarch string) string
func DownloadsDir() (string, error)                       // os.UserHomeDir()/Downloads, MkdirAll
func CheckTargetCore(version, goos, goarch string) (TargetCoreDownload, bool)
//   файл и сайдкар есть, sha256(файл) == сайдкар → ok (сразу ✓, без сети)
func (ac *AppController) DownloadCoreForTarget(ctx context.Context, version, goos, goarch string,
    progress chan DownloadProgress) (TargetCoreDownload, error)
//   temp (platform.GetTempDir) → downloadFile(архив, зеркала, прогресс 15–75)
//   → downloadFile(SHA256SUMS) best-effort → сверка sha архива (несовпадение = ошибка, файл не кладётся)
//   → extractArchiveNamed(binName по goos) → копия в Downloads, chmod 0755
//   → sha256 бинаря → сайдкар "<hex>  <имя>\n" → progress done
```

- Канал `progress` закрывает сама функция (`defer close(progress)`), как
  `DownloadCore`. Ядро лаунчера она **не трогает** (никаких `ResolveCore`,
  уведомлений службы, `installBinary` в `bin/`).
- **[решение]** Сайдкар `.sha256` рядом с файлом в `~/Downloads` (формат
  `sha256sum`) — источник «уже скачано и проверено» между запусками; запись в
  данные лаунчера не нужна.

## 5. Данные

### 5.1 Запись машины — `services.RemoteDaemon`

```go
SSH         string          `json:"ssh,omitempty"`          // user@host[:port]; "" = DefaultSSHTarget(Addr)
InitSystem  string          `json:"init_system,omitempty"`  // "" | "systemd" | "procd"
CoreWarnAck string          `json:"core_warn_ack,omitempty"`// версия ядра, для которой Deploy не спрашивает
Passport    *RemotePassport `json:"passport,omitempty"`

type RemotePassport struct {
    Version    string `json:"version,omitempty"`
    Executable string `json:"executable,omitempty"`
    LogPath    string `json:"log_path,omitempty"`
    Listen     string `json:"listen,omitempty"`
    TLS    *bool  `json:"tls,omitempty"`
    SeenAt string `json:"seen_at,omitempty"` // RFC3339
}
```

- `state_dir` в паспорт **не** дублируется: источник — существующее
  `RemoteDaemon.StateDir` (его уже кэширует `SetStateDir`, его читает генерация).
- Сеттеры: `SetSSH(id, s)` (валидирует `ParseSSHTarget`, пустое — сброс),
  `SetInitSystem(id, v)`, `SetCoreWarnAck(id, version)`, `SetSecret(id, s)`
  (для plain-режима во вкладке Pairing), `SetPassport(id, p)`.
- `SetPassport` — **no-op**, если поля, кроме `SeenAt`, не изменились и
  прежний `SeenAt` моложе `passportSeenRefresh = 10 * time.Minute`: иначе
  heartbeat перезаписывал бы файл каждые 5 с.
- `healthCtx` (`:874–881`): вместо одного `SetStateDir` — `SetStateDir` +
  `SetPassport` из `InfoData`.
- `RePair` новые поля не трогает (паспорт помечен временем; после
  переустановки обновится на первом ответе).
- Хранение — тот же `remote-daemons.json`. Миграции нет: поля `omitempty`,
  отсутствие = прежнее поведение. `ImportFrom` переносит их как есть.
- **Бэкап/контракт:** реестр не входит ни в LX Backup, ни в `contract/`
  (проверено, CODEMAP §0) → бамп `contract/VERSION`, changelog и параграф
  `contract/TASKS_LXBOX.md` **не нужны**.

### 5.2 `services.RemoteHealth`

Добавить `Executable, LogPath, Listen string; TLS *bool; UptimeSeconds int`.
`healthCtx` их заполняет. В heartbeat сравнение `prev != h`
(`ui/machine_heartbeat.go:146`) заменяется на `healthChanged(prev, h)`,
которое обнуляет `UptimeSeconds` перед сравнением (иначе перерисовка каждые
5 с). `*bool` сравнивается по указателю — поэтому `healthChanged` сравнивает
`TLS` по значению.

### 5.3 Локальный демон

`core/daemon_manager.go`:
- `DaemonUIStatus` += `ReachErr string` (текст ошибки `Status()`),
  `Passport lxdclient.InfoData`, `PassportSeenAt time.Time`,
  `PassportCached bool` (паспорт из кэша, демон сейчас молчит).
- Кэш паспорта — переменная пакета в `daemon_manager.go`
  (`daemonPassportCache{mu sync.Mutex; info; seen}`): обновляется на каждом
  успешном `Info()`, отдаётся при недоступности. В память, не на диск
  (**[решение]**: после перезапуска лаунчера пути берутся из точных
  платформенных дефолтов — для macOS/Windows они известны).
- `DaemonServicePaths() core.ServicePaths` — платформенные точные значения:
  macOS `daemonServiceCorePath()`, сайдкар, `daemonSystemPlistPath()`,
  `daemonFallbackRuntimeDir/state`, лог `<runtime>/lxd.log`; Windows
  `daemonServiceCorePath()`, сайдкар, SCM `sing-box-lxd`,
  `daemonFallbackStateDir()`, `PrivilegedDataDir()\logs\lxd.log`.
  Реализация — `daemonServicePathsPlatform()` в `_darwin.go`/`_windows.go`.
- Операция перезапуска: `DaemonOpRestart`, `ac.DaemonRestartCommand() string`,
  `ac.DaemonRestartService() DaemonRunResult` — macOS: Terminal с
  `DaemonKickstartCommand`; Windows: `RunElevated(powershell.exe, -NoProfile
  -NonInteractive -Command "Restart-Service -Name sing-box-lxd -Force")`, затем
  `waitDaemonServiceRunning`.
- `DaemonClientListCommand()`, `DaemonClientRemoveCommand(name)` — тот же
  бинарь, что `DaemonRepairCommand` (`daemonServiceBinaryFor`).
- `DaemonKickstartCommand` на Windows остаётся "" (Debug API совместим).

## 6. Модель окна (ui)

### 6.1 `ui/service_model.go` (без тегов)

```go
type serviceTab int // tabNotRunning, tabCore, tabPairing, tabReference, tabUninstall
type serviceSnapshot struct {
    Title       string               // имя машины / "This computer"
    Platform    core.ServicePlatform
    InitChoice  bool                 // показывать Select init (Linux удалённо)
    Reachable   bool; Err string; DownSince time.Time; Attempts int
    Reach       core.DaemonReachKind
    CoreStatus  string; Running string; Uptime time.Duration
    Paths       core.ServicePaths; PassportAt time.Time; PassportLive bool
    Paired      bool                 // локально: есть пара; удалённо: true (запись = пара)
    InterruptedApply bool
    ServiceNote string; ServiceNoteDanger bool // локально: daemonServiceNoticeText
    Local       *localServiceExtras  // локально: DaemonServiceCheck, LauncherVersion; nil удалённо
}
type serviceSource interface {
    Key() string                         // "local" | id машины — ключ окна
    Snapshot() serviceSnapshot           // из кэша, без сети, UI-поток
    Poll(onUpdate func())                // старт тикера/подписки; onUpdate в UI-потоке
    StopPoll()
    SSH() (services.SSHTarget, bool)       // false — команды выполняются здесь
    SetInit(core.ServiceInit) error      // удалённо: SetInitSystem; локально — nil
    Pair(invite, addr, secret string, done func(error))
    OpenLiveLog(win fyne.Window)
    LocalRows(win fyne.Window) localServiceRows // локально — строки daemonOps; удалённо — пусто
}
```

`localServiceRows` — набор готовых `fyne.CanvasObject` (install, bootstrap,
restart, fresh invite, Pair-поле, адрес, секрет, Uninstall-вкладка), которые
строит локальный источник через существующий `daemonOps`: так в
нетегированный код не утекают `core.DaemonOpsElevated`,
`DaemonInstallOrUpdate` и прочие символы с тегом.

### 6.2 `ui/service_window.go`

- `OpenServiceWindow(ac, src serviceSource, tab serviceTab)` — одно окно на
  `src.Key()` (карта + мьютекс, как `wireLogWindows`); повторный вызов —
  `RequestFocus` + `SelectTab`. 640×560, `fynetooltip.AddWindowToolTipLayer`,
  на закрытии `StopPoll` и `DestroyWindowToolTipLayer`.
  `CloseServiceWindow(key)` — для удаления машины.
- `buildServiceView(ac, win, src, tab) (fyne.CanvasObject, func())` — тело
  окна; его же встраивает панель Local (§7). Возвращает `dispose`.
- Шапка (§5.1 SPEC): строка платформы (`<OS> · goos/goarch · service
  sing-box-lxd (<init>)`) + `Select` init (только удалённый Linux) + «Guides:»;
  строка версии `Core <running> (required <req> ⚠)`; строка состояния
  (зелёная `✅ answers · uptime 3 h` / красная `✖ Not answering for 12 min:
  <err>, N attempts`); строка `Commands run via ssh root@…` /
  `Commands run on this computer`.
- Вкладки: `AppTabs`; ярлык с глифом (`✖ Not running`, `⚠ Core`,
  `✖ Pairing`) по `serviceDiagnosis(snapshot)` — общая чистая функция в
  `service_model.go`, её же зовёт строка машины (единый вердикт).
- Стартовая вкладка — первая проблемная в порядке Not running → Core →
  Pairing; иначе Not running; явный `tab` из вызова (Deploy → Core) важнее.
- Ре-рендер: `src.Poll(onUpdate)`; `onUpdate` сравнивает новый снапшот со
  старым (`serviceSnapshotEqual`, без `Uptime`) и при изменении перестраивает
  шапку и **содержимое** вкладок (`TabItem.Content = …; tabs.Refresh()`),
  сохраняя выбранную вкладку. Виджеты с вводом/состоянием (поле приглашения,
  секрет, прогресс скачивания, раскрытие секций) создаются **один раз** и
  переиспользуются при перестройке — иначе тик стирал бы введённое.
- Каждая вкладка — `VScroll` с минимумом `serviceTabScrollMinHeight = 160`
  (константа): `AppTabs` берёт минимум по всем вкладкам, и высокая Core иначе
  держала бы высоту окна.
- Опасные секции (Roll back, Install from scratch, Remove the service,
  Revoke client) — свёрнутые `widget.Accordion`; заголовок красным
  (`widget.Label{Importance: DangerImportance}` рядом), перед ▶ —
  `ShowConfirm`.

### 6.3 Строка шага — `ui/service_step_row.go`

`serviceStepRow(win, title string, step core.ServiceStep, run serviceRunner) fyne.CanvasObject`:
номер+заголовок (Label с Wrapping), поле команды (`MultiLineEntry`,
`TextWrapWord`, `SetMinRowsVisible` по длине), справа ⧉ (`NewCopyButton`,
копирует `step.Command` без обёртки) и ▶. Подпись `default` при
`UsesDefault`. `serviceRunner` решает ▶:

| Случай | ▶ |
|---|---|
| удалённо, POSIX (procd/systemd/launchd) | `openTerminal(core.WrapSSH(target, cmd, tty))`, `tty` = команда с `sudo`; не-root пользователь + `NeedsRoot` → `sudo ` в команде |
| `RunsLocally` (upload) | `openTerminal(cmd)` без обёртки |
| удалённо, scm | ▶ нет (**[решение]**: PowerShell через ssh в cmd.exe-шелл не переносим); подпись «run in an elevated PowerShell on the machine» |
| локально macOS | `openTerminal(cmd)` |
| локально Windows | шаги с операцией (restart, bootstrap, install, fresh invite) — строки `daemonOps` из `LocalRows` («Run as administrator»); прочие — только ⧉ |
| `openTerminal == nil` (Windows/Linux-лаунчер) | только ⧉ |

### 6.4 Источник «машина» — `ui/service_source_remote.go` (без тегов)

- Держит `*machineListPanel`, `services.RemoteDaemon` (перечитывает запись
  из реестра на каждом опросе — SSH/init могли смениться в Edit).
- `Snapshot`: `p.healthOf(id)`, `p.livenessOf(id)`, `d.Passport`, `d.StateDir`,
  `MergeServicePaths`, `ClassifyDaemonReachError(err)`.
- `Poll`: тикер `serviceRemoteRefresh = time.Second` (как `wireLogRefresh`),
  читает кэш панели через `fyne.Do` — сети не трогает; сеть — только
  heartbeat. Окно зеленеет после `restart` на тике heartbeat (≤ 5–10 с).
- Не подключённая машина: снапшот из записи (паспорт `SeenAt`), шапка
  «not connected — press Connect»; рецепты строятся по кэшу/дефолтам.
- `Pair` — вынесенная из `machineEditRePair` функция
  `rePairMachine(registry, d, invite, addr, secret, done)` (общая для окна
  Edit и вкладки Pairing; закрытие окон машины — внутри).
- `OpenLiveLog` — `OpenMachineCoreLogWindow(ac, d)`; доступно только для
  активной машины (`lxdOverrideTransportForID`), иначе кнопка неактивна с
  подсказкой «Connect first».
- Вердикт строки — `machineServiceVerdict(d, h, live, connected)
  (level serviceLevel, line, tip string)`: `levelDown` (красный) —
  `markerFor == markerDown`, либо `InterruptedApply` при `live.FailStreak > 0`;
  `levelCore` (жёлтый) — `CompareCoreVersion == Older`; иначе нет. Им пользуются
  и строка (§8), и шапка окна.

### 6.5 Источник «локальный демон» — `ui/service_source_local.go` (тег `darwin || (windows && !386)`)

- `Poll`: тикер `serviceLocalRefresh = 5 * time.Second` + немедленный опрос;
  горутина `ac.DaemonStatusSnapshot()` → `fyne.Do`. Пока предыдущий опрос не
  вернулся, новый не стартует (флаг). Также `ops.after` зовёт внеочередной опрос.
- `DownSince`/`Attempts` — считает сам источник по переходам `Reachable`.
- `Paths`: `MergeServicePaths(passport, ac.DaemonServicePaths())`.
- `LocalRows` — перенесённые из `buildDaemonPanel` строки (§7).
- `OpenLiveLog` — `OpenLogViewerWindow(ac)` (вкладка Core в daemon-режиме уже
  читает `SubscribeLog`).

### 6.6 Ядро во вкладке Core удалённо — `ui/service_core_download.go`

Кнопка `Download <RequiredCoreVersion>` + `ProgressBar` + строка результата;
при открытии — `core.CheckTargetCore` в горутине (готовый файл → сразу ✓).
Прогресс — тот же приём, что `handleDownload` (`ui/core_dashboard_tab.go:984`):
канал `DownloadProgress` (буфер 10), `fyne.Do`, текст ошибки —
`downloadFailureReason`. Итог: `~/Downloads/<имя>  ✓ sha256 matches
SHA256SUMS` или `sha256 <binary> — SHA256SUMS unavailable, compare it in step 3`.
Путь файла подставляется в `core_upload`. Состояние скачивания живёт в окне
(переживает ре-рендер). Нет ассета для платформы (linux/386) — текст «no
release asset for linux/386», шаги 2–4 с плейсхолдером.

### 6.7 Живой лог машины — `ui/machine_core_log_window.go`

Окно на машину (карта), `MultiLineEntry` только для чтения с кольцом
`machineCoreLogMax = 1000` строк, перерисовка батчем раз в 250 мс;
`SubscribeLogLines` на открытии, `cancel` на закрытии;
`CloseMachineCoreLogWindow(id)` зовут те же места, что
`CloseMachineProfiler` (Connect другой машины, Disconnect, Remove, re-pair).

## 7. Local: панель перестраивается в окно Service

`buildDaemonPanel` (`ui/connection_local_daemon.go:61`) превращается в:

```
[подсказка «The core runs inside a system daemon» ?]
[✓ Stop VPN when quitting the launcher]          ← остаётся вне окна Service
┌ buildServiceView(local source) ───────────────┐
│ шапка                                         │
│ ✖ Not running │ ⚠ Core │ Pairing │ Reference │ Uninstall │
└───────────────────────────────────────────────┘
```

Перераспределение (SPEC §4.2) и решения:

| Было | Стало |
|---|---|
| Status: строки статуса `renderDaemonStatusText` | шапка. `renderDaemonStatusText` зовёт только эта панель (`:79`) → функция удаляется; её строки, не переиспользованные шапкой (`✅ System service installed`, `— Not paired`, `— Daemon engine is not active yet: … on the Install tab …` и др.), уходят из `ru.json` по списку orphan от `l10n_check` |
| Status: плашка `serviceBox` (текст) | строка в шапке (`ServiceNote`, красная/жёлтая) |
| плашка: install-строка (Unsafe/Stale/ProcessStale) | Core, шаг «Install or update service» |
| плашка: bootstrap-строка (NotRunning) | Not running, шаг «Load the service» |
| Install: install + `installCoreHint` | Core |
| Install: приглашение + Pair, «Need a fresh invite» | Pairing |
| Status: секрет + `secretHelp` (`DaemonShowSecretCommand`) | Pairing → «Plain mode»; команда показа секрета — Reference → daemon.json |
| Status: адрес демона | **[решение]** Pairing (параметр канала, рядом с секретом) |
| Status: `Stop VPN when quitting` | над окном Service (не входит в окно, SPEC §4.2) |
| ↻ `Refresh daemon status` | удаляется (окно обновляется само); ключ — из `ru.json` |
| — | Not running: status (`launchctl print` / `sc.exe query`), restart (`DaemonRestartService`), log, last-good |
| Uninstall | Uninstall без изменений (перенос кода целиком) |
| автовыбор Install | автовыбор первой проблемной вкладки |

- Локальная вкладка Core: шаг 1 — строка «Launcher core <ver> (required
  <req>)» и при устаревании `core.DaemonServiceCoreHint` (скачивание — на
  вкладке Core дашборда, **[решение]**: второй загрузчик ядра лаунчера не
  заводим); шаги 2–4 — одна строка `Install or update service`; Roll back нет;
  Remove — ссылка «→ Uninstall tab» (`tabs.Select`).
- Радио движка и `buildLocalEngineTab` (`ui/connection_local.go`) не меняются.
- `ui/connection_window.go`: контент оборачивается в
  `fynetooltip.AddWindowToolTipLayer` (тултипы ⚙/▶ в окне), снятие на закрытии.
- Глифы ярлыков локально: ✖ Not running — служба установлена и сопряжена,
  но `!Reachable`, либо `NeedsBootstrap`; ⚠ Core — `NeedsInstall`,
  `CoreTooOld`, ядро службы старее требуемого или ядро лаунчера устарело;
  ✖ Pairing — `!Paired` или `Reach ∈ {CertChanged, NotTrusted, ChannelMismatch}`.

## 8. Строка машины, Edit, Deploy

`ui/machine_list_panel.go`:
- ⚙ (`ttwidget.NewButton("⚙")`, LowImportance) → `OpenServiceWindow(ac,
  newRemoteServiceSource(p, d), tabAuto)`.
  - подключена: `statusRow` = `HBox(infoBtn, gearObj, powerBtn, restartBtn)`;
  - **[решение]** до Connect: ⚙ без точки слева от Connect в `metaRow` — окно
    полезно и без связи (кэш паспорта, рецепты), но выдумывать состояние
    точкой нельзя.
- Точка: `levelDown` → `withCornerDot(gear, ColorNameError)`, тултип «Daemon does
  not answer — open Service»; `levelCore` → `ColorNameWarning` (тот же цвет,
  что у Deploy•, — три точки единообразны), «Core is older than required —
  open Service».
- Строка предупреждения под `statusRow` (только при проблеме):
  `widget.Label` c `Wrapping = TextWrapWord`, `Importance = Warning/Danger`,
  текст `⚠ Core lx.11 is older than required (lx.14)` / `✖ <первая строка
  ошибки>`.
- `deployTo`: если `CompareCoreVersion(health.Version, Required) == Older` и
  `d.CoreWarnAck != health.Version` — `dialogs.NewCustom` (текст SPEC §4.3,
  чекбокс «Don't ask again for <lx.N> on this machine», кнопки How to update /
  Deploy anyway / Cancel) **вместо** прежнего `ShowConfirm` (двойного
  подтверждения нет); чекбокс → `SetCoreWarnAck(id, health.Version)` при Deploy
  anyway; How to update → `OpenServiceWindow(..., tabCore)`. Иначе — прежний
  `ShowConfirm`. Предупреждение возвращается само: ack хранит версию, новая
  версия ≠ ack.
- `connectMachine`/`disconnectMachine`/`removeMachine`: + `CloseMachineCoreLogWindow`;
  `removeMachine`: + `CloseServiceWindow(d.ID)`.

`ui/machine_heartbeat.go`: `machineLiveness.FailSince time.Time` (ставится на
переходе 0→1; Connect-провал ставит время первой неудачной попытки);
`healthChanged` (§5.2).

`ui/machine_edit_window.go`: поле **SSH** в форме паспорта после Address
(плейсхолдер — `DefaultSSHTarget(addr)`), Save → `SetSSH`; ошибка разбора —
`dialog.ShowError`. Re-pair → общий `rePairMachine` (§6.4). Команда
приглашения в Re-pair остаётся `sudo sing-box lxd client add` (точная —
во вкладке Pairing окна Service).

## 9. Гайды

`internal/constants/constants.go`:

```go
LauncherDocsBaseURL = "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/"
CoreDocsBaseURL     = "https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/"
```

`ui/service_guides.go`: `serviceGuide{Label, Base, Name, AnchorEN, AnchorRU}`,
`guideURL(g)`: при `locale.GetLang() == "ru"` — `<name>.ru.md#<AnchorRU>`, иначе
`<name>.md#<AnchorEN>` (якоря GitHub для кириллицы другие — таблица держит оба).
`guideLink(win, g)` — `widget.NewHyperlink` + `OnTapped → platform.OpenURL`
(образец `ui/machine_add_window.go:110–117`). Подписи-имена документов
(`lxd`, `OpenWrt`) — `// l10n-exempt: document name`.

| Где | Гайд → якорь EN / RU |
|---|---|
| шапка | lxd-daemon (без якоря); OpenWrt-машина: openwrt-vpn-ssid |
| Not running | TROUBLESHOOTING `#daemon` / `#демон`; lxd-daemon `#11-diagnosing-a-misbehaving-daemon` / `#11-диагностика-неисправного-демона` |
| Core | macOS `#7-macos--automatic-installation` / `#7-macos--автоматическая-установка`; Windows `#7a-windows--automatic-installation` / `#7a-windows--автоматическая-установка`; Linux `#8-linux--setup-approaches` / `#8-linux--подходы-к-настройке`; OpenWrt: openwrt-vpn-ssid |
| Pairing | `#9-pairing-a-client-the-same-on-every-os` / `#9-сопряжение-клиента-одинаково-на-всех-ос` |
| Reference | DAEMON_AND_REMOTE (без якоря); lxd-daemon `#3-daemonjson--the-daemons-settings` / `#3-daemonjson--настройки-демона` |

## 10. Debug API — не разойтись с окном

- `DaemonStatus` += `log_path`, `executable`, `listen`, `tls`,
  `uptime_seconds`, `reach_error`, `passport_cached`.
- `DaemonCommands` += `restart` (`DaemonRestartCommand`), `client_list`.
- `machineView` += `ssh`, `init_system`, `core_warn_ack`, `passport`;
  PATCH `/remote/machines/{id}` принимает `ssh`, `init_system`.
- `/remote/machines/{id}/health` += `core_required`, `core_outdated`
  (`CompareCoreVersion == Older`).
- **[решение]** Отдельного эндпоинта с рецептами не заводим: рецепты — чистая
  функция поверх полей, которые API теперь отдаёт; при надобности
  `GET …/service` добавляется одной строкой реестра позже.
- Тесты API не пишутся (политика: один тест на волну — в core).

## 11. Решения по открытым вопросам

1. `/admin/logs` (хвост лога демона по сети) — **не входит** в 161: SPEC
   требует только стрим ядра; команда `tail` и окно Logs покрывают случай.
2. Локальный кэш паспорта — в памяти (§5.3).
3. ⚙ до Connect — есть, без точки (§8).
4. Remote Windows — ▶ нет, только ⧉ (§6.3); remote darwin/windows — без шагов
   2–4 Core (§3).
5. «Неизвестная» версия ядра — без ⚠ (§2).
6. Restart на Windows — `Restart-Service -Force` (§3).
7. Бэкап ядра: procd `/root/…`, systemd рядом с бинарём; имя с полной версией (§3).
8. Адрес демона локально — во вкладке Pairing (§7).
9. Деплой со старым ядром — новый диалог заменяет старый confirm (§8).
10. Цвет жёлтой точки — `theme.ColorNameWarning` (§8).
11. Сайдкар `.sha256` в `~/Downloads` (§4).
12. `diagnoseReachError` переводится на общий классификатор (§3).

## 12. Тест (один на волну)

`core/service_recipes_test.go`, `TestServiceRecipes` — табличный:
- `CompareCoreVersion`: lx.11 < lx.14; lx.14-rc1 < lx.14; 1.14.2-lx.14 < 1.14.3-lx.14;
  равные; новее; `unknown`/апстрим → Unknown; `CoreVersionPairLabels`.
- `DefaultServiceInit`: linux/arm64, arm, mipsle → procd; linux/amd64 → systemd; darwin, windows.
- `BuildServiceRecipes` по четырём init: status/restart/log_tail/last_good с
  путями из паспорта и без (флаг `UsesDefault`), `core_swap` (имя бэкапа по
  версии и по дате), `core_upload` (`RunsLocally`, `-p` для нестандартного порта).
- `ParseSSHTarget` (`root@h`, `u@h:2222`, `h`, `[fe80::1]:22`, ошибки) и
  `WrapSSH` (одинарная кавычка в команде, `-t` с sudo).
- `ClassifyDaemonReachError` на образцах строк.
- `SingboxAssetSuffixFor`: linux/arm64, arm → armv7, mipsle → softfloat, linux/386 → "".

Запуск: `go test ./core -run '^TestServiceRecipes$' -count=1`.

## 13. Документация

- `docs/TROUBLESHOOTING.md` + `.ru.md`: новый раздел `## Daemon` / `## Демон`
  (якоря `#daemon` / `#демон` — на них ведёт окно): «не поднимается»
  (лестница status → restart → лог → last-good → переустановка), «обновить
  ядро на машине» (download → `cat >` по ssh → check → backup/swap → rollback),
  «сопряжение» (когда re-pair, `client add/list/remove`), «где что лежит»
  (таблица путей procd/systemd/macOS/Windows, ключи daemon.json). Команды —
  те же, что строит `core/service_recipes.go` (при правке рецептов править и
  таблицу).
- `docs/DAEMON_AND_REMOTE.md` + `.ru.md`: §2 — окно Service локально (вкладки,
  что куда переехало); §4.1 — ⚙ и строка предупреждения; §4.2 — новые поля
  записи; §4.4 — предупреждение о версии ядра при Deploy.
- `docs/API.md` + `.ru.md`: новые поля §10.
- `docs/ARCHITECTURE.md`: L6 — окно Service и окно лога машины; §11 —
  `core/service_recipes.go`, `core_build.go`, источник local/remote.
- `docs/release_notes/upcoming.md` (EN Highlights / RU Основное), `RELEASE_NOTES.md`
  (значимый UX).
- `SPECS/README.md` — строка 161 при закрытии.

## 14. Ловушки

- **Win7 = go1.20**: без `min`/`max`/`clear`, `slices`, `maps`, `range` по
  int, `PathValue` в общем коде (`core/core_build.go`,
  `core/service_recipes.go`, `core/core_download_target.go`, все нетегированные
  `ui/service_*.go`). Страж: `go run ./tools/win7guard`.
- **Теги**: нетегированный UI не ссылается на `core.DaemonOpsElevated`,
  `ac.DaemonInstallOrUpdate`, `daemonOps`, `buildDaemonPanel` — только через
  `serviceSource.LocalRows` из тегированного `service_source_local.go`.
  Проверка локально: пакет `core` кросс-собрать нельзя (тянет Fyne через
  `core/uiservice`, нужен CGO) — кросс-сборка работает только для
  `GOOS=linux go build ./core/services/... ./internal/...` (проверено 10.10);
  для `core` и `ui` — `GOOS=linux go list -f '{{.GoFiles}}' ./core ./ui`
  (новые файлы обязаны быть в списке), `go run ./tools/win7guard` и CI
  (Win7-сборка с `go.win7.mod` локально запрещена).
- **AppTabs = максимум по вкладкам**: корень вкладки — VScroll с
  константным минимумом; никаких долей высоты окна.
- **Label min-width**: длинные строки (ошибки, пути) — только с `Wrapping`;
  команды — `MultiLineEntry` с переносом, не Label.
- **Перерисовка**: `healthChanged` без uptime; `SetPassport` без записи файла
  на каждый тик; окно перестраивает только при изменении снапшота, ввод не
  теряет.
- **l10n**: каждый новый `locale.T` — сразу в `bin/locale/ru.json`; удалённые
  ключи (`Refresh daemon status`, подписи вкладок Status/Install, если уйдут) —
  удалить из `ru.json` (CI `l10n_check --strict` роняет orphan). Литералы с
  буквами в display-позициях без `locale.*` роняют `hardcoded_check`
  (`// l10n-exempt:` только для имён документов/продуктов). Проверка:
  `go run ./tools/l10n/l10n_check --strict` и `go run ./tools/l10n/hardcoded_check --strict`
  (это не тесты — разрешено локально).
- **Глифы**: ⚠ и ✖ в ярлыках вкладок — проверяет владелец визуально; узкие
  эмодзи в Fyne режутся (`⚡`) — не использовать.
- **Тесты**: только `TestServiceRecipes`; UI без тестов; `go test ./...`,
  `go vet ./...` локально запрещены — полный прогон в CI
  (`gh workflow run ci.yml --ref develop -f run_mode=tests`).
- Секреты в `remote-daemons.json` лежат открыто намеренно.
