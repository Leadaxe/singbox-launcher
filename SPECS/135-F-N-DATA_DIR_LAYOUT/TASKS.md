# SPEC 135 · Задачи по этапам

Отмечать по факту коммита. Номера этапов — PLAN.md.

## Этап 0 · CODEMAP
- [x] `CODEMAP.md`: хелперы путей с предлагаемым типом аргумента; все обращения к `ExecDir` по файлам с классификацией; швы; поставляемое vs скачанное; тесты под удар; инструменты для этапов 7–9.

## Этап 1 · `internal/paths`
- [x] Типы `AppDir`, `DataDir`, `LogDir`, `Mode`, `Layout`.
- [x] `Resolve(exe, env, goos, probe)` по §3.1–3.2, включая фоллбэки `%LOCALAPPDATA%` и отключение правил 2–3 для macOS `.app`.
- [x] `Executable()` с `EvalSymlinks`, `ProbeWritable`, `IsAppBundle`, `LogLine()`.
- [x] Константы: `portable.txt`, имя каталога, имена переменных окружения, `.migrated_from`.
- [x] `TestResolveMatrix`.

## Этап 2 · Литералы
- [x] `GetRuleSetPath`, `GetTailscaleStateDir` в `platform_common.go`.
- [x] `preset_merge.go:58,760`, `file_service.go:94`, `core_dashboard_tab_status.go:254`, `controller.go:306` и всё найденное грепом.
- [x] golden-тест по имени зелёный.

## Этап 3 · Ядро раскладки
- [x] `main()`: `paths.Executable` + `Resolve` первым; `crash.log`/`native-stderr.log` в `LogDir`; `RunGLProbeChild` получает `AppDir`; ошибка резолва — понятный текст и выход.
- [x] `platform_common.go`: все хелперы на именованные типы; `EnsureDirectories(Layout)` создаёт только `Data/bin`, `Data/bin/rule-sets`, `Logs`.
- [x] `glstate.go`: `gl-state.json` от `DataDir`; Mesa-функции от `AppDir` с комментарием-исключением.
- [x] `FileService`: поле `Layout`, `ExecDir` удалён; `OpenLogFiles`/`ReopenChildLogFile` от `LogDir`; `WintunPath` временно от `Data` (до этапа 5).
- [x] `NewAppController(layout)`.

## Этап 4 · Прокидка
- [x] `core/*` по CODEMAP: каждое обращение получает `Data`/`App`/`Logs`.
- [x] `wizard_model.ExecDir` → `DataDir`; `FileServiceInterface.ExecDir()` → `Layout()`.
- [x] `debugapi.ControllerFacade.GetExecDir()` → `GetLayout()` (снапшоту нужны шаблон и state); `remote_endpoints.ExecDir` → `DataDir`.
- [x] Локали: `LoadExternalLocales(App/bin/locale)` затем `LoadExternalLocales(Data/bin/locale)`; скачивание локалей — в `Data`.
- [x] Diagnostics: Mesa-кнопки недоступны, когда `ProbeWritable(App)` ложь.
- [x] Тесты из CODEMAP §5 переведены на новые сигнатуры (без новых тестов).
- [ ] Страж «AppDir не пишем»: `tools/paths_guard` (AST-скан по образцу `tools/l10n/l10n_check/scan.go:131`) с поимённым исключением Mesa; запуск в CI-lint.

## Этап 5 · Двухуровневое чтение
- [x] Ядро: `SINGBOX_LAUNCHER_CORE` → `Data/bin` → `App/bin` → `PATH`; лог обеих версий и `shadowed`.
- [x] `WintunPath = Dir(SingboxPath)/wintun.dll`; проверка/скачивание wintun с этим каталогом; сообщение при read-only каталоге.
- [x] Шаблон: правило маркера (§3.3); `RefreshTemplateIfStale` читает маркер из `App`, штамп и скачивание в `Data`; все места загрузки шаблона через один резолвер.
- [x] `settings.json`: `config_data_root`; старт сравнивает с `Data` → `MarkConfigStale`; успешная сборка пишет штамп.
- [x] `core/template_migration_test.go` по имени зелёный.

## Этап 6 · Миграция
- [x] `internal/paths/copytree.go`: временный каталог → права → пропуск нечитаемого с подсчётом → rename.
- [x] `internal/paths/migrate.go`: условие (§3.4), `.migrated_from` последним, одна строка в лог.
- [x] Вызов в `NewFileService`/`main` до `EnsureDirectories`.
- [x] Одноразовое уведомление «данных предыдущей версии не найдено» (флаг в `settings.json`).
- [x] Интеграционный тест миграции (§8).

## Этап 7 · Storage и пути
- [x] `ui/settings_storage.go`: раздел Storage, строки Mode/Program/Data/Logs/Core/Template/wintun, кнопки Open, Copy paths.
- [x] `paths.Describe`-подобный блок общий для лога, UI, `-paths`, `/debug/paths`.
- [x] Флаг `-paths` в `main.go`.
- [x] `GET /debug/paths` + манифест.
- [x] Diagnostics «Logs folder»/«Config folder» → `Logs`/`Data`.
- [ ] Показать владельцу.

## Этап 8 · Переключатель Portable
- [ ] Чекбокс с условиями недоступности (§4.2), скрыт на macOS.
- [ ] `core/storage_switch.go`: включить/выключить по §4.2 (копия → маркер → удаление старого → `RestartSelf`), остаток при отказе удаления → `bin.moved-<дата>`.
- [ ] Интеграционный тест переключения (§8).

## Этап 9 · Очистка
- [ ] `core/purge.go`: план (пути, размеры, лишнее), выполнение по порядку §4.3, сетевая очистка.
- [ ] Диалог «Remove all data…» с чекбоксами; команда `--service=uninstall --purge` при установленном plist.
- [ ] Флаг `-purge-data [-yes]`.
- [ ] Интеграционный тест `-purge-data`.

## Этап 10 · macOS
- [ ] `build_darwin.sh`: без переноса `bin`/`logs`, `codesign --verify` в строю, `-i` = замена бинаря.
- [ ] `DaemonStatusSnapshot`: `ProgramArguments[0]` из plist против `SingboxPath`, статус и команда `--service=install`.
- [ ] Проверка владельцем: `-i`, daemon-режим, sudo-команды.

## Этап 11 · Windows
- [ ] `portable.txt` в `win64`, `win64-full`, `win7-32`.

## Этап 12 · Закрытие
- [ ] `docs/ARCHITECTURE.md`, `docs/release_notes/upcoming.md` (EN/RU; порядок поиска ядра на Linux; `setcap` и `nosuid`), README про пути и очистку, `RELEASE_NOTES.md`.
- [ ] `bin/locale/ru.json`: переводы всех новых ключей.
- [ ] SPEC 022 и 080 → `…-C-…` со ссылкой на 135; `SPECS/README.md`.
- [ ] Ответ в #85, ссылка из #99.
- [ ] CI dispatch зелёный; слияние в `develop`.
