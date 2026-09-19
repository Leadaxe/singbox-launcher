# Протоколы и форматы ссылок singbox-launcher

**🌐 Язык**: [English](Protocols.md) | Русский

## Справочник по схемам генерируется из реестра

Таблицы, которые жили здесь раньше — какие протоколы лаунчер понимает, какие
параметры читает у каждой ссылки и во что они превращаются в `config.json`, —
теперь **генерируются из `contract/registry/`**, реестра, общего с мобильным
приложением LxBox. Один источник, один набор правил, оба приложения.

📄 **[`contract/docs/generated/index.md`](../contract/docs/generated/index.md)** —
оглавление: каждая схема со своим `singbox_type`, источниками и наличием формы
ссылки, плюс общие суб-схемы (TLS/REALITY, транспорты, multiplex, dialer) и
лимиты контракта. У каждой схемы своя страница с двумя таблицами: параметры
ссылки (`uri.query.*` → путь в теле, куда параметр переводится) и поля тела
(тип, допустимые значения, дефолт, что санитайзер делает с негодным значением).

⚠️ **[`contract/docs/generated/warnings.md`](../contract/docs/generated/warnings.md)** —
коды деградации на узле: severity, текст, который показывает приложение, и
обратный индекс «какое поле какой схемы ставит этот код». Окно Info узла ведёт
прямо на якорь оттуда (`warnings.md#<code>`).

Оба файла пересобираются командой `go generate ./contract/...`, и CI-джоба
`Contract` сверяет их на каждом PR: **руками не править — менять надо реестр.**
Реестр нормативен; при расхождении с чем-либо на этой странице прав реестр.

Настройка самого парсера — источники, фильтры, направления (`outbounds`),
маркерные секции, визард — в отдельном документе
[**`ParserConfig.ru.md`**](ParserConfig.ru.md).

## Что осталось описано здесь

Реестр описывает узел: его схемы, параметры и поля. Он не описывает **обвязку
лаунчера вокруг** узла — её и держит остаток этой страницы:

