# Архитектура проекта singbox-launcher

## Обзор

Проект `singbox-launcher` представляет собой лаунчер для sing-box с графическим интерфейсом на базе Fyne. Архитектура проекта построена на принципах чистой архитектуры с четким разделением ответственности между компонентами.

## Принципы архитектуры

### 1. Разделение ответственности (Separation of Concerns)
- **Бизнес-логика** отделена от UI
- **Сервисы** инкапсулируют специфическую функциональность
- **Модели данных** отделены от обработки

### 2. Модульность
- Каждый функциональный блок вынесен в отдельный пакет
- Подпакеты группируют связанную функциональность
- Минимальные зависимости между модулями

### 3. Dependency Injection
- Сервисы получают зависимости через конструкторы
- Callback-функции для обратной связи между компонентами
- Минимизация циклических зависимостей

### 4. Единая точка входа
- `AppController` координирует все компоненты приложения
- Сервисы делегируют специфические задачи
- Централизованное управление состоянием

## Используемые библиотеки

Основные внешние зависимости (прямые из `go.mod`):

- **fyne.io/fyne/v2** — GUI: окна, виджеты, layout, приложение. Основа интерфейса лаунчера и визарда конфигурации.
- **github.com/dweymouth/fyne-tooltip** — тултипы для Fyne. В проекте:
  - Слой тултипов в окне: `fynetooltip.AddWindowToolTipLayer(content, canvas)` при установке контента главного окна и окна визарда; при закрытии визарда вызывается `DestroyWindowToolTipLayer`.
  - Виджеты с тултипами из `github.com/dweymouth/fyne-tooltip/widget`: кнопка Ping на вкладке Servers (Clash API), кнопка SRS в визарде (Rules). Ошибки Ping (одиночный и массовый `test`) хранятся в `APIService.LastPingError` и показываются в tooltip кнопки Ping.
- **github.com/muhammadmuzzammil1998/jsonc** — парсинг JSON с комментариями (JSONC) при чтении config.json.
- **github.com/mitchellh/go-ps** — список процессов (проверка запущенного sing-box и т.п.).
- **github.com/pion/stun** — STUN-запросы (проверка доступности сети).
- **github.com/txthinking/socks5** — клиент SOCKS5 для подписок и парсера.

Косвенные зависимости (драйверы Fyne, системный трей и т.д.) перечислены в `go.mod` в блоке `require` и не описаны здесь.

## Структура проекта

