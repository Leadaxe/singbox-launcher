# Одно пространство имён: состояние лаунчера, состояние LxBox, бэкап — ПРЕДЛОЖЕНИЕ (контракт 1.0)

Статус: **предложение к решению владельца; формы §1 приняты LxBox 13.09.2026 с замечаниями, замечания внесены** (D-104 — направление).
Ничего из этого ещё не реализовано. Цель: одна форма записи для каждой
сущности во всех трёх местах, чтобы бэкап был сериализацией состояния обеих
сторон без мапперов, а владелец читал оба состояния одними глазами.

Принцип выбора формы: (1) никаких синтетических обёрток и переименований
ради контракта (`node_tag`, `config_json`, `value`, `name`-вместо-тега);
(2) тела — объекты sing-box под своим именем `body`/`match`, ключи sing-box
(snake_case), не переизобретённые; (3) при прочих равных — форма, которую
уже держат две стороны из трёх; (4) структура — как у состояния лаунчера
(`sources[]` с видом), потому что папка с `nodes[]` внутри выражает
владение, а плоский список с полем `folder` — нет.

## 1. Сверка трёх сторон (13.09.2026)

### Узел-сервер (свободный)

| Поле | Лаунчер state | LxBox state | Бэкап 0.12 | **Цель 1.0** |
|---|---|---|---|---|
| вид | `kind: server` | `type: user` | (список `servers[]`) | `kind: server` |
| имя/тег | `tag` | `name` | `node_tag` | `tag` |
| тело | `body` (объект) | `raw_body` (строка: URI или JSON) | `config_json` (объект) \| `uri` (строка) | `body` (объект sing-box, без `tag`) |
| происхождение | `origin{kind,raw,sub_url}` | `origin: "manual"` | — | `origin{kind: uri\|wg_ini\|json, raw, sub_url?}` (необязательно) |
| id | `id` (корень) | `id` (uuid) | `id` | `id` |
| включён | `enabled` | `enabled` | `enabled` | `enabled` |
| detour | `detour{folder_id,tag}` | `detour` только у члена папки (`detour_policy` — политика КОНТЕЙНЕРА, local-only, не узла) | `detour_tag`, `detour_node_*` | `detour{folder_id?, tag}` — только про узел; политика контейнера остаётся полем стороны |
| секции | `sections` | (новое, в форме контракта) | `sections` | `sections` (§2) |

Узел из share-URI: `body` — разобранный outbound, `origin.raw` — ссылка.
Строковое тело у LxBox исчезает: тело всегда объект, ссылка — в `origin`.

**Истина узла — `body`, и только он.** `origin.raw` — справка о происхождении;
переразбор `raw` в `body` происходит только по явному действию пользователя
(Regen / re-import), никогда автоматически при сборке или обновлении
приложения. Предупреждения узла пересчитываются из `body` тем же путём, что
импорт sing-box JSON. (У лаунчера это уже норма: `SPECS/features/sources.md`
§Происхождение; LxBox сегодня перепарсивает `raw_body` на каждой сборке —
с 1.0 перестаёт.)

### Контейнеры

| | Лаунчер state | LxBox state | Бэкап 0.12 | **Цель 1.0** |
|---|---|---|---|---|
| список | `sources[]` (union по `kind`) | `server_lists[]` (union по `type`) | `servers[]` + `subscriptions[]` + `chains[]`, у сервера поле `folder` | `sources[]` (union по `kind`: `server` \| `folder` \| `subscription` \| `chain`) |
| папка | `nodes[]` внутри | `members[]` внутри | плоско, `folder: "имя"` | `nodes[]` внутри |
| подписка | папка + `url`, `identity`, `skip` | type=subscription | `subscriptions[]` | `kind: subscription` + `url`, `identity`, `skip`, `nodes[]` (по правилам merge-заливки) |
| цепочка | `kind: chain` + `hops[]` + `body` | ? | `chains[]` | `kind: chain` + `hops[]` + `body` |

### Правило маршрута

| Поле | Лаунчер state | LxBox state | Бэкап 0.12 | **Цель 1.0** |
|---|---|---|---|---|
| вид | `kind` | `kind` | `kind` | `kind: inline \| srs \| preset` |
| id | — | `id` (uuid) | — | `id` (необязательно; сторона без id игнорирует и провозит) |
| имя | `body.name` | `name` | `name` | `name` |
| номер | `order_num` | `num` | `num` | `num` |
| включён | `enabled` | `enabled` | `enabled` | `enabled` |
| цель | `body.outbound` | `outbound` | `outbound` | `outbound` |
| матчеры | `body.match{…sing-box…}` | плоско camelCase (`domainSuffixes`, `ipCidrs`, …) | `match{…sing-box…}` | `match{…sing-box…}` |
| srs | `body.srs_url` | `srsUrl` | `ref` / `refs[]` | `refs[]` (одно имя, всегда массив) |
| preset | `body.vars` | — | `ref` + `vars` | `ref` + `vars` |
| ключи sing-box, которые вторая сторона не применяет | — | `packages`, `wifi`, `inbounds`, `ipIsPrivate` | не едут | в `match` под именами sing-box (`package_name`, `wifi_ssid`, `wifi_bssid`, `inbound`, `ip_is_private`, `source_ip_cidr`, …), типы sing-box (`port` — int[], `port_range` — строки); вторая сторона игнорирует по allowlist |
| расширения приложения (не sing-box) | — | `dns{}`, `resolve{}` | `dns`, `resolve` (LxBox) | поля ЗАПИСИ вне `match`: `dns{}`, `resolve{}` — LxBox; вторая сторона игнорирует молча |

