// Package snapshot собирает срез из четырёх файлов wizard-pipeline'а
// (template, state, cache, config) в одну типизированную структуру.
//
// Чистая read-only функция, не зависит от HTTP / UI / event-bus. Используется:
//   - core/debugapi (handler GET /debug/snapshot) — JSON-сериализация наружу;
//   - UI Diagnostics tab (кнопка «Copy snapshot») — JSON в clipboard;
//   - golden-data capture для SPEC 045 фазы 5.3 (strangler-fig порт BuildConfig).
//
// См. SPECS/038-F-C-DEBUG_API/SUB_SPEC_SNAPSHOT.md для контракта и acceptance.
//
// Никакой редакции секретов: вызывающий слой (debug-API через bearer-токен,
// UI в локальной сессии) — сам и есть trust-boundary. Если шарить публично —
// чистить руками; UI показывает warning «contains your secrets».
package snapshot

import (
	"encoding/json"
	"os"
	"time"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/internal/platform"
)

// Snapshot — данные одного захвата pipeline'а.
//
// JSON-теги совпадают с SUB_SPEC_SNAPSHOT.md §1.1; debug-API сериализует
// эту структуру в response как есть, UI — тем же кодом для clipboard.
//
// Files: содержимое каждого файла как inline-JSON. json.RawMessage
// сериализуется без двойного кодирования (объект, не строка).
type Snapshot struct {
	CapturedAt      string                     `json:"captured_at"`
	LauncherVersion string                     `json:"launcher_version"`
	SingboxVersion  string                     `json:"singbox_version"`
	Files           map[string]json.RawMessage `json:"files,omitempty"`
	Missing         []string                   `json:"missing,omitempty"`
	Errors          map[string]string          `json:"errors,omitempty"`
}

// fileSpec — описание одной из четырёх читаемых записей.
// name — стабильный ключ в Files (template/state/cache/config);
// path — канонический путь через internal/platform helpers (политика v0.8.8.2).
type fileSpec struct {
	name string
	path string
}

// Build собирает текущий Snapshot для exec-dir.
//
// Поведение по каждому файлу:
//   - не существует на диске       → name в Missing;
//   - существует, валидный JSON    → name в Files как json.RawMessage;
//   - существует, невалидный JSON  → name в Errors с понятным сообщением;
//   - прочая I/O ошибка            → name в Errors с err.Error().
//
// Никаких ошибок наружу — частичная полнота снапшота кодируется через
// Missing/Errors, чтобы вызывающий мог различать сценарии без try/catch.
func Build(execDir, launcherVersion, singboxVersion string) Snapshot {
	files := []fileSpec{
		{name: "template", path: platform.GetWizardTemplatePath(execDir)},
		{name: "state", path: platform.GetWizardStatePath(execDir)},
		{name: "cache", path: platform.GetOutboundsCachePath(execDir)},
		{name: "config", path: platform.GetConfigPath(execDir)},
	}

	out := Snapshot{
		CapturedAt:      time.Now().UTC().Format(time.RFC3339),
		LauncherVersion: launcherVersion,
		SingboxVersion:  singboxVersion,
		Files:           map[string]json.RawMessage{},
	}

	for _, f := range files {
		raw, err := os.ReadFile(f.path)
		if err != nil {
			if os.IsNotExist(err) {
				out.Missing = append(out.Missing, f.name)
			} else {
				if out.Errors == nil {
					out.Errors = map[string]string{}
				}
				out.Errors[f.name] = err.Error()
			}
			continue
		}
		// config.json — JSONC (с /** ... */ маркерами); strict json.Valid
		// его отбрасывает. Нормализуем через jsonc.ToJSON и валидируем
		// результат. Для остальных файлов (state, template, cache) тоже
		// безопасно: если они уже чистый JSON, ToJSON — no-op.
		canonical := jsonc.ToJSON(raw)
		if !json.Valid(canonical) {
			if out.Errors == nil {
				out.Errors = map[string]string{}
			}
			out.Errors[f.name] = "invalid JSON"
			continue
		}
		out.Files[f.name] = json.RawMessage(canonical)
	}

	if len(out.Files) == 0 {
		out.Files = nil
	}
	return out
}
