# Синтаксис `wizard_template.json`

Справочник по формату шаблона визарда. Шаблон — один JSON-файл, описывающий
как из подписки и выбора пользователя собирается итоговый `config.json` sing-box.

- **Расположение:** `<каталог_бинаря>/bin/wizard_template.json`. Для macOS `.app` —
  `Contents/MacOS/bin/wizard_template.json` внутри бандла (не файл из репозитория).
- **Внутренние детали движка** (walker, build pipeline, формат stock-JSON):
  [`TEMPLATE_REFERENCE.md`](TEMPLATE_REFERENCE.md). Здесь — только синтаксис для автора шаблона.

---

## 1. Top-level структура

```json
{
  "parser_config": { ... },
  "config":        { ... },
  "dns_options":   { ... },
  "vars":     [ ... ],
  "params":   [ ... ],
  "presets":  [ ... ]
}
```

| Ключ | Тип | Назначение |
|---|---|---|
| `parser_config` | object | Настройки парсера подписки + статические outbound-группы (selector/urltest). Сюда инжектятся ноды из подписки. |
| `config` | object | Базовый скелет конфига sing-box (`route`, `experimental`, `inbounds`, …), в который подставляются `@var` и применяются `params`. |
| `dns_options` | object | Дефолты DNS-вкладки: `servers[]` + `rules[]`. |
| `vars` | array | Типизированные переменные шаблона → строки **Настроек**, источники `@`-плейсхолдеров. |
| `params` | array | Патчи к `config` по dot-path с условиями (`if`) и режимом слияния. |
| `presets` | array | Пресеты для вкладки **Rules** (правила, rule_set, DNS, outbounds). |

> Устаревшее: ключ `selectable_rules` удалён (SPEC 053) — правила только через `presets[]`.

---

## 2. Плейсхолдеры `@var`

В `config`, `params[].value`, телах пресетов строка вида `"@name"` заменяется
значением переменной `name` из `vars[]` (или preset var в scope пресета).

- **Скаляр:** `"mtu": "@tun_mtu"` → значение (для int-вар — числом, не строкой).
- **Список (одноэлементный массив):** `["@dns_list"]` где `dns_list` — `text_list` →
  массив разворачивается в список строк.
- **Тип результата** определяется типом var: `bool` → `true/false`, `text_list` → массив,
  числовые vars (`tun_mtu`, `mixed_listen_port`, `proxy_in_listen_port`, `urltest_tolerance`)
  → число, остальные → строка.
- Имя после `@` не содержит пробелов; неизвестный `@var` в `config` → ошибка загрузки.

`@var` как **ключ** объекта используется только в predicate'ах `#if` (§5), в обычном
JSON ключи не подставляются.

---

## 3. `vars[]` — переменные

Каждый элемент описывает одну переменную (строку на вкладке **Настройки**).

| Поле | Тип | Описание |
|---|---|---|
| `name` | string | Идентификатор (`[A-Za-z_][A-Za-z0-9_]*`). Имя `runtime` зарезервировано. |
| `type` | string | `text` \| `bool` \| `enum` \| `text_list` \| `secret`. |
| `default_value` | см. §3.2 | Значение по умолчанию (если нет в state). |
| `default_node` | string | Альтернатива: dot-path внутри шаблона, откуда взять дефолт. |
| `options` | array | Для `enum`: `["a","b"]` или `[{"title","value"}]`. |
| `wizard_ui` | string | `edit` (по умолч.) \| `view` (read-only) \| `hidden` \| `fix` (правится в другом месте UI). |
| `platforms` | array | ОС, где переменная активна (пусто = все). |
| `title` | string | Подпись строки (пусто → `name`). |
| `tooltip` | string | Всплывающая подсказка. |
| `if` / `if_or` | array | Условие активности строки — `@bool_var` (§3.3). |

### 3.1 Типы

| Тип | Виджет | Подстановка |
|---|---|---|
| `text` | поле ввода (+ combo, если задан `options`) | строка |
| `bool` | чекбокс | `true` / `false` |
| `enum` | dropdown | `value` выбранного варианта |
| `text_list` | многострочное поле | массив строк (по строкам) |
| `secret` | masked-поле (точки) + глаз + кнопка регенерации; всегда предзаполнено случайным значением | строка |

