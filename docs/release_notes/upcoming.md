# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- Every node now takes one path into your config: whatever it came from — a share link, a pasted JSON object, a subscription, a backup or a WireGuard `.conf` — it is checked against a single set of protocol rules and written out by a single emitter. Before, those rules lived in three separate copies that had drifted apart, so the same broken node could be accepted from a link and rejected from JSON.
- A node that lost something now says so. Fields the core would reject are removed with a named reason attached to the node (⚠ in the lists, details in the node card) instead of disappearing silently. Previously a subscription could "update" and quietly strip a node's obfuscation or certificate with nothing to show for it.

### Fixes
- Core pinned to sing-box-lx 1.14.1-lx.7: a REALITY `short_id` longer than 16 hex characters no longer crashes the core (it is a config error now), an unknown `tuic.udp_relay_mode` is rejected at load instead of silently becoming `native`, MASQUE `standard` profile without `uri` gets one clear error, and core errors name the node type and tag (`initialize outbound[0] vless[proxy-de-1]: …`).
- vmess: channel cipher list now matches the core exactly — `aes-128-ctr` (which the core never supported and which killed the whole config) is gone, `aes-128-cfb` is accepted instead of silently falling back to `auto`.
- xhttp: `mode`, `seq_placement`, `session_placement`, `uplink_data_placement`, `x_padding_placement` and `x_padding_method` values outside the core's enum are dropped with an `xhttp_param_reset` warning instead of being passed through and aborting the whole config.
- A hand-written JSON object added as a source is no longer copied into the config verbatim. A typo in a key used to abort the whole config — leaving you with no VPN at all and no hint which node was at fault; now the unknown key is removed and the node says why.
- Several node shapes that the core rejects outright no longer reach your config: an empty server address, a WireGuard node with `jmin` but no `jmax`, a WebSocket/HTTP path with broken percent-encoding, and a naive node carrying TLS options naive does not read. Each of these used to abort the entire config.
- AmneziaWG obfuscation survives. The `h1`–`h4` magic headers, the packet-size fields and the WARP `reserved` triplet are written in the numeric form the core expects, so an AWG node keeps the obfuscation it was configured with.
- Fields your core is too old for (currently `tls.reality.key_share`, which needs 1.14.1-lx.4) are left out when the config is built, and the node keeps working without them. The node itself is unchanged, so the field comes back as soon as you update the core.
- Long but valid share links are accepted again: the launcher's own 8 KB limit is gone in favour of the 64 KB limit the contract defines, which is what the mobile app already used. Amnezia and MASQUE links with full key material were being rejected on the desktop and accepted on the phone.

### Technical / Internal
- Node bodies are produced by `Sanitize` + `Emit` over the contract registry (`contract/registry/**`): the per-protocol emitter chain, the TLS/transport field allowlists and the special-case naive filter are gone. Adding a field to a protocol is now a registry edit, not four code edits.
- The per-field core gate is table-driven from the registry's `min_core`/`platform` instead of one probe per field; `RealityKeyShareSupportProbe` and its cache are removed. Node-level gates (naive/chain/tailscale/AWG3) are unchanged — they drop a node, which the registry does not express.
- `warnings` are recomputed once on load for nodes saved before the pipeline. A node's stored body is rewritten only when the sanitizer actually removes or coerces something, and each such rewrite is a WARN line naming the node and the codes.
- The contract corpus now asserts the body the launcher *stores*, not the output of the old emitter, and a new test feeds every corpus case into one config and runs the pinned core's `check` over it.

## RU
### Основное
- Узел попадает в конфиг одной дорогой: откуда бы он ни пришёл — ссылкой, вставленным JSON-объектом, подпиской, бэкапом или `.conf` WireGuard, — его проверяет один набор правил протокола и записывает один эмиттер. Раньше эти правила жили в трёх разошедшихся копиях, и один и тот же битый узел ссылкой принимался, а объектом — нет.
- Если у узла что-то сняли, он об этом говорит. Поля, которые ядро отвергает, снимаются с названной причиной на самом узле (⚠ в списках, подробности в карточке) вместо молчаливого исчезновения. Прежде подписка «обновлялась», у узла молча срезали обфускацию или сертификат, и увидеть это было негде.

### Исправления
- Ядро закреплено на sing-box-lx 1.14.1-lx.7: REALITY `short_id` длиннее 16 hex больше не роняет ядро паникой (теперь ошибка конфигурации), неизвестный `tuic.udp_relay_mode` отвергается при загрузке, а не молча становится `native`, MASQUE-профиль `standard` без `uri` получает одну понятную ошибку, а ошибки ядра называют тип и тег узла (`initialize outbound[0] vless[proxy-de-1]: …`).
- vmess: набор шифров канала сверен с ядром — `aes-128-ctr`, которого ядро не знало и который валил весь конфиг, убран; `aes-128-cfb` принимается вместо молчаливого отката к `auto`.
- xhttp: значения `mode`, `seq_placement`, `session_placement`, `uplink_data_placement`, `x_padding_placement` и `x_padding_method` вне набора ядра снимаются с кодом `xhttp_param_reset`, а не уезжают в конфиг, роняя его целиком.
- Ручной JSON-объект больше не копируется в конфиг дословно. Опечатка в имени ключа валила весь конфиг — человек оставался вообще без VPN и без подсказки, какой узел виноват; теперь неизвестный ключ снимается, а узел объясняет почему.
- Несколько форм узла, которые ядро отвергает сразу, до конфига больше не доходят: пустой адрес сервера, WireGuard с `jmin` без `jmax`, путь WebSocket/HTTP с битым percent-кодированием и naive с TLS-полями, которых naive не читает. Каждая из них роняла конфиг целиком.
- Обфускация AmneziaWG больше не теряется: магические заголовки `h1`–`h4`, поля размеров пакетов и тройка `reserved` у WARP пишутся в числовой форме, которую ждёт ядро, и AWG-узел сохраняет настроенную обфускацию.
- Поля, которых не знает ваше ядро (сейчас это `tls.reality.key_share`, ему нужно 1.14.1-lx.4), при сборке конфига опускаются, и узел продолжает работать без них. Сам узел не меняется, так что поле вернётся сразу после обновления ядра.
- Длинные, но валидные ссылки снова принимаются: собственный предел лаунчера в 8 КБ заменён контрактными 64 КБ — теми же, что давно у мобильного приложения. Ссылки Amnezia и MASQUE с полным ключевым материалом отбивались на десктопе и принимались на телефоне.

### Техническое / Внутреннее
- Тело узла делают `Sanitize` + `Emit` по реестру контракта (`contract/registry/**`): per-scheme цепочка эмиттера, allowlist-ы полей TLS и транспорта и частный фильтр naive сняты. Новое поле протокола — правка реестра, а не четырёх мест в коде.
- Полевой гейт ядра стал табличным (`min_core`/`platform` реестра) вместо пробы на каждое поле; `RealityKeyShareSupportProbe` и её кэш удалены. Узловые гейты (naive/chain/tailscale/AWG3) не тронуты — они выбрасывают узел, и реестром это не выражается.
- `warnings` разово досчитываются при загрузке у узлов, сохранённых до конвейера. Тело переписывается, только если санитайзер реально что-то снял или привёл, и каждая такая перезапись — строка WARN с тегом узла и кодами.
- Корпус контракта сверяет тело, которое лаунчер СОХРАНЯЕТ, а не вывод старого эмиттера; добавлен тест, который собирает все кейсы корпуса в один конфиг и прогоняет `check` ядром пина.
