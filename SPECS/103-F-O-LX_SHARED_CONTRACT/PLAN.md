# PLAN 103 — LX Shared Contract

Детальный план. Статус: **утверждён пользователем 2026-08-19** («в целом мне
нравится, я могу это утвердить, давай сделаем»), рекомендации таблицы §9 приняты
как рабочие решения (протокол — DECISIONS.md). Программа расширена целями 7–11
(SPEC.md): каналы в лаунчере (SPEC 104), DNS-группы (SPEC 105), ревью архитектуры
по ходу, протоколирование решений.

## 0. Принятые решения (зафиксированы пользователем)

1. Языконезависимое JSON-представление ноды — да, с учётом разделения
   outbound / endpoint (WireGuard/AWG — endpoints начиная с sing-box 1.11).
2. Никакого Go-парсера на мобиле (gomobile/FFI-мост отвергнут). Реализации две,
   общими делаются: форматы данных, блок-схемы, тесты.
3. Шаблоны унифицируются на уровне JSON-структур, не кодовой базы.
4. Конечная цель — безболезненный перенос правил и бэкапов телефон ↔ десктоп;
   приём, придуманный в одном проекте, работает в другом идентично.

## 1. Архитектура: каталог `contract/`

Канонический дом контракта — **репозиторий лаунчера**, каталог `contract/` в корне
(фаза 5 — вынос в отдельный репо `lx-contract` без изменения содержимого).
LxBox получает копию скриптом `app/tool/sync_contract.sh` (паттерн уже обкатан
на `libbox.version` + `fetch-libbox.sh`): пин = `app/contract.lock` (git ref +
sha256 дерева), CI мобилки проверяет соответствие пина.

```
contract/
  VERSION                     # semver контракта
  README.md                   # как читать, как гонять раннеры, правила PR
  docs/
    CANON.md                  # правила канонизации узла (нормативно)
    IDENTITY.md               # алгоритм identity-ключа/хеша (нормативно, как есть)
    TEMPLATE_LANG.md          # язык шаблонов v1 (нормативно)
    BACKUP.md                 # семантика LX Backup v1 (нормативно)
  registry/
    protocols/<scheme>.json   # реестр протоколов: по файлу на схему
    transports.json           # общая суб-схема транспортов (ws/grpc/http/httpupgrade/xhttp)
    tls.json                  # общие TLS-параметры URI (sni/alpn/fp/reality/ech-политика)
    allowlists.json           # канонические allowlist'ы (uTLS fp, ss-методы, …)
    containers.json           # контейнеры тел (vpn://, wgconf INI, base64)
    warnings.json             # коды warning'ов (локализация — на стороне приложений)
    limits.json               # MaxURILength, cap нод и т.д.
    vars.json                 # реестр переносимых имён template-переменных
    presets.json              # реестр общих preset id
  schema/                     # JSON Schema (draft 2020-12)
    node.schema.json          # канонический узел (envelope, §2)
    backup.schema.json        # LX Backup v1 (§7)
    registry.schema.json      # схема самого реестра
  diagrams/                   # нормативные mermaid-схемы
    body_classify.mmd         # классификация тела подписки
    parse_pipeline.mmd        # тело/URI → канонические узлы
    emit_pipeline.mmd         # узел → outbound/endpoint JSON и share-URI
  corpus/                     # golden-фикстуры (§5)
    uri/<scheme>/<case>.uri            + <case>.expected.json
    body/<kind>/<case>.body            [+ <case>.headers.json] + <case>.expected.json
    emit/<scheme>/<case>.entry.json    + <case>.expected.uri
    template/<case>/template.json + vars.json + expected.json
    backup/<case>/backup.json + expected.import.launcher.json + expected.import.lxbox.json
```

Принцип: **контракт — это данные и документы, ни строки исполняемого кода.**
Раннеры живут в проектах и читают корпус.

## 2. Канонический узел (`node.schema.json`)

Форма `expected.json` для uri/body-фикстур и общий язык обсуждения нод:

```json
{
  "v": 1,
  "nodes": [
    {
      "kind": "outbound",              // "outbound" | "endpoint" | "group"
      "scheme": "vless",               // имя из registry/protocols.json
      "label": "Мой сервер",           // имя ДО tag-политики (prefix/mask/uniquify — постобработка вне контракта)
      "entry": { "type": "vless", "server": "…", "server_port": 443 },
      "chain": [ { "…": "…" } ],       // detour-цепочка, ближний хоп первым (опц., cap 8)
      "warnings": ["utls_fp_unknown"]  // коды из registry/warnings.json (опц.)
    }
  ],
  "dropped": [ { "ref": "<строка или индекс входа>", "reason": "invalid_port" } ],
  "meta": { "profile_title": "…" }     // только для body-фикстур (опц.)
}
```

Правила канонизации (`CANON.md`):
- `entry` — sing-box map **без** `tag`; ключи отсортированы; поля с дефолтными
  значениями не пишутся; числа — числами.
