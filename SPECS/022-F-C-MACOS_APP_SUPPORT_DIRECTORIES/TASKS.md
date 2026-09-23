# Задачи: macOS — Application Support / Logs, изменяемый Bundle ID

## Этап 1: Контракт путей и Bundle ID

- [ ] Добавить в код **дефолтный** Bundle ID `com.singbox-launcher` и переменную, перезаписываемую **`-X`** при сборке (имя символа согласовать с `PLAN.md`).
- [ ] Реализовать **`IsMacOSAppBundle()`** (или эквивалент) в `internal/platform` с тестируемой чистой функцией от пути к exe.
- [ ] Реализовать **`MacOSDataRoot(bundleID string) (string, error)`** и **`MacOSLogsRoot(bundleID string) (string, error)`** (или один резолвер), используя `os.UserHomeDir` + `Library/Application Support` / `Library/Logs`.

## Этап 2: FileService

- [ ] В **`NewFileService`** (и связанных местах): вычислять корень для `bin/` и для логов по правилам SPEC (macOS + .app → Library; иначе → `ExecDir`).
- [ ] Обновить **`EnsureDirectories`**, пути конфига, rule-sets, открытие логов — без регрессий на Windows/Linux.
- [ ] Проверить все прямые использования **`ExecDir`** там, где подразумевались данные пользователя (не только `FileService`).

## Этап 3: Миграция

- [ ] Реализовать однократный перенос (или копирование) из `Contents/MacOS/bin/` в Application Support при первом запуске новой версии — по правилам из `PLAN.md`.
- [ ] Залогировать исход миграции через **`internal/debuglog`** (без лишнего шума в UI).

## Этап 4: Сборка macOS

- [ ] В **`build/build_darwin.sh`:** переменная **`MACOS_BUNDLE_ID`** (default `com.singbox-launcher`), подстановка в **`CFBundleIdentifier`** и в **`-ldflags -X`**.
- [ ] Убедиться, что **CI** вызывает скрипт без поломки дефолта.

## Этап 5: Документация и приёмка

- [ ] Обновить **README.md** / **README_RU.md** (где лежат конфиг и логи на macOS в режиме `.app`).
- [ ] При существенном изменении потоков — **docs/ARCHITECTURE.md**.
- [ ] После реализации: **docs/release_notes/upcoming.md**, **IMPLEMENTATION_REPORT.md**, переименовать папку задачи в **`022-F-C-…`** по workflow README.

## Проверки

- [ ] `go vet ./...`, `go build ./...`, `go test ./...` (с учётом CONSTITUTION по GUI).
- [ ] Ручная проверка: запуск из `.app` — данные в `~/Library/…`; `go run` / бинарь из repo — данные рядом с exe.
