# Задачи: 042 — KEYBOARD_SHORTCUTS

## Этап 1 — регистрация (done)

- [x] `ui/app.go` — `registerShortcuts()` на canvas главного окна.
- [x] `Cmd/Ctrl+R` → Reconnect.
- [x] `Cmd/Ctrl+U` → Update subs.
- [x] `Cmd/Ctrl+P` → Ping-all (через hook).

## Этап 2 — Tooltip hints (**critical TODO**)

- [ ] `updateConfigButton` → `ttwidget.NewButton`.
- [ ] `restartButton` → то же.
- [ ] `shortcutLabel(key)` helper (platform-aware).
- [ ] Locale keys: `core.tooltip_update`, `core.tooltip_restart`, расширить `servers.tooltip_ping_all`.

## Этап 3 — Документация

- [ ] README / release-notes: упомянуть шорткаты.
- [ ] docs/ARCHITECTURE.md: короткая секция «Keyboard shortcuts».

## Этап 4 — расширения (следующая итерация)

- [ ] `Cmd/Ctrl+,` → Settings-таб (Mac-конвенция).
- [ ] `Cmd/Ctrl+1..5` → навигация между табами.
- [ ] Help-dialog со списком шортков — когда их станет больше 5.
