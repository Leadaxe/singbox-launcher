# SPEC 127 · Одно пространство имён: state v8 лаунчера и бэкап 1.0

Статус: N. Ветка develop. Норма — `contract/docs/ONE_NAMESPACE.md`
(утверждена владельцем 14.09.2026, D-106/D-107). Карта кода — `CODEMAP.md`
рядом (строится первой; исполнители читают код точечно по её адресам).
Связанные: SPEC 121 (секции узла), 126 (Tailscale в LxBox), 113-C (ось
порядка), 114 (бэкап = состояние), 118 (state v7).

## 1. Цель

Одна форма записи для каждой сущности в `state.json`, в состоянии LxBox и в
файле бэкапа: **запись = метаданные приложения + `body` = объект sing-box
как есть** (тег — единственное исключение, он в метаданных). Бэкап
лаунчера становится сериализацией состояния **без маппера**; старые файлы
0.x читаются legacy-входом. Инварианты: собранный `config.json` из того же
состояния **байт-в-байт прежний** (golden `real-v088`, эталон v6mig);
семантика импорта (BACKUP.md §9 слияние) не меняется.

## 2. Целевые формы (state v8 = бэкап 1.0)

Правило маршрута:

```json
{ "kind": "inline", "id": "…?", "name": "4pda", "enabled": true, "num": 1000,
  "body": { "domain_suffix": ["4pda.to"], "outbound": "proxy-out" } }
{ "kind": "srs", "name": "…", "enabled": true, "num": 1010,
  "refs": ["https://…/a.srs", "https://…/b.srs"], "body": { "outbound": "proxy-out" } }
{ "kind": "preset", "ref": "block-ads", "enabled": true, "num": 960, "vars": { "out": "reject" } }
```

- `body` — правило sing-box целиком: матчеры и `outbound` | `action`.
  `rule_set` в `body` **не хранится**: сборка вписывает теги наборов по
  `refs[]` при эмиссии (как сегодня по `srs_url`/`refs`).
- `num` вместо `order_num`; `name` — метаданные; `refs[]` всегда массив
  (одиночный набор — массив из одного).
- `preset`: `ref` + `vars` на уровне записи, `body` нет.

DNS-сервер и DNS-правило:

```json
{ "kind": "user", "tag": "my-doh", "enabled": true, "body": { "type": "https", "server": "1.1.1.1" } }
{ "kind": "template", "tag": "dns-google", "enabled": true }
{ "kind": "preset", "ref": "russian:yandex_udp", "enabled": true }
{ "kind": "user", "name": "…?", "enabled": true, "body": { "domain_suffix": [".ts.net"], "server": "my-doh" } }
```

- У сервера `tag` в метаданных, в `body` тега нет. У `template`/`preset`
  тела нет (оно у шаблона/пресета), только ссылка.
- У правила `body` — DNS-правило sing-box целиком, `server` внутри;
  `name` необязателен.

Узел (`sources[]` и `nodes[]` папок) — как в v7 (`kind`, `tag`, `body`,
`enabled`, `origin`, `detour`, `hops`, `group`, `service`, `reason`),
плюс `sections` в целевой форме:

```json
"sections": {
  "rules": [ { "kind": "inline", "name": "@{self} network", "enabled": true, "num": 945,
               "body": { "ip_cidr": ["100.64.0.0/10"], "outbound": "@self" } } ],
  "dns": { "servers": [ { "kind": "user", "tag": "@{self}-dns", "enabled": true,
                          "body": { "type": "tailscale", "endpoint": "@self" } } ],
           "rules":   [ { "kind": "user", "enabled": true,
                          "body": { "domain_suffix": [".ts.net"], "server": "@{self}-dns" } } ] }
}
```

## 3. State v8

- `SchemaVersionV8 = 8`, `meta.schema = "sources_v8"`; гейт схемы
  (`core/state/schema_gate.go`) — по образцу v7.
- **Миграция v7 → v8** одним проходом (`migration_v7_to_v8.go`, по образцу
  `migration_v6_to_v7.go`): `rules[]` — `order_num`→`num`,
  `body{name,match,outbound}` → `name` + `body` = `match` ∪ `{outbound}`,
  `body{name,srs_url|refs,outbound}` → `name`, `refs[]`, `body{outbound}`,
  `body{vars}` у preset → `vars`; `dns.servers[]`/`dns.rules[]` — плоские
  поля тела → `body{…}`, `tag` остаётся снаружи; `sections` узлов (SPEC 121
  §10) — те же правила. Цепочка миграций v6→v7→v8 остаётся рабочей
  (`load_router.go`), отчёт миграции (`migration_report.go`) знает шаг.
- **Общий парсер `body`-формы** для корневых записей и для секций — одна
  реализация (условие §4 п. 2 нормы). Секционные типы `NodeSections`
  используют те же `Rule`/`DNSServer`/`DNSRule`.
- Эмиссия/резолв (`core/build/resolve_route.go`, `resolve_dns.go`,
  `preset_merge.go`, инъекция секций) читают новую форму; выход конфига —
  байт-в-байт прежний.
- UI-модели (`ui/configurator/models/*`: `RuleState`, `DNSUserRule`,
  `PresetRefState`, `NodeRuleRef`, слоты и ось) — переезд на `num`/`body`
  без изменения поведения; тексты UI не меняются.
- Эталон: `TestGoldenScenarios` и `etalon_v6mig_capture_test` зелёные без
  пересчёта; новый фикстурный тест миграции v7→v8 на `core/state/testdata/v7_roundtrip.json`
  → ожидаемый v8 + обратная проверка «сборка того же конфига».

## 4. Бэкап 1.0

