# План: 039 — SETTINGS_TAB_PREFERENCES

## 1. Settings.go

- Новые поля `SubscriptionAutoUpdateDisabled`, `AutoPingAfterConnectDisabled` (оба `omitempty`).

## 2. StateService

- `AutoPingAfterConnect` + `AutoPingAfterConnectMutex` + `Is/Set`-геттеры.
- `AutoUpdateEnabled` не трогаем — уже есть.

## 3. UIService

- `AutoPingAfterConnectFunc func()` — новый hook, регистрируется в `clash_api_tab.go`.

## 4. RunningState

- Поле `autoPingTimer *time.Timer`.
- На false→true: stop existing, arm new `time.AfterFunc(5s)`.
- На true→false: stop timer.
- Внутри timer callback — re-check `IsRunning()` (race window 5s).

## 5. UI

- `ui/settings_tab.go`: новый файл с `CreateSettingsTab(ac)`.
- `ui/app.go`: добавить `container.NewTabItem(locale.T("app.tab.settings"), CreateSettingsTab(controller))` между Servers и Diagnostics.
- `ui/help_tab.go`: убрать language-селектор + Download-locales из return'а + imports.
- `ui/core_dashboard_tab.go`: убрать autoUpdateCheck + autoPingCheck из `createConfigBlock` + their `autoUpdateRow`.

## 6. main.go

- После `LoadSettings`: применить `SubscriptionAutoUpdateDisabled` / `AutoPingAfterConnectDisabled` к StateService.

## 7. Локализация

- 5 новых ключей в `en.json` + `ru.json`.

## 8. Тесты

- TODO: unit на `Is/Set`-геттеры AutoPing (уже есть из `0c15fd8`).
- TODO: integration smoke Settings-таб (нет harness'а для Fyne UI).
