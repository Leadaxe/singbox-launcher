# Задачи: 039 — SETTINGS_TAB_PREFERENCES

## Этап 1 — Core / state

- [x] `StateService.AutoPingAfterConnect` + mutex + Is/Set getters (`500322c` / `f04b0e9`).
- [x] `StateService.SubscriptionAutoUpdateEnabled` — уже существовал, просто wire в UI.
- [x] `UIService.AutoPingAfterConnectFunc` hook.
- [x] `RunningState.autoPingTimer` + Set-логика.

## Этап 2 — Persist

- [x] `Settings.SubscriptionAutoUpdateDisabled` + `Settings.AutoPingAfterConnectDisabled` (обе `omitempty`).
- [x] main.go — apply на старте.

## Этап 3 — UI

- [x] `ui/settings_tab.go` — `CreateSettingsTab` с двумя секциями.
- [x] `ui/app.go` — регистрация таба между Servers и Diagnostics.
- [x] `ui/help_tab.go` — убрать language-блок (`e7c46f9`).
- [x] `ui/core_dashboard_tab.go` — убрать galki (`e7c46f9`).

## Этап 4 — Bugfix

- [x] Data-loss баг в language-handler'е починен: load-mutate-save вместо `Settings{Lang: code}` (`e7c46f9`).

## Этап 5 — Локализация

- [x] 5 новых ключей (en + ru).

## Этап 6 — Тесты

- [x] Unit на StateService flags (`0c15fd8`).
- [ ] **TODO:** integration-тест UI (нет Fyne-harness пока).

## Этап 7 — Документация

- [ ] **TODO:** `docs/release_notes/upcoming.md` — корректная запись в «Added — Settings tab».
- [ ] **TODO:** README — упомянуть что language моргнуло из Help в Settings.
