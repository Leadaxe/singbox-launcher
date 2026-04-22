# Задачи: 043 — DIRTY_CONFIG_MARKER

## Этап 1 — минимум (done)

- [x] `StateService.TemplateDirty` + mutex + Is/Set.
- [x] Wizard Save ставит true.
- [x] RunParserProcess на успехе ставит false.
- [x] `*` префикс + HighImportance на кнопке Update.

## Этап 2 — минимальное улучшение (TODO)

- [ ] Разделить `SetTemplateDirty` на `SetSourcesDirty` + `SetRuntimeDirty`.
- [ ] `presenter_save.go` — diff перед Save, вызов соответствующего setter'а.
- [ ] `updateConfigInfo` — `*` на Update только для sources-dirty.
- [ ] Подсветка Restart-button (или status-chip) для runtime-dirty.
- [ ] Unit-тесты на новые setter'ы.

## Этап 3 — большой редизайн (TODO, следующий релиз)

- [ ] Declarative `state.json` формат.
- [ ] Wizard Save → только state.json.
- [ ] `ConfigService.BuildFromState(state, cache) → config.json`.
- [ ] Startup: auto-build если state новее.
- [ ] Event-bus `StateChanged{kind}`.
- [ ] Миграция старых state'ов.
- [ ] Тесты на build / miss-cache / стартап flow.

## Этап 4 — документация

- [ ] Обновить `docs/ARCHITECTURE.md` после большого редизайна.
- [ ] Описать миграцию старых state'ов в release-notes.
