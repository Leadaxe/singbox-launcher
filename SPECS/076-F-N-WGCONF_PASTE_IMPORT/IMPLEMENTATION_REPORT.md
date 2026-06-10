# IMPLEMENTATION_REPORT 076 — Вставка голого WG/AWG `.conf`-текста

**Статус:** реализовано, тесты зелёные. Дата: 2026-06-10.

## Что сделано

1. **Helpers** — новый [`core/config/subscription/wgconf_text.go`](../../core/config/subscription/wgconf_text.go):
   * `ExtractWGConfBlocks` — режет вставленный текст на `[Interface]`-блоки (регистронезависимо, по строкам); текст до первого блока возвращается для обычной построчной классификации — ссылки и conf можно вставлять вместе.
   * `ConvertWGConfText` — блок → канонический `wireguard://`-URI через конвертер SPEC 075 (`parseWGConfSections`/`wgConfToURI`); label = хост из `Endpoint` (IPv6 — без скобок), чтобы вставленные узлы не получали одинаковый тег.
2. **Врезка** — [`ui/configurator/business/parser.go`](../../ui/configurator/business/parser.go) `classifyInputLines`: блоки выделяются и конвертируются до построчного цикла, готовые URI добавляются к connections; невалидный блок — warn, остальная вставка живёт. Дальше всё по существующим путям: `AppendURLsToSources` создаёт `Source{Type: server}` (Label из фрагмента URI), `parseWireGuardURI` даёт нормализацию CIDR, promote AWG-полей и кламп MTU AWG → 1280.
3. **Тесты** — [`wgconf_text_test.go`](../../core/config/subscription/wgconf_text_test.go) (разрезка 0/1/2 блока, AWG end-to-end через `ParseNode` с клампом, IPv6-endpoint, три вида невалидных блоков) и [`parser_wgconf_test.go`](../../ui/configurator/business/parser_wgconf_test.go) (смешанная вставка sub-URL + vless + AWG-conf → 1 подписка + 2 connections; битый блок не срывает вставку).

## Проверки

* `go build ./...` — OK; `go test ./...` — OK; `go vet ./...` — OK; `gofmt` — чисто.

## Ограничения / Assumptions

* Conf-текст распознаётся только в путях вставки через `classifyInputLines` (поле Add / multi-line). Тело HTTP-подписки и edit-окно источника — вне объёма (SPEC).
* `.conf` не хранится сырым: источник всегда канонический `wireguard://`-URI (share/round-trip без новых форматов).
* Несколько `[Peer]` в блоке — берётся первый (политика multi-peer из SPEC 073/shareuri).
