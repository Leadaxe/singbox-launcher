# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Hysteria2 links from 3x-ui with gecko obfuscation no longer lose the packet size range: `minPacketSize`/`maxPacketSize` are read, and `security=tls` is accepted silently (contract 1.1.54).
- VLESS nodes with Vision flow over a transport (xhttp, ws, …) keep `flow` when the node has VLESS Encryption: the flow was stripped before and the server dropped the connection (contract 1.1.55).

### Technical / Internal
- **Core pinned to sing-box-lx 1.14.2-lx.3** (was 1.14.2-lx.2). Vision (`flow: xtls-rprx-vision`) now runs on top of VLESS Encryption on any transport, `xhttp` included (fork SPEC 105, sing-box-lx#29); together with contract 1.1.55 such nodes connect instead of being dropped by the server. No configuration or state migration.

## RU
### Основное
- Ссылки hysteria2 из 3x-ui с gecko-обфускацией больше не теряют диапазон размеров пакетов: `minPacketSize`/`maxPacketSize` читаются, `security=tls` принимается молча (контракт 1.1.54).
- VLESS-узлы с Vision поверх транспорта (xhttp, ws и др.) сохраняют `flow`, если у узла есть VLESS Encryption: раньше flow снимался, и сервер рвал соединение (контракт 1.1.55).

### Техническое / Внутреннее
- **Пин ядра sing-box-lx 1.14.2-lx.3** (было 1.14.2-lx.2). Vision (`flow: xtls-rprx-vision`) теперь работает поверх VLESS Encryption на любом транспорте, `xhttp` в том числе (SPEC 105 форка, sing-box-lx#29); вместе с контрактом 1.1.55 такие узлы подключаются, а не рвутся сервером. Миграции конфига и состояния нет.
