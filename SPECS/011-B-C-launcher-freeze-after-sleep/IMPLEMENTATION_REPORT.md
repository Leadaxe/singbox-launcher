# Отчёт о реализации: 001 — лаунчер «Не отвечает» после сна/гибернации

**Статус:** реализовано (resume + сброс транспорта; sleep/resume события и подписчики; рефакторинг без зависимости клиентов от ОС).

**Дата:** 2025-03.

## Изменения

1. **Сброс HTTP-транспорта Clash API после resume**  
   При событии resume платформа вызывает зарегистрированный callback; в main зарегистрирован callback, вызывающий `api.ResetClashHTTPTransport()` и логирование. На Windows — по WM_POWERBROADCAST (resume 7/18); на прочих ОС callback не вызывается (no-op).

2. **События sleep/resume и статус sleep (platform)**  
   - Sleep (на Windows — wParam 4): отмена PowerContext(), установка статуса sleep, вызов RegisterSleepCallback; логирование «power: system entering sleep/hibernation».
   - Resume (7/18): сброс sleep, новый PowerContext(), вызов resume callback; логирование «power: system resumed from sleep/hibernation».
   - **IsSleeping()**, **PowerContext()**, **RegisterSleepCallback**, **RegisterPowerResumeCallback**, **StopPowerResumeListener** — единый API; документация без привязки к «только Windows» (платформа сама решает, слать события или no-op).

3. **api/clash.go**  
   - IdleConnTimeout 30 с; getHTTPClient() под мьютексом; ResetClashHTTPTransport().
   - Хелперы **requestContext()** (при IsSleeping возвращает ErrPlatformInterrupt, иначе PowerContext()) и **normalizeRequestError()** (context.Canceled → ErrPlatformInterrupt). TestAPIConnection, GetProxiesInGroup, SwitchProxy, GetDelay используют их; публичный API без контекста, при sleep/отмене возвращают **ErrPlatformInterrupt**. Зависимости от runtime.GOOS в api нет.

4. **Клиенты без зависимости от ОС**  
   main.go вызывает `RegisterPowerResumeCallback` и `StopPowerResumeListener` безусловно (без проверки GOOS). Остальные клиенты (api_service, clash_api_tab) только проверяют platform.IsSleeping() и обрабатывают api.ErrPlatformInterrupt. Реализация событий — только в platform (power_windows.go / power_stub.go).

## Риски и ограничения

- **Блокировка UI на ~7 минут** по гипотезе может быть в драйвере/OpenGL; сброс соединений и обработка resume не гарантируют устранение этой блокировки, но снижают нагрузку и последствия «протухших» TCP после пробуждения.
- Полная сборка (`go build .`) на машине без CGO/компилятора C (для go-gl) по-прежнему может падать; к изменённым пакетам это не относится.

## Проверка

- `go build ./api/... ./internal/platform/...` — успешно (Windows).
- `go vet ./api/ ./internal/platform/` — без замечаний.
- Вручную: запуск лаунчера на Windows, переход в сон/гибернацию, пробуждение — в логе должно появиться сообщение «Power resume: Clash API HTTP transport reset».

## Дополнение (2026-04-22): UI re-sync + Linux

### Коммит `735cebe` — UI re-sync после resume

До: resume только `ResetClashHTTPTransport()`. UI оставался с pre-sleep данными.
Стало (`main.go` resume-callback):

```go
platform.RegisterPowerResumeCallback(func() {
    api.ResetClashHTTPTransport()
    time.AfterFunc(3*time.Second, func() {
        fyne.Do(controller.UIService.RefreshAPIFunc)  // /proxies перезагрузка
        if controller.RunningState.IsRunning() &&
           controller.StateService.IsAutoPingAfterConnectEnabled() {
            time.AfterFunc(2*time.Second, func() {
                if controller.RunningState.IsRunning() {
                    fyne.Do(controller.UIService.AutoPingAfterConnectFunc)
                }
            })
        }
    })
})
```

3s/5s задержки — дать сетевым интерфейсам реально встать после wake.
`fyne.Do` обёртки (коммит `76e5628`) — потому что `time.AfterFunc` в goroutine,
а `RefreshAPIFunc` мутирует labels напрямую до диспатча.

### Коммит `3d31687` — Linux systemd-logind

Новый файл `internal/platform/power_linux.go`:
- Build tag `linux`.
- `startListenerLocked()` — lazy init на первой регистрации callback'а.
- `dbus.SystemBus()` + `AddMatch` на `org.freedesktop.login1.Manager.PrepareForSleep`.
- Goroutine `for sig := range ch` — обработка сигналов.
- `true` payload → sleep callbacks, `powerCtxCancel()`, `sleepingFlag.Store(true)`.
- `false` payload → resume callbacks, новый `powerCtx`, `sleepingFlag.Store(false)`.
- `SystemBus()` fail → silent no-op (минимальный дистрибутив без logind, WSL).
- `power_stub.go` build tag сужен с `!windows` до `!windows && !linux`.

Используется уже присутствующий `github.com/godbus/dbus/v5` — **новых зависимостей не добавляем**.

### macOS — не сделано, отдельной веткой

Требует cgo (IOKit framework) + CFRunLoop + `runtime.LockOSThread`.
Список задач — в TASKS.md, Этап 2 / macOS IOKit.

### Проверки

- `go build ./...` на macOS — успешно (linux-файл исключён build tag'ом).
- `go build ./internal/platform/` c `GOOS=linux` — успешно.
- `go test ./...` — зелёные.
- Ручной тест Linux: **TODO**.
