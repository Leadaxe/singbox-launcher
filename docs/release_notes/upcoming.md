# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **Native file picker for "Add from file".** The Sources "Add from file" button now opens the real system file dialog instead of the in-app one — Finder on macOS, the Windows Open dialog, and zenity/kdialog on Linux (falls back to the in-app dialog if neither is installed). (SPEC 082)

### Technical / Internal
- `platform.PickOpenFile` (file_dialog_{darwin,windows,linux,stub}.go) shells out to `osascript` / PowerShell `OpenFileDialog -STA` / `zenity`|`kdialog` — no new Go dependency, matching the existing per-OS `exec.Command` pattern. Returns `ErrNativeDialogUnavailable` so the UI falls back to the Fyne dialog. (SPEC 082)

## RU
### Основное
- **Нативный выбор файла для «Добавить из файла».** Кнопка «Добавить из файла» на вкладке Sources теперь открывает **родное системное окно** выбора файла вместо встроенного — Finder на macOS, окно «Открыть» на Windows, zenity/kdialog на Linux (если ни того, ни другого нет — встроенный диалог). (SPEC 082)

### Техническое / Внутреннее
- `platform.PickOpenFile` (file_dialog_{darwin,windows,linux,stub}.go) вызывает `osascript` / PowerShell `OpenFileDialog -STA` / `zenity`|`kdialog` — без новых Go-зависимостей, по существующему per-OS `exec.Command` паттерну. Возвращает `ErrNativeDialogUnavailable` → UI откатывается на Fyne-диалог. (SPEC 082)
