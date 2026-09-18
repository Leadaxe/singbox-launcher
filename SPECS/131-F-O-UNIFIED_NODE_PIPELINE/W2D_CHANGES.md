# W2D — парсеры стали мапперами: что изменилось

SPEC 131, волна W2d. Задача: парсер схемы переводит диалект ссылки в карту
sing-box по `maps_to` и **не принимает решений о значениях** — решения
принимает санитайзер по реестру контракта.

Ниже — всё, что изменилось наблюдаемо: коды узлов, тела, правила реестра.
Таблицы §3 и §4 предназначены для переноса в `TASKS_LXBOX.md`.

---

## 1. Итог в цифрах

| | |
|---|---|
| Правил значения **удалено** из парсеров (дублировали реестр) | 21 |
| Правил **перенесено в реестр** (в реестре не было, ядро требует) | 6 |
| Правил **оставлено структурными** (перевод диалекта) | 14 |
| Констант `Warn*` снято как мёртвые | 13 |
| Кейсов корпуса с изменённым `warnings[]` | 24 |
| Кейсов корпуса с изменённым **телом** | 2 (из 281 эмитируемого) |
| Per-app override'ов `.expected.lxbox.json` снято | 3 (24 → 21) |

Парсеры схем после выноса: anytls 74 строки (было 117), tuic 81 (140),
hysteria2 76 (134), transport 1125 (1225).

**Критерий §9.2 выполнен.** Пара `uri/vless/junk_pair_with_body` ↔
`body/singbox/vless_junk_pair` даёт теперь одинаковые тела **и одинаковые
коды с одинаковыми путями**; комментарий «после W2d обе половины обязаны дать
один набор» в фикстурах исполнен.

---

## 2. Что осталось в парсере и почему

Это не «недоделано» — это и есть работа маппера (SPEC §3.1). Санитайзер
видит готовое тело и таких решений принять не может в принципе.

| Правило | Почему структурное |
|---|---|
| `security=none` → ключа `tls` нет вовсе | Отсутствие ключа санитайзеру неотличимо от «не задано»; `enabled:false` роняет ядра lx.5–lx.18 в SIGSEGV (SPEC 045) |
| Порты открытого HTTP (80/8080/…) у vless без `security` → без TLS | Тот же вопрос «есть ли блок»; `policy.plaintext_ports` реестра |
| `pbk=` присутствует → в теле появляется блок `reality` | Наличие блока, а не годность ключа (её судит реестр) |
| `xtls-rprx-vision-udp443` → `flow` + `packet_encoding` | Составное ИМЯ, а не значение поля; санитайзер увидел бы мусор вне enum |
| `packetEncoding=none` → ключа нет | Синоним отсутствия в диалекте подписок; ядро значения `none` не знает |
| `HelloChrome_120` → `chrome` | Написание значения в чужом диалекте (Xray-идентификаторы uTLS) |
| `?ed=N` в пути ws → `max_early_data` + `early_data_header_name` | Один параметр ссылки → два поля тела |
| `headerType=http`, `net=h2` → `transport.type` | Имя транспорта в диалекте Xray/vmess |
| `alpn` строка через запятую → список | Форма записи |
| `mport=1000-2000` → `server_ports: ["1000:2000"]` | Форма записи диапазона |
| `upmbps=100 mbps` → `100` | Суффикс единицы: форма записи, не значение |
| `heartbeat=10` → `"10s"` | Голое число → duration-строка |
| SNI-эвристика (`🔒`, значение без `.` и `:`) → адрес сервера | Выбор ИСТОЧНИКА поля; решение DRIFT §7.5 (вариант А), теперь одна функция вместо четырёх копий |
| `ech=` → снимается с `ech_ignored` | Единственный параметр без адреса в теле; различить его и валидный `tls.ech{}` из JSON может только тот, кто знает ИСТОЧНИК (DRIFT §7.2) |

Дефолты, которые парсер продолжает материализовать, и почему:

| Дефолт | Обоснование |
|---|---|
| `fp` пустой у vless/anytls → `random` | Конвенция ОБЕИХ сторон (D-009), не дефолт ядра (у него пустой = chrome). Входит в identity-хеш живых узлов. В реестре `default_when` не заводился: см. §6 |
| `vhttp` пустой у masque → `h3` | То же: конвенция обеих сторон, дефолт ядра — `auto`. Снятие меняет identity всех MASQUE-узлов |

---

## 3. Изменения кодов на кейсах корпуса

**Механические** (тот же код, добавились `path` и `value` — правило переехало
в реестр, и санитайзер знает адрес поля):

