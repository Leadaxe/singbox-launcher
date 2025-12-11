# Тесты для singbox-launcher

Этот документ описывает автоматизированные тесты, созданные для проекта singbox-launcher.

## Структура тестов

### 1. Тесты парсера узлов (`core/parsers/node_parser_test.go`)

Покрывают функциональность парсинга различных типов прокси-узлов:

- **TestIsDirectLink** - проверка определения прямых ссылок (VLESS, VMess, Trojan, Shadowsocks)
- **TestParseNode_VLESS** - парсинг VLESS узлов с различными параметрами (Reality, TLS, порты)
- **TestParseNode_VMess** - парсинг VMess узлов из base64 формата
- **TestParseNode_Trojan** - парсинг Trojan узлов
- **TestParseNode_Shadowsocks** - парсинг Shadowsocks узлов (SIP002 формат)
- **TestParseNode_SkipFilters** - тестирование фильтров пропуска узлов (по тегу, хосту, regex)
- **TestParseNode_RealWorldExamples** - парсинг реальных примеров из подписки
- **TestBuildOutbound** - генерация outbound конфигураций для различных типов узлов

### 2. Тесты парсера подписок (`core/subscription_parser_test.go`)

Покрывают функциональность работы с подписками:

- **TestDecodeSubscriptionContent** - декодирование подписок (base64 URL/стандарт, plain text)
- **TestNormalizeParserConfig** - нормализация ParserConfig (миграция версий, установка значений по умолчанию)
- **TestExtractParserConfig** - извлечение @ParserConfig блока из config.json
- **TestUpdateLastUpdatedInConfig** - обновление поля last_updated в конфигурации
- **TestIsSubscriptionURL** - определение URL подписок

### 3. Тесты сервиса конфигурации (`core/config_service_impl_test.go`)

Покрывают логику обработки прокси-источников:

- **TestProcessProxySource_Subscription** - обработка подписок и прямых ссылок
- **TestProcessProxySource_SkipFilters** - применение фильтров пропуска
- **TestProcessProxySource_TagDeduplication** - дедупликация тегов
- **TestMakeTagUnique** - создание уникальных тегов
- **TestLogDuplicateTagStatistics** - логирование статистики дубликатов
- **TestProcessProxySource_RealWorldExamples** - обработка реальных примеров

### 4. Тесты визарда (`ui/config_wizard_test.go`)

Покрывают логику мастера конфигурации:

- **TestApplyURLToParserConfig** - разделение подписок и прямых ссылок
- **TestSerializeParserConfig** - сериализация ParserConfig в JSON
- **TestGetAvailableOutbounds** - получение доступных outbound опций
- **TestEnsureFinalSelected** - выбор финального outbound
- **TestRealWorldSubscriptionParsing** - парсинг реальных подписок

### 5. Интеграционные тесты (`core/integration_test.go`)

Комплексные тесты, проверяющие полный цикл работы:

- **TestIntegration_RealWorldSubscription** - парсинг реальных подписок из BLACK_VLESS_RUS.txt
- **TestIntegration_SubscriptionDecoding** - декодирование и парсинг подписок
- **TestIntegration_ParserConfigFlow** - полный цикл от подписки до ParserConfig

## Запуск тестов

### Запуск всех тестов
```bash
go test ./...
```

### Запуск тестов конкретного модуля
```bash
# Тесты парсера узлов
go test ./core/parsers -v

# Тесты парсера подписок
go test ./core -v -run TestDecodeSubscriptionContent

# Тесты сервиса конфигурации
go test ./core -v -run TestProcessProxySource

# Интеграционные тесты
go test ./core -v -run TestIntegration
```

### Запуск конкретного теста
```bash
go test ./core/parsers -v -run TestIsDirectLink
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

## Примечания

- Некоторые тесты требуют отсутствия UI зависимостей (fyne)
- Интеграционные тесты могут требовать сетевого доступа для проверки подписок
- Тесты используют временные файлы и директории для изоляции

