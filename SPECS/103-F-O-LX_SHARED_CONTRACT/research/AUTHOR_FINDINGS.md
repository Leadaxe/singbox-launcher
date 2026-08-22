# Находки авторинга реестра (Фаза 0, воркфлоу wc6s9165z)

## Автор: proto-vless-family

Все шесть файлов существуют по указанным путям в требуемой форме; сверил каждое load-bearing утверждение с текущим кодом обеих кодовых баз (включая develop-состояние SPEC 102) — расхождений с кодом нет, правки не потребовались.

**Написанные файлы:**
1. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/vless.json
2. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/vmess.json
3. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/trojan.json
4. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/shadowsocks.json
5. /Users/macbook/projects/singbox-launcher/contract/registry/transports.json
6. /Users/macbook/projects/singbox-launcher/contract/registry/tls.json

**НОВЫЕ расхождения (вне §9 PLAN), зафиксированы в файлах с refs:**

Системные:
- Регистр query-ключей: Go читает все ключи case-insensitively (queryGetFold), Dart — case-sensitively (точечные исключения queryParamCI).

VLESS:
- `encryption` (§335 post-quantum): парсит только Dart; Go лишь статично пишет `encryption=none` в share-URI (shareuri_vless.go:72).
- `type=h2` в URI vless/trojan: ветка есть только у Dart; Go молча теряет транспорт (uriTransportFromQuery без case "h2").

VMess:
- vmess-JSON `serviceName`: читает только Dart; Go берёт grpc service_name только из path.
- vmess-JSON `mode` (xhttp): читает только Go (parseVMessJSON); Dart не прокидывает — зеркальная пара к предыдущему.
- Go share-URI эмитит httpupgrade как `net=ws` (shareuri_vmess.go:70-74) — подмена wire-протокола на round-trip, кандидат на баг.
- Dart toUriVmess не эмитит `insecure` — флаг теряется на mobile round-trip.
- Query-хвост legacy cleartext: Go читает (network/type/tls), Dart отбрасывает всё после `?`.
- Битый строковый port в vmess-JSON: Go → drop ноды, Dart → fallback 443.

Trojan:
- Userinfo с `:`: Go берёт паролем часть ДО `:`, Dart — весь userinfo целиком.
- Dart emitTrojan пишет `tls:{enabled:false}` (node_spec_emit.dart:211) — конфликт со SPEC 045 (Go омитит ключ) + A-класс расхождение identity-хеша trojan-без-TLS.

Shadowsocks:
- SIP003 `plugin`/`plugin_opts`: только Dart (Go теряет ss-query молча); при этом Dart сам не возвращает plugin в URI-эмит.
- Порт: Go без порта → дефолт 443; Dart host без `:` → drop ноды, неразбираемый порт → 8388.

Транспорты:
- Плоский `ed=N`: только Dart (Go — лишь хвост пути `?ed=N`); плоский `eh` (кастомное early_data_header_name): только Dart, Go всегда константа Sec-WebSocket-Protocol.
- Go httpupgrade URI-ветка НЕ срезает `?ed`-хвост из path (Dart срезает везде; Go — лишь в Xray-JSON ветке) → хвост уедет в конфиг, 404.
- Residual double percent-decode ws-пути (decodeResidualPercent §320): только Dart.
- XHTTP SPEC 102 поля `no_sse_header`, `sc_stream_up_server_secs`, `sc_max_buffered_posts` (int!), `xmux` (вложенный объект): только Go; в Dart XhttpTransport их нет (помечены ext:desktop до закрытия SPEC 102).
- Go-эмиттер share-URI не возвращает no_sse_header/sc_max_buffered_posts/xmux в ссылку — внутренняя эмиттер/парсер-асимметрия Go (память emitter-parser-pairing).
- Xray-алиас `splithttp`/`splithttpSettings`: Go читает, Dart Xray-ветка — только `xhttpSettings` (network=splithttp даст transport без полей).

