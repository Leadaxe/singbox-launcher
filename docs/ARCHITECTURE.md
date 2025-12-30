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
│   ├── error_handler.go       # Обработка ошибок
│   │   │   - (утилиты обработки ошибок)
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
│       ├── generator.go        # Генерация конфигурации
│       │   │   - GenerateNodeJSON()                          # Генерация JSON узла
│       │   │   - GenerateSelector()                          # Генерация селектора (legacy)
│       │   │   - GenerateSelectorWithFilteredAddOutbounds()  # Генерация селектора с фильтрацией
│       │   │   - GenerateOutboundsFromParserConfig()         # Генерация outbounds (трехпроходный алгоритм)
│       │   │   - OutboundGenerationResult struct             # Результат генерации
│       │   │   - outboundInfo struct                         # Информация о динамическом селекторе
│       │   │
│       ├── updater.go          # Обновление конфигурации
│       │   │   - UpdateConfigFromSubscriptions()        # Обновление из подписок
│       │   │   - writeToConfig()                        # Запись в config.json
│       │   │
│       ├── parser/             # Парсинг @ParserConfig блока
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
│   ├── dialogs.go              # Общие диалоги
│   │   │   - ShowError()                                # Показать ошибку
│   │   │   - ShowErrorText()                            # Показать текст ошибки
│   │   │   - ShowInfo()                                 # Показать информацию
│   │   │   - ShowConfirm()                              # Показать подтверждение
│   │   │   - ShowAutoHideInfo()                         # Автоскрываемая информация
│   │   │
│   ├── error_banner.go         # Баннеры ошибок
│   │   │   - NewErrorBanner()                           # Создание баннера ошибки
│   │   │   - ErrorBanner struct                         # Структура баннера
│   │   │
│   └── wizard/                 # Мастер конфигурации
│       ├── wizard.go           # Точка входа (ShowConfigWizard)
│       │   │   - ShowConfigWizard()                     # Точка входа визарда
│       │   │
│       ├── state/              # Управление состоянием визарда
│       │   ├── state.go        # WizardState, SelectableRuleState
│       │   │   │   - WizardState struct                 # Состояние визарда
│       │   │   │   - SelectableRuleState struct         # Состояние правила
│       │   │   │   - ContainsString()                   # Проверка строки
│       │   │   │   - EnsureDefaultOutbound()            # Установка дефолтного outbound
│       │   │   │   - GetEffectiveOutbound()             # Получение эффективного outbound
│       │   │   │
│       │   └── helpers.go      # Вспомогательные функции
│       │       │   - SafeFyneDo()                            # Безопасный вызов Fyne
│       │       │   - DebugLog(), InfoLog(), ErrorLog()       # Логирование
│       │       │
│       ├── tabs/               # UI компоненты вкладок
│       │   ├── source_tab.go   # Вкладка Sources & ParserConfig
│       │   │   │   - createSourceTab()                       # Создание вкладки Sources & ParserConfig
│       │   │   │
│       │   ├── rules_tab.go    # Вкладка правил
│       │   │   │   - createTemplateTab()                     # Создание вкладки правил
│       │   │   │   - createRulesScroll()                     # Создание списка правил
│       │   │   │
│       │   └── preview_tab.go  # Вкладка превью
│       │       │   - createPreviewTab()                      # Создание вкладки превью
│       │       │
│       ├── dialogs/            # Диалоги визарда
│       │   ├── add_rule_dialog.go  # Диалог добавления правила
│       │   │   │   - ShowAddRuleDialog()                     # Показать диалог добавления правила
│       │   │   │
│       │   └── rule_dialog.go      # Утилиты для диалогов
│       │       │   - extractStringArray()                    # Извлечение массива строк
│       │       │   - parseLines()                            # Парсинг строк
│       │       │
│       ├── business/           # Бизнес-логика
│       │   ├── parser.go       # Парсинг URL и конфигурации
│       │   │   │   - ParseAndPreview()                       # Парсинг и превью
│       │   │   │   - CheckURL()                              # Проверка URL
│       │   │   │   - ApplyURLToParserConfig()                # Применение URL
│       │   │   │   - SetPreviewText()                         # Установка текста превью
│       │   │   │
│       │   ├── generator.go    # Генерация конфигурации
│       │   │   │   - BuildTemplateConfig()                   # Построение конфигурации
│       │   │   │   - BuildParserOutboundsBlock()             # Построение блока outbounds
│       │   │   │   - MergeRouteSection()                      # Объединение route секции
│       │   │   │
│       │   ├── validator.go    # Валидация данных
│       │   │   │   - ValidateParserConfig()                   # Валидация конфигурации
│       │   │   │   - ValidateURL()                             # Валидация URL
│       │   │   │   - ValidateURI()                             # Валидация URI
│       │   │   │   - ValidateJSONSize()                        # Валидация размера JSON
│       │   │   │
│       │   └── loader.go       # Загрузка и сохранение
│       │       │   - LoadConfigFromFile()                      # Загрузка из файла
│       │       │   - SerializeParserConfig()                   # Сериализация конфигурации
│       │       │   - EnsureRequiredOutbounds()                 # Обеспечение outbounds
│       │       │
│       ├── template/            # Работа с шаблонами
│       │   └── loader.go        # Загрузка шаблонов
│       │       │   - LoadTemplateData()                        # Загрузка данных шаблона
│       │       │   - GetTemplateURL()                          # Получение URL шаблона
│       │       │   - TemplateData struct                       # Структура данных шаблона
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
│   ├── dialogs/                # Утилиты диалогов
│   └── platform/              # Платформо-зависимый код
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
- Структуры: `UIService` с полями для Fyne компонентов и callbacks

