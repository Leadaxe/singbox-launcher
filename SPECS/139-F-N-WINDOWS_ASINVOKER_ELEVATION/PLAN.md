# SPEC 139 · План реализации

Норма — SPEC.md. Ветка — `spec-139-asinvoker` (worktree от `develop`), не
пушится до команды владельца. SPEC 135 уже в `develop`; SPEC 140 без 139 не
выпускается (её §8), SPEC 141 берёт из 139 `IsElevated`, `RunElevated` и
диалог §4.

## Архитектурные решения

| Решение | Почему |
|---|---|
| `platform.IsElevated()` считается один раз (`sync.Once`): `TokenElevation` или членство в `Administrators` | токен процесса не меняется; второе условие — машины с выключенным UAC (SPEC §2 п. 2) |
| Гейт TUN — в `ProcessService.Start` после пересборки, до `exec` | одна точка для кнопки, трея, `-start`, Debug API и авто-рестарта; `config.json` уже итоговый |
| Диалог и действия — в `core` (`core/elevation.go`), кнопки через `internal/dialogs` | как гейт SPEC 137 (`core/classic_privileged_darwin.go` → `dialogs.ShowCommandRetry`); SPEC 141 добавит кнопку в тот же список действий |
| Перезапуск: новый экземпляр первым, старый выходит после успеха `ShellExecuteExW` | отказ в UAC ничего не ломает |
| Новый ждёт выхода старого по PID до любых файлов, порта и трея | single-instance нет (SPEC §5), мьютекс SPEC 140 появится позже и тоже встанет после ожидания |
| `ShellExecuteExW` — своя обёртка над `shell32.dll` (`platform.RunElevated`) | в `x/sys/windows` v0.25.0 и v0.47.0 только `ShellExecute`; нужны `SEE_MASK_NOASYNC` и дескриптор; тот же примитив нужен SPEC 141 §5.2 |
| Раскладка повышенному — флагом `-handoff`, не переменными окружения | окружение под `runas` не рассчитываем; флаг переживает и другую учётную запись |
| Предикат «AppDir пишется пользователем» в `internal/paths` (проба + защищённые каталоги Windows) | пакет-лист без `x/sys`: переменные `ProgramFiles*`/`SystemRoot` приходят через `env` резолвера, `Resolve` остаётся чистым |
| «Switch to proxy mode» — путь `ApplyLogLevelAndReloadCore` (state на диске → `RebuildConfigIfDirty(true)`) | проверенная запись `vars` с пересборкой |
| Автозапуск: registry в `internal/platform`, состояние для UI — `core/autostart.go` | UI ходит через контроллер (CONSTITUTION §1.5); CLI `-autostart` и очистка — тот же код |

## Файлы

**Новые** (по одному на платформенный слой):

| Файл | Что |
|---|---|
| `internal/platform/elevation_windows.go` | `IsElevated`, `ElevationAsksOtherAccount` (`TokenElevationType`), `RunElevated` (`ShellExecuteExW`, `CoInitializeEx`, `GetForegroundWindow`), `WaitForProcessExit(pid, exe, timeout)` (`OpenProcess` + проверка образа, иначе опрос списка) |
| `internal/platform/elevation_other.go` (`!windows`) | `IsElevated` = `os.Geteuid() == 0`; остальное — «not supported» / no-op |
| `internal/platform/autostart_windows.go` | чтение, запись, удаление значения `HKCU\…\Run\singbox-launcher`, разбор пути в кавычках |
| `internal/platform/autostart_other.go` (`!windows`) | «not supported» |
| `core/elevation.go` | гейт TUN, диалог, перезапуск (`flag.Visit` → аргументы, `-handoff`), `SwitchToProxyMode`, строка INFO пропусков, сообщение Kill без прав |
| `core/autostart.go` | `AutostartState`, `SetAutostart` для Settings, `AutostartCLI` для `-autostart`, удаление при очистке |

**Существующие:**

