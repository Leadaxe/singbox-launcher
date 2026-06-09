# Debug API

Локальный HTTP API на `127.0.0.1`, bearer-auth, выключен по умолчанию. **25 endpoint'ов** (24 protected + unauthenticated `/ping`) в 6 группах: health/info, state read, state write, actions, traffic profiler, snapshot. Используется для автоматизации (bash + curl), MCP-обёрток для AI-агентов, CI/CD-валидации шаблонов, headless-deployment, и для одной кнопки **Copy snapshot** в Diagnostics (это тот же `/debug/snapshot`).

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
# → {"launcher":"v1.1.0","singbox":"1.13.13-lx.3","api":"debugapi/v1"}
```

---

## Подключение

| Что | Где |
|---|---|
| Bind | `127.0.0.1:<port>` — **hard-coded loopback**, на LAN не вынесешь |
| Дефолтный порт | **9263** |
| Override порта | `bin/settings.json` → `debug_api_port` (1024–65535, `0` = дефолт) |
| Включить/выключить | `bin/settings.json` → `debug_api_enabled` (UI: Settings → checkbox) |
| Bearer-токен | `bin/settings.json` → `debug_api_token` (UI: Diagnostics → Copy token) |
| Регенерация токена | UI: **Settings → Debug API → «Regenerate»** (с подтверждением; ротирует токен и перезапускает listener). Альтернатива — удалить ключ из `settings.json` и перезапустить лаунчер |
| Comparison | `subtle.ConstantTimeCompare` (constant-time) |
| Header | `Authorization: Bearer <token>` |

Адрес виден в Diagnostics-табе рядом с чекбоксом — копи-пейст готовой строки `127.0.0.1:<port>`.

---

## Health & info

| Method | Path | Auth | Response |
|---|---|---|---|
| GET | `/ping` | — | `{"ok":true}` |
| GET | `/version` | ✓ | `{"launcher":"v…","singbox":"1.13.13-lx.3","api":"debugapi/v1"}` |

```bash
curl -s "$API/ping"
curl -s -H "Authorization: Bearer $TOKEN" "$API/version"
```

---

## State read

| Method | Path | Назначение |
|---|---|---|
| GET | `/state` | Live runtime snapshot: `{running, active_proxy, selected_group, singbox_version, subs_last_updated_unix}` |
| GET | `/proxies` | Список прокси (`[]api.ProxyInfo`) — из текущего sing-box config |
| GET | `/state/full` | Полный `state.json` (после load + миграций) |
| GET | `/state/rules` | `{"rules":[]state.Rule}` — секция SPEC 053 |
| GET | `/state/dns` | Вся секция `state.DNSOptions` (SPEC 056) |
| GET | `/state/dns/rules` | `{"text":"..."}` — **только USER**-правила как wizard-текст. Preset-правила не включаются (они toggle-ref'ы) |
| GET | `/state/outbounds/resolved` | `{"outbounds": []OutboundConfig}` — merge'нутые после SPEC 057/058 expansion (template + preset patches + user overrides) |

```bash
# Что сейчас выбрано
curl -s -H "Authorization: Bearer $TOKEN" "$API/state" | jq

# Полная конфигурация
curl -s -H "Authorization: Bearer $TOKEN" "$API/state/full" > backup.json
```

**Ошибки:** `401` (no/bad bearer), `404` (state.json не существует — fresh install), `500` (load/parse error).

---

## State write

Все patch-endpoint'ы возвращают `{"ok":true,"diff_summary":["..."]}` на успех. Sync-write через `state.Save` → atomic `.tmp + Rename`; **per-path mutex отсутствует** (полагается на atomic write — concurrent PATCH safe от частичной записи, но last-write-wins).

| Method | Path | Body | Что делает |
|---|---|---|---|
| PATCH | `/state/rules` | `{"mode":"replace"\|"append", "rules":[]state.Rule}` | Заменяет / добавляет правила. Каждое валидируется через `r.DecodeBody()` (kind discriminator: preset/inline/srs). |
| PATCH | `/state/dns` | `state.DNSOptions` | Заменяет **всю** dns_options (servers + rules). Каждый server/rule валидируется по `kind`. |
| PATCH | `/state/dns/rules` | `{"text":"..."}` | Заменяет **только USER** rules; preset-rules сохраняются. `""` (пустой текст) = wipe user rules. |

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
```

