# IMPLEMENTATION_REPORT: OUTBOUNDS_CONFIGURATOR

Отчёт о реализованной функциональности по состоянию кода.

---

## 1. Вкладки визарда

- **Вкладка 1: Sources** (`ui/wizard/tabs/source_tab.go` — `CreateSourcesTab`)
- **Вкладка 2: Outbounds** (в UI называется "Outbounds", не "Outbounds and ParserConfig") — `CreateOutboundsAndParserConfigTab`
- Создание табов: `ui/wizard/wizard.go` — два таба создаются первыми, затем Rules, Preview.

---

## 2. Вкладка Sources — реализовано

### 2.1 Поле URL и кнопки

- Многострочное поле **SourceURLEntry** (подписки и прямые ссылки), подсказка под полем.
- **Автоприменения нет**: URL применяются только по кнопке **Add**. При изменении текста только выставляется `PreviewNeedsParse = true`.
- Кнопка **Add** справа от поля (в одной строке, через `Border`): вызывает `AppendURLsToParserConfig` — добавляет URL к существующим источникам, не заменяет список. После Add поле очищается.
- **Дубликаты**: при Add проверяется наличие источника (по `Source`) и набора connections; уже существующие не добавляются.
- Кнопка **Get free VPN!** в заголовке строки (справа).
- Нет поля ParserConfig JSON, нет кнопки Parse. Нет CheckURL, URLStatusLabel, CheckURLProgress/Button (весь этот функционал удалён).

### 2.2 Список Sources

- Строится по `model.ParserConfig.ParserConfig.Proxies`.
- В каждой строке: кнопка с коротким label (обрезанный URL или "Source N"), **редактируемое поле tag_prefix** (`prefixEntry`), кнопки **View** и **Del**.
- **Tooltip** на кнопке-лейбле: полный URL, `tag_prefix`/`tag_postfix`/`tag_mask`, локальные outbounds (через `SetToolTip` если доступен).
- **Del** удаляет соответствующий `ProxySource` из слайса, сериализует ParserConfig, обновляет UI и список.
- **View**: открывает отдельное окно, запрашивает/парсит ссылку подписки (или прямую ссылку) через `fetchAndParseSource`, показывает список серверов (нод) в формате как на вкладке Servers (`nodeDisplayLine`).

### 2.3 Preview внизу вкладки

- Блок без заголовка "Preview": одна строка — текст статуса ("Click Refresh to load servers from all sources." / "N server(s) from M source(s)") и кнопка **Refresh**.
- Список серверов — объединённый результат парсинга всех источников (те же вызовы, что и для View). Высота списка ограничена (~180px), список на всю ширину; справа зарезервирована полоса под скролл (прозрачный прямоугольник 10px в коде, в требованиях упоминалось 20px).
- По **Refresh** в фоне для каждого ProxySource вызывается `fetchAndParseSource(Source, Skip)`, результаты объединяются и отображаются в списке.

---

## 3. Вкладка Outbounds — реализовано

### 3.1 Структура вкладки

- Сверху: заголовок **ParserConfig:**, кнопка **📖 Documentation** (открывает ParserConfig.md).
- Многострочный редактор **ParserConfigEntry** (ParserConfig JSON), фиксированная высота ~200px со скроллом.
- Ниже: встроенный конфигуратор outbounds (список + кнопка Add). Кнопок **Parse** и **ChatGPT** на вкладке нет.

### 3.2 Конфигуратор outbounds (`ui/wizard/outbounds_configurator/configurator.go`)

- **NewConfiguratorContent(parent, parserConfig, onApply)** возвращает контент для встраивания во вкладку. `onApply` вызывается после каждой мутации (Up/Down, Edit, Del, Add).
- Порядок строк: сначала локальные outbounds по каждому источнику (`proxies[i].Outbounds`), затем глобальные `ParserConfig.Outbounds` (`collectRows`).
- В каждой строке: кнопки **↑** и **↓** (ASCII), текст `tag (type) — SourceLabel` (обрезание до 56 символов), справа **Edit** (иконка `DocumentCreateIcon`) и **Del** (иконка `DeleteIcon`). Справа в строке добавлен прозрачный отступ 30px под полосу прокрутки списка.
- Up/Down перемещают outbound только внутри своего scope (локальные по источнику / глобальные), переиспользуются `moveOutboundUp`, `moveOutboundDown`.
- После любой операции вызывается `onApply`: сериализация ParserConfig, обновление `model.ParserConfigJSON`, `UpdateParserConfig`, `RefreshOutboundOptions`, `RefreshSourcesList`.

