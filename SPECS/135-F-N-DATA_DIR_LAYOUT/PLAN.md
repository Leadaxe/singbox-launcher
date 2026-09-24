# SPEC 135 · План реализации

Норма — SPEC.md. Здесь: где живёт каждый этап, что от чего зависит, как
проверяется. Карта обращений к путям — CODEMAP.md (строится этапом 0 и
поддерживается каждым этапом: правишь код — правишь строку карты).

Ветка работы: `spec-135-impl` (worktree от `develop`). Слияние в `develop`
после этапа 12 и зелёного CI.

## Архитектурные решения

| Решение | Где | Почему |
|---|---|---|
| Резолвер раскладки — пакет-лист `internal/paths` без platform/debuglog | §5 | импортируется из `main`, тестов и `internal/platform` без циклов |
| Именованные типы `AppDir`/`DataDir`/`LogDir` — единственная защита от подмены | §5 | компилятор ловит `AppDir` в пишущем хелпере на ~190 местах |
| `Layout` считается в `main()` первым и идёт значением в `NewAppController(layout)` | §3.2, §5 | `crash.log` и `RunGLProbeChild` нужны раньше контроллера |
| `platform_common.go` остаётся местом всех хелперов путей, но их аргументы — именованные типы | §5 | не плодить второй набор хелперов |
| `FileService` получает `Layout` и теряет `ExecDir`; появляются `App()`, `Data()`, `Logs()` | §5 | два шва (`wizard_model`, `debugapi`) закрываются на уровне полей |
| Один копировщик каталогов (`internal/paths/copytree.go`) для миграции и переключателя | §3.4, §4.2 | временный каталог, права, пропуск нечитаемого, rename в конце |
| Штамп `config_data_root` в `settings.json` | §3.5 | любая смена DataDir → `MarkConfigStale`, не только macOS |
| Спутники ядра от каталога выбранного ядра | §3.3 | wintun/libcronet ищет загрузчик рядом с `sing-box`, не с лаунчером |

## Этапы

| # | Что | Пакеты / файлы | Зависит от | Проверка |
|---|---|---|---|---|
| 0 | CODEMAP | `SPECS/135…/CODEMAP.md` | — | — |
| 1 | `internal/paths`: типы, `Resolve`, `ProbeWritable`, `Executable`, `LogLine` | `internal/paths/*.go`, константы в `internal/constants` | — | `TestResolveMatrix` |
| 2 | Литералы путей → хелперы, поведение прежнее | `core/build/preset_merge.go`, `core/services/file_service.go`, `ui/core_dashboard_tab_status.go`, `core/controller.go`, новые хелперы в `platform_common.go` | — | `go build`, golden-тест по имени |
| 3 | `main()` → `Resolve` первым; `platform_common.go` на именованные типы; `EnsureDirectories(Layout)`; `FileService(Layout)`; логи от `LogDir` | `main.go`, `internal/platform/*.go`, `core/services/file_service.go`, `core/controller.go` | 1, 2 | `go build` (компилятор ведёт) |
| 4 | Прокидка по слоям: `core/*`, `wizard_model`, `debugapi` facade, Mesa как явное исключение, две папки локалей | по CODEMAP | 3 | `go build`, правка сломанных тестов на новые сигнатуры |
| 5 | Двухуровневое чтение шаблона/ядра, спутники от ядра, `RefreshTemplateIfStale` на два каталога, штамп `config_data_root` | `core/template_migration.go`, `core/template/*`, `core/config/varsubst.go`, `core/services/file_service.go`, `internal/locale/settings.go`, `core/rebuild.go` | 4 | `go build`; существующий `core/template_migration_test.go` по имени |
| 6 | Копировщик, миграция, одноразовое уведомление | `internal/paths/copytree.go`, `internal/paths/migrate.go`, `core/controller.go`, `main.go` | 4 | интеграционный тест миграции |
| 7 | Storage: таблица путей, Open, Copy paths; `-paths`; `GET /debug/paths`; Diagnostics на LogDir/DataDir | `ui/settings_tab.go` (новый файл `ui/settings_storage.go`), `main.go`, `core/debugapi/paths_endpoint.go`, `ui/diagnostics_tab.go` | 5 | `go build`, показать владельцу |
| 8 | Переключатель Portable | `ui/settings_storage.go`, `core/storage_switch.go` | 6, 7 | интеграционный тест переключения |
| 9 | Очистка: диалог, сетевая очистка, `-purge-data [-yes]` | `ui/settings_storage.go`, `core/purge.go`, `main.go` | 6, 7 | интеграционный тест `-purge-data` |
| 10 | `build_darwin.sh`; демон: сверка plist с `SingboxPath` | `build/build_darwin.sh`, `core/daemon_manager_darwin.go`, `ui/connection_local_daemon_darwin.go` | 8 | руками владельцем |
| 11 | `portable.txt` в три zip, CI | `.github/workflows/ci.yml` | 8 | CI dispatch |
| 12 | Доки, заметки релиза, ru.json, закрытие 022/080, ответ в #85 | `docs/*`, `RELEASE_NOTES.md`, `bin/locale/ru.json`, `SPECS/README.md` | всё | — |

Порядок: 0 ∥ 1 ∥ 2 → 3 → 4 → 5 → 6 → 7 → 8 ∥ 9 → 10 ∥ 11 → 12.

## Политика проверок

Локально: `go build ./...` после каждого этапа; из тестов — только новые
(три интеграционных: миграция, переключатель, очистка) и существующие,
сломанные сменой сигнатур, по имени. Полный прогон — CI
(`gh workflow run ci.yml --ref <ветка> -f run_mode=tests`) после этапа 6 и
после этапа 12. UI (этапы 7–9) — сборка и показ владельцу, тестов нет.

## Границы

Не делаем: относительные пути `.srs`, переименование `bin/`, установщик
Windows (#99), пакеты Linux (#103), удаление exe.