- `kind:"endpoint"` — wireguard/awg (и будущие endpoint-протоколы); `kind:"group"` —
  selector/urltest из импорта sing-box/Xray-балансировщиков, члены группы — по `label`.
- Порядок `nodes` нормативен (включая правила ownership/порядка для Xray-массивов).
- Деградация — по философии обоих проектов: битое значение → чинится/выкидывается
  с warning-кодом, нода живёт; битая нода → в `dropped`, подписка живёт.

## 3. Реестр протоколов (`registry/protocols/<scheme>.json`)

Единое «поле протоколов»: по файлу на схему (плюс общие `transports.json`,
`tls.json`, `allowlists.json`). Иллюстрация формы (актуальная — в
`schema/registry.schema.json`):

```json
{
  "v": 1,
  "schemes": {
    "vless":     { "kind": "outbound", "singbox_type": "vless", "aliases": [],
                   "sources": ["uri", "singbox", "xray"],
                   "uri": { "userinfo": "uuid", "query": {
                       "flow":     { "type": "enum", "values": ["", "xtls-rprx-vision"] },
                       "fp":       { "type": "alias_enum", "allowlist": "utls_fingerprints" },
                       "pbk":      { "type": "reality_pubkey" },
                       "ed":       { "type": "int", "note": "ws early data, также хвост path ?ed=N" }
                   } },
                   "emit": { "share_uri": true, "param_order": ["type", "security", "…"] } },
    "wireguard": { "kind": "endpoint", "aliases": ["wg", "awg"], "…": "…" },
    "…": {}
  },
  "allowlists": {
    "utls_fingerprints": ["chrome", "firefox", "…"],
    "ss_methods": ["…"], "tuic_congestion": ["…"],
    "hysteria2_obfs": ["salamander", "gecko"],
    "packet_encoding": ["", "packetaddr", "xudp"]
  }
}
```

- Поле/схема, которую один проект поддерживает намеренно, а второй пока нет,
  помечается `"extension": "desktop" | "mobile"` — цель, чтобы список расширений
  стремился к пустому; каждое расширение = строка в таблице расхождений (§9).
- **Сверочные тесты вместо кодогенерации**: Go-тест сверяет список схем
  (`IsDirectLink`) и свои allowlist-таблицы с реестром; Dart-тест — свой `parseUri`
  switch, `kUtlsFingerprints` и т.д. Дёшево и не даёт реестру протухнуть.

## 4. Блок-схемы (`diagrams/*.mmd`)

Нормативный алгоритм классификации тела (объединение Go `BodyKind` и Dart
`DecodedBody`/`JsonFlavor` — сегодня они чуть разные, см. §9):

1. trim / UTF-8-проверка →
2. `vpn://…` → Amnezia-контейнер (base64url + qCompress/zlib, анти-bomb cap) →
3. попытка base64 (все 4 варианта кодировки) → рекурсивно к шагу 4 →
4. `{`/`[` → JSON-флейвор: Xray-массив (`outbounds[].protocol`) | sing-box
   outbound / массив / полный конфиг (`outbounds`/`endpoints`) / массив конфигов |
   Clash YAML-детект (`proxies`) → подсказка, 0 узлов →
5. `[Interface]`+`[Peer]` → wgconf INI (в Go-классификаторе тел этой ветки
   сегодня нет — wgconf принимается только через UI-paste; выравнивание — §9.B11) →
6. построчный URI-list (скип `#`, `//`, `;`) →
7. пусто → диагностика (HTML-страница / plain error / полный конфиг с inbounds).

Каждый узел схемы ссылается на фикстуры `corpus/body/`. Схемы parse/emit-пайплайнов —
аналогично, с точками «identity», «dedup», «tag-политика» помеченными как
«вне контракта» (постобработка).

## 5. Конформанс-корпус и раннеры

Форматы фикстур — §1. Источники наполнения: существующие `app/test/fixtures/**`
LxBox (33 файла, уже в нужной структуре), inline-кейсы из 36 Go `*_test.go` и
25 Dart-тестов, включая всю «боевую» коллекцию: `?ed=N`, uTLS-мусор, reality
`pbk=enabled`, short_id нечётной длины, hysteria2 mport/gecko, AWG ranged headers +
MTU clamp, WARP reserved, XHTTP полный набор + `extra`, двойное percent-кодирование,
ECH-игнор, vmess legacy, userinfo с пробелом и т.д.

- **Go-раннер**: новый `core/config/subscription/contract_test.go` + канонизатор
  `contract_canon_test.go` (ParsedNode → envelope; оба — `*_test.go`, чтобы
  Win7-джоба на go1.20 их не компилировала). body-кейсы обязаны входить через
  decode-слой (`DecodeSubscriptionContent` → classify), а не через кэш-хук
  `LookupCachedBody` — иначе фикстура B4 будет зелёной при живом баге на
  боевом fetch-пути.
- **Dart-раннер**: `app/test/contract/contract_test.dart` — это ровно «Фаза 2»,
  которую LxBox сам себе задекларировал в `test/fixtures/README.md`
  (`.expected.json` там задуманы, но не реализованы).
