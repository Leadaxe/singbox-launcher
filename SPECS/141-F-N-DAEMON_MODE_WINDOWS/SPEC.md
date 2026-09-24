# 141 · Daemon-режим на Windows (служба sing-box-lxd)

Перенос daemon-движка SPEC 096 и модели защищённой копии ядра SPEC 136/137
на Windows. Пара к SPEC 103 форка (`sing-box-lx`, ядро **v1.14.2-lx.2**,
линия v1.14.2: SCM-служба, `--service=*` на Windows). Смежные: **SPEC 139**
(лаунчер `asInvoker`, `IsElevated`, диалог «TUN без прав» с кнопкой
«Install service», перезапуск через `runas`, автозапуск HKCU Run) и
**SPEC 140** (установщик Inno, галочка «Install sing-box-lxd service») —
здесь только ссылки.

Статус: **черновик, 24.09.2026**. Интерфейс ядра подтверждён сессией ядра
по всем пунктам (§3), развилки решены владельцем (§13).

## 1. Мотивация

1. **Дыра из SPEC 137 §8 п. 5.** Лаунчер с `requireAdministrator`
   (`app.manifest`) запускает `sing-box.exe` из
   `%LOCALAPPDATA%\singbox-launcher\bin`, куда пишет любой процесс
   пользователя обычной целостности: подмена файла = код с высокой
   целостностью на ближайшем старте VPN. SPEC 139 снимает
   `requireAdministrator`, но TUN по-прежнему требует прав.
2. **VPN без залогиненного пользователя.** Служба стартует с системой
   (`StartAutomatic`, `Tcpip`), демон поднимает last-good, если run-state —
   «работал» (`lxd/daemon.go`, `bootstrap`): TUN есть до входа и после
   выхода, а лаунчеру права не нужны вовсе.
3. **Один движок на двух ОС.** Daemon-код за тегом darwin, хотя
   платформенного в нём — launchd, plist, uid и терминал; `lxdclient`,
   `daemonpb`, `core/services/lxd_remote_*.go`, маркер дашборда и
   remote-override уже кросс-платформенны, grpc есть в `go.win7.mod`
   (`docs/DAEMON_AND_REMOTE.md` §1, §6 здесь устарели).

## 2. Решение (владелец, закреплено)

