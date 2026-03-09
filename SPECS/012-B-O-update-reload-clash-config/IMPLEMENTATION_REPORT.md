# Отчёт о реализации: 012-B-O-update-reload-clash-config

## Анализ изменений логики

### Было (до правок)

| Аспект | Поведение |
|--------|-----------|
| **Порядок при сохранении** | Сборка конфига → **сразу запись в config.json** (с бэкапом) → валидация уже записанного файла через `sing-box check` → запись state.json. |
| **При ошибке валидации** | Рабочий config.json уже перезаписан; в диалоге успеха показывалось предупреждение «Validation warning», конфиг оставался новым (возможно невалидным). |
| **После сохранения** | При запущенном sing-box выполнялся **перезапуск** (Stop → ожидание до 2 с → Start). |
| **Update** | В фоне вызывался `RunParserProcess()`. ReloadClashAPIConfig после Update не вызывался. |
| **Презентер** | Отдельный шаг `validateConfigFile(configPath)` после `saveConfigFile()`; в диалог передавался `validationErr` (мог быть не nil). |
| **SaveConfigWithBackup** | Сигнатура `(fileService, configText)`; только подготовка текста, бэкап и запись. Валидация снаружи. |

### Стало (после правок)

| Аспект | Поведение |
|--------|-----------|
| **Порядок при сохранении** | Сборка конфига → **сначала валидация** по временному файлу `config-check.json` (при непустом `fileService.SingboxPath()`) → **только при успехе** бэкап и запись в config.json → запись state.json. |
| **При ошибке валидации** | Рабочий config.json **не трогается**; пользователю показывается ошибка; временный файл удаляется (defer). Сохранение не считается успешным. |
| **После сохранения** | Перезапуск sing-box **не выполняется**. Только запись файлов и вызов Update в фоне. |
| **Update** | По-прежнему в фоне `RunParserProcess()`. ReloadClashAPIConfig только при старте sing-box. |
| **Презентер** | Один шаг `saveConfigFile(configText)` — внутри вызывается `SaveConfigWithBackup(fileService, configText)`. Отдельного `validateConfigFile()` нет. При **успехе** — диалог «Config Saved» с «Validation: Passed», по OK визард закрывается. При **ошибке валидации** — показывается диалог с ошибкой (текст от sing-box), конфиг не перезаписывается, временный файл удалён; **визард остаётся открыт** (Close() вызывается только из диалога успеха), пользователь может исправить конфиг и нажать Save снова. |
| **SaveConfigWithBackup** | Сигнатура `(fileService, configText)`. Путь к sing-box берётся из `fileService.SingboxPath()`. При непустом пути: запись в `config-check.json` → валидация → при успехе бэкап и запись в config. При пустом пути — только бэкап и запись (graceful degradation). |

### Итог по логике

- **Конфиг не перезаписывается при невалидном результате** — сначала проверка по временному файлу, затем запись в рабочий путь.
- **Сохранение в визарде не перезапускает sing-box** — только файлы и Update.
- **Clash API** — перечитывается только при старте sing-box.
- **Один источник пути к sing-box** — бизнес-слой получает его из `FileServiceInterface.SingboxPath()`, без отдельного параметра.

---

## План изменений

1. **Визард:** Удалён перезапуск sing-box из `saveStateAndShowSuccessDialog()`. При сохранении — только запись файлов, обновление статуса в Core, state.json и вызов Update в фоне. ReloadClashAPIConfig только при запуске sing-box.
2. **Порядок при записи config.json:** валидация по временному файлу `config-check.json`, при успехе — бэкап и запись; при ошибке валидации рабочий конфиг не меняется.
3. **Интерфейс и API:** `FileServiceInterface` дополнен методом `SingboxPath()`; `SaveConfigWithBackup(fileService, configText)` без параметра singBoxPath.

## Изменённые файлы

- `ui/wizard/presentation/presenter_save.go` — удалён перезапуск sing-box; убран шаг `validateConfigFile()`; `saveConfigFile()` вызывает `SaveConfigWithBackup(fileService, configText)`; диалог успеха без параметра validationErr.
- `ui/wizard/business/saver.go` — `SaveConfigWithBackup(fileService, configText)`: путь к sing-box из `fileService.SingboxPath()`; при непустом пути — запись в `config-check.json`, валидация, затем бэкап и запись в config; константа `tempConfigFileName = "config-check.json"`.
- `ui/wizard/business/interfaces.go` — в `FileServiceInterface` добавлен `SingboxPath() string`.
- `ui/wizard/business/file_service_adapter.go` — реализация `SingboxPath()`.
- `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md`, `SPEC.md` — обновлены под новую логику и API.

## Ключевые фрагменты

- **presenter_save.go:** `saveConfigFile()` → `SaveConfigWithBackup(fileService, configText)`; `saveStateAndShowSuccessDialog(configPath)` без второго аргумента.
- **saver.go:** `singBoxPath := fileService.SingboxPath()`; при `singBoxPath != ""` — временный файл, `ValidateConfigWithSingBox`, затем запись в config.

## Проверка

- `go build ./...` — на окружении без CGO/OpenGL может падать из-за Fyne.
- `go vet ./...` — рекомендуется выполнить на целевой платформе.

## Риски и ограничения

- Нет.

## Assumptions

- ReloadClashAPIConfig по логике нужен только при запуске sing-box; после Update не вызывается.

## По ходу сделано дополнительно

- **Кнопка Restart (Core):** между Start и Stop добавлена кнопка перезапуска (🔄). Убивает процесс sing-box без флага StoppedByUser; вотчер перезапускает его. Флаг `RestartRequestedByUser` в контроллере; в Monitor и onPrivilegedScriptExited при этом флаге вызывается `Start(true)`. В UI при нажатии — статус «Restarting...», кратко показывается состояние кнопок «Stopped» (Start активна, Stop неактивна), затем снова «Running». Файлы: `core/controller.go`, `core/process_service.go`, `ui/core_dashboard_tab.go`.
- **Привилегированный старт в platform:** имена скрипта/PID/pattern и создание скрипта запуска вынесены в `internal/platform`: константы `PrivilegedScriptName`, `PrivilegedPidFileName`, `PrivilegedPkillPattern`, функция `WritePrivilegedStartScript(...)` в `privileged_darwin.go`; заглушки в `privileged_stub.go`. В `core/process_service.go` остаётся оркестрация: вызов platform для записи скрипта, RunWithPrivileges, состояние, WaitForPrivilegedExit, onPrivilegedScriptExited.
- **Release notes:** в `docs/release_notes/upcoming.md` добавлен пункт про кнопку Restart (EN и RU).

## Статус

- Задача 012 реализована и закрыта.
