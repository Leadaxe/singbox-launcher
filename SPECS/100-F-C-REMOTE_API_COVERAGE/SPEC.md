# SPEC 100 — Remote API Coverage: карта новых endpoints Debug API

Статус: **C** (реализовано, см. IMPLEMENTATION_REPORT.md). Дата: 2026-08-15.

## 1. Проблема

SPEC 096–099 дали лаунчеру полноценный remote-функционал: реестр удалённых машин,
сопряжение по mTLS, per-machine профили, Deploy с ресурсами, наблюдаемость
(gRPC-стримы, телеметрия хоста), управление локальным демоном. Всё это доступно
**только из UI** — Debug API (SPEC 038/050/078) заканчивается на локальном
classic-ядре. Автоматизация (curl-скрипты, MCP-агенты, CI) не видит ни одной
remote-операции.

Цель: **весь remote-функционал доступен через Debug API**, плюс два
универсальных passthrough-endpoint'а — произвольный REST- и произвольный
gRPC-вызов к любому сопряжённому демону (удалённому или локальному).

## 2. Принципы

1. **Одна поверхность.** Расширяем существующий `core/debugapi` (127.0.0.1,
   bearer, `debugapi/v1`), не второй сервер. Каждый новый endpoint добавляется в
   `Server.endpoints()` — самоописание в `GET /` и `GET /help` получается
   бесплатно (SPEC 078).
2. **Stateless-адресация машин.** Рабочие вызовы API не зависят от
   UI-концепции «активной машины» (remote-override): каждый адресует машину
   явно — `/remote/machines/{id}/…`. Go 1.25 → `http.ServeMux` с
   `{id}`-wildcards. Сам remote-override при этом управляем через API как
   отдельный, явно именованный объект (§3.8): ручки `/remote/ui*` меняют то,
   на что смотрит вкладка Servers, и ни на что больше не влияют.
3. **Passthrough — туннель, не прокси.** Raw-запросы уходят **только** на
   управляющий канал конкретной сопряжённой машины (её mTLS-ключи и пин из
   реестра). Никаких запросов на произвольные адреса.
4. **«Sudo только в вашем терминале»** сохраняется: API отдаёт готовые строки
   привилегированных команд (`/daemon/commands`), но никогда их не исполняет.
5. **Секреты не маскируются** — API loopback-only, локальная машина =
   trust-boundary (как в `/state/full` и `/debug/snapshot`).
6. **Блокирующие сетевые вызовы** — у каждого remote-handler'а server-side
   timeout (default 10s, у deploy 60s); недоступная машина = `502`, таймаут =
   `504`, а не повисший запрос.

## 3. Карта endpoints

### 3.1 Реестр машин — `/remote/machines`

Обёртка над `services.RemoteRegistry` (core/services/lxd_remote_registry.go).

| Метод | Путь | Тело / параметры | Реализация | Заметки |
|---|---|---|---|---|
| GET | `/remote/machines` | — | `Registry.List()` | Список `RemoteDaemon` (id, name, addr, fingerprint, goos/goarch, state_dir, added_at) |
| POST | `/remote/machines` | `{invite, name, addr?, secret?}` | `Registry.PairWithAddr()` | Сопряжение по приглашению `адрес#отпечаток#код`. Код одноразовый: неудача сети = код сгорел, `502` с этим предупреждением |
| GET | `/remote/machines/{id}` | — | `Registry.Get()` | `404` если id неизвестен |
| PATCH | `/remote/machines/{id}` | `{name?, addr?, goos?, goarch?}` | `Registry.Update()` + `SetPlatform()` | Ключи и пин не трогаются |
| DELETE | `/remote/machines/{id}` | — | `Registry.Remove()` | Сносит ключи и всё имущество машины. Ответ несёт `warning`: доступ на стороне демона НЕ отозван (`sing-box lxd client remove` — там) |
| POST | `/remote/machines/{id}/repair` | `{invite, addr?, secret?}` | `Registry.RePair()` | Пере-сопряжение с перевыпуском клиентской пары; профиль машины сохраняется |
| POST | `/remote/machines/{id}/profile/copy-from` | `{source_id, overwrite?}` | `Registry.CopyProfileFrom()` | Копия настроек (state) с другой машины с ретаргетом платформы. Целевой state существует и `overwrite!=true` → `409` |

### 3.2 Здоровье, ядро, конфиг машины

