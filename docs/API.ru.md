# Debug API

**🌐 Язык**: [English](API.md) | Русский

Локальный HTTP API на `127.0.0.1`, bearer-auth, выключен по умолчанию. **Самоописываемый** (SPEC 078): `GET /` отдаёт манифест, `GET /help` — список эндпоинтов, так что агенту достаточно дать base URL + токен. Группы: discovery/info, state read, state write, actions, traffic profiler, snapshot. Используется для автоматизации (bash + curl), MCP-обёрток для AI-агентов, CI/CD-валидации шаблонов, headless-deployment и снятия полного снапшота для bug-report’а (`/debug/snapshot`).

> Source of truth: код `core/debugapi/`. Этот документ — generated-style сводка из реальных handler-ов; SPEC 038 описывает оригинальный дизайн и осталась как историческая референс.

---

## TL;DR

```bash
# 1. Включить в UI: Settings → Debug API (localhost) → ✓
# 2. Скопировать токен: тот же экран, кнопка "Copy token"
# 3. Записать в env
export TOKEN="<paste-here>"
export API="http://127.0.0.1:9263"

# 4. Проверить
curl -s "$API/ping"                                    # → {"ok":true}    (без auth)
curl -s -H "Authorization: Bearer $TOKEN" "$API/version"
# → {"launcher":"v1.2.2","singbox":"1.14.0-lx.5","api":"debugapi/v1"}
```

---

## Подключение

| Что | Где |
|---|---|
| Bind | `127.0.0.1:<port>` — **hard-coded loopback**, на LAN не вынесешь |
| Дефолтный порт | **9263** |
| Override порта | `bin/settings.json` → `debug_api_port` (1024–65535, `0` = дефолт) |
| Включить/выключить | `bin/settings.json` → `debug_api_enabled` (UI: Settings → checkbox) |
| Bearer-токен | `bin/settings.json` → `debug_api_token` (UI: Settings → Debug API → Copy token) |
| Регенерация токена | UI: **Settings → Debug API → «Regenerate»** (с подтверждением; ротирует токен и перезапускает listener). Альтернатива — удалить ключ из `settings.json` и перезапустить лаунчер |
| Comparison | `subtle.ConstantTimeCompare` (constant-time) |
| Header | `Authorization: Bearer <token>` |

Адрес виден в Settings → Debug API рядом с чекбоксом — копи-пейст готовой строки `127.0.0.1:<port>`.

---

## Обнаружение и справка

API **самоописываемый** (SPEC 078): дайте агенту base URL и токен — дальше он прочитает поверхность сам.

| Метод | Путь | Auth | Ответ |
|---|---|---|---|
| GET | `/ping` | — | `{"ok":true}` |
| GET | `/` | ✓ | **Манифест** — `api`, `spec`, `launcher`, `core`, `auth`, `docs` (ссылка на этот файл, привязанная к версии), `hint`, `endpoints[]` (метод/путь/описание). |
| GET | `/help` | ✓ | `{"endpoints":[{method,path,summary,auth}, …]}` — только список эндпоинтов. |
| GET | `/version` | ✓ | `{"launcher":"v…","singbox":"1.14.0-lx.5","api":"debugapi/v1"}` |

Авторизованный запрос к **неизвестному** пути возвращает `404` с указателем `docs` — агент, промахнувшийся с путём, возвращается к `/` и к этому файлу.

На экране Settings → Debug API есть кнопка **Copy API info**: она кладёт в буфер JSON-карточку подключения (`base_url`, `token`, `launcher`, `core`, `auth`, `docs`, `hint`) — передайте её агенту, и у него есть всё, чтобы подключиться с нуля.

```bash
curl -s "$API/ping"
curl -s -H "Authorization: Bearer $TOKEN" "$API/"       # manifest
curl -s -H "Authorization: Bearer $TOKEN" "$API/help"   # endpoint list
curl -s -H "Authorization: Bearer $TOKEN" "$API/version"
```

---