### 3.3 Диалог Add/Edit Outbound (`edit_dialog.go`)

- Открывается как **отдельное окно** (`app.NewWindow`), не модальный диалог — по аналогии с диалогом добавления правил.
- Поля: Scope (For all / For source: &lt;label&gt;), Tag, Type (manual/auto), Comment, Filters (ключ `tag`), Preferred default (`tag`), AddOutbounds (direct-out, reject, теги outbounds выше в списке).
- `ShowEditDialog(..., onSave)` — при сохранении обновляется переданный outbound, вызывается `refreshList` и `onApply` у конфигуратора.

### 3.4 Синхронизация ParserConfig при ручном редактировании JSON

- При переключении **с вкладки Outbounds** вызывается `presenter.ValidateAndApplyParserConfigFromEntry()` (`wizard.go`, обработчик смены таба).
- Читается текст из `ParserConfigEntry`, парсится JSON; при успехе обновляются `model.ParserConfig`, `model.ParserConfigJSON`, `LastValidParserConfigJSON`, обновляется UI; при ошибке — `dialogs.ShowError`, откат текста в поле к `LastValidParserConfigJSON`.
- `LastValidParserConfigJSON` хранится в `GUIState` и выставляется при `SyncModelToGUI` при обновлении ParserConfig.

---

## 4. Бизнес-логика (parser.go)

- **ApplyURLToParserConfig** — полная замена списка источников по вводу (используется из ParseAndPreview при сохранении/превью, не из кнопки Add на Sources).
- **AppendURLsToParserConfig** — добавление URL к существующим источникам; используется кнопкой **Add** на вкладке Sources. Дубликаты отфильтровываются: подписки по существующему `Source`, connection-прокси по совпадению набора connections (`proxyListHasConnections`). Если добавлять нечего (все дубликаты), выходит без ошибки.
- **CheckURL** и весь связанный с ним UI (URLStatusLabel, SetCheckURLState, UpdateURLStatus, UpdateCheckURLProgress, UpdateCheckURLButtonText) удалены.

---

## 5. Файлы и ключевые изменения

| Область | Файлы |
|--------|--------|
| Вкладки | `ui/wizard/wizard.go` (табы Sources, Outbounds; при уходе с Outbounds — ValidateAndApplyParserConfigFromEntry) |
| Sources UI | `ui/wizard/tabs/source_tab.go` (URL, Add, список с prefix/View/Del, Preview с Refresh) |
| Outbounds UI | `ui/wizard/tabs/source_tab.go` (CreateOutboundsAndParserConfigTab: ParserConfig editor, configurator, Documentation) |
| Конфигуратор | `ui/wizard/outbounds_configurator/configurator.go`, `edit_dialog.go` |
| Синхронизация | `ui/wizard/presentation/presenter_sync.go` (SyncModelToGUI, ValidateAndApplyParserConfigFromEntry), `gui_state.go` (RefreshSourcesList, LastValidParserConfigJSON) |
| Бизнес-логика | `ui/wizard/business/parser.go` (AppendURLsToParserConfig, дедупликация; удалён CheckURL и связанное) |
| UI state | `ui/wizard/presentation/gui_state.go` (без URLStatusLabel, CheckURL*) |

---

## 6. Отличия от исходного SPEC/PLAN

- Вкладка называется **Outbounds**, не "Outbounds and ParserConfig".
- На Sources нет CheckURL, статуса проверки URL, кнопки Check; применение URL только по **Add**, добавление через **AppendURLsToParserConfig** с очисткой поля и дедупликацией.
- Preview на Sources — не read-only текст outbounds, а список **распарсенных серверов** по всем источникам с кнопкой **Refresh**; отдельного заголовка "Preview" нет.
- На Outbounds нет кнопок **Parse** и **ChatGPT**, только **Documentation**.
- Диалог Add/Edit outbound — **отдельное окно**, не модальный диалог.
- В списке outbounds кнопки Up/Down — символы **↑**/**↓**, кнопка удаления подписана **Del** с иконкой Delete; справа в строке отступ 30px под скролл.
- В списке Sources у каждого источника есть редактируемое поле **tag_prefix** и кнопка **View** (окно с серверами по этой ссылке).
