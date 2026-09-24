# SPEC 136 · Задачи

Отмечать по факту коммита. Этапы — PLAN.md.

## Этап 1 · Спека
- [x] SPEC.md, PLAN.md, TASKS.md.

## Этап 2 · Классификатор
- [x] Константы раскладки lx.11: каталог службы, `sing-box`, `install.json`, корень цепочки `/Library` (24.09 — заменено плоским файлом `/Library/PrivilegedHelperTools/com.leadaxe.sing-box-lxd` и сайдкаром `….install.json`, коммит в ветке SPEC 137).
- [x] `inspectDaemonServiceDefinition`: NotInstalled / Unsafe (plist, путь, цепочка `Lstat`).
- [x] `compareDaemonServiceFiles`: Stale по sha (кэш `(dev, inode, size, mtime)`), отсутствующая копия.
- [x] `compareDaemonServiceProcess`: ProcessStale по `executable_sha256`/`executable`, запасной путь по версии.
- [x] Сайдкар — только для показа версии.
- [x] `lxdclient.InfoData`: `executable`, `executable_sha256`.
- [x] `TestDaemonServiceClassifier`.

## Этап 3 · Команды
- [x] `daemonServiceCommand` — одно место квотинга; install всегда от `SingboxPath`.
- [x] Uninstall и `client add` — через копию, если она безопасна.
- [x] Диалог после обновления ядра: условие «plist есть», команда install.
- [x] `appleScriptString` вынесен из `OpenTerminalWithCommand`.
- [x] `TestDaemonServiceCommandQuoting` (пробел и `'` в пути, `sh -n`, разбор аргументов, литерал AppleScript).

## Этап 4 · Снимок и UI
- [x] `DaemonUIStatus.Service` вместо сравнения путей.
- [x] Debug API `/daemon/status`: `service_state`, `service_path`, `service_detail`.
- [x] WARN перед apply.
- [x] Плашка Unsafe/Stale/ProcessStale на вкладке Status, kickstart-строка убрана.
- [x] Install, шаг 1 — «Install or update the service».
- [x] Текст «ядро без lxd» без номера релиза.
- [x] Модальное предупреждение раз на версию лаунчера (`daemon_unsafe_notice_version`).
- [x] Ключи `locale.T` с переводом в `bin/locale/ru.json`, сироты сняты.
- [ ] Показать владельцу.

## Этап 5 · Очистка
- [x] Подсказка удаления службы через копию (диалог и `-purge-data`), текст «служба переживает удаление».

## Этап 6 · Доки
- [x] `docs/DAEMON_AND_REMOTE.md` / `.ru.md` — root-owned копия и одна команда.
- [x] `docs/API.md` / `.ru.md` — поля `/daemon/status`.
- [x] `docs/ARCHITECTURE_PACKAGES.md` / `.ru.md` — новый файл.
- [x] `docs/release_notes/upcoming.md` EN/RU (строка SPEC 135 про сверку путей заменена), выжимка в `RELEASE_NOTES.md`.
- [x] Ссылка из SPEC 135 §5.1; запись в `SPECS/README.md`.

## Приёмка
- [ ] Ядро lx.11 собрано; ручная проверка SPEC §9 на Mac владельца.
- [ ] CI `run_mode=tests` зелёный (macOS: `sh -n`, `osascript`).
- [ ] `RequiredCoreVersion` → lx.11 — отдельным коммитом при релизе.