Объектная форма `options` (`[{title,value}]`) автоматически делает переменную `enum`.

### 3.2 `default_value` — три формы

```json
"default_value": "system"                              // 1. скаляр (строка/число/bool)
"default_value": {"win7": "gvisor", "default": "system"}  // 2. по платформе
"default_value": {"#if": { ... }}                      // 3. выражение #if (только @runtime.*)
```

**По платформе** — перебор ключей: `win7` (только `windows`/`386`) → `<goos>`
(`windows`/`linux`/`darwin`) → `default`.

**`#if`** (§5) вычисляется в runtime; внутри разрешены **только** `@runtime.*` globals,
ссылки на другие vars запрещены (ошибка загрузки). Можно и в платформенном ключе:
`{"default": {"#if": {...}}}`.

### 3.3 `if` / `if_or` (внешнее условие)

```json
{"name": "tun_address", "type": "text", "if": ["@tun"]}
```

- Каждый элемент — **`@имя_bool_var`** (префикс `@` обязателен; голое `"tun"` → ошибка загрузки).
- `if` — активна, если **все** перечисленные `true`; `if_or` — если **хотя бы одна**. Взаимоисключение.
- На вкладке **Настройки** строка остаётся видимой, но **поля отключены**, пока условие не выполнено.
- Runtime globals (`@runtime.*`) в outer `if`/`if_or` **запрещены** — только в `#if`.

### 3.4 Разделитель

```json
{"separator": true}
```
Горизонтальная линия между строками Настроек. Без `name`/`type`/`default_value`/прочего
(допускается только `platforms` и `wizard_ui: "hidden"`).

---

## 4. `#if` — условные поля

Control-construct: включает/исключает поля или элементы по условию. Ключ `#if`
**не попадает** в выходной JSON.

### 4.1 Форма

```json
"#if": {
  "and":   [ <predicate>, ... ],   // ИЛИ "or" — ровно один из and/or
  "value": <любой JSON>,           // подставляется при истинном условии
  "else":  <любой JSON>            // опционально — при ложном
}
```

- Ровно один из `and` / `or` (оба или ни одного → ошибка). `value` обязателен.
- `and` — все predicate'ы `true`; `or` — хотя бы один.

### 4.2 Два режима размещения

**Map-spread** — `#if` как ключ внутри объекта; при истине поля ветви **мерджатся в родителя**:
```json
{"type": "tun", "tag": "tun-in",
 "#if": {"and": [{"@runtime.platform": {"#in": ["windows","linux"]}}],
         "value": {"interface_name": "singbox-tun0"}}}
```

**Array-element** — одиночный `{"#if": {...}}` как элемент массива; элемент
**заменяется** ветвью или **выбрасывается** (если ложь и нет `else`):
```json
"inbound": [
  {"#if": {"and": ["@tun"],             "value": "tun-in"}},
  {"#if": {"and": ["@enable_proxy_in"], "value": "proxy-in"}}
]
```

### 4.3 Predicate'ы (элементы `and` / `or`)

| Форма | Семантика |
|---|---|
| `"@var"` | bool var истинна (`scalar == "true"`) |
| `{"@var": "literal"}` | равенство: `trim(scalar) == "literal"` |
| `{"@var": "#notEmpty"}` | непусто (text: длина > 0; text_list: есть элементы; bool: true) |
| `{"@var": "#isEmpty"}` | инверсия `#notEmpty` |
| `{"@var": {"#in": ["a","b"]}}` | `scalar` в списке (список может быть `"@text_list_var"`) |
| `{"@var": {"#notIn": ["a","b"]}}` | `scalar` не в списке |
| `{"@var": {"#matches": "^re$"}}` | `scalar` матчит Go-regexp |
| `{"#not": <predicate>}` | отрицание любого predicate'а (рекурсивно) |

Аргументы (literal, элементы `#in`, regex `#matches`) могут содержать `@var` — подставляются до оценки.

### 4.4 Runtime globals — `@runtime.*`

Псевдо-переменные, доступные **только** в predicate'ах `#if` (не объявляются в `vars`):