- **Cross-emit**: `emit/`-фикстуры проверяют «URI, который эмитит проект A,
  парсится проектом B в тот же канонический узел» — обе стороны гоняют оба
  направления.
- **Ярусы строгости**: `nodes` сверяются строго с фазы 1; `warnings` — строго
  у Dart сразу, у Go — после появления структурированного накопителя warning'ов
  (фаза 2); до того Go игнорирует поле.
- **Per-app override**: для задокументированных by-design различий (сегодня
  единственное — схлопывание нод одного сервера с разными SNI по
  `nodeIdentityKey` в LxBox, IDENTITY.md) кейс может нести
  `<case>.expected.lxbox.json` / `<case>.expected.launcher.json` поверх общего
  expected. Каждый override обязан ссылаться на пункт IDENTITY.md/реестра;
  бесхозный override — ошибка линтера корпуса.
- У раннеров есть `-update`/`--update` для перегенерации expected, но expected —
  нормативные: перегенерация = осознанный PR в контракт с ревью диффа.
- Вне корпуса (намеренно): dedup-хуки, tag-префиксация, disabled-TTL, авто-группы,
  каналы — это постобработка, завязанная на инфраструктуру приложений; она
  покрывается нативными тестами и (для переносимого) фикстурами backup.

## 6. Язык шаблонов v1 (`docs/TEMPLATE_LANG.md`)

База уже общая: `#if`-движок LxBox заимствован из SPEC 067 лаунчера (комментарий
в `if_engine.dart` прямо это фиксирует), SPEC 090 назвал формат «shared». План —
превратить «заимствован» в «специфицирован»:

- **Ядро (обязаны оба)**: объявления `vars` (name/type/default/options), подстановка
  `"@var"`, coercion **по объявленному `type`** (`bool`/`int`/`text`/`text_list`) —
  а не по хардкод-списку имён ключей (сегодняшний Go-подход; лаунчер мигрирует, §9),
  конструкция `#if` (полная таблица предикатов — выравнивается по сверке §9):
  map-spread, array-element, `else`, вложенность, семантика Dropped, политика
  unresolved var (обычный режим и strict для пресетов).
- **Пресеты (ядро)**: единая схема `{id, label, default_enabled, vars, rule_set,
  rules[], dns_rule(s), dns_servers}` + неймспейс локальных тегов
  `<preset_id>:<tag>`; `outbounds(add/update)` — desktop-расширение (в Dart его
  нет, переносимость preset-ссылок не страдает: тело пресета живёт в шаблоне
  каждого приложения, бэкап несёт только ref+vars). Имя типа переменной
  канонизируется как `dns_servers` (`dns_server` в Go — алиас). `registry/presets.json` —
  реестр общих id (нужен переносимости preset-ссылок в бэкапе:
  `{kind:"preset", ref:"<id>"}` обязан значить одно и то же на обоих устройствах).
- **Расширения desktop**: `params[]` (dot-notation патчи), `@runtime.platform/arch/target`,
  секретные переменные, per-platform default_value.
- **Расширения mobile**: `group_templates`/`default_channels`, `sections[].vars`,
  `ping_options`/`speed_test_options`, локализационный overlay.
- **Правило толерантности**: неизвестные top-level секции шаблона и неизвестные
  поля объявления переменной движок обязан игнорировать — тогда шаблоны и куски
  шаблонов можно переносить между проектами.
- Конформанс: `corpus/template/` — «шаблон + значения переменных → итоговый JSON
  после движка» (без вставки нод), гоняют оба движка.

## 7. LX Backup v1 (`schema/backup.schema.json`, `docs/BACKUP.md`)

Хранилища остаются свои (`state.json` v6 / `lxbox_settings.json`); унифицируется
**обменный формат**:

```json
{
  "lx_backup": 1,
  "exported_by": { "app": "launcher", "version": "1.4.2", "platform": "darwin" },
  "exported_at": "2026-08-18T20:00:00Z",
  "subscriptions": [ {
      "url": "https://…", "label": "…",
      "tag": { "prefix": "", "postfix": "", "mask": "" },
      "update": { "interval_hours": 12, "auto": true },
      "disabled": { "<identity-hash>": 1755550000 },
      "detour": { "…": "…" }, "max_nodes": 0, "skip": false,
      "extensions": { "lxbox": { "import_rules": ["…"], "identity_override": {} } }
  } ],
  "servers": [ { "uri": "vless://…", "config_json": null, "label": "…", "enabled": true } ],
  "rules":   [ { "kind": "inline|srs|preset|json", "num": 100, "…": "…" } ],
  "dns":     { "servers": [ "…" ], "rules": [ "…" ], "final": "…", "strategy": "…" },
  "vars":    { "log_level": "warn" },
  "route":   { "final": "…" },
  "warp":    [ { "type": "wg|masque", "…": "…" } ],
  "extensions": { "launcher": { "…": "…" }, "lxbox": { "…": "…" } }
}
```