| | Решение | Где |
|---|---|---|
| A | общее с macOS — из `*_darwin.go` в общие файлы, платформенное — `_darwin.go` / `_windows.go`; Linux и Win7 — заглушка; логику не переписывать, только делить | §4 |
| B | команды под правами — `ShellExecuteEx` `runas` (стандартный UAC), без терминала; ждать завершения, читать код выхода; текст команды — для Copy | §5 |
| C | те же состояния SPEC 136 §4: launchd → SCM, uid 0 → владелец/DACL, plist → `BinaryPathName`, сайдкар — sha всего набора; гейт ядра **≥ 1.14.2-lx.2** | §6 |
| D | системный прокси в daemon-режиме ставит и снимает лаунчер в профиле пользователя; демону `set_system_proxy` не передаётся | §7 |
| E | classic с правами исполняет только копию из `C:\Program Files\sing-box-lxd\` (инвариант + sha, как SPEC 137); лог — `C:\ProgramData\sing-box-lxd\logs\classic.log` | §8 |
| F | панель Local: переключатель движка, плашки, install (runas), Uninstall `--keep-copy`, «Remove all data» — полный uninstall, диалог после Core → Download | §9, §10 |
| G | установщик (SPEC 140) ставит службу той же командой; здесь — команда и идемпотентность | §5.4 |
| H | `daemon_secret` / `daemon_server_fingerprint` — в `settings.json`, как сейчас (by design) | — |

## 3. Интерфейс ядра v1.14.2-lx.2 (SPEC 103 форка)

Норма ядра — SPEC 103 форка:
`sing-box-lx/SPECS/TASKS/103-LXD_WINDOWS_SERVICE/SPEC.md`, тег **v1.14.2-lx.2**.

Подтверждён сессией ядра 24.09.2026 по всем пунктам. Только amd64/arm64 с
`with_lxd`; Win7 (`windows-386-legacy-windows-7`) — без службы. `C:\` в
тексте — пример: пути ядро и лаунчер берут через `windows.KnownFolderPath`
(`FOLDERID_ProgramFiles`, `FOLDERID_ProgramData`).

| # | Что | Значение | У лаунчера |
|---|---|---|---|
| 1 | Служба | SCM `sing-box-lxd`, `LocalSystem`, `StartAutomatic`, зависимость `Tcpip`, restart-on-failure; `BinaryPathName` в кавычках, argv[0] — путь копии, аргументы как на macOS | §6.2 Unsafe |
| 1a | DACL службы | `(A;;0x2008d;;;AU)` — `QUERY_STATUS`, `QUERY_CONFIG`, `READ_CONTROL` для Authenticated Users, без `CHANGE_CONFIG`/`WRITE_DAC`/`WRITE_OWNER`/`DELETE`/`START`/`STOP`; SYSTEM и Administrators — полный доступ | классификатор без прав; «Start the service» — через runas (§5.1) |
| 2 | Команды | `lxd --service=install\|copy\|status\|uninstall [--keep-copy] [--purge]`; копия — `<ProgramFiles>\sing-box-lxd\`, каталог данных — `<ProgramData>\sing-box-lxd\` (SYSTEM + Administrators): `state\` = `<StateDir>` (`DefaultServiceStateDir`, как `state/` на macOS: `daemon.json`, tls, клиенты, `resources`, `tailscale`) и `logs\` (п. 8); `--purge` удаляет весь `<ProgramData>\sing-box-lxd`, включая `logs\classic.log`; install/copy/uninstall — только с elevated token; строка рестарта в сводке install — `Restart-Service sing-box-lxd` (PowerShell от администратора) | «Remove all data» (§9); NotRunning — `sc.exe start` под runas (§5.1) |
| 2a | Захват `ProgramData` | install и copy забирают владение `<ProgramData>\sing-box-lxd` и заменяют DACL целиком на всём дереве без исключений (`state\`, `logs\` и содержимое, в том числе `classic.log`; с явными строками вывода); отказ — только если владение забрать нельзя. Секрет и серверная пара не перегенерируются; если до install каталог принадлежал чужому SID или был ему читаем — `WARN: <путь> was readable by <имя> (<SID>) before this install; rotate the admin secret …`, в сайдкар — код `state_dir_foreign_before_install` (п. 3) | `daemon_secret` в `settings.json` остаётся валидным; WARN — §5.3; чтение `classic.log` — §8 |
| 3 | Набор копии | `sing-box-lxd.exe` + `libcronet.dll` (если лежит рядом с источником); `wintun.dll` не нужен — `sing-tun` несёт его `go:embed`. Сайдкар `sing-box-lxd.install.json`: `files[{name, sha256}]`, `source`, `version`, `installed_at`, `service`, `warnings[{code, text}]` — предупреждения install (п. 2a; SPEC 103 §2.5: каждый install/copy переписывает поле заново, даже при неизменном наборе; поля нет или массив пуст — предупреждений нет); идемпотентность по sha набора; лишний файл в каталоге копии — MISMATCH (остатки п. 3a — не лишние) | §6.2 Stale; `warnings` — §5.3 |
| 3a | Замена образа | stop службы → temp `.<имя>.tmp-<hex>` + fsync + sha → rename в `<имя>.old` → rename на место → удалить `.old` (занят — удалит следующий install); copy без службы — то же без stop. Остатки `<член набора>.old` и `.<член набора>.tmp-<hex>` (пока старый образ исполняется classic'ом) — не MISMATCH: `--service=status` их называет, вердикт не меняет | §6.2 Stale, §10 |
| 4 | Инвариант | модель `boxdd`: предки — владелец из {SYSTEM, Administrators, TrustedInstaller}, чужим SID нельзя `DELETE`/`WRITE_DAC`/`WRITE_OWNER`/`GENERIC_WRITE\|ALL`/`FILE_DELETE_CHILD`; каталог копии и файлы — сверх того `FILE_WRITE_DATA`/`APPEND_DATA`/`WRITE_EA`/`WRITE_ATTRIBUTES`; «чужой» — любой SID вне allowlist; без reparse points; локальный NTFS | проверяет так же (§6.1) |
| 5 | Самопроверка | строгий отказ — только при `svc.IsWindowsService()`; иначе WARN; `--allow-unsafe-exec` | classic с правами — гейт лаунчера (§8) |
| 6 | `--service=status` | 0 OK, 2 MISMATCH/UNSAFE, 3 NOT INSTALLED, 4 COPY ONLY, 5 NOT RUNNING (установлена, но не `SERVICE_RUNNING`), 1 ошибка; `QueryServiceStatus` без прав | вызывает только после install с кодом 1 (§5.3); иначе — для ручной проверки |
| 7 | Приглашение | `--invite-out <file>` у `--service=install` (создаёт клиента `--invite-name`, по умолчанию `singbox-launcher`, лаунчер на Windows передаёт имя на пользователя `singbox-launcher-<user>` (§5.3); `--invite-name` есть только у install; повторный install без флага клиентов не трогает) и у `client add` (имя — существующий `--name`); O_EXCL, без следования ссылкам, отказ, если файл есть; в stdout не печатается; флаг на всех платформах. Демон не дал приглашение за 15 с — install с `--invite-out` выходит с кодом 1: служба остаётся установленной, файл удаляется. Enroll по приглашению с именем заменяет запись клиента с тем же именем, старый сертификат отзывается | §5.1, §5.3 |
| 8 | Логи | install и copy создают `<ProgramData>\sing-box-lxd\logs\` (SYSTEM + Administrators, наследуемый DACL); демон пишет туда `lxd.log` с ротацией (путь install записывает в `daemon.json` ключом `log_file`); Event Log нет | `classic.log` — файл лаунчера там же (§8) |
| 9 | `/admin/info` | `executable` (путь exe), `executable_sha256` | ProcessStale |
| 10 | Под SYSTEM | рабочий каталог службы — `<StateDir>` (ядро делает `Chdir` в `Execute`): относительные пути конфига — в `state\`, не в `System32`; state tailscale по умолчанию — `<StateDir>\tailscale`, каталог `<тег>` создаёт демон при apply (DACL от StateDir); `find_process` без ограничений; системный DNS без изменений; системный прокси — сторона лаунчера | §7 |
| 11 | Канал | TCP loopback + mTLS, порт из `daemon.json`; named pipe нет | как на macOS |
| 12 | Поиск DLL | `SetDefaultDllDirectories(APPLICATION_DIR \| SYSTEM32)` — в `init` пакета `main` ядра, тег `with_lxd && windows`; `libcronet.dll` (purego) грузится после, по полному пути: в контексте службы и при повышенном `run` — только из каталога exe, нет её там — отказ старта (naive-outbound) без поиска по `PATH` | условие релиза §8 закрыто ядром |

Референс — `experimental/boxdd/` форка (`cmd_service_windows.go`,
`security_windows.go`).

## 4. Разделение файлов

«Daemon-платформы» — тег `darwin || (windows && !386)`; заглушки —
`!darwin && (!windows || 386)` (Linux и Win7). Файл с суффиксом
`_darwin.go` без платформенного кода переименовывается и получает тег.

| Сейчас | Общее (тег daemon-платформ) | darwin | windows |
|---|---|---|---|
| `core/backend_daemon_darwin.go` | `DaemonBackend` целиком: apply/stop/exit, стримы статуса и логов, `daemonProxyTransport`, `diagnoseReachError` | — | хук системного прокси (§7) |
| `core/backend_daemon_dns_darwin.go`, `…_traffic_darwin.go`, `…_tailscale_darwin.go` | целиком (только grpc/daemonpb) | — | — |
| `core/chain_probe.go` | целиком (меняется только тег) | — | — |
| `core/debugapi_wiring_daemon_darwin.go` | целиком | `Kickstart` | `Kickstart` = "" |
| `core/daemon_manager_darwin.go` | `prepareConfigForDaemon`, `DaemonUIStatus`, `DaemonStatusSnapshot`, `daemonServiceCheck`, `CoreSupportsLxd`, Pair/Unpair/SetAddress/SetSecret, `followDaemonPlainChannel`, `notifyDaemonServiceAfterCoreUpdate`, сборка команд (§5) | plist, метка launchd, `readPlistProgramPath`, bootstrap/kickstart, `OpenTerminalWithCommand`, `appleScriptString`, `shellQuote`, рендер `sudo …`, show-secret | имя службы SCM, исполнитель `runas`, рендер команды, старт службы, show-secret |
| `core/daemon_service_state_darwin.go` | типы состояний, `NeedsInstall`/`NeedsBootstrap`/`CopyUsable`, гейт и разбор версий ядра, сверка sha (обобщается на набор), `compareDaemonServiceProcess`, кэши sha и версии (без ключа), `shortSHA` | раскладка `/Library/…`, цепочка по `Stat_t`, `launchctl print`, legacy lx.11, ключ кэша `(dev, inode, size, mtime)` | раскладка, SCM, владелец/DACL, ключ кэша (§6) |
| `core/classic_privileged_darwin.go` | вердикты гейта, `checkPrivilegedCoreCopy` над набором, выбор команды copy/install, диалог | AEWP-путь, тексты «root» | тексты «administrator», лог (§8) |
| `core/purge_darwin.go` | целиком | — | — |
| `core/process_detect_darwin.go` | — | как есть | копия в сессии пользователя vs служба в сессии 0 (§8) |
| `ui/connection_local_daemon_darwin.go` | панель целиком | подписи «Run in Terminal» | подписи «Run as administrator» (§9) |
| `ui/command_row_darwin.go` | — | хук `openTerminal` | `command_row_windows.go`: хук → `runas` |
| `internal/platform/privileged_stub.go` | — | — | имя копии, `PrivilegedCoreLogPath` переезжают в `privileged_windows.go`; AEWP-функции остаются заглушкой |

Тесты с тегом darwin делятся по тому же правилу.

## 5. Менеджер: команды через runas

### 5.1 Команды

Команда хранится структурой `{Binary, Args}`: исполняется argv, а строка —
только для показа и Copy (PowerShell: `& '<путь>' <args>`, `'` → `''`).

