# SPEC 131 · DRIFT — инвентарь расхождений Go ↔ Dart

Статус: **инвентарь**, собран 17.09.2026. Ветка `develop` от `d0e5aca3`,
контракт `1.0.4`, LxBox `app/contract.lock` снят `2026-09-16T12:08:33Z`
(sha256 `b90ff80d…`), ядро-пин `1.14.1-lx.4`.

Соседи: `SPEC.md` (ТЗ конвейера), `CODEMAP.md` (адреса в лаунчере),
`CORE_SCHEMA.md` + `core_schema.draft.json` (поля тел из `option/*.go`).

Ничего в репозиториях не менялось — это только карта.

---

## 0. Как читать и откуда взяты вердикты

**Правило владельца (17.09.2026):** там, где Go и Dart расходятся, целевое
поведение = **вариант LxBox**, но каждый пункт обоснован тем, что делает
ЯДРО. Где вариант LxBox ломает узел, который ядро приняло бы, — пункт
помечен **⛔ ВОЗРАЖЕНИЕ** и вынесен в §7 на решение владельца.

Колонка «ядро» использует четыре вердикта:

| Код | Что значит | Последствие для пользователя |
|---|---|---|
| **A** | `sing-box check` падает на **декоде** (тип/неизвестный ключ/`enum`-тег) | весь конфиг мёртв, VPN нет |
| **B** | `check` падает на **инициализации outbound'а** | весь конфиг мёртв, VPN нет |
| **C** | значение принято декодером, но **молча игнорируется** или бьёт позже в рантайме | узел «есть», но не работает / работает не так |
| **D** | **работает** | — |

Важно: `sing-box check` зовёт `box.New` (`cmd/sing-box/cmd_check.go:35`),
который создаёт **все** outbound'ы (`box.go:378`). Поэтому A и B одинаково
валят проверку и **различить их запуском `check` нельзя** — разница лишь в
том, что A ловится любым валидатором JSON-схемы, а B требует конструктора.
Оба валят **ВЕСЬ** конфиг, а не один узел, — это и есть причина, по которой
санитайзер обязан снимать поле, а не надеяться на ядро.

Практически опасен именно **C**: конфиг собрался, узел в списке, а поле не
работает. Ниже такие строки помечены отдельно.

**Строгость декодера — корень всего:** `option/options.go:41` включает
`decoder.DisallowUnknownFields()`, а `badjson.UnmarshallExcludedContext`
(`option/outbound.go:51`) протаскивает её во **все** вложенные структуры.

### 0.1 Как получены вердикты ядра

Статика: `option/*.go` (структура + JSON-теги) → потребитель в
`protocol/*/`, `common/tls/`, `transport/*/`.

Динамика: **прогон бинарём** `bin/sing-box` (`1.14.0-lx.33`, теги
`with_gvisor,with_quic,with_wireguard,with_utls,with_naive_outbound,with_xhttp,with_awg,with_lx_command,with_lxd,with_lx_chain,with_tailscale`;
`with_ech` НЕТ) командой `sing-box check -c <минимальный конфиг>`.
Прогон отмечен ✅.

⚠️ **Оговорка о версии.** Доступный бинарь — линия **1.14.0-lx.33**, а
пин ТЗ — **1.14.1-lx.4**. Для полей, появившихся в 1.14.1-lx.\* (прежде
всего `tls.reality.key_share`, SPEC 089 ядра), прогон на lx.33 даёт
`unknown field` — это подтверждает строгость декодера, но НЕ проверяет
enum на пине. Такие строки помечены **«проверить на пине»**. Все
остальные вердикты сверены и статикой, и прогоном.

### 0.2 Общая находка о декодере

✅ Декодер ядра **строгий**: неизвестный ключ в outbound'е — фатал на
декоде.

```
outbounds[0].totally_unknown_key: json: unknown field "totally_unknown_key"
```

Это фундамент для правила `unknown_key` из ТЗ §4: лишний ключ — не
безобидный мусор, а мгновенная смерть конфига. Санитайзер обязан снимать
неизвестные ключи, а не «пропускать вперёд на всякий случай».

Следствие для эмиттера: сегодняшний allowlist-эмиттер Go (`outbound_tls_emit.go`)
защищает от этого **случайно** — он пишет только известные ему ключи. После
перехода на табличный эмиттер защиту обязан давать реестр, иначе любое
новое поле подписки станет фаталом.

---

## 1. Где живут правила — карта

### 1.1 Dart (LxBox), `/Users/macbook/projects/LxBox/app/lib`

Архитектура — **три яруса**: канонизация в парсерах, почти-тупой эмиттер,
«лечащие» пост-шаги сборки для путей мимо парсера.

| Правило | Файл:строка |
|---|---|
| **URI-парсеры** (диспетчер + лимит длины) | `services/parser/uri_parsers.dart:38,43,83-87` |
| vless / trojan / anytls / tuic / ssh / socks / http / naive / ss / vmess / hysteria2 / masque / wireguard | `services/parser/uri_parsers/<name>_parser.dart` |
| **TLS-суб-парсер** (`parseVlessTls`, `parseTrojanTls`, `parseVmessTls`) | `services/parser/transport.dart:443-570` |
| **Транспорт-суб-парсер** (диспетчер ws/grpc/http/h2/httpupgrade/xhttp) | `services/parser/transport.dart:18-136` |
| xhttp-поля + merge `extra` | `services/parser/transport.dart:212-272,347-394` |
| **JSON-парсеры** (sing-box + Xray) | `services/parser/json_parsers.dart` |
| — Xray `streamSettings` → транспорт | `json_parsers.dart:743-792` |
| — sing-box `tls{}` | `json_parsers.dart:1444-1533` |
| — naive TLS allowlist | `json_parsers.dart:1429-1436` |
| **Хелперы-канонизаторы** | `services/parser/uri_utils.dart` |
| — `isTlsInsecure` | `uri_utils.dart:221-233` |
| — `normalizeRealityShortId` / `realityShortIdWouldDegrade` | `uri_utils.dart:411-424 / 432-435` |
| — `isValidRealityPublicKey` | `uri_utils.dart:394-399` |
| — `normalizePacketEncoding` | `uri_utils.dart:254-267` |
| — `shadowsocksMethods` / `isValidShadowsocksMethod` | `uri_utils.dart:470-483` |
| — `normalizeVmessSecurity` | `uri_utils.dart:451-467` |
| — `normalizeSingboxDuration` | `uri_utils.dart:444-448` |
| — `isValidNaiveHeaderName` | `uri_utils.dart:30-36` |
| **uTLS** (allowlist, алиасы, reality-гейт) | `services/parser/utls_fingerprint.dart:21-37,62-73,83-93,98-133` |
| **hysteria2 obfs** | `services/parser/hysteria2_obfs.dart:23,31-47` |
| **Модель узла**, поле `warnings` | `models/node_spec.dart:55,111-112` |
| **Эмиттер** (почти тупой, см. §1.3) | `models/node_spec_emit.dart` |
| — гейт `flow` (универсальный, все пути) | `node_spec_emit.dart:84-86` |
| **TLS-спек**: passthrough-ключи, QUIC-срез, key_share | `models/tls_spec.dart:13-30,42-48,52,130-136,150-155,225,246` |
| **Транспорт-спек**: эмит + enum-гейты xhttp + §416-гард | `models/transport_spec.dart:55-68,243-252,262-267,284-300` |
| **Классы warning'ов** (37 шт.) | `models/node_warning.dart:14-50` (база), далее по классу |
| **Пост-шаги сборки** («страховка мимо парсера») | `services/builder/post_steps/` |
| — `heal_unknown_utls_fingerprints.dart:29-88`, `heal_invalid_reality.dart:21-67`, `tls_transforms.dart:57-92` | |

### 1.2 Go (лаунчер), `/Users/macbook/projects/singbox-launcher`

Архитектура — **три независимых яруса канонизации**, правила дублируются.

| Ярус | Файл | Вход |
|---|---|---|
| 1. URI-парсеры | `core/config/subscription/node_parser_*.go`, `shareuri_*.go` | query share-URI |
| 2. Санитайзер | `core/config/subscription/singbox_sanitize.go:63-82` | готовая карта outbound'а (sing-box-импорт + Xray-конверт) |
| 3. Эмиттер | `core/config/outbound_generator.go`, `outbound_tls_emit.go`, `outbound_jsonbuilder.go` | `node.Outbound` → текст JSON |

Санитайзер зовут ровно три места: `singbox_import.go:355`,
`xray_protocols.go:338`, `xray_hysteria.go:115`. **URI-путь через санитайзер
НЕ проходит** — отсюда большая часть внутренних несимметрий Go.

| Правило | Файл:строка |
|---|---|
| `tlsInsecureTrue` (базовый набор) | `node_parser_transport.go:37-45` |
| `queryGetFold` (регистронезависимый поиск ключа) | `node_parser_transport.go:16-23` |
| `singboxUTLSFingerprints` | `node_parser_transport.go:47-58` |
| `utlsAliasPrefixes` | `node_parser_transport.go:63-74` |
| `utlsJunkFallback = "chrome"` | `node_parser_transport.go:126` |
| `chromeFamilyUTLSFingerprints` / `realityHybridUTLSFingerprints` | `node_parser_transport.go:1018-1021 / 1098-1100` |
| `normalizeRealityShortID` (+ `maxRealityShortIDHexLen = 16`) | `node_parser_transport.go:769,782-803` |
| `isValidRealityPublicKey` | `node_parser_transport.go:814-823` |
| `NormalizeRealityKeyShare` | `node_parser_transport.go:825-849` |
| `splitWSEarlyData` / `applyWSEarlyData` / `decodeResidualPercent` | `node_parser_transport.go:675-698 / 707-719 / 722-742` |
| `xhttpGuardUplinkPlacement` | `node_parser_transport.go:418-465` |
| `isValidShadowsocksMethod` | `node_parser_ss.go:6-21` |
| `normalizeVMessSecurity` (URI) / `normalizeVMessSecurityValue` (Xray) | `node_parser_vmess.go:18-31` / `xray_protocols.go:368-377` |
| `isValidHysteria2ObfsType` | `node_parser_hysteria2.go:15-17` |
| `isValidTuicCongestionControl` | `node_parser_tuic.go:14-16` |
| `packet_encoding` (URI / санитайзер) | `node_parser_core.go:689-704` / `singbox_sanitize.go:271-290` |
| `flow` (санитайзер / эмиттер) | `singbox_sanitize.go:244-266` / `outbound_generator.go:664-671` |
| naive: TLS-ключи, extra-headers charset | `outbound_tls_emit.go:33-35` / `node_parser_naive.go:45` |
| never-emit список (`ech` и пр.) | `outbound_tls_emit.go:11-13` |
| коды warning'ов (30 шт.) | `core/config/subscription/parse_warnings.go:22-129` |
| `ParsedNode.Warnings` + `AddWarning` | `core/config/configtypes/types.go:765,769-780` |
| emission-warnings (СВОБОДНЫЙ ТЕКСТ, без кодов) | `core/config/emission_warning.go:32-51` |

### 1.3 Тупой ли эмиттер — ответ по обеим сторонам

**Dart — «почти тупой», по делу:** основная канонизация в парсере, но
эмиттер осознанно держит три вещи:
- универсальный гейт `flow` (`node_spec_emit.dart:84-86`) — единственная
  точка фильтрации flow для sing-box-JSON-пути;
- перефильтр имён naive-заголовков (`node_spec_emit.dart:498`);
- QUIC-срез `utls`/`reality` (`node_spec_emit.dart:400,544` →
  `tls_spec.dart:150-155`).

Настоящая emit-валидация живёт ярусом ниже, в
`transport_spec.dart:243-252,284-300` (enum-гейты xhttp + §416-гард), и это
**задокументировано намеренно** (`transport.dart:209-211`): парсер читает
дословно, нормализацию против ядра делает `toSingbox`, потому что там есть
канал `NodeWarning` для ⚠ в подписке.

Эмиттер Dart **дописывает warnings** (`node_spec_emit.dart:72-77,167-172,248-253`) —
это by design (`node_spec.dart:24-25`).

**Go — эмиттер валидирует наравне с парсером**, и это главный источник
дублей. Примеры дублирования одного правила в 3 местах:

| Правило | Парсер | Санитайзер | Эмиттер |
|---|---|---|---|
| `tls.enabled:false` → снять блок (SPEC 045, SIGSEGV lx.5–lx.18) | `node_parser_transport.go:895,982` | `singbox_sanitize.go:130-133` | `outbound_tls_emit.go:47-56` |
| uTLS allowlist | `node_parser_transport.go:94-109` | `singbox_sanitize.go:171` | `outbound_tls_emit.go:139` |
| `key_share` enum | `node_parser_transport.go:924-928` | `singbox_sanitize.go:220-239` | `outbound_tls_emit.go:166-179` |
| `flow` allowlist + vision/transport | — | `singbox_sanitize.go:244-266` | `outbound_generator.go:664-671` |
| REALITY fp-not-chrome | `node_parser_transport.go:913`, `node_parser_anytls.go:103` | `singbox_import.go:377` | `core/build/tls_transforms.go:228` |

И **дыры** — правило есть в парсере/санитайзере, но НЕ в эмиттере:
`packet_encoding`, `pbk`, `short_id`, TUIC-enum'ы, hysteria2 obfs. Поэтому
рукописный или сторонний `node.Outbound`, попавший прямо в
`GenerateNodeJSON`, до сих пор способен произвести конфиг, который ядро
отвергнет целиком. Это ровно то, что ТЗ §3.3 закрывает табличным эмиттером.

---

## 2. Таблица расхождений

Формат: **поле** | **Go** | **Dart** | **ядро** | **целевое** | **обоснование**.
Где Go и Dart совпадают — строка не приводится (кроме случаев, где обе
стороны расходятся с ядром: такие помечены 🔴).

### (a) insecure — алиасы

**Go, набор зависит от парсера** (это само по себе внутренняя дыра):

| Парсер | Алиасы | Истина | Файл:строка |
|---|---|---|---|
| vless/trojan/http-proxy (база) | `insecure`, `allowInsecure`, `allowinsecure` | `1\|true\|yes` | `node_parser_transport.go:37-45` |
| TUIC | +`allow_insecure`, `skip-cert-verify`, `skipCertVerify` | `1\|true\|yes` | `node_parser_tuic.go:120-124` |
| AnyTLS | те же 6 | `1\|true\|yes` | `node_parser_anytls.go:65-70` |
| Hysteria2 / Hysteria v1 | база + `skip-cert-verify` | база `1\|true\|yes`; **skip-ветка только `true\|1`, без `yes`, без lower** | `node_parser_hysteria2.go:108-112`, `node_parser_hysteria.go:140-144` |
| MASQUE | `insecure`, `skip_cert_verify` (подчёркивание!), `allowinsecure` | `1` или `EqualFold true`, **без `yes`**; поиск **регистрозависимый** | `node_parser_masque.go:142-145,203-210` |
| Xray-JSON | только JSON-bool `allowInsecure` (строка `"true"` игнорируется) | — | `xray_outbound_convert.go:241-243,279-281` |