Семантика (конверт и процедуры — эволюция уже работающего бэкапа LxBox §040:
`{app, kind, created_at, source_app_version, storage}` + категорийные тумблеры,
preview с merge/replace и default-deny allowlist §159; лаунчер сегодня файлового
экспорта/импорта не имеет вовсе — только снапшоты `wizard_states/*.json`):
- **Инвариант lossless round-trip**: `import(export(x))` в том же приложении = `x`;
  непереносимое (каналы LxBox, папки, remote-таргеты лаунчера, tun_apps,
  native_prefs, per-subscription import_rules/identity_override) едет в
  `extensions.<app>` (top-level или внутри сущности) и переживает круг через
  другое устройство нетронутым. Для этого каждый импортёр обязан **сохранять
  чужой блоб**: лаунчер — новое поле в state v6, LxBox — новый ключ в allowlist
  §159 (отдельные задачи фазы 4; без них инвариант противоречит default-deny).
- **Импорт по allowlist** (обобщение LxBox §159, default-deny): неизвестные ключи —
  в предупреждение, не в состояние. Исключение одно — opaque-блоб `extensions`.
- **Символические ссылки на outbound-цели**: `rules[].outbound` и `route.final`
  переносятся как есть; если целевой тег в принимающем приложении не существует
  (например, канал `vpn-3` LxBox на десктопе) — правило импортируется
  **выключенным с warning**, `route.final` не применяется с warning (та же
  политика, что для неизвестного preset id). Молчаливой потери нет.
- `vars` — только имена из `registry/vars.json` (пересечение шаблонов); остальное —
  в `extensions`.
- Правило с `kind:"preset"` и id вне `registry/presets.json` — импортируется
  выключенным с warning (не теряется).
- `rules[].num` — общая ось порядка (модель §370 LxBox); импортёр пересчитывает
  свою нумерацию, сохраняя относительный порядок.
- `disabled` переносится **только по identity-хешу**, значения — unix seconds
  (импортёры конвертируют в свой нативный формат: Go int64 / Dart ISO-8601).
  Кросс-прогон показал: хеши уже байт-идентичны для 10 из 14 форм нод;
  4 расхождения (§9.A) закрываются выравниванием эмиттеров до фазы 4.
  Честные ограничения (документируются в BACKUP.md): отметки нод, изменённых
  import-rules §302 на мобиле (хеш считается от `patchedJson`), и нод,
  схлопнутых по SNI, на другой стороне не совпадут — такие отметки просто
  истекают по TTL, механизм пересопоставления в v1 не строится.
- Секреты в бэкапе — by design открыты (граница доверия — локальная машина,
  как в state.json); шифрование архива — вне scope v1.
- UI: у лаунчера — экспорт/импорт файла в конфигураторе; у LxBox — в существующий
  backup-экран добавляется формат «LX Backup» рядом с нативным.

## 8. Процесс («контракт раньше кода»)

- Поправка в CONSTITUTION обоих репо: новый протокол / параметр / приём парсинга
  или шаблона начинается с PR в `contract/` (реестр + фикстуры + при необходимости
  схема), реализация — вторым шагом; в PR-чеклист входит «фикстуры добавлены,
  второй проект уведомлён» (issue в соседнем репо).
- CI: лаунчер — контракт-тесты внутри обычного `go test ./...`; LxBox —
  `flutter test test/contract/` + проверка `contract.lock`.
- Версионирование: `contract/VERSION` (semver); major — при изменении правил
  канонизации; приложения обновляют пин осознанным PR.

## 9. Таблица расхождений (backlog решений)

Факты сверены точечными агентами по коду обоих проектов (identity — дополнительно
эмпирическим кросс-прогоном 14 форм нод через оба парсера). Каждая строка станет
фикстурой корпуса + записью в реестре. Колонка «Решение» — моя рекомендация,
финальное слово за пользователем на ревью.

### A. Identity-хеш (кросс-прогон: 10 из 14 форм уже байт-идентичны)

Скелет алгоритма идентичен до буквы: sha256( компактный JSON( deepSort( emit − tag − detour )))
→ 64 hex. Расходятся 4 класса нод:

| # | Расхождение | Go | Dart | Решение (рекомендация) |
|---|---|---|---|---|
| A1 | HTML-escaping в каноническом JSON (`<>&` в пароле/SNI/path; теоретически также U+2028/U+2029) | `json.Marshal` экранирует `<…` и U+2028/29 | `jsonEncode` не экранирует | Канон = **без escaping** (сырой UTF-8, включая U+2028/29 — CANON.md фиксирует явно); Go переходит на `SetEscapeHTML(false)` + выравнивание юникод-эскейпов в `marshalCanonicalJSON` |
| A2 | ws `?ed=N` без `eh` | эмитит `max_early_data` + `early_data_header_name:"Sec-WebSocket-Protocol"` | только `max_early_data` (`transport.dart:44-53`) | Решить по конвенции экосистемы (v2ray ed = заголовок Sec-WebSocket-Protocol); рекомендация — **поведение Go**, Dart добавляет дефолт |
| A3 | anytls без `fp=` в URI | `utls` не эмитится (`node_parser_anytls.go:71-80`) | дефолт `fp='random'` через VLESS-конвенцию (`transport.dart:333`) | Выровнять по vless-конвенции (оба проекта дефолтят `random` для vless) → **Go добавляет дефолт** |
| A4 | wireguard endpoint | эмитит `name:"singbox-wg0"` и `system:false` (`node_parser_wireguard.go:136-138`) | не эмитит ни то, ни другое | Канон = **без дефолтных полей** → Go перестаёт эмитить дефолты |