```
singbox-launcher/
├── main.go                    # Точка входа приложения
│   │   - main()               # Точка входа, инициализация AppController
│   │
├── core/                      # Ядро приложения
│   ├── controller.go          # Главный контроллер (AppController)
│   │   │   - NewAppController()              # Создание контроллера
│   │   │   - UpdateUI()                      # Обновление UI
│   │   │   - GracefulExit()                  # Корректное завершение
│   │   │   - StartSingBoxProcess()           # Запуск sing-box
│   │   │   - StopSingBoxProcess()             # Остановка sing-box
│   │   │   - CreateTrayMenu()                # Создание меню трея
│   │   │   - GetVPNButtonState()             # Состояние кнопок VPN
│   │   │
│   ├── config_service.go     # Сервис работы с конфигурацией
│   │   │   - NewConfigService()                    # Создание сервиса
│   │   │   - RunParserProcess()                    # Запуск парсинга
│   │   │   - UpdateConfigFromSubscriptions()        # Обновление из подписок
│   │   │
│   ├── process_service.go    # Сервис управления процессом sing-box
│   │   │   - NewProcessService()                  # Создание сервиса
│   │   │   - Start()                              # Запуск процесса
│   │   │   - Stop()                               # Остановка процесса
│   │   │   - Monitor()                            # Мониторинг процесса
│   │   │   - CheckIfRunningAtStart()              # Проверка при старте
│   │   │
│   ├── core_downloader.go    # Загрузка sing-box
│   │   │   - DownloadCore()                        # Загрузка sing-box
│   │   │   - ReleaseInfo struct                    # Информация о релизе
│   │   │   - Asset struct                          # Информация об ассете
│   │   │   - DownloadProgress struct               # Прогресс загрузки
│   │   │
│   ├── core_version.go       # Работа с версиями sing-box
│   │   │   - GetInstalledCoreVersion()             # Получение установленной версии
│   │   │   - GetLatestCoreVersion()                 # Получение последней версии
│   │   │   - CheckVersionInBackground()             # Проверка версии в фоне
│   │   │   - CompareVersions()                      # Сравнение версий
│   │   │   - CoreVersionInfo struct                 # Информация о версии
│   │   │
│   ├── wintun_downloader.go   # Загрузка wintun.dll
│   │   │   - DownloadWintunDLL()                     # Загрузка wintun.dll
│   │   │
│   ├── tray_menu.go           # Меню системного трея
│   │   │   - CreateTrayMenu()                # Создание меню трея
│   │   │   - addHideDockMenuItem()           # Скрытие Dock (macOS)
│   │   │
│   ├── auto_update.go         # Автообновление конфигурации
│   │   │   - startAutoUpdateLoop()           # Цикл автообновления
│   │   │   - shouldAutoUpdate()              # Проверка необходимости обновления
│   │   │   - attemptAutoUpdateWithRetries()  # Обновление с ретраями
│   │   │   - resumeAutoUpdate()              # Возобновление автообновления
│   │   │
│   ├── error_handler.go       # Обработка ошибок
│   │   │   - showErrorUI()                   # Единый метод отображения ошибок
│   │   │
│   ├── network_utils.go       # Сетевые утилиты
│   │   │   - CreateHTTPClient()                     # Создание HTTP клиента
│   │   │   - IsNetworkError()                       # Проверка сетевой ошибки
│   │   │   - GetNetworkErrorMessage()               # Сообщение об ошибке
│   │   │
│   ├── services/              # Сервисы приложения
│   │   ├── ui_service.go      # Управление UI состоянием и callbacks
│   │   │   │   - NewUIService()                     # Создание сервиса
│   │   │   │   - UpdateUI()                         # Обновление UI
│   │   │   │   - StopTrayMenuUpdateTimer()          # Остановка таймера
│   │   │   │   - QuitApplication()                  # Выход из приложения
│   │   │   │
│   │   ├── api_service.go     # Взаимодействие с Clash API
│   │   │   │   - NewAPIService()                    # Создание сервиса
│   │   │   │   - GetClashAPIConfig()                # Получение конфигурации API
│   │   │   │   - GetProxiesList()                    # Получение списка прокси
│   │   │   │   - SwitchProxy()                       # Переключение прокси
│   │   │   │   - AutoLoadProxies()                   # Автозагрузка прокси
│   │   │   │
│   │   ├── state_service.go   # Управление состоянием приложения
│   │   │   │   - NewStateService()                  # Создание сервиса
│   │   │   │   - GetCachedVersion()                  # Получение кешированной версии
│   │   │   │   - SetCachedVersion()                  # Установка кешированной версии
│   │   │   │   - IsAutoUpdateEnabled()               # Проверка автообновления
│   │   │   │   - SetAutoUpdateEnabled()             # Установка автообновления
│   │   │   │
│   │   └── file_service.go    # Управление файлами и путями
│   │       │   - NewFileService()                   # Создание сервиса
│   │       │   - OpenLogFiles()                      # Открытие лог-файлов
│   │       │   - CloseLogFiles()                     # Закрытие лог-файлов
│   │       │   - OpenLogFileWithRotation()          # Открытие лог-файла с ротацией
│   │       │   - CheckAndRotateLogFile()            # Проверка и ротация лог-файла
│   │       │   - GetMainLogFile()                    # Получение основного лог-файла
│   │       │   - BackupPath()                        # Путь для бэкапа файла (.old)
│   │       │   - BackupFile()                        # Создание бэкапа с ротацией (макс 1 старый)
│   │       │
│   └── config/                # Работа с конфигурацией
│       ├── models.go           # Модели данных конфигурации
│       │   │   - ParserConfig struct                # Конфигурация парсера
│       │   │   - ProxySource struct                 # Источник прокси
│       │   │   - OutboundConfig struct              # Конфигурация outbound
│       │   │   - WizardConfig struct                # Настройки визарда
│       │   │   - IsWizardHidden()                   # Проверка скрытия визарда
│       │   │   - GetWizardRequired()                # Получение обязательных полей
│       │   │
│       ├── config_loader.go    # Загрузка и чтение config.json
│       │   │   - GetSelectorGroupsFromConfig()      # Получение групп селекторов
│       │   │   - GetTunInterfaceName()              # Получение имени TUN интерфейса
│       │   │   - readConfigFile()                   # Чтение config.json
│       │   │   - cleanJSONC()                       # Очистка JSONC
│       │   │
│       ├── outbound_generator.go  # Генерация outbounds (ноды + селекторы)
│       │   │   - GenerateNodeJSON()                          # Генерация JSON узла
│       │   │   - GenerateSelectorWithFilteredAddOutbounds() # Генерация селектора с фильтрацией
│       │   │   - GenerateOutboundsFromParserConfig()        # Оркестрация: buildOutboundsInfo, computeOutboundValidity, generateSelectorJSONs
│       │   │   - OutboundGenerationResult struct             # Результат генерации
│       │   │   - outboundInfo struct                         # Информация о динамическом селекторе
│       │   │
│       ├── updater.go          # Обновление конфигурации
│       │   │   - UpdateConfigFromSubscriptions()        # Обновление из подписок
│       │   │   - writeToConfig()                        # Запись в config.json
│       │   │
│       ├── parser/             # Парсинг ParserConfig блока
│       │   ├── factory.go      # Фабрика ParserConfig
│       │   │   │   - ExtractParserConfig()                # Извлечение ParserConfig
│       │   │   │   - NormalizeParserConfig()               # Нормализация конфигурации
│       │   │   │   - LogDuplicateTagStatistics()          # Логирование статистики
│       │   │   │
│       │   ├── migrator.go     # Миграция версий
│       │   │   │   - (миграция версий @ParserConfig)
│       │   │   │
│       │   └── block_extractor.go  # Извлечение блока
│       │       │   - ExtractParserConfigBlock()            # Извлечение блока из JSON
│       │       │
│       └── subscription/       # Работа с подписками
│           ├── source_loader.go    # Загрузка узлов из источников
│           │   │   - LoadNodesFromSource()                   # Загрузка узлов
│           │   │   - applyTagPrefixPostfix()                 # Применение префикса/постфикса
│           │   │   - replaceTagVariables()                   # Замена переменных
│           │   │   - MakeTagUnique()                         # Уникальность тегов
│           │   │   - IsSubscriptionURL()                     # Проверка URL подписки
│           │   │
│           ├── node_parser.go      # Парсинг узлов прокси
│           │   │   - ParseNode()                               # Парсинг URI узла
│           │   │   - IsDirectLink()                             # Проверка прямого линка
│           │   │
│           ├── decoder.go          # Декодирование подписок
│           │   │   - DecodeSubscriptionContent()              # Декодирование (base64, yaml)
│           │   │
│           └── fetcher.go          # Загрузка подписок
│               │   - FetchSubscription()                      # Загрузка по HTTP
│               │
├── ui/                         # Пользовательский интерфейс
│   ├── app.go                  # Главное приложение UI
│   │   │   - NewApp()                                  # Создание главного окна
│   │   │   - GetTabs()                                 # Получение вкладок
│   │   │   - GetWindow()                               # Получение окна
│   │   │   - GetController()                           # Получение контроллера
│   │   │
│   ├── core_dashboard_tab.go  # Вкладка Core Dashboard
│   │   │   - CreateCoreDashboardTab()                  # Создание вкладки
│   │   │   - updateBinaryStatus()                      # Проверка бинарника
│   │   │   - updateRunningStatus()                     # Обновление статуса
│   │   │   - updateVersionInfo()                       # Обновление версии
│   │   │   - updateWintunStatus()                      # Обновление wintun.dll
│   │   │   - updateConfigInfo()                        # Обновление конфигурации
│   │   │
│   ├── clash_api_tab.go        # Вкладка Clash API
│   │   │   - CreateClashAPITab()                      # Создание вкладки
│   │   │   - onLoadAndRefreshProxies()                # Загрузка прокси
│   │   │   - onTestAPIConnection()                    # Тестирование API
│   │   │   - onResetAPIState()                        # Сброс состояния API
│   │   │   - pingProxy()                              # Пинг прокси
│   │   │
│   ├── diagnostics_tab.go      # Вкладка диагностики
│   │   │   - CreateDiagnosticsTab()                    # Создание вкладки диагностики
│   │   │
│   ├── help_tab.go             # Вкладка помощи
│   │   │   - CreateHelpTab()                           # Создание вкладки помощи
│   │   │
│   ├── dialogs.go              # Общие диалоги (реэкспорт из internal/dialogs)
│   │   │   - ShowError()                                # Показать ошибку
│   │   │   - ShowErrorText()                            # Показать текст ошибки
│   │   │   - ShowInfo()                                 # Показать информацию
│   │   │   - ShowConfirm()                              # Показать подтверждение
│   │   │   - ShowAutoHideInfo()                         # Автоскрываемая информация
│   │   │   - ShowDownloadFailedManual()                 # Диалог «загрузка не удалась — скачайте вручную» (ссылка, копирование, Open folder)
│   │   │   - ShowCustom() / NewCustom()                  # Кастомный диалог (через internal/dialogs)
│   │   │
│   ├── error_banner.go         # Баннеры ошибок
│   │   │   - NewErrorBanner()                           # Создание баннера ошибки
│   │   │   - ErrorBanner struct                         # Структура баннера
│   │   │
│   └── wizard/                 # Мастер конфигурации
│       ├── wizard.go           # Точка входа (ShowConfigWizard)
│       │   │   - ShowConfigWizard()                     # Точка входа визарда
│       │   │
│       ├── models/             # Модели данных визарда (без GUI зависимостей)
│       │   ├── wizard_model.go # WizardModel
│       │   │   │   - WizardModel struct                 # Модель данных визарда
│       │   │   │   - NewWizardModel()                   # Создание модели
│       │   │   │
│       │   ├── rule_state.go   # RuleState
│       │   │   │   - RuleState struct                   # Состояние правила
│       │   │   │
│       │   ├── rule_state_utils.go # Утилиты для RuleState
│       │   │   │   - GetEffectiveOutbound()             # Получение эффективного outbound
│       │   │   │   - EnsureDefaultOutbound()            # Установка дефолтного outbound
│       │   │   │
│       │   ├── wizard_state_file.go # Модель состояния визарда
│       │   │   │   - WizardStateFile struct                  # Сериализуемое состояние визарда (version 2)
│       │   │   │   - PersistedSelectableRuleState struct     # Упрощённое состояние правила (label, enabled, selected_outbound)
│       │   │   │   - PersistedCustomRule struct              # Полное определение пользовательского правила
│       │   │   │   - WizardStateMetadata struct              # Метаданные состояния
│       │   │   │   - ValidateStateID()                       # Валидация ID состояния
│       │   │   │   - MigrateSelectableRuleStates()           # Миграция v1 → v2 selectable rules
│       │   │   │   - MigrateCustomRules()                    # Миграция v1 → v2 custom rules
│       │   │   │   - NewWizardStateFile()                    # Фабрика для создания WizardStateFile из компонентов
│       │   │   │   - StateFileName const                     # Имя файла состояния
│       │   │   │
│       │   └── wizard_model.go  # Модель + константы
│       │       │   - DefaultOutboundTag                 # Дефолтный outbound
│       │       │   - RejectActionName                   # Действие reject
│       │       │
│       ├── presentation/       # Слой представления (MVP Presenter)
│       │   ├── presenter.go    # WizardPresenter
│       │   │   │   - WizardPresenter struct             # Презентер визарда
│       │   │   │   - NewWizardPresenter()               # Создание презентера
│       │   │   │   - SafeFyneDo()                       # Безопасный вызов Fyne из горутин
│       │   │   │   - SetCreateRulesTabFunc()            # Установка функции создания вкладки Rules (DI)
│       │   │   │   - createRulesTabFunc                 # Функция создания вкладки Rules (хранится для синхронизации)
│       │   │   │
│       │   ├── gui_state.go    # GUIState
│       │   │   │   - GUIState struct                    # Состояние GUI (только виджеты)
│       │   │   │   - RuleWidget struct                  # Виджет правила (Select, Checkbox, RuleState)
│       │   │   │
│       │   ├── presenter_methods.go # Методы управления UI
│       │   │   │   - SetSaveState()                     # Состояние кнопки Save
│       │   │   │   - RefreshOutboundOptions()           # Обновление опций outbound
│       │   │   │   - InitializeTemplateState()          # Инициализация шаблона
│       │   │   │   - SetTemplatePreviewText()           # Установка preview
│       │   │   │
│       │   ├── presenter_sync.go # Синхронизация модели и GUI
│       │   │   │   - SyncModelToGUI()                   # Синхронизация модели → GUI
│       │   │   │   - SyncGUIToModel()                   # Синхронизация GUI → модели
│       │   │   │
│       │   ├── presenter_async.go # Асинхронные операции
│       │   │   │   - TriggerParseForPreview()           # Парсинг для preview
│       │   │   │   - UpdateTemplatePreviewAsync()       # Обновление preview
│       │   │   │
│       │   ├── presenter_save.go # Сохранение конфигурации
│       │   │   │   - SaveConfig()                       # Сохранение конфигурации (основная функция)
│       │   │   │   - validateSaveInput()               # Валидация входных данных
│       │   │   │   - checkSaveOperationState()         # Проверка состояния операции
│       │   │   │   - executeSaveOperation()            # Выполнение операции сохранения
│       │   │   │   - finalizeSaveOperation()           # Завершение операции
│       │   │   │   - waitForParsingIfNeeded()          # Ожидание парсинга при необходимости
│       │   │   │   - buildConfigForSave()              # Построение конфигурации
│       │   │   │   - saveConfigFile()                  # Сохранение файла с бэкапом
│       │   │   │   - validateConfigFile()              # Валидация конфига через sing-box
│       │   │   │   - saveStateAndShowSuccessDialog()   # Сохранение state и показ диалога
│       │   │   │   - showSaveSuccessDialog()           # Диалог успешного сохранения
│       │   │   │   - completeSaveOperation()           # Завершение операции с задержкой
│       │   │   │
│       │   ├── presenter_state.go # Управление состояниями визарда
│       │   │   │   - CreateStateFromModel()             # Создание состояния из модели
│       │   │   │   - SaveCurrentState()                 # Сохранение текущего состояния
│       │   │   │   - SaveStateAs()                      # Сохранение состояния под ID
│       │   │   │   - LoadState()                       # Загрузка состояния в модель
│       │   │   │   - HasUnsavedChanges()                # Проверка несохранённых изменений
│       │   │   │   - MarkAsChanged()                    # Установка флага изменений
│       │   │   │   - MarkAsSaved()                      # Сброс флага изменений
│       │   │   │
│       │   ├── presenter_rules.go # Работа с правилами
│       │   │   │   - RefreshRulesTab()                  # Обновление таба правил
│       │   │   │   - RefreshRulesTabAfterLoadState()    # Пересоздание вкладки Rules после LoadState (в UI-потоке)
│       │   │   │   - OpenRuleDialogs()                  # Открытые диалоги
│       │   │   │
│       │   ├── presenter_ui_updater.go # Реализация UIUpdater
│       │   │   │   - UpdateParserConfig()               # Обновление ParserConfig
│       │   │   │   - UpdateTemplatePreview()            # Обновление preview
│       │   │   │   - UpdateSaveProgress()               # Прогресс сохранения
│       │       │
│       ├── tabs/               # UI компоненты вкладок
│       │   ├── source_tab.go   # Вкладка Sources & ParserConfig
│       │   │   │   - createSourceTab()                       # Создание вкладки Sources & ParserConfig
│       │   │   │
│       │   ├── rules_tab.go    # Вкладка правил
│       │   │   │   - CreateRulesTab()                        # Создание вкладки правил (основная функция)
│       │   │   │   - createSelectableRulesUI()               # UI для selectable rules из шаблона
│       │   │   │   - createCustomRulesUI()                  # UI для пользовательских правил
│       │   │   │   - createFinalOutboundSelect()           # Селектор финального outbound
│       │   │   │   - createOutboundSelectorForSelectableRule() # Селектор outbound для правила
│       │   │   │   - createSelectableRuleCheckbox()         # Checkbox для selectable rule
│       │   │   │   - createOutboundSelectorForCustomRule()  # Селектор outbound для custom rule
│       │   │   │   - createCustomRuleActionButtons()        # Кнопки редактирования/удаления
│       │   │   │   - deleteCustomRule()                     # Удаление пользовательского правила
│       │   │   │   - createAddRuleButton()                  # Кнопка добавления правила
│       │   │   │   - buildRulesTabContainer()               # Финальный контейнер таба
│       │   │   │   - CreateRulesScroll()                    # Создание прокручиваемого списка правил
│       │   │   │
│       │   └── preview_tab.go  # Вкладка превью
│       │       │   - createPreviewTab()                      # Создание вкладки превью
│       │       │
│       ├── dialogs/            # Диалоги визарда
│       │   ├── add_rule_dialog.go  # Диалог добавления правила
│       │   │   │   - ShowAddRuleDialog()                     # Показать диалог добавления правила
│       │   │   │
│       │   ├── load_state_dialog.go # Диалог загрузки состояния
│       │   │   │   - ShowLoadStateDialog()                   # Показать диалог загрузки состояния
│       │   │   │
│       │   ├── save_state_dialog.go # Диалог сохранения состояния
│       │   │   │   - ShowSaveStateDialog()                   # Показать диалог сохранения состояния
│       │   │   │
│       │   └── rule_dialog.go      # Утилиты для диалогов
│       │       │   - extractStringArray()                    # Извлечение массива строк
│       │       │   - parseLines()                            # Парсинг строк
│       │       │
│       ├── business/           # Бизнес-логика (без GUI зависимостей)
│       │   ├── parser.go       # Парсинг URL и конфигурации
│       │   │   │   - ParseAndPreview()                       # Парсинг и превью
│       │   │   │   - ApplyURLToParserConfig()                # Применение URL (основная функция)
│       │   │   │   - validateApplyURLInput()                # Валидация входных данных
│       │   │   │   - parseParserConfigForApply()            # Парсинг ParserConfig
│       │   │   │   - classifyInputLines()                   # Классификация строк на подписки/connections
│       │   │   │   - preserveExistingProperties()           # Сохранение существующих свойств
│       │   │   │   - createSubscriptionProxies()            # Создание ProxySource для подписок
│       │   │   │   - restoreTagPrefixAndPostfix()           # Восстановление тегов
│       │   │   │   - connectionsMatch()                    # Сравнение connections
│       │   │   │   - matchOrCreateConnectionProxy()          # Сопоставление или создание connection proxy
│       │   │   │   - updateAndSerializeParserConfig()       # Обновление и сериализация
│       │   │   │
│       │   ├── create_config.go  # Сборка конфигурации из шаблона
│       │   │   │   - BuildTemplateConfig()                   # Построение конфигурации
│       │   │   │   - BuildParserOutboundsBlock()             # Построение блока outbounds
│       │   │   │   - MergeRouteSection()                      # Объединение route секции
│       │   │   │
│       │   ├── formatting.go   # Форматирование и константы
│       │   │   │   - IndentBase const                         # Базовый отступ (2 пробела)
│       │   │   │   - Indent(level)                            # Генерация отступа для уровня
│       │   │   │
│       │   ├── validator.go    # Валидация данных
│       │   │   │   - ValidateParserConfig()                   # Валидация конфигурации
│       │   │   │   - ValidateURL()                             # Валидация URL
│       │   │   │   - ValidateURI()                             # Валидация URI
│       │   │   │   - ValidateJSONSize()                        # Валидация размера JSON
│       │   │   │
│       │   ├── loader.go       # Загрузка конфигурации
│       │   │   │   - LoadConfigFromFile()                      # Загрузка из файла
│       │   │   │   - EnsureRequiredOutbounds()                 # Обеспечение outbounds
│       │   │   │   - CloneOutbound()                           # Клонирование outbound
│       │   │   │
│       │   ├── saver.go        # Сохранение конфигурации
│       │   │   │   - SaveConfigWithBackup()                    # Сохранение с бэкапом
│       │   │   │   - ValidateConfigWithSingBox()              # Валидация через sing-box check
│       │   │   │   - NextBackupPath()                          # Путь для бэкапа
│       │   │   │   - FileServiceAdapter                        # Адаптер FileService
│       │   │   │
│       │   ├── outbound.go     # Работа с outbounds
│       │   │   │   - GetAvailableOutbounds()                   # Получение доступных outbounds
│       │   │   │   - EnsureDefaultAvailableOutbounds()         # Обеспечение дефолтных
│       │   │   │   - EnsureFinalSelected()                     # Обеспечение выбранного final
│       │   │   │
│       │   ├── state_store.go  # Управление состояниями визарда
│       │   │   │   - NewStateStore()                           # Создание StateStore
│       │   │   │   - SaveWizardState()                         # Сохранение состояния по ID
│       │   │   │   - SaveCurrentState()                        # Сохранение текущего состояния
│       │   │   │   - LoadWizardState()                         # Загрузка состояния по ID
│       │   │   │   - LoadCurrentState()                        # Загрузка текущего состояния
│       │   │   │   - ListWizardStates()                        # Список всех состояний
│       │   │   │   - ValidateStateID()                         # Валидация ID состояния
│       │   │   │   - StateStore struct                         # Хранилище состояний
│       │   │   │
│       │   ├── ui_updater.go   # Интерфейс UIUpdater
│       │   │   │   - UIUpdater interface                       # Интерфейс обновления GUI
│       │   │   │
│       │   ├── config_service.go # Адаптер ConfigService
│       │   │   │   - ConfigService interface                   # Интерфейс ConfigService
│       │   │   │   - ConfigServiceAdapter                      # Адаптер для core.ConfigService
│       │   │   │
│       │   └── template_loader.go # Адаптер TemplateLoader
│       │       │   - TemplateLoader interface                  # Интерфейс TemplateLoader
│       │       │   - DefaultTemplateLoader                     # Реализация по умолчанию
│       │       │
│       ├── template/            # Работа с шаблонами конфигурации
│       │   ├── loader.go        # Загрузка единого JSON-шаблона
│       │   │   │   - LoadTemplateData()                        # Загрузка и разбор шаблона
│       │   │   │   - GetTemplateFileName()                     # Имя файла шаблона (wizard_template.json)
│       │   │   │   - GetTemplateURL()                          # URL для загрузки шаблона
│       │   │   │   - TemplateData struct                       # Данные шаблона для визарда
│       │   │   │   - TemplateSelectableRule struct             # Правило маршрутизации из шаблона
│       │   │   │   - UnifiedTemplate struct                    # Структура JSON-шаблона
│       │   │   │   - UnifiedSelectableRule struct              # Правило в шаблоне
│       │   │   │   - UnifiedTemplateParam struct               # Платформенный параметр
│       │   │   │
│       │   └── rule_utils.go    # Утилиты для работы с правилами
│       │       │   - HasOutbound()                             # Проверка наличия outbound
│       │       │   - GetDefaultOutbound()                      # Извлечение outbound по умолчанию
│       │       │   - CloneRuleRaw()                            # Глубокое копирование правила
│       │       │
│       └── utils/              # Утилиты
│           ├── comparison.go    # Сравнение структур
│           │   │   - OutboundsMatchStrict()                    # Строгое сравнение outbounds
│           │   │   - StringSlicesEqual()                       # Сравнение слайсов строк
│           │   │   - MapsEqual()                                # Сравнение карт
│           │   │
│           └── constants.go    # Константы (таймауты, лимиты)
│               │   - MaxSubscriptionSize                       # Максимальный размер подписки
│               │   - MaxJSONConfigSize                          # Максимальный размер JSON
│               │   - MaxURILength                               # Максимальная длина URI
│               │   - HTTPRequestTimeout                         # Таймаут HTTP запроса
│               │
├── api/                        # API клиенты
│   └── clash.go                # Clash API клиент
│       │   - LoadClashAPIConfig()                              # Загрузка конфигурации API
│       │   - TestAPIConnection()                              # Тестирование соединения
│       │   - GetProxiesInGroup()                              # Получение прокси в группе
│       │   - SwitchProxy()                                    # Переключение прокси
│       │   - GetDelay()                                       # Получение задержки
│       │   - ProxyInfo struct                                 # Информация о прокси
│       │
├── internal/                   # Внутренние пакеты
│   ├── constants/              # Константы приложения
│   │   │   - ConfigFileName                    # Имя файла конфигурации
│   │   │   - различные константы приложения
│   │   │
│   ├── debuglog/               # Логирование с уровнями
│   │   │   - Log()                             # Основная функция логирования
│   │   │   - LogTextFragment()                 # Логирование больших текстов (с обрезкой)
│   │   │   - ShouldLog()                       # Проверка уровня логирования
│   │   │   - Level enum (Off/Error/Warn/Info/Verbose/Trace)
│   │   │
│   ├── dialogs/                # Диалоги (без зависимости от ui)
│   │   │   - NewCustom()                                # Кастомный диалог: mainContent (центр), buttons (низ), Border; ESC закрывает
│   │   │   - ShowDownloadFailedManual()                 # Единый диалог при ошибке загрузки (sing-box, wintun, шаблон, SRS): короткое сообщение, ссылка «Open download page» + кнопка копирования URL, «Open folder», «Close»
│   │   │   - ShowError() / ShowErrorText()              # Показать ошибку (используются из ui/dialogs)
│   │   │
│   └── platform/              # Платформо-зависимый код
│       │   - платформо-специфичные функции
│
└── assets/                     # Ресурсы (иконки)
```

