# PLAN 125 · Реализация

Принцип разбиения: всё, что не трогает Win32, — в файлах **без build-тегов**,
чтобы собиралось и тестировалось на darwin. Win32 — только проба, `MessageBoxW`,
`SetStdHandle`, preload DLL.

## 1. Файлы

### Новые

| Файл | Содержимое |
|---|---|
| `internal/platform/glstate.go` | `GLState` + `LoadGLState/SaveGLState/MarkGLStarting/MarkGLRendered`; `IsMesaInstalled/IsMesaDisabled/HasMesaBundle/DisableMesa/EnableMesa`; чистая функция решения `decideGate` (см. §3). Без build-тегов |
| `internal/platform/glstate_test.go` | тесты §5 |
| `internal/platform/glprobe_notify.go` | канал «железо доступно»: `SetOnHardwareGLAvailable(func(renderer string))` + вызов из фоновой пробы. Без тегов (на не-Windows никогда не срабатывает) |
| `internal/debuglog/stderr_windows.go` | `RedirectNativeStderr(path string) error` |
| `internal/debuglog/stderr_other.go` | `//go:build !windows` no-op |

### Изменяемые

| Файл | Что |
|---|---|
| `internal/platform/glprobe_windows.go` | гейт по SPEC §2.2; `probeDesktopOpenGL(local bool)`; `RunGLProbeChild(local bool)`; типизированный таймаут; тексты D1–D5; удалить старые двуязычные константы; `installMesaFromBundle` без preload (preload отдельно после проверки); пин `GALLIUM_DRIVER` |
| `internal/platform/glprobe_stub.go` | сигнатуры `EnsureDesktopOpenGL(execDir string, interactive bool)`, `RunGLProbeChild(local bool)` |
| `internal/constants/constants.go` | `GLStateFileName = "gl-state.json"`, `NativeStderrLogFileName = "native-stderr.log"`, `MesaDisabledSuffix = ".off"` |
| `main.go` | флаг `-gl-probe-local`; `RedirectNativeStderr` после `EnableCrashOutput`; строка версии WARN; `EnsureDesktopOpenGL(execDir, !*startInTray)`; в `SetOnStarted` — WARN «event loop started» + отложенный `MarkGLRendered`; подписка `SetOnHardwareGLAvailable` → Fyne-диалог возврата; поднять Info→WARN по SPEC §2.7 |
| `core/process_service.go` | три строки Info→WARN (`:213`, `:268`, `:441`) |
| `ui/diagnostics_tab.go` | кнопка Mesa (§4) |
| `bin/locale/ru.json` | ключи кнопки и диалога возврата |
| `docs/RDP_OPENGL.md`, `docs/RDP_OPENGL.ru.md` | новое поведение: диалоги, `gl-state.json`, кнопка, `.off`, `native-stderr.log`, `GALLIUM_DRIVER` |
| `docs/release_notes/upcoming.md` | EN + RU, раздел Fixes |
| `CHANGELOG.md` | «Не выпущено» → Исправления |

## 2. Состояние (`glstate.go`)

```go
const (
    GLPhaseStarting = "starting"
    GLPhaseRendered = "rendered"
    GLModeHardware  = "hardware"
    GLModeMesa      = "mesa"
)

type GLState struct {
    Phase             string    `json:"phase"`
    Mode              string    `json:"mode"`
    Driver            string    `json:"driver,omitempty"`
    Renderer          string    `json:"renderer,omitempty"`
    LauncherVersion   string    `json:"launcher_version"`
    UpdatedAt         time.Time `json:"updated_at"`
    OfferedHWRenderer string    `json:"offered_hw_renderer,omitempty"`
}

func GLStatePath(execDir string) string            // GetBinDir + GLStateFileName
func LoadGLState(execDir string) (GLState, bool)    // ok=false: нет файла или битый JSON (WARN)
func SaveGLState(execDir string, s GLState) error   // tmp + rename, как locale.SaveSettings
func MarkGLStarting(execDir, mode string)           // Phase=starting, Mode, LauncherVersion, UpdatedAt; Renderer/Driver/Offered — сохранить из прежнего
func MarkGLRendered(execDir string)                 // Phase=rendered, UpdatedAt; остальное сохранить
```

