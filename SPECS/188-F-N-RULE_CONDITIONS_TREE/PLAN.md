# PLAN 188

Адреса — `CODEMAP.md` (снимок 23.11.2026). Пять волн; после каждой
`go build ./...` зелёный, существующие правила собираются как раньше, новые
кейсы корпуса не ломают действующие раннеры. Расхождения SPEC с кодом и
решения по умолчанию — раздел «Вопросы и расхождения» в конце.

Сквозные правила:

- **Имён ключей в коде движка и формы нет.** Что куда идёт (в набор, в DNS,
  какая рамка, какой вид поля) — из таблицы реестра `rule_conditions.json`
  через `core/config/registry`. Исключения — ключи формы записи и логики
  ядра: `refs`, `type`, `mode`, `rules`, `invert`, `rule_set`, цель
  (`outbound`/`action`/`method`); они — константы одного файла
  `core/state/rule_tree.go`.
- **Win7 (go1.20):** без builtin `min`/`max`, без `slices`/`maps`, без
  `http.PathValue`. Сортировки — `sort.SliceStable`.
- **Один читатель тела.** Обход дерева, подъём `refs`, вывод `kind` — только
  `core/state/rule_tree.go`; сборка, бэкап, Debug API, UI зовут его, а не
  разбирают тело сами.
- **Одна раздача тегов.** Теги наборов правил считает одна чистая функция
  `core/state/rule_tags.go`; все четыре вызова `ResolveRouteChecked` и
  миграция ссылок получают одинаковый результат.

## Волна 1 — контракт

Данные и документы; кода — загрузка таблицы реестра и проверка её
согласованности со списками зеркала.

### Файлы

| Файл | Изменение |
|---|---|
| `contract/registry/rule_conditions.json` (новый) | таблица условий (ниже) |
| `core/config/registry/rule_conditions.go` (новый) | типы `RuleCondition`, `RuleConditionKey`; `(*Registry).RuleConditions()`, `RuleConditionOfKey(key)`, `RuleSetKeys()`, `RuleDNSKeys()`; проверка согласованности при загрузке |
| `core/config/registry/registry.go` | `Load` (`:846`) читает `rule_conditions.json` рядом с `allowlists.json` (`:985`) |
| `contract/registry/allowlists.json` | `rule_dns_mirror_match` + `network_is_expensive`; `rule_dns_mirror_unseen` + `source_port`, `source_port_range`; правка `note` трёх списков (зеркало по дереву) |
| `contract/schema/backup.schema.json` | `lx_backup` → `enum [2, 3]`; `$defs/rule_body` — рекурсивное тело (простое с `refs`, `logical` без `refs`); `$defs/rule.properties.refs` — «устаревшая форма, читается» |
| `contract/docs/TEMPLATE_LANG.md` §6.11 (`:1610`) | зеркало по дереву: таблица SPEC §5, обёртка действий над `logical` (вопрос 7), `invert`, исключение адресного inline-набора, ⚠ пустого зеркала |
| `contract/docs/TEMPLATE_LANG.md` (новый §6.13) | норма выпуска наборов правила (SPEC §4: ссылки, inline-набор, `rule_set` на правиле, всё или ничего) и норма тегов (база, развод, производные, h8, пути листьев) |
| `contract/docs/ONE_NAMESPACE.md` §1 (`:94`) | строка «srs-наборы»: `refs` в теле простого правила; пример записи |
| `contract/docs/BACKUP.md` | таблица `rules[]` (`:430`): `refs` в `body`, `kind` выводится, запись `refs` — старая форма; «Ключ `rule_set` в `body` не хранится» → уточнение про рукописный; §8 (`:740`): `lx_backup: 3`; §11 — раздел «Что изменилось против 1.0» |
| `contract/corpus/rule_tree/` (новый каталог) + `README.md` | кейсы выпуска (ниже); раннер — волна 3 |
| `contract/corpus/backup/` | `v11_rule_refs_in_body` (новая форма, `lx_backup: 3`), `v10_rule_refs_on_record` (старая форма, `lx_backup: 2` → ожидание новой) |
| `contract/VERSION`, `contract/README.md` | 1.1.179, строка changelog |
| `contract/TASKS_LXBOX.md` | §182 |
| `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md` | D-156 (дерево и `refs` в теле), D-157 (схема тегов) |

### Таблица условий

