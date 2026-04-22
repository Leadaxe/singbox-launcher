# План: 042 — KEYBOARD_SHORTCUTS

## 1. Регистрация (done)

- `ui/app.go` — метод `(*App).registerShortcuts()`.
- Вызывается после `InitWizardOverlay` в `NewApp`.
- Использует `window.Canvas().AddShortcut(&desktop.CustomShortcut{...}, handler)`.
- Modifier — `fyne.KeyModifierShortcutDefault` (платформо-агностичный).

## 2. Маппинг шортков → функций

| Шорткат | Key       | Функция                                 |
|---------|-----------|-----------------------------------------|
| Reconnect | R       | `core.KillSingBoxForRestart()`          |
| Update    | U       | `core.RunParserProcess()`                |
| Ping-all  | P       | `UIService.AutoPingAfterConnectFunc()`  |

## 3. Tooltip подсказки (TODO)

- Сменить `widget.NewButton` → `ttwidget.NewButton` для Update + Restart.
- `shortcutLabel(key)` helper с `runtime.GOOS` разветвлением.
- 2 новых locale-ключа `core.tooltip_update` / `core.tooltip_restart`.
- Расширить существующий `servers.tooltip_ping_all`.

## 4. Тесты (TODO)

- Unit на handler'ы — вызов правильной core-функции.
- Integration-тест (если когда-либо появится Fyne UI harness).
