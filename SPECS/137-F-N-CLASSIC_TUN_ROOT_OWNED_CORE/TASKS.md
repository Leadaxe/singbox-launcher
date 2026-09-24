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
- [x] Старт: `/usr/bin/env -i PATH=… /bin/sh -c <тело-константа> start-singbox-privileged <аргументы>`; скрипт в DataDir больше не пишется, старый удаляется.
- [x] Stop/рестарт: `/bin/kill -TERM` по PID, pid-файл удаляет лаунчер.
- [x] pkill (диалог «already running», Diagnostics): `/usr/bin/pkill` без шелла.
- [x] Снятие TUN: `/bin/rm -rf --` без шелла (в 137.1 заменено удалением без root).
- [x] `privilegedAuthReuse` (вариант А; Б — `false`).
- [x] `TestPrivilegedStartCommand` (`sh -n`, прогон с поддельным ядром, `BASH_FUNC_echo%%` и `PATH` не доходят).

## Этап 4 · Диалог
- [x] `dialogs.ShowCommandRetry`: Copy the command, Run in Terminal, Retry, Close.
- [x] Отказ гейта: WARN с обоими sha, диалог вместо «Failed to start».
- [x] После обновления ядра: WARN с обоими sha, если копия отстаёт и службы нет.
- [x] Ключи `locale.T` с переводом в `bin/locale/ru.json`, `l10n_check --strict` без сирот.
- [ ] Показать владельцу.

## Этап 5 · Доки
- [x] `docs/DAEMON_AND_REMOTE.md` / `.ru.md` — classic TUN на той же копии.
- [x] `docs/ARCHITECTURE.md` / `.ru.md` — привилегированный старт classic.
- [x] `docs/ARCHITECTURE_PACKAGES.md` / `.ru.md` — новые файлы.
- [x] `docs/release_notes/upcoming.md` EN/RU, выжимка в `RELEASE_NOTES.md`.
- [x] Ссылка из SPEC 136; запись в `SPECS/README.md`.

## Приёмка
- [x] Ядро lx.11 с `--service=copy` — в ветке форка `27e245e2e` (интерфейс SPEC §10).
- [ ] Релиз lx.11; ручная проверка SPEC §9 на Mac владельца.
- [ ] CI `run_mode=tests` зелёный.
- [ ] Решение владельца по развилке А/Б (SPEC §6).
- [ ] `RequiredCoreVersion` → lx.11 — вместе с SPEC 136, при релизе.

## Попутно (SPEC 136, по сообщению главной сессии)
- [x] Пустой `executable_sha256` — «неизвестно», судит версия (lx.11 считает хэш в фоне); тест классификатора дополнен.
- [x] Коды выхода `--service=status` lx.11 (3 — NOT INSTALLED, 4 — COPY ONLY) в SPEC 136 и `docs/DAEMON_AND_REMOTE*.md`.

## 137.1 · Root не пишет по путям пользователя (решение координатора)
- [x] Вывод ядра под root — `/Library/Logs/sing-box-lxd/classic.log`: каталог и файл создаёт, проверяет (не симлинк, владелец, тип через `/usr/bin/stat -f '%u:%HT'`) и ротирует постоянное тело; отказ — причина вместо PID.
- [x] `AppController.CoreLogPath()`; Core-вкладка логов и тейлер профайлера трафика (`StartFollowing`) следуют за ним.
- [x] Снятие TUN без root: `removeTunLeftover` своим uid (внутри DataDir/LogDir, не симлинк); `RemoveWithPrivileges` удалён.
- [x] `TestPrivilegedStartCommand` — новый лог, ротация, отказы по симлинку и владельцу.
- [x] Уровень лога по умолчанию `warn`, debug/trace — только явно; stderr/паники — известный риск (SPEC §8).
- [x] Аудит мест исполнения команд — SPEC §12.
- [x] fix(136): Uninstall службы с `--keep-copy`, «Remove all data…» — полный uninstall.
- [x] Доки и заметки (Windows — «следующим»).
- [x] Права `classic.log` — пользователю лаунчера, `0600` (решение координатора): uid аргументом, проверка в теле (цифры, ≥ 501, `/usr/bin/id`), `chown`/`chmod` файла и `.old`; тест — невалидные uid и владелец/права.
- [x] fix(136): служба не запущена — состояние NotRunning и команда `launchctl bootstrap`, не install.
- [x] fix(136/137): копия — плоский файл `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd` (заменено следующим пунктом), сайдкар `….install.json` (решение владельца 24.09); каталог на месте копии — Unsafe «legacy layout, remove it»; процесс копии зовётся `com.leadaxe.sing-box-lxd` — шаблон pgrep/pkill и проверка pid-файла это учитывают; тесты классификатора, гейта и `TestPrivilegedStartCommand` (pgrep находит копию).
- [x] fix(136/137): копия называется `sing-box-lxd` (`/Library/PrivilegedHelperTools/sing-box-lxd`, сайдкар `sing-box-lxd.install.json`, ядро lx.12; решение владельца 24.09); `PrivilegedPkillPattern` = `sing-box run|sing-box-lxd run|start-singbox-privileged`; раскладки dev-сборок lx.11 (`com.leadaxe.sing-box-lxd/` и плоский `com.leadaxe.sing-box-lxd`) — legacy с советом `sudo rm -rf`; тесты классификатора, гейта и старта.
