# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
-

### Technical / Internal
- Shared contract 1.1.47: XHTTP now also reads the `sessionIDPlacement` / `sessionIDKey` spellings that Xray itself writes — previously those keys were not read by any source and the session settings were lost silently.
- Shared contract 1.1.48: source wrappers — `vpn://` holding a bare wg-quick/AWG `.conf`, the full `amneziawg://` scheme, a single Xray config (not an array), bandwidth written with a unit (`"100mbps"`), and Hysteria2 obfuscation carried in `finalmask`. Each of these gave zero nodes (or a partial list) from a perfectly healthy subscription. Routing commands for other clients (`incy://routing/…`, `happ://routing/…`) are now skipped quietly instead of being reported as errors.
- Shared contract 1.1.50: the Xray dialect audit — see Fixed. Internally: a JSON array or map in the source now reaches the registry record (previously an array spelling of a field was silently unreadable), and the declared `on_invalid` of the transport selector is actually executed.
- Shared contract 1.1.49: every dropped entry in the contract envelope now carries a machine `code` and the element `index`, including entries that could not be parsed at all (new code `form_unrecognized`); the group genus is declared as the sing-box body type; the “comment with `=` is not a node name” rule for `.conf` moved from engine code into the registry.

### Fixed
- Xray configs: WireGuard, SOCKS and HTTP outbounds are imported at last — such a node used to disappear entirely, reported as an unsupported protocol.
- Xray configs: a node asking for the kcp or quic transport is now rejected with a clear reason. Before, it was imported as a plain TCP node that looked fine and could never connect.
- Xray configs: `alpn`, `mux`, WebSocket and HTTP headers, transport timeouts, TLS versions, cipher suites, certificates and ECH settings are no longer dropped. A node whose server only speaks h2 or h3 failed to connect without `alpn`; a server requiring multiplexing refused single connections.
- A list with one unusable element (for example `alpn` containing a number) now loses just that element instead of the whole setting, so the working values next to it survive.
- Long but perfectly valid links — a VLESS link with a post-quantum key, or an Amnezia profile with certificates — are no longer rejected as too long.
- Hysteria / Hysteria2: a port-hopping range outside 0–65535 or with leading zeros (`99999:99999`, `00443:00444`) is now dropped from the node with a warning instead of making the core reject the whole configuration.

## RU
### Основное
-

### Техническое / Внутреннее
- Общий контракт 1.1.47: XHTTP читает и те написания `sessionIDPlacement` / `sessionIDKey`, которые пишет сам Xray — прежде эти ключи не читал ни один источник, и настройки сессии терялись молча.
- Общий контракт 1.1.48: обёртки источника — `vpn://` с голым `.conf` wg-quick/AWG, полное имя схемы `amneziawg://`, одиночный конфиг Xray (не массив), полоса с единицей измерения (`"100mbps"`) и обфускация Hysteria2 из `finalmask`. Каждый из этих входов давал ноль узлов (или часть) при полностью исправной подписке. Команды маршрутизации для других клиентов (`incy://routing/…`, `happ://routing/…`) теперь пропускаются тихо, а не показываются ошибкой.
- Общий контракт 1.1.50: аудит диалекта Xray — см. «Исправлено». Внутреннее: массив и карта в источнике теперь доезжают до записи реестра (прежде массивное написание поля было нечитаемым молча), а объявленный `on_invalid` селектора транспорта действительно исполняется.
- Общий контракт 1.1.49: каждая отбраковка в конверте контракта несёт машинный `code` и `index` элемента, включая записи, которые не удалось прочитать вовсе (новый код `form_unrecognized`); род группы объявлен типом тела sing-box; правило «комментарий с `=` — не имя узла» для `.conf` перенесено из кода движка в реестр.

### Исправлено
- Конфиги Xray: outbound'ы WireGuard, SOCKS и HTTP наконец импортируются — прежде такой узел пропадал целиком с диагнозом «протокол не поддержан».
- Конфиги Xray: узел, запрашивающий транспорт kcp или quic, теперь отбраковывается с внятной причиной. Прежде он импортировался обычным TCP-узлом, который выглядел рабочим и не мог соединиться никогда.
- Конфиги Xray: больше не теряются `alpn`, `mux`, заголовки WebSocket и HTTP, таймауты транспортов, версии TLS, наборы шифров, сертификаты и настройки ECH. Узел, чей сервер говорит только по h2 или h3, без `alpn` не соединялся; сервер, требующий мультиплексирования, не принимал одиночные соединения.
- Список с одним негодным элементом (например число в `alpn`) теперь теряет только этот элемент, а не всю настройку — рабочие значения рядом с ним остаются.
- Длинные, но полностью корректные ссылки — VLESS с постквантовым ключом или профиль Amnezia с сертификатами — больше не отбраковываются как слишком длинные.
- Hysteria / Hysteria2: диапазон прыжков по портам вне 0–65535 или с ведущими нулями (`99999:99999`, `00443:00444`) теперь снимается с узла с предупреждением, а не заставляет ядро отвергнуть весь конфиг.