```json
{ "v": 1,
  "conditions": [
    { "id": "destination", "label": "Destination / Rule sets", "order": 10,
      "keys": [
        { "key": "refs",           "field": "refs",   "dns": true,  "set": false },
        { "key": "domain",         "field": "lines",  "dns": true,  "set": true, "group": "domain" },
        { "key": "domain_suffix",  "field": "lines",  "dns": true,  "set": true, "group": "domain" },
        { "key": "domain_keyword", "field": "lines",  "dns": true,  "set": true, "group": "domain" },
        { "key": "domain_regex",   "field": "lines",  "dns": true,  "set": true, "group": "domain" },
        { "key": "ip_cidr",        "field": "cidr_lines", "dns": false, "set": true, "group": "address" },
        { "key": "ip_is_private",  "field": "bool",   "dns": false, "set": false, "conflicts": ["refs"] } ] },
    { "id": "port",        "keys": [ {"key": "port", "field": "ports"}, {"key": "port_range", "field": "lines"} ] },
    { "id": "source",      "keys": [ {"key": "source_ip_cidr", "field": "cidr_lines", "dns": true}, {"key": "source_ip_is_private", "field": "bool", "dns": true} ] },
    { "id": "source_port", "keys": [ {"key": "source_port", "field": "ports"}, {"key": "source_port_range", "field": "lines"} ] },
    { "id": "process", "one_of": true,
      "keys": [ {"key": "process_name", "field": "processes", "dns": true}, {"key": "process_path", "field": "lines", "dns": true}, {"key": "process_path_regex", "field": "lines", "dns": true} ] },
    { "id": "protocol",    "keys": [ {"key": "protocol", "field": "enum_multi", "values": ["http", "tls", "quic", "stun", "dns", "bittorrent", "dtls", "ssh", "rdp", "ntp"], "open": true} ] },
    { "id": "network",     "keys": [ {"key": "network",  "field": "enum_multi", "values": ["tcp", "udp", "icmp"]} ] },
    { "id": "inbound",     "keys": [ {"key": "inbound",  "field": "lines", "dns": true} ] },
    { "id": "wifi", "platforms": ["windows", "linux", "android"],
      "keys": [ {"key": "wifi_ssid", "field": "lines", "dns": true}, {"key": "wifi_bssid", "field": "lines", "dns": true} ] },
    { "id": "apps",         "forms": ["lxbox"], "keys": [ {"key": "package_name", "dns": true}, {"key": "package_name_regex", "dns": true} ] },
    { "id": "network_type", "forms": ["lxbox"], "keys": [ {"key": "network_type", "dns": true}, {"key": "network_is_expensive", "dns": true} ] }
  ] }
```

Поля по умолчанию: `dns: false`, `set: false`, `forms: ["launcher",
"lxbox"]`. `group` — признак ключа для правил сборки: `domain` (доменный ключ:
Block HTTPS, «inline-набор с доменами»), `address`. Подписи условий — ключи
`locale.T` (английский текст), у LxBox свои. Точная форма — на усмотрение
исполнителя волны 1, содержание — как выше.

**Проверка при загрузке реестра** (`rule_conditions.go`, без отдельного
теста — `registry.Get()` зовёт каждый раннер корпуса): ключ встречается в
таблице один раз; каждый ключ, кроме `refs`, есть в `route_rule_conditions`;
`dns: true` ⇔ ключ в `rule_dns_mirror_match` (для ключей таблицы; `user`,
`user_id` списка — вне таблицы, допустимо); ключ с `group: domain` ⇔ в
`rule_dns_block_https_domains`; `set: true` — только `domain*` и `ip_cidr`.
Расхождение — ошибка загрузки реестра: в CI красный любой тест сборки.

### Корпус `rule_tree/`

Формат: `<case>.rules.json` (`{"_": "…", "template_rule_set_tags": [...], "rules": [...]}`
— записи в форме состояния/бэкапа, наборы считаются скачанными) →
`<case>.expected.json` (`{"rule_set": [{tag, type, format, rules?}],
"route_rules": [...], "dns_rules": [...]}`); у записей `rule_set` вида
`local`/`remote` сверяются `tag`, `type`, `format`; `path` и `url` — нет.
Нормативно всё остальное, включая матчер DNS (вопрос 7).

Кейсы (SPEC §7 + схема тегов):

| Кейс | Проверяет |
|---|---|
| `flat_refs_domains` | плоское правило: 2 ссылки + `domain_suffix` + `ip_cidr` → `B`, `B:<h8>`, `B:inline`; зеркало без исключения inline (домены есть) |
| `flat_refs_ip_only` | ссылка + `ip_cidr` → `B:inline` исключён из зеркала |
| `flat_no_refs_bytes` | плоское без ссылок — тело на правиле как есть, наборов нет (критерий 5) |
| `tree_and_or` | `logical and` [условия, `or` группа] → `B:inline:1.0` у листа, зеркало-дерево |
| `invert_port_dropped` | `not port 443` + DNS → зеркало пустое (`dns_rules: []`), признак пустоты |
| `invert_carried` | `invert` с доменами → перенесён в зеркало |
| `single_child_unwrap` | после выброса пустых детей обёртка снята |
| `same_ref_two_leaves` | одна ссылка в двух листах — один тег, одна запись |
| `name_dedup` | два правила «Ads» + выключенное «Ads» + пресет с тегом-коллизией → `Ads`, `Ads (2)`, `Ads (3)` |
| `derived_tag_collision` | правило `X` и правило `X:inline` |
| `h8_collision` | две ссылки с совпадающими первыми 8 hex — у обеих 16 символов (пара подбирается один раз перебором, ~2³² хэшей, и фиксируется в кейсе) |
| `ip_cidr_inbound_mirror` | лист `{ip_cidr, inbound}` с DNS → зеркало `{inbound}` на все запросы входа (SPEC §5) |
| `block_https_over_tree` | Block HTTPS / Force IPv4 / Fixed над `logical` — обёртка `and` с `query_type`/`ip_version` |

Кейсы бэкапа — в `corpus/backup/` действующего раннера (ниже, волна 2).
Ожидания пишутся с нормы, не с вывода движка (corpus/README.md).

### Решения

- Таблица — отдельный файл, а не новый allowlist: у ключа пять свойств,
  список строк их не выражает. Списки зеркала остаются (их читает LxBox и
  действующий код), согласованность с таблицей держит загрузка реестра.
