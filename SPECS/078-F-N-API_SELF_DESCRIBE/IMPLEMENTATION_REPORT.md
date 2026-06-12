# IMPLEMENTATION_REPORT 078 — API self-describe + connection card

**Статус:** реализовано, тесты зелёные. Дата: 2026-06-12.

## Что сделано

1. **Единый реестр эндпоинтов** — [`core/debugapi/server.go`](../../core/debugapi/server.go): `s.endpoints()` возвращает `[]apiEndpoint{Method, Path, Auth, Summary, handler}`; `routes()` регистрирует из него (auth → protected, no-auth → mux). Один источник правды для роутинга, `GET /` и `GET /help`.
2. **`GET /` манифест** — `handleManifest`: `api`, `spec` (`debugapi/v1`), `launcher`, `core`, `auth`, `docs` (через `DocsURL`), `hint`, `endpoints[]`. Неизвестный путь (ServeMux catch-all на `/`) → `404` с `docs`-указателем.
3. **`GET /help`** — `handleHelp`: только `endpoints[]` (method/path/summary/auth).
4. **`DocsURL(launcherVer)`** + константы (`APIDisplayName`/`APISpec`/`APIAuthScheme`/`APIHint`) — [`core/debugapi/manifest.go`](../../core/debugapi/manifest.go): релизный тег `vX.Y.Z` → `blob/<tag>`, dev → `main`.
5. **Connection card** — `ConnectionCardJSON(baseURL, token, launcherVer, coreVer)` (manifest.go) + кнопка **«Copy API info»** в [`ui/settings_tab.go`](../../ui/settings_tab.go): кладёт JSON (base_url/token/versions/auth/docs/hint) в буфер, показывает toast; enable/disable синхронно с toggle API. Локали en + ru.
6. **Auth:** `/` и `/help` за Bearer; `/ping` открыт.
7. Документация: [`docs/API.md`](../../docs/API.md) (разделы `GET /`, `GET /help`, connection card), `docs/release_notes/upcoming.md`.

## Проверки

* `go build ./...` — OK; `go vet ./...` — OK.
* Тесты: [`core/debugapi/manifest_test.go`](../../core/debugapi/manifest_test.go) — `DocsURL` (release/dev), манифест/help shape, **single-source-of-truth** (каждый заявленный эндпоинт реально отвечает не-404), auth 401 на `/` и `/help`, 404+docs на неизвестный путь. Locale-паритет en↔ru — `internal/locale` тест зелёный.

## Дизайн-решения (как в SPEC)

* Реестр — один источник; дока не расходится с роутингом by construction.
* Токен только в connection card (для bootstrap агента), не в `GET /` (там агент уже авторизован).
* Versioned docs-ссылка → агент читает доку своей сборки.
* OpenAPI/Swagger отклонён (overkill, зависимости).