| Метод | Путь | Тело / параметры | Реализация | Заметки |
|---|---|---|---|---|
| GET | `/remote/machines/{id}/health` | — | `Registry.Health()` | `{reachable, core_status, last_error, version, state_dir, active_sha, last_good_sha, interrupted_apply}` — «доехал ли деплой» проверяется сравнением SHA |
| POST | `/remote/machines/{id}/core/start` | — | `Registry.StartCore()` | admin REST `POST /admin/start` |
| POST | `/remote/machines/{id}/core/stop` | — | `Registry.StopCore()` | Рвёт VPN у всех клиентов машины — UI спрашивает подтверждение, API исполняет молча (вызывающий скрипт сам отвечает) |
| POST | `/remote/machines/{id}/core/rollback` | — | новая обёртка → `lxdclient.Client.Rollback()` | Откат на last-good конфиг |
| GET | `/remote/machines/{id}/config/active` | — | `lxdclient.Client.ActiveConfig()` | Работающий конфиг **с машины** |
| GET | `/remote/machines/{id}/config/built` | — | `os.ReadFile(platform.GetRemoteConfigPathFor)` | Локально собранный конфиг машины; `404` если Configure ещё не делали |
| POST | `/remote/machines/{id}/deploy` | `{config?}` (опц. произвольный JSON-конфиг) | цепочка из `machineListPanel.deployTo`: built config → `CollectDeployResources()` → `Registry.SyncResources()` → `Registry.ApplyConfig()` | Ресурсы строго раньше конфига. Ответ `{ok, resources_uploaded, config_sha}`. Демон отклонил конфиг (`ApplyError.Rejected`) → `422`, инстанс не тронут; конфликт ресурса in-use → `409` |

### 3.3 Профиль машины (wizard state) — зеркало `/state/*`

Существующие state-handlers параметризуются scope'ом: тот же код, но пути файлов
берутся от `platform.GetWizardStatePathFor(execDir, ConfigTargetRemote, id)`.

| Метод | Путь | Реализация |
|---|---|---|
| GET | `/remote/machines/{id}/state/full` | зеркало `/state/full` |
| GET/PATCH | `/remote/machines/{id}/state/rules` | зеркало `/state/rules` |
| GET/PATCH | `/remote/machines/{id}/state/dns` | зеркало `/state/dns` |
| GET/PATCH | `/remote/machines/{id}/state/dns/rules` | зеркало `/state/dns/rules` |
| GET | `/remote/machines/{id}/state/outbounds/resolved` | зеркало `/state/outbounds/resolved` |

**Известное ограничение v1:** PATCH меняет state машины, но её `config.json`
сегодня пересобирает только визард (`presenter_save.go`) — программной сборки
remote-конфига нет. До выноса сборки из презентера (§7) цикл
«PATCH → deploy» требует шага Save в UI. Endpoint
`POST /remote/machines/{id}/action/rebuild-config` заводится **вместе** с этим
рефакторингом, не раньше.

### 3.4 Наблюдаемость машины (gRPC `daemon.StartedService` + admin REST)

Обёртка над `services.LxdRemoteTransport` (core/services/lxd_remote_transport.go).
Транспорты кешируются per-machine (§6.2).

| Метод | Путь | Тело / параметры | Реализация | Заметки |
|---|---|---|---|---|
| GET | `/remote/machines/{id}/groups` | — | `Transport.Groups()` | Теги selector-групп |
| GET | `/remote/machines/{id}/proxies` | `?group=` | `Transport.GroupProxies()` | `{proxies[], selected}`; без `group` — первая группа |
| POST | `/remote/machines/{id}/proxies/switch` | `{group, name}` | `Transport.SwitchProxy()` | gRPC `SelectOutbound` |
| POST | `/remote/machines/{id}/proxies/delay` | `{name}` | `Transport.Delay()` | Точечный URL-test, `{delay_ms}` |
| GET | `/remote/machines/{id}/pool` | `?group=` | `Transport.PoolSlots()` | Пул балансировщика (lx-RPC `GetPool`) |
| GET | `/remote/machines/{id}/rules` | — | `Transport.Rules()` | Таблица правил ядра машины |
| GET | `/remote/machines/{id}/outbounds` | — | `Transport.Outbounds()` | Теги outbound'ов |
| GET | `/remote/machines/{id}/status` | — | первое событие `SubscribeStatus` + `StartedAt()` | `{status{…}, started_at, uptime_s}` |
| GET | `/remote/machines/{id}/connections` | — | snapshot из `SubscribeConnections` | Разово: подключиться, снять snapshot, закрыть |
| DELETE | `/remote/machines/{id}/connections/{conn_id}` | — | `Transport.CloseConnection()` | |
| DELETE | `/remote/machines/{id}/connections` | — | новая обёртка → gRPC `CloseAllConnections` | |
| GET | `/remote/machines/{id}/dns/queries` | `?duration=5s&max=200` | окно сборки `SubscribeDNSQueries` | Стрим → снапшот: собрать события окна, отдать `{events[], truncated}` |
| GET | `/remote/machines/{id}/logs` | `?duration=5s&max=500` | новая обёртка → gRPC `SubscribeLog` | Тот же паттерн окна |
| GET | `/remote/machines/{id}/host` | — | `Transport.HostInfo()` | Телеметрия: CPU, память, диски, сенсоры (admin REST) |
| GET | `/remote/machines/{id}/host/interfaces` | — | `Transport.HostInterfaces()` | Интерфейсы со счётчиками |
| GET | `/remote/machines/{id}/clients` | — | `Transport.ClientsInfo()` | Справочник устройств сети машины |
| PUT | `/remote/machines/{id}/clients/{key}/label` | `{name}` | `Transport.SetClientLabel()` | |
| DELETE | `/remote/machines/{id}/clients/{key}/label` | — | `Transport.DeleteClientLabel()` | |

