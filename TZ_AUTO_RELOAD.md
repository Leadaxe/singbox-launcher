# Техническое задание: Автоматическое обновление конфигурации с меткой времени

## Цель
Добавить автоматическое периодическое обновление конфигурации из подписок с возможностью указания интервала и отслеживания времени последнего обновления.

## Требования

### 1. Расширение структуры ParserConfig

#### 1.1. Добавить объект `parser` с полями `reload` и `last_updated`
- **Расположение**: внутри объекта `ParserConfig` в JSON блоке `@ParcerConfig`
- **Структура**: новый объект `parser` на том же уровне, что и `proxies`, `outbounds`

#### 1.1.1. Поле `last_updated` (время последнего обновления)
- **Тип**: строка в формате RFC3339 (ISO 8601), например: `"2024-01-15T14:30:00Z"`
- **Расположение**: внутри объекта `parser` → `ParserConfig.parser.last_updated`
- **Назначение**: хранит время последнего успешного обновления конфигурации
- **Инициализация**: при первом обновлении устанавливается текущее время
- **Обновление**: перезаписывается при каждом успешном обновлении конфига

#### 1.1.2. Поле `reload` (интервал автоматического обновления)
- **Тип**: строка в формате Go duration (например: `"4h"`, `"30m"`, `"1h30m"`)
- **Расположение**: внутри объекта `parser` → `ParserConfig.parser.reload`
- **Назначение**: определяет, как часто автоматически обновлять конфигурацию
- **Значение по умолчанию**: если не указано, автоматическое обновление отключено
- **Примеры значений**:
  - `"4h"` - каждые 4 часа
  - `"30m"` - каждые 30 минут
  - `"1h30m"` - каждые 1 час 30 минут
  - `""` или отсутствие объекта `parser` - автоматическое обновление отключено

### 2. Изменения в структуре данных

#### 2.1. Обновить структуру `ParserConfig` в `core/subscription_parser.go`
```go
type ParserConfig struct {
    ParserConfig struct {
        Version   int                `json:"version"`
        Proxies   []ProxySource      `json:"proxies"`
        Outbounds []OutboundConfig   `json:"outbounds"`
        Parser    struct {
            Reload      string `json:"reload,omitempty"`      // Интервал обновления
            LastUpdated string `json:"last_updated,omitempty"` // Время последнего обновления
        } `json:"parser,omitempty"`
    } `json:"ParserConfig"`
}
```

**Структура JSON:**
```json
{
  "ParserConfig": {
    "version": 1,
    "proxies": [...],
    "outbounds": [...],
    "parser": {
      "reload": "4h",
      "last_updated": "2024-01-15T14:30:00Z"
    }
  }
}
```

### 3. Логика чтения и записи

#### 3.1. При чтении конфига (`ExtractParcerConfig`)
- Считывать объект `parser` из объекта `ParserConfig` внутри JSON блока `@ParcerConfig`
- Если объект `parser` отсутствует - автоматическое обновление отключено
- Считывать поле `last_updated` из `ParserConfig.parser.last_updated`
- Если поле отсутствует или пустое - считать, что обновление еще не выполнялось
- Считывать поле `reload` из `ParserConfig.parser.reload`
- Если поле отсутствует или пустое - автоматическое обновление отключено

#### 3.2. При записи конфига (`writeToConfig` / `UpdateConfigFromSubscriptions`)
- После успешного обновления конфигурации:
  1. Получить текущее время в формате RFC3339 (UTC)
  2. Обновить поле `last_updated` в структуре `ParserConfig`
  3. При следующей записи в `@ParcerConfig` блок включить обновленное значение `last_updated`
- **Важно**: обновлять `last_updated` только после успешного завершения всех операций (загрузка подписок, парсинг, генерация JSON, запись в файл)

### 4. Автоматическое обновление (фоновый процесс)

#### 4.1. Создать функцию запуска фонового процесса
- **Название**: `StartAutoReloadScheduler` или `StartConfigAutoReload`
- **Расположение**: `core/controller.go` или новый файл `core/auto_reload.go`
- **Параметры**: `*AppController`
- **Логика**:
  1. При старте приложения вызывать эту функцию
  2. Функция должна работать в отдельной горутине
  3. Периодически (например, каждую минуту) проверять:
     - Загружен ли `ParserConfig` из конфига
     - Указан ли интервал `reload` (не пустой)
     - Прошло ли время с момента `last_updated` больше или равно интервалу `reload`
  4. Если все условия выполнены - запустить `RunParserProcess(ac)`
  5. После успешного обновления `last_updated` будет автоматически обновлен в конфиге

