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
│   │   │   - CreateHTTPClient()                     # Клиент с общим Transport (пул соединений, ProxyFromEnvironment)
│   │   │   - GetURLBytes()                          # GET по URL тем же клиентом (UI через AppController)
│   │   │   - IsNetworkError()                       # Проверка сетевой ошибки
│   │   │   - GetNetworkErrorMessage()               # Сообщение об ошибке (+ redact URL userinfo)
│   │   │
│   ├── uiservice/             # UI-сервис (Fyne-зависимый, отдельный пакет)
│   │   └── ui_service.go      # Управление UI состоянием и callbacks
│   │       │   - NewUIService()                     # Создание сервиса
│   │       │   - UpdateUI()                         # Обновление UI
│   │       │   - StopTrayMenuUpdateTimer()          # Остановка таймера
│   │       │   - QuitApplication()                  # Выход из приложения
│   │
│   ├── services/              # Сервисы приложения (Fyne-free)
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
│       ├── configtypes/        # Общие типы (отдельный пакет для разрыва циклической зависимости)
│       │   └── types.go        # ParserConfig, ProxySource (exclude_from_global, expose_group_tags_to_global), OutboundConfig, ParsedNode (SourceIndex), NormalizeParserConfig
│       │
│       ├── models.go           # Type aliases → configtypes (обратная совместимость: config.ParsedNode и т.д.)
│       │   │   - ParserConfig = configtypes.ParserConfig
│       │   │   - ProxySource = configtypes.ProxySource
│       │   │   - OutboundConfig = configtypes.OutboundConfig
│       │   │   - ParsedNode = configtypes.ParsedNode
│       │   │
│       ├── config_loader.go    # Загрузка и чтение config.json
│       │   │   - GetSelectorGroupsFromConfig()      # Получение групп селекторов
│       │   │   - GetTunInterfaceName()              # Получение имени TUN интерфейса
│       │   │   - readConfigFile()                   # Чтение config.json
│       │   │   - cleanJSONC()                       # Очистка JSONC
│       │   │
│       ├── outbound_share.go   # Share URI из записанного config.json
│       │   │   - GetOutboundMapByTag()                # outbound из outbounds[] по tag
│       │   │   - GetEndpointMapByTag()                # endpoint из endpoints[] по tag (WireGuard)
│       │   │   - ShareProxyURIForOutboundTag()        # outbound, иначе WG endpoint → share URI
│       │   │
│       ├── outbound_generator.go  # Генерация outbounds и endpoints (ноды + селекторы)
│       │   │   - GenerateNodeJSON() / GenerateEndpointJSON()  # Генерация JSON узла (outbound или WireGuard endpoint)
│       │   │   - GenerateSelectorWithFilteredAddOutbounds() # Генерация селектора с фильтрацией
│       │   │   - GenerateOutboundsFromParserConfig()        # Оркестрация: wireguard → EndpointsJSON, остальные → OutboundsJSON; пул глобальных нод без exclude_from_global; expose → рёбра в глобальные селекторы
│       │   │   - OutboundGenerationResult struct             # Результат (OutboundsJSON, EndpointsJSON, счётчики)
│       │   │   - outboundInfo struct                         # Информация о динамическом селекторе
│       │   │
│       ├── outbound_filter.go    # Логика фильтрации нод для селекторов
│       │   │   - filterNodesForSelector()                   # Фильтрация по tag/host/scheme/label
│       │   │   - FilterNodesExcludeFromGlobal()             # Пул нод для глобальных outbound (исключение по источнику)
│       │   │   - PreviewGlobalSelectorNodes()               # Превью глобального селектора с учётом exclude
│       │   │   - ExposeTagSyntheticNode()                   # Синтетическая нода для сопоставления expose с фильтрами
│       │   │   - matchesFilter() / matchesPattern()         # Literal / regex / negation matching
│       │   │   - PreviewSelectorNodes()                     # Фильтрация для UI preview
│       │   │
│       ├── updater.go          # Обновление конфигурации
│       │   │   - UpdateConfigFromSubscriptions()        # Обновление из подписок (outbounds + endpoints)
│       │   │   - writeToConfig()                        # Запись в config.json (@ParserSTART/E, @ParserSTART_E/END_E)
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
│           ├── node_parser.go           # Парсинг узлов прокси (диспетчер + общие утилиты)
│           │   │   - ParseNode()                               # Парсинг URI узла
│           │   │   - IsDirectLink()                             # Проверка прямого линка
│           │   │   - buildOutbound()                            # Диспетчер построения outbound по протоколу
│           │   │
│           ├── share_uri_encode.go      # Обратная операция: outbound / WG endpoint → share URI
│           │   │   - ShareURIFromOutbound()                     # vless/…/ssh; wireguard → ShareURIFromWireGuardEndpoint
│           │   │   - ShareURIFromWireGuardEndpoint()            # wireguard:// из endpoints[] (один peer)
│           │   │   - ErrShareURINotSupported                    # селекторы, multi-peer WG и пр.
│           │   │
│           ├── node_parser_transport.go # VLESS/Trojan: transport и TLS из URI query
│           │   │   - uriTransportFromQuery() / vlessTLSFromNode() / trojanTLSFromNode()  # зеркало для encode; xhttp/httpupgrade; WS Host: host/sni → obfsParam
│           │   │
│           ├── node_parser_vmess.go     # VMess: parseVMessDecoded (JSON + legacy), нормализация net (httpupgrade, h2)
│           ├── node_parser_wireguard.go # WireGuard протокол
│           ├── node_parser_hysteria2.go # Hysteria2 протокол
│           ├── node_parser_ssh.go       # SSH протокол
│           │
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
│   ├── clash_api_tab.go        # Вкладка Clash API (Servers)
│   │   │   - CreateClashAPITab()                      # Создание вкладки
│   │   │   - onLoadAndRefreshProxies()                # Загрузка прокси
│   │   │   - onTestAPIConnection()                    # Тестирование API
│   │   │   - onResetAPIState()                        # Сброс состояния API
│   │   │   - pingProxy()                              # Пинг прокси
│   │   │   - ПКМ по строке: `fynewidget.SecondaryTapWrap`, `serversProxyContextMenu` → `ProxyInfo.ContextMenuTypeLine` + Copy link → `ShareProxyURIForOutboundTag`
│   │   │
│   ├── diagnostics_tab.go      # Вкладка диагностики
│   │   │   - CreateDiagnosticsTab()                    # Создание вкладки диагностики
│   │   │
│   ├── help_tab.go             # Вкладка помощи
│   │   │   - CreateHelpTab()                           # Создание вкладки помощи
│   │   │
│   ├── dialogs.go              # Общие диалоги (fyne.Do + стандартный dialog)
│   │   │   - ShowError()                                # Показать ошибку
│   │   │   - ShowErrorText()                            # Показать текст ошибки
│   │   │   - ShowInfo()                                 # Показать информацию
│   │   │   - ShowConfirm()                              # Показать подтверждение
│   │   │   - ShowCustom()                               # Кастомный диалог
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
│       │   │   │   - WizardStateFile struct                  # Сериализуемое состояние (save: v3; load: v2..v3, rules_library_merged)
│       │   │   │   - PersistedSelectableRuleState struct     # Упрощённое состояние правила (label, enabled, selected_outbound)
│       │   │   │   - PersistedCustomRule struct              # Полное определение пользовательского правила (type, params, rule_set для srs)
│       │   │   │   - DetermineRuleType()                     # Вывод типа правила из rule при загрузке (ips, urls, processes, srs, raw)
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
│       │   │   │   - SyncGUIToModel()                   # GUI → модель + hasChanges при отличиях
│       │   │   │   - MergeGUIToModel()                  # GUI → модель без hasChanges (табы, parse, закрытие)
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
│       │   │   │   - ensureOutboundsParsed()           # Ждёт фоновый парсинг или запускает ParseAndPreview перед сборкой конфига
│       │   │   │   - buildConfigForSave()              # Построение конфигурации
│       │   │   │   - saveConfigFile()                  # Валидация по временному файлу (config-check.json) и запись config.json с бэкапом
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
│       │   ├── source_edit_window.go  # Окно Edit источника (Настройки / Preview / JSON; Preview: локальные outbounds + ноды подписки; exclude/expose, WIZARD через business)
│       │   │   │   - showSourceEditWindow()                  # Диалог правки одного ProxySource
│       │   │   │
│       │   ├── rules_tab.go    # Вкладка правил
│       │   │   │   - CreateRulesTab()                        # Список custom_rules, toolbar Add / Add from library, SRS, Final
│       │   │   │   - buildCustomRuleRows()                   # Строки: отдельный enable Check, label (тултип), outbound Select, ↑↓ Edit Del
│       │   │   │   - createFinalOutboundSelect()             # Селектор финального outbound
│       │   │   │   - createOutboundSelectorForCustomRule()   # Селектор outbound для правила
│       │   │   │   - createCustomRuleActionButtons()         # ↑↓ Edit Del
│       │   │   │   - deleteCustomRule() / moveCustomRuleUp|Down
│       │   │   │   - buildRulesTabContainer() / CreateRulesScroll()  # Прокрутка с gutter
│       │   ├── settings_tab.go   # Вкладка настроек шаблона (vars)
│       │   │   │   - CreateSettingsTab()                   # Поля из TemplateData.Vars; vars.separator → горизонтальная линия; optional vars.if/if_or → Disable до выполнения условия; Reset → снять override
│       │   ├── settings_tun_darwin.go / settings_tun_stub.go  # darwin: при снятии bool `tun` — блок, если ядро Running; иначе привилегированный rm: cache под bin/ + logs/sing-box.log(.old) под ExecDir (см. CREATE_WIZARD_TEMPLATE)
│       │   ├── library_rules_dialog.go  # Модалка пресетов шаблона
│       │   │   │   - ShowRulesLibraryDialog()                # Чекбоксы, подсветка строк, Add selected → append в CustomRules
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
│       │   │   │   - toProxyInputs() / buildProxiesFromInputs()  # Единая сборка ProxySource (подписки + connection)
│       │   │   │   - restoreTagPrefixAndPostfix()           # Восстановление тегов
│       │   │   │   - connectionsMatch() / isConnectionOnlyProxy()  # Сравнение и определение типа proxy
│       │   │   │   - updateAndSerializeParserConfig()       # Обновление и сериализация
│       │   │   │
│       │   ├── preview_cache.go  # Кэш превью outbounds для UI
│       │   │   │   - Присвоение ParsedNode.SourceIndex при сборке превью
│       │   │   │
│       │   ├── source_local_wizard.go  # Синхронизация локальных urltest/selector с proxies[] (маркеры WIZARD:)
│       │   │   │   - ensure/remove локальных outbounds, expose, переименование тегов при смене префикса
│       │   │   │
│       │   ├── create_config.go  # Сборка конфигурации из шаблона
│       │   │   │   - BuildTemplateConfig()                   # Построение конфигурации
│       │   │   │   - BuildParserOutboundsBlock()             # Построение блока outbounds
│       │   │   │   - buildEndpointsSection()                  # Блок endpoints (WireGuard) @ParserSTART_E/@ParserEND_E
│       │   │   │   - MergeRouteSection()                      # route: база из шаблона + append из custom_rules (+ final / default_domain_resolver)
│       │   │   │
│       │   ├── rules_library.go  # Клон пресета → RuleState, миграция selectable → custom_rules
│       │   │   │   - CloneTemplateSelectableToRuleState(), ClonePresetWithSRSGuard(), AppendClonedPresetsToCustomRules(), ApplyRulesLibraryMigration(), EnsureCustomRulesDefaultOutbounds()
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
│       │   - SwitchProxy()                                    # Переключение прокси (PathEscape группы; json.Marshal name)
│       │   - GetDelay()                                       # Задержка: PathEscape имени прокси; QueryEscape url=
│       │   - ProxyInfo (Name — сырой тег из API для delay/switch; DisplayName — textnorm для UI)
│       │   - ProxyInfo.DisplayOrName()                        # UI: DisplayName или нормализованный Name
│       │
├── internal/                   # Внутренние пакеты
│   ├── textnorm/               # Нормализация UTF-8 и символов в тегах подписок / Clash (❯ → >)
│   │   │   - NormalizeProxyDisplay()
│   │   │
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
│   ├── locale/                 # Локализация (i18n)
│   │   │   - T(key) / Tf(key, args...)            # Перевод строки / с форматированием
│   │   │   - SetLang(lang) / GetLang()            # Установка/получение языка
│   │   │   - Languages() / LangDisplayName(code)  # Список языков / имя для отображения
│   │   │   - LoadSettings(binDir) / SaveSettings() # Чтение/запись settings.json (lang)
│   │   │   - en.json, ru.json (go:embed)          # Встроенные JSON-переводы
│   │   │
│   ├── dialogs/                # Диалоги (без зависимости от ui)
│   │   │   - NewCustom()                                # Кастомный диалог: mainContent (центр), buttons (низ), Border; ESC закрывает
│   │   │   - ShowDownloadFailedManual()                 # Единый диалог при ошибке загрузки (sing-box, wintun, шаблон, SRS, локали): короткое сообщение, ссылка «Open download page» + кнопка копирования URL, «Open folder», «Close»
│   │   │   - ShowAutoHideInfo()                         # Уведомление + диалог на ~2 с (core, визард, вкладки)
│   │   │   - ShowError() / ShowErrorText()              # Показать ошибку (используются из ui/dialogs)
│   │   │
│   ├── fynewidget/             # Переиспользуемые мелкие Fyne-виджеты
│   │   │   - NewCheckWithContent()                     # Пустой Check + произвольный контент: тап/hover с контента на галку; опциональный тултип на контенте
│   │   │   - HoverRow / HoverRowConfig                 # Фон под строкой списка: hover (смесь theme ColorNameBackground + ColorNamePrimary), опционально выделение по cfg; WireTooltipLabelHover(ttwidget.Label) — проброс hover к строке с сохранением колбэков лейбла
│   │   │   - HoverForwardButton, HoverForwardSelect, HoverForwardTTButton  # Кнопка/Select/ttwidget.Button по значению + RowHoverGetter → те же MouseIn/Out на HoverRow; TTWidget() → *ttwidget.Button для async/API
│   │   │   - doc.go                                    # Package comment: порядок row / rowGetter, запрет оборачивать только *widget.Button от NewButton
│   │   │   - NewSecondaryTapWrap()                     # Обёртка строки: `MouseDown` сохраняет модификаторы, `Tapped`/`OnPrimary(mods)` — выбор (Ctrl/Cmd/Shift надёжнее, чем `CurrentKeyModifiers` на отпускании в Windows); `TappedSecondary` → контекстное меню
│   │   │
│   ├── wizardsync/             # Предикаты слияния GUI→модель визарда без Fyne (тестируемо без CGO/GL)
│   │   │   - GuiTextAwaitingProgrammaticFill() / FinalOutboundSelectReadLooksStale()
│   │   │
│   └── platform/              # Платформо-зависимый код
│       │   - платформо-специфичные функции (пути, трей, Dock и т.д.)
│       │   - события питания (Windows): sleep / resume, статус sleep — см. ниже «Platform: события питания»
│
└── assets/                     # Ресурсы (иконки)
```

### Platform: события питания (internal/platform)

Платформа даёт единый контракт для реакции на сон/пробуждение. **Клиенты не зависят от ОС:** они всегда вызывают одни и те же API; платформа сама решает, слать события и выставлять статус или нет (на поддерживаемых ОС — полная реализация, на остальных — no-op).

- **События для подписки:** **sleep** (система уходит в сон) и **resume** (система вышла из сна/гибернации). Подписчики регистрируют колбэки; при sleep — прерывают текущие запросы и не начинают новые; при resume — могут возобновить работу (напр. сброс HTTP-транспорта).
- **Статус sleep:** IsSleeping() — true между sleep и resume. Пакет **api** использует внутри requestContext() и normalizeRequestError() (PowerContext, ErrPlatformInterrupt); публичный API api без контекста. Таймеры (меню трея и т.д.) и цикл AutoLoadProxies проверяют IsSleeping() перед срабатыванием. main.go вызывает RegisterPowerResumeCallback и StopPowerResumeListener безусловно.

Подписчики: api, таймер меню трея, AutoLoadProxies, UI (clash_api_tab). См. SPECS/011-B-C-launcher-freeze-after-sleep (SPEC, PLAN, IMPLEMENTATION_REPORT).

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
- `ReopenChildLogFile()` - после внешнего удаления `logs/sing-box.log` заново открыть файл (macOS: визард после привилегированного rm)
- `GetMainLogFile()` - получение основного лог-файла
- `GetChildLogFile()` - получение лог-файла дочернего процесса
- `GetApiLogFile()` - получение лог-файла API
- Поля: `ExecDir`, `ConfigPath`, `SingboxPath` (запуск/версия; на Linux может указывать на `PATH`), `SingboxBundledPath` (локальный `bin/` — установка из лаунчера), `WintunPath`

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

**outbound_share.go**
- `GetOutboundMapByTag()` - один объект из массива `outbounds` по полю `tag`
- `GetEndpointMapByTag()` - один объект из `endpoints[]` по полю `tag`
- `ShareProxyURIForOutboundTag()` - share URI: сначала outbound, иначе WireGuard endpoint (`subscription.ShareURIFromWireGuardEndpoint`)

**outbound_generator.go**
- `GenerateNodeJSON()` - генерация JSON узла из ParsedNode (vless, vmess, trojan, shadowsocks, hysteria2)
- `GenerateEndpointJSON()` - генерация JSON строки для WireGuard endpoint (ноды с Scheme wireguard)
- `GenerateSelectorWithFilteredAddOutbounds()` - генерация селектора с фильтрацией addOutbounds; опционально глобальный режим и кандидаты **expose** (теги локальных групп источника)
- `GenerateOutboundsFromParserConfig()` - генерация outbounds и endpoints (wireguard-ноды → EndpointsJSON, остальные → OutboundsJSON; трёхпроходный алгоритм для селекторов)
  - На нодах выставляется `SourceIndex`; пул для глобальных outbound — без узлов из источников с `exclude_from_global`
  - Pass 1: Создание outboundsInfo и подсчет узлов
  - Pass 2: Топологическая сортировка зависимостей и расчет валидности; рёбра **expose** → глобальные селекторы
  - Pass 3: Генерация JSON только для валидных селекторов; подмешивание expose-кандидатов в глобальные селекторы
- `OutboundGenerationResult` struct - результат генерации (OutboundsJSON, EndpointsJSON, статистика и счётчики)
- `outboundInfo` struct - информация о динамическом селекторе (для трехпроходного алгоритма)
- `filterNodesForSelector()` - фильтрация узлов для селектора
- `matchesFilter()`, `getNodeValue()`, `matchesPattern()` - вспомогательные функции фильтрации

**outbound_filter.go**
- `FilterNodesExcludeFromGlobal()` — пул нод для глобальных outbound (исключение по `ProxySource.exclude_from_global` и индексу источника)
- `PreviewGlobalSelectorNodes()` — превью списка узлов глобального селектора с учётом exclude
- `ExposeTagSyntheticNode()` — синтетическая нода для проверки фильтров селектора на теги expose
- `PreviewSelectorNodes()` / `filterNodesForSelector()` — как в дереве файлов

**updater.go**
- `UpdateConfigFromSubscriptions()` - обновление config.json из подписок (запись outbounds и endpoints между маркерами @ParserSTART/@ParserEND и @ParserSTART_E/@ParserEND_E)
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

**subscription/** - Работа с подписками (см. SPECS/023-F-C-SUBSCRIPTION_TRANSPORT_VLESS_TROJAN; расширения парсера URI — SPECS/029-Q-С-SUBSCRIPTION_PARSER_CLASH_CONVERTOR_PARITY, **docs/ParserConfig.md**)
- `source_loader.go`:
  - `LoadNodesFromSource()` - загрузка узлов из источника
  - `applyTagPrefixPostfix()` - применение префикса/постфикса к тегам
  - `replaceTagVariables()` - замена переменных в тегах
  - после префикса: **`textnorm.NormalizeProxyDisplay` на тег, затем `MakeTagUnique`** (уникальность по нормализованным строкам)
  - `MakeTagUnique()` - обеспечение уникальности тегов
  - `IsSubscriptionURL()` - проверка URL подписки
  - `MaxNodesPerSubscription` const - лимит узлов на один источник подписки (3000)
- `node_parser_transport.go`:
  - `uriTransportFromQuery()` — VLESS/Trojan: ws/http/grpc; **`xhttp` и `httpupgrade` → httpupgrade**; **ws: `headers.Host` из `host`, `sni` или `obfsParam`**
  - `vlessTLSFromNode()`, `trojanTLSFromNode()` — TLS / Reality по query; **`server_name`: `sni` → `peer` → (Trojan: `host`) → сервер**
- `node_parser.go`:
  - `ParseNode()` - парсинг URI узла прокси; **VLESS:** REALITY без транспорта и без `flow` → `flow: xtls-rprx-vision`; лейбл после sanitize — **textnorm**; **VMess:** JSON в base64, legacy cleartext, отрезание `#` до base64
  - `buildOutbound()` — сборка outbound-мапы для sing-box
  - `IsDirectLink()` - проверка прямого линка
