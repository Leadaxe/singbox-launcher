# Задачи: 037 — SUBSCRIPTION_SOURCE_TOGGLE

## Этап 0 — подготовка

- [x] Утвердить модель (`Disabled bool` + `omitempty`).

## Этап 1 — модель и парсер

- [x] `ProxySource.Disabled bool json:"disabled,omitempty"` в `configtypes/types.go`.
- [x] `GenerateOutboundsFromParserConfig`: подсчёт только enabled для `TotalSources`.
- [x] Пропуск disabled-источников до `loadNodesFunc` с `debuglog.DebugLog` строкой.
- [x] Early-return «no enabled sources …» если `totalSources == 0`.

## Этап 2 — UI

- [x] Чекбокс слева в строке `refreshSourcesList` в `ui/wizard/tabs/source_tab.go`.
- [x] OnChanged: мутация модели → `SerializeParserConfig` → `PreviewNeedsParse=true` → `InvalidatePreviewCache` → `presenter.UpdateParserConfig` → refresh.
- [x] LowImportance на label'ах (sourceLabel + prefixLabel) для disabled-строк.

## Этап 3 — тесты

- [ ] **TODO:** unit на `GenerateOutboundsFromParserConfig` (3 сценария).
- [ ] **TODO:** unit на сериализацию `ProxySource` с/без `Disabled`.
- [ ] **TODO:** smoke-тест wizard UI (если появится тестовый harness для Fyne).

## Этап 4 — документация

- [x] Запись в `docs/release_notes/upcoming.md` (EN + RU).
- [ ] **TODO:** upd `docs/ARCHITECTURE.md` при следующем sync.
