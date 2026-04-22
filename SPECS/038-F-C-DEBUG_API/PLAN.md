# План: 038 — DEBUG_API

**Статус:** реализовано (retrospective plan). Подробности — IMPLEMENTATION_REPORT.md.

## 1. Пакет

- Новый `core/debugapi/` — не в `core/`, чтобы быть независимым и тестируемым изолированно.
- `ControllerFacade` — узкий интерфейс, не ссылается на `AppController` напрямую (нет cycle).
- Adapter `debugAPIFacade` живёт в `core/debugapi_wiring.go` (package `core`).

## 2. HTTP-пайплайн

- `http.ServeMux` для public endpoints (`/ping`).
- Отдельный `http.ServeMux` для protected; оборачивается `authMiddleware`.
- `authMiddleware`:
  1. Prefix-check `"Bearer "` — `strings.HasPrefix`.
  2. Extract remainder, `strings.TrimSpace`.
  3. `crypto/subtle.ConstantTimeCompare([]byte(got), []byte(s.token))`.
  4. ≠ 1 → 401 JSON `{"error":"unauthorized"}`.
- Action endpoints — `if r.Method != http.MethodPost` → 405 JSON.

## 3. Токен

- `GenerateToken()`: `rand.Read(32-byte buffer)` + `base64.RawURLEncoding.EncodeToString`.
- Длина 43 символа в base64. Entropy ≥ 128-bit.
- Unit-тест: два вызова != равны, длина ≥ 32.

## 4. UI

- `buildDebugAPIRow(ac)` — отдельная функция в `diagnostics_tab.go`, возвращает `fyne.CanvasObject`.
- Хинт обязательно `TextWrapWord`, иначе растягивает окно (было в review, commit `ea63381`).
- Copy-token через `ac.UIService.MainWindow.Clipboard().SetContent` + toast `ShowAutoHideInfo`.

## 5. Settings

- `Settings.DebugAPIEnabled bool json:"debug_api_enabled,omitempty"`
- `Settings.DebugAPIToken string json:"debug_api_token,omitempty"`
- `Settings.DebugAPIPort int json:"debug_api_port,omitempty"` (0 = default 9269)

## 6. Startup

- `main.go`: после `LoadSettings`, если `DebugAPIEnabled && DebugAPIToken != ""` — `controller.StartDebugAPI(port, token)`.
- Graceful stop — при shutdown процесса; явного `Stop()` в `main.go` не нужно (OS TCP cleanup).
  - TODO на follow-up: явный `defer controller.StopDebugAPI()` в main — уточнить.

## 7. Тесты

- `TestServerAuthAndState` — 5 кейсов (см. SPEC §8.1).
- `TestGenerateTokenUnique` — уникальность + длина.
- TODO: integration-тест (startup/shutdown).

## 8. Документация

- `docs/release_notes/upcoming.md` — запись в EN+RU (уже сделано в commit `0db7051`).
- TODO: `docs/ARCHITECTURE.md` — описать debug-API слой.
- TODO: README — пример curl-запросов в секцию «Usage».