## Детальное описание компонентов

### Core Layer (Ядро приложения)

#### AppController (`core/controller.go`)

Главный контроллер приложения, координирующий все компоненты.

**Ответственность:**
- Инициализация всех сервисов
- Координация взаимодействия между компонентами
- Управление жизненным циклом приложения
- Предоставление единого API для UI

**Сервисы:**
- `UIService` - управление UI состоянием
- `APIService` - взаимодействие с Clash API
- `StateService` - кеширование и состояние
- `FileService` - управление файлами
- `ProcessService` - управление процессом sing-box
- `ConfigService` - работа с конфигурацией

#### Services (`core/services/`)

**UIService** (`ui_service.go`)
- `NewUIService()` - создание сервиса
- `UpdateUI()` - обновление всех UI элементов
- `StopTrayMenuUpdateTimer()` - остановка таймера обновления меню
- `QuitApplication()` - выход из приложения
- `FocusOpenChildWindows` - callback для переноса фокуса на одно из дочерних окон визарда (View, Outbound Edit, rule dialog) при клике по окну визарда; устанавливается в `wizard.go`, вызывается из `ui/components/click_redirect.go`
- Структуры: `UIService` с полями для Fyne компонентов и callbacks
- Тултипы: см. раздел «Используемые библиотеки» (fyne-tooltip).

