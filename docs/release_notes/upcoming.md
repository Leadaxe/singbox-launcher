# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Hysteria2 links from 3x-ui with gecko obfuscation no longer lose the packet size range: `minPacketSize`/`maxPacketSize` are read, and `security=tls` is accepted silently (contract 1.1.54).
- VLESS nodes with Vision flow over a transport (xhttp, ws, …) keep `flow` when the node has VLESS Encryption: the flow was stripped before and the server dropped the connection (contract 1.1.55).
- XHTTP `uplink_data_placement=header|cookie` without `mode` gets `mode: packet-up` filled in on every input, sing-box JSON included; with another explicit mode the placement is dropped instead of the core rejecting the whole config. `body`/`auto` placements are no longer touched: links with `uplinkDataPlacement=body` and `mode=stream-one` used to get a false warning. Shadowsocks `plugin_opts` without `plugin` is dropped (contract 1.1.56).

### Technical / Internal
- Contract 1.1.56 primitives: body `condition` accepts value predicates on paths (`in`/`not_in`/scalar, mapper `when` grammar), relations take `when`, mapper `when` gets the `$value` selector (own source value; a miss is a silent skip, no `on_when_false`). Sanitizer rules: relations and conditions see fields of the object still being walked; an empty string or an `absent_values` literal counts as a missing key for `default_when`; an empty string does not satisfy `any_set`. All engine-level, no scheme names in code.
- **Core pinned to sing-box-lx 1.14.2-lx.3** (was 1.14.2-lx.2). Vision (`flow: xtls-rprx-vision`) now runs on top of VLESS Encryption on any transport, `xhttp` included (fork SPEC 105, sing-box-lx#29); together with contract 1.1.55 such nodes connect instead of being dropped by the server. No configuration or state migration.

## RU
### Основное
- Ссылки hysteria2 из 3x-ui с gecko-обфускацией больше не теряют диапазон размеров пакетов: `minPacketSize`/`maxPacketSize` читаются, `security=tls` принимается молча (контракт 1.1.54).
- VLESS-узлы с Vision поверх транспорта (xhttp, ws и др.) сохраняют `flow`, если у узла есть VLESS Encryption: раньше flow снимался, и сервер рвал соединение (контракт 1.1.55).
- XHTTP `uplink_data_placement=header|cookie` без `mode` получает `mode: packet-up` на любом входе, включая sing-box JSON; при явном другом режиме placement снимается, а не роняет весь конфиг ядра. `body`/`auto` больше не трогаются: ссылки с `uplinkDataPlacement=body` и `mode=stream-one` раньше получали ложное предупреждение. У Shadowsocks `plugin_opts` без `plugin` снимается (контракт 1.1.56).

### Техническое / Внутреннее
- Примитивы контракта 1.1.56: `condition` тела принимает предикаты по значению путей (`in`/`not_in`/скаляр, грамматика `when` маппера), у связей появился `when`, в `when` маппера — селектор `$value` (собственное значение записи; промах — молчаливый пропуск без `on_when_false`). Нормы санитайзера: связи и условия видят поля объекта, обход которого ещё идёт; пустая строка и литерал `absent_values` для `default_when` равны отсутствию ключа; пустая строка не выполняет `any_set`. Всё на уровне движка, без имён схем в коде.
- **Пин ядра sing-box-lx 1.14.2-lx.3** (было 1.14.2-lx.2). Vision (`flow: xtls-rprx-vision`) теперь работает поверх VLESS Encryption на любом транспорте, `xhttp` в том числе (SPEC 105 форка, sing-box-lx#29); вместе с контрактом 1.1.55 такие узлы подключаются, а не рвутся сервером. Миграции конфига и состояния нет.
