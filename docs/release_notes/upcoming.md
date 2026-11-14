# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- XHTTP: Xray `sessionIDTable` / `sessionIDLength` (links, `extra`, Xray JSON) now reach the node as `session_table` / `session_length` — servers behind CDNs that reject dashed-UUID session ids work out of the box (contract 1.1.168, LxBox#211). Half a pair is dropped with a warning instead of failing the core.
- Xray subscriptions: an element with `mux` turned on no longer gets sing-box `multiplex` — Xray mux (Mux.Cool) is a different protocol, and such nodes did not connect at all. The node now imports without mux, with an info note (contract 1.1.169). An explicit `multiplex` in sing-box JSON is kept as is.
- **Sniff guard replaces the QUIC decoy.** Settings has **Sniff guard: TLS** and **Sniff guard: QUIC**, both on by default, plus **only masked SNI** (on: the guard covers only nodes whose SNI differs from the server address) and **remember relays** (off): the config carries the core's `lx.sniff_guard` block, which keeps a relay or router that sniffs the first handshake packet from redirecting a node by its SNI, and learns per path where it is needed. The four **Anti-sniff decoy** options are gone; their saved values are ignored. The block takes effect with core `1.14.4-lx.34` and newer; an older core runs without it and keeps its own behaviour (contract 1.1.170).
- Windows: files and folders with non-Latin names (Cyrillic and others) can be picked in the system file dialog again — Taildrop **Send files**, config import and backup save failed on such paths because the path came back in the console code page.
- Logs window, **Core** tab: multi-line entries (goroutine dumps from the service) no longer paint over each other, and terminal colour codes are removed.
- Russian UI: the pattern row label in the emoji picker and in the server filter no longer runs under the field.

### Technical / Internal
- `sing-box-lx 1.14.4-lx.35` (was 1.14.4-lx.33). A breaking config change in the core since lx.34: `lx.quic_sniff_decoy` is removed (any value fails the load) and a node routed through another node no longer gets its TLS ClientHello split automatically — both are replaced by `lx.sniff_guard`. The release also carries the upstream stable sync up to 2026-11-14 (uTLS, REALITY client, sing-tun).
- Contract 1.1.170: `registry/vars.json` drops the four `quic_sniff_decoy*` variables and gains `sniff_guard_tls`, `sniff_guard_quic`, `sniff_guard_sni_mismatch`, `sniff_guard_persist`; `registry/core_config.json` gates `lx.sniff_guard` at `1.14.4-lx.34` and forbids `lx.quic_sniff_decoy` for any core. The config gate now drops forbidden keys when the executing core's version is unknown too (it used to skip the gate entirely).

## RU
### Основное
- XHTTP: настройки Xray `sessionIDTable` / `sessionIDLength` (ссылка, `extra`, Xray JSON) доезжают до узла как `session_table` / `session_length` — серверы за CDN, режущим UUID-идентификатор сессии, работают без правки (контракт 1.1.168, LxBox#211). Половина пары снимается с предупреждением, а не валит ядро.
- Подписки Xray: элемент с включённым `mux` больше не получает `multiplex` sing-box — Xray mux (Mux.Cool) другой протокол, и такие узлы не соединялись вовсе. Теперь узел импортируется без mux, с информационной заметкой (контракт 1.1.169). Явный `multiplex` в sing-box JSON остаётся как есть.
- **Защита от сниффинга вместо QUIC-приманки.** В Settings — **Sniff guard: TLS** и **Sniff guard: QUIC**, обе включены по умолчанию, плюс **only masked SNI** (включена: защищаются только узлы, у которых SNI не совпадает с адресом сервера) и **remember relays** (выключена): в конфиг пишется блок ядра `lx.sniff_guard`. Он не даёт релею или роутеру, который читает первый пакет рукопожатия, увести узел по SNI, и сам учится, на каких путях защита нужна. Четыре настройки **Anti-sniff decoy** убраны, их сохранённые значения игнорируются. Блок действует с ядром `1.14.4-lx.34`; ядро старше работает без него, как раньше (контракт 1.1.170).
- Windows: файлы и папки с нелатинскими именами (кириллица и другие) снова выбираются в системном диалоге — Taildrop **Send files**, импорт конфига и сохранение бэкапа падали на таких путях: путь приходил в кодовой странице консоли.
- Окно логов, вкладка **Core**: многострочные записи (дампы горутин от службы) больше не рисуются друг поверх друга, цветовые коды терминала убраны.
- Русский интерфейс: подпись строки шаблона в выборе эмодзи и в фильтре серверов больше не уезжает под поле.

### Техническое / Внутреннее
- `sing-box-lx 1.14.4-lx.35` (было 1.14.4-lx.33). Несовместимое изменение конфига ядра с lx.34: ключ `lx.quic_sniff_decoy` удалён (любое значение — отказ загрузки), а узлу, который ведётся через другой узел, ядро больше не режет TLS ClientHello само — оба механизма заменяет `lx.sniff_guard`. В релизе также синк с upstream stable до 14.11.2026 (uTLS, клиент REALITY, sing-tun).
- Контракт 1.1.170: из `registry/vars.json` сняты четыре переменные `quic_sniff_decoy*`, заведены `sniff_guard_tls`, `sniff_guard_quic`, `sniff_guard_sni_mismatch`, `sniff_guard_persist`; в `registry/core_config.json` блок `lx.sniff_guard` получил границу `1.14.4-lx.34`, а `lx.quic_sniff_decoy` запрещён для любого ядра. Гейт ключей конфига снимает запрещённые ключи и при неизвестной версии ядра-исполнителя (раньше гейт в этом случае не работал вовсе).
