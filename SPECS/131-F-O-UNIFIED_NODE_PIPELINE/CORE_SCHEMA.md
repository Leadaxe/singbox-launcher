# CORE_SCHEMA — схема тел outbound/endpoint ядра sing-box-lx

Реестр полей для единого конвейера узла (SPEC 131). Источник — исходники ядра, не документация.

## Как читалось

| | |
|---|---|
| Репозиторий | `/Users/macbook/projects/sing-box-lx` |
| Тег | `v1.14.1-lx.4` (commit `900cdb24b7ff4b5eba98e5596786ba77658482ea`) |
| Дата снятия | 2026-09-17 |
| Команда чтения | `git show v1.14.1-lx.4:option/<file>.go` (ветки не переключались, рабочая копия не менялась) |
| База upstream | `9dddbefb2c98611652b19bc25d2fcb8d7dc03b8d` = `git merge-base v1.14.1-lx.4 upstream/testing`, он же `v1.14.1-2-g9dddbefb2` |
| Определение `lx_only` | `git diff 9dddbefb2 v1.14.1-lx.4 -- option/` — то, чего нет в upstream-базе форка |
| `min_core` | `git log --reverse -S'<symbol>' -- option/` → первый тег через `git tag --contains` |
| Дефолты / обязательность | `protocol/*/outbound.go`, `protocol/*/endpoint.go`, `transport/*`, `common/tls/*` — ветки `E.New`/`E.Cause` и присваивания при пустом значении |

**Важно про `lx_only`.** Сравнение велось с upstream-базой форка (1.14.1), а не с v1.13.11 из module cache. Поля вроде `anytls.client_metadata`, `ssh.cipher/mac/kex_algorithm`, `wireguard.udp_mapping`, `tailscale.ssh_server`, `hysteria2.bbr_profile/realm/hop_interval_max`, `tls.spoof`, `tls.engine`, `tls.handshake_timeout` в v1.13.11 отсутствуют, но **это upstream 1.14**, а не форк — они не помечены `lx_only`.

**Полный список того, что форк добавил в `option/`** (15 файлов в диффе, из них относящиеся к узлам): `chain_lx.go`, `masque.go`, `v2ray_xhttp.go`, `v2ray_xhttp_xmux_range.go`, `wireguard_awg.go`, плюс точечные правки в `tls.go` (`reality.key_share`), `vless.go` (`encryption`), `wireguard.go` (встроенные AWG-опции + `AWGRange` у `persistent_keepalive_interval`), `v2ray_transport.go` (вариант `xhttp`).

## Условные обозначения

- **listable** — `badoption.Listable[T]`: принимается и одиночное значение, и массив.
- **обязателен** — «да», если без него `New*` возвращает ошибку (падение на `run`; `sing-box check` часть таких ошибок **не** ловит — см. память «chain: check не ловит ошибки старта»).
- json-тег **без `omitempty`** отмечен отдельно: ядро эмитирует такой ключ всегда, и лаунчер тоже обязан его писать.
- `platform` — поле работает не везде.
- `build_tag` — модуль собирается по тегу; в сборке без него заданное поле обычно даёт ошибку старта.

---

## 1. Общие под-структуры

### 1.1 ServerOptions (встроено плоско во все outbound'ы)

| путь | json-ключ | Go-тип | listable | enum/дефолт | обязателен | lx_only/min_core | platform | заметка |
|---|---|---|---|---|---|---|---|---|
| `server` | server | string | — | — | да | — | — | **без omitempty** |
| `server_port` | server_port | uint16 | — | — | да | — | — | **без omitempty** |

### 1.2 DialerOptions (встроено плоско; «клиентские» — те, что реально пишет лаунчер)

| путь | json-ключ | Go-тип | listable | enum/дефолт | обязателен | lx_only/min_core | platform | заметка |
|---|---|---|---|---|---|---|---|---|
| `detour` | detour | string | — | — | нет | — | — | **клиентское**; `reference:outbound`; включает авто-`record_fragment` (SPEC 060) |
| `bind_interface` | bind_interface | string | — | — | нет | — | — | **клиентское** |
| `inet4_bind_address` | inet4_bind_address | badoption.Addr | — | — | нет | — | — | **клиентское** |
| `inet6_bind_address` | inet6_bind_address | badoption.Addr | — | — | нет | — | — | **клиентское** |
| `bind_address_no_port` | bind_address_no_port | bool | — | false | нет | — | — | |
| `protect_path` | protect_path | string | — | — | нет | — | android | |
| `routing_mark` | routing_mark | FwMark | — | — | нет | — | linux | |
| `reuse_addr` | reuse_addr | bool | — | false | нет | — | — | |
| `netns` | netns | string | — | — | нет | — | linux | `reference:network_namespace` |
| `connect_timeout` | connect_timeout | badoption.Duration | — | — | нет | — | — | **клиентское** |
| `tcp_fast_open` | tcp_fast_open | bool | — | false | нет | — | — | **клиентское**; конфликт с anytls |
| `tcp_multi_path` | tcp_multi_path | bool | — | false | нет | — | — | |
| `disable_tcp_keep_alive` | disable_tcp_keep_alive | bool | — | false | нет | — | — | |
| `tcp_keep_alive` | tcp_keep_alive | badoption.Duration | — | — | нет | — | — | |
| `tcp_keep_alive_interval` | tcp_keep_alive_interval | badoption.Duration | — | — | нет | — | — | |
| `udp_fragment` | udp_fragment | *bool | — | — | нет | — | — | **клиентское**; tri-state: отсутствие ≠ `false` |
| `domain_resolver` | domain_resolver | *DomainResolveOptions | — | — | нет | — | — | **клиентское**; строка (тег сервера) ИЛИ объект |
| `network_strategy` | network_strategy | *NetworkStrategy | — | enum не виден из `option/` | нет | — | — | значения в `C.StringToNetworkStrategy` |
| `network_type` | network_type | InterfaceType | **да** | enum в `C.StringToInterfaceType` | нет | — | — | |
| `fallback_network_type` | fallback_network_type | InterfaceType | **да** | там же | нет | — | — | |
| `fallback_delay` | fallback_delay | badoption.Duration | — | — | нет | — | — | |
| `domain_strategy` | domain_strategy | DomainStrategy | — | `""`/`as_is`/`prefer_ipv4`/`prefer_ipv6`/`ipv4_only`/`ipv6_only` | нет | — | — | **deprecated**, `schema:"omit"` |

