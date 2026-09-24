# SPEC 141 · Задачи

Отмечать по факту коммита. Этапы — PLAN.md.

## Этап 1 · Спека
- [x] SPEC.md, PLAN.md, TASKS.md (черновик).
- [x] Интерфейс ядра подтверждён сессией ядра по всем пунктам (v1.14.2-lx.2, SPEC 103 форка).
- [x] Развилки решены владельцем — SPEC §13 «Решения».

## Этап 2 · Разделение (без изменения поведения)
- [ ] `backend_daemon*_darwin.go` → общие файлы (пока с тегом `darwin`).
- [ ] `daemon_manager`: общее / darwin.
- [ ] `daemon_service_state`: общее / darwin; сверка sha — по набору (на darwin набор из одного файла).
- [ ] `classic_privileged`: общее / darwin.
- [ ] `purge_darwin.go`, `chain_probe.go`, `debugapi_wiring_daemon_darwin.go`, `ui/connection_local_daemon_darwin.go` — переименование.
- [ ] Команда `{Binary, Args}` с платформенным рендером; darwin-строки байт в байт прежние.
- [ ] Тесты darwin разделены тем же правилом.

## Этап 3 · Платформенный слой Windows
- [ ] `runas_windows.go`: `ShellExecuteExW` (`runas`, `NOCLOSEPROCESS | NOASYNC`, `SW_HIDE`), ожидание 120 с, код выхода, `ERROR_CANCELLED` (согласовать с SPEC 139).
- [ ] `scm_windows.go`: конфигурация, статус, DACL службы без прав.
- [ ] `winacl_windows.go`: владелец/DACL звена по SPEC §6.1 (список разрешённых SID, маска предков, маска каталога копии — как у ядра), reparse, `DRIVE_FIXED` + NTFS, ключ файла, `KnownFolderPath`.
- [ ] `sysproxy_windows.go`: set/clear WinINet как у ядра, чтение HKCU.
- [ ] `privileged_windows.go`: `sing-box-lxd.exe`, `PrivilegedCoreLogPath`.

## Этап 4 · Классификатор и менеджер Windows
- [ ] Тег `darwin || (windows && !386)`; заглушки — `!darwin && (!windows || 386)`.
- [ ] Раскладка: `<ProgramFiles>\sing-box-lxd`, `<ProgramData>\sing-box-lxd` через `windows.KnownFolderPath`; набор — exe + `libcronet.dll`.
- [ ] NotInstalled / Unsafe (`BinaryPathName`, кавычки, DACL службы, цепочка) / Stale (набор, лишний файл, `CopyMissing`) / NotRunning (≠ `SERVICE_RUNNING`) / ProcessStale / OK.
- [ ] `minCoreForRootOwnedService` по платформе: windows `1.14.2-lx.2`; CoreTooOld во всех каналах.
- [ ] Команды §5.1: install `--invite-out`, uninstall `--keep-copy [--purge]`, полный uninstall, `client add --invite-out`, copy, `sc.exe start` (runas); Kickstart пуст.
- [ ] Файл приглашения в `<Data>\bin\daemon\`, автосопряжение, удаление файла, приглашение не в лог.
- [ ] Ошибки §5.3 (отказ UAC, код ≠ 0, таймаут).
- [ ] Debug API `/daemon/*` и `capabilities` на Windows.

## Этап 5 · Системный прокси
- [ ] Шаг 3 `prepareConfigForDaemon`: `set_system_proxy` → `false`, адрес первого inbound, WARN для остальных.
- [ ] Хук в `DaemonBackend`: apply, кадр статуса, Stop, выход, смена движка, Unpair/Uninstall; «снять своё» по метке `daemon_system_proxy`.
- [ ] Сверка на старте daemon-движка (краш лаунчера).
- [ ] Шаг 4 `prepareConfigForDaemon` (Windows): `state_directory` tailscale → `<StateDir>\tailscale\<тег>`.
- [ ] `TestPrepareConfigForDaemonSystemProxy`.

## Этап 6 · Classic с правами (после SPEC 139)
- [ ] Гейт по `IsElevated`: вердикты над набором, диалог с copy/install, Retry.
- [ ] Старт копии, `Dir` = `<Data>\bin`.
- [ ] `classic.log`: проверка цепочки `ProgramData\sing-box-lxd\logs`, файл с DACL (пользователю — чтение), ротация 2 МиБ, `CoreLogPath`.
- [ ] Stop/рестарт/Kill только по PID; детект `sing-box-lxd.exe` по сессии.
- [x] Условие релиза: поиск DLL из cwd закрыт ядром (SPEC §3 п. 12).

## Этап 7 · UI
- [ ] Переключатель движка на Windows; плашки; Run as administrator; Start the service.
- [ ] Install: одно окно UAC (install `--invite-out`), автосопряжение; вставка — запасной путь.
- [ ] Uninstall `--keep-copy`, `--purge`; Remove all data — полный uninstall; `-purge-data`.
- [ ] Диалог «Core updated» (Windows); WARN копии после обновления ядра.
- [ ] Модальное предупреждение при Unsafe — раз на версию.
- [ ] Ключи `locale.T` с переводом в `bin/locale/ru.json`.
- [ ] Показать владельцу.

## Этап 8 · Доки
- [ ] `docs/DAEMON_AND_REMOTE.md` / `.ru.md`: платформы §1 и §6 (снять «всё под darwin, не в go.win7.mod»), Windows-команды, классификатор, прокси, classic с правами.
- [ ] `docs/API.md` / `.ru.md`: `/daemon/*` на Windows.
- [ ] `docs/ARCHITECTURE*.md`: новые файлы и теги.
- [ ] `docs/release_notes/upcoming.md` EN/RU, выжимка в `RELEASE_NOTES.md`.
- [ ] Запись в `SPECS/README.md`; ссылка из SPEC 137 §8 п. 5.

## Приёмка
- [x] Ответы ядра по §3 внесены в SPEC.
- [ ] Ядро v1.14.2-lx.2 собрано.
- [ ] Ручная проверка SPEC §12 на Windows 10 и Windows 11 (x64), выборочно — arm64.
- [ ] CI `run_mode=tests` зелёный (включая Win7-сборку).
- [ ] `RequiredCoreVersion` → `1.14.2-lx.2` — отдельным коммитом при релизе ядра.
