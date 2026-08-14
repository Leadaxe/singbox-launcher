# IMPLEMENTATION REPORT 096 — Daemon core engine (lxd)

**Дата:** 2026-08-09
**Статус:** Complete. Проверено на живой системе (macOS, локально собранное ядро форка).

---

## 1. Движок

`CoreBackend` (`core/backend.go`) — единый шов: UI, трей, шорткаты и debug-API
не обращаются к процесс-менеджеру или Clash API напрямую, а ходят через
активный движок. Package-level обёртки диспетчеризуют в него.

| Движок | Файл |
|---|---|
| `LegacyBackend` (classic spawn) | `core/backend_legacy.go` |
| `DaemonBackend` (lxd, darwin) | `core/backend_daemon_darwin.go` |
| stub (не-darwin) | `core/backend_daemon_stub.go` |

Активный движок — в `AppController` (`backend`/`backendMu`); выбор при старте
из `settings.json` (`initBackendFromSettings`), рантайм-смена — `SwitchBackendMode`
(только при остановленном VPN). Переключатель — в секции настроек.

Операции с прокси — через `services.ProxyTransport`
(`core/services/proxy_transport.go`): classic → `ClashTransport` (Clash HTTP),
daemon → `daemonProxyTransport` (gRPC). UI выбирает транспорт в
`EffectiveProxyTransport` (`ui/clash_remote.go`).

## 2. Daemon-клиент и наблюдаемость

`internal/lxdclient/` (darwin): клиентская пара (`identity.go`), разбор
приглашения `адрес#отпечаток#код` с loopback-гейтом (`invite.go`), mTLS-клиент
с пином сервера + admin REST + gRPC-дайл (`client.go`).

gRPC-стабы протокола `daemon.StartedService` — `internal/daemonpb/`, вендорятся
из форка `scripts/sync_daemonpb.sh`. Зависимости `grpc` + `protobuf` — под
darwin build-tags; `go.win7.mod` их не содержит, `internal/lxdclient` и
`internal/daemonpb` вне darwin — пустые пакеты.

`DaemonBackend` держит supervisor-стримы: `SubscribeServiceStatus` → `RunningState`,
`SubscribeLog` → кольцевой буфер (вьюер логов, `ui/log_viewer_window.go`),
`SubscribeConnections` → трафик. Proxy-операции — `GetGroups`/`SelectOutbound`/
`URLTestOutbound`; пул — `GetPool` (`ui/pool_window.go`).

Трафик-профайлер (`internal/traffic`) получил pluggable источник
(`SnapshotFunc` в `ConnPoller`): classic — Clash HTTP, daemon — gRPC-источник
(`core/backend_daemon_traffic_darwin.go`: `connTracker` + `protoConnToClash`),
переустанавливаемый при смене режима через `backendModeChangeHook`.

Конфиг для демона готовит `prepareConfigForDaemon`
(`core/daemon_manager_darwin.go`): `cache_file.path` абсолютизируется в каталог
демона (демон работает с cwd=`/`), `experimental.clash_api` удаляется — всё
управление и наблюдаемость идут по gRPC. Применение — `applyCurrentConfig`
(rebuild → prepare → `admin.Apply`).

## 3. Служба, сопряжение, UI

Менеджер службы (`core/daemon_manager_darwin.go`): установка/удаление системного
LaunchDaemon (в т.ч. полное с данными), авто-сопряжение с локальной службой и
ручное по приглашению, снимок статуса. Привилегированный вызов даёт процессу
настоящий root через self-хелпер `--priv-exec` (`internal/platform/priv_exec_darwin.go`,
`privileged_darwin.go`): `AuthorizationExecuteWithPrivileges` даёт лишь
effective uid=0, хелпер поднимает real uid через `setuid(0)` перед запуском цели.

Детект уже запущенного sing-box (`core/process_detect_darwin.go`) сопоставляет
по командной строке `run`-процесса, поэтому установленный демон не считается
«чужим» ядром.

Секция настроек — `ui/settings_daemon_darwin.go` (stub для не-darwin):
переключатель режима, установка/удаление службы, терминальный путь установки,
поле сопряжения, статус, «останавливать VPN при выходе». Настройки —
`internal/locale/settings.go` (`core_backend_mode`, `daemon_address`,
`daemon_server_fingerprint`, `daemon_secret`, `daemon_stop_vpn_on_exit`);
локали EN/RU. После обновления ядра установленная служба перезапускается
(`core/core_downloader.go`).

## 4. Проверки

- `go build ./...`, `go test ./...`, `go vet ./...` — зелёные.
- Win7-сборка компилируется без gRPC/daemon-кода (пакеты пусты вне darwin,
  `go.win7.mod` без grpc).
- E2E против настоящего демона (`internal/lxdclient/e2e_test.go`, флаг
  `LXD_E2E_BIN`): сопряжение mTLS, apply/status, gRPC-стримы, пул, переживание
  смены конфига.
- Сквозной прогон на живой системе: установка службы, сопряжение, apply,
  ядро в демоне (status started), ноды/пул/логи/трафик по gRPC, рабочий
  classic-VPN не затронут.

## 5. Ограничения

- Только macOS.
- Требует ядра с сабкомандой `lxd` (`with_lx_command`, форк `1.14.0-lx.23+`).
  Для конечных пользователей нужен бамп `constants.RequiredCoreVersion` до
  такого релиза; до этого — локально собранное ядро.
