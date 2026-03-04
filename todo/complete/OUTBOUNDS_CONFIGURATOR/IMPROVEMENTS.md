# Задачи на улучшения: визард, дочерние окна, overlay

Заметки по коду после реализации конфигуратора outbounds, View, Edit и единого overlay. Не блокируют приёмку; можно делать по мере рефакторинга.

---

## 1. Именование под «дочерние окна», а не только Rule dialog

**Суть:** Overlay и фокус теперь общие для трёх типов окон (Rule, View, Outbound Edit), но имена остались от одного сценария.

| Где | Сейчас | Предложение |
|-----|--------|-------------|
| `presentation/gui_state.go` | `RuleDialogOverlay` | `ChildWindowsOverlay` (или `WizardChildOverlay`) |
| `core/services/ui_service.go` | `FocusOpenRuleDialogs` | `FocusOpenChildWindows` (или оставить имя для обратной совместимости и только обновить комментарий) |
| `ui/wizard/wizard.go` | присвоение `RuleDialogOverlay`, вызов `FocusOpenRuleDialogs` | те же имена или новые, согласованно с gui_state и UIService |
| `ui/components/click_redirect.go` | использование `FocusOpenRuleDialogs` | без изменений, если в UIService поле не переименовывать |
| Комментарии в `presentation/presenter.go` | «open rule dialogs», «rule-dialog overlay» | «child windows», «overlay for child windows» |

**Задача:** Переименовать поля/комментарии под общую модель «дочерние окна визарда» и обновить ссылки (в т.ч. в ARCHITECTURE.md).

---

## 2. Контракт регистрации дочерних окон

**Суть:** Сейчас контракт разбросан: Rule — через `openRuleDialogs` и прямой вызов `UpdateChildOverlay` из диалога; View/Edit — через методы презентера и опциональный `OutboundEditPresenter`. Нет одного места, где описано «как правильно открыть дочернее окно».

**Задача:** Оформить короткий контракт (в `docs/` или в коде, рядом с презентером):

- при открытии: зарегистрировать окно у презентера (или в общем реестре), вызвать `UpdateChildOverlay()`;
- при закрытии: снять регистрацию, вызвать `UpdateChildOverlay()`;
- для «одно экземпляр» (View, Edit): перед созданием проверить уже открытое и при наличии — только `RequestFocus()`;
- клик по визарду (overlay) переводит фокус на одно из зарегистрированных окон (логика в wizard.go).

Опционально: один интерфейс/хелпер «зарегистрировать дочернее окно» с callback при закрытии, чтобы не дублировать вызовы UpdateChildOverlay в каждом диалоге.

---

## 3. add_rule_dialog и единый overlay

**Сделано:** Вместо локального `updateRuleDialogOverlay()` везде вызывается `presenter.UpdateChildOverlay()`.

**Возможное улучшение:** При «only one rule dialog» при закрытии всех существующих диалогов в цикле после каждого `Close()` и `delete(openDialogs, key)` теоретически можно один раз вызвать `UpdateChildOverlay()` после цикла (сейчас overlay обновляется при открытии нового диалога и в SetCloseIntercept). Проверить сценарий «закрыли все rule dialogs» и убедиться, что overlay скрывается без задержки; при необходимости добавить один вызов после цикла.

---

## 4. Порядок фокуса при клике по overlay

**Суть:** В `wizard.go` порядок: View → Outbound Edit → любой Rule dialog. Это жёстко зашито.

**Задача (низкий приоритет):** Если понадобится «фокус на последнем активном дочернем окне», вести последнее сфокусированное окно и по клику на overlay отдавать фокус ему. Пока можно оставить текущий порядок и зафиксировать в контракте (см. п. 2).

---

## 5. Документация архитектуры

**Суть:** В `docs/ARCHITECTURE.md` упоминаются Rule dialog overlay и, возможно, только rule dialogs.

**Задача:** Обновить разделы про визард и overlay: один overlay для всех дочерних окон (Rule, View, Outbound Edit), фокус по клику, запрет двух окон View и двух Edit. После переименований (п. 1) подставить актуальные имена полей и callback.

---

## 6. OUTBOUNDS_CONFIGURATOR: описание в todo/README

**Суть:** В `todo/README.md` в «Текущие фичи» для OUTBOUNDS_CONFIGURATOR указано «кнопка Config Outbounds» и окно.

**Задача:** Обновить описание под текущую реализацию: встроенный конфигуратор во вкладке Outbounds, отдельные окна View и Edit/Add, единый overlay и один экземпляр View и Edit.

---

## Чеклист (для постановки в TASKS при рефакторинге)

- [x] Переименовать `RuleDialogOverlay` → `ChildWindowsOverlay` (или аналог) в gui_state, wizard, presenter; обновить комментарии.
- [x] По желанию переименовать `FocusOpenRuleDialogs` → `FocusOpenChildWindows` в UIService и wizard (и click_redirect при смене имени).
- [x] Описать контракт «дочерние окна визарда» (регистрация, overlay, фокус, один экземпляр) в docs или в коде.
- [x] Проверить скрытие overlay при закрытии всех rule dialogs; при необходимости добавить вызов UpdateChildOverlay после цикла.
- [x] Обновить ARCHITECTURE.md под общий overlay и три типа дочерних окон.
- [x] Обновить описание OUTBOUNDS_CONFIGURATOR в todo/README.md.
