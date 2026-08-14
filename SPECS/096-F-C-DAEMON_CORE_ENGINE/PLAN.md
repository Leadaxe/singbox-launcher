# PLAN 096 — Daemon core engine

Итоговая архитектура. Три слоя, три задачи (§ TASKS.md).

---

## Слой 1 — Абстракция движка ядра

Интерфейс `CoreBackend` (`core/backend.go`) — единственный шов между UI и ядром:
`StartVPN` / `StopVPN` / `RestartVPN` / `OnAppExit` / `Close` / `Mode`.
Package-level обёртки (`StartSingBoxProcess`, `StopSingBoxProcess`,
`KillSingBoxForRestart`) диспетчеризуют в активный backend.

| Реализация | Файл | Роль |
|---|---|---|
| `LegacyBackend` | `core/backend_legacy.go` | classic: делегирует в `ProcessService` без изменений |
| `DaemonBackend` | `core/backend_daemon_darwin.go` | daemon: gRPC + admin REST |
| stub | `core/backend_daemon_stub.go` | не-darwin: `newDaemonBackend` → ошибка |

Активный backend хранится в `AppController` (`backend`, `backendMu`); смена —
`setBackend` (закрывает предыдущий, ставит новый, дёргает
`backendModeChangeHook`). Выбор при старте — `initBackendFromSettings`
(`core_backend_mode` в settings.json); рантайм-смена — `SwitchBackendMode`
(требует остановленного VPN).

**Транспорт прокси** — `services.ProxyTransport` (`core/services/proxy_transport.go`):
`GroupProxies` / `SwitchProxy` / `Delay`. Classic — `ClashTransport` (Clash
HTTP), daemon — `daemonProxyTransport` (gRPC). `APIService` и Servers-tab ходят
через транспорт; UI выбирает его в `EffectiveProxyTransport`
(`ui/clash_remote.go`).

## Слой 2 — Daemon-клиент и наблюдаемость

**Клиент управляющего канала** — `internal/lxdclient/` (darwin):

| Файл | Роль |
|---|---|
| `identity.go` | клиентская ключевая пара (ECDSA P-256), отпечаток SHA-256(DER) |
| `invite.go` | разбор приглашения `адрес#отпечаток#код`, loopback-гейт |
| `client.go` | mTLS-дайл (пин сервера), admin REST (apply/start/stop/status/enroll), gRPC-дайл с Bearer |

**gRPC-стабы** — `internal/daemonpb/` (darwin), вендорятся из форка скриптом
`scripts/sync_daemonpb.sh`. Зависимости `google.golang.org/grpc` + `protobuf`
— только под darwin build-tags.

**Наблюдаемость в `DaemonBackend`**: supervisor-горутины держат стримы
`SubscribeServiceStatus` (→ `RunningState`), `SubscribeLog` (кольцевой буфер →
вьюер логов), `SubscribeConnections` (→ трафик-профайлер). `daemonProxyTransport`
реализует proxy-операции через `GetGroups`/`SelectOutbound`/`URLTestOutbound`;
пул — `GetPool` (`ui/pool_window.go`).

**Трафик** (`internal/traffic`): `ConnPoller` получает pluggable источник
снапшотов (`SnapshotFunc`) — classic оставляет Clash HTTP, daemon подставляет
gRPC-источник (`core/backend_daemon_traffic_darwin.go`: `connTracker` +
`protoConnToClash`). Переустановка источника при смене режима — через
`backendModeChangeHook` (`ui/traffic_bootstrap.go`).

**Подготовка конфига** — `prepareConfigForDaemon`
(`core/daemon_manager_darwin.go`): абсолютизация `cache_file.path`, удаление
`experimental.clash_api`. Применение — `applyCurrentConfig`: rebuild → prepare →
`admin.Apply` (Restart форсирует полную пересборку).

## Слой 3 — Служба, сопряжение, UI настроек

**Менеджер службы** — `core/daemon_manager_darwin.go`:
`InstallDaemonService` / `UninstallDaemonService(purge)` / `PairWithLocalService`
/ `PairDaemonWithInvite` / `DaemonStatusSnapshot`. Привилегированный вызов —
`platform.RunPrivilegedProgramAndWait` (`internal/platform/privileged_darwin.go`)
через self-хелпер `--priv-exec` (`internal/platform/priv_exec_darwin.go`),
дающий процессу настоящий root.

**Детект чужого sing-box** — `core/process_detect_darwin.go`: `pgrep -f` по
паттерну `run`-процесса, чтобы установленный демон не считался «чужим» ядром.

**UI настроек** — `ui/settings_daemon_darwin.go` (secтion; stub для не-darwin):
переключатель режима, установка/удаление службы (+полное), терминальный путь,
поле сопряжения, статус, «останавливать VPN при выходе». Хранение —
`internal/locale/settings.go` (`core_backend_mode`, `daemon_address`,
`daemon_server_fingerprint`, `daemon_secret`, `daemon_stop_vpn_on_exit`).