#### 4.2. Проверка интервала
- Проверить наличие объекта `parser` в `ParserConfig`
- Если объекта нет - автоматическое обновление отключено
- Парсить строку `parser.reload` в `time.Duration` используя `time.ParseDuration()`
- Сравнивать `time.Now().UTC()` с `parser.last_updated + parser.reload`
- Если `time.Now().UTC() >= parser.last_updated + parser.reload` - запускать обновление
- Если `parser.last_updated` пустое - считать, что нужно обновить сразу (или после первой проверки)

#### 4.3. Защита от одновременных обновлений
- Использовать существующий `ParserMutex` в `AppController` для предотвращения одновременных запусков
- Проверять `ParserRunning` перед запуском автоматического обновления
- Если парсер уже запущен - пропустить эту итерацию, проверить в следующий раз

### 5. Обновление блока @ParcerConfig при записи

#### 5.1. Модификация функции записи конфига
- В функции `writeToConfig` или в месте, где обновляется `@ParcerConfig` блок:
  1. После успешного обновления конфига получить обновленную структуру `ParserConfig`
  2. Сериализовать её в JSON
  3. Заменить весь блок `@ParcerConfig` в файле на новый, включая обновленное `last_updated`

#### 5.2. Формат обновленного блока
```json
/** @ParcerConfig
{
  "ParserConfig": {
    "version": 1,
    "proxies": [
      {
        "source": "https://raw.githubusercontent.com/igareck/vpn-configs-for-russia/main/BLACK_VLESS_RUS.txt"
      }
    ],
    "outbounds": [
      {
        "tag": "proxy-out",
        "type": "selector",
        "options": {
          "interrupt_exist_connections": true
        },
        "outbounds": {},
        "comment": "Proxy group for everything that should go through VPN"
      }
    ],
    "parser": {
      "reload": "4h",
      "last_updated": "2024-01-15T14:30:00Z"
    }
  }
}
*/
```

**Важно**: Поля `reload` и `last_updated` находятся **внутри объекта `parser`**, который находится внутри `ParserConfig` на том же уровне, что и `proxies` и `outbounds`.

### 6. Инициализация при старте приложения

#### 6.1. Запуск фонового процесса
- В `NewAppController` или в `main.go` после инициализации контроллера:
  - Вызвать `StartAutoReloadScheduler(controller)` в отдельной горутине
  - Или вызвать в `SetOnStarted` callback (как для tray menu)

#### 6.2. Первая проверка
- При первом запуске, если `last_updated` пустое и `reload` указан:
  - Можно сразу запустить обновление или подождать первый интервал проверки
  - Рекомендуется: подождать первую проверку (через 1 минуту после старта)

### 7. Логирование

#### 7.1. Логи для автоматического обновления
- При запуске фонового процесса: `"AutoReload: Starting scheduler"`
- При каждой проверке (debug уровень): `"AutoReload: Checking if update needed (last_updated: %s, reload: %s)"`
- При запуске автоматического обновления: `"AutoReload: Triggering automatic config update (interval: %s)"`
- При пропуске (парсер уже запущен): `"AutoReload: Parser already running, skipping this check"`
- При ошибке парсинга интервала: `"AutoReload: Error parsing reload interval '%s': %v"`

### 8. Обработка ошибок

#### 8.1. Некорректный формат `reload`
- Если `time.ParseDuration()` возвращает ошибку:
  - Логировать ошибку
  - Пропустить автоматическое обновление для этого конфига
  - Не падать с ошибкой, продолжать работу

#### 8.2. Некорректный формат `last_updated`
- Если `time.Parse()` возвращает ошибку:
  - Считать, что обновление еще не выполнялось
  - Можно установить `last_updated` в нулевое время или текущее время минус интервал