**APIService** (`api_service.go`)
- `NewAPIService()` - создание сервиса
- `GetClashAPIConfig()` - получение конфигурации API
- `GetProxiesList()` - получение списка прокси
- `SetProxiesList()` - установка списка прокси
- `GetActiveProxyName()` - получение активного прокси
- `SetActiveProxyName()` - установка активного прокси
- `SwitchProxy()` - переключение прокси
- `AutoLoadProxies()` - автозагрузка прокси
- `SetLastPingError()` / `GetLastPingError()` - хранение текста последней ошибки Ping для прокси (показывается в tooltip кнопки Ping)

**StateService** (`state_service.go`)
- `NewStateService()` - создание сервиса
- `GetCachedVersion()` - получение кешированной версии
- `SetCachedVersion()` - установка кешированной версии
- `IsAutoUpdateEnabled()` - проверка автообновления
- `SetAutoUpdateEnabled()` - установка автообновления
- `GetLastUpdatedTime()` - получение времени последнего обновления
- `SetLastUpdatedTime()` - установка времени обновления

**FileService** (`file_service.go`)
- `NewFileService()` - создание сервиса
- `OpenLogFiles()` - открытие лог-файлов
- `CloseLogFiles()` - закрытие лог-файлов
- `GetMainLogFile()` - получение основного лог-файла
- `GetChildLogFile()` - получение лог-файла дочернего процесса
- `GetApiLogFile()` - получение лог-файла API
- Поля: `ExecDir`, `ConfigPath`, `SingboxPath`, `WintunPath`

#### Config (`core/config/`)

**models.go**
- `ParserConfig` struct - конфигурация парсера
- `ProxySource` struct - источник прокси
- `OutboundConfig` struct - конфигурация исходящего соединения
- `WizardConfig` struct - настройки визарда
- `ParserConfigVersion` type - версия конфигурации
- `SubscriptionUserAgent` const - User-Agent для подписок
- Методы: `IsWizardHidden()`, `GetWizardRequired()`

**config_loader.go**
- `GetSelectorGroupsFromConfig()` - получение групп селекторов из config.json
- `GetTunInterfaceName()` - получение имени TUN интерфейса
- `readConfigFile()` - чтение и очистка JSONC файла
- `cleanJSONC()` - очистка JSONC от комментариев

**outbound_generator.go**
- `GenerateNodeJSON()` - генерация JSON узла из ParsedNode (vless, vmess, trojan, shadowsocks, hysteria2)
- `GenerateSelectorWithFilteredAddOutbounds()` - генерация селектора с фильтрацией addOutbounds
- `GenerateOutboundsFromParserConfig()` - генерация outbounds из конфигурации (трехпроходный алгоритм)
  - Pass 1: Создание outboundsInfo и подсчет узлов
  - Pass 2: Топологическая сортировка зависимостей и расчет валидности
  - Pass 3: Генерация JSON только для валидных селекторов
- `OutboundGenerationResult` struct - результат генерации (статистика и JSON строки)
- `outboundInfo` struct - информация о динамическом селекторе (для трехпроходного алгоритма)
- `filterNodesForSelector()` - фильтрация узлов для селектора
- `matchesFilter()`, `getNodeValue()`, `matchesPattern()` - вспомогательные функции фильтрации

**updater.go**
- `UpdateConfigFromSubscriptions()` - обновление config.json из подписок
- `writeToConfig()` - запись конфигурации в файл

