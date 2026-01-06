# Анализ проблемы потери комментариев при создании конфига из Wizard

## Описание проблемы

При создании конфигурации через Wizard теряются все комментарии из исходного шаблона конфигурации. Это критическая проблема, так как:

1. **Важность комментариев для понимания**: Комментарии в секциях конфигурации помогают пользователям понимать назначение различных параметров и настроек
2. **Использование конфигов как шаблонов**: Пользователи хотят использовать свои отредактированные конфиги как шаблоны для других установок, заменяя только подписки
3. **Документация в конфиге**: Комментарии служат встроенной документацией, объясняющей сложные настройки

## Изучение структуры конфигов

### Найденные JSON конфиги в проекте

1. **`bin/config_template.json`** - основной шаблон для Windows/Linux
2. **`bin/config_template_macos.json`** - шаблон для macOS
3. **`bin/config.example.json`** - пример конфигурации с подробными комментариями

### Типы комментариев, найденные в конфигах

#### 1. Однострочные комментарии `//`

**Примеры из config_template.json:**

```json
// Documentation:
// https://github.com/Leadaxe/singbox-launcher/blob/main/README.md#configuring-configjson

// --- LOGGING SETTINGS -----------------------------------------------------
// Defines log detail level, timestamp inclusion, and file output.
"log": {
  // trace debug info warn error fatal panic.
  "level": "warn",   // "info" for daily use, "debug" for troubleshooting.
  "timestamp": true  // Add timestamp to each log entry.
 // "output": "singbox.log" // Uncomment to write logs to file.
},

// --- DNS CONFIGURATION ----------------------------------------------------
// Defines how sing-box resolves domain names. Processed from top to bottom.
"dns": {
  "servers": [
    {
      "type": "udp",
      "tag": "direct_dns_resolver",
      "server": "1.1.1.1",
      "server_port": 53
    },
    // ... другие серверы
  ],
  // Rules for processing DNS queries. Processed from top to bottom.
  "rules": [
    { "domain_suffix": ["githubusercontent.com", "github.com"], "server": "direct_dns_resolver" },
    // ... другие правила
  ],
  "final": "direct_dns_resolver",
  "strategy": "ipv4_only",
  "independent_cache": false
},
```

**Контекст использования:**
- Заголовки секций с разделителями
- Описание назначения секций
- Пояснения к параметрам
- Инструкции по использованию (например, "Uncomment to...")
- Ссылки на документацию

#### 2. Многострочные комментарии `/** */`

**Примеры из config_template.json:**

```json
/** @ParserConfig
  {
    "ParserConfig": {
      "version": 4,
      "proxies": [{ "source": "https://your-subscription-url-here" }],
      // ...
    }
  }
*/

/** @PARSER_OUTBOUNDS_BLOCK */

/**   @SelectableRule
      @label Block Ads (ads-all, soft)
      @default
      @description Soft-block ads by rejecting connections instead of dropping packets
      {"rule_set": "ads-all", "action": "reject"},
*/
```

**Контекст использования:**
- Блоки метаданных для парсера (`@ParserConfig`, `@PARSER_OUTBOUNDS_BLOCK`)
- Блоки правил маршрутизации с метаданными (`@SelectableRule`)
- Документация в начале файла (в config.example.json)

#### 3. Комментарии в секции inbounds

**Примеры из config_template.json:**

```json
// --- INCOMING CONNECTIONS (INBOUNDS) -------------------------------------
"inbounds": [
// TUN Интерфейс: для прозрачного проксирования всего системного трафика (работает как VPN).
  {
    "type": "tun",
    "tag": "tun-in",
    "interface_name": "singbox-tun0", // Имя виртуального сетевого адаптера, создаваемого Sing-box.
    "address": [                      // Используйте диапазон, который не пересекается с вашей реальной LAN.
      "172.16.0.1/30"                 // Внутренний IPv4 адрес для TUN-интерфейса.
    ],
    "mtu": 1492,                      // Максимальный размер передаваемого блока (стандартное значение для Ethernet).
    "auto_route": true,               // Автоматически добавлять/удалять маршруты в системную таблицу маршрутизации.
    "strict_route": false,            // На Windows: принудительно направлять весь трафик через TUN, помогает предотвратить утечки DNS.
    "route_exclude_address": [
      "10.0.0.0/8",          // LAN: WG / другие VPN
      "172.16.0.0/12",       // LAN: TUN sing-box
      "192.168.0.0/16",      // LAN
      "127.0.0.0/8",         // loopback
      // ... другие адреса
    ],
    "stack": "system"                // Использовать гибридный стек TCP/IP для обработки трафика на TUN интерфейсе.
                                    // TCP - внутренний стек Sing-box, UDP - системный стек.
    // Объект "platform" для Windows-специфичных настроек tun, таких как http_proxy,
    // не поддерживается напрямую в этом контексте и удален.
  }
],
```

