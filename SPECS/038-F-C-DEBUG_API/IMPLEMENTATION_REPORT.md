# Реализация: 038 — DEBUG_API

**Коммиты:** `220256c`, `154a1e7`, `ba15424`, `264ffaf` (ветка `night-work`, 2026-04-22).
**Спека написана ретроспективно** — фича шипилась без предварительного SPEC.

## Что сделано

### `220256c` — базовый сервер

- `core/debugapi/server.go`: `Server`, `New`, `Start`, `Stop`, `GenerateToken`, `ControllerFacade`.
- `core/debugapi/server_test.go`: unit-тесты.
- `core/debugapi_wiring.go`: `debugAPIFacade`, адаптер `*AppController → ControllerFacade`. Методы `StartDebugAPI / StopDebugAPI / DebugAPIAddr`.
- `internal/locale/settings.go`: 3 новых поля.
- `main.go`: auto-start при enabled на старте.
- `ui/diagnostics_tab.go`: `buildDebugAPIRow` с чекбоксом + Copy token + статусом.
- 8 новых ключей локализации (en + ru).

### `154a1e7` — constant-time compare

- `authMiddleware` переведён с `==` на `crypto/subtle.ConstantTimeCompare`.
- Разделение: prefix-check (без секрета) + constant-time остаток.

### `ba15424` — `/version` + Copy toast

- `GET /version` (auth): `{launcher, singbox, api: "debugapi/v1"}`.
- `ControllerFacade.GetLauncherVersion()` (из `constants.AppVersion`).
- Toast `dialogs.ShowAutoHideInfo` после успешного Copy с инструкцией «Pass as: Authorization: Bearer …».
- 2 новых ключа локализации.

### `264ffaf` — `/action/ping-all` + Cmd/Ctrl+P

- `POST /action/ping-all`: триггерит `UIService.AutoPingAfterConnectFunc` (тот же hook, что auto-after-connect и power-resume).
- `ControllerFacade.PingAllProxies()` — делегирует через hook; возвращает nil даже если hook не зарегистрирован.
- `app.registerShortcuts()`: `desktop.CustomShortcut{KeyName: fyne.KeyP, Modifier: fyne.KeyModifierShortcutDefault}`.

## Что не сделано (TODO)

- Integration-тесты startup/shutdown.
- Тесты rotate token + двойной Start.
- `docs/ARCHITECTURE.md` раздел про debug-API.
- README — секция Usage с curl-примерами.
- `docs/api/debug-api-reference.md` — полный reference.

## Известные ограничения

- **Токен не ротируется автоматически.** Пользователь должен вручную удалить поле в `settings.json` для регенерации. Не страшно, но неочевидно.
- **`Stop()` не вызывается в `main.go` на shutdown.** Полагается на OS TCP cleanup. Надо добавить `defer`.
- **Нет rate-limit** на `/action/update-subs` — теоретически можно задолбать провайдера подписок.

## Use-case: MCP-обёртка для AI-агентов

Основное обоснование фичи — внешний MCP-сервер будет оборачивать эти endpoints:

| MCP tool              | Endpoint                      | Статус           |
|-----------------------|-------------------------------|------------------|
| `vpn_status`          | GET /state + GET /version     | ✅ доступен       |
| `vpn_start`           | POST /action/start            | ✅ доступен       |
| `vpn_stop`            | POST /action/stop             | ✅ доступен       |
| `update_subscriptions`| POST /action/update-subs      | ✅ доступен       |
| `ping_all`            | POST /action/ping-all         | ✅ доступен       |
| `list_proxies`        | GET /proxies                  | ✅ доступен       |
| `switch_proxy`        | POST /action/switch-proxy (!) | ⏳ TODO (§6.1)   |
| `list_groups`         | GET /groups (!)               | ⏳ TODO (§6.3)   |
| `get_logs`            | GET /logs?tail (!)            | ⏳ TODO (§6.2)   |

MCP-сервер — отдельным репо, не в этой спеке.
