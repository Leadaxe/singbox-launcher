# Задачи: 038 — DEBUG_API

## Этап 0 — дизайн

- [x] Утвердить scope (read + controlled actions, без CRUD / config-dump / logs).
- [x] Bearer-auth + constant-time compare.
- [x] Listener строго 127.0.0.1.

## Этап 1 — core

- [x] Пакет `core/debugapi/` с `Server`, `New`, `Start`, `Stop`.
- [x] Endpoints: `/ping`, `/version`, `/state`, `/proxies`, `/action/{start,stop,update-subs,ping-all}`.
- [x] Middleware: `authMiddleware` с `crypto/subtle.ConstantTimeCompare`.
- [x] Method-gate: POST-only на action-эндпоинтах (405 на GET).
- [x] `GenerateToken()`.

## Этап 2 — wiring

- [x] `ControllerFacade` интерфейс в debugapi.
- [x] `debugAPIFacade` в `core/debugapi_wiring.go` адаптирует `AppController`.
- [x] `StartDebugAPI / StopDebugAPI / DebugAPIAddr` на `*AppController`.

## Этап 3 — settings + startup

- [x] Поля `DebugAPIEnabled / Token / Port` в `internal/locale/settings.go`.
- [x] `main.go` — auto-start при `Enabled=true`.

## Этап 4 — UI

- [x] Секция на Diagnostics-табе (`buildDebugAPIRow`).
- [x] Чекбокс + Copy-token + статус.
- [x] `TextWrapWord` на хинте (фикс `ea63381`).
- [x] Toast подтверждения Copy.
- [x] Горячая клавиша `Cmd/Ctrl+P` → ping-all (через hook, не HTTP).

## Этап 5 — локалии

- [x] 8 ключей в `en.json` + `ru.json`.

## Этап 6 — тесты

- [x] `TestServerAuthAndState` — 5 кейсов.
- [x] `TestGenerateTokenUnique`.
- [ ] **TODO:** integration startup/shutdown cycle.
- [ ] **TODO:** rotate token (stop → change settings → start с новым токеном).
- [ ] **TODO:** двойной `StartDebugAPI` — корректный restart.

## Этап 7 — документация

- [x] `docs/release_notes/upcoming.md` (EN+RU).
- [ ] **TODO:** `docs/ARCHITECTURE.md` — описать слой.
- [ ] **TODO:** README — примеры curl в «Usage».
- [ ] **TODO:** `docs/api/debug-api-reference.md` — подробный reference всех endpoints с примерами.

## Этап 8 — расширения (следующие релизы)

- [ ] `POST /action/switch-proxy {group, name}`.
- [ ] `GET /groups` — список selector-групп с активным в каждой.
- [ ] `GET /logs?tail=N` — когда появится in-memory log ring.
- [ ] Rate-limit на `/action/update-subs`.
- [ ] Отдельный репо `singbox-launcher-mcp` — MCP-сервер-обёртка для AI-агентов.