**Контекст использования:**
- Описание назначения интерфейса
- Пояснения к каждому параметру
- Предупреждения и важные замечания
- Объяснение технических деталей

#### 4. Комментарии в секции route

**Примеры из config_template.json:**

```json
// --- ROUTING RULES -------------------------------------------------------
"route": {
  "default_domain_resolver": "direct_dns_resolver",
  "rule_set": [
    { "tag": "ru-domains", "type": "inline", "format": "domain_suffix", "rules": [{ "domain_suffix": ["ru", "xn--p1ai", "su"] }] },
    // ... другие rule_set
  ],
  "rules": [
    { "inbound": "tun-in", "action": "resolve", "strategy": "prefer_ipv4" },
    { "inbound": "tun-in", "action": "sniff", "timeout": "1s" },
    { "protocol": "dns", "action": "hijack-dns" },
    { "ip_is_private": true, "outbound": "direct-out" },
    { "domain_suffix": ["local", "lan"], "outbound": "direct-out" }
/**   @SelectableRule
      @label Block Ads (ads-all, soft)
      @default
      @description Soft-block ads by rejecting connections instead of dropping packets
      {"rule_set": "ads-all", "action": "reject"},
*/
/**   @SelectableRule
      @label Russian domains direct
      @description Route Russian domains
      { "rule_set": "ru-domains", "outbound": "direct-out" },
*/
// ... другие правила
  ],
  "final": "proxy-out",
  "auto_detect_interface": true
},
```

**Контекст использования:**
- Описание логики маршрутизации
- Блоки `@SelectableRule` с метаданными для UI
- Комментарии к отдельным правилам

#### 5. Комментарии в секции experimental

**Примеры из config_template_macos.json:**

```json
"experimental": {
  "clash_api": {
    "external_controller": "127.0.0.1:9090",
    "secret": "CHANGE_THIS_TO_YOUR_SECRET_TOKEN"
  },
  "cache_file": {
    "enabled": true,          // Обязательно — включает кеш для remote rule-set (и FakeIP, если используешь)
    "path": "cache.db"        // Опционально: имя файла (по умолчанию cache.db)
  }
}
```

**Контекст использования:**
- Пояснения к обязательным параметрам
- Описание опциональных параметров
- Значения по умолчанию

#### 6. Расширенные комментарии в config.example.json

**Примеры:**

```json
/**
 * ============================================================================
 * CONFIG.JSON SETUP INSTRUCTIONS
 * ============================================================================
 * 
 * This file is an example configuration for sing-box launcher.
 * 
 * GETTING STARTED:
 * 
 * 1. Copy this file to config.json:
 *    - Windows: copy config.example.json to config.json in the same bin/ folder
 *    - macOS/Linux: cp bin/config.example.json bin/config.json
 * 
 * 2. Open config.json in a text editor and fill in:
 *    - In the @ParcerConfig block, specify your subscription URL
 *      Replace "https://your-subscription-url-here" with your actual subscription URL
 *    - In the experimental.clash_api section, change "secret" to your token
 *      Replace "CHANGE_THIS_TO_YOUR_SECRET_TOKEN" with a random string
 *    - Configure DNS, routing rules, and other parameters as needed
 * 
 * 3. Save the file and launch singbox-launcher
 * 
 * 4. In the application, click the "Update Config" button in the "Tools" tab
 *    This will automatically load proxies from your subscription and add them to the configuration
 * 
 * DOCUMENTATION:
 * - Full documentation: https://github.com/Leadaxe/singbox-launcher/blob/main/README.md
 * - Official sing-box documentation: https://sing-box.sagernet.org/configuration/
 * - Configuration examples: https://deepwiki.com/chika0801/sing-box-examples/
 * - Parser documentation: see ../docs/ParserConfig.md
 * 
 * IMPORTANT:
 * - Do NOT commit config.json to Git - it contains your personal settings and secrets!
 * - Use config.example.json as a template for new installations
 * - After updating configuration via "Update Config", proxies will be automatically
 *   added between @ParserSTART and @ParserEND markers
 * 
 * CONFIGURATION STRUCTURE:
 * - @ParcerConfig - subscription parser settings (subscription URLs, filters, groups)
 * - log - logging settings
 * - dns - DNS server and rule settings
 * - inbounds - incoming connections (TUN interface for VPN)
 * - outbounds - outgoing connections (proxies, direct, blocking)
 * - route - traffic routing rules
 * - experimental.clash_api - Clash API settings for proxy management
 * 
 * ============================================================================
 */
```

