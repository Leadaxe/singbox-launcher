# TASKS 096 — Daemon core engine

Три логические задачи по слоям PLAN.md.

---

## T1 — Абстракция движка ядра

- [x] Интерфейс `CoreBackend` + диспетчеризация package-level обёрток
      (`core/backend.go`).
- [x] `LegacyBackend` — classic без изменения поведения
      (`core/backend_legacy.go`).
- [x] Хранение активного backend в `AppController`, `setBackend` /
      `SwitchBackendMode` / `initBackendFromSettings`,
      `backendModeChangeHook`.
- [x] `ProxyTransport`-шов: `ClashTransport` (classic) + выбор в
      `EffectiveProxyTransport`; `APIService`/Servers-tab через транспорт.
- [x] Переключатель режима + хранение настроек
      (`internal/locale/settings.go`).
- [x] Тесты: режимы backend, транспорт-override.

## T2 — Daemon-клиент, наблюдаемость, трафик по gRPC

- [x] `internal/lxdclient/` — identity, invite, mTLS-клиент, admin REST,
      gRPC-дайл.
- [x] Вендоринг gRPC-стабов `internal/daemonpb/` + `scripts/sync_daemonpb.sh`;
      зависимости под darwin build-tags (win7 не затронут).
- [x] `DaemonBackend` — supervisor-стримы (status/log/connections),
      `daemonProxyTransport` (groups/select/urltest), пул (`ui/pool_window.go`).
- [x] Логи ядра из `SubscribeLog` (`ui/log_viewer_window.go`).
- [x] Трафик по gRPC: pluggable `SnapshotFunc` в `ConnPoller`, gRPC-источник
      (`connTracker` + `protoConnToClash`), переустановка при смене режима.
- [x] `prepareConfigForDaemon` (абсолютизация cache_file, удаление clash_api)
      + `applyCurrentConfig`.
- [x] Тесты: разбор приглашения, identity, admin REST-коды, конвертер трафика,
      подготовка конфига; e2e против настоящего демона (`LXD_E2E_BIN`).

## T3 — Служба, сопряжение, UI настроек

- [x] Менеджер службы: install/uninstall(+purge), сопряжение (авто/по
      приглашению), статус (`core/daemon_manager_darwin.go`).
- [x] Привилегированный root через self-хелпер `--priv-exec`
      (`internal/platform/priv_exec_darwin.go`, `privileged_darwin.go`).
- [x] Детект чужого sing-box исключает демон
      (`core/process_detect_darwin.go`).
- [x] Секция настроек daemon-режима (`ui/settings_daemon_darwin.go` + stub):
      режим, установка/удаление, терминальный путь, сопряжение, статус,
      поведение при выходе; локали EN/RU.
- [x] Перезапуск службы после обновления ядра
      (`core/core_downloader.go`).

---

**Проверки:** `go build ./...`, `go test ./...`, `go vet ./...` — зелёные;
win7-сборка без gRPC/daemon-кода; e2e против локального ядра пройдён.
