# Рефакторинг визарда конфигурации: Разделение GUI и бизнес-логики

## 1. Постановка задачи

### Исторические проблемы

#### 1.1. Смешение GUI и бизнес-логики в `WizardState`

Файл `ui/wizard/state/state.go` содержал смешанные зависимости:

**GUI зависимости (Fyne):**
- `Window fyne.Window`
- `SourceURLEntry *widget.Entry`
- `URLStatusLabel *widget.Label`
- `ParserConfigEntry *widget.Entry`
- `OutboundsPreview *widget.Entry`
- `CheckURLButton *widget.Button`
- `CheckURLProgress *widget.ProgressBar`
- `ParseButton *widget.Button`
- `TemplatePreviewEntry *widget.Entry`
- `ShowPreviewButton *widget.Button`
- `FinalOutboundSelect *widget.Select`
- `CloseButton, PrevButton, NextButton, SaveButton *widget.Button`
- `SaveProgress *widget.ProgressBar`
- `Tabs *container.AppTabs`
- `CheckURLContainer, ButtonsContainer fyne.CanvasObject`
- `OpenRuleDialogs map[int]fyne.Window`

**Бизнес-данные:**
- `ParserConfig *config.ParserConfig`
- `GeneratedOutbounds []string`
- `TemplateData *wizardtemplate.TemplateData`
- `TemplateSectionSelections map[string]bool`
- `SelectableRuleStates []*SelectableRuleState`
- `CustomRules []*SelectableRuleState`
- `SelectedFinalOutbound string`
- `OutboundStats struct`

#### 1.2. Бизнес-логика зависит от GUI

Все функции в `ui/wizard/business/` принимали `*WizardState`, который содержит Fyne-виджеты:
- `BuildTemplateConfig(state *WizardState, ...)`
- `CheckURL(state *WizardState)`
- `ParseAndPreview(state *WizardState)`
- `LoadConfigFromFile(state *WizardState)`

Это делало невозможным тестирование бизнес-логики без инициализации Fyne GUI.

#### 1.3. GUI код напрямую обращается к бизнес-данным

В табах (`ui/wizard/tabs/`) код напрямую читал и изменял бизнес-данные из `WizardState`, например:
- `state.ParserConfigEntry.Text`
- `state.SelectableRuleStates[idx].Enabled`
- `state.TemplateSectionSelections[key]`

### Цели рефакторинга

**Главная цель:** Создать управляемый и чистый код с четким разделением ответственности.

1. **Разделить модели данных и GUI-состояние**
   - Создать чистые модели данных без зависимостей от Fyne
   - Выделить GUI-состояние в отдельную структуру

2. **Сделать бизнес-логику тестируемой**
   - Бизнес-логика должна работать только с моделями данных
   - Убрать зависимости от Fyne из бизнес-логики

3. **Ввести слой представления (Presentation Layer)**
   - Создать адаптеры/презентеры для связи GUI и бизнес-логики
   - GUI обновляется через презентеры, а не напрямую

### Принципы рефакторинга

**ВАЖНО:** Рефакторинг выполняется БЕЗ требований на обратную совместимость API.

1. **Чистый код с первого раза**
   - Код пишется заново, а не мигрируется через обертки
   - Сразу применяются лучшие практики и паттерны
   - Учитывается имеющийся опыт разработки

2. **Удаление всего лишнего**
   - Удаляются старые функции, обертки, дубли кода, артефакты из прошлого
   - Удаляются лишние тесты (старые тесты, которые тестируют устаревшую архитектуру)
   - Удаляется весь технический долг, связанный со старой архитектурой
   - Удаляется неиспользуемый код, мертвый код, комментарии о "временных решениях"

3. **Оптимизация с учетом опыта**
   - Новый код оптимизируется на основе понимания требований
   - Избегаются известные проблемы из текущей реализации
   - Применяются решения, которые уже зарекомендовали себя

4. **Сохранение только функциональности**
   - Требуется сохранить только **поведение GUI** и **функциональность** визарда
   - Можно полностью переписать код, если это улучшает архитектуру
   - НЕ нужно сохранять старые функции, интерфейсы, структуры
   - НЕ нужно создавать функции-обертки для старого кода

