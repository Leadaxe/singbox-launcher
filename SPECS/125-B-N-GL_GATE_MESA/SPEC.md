# SPEC 125 · Гейт OpenGL: Mesa ставится по таймауту, уходит в d3d12 и убивает окно

Статус: N. Ветка develop. Windows-only логика (`internal/platform/glprobe_windows.go`),
платформо-независимая часть (состояние, переключение файлов) — без build-тегов.

Предыстория: issue #105 (RDP/сервер без GPU — окно Fyne не отрисовывается),
фикс `da28185d`. Полевой репорт 09–10.09.2026: обычный домашний ПК с рабочей
видеокартой, лаунчер v1.5.5 (архив `win64-full`).

## 1. Проблема

### 1.1 Что случилось

08.09 в 12:26 GL-пробник не ответил за 10 секунд:

```
[WARN] gl: insufficient OpenGL (got 0.0, renderer="", err=gl-probe timed out after 10s) — Fyne needs 2.1; offering Mesa3D
[WARN] gl: system opengl32.dll already loaded in this process — restart required to pick up Mesa
```

Гейт истолковал таймаут как «аппаратного OpenGL нет» и скопировал DLL Mesa3D из
`mesa3d/` рядом с exe **без вопроса** (`installMesaFromBundle`). С этого момента
папка рендерится через Mesa, и лаунчер умирает: окно и иконка в трее появляются
на секунду и исчезают.

Ни один канал диагностики не сработал: `crash.txt` через `2>` пуст (паники Go
нет), `Fyne error` в логе нет (`initFailed` не срабатывал), Event Viewer пуст,
лог обрывается на `nla cleanup`, потому что успешный старт не пишет ни одной
строки уровня WARN. Падение — в потоке, созданном DLL.

Причина подтверждена экспериментом: переименование `opengl32.dll` рядом с exe →
лаунчер запускается. Замена всего `bin/` из рабочей копии не помогала — виновник
лежал рядом с exe.

### 1.2 Три дефекта

**A. Таймаут = «нет OpenGL».** `glprobe_windows.go:258-263`: любая ошибка
`err != nil` ведёт к установке Mesa. `probeGLViaSubprocess` (`:310-354`)
возвращает одинаковый `error` для «драйвер отдал 1.1» и «не ответил за 10 с».

**B. Обещаем llvmpipe, получаем d3d12.** `GALLIUM_DRIVER` не выставляется. В
ассете `mesa3d-26.2.0-win64.zip` лежат все драйверы (строки в
`libgallium_wgl.dll`: `d3d12` 57, `zink` 38, `llvmpipe` 15). На машине с
видеокартой Mesa берёт d3d12 — то есть уходит в тот же драйвер GPU, который
только что не ответил. `dxil.dll` рядом с exe у пользователя — доказательство.
`mesaConsentText` и `docs/RDP_OPENGL*.md` обещают «llvmpipe».

**C. Необратимо и невидимо.** `glprobe_windows.go:252-255`: при локальном
`opengl32.dll` — `InfoLog` (невидим на релизе, `GlobalLevel=LevelWarn`) и
возврат. В UI про Mesa ничего. Пути назад нет.

### 1.3 Кого задевает

| Архив | Сейчас |
|---|---|
| `win64-full` | DLL в `mesa3d/` (CI, `ci.yml:683-696`); `installMesaFromBundle` копирует **без диалога** |
| `win64` | `mesa3d/` нет → диалог согласия; без «Да» не ставится |

A и C бьют по `win64-full`; B — по обоим.

### 1.4 Уточнение о пробнике

Подпроцесс грузит `opengl32.dll` через `windows.NewLazySystemDLL` — она ищет
**только в system32**. Проба всегда измеряет аппаратный GL, даже когда рядом с
exe лежит Mesa. Следствия: (а) железо можно проверить, не снимая Mesa; (б) чтобы
проверить саму Mesa, нужен второй режим пробы, грузящий `execDir\opengl32.dll`
по полному пути.

## 2. Решение

### 2.1 Одно состояние

`bin/gl-state.json`:

```json
{
  "phase": "rendered",
  "mode": "mesa",
  "driver": "llvmpipe",
  "renderer": "llvmpipe (LLVM 22.1.8, 256 bits)",
  "launcher_version": "v1.5.6",
  "updated_at": "2026-09-11T09:12:03Z",
  "offered_hw_renderer": ""
}
```

- Гейт перед инициализацией GL пишет `phase: "starting"` и текущий `mode`.
- Первый кадр переписывает `phase: "rendered"`.
- `mode` = `"mesa"`, если рядом с exe лежат `opengl32.dll` **и**
  `libgallium_wgl.dll` (признак нашей Mesa; одиночный чужой `opengl32.dll` не
  трогаем — только WARN); иначе `"hardware"`.

Одно правило гейта: **прошлый старт дошёл до кадра — пробу не делаем; иначе —
делаем.** «Иначе» = файла нет (первый запуск) или осталось `starting` (умер).

### 2.2 Гейт

