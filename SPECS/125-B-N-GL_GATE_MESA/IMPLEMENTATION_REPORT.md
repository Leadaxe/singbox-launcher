# IMPLEMENTATION REPORT 125 · Гейт OpenGL

Ветка: `develop`. Не коммичено. В рабочей копии присутствовали чужие
незакоммиченные правки (`main.go`, `core/controller.go`,
`core/process_service.go`, `core/debugapi/state_endpoints.go`,
`core/uiservice/ui_service.go`, `internal/debuglog/crash_*.go`) — сохранены,
правки SPEC 125 сделаны поверх.

## 1. Файлы

### Новые

| Файл | Содержимое |
|---|---|
| `internal/platform/glstate.go` | `GLState`, `GLStatePath/LoadGLState/SaveGLState/UpdateGLState/MarkGLStarting/MarkGLRendered`; `IsMesaInstalled/IsMesaDisabled/HasMesaBundle/HasForeignOpenGL/DisableMesa/EnableMesa/copyMesaFromBundle/removeMesaFiles`; `errForeignOpenGL`; `probeResult`; `gateInput/gateAction/decideGate`; пороги `glMinMajor/glMinMinor/glProbeTimeout`. Без build-тегов |
| `internal/platform/glprobe_notify.go` | `SetOnHardwareGLAvailable` / `notifyHardwareGLAvailable` с буфером до регистрации. Без build-тегов |
| `internal/platform/glstate_test.go` | четыре группы тестов PLAN §6 |
| `internal/debuglog/stderr_windows.go` | `RedirectNativeStderr` — `MkdirAll`, append-`OpenFile`, `windows.SetStdHandle`, `os.Stderr = f`, хендл в package var |
| `internal/debuglog/stderr_other.go` | `//go:build !windows` no-op |

### Изменённые

| Файл | Что |
|---|---|
| `internal/platform/glprobe_windows.go` | гейт по SPEC §2.2; `probeDesktopOpenGL(local bool)` с `LoadLibraryEx`+`LOAD_WITH_ALTERED_SEARCH_PATH` для local; `RunGLProbeChild(local bool)`; `probeGLViaSubprocess(local) probeResult` с полем `Timeout`; английские D1–D5; `installAndVerifyMesa`; `downloadMesa` (бывший `installMesa`, без preload); `startBackgroundHardwareProbe`; `mesaDriverName`; удалены `mesaConsentText`, `mesaInstalledNote`, `installMesaFromBundle`, `copyFile` |
| `internal/platform/glprobe_stub.go` | новые сигнатуры |
| `internal/constants/constants.go` | `GLStateFileName`, `MesaDisabledSuffix`, `NativeStderrLogFileName` |
| `main.go` | флаг `-gl-probe-local`; `RedirectNativeStderr`; WARN-строка версии; `EnsureDesktopOpenGL(execDir, !*startInTray)`; `glRenderedGrace`; `rememberOfferedRenderer`; в `SetOnStarted` — WARN «ui: event loop started», `time.AfterFunc(3s, MarkGLRendered)`, подписка `SetOnHardwareGLAvailable` → `dialog.NewConfirm` (Yes/Later); Info→WARN ×3 |
| `core/process_service.go` | Info→WARN ×3 |
| `ui/diagnostics_tab.go` | `buildMesaToggleButton(ac)`, вставка в VBox по условию |
| `bin/locale/ru.json` | 9 ключей (5 из PLAN §5 + 3 подтверждения/заголовок + `Yes`) |
| `docs/RDP_OPENGL.md`, `docs/RDP_OPENGL.ru.md` | новое поведение по PLAN §7 |
| `docs/release_notes/upcoming.md` | одна запись в Fixes, EN и RU |
| `CHANGELOG.md` | «Не выпущено → Исправления», счётчик 6→7, строка с `(#125)` |

## 2. Отклонения от PLAN

1. **`UpdateGLState` вместо прямых `Load`+`Save`.** PLAN описывал
   `MarkGLStarting`/`MarkGLRendered` как отдельные read-modify-write. Но
   `MarkGLRendered` взводится таймером на 3 с, а диалог возврата на железо
   пишет `OfferedHWRenderer` по клику пользователя — окна пересекаются, и без
   сериализации одна запись затирала бы поле другой (ровно та ловушка, ради
   которой PLAN §2 требует не терять `OfferedHWRenderer`). Добавлены
   `glStateMu` и публичный `UpdateGLState(execDir, func(*GLState))`; через него
   же `main.rememberOfferedRenderer`.

2. **`glMinMajor`/`glMinMinor`/`glProbeTimeout` переехали в `glstate.go`.**
   Их читают `probeResult.ok()` и `describe()`, которые обязаны собираться на
   darwin. В `glprobe_windows.go` их больше нет.

