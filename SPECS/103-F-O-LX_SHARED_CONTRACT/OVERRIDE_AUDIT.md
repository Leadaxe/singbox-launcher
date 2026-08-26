# Аудит содержательных `.expected.lxbox.json` (корпус URI)

Всего override-файлов с реальным различием: **81** из 279 (остальные 198 — побайтовые копии основного expected, в список не включены).

Классы по убыванию риска:

- **ВЕРДИКТ** — расходится сам приём/отклонение ноды: одна сторона узел принимает, другая роняет. Самое опасное: один и тот же URI даёт разный набор нод.
- **СХЕМА** — разное имя схемы у одного узла.
- **ПОЛЯ** — расходятся значения полей узла.
- **WARNINGS** — расходится только набор кодов деградации (класс N9/N10).

Обозначения: `L` — launcher (`.expected.json`), `X` — LxBox (`.expected.lxbox.json`).


## СХЕМА — 14

- `shadowsocks/base64_userinfo`
  - scheme launcher=['ss'] lxbox=['shadowsocks']
- `shadowsocks/legacy_base64`
  - scheme launcher=['ss'] lxbox=['shadowsocks']
- `shadowsocks/legacy_full_base64`
  - scheme launcher=['ss'] lxbox=['shadowsocks']
- `shadowsocks/sip002_aead`
  - scheme launcher=['ss'] lxbox=['shadowsocks']
- `shadowsocks/sip002_base64_userinfo`
  - scheme launcher=['ss'] lxbox=['shadowsocks']
- `shadowsocks/sip002_chacha`
  - scheme launcher=['ss'] lxbox=['shadowsocks']
- `shadowsocks/sip002_escaped_padding`
  - scheme launcher=['ss'] lxbox=['shadowsocks']
- `shadowsocks/ss2022_blake3`
  - scheme launcher=['ss'] lxbox=['shadowsocks']
- `socks/auth_userpass`
  - scheme launcher=['socks5'] lxbox=['socks']
- `socks/default_port_1080`
  - scheme launcher=['socks5'] lxbox=['socks']
- `socks/no_auth`
  - scheme launcher=['socks5'] lxbox=['socks']
- `socks/socks5_auth_tag`
  - scheme launcher=['socks5'] lxbox=['socks']
- `socks/socks5_credentials`
  - scheme launcher=['socks5'] lxbox=['socks']
- `socks/socks5_no_auth_default_tag`
  - scheme launcher=['socks5'] lxbox=['socks']

## ПОЛЯ — 31

- `anytls/empty_password_rejected`
  - различие вне entry/warnings
- `anytls/missing_userinfo_rejected`
  - различие вне entry/warnings
- `hysteria2/utls_fingerprint_pinsha256`
  - `tls`: L={"certificate_public_key_sha256": ["YWJjZGVmZ2g="], "enabled": true, "server_name": "hy.example-1.com"} · X={"enabled": true, "server_name": "hy.example-1.com"}
- `masque/missing_address_rejected`
  - различие вне entry/warnings
- `masque/missing_publickey_rejected`
  - различие вне entry/warnings
- `naive/bare_scheme_rejected`
  - различие вне entry/warnings
- `shadowsocks/empty_userinfo_rejected`
  - различие вне entry/warnings
- `socks/missing_hostname_rejected`
  - различие вне entry/warnings
- `ssh/full_config`
  - `client_version`: L="SSH-2.0-OpenSSH_7.4p1" · X=null; `private_key_path`: L="/home/user/.ssh/id_rsa" · X=null
- `ssh/missing_hostname_rejected`
  - различие вне entry/warnings
- `ssh/missing_user_rejected`
  - различие вне entry/warnings
- `ssh/private_key_path`
  - `private_key_path`: L="$HOME/.ssh/deploy_key" · X=null
- `tuic/default_port_443`
  - различие вне entry/warnings
- `tuic/heartbeat_bare_seconds`
  - различие вне entry/warnings
- `tuic/missing_userinfo_rejected`
  - различие вне entry/warnings
- `tuic/reduce_rtt_alias`
  - различие вне entry/warnings
- `vless/empty_authority_rejected`
  - различие вне entry/warnings
- `vless/invalid_no_host`
  - различие вне entry/warnings
- `vless/tls_pbk_junk_enabled`
  - различие вне entry/warnings
- `vmess/invalid_base64`
  - различие вне entry/warnings
- `vmess/not_base64_rejected`
  - различие вне entry/warnings
- `wireguard/amnezia_vpn_not_base64`
  - различие вне entry/warnings
- `wireguard/amnezia_vpn_size_bomb`
  - различие вне entry/warnings
- `wireguard/amnezia_vpn_truncated`
  - различие вне entry/warnings
- `wireguard/junk_presharedkey_rejected`
  - различие вне entry/warnings
- `wireguard/junk_publickey_rejected`
  - различие вне entry/warnings
- `wireguard/masked_private_key_rejected`
  - различие вне entry/warnings