- Новый корпус `rule_tree/`, а не дописка `rule_dns/`: раннер `rule_dns`
  снимает матчер как «выбор стороны» (`contract_rule_dns_test.go:86-101`), а
  дерево зеркала теперь норма; и кейсы с `refs` в теле до волны 2 положили бы
  действующий раннер.
- `lx_backup: 3` (вопрос 2).

### Ловушки

- Генератор `go generate ./contract/...` таблицу условий не видит
  (`gendocs` — протоколы и предупреждения). Прогнать: diff в
  `contract/docs/generated` должен остаться пустым.
- `rule_dns_mirror_unseen` читает окно правила (`RuleDNSMirrorNarrowed`,
  `rule_dns_mirror.go:98`): добавление `source_port*` сразу меняет подсказку —
  это и нужно, поведение верное и до волны 3.
- Кейсы `corpus/backup/` с `lx_backup: 3` действующий раннер пропускает
  («файл новее читаемого», `corpus_test.go` шапка) — до волны 2 он зелёный.

### Проверка

`go build ./...`; `go generate ./contract/...` без diff. Новых тестов нет.

## Волна 2 — хранение

`refs` в теле, вывод `kind`, версии форматов, привязка копии к ссылке.
Теги наборов и их ссылки **не трогаются** (перенесено в волну 3 — вопрос 5).
Сборка учится читать `refs` из тела плоского правила; дерево с вложенными
`refs` до волны 3 не выпускается (пропуск правила с предупреждением в лог).

### Файлы

| Файл | Изменение |
|---|---|
| `core/state/rule_tree.go` (новый) | обход и нормализация дерева (ниже) |
| `core/state/rule_types.go` | `NewSrsRule` пишет `refs` в тело; новый писатель `NewTreeRule(name, body, outbound)` — `kind` по дереву; `DecodeBody` берёт ссылки из `RuleRefs`; `SrsBody.Refs` — все ссылки дерева; комментарий шапки файла |
| `core/state/disk_v8.go` | `SchemaVersionV9 = 9`; `parseV8` зовёт `NormalizeRuleRecords` (подъём `refs` записи в тело, вывод `kind`) до `legacyCustomRulesFromV6` |
| `core/state/save.go`, `state.go:43`, `schema_gate.go:28`, `load_router.go:143` | пишется v9; `SchemaMajor = 9`; читатель `meta >= 8` → тот же разбор |
| `core/state/record_clone.go` | `Refs` записи больше не пишется, копия без изменений |
| `core/state/load_v6.go:214` | UI-вид: `RuleSet` из `RuleRefs`; `cr.Rule` srs-правила — тело с `refs` (заглушку `rule_set`, `:262-275`, снять — волна 4; в волне 2 оставить, но теги брать как прежде) |
| `core/state/rule_set_options.go:63` | очистка настроек — по выведенному `kind` |
| `core/build/resolve_route.go` | `resolveSrsRouteRule`/`resolveInlineRouteRule`: ссылки из `RuleRefs`, ключ `refs` снимается с выпускаемого тела; вложенные `refs` → пропуск с логом |
| `core/build/preset_merge.go:811` `CollectSrsCachedPaths` | карта **по ссылке**: `map[ref]path`; сигнатура `PresetMergeContext.SrsCachedPaths` → `map[string]string` |
| `core/build/rule_set_policy.go` | `RulePolicy` (`:64`) и `RuleSetFetches` (`:155`) — `RuleRefs`; `DropUncachedSrsPaths` по новой карте |
| `core/config_service.go:471-482` | GC keep-лист — `RuleRefs` |
| `core/config_service_context.go:92`, `ui/configurator/business/create_config.go:200`, `add_rule_dialog.go:1656` | новая карта кэша |
| `core/stateedit/rules.go:64`, `servers.go:654` | `AddRule` — нормализация записи; `nameRefs` — `RuleRefs` |
| `core/debugapi/state_endpoints.go:210-241` | PATCH: нормализация каждой записи до `DecodeBody` |
| `core/backup/backup10.go`, `file.go:154`, `export10.go:53`, `types.go` | `FormatVersion11 = 3`; `Parse`: 2 и 3 — один путь `decodeTolerant10`; экспорт пишет 3; текст отказа на 4+ |
| `core/backup/import10.go:316` `decode10Rule`, `legacy_read_0x.go:408` | нормализация записи (общая функция состояния) |
| `core/backup/corpus_test.go` | ожидание `refs` — `RuleRefs` записи |
| `ui/configurator/models/preset_ref_sync.go:252` | `customRuleStateToV6Rule` — `NewSrsRule` уже пишет новую форму; изменений, кроме комментария, не нужно |
| `core/build/contract_rule_dns_test.go:73-80` | кэш по ссылке |

### `core/state/rule_tree.go`

```go
const ( ruleKeyRefs = "refs"; ruleKeyType = "type"; ruleKeyMode = "mode"; ruleKeyRules = "rules"; ruleKeyInvert = "invert" )

// RuleNode — вид узла тела без копии значений: Simple с картой, Logical с детьми.
func WalkRuleTree(body map[string]interface{}, fn func(path []int, node map[string]interface{}, logical bool) bool)
func IsLogicalRule(node map[string]interface{}) bool           // type == "logical"
func RuleRefs(r Rule) []string                                   // все ссылки дерева в порядке обхода (pre-order), дедуп; пустое дерево — r.Refs (старая форма до нормализации)
func NormalizeRuleRecord(r *Rule) (changed bool)                 // refs записи → тело корня (если в дереве refs нет), r.Refs = nil, Kind = srs|inline по дереву; preset не трогает
func NormalizeRuleRecords(rules []Rule) int
```