### DNS-сервер и DNS-правило

| Поле | Лаунчер state | LxBox state | Бэкап 0.12 | **Цель 1.0** |
|---|---|---|---|---|
| вид пользовательской записи | `user` | `inline` | `user` | `user` (2 из 3) |
| тег сервера | `tag` | `tag` | `name` | `tag` |
| тело сервера | плоско рядом с `kind` | `body` | `value` | `body` (объект sing-box без `tag`) |
| тело правила | плоско рядом с `kind` | `rule` | `value` | `body` (объект sing-box, `server` внутри) |
| включён | `enabled` | `enabled` (у правил сегодня не хранится — добавят) | `enabled` | `enabled` |
| имя правила | — | `name` | `name` | `name` (необязательно; LxBox пишет всегда) |
| id правила | — | `id` у srs-DNS-правил | — | `id` (необязательно, как у правил маршрута) |
| preset/template | `ref` | `preset`/`template` | `ref` | `kind: preset \| template` + `ref` |

Целевая запись DNS-сервера: `{ "kind": "user", "tag": "my-doh", "enabled": true, "body": { "type": "https", "server": "…" } }`.
Целевая запись DNS-правила: `{ "kind": "user", "name": "…", "enabled": true, "body": { "domain_suffix": […], "server": "my-doh" } }`.

## 2. Секции узла в целевой форме

```json
"sections": {
  "rules": [
    { "kind": "inline", "name": "@{self} network", "enabled": true, "num": 945,
      "match": { "ip_cidr": ["100.64.0.0/10"] }, "outbound": "@self" }
  ],
  "dns": {
    "servers": [ { "kind": "user", "tag": "@{self}-dns", "enabled": true,
                   "body": { "type": "tailscale", "endpoint": "@self" } } ],
    "rules":   [ { "kind": "user", "enabled": true,
                   "body": { "domain_suffix": [".ts.net"], "server": "@{self}-dns" } } ]
  }
}
```

Это **одна** форма для `state.json`, `lxbox_settings.json` и файла бэкапа.
Семантика — `NODE_SECTIONS.md` без изменений (плейсхолдер, инъекция,
слияние, перенумерация); меняются только имена полей записей: `num`,
`match`, `tag`+`body`, вместо `order_num`/`body`/`value`/`name`.

## 3. Цена

| Сторона | Что меняется | Объём |
|---|---|---|
| Контракт | схема бэкапа 1.0 (`sources[]`, `body`, `tag`, `num`, `match`, `refs`, DNS `tag`+`body`), BACKUP.md, CANON.md, корпус `corpus/backup/**` перегенерировать, `NODE_SECTIONS.md` §1 | средний |
| Лаунчер | state v8: правила `order_num/body` → `num/name/outbound/match/refs`, DNS плоско → `body`, миграция v7→v8 с эталоном; бэкап = состояние без маппера; legacy-чтение файлов 0.x (`node_tag`/`config_json`/`uri`/`value`/`servers[]`) | большой |
| LxBox | хранение: `type`→`kind`, `name`→`tag`, `raw_body`(строка)→`body`+`origin`, матчеры camelCase→`match` с типами sing-box, `srsUrl`→`refs[]`, DNS `inline`→`user`, `rule`→`body`, `enabled` у DNS-правил; миграция `lxbox_settings.json` на диске при обновлении приложения, allowlist импорта, Debug API, STORAGE.md; legacy-чтение файлов 0.x | большой |

Legacy-вход обязателен обеим сторонам: файлы 0.12 уже выпущены релизами.
Это единственное место, где старые имена остаются, и только на чтении.

## 4. Порядок

Замечание LxBox (принято): если секции лягут в файл в целевой форме
(`tag`/`body`) раньше, чем корень, то в одном файле 0.13 будут две формы
DNS-записи и у обеих сторон второй парсер — ровно то, что запрещено. Поэтому
**секции едут в бэкап только вместе с корнем, в 1.0**; версия 0.13.0 для
секций не выпускается.

1. Владелец утверждает целевые формы §1–§2 (или правит).
2. **В состоянии** обе стороны кладут секции узла сразу в целевую форму
   (они новые, наследия нет) — у лаунчера это переезд `sections` с
   `order_num`/плоских DNS на `num`/`match`/`tag`+`body` в `state.json`, у
   LxBox — новое поле в целевой форме. `NODE_SECTIONS.md` §1 → §2 этого
   документа. **В бэкап до 1.0 секции не пишутся** (поле остаётся
   объявленным, но обе стороны его до 1.0 не экспортируют; приехавшее —
   провозится по правилу §1 BACKUP.md).
3. Контракт 1.0: схема + документы + корпус (лаунчер), D-105; `## 12`
   (`refs[]`) закрывается этим же решением по варианту А — слово владельца.
4. Лаунчер state v8 (корневые правила и DNS в целевую форму) и бэкап без
   маппера — кампания SPEC 127.
5. LxBox — миграция хранения `lxbox_settings.json` + legacy-чтение файлов
   0.x (решение владельца отдельной задачей).
6. `contract/VERSION` → 1.0.0, когда обе стороны читают и пишут новую форму.
   Приёмка SPEC 126 §4 (бэкап с узлом Tailscale десктоп ↔ телефон) — на 1.0.