`duration` клампится (≤ 60s), `max` — (≤ 5000): endpoint остаётся снапшотом, не
вечным стримом. Подписки (SSE) — сознательно v2 (§7).

### 3.5 Ресурсы машины — `/remote/machines/{id}/resources`

Обёртка над core/services/lxd_remote_resources.go.

| Метод | Путь | Реализация | Заметки |
|---|---|---|---|
| GET | `/remote/machines/{id}/resources` | `Registry.ResourceOverview()` | Сводка local vs remote: имя, sha, состояние (synced / stale / local-only / remote-only) |
| GET | `/remote/machines/{id}/resources/{name}` | `lxdclient.Client.ResourceContent()` | Отдаёт содержимое с машины |
| PUT | `/remote/machines/{id}/resources/{name}` | `Registry.UploadResource()` | Заливает файл из локальной папки машины; произвольное содержимое — через raw REST passthrough (`PUT /admin/resources/{name}`) |
| POST | `/remote/machines/{id}/resources/{name}/download` | `Registry.DownloadResource()` | Сохраняет с машины в локальную папку машины |
| DELETE | `/remote/machines/{id}/resources/{name}` | `Registry.DeleteRemoteResource()` | `409` если имя занято ссылкой активного/last-good конфига |
| POST | `/remote/machines/{id}/resources/sync` | `CollectDeployResources()` + `SyncResources()` | Синк всех ресурсов built-конфига без деплоя самого конфига |

### 3.6 Локальный демон — `/daemon/*` (darwin-only)

Обёртка над core/daemon_manager_darwin.go. Группа существует только в
darwin-сборке; на остальных платформах путей нет и в манифесте они не значатся.

| Метод | Путь | Тело | Реализация | Заметки |
|---|---|---|---|---|
| GET | `/daemon/status` | — | `AppController.DaemonStatusSnapshot()` | Сопряжение, адрес, режим движка, доступность, статус ядра, поддержка `lxd` ядром (`CoreSupportsLxd`) |
| POST | `/daemon/pair` | `{invite, secret?}` | `PairDaemonWithInvite()` | |
| POST | `/daemon/unpair` | — | `UnpairDaemon()` | |
| PATCH | `/daemon/settings` | `{addr?, secret?}` | `SetDaemonAddress()` / `SetDaemonSecret()` | |
| GET | `/daemon/engine` | — | settings `BackendMode` | `{mode:"classic"\|"daemon"}` |
| POST | `/daemon/engine` | `{mode}` | запись `BackendMode` + `reloadDaemonBackendIfActive()` | `409` если ядро запущено (движок переключается только на остановленном VPN) |
| GET | `/daemon/commands` | — | `DaemonInstallCommand()`, `DaemonUninstallCommand(purge)`, `DaemonRepairCommand()`, `DaemonKickstartCommand()`, `DaemonShowSecretCommand()` | `{install, uninstall, uninstall_purge, repair, kickstart, show_secret}` — только строки, API их не исполняет |

Start/stop ядра при daemon-движке **уже** покрыты общими `/action/start`,
`/action/stop` — они ходят через шов `CoreBackend` и не знают о движке. Новых
ручек не нужно.

### 3.7 Произвольные запросы (passthrough) — ядро запроса