Подъём по сырому JSON (`decodeObjectOrdered`, `rule_types.go:362`): `refs`
вставляется первым ключом корня, остальные байты тела не трогаются — у
inline-правил без ссылок файл и `config.json` байт-в-байт прежние. Корень
`logical` с `refs` на записи: ссылки уходят в обёртку `{"type":"logical",
"mode":"and","rules":[{"refs":[…]}, <тело без цели>], <цель>}` — форма SPEC §3
«правило с группами»; так выражает и LxBox.

### Решения

- **`kind` пишется** (выведенный): LxBox и старые читатели различают вид по
  нему; источник правды — дерево, расхождение чинит нормализация.
- **state v9.** Нормализация идемпотентна и работает при каждом чтении, но
  мажор нужен гейту переноса состояния машин: сборка 2.4.x прочитает v9 как
  v8 молча и выпустит `refs` в `config.json` ядру (неизвестное поле — отказ
  старта). `CheckSchemaCompatible` старой сборки на 9 откажет внятно.
- **Карта кэша по ссылке.** Позиционная привязка `[id][i] ↔ Refs[i]`
  (`resolve_route.go:380-386`) в дереве не определена, а одинаковые имена
  правил (из бэкапа, из API) давали общий ключ. Имя файла уже от ссылки —
  копии не перекачиваются ни сейчас, ни после смены тегов.
- **`lx_backup: 3` всегда**, а не только для деревьев: старый читатель на 2
  с `refs` в теле выпустил бы ключ `refs` ядру (inline) или потерял правило
  (`DecodeBody` srs без `Refs`). Вопрос 2.

### Ловушки

- `BodyMap()` теперь отдаёт и `refs` — каждый путь, где тело едет в конфиг
  (`resolve_route.go:319`, `:424`), обязан снять ключ. Иначе ядро не
  стартует на `unknown field "refs"`. Искать: `grep -n "BodyMap()" core ui`.
- `DecodeBody` (`rule_types.go:507`) зовут бэкап (`import10.go:371`),
  stateedit (`rules.go:65`), Debug API (`state_endpoints.go:210`), сборка.
  Нормализация обязана идти **до** него на каждом входе, иначе srs-запись
  старой формы с пустым деревом пройдёт, а новой — нет, или наоборот.
- `MigrateRuleSetTagRefs` (`disk_v8.go:112`) работает по `r.Refs` записи
  (`rule_identity.go:110`) — вызывать **до** нормализации либо перевести на
  `RuleRefs`. Иначе переписывание хэш-тегов 22.11 молча перестанет работать.
- `cloneRule` (`record_clone.go:43`) копирует `Refs` — после нормализации
  он пуст, но во входах до нормализации (бэкап) копия должна его сохранить.
- Канонический roundtrip (`canonical_roundtrip_test.go`) и голдены с
  `"version": 8` (2 файла) и экспорт с `lx_backup: 2` (тесты схемы,
  `schema_test.go`) — обновятся в CI; локально не гонять.
- `rule10Keys` (`file_keys_10.go:81`) строится из тегов `state.Rule`: поле
  `Refs` оставить в структуре (читается старая форма), иначе импорт
  предупредит «неизвестный ключ `refs`».

### Тест

`core/state`: `TestRuleRefsLiftedIntoBody` — файл v8 старой формы: srs с двумя
`refs` на записи и телом-целью, inline с порядком ключей «как в файле»,
srs с корнем `logical`; `Load` → `refs` в теле первым ключом, запись без
`refs`, `kind` выведен, inline байт-в-байт; `Save` → `meta.version` 9;
повторная загрузка ничего не меняет. Плюс раннер `TestBackupCorpus` по имени
(кейсы `v10_rule_refs_on_record`, `v11_rule_refs_in_body`).

### Проверка

`go build ./...`; `go test ./core/state -run TestRuleRefsLiftedIntoBody -count=1`;
`go test ./core/backup -run TestBackupCorpus -count=1`.

## Волна 3 — сборка

Обход дерева, новая схема тегов, inline-наборы, зеркало по дереву, миграция
рукописных ссылок на теги.

### Файлы

