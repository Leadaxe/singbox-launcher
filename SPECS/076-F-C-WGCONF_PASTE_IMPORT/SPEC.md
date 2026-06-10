# SPEC 076-F-C — ВСТАВКА ГОЛОГО WG/AWG `.conf`-ТЕКСТА

## Цель

Пользователь вставляет в поле Add вкладки Sources содержимое `.conf`-файла WireGuard/AmneziaWG (`[Interface]/[Peer]`, включая AWG-поля `Jc`/`Jmin`/`Jmax`/`S1`–`S4`/`H1`–`H4`/`I1`–`I5`) — лаунчер сам понимает, что это не ссылка, и добавляет узел. Закрывает последний пробел импорта Amnezia после SPEC 075 (`vpn://`): `.conf` — второй формат, который выдаёт AmneziaWG 2.0.

Сегодня `classifyInputLines` (`ui/configurator/business/parser.go`) рубит ввод на строки и распознаёт только URI — многострочный INI-текст молча выбрасывается («no valid URLs to add»).

## Объём

* `core/config/subscription` — новый `wgconf_text.go`, экспортируемые helpers (конвертер `parseWGConfSections`/`wgConfToURI` из SPEC 075 переиспользуется):
  * `ExtractWGConfBlocks(input) (rest string, blocks []string)` — отделяет `[Interface]`-блоки (с первой строки `[Interface]`, новый блок на каждой следующей; регистронезависимо) от остального текста; текст до первого блока возвращается как `rest` для обычной построчной классификации.
  * `ConvertWGConfText(conf) (string, error)` — один блок → канонический `wireguard://`-URI; label (fragment) — хост из `Endpoint`.
* `classifyInputLines`: до построчного разбора выделить блоки, сконвертировать в URI, добавить к connections; невалидный блок — warn в лог, не срыв всей вставки. Смешанный ввод (ссылки + conf) работает.
* Дальше — существующий путь: каждый URI становится `Source{Type: server}` (AppendURLsToSources), парсится `parseWireGuardURI` (нормализация CIDR, promote AWG, MTU-кламп AWG → 1280 из SPEC 073).
* Unit-тесты в пакете `subscription`; документация (`ParserConfig.md`, `upcoming.md`).

## Вне объёма

* Импорт `.conf` файлом (file picker / drag&drop) — только вставка текста.
* Распознавание `.conf`-текста в теле HTTP-подписки и в edit-окне источника.
* Прочие INI-форматы (OpenVPN `client`-конфиги и т.п.).

## Критерии приёмки

1. Вставка содержимого `.conf` от AmneziaWG 2.0 в поле Add даёт рабочий AWG-узел (MTU ≤ 1280, AWG-поля в endpoint root); label — хост эндпоинта.
2. Несколько `[Interface]`-блоков за одну вставку → несколько узлов; ссылки в том же тексте продолжают работать.
3. Блок без обязательных полей пропускается с warn-логом, остальной ввод обрабатывается.
4. `go build ./... && go test ./... && go vet ./...` зелёные.
