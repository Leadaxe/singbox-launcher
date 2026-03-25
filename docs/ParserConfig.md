# Документация парсера подписок singbox-launcher

## Назначение

Парсер обновляет файл `bin/config.json`, загружая подписки (поддерживаются протоколы: VLESS, VMess, Trojan, Shadowsocks, Hysteria2, SSH, WireGuard), фильтруя и группируя их в селекторы. Результат записывается в секции между маркерами `/** @ParserSTART */` и `/** @ParserEND */` (outbounds), а узлы WireGuard — между `/** @ParserSTART_E */` и `/** @ParserEND_E */` (endpoints). Секция **endpoints** (WireGuard) поддерживается в sing-box начиная с версии **1.11**.

## Документы и исходный код парсера URI

| Документ / место | Содержание |
|------------------|------------|
| **Этот файл** (`docs/ParserConfig.md`) | Форматы прямых ссылок в `connections`, Share URI, структура ParserConfig, пайплайн обновления. |
| **`SPECS/023-F-C-SUBSCRIPTION_TRANSPORT_VLESS_TROJAN/SUBSCRIPTION_PARAMS_REPORT.md`** | Таблицы: query VLESS/Trojan → поля sing-box; примеры из публичных подписок; ключи query. |
| **`SPECS/029-Q-С-SUBSCRIPTION_PARSER_CLASH_CONVERTOR_PARITY/SPEC.md`** | Расширения совместимости (029): `type=httpupgrade`, `peer`, `obfsParam`, VMess legacy / `httpupgrade` / `h2`, Hysteria2 TLS; сверка со схемой sing-box. |
| Пакет **`core/config/subscription`** | `ParseNode`, `buildOutbound` — `node_parser.go`; VLESS/Trojan transport+TLS — `node_parser_transport.go`; VMess — `node_parser_vmess.go` (`parseVMessDecoded`, `parseVMessJSON`, `parseVMessLegacyCleartext`); Hysteria2 — `node_parser_hysteria2.go`; WireGuard / SSH — `node_parser_wireguard.go`, `node_parser_ssh.go`; share URI — `share_uri_encode.go`. |

## Share URI из outbound и WireGuard endpoint (обратно к ссылке)

Спецификация фичи (ПКМ на вкладке Servers, контекстное меню, детали реализации): **`SPECS/025-F-C-SERVERS_CONTEXT_MENU_SHARE_URI/`** (SPEC, PLAN, IMPLEMENTATION_REPORT).

Парсер переводит **строку подписки** (`ParseNode` → `buildOutbound` или для WireGuard — объект в `endpoints[]`) в JSON sing-box. Обратная операция — **сборка share URI из уже записанного outbound или WireGuard endpoint** в `config.json`, чтобы делиться ссылкой без хранения исходной строки подписки.

### Принцип и соответствие форматам

- **Вход кодировщика:** один элемент массива `outbounds` **или** один элемент `endpoints[]` с `type: wireguard` (тот же набор полей, что даёт `parseWireGuardURI` / `GenerateEndpointJSON`).
- **Выход:** одна строка URI в форматах, которые снова понимает этот проект: `vless://`, `vmess://` (base64 JSON), `trojan://`, `ss://` (SIP002), `socks5://`, `hysteria2://`, `ssh://`, **`wireguard://`**.
- **Query / transport / TLS:** для VLESS и Trojan при кодировании используются те же соглашения, что и при разборе (`uriTransportFromQuery`, `vlessTLSFromNode`, `trojanTLSFromNode` в `node_parser_transport.go`). VMess при разборе не использует стандартный URI-query в основном формате (JSON в base64); legacy и поля JSON — в `node_parser_vmess.go`. Подробный справочник VLESS/Trojan: **`SUBSCRIPTION_PARAMS_REPORT.md`** (023); расширения 029 — спека **`029-Q-С-…/SPEC.md`** и разделы URI ниже.

### API в коде

| Функция | Пакет | Назначение |
|--------|--------|------------|
| `ShareURIFromOutbound(out map[string]interface{})` | `core/config/subscription` (`share_uri_encode.go`) | Кодирование из JSON-объекта outbound; для `type: wireguard` делегирует в `ShareURIFromWireGuardEndpoint` |
| `ShareURIFromWireGuardEndpoint(ep map[string]interface{})` | `core/config/subscription` (`share_uri_encode.go`) | Кодирование `wireguard://` из одного endpoint (один peer в `peers[]`) |
| `GetOutboundMapByTag(configPath, tag)` | `core/config` (`outbound_share.go`) | Поиск outbound по полю `tag` в `config.json` |
| `GetEndpointMapByTag(configPath, tag)` | `core/config` (`outbound_share.go`) | Поиск endpoint по полю `tag` в `endpoints[]` |
| `ShareProxyURIForOutboundTag(configPath, tag)` | `core/config` (`outbound_share.go`) | Сначала outbound по тегу, иначе WireGuard в `endpoints[]` |

Ошибка **`ErrShareURINotSupported`** (`subscription`) — тип outbound не кодируется в один URI или не хватает полей.

### Поддерживаемые типы `outbound.type`

| `type` в JSON | Схема URI | Замечания |
|---------------|-----------|-----------|
| `vless` | `vless://` | `encryption=none`, transport/TLS как в подписках |
| `vmess` | `vmess://` + base64 | Поля JSON узла согласованы с `parseVMessJSON` |
| `trojan` | `trojan://` | Пароль в userinfo |
| `shadowsocks` | `ss://` | SIP002, base64(`method:password`) |
| `socks` | `socks5://` | `version` 5; user/password при наличии |
| `hysteria2` | `hysteria2://` | TLS SNI, `mport`, obfs и т.д. по возможности |
| `ssh` | `ssh://` | **Нет** кодирования inline `private_key` в URI; путь к ключу и прочие поля — в query, как в документации SSH URI |
| `wireguard` | `wireguard://` | Обычно узел только в `endpoints[]`; формат и query — раздел **WireGuard** ниже. **Один URI ↔ один удалённый peer:** при нескольких элементах в `peers[]` кодирование не поддерживается (`ErrShareURINotSupported`). |

