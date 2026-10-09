# PLAN 159

Правка — в данных контракта; код движка не меняется (`core/config/linkmap` уже исполняет `maps_to` и кодирует значение query через `queryEscape`).

## Реестр

`contract/registry/transports.json`:

- `blocks.uri.xhttp.path` — `extract` `^(?P<path>[^?]*)` → `maps_to: transport.path`; `decode_extra` (mode path, 2 прохода) прежний. `impl` — норма Xray, SPEC 119, issue #36, почему прежний срез был ошибкой, что ws/httpupgrade не меняются.
- `blocks.xray.xhttp.path` — то же, без `decode_extra` (его не было).
- `transports.xhttp.params.path` и `body.variants.xhttp.fields.path` — `desc_en`/`desc_ru`/`impl`: хвост `?` уходит query запроса.

Обратный ход отдельной правки не требует: `emitState.putEncoded` → `queryEscape` (`url.QueryEscape`) кодирует `?`, `=`, `&` внутри значения.

## Контракт

- `contract/VERSION` → 1.1.115; строка в таблице «Версии» `contract/README.md`; `contract/TASKS_LXBOX.md` §112.
- `go generate ./contract/...` — `contract/docs/generated/**`.
- Корпус: `uri/vless/xhttp_path_query_tail_trimmed` → `xhttp_path_query_tail_kept`, `uri/vless/xhttp_path_ed_tail_stripped` → `xhttp_path_ed_tail_kept` (ожидания с хвостом); новые `uri/vless/xhttp_path_proxyip_query_kept`, `body/xray/xhttp_path_query_kept`.
- `core/config/linkmap/testdata/emit_snapshot.json` не трогается: это снимок рукописного эмиттера, его тела пути с хвостом не несут.

## Тесты

- Новый `core/config/xhttp_path_query_test.go` (`TestXHTTPPathQueryVerbatim`): три пути × ссылка (+ круг через тело состояния и ссылку «Поделиться»), Xray-JSON, тело sing-box; ws-регрессия `?ed=N`.
- `core/config/subscription/xhttp_v2_test.go`: `TestXHTTPv2_PathQueryTailTrimmed` → `TestXHTTPv2_PathQueryTailKept`.

## Документы

`docs/release_notes/upcoming.md` (EN/RU), `SPECS/README.md`. `SPECS/features/subscriptions.md` не правится: поля протоколов фича отдаёт реестру.
