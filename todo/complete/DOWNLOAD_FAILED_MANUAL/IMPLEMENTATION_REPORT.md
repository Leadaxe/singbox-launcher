# Отчёт о реализации: Единая подсистема «ручная загрузка при ошибке»

## Статус

- [x] Реализовано
- [x] Сборка internal-пакетов и vet проходят
- [ ] Ручная проверка UI (при ошибках загрузки — диалог с ссылкой и «Open folder»)

## Выполненные задачи

1. **Константы URL** — в `internal/constants`: `SingboxReleasesURL`, `WintunHomeURL`.
2. **NewCustom** — перенесена из `ui/components/custom_dialog.go` в `internal/dialogs`; файл `ui/components/custom_dialog.go` удалён. Все вызовы заменены на `dialogs.NewCustom` / `internaldialogs.NewCustom` (ui, core_dashboard_tab, wizard, clash_api_tab, presenter_save, get_free_dialog, save_state_dialog, load_state_dialog, diagnostics_tab).
3. **ShowDownloadFailedManual** — в `internal/dialogs`: два фиксированных текста («Download failed. See the log for details.» + «Please download the file manually and place it in the folder below.»), строка со ссылкой «Open download page» и кнопкой с иконкой копирования (копирует URL в буфер), зарезервированная высота строки со ссылкой (спейсер), кнопка «Open folder», кнопка «Close»; реализация через вызов **NewCustom** (без импорта `ui/components`).
4. **Реэкспорт** — в `ui/dialogs.go`: вызов `dialogs.ShowDownloadFailedManual` для вызовов из пакета `ui`.
5. **Core Dashboard** — при ошибках загрузки sing-box, wintun и шаблона вызывается `ShowDownloadFailedManual` с соответствующими URL и папкой `bin`.
6. **Визард — шаблон** — при ошибке/отсутствии шаблона (3 места) показывается тот же диалог с `GetTemplateURL()` и папкой `bin`.
7. **Визард — SRS** — при ошибке загрузки SRS показывается диалог с первым URL из правила и папкой `bin/rule-sets`.

## Изменённые файлы

| Файл | Изменения |
|------|-----------|
| `internal/constants/constants.go` | Константы SingboxReleasesURL, WintunHomeURL |
| `internal/dialogs/dialogs.go` | NewCustom (перенесена из ui/components); ShowDownloadFailedManual через NewCustom (два текста, ссылка + кнопка копирования URL, спейсер под ссылкой, кнопки) |
| `ui/dialogs.go` | Реэкспорт ShowDownloadFailedManual; вызов dialogs.NewCustom вместо components |
| `ui/core_dashboard_tab.go` | constants, dialogs; при ошибках sing-box, wintun, template — ShowDownloadFailedManual; dialogs.NewCustom для Update popup |
| `ui/wizard/wizard.go` | constants, dialogs; при ошибке/отсутствии шаблона — dialogs.ShowDownloadFailedManual; dialogs.NewCustom для Confirmation |
| `ui/wizard/tabs/rules_tab.go` | filepath, constants, dialogs; при ошибке SRS — dialogs.ShowDownloadFailedManual |
| `ui/clash_api_tab.go` | dialogs.NewCustom (импорт ui/components убран) |
| `ui/wizard/presentation/presenter_save.go` | internal/dialogs.NewCustom вместо ui/components |
| `ui/wizard/dialogs/get_free_dialog.go` | internaldialogs.NewCustom |
| `ui/wizard/dialogs/save_state_dialog.go` | internaldialogs.NewCustom |
| `ui/wizard/dialogs/load_state_dialog.go` | internaldialogs.NewCustom |
| `ui/diagnostics_tab.go` | dialogs.NewCustom (вместо components) |
| `ui/components/custom_dialog.go` | **Удалён** (NewCustom перенесена в internal/dialogs) |

## Ключевые фрагменты

### internal/dialogs/dialogs.go
```go
func NewCustom(title, mainContent, buttons, dismissText, parent) dialog.Dialog
// Border: mainContent центр, buttons низ; при dismissText — кнопка Close слева, ESC закрывает.

func ShowDownloadFailedManual(window, title, downloadURL, targetDir string)
// Константы: downloadFailedMessage, downloadFailedManualHint.
// mainContent: два лейбла + строка (ссылка «Open download page» + кнопка с theme.ContentCopyIcon для копирования URL) + спейсер; buttons: при targetDir — «Open folder».
// Вызов: NewCustom(title, mainContent, buttons, "Close", window); d.Show().
```

### Точки вызова
- **sing-box**: title «sing-box download failed», URL `constants.SingboxReleasesURL`, dir `bin`
- **wintun**: title «wintun.dll download failed», URL `constants.WintunHomeURL`, dir `bin`
- **template**: title «Config template download failed» / «Config template failed to load» / «Config template missing», URL `wizardtemplate.GetTemplateURL()`, dir `bin`
- **SRS**: title «Rule-set (SRS) download failed», URL первый из `srsEntries`, dir `bin/rule-sets`

## Команды для проверки

```bash
go build ./internal/...
go vet ./internal/dialogs/... ./internal/constants/...
```

Полная сборка приложения (`go build ./...`) может требовать CGO/OpenGL (Fyne).

## Риски и ограничения

- Цикл импортов: `internal/dialogs` не импортирует `ui/components`; NewCustom и ShowDownloadFailedManual используют только fyne и dialog.
- Для SRS в диалоге показывается только первый URL из правила; папка одна — `bin/rule-sets`.

## Дата

2025-02-20 (обновление: кнопка копирования ссылки, спейсер под ссылкой)