- **Экспорт = сериализация состояния v8** теми же типами (`core/backup` без
  своих `Rule`/`DNSRef`/`Server`; допускается тонкий слой для полей,
  которые в бэкап не едут: local-only, кэш узлов подписки, `service`).
  Корень файла: `version: "1.0.0"`, `sources[]` (union: `server` | `folder`
  | `subscription` | `chain`; у папки `nodes[]`; у подписки — `url`,
  `identity`, `skip`, `tag_policy`, отметки disabled по тегам, **без**
  `nodes[]`-кэша), `directions[]`, `rules[]`, `dns{servers,rules,final,strategy}`,
  `vars`, `route{final}`, `warp[]`. Узел: `kind`, `id`, `tag`, `enabled`,
  `body`, `origin`, `detour`, `sections`; цепочка: `kind: chain`, `hops[]`,
  `body`. Узлы подписки в бэкап — по правилам merge-заливки, как сейчас.
- **Импорт**: читает 1.0 напрямую в типы состояния; файлы **0.x** — через
  `legacy_read_0x.go`: `servers[]`/`subscriptions[]`/`chains[]` +
  `folder` → `sources[]`; `node_tag`/`config_json`/`uri` → `tag`/`body`/
  `origin`; правила `match`+`outbound`/`ref` → `body`/`refs[]`; DNS
  `name`/`value` → `tag`/`body`. Дальше — один общий путь слияния
  (BACKUP.md §9 без изменений), `scanUnknown` — по спискам 1.0, для 0.x —
  по прежним. Коды предупреждений прежние + `backup_section_record_dropped`.
- Секции узла едут в бэкап **только в 1.0** (норма §4).
- **Окно совместимости (договорённость с LxBox 14.09.2026).** Файлы гуляют
  между десктопом и телефоном у живых пользователей, релиз LxBox доезжает
  до F-Droid с задержкой в дни. Поэтому экспорт держит **два писателя**:
  1.0 (сериализация состояния) и переходный 0.12 (`legacy_write_012.go`,
  прежний маппер, без секций). По умолчанию в релизе лаунчера пишется 0.12
  до выхода релиза LxBox с чтением 1.0 на GitHub; переключение дефолта —
  одна константа `BackupExportFormatDefault`, в диалоге экспорта — чекбокс
  «Backup format 1.0 (new; requires LxBox ≥ <версия>)». Импорт читает оба
  всегда. После выхода LxBox: дефолт → 1.0, писатель 0.12 удаляется
  следующим релизом. LxBox зеркально: чтение 0.x+1.0 первой волной,
  запись 1.0 после миграции хранения.
- Тесты: round-trip состояние → файл → состояние на полном состоянии
  (источники всех видов, правила всех видов, DNS, секции); чтение всех
  существующих кейсов `contract/corpus/backup/*.backup.json` (0.12) через
  legacy-вход с теми же `expected` (они становятся кейсами legacy-чтения).

## 5. Контракт

- `contract/schema/backup.schema.json` → 1.0 (`sources[]` union, `body`,
  `refs[]`, `dns` `tag`+`body`), старая схема сохраняется как
  `backup-0.12.schema.json` для legacy-кейсов.
- `contract/docs/BACKUP.md` — таблицы полей под 1.0, §9 без изменений по
  смыслу; `NODE_SECTIONS.md` §1 → форма из ONE_NAMESPACE §2 и снятие
  «черновик»; `ONE_NAMESPACE.md` — статус «норма».
- Корпус: `corpus/backup/**` — новые кейсы 1.0 (в т.ч. `node_sections`,
  `sources_union`, `legacy_012_read`), прежние 0.12-кейсы остаются как
  legacy; `corpus/body/singbox/whole_config_sections`, `tailscale_endpoint`.
- `contract/VERSION` → `1.0.0` — **после** того как LxBox читает и пишет 1.0
  (правило обеих сторон); до этого лаунчер пишет 1.0 и читает 0.x, что
  создаёт окно «десктоп → телефон не импортируется», пока LxBox не выпустит
  чтение 1.0. Это принятое следствие порядка §4.
- `TASKS_LXBOX.md` `## 16` — контракт 1.0 (`## 14`/`## 15` заняты REALITY и #121): что изменилось в схеме, что
  ждём (чтение 1.0 + legacy 0.x, запись 1.0, корпус зелёный).
- Решение D-109 — норма контракта 1.0 (после реализации).

## 6. Решено между сторонами (14.09.2026, без вопроса владельцу)

- `entry` остаётся термином канона **разбора** (результат разбора узла),
  `body` — термином хранения и бэкапа; корпус разбора не переименовывается.
- Корень бэкапа 1.0 — как в §4; шапка `exported_by`/`exported_at` — как в 0.12.
- Окно совместимости — два писателя, дефолт 0.12 до релиза LxBox (§4).
- Порядок LxBox: ## 12 → ## 13 в целевой форме в состоянии (без экспорта)
  → чтение бэкапа 1.0 → миграция хранения + запись 1.0.

## 7. Порядок работ

1. CODEMAP (state, backup, corpus, UI-модели правил/DNS).
2. Волна 1 — state v8 + миграция + сборка + UI-модели + тесты миграции и
   golden.
3. Волна 2 — бэкап 1.0 (экспорт = состояние, legacy-вход 0.x) + round-trip
   + корпус legacy.
4. Волна 3 — контракт: схема, документы, корпус 1.0, TASKS_LXBOX `## 16`,
   D-109; пинг LxBox с хэшем.
5. Приёмка: SPEC 126 §4 (бэкап десктоп ↔ телефон) — после реализации LxBox.