#### REST

| Метод | Путь | Назначение |
|---|---|---|
| POST | `/remote/machines/{id}/raw/rest` | Произвольный admin-REST-вызов на демон машины |
| POST | `/daemon/raw/rest` | То же на локальный демон (darwin) |

Тело запроса:

```json
{
  "method": "GET",
  "path": "/admin/status",
  "body": {"any": "json"},
  "body_base64": "…",
  "timeout": "10s"
}
```

`path` обязан начинаться с `/`; `body` (JSON) и `body_base64` (бинарь)
взаимоисключающие. Транспорт — тот же `lxdclient.Client.do()`: mTLS-ключи, пин
и секрет из записи реестра / настроек демона. Ответ:

```json
{"status": 200, "content_type": "application/json", "body": {…}}
```

`body` отдаётся распарсенным JSON'ом, если парсится; иначе `body_base64` +
`content_type`.

#### gRPC

| Метод | Путь | Назначение |
|---|---|---|
| POST | `/remote/machines/{id}/raw/grpc` | Произвольный gRPC-вызов `daemon.*` на машину |
| POST | `/daemon/raw/grpc` | То же на локальный демон (darwin) |
| GET | `/grpc/methods` | Discovery: список методов обоих сервисов |

Тело запроса:

```json
{
  "method": "/daemon.StartedService/URLTest",
  "request": {"link": "https://cp.cloudflare.com"},
  "timeout": "15s",
  "duration": "5s",
  "max_events": 100
}
```

Реализация **без ручной таблицы методов**: `internal/daemonpb` сгенерирован
стандартным `protoc-gen-go`, все дескрипторы уже в `protoregistry.GlobalFiles`.
Резолв по полному имени метода → `protojson.Unmarshal` JSON'а в request-тип →
`grpc.ClientConn.Invoke` / `NewStream` → `protojson.Marshal` ответа. Новые
методы после обновления pb-файлов (SYNC_REV) подхватываются без правок API.

| Вид метода | Поведение |
|---|---|
| unary | один вызов → `{"response": {…}}` |
| server-stream | собрать события до `duration`/`max_events` → `{"events": […], "truncated": bool}` |
| client-stream / bidi (`ProvideUSBDevices`, `StartTailscaleSSHSession`) | `501 Not Implemented` |

`GET /grpc/methods` отдаёт из дескрипторов:
`[{service, method, kind: "unary"|"server_stream"|…, input, output}]` — агенту
достаточно этого списка плюс знания схемы protojson.

Auth-канал (bearer-секрет в metadata для plain-h2c) — те же интерсепторы, что в
`lxdclient.DialGRPC()`.

### 3.8 UI-override — `/remote/ui*` (добавлено по запросу после v1-карты)

То, что в UI делают кнопки Connect/Disconnect вкладки Remote (SPEC 097/098):
перевод вкладки Servers лаунчера на машину. Реализация — те же
`SetLxdRemoteOverride`/`ClearLxdRemoteOverride`, что у кнопок, через хуки
`UIService.LxdOverride*Func` (регистрирует `RegisterOverrideAPIHooks` в
`NewApp`); паритет по построению.

| Метод | Путь | Реализация | Заметки |
|---|---|---|---|
| GET | `/remote/ui` | `GetLxdRemoteOverride` | `{connected, machine_id, machine_name}` |
| POST | `/remote/machines/{id}/ui/connect` | health-гейт → `SetLxdRemoteOverride` | Недоступная машина → `502`, override не тронут; idle-ядро — `warning`, не отказ |
| POST | `/remote/ui/disconnect` | `ClearLxdRemoteOverride` | Идемпотентен |

Headless-запуск / UI ещё не создан → `503` (`ErrUIUnavailable`): хуки
читаются на каждый вызов, поэтому порядок старта Debug API и окна не важен.

## 4. Коды ошибок (общие для новых групп)

| Код | Смысл |
|---|---|
| 400 | битый JSON, невалидный `method`/`path`/`timeout` |
| 404 | неизвестный machine id / ресурс / built-конфиг ещё не собран |
| 409 | конфликт: ресурс in-use, engine-switch при запущенном ядре, overwrite без флага |
| 422 | демон отклонил конфиг валидацией (`ApplyError.Rejected`); семантическая ошибка тела |
| 501 | client/bidi-stream в raw gRPC; группа `/daemon/*` вне darwin (если решим отвечать, а не скрывать) |
| 502 | машина недоступна (сеть/пин/отзыв) — с текстом причины из транспорта |
| 504 | server-side timeout вызова |