**Контекст использования:**
- Полная документация в начале файла
- Инструкции по настройке
- Ссылки на внешние ресурсы
- Важные предупреждения

## Анализ кода генерации конфига

### Точки потери комментариев

#### 1. `ui/wizard/business/generator.go` - функция `BuildTemplateConfig`

**Проблема:** Использует `FormatSectionJSON`, которая парсит JSON и переформатирует его через `json.Indent`, что теряет комментарии.

**Код:**
```go
// Строка 133, 141
formatted, err = FormatSectionJSON(raw, 2)
if err != nil {
    formatted = string(raw)
}

// FormatSectionJSON использует json.Indent
func FormatSectionJSON(raw json.RawMessage, indentLevel int) (string, error) {
    var buf bytes.Buffer
    prefix := strings.Repeat(" ", indentLevel)
    if err := json.Indent(&buf, raw, prefix, "  "); err != nil {
        return "", err
    }
    return buf.String(), nil
}
```

**Что происходит:**
- `json.Indent` парсит JSON, удаляя все комментарии
- Результат - чистый JSON без комментариев
- Все комментарии из исходного шаблона теряются

#### 2. `ui/wizard/business/generator.go` - функция `MergeRouteSection`

**Проблема:** Использует `json.Unmarshal` и `json.Marshal`, что теряет комментарии при обработке секции route.

**Код:**
```go
// Строка 224-282
func MergeRouteSection(raw json.RawMessage, states []*wizardmodels.RuleState, customRules []*wizardmodels.RuleState, finalOutbound string) (json.RawMessage, error) {
    var route map[string]interface{}
    if err := json.Unmarshal(raw, &route); err != nil {
        return nil, err
    }
    // ... обработка правил ...
    return json.Marshal(route)
}
```

**Что происходит:**
- `json.Unmarshal` парсит JSON, теряя комментарии
- `json.Marshal` создает новый JSON без комментариев
- Все комментарии в секции route теряются

#### 3. `ui/wizard/business/saver.go` - функция `SaveConfigWithBackup`

**Проблема:** При замене secret использует `json.MarshalIndent`, что теряет комментарии.

**Код:**
```go
// Строка 59-99
jsonBytes := jsonc.ToJSON([]byte(configText))  // Конвертирует JSONC в JSON (теряет комментарии)
var configJSON map[string]interface{}
if err := json.Unmarshal(jsonBytes, &configJSON); err != nil {
    return "", fmt.Errorf("invalid JSON: %w", err)
}

// Если не удалось заменить через regex, использует json.MarshalIndent
finalJSONBytes, err := json.MarshalIndent(configJSON, "", "  ")
if err != nil {
    return "", fmt.Errorf("failed to marshal config: %w", err)
}
finalText = string(finalJSONBytes)
```

**Что происходит:**
- `jsonc.ToJSON` конвертирует JSONC в чистый JSON (теряет комментарии)
- `json.MarshalIndent` создает новый JSON без комментариев
- Все комментарии теряются, даже если они были в `configText`

#### 4. `ui/wizard/business/generator.go` - сериализация ParserConfig

**Проблема:** Использует `json.MarshalIndent` для сериализации ParserConfig.

**Код:**
```go
// Строка 67-78
configToSerialize := map[string]interface{}{
    "ParserConfig": parserConfig.ParserConfig,
}
serialized, err := json.MarshalIndent(configToSerialize, "", "  ")
if err == nil {
    parserConfigText = string(serialized)
}
```

**Что происходит:**
- Если ParserConfig был в JSONC формате с комментариями, они теряются
- Создается новый чистый JSON без комментариев

### Цепочка обработки конфига

1. **Загрузка шаблона** (`template/loader.go`)
   - Шаблон загружается с комментариями (JSONC формат)
   - Комментарии сохраняются в `TemplateData.Sections[key]` как `json.RawMessage`