- `node_parser_vmess.go`:
  - `parseVMessDecoded`, `parseVMessJSON`, `parseVMessLegacyCleartext` — разбор payload после base64; в `buildOutbound` для VMess — транспорты **ws/http/grpc/httpupgrade/h2** и TLS (`sni` → `peer` → сервер)
- `node_parser_hysteria2.go`:
  - TLS: insecure, `fp`/`fingerprint` → utls, `pinSHA256` → `certificate_public_key_sha256` (см. **docs/ParserConfig.md**)
- `share_uri_encode.go`:
  - `ShareURIFromOutbound()` — map outbound (как в config.json) → share URI; для `type: wireguard` — `ShareURIFromWireGuardEndpoint`; см. **docs/ParserConfig.md** (раздел Share URI)
  - `ShareURIFromWireGuardEndpoint()` — `wireguard://` из объекта endpoint (один элемент в `peers[]`)
  - `ErrShareURINotSupported` — селекторы, multi-peer WG, нехватка полей, inline SSH key и т.д.
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
- `pingProxy()` - пинг прокси (**имя — `ProxyInfo.Name`**, не DisplayName; путь кодируется в `api.GetDelay`)
- Массовый пинг (**Test all**): счётчик **`pingAllGeneration`** (`sync/atomic`); при повторном запуске ответы предыдущего запуска **не** обновляют список и статус (меньше гонок UI и «откатов» прогресса)
- Список прокси: **`DisplayOrName()`** для подписей; сортировка по отображаемому имени
- Контекстное меню (ПКМ): строка списка обёрнута в `internal/fynewidget.NewSecondaryTapWrap`; `serversProxyContextMenu` / `serversRunCopyShareURIToClipboard`; сверху `ProxyInfo.ContextMenuTypeLine`, затем **Copy link** → `ShareProxyURIForOutboundTag` (outbound или WireGuard в `endpoints[]`); `subscription.ErrShareURINotSupported` → локализованное сообщение пользователю