`domain_resolver` как объект: `server` (обязателен, пустой = ошибка разбора), `timeout`, `strategy` (тот же enum), `disable_cache`, `disable_optimistic_cache`, `rewrite_ttl`, `client_subnet`. В короткой форме сериализуется обратно строкой, если задан только `server`.

### 1.3 NetworkList (`network`)

Строка или массив; допустимо только `tcp` / `udp`, иное = ошибка разбора. Пустое значение = **оба**.

### 1.4 OutboundTLSOptions (`tls`)

| путь | json-ключ | Go-тип | listable | enum/дефолт | обязателен | lx_only/min_core | platform | заметка |
|---|---|---|---|---|---|---|---|---|
| `tls.enabled` | enabled | bool | — | false | нет | — | — | `false` → весь блок игнорируется |
| `tls.engine` | engine | string | — | `""`/`go`/`apple`/`windows`, деф. `go` | нет | — | apple=darwin/ios, windows=win | неизвестное = ошибка |
| `tls.disable_sni` | disable_sni | bool | — | false | нет | — | — | включает `InsecureSkipVerify`; несовместимо с reality/spoof |
| `tls.server_name` | server_name | string | — | адрес сервера | условно | — | — | пусто + `insecure=false` = `missing server_name or insecure=true` |
| `tls.insecure` | insecure | bool | — | false | нет | — | — | |
| `tls.alpn` | alpn | string | **да** | enum не найден в коде | нет | — | — | примеры `http/1.1`, `h2`, `h3` |
| `tls.min_version` | min_version | string | — | `1.0`/`1.1`/`1.2`/`1.3` | нет | — | — | иное = ошибка `ParseTLSVersion` |
| `tls.max_version` | max_version | string | — | те же | нет | — | — | |
| `tls.cipher_suites` | cipher_suites | string | **да** | enum не фиксирован (`tls.CipherSuites()`) | нет | — | — | неизвестное имя = ошибка |
| `tls.curve_preferences` | curve_preferences | CurvePreference | **да** | `P256`/`P384`/`P521`/`X25519`/`X25519MLKEM768` | нет | — | — | разбор нечувствителен к регистру |
| `tls.certificate` | certificate | string | **да** | — | нет | — | — | PEM построчно |
| `tls.certificate_path` | certificate_path | string | — | — | нет | — | — | |
| `tls.certificate_public_key_sha256` | certificate_public_key_sha256 | []byte | **да** | — | нет | — | — | **конфликт** с certificate/certificate_path |
| `tls.client_certificate` | client_certificate | string | **да** | — | нет | — | — | только вместе с client_key |
| `tls.client_certificate_path` | client_certificate_path | string | — | — | нет | — | — | |
| `tls.client_key` | client_key | string | **да** | — | нет | — | — | |
| `tls.client_key_path` | client_key_path | string | — | — | нет | — | — | |
| `tls.fragment` | fragment | bool | — | false | нет | — | — | |
| `tls.fragment_fallback_delay` | fragment_fallback_delay | badoption.Duration | — | — | нет | — | — | |
| `tls.record_fragment` | record_fragment | bool | — | false | нет | — | — | SPEC 060: авто-`true` при непустом `detour`, если ни `fragment`, ни `record_fragment` не заданы |
| `tls.spoof` | spoof | string | — | — | нет | — | не все ОС (`PlatformSupported`) | требует SNI-домен, ≠ `server_name` |
| `tls.spoof_method` | spoof_method | string | — | `wrong-sequence`(деф)/`wrong-checksum`/`wrong-ack`/`wrong-md5`/`wrong-timestamp` | нет | — | — | без `spoof` = ошибка |
| `tls.kernel_tx` | kernel_tx | bool | — | false | нет | — | **linux** | вне Linux = ошибка старта |
| `tls.kernel_rx` | kernel_rx | bool | — | false | нет | — | **linux** | вне Linux = ошибка старта |
| `tls.handshake_timeout` | handshake_timeout | badoption.Duration | — | — | нет | — | — | |
| `tls.ech` | ech | *OutboundECHOptions | — | — | нет | — | — | см. 1.5 |
| `tls.utls` | utls | *OutboundUTLSOptions | — | — | нет | — | — | см. 1.6 |
| `tls.reality` | reality | *OutboundRealityOptions | — | — | нет | — | — | см. 1.7 |

### 1.5 OutboundECHOptions (`tls.ech`)

| путь | json-ключ | Go-тип | listable | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|---|
| `tls.ech.enabled` | enabled | bool | — | false | нет | |
| `tls.ech.config` | config | string | **да** | — | нет | PEM-блок `ECH CONFIGS`, иначе ошибка |
| `tls.ech.config_path` | config_path | string | — | — | нет | |
| `tls.ech.query_server_name` | query_server_name | string | — | — | нет | |
| `tls.ech.pq_signature_schemes_enabled` | … | bool | — | — | нет | **deprecated**, `schema:"omit"` |
| `tls.ech.dynamic_record_sizing_disabled` | … | bool | — | — | нет | **deprecated**, `schema:"omit"` |

### 1.6 OutboundUTLSOptions (`tls.utls`)

| путь | json-ключ | Go-тип | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|
| `tls.utls.enabled` | enabled | bool | false | нет | обязателен при reality |
| `tls.utls.fingerprint` | fingerprint | string | `""`(=chrome), `chrome`, `chrome_psk`, `chrome_psk_shuffle`, `chrome_padding_psk_shuffle`, `chrome_pq`, `chrome_pq_psk`, `firefox`, `edge`, `safari`, `360`, `qq`, `ios`, `android`, `random`, `randomized` | нет | список — `uTLSClientHelloID` в `common/tls/utls_client.go:380`; неизвестное = ошибка старта |

Гибридный ключевой шар (`X25519MLKEM768`) на lx.4 есть только у chrome/firefox/safari (см. память «REALITY firefox без MLKEM»); это влияет на `key_share=hybrid`.

### 1.7 OutboundRealityOptions (`tls.reality`)

| путь | json-ключ | Go-тип | enum/дефолт | обязателен | lx_only/min_core | заметка |
|---|---|---|---|---|---|---|
| `tls.reality.enabled` | enabled | bool | false | нет | — | требует `tls.utls.enabled=true` |
| `tls.reality.public_key` | public_key | string | — | **да** (при enabled) | — | base64 raw-url x25519; невалидный = `invalid public_key` |
| `tls.reality.short_id` | short_id | string | — | нет | — | hex; невалидный = `invalid short_id` |
| `tls.reality.key_share` | key_share | string | `""`(деф) / `hybrid` / `classical` | нет | **lx_only**, `min_core 1.14.1-lx.4` | SPEC 089; иное = ошибка старта; `hybrid` + отпечаток без MLKEM = ошибка на хендшейке |

