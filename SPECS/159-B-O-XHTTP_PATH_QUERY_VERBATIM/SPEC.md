# SPEC 159 — путь XHTTP теряет хвост `?query`

**Фича:** [SPECS/features/subscriptions.md](../features/subscriptions.md) (диалекты: ссылка, Xray, sing-box). Транспорт — [SPEC 071](../071-F-N-XHTTP_TRANSPORT/SPEC.md).
**Внешнее:** issue [Leadaxe/sing-box-lx#36](https://github.com/Leadaxe/sing-box-lx/issues/36); ядро — sing-box-lx SPEC 119 (`XHTTP_PATH_QUERY`). Контракт 1.1.115, TASKS_LXBOX §112.

## Проблема

Узел VLESS+XHTTP с `transport.path` вида `/?proxyip=192.0.2.10` — релей Cloudflare Worker (edgetunnel): Worker берёт `proxyip` из query запроса. Норма Xray (`transport/internet/splithttp/config.go`, `GetNormalizedPath`/`GetNormalizedQuery`): всё после первого `?` в `path` — query запроса, клиент шлёт его как есть. Ядро sing-box-lx делает так же с SPEC 119.

Разбор лаунчера срезал хвост раньше ядра. Правило `path` у xhttp в `contract/registry/transports.json` было списано с ws `?ed=N` (`extract` `^(?P<path>[^?]*)`, пометка «иначе 404»):

| path в ссылке | transport.path в конфиге |
|---|---|
| `/?proxyip=149.56.109.62` | `/` |
| `/base?x=1` | `/base` |
| `/proxyip=149.56.109.62` | `/proxyip=149.56.109.62` |

У xhttp нет early data, а 404 давало ядро до SPEC 119: оно клеило хвост в путь (`/%3Fproxyip=…/`). Срез прятал это ценой потери query.

## Требования

1. `transport.path` у `type=xhttp` берётся дословно, вместе с хвостом `?…`, на всех входах: share-ссылка, Xray-JSON (`xhttpSettings.path`, `splithttpSettings.path`), тело sing-box, тело узла в `state.json` и сгенерированный конфиг.
2. ws и httpupgrade не меняются: у ws `?ed=N` раскладывается в `max_early_data` (+ `early_data_header_name`), у httpupgrade срезается.
3. Ссылка из узла кодирует `?` (а также `=`, `&`) внутри значения `path=`, и хвост не смешивается с query самой ссылки; разбор такой ссылки возвращает тот же путь.
4. Тексты реестра (`desc_*`, `impl`) говорят, что путь xhttp дословный по норме Xray, со ссылкой на sing-box-lx SPEC 119 / issue #36. Контракт поднимается до 1.1.115, корпус приводится к норме.

## Поведение

- Узлы с хвостом пути на ядре до SPEC 119 получают 404 (ядро клеит хвост в путь); разбор старое ядро не обходит.
- Подпись дедупа подписки — эмиссия узла без тега, поэтому узлы, различающиеся только хвостом пути (edgetunnel с разными `proxyip`), больше не схлопываются в один. Идентичность узла (тег) не затронута.
- Сохранённое тело узла не перечитывается из происхождения: узлы подписки получают путь с хвостом при следующем обновлении, узел, добавленный ссылкой вручную, — после повторного добавления.

## Критерии приёмки

- Три пути из таблицы доезжают до `transport.path` без изменений через ссылку, Xray-JSON, тело sing-box и круг через тело `state.json`; ссылка из узла с `?` в пути содержит `%3F` и разбирается обратно в тот же путь.
- ws-ссылка с `path=/ws?ed=2560` даёт `path: /ws`, `max_early_data: 2560`.
- Корпус контракта: ожидания `uri/vless/xhttp_path_*_kept` с хвостом, новые кейсы `uri/vless/xhttp_path_proxyip_query_kept` и `body/xray/xhttp_path_query_kept`; раннеры корпуса и круга эмита зелёные.