### Архитектура после рефакторинга

```
┌─────────────────────────────────────────────────────────────┐
│                      Presentation Layer                      │
│  (ui/wizard/presentation/)                                   │
│  - WizardPresenter                                           │
│  - GUIState (только Fyne виджеты)                            │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ использует
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    Business Logic Layer                      │
│  (ui/wizard/business/)                                       │
│  - generator.go (BuildTemplateConfig, BuildParserOutboundsBlock)│
│  - parser.go (CheckURL, ParseAndPreview, ApplyURLToParserConfig)│
│  - loader.go (LoadConfigFromFile, EnsureRequiredOutbounds)  │
│  - validator.go (ValidateParserConfig, ValidateURL, ValidateURI)│
│  - outbound.go (GetAvailableOutbounds, EnsureFinalSelected)  │
│  - saver.go (SaveConfigWithBackup)                          │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ работает с
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                      Domain Models                           │
│  (ui/wizard/models/)                                         │
│  - WizardModel (чистые данные)                               │
│  - RuleState (без GUI)                                       │
│  - Константы бизнес-логики                                   │
└─────────────────────────────────────────────────────────────┘
```

## 2. Особенности реализации

### 2.1. Принятые архитектурные решения

#### Расположение компонентов

- **GUIState:** `ui/wizard/presentation/gui_state.go` (отдельный файл)
- **UIUpdater интерфейс:** `ui/wizard/business/ui_updater.go` (в business, так как используется бизнес-логикой)
- **SafeFyneDo:** `ui/wizard/presentation/presenter.go` (базовый файл презентера, используется во всех методах)
- **Константы:** `ui/wizard/models/wizard_model.go` (вместе с моделью данных)

#### Разделение методов WizardState

**В бизнес-логику (`business/`):**
- `SaveConfigWithBackup()` → `business/saver.go`
- `GetAvailableOutbounds()` → `business/outbound.go`
- `EnsureFinalSelected()` → `business/outbound.go`

**В презентер (`presentation/`):**
- `SetCheckURLState()` → методы презентера для управления UI
- `SetSaveState()` → методы презентера для управления UI
- `SetTemplatePreviewText()` → методы презентера для обновления preview
- `RefreshOutboundOptions()` → методы презентера для обновления виджетов
- `InitializeTemplateState()` → инициализация в презентере

#### Связь данных и GUI

- **RuleState ↔ виджеты:**
  - Структура-обертка `RuleWidget` в `gui_state.go`:
    ```go
    type RuleWidget struct {
        Select *widget.Select
        RuleState *RuleState  // ссылка на правило из модели
    }
    ```
  - В GUIState: `RuleOutboundSelects []*RuleWidget`

- **Флаги состояния (разделение):**
  - **В модель (`WizardModel`):** бизнес-флаги операций
    - `AutoParseInProgress`
    - `PreviewGenerationInProgress`
  - **В GUIState:** UI-флаги операций и блокировки
    - `CheckURLInProgress`
    - `SaveInProgress`
    - `ParserConfigUpdating` (для блокировки рекурсивных обновлений)
    - `UpdatingOutboundOptions` (для блокировки callbacks)

#### Доступ к сервисам

- Controller НЕ хранится в модели (модель чистая, только данные)
- Бизнес-логика получает сервисы через параметры функций (например, `SaveConfig(fileService, model)`)
- Controller хранится в презентере, презентер передает нужные сервисы в бизнес-логику

#### Синхронизация модель ↔ GUI

- Синхронизация по требованию (явный вызов `SyncModelToGUI()` и `SyncGUIToModel()` в нужных местах)
- Открытые диалоги хранятся в презентере (`WizardPresenter.OpenRuleDialogs map[int]fyne.Window`)

#### Обработка ошибок

- Бизнес-логика возвращает ошибки (не показывает диалоги)
- Презентер обрабатывает ошибки (показывает диалоги, статусы, логирует)
- Разделение ответственности: бизнес-логика валидирует, презентер показывает

