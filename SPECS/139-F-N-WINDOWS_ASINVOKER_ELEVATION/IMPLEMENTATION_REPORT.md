# SPEC 139 · Отчёт о реализации

Дата: 24.09.2026. Ветка `spec-139-asinvoker` от `develop` `879f276e`. Код
готов, CI зелёный (раздел «Проверки»); ручная приёмка §11 на Windows не
проводилась.

## Что сделано

**Платформа** (`internal/platform/elevation*.go`, `autostart*.go`).
`IsElevated` — `TokenElevation` или членство в `BUILTIN\Administrators`
(`Token(0).IsMember`: CheckTokenMembership требует токен олицетворения, нулевой
токен берёт его из процесса), один раз на процесс. `ElevationAsksOtherAccount` —
`TokenElevationTypeDefault` без повышения. `RunElevated(exe, args, dir, show)` —
`ShellExecuteExW` через `shell32.dll`, `runas`,
`SEE_MASK_NOCLOSEPROCESS|SEE_MASK_NOASYNC|SEE_MASK_FLAG_NO_UI`, владелец окна UAC —
`GetForegroundWindow`, закреплённый поток с `CoInitializeEx(STA|DISABLE_OLE1DDE)`;
возвращает `*ElevatedProcess{Pid}` с `Wait(timeout) (exitCode, exited, err)` и
`Close()` — ровно то, что нужно SPEC 141 §5.2 (`SW_HIDE`, ожидание 120 с, код
выхода). Отказ в UAC — `ErrElevationCancelled`. `WaitForProcessExit(pid, exe,
timeout)` — `OpenProcess(SYNCHRONIZE|PROCESS_QUERY_LIMITED_INFORMATION)`, сверка
образа `QueryFullProcessImageName`, `WaitForSingleObject`; отказ доступа — опрос
списка процессов раз в 250 мс. Автозапуск — значение `singbox-launcher` в
`HKCU\…\Run` (`x/sys/windows/registry`). Всё — из `x/sys` v0.25.0, без
`min`/`max`/`slices`/`maps` (`win7guard` чист).

**Раскладка** (`internal/paths`). `AppDirUserWritable` — проба и защищённые
каталоги Windows (`ProgramFiles`, `ProgramFiles(x86)`, `ProgramW6432`,
`SystemRoot` из `env` резолвера; без регистра, по границе каталога, оба
разделителя — решение не зависит от ОС теста). Правило 2 и правило 3, Windows-фоллбэк
и `SystemDefault` — на предикате; маркер в непишущемся AppDir —
`Layout.MarkerIgnored` (WARN при старте, `, portable.txt ignored` в `LogLine`,
`(portable.txt ignored)` в строке Mode для `-paths`, `/debug/paths`, Copy paths,
локализованная пометка в Storage). `Layout.Handoff(pid)` / `ParseHandoff`. План
очистки: остатки под AppDir без пробы записи — `NeedsAdmin` (сняты, пропуск,
код выхода 0).

**Старт** (`main.go`). `flag.Parse` до раскладки; `-handoff` (раскладка из
флага, невалидное — stderr и `Resolve`), затем `-paths`, `-purge-data`,
`-autostart=on|off` (без GUI, код 0/1) и ожидание родителя 25 с до crash-лога,
GL-пробы и контроллера; WARN о таймауте — после открытия логов. Первая
WARN-строка — `elevated=yes|no`; заголовок окна повышенного экземпляра —
`Singbox Launcher (Administrator)` (слово локализуется). Манифест — `asInvoker`
(`-manifest` в CI и `build_windows.bat` остаётся, комментарий в `ci.yml`
переписан).

**Гейты** (`core/elevation.go` и места §6). TUN — в `ProcessService.Start` сразу
после пересборки: диалог `internal/dialogs.ShowActions` (новый примитив: список
действий, подсказки недоступных, строка статуса; `ShowCommandRetry` не подошёл —
у него нет статуса и набора кнопок). Restart as administrator — аргументы через
`flag.Visit`, `RunElevated`, WARN `restart as administrator: started pid N,
exiting` и `GracefulExit` на UI-потоке. Switch to proxy mode — общий хелпер записи
vars (`setLocalStateVars`, им же теперь пользуется `ApplyLogLevelAndReloadCore`)
→ `RebuildConfigIfDirty(true)` → `StartSingBoxProcess`. Без прав: очистка при
старте — одна строка INFO, после Stop — Debug, сетевая очистка в Remove all data
— пункт снят с подсказкой, в `-purge-data` — строка `Network cleanup: skipped
(needs administrator)…` с командой, Kill — сообщение с Restart as administrator
без сброса `RunningState`, Portable — предикат и блок в повышенном экземпляре.
Тексты, исходившие из «всегда админ», переписаны (§6 п. 10).

**Автозапуск** (`core/autostart.go`, `ui/settings_tab.go`). Два чекбокса в
Connection (только Windows), подсказка «another copy», блок в повышенном
экземпляре; `-autostart`; удаление в Remove all data (свой чекбокс) и
`-purge-data` — только значение, указывающее на этот exe.

**Доки.** `upcoming.md` (EN Highlights/Technical, RU), выжимка в
`RELEASE_NOTES.md` (раздел «ветка, версия не назначена», как у 136/137),
`README*.md` (распаковка в пишущуюся папку, TUN и права, автозапуск, очистка без
прав, строка таблицы проблем), тело релиза в `ci.yml`, `ARCHITECTURE*.md`
(§7a.2, §7a.4, новый §11.7), `ARCHITECTURE_PACKAGES*.md`, `BUILD_WINDOWS*.md`,
`TROUBLESHOOTING*.md` (раздел Windows), ссылки из SPEC 135 §3.5 и 137 §8 п. 5,
`SPECS/README.md`.