#### 8.3. Ошибка при обновлении конфига
- Если `UpdateConfigFromSubscriptions` вернула ошибку:
  - `last_updated` НЕ обновлять
  - Логировать ошибку
  - Продолжить работу, следующая проверка произойдет через интервал

### 9. Пример конфигурации

#### 9.1. Пример с автоматическим обновлением каждые 4 часа
```json
/** @ParcerConfig
{
  "ParserConfig": {
    "version": 1,
    "proxies": [
      {
        "source": "https://raw.githubusercontent.com/igareck/vpn-configs-for-russia/main/BLACK_VLESS_RUS.txt"
      }
    ],
    "outbounds": [
      {
        "tag": "proxy-out",
        "type": "selector",
        "options": {
          "interrupt_exist_connections": true
        },
        "outbounds": {},
        "comment": "Proxy group for everything that should go through VPN"
      }
    ],
    "parser": {
      "reload": "4h",
      "last_updated": "2024-01-15T10:30:00Z"
    }
  }
}
*/
```

**Структура**: 
- `@ParcerConfig` - это блок комментария `/** ... */` в `config.json`
- Внутри блока находится JSON объект с полем:
  - `ParserConfig` (объект) - настройки парсера, содержащий:
    - `version` (число) - версия конфигурации
    - `proxies` (массив) - источники подписок
    - `outbounds` (массив) - конфигурация селекторов
    - `parser` (объект, опционально) - настройки автоматического обновления:
      - `reload` (строка, опционально) - интервал автоматического обновления
      - `last_updated` (строка, опционально) - время последнего обновления

#### 9.2. Пример без автоматического обновления
```json
/** @ParcerConfig
{
  "ParserConfig": {
    "version": 1,
    "proxies": [
      {
        "source": "https://raw.githubusercontent.com/igareck/vpn-configs-for-russia/main/BLACK_VLESS_RUS.txt"
      }
    ],
    "outbounds": [
      {
        "tag": "proxy-out",
        "type": "selector",
        "options": {
          "interrupt_exist_connections": true
        },
        "outbounds": {},
        "comment": "Proxy group for everything that should go through VPN"
      }
    ]
  }
}
*/
```
(объект `parser` отсутствует - автоматическое обновление отключено)

### 10. Детали реализации

#### 10.1. Функция обновления `last_updated` в конфиге
- Создать функцию `updateLastUpdatedInConfig(configPath string, lastUpdated time.Time) error`
- Эта функция:
  1. Читает `config.json`
  2. Находит блок `@ParcerConfig`
  3. Парсит JSON внутри блока
  4. Создает объект `parser` если его нет
  5. Обновляет поле `parser.last_updated`
  6. Сериализует обратно в JSON
  7. Заменяет блок в файле

#### 10.2. Интеграция с существующим кодом
- В `UpdateConfigFromSubscriptions` после успешной записи конфига:
  - Вызвать `updateLastUpdatedInConfig(ac.ConfigPath, time.Now().UTC())`
- В фоновом процессе использовать `time.Ticker` с интервалом проверки (например, 1 минута)

#### 10.3. Синхронизация
- Использовать `ParserMutex` для защиты от одновременных обновлений
- Использовать мьютекс при чтении/записи `ParserConfig` из конфига в фоновом процессе

## Этапы реализации

1. **Этап 1**: Расширить структуру `ParserConfig`, добавить поля `reload` и `last_updated`
2. **Этап 2**: Модифицировать `ExtractParcerConfig` для чтения новых полей
3. **Этап 3**: Создать функцию `updateLastUpdatedInConfig` для обновления метки времени
4. **Этап 4**: Интегрировать обновление `last_updated` в `UpdateConfigFromSubscriptions`
5. **Этап 5**: Создать функцию фонового процесса `StartAutoReloadScheduler`
6. **Этап 6**: Интегрировать запуск фонового процесса при старте приложения
7. **Этап 7**: Добавить логирование и обработку ошибок
8. **Этап 8**: Тестирование

## Тестирование

1. Проверить чтение `last_updated` и `reload` из конфига
2. Проверить обновление `last_updated` после успешного обновления
3. Проверить автоматический запуск обновления через указанный интервал
4. Проверить защиту от одновременных обновлений
5. Проверить обработку некорректных форматов
6. Проверить работу без указания `reload` (автообновление отключено)

