# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- JSON tabs of the node windows (node info "Outbound JSON", Add server "JSON", outbound editor "JSON", source window "JSON") now use a real JSON editor: syntax highlighting, line numbers, folding, search (Ctrl/Cmd+F), mouse selection and copy. Windows 7 builds keep the plain text field.
-

### Technical / Internal
- New dependency `github.com/ideaconnect/go-fyne-pretty-view/v2` behind `internal/fynewidget.JSONEditor`; the file using it carries a `go1.26` build constraint, so the Win7 toolchain (go1.21) compiles the Entry-based fallback and `go.win7.mod` never sees the module. `go.mod` now says `go 1.26.0`.
-

## RU
### Основное
- JSON-вкладки окон узла («Outbound JSON» в сведениях, «JSON» в добавлении сервера, в редакторе outbound и в окне источника) переведены на настоящий JSON-редактор: подсветка синтаксиса, номера строк, свёртка, поиск (Ctrl/Cmd+F), выделение мышью и копирование. Сборки для Windows 7 остаются с обычным текстовым полем.
-

### Техническое / Внутреннее
- Новая зависимость `github.com/ideaconnect/go-fyne-pretty-view/v2` за интерфейсом `internal/fynewidget.JSONEditor`; файл с ней помечен ограничением сборки `go1.26`, поэтому тулчейн Win7 (go1.21) собирает запасной вариант на Entry, а `go.win7.mod` модуль не видит. В `go.mod` теперь `go 1.26.0`.
-
