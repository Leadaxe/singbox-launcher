## TASKS: OUTBOUNDS_CONFIGURATOR

**Реализация:** см. [IMPLEMENTATION_REPORT.md](IMPLEMENTATION_REPORT.md).

### Этап 1. Перестройка вкладок визарда

- [x] В `wizard.go` выделены два таба: **Sources** и **Outbounds** (название в UI — "Outbounds").
- [x] Вкладка Sources: поле SourceURLEntry, кнопка Add (AppendURLsToParserConfig, без автоприменения), подсказка, список Sources, Preview с Refresh. CheckURL и статус URL удалены.
- [x] С вкладки Sources убраны ParserConfigJSON, ParseButton, Config Outbounds, Documentation/ChatGPT (последние перенесены на Outbounds; ChatGPT на Outbounds нет).
- [x] Вторая вкладка Outbounds: редактор ParserConfigJSON, кнопка Documentation, встроенный конфигуратор outbounds.

### Этап 2. Вкладка Sources: Sources‑лист и Preview

- [x] Список Sources по `ParserConfig.ParserConfig.Proxies`:
  - один элемент на каждый `ProxySource`;
  - короткий label (обрезанный URL или `Source N`).
- [x] Интегрировать tooltip’ы для элементов списка через `fyne-tooltip`:
  - полный URL источника;
  - `tag_prefix`, `tag_postfix`, `tag_mask`;
  - количество и список тегов локальных outbounds этого источника.
- [x] Разместить Preview внизу вкладки Sources (Refresh + список серверов по всем источникам).
- [x] Убедиться, что список Sources и tooltip’ы автоматически обновляются при изменении ParserConfig (после `ApplyURLToParserConfig`, после правок в outbounds и ParserConfigJSON).

### Этап 3. Вкладка Outbounds: список outbounds

- [x] `NewConfiguratorContent(parent, parserConfig, onApply)` возвращает контент для вкладки; используется в CreateOutboundsAndParserConfigTab.
- [x] Порядок: локальные по каждому ProxySource, затем глобальные (`collectRows`).
- [x] Строка: кнопки ↑/↓ (ASCII), текст tag (type) — SourceLabel (обрезание 56 символов), Edit (иконка), Del (иконка), справа отступ 30px под скролл.
- [x] Up/Down только внутри scope (`moveOutboundUp`/`moveOutboundDown`), после операции refreshList + onApply.
- [x] Кнопка Add открывает окно создания outbound (ShowEditDialog).

### Этап 4. Диалог Edit/Add Outbound

- [x] ShowEditDialog открывается как **отдельное окно** (app.NewWindow), не модальный диалог; вызывается из конфигуратора на вкладке Outbounds.
- [x] Поля: Scope (For all / For source: label), Tag, Type (manual/auto), Comment, Filters (ключ tag), Preferred default (tag), AddOutbounds (direct-out, reject, теги выше).
- [x] Диалог со скроллом и отступом под полосу прокрутки.

### Этап 5. Синхронизация ParserConfig

- [x] На Sources применение URL только по Add через `AppendURLsToParserConfig` (добавление к существующим, дедупликация); после применения — сериализация, UpdateParserConfig, RefreshSourcesList.
- [x] Операции в конфигураторе (Edit/Add/Del/Up/Down): изменение model.ParserConfig, в onApply — SerializeParserConfig, обновление model.ParserConfigJSON, UpdateParserConfig, RefreshOutboundOptions, RefreshSourcesList.
- [x] Ручное редактирование JSON: при уходе с вкладки Outbounds вызывается `ValidateAndApplyParserConfigFromEntry` (парсинг, при успехе — обновление модели и LastValidParserConfigJSON; при ошибке — ShowError и откат к LastValidParserConfigJSON).

### Этап 6. Интеграция с Rules/Preview и финальная проверка

- [x] Parse/Preview по существующим механизмам (TriggerParseForPreview и т.д.); кнопки Parse на вкладках нет.
- [x] Rules/Preview используют актуальный ParserConfig.
- [x] Отдельное окно Config Outbounds убрано; конфигуратор только во вкладке Outbounds.
- [x] Критерии приёмки отражены в SPEC (раздел «Критерии приёмки (реализовано)»).

---

### Подзадача (опционально): сохранение без парсинга outbounds

- [x] **SUBTASK_SAVE_WITHOUT_PARSE** — не парсить при Save; сохранять текущее состояние; после Save запускать Update с главной (RunParserProcess). Подробно: [SUBTASK_SAVE_WITHOUT_PARSE.md](SUBTASK_SAVE_WITHOUT_PARSE.md).
