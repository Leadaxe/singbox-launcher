# IMPLEMENTATION_REPORT — SPEC 100 Remote API Coverage

Дата: 2026-08-15. Статус: **реализовано** (границы v1 — как в SPEC §7).

## Что сделано

Debug API покрывает весь remote-функционал SPEC 096–099 плюс raw-passthrough:

- **`/remote/machines*`** — реестр (list/pair/repair/update/remove/copy-profile),
  health, start/stop/rollback ядра, config active/built, deploy, зеркала
  `/state/*` на профиль машины, наблюдаемость (proxies/switch/delay/pool/rules/
  outbounds/status/connections/dns/logs/host/clients/labels), ресурс-стор.
- **`/daemon/*`** (darwin) — status/pair/unpair/settings/engine/commands.
- **Passthrough** — `…/raw/rest` и `…/raw/grpc` для машин и локального демона,
  `GET /grpc/methods` discovery. gRPC-резолв по имени через protoregistry —
  ручной таблицы методов нет.
- **Манифест** — `capabilities: {remote, daemon, raw_grpc}`.

## Ключевые файлы

| Файл | Роль |
|---|---|
| `core/debugapi/remote_endpoints.go` | Группа remote: реестр/ядро/деплой, error-маппинг (404/409/422/502/504), `RemoteAPI` |
| `core/debugapi/remote_state_endpoints.go` | Зеркала state; `machineStateAccess` |
| `core/debugapi/remote_observe_endpoints.go` | Наблюдаемость; окна стримов `?duration=&max=` |
| `core/debugapi/remote_resources_endpoints.go` | Ресурс-стор |
| `core/debugapi/raw_endpoints.go` | REST/gRPC passthrough, `/grpc/methods` |
| `core/debugapi/daemon_endpoints.go` | `DaemonFacade` + группа /daemon/* |
| `core/debugapi/state_endpoints.go` | Рефакторинг на `stateAccess` (общие тела local/machine) |
| `core/debugapi/server.go` | `EnableRemote/EnableDaemon`, `capabilities()`, per-machine мьютексы, роутер в `Start()` |
| `core/debugapi_wiring.go` / `…_daemon_darwin.go` / `…_daemon_stub.go` | Подключение групп к AppController |
| `core/services/lxd_remote_deploy.go` | `Deploy` (общая цепочка UI+API), `RollbackCore`, `ActiveConfig`, `AdminDo` |
| `core/services/lxd_transport_pool.go` | Кеш gRPC-транспортов per-machine |
| `core/services/lxd_remote_transport_extra.go` | `GRPCConn`, `CloseAllConnections`, `SubscribeLogLines` |
| `internal/lxdclient/client.go` | `Do` — raw REST примитив |
| `ui/machine_list_panel.go` | `deployTo` → `registry.Deploy` |

## Решения по ходу (сверх SPEC)

1. **Роутер собирается в `Start()`, а не в `New()`** — иначе группы,
   включённые между `New` и `Start`, не попадали в роутинг (найдено тестом).
2. **Kind-гейт raw gRPC до dial** — на client/bidi (501) соединение не
   открывается вовсе.
3. **Битое приглашение = 400** (валидация `ParseInvite` до enroll), а не 5xx:
   вина запроса, и одноразовый код не сжигается.
4. **PATCH state при отсутствии файла = 404** (`stateErrStatus`), в т.ч. для
   локального scope — согласовано с GET-контрактом (`fresh install`).
5. **REST-тело passthrough без поля `timeout`** — у `lxdclient` фиксированный
   клиентский таймаут 30s; обещать per-request таймаут, который не
   применяется, хуже, чем не принимать его.
6. **Ответ raw REST всегда наш 200** — статус демона в поле `status`: иначе
   не отличить «нашу» 404 (unknown machine) от 404 демона.

## Проверки

- `go build ./...`, `go vet ./...` — чисто; `go test ./...` — все пакеты ok.
- Новые тесты: `remote_endpoints_test.go` (capabilities, реестр, 404, битый
  invite, raw REST/health/deploy через httptest-демон, зеркала state и их
  изоляция от local), `raw_endpoints_test.go` (резолв методов, kinds,
  discovery, валидация без сети), `daemon_endpoints_test.go` (контракт группы
  через фейковый фасад: engine 400/409, raw REST).

## Дополнение: UI-override (§3.8, по запросу после v1-карты)

`GET /remote/ui`, `POST /remote/machines/{id}/ui/connect`,
`POST /remote/ui/disconnect` — управление тем, на какую машину смотрит
вкладка Servers. Через хуки `UIService.LxdOverride*Func`
(`ui/lxd_remote_override.go: RegisterOverrideAPIHooks`, зовётся из `NewApp`)
— API и кнопки вкладки Remote ходят в один и тот же
`SetLxdRemoteOverride`/`ClearLxdRemoteOverride`. Connect гейтится health'ом
(502 без переключения), headless → 503. Тест:
`TestRemoteUIOverrideEndpoints`.

## Не вошло (по SPEC §7)

SSE-стримы; `POST …/action/rebuild-config` (ждёт выноса сборки remote-конфига
из презентера); Clash remote-override (SPEC 064); `ImportPairedDaemon`;
исполнение привилегированных команд (никогда).
