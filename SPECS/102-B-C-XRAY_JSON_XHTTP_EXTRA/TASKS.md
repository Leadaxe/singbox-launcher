# TASKS — 102-B XHTTP `extra` в Xray-JSON ветке

Файлы, которые задача имеет право менять:

- `core/config/subscription/node_parser_transport.go`
- `core/config/subscription/xray_outbound_convert.go`
- `core/config/subscription/xray_protocols_test.go`
- `docs/release_notes/upcoming.md`
- артефакты задачи в `SPECS/102-B-N-XRAY_JSON_XHTTP_EXTRA/`

---

## Этап 1 — общее ядро сборки транспорта

- [x] 1.1 Ввести `xhttpBuildTransport(primary, fallback map[string]string)` в
      `node_parser_transport.go`: перенести в него логику
      `xhttpTransportFromQuery` (mode/path/host, `x_padding_bytes`, bool-флаги,
      `xhttpStringFields`, `xhttpRangeFields`).
- [x] 1.2 Добавить case-insensitive чтение ключа из плоской карты
      (эквивалент `queryGetFold` для `map[string]string`).
- [x] 1.3 `xhttpTransportFromQuery(q)` → тонкая обёртка над ядром
      (`primary` = `xhttpMergeSource(q)`, `fallback` = свёртка `q`).
- [x] 1.4 `go test ./core/config/subscription/...` — существующие тесты
      (`xhttp_v2_test.go`) зелёные **без правок**: URI-ветка не изменилась.

## Этап 2 — Xray-ветка на общее ядро

- [x] 2.1 В `xrayTransportFromStreamSettings` (`case "xhttp", "splithttp"`)
      собрать плоский слой из скаляров `xhttpSettings` через
      `xhttpStringifyJSON`.
- [x] 2.2 Собрать приоритетный слой из `xhttpSettings.extra` (если объект).
- [x] 2.3 Вызвать `xhttpBuildTransport(extra, flat)`; удалить ручное чтение
      только `path`/`host`/`mode`.
- [x] 2.4 ~~Комментарий: `xmux` намеренно не переносится (R7, вне scope).~~
      **Отклонение от плана:** реализация не оставила `xmux` за scope — он
      перенесён во всех трёх слоях (парсер/конвертер/эмиттер) с покрытием
      тестами. R7 предполагал обратное; расхождение зафиксировано в
      IMPLEMENTATION_REPORT.md, TASKS/PLAN задним числом не переписывались.
- [x] 2.5 Битый / не-объектный `extra` не роняет узел (R6).

## Этап 3 — тесты

- [x] 3.1 `uplinkHTTPMethod: "GET"` → `"uplink_http_method": "GET"`.
- [x] 3.2 `xPaddingBytes` `"50-150"` и `"0-0"` доходят дословно.
- [x] 3.3 Числа из `extra` (`scMaxBufferedPosts: 30`) → строки (`"30"`),
      без `1e+06`.
- [x] 3.4 Плоское поле читается; одноимённое в `extra` его перекрывает.
- [x] 3.5 Битый `extra` (не объект / невалидный) — узел собран, без мусора.
- [x] 3.6 **Тест паритета:** один набор полей через URI и через Xray-JSON даёт
      идентичный транспорт (гейт против повторного расхождения веток).
- [x] 3.7 Фикстуры трёх узлов из SPEC §1.2 — адреса и ключи обезличены.

## Этап 4 — проверка и закрытие

- [x] 4.1 `go build ./...`, `go test ./...`, `go vet ./...` — чисто.
- [x] 4.2 Живая проверка: три узла через `/proxies` + `/action/ping-all`,
      результат в IMPLEMENTATION_REPORT (два узла работают, третий мёртв по
      причине на стороне сервера — рядом лежит и не-XHTTP узел того же
      провайдера).
- [x] 4.3 `docs/release_notes/upcoming.md` (EN → Highlights, RU → Основное).
- [x] 4.4 Заполнить `IMPLEMENTATION_REPORT.md`.
- [x] 4.5 Папка переименована в `102-B-C-…` после живой проверки.

---

## Не входит

- `xmux` (R7) — отдельная задача после подтверждения поддержки ядром.
- Дефект массового ping-all (ложные Error на живых узлах) — отдельная задача.
- `outbound_jsonbuilder.go` — allowlist уже содержит нужные ключи.