TLS:
- Политика junk-fp расходится и ломает identity-хеш: Go → drop utls-блока ('' → для vless дефолт random); Dart → замена на 'chrome' + UnknownFingerprintWarning. Пустой fp при reality: Dart → chrome, Go → random.
- Dart не знает алиас-префикс `hellorandom` (только hellorandomized) — уходит в junk.
- Семейство insecure-ключей: Go 3 имени case-insensitive, Dart 5 имён (+allow_insecure, skip-cert-verify) case-sensitive.
- ALPN: Dart выбрасывает элементы, не похожие на валидный protocol-id; Go пропускает как есть (и не декодит alpn во vmess-ветке).
- reality `sid`: нечётная длина/>16 hex — Dart дропает значение в '' целиком; Go обрезает >16 и нечётную длину пропускает (ядро упадёт на encoding/hex) — кандидат на фикс Go по модели Dart.

Паритетные дыры (не расхождения, для фикстур): плоский не-base64 SIP002-userinfo не поддержан нигде; vmess legacy cleartext вне base64 не поддержан нигде (§9.B8 подтверждён).

## Автор: proto-group2-allowlists

Файлы (все валидный JSON, единая форма):
1. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/hysteria2.json (mport/§9.B2 как цель, gecko-ключи ext:mobile)
2. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/tuic.json
3. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/anytls.json (D-009 fp→random как цель, §9.A3)
4. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/naive.json (naive+quic/§9.B1 как цель)
5. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/ssh.json
6. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/socks.json
7. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/http.json (§9.B6: extension null как цель, все URI-params ext:mobile)
8. /Users/macbook/projects/singbox-launcher/contract/registry/allowlists.json (7 списков; utls 15 значений — байт-идентичны в обоих; ss_methods 9 — идентичны)

Новые расхождения (не было в §9 PLAN, найдены по коду; каждое отражено в note соответствующего json):
1. **hysteria2 bandwidth-ключи**: Go читает/пишет `upmbps`/`downmbps`, Dart — `up_mbps`/`down_mbps` — кросс-эмит молча теряет значения.
2. **hysteria2 obfs=gecko**: ядро lx и Dart поддерживают (§358), Go режет везде (`isValidHysteria2ObfsType` только salamander — URI и singbox-sanitize); канон-пересечение в allowlists = [salamander], целевое [salamander, gecko].
3. **uTLS на QUIC-протоколах**: Go URI-путь эмитит `tls.utls` для hysteria2/tuic из `fp=`; Dart намеренно срезает (§282 `toSingboxForQuic`), и сам Go на Xray-пути тоже срезает (xray_protocols.go:276) — внутреннее противоречие Go + разрыв identity.
4. **TUIC дефолты в эмите**: Dart всегда пишет `congestion_control`(cubic), `udp_relay_mode`(native), `alpn`(["h3"]); Go — только при наличии в URI → identity-хеши tuic-нод расходятся, противоречит CANON «дефолты не пишутся».
5. **TUIC params**: `fp`/`fingerprint` и `heartbeat` — только Go; `disable_sni` — только Dart; zero-RTT алиасы разъехались (Go: zero_rtt_handshake+reduce_rtt, Dart: reduce_rtt+zero_rtt).
6. **anytls REALITY**: `security=reality`/`pbk`/`sid` парсит и эмитит только Dart; Go теряет REALITY-блок на anytls.
7. **naive одиночный userinfo**: `naive+https://secret@host` — Go кладёт в username, Dart в password; Go-эмит password-only ноды пишет пароль в user-слот → Dart прочитает наоборот.
8. **ssh без user**: Go дефолтит root с warning, Dart дропает ноду.
9. **ssh inline private_key в share-URI**: Go отказывается кодировать (ErrShareURINotSupported), Dart пишет секрет в query — нужна единая политика.
10. **http из sing-box-импорта в Go**: type http принимается (singbox_import.go:315), но в GenerateNodeJSON нет http-ветки → нода молча урезается до {tag,type,server,server_port} (ловушка emitter-parser-pairing); чинить вместе с B6.
11. **uTLS junk-деградация**: Go → срезает utls-блок (""), Dart → chrome + warning; плюс alias-префикс `hellorandom`→random есть только в Go.
12. **Xray-источник hysteria**: Go принимает protocol `"hysteria2"`, Dart — `"hysteria"` (version:2, форма форка) — наборы не пересекаются вовсе.
13. Минорные: `skipCertVerify` (camelCase) — только Go (tuic/anytls); `allow_insecure` на hysteria2 — только Dart; `pinSHA256`→certificate_public_key_sha256 — только Go; base64-обёрнутый hysteria2-payload — только Go; anytls-пароль с `:` — Dart сохраняет целиком, Go берёт только Username(); socks password-only — Dart теряет пароль в toUriSocks, Go сохраняет.