| Операция | Binary (`lpFile`) | Args |
|---|---|---|
| Install or update service | ядро лаунчера `<CoreDir>\sing-box.exe` | `lxd --service=install --invite-out <file> --invite-name <имя клиента>` — одно окно UAC (§13 п. 2); `<имя клиента>` — §5.3 |
| Uninstall (вкладка, Debug API) | копия, если `CopyUsable`, иначе ядро лаунчера | `lxd --service=uninstall --keep-copy [--purge]` |
| Uninstall в «Remove all data…» | то же правило | `lxd --service=uninstall --purge` |
| Свежее приглашение | то же правило | `lxd client add --name <имя клиента> --invite-out <file>` |
| Копия для classic (службы нет) | ядро лаунчера | `lxd --service=copy` |
| Запустить службу (NotRunning) | `%SystemRoot%\System32\sc.exe` (runas: `START` у AU нет, §3 п. 1a) | `start sing-box-lxd` |
| Kickstart | — | нет (Debug API — пусто) |

install и copy проходят гейт версии (§6.2); ниже 1.14.2-lx.2 команды нет —
подсказка обновить ядро, как SPEC 136 §4.1.

### 5.2 Последовательность

1. Команда показана с кнопками **Copy the command**, **Run as
   administrator** (у гейта classic — ещё **Retry**, **Close**).
