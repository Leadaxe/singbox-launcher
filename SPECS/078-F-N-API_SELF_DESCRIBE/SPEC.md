# SPEC 078-F-N — САМООПИСАНИЕ DEBUG-API + CONNECTION CARD

## Цель

Сделать debug-API самоописываемым, чтобы пользователю было достаточно дать агенту **адрес + токен** — дальше агент разбирается сам:

1. **`GET /`** (за авторизацией) — JSON-манифест API: имя, spec-версия, версии лаунчера/ядра, формат auth, ссылка на полную документацию (привязанную к версии сборки) и список эндпоинтов.
2. **`GET /help`** (за авторизацией) — только список эндпоинтов (method/path/summary/auth). «Дальше агент сам».
3. **Кнопка в настройках** (секция Debug API) — копирует/показывает **connection card** JSON со всем, что нужно агенту для подключения: `base_url` (адрес+порт), реальный `token`, версии, `auth`, `docs`, `hint`. Пользователь жмёт → отдаёт JSON агенту → агент подключается.

## Контекст (как сейчас)

- Маршруты регистрируются вручную, ~25 строк `protected.HandleFunc(...)` в `routes()` (`core/debugapi/server.go:166-209`). `docs/API.md` ведётся отдельно руками → рассинхрон (в этой сессии версии в нём правились вручную).
- Версии доступны: `facade.GetLauncherVersion()` / `facade.GetSingboxVersion()` (см. `handleVersion`, server.go:234). Порт: `DefaultPort=9263`, адрес: `Server.Addr()` → `127.0.0.1:N`. Хелпер ответа: `writeJSON(w, status, v)`.
- Настройки Debug API (toggle/token/port) — `ui/settings_tab.go` (`SubscriptionUserAgent` рядом; токен `Settings.DebugAPIToken`, порт `Settings.DebugAPIPort`).

## Объём

**В объёме:**

1. **Единый реестр эндпоинтов** (`core/debugapi`): `apiEndpoint{Method, Path, Auth, Summary, handler}` + `s.endpoints() []apiEndpoint`. `routes()` регистрирует из него (один источник правды → `/`, `/help` и роутинг не расходятся).
2. **`GET /`** — манифест: `api`, `spec` (`debugapi/v1`), `launcher`, `core`, `auth`, `docs`, `hint`, `endpoints[]` (method/path/summary). За auth. Поле `docs` строит хелпер `DocsURL(launcherVer)` — ссылка на `docs/API.md`, привязанная к версии: релизный тег `vX.Y.Z` → `blob/vX.Y.Z/...`; dev/непил-версия → `blob/main/...`.
3. **`GET /help`** — `endpoints[]` (method/path/summary/auth). За auth.
4. **Connection card в настройках**: кнопка «Copy API info for agent» (рядом с toggle/token). Строит JSON (`base_url`, `launcher`, `core`, `auth`, `token`, `docs`, `hint`, `api`, `spec`), кладёт в буфер и показывает в read-only поле. Активна только когда API включён и токен есть. `docs` — тот же `DocsURL`.
5. Обновить `docs/API.md` (описать `GET /` и `GET /help`), `docs/release_notes/upcoming.md`. Тесты.

**Вне объёма:**

- OpenAPI/Swagger (overkill для локального debug-API, тянет зависимости — отклонено в обсуждении).
- Реальный токен в манифесте `GET /` — там агент уже подключён, токен не нужен; токен есть только в connection card (осознанное действие пользователя «дать агенту»).
- Параметры/тела запросов в описании эндпоинтов — только method/path/summary (агент дочитывает детали в `docs/API.md` по ссылке).

## Дизайн-решения

- **Один реестр.** `endpoints()` — источник для роутинга и для `/` + `/help`; документация не может разойтись с кодом by construction.
- **Versioned docs-ссылка.** Агент читает доку именно своей сборки. Dev-сборки → `main` (там нет стабильного тега).
- **Токен только в connection card.** `GET /` за auth не светит токен (агент его уже имеет). Card — для bootstrap: пользователь сам решает кому отдать локальный токен.
- **`/ping` остаётся открытым** (liveness), всё прочее — за auth, включая `/` и `/help`.

## Критерии приёмки

1. `curl -H "Authorization: Bearer <t>" http://127.0.0.1:9263/` → манифест с `endpoints[]`, `docs` на `blob/<version>/docs/API.md`, `core`/`launcher` актуальные.
2. `GET /help` → список эндпоинтов; в нём присутствуют реально зарегистрированные пути (тест-страж: каждый `endpoints()` путь зарегистрирован).
3. Без токена `GET /` и `GET /help` → 401; `GET /ping` → 200.
4. Кнопка в настройках даёт валидный JSON с `base_url=http://127.0.0.1:<port>`, реальным `token`, versioned `docs`; неактивна при выключенном API.
5. `go build ./... && go test ./... && go vet ./...` зелёные.
