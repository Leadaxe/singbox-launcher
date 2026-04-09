# Схема файла state.json

> **Актуальный поток** (корневой **`version`: 4**, rules library): описание загрузки, миграций и поля **`rules_library_merged`** — в **`docs/WIZARD_STATE.md`**. Этот файл — справочник по полям и историческим форматам; при расхождении приоритет у **`docs/WIZARD_STATE.md`** и кода **`WizardStateFile`**.

## Общая структура

Пример **нового** сохранения (корневая версия **4**, без отдельного слоя selectable в state):

```json
{
  "version": 4,
  "rules_library_merged": true,
  "comment": "Описание состояния (опционально)",
  "created_at": "2026-01-30T12:00:00Z",
  "updated_at": "2026-01-30T12:00:00Z",

  "parser_config": {
    "version": 4,
    "proxies": [...],
    "outbounds": [...],
    "parser": {...}
  },
  "config_params": [
    {
      "name": "route.final",
      "value": "proxy-out"
    },
    {
      "name": "experimental.clash_api.secret",
      "value": "generated-secret-token"
    }
  ],
  "custom_rules": [...]
}
```

У **старых** файлов (до миграции library) может быть **`version": 2`**, **`selectable_rule_states`** и отсутствие **`rules_library_merged`** — при **`LoadState`** выполняется **`ApplyRulesLibraryMigration`** (см. **`docs/WIZARD_STATE.md`**).

---

## Детальное описание полей

### 1. Метаданные состояния