## Отступления от SPEC/PLAN

1. **Два новых общих файла вне списка PLAN:** `internal/platform/elevation.go`
   (сентинелы, константы `ElevatedShowNormal/Hidden`, опрос списка процессов —
   общий для Windows-фоллбэка и не-Windows) и `internal/platform/autostart.go`
   (формат значения `"<exe>" -tray [-start]` и его разбор). Иначе объявления
   дублировались бы в `_windows` и `_other`.
2. **`SEE_MASK_FLAG_NO_UI`** добавлен к маске §5 п. 3: ошибку показывает наш
   диалог, второе системное окно не нужно. На UAC не влияет.
3. **Лишний ключ перевода** `portable.txt ignored` → «portable.txt не
   учитывается» (строка Mode в Storage); в таблице §4 его нет.
4. **Restart as administrator из сообщения Kill** не добавляет `-start`:
   пользователь не нажимал Start, повышенный экземпляр покажет «already running»
   и снимет ядро по Kill (§11 п. 5). Из диалога TUN — с `-start`, как в §5.
5. **`-purge-data` без прав сохраняет `<Data>/.migrated_from`**, если источник
   миграции в защищённом AppDir пропущен: иначе повторная команда из консоли
   администратора (§11 п. 6, «доделывает») не нашла бы источник. В режиме Env
   маркер убирается из списка поштучно удаляемых файлов.
6. **`ParseHandoff`** восстанавливает `MarkerIgnored` по диску (System и маркер
   рядом с exe), `EnvSource` не передаётся (формат флага — ровно четыре поля).
7. **`WaitForProcessExit` вне Windows** — опрос списка процессов, а не «not
   supported»: `-handoff` там не передаётся, но ручной запуск с флагом не
   ломается.
8. **Kill-сообщение** — заголовок `Warning` (ключ уже есть), кнопка Close.
9. **Неиспользуемый `AppController.NetworkCleanup()`** заменён на
   `NetworkCleanupNeedsAdmin()`; `ExecutePurgeAndExit` получил параметр
   `autostart`.
10. **Тест.** Новых тестовых функций нет: строки `TestResolveMatrix` (защищённые
    каталоги всех четырёх переменных, регистр, граница каталога, маркер в
    непишущемся каталоге на Linux и Windows, Linux игнорирует защищённые
    каталоги, фоллбэк без `LOCALAPPDATA`) и подблок `handoff` (разбор, обратный
    ход, `MarkerIgnored`, восемь невалидных значений). Две существующие строки
    получили `probe: true` — правило 2 теперь пробует запись.
11. **`RELEASE_NOTES.md`** — раздел ветки без номера версии, как у 136/137;
    итоговую выжимку версии пишет релиз.
12. **Рефактор `core/log_level.go`.** Запись переменных в state локального
    профиля вынесена в общий `setLocalStateVars` (`core/elevation.go`):
    Switch to proxy mode пишет три переменные тем же путём, что
    `ApplyLogLevelAndReloadCore` — одну. Поведение уровня лога не менялось.
13. **`ui/settings_storage.go` вне списка PLAN.** Строка Mode в Settings →
    Storage строится там (`storageModeText`), и пометка
    `portable.txt ignored` (§7) иначе в интерфейс не попадала бы.
14. **Cancel слева.** В §4 Cancel — последняя кнопка; диалог собран через
    `dialogs.NewCustom`, где кнопка закрытия по конвенции проекта стоит слева,
    действия — справа в порядке списка. На экране с RU проверить, что ряд
    «Отмена · Перезапустить от имени администратора · Перейти в режим прокси»
    не переносится; с четвёртой кнопкой Install service из SPEC 141 диалог
    станет шире.

## Проверки

- Локально: `go build ./...`; `GOOS=windows GOARCH=amd64|386 CGO_ENABLED=0 go vet`
  для `internal/platform`, `internal/paths`, `internal/process` (UI и `core` под
  Windows без cgo не собираются); `go test ./internal/paths -run
  TestResolveMatrix`; `win7guard`, `paths_guard --strict`, `l10n_check --strict`,
  `hardcoded_check --strict` — чисто.
- CI `run_mode=tests` — run 36023432577: Test на macOS, Ubuntu, Windows — зелёный.
- CI `run_mode=build` — run 36023446055: Test ×3, Build Windows (Win64), Build
  Windows 7 (x86, legacy, go1.20 + x/sys v0.25.0), Build macOS — зелёный.

## Не проверено (ручная приёмка SPEC §11)

Вся Windows-специфика проверена только сборкой (CI win64 и win7-32) и
`GOOS=windows go vet` пакетов без cgo; UI-пакет под Windows локально не
собирается. На машине с Windows 10/11 и Win7 SP1 — все пункты §11, в первую
очередь: манифест и `elevated=no` (п. 1), отмена/успех UAC и одна иконка трея,
тот же Data и порт Debug API после перезапуска (п. 3), сирота с правами (п. 5),
миграция из Program Files и `Access denied` после повышенного экземпляра (п. 6),
автозапуск без UAC при входе (п. 8), обычный пользователь и пароль
администратора (п. 9), отсутствие `VirtualStore` на Win7 (п. 10). Пункты §12
(выключенный UAC, окружение под `runas`, `OpenProcess` родителя другой учётной
записи, DACL файлов в профиле) — там же.
