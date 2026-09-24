# SPEC 141 · План реализации

Норма — SPEC.md. Ветка — `spec-141-daemon-windows` (worktree от `develop`),
не пушится до команды владельца. Этапы 2–5 от SPEC 139 не зависят; этап 6
(classic с правами) — после SPEC 139 (`platform.IsElevated`, перезапуск
через `runas`).

## Архитектурные решения

| Решение | Почему |
|---|---|
| Этап 2 — чистое разделение файлов под прежним тегом `darwin`; тег daemon-платформ `darwin \|\| (windows && !386)` включается только на этапе 4 | рефакторинг без изменения поведения проверяется отдельно от Windows-кода; решение A: «только разделить» |
| Классификатор остаётся чистой функцией от раскладки (`daemonServiceLayout`), платформенное — за интерфейсом «определение службы» (plist ↔ SCM), «проверка звена» (uid ↔ владелец/DACL), «ключ файла» (`Stat_t` ↔ `GetFileInformationByHandle`) | общие шаги (sha набора, процесс, гейт) не дублируются; тест раскладки во временном каталоге работает на обеих ОС |
| Сравнение копии — по **набору** (список имён по платформе: darwin — один файл, windows — exe + `libcronet.dll`; `wintun.dll` ядро несёт `go:embed`) | одна функция `compareDaemonServiceFiles` для обеих ОС и для гейта classic |
| Команда — структура `{Binary, Args}` с платформенным рендером строки | darwin исполняет строку в Terminal, windows — argv через `ShellExecuteExW`; квотинг — одно место на платформу |
| `runas`-исполнитель — `internal/platform/runas_windows.go`, общий с SPEC 139 | перезапуск лаунчера с правами (139) и команды службы (141) — один вызов `ShellExecuteExW`; кто первый — тот создаёт |
| Системный прокси и `state_directory` tailscale — шаги (3) и (4) `prepareConfigForDaemon` + хук в `DaemonBackend` (darwin — no-op) | конфиг на диске не меняется; подмена в последнем звене перед доставкой, как `cache_file` и `clash_api` |
| WinINet — свой порт на ~60 строк, не импорт `sing/common/wininet` | лаунчер не зависит от `sing`; поведение совпадает с ядром байт в байт (та же строка сервера — «своё» узнаётся) |
| Порог версии — константа по платформе (`minCoreForRootOwnedService`: darwin `1.14.1-lx.12`, windows `1.14.2-lx.2`) | разбор и сравнение (`parseCoreBuild`) общие |

## Файлы

