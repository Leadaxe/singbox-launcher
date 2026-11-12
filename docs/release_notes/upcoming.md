# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- XHTTP: Xray `sessionIDTable` / `sessionIDLength` (links, `extra`, Xray JSON) now reach the node as `session_table` / `session_length` — servers behind CDNs that reject dashed-UUID session ids work out of the box (contract 1.1.168, LxBox#211). Half a pair is dropped with a warning instead of failing the core.
- Xray subscriptions: an element with `mux` turned on no longer gets sing-box `multiplex` — Xray mux (Mux.Cool) is a different protocol, and such nodes did not connect at all. The node now imports without mux, with an info note (contract 1.1.169). An explicit `multiplex` in sing-box JSON is kept as is.

### Technical / Internal
-

## RU
### Основное
- XHTTP: настройки Xray `sessionIDTable` / `sessionIDLength` (ссылка, `extra`, Xray JSON) доезжают до узла как `session_table` / `session_length` — серверы за CDN, режущим UUID-идентификатор сессии, работают без правки (контракт 1.1.168, LxBox#211). Половина пары снимается с предупреждением, а не валит ядро.
- Подписки Xray: элемент с включённым `mux` больше не получает `multiplex` sing-box — Xray mux (Mux.Cool) другой протокол, и такие узлы не соединялись вовсе. Теперь узел импортируется без mux, с информационной заметкой (контракт 1.1.169). Явный `multiplex` в sing-box JSON остаётся как есть.

### Техническое / Внутреннее
-
