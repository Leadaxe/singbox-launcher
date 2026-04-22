# Реализация: 039 — SETTINGS_TAB_PREFERENCES

**Коммиты:** `500322c` (auto-update toggle), `f04b0e9` (auto-ping), `e7c46f9` (Settings tab + help reorg). Ветка `night-work`, 2026-04-22 (ночь + утро).
**Спека написана ретроспективно.**

## Что сделано

### Core / state
- `StateService.AutoPingAfterConnect` + `AutoPingAfterConnectMutex` + `Is/Set`-методы.
- `UIService.AutoPingAfterConnectFunc` hook.
- `RunningState.autoPingTimer` — `time.Timer`, arm на false→true, stop на true→false, re-check `IsRunning()` в timer callback.

### Persist
- Два новых поля в `internal/locale/settings.go`:
  - `SubscriptionAutoUpdateDisabled bool json:"subscription_auto_update_disabled,omitempty"`
  - `AutoPingAfterConnectDisabled bool json:"auto_ping_after_connect_disabled,omitempty"`
- `main.go` — apply на старте через setter'ы.

### UI
- `ui/settings_tab.go` — новый файл, `CreateSettingsTab(ac)` с двумя секциями:
  - Subscriptions: два чекбокса.
  - Language: селектор + Download-locales кнопка.
- `ui/app.go` — таб `⚙️ Settings` между Servers и Diagnostics.
- Language-блок убран из `ui/help_tab.go`.
- Galki auto-update/auto-ping убраны из `ui/core_dashboard_tab.go createConfigBlock`.

### Bugfix
- В старом `help_tab.go` было `locale.SaveSettings(binDir, locale.Settings{Lang: code})` — затирало ВСЕ остальные поля. В `settings_tab.go` — load-mutate-save.

## Что не сделано (TODO)

- Integration-тест UI (нет Fyne UI-harness'а).
- `docs/release_notes/upcoming.md` — актуальная запись.
- README / Help — ссылки на новый расположение language-селектора.

## Связанные TODO из review

- **`92697c7` dirty-marker редизайн**: сейчас чекбоксы Settings-таба на `OnChanged` не триггерят dirty-state. Это правильно — они не меняют template. При редизайне по модели «separate state vs config-build» семантика сохраняется.
- **`f28d8db` event-bus редизайн**: `OnChanged`-handler сейчас напрямую зовёт `SaveSettings`. После миграции на event-bus — может публиковать `SettingsChanged{field, value}` событие, а persistence-layer подписывается. Сейчас — синхронно и прямолинейно, этого хватает.