- [Транспорт xhttp и AmneziaWG](#транспорт-xhttp-и-amneziawg) — теги сборки, которые нужны ядру форка
- [JSON-массив полных конфигов Xray/V2Ray](#json-массив-полных-конфигов-xrayv2ray) — вид тела подписки
- [Коды деградации на узле](#коды-деградации-на-узле) — как код попадает на узел и где живёт
- [Share URI из outbound](#share-uri-из-outbound-и-wireguard-endpoint-обратно-к-ссылке) — обратное направление, из `config.json` снова в ссылку
- [Документы и исходный код парсера URI](#документы-и-исходный-код-парсера-uri)
- [Голый `.conf`-текст, Amnezia `vpn://`, добавление из файла](#amnezia-vpn) — входные формы, которые не являются ссылками

## Транспорт xhttp и AmneziaWG

Лаунчер собран под ядро **[sing-box-lx](https://github.com/Leadaxe/sing-box-lx)** (upstream sing-box + ровно две клиентские фичи под build-тегами). Парсер/генератор/share-URI лаунчера поддерживают обе сквозно; в рантайме они работают **только** на ядре с соответствующим тегом — на стоковом sing-box конфиг с этими полями отвергается на load-time (явная ошибка, без тихого даунгрейда).

**✅ `xhttp` транспорт — полноценно (build-tag `with_xhttp`).** Прежняя деградация в `httpupgrade` снята. При `type=xhttp` (VLESS/Trojan) или `net=xhttp` (VMess) строится честный транспорт `type:"xhttp"` (Xray-совместимый splithttp) со всеми полями, и без потерь сериализуется обратно в share-URI:

- Поля: `mode` (`auto` \| `packet-up` \| `stream-up` \| `stream-one`; у форка `auto`=`packet-up`, у `stream-one` известный баг downlink-framing), `host`, `path`, `headers`, `x_padding_bytes` (диапазон `"min-max"`, дефолт `100-1000`, несётся в заголовке `Referer`), `no_grpc_header`. Композится с TLS/Reality (не с XTLS-Vision — ограничение протокола).
- `httpupgrade` теперь **отдельный** транспорт (HTTP/1.1 Upgrade) — больше не путается с xhttp ни на входе, ни на выходе share-URI.
- Детали: `SPECS/071-F-N-XHTTP_TRANSPORT/SPEC.md`, `sing-box-lx/docs-lx/lx-config.md`.

**✅ AmneziaWG 2.0 (AWG2) — обфускация WireGuard (build-tag `with_awg`).** WireGuard-endpoint (`wireguard://`) может нести promoted-поля AWG: числа `jc`/`jmin`/`jmax`, `s1`–`s4`, `h1`–`h4` и CPS-строки `i1`–`i5` (AWG 2.0, case-sensitive tag-формат). `h1`–`h4` — одиночное число **или диапазон** `lo-hi` (header randomization AWG 2.0; ядро ≥ `1.13.13-lx.6` само выбирает значение per-handshake — сабтаска 073.2). Источники импорта: `wireguard://`/`awg://`-URI, `vpn://`-профили Amnezia (SPEC 075) и вставленный `.conf`-текст (SPEC 076); эмиссия в `endpoints[]`, round-trip в share-URI без потерь. Endpoint **без** AWG-полей — обычный WireGuard (byte-identical с апстримом). Детали полей — на сгенерированной странице [`wireguard`](../contract/docs/generated/protocols/wireguard.md); `SPECS/073-F-N-AMNEZIAWG_PARAMS/SPEC.md`, `sing-box-lx/docs-lx/lx-config.md`.

Подробности по каждой схеме (query-параметры, TLS, transport, edge cases) — на сгенерированных страницах: [`contract/docs/generated/index.md`](../contract/docs/generated/index.md).

## JSON-массив полных конфигов Xray/V2Ray

Если тело подписки (plain или после декодирования Base64) — **валидный JSON-массив** `[...]`, а элементы похожи на Xray (`outbounds[].protocol`, VLESS с `settings.vnext`), лаунчер обрабатывает его как подписку: из **каждого элемента** извлекается **одна** логическая нода. Для разбора используются поля **`outbounds`** и (при наличии) **`remarks`**; корневые **`dns`**, **`routing`**, **`inbounds`** и прочее из элемента **не** подмешиваются в общий конфиг лаунчера.

**Как отличить Xray-массив от sing-box-массива (016, не реализовано)**

| Шаг | Эвристика |
|-----|-----------|
| Декодер | После trim строка начинается с **`[`**, **`json.Valid`**, успешный `json.Unmarshal` в массив — тело не отвергается как «не подписка» (`DecodeSubscriptionContent`). |
| Вход в парсер | **`IsXrayJSONArrayBody`**: то же — префикс `[`, валидный JSON, массив объектов. |
| Элемент массива | **`xrayElementHasProtocolOutbounds`**: в **`outbounds`** есть хотя бы один объект с полем **`protocol`** (строка) — признак **Xray-диалекта**. Элементы только с sing-box **`type`** без **`protocol`** не считаются Xray для этой ветки и **пропускаются** с `debuglog` (ожидается follow-up **016**). |
| Нода | Основной outbound элемента выбирается на уровне документа, а переводит его движок реестра (`parseXrayElementByEngine` → `core/config/linkmap`); какая секция `mappers.xray` его ведёт, решает её собственный `detect`, а не список протоколов в коде. При **`dialerProxy`** hop разбирается как **`socks`** или **`vless`** (`xrayChainHopFromOutbound`; socks-звено — `xrayBuildJumpFromSocksOutbound`); иные `protocol` у hop — пропуск элемента (`WarnLog`). |

**`remarks` и теги sing-box**

- В **`ParsedNode.Label`** попадает полный текст **`remarks`** (если пусто — запасной вариант: тег основного Xray-outbound или `xray-{индекс}`).
- **Теги** генерируемых outbound в sing-box: если **`remarks`** непустой, из него строится **slug** (буквы/цифры в любой скрипте, **символы региональных индикаторов** для UTF-флагов, нормализация через `textnorm`, обрезка длины; прочие знаки и emoji кроме флагов в slug не входят). **Основной** outbound получает тег **`{slug}`**; при цепочке через SOCKS второй outbound (jump) — **`{slug}_jump_server`**, а у основного в JSON задаётся **`detour`** на этот тег. Если **`remarks`** пустой — **`xray-{индекс}`** и **`xray-{индекс}_jump_server`**. Далее, как у обычных подписок, применяются **`tag_prefix` / `tag_postfix` / `tag_mask`**, **`textnorm.NormalizeProxyDisplay`** и **`MakeTagUnique`** (в т.ч. для jump).
- В сгенерированном фрагменте `config.json` над outbound по-прежнему пишется **комментарий** `// …` из **`Label`** (полный `remarks`), т.к. у sing-box нет поля «remarks» в outbound.

**Цепочка `dialerProxy`**

При **`streamSettings.sockopt.dialerProxy`** (или **`dialer`**) → outbound с тем же **`tag`**: поддерживаются hop’ы **`protocol: socks`** и **`protocol: vless`**; в `config.json` сначала генерируется outbound hop’а, затем основной (VLESS и т.д.) с полем **`detour`** на тег hop’а. Если outbound по тегу не найден или **`protocol`** hop’а не **socks** / не **vless** — элемент массива **не** даёт ноды (`WarnLog`). Детали и расширение на другие типы: **`SPECS/036-F-C-XRAY_JUMP_ANY_PROTOCOL/SPEC.md`**. Массив конфигов **только в формате sing-box** (`type` в outbounds без Xray-`protocol`) в MVP **не** разбирается (follow-up **016**).

**Пример и код**

Структура как у публичных Xray-подписок (**`dns`**, **`inbounds`**, **`log`**, **`mux`**, **`tcpSettings`**, **`routing`**, **`freedom`/`blackhole`**), с вымышленными данными: **`docs/examples/xray_subscription_array_sample.json`**. Тот же сценарий в тестах: **`core/config/subscription/testdata/xray_provider_anon.json`** (`go:embed` в **`xray_json_array_test.go`**). Реализация: **`xray_json_array.go`**, **`xray_outbound_convert.go`** и **`xray_protocols.go`** (уровень **документа**: какой элемент становится узлом, какой звеном цепочки, какой группой-балансером), **`xray_element_engine.go`** (сам элемент — движку реестра), **`decoder.go`** (`DecodeSubscriptionContent`), **`source_loader.go`** (`LoadNodesFromSource`, **`applyTagsToXrayNode`**), configurator: **`ui/configurator/tabs/source_tab.go`** (`refreshOneSourceFromUI`).

## Коды деградации на узле

Ссылка из публичной подписки сплошь и рядом кривая не по вине пользователя:
мусорный `fp=`, `packet_encoding` вне allowlist, неизвестный ядру тип обфускации.
Правило парсера — **деградируй узел, а не конфиг**: одно битое значение не должно
заставлять `sing-box check` отвергнуть весь файл и оставить пользователя без VPN.

Но деградация, доехавшая только до `debuglog`, невидима: UI и LxBox показывают
узел как ни в чём не бывало. Поэтому выживший узел несёт машиночитаемые коды
всего, что было молча подправлено:

- `configtypes.ParsedNode.Warnings []string`, добавляются через `AddWarning`
  (с дедупликацией).
- Нормативный словарь — **`contract/registry/warnings.json`**, общий с LxBox:
  обе стороны сообщают об одном событии одним именем.
- Go-константы живут в `core/config/subscription/parse_warnings.go`.

Коды делятся на два вида, и деление осознанное:

| Вид | Severity | Где живёт |
|---|---|---|
| Узел **выжил**, значение подправлено | `info` / `warning` | на узле, в `Warnings[]` |
| Узел **отброшен** на разборе | `error` | в причине отброса — объекта `ParsedNode` не существует |

📄 **Сами коды — все до одного, с обоими текстами и полем, которое их ставит, —
в [`contract/docs/generated/warnings.md`](../contract/docs/generated/warnings.md).**
Эта страница генерируется из реестра, поэтому отстать от правил не может; здесь
объясняется только, как код попадает на узел, а не какие коды бывают.

Тест-страж (`registry_sync_test.go`) держит обе стороны честными: каждый Go-код
обязан быть в реестре, а код, который никогда не вешается на узел, обязан быть
объявлен как `severity: error`.

## Документы и исходный код парсера URI

| Документ / место | Содержание |
|------------------|------------|
| **Этот файл** (`docs/ParserConfig.md`) | Форматы прямых ссылок в `connections`, Share URI, структура ParserConfig, пайплайн обновления. |
| **`contract/registry/protocols/<scheme>.json`** | **Нормативный справочник полей**, общий с мобильным приложением LxBox (SPEC 103): query-параметры каждой схемы, алиасы, allowlist'ы, правила деградации и пометки, что где реализовано. При расхождении этого файла с реестром прав реестр. |
| **`contract/docs/CANON.md`, `IDENTITY.md`** | Как канонизируется разобранный узел (без дефолтов, без `tag`/`detour`, сортировка ключей) и как считается его identity-хеш — оба документа общие с LxBox. |
| **`contract/corpus/uri/`** | Конформанс-фикстуры, которые гоняют оба проекта (`core/config/contract_test.go` здесь, `test/contract/` там). Правка парсера, меняющая поведение, видна как дифф корпуса. |
| **`SPECS/023-F-C-SUBSCRIPTION_TRANSPORT_VLESS_TROJAN/SUBSCRIPTION_PARAMS_REPORT.md`** | Таблицы: query VLESS/Trojan → поля sing-box; примеры из публичных подписок; ключи query. |
| **`SPECS/029-Q-С-SUBSCRIPTION_PARSER_CLASH_CONVERTOR_PARITY/SPEC.md`** | Расширения совместимости (029): `type=httpupgrade`, `peer`, `obfsParam`, VMess legacy / `httpupgrade` / `h2`, Hysteria2 TLS; сверка со схемой sing-box. |
| **`SPECS/033-F-N-SUBSCRIPTION_XRAY_JSON_ARRAY/SPEC.md`** | Подписка как JSON-массив полных конфигов Xray: `remarks`, slug-теги, `dialerProxy` → `detour`, границы MVP (sing-box-массив — **016**, follow-up). |
| **`SPECS/036-F-C-XRAY_JUMP_ANY_PROTOCOL/SPEC.md`** | `dialerProxy`: hop **SOCKS** или **VLESS**; прочие протоколы — по мере маппинга (**завершено** по объёму SPEC). |
| **`contract/docs/MAPPER_ENGINE.md`** | Как разбирается источник: стадии конвейера, пространство источников, порядок исполнения записей реестра, обратный ход. Источник истины — общий документ контракта. |
| Пакет **`core/config/linkmap`** | Движок: один путь разбора на все источники, по таблицам `mappers.*`. Имён схем и протоколов в нём нет (сторожит `no_scheme_names_test.go`). |
| Пакет **`core/config/subscription`** | `ParseNode` — три ветки (Amnezia `vpn://`, движок через `node_parser_engine.go`, «схема не поддержана») и общие хелперы в `node_parser_core.go`; решения уровня документа по массиву Xray — `xray_json_array.go`, `xray_element_engine.go`, `xray_outbound_convert.go`, `xray_protocols.go`, `xray_balancer.go`; вставленный текст wg-quick — `wgconf_text.go`; share URI — `share_uri.go`. |

## Share URI из outbound и WireGuard endpoint (обратно к ссылке)

Спецификация фичи (ПКМ на вкладке Servers, контекстное меню, детали реализации): **`SPECS/025-F-C-SERVERS_CONTEXT_MENU_SHARE_URI/`** (SPEC, PLAN, IMPLEMENTATION_REPORT).

Парсер переводит **строку подписки** (`ParseNode` или для WireGuard — объект в `endpoints[]`) в JSON sing-box. Обратная операция — **сборка share URI из уже записанного outbound или WireGuard endpoint** в `config.json`, чтобы делиться ссылкой без хранения исходной строки подписки.

### Принцип и соответствие форматам

- **Вход кодировщика:** один элемент массива `outbounds` **или** один элемент `endpoints[]` с `type: wireguard` (тот же набор полей, что дают секция `wireguard` реестра и `GenerateEndpointJSON`).
- **Выход:** одна строка URI в форматах, которые снова понимает этот проект: `vless://`, `vmess://` (base64 JSON), `trojan://`, `ss://` (SIP002), `socks5://`, `hysteria2://`, `tuic://`, `ssh://`, **`wireguard://`**.
- **Query / transport / TLS:** соглашения кодирования и разбора совпадают, потому что читаются ОДНИ И ТЕ ЖЕ таблицы реестра в обе стороны — вид ссылки есть свойство схемы, а не рукописного эмиттера (`contract/docs/MAPPER_ENGINE.md` §9). Подробный справочник VLESS/Trojan: **`SUBSCRIPTION_PARAMS_REPORT.md`** (023); расширения 029 — спека **`029-Q-С-…/SPEC.md`** и [сгенерированные страницы схем](../contract/docs/generated/index.md).

### API в коде

| Функция | Пакет | Назначение |
|--------|--------|------------|
| `ShareURIFromOutbound(out map[string]interface{})` | `core/config/subscription` (`share_uri.go`) | Кодирование из JSON-объекта outbound; для `type: wireguard` делегирует в `ShareURIFromWireGuardEndpoint` |
| `ShareURIFromWireGuardEndpoint(ep map[string]interface{})` | `core/config/subscription` (`share_uri.go`) | Кодирование `wireguard://` из одного endpoint (один peer в `peers[]`) |
| `GetOutboundMapByTag(configPath, tag)` | `core/config` (`outbound_share.go`) | Поиск outbound по полю `tag` в `config.json` |
| `GetEndpointMapByTag(configPath, tag)` | `core/config` (`outbound_share.go`) | Поиск endpoint по полю `tag` в `endpoints[]` |
| `ShareProxyURIForOutboundTag(configPath, tag)` | `core/config` (`outbound_share.go`) | Сначала outbound по тегу, иначе WireGuard в `endpoints[]` |

Ошибка **`ErrShareURINotSupported`** (`subscription`) — тип outbound не кодируется в один URI или не хватает полей.

### Поддерживаемые типы `outbound.type`

| `type` в JSON | Схема URI | Замечания |
|---------------|-----------|-----------|
| `vless` | `vless://` | `encryption=none`, transport/TLS как в подписках |
| `vmess` | `vmess://` + base64 | Поля JSON контейнера объявлены секцией `vmess` реестра — теми же, что читает разбор |
| `trojan` | `trojan://` | Пароль в userinfo |
| `shadowsocks` | `ss://` | SIP002, base64(`method:password`) |
| `socks` | `socks5://` | `version` 5; user/password при наличии |
| `hysteria2` | `hysteria2://` | TLS SNI, `mport`, obfs и т.д. по возможности |
| `tuic` | `tuic://` | `uuid:password`; `congestion_control`, `udp_relay_mode`, `zero_rtt_handshake`, `heartbeat`; `alpn`/`sni`/`insecure` из TLS |
| `ssh` | `ssh://` | **Нет** кодирования inline `private_key` в URI; путь к ключу и прочие поля — в query, как в документации SSH URI |
| `naive` | `naive+https://` / `naive+quic://` | HTTP/2 (`naive+https`) или QUIC (`naive+quic`); user/pass в userinfo; `extra-headers` в query с `\r\n`-разделёнными парами (см. [`naive`](../contract/docs/generated/protocols/naive.md)). Требует sing-box **≥ 1.13.0** с build tag `with_naive_outbound` (ядро форка `1.14.0-lx.4+`). |
| `anytls` | `anytls://` | Пароль в userinfo; TLS-блок обязателен; поля session-пула, если заданы |
| `masque` | `masque://` | Ключи base64(DER) в userinfo/`publickey=`, `ip`/`ipv6` склеиваются в `address=`; `vhttp` и `tls.server_name` → `vhttp=`/`sni=`. Требует ядро `1.14.0-lx.26+`. |
| `wireguard` | `wireguard://` | Обычно узел только в `endpoints[]`; формат и query — на странице [`wireguard`](../contract/docs/generated/protocols/wireguard.md). **Один URI ↔ один удалённый peer:** при нескольких элементах в `peers[]` кодирование не поддерживается (`ErrShareURINotSupported`). |

**Не кодируются в один share URI:** `selector`, `urltest`, `direct`, `block`, `dns`, `http`, произвольные служебные типы; WireGuard с **несколькими** `peers`; outbound с непустым **`detour`** (цепочка через jump из подписки Xray JSON).

### GUI

Вкладка **Servers** (список прокси Clash API): **ПКМ** по строке → `serversProxyContextMenu`: первая строка — **`api.ProxyInfo.ContextMenuTypeLine`** (нижний регистр поля **`type`** из API или `servers.menu_context_type_unknown`); затем **«Копировать ссылку»** (`servers.menu_copy_link`). Верхняя строка без `Disabled`, `Action: nil` (цвет текста как у обычного пункта меню). В буфер попадает строка через `config.ShareProxyURIForOutboundTag` и путь `FileService.ConfigPath`: сначала outbound по тегу, иначе WireGuard в `endpoints[]`. Правый клик по кнопкам Ping/Switch может не открыть меню (иерархия hit-test Fyne). Сообщения статуса: `servers.copy_link_resolving`, `servers.copy_link_done`, `servers.copy_link_not_supported`.

### Тесты

Round-trip и выборочные сценарии: `core/config/subscription/share_uri_encode_test.go`, интеграция с файлом конфига: `core/config/outbound_share_test.go`.

## Входные формы, которые не являются ссылками

Узел приезжает не всегда в виде URI. Прежнее утверждение здесь больше не верно:
формы ниже **не сводятся к промежуточной ссылке `wireguard://`**. Текст wg-quick
`.conf` ведёт своя секция реестра — **`mappers.conf`** поверх ini-пространства, а
профиль Amnezia распаковывается в такой же текст `.conf` и уезжает в ту же секцию.
Обход через ссылку снят потому, что терял то, чего в ссылке нет по построению: код
`wgconf_dns_ignored` и метку из комментария `[Peer]`.

### Amnezia (`vpn://`)

Ссылки **`vpn://…`**, которые экспортирует Amnezia VPN / AmneziaWG 2.0 (файл `.vpn` — это одна такая ссылка), принимаются напрямую: вставьте ссылку в Sources или Connections. Формат (эталон — `amnezia-vpn/config-decoder`): `vpn://` + base64url без padding, внутри qCompress (4 байта big-endian длины + zlib), под ним JSON всего профиля Amnezia.

Из профиля импортируется **только WireGuard/AmneziaWG-контейнер** (OpenVPN/Cloak/XRay-контейнеры пропускаются): сначала пробуется `defaultContainer`, затем остальные по порядку. Найденный `[Interface]/[Peer]`-конфиг уезжает в секцию `mappers.conf` реестра (набор полей — на странице [`wireguard`](../contract/docs/generated/protocols/wireguard.md)), поэтому применяются те же правила, что и у вставленного `.conf`: нормализация голых IP до CIDR, promote AWG-полей `Jc`/`Jmin`/`Jmax`/`S1`–`S4`/`H1`–`H4`/`I1`–`I5` в корень endpoint и **кламп MTU AWG-эндпоинта до 1280** — `MTU = 1420` из амнезиевского конфига заведомо ломает передачу данных (`sendmsg: message too long`). Имя профиля (`description`, затем `hostName`, затем имя контейнера) едет источником `hint`, а **куда его поставить в цепочке метки, решает секция**, а не вызывающий: комментарий под `[Peer]` его перебивает, хост `Endpoint` — последнее звено.

Лимиты: ссылка до 512 КБ, распакованный профиль до 8 МБ (защита от zlib-бомб). Профиль без WG/AWG-контейнера даёт ошибку с перечислением контейнеров. Реализация: `core/config/subscription/node_parser_amnezia.go`; спека: `SPECS/075-F-C-AMNEZIA_VPN_IMPORT/SPEC.md`; референс-декодер для отладки: `scripts/decode_amnezia_vpn.py`.

### Голый `.conf`-текст (`[Interface]/[Peer]`)

Содержимое `.conf`-файла WireGuard/AmneziaWG можно вставить в поле Add вкладки Sources **как есть** — классификатор сам выделяет `[Interface]`-блоки из вставленного текста до построчного разбора, и каждый блок ведёт секция `mappers.conf` реестра. Несколько блоков за одну вставку → несколько узлов; ссылки в том же тексте продолжают работать. Имя узла — комментарий сразу под `[Peer]`, если провайдер его написал (в `.conf` это единственное человекочитаемое имя), иначе хост из `Endpoint`. AWG-поля и кламп MTU — как у `vpn://` выше. Невалидный блок пропускается с предупреждением в лог, не срывая вставку. Реализация: `core/config/subscription/wgconf_text.go` + врезка в `classifyInputLines` (`ui/configurator/business/parser.go`); спека: `SPECS/076-F-C-WGCONF_PASTE_IMPORT/SPEC.md`.

### Подписка, отдающая `.conf` или профиль `vpn://`

Выше описано то, что пользователь может **вставить** руками. Но подписка по
ссылке может и *отдавать* такое тело, и до фазы 2 SPEC 103 оно давало ноль
узлов без единого сообщения: всё, что не JSON, уходило в построчный разбор
ссылок, который не находил ни одной.

Обе формы теперь распознаются как отдельные виды тела:

- **wg-quick `.conf`** (`BodyKindWGConf`) — тело сводится к каноническим
  `wireguard://`-URI *до* ветвления, поэтому AWG-поля и кламп MTU продолжают
  работать через единственный парсер, где они уже реализованы. Несколько
  секций `[Interface]` в одном файле дают несколько узлов; блок без
  `[Peer] Endpoint` пропускается со счётчиком, а не обнуляет подписку целиком.
- **Amnezia `vpn://`** (`BodyKindVPNLink`) — распознаётся раньше base64-эвристики
  (`:` не входит в base64-алфавит). Импортируются **все** WG/AWG-контейнеры
  профиля, а не только дефолтный: профиль с несколькими локациями — штатный
  случай Amnezia. Порядок детерминирован (дефолтный контейнер первым), а метка
  узла дополняется именем контейнера — иначе `MakeTagUnique` превратил бы
  локации в «…-2»/«…-3», и отличить их было бы нечем. Несжатые профили (голый
  base64-JSON, который Amnezia тоже экспортирует) принимаются наравне с
  qCompress-формой.

Реализация: `core/config/subscription/body_classify.go`, `wgconf_text.go`
(`WGConfBodyToURIs`), `node_parser_amnezia.go` (`ParseAmneziaVPNLinkAll`).
Фикстуры: `contract/corpus/body/`.


### Добавление из файла (Add from file)

Конфиги WG/AmneziaWG часто раздают файлом — кнопка **«Add from file»** на вкладке Sources (рядом с Get free) открывает **нативное системное окно** выбора файла (`.conf` / `.vpn` / `.txt`) и прогоняет его содержимое через тот же путь, что и поле Add: `.conf` → WG/AWG-узел, `.vpn` → профиль Amnezia, текст со ссылками → узлы. Лимит файла — 1 МБ. Нативный диалог: `osascript` (macOS), PowerShell `OpenFileDialog` (Windows), `zenity`/`kdialog` (Linux); если на Linux ни того, ни другого нет — fallback на встроенный Fyne-диалог. Реализация: `platform.PickOpenFile` (SPEC 082) + `business.ReadSourceFileText` в `ui/configurator/tabs/source_tab.go`; спеки: `SPECS/079-F-N-ADD_SOURCE_FROM_FILE/`, `SPECS/082-F-N-NATIVE_FILE_PICKER/`.
