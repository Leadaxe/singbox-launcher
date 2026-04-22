# План: 043 — DIRTY_CONFIG_MARKER

## 1. Минимальная версия (done)

- `StateService.TemplateDirty` + mutex + Is/Set геттеры.
- `presenter_save.go saveStateAndShowSuccessDialog` — ставит true.
- `RunParserProcess` успех — ставит false + `UpdateConfigStatusFunc`.
- `updateConfigInfo` — `*` префикс + HighImportance на Update button.

## 2. TODO — минимальное улучшение (до большого редизайна)

- Разделить на два сигнала:
  - `SetSourcesDirty()` — wizard menalli Proxies.
  - `SetRuntimeDirty()` — wizard menalli vars / rules / DNS.
- `*` на Update — только sources-dirty.
- Подсветка Restart-button — runtime-dirty.
- `presenter_save.go` — при Save перед `SetSourcesDirty` делать diff проксей с последним известным состоянием; то же для vars / rules.

## 3. TODO — большой редизайн (state ↔ config separation)

- Новый declarative `state.json` формат.
- Wizard Save → только state.json (не config.json).
- `ConfigService.BuildFromState(state, cache) → config.json` — отдельный шаг.
- Startup: если state новее config.json → auto-build.
- Event-bus `StateChanged{kind}` — подписчики рендерят правильный маркер.
- Миграция прежних state'ов.

## 4. Тесты

- TODO: unit на `SetSourcesDirty` / `SetRuntimeDirty` round-trip (после разделения).
- TODO: integration wizard → маркер установлен → Update → сброшен.
