# Реализация: 042 — KEYBOARD_SHORTCUTS

**Коммиты:** `b398399` (Cmd/Ctrl+R / +U), `264ffaf` (Cmd/Ctrl+P).
Ветка `night-work`, 2026-04-22.
**Спека написана ретроспективно.**

## Что сделано

`ui/app.go` — функция `(*App).registerShortcuts()`:

```go
func (a *App) registerShortcuts() {
    if a.window == nil || a.window.Canvas() == nil { return }

    reconnect := &desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierShortcutDefault}
    a.window.Canvas().AddShortcut(reconnect, func(fyne.Shortcut) {
        core.KillSingBoxForRestart()
    })
    updateSubs := &desktop.CustomShortcut{KeyName: fyne.KeyU, Modifier: fyne.KeyModifierShortcutDefault}
    a.window.Canvas().AddShortcut(updateSubs, func(fyne.Shortcut) {
        core.RunParserProcess()
    })
    pingAll := &desktop.CustomShortcut{KeyName: fyne.KeyP, Modifier: fyne.KeyModifierShortcutDefault}
    a.window.Canvas().AddShortcut(pingAll, func(fyne.Shortcut) {
        if a.core != nil && a.core.UIService != nil &&
           a.core.UIService.AutoPingAfterConnectFunc != nil {
            a.core.UIService.AutoPingAfterConnectFunc()
        }
    })
}
```

Вызов: после `InitWizardOverlay(app, controller)` в `NewApp`.

## Что не сделано — критический TODO

**Tooltip hints** на соответствующих кнопках — без них шорткаты fe-discoverable. См. SPEC §4 и TASKS.md этап 2.

## Проверка

- `go build ./...` — успешно.
- Ручной тест на macOS: `Cmd+R` → sing-box перезапустился; `Cmd+U` → парсер запустился; `Cmd+P` → ping-all.
- Entry в визарде проглатывает keystroke — проверено, корректно.