Миграционная цена выравнивания: у затронутых классов нод изменится identity-хеш →
их disabled-отметки истекут по штатному TTL (макс. 30 дней). Для остальных форм
перенос отметок между устройствами заработает сразу.

Отдельно (by design, в IDENTITY.md, не чинится): `nodeIdentityKey`
(protocol|server|port|credential, схлопывание двух SNI одного сервера) есть только
в Dart — Go сознательно живёт одним хешом (`node_hash.go:14-25`); семантика дедупа
чуть разная и остаётся задокументированным различием (в корпусе — per-app
override, §5). Туда же: на мобиле хеш считается от ноды **после** import-rules
§302 (`patchedJson`) — отметки патченных нод с десктопом не совпадут (§7).

### B. Парсер: дыры и несимметрия

| # | Расхождение | Go | Dart | Решение |
|---|---|---|---|---|
| B1 | `naive+quic://` | есть (`node_parser_core.go:272-281`) | **нет** (только `naive+https`, `uri_parsers.dart:59`) | Dart добавляет |
| B2 | hysteria2 multi-port (`mport`, server_ports) | есть (`hysteria2_ports.go`) | **нет** (ни поля в `Hysteria2Spec`) | Dart добавляет |
| B3 | Пробел в userinfo | явный фикс (`percentEncodeUserinfoSpaces`) | неявно через Dart SDK (не покрыто тестом) | Фикстура в корпус; Dart — явный тест (возможно, уже проходит) |
| B4 | Тело = одиночный JSON-объект | **сам с собой не согласен**: классификатор принимает, а fetch-путь режет в `decoder.go:68-71` (обходы: base64-обёртка, кэш) | принимает | Go чинит decoder (принимать и передавать классификатору) — заодно закрывается внутренний баг |
| B5 | Clash YAML → подсказка | **нет** (падает в generic-ошибку) | есть (`clashYaml` flavor + `parse_hints.dart`) | Go добавляет детект + warning-код `clash_yaml_unsupported` |
| B6 | `proxy-http://`/`proxy+https://` | **нет** | есть (4 алиаса, `uri_parsers.dart:70-74`) | Go добавляет |
| B7 | ECH | URI: молча теряется; sing-box импорт: `tls.ech` проходит насквозь (коммент в `singbox_sanitize.go:47-48` врёт — кода удаления нет) | вычищает с warning §320 | **Решено**: `with_ech` в `LX_TAGS` ядра нет (`Makefile.lx:37`, есть `ech_tag_stub.go`) → оба режут с warning-кодом; Go добавляет вычистку в sanitize |
| B8 | vmess legacy cleartext | только внутри base64 | только внутри base64 | Паритет подтверждён — фикстура, решения не требует |
| B9 | Лимиты | URI 8192; нод 3000; тело 10 MB | URI 65536; **лимитов нод и тела нет** | `registry/limits.json`: URI = 65536 (Go поднимает), нод = 3000 и тело = 10 MB (Dart добавляет — защита от CIDR-подписок на 500+ нод и от стампида) |
| B10 | wgconf INI-экспорт из спеки | нет | нет (только WARP-специфичный писатель) | Не расхождение; корпус покрывает только импорт INI |
| B11 | wgconf INI как **тело подписки** | **нет** — в `BodyKind` INI-ветки нет, wgconf принимается только через UI-paste (`ui/configurator/business/parser.go`) | есть (`IniConfig` — полноценный body-kind) | Go добавляет INI-ветку в `ClassifySubscriptionBody` (нормативная схема §4 иначе невыполнима) |
| B12 | `vpn://` строкой внутри URI-списка | парсится построчно через `ParseNode` (и обходит MaxURILength) | перехватывается только целым телом (`body_decoder.dart:86`), в `parseUri` схемы `vpn` нет → строка молча теряется | Dart добавляет ветку в `parseUri`; лимиты `vpn://` расходятся и фиксируются в `limits.json`: длина ссылки 512 KiB (Go) vs 65536 (Dart), анти-bomb 8 MiB (Go) vs 4 MiB (Dart) |

### C. Язык шаблонов (движки уже родственные, 9 семантических расхождений)

