# SPEC 141 · Отчёт о реализации (фаза 2, core)

Дата: 24.09.2026. Ветка `spec-141-windows` (после слияния develop c3fcffff:
SPEC 139, 140, пин ядра 1.14.2-lx.2-rc.1). Этот заход — core/internal/main;
панель Local (`ui/**`) делается отдельно в `spec-141-windows-ui`.

## Что сделано

1. **Платформенный слой** (`internal/platform`, `windows && !386`).
   `winservice_windows.go` — раскладка через `windows.KnownFolderPath`
   (`PrivilegedCopyDir`, `PrivilegedDataDir`); `QueryService` без прав
   (`SC_MANAGER_CONNECT`, `QUERY_CONFIG|QUERY_STATUS|READ_CONTROL`, только
   `QueryServiceConfig` → `BinaryPathName`, `QueryServiceStatus`, DACL службы);
   `ServiceExecutablePath` (`DecomposeCommandLine`, `\??\`, `%SystemRoot%`,
   путь с пробелом без кавычек); инвариант §6.1 — порт `lxd/aclcheck.go` +
   `execsafe_windows.go` ядра (allowlist SYSTEM / Administrators /
   TrustedInstaller, маски предков и копии, reparse, NULL DACL, локальный
   NTFS) — `CheckProtectedCopy`; `StatFileKey` (VolumeSerialNumber,
   FileIndex, size, mtime). `sysproxy_windows.go` — порт `wininet` ядра
   (`PER_CONNECTION_OPTION` + SETTINGS_CHANGED / PROXY_SETTINGS_CHANGED /
   REFRESH), чтение `ProxyEnable`/`ProxyServer` из HKCU. `privileged_windows.go`
   — имя копии `sing-box-lxd.exe`, `PrivilegedCoreLogPath`,
   `OpenPrivilegedCoreLog`. runas-исполнитель — `RunElevated` + `Wait` из
   SPEC 139, без изменений. `privileged_stub.go` → `!darwin && (!windows || 386)`.
2. **Классификатор** (`core/daemon_service_state_windows.go`). NotInstalled
   (1060) / Unsafe (конфигурация, разбор, кавычки, argv[0] ≠ копии без учёта
   регистра, DACL службы, цепочка) / Stale (exe ↔ `sing-box.exe`,
   `libcronet.dll` — присутствие и sha, лишний файл; остатки `.old` /
   `.<член>.tmp-` и сайдкар — не лишние; `CopyMissing`) / NotRunning
   (≠ `SERVICE_RUNNING`) / ProcessStale (общий шаг, путь без учёта регистра) /
   OK; `MismatchFile`, `ExtraFile`, `LaunchdState` = состояние SCM. Порог
   `minCoreForRootOwnedService` — константа платформы: darwin `1.14.1-lx.12`,
   windows `1.14.2-lx.2-rc.1` (rc и релиз lx.2 проходят, 1.14.2-lx.1 и
   1.14.1-lx.13 — нет). Сверка набора (`daemonSetMismatch`) — и в
   классификаторе, и в гейте classic. `daemonEngineAvailable` на Windows
   открыт (движок, Debug API `/daemon/*`).
3. **Менеджер** (`core/daemon_manager_windows.go`). Install / Start
   (`sc.exe start`, 1056 = успех, ожидание RUNNING до 10 с) / Fresh invite /
   Uninstall (`--keep-copy`, `--purge`) / Copy через `RunElevated`
   (`SW_HIDE`, рабочий каталог System32, ожидание 120 с, отказ UAC —
   `Cancelled`, таймаут — `TimedOut`). Приглашение — `<Data>\bin\daemon\
   invite-<hex>.txt`: ядро пишет, лаунчер читает, сопрягается, удаляет; в
   лог не пишется. Install код 1 → `--service=status` без прав → ≠ 0 —
   вердикт; 0, служба есть, файла нет — `NoInvite` + `FreshInvite`. Warnings
   сайдкара — в результат и WARN. Имя клиента `singbox-launcher-<user>`
   (SAM, lowercase, `[a-z0-9_-]`, ≤ 64) — в `--invite-name` / `--name`, в
   показываемой команде install тоже. `DaemonShowSecretCommand` —
   `Select-String` по `daemon.json` службы. Uninstall — снять свой прокси.
4. **Системный прокси** (§7). `prepareConfigForDaemonWith`: шаг (3) —
   `set_system_proxy` → `false`, адрес первого как у ядра (пусто/unspecified →
   127.0.0.1, `http://host:port`), второй и далее — WARN; шаг (4) —
   `state_directory` tailscale под DataDir → `<StateDir>\tailscale\<тег>`.
   `config.json` на диске не меняется. Set — после успешного apply и на
   кадре STARTED; «снять своё» (метка `daemon_system_proxy` = HKCU) — apply
   без прокси, кадр IDLE/FATAL (в т. ч. первый кадр после краша), StopVPN,
   выход с `daemon_stop_vpn_on_exit`, daemon → classic, Unpair, Uninstall.
   macOS — no-op, шаги (3)–(4) выключены.
5. **Classic под правами** (§8). Повышенный лаунчер (любой конфиг) → гейт
   копии над набором → `exec` копии, `Dir` = `<Data>\bin`, вывод в
   `classic.log` (цепочка `ProgramData\sing-box-lxd\logs` по §6.1, нет
   `logs\` — диалог `missing`, ротация > 2 МиБ, открытие без следования
   ссылкам, явный DACL SY/BA + чтение пользователю токена на каждом
   старте); `CoreLogPath` отдаёт classic.log. «Already running» видит
   `sing-box-lxd.exe` в сессии ≠ 0 и снимает его только по PID; служба
   (сессия 0) не видна.
6. **Диалоги в core.** «TUN без прав» — первая кнопка **Install service**
   (install → сопряжение → движок daemon в settings.json → Start; после
   `NoInvite` та же кнопка делает fresh invite). Гейт classic, «Core
   updated», модальное Unsafe (main.go) — `ShowActions` с **Run as
   administrator** / Copy the command (/ Retry). В `internal/dialogs` —
   `ActionsDialog.SetBusyStatus` (строка ожидания при выключенных кнопках).
7. **После 139+140.** `glprobe_windows.go`: проба записи —
   `paths.AppDirUserWritable`; D5 — подсказка переустановить без задачи
   Mesa, если программа в защищённой папке.

Locale: 10 новых ключей с RU (`Run as administrator`, `Install service`,
тексты гейта / Core updated / Unsafe для Windows). Ключ `Run as
administrator` UI-ветка по API_WINDOWS.md добавляет тоже (с тем же
переводом) — при слиянии дубль в `ru.json` оставить один. `ui/**` не
тронут. Release notes — EN/RU в `upcoming.md`.

## Отступления от SPEC

- WinINet ставится и снимается через `INTERNET_OPTION_PER_CONNECTION_OPTION`
  (порт ядра, §7 / PLAN), реестр HKCU — только чтение: WinINet сам
  обновляет `ProxyEnable`/`ProxyServer`.
- SCM не открылся — вердикт `not_installed` с причиной в `Detail` (в §6.2 —
  «не судим»; отдельного состояния нет).
- Сверка прокси по «порогу промахов канала» (§7, краш лаунчера) не сделана —
  только по кадру статуса.
- Лишний файл в каталоге копии — Stale с `ExtraFile`; ядро на неизвестный
  файл install отказывает («remove it»), так что лечится удалением файла,
  а не install.
- Общие тексты CoreTooOld и `diagnoseReachError` по-прежнему говорят про
  root/sudo — на Windows не переписаны.
- `classic.log`: чтение выдаётся SID токена повышенного процесса; при
  повышении чужой учётной записью это администратор, а не исходный
  пользователь.

## Не сделано

- UI-панель Local (другая ветка); без неё операции доступны из диалогов
  core и Debug API.
- Лог повышенного classic из неповышенного лаунчера (§8 «без прав файл
  читается по полному пути») — неповышенный лаунчер показывает свой лог.
- Доки `DAEMON_AND_REMOTE*`, `API*`, `ARCHITECTURE*`, отметки TASKS.md —
  по указанию владельца (только upcoming.md и этот отчёт).
- Ручная приёмка §12 на Windows 10/11.

## Проверки

- `go build ./...` (macOS) — чисто.
- Сверка типов (утилита SPLIT_REPORT) `core` с тестами: windows/amd64,
  windows/arm64, windows/386, linux/amd64 — 0 ошибок; `main`, `ui` —
  windows/amd64 и windows/386 — 0; `GOOS=windows GOARCH=amd64|arm64|386
  CGO_ENABLED=0 go build ./internal/...` — чисто.
- `go test ./core/ -run TestPrepareConfigForDaemonSystemProxy` — ok
  (единственный новый тест). `TestServiceCoreVersionGate` дополнен порогом
  Windows.
- CI `ci.yml` `run_mode=build`: run 36035927895 — красный только
  `l10n_check --strict` (orphan: текст macOS «Core updated» переехал в
  другой файл от константы); исправлено (bf1d9781), локально
  `l10n_check` / `hardcoded_check` / `paths_guard --strict` — чисто. Run
  36037643673 — зелёный: тесты ubuntu / macOS / Windows, сборки macOS,
  Win64, Win7 (x86), установщик Win64.