#### Wizard (`ui/wizard/`)

Визард следует архитектуре MVP (Model-View-Presenter) с четким разделением ответственности:
- **Model** (`models/`) - чистые бизнес-данные без GUI зависимостей
- **View** (`tabs/`, `dialogs/`, `GUIState`) - только GUI виджеты и их компоновка
- **Presenter** (`presentation/`) - связывает модель и представление, координирует бизнес-логику

Списочные строки (вкладки **Rules**, **Sources**, список outbounds в конфигураторе на **Outbounds**, **DNS**, модал **Add from library**) используют **`internal/fynewidget.HoverRow`**, **`WireTooltipLabelHover`** для **`ttwidget.Label`** и **`HoverForward*`** для кнопок/селектов; см. **`internal/fynewidget/doc.go`**, **SPECS/028-F-C-WIZARD_LIST_ROW_HOVER/SPEC.md**.

**wizard.go**
- `ShowConfigWizard()` - точка входа, создание окна визарда
- Создание модели (`WizardModel`), GUI-состояния (`GUIState`) и презентера (`WizardPresenter`)
- Инициализация табов и координация шагов
- Настройка обработчиков событий и навигация

**models/** - Модели данных (без GUI зависимостей)
- `wizard_model.go`:
  - `WizardModel` struct - модель данных визарда (ParserConfig, SourceURLs, GeneratedOutbounds, TemplateData, Rules и т.д.); in-memory мемо **`AvailableOutboundsMemo*`** для **`business.GetAvailableOutbounds`** при работе только от JSON (сброс в **`InvalidatePreviewCache`**)
  - `NewWizardModel()` - создание новой модели
- `rule_state.go`:
  - `RuleState` struct - состояние правила маршрутизации (Rule, Enabled, SelectedOutbound)
- `rule_state_utils.go`:
  - `GetEffectiveOutbound()` - получение эффективного outbound для правила
  - `EnsureDefaultOutbound()` - установка дефолтного outbound
- `wizard_state_file.go`:
  - `WizardStateFile` struct - сериализуемое состояние визарда (version **3** при сохранении; чтение **2..3**, метаданные, ParserConfig, ConfigParams, **`rules_library_merged`**, правила)
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
  - `RefreshOutboundOptions()` - обновление опций outbound для правил (немедленно; отменяет отложенный таймер)
  - `ScheduleRefreshOutboundOptionsDebounced()` / `CancelDebouncedOutboundRefresh()` - debounce ~300 ms при наборе JSON/prefix на вкладке Outbounds
  - `InitializeTemplateState()` - инициализация состояния шаблона
  - `SetTemplatePreviewText()` - установка текста preview
- `presenter_sync.go`:
  - `SyncModelToGUI()` - синхронизация данных из модели в GUI (обновляет текстовые поля, селекторы, пересоздаёт вкладку Rules при необходимости)
  - `SyncGUIToModel()` / `MergeGUIToModel()` - GUI→модель с `MarkAsChanged` или без (смена вкладок, закрытие, preview)
  - Внутри: `syncGUIToModelSourceParserFinal` / `syncGUIToModelDNS`; предикаты «не затирать модель пустым виджетом» — **`internal/wizardsync`** (`GuiTextAwaitingProgrammaticFill`, `FinalOutboundSelectReadLooksStale`), без Fyne для юнит-тестов
- `presenter_async.go`:
  - `TriggerParseForPreview()` - запуск парсинга конфигурации для preview асинхронно
  - `UpdateTemplatePreviewAsync()` - обновление preview шаблона асинхронно
- `presenter_save.go`:
  - `SaveConfig()` - сохранение конфигурации с прогресс-баром и проверками (основная функция)
  - `validateSaveInput()`, `checkSaveOperationState()` - проверки перед сохранением
  - `executeSaveOperation()` - выполнение сохранения в горутине: `ensureOutboundsParsed` (ожидание/парсинг outbounds), затем **`MergeGUIToModelFromMainThread`** (актуализация модели с виджетов после долгого ожидания), сборка конфига, валидация по временному файлу (config-check.json) и запись config.json, state.json, диалог; перезапуск sing-box не выполняется; по завершении в фоне вызывается `core.RunParserProcess()` (обновление конфига из подписок)
  - `finalizeSaveOperation()` - завершение операции и восстановление UI
  - `buildConfigForSave()` - построение конфигурации из шаблона и модели
  - `saveConfigFile()` - валидация sing-box check по временному файлу (config-check.json) и при успехе запись в config.json с бэкапом (вызов SaveConfigWithBackup)
  - `saveStateAndShowSuccessDialog()`, `showSaveSuccessDialog()` - сохранение state и диалог успеха (без перезапуска sing-box)
  - `completeSaveOperation()` - финализация и запуск RunParserProcess в фоне
- `presenter_state.go`:
  - `CreateStateFromModel()` - создание WizardStateFile из текущей модели
  - `SaveCurrentState()` - сохранение текущего состояния в state.json
  - `SaveStateAs()` - сохранение состояния под новым ID
  - `LoadState()` - загрузка состояния из файла в модель
  - `HasUnsavedChanges()` - проверка наличия несохранённых изменений
  - `MarkAsChanged()` - установка флага изменений
  - `MarkAsSaved()` - сброс флага изменений
  - **Хранение и загрузка state:** состояние хранится в `bin/wizard_states/state.json` (текущее) и в `bin/wizard_states/<id>.json` (именованные). При сохранении презентер вызывает `CreateStateFromModel()` (внутри — `SyncGUIToModel`), затем state_store записывает файл. При загрузке state_store читает файл, вызывается `LoadState()`: миграции JSON (MigrateCustomRules, MigrateSelectableRuleStates), затем **`ApplyRulesLibraryMigration`**, **`SelectableRuleStates` в модели всегда nil**, **`restoreCustomRules`**, **`EnsureCustomRulesDefaultOutbounds`** (outbound после миграции), DNS (`restoreDNS` / **`ApplyWizardDNSTemplate`** / **`ApplyDNSVarsFromSettingsToModel`**). При первой миграции library успешная запись state на диск сбрасывает dirty-флаг. В текущей DNS-модели в state: **`dns_options`** хранит только **`servers`**/**`rules`**, а скаляры DNS-вкладки (`dns_strategy`, `dns_independent_cache`, `dns_final`, `dns_default_domain_resolver`) живут в **`vars`**; устаревшие ключи `dns_options` мигрируют при загрузке. Подробно — **docs/WIZARD_STATE.md**, **`SPECS/032-F-C-WIZARD_SETTINGS_TAB/SUB_SPEC_DNS_TAB_VARS.md`**.
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
  - `CreateSourcesTab()` — вкладка **Sources**: URL, список источников на всю оставшуюся высоту (`Border`); **Preview all servers** открывает отдельное окно (`showSourcePreviewAllWindow`, слот `SetViewWindow`); **Edit** — `showSourceEditWindow`
