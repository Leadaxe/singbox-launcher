# SPEC 054-B-N — XRAY_JSON_PREVIEW_NODES_BLOAT

**Status:** New (N)
**Type:** Bug (B)
**Discovered:** 2026-05-15 (user feedback while testing SPEC 053 preset bundles)
**Related:** SPEC 033 (XRAY_JSON_ARRAY subscription parser), SPEC 052 (CONNECTIONS_REDESIGN — preview_nodes contract)

---

## Проблема

Подписка формата **Xray JSON Array** (response body `[{...}, {...}, ...]` где каждый элемент — Xray native config с `outbounds[]`) при первом fetch'е раздувает `state.json` примерно в **20-50 раз** относительно ожидаемого размера.

### Конкретный симптом

`state.connections.sources[i].meta.preview_nodes` для одной Xray-JSON подписки выглядит так:

```jsonc
{
  "id": "01KQCTRQBSSF0CCYFD2WWTVY9V",
  "type": "subscription",
  "url": "https://example.com/subs/xray-array",
  "meta": {
    "preview_nodes": [
      "[{\"dns\": {\"hosts\": {\"domain:googleapis.cn\": \"googleapis.com\", \"dot.pub\": [...], ...} ...весь JSON body ~983 KB..."
    ],
    "nodes_count_fetched": 1   // ← неверно: на самом деле там 50+ нод внутри JSON
  }
}
```

Вместо ожидаемого по SPEC 052 формата:

```jsonc
"preview_nodes": [
  "vless://...",
  "vless://...",
  ... (до 50 URI-строк ~200-500 байт каждая)
]
```

### Численно

| Тип подписки | `preview_nodes[i]` размер | Total `preview_nodes` |
|---|---|---|
| base64 / urls | 50 × ~300 байт = ~15 KB | ✅ норма (SPEC 052 §state.json size guarantee) |
| **Xray JSON array** | **1 × ~983 KB** | ❌ **~983 KB на одну подписку** |

Пользовательский state.json при наличии одной Xray-JSON подписки достигает **1.1 MB** — пересекает рекомендуемый порог `262144 bytes` (`loadStateFromFile: state file size exceeds recommended maximum`).

---

## Корень

`core/config/subscription/fetcher.go` (или связанный код в SPEC 052 phase 5 — `refreshSubscriptionsMetaAndCache`) делает first-pass URI counting:

```go
nodesCountFetched := countURIsInBody(body)
previewNodes := firstNURIsAsRaw(body, 50)
```

Для base64/text-line подписок `countURIsInBody` находит N строк (по `\n`-split + URI scheme detection), `previewNodes` берёт первые 50 строк.

Для **Xray JSON array** body это **одна JSON-строка** (не разделена `\n` на URI), первый pass:
- `countURIsInBody` находит "1 line" (потому что весь JSON в одной строке)
- `firstNURIsAsRaw` возвращает **весь body** как `preview_nodes[0]`

То есть **preview-генератор не знает что body — Xray JSON**. Парсинг JSON Array → list of `ParsedNode` уже **умеет** Xray-array (см. SPEC 033 — `xray_json_array.go`), но это происходит **позже**, на этапе LoadNodesFromSource. preview-генератор работает раньше, на raw body.

---

## Fix

Один из двух подходов:

### Вариант A — detect Xray JSON в preview-генераторе

В `refreshSubscriptionsMetaAndCache` (или `FetchSubscriptionWithMeta`) перед formation'ом `preview_nodes`:

```go
if bodyLooksLikeJSONArray(body) {
    // Xray-JSON path: parse через xray_json_array, take first 50 tag'ов / generated URI'ев.
    parsed, err := xrayJSONArrayParse(body)
    if err == nil && len(parsed) > 0 {
        previewNodes = make([]string, 0, min(50, len(parsed)))
        for i, n := range parsed {
            if i >= 50 { break }
            previewNodes = append(previewNodes, n.Tag) // или n.URI() если есть encoder
        }
        nodesCountFetched = len(parsed)
        return
    }
}
// Fallback к старому line-based path.
```

**Плюс:** state.json остаётся в норме, nodes_count_fetched корректный.
**Минус:** требует уметь генерить URI обратно из ParsedNode (или хранить tag вместо URI в preview — это допустимо для UI отображения).

### Вариант B — strip body из preview если он "выглядит как JSON"

Если body начинается с `[` или `{` после whitespace — `preview_nodes` оставить пустым:

```go
if bodyLooksLikeJSON(body) {
    previewNodes = nil
    // nodes_count_fetched всё равно надо считать через парсер
}
```

**Плюс:** simple, no parser dependency in preview generator.
**Минус:** юзер в UI не видит превью нод для Xray-JSON подписок (хотя они нормально работают в config).

### Рекомендация

**Вариант A** — preserve UX (preview работает для всех форматов). Требует мелкий шим в preview-генераторе вызывающий уже-существующий `xray_json_array_parse`.

---

## Acceptance

- [ ] State.json после fetch Xray-JSON array подписки **не превышает** размер по SPEC 052 (~16 KB на одну подписку с meta + preview).
- [ ] `meta.preview_nodes[i]` — каждый элемент **< 1 KB** (одна нода / её tag / её URI).
- [ ] `meta.nodes_count_fetched` корректное (реальное количество нод в JSON array, не "1").
- [ ] `meta.truncated` правильно ставится если nodes_count_fetched > max_nodes.
- [ ] Unit test в `core/config/subscription/`: fixture Xray-JSON array body → meta.preview_nodes имеет ≤ 50 entries, каждый < 1 KB.
- [ ] Не ломает существующие base64/text-line форматы (regression test).

---

## Out of scope

- Polishing UI preview (как отображать ноды без URI) — отдельная задача если потребуется.
- Backward-compat для уже-раздутых state.json у юзеров: при следующем Update подписки preview перепишется корректно — миграция не нужна.

---

## Discovery context

Обнаружено во время debug'а SPEC 053 preset_bundles: user state.json показывал ~1.1 MB размер с warning `state file size (1135290 bytes) exceeds recommended maximum (262144 bytes)`. Анализ через `jq` выявил один источник с `preview_nodes_count: 1, preview_total_bytes: 982976` — что и оказалось dump'ом whole JSON body. Не связано с SPEC 053; pre-existing bug в SPEC 033 area.
