# План: Единая подсистема «ручная загрузка при ошибке»

## 1. Архитектура

### 1.1 Компоненты

```
┌─────────────────────────────────────────────────────────────────┐
│  Точки вызова (при ошибке загрузки)                              │
│  ├── ui/core_dashboard_tab.go: sing-box, wintun, template       │
│  └── ui/wizard: template (wizard.go), SRS (rules_tab.go)         │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  internal/dialogs.ShowDownloadFailedManual(window, title,       │
│    downloadURL, targetDir)                                       │
│  └── вызывает NewCustom(title, mainContent, buttons, "Close", window)
│      mainContent: «Download failed. See the log...» +            │
│        «Please download the file manually...» + ссылка + кнопка  │
│        копирования URL                                          │
│      buttons: при targetDir != "" — кнопка «Open folder»         │
└─────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
┌─────────────────────────┐     ┌─────────────────────────┐
│  platform.OpenURL(url)  │     │  platform.OpenFolder   │
│  (браузер)              │     │  (проводник ОС)         │
└─────────────────────────┘     └─────────────────────────┘
```

### 1.2 Ограничение импортов

- Реализация диалога — в **internal/dialogs**. Функция **NewCustom** перенесена из `ui/components/custom_dialog.go` (файл удалён); все вызовы `components.NewCustom` заменены на `dialogs.NewCustom` или `internaldialogs.NewCustom` (в пакете wizard/dialogs).
- **ShowDownloadFailedManual** строит mainContent (две строки текста + ссылка) и кнопки, вызывает **NewCustom** и Show(). Без импорта `ui/components`.

### 1.3 Константы URL

- В **internal/constants**: `SingboxReleasesURL`, `WintunHomeURL`.
- URL шаблона — `wizardtemplate.GetTemplateURL()`.
- URL SRS — первый из `srsEntries` (или пустая строка).

---

## 2. Изменения по файлам

### 2.1 internal/constants/constants.go

- Добавить константы:
  - `SingboxReleasesURL = "https://github.com/SagerNet/sing-box/releases"`
  - `WintunHomeURL = "https://www.wintun.net/"`

### 2.2 internal/dialogs/dialogs.go

- **NewCustom(title, mainContent, buttons, dismissText, parent)** — перенесена из ui/components; Border-диалог с кнопками снизу, ESC закрывает при непустом dismissText.
- **ShowDownloadFailedManual(window, title, downloadURL, targetDir)** — фиксированные тексты (downloadFailedMessage + downloadFailedManualHint), при downloadURL — строка: ссылка «Open download page» + кнопка с иконкой копирования (theme.ContentCopyIcon), копирующая URL в буфер; зарезервирована минимальная высота строки со ссылкой (спейсер), чтобы панель кнопок не наезжала; при targetDir — кнопка «Open folder»; собирает mainContent и buttons, вызывает **NewCustom(..., "Close", window)** и d.Show().

### 2.3 ui/dialogs.go

- Реэкспорт: **ShowDownloadFailedManual** — обёртка над `dialogs.ShowDownloadFailedManual` для вызовов из пакета `ui`.

### 2.4 ui/core_dashboard_tab.go

- При **ошибке DownloadCore** (`progress.Status == "error"`): вызывать `ShowDownloadFailedManual` с заголовком «sing-box download failed», `constants.SingboxReleasesURL`, папка `bin`.
- При **ошибке DownloadWintunDLL**: заголовок «wintun.dll download failed», `constants.WintunHomeURL`, папка `bin`.
- При **ошибках downloadConfigTemplate**: `ShowDownloadFailedManual` с заголовком «Config template download failed», `wizardtemplate.GetTemplateURL()`, папка `bin`.

### 2.5 ui/wizard/wizard.go

- При **ошибке LoadTemplateData** при открытии визарда: показывать `dialogs.ShowDownloadFailedManual(parent, ...)` с сообщением об отсутствии/невалидности шаблона, `wizardtemplate.GetTemplateURL()`, папка `bin`.
- В **loadConfigFromFile**: при отсутствии конфига и шаблона — вместо `dialog.ShowError` вызывать `dialogs.ShowDownloadFailedManual(wizardWindow, ...)` с ссылкой и папкой, затем закрыть окно визарда.
- При **Load State → New**: при ошибке LoadTemplateData — `dialogs.ShowDownloadFailedManual(wizardWindow, ...)`.

### 2.6 ui/wizard/tabs/rules_tab.go

- При **ошибке загрузки SRS** (в кнопке SRS, `lastErr != nil`): вызывать `dialogs.ShowDownloadFailedManual(guiState.Window, "Rule-set (SRS) download failed", downloadURL, ruleSetsDir)`, где `downloadURL` — первый URL из `srsEntries`, `ruleSetsDir = filepath.Join(execDir, constants.BinDirName, constants.RuleSetsDirName)`.

---

## 3. Тесты

- Юнит-тесты для диалога не требуются (GUI). Проверка — ручная: симуляция ошибки сети или отсутствия файла, убедиться, что во всех четырёх сценариях показывается один и тот же по смыслу диалог с ссылкой и кнопкой открытия папки.

---

## 4. Чеклист изменений

| Файл | Действие |
|------|----------|
| internal/constants/constants.go | Константы SingboxReleasesURL, WintunHomeURL |
| internal/dialogs/dialogs.go | NewCustom (перенесена из ui/components) + ShowDownloadFailedManual через NewCustom |
| ui/dialogs.go | Реэкспорт ShowDownloadFailedManual |
| ui/core_dashboard_tab.go | Вызов ShowDownloadFailedManual при ошибках sing-box, wintun, template |
| ui/wizard/wizard.go | Вызов dialogs.ShowDownloadFailedManual при ошибке/отсутствии шаблона (3 места) |
| ui/wizard/tabs/rules_tab.go | Вызов dialogs.ShowDownloadFailedManual при ошибке SRS |
| ui/components/custom_dialog.go | Удалён; NewCustom перенесена в internal/dialogs |
| ui/dialogs.go, core_dashboard_tab, wizard, clash_api_tab, presenter_save, get_free_dialog, save_state_dialog, load_state_dialog, diagnostics_tab | Вызовы components.NewCustom заменены на dialogs.NewCustom / internaldialogs.NewCustom |