### 1.8 OutboundMultiplexOptions (`multiplex`)

| путь | json-ключ | Go-тип | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|
| `multiplex.enabled` | enabled | bool | false | нет | |
| `multiplex.protocol` | protocol | string | `""`(=h2mux)/`h2mux`/`smux`/`yamux` | нет | |
| `multiplex.max_connections` | max_connections | int | 0 | нет | |
| `multiplex.min_streams` | min_streams | int | 0 | нет | |
| `multiplex.max_streams` | max_streams | int | 0 | нет | |
| `multiplex.padding` | padding | bool | false | нет | |
| `multiplex.brutal` | brutal | *BrutalOptions | — | нет | |
| `multiplex.brutal.enabled` | enabled | bool | false | нет | |
| `multiplex.brutal.up_mbps` | up_mbps | int | — | **да при enabled** | `<=0` = `brutal: invalid upload speed` |
| `multiplex.brutal.down_mbps` | down_mbps | int | — | **да при enabled** | `<=0` = `brutal: invalid download speed` |

### 1.9 UDPOverTCPOptions (`udp_over_tcp`)

Форма: `bool` **или** объект `{enabled, version}`. `version` ∈ {1, 2}, дефолт 2. Есть у shadowsocks, socks, naive.

### 1.10 QUICOptions (встроено плоско в hysteria / hysteria2 / tuic)

`idle_timeout`, `keep_alive_period`, `stream_receive_window` (MemoryBytes), `connection_receive_window` (MemoryBytes), `max_concurrent_streams` (int) — из `HTTP2Options`; плюс `initial_packet_size` (int), `disable_path_mtu_discovery` (bool).

---

## 2. Транспорты V2Ray (`transport`)

Дискриминатор — `transport.type`. Значения: `http`, `ws`, `quic`, `grpc`, `httpupgrade`, **`xhttp` (lx_only)**. Пустой/неизвестный тип = ошибка разбора JSON.

### 2.1 `type: http`

| путь | json-ключ | Go-тип | listable | enum/дефолт | заметка |
|---|---|---|---|---|---|
| `transport.host` | host | string | **да** | — | |
| `transport.path` | path | string | — | `/` | ведущий `/` дописывается |
| `transport.method` | method | string | — | `GET`; enum не найден в коде | |
| `transport.headers` | headers | badoption.HTTPHeader | — | — | map ключ → строка-или-массив |
| `transport.idle_timeout` | idle_timeout | Duration | — | — | |
| `transport.ping_timeout` | ping_timeout | Duration | — | — | |

### 2.2 `type: ws`

| путь | json-ключ | Go-тип | enum/дефолт | заметка |
|---|---|---|---|---|
| `transport.path` | path | string | `/` | |
| `transport.headers` | headers | HTTPHeader | — | |
| `transport.max_early_data` | max_early_data | uint32 | 0 | **парное** с `early_data_header_name` |
| `transport.early_data_header_name` | early_data_header_name | string | `""` | пусто + `max_early_data>0` → early data уезжает в путь. Xray пишет это как `?ed=N` в пути — конвертировать раздельно (память «WS Early Data ?ed=N») |

### 2.3 `type: quic`

`V2RayQUICOptions` — **пустая структура**, полей нет.

### 2.4 `type: grpc`

`service_name` (string, дефолт `""`, путь `/<service_name>/Tun`; enum нет), `idle_timeout`, `ping_timeout`, `permit_without_stream` (bool).

### 2.5 `type: httpupgrade`

`host` (string, не listable — в отличие от http!), `path` (дефолт `/`), `headers`.

### 2.6 `type: xhttp` — **lx_only**, `min_core 1.13.13-lx.1`, build tag `with_xhttp`

| путь | json-ключ | Go-тип | enum/дефолт | заметка |
|---|---|---|---|---|
| `transport.host` | host | string | пусто → SNI или адрес сервера | |
| `transport.path` | path | string | — | к нему дописывается session id (и seq в packet-up) |
| `transport.mode` | mode | string | `""`(=auto)/`auto`/`packet-up`/`stream-up`/`stream-one` | иное = ошибка старта |
| `transport.headers` | headers | HTTPHeader | — | |
| `transport.x_padding_bytes` | x_padding_bytes | string | `100-1000` | диапазон `min-max` или одно число |
| `transport.no_grpc_header` | no_grpc_header | bool | false | без него stream-one может висеть за CDN |
| `transport.xmux` | xmux | *V2RayXHTTPXmuxOptions | — | nil ≠ выключено — XMUX всегда включён |
| `transport.session_placement` | session_placement | string | `path`(деф)/`query`/`header`/`cookie` | иное = ошибка |
| `transport.session_key` | session_key | string | `X-Session` (header) / `x_session` (query,cookie) | |
| `transport.seq_placement` | seq_placement | string | `path`(деф)/`query`/`header`/`cookie` | |
| `transport.seq_key` | seq_key | string | `X-Seq` / `x_seq` | |
| `transport.session_table` | session_table | string | литеральный ASCII-алфавит **или** имя: `hex`,`HEX`,`number`,`alphabet`,`Alphabet`,`ALPHABET`,`base36`,`BASE36`,`Base62` | **парное** с `session_length`, иначе ошибка |
| `transport.session_length` | session_length | string | `min-max` | floor > 0 и `len(table)^min > 2^31`, иначе ошибка |
| `transport.uplink_data_placement` | uplink_data_placement | string | `auto`(деф)/`body`/`header`/`cookie` | `header`/`cookie` **только** при `mode=packet-up`, иначе ошибка |
| `transport.uplink_data_key` | uplink_data_key | string | `X-Data` / `x_data` | |
| `transport.uplink_chunk_size` | uplink_chunk_size | string | cookie `2048-3072` / header `3000-4000` / иначе `sc_max_each_post_bytes` | |
| `transport.uplink_http_method` | uplink_http_method | string | `POST` | `GET` вне packet-up = **warn + откат на POST**, не ошибка |
| `transport.x_padding_obfs_mode` | x_padding_obfs_mode | bool | false | false = legacy Referer-паддинг |
| `transport.x_padding_key` | x_padding_key | string | `x_padding` | |
| `transport.x_padding_header` | x_padding_header | string | `X-Padding` | |
| `transport.x_padding_placement` | x_padding_placement | string | `queryInHeader`(деф)/`cookie`/`header`/`query` | иное = ошибка |
| `transport.x_padding_method` | x_padding_method | string | `repeat-x`(деф)/`tokenish` | иное = ошибка |
| `transport.sc_max_each_post_bytes` | sc_max_each_post_bytes | string | `1000000-1000000` | |
| `transport.sc_min_posts_interval_ms` | sc_min_posts_interval_ms | string | `30-30` | |
| `transport.sc_max_concurrent_posts` | sc_max_concurrent_posts | int | — | принимается, **ИГНОРИРУЕТСЯ** |
| `transport.server_max_header_bytes` | server_max_header_bytes | int | — | server-only, **ИГНОРИРУЕТСЯ** |
| `transport.no_sse_header` | no_sse_header | bool | — | server-only, **ИГНОРИРУЕТСЯ** |
| `transport.sc_max_buffered_posts` | sc_max_buffered_posts | int64 | — | server-only, **ИГНОРИРУЕТСЯ** |
| `transport.sc_stream_up_server_secs` | sc_stream_up_server_secs | string | — | server-only, **ИГНОРИРУЕТСЯ** |