## Чтение состояния

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/state` | Снимок рантайма: `{running, active_proxy, selected_group, singbox_version, subs_last_updated_unix}` |
| GET | `/proxies` | Список прокси (`[]api.ProxyInfo`) — из текущего sing-box config |
| GET | `/state/full` | Полный `state.json` (после load + миграций) |
| GET | `/state/rules` | `{"rules":[]state.Rule}` — секция SPEC 053 |
| GET | `/state/dns` | Вся секция `state.DNSOptions` (SPEC 056) |
| GET | `/state/dns/rules` | `{"text":"..."}` — **только USER**-правила как wizard-текст. Preset-правила не включаются (они toggle-ref'ы) |
| GET | `/state/outbounds/resolved` | `{"outbounds": []Direction}` — merge'нутые после SPEC 057/058 expansion (template + preset patches + user overrides); поля SPEC 104 `label`/`disabled`/`auto` включены |
| GET | `/state/log-level` | `{level, is_set, default, effective, allowed}` — `level` = сырое `vars[log_level]` (`""` если не задан), `effective` = что реально возьмёт sing-box (при пустом — `default`, т.е. `warn`) |

```bash
# Что сейчас выбрано
curl -s -H "Authorization: Bearer $TOKEN" "$API/state" | jq

# Полная конфигурация
curl -s -H "Authorization: Bearer $TOKEN" "$API/state/full" > backup.json
```

**Ошибки:** `401` (no/bad bearer), `404` (state.json не существует — fresh install), `500` (load/parse error).

---

## Запись состояния

Все patch-endpoint'ы возвращают `{"ok":true,"diff_summary":["..."]}` на успех. Sync-write через `state.Save` → atomic `.tmp + Rename`; **per-path mutex отсутствует** (полагается на atomic write — concurrent PATCH safe от частичной записи, но last-write-wins).

| Метод | Путь | Тело | Что делает |
|---|---|---|---|
| PATCH | `/state/rules` | `{"mode":"replace"\|"append", "rules":[]state.Rule}` | Заменяет / добавляет правила. Каждое валидируется через `r.DecodeBody()` (kind discriminator: preset/inline/srs). |
| PATCH | `/state/dns` | `state.DNSOptions` | Заменяет **всю** dns_options (servers + rules). Каждый server/rule валидируется по `kind`. **Тело обязано содержать `servers` и/или `rules`** — keyless `{}` → `422` (защита от молчаливого стирания всей секции), состояние не трогается. |
| PATCH | `/state/dns/rules` | `{"text":"..."}` | Заменяет **только USER** rules; preset-rules сохраняются. `""` (пустой текст) = wipe user rules. |
| PATCH | `/state/log-level` | `{"level":"trace"\|"debug"\|"info"\|"warn"\|"error"\|"fatal"\|"panic"}` | Пишет `vars[log_level]` → forced rebuild `config.json` → **restart sing-box** (активные соединения рвутся). Отвечает `202` + `{"ok":true,"level":"...","warning":"active connections reset"}`, а не общим `{"ok":true,"diff_summary":[...]}`. Поле `level` обязательно; невалидный уровень → `400` со списком `allowed` (ядро не трогается). |

```bash
# Replace all rules с одним preset-ref'ом
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/state/rules" \
  -d '{"mode":"replace","rules":[{"kind":"preset","ref":"ru-direct","enabled":true,"body":{"vars":{}}}]}'

# Добавить одно inline-правило не трогая остальные
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/state/rules" \
  -d '{"mode":"append","rules":[{"kind":"inline","enabled":true,
        "body":{"name":"Block Reddit","match":{"domain_suffix":["reddit.com"]},"outbound":"reject"}}]}'

# Patch DNS rules text (как в UI Raw-режиме)
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/state/dns/rules" \
  -d '{"text":"{\"rules\":[{\"domain\":\"example.com\",\"server\":\"cf\"}]}"}'

# Поднять логи до trace (рвёт активные соединения — ядро перезапускается)
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/state/log-level" -d '{"level":"trace"}'
```

> `POST /traffic/verbose` — булев частный случай этой же ручки: умеет только `debug` (`true`) и `warn` (`false`). Для остальных уровней используйте `PATCH /state/log-level`.

**Ошибки:** `400` (битый JSON / неизвестный mode), `422` (semantic validation: unknown rule kind, unknown DNS server kind, body decode fail), `500` (load/save), `405` (метод).

---

## Настройки

`bin/settings.json` — launcher-level preferences (отдельный namespace от `state.json`). Изменения подхватываются на лету: subscription fetcher читает `LoadSubscriptionSettingsFunc` на каждом запросе, sing-box restart НЕ нужен.

| Метод | Путь | Что делает |
|---|---|---|
| GET | `/settings/user-agent` | `{user_agent, default, effective}` — `user_agent` raw stored (может быть пустой), `default` — что отдаст `BuildSubscriptionUserAgent()`, `effective` — что реально уйдёт в следующий fetch |
| PATCH | `/settings/user-agent` | `{"user_agent":"..."}` — записать кастомный UA. `{"user_agent":""}` = reset к default. Поле обязательно (пропуск = `400`) — иначе truncated request мог бы случайно стереть значение |

```bash
# Прочитать текущий + default + effective
curl -s -H "Authorization: Bearer $TOKEN" "$API/settings/user-agent" | jq