| Global | Источник | Значения |
|---|---|---|
| `@runtime.platform` | `runtime.GOOS` | `"darwin"`, `"windows"`, `"linux"` |
| `@runtime.arch` | `runtime.GOARCH` | `"amd64"`, `"arm64"`, `"386"` |

- Только text-predicate'ы (equality, `#in`, `#matches`, …); **голое** `"@runtime.platform"` в списке → ошибка.
- Win7 = `windows`+`386`: `{"and": [{"@runtime.platform": "windows"}, {"@runtime.arch": "386"}]}`.
- Namespace расширяемый; имя `runtime` в `vars[].name` зарезервировано.

### 4.5 Дисциплина именования

- `#` — control-constructs и predicate'ы (`#if`, `#in`, `#not`, `#notEmpty`, …); не попадают в выход.
- `@` — ссылки на переменные (`@var`) и runtime globals (`@runtime.*`).
- bare — обычные ключи данных и inner-ключи тела `#if` (`and`/`or`/`value`/`else`).
- Неизвестный `#*`-ключ → warn + drop (forward-compat для будущих constructs).

---

## 5. `params[]` — патчи `config`

Применяют `value` к секции `config` по dot-path при выполнении условия.

```json
{
  "name": "route.rules",
  "if_or": ["@tun", "@enable_proxy_in"],
  "mode": "prepend",
  "value": [ { ... } ]
}
```

| Поле | Описание |
|---|---|
| `name` | dot-path в `config` (`"inbounds"`, `"route.rules"`, `"dns.servers"`). |
| `value` | применяемое значение (любой JSON); поддерживает `@var` и `#if`. |
| `mode` | `replace` (по умолч.) \| `prepend` \| `append` (для массивов). |
| `platforms` | ОС применения (пусто = все). |
| `if` / `if_or` | условие на **всю** запись — `@bool_var` (§3.3). |

Порядок при загрузке: проверка `platforms` → `if`/`if_or` → слияние `value` по `mode`
→ подстановка `@var` → обход `#if`.

---

## 6. `presets[]` — пресеты вкладки Rules

Самодостаточные наборы правил/наборов-правил/DNS/outbound'ов. Переменные пресета
имеют **локальный scope** (`@name` внутри пресета резолвится по его `vars[]`).

| Поле | Тип | Описание |
|---|---|---|
| `id` | string | Стабильный slug (`[a-z0-9_-]+`), на него ссылается state. |
| `label` | string | Название в UI. |
| `description` | string | Описание (tooltip / карточка). |
| `default_enabled` | bool | Рекомендация: включён ли на fresh install. |
| `platforms` | array | ОС доступности (пусто = все). |
| `vars` | array | Локальные переменные (`PresetVar`, §6.1). |
| `rules` | array | Routing rules (массив объектов; `@var`, `rule_set`-ссылки, свои `if`). |
| `rule_set` | array | Определения rule_set (`PresetRuleSet`, §6.2). |
| `dns_servers` | array | Bundled DNS-серверы (`PresetDNSServer`, §6.3). |
| `dns_rule` | object | DNS-rule пресета (свой `if`). |
| `outbounds` | array | Добавление/патч outbound'ов (`PresetOutbound`, §6.4). |

### 6.1 `PresetVar`

| Поле | Описание |
|---|---|
| `name` | локальное имя; `@name` в телах пресета. |
| `type` | `outbound` \| `dns_server` \| `enum` \| `text` \| `number` \| `bool`. |
| `default` | обязательный дефолт (строка; для bool — `"true"`/`"false"`). |
| `title` / `tooltip` | UI. |
| `options` | `enum` → `[{title,value}]`; `outbound`/`dns_server` → `["tag", …]` (whitelist). |
| `select` | для `dns_server`: `"local"` (только bundled пресета) \| `"global"` (все доступные, по умолч.). |
| `if` / `if_or` | условие активности (`@bool_var`). |

> Типы `secret` / `text_list` в preset-vars **не** поддерживаются (только top-level `vars`).

### 6.2 `PresetRuleSet`