Go ищет ключ регистронезависимо (`queryGetFold`), но разделители не
нормализует: `allow_insecure` базовым набором НЕ ловится.

**Dart, один хелпер на все URI-пути** — `uri_utils.dart:221-233`:
`insecure`, `allowInsecure`, `allowinsecure`, `allow_insecure`,
`skip-cert-verify`; истина `1|true|yes` после `toLowerCase().trim()`.
Поиск **регистрозависимый** по литералам (`Insecure=1` не поймает).
`skipCertVerify` (camelCase) Dart не знает вовсе.

Более узкие читатели Dart: vmess-JSON только `insecure` (`transport.dart:563`),
vmess-legacy только `insecure` (`vmess_parser.dart:159`), Xray-JSON только
bool `allowInsecure` (`json_parsers.dart:918`), sing-box-JSON только bool
`insecure` (`json_parsers.dart:1474`), **naive не читает вовсе**
(`naive_parser.dart:84`).

| Поле | Go | Dart | Ядро | Целевое | Обоснование |
|---|---|---|---|---|---|
| `tls.insecure` — набор имён | 3 базово / 6 у tuic+anytls / 4 у hysteria / 3 у masque (с `skip_cert_verify`) | 5 единым хелпером | ✅ **A** на строку: `tls.insecure:"true"` → `cannot unmarshal string into … insecure of type bool` | **Объединение восьми имён, один хелпер, регистр и разделители нормализуются**: `insecure`, `allowinsecure`, `allow_insecure`, `allow-insecure`, `skipcertverify`, `skip_cert_verify`, `skip-cert-verify`, `noverify` — сверка по `strings.ToLower` + снятие `-`/`_` | Форма LxBox (один хелпер) принимается; **набор берём шире обоих**, потому что ни один вариант не полон: у Go шире у tuic/anytls, у Dart шире базово, а masque-вариант `skip_cert_verify` не знает никто, кроме Go. Реестр (`tls.json /tls/params/insecure`) уже назначил «целевой набор — объединение». Расширение безопасно: значение всё равно уезжает в **bool**, а лишний URI-параметр ядру не виден |
| `tls.insecure` — истина | `1\|true\|yes`, кроме skip-веток hysteria (`true\|1`) и masque (без `yes`) | `1\|true\|yes` везде | — | **`1\|true\|yes` единообразно** | Dart-вариант; Go-исключения — не решение, а недосмотр в двух парсерах |
| эмит в share-URI | `insecure=1` | `allowInsecure=1` (hysteria2 `insecure=1`, tuic `allow_insecure=1`) | — | **Решить на ревью** (см. §7.6) | Обе стороны парсят обе формы, поэтому это вопрос канона round-trip'а, а не работоспособности; ставить решение владельца ради байтовой разницы URI — избыточно, но identity-хеш от этого зависит |

### (b) REALITY short_id

| Случай | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| не-hex руны (моджибейк, пробелы) | вычищаются, `A-F`→`a-f` (`node_parser_transport.go:788-795`) | то же (`uri_utils.dart:411-424`) | — | совпадает |
| **нечётная длина** | проверка на отфильтрованном значении → **сброс в `""`** + `WarnRealityShortIDInvalid` (`:798-800`) | сброс в `""` + `RealityShortIdInvalidWarning` (`uri_utils.dart:423`) | ✅ **B**: `decode short_id: encoding/hex: odd length hex string` | **сброс в `""`** |
| **>16 hex-символов** | сброс в `""` (НЕ обрезка — D-032) | сброс в `""` (`§343`) | ✅ **B** при 17 симв.; ✅ **ПАНИКА** при 18: `panic: index out of range [8] with length 8` | **сброс в `""`** |
| на sing-box-импорте | чистится **молча**, кода нет (`singbox_sanitize.go:210-218`) | тот же класс warning | — | **код ставить на всех путях** |

**Обоснование.** Тут Go и Dart уже сошлись — реестр (`tls.json /tls/reality/sid`)
описывал Go как «обрезает >16 и пропускает нечётную длину», но это
**протухшая запись**: в коде `node_parser_transport.go:798-800` стоит
именно сброс, а константа `maxRealityShortIDHexLen = 16` используется как
граница проверки, а не обрезки. Реестр в этой части надо править.

Почему сброс, а не подгонка: пустой `short_id` для ядра легален (✅ прогон),
а обрезанный — это **валидная форма с ЧУЖИМ идентификатором**; сервер
такой узел не узнает, и пользователь получит «узел есть, но не
подключается» вместо честного ⚠.

#### 🔴 Баг ядра: >16 hex-символов роняют процесс

`common/tls/reality_client.go:79-86`:
```go
var shortID [8]byte
decodedLen, err := hex.Decode(shortID[:], []byte(options.Reality.ShortID))
if err != nil { … }
if decodedLen > 8 { return nil, E.New("invalid short_id") }   // ← недостижимо
```
`hex.Decode` пишет в приёмник по ходу разбора, поэтому на 18 символах он
индексирует `shortID[8]` и **паникует до возврата**. Проверка длины на
строке 84 — мёртвый код.

✅ Прогон:
```
panic: runtime error: index out of range [8] with length 8
encoding/hex.Decode(…) hex.go:101
…/common/tls.newRealityClient(…) reality_client.go:80
```

Это достижимо из любого файла конфига: `sing-box check` падает стектрейсом
вместо диагностики. Значит гард на стороне клиента — не «строгость ради
чистоты», а **единственная защита от краша ядра**. Аргумент за сброс
сильнее, чем казалось.

→ Отдельным пунктом: **завести issue в форк ядра** (переставить проверку
длины перед `hex.Decode`). Это правка на три строки и она полезна всем
пользователям форка, независимо от SPEC 131.

⚠️ Реестровая заметка про Go — устарела, поправить вместе с §4.

### (b2) REALITY public_key (pbk)

| Аспект | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| валидация | `TrimRight("=")`, длина **ровно 43**, затем `RawURL` или `RawStd` (`:814-823`) | `decodeBase64Safe` (4 варианта) → **длина 32 байта** (`uri_utils.dart:394-399`) | ✅ **B**: `pbk="enabled"` → `initialize outbound[0]: invalid public_key` | **вариант Dart (32 байта после декода)** |
| гейт | REALITY строится по **валидности pbk**, не по `security=reality` | то же | — | совпадает |
| код | URI-путь — **молча** (только debuglog); import-пути — с кодом | класса нет вовсе | — | **`reality_pbk_invalid` на всех путях в обоих** |

**Обоснование.** Практический паритет на реальных ключах (43 символа
base64url ⇔ 32 байта), но вариант Dart строже формально корректен: он
проверяет то, что проверит ядро (длину дешифрованного ключа), а не длину
строки. Go-вариант отвергнет валидный ключ, записанный std-base64 **с**
padding'ом (`44` символа до `TrimRight` — ок, но `TrimRight` это чинит) —
на практике эквивалентно, поэтому переход на Dart-форму безопасен и
убирает магическое число 43.

Код `reality_pbk_invalid` в реестре есть, `dart: None` — **Dart добавляет класс**.

### (b3) REALITY — прочие инварианты ядра, которые не проверяет НИКТО 🔴

| Инвариант | Ядро | Go | Dart |
|---|---|---|---|
| REALITY **требует** блок `utls` | ✅ **B**: `uTLS is required by reality client` | закрыто на сборке (`EnforceRealityFingerprint`, `core/build/tls_transforms.go:228`) | закрыто пост-шагом (`heal_unknown_utls_fingerprints.dart:47-58`) + `utls_fingerprint.dart:120` |
| REALITY на naive | ✅ **B**: `reality is not supported on naive outbound` | naive-allowlist эмиттера случайно закрывает | naive-allowlist `tls_spec.dart:52` закрывает |

Обе стороны закрывают это **пост-фактум, на сборке**, а не в санитайзере.
В новом конвейере это правило реестра (`requires` + `forbidden_for`), и
пост-шаги можно снять.

### (c) uTLS fingerprint

Канонический список **байт-идентичен** (15 значений, `allowlists.json`
`utls_fingerprints`), алиасы по префиксу `hello*` — тоже, **кроме одного**:

| Аспект | Go | Dart | Ядро | Целевое | Обоснование |
|---|---|---|---|---|---|
| префикс `hellorandom` (без `ized`) | есть → `random` (`:73`) | **нет** — уходит в junk → `chrome` | — | **добавить в Dart** | ⛔ здесь целевое = **вариант Go**, см. §7.1. `HelloRandom` — реальная Xray-запись; у Dart она молча становится `chrome`, то есть подписка просит случайный отпечаток, а получает хромовый. Это не «строже», а **неверно** |
| **мусор вне списка** | → `""` … но дальше **`utlsJunkFallback = "chrome"`** (`:126`) на URI-пути; в санитайзере → `chrome`; **в эмиттере → `""` и ключ не пишется** (`outbound_tls_emit.go:139`) | → **`chrome`** + `UnknownFingerprintWarning` (`utls_fingerprint.dart:105,121`) | ✅ **B**: `HelloChrome_120` как есть → `unknown uTLS fingerprint: HelloChrome_120` | **`chrome` + код `utls_fp_unknown`** (вариант Dart) | Мусор ядро не прощает — **B**, весь конфиг. Значит выбор только между «`chrome`» и «снять блок». Снять блок нельзя под REALITY (ядро: `uTLS is required by reality client` — **B**), а вне REALITY «снять» означает молча сменить ClientHello на голый Go-TLS, что отличимо на DPI. `chrome` — рабочий и предсказуемый. Реестр (`allowlists.json`) уже помечал это «выровнять на ревью» — выравниваем на Dart |
| пустой `fp` | vless/anytls/Xray-reality → дефолт `random` (`:892-894`, `node_parser_anytls.go:72-77`) | то же (`transport.dart:474`) | ✅ **D**: `utls{enabled:true}` без fingerprint проходит | совпадает, дефолт `random` (D-009) | — |
| пустой/`random` **при REALITY** | `EnforceRealityFingerprint` на сборке: `""`→`chrome`, **`random`→`chrome`** (`:1060-1090`) | пост-шаг `heal_unknown_utls_fingerprints.dart:80-85`: `null`/`""`/`random`→`chrome` | ✅ **D** — `random` на reality проходит check | совпадает | оба правы: `random` на REALITY технически валиден для ядра, но спека без X25519MLKEM768 уводит на камуфляж-сайт |
| **`fp` на QUIC** (hysteria2/tuic/masque) | hysteria2/hysteria/tuic/masque URI **не читают** `fp`… но hysteria2 **эмитит** `tls.utls` (`node_parser_hysteria2.go:91-100`), а Xray-путь срезает (`xray_protocols.go:276`) | не читает и **срезает** на эмите (`tls_spec.dart:150-155`) | ✅ **D** (ядро принимает utls на QUIC, просто не применяет — **C** по смыслу) | **срезать (вариант Dart)** | Внутренняя несимметрия Go (читает в одном месте, срезает в другом) ломает identity-хеш между путями. Ядро utls на QUIC не использует → поле бессмысленно, а его наличие меняет хеш узла |

### (c2) REALITY fp-not-chrome (D-119)

Полный паритет: обе стороны **не подменяют** значение, только ставят код
`reality_fp_not_chrome`. Наборы совпадают: chrome-семейство (6) +
`firefox`, `safari` (lx.2/lx.3) — исключены; `edge`/`ios`/`android`/`360`/`qq`/`randomized`
— под код; `random` не под кодом.

Go `node_parser_transport.go:1018-1021,1098-1100,1112-1118`;
Dart `utls_fingerprint.dart:62-73,78-79,129-133`.

✅ Прогон: `firefox` на REALITY проходит check (**D**). Статика
подтверждает и уточняет: chrome в этом форке **не требуется** на
конструировании вообще — любой отпечаток строится
(`common/tls/reality_client.go:59-67`). Единственный гейт —
`key_share:"hybrid"` при отпечатке без X25519MLKEM768, и он срабатывает
**в начале хендшейка** (`reality_client.go:225-227`), а не на `check`.

То есть `reality_fp_not_chrome` — предупреждение о поведении **сервера**
(Xray ≥ v26.9.8 уведёт на камуфляж-сайт), а `reality_key_share_invalid` —
о поведении **ядра**. Это разные вещи, и обе стороны разводят их верно.
Менять нечего.

⚠️ Диагностическая ловушка для §8: провал хендшейка при
firefox/safari + `hybrid` даёт `reality verification failed` — **неотличимо
от неверного ключа**. Это стоит упомянуть в тексте кода для пользователя.

### (d) ech

| Аспект | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| URI-параметр `ech=` | **не читает вовсе**; теряется на allowlist-эмиттере **молча** | читает, **выбрасывает**, ставит `EchIgnoredWarning` (берёт часть до `+`) (`transport.dart:443-447,465,530`) | — | **вариант Dart** |
| `tls.ech` из sing-box-JSON | санитайзер **пропускает насквозь** (коммент `singbox_sanitize.go:47-48` врёт); эмиттер не пишет (`outbound_tls_emit.go:11-13`) → снимается молча | `ech` не в `kTlsPassthroughKeys` (`tls_spec.dart:10-11`) → снимается | ✅ **D** на этой сборке: `tls.ech{enabled:true}` **проходит check** | **снимать + код `ech_ignored`** |
| `echfq` | не читает | **намеренно** не читает (`transport.dart:441-442`) | — | совпадает |
| у naive | `naiveTLSKeys` не включает `ech` → снимается с debuglog (`outbound_tls_emit.go:66`) | `kNaiveTlsPassthroughKeys = {certificate, certificate_path}` → снимается | ✅ naive **читает** ECH (`protocol/naive/outbound.go:141-155`) | снимать (см. §7.2) |

#### 🔴 Поправка первой величины: посылка про `with_ech` НЕВЕРНА

Реестр (`tls.json /tls/params/ech`), комментарий Go
(`outbound_tls_emit.go:11-13`) и обоснование Dart исходят из того, что
**ядро собрано без `with_ech`** и потому «пропущенный `ech` уронил бы
конфиг». Проверка исходников показывает обратное:

```
common/tls/ech.go:1        //go:build go1.24          ← ECH компилируется ВСЕГДА
common/tls/ech_tag_stub.go //go:build with_ech
  var _ int = "Due to the migration to stdlib, the separate `with_ech`
               build tag has been deprecated and is no longer needed…"
```

То есть `with_ech` не просто отсутствует — он **намеренно отравлен**: сборка
С этим тегом не компилируется, потому что ECH переехал в stdlib и доступен
безусловно. Никакой ошибки «ECH is not included» в ядре не существует.

✅ Прогон подтверждает: `tls.ech{enabled:true}` **проходит** `check`.
Вердикт ядра — **D (работает)**, а не A/B и даже не C.

**Что из этого следует.**

1. Обоснование «иначе падение» — **ложное**, и его надо убрать из реестра и
   из комментариев обеих сторон, иначе оно будет воспроизводиться дальше.
