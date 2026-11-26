# TASKS 190

- [x] `/impair/{outbound}`, `…/set`, `…/schedule` — `core/debugapi/impair_endpoints.go`.
- [x] `PATCH /state/servers` (и у удалённой машины) — `stateedit.SetServerDetour`, `core/debugapi/state_crud_endpoints.go`.
- [x] `POST /proxies/delay`, `GET /pool`, `GET /outbounds/{node}/config` — `core/debugapi/daemon_observe_endpoints.go`.
- [x] Тесты `TestImpairEndpointsPassThrough`, `TestSetServerDetourImpairScenario`; `docs/API.md`, `docs/API.ru.md`, release notes.
