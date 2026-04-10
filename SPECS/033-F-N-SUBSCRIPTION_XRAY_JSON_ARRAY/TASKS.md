# TASKS: 033 — SUBSCRIPTION_XRAY_JSON_ARRAY

## Этап 0 — декодер и ветка массива (MVP)

- [x] **016** без кода: правку `decoder.go` (тело `[` + валидный JSON-массив не отклонять) и ветку загрузчика/визарда делать **в рамках 033** (см. **016-F-C IMPLEMENTATION_REPORT**).
- [x] **MVP:** обрабатывать только **Xray**-элементы массива; элементы в стиле sing-box (016) — пропуск + `debuglog`, реализация 016 — **follow-up**.

## Этап 1 — модель и генерация

- [x] Расширить `configtypes.ParsedNode` (или согласованную структуру) полями для **jump** (SOCKS) при наличии цепочки.
- [x] Добавить в `GenerateNodeJSON` (или вспомогательную функцию, вызываемую из `GenerateOutboundsFromParserConfig`) поддержку **`detour`** и при `jump` — **две** JSON-строки в правильном порядке.
- [x] Селекторы / списки: одна запись `ParsedNode` на логическую ноду; jump не добавляет вторую строку в список серверов (только второй outbound в JSON).

## Этап 2 — парсер Xray элемента

- [x] Реализовать разбор одного элемента массива: `outbounds`, индекс по `tag`, выбор основного VLESS по **PLAN §3**.
- [x] Реализовать извлечение SOCKS jump по `dialerProxy` и маппинг в sing-box `socks` outbound map; **без** `username`/`password`, если в Xray нет пользователя.
- [x] При отсутствии outbound с тегом `dialerProxy` или при типе не SOCKS: **пропуск ноды для этого элемента** + **`WarnLog`** (SPEC §2.4).
- [x] Реализовать конвертацию VLESS (`vnext`, `streamSettings`, reality) → поля, совместимые с `GenerateNodeJSON`.

## Этап 3 — интеграция подписки

- [x] Подключить парсер в `LoadNodesFromSource` при теле `[...]` после декодера.
- [x] Просмотр источников визарда: `ui/wizard/tabs/source_tab.go` — `fetchAndParseSource` (тот же сценарий, что превью по URL).
- [x] Применить `MakeTagUnique`, TagPrefix/Postfix/Mask, `MaxNodesPerSubscription`, Skip — как у обычных подписок (`applyTagsToXrayNode`).

## Этап 4 — Share URI и границы

- [x] «Копировать ссылку»: outbound с **`detour`** → `ErrShareURINotSupported` (`ShareURIFromOutbound`).
- [x] Тест на явный отказ в `share_uri_encode_test.go`.

## Этап 5 — тесты и документация

- [x] Юнит-тесты: `xray_json_array_test.go`, `detour` в `generator_test.go`, decoder, share URI.
- [x] `go test`, `go vet`, `go build` (проверено в сессии).
- [x] Обновить `docs/ParserConfig.md`, `docs/release_notes/upcoming.md`, пример `docs/examples/xray_subscription_array_sample.json`.
- [x] `IMPLEMENTATION_REPORT.md`.