2. Настоящее обоснование снятия остаётся **одно** и оно узкое: Xray-форма
   `ech=<public_name>+<resolver>` несёт **чужой ключ** (public_name ≠ SNI
   узла) и убивает handshake — device-verified, §320 LxBox.
3. Но это аргумент против **конкретной URI-формы Xray**, а не против поля
   `tls.ech` как такового. Валидный `tls.ech` из sing-box-JSON ядро
   применит — и сегодня **обе стороны его молча выбрасывают**, то есть
   деградируют рабочую функцию.

→ Вынесено в **§7.2** как решение владельца: снимать только Xray-форму
(с кодом), а нативный `tls.ech` из sing-box-JSON пропускать.

### (e) naive — какие TLS-ключи допускаются

**Ядро, `protocol/naive/outbound.go` — это закрытый список с фаталами:**

| Ключ | Поведение ядра | Строка |
|---|---|---|
| `tls` целиком | **обязателен**: `tls == nil \|\| !enabled` → **B** `TLS required` | `:45-46` |
| `server_name` | **читается** | `:91-92` |
| `certificate` | **читается** (Listable) | `:109-110` |
| `certificate_path` | **читается** | `:111-114` |
| `ech.*` | **читается** (`query_server_name`, `config`, `config_path`) | `:141-148` |
| `disable_sni` | ✅ **B** `disable_sni is not supported on naive outbound` | `:48-49` |
| `insecure` | ✅ **B** | `:51-52` |
| `alpn` | ✅ **B** | `:54-55` |
| `min_version` / `max_version` | ✅ **B** | `:57-61` |
| `cipher_suites` | ✅ **B** | `:63-64` |
| `curve_preferences` | ✅ **B** | `:66-67` |
| `client_certificate(_path)` | ✅ **B** | `:69-70` |
| `client_key(_path)` | ✅ **B** | `:72-73` |
| `fragment` / `record_fragment` | ✅ **B** | `:75-76` |
| `kernel_tx` / `kernel_rx` | ✅ **B** | `:78-79` |
| `utls.*` | ✅ **B** `uTLS is not supported on naive outbound` | `:81-82` |
| `reality.*` | ✅ **B** | `:84-85` |

✅ Прогон подтверждает: `insecure` → `insecure is not supported on naive outbound`;
`reality` → `reality is not supported on naive outbound`; голый
`{enabled, server_name}` → OK.

| Аспект | Go | Dart | Целевое |
|---|---|---|---|
| allowlist | `{enabled, server_name, certificate, certificate_path}` (`outbound_tls_emit.go:33-35`), **частный фильтр в эмиттере** | `{certificate, certificate_path}` + `enabled`/`server_name` строятся парсером (`tls_spec.dart:52`, `json_parsers.dart:1429-1436`) | **тот же набор из 4 + `ech`, но правилом реестра `forbidden_for:["naive"]` с кодом, а не частным фильтром** (ТЗ §2) |
| `server_name` | всегда `node.Server`, `sni=` игнорируется (`node_parser_naive.go:157-160`) | всегда `server` (`naive_parser.dart:84`) | совпадает |
| `padding` | код `naive_padding_ignored` (`node_parser_core.go:435`) | `NaivePaddingIgnoredWarning` (`naive_parser.dart:64-69`) | совпадает |
| `extra-headers` charset | `node_parser_naive.go:45` — подмножество RFC 7230 tchar | `naiveHeaderNameRe` (`uri_utils.dart:30-36`) — **тот же набор** | совпадает |
| одиночный userinfo `naive+https://secret@host` | `secret` → **username** (`node_parser_core.go:366-374`) | `secret` → **password** (`naive_parser.dart:21-29`) | **вариант Dart (password)**, см. §7.3 |
| `naive+quic` | парсит (`quic:true` + `quic_congestion_control:"bbr"`) | не парсит (§9.B1) | **Dart добавляет**; это не расхождение правил, а пробел покрытия |

**Ключевой вывод для ТЗ.** Пункт «никаких частных фильтров в эмиттере»
выполним ровно потому, что список ядра **закрытый и явный**. Один атрибут
`forbidden_for: ["naive"]` + код `tls_field_unsupported_naive` на 13 TLS-полях
заменяет и Go-allowlist эмиттера, и Dart-passthrough-список. Но **`ech`
обязан попасть в разрешённые для naive** — иначе конвейер станет строже
ядра (см. §7.2).

### (f) alpn — строка vs массив

| Аспект | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| форма в теле | `addListable` **сохраняет форму как пришла**: строка остаётся строкой (`outbound_tls_emit.go:84-96,103`) | всегда массив (`tls_spec.dart:144`) | ✅ **D**: `alpn:"h2"` (голая строка) **проходит** — `badoption.Listable[string]` | **сохранять форму** (вариант Go) — и это же записано в ТЗ §3.3 |
| percent-декод | `normalizePercentDecodeLoop` **только** на vless/trojan-пути (`node_parser_transport.go:855`); vmess (`node_parser_core.go:830`) и tuic (`node_parser_tuic.go:132`) — без него | до 16 проходов, **per-элемент** (`transport.dart:594-618`) | — | **вариант Dart**, единообразно на всех путях |
| «мусорные» элементы | пропускает как есть | **выбрасывает** элемент с `%`, пробелом, управляющим символом после декода (`transport.dart:612`) | ✅ ядро ALPN не валидирует (**C**: невалидный protocol-id просто не сматчится на сервере) | **вариант Dart (выбрасывать)** |
| пути мимо нормализатора в Dart | — | hysteria2 (`hysteria2_parser.dart:87-89`), tuic (`tuic_parser.dart:43-46`) — сырой split; sing-box-JSON — **только массив**, голая строка игнорируется (`json_parsers.dart:1470-1471`) | — | привести к одному нормализатору |

**Обоснование выброса мусора.** Ядро отдаёт ALPN в TLS как есть; элемент
с `%` или пробелом — это не protocol-id, сервер его не выберет, но он
**занимает место** в списке и может сдвинуть выбор на нежелательный
протокол. Выброс безопасен: валидные `h2`/`http/1.1`/`h3` не содержат
таких символов. При этом важно: **выбрасывается элемент, а не поле** —
если после фильтра список пуст, `alpn` не эмитится.

Оговорка по форме: в sing-box-JSON Dart **теряет** голую строку `alpn:"h2"`
(читает только массив), хотя ядро её принимает. Это баг покрытия Dart,
целевое — читать обе формы (`listable_string` в реестре).

### (g)/(h) flow, vision + transport, xtls-rprx-vision-udp443

| Аспект | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| allowlist | `""`, `xtls-rprx-vision`; фильтр в **санитайзере** (`singbox_sanitize.go:254-260`) и **эмиттере** (`outbound_generator.go:668-671`); URI-парсер пропускает мусор в карту | `""`, `xtls-rprx-vision`; **единый гейт в эмиттере** (`node_spec_emit.dart:84-86`) | ✅ **B**: `xtls-rprx-direct` → `unsupported flow: xtls-rprx-direct` | **вариант Dart (один гейт)**, но код `flow_deprecated` ставить |
| vision + транспорт | flow снимается (санитайзер `:261-265`, эмиттер `:664-667`), **кода нет** — только debuglog | flow снимается + `VisionWithTransportWarning` (`vless_parser.dart:42-45`, `json_parsers.dart:541-546`) | ✅ **D** — `vision` + `ws` **проходит check** | **вариант Dart (снимать + код)** |
| `xtls-rprx-vision-udp443` → vision + `packet_encoding:xudp` | оба пути | оба пути | — | совпадает |
| …и **переписывание порта на 443** | URI-путь **переписывает** (`node_parser_core.go:681-685`), Xray-путь — **нет** (`xray_outbound_convert.go:166-171`) | Xray-путь **переписывает** (`json_parsers.dart:524-528`), URI-путь — **нет** | — | **НЕ переписывать ни на одном пути**, см. §7.4 |

**Обоснование по vision+transport.** Ядро такой конфиг **принимает** (**D**),
то есть падения нет — но `vision` поверх ws/grpc бессмыслен: Vision
работает по голому TLS-потоку, а внутри транспорта его инспектор видит не
TLS-рекорды. Узел «работает», но без обещанной обфускации — классический
случай, когда молчание хуже кода. Вариант Dart (снять + info-код) точнее
описывает реальность, чем Go (снять молча).

**Обоснование по allowlist.** Ядро даёт **B** — весь конфиг. Значит фильтр
обязателен; вопрос только где. Dart-схема (один гейт на эмите, покрывающий
все пути включая sing-box-JSON) строго лучше Go-схемы (два гейта, и ни
один не покрывает URI-путь до эмита). В новом конвейере это правило
реестра `on_invalid: {action: drop, code: flow_deprecated}` + `forbidden_when: transport present`.

### (i) packet_encoding

| Значение | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| `xudp`, `packetaddr` | приняты | приняты | ✅ **D** | совпадает |
| `""` | ключ не пишется | ключ не пишется | — | совпадает |
| **`none`** | снимается **молча** (`node_parser_core.go:689-704`) | снимается **молча** (`uri_utils.dart:254-267`, явный коммент: «пусто и none = поле отсутствует») | ✅ **B**: `packet_encoding:"none"` → `unknown packet encoding: none` | **снимать молча** (совпадает) |
| прочий мусор | снять + `WarnPacketEncodingUnknown` | снять + `PacketEncodingUnknownWarning` | ✅ **B**: `unknown packet encoding: garbage` | совпадает |
| **эмиттер** | 🔴 **не валидирует** — пишет любую непустую строку (`outbound_generator.go:676-678`) | гейт в парсере, эмиттер пишет что дали (`node_spec_emit.dart:88`) | — | правило реестра + табличный эмиттер закрывают обе дыры |

Правила совпадают; расхождение только в **дыре эмиттера Go** — сторонний
`node.Outbound` с `packet_encoding:"none"` роняет весь конфиг. Реестровая
формулировка «пустое/none = отсутствие поля» верна лишь потому, что обе
стороны снимают его в парсере; сформулировать надо как «`none` — алиас
отсутствия, ядро его НЕ принимает (B)».

Историческая заметка: реестр и комментарии в коде говорят про «панику ядра
(SPEC 049)». ✅ На lx.33 это уже не паника, а аккуратный фатал
(`unknown packet encoding`) — то есть ядро починили, но **фатальность
осталась**, и гард по-прежнему обязателен.

### (j) ws `?ed=N` / early data

| Аспект | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| хвост пути `path=/x?ed=N` | читает, срезает, ставит `max_early_data` + **всегда** `early_data_header_name="Sec-WebSocket-Protocol"` (`:707-719`) | то же (`transport.dart:66-69`) | ✅ **D** обе формы | совпадает |
| **плоский `ed=N`** | **не читает** | читает (`transport.dart:54`) | — | **вариант Dart** |
| **плоский `eh=NAME`** | **не параметризует** — всегда константа | читает; `eh` без `ed` игнорируется (`transport.dart:57`) | ✅ **D** — произвольное имя принимается | **вариант Dart** |
| приоритет | — | хвост пути > плоский | — | **вариант Dart** |
| `ed <= 0` / не число | значение отброшено, path всё равно чистится | то же (`transport.dart:155-159,185-188`) | — | совпадает |
| **httpupgrade** | хвост срезается **только** в Xray-ветке (`splitWSEarlyData`); в URI-ветке `uriTransportFromQuery` **path пишется как есть** → хвост уезжает в конфиг | срезается **во всех** ветках (`transport.dart:109-122`) | ✅ **D** для ядра (`?ed=` в path не фатал)… но **C** по смыслу: сервер отдаёт **404** | **вариант Dart** |
| код | `WarnWSEarlyDataEDConverted` только для хвоста | `WsEarlyDataConvertedWarning` только для хвоста | — | совпадает |