| Файл | Изменение |
|---|---|
| `core/state/rule_tags.go` (новый) | раздача тегов (ниже) |
| `core/state/rule_identity.go` | `SrsRuleSetTag` → только для миграции (переименовать в `legacySrsRuleSetTag`); `MigrateRuleSetTagRefs` → `MigrateRuleSetTagRefsV9`: старые теги `user:<stable>`, `user:<legacy>` (+`:N`) → новые |
| `core/state/disk_v8.go:112` | вызов новой миграции после нормализации |
| `core/build/rule_tree_emit.go` (новый) | выпуск правила-дерева: записи `rule_set`, тело маршрута, дерево зеркала |
| `core/build/resolve_route.go` | `resolveInlineRouteRule` и `resolveSrsRouteRule` → один `resolveUserRouteRule` через `rule_tree_emit.go`; `ResolvedRouteRule` + поля `Mirror map[string]interface{}` (nil — пусто), `MirrorDomains bool`; раздача тегов один раз в `ResolveRouteChecked` до цикла (`:184`) |
| `core/build/rule_dns_mirror.go` | `collectRuleDNSMirrors` берёт `r.Mirror`; обёртка действий над `logical`; `RuleDNSMirrorMatch`/`BlockHTTPSAvailable`/`Narrowed` → функции над **телом записи** (`RuleDNSMirrorForBody(body)`), для UI |
| `core/build/preset_merge.go` | `PresetMergeContext.TemplateRouteRuleSetTags []string`; `MergePresetsIntoRoute` (`:342`) заполняет из `routeRaw`, если пусто |
| `core/build/reserved_tags.go` (новый, или в `rule_tree_emit.go`) | `ReservedRuleSetTags(presets, rules, templateTags)` — `<preset_id>:<tag>` всех наборов пресетов, стоящих в `rules[]` (включённых и нет, без раскрытия `#if`), плюс теги `route.rule_set` шаблона |
| `core/config_service_context.go:85`, `ui/configurator/business/create_config.go:195` | заполнение `TemplateRouteRuleSetTags` из `td.Config["route"]` |
| `ui/configurator/dialogs/add_rule_dialog.go:1628` `rulePreviewText` | правила модели целиком + шаблон (вопрос 12) |
| `core/build/contract_rule_tree_test.go` (новый) | раннер `contract/corpus/rule_tree/` |
| `core/build/contract_rule_dns_test.go` | кейсы `rule_dns` проходят без правок ожиданий (плоские правила; матчер снимается) |

### `core/state/rule_tags.go`

```go
// RuleTags — теги наборов одного правила.
type RuleTags struct {
    Base   string            // B'
    refs   map[string]string // ссылка → тег
    inline map[string]string // путь листа ("" — корень, "1.0") → тег
}
func (t RuleTags) Ref(ref string) string
func (t RuleTags) Inline(path string) string

// AllocateRuleTags — теги всех inline|srs-записей оси. rules — в порядке оси
// (NormalizeRuleOrder); reserved — занятые заранее (теги шаблона и пресетов).
// Ключ результата — индекс записи в rules.
func AllocateRuleTags(rules []Rule, reserved func(tag string) bool) map[int]RuleTags
func RefHash(ref string, n int) string // sha256(strings.TrimSpace(ref)) → первые n hex (8 или 16)
```

Алгоритм (SPEC §4, по шагам):

1. `taken` = reserved.
2. Проход по оси: каждая запись `inline|srs` (с наборами и без, вкл. и выкл.)
   берёт базу: `B` = `Name` или `unnamed`; `B'` = первый из `B`, `B (2)`, …,
   не в `taken`; `taken += B'`.
3. Второй проход по оси — производные теги правил **с наборами** (ссылки и
   inline-наборы листов, который сборка выпустит): первая ссылка корневого
   простого правила → `B'`; прочие → `B':<h8>` (коллизия h8 внутри правила →
   16 у обеих); inline корня → `B':inline`, листа → `B':inline:<путь>`.
   Производный тег в `taken` → тот же суффикс `" (2)"`, `" (3)"` на этом
   теге. `taken +=` каждый.
4. Одна ссылка в нескольких листах — один тег (по первому появлению в обходе
   pre-order).

Какие листы получат inline-набор, решает таблица реестра (`set: true`) — функция
состояния получает признак через параметр `setKeys map[string]bool`
(`registry.RuleSetKeys()`), чтобы `core/state` не зависел от реестра.

### Выпуск правила (`rule_tree_emit.go`)

```go
type userRuleEmit struct {
    Body      map[string]interface{}   // в route.rules; nil — правило не выпускается
    RuleSets  []ResolvedRouteRuleSet   // ссылки + inline-наборы
    Mirror    map[string]interface{}   // дерево DNS-зеркала; nil — пусто
    MirrorDomains bool                 // доменный ключ или набор хоть в одном простом узле
    SkipReason string
}
func emitUserRule(rule corestate.Rule, tags corestate.RuleTags, cache map[string]string,
                  policy RuleSetPolicy, finalTags map[string]bool, reg *registry.Registry) userRuleEmit
```

Каждый простой узел (рекурсивно, копией):

1. `refs` → запись `rule_set` на ссылку (local из `cache[ref]` или remote по
   `policy`, формат `srstag.FormatFromRef`); узел теряет `refs`.
2. Если у узла есть `refs` и ключи `set: true` — они уходят в inline-набор
   `{type: inline, tag, rules: [{…ключи}]}`; `invert` не уходит никогда. Без
   `refs` — ключи остаются на узле (критерий 5: байт-в-байт).
3. `rule_set` узла = рукописные теги (как были) + теги ссылок в порядке
   `refs` + тег inline последним; строка, если тег один (как сегодня).
4. Всё или ничего: нет кэша / недоступен `download_detour` хоть у одной
   ссылки дерева → `Body = nil`, наборы — `Skipped` с причиной.

Зеркало строится тем же обходом (SPEC §5): у простого узла остаются ключи
`dns: true` таблицы и `rule_set` без inline-тегов «без доменных ключей»;
пустые дети `and`/`or` выбрасываются; один ребёнок — обёртка снимается;
`invert` переносится; опустевший узел выбрасывается и под `invert`; пустой
корень → `Mirror = nil`.

### Миграция ссылок на старые теги

