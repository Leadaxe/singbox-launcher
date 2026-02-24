# Задачи: Окно логов (Diagnostics)

## Этап 1: debuglog — перехват и sink

- [ ] В `internal/debuglog`: добавить публичные `SetInternalLogSink(fn func(Level, string))` и `ClearInternalLogSink()` (под mutex). В `Log()` после записи в log вызывать sink под RLock, если установлен.
- [ ] В документации пакета debuglog описать: опциональный sink для дублирования строк в окно логов; callback вызывается с `(level, line)` из любых горутин и не должен блокировать; окно по выбранному уровню отображает только подходящие записи.

## Этап 2: tail-чтение логов (только для Core)

- [ ] Реализовать чтение последних N строк из файла (tail) **только для Core**: функция в `internal/` или `core/services/file_service.go`: `ReadLastLines(path string, maxLines int) ([]string, error)` для `logs/sing-box.log`. Ограничение отображения: макс. 300 строк (событий) на вкладке. Internal из файла не читается (источник Internal — только `debuglog.Log()`); на вкладке Internal хранить не более 300 последних событий.
- [ ] Парсинг уровня для Core — по ключевым словам в строке (для визуальной подсветки). Для Internal уровень приходит из sink.

## Этап 3: Окно логов (UI)

- [ ] В `ui/diagnostics_tab.go`: кнопка «Open logs»; по нажатию создаётся и показывается отдельное окно (fyne.Window). Содержимое окна можно вынести в `ui/log_viewer_window.go`.
- [ ] В окне логов — **две вкладки**: **Internal** и **Core** (Fyne TabContainer / TabItem).
- [ ] **Вкладка Internal**: выбор уровня (Error | Warn | Info | Verbose | Trace), read-only виджет с логами из sink. Только события с уровнем не ниже выбранного отображаются; callback добавляет в виджет только подходящие строки. Подсветка уровня (цвет/метка) для каждой строки.
- [ ] **Вкладка Core**: кнопка «Refresh», read-only виджет с логами из файла `logs/sing-box.log` (tail). При открытии вкладки или по «Refresh» — загрузка tail. Подсветка уровня по парсингу строки; фильтр по уровню не применяется.
- [ ] При открытии окна вызвать `debuglog.SetInternalLogSink(callback)`; callback передаёт `(level, line)` в UI вкладки Internal и фильтрует по выбранному уровню. При закрытии окна — `debuglog.ClearInternalLogSink()`.
- [ ] Путь к файлу Core: через `AppController.FileService.ExecDir` и `constants.ChildLogFileName` (каталог `logs/`). Internal из файла не читается.
- [ ] При отсутствии файла Core или ошибке чтения — сообщение на вкладке Core (например «Log file not available»), без падения приложения. Тексты в UI — только английский.

## Этап 4: Проверка и отчёт

- [ ] Сборка и тесты: `go build ./...`, `go test ./...`, `go vet ./...`.
- [ ] В новых путях кода — вызовы `debuglog.DebugLog` в точках start/success/error по необходимости.
- [ ] Заполнить IMPLEMENTATION_REPORT.md после реализации.
