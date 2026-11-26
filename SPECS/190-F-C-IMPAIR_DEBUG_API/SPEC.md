# SPEC 190 — Тест с impair целиком скриптом через debug API

Задание владельца 26.11.2026 (через рецензента SPEC 138): как у LxBox (их задача 762),
но без обычного UI — impair в интерфейсе лаунчера нет и не будет. Ядро —
sing-box-lx `1.14.4-lx.37-rc.2` (SPEC 164: outbound `impair`; gRPC `SetImpair`,
`ScheduleImpair`, `GetImpairStatus`).

## 1. Что нужно и чем это покрыто

| Нужно | Маршрут debug API |
|---|---|
| Создать impair-узел как любой другой | `POST /state/servers` с `input` = JSON outbound `{"type":"impair",…}` — узел ложится в состояние и в `config.json` как есть (проверено `check` ядра rc2); удалить — `DELETE /state/servers?tag=` |
| Направить через него трафик | **новое:** `PATCH /state/servers?tag=&folder=` с `{"detour":{"tag":"imp"}}` — повесить, `{"detour":null}` — снять; корневой сервер или член папки; у удалённой машины — тот же `PATCH /remote/machines/{id}/state/servers` |
| Управлять линией на лету | **новое:** `GET /impair/{outbound}`, `POST /impair/{outbound}/set`, `POST /impair/{outbound}/schedule` (gRPC демона) |
| Снимать замеры | статус impair (`counters`, текущая `link_params`) — `GET /impair/{outbound}`; **новое:** `POST /proxies/delay` (URL-тест одного узла своего ядра), `GET /pool?group=` (пул urltest: карантин, счётчики, заморозка), `GET /outbounds/{node}/config` (узел, как его исполняет ядро: detour и правки); есть: `GET /proxies`, `POST /action/ping-all`, `GET /chains`, `POST /chains/{tag}/probe`, `/traffic/*` (соединения, сессии, ошибки по узлу) |

Правка состояния (`/state/*`) сохраняет состояние и пересобирает конфиг, как Save;
маршруты `/impair/*`, `/proxies/delay`, `/pool`, `/outbounds/*` — только в режиме
демона (группа `/daemon/*`). Ошибки ядра → HTTP: нет узла — 404, неверный аргумент /
не impair — 400, ядро не запущено — 409, ядро без RPC — 501.

## 2. Формат `/impair/*`

- `GET /impair/{outbound}` → статус ядра как есть: `{"tag", "link_params", "counters",
  "sublinks", "schedule"}` (схема — sing-box-lx `docs-lx/lx-impair.md`).
- `POST /impair/{outbound}/set`, тело — одна запись расписания без `at`:
  `{"link_params":{…}}` или `{"sublinks":[…]|false}`; ответ — статус.
- `POST /impair/{outbound}/schedule`, тело `{"entries":[…], "start":""|"now"|"first_use",
  "repeat":""|"60s"}`; `entries: []` — сброс к линии из конфига; ответ — статус.

## 3. Сценарий-пример

```bash
API=http://127.0.0.1:9263; H="Authorization: Bearer $TOKEN"; J="Content-Type: application/json"

# 1. impair-узел (direct с плохой линией) — как любой сервер из JSON
curl -sH "$H" -H "$J" -X POST $API/state/servers \
  -d '{"input":"{\"type\":\"impair\",\"tag\":\"imp\",\"link_params\":{\"profile\":\"lte-walk\"}}"}'

# 2. трафик узла de-1 — через imp
curl -sH "$H" -H "$J" -X PATCH "$API/state/servers?tag=de-1" -d '{"detour":{"tag":"imp"}}'
curl -sH "$H" $API/outbounds/de-1/config          # effective.detour == "imp"

# 3. замер до порчи
curl -sH "$H" -H "$J" -X POST $API/proxies/delay -d '{"name":"de-1"}'

# 4. линия на лету: обрыв, затем сценарий кругом каждые 60 с от первого трафика
curl -sH "$H" -H "$J" -X POST $API/impair/imp/set -d '{"link_params":{"outage":true,"reset":true}}'
curl -sH "$H" -H "$J" -X POST $API/impair/imp/schedule \
  -d '{"entries":[{"at":"0s","link_params":{"profile":"lte-walk"}},{"at":"20s","link_params":{"outage":true}}],"start":"first_use","repeat":"60s"}'

# 5. замеры под порчей: задержка узла, пул группы, счётчики impair, соединения
curl -sH "$H" -H "$J" -X POST $API/proxies/delay -d '{"name":"de-1"}'
curl -sH "$H" "$API/pool?group=auto-proxy-out"
curl -sH "$H" $API/impair/imp
curl -sH "$H" $API/traffic/live

# 6. убрать: сброс линии, снять detour, удалить узел
curl -sH "$H" -H "$J" -X POST $API/impair/imp/schedule -d '{"entries":[]}'
curl -sH "$H" -H "$J" -X PATCH "$API/state/servers?tag=de-1" -d '{"detour":null}'
curl -sH "$H" -X DELETE "$API/state/servers?tag=imp"
```

## 4. Проверки

- `TestImpairEndpointsPassThrough` (`core/debugapi`): gRPC-сервер в тесте; запросы
  `/impair/*` доходят до ядра как есть, ошибки — 404/400/409; `/proxies/delay`,
  `/pool`, `/outbounds/{node}/config`.
- `TestSetServerDetourImpairScenario` (`core/stateedit`): impair из JSON, detour
  корневого сервера и члена папки, снятие, отказы.
- `sing-box check` ядром rc2: impair-узел и узел с `detour` на него.
- Живой прогон на ядре — не делался (живые туннели на Mac не поднимаются).
