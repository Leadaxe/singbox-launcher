# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- JSON tabs of the node windows (node info "Outbound JSON", Add server "JSON", outbound editor "JSON", source window "JSON") now use a real JSON editor: syntax highlighting, line numbers, folding, search (Ctrl/Cmd+F), mouse selection and copy. Windows 7 builds keep the plain text field.
- WireGuard/AWG node window (daemon mode and remote machines): the WireGuard section lists the peers — shortened key, address, handshake age ("online" within 3 minutes of a handshake or while bytes are coming in, otherwise "no session, last handshake N ago") and bytes ↓/↑. On a server node it shows which clients are connected and from where. Needs core with SPEC 114 (peer status in `GetOutbounds`); an older core shows no peers.
-

### Technical / Internal
- New dependency `github.com/ideaconnect/go-fyne-pretty-view/v2` behind `internal/fynewidget.JSONEditor`; the file using it carries a `go1.26` build constraint, so the Win7 toolchain (go1.21) compiles the Entry-based fallback and `go.win7.mod` never sees the module. `go.mod` now says `go 1.26.0`.
-

## RU
### Основное
- JSON-вкладки окон узла («Outbound JSON» в сведениях, «JSON» в добавлении сервера, в редакторе outbound и в окне источника) переведены на настоящий JSON-редактор: подсветка синтаксиса, номера строк, свёртка, поиск (Ctrl/Cmd+F), выделение мышью и копирование. Сборки для Windows 7 остаются с обычным текстовым полем.
- Окно узла WireGuard/AWG (режим демона и удалённые машины): в секции WireGuard появились пиры — ключ с вырезанной серединой, адрес, возраст хендшейка («на связи» в пределах 3 минут после хендшейка или пока идут байты, иначе «нет сессии, последний хендшейк N назад») и объём ↓/↑. На серверном узле видно, какие клиенты подключены и откуда. Нужно ядро со SPEC 114 (статус пиров в `GetOutbounds`); со старым ядром пиров нет.
-

### Техническое / Внутреннее
- Новая зависимость `github.com/ideaconnect/go-fyne-pretty-view/v2` за интерфейсом `internal/fynewidget.JSONEditor`; файл с ней помечен ограничением сборки `go1.26`, поэтому тулчейн Win7 (go1.21) собирает запасной вариант на Entry, а `go.win7.mod` модуль не видит. В `go.mod` теперь `go 1.26.0`.
-