```json
{"tag": "geoip-ru", "type": "remote", "format": "binary", "url": "https://…/geoip-ru.srs", "if": ["@geoip_enabled"]}
{"tag": "messengers", "type": "inline", "format": "domain_suffix", "rules": [ { ... } ]}
```
`tag` локальный (при build → `<preset_id>:<tag>`). `type`: `inline` (с `rules`) или `remote` (с `url`). Свои `if`/`if_or`.

### 6.3 `PresetDNSServer`

```json
{"tag": "yandex", "type": "https", "server": "dns.yandex", "detour": "@out", "if": ["@use_dns_override"]}
```
`type`: `udp`/`https`/`tls`/`h3`. Поля `server`, `server_port`, `path`, `tls`, `detour` (может быть `@var`),
`description`. В config попадает только если выбран через `@dns_server`-var или упомянут в `dns_rule.server`.

### 6.4 `PresetOutbound`

```json
{"mode": "add",    "tag": "auto", "type": "urltest", "options": { ... }, "if": ["@flag"]}
{"mode": "update", "tag": "proxy-out", "options": { ... }}
```
`mode`: `add` (новый, нужен `type`) \| `update` (патч существующего по `tag`). Поля зеркалят
outbound sing-box (`options`, `filters`, `addOutbounds`, `preferredDefault`, `comment`). Свои `if`/`if_or`.

---

## 7. `parser_config` / `config` / `dns_options`

- **`parser_config`** — настройки парсера подписки (`reload`, …) + статические outbound-группы
  (`selector`/`urltest` через `addOutbounds`/`filters`). Ноды из подписки инжектятся между маркерами.
- **`config`** — скелет sing-box: `route` (rules, rule_set, final, …), `experimental` (clash_api, cache),
  `inbounds`, `log` и т.д. Принимает `@var`, патчится `params`, обходится `#if`.
- **`dns_options`** — `servers[]` и `rules[]` (дефолты DNS-вкладки). Скаляры DNS задаются переменными
  `vars` с `wizard_ui: "fix"` (`dns_strategy` → `config.dns.strategy`, `dns_final` → `config.dns.final`,
  `dns_default_domain_resolver` → `config.route.default_domain_resolver`).

---

## 8. Стиль форматирования (bundled-шаблон)

Правила оформления **поставляемого** `bin/wizard_template.json`. Порядок ключей и
переносы **не влияют** на загрузку — это читаемость для maintainer'ов и аккуратные
diff'ы. Кастомные шаблоны могут игнорировать §8, но синтаксис (§1–§7) обязателен.

**Принцип:** *компактно* — литералы и мелкие metadata-объекты; *развёрнуто* —
выражения (`@…`, `#if`, outer `if[]`) и длинные списки.

### 8.1 `vars[]` и `presets[].vars[]`

| Часть | Оформление |
|---|---|
| «Шапка» (`name`, `type`, `wizard_ui`, `title`, `tooltip`, `platforms`, `comment`, `select`, …) | строка 1 |
| `default_value` / `default` (в расширенной форме) | отдельная строка |
| `options[]` | multiline — каждый элемент на своей строке |
| outer `if` / `if_or` | отдельная строка, **в конце** объекта |
| `{"separator": true}` | одна строка |
| простой preset-var (`out`, …) — всё целиком (включая `default`) | одна строка |

```jsonc
{"name": "tun_mtu", "type": "text", "wizard_ui": "edit",
  "title": "TUN MTU", "tooltip": "…",
  "default_value": "1492",
  "if": ["@tun"]
},
```

### 8.2 JSON payload (`config`, `params[].value`, `parser_config`)

| Контекст | Правило |
|---|---|
| поля с `@` | одно поле — одна строка |
| литералы (`type`, `tag`, `auto_route`, …) | можно вместе на одной строке |
| `options` **с** `@`-полями | multiline-объект |
| `options` / `filters` / `addOutbounds` **без** `@` | одна строка |
| мелкие struct'ы из литералов (≤2–3 поля: `direct-out`, hijack-dns, `mode:update`) | одна строка |
| крупные объекты (`dns_options.servers[]`, preset `dns_servers[]`, полный `mode:add` outbound) | multiline — одно поле на строку |

```jsonc
"options": {
  "url": "@urltest_url",
  "interval": "@urltest_interval",
  "tolerance": "@urltest_tolerance",
  "interrupt_exist_connections": true
},
```