2. **Генерация конфига** (`business/generator.go` - `BuildTemplateConfig`)
   - Для каждой секции вызывается `FormatSectionJSON`
   - `FormatSectionJSON` использует `json.Indent`, который парсит JSON и теряет комментарии
   - Для секции `route` дополнительно вызывается `MergeRouteSection`, которая использует `json.Unmarshal`/`json.Marshal`

3. **Сохранение конфига** (`business/saver.go` - `SaveConfigWithBackup`)
   - Если нужно заменить secret и regex не сработал, используется `json.MarshalIndent`
   - Это создает новый JSON без комментариев

## Примеры потери комментариев

### Пример 1: Секция log

**Исходный шаблон:**
```json
// --- LOGGING SETTINGS -----------------------------------------------------
// Defines log detail level, timestamp inclusion, and file output.
"log": {
  // trace debug info warn error fatal panic.
  "level": "warn",   // "info" for daily use, "debug" for troubleshooting.
  "timestamp": true  // Add timestamp to each log entry.
 // "output": "singbox.log" // Uncomment to write logs to file.
}
```

**Результат после BuildTemplateConfig:**
```json
"log": {
  "level": "warn",
  "timestamp": true
}
```

**Потеряно:**
- Заголовок секции с разделителем
- Описание назначения секции
- Комментарий о возможных значениях level
- Инструкция по использованию output
- Все inline комментарии

### Пример 2: Секция dns

**Исходный шаблон:**
```json
// --- DNS CONFIGURATION ----------------------------------------------------
// Defines how sing-box resolves domain names. Processed from top to bottom.
"dns": {
  "servers": [
    {
      "type": "udp",
      "tag": "direct_dns_resolver",
      "server": "1.1.1.1",
      "server_port": 53
    },
    // ... другие серверы с комментариями
  ],
  // Rules for processing DNS queries. Processed from top to bottom.
  "rules": [
    { "domain_suffix": ["githubusercontent.com", "github.com"], "server": "direct_dns_resolver" },
    // ... другие правила
  ],
  "final": "direct_dns_resolver",
  "strategy": "ipv4_only",
  "independent_cache": false
}
```

**Результат после BuildTemplateConfig:**
```json
"dns": {
  "servers": [
    {
      "type": "udp",
      "tag": "direct_dns_resolver",
      "server": "1.1.1.1",
      "server_port": 53
    }
  ],
  "rules": [
    { "domain_suffix": ["githubusercontent.com", "github.com"], "server": "direct_dns_resolver" }
  ],
  "final": "direct_dns_resolver",
  "strategy": "ipv4_only",
  "independent_cache": false
}
```

**Потеряно:**
- Заголовок секции
- Описание назначения секции
- Комментарии к правилам обработки
- Все inline комментарии

### Пример 3: Секция inbounds

**Исходный шаблон:**
```json
// --- INCOMING CONNECTIONS (INBOUNDS) -------------------------------------
"inbounds": [
// TUN Интерфейс: для прозрачного проксирования всего системного трафика (работает как VPN).
  {
    "type": "tun",
    "tag": "tun-in",
    "interface_name": "singbox-tun0", // Имя виртуального сетевого адаптера, создаваемого Sing-box.
    "address": [                      // Используйте диапазон, который не пересекается с вашей реальной LAN.
      "172.16.0.1/30"                 // Внутренний IPv4 адрес для TUN-интерфейса.
    ],
    "mtu": 1492,                      // Максимальный размер передаваемого блока (стандартное значение для Ethernet).
    "auto_route": true,               // Автоматически добавлять/удалять маршруты в системную таблицу маршрутизации.
    "route_exclude_address": [
      "10.0.0.0/8",          // LAN: WG / другие VPN
      "172.16.0.0/12",       // LAN: TUN sing-box
      "192.168.0.0/16",      // LAN
      // ... другие адреса с комментариями
    ]
  }
]
```

**Результат после BuildTemplateConfig:**
```json
"inbounds": [
  {
    "type": "tun",
    "tag": "tun-in",
    "interface_name": "singbox-tun0",
    "address": [
      "172.16.0.1/30"
    ],
    "mtu": 1492,
    "auto_route": true,
    "route_exclude_address": [
      "10.0.0.0/8",
      "172.16.0.0/12",
      "192.168.0.0/16"
    ]
  }
]
```

**Потеряно:**
- Заголовок секции
- Описание TUN интерфейса
- Все пояснения к параметрам
- Комментарии к каждому адресу в route_exclude_address
- Важные технические замечания