Ловушка: `MarkGLStarting` **не должна** терять `OfferedHWRenderer` и `Renderer`
прежней записи — иначе диалог возврата будет спрашивать каждый старт.

Файлы Mesa:

```go
var mesaDLLs = []string{"opengl32.dll", "libgallium_wgl.dll", "dxil.dll"}

func IsMesaInstalled(execDir) bool   // opengl32.dll && libgallium_wgl.dll рядом с exe
func IsMesaDisabled(execDir) bool    // opengl32.dll.off рядом с exe
func HasMesaBundle(execDir) bool     // mesa3d/opengl32.dll
func DisableMesa(execDir) error      // rename каждой присутствующей DLL → +".off"; если .off уже есть — удалить старый .off перед rename
func EnableMesa(execDir) error       // .off → без суффикса; если .off нет — copyMesaFromBundle
func copyMesaFromBundle(execDir) ([]string, error) // тело нынешнего installMesaFromBundle без preload
```

Ловушка: `DisableMesa` при отсутствии `libgallium_wgl.dll` (чужой одиночный
`opengl32.dll`) — **ничего не делать**, вернуть ошибку `errForeignOpenGL`;
гейт пишет WARN и считает `mode=hardware`.

## 3. Гейт (`glprobe_windows.go`)

### 3.1 Проба

```go
type probeResult struct {
    Major, Minor int
    Renderer     string
    Err          error   // nil = ответила
    Timeout      bool    // true = не ответила за glProbeTimeout
}
func probeGLViaSubprocess(local bool) probeResult
func (r probeResult) ok() bool  // Err==nil && !Timeout && версия ≥ 2.1
```

Подпроцесс: `exe -gl-probe` или `exe -gl-probe-local`. В `probeDesktopOpenGL(local)`:
`local=true` → `windows.LoadLibraryEx(filepath.Join(execDir,"opengl32.dll"), 0, LOAD_WITH_ALTERED_SEARCH_PATH)`
и дальше `NewProc` от этого хендла (не `NewLazySystemDLL` — та ищет только system32).
`execDir` в дочернем процессе — `filepath.Dir(os.Executable())`.

Ловушка: сейчас `RunGLProbeChild` вызывается из `main.go:67` **до**
`NewAppController`, логов ещё нет — так и оставить, `debuglog` в дочернем
процессе не трогать.

### 3.2 Решение — чистая функция (в `glstate.go`, без тегов, тестируется)

```go
type gateInput struct {
    State         GLState; HasState bool
    MesaInstalled bool
    Interactive   bool
    Probe         probeResult   // заполняется только когда decideGate вернул needProbe
}
type gateAction int
const (
    actStart gateAction = iota      // MarkStarting(mode) и старт
    actProbe                        // нужна проба железа, потом decideGate ещё раз с Probe
    actAskDisableMesa               // D1
    actAskTimeout                   // D2
    actAskInstallMesa               // D3
    actNeitherWorks                 // D5 (или WARN в non-interactive)
)
func decideGate(in gateInput) gateAction
```

`EnsureDesktopOpenGL` — тонкая обёртка: `Load` → `decideGate` → проба/диалоги →
действия → `MarkGLStarting`. Все `MessageBoxW` — только при `interactive`.
Retry в D2 — цикл `for` без ограничения (каждая итерация — клик пользователя).

### 3.3 Установка

```
installMesa*(execDir)             // copy из mesa3d/ или download — как сейчас, БЕЗ preload
if os.Getenv("GALLIUM_DRIVER") == "" { os.Setenv("GALLIUM_DRIVER", "llvmpipe") }
r := probeGLViaSubprocess(true)
if r.ok() && strings.Contains(strings.ToLower(r.Renderer), "llvmpipe") → preloadMesa; mode=mesa; WARN «gl: Mesa3D installed and verified (renderer=…)»
else → DisableMesa (или удалить только что скопированные); D4; mode=hardware
```