`MigrateRuleSetTagRefsV9(rules, dnsRules)`: для каждой srs-записи старые теги
`legacySrsRuleSetTag(StableRuleID, i)` и `legacySrsRuleSetTag(LegacyRuleID, i)`
по **прежнему порядку `RuleRefs`** → новый тег той же ссылки
(`AllocateRuleTags(...).Ref(ref)`); неоднозначный старый id (два правила)
не переписывается — как в `MigrateRuleSetTagRefs` (`rule_identity.go:95-108`).
Замена — строковым литералом в сыром теле inline/srs-правил (сохраняя порядок
ключей) и `renameRuleSetRefs` в телах DNS-правил `user`. Шаблона при чтении
состояния нет: `reserved` = «тег с префиксом `<ref пресета из rules[]>:`»
(вопрос 9). Идемпотентна: после переписывания старых тегов нет.

### Решения

- Раздача тегов — в `core/state`, а не в `build`: её зовёт загрузка
  состояния (миграция), а `core/state` не может импортировать `build`.
- `ResolvedRouteRule.Mirror` считается при выпуске, а не из `Body` потом:
  только выпуск знает, какие ключи ушли в inline-набор и есть ли в нём
  домены. UI зовёт тот же обход над телом записи (`RuleDNSMirrorForBody`)
  без кэша — признак пустоты зеркала от кэша не зависит.
- Рукописный висячий `rule_set` теперь снимает правило целиком
  (`cleanDanglingRuleSetInRule`, `preset_merge.go:101`) — вопрос 3.
- Действия зеркала над деревом (Block HTTPS, Force IPv4, Fixed A/AAAA)
  оборачиваются: `{type: logical, mode: and, rules: [M, {query_type|ip_version}], action…}`;
  над простым `M` — как сегодня (ключи рядом), чтобы плоские правила дали
  прежний `config.json`.

### Ловушки

- Четыре вызова `ResolveRouteChecked` (`preset_merge.go:360`, `:688`,
  `rule_dns_mirror.go:236`, `add_rule_dialog.go:1659`) обязаны передать одни
  и те же `rules` и `reserved` — иначе зеркало ссылается на тег, которого нет в
  `route.rule_set`, и ядро не стартует. `ruleDNSMirrorsForMerge` и
  `CollectEmittedRouteRuleSetTags` строят `TemplateData` сами
  (`rule_dns_mirror.go:235`, `preset_merge.go:687`) — туда же
  `TemplateRouteRuleSetTags` из контекста.
- Порядок оси для тегов — после `NormalizeRuleOrder` (`resolve_route.go:184`),
  с выключенными записями; миграция при чтении — по `SortRulesByNum`
  состояния. Расхождение порядка = разные теги у сборки и миграции.
- Теги с пробелами и не-ASCII (`Реклама (2)`) — допустимы ядром; в имена
  файлов они не идут (`srstag.FileName` от URL). Проверить, что
  `ResourceNameForSRS` (`route_merge.go:173`) зовётся от тега-URL, а не от
  тега правила.
- `emittedTags` (`resolve_route.go:178`) — first-wins: теги пресетов заняты
  заранее, поэтому пользовательское правило с именем `russian:ru-domains`
  получит `russian:ru-domains (2)`, а не затрёт пресет.
- `RuleDNSBlockHTTPSAvailable(body, srs)` знает только плоское тело
  (`rule_dns_mirror.go:87`); UI вызывает его в `dns_unified_rules.go:127` и
  окне правила — заменить на `MirrorDomains` из нового обхода.
- `invert` плоского правила сегодня не переносится (`rule_dns_mirror.go:58`
  — список без `invert`): правка меняет `config.json` правил с `invert` и
  DNS-опцией — это заявленное исправление SPEC §5, записать в release notes.
- Пресетные правила (`resolvePresetRouteRule`) не трогать: дерево и теги —
  только для `inline|srs`.

### Тест

`core/build`: `TestContractCorpusRuleTree` — раннер `contract/corpus/rule_tree/`
(один data-критичный, покрывает теги, наборы, маршрут и зеркало). По имени —
действующие `TestContractCorpusRuleDNS` и `TestBackupCorpus`. Миграцию ссылок
покрывает расширение кейса волны 2 (`TestRuleRefsLiftedIntoBody`: inline с
`user:<id>:2` и DNS-правило `user` с `user:<id>` → новые теги) — отдельного
теста нет.

### Проверка

`go build ./...`; `go test ./core/build -run 'TestContractCorpusRuleTree|TestContractCorpusRuleDNS' -count=1`;
`go test ./core/state -run TestRuleRefsLiftedIntoBody -count=1`;
`go test ./core/backup -run TestBackupCorpus -count=1`. Владельцу — собрать
свой `config.json` и `sing-box check` (агент ядро не запускает).

## Волна 4 — форма

Условия-рамки по таблице реестра, «+ Condition», Rule group, ⚠, локализация;
разбор окна на файлы.

### Раскладка (пакет `dialogs`, без переноса пакетов)

