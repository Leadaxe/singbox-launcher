# SPEC 075-F-C — ИМПОРТ ПРОФИЛЕЙ AMNEZIA (`vpn://`)

## Цель

Научить парсер принимать ссылки **`vpn://…`**, которые экспортирует Amnezia VPN / AmneziaWG 2.0 (файл `.vpn` = одна такая ссылка). Пользователь вставляет ссылку в Sources/Connections — лаунчер сам декодирует профиль, достаёт WireGuard/AmneziaWG-конфиг и превращает его в обычный WG/AWG-узел (endpoint). Сегодня такие ссылки молча игнорируются (`IsDirectLink` их не знает), и пользователю приходится вручную собирать `awg://`-ссылку из `.conf`.

## Формат `vpn://` (эталон: amnezia-vpn/config-decoder, mainwindow.cpp)

1. `vpn://` + **base64url** (алфавит `-_`, **без padding**: Qt `Base64UrlEncoding | OmitTrailingEquals`).
2. Внутри — **qCompress(json, 8)**: Qt-обёртка над zlib — первые **4 байта big-endian** длина распакованных данных, дальше обычный **zlib**-поток.
3. Распакованное — **JSON всего профиля Amnezia**: `containers[]` (по контейнеру на протокол), `defaultContainer`, `hostName`, `description`, `dns1`/`dns2`. Сам WG/AWG-конфиг лежит внутри контейнера как `last_config` — JSON-строка, в поле `config` которой классический INI-текст `[Interface]/[Peer]` (включая AWG-поля `Jc`/`Jmin`/`Jmax`/`S1`–`S4`/`H1`–`H4`/`I1`–`I5`).

Референс-реализация обоих шагов (проверена против формата): `scripts/decode_amnezia_vpn.py`.

## Объём

* `IsDirectLink`: распознавать `vpn://`.
* `ParseNode`: ветка `vpn://` → новый `parseAmneziaVPNLink` (`core/config/subscription/node_parser_amnezia.go`):
  * декод base64url (терпимо к padding/переносам строк) → qUncompress (с защитой от zlib-бомбы) → JSON профиля;
  * выбор контейнера: сначала `defaultContainer`, затем по порядку `containers[]`; берётся первый, в котором находится `[Interface]`-текст (рекурсивный поиск, включая вложенные JSON-строки `last_config` → `config`);
  * INI `[Interface]/[Peer]` → канонический `wireguard://`-URI (маппинг как в `docs/ParserConfig.md`); далее **существующий** `parseWireGuardURI` — нормализация CIDR, promote AWG-полей, MTU-кламп AWG → 1280 (SPEC 073) переиспользуются без правок;
  * label узла: `description` → `hostName` → имя контейнера.
* Лимит длины: `vpn://`-ссылки штатно больше `MaxURILength` (профиль может несли сертификаты) — отдельный потолок на ссылку и на распакованный JSON.
* Unit-тесты: синтетическая `vpn://` (qCompress-эмуляция в тесте), AWG- и plain-WG-профили, выбор defaultContainer, профиль без WG-контейнера, битый base64.
* Документация: `docs/ParserConfig.md` (раздел `vpn://`), `docs/release_notes/upcoming.md`.

## Вне объёма

* Импорт прочих протоколов из профиля Amnezia (OpenVPN, Cloak, XRay-контейнеры) — только WG/AWG.
* Несколько узлов из одного профиля: `ParseNode` возвращает один узел; при нескольких WG-контейнерах берётся defaultContainer/первый (предупреждение в лог).
* Импорт голого `[Interface]/[Peer]`-текста, вставленного в UI (отдельная возможная фича; конвертация покрыта `scripts/decode_amnezia_vpn.py`).
* Share-URI обратно в `vpn://` — share для WG/AWG уже есть (`wireguard://`, SPEC 073).

## Критерии приёмки

1. Вставка `vpn://`-ссылки от AWG 2.0 в Sources даёт рабочий AWG-endpoint: AWG-поля в корне endpoint (числа — JSON number, `i1`–`i5` — строки), `mtu` ≤ 1280.
2. Профиль c plain-WG контейнером даёт обычный WG-endpoint (без AWG-ключей, mtu из конфига/1420).
3. Профиль без WG/AWG-контейнера даёт понятную ошибку (имена контейнеров в сообщении), не падение.
4. `go build ./... && go test ./... && go vet ./...` зелёные.