3. **`SINGBOX_LAUNCHER_FORCE_MESA=1` удалён.** В новом гейте у него нет
   осмысленного места: он означал «пропустить пробу и предложить Mesa», а
   теперь предложение Mesa — результат конкретного диалога, и то же самое
   делает кнопка в Диагностике (доступная в том числе при работающем железе,
   если рядом есть `mesa3d/`). Из обоих `docs/RDP_OPENGL*.md` упоминание
   убрано; исторический `docs/release_notes/1-4-2.md` не трогал.

4. **`HasForeignOpenGL` добавлен сверх PLAN.** Нужен, чтобы отличить чужой
   одиночный `opengl32.dll` (SPEC §2.1: «только WARN») от нашей Mesa и чтобы
   `DisableMesa` возвращал `errForeignOpenGL`, а не молча переименовывал чужой
   файл.

5. **`installMesa` переименован в `downloadMesa`** и возвращает
   `([]string, error)` — список распакованных файлов нужен для отката, когда
   локальная проба не приняла установку. Общая обёртка — `installAndVerifyMesa`.

6. **`copyFile` из `glprobe_windows.go` удалён** в пользу `copyFileGL` в
   `glstate.go` (тот же код, но доступен без build-тегов). В пакете других
   вызывающих не было.

7. **Диалог кнопки в Диагностике получил отдельный текст вопроса.** PLAN
   говорил только «подтверждение — `dialog.NewConfirm`»; ставить подпись
   кнопки и заголовком, и телом вопроса читается плохо, поэтому добавлены две
   строки (`Stop rendering through Mesa3D and use hardware OpenGL?` /
   `Render the window through the Mesa3D software renderer instead of the GPU?`)
   и переведены в `ru.json`.

8. **Ссылки `rdpOpenGLDocURL`/`win7OpenGLDocURL` переведены на английские
   файлы** (`RDP_OPENGL.md`, `WIN7_OPENGL.md`) — диалоги гейта теперь целиком
   английские, отправлять из них на русскую страницу непоследовательно.

9. **`installAndVerifyMesa` пишет `Renderer` и `Driver` в состояние.** PLAN
   этого не требовал явно, но WARN-строка SPEC §2.7 содержит `driver=…`, а
   брать его на следующих стартах неоткуда, если он не сохранён.

10. **`gateInput.PrevDiedUnderMesa` — «Mesa сломана» ≠ «Mesa просто стоит».**
    Дефект найден при ревью координатором уже после первой сборки. Ветка
    «отказ железа + Mesa установлена → `actNeitherWorks`» верна, только когда
    прошлый старт умер именно под Mesa. У целевой аудитории Mesa — RDP-машины
    и ВМ без GPU — первый же старт после обновления выглядел иначе:
    `bin/gl-state.json` ещё нет → состояние «грязное» → делается проба железа
    → железа в RDP нет по определению → и исправная Mesa получала ложный D5
    «Neither hardware OpenGL nor Mesa3D works» плюс `ErrorLog`, хотя окно
    прекрасно отрисовывалось. Симметрично D1 на первом старте (Mesa есть,
    железо работает) утверждал «The previous start did not reach the window»
    про старт, которого не было.

    Исправлено: в `gateInput` добавлено поле `PrevDiedUnderMesa`, которое гейт
    считает как `hasState && Phase == starting && Mode == mesa`. Ветка отказа
    теперь `if in.MesaInstalled { if in.PrevDiedUnderMesa { actNeitherWorks };
    actStart }`; на этом `actStart` гейт пишет
    `gl: hardware OpenGL unavailable (%s) — keeping the installed Mesa3D` и
    штампует `mode=mesa`. Текст D1 разведён на два варианта — про смерть
    старта и нейтральный «Mesa3D is installed next to the exe, but hardware
    OpenGL works…». Покрыто тремя новыми кейсами `TestDecideGate`, а
    инвариантный перебор non-interactive расширен осью `PrevDiedUnderMesa`.

11. **Чужой `opengl32.dll` закрывает путь установки, а не только `DisableMesa`.**
    PLAN ставил `errForeignOpenGL` только на `DisableMesa`. Но сценарий
    «чужой одиночный `opengl32.dll` + папка `mesa3d/`» проходил бы мимо этой
    защиты: `copyMesaFromBundle` затёр бы чужой файл. Теперь `copyMesaFromBundle`
    возвращает `errForeignOpenGL` в этом случае, а кнопка в Диагностике при
    чужом DLL не показывается вовсе (пятый подслучай теста `DisableEnableMesa`).

## 3. Что не сделано / ограничения

- **Не проверено на живой Windows.** Вся Win32-часть (`LoadLibraryEx` local-
  пробы, `MessageBoxW` с `MB_ABORTRETRYIGNORE`, `SetStdHandle`) собирается и
  проходит `go vet` под `GOOS=windows`, но выполнить её на этой машине
  (macOS) нельзя. Критерии приёмки 1–7, 9 требуют ручного прогона на Windows.
