# 028 — Подсветка строк списка при наведении (визард)

| Поле | Значение |
|------|----------|
| **Статус** | **Complete (C)** — оформлено как закрытая задача одним документом |
| **Тип** | F (feature, UX) |
| **Дата закрытия** | 2026-03-25 |

---

## 1. Цель

Списочные строки в визарде должны визуально реагировать на наведение курсора **на всю строку**, а не только на «пустые» области: лёгкая подсветка фона, согласованная с темой, без артефактов layout при уходе курсора с интерактивных дочерних виджетов.

**Охват UI:** вкладки **Rules**, **Sources** (список подписок), **Outbounds** (список outbound’ов в конфигураторе под полем ParserConfig), **DNS** (список серверов); модал **Add from library** (библиотека пресетов правил).

---

## 2. Проблемы, которые решались

1. **Лейблы с тултипами** (`github.com/dweymouth/fyne-tooltip`) обрабатывают hover сами — события не доходят до родительского контейнера; без проброса подсветка не работала на зоне текста.
2. **Кнопки, Select, кнопка SRS** перехватывают hover; строка не знала, что курсор всё ещё «над строкой».
3. **SRS** требует `ttwidget.Button` (тултип с URL). Наивная обёртка с отдельным внутренним `*widget.Button` и/или делегированием `CreateRenderer` приводила к **смещению иконки/текста** после `MouseOut` и к потере согласованности `BaseWidget` с листом дерева.
4. **Цвет подсветки**: смесь с `ColorNameHover` давала **серый** оттенок; требовалась **слегка голубоватая** подсветка за счёт темы.

---

## 3. Решение (схема)

- **`HoverRow`** (`internal/fynewidget/hover_row.go`) — обёртка над контентом строки: под контентом `canvas.Rectangle`, реализация `desktop.Hoverable`, опционально фон выбранной строки через `HoverRowConfig`.
- **`WireTooltipLabelHover`** — цепочка `OnMouseIn` / `OnMouseMoved` / `OnMouseOut` у `ttwidget.Label` к методам `HoverRow` (с сохранением предыдущих колбэков).
- **`HoverForwardButton`**, **`HoverForwardSelect`**, **`HoverForwardTTButton`** (`internal/fynewidget/hover_forward.go`) — встраивание `widget.Button` / `widget.Select` / `ttwidget.Button` **по значению**, один вызов `ExtendBaseWidget` на внешний тип; в обработчиках мыши — вызов базового поведения и **`forwardRowHover`**: по `RowHoverGetter` вызывается тот же `MouseIn`/`MouseOut`/… у **`HoverRow`**.
- **Порядок создания**: `var row *HoverRow`; `rowGetter := func() *HoverRow { return row }`; сборка дочерних виджетов с `rowGetter`; затем `row = NewHoverRow(...)`.
- **`HoverForwardTTButton.TTWidget()`** — доступ к `*ttwidget.Button` для кода, который ждёт именно этот тип (async `Disable`/`SetText`, поля в `GUIState`); адрес совпадает с первым полем встроенного значения (документировано в коде и в `internal/fynewidget/doc.go`).
- **Цвет hover**: смесь **`ColorNameBackground`** и **`ColorNamePrimary`** (доли константами), без опоры на «серый» `ColorNameHover` как основной вклад.

Документация для повторного использования в проекте: **`internal/fynewidget/doc.go`** (package comment).

---

## 4. Задействованные файлы (ориентир)

| Область | Путь |
|---------|------|
| Виджеты hover | `internal/fynewidget/hover_row.go`, `internal/fynewidget/hover_forward.go`, `internal/fynewidget/doc.go` |
| Rules | `ui/wizard/tabs/rules_tab.go` |
| Sources (список источников) | `ui/wizard/tabs/source_tab.go` — `HoverRow`, подписи/префикс через `ttwidget.Label` + `WireTooltipLabelHover`, кнопки Copy/Edit/Del через `HoverForward*` |
| Outbounds (список в конфигураторе) | `ui/wizard/outbounds_configurator/configurator.go` — то же для строк с ↑/↓/Edit/Delete |
| DNS (список серверов) | `ui/wizard/tabs/dns_tab.go` — `HoverRow`, `WireTooltipLabelHover` для summary-лейбла (после `CheckWithContent`), Edit/Del через `HoverForward*` |
| Библиотека правил | `ui/wizard/tabs/library_rules_dialog.go` |
| Состояние GUI | `ui/wizard/presentation/gui_state.go` (комментарий к `SRSButton` и `*ttwidget.Button` через `TTWidget`) |
| Документация репозитория | `docs/ARCHITECTURE.md` (узел `internal/fynewidget`, абзац про списки визарда в разделе Wizard), `docs/release_notes/upcoming.md`, `RELEASE_NOTES.md` |

---

## 5. Критерии приёмки (выполнено)

- [x] При наведении на строку подсвечивается фон **по всей ширине** строки, в том числе над подписью с тултипом.
- [x] При наведении на кнопки действий, outbound **Select** и кнопку **SRS** (Rules) подсветка строки сохраняется; при уходе курсора — корректно снимается.
- [x] То же для списков **Sources**, **Outbounds** (конфигуратор) и **DNS**: кнопки в строке через `HoverForward*`, подписи с тултипом — `ttwidget.Label` + `WireTooltipLabelHover` (DNS: цепочка с `CheckWithContent` сохраняется).
- [x] Кнопка SRS не «прыгает» по layout при hover/out; тултип URL сохраняется.
- [x] Подсветка визуально **не серая**, а слегка **с холодным оттенком** (через `ColorNamePrimary`).
- [x] Паттерн задокументирован для повторного использования (`doc.go`).
- [x] Обновлены `docs/ARCHITECTURE.md`, черновик `docs/release_notes/upcoming.md` и `RELEASE_NOTES.md`.

---

## 6. Закрытие

Задача считается **полностью реализованной**; отдельные `PLAN.md` / `TASKS.md` / `IMPLEMENTATION_REPORT.md` для этой записи **не ведутся** — вся фиксация в данном файле.