### Пример 4: Секция route с @SelectableRule

**Исходный шаблон:**
```json
// --- ROUTING RULES -------------------------------------------------------
"route": {
  "rules": [
    { "inbound": "tun-in", "action": "resolve", "strategy": "prefer_ipv4" },
    { "protocol": "dns", "action": "hijack-dns" },
    { "ip_is_private": true, "outbound": "direct-out" },
/**   @SelectableRule
      @label Block Ads (ads-all, soft)
      @default
      @description Soft-block ads by rejecting connections instead of dropping packets
      {"rule_set": "ads-all", "action": "reject"},
*/
/**   @SelectableRule
      @label Russian domains direct
      @description Route Russian domains
      { "rule_set": "ru-domains", "outbound": "direct-out" },
*/
  ],
  "final": "proxy-out"
}
```

**Результат после BuildTemplateConfig и MergeRouteSection:**
```json
"route": {
  "rules": [
    { "inbound": "tun-in", "action": "resolve", "strategy": "prefer_ipv4" },
    { "protocol": "dns", "action": "hijack-dns" },
    { "ip_is_private": true, "outbound": "direct-out" },
    { "rule_set": "ads-all", "action": "reject" },
    { "rule_set": "ru-domains", "outbound": "direct-out" }
  ],
  "final": "proxy-out"
}
```

**Потеряно:**
- Заголовок секции
- Блоки `@SelectableRule` с метаданными (label, description, default)
- Комментарии к правилам

**Примечание:** Правила из `@SelectableRule` добавляются в rules, но метаданные теряются. Это может быть ожидаемым поведением для активных правил, но теряются комментарии из исходного шаблона.

### Пример 5: Секция experimental

**Исходный шаблон:**
```json
"experimental": {
  "clash_api": {
    "external_controller": "127.0.0.1:9090",
    "secret": "CHANGE_THIS_TO_YOUR_SECRET_TOKEN"
  },
  "cache_file": {
    "enabled": true,          // Обязательно — включает кеш для remote rule-set (и FakeIP, если используешь)
    "path": "cache.db"        // Опционально: имя файла (по умолчанию cache.db)
  }
}
```

**Результат после BuildTemplateConfig:**
```json
"experimental": {
  "clash_api": {
    "external_controller": "127.0.0.1:9090",
    "secret": "CHANGE_THIS_TO_YOUR_SECRET_TOKEN"
  },
  "cache_file": {
    "enabled": true,
    "path": "cache.db"
  }
}
```

**Потеряно:**
- Комментарии о назначении параметров
- Информация о значениях по умолчанию
- Указания на обязательность параметров

## Контексты использования комментариев

### 1. Документация параметров

Комментарии объясняют назначение каждого параметра, что особенно важно для:
- Сложных технических параметров (например, `stack: "system"` в TUN)
- Параметров с неочевидным поведением (например, `strict_route`)
- Параметров, влияющих на безопасность (например, DNS настройки)

### 2. Инструкции по настройке

Комментарии содержат инструкции:
- Как включить опциональные функции (например, "Uncomment to write logs to file")
- Какие значения использовать в разных сценариях
- Предупреждения о возможных проблемах

### 3. Объяснение логики

Комментарии объясняют:
- Порядок обработки (например, "Processed from top to bottom")
- Взаимосвязи между параметрами
- Технические детали реализации

### 4. Использование как шаблона

Пользователи хотят:
- Сохранить свои комментарии при использовании конфига как шаблона
- Переиспользовать отредактированные конфиги для других установок
- Заменять только подписки, сохраняя все остальные настройки и комментарии

### 5. Метаданные для UI

Блоки `@SelectableRule` содержат:
- `@label` - название правила для отображения в UI
- `@description` - описание правила
- `@default` - флаг, что правило включено по умолчанию

Эти метаданные используются Wizard для отображения правил в UI, но теряются в финальном конфиге.

## Статистика комментариев в конфигах

### config_template.json

- **Общее количество строк:** 285
- **Строк с комментариями:** ~120 (42%)
- **Типы комментариев:**
  - Заголовки секций: ~10
  - Описания секций: ~15
  - Inline комментарии к параметрам: ~60
  - Комментарии в массивах: ~20
  - Блоки @SelectableRule: ~15

### config_template_macos.json

- **Общее количество строк:** 267
- **Строк с комментариями:** ~110 (41%)
- **Аналогичная структура комментариев**