- `wireguard/missing_address_rejected`
  - различие вне entry/warnings
- `wireguard/missing_hostname_rejected`
  - различие вне entry/warnings
- `wireguard/missing_publickey_rejected`
  - различие вне entry/warnings
- `wireguard/short_private_key_rejected`
  - различие вне entry/warnings

## WARNINGS — 36

- `anytls/min_idle_session_invalid`
  - `warnings`: L=[["anytls_min_idle_invalid"]] · X=[null]
- `anytls/security_none_params_kept`
  - `warnings`: L=[null] · X=[["tls_insecure"]]
- `anytls/session_fields`
  - `warnings`: L=[null] · X=[["tls_insecure"]]
- `http/proxy_https_full_tls`
  - `warnings`: L=[null] · X=[["tls_insecure"]]
- `hysteria2/allowinsecure_alias`
  - `warnings`: L=[null] · X=[["tls_insecure"]]
- `hysteria2/hy2_alias`
  - `warnings`: L=[null] · X=[["tls_insecure"]]
- `hysteria2/mport_authority_comma_recovery`
  - `warnings`: L=[null] · X=[["tls_insecure"]]
- `masque/vhttp_invalid_forced_h3`
  - `warnings`: L=[["masque_vhttp_invalid"]] · X=[null]
- `naive/canonical_full`
  - `warnings`: L=[["naive_padding_ignored"]] · X=[null]
- `naive/fragment_utf8_escaped`
  - `warnings`: L=[["naive_padding_ignored"]] · X=[null]
- `naive/https_anonymous_padding_true`
  - `warnings`: L=[["naive_padding_ignored"]] · X=[null]
- `naive/https_userpass_padding_false`
  - `warnings`: L=[["naive_padding_ignored"]] · X=[null]
- `trojan/ech_bare_name_ignored`
  - `warnings`: L=[null] · X=[["ech_ignored"]]
- `trojan/ech_name_resolver_ignored`
  - `warnings`: L=[null] · X=[["ech_ignored"]]
- `trojan/ws_ed_path_tail`
  - `warnings`: L=[["ws_early_data_converted"]] · X=[null]
- `trojan/ws_ed_path_tail_beats_flat`
  - `warnings`: L=[["ws_early_data_converted"]] · X=[null]
- `trojan/ws_path_double_encoded_ed_tail`
  - `warnings`: L=[["ws_early_data_converted"]] · X=[null]
- `tuic/canonical_full`
  - `warnings`: L=[null] · X=[["tls_insecure"]]
- `tuic/congestion_bogus_default_cubic`
  - `warnings`: L=[["tuic_congestion_invalid"]] · X=[null]
- `tuic/cubic_quic_relay_insecure`
  - `warnings`: L=[null] · X=[["tls_insecure"]]
- `tuic/unknown_congestion_dropped`
  - `warnings`: L=[["tuic_congestion_invalid"]] · X=[null]
- `tuic/v5_cubic`
  - `warnings`: L=[null] · X=[["tls_insecure"]]
- `vless/ech_ignored_reality_kept`
  - `warnings`: L=[null] · X=[["ech_ignored"]]
- `vless/flow_vision_ws_suppressed`
  - `warnings`: L=[null] · X=[["vision_with_transport"]]
- `vless/flow_vision_xhttp_suppressed`
  - `warnings`: L=[null] · X=[["vision_with_transport"]]
- `vless/packet_encoding_garbage_dropped`
  - `warnings`: L=[["packet_encoding_unknown"]] · X=[null]
- `vless/reality_sid_nbsp_sanitized`
  - `warnings`: L=[["reality_short_id_invalid"]] · X=[null]
- `vless/reality_sid_nonhex_cleaned`
  - `warnings`: L=[["reality_short_id_invalid"]] · X=[null]
- `vless/reality_sid_odd_dropped`
  - `warnings`: L=[["reality_short_id_invalid"]] · X=[null]
- `vless/reality_sid_too_long_dropped`
  - `warnings`: L=[["reality_short_id_invalid"]] · X=[null]
- `vless/reality_vision_full`
  - `warnings`: L=[null] · X=[["tls_insecure"]]
- `vless/ws_early_data_ed_in_path`
  - `warnings`: L=[["ws_early_data_converted"]] · X=[null]
- `vmess/fp_junk_in_json`
  - `warnings`: L=[null] · X=[["utls_fp_unknown"]]
- `vmess/json_ws_early_data_ed`
  - `warnings`: L=[["ws_early_data_converted"]] · X=[null]
- `wireguard/awg_header_ranges`
  - `warnings`: L=[["awg_header_invalid"]] · X=[null]
- `wireguard/awg_ranged_h_broken_dropped`
  - `warnings`: L=[["awg_header_invalid"]] · X=[null]

---

## Сверка класса WARNINGS с `registry/warnings.json`