## 5. Изменения в манифесте

- Все новые endpoints — через `Server.endpoints()` (SPEC 078): попадают в
  `GET /` и `GET /help` автоматически.
- `GET /version` остаётся `debugapi/v1` — поверхность аддитивная, breaking
  changes нет.
- В манифест добавляется секция `capabilities`: `{"remote": true, "daemon":
  true|false, "raw_grpc": true}` — агент сразу видит, есть ли на этой сборке
  remote/daemon-группы (Win7 — нет, non-darwin — без `/daemon/*`).

## 6. Архитектура реализации

### 6.1 Фасады

`ControllerFacade` не раздувается — две новые узкие поверхности в
`core/debugapi`:

- `RemoteFacade` — обёртка над `*services.RemoteRegistry` + транспорт-кеш +
  deploy-цепочка (логика `deployTo` выносится из `ui/machine_list_panel.go` в
  `core/services`, UI и API зовут одну функцию — паритет по построению, как
  того требует урок SPEC 098).
- `DaemonFacade` (darwin) — методы `AppController` из
  `daemon_manager_darwin.go`.

Wiring — в `core/debugapi_wiring.go` рядом с существующим.

### 6.2 Кеш транспортов

gRPC-dial на каждый HTTP-запрос — недопустимо (mTLS handshake + HTTP/2 на
каждый `GET /proxies`). Пул `map[machineID]*LxdRemoteTransport` с lazy dial,
close по idle-таймеру (например 90s) и обязательным close при
`DELETE /remote/machines/{id}` и `repair`.

### 6.3 Build tags

- `/remote/*` и raw-gRPC зависят от `daemonpb`/`grpc` → файлы под тем же
  набором тегов, что и remote-сервисы; **Win7-сборка** (`go.win7.mod` без
  gRPC) собирается без этой группы, `endpoints()` её не регистрирует.
- `/daemon/*` — `//go:build darwin`.

### 6.4 Конкуррентность

- Registry сам сериализует запись реестра (внутренний mutex) — API-слою
  мьютекс не нужен.
- Зеркала `/remote/machines/{id}/state/*` используют **per-machine** mutex
  (аналог `stateMu`), не глобальный — PATCH двух разных машин не должен
  выстраиваться в очередь.

## 7. Сознательно за рамками v1

| Что | Почему | Куда |
|---|---|---|
| SSE/WebSocket-подписки на стримы (`connections`, `dns`, `logs`, `status`) | debugapi сегодня snapshot-only; окно `?duration=` покрывает скриптовые сценарии | v2: `/remote/machines/{id}/stream/*` с `text/event-stream` |
| `POST /remote/machines/{id}/action/rebuild-config` | Сборка remote-конфига живёт в презентере визарда; сначала рефакторинг выноса в core | вместе с рефакторингом (отдельная спека) |
| Remote-override Clash API (SPEC 064, вкладка REMOTE) | Legacy-канал к classic-лаунчеру по Clash HTTP; иной протокол, не lxd. Управление им из API — отдельный разговор | при спросе |
| `ImportPairedDaemon` (перенос локального сопряжения в реестр) | Одноразовая миграционная операция UI | не экспонируем |
| Исполнение привилегированных команд (`--service=install` и т.п.) | Принцип «sudo только в вашем терминале» — неизменяемый (CONSTITUTION) | никогда |

## 8. Критерии приёмки

1. `GET /help` показывает все новые endpoints; на Win7-сборке remote-группы
   нет, на Windows/Linux нет `/daemon/*` — и `capabilities` манифеста это
   отражает.
2. Полный цикл без UI: pair → configure (PATCH state) → *(Save в UI, пока нет
   rebuild)* → deploy → health показывает `active_sha` == sha задеплоенного
   файла → switch node → stop core.
3. `POST /remote/machines/{id}/raw/grpc` c `/daemon.StartedService/GetGroups`
   возвращает те же группы, что `GET /remote/machines/{id}/groups`.
4. `POST /remote/machines/{id}/raw/rest` c `{"method":"GET","path":"/admin/status"}`
   эквивалентен `/health` (модуль reachability-полей).
5. Недоступная машина: любой её endpoint отвечает `502` за ≤ timeout, не
   вешает listener.
6. Go-тесты: unit на резолв gRPC-метода по имени + protojson round-trip; на
   окно server-stream (`max_events`/`duration`); на 404/409/422/502-маппинг.