### config.example.json

- **Общее количество строк:** 296
- **Строк с комментариями:** ~150 (51%)
- **Дополнительно:** Большой блок документации в начале файла (~50 строк)

## Выводы

### Критичность проблемы

1. **Высокая важность комментариев:**
   - Комментарии составляют 40-50% содержимого конфигов
   - Они критически важны для понимания конфигурации
   - Без комментариев конфиг становится "черным ящиком"

2. **Потеря функциональности:**
   - Невозможность использовать конфиги как шаблоны
   - Потеря встроенной документации
   - Ухудшение пользовательского опыта

3. **Техническая сложность:**
   - Проблема затрагивает несколько мест в коде
   - Требуется работа с JSONC (JSON with Comments) вместо чистого JSON
   - Необходимо сохранять комментарии при парсинге, форматировании и слиянии

### Области, требующие решения

1. **Парсинг и хранение комментариев:**
   - Нужно сохранять комментарии при загрузке шаблона
   - Хранить комментарии вместе с данными секций

2. **Форматирование с сохранением комментариев:**
   - Заменить `json.Indent` на форматирование с сохранением комментариев
   - Использовать библиотеку, поддерживающую JSONC

3. **Слияние секций с сохранением комментариев:**
   - При слиянии route секции сохранять комментарии из шаблона
   - Добавлять новые правила, не удаляя существующие комментарии

4. **Сохранение конфига:**
   - Сохранять комментарии при записи в файл
   - Не использовать `json.MarshalIndent` для финального конфига

## Постановка задачи

### Цель

Реализовать сохранение всех комментариев из исходного шаблона конфигурации при создании конфига через Wizard.

### Требования

1. **Сохранение всех типов комментариев:**
   - Однострочные комментарии `//`
   - Многострочные комментарии `/** */`
   - Inline комментарии
   - Комментарии в начале файла
   - Комментарии в секциях
   - Комментарии в массивах и объектах

2. **Сохранение структуры комментариев:**
   - Позиция комментариев относительно кода
   - Форматирование комментариев
   - Заголовки секций с разделителями

3. **Сохранение при всех операциях:**
   - При загрузке шаблона
   - При форматировании секций
   - При слиянии route секции
   - При генерации outbounds
   - При сохранении конфига

4. **Совместимость:**
   - Сохранение обратной совместимости с существующим кодом
   - Поддержка как JSON, так и JSONC форматов
   - Корректная работа с библиотекой jsonc

### Ограничения

1. **Блоки @SelectableRule:**
   - Метаданные из `@SelectableRule` могут не сохраняться в финальном конфиге (это ожидаемо, так как они используются только в UI)
   - Но комментарии вокруг этих блоков должны сохраняться

2. **Динамически генерируемые части:**
   - Комментарии в динамически генерируемых outbounds (из парсера) могут быть минимальными
   - Но комментарии из шаблона должны сохраняться

3. **Валидация:**
   - Конфиг должен оставаться валидным JSONC
   - Sing-box должен корректно парсить финальный конфиг

### Критерии успеха

1. **Функциональность:**
   - Все комментарии из шаблона сохраняются в финальном конфиге
   - Комментарии находятся в правильных позициях
   - Конфиг остается валидным JSONC

2. **Тестирование:**
   - Создать тесты, проверяющие сохранение комментариев
   - Проверить все типы комментариев
   - Проверить все секции конфига

3. **Пользовательский опыт:**
   - Пользователи могут использовать свои конфиги как шаблоны
   - Комментарии помогают понимать конфигурацию
   - Конфиг остается читаемым и документированным

## Следующие шаги

После изучения проблемы и постановки задачи необходимо:

1. **Исследовать библиотеки для работы с JSONC:**
   - Изучить возможности библиотеки `github.com/muhammadmuzzammil1998/jsonc`
   - Найти или создать решение для сохранения комментариев при форматировании

2. **Спроектировать решение:**
   - Определить структуру данных для хранения комментариев
   - Спроектировать API для работы с комментариями
   - Определить места в коде, требующие изменений

3. **Реализовать решение:**
   - Сохранять комментарии при загрузке шаблона
   - Модифицировать форматирование для сохранения комментариев
   - Обновить слияние секций для сохранения комментариев
   - Обновить сохранение конфига

4. **Протестировать:**
   - Создать тесты для всех типов комментариев
   - Проверить работу со всеми секциями
   - Убедиться в валидности финального конфига