| Файл | Содержит | ~строк |
|---|---|---|
| `ui/configurator/models/rule_conditions.go` (новый, без Fyne) | модель стопки: `RuleCondStack{Conds []CondValue; Groups []RuleGroup}`, `RuleGroup{Any bool; Not bool; Rules []RuleCondStack}`; `StackFromBody(body) (stack, ok)` (ключ вне таблицы, `process_*` двумя способами, лишняя форма `logical` → `ok=false` → JSON); `(stack).Body(outbound)` — SPEC §3: группы → `logical and`, одна группа без условий — сама группа; пустые поля и условия не пишутся | 350 |
| `ui/configurator/dialogs/add_rule_dialog.go` | `ShowAddRuleDialog`: окно, имя, выход, вкладки, `saveRule`, синхронизация JSON ↔ форма через модель | 450 |
| `ui/configurator/dialogs/rule_cond_stack.go` (новый) | виджет стопки: рамки, подписи «AND», «+ Condition» (меню из таблицы, добавленное пропадает, «Rule group» за чертой), удаление рамки | 250 |
| `ui/configurator/dialogs/rule_cond_frame.go` (новый) | рамка условия: поля по `field` таблицы (`lines`, `cidr_lines`, `ports`, `bool`, `enum_multi`, `processes`, `refs`), «any of», `one_of` у Process, пометка платформы у Wi-Fi по `model.Target` | 350 |
| `ui/configurator/dialogs/rule_cond_group.go` (новый) | рамка Rule group: «all of / any of», «Not», список правил (каждое — стопка), «+ Rule»; рекурсия без предела | 200 |
| `ui/configurator/dialogs/rule_cond_refs.go` (новый) | поле наборов: перенос `:175-182`, `normalizeRuleSetRef` (`:1733`), `ruleSetFileError` (`:1760`), «Add file…», подсказка SPEC §6, `ip_is_private` недоступна при `refs` | 150 |
| `ui/configurator/dialogs/rule_dialog_dns.go` (новый) | вкладка DNS: перенос `:728-957`; опции **не выключаются** при пустом зеркале — ⚠ «DNS options have no effect: the rule has no condition DNS can see» | 260 |
| `ui/configurator/dialogs/rule_dialog_preview.go` (новый) | `rulePreviewText` (`:1606-1727`) | 150 |
| `ui/configurator/dialogs/rule_process_picker.go` (новый) | окно выбора процессов (`:1208-1300`), `normalizeProcName`/`sort…`/`dedupe…` (`:1570-1603`) | 170 |
| `ui/configurator/dialogs/rule_type_selection.go` | удалить | −40 |
| `ui/configurator/dialogs/rule_set_options.go` | блок настроек наборов — под стопкой, виден, если в дереве есть `refs` | ±10 |

### Прочие файлы

| Файл | Изменение |
|---|---|
| `core/state/load_v6.go:214` | `cr.Rule` = тело с `refs` (заглушка `rule_set`, `:262-275`, снимается); `cr.Type` по `kind` |
| `ui/configurator/models/wizard_state_file.go:190` | `DetermineRuleType` — только для legacy `Type`; всё чтение вида — `state.RuleRefs`/модель |
| `ui/configurator/models/preset_ref_sync.go:252` | `customRuleStateToV6Rule` → `state.NewTreeRule(label, body, outbound)`; `RuleSets` модели — производное от `refs` (для кнопки `upd`, `routeConfigFromModel`, `modelHasRuleSetFiles`) |
| `ui/configurator/tabs/rules_tab.go:95`, `:396` | srs-признак — `refs` в дереве; ⚠ в метке строки, если DNS-опция включена и зеркало пустое |
| `ui/configurator/tabs/dns_unified_rules.go:119-170` | `ruleDNSMirrorText` от `RuleDNSMirrorForBody`; строка зеркала с ⚠ вместо пропуска; `ruleStateIsSRS` (`:145`) — по `refs` |
| `bin/locale/ru.json` | ключи новых подписей (условия, «+ Condition», «AND», «any of», «all of», «Not», «+ Rule», ⚠-текст, подсказка наборов, «does not apply on this machine») |

### Решения

- Модель стопки — в `models` (нет Fyne): тело ↔ стопка — данные, их зовут
  окно, вкладка JSON и Preview; виджеты только рисуют.
- `Params` для своих правил больше не пишутся: до диска они не доживали
  (CODEMAP §4.5); всё восстанавливается из тела.
- Вкладка JSON показывает **тело записи** (с `refs`), а не правило
  `config.json`: теги наборов — производное сборки, их видно в Preview.
  `srsRuleSetsForRawBody` (`:551`) уходит.
- Подписи условий — из таблицы реестра через `locale.T`; имён ключей в
  виджетах нет, виды полей — конечный набор `field`.
- Wi-Fi: пометка по платформе цели (`model.Target`), а не `runtime.GOOS`.

### Ловушки

- Порядок ключей нового тела — алфавит (`NewInlineRule`); тело, открытое
  формой и сохранённое без правок, не должно перезаписываться: сравнить
  стопку с исходной и при равенстве оставить исходные байты (иначе каждое
  открытие меняет файл и голдены).
- Цель — только у корня: JSON с `outbound`/`action` в под-правиле — ошибка
  сохранения с понятным текстом.
- `fyne` `Select`/`Check` в рекурсивной рамке: `OnChanged` до `Refresh` —
  обновление стопки и Preview по событию, без цикла `SetText` → `OnChanged`.
- Label без Wrapping раздувает окно (memory «Label min-width») — подписи рамок
  с `Truncation`.
- `conflictingRuleName` (`:1770`) оставить: одинаковые имена теперь
  разводятся тегом, но UI-сопоставление слотов (`RuleOrderFromAxis`) идёт по
  `StableRuleID`.

### Тест

Нет (UI). `go build ./ui/...`, показ владельцу на своей сборке
(`build_darwin.sh -i`).

## Волна 5 — закрытие