### 2.7 `transport.xmux` — **lx_only**, `min_core 1.13.13-lx.1`

Каждое range-поле — `XmuxRange`: `"min-max"`, одно число (`"4"` == `4-4`) или JSON-массив `[600,900]`.
**Правило «всё или ничего»:** секция отсутствует ИЛИ полностью пуста → дефолты `max_concurrency=1-1`, `h_max_request_times=600-900`, `h_max_reusable_secs=1800-3000`. Если задано **хоть одно** поле — все берутся как написаны, а незаданные остаются 0 (= без лимита).

| путь | json-ключ | Go-тип | дефолт | заметка |
|---|---|---|---|---|
| `transport.xmux.max_concurrency` | max_concurrency | XmuxRange | `1-1` | взаимоисключающе с `max_connections` |
| `transport.xmux.max_connections` | max_connections | XmuxRange | — | взаимоисключающе с `max_concurrency` |
| `transport.xmux.c_max_reuse_times` | c_max_reuse_times | XmuxRange | — | |
| `transport.xmux.h_max_request_times` | h_max_request_times | XmuxRange | `600-900` | считает **запросы**, не потоки |
| `transport.xmux.h_max_reusable_secs` | h_max_reusable_secs | XmuxRange | `1800-3000` | |
| `transport.xmux.h_keep_alive_period` | h_keep_alive_period | int64 | 0 | **простое число** секунд; отрицательное = выключить |

---

## 3. Outbound'ы

### 3.1 vless

| путь | json-ключ | Go-тип | listable | enum/дефолт | обязателен | lx_only/min_core | заметка |
|---|---|---|---|---|---|---|---|
| `uuid` | uuid | string | — | — | **да** | — | **без omitempty** |
| `flow` | flow | string | — | `""` / `xtls-rprx-vision` | нет | — | проверка в `sing-vmess/vless.NewClient`; vision не поддерживает UDP |
| `encryption` | encryption | string | — | пусто/`none` = выключено | нет | **lx_only**, `1.14.0-lx.18` | SPEC 032, `mlkem768x25519plus.<native\|xorpub\|random>.<0rtt\|1rtt>[.<padding>].<key>`; enum нет |
| `network` | network | NetworkList | **да** | `tcp`+`udp` | нет | — | |
| `packet_encoding` | packet_encoding | *string | — | `""` / `packetaddr` / `xudp` | нет | — | **`*string`**: отсутствие ключа = `xudp`; явная `""` = без инкапсуляции. Иное = ошибка |
| `tls` | tls | *OutboundTLSOptions | — | — | нет | — | §1.4 |
| `multiplex` | multiplex | *OutboundMultiplexOptions | — | — | нет | — | §1.8 |
| `transport` | transport | *V2RayTransportOptions | — | — | нет | — | §2 |
| + DialerOptions, ServerOptions | | | | | | | §1.1, §1.2 |

### 3.2 vmess

| путь | json-ключ | Go-тип | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|
| `uuid` | uuid | string | — | **да** | **без omitempty** |
| `security` | security | string | `auto`/`none`/`zero`/`aes-128-cfb`/`aes-128-gcm`/`chacha20-poly1305`, деф. `auto` | **да** | **без omitempty** — ключ эмитируется всегда |
| `alter_id` | alter_id | int | 0 | нет | |
| `global_padding` | global_padding | bool | false | нет | |
| `authenticated_length` | authenticated_length | bool | false | нет | |
| `network` | network | NetworkList | оба | нет | |
| `packet_encoding` | packet_encoding | string | `""`(деф)/`packetaddr`/`xudp` | нет | здесь **не** указатель — пусто = без инкапсуляции |
| `tls` / `multiplex` / `transport` | | | | нет | §1.4 / §1.8 / §2 |

### 3.3 trojan

| путь | json-ключ | Go-тип | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|
| `password` | password | string | — | **да** | **без omitempty** |
| `network` | network | NetworkList | оба | нет | |
| `tls` / `multiplex` / `transport` | | | | нет | |

### 3.4 shadowsocks (TLS-блока нет)

| путь | json-ключ | Go-тип | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|
| `method` | method | string | `none`, `aes-128-gcm`, `aes-192-gcm`, `aes-256-gcm`, `chacha20-ietf-poly1305`, `xchacha20-ietf-poly1305`, `2022-blake3-aes-128-gcm`, `2022-blake3-aes-256-gcm`, `2022-blake3-chacha20-poly1305`, `aes-128-ctr`, `aes-192-ctr`, `aes-256-ctr`, `aes-128-cfb`, `aes-192-cfb`, `aes-256-cfb`, `rc4-md5`, `chacha20-ietf`, `xchacha20` | **да** | **без omitempty**; неизвестный = ошибка `CreateMethod` |
| `password` | password | string | — | **да** | **без omitempty**; для `2022-*` обязан быть base64 нужной длины |
| `plugin` | plugin | string | enum не найден в коде | нет | |
| `plugin_opts` | plugin_opts | string | — | нет | |
| `network` | network | NetworkList | оба | нет | |
| `udp_over_tcp` | udp_over_tcp | *UDPOverTCPOptions | §1.9 | нет | |
| `multiplex` | multiplex | *OutboundMultiplexOptions | §1.8 | нет | |

### 3.5 hysteria (build tag `with_quic`)