| # | Расхождение | Go | Dart | Решение |
|---|---|---|---|---|
| C1 | bare `"@boolvar"` сравнение | trim + case-insensitive | строгое `== 'true'` | Спека: **trim + case-insensitive** (толерантнее к рукам пользователя) |
| C2 | `#notEmpty` на bool-var | значение переменной | всегда true (bool коэрсится в строку) | Спека: **семантика Go** |
| C3 | `#if` без and/or | warn + **true** | **false** (+ жёсткая load-валидация) | Спека: load-валидация как в Dart; fallback — false |
| C4 | Unresolved var | `""` + warn; strict-режим роняет пресет целиком | необъявленная → плейсхолдер остаётся; объявленная null → `Dropped` (выпадение фрагмента) | Спека: **модель Dart** (видимый плейсхолдер ловит опечатки, Dropped даёт пофрагментную деградацию); Go мигрирует в фазе 3 |
| C5 | int-коэрция | хардкод-список имён (`substitute.go:15-21`), ошибка → 0 | по объявленному `type:'int'` + clamp uint16 | Спека: **по типу**; Go мигрирует, список имён — временный fallback |
| C6 | `text_list` + `#in` со строкой-`@var` | есть | **типа нет вообще** | Ядро спеки; Dart добавляет тип |
| C7 | Неймспейс тегов пресета | автопрефикс `<preset_id>:<tag>` | префикса нет, дедуп по tag first-wins + warning | Спека: **префикс** (переносимость пресетов требует детерминированных тегов); Dart мигрирует — риск: поменяются теги в конфиге мобилки, нужны heal-редиректы ссылок |
| C8 | @var-подстановка в RHS предикатов (`{"@a":"@b"}`, `#in "@list"`, `#matches` c @var) | есть | нет | Ядро спеки; Dart добавляет |
| C9 | Поля объявления var | `platforms`, `if/if_or`, `separator`, `default_node`, per-platform default | `section`, `required`, `on_change`, `ref`, clamp | Ядро = общий минимум (name/type/default/options/title/tooltip/wizard_ui); остальное — задекларированные расширения + правило толерантности к неизвестным полям |

`@runtime.*` и `params[]` — подтверждённо отсутствуют в Dart (заявлено в
`if_engine.dart:17`) → задекларированные desktop-расширения, в ядро не входят.
`on_change`/`ref`/`enabled:"@var"` — mobile-расширения.

### D. Состояние и бэкап

| # | Расхождение | Go | Dart | Решение |
|---|---|---|---|---|
| D1 | Файловый экспорт/импорт | **нет вообще** (только снапшоты `wizard_states/*.json` рядом с бинарём) | есть: конверт `{app,kind,created_at,source_app_version,storage}`, категории, merge/replace, default-deny allowlist §159 | LX Backup строится на **конвенциях LxBox** (конверт, allowlist, preview); лаунчеру — новый export/import UI в фазе 4 |
| D2 | disabled-отметки | `disabled_nodes`: hash → unix int | `disabled_hashes`: hash → ISO-8601 без TZ (unix-числа молча выкинет) | В бэкапе — **unix seconds**; импортёры конвертируют в свой формат. TTL уже идентичен (clamp 3×interval, 24h–30d) |
| D3 | WARP/MASQUE аккаунты | `warp_accounts.{wg,masque}`, `private_key`/`peer_public` | top-level `warp_account`/`masque_account`, `priv_key`/`peer_pub` + `endpoint`, `awg`, `sni/idle_timeout/keep_alive` | Смысл 1:1 — в бэкапе единые имена, маппинг при импорте; доп. поля LxBox — опциональные |
| D4 | Тела inline-правил | сырой sing-box `match`-объект | типизированные camelCase-поля + `packages`/`wifi` + аспекты `dns`/`resolve` | Обменная форма: `{kind, name, enabled, num, outbound, match: <sing-box rule>, dns?, resolve?}`; mobile-only (`packages`, `wifi`) — в `extensions.lxbox`, десктоп их не применяет, но сохраняет |
| D5 | `route_final` | var `route_final` в `vars[]` | top-level `route_final` | В бэкапе — секция `route.final`; маппинг тривиален |
| D6 | vars-контейнер | массив `{name,value}` | map `{name: value}` | В бэкапе — map; имена — по `registry/vars.json` (пересечение уже есть: `dns_final`, `dns_strategy`, `tun_*`, `log_level`, …) |
| D7 | DNS-настройки | servers/rules с kind `template\|preset\|user` | servers/rules-map'ы без kind-дискриминатора (+legacy `rules_json`) | Обменная форма с kind-дискриминатором (модель лаунчера v6 ближе к цели); LxBox мигрирует чтение |
| D8 | Каналы vs глобальные outbounds | `connections.outbounds[]` | `channels[]` (vpn-1..10, фильтры, auto) | **v1 не маппит** — едут в `extensions.<app>`, lossless round-trip. После **SPEC 104** (каналы в лаунчере, цель 7) каналы переезжают в переносимую секцию бэкапа (v1.1) |
| D9 | Цели правил (`rules[].outbound`, `route.final`) ссылаются на разные неймспейсы тегов | глобальные outbounds шаблона | каналы `vpn-1..10` | Символические ссылки + политика «нет тега → правило импортируется выключенным с warning / final не применяется» (§7) |
| D10 | import_rules / identity_override подписки | **фичи нет** | есть (§302/§289) | v1: едут в `extensions.lxbox` внутри записи подписки — десктоп сохраняет, не применяет; порт import-rules в лаунчер — отдельная будущая задача вне 103 |

