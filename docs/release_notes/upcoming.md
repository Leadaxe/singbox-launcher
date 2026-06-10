# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- Amnezia `vpn://` links (the `.vpn` files exported by Amnezia VPN / AmneziaWG 2.0) can now be pasted directly into Sources/Connections: the launcher decodes the profile, picks the WireGuard/AmneziaWG container and imports it as a regular WG/AWG endpoint (AWG obfuscation params included, MTU safely clamped). (SPEC 075)

- Raw WireGuard/AmneziaWG `.conf` text (`[Interface]/[Peer]`, AWG fields included) can be pasted straight into the Sources Add field — conf blocks are detected before line splitting, converted to `wireguard://` URIs and added as server sources; links pasted alongside keep working. (SPEC 076)

### Technical / Internal
- New `core/config/subscription/node_parser_amnezia.go`: base64url + qCompress (4-byte BE size + zlib) profile decoding with size caps, recursive `last_config` → `[Interface]/[Peer]` extraction, conversion to the canonical `wireguard://` URI (reuses SPEC 073 parsing/clamping). Reference decoder for debugging: `scripts/decode_amnezia_vpn.py`.
- New `core/config/subscription/wgconf_text.go` (`ExtractWGConfBlocks`/`ConvertWGConfText`) + hook in `classifyInputLines`: pasted conf text never stored raw — converted to canonical URIs at classification time. (SPEC 076)

## RU
### Основное
- Ссылки Amnezia `vpn://` (файлы `.vpn` из Amnezia VPN / AmneziaWG 2.0) теперь можно вставлять прямо в Sources/Connections: лаунчер сам декодирует профиль, находит WireGuard/AmneziaWG-контейнер и импортирует его как обычный WG/AWG-узел (с параметрами обфускации AWG и безопасным клампом MTU). (SPEC 075)

- Голый текст `.conf` WireGuard/AmneziaWG (`[Interface]/[Peer]`, включая AWG-поля) теперь можно вставлять прямо в поле Add на вкладке Sources — conf-блоки распознаются до построчного разбора, конвертируются в `wireguard://`-URI и добавляются как server-источники; ссылки в той же вставке продолжают работать. (SPEC 076)

### Техническое / Внутреннее
- Новый `core/config/subscription/node_parser_amnezia.go`: декодирование base64url + qCompress (4 байта BE-длины + zlib) с лимитами размера, рекурсивное извлечение `last_config` → `[Interface]/[Peer]`, конвертация в канонический `wireguard://`-URI (переиспользует разбор/кламп SPEC 073). Референс-декодер для отладки: `scripts/decode_amnezia_vpn.py`.
- Новый `core/config/subscription/wgconf_text.go` (`ExtractWGConfBlocks`/`ConvertWGConfText`) + врезка в `classifyInputLines`: вставленный conf-текст не хранится сырым — конвертируется в канонические URI на этапе классификации. (SPEC 076)
