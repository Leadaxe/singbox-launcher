# Список содержательных `.expected.lxbox.json` для классификации

Материал для LxBox (пункт 1 согласованного порядка). Правок корпуса и кода не содержит.

**81 файл, 84 строки различий** (в трёх файлах различие двойное).
Побайтовые копии (198 шт.) в список не входят.

**Класса «расходится вердикт приёма ноды» нет ни одного** — ни в одном кейсе стороны
не расходятся в том, принять узел или уронить.

## Приоритет разбора

1. **ПОЛЕ (4)** — теряются данные узла. Смотреть первым: `certificate_public_key_sha256`
   у hysteria2 — это пиннинг сертификата, его отсутствие меняет свойства безопасности соединения.
2. **МЕТКА (6)** — расходится имя узла при URI без `#fragment`. Launcher подставляет метку
   из userinfo, и в двух vless-кейсах это **uuid в открытом виде как имя узла** — стоит решить,
   не является ли поведение launcher'а нежелательным (утечка идентификатора в UI/логи/бэкап).
3. **WARNING (36)** — коды деградации не доезжают до Dart. Ваш класс N9/N10.
4. **СХЕМА (14)** — два паттерна: `ss`/`shadowsocks`, `socks5`/`socks`. Похоже на by-design,
   но нигде не записано → кандидат в реестр/спеку.
5. **NULL_VS_EMPTY (24)** — разбора не требует: `null` (Go nil-срез) против `[]` (Dart).
   Снимается нормализацией в раннерах, по одной правке с каждой стороны.



## WARNING — код деградации не доезжает (ваш класс «код не доезжает») — 36

| # | кейс | ключ | launcher vs lxbox |
|---|---|---|---|
| 1 | `anytls/min_idle_session_invalid` | `warnings` | **нет у Dart:** `anytls_min_idle_invalid` |
| 2 | `anytls/security_none_params_kept` | `warnings` | **нет у Go:** `tls_insecure` |
| 3 | `anytls/session_fields` | `warnings` | **нет у Go:** `tls_insecure` |
| 4 | `http/proxy_https_full_tls` | `warnings` | **нет у Go:** `tls_insecure` |
| 5 | `hysteria2/allowinsecure_alias` | `warnings` | **нет у Go:** `tls_insecure` |
| 6 | `hysteria2/hy2_alias` | `warnings` | **нет у Go:** `tls_insecure` |
| 7 | `hysteria2/mport_authority_comma_recovery` | `warnings` | **нет у Go:** `tls_insecure` |
| 8 | `masque/vhttp_invalid_forced_h3` | `warnings` | **нет у Dart:** `masque_vhttp_invalid` |
| 9 | `naive/canonical_full` | `warnings` | **нет у Dart:** `naive_padding_ignored` |
| 10 | `naive/fragment_utf8_escaped` | `warnings` | **нет у Dart:** `naive_padding_ignored` |
| 11 | `naive/https_anonymous_padding_true` | `warnings` | **нет у Dart:** `naive_padding_ignored` |
| 12 | `naive/https_userpass_padding_false` | `warnings` | **нет у Dart:** `naive_padding_ignored` |
| 13 | `trojan/ech_bare_name_ignored` | `warnings` | **нет у Go:** `ech_ignored` |
| 14 | `trojan/ech_name_resolver_ignored` | `warnings` | **нет у Go:** `ech_ignored` |
| 15 | `trojan/ws_ed_path_tail` | `warnings` | **нет у Dart:** `ws_early_data_converted` |
| 16 | `trojan/ws_ed_path_tail_beats_flat` | `warnings` | **нет у Dart:** `ws_early_data_converted` |
| 17 | `trojan/ws_path_double_encoded_ed_tail` | `warnings` | **нет у Dart:** `ws_early_data_converted` |
| 18 | `tuic/canonical_full` | `warnings` | **нет у Go:** `tls_insecure` |
| 19 | `tuic/congestion_bogus_default_cubic` | `warnings` | **нет у Dart:** `tuic_congestion_invalid` |
| 20 | `tuic/cubic_quic_relay_insecure` | `warnings` | **нет у Go:** `tls_insecure` |
| 21 | `tuic/unknown_congestion_dropped` | `warnings` | **нет у Dart:** `tuic_congestion_invalid` |
| 22 | `tuic/v5_cubic` | `warnings` | **нет у Go:** `tls_insecure` |
| 23 | `vless/ech_ignored_reality_kept` | `warnings` | **нет у Go:** `ech_ignored` |
| 24 | `vless/flow_vision_ws_suppressed` | `warnings` | **нет у Go:** `vision_with_transport` |
| 25 | `vless/flow_vision_xhttp_suppressed` | `warnings` | **нет у Go:** `vision_with_transport` |
| 26 | `vless/packet_encoding_garbage_dropped` | `warnings` | **нет у Dart:** `packet_encoding_unknown` |
| 27 | `vless/reality_sid_nbsp_sanitized` | `warnings` | **нет у Dart:** `reality_short_id_invalid` |
| 28 | `vless/reality_sid_nonhex_cleaned` | `warnings` | **нет у Dart:** `reality_short_id_invalid` |
| 29 | `vless/reality_sid_odd_dropped` | `warnings` | **нет у Dart:** `reality_short_id_invalid` |
| 30 | `vless/reality_sid_too_long_dropped` | `warnings` | **нет у Dart:** `reality_short_id_invalid` |
| 31 | `vless/reality_vision_full` | `warnings` | **нет у Go:** `tls_insecure` |
| 32 | `vless/ws_early_data_ed_in_path` | `warnings` | **нет у Dart:** `ws_early_data_converted` |
| 33 | `vmess/fp_junk_in_json` | `warnings` | **нет у Go:** `utls_fp_unknown` |
| 34 | `vmess/json_ws_early_data_ed` | `warnings` | **нет у Dart:** `ws_early_data_converted` |
| 35 | `wireguard/awg_header_ranges` | `warnings` | **нет у Dart:** `awg_header_invalid` |
| 36 | `wireguard/awg_ranged_h_broken_dropped` | `warnings` | **нет у Dart:** `awg_header_invalid` |