# Установить UA как v2rayN (для провайдеров, которые режут наш default)
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/settings/user-agent" \
  -d '{"user_agent":"v2rayN/7.5.0"}'

# Сбросить на default
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/settings/user-agent" \
  -d '{"user_agent":""}'
```

**Ошибки:** `400` (битый JSON / отсутствует `user_agent` поле), `500` (save settings.json), `405` (метод).

---

## Действия

Все `POST`-only (`GET` → 405). Synchronous (блокируют до завершения). Success = `{"ok":true}`.

| Метод | Путь | Что делает |
|---|---|---|
| POST | `/action/update-subs` | `ConfigService.UpdateConfigFromSubscriptions` — synchronous re-fetch всех подписок |
| POST | `/action/start` | Запускает sing-box (fire-and-forget) |
| POST | `/action/stop` | Останавливает sing-box (graceful, deadline 2s) |
| POST | `/action/ping-all` | Запускает ping всех прокси. **Caveat:** silent no-op если UIService не инициализирован (headless edge-case) |
| POST | `/action/rebuild-config` | `RebuildConfigIfDirty` — пересобирает `config.json` если есть stale-маркеры. Atomic `.tmp + Rename`. **Note:** doc-comment в коде обещает `{"rebuilt":bool}` в response, но handler возвращает только `{"ok":true}` (на доработке) |

```bash
# Обновить подписки и пересобрать config
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/action/update-subs"
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/action/rebuild-config"

# Рестарт sing-box
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/action/stop"
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/action/start"
```

---

## Traffic Profiler (SPEC 059)

Контроль за live DNS/TCP/UDP capture session'ом и просмотр rolling buffer'а (последние 60 секунд; параметр `last` клампится до 10 минут). Та же подсистема, что окно **Traffic Profiler** в Diagnostics.

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/traffic/status` | Состояние активной сессии (recording, target, events_dropped, etc.) |
| GET | `/traffic/live?last=60s` | Snapshot rolling buffer'а. `last` — Go duration (≤ 10 минут, > 0). Возвращает `{events, cutoff_ts}` |
| POST | `/traffic/start` | Body `{"target":"<process_path>","verbose":<bool>}`. Пустой target = system-wide. Verbose flips `log_level=debug` и рестартит sing-box. **409** если сессия уже активна |
| POST | `/traffic/stop` | Финализирует активную сессию. **404** если нет активной |
| POST | `/traffic/clear` | Стирает все завершённые сессии. Возвращает `{"cleared":N}` |
| GET | `/traffic/sessions` | Список всех сессий (completed + active с `active:true`) |
| GET | `/traffic/sessions/{id}` | Полный dump событий сессии |
| DELETE | `/traffic/sessions/{id}` | Удалить одну. **409** если сессия активна |
| GET | `/traffic/processes` | Список distinct-процессов в rolling buffer'е (для UI dropdown'а) |
| GET | `/traffic/verbose` | Текущий sing-box `log_level` |
| POST | `/traffic/verbose` | Body `{"enabled":<bool>}`. Toggle `log_level=debug/warn`. **202 Accepted** (требует sing-box reload); response: `{"ok":true,"level":"debug","warning":"active connections reset"}` |

```bash
# Записать всё что происходит в Firefox 10 секунд
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/traffic/start" -d '{"target":"/Applications/Firefox.app/Contents/MacOS/firefox","verbose":true}'
sleep 10
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/traffic/stop" | jq .session.id
# → "01J…"

# Получить полный лог сессии
curl -s -H "Authorization: Bearer $TOKEN" "$API/traffic/sessions/01J…" > firefox_session.json

# Live snapshot последних 30 секунд (без записи)
curl -s -H "Authorization: Bearer $TOKEN" "$API/traffic/live?last=30s" | jq '.events | length'
```