| путь | json-ключ | Go-тип | listable | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|---|
| `server_ports` | server_ports | string | **да** | — | нет | диапазоны `a:b` |
| `hop_interval` | hop_interval | Duration | — | — | нет | |
| `up` / `down` | up / down | NetworkBytesCompat | — | — | нет | число или `"100 Mbps"` |
| `up_mbps` / `down_mbps` | … | int | — | — | нет | |
| `obfs` | obfs | string | — | — | нет | строка-пароль (в отличие от hysteria2) |
| `auth` | auth | []byte | — | — | нет | base64 в JSON |
| `auth_str` | auth_str | string | — | — | нет | |
| `network` | network | NetworkList | **да** | оба | нет | |
| `tls` | tls | *OutboundTLSOptions | — | — | **да** (`enabled=true`) | иначе `C.ErrTLSRequired` |
| `recv_window_conn`, `recv_window`, `disable_mtu_discovery` | | | | | нет | **deprecated**, `schema:"omit"` |
| QUICOptions плоско | | | | | нет | §1.10 |

### 3.6 hysteria2 (build tag `with_quic`)

| путь | json-ключ | Go-тип | listable | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|---|
| `server_ports` | server_ports | string | **да** | — | нет | конфликт с `realm` |
| `hop_interval` / `hop_interval_max` | … | Duration | — | — | нет | |
| `up_mbps` / `down_mbps` | … | int | — | — | нет | |
| `obfs` | obfs | *Hysteria2Obfs | — | объект-юнион | нет | см. ниже |
| `password` | password | string | — | — | нет | |
| `network` | network | NetworkList | **да** | оба | нет | |
| `tls` | tls | *OutboundTLSOptions | — | — | **да** | `C.ErrTLSRequired` |
| `bbr_profile` | bbr_profile | string | — | `""`/`standard`(деф)/`conservative`/`aggressive` | нет | |
| `brutal_debug` | brutal_debug | bool | — | false | нет | |
| `disable_chrome_parrot` | disable_chrome_parrot | bool | — | false | нет | |
| `realm` | realm | *Hysteria2Realm | — | — | нет | **конфликтует** с server/server_port/server_ports |
| QUICOptions плоско | | | | | нет | §1.10 |

`obfs` — дискриминированный юнион по `type`:

| путь | json-ключ | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|
| `obfs.type` | type | `salamander` / `gecko` | **да** | иное = ошибка разбора JSON |
| `obfs.password` | password | — | **да** | пустой = `missing obfs password` |
| `obfs.min_packet_size` | min_packet_size | int | нет | только `type=gecko` |
| `obfs.max_packet_size` | max_packet_size | int | нет | только `type=gecko` |

### 3.7 tuic (build tag `with_quic`)

| путь | json-ключ | Go-тип | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|
| `uuid` | uuid | string | — | **да** | невалидный = `invalid uuid` |
| `password` | password | string | — | нет | |
| `congestion_control` | congestion_control | string | `""`(=cubic)/`cubic`/`new_reno`/`bbr` | нет | |
| `udp_relay_mode` | udp_relay_mode | string | `""`(=native)/`native`/`quic` | нет | **конфликт** с `udp_over_stream` |
| `udp_over_stream` | udp_over_stream | bool | false | нет | конфликт с `udp_relay_mode` |
| `zero_rtt_handshake` | zero_rtt_handshake | bool | false | нет | |
| `heartbeat` | heartbeat | Duration | 10s | нет | |
| `network` | network | NetworkList | оба | нет | |
| `tls` | tls | *OutboundTLSOptions | — | **да** | `C.ErrTLSRequired` |
| QUICOptions плоско | | | | нет | §1.10 |

### 3.8 anytls

| путь | json-ключ | Go-тип | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|
| `password` | password | string | — | нет | |
| `idle_session_check_interval` | … | Duration | 30s | нет | клампится в `sing-anytls`: `<= 5s` → 30s |
| `idle_session_timeout` | … | Duration | 30s | нет | тот же кламп `<= 5s` → 30s |
| `min_idle_session` | min_idle_session | int | 0 | нет | |
| `client_metadata` | client_metadata | string | — | нет | |
| `tls` | tls | *OutboundTLSOptions | — | **да** | `C.ErrTLSRequired` |

**Конфликт:** `tcp_fast_open=true` → ошибка `tcp_fast_open is not supported with anytls outbound`.

### 3.9 naive (build tag `with_naive_outbound`, purego cronet)

| путь | json-ключ | Go-тип | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|
| `username` | username | string | — | нет | |
| `password` | password | string | — | нет | |
| `insecure_concurrency` | insecure_concurrency | int | 0 | нет | |
| `extra_headers` | extra_headers | HTTPHeader | — | нет | берётся **только первое** значение каждого ключа |
| `stream_receive_window` | stream_receive_window | MemoryBytes | — | нет | |
| `udp_over_tcp` | udp_over_tcp | *UDPOverTCPOptions | §1.9 | нет | **без него outbound только TCP** |
| `quic` | quic | bool | false | нет | |
| `quic_congestion_control` | quic_congestion_control | string | `""`/`bbr`/`bbr2`/`cubic`/`reno` | нет | иное = ошибка старта |
| `quic_session_receive_window` | … | MemoryBytes | — | нет | |
| `tls` | tls | *OutboundTLSOptions | — | **да** | подмножество — см. §5 |

Требует `libcronet.*` рядом с бинарём (память «naive требует libcronet»).

### 3.10 socks (TLS-блока нет)

| путь | json-ключ | Go-тип | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|
| `version` | version | string | `""`(=5)/`4`/`4a`/`5` | нет | |
| `username` / `password` | … | string | — | нет | |
| `network` | network | NetworkList | оба | нет | |
| `udp_over_tcp` | udp_over_tcp | *UDPOverTCPOptions | §1.9 | нет | |

### 3.11 http (только TCP)

`username`, `password`, `path` (string), `headers` (HTTPHeader), `tls` (§1.4). Ничего обязательного сверх server/server_port.

### 3.12 ssh (TLS-блока нет, только TCP)

| путь | json-ключ | Go-тип | listable | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|---|
| `user` | user | string | — | `root` | нет | |
| `password` | password | string | — | — | нет | |
| `private_key` | private_key | string | **да** | — | нет | |
| `private_key_path` | private_key_path | string | — | — | нет | |
| `private_key_passphrase` | private_key_passphrase | string | — | — | нет | |
| `host_key` | host_key | string | **да** | — | нет | authorized_keys-строки; несовпадение = `host key mismatch` |
| `host_key_algorithms` | host_key_algorithms | string | **да** | enum не найден в коде | нет | |
| `client_version` | client_version | string | — | случайный из списка ядра | нет | |
| `cipher` | cipher | string | **да** | enum не найден в коде | нет | |
| `mac` | mac | string | **да** | enum не найден в коде | нет | |
| `kex_algorithm` | kex_algorithm | string | **да** | enum не найден в коде | нет | |

