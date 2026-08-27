# SPEC: WIREGUARD_URI — поддержка формата wireguard:// в источниках

Спецификация добавления парсинга URI схемы `wireguard://` в singbox-launcher: использование в поле Source, в Connections и в теле подписки (по одной ссылке на строку).

---

## 1. Проблема

### 1.1 Текущее состояние

- В **Source** и **Connections** парсер поддерживает только ссылки вида `vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://`, `hy2://`, `ssh://` (см. `IsDirectLink` и `ParseNode` в `core/config/subscription`).
- Конфигурацию WireGuard в формате JSON (sing-box `endpoints` / outbound `type: "wireguard"`) в источник положить нельзя: парсер не распознаёт ни сырой JSON, ни отдельную схему для WireGuard.
- Пользователь не может добавить WireGuard-узел через тот же UX, что и остальные протоколы (вставка ссылки в Source/Connections или строка в подписке).

### 1.2 Что нужно

- Поддержать **формат URI `wireguard://`** по единой спецификации (см. раздел 3).
- Обрабатывать `wireguard://` ссылки там же, где обрабатываются остальные прямые ссылки:
  - в поле **Source** (одна прямая ссылка, не URL подписки);
  - в массиве **Connections**;
  - в теле подписки, загруженной по **URL** (каждая строка — одна ссылка, в т.ч. `wireguard://`).
- Результат парсинга — **одна нода** (`config.ParsedNode`) с полем `Outbound` в формате sing-box **endpoint** для WireGuard. При сборке конфига WireGuard-ноды попадают в топ-уровневую секцию **endpoints**, а не в массив **outbounds** (в sing-box с 1.11 WireGuard описан как endpoint). Всё, что в приложении работает с outbounds (стейдж генерации, превью, селекторы, фильтры, Edit Outbound), должно учитывать и наличие endpoints — см. раздел 2.6.

---

## 2. Требования

### 2.1 Распознавание и ветвление

- В `IsDirectLink()` добавить распознавание префикса `wireguard://` (после trim). Строка, начинающаяся с `wireguard://`, считается прямой ссылкой.
- В `ParseNode()` (или вызываемой из него логике) добавить ветку для схемы `wireguard://`: парсинг URI, извлечение полей и query-параметров по спецификации ниже, построение `ParsedNode` с `Scheme: "wireguard"` и `Outbound` в виде полного sing-box endpoint для WireGuard.

### 2.2 Где действует

- **Source**: если `ProxySource.Source` не является URL подписки (`IsSubscriptionURL` = false) и является прямой ссылкой (`IsDirectLink` = true), в т.ч. `wireguard://` — парсить как один узел (как сейчас для vless/trojan/…).
- **Connections**: каждая строка в `ProxySource.Connections`, являющаяся прямой ссылкой `wireguard://`, парсится как один узел.
- **Подписка по URL**: при загрузке контента по URL тело разбивается по строкам; строка, начинающаяся с `wireguard://`, обрабатывается через `ParseNode` и даёт одну ноду (как для других схем).

### 2.3 Теги и отображение

- Тег ноды (`ParsedNode.Tag`) формируется по тем же правилам, что и для остальных протоколов: фрагмент URL (`#label`) или параметр `name`, с учётом `tag_prefix` / `tag_postfix` / `tag_mask` источника и дедупликации тегов (`MakeTagUnique`).
- В превью и списках узлов WireGuard-ноды отображаются наравне с остальными (тег, источник, при необходимости тип/схема).

### 2.4 Ошибки и валидация

- При невалидном URI (отсутствуют обязательные компоненты или параметры, неверный base64, нечисловые значения там, где ожидается число) парсер возвращает ошибку и не добавляет ноду; логирование через `debuglog` в точках start/success/error (согласно constitution и IMPLEMENTATION_PROMPT).
- Максимальная длина URI — в пределах существующего лимита для ссылок (например `MaxURILength` в node_parser), если не оговорено иное в плане реализации.

### 2.5 Критерии приёмки

