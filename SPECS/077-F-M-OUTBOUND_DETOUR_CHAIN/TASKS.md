# TASKS 077 — Detour-цепочки

## Фаза 1 — модель + проброс + применение ✅
- [x] `Source.DetourTag` (state/connections.go)
- [x] `ProxySource.DetourTag` (configtypes/types.go)
- [x] Проброс в `ToProxySourceV4` (adapter_source.go), `sync_to_legacy.go`, `sync_to_connections.go`
- [x] `LoadNodesFromSource`: `applySourceDetour` проставляет `node.Outbound["detour"]` (skip wireguard / Jump)
- [x] Тесты: round-trip Source↔ProxySource (detour_mapping_test.go); applySourceDetour + server+detour end-to-end (detour_test.go). Эмиссия `"detour"` в JSON — существующий механизм GenerateNodeJSON.

## Фаза 2 — валидация (fail-open) ✅
- [x] `sanitizeNodeDetours` в outbound_generator: self → drop+warn; цикл среди узлов (DFS-раскраска) → разорвать+warn. Висячий-на-шаблон НЕ дропается (теги шаблонных групп неизвестны до финальной сборки — уточнено в SPEC §3).
- [x] Тесты: self / valid-chain / external-target-kept / 2-cycle / 3-cycle (detour_sanitize_test.go). wg-skip и Jump-приоритет покрыты в Фазе 1 (applySourceDetour).

## Фаза 3 — UI ✅
- [x] Дропдаун «Detour server» в source_edit_window (server + subscription), фильтр собственных тегов, «(none)»
- [x] Хелпер `DetourOptions` в business (+ тесты); локали `wizard.source.label_detour`/`detour_none`/`detour_hint` (en + ru)
- [x] Round-trip: `applyProxyEditToSource` пробрасывает DetourTag (+ ToProxySourceV4 при открытии); тест detour_persist_test.go (subscription/server/cleared)

## Фаза 4 — докуменация
- [ ] `docs/ParserConfig.md` + `docs/release_notes/upcoming.md`
- [ ] golden config-фрагмент (опц. через sing-box check)
- [ ] `IMPLEMENTATION_REPORT.md`
- [ ] `go build ./... && go test ./... && go vet ./...`