### E. Расхождения, найденные при авторинге реестра (фаза 0)

Авторинг `contract/registry/` с построчной сверкой обоих парсеров вскрыл ~45
расхождений сверх таблиц A–D. Пообъектные факты с refs зафиксированы в notes
самих файлов реестра и в `research/AUTHOR_FINDINGS.md`; решения принимаются
по-фикстурно в фазах 1–2 на четырёх принципах (D-016):
(а) ядро поддерживает → расширяем allowlist, не режем;
(б) секреты в share-URI не пишем;
(в) канон — без дефолтных полей;
(г) деградируй ноду, не конфиг.

Главное, сгруппировано:

- **Живые баги Go** (чинятся в фазе 1, вне очереди фикстур): hysteria2 `obfs`
  без password эмитится → fatal всего конфига; wgconf-импорт теряет `reserved`
  из `[Peer]` → WARP без трафика; share-URI подменяет httpupgrade на `net=ws`;
  httpupgrade-ветка URI не срезает `?ed`-хвост пути (404); reality `sid`
  нечётной длины уезжает в конфиг (ядро падает на hex-декоде); http-нода из
  sing-box-импорта теряет auth/path/headers на эмите; hysteria2 `gecko`
  режется, хотя ядро lx его поддерживает (у LxBox это был баг #53).
- **Живые баги Dart**: `toUriVmess` теряет `insecure`; `toUriSocks` теряет
  пароль password-only нод; multi-peer WG из sing-box-импорта берёт только
  первого peer; WG-ключи не валидируются (мусор уедет в конфиг и уронит check —
  класс `pbk=enabled`); ssh inline private_key пишется в share-URI (секрет
  в ссылке — против принципа (б), политика Go: ErrShareURINotSupported).
- **Ломают identity-хеш** (дополнение к §9.A, фаза 2): trojan без TLS — Dart
  пишет `tls:{enabled:false}`, Go омитит; TUIC — Dart всегда пишет дефолты
  cubic/native/h3; uTLS на QUIC (hy2/tuic) — Go URI-путь эмитит, Dart и
  Go-Xray-путь срезают; plain-WG дефолт MTU 1420 (Go) vs 1408 (Dart);
  junk-fp — три разных поведения деградации.
- **Несимметрия имён/алиасов** (фаза 1): hysteria2 `upmbps`↔`up_mbps` (кросс-эмит
  теряет значения); naive одиночный userinfo user↔password; tuic zero-rtt
  алиасы; семейство insecure-ключей (3 имени Go / 5 имён Dart); регистр
  query-ключей (Go fold / Dart case-sensitive); Xray hysteria: Go принимает
  `"hysteria2"`, Dart `"hysteria"` — наборы не пересекаются.
- **Односторонние фичи** (реестр помечает ext, план — выравнивание): anytls
  REALITY, ss `plugin`/`plugin_opts`, vless `encryption` §335, `type=h2`,
  плоские `ed`/`eh`, residual percent-decode — только Dart; XHTTP-поля SPEC 102
  (`no_sse_header`, `xmux`, …), `splithttp`-алиас, hysteria2 base64-payload,
  tuic `heartbeat` — только Go.
- **Контейнеры/группы**: amnezia — Go импортирует один контейнер, Dart все;
  `$PRIMARY_DNS`-подстановка только Dart; sing-box selector Dart конвертит
  в urltest (default теряется); дефолты Xray-балансировщика (url/interval)
  разные.
- **Шаблоны**: N1–N8 в TEMPLATE_LANG.md; пересечение vars — 16 имён (15
  portable), preset id — 4 общих + 2 пары синонимов (N2).

## 10. Фазы

- **Фаза 0 — Скелет контракта** (только репо лаунчера, без изменений кода
  приложений): каталог `contract/` целиком — схемы, реестр (перепись фактического
  состояния обоих парсеров), диаграммы, CANON/IDENTITY/TEMPLATE_LANG/BACKUP
  черновиками. Выход: контракт на ревью.
- **Фаза 1 — Корпус URI + раннеры**: перенос фикстур LxBox + экстракция inline-кейсов
  обоих проектов в `corpus/uri`; Go- и Dart-раннеры; первые решения по таблице §9;
  расхождения либо чинятся, либо помечаются `extension`.
- **Фаза 2 — Тела, эмиссия, identity**: `corpus/body` (Xray ownership/порядок,
  sing-box импорт, amnezia, wgconf — c INI-веткой в Go-классификаторе (B11),
  base64), `corpus/emit` + cross-emit, нормативный IDENTITY.md + сверочные тесты,
  структурированные warnings в Go.
- **Фаза 3 — Шаблоны**: TEMPLATE_LANG.md нормативно, `corpus/template/`,
  выравнивание движков (coercion по типу в Go; недостающие предикаты — по сверке),
  `registry/presets.json` + `vars.json`.
- **Фаза 4 — LX Backup v1**: схема + `docs/BACKUP.md`, экспорт/импорт в лаунчере
  (state v6 ↔ backup) и LxBox (settings ↔ backup), `corpus/backup/`, минимальный UI.
- **Фаза 5 — Процесс**: CONSTITUTION-поправки, CI-пины, (опционально, по решению
  пользователя) вынос `contract/` в отдельный репо `lx-contract`.

Фазы 1–4 дробимы на независимые PR; после каждой фазы обе кодовые базы зелёные
и совместимость строго не хуже текущей.

Параллельные треки программы (цели 7–8, отдельные спеки со своими PLAN/TASKS):
- **SPEC 104 — LAUNCHER_CHANNELS_MODEL**: логика «правила → каналы → узлы»
  в лаунчере по модели LxBox (§125/§267: channels как top-level state-сущность,
  материализация selector+urltest на build-time). Активирует spec-only задел 087.
  Начинать после фазы 1 (нужен корпус, чтобы не ломать паритет молча).
- **SPEC 105 — LAUNCHER_DNS_GROUPS**: DNS-группы как в LxBox (§312/§354).
  После SPEC 104 (использует ту же модель ссылок).
Обе спеки следуют правилу целей 9–10: перед портом — короткое архитектурное
ревью обеих реализаций, переносится лучшее решение, вердикт — в DECISIONS.md.

**Документация — сквозной deliverable каждой фазы (цель 12, D-020).** Фаза,
менявшая поведение, обновляет доки обоих проектов до закрытия:
- парсер: лаунчер `docs/ParserConfig.md` + `docs/DATA_FLOW.md` ↔ LxBox
  `docs/PROTOCOLS.md` — приводятся к общей структуре со ссылками на
  `contract/registry/` как единый источник истины (фазы 1–2);
- шаблоны/пресеты: лаунчер `docs/TEMPLATE_REFERENCE.md` + `docs/WIZARD_TEMPLATE.md`
  ↔ LxBox `docs/TEMPLATE.md` — сверяются с `contract/docs/TEMPLATE_LANG.md`
  (фаза 3);
- правила/состояние: лаунчер `docs/WIZARD_STATE.md` ↔ LxBox `docs/STORAGE.md` —
  дополняются форматом LX Backup (фаза 4); правила каналов — SPEC 104.
- `.ru`-зеркала лаунчера обновляются вместе с английскими.

**Арбитраж неоднозначного парсинга (D-019).** Если из кода/тестов обоих проектов
и конвенций непонятно, как разбирать параметр подписки, порядок сверки:
(1) поведение ядра sing-box-lx; (2) **как этот параметр парсит Happ**
(документация формата ссылок Happ + эмпирика); (3) конвенции Xray/v2rayN.
Вердикт фиксируется в note реестра и в DECISIONS.md.

## 11. Риски

1. **Выравнивание эмиттеров меняет identity-хеши** затронутых классов нод
   (§9.A: строки с `<>&`, ws `?ed=` без `eh`, anytls без fp, все wireguard) —
   их disabled-отметки истекут по TTL (макс. 30 дней) или пересопоставятся по
   `nodeIdentityKey` при импорте бэкапа. Прочие 10 форм переносимы уже сейчас.
   Выравнивания A2–A4 меняют и боевой эмит конфига — прогнать через обычный
   релизный цикл, не «тихим» патчем.
2. **Канонизация порядка Xray** (два прохода ownership в обоих, «одиночные раньше
   пулов» §342) — самое тонкое место корпуса; фиксируем текущее поведение
   фикстурами до любых правок.
3. **Структурированные warnings в Go** — заметная правка парсера лаунчера
   (сегодня warnings размазаны по debuglog); ограничить фазу 2 минимальным
   аккумулятором в `LoadNodesFromSourceEx`/`ParseNode`.
4. **SPEC 102 (XHTTP extra) в работе на develop** — xhttp-фикстуры корпуса писать
   после его закрытия, чтобы не гоняться за движущейся мишенью.
5. **Win7-джоба (go1.20)**: весь контракт-код лаунчера — строго `*_test.go`
   (включая канонизатор), `go build` их не собирает; грепы перед релизом всё
   равно прогнать.
6. **Смена семантики шаблонного движка задевает существующих пользователей.**
   На десктопе C1/C3/C4 (bool-сравнение, `#if` без and/or, unresolved-политика)
   меняют результат сборки конфига у людей с ручными шаблонами; на мобиле C7
   (автопрефикс тегов пресетов) меняет теги в собранном конфиге — нужны
   heal-редиректы ссылок (route.final, detour, отметки) и прогон через обычный
   релизный цикл с записью в release notes, как и A2–A4.
6. **Дрейф версий ядра** между приложениями (поле появилось в lx.N) — реестр
   может нести `"since_core"` у поля; в v1 не усложняем, оба проекта на одной
   линии форка.

## 12. Что нужно от пользователя

1. Ревью SPEC/PLAN (этот документ).
2. Решения по строкам таблицы §9, где предложены варианты (рекомендации будут
   проставлены).
3. Фаза 5: создавать ли отдельный репо `lx-contract` (до неё — не требуется).
