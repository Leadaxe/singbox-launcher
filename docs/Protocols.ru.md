# Протоколы и форматы ссылок singbox-launcher

**🌐 Язык**: [English](Protocols.md) | Русский

Справочник по **форматам URI**: какие протоколы лаунчер понимает, какие
параметры читает у каждого и во что они превращаются в `config.json`.

Настройка самого парсера — источники, фильтры, направления (`outbounds`),
маркерные секции, визард — в отдельном документе
[**`ParserConfig.ru.md`**](ParserConfig.ru.md).

## Содержание

- [Поддерживаемые протоколы](#поддерживаемые-протоколы) — таблица 14 схем
- [Транспорт xhttp и AmneziaWG](#транспорт-xhttp-и-amneziawg)
- [JSON-массив полных конфигов Xray/V2Ray](#json-массив-полных-конфигов-xrayv2ray)
- [Коды деградации на узле](#коды-деградации-на-узле)
- [Share URI из outbound](#share-uri-из-outbound-и-wireguard-endpoint-обратно-к-ссылке) — обратное направление
- [Форматы URI по протоколам](#форматы-uri-для-прямых-ссылок) — параметры каждой схемы

### Поддерживаемые протоколы

| # | Схема URI | sing-box `type` | Секция конфига | Версия / build-tag | Описание |
|---|-----------|-----------------|----------------|--------------------|----------|
| 1 | `vless://` | `vless` | `outbounds[]` | core (+ **`with_xhttp`** для xhttp) | TCP/raw/ws/grpc/http/`httpupgrade`/quic/**`xhttp`** (splithttp), TLS, Reality, Vision flow. xhttp — нативно на ядре sing-box-lx (см. ниже). |
| 2 | `vmess://` | `vmess` | `outbounds[]` | core (+ **`with_xhttp`**) | Base64 JSON или legacy cleartext `method:uuid@host:port`. `net=h2`→`http`+TLS; `net=xhttp`→**`xhttp`**, `net=httpupgrade`→`httpupgrade` (разные транспорты). |
| 3 | `trojan://` | `trojan` | `outbounds[]` | core | Те же transport/TLS, что и VLESS. Пароль в userinfo. |
| 4 | `ss://` | `shadowsocks` | `outbounds[]` | core | SIP002 + legacy `ss://base64("method:password@host:port")`. Методы — фиксированный allow-list (2022-blake3, AEAD GCM, ChaCha20-Poly1305). |
| 5 | `hysteria2://`, `hy2://` | `hysteria2` | `outbounds[]` | core (QUIC) | Multi-port (`mport`/`ports` query или `host:123,5000-6000` в authority); obfs — `salamander` или `gecko` (ядро форка умеет оба). |
| 6 | `ssh://` | `ssh` | `outbounds[]` | core | **Собственный URI-диалект singbox-launcher**, не RFC. Inline-ключ / путь к ключу / passphrase / host_key. |
| 7 | `socks5://`, `socks://` | `socks` (version=5) | `outbounds[]` | core | User/pass опциональны. Поле фильтра `scheme` сохраняет оригинал (`socks5` vs `socks`). |
| 8 | `naive+https://`, `naive+quic://` | `naive` | `outbounds[]` | **sing-box ≥ 1.13.0** + build tag **`with_naive_outbound`** (ядро форка `1.14.0-lx.4+` — все desktop-платформы; на Windows нужен `libcronet.dll`, лаунчер ставит его сам). На ядре без поддержки ноды **деградируются с warning**, конфиг не ломается. | DuckSoft 2020 URI-диалект. `extra-headers=` (CRLF-разделённые пары). TLS только `server_name`. |
| 9 | `wireguard://` | `wireguard` | **`endpoints[]`** | **sing-box ≥ 1.11** (+ **`with_awg`** для AmneziaWG) | Один peer; маркеры `@ParserSTART_E`/`@ParserEND_E`. Default port 51820, mtu 1420. Опциональные параметры **AmneziaWG 2.0** (jc/jmin/jmax, s1–s4, h1–h4, i1–i5) — см. ниже. |
| 10 | `tuic://` | `tuic` | `outbounds[]` | core (QUIC) | TUIC v5: `uuid:password` в userinfo. Query: `congestion_control` (cubic/new_reno/bbr), `udp_relay_mode` (native/quic), `alpn`, `sni`, `allow_insecure`, `reduce_rtt`/`zero_rtt_handshake`, `heartbeat`, `fp`. TLS обязателен (QUIC). |
| 11 | `vpn://` | `wireguard` | **`endpoints[]`** | как №9 | Профиль **Amnezia** (`.vpn`-файл: base64url + qCompress + JSON, SPEC 075): импортируется WG/AWG-контейнер, конвертация в канонический `wireguard://`-URI. См. секцию Amnezia (`vpn://`) ниже. |
| 12 | `masque://` | `masque` | `outbounds[]` | **ядро форка `1.14.0-lx.26+`** (схема `vhttp`+`tls`, core SPEC 062) | **Собственный URI-диалект singbox-launcher.** MASQUE / CONNECT-IP (RFC 9484) — целые IP-пакеты поверх HTTP/3 (`h3`) или HTTP/2 (`h2`), в первую очередь Cloudflare WARP. Ключи base64(DER), адреса туннеля в `address=`. Узлы обычно создаёт мастер WARP. См. секцию MASQUE (`masque://`) ниже. |
| 13 | `anytls://` | `anytls` | `outbounds[]` | core (ядро rc.17+: `option/anytls.go`) | Пароль в userinfo, обязательный TLS-блок; тюнинг session-пула (`idle_session_check_interval`, `idle_session_timeout`, `min_idle_session`). Спека SPEC 091. |
| 14 | `proxy-http://`, `proxy-https://` (алиасы `proxy+http://`, `proxy+https://`) | `http` | `outbounds[]` | core | **HTTP(S) CONNECT-прокси** (SPEC 103 §9.B6, конвенция LxBox). Учётные данные в userinfo; `proxy-https` добавляет TLS (порт по умолчанию 443, без TLS — 80). Своя схема вместо голого `http(s)://`, потому что тот означает **источник-подписку** и перехватывается выше по стеку раньше, чем мог бы быть прочитан как узел. |

Кроме URI, поле Add принимает **голый `[Interface]/[Peer]`-текст** (`.conf` WireGuard/AmneziaWG, включая AWG-поля) — conf-блоки распознаются до построчного разбора и конвертируются в `wireguard://`-URI (SPEC 076); см. секцию про `.conf`-текст ниже.

**Не поддерживаются** (явно, не реализованы): **ShadowTLS**, **Mieru**, **Hysteria 1** (только v2), **ShadowsocksR / SSR**, **Tor**, plain HTTP-proxy как тип ноды (URL `http(s)://...` — это всегда **источник подписки**, не нода). Селекторы (`selector`, `urltest`, `direct`, `block`, `dns`) — не URI-протоколы; собираются на стороне ParserConfig (см. [секцию `outbounds`](ParserConfig.ru.md#секция-outbounds)).

### Транспорт xhttp и AmneziaWG

Лаунчер собран под ядро **[sing-box-lx](https://github.com/Leadaxe/sing-box-lx)** (upstream sing-box + ровно две клиентские фичи под build-тегами). Парсер/генератор/share-URI лаунчера поддерживают обе сквозно; в рантайме они работают **только** на ядре с соответствующим тегом — на стоковом sing-box конфиг с этими полями отвергается на load-time (явная ошибка, без тихого даунгрейда).

**✅ `xhttp` транспорт — полноценно (build-tag `with_xhttp`).** Прежняя деградация в `httpupgrade` снята. При `type=xhttp` (VLESS/Trojan) или `net=xhttp` (VMess) строится честный транспорт `type:"xhttp"` (Xray-совместимый splithttp) со всеми полями, и без потерь сериализуется обратно в share-URI:

- Поля: `mode` (`auto` \| `packet-up` \| `stream-up` \| `stream-one`; у форка `auto`=`packet-up`, у `stream-one` известный баг downlink-framing), `host`, `path`, `headers`, `x_padding_bytes` (диапазон `"min-max"`, дефолт `100-1000`, несётся в заголовке `Referer`), `no_grpc_header`. Композится с TLS/Reality (не с XTLS-Vision — ограничение протокола).
- `httpupgrade` теперь **отдельный** транспорт (HTTP/1.1 Upgrade) — больше не путается с xhttp ни на входе, ни на выходе share-URI.
- Детали: `SPECS/071-F-N-XHTTP_TRANSPORT/SPEC.md`, `sing-box-lx/docs-lx/lx-config.md`.

**✅ AmneziaWG 2.0 (AWG2) — обфускация WireGuard (build-tag `with_awg`).** WireGuard-endpoint (`wireguard://`) может нести promoted-поля AWG: числа `jc`/`jmin`/`jmax`, `s1`–`s4`, `h1`–`h4` и CPS-строки `i1`–`i5` (AWG 2.0, case-sensitive tag-формат). `h1`–`h4` — одиночное число **или диапазон** `lo-hi` (header randomization AWG 2.0; ядро ≥ `1.13.13-lx.6` само выбирает значение per-handshake — сабтаска 073.2). Источники импорта: `wireguard://`/`awg://`-URI, `vpn://`-профили Amnezia (SPEC 075) и вставленный `.conf`-текст (SPEC 076); эмиссия в `endpoints[]`, round-trip в share-URI без потерь. Endpoint **без** AWG-полей — обычный WireGuard (byte-identical с апстримом). Детали полей — секция [WireGuard](#wireguard-wireguard) ниже; `SPECS/073-F-N-AMNEZIAWG_PARAMS/SPEC.md`, `sing-box-lx/docs-lx/lx-config.md`.

Подробности по каждой схеме (query-параметры, TLS, transport, edge cases) — в разделе [Форматы URI для прямых ссылок](#форматы-uri-для-прямых-ссылок) ниже.

### JSON-массив полных конфигов Xray/V2Ray

Если тело подписки (plain или после декодирования Base64) — **валидный JSON-массив** `[...]`, а элементы похожи на Xray (`outbounds[].protocol`, VLESS с `settings.vnext`), лаунчер обрабатывает его как подписку: из **каждого элемента** извлекается **одна** логическая нода. Для разбора используются поля **`outbounds`** и (при наличии) **`remarks`**; корневые **`dns`**, **`routing`**, **`inbounds`** и прочее из элемента **не** подмешиваются в общий конфиг лаунчера.

**Как отличить Xray-массив от sing-box-массива (016, не реализовано)**

| Шаг | Эвристика |
|-----|-----------|
| Декодер | После trim строка начинается с **`[`**, **`json.Valid`**, успешный `json.Unmarshal` в массив — тело не отвергается как «не подписка» (`DecodeSubscriptionContent`). |
| Вход в парсер | **`IsXrayJSONArrayBody`**: то же — префикс `[`, валидный JSON, массив объектов. |
| Элемент массива | **`xrayElementHasProtocolOutbounds`**: в **`outbounds`** есть хотя бы один объект с полем **`protocol`** (строка) — признак **Xray-диалекта**. Элементы только с sing-box **`type`** без **`protocol`** не считаются Xray для этой ветки и **пропускаются** с `debuglog` (ожидается follow-up **016**). |
| Нода | Среди VLESS с **`settings.vnext`** выбирается основной outbound (`xrayBuildVLESSFromOutbound`); при **`dialerProxy`** hop разбирается как **`socks`** или **`vless`** (`xrayChainHopFromOutbound`; socks-звено — `xrayBuildJumpFromSocksOutbound`); иные `protocol` у hop — пропуск элемента (`WarnLog`). |

**`remarks` и теги sing-box**

- В **`ParsedNode.Label`** попадает полный текст **`remarks`** (если пусто — запасной вариант: тег основного Xray-outbound или `xray-{индекс}`).
- **Теги** генерируемых outbound в sing-box: если **`remarks`** непустой, из него строится **slug** (буквы/цифры в любой скрипте, **символы региональных индикаторов** для UTF-флагов, нормализация через `textnorm`, обрезка длины; прочие знаки и emoji кроме флагов в slug не входят). **Основной** outbound получает тег **`{slug}`**; при цепочке через SOCKS второй outbound (jump) — **`{slug}_jump_server`**, а у основного в JSON задаётся **`detour`** на этот тег. Если **`remarks`** пустой — **`xray-{индекс}`** и **`xray-{индекс}_jump_server`**. Далее, как у обычных подписок, применяются **`tag_prefix` / `tag_postfix` / `tag_mask`**, **`textnorm.NormalizeProxyDisplay`** и **`MakeTagUnique`** (в т.ч. для jump).
- В сгенерированном фрагменте `config.json` над outbound по-прежнему пишется **комментарий** `// …` из **`Label`** (полный `remarks`), т.к. у sing-box нет поля «remarks» в outbound.

**Цепочка `dialerProxy`**

При **`streamSettings.sockopt.dialerProxy`** (или **`dialer`**) → outbound с тем же **`tag`**: поддерживаются hop’ы **`protocol: socks`** и **`protocol: vless`**; в `config.json` сначала генерируется outbound hop’а, затем основной (VLESS и т.д.) с полем **`detour`** на тег hop’а. Если outbound по тегу не найден или **`protocol`** hop’а не **socks** / не **vless** — элемент массива **не** даёт ноды (`WarnLog`). Детали и расширение на другие типы: **`SPECS/036-F-C-XRAY_JUMP_ANY_PROTOCOL/SPEC.md`**. Массив конфигов **только в формате sing-box** (`type` в outbounds без Xray-`protocol`) в MVP **не** разбирается (follow-up **016**).

**Пример и код**

Структура как у публичных Xray-подписок (**`dns`**, **`inbounds`**, **`log`**, **`mux`**, **`tcpSettings`**, **`routing`**, **`freedom`/`blackhole`**), с вымышленными данными: **`docs/examples/xray_subscription_array_sample.json`**. Тот же сценарий в тестах: **`core/config/subscription/testdata/xray_provider_anon.json`** (`go:embed` в **`xray_json_array_test.go`**). Реализация: **`xray_json_array.go`**, **`xray_outbound_convert.go`**, **`decoder.go`** (`DecodeSubscriptionContent`), **`source_loader.go`** (`LoadNodesFromSource`, **`applyTagsToXrayNode`**), configurator: **`ui/configurator/tabs/source_tab.go`** (`refreshOneSourceFromUI`).

### Коды деградации на узле

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

Примеры первого вида: `masque_vhttp_invalid` (`vhttp` вне `{h3,h2}` принудительно
становится `h3`), `naive_padding_ignored`, `packet_encoding_unknown`,
`ws_early_data_converted` (хвост Xray `?ed=N` разложен на `max_early_data` +
`early_data_header_name` — путь в конфиге намеренно не тот, что в ссылке),
`amnezia_container_choice` (в профиле `vpn://` было несколько контейнеров, и
одиночный путь взял один).

Примеры второго: `ss_method_invalid`, `port_invalid`, `awg_headers_overlap` —
ядро отвергает такой endpoint на загрузке, то есть падает *весь* конфиг, поэтому
узел выбрасывается ещё на разборе.

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
| Пакет **`core/config/subscription`** | `ParseNode`, `buildOutbound` — `node_parser_core.go`; VLESS/Trojan transport+TLS — `node_parser_transport.go`; VMess — `node_parser_vmess.go` (`parseVMessDecoded`, `parseVMessJSON`, `parseVMessLegacyCleartext`); Hysteria2 — `node_parser_hysteria2.go`; WireGuard / SSH — `node_parser_wireguard.go`, `node_parser_ssh.go`; share URI — диспетчер `share_uri.go` + реализации `shareuri_*.go`; JSON-массив Xray — `xray_json_array.go`, `xray_outbound_convert.go`, `xray_protocols.go`, `xray_balancer.go`. |

## Share URI из outbound и WireGuard endpoint (обратно к ссылке)

Спецификация фичи (ПКМ на вкладке Servers, контекстное меню, детали реализации): **`SPECS/025-F-C-SERVERS_CONTEXT_MENU_SHARE_URI/`** (SPEC, PLAN, IMPLEMENTATION_REPORT).

Парсер переводит **строку подписки** (`ParseNode` → `buildOutbound` или для WireGuard — объект в `endpoints[]`) в JSON sing-box. Обратная операция — **сборка share URI из уже записанного outbound или WireGuard endpoint** в `config.json`, чтобы делиться ссылкой без хранения исходной строки подписки.

### Принцип и соответствие форматам

- **Вход кодировщика:** один элемент массива `outbounds` **или** один элемент `endpoints[]` с `type: wireguard` (тот же набор полей, что даёт `parseWireGuardURI` / `GenerateEndpointJSON`).
- **Выход:** одна строка URI в форматах, которые снова понимает этот проект: `vless://`, `vmess://` (base64 JSON), `trojan://`, `ss://` (SIP002), `socks5://`, `hysteria2://`, `tuic://`, `ssh://`, **`wireguard://`**.
- **Query / transport / TLS:** для VLESS и Trojan при кодировании используются те же соглашения, что и при разборе (`uriTransportFromQuery`, `vlessTLSFromNode`, `trojanTLSFromNode` в `node_parser_transport.go`). VMess при разборе не использует стандартный URI-query в основном формате (JSON в base64); legacy и поля JSON — в `node_parser_vmess.go`. Подробный справочник VLESS/Trojan: **`SUBSCRIPTION_PARAMS_REPORT.md`** (023); расширения 029 — спека **`029-Q-С-…/SPEC.md`** и разделы URI ниже.

### API в коде

| Функция | Пакет | Назначение |
|--------|--------|------------|
| `ShareURIFromOutbound(out map[string]interface{})` | `core/config/subscription` (`share_uri.go`) | Кодирование из JSON-объекта outbound; для `type: wireguard` делегирует в `ShareURIFromWireGuardEndpoint` |
| `ShareURIFromWireGuardEndpoint(ep map[string]interface{})` | `core/config/subscription` (`shareuri_wireguard.go`) | Кодирование `wireguard://` из одного endpoint (один peer в `peers[]`) |
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
| `tuic` | `tuic://` | `uuid:password`; `congestion_control`, `udp_relay_mode`, `zero_rtt_handshake`, `heartbeat`; `alpn`/`sni`/`insecure` из TLS |
| `ssh` | `ssh://` | **Нет** кодирования inline `private_key` в URI; путь к ключу и прочие поля — в query, как в документации SSH URI |
| `naive` | `naive+https://` / `naive+quic://` | HTTP/2 (`naive+https`) или QUIC (`naive+quic`); user/pass в userinfo; `extra-headers` в query с `\r\n`-разделёнными парами (см. раздел **NaïveProxy** ниже). Требует sing-box **≥ 1.13.0** с build tag `with_naive_outbound` (ядро форка `1.14.0-lx.4+`). |
| `anytls` | `anytls://` | Пароль в userinfo; TLS-блок обязателен; поля session-пула, если заданы |
| `masque` | `masque://` | Ключи base64(DER) в userinfo/`publickey=`, `ip`/`ipv6` склеиваются в `address=`; `vhttp` и `tls.server_name` → `vhttp=`/`sni=`. Требует ядро `1.14.0-lx.26+`. |
| `wireguard` | `wireguard://` | Обычно узел только в `endpoints[]`; формат и query — раздел **WireGuard** ниже. **Один URI ↔ один удалённый peer:** при нескольких элементах в `peers[]` кодирование не поддерживается (`ErrShareURINotSupported`). |

**Не кодируются в один share URI:** `selector`, `urltest`, `direct`, `block`, `dns`, `http`, произвольные служебные типы; WireGuard с **несколькими** `peers`; outbound с непустым **`detour`** (цепочка через jump из подписки Xray JSON).

### GUI

Вкладка **Servers** (список прокси Clash API): **ПКМ** по строке → `serversProxyContextMenu`: первая строка — **`api.ProxyInfo.ContextMenuTypeLine`** (нижний регистр поля **`type`** из API или `servers.menu_context_type_unknown`); затем **«Копировать ссылку»** (`servers.menu_copy_link`). Верхняя строка без `Disabled`, `Action: nil` (цвет текста как у обычного пункта меню). В буфер попадает строка через `config.ShareProxyURIForOutboundTag` и путь `FileService.ConfigPath`: сначала outbound по тегу, иначе WireGuard в `endpoints[]`. Правый клик по кнопкам Ping/Switch может не открыть меню (иерархия hit-test Fyne). Сообщения статуса: `servers.copy_link_resolving`, `servers.copy_link_done`, `servers.copy_link_not_supported`.

### Тесты

Round-trip и выборочные сценарии: `core/config/subscription/share_uri_encode_test.go`, интеграция с файлом конфига: `core/config/outbound_share_test.go`.

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
- **Дефолт `fp`:** если ни `fp`, ни `fingerprint` не заданы, для VLESS подставляется `random`. У Trojan такого дефолта нет — там uTLS-блок появляется только при распознанном `fp` (и ключ `fingerprint` не читается).
- `packetEncoding` — поле outbound `packet_encoding`. **Allow-list:** только `xudp`, `packetaddr`, `none` (включая пустое значение). Любое другое значение **отбрасывается с warning** в `debuglog` — sing-box не примёт неизвестные. См. [доку VLESS](https://sing-box.sagernet.org/configuration/outbound/vless/)
- `spx`, `quicSecurity`, `authority` — часто встречаются в ссылках Xray/панелей; в документированный клиентский JSON sing-box **не переносятся**, на разбор ссылки не влияют
- `mode` и `extra` — **влияют**, но только при `type=xhttp`: `mode` уезжает в транспорт как есть, `extra` — это URL-encoded JSON, из которого читаются те же поля xhttp (значения из `extra` перекрывают одноимённые flat-параметры). См. [параметры `xhttp`](#параметры-транспорта-xhttp) ниже

**⚠️ TLS отключается по порту.** Если `security` пуст, а порт входит в список типичных plaintext-портов (80, 8080, 8880, 2052, 2082, 2086, 2095), TLS-блок не эмитится вовсе — ссылка считается plain-HTTP-нодой (частый случай для Cloudflare-подписок). Чтобы TLS на таком порту всё-таки был, задайте `security=tls` явно.

**⚠️ Early data в пути WebSocket (`?ed=N`).** Xray прячет настройку раннего отправления в сам путь: `path=/ws?ed=2048`. sing-box требует её отдельными полями, поэтому парсер вырезает `ed` из пути и раскладывает в `max_early_data` + `early_data_header_name` (`Sec-WebSocket-Protocol`). Без этой конвертации нода проходит `check`, но в рантайме отвечает 404.

**⚠️ Vision на UDP:443 — авто-перезапись порта.** Если `flow=xtls-rprx-vision-udp443`, парсер **принудительно** ставит `server_port=443` (независимо от порта в URI) и `packet_encoding=xudp`. Семантика flow — XTLS Vision поверх UDP-трафика к стандартному 443. Если ваш сервер слушает Vision на нестандартном порту, используйте `flow=xtls-rprx-vision` (без `-udp443` суффикса).

**Пример:**
```
vless://uuid@server.com:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=example.com&fp=chrome&pbk=...&sid=...&type=tcp#🇳🇱 Netherlands
```

### Параметры транспорта `xhttp`

При `type=xhttp` (VLESS/Trojan) или `net=xhttp` (VMess) собирается транспорт `{"type":"xhttp", …}`. Значения берутся из двух источников: обычных query-параметров и **`extra`** — URL-encoded JSON; при совпадении ключей выигрывает `extra`. Имена читаются и в snake_case, и в camelCase (`session_key` = `sessionKey`).

| Параметр (snake_case / camelCase) | Тип в конфиге | Значение |
|---|---|---|
| `mode` | строка | `auto` \| `packet-up` \| `stream-up` \| `stream-one`. У форка `auto` = `packet-up`; у `stream-one` известен баг downlink-framing |
| `path` | строка | путь запроса; хвост от первого `?` отрезается (см. предупреждение ниже) |
| `host` | строка | заголовок Host; при пустом подставляется SNI из TLS |
| `x_padding_bytes` / `xPaddingBytes` | строка | диапазон `"min-max"`, дефолт `100-1000`; несётся в заголовке `Referer` |
| `no_grpc_header` / `noGRPCHeader` | bool | убрать gRPC-совместимый заголовок |
| `no_sse_header` / `noSSEHeader` | bool | убрать SSE-совместимый заголовок |
| `x_padding_obfs_mode` / `xPaddingObfsMode` | bool | включить обфускацию x-padding |
| `session_placement`, `session_key` / `sessionPlacement`, `sessionKey` | строка | размещение и имя ключа сессии |
| `seq_placement`, `seq_key` / `seqPlacement`, `seqKey` | строка | размещение и имя ключа последовательности |
| `uplink_data_placement`, `uplink_data_key` / `uplinkDataPlacement`, `uplinkDataKey` | строка | размещение и имя ключа uplink-данных |
| `uplink_chunk_size` / `uplinkChunkSize` | строка | размер чанка uplink'а |
| `uplink_http_method` / `uplinkHTTPMethod` | строка | HTTP-метод uplink'а |
| `x_padding_key`, `x_padding_header`, `x_padding_placement`, `x_padding_method` (camelCase: `xPaddingKey`, `xPaddingHeader`, `xPaddingPlacement`, `xPaddingMethod`) | строка | тонкая настройка x-padding-обфускации |
| `sc_max_each_post_bytes` / `scMaxEachPostBytes` | строка | ожидается ядром как `"min-max"`; голое число (в т.ч. `30.0` из `extra`) нормализуется в строку |
| `sc_min_posts_interval_ms` / `scMinPostsIntervalMs` | строка | то же правило |
| `sc_stream_up_server_secs` / `scStreamUpServerSecs` | строка | то же правило |
| `sc_max_buffered_posts` / `scMaxBufferedPosts` | **число** | ядро декодирует как int, а не как строку |

**Поля `xmux`** пишутся такими же плоскими параметрами — оборачивать их во вложенный объект не нужно; парсер сам соберёт из них `"xmux": {…}` внутри транспорта:

| Параметр (snake_case / camelCase) | Тип в конфиге | Значение |
|---|---|---|
| `max_connections` / `maxConnections` | строка | предел числа соединений (допускается диапазон `"min-max"`) |
| `max_concurrency` / `maxConcurrency` | строка | предел параллелизма (допускается диапазон) |
| `c_max_reuse_times` / `cMaxReuseTimes` | строка | сколько раз переиспользуется соединение |
| `h_max_request_times` / `hMaxRequestTimes` | строка | предел запросов на HTTP-соединение |
| `h_max_reusable_secs` / `hMaxReusableSecs` | строка | срок переиспользования HTTP-соединения |
| `h_keep_alive_period` / `hKeepAlivePeriod` | **число** | ядро декодирует как int, а не как строку |

**Булевы поля эмитятся только при истине.** `no_grpc_header`, `no_sse_header`, `x_padding_obfs_mode` при `false` не пишутся вовсе — у ядра дефолт равен отсутствующему полю. Истиной считаются `1`, `true`, `yes` (регистр не важен).

**Пример со всеми группами полей** (flat-параметры, рекомендуемая форма):

```
vless://UUID@example.com:443?encryption=none&security=tls&sni=a.com&type=xhttp&mode=packet-up&path=%2Fgtm.js&host=a.com&xPaddingBytes=100-1000&scMaxEachPostBytes=1000000&scMaxBufferedPosts=30&maxConnections=1&maxConcurrency=16-32&hKeepAlivePeriod=30#node-01
```

даёт транспорт:

```json
{
  "type": "xhttp",
  "mode": "packet-up",
  "path": "/gtm.js",
  "host": "a.com",
  "x_padding_bytes": "100-1000",
  "sc_max_each_post_bytes": "1000000",
  "sc_max_buffered_posts": 30,
  "xmux": { "max_connections": "1", "max_concurrency": "16-32", "h_keep_alive_period": 30 }
}
```

Те же поля можно передать через `extra` (URL-encoded JSON) — при совпадении ключей выигрывает `extra`:

```
&extra=%7B%22maxConnections%22%3A1%2C%22scMaxBufferedPosts%22%3A30%7D
```

Вложенная форма `extra={"xmux":{…}}` — та, что пишет сам Xray, — тоже читается: её члены разворачиваются в те же поля. Для своих ссылок она не нужна, плоская форма короче и эквивалентна.

**⚠️ Хвост запроса в `path` отрезается.** `path=/gtm.js?id-aabbccdd` даёт `"path": "/gtm.js"` — всё от первого `?` считается query, а не путём (SPEC 002 §4.1; реальные ноды шлют `path=/GaMeOpTiMiZeR?ed=2048`). Больше никакой нормализации нет: обратный слеш (`\gtm.js`) и остаточное percent-кодирование уезжают в конфиг как есть, `check` их пропустит, а сервер ответит 404.

Значения дальше не валидируются — их разбирает ядро. Реализация: `xhttpTransportFromQuery` / `xhttpBuildTransport` в `core/config/subscription/node_parser_transport.go`; спеки: `SPECS/071-F-N-XHTTP_TRANSPORT/SPEC.md`, `sing-box-lx` SPEC 002.

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
- `obfs` — тип обфускации: `salamander` или `gecko`. Неизвестный тип деградирует узел до «без обфускации» с предупреждением, а не роняет конфиг; обфускация без пароля тоже оставляет узел рабочим (`node_parser_hysteria2.go`).
- `obfs-password` - пароль для указанного типа обфускации
- `sni` - Server Name Indication для TLS соединений
- `insecure`, **`allowInsecure` / `allowinsecure`** — небезопасный TLS (как у VLESS: `1` / `true` / `yes`); также учитывается `skip-cert-verify`, но он признаётся только в точных формах `true` / `1`
- `fingerprint` / `fp` — uTLS fingerprint → `tls.utls` в sing-box
- `pinSHA256` — base64 SHA-256 публичного ключа сертификата → `tls.certificate_public_key_sha256` в sing-box

- `alpn` — список ALPN через запятую (для hysteria2 обычно `h3`)
- `upmbps` / `downmbps` — полоса пропускания в Мбит/с → `up_mbps` / `down_mbps` в sing-box. Нечисловые значения игнорируются

**⚠️ Про полосу пропускания.** `upmbps`/`downmbps` — это настройки *вашего* канала, а не сервера, поэтому в общей подписке им, строго говоря, не место (у каждого пользователя они свои). Но если они в ссылке есть — парсер их **читает и переносит** в конфиг, а не отбрасывает. Режимы клиента (HTTP, SOCKS5) в URI не поддерживаются.

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

**Требование:** sing-box должен быть собран с поддержкой NaïveProxy (build tag `with_naive_outbound`). Ядро форка `sing-box-lx` начиная с **`1.14.0-lx.4`** поддерживает naive на всех desktop-платформах: на Windows outbound в рантайме подгружает `libcronet.dll` (лаунчер извлекает её из релизного архива ядра в `bin/` при Download/Reinstall), на macOS и Linux cronet вкомпилен статически. Исключения навсегда: `windows-386-legacy-windows-7` и mips (cronet туда не собирается).

Если текущее ядро naive не поддерживает (нет тега, или purego-сборка без libcronet рядом с бинарём) — лаунчер **деградирует naive-ноды с предупреждением** (тост Update, warnings при rebuild), не позволяя одной ноде завалить весь `sing-box check` (probe: `core/core_capabilities.go::CoreSupportsNaive`).

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

Реализация: `core/config/subscription/node_parser_naive.go` (helpers), `node_parser_core.go` (dispatch), `shareuri_naive.go` (`shareURIFromNaive`). Спека: [**SPECS/044-F-C-NAIVE_PROXY_PARSER/SPEC.md**](../SPECS/044-F-C-NAIVE_PROXY_PARSER/SPEC.md).

### TUIC (`tuic://`)

TUIC v5 — прокси поверх QUIC: аутентификация парой `uuid` + `password`, UDP-релей и опциональный 0-RTT. TLS-блок обязателен (QUIC без TLS не бывает), поэтому парсер всегда пишет `tls.enabled: true`.

**Формат:** `tuic://<uuid>:<password>@<host>[:port]?<params>#<tag>`

**Структура:**
- `<uuid>` — userinfo-username → `uuid` в outbound. Без него нода собирается с warning `TUIC link missing uuid`.
- `<password>` — userinfo-password → `password`. Тоже обязателен, иначе warning `TUIC link missing password`.
- `port` — по умолчанию **443**.
- `#tag` — тег/комментарий (опционально).

**Параметры query:**

| Параметр | Значение |
|---|---|
| `congestion_control` | контроллер перегрузки QUIC: `cubic`, `new_reno`, `bbr` (регистр не важен). **⚠️ Любое другое значение отбрасывается с warning** — sing-box ругается на неизвестные контроллеры при загрузке конфига, поэтому парсер лучше промолчит, чем завалит весь `config.json` |
| `udp_relay_mode` | `native` или `quic`. Неизвестное значение отбрасывается с warning |
| `zero_rtt_handshake` | `1`/`true`/`yes` → `zero_rtt_handshake: true`. Принимается алиас **`reduce_rtt`** (его шлют v2rayN/Nekobox) — достаточно любого из двух |
| `heartbeat` | интервал keepalive. **Голое число = секунды**: `heartbeat=10` → `"10s"`; значение с единицей (`10s`, `1m`) проходит как есть |
| `sni` | → `tls.server_name`. **⚠️ Fallback:** если `sni` пустой, равен `🔒` или не похож на хост (нет ни точки, ни двоеточия) — подставляется адрес сервера |
| `insecure` / `allowInsecure` / `allow_insecure` / `skip-cert-verify` / `skipCertVerify` | `1`/`true`/`yes` → `tls.insecure: true`. Все пять написаний проверяются явно: поиск ключа регистронезависим, но не «разделителенезависим» |
| `fp` / `fingerprint` | uTLS fingerprint → `tls.utls`. Значение нормализуется по allowlist (`NormalizeUTLSFingerprint`); `fp` имеет приоритет, `fingerprint` — запасной |
| `alpn` | список через запятую → `tls.alpn` (пробелы обрезаются). Не задан — ядро само использует свой дефолт |

**Примеры:**
```
tuic://b8b1a1e3-0a2c-4d0f-9a3b-1c2d3e4f5a6b:[email protected]:443?congestion_control=bbr&udp_relay_mode=quic&alpn=h3&sni=cdn.example.com#🇩🇪 TUIC DE
tuic://b8b1a1e3-0a2c-4d0f-9a3b-1c2d3e4f5a6b:[email protected]:8443?reduce_rtt=1&heartbeat=10&allow_insecure=1&fp=chrome#TUIC self-signed
```

**Share URI (обратная сборка)** — round-trip «по смыслу», не побайтно: `insecure` всегда эмитится каноническим `insecure=1`, `zero_rtt_handshake=1` — новым именем (не `reduce_rtt`).

Реализация: `core/config/subscription/node_parser_tuic.go`, `node_parser_core.go` (dispatch), `shareuri_tuic.go`.

### AnyTLS (`anytls://`)

AnyTLS — прокси поверх обычного TLS с пулом переиспользуемых сессий и padding'ом, маскирующим размеры пакетов. Учётные данные — **один пароль**, как у Trojan. TLS обязателен, парсер всегда пишет `tls.enabled: true`.

**Формат:** `anytls://<password>@<host>[:port]?<params>#<tag>`

**Структура:**
- `<password>` — userinfo-**username** (не password!), как у Trojan → `password` в outbound. Без него warning `AnyTLS link missing password (userinfo)`.
- `port` — по умолчанию **443**.
- `#tag` — тег/комментарий (опционально).

**Параметры query:**

| Параметр | Значение |
|---|---|
| `sni` / `peer` | → `tls.server_name`; `sni` в приоритете, `peer` — запасной. **⚠️ Fallback:** если оба пустые, значение равно `🔒` или не похоже на хост (нет ни точки, ни двоеточия) — подставляется адрес сервера |
| `insecure` / `allowInsecure` / `allow_insecure` / `skip-cert-verify` / `skipCertVerify` | `1`/`true`/`yes` → `tls.insecure: true` (все пять написаний, как у TUIC) |
| `fp` / `fingerprint` | uTLS fingerprint → `tls.utls`, нормализуется по allowlist; `fp` в приоритете |
| `alpn` | список через запятую → `tls.alpn` (пробелы обрезаются) |
| `idle_session_check_interval` | как часто пул проверяет простаивающие сессии. **Голое число = секунды** (`30` → `"30s"`), та же конвенция, что у TUIC `heartbeat` |
| `idle_session_timeout` | через сколько простоя сессия закрывается; голое число тоже трактуется как секунды |
| `min_idle_session` | сколько сессий держать «тёплыми», целое ≥ 0. **⚠️ Не-число или отрицательное отбрасывается с warning** |

**Примеры:**
```
anytls://[email protected]:443?sni=cdn.example.com&alpn=h2,http/1.1&fp=chrome#🇳🇱 AnyTLS NL
anytls://[email protected]:8443?peer=example.com&insecure=1&idle_session_check_interval=30&idle_session_timeout=60&min_idle_session=2#AnyTLS pool
```

**Share URI (обратная сборка)** — round-trip «по смыслу»: SNI всегда выезжает как `sni` (не `peer`), небезопасный TLS — как `insecure=1`, `min_idle_session` пишется только при значении > 0.

Реализация: `core/config/subscription/node_parser_anytls.go`, `node_parser_core.go` (dispatch), `shareuri_anytls.go`. Спека: [**SPECS/091-F-C-ANYTLS_PROTOCOL/SPEC.md**](../SPECS/091-F-C-ANYTLS_PROTOCOL/SPEC.md).

### MASQUE (`masque://`)
**⚠️ Собственный формат:** как и SSH, `masque://` — URI-диалект singbox-launcher, а не стандарт. Он симметричен эмиссии лаунчера: то, что генерирует мастер WARP (MASQUE), парсер принимает обратно без потерь.

MASQUE (CONNECT-IP, RFC 9484) туннелирует **целые IP-пакеты** поверх HTTP/3 или HTTP/2 — в первую очередь это Cloudflare WARP. Требуется ядро **`1.14.0-lx.26+`** (схема `vhttp` + вложенный `tls`, core SPEC 062); узлы пишутся в `outbounds[]`.

**Формат:** `masque://<PRIVATE_KEY_DER>@<SERVER>:<PORT>?publickey=<PUB_DER>&address=<v4,v6>&...#tag`

**Структура:**
- `<PRIVATE_KEY_DER>` — клиентский EC-приватный ключ, **base64(DER)** (SEC1, `x509.ParseECPrivateKey`). Кладётся в userinfo; допускается и `?private_key=`/`?privatekey=`. Символ `/` внутри base64 экранируется парсером самостоятельно.
- `<SERVER>:<PORT>` — WARP-endpoint (обычно IP), порт по умолчанию `443`. Нечисловой порт здесь не ошибка — молча берётся 443 (у большинства других схем такой URI отбраковывается).
- `#tag` — тег/комментарий (опционально).

**Параметры query:**

| Параметр | Обяз. | Дефолт | Значение |
|---|---|---|---|
| `publickey` (`public_key`) | **да** | — | публичный ключ endpoint'а, **base64(DER)** PKIX (`x509.ParsePKIXPublicKey`, ECDSA). Именно им аутентифицируется endpoint — поэтому SNI свободен |
| `address` | **да** | — | локальные адреса **внутри** туннеля через запятую (`172.16.0.2/32,2606:4700:...::/128`). Голый адрес → `/32` (v4) / `/128` (v6). Нужен хотя бы один; без него ядро отвергает узел (`at least one of ip/ipv6 is required`) |
| `vhttp` | нет | `h3` | **версия HTTP**, несущая CONNECT-IP: `h3` (QUIC) или `h2` (HTTP/2, TCP:443). Не L4-список! Нераспознанное значение форсится в `h3` с warning |
| `profile` | нет | `cloudflare` | `cloudflare` (кварки WARP) или `standard` (строгий RFC 9484, свой сервер) |
| `sni` | нет | — | имя в ClientHello → `tls.server_name`. Принимается и как `server_name` |
| `insecure` | нет | — | `1`/`true` → `tls.insecure`. Принимается и как `skip_cert_verify`, `allowinsecure` |
| `mtu` | нет | `1280` | MTU userspace-стека. Парсер принимает любое положительное число; верхнюю границу (на `h2` — 16000) проверяет ядро |
| `idle_timeout` | нет | — | простой, после которого туннель усыпляется (следующий dial поднимает заново) |
| `keep_alive` (`keep_alive_period`) | нет | — | QUIC-keepalive, только для `h3` |

**⚠️ `vhttp`, а не `network`.** До ядра `lx.26` версия HTTP звалась `network` — то есть ровно наоборот к смыслу `network` у всех остальных протоколов (там это список tcp/udp). Ядро переименовало поле (core SPEC 062), старое принимает до `lx.30` с предупреждением. **Парсер лаунчера понимает обе формы** (`?network=h2` работает — чужие подписки его ещё шлют), но в `config.json` всегда пишет новую: `vhttp` + вложенный `tls`. При конфликте (`vhttp=h3&network=h2`) выигрывает новое имя.

**⚠️ SNI по умолчанию — не адрес endpoint'а.** Назвать MASQUE-endpoint в ClientHello — это ровно то, на что смотрит DPI. Мастер WARP подставляет нейтральный домен из своего пула, а ядро при пустом SNI берёт дефолт профиля. Endpoint аутентифицируется пиннингом `publickey`, поэтому SNI может быть любым.

**Примеры:**
```
masque://MHcCAQEEIA...@162.159.198.2:443?publickey=MFkwEwYHKoZI...&address=172.16.0.2/32,2606:4700:110:8142::/128&vhttp=h3&sni=www.microsoft.com#🎭 WARP
masque://MHcCAQEEIA...@162.159.198.2:443?publickey=MFkwEwYHKoZI...&address=172.16.0.2/32&vhttp=h2&mtu=1400#WARP h2
```

**Что получается в `config.json`:**
```jsonc
{
  "type": "masque",
  "tag": "🎭 WARP",
  "server": "162.159.198.2",
  "server_port": 443,
  "profile": "cloudflare",
  "vhttp": "h3",
  "tls": { "server_name": "www.microsoft.com" },
  "private_key": "<base64 DER EC>",
  "public_key":  "<base64 DER PKIX>",
  "ip":   "172.16.0.2/32",
  "ipv6": "2606:4700:110:8142::/128",
  "mtu":  1280
}
```

**Импорт готового конфига.** Outbound со **старой** схемой (плоские `network`/`sni`/`skip_cert_verify`) нормализуется при импорте: плоский `sni` рядом с `tls.server_name` смешивать нельзя — это два источника имени, и при расхождении ядро падает fail-fast'ом. `utls`/`reality` для masque срезаются (он идёт поверх QUIC, ядро их всё равно игнорирует).

**Ключи не добываются из URI** — их выдаёт регистрация в Cloudflare. Проще всего получить узел через мастер: **Config Wizard → WARP → MASQUE**, он регистрирует аккаунт и сам собирает `masque://`-ссылку.

Реализация: `core/config/subscription/node_parser_masque.go`, `shareuri_masque.go`, `core/warp/masque.go`. Спеки: [**SPECS/086-F-O-MASQUE_WARP_INTEGRATION/SPEC.md**](../SPECS/086-F-O-MASQUE_WARP_INTEGRATION/SPEC.md), схема полей — `sing-box-lx/docs-lx/lx-config.md` §4.

### WireGuard (`wireguard://`)
**⚠️ Особенность:** Узлы WireGuard записываются в секцию **endpoints** конфига (не в outbounds). Требуется **sing-box 1.11+**.

Стандартный URI формат: `wireguard://<PRIVATE_KEY>@<SERVER>:<PORT>?params#tag`

В userinfo указывается приватный ключ клиента. Спецсимволы в query — URL-encode: `/` → `%2F`, `,` → `%2C`. **Сырой `/` в base64-ключе** (часто встречается в нелоканонических ссылках) парсер теперь терпит — percent-энкодит сам перед разбором (см. сабтаску 073.1 в SPEC AmneziaWG).

**Параметры query string:**
- `publickey` — публичный ключ сервера (base64, обязательный)
- `address` — адрес клиента в VPN, CIDR (например `10.10.10.2/32`, обязательный). Голый IP без маски (`172.16.0.2`, как в экспортах AmneziaWG/`.conf`) парсер дополняет до `/32` (IPv4) / `/128` (IPv6).
- `allowedips` — разрешённые маршруты, CIDR через запятую (например `0.0.0.0/0,::/0`, обязательный). Голые IP так же дополняются префиксом.
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

**`reserved` (Cloudflare WARP).** Тройка байтов `reserved=b0,b1,b2` уезжает в `peers[0].reserved`. Значение обязано быть тремя числами 0–255; иначе поле молча пропускается (нода при этом остаётся рабочей — на многих путях WARP живёт и без него).

**Masquerade-сахар `ip` / `id` / `ib`.** WireSock-подобная короткая форма для первого decoy-пакета AWG: `ip` — протокол маскировки (`quic` \| `dns` \| `stun` \| `sip`), `id` — домен маскировки (LDH-валидация; обязателен для `quic`, для `dns`/`sip` генерируется псевдоимя, `stun` его игнорирует), `ib` — браузер (`chrome` \| `firefox` \| `curl`, осмысленно только с `ip=quic`). Ядро само разворачивает их в `i1`; поэтому сахар **взаимоисключающ** с явным `i1`.

**Детали разбора:** Приватный ключ из userinfo декодируется через PathUnescape. В `publickey` и `presharedkey` символ `+` (в base64) при разборе сохраняется.

**AmneziaWG 2.0 (опционально — ядро sing-box-lx с `with_awg`):**

Те же `wireguard://` ссылки могут нести параметры обфускации AmneziaWG — они promoted в корень WireGuard-endpoint рядом с `private_key`/`peers`:

- **Числовые** (uint32 → JSON-число): `jc` (число junk-пакетов до handshake), `jmin`/`jmax` (мин/макс размер junk), `s1`/`s2` (junk перед init/response handshake), `s3`/`s4` (junk перед cookie-reply/transport — **AWG 2.x**), `h1`–`h4` (magic-заголовки для 4 типов сообщений WireGuard).
- **Строковые** (case-sensitive tag-формат): `i1`–`i5` — CPS decoy-пакеты **AWG 2.0**, отправляются по порядку до handshake. Теги: `<b 0xHEX>` статичные байты, `<c>` счётчик, `<t>` timestamp, `<r N>` / `<rc N>` / `<rd N>` — random байты / символы / цифры.

Имена числовых полей читаются из query в любом регистре; `i1`–`i5` берутся как есть (кейс сохраняется). `H1`–`H4` могут быть заданы **диапазоном** `lo-hi` (header randomization из AWG 2.0 — так экспортирует Amnezia): диапазон пробрасывается в endpoint **как есть, строкой** `"h1": "N-M"` (перевёрнутый нормализуется, границы — uint32) — ядро **sing-box-lx ≥ 1.13.13-lx.6** само выбирает значение внутри диапазона на каждый handshake; одиночные значения, как раньше, эмитятся JSON-числом. Диапазоны допустимы только на `h1`–`h4` (сабтаска 073.2). Endpoint **без единого** AWG-поля = обычный WireGuard (byte-identical с апстримом). Клиент и сервер должны иметь **совпадающие** AWG-параметры — I-пакеты являются конфигурацией, не согласуются по сети. Маппинг 1:1 с `awg.conf` (awg-quick): `[Interface] Jc/Jmin/Jmax/S1–S4/H1–H4/I1–I5` → корень endpoint, `[Peer] …` → `peers[0]`.

**MTU AWG-эндпоинта клампится сверху до `1280`** (рекомендация AmneziaWG). `s3`/`s4` добавляют padding к **каждому** transport-пакету, поэтому при `mtu=1420` итоговый пакет вылезает за path-MTU и ОС отбивает его (`sendmsg: message too long`): handshake проходит, но данные молча встают. Политика парсера для WireGuard-эндпоинта **с** AWG-полями: `mtu` из URI выше 1280 → понижается до 1280; явно меньшее значение (напр. `mtu=1200`) уважается; при отсутствии `mtu` — дефолт 1280 (а не 1420). Обычный WireGuard (без AWG-полей) сохраняет апстрим-дефолт 1420. Потолок без запаса: `1500 − 28 (UDP/IP) − 32 (WG) − max(s3,s4)`; 1280 = IPv6-минимум, безопасен на PPPoE/мобайл/вложенных путях. **Сервер должен иметь симметричный MTU** — иначе крупные обратные пакеты упрутся в path-MTU так же.

**Пример (AWG2):**
```
wireguard://privkey-base64@server.example.com:51821?publickey=server-pubkey&address=10.0.0.2%2F32&allowedips=0.0.0.0%2F0%2C%3A%3A%2F0&keepalive=25&jc=10&jmin=50&jmax=100&s1=20&s2=20&s3=60&s4=60&h1=1234567890&h2=1234567891&h3=1234567892&h4=1234567893&i1=%3Cb%200x000100002112a442%3E%3Cr%2012%3E#AWG2
```
(`i1` здесь — URL-encoded `<b 0x000100002112a442><r 12>`.) Поддержка реализована в `applyAWGFields` / `ShareURIFromWireGuardEndpoint` (`core/config/subscription/node_parser_wireguard.go`, `shareuri_wireguard.go`); рантайм — на ядре с `with_awg`. См. `SPECS/073-F-N-AMNEZIAWG_PARAMS/SPEC.md` и `sing-box-lx/docs-lx/lx-config.md`.

### Amnezia (`vpn://`)

Ссылки **`vpn://…`**, которые экспортирует Amnezia VPN / AmneziaWG 2.0 (файл `.vpn` — это одна такая ссылка), принимаются напрямую: вставьте ссылку в Sources или Connections. Формат (эталон — `amnezia-vpn/config-decoder`): `vpn://` + base64url без padding, внутри qCompress (4 байта big-endian длины + zlib), под ним JSON всего профиля Amnezia.

Из профиля импортируется **только WireGuard/AmneziaWG-контейнер** (OpenVPN/Cloak/XRay-контейнеры пропускаются): сначала пробуется `defaultContainer`, затем остальные по порядку. Найденный `[Interface]/[Peer]`-конфиг конвертируется в канонический `wireguard://`-URI (см. таблицу параметров выше), поэтому применяются те же правила: нормализация голых IP до CIDR, promote AWG-полей `Jc`/`Jmin`/`Jmax`/`S1`–`S4`/`H1`–`H4`/`I1`–`I5` в корень endpoint и **кламп MTU AWG-эндпоинта до 1280** — `MTU = 1420` из амнезиевского конфига заведомо ломает передачу данных (`sendmsg: message too long`). Имя узла берётся из `description` профиля, затем `hostName`, затем имя контейнера.

Лимиты: ссылка до 512 КБ, распакованный профиль до 8 МБ (защита от zlib-бомб). Профиль без WG/AWG-контейнера даёт ошибку с перечислением контейнеров. Реализация: `core/config/subscription/node_parser_amnezia.go`; спека: `SPECS/075-F-C-AMNEZIA_VPN_IMPORT/SPEC.md`; референс-декодер для отладки: `scripts/decode_amnezia_vpn.py`.

### Голый `.conf`-текст (`[Interface]/[Peer]`)

Содержимое `.conf`-файла WireGuard/AmneziaWG можно вставить в поле Add вкладки Sources **как есть** — классификатор сам выделяет `[Interface]`-блоки из вставленного текста до построчного разбора и конвертирует каждый в канонический `wireguard://`-URI (хранится и шарится именно URI). Несколько блоков за одну вставку → несколько узлов; ссылки в том же тексте продолжают работать. Имя узла — хост из `Endpoint`. AWG-поля и кламп MTU — как у `vpn://` выше. Невалидный блок пропускается с предупреждением в лог, не срывая вставку. Реализация: `core/config/subscription/wgconf_text.go` + врезка в `classifyInputLines` (`ui/configurator/business/parser.go`); спека: `SPECS/076-F-C-WGCONF_PASTE_IMPORT/SPEC.md`.

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