#### `version` (int, обязательное)
- **Тип:** `integer`
- **Значение при сохранении:** **`4`** (текущая). Чтение поддерживает **`2`**, **`3`** и **`4`** (см. **`state_store`**, **`WizardStateFile`**, **`docs/WIZARD_STATE.md`**). **`3`** — rules library. **`4`** — та же база **плюс** модель **032**: в state — **`vars`**, в шаблоне — **`vars`**, **`@name`**, **`if`**/**`if_or`** у **params**; отдельной принудительной миграции v3→v4 нет (файлы **3** без **`vars`** валидны).
- **Описание:** Версия формата **снимка визарда** (корень `state.json`). **Не** то же самое, что **`parser_config.version`** внутри файла. Миграции содержимого: v1→v2 (поля правил), v2→v3 (объединение **`selectable_rule_states`** в **`custom_rules`**, флаг **`rules_library_merged`**).

#### `id` (string, опциональное)
- **Тип:** `string`
- **Обязательность:** 
  - Для `state.json` — **не требуется** (имя файла является идентификатором)
  - Для именованных состояний (`<id>.json`) — **обязательно** и должно совпадать с именем файла (без `.json`)
- **Валидация:** 
  - Только буквы (a-z, A-Z), цифры (0-9), дефис (-), подчёркивание (_)
  - Максимальная длина: 50 символов
  - Должно совпадать с именем файла (для именованных состояний)
- **Пример:** 
  - Для `state.json`: поле отсутствует или `null`
  - Для `my-config.json`: `"my-config"`
  - Для `work-vpn.json`: `"work-vpn"`

#### `comment` (string, опциональное)
- **Тип:** `string`
- **Описание:** Комментарий/описание состояния для пользователя. Может использоваться в UI для отображения дополнительной информации о состоянии.
- **Пример:** `"Конфигурация для работы"`, `"Домашняя сеть с блокировкой рекламы"`, `""` (пустая строка) или поле отсутствует

#### `created_at` (string, обязательное)
- **Тип:** `string` (RFC3339)
- **Описание:** Дата и время создания состояния в формате RFC3339 (UTC)
- **Пример:** `"2026-01-30T12:00:00Z"`

#### `updated_at` (string, обязательное)
- **Тип:** `string` (RFC3339)
- **Описание:** Дата и время последнего обновления состояния в формате RFC3339 (UTC)
- **Пример:** `"2026-01-30T15:30:00Z"`

---

### 2. ParserConfig данные

#### `parser_config` (object, обязательное)
- **Тип:** `object` (JSON объект)
- **Описание:** Полная конфигурация парсера в формате JSON объекта. Это единственный источник ParserConfig для всего приложения:
  - Используется визардом при работе с конфигурацией
  - Используется при нажатии кнопки "🔄 Update" в главном окне для обновления конфигурации из подписок и для регулярных обновлений
- **Важно:** ParserConfig хранится только в `state.json`. При загрузке состояния визард и функция обновления используют этот блок.
- **Структура:** JSON объект с содержимым напрямую (без обертки `ParserConfig`), согласовано с `wizard_template.json`:
  ```json
  {
    "version": 4,
    "proxies": [
      {
        "source": "https://example.com/sub.txt"
      }
    ],
    "outbounds": [],
    "parser": {
      "reload": "1h",
      "last_updated": "2026-01-30T12:00:00Z"
    }
  }
  ```

**Важно:** 
- Это единственный источник ParserConfig для визарда
- При загрузке состояния визард использует этот блок
- При сохранении финальной конфигурации этот блок не копируется в `config.json`, остается только тут

---

**Примечание:** URL источников прокси хранятся в `parser_config.proxies` (поля `source` и `connections`). При восстановлении состояния URL'ы извлекаются из `parser_config.proxies` обратно в текстовый формат для отображения в UI.



---

### 4. Параметры конфигурации

#### `config_params` (array, обязательное)
- **Тип:** `array` of `ConfigParam`
- **Описание:** Параметры конфигурации, используемые при генерации финального конфига. Каждый параметр имеет имя (путь в JSON) и значение.
- **Структура элемента:**
  ```json
  {
    "name": "route.final",
    "value": "proxy-out"
  }
  ```
- **Поля элемента:**
  - `name` (string, обязательное) — путь к параметру в конфигурации, используя точечную нотацию (например, `"route.final"`, `"experimental.clash_api.secret"`)
  - `value` (string, обязательное) — значение параметра (может быть пустой строкой `""` для использования значения по умолчанию из шаблона)

**Стандартные параметры:**
- `route.final` — финальный outbound по умолчанию для правил маршрутизации (используется, если правило не имеет явного outbound)
- `experimental.clash_api.secret` — секретный токен для Clash API. Если отсутствует при сохранении конфига, генерируется случайный, но затем сохраняется в `state.json` для последующего использования

**Пример:**
```json
[
  {
    "name": "route.final",
    "value": "proxy-out"
  },
  {
    "name": "experimental.clash_api.secret",
    "value": "my-secret-token-12345"
  }
]
```

**Важно:**
- Единый шаблон (`wizard_template.json`) используется при генерации конфига (содержит `parser_config`, `config`, `selectable_rules`, `params`)
- Параметры из `config_params` применяются к шаблону при генерации
- Если параметр отсутствует в `config_params`, используется значение по умолчанию из шаблона (если есть)
- Параметр `experimental.clash_api.secret` генерируется автоматически при первом сохранении, если отсутствует, но затем сохраняется в `state.json`

---

### 5. Правила маршрута и legacy-selectable

#### `rules_library_merged` (bool, опционально в старых файлах)
- **Тип:** `boolean`
- **Описание:** После **027** при **`true`** визард хранит правила маршрута **только** в **`custom_rules`** (без слоя **`selectable_rule_states`** в файле); при сборке **`config.json`** к скелету **`route`** из шаблона добавляются включённые записи из **`custom_rules`**. Подробности — **`docs/WIZARD_STATE.md`**, **`ApplyRulesLibraryMigration`**.

#### `selectable_rule_states` (array, только у старых снимков до миграции)
- **Тип:** `array` of `PersistedSelectableRuleState`
- **Описание (исторически v2):** Упрощённые состояния для пресетов шаблона. В актуальном формате (**корневой `version` 4**) после миграции ключ отсутствует или пуст — данные перенесены в **`custom_rules`**. Не путать с **`selectable_rules`** в **`wizard_template.json`** (библиотека пресетов в UI).

**Структура элемента `PersistedSelectableRuleState`:**
```json
{
  "label": "Block ads",
  "enabled": true,
  "selected_outbound": "reject"
}
```

**Поля `PersistedSelectableRuleState`:**
- `label` (string, обязательное) — название правила (используется как ключ для маппинга с шаблоном)
- `enabled` (boolean, обязательное) — включено ли правило в финальной конфигурации
- `selected_outbound` (string, обязательное) — выбранный outbound для правила (может быть "reject", "drop", или имя outbound)

**Пример массива:**
```json
[
  {
    "label": "Block ads",
    "enabled": true,
    "selected_outbound": "reject"
  },
  {
    "label": "Route Russian domains directly",
    "enabled": true,
    "selected_outbound": "direct-out"
  },
  {
    "label": "Messengers via proxy",
    "enabled": false,
    "selected_outbound": "proxy-out"
  }
]
```

**Важно (для файлов, где слой ещё не смержен):**
- Определение правила (rule, description, rule_set, platforms) **не дублируется** в **`selectable_rule_states`** — тело пресета из шаблона.
- При **`ApplyRulesLibraryMigration`** записи переносятся в **`custom_rules`** как полные **`PersistedCustomRule`**.

---

### 6. Пользовательские правила (единый список маршрута после 027)

#### `custom_rules` (array, обязательное)
- **Тип:** `array` of `PersistedCustomRule`
- **Описание:** Все правила маршрута, которыми управляет визард: засев с **`default: true`**, копии из библиотеки пресетов, правила «Add Rule», результат миграции из **`selectable_rule_states`**. Порядок в массиве = порядок в **`MergeRouteSection`** (после базового `route` шаблона).

**Структура элемента `PersistedCustomRule`:**
```json
{
  "label": "My custom rule",
  "type": "urls",
  "enabled": true,
  "selected_outbound": "proxy-out",
  "description": "Route specific domains to proxy",
  "rule": {
    "domain": ["custom.example.com"],
    "outbound": "proxy-out"
  },
  "default_outbound": "proxy-out",
  "has_outbound": true,
  "params": {}
}
```

**Поля `PersistedCustomRule`:**
- `label` (string, обязательное) — название правила
- `type` (string, опциональное) — тип правила; только константы: `ips`, `urls`, `processes`, `srs`, `raw`. При отсутствии или старом формате тип выводится из `rule` при загрузке (DetermineRuleType).
- `enabled` (boolean, обязательное) — включено ли правило
- `selected_outbound` (string, обязательное) — выбранный outbound
- `description` (string, опциональное) — описание правила
- `rule` (object, опциональное) — JSON-объект правила (domain, domain_suffix, ip_cidr, rule_set и т.д.)
- `default_outbound` (string, опциональное) — outbound по умолчанию
- `has_outbound` (boolean) — есть ли поле outbound в правиле
- `params` (object, опциональное) — состояние UI по типу правила; в конфиг не попадает. Для **processes:** `match_by_path` (bool), `path_mode` ("Simple"|"Regex"). Для **urls:** `domain_regex` (bool). Для типов `ips`, `srs`, `raw` может не использоваться.
- `rule_set` (array, опциональное) — только для типа `srs`: массив определений rule-set'ов в формате как в bin/wizard_template.json: `[{ "tag", "type", "format", "url" }, ...]`.

**Пример:**
```json
[
  {
    "label": "Work VPN domains",
    "type": "urls",
    "enabled": true,
    "selected_outbound": "proxy-out",
    "description": "Route work domains through VPN",
    "rule": {
      "domain": ["work.example.com", "internal.corp.com"],
      "outbound": "proxy-out"
    },
    "default_outbound": "proxy-out",
    "has_outbound": true
  },
  {
    "label": "Block trackers",
    "type": "urls",
    "enabled": true,
    "selected_outbound": "reject",
    "rule": {
      "domain_suffix": ["tracker.com", "analytics.net"],
      "action": "reject"
    },
    "has_outbound": false
  }
]
```

**Важно:**
- Эти правила создаются пользователем вручную
- Они не привязаны к шаблону, поэтому хранят полную структуру
- При восстановлении состояния они загружаются полностью (включая `rule`)

---


## Полный пример файла state.json

```json
{
  "version": 2,
  "comment": "Текущее рабочее состояние",
  "created_at": "2026-01-30T12:00:00Z",
  "updated_at": "2026-01-30T15:30:00Z",
  
  "parser_config": {
    "version": 4,
    "proxies": [
      {
        "source": "https://example.com/sub.txt"
      },
      {
        "source": "vless://uuid@server.com:443?encryption=none&security=tls&sni=server.com#Server1"
      }
    ],
    "outbounds": [],
    "parser": {
      "reload": "1h",
      "last_updated": "2026-01-30T12:00:00Z"
    }
  },
  
  "config_params": [
    {
      "name": "route.final",
      "value": "proxy-out"
    },
    {
      "name": "experimental.clash_api.secret",
      "value": "my-secret-token-12345"
    }
  ],
  
  "selectable_rule_states": [
    {
      "label": "Block ads",
      "enabled": true,
      "selected_outbound": "reject"
    },
    {
      "label": "Route Russian domains directly",
      "enabled": true,
      "selected_outbound": "direct-out"
    },
    {
      "label": "Messengers via proxy",
      "enabled": false,
      "selected_outbound": "proxy-out"
    }
  ],
  
  "custom_rules": [
    {
      "label": "Work domains",
      "type": "urls",
      "enabled": true,
      "selected_outbound": "proxy-out",
      "description": "Route work domains through VPN",
      "rule": {
        "domain": ["work.example.com"],
        "outbound": "proxy-out"
      },
      "default_outbound": "proxy-out",
      "has_outbound": true
    }
  ]
}
```

---

## Миграция со старого формата

### Формат v1 → v2

При загрузке `state.json` с `version: 1` (или без version) выполняется автоматическая миграция:

#### `selectable_rule_states`

**Старый формат (v1):**
```json
{
  "type": "System",
  "rule": {
    "label": "Block ads",
    "description": "Block advertisement domains",
    "raw": {
      "domain_suffix": ["ads.example.com"],
      "action": "reject"
    },
    "default_outbound": "reject",
    "has_outbound": false,
    "is_default": true
  },
  "enabled": true,
  "selected_outbound": "reject"
}
```

**Новый формат (v2):**
```json
{
  "label": "Block ads",
  "enabled": true,
  "selected_outbound": "reject"
}
```

**Логика миграции:** Из старого формата извлекается `rule.label`, `enabled` и `selected_outbound`. Остальное (description, raw, default_outbound, has_outbound, is_default, type) берётся из шаблона.

#### `custom_rules`

**Старый формат (v1):**
```json
{
  "type": "Domains/URLs",
  "rule": {
    "label": "Custom rule",
    "description": "Route specific domain",
    "raw": {
      "domain": ["custom.example.com"],
      "outbound": "proxy-out"
    },
    "default_outbound": "proxy-out",
    "has_outbound": true,
    "is_default": false
  },
  "enabled": true,
  "selected_outbound": "proxy-out"
}
```

**Новый формат (v2):**
```json
{
  "label": "Custom rule",
  "type": "Domains/URLs",
  "enabled": true,
  "selected_outbound": "proxy-out",
  "description": "Route specific domain",
  "rule": {
    "domain": ["custom.example.com"],
    "outbound": "proxy-out"
  },
  "default_outbound": "proxy-out",
  "has_outbound": true
}
```

**Логика миграции:** Из вложенного `rule` извлекаются поля на верхний уровень. `rule.raw` становится `rule`. `rule.label` становится `label`. `is_default` удаляется.

---

## Что НЕ сохраняется в state.json

### TemplateData (не сохраняется)
- **Причина:** Шаблон загружается из единого файла `wizard_template.json` при каждом запуске визарда
- **Содержимое:**
  - `ParserConfig` (string) — текст блока parser_config из шаблона. **Используется как fallback** при первой инициализации
  - `Config` (map[string]json.RawMessage) — секции конфига из шаблона (после применения params). **Всегда используются из шаблона** для генерации конфига
  - `ConfigOrder` ([]string) — порядок секций. **Всегда используется из шаблона**
  - `SelectableRules` ([]TemplateSelectableRule) — пресеты секции **`selectable_rules`** в шаблоне (библиотека **Add from library**, первый засев **`default: true`**). После **027** они **не** восстанавливают отдельный маршрутный слой в модели — источник правды для списка на вкладке Rules — **`custom_rules`** в state (**`docs/WIZARD_STATE.md`**).
  - `DefaultFinal` (string) — outbound по умолчанию из config.route.final. **Используется как fallback**, если не задан в `config_params`
- **Восстановление:** шаблон всегда читается с диска; маршрут визарда при **`LoadState`** — из **`custom_rules`** после **`ApplyRulesLibraryMigration`** (если нужно). Режим **New** / «настроить заново» — **`InitializeTemplateState`** засевает **`custom_rules`** из пресетов с **`default: true`**, без **`selectable_rule_states`**.

### GeneratedOutbounds (не сохраняется)
- **Причина:** Генерируются из `parser_config_json` при парсинге
- **Восстановление:** При загрузке состояния запускается парсинг, и outbounds генерируются заново

### TemplatePreviewText (не сохраняется)
- **Причина:** Это кэш для preview, не критично для восстановления состояния
- **Восстановление:** Генерируется заново при необходимости

---

## Восстановление состояния

### Режим редактирования (по умолчанию)

При загрузке `state.json` (по умолчанию):

1. **Загрузить шаблон** из файла `wizard_template.json` — единый шаблон для всех платформ, с фильтрацией по `platforms` и применением `params` для текущей ОС
2. **Восстановить ParserConfig** из `parser_config`:
   - Используется визардом для работы с конфигурацией
   - Используется при нажатии кнопки "🔄 Update" в главном окне для обновления конфигурации
   - Это единственный источник ParserConfig
3. **Извлечь SourceURLs** из `parser_config.proxies` (поля `source` и `connections`) для отображения в UI

4. **Восстановить параметры конфигурации** из `config_params` (например, `route.final`, `experimental.clash_api.secret`)
5. **Восстановить правила маршрута:** **`LoadState`** — миграции JSON, **`ApplyRulesLibraryMigration`**, затем **`custom_rules`** в модель (см. **docs/WIZARD_STATE.md**); пресеты шаблона не подмешиваются вторым списком после миграции.
6. **Запустить парсинг** для генерации `GeneratedOutbounds`
7. **Синхронизировать GUI** с восстановленной моделью

**Важно:** Актуальный маршрут визарда — массив **`custom_rules`**; старый слой **`selectable_rule_states`** при первой загрузке сливается в него.

### Режим "Настроить заново"

При выборе опции "Настроить заново" в UI:

1. **Игнорировать `state.json`** — не загружать сохраненное состояние
2. **Загрузить шаблон** из файла `wizard_template.json`
3. **Инициализировать новое состояние** на основе шаблона:
   - **`InitializeTemplateState`**: заполнить **`custom_rules`** клонами пресетов с **`default: true`**, выставить **`RulesLibraryMerged`** (без **`selectable_rule_states`**)
   - Инициализировать `config_params`:
     - `route.final`: значение из `TemplateData.DefaultFinal` (если есть в шаблоне), иначе пустая строка `""`
     - `experimental.clash_api.secret`: **не инициализируется** — будет сгенерирован автоматически при первом сохранении конфига
   - Загрузить ParserConfig из шаблона (из секции `parser_config`)
4. **Запустить парсинг** для генерации `GeneratedOutbounds`
5. **Синхронизировать GUI** с новой моделью

**Важно:** Режим "Настроить заново" полезен, когда шаблон обновился и нужно начать с чистого листа, игнорируя старое сохраненное состояние.

---

## Валидация при загрузке

- Корневой `version` при записи — **`4`**; допустимо чтение **`2`**, **`3`** и **`4`** (v1→v2 и v2→v3 мигрируются при загрузке). **4** — номер линии с **vars** / **`@…`** / **`if`**/**`if_or`** (**032**); **3** — только rules library, без обязательного **`vars`**
- `id` должен быть валидным (только разрешённые символы, макс. 50 символов)
- `parser_config` должен быть валидным JSON объектом с полями `version`, `proxies`, `outbounds`, `parser`

- `config_params` должен быть массивом
- Каждый элемент `config_params` должен иметь поля `name` и `value` (оба строки)
- У актуального файла **`custom_rules`** — массив; **`selectable_rule_states`** отсутствует или пуст после миграции
- Если присутствует **`selectable_rule_states`** (старый формат), каждый элемент должен иметь поля `label`, `enabled`, `selected_outbound`
- `custom_rules` должен быть массивом
- Каждый элемент `custom_rules` должен иметь поля `label`, `enabled`, `selected_outbound`, `rule`

---

## Размер файла

- **Максимальный размер:** 256 KB
- **Типичный размер:** 5-20 KB (упрощённый формат selectable_rule_states значительно уменьшил размер)
- При превышении лимита показывается предупреждение, но сохранение не блокируется