Ловушка: подпроцесс пробы наследует окружение — `Setenv` должен быть **до**
запуска `-gl-probe-local`, иначе проверяется не тот драйвер.

Ловушка: существующая проверка `getModuleHandleW("opengl32.dll") != 0` в
`preloadMesa` (`:460-466`) остаётся — но текст `mesaRestartText` → английский.

### 3.4 Фоновая проба в режиме mesa

```go
go func() {
    r := probeGLViaSubprocess(false)
    if r.ok() && r.Renderer != state.OfferedHWRenderer { notifyHardwareGLAvailable(r.Renderer) }
}()
```

`glprobe_notify.go`: `SetOnHardwareGLAvailable(fn)`; если `fn` ещё не задана к
моменту результата — результат запомнить и отдать при регистрации (гонка со
стартом UI).

## 4. main.go

Порядок вставок (номера строк — текущие):

1. `:59` после `EnableCrashOutput`: `debuglog.RedirectNativeStderr(filepath.Join(filepath.Dir(ex), "logs", constants.NativeStderrLogFileName))`; ошибку — `WarnLog`. `logs/` может ещё не существовать — `MkdirAll` внутри `RedirectNativeStderr`.
2. `:67` `if *glProbe || *glProbeLocal { platform.RunGLProbeChild(*glProbeLocal) }`.
3. `:75` после успешного `NewAppController`: **первая WARN-строка**
   `debuglog.WarnLog("launcher %s %s/%s started, exec=%s", constants.AppVersion, runtime.GOOS, runtime.GOARCH, controller.FileService.ExecDir)`.
4. `:83` `platform.EnsureDesktopOpenGL(controller.FileService.ExecDir, !*startInTray)`.
5. `SetOnStarted` (`:271`): первой строкой `debuglog.WarnLog("ui: event loop started")`;
   затем `time.AfterFunc(glRenderedGrace, func(){ platform.MarkGLRendered(execDir) })`,
   `glRenderedGrace = 3 * time.Second`. Обоснование в комментарии: `OnStarted`
   срабатывает до первого кадра; падение Mesa приходило через 1–2 с после
   появления окна; 3 с без смерти процесса = кадр отрисован.
6. Там же: `platform.SetOnHardwareGLAvailable(func(renderer string){ fyne.Do(func(){ …dialog.NewConfirm… }) })`.
   Yes → `platform.DisableMesa(execDir)`, `SaveGLState` с `OfferedHWRenderer`, `dialogs.ShowInfo` «Restart…».
   Later → только `OfferedHWRenderer`.
7. Info→WARN: `:173`, `:286`, `:453`.

Ловушка: `runtime` уже импортирован в `main.go` (используется для darwin-веток).

## 5. Диагностика (`ui/diagnostics_tab.go`)

После `cleanRuleSetsButton` (`:353`), только `runtime.GOOS == "windows"`:

```go
mesaBtn := buildMesaToggleButton(ac)   // nil → не добавлять в VBox
```

Подпись и действие по таблице SPEC §2.6. Подтверждение — `dialog.NewConfirm`.
После действия — `dialogs.ShowInfo(..., locale.T("Restart the launcher to apply."))`.
Кнопку после действия перестроить (подпись меняется) — проще всего
`btn.SetText(...)` по новому состоянию.

Ключи `locale.T` (natural keys, SPEC 111) → добавить в `bin/locale/ru.json`:
- `Disable Mesa3D (use hardware OpenGL)`
- `Enable Mesa3D (software rendering)`
- `Restart the launcher to apply.`
- `Hardware OpenGL is now available:\n%s\nDisable Mesa3D and use it? The change applies after you restart the launcher.`
- `Later`

Ловушка: не писать тесты на подписи/вёрстку (память «Без тестов на UI-формат»).

## 6. Тесты (`glstate_test.go`, в конце)

Только критичные, платформо-независимые:

