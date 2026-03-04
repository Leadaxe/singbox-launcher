# Задачи: Окно логов (Diagnostics)

## Этап 1: перехват в месте генерации (debuglog и api)

- [ ] В `internal/debuglog`: добавить публичные `SetInternalLogSink(fn func(Level, string))` и `ClearInternalLogSink()` (под mutex). В `Log()` после записи в log вызывать sink под RLock, если установлен.
- [ ] В документации пакета debuglog описать: опциональный sink для дублирования строк в окно логов; callback вызывается с `(level, line)` из любых горутин и не должен блокировать; окно по выбранному уровню отображает только подходящие записи.
- [ ] В пакете `api`: добавить публичные `SetAPILogSink(fn func(debuglog.Level, string))` и `ClearAPILogSink()` (под mutex). В `writeLog()` после записи в apiLogFile вызывать sink с тем же уровнем и сформированной строкой (как для файла). Sink не должен блокировать. Сбор API-логов из места генерации, как для Internal в debuglog.

## Этап 2: tail-чтение логов (только Core)

- [ ] Реализовать чтение последних N строк из файла (tail): функция в `internal/` или `core/services/file_service.go`: `ReadLastLines(path string, maxLines int) ([]string, error)`. Использовать **только для Core** (`logs/sing-box.log`). Internal и API из файла не читаются (источники — sink в месте генерации). Ограничение отображения: макс. 300 строк/событий на вкладке.
- [ ] Парсинг уровня для Core — по ключевым словам в строке (для визуальной подсветки). Для Internal и API уровень приходит из sink.

## Этап 3: Окно логов (UI)

- [ ] В `ui/diagnostics_tab.go`: кнопка «Open logs»; по нажатию создаётся и показывается отдельное окно (fyne.Window). Содержимое окна можно вынести в `ui/log_viewer_window.go`.
- [ ] В окне логов — **три вкладки**: **Internal**, **Core**, **API** (Fyne TabContainer / TabItem).
- [ ] **Вкладка Internal**: выбор уровня (Error | Warn | Info | Verbose | Trace), read-only виджет с логами из sink. Только события с уровнем не ниже выбранного отображаются; callback добавляет в виджет только подходящие строки. Подсветка уровня (цвет/метка) для каждой строки.
- [ ] **Вкладка Core**: кнопка «Refresh», read-only виджет с логами из файла `logs/sing-box.log` (tail, макс. 300 строк). При открытии вкладки или по «Refresh» — загрузка tail; при активной вкладке Core включён таймер автообновления раз в 5 секунд. Подсветка уровня по парсингу строки; фильтр по уровню не применяется. Таймер автообновления должен удаляться при закрытии окна логов или при переключении с вкладки Core.
- [ ] **Вкладка API**: сбор из места генерации — виджет с логами из sink `api.writeLog()`. При открытии окна зарегистрировать `api.SetAPILogSink(callback)`; callback передаёт `(level, line)` в UI вкладки API; опционально фильтр по уровню; макс. 300 событий. При закрытии окна — `api.ClearAPILogSink()`.
- [ ] При открытии окна вызвать `debuglog.SetInternalLogSink(callback)` и `api.SetAPILogSink(callback)`; при закрытии — `debuglog.ClearInternalLogSink()` и `api.ClearAPILogSink()`.
- [ ] Путь к файлу только для Core: строить **только через ту же переменную** `childLogFileName` (как при запуске sing-box): `filepath.Join(ac.FileService.ExecDir, childLogFileName)`; в `core/controller.go` уже задано `childLogFileName = "logs/" + constants.ChildLogFileName`. Не дублировать конкатенацию в окне логов — одно место изменения. Internal и API из файла не читаются.
- [ ] При отсутствии файла Core или ошибке чтения — сообщение на вкладке Core (например «Log file not available»), без падения приложения. Тексты в UI — только английский.

## Этап 4: Проверка и отчёт

- [ ] Сборка и тесты: `go build ./...`, `go test ./...`, `go vet ./...`.
- [ ] В новых путях кода — вызовы `debuglog.DebugLog` в точках start/success/error по необходимости.
- [ ] Заполнить IMPLEMENTATION_REPORT.md после реализации.
