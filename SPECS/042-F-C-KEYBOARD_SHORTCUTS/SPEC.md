# SPEC: Горячие клавиши главного окна

Задача: дать power-пользователям `Cmd/Ctrl+R` / `+U` / `+P` для самых частых действий (Reconnect sing-box / Update subscriptions / Ping-all), доступных с любой вкладки.

**Статус:** реализовано. Коммиты:
- `b398399` — `Cmd/Ctrl+R` (Reconnect) + `Cmd/Ctrl+U` (Update subs).
- `264ffaf` — `Cmd/Ctrl+P` (Ping-all, едет вместе с debug-API ping-all endpoint).

**Спека написана ретроспективно. Есть TODO на tooltip-подсказки (см. §4).**

---

## 1. Проблема

### 1.1 До изменений

- Для Reconnect / Update / Ping-all пользователь обязан:
  - Переключиться на нужную вкладку (Core Dashboard / Servers).
  - Найти конкретную кнопку.
  - Нажать мышью.
- Даже простые случаи («обновил подписки 3 раза подряд отлаживая template») требуют цикла мышью.
- Power-users, привыкшие к Electron-клиентам (`Cmd+R` везде = reload), просят «хоткеи».

### 1.2 Цель

Три шортката, работают на главном окне с любой вкладки (если нет активного Entry-фокуса):

- `Cmd/Ctrl+R` — `core.KillSingBoxForRestart()` (Reconnect).
- `Cmd/Ctrl+U` — `core.RunParserProcess()` (Update subscriptions).
- `Cmd/Ctrl+P` — `UIService.AutoPingAfterConnectFunc()` (Ping-all, если hook зарегистрирован).

---

## 2. Требования

### 2.1 Платформенная совместимость

- Использовать `fyne.KeyModifierShortcutDefault` — Fyne сам маппит:
  - macOS → `KeyModifierSuper` (Cmd).
  - Windows / Linux → `KeyModifierControl`.
- Не изобретать свою helper-функцию.

### 2.2 Регистрация

- В `ui/app.go`, после `InitWizardOverlay`:
  ```go
  app.registerShortcuts()
  ```
- Регистрация на `app.window.Canvas()` — единая поверхность на всё главное окно.
- Обработчики — прямые вызовы core-функций; не публикуем события (event-bus редизайн — отдельная задача).

### 2.3 Что НЕ делает

- Шорткаты не переопределяют фокусированные Entry / multiline. Если пользователь печатает `user:password` в Wizard — `Cmd+U` уйдёт в Entry (стандартное Select all / whatever), а не в `RunParserProcess`. Это корректное поведение Fyne из коробки — `AddShortcut` уступает focus-chain'у.
- Не работают в окне визарда, только в главном окне. Визард — отдельное `fyne.Window`, у него свой canvas.
- Не работают когда главное окно скрыто в трей. Тоже ожидаемо — нет фокуса, некуда диспатчить keystroke.

### 2.4 Tooltip hints (TODO)

**Критический gap**: шорткаты работают, но **пользователь о них не знает** без чтения release-notes.

Нужно добавить tooltip на соответствующие кнопки:

- `updateConfigButton` (Update) — сейчас `widget.NewButton`, без tooltip. Сменить на `ttwidget.NewButton` (пакет `github.com/dweymouth/fyne-tooltip/widget`, уже используется в других местах) и `SetToolTip("Update subscriptions (Cmd+U)" / "... (Ctrl+U)")`.
- `restartButton` (круговая стрелка) — аналогично, `"Restart sing-box (Cmd+R)" / "... (Ctrl+R)"`.
- Servers tab «test» кнопка (это `ttwidget.NewButton` — хорошо) — `SetToolTip("Test all (Cmd+P)" / "... (Ctrl+P)")`.

**Helper для platform-specific текста:**

```go
func shortcutLabel(key string) string {
    if runtime.GOOS == "darwin" {
        return "⌘" + key
    }
    return "Ctrl+" + key
}
// usage: SetToolTip(locale.Tf("core.tooltip_update", shortcutLabel("U")))
```

Локализационные ключи (новые):

- `core.tooltip_update` → `"Update subscriptions (%s)"` / `"Обновить подписки (%s)"`
- `core.tooltip_restart` → `"Restart sing-box (%s)"` / `"Перезапустить sing-box (%s)"`
- `servers.tooltip_ping_all` уже существует, расширить `%s` для шортката.

---

## 3. Инварианты

1. **Не интерферировать с text-input фокусом.** Фокусированный Entry проглатывает keystroke — это by design.
2. **Шорткат == действие существующей кнопки.** Не дублировать логику — только reuse of the button's OnTapped / hook.
3. **ShortcutDefault** как modifier — не хардкодить Ctrl или Super.

---

## 4. Критический TODO

Tooltip-подсказки обязательны — без них фичу никто не найдёт.

- [ ] `updateConfigButton` → `ttwidget.NewButton`, tooltip `"Update subscriptions (⌘U / Ctrl+U)"`.
- [ ] `restartButton` → tooltip `"Restart sing-box (⌘R / Ctrl+R)"`.
- [ ] Ping-all button tooltip расширить с упоминанием шортката.
- [ ] `shortcutLabel()` helper в `internal/platform/` или `ui/`.
- [ ] Локализационные ключи для tooltip-форматов.

---

## 5. Не-цели

- Не делаем customizable keybindings (редактируемые пользователем).
- Не добавляем глобальные shortcuts (работающие когда приложение в фоне) — это нужен system-hotkey API на каждой платформе, отдельная работа.
- Не добавляем help-dialog со списком всех шортков (пока их 3, tooltip'ов хватит).

---

## 6. Открытые вопросы

- `Cmd/Ctrl+,` (comma) → открыть Settings tab? Mac-конвенция для prefs. Можно добавить.
- `Cmd/Ctrl+Q` → Quit? macOS уже перехватывает. На Linux/Windows — можно.
- Навигация между табами `Cmd+1..5`? Fyne AppTabs поддерживает.
- Всё это — **следующая итерация**, не текущий SPEC.
