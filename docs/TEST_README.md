# Тесты для singbox-launcher

Этот документ описывает автоматизированные тесты, созданные для проекта singbox-launcher.

## Структура тестов

### 1. Тесты парсера узлов (`core/config/subscription/node_parser_test.go`)

Покрывают функциональность парсинга различных типов прокси-узлов:

- **TestIsDirectLink** - проверка определения прямых ссылок (VLESS, VMess, Trojan, Shadowsocks)
- **TestParseNode_VLESS** - парсинг VLESS узлов с различными параметрами (Reality, TLS, порты)
- **TestParseNode_VMess** - парсинг VMess узлов из base64 формата
- **TestParseNode_Trojan** - парсинг Trojan узлов
- **TestParseNode_Shadowsocks** - парсинг Shadowsocks узлов (SIP002 формат)
- **TestParseNode_SkipFilters** - тестирование фильтров пропуска узлов (по тегу, хосту, regex)
- **TestParseNode_RealWorldExamples** - парсинг реальных примеров из подписки
- **TestBuildOutbound** - генерация outbound конфигураций для различных типов узлов

### 2. Тесты парсера подписок (`core/config/subscription/subscription_parser_test.go`)

Покрывают функциональность работы с подписками:

- **TestDecodeSubscriptionContent** - декодирование подписок (base64 URL/стандарт, plain text)
- **TestNormalizeParserConfig** - нормализация ParserConfig (миграция версий, установка значений по умолчанию)
- **TestExtractParserConfig** - извлечение @ParserConfig блока из config.json
- **TestUpdateLastUpdatedInConfig** - обновление поля last_updated в конфигурации
- **TestIsSubscriptionURL** - определение URL подписок

### 3. Тесты сервиса конфигурации (`core/config_service_test.go`)

Покрывают логику обработки прокси-источников:

- **TestProcessProxySource_Subscription** - обработка подписок и прямых ссылок
- **TestProcessProxySource_SkipFilters** - применение фильтров пропуска
- **TestProcessProxySource_TagDeduplication** - дедупликация тегов
- **TestMakeTagUnique** - создание уникальных тегов
- **TestLogDuplicateTagStatistics** - логирование статистики дубликатов
- **TestProcessProxySource_RealWorldExamples** - обработка реальных примеров

### 4. Тесты визарда (`ui/wizard/business/`)

Покрывают логику мастера конфигурации:

- **TestValidateParserConfig** (`validator_test.go`) - валидация ParserConfig
- **TestMergeRouteSection** (`generator_test.go`) - объединение правил маршрутизации
- **TestApplyURLToParserConfig_Logic** (`parser_test.go`) - логика применения URL к конфигурации
- **TestLoadConfigFromFile** (`loader_test.go`) - загрузка конфигурации из файла

### 5. Интеграционные тесты (`core/integration_test.go`)

Комплексные тесты, проверяющие полный цикл работы:

- **TestIntegration_RealWorldSubscription** - парсинг реальных подписок из BLACK_VLESS_RUS.txt
- **TestIntegration_SubscriptionDecoding** - декодирование и парсинг подписок
- **TestIntegration_ParserConfigFlow** - полный цикл от подписки до ParserConfig

## Запуск тестов

### Запуск через батник (Windows, рекомендуется)

Батник автоматически настраивает окружение (CGO, PATH, GCC) и запускает тесты.

**Особенности батника:**
- Показывает список пакетов, которые будут тестироваться перед запуском
- Отображает время начала и окончания тестов
- Сохраняет полный лог тестов в файл `temp\windows\test_output.log`
- Выводит прогресс выполнения тестов в реальном времени
- Использует флаг `-count=1` для гарантии выполнения тестов без использования кеша

#### Запуск всех тестов
```bash
# С паузой в конце
.\build\test_windows.bat

# Без паузы
.\build\test_windows.bat nopause
```

#### Запуск тестов конкретного пакета
```bash
# Тесты парсера узлов
.\build\test_windows.bat nopause ./core/config/subscription

# Тесты сервиса конфигурации
.\build\test_windows.bat nopause ./core

# Тесты визарда (требуют CGO)
.\build\test_windows.bat nopause ./ui/wizard/business

# Интеграционные тесты (требуют CGO)
.\build\test_windows.bat nopause ./core
```

#### Запуск конкретного теста
```bash
# По имени теста
.\build\test_windows.bat nopause run TestParseNode_VLESS ./core/config/subscription

# Короткие тесты (если поддерживается)
.\build\test_windows.bat nopause short
```

#### Параметры батника
- `nopause` или `silent` - не ждать нажатия клавиши в конце
- `short` - запустить только короткие тесты
- `run TestName` - запустить конкретный тест (требует указания имени теста)
- Второй параметр - путь к пакету (по умолчанию `./...` - все тесты)