| Кейс | Код | Добавлено |
|---|---|---|
| `uri/anytls/min_idle_session_invalid` | `anytls_min_idle_invalid` | `path: min_idle_session`, `value: abc` |
| `uri/hysteria2/obfs_no_password_dropped` | `obfs_password_missing` | `path: obfs.password` |
| `uri/hysteria2/obfs_unknown_dropped` | `obfs_unknown` | `path: obfs.type`, `value: xyz` |
| `uri/tuic/congestion_bogus_default_cubic` | `tuic_congestion_invalid` | `path: congestion_control`, `value: bogus` |
| `uri/tuic/unknown_congestion_dropped` | `tuic_congestion_invalid` | `path`, `value: reno-xyz` |
| `uri/vless/allowinsecure_lowercase_zero` | `reality_fp_not_chrome` | `path: tls.utls.fingerprint`, `value: qq` |
| `uri/vless/fp_garbage_fallback_warning` | `utls_fp_unknown` | `path: tls.utls.fingerprint`, `value: garbage` |
| `uri/vless/utls_junk_fp_fallback` | `utls_fp_unknown` | `path`, `value: enabled` |
| `uri/vless/packet_encoding_garbage_dropped` | `packet_encoding_unknown` | `path: packet_encoding`, `value: somethingweird` |
| `uri/vless/reality_key_share_bad_dropped` | `reality_key_share_invalid` | `path: tls.reality.key_share`, `value: quantum` |
| `uri/vless/reality_sid_nbsp_sanitized` | `reality_short_id_invalid` | `path: tls.reality.short_id`, `value: "48 ab12"` |
| `uri/vless/reality_sid_nonhex_cleaned` | `reality_short_id_invalid` | `path`, `value: 0x1a2` |
| `uri/vless/reality_sid_odd_dropped` | `reality_short_id_invalid` | `path`, `value: abc` |
| `uri/vless/reality_sid_too_long_dropped` | `reality_short_id_invalid` | `path`, `value: 0123456789abcdef0` |
| `uri/vless/junk_pair_with_body` | все три | `path`+`value` у каждого; ПОРЯДОК стал `body.order` — совпал с JSON-половиной (критерий §9.2) |

**По смыслу** (код появился или изменилось поведение):

| Кейс | Было | Стало | Почему |
|---|---|---|---|
| `uri/vless/reality_pbk_junk_degrade` | кодов нет | `reality_pbk_invalid` @ `tls.reality.public_key` = `true` | DRIFT §2(b2): «код на всех путях в обоих». Прежде URI-путь снимал REALITY **молча** (только debuglog), а импорт — с кодом |
| `uri/vless/reality_pbk_junk_on_tls` | кодов нет | `reality_pbk_invalid` = `enabled` | то же |
| `uri/vless/tls_pbk_junk_enabled` | кодов нет | `reality_pbk_invalid` = `enabled` | то же |
| `uri/vless/reality_key_share_without_pbk_ignored` | кодов нет | `reality_pbk_invalid` = `enabled` | то же |
| `uri/vmess/fp_junk_in_json` | кодов нет | `utls_fp_unknown` @ `tls.utls.fingerprint` = `wat` | vmess-ветка мусорный отпечаток теряла молча; теперь правило одно на все схемы |
| `uri/vless/ech_ignored_reality_kept` | кодов нет (был override) | `ech_ignored` | DRIFT §2(d)/§7.2: Go не читал `ech=` вовсе. **Override снят** |
| `uri/trojan/ech_bare_name_ignored` | кодов нет (был override) | `ech_ignored` | то же. **Override снят** |
| `uri/trojan/ech_name_resolver_ignored` | кодов нет (был override) | `ech_ignored` | то же. **Override снят** |
| `uri/hysteria2/salamander_ignores_gecko_sizes` | кодов нет | `field_requires` @ `obfs.min_packet_size` | Поле gecko на salamander прежде снимал парсер молча; правило выражено в реестре (`requires` + `equals`) |

**Изменения ТЕЛА** (их всего два):

| Кейс | Было | Стало | Почему |
|---|---|---|---|
| `uri/hysteria2/up_down_mbps` | полосы в теле НЕТ | `up_mbps: 100`, `down_mbps: 200` | Ссылка несёт `up_mbps=`/`down_mbps=`, а парсер читал только `upmbps`/`downmbps`. Написания теперь из реестра (DRIFT §2(l): «оба имени на входе в обоих») |
| `uri/naive/password_only_userinfo` | `username: onlypass` | `password: onlypass` | DRIFT §7.3, вариант А, синхронно с LxBox 24.09.2026. Эмиттер Go и так писал пароль в user-слот — ссылка читалась «наоборот» |

---

## 4. Правила, добавленные в реестр