- `source_edit_window.go`:
  - `showSourceEditWindow()` — диалог одного `ProxySource`: вкладки **Настройки**, **Просмотр** (локальные outbounds + ноды), **JSON** (read-only `proxies[i]`); `exclude_from_global` / `expose_group_tags_to_global`; `business` для маркеров **WIZARD:** в `proxies[].outbounds`; обновление Preview/JSON при смене настроек, если вкладка активна
- `rules_tab.go`:
  - `CreateRulesTab()` — единый список **`CustomRules`**, пустое состояние, SRS-кнопки по типу правила
  - `CreateRulesScroll()` — прокрутка с gutter
- `settings_tab.go`:
  - `CreateSettingsTab()` — переменные **`TemplateData.Vars`**, **`separator`**, платформа и **`wizard_ui`**; bool **`tun`** на darwin — см. **`maybeTunOffDarwin`** в **`settings_tun_darwin.go`** (**`settings_tun_stub.go`** на других ОС)
- `business/create_config.go`:
  - **`effectiveTemplateConfig()`** / публичный **`EffectiveConfigSection()`** — мерж шаблона как у превью (для чтения секции **`experimental`** при очистке кеша TUN на macOS)
- `core/config/config_loader.go`:
  - **`ExperimentalCacheFileFromSection()`** — разбор **`experimental.cache_file`** из JSON секции **`experimental`**