#### Мониторинг прогресса тестов

Батник предоставляет несколько способов отслеживания прогресса:

1. **Вывод на экран**: Тесты выводят информацию в реальном времени с флагом `-v` (verbose)
2. **Лог-файл**: Полный вывод сохраняется в `temp\windows\test_output.log`
3. **Список пакетов**: Перед запуском показывается список всех пакетов, которые будут тестироваться
4. **Временные метки**: Отображается время начала и окончания тестов

**Проверка активности тестов:**

Если кажется, что тесты зависли, можно проверить:

```cmd
:: Проверить процессы Go
tasklist | findstr go

:: Проверить использование CPU (PowerShell)
powershell -Command "Get-Process go -ErrorAction SilentlyContinue | Format-Table ProcessName, Id, CPU, @{Name='Memory(MB)';Expression={[math]::Round($_.WorkingSet/1MB,2)}}, @{Name='Runtime';Expression={(Get-Date) - $_.StartTime}} -AutoSize"

:: Проверить последние изменения в лог-файле
dir /o-d temp\windows\test_output.log
```

Если процессы `go.exe` или `cgo.exe` активны (используют CPU или память меняется), тесты работают нормально.

### Запуск через go test напрямую

**Важно:** Для тестов, использующих Fyne (визард, интеграционные), требуется `CGO_ENABLED=1` и компилятор C (gcc).

#### Запуск всех тестов
```bash
# С CGO (требуется для визарда и интеграционных тестов)
set CGO_ENABLED=1
go test ./...

# Без CGO (только тесты без UI зависимостей)
set CGO_ENABLED=0
go test ./core/config/subscription
```

#### Запуск тестов конкретного модуля
```bash
# Тесты парсера узлов
go test ./core/config/subscription -v

# Тесты парсера подписок
go test ./core/config/subscription -v -run TestDecodeSubscriptionContent

# Тесты сервиса конфигурации
go test ./core -v -run TestProcessProxySource

# Интеграционные тесты (требуют CGO)
set CGO_ENABLED=1
go test ./core -v -run TestIntegration
```

#### Запуск конкретного теста
```bash
go test ./core/config/subscription -v -run TestParseNode_VLESS
```

## Покрытие тестами

Тесты покрывают следующие основные сценарии:

1. **Парсинг узлов**:
   - Все поддерживаемые протоколы (VLESS, VMess, Trojan, Shadowsocks)
   - Различные параметры (Reality, TLS, порты, пути)
   - Обработка ошибок и некорректных данных

2. **Работа с подписками**:
   - Декодирование base64 (URL и стандартное)
   - Обработка plain text подписок
   - Извлечение и обновление ParserConfig

3. **Фильтрация узлов**:
   - Пропуск по тегу, хосту, схеме
   - Regex фильтры (с учетом регистра и без)
   - Негативные фильтры

4. **Генерация конфигураций**:
   - Создание outbound JSON для различных типов узлов
   - Генерация селекторов
   - Нормализация ParserConfig

5. **Реальные данные**:
   - Использование примеров из BLACK_VLESS_RUS.txt
   - Проверка работы с реальными подписками

## Примеры тестовых данных

Тесты используют реальные примеры из подписки:
- VLESS узлы с Reality
- VLESS узлы с WebSocket
- VLESS узлы с gRPC
- Различные страны и теги

## Требования для запуска тестов

### Для всех тестов
- Go 1.24.4 или выше
- Установленные зависимости проекта (`go mod download`)

### Для тестов с UI зависимостями (визард, интеграционные)
- **CGO_ENABLED=1** (включен по умолчанию в батнике)
- Компилятор C (gcc) - обычно устанавливается вместе с MinGW-w64 или TDM-GCC
- Батник автоматически проверяет наличие GCC и добавляет его в PATH

### Рекомендации
- **Используйте батник** (`test_windows.bat`) для запуска тестов - он автоматически настраивает окружение
- Если GCC не найден, батник попытается найти его в стандартных местах (C:\msys64\mingw64\bin)
- Для ручного запуска через `go test` убедитесь, что `CGO_ENABLED=1` установлен для тестов визарда

## Примечания

- Тесты визарда (`ui/wizard/business`) и интеграционные тесты (`core/integration_test.go`) требуют CGO из-за зависимостей от Fyne
- Интеграционные тесты могут требовать сетевого доступа для проверки подписок
- Тесты используют временные файлы и директории для изоляции
- Батник автоматически устанавливает правильные переменные окружения (CGO_ENABLED, PATH, GOROOT)
- Полный лог тестов сохраняется в `temp\windows\test_output.log` для последующего анализа
- Батник показывает список пакетов перед запуском и временные метки для удобного отслеживания прогресса