**parser/** - Работа с ParserConfig блоком
- `factory.go`:
  - `ExtractParserConfig()` - извлечение ParserConfig из config.json
  - `NormalizeParserConfig()` - нормализация конфигурации
  - `LogDuplicateTagStatistics()` - логирование статистики дубликатов
- `migrator.go`:
  - Миграция версий ParserConfig (v1 → v2 → v3 → v4)
- `block_extractor.go`:
  - `ExtractParserConfigBlock()` - извлечение блока из JSON

**subscription/** - Работа с подписками
- `source_loader.go`:
  - `LoadNodesFromSource()` - загрузка узлов из источника
  - `applyTagPrefixPostfix()` - применение префикса/постфикса к тегам
  - `replaceTagVariables()` - замена переменных в тегах
  - `MakeTagUnique()` - обеспечение уникальности тегов
  - `IsSubscriptionURL()` - проверка URL подписки
  - `MaxNodesPerSubscription` const - лимит узлов
- `node_parser.go`:
  - `ParseNode()` - парсинг URI узла прокси
  - `IsDirectLink()` - проверка прямого линка
- `decoder.go`:
  - `DecodeSubscriptionContent()` - декодирование подписки (base64, yaml)
- `fetcher.go`:
  - `FetchSubscription()` - загрузка подписки по HTTP

#### ProcessService (`core/process_service.go`)

**Основные функции:**
- `NewProcessService()` - создание сервиса
- `Start()` - запуск процесса sing-box
- `Stop()` - остановка процесса sing-box
- `Monitor()` - мониторинг процесса
- `CheckIfRunningAtStart()` - проверка запущенного процесса при старте

**Вспомогательные функции:**
- `checkAndShowSingBoxRunningWarning()` - проверка и предупреждение о запущенном процессе
- `isSingBoxProcessRunning()` - проверка запущенного процесса
- `isSingBoxProcessRunningWithPS()` - проверка через ps библиотеку
- `checkTunInterfaceExists()` - проверка существования TUN интерфейса
- `removeTunInterface()` - удаление TUN интерфейса

#### ConfigService (`core/config_service.go`)

**Основные функции:**
- `NewConfigService()` - создание сервиса
- `RunParserProcess()` - запуск процесса парсинга конфигурации
- `UpdateConfigFromSubscriptions()` - обновление конфигурации из подписок

**Примечание:** Генерация outbounds выполняется функциями из пакета `core/config/outbound_generator.go`:
- `GenerateOutboundsFromParserConfig()` - оркестрация (проходы: buildOutboundsInfo, computeOutboundValidity, generateSelectorJSONs)
- `GenerateSelectorWithFilteredAddOutbounds()` - генерация селектора с фильтрацией addOutbounds
- `GenerateNodeJSON()` - генерация JSON узла

### UI Layer (Пользовательский интерфейс)

#### Основные компоненты

**app.go**
- `NewApp()` - создание главного окна приложения
- `GetTabs()` - получение контейнера вкладок
- `GetWindow()` - получение главного окна
- `GetController()` - получение контроллера
- `updateClashAPITabState()` - обновление состояния вкладки Clash API

**core_dashboard_tab.go**
- `CreateCoreDashboardTab()` - создание вкладки Core Dashboard
- `updateBinaryStatus()` - проверка наличия бинарника sing-box
- `updateRunningStatus()` - обновление статуса запуска
- `updateVersionInfo()` - обновление информации о версии
- `updateWintunStatus()` - обновление статуса wintun.dll
- `updateConfigInfo()` - обновление информации о конфигурации
- `handleDownload()` - обработка загрузки sing-box
- `handleWintunDownload()` - обработка загрузки wintun.dll

**clash_api_tab.go**
- `CreateClashAPITab()` - создание вкладки Clash API
- `onLoadAndRefreshProxies()` - загрузка и обновление прокси
- `onTestAPIConnection()` - тестирование соединения с API
- `onResetAPIState()` - сброс состояния API
- `pingProxy()` - пинг прокси

#### Wizard (`ui/wizard/`)

Визард следует архитектуре MVP (Model-View-Presenter) с четким разделением ответственности:
- **Model** (`models/`) - чистые бизнес-данные без GUI зависимостей
- **View** (`tabs/`, `dialogs/`, `GUIState`) - только GUI виджеты и их компоновка
- **Presenter** (`presentation/`) - связывает модель и представление, координирует бизнес-логику

**wizard.go**
- `ShowConfigWizard()` - точка входа, создание окна визарда
- Создание модели (`WizardModel`), GUI-состояния (`GUIState`) и презентера (`WizardPresenter`)
- Инициализация табов и координация шагов
- Настройка обработчиков событий и навигация

**models/** - Модели данных (без GUI зависимостей)
- `wizard_model.go`:
  - `WizardModel` struct - модель данных визарда (ParserConfig, SourceURLs, GeneratedOutbounds, TemplateData, Rules и т.д.)
  - `NewWizardModel()` - создание новой модели
- `rule_state.go`:
  - `RuleState` struct - состояние правила маршрутизации (Rule, Enabled, SelectedOutbound)
- `rule_state_utils.go`:
  - `GetEffectiveOutbound()` - получение эффективного outbound для правила
  - `EnsureDefaultOutbound()` - установка дефолтного outbound
- `wizard_state_file.go`:
  - `WizardStateFile` struct - сериализуемое состояние визарда (version 2, метаданные, ParserConfig, ConfigParams, правила)
  - `PersistedSelectableRuleState` struct - упрощённое состояние правила из шаблона (только label, enabled, selected_outbound)
  - `PersistedCustomRule` struct - полное определение пользовательского правила (label, type, rule, enabled и т.д.)
  - `WizardStateMetadata` struct - метаданные состояния для списка
  - `ValidateStateID()` - валидация ID состояния
  - `MigrateSelectableRuleStates()` - миграция selectable_rule_states из формата v1 (вложенный rule) в v2 (плоский)
  - `MigrateCustomRules()` - миграция custom_rules из формата v1 (rule.raw) в v2 (rule на верхнем уровне)
  - `StateFileName` const - имя файла текущего состояния
- `wizard_model.go`:
  - `WizardModel` - основная модель данных
  - `DefaultOutboundTag`, `RejectActionName`, `RejectActionMethod` - константы для правил

**presentation/** - Слой представления (MVP Presenter)
- `presenter.go`:
  - `WizardPresenter` struct - презентер, связывающий модель, GUI и бизнес-логику
  - `NewWizardPresenter()` - создание презентера
  - Методы доступа: `Model()`, `GUIState()`, `ConfigServiceAdapter()`, `Controller()`
- `gui_state.go`:
  - `GUIState` struct - состояние GUI (только Fyne виджеты: Entry, Label, Button, Select и т.д.)
  - `ChildWindowsOverlay` - полупрозрачный слой поверх контента визарда при открытых дочерних окнах (Rule, View, Outbound Edit); показ/скрытие через `UpdateChildOverlay()`
  - `RuleWidget` struct - связь между виджетом Select, Checkbox и правилом из модели (для обновления UI после LoadState)
- `presenter_methods.go`:
  - `SetSaveState()` - управление состоянием кнопки Save и прогресс-бара
  - `RefreshOutboundOptions()` - обновление опций outbound для правил
  - `InitializeTemplateState()` - инициализация состояния шаблона
  - `SetTemplatePreviewText()` - установка текста preview
- `presenter_sync.go`:
  - `SyncModelToGUI()` - синхронизация данных из модели в GUI (обновляет текстовые поля, селекторы, пересоздаёт вкладку Rules при необходимости)
  - `SyncGUIToModel()` - синхронизация данных из GUI в модель
- `presenter_async.go`:
  - `TriggerParseForPreview()` - запуск парсинга конфигурации для preview асинхронно
  - `UpdateTemplatePreviewAsync()` - обновление preview шаблона асинхронно
- `presenter_save.go`:
  - `SaveConfig()` - сохранение конфигурации с прогресс-баром и проверками (основная функция)
  - `validateSaveInput()`, `checkSaveOperationState()` - проверки перед сохранением
  - `executeSaveOperation()` - выполнение сохранения в горутине: сборка конфига из текущей модели (без ожидания парсинга outbounds), запись файла, валидация, state.json, диалог; по завершении в фоне вызывается `core.RunParserProcess()` (обновление конфига из подписок)
  - `finalizeSaveOperation()` - завершение операции и восстановление UI
  - `buildConfigForSave()` - построение конфигурации из шаблона и модели
  - `saveConfigFile()`, `validateConfigFile()` - запись в файл и валидация sing-box
  - `saveStateAndShowSuccessDialog()`, `showSaveSuccessDialog()` - сохранение state и диалог успеха
  - `completeSaveOperation()` - финализация и запуск RunParserProcess в фоне
- `presenter_state.go`:
  - `CreateStateFromModel()` - создание WizardStateFile из текущей модели
  - `SaveCurrentState()` - сохранение текущего состояния в state.json
  - `SaveStateAs()` - сохранение состояния под новым ID
  - `LoadState()` - загрузка состояния из файла в модель
  - `HasUnsavedChanges()` - проверка наличия несохранённых изменений
  - `MarkAsChanged()` - установка флага изменений
  - `MarkAsSaved()` - сброс флага изменений
- `presenter_rules.go`:
  - `RefreshRulesTab()` - обновление содержимого таба Rules (принимает функцию создания вкладки)
  - `RefreshRulesTabAfterLoadState()` - пересоздание вкладки Rules после LoadState (использует сохранённую функцию через DI)
  - `OpenRuleDialogs()` - возврат карты открытых диалогов правил
- `presenter_ui_updater.go`:
  - Реализация интерфейса `UIUpdater` для обновления GUI из бизнес-логики
  - Методы: `UpdateParserConfig()`, `UpdateTemplatePreview()`, `UpdateSaveProgress()`, `UpdateSaveStatusText()` (статус слева от Prev при Save), `UpdateSaveButtonText()`
- `presenter.go`:
  - `WizardPresenter` struct - структура презентера
  - `NewWizardPresenter()` - создание презентера
  - `SetCreateRulesTabFunc()` - установка функции создания вкладки Rules через DI (для пересоздания после LoadState)
  - `SafeFyneDo()` - безопасный вызов Fyne функций из других горутин (утилита для всех методов презентера)
  - Дочерние окна: `SetViewWindow`/`ClearViewWindow`, `SetOutboundEditWindow`/`ClearOutboundEditWindow`, `UpdateChildOverlay()` — контракт и порядок фокуса см. **docs/WIZARD_CHILD_WINDOWS.md**

**tabs/** - UI вкладок
- `source_tab.go`:
  - `createSourceTab()` - создание вкладки Sources & ParserConfig
  - UI компоненты первой вкладки (URL поля, кнопки)
- `rules_tab.go`:
  - `createTemplateTab()` - создание вкладки правил
  - `createRulesScroll()` - создание прокручиваемого списка правил
  - UI компоненты вкладки правил
- `preview_tab.go`:
  - `createPreviewTab()` - создание вкладки превью
  - UI компоненты вкладки превью конфигурации

**dialogs/** - Диалоги
- `add_rule_dialog.go`:
  - `ShowAddRuleDialog()` - диалог добавления правила
- `load_state_dialog.go`:
  - `ShowLoadStateDialog()` - диалог загрузки состояния визарда
  - Отображение списка сохранённых состояний с метаданными
  - Загрузка выбранного состояния через презентер
- `save_state_dialog.go`:
  - `ShowSaveStateDialog()` - диалог сохранения состояния визарда
  - Ввод ID и комментария для нового состояния
  - Сохранение состояния через презентер
- `get_free_dialog.go`:
  - `ShowGetFreeVPNDialog()` - диалог загрузки конфигурации из get_free.json
  - `downloadGetFreeJSON()` - скачивание get_free.json с GitHub
  - `loadGetFreeJSON()` - загрузка и парсинг get_free.json
  - `convertGetFreeDataToStateFile()` - преобразование в WizardStateFile
  - Работа с упрощенным форматом: parser_config, selectable_rules (без дефолтов)
  - Использует фабрику `wizardmodels.NewWizardStateFile()` для инкапсуляции логики
  - Применяет конфигурацию через `presenter.LoadState()` (та же логика, что и для state.json)
- `rule_dialog.go`:
  - `extractStringArray()` - извлечение массива строк
  - `parseLines()` - парсинг строк

**business/** - Бизнес-логика (без GUI зависимостей)
- `parser.go`:
  - `ParseAndPreview()` - парсинг ParserConfig и генерация outbounds через ConfigService
  - `ApplyURLToParserConfig()` - применение URL к ParserConfig (основная функция)
    - `validateApplyURLInput()` - проверка входных данных перед применением URL
    - `parseParserConfigForApply()` - парсинг ParserConfig из JSON строки
    - `classifyInputLines()` - классификация входных строк на подписки и прямые ссылки
    - `preserveExistingProperties()` - сохранение существующих свойств из текущего ParserConfig
    - `createSubscriptionProxies()` - создание ProxySource для каждой подписки
    - `restoreTagPrefixAndPostfix()` - восстановление tag_prefix и tag_postfix из сохраненных свойств
    - `connectionsMatch()` - проверка совпадения двух массивов connections (порядок не важен)
    - `matchOrCreateConnectionProxy()` - сопоставление connections с существующим ProxySource или создание нового
    - `updateAndSerializeParserConfig()` - обновление ParserConfig и сериализация его
  - Все функции работают с `WizardModel` и используют `UIUpdater` для обновления GUI
- `create_config.go`:
  - `BuildTemplateConfig()` - построение финальной конфигурации из шаблона и модели
  - `BuildParserOutboundsBlock()` - формирование блока outbounds из сгенерированных outbounds
  - `MergeRouteSection()` - объединение правил маршрутизации из шаблона и пользовательских правил
  - `FormatSectionJSON()`, `IndentMultiline()` - вспомогательные функции форматирования JSON
- `validator.go`:
  - `ValidateParserConfig()` - валидация структуры и содержимого ParserConfig
  - `ValidateURL()` - валидация URL подписок (формат, схема, хост)
  - `ValidateURI()` - валидация URI для прямых ссылок (vless://, vmess:// и т.д.)
  - `ValidateOutbound()`, `ValidateRule()` - валидация outbound и правил
  - `ValidateJSON()`, `ValidateJSONSize()`, `ValidateHTTPResponseSize()` - валидация JSON и размеров
- `loader.go`:
  - `LoadConfigFromFile()` - загрузка ParserConfig из config.json (приоритет) или template (fallback)
  - `EnsureRequiredOutbounds()` - обеспечение наличия требуемых outbounds из template
  - `CloneOutbound()` - создание глубокой копии OutboundConfig
- `saver.go`:
  - `SaveConfigWithBackup()` - сохранение конфигурации с бэкапом (через services.BackupFile) и генерацией secret для Clash API
  - `FileServiceAdapter` - адаптер для services.FileService
- `state_store.go`:
  - `NewStateStore()` - создание хранилища состояний
  - `SaveWizardState()` - сохранение состояния по ID в файл
  - `SaveCurrentState()` - сохранение текущего состояния в state.json
  - `LoadWizardState()` - загрузка состояния по ID из файла
  - `LoadCurrentState()` - загрузка текущего состояния из state.json
  - `ListWizardStates()` - получение списка всех сохранённых состояний
  - `ValidateStateID()` - валидация ID состояния
  - `StateStore` struct - хранилище состояний визарда
  - Состояния хранятся в `<execDir>/bin/wizard_states/`
- `outbound.go`:
  - `GetAvailableOutbounds()` - получение списка доступных outbound тегов из модели
  - `EnsureDefaultAvailableOutbounds()` - обеспечение наличия обязательных outbounds (direct-out, reject, drop)
  - `EnsureFinalSelected()` - обеспечение выбранного final outbound в модели
- `ui_updater.go`:
  - `UIUpdater` interface - интерфейс для обновления GUI из бизнес-логики (реализуется в презентере)
- `config_service.go`:
  - `ConfigService` interface - интерфейс для генерации outbounds из ParserConfig
  - `ConfigServiceAdapter` - адаптер для core.ConfigService
- `template_loader.go`:
  - `TemplateLoader` interface - интерфейс для загрузки TemplateData
  - `DefaultTemplateLoader` - реализация по умолчанию

**template/** - Работа с единым шаблоном конфигурации
- `loader.go`:
  - `LoadTemplateData()` - загрузка единого JSON-шаблона (`wizard_template.json`), парсинг секций, применение `params` по текущей платформе, фильтрация `selectable_rules` по `platforms`
  - `GetTemplateFileName()` - возврат имени файла шаблона (`wizard_template.json`, единый для всех платформ)
  - `GetTemplateURL()` - возврат URL для загрузки шаблона с GitHub
  - `UnifiedTemplate` struct - структура JSON-шаблона (`parser_config`, `config`, `selectable_rules`, `params`)
  - `UnifiedSelectableRule` struct - правило в шаблоне (label, description, default, platforms, rule_set, rule/rules)
  - `UnifiedTemplateParam` struct - платформенный параметр (name, platforms, mode, value)
  - `TemplateData` struct - данные шаблона, подготовленные для визарда (ParserConfig, Config, ConfigOrder, SelectableRules, DefaultFinal)
  - `TemplateSelectableRule` struct - правило маршрутизации для визарда (Label, Description, IsDefault, Platforms, RuleSets, Rule/Rules)
- `rule_utils.go`:
  - `HasOutbound()` - проверка наличия поля outbound в правиле
  - `GetDefaultOutbound()` - извлечение outbound по умолчанию из правила
  - `CloneRuleRaw()` - глубокое копирование правила (map[string]interface{})

**utils/** - Утилиты
- `comparison.go`:
  - `OutboundsMatchStrict()` - строгое сравнение outbounds
  - `StringSlicesEqual()` - сравнение слайсов строк
  - `MapsEqual()` - сравнение карт
  - `ValuesEqual()` - сравнение значений
- `constants.go`:
  - Константы таймаутов: `HTTPRequestTimeout`, `SubscriptionFetchTimeout`, `URIParseTimeout`
  - Константы лимитов: `MaxSubscriptionSize`, `MaxJSONConfigSize`, `MaxURILength`, `MinURILength`
  - Константы UI: `MaxWaitTime`

## Ключевые точки входа

### Точки входа приложения

```
┌──────────────────────────────────────────────────────────────┐
│                    ТОЧКИ ВХОДА                               │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  1. main() [main.go]                                         │
│     └─> Создание AppController                               │
│     └─> Инициализация UI                                     │
│     └─> Запуск приложения                                    │
│                                                              │
│  2. core.NewAppController() [core/controller.go]             │
│     └─> Инициализация всех сервисов                          │
│     └─> Настройка callbacks                                  │
│     └─> Запуск фоновых процессов                             │
│                                                              │
│  3. wizard.ShowConfigWizard() [ui/wizard/wizard.go]          │
│     └─> Создание окна визарда                                │
│     └─> Инициализация вкладок                                │
│     └─> Координация шагов                                    │
│                                                              │
│  4. ConfigService.RunParserProcess() [core/config_service.go]│
│     └─> Запуск процесса парсинга                             │
│     └─> Обновление конфигурации                              │
│                                                              │
│  5. ProcessService.Start() [core/process_service.go]         │
│     └─> Запуск sing-box процесса                             │
│     └─> Мониторинг процесса                                  │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

### Пользовательские точки входа (UI)

```
┌─────────────────────────────────────────────────────────────┐
│              ПОЛЬЗОВАТЕЛЬСКИЕ ТОЧКИ ВХОДА                   │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Core Dashboard Tab:                                        │
│    • Start/Stop VPN                                         │
│    • Download sing-box                                      │
│    • Download wintun.dll                                    │
│    • Open Config Wizard                                     │
│    • Update Config                                          │
│                                                             │
│  Clash API Tab:                                             │
│    • Load Proxies                                           │
│    • Switch Proxy                                           │
│    • Test Connection                                        │
│    • Ping Proxy (single & mass ping with tooltips for errors)│
│                                                             │
│  Config Wizard:                                             │
│    • Add Source                                             │
│    • Add/Edit Rules                                         │
│    • Preview Config                                         │
│    • Save Config                                            │
│                                                             │
│  System Tray:                                               │
│    • Show/Hide Window                                       │
│    • Start/Stop VPN                                         │
│    • Switch Proxy                                           │
│    • Quit                                                   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Зоны ответственности

### Карта ответственности компонентов

```
┌─────────────────────────────────────────────────────────────┐
│                    ЗОНЫ ОТВЕТСТВЕННОСТИ                     │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  AppController [core/controller.go]                  │   │
│  │  • Координация всех компонентов                      │   │
│  │  • Управление жизненным циклом                       │   │
│  │  • Предоставление единого API                        │   │
│  │  • Управление RunningState                           │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Services [core/services/]                           │   │
│  │                                                      │   │
│  │  UIService:                                          │   │
│  │  • Fyne приложение и окна                            │   │
│  │  • Системный трей и меню                             │   │
│  │  • Callbacks для обновления UI                       │   │
│  │  • Иконки приложения                                 │   │
│  │                                                      │   │
│  │  APIService:                                         │   │
│  │  • Взаимодействие с Clash API                        │   │
│  │  • Управление списком прокси                         │   │
│  │  • Переключение прокси                               │   │
│  │  • Автозагрузка прокси                               │   │
│  │                                                      │   │
│  │  StateService:                                       │   │
│  │  • Кеширование версий                                │   │
│  │  • Состояние автообновления                          │   │
│  │  • Временные метки                                   │   │
│  │                                                      │   │
│  │  FileService:                                        │   │
│  │  • Управление путями к файлам                        │   │
│  │  • Открытие/закрытие лог-файлов                      │   │
│  │  • Ротация логов (макс 1 старый файл)                │   │
│  │  • Бэкап файлов (BackupFile, BackupPath)             │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  ProcessService [core/process_service.go]            │   │
│  │  • Запуск sing-box процесса                          │   │
│  │  • Остановка процесса                                │   │
│  │  • Мониторинг процесса                               │   │
│  │  • Автоперезапуск при сбоях                          │   │
│  │  • Управление TUN интерфейсом                        │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  ConfigService [core/config_service.go]              │   │
│  │  • Запуск процесса парсинга                          │   │
│  │  • Обновление прогресса                              │   │
│  │  • Обработка ошибок парсинга                         │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Config Package [core/config/]                       │   │
│  │                                                      │   │
│  │  models.go:                                          │   │
│  │  • Модели данных конфигурации                        │   │
│  │  • Типы: ParserConfig, ProxySource, OutboundConfig   │   │
│  │                                                      │   │
│  │  config_loader.go:                                   │   │
│  │  • Чтение config.json                                │   │
│  │  • Извлечение селекторов                             │   │
│  │  • Получение TUN интерфейса                          │   │
│  │                                                      │   │
│  │  outbound_generator.go:                              │   │
│  │  • Генерация JSON узлов                              │   │
│  │  • Генерация селекторов (с фильтрацией addOutbounds) │   │
│  │  • Генерация outbounds (трехпроходный алгоритм)      │   │
│  │  • Топологическая сортировка зависимостей            │   │
│  │                                                      │   │
│  │  updater.go:                                         │   │
│  │  • Обновление config.json из подписок                │   │
│  │  • Запись конфигурации                               │   │
│  │                                                      │   │
│  │  parser/:                                            │   │
│  │  • Извлечение ParserConfig блока из config.json      │   │
│  │  • Нормализация конфигурации                         │   │
│  │  • Миграция версий (v1 → v4)                         │   │
│  │                                                      │   │
│  │  subscription/:                                      │   │
│  │  • Загрузка подписок по HTTP                         │   │
│  │  • Декодирование (base64, yaml)                      │   │
│  │  • Парсинг URI узлов                                 │   │
│  │  • Загрузка узлов из источников                      │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  UI Package [ui/]                                    │   │
│  │                                                      │   │
│  │  app.go:                                             │   │
│  │  • Создание главного окна                            │   │
│  │  • Управление вкладками                              │   │
│  │                                                      │   │
│  │  core_dashboard_tab.go:                              │   │
│  │  • Управление sing-box (старт/стоп)                  │   │
│  │  • Загрузка компонентов                              │   │
│  │  • Статус конфигурации                               │   │
│  │                                                      │   │
│  │  clash_api_tab.go:                                   │   │
│  │  • Отображение прокси                                │   │
│  │  • Переключение прокси                               │   │
│  │  • Тестирование соединения                           │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Wizard Package [ui/wizard/] (MVP Architecture)      │   │
│  │                                                      │   │
│  │  wizard.go:                                          │   │
│  │  • Координация шагов визарда                         │   │
│  │  • Создание Model, GUIState и Presenter              │   │
│  │  • Инициализация табов                               │   │
│  │                                                      │   │
│  │  models/:                                            │   │
│  │  • WizardModel - чистые бизнес-данные                │   │
│  │  • RuleState - состояние правил маршрутизации        │   │
│  │  • WizardStateFile - сериализуемое состояние визарда │   │
│  │  • Константы для правил и outbounds                  │   │
│  │                                                      │   │
│  │  presentation/:                                      │   │
│  │  • WizardPresenter - связывает модель и GUI          │   │
│  │  • GUIState - только Fyne виджеты                    │   │
│  │  • Синхронизация данных (Model ↔ GUI)                │   │
│  │  • Асинхронные операции (парсинг, preview)           │   │
│  │  • Сохранение конфигурации                           │   │
│  │  • Управление состояниями (сохранение/загрузка)      │   │
│  │  • Отслеживание несохранённых изменений              │   │
│  │  • Реализация UIUpdater для бизнес-логики            │   │
│  │                                                      │   │
│  │  business/:                                          │   │
│  │  • Парсинг URL и конфигурации (parser.go)            │   │
│  │  • Сборка конфигурации из шаблона (create_config.go) │   │
│  │  • Валидация данных (validator.go)                   │   │
│  │  • Загрузка конфигурации (loader.go)                 │   │
│  │  • Сохранение конфигурации (saver.go)                │   │
│  │  • Работа с outbounds (outbound.go)                  │   │
│  │  • Управление состояниями (state_store.go)           │   │
│  │  • Интерфейсы: UIUpdater, ConfigService, TemplateLoader│ │
│  │                                                      │   │
│  │  tabs/:                                              │   │
│  │  • UI компоненты вкладок (Source, Rules, Preview)    │   │
│  │  • Все взаимодействие через Presenter                │   │
│  │                                                      │   │
│  │  dialogs/:                                           │   │
│  │  • Диалоги визарда (добавление/редактирование правил)│   │
│  │  • Диалоги сохранения/загрузки состояний             │   │
│  │  • Взаимодействие через Presenter                    │   │
│  │                                                      │   │
│  │  template/:                                          │   │
│  │  • Загрузка единого JSON-шаблона (wizard_template.json) │  │
│  │  • Парсинг секций: parser_config, config, selectable_rules, params │
│  │  • Применение params по платформе (runtime.GOOS)     │   │
│  │  • Фильтрация selectable_rules по platforms          │   │
│  │                                                      │   │
│  │  utils/:                                             │   │
│  │  • Утилиты и константы (сравнение, лимиты, таймауты) │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Взаимодействие компонентов

### Поток инициализации

```
main.go
  └─> core.NewAppController()
      ├─> services.NewFileService()
      ├─> services.NewUIService()
      ├─> services.NewAPIService()
      ├─> services.NewStateService()
      ├─> NewProcessService()
      └─> NewConfigService()
```

### Поток обновления конфигурации

```
UI (core_dashboard_tab.go)
  └─> ConfigService.RunParserProcess()
      └─> config/updater.go: UpdateConfigFromSubscriptions()
          ├─> subscription/fetcher.go: FetchSubscription()
          ├─> subscription/decoder.go: DecodeSubscriptionContent()
          ├─> subscription/node_parser.go: ParseNode()
          └─> config/outbound_generator.go: GenerateOutboundsFromParserConfig()
```

### Поток работы визарда

```
UI (core_dashboard_tab.go)
  └─> wizard.ShowConfigWizard()
      ├─> wizard/models: NewWizardModel()
      ├─> wizard/presentation: NewGUIState(), NewWizardPresenter()
      ├─> wizard/template/loader.go: LoadTemplateData()  # единый JSON-шаблон
      ├─> wizard/tabs/source_tab.go: CreateSourceTab(presenter)
      ├─> wizard/tabs/rules_tab.go: CreateRulesTab(presenter)
      ├─> wizard/tabs/preview_tab.go: CreatePreviewTab(presenter)
      │
      ├─> wizard/business/loader.go: LoadConfigFromFile()
      ├─> wizard/presentation/presenter_state.go: LoadState()
      │   ├─> wizard/business/state_store.go: LoadCurrentState()
      │   └─> wizard/presentation/presenter_sync.go: SyncModelToGUI()
      │       └─> wizard/presentation/presenter_rules.go: RefreshRulesTabAfterLoadState() - пересоздание вкладки Rules (DI)
      ├─> wizard/dialogs/get_free_dialog.go: ShowGetFreeVPNDialog()
      │   ├─> downloadGetFreeJSON() - скачивание get_free.json с GitHub
      │   ├─> loadGetFreeJSON() - загрузка и парсинг get_free.json
      │   ├─> convertGetFreeDataToStateFile() - преобразование в WizardStateFile
      │   │   └─> wizard/models/wizard_state_file.go: NewWizardStateFile() - фабрика
      │   └─> presenter.LoadState() - применение конфигурации (та же логика, что и для state.json)
      │       └─> SyncModelToGUI() → RefreshRulesTabAfterLoadState() - обновление UI после загрузки
      ├─> wizard/presentation/presenter_async.go: TriggerParseForPreview()
      │   └─> wizard/business/parser.go: ParseAndPreview()
      ├─> wizard/presentation/presenter_async.go: UpdateTemplatePreviewAsync()
      │   └─> wizard/business/create_config.go: BuildTemplateConfig()
      ├─> wizard/presentation/presenter_save.go: SaveConfig()
      │   ├─> validateSaveInput() / checkSaveOperationState()
      │   ├─> executeSaveOperation()
      │   │   ├─> waitForParsingIfNeeded()
      │   │   ├─> buildConfigForSave()
      │   │   │   └─> wizard/business/create_config.go: BuildTemplateConfig()
      │   │   ├─> saveConfigFile()
      │   │   │   └─> wizard/business/saver.go: SaveConfigWithBackup()
      │   │   ├─> validateConfigFile()
      │   │   │   └─> wizard/business/saver.go: ValidateConfigWithSingBox()
      │   │   └─> saveStateAndShowSuccessDialog()
      │   │       ├─> wizard/presentation/presenter_state.go: SaveCurrentState()
      │   │       │   └─> wizard/business/state_store.go: SaveCurrentState()
      │   │       └─> showSaveSuccessDialog()
```

## Принципы организации кода

### 1. Именование

- **Пакеты**: строчные, без подчеркиваний (`config`, `wizard`, `services`)
- **Файлы**: snake_case для многословных имен (`config_loader.go`, `add_rule_dialog.go`)
- **Типы**: PascalCase (`ParserConfig`, `WizardState`)
- **Функции**: PascalCase для экспортируемых, camelCase для приватных
- **Константы**: PascalCase (`MaxSubscriptionSize`, `HTTPRequestTimeout`)

### 2. Структура файлов

- Один файл = одна ответственность
- Связанные функции группируются в пакеты
- Подпакеты для логической группировки
- Тесты рядом с кодом (`*_test.go`)

### 3. Обработка ошибок

- Все ошибки оборачиваются с контекстом: `fmt.Errorf("function: operation failed: %w", err)`
- Префикс функции в сообщении об ошибке для трассировки
- Использование `errors.Is()` и `errors.As()` для проверки типов ошибок

### 4. Ресурсы

- Все файлы закрываются через `defer Close()`
- HTTP ответы закрываются через `defer resp.Body.Close()`
- Использование `context.WithTimeout()` для долгих операций

### 5. Валидация

- Валидация размеров HTTP ответов
- Валидация размеров JSON конфигурации
- Валидация длины URI
- Лимиты определены в константах

### 6. Комментарии

- Комментарии на русском языке (Go-style, объясняют "зачем", а не "что")
- Документация для экспортируемых функций
- Описание сложной логики
- Self-documenting code предпочтительнее комментариев

## Зависимости между пакетами

```
main.go
  └─> core
      ├─> core/services
      ├─> core/config
      │   └─> core/config/subscription
      └─> ui
          └─> ui/wizard
              ├─> ui/wizard/models
              ├─> ui/wizard/presentation
              ├─> ui/wizard/business
              ├─> ui/wizard/tabs
              ├─> ui/wizard/dialogs
              ├─> ui/wizard/template
              └─> ui/wizard/utils
```

**Правила зависимостей:**
- `core` не зависит от `ui`
- `ui/wizard` не зависит от `ui` (кроме точки входа)
- `core/config` не зависит от `core/services`
- Подпакеты не зависят друг от друга (кроме явной необходимости)

## Тестирование

### Структура тестов

- Тесты находятся рядом с кодом (`*_test.go`)
- Тесты для бизнес-логики в `ui/wizard/business/*_test.go`
- Тесты для парсинга в `core/config/subscription/*_test.go`
- Build constraints для тестов с UI зависимостями: `//go:build cgo`

### Типы тестов

- **Unit тесты** - тестирование отдельных функций
- **Integration тесты** - тестирование взаимодействия компонентов
- **Functional тесты** - тестирование бизнес-логики

## Расширение архитектуры

### Добавление нового сервиса

1. Создать файл в `core/services/`
2. Определить структуру сервиса
3. Создать конструктор `NewServiceName()`
4. Добавить сервис в `AppController`
5. Инициализировать в `NewAppController()`

### Добавление новой вкладки UI

1. Создать файл `ui/new_tab.go`
2. Реализовать функцию `CreateNewTab()`
3. Добавить вкладку в `ui/app.go`
4. Зарегистрировать callbacks в `AppController`

### Добавление нового шага визарда

1. Создать файл в `ui/wizard/tabs/`
2. Реализовать функцию создания вкладки
3. Добавить в `wizard.go` в список шагов
4. Обновить навигацию между шагами

## Заключение

Архитектура проекта построена на принципах чистой архитектуры с четким разделением ответственности. Модульная структура позволяет легко расширять функциональность и поддерживать код. Разделение на слои (core, ui, api) обеспечивает независимость компонентов и упрощает тестирование.