**Не кодируются в один share URI:** `selector`, `urltest`, `direct`, `block`, `dns`, произвольные служебные типы; WireGuard с **несколькими** `peers`.

### GUI

Вкладка **Servers** (список прокси Clash API): **ПКМ** по строке → `serversProxyContextMenu`: первая строка — **`api.ProxyInfo.ContextMenuTypeLine`** (нижний регистр поля **`type`** из API или `servers.menu_context_type_unknown`); затем **«Копировать ссылку»** (`servers.menu_copy_link`). Верхняя строка без `Disabled`, `Action: nil` (цвет текста как у обычного пункта меню). В буфер попадает строка через `config.ShareProxyURIForOutboundTag` и путь `FileService.ConfigPath`: сначала outbound по тегу, иначе WireGuard в `endpoints[]`. Правый клик по кнопкам Ping/Switch может не открыть меню (иерархия hit-test Fyne). Сообщения статуса: `servers.copy_link_resolving`, `servers.copy_link_done`, `servers.copy_link_not_supported`.

### Тесты

Round-trip и выборочные сценарии: `core/config/subscription/share_uri_encode_test.go`, интеграция с файлом конфига: `core/config/outbound_share_test.go`.

## Версионирование конфигурации

Парсер использует систему версионирования для управления изменениями в структуре конфигурации:

- **Версия 1** (устарела): версия находилась на верхнем уровне JSON
- **Версия 2** (устарела): версия перемещена внутрь `ParserConfig`, появился вложенный объект `outbounds` с полями `proxies`, `addOutbounds`, `preferredDefault`
- **Версия 3** (устарела): плоская структура, поля `filters`, `addOutbounds` и `preferredDefault` на верхнем уровне объекта outbound
- **Версия 4** (текущая): добавлена поддержка локальных outbounds в `ProxySource` и префиксов/постфиксов для тегов узлов

## Формат конфигурации

В файле `bin/config.json` должен быть блок комментария `/** @ParserConfig ... */`, внутри которого размещается JSON следующей структуры:

```json
{
  "ParserConfig": {
    "version": 4,
    "proxies": [...],
    "outbounds": [...],
    "parser": {
      "reload": "4h",
      "last_updated": "2025-12-16T03:21:19Z"
    }
  }
}
```

## Полный пример конфигурации с комментариями

```json
{
  /** @ParserConfig
  {
    "ParserConfig": {
      // Версия конфигурации (текущая: 4)
      "version": 4,
      
      // Список источников прокси-серверов
      "proxies": [
        {
          // URL подписки (Base64 или plain-текст)
          // Поддерживаются: VLESS, VMess, Trojan, Shadowsocks, Hysteria2, SOCKS5, WireGuard
          "source": "https://your-subscription-url.com/subscription",
          
          // Прямые ссылки на прокси-серверы (необязательно)
          // Можно комбинировать с подписками
          "connections": [
            "vless://uuid@server.com:443?security=reality&sni=example.com&fp=chrome&pbk=...&sid=...&type=tcp#🇳🇱 Netherlands",
            "vmess://eyJ2IjoiMiIsInBzIjoi...",
            "trojan://password@server.com:443?security=tls&sni=example.com#🇺🇸 United States",
            "hysteria2://password@server.com:443?sni=example.com&insecure=1#🇺🇸 United States",
            "hy2://password@server.com:443?sni=example.com#🇺🇸 United States (short form)",
            "ssh://root:admin@127.0.0.1:22#Local SSH",
            "socks5://user:pass@proxy.example.com:1080#Office SOCKS5",
            "wireguard://privatekey@10.0.0.1:51820?publickey=...&address=10.10.10.2/32&allowedips=0.0.0.0/0,::/0#WireGuard VPN"
          ],
          
          // Фильтры для исключения узлов (необязательно)
          // Если хотя бы один фильтр совпал - узел пропускается
          "skip": [
            { "tag": "/🇷🇺/i" },  // Исключить все узлы с тегом содержащим 🇷🇺
            { "host": "/test\\./i" } // Исключить узлы с host содержащим "test."
          ],
          
          // Префикс для всех тегов узлов из этого источника (необязательно, версия 4)
          // Добавляется перед оригинальным тегом узла
          // Визард автоматически добавляет "1:", "2:", "3:" и т.д. при наличии нескольких подписок
          // Поддерживает переменные: {$tag}, {$scheme}, {$protocol}, {$server}, {$port}, {$label}, {$comment}, {$num}
          // Пример: "tag_prefix": "{$num} {$protocol}:" → "1 vless:", "2 vmess:" и т.д.
          // Игнорируется, если указан tag_mask
          "tag_prefix": "1:",
          
          // Постфикс для всех тегов узлов из этого источника (необязательно, версия 4)
          // Добавляется после оригинального тега узла
          // Поддерживает те же переменные, что и tag_prefix
          // Игнорируется, если указан tag_mask
          "tag_postfix": "--xx",
          
          // Маска для полной замены тега узла (необязательно, версия 4)
          // Если указан, полностью заменяет тег узла, игнорируя tag_prefix и tag_postfix
          // Поддерживает те же переменные, что и tag_prefix/tag_postfix
          // Пример: "tag_mask": "{$num} {$protocol} : {$label}" → "1 vless : United States, New York"
          "tag_mask": "",
          
          // Локальные outbounds для этого источника (необязательно, версия 4)
          // Применяются только к узлам из этого источника
          // Теги локальных outbounds автоматически добавляются в список доступных outbounds
          // на второй вкладке (Rules) визарда, что позволяет использовать их в правилах маршрутизации
          "outbounds": [
            {
              "tag": "local-selector",
              "type": "selector",
              "filters": { "tag": "/source1-/i" },
              "comment": "Local selector for this source"
            }
          ]
        },
        {
          // Можно добавить несколько источников
          "source": "https://another-subscription-url.com/sub",
          "connections": [],
          "skip": []
        }
      ],
      
      // Список селекторов (групп прокси)
      "outbounds": [
        {
          // Имя селектора (обязательно)
          // Используется в UI Clash API таба для переключения прокси
          "tag": "proxy-out",
          
          // Тип селектора (обязательно)
          // Поддерживается: "selector", "urltest"
          "type": "selector",
          
          // Дополнительные опции для селектора (необязательно)
          // Эти поля добавляются как верхнеуровневые ключи в итоговый JSON селектора
          "options": {
            "interrupt_exist_connections": true,  // Прервать существующие соединения при переключении
            "default": "auto-proxy-out"            // Тег узла по умолчанию (если не указан preferredDefault)
          },
          
          // Главный фильтр для выбора узлов (версия 4, необязательно)
          // Логика: OR между объектами в массиве, AND между ключами внутри объекта
          // В версии 2 называлось "outbounds.proxies"
          "filters": {
            // Исключить все узлы с тегом содержащим 🇷🇺 или 🇺🇸
            "tag": "!/(🇷🇺|🇺🇸)/i"
          },
          
          // Список тегов, которые добавляются в начало списка outbounds селектора (необязательно)
          // Полезно для добавления "direct-out", "reject" и других статических outbounds
          // В версии 2 называлось "outbounds.addOutbounds"
          "addOutbounds": ["direct-out", "auto-proxy-out"],
          
          // Фильтр для определения узла по умолчанию (необязательно)
          // Первый узел, совпавший с фильтром, станет значением поля "default" в селекторе
          // В версии 2 называлось "outbounds.preferredDefault"
          "preferredDefault": {
            "tag": "/🇳🇱/i"  // Выбрать узел с тегом содержащим 🇳🇱 как default
          },
          
          // Комментарий, который будет выведен перед JSON селектора (необязательно)
          "comment": "Proxy group for international connections"
        },
        {
          // Пример селектора типа urltest (автоматический выбор лучшего узла)
          "tag": "auto-proxy-out",
          "type": "urltest",
          "options": {
            "url": "https://cp.cloudflare.com/generate_204",  // URL для тестирования
            "interval": "5m",                                 // Интервал проверки
            "tolerance": 100,                                 // Допустимое отклонение (мс)
            "interrupt_exist_connections": true                // Прервать соединения при переключении
          },
          "filters": {
            "tag": "!/(🇷🇺)/i"  // Исключить узлы с 🇷🇺
          },
          "comment": "Proxy automated group for everything that should go through VPN"
        }
      ],
      
      // Настройки парсера (необязательно, устанавливаются автоматически)
      "parser": {
        "reload": "4h",                    // Интервал автоматического обновления (по умолчанию "4h")
        "last_updated": "2025-12-16T03:21:19Z"  // Время последнего обновления (RFC3339, UTC, обновляется автоматически)
      }
    }
  }
  */
}
```

