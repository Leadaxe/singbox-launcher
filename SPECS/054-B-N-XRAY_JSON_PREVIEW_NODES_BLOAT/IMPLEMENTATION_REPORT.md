# SPEC 054 — Implementation Report

**Status:** ✅ **COMPLETE** — Variant A implemented end-to-end. Build green, 24 packages tests pass, 6 new unit tests added.

---

## Что сделано

### `core/config_service.go`

1. **`extractXrayJSONPreviewNodes(body, limit) ([]string, int)`** (new function, ~30 LOC):
   - Парсит body через `subscription.ParseNodesFromXrayJSONArray`
   - Эмитит первые `limit` нод в формате `<scheme>://<server>:<port>#<tag>` (~50-150 байт/entry)
   - Возвращает `(previewNodes, totalCount)` — totalCount = реальное количество нод в JSON array (правильный `meta.nodes_count_fetched`)
   - На parse-error возвращает `(nil, 0)` — caller контролирует path через `IsXrayJSONArrayBody` upfront
   - UUID/Flow намеренно НЕ включаются в preview — это секреты, не место им в state.json

2. **Dispatcher в `refreshOneSubscriptionSource`**:
   ```go
   if subscription.IsXrayJSONArrayBody(string(res.Body)) {
       merged.PreviewNodes, merged.NodesCountFetched = extractXrayJSONPreviewNodes(res.Body, 50)
   } else {
       merged.PreviewNodes = extractPreviewNodes(res.Body, 50)
       merged.NodesCountFetched = countURIs(res.Body)
   }
   ```
   - Format-aware: JSON array → parse path; всё остальное (base64/text-line) → legacy line-based path
   - `IsXrayJSONArrayBody` уже существует в `subscription` package (SPEC 033), переиспользуется без новых API

### `core/preview_nodes_test.go` (new file, ~140 LOC)

Шесть unit-тестов:

| Test | Покрытие |
|---|---|
| `TestExtractPreviewNodes_LineBased_Base64Decoded` | regression: line-based path всё ещё работает для base64 |
| `TestExtractPreviewNodes_LineBased_TruncationLimit` | limit=50 enforced |
| `TestExtractXrayJSONPreviewNodes_Smoke` | реальная Xray фикстура (`xray_provider_anon.json`) — preview entries < 1KB, URI-like |
| `TestExtractXrayJSONPreviewNodes_SyntheticArray` | 100-node synthetic → preview limit=50, count=100 |
| `TestExtractXrayJSONPreviewNodes_BloatedBodyNotBloatedPreview` | bug repro: 50KB body → preview total << body size |
| `TestPreviewDispatcher_LineBasedFallthrough` | non-JSON body не уходит в Xray-path |

---

## Acceptance criteria — статус

- [x] State.json после fetch Xray-JSON array подписки **не превышает** размер по SPEC 052 (~16 KB / подписку). Test: `BloatedBodyNotBloatedPreview` (preview << body).
- [x] `meta.preview_nodes[i]` — каждый элемент **< 1 KB**. Test: explicit byte-count assertion.
- [x] `meta.nodes_count_fetched` корректное. Test: `SyntheticArray` (100 → 100, не 1).
- [x] `meta.truncated` правильно ставится. Inherited: `effectiveMax := src.MaxNodes; truncated := total > effectiveMax` — без изменений в этой логике, total теперь корректный.
- [x] Unit tests добавлены (см. выше).
- [x] Не ломает существующие base64/text-line форматы. Tests: `LineBased_Base64Decoded`, `LineBased_TruncationLimit`, `LineBasedFallthrough`.

---

## Architecture notes

### Почему Variant A, не Variant B

Variant A (parse → emit URI summaries) выбран потому что:
1. **UX preservation** — preview работает одинаково для всех форматов; юзер в UI видит preview-сводку для Xray JSON подписок как для остальных
2. **Reuse уже-существующей логики** — `ParseNodesFromXrayJSONArray` уже работает в production (SPEC 033), нужен только small shim в preview path
3. **Корректный nodes_count_fetched** — без парсинга было бы "1" для всех JSON подписок, что ломает UI (Total: 1 nodes), truncation, и stats

Variant B (strip preview для JSON) дал бы пустое UI поле для Xray-юзеров.

### Backward compatibility для уже-раздутых state.json

**Миграция не требуется.** При следующем refresh подписки (manual click / auto-update heartbeat) `merged.PreviewNodes` перепишется через новый path. Старые раздутые state.json постепенно «оздоровятся» сами.

Юзеры без Xray JSON подписок не затронуты (path выбирается per-source).

### Performance

Парсинг Xray JSON для preview-generator'а — overhead такой же, как при `ParseNodesFromXrayJSONArray` для главного outbound builder'а. Происходит **раз за fetch** (когда server отдал свежий body), не на каждый UI render. Для типичной подписки 50-100 нод — миллисекунды.

---

## Files changed

```
SPECS/054-B-N-XRAY_JSON_PREVIEW_NODES_BLOAT/
├── SPEC.md                      (existing, 130 LOC)
└── IMPLEMENTATION_REPORT.md     (this file)

core/
├── config_service.go            (+45 LOC — extractXrayJSONPreviewNodes + dispatcher)
└── preview_nodes_test.go        (NEW, 145 LOC, 6 tests)

docs/release_notes/
└── upcoming.md                  (modified — SPEC 054 entry)
```

---

## Known follow-ups

Нет. Bug полностью закрыт. Если в будущем появится новый subscription format с похожей проблемой (например YAML-based или mihomo nested), dispatcher паттерн легко расширяется ещё одной веткой.
