# Задачи: 001 — лаунчер «Не отвечает» после сна/гибернации

- [x] api/clash.go: IdleConnTimeout (30 с), clashHTTPClient(), mutex + getHTTPClient(), ResetClashHTTPTransport(), заменить все httpClient.Do на getHTTPClient().Do
- [x] internal/platform/power_windows.go: скрытое окно, WM_POWERBROADCAST (resume 7/18, sleep 4), RegisterPowerResumeCallback, RegisterSleepCallback, StopPowerResumeListener; состояние sleep, PowerContext(), IsSleeping()
- [x] internal/platform/power_stub.go: IsSleeping false, PowerContext background, RegisterSleepCallback no-op
- [x] main.go: безусловно RegisterPowerResumeCallback (ResetClashHTTPTransport + лог) и StopPowerResumeListener; в таймере обновления трея проверка IsSleeping()
- [x] api/clash.go: requestContext(), normalizeRequestError(); HTTP-функции используют их, возвращают ErrPlatformInterrupt при sleep/отмене; контекст не в публичном API; без зависимости от GOOS
- [x] core/services/api_service.go, ui/clash_api_tab.go: проверка platform.IsSleeping() перед запросами (ранний выход)
- [x] PLAN.md, IMPLEMENTATION_REPORT.md обновлены

## Этап 2 — расширение (2026-04-22)

### UI re-sync (коммит `735cebe`)
- [x] main.go resume-callback: `time.AfterFunc(3s)` → `fyne.Do(RefreshAPIFunc)`.
- [x] +2s chained → `fyne.Do(AutoPingAfterConnectFunc)` если running + auto-ping enabled.
- [x] `fyne.Do`-обёртка для race-safety (commit `76e5628`).

### Linux systemd-logind (коммит `3d31687`)
- [x] `internal/platform/power_linux.go` (build tag `linux`).
- [x] DBus-подписка на `org.freedesktop.login1.Manager.PrepareForSleep`.
- [x] Sleep/resume callbacks, sleepingFlag atomic, powerCtx lifecycle.
- [x] Silent no-op при отсутствии logind (минимальный дистрибутив / WSL).
- [x] `power_stub.go` build tag сужен до `!windows && !linux`.

### macOS IOKit — **TODO**
- [ ] `internal/platform/power_darwin.go` (build tag `darwin`).
- [ ] cgo IOKit: `IORegisterForSystemPower` + CFRunLoop + `runtime.LockOSThread`.
- [ ] Обработка `kIOMessageSystemWillSleep` (с `IOAllowPowerChange`) + `kIOMessageSystemHasPoweredOn`.
- [ ] `power_stub.go` сужается до `!windows && !linux && !darwin`.
- [ ] Ручной тест `pmset sleepnow` + проверка логов.
- [ ] Отдельной веткой (`feat/macos-power-resume`).

### Ручные тесты
- [ ] Linux: `systemctl suspend` + проверка `debuglog` строки `platform/power_linux: subscribed`.
- [ ] macOS: после реализации IOKit — `pmset sleepnow`.
