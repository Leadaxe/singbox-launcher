# IMPLEMENTATION_REPORT 075 — Импорт профилей Amnezia (`vpn://`)

**Статус:** реализовано, тесты зелёные. Дата: 2026-06-10.

## Что сделано

1. **Парсер `vpn://`** — новый [`core/config/subscription/node_parser_amnezia.go`](../../core/config/subscription/node_parser_amnezia.go):
   * `decodeAmneziaProfile` — base64url (толерантно к переносам/padding, fallback на std-алфавит) → qCompress-фрейминг (4 байта BE-длины + zlib) → JSON профиля; лимиты 512 КБ на ссылку / 8 МБ на распакованный профиль, проверка заявленной длины до распаковки + `io.LimitReader`.
   * `amneziaWGConfText` + `findWGIniText` — выбор контейнера (`defaultContainer` → порядок массива), рекурсивный поиск `[Interface]`-текста с приоритетом известных ключей (`config`, `last_config`, `awg`, `wireguard`) и лимитом глубины; JSON-строки разворачиваются раньше проверки на `[Interface]`, иначе матчится обёртка `last_config` с `\n`-эскейпами.
   * `parseWGConfSections` + `wgConfToURI` — INI → канонический `wireguard://`-URI; AWG-поля 1:1 в query; MTU passthrough (кламп AWG→1280 делает существующий `parseWireGuardURI`, SPEC 073); только первый `[Peer]`.
2. **Роутер** — [`node_parser_core.go`](../../core/config/subscription/node_parser_core.go): `IsDirectLink` += `vpn://`; ранняя ветка в `ParseNode` до проверки `MaxURILength` (у vpn:// свой потолок). Этим покрыты все пути: вставка в Sources (classifyInputLines), Connections, строки тела подписки, превью edit-окна.
3. **Тесты** — [`node_parser_amnezia_test.go`](../../core/config/subscription/node_parser_amnezia_test.go): qCompress-эмуляция; AWG-профиль (поля/типы/кламп 1420→1280/label из `description`/PSK/keepalive), plain-WG (без AWG-ключей, честный MTU), предпочтение defaultContainer, профиль без WG-контейнера (ошибка с именами), 6 видов мусора (включая объявленную zlib-бомбу), перенесённый по строкам base64.
4. **Кросс-валидация** — ссылка, собранная независимой референс-реализацией (`scripts/decode_amnezia_vpn.py`-эмулятор формата на Python), разобрана Go-парсером: тег, AWG-поля и кламп MTU совпали.
5. **Документация** — `docs/ParserConfig.md` (раздел «Amnezia (`vpn://`)»), `docs/release_notes/upcoming.md` (EN/RU).

## Проверки

* `go build ./...` — OK
* `go test ./...` — OK (30 пакетов)
* `go vet ./...` — OK
* `gofmt` — чисто

## Ограничения / Assumptions

* Из профиля берётся **один** узел (один WG/AWG-контейнер); при нескольких — defaultContainer/первый, предупреждение в лог. `ParseNode` возвращает один узел — multi-node из одного профиля вне объёма.
* Прочие протоколы Amnezia (OpenVPN, Cloak, XRay) не импортируются — понятная ошибка с именами контейнеров.
* Схема профиля Amnezia менялась между версиями — поэтому поиск конфига рекурсивный, а не жёсткий путь `containers[i].<proto>.last_config.config`; глубина ограничена.
* Секреты (ключи) в логи не пишутся — только имена контейнеров, hostName и размеры.
