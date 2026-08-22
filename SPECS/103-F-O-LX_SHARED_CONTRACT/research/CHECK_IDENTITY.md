# Сравнение алгоритмов идентичности нод: singbox-launcher (Go) vs LxBox (Dart)

Проверено не только чтением кода, но и **эмпирическим кросс-прогоном**: одинаковые share-URI (14 форм нод) прогнаны через `subscription.ParseNode` → `config.NodeIdentityHash` (Go) и `parseUri` → `nodeIdentityHash` (Dart, через flutter test), сравнивались канонические байты до хеша и сами хеши.

## 1. Go `NodeIdentityHash` (`/Users/macbook/projects/singbox-launcher/core/config/node_hash.go`)

**Вход:** `*ParsedNode` (= `configtypes.ParsedNode`). Хеш считается не от полей структуры, а от **эмитированного sing-box outbound JSON**:
- `Scheme == "wireguard"` → `GenerateEndpointJSON(node)` (SPEC 101, иначе per-scheme switch урезал бы WG до `{tag,type,server,server_port}`);
- всё остальное → `GenerateNodeJSON(node)` (группы и `EmitRaw`/config_json — свои ветки внутри).

**Пайплайн:**
1. `decodeEmittedOutbound`: из фрагмента `"\t// <label>\n\t{...},"` берётся всё от первой `{`, срезается хвостовая `,`, `json.Unmarshal` в `map[string]interface{}` (комментарий-лейбл в хеш не попадает).
2. Удаляются ровно два поля: `tag`, `detour` (`nodeHashIgnoredFields`). Всё остальное входит: server, server_port, креды, `tls` целиком (server_name/utls/reality/alpn/insecure), `transport`, flow, obfs и т.д.
3. `marshalCanonicalJSON`: рекурсивная сортировка ключей всех вложенных map (`canonicalizeJSONValue`), порядок элементов списков **сохраняется** (alpn, allowed_ips значимы), затем `json.Marshal` (компактный, без пробелов).
4. `sha256.Sum256` → `hex.EncodeToString` → **64 символа lowercase hex, без префикса**.

**Отказ:** невозможность эмиссии/декода → `""`; вызывающие обязаны трактовать как «нет идентичности» и пропускать дедуп.

## 2. Dart `nodeIdentityHash` (`/Users/macbook/projects/LxBox/app/lib/services/node_hash.dart`)

**Вход:** `NodeSpec`. Формула: `sha256( jsonEncode( deepSortKeys( emit(TemplateVars.empty).map − 'tag' − 'detour' ) ) )`.
1. `node.emit(TemplateVars.empty).map` — канонический sing-box-map узла (vars на emit не влияет).
2. `..remove('tag')..remove('detour')` — те же два исключения.
3. `deepSortKeys` — рекурсивная сортировка ключей Map (ключи через `toString()`), списки в исходном порядке.
4. `jsonEncode` (компактный) → `utf8.encode` → `sha256.convert(...).toString()` → **64 символа lowercase hex, без префикса**.

Скелет алгоритма идентичен Go до буквы.

## 3. Байт-идентичность: **НЕТ (совпадает часто, но не всегда)**

Эмпирика: **10 из 14 форм дали байт-идентичный канонический JSON и одинаковый хеш** — vless+reality(+vision), vless+ws+tls (без early data), vless+grpc (оба дефолтят `fp=random`), trojan, ss, hysteria2 (голый и с obfs), vmess+ws+tls, tuic (+alpn), socks5, ssh. Совпали и escaping UTF-8, и числа (Go после float64-roundtrip эмитит `443`, как Dart int).

Найденные расхождения (каждое даёт разные хеши одной ноды):

