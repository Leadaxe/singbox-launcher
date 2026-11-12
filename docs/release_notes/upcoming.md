# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- XHTTP: Xray `sessionIDTable` / `sessionIDLength` (links, `extra`, Xray JSON) now reach the node as `session_table` / `session_length` — servers behind CDNs that reject dashed-UUID session ids work out of the box (contract 1.1.168, LxBox#211). Half a pair is dropped with a warning instead of failing the core.

### Technical / Internal
-

## RU
### Основное
- XHTTP: настройки Xray `sessionIDTable` / `sessionIDLength` (ссылка, `extra`, Xray JSON) доезжают до узла как `session_table` / `session_length` — серверы за CDN, режущим UUID-идентификатор сессии, работают без правки (контракт 1.1.168, LxBox#211). Половина пары снимается с предупреждением, а не валит ядро.

### Техническое / Внутреннее
-
