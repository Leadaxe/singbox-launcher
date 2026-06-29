# SPEC 082-F-N — НАТИВНЫЙ ДИАЛОГ ВЫБОРА ФАЙЛА

## Цель

Кнопка «Add from file» (SPEC 079) открывает встроенный в Fyne файловый диалог — он рисуется приложением и не похож на системный. Заменить на **нативное системное окно** на каждой ОС (привычнее пользователю), без новой Go-зависимости — через системные команды (паттерн уже используется в `internal/platform/*.go`: `osascript`-стиль, `explorer`, `xdg-open`, `wmic`).

## Объём

1. **`platform.PickOpenFile(prompt string, exts []string) (path string, ok bool, err error)`** — нативный open-file диалог:
   - **macOS** — `osascript` `choose file of type {…}` (родное окно Finder).
   - **Windows** — `powershell -STA` + `System.Windows.Forms.OpenFileDialog` (родное окно; есть на Win7+).
   - **Linux** — `zenity --file-selection` → `kdialog --getopenfilename`; если ни того, ни другого нет → `ErrNativeDialogUnavailable`.
   - прочие ОС (stub) → `ErrNativeDialogUnavailable`.
   - Возврат: `(path,true,nil)` выбор; `("",false,nil)` отмена; `("",false,ErrNativeDialogUnavailable)` нет нативного диалога (caller делает fallback).
2. **UI** (`source_tab.go`): кнопка зовёт `platform.PickOpenFile`; на `ErrNativeDialogUnavailable` → текущий `dialog.NewFileOpen` (Fyne) как fallback (Linux без zenity/kdialog). Прочитать выбранный файл через `business.ReadSourceFileText` → тот же apply-путь.
3. Локаль prompt (en + ru). Тесты парсинга/экранирования (где детерминировано). Release notes.

## Вне объёма

- CGO-библиотека нативных диалогов (`sqweek/dialog` и т.п.) — отклонено: новая зависимость + GTK-dev на Linux при сборке; CONSTITUTION предпочитает stdlib + системные команды.
- Запоминание последней папки между запусками (системный диалог сам это делает на mac/win).
- Save-диалоги (traffic export остаётся на Fyne — отдельная история).

## Дизайн-решения

- **Системные команды, не зависимость.** Консистентно с `internal/platform` (там уже `exec.Command` per-OS). Ноль новых модулей.
- **Fallback на Fyne** только там, где нативного нет (Linux без zenity/kdialog) — фича не ломается ни на одной конфигурации.
- **Экранирование ввода** (prompt) под каждый язык команд: AppleScript-литерал, PowerShell single-quote, zenity/kdialog — через argv (без shell), так что инъекция исключена.

## Критерии приёмки

1. macOS/Windows: «Add from file» открывает **родное** системное окно выбора файла с фильтром `.conf`/`.vpn`/`.txt`.
2. Linux с zenity или kdialog — родное окно; без них — Fyne-диалог (фича работает).
3. Отмена диалога ничего не меняет; выбор файла импортирует так же, как раньше.
4. `go build ./... && go test ./... && go vet ./...` зелёные (на всех целевых GOOS компилируется — build tags).