```
NO_MESA=1 → выход (как сейчас)

прошлый старт дошёл до кадра?
  да, mode=hardware → MarkStarting; старт
  да, mode=mesa     → WARN «rendering via Mesa3D (driver=…)»; MarkStarting;
                      фоновая проба железа (§2.5); старт
  нет               → WARN «previous start did not reach the first frame»
                      (если файл был); ПРОБА железа

ПРОБА железа
  OK (≥2.1)
    Mesa рядом  → interactive: диалог D1 [Yes/No]; Yes → DisableMesa
                  non-interactive: WARN
    Mesa нет    → ничего
  таймаут     → interactive: диалог D2 [Retry/Ignore/Abort]
                  Retry → проба ещё раз; Ignore → старт как есть;
                  Abort → ветка «отказ»
                non-interactive: WARN, старт как есть
  отказ (1.1 / ошибка WGL / нераспознан)
    Mesa рядом  → WARN «neither hardware OpenGL nor Mesa3D works»;
                  interactive: диалог D5 [OK]; старт как есть
    Mesa нет    → GOARCH=386: как сейчас (ручная инструкция)
                  interactive: диалог D3 [Yes/No]; Yes → УСТАНОВКА
                  non-interactive: WARN, установки нет

MarkStarting(mode по факту после решений); старт

УСТАНОВКА
  copy из mesa3d/ (есть) | download (нет) — как сейчас
  GALLIUM_DRIVER=llvmpipe, только если переменная не задана в окружении
  проба local (§2.4)
    OK  → preloadMesa; mode=mesa
    нет → откат (снять скопированное / переименовать в .off); диалог D4 [OK];
          mode=hardware
```

`interactive` = не `-tray`. Диалоги — `MessageBoxW` (GDI, не зависит от GL);
в `-tray` их нет вообще.

Пинг-понг невозможен: каждое переключение режима — «Yes» в диалоге.

### 2.3 Диалоги (только английский)

`locale.T` здесь неприменим: `locale.SetLang` идёт в `main.go:114`, ниже гейта
(`main.go:83`). Все существующие двуязычные тексты гейта (`mesaConsentText`,
`mesaRestartText`, сообщения об ошибках) — тоже перевести на чистый английский.

**D1 — Mesa упала, железо работает** (`MB_YESNO | ICONWARNING`):
```
Singbox Launcher — OpenGL

The previous start did not reach the window while rendering via Mesa3D,
but hardware OpenGL works on this machine:
  <renderer>

Disable Mesa3D and use hardware OpenGL?
```

**D2 — таймаут** (`MB_ABORTRETRYIGNORE | ICONWARNING`):
```
Singbox Launcher — OpenGL

The OpenGL check did not finish within 10 seconds.
This is usually a temporary driver hiccup, not a missing OpenGL.

  Retry   — run the check again
  Ignore  — start with the system OpenGL as is
  Abort   — treat as "no hardware OpenGL" (offers Mesa3D / disables it)
```

**D3 — нет железа, Mesa нет** (`MB_YESNO | ICONWARNING`):
```
Singbox Launcher — OpenGL

Hardware OpenGL 2.1 was not found (got <version>, renderer "<renderer>").
The window cannot be rendered without it. This is typical for RDP sessions
and servers/VMs without a GPU.

Install the Mesa3D software renderer (llvmpipe)?
<если mesa3d/ нет: "About 24 MB will be downloaded. Internet access required.">
DLLs are placed next to singbox-launcher.exe.
```

**D4 — Mesa не поднялась** (`MB_OK | ICONERROR`):
```
Singbox Launcher — OpenGL

Mesa3D was installed but did not start (<error>).
It has been disabled again. The launcher will start with the system OpenGL.

Details: logs/native-stderr.log, logs/singbox-launcher.log
Manual guide: <rdpOpenGLDocURL>
```

**D5 — не работает ничего** (`MB_OK | ICONERROR`):
```
Singbox Launcher — OpenGL

Neither hardware OpenGL (<error>) nor Mesa3D works on this machine.
The launcher will start, but the window may stay blank.

Details: logs/native-stderr.log, logs/singbox-launcher.log
Manual guide: <rdpOpenGLDocURL>
```

### 2.4 Пин llvmpipe и проверка

Перед `preloadMesa`: `os.Setenv("GALLIUM_DRIVER", "llvmpipe")` — **только если**
переменная не задана в окружении (пользователь/поддержка могут выбрать другой
драйвер сами). Переменная читается при инициализации DLL, после загрузки менять
поздно.

Проверка — второй режим пробника `-gl-probe-local`: подпроцесс грузит
`execDir\opengl32.dll` по полному пути (`LoadLibraryEx` +
`LOAD_WITH_ALTERED_SEARCH_PATH`, как `preloadMesa`), остальное как в обычной
пробе. Успех = `renderer` содержит `llvmpipe`. Неуспех (включая
`llvmpipe: JIT is not permitted in this process` — есть в DLL, JIT может быть
заблокирован политикой) — откат и D4.

### 2.5 Сценарий возврата на железо

В режиме `mesa` при каждом старте — **фоновая** проба железа (подпроцесс,
не блокирует запуск). Если железо ответило ≥2.1 и `offered_hw_renderer` не
равен полученному renderer — после появления окна обычный Fyne-диалог:

```
Hardware OpenGL is now available:
  <renderer>
Disable Mesa3D and use it? The change applies after you restart the launcher.
[ Yes ]  [ Later ]
```

Later → `offered_hw_renderer = <renderer>`, больше не спрашиваем, пока renderer
не сменится. Yes → DisableMesa, `offered_hw_renderer = <renderer>`, сообщение
«Restart the launcher to apply».

### 2.6 Кнопка в Диагностике

Одна кнопка на вкладке Diagnostics, Windows-only, подпись по состоянию:

| Состояние файлов | Подпись | Действие |
|---|---|---|
| Mesa рядом с exe (§2.1) | `Disable Mesa3D (use hardware OpenGL)` | подтверждение → DisableMesa → «Restart the launcher to apply» |
| есть `opengl32.dll.off` | `Enable Mesa3D (software rendering)` | подтверждение → EnableMesa (переименовать обратно) → то же сообщение |
| ни того, ни другого, но есть `mesa3d/` | `Enable Mesa3D (software rendering)` | подтверждение → копия из `mesa3d/` (без preload) → то же |
| ничего из этого | кнопка скрыта | — |

Роль — переключение, когда UI есть. Восстановление делает гейт.

`DisableMesa` = переименовать `opengl32.dll`, `libgallium_wgl.dll`, `dxil.dll`
(те, что есть) в `<имя>.off`. `EnableMesa` = обратно. `mesa3d/` не трогается.

### 2.7 Логирование

**Гейт** — каждый старт через Mesa:
```
[WARN] gl: rendering via local Mesa3D (driver=llvmpipe) — hardware OpenGL is not in use; Diagnostics → "Disable Mesa3D" to switch
```

**Поднять с Info на WARN** (только эти — остальные Info повторяются в циклах):

| Строка | Где |
|---|---|
| версия сборки, GOOS/GOARCH, execDir — **новая**, безусловно первой строкой после открытия логов | `main.go` после `NewAppController` |
| `Application startup: state.json loaded (...)` | `main.go:286` |
| `Locale: language set to %q` | `main.go:173` |
| `startSingBox: Starting Sing-Box...` | `core/process_service.go:213` |
| `startSingBox: Starting Sing-Box with elevated privileges (TUN)...` | `core/process_service.go:268` |
| `monitorSingBox: Sing-Box exited gracefully (exit code 0).` | `core/process_service.go:441` |
| `Application shutting down.` | `main.go:453` |
| `ui: event loop started` — **новая** | `SetOnStarted`, `main.go:271` |

### 2.8 stderr нативного кода — в файл

`-H windowsgui` → подсистема `WINDOWS`, консоли нет, stderr ведёт в
`INVALID_HANDLE_VALUE`, запись теряется. Поэтому вывод Mesa/драйверов/GLFW
пропадает бесследно.

В начале `main()`, сразу после `EnableCrashOutput`:
`debuglog.RedirectNativeStderr(<execDir>/logs/native-stderr.log)` —
`windows.SetStdHandle(STD_ERROR_HANDLE, f.Fd())` + `os.Stderr = f`; файл держать
открытым весь сеанс. Не-Windows — no-op. Это не прикладное логирование
(конституция §5): наш код по-прежнему пишет через `debuglog`; подменяется
системный хендл для чужого нативного кода. Дополняет `debug.SetCrashOutput`
(только паника Go).

## 3. Критерии приёмки

1. Mesa3D никогда не ставится без «Yes» в диалоге — ни в `win64`, ни в `win64-full`.
2. Таймаут пробы → D2; Ignore → старт на системном GL; после отрисованного окна
   `phase=rendered, mode=hardware`, и следующие старты пробу не запускают.
3. Прошлый старт умер под Mesa, железо работает → D1; Yes → DLL переименованы
   в `.off`, старт на железе.
4. Установленная Mesa проверяется пробой `local`; `renderer` содержит `llvmpipe`;
   при неуспехе — откат и D4, установка не остаётся.
5. Режим `mesa` + железо появилось → Fyne-диалог возврата ровно один раз на
   renderer; Later — не повторяется.
6. Каждый старт через Mesa пишет WARN из §2.7; версия сборки — первой WARN-строкой
   любого старта; «event loop started» есть на каждом успешном старте.
7. `logs/native-stderr.log` создаётся; с `MESA_DEBUG=1` в него попадают строки Mesa.
8. Все тексты гейта — английские; кнопка и Fyne-диалоги — через `locale.T` с
   записями в `bin/locale/ru.json`.
9. `-tray`: ни одного `MessageBoxW`; все решения — WARN в лог, файлы не меняются.
10. `go build ./...`, `go test ./...`, `go vet ./...` на darwin;
    `GOOS=windows GOARCH=amd64 go build ./internal/platform/ ./internal/debuglog/`.

## 4. Вне scope

- Версия ассета Mesa и механизм скачивания не меняются.
- Win7 / 386 — прежняя ветка с ручной инструкцией.
- Самоперезапуск лаунчера после переключения — только сообщение «restart».
- Общая ревизия уровней логирования за пределами списка §2.7.