### 8.3 `#if`

| `value` / `else` | Оформление |
|---|---|
| скаляр | одна строка: `"#if": {"and": [...], "value": "…", "else": "…"}` |
| объект | условие + `"value": {` на строке 1; тело ниже; закрытие `}}` (с `else` → `}, "else": {` … `}}}`) |

Скалярная ветвь (array-element) — одна строка:
```jsonc
"inbound": [
  {"#if": {"and": ["@tun"], "value": "tun-in"}},
  {"#if": {"and": ["@enable_proxy_in"], "value": "proxy-in"}}
]
```

Объектная ветвь (map-spread) — условие + `"value": {` на первой строке, тело ниже:
```jsonc
"#if": {"and": [{"@runtime.platform": {"#in": ["windows", "linux"]}}], "value": {
  "interface_name": "singbox-tun0"
}}
```

С `else` (обе ветви — объекты); массивы значений пишутся **как есть**, без выравнивания колонками:
```jsonc
{"#if": {"and": [{"@runtime.platform": "windows"}], "value": {
  "process_name": ["Telegram.exe", "Discord.exe", "WhatsApp.exe", "Signal.exe", "Zoom.exe"]
}, "else": {
  "process_name": ["Telegram", "Discord", "WhatsApp", "Signal", "zoom.us"]
}}}
```

### 8.4 Presets

| Секция | Оформление |
|---|---|
| `rules[]`, `dns_rule` | одна строка |
| `rule_set[]` (inline и remote) | строка 1: metadata (`tag`, `type`, `format`, **`if`/`if_or`**); строка 2: `rules` / `url` |
| `rule_set[].rules` inline | одна строка, если влезает; очень длинные suffix-списки — массив с переносами |
| `outbounds[]` `mode:update` | одна строка на entry |
| `outbounds[]` `mode:add` (полный outbound) | multiline-объект; `options` / `filters` — одна строка |
| `params[]` route.rules (скалярные `#if`) | правило целиком в одну строку |

> В отличие от `vars[]` (где `if` на отдельной строке в конце), у **condensed** объектов с metadata в одну строку (`rule_set[]`) `if`/`if_or` ставится **на этой же строке**.

```jsonc
{"tag": "geoip-ru", "type": "remote", "format": "binary", "if": ["@geoip_enabled"],
  "url": "https://…/geoip-ru.srs"}
```

### 8.5 Шпаргалка

| | Одна строка | Multiline |
|---|---|---|
| var-«шапка» | ✓ | — |
| простой preset-var (вкл. `default`) | ✓ | — |
| `default_value` / `default` (расш. форма) | — | ✓ |
| элементы `options[]` | — | ✓ |
| outer `if[]` в vars | — | ✓ (в конце) |
| `rule_set[]` metadata (вкл. `if`) | ✓ | `rules` / `url` ниже |
| `@` в payload | — | ✓ (по полю) |
| мелкий literal-struct (≤2–3 поля) | ✓ | — |
| крупный config-объект (`dns_servers[]`, `mode:add`) | — | ✓ |
| `#if` + object `value` | условие | тело |
| `#if` + скаляр | ✓ | — |
| `filters`, literal `options` | ✓ | — |

---

## 9. Ошибки загрузки (частые)

| Симптом | Причина |
|---|---|
| `bare var-ref "x" in if[]; use "@x"` | элемент `if`/`if_or` без `@`. |
| `vars[].name "runtime" is reserved` | имя занято namespace'ом `@runtime.*`. |
| `@x is not declared in vars` | `@var` в `config`/`params` без объявления в `vars`. |
| `#if has both "and" and "or"` | в теле `#if` оба — допустим один. |
| `#if missing "value"` | в теле `#if` нет обязательного `value`. |
| `predicate key "x" must start with @ or be #not` | неверная форма predicate'а (var — это ключ `@var`, предикат — значение). |
| `unknown runtime global @runtime.x` | поле namespace вне `platform`/`arch`. |

После изменения шаблона в репозитории — пересобрать/переустановить приложение
или скопировать файл в `Contents/MacOS/bin/`, иначе правки не подхватятся.
