# Задачи: Единая подсистема «ручная загрузка при ошибке»

## Этап 1: Константы и диалог

- [x] Добавить в `internal/constants/constants.go` константы `SingboxReleasesURL` и `WintunHomeURL`
- [x] Перенести `NewCustom` из `ui/components/custom_dialog.go` в `internal/dialogs/dialogs.go`; удалить `ui/components/custom_dialog.go`
- [x] Заменить все вызовы `components.NewCustom` на `dialogs.NewCustom` или `internaldialogs.NewCustom` (ui, wizard, clash_api_tab, presenter_save, get_free_dialog, save_state_dialog, load_state_dialog, diagnostics_tab)
- [x] Реализовать `ShowDownloadFailedManual(window, title, downloadURL, targetDir)` через `NewCustom`: фиксированные тексты (downloadFailedMessage + downloadFailedManualHint), ссылка «Open download page» + кнопка копирования URL (иконка), зарезервированная высота строки со ссылкой, кнопка «Open folder», «Close»
- [x] В `ui/dialogs.go` реэкспорт `ShowDownloadFailedManual`

## Этап 2: Core Dashboard

- [x] В `ui/core_dashboard_tab.go` при ошибке загрузки sing-box (`progress.Status == "error"`) вызывать `ShowDownloadFailedManual` с `SingboxReleasesURL` и папкой `bin`
- [x] При ошибке загрузки wintun — `ShowDownloadFailedManual` с `WintunHomeURL` и папкой `bin`
- [x] Во всех ветках ошибок `downloadConfigTemplate` вызывать `ShowDownloadFailedManual` с `GetTemplateURL()` и папкой `bin` (вместо `ShowError`)

## Этап 3: Визард — шаблон

- [x] В `ui/wizard/wizard.go` при ошибке `LoadTemplateData` при открытии визарда показывать `dialogs.ShowDownloadFailedManual(parent, ...)` с ссылкой и папкой `bin`
- [x] В `loadConfigFromFile` при отсутствии конфига и шаблона показывать `dialogs.ShowDownloadFailedManual(wizardWindow, ...)` вместо `dialog.ShowError`, затем закрыть окно
- [x] В обработчике Load State → New при ошибке `LoadTemplateData` показывать `dialogs.ShowDownloadFailedManual(wizardWindow, ...)`

## Этап 4: Визард — SRS

- [x] В `ui/wizard/tabs/rules_tab.go` при ошибке загрузки SRS в кнопке SRS вызывать `dialogs.ShowDownloadFailedManual` с первым URL из `srsEntries`, папкой `bin/rule-sets`

## Этап 5: Документация и проверка

- [x] Заполнить IMPLEMENTATION_REPORT.md
- [x] Обновить SPEC, PLAN, TASKS, IMPLEMENTATION_REPORT под текущую реализацию
- [ ] Ручная проверка: ошибка загрузки sing-box, wintun, шаблона, SRS — диалог с двумя строками текста, ссылкой (и кнопкой копирования ссылки) и кнопкой «Open folder»
