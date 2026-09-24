# SPEC 139 · Задачи

Отмечать по факту коммита. Этапы — PLAN.md, номера разделов — SPEC.md.

## Этап 1 · Спека
- [x] SPEC.md, PLAN.md, TASKS.md.

## Этап 2 · Платформа
- [x] `internal/platform/elevation_windows.go`: `IsElevated` (`sync.Once`; `Token.IsElevated` или `Token.IsMember(WinBuiltinAdministratorsSid)`), `ElevationAsksOtherAccount` (`TokenElevationType == Default` и не повышен).
- [x] Там же `RunElevated(exe, args, dir, show)`: `ShellExecuteExW` через `NewLazySystemDLL("shell32.dll")`, `runas`, `SEE_MASK_NOCLOSEPROCESS|SEE_MASK_NOASYNC`, `GetForegroundWindow`, `LockOSThread` + `CoInitializeEx`; `ERROR_CANCELLED` — отдельная ошибка-сентинел.
- [x] Там же `WaitForProcessExit(pid, exe, timeout)`: `OpenProcess(SYNCHRONIZE|PROCESS_QUERY_LIMITED_INFORMATION)`, `QueryFullProcessImageName` против своего exe, `WaitForSingleObject`; отказ `OpenProcess` — опрос списка процессов раз в 250 мс.
- [x] `internal/platform/elevation_other.go`: `IsElevated` = `os.Geteuid() == 0`, остальное — not supported.
- [x] `internal/platform/autostart_windows.go` / `autostart_other.go`: чтение (путь в кавычках, флаг `-start`), запись `"<exe>" -tray [-start]`, удаление только своего значения.
- [x] Без `min`/`max`, `slices`, `maps`; API только из `x/sys` v0.25.0.

## Этап 3 · Раскладка (SPEC §7)
- [x] `internal/paths/paths.go`: предикат «AppDir пишется пользователем» (проба + `ProgramFiles`, `ProgramFiles(x86)`, `ProgramW6432`, `SystemRoot` из `env`, без регистра, по границе каталога).
- [x] Правило 2 — только при предикате; иначе `MarkerIgnored`, дальше правила 3–4. Правило 3 и Windows-фоллбэк — на предикат.
- [x] `Layout` из `-handoff=<PID>|<Mode>|<DataDir>|<LogDir>`: разбор и проверка (PID > 0, известный Mode, абсолютные пути); невалидное — ошибка, вызывающий идёт в `Resolve`.
- [x] `LogLine` и строка Mode (`PathsInfo.Lines`) — суффикс `portable.txt ignored`.
- [x] `internal/paths/switch.go`: `SystemDefault` на предикат.
- [x] `internal/paths/resolve_test.go`: строки `TestResolveMatrix` — маркер и Legacy под `C:\Program Files` при проходящей пробе → System; маркер в непишущемся каталоге (Linux, Windows) → System; пишущаяся папка → Portable/Legacy как было; разбор `-handoff`.

## Этап 4 · Манифест и старт
- [x] `app.manifest`: `level="asInvoker"`.
- [x] `.github/workflows/ci.yml:548-553`: комментарий под asInvoker (почему `-manifest` обязателен: виртуализация 32-бит).
- [x] `main.go`: `flag.Parse` до `paths.Resolve`; флаги `-handoff`, `-autostart=on|off`.
- [x] `main.go`: с `-handoff` — раскладка из флага, `WaitForProcessExit` до crash-лога и GL-пробы (25 с); WARN о таймауте после открытия логов.
- [x] `main.go`: `-autostart` до GUI, как `-paths` (печать, код 0/1).
- [x] `main.go:279`: `elevated=yes|no` в первой WARN-строке; `main.go:616`: заголовок `(Administrator)` при повышении (Windows).

