# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights

- **Main window stays usable while the configurator is open.** Previously a transparent click-redirect overlay on the main window forwarded every click to the wizard, which made Update / Restart / Start / Stop / tab switches silently fail until you closed the configurator. The overlay is now disabled by default — the configurator opens as a sibling top-level window and the launcher controls keep working in parallel.
- **DNS rules tab — unified ordered list (drag ↑↓).** The wizard's DNS Rules section used to render preset DNS rules and user DNS rules in two fixed sections — preset rules always won sing-box first-match. The tab now shows a single ordered list where preset (🔗) and user (→) rules can be interleaved freely; use ↑↓ buttons on each row to reorder, and your custom domain can finally beat a preset's broader match. A small toggle at the top-right of the section swaps between this list and the raw-JSON editor (user rules only).

### Technical / Internal

- `ui/wizard_overlay.go::wizardOverlayEnabled` (const `bool`) gates the legacy main-window click-redirect overlay. Default `false`. Flip to `true` to restore the «wizard owns the foreground» UX without ripping out the implementation. The wizard's *internal* `ChildWindowsOverlay` (used over wizard tabs while a child dialog is open) is independent and unchanged.

## RU
### Основное

- **Главное окно работает пока открыт конфигуратор.** Раньше невидимый overlay перехватывал любой клик по главному окну пока визард открыт — Update / Restart / Start / Stop / переключение вкладок молча игнорились. Теперь overlay по умолчанию выключен, конфигуратор — обычное соседнее окно, лаунчер можно использовать параллельно.
- **Вкладка DNS — единый упорядоченный список правил (drag ↑↓).** Секция DNS rules в визарде раньше рисовала preset- и user-правила в двух фиксированных секциях — preset всегда выигрывал first-match у sing-box. Теперь это один ordered список, в котором preset (🔗) и user (→) правила можно перемешать произвольно: кнопки ↑↓ на каждой строке двигают порядок, и твой кастомный домен наконец может опередить более широкий match preset'а. Маленький toggle справа от заголовка переключает между этим списком и raw-JSON редактором (только user-правила).

### Техническое / Внутреннее

- `ui/wizard_overlay.go::wizardOverlayEnabled` (const `bool`) — фича-флаг legacy-overlay'я главного окна. Default `false`. Поставь `true` — вернётся прежнее поведение «wizard захватывает фокус». Внутренний wizard'овый `ChildWindowsOverlay` (над wizard-табами когда открыто child-окно) — независим и не тронут.