## Описание полей

### Секция `proxies`

Массив объектов, описывающих источники прокси-серверов.

| Поле          | Тип      | Обязательное | Описание |
|---------------|----------|--------------|----------|
| `source`      | string   | Да           | URL подписки (поддерживаются протоколы: VLESS, VMess, Trojan, Shadowsocks, Hysteria2, SSH, WireGuard). Допускаются Base64 и plain-текст. |
| `connections` | array    | Нет          | Массив прямых ссылок (vless://, vmess://, trojan://, ss://, hysteria2://, ssh://, socks5:// или socks://, wireguard://). Можно комбинировать с подписками. Узлы WireGuard попадают в секцию `endpoints` конфига (требуется sing-box 1.11+). Подробнее о форматах URI см. раздел [Форматы URI для прямых ссылок](#форматы-uri-для-прямых-ссылок). |
| `skip`        | array    | Нет          | Список фильтров. Если хотя бы один совпал — узел пропускается. |
| `tag_prefix`  | string   | Нет          | Префикс, добавляемый ко всем тегам узлов из этого источника (версия 4). Применяется перед оригинальным тегом. Поддерживает переменные: `{$tag}`, `{$scheme}`, `{$protocol}`, `{$server}`, `{$port}`, `{$label}`, `{$comment}`, `{$num}`. Игнорируется, если указан `tag_mask`. |
| `tag_postfix` | string   | Нет          | Постфикс, добавляемый ко всем тегам узлов из этого источника (версия 4). Применяется после оригинального тега. Поддерживает те же переменные, что и `tag_prefix`. Игнорируется, если указан `tag_mask`. |
| `tag_mask`    | string   | Нет          | Маска для полной замены тега узла (версия 4). Если указан, полностью заменяет тег узла, игнорируя `tag_prefix` и `tag_postfix`. Поддерживает те же переменные, что и `tag_prefix`/`tag_postfix`. |
| `outbounds`   | array    | Нет          | Локальные outbounds для этого источника (версия 4). Применяются только к узлам из этого источника. Теги локальных outbounds автоматически добавляются в список доступных outbounds на второй вкладке (Rules) визарда, что позволяет использовать их в правилах маршрутизации. |
| `exclude_from_global` | bool | Нет | Если `true`, узлы этого источника **не** попадают в пул для **глобальных** записей `ParserConfig.outbounds` при генерации конфига. Локальные `proxies[i].outbounds` по-прежнему используют только узлы этого источника. Поле с `omitempty`; только поведение генератора, глобальный JSON не меняется. |
| `expose_group_tags_to_global` | bool | Нет | Если `true`, при генерации теги **помеченных** визардом локальных групп (см. ниже) **добавляются** к эффективному списку исходящих **каждой** глобальной записи `ParserConfig.outbounds`. Сохранённый массив `outbounds[].addOutbounds` **не** переписывается. Строки из пользовательского `addOutbounds` по-прежнему **не** фильтруются через `filters`; подмешиваемые теги проходят те же `filters`, что и узлы (синтетическое сопоставление по `tag`/`comment`). |

На первой вкладке визарда (**Sources**) кнопка **Edit** у источника открывает окно с подвкладками **Настройки** (префикс, локальные auto/select, оба флага), **Просмотр** (список локальных `proxies[i].outbounds` и узлов подписки) и **JSON** (только чтение: весь объект `proxies[i]`).

#### Локальные группы визарда (`WIZARD:` в `comment`) и глобальная генерация

Визард может создавать в `proxies[i].outbounds` записи с подстроками в поле **`comment`**:

- **`WIZARD:auto`** — локальный urltest (тег обычно `trim(tag_prefix)+"auto"`).
- **`WIZARD:select`** или **`WIZARD:selector`** — локальный selector с `default` на auto и `addOutbounds`, содержащим тег auto.

Поля **`exclude_from_global`** и **`expose_group_tags_to_global`** независимы. **`expose`** учитывает только исходящие с указанными маркерами в `comment` и включённым флагом **`expose_group_tags_to_global`** на том же элементе `proxies[]`.

#### Префиксы, постфиксы и маски тегов (версия 4)

Поля `tag_prefix`, `tag_postfix` и `tag_mask` позволяют автоматически модифицировать теги узлов из конкретного источника. Это полезно для:

- Группировки узлов по источникам в тегах
- Упрощения фильтрации в селекторах
- Избежания конфликтов тегов между разными источниками
- Полной замены формата тегов через `tag_mask`

**Автоматическое добавление префиксов:**
При использовании визарда конфигурации, если для подписки ещё не задан `tag_prefix` (новый источник или не было сохранено в конфиге), порядок такой:
1. **Фрагмент URL** — если в ссылке на подписку есть часть после `#` (например `https://host/list.json#abvpn`), визард подставляет `tag_prefix` из этого фрагмента: пробелы по краям и управляющие символы убираются, при необходимости применяется процент-декодирование; если строка не заканчивается на `:`, к ней добавляется `:` (как у числовых префиксов `1:`).
2. Иначе — **порядковый номер** в формате `"1:"`, `"2:"`, `"3:"` и т.д. (общая нумерация по всем источникам: подписки, затем блок connections).

Если `tag_prefix` для данного URL уже был в сохранённом `ParserConfig`, он **восстанавливается** и не заменяется ни фрагментом, ни номером.

**Порядок применения:**
1. Узел парсится с оригинальным тегом (например, `"🇷🇺 Moscow"`)
2. Если указан `tag_mask`, он полностью заменяет тег с подстановкой переменных (этапы 3-4 пропускаются)
3. Если `tag_mask` не указан:
   - Применяется `tag_prefix` (если указан) с подстановкой переменных.
   - Применяется `tag_postfix` (если указан) с подстановкой переменных.
4. Тег проверяется на уникальность (через `MakeTagUnique`) (добавляется суффикс `-N` при дубликатах)

**Поддерживаемые переменные:**

В `tag_prefix`, `tag_postfix` и `tag_mask` можно использовать следующие переменные:

| Переменная | Описание | Пример значения |
|------------|----------|-----------------|
| `{$tag}` | Оригинальный тег узла | `"🇷🇺 Moscow"` |
| `{$scheme}` или `{$protocol}` | Протокол узла | `"vless"`, `"vmess"`, `"trojan"`, `"ss"`, `"hysteria2"` |
| `{$server}` | Адрес сервера | `"example.com"`, `"192.168.1.1"` |
| `{$port}` | Порт сервера (число) | `"443"`, `"8080"` |
| `{$label}` | Метка из URL (фрагмент после `#`) | `"United States, New York"` |
| `{$comment}` | Комментарий узла | `"United States, New York"` |
| `{$num}` | Порядковый номер узла (начиная с 1) | `"1"`, `"2"`, `"3"` |

**Примеры:**

Автоматический формат (визард добавляет при нескольких подписках):
```json
{
  "source": "https://example.com/subscription1",
  "tag_prefix": "1:"
},
{
  "source": "https://example.com/subscription2",
  "tag_prefix": "2:"
}
```

Ручной формат:
```json
{
  "source": "https://example.com/subscription",
  "tag_prefix": "source1-",
  "tag_postfix": "--xx"
}
```

Использование переменных:
```json
{
  "connections": [
    "vless://uuid@server.com:443#🇷🇺 Moscow",
    "vmess://...",
    "hysteria2://password@server.com:443#🇺🇸 New York"
  ],
  "tag_prefix": "{$num} {$protocol}:"
}
```

Результат:
- Для первого узла (vless): `"1 vless:🇷🇺 Moscow"`
- Для второго узла (vmess): `"2 vmess:..."`  
- Для третьего узла (hysteria2): `"3 hysteria2:🇺🇸 New York"`

Другие примеры с переменными:
```json
{
  "tag_prefix": "[{$protocol}] {$server}:{$port} - ",
  "tag_postfix": " ({$label})"
}
```

Если узел имел тег `"🇷🇺 Moscow"`, сервер `"example.com"`, порт `443`, протокол `"vless"`, то итоговый тег будет:
- `"[vless] example.com:443 - 🇷🇺 Moscow (United States, Moscow)"`

**Использование tag_mask:**

`tag_mask` позволяет полностью заменить тег узла, игнорируя `tag_prefix` и `tag_postfix`:

```json
{
  "connections": [
    "vless://uuid@server.com:443#🇷🇺 Moscow",
    "vmess://...",
    "hysteria2://password@server.com:443#🇺🇸 New York"
  ],
  "tag_mask": "{$num} {$protocol} : {$label}"
}
```

Результат:
- Для первого узла (vless): `"1 vless : 🇷🇺 Moscow"`
- Для второго узла (vmess): `"2 vmess : ..."`  
- Для третьего узла (hysteria2): `"3 hysteria2 : 🇺🇸 New York"`

**Важно:** Если указан `tag_mask`, параметры `tag_prefix` и `tag_postfix` полностью игнорируются.

#### Поддерживаемые ключи фильтров

- `tag` — имя тега (с учётом регистра и эмодзи)
- `host` — hostname узла
- `label` — исходная строка после `#` в URI
- `scheme` — схема протокола (`vless`, `vmess`, `trojan`, `ss`)
- `fragment` — URI фрагмент (равен `label`)
- `comment` — правая часть `label` после `|`

#### Формат `pattern` в фильтрах

- `"literal"` — подстрочное совпадение, учитывает регистр
- `"!literal"` — отрицание (исключить узлы с таким значением)
- `"/regex/i"` — регулярное выражение с флагом `i` (игнорировать регистр)
- `"!/regex/i"` — отрицание регулярного выражения

**Примеры:**
```json
"skip": [
  { "tag": "!/🇷🇺/i" },           // Исключить все узлы с тегом содержащим 🇷🇺
  { "host": "/test\\./i" },        // Исключить узлы с host содержащим "test."
  { "scheme": "trojan" },          // Исключить все Trojan узлы
  { "label": "/Netherlands/i" }   // Исключить узлы с label содержащим "Netherlands"
]
```

### Секция `outbounds`

Массив объектов, описывающих селекторы (группы прокси).

| Поле              | Тип      | Обязательное | Описание |
|-------------------|----------|--------------|----------|
| `tag`             | string   | Да           | Имя селектора. Используется в UI Clash API таба для переключения прокси. |
| `type`            | string   | Да           | Тип селектора: `"selector"` (ручной выбор) или `"urltest"` (автоматический выбор лучшего). |
| `options`         | object   | Нет          | Дополнительные поля, добавляются как верхнеуровневые ключи в результат. |
| `filters`         | object   | Нет          | Главный фильтр для выбора узлов (версия 4). OR между объектами в массиве, AND между ключами внутри объекта. В версии 2 называлось `outbounds.proxies`. |
| `addOutbounds`    | array    | Нет          | Строки, которые добавляются в начало итогового списка outbounds (например `"direct-out"`). В версии 2 называлось `outbounds.addOutbounds`. |
| `preferredDefault`| object   | Нет          | Фильтр для определения узла по умолчанию. Первый узел, совпавший с фильтром, станет значением поля `default` в селекторе. В версии 2 называлось `outbounds.preferredDefault`. |
| `comment`         | string   | Нет          | Комментарий, выводится перед JSON селектора в результирующем файле. |
| `wizard`          | string/object | Нет          | Параметр для скрытия outbound в визарде и управления обязательностью. Поддерживает два формата:<br/>- **Старый формат (обратная совместимость)**: `"wizard": "hide"` — скрывает outbound из списка доступных outbounds на второй вкладке (Rules) визарда<br/>- **Новый формат**: `"wizard": {"hide": true, "required": 2}` — объект с полями `hide` (boolean) и `required` (int). Поле `required` может иметь значения: `0` или отсутствует — игнорировать; `1` — проверить только наличие тега (если отсутствует, добавить из шаблона); `>1` (например, `2`) — строгое соответствие шаблону (если отсутствует или не совпадает, заменить/добавить из шаблона). |

#### Логика фильтрации в `filters`

Фильтр `filters` работает следующим образом:

1. **AND логика внутри объекта**: все ключи в объекте должны совпасть
   ```json
   "filters": {
     "tag": "/🇳🇱/i",      // И тег должен содержать 🇳🇱
     "host": "/example/i"  // И host должен содержать "example"
   }
   ```

2. **OR логика между объектами** (если `filters` - массив):
   ```json
   "filters": [
     { "tag": "/🇳🇱/i" },   // ИЛИ тег содержит 🇳🇱
     { "tag": "/🇺🇸/i" }    // ИЛИ тег содержит 🇺🇸
   ]
   ```

3. **Если `filters` не указан**: в селектор попадают все узлы (кроме исключенных через `skip`)

#### Примеры использования `filters`

```json
// Исключить узлы с 🇷🇺 или 🇺🇸
"filters": {
  "tag": "!/(🇷🇺|🇺🇸)/i"
}

// Включить только узлы с 🇳🇱
"filters": {
  "tag": "/🇳🇱/i"
}

// Включить узлы с 🇳🇱 И host содержащим "example"
"filters": {
  "tag": "/🇳🇱/i",
  "host": "/example/i"
}

// Включить узлы с 🇳🇱 ИЛИ 🇺🇸 (если filters - массив)
"filters": [
  { "tag": "/🇳🇱/i" },
  { "tag": "/🇺🇸/i" }
]
```

### Секция `parser`

Настройки парсера (необязательно, устанавливаются автоматически).

| Поле          | Тип      | Обязательное | Описание |
|---------------|----------|--------------|----------|
| `reload`      | string   | Нет          | Интервал автоматического обновления. По умолчанию `"4h"`. Формат: `"1h"`, `"30m"`, `"24h"` и т.д. |
| `last_updated`| string   | Нет          | Время последнего обновления в формате RFC3339 (UTC). Обновляется автоматически при каждом обновлении конфигурации. |

## Процесс обновления конфигурации

Когда вы нажимаете кнопку **"Update Config"** на вкладке "Core" (или используете Config Wizard):

1. **Извлечение конфигурации**
   - Парсер находит блок `@ParserConfig` в `config.json`
   - Извлекает JSON конфигурации
   - Определяет версию конфигурации

2. **Загрузка подписок**
   - Для каждого URL из `proxies[].source`:
     - Скачивается содержимое подписки (поддерживаются Base64 и plain-текст)
     - Декодируется и парсится список прокси-серверов
   - Для каждой прямой ссылки из `proxies[].connections`:
     - Парсится прямая ссылка (vless://, vmess://, trojan://, ss://, hysteria2:// или hy2://, ssh://, socks5:// или socks://, wireguard://) и добавляется в список прокси

3. **Поддерживаемые протоколы**
   - ✅ VLESS
   - ✅ VMess
   - ✅ Trojan
   - ✅ Shadowsocks (SS)
   - ✅ Hysteria2
   - ✅ SSH
   - ✅ SOCKS5 (socks5://, socks:// — outbound type "socks")
   - ✅ WireGuard (попадает в секцию endpoints; sing-box 1.11+)

4. **Извлечение информации**
   - Из каждого URI извлекается:
     - **Тег (tag)**: левая часть комментария до `|` (например, `🇳🇱Нидерланды`)
     - **Комментарий (comment)**: весь текст после `#` в URI
     - **Параметры подключения**: сервер, порт, UUID, TLS настройки и т.д.

5. **Фильтрация узлов**
   - Применяются фильтры `skip` из `proxies[]` - исключаются узлы
   - Применяются фильтры `filters` из `outbounds[]` - выбираются узлы для каждого селектора
   - Узлы с дублирующимися тегами автоматически переименовываются (добавляется суффикс `-2`, `-3` и т.д.)

6. **Генерация JSON узлов**
   - Узлы VLESS/VMess/Trojan/SS/Hysteria2/SSH/SOCKS5 сериализуются в outbounds; узлы WireGuard — в endpoints (sing-box 1.11+)
   - Комментарии выводятся из `label`
   - Порядок полей оптимизирован для читаемости

7. **Генерация селекторов**
   - Селекторы создаются согласно `outbounds[]`
   - Комментарии берутся из поля `comment`
   - Порядок полей фиксирован: `tag`, `type`, `outbounds`, `default`, `interrupt_exist_connections`, остальные
   - `addOutbounds` добавляются в начало списка `outbounds`
   - `preferredDefault` определяет значение поля `default`

8. **Запись результата**
   - Блок между маркерами `/** @ParserSTART */` и `/** @ParserEND */` заменяется на новый контент (outbounds)
   - Блок между `/** @ParserSTART_E */` и `/** @ParserEND_E */` — на сгенерированные endpoints (WireGuard), если маркеры присутствуют в конфиге
   - Обновляется поле `last_updated` в секции `parser`
   - Все операции выполняются в одном проходе (одно чтение, одна запись файла)

## Форматы URI для прямых ссылок

Парсер поддерживает прямые ссылки в массиве `connections`. Формат зависит от протокола:

### VLESS (`vless://`)
Стандартный URI формат: `vless://uuid@server:port?params#tag`

**Соответствие query → полям outbound sing-box** (TLS, [V2Ray transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/), Reality, `security=none`, нормализация ключей): подробный справочник и таблицы — в репозитории `SPECS/023-F-C-SUBSCRIPTION_TRANSPORT_VLESS_TROJAN/SUBSCRIPTION_PARAMS_REPORT.md` (раздел «Справочник» и § 1а).

**Параметры query string (типичные):**
- `encryption` — в ссылках Xray часто `none`; в JSON outbound VLESS отдельным полем не дублируется
- `flow` — подпротокол VLESS в sing-box (например `xtls-rprx-vision`), см. [доку VLESS](https://sing-box.sagernet.org/configuration/outbound/vless/). Если в ссылке **нет** `flow`, но задан **REALITY** (`pbk` + обычно `sid`) и транспорт **не** `ws` / `grpc` / `http` / `xhttp` / `httpupgrade` (только «голый» TCP), в outbound подставляется `flow: xtls-rprx-vision` — многие серверы без этого не поднимают сессию.
- `security` — `none` | `tls` | `reality`; при `none` TLS в outbound не добавляется
- `sni` — имя для SNI / проверки сертификата → `tls.server_name`; при пустом `sni` используется **`peer`** (тот же смысл в части подписок)
- `fp` — отпечаток uTLS → `tls.utls.fingerprint` (значения: `chrome`, `firefox`, `qq`, `random`, …)
- `alpn` — список через запятую → `tls.alpn`
- `insecure`, `allowInsecure` / `allowinsecure` — при `1` / `true` → `tls.insecure`
- `pbk`, `sid` — Reality → `tls.reality.public_key`, `short_id`
- `type` — транспорт: `tcp` / `raw`, `ws`, `grpc`, `http`, `xhttp`, **`httpupgrade`** (синоним `xhttp` → sing-box `httpupgrade`), реже `quic`
- `path` — путь WebSocket / HTTP / httpupgrade или fallback имени сервиса для gRPC
- `host` / `Host` — для WS → заголовок `Host`; если `host` и `sni` в query нет, для WS используется **`obfsParam`**. Если есть `host` или `sni`, они имеют приоритет. Для HTTP/httpupgrade — поле `host` транспорта (регистр ключа `Host` в query учитывается)
- `headerType` — вместе с `type=raw` или `tcp` и значением `http` задаёт транспорт типа HTTP (обфускация), см. отчёт 023
- `serviceName` / `service_name` — имя gRPC-сервиса → `transport.service_name`
- `packetEncoding` — например `xudp` → поле outbound `packet_encoding`, см. [доку VLESS](https://sing-box.sagernet.org/configuration/outbound/vless/)
- `mode`, `spx`, `extra`, `quicSecurity`, `authority` — часто встречаются в ссылках Xray/панелей; в документированный клиентский JSON sing-box **не переносятся**, на разбор ссылки не влияют

**Пример:**
```
vless://uuid@server.com:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=example.com&fp=chrome&pbk=...&sid=...&type=tcp#🇳🇱 Netherlands
```

### VMess (`vmess://`)
**⚠️ Особенность:** обычно VMess — base64(JSON); поддерживается и **legacy**-строка после base64: `method:uuid@host:port` с опциональным `?query` (как в части клиентов). Фрагмент `#tag` отрезается **до** декодирования base64.

Формат: `vmess://base64(json)` или `vmess://base64(cleartext)#tag`

JSON должен содержать поля:
- `v` - версия (обычно `"2"`)
- `ps` - название/тег
- `add` - адрес сервера
- `port` - порт
- `id` - UUID клиента
- `aid` - alterId (опционально)
- `scy` - метод шифрования (опционально)
- `net` - тип сети (`tcp`, `ws`, `http`, `grpc`, **`xhttp`/`httpupgrade`** → sing-box transport `httpupgrade`; **`h2`** → transport `http` + TLS по схеме sing-box)
- `type` - тип заголовка (для `tcp`)
- `host` - хост (для `ws`/`http`; для WS при пустом `host` подставляется SNI из TLS, если есть)
- `path` - путь (для `ws`/`http`/`grpc`)
- `tls` - использование TLS (`"tls"` или отсутствует)
- `sni` - SNI (опционально)
- `alpn` - ALPN (опционально)
- `fp` - fingerprint (опционально)
- `insecure` в JSON (`"1"`) — небезопасный TLS, как у VLESS

**Сборка outbound с TLS для VMess:** `tls.server_name` берётся из `sni`, при отсутствии — из поля **`peer`** в query (если провайдер продублировал имя в `peer`), иначе — **адрес сервера** (`add`). Флаги **`insecure` / `allowInsecure` / `allowinsecure`** в query обрабатываются так же, как для VLESS (`tlsInsecureTrue`).

**Legacy (не JSON):** в query допускаются, например, `type=ws`, `path`, `tls=1` — они маппятся в `transport` и `tls` так же, как у URI-протоколов с query.

**Пример:**
```
vmess://eyJ2IjoiMiIsInBzIjoiVGVzdCIsImFkZCI6InNlcnZlci5jb20iLCJwb3J0Ijo0NDMsImlkIjoi dXVpZCIsImFpZCI6MCwic2N5IjoiYXV0byIsIm5ldCI6InRjcCIsInR5cGUiOiJub25lIiwidGxzIjoiIn0=
```

### Trojan (`trojan://`)
Стандартный URI формат: `trojan://password@server:port?params#tag`

Те же правила **TLS** и **[V2Ray transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/)**, что и для VLESS (в т.ч. `type=ws`, `path`, `host` / `Host`, **`type=httpupgrade`** как у `xhttp`), см. **`SUBSCRIPTION_PARAMS_REPORT.md`** (023) и спеку **029**.

**Параметры query string (типичные):**
- `security` — например `tls` или `none` (без TLS)
- `sni`, `host`, **`peer`** — SNI / имя сертификата (приоритет `sni`, затем `peer`, затем `host`); для WS также заголовок Host
- `type` — `ws`, `grpc`, `http`, `xhttp`, **`httpupgrade`**, `tcp`/`raw` (+ при необходимости `headerType=http`) — как у VLESS
- `path` — путь WebSocket
- `alpn`, `fp`, `insecure` / `allowInsecure` — как у VLESS

**Пример:**
```
trojan://password123@server.com:443?security=tls&sni=example.com#🇺🇸 United States
```

### Shadowsocks (`ss://`)
Формат SIP002: `ss://base64(method:password)@server:port#tag`

**Методы шифрования (поддерживаемые):**
- `2022-blake3-aes-128-gcm`
- `2022-blake3-aes-256-gcm`
- `2022-blake3-chacha20-poly1305`
- `aes-128-gcm`
- `aes-192-gcm`
- `aes-256-gcm`
- `chacha20-ietf-poly1305`
- `xchacha20-ietf-poly1305`

**Пример:**
```
ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@server.com:443#Shadowsocks Server
```

### Hysteria2 (`hysteria2://` или `hy2://`)
**Схема:** `hysteria2://` или `hy2://` (официальная короткая форма)

Стандартный URI формат: `hysteria2://[auth@]hostname[:port]/?[key=value]&[key=value]...`

**Структура:**
- `auth` - учетные данные аутентификации (password или username:password для userpass)
- `hostname` - адрес сервера
- `port` - порт (по умолчанию 443, если не указан)
  - Поддерживается multi-port формат в части порта (например, `123,5000-6000`)
- `#tag` - тег/комментарий (опционально)

**Параметры query string (согласно официальной спецификации):**
- `obfs` - тип обфускации (в настоящее время поддерживается только `salamander`)
- `obfs-password` - пароль для указанного типа обфускации
- `sni` - Server Name Indication для TLS соединений
- `insecure`, **`allowInsecure` / `allowinsecure`** — небезопасный TLS (как у VLESS: `1` / `true` / `yes`); также учитываются `skip-cert-verify`
- `fingerprint` / `fp` — uTLS fingerprint → `tls.utls` в sing-box
- `pinSHA256` — base64 SHA-256 публичного ключа сертификата → `tls.certificate_public_key_sha256` в sing-box

**⚠️ Важно:** Параметры полосы пропускания (`upmbps`, `downmbps`) и режимы клиента (HTTP, SOCKS5) **не должны** быть в URI, так как это клиентские настройки, специфичные для каждого пользователя.

**Примеры:**
```
hysteria2://password123@server.com:443?sni=example.com&insecure=1#🇺🇸 United States
hy2://password@server.com:443?obfs=salamander&obfs-password=secret&sni=real.example.com#Server
hysteria2://[email protected]:123,5000-6000/?insecure=1&pinSHA256=deadbeef#Multi-port Server
```

**Ссылка на официальную документацию:** [Hysteria 2 URI Scheme](https://v2.hysteria.network/docs/developers/URI-Scheme/)

### SSH (`ssh://`)
**⚠️ Собственный формат:** SSH URI формат является собственным форматом singbox-launcher, не стандартным протоколом.

Стандартный URI формат: `ssh://user:password@server:port?params#tag`

**Параметры query string:**
- `password` - пароль (можно также указать в userinfo: `user:password@`)
- `private_key` - приватный ключ (inline, URL-encoded)
- `private_key_path` - путь к файлу приватного ключа (например, `$HOME/.ssh/id_rsa`)
- `private_key_passphrase` - парольная фраза для приватного ключа
- `host_key` - ключ хоста (можно несколько через запятую, URL-encoded)
- `host_key_algorithms` - алгоритмы ключа хоста (через запятую)
- `client_version` - версия клиента (например, `SSH-2.0-OpenSSH_7.4p1`)

**Порт по умолчанию:** 22 (если не указан)

**Примеры:**
```
ssh://root:admin@127.0.0.1:22#Local SSH
ssh://user@server.com:22?private_key_path=$HOME/.ssh/id_rsa#Git Server
ssh://root:password@192.168.1.1:22?private_key_path=/path/to/key&host_key=ecdsa-sha2-nistp256%20AAAA...&client_version=SSH-2.0-OpenSSH_7.4p1#My SSH Server
```

### SOCKS5 (`socks5://` или `socks://`)

Формат: `socks5://[user:password@]host[:port]#tag` или `socks://...` (короткая форма). В сгенерированном конфиге sing-box — outbound **`type`: `socks`** с **`version`: `5`** (отдельного типа `socks5` в sing-box нет). В фильтрах парсера поле **`scheme`**: для ссылок `socks5://` — **`socks5`**, для `socks://` — **`socks`**.

**Структура:**
- `user:password` — опциональная авторизация (логин и пароль прокси)
- `host` — хост или IP SOCKS5-сервера (обязательный)
- `port` — порт (по умолчанию **1080**, если не указан)
- `#tag` — тег/комментарий ноды (опционально)

**Примеры:**
```
socks5://myuser:mypass@proxy.example.com:1080#Office SOCKS5
socks5://proxy.example.com:1080
socks://127.0.0.1:1080#Local
```

### WireGuard (`wireguard://`)
**⚠️ Особенность:** Узлы WireGuard записываются в секцию **endpoints** конфига (не в outbounds). Требуется **sing-box 1.11+**.

Стандартный URI формат: `wireguard://<PRIVATE_KEY>@<SERVER>:<PORT>?params#tag`

В userinfo указывается приватный ключ клиента (URL-encoded при необходимости). Спецсимволы в query — URL-encode: `/` → `%2F`, `,` → `%2C`.

**Параметры query string:**
- `publickey` — публичный ключ сервера (base64, обязательный)
- `address` — адрес клиента в VPN, CIDR (например `10.10.10.2/32`, обязательный)
- `allowedips` — разрешённые маршруты, CIDR через запятую (например `0.0.0.0/0,::/0`, обязательный)
- `mtu` — MTU (по умолчанию 1420)
- `keepalive` — интервал keepalive, секунды
- `presharedkey` — ключ PSK (base64)
- `listenport` — локальный listen port (если задан, в endpoint добавляется `listen_port`)
- `name` — имя интерфейса
- `dns` — DNS-серверы

**Пример:**
```
wireguard://privatekey-base64@10.0.0.1:51820?publickey=server-pubkey-base64&address=10.10.10.2%2F32&allowedips=0.0.0.0%2F0%2C%3A%3A%2F0&keepalive=25&mtu=1420#My WG
```

**Детали разбора:** Приватный ключ из userinfo декодируется через PathUnescape. В `publickey` и `presharedkey` символ `+` (в base64) при разборе сохраняется.

## Маркерная секция в `config.json`

Парсер перезаписывает блок между `/** @ParserSTART */` и `/** @ParserEND */`. Пример результата:

```
/** @ParserSTART */
    // 🇳🇱Нидерланды
    {"tag":"🇳🇱Нидерланды","type":"vless","server":"...","port":443,...},

    // Proxy group for international connections
    {"tag":"proxy-out","type":"selector","outbounds":["direct-out","auto-proxy-out","🇳🇱Нидерланды",...],"default":"🇳🇱Нидерланды","interrupt_exist_connections":true},
/** @ParserEND */
```

Каждая строка заканчивается запятой, чтобы после блока можно было разместить дополнительные объекты (`direct-out`, `reject` и т.д.).

## Поведение Config Wizard

Config Wizard (мастер настройки) использует специальную логику загрузки ParserConfig для обеспечения согласованности конфигурации:

### Загрузка из config.json и шаблона

При открытии Config Wizard:

1. **Приоритет: ParserConfig загружается из config.json** (если файл существует)
   - Полный ParserConfig (включая все outbounds и настройки) загружается из существующего `config.json`
   - Это сохраняет все персональные настройки пользователя, включая сложные конфигурации парсера

2. **Проверка обязательных outbounds** (если config.json существует)
   - Сначала читается шаблон (`bin/config_template.json` или `bin/config_template_macos.json`)
   - В шаблоне находятся все outbounds с полем `wizard.required > 0`
   - Для каждого такого outbound проверяется, есть ли он в текущем ParserConfig (загруженном из config.json)
   - **Логика проверки:**
     - **`required: 0` или отсутствует** — outbound игнорируется (не проверяется)
     - **`required: 1`** — проверяется только наличие тега; если outbound отсутствует в config.json, он добавляется из шаблона; если присутствует, сохраняется существующая версия из config.json
     - **`required > 1` (например, `2`)** — всегда переписывается из шаблона, независимо от наличия в config.json или соответствия шаблону
   - **Формат**: Используйте формат `"wizard": {"hide": true, "required": 2}`. Старый формат `"wizard": "hide"` поддерживается для обратной совместимости, но без поля `required`.

3. **Fallback: использование шаблона** (если config.json не существует или не содержит ParserConfig)
   - Если `config.json` не существует или не содержит валидный ParserConfig, используется шаблон (`bin/config_template.json`)
   - Все outbounds и proxies берутся из шаблона

### Пример работы

**Шаг 1: Чтение шаблона** (`config_template.json`):
При открытии визарда сначала читается шаблон, в котором находится:
```json
{
  "ParserConfig": {
    "outbounds": [
      {"tag": "proxy-out", "type": "selector", ...}
    ],
    "proxies": [{"source": "https://your-subscription-url-here"}]
  }
}
```

**Шаг 2: Загрузка из config.json** (если файл существует):
Загружается полный ParserConfig из существующего `config.json`, включая все outbounds, настройки и proxies.

**Шаг 3: Проверка обязательных outbounds**:
Система находит в шаблоне outbounds с `"wizard": {"required": 1}` или `"required": 2` и проверяет их наличие в загруженном ParserConfig.

**Шаг 4: Действие**:
- Для `required: 1` — если outbound отсутствует в config.json, добавляется из шаблона
- Для `required: 2` — outbound всегда переписывается из шаблона

**Результат в визарде**:
- ParserConfig: полностью из config.json (сохраняются все персональные настройки)
- Обязательные outbounds: проверяются и добавляются/обновляются из шаблона согласно полю `wizard.required`
- Proxies: из config.json

**Примечание**: Старый формат `"wizard": "hide"` поддерживается для обратной совместимости, но без поля `required` (только скрытие из визарда).

## Особенности и советы

- **Остановите sing-box перед обновлением**: Clash API может отреагировать на промежуточный файл
- **Нормализация флагов**: Если в подписке странные флаги, можно расширять `normalizeFlagTag` в `core/parser.go`
- **UI Clash API**: Подхватывает список селекторов из конфигурации. По умолчанию выбран селектор из `route.rules[].final` (если значение существует и совпадает с тегом). Если `final` отсутствует или не совпадает — выбирается первый селектор из списка конфигурации
- **Дублирование тегов**: Автоматически обрабатывается — дубликаты переименовываются с суффиксом
- **Config Wizard и шаблоны**: Outbounds всегда загружаются из шаблона, proxies — из config.json (если существует). Это гарантирует актуальность списка outbounds и сохранность пользовательских подписок
- **Локальные outbounds в визарде**: Теги локальных outbounds из `ProxySource.Outbounds` автоматически добавляются в список доступных outbounds на второй вкладке (Rules) визарда. Это позволяет использовать локальные селекторы в правилах маршрутизации, например, для создания специфичных правил для конкретного источника подписок

