# SPEC 141 · Отчёт этапа «разделение файлов» (§4)

Ветка `spec-141-daemon-split` от `develop` 879f276e. Чистый рефакторинг: логика
перенесена построчно (сверка «ни одна строка не потеряна» по каждому
разделённому файлу), имена экспортов сохранены, поведение на macOS не меняется.
На Windows x64/arm64 общий код собирается, но движок закрыт гейтом
`daemonEngineAvailable`, поэтому в рантайме Windows, как и раньше, — только
classic. Linux и Win7 — прежние заглушки с новым тегом.

## Теги

- daemon-платформы: `//go:build darwin || (windows && !386)`;
- заглушки Linux и Win7: `//go:build !darwin && (!windows || 386)`;
- платформенный слой: `_darwin.go` (`//go:build darwin`) и `_windows.go`
  (`//go:build windows && !386`).

Отступление от PLAN (этап 2 «под прежним тегом `darwin`»): по указанию
владельца тег daemon-платформ включён сразу, вместе с заглушками Windows.
Промежуточные коммиты ветки делят файлы под тегом `darwin`, последний меняет
теги.

## Файл → тег

| Файл | Тег | Что внутри |
|---|---|---|
| `core/backend_daemon.go` (было `_darwin.go`) | daemon-платформы | `DaemonBackend`, apply/stop/exit, стримы статуса и логов, `daemonProxyTransport`, `diagnoseReachError`; гейт `daemonEngineAvailable` в `newDaemonBackend` |
| `core/backend_daemon_dns.go`, `…_traffic.go`, `…_tailscale.go` (были `_darwin.go`) | daemon-платформы | без изменений |
| `core/chain_probe.go` | daemon-платформы | без изменений |
| `core/debugapi_wiring_daemon.go` (было `_darwin.go`) | daemon-платформы | фасад `/daemon/*`; `nil` при закрытом движке |
| `core/purge_daemon.go` (было `purge_darwin.go`) | daemon-платформы | подсказка удаления службы |
| `core/daemon_manager.go` (новый) | daemon-платформы | `prepareConfigForDaemon`, `DaemonUIStatus`, `DaemonStatusSnapshot`, `daemonServiceCheck`, `CoreSupportsLxd`, Pair/Unpair/SetAddress/SetSecret, `followDaemonPlainChannel`, `notifyDaemonServiceAfterCoreUpdate`, сборка команд install/uninstall/repair, `DaemonServiceCoreHint`, `DaemonUnsafeServiceNotice` |
| `core/daemon_manager_darwin.go` | darwin | метка и plist launchd, `readPlistProgramPath`, bootstrap/kickstart, `daemonServiceCommand` (рендер `sudo …`), show-secret, `OpenTerminalWithCommand`, `appleScriptString`, `shellQuote`, `daemonFallbackRuntimeDir`, `daemonEngineAvailable` = nil |
| `core/daemon_manager_windows.go` (новый) | windows && !386 | заглушки: `errDaemonWindowsPending`, `daemonEngineAvailable` (отказ), `daemonServiceCommand`, `daemonBootstrapCommand`, `daemonFallbackRuntimeDir`, show-secret; `DaemonKickstartCommand` = "" |
| `core/daemon_service_state.go` (было `_darwin.go`) | daemon-платформы | состояния, `NeedsInstall`/`NeedsBootstrap`/`CopyUsable`, гейт и разбор версий ядра, обход цепочки `checkRootOwnedChain`, сверка sha, `compareDaemonServiceProcess`, сайдкар-версия, кэши sha и версии (без ключа), `shortSHA` |
| `core/daemon_service_state_darwin.go` (новый) | darwin | раскладка `/Library/…`, legacy lx.11, `inspectDaemonServiceDefinition` (plist), звено цепочки `checkRootOwnedEntry` (`Stat_t`), ключ кэша `fileHashKey`/`statHashKey` (dev, inode, size, mtime), `launchctl print`, `compareDaemonServiceRunning` |
| `core/daemon_service_state_windows.go` (новый) | windows && !386 | заглушки: раскладка пуста, служба «не установлена», звено и ключ файла — ошибка (`fileHashKey` — пустой тип), состояние SCM — пусто |
| `core/classic_privileged.go` (было `_darwin.go`) | daemon-платформы | вердикты гейта, `checkPrivilegedCoreCopy`, выбор команды copy/install, `privilegedCoreCopyGate`, `notifyPrivilegedCopyAfterCoreUpdate` |
| `core/classic_privileged_darwin.go` (новый) | darwin | тексты «root»/Terminal/sudo и `showPrivilegedCopyDialog` |
| `core/classic_privileged_windows.go` (новый) | windows && !386 | заглушка `showPrivilegedCopyDialog` (этап 6) |
| `core/backend_daemon_stub.go`, `core/debugapi_wiring_daemon_stub.go`, `core/classic_privileged_other.go`, `core/purge_other.go` | Linux и Win7 | прежние заглушки, новый тег |
| `core/backend_daemon_dns_test.go`, `…_traffic_test.go`, `…_paths_test.go` | daemon-платформы | `TestPrepareConfigForDaemon` — фиктивные абсолютные пути, годные и на Windows |
| `core/daemon_service_state_test.go` | daemon-платформы | `TestServiceCoreVersionGate`, `TestDaemonServiceProcessVerdict` (вынесен из классификатора, пути фиктивные) |
| `core/daemon_service_state_darwin_test.go` (было `…_test.go`) | darwin | `TestDaemonServiceClassifier` (файлы, uid, launchd), `TestDaemonServiceCommandQuoting`, `TestDaemonServiceCoreTooOld` |
| `core/classic_privileged_darwin_test.go` (было `…_test.go`) | darwin | `TestPrivilegedCoreCopyGate` |

