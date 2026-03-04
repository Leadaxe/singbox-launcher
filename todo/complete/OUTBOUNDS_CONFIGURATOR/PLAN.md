## PLAN: OUTBOUNDS_CONFIGURATOR — вкладки Sources и Outbounds

### 1. Общая идея

- **Разделить текущую вкладку `Sources and ParserConfig` на две**: `Sources` и `Outbounds and ParserConfig`.
- **Встроить конфигуратор outbounds во вторую вкладку**, чтобы убрать отдельное окно Config Outbounds.
- **Сделать `config.ParserConfig` единственным источником правды** и синхронизировать с ним все изменения из Sources, UI‑списка outbounds и текстового `ParserConfigJSON`.

### 2. Компоненты и изменения по файлам

- **Визард и вкладки**
  - Актуализировать модель визарда (поле `ParserConfig`, `ParserConfigJSON`, список Sources/Outbounds).
  - В `wizard.go` и связанных файлах:
    - выделить из текущего `CreateSourceTab` два таба:
      - `Sources` — работа с источниками и Preview;
      - `Outbounds and ParserConfig` — работа с outbounds и текстовым ParserConfigJSON;
    - перенести UI‑элементы ParserConfig (многострочный `ParserConfigEntry`, кнопки Documentation и ChatGPT) и управление outbounds из первой вкладки во вторую.
  - Удалить кнопку Parse из UI (вместе с текущей логикой `ParseButton` в `source_tab.go`) и заменить её поведение автосинхронизацией ParserConfig и триггером для Rules/Preview.

- **Вкладка `Sources`**
  - На базе существующего `source_tab.go`:
    - оставить поле `SourceURLEntry`, подсказку и статус проверки URL (текущая логика `ApplyURLToParserConfig` и CheckURL уже используется);
    - убрать с этой вкладки элементы ParserConfig (`ParserConfigEntry`, ParseButton, Config Outbounds, Documentation/ChatGPT);
    - добавить список Sources (по `ProxySource`) с tooltip’ами;
    - оставить/адаптировать read‑only Preview сгенерированных нод/селекторов (re‑use текущего `OutboundsPreview`).
  - Убедиться, что на вкладке `Sources` нет поля ParserConfigJSON и кнопки Parse.

- **Вкладка `Outbounds and ParserConfig`**
  - Переиспользовать текущую реализацию окна Config Outbounds в `ui/wizard/outbounds_configurator/configurator.go` и диалога в `edit_dialog.go`:
    - вынести из функции `Show` построение списка outbounds (`collectRows`, `tagsAbove`, Up/Down, Edit/Delete/Add) в отдельный компонент/функцию, возвращающий контейнер для встраивания во вкладку;
    - использовать тот же диалог `ShowEditDialog` для Edit/Add, но привязать его к основному окну визарда, а не к отдельному `NewWindow`.
  - Во второй вкладке:
    - отрисовать общий список outbounds (локальные по источникам → глобальные) с иконками Up/Down, Edit/Delete, Add;
    - добавить многострочный редактор ParserConfigJSON с кнопками Documentation и ChatGPT (перенеся их из `source_tab.go`);
    - связать список и редактор с `config.ParserConfig` модели визарда (из `WizardPresenter`), без отдельного окна.

- **Синхронизация ParserConfig**
  - Учитывая, что `SourceURLEntry.OnChanged` уже вызывает `ApplyURLToParserConfig` и выставляет `PreviewNeedsParse`:
    - дополнить цепочку так, чтобы после успешного `ApplyURLToParserConfig` происходила нормализация ParserConfig, сериализация в `ParserConfigJSON` и обновление UI второй вкладки (редактор ParserConfigJSON, список Sources/Outbounds).
  - Для операций Edit/Add/Delete/Up/Down в списке outbounds (логика уже частично реализована в `configurator.go`):
    - перенастроить их так, чтобы они модифицировали только структуру ParserConfig в модели визарда;
    - после каждой операции вызывать нормализацию и сериализацию в `ParserConfigJSON`, обновление редактора и Preview.
  - Для ручного редактирования ParserConfigJSON во второй вкладке:
    - реализовать парсинг текста при потере фокуса / по debounce / при переключении вкладки, с валидацией JSON;
    - при успехе — заменять структуру `config.ParserConfig` и пересчитывать списки Sources/Outbounds;
    - при ошибке — показывать `dialogs.ShowError` и откатывать текст к последнему валидному состоянию.

- **Интеграция с Rules/Preview и удаление старого окна**
  - Обновить вкладки Rules/Preview так, чтобы они всегда брали данные из актуального ParserConfig (структуры в модели визарда).
  - Привязать запуск Parse/Preview к изменениям ParserConfig и/или переходу на соответствующие вкладки (ре‑use существующих `TriggerParseForPreview`/`UpdateTemplatePreviewAsync` вместо отдельной кнопки Parse).
  - Постепенно отказаться от отдельного окна Config Outbounds:
    - на переходном этапе использовать общую реализацию списка/диалога и для вкладки, и для окна;
    - после переноса всех сценариев во вкладку `Outbounds and ParserConfig` удалить кнопку `Config Outbounds` из `source_tab.go` и функцию `Show` из `outbounds_configurator`.

### 3. Этапы реализации

1. **Перестроить вкладки визарда** (разделение Sources / Outbounds and ParserConfig, перенос существующего UI).
2. **Реализовать список Sources и Preview** на первой вкладке (включая tooltip’ы и привязку к ParserConfig.ParserConfig.Proxies).
3. **Реализовать конфигуратор outbounds** на второй вкладке (список, Up/Down, Edit/Delete/Add, диалог).
4. **Настроить полную синхронизацию ParserConfig** между структурой модели, SourceURLEntry, UI‑списком и текстовым JSON.
5. **Интегрировать изменения с Rules/Preview** и удалить старое окно Config Outbounds, провести финальную проверку по критериям приёмки SPEC.

### 4. Как реализовано (соответствие коду)

- Вкладки: **Sources** и **Outbounds** (название в UI — "Outbounds"). Создание в `wizard.go`; при уходе с Outbounds вызывается `ValidateAndApplyParserConfigFromEntry`.
- **Sources**: поле URL + кнопка Add (только по нажатию, `AppendURLsToParserConfig`, дедупликация), список с prefix/View/Del, Preview с Refresh и списком серверов по всем источникам. CheckURL и статус URL удалены.
- **Outbounds**: редактор ParserConfig JSON, кнопка Documentation, встроенный конфигуратор (`outbounds_configurator.NewConfiguratorContent`) с ↑/↓, Edit, Del, Add; диалог Add/Edit — отдельное окно. Parse и ChatGPT убраны. Синхронизация: onApply после каждой мутации; ручной JSON — при смене вкладки.
- Детали: [IMPLEMENTATION_REPORT.md](IMPLEMENTATION_REPORT.md).