| # | Ситуация | Go | Dart |
|---|----------|----|------|
| 1 | **HTML-escaping строк** (`<`, `>`, `&` в пароле/SNI/path) | `json.Marshal` экранирует: `p\u003c\u0026\u003ez` | `jsonEncode` не экранирует: `p<&>z` |
| 2 | **ws early data `?ed=N` без `eh`** (issue #96 / §303) | эмитит `max_early_data` **и** `early_data_header_name:"Sec-WebSocket-Protocol"` | только `max_early_data` (режим «ed в пути»; `early_data_header_name` лишь при явном `eh`) — `transport.dart:44-53` |
| 3 | **anytls без `fp=` в URI** | `utls` не эмитится вовсе (`node_parser_anytls.go:71-80` — только при явном fp) | TLS через VLESS-конвенцию → `parseVlessTls` дефолтит `fp='random'` (`transport.dart:333`) → `utls:{enabled,fingerprint:"random"}` |
| 4 | **wireguard (все WG-ноды)** | endpoint-JSON содержит `"name"` (дефолт `"singbox-wg0"`, `node_parser_wireguard.go:136-138`) и `"system":false` | ни `name`, ни `system` не эмитятся |

Теоретический (не проверялся): Go экранирует U+2028/U+2029, Dart — нет.

Прочие слои эквивалентны: одинаковый набор исключаемых полей (tag/detour), одинаковая рекурсивная сортировка ключей с сохранением порядка списков, одинаковый компактный JSON без пробелов, одинаковый hex-формат. Порт как `server_port` — число в обоих.

## 4. Dart `nodeIdentityKey` и аналог в Go

**Dart** (`/Users/macbook/projects/LxBox/app/lib/services/node_identity.dart`): строковый ключ `protocol|server|port|credential`, **без транспорта и TLS** (решение юзера 30.07.2026 — один сервер с двумя SNI = один узел). `null` для групп/безадресных. `nodeIdentityKey` учитывает §302-патч (`patchedJson`), `nodeIdentityKeyRaw` — как приехало. Credential per-протокол: uuid (vless/vmess/tuic), password (trojan/ss/hy2/anytls/naive), username (socks/http), user (ssh), privateKey (wg), privateKeyDer (masque).

Использования:
- `services/parser/json_parsers.dart:479 _xrayIdentity` — тот же ключ по сырому Xray-JSON (инвариант: посимвольное совпадение с `nodeIdentityKey`); дедуп/владение серверами при парсе Xray-массива (§321 P4, строки 124, 215);
- `services/builder/server_list_build.dart:172,190` — резолв состава пула автовыбора на билде (§322);
- `controllers/subscription_controller.dart:291-292` — ремап синонимов «было→стало» при §302-патче (`Raw` → патченный);
- `screens/folder_detail_screen.dart:828` — UI-выбор членов пула.

**Go: аналога НЕТ — сознательно.** Комментарий в `node_hash.go:14-25`: «Отдельного строкового ключа `scheme|server|port|credential` намеренно нет — два механизма с разной строгостью разошлись бы в поведении»; и там же зафиксировано отличие от LxBox: в Go два SNI одного сервера **не схлопываются**. Всё, что LxBox делает ключом, Go делает хешем: дедуп в пределах источника (`source_loader.go: dedupNodesByIdentity`), владение серверами между элементами Xray-массива (`xray_json_array.go: computeXrayIdentityOwners`/`filterByIdentityOwner`), ссылки групп на членов (`rememberGroupMemberIdentities`, `finalTagByIdentity`), привязка detour-ноды (`ProxySource.DetourNodeHash`, SPEC 101), disabled-ноды.

**Следствие для переносимости:** семантика дедупа разная by design — нода, схлопнутая в LxBox по ключу (другой SNI), в лаунчере останется отдельной, и наоборот.

## 5. Хранение disabled-нод

| | Go singbox-launcher | Dart LxBox |
|---|---|---|
| Контейнер | `ProxySource.DisabledNodes` (`core/config/configtypes/types.go:130+`, в parser config / state) | `SubscriptionServers.disabledHashes` (`app/lib/models/server_list.dart:171`, в server_list) |
| JSON-ключ | **`disabled_nodes`** | **`disabled_hashes`** |
| Ключ карты | identity-хеш (hex, 64) | identity-хеш (hex, 64) |
| Значение | **int64 unix seconds** (`time.Now().Unix()`, `source_edit_window.go:156`) — lastSeen | **ISO-8601 строка** `DateTime.toIso8601String()` (локальное время, без TZ-суффикса); при парсе битые значения молча скипаются (`_disabledHashesFromJson`) |
| TTL | `clamp(3×intervalHours, 24h, 30d)`; interval≤0 → 24h (`source_loader.go: disabledNodeTTL`) | идентично: `clamp(3×interval, 24h, 720h)` (`node_hash.dart: disabledHashTtl`) |
| Продление lastSeen | `filterDisabledNodes` на парсе (нода в теле → `now.Unix()`), персист решает вызывающий | `gcDisabledHashes`: хеш в свежем теле → `now` |
| GC | только после успешного **сетевого** обновления (`config_service_subscriptions.go:94-113`); interval-fallback на `Meta.ProfileUpdateIntervalHours` | только успешный сетевой refresh (`subscription_controller.dart:2024-2028`); **file:-подписки GC не проходят вовсе** (guard в `_fetchEntryByRef`); fallback на meta-интервал нет |
| Поверх GC | — | §332 `applyRuleMarks`: ENABLE-правило снимает отметку, DISABLE ставит с `now` |

**Вывод для бэкапа между устройствами:** прямой перенос отметок сейчас невозможен по трём независимым причинам: (а) разные JSON-ключи и контейнеры (`disabled_nodes` в ProxySource vs `disabled_hashes` в SubscriptionServers); (б) несовместимый формат значения — unix int в Go vs ISO-строка в Dart, причём Dart-парсер молча выкинет unix-числа, а Go ISO-строку не распарсит в int64; (в) сами хеши расходятся для четырёх классов нод: любые строки с `<>&`, ws с `?ed=N` без `eh`, anytls без явного `fp`, и **все wireguard-ноды** (Go подмешивает `name`/`system`). Для остальных распространённых форм (vless/vmess/trojan/ss/hy2/tuic/socks/ssh/anytls-с-fp) хеши уже байт-идентичны.