**Ошибки:** `400` (битый JSON / неизвестный mode), `422` (semantic validation: unknown rule kind, unknown DNS server kind, body decode fail), `500` (load/save), `405` (метод).

---

## Settings

`bin/settings.json` — launcher-level preferences (отдельный namespace от `state.json`). Изменения подхватываются на лету: subscription fetcher читает `LoadSubscriptionSettingsFunc` на каждом запросе, sing-box restart НЕ нужен.

| Method | Path | Что делает |
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

## Actions

Все `POST`-only (`GET` → 405). Synchronous (блокируют до завершения). Success = `{"ok":true}`.

| Method | Path | Что делает |
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

Контроль за live DNS/TCP/UDP capture session'ом и просмотр rolling buffer'а (последние 10 минут). Та же подсистема, что окно **Traffic Profiler** в Diagnostics.

| Method | Path | Назначение |
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

## Snapshot

| Method | Path | Назначение |
|---|---|---|
| GET | `/debug/snapshot` | `core.snapshot.Build()` — template + state + cache + config.json в одном JSON-е. Та же функция, что под кнопкой **Copy snapshot** в Diagnostics. Идеально для bug-report'а |

```bash
# Сохранить полный snapshot для bug-report'а
curl -s -H "Authorization: Bearer $TOKEN" "$API/debug/snapshot" > snapshot-$(date +%Y%m%d-%H%M%S).json
```

Response shape:
```json
{
  "captured_at": "2026-05-28T12:00:00Z",
  "launcher_version": "v1.1.0",
  "singbox_version": "1.13.13-lx.3",
  "files": { "state.json": "...", "config.json": "...", "wizard_template.json": "..." },
  "missing": [],
  "errors": []
}
```

---

## Общие правила

- **Auth header:** `Authorization: Bearer <token>` обязателен везде кроме `GET /ping`.
- **Content-Type:** `application/json` для всех PATCH/POST с body.
- **Errors:** `401` — нет/неверный bearer; `404` — ресурс не найден; `405` — метод не разрешён; `409` — конфликт состояния (traffic session); `422` — semantic validation fail; `500` — внутренняя ошибка.
- **Concurrency:** state-write через atomic `.tmp + Rename`; per-resource mutex нет — concurrent PATCH safe от частичной записи, но **last-write-wins**, не merge.
- **Versioning:** header `api` в `/version` сейчас фиксирован `debugapi/v1`. Breaking changes планируются как `v2`-namespace (`/v2/...`), пока без авто-discovery.

---

## Use-cases

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
- **Token rotation** — нет UI; ручной flow: stop launcher → удалить `debug_api_token` из `bin/settings.json` → start launcher → токен будет регенерирован при первом включении.

---

## Source

| Файл | Что внутри |
|---|---|
| `core/debugapi/server.go` | Routing, auth middleware, `/ping`, `/version`, `/state`, `/proxies`, `/action/*` |
| `core/debugapi/state_endpoints.go` | `/state/full`, `/state/rules`, `/state/dns`, `/state/dns/rules`, `/state/outbounds/resolved` |
| `core/debugapi/traffic_endpoints.go` | Все `/traffic/*` |
| `core/debugapi/snapshot.go` | `/debug/snapshot` |
| `core/debugapi/debugapi_wiring.go` | Bridge между Server и controller (StartSingBox, StopSingBox, Update, Rebuild, PingAll) |
| `internal/locale/settings.go` | `debug_api_enabled`, `debug_api_port`, `debug_api_token` |
| `ui/diagnostics_tab.go` | UI toggle / Copy token / port entry |

История дизайна (необязательно к чтению): [SPEC 038](../SPECS/038-F-C-DEBUG_API/SPEC.md), [IMPLEMENTATION_REPORT](../SPECS/038-F-C-DEBUG_API/IMPLEMENTATION_REPORT.md).