**APIService** (`api_service.go`)
- `NewAPIService()` - создание сервиса
- `GetClashAPIConfig()` - получение конфигурации API
- `GetProxiesList()` - получение списка прокси
- `SetProxiesList()` - установка списка прокси
- `GetActiveProxyName()` - получение активного прокси
- `SetActiveProxyName()` - установка активного прокси
- `SwitchProxy()` - переключение прокси
- `AutoLoadProxies()` - автозагрузка прокси

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

**generator.go**
- `GenerateNodeJSON()` - генерация JSON узла из ParsedNode (vless, vmess, trojan, shadowsocks, hysteria2)
- `GenerateSelector()` - генерация селектора из узлов (legacy, для обратной совместимости)
- `GenerateSelectorWithFilteredAddOutbounds()` - генерация селектора с фильтрацией addOutbounds (новый метод)
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

**parser/** - Работа с @ParserConfig блоком
- `factory.go`:
  - `ExtractParserConfig()` - извлечение ParserConfig из config.json
  - `NormalizeParserConfig()` - нормализация конфигурации
  - `LogDuplicateTagStatistics()` - логирование статистики дубликатов
- `migrator.go`:
  - Миграция версий @ParserConfig (v1 → v2 → v3 → v4)
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

**Примечание:** Генерация конфигурации выполняется функциями из пакета `core/config/generator.go`:
- `GenerateOutboundsFromParserConfig()` - генерация outbounds из конфигурации (трехпроходный алгоритм)
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

**wizard.go**
- `ShowConfigWizard()` - точка входа, создание окна визарда
- Инициализация визарда
- Координация шагов и навигация

**state/** - Управление состоянием
- `state.go`:
  - `WizardState` struct - состояние визарда
  - `SelectableRuleState` struct - состояние правила
  - Константы: `defaultOutboundTag`, `rejectActionName`
  - `ContainsString()` - проверка наличия строки в слайсе
  - `EnsureDefaultAvailableOutbounds()` - установка дефолтных outbounds
  - `EnsureDefaultOutbound()` - установка дефолтного outbound
  - `GetEffectiveOutbound()` - получение эффективного outbound
- `helpers.go`:
  - `SafeFyneDo()` - безопасный вызов Fyne функций
  - `DebugLog()`, `InfoLog()`, `ErrorLog()` - логирование

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
- `rule_dialog.go`:
  - `extractStringArray()` - извлечение массива строк
  - `parseLines()` - парсинг строк

**business/** - Бизнес-логика
- `parser.go`:
  - `ParseAndPreview()` - парсинг URL и превью конфигурации
  - `CheckURL()` - проверка URL
  - `ApplyURLToParserConfig()` - применение URL к конфигурации
  - `SetPreviewText()` - установка текста превью
  - `SerializeParserConfig()` - сериализация конфигурации
  - `GenerateTagPrefix()` - генерация префикса тега
- `generator.go`:
  - `BuildTemplateConfig()` - построение конфигурации из шаблона
  - `BuildParserOutboundsBlock()` - построение блока outbounds
  - `MergeRouteSection()` - объединение секции route
  - `cloneRule()` - клонирование правила
  - `getEffectiveOutbound()` - получение эффективного outbound
  - `IndentMultiline()` - форматирование многострочного текста
  - `FormatSectionJSON()` - форматирование JSON секции
- `validator.go`:
  - `ValidateParserConfig()` - валидация конфигурации парсера
  - `ValidateURL()` - валидация URL
  - `ValidateURI()` - валидация URI
  - `ValidateOutbound()` - валидация outbound
  - `ValidateRule()` - валидация правила
  - `ValidateJSONSize()` - валидация размера JSON
  - `ValidateJSON()` - валидация JSON
  - `ValidateHTTPResponseSize()` - валидация размера HTTP ответа
- `loader.go`:
  - `LoadConfigFromFile()` - загрузка конфигурации из файла
  - `EnsureRequiredOutbounds()` - обеспечение необходимых outbounds
  - `CloneOutbound()` - клонирование outbound
  - `SerializeParserConfig()` - сериализация конфигурации

**template/** - Шаблоны
- `loader.go`:
  - `LoadTemplateData()` - загрузка данных шаблона
  - `GetTemplateURL()` - получение URL шаблона
  - `TemplateData` struct - структура данных шаблона
  - `TemplateSelectableRule` struct - правило из шаблона

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
│    • Ping Proxy                                             │
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
│  │  • Ротация логов                                     │   │
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
│  │  generator.go:                                       │   │
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
│  │  • Извлечение @ParserConfig блока                    │   │
│  │  • Нормализация конфигурации                         │   │
│  │  • Миграция версий                                   │   │
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
│  │  Wizard Package [ui/wizard/]                         │   │
│  │                                                      │   │
│  │  wizard.go:                                          │   │
│  │  • Координация шагов визарда                         │   │
│  │  • Инициализация визарда                             │   │
│  │                                                      │   │
│  │  state/:                                             │   │
│  │  • Управление состоянием визарда                     │   │
│  │  • WizardState, SelectableRuleState                  │   │
│  │                                                      │   │
│  │  tabs/:                                              │   │
│  │  • UI компоненты вкладок                             │   │
│  │  • Source, Rules, Preview                            │   │
│  │                                                      │   │
│  │  business/:                                          │   │
│  │  • Парсинг URL и конфигурации                        │   │
│  │  • Генерация конфигурации                            │   │
│  │  • Валидация данных                                  │   │
│  │  • Загрузка и сохранение                             │   │
│  │                                                      │   │
│  │  template/:                                          │   │
│  │  • Загрузка шаблонов                                 │   │
│  │                                                      │   │
│  │  dialogs/:                                           │   │
│  │  • Диалоги визарда                                   │   │
│  │                                                      │   │
│  │  utils/:                                             │   │
│  │  • Утилиты и константы                               │   │
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
          └─> config/generator.go: GenerateOutboundsFromParserConfig()
```

### Поток работы визарда

```
UI (core_dashboard_tab.go)
  └─> wizard.ShowConfigWizard()
      ├─> wizard/tabs/source_tab.go: createSourceTab()
      ├─> wizard/tabs/rules_tab.go: createTemplateTab()
      ├─> wizard/tabs/preview_tab.go: createPreviewTab()
      │
      ├─> wizard/business/parser.go: ParseAndPreview()
      ├─> wizard/business/validator.go: ValidateParserConfig()
      ├─> wizard/business/generator.go: BuildTemplateConfig()
      └─> wizard/business/loader.go: SerializeParserConfig()
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

- Все комментарии на английском языке
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
              ├─> ui/wizard/state
              ├─> ui/wizard/tabs
              ├─> ui/wizard/dialogs
              ├─> ui/wizard/business
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