Порядок query-параметров эмита: Go всюду сортирует алфавитно (url.Values.Encode), Dart — фиксированный порядок вставки; в `emit.param_order` зафиксирован порядок Dart, это отмечено в note (cross-emit сверяет по смыслу, не по байтам).

## Автор: proto-endpoints-groups

Файлы (все валидный JSON, по форме registry.schema.json):
1. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/wireguard.json — kind endpoint, aliases wg/awg, sources uri/singbox/wgconf/amnezia; D-010 отражён (name/system не эмитятся); AWG jc/jmin/jmax/s1-s4/h1-h4 (+ranged "lo-hi"), i1-i5, masquerade id/ip/ib; mtu-семантика с AWG-клампом ≤1280; reserved (WARP).
2. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/masque.json — **kind записан "outbound", НЕ "endpoint" как в задании**: оба проекта эмитят masque в outbounds[] (Go GenerateNodeJSON:281, ядро outbound.Register; Dart MasqueSpec → Outbound, node_spec_emit.dart:649). Legacy network/sni → vhttp/tls зафиксированы.
3. /Users/macbook/projects/singbox-launcher/contract/registry/protocols/group.json — kind group, selector|urltest, члены по label; autogroup:// — все параметры ext:mobile, share_uri:false, непереносим.
4. /Users/macbook/projects/singbox-launcher/contract/registry/containers.json — amnezia_vpn_link / wgconf_ini / base64_body с decode-цепочками, лимитами и refs на оба кода; §9.B11/B12/B4/B9 отражены как целевое состояние.