`server_port` по умолчанию 22 (подставляется реализацией).

### 3.13 masque — **lx_only**, `min_core 1.14.0-lx.1`, build tag `with_quic`

| путь | json-ключ | Go-тип | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|
| `profile` | profile | string | `""`(=cloudflare)/`cloudflare`/`standard` | нет | иное = ошибка |
| `vhttp` | vhttp | string | `auto`(деф)/`h3`/`h2` | нет | SPEC 062/074; для `profile=standard` h2 **не реализован** |
| `network` | network | string | — | нет | **deprecated**: старое имя `vhttp` (h3/h2), НЕ tcp/udp. Удаляется в `v1.14.0-lx.30` |
| `private_key` | private_key | string | base64 DER EC | **да при profile=cloudflare** | |
| `public_key` | public_key | string | base64 DER PKIX | **да при profile=cloudflare** | |
| `ip` | ip | string | — | **минимум один из ip/ipv6** | без маски = /32 |
| `ipv6` | ipv6 | string | — | — | без маски = /128 |
| `uri` | uri | string | `https://cloudflareaccess.com` (cloudflare) | **да при profile=standard** | |
| `mtu` | mtu | uint32 | **1280** | нет | для `vhttp=h2` максимум 16000 |
| `idle_timeout` | idle_timeout | Duration | 0 = выключено | нет | только положительное включает idle-suspend |
| `keep_alive_period` | keep_alive_period | Duration | 30s | нет | отрицательное = выключить |
| `network_list` | network_list | NetworkList | оба | нет | **именно это** — tcp/udp allow-list |
| `tls` | tls | *OutboundTLSOptions | `server_name` по умолчанию `www.cloudflare.com` (cloudflare) | нет | §1.4 |
| `sni` | sni | string | — | нет | **deprecated** → `tls.server_name` |
| `skip_cert_verify` | skip_cert_verify | bool | — | нет | **deprecated** → `tls.insecure`; переносится только явное `true` |
| `fragment` | fragment | bool | — | нет | **deprecated** → `tls.fragment` |
| `fragment_fallback_delay` | fragment_fallback_delay | Duration | — | нет | **deprecated** → `tls.fragment_fallback_delay` |
| `record_fragment` | record_fragment | bool | — | нет | **deprecated** → `tls.record_fragment` |

Для `profile=cloudflare` сертификат сервера **пинится по публичному ключу**, обычная валидация цепочки отключена.

### 3.14 chain — **lx_only**, `min_core 1.14.0-lx.27`, build tag `with_lx_chain`

Ни ServerOptions, ни TLS у него нет.

| путь | json-ключ | Go-тип | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|
| `outbounds` | outbounds | []string | — | **да** | **без omitempty**; `[0]` — первый хоп от клиента; `reference:outbound` |
| `idle_timeout` | idle_timeout | Duration | **5m** | нет | 0 = жить до остановки |
| `strip_evasion` | strip_evasion | *bool | **nil = true** | нет | tri-state |
| `strip` | strip | map[string]bool | — | нет | патч к каталогу; **неизвестный ключ = ошибка старта** |
| `rewrite` | rewrite | map[string]any | — | нет | JSON merge-patch (RFC 7396) поверх опций узла; только позиции ≥ 1 |
| `interrupt_exist_connections` | interrupt_exist_connections | bool | false | нет | SPEC 075 |

---

## 4. Endpoint'ы

### 4.1 wireguard (build tag `with_wireguard`)

| путь | json-ключ | Go-тип | listable | enum/дефолт | обязателен | lx_only/min_core | заметка |
|---|---|---|---|---|---|---|---|
| `system` | system | bool | — | false | нет | — | системный интерфейс вместо userspace-стека |
| `name` | name | string | — | — | нет | — | имя интерфейса при `system=true` |
| `mtu` | mtu | uint32 | — | **1408**; при `s4>0` → **1280** | нет | — | предупреждение, если `mtu > 1492-60-s4` (память «AWG MTU too high») |
| `address` | address | netip.Prefix | **да** | — | **да** | — | **без omitempty** |
| `private_key` | private_key | string | — | — | **да** | — | **без omitempty**; пустой = `missing private key` |
| `listen_port` | listen_port | uint16 | — | — | нет | — | **конфликт с `detour`** |
| `peers` | peers | []WireGuardPeer | — | — | **да** | — | см. ниже |
| `udp_timeout` | udp_timeout | Duration | — | — | нет | — | |
| `udp_mapping` | udp_mapping | UDPNATBehavior | — | enum не извлечён | нет | — | |
| `udp_filtering` | udp_filtering | UDPNATBehavior | — | enum не извлечён | нет | — | |
| `udp_nat_max` | udp_nat_max | uint32 | — | — | нет | — | |
| `workers` | workers | int | — | — | нет | — | |
| AmneziaWG-поля | — | AmneziaWGOptions | — | — | нет | **lx_only** | **встроены плоско в корень endpoint'а**, без обёртки — см. §4.2 |
| + DialerOptions | | | | | | — | §1.2 |

`peers[]`:

| путь | json-ключ | Go-тип | listable | обязателен | заметка |
|---|---|---|---|---|---|
| `peers[].address` | address | string | — | **да** | |
| `peers[].port` | port | uint16 | — | **да** | |
| `peers[].public_key` | public_key | string | — | **да** | невалидный = ошибка декодирования |
| `peers[].pre_shared_key` | pre_shared_key | string | — | нет | |
| `peers[].allowed_ips` | allowed_ips | netip.Prefix | **да** | **да** | пустой = `missing allowed ips for peer N` |
| `peers[].persistent_keepalive_interval` | … | **AWGRange** | — | нет | **lx**: число (как в upstream) ИЛИ `"min-max"` секунд; форма-диапазон требует `with_awg` |
| `peers[].reserved` | reserved | []uint8 | — | нет | ровно **3** байта, иначе ошибка |

### 4.2 AmneziaWG-поля (плоско внутри wireguard) — **lx_only**, build tag `with_awg`

Без `with_awg` **любое** заданное поле = ошибка старта `AmneziaWG (awg) support is not included in this build`.
`min_core`: AWG 2.0 — `1.13.13-lx.1`; AWG 3.x — `1.14.0-lx.32`.