| Файл | Что |
|---|---|
| `app.manifest` | `level="asInvoker"` |
| `.github/workflows/ci.yml` | комментарий `:548-553`; `-manifest` остаётся |
| `main.go` | `flag.Parse` до `Resolve`; `-handoff` (раскладка, ожидание родителя до crash-лога); `-autostart`; `elevated=` в WARN старта; заголовок окна `(Administrator)` |
| `internal/paths/paths.go` | предикат §7, правило 2 с пробой, правило 3 и фоллбэк на предикат, `Layout` из `-handoff`, `MarkerIgnored`, `LogLine` и строка Mode |
| `internal/paths/switch.go` | `SystemDefault` на предикат |
| `internal/paths/resolve_test.go` | строки матрицы: маркер/Legacy под Program Files и в непишущемся каталоге → System, повышенная проба в Program Files → System, разбор `-handoff` |
| `internal/paths/purge.go` | остатки под непишущимся AppDir — пропуск с пометкой |
| `core/process_service.go` | вызов гейта в `Start`; гейт очистки после Stop (`runGhostTunCleanup`); Kill без прав |
| `core/controller.go` | гейт `CleanupStaleTunAtStartUtil` и правил брандмауэра, одна строка INFO |
| `core/purge.go` | `networkCleanup` за гейтом, текст CLI, удаление значения автозапуска |
| `core/storage_switch.go` | блокировка Portable: предикат и повышенный экземпляр |
| `internal/dialogs/dialogs.go` | диалог с набором действий и строкой статуса (отмена UAC), если `ShowCommandRetry` не подходит |
| `ui/settings_tab.go` | чекбоксы автозапуска в Connection (Windows) |
| `ui/settings_purge.go` | пункты сети и остатков недоступны без прав; пункт автозапуска |
| `ui/diagnostics_tab.go` | Kill без прав, комментарий `:46-48` |
| `internal/platform/file_dialog_ps.go`, `file_dialog_windows.go`, `singtun_fwrules_windows.go`, `wintun_cleanup_windows_device.go` | комментарии и текст WARN (SPEC §6 п. 10) |
| `bin/locale/ru.json` | ключи SPEC §4 |
| `docs/release_notes/upcoming.md`, `RELEASE_NOTES.md`, `README.md`/`.ru.md` (совет «Program Files» → пользовательская папка или установщик, TUN и права), тело релиза `ci.yml:874`, `docs/ARCHITECTURE*.md`, `docs/ARCHITECTURE_PACKAGES*.md`, `docs/BUILD_WINDOWS*.md`, `docs/TROUBLESHOOTING*.md`, `SPECS/README.md`, ссылки из SPEC 135 §3.5 и 137 §8 п. 5 | документация |

## Этапы (коммиты)

| # | Что | Проверка |
|---|---|---|
| 1 | SPEC/PLAN/TASKS | — |
| 2 | Платформа: `elevation_*`, `autostart_*` | `go build ./...`; `GOOS=windows CGO_ENABLED=0 go build ./internal/platform/ ./internal/paths/` |
| 3 | Раскладка: предикат, правило 2, `-handoff`, `SystemDefault`, пометки; тест матрицы (единственный новый тест волны, data-критичный) | `go test ./internal/paths/ -run TestResolveMatrix -count=1` |
| 4 | Манифест, `main.go` (порядок флагов, `-handoff`, ожидание, `-autostart`, `elevated=`, заголовок) | `go build ./...` |
| 5 | Гейты: очистка при старте и после Stop, очистка данных, Kill, Portable; тексты | `go build ./...`; кросс-сборка п. 2 |
| 6 | Диалог TUN, перезапуск, Switch to proxy mode; locale | `go build ./...`; `l10n_check --strict`; показать владельцу |
| 7 | Автозапуск в Settings и очистке | `go build ./...`; показать владельцу |
| 8 | Доки и заметки | `paths_guard`; `win7guard` — в CI |
| 9 | CI `run_mode=tests`, затем `run_mode=build`; ручная приёмка SPEC §11 на Windows 10/11 и Win7 | — |

Порядок: 2 → 3 → 4 → 5 ∥ 6 ∥ 7 → 8 → 9.

## Политика проверок

Локально — `go build ./...`, кросс-сборка затронутых не-GUI пакетов под
Windows и один новый тест по имени. UI (этапы 6–7) — сборка и показ, тестов
нет. Полный прогон, `go vet`, Win7 (go1.20, `go.win7.mod`) — только CI.

## Риски

1. **Раскладка повышенного и обычного расходится.** Закрыто `-handoff` для
   перезапуска и предикатом §7 для ручного «Запуск от имени администратора».
   Остаток: папка вне Program Files с нестандартными ACL, запущенная
   повышенной вручную, — Portable/Legacy у повышенного, System у обычного.
2. **Два экземпляра при перезапуске.** Ожидание 25 с против бюджета
   `GracefulExit` 18 с; после таймаута новый стартует с WARN, старый
   `forceExitAfter` всё равно завершит.
3. **Окно UAC за окном лаунчера.** `hwnd = GetForegroundWindow()` в момент
   нажатия; проверка SPEC §11 п. 3.
4. **Win7-32 без манифеста** — виртуализация и ложная проба. Манифест
   остаётся; SPEC §11 п. 10 проверяет отсутствие `VirtualStore`.
5. **Файлы повышенного экземпляра в профиле.** Владелец — `Administrators`,
   DACL наследуется от профиля; не подтвердится — обычный экземпляр
   получит «Access denied» на `state.json`. SPEC §11 п. 6.
6. **Системный прокси после падения ядра** в proxy-режиме остаётся на мёртвом
   порту — поведение ядра, вне рамок (SPEC 140 §2).
7. **Switch to proxy mode при открытом конфигураторе** — кнопка недоступна;
   иначе его Save вернул бы `tun=true`.
8. **INFO не видна в релизном логе** — поэтому `elevated=` в первой WARN-строке.
9. **Ярлыки с флагом «Запускать от имени администратора»** — `IsElevated`
   истинно, всё как до 139; данные — по предикату §7.
10. **Правила маршрутизации по процессу в proxy-режиме** (ядро без прав не
    всегда прочтёт путь повышенного процесса) — не проверено, наблюдать по
    issue после релиза.
