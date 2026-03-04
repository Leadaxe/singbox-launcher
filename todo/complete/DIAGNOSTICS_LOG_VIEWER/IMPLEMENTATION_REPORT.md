# Отчёт о реализации: Окно логов (Diagnostics)

**Статус:** реализовано.

**Дата:** 2025-03-03.

## Изменённые файлы

| Файл | Изменения |
|------|-----------|
| `internal/debuglog/debuglog.go` | Пакетная документация про sink; переменные `internalLogSinkMu`, `internalLogSink`; в `Log()` вызов sink под RLock с `(level, line)`; `SetInternalLogSink`, `ClearInternalLogSink` |
| `api/clash.go` | Импорт `sync`; переменные `apiLogSinkMu`, `apiLogSink`; `SetAPILogSink`, `ClearAPILogSink`; в `writeLog()` формирование `line` и вызов sink после записи в файл |
| `core/services/file_service.go` | Поле `ChildLogRelativePath`, присвоение в `OpenLogFiles`; функция `ReadLastLines(path, maxLines)` (tail до 2MB, последние N строк) |
| `ui/log_viewer_window.go` | **Новый файл**: окно с тремя вкладками (Internal, Core, API), регистрация/снятие sink при открытии/закрытии, каналы и горутины для обновления UI без блокировки, фильтр по уровню на Internal/API, Core — tail из файла, кнопка Refresh и автообновление 5 с при активной вкладке Core, таймер снимается при закрытии окна или смене вкладки |
| `ui/diagnostics_tab.go` | Кнопка «Open logs», вызывающая `OpenLogViewerWindow(ac)` |

## Краткое описание

1. **Перехват в месте генерации:** в `debuglog.Log()` и в `api.writeLog()` после записи вызывается опциональный callback `(level, line)`; callback не блокирует (отправка в буферизованный канал, обновление UI через `fyne.Do`).
2. **Tail для Core:** путь к логу sing-box берётся из одного места — `FileService.ChildLogRelativePath`, задаётся при `OpenLogFiles(logFileName, childLogFileName, apiLogFileName)` из `core/controller.go`. Функция `ReadLastLines` читает последние N строк из файла (до 2MB с конца).
3. **Окно логов:** кнопка «Open logs» на вкладке Diagnostics открывает отдельное окно с вкладками Internal, Core, API. На Internal и API — выбор уровня (Error…Trace), отображаются только события с уровнем не ниже выбранного; макс. 300 записей. На Core — кнопка «Refresh» и автообновление раз в 5 с при активной вкладке; таймер снимается при закрытии окна или переключении вкладки. При отсутствии файла Core показывается «Log file not available».

## Ключевые фрагменты кода

- **debuglog:** `SetInternalLogSink(fn func(Level, string))`, в `Log()` после `log.Print*` вызов `fn(level, line)` под RLock.
- **api:** `SetAPILogSink(fn func(debuglog.Level, string))`, в `writeLog()` после `fmt.Fprintf` в файл вызов `fn(level, fmt.Sprintf(format, args...))`.
- **file_service:** `ReadLastLines(path string, maxLines int) ([]string, error)` — открытие файла, чтение с конца блоками по 4KB, сбор последних maxLines строк; при отсутствии файла возврат `(nil, nil)`.
- **log_viewer_window:** путь Core — `filepath.Join(ac.FileService.ExecDir, ac.FileService.ChildLogRelativePath)`; при закрытии окна — `ClearInternalLogSink()`, `ClearAPILogSink()`, закрытие каналов, остановка таймера Core.

## Команды для проверки

```bash
go build ./...
go test ./...
go vet ./...
```

Примечание: полная сборка с fyne на Windows может требовать CGO/OpenGL; пакеты `internal/debuglog`, `api` проходят `go vet`.

## Риски и ограничения

- Окно логов не блокирует вызовы логирования: при переполнении канала новые записи Internal/API отбрасываются (non-blocking send).
- Файл Core может быть открыт на запись другим процессом; чтение tail при этом обычно возможно.

## Assumptions

- Уровни для отображения: «не ниже выбранного» = событие показывается, если `event.Level <= selectedLevel` (Error=1, Warn=2, … Trace=5).
- Подсветка уровня на вкладке Core — по ключевым словам в строке (error, warn, info, debug, trace) без изменения формата вывода sing-box.
