# SPEC 137 · Задачи

Отмечать по факту коммита. Этапы — PLAN.md.

## Этап 1 · Спека
- [x] SPEC.md, PLAN.md, TASKS.md.

## Этап 2 · Гейт
- [x] `checkPrivilegedCoreCopy`: no core / missing / unsafe / outdated / ok на функциях SPEC 136 (цепочка, кэш sha, `EvalSymlinks`).
- [x] `privilegedCopyCommandFor`: install при plist службы, иначе `--service=copy`; квотинг `daemonServiceCommand`.
- [x] Гейт перед AEWP в `startSingBoxPrivileged`; стартует копия.
- [x] `TestPrivilegedCoreCopyGate`.

## Этап 3 · Исполнение под root
- [ ] Старт: `/usr/bin/env -i PATH=… /bin/sh -c <тело-константа> start-singbox-privileged <аргументы>`; скрипт в DataDir больше не пишется, старый удаляется.
- [ ] Stop/рестарт: `/bin/kill -TERM` по PID, pid-файл удаляет лаунчер.
- [ ] pkill (диалог «already running», Diagnostics): `/usr/bin/pkill` без шелла.
- [ ] Снятие TUN: `/bin/rm -rf --` без шелла.
- [ ] `privilegedAuthReuse` (вариант А; Б — `false`).
- [ ] `TestPrivilegedStartCommand` (`sh -n`, прогон с поддельным ядром, `BASH_FUNC_echo%%` и `PATH` не доходят).

## Этап 4 · Диалог
- [ ] `dialogs.ShowCommandRetry`: Copy the command, Run in Terminal, Retry, Close.
- [ ] Отказ гейта: WARN с обоими sha, диалог вместо «Failed to start».
- [ ] После обновления ядра: WARN с обоими sha, если копия отстаёт и службы нет.
- [ ] Ключи `locale.T` с переводом в `bin/locale/ru.json`, `l10n_check --strict` без сирот.
- [ ] Показать владельцу.

## Этап 5 · Доки
- [ ] `docs/DAEMON_AND_REMOTE.md` / `.ru.md` — classic TUN на той же копии.
- [ ] `docs/ARCHITECTURE.md` / `.ru.md` — привилегированный старт classic.
- [ ] `docs/ARCHITECTURE_PACKAGES.md` / `.ru.md` — новые файлы.
- [ ] `docs/release_notes/upcoming.md` EN/RU, выжимка в `RELEASE_NOTES.md`.
- [ ] Ссылка из SPEC 136; запись в `SPECS/README.md`.

## Приёмка
- [ ] Ядро lx.11 с `--service=copy`; ручная проверка SPEC §9 на Mac владельца.
- [ ] CI `run_mode=tests` зелёный.
- [ ] Решение владельца по развилке А/Б (SPEC §6).
- [ ] `RequiredCoreVersion` → lx.11 — вместе с SPEC 136, при релизе.