- **`GOOS=windows go build ./...` целиком невозможен** — Fyne тянет cgo-пакет
  `go-gl/gl`, для которого нужен кросс-тулчейн. Собираются и проверяются
  `./internal/platform/` и `./internal/debuglog/`, как и требует SPEC §3 п.10.
- **Тестов на Win32-ветки нет** — `decideGate` вынесен в чистую функцию именно
  затем, чтобы покрыть логику решений без Win32; сами диалоги и загрузка DLL
  не тестируются.

## 4. Финальный прогон

```
$ go build ./...
# singbox-launcher
ld: warning: ignoring duplicate libraries: '-lobjc'        # норма для darwin/cgo

$ go vet ./...
(чисто, ни строки)

$ GOOS=windows GOARCH=amd64 go build ./internal/platform/ ./internal/debuglog/
(чисто)

$ GOOS=windows GOARCH=amd64 go vet ./internal/platform/
(чисто)

$ go test -p 1 ./...
exit=0, ни одного FAIL
...
ok  	singbox-launcher/ui/configurator/presentation	(cached)
ok  	singbox-launcher/ui/configurator/tabs	(cached)
ok  	singbox-launcher/ui/configurator/utils	(cached)

$ go test ./internal/platform/ -run 'GLState|Gate|Mesa' -v
--- PASS: TestGLStateLoadSaveRoundTrip
--- PASS: TestGLStateMarkStartingKeepsPreviousRendererAndOffer
--- PASS: TestDecideGate                       (14 подслучаев)
--- PASS: TestDisableEnableMesa                (4 подслучая)
PASS
ok  	singbox-launcher/internal/platform	0.835s

$ go run ./tools/l10n/l10n_check
[l10n_check] keys used: 1333, catalog: 1433, missing+orphan warns: 159, hard fails: 0
```

### Замечание о параллельном `go test ./...`

Первый прогон **без** `-p 1` убил `core/debugapi` по таймауту 11 минут
(`FAIL singbox-launcher/core/debugapi 661.119s`). Это **не** регрессия SPEC 125:
пакет не ссылается ни на одну новую сущность (грепом по `MarkGL|GLState|Mesa|
EnsureDesktopOpenGL|RedirectNativeStderr` — ноль совпадений), отдельно
`go test ./core/debugapi/ -v` проходит за 3.9 с, и весь набор с `-p 1`
зелёный. Причина — конкуренция за реальные сетевые порты между пакетами при
параллельном запуске. Пакет содержит чужие незакоммиченные правки
(`state_endpoints.go`), которых я не касался.

## 5. §6 после RC (прогон `v1.5.5-23-ge516f0f5-prerelease`, 13.09.2026)

### 5.1 Изменённые и новые файлы

