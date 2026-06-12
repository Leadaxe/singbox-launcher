# TASKS 078 — API self-describe + connection card

## Backend (core/debugapi)
- [x] `manifest.go`: `APIDisplayName`/`APISpec`/`APIHint` consts, `DocsURL(launcherVer)` (release tag → blob/<tag>, else main)
- [x] `apiEndpoint` type + `s.endpoints()` registry; `routes()` registers from it
- [x] `GET /` manifest handler (api/spec/launcher/core/auth/docs/hint/endpoints)
- [x] `GET /help` handler (endpoints: method/path/summary/auth)
- [x] `/` and `/help` behind auth; `/ping` stays open
- [x] Tests: manifest/help shape, every registry path registered, DocsURL release vs dev, 401 without token

## UI (settings)
- [x] Кнопка «Copy API info for agent» в секции Debug API: JSON (base_url/token/versions/auth/docs/hint), в буфер + read-only показ; disabled при выключенном API/пустом токене
- [x] Локали en + ru (button label, copied toast)

## Docs
- [x] `docs/API.md`: разделы `GET /` и `GET /help`
- [x] `docs/release_notes/upcoming.md`: EN/RU
- [x] `IMPLEMENTATION_REPORT.md`
- [x] `go build ./... && go test ./... && go vet ./...`