| путь | json-ключ | Go-тип | enum/дефолт | min_core | заметка |
|---|---|---|---|---|---|
| `jc` | jc | uint32 | — | 1.13.13-lx.1 | |
| `jmin` | jmin | uint32 | — | 1.13.13-lx.1 | `jmin <= jmax`, иначе ошибка |
| `jmax` | jmax | uint32 | — | 1.13.13-lx.1 | |
| `s1` | s1 | uint32 | — | 1.13.13-lx.1 | |
| `s2` | s2 | uint32 | — | 1.13.13-lx.1 | |
| `s3` | s3 | uint32 | — | 1.13.13-lx.1 | padding cookie-reply; **не** влияет на MTU |
| `s4` | s4 | uint32 | — | 1.13.13-lx.1 | junk перед каждым transport-пакетом — **влияет на MTU** |
| `h1`..`h4` | h1..h4 | MagicHeader (=AWGRange) | число или `min-max` | 1.13.13-lx.1 | |
| `i1`..`i5` | i1..i5 | string | — | 1.13.13-lx.1 | `i1` конфликтует с masquerade (`id`/`ip`/`ib`), `i2` — с `ip=sip` |
| `id` | id | string | — | 1.13.13-lx.1 | masquerade-домен; **обязателен** при `ip=quic\|dns\|sip`, опционален при `stun`; ≤253 байт, валидные метки |
| `ip` | ip | string | `quic`/`dns`/`stun`/`sip` | 1.13.13-lx.1 | **обязателен**, если задан `id` или `ib`; иное = ошибка |
| `ib` | ib | string | `chrome`/`firefox`/`curl` | 1.13.13-lx.1 | осмыслен **только** при `ip=quic`, иначе ошибка |
| `header_protection_key` | … | string | base64 фикс. длины | 1.14.0-lx.32 | всё-нули = ошибка; требует `s1`-`s4` не ниже порога |
| `content_padding_addition` | … | AWGRange | — | 1.14.0-lx.32 | |
| `rekey_after_time` | … | AWGRange | деф. WG 120 | 1.14.0-lx.32 | секунды |
| `rekey_timeout` | … | AWGRange | деф. WG 5 | 1.14.0-lx.32 | |
| `reject_after_time` | … | AWGRange | деф. WG 180 | 1.14.0-lx.32 | |
| `keepalive_timeout` | … | AWGRange | деф. WG 10 | 1.14.0-lx.32 | |
| `max_handshake_attempts` | … | AWGRange | деф. WG 18 | 1.14.0-lx.32 | попытки, не секунды |
| `random_trailers` | … | bool | false | 1.14.0-lx.32 | AWG 3.1 |
| `disable_cookies` | … | bool | false | 1.14.0-lx.32 | AWG 3.1 |

### 4.3 tailscale (build tag `with_tailscale`, TLS-блока нет)

| путь | json-ключ | Go-тип | listable | enum/дефолт | обязателен | заметка |
|---|---|---|---|---|---|---|
| `state_directory` | state_directory | string | — | `tailscale` | нет | расширяются env-переменные, путь абсолютизируется |
| `auth_key` | auth_key | string | — | — | нет | без него — интерактивный логин по URL |
| `control_url` | control_url | string | — | Tailscale-дефолт | нет | |
| `ephemeral` | ephemeral | bool | — | false | нет | |
| `hostname` | hostname | string | — | `os.Hostname()`, иначе `sing-box` | нет | |
| `accept_routes` | accept_routes | bool | — | false | нет | |
| `exit_node` | exit_node | string | — | — | нет | **конфликт** с `advertise_exit_node` |
| `exit_node_allow_lan_access` | … | bool | — | false | нет | |
| `advertise_routes` | advertise_routes | []netip.Prefix | — | — | нет | `0.0.0.0/0` = ошибка, нужен `advertise_exit_node` |
| `advertise_exit_node` | … | bool | — | false | нет | конфликт с `exit_node` |
| `advertise_tags` | advertise_tags | string | **да** | — | нет | |
| `listen_port` | listen_port | uint16 | — | — | нет | |
| `relay_server_port` | relay_server_port | *uint16 | — | — | нет | |
| `relay_server_static_endpoints` | … | []netip.AddrPort | — | — | нет | |
| `system_interface` | system_interface | bool | — | false | нет | |
| `system_interface_name` | … | string | — | — | нет | |
| `system_interface_mtu` | … | uint32 | — | — | нет | |
| `udp_timeout` | udp_timeout | UDPTimeoutCompat | — | `C.UDPTimeout` | нет | |
| `ssh_server` | ssh_server | *TailscaleSSHServerOptions | — | `{enabled,disable_pty,disable_sftp,disable_forwarding}` | нет | `enabled=true` проходит `CheckSecurityFeature` |
| `taildrop_directory` | taildrop_directory | string | — | `Taildrop` | нет | |
| + DialerOptions | | | | | | §1.2 |

---

## 5. naive: подмножество TLS

`protocol/naive/outbound.go` (строки 44–83) **отвергает ошибкой старта** всё, чего cronet не умеет, а не игнорирует. То есть неподходящее поле роняет весь конфиг, а не одну ноду.

| группа | ключи | что происходит |
|---|---|---|
| **используется** | `tls.enabled`, `tls.server_name` (пусто → адрес сервера), `tls.certificate`, `tls.certificate_path`, `tls.ech.enabled`, `tls.ech.config`, `tls.ech.config_path`, `tls.ech.query_server_name` | передаётся в `cronet.NaiveClientOptions` |
| **ошибка старта** | `disable_sni`, `insecure`, `alpn`, `min_version`, `max_version`, `cipher_suites`, `curve_preferences`, `client_certificate`(+`_path`), `client_key`(+`_path`), `fragment`, `record_fragment`, `kernel_tx`, `kernel_rx`, `utls.enabled`, `reality.enabled` | `E.New("<ключ> is not supported on naive outbound")` |
| **молча игнорируется** | `engine`, `certificate_public_key_sha256`, `spoof`, `spoof_method`, `fragment_fallback_delay`, `handshake_timeout` | в структуру cronet не попадают, проверки на них нет |
| **обязательно** | `tls.enabled=true` | иначе `C.ErrTLSRequired` |

Практический вывод для конвейера: для naive санитайзер обязан **вырезать** вторую группу (и `utls`/`reality` целиком), иначе одна нода валит запуск ядра.

---

## 6. Конфликты ключей (fatal)

