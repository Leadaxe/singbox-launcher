# SPEC 079-F-N — ДОБАВЛЕНИЕ ИСТОЧНИКА ИЗ ФАЙЛА

## Цель

WG/AmneziaWG-конфиги часто раздают **файлом** (`.conf` с `[Interface]/[Peer]`, или `.vpn` со ссылкой `vpn://`). Сейчас на вкладке Sources их можно только вставить текстом. Добавить кнопку **«Add from file»**: выбрать файл → его содержимое уходит в тот же путь импорта, что и поле Add.

## Контекст

- Кнопка **Add** (`ui/configurator/tabs/source_tab.go`, `addURLButton`) берёт текст из `SourceURLEntry` → `wizardbusiness.AppendURLsToSources`.
- `AppendURLsToSources` → `classifyInputLines` уже понимает: URI-ссылки (`vless://`, `wireguard://`, `awg://`, `vpn://`, …) и голый `[Interface]/[Peer]`-текст (SPEC 075/076). То есть **парсинг файла уже реализован** — не хватает только чтения файла и передачи текста в этот путь.

## Объём

1. **`business.ReadSourceFileText(r io.Reader) (string, error)`** — читает содержимое с лимитом (1 МБ, защита), trim. Тестируемо без Fyne.
2. **UI**: кнопка «Add from file» рядом с Get free (header вкладки Sources). Клик → `dialog.ShowFileOpen` (фильтр `.conf`/`.vpn`/`.txt`) → `ReadSourceFileText` → тот же apply-путь, что у Add (`AppendURLsToSources` + refresh). Ошибки чтения → `dialog.ShowError`.
3. Вынести apply-логику Add в локальную `applyAddedSources(text)` для переиспользования обеими кнопками (без дублирования).
4. Локали en + ru. Тест `ReadSourceFileText`. Release notes.

## Вне объёма

- Drag&drop файла (отдельная возможность).
- Парсинг новых форматов — всё уже покрыто SPEC 075/076 (vpn://, conf-текст) и URI-парсером.
- Импорт HTTP-подписки из файла как «тела подписки» — файл трактуется как набор ссылок / conf-текст (та же семантика, что вставка в поле).

## Критерии приёмки

1. Кнопка «Add from file» открывает системный диалог выбора файла.
2. Выбор `.conf` (AmneziaWG `[Interface]/[Peer]`) → добавляется WG/AWG-узел; `.vpn` (`vpn://…`) → добавляется узел из профиля Amnezia; `.txt` со списком `vless://`-ссылок → добавляются узлы. Поведение идентично вставке того же содержимого в поле Add.
3. Слишком большой файл (> 1 МБ) / ошибка чтения → понятная ошибка, состояние не меняется.
4. `go build ./... && go test ./... && go vet ./...` зелёные.