| Файл | Что |
|---|---|
| `core/backend_daemon.go` (из `_darwin.go`) | `DaemonBackend` целиком; вызовы хука прокси после apply/stop/кадра статуса/выхода |
| `core/backend_daemon_dns.go`, `…_traffic.go`, `…_tailscale.go` (из `_darwin.go`) | без изменений, тег |
| `core/chain_probe.go`, `core/debugapi_wiring_daemon.go` | тег; `Kickstart` — через платформенную функцию |
| `core/daemon_manager.go` (новый) + `_darwin.go` + `_windows.go` (новый) | общее: снимок, сопряжение, `prepareConfigForDaemon` (+ шаги 3–4), сборка команд; darwin: plist/launchd/Terminal/`shellQuote`; windows: SCM-имя, рендер PowerShell, `sc.exe start`, install/`client add` с `--invite-out` и автосопряжение |
| `core/daemon_service_state.go` (новый) + `_darwin.go` + `_windows.go` (новый) | общее: состояния, гейт, сверка набора, процесс, кэши; darwin: `/Library`, `Stat_t`, `launchctl`, legacy; windows: `Program Files`, SCM, DACL, ключ файла |
| `core/classic_privileged.go` (новый) + `_darwin.go` + `_windows.go` (новый) | общее: вердикты, `checkPrivilegedCoreCopy` над набором, выбор команды, диалог; windows: открытие `classic.log` с DACL и ротацией |
| `core/daemon_sysproxy_darwin.go`, `core/daemon_sysproxy_windows.go` (новые) | хук «прокси ставит лаунчер»: no-op / поставить, снять своё, сверка на старте |
| `core/purge_daemon.go` (из `purge_darwin.go`), `core/purge_other.go` | тег |
| `core/process_detect_windows.go` (новый), `core/process_service.go` | копия в сессии пользователя vs служба в сессии 0; повышенный старт через гейт; Kill только по PID; `CoreLogPath` на Windows |
| заглушки `core/backend_daemon_stub.go`, `core/debugapi_wiring_daemon_stub.go`, `core/classic_privileged_other.go`, `core/process_detect_stub.go`, `ui/connection_local_daemon_stub.go` | тег `!darwin && (!windows \|\| 386)` |
| `internal/platform/runas_windows.go` (новый или из SPEC 139) | `ShellExecuteExW` runas, ожидание, код выхода, `ERROR_CANCELLED` |
| `internal/platform/scm_windows.go` (новый) | `QueryServiceConfig`, `QueryServiceStatus`, `QueryServiceObjectSecurity` без прав |
| `internal/platform/winacl_windows.go` (новый) | владелец/DACL звена по списку разрешённых SID (как ядро), reparse, `DRIVE_FIXED` + NTFS, ключ файла, `KnownFolderPath` |
| `internal/platform/sysproxy_windows.go` (новый) | WinINet set/clear, чтение `ProxyEnable`/`ProxyServer` из HKCU |
| `internal/platform/privileged_windows.go` (новый), `privileged_stub.go` | имя копии `sing-box-lxd.exe`, `PrivilegedCoreLogPath`; stub → `!darwin && !windows` |
| `internal/locale/settings.go` | `daemon_system_proxy`; комментарии «только macOS» |
| `ui/connection_local_daemon.go` (из `_darwin.go`), `ui/command_row_windows.go` (новый), `ui/connection_local.go`, `ui/settings_purge.go` | панель на Windows, кнопка Run as administrator, шаг Pair, Remove all data |
| `core/core_downloader.go` | комментарий шага 6.7 |
| `core/debugapi/server.go`, `core/debugapi/daemon_endpoints.go` | комментарии «darwin» → daemon-платформы |
| `bin/locale/ru.json` | новые ключи |
| `core/daemon_sysproxy_test.go` (новый) | `TestPrepareConfigForDaemonSystemProxy` — единственный новый тест волны: данные, уходящие демону (`set_system_proxy`, `state_directory` tailscale) |
| `docs/DAEMON_AND_REMOTE*.md`, `docs/API*.md`, `docs/ARCHITECTURE*.md`, `docs/release_notes/upcoming.md`, `RELEASE_NOTES.md`, `SPECS/README.md` | документация |

## Этапы (коммиты)

| # | Что | Проверка |
|---|---|---|
| 1 | SPEC/PLAN/TASKS | — |
| 2 | Разделение файлов под тегом `darwin`, без изменения поведения; тесты делятся тем же правилом | `go build ./...` (macOS) |
| 3 | Платформенный слой Windows: `runas`, SCM, DACL/владелец/NTFS, WinINet, ключ файла | `GOOS=windows GOARCH=amd64 go build ./...` |
| 4 | Тег daemon-платформ; классификатор и менеджер Windows (команды, install с `--invite-out`, автосопряжение, `sc.exe start` через runas), гейт `1.14.2-lx.2`, Debug API | `go build ./...`; `GOOS=windows GOARCH=amd64`, `GOARCH=arm64`, `GOARCH=386` (заглушки) `go build ./...` |
| 5 | Системный прокси: шаг 3 `prepareConfigForDaemon`, хук, метка, сверка на старте; шаг 4 — `state_directory` tailscale; `TestPrepareConfigForDaemonSystemProxy` | `go test ./core/ -run TestPrepareConfigForDaemonSystemProxy -count=1` |
| 6 | Classic с правами (после SPEC 139): гейт по токену, лог, детект, Kill по PID | `GOOS=windows go build ./...` |
| 7 | UI: панель, Pair, Remove all data, диалог после обновления ядра; locale | `go build ./...`, `l10n_check --strict`, показать владельцу |
| 8 | Доки и заметки | — |
| 9 | CI после волны | `gh workflow run ci.yml --ref develop -f run_mode=tests` (Win7-сборка — там же) |
| — | Бамп `RequiredCoreVersion` → `1.14.2-lx.2` | отдельный коммит при релизе ядра |
