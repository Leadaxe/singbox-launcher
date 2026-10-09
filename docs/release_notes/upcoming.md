# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **XHTTP path keeps its `?query` part.** A VLESS/XHTTP node whose path is `/?proxyip=…` (Cloudflare Worker relays such as edgetunnel) used to lose everything after `?` on import from a link or Xray JSON, so the relay got no `proxyip`. The path is now kept as written, like Xray does; with a sing-box-lx core that has SPEC 119 the tail goes to the server as the request query. Shared links encode the `?` inside `path=`. Nodes that differ only in that tail are no longer merged as duplicates. Already imported subscription nodes pick the fix up on the next update; a node added by pasting a link needs to be added again. WebSocket `?ed=N` handling is unchanged. Contract 1.1.115.

### Technical / Internal
- **Release gate: ship only together with a core that has sing-box-lx SPEC 119** (`RequiredCoreVersion` is still `1.14.2-lx.12`, which predates it). An older core glues the XHTTP path tail into the path (`/x?ed=2048` → `/x%3Fed=2048`) and the server answers 404, so XHTTP nodes with a `?` tail that used to connect (tail stripped) would break.

## RU
### Основное
- **Путь XHTTP сохраняет хвост `?query`.** У узла VLESS/XHTTP с путём `/?proxyip=…` (релеи Cloudflare Worker, например edgetunnel) при импорте ссылки или Xray JSON терялось всё после `?`, и релей не получал `proxyip`. Теперь путь берётся как написан, как у Xray; ядро sing-box-lx с SPEC 119 отправляет хвост серверу query запроса. В ссылке «Поделиться» `?` внутри `path=` кодируется. Узлы, различающиеся только этим хвостом, больше не схлопываются как дубли. Уже импортированные узлы подписки исправятся при следующем обновлении; узел, добавленный ссылкой вручную, нужно добавить заново. Обработка `?ed=N` у WebSocket не менялась. Контракт 1.1.115.

### Техническое / Внутреннее
- **Условие выпуска: только вместе с ядром, где есть sing-box-lx SPEC 119** (`RequiredCoreVersion` пока `1.14.2-lx.12`, оно раньше SPEC 119). Старое ядро клеит хвост пути XHTTP в путь (`/x?ed=2048` → `/x%3Fed=2048`), сервер отвечает 404 — узлы XHTTP с хвостом `?…`, которые прежде соединялись (хвост срезался), перестанут работать.