## МЕТКА — label узла при URI без `#fragment` — 6

| # | кейс | ключ | launcher vs lxbox |
|---|---|---|---|
| 37 | `tuic/default_port_443` | `label` | L="u" · X=null |
| 38 | `tuic/heartbeat_bare_seconds` | `label` | L="u" · X=null |
| 39 | `tuic/reduce_rtt_alias` | `label` | L="u" · X=null |
| 40 | `tuic/unknown_congestion_dropped` | `label` | L="u" · X=null |
| 41 | `vless/reality_sid_nbsp_sanitized` | `label` | L="11111111-1111-1111-1111-111111111111" · X=null |
| 42 | `vless/tls_pbk_junk_enabled` | `label` | L="11111111-1111-1111-1111-111111111111" · X=null |

## СХЕМА — имя схемы — 14

| # | кейс | ключ | launcher vs lxbox |
|---|---|---|---|
| 43 | `shadowsocks/base64_userinfo` | `scheme` | L=`ss` · X=`shadowsocks` |
| 44 | `shadowsocks/legacy_base64` | `scheme` | L=`ss` · X=`shadowsocks` |
| 45 | `shadowsocks/legacy_full_base64` | `scheme` | L=`ss` · X=`shadowsocks` |
| 46 | `shadowsocks/sip002_aead` | `scheme` | L=`ss` · X=`shadowsocks` |
| 47 | `shadowsocks/sip002_base64_userinfo` | `scheme` | L=`ss` · X=`shadowsocks` |
| 48 | `shadowsocks/sip002_chacha` | `scheme` | L=`ss` · X=`shadowsocks` |
| 49 | `shadowsocks/sip002_escaped_padding` | `scheme` | L=`ss` · X=`shadowsocks` |
| 50 | `shadowsocks/ss2022_blake3` | `scheme` | L=`ss` · X=`shadowsocks` |
| 51 | `socks/auth_userpass` | `scheme` | L=`socks5` · X=`socks` |
| 52 | `socks/default_port_1080` | `scheme` | L=`socks5` · X=`socks` |
| 53 | `socks/no_auth` | `scheme` | L=`socks5` · X=`socks` |
| 54 | `socks/socks5_auth_tag` | `scheme` | L=`socks5` · X=`socks` |
| 55 | `socks/socks5_credentials` | `scheme` | L=`socks5` · X=`socks` |
| 56 | `socks/socks5_no_auth_default_tag` | `scheme` | L=`socks5` · X=`socks` |