Шесть правил, которых в реестре не было, а ядро их требует. Все — в
`body.*`, все отражены в `contract/docs/generated`.

| Правило | Где | Что закрывает |
|---|---|---|
| `format: base64_32` вместо `base64` у `tls.reality.public_key`, `wireguard.private_key`/`peers[].public_key`/`pre_shared_key`/`header_protection_key` | `tls.json`, `wireguard.json` | `base64` проверял только декодируемость: `enabled` — валидный base64 на 5 байт, `true` — на 3. Ядро отвечает «invalid public_key» ФАТАЛОМ НА ВЕСЬ конфиг. Теперь 32 байта ПОСЛЕ декода (DRIFT §2(b2), целевое = вариант Dart). У masque ключи остались `base64`: там DER EC P-256, не 32 байта |
| `normalize: hex_only` + `normalize_code` у `tls.reality.short_id` | `tls.json` | Чистка не-hex рун (моджибейк U+00C2, пробелы) жила в парсере. `0x1a2` → `01a2` — это ДРУГОЙ short_id, и код о потере обязан ставиться на всех путях (DRIFT §2(b)) |
| `advisory` с `except` + `when` у `tls.utls.fingerprint` | `tls.json` | `reality_fp_not_chrome` (D-119) ставился ЧЕТЫРЬМЯ копиями в парсерах (vless, anytls, Xray, sing-box-импорт); список гибридных отпечатков тоже лежал в коде |
| `code: obfs_password_missing` у `hysteria2.obfs.password` | `hysteria2.json` | `required` без `code` давал общий `field_missing` вместо объявленного кода |
| `requires` с `equals` у `obfs.min_packet_size`/`max_packet_size` | `hysteria2.json` | «Только для gecko» не было выражено ничем; парсер снимал поле молча |
| `default_when` у `hysteria.up_mbps`/`down_mbps` | `hysteria.json` | Дефолт 100 Мбит/с лежал ТРЕМЯ копиями (URI-парсер, санитайзер импорта, Xray-конвертер). Без него ядро не поднимает outbound v1 («missing upload speed») — фатал на весь конфиг |

Новые возможности санитайзера, потребовавшиеся для этих правил (все — данные
реестра, не схемные ветки в коде): `Advisory.except`/`Advisory.when`,
`Relation.equals`, `Field.normalize_code`, `Field.default_when`,
`format: base64_32`, `normalize: hex_only`. Линтер реестра
(`registry_body_test.go`) и генератор документации знают о каждой.

---

## 5. Снятые копии правил

| Что снято | Сколько копий было |
|---|---|
| Набор написаний `insecure` | 6 (база, TUIC, AnyTLS, Hysteria2, Hysteria, MASQUE) + Xray читал только JSON-bool |
| Список отпечатков с гибридным шаром + код `reality_fp_not_chrome` | 4 |
| Дефолт полосы hysteria v1 = 100 | 3 |
| Allowlist методов shadowsocks | 3 (URI-парсер, Xray-конвертер, эмиттер share-URI) |
| Гейт `isValidRealityPublicKey` | 3 (vless, anytls, Xray) |
| `normalizeRealityShortID` | 3 |
| Allowlist `tuic congestion` / `udp_relay_mode` | 1 + сторожевой тест |
| Allowlist `hysteria2 obfs` | 2 (парсер + санитайзер импорта) |
| Валидация `masque vhttp` | 1 |

Снятые функции: `isValidHysteria2ObfsType`, `isValidTuicCongestionControl`,
`tuicQueryFlagTrue`, `normalizeRealityShortID`, `isValidRealityPublicKey`,
`utlsFingerprintOrFallback`, `IsChromeFamilyFingerprint`,
`realityFingerprintRisky`, `noteRealityFingerprint`,
`hysteriaBandwidthOrDefault`, `realityShortIDWouldDegrade`,
`utlsFingerprintWouldDegrade`, `maxRealityShortIDHexLen`,
`chromeFamilyUTLSFingerprints`, `realityHybridUTLSFingerprints`.
`isValidShadowsocksMethod` остался, но читает список ИЗ РЕЕСТРА и зовётся
одним местом — эмиттером share-URI.

Мёртвые константы `Warn*` сняты (12 переехали в реестр + `WarnMasqueVHTTPInvalid`).
Среди них `WarnSSMethodInvalid` и `WarnPortInvalid`, которые не выставлялись
НИКОГДА (DRIFT §4): реальным поведением был жёсткий дроп узла. Теперь оба
кода реально ставятся — санитайзером, из реестра.

---

## 6. Решения, принятые по ходу