| код | кейсов | `dart` в реестре | что говорит `desc` |
|---|---|---|---|
| `ws_early_data_converted` | 5 | null | Xray-хвост `?ed=N` в WebSocket-пути разложен на sing-box-поля max_early_data + early_data_header_name — путь и |
| `naive_padding_ignored` | 4 | null | URI-параметр padding у naive не имеет sing-box-эквивалента — игнорируется, узел живёт. |
| `reality_short_id_invalid` | 4 | null | REALITY short_id содержит не-hex или слишком длинный — фильтруется до hex-цифр/усекается, мусор целиком → поле |
| `tuic_congestion_invalid` | 2 | null | TUIC congestion_control вне {cubic, new_reno, bbr} — поле снимается, узел живёт. |
| `awg_header_invalid` | 2 | null | AWG magic-header (H1..H4/S1/S2 и др.) не uint32 и не lo-hi диапазон — поле снимается, ядро возьмёт WireGuard-д |
| `masque_vhttp_invalid` | 1 | null | MASQUE vhttp вне {h3, h2} — принудительно h3. Desktop-протокол (masque в LxBox нет). ⚠️ desc утверждает отсутствие |
| `anytls_min_idle_invalid` | 1 | null | anytls min_idle_session не неотрицательное целое — поле снимается, узел живёт. |
| `packet_encoding_unknown` | 1 | null | packet_encoding вне {xudp, packetaddr} (пустое/none = отсутствие поля) — поле снимается, иначе паника ядра (SP ⚠️ desc утверждает отсутствие |

## Протухшие записи реестра (найдено при сверке)

Записи с `"dart": null`, чей `desc` утверждает отсутствие протокола/класса на мобиле —
но корпус показывает, что LxBox эти кейсы прогоняет и узлы парсит:

- `masque_vhttp_invalid` — desc: «MASQUE vhttp вне {h3, h2} — принудительно h3. Desktop-протокол (masque в LxBox нет).»
- `ssh_user_default` — desc: «SSH-ссылка без пользователя — подставлен root. Desktop-протокол.»

  Факт: у `masque` LxBox сегодня имеет полноценный MasqueSpec (весь снос легаси 0.8.0 делался с их стороны);
  у `ssh` в корпусе 11 lxbox-override'ов, причём `ssh/full_config` показывает распарсенный узел с `user: "root"`.
  То есть формулировка «Desktop-протокол (в LxBox нет)» устарела в обоих случаях.

  Отдельно проверено и НЕ подтвердилось: `ssh/missing_user_rejected` выглядел расхождением вердикта,
  но обе стороны роняют узел одинаково (`parse_error`). Вся разница — `"nodes": null` (Go: nil-срез)
  против `"nodes": []` (Dart: пустой список). Это не различие поведения, а различие СЕРИАЛИЗАЦИИ
  пустого списка — и таких кейсов в классе ПОЛЯ может быть больше.

## Кандидат в отдельный класс: `nodes: null` vs `nodes: []`

Go отдаёт nil-срез как `null`, Dart — как `[]`. Сравнение в раннерах идёт по значению после
канонизации (CANON §7), но пустой список и `null` — разные значения, поэтому каждый reject-кейс
обязан получить override, хотя поведение сторон идентично. Это чистый шум, который стоит убрать
нормализацией в раннерах (пустой/nil → `[]`), а не override-файлами.

Посчитано: **24 из 81** override-файлов различаются ИСКЛЮЧИТЕЛЬНО этим — после нормализации
`null → []` их expected совпадают побайтово. То есть реальных различий поведения не 81, а **57**.

---

## Итог: 279 override-файлов раскладываются так

| группа | шт | что это |
|---|---|---|
| побайтовые копии | 198 | паритет полный, override не нужен вовсе |
| `null` vs `[]` | 24 | артефакт сериализации пустого списка, поведение идентично |
| WARNINGS | ~36 | коды деградации не доезжают до Dart (класс N9/N10) |
| СХЕМА | 14 | `ss`/`shadowsocks`, `socks5`/`socks` — вероятно by-design, но нигде не записано |
| ПОЛЯ | ~7 | требуют разбора глазами |

Из 279 файлов **222 (80%) не несут информации** о различиях сторон.

## Что предлагается (порядок из договорённости)

1. **LxBox** проходит классы WARNINGS / СХЕМА / ПОЛЯ и классифицирует: (а) настоящий разрыв Dart →
   чинит у себя; (б) by-design → пишем в спеку/реестр со ссылкой; (в) протухшая запись реестра → правим реестр.
2. **Launcher** после п.1 сносит копии — но только там, где паритет подтверждён прогоном обеих сторон,
   а не совпадением байтов.
3. Отдельно и независимо: нормализация `null → []` в обоих раннерах убирает 24 override'а без всякого
   разбора — различия там нет по построению.
4. Правило `corpus/README` про «бесхозный override — ошибка линтера» либо начинает исполняться линтером,
   либо переформулируется. Сейчас 198 нарушений живут при наличии правила.