2. Нажатие → горутина: `ShellExecuteExW` (`runas`,
   `SEE_MASK_NOCLOSEPROCESS | SEE_MASK_NOASYNC`, `SW_HIDE` — ядро
   консольное; `lpParameters` = `windows.ComposeCommandLine(Args)`); в
   `x/sys/windows` только `ShellExecute`, Ex-вариант — через `shell32.dll`.
   UI не блокируется («Waiting for the administrator command…»).
3. `WaitForSingleObject(hProcess, 120 с)` → `GetExitCodeProcess`; таймаут —
   «still running», процесс не убивается. Затем — пересчёт классификатора.

### 5.3 Ошибки

| Исход | Показ |
|---|---|
| `ERROR_CANCELLED` (1223) — пользователь отказал в UAC | INFO в лог, строка «The administrator prompt was cancelled.»; диалог остаётся открытым |
| иная ошибка `ShellExecuteExW` | текст ошибки + команда для Copy |
| код выхода ≠ 0 (install с кодом 1 — ниже) | «The command failed (exit code N). Run it in an elevated terminal to see its output:» + команда для Copy; вывод процесса под `runas` не перехватывается |
| `client add`, код 0 | лаунчер читает файл `--invite-out`, `PairDaemonWithInvite`, удаляет файл; приглашение в лог не пишется |
| install, код 0 | то же, что у `client add`: файл `--invite-out` → сопряжение; поле ручной вставки остаётся |
| install, код 1 | сначала `--service=status` ядром лаунчера (без прав, как классификатор §6). status ≠ 0 — строка общего случая и вердикт классификатора, повторное сопряжение не предлагается. status = 0, служба есть в SCM, файла `--invite-out` нет — приглашение не получено (§3 п. 7): WARN в лог, строка «The service is installed, but no invite was received. Pair it as a separate step:» и команда «Свежее приглашение» (§5.1: `client add --name <имя клиента> --invite-out <file>`) с кнопками Copy / Run as administrator — второе окно UAC; её код 0 — как у `client add` выше Причина раннего провала install (до выдачи приглашения) лаунчеру не видна: вывод под runas не перехватывается, при status = 0 пользователь получит только это сообщение — известное ограничение |
| после шага install/update в сайдкаре (§3 п. 3) есть `warnings` | лаунчер читает сайдкар, показывает предупреждения в результате шага и пишет их в лог (WARN); `state_dir_foreign_before_install` — «The service data folder was readable by another account before this install. Rotate the admin secret in daemon.json and pair again if this computer is shared.», прочие коды — `text` как есть; поле переписывается каждым install/copy, устаревших предупреждений не бывает; список кодов — SPEC 103 §2.5 |

Файл приглашения — `<Data>\bin\daemon\invite-<случайное>.txt` (ядро:
O_EXCL, без следования ссылкам). При elevation чужими учётными данными файл
пишет администратор, ACL наследуется от профиля — пользователь его читает.

**Имя клиента и повторное сопряжение.** Имя клиента на Windows — на
пользователя: `singbox-launcher-<user>`, где `<user>` — имя учётной записи
(SAM account name) в нижнем регистре, символы вне `[a-z0-9_-]` заменяются на
`_`, итоговое имя — не длиннее 64 символов; оно уходит в `--invite-name` у
install и в `--name` у `client add` (§5.1). Норма ядра с lx.2: после обрезки
пробелов по краям от 1 до 64 рун, только печатные символы; нарушение → HTTP 400
`client name: …` и ненулевой код у install / `client add`. Нормализованное имя
укладывается в неё с запасом. Enroll по приглашению с именем
заменяет запись клиента с тем же именем, старый сертификат отзывается (§3
п. 7). Поэтому «Install or update service» не копит клиентов, а учётные
записи не выбивают пары друг друга (§11), но прежняя пара этого пользователя
после переустановки не сохраняется: чтение нового invite-файла и сопряжение
по нему — обязательный следующий шаг после install/update, а не опция. Пара,
вытесненная другим enroll под тем же именем, не работает до чтения нового
приглашения («Need a fresh invite», §9).