---

## Снапшот

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/debug/snapshot` | `core.snapshot.Build()` — template + state + cache + config.json в одном JSON-е. Идеально для bug-report'а |

```bash
# Сохранить полный snapshot для bug-report'а
curl -s -H "Authorization: Bearer $TOKEN" "$API/debug/snapshot" > snapshot-$(date +%Y%m%d-%H%M%S).json
```

Форма ответа:
```json
{
  "captured_at": "2026-05-28T12:00:00Z",
  "launcher_version": "v1.2.2",
  "singbox_version": "1.14.0-lx.5",
  "files": { "state.json": "...", "config.json": "...", "wizard_template.json": "..." },
  "missing": ["cache.json"],
  "errors": { "config.json": "read: permission denied" }
}
```

`missing` — массив, `errors` — объект `{файл: сообщение}`; пустые поля опускаются целиком (omitempty).

---

## Удалённые машины (SPEC 100)

Полная обёртка над реестром удалённых lxd-машин (SPEC 096–099). Каждый вызов
адресует машину явно — `/remote/machines/{id}/…`; понятия «активная машина» в
API нет. Манифест `GET /` несёт `capabilities` (`remote`/`daemon`/`raw_grpc`) —
по ним агент видит, какие группы есть в этой сборке (Win7 — без remote-группы,
не-macOS — без `/daemon/*`).

**Реестр:**

| Метод | Путь | Что делает |
|---|---|---|
| GET/POST | `/remote/machines` | Список / сопряжение `{invite, name?, addr?, secret?}` (приглашение `адрес#отпечаток#код`) |
| GET/PATCH/DELETE | `/remote/machines/{id}` | Запись / правка `{name?,addr?,goos?,goarch?}` / удаление (ответ предупреждает: доступ на стороне демона не отозван) |
| POST | `/remote/machines/{id}/repair` | Пере-сопряжение `{invite, addr?, secret?}` с перевыпуском ключа; профиль машины сохраняется |
| POST | `/remote/machines/{id}/profile/copy-from` | Копия настроек `{source_id, overwrite?}`; существующий state без `overwrite=true` → `409` |

**Ядро и деплой:**

| Метод | Путь | Что делает |
|---|---|---|
| GET | `/remote/machines/{id}/health` | `{reachable, core_status, active_sha, last_good_sha, …}` — сверка SHA = проверка «доехало» |
| POST | `/remote/machines/{id}/core/start` \| `stop` \| `rollback` | Управление ядром машины (stop рвёт VPN её клиентов — подтверждения на стороне API нет) |
| GET | `/remote/machines/{id}/config/active` \| `built` | Работающий конфиг с машины / локально собранный |
| POST | `/remote/machines/{id}/deploy` | Ресурсы → конфиг (та же цепочка, что кнопка Deploy). Body `{config:{…}}` опционален. `422` = демон отклонил конфиг, инстанс не тронут |

**Состояние (зеркала `/state/*`):** `GET /remote/machines/{id}/state/full`,
`GET/PATCH …/state/rules`, `…/state/dns`, `…/state/dns/rules`,
`GET …/state/outbounds/resolved` — те же контракты, что у локальных ручек.
**Ограничение:** PATCH меняет state машины, но её `config.json` собирает только
визард (Configure → Save) — программной пересборки пока нет.

**Наблюдаемость:** `GET …/groups`, `GET …/proxies?group=`,
`POST …/proxies/switch {group,name}`, `POST …/proxies/delay {name}`,
`GET …/pool?group=`, `GET …/rules`, `GET …/outbounds`, `GET …/status`,
`GET/DELETE …/connections`, `DELETE …/connections/{conn_id}`,
`GET …/dns/queries?duration=5s&max=200`, `GET …/logs?duration=&max=`,
`GET …/host`, `GET …/host/interfaces`, `GET …/clients`,
`PUT/DELETE …/clients/{key}/label`. Стримовые источники отдаются окнами
(`duration` ≤ 60s, `max` ≤ 5000) — SSE-подписок в v1 нет.

**Ресурс-стор:** `GET …/resources` (сводка local vs machine),
`POST …/resources/sync`, `GET/PUT/DELETE …/resources/{name}`,
`POST …/resources/{name}/download`. `409` = имя занято живой ссылкой конфига.

**UI-override (кнопки Connect/Disconnect вкладки Remote):** обычные
remote-вызовы выбор в UI не трогают — эти три ручки управляют именно тем, на
какую машину смотрит вкладка Servers лаунчера.

| Метод | Путь | Что делает |
|---|---|---|
| GET | `/remote/ui` | `{connected, machine_id, machine_name}`; `connected:false` = локальное ядро |
| POST | `/remote/machines/{id}/ui/connect` | Перевести вкладку Servers на машину. Health-гейт до переключения: недоступная машина → `502`, override не тронут. Ядро в idle — не отказ (в ответе `warning`) |
| POST | `/remote/ui/disconnect` | Вернуть вкладку Servers к локальному ядру. Идемпотентен |

`503` на всех трёх — лаунчер запущен headless либо UI ещё не создан.

**Ошибки группы:** `404` неизвестная машина / нет built-конфига; `409`
конфликт; `422` демон отклонил конфиг; `502` машина недоступна; `504` таймаут.

```bash
# Сопрячься с роутером и посмотреть его узлы
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/remote/machines" -d '{"invite":"192.168.10.1:19091#3f9c…#Q7PLM2","name":"RouteRich"}'
curl -s -H "Authorization: Bearer $TOKEN" "$API/remote/machines/routerich/proxies?group=proxy-out" | jq

# Деплой и проверка «доехало»
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/remote/machines/routerich/deploy" | jq .config_sha
curl -s -H "Authorization: Bearer $TOKEN" "$API/remote/machines/routerich/health" | jq .active_sha
```

---

## Локальный демон `/daemon/*` (macOS)

Группа существует только в darwin-сборке (см. `capabilities.daemon`).
Start/stop ядра при daemon-движке идут через общие `/action/start|stop`
(шов `CoreBackend`) — отдельных ручек нет.

| Метод | Путь | Что делает |
|---|---|---|
| GET | `/daemon/status` | Сопряжение, служба, доступность, статус ядра, паспорт демона |
| POST | `/daemon/pair` | `{invite, secret?}` — сопряжение с локальным демоном |
| POST | `/daemon/unpair` | Забыть сопряжение (ключи, пин, секрет) |
| PATCH | `/daemon/settings` | `{addr?, secret?}` |
| GET/POST | `/daemon/engine` | Движок ядра: `{"mode":"classic"\|"daemon"}`; POST при работающем VPN → `409` |
| GET | `/daemon/commands` | Готовые sudo-команды (install/uninstall/repair/kickstart/show_secret). **API их не исполняет** — принцип «sudo только в вашем терминале» |

---

## Произвольные вызовы (raw passthrough)

Туннель к **сопряжённому** демону — удалённой машине или локальному. Канал,
пин и мандат берутся из реестра/настроек; запрос на произвольный адрес через
эти ручки сделать нельзя.

| Метод | Путь |
|---|---|
| POST | `/remote/machines/{id}/raw/rest` \| `/daemon/raw/rest` |
| POST | `/remote/machines/{id}/raw/grpc` \| `/daemon/raw/grpc` |
| GET | `/grpc/methods` — discovery всех `daemon.*` методов (kind, input, output) |

**REST:** body `{"method":"GET","path":"/admin/status","body":{…}|"body_base64":"…","content_type":"…"}`.
`path` начинается с `/`; `body` и `body_base64` взаимоисключающие. Ответ —
`{"status":<код демона>,"content_type":…,"body":{…}|"body_base64":"…"}`; наш
HTTP-статус всегда 200, статус демона — данные (иначе не отличить «нашу» 404
от 404 демона).

**gRPC:** body `{"method":"/daemon.StartedService/URLTest","request":{…},"timeout":"15s","duration":"5s","max_events":100}`.
Метод резолвится по имени через protoregistry (ручной таблицы нет — новые RPC
после обновления `internal/daemonpb` подхватываются сами), JSON ↔ proto через
protojson. Unary → `{"response":{…}}`; server-stream → окно
`{"events":[…],"truncated":bool}`; client/bidi-stream → `501`.

```bash
# Любой admin-REST запрос к машине
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/remote/machines/routerich/raw/rest" -d '{"method":"GET","path":"/admin/info"}' | jq .body

# Любой gRPC: URL-тест узла на стороне машины
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/remote/machines/routerich/raw/grpc" \
  -d '{"method":"/daemon.StartedService/URLTestOutbound","request":{"outbound_tag":"JP-01","link":"https://cp.cloudflare.com","timeout":10000}}' | jq

# Окно лога ядра машины через стрим
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/remote/machines/routerich/raw/grpc" \
  -d '{"method":"/daemon.StartedService/SubscribeLog","duration":"3s","max_events":200}' | jq '.events | length'
```

---

## Общие правила

- **Auth header:** `Authorization: Bearer <token>` обязателен везде кроме `GET /ping`.
- **Content-Type:** `application/json` для всех PATCH/POST с body.
- **Errors:** `401` — нет/неверный bearer; `404` — ресурс не найден; `405` — метод не разрешён; `409` — конфликт состояния (traffic session); `422` — semantic validation fail; `500` — внутренняя ошибка.
- **Concurrency:** state-write через atomic `.tmp + Rename`; per-resource mutex нет — concurrent PATCH safe от частичной записи, но **last-write-wins**, не merge.
- **Versioning:** header `api` в `/version` сейчас фиксирован `debugapi/v1`. Breaking changes планируются как `v2`-namespace (`/v2/...`), пока без авто-discovery.

---

## Сценарии использования

- **Bash + curl скрипты** — health-check в systemd-юните, регулярный refresh подписок из cron, валидация что `running=true` после deploy.
- **MCP-обёртки для AI-агентов** — Claude / GPT / прочие могут читать `/state/full`, делать PATCH'и, триггерить rebuild. См. [SPEC 038 §6.5](../SPECS/038-F-C-DEBUG_API/SPEC.md).
- **CI/CD валидация шаблонов** — `wizard_template.json` подложить, запустить лаунчер headless, PATCH-нуть state через API, дождаться rebuild, прочитать generated `config.json`, прогнать sing-box-check.
- **Regression-фикстуры** — снимать `/debug/snapshot` до/после изменения, diff'ить.
- **Live observability** — `/traffic/live?last=10s` + `jq` = realtime tail соединений без открытия UI.

---

## Ограничения

- **Loopback-only.** Нет TLS, нет CORS, нет LAN-bind. Для удалённого доступа — ssh-tunnel: `ssh -L 9263:127.0.0.1:9263 user@host`.
- **Нет streaming endpoint'ов** (WebSocket / SSE). `/traffic/live?last=...` — snapshot, не subscribe. Для long-tail polling берите rolling buffer чанками.
- **Нет `GET /logs?tail=N`** — sing-box логи читать напрямую из `bin/logs/`.
- **Нет switch_proxy / list_groups / get_logs** — упоминались в SPEC 038 §183 как future work, не реализованы.
- **Toggle verbose** рестартит sing-box — активные TCP-соединения дропаются. Response предупреждает (`"warning":"active connections reset"`).
- **Token rotation** — кнопка **Settings → Debug API → «Regenerate»** (с подтверждением; ротирует токен и перезапускает listener). Альтернатива без UI: stop launcher → удалить `debug_api_token` из `bin/settings.json` → start launcher → токен будет регенерирован при первом включении.

---

## Исходники

| Файл | Что внутри |
|---|---|
| `core/debugapi/server.go` | Routing, auth middleware, `/ping`, `/version`, `/state`, `/proxies`, `/action/*` |
| `core/debugapi/state_endpoints.go` | `/state/full`, `/state/rules`, `/state/dns`, `/state/dns/rules`, `/state/outbounds/resolved` |
| `core/debugapi/log_level_endpoint.go` | `/state/log-level` (валидация уровня + core restart через `core.ApplyLogLevelAndReloadCore`) |
| `core/debugapi/traffic_endpoints.go` | Все `/traffic/*` |
| `core/debugapi/snapshot.go` | `/debug/snapshot` |
| `core/debugapi_wiring.go` | Bridge между Server и controller (StartSingBox, StopSingBox, Update, Rebuild, PingAll) |
| `internal/locale/settings.go` | `debug_api_enabled`, `debug_api_port`, `debug_api_token` |
| `ui/settings_tab.go` | UI toggle / Copy token / port entry |

История дизайна (необязательно к чтению): [SPEC 038](../SPECS/038-F-C-DEBUG_API/SPEC.md), [IMPLEMENTATION_REPORT](../SPECS/038-F-C-DEBUG_API/IMPLEMENTATION_REPORT.md).