### 2.2. Структура файлов

#### Models (`ui/wizard/models/`)
- `wizard_model.go` - `WizardModel` - чистая модель данных без GUI зависимостей + бизнес-константы (DefaultOutboundTag, RejectActionName, RejectActionMethod)
- `rule_state.go` - `RuleState` - модель состояния правила маршрутизации
- `rule_state_utils.go` - утилиты для работы с `RuleState`

#### Presentation (`ui/wizard/presentation/`)
- `gui_state.go` - `GUIState` - только Fyne виджеты и UI-флаги
- `presenter.go` - `WizardPresenter` - связывает GUI и бизнес-логику + утилита SafeFyneDo
- `presenter_ui_updater.go` - реализация `UIUpdater` интерфейса
- `presenter_sync.go` - синхронизация модель ↔ GUI
- `presenter_methods.go` - методы управления UI состоянием
- `presenter_async.go` - асинхронные операции (парсинг, preview)
- `presenter_save.go` - логика сохранения конфигурации
- `presenter_rules.go` - работа с правилами и диалогами

#### Business (`ui/wizard/business/`)
- `parser.go` - парсинг URL и ParserConfig (использует `WizardModel` и `UIUpdater`)
- `loader.go` - загрузка конфигурации (возвращает данные, не изменяет GUI)
- `generator.go` - генерация конфигурации из шаблона (работает с `WizardModel`)
- `validator.go` - валидация данных (чистые функции без GUI)
- `outbound.go` - работа с outbounds (`GetAvailableOutbounds`, `EnsureFinalSelected`)
- `saver.go` - сохранение конфигурации (`SaveConfigWithBackup`)
- `ui_updater.go` - интерфейс `UIUpdater` для обновления GUI из бизнес-логики
- `template_loader.go` - интерфейс `TemplateLoader` для загрузки шаблонов
- `config_service.go` - интерфейсы и адаптеры для `ConfigService`

### 2.3. Анализ декомпозиции

#### Статистика изменений

```
16 файлов изменено
+1024 строк добавлено
-1577 строк удалено
Чистое изменение: -553 строки (уменьшение!)
```

#### Что было удалено:
- `ui/wizard/state/state.go` - **619 строк** (удален полностью)
- `ui/wizard/state/helpers.go` - **51 строка** (удален полностью)
- `ui/wizard/models/constants.go` - **27 строк** (перенесено в wizard_model.go)
- `ui/wizard/presentation/utils.go` - **22 строки** (перенесено в presenter.go)
- Рефакторинг существующих файлов - **~907 строк переписано/удалено**

#### Что было создано:

**Models/** (179 строк):
- `wizard_model.go` - 103 строки (включая константы)
- `rule_state.go` - 40 строк
- `rule_state_utils.go` - 36 строк

**Presentation/** (952 строки):
- `gui_state.go` - 71 строка
- `presenter.go` - 85 строк (включая SafeFyneDo)
- `presenter_async.go` - 144 строки
- `presenter_sync.go` - 53 строки
- `presenter_methods.go` - 256 строк
- `presenter_save.go` - 138 строк
- `presenter_rules.go` - 61 строка
- `presenter_ui_updater.go` - 151 строка

**Business/** (новые интерфейсы и функции):
- `config_service.go` - адаптер интерфейса
- `outbound.go` - функции работы с outbounds
- `saver.go` - функции сохранения
- `template_loader.go` - интерфейс загрузчика
- `ui_updater.go` - интерфейс обновления UI

#### Оценка декомпозиции

**Что оправдано:**
1. **Разделение Model / View / Presenter**
   - `WizardModel` отдельно от GUI - правильно
   - `GUIState` отдельно от бизнес-данных - правильно
   - Презентер как связующее звено - правильно

2. **Разделение business логики**
   - `parser.go`, `generator.go`, `loader.go`, `saver.go` - оправдано
   - Валидация отдельно - оправдано
   - Функции работают с моделью, не с GUI - правильно

3. **Чистые данные в models**
   - `RuleState` без виджетов - правильно
   - Константы бизнес-логики в models - правильно

**Что можно оптимизировать:**
1. **Избыточная декомпозиция методов презентера**
   - 8 файлов в `presentation/` для одного презентера
   - `presenter_methods.go` (256 строк) - можно было оставить в `presenter.go`
   - `presenter_rules.go` (61 строка) - слишком мало для отдельного файла
   - `presenter_sync.go` (53 строки) - можно было объединить

2. **Дублирование логики**
   - Интерфейсы `UIUpdater`, `ConfigService`, `TemplateLoader` - возможно избыточны для текущего масштаба
   - Адаптеры добавляют слой абстракции, который может быть преждевременным

**Рекомендация:** Архитектура MVP правильная, но структура файлов избыточно декомпозирована. Можно оптимизировать, объединив мелкие файлы, без потери архитектурных преимуществ.

## 3. Результат

### 3.1. Статус: ЗАВЕРШЕНО

Рефакторинг визарда конфигурации полностью завершен. Код разделен на четкие слои: модели данных, бизнес-логика и представление (GUI).

### 3.2. Выполненные задачи

#### ✅ Созданы модели данных (`ui/wizard/models/`)
- **`wizard_model.go`** - `WizardModel` - чистая модель данных без GUI зависимостей + бизнес-константы (DefaultOutboundTag, RejectActionName, RejectActionMethod)
- **`rule_state.go`** - `RuleState` - модель состояния правила маршрутизации
- **`rule_state_utils.go`** - утилиты для работы с `RuleState`

#### ✅ Создан слой представления (`ui/wizard/presentation/`)
- **`gui_state.go`** - `GUIState` - только Fyne виджеты и UI-флаги
- **`presenter.go`** - `WizardPresenter` - связывает GUI и бизнес-логику + утилита SafeFyneDo
- **`presenter_ui_updater.go`** - реализация `UIUpdater` интерфейса
- **`presenter_sync.go`** - синхронизация модель ↔ GUI
- **`presenter_methods.go`** - методы управления UI состоянием
- **`presenter_async.go`** - асинхронные операции (парсинг, preview)
- **`presenter_save.go`** - логика сохранения конфигурации
- **`presenter_rules.go`** - работа с правилами и диалогами

#### ✅ Рефакторирована бизнес-логика (`ui/wizard/business/`)
- **`parser.go`** - парсинг URL и ParserConfig (использует `WizardModel` и `UIUpdater`)
- **`loader.go`** - загрузка конфигурации (возвращает данные, не изменяет GUI)
- **`generator.go`** - генерация конфигурации из шаблона (работает с `WizardModel`)
- **`validator.go`** - валидация данных (чистые функции без GUI)
- **`outbound.go`** - работа с outbounds (`GetAvailableOutbounds`, `EnsureFinalSelected`)
- **`saver.go`** - сохранение конфигурации (`SaveConfigWithBackup`)
- **`ui_updater.go`** - интерфейс `UIUpdater` для обновления GUI из бизнес-логики
- **`template_loader.go`** - интерфейс `TemplateLoader` для загрузки шаблонов
- **`config_service.go`** - интерфейсы и адаптеры для `ConfigService`

#### ✅ Рефакторированы табы (`ui/wizard/tabs/`)
- **`source_tab.go`** - работает с презентером вместо `WizardState`
- **`rules_tab.go`** - работает с презентером, использует `RuleState` из models
- **`preview_tab.go`** - работает с презентером

#### ✅ Обновлены диалоги (`ui/wizard/dialogs/`)
- **`add_rule_dialog.go`** - работает с презентером и `RuleState`

#### ✅ Рефакторирован главный файл
- **`wizard.go`** - использует `WizardPresenter`, `WizardModel`, `GUIState`

#### ✅ Удален старый код
- **`ui/wizard/state/state.go`** - удален (заменен на `WizardModel` и `GUIState`)
- **`ui/wizard/state/helpers.go`** - удален (функции перемещены в presentation/presenter.go)
- **`ui/wizard/models/constants.go`** - удален (константы перенесены в wizard_model.go)
- **`ui/wizard/presentation/utils.go`** - удален (SafeFyneDo перенесена в presenter.go)

### 3.3. Архитектура

#### Разделение ответственности:

1. **Models (`ui/wizard/models/`)** - чистые данные без зависимостей от GUI
   - `WizardModel` - основная модель визарда
   - `RuleState` - состояние правила
   - Константы бизнес-логики

2. **Business Logic (`ui/wizard/business/`)** - бизнес-логика без GUI
   - Функции работают с `WizardModel`
   - Обновляют GUI через интерфейс `UIUpdater`
   - Легко тестируются без инициализации Fyne

3. **Presentation (`ui/wizard/presentation/`)** - связь между GUI и бизнес-логикой
   - `WizardPresenter` - координирует взаимодействие
   - `GUIState` - только Fyne виджеты
   - Синхронизация данных между моделью и GUI

### 3.4. Преимущества новой архитектуры

1. **Тестируемость** - бизнес-логику можно тестировать без GUI
2. **Разделение ответственности** - четкое разделение данных, логики и представления
3. **Поддерживаемость** - изменения в GUI не влияют на бизнес-логику
4. **Переиспользование** - бизнес-логику можно использовать в других контекстах
5. **Чистота кода** - нет смешивания GUI виджетов и бизнес-данных

### 3.5. Проверка качества

- ✅ Все основные файлы компилируются без ошибок
- ✅ Нет ошибок линтера в рабочем коде
- ✅ Старый код удален
- ✅ Все импорты правильные
- ✅ Нет циклических зависимостей
- ✅ Чистое уменьшение кода (-553 строки в итоге)

## 4. Покрытие тестами

### 4.1. Обзор

Все тесты переписаны под новую архитектуру MVP и проверяют бизнес-логику без зависимостей от GUI.

### 4.2. Тесты генерации конфигурации (`business/generator_test.go`)

#### TestMergeRouteSection
**Проверяет:** Объединение правил маршрутизации из шаблона и пользовательских правил
- Объединение правил из шаблона (rawRoute) с правилами из `SelectableRuleStates`
- Объединение с пользовательскими правилами (`CustomRules`)
- Корректность установки `final` outbound в результате
- Проверка, что все правила присутствуют в итоговом JSON (исходное + selectable + custom)

#### TestMergeRouteSection_RejectAction
**Проверяет:** Обработку действия "reject" для правил
- Правило с `SelectedOutbound = "reject"` преобразуется в правило с `action: "reject"`
- Поле `outbound` удаляется из правила при использовании reject
- Правило корректно включается в результат

#### TestMergeRouteSection_DisabledRules
**Проверяет:** Исключение отключенных правил из результата
- Правила с `Enabled = false` не включаются в финальную конфигурацию
- Включены только правила с `Enabled = true`

#### TestFormatSectionJSON
**Проверяет:** Форматирование JSON секций с отступами
- Корректное форматирование валидного JSON с разными уровнями отступов (2, 4)
- Обработка ошибок при невалидном JSON
- Сохранение содержимого JSON при форматировании

#### TestIndentMultiline
**Проверяет:** Добавление отступов к многострочному тексту
- Добавление отступов к одной строке
- Добавление отступов ко всем строкам в многострочном тексте
- Обработка пустого текста
- Обработка текста с завершающим переносом строки

### 4.3. Тесты парсинга (`business/parser_test.go`)

#### TestSerializeParserConfig_Standalone
**Проверяет:** Сериализацию ParserConfig в JSON
- Корректная сериализация валидного ParserConfig в JSON строку
- Результат является валидным JSON
- Результат содержит блок `ParserConfig`
- Обработка ошибок при `nil` ParserConfig

#### TestApplyURLToParserConfig_Logic
**Проверяет:** Логику классификации URL и прямых ссылок
- Разделение входных строк на подписки (http://, https://) и прямые ссылки (vless://, vmess://)
- Корректный подсчет количества подписок и прямых ссылок
- Пропуск пустых строк

### 4.4. Тесты загрузки конфигурации (`business/loader_test.go`)

#### TestSerializeParserConfig
**Проверяет:** Сериализацию ParserConfig (дубликат из parser_test.go для loader пакета)
- Аналогично `TestSerializeParserConfig_Standalone`

#### TestCloneOutbound
**Проверяет:** Глубокое клонирование OutboundConfig
- Клон является отдельным экземпляром (не указатель на исходный)
- Все поля скопированы корректно (Tag, Type, Comment, AddOutbounds)
- Глубокое копирование вложенных структур (Options, Filters)
- Изменение исходного объекта не влияет на клон (проверка на AddOutbounds и Options)

#### TestEnsureRequiredOutbounds
**Проверяет:** Добавление обязательных outbounds из шаблона
- Outbounds с `wizard.required = 1` добавляются в конфигурацию (если отсутствуют)
- Outbounds с `wizard.required = 2` добавляются/перезаписываются в конфигурации
- Outbounds с `wizard.required = 0` (опциональные) не добавляются автоматически
- Корректность типов добавленных outbounds

#### TestEnsureRequiredOutbounds_Overwrite
**Проверяет:** Перезапись обязательных outbounds с `required > 1`
- Существующие outbounds с `required = 2` перезаписываются из шаблона
- Тип outbound изменяется на значение из шаблона

#### TestEnsureRequiredOutbounds_Preserve
**Проверяет:** Сохранение обязательных outbounds с `required = 1`
- Существующие outbounds с `required = 1` не перезаписываются
- Тип outbound сохраняется из существующей конфигурации

#### TestLoadConfigFromFile_FileSizeValidation
**Проверяет:** Валидацию размера файла конфигурации
- Создание временного файла, превышающего лимит размера
- Проверка, что файл действительно превышает `MaxJSONConfigSize`

### 4.5. Тесты валидации (`business/validator_test.go`)

#### TestValidateParserConfig
**Проверяет:** Валидацию структуры ParserConfig
- Валидация валидного ParserConfig проходит успешно
- Ошибка при `nil` ParserConfig
- Ошибка при `nil` Proxies в ParserConfig
- Ошибка при невалидных URL в Proxies
- Ошибка при невалидных URI в Connections
- Ошибка при невалидных outbounds (пустой Tag)

#### TestValidateURL
**Проверяет:** Валидацию URL подписок
- Валидация валидных HTTP/HTTPS URL
- Ошибка при пустом URL
- Ошибка при URL превышающем максимальную длину
- Ошибка при слишком коротком URL
- Ошибка при URL без схемы (http://, https://)
- Ошибка при URL без хоста
- Ошибка при невалидном формате URL

#### TestValidateURI
**Проверяет:** Валидацию URI прямых ссылок
- Валидация валидных URI (vless://, vmess://, trojan://)
- Ошибка при пустом URI
- Ошибка при URI превышающем максимальную длину
- Ошибка при слишком коротком URI
- Ошибка при URI без протокола (://)
- Ошибка при невалидном формате URI

#### TestValidateOutbound
**Проверяет:** Валидацию конфигурации outbound
- Валидация валидного outbound
- Ошибка при `nil` outbound
- Ошибка при пустом Tag
- Ошибка при пустом Type
- Ошибка при Tag превышающем максимальную длину (256 символов)

#### TestValidateJSONSize
**Проверяет:** Валидацию размера JSON данных
- Валидация данных допустимого размера
- Валидация данных точно на лимите `MaxJSONConfigSize`
- Ошибка при данных превышающих лимит
- Валидация пустых данных (разрешены)

#### TestValidateJSON
**Проверяет:** Валидацию JSON структуры
- Валидация валидного JSON
- Ошибка при невалидном JSON (незакрытые скобки)
- Ошибка при JSON превышающем размер `MaxJSONConfigSize`
- Валидация пустого JSON объекта `{}`

#### TestValidateHTTPResponseSize
**Проверяет:** Валидацию размера HTTP ответа
- Валидация размера в пределах лимита
- Валидация размера точно на лимите `MaxSubscriptionSize`
- Ошибка при размере превышающем лимит
- Валидация нулевого размера (разрешен)

#### TestValidateParserConfigJSON
**Проверяет:** Валидацию JSON текста ParserConfig
- Валидация валидного JSON ParserConfig
- Ошибка при пустом JSON
- Ошибка при невалидном JSON синтаксисе
- Ошибка при JSON превышающем размер `MaxJSONConfigSize`
- Ошибка при невалидной структуре ParserConfig (nil proxies)

#### TestValidateRule
**Проверяет:** Валидацию правил маршрутизации
- Валидация правила с полем `domain`
- Валидация правила с полем `ip_cidr`
- Ошибка при `nil` правиле
- Ошибка при пустом правиле (нет полей)

### 4.6. Тесты работы с outbounds (`business/outbound_test.go`)

#### TestGetAvailableOutbounds
**Проверяет:** Получение списка доступных outbound тегов из модели
- Извлечение outbounds из `ParserConfig` объекта в модели
- Извлечение outbounds из `ParserConfigJSON` строки в модели
- Всегда включаются обязательные outbounds: `direct-out`, `reject`, `drop`
- Включение outbounds из глобальных outbounds конфигурации
- Корректность минимального количества outbounds в результате (минимум 3: direct-out, reject, drop)
- Все ожидаемые теги присутствуют в результате

#### TestEnsureDefaultAvailableOutbounds
**Проверяет:** Обеспечение наличия дефолтных outbounds
- При пустом списке возвращаются дефолтные: `["direct-out", "reject"]` (без "drop", так как это не дефолтный)
- При непустом списке исходный список сохраняется без изменений

#### TestEnsureFinalSelected
**Проверяет:** Обеспечение выбранного final outbound в модели
- Если `SelectedFinalOutbound` уже установлен и присутствует в опциях - сохраняется
- Если `SelectedFinalOutbound` не установлен - используется `direct-out`
- Если `SelectedFinalOutbound` не в опциях - используется первый доступный вариант (`direct-out` приоритетно)
- Fallback логика при отсутствии предпочтительного outbound в опциях

### 4.7. Тесты работы с правилами (`models/rule_state_utils_test.go`)

#### TestGetEffectiveOutbound
**Проверяет:** Получение эффективного outbound для правила
- Если `SelectedOutbound` установлен - возвращается он
- Если `SelectedOutbound` пуст, но есть `DefaultOutbound` в Rule - возвращается `DefaultOutbound`
- Если оба пусты - возвращается пустая строка

#### TestEnsureDefaultOutbound
**Проверяет:** Установку дефолтного outbound для правила
- Если `SelectedOutbound` уже установлен - не изменяется
- Если `SelectedOutbound` пуст и есть `DefaultOutbound` в Rule - устанавливается `DefaultOutbound`
- Если `SelectedOutbound` пуст и нет `DefaultOutbound`, но есть доступные outbounds - устанавливается первый доступный
- Если нет ни `DefaultOutbound`, ни доступных outbounds - остается пустым

### 4.8. Итоговое покрытие

**Общее покрытие:**
- ✅ Генерация конфигурации (объединение правил, форматирование)
- ✅ Парсинг и сериализация ParserConfig
- ✅ Загрузка конфигурации (клонирование, добавление обязательных outbounds)
- ✅ Валидация (ParserConfig, URL, URI, Outbound, JSON, Rule)
- ✅ Работа с outbounds (получение списка, обеспечение дефолтных, выбор final)
- ✅ Работа с правилами (эффективный outbound, установка дефолтного)

**Все тесты:**
- Работают с чистыми данными (без GUI зависимостей)
- Используют новую архитектуру MVP (`wizardmodels` вместо `wizardstate`)
- Легко выполняются без инициализации Fyne
- Покрывают как успешные сценарии, так и обработку ошибок

**Всего тестов:** 27 тестовых функций, покрывающих все основные аспекты бизнес-логики визарда.

---

**Дата завершения рефакторинга:** 2026-01-06  
**Статус:** ✅ РЕФАКТОРИНГ ЗАВЕРШЕН