1. `Load/Save` round-trip; битый JSON → `ok=false`.
2. `MarkGLStarting` сохраняет `OfferedHWRenderer` и `Renderer` прежней записи.
3. `decideGate`: таблица случаев из SPEC §2.2 — нет файла; `rendered/hardware`;
   `rendered/mesa`; `starting` + проба OK + Mesa есть/нет; таймаут interactive /
   non-interactive; отказ + Mesa есть/нет; `interactive=false` никогда не
   возвращает `actAsk*`.
4. `DisableMesa/EnableMesa` в `t.TempDir()`: три DLL → `.off` → обратно;
   одиночный `opengl32.dll` без `libgallium_wgl.dll` → `errForeignOpenGL`, файл
   не тронут; `EnableMesa` без `.off` копирует из `mesa3d/`.

Полный прогон один раз в конце: `go build ./... && go vet ./... && go test ./...`
плюс `GOOS=windows GOARCH=amd64 go build ./internal/platform/ ./internal/debuglog/`
и `GOOS=windows GOARCH=amd64 go vet ./internal/platform/`.

## 7. Документация

- `docs/RDP_OPENGL.md` / `.ru.md`: заменить описание «автоустановка из mesa3d/
  без вопроса» на диалоги D1–D5; описать `bin/gl-state.json` (и что его можно
  удалить для сброса), кнопку в Diagnostics, ручной откат через `.off`,
  `logs/native-stderr.log`, `GALLIUM_DRIVER` (лаунчер пинит llvmpipe, если
  переменная не задана).
- `upcoming.md` EN + RU, раздел Fixes: одна запись, из чего сложилась ловушка и
  что теперь.
- `CHANGELOG.md`: «Не выпущено» → Исправления, одна строка с `(#125)`.

## 8. Добавка по SPEC §6 (после RC 13.09.2026)

| Файл | Что |
|---|---|
| `internal/platform/glstate.go` | `GLPhaseRestart`; `decideGate`: `Phase ∈ {rendered, restart}` → `actStart`; `probeResult.Vendor`, `SawMesa`; `ok()` учитывает `SawMesa`; `describe()` для него |
| `internal/platform/glprobe_windows.go` | `pinMesaDriver()` в начале гейта (есть); `probeHardware(execDir)` с временным `.probe`-переименованием `opengl32.dll` и разбором `vendor=`; удалить `preloadMesa`/`mesaRestartText`; после D1-Yes и D3-Yes+verify — `UpdateGLState(restart)` → D6 → `RestartSelf()` → `os.Exit(0)`; WARN §2.7 по R4 |
| `internal/platform/restart_windows.go` + `restart_other.go` | `RestartSelf() error` |
| `ui/diagnostics_tab.go` | подтверждение с «will restart now» → `UpdateGLState(restart)` → `RestartSelf()`; при ошибке — прежнее «Restart the launcher to apply.» |
| `bin/locale/ru.json` | обновлённые тексты кнопки |
| `glstate_test.go` | два кейса из §6.3 п.14 |
| `docs/RDP_OPENGL*.md`, `CHANGELOG.md`, `upcoming.md` | F1 (импорт, перезапуск), R3 |

Ловушки:
- Переименование `opengl32.dll` для пробы — только когда `IsMesaInstalled`;
  `defer` обязан вернуть имя даже если `probeGLViaSubprocess` запаниковала.
  Между rename и restore не вызывать ничего, что могло бы `LoadLibrary`.
- В фоновой пробе (§2.5, UI работает под Mesa) то же переименование; Mesa
  уже отображена, `libgallium_wgl.dll`/`dxil.dll` не трогать.
- `RestartSelf`: `PrepareCommand` (HideWindow) — но окно новому процессу
  нужно; для Windows использовать `SysProcAttr{CreationFlags:
  windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}` и НЕ
  `HideWindow`. `cmd.Dir = execDir`.
- После `os.Exit(0)` из гейта лог-файлы уже открыты — `debuglog` перед выходом
  написать WARN «gl: restarting to apply <mode>».
- `MarkGLRendered` в новом процессе перепишет `restart` → `rendered` штатно.
