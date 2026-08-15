# TASKS — SPEC 100 Remote API Coverage

## Этап 1 — services-слой
- [x] `services.(*RemoteRegistry).Deploy(id, config)` — цепочка ресурсы→конфиг, вынесена из `ui/machine_list_panel.go`; `ErrBuiltConfigMissing`; `DeployResult{ResourcesUploaded, ConfigSHA}`
- [x] `SyncResources` → обёртка над `syncResourcesCounted` (дедупликация с Deploy, контракт 409 in-use сохранён)
- [x] `RollbackCore`, `ActiveConfig`, `AdminDo` на реестре
- [x] `services.TransportPool` — кеш транспортов per-machine (lazy dial, idle 90s, Invalidate, CloseAll)
- [x] Transport: `GRPCConn()`, `CloseAllConnections()`, `SubscribeLogLines()`
- [x] `lxdclient.(*Client).Do` — raw admin-REST примитив
- [x] UI `deployTo` переведён на `registry.Deploy` (одна функция для UI и API)

## Этап 2 — debugapi: remote-группа
- [x] `Server`: `EnableRemote`/`EnableDaemon`, per-machine мьютексы, `capabilities()` в манифесте; роутер собирается в `Start()` (группы включаются между `New` и `Start`)
- [x] `remote_endpoints.go`: реестр (list/pair/get/patch/delete/repair/copy-from), health, core start/stop/rollback, config active/built, deploy; маппинг ошибок 404/409/422/502/504
- [x] `remote_state_endpoints.go` + рефакторинг `state_endpoints.go` на `stateAccess` (общие тела local/machine)
- [x] `remote_observe_endpoints.go`: groups/proxies/switch/delay/pool/rules/outbounds/status/connections/dns-окно/log-окно/host/interfaces/clients/labels
- [x] `remote_resources_endpoints.go`: overview/sync/get/put/delete/download

## Этап 3 — passthrough
- [x] `raw_endpoints.go`: REST-туннель (метод/путь/body|body_base64, ответ демона как данные), gRPC через protoregistry+protojson+dynamicpb (unary, server-stream окна, client/bidi → 501), `GET /grpc/methods`
- [x] Ограничение туннеля пакетом `daemon.*`

## Этап 4 — daemon-группа и wiring
- [x] `daemon_endpoints.go`: интерфейс `DaemonFacade`, status/pair/unpair/settings/engine/commands/raw
- [x] `core/debugapi_wiring_daemon_darwin.go` + `_stub.go`; `SwitchEngine` = `SwitchBackendMode` + персист `CoreBackendMode` (порядок как у радио conn.local)
- [x] `core/debugapi_wiring.go`: EnableRemote (реестр + общий пул), EnableDaemon, закрытие пула в StopDebugAPI

## Этап 5 — проверки и документация
- [x] `go build ./...`, `go vet ./...`, `go test ./...` — чисто
- [x] Тесты: capabilities/реестр/404/битый invite; raw REST + health + deploy через httptest-демон; зеркала state машины (изоляция от local); резолв gRPC-методов и discovery; контракт daemon-группы через фейковый фасад
- [x] `docs/API.md` + `docs/API.ru.md`: секции Remote machines / Local daemon / Raw passthrough
- [x] `docs/release_notes/upcoming.md` (EN+RU)
- [x] IMPLEMENTATION_REPORT.md; папка переименована в статус C
