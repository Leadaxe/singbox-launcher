# IMPLEMENTATION_REPORT: 033 — SUBSCRIPTION_XRAY_JSON_ARRAY

## Сделано

- **`DecodeSubscriptionContent`**: валидный JSON-массив `[...]` (plain или после base64) проходит как тело подписки; одиночный `{...}` по-прежнему ошибка.
- **`LoadNodesFromSource`** и **`fetchAndParseSource`** (вкладка Sources): ветка **`IsXrayJSONArrayBody`** → **`ParseNodesFromXrayJSONArray`**; для Xray-нод с цепочкой **`applyTagsToXrayNode`** (prefix/postfix/mask и **`MakeTagUnique`** для main и jump).
- **Парсер элемента** (`xray_json_array.go`, `xray_outbound_convert.go`): VLESS + `vnext`, REALITY и базовые транспорты; **`dialerProxy`** / **`dialer`** → SOCKS; битая цепочка — пропуск ноды + **`WarnLog`**; **`remarks`** → **`Label`**; при непустом **`remarks`** — slug → основной тег **`{slug}`**, jump **`{slug}_jump_server`**, иначе **`xray-{i}`** / **`xray-{i}_jump_server`**.
- **`ParsedNode.Jump`** (`ParsedJump`), **`GenerateOutboundsFromParserConfig`**: сначала SOCKS, затем основной outbound с **`detour`**; **`GenerateNodeJSON`** эмитит **`detour`** при наличии в outbound-мапе.
- **Share URI**: непустой **`detour`** в outbound → **`ErrShareURINotSupported`** (`share_uri_encode.go`).
- **Тесты**: `xray_json_array_test.go` (в т.ч. `testdata/xray_provider_anon.json`), `decoder_test`, `share_uri_encode_test`, `generator_test` (detour).
- **Документация**: `docs/ParserConfig.md`, `docs/ARCHITECTURE.md`, `docs/release_notes/upcoming.md`, пример `docs/examples/xray_subscription_array_sample.json`.

## MVP / границы

- Массив **sing-box** без Xray-`protocol` — пропуск элемента (`debuglog`), парсер **016** не в MVP.
- Одиночный JSON-объект `{...}` — ошибка декодера подписки.

## Проверки

- `go test`, `go vet`, `go build` (по проекту).