- `library_rules_dialog.go`:
  - `ShowRulesLibraryDialog()` — пресеты **`TemplateData.SelectableRules`**, выбор строк, **Add selected**
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
  - Работа с упрощенным форматом: parser_config, selectable_rules (в JSON); при **`LoadState`** срабатывает та же миграция library, что и для обычного state
  - Использует фабрику `wizardmodels.NewWizardStateFile()` для инкапсуляции логики
  - Применяет конфигурацию через `presenter.LoadState()` (та же логика, что и для state.json)
- `rule_dialog.go`:
  - `extractStringArray()` - извлечение массива строк
  - `parseLines()` - парсинг строк

**business/** - Бизнес-логика (без GUI зависимостей)
- `parser.go`:
  - `ParseAndPreview()` - парсинг ParserConfig и генерация outbounds/endpoints через ConfigService (модель берётся из `UIUpdater.Model()`); после генерации сравнивается снимок JSON со старта с текущим `model.ParserConfigJSON` — при расхождении (правки во время долгой генерации) сгенерированные outbounds/endpoints сбрасываются и выставляется `PreviewNeedsParse`, чтобы Save не смешивал ревизии
  - `ApplyURLToParserConfig()` / `AppendURLsToParserConfig()` - применение/добавление URL к ParserConfig
    - `validateApplyURLInput()` - проверка входных данных перед применением URL
    - `parseParserConfigForApply()` - парсинг ParserConfig из JSON строки
    - `classifyInputLines()` - классификация входных строк на подписки и прямые ссылки
    - `preserveExistingProperties()` - сохранение существующих свойств из текущего ParserConfig
    - `toProxyInputs()` / `buildProxiesFromInputs()` - единая сборка списка ProxySource (подписки и connection-only), общий индекс 1:, 2:, …
    - `restoreTagPrefixAndPostfix()` - восстановление tag_prefix и tag_postfix из сохраненных свойств
    - `connectionsMatch()` / `isConnectionOnlyProxy()` - сравнение connections и определение типа proxy
    - `updateAndSerializeParserConfig()` - обновление ParserConfig и сериализация его
  - Бизнес-функции принимают `UIUpdater` (с методом `Model()`) и получают модель из него
- `preview_cache.go`:
  - Кэш превью сгенерированных outbounds; при сборке нод выставляется `ParsedNode.SourceIndex` для фильтрации по источнику в UI
- `source_local_wizard.go`:
  - Синхронизация локальных urltest/selector с `ParserConfig.proxies[i].outbounds` (маркеры **WIZARD:**), флаги expose, переименование тегов при смене префикса
- `create_config.go`:
  - `BuildTemplateConfig()` - построение финальной конфигурации из шаблона и модели
  - `BuildParserOutboundsBlock()` - формирование блока outbounds из сгенерированных outbounds
  - `buildEndpointsSection()` - формирование блока endpoints (WireGuard) между @ParserSTART_E и @ParserEND_E
  - `MergeRouteSection()` - слияние секции **`route`**: базовые **`rules`** / **`rule_set`** из шаблона, затем включённые правила из **`custom_rules`** модели (пресеты шаблона в `route` отдельно не подмешиваются — только через **`custom_rules`**)
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
  - `SaveConfigWithBackup()` - при непустом fileService.SingboxPath(): запись во временный файл `config-check.json`, валидация `sing-box check`, при успехе — бэкап и запись в config; генерация secret для Clash API (prepareConfigText)
  - `ValidateConfigWithSingBox()` - валидация конфига через sing-box check
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
  - `UIUpdater` interface - интерфейс для обновления GUI и доступа к модели: `Model()`, `UpdateParserConfig()`, `UpdateTemplatePreview()`, `UpdateSaveProgress()`, `UpdateSaveButtonText()` (реализуется в презентере)
- `config_service.go`:
  - `ConfigService` interface - интерфейс для генерации outbounds из ParserConfig
  - `ConfigServiceAdapter` - адаптер для core.ConfigService
- `template_loader.go`:
  - `TemplateLoader` interface - интерфейс для загрузки TemplateData
  - `DefaultTemplateLoader` - реализация по умолчанию

**template/** - Работа с единым шаблоном конфигурации
- `loader.go`:
  - `LoadTemplateData()` - загрузка единого JSON-шаблона (`wizard_template.json`), парсинг секций, применение `params` по текущей платформе, фильтрация `selectable_rules` по `platforms`; условия **`params.if`** / **`params.if_or`** смотрят на bool-**`vars`**: при несовпадении **`vars[].platforms`** с текущей ОС переменная даёт **false** в условии (см. **`VarAppliesOnGOOS`** / **`ParamBoolVarTrue`** в **`ui/wizard/template/vars_resolve.go`**, **docs/CREATE_WIZARD_TEMPLATE.md**)
  - `GetTemplateFileName()` - возврат имени файла шаблона (`wizard_template.json`, единый для всех платформ)
  - Объектные **`vars[].default_value`**: **`VarDefaultValue`** / **`defaultValueKeyOrder`** в **`ui/wizard/template/vars_default.go`** (как **`platforms`**: только **`GOOS`**, плюс **`win7`** для **windows/386**, затем **`default`**), разрешение в **`vars_resolve.go`** — **docs/CREATE_WIZARD_TEMPLATE.md** / **_RU.md**
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
      ├─> wizard/tabs/source_tab.go: CreateSourcesTab, CreateOutboundsAndParserConfigTab
      ├─> wizard/tabs/dns_tab.go: CreateDNSTab (модель ↔ UI; частичное обновление: RefreshDNSDependentSelectsOnly / RefreshDNSListAndSelects)
      ├─> wizard/business/wizard_dns.go: ApplyWizardDNSTemplate — слияние шаблона и модели DNS
      ├─> wizard/tabs/rules_tab.go: CreateRulesTab
      ├─> wizard/tabs/settings_tab.go: CreateSettingsTab (+ darwin: settings_tun_darwin — снятие tun / кеш + логи ядра)
      ├─> wizard/tabs/preview_tab.go: CreatePreviewTab
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
      │   │   ├─> ensureOutboundsParsed()
      │   │   ├─> MergeGUIToModelFromMainThread()
      │   │   ├─> buildConfigForSave()
      │   │   │   └─> wizard/business/create_config.go: BuildTemplateConfig()
      │   │   ├─> saveConfigFile()
      │   │   │   └─> wizard/business/saver.go: SaveConfigWithBackup() (внутри: запись в config-check.json, ValidateConfigWithSingBox, при успехе — запись в config)
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
      ├─> core/services          # Fyne-free: FileService, APIService, StateService
      ├─> core/uiservice         # Fyne-зависимый UIService (отдельный пакет)
      ├─> core/config
      │   ├─> core/config/configtypes  # Общие типы (ParsedNode, ParserConfig и пр.)
      │   ├─> core/config/parser
      │   └─> core/config/subscription # Импортирует configtypes (не config) → нет цикла
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
- `core/services` не зависит от Fyne (UIService вынесен в `core/uiservice`)
- `core/config/subscription` импортирует `core/config/configtypes` (не `core/config`) — цикл разорван
- `core/config` может импортировать `core/config/subscription`
- Подпакеты не зависят друг от друга (кроме явной необходимости)

## Известные архитектурные ограничения

### AppController как god object

`AppController` (`core/controller.go`) является центральным координатором, совмещающим:
- Координацию сервисов
- Управление жизненным циклом процессов
- Прямое взаимодействие с UI (диалоги, иконки трея)
- Управление состоянием VPN
- Управление обновлениями

**Риск:** по мере роста проекта контроллер станет сложнее для понимания и тестирования.
**Текущий статус:** оставлено как есть — декомпозиция потребует значительного рефакторинга.
**Рекомендации при будущем рефакторинге:**
- Выделить `UpdateCoordinator` для управления обновлениями ядра
- Выделить `DialogCoordinator` для показа диалогов из core (вместо прямого доступа к `fyne.Window`)
- Перенести логику трея в отдельный сервис

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