## ПОЛЕ — значение поля узла — 4

| # | кейс | ключ | launcher vs lxbox |
|---|---|---|---|
| 57 | `hysteria2/utls_fingerprint_pinsha256` | `tls` | L={"certificate_public_key_sha256": ["YWJjZGVmZ2g="], "enabled": true, "server_name": "hy.example-1.com"} · X={"enabled": true, "server_name": "hy.example-1.com"} |
| 58 | `ssh/full_config` | `client_version` | L="SSH-2.0-OpenSSH_7.4p1" · X=null |
| 59 | `ssh/full_config` | `private_key_path` | L="/home/user/.ssh/id_rsa" · X=null |
| 60 | `ssh/private_key_path` | `private_key_path` | L="$HOME/.ssh/deploy_key" · X=null |

## NULL_VS_EMPTY — артефакт сериализации, разбора не требует — 24

| # | кейс | ключ | launcher vs lxbox |
|---|---|---|---|
| 61 | `anytls/empty_password_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 62 | `anytls/missing_userinfo_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 63 | `masque/missing_address_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 64 | `masque/missing_publickey_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 65 | `naive/bare_scheme_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 66 | `shadowsocks/empty_userinfo_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 67 | `socks/missing_hostname_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 68 | `ssh/missing_hostname_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 69 | `ssh/missing_user_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 70 | `tuic/missing_userinfo_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 71 | `vless/empty_authority_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 72 | `vless/invalid_no_host` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 73 | `vmess/invalid_base64` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 74 | `vmess/not_base64_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 75 | `wireguard/amnezia_vpn_not_base64` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 76 | `wireguard/amnezia_vpn_size_bomb` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 77 | `wireguard/amnezia_vpn_truncated` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 78 | `wireguard/junk_presharedkey_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 79 | `wireguard/junk_publickey_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 80 | `wireguard/masked_private_key_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 81 | `wireguard/missing_address_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 82 | `wireguard/missing_hostname_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 83 | `wireguard/missing_publickey_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |
| 84 | `wireguard/short_private_key_rejected` | `nodes` | Go nil-срез → `null`, Dart → `[]`. Поведение идентично |

**Итого строк: 84** по 81 файлам.

---

## Сверка WARNING-класса с `registry/warnings.json`

Все 8 кодов, не доезжающих до Dart, имеют в реестре `"dart": null`. Из них:

- `reality_pbk_invalid`, `packet_encoding_unknown` — честно помечены «кандидат на добавление класса»;
- `masque_vhttp_invalid` — desc «Desktop-протокол (masque в LxBox нет)» **устарел**;
- `ssh_user_default` — desc «Desktop-протокол» **устарел** (у ssh 11 override-файлов, `full_config`
  даёт распарсенный узел);
- остальные (`ws_early_data_converted`, `naive_padding_ignored`, `reality_short_id_invalid`,
  `tuic_congestion_invalid`, `awg_header_invalid`, `anytls_min_idle_invalid`) — без пометки,
  требуют решения «обязан ли Dart ставить код».

Всего в реестре 49 кодов, из них 21 с `"dart": null` — то есть класс шире, чем видно по корпусу.
