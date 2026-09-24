# SPEC 136 · Задачи

Отмечать по факту коммита. Этапы — PLAN.md.

## Этап 1 · Спека
- [x] SPEC.md, PLAN.md, TASKS.md.

## Этап 2 · Классификатор
- [ ] Константы раскладки lx.11: каталог службы, `sing-box`, `install.json`, корень цепочки `/Library`.
- [ ] `inspectDaemonServiceDefinition`: NotInstalled / Unsafe (plist, путь, цепочка `Lstat`).
- [ ] `compareDaemonServiceFiles`: Stale по sha (кэш `(dev, inode, size, mtime)`), отсутствующая копия.
- [ ] `compareDaemonServiceProcess`: ProcessStale по `executable_sha256`/`executable`, запасной путь по версии.
- [ ] Сайдкар — только для показа версии.
- [ ] `lxdclient.InfoData`: `executable`, `executable_sha256`.
- [ ] `TestDaemonServiceClassifier`.

## Этап 3 · Команды
- [ ] `daemonServiceCommand` — одно место квотинга; install всегда от `SingboxPath`.
- [ ] Uninstall и `client add` — через копию, если она безопасна.
- [ ] Диалог после обновления ядра: условие «plist есть», команда install.
- [ ] `appleScriptString` вынесен из `OpenTerminalWithCommand`.
- [ ] `TestDaemonServiceCommandQuoting` (пробел и `'` в пути, `sh -n`, разбор аргументов, литерал AppleScript).

## Этап 4 · Снимок и UI
- [ ] `DaemonUIStatus.Service` вместо сравнения путей.
- [ ] Debug API `/daemon/status`: `service_state`, `service_path`, `service_detail`.
- [ ] WARN перед apply.
- [ ] Плашка Unsafe/Stale/ProcessStale на вкладке Status, kickstart-строка убрана.
- [ ] Install, шаг 1 — «Install or update the service».
- [ ] Текст «ядро без lxd» без номера релиза.
- [ ] Модальное предупреждение раз на версию лаунчера (`daemon_unsafe_notice_version`).
- [ ] Ключи `locale.T` с переводом в `bin/locale/ru.json`, сироты сняты.
- [ ] Показать владельцу.

## Этап 5 · Очистка
- [ ] Подсказка удаления службы через копию (диалог и `-purge-data`), текст «служба переживает удаление».

## Этап 6 · Доки
- [ ] `docs/DAEMON_AND_REMOTE.md` / `.ru.md` — root-owned копия и одна команда.
- [ ] `docs/API.md` / `.ru.md` — поля `/daemon/status`.
- [ ] `docs/ARCHITECTURE_PACKAGES.md` / `.ru.md` — новый файл.
- [ ] `docs/release_notes/upcoming.md` EN/RU.
- [ ] Ссылка из SPEC 135 §5.1; запись в `SPECS/README.md`.

## Приёмка
- [ ] Ядро lx.11 собрано; ручная проверка SPEC §9 на Mac владельца.
- [ ] CI `run_mode=tests` зелёный (macOS: `sh -n`, `osascript`).
- [ ] `RequiredCoreVersion` → lx.11 — отдельным коммитом при релизе.