## Этап 5 · Гейты (SPEC §6)
- [x] `core/controller.go:821-852`: без прав — ни NLA/адаптеров, ни правил брандмауэра; одна строка INFO с перечнем.
- [x] `core/process_service.go:52`: `runGhostTunCleanup` без прав — пропуск, DebugLog.
- [x] `core/purge.go:50`: `networkCleanup` без прав — пропуск; CLI-строка `Network cleanup: skipped (needs administrator)…` с командой.
- [x] `internal/paths/purge.go`: остатки под AppDir без пробы записи процесса — пропуск с пометкой, не ошибка (код выхода 0).
- [x] `core/process_service.go:710-738`, `ui/diagnostics_tab.go:49-59`: `taskkill` не смог и процесс жив при `!IsElevated` → сообщение с Restart as administrator; `RunningState` не сбрасывать, пока процесс жив.
- [x] `core/storage_switch.go:46`: предикат §7; повышенный экземпляр — недоступно с подсказкой.
- [x] Комментарии и текст WARN: `internal/platform/file_dialog_ps.go:12`, `file_dialog_windows.go:14`, `singtun_fwrules_windows.go:18-19`, `wintun_cleanup_windows_device.go:155-159`, `ui/diagnostics_tab.go:46-48`.

## Этап 6 · Диалог TUN и перезапуск (SPEC §4–§5)
- [x] `core/elevation.go`: гейт в `ProcessService.Start` после `rebuildConfigBeforeStart` (Windows, `!IsElevated`, `ConfigHasTun`); скрытое окно показать до диалога; без UI — WARN.
- [x] Диалог: Restart as administrator / Switch to proxy mode / Cancel; фраза про учётную запись по `ElevationAsksOtherAccount`; порт из `proxy_in_listen_port`; место под Install service (SPEC 141).
- [x] Перезапуск: аргументы через `flag.Visit` (без `-tray` и старого `-handoff`, плюс `-start` и `-handoff`), `RunElevated`, отмена — строка в диалоге, успех — WARN и `GracefulExit`.
- [x] `SwitchToProxyMode`: `tun=false`, `enable_proxy_in=true`, `proxy_in_set_system_proxy=true` → `Save` → `RebuildConfigIfDirty(true)` → `Start`; недоступна при открытом конфигураторе.
- [x] `internal/dialogs`: диалог с действиями и строкой статуса (если `ShowCommandRetry` не подходит).
- [x] Ключи SPEC §4 с переводом в `bin/locale/ru.json`; `l10n_check --strict` без сирот.
- [ ] Показать владельцу.

## Этап 7 · Автозапуск (SPEC §8)
- [x] `core/autostart.go`: состояние (свой exe / чужой / нет), запись, удаление; CLI.
- [x] `ui/settings_tab.go`: Start with Windows и Connect VPN at sign-in в Connection (только Windows); подсказка «another copy»; недоступны в повышенном.
- [x] Remove all data (`ui/settings_purge.go`) и `-purge-data -yes`: значение автозапуска удаляется, если указывает на этот exe.
- [ ] Показать владельцу.

## Этап 8 · Доки
- [x] `docs/release_notes/upcoming.md` EN/RU; выжимка в `RELEASE_NOTES.md` (UAC больше нет на каждом старте, TUN — перезапуск с правами, переезд данных из Program Files, автозапуск).
- [x] `README.md`/`.ru.md` и тело релиза `ci.yml:874`: вместо `C:\Program Files\…` — пользовательская папка (или установщик SPEC 140); TUN и права.
- [x] `docs/ARCHITECTURE*.md`, `docs/ARCHITECTURE_PACKAGES*.md` — повышение по требованию, новые файлы; `docs/BUILD_WINDOWS*.md` — asInvoker; `docs/TROUBLESHOOTING*.md` — «TUN без прав», «сирота с правами».
- [x] Ссылки: SPEC 135 §3.5 → 139; SPEC 137 §8 п. 5 → 139/141; запись в `SPECS/README.md`.

## Приёмка
- [x] CI `run_mode=tests` зелёный (run 36023432577).
- [x] CI `run_mode=build`: артефакты win64 и win7-32 (run 36023446055).
- [ ] Ручная проверка SPEC §11 на Windows 10/11 (п. 1–9) и Win7 SP1 (п. 10).
- [ ] Пункты SPEC §12 подтверждены или спека поправлена.
- [ ] Закрытие #99 — после релиза, ответ публикует владелец.