**`fp` пустой у vless → `random` остаётся правилом ПАРСЕРА, а не реестра.**
Проверено: `tls.json uri.query.fp.impl` (D-009/D-119) называет `random`
дефолтом обеих сторон, но это дефолт КОНВЕНЦИИ, а не ядра — у ядра пустой
`fp` означает `chrome`. `default_when` в реестре означает «ядро без этого
поля не работает» (случай hysteria-полосы); распространять его на конвенцию
значило бы смешать два разных основания. Значение входит в identity-хеш живых
узлов, поэтому менять его в эту волну нельзя в любом случае. То же и по той
же причине — `vhttp=h3` у masque.

**Порт вне 1..65535 продолжает дропать узел в парсере.** Реестр умеет это
сам (`dialer.common.server_port`, код `port_invalid` — проверено, ставится),
но URI без валидного порта не образует адреса вовсе, и убирать проверку из
`ParseNode` — отдельное решение о форме отбраковки (`dropped[]` против узла
с кодом), не предмет W2d.

**ss legacy-шифры приняты (DRIFT §7.10, вариант А).** Прежде оба клиента
дропали узел на девяти рабочих шифрах ядра. Теперь: вне 18 значений ядра —
дроп с `ss_method_invalid`, legacy — живой узел с info-кодом
`ss_method_legacy`. Это меняет поведение на живых подписках, а не только
коды: узлы, которых раньше не было, появятся.

---

## 7. Тесты

- Новый интеграционный `TestPipelineAllInputsAgree` (SPEC §10): один
  мусорный узел ссылкой, телом sing-box и Xray-JSON → равные тела и равные
  коды. Он и нашёл расхождение: Xray-ветка молча превращала мусорный
  отпечаток в `random`, тогда как ссылка отдавала его санитайзеру.
- `TestPipelineSetsDegradationCodes` (10 кодов) и
  `TestPipelineCleanNodeStaysClean` — переехали из `subscription` на выход
  конвейера вместе с правилами; каждый код проверяется ВМЕСТЕ с путём.
- `TestPipelineHysteriaV1BandwidthDefault` — гарантия полосы v1 на каждом
  входе (была в `subscription`, проверяла три копии дефолта).
- Сторожевой `registry_sync_test.go` (Л12): грепа `parse_warnings.go`
  оказалось мало — после выноса он объявил бы дюжину живых правил мёртвыми.
  Добавлен встречный `TestRegistryWarningCodesHaveAProducer`: у каждого кода
  ПОЛЕЙ УЗЛА обязан быть тот, кто его ставит, — константа Go **или** правило
  секции реестра. Проверено на регрессии (снял `code` у tuic — тест упал).
- `TestContractCorpusEmitRoundTrip` теперь эмитит ссылку из ТЕЛА КОНВЕЙЕРА,
  а не из сырой карты парсера: иначе ссылка несла бы `fingerprint:"enabled"`,
  которого в теле узла нет. 281 кейс round-trip зелёный.
- Удалены/переписаны юниты, проверявшие снятые правила на выходе парсера:
  `TestParseNode_VLESS_RealityShortIDSanitized`,
  `TestParseNode_VLESS_JunkFingerprintDropped`,
  `TestParseNode_VLESS_JunkPbkOnTLSNode`,
  `TestBuildOutbound_Tuic_UnknownCongestionDropped`,
  `TestNormalizeRealityShortID`, `TestIsValidRealityPublicKey`,
  три подтеста `packetEncoding`, `TestNodeWarningsSetOnDegradation`.
  `TestXrayArrayDropsShadowsocksWithBadMethod` → `…KeepsShadowsocksLegacyMethod`
  (его посылка — дроп на `rc4-md5` — была ошибкой, §7.10).

`bin/sing-box` 1.14.0-lx.39 принимает сводный конфиг из всего корпуса
(`TestCorpusBodiesPassSingboxCheck`).

---

## 8. Для LxBox

Что стороне LxBox стоит забрать из этой волны:

1. **`ech_ignored` на `ech=`** — Go догнал; три per-app override'а сняты.
2. **naive одиночный userinfo = password** — правка синхронная, релиз не
   раньше 24.09.2026.
3. **ss legacy-шифры** — прежде обе стороны дропали узел; теперь узел живёт
   с `ss_method_legacy`.
4. **`up_mbps`/`down_mbps` как имена параметров hysteria2** — Go читал только
   `upmbps`/`downmbps`.
5. **32 байта после декода у REALITY `pbk`** — вариант Dart принят целевым.
6. **Порядок `warnings[]` = `body.order`** — не порядок разбора. Коды в
   конверте сравнимы поэлементно.
7. **`reality_pbk_invalid`** — у Dart класса нет вовсе (DRIFT §2(b2):
   «Dart добавляет класс»).
