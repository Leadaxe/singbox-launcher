# План: 037 — Subscription source toggle

**Статус:** реализовано. Детали — IMPLEMENTATION_REPORT.md.

## 1. Модель

- `ProxySource.Disabled bool json:"disabled,omitempty"` в `core/config/configtypes/types.go`.
- Никаких новых констант / enum'ов. Boolean достаточно.

## 2. Парсер

- `GenerateOutboundsFromParserConfig` (`core/config/outbound_generator.go`):
  - Pre-scan `parserConfig.ParserConfig.Proxies` — подсчёт только enabled для `totalSources`.
  - Цикл по всем proxy, `if proxySource.Disabled { debuglog.DebugLog(…); continue }` до `loadNodesFunc`.
  - Progress callback выводит `Processing source N/M` где M = enabled count, N увеличивается на каждом non-disabled.
  - Новый early-return: `if totalSources == 0 { return nil, fmt.Errorf("no enabled sources …") }` — до попытки считать nodes.
- Сигнатура `loadNodesFunc` не трогается — вызывающий уже не дойдёт до неё для disabled.

## 3. UI (Sources tab)

- `refreshSourcesList` → внутри IIFE:
  - После создания `sourceLabel` + `prefixLabel` создаётся `enableCheck := widget.NewCheck("", nil)`.
  - `SetChecked(!proxyPtr.Disabled)`.
  - Если `proxyPtr.Disabled` — `sourceLabel.Importance = widget.LowImportance`; то же для `prefixLabel` если есть.
  - `enableCheck.OnChanged`:
    1. Проверка `m.ParserConfig != nil && sourceIndex < len(m.ParserConfig.ParserConfig.Proxies)`.
    2. Мутация поля `Disabled`.
    3. `SerializeParserConfig` → `ParserConfigJSON`.
    4. `PreviewNeedsParse = true`, `InvalidatePreviewCache`.
    5. `presenter.UpdateParserConfig` + `guiState.RefreshSourcesList()`.
- Layout: `rowInner := container.NewBorder(nil, nil, enableCheck, rightControls, rowCenter)` — чекбокс заменил nil в левой позиции.

## 4. Тесты

- Unit-тест на `GenerateOutboundsFromParserConfig` с миксом enabled / disabled:
  1. 3 источника, все enabled → как было.
  2. 3 источника, 1 disabled → TotalSources=2, Succeeded/Failed по enabled; disabled не в `nodesBySource`.
  3. 3 источника, все disabled → error «no enabled sources …».
- Unit-тест на (де)сериализацию `ProxySource` с `Disabled: true` → присутствует в JSON; `Disabled: false` → отсутствует (omitempty).

## 5. Документация

- `docs/release_notes/upcoming.md` — запись в EN + RU.
- `docs/ARCHITECTURE.md` — в следующем sync-проходе обновить описание wizard Sources tab.