| ключи | эффект | где в коде |
|---|---|---|
| `tls.certificate_public_key_sha256` + `tls.certificate`/`certificate_path` | `certificate_public_key_sha256 is conflict with certificate or certificate_path` | `std_client.go:139`, `utls_client.go:223` |
| `tls.reality.enabled` + `tls.ech.enabled` | `Reality is conflict with ECH` | `utls_client.go:336` |
| `tls.reality.enabled` + `tls.disable_sni` | `disable_sni is unsupported in reality` | `utls_client.go:218` |
| `tls.reality.enabled` + `tls.spoof` | `spoof is unsupported in reality` | `reality_client.go:64` |
| `tls.reality.enabled` **без** `tls.utls.enabled` | `uTLS is required by reality client` | `reality_client.go:61` |
| `tls.reality.key_share=hybrid` + отпечаток без MLKEM | ошибка в начале хендшейка | `reality_client.go:226` |
| `tls.spoof` + IP/пустой `server_name`/`disable_sni`, либо `spoof == server_name` | «spoof requires TLS ClientHello with SNI» / «must differ from server_name» | `client.go:33,36` |
| `tls.spoof_method` без `tls.spoof` | `spoof_method requires spoof` | `tlsspoof/spoof.go:29` |
| пустые `tls.server_name` + `insecure=false` + пустой адрес | `missing server_name or insecure=true` | `client.go:22` |
| `tls.client_certificate` без `tls.client_key` (и наоборот) | `client certificate and client key must be provided together` | `std_client.go:222` |
| `tls.kernel_tx`/`kernel_rx` вне Linux | `kTLS is only supported on Linux` | `std_client.go:257`, `utls_client.go:345` |
| `tuic.udp_over_stream` + `tuic.udp_relay_mode` | `udp_over_stream is conflict with udp_relay_mode` | `protocol/tuic/outbound.go:57` |
| anytls + `tcp_fast_open` | `tcp_fast_open is not supported with anytls outbound` | `protocol/anytls/outbound.go:60` |
| `wireguard.listen_port` + `detour` | `` `listen_port` is conflict with `detour` `` | `protocol/wireguard/endpoint.go:110` |
| `tailscale.advertise_exit_node` + `tailscale.exit_node` | `cannot advertise an exit node and use an exit node at the same time` | `protocol/tailscale/endpoint.go:168` |
| `hysteria2.realm` + `server`/`server_port`/`server_ports` | `realm conflicts with server, server_port, and server_ports` | `protocol/hysteria2/outbound.go:172` |
| `awg.i1` + masquerade (`id`/`ip`/`ib`) | `masquerade conflicts with an explicit i1` | `transport/wireguard/masque_awg.go:73` |
| `awg.i2` + `awg.ip=sip` | `sip masquerade fills i2` | `transport/wireguard/device_awg.go:108` |
| `awg.id`/`awg.ib` без `awg.ip` | `ip (masquerade protocol) is required when id/ib is set` | `masque_awg.go:78` |
| `xhttp.xmux.max_concurrency` + `max_connections` | взаимоисключающие (по документации поля) | `option/v2ray_xhttp.go` |
| `xhttp.uplink_data_placement=header\|cookie` при `mode != packet-up` | ошибка старта | `transport/v2rayxhttp/meta.go:117` |
| `xhttp.session_table` без `session_length` (и наоборот) | `session_table and session_length must be set together` | `meta.go:220` |
| `masque.profile=cloudflare` без `private_key`+`public_key` | `private_key and public_key are required for the cloudflare profile` | `protocol/masque/outbound.go:198` |
| `masque.ip` и `masque.ipv6` оба пустые | ошибка `parsePrefixes` | `protocol/masque/outbound.go:189` |
| `masque.profile=standard` + `vhttp=h2` | `vhttp h2 is not implemented for the standard profile` | `protocol/masque/outbound.go:158` |
| `chain.strip` с неизвестным ключом | ошибка старта | `option/chain_lx.go` (комментарий), реализация в `protocol/chain` |

---

## 7. Сводка lx_only

| сущность | min_core | build tag | где |
|---|---|---|---|
| `tls.reality.key_share` | **1.14.1-lx.4** | — | `option/tls.go` |
| `vless.encryption` | 1.14.0-lx.18 | — | `option/vless.go` |
| транспорт `xhttp` + `xmux` | 1.13.13-lx.1 | `with_xhttp` | `option/v2ray_xhttp*.go`, `v2ray_transport.go` |
| outbound `masque` | 1.14.0-lx.1 | `with_quic` | `option/masque.go` |
| outbound `chain` | 1.14.0-lx.27 | `with_lx_chain` | `option/chain_lx.go` |
| AmneziaWG 2.0 (`jc`…`i5`, `id`/`ip`/`ib`) | 1.13.13-lx.1 | `with_awg` | `option/wireguard_awg.go` |
| AmneziaWG 3.x (`header_protection_key`, ranged timings, `random_trailers`, `disable_cookies`) | 1.14.0-lx.32 | `with_awg` | `option/wireguard_awg.go` |
| `peers[].persistent_keepalive_interval` в форме `min-max` | 1.14.0-lx.32 | `with_awg` | `option/wireguard.go` |

Не-lx (upstream 1.14, но отсутствуют в v1.13.11 — не помечать `lx_only`): `tls.engine`, `tls.spoof`/`spoof_method`, `tls.handshake_timeout`, `anytls.client_metadata`, `ssh.cipher`/`mac`/`kex_algorithm`, `wireguard.udp_mapping`/`udp_filtering`/`udp_nat_max`, `tailscale.ssh_server`/`listen_port`/`taildrop_directory`, `hysteria2.bbr_profile`/`realm`/`hop_interval_max`/`disable_chrome_parrot`, `domain_resolver.disable_optimistic_cache`.

---

## 8. Чего не удалось определить из кода

- `network_strategy` — enum живёт в `C.StringToNetworkStrategy`, из `option/*.go` не виден.
- `udp_mapping` / `udp_filtering` (`UDPNATBehavior`) — enum не извлечён.
- `tls.alpn`, `tls.cipher_suites`, `ssh.cipher`/`mac`/`kex_algorithm`/`host_key_algorithms`, `shadowsocks.plugin`, `transport.method` (http) — фиксированного enum в ядре нет, значения свободные.
- `vless.encryption` — свободная строка, формат только в комментарии.
- Дефолты `anytls.idle_session_*` (30s, кламп `<= 5s`) и `tuic.heartbeat` (10s) живут не в ядре, а в библиотеках `anytls/sing-anytls@v0.0.11/session/client.go:52-56` и `sagernet/sing-quic@v0.7.0/tuic/client.go:54`; в `option/` их нет.
