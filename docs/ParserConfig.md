# Документация парсера подписок singbox-launcher

> **SPEC 052 update (state.json v5):** top-level ключ `parser_config` в state.json **удалён**. Список подписок теперь в `connections.sources[]` (см. `docs/WIZARD_STATE.md`); per-source `tag_prefix`, `skip`, `outbounds`, `disabled` живут на уровне Source и сохраняют ту же семантику. Раздел `parser_config.outbounds[]` переехал в `connections.outbounds[]`. Документ ниже описывает **формат полей и логику парсинга** — она актуальна для v5 connections.sources / connections.outbounds 1:1.

## Назначение

Парсер обновляет файл `bin/config.json`, загружая подписки (см. таблицу [«Поддерживаемые протоколы»](#поддерживаемые-протоколы) ниже — 9 протоколов: VLESS, VMess, Trojan, Shadowsocks, Hysteria2, SSH, SOCKS5, NaïveProxy, WireGuard), фильтруя и группируя их в селекторы. Результат записывается в секции между маркерами `/** @ParserSTART */` и `/** @ParserEND */` (outbounds), а узлы WireGuard — между `/** @ParserSTART_E */` и `/** @ParserEND_E */` (endpoints). Секция **endpoints** (WireGuard) поддерживается в sing-box начиная с версии **1.11**.

### Поддерживаемые протоколы

| # | Схема URI | sing-box `type` | Секция конфига | Версия / build-tag | Описание |
|---|-----------|-----------------|----------------|--------------------|----------|
| 1 | `vless://` | `vless` | `outbounds[]` | core (+ **`with_xhttp`** для xhttp) | TCP/raw/ws/grpc/http/`httpupgrade`/quic/**`xhttp`** (splithttp), TLS, Reality, Vision flow. xhttp — нативно на ядре sing-box-lx (см. ниже). |
| 2 | `vmess://` | `vmess` | `outbounds[]` | core (+ **`with_xhttp`**) | Base64 JSON или legacy cleartext `method:uuid@host:port`. `net=h2`→`http`+TLS; `net=xhttp`→**`xhttp`**, `net=httpupgrade`→`httpupgrade` (разные транспорты). |
| 3 | `trojan://` | `trojan` | `outbounds[]` | core | Те же transport/TLS, что и VLESS. Пароль в userinfo. |
| 4 | `ss://` | `shadowsocks` | `outbounds[]` | core | SIP002 + legacy `ss://base64("method:password@host:port")`. Методы — фиксированный allow-list (2022-blake3, AEAD GCM, ChaCha20-Poly1305). |
| 5 | `hysteria2://`, `hy2://` | `hysteria2` | `outbounds[]` | core (QUIC) | Multi-port (`mport`/`ports` query или `host:123,5000-6000` в authority); obfs только `salamander`. |
| 6 | `ssh://` | `ssh` | `outbounds[]` | core | **Собственный URI-диалект singbox-launcher**, не RFC. Inline-ключ / путь к ключу / passphrase / host_key. |
| 7 | `socks5://`, `socks://` | `socks` (version=5) | `outbounds[]` | core | User/pass опциональны. Поле фильтра `scheme` сохраняет оригинал (`socks5` vs `socks`). |
| 8 | `naive+https://`, `naive+quic://` | `naive` | `outbounds[]` | **sing-box ≥ 1.13.0** + build tag **`with_naive_proxy`** (Apple/Android/Windows-сборки SagerNet ОК; минимальный Linux отвергает в runtime) | DuckSoft 2020 URI-диалект. `extra-headers=` (CRLF-разделённые пары). TLS только `server_name`. |
| 9 | `wireguard://` | `wireguard` | **`endpoints[]`** | **sing-box ≥ 1.11** (+ **`with_awg`** для AmneziaWG) | Один peer; маркеры `@ParserSTART_E`/`@ParserEND_E`. Default port 51820, mtu 1420. Опциональные параметры **AmneziaWG 2.0** (jc/jmin/jmax, s1–s4, h1–h4, i1–i5) — см. ниже. |

**Не поддерживаются** (явно, не реализованы): **TUIC**, **AnyTLS**, **ShadowTLS**, **Mieru**, **Hysteria 1** (только v2), **ShadowsocksR / SSR**, **Tor**, plain HTTP-proxy как тип ноды (URL `http(s)://...` — это всегда **источник подписки**, не нода). Селекторы (`selector`, `urltest`, `direct`, `block`, `dns`) — не URI-протоколы; собираются на стороне ParserConfig (см. [секцию `outbounds`](#секция-outbounds)).

### Транспорт xhttp и AmneziaWG

Лаунчер собран под ядро **[sing-box-lx](https://github.com/Leadaxe/sing-box-lx)** (upstream sing-box + ровно две клиентские фичи под build-тегами). Парсер/генератор/share-URI лаунчера поддерживают обе сквозно; в рантайме они работают **только** на ядре с соответствующим тегом — на стоковом sing-box конфиг с этими полями отвергается на load-time (явная ошибка, без тихого даунгрейда).

**✅ `xhttp` транспорт — полноценно (build-tag `with_xhttp`).** Прежняя деградация в `httpupgrade` снята. При `type=xhttp` (VLESS/Trojan) или `net=xhttp` (VMess) строится честный транспорт `type:"xhttp"` (Xray-совместимый splithttp) со всеми полями, и без потерь сериализуется обратно в share-URI:

- Поля: `mode` (`auto` \| `packet-up` \| `stream-up` \| `stream-one`; у форка `auto`=`packet-up`, у `stream-one` известный баг downlink-framing), `host`, `path`, `headers`, `x_padding_bytes` (диапазон `"min-max"`, дефолт `100-1000`, несётся в заголовке `Referer`), `no_grpc_header`. Композится с TLS/Reality (не с XTLS-Vision — ограничение протокола).
- `httpupgrade` теперь **отдельный** транспорт (HTTP/1.1 Upgrade) — больше не путается с xhttp ни на входе, ни на выходе share-URI.
- Детали: `SPECS/071-F-N-XHTTP_TRANSPORT/SPEC.md`, `sing-box-lx/docs/lx-config.md`.

**✅ AmneziaWG 2.0 (AWG2) — обфускация WireGuard (build-tag `with_awg`).** WireGuard-endpoint (`wireguard://`) может нести promoted-поля AWG: числа `jc`/`jmin`/`jmax`, `s1`–`s4`, `h1`–`h4` и CPS-строки `i1`–`i5` (AWG 2.0, case-sensitive tag-формат). Парсятся из URI, эмитятся в `endpoints[]`, round-trip в share-URI без потерь. Endpoint **без** AWG-полей — обычный WireGuard (byte-identical с апстримом). Детали полей — секция [WireGuard](#wireguard-wireguard) ниже; `SPECS/073-F-N-AMNEZIAWG_PARAMS/SPEC.md`, `sing-box-lx/docs/lx-config.md`.

Подробности по каждой схеме (query-параметры, TLS, transport, edge cases) — в разделе [Форматы URI для прямых ссылок](#форматы-uri-для-прямых-ссылок) ниже.

### JSON-массив полных конфигов Xray/V2Ray

Если тело подписки (plain или после декодирования Base64) — **валидный JSON-массив** `[...]`, а элементы похожи на Xray (`outbounds[].protocol`, VLESS с `settings.vnext`), лаунчер обрабатывает его как подписку: из **каждого элемента** извлекается **одна** логическая нода. Для разбора используются поля **`outbounds`** и (при наличии) **`remarks`**; корневые **`dns`**, **`routing`**, **`inbounds`** и прочее из элемента **не** подмешиваются в общий конфиг лаунчера.

**Как отличить Xray-массив от sing-box-массива (016, не реализовано)**

| Шаг | Эвристика |
|-----|-----------|
| Декодер | После trim строка начинается с **`[`**, **`json.Valid`**, успешный `json.Unmarshal` в массив — тело не отвергается как «не подписка» (`DecodeSubscriptionContent`). |
| Вход в парсер | **`IsXrayJSONArrayBody`**: то же — префикс `[`, валидный JSON, массив объектов. |
| Элемент массива | **`xrayElementHasProtocolOutbounds`**: в **`outbounds`** есть хотя бы один объект с полем **`protocol`** (строка) — признак **Xray-диалекта**. Элементы только с sing-box **`type`** без **`protocol`** не считаются Xray для этой ветки и **пропускаются** с `debuglog` (ожидается follow-up **016**). |
| Нода | Среди VLESS с **`settings.vnext`** выбирается основной outbound (`pickMainXrayVLESS`); при **`dialerProxy`** hop разбирается как **`socks`** или **`vless`** (`xrayBuildJumpFromOutbound`); иные `protocol` у hop — пропуск элемента (`WarnLog`). |

**`remarks` и теги sing-box**

- В **`ParsedNode.Label`** попадает полный текст **`remarks`** (если пусто — запасной вариант: тег основного Xray-outbound или `xray-{индекс}`).
- **Теги** генерируемых outbound в sing-box: если **`remarks`** непустой, из него строится **slug** (буквы/цифры в любой скрипте, **символы региональных индикаторов** для UTF-флагов, нормализация через `textnorm`, обрезка длины; прочие знаки и emoji кроме флагов в slug не входят). **Основной** outbound получает тег **`{slug}`**; при цепочке через SOCKS второй outbound (jump) — **`{slug}_jump_server`**, а у основного в JSON задаётся **`detour`** на этот тег. Если **`remarks`** пустой — **`xray-{индекс}`** и **`xray-{индекс}_jump_server`**. Далее, как у обычных подписок, применяются **`tag_prefix` / `tag_postfix` / `tag_mask`**, **`textnorm.NormalizeProxyDisplay`** и **`MakeTagUnique`** (в т.ч. для jump).
- В сгенерированном фрагменте `config.json` над outbound по-прежнему пишется **комментарий** `// …` из **`Label`** (полный `remarks`), т.к. у sing-box нет поля «remarks» в outbound.

**Цепочка `dialerProxy`**

При **`streamSettings.sockopt.dialerProxy`** (или **`dialer`**) → outbound с тем же **`tag`**: поддерживаются hop’ы **`protocol: socks`** и **`protocol: vless`**; в `config.json` сначала генерируется outbound hop’а, затем основной (VLESS и т.д.) с полем **`detour`** на тег hop’а. Если outbound по тегу не найден или **`protocol`** hop’а не **socks** / не **vless** — элемент массива **не** даёт ноды (`WarnLog`). Детали и расширение на другие типы: **`SPECS/036-F-C-XRAY_JUMP_ANY_PROTOCOL/SPEC.md`**. Массив конфигов **только в формате sing-box** (`type` в outbounds без Xray-`protocol`) в MVP **не** разбирается (follow-up **016**).

**Пример и код**

Структура как у публичных Xray-подписок (**`dns`**, **`inbounds`**, **`log`**, **`mux`**, **`tcpSettings`**, **`routing`**, **`freedom`/`blackhole`**), с вымышленными данными: **`docs/examples/xray_subscription_array_sample.json`**. Тот же сценарий в тестах: **`core/config/subscription/testdata/xray_provider_anon.json`** (`go:embed` в **`xray_json_array_test.go`**). Реализация: **`xray_json_array.go`**, **`xray_outbound_convert.go`**, **`decoder.go`** (`DecodeSubscriptionContent`), **`source_loader.go`** (`LoadNodesFromSource`, **`applyTagsToXrayNode`**), configurator: **`ui/configurator/tabs/source_tab.go`** (`refreshOneSourceFromUI`).

## Документы и исходный код парсера URI

| Документ / место | Содержание |
|------------------|------------|
| **Этот файл** (`docs/ParserConfig.md`) | Форматы прямых ссылок в `connections`, Share URI, структура ParserConfig, пайплайн обновления. |
| **`SPECS/023-F-C-SUBSCRIPTION_TRANSPORT_VLESS_TROJAN/SUBSCRIPTION_PARAMS_REPORT.md`** | Таблицы: query VLESS/Trojan → поля sing-box; примеры из публичных подписок; ключи query. |
| **`SPECS/029-Q-С-SUBSCRIPTION_PARSER_CLASH_CONVERTOR_PARITY/SPEC.md`** | Расширения совместимости (029): `type=httpupgrade`, `peer`, `obfsParam`, VMess legacy / `httpupgrade` / `h2`, Hysteria2 TLS; сверка со схемой sing-box. |
| **`SPECS/033-F-N-SUBSCRIPTION_XRAY_JSON_ARRAY/SPEC.md`** | Подписка как JSON-массив полных конфигов Xray: `remarks`, slug-теги, `dialerProxy` → `detour`, границы MVP (sing-box-массив — **016**, follow-up). |
| **`SPECS/036-F-C-XRAY_JUMP_ANY_PROTOCOL/SPEC.md`** | `dialerProxy`: hop **SOCKS** или **VLESS**; прочие протоколы — по мере маппинга (**завершено** по объёму SPEC). |
| Пакет **`core/config/subscription`** | `ParseNode`, `buildOutbound` — `node_parser.go`; VLESS/Trojan transport+TLS — `node_parser_transport.go`; VMess — `node_parser_vmess.go` (`parseVMessDecoded`, `parseVMessJSON`, `parseVMessLegacyCleartext`); Hysteria2 — `node_parser_hysteria2.go`; WireGuard / SSH — `node_parser_wireguard.go`, `node_parser_ssh.go`; share URI — `share_uri_encode.go`; JSON-массив Xray — `xray_json_array.go`, `xray_outbound_convert.go`. |

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
| `naive` | `naive+https://` / `naive+quic://` | HTTP/2 (`naive+https`) или QUIC (`naive+quic`); user/pass в userinfo; `extra-headers` в query с `\r\n`-разделёнными парами (см. раздел **NaïveProxy** ниже). Требует sing-box **≥ 1.13.0** с `with_naive_proxy` build tag. |
| `wireguard` | `wireguard://` | Обычно узел только в `endpoints[]`; формат и query — раздел **WireGuard** ниже. **Один URI ↔ один удалённый peer:** при нескольких элементах в `peers[]` кодирование не поддерживается (`ErrShareURINotSupported`). |

**Не кодируются в один share URI:** `selector`, `urltest`, `direct`, `block`, `dns`, произвольные служебные типы; WireGuard с **несколькими** `peers`; outbound с непустым **`detour`** (цепочка через jump из подписки Xray JSON).

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
          // URL подписки (Base64, plain-текст или JSON-массив конфигов Xray)
          // Поддерживаются: VLESS, VMess, Trojan, Shadowsocks, Hysteria2,
          // SSH, SOCKS5, NaïveProxy, WireGuard. См. таблицу «Поддерживаемые
          // протоколы» в начале документа.
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
| `source`      | string   | Да           | URL подписки. Все 9 протоколов из таблицы [«Поддерживаемые протоколы»](#поддерживаемые-протоколы): VLESS, VMess, Trojan, Shadowsocks, Hysteria2, SSH, SOCKS5, NaïveProxy, WireGuard. Допускаются Base64 и plain-текст; также **JSON-массив** полных конфигов Xray (`[ {...}, ... ]`), см. выше. |
| `connections` | array    | Нет          | Массив прямых ссылок. Все 9 схем из таблицы [«Поддерживаемые протоколы»](#поддерживаемые-протоколы): `vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://`/`hy2://`, `ssh://`, `socks5://`/`socks://`, `naive+https://`/`naive+quic://`, `wireguard://`. Можно комбинировать с подписками. Узлы WireGuard попадают в секцию `endpoints` конфига (sing-box ≥ 1.11). NaïveProxy требует sing-box ≥ 1.13.0 + build tag `with_naive_proxy`. Подробнее — раздел [Форматы URI для прямых ссылок](#форматы-uri-для-прямых-ссылок). |
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

3. **Поддерживаемые протоколы** (полная матрица — см. таблицу в начале документа)
   - ✅ **VLESS** (`vless://`)
   - ✅ **VMess** (`vmess://`)
   - ✅ **Trojan** (`trojan://`)
   - ✅ **Shadowsocks / SS** (`ss://` — SIP002 + legacy)
   - ✅ **Hysteria2** (`hysteria2://` и короткая форма `hy2://`)
   - ✅ **SSH** (`ssh://` — собственный URI-диалект singbox-launcher)
   - ✅ **SOCKS5** (`socks5://`, `socks://` — outbound type `socks`, version=5)
   - ✅ **NaïveProxy** (`naive+https://`, `naive+quic://` — sing-box ≥ 1.13.0 + build tag `with_naive_proxy`)
   - ✅ **WireGuard** (`wireguard://` — секция `endpoints[]`; sing-box ≥ 1.11)

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
   - Узлы VLESS / VMess / Trojan / SS / Hysteria2 / SSH / SOCKS5 / **NaïveProxy** сериализуются в `outbounds[]`; узлы WireGuard — в `endpoints[]` (sing-box ≥ 1.11)
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
- `flow` — подпротокол VLESS в sing-box (например `xtls-rprx-vision`), см. [доку VLESS](https://sing-box.sagernet.org/configuration/outbound/vless/). Если в ссылке **`flow` нет**, в outbound **ничего не подставляется** (нужен Vision — укажите `flow=xtls-rprx-vision` в подписке).
- `security` — `none` | `tls` | `reality`; при `none` TLS в outbound не добавляется
- `sni` — имя для SNI / проверки сертификата → `tls.server_name`; при пустом `sni` используется **`peer`** (тот же смысл в части подписок)
- `fp`, **`fingerprint`** — отпечаток uTLS → `tls.utls.fingerprint`. Допустимые строки — как в [документации sing-box (TLS, utls, fingerprint)](https://sing-box.sagernet.org/configuration/shared/tls/#outbound): перечисление там в **нижнем регистре** (`chrome`, `firefox`, `qq`, `random`, `randomized`, …). Значения из ссылок и поле при **генерации** `config.json` приводятся к нижнему регистру, иначе sing-box может вернуть ошибку вида `unknown uTLS fingerprint` для вариантов вроде `QQ`.
- `alpn` — список через запятую → `tls.alpn`
- `insecure`, `allowInsecure` / `allowinsecure` — при `1` / `true` → `tls.insecure`
- `pbk`, `sid` — Reality → `tls.reality.public_key`, `short_id`
- `type` — транспорт: `tcp` / `raw`, `ws`, `grpc`, `http`, **`httpupgrade`**, **`xhttp`**, реже `quic`. `xhttp` строится как нативный splithttp-транспорт (ядро sing-box-lx, build-tag `with_xhttp`; см. [секцию про xhttp и AmneziaWG](#транспорт-xhttp-и-amneziawg) в начале документа), отдельно от `httpupgrade`
- `path` — путь WebSocket / HTTP / httpupgrade или fallback имени сервиса для gRPC
- `host` / `Host` — для WS → заголовок `Host`; если `host` и `sni` в query нет, для WS используется **`obfsParam`**. Если есть `host` или `sni`, они имеют приоритет. Для HTTP/httpupgrade — поле `host` транспорта (регистр ключа `Host` в query учитывается)
- `headerType` — вместе с `type=raw` или `tcp` и значением `http` задаёт транспорт типа HTTP (обфускация), см. отчёт 023
- `serviceName` / `service_name` — имя gRPC-сервиса → `transport.service_name`
- `packetEncoding` — поле outbound `packet_encoding`. **Allow-list:** только `xudp`, `packetaddr`, `none` (включая пустое значение). Любое другое значение **отбрасывается с warning** в `debuglog` — sing-box не примёт неизвестные. См. [доку VLESS](https://sing-box.sagernet.org/configuration/outbound/vless/)
- `mode`, `spx`, `extra`, `quicSecurity`, `authority` — часто встречаются в ссылках Xray/панелей; в документированный клиентский JSON sing-box **не переносятся**, на разбор ссылки не влияют

**⚠️ Vision на UDP:443 — авто-перезапись порта.** Если `flow=xtls-rprx-vision-udp443`, парсер **принудительно** ставит `server_port=443` (независимо от порта в URI) и `packet_encoding=xudp`. Семантика flow — XTLS Vision поверх UDP-трафика к стандартному 443. Если ваш сервер слушает Vision на нестандартном порту, используйте `flow=xtls-rprx-vision` (без `-udp443` суффикса).

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
- `net` - тип сети (`tcp`, `ws`, `http`, `grpc`, **`httpupgrade`**, **`xhttp`**; **`h2`** → transport `http` + TLS; **`xhttp`** → нативный splithttp-транспорт `xhttp` (ядро sing-box-lx, build-tag `with_xhttp`), отдельно от `httpupgrade`; см. [секцию про xhttp и AmneziaWG](#транспорт-xhttp-и-amneziawg) в начале документа)
- `type` - тип заголовка (для `tcp`)
- `host` - хост (для `ws`/`http`; для WS при пустом `host` подставляется SNI из TLS, если есть)
- `path` - путь (для `ws`/`http`/`grpc`)
- `tls` - использование TLS (`"tls"` или отсутствует)
- `sni` - SNI (опционально)
- `alpn` - ALPN (опционально)
- `fp` - fingerprint (опционально)
- `insecure` в JSON (`"1"`) — небезопасный TLS, как у VLESS

**Сборка outbound с TLS для VMess:** `tls.server_name` берётся из `sni`, при отсутствии — из поля **`peer`** в query (если провайдер продублировал имя в `peer`), иначе — **адрес сервера** (`add`). Флаги **`insecure` / `allowInsecure` / `allowinsecure`** в query обрабатываются так же, как для VLESS (`tlsInsecureTrue`).

**Legacy cleartext (не JSON):** парсер также принимает `vmess://base64("method:uuid@host:port?query")` — старый формат, используемый частью клиентов (V2RayN early и т.п.). После base64-декодирования распознаётся как URI с теми же query-параметрами, что у URI-протоколов: `type=ws`, `path`, `tls=1`, `host`, `sni` и т.д.; они маппятся в `transport` и `tls`. Парсер автоматически детектит формат по первому символу декодированной строки: `{` → JSON, иначе → legacy cleartext.

**Пример:**
```
vmess://eyJ2IjoiMiIsInBzIjoiVGVzdCIsImFkZCI6InNlcnZlci5jb20iLCJwb3J0Ijo0NDMsImlkIjoi dXVpZCIsImFpZCI6MCwic2N5IjoiYXV0byIsIm5ldCI6InRjcCIsInR5cGUiOiJub25lIiwidGxzIjoiIn0=
```

### Trojan (`trojan://`)
Стандартный URI формат: `trojan://password@server:port?params#tag`

Те же правила **TLS** и **[V2Ray transport](https://sing-box.sagernet.org/configuration/shared/v2ray-transport/)**, что и для VLESS (в т.ч. `type=ws`, `path`, `host` / `Host`, `type=httpupgrade`, `type=xhttp` — нативный splithttp как у VLESS, см. [секцию про xhttp и AmneziaWG](#транспорт-xhttp-и-amneziawg) в начале документа), см. **`SUBSCRIPTION_PARAMS_REPORT.md`** (023) и спеку **029**.

**Параметры query string (типичные):**
- `security` — например `tls` или `none` (без TLS)
- `sni`, `host`, **`peer`** — SNI / имя сертификата (приоритет `sni`, затем `peer`, затем `host`); для WS также заголовок Host
- `type` — `ws`, `grpc`, `http`, **`httpupgrade`**, **`xhttp`**, `tcp`/`raw` (+ при необходимости `headerType=http`) — как у VLESS. `xhttp` — нативный splithttp-транспорт (ядро sing-box-lx, build-tag `with_xhttp`), отдельно от `httpupgrade`
- `path` — путь WebSocket
- `alpn`, `fp`, `insecure` / `allowInsecure` — как у VLESS

**Пример:**
```
trojan://password123@server.com:443?security=tls&sni=example.com#🇺🇸 United States
```

### Shadowsocks (`ss://`)

Два формата:

1. **SIP002 (предпочтительный):** `ss://base64(method:password)@server:port#tag` — userinfo в base64-кодировке метода и пароля, server/port в чистом виде.
2. **Legacy non-SIP002:** `ss://base64("method:password@server:port")#tag` — всё `method:password@server:port` в одном base64-блоке. Парсер автоматически детектит и поддерживает оба формата.

**Методы шифрования (allow-list `isValidShadowsocksMethod`):**

| Категория | Методы |
|-----------|--------|
| Shadowsocks 2022 (рекомендуется) | `2022-blake3-aes-128-gcm`, `2022-blake3-aes-256-gcm`, `2022-blake3-chacha20-poly1305` |
| AEAD GCM | `aes-128-gcm`, `aes-192-gcm`, `aes-256-gcm` |
| AEAD ChaCha20 | `chacha20-ietf-poly1305`, `xchacha20-ietf-poly1305` |
| Без шифрования | `none` (только если сервер сконфигурирован соответственно) |

Любой другой метод (например, legacy streaming RC4/AES-CFB) **отвергается на парсе** — sing-box не примёт его в outbound, поэтому узел не имеет смысла создавать.

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
- `#tag` - тег/комментарий (опционально)

**Multi-port:** Hysteria2 поддерживает несколько источников для списка портов, в порядке приоритета:
1. **Query `mport`** или `ports` — основной канонический способ. Значение — comma-separated список портов / диапазонов: `mport=443,5000-6000,8443`.
2. **Авторити-style `host:123,5000-6000`** — если в части порта URI есть запятая (что нарушает RFC), парсер сначала «спасает» хвост через `hysteria2RecoverMultiPortAuthority`: первый порт идёт в `server_port`, остаток (включая первый) уезжает в query как `mport`. URI вида `hysteria2://[email protected]:123,5000-6000/?...` отрабатывается корректно.
3. Если `mport` пустой и в порту только одно число — простой одно-портовый случай.

Дальше sing-box сам разруливает: при наличии `server_ports` (список) клиент open'ит по любому из них.

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

### NaïveProxy (`naive+https://` / `naive+quic://`)

**Требование:** sing-box должен быть собран с поддержкой NaïveProxy (build tag `with_naive_proxy`). Официальные релизы `SagerNet/sing-box` для **Apple, Android, Windows и отдельных Linux-сборок** включают эту поддержку; на минимальных сборках парсинг URI пройдёт, но `sing-box check` при запуске отклонит конфиг как «unknown outbound type 'naive'».

**Схема URI** (де-факто, DuckSoft 2020 — [gist](https://gist.github.com/DuckSoft/ca03913b0a26fc77a1da4d01cc6ab2f1)):

```
naive+https://<user>:<pass>@<host>:<port>/?<params>#<label>
naive+quic://<user>:<pass>@<host>:<port>/?<params>#<label>
```

- **Схема:** `naive+https` — транспорт HTTP/2; `naive+quic` — QUIC (с автоматическим `quic_congestion_control: bbr` в JSON).
- **Userinfo:** `<user>:<pass>` или только `<pass>` (тогда ложится в user-slot — как у hysteria2). Anonymous-режим — без userinfo.
- **Port:** опциональный, default **443**.
- **Query:**
  - `padding=true|false` — **игнорируется** с warning (в sing-box нет соответствующего поля).
  - `extra-headers=<urlencoded "Header1: Value1\r\nHeader2: Value2">` — дополнительные HTTP-заголовки; невалидные пары (неправильный charset имени, CR/LF/NUL в значении) пропускаются с warning, остальные сохраняются.
- **Fragment (`#label`):** URL-decoded, UTF-8-fixup — стандартно.

**Примеры:**

```
naive+https://what:happened@test.someone.cf?padding=false#Naive!
naive+https://some.public.rs?padding=true#Public-01
naive+quic://manhole:114514@quic.test.me
naive+https://some.what?extra-headers=X-Username%3Auser%0D%0AX-Password%3Apassword
```

**Результирующий JSON outbound** (sing-box ≥ 1.13.0, [doc](https://sing-box.sagernet.org/configuration/outbound/naive/)):

```json
{
  "type": "naive",
  "tag": "…",
  "server": "test.someone.cf",
  "server_port": 443,
  "username": "what",
  "password": "happened",
  "tls": { "enabled": true, "server_name": "test.someone.cf" }
}
```

Для `naive+quic://` добавляются `"quic": true` и `"quic_congestion_control": "bbr"`. Блок `extra-headers` разворачивается в `"extra_headers": {"X-Username": "user", "X-Password": "password"}`.

**TLS-блок:** sing-box naive outbound поддерживает **только** `server_name`, `certificate`, `certificate_path`, `ech` — `alpn / utls / reality / min_version` для этого типа не применимы и не эмитятся парсером. Custom SNI в URI пока не поддерживается (v1); `tls.server_name` = `host`. Для ручного переопределения — правка `config.json` после wizard Save.

**Share URI (обратная сборка)** — `ShareURIFromOutbound` для `type: "naive"`:
- `naive+https://` или `naive+quic://` в зависимости от `quic: true/false`.
- `extra_headers` map'а сортируется лексикографически по ключам (для детерминизма round-trip'а), склеивается `\r\n`, шифруется в query.
- `padding` **не восстанавливается** (не хранится в outbound).

Реализация: `core/config/subscription/node_parser_naive.go` (helpers), `node_parser.go` (dispatch), `share_uri_encode.go` (`shareURIFromNaive`). Спека: [**SPECS/044-F-C-NAIVE_PROXY_PARSER/SPEC.md**](../SPECS/044-F-C-NAIVE_PROXY_PARSER/SPEC.md).

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

**AmneziaWG 2.0 (опционально — ядро sing-box-lx с `with_awg`):**

Те же `wireguard://` ссылки могут нести параметры обфускации AmneziaWG — они promoted в корень WireGuard-endpoint рядом с `private_key`/`peers`:

- **Числовые** (uint32 → JSON-число): `jc` (число junk-пакетов до handshake), `jmin`/`jmax` (мин/макс размер junk), `s1`/`s2` (junk перед init/response handshake), `s3`/`s4` (junk перед cookie-reply/transport — **AWG 2.x**), `h1`–`h4` (magic-заголовки для 4 типов сообщений WireGuard).
- **Строковые** (case-sensitive tag-формат): `i1`–`i5` — CPS decoy-пакеты **AWG 2.0**, отправляются по порядку до handshake. Теги: `<b 0xHEX>` статичные байты, `<c>` счётчик, `<t>` timestamp, `<r N>` / `<rc N>` / `<rd N>` — random байты / символы / цифры.

Имена числовых полей читаются из query в любом регистре; `i1`–`i5` берутся как есть (кейс сохраняется). Endpoint **без единого** AWG-поля = обычный WireGuard (byte-identical с апстримом). Клиент и сервер должны иметь **совпадающие** AWG-параметры — I-пакеты являются конфигурацией, не согласуются по сети. Маппинг 1:1 с `awg.conf` (awg-quick): `[Interface] Jc/Jmin/Jmax/S1–S4/H1–H4/I1–I5` → корень endpoint, `[Peer] …` → `peers[0]`.

**MTU AWG-эндпоинта клампится сверху до `1280`** (рекомендация AmneziaWG). `s3`/`s4` добавляют padding к **каждому** transport-пакету, поэтому при `mtu=1420` итоговый пакет вылезает за path-MTU и ОС отбивает его (`sendmsg: message too long`): handshake проходит, но данные молча встают. Политика парсера для WireGuard-эндпоинта **с** AWG-полями: `mtu` из URI выше 1280 → понижается до 1280; явно меньшее значение (напр. `mtu=1200`) уважается; при отсутствии `mtu` — дефолт 1280 (а не 1420). Обычный WireGuard (без AWG-полей) сохраняет апстрим-дефолт 1420. Потолок без запаса: `1500 − 28 (UDP/IP) − 32 (WG) − max(s3,s4)`; 1280 = IPv6-минимум, безопасен на PPPoE/мобайл/вложенных путях. **Сервер должен иметь симметричный MTU** — иначе крупные обратные пакеты упрутся в path-MTU так же.

**Пример (AWG2):**
```
wireguard://privkey-base64@server.example.com:51821?publickey=server-pubkey&address=10.0.0.2%2F32&allowedips=0.0.0.0%2F0%2C%3A%3A%2F0&keepalive=25&jc=10&jmin=50&jmax=100&s1=20&s2=20&s3=60&s4=60&h1=1234567890&h2=1234567891&h3=1234567892&h4=1234567893&i1=%3Cb%200x000100002112a442%3E%3Cr%2012%3E#AWG2
```
(`i1` здесь — URL-encoded `<b 0x000100002112a442><r 12>`.) Поддержка реализована в `applyAWGFields` / `ShareURIFromWireGuardEndpoint` (`core/config/subscription/node_parser_wireguard.go`, `shareuri_wireguard.go`); рантайм — на ядре с `with_awg`. См. `SPECS/073-F-N-AMNEZIAWG_PARAMS/SPEC.md` и `sing-box-lx/docs/lx-config.md`.

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
   - Сначала читается шаблон (`bin/wizard_template.json`)
   - В шаблоне находятся все outbounds с полем `wizard.required > 0`
   - Для каждого такого outbound проверяется, есть ли он в текущем ParserConfig (загруженном из config.json)
   - **Логика проверки:**
     - **`required: 0` или отсутствует** — outbound игнорируется (не проверяется)
     - **`required: 1`** — проверяется только наличие тега; если outbound отсутствует в config.json, он добавляется из шаблона; если присутствует, сохраняется существующая версия из config.json
     - **`required > 1` (например, `2`)** — всегда переписывается из шаблона, независимо от наличия в config.json или соответствия шаблону
   - **Формат**: Используйте формат `"wizard": {"hide": true, "required": 2}`. Старый формат `"wizard": "hide"` поддерживается для обратной совместимости, но без поля `required`.

3. **Fallback: использование шаблона** (если config.json не существует или не содержит ParserConfig)
   - Если `config.json` не существует или не содержит валидный ParserConfig, используется шаблон (`bin/wizard_template.json`)
   - Все outbounds и proxies берутся из шаблона

### Пример работы

**Шаг 1: Чтение шаблона** (`wizard_template.json`):
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