| Файл | Изменение |
|---|---|
| `docs/release_notes/upcoming.md` | EN Highlights / RU Основное: условия в форме, группы, теги = имя правила, `invert` в DNS-зеркале, ⚠, файл бэкапа 3 |
| `RELEASE_NOTES.md` | пункт UX |
| `docs/ARCHITECTURE.md`, `docs/WIZARD_STATE.md` | `refs` в теле, state v9, раздача тегов, выпуск дерева |
| `SPECS/188-…/IMPLEMENTATION_REPORT.md` | отчёт, отступления от плана |
| `SPECS/README.md` | строка 188 |
| папка | `188-F-N-…` → `188-F-C-…` после приёмки |

CI: `gh workflow run ci.yml --ref develop -f run_mode=tests` после волны 3 и
после волны 4. Релиз — после LxBox (SPEC §8).

## Вопросы и расхождения

1. **(а) Имя файла копии от тега не зависит.** `srstag.TagFromURL(ref)` =
   `<basename>-<h8(url)>` (`srstag.go:31`), путь собирают
   `CollectSrsCachedPaths`, загрузчик и GC одинаково. Перекачки при смене
   тегов не будет. Зависит от правила только **ключ** карты кэша
   (`StableRuleID`) и позиция. Умолчание: карта `ref → path` (волна 2).
2. **(б) Версия бэкапа есть — `lx_backup`, мажор.** Старый лаунчер на `3`
   отказывает **всему файлу** («backup format v3 is newer than supported v2 —
   update the app», `file.go:184-186`): правила не теряются молча, но не
   ввозятся и серверы. SPEC требует «предупреждает» — отказ строже. Минорной
   версии в формате нет (BACKUP.md §8 запрещает второй маркер). Умолчание:
   `lx_backup: 3` всегда, читатели берут 2 и 3. Аналогично `state.json` v9 —
   ради гейта копирования состояния машин.
3. **(в) Рукописный `rule_set` сегодня:** inline — едет как есть, висячий тег
   роняет ядро (`resolve_route.go:314`, проверки нет); srs — затирается
   сборкой (`:429`). SPEC §4.3 о рукописных тегах молчит. Умолчание: остаются
   первыми, за ними теги `refs` и inline; висячий → правило не выпускается
   (как SPEC 153 и «всё или ничего»).
4. **(г) Свои DNS-правила (`dns.rules`, `kind: user`) наборов не держат** —
   только ссылаются текстом на теги (`dns_user_rules.go:525`; `user:`
   форма не предлагает с 22.11, `:567-574`). Новая схема их задевает: старые
   `user:…` переписываются миграцией (как `MigrateRuleSetTagRefs` уже делает
   для DNS-правил), висячие снимает `cleanDanglingDNSRule`
   (`preset_merge.go:553`). DNS-зеркала в состоянии не хранятся.
5. **Перезапись ссылок `user:…` — в волне 3, не 2.** Новые теги выпускает
   волна 3; переписанная в волне 2 ссылка указывала бы на тег, которого
   сборка ещё не выпускает.
6. **`rule_dns_mirror_match`:** `network_type` и `source_ip_is_private` в
   списке уже есть; добавляется только `network_is_expensive`. По таблице SPEC
   Source port «в DNS — нет» — `source_port*` добавляются в
   `rule_dns_mirror_unseen` (подсказка). `user`, `user_id` остаются в списке
   вне таблицы.
7. **Действия зеркала над `logical`.** Ядро у `logical` не принимает
   `query_type`/`ip_version` (`sing-box-lx/option/rule_dns.go:237`); SPEC §5
   этого не называет. Умолчание: обёртка `and` с условием действия, плоский
   матчер — как сегодня. Матчер дерева в корпусе `rule_tree/` — нормативен
   (раннер `rule_dns` по-прежнему снимает матчер плоских кейсов).
8. **h8: «нижний регистр»** — хэш от строки ссылки после `TrimSpace`, hex в
   нижнем регистре; сама ссылка не опускается в нижний регистр (пути
   регистрозависимы). Подтвердить с LxBox в §182.
9. **Занятые теги при миграции.** При чтении состояния шаблона нет; занятыми
   считаются теги с префиксом `<preset_id>:` пресетов из `rules[]`. Сборка
   знает точный список. Расхождение — только у правила с именем, начинающимся
   на `<preset_id>:`; принять.
10. **«Тег первой ссылки плоского правила остаётся прежним»** (SPEC §4) верно
    для LxBox; у лаунчера тег меняется (`user:<id>` → имя), как и сказано
    выше в том же разделе и в критерии 5.
11. **DNS-вкладка выключает опции при пустом матчере**
    (`add_rule_dialog.go:917-925`), строка зеркала пропадает
    (`dns_unified_rules.go:123`). SPEC: правило сохраняется, ⚠. Умолчание:
    опции доступны всегда, ⚠ вместо подсказки «Unavailable…».
12. **Preview окна правила** собирает правило в одиночку с пустым шаблоном
    (`add_rule_dialog.go:1659`) и показал бы `B` вместо `B (2)`. Умолчание:
    все правила модели и шаблон.
13. **`Params` до диска не доживают** (CODEMAP §4.5) — вопрос «где хранить
    UI-состояние типа» снимается, форма читает всё из тела.
14. **Код предупреждения для пустого зеркала** не заводится: ⚠ — подпись UI
    (текст в TEMPLATE_LANG §6.11 для обеих сторон), в отчёт сборки не идёт.
15. **SPEC называет `add_rule_dialog.go` 1655 строк** — сейчас 1782,
    `ShowAddRuleDialog` `:78-1552`. На план не влияет.