- В Source или Connections можно вставить ссылку `wireguard://...` (полную по спецификации) и после парсинга/обновления превью получить одну ноду с типом WireGuard.
- В подписке (текст по URL) строка с одной ссылкой `wireguard://...` парсится в одну ноду.
- Сгенерированный endpoint для этой ноды совместим с sing-box: тип `wireguard`, структура (address, private_key, peers, mtu и т.д.) соответствует документации sing-box и маппингу из раздела 3.4.
- В итоговом config.json WireGuard-ноды попадают в секцию **endpoints**, остальные ноды и селекторы — в **outbounds**; селекторы могут ссылаться на теги WireGuard (теги endpoint'ов).
- Существующие протоколы (vless, vmess, trojan, ss, hysteria2, ssh) продолжают работать без изменений.
- Документация (например `docs/ParserConfig.md`) обновлена: в примерах connections и в списке поддерживаемых форматов указан `wireguard://`.

### 2.6 Endpoints в конфиге sing-box и интеграция в стейдж

В sing-box конфиг имеет два отдельных массива верхнего уровня: **outbounds** (прокси, селекторы, direct, block) и **endpoints** (с 1.11 — в т.ч. WireGuard, Tailscale). WireGuard описан в документации как [endpoint](https://sing-box.sagernet.org/configuration/endpoint/wireguard/), не как классический outbound.

Поэтому внедрение WireGuard через `wireguard://` затрагивает не только парсер, но и:

1. **Генерация (стейдж)**  
   - Разделение нод при генерации: ноды с `Scheme == "wireguard"` дают JSON для массива **endpoints**; остальные ноды — для **outbounds** (как сейчас). Селекторы и urltest по-прежнему генерируются в outbounds и могут ссылаться на теги как outbound-, так и endpoint-нод (тег WireGuard-ноды указывается в списке outbounds селектора).
   - Результат генерации должен содержать два среза: `OutboundsJSON` (ноды-не-wireguard + локальные и глобальные селекторы) и **EndpointsJSON** (только WireGuard-ноды). Подсчёты: отдельно число нод-outbounds и число нод-endpoints (при необходимости для превью/статистики).

2. **Сборка конфига**  
   - Секция **endpoints** в финальном config.json: либо из шаблона с подстановкой сгенерированных endpoint'ов (по аналогии с @ParserSTART/@ParserEND для outbounds), либо отдельная секция, заполняемая только сгенерированным списком. Шаблон и порядок секций (ConfigOrder) должны предусматривать ключ `"endpoints"`.

3. **Модель визарда**  
   - Помимо `GeneratedOutbounds` и `OutboundStats` нужны поля для сгенерированных endpoints (например `GeneratedEndpoints` и при необходимости счётчики), чтобы превью и сохранение конфига записывали и outbounds, и endpoints.

4. **Всё, что работает с outbounds**  
   - **Фильтрация и селекторы**: ноды WireGuard входят в общий список `allNodes`; `filterNodesForSelector` и фильтры по `scheme`/tag уже учитывают их (достаточно `Scheme: "wireguard"` и тег в `ParsedNode`). Селектор может включать тег WireGuard-ноды в свой список outbounds — в конфиге этот тег будет ссылкой на элемент из массива endpoints.
   - **Превью (Sources, Rules, Preview, кеш)**: WireGuard-ноды отображаются в списках узлов наравне с остальными; кеш и логика превью работают с единым списком нод, без разделения на «только outbounds».
   - **Edit Outbound / конфигуратор**: если в UI показываются теги для выбора (default, addOutbounds), в списке должны быть и теги WireGuard-endpoint'ов. Генерация JSON для одной ноды: для WireGuard — сериализация endpoint'а (в стейдже эта строка идёт в EndpointsJSON, не в OutboundsJSON).

Итого: парсер выдаёт ноду с `Scheme: "wireguard"` и полным `Outbound` (endpoint); стейдж разделяет вывод на outbounds и endpoints; конфиг собирается с двумя секциями; модель и UI учитывают, что часть «нод» физически попадает в endpoints, но логически участвует в тех же списках и фильтрах, что и outbound-ноды.

### 2.7 Что ещё учесть, чтобы WG заработал целиком

- **Updater (обновление конфига по подпискам)**  
  Сейчас `UpdateConfigFromSubscriptions` вызывает `GenerateOutboundsFromParserConfig` и пишет только `result.OutboundsJSON` в файл через `WriteToConfig` (между @ParserSTART и @ParserEND). Секция **endpoints** при этом не обновляется. Если этого не изменить, то при автообновлении (или ручном «обновить из подписок») новые или изменённые WireGuard-ноды попадут в `result.EndpointsJSON`, но в config.json секция `endpoints` останется старой. Нужно: при обновлении конфига также записывать сгенерированные endpoints — либо расширить `WriteToConfig` (второй блок по маркерам @ParserSTART_E / @ParserEND_E), либо отдельная запись секции `endpoints` в тот же файл за один проход.

- **Edit Outbound — список тегов**  
  В диалоге Edit/Add Outbound список тегов для AddOutbounds и default строится из `collectAllTags(parserConfig)` — это теги **outbound-конфигов** (селекторы, direct-out, reject), а не теги отдельных нод. Отдельные ноды (в т.ч. WireGuard) попадают в селектор через фильтры (например по тегу/схеме); их теги в этот список чекбоксов не подставляются. Менять логику не обязательно: WireGuard-нода будет доступна в селекторе по фильтру, а тег endpoint’а попадёт в список outbounds селектора при генерации. При необходимости можно позже добавить в список выбора и теги нод (в т.ч. endpoint’ов).

- **Шаблоны помимо wizard_template**  
  Если при сборке конфига используются другие шаблоны (например `config_template_macos.json`, `config_template.json`), в них тоже нужна секция `"endpoints": []` и учёт в ConfigOrder, иначе в части сценариев секция endpoints не появится в итоговом config.

- **Версия sing-box**  
  Endpoints (WireGuard) поддерживаются с sing-box 1.11+. Имеет смысл упомянуть в документации для пользователя требование версии.

---

## 3. WireGuard URI Specification

Формальная спецификация формата URI для передачи конфигурации WireGuard в singbox-launcher.

### 3.1 Формат

```
wireguard://<PRIVATE_KEY>@<SERVER_IP>:<SERVER_PORT>?<параметры>
```

**Пример:**

```
wireguard://RAUTG+IXUH+KW8Ocva7RTpv6y/gdVQIIgh9MeuzeMtU=@198.51.100.7:51820?publickey=CY6LY0SWr69h/WZokDYecQlTfIsZs8EhdSMd+NuaWJ4=&address=10.10.10.2%2F32&allowedips=0.0.0.0%2F0%2C%3A%3A%2F0&keepalive=25&mtu=1420
```

### 3.2 Компоненты URL

| Компонент      | Обязательный | Описание |
|----------------|--------------|----------|
| `PRIVATE_KEY`  | ✅           | Приватный ключ клиента в base64. Передаётся как userinfo (до `@`). |
| `SERVER_IP`    | ✅           | IP-адрес или hostname WireGuard сервера. |
| `SERVER_PORT`  | ✅           | UDP порт WireGuard сервера (обычно `51820`). |

### 3.3 Query-параметры

| Параметр        | Обязательный | Тип            | Пример                    | Описание |
|-----------------|--------------|----------------|---------------------------|----------|
| `publickey`     | ✅           | string (base64)| `fiK9...Fk=`              | Публичный ключ сервера (peer public key). |
| `address`       | ✅           | CIDR           | `10.10.10.2/32`           | IP-адрес клиента внутри WireGuard туннеля. |
| `allowedips`    | ✅           | CIDR, через `,`| `0.0.0.0/0,::/0`          | Подсети, маршрутизируемые через туннель. |
| `dns`           | ❌           | IP             | `1.1.1.1`                 | DNS-сервер для использования внутри туннеля. |
| `mtu`           | ❌           | integer        | `1420`                    | MTU интерфейса WireGuard (по умолчанию: `1420`). |
| `keepalive`     | ❌           | integer (сек)  | `25`                      | Persistent keepalive interval в секундах. |
| `presharedkey`  | ❌           | string (base64)| `abc...=`                 | Pre-shared key (PSK), если требуется. |
| `listenport`    | ❌           | integer        | `10000`                   | Локальный UDP порт клиента (по умолчанию: `0` — случайный). |
| `name`          | ❌           | string         | `my-vpn`                  | Имя интерфейса (используется как `name` в sing-box endpoint). |

### 3.4 Кодирование

- `PRIVATE_KEY` может содержать символы `+`, `/`, `=` — в userinfo их нужно URL-encode при формировании ссылки и корректно декодировать при парсинге.
- В query-параметрах `address` и `allowedips` содержат `/` и `,` — при передаче в URI обязательно URL-encode, например:
  - `/` → `%2F`
  - `,` → `%2C`
  - `:` → `%3A`

### 3.5 Маппинг в sing-box endpoint

Результат парсинга одной ссылки `wireguard://` должен приводиться к структуре sing-box **endpoint** (для использования в конфиге):

```json
{
  "type": "wireguard",
  "tag": "<tag ноды>",
  "name": "<name из query или 'singbox-wg0'>",
  "system": false,
  "mtu": <mtu>,
  "address": ["<address>"],
  "private_key": "<PRIVATE_KEY>",
  "listen_port": <listenport>,
  "peers": [
    {
      "address": "<SERVER_IP>",
      "port": <SERVER_PORT>,
      "public_key": "<publickey>",
      "pre_shared_key": "<presharedkey>",
      "allowed_ips": ["<allowedips разбитые по запятой>"],
      "persistent_keepalive_interval": <keepalive>
    }
  ]
}
```

- Поле `pre_shared_key` включается только если передан параметр `presharedkey`.
- Поле `persistent_keepalive_interval` — по значению `keepalive`; если не указано — не добавлять поле или использовать `0` (по решению реализации в соответствии с рекомендациями ниже).
- `allowedips` в URI — строка с одним или несколькими CIDR через запятую → в JSON массив строк (каждый элемент — один CIDR).
- `address` в URI может содержать несколько значений через запятую (например IPv4 + IPv6) → в JSON массив строк.

### 3.6 Поведение парсера (рекомендации для реализации)

- Если `mtu` не указан — использовать `1420`.
- Если `listenport` не указан — использовать `0` (случайный порт в sing-box).
- Если `keepalive` не указан — не добавлять поле в peer или использовать `0`.
- Если `presharedkey` не указан — не добавлять поле `pre_shared_key` в peer.
- Если `name` не указан — использовать `singbox-wg0`.
- `allowedips`: разбить по запятой, каждый элемент — элемент массива.
- `address`: разбить по запятой при нескольких значениях → массив строк.

### 3.7 Совместимость

Формат частично совместим с URI-схемой, используемой в WireGuard Android (импорт через QR) и в некоторых клиентах Amnezia VPN. Передача конфигурации в одну строку удобна для QR-кодов, deep link в GUI и вставки в поле импорта singbox-launcher.

---

## 4. Структуры данных (кратко)

- **Вход**: строка URI вида `wireguard://...` (в Source, в элементе Connections или в строке тела подписки).
- **Выход**: `*config.ParsedNode` с:
  - `Scheme`: `"wireguard"`;
  - `Tag`, `Label`, `Comment` — из фрагмента `#...` и/или параметра `name` по общим правилам;
  - `Server`, `Port` — из host и port URI (peer address/port);
  - `Outbound`: `map[string]interface{}` — полная структура sing-box endpoint для WireGuard (см. 3.5), готовая к сериализации в JSON.

Детали полей ParsedNode для WireGuard (использование `UUID`, `Query` и т.д.) оставляются на этап PLAN.md при необходимости минимизации изменений в существующем коде.

---

## 5. Ограничения и не входит в задачу

- **Не входит**: поддержка сырого JSON с `endpoints` или вставка произвольного JSON WireGuard в Source.
- **Не входит**: загрузка по URL отдельного «WG-конфига по ссылке» (например URL, возвращающий JSON с массивом endpoints). Поддержка только строки `wireguard://...` как одной ссылки в Source, Connections или в одной строке подписки.
- Изменения только в объёме, необходимом для парсинга `wireguard://` и генерации endpoint; без расширения архитектуры ParserConfig (версия 4 остаётся).

---

## 6. Ссылки

- Текущий парсер: `core/config/subscription/node_parser.go`, `source_loader.go`.
- Модель: `core/config/models.go` (`ParsedNode`, `ProxySource`).
- Документация форматов: `docs/ParserConfig.md`.