| Файл | Что |
|---|---|
| `internal/platform/restart_windows.go` (новый) | `RestartSelf()` — `exec.Command(exe, os.Args[1:]...)`, `cmd.Dir = <каталог exe>`, `SysProcAttr{CreationFlags: CREATE_NEW_PROCESS_GROUP \| DETACHED_PROCESS}`, `Start()`. Без `PrepareCommand`: он прячет окно, а новому процессу окно нужно |
| `internal/platform/restart_other.go` (новый) | `RestartSelf()` → ошибка «not supported» |
| `internal/platform/glstate.go` | `GLPhaseRestart`; `decideGate` считает `restart` чистой фазой наравне с `rendered`; `probeResult.Vendor`, `probeResult.SawMesa`, учёт `SawMesa` в `ok()`/`describe()`; `RequestRestartAfterExit`/`RestartRequested` |
| `internal/platform/glprobe_windows.go` | `probeHardware(execDir)`; разбор `vendor=` в `probeGLViaSubprocess`; `restartToApply` (состояние `restart` → D6 → `RestartSelf` → `os.Exit(0)`); D1-Yes и D3-Yes+verify уходят в него; `mesaDriverName` → `mesaActualDriver`+`mesaVerifiedRenderer`; удалены `preloadMesa` и `mesaRestartText`; фоновая проба §2.5 переведена на `probeHardware` |
| `main.go` | в самом конце (после `Run()`, `GracefulExit` и закрытия логов) — `platform.RestartRequested()` → `RestartSelf()` |
| `ui/diagnostics_tab.go` | вопрос кнопки дополнен «The launcher will restart now.»; после успешного Disable/Enable — `UpdateGLState(phase=restart, mode)` → `RequestRestartAfterExit()` → `ac.GracefulExit()` |
| `bin/locale/ru.json` | новый ключ `The launcher will restart now.` |
| `internal/platform/glstate_test.go` | два кейса §6.3 п.14 |
| `docs/RDP_OPENGL.md`, `.ru.md` | раздел «почему смена рендерера перезапускает лаунчер» (импорт `OPENGL32.dll`, обход Mesa пробой); кнопка Диагностики описана как перезапускающая |
| `CHANGELOG.md`, `docs/release_notes/upcoming.md` | существующие записи (#125) дополнены самоперезапуском и обходом Mesa пробой (EN и RU) |

### 5.2 Отклонения от ТЗ §6 и почему

1. **Кнопка Диагностики не зовёт `RestartSelf()` на месте.** ТЗ: «после
   успешного Disable/Enable → `UpdateGLState(restart)` → `RestartSelf()`». Так
   новый процесс поднялся бы, пока старый ещё держит запущенный sing-box, и
   встретил бы пользователя диалогом «уже запущено» (а в TUN-режиме — ещё и
   гонкой за интерфейс). Поэтому кнопка ставит флаг
   `platform.RequestRestartAfterExit()` и вызывает `ac.GracefulExit()` (тот же
   путь, что у кнопки Exit), а сам `RestartSelf()` выполняется последней
   строкой `main()` — после `Application.Run()`, остановки ядра и закрытия
   логов. Состояние `phase=restart` пишется до выхода, как и требует ТЗ.

   Следствие: ветку «`RestartSelf` вернул ошибку → показать `Restart the
   launcher to apply.`» на этом пути показать уже некому (окно закрыто), она
   остаётся только `ErrorLog`. В гейте (D6) эта ветка работает как описано —
   там UI ещё нет и всё идёт через `MessageBoxW`.

2. **Порядок внутри D6.** Диалог показывается ДО `RestartSelf()`, а не после:
   иначе новый процесс успевает открыть окно поверх модального сообщения
   старого и пользователь видит два лаунчера сразу. При ошибке `Start()`
   показывается второй `MessageBox` с «Please restart the launcher manually.»
   и старт продолжается (`MarkGLStarting` ниже перепишет `restart` в
   `starting`, что верно: этот процесс всё-таки стартует).

3. **`mesaDriverName(state)` не переписан, а разделён на две функции.**
   `mesaActualDriver()` — только `GALLIUM_DRIVER` из окружения (`unset`, если
   пусто: после `pinMesaDriver` такого быть не должно, но врать нельзя),
   `mesaVerifiedRenderer(state)` — `state.Renderer` или `not verified`.
   Смешивать их в одну функцию нельзя: WARN печатает оба поля, и именно
   подстановка пина вместо факта дала ложное `driver=llvmpipe` при живом
   d3d12.

4. **Диалог возврата §2.5 (Fyne, «Hardware OpenGL is now available») оставлен
   с текстом «applies after you restart the launcher».** ТЗ §6 его не
   упоминало, а сам он ведёт себя честно: `DisableMesa` без перезапуска. См.
   §5.4 — это кандидат на приведение к R5 отдельным решением.

### 5.3 Проверки

```
$ gofmt -l internal/platform/ ui/diagnostics_tab.go main.go      # пусто
$ go build ./...                                                 # только ld: warning -lobjc
$ go vet ./...                                                   # чисто
$ go test ./internal/platform/                                   # ok
$ GOOS=windows GOARCH=amd64 go build ./internal/platform/ ./internal/debuglog/   # чисто
$ GOOS=windows GOARCH=amd64 go vet ./internal/platform/          # чисто
```

### 5.4 Что остаётся сомнительным

- **Fyne-диалог §2.5 не перезапускает лаунчер** (`main.go`, подписка
  `SetOnHardwareGLAvailable`): он делает `DisableMesa` и советует
  перезапустить руками. По духу R5 его стоило бы свести к тому же
  самоперезапуску, но ТЗ §6 этого не требовало, а трогать поведение диалога
  без решения владельца не стал.
- **`probeHardware` и `DisableMesa` из UI могут пересечься.** Фоновая проба
  §2.5 уводит `opengl32.dll` в `.probe` на время дочернего процесса; если
  ровно в это окно пользователь нажмёт кнопку в Диагностике, `DisableMesa`
  увидит `IsMesaInstalled()==false`. Окно — секунды раз в старт, и худший
  исход — сообщение «no Mesa3D DLLs next to exe» вместо переключения, файлы
  при этом целы. Замка не ставил: он потребовал бы разделяемого состояния
  между гейтом и UI ради сценария, который на практике не воспроизвести.
- **Win32-часть по-прежнему не проверена вживую** — переименование
  отображённой DLL, `DETACHED_PROCESS` и D6 собираются и проходят `go vet`
  под `GOOS=windows`, но выполнить их на macOS нельзя. Критерии 11–13
  требуют прогона на Windows.