**Обоснование.** Память проекта (`ws-early-data-ed-param`, issue #96)
фиксирует: `?ed=` в пути даёт **404 в рантайме, а check проходит** — то
есть вердикт ядра **C**, самый коварный. Go-дыра в URI-ветке httpupgrade
воспроизводит ровно этот баг. Вариант Dart (срезать везде) — единственный
правильный.

Плоские `ed`/`eh` — чистое расширение покрытия: Go их теряет, Dart читает;
ядро принимает произвольный `early_data_header_name`, значит
параметризация безопасна.

### (k) sni / server_name / Host по умолчанию

| Протокол | Go | Dart | Целевое |
|---|---|---|---|
| vless | `sni`→`peer`→`server`, **без** эвристики мусора (`:877-884`) | `sni`→`peer`→`server` (`transport.dart:471-472`) | совпадает |
| trojan | `sni`→`peer`→`host`→`server` (`:986-994`) | то же (`transport.dart:534-535`) | совпадает |
| hysteria2 / hysteria / anytls | `sni` + **эвристика мусора**: пусто, `🔒`, или нет ни `.` ни `:` → `server` | hysteria2 — **та же эвристика** (`hysteria2_parser.dart:82-85`); **anytls — только пустой** (`transport.dart:330-331`), мусор уезжает в `server_name` | **эвристика на всех TLS-протоколах** (вариант Go для anytls) |
| tuic | `sni`, пусто → `server` | то же + `disable_sni=1` → `serverName:null` (`tuic_parser.dart:50`) | **Dart-вариант** (+`disable_sni`) |
| masque `disable_sni` | **не парсит** | парсит (§393) | **вариант Dart** |
| naive | всегда `server` | всегда `server` | совпадает |
| **ws Host** | `host`→`sni`→`obfsParam` (`:207-216`) | то же (`transport.dart:49-51`) | совпадает |
| **httpupgrade/xhttp host** | vmess-ветка `host`→`sni`; vless/trojan — только `host` | httpupgrade/xhttp — **только `host`**, намеренно (D-016в, identity-паритет) | **вариант Dart** |

✅ Ядро: `tls{enabled:true}` без `server_name` **проходит check** (**D**) —
ядро подставит адрес дозвона. То есть пустой SNI не фатал, а вопрос
корректности: эвристика мусора (`🔒`) нужна не ради ядра, а ради
реальных кривых подписок.

⛔ Здесь один пункт против правила «целевое = LxBox»: **anytls**. Вариант
Go (эвристика) строго лучше — см. §7.5.

### (l) hysteria2

| Аспект | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| obfs allowlist | `salamander`, `gecko`, **регистрозависимо** на URI (`:15-17,39`), санитайзер лоуэркейсит | те же два, `trim().toLowerCase()` (`hysteria2_obfs.dart:31-47`) | ✅ **D** для `gecko` | **вариант Dart (лоуэркейс везде)** |
| obfs без пароля | снять + код (`:45-50`); санитайзер — **без кода** | снять + `MissingObfsPasswordWarning` | ✅ **B**: `missing obfs password` | **вариант Dart (код на всех путях)** |
| **up/down mbps — имена в URI** | `upmbps` / `downmbps` (`:75-84`) | `up_mbps` / `down_mbps`… **нет**: читает `upmbps`/`downmbps` (`hysteria2_parser.dart:129-130`), а **эмитит** `upmbps`/`downmbps` (`node_spec_emit.dart:426-427`) | — | **оба имени на входе в обоих**; канон эмита — §7.6 |
| up/down — тип | URI: `Atoi`, провал → ключ отсутствует. Санитайзер принимает `string`/`float64`/`int` (`singbox_sanitize.go:298-334`) | URI: `int.tryParse`. sing-box-JSON: `num` → `toInt()` (т.е. `100.0` ок, `"100"` → null) | ✅ **A**: `up_mbps:"100"` → `cannot unmarshal string into … up_mbps of type int` | **число; строку приводить при разборе, в тело писать int** |
| суффикс единиц (`"100 mbps"`) | **hysteria v1** режет по первому не-цифре (`:103-118`); hysteria2 — нет | нигде | — | привести к одному правилу (v1-поведение на оба) |
| `mport` / port hopping | читает, восстанавливает из authority (`hysteria2_ports.go`) | читает `mport`→`ports`, восстанавливает из authority (`hysteria2_parser.dart:74-79,179-235`) | ✅ формат `"low:high"` | совпадает (реестровая заметка «только Go» **протухла**) |
| `pinSHA256` | читает | читает (`hysteria2_parser.dart:97,107`) | — | совпадает (реестр «только Go» — **протухло**) |

**Обоснование по типам.** Это прямая иллюстрация памяти
`json-map-type-assert-trap`: `up_mbps` из JSON-тела приезжает `float64`,
а эмиттер с `.(int)` его теряет. Ядро при этом на строку даёт **A** — то
есть «пробросить как есть» нельзя. Правило реестра: `type: int` с
приведением `"100"`→100, `100.0`→100, `"100 mbps"`→100, иначе снять поле.

### (m) tuic

| Поле | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| `congestion_control` | `{cubic, new_reno, bbr}`; мусор → **снять ключ** + код (`:69-76`) | тот же набор; мусор → **`null`** (= ключ не пишется) + код (`tuic_parser.dart:59-61,92-96`) | ✅ **B**: `unknown congestion control algorithm: garbage` | **снять ключ + код** (совпадает по сути) |
| **дефолт в эмите** | пишет **только при наличии** в URI | 🔴 пишет **всегда** (дефолт `cubic`) | — | **не писать дефолт** (вариант Go), см. §7.7 |
| `udp_relay_mode` | `{native, quic}`; мусор → снять + `WarnTuicUDPRelayModeInvalid` (`:79-86`) | «не `quic` ⇒ `native`» — **мусор становится `native`, без кода** (`tuic_parser.dart:32-35`) | ✅ **C — молча игнорируется**: `switch` без `default` (`protocol/tuic/outbound.go:59-63`), мусор проваливается в native | **снять + код** (вариант Go), см. §7.8 |
| `udp_relay_mode` + `udp_over_stream` | — | — | **B**: `udp_over_stream is conflict with udp_relay_mode` (`outbound.go:56-58`) | правило `conflicts` в реестре |
| `alpn` дефолт | не подставляет | 🔴 подставляет `["h3"]` | — | **не подставлять** (вариант Go), §7.7 |
| zero-RTT алиасы | `zero_rtt_handshake` + `reduce_rtt` | `reduce_rtt` + `zero_rtt` | — | **объединение трёх** |
| `heartbeat` | голое число → `Ns` (`:34-46`) | то же (`normalizeSingboxDuration`) | — | совпадает |
| требование пароля | uuid обязателен, пустой пароль → warning, узел жив (`:62-66`) | требует **оба** непустыми, иначе узел `null` | — | см. §2(s) и §7.9 |

### (n) shadowsocks

| Аспект | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| allowlist методов | 9 значений, **регистрозависимо** (`node_parser_ss.go:6-21`) | те же 9 (`uri_utils.dart:470-483`) | 🔴 см. ниже | **см. §7.10** |
| метод вне списка | **узел дропается** (жёсткая ошибка) | **узел дропается** (`shadowsocks_parser.dart:50`) | ✅ `totally-bogus-method` → **B** `unknown method`; но ✅ **`rc4-md5`, `aes-128-cfb`, `aes-256-cfb` — ПРОХОДЯТ (D)** | 🔴 **расширить allowlist legacy-шифрами**, §7.10 |
| `plugin` / `plugin_opts` | 🔴 **не парсит и не эмитит вовсе** | парсит SIP003 (`plugin_name;opts`), эмитит в outbound; в URI не возвращает (`shadowsocks_parser.dart:87-88,95-106`) | ✅ `obfs-local` → **D**; неизвестное имя → **B** `plugin not found` | **вариант Dart** + валидация имени плагина по списку сборки ядра |

### (o) vmess

| Поле | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| `security` — URI-путь | 6 значений, **включая `aes-128-ctr`** (`node_parser_vmess.go:18-31`) | те же 6, **включая `aes-128-ctr`** (`uri_utils.dart:451-467`) | 🔴 ✅ **B**: `unsupported security type: aes-128-ctr` | 🔴 **убрать `aes-128-ctr` из обоих**, §7.11 |
| `security` — Xray-путь Go | 5 значений, **без** `aes-128-ctr` (`xray_protocols.go:368-377`) | — | — | ближе к истине, но тоже неполный (нет `aes-128-cfb`) |
| **`aes-128-cfb`** | 🔴 **нет ни у кого** → уезжает в `auto` | 🔴 **нет** → `auto` | ✅ **D** — ядро принимает | 🔴 **добавить в обоих**, §7.11 |
| `security` — sing-box-JSON Dart | — | **не валидирует** (`json_parsers.dart:1039`) | **B** на мусоре | валидировать на всех путях |
| алиас `chacha20-ietf-poly1305` | → `chacha20-poly1305` | то же | ✅ | совпадает |
| пусто/`null`/`undefined`/мусор | → `auto` | → `auto` | ✅ `auto`, `none`, `zero` — **D** | совпадает |
| `auto` при включённом TLS | — | — | ⚠️ ядро **молча переписывает** `auto`→`zero` (`protocol/vmess/outbound.go:106`) | знать при сверке identity |

**Точный набор ядра** (`sing-vmess@v0.2.8-…/client.go:44-52`, verbatim):
`auto`, `none`, `zero`, `aes-128-cfb`, `aes-128-gcm`, `chacha20-poly1305`.
Обе стороны ошиблись **в обе стороны сразу**: добавили несуществующий
`aes-128-ctr` (→ фатал) и потеряли реальный `aes-128-cfb` (→ узел молча
идёт на `auto`). Это ровно тот случай, который память `utls-fp-allowlist`
описывает как «канонический список сверять по module cache, а не по памяти».
| `alter_id` | `string`/`float64`, без диапазона; **эмиттер требует `int`** → `float64` из JSON молча теряется (`outbound_generator.go:374-376`) | `num`→`toInt()`, эмит только `!= 0` | ✅ `alter_id:64` — **D** | **вариант Dart** (нормализовать в int при разборе) |
| `global_padding`, `authenticated_length` | не читает | **не существует нигде** | ✅ оба — **D**, ядро принимает | **добавить в реестр** как поля тела (пробел покрытия обеих сторон) |

### (p) reality key_share

| Аспект | Go | Dart |
|---|---|---|
| enum | `{hybrid, classical}`, `trim`+`lower`, мусор → снять + `WarnRealityKeyShareInvalid` (`:825-849`) | `{hybrid, classical}`, **регистр НЕ нормализуется** (`"Hybrid"`→null), мусор → снять **молча** (`transport.dart:452-455`, `tls_spec.dart:225`) |
| читается | только при валидном pbk (D-121) | то же (`transport.dart:496`) |
| гейт версии ядра | `outbound_tls_emit.go:166-179` + проба `CoreSupportsRealityKeyShare` | нет (ядро едет в AAR) |

**Ядро** (`common/tls/reality_client.go:87-93`, статика):
```go
switch options.Reality.KeyShare {
case C.RealityKeyShareDefault, C.RealityKeyShareHybrid, C.RealityKeyShareClassical:
default:
    return nil, E.New("unknown reality key_share: ", …)
}
```
→ **B** на пине lx.4. ✅ На lx.33 поля ещё нет: `unknown field "key_share"` → **A**.
**Проверить на пине** — но обе ветки фатальны, значит гард обязателен.

**Целевое:** `trim`+`lower` (вариант Go) + код (вариант Go) — то есть
⛔ здесь целевое **не** вариант LxBox, см. §7.12. Dart-вариант теряет
`"Hybrid"` из кривой подписки молча и не нормализует регистр, хотя enum
ядра лоуэркейсный.

### (q) fragment / record_fragment

| Аспект | Go | Dart | Ядро |
|---|---|---|---|
| поля | `fragment`, `fragment_fallback_delay`, `record_fragment`, `kernel_tx`/`kernel_rx` (последние — только Linux) (`outbound_tls_emit.go:118-125`) | те же в `kTlsPassthroughKeys` (`tls_spec.dart:13-30`), булевы только при `true` (`:42-48`) | ✅ `fragment`+`record_fragment` — **D** |
| из URI | **никогда** | **никогда** | — |
| источник | только sing-box-JSON round-trip | только sing-box-JSON + глобальные переключатели шаблона | — |
| глобальные переключатели | `core/build/tls_transforms.go` | `post_steps/tls_transforms.dart:57-92` (first-hop only, skip naive, masque только при `vhttp=h2`) | naive+fragment → **B** |
| `tls_fragment` | **не существует** (только имя переменной шаблона) | то же | — |

Расхождений правил нет; отличается только объём пост-обработки. Наличие
`fragment` у naive — фатал ядра, обе стороны закрывают (Go allowlist'ом,
Dart явным skip'ом).

### (r) multiplex (mux)

| Go | Dart | Ядро |
|---|---|---|
| 🔴 **нет поддержки вовсе**; `multiplex` — только имя strip-ключа цепочки (`configtypes/types.go:962`). Xray-`mux` из импорта **молча игнорируется** | 🔴 **нет поддержки вовсе**; `kChainStripMultiplexPadding` — то же (`models/source_chain.dart:37`) | ✅ **D**: `multiplex{enabled, protocol:"smux", max_connections, padding}` и `brutal{...}` принимаются; `protocol:"garbage"` → **B** `unknown protocol` |

Расхождения **нет** — есть общий пробел. Для ТЗ важно: `multiplex.json`
как новая суб-схема (ТЗ §4) создаётся **с нуля**, переносить нечего, и
обе стороны получают её одновременно. Enum `protocol` обязателен: мусор
даёт **B**.

### (s) транспорты, xhttp

| Аспект | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| неизвестный `type` | Xray-конверт молча возвращает `nil` — транспорт **теряется без кода** (`xray_outbound_convert.go:337`) | `UnsupportedTransportWarning` | ✅ **A**: `unknown transport type: kcp` (на декоде!) | **вариант Dart (код)** |
| `splithttp` (алиас xhttp в Xray-JSON) | читает (`:325-336`) | 🔴 **не читает** — узел остаётся **вообще без транспорта** (plain tcp) | — | ⛔ **вариант Go**, §7.13 |
| `h2` в URI-query vless/trojan | **не читает** (только vmess `net=h2` и Xray) | читает (`transport.dart:93-108`) | — | **вариант Dart** |
| xhttp `mode` enum | **не валидирует**, отдаёт ядру (`:287,385-387`) | **не валидирует** (`transport_spec.dart:230`) | ✅ **B**: `unknown mode: garbage` | 🔴 **валидировать обеим**, §7.14 |
| `uplink_data_placement=header` без режима | → дописать `mode:packet-up` + код (`xhttpGuardUplinkPlacement:418-465`) | то же (`transport_spec.dart:284-300`) | ✅ **B** `uplink_data_placement can be header only in packet-up` (проверено и при явном другом режиме) | совпадает |
| `seq_placement`/`x_padding_placement`/`x_padding_method` | **passthrough** | **enum-гейт** + `XhttpParamResetWarning` (`transport_spec.dart:243-252,264-265,314-317`) | не проверено (**проверить запуском**) | **вариант Dart** |
| `sc_stream_up_server_secs`, `sc_max_buffered_posts`, `no_sse_header`, `xmux` | **только Go** (SPEC 102) | нет | — | Dart догоняет (пробел, не конфликт) |
| приоритет `extra` vs плоские | `extra` выигрывает, **кроме** `host`/`path`/`mode` (D-097) | то же (`transport.dart:319,371`) | — | совпадает |
| `grpc service_name` | `serviceName`→`service_name`→`path`; **пустой не пишет** | тот же приоритет; **пишет даже пустой** (`transport_spec.dart:74-77`) | — | **не писать пустой** (вариант Go) |
| vmess `net=httpupgrade` в share-URI | 🔴 Go пишет `net="ws"` (`shareuri_vmess.go:70-74`) — **меняет wire-протокол** | — | — | **исправить Go** (память `emitter-parser-pairing`) |

### (t) порт, uuid, условия дропа

| Аспект | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| порт из URI | `1..65535`, иначе **узел дропается** (`node_parser_core.go:401-406`) | 🔴 **нет проверки диапазона**; `Uri` отсекает синтаксис, иначе дефолт схемы | ✅ **A**: `server_port:99999` → `cannot unmarshal number 99999 into … uint16`; `"443"` строкой → **A** | **проверка `1..65535`, узел в `dropped` с `port_invalid`** — вариант Go |
| uuid | 🔴 только непустота | 🔴 только непустота | ✅ **D** — ядро мусорный uuid **принимает** (`vless.NewClient` хеширует) | оставить как есть; формат — `format:"uuid"` в реестре на уровне **info**, не дропа |
| ss-метод вне списка | дроп | дроп | см. §7.10 | §7.10 |
| tuic без пароля | warning, узел жив | узел `null` | ✅ **D** | **вариант Dart**? — §7.9 |
| hysteria2 без пароля | warning, узел жив (`node_parser_hysteria2.go:23`) | узел жив (`hysteria2_parser.dart:60-63`) | — | совпадает |
| anytls без пароля | warning, узел жив (`node_parser_anytls.go:25`) | узел `null` (`anytls_parser.dart:23`) | — | §7.9 |

**Про `WarnPortInvalid` и `WarnSSMethodInvalid`.** Оба объявлены в
`parse_warnings.go:76,78`, но **никогда не выставляются**: реальное
поведение — жёсткий дроп узла. Doc-комментарии («порт заменён значением по
умолчанию») противоречат коду. Реестр это уже частично признал («КОД НА
УЗЛЕ НЕ СТАВИТСЯ … причина живёт в конверте dropped»), но константы в Go
остались как мёртвый код — снять при рефакторинге.

### (u) прочие находки

| Поле | Go | Dart | Ядро | Целевое |
|---|---|---|---|---|
| anytls REALITY (`pbk`/`sid` в URI) | 🔴 **не поддерживает** | поддерживает (`toUriAnyTls` пишет `security=reality&pbk=&sid=`) | ✅ anytls+reality валиден | **вариант Dart** |
| anytls `min_idle_session` | мусор → снять + код | мусор → `null` | ✅ **A** на строку (`cannot unmarshal string`); отрицательное — **D** | **снять + код**; отрицательное ядро глотает, но смысла не имеет → снимать |
| anytls пароль с `:` | берёт только `Username()` → **хвост теряется** | схлопывает обратно (`userParts.join(':')`) | — | **вариант Dart** |
| trojan пароль с `:` | часть **до** `:` | **весь** userinfo | — | **вариант Dart** |
| socks password-only | пишет `:pass@` | **опускает userinfo целиком** → пароль теряется | — | ⛔ **вариант Go**, §7.15 |
| ssh без пользователя | узел дропается раньше (root-ветка — мёртвый код) | `null`-skip на URI; `root` в sing-box-импорте | ✅ **D** — ядро ssh без user принимает | **root по умолчанию** + код `ssh_user_default` |
| ssh inline `private_key` в share-URI | отказ (`ErrShareURINotSupported`) | **пишет ключ в query** (`node_spec_emit.dart:482`) | — | ⛔ секрет в URI — §7.16 |
| masque `insecure` | читает | **не читает** | — | **Dart добавляет** |
| wireguard plain MTU по умолчанию | 1420 | 1408 | — | §7.17 |
| wireguard ключи (private/public/psk) | валидирует 32 байта, мусор → дроп | 🔴 **не валидирует** — мусор уедет в конфиг | ✅ битый ключ валит конфиг | ⛔ **вариант Go**, §7.18 |
| wireguard `reserved` | только десятичная тройка | + base64 `client_id` | — | **вариант Dart** |
| vmess `serviceName` (grpc) в JSON | не читает (берёт из `path`) | читает | — | **вариант Dart** |
| vmess `mode` (xhttp) в JSON | читает | **не читает** | — | **вариант Go** (зеркально) |
| vless `encryption` (постквантовый) | 🔴 не парсит, статично пишет `encryption=none` | читает как есть (§335) | — | **вариант Dart** |
| Clash YAML в теле подписки | не детектит | детектит (`body_decoder.dart:201`) | — | **вариант Dart** |
| `max_nodes_per_subscription` | есть (`source_loader.go:606`) | нет | — | **Dart добавляет** (память `subscription_scale_fanout`) |

---

## 3. Покрытие warning-кодов: реестр ↔ Dart ↔ Go

Реестр: **61** код (`contract/registry/warnings.json`, `v:1`).
Dart: **37** классов (`models/node_warning.dart`), из них **35** замаплены
на коды в `test/contract/contract_test.dart:84-140`.
Go: **30** строковых констант (`parse_warnings.go:22-129`), из них **2 мёртвые**.

Легенда: ✅ есть · ❌ нет · ⚪ не применимо (чужая платформа/уровень) ·
💀 объявлено, но не выставляется.

| # | Код | Реестр | Dart | Go |
|---|---|---|---|---|
| 1 | `transport_unsupported` | ✅ | ✅ `UnsupportedTransportWarning` | ❌ (молчит) |
| 2 | `protocol_unsupported` | ✅ | ✅ `UnsupportedProtocolWarning` | ❌ (скип без кода) |
| 3 | `field_missing` | ✅ | ✅ `MissingFieldWarning` | ❌ |
| 4 | `flow_deprecated` | ✅ | ✅ `DeprecatedFlowWarning` | ❌ (debuglog) |
| 5 | `vision_with_transport` | ✅ | ✅ `VisionWithTransportWarning` | ❌ (debuglog) |
| 6 | `tls_insecure` | ✅ | ✅ `InsecureTlsWarning` | ❌ |
| 7 | `naive_unavailable` | ✅ | ✅ `NaiveBuildTagWarning` | ✅ (`outbound_generator.go:822`, не через `AddWarning`) |
| 8 | `utls_fp_unknown` | ✅ | ✅ `UnknownFingerprintWarning` | ✅ `WarnUTLSFingerprintUnknown` |
| 9 | `reality_fp_not_chrome` | ✅ | ✅ `RealityFingerprintWarning` | ✅ `WarnRealityFPNotChrome` |
| 10 | `xhttp_param_reset` | ✅ | ✅ `XhttpParamResetWarning` | ✅ `WarnXHTTPParamReset` |
| 11 | `xhttp_mode_forced_packet_up` | ✅ | ✅ `XhttpModeForcedPacketUpWarning` | ✅ `WarnXHTTPModeForcedPacketUp` |
| 12 | `ech_ignored` | ✅ | ✅ `EchIgnoredWarning` | ❌ |
| 13 | `obfs_unknown` | ✅ | ✅ `UnknownObfsWarning` | ✅ `WarnObfsUnknown` |
| 14 | `obfs_password_missing` | ✅ | ✅ `MissingObfsPasswordWarning` | ✅ `WarnObfsPasswordMissing` |
| 15 | `detour_cycle_broken` | ✅ | ✅ `DetourCycleBrokenWarning` | ❌ (debuglog) |
| 16 | `detour_target_missing` | ✅ | ✅ `DetourTargetMissingWarning` | ❌ |
| 17 | `detour_to_group` | ✅ | ✅ `DetourToGroupWarning` | ❌ |
| 18 | `detour_chain_too_deep` | ✅ | ✅ `DetourChainTooDeepWarning` | ❌ |
| 19 | `selector_as_auto` | ✅ | ✅ `SelectorAsAutoWarning` | ⚪ (Go импортирует selector нативно) |
| 20 | `group_member_missing` | ✅ | ✅ `GroupMemberMissingWarning` | ❌ |
| 21 | `reality_pbk_invalid` | ✅ | ❌ | ❌ (debuglog) |
| 22 | `reality_short_id_invalid` | ✅ | ✅ `RealityShortIdInvalidWarning` | ✅ `WarnRealityShortIDInvalid` |
| 23 | `reality_key_share_invalid` | ✅ | ❌ | ✅ `WarnRealityKeyShareInvalid` |
| 24 | `packet_encoding_unknown` | ✅ | ✅ `PacketEncodingUnknownWarning` | ✅ `WarnPacketEncodingUnknown` |
| 25 | `naive_padding_ignored` | ✅ | ✅ `NaivePaddingIgnoredWarning` | ✅ `WarnNaivePaddingIgnored` |
| 26 | `naive_extra_headers_invalid` | ✅ | ✅ `NaiveExtraHeadersInvalidWarning` | ✅ `WarnNaiveExtraHeadersInvalid` |
| 27 | `ss_method_invalid` | ✅ | ❌ (дроп) | 💀 `WarnSSMethodInvalid` (объявлен, не выставляется) |
| 28 | `port_invalid` | ✅ | ❌ (дефолт) | 💀 `WarnPortInvalid` (объявлен, не выставляется) |
| 29 | `uri_too_long` | ✅ | ❌ (дроп без кода) | ✅ (`node_parser_core.go:96`, не через код) |
| 30 | `tuic_congestion_invalid` | ✅ | ✅ `TuicCongestionInvalidWarning` | ✅ `WarnTuicCongestionInvalid` |
| 31 | `tuic_udp_relay_mode_invalid` | ✅ | ❌ (мусор→`native`) | ✅ `WarnTuicUDPRelayModeInvalid` |
| 32 | `anytls_min_idle_invalid` | ✅ | ✅ `AnyTlsMinIdleInvalidWarning` | ✅ `WarnAnyTLSMinIdleInvalid` |
| 33 | `masque_vhttp_invalid` | ✅ | ✅ `MasqueVhttpInvalidWarning` | ✅ `WarnMasqueVHTTPInvalid` |
| 34 | `ssh_user_default` | ✅ | ❌ | ✅ `WarnSSHUserDefault` |
| 35 | `awg_header_invalid` | ✅ | ✅ `AwgHeaderInvalidWarning` | ✅ `WarnAWGHeaderInvalid` |
| 36 | `awg3_field_invalid` | ✅ | ✅ `Awg3FieldInvalidWarning` | ✅ `WarnAWG3FieldInvalid` |
| 37 | `awg3_header_key_invalid` | ✅ | ✅ `Awg3HeaderKeyInvalidWarning` | ✅ `WarnAWG3HeaderKeyInvalid` |
| 38 | `awg3_padding_too_short` | ✅ | ✅ `Awg3PaddingTooShortWarning` | ✅ `WarnAWG3PaddingTooShort` |
| 39 | `awg3_random_trailers_wide_headers` | ✅ | ✅ `Awg3RandomTrailersWideHeadersWarning` | ✅ `WarnAWG3RandomTrailersWideHeaders` |
| 40 | `awg3_core_unsupported` | ✅ | ⚪ (ядро в AAR) | ✅ `WarnAWG3CoreUnsupported` |
| 41 | `awg_headers_overlap` | ✅ | ❌ | ✅ `WarnAWGHeadersOverlap` |
| 42 | `amnezia_container_choice` | ✅ | ❌ | ✅ `WarnAmneziaContainerChoice` |
| 43 | `group_empty` | ✅ | ❌ | ❌ |
| 44 | `detour_with_listen_port` | ✅ | ⚪ (desktop) | ❌ |
| 45 | `source_detour_missing` | ✅ | ⚪ (desktop) | ❌ |
| 46 | `max_nodes_exceeded` | ✅ | ❌ | ❌ |
| 47 | `clash_yaml_unsupported` | ✅ | ❌ (детект есть, кода нет) | ❌ |
| 48 | `template_var_undeclared` | ✅ | ✅ `if_engine.dart` | ✅ `substitute_canon.go` |
| 49 | `template_unknown_directive` | ✅ | ✅ `if_engine.dart` | ✅ `substitute_canon.go` |
| 50 | `template_int_clamped` | ✅ | ⚪ (только в тесте) | ✅ `intCastCanon` |
| 51 | `template_int_invalid` | ✅ | ⚪ (только в тесте) | ✅ `intCastCanon` |
| 52 | `chain_unsupported_by_core` | ✅ | ✅ `ChainDegradation` (строковый код) | ✅ `core_chain_capability.go:96` |
| 53 | `chain_hop_missing` | ✅ | ✅ `chain_nodes.dart:137` | ✅ `chain_nodes.go:130` |
| 54 | `chain_invalid` | ✅ | ✅ `chain_nodes.dart:107,117` | ✅ `chain_generator.go:54` |
| 55 | `chain_strip_utls_on_reality` | ✅ | ✅ `ChainIssueCode.stripUtlsOnReality` | ✅ `chain_validate.go:59` |
| 56 | `chain_nested_position` | ✅ | ❓ (проверить) | ✅ |
| 57 | `chain_cycle_through_direction` | ✅ | ❓ | ✅ |
| 58 | `dialer_proxy_unusable` | ✅ | ✅ `DialerProxyUnusableWarning` | ✅ `WarnDialerProxyUnusable` |
| 59 | `tailscale_core_unsupported` | ✅ | ⚪ | ✅ `WarnTailscaleCoreUnsupported` |
| 60 | `tailscale_from_subscription` | ✅ | ❌ | ✅ `WarnTailscaleFromSubscription` |
| 61 | `ws_early_data_converted` | ✅ | ✅ `WsEarlyDataConvertedWarning` | ✅ `WarnWSEarlyDataEDConverted` |

### 3.1 Сводка

| Метрика | Значение |
|---|---|
| Кодов в реестре | 61 |
| Реализовано в Dart | 35 (+2 класса вне маппинга: `SectionsRecordDroppedWarning`, `SectionsConflictWarning` — SPEC 121/122, **в реестре отсутствуют**) |
| Реализовано в Go | 28 живых + 2 мёртвых |
| **В обоих** | 24 |
| **Только Dart** | 11 (`transport_unsupported`, `protocol_unsupported`, `field_missing`, `flow_deprecated`, `vision_with_transport`, `tls_insecure`, `ech_ignored`, `detour_*` ×4, `group_member_missing`, `selector_as_auto`) |
| **Только Go** | 9 (`reality_key_share_invalid`, `tuic_udp_relay_mode_invalid`, `ssh_user_default`, `awg_headers_overlap`, `amnezia_container_choice`, `awg3_core_unsupported`, `tailscale_*`, `uri_too_long`) |
| **Ни там ни там** | 5 (`reality_pbk_invalid`, `group_empty`, `max_nodes_exceeded`, `clash_yaml_unsupported`, + `ss_method_invalid`/`port_invalid` как «код на узле не ставится») |

### 3.2 Расхождение механизмов хранения

| | Go | Dart |
|---|---|---|
| Поле | `ParsedNode.Warnings []string` (`configtypes/types.go:765`) | `NodeSpec.warnings List<NodeWarning>` (`node_spec.dart:55`) |
| Содержимое | **только код** | **объект с параметрами** (`props`, `messageWith(t)`, `renderEn()`) |
| Дедуп | по коду (`AddWarning:769-780`) | по `props`-равенству (`node_warning.dart:35-38`) |
| Сериализация | в state не пишется | **не сериализуется**, пересоздаётся при parse/emit |
| **⚠ в UI** | 🔴 **нет ни одного потребителя** `ParsedNode.Warnings` в `ui/`; ⚠ есть только у источников (`ui/configurator/dialogs/source_error_dialog.go`) и у висячего члена группы (`ui/servers_node_info.go:452`) | ✅ **есть**: `NodeWarningRow` (`subscription_detail_screen/widgets/node_warning_row.dart:16-38`), строка узла (`subscription_node_list.dart:199`), баннер-счётчик actionable (`:107-129`), карточка узла (`node_settings_screen.dart:518-523`), импорт бэкапа (`backup_screen.dart:544`) |

**Вывод для ТЗ §5.1.** Go придётся не только добавить `Warnings` в
`state.Node`, но и **построить UI-канал с нуля** — в LxBox он уже есть и
может служить эталоном (иконка/цвет по severity, баннер считает только
non-info). Параметры (`Path`, `Value`, `Params`) у Go сегодня нет вовсе —
`AddWarning(code)` принимает только код; это ломающее изменение сигнатуры.

---

## 4. Протухшие записи реестра (править вместе с §4 ТЗ)

Обнаружены при сверке с кодом и прогоном ядра:

| Файл / путь | Утверждение | Реальность |
|---|---|---|
| `tls.json /tls/reality/sid` | «Go обрезает >16 до 16 и нечётную длину пропускает как есть (ядро упадёт)» | Go **сбрасывает в `""`** обе ситуации (`node_parser_transport.go:798-800`) — расхождение уже закрыто, целевое достигнуто |
| `tls.json /tls/params/ech` (+ D-006, §9.B7, коммент `outbound_tls_emit.go:11-13`) | «`with_ech` в LX_TAGS нет, пропущенный `ech` уронил бы конфиг» | 🔴 **посылка неверна целиком**: ECH компилируется всегда (`common/tls/ech.go`, `//go:build go1.24`), тег `with_ech` намеренно **отравлен** (`ech_tag_stub.go`). `tls.ech` — **D**. Остаётся только аргумент про Xray-форму с чужим ключом → §7.2 |
| `vmess.json /uri/query/scy` | «мусор → `auto`; в Xray-ветке Go allowlist чуть уже» | набор ядра — `auto,none,zero,aes-128-cfb,aes-128-gcm,chacha20-poly1305`; **оба** проекта держат несуществующий `aes-128-ctr` и **теряют** реальный `aes-128-cfb` → §7.11 |
| `allowlists.json packet_encoding` | «иначе паника ядра (SPEC 049)» | ✅ на lx.33 — аккуратный фатал `unknown packet encoding` (**B**), не паника. Гард всё равно обязателен |
| `hysteria2.json /uri/query/mport` | «Сейчас только Go; §9.B2: Dart добавляет» | Dart **уже** читает `mport`/`ports` и восстанавливает из authority (`hysteria2_parser.dart:74-79,179-235`) |
| `hysteria2.json /uri/query/pinSHA256` | «Только Go» | Dart читает (`hysteria2_parser.dart:97,107`) |
| `masque.json` (заголовок) | «задание ошибочно относило masque к endpoint» | ⚠️ строка исправлена по §9.4: на пине lx.4 `masque` — **outbound** (`include/quic.go:29`), в `endpoints[]` даёт `unknown endpoint type: masque`; лаунчер и реестр (`kind: outbound`) правы, ранний прогон этой строки был на другой версии |
| `warnings.json ss_method_invalid` / `port_invalid` | описывают деградацию поля | в Go — жёсткий дроп узла, константы **мёртвые**; в реестре уже есть оговорка «КОД НА УЗЛЕ НЕ СТАВИТСЯ», но константы в Go надо снять |
| `warnings.json awg_headers_overlap` | «ядро отвергает такой endpoint **на загрузке**» | ✅ прогон: `check` **проходит**; ошибка `headers must not overlap` возникает в `submodules/wireguard-go/device/uapi.go:839` при конфигурировании устройства, т.е. **на старте** (**B**, но не на check) |

Ещё две записи требуют не правки, а **дополнения**: `vmess` не описывает
`global_padding`/`authenticated_length` (ядро принимает), и нигде нет
`multiplex` (ядро принимает, обе стороны не умеют).

---

## 5. Сводная матрица вердиктов ядра

Быстрая шпаргалка «что будет, если мусор доедет до ядра». ✅ = прогон.

| Значение | Вердикт | Сообщение |
|---|---|---|
| неизвестный ключ в outbound | ✅ **A** | `json: unknown field "…"` |
| `tls.insecure:"true"` (строка) | ✅ **A** | `cannot unmarshal string into … insecure of type bool` |
| `server_port:"443"` (строка) | ✅ **A** | `cannot unmarshal string into … uint16` |
| `server_port:99999` | ✅ **A** | `cannot unmarshal number 99999 into … uint16` |
| `up_mbps:"100"` (строка) | ✅ **A** | `cannot unmarshal string into … up_mbps of type int` |
| `min_idle_session:"5"` | ✅ **A** | `cannot unmarshal string into … AnyTLSOutboundOptions` |
| `reality.short_id:["a","b"]` (массив) | ✅ **A** | не Listable |
| `transport.type:"kcp"` | ✅ **A** | `unknown transport type: kcp` |
| `masque.vhttp:"garbage"` | ✅ **A** | enum-тег на декоде |
| `reality.key_share` на lx.33 | ✅ **A** | `unknown field "key_share"` (на пине lx.4 → **B**) |
| `reality.short_id` нечётной длины | ✅ **B** | `decode short_id: encoding/hex: odd length hex string` |
| `reality.short_id` не-hex | ✅ **B** | `encoding/hex: invalid byte` |
| **`reality.short_id` 18 hex-символов** | ✅ **ПАНИКА** | `panic: index out of range [8] with length 8` |
| `reality.public_key:"enabled"` | ✅ **B** | `invalid public_key` |
| REALITY без блока `utls` | ✅ **B** | `uTLS is required by reality client` |
| `utls.fingerprint:"HelloChrome_120"` | ✅ **B** | `unknown uTLS fingerprint` |
| `packet_encoding:"none"` | ✅ **B** | `unknown packet encoding: none` |
| `packet_encoding:"garbage"` | ✅ **B** | `unknown packet encoding: garbage` |
| `flow:"xtls-rprx-direct"` | ✅ **B** | `unsupported flow` |
| `vmess.security:"aes-128-ctr"` | ✅ **B** | `unsupported security type: aes-128-ctr` |
| `tuic.congestion_control:"garbage"` | ✅ **B** | `unknown congestion control algorithm` |
| hysteria2 `obfs` без `password` | ✅ **B** | `missing obfs password` |
| `multiplex.protocol:"garbage"` | ✅ **B** | `unknown protocol: garbage` |
| ss `method:"totally-bogus-method"` | ✅ **B** | `unknown method` |
| ss `plugin:"bogus-plugin"` | ✅ **B** | `plugin not found` |
| naive + `insecure`/`alpn`/`utls`/`reality`/`fragment`/… | ✅ **B** | `… is not supported on naive outbound` |
| xhttp `mode:"garbage"` | ✅ **B** | `unknown mode: garbage` |
| xhttp `uplink_data_placement:"header"` вне packet-up | ✅ **B** | `can be header only in packet-up mode` |
| AWG пересекающиеся `h1`/`h2` | ✅ **B** (на **старте**, не на check) | `headers must not overlap` |
| `tuic.udp_relay_mode:"garbage"` | ✅ **C** | `switch` без `default` → молча native |
| `udp_over_stream` + `udp_relay_mode` | **B** | `udp_over_stream is conflict with udp_relay_mode` |
| висячий `detour` на несуществующий тег | **C** | check проходит, падает на первом дозвоне |
| `socks.version:"9"` | **B** | `unknown socks version: 9` |
| hysteria2 `server_ports:"1000-2000"` (дефис) | **B** | `bad port range` — нужен **двоеточие** `"1000:2000"` |
| hysteria2 `obfs.type` неизвестный | **A** | свой `UnmarshalJSON` (`option/hysteria2.go:85-91`) |
| masque `tls.alpn` | **C** | ядро **предупреждает и игнорирует** (`legacy_options_lx.go:114`) |
| masque фрагментация на h3 | **C** | игнорируется (`legacy_options_lx.go:128`) |
| `vmess.security:"auto"` при TLS | **C** | ядро молча переписывает в `zero` (`protocol/vmess/outbound.go:106`) |
| `tls.ech` (любая сборка) | ✅ **D** | 🔴 ECH скомпилирован всегда; `with_ech` отравлен |
| `vmess.security:"aes-128-cfb"` | ✅ **D** | 🔴 обе стороны это **теряют** (→`auto`) |
| `vless.uuid:"zzz"` | ✅ **C** | check проходит, соединение не встанет |
| `flow:vision` + `transport:ws` | ✅ **C** | check проходит, Vision не работает |
| ws `path:"/x?ed=2560"` (хвост не срезан) | ✅ **C** | check проходит, сервер отдаёт 404 |
| `alpn:"h2"` (голая строка) | ✅ **D** | Listable |
| `tls{enabled:true}` без `server_name` | ✅ **D** | адрес дозвона |
| `tls{enabled:false}` явно | ✅ **D** | на lx.33; на lx.5–lx.18 был SIGSEGV (SPEC 045) |
| `utls{enabled:true}` без `fingerprint` | ✅ **D** | |
| `firefox`/`random` на REALITY | ✅ **D** | вопрос поведения сервера, не конфига |
| ss `method:"rc4-md5"` / `aes-128-cfb` / `aes-256-cfb` | ✅ **D** | 🔴 обе стороны это **дропают** |
| `vmess.security:"zero"`, `alter_id:64` | ✅ **D** | |
| `global_padding` / `authenticated_length` | ✅ **D** | 🔴 обе стороны не умеют |
| `multiplex` (`smux`, `brutal`) | ✅ **D** | 🔴 обе стороны не умеют |
| `anytls.min_idle_session:-5` | ✅ **D** | смысла не имеет |
| ssh без `user` | ✅ **D** | |

---

## 6. Как LxBox потребляет контракт (потребуют ли новую секцию автоматически)

**Короткий ответ: нет, автоматически не потребят. Нужна ручная работа на
стороне LxBox, но красный тест они получат сами.**

### 6.1 Механизм синхронизации

`/Users/macbook/projects/LxBox/app/tool/sync_contract.sh` — 60 строк:
- источник `LX_CONTRACT_SRC`, дефолт `/Users/macbook/projects/singbox-launcher/contract` (`:18`) — **репо лаунчера нормативно**;
- назначение `app/contract/` — полная пересборка (`rm -rf` + `cp -R`, `:37-39`), чтобы не оставалось хвостов от удалённых файлов;
- пин `app/contract.lock` (`:54-58`): `source=`, `synced_at=`, `sha256=` — хеш дерева (сортировка `LC_ALL=C`, конкатенация содержимого, `shasum -a 256`).

Текущее состояние: `synced_at=2026-09-16T12:08:33Z`,
`sha256=b90ff80d1d1c073ca30c19cb6186ea64a86aa3406276f049bec3a5c832495911`.

`app/tool/check_contract_lock.dart` — CI-страж: если копия есть, её хеш
обязан совпадать с `contract.lock`, иначе ошибка «копию не правят руками».
Копии нет — **не ошибка**, контрактные тесты просто скипаются.

### 6.2 Кодогенерации из реестра НЕТ

Проверено: в `app/pubspec.yaml` нет `build_runner`, `json_serializable`,
`freezed`; `find app -name '*.g.dart'` (вне `.dart_tool`) — **пусто**.

Аллоулисты в Dart — **рукописные константы** (`kUtlsFingerprints`,
`shadowsocksMethods`, `kHysteria2ObfsTypes`, `kRealityKeyShares`,
`kTlsPassthroughKeys`, `kNaiveTlsPassthroughKeys`). Синхронность держат
**тесты**, а не генератор.

### 6.3 Реестр не бандлится в приложение

`app/pubspec.yaml:52-62`, полный список `assets:` — `wizard_template.json`,
`l10n/ru/`, `l10n/zh/`, `donate.json`, `support.json`,
`warp_endpoints.json`, две иконки. **Ни одной записи `contract/`.**

То есть реестр — артефакт **времени сборки и тестов**; рантайм несёт
рукописные константы.

### 6.4 Кто читает реестр с диска

Только тесты, корень `const _contractRoot = 'contract'` (относительно `app/`):

| Тест | Что читает |
|---|---|
| `test/contract/registry_sync_test.dart:20` | `registry/allowlists.json` → **двусторонний** diff против `kUtlsFingerprints` (`:104-106`), `kHysteria2ObfsTypes` (`:108-110`) — падает и при «нет в реестре», и при «нет в коде» |
| то же, `:65` | `registry/backup_warnings.json` ↔ константы `kWarn*` |
| `test/contract/contract_test.dart:35,56` | корпус `corpus/uri/**/*.uri`; `registry/protocols/*.json` для пропуска чужих `extension`; карта `Type → code` (`:84-140`) — зеркало поля `"dart"` из `warnings.json` |
| `test/contract/template_contract_test.dart:27` | `registry/warnings.json` (коды движка шаблонов) |
| `test/contract/lx_backup_test.dart:36` | `registry/vars.json` |
| `test/contract/body_contract_test.dart:48` | `registry/protocols/*.json` → канонический `scheme` |

Все мягко скипаются при отсутствии копии (`if (!file.existsSync()) return null;`).

Регенерация ожиданий: `cd app && UPDATE_CONTRACT=1 flutter test test/contract/`
— пишет **только** существующие `*.expected.lxbox.json`, новых копий не
создаёт (аудит 25.08 снёс 257 побайтовых дублей, которые глушили расхождения).

### 6.5 Что это значит для секции `body` из ТЗ §4

| Аспект | Вывод |
|---|---|
| Приедет ли секция в дерево LxBox | **Да, механически** — `sync_contract.sh` копирует `contract/` целиком, новые файлы и поля приезжают без правки скрипта |
| Заработает ли она сама | **Нет** — ни кодогенерации, ни чтения реестра в рантайме. Санитайзер по реестру в Dart придётся **написать руками** |
| Узнают ли они о расхождении | **Да, если расширить тесты.** Сегодня двусторонний diff покрывает только `utls_fingerprints` и `hysteria2_obfs`. Чтобы `body` работала как контракт, нужен аналог `registry_sync_test` на новые поля — иначе секция будет лежать мёртвым грузом |
| Точка входа для их стороны | `test/contract/contract_test.dart` (корпус) — главный рычаг: если корпус пополнить кейсами с `warnings[]` по новым кодам, LxBox получит **красный тест** и будет вынужден реализовать правило |
| Риск | Реестр в Dart — **рукописная копия правил**. Пока нет генератора, третий источник правды (Go-санитайзер, Dart-санитайзер, JSON) может разъехаться так же, как разъехались сегодняшние аллоулисты |

**Рекомендация.** В волне «контракт» (ТЗ §2, порядок работ) договориться с
LxBox о двух вещах: (1) расширить `registry_sync_test.dart` на все enum'ы
секции `body` (двусторонний diff, как у fingerprints); (2) пополнить
корпус кейсами на новые коды (`unknown_key`, `type_invalid`,
`alias_shadowed`, `field_conflict`, `tls_field_unsupported_naive`), чтобы
реализация двигалась красными тестами, а не перепиской. Канал —
`contract/TASKS_LXBOX.md` (память `lxbox-cross-session-channel`).

---

## 7. Пункты на решение владельца

Ниже — случаи, где механическое «целевое = LxBox» либо **ломает узел,
который ядро приняло бы**, либо противоречит вердикту ядра. Каждый —
сценарий А/Б с рекомендацией.

### 7.1 `hellorandom` → `random` (есть только в Go)

**А.** Добавить префикс в Dart. **Б.** Оставить как есть (уходит в `chrome`).
→ **Рекомендация: А.** Не вопрос строгости: `HelloRandom` — реальная
Xray-запись, и Dart подменяет запрошенный случайный отпечаток на хромовый
молча. Целевое здесь = вариант Go.

### 7.2 🔴 ECH: посылка «ядро без `with_ech`» неверна

Установлено (см. §2(d)): ECH в ядре **всегда скомпилирован**
(`common/tls/ech.go` под `//go:build go1.24`), а тег `with_ech` намеренно
**отравлен** — сборка с ним не компилируется, потому что ECH переехал в
stdlib. ✅ Прогон: `tls.ech` проходит `check` (**D**). naive его тоже
**читает** (`protocol/naive/outbound.go:142-155`).

Сегодня обе стороны выбрасывают `tls.ech` безусловно, на ложном основании.

**А.** Оставить как есть (снимать всегда) — но переписать обоснование в
реестре на честное («Xray-форма несёт чужой ключ»), признав, что заодно
теряется рабочий нативный ECH.
**Б.** Разделить два случая:
 - URI-параметр `ech=<name>+<resolver>` (Xray-форма) — **снимать** + код
   `ech_ignored`: ключ чужой, handshake умрёт (device-verified §320);
 - `tls.ech{…}` из sing-box-JSON / ручного тела — **пропускать**: это
   валидная конфигурация, ядро её применит.

→ **Рекомендация: Б.** Вариант А сохраняет поведение, но ценой молчаливой
деградации рабочей функции ядра — ровно того, против чего затеян SPEC 131.
Разделение стоит дёшево (источник поля уже известен конвейеру: маппер
знает, пришло ли значение из `uri.query.ech` или из тела) и снимает
единственное место, где обе стороны сознательно режут то, что ядро умеет.

⚠️ Это **самый крупный пересмотр** в документе: он меняет не реализацию, а
посылку, на которой построены записи D-006 / §9.B7 / `tls.json`.

### 7.3 naive: одиночный userinfo в username или password

Go → username, Dart → password. Кросс-эмит читается «наоборот».
**А.** Password (LxBox). **Б.** Username (Go).
→ **Рекомендация: А.** `naive+https://secret@host` — это пароль по
конвенции DuckSoft и hysteria2; вариант Dart согласован с тем, как Go сам
**эмитит** (пароль в user-слот). Но требует одновременной правки обеих
сторон, иначе round-trip сломается сильнее, чем сейчас.

### 7.4 `xtls-rprx-vision-udp443` → переписывание порта на 443

Go переписывает на URI-пути, Dart — на Xray-пути. То есть **ни у кого не
согласовано внутри себя**.
**А.** Не переписывать нигде (алиас разворачивается только в
`flow`+`packet_encoding`). **Б.** Переписывать везде.
→ **Рекомендация: А.** Порт — свойство узла, а не флоу; переписывание
превращает `…:8443` в `…:443` и делает узел недозваниваемым. Суффикс
`-udp443` в Xray означает «UDP/443 идёт напрямую», а не «TCP-порт = 443».
Это **не** «вариант LxBox» механически — это отказ от бага у обоих.

### 7.5 anytls: эвристика мусорного SNI

Go фолбэчит на `server` при `🔒`/отсутствии точки; Dart — только при пустом.
**А.** Эвристика на всех TLS-протоколах (Go). **Б.** Только пустой (Dart).
→ **Рекомендация: А.** ⛔ Вариант LxBox здесь пропускает `server_name:"🔒"`
в конфиг: ядро **примет** (**D**), а TLS-handshake уйдёт с мусорным SNI и
сервер оборвёт. Узел «есть», но мёртв — ровно то, что конвейер должен
предотвращать.

### 7.6 Канон эмита share-URI (`insecure=1` vs `allowInsecure=1`, `upmbps` vs `up_mbps`)

Обе стороны парсят обе формы, ломается только байтовый round-trip и
identity-хеш.
**А.** Единый канон (выбрать по одному имени на параметр). **Б.** Оставить.
→ **Рекомендация: А**, но **отдельной волной** после конвейера: это
чистый канон эмиссии, к безопасности узла отношения не имеет, а трогает
корпус целиком.

### 7.7 TUIC: Dart пишет дефолты (`congestion_control:cubic`, `alpn:["h3"]`)

**А.** Не писать дефолты (Go). **Б.** Писать (Dart).
→ **Рекомендация: А.** ⛔ Против правила «целевое = LxBox». Обоснование:
CANON прямо требует «дефолты не пишутся»; материализованный дефолт ломает
identity-паритет tuic-узлов между платформами и мешает ядру сменить
дефолт в будущем. ТЗ §3.2 это подтверждает: «дефолты ядра не
материализуются».

### 7.8 TUIC `udp_relay_mode`: мусор → `native` (Dart) vs снять+код (Go)

**А.** Снять поле + код (Go). **Б.** Подставить `native` (Dart).
→ **Рекомендация: А.** ⛔ Ядро мусор **принимает** (**C**), значит
подстановка `native` не спасает от падения — она просто скрывает от
пользователя, что подписка просила режим, которого нет. Код честнее.
Симметрично `congestion_control`, где обе стороны уже ставят код.

### 7.9 Пустой пароль: узел живой (Go) vs дроп (Dart) — tuic, anytls

**А.** Дроп (Dart, `field_missing` = error). **Б.** Warning, узел жив (Go).
→ **Рекомендация: А** для anytls/tuic, где пароль обязателен протоколом.
Реестр (`field_missing`) уже назначил severity по Dart. Но отметить: ядро
такой узел **принимает** (**D**) — то есть это решение про UX
(«не показывать заведомо нерабочий узел»), а не про защиту конфига.
Формально это тот случай, когда вариант LxBox строже ядра — владелец уже
выбрал Dart, возражения нет, фиксируем обоснование.

### 7.10 🔴 Shadowsocks: allowlist строже ядра

✅ Прогон: `rc4-md5`, `aes-128-cfb`, `aes-256-cfb` ядро **принимает** (**D**);
обе стороны **дропают узел**. То есть оба клиента выбрасывают рабочие узлы.
**А.** Расширить allowlist legacy-шифрами из `sing-shadowsocks`, дроп
оставить только для по-настоящему неизвестных. **Б.** Оставить (политика
«не поощряем слабые шифры»).
→ **Рекомендация: А с оговоркой.** Дропать рабочий узел — худший исход:
пользователь не получает ни узла, ни объяснения. Предлагаю: принимать
legacy-шифры, но вешать **info-код** `ss_method_legacy` («шифр устарел,
трафик слабо защищён»). Точный список брать из `sing-shadowsocks`
(`CreateMethod`), а не по памяти — как требует память `utls-fp-allowlist`.
Если владелец выберет Б — записать в реестр явным решением, чтобы это
перестало выглядеть багом.

### 7.11 🔴 vmess `security`: allowlist неверен у обоих в обе стороны

Точный набор ядра (`sing-vmess/client.go:44-52`): `auto`, `none`, `zero`,
**`aes-128-cfb`**, `aes-128-gcm`, `chacha20-poly1305`.

Обе стороны:
- **держат `aes-128-ctr`**, которого в ядре нет → ✅ **B**
  `unsupported security type: aes-128-ctr`, **весь конфиг мёртв**;
- **не знают `aes-128-cfb`**, который ядро принимает → узел молча уезжает
  на `auto`, то есть шифр канала не тот, что просила подписка.

**А.** Привести оба к набору ядра (убрать `ctr`, добавить `cfb`).
**Б.** Только убрать `ctr` (минимальная правка).
→ **Рекомендация: А, без вариантов, и первым приоритетом.** Это не
расхождение Go/Dart, а общий баг с фатальным исходом. Список брать из
module cache `sing-vmess`, а не из реестра и не по памяти.

### 7.12 `key_share`: нормализация регистра и код

Go: `trim`+`lower`+код. Dart: без нормализации, молча.
**А.** Вариант Go. **Б.** Вариант Dart.
→ **Рекомендация: А.** ⛔ Против правила. Enum ядра лоуэркейсный, значит
`"Hybrid"` из подписки — валидное намерение в неверном регистре; Dart его
теряет. И молчание здесь особенно дорого: поле не доезжает, узел идёт **не
с тем обменом ключами**, который просила подписка (это уже записано в
реестре как обоснование кода).

### 7.13 Xray `splithttp` (алиас xhttp)

Go читает, Dart — нет (узел остаётся вообще без транспорта).
**А.** Вариант Go. **Б.** Оставить.
→ **Рекомендация: А.** ⛔ Против правила. Вариант Dart не «строже» — он
теряет транспорт молча, и узел идёт plain TCP туда, где сервер ждёт
splithttp. Чистый пробел покрытия.

### 7.14 🔴 xhttp `mode`: не валидирует никто

✅ Прогон: `mode:"garbage"` → **B** `unknown mode: garbage`.
Реестр enum объявляет (`transports.json:101`), но обе стороны отдают
значение ядру.
**А.** Валидировать по реестру, мусор → снять + `xhttp_param_reset`.
**Б.** Оставить ядру.
→ **Рекомендация: А.** Общий пробел, а не расхождение. Инвариант ТЗ §3.2
(«fatal-значение не может остаться в теле») требует гарда.

### 7.15 socks: password-only узел

Go пишет `:pass@`, Dart опускает userinfo → **пароль теряется**.
→ **Рекомендация: вариант Go.** ⛔ Против правила: вариант LxBox молча
теряет кредентиал.

### 7.16 ssh: inline `private_key` в share-URI

Go отказывается эмитить, Dart пишет ключ в query.
**А.** Не эмитить (Go). **Б.** Эмитить (Dart).
→ **Рекомендация: А.** Приватный ключ в URI утечёт через историю, логи и
буфер обмена. Память `secrets-in-state-by-design` разрешает секреты в
локальном state, но share-URI — формат **для передачи**, это другая
граница доверия.

### 7.17 wireguard: MTU по умолчанию для plain WG

Go 1420 (upstream), Dart 1408 (sing-box). Влияет на identity-хеш узлов
без явного `mtu=`.
→ **Рекомендация: 1408 (Dart)** — совпадает с дефолтом самого sing-box,
то есть не материализует чужой дефолт. Для AWG у обоих уже 1280 (память
`awg-mtu-too-high`), расхождения нет.

### 7.18 🔴 wireguard: Dart не валидирует ключи

Go проверяет 32 байта и дропает узел; Dart пропускает мусор в конфиг.
→ **Рекомендация: вариант Go.** ⛔ Против правила. Битый ключ роняет
конфиг целиком; память `wg-handshake-ok-no-data-shared-key` описывает,
насколько дорого диагностируются WG-проблемы. Дроп с кодом — правильный
исход.

### 7.19 Сводка «где целевое ≠ LxBox»

| № | Пункт | Почему не LxBox |
|---|---|---|
| 7.1 | `hellorandom` | Dart подменяет запрошенный `random` на `chrome` |
| 7.2 | ECH | **оба** режут то, что ядро умеет (посылка про `with_ech` ложна) |
| 7.4 | порт при `-udp443` | баг у обоих, целевое — не делать ни там ни там |
| 7.5 | anytls мусорный SNI | Dart пропускает `🔒` в конфиг, узел мёртв |
| 7.7 | TUIC дефолты в эмите | нарушает CANON «дефолты не пишутся» |
| 7.8 | TUIC `udp_relay_mode` | Dart скрывает потерю намерения подписки |
| 7.10 | ss legacy-шифры | **оба** строже ядра — дропают рабочие узлы |
| 7.11 | vmess `security` | **оба** неверны в обе стороны: `ctr` роняет конфиг, `cfb` теряется |
| 7.12 | `key_share` регистр | Dart теряет `"Hybrid"` молча |
| 7.13 | `splithttp` | Dart теряет транспорт молча |
| 7.14 | xhttp `mode` | **оба** не валидируют, ядро даёт B |
| 7.15 | socks password-only | Dart теряет пароль |
| 7.18 | WG-ключи | Dart пропускает мусор, конфиг падает |

Из 13 пунктов **пять** (7.2, 7.4, 7.10, 7.11, 7.14) — не расхождения
Go/Dart, а **общие баги обеих сторон**, вскрытые сверкой с ядром.

Приоритет для первой волны, по тяжести исхода:

| Приоритет | Пункт | Исход сегодня |
|---|---|---|
| 🔴 1 | §7.11 `aes-128-ctr` | **весь конфиг мёртв** на узле из подписки |
| 🔴 2 | §7.14 xhttp `mode` | **весь конфиг мёртв** на мусорном режиме |
| 🟠 3 | §7.10 ss legacy | рабочие узлы выброшены без объяснения |
| 🟠 4 | §7.4 порт `-udp443` | узел недозваниваем |
| 🟡 5 | §7.2 ECH | рабочая функция ядра режется молча |

Первые два не требуют ни конвейера, ни контракта — это правка двух
списков и одного enum'а, и её стоит выпустить раньше SPEC 131.

---

## 8. Открытые вопросы

### 8.1 Проверить запуском

1. **`tls.reality.key_share` на пине 1.14.1-lx.4.** Статика даёт **B**
   (`reality_client.go:89-93`, текст `unknown reality key_share: … (expected
   "hybrid" or "classical")`); прогон на lx.33 дал **A** (`unknown field`),
   т.к. поля ещё нет. Подтвердить на пине — гард обязателен в обоих случаях.
2. **xhttp `seq_placement` / `x_padding_placement` / `x_padding_method`.**
   Dart валидирует, Go нет; вердикт ядра на мусор не установлен. Если
   **C** — гейт Dart полезен, но не критичен; если **A/B** — Go обязан
   догнать (см. §7.14, там же `mode`, где **B** уже подтверждён).
3. **Полный список ss-методов** из `sing-shadowsocks2`
   (`cipher/method_registry.go` — реестр shadowaead / shadowaead_2022 /
   shadowstream / none). Для §7.10 нужен точный перечень, а не выборка из
   трёх подтверждённых (`rc4-md5`, `aes-128-cfb`, `aes-256-cfb`).
4. **`masque.vhttp:"auto"` на пине** — на lx.33 enum-тег отверг `garbage`
   на декоде (**A**); статика ядра говорит **B** с текстом
   `invalid vhttp: … (expected h3, h2 or auto)`, т.е. вердикт зависит от
   версии. Заодно проверить `h2` + `profile:standard` (статика: **B**).
5. **flow `vision` + ws/grpc на живом сервере** (§2(g)). Ядро
   конструирует молча (**C**): транспорт выигрывает
   (`protocol/vless/outbound.go:183-189`), а vision-обёртка всё равно
   применяется поверх не-TLS соединения. Проверить, портит ли это трафик
   или просто не даёт эффекта — от этого зависит severity кода
   `vision_with_transport` (сейчас `info`).
6. **naive + cronet.** На проверяющей машине бинарь падает
   `cronet: library not found` — то есть naive-узел, прошедший все
   проверки опций, всё равно валит `check` без libcronet рядом. Сверить с
   механизмом деградации Go (память `naive-needs-libcronet`,
   `outbound_generator.go:822`): гейт обязан срабатывать **до** попытки
   собрать конфиг.

### 8.2 Закрыто в ходе сбора (пересмотреть решения, где опирались на старое)

| Было | Стало |
|---|---|
| «ядро собрано без `with_ech`, поле уронило бы конфиг» | ECH компилируется **всегда**, тег отравлен; `tls.ech` — **D** → §7.2 |
| «`packet_encoding` мусор = паника ядра (SPEC 049)» | аккуратный фатал **B**; гард по-прежнему обязателен |
| «AWG overlap — ошибка загрузки конфига» | **check проходит**, ошибка в `Endpoint.Start` через IpcSet (`wireguard-go/device/uapi.go:839`) — т.е. **B на старте**, не на check |
| «Go обрезает `short_id` >16 и пропускает нечётную длину» | Go **сбрасывает** обе формы; расхождение с Dart уже закрыто |
| «vmess-набор = 6 значений с `aes-128-ctr`» | ядро: `auto,none,zero,aes-128-cfb,aes-128-gcm,chacha20-poly1305` → §7.11 |
| «REALITY требует chrome» | не требует на конструировании; гейт только у `key_share:hybrid`, и он **на хендшейке** |
| «`mport`/`pinSHA256` — только Go» | Dart читает оба |

### 8.3 Предложить в ядро (вне SPEC 131)

1. **Паника на `short_id` >16 hex** (`reality_client.go:80`) — переставить
   проверку длины перед `hex.Decode`. Правка на три строки, чинит краш
   `sing-box check` для всех пользователей форка.
2. **`tuic.udp_relay_mode` без `default`** (`protocol/tuic/outbound.go:59-63`) —
   мусор молча становится native. Добавить `default: return error` было бы
   честнее, но это ломающее изменение; как минимум — лог.

### 8.4 Требует решения владельца

Все пункты §7 (19 штук), из них приоритетные — **§7.11** (vmess-набор,
роняет конфиг), **§7.14** (xhttp `mode`, роняет конфиг), **§7.2** (ECH,
пересмотр посылки), **§7.10** (ss legacy, теряются рабочие узлы).

---

## 9. Прогоны на пине lx.4 (закрытие §8.1)

Дата: **17.09.2026**. Методика §0.1 без изменений: минимальный конфиг на
случай, `sing-box check -c`, вердикт по шкале A/B/C/D из §0.

**Бинарь:** `sing-box version 1.14.1-lx.4`, revision `900cdb24b7ff4b5eba98e5596786ba77658482ea`
(совпадает с коммитом тега `v1.14.1-lx.4` в `/Users/macbook/projects/sing-box-lx`),
`go1.26.8 darwin/arm64`, теги
`with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_clash_api,with_naive_outbound,with_xhttp,with_awg,with_lx_command,with_lxd,with_openvpn,with_openconnect,with_lx_chain,with_tailscale`.
`with_ech` по-прежнему НЕТ. Это **официальный релизный архив**
`sing-box-1.14.1-lx.4-darwin-arm64.tar.gz`, не локальная пересборка.

Оговорка §0.1 «проверить на пине» снимается для пунктов 1–4 §8.1 и для
бонусного vmess-набора. Пункты §8.1.5 (flow vision на живом сервере) и
§8.1.6 (naive + cronet) прогоном `check` не закрываются и остаются открытыми.

### 9.1 `tls.reality.key_share` — §8.1.1 закрыт

Конфиг: один `vless` + `tls.reality{enabled,public_key,short_id}` +
`utls{enabled,fingerprint:chrome}`.

| Значение | Вердикт | Точный текст |
|---|---|---|
| поле отсутствует | **D** | — (check OK) |
| `"hybrid"` | **D** | — (check OK) |
| `"classical"` | **D** | — (check OK) |
| `"Hybrid"` | **B** | `initialize outbound[0]: unknown reality key_share: Hybrid (expected "hybrid" or "classical")` |
| `"garbage"` | **B** | `initialize outbound[0]: unknown reality key_share: garbage (expected "hybrid" or "classical")` |
| `""` (пустая строка) | **D** | — (check OK), эквивалент отсутствия поля |

**Итог.** Статика §8.1.1 подтверждена: на пине это **B**
(`reality_client.go:89-93`), а не A. Enum **регистрозависим** — `Hybrid`
падает так же, как `garbage`. Пустая строка безопасна (проходит как
дефолт), поэтому санитайзеру достаточно **сбрасывать поле** при
не-allowlist значении, отдельного «пустого» кейса не нужно. Гард
обязателен: на lx.33 то же самое даёт A (`unknown field`), то есть узел с
`key_share` валит конфиг и на старых ядрах, и на новых — разными путями.

### 9.2 xhttp `mode` / `seq_placement` / `x_padding_placement` / `x_padding_method` — §8.1.2 закрыт

Конфиг: один `vless` + `tls{enabled}` + `transport{type:xhttp,path:"/x"}`.

Источник enum'ов в ядре (`transport/v2rayxhttp/meta.go:19-38, 91-159, 268-276`,
тег `v1.14.1-lx.4`):

* placement-константы: `path`, `query`, `header`, `cookie`, `body`, `auto`, `queryInHeader`;
* `session_placement` / `seq_placement` → `path|query|header|cookie` (дефолт `path`);
* `uplink_data_placement` → `body|auto|header|cookie` (дефолт `auto`);
* `x_padding_placement` → `cookie|header|query|queryInHeader` (дефолт `queryInHeader`);
* `x_padding_method` → `repeat-x|tokenish` (дефолт `repeat-x`).

| Поле / значение | Вердикт | Точный текст |
|---|---|---|
| baseline (только `type`+`path`) | **D** | — |
| `mode: "stream-one"` | **D** | — |
| `mode: "garbage"` | **B** | `initialize outbound[0]: create client transport: xhttp: v2ray-xhttp: unknown mode: garbage` |
| `seq_placement: "query"` | **D** | — |
| `seq_placement: "garbage"` | **B** | `… v2ray-xhttp: unsupported seq_placement: garbage` |
| `seq_placement: "Path"` | **B** | `… v2ray-xhttp: unsupported seq_placement: Path` |
| `x_padding_placement: "queryInHeader"` | **D** | — |
| `x_padding_placement: "garbage"` | **B** | `… v2ray-xhttp: unsupported x_padding_placement: garbage` |
| `x_padding_placement: "queryinheader"` | **B** | `… v2ray-xhttp: unsupported x_padding_placement: queryinheader` |
| `x_padding_method: "tokenish"` | **D** | — |
| `x_padding_method: "garbage"` | **B** | `… v2ray-xhttp: unknown x_padding_method: garbage` |
| `x_padding_method: "Repeat-X"` | **B** | `… v2ray-xhttp: unknown x_padding_method: Repeat-X` |

Дополнительно проверено, что валидация **не зависит от режима**:
`mode:"packet-up"` + `seq_placement:"garbage"` → та же **B**, а
`mode:"packet-up"` без extras → **D**. То есть `normalizeMeta` зовётся на
каждом режиме и валидирует весь набор placement'ов, даже те, что в
выбранном режиме не используются (`seq_placement` осмыслен только в
packet-up, но проверяется всегда).

**Итог для §8.1.2 и §7.14.** Ответ — **B, а не C**: гейт Dart не «полезен,
но не критичен», а **обязателен**, и Go обязан догнать. Все четыре поля
ведут себя одинаково: строгий регистрозависимый allowlist, фатал на
инициализации транспорта, весь конфиг мёртв. Пустая строка везде = дефолт
(`orDefault`), так что санитайзеру, как и с `key_share`, достаточно
снимать поле.

Также подтверждено: `x_padding_placement` — **единственный** enum в реестре
с camelCase-значением (`queryInHeader`); нижний регистр отвергается. Любая
«нормализация к lowercase» на этом поле сломает рабочую ноду.

### 9.3 Полный список shadowsocks-методов ядра — §8.1.3 закрыт

Источник: `github.com/sagernet/sing-shadowsocks2 v0.2.1` (из
`git show v1.14.1-lx.4:go.mod`), module cache
`~/go/pkg/mod/github.com/sagernet/sing-shadowsocks2@v0.2.1`. Реестр —
`cipher/method_registry.go` (плоская `map[string]MethodCreator`,
заполняется через `RegisterMethod` из четырёх `init()`), ошибка на промах:
`unknown method: <имя>`. Потребитель в ядре —
`protocol/shadowsocks/outbound.go:43` (`shadowsocks.CreateMethod`).

Точный перечень (**18 строк**, четыре источника регистрации):

| Источник | `MethodList` |
|---|---|
| `cipher/method_none.go:15` | `none` |
| `shadowaead/method.go:24-29` | `aes-128-gcm`, `aes-192-gcm`, `aes-256-gcm`, `chacha20-ietf-poly1305`, `xchacha20-ietf-poly1305` |
| `shadowaead_2022/method.go:33-36` | `2022-blake3-aes-128-gcm`, `2022-blake3-aes-256-gcm`, `2022-blake3-chacha20-poly1305` |
| `shadowstream/method.go:24-33` | `aes-128-ctr`, `aes-192-ctr`, `aes-256-ctr`, `aes-128-cfb`, `aes-192-cfb`, `aes-256-cfb`, `rc4-md5`, `chacha20-ietf`, `xchacha20` |

Все 18 прогнаны на пине — **все D** (check OK). Контроль:
`"garbage"` → **B** `initialize outbound[0]: unknown method: garbage`;
`"AES-128-GCM"` → **B** `unknown method: AES-128-GCM` (реестр
регистрозависим); `""` → **B** `unknown method: ` (пустое значение
не имеет дефолта, в отличие от enum'ов §9.1/§9.2).

Сверка с `contract/registry/allowlists.json` → `allowlists.ss_methods.values`
(9 значений) и зеркалом в Go `node_parser_ss.go:6-21`:

| Метод | в ядре lx.4 | в allowlist | Вердикт |
|---|---|---|---|
| `none` | ✅ D | ✅ | ✅ совпало |
| `aes-128-gcm` | ✅ D | ✅ | ✅ совпало |
| `aes-192-gcm` | ✅ D | ✅ | ✅ совпало |
| `aes-256-gcm` | ✅ D | ✅ | ✅ совпало |
| `chacha20-ietf-poly1305` | ✅ D | ✅ | ✅ совпало |
| `xchacha20-ietf-poly1305` | ✅ D | ✅ | ✅ совпало |
| `2022-blake3-aes-128-gcm` | ✅ D | ✅ | ✅ совпало |
| `2022-blake3-aes-256-gcm` | ✅ D | ✅ | ✅ совпало |
| `2022-blake3-chacha20-poly1305` | ✅ D | ✅ | ✅ совпало |
| `aes-128-ctr` | ✅ D | ❌ | 🔴 **рабочий узел дропается** |
| `aes-192-ctr` | ✅ D | ❌ | 🔴 **рабочий узел дропается** |
| `aes-256-ctr` | ✅ D | ❌ | 🔴 **рабочий узел дропается** |
| `aes-128-cfb` | ✅ D | ❌ | 🔴 **рабочий узел дропается** |
| `aes-192-cfb` | ✅ D | ❌ | 🔴 **рабочий узел дропается** |
| `aes-256-cfb` | ✅ D | ❌ | 🔴 **рабочий узел дропается** |
| `rc4-md5` | ✅ D | ❌ | 🔴 **рабочий узел дропается** |
| `chacha20-ietf` | ✅ D | ❌ | 🔴 **рабочий узел дропается** |
| `xchacha20` | ✅ D | ❌ | 🔴 **рабочий узел дропается** |

**Итог для §7.10.** Выборка из трёх подтверждённых (`rc4-md5`,
`aes-128-cfb`, `aes-256-cfb`) заменена полным перечнем: недостающих
**девять**, все из `shadowstream`. Оба проекта (Go и Dart) держат
идентичный allowlist из 9 AEAD/2022-значений и молча теряют весь
legacy-stream-набор, который ядро на пине принимает без единого warning'а.
Решение по расширению — за владельцем (§7.10); данные для решения теперь
полные.

### 9.4 `masque` `vhttp` / `profile` — §8.1.4 закрыт

**Сначала поправка к постановке §8.1.4.** `masque` — это **outbound**, а не
endpoint: регистрация `masque.RegisterOutbound(registry)` в
`include/quic.go:29`. В `endpoints[]` он не существует ни на одной версии —
конфиг с `endpoints[0].type = "masque"` падает **A** на декоде:

```
decode config at …: endpoints[0]: unknown endpoint type: masque
```

Дальнейшие прогоны — в `outbounds[]`. Конфиг: `type:masque`, `server`,
`server_port`, `private_key`/`public_key` (base64 DER EC P-256),
`ip`/`ipv6`, `tls{enabled,server_name}`.

| Случай | Вердикт | Точный текст |
|---|---|---|
| baseline (без `vhttp`) | **D** | — |
| `vhttp: "auto"` | **D** | — |
| `vhttp: "h3"` | **D** | — |
| `vhttp: "h2"` | **D** | — |
| `vhttp: ""` | **D** | — (дефолт) |
| `vhttp: "garbage"` | **B** | `initialize outbound[0]: masque: invalid vhttp: garbage (expected h3, h2 or auto)` |
| `vhttp: "Auto"` | **B** | `initialize outbound[0]: masque: invalid vhttp: Auto (expected h3, h2 or auto)` |
| `profile: "cloudflare"` | **D** | — |
| `profile: "garbage"` | **B** | `initialize outbound[0]: masque: unknown profile: garbage` |
| `vhttp:"h2"` + `profile:"standard"` | **B** | `initialize outbound[0]: masque: vhttp h2 is not implemented for the standard profile` |
| `vhttp:"h3"` + `profile:"standard"` | **B** | `initialize outbound[0]: masque: uri is required for the standard profile` |
| `vhttp:"auto"` + `profile:"standard"` | **B** | `initialize outbound[0]: masque: uri is required for the standard profile` |

**Итог.** Гипотеза §8.1.4 о зависимости вердикта от версии **подтверждена**:
на lx.33 мусорный `vhttp` ловился enum-тегом на декоде (**A**), на пине
lx.4 — конструктором (**B**). Поле по-прежнему несёт
`enum:"h3,h2,auto"` в `option/masque.go`, но на пине до enum-проверки
декодера дело не доходит — фатал приходит из конструктора outbound'а.
Для санитайзера разницы нет: и A, и B валят весь конфиг, гард обязателен.

Отдельно: `vhttp:"h2"` + `profile:"standard"` — **B**, как и предсказывала
статика. Но и `h3`/`auto` на `standard` без `uri` тоже **B**: на
standard-профиле `uri` обязателен. То есть `profile:"standard"` без `uri`
валит конфиг при **любом** `vhttp` — это более широкая ловушка, чем
записанная в §8.1.4 пара «h2 + standard», и её стоит учесть в реестре.

### 9.5 vmess `security` — бонус, §8.2 подтверждён на пине

Конфиг: один `vmess` + `uuid`, без TLS и транспорта.

| Значение | Вердикт | Точный текст |
|---|---|---|
| `auto` | **D** | — |
| `none` | **D** | — |
| `zero` | **D** | — |
| `aes-128-cfb` | **D** | — |
| `aes-128-gcm` | **D** | — |
| `chacha20-poly1305` | **D** | — |
| `""` | **D** | — (дефолт) |
| `aes-128-ctr` | **B** | `initialize outbound[0]: vmess: unsupported security type: aes-128-ctr` |
| `garbage` | **B** | `initialize outbound[0]: vmess: unsupported security type: garbage` |

**Итог.** Набор ядра из §8.2 подтверждён на пине дословно:
`auto, none, zero, aes-128-cfb, aes-128-gcm, chacha20-poly1305`.
`aes-128-ctr` → **B**, `aes-128-cfb` → **D**. Расхождение §7.11 (оба
проекта держат несуществующий `aes-128-ctr` и теряют рабочий
`aes-128-cfb`) остаётся в силе на пине.

### 9.6 Что осталось открытым в §8.1

* **§8.1.5** (flow `vision` + ws/grpc на живом сервере) — `check`
  конструирует молча, вердикт **C** подтверждён и на пине; вопрос «портит
  ли трафик» требует живого сервера, не `check`.
* **§8.1.6** (naive + cronet) — требует прогона с/без `libcronet.*` рядом с
  бинарём; не проверялось, гейт Go не сверялся.
