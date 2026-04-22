# Реализация: 043 — DIRTY_CONFIG_MARKER

**Коммит:** `92697c7` (ветка `night-work`, 2026-04-22).
**Спека написана ретроспективно.**

## Что сделано (минимальная версия)

- `core/services/state_service.go`:
  - `TemplateDirty bool` + `TemplateDirtyMutex sync.RWMutex`.
  - `IsTemplateDirty()` / `SetTemplateDirty(bool)` — mutex-guarded.
- `ui/wizard/presentation/presenter_save.go saveStateAndShowSuccessDialog`:
  ```go
  if ac.StateService != nil {
      ac.StateService.SetTemplateDirty(true)
  }
  ```
- `core/config_service.go RunParserProcess` success branch:
  ```go
  if ac.StateService != nil {
      ac.StateService.SetTemplateDirty(false)
      ac.StateService.RecordUpdateSuccess()
  }
  if ac.UIService != nil && ac.UIService.UpdateConfigStatusFunc != nil {
      ac.UIService.UpdateConfigStatusFunc()
  }
  ```
- `ui/core_dashboard_tab.go updateConfigInfo` — `*` + HighImportance.

## Что не сделано / TODO

### Критично — минимальное улучшение

Текущий маркер семантически размазан: `*` на Update загорается на любом wizard Save, включая правки tun/dns/rules которые уже в config.json и требуют не парсера а Restart'а.

См. SPEC §3.4:
- Разделить на `SourcesDirty` и `RuntimeDirty` с разными UI-сигналами.
- Presenter_save делает diff и вызывает соответствующий setter.

### Большой редизайн (следующий релиз)

Правильная модель — из LxBox мобильного: отдельно декларативный `state.json`, отдельно generated `config.json`, явный шаг Build Config. См. SPEC §3.2 и PLAN.md §3.

## Проверка

- Unit-тесты на `SetTemplateDirty` / `IsTemplateDirty` — частично в `0c15fd8`.
- Ручной тест: wizard → любая правка → Save → Update button `* Update` синий. Update → вернулся в `Update` серый. OK.
- Демо бага семантики: wizard → сменить `log_level` → Save → `* Update` горит. Нажать Update — парсер прошёл, маркер погас, но `log_level` в работающем sing-box всё ещё старый (нужен Restart). Продемонстрирован design-gap.