### 5.4 Идемпотентность (для SPEC 140)

install идемпотентен по sha набора (копия не трогается), сохраняет
`daemon.json`, секрет и клиентов, перезапускает службу (повтор установщиком
— моргание VPN). Сопряжение — не дело установщика (пара в `%LOCALAPPDATA%`):
первый запуск лаунчера видит «OK, не сопряжено» и предлагает Pair.

## 6. Классификатор

### 6.1 Инвариант (лаунчер, без прав)

Цепочка: корень тома → `Program Files` → `sing-box-lxd` → файлы набора;
по каждому звену — атрибуты без следования ссылкам и
`GetNamedSecurityInfo(OWNER | DACL)`. «Чужой SID» — любой, кроме SYSTEM,
Administrators, TrustedInstaller (список разрешённых, как
`trustedAdministrativeUser` `boxdd`; ядро проверяет так же, §3 п. 4;
detail называет SID).

- не reparse point; каталоги — каталоги, файлы — обычные; том —
  `DRIVE_FIXED` и NTFS;
- владелец копии и набора — Administrators или SYSTEM (норма ядра); у
  предков допустим и TrustedInstaller (владелец `C:\Program Files`);
- предки: ACE `ACCESS_ALLOWED` (кроме `INHERIT_ONLY`) чужого SID без
  `DELETE`, `WRITE_DAC`, `WRITE_OWNER`, `GENERIC_WRITE|ALL`,
  `FILE_DELETE_CHILD` (маска `boxdd`: создание папок в `C:\` не мешает);
- каталог копии и набор: сверх того без `FILE_WRITE_DATA`/`ADD_FILE`,
  `FILE_APPEND_DATA`/`ADD_SUBDIRECTORY`, `FILE_WRITE_EA`,
  `FILE_WRITE_ATTRIBUTES` — чужой файл рядом с exe = подсадка DLL;
- DACL службы (`QueryServiceObjectSecurity`, §3 п. 1a): чужому SID —
  ни `SERVICE_CHANGE_CONFIG`, ни `WRITE_DAC`/`WRITE_OWNER`/`DELETE`/
  `GENERIC_WRITE|ALL`.

### 6.2 Состояния

Вердикт — первый сработавший сверху вниз; тексты плашек и Debug API
(`service_state`, `service_path`, `service_detail`) — SPEC 136 §4, §8.

| Состояние | macOS (SPEC 136) | Windows |
|---|---|---|
| **NotInstalled** | plist нет | `OpenService` → `ERROR_SERVICE_DOES_NOT_EXIST` (1060) |
| **Unsafe** | plist не разобрался; `ProgramArguments[0]` ≠ копия; цепочка нарушена | `QueryServiceConfig` не прочитался (в т. ч. отказ в доступе); `BinaryPathName` не разбирается `DecomposeCommandLine`; argv[0] ≠ канонической копии (без учёта регистра, после `Clean`); путь с пробелом без кавычек; DACL службы (§6.1); цепочка или набор нарушают инвариант |
| **Stale** | sha копии ≠ ядра лаунчера; копии нет | по файлам набора: `sing-box-lxd.exe` ↔ `<CoreDir>\sing-box.exe`, `libcronet.dll` ↔ `<CoreDir>\libcronet.dll` (`wintun.dll` в набор не входит, §3 п. 3); разное присутствие или sha — Stale; лишний файл в каталоге копии — Stale (у ядра MISMATCH, лечит install) — это любое имя, кроме членов набора, сайдкара и остатков замены образа; остатки `<член набора>.old` и `.<член набора>.tmp-<hex>` (живут, пока старый exe исполняется classic'ом) — не лишние и не Stale: ядро называет их в `--service=status`, вердикт не меняет (§3 п. 3a); нет exe при целой цепочке — Stale (`CopyMissing`) |
| **NotRunning** | `launchctl print`: не загружена / `state` ≠ running | `QueryServiceStatus` без прав: `CurrentState` ≠ `SERVICE_RUNNING` — как exit 5 у ядра (§3 п. 6); `START_PENDING` виден как NotRunning до следующего Refresh |
| **ProcessStale** | паспорт `/admin/info` ≠ копии | то же (`executable` — путь exe, §3 п. 9); сравнение без учёта регистра после `Clean` |
| **OK** | иначе | иначе |
| **CoreTooOld** | ядро лаунчера < `1.14.1-lx.12` | ядро лаунчера < **`1.14.2-lx.2`** (порог — константа по платформе; `parseCoreBuild` сравнивает базу, затем `lx.N`: `1.14.1-lx.13` ниже, `1.14.2-lx.2-rc1` проходит) |

- **Ключ кэша sha и версии** на Windows — `(VolumeSerialNumber, FileIndex,
  size, mtime)` из `GetFileInformationByHandle`; `Stat_t` там нет.
- **Сайдкар** — для показа версии и списка файлов набора; sha лаунчер
  считает сам.
- **Канонические пути** — `windows.KnownFolderPath`, как у ядра (§3); путь
  службы — только из SCM, не из сайдкара.
- **Не судим** — как SPEC 136 §4: ядро лаунчера не прочиталось, демон
  недостижим или не на loopback, пустой `executable_sha256`, SCM не ответил.

## 7. Системный прокси

**Почему.** Ядро ставит прокси через WinINet (`sing/common/wininet`,
`InternetSetOptionW` + `INTERNET_OPTION_PER_CONNECTION_OPTION`) — в HKCU
**своего** процесса; под `LocalSystem` это профиль SYSTEM, у пользователя
прокси нет. macOS не меняется (ядро под root ставит прокси системно).

**Где переопределяется.** `config.json` на диске не меняется — шаблон
(`proxy_in_set_system_proxy`) и classic видят свой `true`. Подмена — в
последнем звене перед доставкой, `prepareConfigForDaemon` (станет общей),
шагом (3) под платформенным признаком «прокси ставит лаунчер» (Windows):
каждый inbound с `set_system_proxy: true` получает `false`; адрес первого
запоминается так, как его строит ядро (`common/listener/listener.go`:
пустой или неуказанный `listen` → `127.0.0.1`, порт — `listen_port`, строка
`http://<addr>:<port>`); второй и следующие — WARN (у WinINet один прокси).
По умолчанию на Windows `proxy-in` выключен и `set_system_proxy = false` —
ветка работает, только если пользователь включил их. Remote-таргет не
затрагивается (`false` по шаблону). Шаг (4) того же звена (Windows):
`state_directory` узла tailscale из DataDir → `<StateDir>\tailscale\<тег>`
(`<StateDir>` = `<ProgramData>\sing-box-lxd\state`, §3 п. 2; §13 п. 5;
каталог создаёт демон, §3 п. 10).

**Как ставится.** `internal/platform/sysproxy_windows.go` — порт
`wininet.SetSystemProxy`/`ClearSystemProxy` (~60 строк, без зависимости от
`sing`): LAN-настройки, `PROXY_TYPE_PROXY | PROXY_TYPE_DIRECT`, та же
строка сервера, что у ядра, без bypass (§13 п. 3), затем
`SETTINGS_CHANGED`, `PROXY_SETTINGS_CHANGED`, `REFRESH`. Метка владения —
`daemon_system_proxy` в `settings.json`. **«Снять своё»**: в HKCU
`ProxyEnable = 1` и `ProxyServer` = метке → `DIRECT | AUTO_DETECT` (как
`ClearSystemProxy` ядра); иначе стирается только метка.

| Событие | Действие |
|---|---|
| успешный `Apply` с адресом прокси | поставить, записать метку |
| успешный `Apply` без `set_system_proxy` | снять своё |
| кадр статус-стрима: ядро не `started` (idle, fatal, откат) | снять своё; снова `started` при адресе в последнем apply — поставить |
| `StopVPN` | после успешного `/admin/stop` — снять своё |
| выход, `daemon_stop_vpn_on_exit = true` | stop и снять своё |
| выход, VPN оставлен работать | не трогать: прокси указывает на демон, он слушает |
| краш лаунчера | ничего; при следующем старте в daemon-режиме первый кадр статуса (или порог промахов канала) сверяет: ядро не работает — снять своё |
| смена движка daemon → classic, Unpair, Uninstall | снять своё |

Прокси — настройка пользователя: без входа в систему работает только TUN.

## 8. Classic с правами администратора: только копия

После SPEC 139 лаунчер повышается только по запросу (TUN без прав →
перезапуск через `runas`). Правило 137 переносится с поправкой: гейт
включает **повышенный токен** (`platform.IsElevated`), а не TUN — ядро
наследует токен лаунчера при любом конфиге (§13 п. 1).

- **Гейт** — вердикты SPEC 137 §4 (`no core`/`missing`/`unsafe`/
  `outdated`/`ok`) над набором (§6.2) и инвариантом §6.1, закрыт по
  умолчанию. Отказ — диалог SPEC 137 §5: `--service=install` (служба есть)
  или `--service=copy`, кнопки Copy / Run as administrator / Retry / Close;
  ядро ниже 1.14.2-lx.2 — подсказка без команды.
- **Старт** — `exec.Command(<копия>, "run", "-c", "config.json")`, `Dir` =
  `<Data>\bin` (относительные пути конфига), `PrepareCommand`. Конфиг
  пользователя под правами — принятый риск SPEC 137 §8 п. 2. Поиск DLL из
  cwd закрыт ядром (§3 п. 12) — условие релиза выполнено.
- **Лог** — `C:\ProgramData\sing-box-lxd\logs\classic.log` (аналог
  `/Library/Logs/sing-box-lxd/classic.log`). Каталог `logs\` (там же
  `lxd.log` демона) создают install и copy (§3 п. 8), нет его —
  `missing`; цепочка `ProgramData\sing-box-lxd\logs` — по §6.1. Файл создаёт
  повышенный лаунчер с явным DACL: SYSTEM и Administrators — полный доступ,
  SID пользователя лаунчера — чтение. install и copy заменяют DACL на всём
  дереве `<ProgramData>\sing-box-lxd` без исключений (§3 п. 2a) и снимают
  это чтение, поэтому явный DACL файла лаунчер выставляет на каждом
  повышенном старте classic, а не только при создании; reparse point или
  чужой тип на месте файла — отказ старта с причиной; ротация > 2 МиБ — rename в
  `classic.log.old`. Без прав файл читается по полному пути (обход traverse
  — `SeChangeNotifyPrivilege` у Everyone). `CoreLogPath()` после повышенного
  старта отдаёт этот файл (окно логов, профайлер трафика). Уходит вместе
  со всем `<ProgramData>\sing-box-lxd` при uninstall с `--purge` (§3 п. 2).
- **Stop, рестарт, Kill** — только по PID своего процесса
  (`KillProcessByPID`); `taskkill /IM sing-box-lxd.exe` запрещён — так же
  зовётся служба.
- **«Sing-Box already running»** — `sing-box.exe`, как сейчас, плюс
  `sing-box-lxd.exe` в сессии пользователя (`ProcessIdToSessionId` ≠ 0);
  сессия 0 — служба, к завершению не предлагается.

## 9. UI

- **Local → ⚙ → LOCAL:** переключатель Process / Daemon появляется на
  Windows (сейчас `buildDaemonPanel` → nil, `ui/connection_local.go`);
  вкладки Status / Install / Uninstall общие с macOS.
- **Status:** плашки Unsafe (красная), Stale / ProcessStale / NotRunning
  (жёлтые), CoreTooOld (без команды) — тексты SPEC 136 §6; кнопка —
  «Run as administrator», для NotRunning — «Start the service».
- **Install:** «Install or update the service» — install с `--invite-out`,
  сопряжение само (§5.3); вставка приглашения — запасной путь; ядро ниже
  1.14.2-lx.2 — подсказка обновить ядро.
- **Uninstall:** `--keep-copy` (копию исполняет classic, §8), галка
  `--purge`; «Need a fresh invite» — runas `client add`.
- **Remove all data…** (SPEC 135 §4.3): полный uninstall кнопкой Run as
  administrator; `-purge-data` печатает ту же команду.
- Модальное предупреждение при Unsafe — раз на версию лаунчера; WARN перед
  каждым apply, если служба не OK (SPEC 136 §6).
- Debug API `/daemon/*` и манифест `capabilities` — и на Windows.
- Диалог SPEC 139 «TUN без прав» → «Install service» ведёт в этот же
  install и переключает движок на daemon после сопряжения.

## 10. Обновление ядра

Core → Download пишет `<Data>\bin\sing-box.exe` (+ `libcronet.dll`,
`DownloadCore` шаг 6.5), затем шаг 6.7:

- `notifyDaemonServiceAfterCoreUpdate`: служба есть и вердикт по файлам не
  OK → диалог «Core updated — update the daemon service» с install (Run as
  administrator / Copy); скачанное ядро ниже 1.14.2-lx.2 — WARN без диалога;
- `notifyPrivilegedCopyAfterCoreUpdate`: службы нет, копия отстаёт — WARN с
  sha; ближайший повышенный старт покажет `outdated` (SPEC 137 §7).

Работающий образ на месте не заменить: install/copy останавливает службу и
меняет файлы через `.old` и rename (§3 п. 3a); повышенный classic из копии живёт на
старом образе до рестарта, а в каталоге копии до следующего install остаются
`<член набора>.old` / `.<член набора>.tmp-<hex>` — не Stale (§6.2).

Пин `RequiredCoreVersion` (`1.14.1-lx.13`) → `1.14.2-lx.2` отдельным
коммитом при релизе ядра (как SPEC 136 §10), до него — CoreTooOld; гейт
macOS (`lx.12`) новая линия проходит.

## 11. Границы

- **Linux** — без изменений: заглушки.
- **Win7** (`windows/386`, go1.20, `go.win7.mod`) — службы нет: daemon-код
  под тегом `darwin || (windows && !386)`, панель — только classic,
  `tools/win7guard` эти файлы не видит; Remote-машины не меняются.
- **ARM64** — код общий с amd64, доступность решает ядро (`lxd --help` с
  `--state-dir`, гейт версии); `with_lxd` в `windows-arm64` ядро
  подтвердило.
- **Несколько учётных записей** — служба одна, сопряжение и прокси у
  каждого свои: имя клиента — на пользователя (`singbox-launcher-<user>`,
  §5.3), каждая учётная запись держит свою пару и не выбивает чужую;
  последний apply побеждает.
- **macOS** — имя клиента остаётся фиксированным `singbox-launcher` (текущее
  поведение; та же коллизия при нескольких учётных записях — отдельная
  задача, здесь не решается).
- **Вне рамок:** Windows как Remote-таргет, named pipe, Authenticode копии,
  установщик (SPEC 140), перезапуск с правами и автозапуск (SPEC 139).
- **Долг macOS:** `state_directory` tailscale под root остаётся в DataDir —
  root пишет в каталог пользователя (класс SPEC 137.1).

## 12. Приёмка (ручная, Windows 10 и Windows 11, x64)

Предусловия: лаунчер после SPEC 139 (без прав), ядро 1.14.2-lx.2 в `<Data>\bin`.

1. **Установка.** Local → Daemon → Install → одно окно UAC → сопряжено
   без вставки. `sc qc sing-box-lxd`: путь в кавычках на
   `C:\Program Files\sing-box-lxd\sing-box-lxd.exe`, `AUTO_START`, `Tcpip`,
   `LocalSystem`; `sc qfailure` — RESTART; `sc sdshow` — у AU нет записи;
   `icacls` копии и `C:\ProgramData\sing-box-lxd` — без (W)/(M)/(F) для
   Users/AU; `type …\sing-box-lxd.install.json` — `files[]` без `wintun.dll`,
   `Get-FileHash` совпадает с ядром лаунчера; `--service=status` → 0;
   `service_state = ok`; Start/Stop без UAC.
2. **Отказ UAC** → «prompt was cancelled», ничего не изменилось.
3. **Без входа.** Перезагрузка, 2 мин не входить: процесс службы стартовал
   раньше входа, TUN работает, лаунчер из автозапуска — без UAC.
4. **Stale.** Подменить `<Data>\bin\sing-box.exe` (затем `libcronet.dll`)
   → жёлтая плашка с двумя sha → install → OK.
5. **Unsafe.** `sc config sing-box-lxd binPath= …` на ядро в
   `%LOCALAPPDATA%` → красная плашка, модальное окно раз на версию, WARN
   перед apply → install → OK; `icacls "C:\Program Files\sing-box-lxd"
   /grant Users:(M)` → Unsafe по цепочке, `--service=status` → 2.
6. **NotRunning.** `sc stop sing-box-lxd` → «installed but not running» →
   Start the service → OK.
7. **CoreTooOld.** Ядро лаунчера 1.14.1-lx.13 → подсказка без команды, Debug
   API `core_too_old`, `install` пуст. Core → Download 1.14.2-lx.2 → диалог
   install.
8. **Прокси.** proxy-in и set_system_proxy в визарде; daemon Start →
   `reg query "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings"`
   → `ProxyEnable 1`, `ProxyServer` на `127.0.0.1:<порт>`, демону ушёл
   `false`; Stop → `ProxyEnable 0`; выход с работающим VPN → прокси
   работает; `taskkill /F` лаунчера + stop ядра через Debug API + старт
   лаунчера → прокси снят; чужой прокси лаунчер не трогает.
9. **Classic с правами.** Копии нет → повышенный Start → `missing` с
   `--service=copy` → Run as administrator → Retry → в Диспетчере задач
   `…\sing-box-lxd.exe run -c config.json`, лог в `classic.log` (у
   пользователя — чтение, в том числе после `--service=copy` и нового
   повышенного старта); подмена ядра лаунчера → `outdated`; Kill в
   «already running» службу не трогает.
10. **Uninstall** (`--keep-copy`) → `sc query` → 1060, копия на месте,
    `--service=status` → 4; **Remove all data…** → нет ни службы, ни копии.
11. **Установщик** (SPEC 140) → служба OK, лаунчер предлагает fresh invite.
12. **Win7** — только classic; **ARM64** — сценарии 1, 4, 8 на arm64.

## 13. Решения (владелец, 24.09.2026)

1. Гейт копии — по повышенному токену при любом конфиге (§8).
2. `--invite-out` и у `--service=install` — одно окно UAC; ядро подтвердило
   (§3 п. 7); `client add` — для «fresh invite» (§5.1, §5.3).
3. Bypass прокси — паритет с ядром, пусто: WinINet и так не проксирует
   loopback без `<-loopback>` (§7).
4. NotRunning — `sc.exe start sing-box-lxd`, без ядра (§5.1).
5. `state_directory` tailscale в daemon-режиме на Windows переносится в
   `<StateDir>\tailscale\<тег>` (§7): SYSTEM не пишет в каталог
   пользователя; цена — смена движка = новый логин узла; macOS — долг §11.
6. Инвариант — список разрешённых SID: SYSTEM, Administrators, у предков ещё
   TrustedInstaller (§6.1); ядро проверяет так же (§3 п. 4).
