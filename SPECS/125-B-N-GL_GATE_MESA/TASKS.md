# TASKS 125

Порядок обязателен: состояние → проба → гейт → main → UI → логи → доки → тесты.

## Этап 1 · Состояние и файлы Mesa (без build-тегов)
- [x] `internal/constants/constants.go`: `GLStateFileName`, `NativeStderrLogFileName`, `MesaDisabledSuffix`
- [x] `internal/platform/glstate.go`: `GLState`, `GLStatePath`, `LoadGLState`, `SaveGLState` (tmp+rename), `MarkGLStarting`, `MarkGLRendered` — PLAN §2
- [x] там же: `IsMesaInstalled`, `IsMesaDisabled`, `HasMesaBundle`, `DisableMesa`, `EnableMesa`, `copyMesaFromBundle`, `errForeignOpenGL`
- [x] там же: `probeResult`, `gateInput`, `gateAction`, `decideGate` — PLAN §3.2 (типы без Win32-зависимостей)
- [x] `internal/platform/glprobe_notify.go`: `SetOnHardwareGLAvailable`, `notifyHardwareGLAvailable` с буфером до регистрации
- [x] `go build ./...` на darwin проходит

## Этап 2 · Проба (Windows)
- [x] `probeDesktopOpenGL(local bool)`: при `local` — `LoadLibraryEx(execDir\opengl32.dll, LOAD_WITH_ALTERED_SEARCH_PATH)` и `NewProc` от хендла
- [x] `RunGLProbeChild(local bool)` (+ stub)
- [x] `probeGLViaSubprocess(local bool) probeResult` с полем `Timeout`
- [x] `main.go`: флаг `-gl-probe-local`, диспетчер `RunGLProbeChild(*glProbeLocal)`

## Этап 3 · Гейт (Windows)
- [x] `EnsureDesktopOpenGL(execDir string, interactive bool)` (+ stub) = `Load` → `decideGate` → действия → `MarkGLStarting`
- [x] Диалоги D1–D5 английские, константами; старые двуязычные тексты удалить
- [x] D2: цикл Retry; Ignore → старт как есть; Abort → ветка отказа
- [x] `installMesaFromBundle` без preload; установка по PLAN §3.3: `Setenv GALLIUM_DRIVER=llvmpipe` (если не задан) → `-gl-probe-local` → проверка `llvmpipe` в renderer → preload | откат + D4
- [x] режим `rendered/mesa`: WARN из SPEC §2.7 + фоновая проба железа → `notifyHardwareGLAvailable`
- [x] `-tray` (`interactive=false`): ни одного `messageBox`; все ветки — WARN
- [x] `GOOS=windows GOARCH=amd64 go build ./internal/platform/` и `go vet` проходят

## Этап 4 · stderr и main.go
- [x] `internal/debuglog/stderr_windows.go` + `stderr_other.go`: `RedirectNativeStderr(path)` — `MkdirAll`, `OpenFile` append, `SetStdHandle`, `os.Stderr = f`, хендл в package var
- [x] `main.go:59`: вызов после `EnableCrashOutput`
- [x] `main.go:75`: первая WARN-строка с версией, GOOS/GOARCH, execDir
- [x] `main.go:83`: `EnsureDesktopOpenGL(execDir, !*startInTray)`
- [x] `SetOnStarted`: WARN «ui: event loop started»; `time.AfterFunc(3s, MarkGLRendered)` с комментарием-обоснованием
- [x] `SetOnStarted`: `SetOnHardwareGLAvailable` → `fyne.Do` → `dialog.NewConfirm` (Yes/Later) по SPEC §2.5

## Этап 5 · Диагностика
- [x] `ui/diagnostics_tab.go`: `buildMesaToggleButton(ac)` — Windows-only, подпись по состоянию, подтверждение, действие, «Restart the launcher to apply.», перестроение подписи
- [x] `bin/locale/ru.json`: пять ключей из PLAN §5

## Этап 6 · Логи
- [x] Info→WARN: `main.go:173`, `:286`, `:453`; `core/process_service.go:213`, `:268`, `:441`

## Этап 7 · Документация
- [x] `docs/RDP_OPENGL.md` и `.ru.md` — PLAN §7
- [x] `docs/release_notes/upcoming.md` — EN + RU, Fixes
- [x] `CHANGELOG.md` — «Не выпущено» → Исправления, `(#125)`

## Этап 8 · Тесты и прогон (один раз, в конце)
- [x] `internal/platform/glstate_test.go` — четыре группы из PLAN §6
- [x] `go build ./... && go vet ./... && go test ./...` — зелёные
- [x] `GOOS=windows GOARCH=amd64 go build ./internal/platform/ ./internal/debuglog/` и `go vet ./internal/platform/` — зелёные
- [x] `IMPLEMENTATION_REPORT.md`: что сделано, отклонения от PLAN с причинами, список изменённых файлов