Новые расхождения (не было в §9 PLAN; кандидаты в таблицу):
- **masque kind**: задание относило к endpoint — фактически outbound в обоих проектах (исправлено в реестре).
- **Plain-WG дефолт MTU**: Go 1420 vs Dart 1408 — влияет на identity-хеш WG-нод без mtu= (кандидат в §9.A).
- **Валидация WG-ключей**: Go нормализует base64→32 байта (нода дропается на мусоре), Dart не валидирует вовсе — мусорный ключ уедет в конфиг и уронит check (класс pbk=enabled).
- **allowedips**: в URI Go требует (нода дропается), Dart дефолтит 0.0.0.0/0,::/0; в INI Go address обязателен, в Dart опционален.
- **reserved из INI**: Dart читает Reserved/ClientId из [Peer], Go wgConfToURI его ТЕРЯЕТ → WARP-conf на десктопе без reserved (handshake OK, трафика нет).
- **reserved-формы**: Dart дополнительно принимает base64 client_id и алиас-параметр client_id; Go — только "b0,b1,b2".
- Алиасы только Dart: схема wg://, параметры allowed_ips/preshared_key/public_key, private_key-fallback из query у wireguard (у masque fallback есть в обоих).
- Только Go (desktop-ext): listenport, name, dns (dns при этом самим Go-парсером игнорируется — лоссы by design), masque insecure/skip_cert_verify/allowinsecure, входной алиас keep_alive_period.
- Только Dart (mobile-ext): masque disable_sni.
- **masque vhttp**: Go валидирует h3/h2 (warn+force h3), Dart принимает любое значение как есть.
- **Multi-peer WG в sing-box импорте**: Go сохраняет всех peers (map as-is), Dart берёт только peers.first — тихая потеря.
- **sing-box группы**: Go сохраняет selector selector'ом (+default при вхождении в состав), Dart конвертит в urltest (default теряется); члены Go по тегам / Dart по identity-ключам; Dart эмитит urltest-опции всегда со своими дефолтами, Go — по наличию.
- **Xray-балансировщик**: Go игнорирует strategy (plain urltest, дефолты gstatic/3m), Dart маппит strategy→mode/pool (дефолты cp.cloudflare.com/15m) — разные дефолты url/interval.
- **Amnezia**: Go требует qCompress-фрейминг, Dart принимает и несжатый base64-JSON; Go импортирует ОДИН контейнер (default предпочтён, рекурсивный скан depth 8), Dart — ВСЕ awg/wireguard (плоский путь last_config.config); подстановка $PRIMARY_DNS/$SECONDARY_DNS — только Dart; лимиты расходятся: ссылка 512 KiB (Go) vs 65536 (Dart), анти-bomb 8 MiB (Go) vs 4 MiB (Dart) — PLAN §9.B12 ошибочно считал cap 4 MiB общим.
- **masquerade из INI**: id/ip/ib читает только Dart; i1-эксклюзивность guard'ит только Go.
- **Порядок query-параметров эмита**: Go алфавитный (url.Values.Encode), Dart семантический — URI байтово разные, парс-эквивалентные (пометено как не нормативное).
- Мелочь: Dart INI-детект тела требует "[Peer]" с точным регистром; Go paste-путь поддерживает несколько [Interface]-блоков в одной вставке, Dart — один на тело.

## Автор: warnings-limits

Файлы:
- /Users/macbook/projects/singbox-launcher/contract/registry/warnings.json — 38 кодов: все 18 Dart-подклассов NodeWarning (включая ech_ignored ← EchIgnoredWarning §320), 19 Go-деградаций без Dart-класса, clash_yaml_unsupported (§9.B5). Severity везде по Dart, расхождения отмечены в desc.
- /Users/macbook/projects/singbox-launcher/contract/registry/limits.json — 9 лимитов: пять из D-013 + amnezia_link_max_bytes (512 KiB), amnezia_scan_depth (8), fetch_timeout_seconds, disabled_ttl_clamp {3×interval, 24h, 720h} (уже идентичен).

