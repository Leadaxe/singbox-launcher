# SPEC 136 · План реализации

Норма — SPEC.md. Ветка — `spec-136-daemon-root-owned` (worktree от
`develop`), не пушится до команды владельца.

## Архитектурные решения

| Решение | Почему |
|---|---|
| Классификатор — чистая функция от путей (`plist`, каталог службы, корень цепочки, ядро лаунчера) и ожидаемого uid | тест строит раскладку во временном каталоге от своего uid; прод — `/Library/…`, uid 0 |
| Два шага: определение службы (plist + инвариант, без хэшей) и сверка файлов/процесса | стартовому предупреждению и командам Uninstall/pair хватает первого шага — без чтения десятков мегабайт |
| Кэш sha256 по `(dev, inode, size, mtime)` — один экземпляр на пакет с мьютексом | снимок статуса и apply зовут классификатор часто; замена файла меняет ключ |
| Вердикт процесса — из паспорта `/admin/info`, который снимок и apply уже запрашивают | ни одного нового сетевого вызова |
| Команды собирает одна функция `daemonServiceCommand(bin, args…)` | одно место квотинга, тест на него |
| Модальное предупреждение — из `main.go` через `whenWindowVisible`, как уведомления SPEC 135 | одна механика «показать, когда окно видно» |

## Файлы

| Файл | Что |
|---|---|
| `core/daemon_service_state_darwin.go` (новый) | константы раскладки lx.11, `DaemonServiceState`/`DaemonServiceCheck`, `inspectDaemonServiceDefinition`, `compareDaemonServiceFiles`, `compareDaemonServiceProcess`, кэш sha, чтение сайдкара, `daemonServiceBinary` |
| `core/daemon_service_state_test.go` (новый, darwin) | `TestDaemonServiceClassifier`, `TestDaemonServiceCommandQuoting` |
| `core/daemon_manager_darwin.go` | `DaemonUIStatus.Service` вместо `ServiceCorePath`/`ServiceCoreMismatch`; снимок; команды через `daemonServiceCommand`; Uninstall/pair через копию; диалог после обновления ядра — install; `appleScriptString` вынесен из `OpenTerminalWithCommand` |
| `core/backend_daemon_darwin.go` | WARN перед apply, если служба не OK |
| `core/backend_daemon_stub.go` | заглушка `DaemonUnsafeServiceNotice` |
| `core/purge.go`, `core/purge_darwin.go`, `core/purge_other.go` | подсказка удаления службы через копию, признак «переживает очистку» |
| `core/debugapi/*.go`, `core/debugapi_wiring_daemon_darwin.go` | `service_state`, `service_path`, `service_detail` в `/daemon/status` |
| `internal/lxdclient/client.go` | `InfoData.Executable`, `InfoData.ExecutableSHA256` |
| `internal/locale/settings.go` | `daemon_unsafe_notice_version`, `MarkDaemonUnsafeNoticeShown` |
| `main.go` | модальное предупреждение при Unsafe |
| `ui/connection_local_daemon_darwin.go` | плашка по состояниям, шаг 1 Install, текст про ядро без lxd, kickstart-строка убрана |
| `ui/settings_purge.go` | текст подсказки по признаку «переживает очистку» |
| `bin/locale/ru.json` | новые ключи, сироты сняты |
| `docs/DAEMON_AND_REMOTE*.md`, `docs/API*.md`, `docs/ARCHITECTURE_PACKAGES*.md`, `docs/release_notes/upcoming.md`, `SPECS/README.md`, SPEC 135 §5.1 | документация |

## Этапы (коммиты)

| # | Что | Проверка |
|---|---|---|
| 1 | SPEC/PLAN/TASKS | — |
| 2 | Классификатор, кэш sha, поля паспорта, тест классификатора | `go build ./...`, `go test ./core/ -run TestDaemonServiceClassifier` |
| 3 | Команды (одна install, Uninstall/pair через копию), диалог после обновления ядра, тест квотинга | `go test ./core/ -run TestDaemonServiceCommandQuoting` |
| 4 | Снимок, Debug API, WARN перед apply, UI-плашка, модальное предупреждение, locale | `go build ./...`, `l10n_check --strict`, показать владельцу |
| 5 | Подсказка очистки через копию | `go build ./...` |
| 6 | Доки и заметки | — |