## Что осталось в darwin и почему

- **`core/process_detect_*`** (`!darwin`) — детект процесса на Windows —
  этап 6 (§8), в этом этапе не нужен.
- **`ui/connection_local_daemon_darwin.go`, `ui/command_row_darwin.go`**
  (`!darwin` — заглушка) — панель целиком завязана на Terminal/sudo и
  launchd; общий код с подписями Windows и `runas` — этап 7 (UI, новые ключи
  locale). Перенос сейчас показал бы на Windows нерабочую панель.
- **`internal/platform/privileged_*`** — имя копии и `PrivilegedCoreLogPath`
  для Windows — этап 3 (TASKS).
- **Тексты с Terminal/sudo в общем коде** (`daemonCoreUpdatedBodyText`,
  `daemonServiceCoreTooOldText`, подсказки `diagnoseReachError`) — остались
  рядом со своими общими функциями: платформенные формулировки и перевод —
  этап 4/7. На Windows эти ветки сейчас недостижимы.
- **Не сделано из TASKS этапа 2** (требуют изменения логики, отложено к
  этапу 4): команда структурой `{Binary, Args}` — пока платформенный рендер
  `daemonServiceCommand(binary, args...)`; сверка sha «по набору» — пока один
  файл; `minCoreForRootOwnedService` — одна константа.

## Проверки

- macOS: `go build ./...`, `go vet ./core`; тесты по имени —
  `TestDaemonService*`, `TestServiceCoreVersionGate`,
  `TestPrivilegedCoreCopyGate`, `TestPrepareConfigForDaemon`,
  `TestIsUnimplemented`, `TestConsumeDNSStream*`, `TestProtoConnToClash`,
  `TestConnTrackerLifecycle` — зелёные.
- `GOOS=windows|linux CGO_ENABLED=0 go vet` пакетов `./core/... ./internal/...`,
  которые грузятся без cgo, — чисто. Пакеты `core` и `core/uiservice` без
  cgo не грузятся и до этапа (fyne/gl), поэтому `core` проверен сверкой типов
  с тестами под windows/amd64, windows/arm64, windows/386 и linux/amd64 — без
  ошибок; полная сборка — в CI.
- `go run ./tools/win7guard` — чисто; в наборе windows/386 из daemon-файлов
  только заглушки.