НОВЫЕ расхождения, не учтённые в §9 PLAN / DECISIONS (кандидаты в таблицу):
1. amnezia_decompress_cap: PLAN §9.B12 называет cap 4 MiB «уже общим» — фактически Go = 8 MiB (node_parser_amnezia.go:41), Dart = 4 MiB; плюс Dart проверяет только claimed-length из qCompress-заголовка, а не фактический распакованный вывод (guard слабее).
2. utls_fp_unknown, fallback для мусорного fp: Dart → chrome + warning; Go URI-путь → random молча (node_parser_transport.go:646); Go singbox-импорт → снимает utls-блок целиком (singbox_sanitize.go:177). Три поведения, нужен единый канон.
3. hysteria2 obfs allowlist: Go = только salamander (node_parser_hysteria2.go:14) — gecko молча дропается, хотя ядро его поддерживает (у LxBox это был баг #53); Dart = salamander+gecko.
4. hysteria2 obfs без пароля: Go проверяет только в singbox-импорте (singbox_sanitize.go:292); URI-путь эмитит obfs без password (node_parser_hysteria2.go:44-47) → «missing obfs password» fatal для всего конфига — живой баг Go.
5. vpn:// с несколькими WG/AWG-контейнерами: Go импортирует один (дефолтный предпочтителен, node_parser_amnezia.go:171), Dart — все (amnezia_link.dart). Нужно решение.
6. fetch_timeout: Go 30s / Dart 9s — решения нет, value=30 в limits.json помечен как предложение.

Замечания: Go xhttp сегодня без param-reset нормализации (SPEC 102 в работе — xhttp_param_reset у Go null); Go source-detour (fail-closed drop, outbound_generator.go:1200) выделен отдельным кодом source_detour_missing от импортного detour_target_missing (политики противоположны by design).

## Автор: template-lang-vars

Файлы (все — валидный JSON/MD):
1. /Users/macbook/projects/singbox-launcher/contract/docs/TEMPLATE_LANG.md — нормативная спека v1: структура шаблона + правило толерантности, 8 канонических типов var (канон dns_servers; алиасы dns_server, number→int), таблица coercion строго по type (int c clamp [0,65535], не-число → строка advisory), полная таблица предикатов #if (P1–P7, все решения C1/C2/C3/C8 отражены), map-spread/array-element/else/вложенность, Dropped-каскад и unresolved по D-011, пресеты с префиксом `<preset_id>:<tag>` по D-012 (outbounds add/update — desktop-расширение), помеченные desktop/mobile-расширения, финальная таблица «разрыв → чей ход» C1–C9 (Dart: C1,C2,C6,C7,C8; Go: C3,C4,C5; оба: C9) + 8 новых разрывов N1–N8.
2. /Users/macbook/projects/singbox-launcher/contract/registry/vars.json — 47 имён.
3. /Users/macbook/projects/singbox-launcher/contract/registry/presets.json — 21 id.

Пересечение vars: 16 общих имён, из них **15 portable** (log_level, auto_detect_interface, dns_strategy, dns_final, dns_default_domain_resolver, resolve_strategy, urltest_url, urltest_interval, urltest_tolerance, tun_mtu, tun_stack, tls_fragment, tls_record_fragment, tls_fragment_fallback_delay, tls_mixed_case_sni); 16-е — tun_address (имя общее, семантика разная → portable=false). App-флаги LxBox исключены (живут только в коде, settings_storage.dart:164).

Пересечение preset id: **4 буквальных** — block-ads, ru-inside, bittorrent, fakeip.

Новые расхождения (не было в §9 PLAN):
- **N1**: одинаковая семантика под разными именами vars — cert_store↔certificate_store, strict_route↔tun_strict_route, tun_interface_name↔tun_name, семейство proxy_in_*↔proxy_* (6 пар) — блокирует перенос vars в бэкапе;
- **N2**: пары preset id — private-ips↔private-ip, russian↔ru-direct (семантика идентична, ссылка непереносима);
- **N3**: поле дефолта preset-var: Go пишет `default`, канон/Dart — `default_value`;
- **N4**: секция пресетов presets[]+id vs selectable_rules[]+preset_id/ui{};
- **N5**: объектная форма options форсит type→enum только в Go;
- **N6**: условность фрагментов пресета — Go if/if_or vs Dart enabled:"@var" (канон не выбран);
- **N7**: tun_address text_list vs text+ipv6_enabled/tun_address6;
- **N8**: форма записей dns_options.servers (плоская vs vars[]+server{}) — тема SPEC 105.

## Проверяющий 1

All files and their refs have been verified against both codebases. Final defect list:

**vless.json**
1. `flow` note: quirk `xtls-rprx-vision-udp443` описан как общий «vision + packet_encoding=xudp + server_port=443». Порт на 443 переписывает Go на обоих путях (`core/config/subscription/node_parser_core.go:600-607`, `xray_outbound_convert.go:153-156`) и Dart только в Xray-JSON ветке (`LxBox/app/lib/services/parser/json_parsers.dart:424-428`), но Dart URI-парсер порт НЕ трогает (`LxBox/app/lib/services/parser/uri_parsers/vless_parser.dart:31-35` — меняются только flow и packetEncoding, порт из строки 17 остаётся). Реальное расхождение подано как паритет, «новым расхождением» не помечено.

**vmess.json**
1. `userinfo` note: «'#fragment' после base64-части читается обоими (percent-decoded label)» — верно только для legacy-cleartext формы. В основной JSON-форме fragment выбрасывают ОБА: Go не передаёт его в parseVMessJSON (`core/config/subscription/node_parser_vmess.go:57` — вызов без fragmentLabel; используется только в parseVMessLegacyCleartext:92), Dart не передаёт в _vmessFromJson (`LxBox/app/lib/services/parser/uri_parsers/vmess_parser.dart:30-31`; fragment уходит только в _vmessLegacy:36). Поле `"fragment": "label"` в этой схеме работает только для legacy-ветки.

**trojan.json: чисто**

**shadowsocks.json: чисто**

**hysteria2.json**
1. emit note «Порядок — вставка Dart toUriHysteria2» — неверно: Dart `_buildUri` → `buildQuery` сортирует ключи лексикографически (`LxBox/app/lib/services/parser/uri_utils.dart:306-312`). Соседние vless/trojan.json сами это фиксируют («Dart buildQuery sort»).
2. `param_order` содержит `upmbps`,`downmbps`,`mport` как эмит Dart — Dart пишет `up_mbps`/`down_mbps` (`LxBox/app/lib/models/node_spec_emit.dart:360-361`), а `mport` не эмитит вовсе (нет поля в Hysteria2Spec, §9.B2; mport-эмит только Go `shareuri_hysteria2.go:52-58`). Противоречит собственной note про разрыв имён upmbps/up_mbps.

**tuic.json**
1. degrade «отсутствие uuid или password → Go: ошибка ноды» — для password неверно: `tuic://uuid@host` проходит валидацию (`node_parser_core.go:308-316` проверяет только Username), buildTuicOutbound лишь пишет warning и нода живёт (`node_parser_tuic.go:62-66`). То же завышение в userinfo note «Оба обязательны».
2. emit note «Порядок — вставка Dart toUriTuic» — неверно, `buildQuery` сортирует (`uri_utils.dart:306-312`; toUriTuic → `node_spec_emit.dart:456`).

**anytls.json**
1. `insecure` note «allow_insecure на этом пути — только Dart» — неверно: Go явно читает `allow_insecure` (`core/config/subscription/node_parser_anytls.go:64-65`, `tuicQueryFlagTrue(q, "allow_insecure")`).
2. degrade «пустой пароль → Go: warning, нода живёт» — неверно: anytls входит в валидацию userinfo (`node_parser_core.go:308-316`) → пустой userinfo = drop ноды; warning-ветка `node_parser_anytls.go:24-25` с URI-пути недостижима.
3. `sni` note «Мусор (без точки/двоеточия, эмодзи) → fallback на server» подан как общий — эвристика только у Go (`node_parser_anytls.go:58`); Dart через parseVlessTls фолбэчит только ПУСТОЙ sni (`LxBox/app/lib/services/parser/transport.dart:330-331`), мусор уходит в server_name как есть.
4. emit note «Порядок — вставка Dart toUriAnyTls» — неверно, `buildQuery` сортирует.

**naive.json: чисто**

**socks.json**
1. «version '4'/'4a' достижим только через sing-box-импорт в Dart — param-level mobile-расширение» — неверно: Dart parseSingboxEntry в socks-ветке `entry['version']` не читает (`LxBox/app/lib/services/parser/json_parsers.dart:973-984`), version нигде не заполняется из импорта — поле модели с дефолтом '5' (`node_spec.dart:542,553`), '4'/'4a' недостижимы ни одним parse-путём.

**ssh.json**
1. userinfo note + degrade: «Go при пустом user из singbox-импорта дефолтит root с warning» — неверно и перепутано по проектам. Go singbox-импорт кладёт map как есть без дефолта user (`singbox_import.go:248-298`), GenerateNodeJSON просто не эмитит user (`outbound_generator.go:310-316`); root-ветка `node_parser_ssh.go:20-23` вызывается только с URI-пути (`node_parser_core.go:781`), где пустой userinfo уже отсечён валидацией (308-316) — мёртвый код; root без warning есть в share-URI эмиттере (`shareuri_ssh.go:12-15`). Root на singbox-импорте дефолтит как раз Dart (`LxBox/app/lib/services/parser/json_parsers.dart:967`), а «Dart: null-skip» верно только для URI-пути (`ssh_parser.dart:17`).
2. emit note «Порядок — вставка Dart toUriSsh» — неверно, `buildQuery` сортирует (`node_spec_emit.dart:494`).

**http.json**
1. «нода молча урезается до {tag,type,server,server_port}» — завышено: общие секции GenerateNodeJSON всё же эмитят transport и tls из импортированной map (`outbound_generator.go:368` и `370-438`); теряются username/password/path/headers, а не всё кроме четырёх полей.

**transports.json**
1. xhttp note: «алиас network 'splithttp' … оба читают… Dart Xray-ветка читает ТОЛЬКО xhttpSettings — network 'splithttp' даст transport без полей» — неверно вдвойне: у Dart `_xrayTransportFromStream` нет ветки 'splithttp' вообще (`LxBox/app/lib/services/parser/json_parsers.dart:743-792`, case только 'xhttp':781) → network 'splithttp' возвращает null, т.е. узел остаётся ВООБЩЕ без транспорта (plain tcp), а не «transport без полей»; и алиас network Dart не читает — «оба читают» относится только к Go (`xray_outbound_convert.go:325-336`).

**tls.json: чисто**

**allowlists.json**
1. `vless_flow` note: «Go гейтит только на singbox-импорте…, а URI-путь пропускает неизвестный flow как есть — разрыв строгости, целевое = allowlist на всех путях» — неверно: финальный эмиттер Go фильтрует flow на ВСЕХ путях — гасит и не-vision значения, и vision при транспорте (`core/config/outbound_generator.go:351-358`); в buildOutbound мусор попадает в Outbound["flow"], но в config.json не доезжает. Заявленного разрыва в итоговом конфиге нет (в `vless.json` это же место описано корректно — «drop поля»); противоречие двух файлов реестра между собой.

## Проверяющий 2

Проверка завершена (каждый файл зоны сверен с кодом обоих проектов в обе стороны). Дефекты:

**contract/registry/protocols/wireguard.json**
1. `emit.note`: «mtu только != дефолта (Go: !=1420)» подано как общее правило — у Dart подавления дефолта нет: `toUriWireguard` эмитит mtu всегда, когда он задан (`LxBox/app/lib/models/node_spec_emit.dart:623`), а после URI/INI-парса он задан всегда, включая дефолт 1408/1280 (`app/lib/services/parser/uri_parsers/wireguard_parser.dart:70-71`). Правило «!= дефолта» — только Go (`core/config/subscription/shareuri_wireguard.go:60`).
2. `query.mtu.note` и `degrade[1]`: «битое значение → параметр опускается» неверно для mtu — поле mtu в endpoint эмитится всегда, при битом значении остаётся дефолт, а не опускается (Go `core/config/subscription/node_parser_wireguard.go:117-129,183` — mtu безусловно в endpoint; Dart `wireguard_parser.dart:70-71`). Реально опускаются только keepalive/listenport/reserved.

**contract/registry/protocols/masque.json**: чисто.

**contract/registry/protocols/group.json**
1. `emit.note`: «Go generateGroupNodeJSON переносит опции по закрытому списку url/interval/tolerance/idle_timeout/interrupt_exist_connections + default» — закрытый список применяется не при эмиссии, а при импорте (`singboxGroupToNode`, `core/config/subscription/singbox_groups.go:32-38,111-127`); сам `generateGroupNodeJSON` эмитит ВСЕ поля Outbound подряд в сортировке (`core/config/outbound_generator.go:977-994`). Для Xray-групп в Outbound лежат url/interval без фильтра (`xray_balancer.go:66-73`).
2. `degrade[7]`: «Xray strategy неизвестного типа → round_robin по всему набору (Dart)» — фактически для неизвестного type `spreadAll=false`, pool = `expected ?? 3` (дефолт), а не весь набор: «весь набор» только для null/random/roundRobin (`LxBox/app/lib/services/parser/json_parsers.dart:281-300`).

**contract/registry/containers.json**: чисто.

**contract/registry/warnings.json**: чисто (все 40+ go/dart-refs и классы NodeWarning сошлись построчно).

**contract/registry/limits.json**: чисто (все значения и refs подтверждены: 8192/65536, 3000, 10 MiB, 8/4 MiB, 524288, 8, 30s/9s, clamp 24h–720h).

**contract/registry/vars.json**: чисто (32 desktop-вара и 31 mobile-вар — полное покрытие без выдуманных имён; типы, дефолты 5m/15m, 100/30, 1492, prefer_ipv4/ipv4_only, ''/500ms, N1-пары, on_change ipv6_enabled и fakeip→resolve_enabled — всё сошлось).

**contract/registry/presets.json**: чисто (15 desktop + 10 mobile id — полное покрытие; rule_set-URL block-ads/ru-blocked, vars ru-direct/fakeip/ru-inside, locked/num=0 traffic-processing, invert+package_name_regex unknown-traffic подтверждены).

**contract/docs/TEMPLATE_LANG.md**
1. §4.2 P7: «вложенные `and`/`or` как предикат» заявлены ядром с «Разрыв: —» (т.е. как уже существующий паритет) — **ни один движок эту форму не поддерживает**: Go даёт warn+false на неизвестный ключ предиката (`core/template/substitute.go:386`), Dart — false в рантайме (не-@ ключ, `app/lib/services/builder/if_engine.dart:277-279,307-308`) и **TemplateIfError на load** (`if_engine.dart:452-460`); в `research/CHECK_TEMPLATE_ENGINE.md` §1 такой формы предиката нет вовсе.
2. §4.2 (после таблицы): «неизвестная переменная в предикате → false **+ warning** (обе стороны уже согласны: … if_engine.dart:312-316)» — у Dart warning нет: resolve→null→`''`→false молча (`if_engine.dart:265,281,310-315`; research §3 фиксирует именно «без warning»), а необъявленное имя в предикате у Dart вообще ошибка загрузки (`_requireVar`, `if_engine.dart:435,461,508`).
3. §4.1: «Неизвестные ключи, начинающиеся с `#`, выбрасываются **с warning** … обе реализации уже согласны» — Dart удаляет их молча (`if_engine.dart:121-133`, лога нет; research §4: «silent drop»); warning только в Go (`substitute.go:127`).
4. §6.1: «сингулярные формы `rule` / `dns_rule` — синонимы "список из одного" (**обе реализации их уже читают**)» — Go сингулярный `rule` не читает: у Preset только `Rules json:"rules"` (`core/template/preset_types.go:63`), сингулярное поле есть лишь для `dns_rule` (`preset_types.go:66`). Обе формы читает только Dart (`app/lib/models/parser_config.dart:779,788`). Mobile-шаблон реально пишет `rule` — на десктопе такой пресет молча потеряет правило.
5. (мелкое) §2.2 нормирует bool-коэрцию как trim+case-insensitive, но существующий разрыв Dart-коэрции (`raw == 'true'` строго, `if_engine.dart:40`; research §2 это фиксирует) не отражён ни в одной строке разрывов — C1 в §9 указывает «где чинить» только bare-предикат `if_engine.dart:262-266`.
