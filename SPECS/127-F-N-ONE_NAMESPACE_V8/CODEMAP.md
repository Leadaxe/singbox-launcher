# CODEMAP — 127-F-N-ONE_NAMESPACE_V8 (state v8 + бэкап 1.0)

Карта построена по develop на 14.09.2026 (HEAD `b6dc0984`, в ветке SPEC 122/123/125
и `fix/srs-multi-url`). Адреса `файл:строка` проверены `rg -n`/`sed -n` на этом
коммите; правка файла выше по тексту сдвигает всё, что ниже.

**Этап S1 выполнен** (ветка `spec127/state-v8`): пакет `core/state` переехал
на v8. Все адреса §1 (кроме §1.12 и §1.13) — ДО переезда; актуальные адреса
пакета — в §8 внизу. §1 оставлен как справка «что было», чтобы S2/S3 понимали,
откуда что уехало.

Норма целевых форм — `contract/docs/ONE_NAMESPACE.md`, задача — `SPEC.md` рядом.
Часть общего контекста (пресеты, ось порядка, DNS-формы, канон узла) лежит в
`SPECS/121-F-N-NODE_SECTIONS/CODEMAP.md` §1, §3.6, §4, §7, §11–§14 — **адреса там
устарели**, пользоваться только описательной частью.

Соглашение карты: «тело» = `body` в смысле нормы; «плоское тело» = v7-форма DNS,
где поля sing-box лежат рядом с `kind` на одном уровне.

---

## 0. Расхождение имён, которое надо держать в голове

Норма §2 говорит `refs[]`. В коде **сегодня два разных имени** для одного и того же:

| Слой | Имя | Адрес |
|---|---|---|
| Состояние v7 | `SrsURL` + `SrsURLs` (`srs_url`, `srs_urls`) | `core/state/rule_types.go:107-112` |
| Бэкап 0.12 | `Ref` + `Refs` (`ref`, `refs`) | `core/backup/types.go:432-438` |
| Схема контракта | `ref` + `refs` | `contract/schema/backup.schema.json:305-316` |

`Refs` как поле в состоянии **не существует** (`rg 'Refs\b' core/state` пусто).
Приведение к `refs[]` в v8 = переименование `srs_url`/`srs_urls` в состоянии и
подъём поля из тела наружу; бэкапная сторона уже в целевой форме.

---

## 1. Состояние v7

### 1.1 Корень

| Сущность | Адрес | Заметка |
|---|---|---|
| `SchemaVersion = SchemaVersionV7` | `core/state/state.go:40` | единственная точка «что пишет Save» |
| `type State` | `core/state/state.go:51-166` | canonical-поля + legacy-проекция |
| `State.Sources []Source` | `core/state/state.go:97` | ключ `sources` |
| `State.Rules []Rule` | `core/state/state.go:144` | ключ `rules`, историческое имя `RulesV6` |
| `State.DNS DNSOptions` | `core/state/state.go:152` | ключ на диске `dns_options` |
| `State.Migration *MigrationReport` | `core/state/state.go:165` | только в памяти, Save не пишет |
| `SchemaVersionV7 = 7` | `core/state/disk_v7.go:29` | **v8: рядом `SchemaVersionV8 = 8`** |
| `SchemaNameV7 = "sources_v7"` | `core/state/disk_v7.go:33` | `meta.schema`, он же мажор для remote-гейта |
| `migrationPurgesLegacy = true` | `core/state/disk_v7.go:42` | гейт шага 8 (снос легаси после успешной записи) |
| `type diskStateV7` | `core/state/disk_v7.go:46-54` | **порядок полей = порядок ключей файла** |
| `parseV7(data []byte) (*State, error)` | `core/state/disk_v7.go:61` | прямое чтение v7 |
| `normalizeSourceShape` в цикле | `core/state/disk_v7.go:67-75` | лишние для kind поля — warning, чужой kind — отказ |
| `legacyCustomRulesFromV6(s.Rules)` | `core/state/disk_v7.go:99` | v7-парс строит legacy-вид CustomRules |

### 1.2 Запись на диск

| Сущность | Адрес | Заметка |
|---|---|---|
| `(*State).Save(path string) error` | `core/state/save.go:27` | атомарная запись; `s.Version = SchemaVersionV7` на `:41` |
| `(*State).MarshalV7() ([]byte, error)` | `core/state/save.go:103` | `GET /state/full` и близнец машины |
| `(*State).marshalDisk() ([]byte, error)` | `core/state/save.go:125` | единственный сериализатор |
| `normalizeSectionsOfSources(s.Sources)` | `core/state/save.go:129` | секции нормализуются ПЕРЕД записью |
| `enc.SetEscapeHTML(false)` + `SetIndent("", "  ")` | `core/state/save.go:166-167` | **байт-в-байт значимо** |
| `bytes.TrimRight(buf.Bytes(), "\n")` | `core/state/save.go:173` | Encode дописывает \n, golden его не ждёт |
| `hasReferencedOutbounds` / `maybeBackupSPEC058` | `core/state/save.go:178`, `:193` | к v8 отношения не имеет, но вызывается на каждом Save |

### 1.3 Маршрутизация версий и гейт

| Сущность | Адрес | Заметка |
|---|---|---|
| `Load(path string) (*State, error)` | `core/state/load_router.go:30` | бэкап `.v6.bak` на `:46`, персист отчёта `:75`, purge `:80-89` |
| `Parse(data []byte)` | `core/state/load_router.go:96` | без путей: без raw-кэша и без бэкапа |
| `parseWithContext` — **switch по версии** | `core/state/load_router.go:105-132` | `meta>=7 → parseV7`; 6→`parseV6Legacy`; 5→`parseV5Legacy`; top 2–4→`parseLegacyAndMigrate` |
| `sniffSchemaVersion` | `core/state/load_router.go:136` | читает `version` и `meta.version` |
| `deriveLoadContext` | `core/state/load_router.go:150` | BinDir/SubsDir |
| `writeLegacyBackupOnce` | `core/state/load_router.go:196` | O_EXCL, суффикс `.v6.bak` |
| `SchemaMajor = SchemaVersionV7` | `core/state/schema_gate.go:28` | **v8: поднять вместе с `SchemaVersionV8`** |
| `SchemaVersionOfFile` / `OfBytes` | `core/state/schema_gate.go:39`, `:51` | шапка без миграции |
| `SchemaMismatchError` | `core/state/schema_gate.go:70-82` | текст называет обе версии |
| `CheckSchemaCompatible` | `core/state/schema_gate.go:90` | ниже — пропускаем, выше — отказ |

**Ключевое для v8.** `parseWithContext:107` ловит `meta >= 7` в `parseV7`. Если
оставить как есть, v8-файл прочитается как v7 и молча потеряет всё новое.
Ветка обязана стать `meta == 7 → parseV7 + migrateV7ToV8`, `meta >= 8 → parseV8`.

### 1.4 Правила (`rule_types.go`)

| Сущность | Адрес | Заметка |
|---|---|---|
| `RuleKind` | `core/state/rule_types.go:10` | строковый дискриминатор |
| `RuleKindPreset` / `Inline` / `Srs` | `:15`, `:19`, `:23` | имена в v8 не меняются |
| `type Rule` | `core/state/rule_types.go:46-72` | `Kind`, `Ref`, `Enabled`, `OrderNum`, `Body` |
| `Rule.Ref string \`json:"ref,omitempty"\`` | `:51` | **только kind=preset**; в v8 у srs сюда `refs[]` не кладём |
| `Rule.OrderNum *int \`json:"order_num,omitempty"\`` | `:68` | **→ `num`**; nil = не размечено |
| `Rule.Body json.RawMessage \`json:"body"\`` | `:72` | **RawMessage, не map** — порядок ключей тела сохраняется |
| `type PresetBody{Vars map[string]string}` | `:79-81` | **→ `vars` на уровне записи** |
| `type InlineBody{Name, Match, Outbound}` | `:84-93` | **→ `name` наружу, `body` = `Match` ∪ `{outbound}`** |
| `type SrsBody{Name, SrsURL, SrsURLs, Outbound}` | `:107-112` | **→ `name`, `refs[]`, `body{outbound}`** |
| `NewSrsBody(name, urls, outbound) SrsBody` | `core/state/rule_types.go:117` | канонизация: дедуп, первый в `SrsURL`, полный в `SrsURLs` при ≥2 |
| `(*SrsBody).URLs() []string` | `:124` | **единственный законный способ прочитать список** |
| `(*SrsBody).normalize()` | `:135` | любая комбинация → канон |
| `dedupNonEmpty` | `:153` | сохраняет порядок |
| `(*Rule).DecodeBody() (interface{}, error)` | `core/state/rule_types.go:186` | **дispatcher; вся валидация kind здесь** |
| ветка preset (ref required) | `:188-201` | `Vars` никогда не nil на выходе |
| ветка inline (`body.Name` required) | `:203-214` | |
| ветка srs (`Name` + ≥1 URL required) | `:216-231` | вызывает `normalize()` на `:227` |

### 1.5 Ось порядка (`rule_order.go`)

| Сущность | Адрес | Заметка |
|---|---|---|
| `UserRuleNumStart = 1000` / `End = 1100` | `core/state/rule_order.go:34`, `:36` | зона пользователя |
| `DefaultRuleNum` | `:38` | номер неразмеченного при сортировке |
| `MinSortableRuleNum = 1` | `:46` | единственная жёсткая граница (голова на 0) |
| `NodeRuleDefaultNum = 945` | `:55` | старт правила узла (SPEC 121 §10.1) |
| `type RuleOrderSpec{Num, Sortable, DefaultEnabled}` | `:61-69` | вход из шаблона |
| `MarkRuleOrder(rules, specs) bool` | `core/state/rule_order.go:88` | разметка nil → номер; заканчивается сортировкой `:111` |
| `sortRulesByNumInPlace` | `:117` | стабильная |
| `SortRulesByNum(rules) []Rule` | `core/state/rule_order.go:127` | копия + стабильная сортировка |
| `ruleNum(r Rule) int` | `:136` | **читает `r.OrderNum`; в v8 — `r.Num`** |
| `SeedRequiredRules` | `core/state/rule_order.go:151` | тело сидируемого правила — литерал `{"vars":{}}` на `:182` |
| `DedupePresetRules` | `core/state/rule_order.go:194` | должен идти ПЕРЕД seed (D-053в) |
| `NormalizeRuleOrder(rules, specs) []Rule` | `core/state/rule_order.go:222` | дедуп → seed → разметка → сортировка; идемпотентна |
| `NextUserRuleNum(rules) int` | `core/state/rule_order.go:234` | конец занятой части зоны |
| `PlaceRuleAfter` / `PlaceRuleBefore` | `:287`, `:325` | драг-н-дроп |
| `placeRuleAt` | `core/state/rule_order.go:335` | ленивый сдвиг сплошного блока; читает/пишет `OrderNum` на `:350`, `:353`, `:374-375`, `:379` |

### 1.6 Идентичность правила (`rule_identity.go`)

| Сущность | Адрес | Заметка |
|---|---|---|
| `StableRuleID(r Rule) string` | `core/state/rule_identity.go:29` | preset → `Ref`; inline/srs → `sanitizeIDPart(body.Name)`; иначе `"unnamed"` |
| вызов `r.DecodeBody()` внутри | `:33` | **identity зависит от разбора тела** |
| `SrsRuleSetTag(id string, i int) string` | `core/state/rule_identity.go:58` | `user:<id>` для i=0, `user:<id>:N+1` дальше |
| `sanitizeIDPart` | `core/state/rule_identity.go:74` | a-zA-Z0-9-_ остаются, пробел → `-`, прочее (в т.ч. кириллица) отбрасывается |

**Для v8 критично:** `StableRuleID` считается от `name`, а `name` в v8 переезжает
из тела наружу — **значение не меняется, если перенос сохранил строку**. Именно
на этом id висит srs-кэш (`CollectSrsCachedPaths`, `core/build/preset_merge.go:708`)
и теги `rule_set` в конфиге. Любое изменение алгоритма меняет имена в конфиге →
golden красный.

### 1.7 DNS (`dns_options.go`) — плоская сериализация

| Сущность | Адрес | Заметка |
|---|---|---|
| `DNSServerKind` + `Template/Preset/User` | `core/state/dns_options.go:40`, `:45`, `:50`, `:54` | |
| `DNSRuleKind` + `Preset/User` | `:58`, `:63`, `:67` | |
| `type DNSServer{Kind, Tag, Ref, Enabled, Body}` | `core/state/dns_options.go:75-96` | **без json-тегов: сериализацию делают Marshal/Unmarshal вручную** |
| `type DNSRule{Kind, Ref, Enabled, Body}` | `:99-111` | у правила тега нет |
| `type DNSOptions{Strategy, Final, DefaultDomainResolver, Servers, Rules}` | `:128-135` | scalars дублируют `state.vars[]`, на диск идут из vars |
| **`(DNSServer).MarshalJSON`** | `core/state/dns_options.go:141` | **плоско: тело разливается по верхнему уровню, `:156-170`** |
| — `tag` предпочитается полю `Tag` | `:161-167` | тело тоже может нести `tag`, поле сильнее |
| **`(*DNSServer).UnmarshalJSON`** | `core/state/dns_options.go:177` | остаток карты → `Body`, **включая `tag`** (`:196-202`) |
| **`(DNSRule).MarshalJSON`** | `core/state/dns_options.go:208` | зеркало |
| **`(*DNSRule).UnmarshalJSON`** | `core/state/dns_options.go:228` | |
| `FindServerByTag` / `ByRef` / `FindRuleByRef` | `:258`, `:271`, `:284` | |
| `(*DNSOptions).IsEmpty()` | `:298` | |

**Для v8:** четыре метода на `:141`, `:177`, `:208`, `:228` — и есть весь переезд
DNS на `body`. Заменяются на обычные struct-теги (`tag` снаружи, `body` объектом)
плюс legacy-чтение плоской формы в миграции. Заметить: `Unmarshal` кладёт `tag`
и в `Body` тоже (`:198-202` исключает только `kind`/`ref`/`enabled`) — это второй
источник тега, и он уезжает в конфиг через `resolve_dns.go:307-308`.

### 1.8 Секции узла (SPEC 121 §10)

| Сущность | Адрес | Заметка |
|---|---|---|
| `SelfPlaceholder = "@self"` | `core/state/node_sections.go:52` | целой строкой |
| `SelfPlaceholderBraced = "@{self}"` | `:53` | внутри строки |
| `type NodeSections{Rules []Rule, DNS *NodeSectionsDNS}` | `core/state/node_sections.go:57-63` | **те же типы, что в корне** — переезд формы автоматически меняет и секции |
| `type NodeSectionsDNS{Servers []DNSServer, Rules []DNSRule}` | `:66-69` | |
| `IsEmpty` / `HasRules` / `DNSServers` / `DNSRules` / `SetDNS` / `Clone` | `:72`, `:88`, `:93`, `:100`, `:108`, `:121` | |
| `cloneRule(r Rule) Rule` | `core/state/node_sections.go:148` | копирует `OrderNum` по значению |
| `(*Node).NormalizeNodeSections()` | `core/state/node_sections.go:180` | вызов `dropForeignKinds`, пустой набор → nil |
| `(*NodeSections).dropForeignKinds(nodeTag)` | `core/state/node_sections.go:197` | **rules: только inline\|srs; DNS: только user** |
| `normalizeSectionsOfSources(sources)` | `core/state/node_sections.go:244` | корневые узлы + члены контейнеров |
| `ReadNodeSections(raw) (*NodeSections, error)` | `core/state/node_sections.go:258` | вкладка JSON: чужой kind — **отказ**, не отбрасывание |
| `Node.Sections *NodeSections \`json:"sections,omitempty"\`` | `core/state/sources_v7.go:174` | |

### 1.9 Перевод sing-box ↔ хранимая форма секций

| Сущность | Адрес | Заметка |
|---|---|---|
| `type SingboxNodeFragments{NodeTag, DNSServers, DNSRules, RouteRules}` | `core/state/node_sections_convert.go:38-47` | вход перевода |
| `NodeSectionsFromSingbox(in) (*NodeSections, error)` | `core/state/node_sections_convert.go:55` | **единственный перевод внутрь** |
| — DNS-сервер: `tag` из тела → поле `Tag`, `delete(body,"tag")` | `:70-77` | уже целевое поведение |
| — DNS-правило: тело целиком | `:79-91` | уже целевое |
| — route-правило: `num` от `NodeRuleDefaultNum`, +1 на каждое | `:93-110` | |
| `nodeSectionRuleFromBody(body, idx, num) (*Rule, error)` | `core/state/node_sections_convert.go:123` | `outbound` или `@self`; `match` = всё остальное; `json.Marshal(InlineBody{...})` на `:144` |
| `nodeSectionFragmentBody(raw, nodeTag, where, idx, routeRule)` | `core/state/node_sections_convert.go:162` | `rule_set` в секции — отказ; `@var` кроме `@self` — отказ |
| `NodeSectionsToSingbox(sections) SingboxNodeFragments` | `core/state/node_sections_convert.go:209` | **обратный эмиттер (пара к парсеру)** |
| `nodeSectionRuleToSingbox(r Rule)` | `core/state/node_sections_convert.go:242` | `DecodeBody` на `:243`, ветки `*InlineBody` `:249`, `*SrsBody` `:256` |
| — ветка `*SrsBody` пишет **только `b.SrsURL`** | `core/state/node_sections_convert.go:256-262` | **`SrsURLs` теряются** — см. ловушку 20 |
| `nodeSectionForeignVars(raw)` | `:281` | |

### 1.10 Плейсхолдер `@self`

| Сущность | Адрес | Заметка |
|---|---|---|
| `SubstituteSelf(raw []byte, finalTag string) []byte` | `core/state/selfvar.go:51` | **по сырому JSON, порядок ключей цел** |
| `SubstituteSelfInString(s, finalTag)` | `core/state/selfvar.go:79` | |
| `SubstituteSelfInMap(in, finalTag)` | `core/state/selfvar.go:97` | |
| `rewriteJSONStringValues(raw, fn)` | `core/state/selfvar.go:131` | |

### 1.11 Миграция v6→v7 — образец для v7→v8

| Сущность | Адрес | Заметка |
|---|---|---|
| `type LoadContext{StatePath, BinDir, SubsDir}` | `core/state/migration_v6_to_v7.go:35` | |
| `type migrationV7` | `core/state/migration_v6_to_v7.go:46` | носитель прохода |
| `migrateLegacyStateToV7(s, fromVersion, lc, legacy)` | `core/state/migration_v6_to_v7.go:78` | **точка входа — образец сигнатуры для v8** |
| шаги: `adoptWizardMarkerFolds`, `materializeSources`, `migrateChainHops`, `migrateDetours`, `migrateFolds`, `applyRenames` | `:123`, `:163`, `:482`, `:553`, `:649`, `:727` | последовательность внутри `migrateLegacyStateToV7` |
| `applyRenames` → перепись `Outbound` в телах правил | `core/state/migration_v6_to_v7.go:741-774` | `DecodeBody` `:743`, ветки `*InlineBody` `:749`, `*SrsBody` `:754`, `*PresetBody` (`Vars`) `:759`, **обратный `json.Marshal(body)` на `:771`** |
| `purgeLegacyAfterMigration(s, lc)` | `core/state/migration_v6_to_v7.go:860` | необратимо, только после успешного Save |
| `type MigrationReport{FromVersion, Warnings, BackupPath}` | `core/state/migration_report.go:43-51` | |
| `(*MigrationReport).add(format, args...)` | `core/state/migration_report.go:55` | **`locale.Tf`: ключи английские, перевод здесь**; лог `state migration v%d→v7` на `:64` — **строка знает «v7» жёстко** |
| `(*MigrationReport).Text(statePath)` | `core/state/migration_report.go:78` | заголовок `schema v%d -> v%d` с `SchemaVersion` |
| `PersistMigrationReport(binDir, rep, statePath)` | `core/state/migration_report.go:106` | дописывает в `bin/migration_report.txt` |
| `MigrationReportFileName` | `:32` | |
| `MigrationHooks` и прочее | `core/state/migration_hooks.go:15-82` | материализация подписок при миграции; для v8 не нужна |
| `migrateV5ToV6` — правила из CustomRules | `core/state/migration_v5_to_v6.go:31`, `:47`, `:71` | ещё один конструктор тел (`PresetBody`, `SrsBody`, `InlineBody`) |

### 1.12 Узел и источник v7 (контекст, менять не надо)

| Сущность | Адрес | Заметка |
|---|---|---|
| `type Node` | `core/state/sources_v7.go:116-176` | `kind`, `tag`, `enabled`, `origin`, `body`, `detour`, `hops`, `group`, `service`, `reason`, `sections` |
| `Node.Body json.RawMessage` | `:135` | **узел уже в целевой форме** |
| `type Source` | `core/state/sources_v7.go:213-290` | union по kind; `Nodes []Node` `:227` |
| `type Origin{Kind, Raw, SubURL}` | `:46-54` | |
| `type NodeLink{FolderID, Tag}` | `:38-43` | |
| `normalizeSourceShape(s) ([]string, error)` | `core/state/sources_v7.go:397` | |
| `normalizeNodeShape(n, name) []string` | `core/state/sources_v7.go:492` | |

### 1.13 ПОЛНЫЙ список читателей v7-формы правил и DNS (кроме `core/state`)

Это главная таблица: всё, что придётся тронуть волной 1. Тесты — отдельно в §6.

**`core/build`**

| Адрес | Что там |
|---|---|
| `core/build/resolve_route.go:162` | switch `RuleKindPreset` |
| `core/build/resolve_route.go:164` | switch `RuleKindInline` |
| `core/build/resolve_route.go:166` | switch `RuleKindSrs` |
| `core/build/resolve_route.go:189`, `:194` | `DecodeBody` → `*PresetBody` (vars пресета) |
| `core/build/resolve_route.go:252` | `OrderNum: ruleOrderNum(rule)` у preset-правила |
| `core/build/resolve_route.go:259`, `:264` | `DecodeBody` → `*InlineBody` |
| `core/build/resolve_route.go:265-274` | `ib.Match` копируется в карту правила |
| `core/build/resolve_route.go:275` | `ApplyOutboundToRule(routeRule, ib.Outbound)` |
| `core/build/resolve_route.go:280` | `OrderNum` inline-правила |
| `core/build/resolve_route.go:291`, `:296` | `DecodeBody` → `*SrsBody` |
| `core/build/resolve_route.go:298` | `sb.URLs()` — список наборов |
| `core/build/resolve_route.go:300-312` | «все наборы или ни одного»: `len(paths) != len(urls)` → правило пропускается целиком |
| `core/build/resolve_route.go:317-338` | **запись `rule_set` на каждый URL**, теги `SrsRuleSetTag(id, i)` |
| `core/build/resolve_route.go:339-343` | один тег → строка, несколько → массив в `body.rule_set` |
| `core/build/resolve_route.go:344` | `ApplyOutboundToRule(routeRule, sb.Outbound)` |
| `core/build/resolve_route.go:359-364` | `ruleOrderNum(r)` — читает `r.OrderNum` |
| `core/build/resolve_route.go:81-84` | `ResolvedRouteRule.OrderNum int` — поле сборочной проекции |
| `core/build/resolve_dns.go:228`, `:232` | `DecodeBody` → `*PresetBody` для DNS-пресета |
| `core/build/resolve_dns.go:300-315` | **`srv.Kind != DNSServerKindUser` → skip; тело копируется, `tag` дописывается из `srv.Tag`, если его нет в теле** |
| `core/build/resolve_dns.go:324` | `case DNSRuleKindPreset` |
| `core/build/resolve_dns.go:338-345` | `case DNSRuleKindUser` — тело копируется как есть |
| `core/build/resolve_dns.go:400` | `tagFromBody(body)` |
| `core/build/resolve_dns.go:432-441` | `stateTemplateEnabled` — ищет `DNSServerKindTemplate` по `Tag` |
| `core/build/resolve_dns.go:446-455` | `statePresetServerEnabled` — по `Ref` |
| `core/build/resolve_dns.go:459-468` | `statePresetRuleEnabled` — по `Ref` |
| `core/build/preset_merge.go:206` | `PresetMergeContext.NodeSections []NodeSectionSet` |
| `core/build/preset_merge.go:214-238` | `rulesWithNodeSections()` — конкатенация корня и секций |
| `core/build/preset_merge.go:246-266` | `dnsWithNodeSections()` |
| `core/build/preset_merge.go:268-289` | `hasNodeRouteRules` / `hasNodeDNSFragments` |
| `core/build/preset_merge.go:330` | `state.State{Rules: …, DNS: …}` собирается на лету |
| `core/build/preset_merge.go:412` | то же в `MergePresetsIntoDNS` |
| `core/build/preset_merge.go:571` | `CollectEmittedRouteRuleSetTags(routeRaw, routeCfg, ctx)` |
| `core/build/preset_merge.go:608` | и там же синтетический `state.State` |
| `core/build/preset_merge.go:43` | `convertPresetRuleSetRemoteToLocal(rs, execDir, resourceDir, srsLocalDir)` — **srs URL → локальный файл для пресетов** |
| `core/build/preset_merge.go:93` | `cleanDanglingRuleSetInRule` |
| `core/build/preset_merge.go:150`, `:152` | `SRSTagFromURL` / `srsTagFromURLLocal` → `srstag.TagFromURL` |
| `core/build/preset_merge.go:708-742` | **`CollectSrsCachedPaths(rules, execDir, resourceDir)`**: `Kind != RuleKindSrs` skip `:714`, `DecodeBody` `:717`, `*SrsBody` `:721`, `sb.URLs()` `:725`, **ключ карты = `StableRuleID(r)` `:739`** |
| `core/build/preset_merge.go:746-753` | `hasAnyV6Rule(rules)` — есть ли enabled правило |
| `core/build/parsed_cache.go:47` | `ParsedCache.NodeSections []NodeSectionSet` |
| `core/build/parsed_cache.go:55-64` | `type NodeSectionSet{FinalTag, Link, Sections}` |
| `core/build/parsed_cache.go:70-87` | `RulesWithSelf()` — **копирует `OrderNum` по значению `:79-83`**, `SubstituteSelf(r.Body, …)` `:84` |
| `core/build/parsed_cache.go:91-105` | `DNSServersWithSelf()` — `Tag` и `Body` отдельно |
| `core/build/parsed_cache.go:106-119` | `DNSRulesWithSelf()` |
| `core/build/sync_outbounds.go:99`, `:103` | `DecodeBody` → `*PresetBody` |
| `core/build/sync_outbounds.go:218`, `:222` | то же во втором проходе |
| `core/build/migrate_outbounds_spec058.go:93`, `:97` | `DecodeBody` → `*PresetBody` |
| `core/build/build.go:272` | `CollectEmittedRouteRuleSetTags` в конвейере |

**`core/config` и корень `core/`**

| Адрес | Что там |
|---|---|
| `core/config_service.go:493-509` | orphan-GC .srs: `Kind != RuleKindSrs` skip, `DecodeBody`, `*SrsBody`, `sb.URLs()` → `build.SRSTagFromURL(u)` |
| `core/debugapi/state_endpoints.go:188` | PATCH правил: `(&req.Rules[i]).DecodeBody()` — валидация формы на входе API |
| `core/state/load_v6.go:204-240` | `legacyCustomRulesFromV6`: `DecodeBody` `:204`, `*InlineBody` `:210`, `*SrsBody` `:225`, **по записи `rule_set` на каждый URL** |

**`core/backup`** — см. §4.

**`ui/configurator`** — см. §3.

---

## 2. Сборка

### 2.1 Маршрут

| Сущность | Адрес | Заметка |
|---|---|---|
| `ResolveRoute(...)` | `core/build/resolve_route.go:116` | |
| `ResolveRouteWithGlobals(...)` | `core/build/resolve_route.go:130` | |
| **switch по `rule.Kind`** | `core/build/resolve_route.go:160-168` | три ветки |
| `resolvePresetRouteRule(...)` | `core/build/resolve_route.go:175` | expand пресета → rule_set + правило |
| `resolveInlineRouteRule(out, rule)` | `core/build/resolve_route.go:258` | `Match` → карта правила, `Outbound` поверх |
| `resolveSrsRouteRule(out, rule, srsCachedPaths, emittedTags)` | `core/build/resolve_route.go:285` | **`rule_set` вписывается здесь, в состоянии его нет** |
| `type ResolvedRouteRuleSet{Tag, Body, Source, SrsID, Enabled, Skipped, SkippedReason}` | `core/build/resolve_route.go:34-62` | |
| `type ResolvedRouteRule{… OrderNum int}` | `:81-84` | |
| `ruleOrderNum(r)` | `core/build/resolve_route.go:359` | |
| `outboundutil.ApplyOutboundToRule` | вызовы `:275`, `:344` | **`outbound` vs `action` решается здесь** |

Норма §2 «`rule_set` в `body` не хранится» уже выполнена: в состоянии `rule_set`
нет, его вписывает `resolveSrsRouteRule:343`. Переезд на `refs[]` меняет только
источник списка URL (`sb.URLs()` → `rule.Refs`).

### 2.2 DNS

| Сущность | Адрес | Заметка |
|---|---|---|
| `ResolveDNS(state, td, templateVars, target) ResolvedDNS` | `core/build/resolve_dns.go:161` | |
| Pass: user-серверы | `core/build/resolve_dns.go:298-318` | тело + `tag` из поля |
| Pass: правила preset/user | `core/build/resolve_dns.go:320-350` | |
| `substitutePresetDNSServer` | `core/build/resolve_dns.go:494` | `"tag": ds.Tag` на `:496` |
| `substituteTemplateDNSServer` | `:548` | |
| `substitutePresetDNSRules` | `:582` | |
| `buildPresetVarsMap(p, userVars, target)` | `core/build/resolve_dns.go:474` | userVars = `state.Rules[].Body.Vars` |
| `bodyRequired` / `bodyEnabled` / `tagFromBody` | `:406`, `:416`, `:400` | |

### 2.3 Инъекция секций и пресетов

| Сущность | Адрес | Заметка |
|---|---|---|
| `MergePresetsIntoRoute(routeRaw, ctx)` | `core/build/preset_merge.go:309` | |
| `MergePresetsIntoDNS(dnsRaw, ctx)` | `core/build/preset_merge.go:407` | |
| `CollectEmittedRouteRuleSetTags(routeRaw, routeCfg, ctx)` | `core/build/preset_merge.go:571` | множество тегов, реально доехавших до конфига |
| `convertPresetRuleSetRemoteToLocal(rs, execDir, resourceDir, srsLocalDir)` | `core/build/preset_merge.go:43` | remote→local для пресетных наборов |
| `cleanDanglingDNSRule(rule, validTags)` | `core/build/preset_merge.go:636` | |
| `repairDanglingDNSRefs(dns, servers, dnsRules)` | `core/build/preset_merge.go:767` | |
| `pruneDNSGroupMembers(servers)` | `core/build/preset_merge.go:835` | |
| `templateLikeFromCtx(ctx)` | `core/build/preset_merge.go:525` | |
| `hasNonSortablePreset(presets)` | `core/build/preset_merge.go:291` | |

---

## 3. UI-модели правил и DNS

### 3.1 Модели

| Сущность | Адрес | Заметка |
|---|---|---|
| `type RuleState{Rule, Enabled, SelectedOutbound, OrderNum *int}` | `ui/configurator/models/rule_state.go:31-41` | inline/srs-правило в UI; тело — в `Rule` (`wizardtemplate.TemplateSelectableRule`) |
| `type PresetRefState{Ref, Enabled, Vars, DNSServerEnabled, DNSRuleEnabled, OrderNum *int}` | `ui/configurator/models/preset_ref_state.go:12-38` | `Vars` уже вне тела — целевая форма |
| `(*PresetRefState).Clone` | `:42` | |
| `IsDNSServerEnabled` / `IsDNSRuleEnabled` / `Set*` | `:72`, `:84`, `:92`, `:103` | |
| `type NodeRuleRef{Link, Index, Enabled, OrderNum *int, Name}` | `ui/configurator/models/node_rule_ref.go:31-47` | строка правила узла |
| `(*NodeRuleRef).NodeTag` / `Clone` | `:51`, `:59` | |
| `SeedNodeRuleRefs(m *WizardModel) bool` | `ui/configurator/models/node_rule_ref.go:80` | пересев из Sources |
| `nodeRuleDisplayName(r, nodeTag)` | `ui/configurator/models/node_rule_ref.go:141` | **`DecodeBody` `:142`, `*InlineBody` `:147`, `*SrsBody` `:149`** |
| `SyncNodeRuleRefsToSources(m)` | `:161` | обратная запись enabled/num в секции узла |
| `NodeRuleRefNodeEnabled` / `FindNodeByLink` | `:186`, `:198` | |
| `type DNSUserRule{Enabled bool, Body map[string]interface{}}` | `ui/configurator/models/dns_user_rule.go:22-24` | **уже `body`** |
| `DNSUserRulesFromText` / `ToText` | `:33`, `:63` | вкладка DNS как текст |
| `WizardModel.PresetRefs []*PresetRefState` | `ui/configurator/models/wizard_model.go:112` | |
| `WizardModel.NodeRuleRefs []*NodeRuleRef` | `ui/configurator/models/wizard_model.go:123` | производный от Sources |
| `WizardModel` DNS-порядок (`DNSRuleOrder`) | `ui/configurator/models/wizard_model.go:153` (комментарий) | слоты DNS |

### 3.2 Слоты и ось

| Сущность | Адрес | Заметка |
|---|---|---|
| `RuleSlotKind` + `SlotKindPresetRef`/`SlotKindNodeRef` | `ui/configurator/models/rule_slot.go:20`, `:26`, `:28` | |
| `RuleSlot` и построение по умолчанию | `ui/configurator/models/rule_slot.go:51-58` | CustomRules → PresetRefs → NodeRuleRefs |
| реконсиляция слотов | `:72-118` | |
| `DNSRuleSlotKind` + `DNSSlotKindPresetRef` | `ui/configurator/models/dns_rule_slot.go:25`, `:31` | |
| построение DNS-слотов | `ui/configurator/models/dns_rule_slot.go:54-118` | |
| `(*WizardModel).slotOrderNum(s)` | `ui/configurator/models/rule_order_axis.go:25` | читает `OrderNum` у `PresetRefs` `:29`, `NodeRuleRefs` `:37` |
| `(*WizardModel).setSlotOrderNum(s, num)` | `ui/configurator/models/rule_order_axis.go:44` | пишет `:48`, `:56` |
| `(*WizardModel).axisProxyRules() []corestate.Rule` | `ui/configurator/models/rule_order_axis.go:66` | **строит синтетические `state.Rule` для оси**; `r.Ref` из `PresetRefs` `:74` |
| `applyAxisAfterMove(movedPos)` | `:100` | |
| `markProxyGaps(proxy)` | `:141` | |
| `EnsureRuleOrderNums(m)` | `ui/configurator/models/rule_order_axis.go:165` | |
| `SortRuleOrderByAxis(m)` | `:184` | |
| `axisNum(r)` | `:203` | |
| `(*WizardModel).isSortableAxisRule(r)` | `:213` | |
| `NextRuleOrderNum(m)` / `PresetRuleOrderNum(m, ref)` | `:228`, `:241` | |

### 3.3 Синхронизация UI ↔ состояние (`preset_ref_sync.go`)

| Сущность | Адрес | Заметка |
|---|---|---|
| `EmitStateRulesWithoutOrder(presetRefs, customRules) []state.Rule` | `ui/configurator/models/preset_ref_sync.go:31` | preset-ref'ы первыми |
| **`EmitStateRulesInAxisOrder(order, presetRefs, customRules) []state.Rule`** | `ui/configurator/models/preset_ref_sync.go:65` | **UI → state в порядке оси; главный писатель `state.Rules`** |
| `jsonMarshalPreset(vars)` | `:118` | `json.Marshal(state.PresetBody{Vars: vars})` `:119` |
| **`RuleOrderFromAxis(rules, presetRefs, customRules, nodeRuleRefs) []RuleSlot`** | `ui/configurator/models/preset_ref_sync.go:140` | **state → UI**; `copyOrderNum(r.OrderNum)` `:166`; узловые строки `:186-` |
| эмиссия inline-правила | `ui/configurator/models/preset_ref_sync.go:261` | `json.Marshal(state.InlineBody{Name: rs.Rule.Label})` |
| эмиссия srs-правила | `:296` | `json.Marshal(state.NewSrsBody(label, urls, outbound))` |
| эмиссия inline с match | `:311` | `json.Marshal(state.InlineBody{...})` |
| `SyncDNSOptions…` (сборка `DNSOptions` из слотов) | `ui/configurator/models/preset_ref_sync.go:372-384` | `cfg.Rules = buildDNSRulesFromOrder(order, presetRefs, userRules)` |
| `buildDNSRulesFromOrder(...)` | `ui/configurator/models/preset_ref_sync.go:508` | эмиссия `state.DNSRule` |
| `DNSRuleOrderFromStateRules(rules, presetRefs)` | `ui/configurator/models/preset_ref_sync.go:608` | state → DNS-слоты |
| **`SyncPresetRefsToStateRules(presetRefs) []state.Rule`** | `ui/configurator/models/preset_ref_sync.go:660` | `json.Marshal(state.PresetBody{Vars: vars})` `:674` |
| **`SyncStateRulesToPresetRefs(rules) []*PresetRefState`** | `ui/configurator/models/preset_ref_sync.go:689` | `DecodeBody` `:698`, `*PresetBody` `:702` |

### 3.4 Вызывающие в `business/`, `tabs/`, `presentation/`

| Адрес | Что там |
|---|---|
| `ui/configurator/presentation/presenter_state.go:140` | `EmitStateRulesInAxisOrder(RuleOrder, PresetRefs, CustomRules, …)` — **запись состояния** |
| `ui/configurator/presentation/presenter_state.go:157` | DNS-синк из `PresetRefs` |
| `ui/configurator/presentation/presenter_state.go:174` | `applyPresetEnabledOverrides(&state.DNS, PresetRefs)` |
| `ui/configurator/presentation/presenter_state.go:336-339` | `restorePresetRefs(stateFile)` и порядок после него |
| `ui/configurator/presentation/presenter_state_helpers.go:86-96` | `restorePresetRefs` → `SyncStateRulesToPresetRefs(state.Rules)` |
| `ui/configurator/presentation/presenter_state_helpers.go:100` | `SeedNodeRuleRefs(p.model)` |
| `ui/configurator/presentation/presenter_state_helpers.go:103` | `populatePresetEnabledFromState(PresetRefs, state.DNS)` |
| `ui/configurator/presentation/presenter_state_helpers.go:109` | `RuleOrderFromAxis(state.Rules, …)` |
| `ui/configurator/presentation/presenter_state_helpers.go:134` | `DNSRuleOrderFromStateRules(state.DNS.Rules, PresetRefs)` |
| `ui/configurator/presentation/presenter_sync.go:250` | ещё один `EmitStateRulesInAxisOrder` |
| `ui/configurator/presentation/preset_ref_helpers.go:26-31` | `applyPresetEnabledOverrides(dns, presetRefs)` |
| `ui/configurator/presentation/preset_ref_helpers.go:66-68` | `populatePresetEnabledFromState` |
| `ui/configurator/presentation/preset_ref_helpers.go:251-268` | `AppendTo` — новый `PresetRefState` |
| `ui/configurator/business/create_config.go:124`, `:133` | сборка правил и DNS для превью |
| `ui/configurator/business/parser.go:96` | то же в парсере |
| `ui/configurator/business/preset_bundled_dns.go:31` | обход `PresetRefs` за DNS-тегами |
| `ui/configurator/business/outbound.go:160-169` | список целей по активным preset-ref'ам |
| `ui/configurator/business/direction_rename.go:181` | перепись тега в `PresetRefs` |
| `ui/configurator/business/node_move.go:178` | то же при переносе узла |
| `ui/configurator/business/node_pool.go:169` | `SeedNodeRuleRefs` |
| `ui/configurator/configurator.go:719` | `SeedNodeRuleRefs(model)` |
| `ui/configurator/tabs/rules_unified_rows.go:56-94`, `:165`, `:342` | строки правил по слотам |
| `ui/configurator/tabs/dns_unified_rules.go:54`, `:204` | строки DNS |
| `ui/configurator/tabs/dns_user_rules.go:424`, `:483-486` | пользовательские DNS-правила |
| `ui/configurator/tabs/dns_preset_bundled.go:74-76`, `:185-188` | `SyncPresetRefsToStateRules(m.PresetRefs)` |
| `ui/configurator/tabs/library_rules_dialog.go:74-87`, `:184` | добавление пресетов |
| `ui/configurator/tabs/preset_ref_edit_dialog.go:42-45`, `:63`, `:387` | редактор пресет-правила |
| `ui/configurator/tabs/rules_tab.go:166` | синк `RuleOrder` |
| `ui/configurator/outbounds_configurator/configurator_helpers.go:268` | `EmitStateRulesInAxisOrder` |
| `ui/configurator/dialogs/add_rule_dialog.go:482-511` | `buildSRSRuleSetsAndTags` — **диалог правила принимает список URL** и строит `{tag,type:remote,format:binary,url}` |
| `ui/configurator/dialogs/add_rule_dialog.go:258-262` | переоткрытие правила: URL достаются из `editRule.Rule.RuleSets` |
| `ui/configurator/dialogs/add_rule_dialog.go:642` | валидация «хотя бы один SRS URL» |
| `ui/configurator/dialogs/add_rule_dialog.go:787`, `:790-792`, `:806-808` | запись `Rule.Rule` и `Rule.RuleSets` |
| `ui/configurator/dialogs/add_rule_dialog.go:816` | `OrderNum: NextRuleOrderNum(model)` у нового правила |
| `ui/configurator/tabs/preset_ref_convert.go:43`, `:92`, `:112`, `:160` | конвертация preset→custom, три записи `OrderNum` |
| `ui/configurator/tabs/preset_ref_convert.go:99-103`, `:130-145` | remote `rule_set` и `outbound`/`action`/`method` из фрагментов пресета |
| `ui/configurator/presentation/preset_ref_helpers.go:199-213` | **`s.Body` разворачивается обратно в плоский wizard-JSON** (пропуская kind/ref/enabled/tag) |
| `ui/configurator/presentation/preset_ref_helpers.go:223-240` | user-DNS-правила → `DNSRulesText`, снимая kind/ref/enabled |
| `ui/configurator/presentation/preset_ref_helpers.go:132-160` | `applyDNSServerEnabledFromState` правит `enabled` внутри блобов `model.DNSServers` |
| `ui/configurator/business/node_sections.go:118`, `:131` | чтение `srv.Body` / `r.Body` секций узла |
| `ui/configurator/tabs/dns_user_rules.go:119`, `:285`, `:509-512` | редактор пользовательского DNS-правила: клон/запись/превью `Body` |
| `ui/configurator/tabs/dns_unified_rules.go:85-90`, `:129`, `:145-148` | строки DNS, сводка по `Body`, синк `DNSRulesText` |
| `ui/configurator/tabs/dns_tab.go:392`, `:415-445` | переключатель raw-JSON: `DNSUserRulesToText`/`FromText` + пересбор слотов |
| `ui/configurator/tabs/dns_tab.go:655-667`, `:711-725`, `:775-784`, `:831-860` | CRUD блобов `DNSServers` |
| `ui/configurator/models/preset_ref_sync.go:335` | `stripOutboundAction` — **снимает `outbound`/`action`/`method` из match-карты**; вызов `:307` |
| `ui/configurator/models/preset_ref_sync.go:327` | `copyOrderNum` — **всегда копия значения, не общий указатель** |
| `ui/configurator/models/preset_ref_sync.go:397-490` | `syncDNSServersOnly` — эмиссия `DNSServer` всех трёх видов |
| `ui/configurator/models/preset_ref_sync.go:721-734` | `SyncStateV6ToDNSOverrides` — только `DNSServerKindTemplate` → `map[tag]enabled` |
| `ui/configurator/models/rule_slot.go:47`, `:67`, `:131`, `:172` | `RebuildRuleOrder`, `ReconcileRuleOrder`, `MoveRuleSlot`, `CompactRuleOrderIndices` |
| `ui/configurator/models/dns_rule_slot.go:50`, `:75`, `:141`, `:165` | те же четыре для DNS |
| `core/debugapi/state_endpoints.go:208-214`, `:220` | PATCH правил: раздача `OrderNum` в режиме append + `SortRulesByNum` |
| `core/debugapi/state_endpoints.go:282`, `:290-301` | PUT DNS: `state.DNSOptions` + валидация дискриминаторов |
| `core/debugapi/state_endpoints.go:366-367`, `:397`, `:417-421` | PATCH DNS: чтение/фильтр/запись `DNSRuleKindUser` тел |
| `core/state/sync_dns.go:65`, `:101`, `:130`, `:141`, `:154`, `:190` | `SyncDNSOptionsWithActivePresets` — ветки по kind и `Ref` |
| `core/state/migration_v6_to_v7.go:819-824` | перепись `srv.Body["detour"]` у DNS-серверов при миграции |

---

## 4. Бэкап (`core/backup`)

### 4.1 Типы (`types.go`) — **почти целевая форма**

| Сущность | Адрес | Поля |
|---|---|---|
| `type Backup` | `core/backup/types.go:40-54` | `lx_backup`, `exported_by`, `exported_at`, `subscriptions`, `servers`, `directions`, `chains`, `rules`, `dns`, `vars`, `route`, `warp` |
| `type ExportedBy` | `:58` | кто и чем создал |
| `type SourceRef` | `:71` | |
| `type Subscription` | `:83-146` | `url`, `identity`, `skip`, `tag_policy`, отметки disabled |
| `type SubscriptionIdentity` + кастомный `UnmarshalJSON` | `:148`, `:189` | `UnappliedKeys` `:231`, `IsEmpty` `:247` |
| `type Fold` | `:260` | |
| `type Direction` / `DirectionAuto` | `:273`, `:304` | |
| `type Chain` | `:325` | |
| `type TagPolicy` / `UpdatePolicy` | `:345`, `:352` | |
| **`type Server`** | `core/backup/types.go:358-384` | `id`, `uri`, `config_json`, `label` (legacy-вход), `node_tag`, `folder`, `enabled`, `exclude_from_global`, `sections`, `SourceRef` |
| **`type ServerSections{Raw json.RawMessage}`** | `core/backup/types.go:394-397` | + `MarshalJSON` `:400`, `UnmarshalJSON` `:407` — **блок едет как есть** |
| `RuleKind` + `RuleInline/SRS/Preset/JSON` | `:413`, `:417-420` | у бэкапа есть четвёртый вид `json` |
| **`type Rule`** | `core/backup/types.go:423-443` | `kind`, `name`, `enabled *bool`, **`num *float64`**, `outbound`, `ref`, **`refs []string`**, `vars`, `match json.RawMessage`, `dns`, `resolve` |
| `type DNS{Servers, Rules []DNSRef, Final, Strategy}` | `core/backup/types.go:446-451` | |
| **`type DNSRef`** | `core/backup/types.go:454-462` | `kind`, **`name`** (не `tag`!), `enabled *bool`, `num *float64`, `ref`, `vars`, **`value json.RawMessage`** |
| `type Route{Final}` | `:465` | |
| `boolPtr` / `f64Ptr` | `:471`, `:474` | |

Бэкапный `Rule` уже даёт `num`, `name`, `refs[]`, `vars` на уровне записи. До
целевой формы не хватает: `match` + `outbound` объединить в `body`. `DNSRef`
почти целевой — `value` → `body`, `name` → `tag`.

### 4.2 Экспорт (`export.go`)

| Сигнатура | Адрес | Что делает |
|---|---|---|
| `Export(s *state.State, opts ExportOptions) (*Backup, []Warning, error)` | `core/backup/export.go:52` | корень; проход по `s.Sources` по kind |
| `exportDirections(list []configtypes.Direction) []Direction` | `:199` | |
| `exportSourceRef(src state.Source) SourceRef` | `:216` | |
| `sourceExportName(src)` | `:221` | |
| `exportChain(src, index) Chain` | `:244` | |
| `exportWarp(s) []json.RawMessage` | `:260` | |
| `exportSubscription(src, index) Subscription` | `:285` | **без `nodes[]`-кэша** |
| `exportSourceIdentity(src) *SubscriptionIdentity` | `:323` | |
| `droppedLocalOnlyFields(src) string` | `:357` | local-only поля, которые не едут |
| `exportServer(src state.Source) Server` | `:369` | корневой узел |
| `exportFolder(src state.Source) ([]Server, string)` | `:386` | папка разворачивается в плоские `servers[]` с `folder` |
| `exportServerNode(src state.Node) Server` | `:419` | узел папки |
| **`exportRule(r state.Rule) (Rule, error)`** | `core/backup/export.go:455` | switch по kind |
| — ветка preset | `:472` | `state.PresetBody` → `out.Vars` |
| — ветка inline | `:482-494` | `body.Name`→`Name`, `body.Outbound`→`Outbound`, `json.Marshal(body.Match)`→`Match` |
| — ветка srs | `:496-509` | **`NewSrsBody(...)` для канонизации `:502`**, `urls[0]`→`Ref` `:505`, при ≥2 `urls`→`Refs` `:508` |
| `exportVars(vars) map[string]string` | `:523` | только portable-имена |
| `routeFinal(s) string` | `:540` | |
| `exportDNS(s *state.State) *DNS` | `core/backup/export.go:562` | серверы `:567-569`, правила `:570-572` |
| **`dnsRefFrom(kind, tag, ref string, enabled bool, body map[string]interface{}) DNSRef`** | `core/backup/export.go:579` | `DNSRef{Kind, Name: tag, Ref}` `:580`; **тело только у `kind == "user"`** `:587-591` |

### 4.3 Импорт (`import.go`)

| Сигнатура | Адрес | Заметка |
|---|---|---|
| `(Warning).String()` | `:44` | |
| **`Import(s *state.State, b *Backup, opts ImportOptions) (*ImportResult, error)`** | `core/backup/import.go:214` | |
| **`s.Rules = nil`** | `core/backup/import.go:235` | **единственная секция полной замены (§9 п. 7)**; комментарий `:230-233` |
| `mergeSubscriptions` / `mergeServers` | `:237`, `:241` | |
| `s.Rules = append(s.Rules, rule)` | `:345` | |
| `importDirections(list) []configtypes.Direction` | `:375` | |
| `importSourceRef` / `ensureSourceID` | `:388`, `:395` | |
| `importSubscription(sub, index) (state.Source, []Warning)` | `:412` | |
| `subscriptionLabel` / `importSourceIdentity` / `backupReplaceTag` | `:476`, `:495`, `:528` | |
| `importServer(srv Server) (state.Source, []Warning)` | `:541` | |
| `serverLabel` | `:587` | |
| `importChain(in Chain) (state.Source, []Warning)` | `:603` | |
| **`importRule(r Rule, known, presets tagSet) (state.Rule, []Warning, error)`** | `core/backup/import.go:626` | |
| — ветка preset | `:655-661` | `state.PresetBody{Vars: r.Vars}` → `out.Body` |
| — ветка inline | `:666-673` | `json.Unmarshal(r.Match, &match)`, `state.InlineBody{Name, Match, Outbound}` |
| — ветка srs | `:675-684` | **`urls := r.Refs`, пусто → `[]string{r.Ref}` `:676-679`**, `state.NewSrsBody(...)` `:680` |
| — ветка json | `:685-690` | правило пропускается с `WarnBackupUnknownField` |
| `ruleLabel(r Rule)` | `:698` | |
| **`renumberImportedRules(rules []state.Rule)`** | `core/backup/import.go:714` | размеченные → `UserRuleNumStart + pos` `:727`; неразмеченные в хвост `:733-735` |
| `importedAxisNum(r)` | `:739` | nil → `UserRuleNumEnd + 1` |
| `importVars` / `setVar` | `:746`, `:769` | |
| `newTagSet` / `empty` / `has` | `:782`, `:793`, `:795` | |
| **`importDNS(s *state.State, dns *DNS)`** | `core/backup/import.go:827` | **слияние, не замена** |
| — ключ сервера `kind + \x00 + tag` | `:832-837` | своё сильнее `:840` |
| — `state.DNSServer{Kind, Tag: ref.Name, Ref, Enabled}` | `:843-848` | `Body` из `ref.Value` только при `kind == "user"` `:849-854` |
| — ключ правила `kind + ref + канон(body)` | `:858-861` | `canonicalJSONValue(body)` |
| — `state.DNSRule{...}` | `:875-880` | |
| `importWarp(s, warp)` | `:903` | добавление, не замена |

### 4.4 Слияние (`merge.go`)

| Сигнатура | Адрес | Заметка |
|---|---|---|
| `mergeSubscriptions(s, subs, warns, cnt)` | `core/backup/merge.go:60` | ключ — URL |
| `applySubscriptionSettings(dst, src)` | `:115` | |
| `mergeDisabledMarks(dst, incoming)` | `:154` | по тегам |
| **`mergeServers(s, list, warns, cnt)`** | `core/backup/merge.go:199` | папки собираются ПО ИМЕНИ поля `folder` |
| `applyImportedSections(node, sec)` | `core/backup/merge.go:297` | **«файл замещает секции узла»** |
| `folderNodeWithBody(folder, node) int` | `:312` | |
| **`nodeBodyKey(n *state.Node) string`** | `core/backup/merge.go:354` | **дедуп узлов по ТЕЛУ, не по тегу** |
| `canonicalJSONKey(raw) string` | `:403` | |
| `canonicalJSONValue(v) interface{}` | `:430` | **рекурсивная канонизация — от неё зависят ключи дедупа и DNS-правил** |
| `takenRootTags` / `takenSourceIDs` / `freshIDIfTaken` / `uniqueTag` | `:461`, `:486`, `:497`, `:507` | |

### 4.5 Файл (`file.go`) и списки известных ключей

| Сигнатура | Адрес | Заметка |
|---|---|---|
| `WriteFile(path, b)` | `core/backup/file.go:33` | |
| `ReadFile(path)` | `:55` | |
| `Parse(data) (*Backup, []Warning, error)` | `core/backup/file.go:83` | |
| `decodeTolerant(data)` | `:112` | битое поле вырезается, импорт продолжается |
| `stripField` / `stripPath` / `joinPath` | `:149`, `:168`, `:230` | |
| `mergeKeys(base, extra)` | `:334` | |
| **`scanUnknown(data) []Warning`** | `core/backup/file.go:360` | |
| `scanDirectionBody` / `note` / `object` / `nested` / `nested2` / `array` | `:404`, `:417`, `:446`, `:454`, `:469`, `:487` | |
| `entryLabel` / `warnings` | `:505`, `:515` | |
| `SuggestFileName(stamp)` | `:531` | |
| `directionKeys` | `core/backup/file.go:288-296` | |
| **`ruleKeys`** | `core/backup/file.go:299-303` | `kind, name, enabled, num, outbound, ref, refs, vars, match, dns, resolve` — **в 1.0 сюда `body`, минус `match`/`outbound`** |
| `directionAutoKeys` | `:304-308` | |
| `chainBodyKeys` | `:312-315` | |
| `warpKeys` | `:320-328` | |
| `dnsKeys` | `:329` | `servers, rules, final, strategy` |
| **`dnsRefKeys`** | `core/backup/file.go:330-333` | **уже принимает и `tag`, и `name`, и `num`** |

### 4.6 Прочее в пакете

| Сигнатура | Адрес | Заметка |
|---|---|---|
| **`decodeBackupSections(sec *ServerSections, nodeTag string) *state.NodeSections`** | `core/backup/node_sections.go:22` | `json.Unmarshal(sec.Raw, &state.NodeSections{})` `:27` — **разбирает форму СОСТОЯНИЯ** |
| `exportFold` / `exportDisabledMap` / `exportNodeLinkRef` / `exportHops` / `exportChainSpec` | `core/backup/convert_v7.go:35`, `:62`, `:91`, `:110`, `:126` | |
| `importFold` / `importNodeLinkRef` / `importHops` / `importChainBody` / `importMaskTag` | `:136`, `:159`, `:178`, `:195`, `:218` | |
| `replaceTagSurvivesExport` / `legacyFoldPrefix` / `foldDerivedDirectionTags` / `resolveImportedHops` | `:243`, `:263`, `:278`, `:314` | |
| `IsPortableVar(name) bool` | `core/backup/portable_vars.go:37` | реестр `registry/vars.json` |
| `exportDirection` / `importDirection` / `templateIntToBackup` | `core/backup/directions.go:24`, `:73`, `:116` | |

### 4.7 Корпус-раннер

| Сущность | Адрес | Заметка |
|---|---|---|
| `backupCorpusRelPath = "../../contract/corpus/backup"` | `core/backup/corpus_test.go:24` | |
| `type corpusExpectation` | `core/backup/corpus_test.go:26-60+` | `rules[]{name,enabled,refs}`, `vars`, `warnings`, `route_final_applied`, `extensions_dropped`, `disabled_hashes`, `replace_tags`, `folders` |
| `Refs []string \`json:"refs"\`` в ожидании | `:33` | отсутствие ключа = «не проверяем» |
| `TestBackupCorpus(t)` | `core/backup/corpus_test.go:134` | |
| открытие кейсов: `.backup.json`, кроме `.pre.backup.json` | `:145-149` | |
| чтение `<case>.expected.json` | `:163` | |
| предсостояние из `<case>.pre.backup.json` | `:177`, `:224` | импортируется в ПУСТОЕ состояние первым |
| сверка кодов: `warnCodes(parseWarns + res.Warnings)` | `:198`, `:543` | **дедуп кодов**, порядок сохранён |
| `DecodeBody` → `URLs()` в хелпере | `:361-365` | проверка `refs` ожидания |
| `InlineBody` / `SrsBody` в хелперах | `:528`, `:533` | |

Кейсов в `contract/corpus/backup/` — 43 файла (20 пар `*.backup.json`/`*.expected.json`
плюс `.pre.` и `.expected.lxbox.json`). Существенные: `srs_multi_refs`,
`order_renumbered_preserving_sequence`, `folders_roundtrip`, `chains_roundtrip`,
`merge_sources_by_url`, `disabled_nodes_by_hash`, `replace_tag_index`,
`legacy_0_10_launcher_extensions`, `extensions_dropped`, `broken_outbound_ref`,
`directions_created_on_import`, `non_portable_var_skipped`,
`unknown_keys_warned_import_continues`, `boolean_skip_type_mismatch`,
`chain_tag_duplicate`, `chain_disabled_enabled_default`, `final_target_missing`,
`directions_ping_roundtrip`. **Кейса на `sections` нет ни одного.**

---

## 5. Контракт

| Сущность | Адрес | Заметка |
|---|---|---|
| `contract/VERSION` | содержимое `0.12.10` | **→ `1.0.0` только после LxBox (SPEC §5)** |
| схема, заголовок | `contract/schema/backup.schema.json:4` | «LX Backup v1 (контракт 0.12.0)» |
| верхние ключи | там же, `properties` | `lx_backup, exported_by, exported_at, subscriptions, servers, rules, dns, vars, route, warp, directions, chains` |
| `$defs` | | `dnsRef`, `direction`, `directionAuto` |
| **`properties.rules.items`** | `contract/schema/backup.schema.json:276-330+` | `kind` (enum `inline\|srs\|preset\|json`), `name` `:293`, `enabled` `:297`, `num` `:300`, `outbound` `:304`, `ref` `:308`, `refs` `:312`, `vars`, `match`, `dns`, `resolve`; `additionalProperties: true` `:284` |
| **`servers[].sections`** | `contract/schema/backup.schema.json:239-273` | `sections.rules[]` = `$ref: #/properties/rules/items` `:245-247`; `sections.dns.servers[]`/`rules[]` = `$ref: #/$defs/dnsRef` `:262-274`; `additionalProperties: false` `:241` |
| **`$defs/dnsRef`** | `contract/schema/backup.schema.json:479-500` | `kind` (enum `template\|preset\|user`), **`tag`** `:492`, `ref`, `enabled`; `required: [kind]`; `additionalProperties: true` |
| `$defs/direction` | `:506` | `additionalProperties` намеренно открыт |
| `contract/docs/BACKUP.md` | §9 слияние — норма, не меняется по смыслу | таблицы полей §2 |
| `contract/docs/NODE_SECTIONS.md` | §1 → форма из ONE_NAMESPACE §2 | снять «черновик» |
| `contract/docs/CANON.md` | термин `entry` | открытый вопрос SPEC §6 |
| `contract/schema/node.schema.json` | `entry` — тело узла разбора | переименование в `body` трогает весь корпус |
| `contract/registry/backup_warnings.json` | коды предупреждений | + `backup_section_record_dropped` |
| `contract/corpus/README.md` | правила раннера, `meta.extension`, `-update` | |
| `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md` | последняя строка `:116` = **D-108** | решения — строки таблицы, не заголовки (`rg '^#* *D-'` пусто); норма 1.0 = **D-109** (так и записано в SPEC §5 и ONE_NAMESPACE §4) |
| `contract/TASKS_LXBOX.md` | `## 10` `:475`, `## 11` `:511`, `## 12` `:539`, `## 13` `:566`, `## 14` `:681`, **`## 15` `:782`** | **и `## 14`, и `## 15` ЗАНЯТЫ** (REALITY/naive; VLESS Encryption); контракт 1.0 → **`## 16`** |
| `contract/registry/backup_warnings.json` — коды | `:5`…`:113`, 19 штук | формат записи `:5-10`: `severity`, `params[]`, `side`, `desc`; **`backup_section_record_dropped` объявлен в D-101/D-102, но в файле ЕГО НЕТ** |
| `contract/corpus/README.md` — `meta.extension` | `:66-70` | сторона без схемы кейс **пропускает**; метка переживает `-update` |
| `contract/corpus/README.md` — `-update` | `:106-112` | «осознанный PR, не рутина»; diff ожиданий = изменение контракта + бамп VERSION |
| `contract/corpus/README.md` — `.pre.backup.json` | `:72-86` | раннер обязан отфильтровать из списка кейсов |
| `contract/docs/BACKUP.md` — таблица `rules[]`+`dns` | `:192-207` | одна общая таблица; строки `rules.*` `:194-198`, `dns.*` `:199` |
| `contract/docs/BACKUP.md` — §9 слияние | `:336`, норма `:349-474` | **по смыслу не меняется** |
| `contract/docs/CANON.md` — `entry` | заголовок `:17`, правила `:19-33` | без `tag` `:19`, без `detour` `:21`, ключи сортируются `:22`, сравнение по значению `:64` |
| `contract/schema/node.schema.json` — `entry` | `:43`, в `required` `:37` | `$defs.node` с `additionalProperties: false` `:38` |

---

## 6. Тесты и эталоны

| Файл / фикстура | Адрес | Что придётся переписать |
|---|---|---|
| `TestGoldenScenarios` | `core/build/golden_test.go:60` | root `testdata/golden` `:61`; сценарий полон при 4 файлах `:91`; сравнение байт-в-байт `:196` |
| сценарии | `core/build/testdata/golden/real-v088/` | `template.json` + `state.json` + `cache.json` + `expected.config.json` — **`state.json` придётся переписать в v8** |
| `TestEtalonV6MigOutboundSnapshot` | `core/etalon_v6mig_capture_test.go:37` | по умолчанию skip `:40`; `ETALON_V6MIG=1` проверить, `=capture` перезаписать; эталон `SPECS/118-F-N-STATE_V7/etalon/v6mig/outbounds.snapshot.json` `:96` |
| эталон конфига | `SPECS/118-F-N-STATE_V7/etalon/real-v088.config.json` | инвариант «байт-в-байт» |
| фикстуры состояния | `core/state/testdata/` | `v7_roundtrip.json` (**вход теста миграции v8**), `v6_roundtrip.json`, `real_v088_v4.json`, `v4_minimal.json`, `v3_legacy_rules.json` + `.v6.bak`/`.pre-058.bak` |
| `core/state/canonical_roundtrip_test.go` | | Load→Save→Load→Save байт-в-байт |
| `core/state/rule_types_test.go` | `:29-223` | **весь файл про `DecodeBody`** — переписывается целиком |
| `core/state/rule_order_test.go`, `rule_order_invariant_test.go` | | ось; `OrderNum` в конструкторах |
| `core/state/rule_identity_test.go` | `:10`, `:15` | конструкторы `InlineBody`/`SrsBody` |
| `core/state/migration_scenarios_test.go` | `:426-430`, `:496-497`, `:795-799` | `DecodeBody` → `*InlineBody` |
| `core/state/node_sections_test.go`, `sync_dns_test.go`, `state_test.go`, `v6_integration_test.go`, `legacy_migration_test.go`, `load_legacy_save_test.go`, `connections_test.go` | | косвенно |
| `core/build/spec106_rule_order_emit_test.go` | `:43` | `InlineBody` |
| `core/build/srs_filename_test.go` | `:25`, `:104` | `SrsBody{Name, SrsURL, Outbound}` |
| `core/build/node_sections_build_test.go` | `:170`, `:213` | `InlineBody` |
| `core/build/resolve_route_test.go`, `resolve_dns_test.go`, `preset_merge_test.go`, `rules_pipeline_test.go`, `dns_merge_test.go`, `contract_dns_test.go` | | сборка правил/DNS |
| `core/backup/corpus_test.go` | см. §4.7 | **все 0.12-кейсы становятся кейсами legacy-чтения** |
| `core/backup/backup_test.go` | `:94`, `:99`, `:322`, `:364` | конструкторы тел |
| `core/backup/rule_order_invariant_test.go` | `:30`, `:70` | `InlineBody` |
| `core/backup/node_sections_roundtrip_test.go` | `:29-60`, `:135` | **строит секции в форме СОСТОЯНИЯ** и ждёт их в файле |
| `core/backup/schema_test.go` | | сверка с `backup.schema.json` |
| `core/backup/merge_test.go`, `file_test.go`, `convert_v7_test.go`, `directions_test.go`, `identity_test.go`, `import_ulid_test.go`, `purity_test.go`, `export_folder_loss_test.go`, `wgini_roundtrip_test.go` | | |
| `ui/configurator/models/preset_ref_sync_test.go` | `:49-53`, `:80-81` | `DecodeBody` → `*InlineBody`/`*SrsBody` |
| `ui/configurator/models/rule_order_axis_test.go` | `:33`, `:42`, `:79-89` | **`:88-89` собирает `InlineBody` вручную в обход `DecodeBody`** |
| `ui/configurator/models/rule_order_invariant_test.go` | `:47` | `InlineBody` |
| `ui/configurator/models/rule_slot_move_test.go`, `rule_slot_system_test.go`, `rule_state_utils_test.go`, `dns_rule_slot_test.go`, `dns_state_test.go` | | слоты и ось |
| `ui/configurator/business/srs_rule_reopen_test.go` | `:212-216` | `DecodeBody` → `URLs()` |
| `core/config/node_document_test.go` | `:65-69`, `:97-101` | секции узла |
| `core/config/tailscale_test.go` | `:84-88` | |
| `core/config/subscription/singbox_sections_extract_test.go` | `:82-86` | |

Политика тестов проекта (память): россыпи юнитов не писать, только критичные
интеграционные в конце куска, полный прогон один раз.

---

## 7. Ловушки

**1. `parseWithContext` ловит `meta >= 7`.** `core/state/load_router.go:107`.
v8-файл прочитается как v7 и на первом же Save потеряет всё новое — **молча**.
Гейт `schema_gate.go` этого не поймает: он про перенос между машинами, а не про
Load. Ветка обязана разойтись: `== 7` → миграция, `>= 8` → `parseV8`.

**2. Четыре кастомных Marshal/Unmarshal DNS.** `core/state/dns_options.go:141`,
`:177`, `:208`, `:228`. Они и есть вся плоская форма. Побочные эффекты, которые
легко потерять: (а) `Unmarshal` кладёт `tag` **и в `Body` тоже** (`:196-202`
исключает только `kind`/`ref`/`enabled`); (б) `Marshal` при непустом поле `Tag`
игнорирует `body["tag"]` (`:161-167`); (в) у `template`/`preset` `Body` остаётся
nil, и это проверяется сборкой. В v8 `tag` внутри `body` быть не должно — но
`resolve_dns.go:303-308` дописывает его в тело при эмиссии, и **эту дописку надо
оставить**, иначе конфиг изменится.

**3. `Rule.Body` — `json.RawMessage`, а не map.** `core/state/rule_types.go:72`.
Порядок ключей тела в файле сохраняется побайтно. `SubstituteSelf` работает по
сырому JSON именно ради этого (`core/state/selfvar.go:51`, комментарий в
`parsed_cache.go:68-70`). Если в v8 `body` станет `map[string]interface{}`,
ключи пересортируются по алфавиту и **golden станет красным на ровном месте**.
DNS-тела при этом **уже** map (`dns_options.go:95`, `:110`) — асимметрия намеренная.

**4. `StableRuleID` считается от тела.** `core/state/rule_identity.go:29-49`,
вызов `DecodeBody` на `:33`. От него зависят: теги `rule_set` в конфиге
(`SrsRuleSetTag`, `:58`), ключ srs-кэша (`core/build/preset_merge.go:739`), теги
`user:<id>` в конвейере правил, orphan-GC файлов `.srs`
(`core/config_service.go:493-509`). Переезд `name` из тела наружу **обязан**
сохранить строку символ в символ: `sanitizeIDPart` (`:74`) выбрасывает кириллицу,
так что правило с русским именем даёт `"rule"` или `"unnamed"` — и **два правила
могут сегодня делить один id**. Не «чинить» это заодно: имена файлов кэша и теги
в конфиге изменятся.

**5. `num` уже занят — в двух смыслах сразу.** В бэкапе `Rule.Num *float64`
(`core/backup/types.go:432`) и `DNSRef.Num *float64` (`:458`) — **float, не int**;
в состоянии `OrderNum *int`. В шаблоне `presets[].num` — стартовый номер
(`RuleOrderSpec.Num`, `core/state/rule_order.go:63`). Три поля с одним именем и
разными типами; при переезде `order_num` → `num` в состоянии надо решить, что
именно хранится (int) и где происходит конверсия float↔int (сейчас — в
`export.go`/`import.go`).

**6. `ServerSections.Raw` — блок формы СОСТОЯНИЯ, а схема обещает форму БЭКАПА.**
`core/backup/types.go:394-397` + `core/backup/node_sections.go:22-32`: блок просто
`json.Unmarshal`-ится в `state.NodeSections`, то есть внутри `order_num` и плоские
DNS-тела. А `contract/schema/backup.schema.json:239-273` (D-102) объявляет
`sections.rules[]` = `#/properties/rules/items` (с `num`) и `sections.dns.*[]` =
`$defs/dnsRef` (тело в `value`). **Код и схема сегодня расходятся**, и это никем
не ловится: кейса корпуса на `sections` нет, а `node_sections_roundtrip_test.go`
круговой — пишет и читает форму состояния. Переезд состояния на v8 **сам собой**
приведёт `Raw` к схеме — но только если целевые имена совпадут (`num`, `body` vs
схемное `value`). Решить явно, а не наблюдать как побочный эффект.

**7. `DNSRef.Name` в коде против `dnsRef.tag` в схеме.** `core/backup/types.go:457`
пишет `json:"name"`, `export.go:580` кладёт туда тег, `import.go:845` читает
обратно; схема на `contract/schema/backup.schema.json:492` объявляет `tag`.
Спасает только `dnsRefKeys` (`core/backup/file.go:330-333`), где **разрешены оба**,
поэтому `scanUnknown` молчит. Реальные файлы 0.12 несут `name` — legacy-вход обязан
читать именно `name`, а не `tag`.

**8. `.(map[string]interface{})` и типы из JSON.** Тела DNS приходят как
`map[string]interface{}` и разливаются по конфигу (`resolve_dns.go:303-306`,
`:339-342`). Числа из JSON — `float64`, массивы — `[]interface{}`. Это уже ломало
эмиссию узлов (память: «JSON-карта и .(int)-ассерты»). При переезде правил на
`body`-объект та же грабля появится и у route-правил, где сегодня `InlineBody.Match`
тоже `map[string]interface{}` (`rule_types.go:89`) — **но он идёт в конфиг
как есть, без ассертов**, поэтому сейчас цел. Новых ассертов по типам не заводить.

**9. Порядок ключей файла = порядок полей структуры.** `diskStateV7`
(`core/state/disk_v7.go:46-54`) и комментарий `:15-16`; `canonical_roundtrip_test.go`
проверяет байт-в-байт. Плюс `SetEscapeHTML(false)` (`save.go:166`) и обрезка
хвостового `\n` (`save.go:173`). `diskStateV8` придётся объявлять с тем же порядком
полей, иначе roundtrip-тест покраснеет, ничего по смыслу не изменив.

**10. `s.Rules = nil` на импорте.** `core/backup/import.go:235` — правила
**замещаются целиком** (§9 п. 7), в отличие от источников/DNS/warp, которые
сливаются. Семантику §9 SPEC запрещает менять; при переписывании импорта на
«чтение прямо в типы состояния» эта асимметрия должна пережить рефакторинг.

**11. Дедуп узлов на импорте — по ТЕЛУ.** `nodeBodyKey`
(`core/backup/merge.go:354`) + `canonicalJSONKey`/`canonicalJSONValue` (`:403`,
`:430`). То же для DNS-правил (`import.go:858-861`). Любое изменение канонизации
меняет, что считается «тем же узлом», — а это видно только на корпусе
`merge_sources_by_url` и `folders_roundtrip`.

**12. Срезы `[:0]` в `dropForeignKinds`.** `core/state/node_sections.go:202`,
`:218`, `:226` — фильтрация на месте поверх исходного массива. Работает, но
**мутирует входной слайс**; при копировании кода в v8-нормализатор легко получить
порчу чужих данных, если вход шарится.

**13. Два конструктора тел правил в миграциях.** `migration_v5_to_v6.go:31`,
`:47`, `:71` строят `PresetBody`/`SrsBody`/`InlineBody` напрямую, минуя
`NewSrsBody`-канон. Цепочка v5→v6→v7→v8 остаётся рабочей (SPEC §3), значит эти
конструкторы выдают **вход** миграции v7→v8 и трогать их не надо — но и
рассчитывать, что они уже дают канон, нельзя (`export.go:501-502` для того и
канонизирует повторно).

**14. `applyRenames` пересобирает тело через `json.Marshal(body)`.**
`core/state/migration_v6_to_v7.go:771`. То есть уже сегодня миграция v6→v7
**переписывает порядок ключей `match`** у правил, которым меняли `Outbound`.
Значит на этом пути байт-в-байт не держится, и эталон v6mig сравнивает не файл,
а снимок outbounds (`core/etalon_v6mig_capture_test.go:96`). Не перепутать два
инварианта: golden — про `config.json`, эталон v6mig — про snapshot.

**15. `## 14` И `## 15` в TASKS_LXBOX заняты.** SPEC §5 планирует контракт 1.0 в
`## 14`, но `contract/TASKS_LXBOX.md:681` — это REALITY/naive, а `:782` (`## 15`) —
VLESS Encryption. Свободен **`## 16`**. Нумерация решений на 14.09.2026 доведена
до **D-108** (`DECISIONS.md:116`); норма контракта 1.0 — **D-109**, как записано в
SPEC §5 и `ONE_NAMESPACE.md:151`. Оба файла живые и правятся параллельно другими
сессиями: **перед записью перепроверить последний номер**, а не верить карте.

**16. Окно совместимости бэкапа (SPEC §4, ред. 14.09.2026).** Экспорт держит
**двух писателей**: 1.0 и переходный 0.12 (`legacy_write_012.go`), дефолт
переключается константой `BackupExportFormatDefault`. Значит `exportRule`/
`dnsRefFrom`/`exportServer` **нельзя удалять** волной 2 — они становятся телом
legacy-писателя. Планировать их как отдельный файл, а не как удаление.

**17. `preset_merge.go` собирает синтетический `state.State` трижды.**
`:330`, `:412`, `:608` — `&state.State{Rules: ctx.rulesWithNodeSections(), DNS:
ctx.dnsWithNodeSections()}`. Три места надо менять согласованно; расхождение
между ними даст разный набор `rule_set` в `route` и в `dns`, а это ровно тот
класс багов, ради которого написан `CollectEmittedRouteRuleSetTags` (`:571`).

**18. Правило srs — «всё или ничего».** `core/build/resolve_route.go:300-312`:
если закэшированы не все наборы правила, правило **пропускается целиком**, а в
`RuleSets` кладётся одна запись со `Skipped: true` и тегом `SrsRuleSetTag(id, 0)`.
Переезд на `refs[]` обязан сохранить эту семантику — частичный набор молча менял
бы смысл правила.

**20. `EncodeBody` не существует — 12+ разрозненных писателей тела.** Читатель
один (`DecodeBody`, `core/state/rule_types.go:186`), а пишут тело вручную
`json.Marshal(state.XBody{...})`: `ui/configurator/models/preset_ref_sync.go:261`,
`:296`, `:311`, `:674`; `core/state/node_sections_convert.go:144`;
`core/state/migration_v5_to_v6.go:31`, `:47`, `:71`; `core/backup/import.go:655`,
`:668`, `:680`; `core/state/rule_order.go:182` (литерал `{"vars":{}}`). Асимметрия
уже дала два бага: (а) `nodeSectionRuleToSingbox`
(`core/state/node_sections_convert.go:256-262`) пишет **только `SrsURL`** — srs-правило
узла с несколькими наборами молча эмитит один (тот же класс, что репорт 1.5.5 в
комментарии `rule_types.go:104`); (б) `core/backup/export.go:495-502` разбирает
`r.Body` в `SrsBody` **в обход `DecodeBody`** и канонизирует повторно вручную —
вторая нормализация, обязанная оставаться в согласии с `SrsBody.normalize()`.
**Волна 1 обязана завести парный `EncodeBody`/конструктор**, иначе переезд формы
придётся делать в 12 местах руками и один из них снова забудут.

**21. Мёртвые поля и мёртвые функции в бэкапе — не принимать за живые.**
`Rule.DNS` / `Rule.Resolve` (`core/backup/types.go:441-442`) и `DNSRef.Num` /
`DNSRef.Vars` (`:458`, `:460`) **объявлены, но никогда не заполняются** — round-trip
их молча теряет; объявлены они только чтобы `scanUnknown` не ругался.
`exportDirections` (`core/backup/export.go:199`) и `importDirections`
(`core/backup/import.go:375`) **не вызываются**: `Export` разворачивает цикл на
`export.go:77-82`, `Import` — на `import.go:275-288`. Не переписывать их как
«часть контракта» и не считать покрытием.

**22. `mergeSubscriptions` матчит URL байт-в-байт.** `core/backup/merge.go:65-77`
строит индекс, `:86` ищет — **без trim и без приведения регистра**. `mergeServers`
матчит по телу (ловушка 11), папки — **по имени байт-в-байт**
(`core/backup/merge.go:220-229`, имя из `srv.Folder` `:239`). Три разных ключа
идентичности в одном проходе импорта; §9 их фиксирует, менять нельзя.

**23. Корпус сверяется не одним DeepEqual.** `core/backup/corpus_test.go:203-214` —
11 отдельных проверок (`checkRules` `:319`, `checkVars` `:407`, `checkChains` `:574`,
`checkFolders` `:369`, `checkSubscriptions` `:239`, …). Единственный
`reflect.DeepEqual` — внутри `jsonDeepEqual` `:644` для цепочек. **Флага `-update`
в `core/backup` НЕТ** — ожидания правятся руками; regenerate есть только в правилах
`contract/corpus/README.md:106-112` для сторон, которые его реализовали. Значит
новые кейсы 1.0 придётся писать руками, и `checkRules` (`:319`) надо будет учить
новой форме.

**24. `contract/schema/backup.schema.json` почти везде открыт.**
`additionalProperties: false` стоит ровно в трёх местах: `:241` (`servers[].sections`),
`:253` (`sections.dns`), `:578` (`$defs.directionAuto`). Везде ещё — `true`
намеренно (root `:5`): отвергать чужой ключ обязан импортёр с warning, не
валидатор. Значит схема **не поймает** форму 1.0, приехавшую в поле 0.12, — это
работа `scanUnknown` (ловушка 19). И наоборот: `sections` со своим `false` —
единственное место, где схема упадёт на новой форме первой.

**25. `SPECS/118-F-N-STATE_V7/etalon/v6mig/outbounds.actual.json` — артефакт
падения.** Пишется строками `core/etalon_v6mig_capture_test.go:110-111` при
расхождении и лежит в дереве рядом с `outbounds.snapshot.json`. Не перепутать,
какой из двух эталонный: сверяется **`.snapshot.json`** (`:96`).

**26. `ruleKeys` в `scanUnknown` — единственный рубеж «непонятого поля».**
`core/backup/file.go:299-303`. Пока там `match`/`outbound` и нет `body`, файл 1.0
со своим `body` даст `backup_unknown_field` на каждом правиле. Списки для 1.0 и
для 0.x обязаны разойтись (SPEC §4), иначе legacy-кейсы корпуса покраснеют
предупреждениями, которых в их `expected` нет.

---

## 8. `core/state` ПОСЛЕ этапа S1 (state v8)

Адреса проверены на ветке `spec127/state-v8` после этапа S1. Гейт этапа
(`gofmt -l core/state && go build/vet/test ./core/state/...`) зелёный.

### 8.1 Запись правила и её вид (`rule_types.go` — переписан целиком)

| Сущность | Адрес | Заметка |
|---|---|---|
| `type Rule{Kind,ID,Ref,Name,Enabled,Num,Refs,Vars,Body}` | `core/state/rule_types.go:68-118` | порядок полей = порядок ключей файла |
| `Rule.Num *int \`json:"num,omitempty"\`` | `:92` | бывший `order_num` |
| `Rule.Refs []string \`json:"refs,omitempty"\`` | `:105` | бывшие `body.srs_url` + `body.srs_urls` |
| `Rule.Vars map[string]string` | `:109` | бывший `body.vars` |
| `Rule.Body json.RawMessage \`json:"body,omitempty"\`` | `:118` | правило sing-box КАК ЕСТЬ; у preset нет |
| `type PresetBody{Vars}` / `InlineBody{Name,Match,Outbound}` / `SrsBody{Name,Refs,Outbound}` | `:121`, `:126`, `:143` | **виды**, не хранимые формы |
| `(*SrsBody).URLs()` | `core/state/rule_types.go:150` | алиас `Refs` (читатели до v8) |
| **`NewPresetRule(ref, vars) Rule`** | `core/state/rule_types.go:160` | |
| **`NewInlineRule(name, match, outbound) Rule`** | `core/state/rule_types.go:177` | body = match ∪ цель через `ApplyOutboundToRule` |
| **`NewSrsRule(name, refs, outbound) Rule`** | `core/state/rule_types.go:196` | дедуп `Refs`, body = только цель |
| **`(*Rule).SetOutbound(outbound) error`** | `core/state/rule_types.go:211` | перепись цели ПО СЫРОМУ JSON: ключи матчеров и их порядок целы |
| `decodeObjectOrdered(raw)` | `core/state/rule_types.go:278` | ключи объекта в порядке появления + сырые значения |
| `dedupNonEmpty` | `:314` | |
| `outboundFromBody(body)` | `core/state/rule_types.go:336` | `action=reject`→`reject`, `+method=drop`→`drop`, иначе `body.outbound` |
| **`(*Rule).DecodeBody()`** | `core/state/rule_types.go:383` (этап FIX) | единственный читатель; валидация: preset→`Ref`, inline→`Name`, srs→`Name`+≥1 `Refs` |
| `decodeRuleBodyMap` | `:420` | |

`NewSrsBody`/`SrsBody.normalize`/`SrsBody.SrsURL`/`SrsURLs` **удалены** —
канонизация живёт в `NewSrsRule`.

### 8.2 DNS (`dns_options.go`) — обычные struct-теги

| Сущность | Адрес | Заметка |
|---|---|---|
| `type DNSServer{Kind,Tag,Ref,Enabled,Body}` | `core/state/dns_options.go:76-97` | все с json-тегами; `body` — только `user` |
| `type DNSRule{Kind,ID,Ref,Name,Enabled,Body}` | `core/state/dns_options.go:99-121` | `id`/`name` — необязательные метаданные, провозятся |
| `FindServerByTag/ByRef/FindRuleByRef/IsEmpty` | `:148`, `:161`, `:174`, `:188` | семантика не менялась |

Четыре кастомных `MarshalJSON`/`UnmarshalJSON` **удалены** (ловушка 2 закрыта).
`tag` внутри `body` в v8 не хранится; дописку тега при эмиссии
(`resolve_dns.go:303-308`) **оставить**.

### 8.3 Диск и версии

| Сущность | Адрес | Заметка |
|---|---|---|
| `SchemaVersionV8 = 8` | `core/state/disk_v8.go:32` | |
| `SchemaNameV8 = "sources_v8"` | `core/state/disk_v8.go:36` | |
| `type diskStateV8{Meta,Sources,Directions,Rules,Vars,DNS,Warp}` | `core/state/disk_v8.go:40-48` | ключи `dns` и `warp` (бывшие `dns_options`/`warp_accounts`) |
| `parseV8(data)` | `core/state/disk_v8.go:55` | |
| `SchemaVersion = SchemaVersionV8` | `core/state/state.go:43` | |
| `SchemaMajor = SchemaVersionV8` | `core/state/schema_gate.go:28` | |
| `s.Version = SchemaVersionV8` | `core/state/save.go:41` | |
| **`(*State).MarshalV8()`** | `core/state/save.go:103` | бывшая `MarshalV7` — вызывающие переименовать (`core/debugapi/state_endpoints.go:137`) |
| `marshalDisk()` → `diskStateV8` | `core/state/save.go:125-148` | |
| `disk_v7.go` | — | остались только `SchemaVersionV7`/`SchemaNameV7`/`migrationPurgesLegacy`; **`parseV7` и `diskStateV7` удалены** |

### 8.4 Маршрутизация версий и бэкап-копии (`load_router.go`)

| Сущность | Адрес | Заметка |
|---|---|---|
| `parseWithContext` — switch | `core/state/load_router.go:121-148` | `meta >= 8` → `parseV8`; `meta == 7` → `migrateV7DocToV8` + `parseV8` + `MigrationReport{FromVersion:7}`; 6/5/2–4 как были |
| копия `.v7.bak` перед миграцией | `core/state/load_router.go:54-61` | O_EXCL, один раз |
| `legacyBackupSuffix` / `v7BackupSuffix` | `core/state/load_router.go:215-218` | `.v6.bak` / `.v7.bak` |
| `legacyBackupPath(path)` | `core/state/load_router.go:222` | сначала `.v7.bak`, затем `.v6.bak` |
| `writeLegacyBackupOnce(path, data, suffix)` | `core/state/load_router.go:235` | **сигнатура изменилась** — добавлен суффикс |
| персист мигрированного + purge | `core/state/load_router.go:88-106` | Save на ЛЮБУЮ миграцию; `purgeLegacyAfterMigration` — только при `FromVersion <= 6` |
| лог миграции без жёсткого «v7» | `core/state/migration_report.go:64` | `v%d→v%d` с `SchemaVersion` |

### 8.5 Миграция v7→v8 по сырому документу (`migration_v7_to_v8.go`, новый файл)

| Сущность | Адрес | Заметка |
|---|---|---|
| `type v7Rule / v7InlineBody / v7SrsBody / v7PresetBody` | `:33`, `:44`, `:52`, `:60` | локальные зеркала; `v7InlineBody.Match` — `json.RawMessage` |
| **`migrateV7DocToV8(doc, rep) ([]byte, error)`** | `core/state/migration_v7_to_v8.go:71` | верх — `map[string]json.RawMessage`; незнакомое проносится сырым |
| `migrateV8Meta` | `:119` | `version`/`schema` |
| `migrateV8Rules` / `migrateV8Rule` | `:136`, `:159` | неизвестный `kind` — запись сырьём + предупреждение |
| **`joinRuleBodyWithTarget(match, outbound)`** | `core/state/migration_v7_to_v8.go:271` (этап FIX) | склейка: матчеры теми же байтами и в том же порядке, цель в конец через `ApplyOutboundToRule` |
| `migrateV8DNS` / `Servers` / `Rules` | `:268`, `:294`, `:318` | плоское тело → `body` |
| `flatDNSBody(flat, tag, where, rep)` | `core/state/migration_v7_to_v8.go:419` (этап FIX) | `tag` из тела выброшен; расхождение с полем — в отчёт |
| `migrateV8Sources` / `migrateV8NodeSections` | `:378`, `:417` | секции корневых узлов и `nodes[]` папок — ТЕМИ ЖЕ функциями |
| **`rulesFromLegacyShape` / `dnsFromLegacyShape`** | `:459`, `:475` | вход цепочки v5/v6: у них та же старая форма записей |

### 8.6 Цепочка v5→v6→v7→v8

| Адрес | Что изменилось |
|---|---|
| `core/state/load_v6.go:40-75` | `rules`/`dns_options` читаются как `json.RawMessage` и прогоняются через `rulesFromLegacyShape`/`dnsFromLegacyShape` — **прямой unmarshal в типы v8 терял бы `name`/`refs`/`order_num`** |
| `core/state/load_v6.go:170-178` | `legacyDevDNSToOptions`: `tag` больше не кладётся в `Body` |
| `core/state/migration_v5_to_v6.go:30-79` | три конструктора тел → `NewPresetRule`/`NewSrsRule`/`NewInlineRule` |
| `core/state/migration_v5_to_v6.go:140-150` | `migrateDNS`: `tag` не кладётся в `Body` |
| `core/state/migration_v6_to_v7.go:740-770` | `applyRenames`: цели правил — через `(*Rule).SetOutbound`, переменные пресета — в `r.Vars`; **обратного `json.Marshal(body)` больше нет** |
| `core/state/migration_v6_to_v7.go:811-820` | detour DNS-серверов — без изменений (`Body` всё ещё карта) |
| `core/state/load_v6.go:203-280` | `legacyCustomRulesFromV6` — без правок: читает вид |

### 8.7 Секции узла

| Адрес | Что изменилось |
|---|---|
| `core/state/node_sections.go:148-171` | `cloneRule` копирует `Num`, `Refs`, `Vars` |
| `core/state/node_sections_convert.go:129-165` | `nodeSectionRuleFromBody`: `body` = правило sing-box как есть (`name` снимается в поле записи, цели нет → `"outbound":"@self"`) |
| `core/state/node_sections_convert.go:247-291` | `nodeSectionRuleToSingbox`: inline — `r.Body` как есть (порядок ключей пользователя цел); srs — `rule_set` из **ВСЕХ** `Refs` (ловушка 20а закрыта) |
| `core/state/node_sections.go:268` | `ReadNodeSections` — форма v8 автоматически (типы те же) |
| `core/state/sync_dns.go` | правок не понадобилось: читает только `Kind`/`Ref`/`Tag` |
| `core/state/rule_identity.go:31` | `StableRuleID` читает `r.Name`; `DecodeBody` больше не зовётся, `sanitizeIDPart` не тронут |
| `core/state/rule_order.go` | `OrderNum` → `Num` (`ruleNum:136`, `MarkRuleOrder:88`, `placeRuleAt:332`, `NextUserRuleNum:231`); `SeedRequiredRules:151` — литерал `{"vars":{}}` заменён на `NewPresetRule` |

### 8.8 Фикстуры и тесты `core/state`

| Файл | Заметка |
|---|---|
| `core/state/testdata/v7_roundtrip.json` | **заморожен как вход миграции**; дополнен srs-правилом с двумя наборами, inline с `drop`, preset с непустыми `vars`, плоским user-DNS-сервером и user-DNS-правилом, секциями на узле `🇯🇵 Tokyo`. Генератора больше нет (v7-форму типы не умеют) |
| `core/state/testdata/v8_roundtrip.json` | **новая**; регенерация `GEN_V8_ROUNDTRIP_FIXTURE=1 go test -run TestGenerateV8RoundtripFixture ./core/state/` |
| `core/state/canonical_roundtrip_test.go` | roundtrip на v8-фикстуре; `TestGenerateV7RoundtripFixture`/`buildV7RoundtripFixture` удалены, добавлены `TestGenerateV8RoundtripFixture` и `freezeFixtureTimestamps` |
| `core/state/migration_v7_to_v8_test.go` | **новый**: байт-в-байт миграции, идемпотентность, `.v7.bak` + `FromVersion`, поимённая сверка «ничего не потеряно», неизвестный `kind`, гейт «v8 не читается как v7» |
| `core/state/rule_types_test.go` | переписан на вид/конструкторы; добавлены сценарии `SetOutbound` (порядок ключей), `reject`/`drop` вида, дедуп `Refs`, порядок ключей записи |
| `core/state/rule_identity_test.go` | `inlineBodyJSON`/`srsBodyJSON` → `inlineTestRule`/`srsTestRule` через конструкторы |
| `core/state/rule_order_test.go`, `rule_order_invariant_test.go` | `OrderNum` → `Num` механически |
| `core/state/sync_dns_test.go` | литералы preset-тел → хелпер `presetRuleRef` |
| `core/state/v6_integration_test.go` | ожидания v7 → v8 (`TestSave_AlwaysWritesV8`, `TestSave_V8_WhenHasPresetRef`) |
| `core/state/migration_scenarios_test.go` | ожидание `"schema": "sources_v7"` → `sources_v8` |

---

## 9. `core/build`, `core/config`, `core/backup`, `core/debugapi` ПОСЛЕ этапа S2

Адреса проверены на ветке `spec127/state-v8` после S2. Гейт этапа
(`gofmt -l core internal`, `go build ./core/... ./internal/...`,
`go vet ./core/...`, `go test ./core/...`) зелёный; про эталон v6mig — §9.7.

### 9.1 `core/state` — добавлено на S2

| Сущность | Адрес | Заметка |
|---|---|---|
| **`(*Rule).BodyMap() (map[string]interface{}, error)`** | `core/state/rule_types.go:443` (этап FIX) | тело правила КАРТОЙ, вместе с целью; копия на каждый вызов. Нужен там, где тело едет в конфиг/файл целиком и разбирать его на матчеры и цель незачем. Читатели: `resolve_route.go:267`, `:344`, `core/backup/export.go:533` |

### 9.2 Маршрут (`core/build/resolve_route.go`)

| Сущность | Адрес | Что стало |
|---|---|---|
| `ResolvedRouteRule.Num int` | `core/build/resolve_route.go:83` | бывший `OrderNum` |
| `resolveInlineRouteRule` | `core/build/resolve_route.go:262` | `DecodeBody` только ВАЛИДИРУЕТ запись; карта правила = `rule.BodyMap()` (`:267`) — цель уже в форме sing-box, `ApplyOutboundToRule` больше не зовётся |
| `resolveSrsRouteRule` — список наборов | `core/build/resolve_route.go:297` | `urls := sb.Refs` (вид отдаёт дедуплицированное поле записи); «всё или ничего» на `:299-311` без изменений, теги `SrsRuleSetTag(id, i)` тоже |
| `resolveSrsRouteRule` — тело | `core/build/resolve_route.go:344-349` | `rule.BodyMap()` + `routeRule["rule_set"] = …`: цель из тела, набор вписывает сборка |
| `ruleNum(r)` | `core/build/resolve_route.go:364` | бывший `ruleOrderNum`; читает `r.Num` |
| импорт `internal/outboundutil` | — | **убран из файла**: applyOutbound в маршруте больше не нужен |

**Осталось как было (не чинил, вне этапа):** `resolveSrsRouteRule` **не
заполняет `Num`** — srs-правило всегда приезжает в emit с `Num == 0` и потому
попадает в «голову» (`preset_merge.go:373`), ПЕРЕД правилами шаблона. Баг
досталcя от v7 (`git show HEAD:core/build/resolve_route.go`), к форме записи
отношения не имеет; его исправление сдвинет правила в `config.json` и покрасит
golden — это отдельное решение владельца.

### 9.3 DNS и слияние

| Адрес | Что стало |
|---|---|
| `core/build/resolve_dns.go:298-318` | **без изменений**: user-серверы и так читали `Body` + дописывали `tag` из поля (ловушка 2 закрыта в S1) |
| `core/build/resolve_dns.go:338-345` | без изменений: правила `user` — тело как есть |
| `core/build/preset_merge.go:373` | `r.Num < state.UserRuleNumStart` (бывший `r.OrderNum`) |
| `core/build/preset_merge.go:708-742` | `CollectSrsCachedPaths` — наборы из вида (`sb.Refs`, `:729`). **Через вид, а не `r.Refs` напрямую**: вид дедуплицирует, и расхождение с `resolve_route.go` объявило бы правило «частично закэшированным» и молча выбросило из конфига |
| `core/build/preset_merge.go:330/:412/:608` | три синтетических `state.State` — правок не потребовали: собираются из `ctx.rulesWithNodeSections()`/`dnsWithNodeSections()`, а те типо-независимы |
| `core/build/parsed_cache.go:70-95` | `RulesWithSelf` копирует `Num` (`:77-81`), плюс теперь копирует `Refs` и `Vars` — слайс и карта иначе разделялись бы с состоянием |
| `core/build/sync_outbounds.go`, `migrate_outbounds_spec058.go` | правок не потребовали: читают `pb.Vars` через вид, а вид собирает его из `r.Vars` |

### 9.4 `core/config` и корень `core/`

| Адрес | Что стало |
|---|---|
| `core/config_service.go:493-505` | orphan-GC `.srs` — по `r.Refs` НАПРЯМУЮ, без `DecodeBody`. Это keep-лист: запись с незаполненным именем всё равно ссылается на файл, и валидация вида удалила бы живой кэш |
| `core/config/node_document.go`, `core/config/subscription/singbox_sections_extract.go` | правок не потребовали: ходят через `state.NodeSectionsFromSingbox` / `state.ReadNodeSections`, форма v8 приезжает сама |

### 9.5 `core/debugapi`

| Адрес | Что стало |
|---|---|
| `core/debugapi/state_endpoints.go:138` | `st.MarshalV8()` (бывшая `MarshalV7`) — `GET /state/full` отдаёт корень v8 (`dns`, `warp`) |
| `core/debugapi/state_endpoints.go:213-215` | раздача номера при `mode=append` — `req.Rules[i].Num` |
| `core/debugapi/state_endpoints.go:236-238`, `:260-262`, `:279` | тексты/комментарии: `dns_options` → `dns`; про «у DNSOptions свой Unmarshal» снято (его больше нет) |
| PATCH правил `:186-194` | без изменений: валидация через `DecodeBody`, и она же теперь требует `name` у inline/srs — **чужой PATCH со старой формой (`body.name`) получит 422**, это намеренно |
| PUT/PATCH DNS `:282-308` | правок не потребовал: читает `state.DNSOptions` обычным Unmarshal, который в v8 и есть целевая форма |

### 9.6 `core/backup` (файл 0.12 не изменился ни на байт)

| Сущность | Адрес | Что стало |
|---|---|---|
| `exportRule` | `core/backup/export.go:462` | `Num` вместо `OrderNum`; preset — `r.Vars` (копия); inline/srs — через новый хелпер; повторная канонизация `NewSrsBody` **убрана** |
| **`ruleMatchAndOutbound(r)`** | `core/backup/export.go:527` | матчеры и цель порознь — ровно пара, из которой состоит запись 0.12. Через `BodyMap`, БЕЗ требования `name`: экспорт не роняет весь файл из-за одной записи без имени, как не ронял до v8 |
| **`dedupRefs(in)`** | `core/backup/export.go:554` | тот же канон наборов, что у `NewSrsRule` |
| `dnsRefFrom` / `exportDNS` | `core/backup/export.go:648` / `:634` | правок не потребовали: и раньше брали `Tag` снаружи и `Body` картой |
| `importRule` | `core/backup/import.go:626` | тело собирают ТОЛЬКО конструкторы (`NewPresetRule`/`NewInlineRule`/`NewSrsRule`), метаданные (`Enabled`, `Num`) дописываются поверх на `:681-683`; свой `json.Marshal(XBody)` из файла ушёл |
| `renumberImportedRules` / `importedAxisNum` | `core/backup/import.go:705` / `:730` | `Num` |
| `importDNS` | `core/backup/import.go:818` | правок не потребовал |
| `decodeBackupSections` | `core/backup/node_sections.go:29` | код без изменений (пронос как есть); в шапке файла записано, что внутри блока теперь форма v8 и что расхождение со схемой 0.12 закрывает волна 2 (ловушка 6) |

### 9.7 Тесты, golden и эталон

| Файл | Что сделано |
|---|---|
| `core/build/testhelpers_v8_test.go` | **новый**: `presetRule(ref, vars, enabled)` — единственный способ собрать запись пресета в тестах пакета |
| `core/debugapi/testhelpers_v8_test.go` | **новый**: `presetRule` + `inlineRule` |
| `core/build/golden_v8_gen_test.go` | **новый**: `TestGenerateGoldenV8Scenario` (генератор папки, `GEN_GOLDEN_V8=1`) и `TestGoldenV8ScenarioMatchesV7Expectation` (обе папки описывают ОДИН конфиг, и состояние сценария действительно v8) |
| `core/build/testdata/golden/real-v088-v8/` | **новый сценарий**: `template.json`/`cache.json`/`expected.config.json` — копии `real-v088`, `state.json` = `Parse`+`Save` v7-файла. Регенерация: `GEN_GOLDEN_V8=1 go test -run TestGenerateGoldenV8Scenario ./core/build/` |
| `core/build/srs_filename_test.go` | `mustMarshalSRS` → `srsRule(name,url,outbound)` через `NewSrsRule` |
| `core/build/dns_ruleset_dangling_test.go` | литерал `{"name":…,"srs_url":…}` → `NewSrsRule` |
| `core/build/node_sections_build_test.go`, `spec106_rule_order_emit_test.go`, `spec106_traffic_processing_test.go`, `fakeip_test.go`, `resolve_dns_test.go`, `preset_merge_test.go`, `sync_outbounds_test.go` | конструкторы вместо `json.Marshal(XBody)`, `Num` вместо `OrderNum` |
| `core/debugapi/state_endpoints_test.go` | `TestStateFull` ждёт корень v8 (`dns` есть, `dns_options`/`warp_accounts` нет) и `state.SchemaVersion`; правила — через конструкторы |
| `core/debugapi/remote_endpoints_test.go:355-362` | PATCH по проводу — в форме v8 (`name` снаружи, `action` в теле) |
| `core/config/unsupported_node_test.go:127` | `MarshalV8` |
| `core/backup/backup_test.go`, `corpus_test.go`, `rule_order_invariant_test.go`, `node_sections_roundtrip_test.go` | конструкторы и `Num`; `ruleName` читает `r.Name`, `ruleRefs` — `r.Refs`; в roundtrip ожидание `"order_num":945` → `"num":945` |
| **корпус `contract/corpus/backup`** | зелёный **без единой правки ожиданий** |
| **golden** | `real-v088` (state v7, проверка миграции) и `real-v088-v8` (state v8) — обе зелёные, `expected.config.json` не пересчитывался |
| **эталон v6mig** | **красный, но это НЕ регрессия S2**: `ETALON_V6MIG=1 go test ./core -run TestEtalonV6MigOutboundSnapshot` падает и на чистом `2d64ca72` теми же строками (`[P] NL-1 •-2` vs `[P] NL-1-2 •`, `[P]auto` vs `[P]select-auto`). Снимок заморожен на W2 SPEC 118 (`e478a925`), а деривацию тегов меняли волны W4/W5–W8 того же спека. Байты `outbounds.actual.json` из этой ветки и из `2d64ca72` **совпадают** — цепочка v6→v7→v8 даёт ровно тот же результат, что v6→v7 |

### 9.8 Документация формата (S2.3)

| Файл | Что стало |
|---|---|
| `docs/WIZARD_STATE.md`, `docs/WIZARD_STATE.ru.md` | врезка «схема v8» вместо «схема v7» (старая оставлена ниже), корневой пример (`rules[]` с `num`/`name`/`refs`/`vars`, корень `dns`/`warp`), таблица полей правила, три примера записей, §3.6 `dns` (бывший `dns_options`), тела DNS «объект sing-box как есть, `tag` снаружи», §6.1/§6.2, строка миграции v6→v7 и v7→v8, карта файлов (`disk_v8.go`, `migration_v7_to_v8.go`) |
| `docs/API.md`, `docs/API.ru.md` | `PATCH /state/dns` — «вся секция `dns`» |
| `docs/DATA_FLOW.md`, `docs/DATA_FLOW.ru.md` | `state.dns_options.servers[]` → `state.dns.servers[]`; `body.srs_urls` → `refs[]` |
| `docs/ARCHITECTURE_PACKAGES.md`, `.ru.md` | строка `dns_options.go` — struct-теги и `body` |
| **не трогал**: `docs/release_notes/*` (исторические записи), строки индекса SPEC 056, `template.dns_options` (ключ ШАБЛОНА, он не переименовывался) | |

---

## 10. `ui/configurator` и `core/template` ПОСЛЕ этапа S3

Адреса проверены на ветке `spec127/state-v8` после S3. Гейт волны
(`gofmt -l .`, `go build ./...`, `go vet ./...`, `go test ./...`, golden) —
§10.5.

### 10.1 Одно имя номера оси: `OrderNum` → `Num`

Переименование механическое и сквозное — инвариант 7 волны («`rg OrderNum`
пусто вне `migration_v7_to_v8.go`») выполнен. Затронуты не только поля, но и
имена функций, иначе имя уехавшего поля осталось бы жить в API пакета.

| Было | Стало | Адрес |
|---|---|---|
| `RuleState.OrderNum *int` | `RuleState.Num` | `ui/configurator/models/rule_state.go:41` |
| `PresetRefState.OrderNum *int` | `PresetRefState.Num` | `ui/configurator/models/preset_ref_state.go:38` |
| `NodeRuleRef.OrderNum *int` | `NodeRuleRef.Num` | `ui/configurator/models/node_rule_ref.go:43` |
| `copyOrderNum` | `copyNum` | `ui/configurator/models/preset_ref_sync.go:309` |
| `(m).slotOrderNum` / `setSlotOrderNum` | `slotNum` / `setSlotNum` | `ui/configurator/models/rule_order_axis.go:25`, `:44` |
| `EnsureRuleOrderNums` | `EnsureRuleNums` | `ui/configurator/models/rule_order_axis.go:165` |
| `NextRuleOrderNum` | `NextRuleNum` | `ui/configurator/models/rule_order_axis.go:228` |
| `PresetRuleOrderNum` | `PresetRuleNum` | `ui/configurator/models/rule_order_axis.go:241` |
| **`(*Preset).OrderNum() int`** | **`(*Preset).StartNum() int`** | `core/template/preset_lite.go:41` |
| локальная `orderNum()` в конвертере пресета | `axisNum()` | `ui/configurator/tabs/preset_ref_convert.go:44` |

`(*Preset).StartNum` — геттер поля `Preset.Num` шаблона; переименовать его в
`Num()` нельзя (коллизия с полем), поэтому у него своё имя, а не `Num`. Зовут
его четверо: `core/template/preset_lite.go:63`,
`ui/configurator/models/rule_order_axis.go:248` и два места в
`core/build/spec106_traffic_processing_test.go:59-60`.

Вызывающие переименованных функций (компилятор ведёт):
`ui/configurator/configurator.go:721`, `ui/configurator/business/node_pool.go:173`,
`ui/configurator/presentation/presenter_state_helpers.go:125`,
`ui/configurator/presentation/preset_ref_helpers.go:275`,
`ui/configurator/dialogs/add_rule_dialog.go:816`,
`ui/configurator/tabs/preset_ref_convert.go:43`.

### 10.2 Эмиссия правил из UI — только конструкторы (ловушка 20 закрыта в UI)

| Сущность | Адрес | Заметка |
|---|---|---|
| **`presetStateRule(pr *PresetRefState) state.Rule`** | `ui/configurator/models/preset_ref_sync.go:108` | **новая**; единственный писатель записи `kind=preset` в пакете. Через `state.NewPresetRule` + `Enabled`/`Num` поверх. Заменила `jsonMarshalPreset` (удалена) и два одинаковых литерала `state.Rule{…Body: body}` |
| вызовы `presetStateRule` | `ui/configurator/models/preset_ref_sync.go:81`, `:658` | `EmitStateRulesInAxisOrder` и `SyncPresetRefsToStateRules` |
| `customRuleStateToV6Rule` — ветка srs | `ui/configurator/models/preset_ref_sync.go:288` | `state.NewSrsRule(label, urls, outbound)`; `state.NewSrsBody` больше нет |
| `customRuleStateToV6Rule` — ветка inline | `ui/configurator/models/preset_ref_sync.go:300` | `state.NewInlineRule(label, match, outbound)`; `stripOutboundAction` (`:317`) остался — он готовит карту матчеров UI ДО конструктора |
| `customRuleStateToIdentityRule` | `ui/configurator/models/preset_ref_sync.go:253` | `state.Rule{Kind: inline, Name: rs.Rule.Label}` — тела больше не собирает: `StableRuleID` в v8 читает поле `Name`, и строка id та же, что была от `body.name` |

`SyncStateRulesToPresetRefs` (`:666`) правок не потребовала: читает вид
(`DecodeBody` → `*PresetBody`), а вид собирает `Vars` из поля записи.

### 10.3 DNS из UI

| Адрес | Что стало |
|---|---|
| **`syncDNSServersOnly`, ветка `user`** — `ui/configurator/models/preset_ref_sync.go:450-457` | **правка по существу:** в `body` теперь НЕ едут `tag`/`kind`/`ref`/`enabled`. До v8 лишний `tag` в теле выбрасывал кастомный `MarshalJSON` у `DNSServer` (ловушка 2); в v8 его нет, и без этого фильтра тег писался бы в файл дважды — расхождение с миграцией v7→v8 и с нормой §0. Ловится тестом §10.4 |
| `buildDNSRulesFromOrder` — `:531-540` | без изменений: фильтр `kind`/`ref`/`enabled` и так оставлял в `Body` тело правила |
| `DNSRuleOrderFromStateRules` — `:619-628` | без изменений (тот же фильтр на обратном пути) |
| `populateUserDNSFromState` — `ui/configurator/presentation/preset_ref_helpers.go:183-212` | кода не потребовал: и раньше собирал плоский wizard-JSON из `Tag` + `Body`. Комментарий `:194-198` переписан: в v8 `Body` чист по форме, фильтр оставлен страховкой от чужого файла |
| `applyDNSServerEnabledFromState` — `ui/configurator/presentation/preset_ref_helpers.go:132` | без изменений (работает по блобам модели, не по типам состояния) |
| `ui/configurator/business/node_sections.go:108-134` | без изменений: `NodeSectionDNSForModel` уже брал тело из `srv.Body` и дописывал `tag` из поля — ровно форма v8 |
| `ui/configurator/tabs/dns_tab.go`, `dns_user_rules.go`, `dns_unified_rules.go`, `dns_preset_bundled.go` | правок не потребовали: работают с `DNSUserRule.Body` (он и был `body`) и читают из `srv.Body` только `type`/`server`/`description` |

### 10.4 Строки правил узла и ось

| Сущность | Адрес | Что стало |
|---|---|---|
| **`nodeRuleDisplayName`** | `ui/configurator/models/node_rule_ref.go:145` | Имя берётся полем `r.Name` по виду записи (`inline`/`srs`), а не через `DecodeBody`. Вид валидирует запись целиком, и у srs-правила с пустыми `refs` вернул бы ошибку — строка списка осталась бы без подписи там, где имя есть. У `preset` имени нет, отдаётся `""` — как и раньше |
| `SeedNodeRuleRefs` / `SyncNodeRuleRefsToSources` | `ui/configurator/models/node_rule_ref.go:80`, `:159` | только переименование поля (`Num`) |
| `axisProxyRules` | `ui/configurator/models/rule_order_axis.go:66` | синтетический `state.Rule` строится по-прежнему из `Kind`/`Ref`/`Num` — тела у прокси нет и в v8 не нужно (ось читает только эти поля) |
| `markProxyGaps`, `applyAxisAfterMove`, `SortRuleOrderByAxis`, `axisNum` | `rule_order_axis.go:141`, `:100`, `:184`, `:203` | только `Num` |
| конвертация preset→custom | `ui/configurator/tabs/preset_ref_convert.go:43`, `:92`, `:112`, `:160` | пишет `RuleState.Num`; работа идёт с UI-типами, форма состояния её не касается |
| новое правило из диалога | `ui/configurator/dialogs/add_rule_dialog.go:816` | `Num: wizardmodels.NextRuleNum(model)` |

### 10.5 Тесты S3

| Файл | Что сделано |
|---|---|
| **`ui/configurator/presentation/spec127_state_v8_roundtrip_test.go`** | **новый, единственный добавленный сценарий волны в UI:** `TestSpec127UIStateRoundTripV8` — модель → `EmitStateRulesInAxisOrder` + `SyncDNSByOrderToState` → **байты** `MarshalV8` → `Parse` → обратно в модель. Проверяет: порядок записей осевой; у inline `name`/`num` снаружи, в `body` нет обёртки `match`, цель внутри; у srs ОБА набора в `refs[]` и в `body` нет `rule_set`; у preset нет `body`, есть `vars`; у DNS-сервера `tag` полем и НЕТ его в `body`; обратно приезжают те же подписи, цели, оба набора (репорт 1.5.5) и номера оси. Через файл, а не через структуры: лишний ключ в `body` виден только в сериализованном виде |
| `ui/configurator/models/rule_order_axis_test.go` | `inlineRule`/`presetRule` → `corestate.NewInlineRule`/`NewPresetRule`; фолбэк безымянного правила в `customRulesFromStateRules` (`:88-102`) читает `BodyMap()` и делит тело на матчеры и цель — в v8 `body` и есть правило sing-box, старый `json.Unmarshal(r.Body, &InlineBody{})` вернул бы пустоту |
| `ui/configurator/models/rule_order_invariant_test.go` | `unnamedRule` → `NewInlineRule("")`; импорт `encoding/json` ушёл |
| `ui/configurator/models/preset_ref_sync_test.go:76-94` | srs-сценарий на форму v8: наборы проверяются полем записи `Refs` и видом `SrsBody.Refs`; `sb.SrsURL` больше нет |
| `ui/configurator/business/srs_rule_reopen_test.go` | **не трогал намеренно:** его фикстура — state v7 на входе `corestate.Parse`, то есть он стал ещё и сквозным тестом миграции v7→v8 через UI-путь (правило с `srs_url` + правило с `srs_url`+`srs_urls` доезжают до `RuleSets` диалога) |

Полный прогон волны — в отчёте этапа S3.

## 11. Этап FIX — правки по ревью волны 1

Восемь подтверждённых находок ревью. Три из них (`#1`/`#4`, `#3`/`#5`/`#8`,
`#6`/`#7`) — один дефект в разных местах, поэтому и правок пять.

### 11.1 Самостоятельный `action` — эффект правила, а не цель

`action` со значением, отличным от `reject`, — не цель правила, а его эффект
(`sniff`, `hijack-dns`, `resolve`, `route`). Код волны 1 снимал ключ `action`
вместе с `outbound`/`method` в четырёх местах, и правило превращалось в матчер
без действия: в v7 такой ключ лежал в `body.match`, эмиттер копировал `match`
дословно, и он доезжал до `config.json`.

| Адрес | Что стало |
|---|---|
| **`isTargetKey`** — `core/state/rule_types.go:358` | **новая:** ключ принадлежит ЦЕЛИ? `outbound`/`method` — всегда, `action` — только со значением `reject`. Единственная точка истины для всех четырёх раскладок |
| `NewInlineRule` — `core/state/rule_types.go:177`, фильтр `:180` | фильтр матчеров через `isTargetKey` |
| `DecodeBody`, ветка inline — `core/state/rule_types.go:383`, фильтр `:408` | `InlineBody.Match` через `isTargetKey`: вид больше не прячет эффект |
| `(*Rule).SetOutbound` — `core/state/rule_types.go:210`, фильтр `:236` | смена цели сохраняет самостоятельный `action` вместе с матчерами |
| **`joinRuleBodyWithTarget`** — `core/state/migration_v7_to_v8.go:271`, фильтр `:296-310` | из `match` снимаются ТОЛЬКО те ключи, которые сейчас же перепишет цель. Пустой v7-`outbound` цели не даёт (`ApplyOutboundToRule` на пустой строке не пишет ничего) — значит и снимать нечего |
| `ruleMatchAndOutbound` — `core/backup/export.go:527`, раскладка `:532-556` | `action` уезжает в `match` файла 0.12, если он не `reject`/`drop`: без этого `import(export(x))` терял ключ на каждом круге (инвариант волны 4) |

### 11.2 Запись неизвестного вида теряла номер

Запись чужого `kind` проносилась сырой — вместе с ключом `order_num`, которого
типы v8 не читают. Первый же `Save` стирал номер с диска, `MarkRuleOrder`
раздавал записи номер из пользовательского диапазона, и она переезжала вперёд
соседей — менялся порядок правил маршрута.

| Адрес | Что стало |
|---|---|
| **`renameOrderNumKey`** — `core/state/migration_v7_to_v8.go:225` | **новая:** `order_num` → `num` в сырой записи; остальные ключи и их порядок не трогаются. Обе формы сразу — авторитетна новая |
| `migrateV8Rule`, ветка `default` — `core/state/migration_v7_to_v8.go:207-213` | запись по-прежнему проносится байтами, но через `renameOrderNumKey` |

### 11.3 `id`/`name` DNS-правила — метаданные, и оба снаружи

Миграция снимала из тела `id`, но оставляла `name` — запись ехала в файл с
дублем метаданных (два источника истины), а `id` пропадал из тела, откуда в v7
эмитился в `config.dns.rules`. Списки исключений у сервера (`flatDNSBody`) и у
правила разошлись.

| Адрес | Что стало |
|---|---|
| `migrateV8DNSRules` — `core/state/migration_v7_to_v8.go:396`, allowlist `:403` | исключаются `kind`/`ref`/`enabled`/`id`/**`name`**: внутри `body` остаётся объект sing-box и ничего кроме (норма `ONE_NAMESPACE.md` §1 стр. 94). Ключи `id`/`name` у DNS-правила ядро всё равно не знает — в рабочем конфиге их не было |

### 11.4 Круг визарда стирал провозимые метаданные

`Rule.ID` и `DNSRule.ID`/`Name` — необязательные поля второй стороны: лаунчер
их не заполняет, но обязан не терять. Сохранение визарда пересобирает
`state.Rules` и `state.DNS` ЦЕЛИКОМ из модели, а модель этих полей не знала —
метаданные пропадали с первого же сохранения.

| Адрес | Что стало |
|---|---|
| `DNSUserRule` — `ui/configurator/models/dns_user_rule.go:28-34` | добавлены поля `ID`/`Name`: модель провозит их, в UI они не показываются |
| `DNSRuleOrderFromStateRules` — `ui/configurator/models/preset_ref_sync.go:629-634` | `ID`/`Name` едут из состояния в модель |
| `buildDNSRulesFromOrder` — `ui/configurator/models/preset_ref_sync.go:539-546` | и обратно в состояние |
| **`CarryRuleMetadata`** — `core/state/rule_identity.go:95` | **новая:** восстанавливает `Rule.ID` на свежеэмитированных записях по `StableRuleID` (та же identity, по которой UI сопоставляет слоты оси). Нужна потому, что у `RuleState`/`PresetRefState` поля `ID` нет, а завести его значило бы тянуть поле через легаси-вид `CustomRule` и типы `core/template` |
| **`CarryDNSRuleMetadata`** — `core/state/rule_identity.go:132` | **новая:** то же для DNS-правил, сопоставление по позиции user-записей (у DNS-правила нет ни имени-identity, ни `ref`). Страховка для записей, приехавших мимо модели (импорт бэкапа, фолбэк на `DNSRulesText`) |
| `ui/configurator/presentation/presenter_state.go:143`, `:175` | единственный авторитетный путь сохранения зовёт обе функции поверх эмиссии из модели |

### 11.5 Тесты этапа FIX

Новых файлов нет — расширены существующие сценарии (память проекта: россыпи
юнитов не плодить).

| Файл | Что сделано |
|---|---|
| `core/state/migration_v7_to_v8_test.go` | **`TestMigrateV7ToV8_UnknownKindKept` переписан на путь ЧЕРЕЗ ДИСК** (`Load` → `Save`): проверка по промежуточному документу дефекта 11.2 не видела, потому что до `parseV8`/`Save` не доходила. Сверяются `num` на диске, отсутствие `order_num`, `id`, целость тела и номера соседей. **Добавлены** `TestMigrateV7ToV8_StandaloneActionKept` (четыре формы: `sniff`/`hijack-dns`/`reject` при пустой цели и `sniff` при непустой — последний остаётся перезаписан целью, как делал `ApplyOutboundToRule`) и `TestMigrateV7ToV8_DNSRuleMetadataOutsideBody` |
| `core/backup/backup_test.go` | `TestRoundTripLossless` расширен правилом-эффектом (`mkEffectRule`, `:107`): круг `export`→`import` обязан вернуть `action: sniff`. Это инвариант волны 4 |
| `ui/configurator/presentation/spec127_state_v8_roundtrip_test.go` | тот же сценарий продолжен кругом провозимых метаданных: состояние → модель визарда → состояние для `DNSRule.ID`/`Name` и для `Rule.ID`, плюс проверка, что метаданные не продублировались внутри `body` |

Каждая правка проверена откатом: без неё соответствующий тест краснеет.

---

## 12. `core/backup` и `core/state` ПОСЛЕ волны 2, этап B1 (два писателя)

Адреса проверены на ветке `spec127/backup-10` после B1. Гейт этапа
(`gofmt -l core internal`, `go build ./...`, `go vet ./core/...`,
`go test ./core/state/... ./core/backup/... ./core/build/...`) зелёный; корпус
`contract/corpus/backup` зелёный **без единой правки ожиданий**; про эталон
v6mig — §9.7 (красный и до волны, не регрессия).

### 12.1 Идентификация подписки — объект в состоянии

| Сущность | Адрес | Заметка |
|---|---|---|
| **`type SubscriptionIdentity`** | `core/state/subscription_identity.go:28-47` | переехал из `core/backup/types.go`; форма и json-теги те же, поэтому запись 0.12 не изменилась ни на байт |
| `(*SubscriptionIdentity).UnmarshalJSON` | `core/state/subscription_identity.go:71` | различает «ключа нет» и `null`; помнит состав ключей (`presentKeys`) |
| `UnappliedKeys` / `MarkPresentKeys` / `IsEmpty` / `Clone` | `:116`, `:131`, `:141`, `:152` | `Clone` — новая: файл не должен делить указатели с состоянием |
| **`Source.Identity *SubscriptionIdentity`** | `core/state/sources_v7.go:233-252` | вместо четырёх плоских `user_agent`/`hwid`/`send_hwid`/`hash_device_model` |
| `IdentityUserAgent` / `IdentityHWID` / `IdentitySendHWID` / `IdentityHashDeviceModel` | `core/state/subscription_identity.go:190`, `:198`, `:206`, `:215` | читатели не знают ни про указатели, ни про nil-объект |
| `SetIdentityUserAgent` / `SetIdentityHWID` / `SetIdentitySendHWID` / `SetIdentityHashDeviceModel` / `SetIdentity` | `:230`, `:234`, `:238`, `:242`, `:278` | правка одной настройки и четвёрки разом; объект снимается, когда опустел |
| `migrateV8SourceIdentity(src, where)` | `core/state/migration_v7_to_v8.go:491` | четыре плоских ключа v7 → объект `identity`; пустая строка UA/HWID = «как в системе» и ключа не заводит |
| `normalizeSourceShape` — сброс полей подписки | `core/state/sources_v7.go:426`, `:459` | `!s.Identity.IsEmpty()` вместо четырёх сравнений |
| **`CloneRule` / `CloneDNSServer` / `CloneDNSRule`** | `core/state/node_sections.go:154`, `:156`, `:162` | публичные обёртки внутренних копий: их зовёт писатель 1.0 |

**Читатели четырёх полей переведены:** `core/source_identity.go:23-28` (строки `:24-27`)
(мостик к fetcher'у), `ui/configurator/tabs/source_identity_block.go`
(форма подписки: 12 писателей и 4 читателя), `source_edit_overview.go:239-244` (строки `:240-243`)
(диагностический Reload), `core/backup/export.go`→`legacy_write_012.go`
(`exportSourceIdentity`), `core/backup/import.go:495`
(`importSourceIdentity` → `SetIdentity` на `:511`), `core/backup/merge.go:135`
(`applySubscriptionSettings` → `Identity.Clone()`).

Фикстуры (`core/state/testdata/v8_roundtrip.json`, golden
`real-v088-v8/state.json`) перегенерации **не потребовали**: identity в них
нет, и байты не изменились (проверено регенерацией — diff пуст).

### 12.2 Два писателя экспорта

| Сущность | Адрес | Заметка |
|---|---|---|
| **`type ExportFormat`** (`ExportFormat012`, `ExportFormat10`) | `core/backup/export.go:22-32` | ноль = 0.12: вызывающий, который про формат не знает, получает прежнее поведение |
| **`const BackupExportFormatDefault = ExportFormat012`** | `core/backup/export.go:39` | ОДНА константа на приложение (SPEC §4); переключение дефолта после релиза LxBox — правка этой строки |
| `ExportOptions{AppVersion, Platform, Now, Format}` | `core/backup/export.go:42-56` | добавлено `Format` |
| **`ExportFile(path, s, opts) ([]Warning, error)`** | `core/backup/export.go:61` | единственная точка на оба писателя; зовёт `WriteFile` либо `WriteFile10` |
| общие хелперы обоих писателей | `core/backup/export.go:78-` | `sourceExportName`, `exportWarp`, `droppedLocalOnlyFields`, `dedupRefs`, `exportVars`, `routeFinal` |
| **`Export012`** (бывший `Export`) | `core/backup/legacy_write_012.go:59` | + `exportDirections`, `exportSourceRef`, `exportChain`, `exportSubscription`, `exportSourceIdentity`, `exportServer`, `exportFolder`, `exportServerNode`, `exportRule`, `ruleMatchAndOutbound`, `exportDNS`, `dnsRefFrom` — весь маппер 0.12 в одном файле, удаляется целиком после релиза LxBox |
| **секции узла 0.12-писателем НЕ пишутся** | `core/backup/legacy_write_012.go:139-144` (корневой узел), `:370-372` (член папки) | норма ONE_NAMESPACE §4; потеря названа `backup_local_only_dropped` с полем `sections`. Ловушка §7.6 (код писал форму состояния в поле, где схема обещала форму бэкапа) закрыта снятием писателя, а не подгонкой формы |
| чтение секций из файлов 0.12 | `core/backup/node_sections.go`, `merge.go:294`, `import.go:561` | **не тронуто**: такие файлы уже у пользователей (П3) |
| **`WriteFile10(path, b)`** / `writeFileAtomic` | `core/backup/file.go:50`, `:62` | запись 1.0; атомарная замена вынесена в общий хелпер |

### 12.3 Форма файла 1.0 и её писатель

| Сущность | Адрес | Заметка |
|---|---|---|
| **`const FormatVersion10 = 2`** | `core/backup/backup10.go:39` | тот же int-маркер `lx_backup`, что у 0.x (там 1) |
| **`type Backup10`** | `core/backup/backup10.go:44-67` | порядок полей = порядок ключей: `lx_backup`, `exported_by`, `exported_at`, `sources`, `directions`, `rules`, `dns`, `vars`, `route`, `warp`. `Rules []state.Rule` и `DNS *state.DNSOptions` — **типы состояния как есть** |
| **`type Source10`** | `core/backup/backup10.go:85-128` | поля `state.Source` перечислены ЯВНО. Встраивание не годится: `json:"-"` поверх встроенной структуры не участвует в разрешении конфликта имён, и `meta`/`update_status`/`pending_disabled` уезжали в файл (проверено на живом выводе) |
| **`source10ExcludedStateKeys`** | `core/backup/backup10.go:133-138` | что исключено намеренно и почему; список читает сверочный тест |
| **`Export10(s, opts) (*Backup10, []Warning, error)`** | `core/backup/export10.go:34` | чистая функция состояния; `backup_local_only_dropped` не эмитит (полей без дома не осталось), `WarnBackupReplaceTagDerived` остался |
| `export10Source(src) (Source10, bool)` | `core/backup/export10.go:105` | у подписки `Nodes=nil` и `Disabled` из `exportDisabledMap`; `Fold` из `exportFold`; `Identity` — объект состояния копией |
| `export10Rules` / `export10DNS` | `:161`, `:176` | срезы состояния копиями (форма записи не трогается) |
| `cloneTagPolicy` / `cloneUpdateSpec` / `cloneSkip` / `cloneNodes` / `cloneNode` | `:201`, `:209`, `:221`, `:236`, `:247` | снимок момента, а не окно в живые данные |

### 12.4 UI экспорта

| Адрес | Что стало |
|---|---|
| `ui/configurator/tabs/settings_backup.go:99` | `handleBackupExport` спрашивает формат ДО выбора пути: `dialog.ShowCustomConfirm` с чекбоксом `settingsBackupFormatNewText` («Backup format 1.0 (new; requires LxBox with 1.0 import)») и пояснением `settingsBackupFormatHintText`; начальное состояние — из `BackupExportFormatDefault` |
| `ui/configurator/tabs/settings_backup.go:126` | `runBackupExport(st, format, win)` — выбор пути и запись через `backup.ExportFile` |
| строки локализации | `:48-53` | новые ключи английские, через `locale.T`; `bin/locale/ru.json` не трогался |

### 12.5 Тесты этапа B1

| Файл | Что сделано |
|---|---|
| **`core/backup/export012_etalon_test.go`** | **новый**: эталоны 0.12-писателя на четырёх состояниях (`mkstate`, `sections`, `v8fixture` = `core/state/testdata/v8_roundtrip.json`, `realv088` = golden `real-v088-v8/state.json`) плюс коды предупреждений. Эталоны сняты **до** правок `core/backup`; регенерация — `GEN_EXPORT012_ETALON=1 go test -run TestExport012MatchesEtalon ./core/backup/`. Сверено: у `mkstate` и `realv088` вывод **байт-в-байт прежний**, у `sections` и `v8fixture` единственная разница — снятые секции и добавленный `backup_local_only_dropped` |
| `core/backup/testdata/export012_*.json` | **новые** эталоны (4 файла) |
| `core/backup/node_sections_roundtrip_test.go` | **переписан на 1.0**, сценарии не выброшены: `TestBackupNodeSectionsFormat10` (1.0 везёт секции записями состояния, правила узла в корневой `rules[]` не уезжают, экспорт — снимок), `TestBackupNodeSections012NotWrittenButNamed` (0.12 не пишет и говорит), `TestBackupNodeSectionsLegacyFileStillRead` (файл 0.12 с секциями читается как прежде, файл без поля локальные секции не трогает) |
| `core/backup/purity_test.go` | **расширен**: `TestExport10IsPureFunctionOfState` (чистота + отсутствие общей памяти с состоянием по всем четырём местам, где легко оставить указатель), `TestExport10FileShape` (состав файла §6.0 на богатом состоянии: identity объектом, кэш узлов снят, `disabled`/`fold` формой контракта, правила и DNS записями состояния, запрещённые ключи не всплывают), `TestSource10CoversStateSourceKeys` (рефлексией: поле, добавленное в `state.Source`, обязано появиться в `Source10` либо в списке исключений с причиной — проверено откатом: снятый `max_nodes` красит тест) |
| `core/state/source_user_agent_test.go` | переписан на v8: `TestSubscriptionIdentityRoundTrip` (объект в JSON, правка одной настройки не уносит остальные, пустая идентификация снимает объект) и `TestSubscriptionIdentityUnmarshalDistinguishesNull` |
| `core/backup/identity_test.go`, `merge_test.go`, `convert_v7_test.go` | литералы четырёх полей → `SetIdentity`/`Identity*`-читатели; сценарии те же |
| `core/backup/*_test.go` (35 мест) | `Export(` → `Export012(` |

**Не сделано на этапе B1 (следующие этапы волны):** импорт 1.0 (`import10.go`),
разделение `Import` на декодирование и слияние, `legacy_read_0x.go`,
`Parse` для `lx_backup: 2` и раздельные списки `scanUnknown` (ловушка §7.26),
код `backup_section_record_dropped` в `contract/registry/backup_warnings.json`,
совместная перенумерация оси, debug API `/backup/*` (W2.9), `upcoming.md`.
Поэтому файл 1.0 сейчас **пишется, но ещё не читается**: `Parse` знает только
`lx_backup: 1`.

---

## 13. `core/backup` ПОСЛЕ волны 2, этап B2 (импорт: два входа, одно слияние)

Адреса проверены на ветке `spec127/backup-10` после B2. Гейт этапа
(`gofmt -l core internal`, `go build ./...`, `go vet ./core/...`,
`go test -count=1 ./core/backup/... ./core/state/...`) зелёный; корпус
`contract/corpus/backup` зелёный **без единой правки ожиданий**; эталоны
0.12-писателя из B1 (`testdata/export012_*.json`) зелёные; про эталон v6mig —
§9.7 (красный и до волны, не регрессия).

### 13.1 Шов: декодирование ↔ слияние

Импорт разрезан надвое. Каждый формат превращает файл в `decodedFile` —
записи СОСТОЯНИЯ плюс порядок плюс предупреждения разбора; дальше работает
ОДИН код слияния. Второй экземпляр правил §9 под второй формат был бы вторым
источником истины: разойдись они на строку — и один файл дал бы у
пользователя два разных состояния.

| Сущность | Адрес | Заметка |
|---|---|---|
| **`type decodedFile`** | `core/backup/decoded.go:42-67` | `Sources`/`Directions`/`Rules`/`DNS`/`Vars`/`RouteFinal`/`Warp` + `KnownTagsFromFile` (теги, которые породит сам импорт: дериваты свёртки 0.x) + `Warnings` разбора |
| **`type decodedSource`** | `core/backup/decoded.go:86-108` | `Kind` (по какому ключу §9 сливать), `Src state.Source`, `Folder` (имя папки у члена 0.x), `FileFolderID` (id папки В ФАЙЛЕ, 1.0), `Sections`/`MemberSections` — **было ли поле** `sections`, а не «есть ли в нём записи» |
| `decodedSourceKind` | `core/backup/decoded.go:70-83` | `decodedSubscription` (url) / `decodedServer` (тело) / `decodedFolder` (имя) / `decodedChain` (тег) |
| `type decodedDNS` | `core/backup/decoded.go:116-121` | те же `state.DNSServer`/`state.DNSRule`, а не третья структура |

**Порядок хранится ПЛОСКИМ списком в порядке файла** — не деревом: слияние
0.x идёт одним проходом «корневой узел, член папки, снова корневой», порядок
нормативен (§9 п. 8), и группировка по видам переставила бы `s.Sources` после
импорта, а с ними и байты обратного экспорта.

### 13.2 Вход 0.x (`legacy_read_0x.go`)

Весь перевод чужих имён в свои. Файл отдельный от `legacy_write_012.go`
намеренно: **писатель** 0.12 удаляется после релиза LxBox, **читатель** — нет,
он живёт, пока живы выпущенные файлы.

| Сигнатура | Адрес | Заметка |
|---|---|---|
| **`decodeLegacy(b *Backup, opts) (*decodedFile, error)`** | `core/backup/legacy_read_0x.go:49` | подписки → серверы → Направления → цепочки → правила; порядок файла |
| `decodeLegacyDNS(dns *DNS) *decodedDNS` | `:140` | `name`+`value` → `Tag`+`Body`; тело только у `kind=user` |
| **`dnsRefTag(ref DNSRef)`** | `:183` | читает **`name`**, а не схемный `tag` (ловушка §7.7): так пишут реальные файлы 0.12 |
| `importSubscription` / `subscriptionLabel` / `importSourceIdentity` / `backupReplaceTag` | `:225`, `:289`, `:308`, `:339` | переехали из `import.go` без изменений семантики |
| `importServer` / `serverLabel` / `importChain` | `:352`, `:402`, `:418` | `importServer` теперь возвращает и предупреждения секций (`:375-380`) |
| `importRule` / `ruleLabel` / `errSkipRule` | `:441`, `:504`, `:43` | без изменений; чужой kind — пропуск с warning |
| `importDirections` / `importSourceRef` / `ensureSourceID` | `:188`, `:201`, `:208` | |

### 13.3 Вход 1.0 (`import10.go`)

| Сигнатура | Адрес | Заметка |
|---|---|---|
| **`decode10(b *Backup10, opts) (*decodedFile, error)`** | `core/backup/import10.go:28` | копии записей; известные цели = `opts` + Направления, цепочки и **группы свёртки** этого же файла |
| `decode10Source(in Source10, subIndex int)` | `:102` | `Source10` → `state.Source`; у подписки `disabled{}` → `PendingDisabled`; у папки состав в `Nodes` + `FileFolderID` |
| **`foldTag10(in, index)`** | `:190` | `fold` — форма контракта, тега в ней НЕТ: тег выводится тем же позиционным деривативом (D-081), что считает `replaceTagSurvivesExport` на экспорте. Иначе правила файла метили бы в `[P]select`, а группа на приёмнике звалась бы иначе |
| `disabledTags10` | `:207` | ключи карты отсортированным списком (обход карты в Go случаен, а поле уезжает на диск) |
| **`decode10Rule(r, known, presets)`** | `:226` | форма записи НЕ трогается; семантика та же: цель, которой нет → `enabled=false` с `backup_unknown_outbound` |
| **`ruleTarget10(r)`** | `:252` | цель читается ВИДОМ (`DecodeBody`), а не сырым телом: вид уже знает, что `action: reject` — это цель |
| `decode10DNS` | `:277` | копии записей состояния |

### 13.4 Единое слияние

| Сигнатура | Адрес | Заметка |
|---|---|---|
| **`Import(s, *Backup, opts)`** | `core/backup/import.go:227` | вход 0.x: `decodeLegacy` + `applyDecoded` |
| **`Import10(s, *Backup10, opts)`** | `core/backup/import.go:250` | вход 1.0: `decode10` + `applyDecoded` |
| **`ImportFile(s, *File, opts)`** | `core/backup/import.go:273` | развилка форматов ОДНА, у вызывающих её нет |
| **`applyDecoded(s, dec, opts)`** | `core/backup/import.go:292` | `s.Rules = nil` (`:311`, единственная полная замена §9 п. 7); снимок `takenRootTags` (`:321`) → Направления → `mergeSources` → `rewriteFolderLinks` (с §23 — `rewriteLinks`) → `resolveImportedHops` → правила → ось → `route.final` → `vars` → DNS → warp |
| `importKnownTags(opts, dec, s)` | `:405` | цели для проверки `route.final`, считаются ПОСЛЕ слияния по живому состоянию |
| **`renumberImportedAxis(rules, sectionRules)`** → с §25 **`placeImportedAxis`** | `core/backup/import.go:446` | ось ЦЕЛИКОМ: корневые правила и правила приехавших узлов одним проходом (NODE_SECTIONS.md §5, SPEC 126 L2); с §25 номера файла сохраняются |
| `importDNS(s, *decodedDNS)` / `importWarp` | `:565`, `:620` | правил слияния не меняли; тип аргумента теперь промежуточный |
| **`mergeSources(s, items, rootTags, warns, cnt)`** | `core/backup/merge.go:467` | ОДИН проход в порядке файла; индексы идентичности (`byURL`, `rootBodies`, `folderAt`, `existingChains`) строятся раз и поддерживаются по ходу |
| `mergeSubscriptionItem` | `core/backup/merge.go:533` | по `url` байт-в-байт |
| `mergeServerItem` | `:570` | по ТЕЛУ; член папки уходит в `addFolderMember` |
| **`mergeFolderItem`** | `:610` | **новый** (форма 1.0): папка по ИМЕНИ, её настройки едут только у новой; `FileFolderID → локальный ID` пишется в карту |
| `ensureFolderAt` / `addFolderMember` | `:637`, `:658` | папка по имени у 0.x; дедуп члена по телу В ПРЕДЕЛАХ папки |
| `mergeChainItem` | `:688` | по тегу; безымянная цепочка не применяется (`:692-699`) |
| `applyImportedSections(node, sec, present)` | `core/backup/merge.go:133` | третий аргумент — **было ли поле**: пустой набор файла ЗАМЕЩАЕТ, отсутствие поля — нет (§9 п. 2) |

**`nodeAddr` вместо указателей** (`core/backup/merge.go:362-383`): слияние
дописывает в `s.Sources`, срез при росте переезжает в памяти, и указатель,
взятый до `append`, смотрел бы в освобождённый массив. Ошибка была бы тихой:
правки номеров уходили бы в никуда, и ось правил узла «иногда» оставалась бы
неперенумерованной.

**`mergedInfo`** (`:385-460`): `nodes` (что приехало — их секции идут в ось),
`folderIDs` (карта «id файла → id здесь»), `linked` (чьи ссылки переписать).
`sectionRules(s)` `:412`, `rewriteFolderLinks(s)` `:432` — ссылка, чьей папки
в файле не было, остаётся КАК ЕСТЬ, ровно как у 0.12: импорт не выдумывает
адрес, а сборка скажет о недостижимой цели сама (fail-closed).

### 13.5 Секции при импорте (W2.5)

| Сущность | Адрес | Заметка |
|---|---|---|
| **`WarnBackupSectionRecordDropped = "backup_section_record_dropped"`** | `core/backup/import.go:171` | side=import, params `["node","kind"]` |
| `decodeBackupSections(sec, nodeTag) (*state.NodeSections, []Warning)` | `core/backup/node_sections.go:35` | блок 0.12 → записи состояния + отсев |
| **`normalizeImportedSections(ns, nodeTag)`** | `core/backup/node_sections.go:57` | общий отсев для обоих входов: правила только `inline`/`srs`, DNS только `user` |
| фраза пользователю | `ui/configurator/tabs/settings_backup.go:358` | новая английская строка через `locale.T`; `ru.json` не тронут |
| запись реестра | `contract/registry/backup_warnings.json` | по формату соседей |
| sync-тест словаря | `core/backup/schema_test.go:274`, `:298` | `TestBackupWarningCodesDeclaredInRegistry` и `TestBackupWarningCodesAreActuallySet`: коды вычитываются ИЗ ИСХОДНИКА (`goBackupWarningConstants` `:346`), а не списком в тесте |

Раньше отсев делал `dropForeignKinds` (`core/state/node_sections.go:228`) и
писал только в `WarnLog` — пользователь, принёсший файл, о потере не узнавал.
Отсев в состоянии **оставлен**: он рубеж чтения `state.json`, а код — рубеж
разговора с пользователем импорта.

### 13.6 Файл: два формата (W2.6)

| Сущность | Адрес | Заметка |
|---|---|---|
| **`type File{Format, Legacy *Backup, V10 *Backup10}`** | `core/backup/file.go:79-87` | union; `ExportedByOf()` `:89` и `Counts()` `:106` — шапка и счётчики без знания формы |
| **`Parse(data) (*File, []Warning, error)`** | `core/backup/file.go:154` | формат по `lx_backup` ДО разбора тела: 1 → 0.x, 2 → 1.0, иное → отказ прежним кодом; нет ключа → прежняя ошибка |
| `ReadFile(path) (*File, …)` | `:120` | |
| `decodeTolerant` / `decodeTolerant10` / **`decodeTolerantInto`** | `:205`, `:219`, `:229` | терпимый разбор общий; таблица имён записей — параметр |
| `legacyArrayLabelKeys` / **`arrayLabelKeys10`** | `:337`, `:352` | у 1.0 секция источников одна (`sources[]`) |
| **`scanUnknown10(data)`** | `core/backup/file_keys_10.go:108` | обход файла 1.0 |
| `scanSourceBody10` | `core/backup/file_keys_10.go:142` | вложенные уровни записи источника И члена папки — ОДИН обход: форма у них одна |
| **`jsonKeys(reflect.Type)`** | `core/backup/file_keys_10.go:38` | списки ключей 1.0 РЕФЛЕКСИЕЙ по struct-тегам состояния (`:69-106`) |
| `identity10Keys` | `core/backup/file_keys_10.go:176` | **явный**, не рефлексией: объект несёт mobile-only ключи, которых в Go-структуре нет; спустись сюда рефлексивный список — `device_os` дал бы второй warning об одной потере |
| `rawObject` | `:184` | |

**Почему списки 0.x остаются написанными руками, а 1.0 — рефлексией.** У 0.x
список НОРМАТИВЕН: это таблица полей BACKUP.md §2, и вывод из Go-структур
объявил бы «схемой» текущую форму кода. У 1.0 нормативно ровно обратное: файл
ЕСТЬ сериализация состояния (П1), и список, переписанный руками, разъехался
бы с первым же новым полем — импорт ругался бы `backup_unknown_field` на СВОЙ
ЖЕ файл. Ловушка §7.26 закрыта: списки разошлись, легаси-кейсы корпуса чисты.

### 13.7 UI

| Адрес | Что стало |
|---|---|
| `ui/configurator/tabs/settings_backup.go:194` | `backupSummary(*backup.File, …)` — шапка через `ExportedByOf()`, счётчики: у 0.x прежняя строка (в `ru.json` она переведена и не трогалась), у 1.0 новая `settingsBackupSummaryCounts10Text` (`:56`) — «Sources / Rules / Variables», потому что секция источников у 1.0 одна |
| `ui/configurator/tabs/settings_backup.go:204` | `applyBackup(…, *backup.File, …)` → `backup.ImportFile` |
| `ui/configurator/tabs/settings_backup.go:358` | фраза для `backup_section_record_dropped` |

### 13.8 Тесты этапа B2

| Файл | Что сделано |
|---|---|
| `core/backup/node_sections_roundtrip_test.go` | **переписан на 1.0** (сценарии не выброшены, разнесены по форматам): `TestBackupNodeSectionsFormat10` (пишется, узловые правила в `rules[]` не уезжают, круг восстанавливает записи, файл замещает, файл без поля не трогает, экспорт — снимок), `TestBackupNodeSections012NotWrittenButNamed`, `TestBackupNodeSectionsLegacyFileStillRead`, **`TestBackupSectionForeignKindDropped`** (оба входа, три чужие записи — три кода), **`TestBackupImportRenumbersAxisWithNodeSections`** (ось одним проходом, пересечения нет), **`TestBackupEmptySectionsReplaceLocalOnes`** (пустой набор замещает, отсутствие поля — нет) |
| `core/backup/purity_test.go` | `richState10()` — состояние со всем, что выражает только 1.0 (папка с политикой и составом трёх видов, узел с секциями, цепочка с адресным хопом, identity, disabled); `TestExport10IsPureFunctionOfState`, `TestExport10FileShape`, `TestSource10CoversStateSourceKeys` (рефлексией), **`TestRoundTrip10ByteIdentical`** и **`TestImport10RewritesFolderLinksToLocalIDs`** |
| `core/backup/schema_test.go` | sync-тест словаря кодов бэкапа (см. §13.5) |
| `core/backup/file_test.go` | доступ к разобранному файлу через `.Legacy`; проверка, что файл 0.12 приезжает своим входом |
| `core/backup/*_test.go` | `Parse` → `ImportFile` там, где импортируется разобранный файл; `loadCorpusPre` возвращает `*File` |

**Про инвариант «круг байт-в-байт».** (С §25 импорт номера оси не трогает,
и первый круг тоже байт-идентичен; абзац — история.) Первый круг НЕ
байт-идентичен, и это норма, а не потеря: импорт перенумеровывает ось (§9 п. 7, NODE_SECTIONS §5), и
правило узла, стоявшее на 945 (перед якорем шаблона), при слиянии оси встаёт
в пользовательскую зону вместе с корневыми. Поэтому тест проверяет два
утверждения: (1) первый круг отличается РОВНО номерами оси и ничем больше
(сравнение файлов со стёртыми `num`), (2) со второго круга — тождество байт в
байт навсегда. Требовать тождества от первого круга значило бы требовать,
чтобы импорт номера не трогал, то есть отменить перенумерацию и вернуть
пересечение номеров, ради снятия которого она и делается.

### 13.9 Не сделано на этапе B2

W2.9 (debug API `/backup/*`), W2.8 (`docs/release_notes/upcoming.md`).
Схема контракта под форму 1.0 — волна 3, как и договорено.

---

## 14. `core/debugapi`, тесты волны и документация ПОСЛЕ волны 2, этап B3

Адреса проверены на ветке `spec127/backup-10` после B3. Гейт ВОЛНЫ
(`gofmt -l .`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`)
зелёный целиком: 38 пакетов `ok`, ни одного `FAIL`; корпус
`contract/corpus/backup` — без правки ожиданий; эталоны 0.12-писателя
(`testdata/export012_*.json`) сходятся. Эталон v6mig в прогоне волны не
запускался (§9.7: красный и до волны, не регрессия).

### 14.1 Перенос настроек через debug API (W2.9)

Паритет с кнопками «Экспорт…» / «Импорт…» вкладки «Файлы». Развилки форматов
у API нет ни на одном конце: пишет `backup.ExportFile` по
`backup.ExportFormat`, читает `backup.ImportFile` по разобранному
`backup.File` — те же две точки, что у UI.

| Сущность | Адрес | Заметка |
|---|---|---|
| **строки реестра** `/backup/formats`, `/backup/export`, `/backup/import` | `core/debugapi/backup_endpoints.go:353-358` | подключены безусловно (`core/debugapi/server.go:282`): группа не зависит от wiring, только от состояния |
| `backupFormatName012` / `backupFormatName10` | `core/debugapi/backup_endpoints.go:52-53` | имена контракта («0.12», «1.0»), а не int-маркер `lx_backup`: маркером агент опознаёт ФАЙЛ, а просит он ВЕРСИЮ |
| `backupFormatByName` / `backupFormatName` | `:58`, `:71` | пустой `?format=` → `BackupExportFormatDefault`, одна константа на UI и API |
| `handleBackupFormats` | `:106` | `{reads:[1,2], writes:["0.12","1.0"], default}` |
| **`backupExportWith(w, r, acc)`** | `:127` | тело ответа — САМ ФАЙЛ; `?envelope=1` — `{format, file_name, file, warnings}`, где `file` лежит `json.RawMessage`, а не строкой |
| `exportBackupBytes` | `:184` | пишет через `backup.ExportFile` во временный файл и отдаёт его байты: второй сериализатор здесь означал бы, что ответ API и файл с диска при одном состоянии — разные байты |
| заголовок `X-Backup-Warnings` | `:167-171` | JSON-массив кодов; без него экспорт молчал бы о потерях (П6). `Content-Disposition` — из `SuggestFileName` (`:152`) |
| **`backupImportWith(w, r, acc, rebuild)`** | `:217` | `guardStateSchema` (`:222`) → `Parse` ДО блокировки (`:236`) → `ImportFile` → `RebuildLegacyRuleView` (`:274`) → `Save` → `RebuildConfigIfDirty` |
| `knownOutboundsFor(st)` | `:314` | цели из Направлений после `MergeOutboundUpdatesInPlace` — тем же способом, что `/state/outbounds/resolved`. Узлы и цепочки сюда НЕ добавляются: их досчитывает `importKnownTags` по живому состоянию (§13.4), и второй список был бы второй правдой |
| `knownPresetIDs()` | `:337` | id пресетов шаблона; пусто = шаблон не прочитался, ссылки не режутся (как в UI) |
| зеркала машин | `core/debugapi/remote_endpoints.go:84-85`, хендлеры `backup_endpoints.go:368`, `:374` | заведены потому, что `/state/*` у машин уже проксируется ОБЩИМ механизмом (`stateAccess`); у импорта `rebuild=false` — конфиг машины собирает её визард (известное ограничение SPEC 100 §3.3) |

**Почему `RebuildLegacyRuleView` повторён у API.** Это не копия UI-кода, а тот
же шов: `Import` заменил `Rules[]` мимо диска, а загрузчик собирает
legacy-вид `CustomRules` из канона — без пересборки inline/srs-правила
терялись бы на следующей загрузке (issue #111). Забыть его в API значило бы
воспроизвести закрытый баг на втором входе.

**Свежая установка — не «цели нет».** `acc.load()` на пустом каталоге отдаёт
`state.ErrNotFound`, и импорт сливает файл в `state.New()`
(`core/debugapi/backup_endpoints.go:246-255`), а `Save` ниже создаёт файл.
Иначе самый частый сценарий переноса — «новая машина, вот файл» — отвечал бы
404, то есть восстановиться можно было бы только поверх уже настроенного
лаунчера. У экспорта поведение ОБРАТНОЕ и прежнее (404): снимать нечего, и
пустой файл был бы враньём о содержимом машины.

**Почему ошибка пересборки не отменяет импорт.** К моменту `RebuildConfigIfDirty`
состояние уже на диске. Ответить ошибкой значило бы сказать «не применилось» о
применённом, и агент повторил бы импорт — то есть слил бы файл дважды.
Пересборка называется отдельными полями `config_rebuilt` /
`config_rebuild_error` (`:284`, `:307`, `:310`).

**Версия API-спеки не поднималась.** Поверхность аддитивная, а `debugapi/v1`
по SPEC 100 §254 меняется только на ломающем изменении; поднять его здесь
значило бы сказать клиентам о разрыве, которого нет.

### 14.2 Тесты этапа B3

| Файл | Что сделано |
|---|---|
| **`core/debugapi/backup_endpoints_test.go`** | **новый**. `backupTestState()` (`:28`) — состояние со всеми сущностями 1.0. `TestBackupExportImportRoundTripOverAPI` (`:150`) — круг ЧЕРЕЗ HTTP: export 1.0 → import в ПУСТОЕ состояние другой сборки → export; первый круг сверяется со стёртыми номерами оси (`stripAPIAxisNums` `:226`), второй — байт в байт. Там же: `Save` состоялся, `RebuildConfigIfDirty` позван ровно раз у импорта и НИ разу у экспорта. `TestBackupExportDefaultFormatMatchesConstant` (`:248`), `TestBackupImportAcceptsLegacyFormat` (`:273`, плюс проверка, что экспорт 0.12 назвал потери заголовком), `TestBackupExportEnvelope` (`:310`), `TestBackupEndpointsRejectBadInput` (`:350` — 400/405 и «отказ ничего не записал»), `TestBackupImportOnFreshInstall` (импорт на пустом каталоге работает, экспорт там же — 404), `TestBackupEndpointsDocumentedInHelp` (SPEC 078) |
| `core/debugapi/server_test.go` | `fakeFacade` получил счётчик `rebuilds` и `rebuildErrV`: «позвали ли пересборку» — часть контракта ручки импорта, а не деталь |
| `core/backup/purity_test.go` | `richState10` (`:234`) дополнен до полноты W2.7 п. 1: правило **srs** с двумя наборами, правило-эффект (`action: sniff`) и правило-отказ (`action: reject` + `method: drop`), ССЫЛОЧНЫЕ (kind=preset) DNS-сервер и DNS-правило. **`assertStateEquivalent10`** (`:587`) — состояние после импорта описывает ту же настройку: id источников, ключи слияния (url/имя папки/тег), состав папки, наличие и объём секций, разрешимость `hops[].folder_id`, тела и виды правил, ОТНОСИТЕЛЬНЫЙ порядок оси, DNS/warp/Направления. `compactJSONForCompare` (`:702`), `hasSourceID` (`:717`) |

**Почему тело правила сравнивается СЖАТЫМ, а не байт-в-байт.** Декодер 1.0
кладёт в запись сырые байты файла как есть (ловушка §7.3 — порядок ключей тела
нормативен), а файл записан с отступами. Отступы умирают на первом же `Save`
(`encoding/json` сжимает `RawMessage` — проверено), поэтому расхождение по
пробелам потерей не является; расхождение по ПОРЯДКУ ключей ею было бы, и
`json.Compact` его сохраняет.

**Почему круг проверяется и в `core/backup`, и в `core/debugapi`.** В пакете
проверяется формат (П1), у API — проводка: состояние доехало до диска, конфиг
пересобран, формат не подменился по дороге. Свойство одно, рубежа два, и
падение каждого называет свою причину.

### 14.3 Документация (W2.8)

| Файл | Что стало |
|---|---|
| `docs/API.md` §«Backup / transfer (SPEC 127)» (`:161`), `docs/API.ru.md` §«Перенос настроек (SPEC 127)» (`:161`) | новый раздел «Backup / transfer (SPEC 127)» / «Перенос настроек (SPEC 127)»: три ручки, оба формата, `X-Backup-Warnings`, конверт, форма ответа импорта, коды ошибок, примеры curl |
| `docs/API.md:331`, `docs/API.ru.md:330` | в разделе удалённых машин — зеркала `/backup/*` и оговорка про `config_rebuilt:false` |
| `docs/API.md:517`, `docs/API.ru.md:515` | строка `core/debugapi/backup_endpoints.go` в таблице исходников |
| `docs/release_notes/upcoming.md` | два пункта в EN и RU: формат 1.0 (выключен по умолчанию, чекбокс, импорт читает оба, код `backup_section_record_dropped`) и ручки `/backup/*` |

### 14.4 Не сделано (и почему)

- **Схема контракта под форму 1.0** (`contract/schema/*`, `contract/docs/*`) —
  волна 3; `schema_test` по-прежнему валидирует ТОЛЬКО `Export012` против
  `backup.schema.json` (W2.7 п. 4), как и задано.
- **Кейсов корпуса на формат 1.0 не заводил**: `contract/corpus/*` — контракт,
  а ожидания там правятся руками и согласуются с LxBox (ловушка §7.23).
- **Перевод новых английских строк UI в `ru.json`** — файл запрещён к правке
  правилами волны; строки идут через `locale.T` и показываются по-английски.

## 15. Этап FIX — правки по ревью волны 2

Шестнадцать находок ревью; по сути — семь дефектов (часть находок описывала
один и тот же дефект с разных сторон). Все воспроизведены запуском ДО правки,
и каждая правка проверена откатом: без неё соответствующий тест краснеет.

Общее у всех семи: они жили ИСКЛЮЧИТЕЛЬНО в пути 1.0 — писатель 0.12 и его
эталоны, корпус 0.12 и правила слияния §9 не тронуты ни на строку.

### 15.1 Имя группы свёртки едет явно (находки 1, 2, 3, 7, 11)

Корень один: объект `fold` — форма контракта 0.11, и поля тега в ней нет
(в 0.11 имя было позиционным деривативом, D-081, и формула определена для
секции `subscriptions[]`). В модели v8 тег замены ЯВНЫЙ — пользователь правит
его руками (`ui/configurator/tabs/source_replace_tab.go:145`), и на это имя
метят правила. Пока 1.0 везла только режим, приёмник выводил имя формулой:

- явное имя подменялось молча («DE-group» → «1:select»);
- у ПАПКИ дериватив брался из счётчика подписок, поэтому папка и первая
  свёрнутая подписка получали ОДИН тег — два владельца одного outbound'а;
- `route.final` и правила того же файла метили в имя, которого на приёмнике
  уже нет (`backup_final_dropped`, правило выключено);
- предупреждения не было вовсе: `replaceTagSurvivesExport` вызывался только в
  ветке `src.Kind == SourceKindSubscription`, папку экспорт проходил молча —
  при том что 0.12-писатель ту же потерю называл (`backup_local_only_dropped`).

| Что | Адрес |
|---|---|
| `Source10.FoldTag` (`json:"fold_tag,omitempty"`) — имя РЯДОМ с `fold`, форму контракта 0.11 волна 2 не трогает | `core/backup/backup10.go:147` |
| `foldTagOf` + запись имени экспортом | `core/backup/export10.go:154`, `:135` |
| `foldTag10` — явное имя главное, дериватив остался запасным ходом для чужого файла, где есть только `fold` | `core/backup/import10.go:257` |
| `WarnBackupReplaceTagDerived` в 1.0 больше не эмитится: он говорил о потере, которой в этом формате нет (у 0.12-писателя остался) | `core/backup/export10.go:63` |

### 15.2 Проверка целей правила в 1.0 была мертва (находка 10)

`DecodeBody` возвращает УКАЗАТЕЛИ (`*InlineBody`/`*SrsBody`,
`core/state/rule_types.go:383`), а `ruleTarget10` матчил значения — switch
всегда падал в `default` и отдавал «цели нет» ДЛЯ ЛЮБОГО правила. Проверка
§9 п. 7 в пути 1.0 не работала: правило с несуществующей целью приезжало
включённым и роняло `config.json` целиком.

Правка — `core/backup/import10.go:330`/`:332` (ветви по указателям).

### 15.3 Настройки совпавшей записи берутся из файла (находки 4, 8, 14, 15)

Форма 1.0 везёт настройки контейнера целиком (§6.0: «настройки папки едут»,
`relays_in_directions` у подписки), но слияние применяло их только к НОВОЙ
записи. Главный сценарий переноса — на приёмнике папка/подписка с таким
именем уже есть — терял их молча: ни применения, ни предупреждения, а обратный
экспорт давал другой файл.

Разница здесь между ВХОДАМИ, а не между записями, и потому выражена флагом:
у 0.12 этих полей нет вовсе, и применить их «ноль» значило бы стирать
локальные настройки импортом старого файла.

| Что | Адрес |
|---|---|
| `decodedSource.FullSettings` — несёт ли формат полный набор настроек источника | `core/backup/decoded.go:111` (ставится в `import10.go:150/166/169/171`) |
| `applySubscriptionSettings(dst, src, fullSettings)` — `relays_in_directions` под флагом | `core/backup/merge.go:87`, присваивание `:102`, вызов `:610` |
| `applyFolderSettings` — enabled, tag_policy, fold, detour совпавшей папки | `core/backup/merge.go:63`, вызов `:690` |
| Счётчик `UpdatedFolders` (состояние → API) | `core/backup/merge.go:47`, `core/backup/import.go:221`/`:355`, `core/debugapi/backup_endpoints.go:305` |

Замещение ЦЕЛИКОМ, а не по непустым: снятая на другой машине политика тегов
обязана доехать снятой, иначе «слить» значило бы «только добавить», и
настройку нельзя было бы отменить переносом.

### 15.4 `dns.default_domain_resolver` не переживал круг (находки 5, 6, 12)

Писатель клал ключ в файл (`export10.go:188`), а у `decodedDNS` такого поля не
было — читатель ронял его молча, и повторный экспорт давал ДРУГОЙ файл.
Асимметрия внутри тройки скаляров, которые везде обрабатываются одинаково.

Правка — три слоя: `core/backup/decoded.go:138`, `core/backup/import10.go:356`,
`core/backup/import.go:614`.

### 15.5 Ссылочные члены папки задваивались на повторном импорте (находка 13)

`nodeBodyKey` — ключ по ТЕЛУ, а у цепочки и провайдерской группы тела нет
вовсе (состав живёт в `hops`/`group`), и адресуются они ТЕГОМ — в корне
слияние их так и ключует. Внутри папки ключ выходил пустым, «сравнивать
нечем» означало «не дубль», и каждый следующий импорт одного файла дописывал
копию (`auto-eu`, `auto-eu-2`, `auto-eu-3`…): лишняя urltest-группа в конфиге
и безграничный рост состояния.

Правка — `folderMemberKey` (`core/backup/merge.go:199`), ключ по ВИДУ узла;
`folderNodeWithBody` зовёт её (`:177`, `:182`).

### 15.6 Секции у узла, которому они не положены (находка 16)

Отсев по ВИДУ ЗАПИСИ делал импорт и называл потерю кодом, а отсев по ВИДУ
УЗЛА — состояние (`NormalizeNodeSections`), и молча. Реестр
`backup_warnings.json:107` обещает код именно на этот случай.

Правка — `sectionsAllowedFor` / `dropSectionsForForeignNode`
(`core/backup/node_sections.go:52`, `:66`), вызовы в `import10.go:138` (узел)
и `:158` (член папки). Во входе 0.x случай недостижим: там `sections` живут
только на `servers[]`, а это всегда `SourceKindServer`.

### 15.7 Mobile-only ключи identity в 1.0 (находка 9)

Вход 1.0 копировал объект `identity` целиком, то есть складывал в состояние
`device_os`/`ver_os`/`device_model`, которых лаунчер не применяет, — и БЕЗ
предупреждения; тот же файл входом 0.12 давал другое состояние и warning.
Контракт исключений по форматам не знает: BACKUP.md §«subscriptions[].identity»
— ключи, которых сторона не применяет, отбрасываются с
`backup_source_identity_dropped`.

Правка — `importIdentity10` (`core/backup/import10.go:200`): объект состояния
собирается из применяемой четвёрки заново, остальное называется вслух — ровно
как в `importSourceIdentity` входа 0.x. Заодно поправлены два комментария,
утверждавших обратное: `core/state/subscription_identity.go` (док `SetIdentity`)
и `core/backup/file_keys_10.go:173`.

### 15.8 Тесты (сценарии расширены, новых файлов нет)

| Тест | Что ловит |
|---|---|
| `purity_test.go` — `richState10` дополнен свёрткой ПАПКИ с явным тегом, `relays_in_directions` и `DNS.DefaultDomainResolver` | 15.1, 15.3, 15.4 через сам инвариант круга |
| `assertStateEquivalent10` — сверяет тег и режим свёртки, настройки папки, `relays_in_directions`, третий скаляр DNS | там же, на уровне СОСТОЯНИЯ |
| `merge_test.go` — `TestMergeSubscriptionTakesFullSettingsFromFormat10`, `TestMergeFolderSettingsComeFromFormat10File`, `TestMergeFolderMembersIdempotentForRefKinds`; в `TestMergeSubscriptionKeepsLocalIdentityAndHistory` добавлено «вход 0.12 поля не трогает» | 15.3, 15.5 и разница входов |
| `backup_test.go` — `TestImport10UnknownOutboundDisablesRule` (inline и srs) | 15.2 |
| `identity_test.go` — `TestIdentityUnappliedKeysWarnOnceFormat10` + `assertMobileOnlyIdentityNotStored` (проверяется СОСТОЯНИЕ, не только warning) | 15.7 |
| `node_sections_roundtrip_test.go` — `TestBackupSectionOnForeignNodeKindNamed` (chain, subscription) | 15.6 |

Каждый проверен мутацией: откат правки красит именно свой тест.

**Замеченное по ходу, шире находок:** мутация 15.1 показала, что дериватив
подменял тег и у ПОДПИСКИ, как только у неё задан префикс тегов
(`1:select` → `[A]select`); инвариант круга этого не ловил, потому что
`richState10` свёрнутой папки не имел, а у подписки тег совпадал с формулой.

### 15.9 Прочее

- `core/debugapi/backup_endpoints.go:305` — `updated_folders` в ответе импорта;
  пример блока `applied` в `docs/API.md:180` и `docs/API.ru.md:180`.
- `contract/registry/backup_warnings.json` НЕ правился этим этапом: запись
  `backup_section_record_dropped` (заведена B2) уже описывала случай 15.6
  дословно — расходился с ней код, а не реестр.
- Из рабочей копии удалены пять файлов-пробников ревьюеров
  (`core/backup/zz_*.go`, untracked): один из них (`zz_sk3c_test.go`) ломал
  сборку пакета `core/backup`, из-за чего `go test` отдавал закэшированный
  результат вместо прогона.

## 16. Правки по реальным данным (после волны 2)

Два дефекта слияния, найденные не ревью, а импортом ЖИВОГО состояния
владельца. Оба воспроизведены запуском до правки и проверены откатом: без
правки соответствующий тест краснеет. Общая черта — ключ идентичности,
выведенный из формы, которую живые данные не подтверждают.

### 16.1 `importDNS` схлопывал preset-серверы (ключ kind+tag)

`state.DNSServer` у `kind: preset` **не имеет тега вовсе**: идентичность там
— `ref` формы `"<preset_id>:<local_tag>"` (`core/state/dns_options.go:76`,
комментарии полей `Tag`/`Ref`). Ключ слияния был `kind + "\x00" + tag`,
поэтому ВСЕ preset-серверы состояния давали один ключ `preset\x00`, и после
первого остальные отбрасывались как «своё сильнее» — молча, без warning'а
(пропуск по §9 п. 5 его и не даёт).

Симптом на живых данных: 17 DNS-серверов, импорт в ПУСТОЕ состояние (оба
входа, 1.0 и 0.12) оставлял 15 — пропадали `russian:yandex_doh` и
`russian:yandex_dot`, оставался `russian:yandex_udp`.

Правка — `core/backup/import.go:584`: ключ стал единым для всех видов,
`kind + "\x00" + tag + "\x00" + ref`. Разбирать по `kind` нечего (у
template/user заполнен `tag` и пуст `ref`, у preset наоборот), а второй ключ
на ту же запись означал бы две несогласуемые модели merge в одном месте.
Норму §9 п. 5 это не меняет: «серверы по `kind`+`tag`» для preset читается как
`kind`+`ref` — это единственная форма, в которой у записи вообще есть имя.

Проверено грепом (`serverKey`, `"\x00"` по `core/`), что этот ключ нигде
больше не строится: `import.go:584` — единственное место; `merge.go:211`,
`:263`… — другие ключи (члены папки и тела узлов), `file.go:395` — списки
имён ключей JSON, к слиянию отношения не имеет.

### 16.2 Папки-тёзки: вход 1.0 матчит сперва по `id`

Норма §9 п. 3 — «папка по имени, одно имя = одна папка» — писалась под файл
0.12, где у папки нет собственной записи и **нет id**: она собирается из поля
`folder` записей `servers[]`. В форме 1.0 у папки есть `id` (ULID
`state.Source.ID`, §6.0), а UI допускает ДВЕ папки с одним именем — на живом
состоянии владельца их две, «Folder 1» с 4 и 6 узлами.

Карта `folderAt` («имя → индекс») из двух тёзок видела только первую. Импорт
собственного экспорта 1.0 в то же состояние матчил ВТОРУЮ папку файла в
ПЕРВУЮ локальную и дописывал туда её состав (дедуп по телу не срабатывал —
тела разные): состояние росло на каждом импорте, идемпотентность круга
ломалась.

Правка — тип `folderIndex` (`core/backup/merge.go:529`, `newFolderIndex():538`,
`lookup():588`): обе карты сразу, порядок поиска «сперва `id`, затем имя».
Вызовы: индекс строится в `mergeSources` (`:633`, `:643`), `mergeFolderItem`
(`:765`) ищет по `lookup(item.FileFolderID, item.Src.Name)`, `ensureFolderAt`
(`:803`) — вход 0.x — зовёт `lookup("", name)`, то есть остаётся ровно на
имени; `mergeServerItem` (`:713`) принимает индекс типом.

Нормы §9 это не меняет, а уточняет для формата, у которого id есть:
совпадение по `id` = «та же самая папка, файл с этой машины» (держит
локальный id, как и совпадение по имени); совпадение по имени = чужая машина
— по-прежнему §9 п. 3. Вход 0.x не затронут: `FileFolderID` там пуст всегда.

**Хвост того же дефекта: импорт файла-близнеца в ПУСТОЕ состояние.** Первой
правки хватило на слияние в само себя, но не на пустое состояние: первая
«Folder 1» (id A) заводилась из файла, вторая (id B) по id не находилась, по
имени попадала в ТОЛЬКО ЧТО ЗАВЕДЁННУЮ A — 18 источников превращались в 17,
состав первой папки = обе. Это потеря структуры САМОГО ФАЙЛА, а не «одно имя
= одна папка»: в файле папок две. Правило: по имени матчатся только папки,
существовавшие в состоянии ДО этого импорта (или уже сопоставленные по id).

Реализация — три разные точки пополнения индекса вместо одной
(`folderIndex.put():563` — общее тело, флаг `fresh`):

| Метод | Кто зовёт | Находится по имени |
|---|---|---|
| `addExisting` (`:544`) | `mergeSources:643` — папки состояния ДО импорта | да |
| `addCreated` (`:551`) | `mergeFolderItem:773` — запись `sources[]` формата 1.0 | **нет** (только по своему id) |
| `addCreated0x` (`:559`) | `ensureFolderAt:808` — вход 0.x | да |

Третий метод — не косметика, а норма входа 0.x: там папка собирается из
плоского `servers[]` по полю `folder: "имя"`, и ИМЯ — единственный ключ,
которым записи файла связаны между собой. Пометь её `fresh` — и каждая
следующая запись с тем же `folder` заводила бы ещё одну папку (мутация
красит `TestBackupCorpus/folders_roundtrip` и `TestFolderRoundTrip`).
Тёзок внутри одного файла 0.x при этом не бывает по построению формата.

**Почему то же не нужно подпискам и корневым серверам.** У подписки ключ —
`url` байт в байт (§9 п. 1), и это НАСТОЯЩАЯ идентичность записи, а не имя:
две подписки на один адрес лаунчер не создаёт (сам `mergeSources:535` это
называет состоянием «которое лаунчер сам не создаёт»), и в живом состоянии
владельца их нет. У корневого сервера ключ — ТЕЛО (§9 п. 2), и два узла с
одним телом — законная раскладка, которую §9 намеренно схлопывает
(«пропуск без warning: у тебя уже есть»). Сделать `id` сильнее в этих двух
местах значило бы менять норму §9 пп. 1–2, а не уточнять её: там ключ выбран
осознанно и от id независим.

### 16.3 Тесты (сценарии в `merge_test.go`, новых файлов нет)

| Тест | Что ловит |
|---|---|
| `TestMergeDNSPresetServersSurviveByRef` (`:711`) | 16.1: три preset одного пресета + template/user-тёзки переживают импорт в пустое и ПОВТОРНЫЙ импорт без дублей |
| `TestMergeDNSPresetServerNotOverwrittenByFile` (`:753`) | 16.1 с другой стороны: новый ключ не превратил слияние в добавление — совпавший по `ref` preset остаётся ЛОКАЛЬНЫМ (`enabled:false` не переписан) |
| `TestImport10TwinFoldersMatchByID` (`:837`) | 16.2 обеими сторонами: состояние с двумя «Folder 1» → `Export10` → `Parse` → (а) `ImportFile` в то же состояние дважды → раскладка не изменилась; (б) `ImportFile` в `state.New()` дважды → две папки с ТЕМИ ЖЕ id и составом |
| `TestImport10FolderFromOtherMachineMatchesByName` (`:874`) | 16.2 не переехала норму: файл с папкой «X» (id `01FILEA`) в состояние с «X» (id `01LOCALB`) сливается ПО ИМЕНИ, id локальный |

Хелперы `twinFolderState` (`:782`) и `folderLayout` (`:812`) — раскладка
«id/имя=теги» в порядке источников.

Мутация: возврат ключа к `kind+tag` красит оба DNS-теста; возврат
`lookup("", …)` в `mergeFolderItem` красит `TestImport10TwinFoldersMatchByID`
на круге в само себя; снятие проверки `freshByName` в `lookup` красит его же
на импорте в пустое (`[01FOLDERA/Folder 1=a1,a2,b1,b2,b3]` вместо двух папок);
подмена `addCreated0x` на `addCreated` красит корпус 0.12. Корпус
`contract/corpus/backup`, эталоны `core/backup/testdata/export012_*.json` и
писатель 0.12 не тронуты.

---

## 17. Контракт ПОСЛЕ волны 3, этап C1 (схема и документы 1.0)

Ветка `spec127/contract-10`. Кода приложения этап не трогал: изменены только
`contract/**` и `core/backup/schema_test.go`. Гейт —
`go test -count=1 ./core/backup/ -run 'Schema|Corpus'`.

### 17.1 Две схемы вместо одной

| Файл | Строк | Что это |
|---|---|---|
| `contract/schema/backup-0.12.schema.json` | 638 | **копия** прежней `backup.schema.json` байт в байт, кроме `$id` (`:3`), `title` (`:4`) и `description` (`:5`). Проверено `diff` — других строк не изменено. Поле `servers[].sections` в ней ОСТАЛОСЬ: оно там было и до кампании, а копия не правится (BACKUP.md §11) |
| `contract/schema/backup.schema.json` | 929 | формат 1.0: `lx_backup` `const: 2`, `sources[]` вместо четырёх секций |

**Разбиение 1.0 по `$defs`** (адрес — строка объявления):

| `$def` | `:строка` | Заметка |
|---|---|---|
| `source` | `:126` | `sources.items`; union по `kind` через `allOf` + три `if/then` (`:143-193`), чтобы ошибка валидации называла ВИД записи |
| `sourceServer` | `:195` | общая часть узла + `id`; обслуживает `kind` ∈ `server\|chain\|auto` |
| `sourceFolder` | `:260` | `id`, `name`, `tag_policy`, `detour`, `nodes[]`, `fold`, `fold_tag` |
| `sourceSubscription` | `:305` | `url` (`required`), `identity`, `detour`, `disabled`, `skip`, `update`, `fold`+`fold_tag` |
| `node` | `:391` | член папки; те же поля, что у `sourceServer` минус `id` |
| `nodeLink` | `:452` | `{folder_id?, tag}` — форма `detour` и элемента `hops[]` |
| `origin` | `:468` | `{kind, raw, sub_url?}`, `kind` enum `uri\|wg_ini\|json` |
| `autoGroup` | `:493` | `group_type`, `default`, `members[]`, `strategy` → `$ref directionAuto` |
| `tagPolicy` | `:523` | `{prefix, postfix}`; `mask` формы 0.12 снят |
| `fold` | `:536` | форма контракта 0.11 (`mode`, `auto`) — тега В НЁМ НЕТ, он рядом ключом `fold_tag` |
| `identity` | `:556` | взят из 0.12 (`properties.subscriptions.items.properties.identity`) без изменений |
| `sections` | `:591` | **единственное место 1.0 с `additionalProperties: false`** (`:593`), как и вложенный `dns` (`:605`); записи внутри — `$ref` на корневые `rule`/`dnsServer`/`dnsRule` |
| `rule` | `:625` | `kind` enum `inline\|srs\|preset` (вид `json` снят), `body`, `refs[]`, `num` integer, `dns`/`resolve` — поля LxBox |
| `dns` | `:692` | `strategy`, `final`, `default_domain_resolver`, `servers[]`, `rules[]` |
| `dnsServer` | `:723` | `kind` enum `template\|preset\|user`, `tag`, `ref`, `body` |
| `dnsRule` | `:758` | `kind` enum `preset\|user`, `id`, `name`, `ref`, `body` |
| `direction` / `directionAuto` | `:795` / `:867` | перенесены из 0.12 БЕЗ ИЗМЕНЕНИЙ (их сверяет `TestBackupDirectionDefMirrorsCanon`) |

Не тронуты: `direction.schema.json`, `source_fold.schema.json`,
`source_chain.schema.json`, `node.schema.json`, `registry/*`.
`contract/VERSION` остался `0.12.11`.

### 17.2 Находка этапа: `detour` у подписки

Схема, написанная по §6.0 ТЗ, не объявляла `detour` у `sourceSubscription` —
и `TestExport10EntityKeysAreDeclared` это поймал на первом же прогоне.
Настройка реальная и применяемая: это ОБЩИЙ detour контейнера (у папки он же
одноимённым ключом), в 0.12 он ехал плоской четвёркой `detour_*` записи
подписки, а слияние применяет его обеими сторонами (`merge.go:67`, `:100`).
Добавлено: `$defs/sourceSubscription.properties.detour` (`:320`) и строка в
таблице подписки `BACKUP.md` `:132`. Проверено сверкой ВСЕХ json-тегов
(`Source10`, `state.Node`, `state.Rule`, `state.DNSServer/DNSRule/DNSOptions`,
`NodeLink`, `Origin`, `AutoGroup`, `TagPolicy`, `NodeSections`,
`SubscriptionIdentity`, `Fold`, `Backup10`) против `properties` схемы —
расхождений больше нет ни в одну сторону.

### 17.3 `core/backup/schema_test.go` — два писателя, две схемы

| Адрес | Что |
|---|---|
| `schemaPath012()` `:35`, `schemaPath10()` `:39` | адреса схем функциями, а не литералами: перепутать их значит проверять писателя чужой схемой и не заметить |
| прежние 0.12-тесты (`exportSample` `:45`, четыре `Test*` `:65`–`:185`) | смотрят в `backup-0.12.schema.json`; тела не менялись |
| `export10Sample` `:204`, `readSchema10` `:213` | образец = `fixedExport10(richState10())` — то же полное состояние, что в круге волны 2 |
| `TestExport10RootKeysAreDeclared` `:233` | корень 1.0: обязательные ключи, `lx_backup == 2`, ни одного ключа мимо `properties`; отдельно — что схема НЕ объявляет плоских секций 0.12 |
| `TestExport10EntityKeysAreDeclared` `:276` | записи против `$defs` по `kind` (как `if/then` в схеме) + проверка, что образец покрывает все четыре вида: иначе тест зелен от отсутствия вида, а не от его описанности |
| `TestExport10SectionsUseRootRecordForm` `:374` | секции — в КОРНЕВОЙ форме: ни `match`/`outbound` у правила, ни `name`/`value` у DNS |
| `TestBackupDirectionDefMirrorsCanon` `:440` | переведён на `schemaPath10()`: копия канона Направления живёт теперь в схеме 1.0 |

Внешний валидатор JSON Schema в зависимости не тянется (как и было): движок
`jsonschema` 2020-12 применялся разово при разработке для сверки образца и
негативных мутаций (`lx_backup: 1`, подписка без `url`, `kind: json`, `num`
строкой, лишний ключ в `sections`, узел папки с `kind: folder` — все
отвергнуты; лишний ключ у правила и в корне — приняты, как и задумано).

### 17.4 Документы

| Файл | Что изменено |
|---|---|
| `contract/docs/BACKUP.md` | заголовок → «контракт 1.0.0»; §1 переписан (сериализация состояния буквально, перечень тонкого слоя, окно совместимости); §2 — все таблицы под 1.0 (корень, общая часть узла, папка, подписка, `fold`/`fold_tag`, `identity`, `sections`, `directions[]`, `rules[]`, `dns`, `vars`/`route`/`warp`); §3 первая строка — цель внутри `body`; §4 — `hops[]` как `nodeLink`; §6 переписан под объект `detour` и переписку `folder_id`; §8 — два мажора и отказ на больший; §9 — единое слияние для обоих входов, `FullSettings`, папки по `id`→имени, preset-DNS по `ref`, перенумерация общей оси, секции; §10 озаглавлен «исторически, внутри 0.x» и таблица цены канонизации отнесена К 0.12-писателю (+ строка `sections`); **новый §11** «Что изменилось против 0.12» |
| `contract/docs/BACKUP_PRINCIPLES.md` | П4 — блок «Уточнение 1.0.0: окно совместимости» (legacy-вход разрешён, legacy-запись = временная константа, окно закрывается по факту, мажоров живых ровно два); таблица статуса зеркала — колонка «Формат 1.0» |
| `contract/docs/NODE_SECTIONS.md` | «черновик» снят, статус — норма; §1 — форма ONE_NAMESPACE §2 + блок отбраковки B3 (`reason`, допустимая строгость LxBox); §2 — норма B4 и допустимая строгость при вводе; §3 — блок висячего `endpoint` (4 пункта); §4 — секции только в 1.0, `backup_local_only_dropped`, E1 (info без кода); §5 — объединённая перенумерация, `backup_section_record_dropped`; §6 — имя каталога и жизненный цикл каталога Tailscale (GC по ВСЕМ хранимым узлам); §7 — A9 (приглушение), тап открывает узел, таблица адресов LxBox; §8 — имена кейсов корпуса и правила конверта |
| `contract/docs/ONE_NAMESPACE.md` | заголовок → «норма контракта 1.0»; статус с хэшами волн (`bfd5fe15`/`4de1e270`, `768ef591`/`9fc880e1`) и фактом байт-в-байт идентичного `config.json`; §4 — пункты 1–4 отмечены ✅, 5–6 ⏳ |
| `contract/docs/IDENTITY.md` | §2.1 — врезка: имена `detour_node_*` это формат 0.12, в 1.0 та же ссылка зовётся `detour{folder_id?, tag}`; смысл раздела не менялся |
| `contract/README.md` | строка `1.0.0` с пометкой «черновик; `contract/VERSION` НЕ поднят» |

### 17.5 Что этап НЕ делал

W3.7/W3.7а (кейсы корпуса 1.0 и раннер), W3.8 (кейсы `body/singbox`),
W3.9 в части реестра (`backup_section_record_dropped` уже заведён волной 2 —
`registry/backup_warnings.json:107`; `reason` в его `params` этап C1 не
добавлял — это код лаунчера, задача C2, **сделано этапом C2, §18.4**),
W3.10 (`TASKS_LXBOX ## 16`), W3.11 (D-109 и статусы SPEC 126/127),
W3.12 (полный прогон).


## 18. Контракт ПОСЛЕ волны 3, этап C2 (корпус 1.0)

### 18.1 Кейсы корпуса бэкапа — формат 1.0

Все четыре с `lx_backup: 2`, ожидания написаны руками (флага `-update` в
`core/backup` нет — ловушка 23), данные синтетические по README корпуса.

| Кейс | Что проверяет | Приёмы |
|---|---|---|
| `v10_sources_union` | все четыре вида `sources[]` + слияние | есть `.pre.backup.json`: подписка совпадает по `url` (настройки из файла — `FullSettings`), папка — по `id` (локальный член `local-fr` остаётся, новые встают в конец), `hops[].folder_id` переписан на локальный id; `fold_tag` → `Replace.Tag` |
| `v10_node_sections` | ось одна на корневые и узловые правила | узловое правило встаёт МЕЖДУ корневыми (1000 → `@{self} network` 1001 → 1002/1003); три отбраковки одним кодом: `preset` у правила, `template` у DNS-сервера, `rule_set` в теле |
| `v10_rules_body_action` | цель внутри `body` | `action: reject` → `reject`, `+ method: drop` → `drop`, тег → тег; `srs` с двумя `refs` по порядку; `preset` с `vars`; `enabled: false` переживает импорт |
| `v10_dns_body` | DNS-записи всех видов | `user` с телом и тегом, `template` с тегом, `preset` по `ref` (выключенный), `user`-правило с `server` внутри тела, `preset`-правило |

Ожидания `v10_*` и старые 0.12-кейсы читает ОДИН раннер: формат он не
выбирает, его опознаёт `Parse` по `lx_backup`. Ни одно старое ожидание не
правилось (`git diff --stat contract/corpus` → только новый текст README).

### 18.2 `core/backup/corpus_test.go` — новые проверки

| Адрес | Что |
|---|---|
| `corpus_test.go:38` | `corpusRuleExpectation` — вынесен из inline-структуры; новое поле `outbound` (вид цели) |
| `corpus_test.go:487` | `ruleOutboundView` — обратная операция к `outboundutil.ApplyOutboundToRule`: тело → `reject` \| `drop` \| тег |
| `corpus_test.go:425` | `checkRules` — теперь идёт по ВСЕЙ оси: корневые правила + правила секций узлов (корневых и членов папок). Половина оси не показала бы расхождение «правило узла уехало в конец» |
| `corpus_test.go:509` | `checkDNS` — вид записи (`kind`) и её ссылка (`tag`/`ref`), число правил; тело не сверяется (оно едет байт в байт) |
| `corpus_test.go:539` | `checkSections` — связка по ТЕГУ корневого сервера: имена и `enabled` правил, теги DNS-серверов, число DNS-правил. Номер узлового правила НЕ сверяется — он зависит от числа корневых, положение проверяет `checkRules` |
| `corpus_test.go:309` | `corpusFormatAhead` — `lx_backup` больше читаемого → `t.Skip`, а не падение (правило README корпуса) |

### 18.3 Конверт тел с секциями (`core/config`)

| Адрес | Что |
|---|---|
| `contract_canon_test.go:75` | **ловушка снята**: было `node.Scheme == "wireguard"`, стало `IsEndpointScheme(node.Scheme)`. Вторая копия таблицы схем-endpoint'ов уводила tailscale в `outbounds[]`, и канон получал от outbound-эмиттера пустые `server`/`server_port` |
| `contract_canon_test.go:44` | поле `Sections json.RawMessage` в `contractNode` |
| `contract_canon_test.go:141` | `canonNodeSections` — снимает `id` и `num` у всех записей секции (§8.6: это метаданные оси приёмника, у второй стороны своя нумерация) |

Кейсы: `contract/corpus/body/singbox/whole_config_sections` (WireGuard-endpoint
+ DNS-сервер на него + DNS-правило + два route-правила, одно именованное) и
`tailscale_endpoint` (`type: tailscale` без адреса → `kind: endpoint`,
`scheme: tailscale`, в `entry` нет `state_directory`). Ожидания сгенерированы
`go test ./core/config -run TestContractCorpusBody -update` и проверены глазами.

### 18.4 Правки кода, которых требовал корпус

Два расхождения кода с нормой, написанной этапом C1; оба найдены кейсами.

| Адрес | Что было и стало |
|---|---|
| `core/config/subscription/singbox_sections_extract.go:89` | `ownServerTags` был `map[string]bool`, теги извлечённых DNS-серверов ехали в секцию КАК В КОНФИГЕ. Норма §7/§8.6 требует `@{self}-<тег из конфига>`. Стало: карта «тег в конфиге → тег в секции», сервер и ссылка `server` у его DNS-правил переписываются вместе. Иначе на чужой машине тег `home-dns` мог быть занят чужой записью, и DNS-правило узла молча ушло бы на чужой резолвер |
| `core/backup/node_sections.go:56,152` | норма B3 требует отбрасывать запись с `rule_set` ЦЕЛИКОМ; код волны 2 отсеивал только чужой `kind`, и такая запись проезжала. Добавлен `ruleBodyHasRuleSet` и ветка отбраковки |
| `core/backup/import.go:52` | `Warning.Reason` — у `backup_section_record_dropped` три причины, а код один |
| `core/backup/node_sections.go:43-47` | `SectionDropKind` / `SectionDropRuleSet` / `SectionDropNotAllowed` — экспортированы: причину читает UI |
| `ui/configurator/tabs/settings_backup.go:363` | у `reason = rule_set` своя фраза: общая («запись такого вида у секций не бывает») на правиле с `rule_set` врала бы — вид там как раз законный |
| `contract/registry/backup_warnings.json` | `params` → `["node","kind","reason"]`, `desc` описывает три причины и почему ключ не вырезается |
| `core/config/subscription/singbox_sections_extract_test.go:57,71` | ожидания SPEC 121 переведены на `@{self}-ts-dns` — они фиксировали поведение, которое норма контракта отменила |

### 18.5 Что этап НЕ делал

W3.10 (`TASKS_LXBOX ## 16`), W3.11 (D-109, статусы SPEC 126/127), W3.12
(полный прогон всего дерева). `TASKS_LXBOX.md` и `DECISIONS.md` не трогались.

## 19. Контракт ПОСЛЕ волны 3, этап C3 (реестр, задача LxBox, решение, статусы)

Этап документов и внешних адресов: кода не трогал ни строки (`git diff --stat`
по `core/`, `ui/`, `internal/` пуст), поэтому и правок кода в таблицах ниже нет.

### 19.1 Реестр — Tailscale стал общим для обеих сторон

Основание — TASKS.md §8.6 и §8.7 (сообщение lxbox-3d 14.09.2026, их `## 13`
слит: ядро `3b52ff87`, UI до `a07f0ca8`; релиз LxBox v2.23.2, тег `39f8f0df`,
ядро `1.14.0-lx.38` с Tailscale в AAR, D-103).

| Адрес | Что было и стало |
|---|---|
| `contract/registry/warnings.json:519` (`tailscale_core_unsupported`) | из `desc` снято «LxBox не применяет: libbox собран без tailscale, узлы схемы туда не доезжают», вписан факт «код общий, LxBox применяет с v2.23.2, гейт по версии ядра у него свой»; `dart` был `null` → `lib/services/builder/core_chain_capability.dart (coreVersionSupportsTailscale)` |
| `contract/registry/protocols/tailscale.json` (`note`) | снято «LxBox узел не применяет», вписано «схема применяется ОБЕИМИ сторонами с v2.23.2, узел хранится всегда и снимается тем же гейтом» |
| `contract/registry/protocols/tailscale.json` (`refs.dart`) | был пустой массив → пять адресов из §8.7: `node_spec.dart` (TailscaleSpec), `json_parsers.dart` (parseSingboxEntry, case tailscale), `server_list_build.dart` (гейт ядра, узел без `exit_node` вне Направлений), `build_config.dart` (`state_directory` при эмиссии, инъекция секций), `add_server_wizard/tailscale_bundle.dart` |

`contract/registry/backup_warnings.json` этап НЕ трогал: `backup_section_record_dropped`
с `params: ["node","kind","reason"]` и описанием трёх причин завёл этап C2.
Записи этого словаря полей `go`/`dart` не несут вовсе (в отличие от
`warnings.json`) — добавлять туда адреса некуда, форма другая.

### 19.2 `contract/TASKS_LXBOX.md` `## 16` (:831)

Номер проверен **по содержимому файла** перед записью (ловушка 15): последняя
секция была `## 15` (:788), `## 16` свободен. Структура:

| Подраздел | Адрес | Содержание |
|---|---|---|
| 16.1 Что изменилось | :840 | union `sources[]`, запись папки, ключи узла/подписки/правила/DNS, `fold_tag`, отказ от `backup_local_only_dropped` у писателя 1.0, тонкий слой исключений, форма схемы (union через `if/then`, три места с `additionalProperties:false`) |
| 16.2 Что ждём | :886 | чтение 1.0 + legacy 0.x ОДНИМ слиянием с тремя уточнениями §9 (ступень `id` у папки; preset-DNS по `ref`; перенумерация оси одним проходом; `FullSettings` у совпавшей записи), запись 1.0 после миграции хранения, перечень кейсов корпуса для прогона, правило `lx_backup` выше читаемого |
| 16.3 Кодек B3/B5 | :929 | отбраковка ЦЕЛИКОМ и почему (match-all), **явный перечень значений `reason`** — `kind` \| `rule_set` \| `not_allowed`, чтобы стороны не завели свои слова (замечание 4 этапа C2); B5 `@self`; E1 info без кода |
| 16.4 Конверт тел | :952 | напоминание правил §8.6 + причина переписи тега DNS-сервера вместе со ссылками |
| 16.5 Окно и просьба | :963 | две константы дефолта, VERSION не поднят, **просьба прислать версию/хэш релиза с чтением 1.0**, debug API как способ прогона |
| 16.6 Хэши | :986 | таблица волн + факт байт-в-байт `config.json` |
| 16.7 Вопросов нет | :998 | что согласовано на 14.09 |

Шапка файла (:1) была `контракт 0.12.7` — отставала на четыре бампа; стала
`0.12.11 (+ черновик 1.0.0, ## 16)`.

### 19.3 `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md` — D-109 (:117)

Номер проверен по содержимому: в файле 108 строк решений, последняя — D-108,
`D-109`/`D-110` не встречаются нигде. Решение помечено **ЧЕРНОВИКОМ до чтения
И записи 1.0 у LxBox**, `contract/VERSION` не поднят. Кто: Пользователь
(принцип D-106/D-107) + обе сессии (детали). Ссылки `D-109` из
`contract/README.md:85` и `ONE_NAMESPACE.md:171`, поставленные этапом C1 «на
будущее», теперь разрешаются — правки не потребовалось (замечание 3 C1 закрыто).

### 19.4 Статусы SPEC

| Файл | Что стало |
|---|---|
| `SPECS/126-F-N-TAILSCALE_LXBOX/SPEC.md:2` | шапка: «черновик до ответа LxBox» и «D-101 (черновик)» сняты — норма, `## 13` закрыт релизом v2.23.2 |
| то же, §2 | **все девять L1–L9 → `[x]`** + таблица «где закрыто» с адресами; L9 помечен перекрытым (форма секций = форма записей состояния, `ServerSections{Raw}` снесён волной 2); версия контракта: вместо 0.13.0 сразу 1.0.0 |
| `SPECS/127-F-N-ONE_NAMESPACE_V8/SPEC.md:3` | статус «N (волны 1–3 сделаны, см. §8; папка остаётся `-N` до приёмки владельцем)» |
| то же, §5 | `legacy_012_read` заменён на фактический список кейсов + объяснение, почему отдельного кейса нет (решение W3.7а); D-109 помечен записанным |
| то же, **новый §8** (:171) | таблица волн с хэшами и статусами, что ещё не закрыто и почему (VERSION, константа дефолта, окно), факт байт-в-байт, перечень четырёх дефектов, найденных данными, и хвост `backup_unknown_outbound` на корневой сервер из того же файла |

### 19.5 `docs/release_notes/upcoming.md`

По строке в EN «Technical / Internal» и RU «Техническое / Внутреннее» — перед
абзацем про debug API: контракт доведён до черновика 1.0 (две схемы, §11
BACKUP.md, снятие черновика с NODE_SECTIONS/ONE_NAMESPACE, четыре кейса
бэкапа + два кейса тела), и явно — почему `contract/VERSION` намеренно остаётся
0.12.11. Строки про сам формат бэкапа 1.0 и debug API стоят с волны 2, не
дублируются.

### 19.6 Гейт этапа (полный прогон, один раз)

```
gofmt -l .          → пусто                     (exit 0)
go build ./...      → exit 0                    (только ld: warning -lobjc)
go vet ./...        → exit 0, пусто
go test -count=1 ./... → exit 0; 38 пакетов ok, 0 FAIL
```
Ключевые: `core/backup 2.717s`, `core/state 3.891s`, `core/config 3.866s`,
`core/config/subscription 3.924s`, `core/build 4.343s`, `core/debugapi 6.175s`.
Эталон `ETALON_V6MIG=1` не запускался (красный на develop до кампании).

### 19.7 Находка этапа: `registry.schema.json` не описывает 7 файлов реестра

Go-теста, валидирующего `registry/protocols/*.json` против
`contract/schema/registry.schema.json`, в проекте нет (`registry_sync_test.go`
сверяет КОДЫ warning'ов, а не форму файлов). Разовая проверка внешним
валидатором: **7 из файлов протоколов схему не проходят** — `tailscale`
(`"uri": null` при `"uri": {type: object}`), `trojan`, `vless`, `vmess`,
`shadowsocks` (`null` там, где схема ждёт строку), `http` (`extension: null`
не в enum), `hysteria` (ключ `xray_dialect` не объявлен). Расхождение
**унаследованное**: у `tailscale.json` число и место ошибок одинаковы в `HEAD`
и после правки этапа (проверено сравнением), то есть правками кампании не
внесено. Схема либо отстала от реестра, либо должна разрешать `null` —
отдельная задача, в волну 3 не входит.

### 19.8 Что этап НЕ делал

`contract/VERSION` не тронут (0.12.11). Схемы, документы `contract/docs/*`,
корпус и код не тронуты — они закрыты этапами C1 и C2. Коммитов нет, ветка не
переключалась, `bin/` не тронут.

## 20. Контракт ПОСЛЕ волны 3, этап FIX

Этап ревью: 15 подтверждённых находок, часть — дубли одной причины
(1≡7 — `kind: auto`; 2≡5 — §6 detour; 4≡10 — третий скаляр DNS;
6≡11 — `hops[].folder_id`; 9≡14 — `reason`). Уникальных дефектов девять.

### 20.1 Код: молчаливая потеря корневой группы (находки 1, 7)

`Export10` выбрасывал источник `kind: auto` БЕЗ предупреждения
(`export10Source` → `ok=false`, вызывающий делал `continue`), тогда как
писатель 0.12 на том же виде эмитит `backup_source_kind_unsupported`
(`legacy_write_012.go:168`). Корневая группа — законная форма состояния
(`state.NewAutoSource` `core/state/sources_v7.go:368`, `normalizeSourceShape`
`:408` принимает её наравне с `server`/`chain`, вставка записи —
`ui/configurator/business/source_record_paste.go:96`), то есть терялась
достижимая пользовательская настройка — нарушение П6. Фраза UI под этот
случай была написана и не срабатывала (`settings_backup.go:401`).

Фикс — `core/backup/export10.go:71`: та же ветка предупреждения, что у 0.12
(код, `Detail`, `Kind`, `Nodes`); шапка `export10Source` (`:103`) больше не
утверждает, будто корневой группы «не бывает».

Третья сторона — схема — обещала обратное и тоже приведена к коду:
`contract/schema/backup.schema.json` убрал `auto` из `$defs/source.kind`
(корневой дискриминатор — ровно `server|chain|folder|subscription`), из
ветки `if/then` и из `$defs/sourceServer.kind` (там теперь `server|chain`).
`$defs/node.kind` не тронут — членом папки `auto` и `unsupported` законны.
Проверено внешним валидатором: файл с корневым `kind: auto` теперь
ОТВЕРГАЕТСЯ схемой, все пять фикстур 1.0 проходят.

### 20.2 Документы: норма, описывавшая несуществующее поведение (находки 2, 5)

`BACKUP.md` §6 (:463) утверждал, что ссылка на ненайденный контейнер
«снимается с предупреждением». Такого поведения нет ни у одного входа:
`rewriteFolderLinks` (`core/backup/merge.go:488`) при промахе по карте не
делает ничего, и его собственный комментарий (`:484`) обосновывает ровно
обратное решение; кода предупреждения не существует (ни одного detour-кода в
`contract/registry/backup_warnings.json`). Абзац противоречил и §4 той же
страницы («цепочка ввозится как есть», запрет на импортное «выключить на
всякий случай»), и врезке `IDENTITY.md:113`.

§6 переписан под код: ссылка ввозится как есть, рубеж — сборка
(fail-closed у `detour`, `chain_hop_missing` у хопа), с объяснением цены
ошибки (снятая ссылка = узел молча пошёл напрямую). Ложная посылка снята и в
источнике — `TASKS.md` §6.0 (:340).

### 20.3 Документы: таблицы §2 (находки 1/7, 8)

- «Общая часть узла» больше не объявляет корневым видом `auto`; добавлен
  абзац «Корневых видов узла два, внутри папки — четыре» (:96) с правилом
  писателя/читателя.
- `id` вынесен из общей таблицы узла в свою (:125) с явным «у члена папки
  его нет»: поле живёт на `state.Source`, а не на `state.Node`, и схема это
  отражала верно (`$defs/sourceServer` объявляет, `$defs/node` — нет), а
  текст — нет. Сторона, написавшая `id` члену папки по таблице, получала бы
  `backup_unknown_field`.
- §11 (:851): строка дискриминатора приведена к четырём видам.

### 20.4 Документы: третий скаляр DNS (находки 4, 10)

§9 п. 5 (:655) перечислял замещаемыми `dns.final` и `dns.strategy`, тогда как
`importDNS` (`core/backup/import.go:632`) замещает и
`dns.default_domain_resolver` — ключ, который эта же волна объявила в §2 и
§11. Перечень закрыт тройкой явно.

### 20.5 Корпус: кейсы не проверяли заявленного (находки 3, 6, 9, 11–15)

Инвариант §8.5 п. 3 («кейс проверяет то, что заявлено именем») нарушался в
семи местах; все закрыты и КАЖДЫЙ подтверждён мутацией — до правки мутация
оставляла корпус зелёным, после правки роняет.

| Что не проверялось | Правка ожидания/фикстуры | Мутация, которая теперь падает |
|---|---|---|
| тело DNS-сервера и `dns.strategy`/`final`/`default_domain_resolver` (3) | ключи `body`, `strategy`, `final`, `default_domain_resolver` у `v10_dns_body`; в фикстуру добавлен третий скаляр | `decode10DNS`: обнулить `Body` и скаляры → 4 ошибки |
| перепись `hops[].folder_id` (6, 11) | ключ `chains[].hops` (тег + ИМЯ папки); в фикстуре разведены id папки файла и предсостояния | `rewriteFolderLinks` → `return` → «папки с таким id нет» |
| ступень слияния папки по `id` (12) | вторая папка «Nordics», совпадающая по `id` при РАЗНОМ имени; ключ `folder_ids` | отключить ветку `byID` в `folderIndex.lookup` → 3 ошибки |
| `reason` у `backup_section_record_dropped` (9, 14) | ключ `warning_reasons`; в фикстуру добавлены запись с `rule_set` в теле и DNS-сервер `kind: template` | `SectionDropRuleSet` → `SectionDropKind` → «причины [kind], ожидались [kind rule_set]» |
| `vars` preset-правила (13) | ключ `vars` у правила; `vars` добавлены в фикстуру (их там не было вовсе) | обнуление `Rule.Vars` |
| секции у члена папки (15) | папка «Carriers» с носителем секций `fm-ts` | снять `Sections.DNS` у члена → «DNS-серверы связки []» |

Раннер `core/backup/corpus_test.go`: `corpusRuleExpectation.Vars`,
`DNS.{Body,Strategy,Final,DefaultDomainResolver}`, `WarningReasons`,
`FolderIDs`, `Chains[].Hops`; `checkSections` (:545) обходит теперь ВСЕХ
носителей секций, а не только корневые серверы (у члена папки свой путь
слияния, и сторож «есть секции, которых в ожиданиях нет» его не видел);
новые `checkChainHops` и `checkWarningReasons` в конце файла.

**Ни одно старое ожидание 0.12 не изменилось** — правились только файлы
`v10_*` и общий раздел `corpus/backup/README.md`.

### 20.6 Отражено в документах контракта

`NODE_SECTIONS.md` §8 — описания всех четырёх кейсов переписаны по факту (что
именно кейс различает и почему ступени разведены). `corpus/backup/README.md` —
новый раздел «Ключи, которые различают ОДИНАКОВО ВЫГЛЯДЯЩИЕ реализации» с
объяснением на каждый ключ. `TASKS_LXBOX.md` §16.1 — корневых видов четыре,
`auto`/`unsupported` только в `nodes[]`, ссылка ввозится как есть; §16.2 п. 3
— что именно проверяет каждый кейс.

### 20.7 Что этап НЕ делал

`contract/VERSION` не тронут (0.12.11). `bin/` не тронут. Коммитов нет, ветка
не переключалась. Схема 0.12 (`backup-0.12.schema.json`) не тронута — она
копия прежней и правкам не подлежит. Хвост `backup_unknown_outbound` на
корневой сервер из того же файла (замечание C2 №1) не чинился — он вне
списка находок и требует общего сборщика `knownTags` на оба входа.

## 21. Писатель 0.12 снят (решение владельца, D-110)

Ветка `spec127/backup-10-only`. Лаунчер с v1.6.0 пишет бэкап только в 1.0;
окно «два писателя, дефолт 0.12» (SPEC §4, §12.2 выше) отменено: релизы
лаунчера 1.6.0 и LxBox 2.23.3 синхронны, обе стороны пишут только 1.0 и
читают 0.x всегда. Импорт (`legacy_read_0x.go`, `import10.go`, слияние) не
тронут ни строкой логики; корпус `contract/corpus/backup` зелёный без правки
ожиданий.

### 21.1 Код

| Адрес | Что стало |
|---|---|
| `core/backup/legacy_write_012.go` | **удалён целиком** (`Export012` и весь маппер 0.12) |
| `core/backup/export.go:20`, `:35` | `ExportOptions` без `Format`; `ExportFile` всегда `Export10` + `WriteFile10`. Сняты `ExportFormat`, `ExportFormat012/10`, `BackupExportFormatDefault` и мёртвые после писателя `droppedLocalOnlyFields`, `dedupRefs` |
| `core/backup/file.go:35` | `WriteFile10` — единственная запись; `WriteFile` (тип 0.12) удалён |
| `core/backup/file.go:65-73` | **`type FileFormat`** (`FileFormatLegacy` = 1, `FileFormat10` = 2; ноль — «не разобран») — тип формата остался только у ЧТЕНИЯ: `File.Format` различает входы |
| `core/backup/convert_v7.go:192` | экспортная половина 0.12 снята (`exportNodeLinkRef`, `exportHops`, `exportChainSpec`, `replaceTagSurvivesExport`); остались `exportFold`/`exportDisabledMap` (писатель 1.0) и вся импортная половина; `legacyFoldPrefix` — у двух входов |
| `core/backup/types.go` | `boolPtr`/`f64Ptr` сняты; типы — форма 0.x для legacy-входа |
| `core/backup/import.go` | константы `WarnBackupLocalOnlyDropped`/`WarnBackupReplaceTagDerived` остались (коды в словаре), лаунчер их больше не ставит; комментарий `Warning.Reason` знает `unknown_key` чужой стороны |
| `core/debugapi/backup_endpoints.go:64` | `backupExportFormatError`: пусто/`1.0` — ок, `0.12` → 400 «format 0.12 is no longer written; import still reads it», иное → 400 «unknown format; use 1.0» |
| `core/debugapi/backup_endpoints.go:75`, `:110`, `:189` | `backupFormatName(FileFormat)` — имя формата принятого файла в ответе импорта; `/backup/formats` → `{"reads":[1,2],"writes":["1.0"],"default":"1.0"}`; `exportBackupBytes` без формата. Зеркала `/remote/machines/{id}/backup/*` идут тем же `backupExportWith` |
| `ui/configurator/tabs/settings_backup.go:98` | `handleBackupExport` пишет сразу: диалог с чекбоксом, `runBackupExport` и ключи `settingsBackupFormatNewText`/`settingsBackupFormatHintText`/«Export settings» сняты; `ru.json` не тронут |

### 21.2 Тесты

**Удалены вместе с писателем** (проверяли свойства самого писателя 0.12):
`export012_etalon_test.go` + `testdata/export012_*.json` (эталоны байтов 0.12);
в `schema_test.go` — пять проверок 0.12 против `backup-0.12.schema.json`;
`TestExportNamesUnrepresentableReplaceTag`, `TestReplaceTagDerivativeCountsSubscriptionsOnly`
(импортную половину держит кейс `replace_tag_index`),
`TestExportNamesLocalOnlySourceFields` (`convert_v7_test.go`);
`TestExportNamesFolderOwnSettings`, `TestExportPutsWholeRecordLossesFirst`
(`export_folder_loss_test.go`); `TestBackupNodeSections012NotWrittenButNamed`;
`TestExportIsPureFunctionOfState` (двойник `TestExport10IsPureFunctionOfState`).

**Переписаны, сценарий сохранён:**

| Адрес | Как |
|---|---|
| `core/backup/backup_test.go:143` | **`importBothFormats`** — одна настройка двумя входами: файл `Export10` и СЫРОЙ JSON 0.12, снятый прежним писателем с того же состояния (выгружен до удаления); проверяет, что каждый файл прочитан своим входом |
| `backup_test.go` | `TestRoundTripLossless` (`legacyMkState012` `:178`), `TestRoundTripDNSSection`, `TestRoundTripDetourNodeRef`/`TagOnlyRef` — оба входа; vars/цепочки/warp/extensions — на писатель 1.0 |
| `core/backup/convert_v7_test.go:40`, `:133` | `TestRoundTripV7ModelEquivalent` — оба входа (`legacyV7Model012`), утверждения в `assertV7ModelEquivalent`; `TestRoundTripV7ResolvesHopIntoContainer` — только сырой 0.12 (резолв строкового хопа есть только у legacy-входа) |
| `identity_test.go`, `wgini_roundtrip_test.go` | identity, папка (`servers[].folder` + выключенный член), INI в `uri` — оба входа |
| `directions_test.go`, `file_test.go`, `export_folder_loss_test.go`, `purity_test.go`, `tailscale_state_dir_test.go` | на писатель 1.0 |
| `core/backup/schema_test.go:430`, `:512` | сверка словаря: коды-«пенсионеры» `backup_local_only_dropped`/`backup_replace_tag_derived` — явным списком; упоминание кода в комментарии больше не считается «ставится» (раньше именно оно держало тест зелёным) |
| `core/backup/corpus_test.go:997`, `:1005` | `checkExtensionsDropped` — re-export 1.0; `label` цепочки в ожидании лаунчера — ошибка кейса (подписи цепочки нет ни в состоянии, ни в 1.0); `chainCanon` — канон цепочки вместо снятого `exportChainSpec` |
| `core/debugapi/backup_endpoints_test.go` | `TestBackupFormatsWriteOnly10`; `TestBackupImportAcceptsLegacyFormat` — сырой файл 0.12; `?format=0.12` → 400 с текстом |

Проверено мутацией: без поля `detour_node_source_id` в `importNodeLinkRef`
падают 0.12-половины `TestRoundTripDetourNodeRef` и
`TestRoundTripV7ModelEquivalent`.

### 21.3 Контракт и документы

`contract/VERSION` → `1.0.0`; `contract/README.md` строка 1.0.0 — норма;
`BACKUP.md` §1/§2/§8/§9 п. 7/§10/§11, `BACKUP_PRINCIPLES.md` П4 и статус
зеркала, `ONE_NAMESPACE.md` §4, `NODE_SECTIONS.md` (статус, B3 `unknown_key`,
§4), `registry/backup_warnings.json` (`backup_source_kind_unsupported` —
`side: both`; desc двух снятых кодов; `unknown_key`), `corpus/backup/README.md`
(1.0.0, раннер `backup_corpus_test.dart`, golden
`v10_sources_union.expected.lxbox.json`), `TASKS_LXBOX.md` `## 16`,
`DECISIONS.md` D-110, `docs/API*.md`, заметки 1.6.0.

## 22. Одно правило — одно тело; ссылка без `folder_id` на член папки (D-111, релиз 1.6.0)

Ветка `fix/rules-array-and-folder-links`. Две правки импорта, обе входят в
1.6.0. Слияние §9 не тронуто; писатель 1.0 не тронут.

### 22.1 Норма массива тел правила

| Адрес | Что |
|---|---|
| `core/backup/import10.go:401` | **`splitRuleBodies(r, where)`** — ОДНА функция на оба входа: объект → запись как есть; массив → по записи на элемент-объект (`name`, `name #2`… по порядку получившихся, безымянная → безымянные; `enabled` общий; `num` исходной записи у всех частей — устойчивая сортировка и сплошная перенумерация импорта ставят их подряд, N+1 столкнулся бы со следующей записью; `id` только у первой); не-объект/битый JSON/пустой массив → `backup_unknown_field` с путём `rules[<имя>].body[#k]` |
| `core/backup/import10.go:88-104` | вход 1.0: `ruleBodyPresent` (`:449`, inline\|srs с непустым не-`null` телом) → `splitRuleBodies` → `decode10Rule` на каждую часть (проверка цели по частям) |
| `core/backup/legacy_read_0x.go:448`, `:526` | `importRule` отдаёт `[]state.Rule`; `kind: json` → **`importJSONRule`**: `match` — тело inline-записи как есть, та же `splitRuleBodies`, проверка цели — `decode10Rule`; плоский `outbound` json-записи не применяется (цель в теле); без частей → `errSkipRule` |
| `core/backup/legacy_read_0x.go:546` | `legacyRuleNum` — номер оси 0.x (float64 → int), общий для inline/srs/preset и json |
| `ui/configurator/dialogs/add_rule_dialog.go:1149` | `ruleArrayPastedError` — вставлен массив → ошибка «Paste one rule at a time: the editor keeps one sing-box rule per entry» (`addRuleArrayPastedText` `:59`, перевод `bin/locale/ru.json:2047`); три места ввода: Custom JSON `:473`, сохранение вкладки JSON `:744`, переход JSON → Form `:1022` |
| `core/debugapi/state_endpoints.go:189` | PATCH `/state/rules`: массив в `body` у inline/srs отвергается `DecodeBody` (422) — правка не нужна |
| `core/state/node_sections_convert.go:167` | вкладка JSON узла: элемент `route.rules[i]` не объект → документ отвергается («expected an object») — правка не нужна |
| `contract/corpus/backup/legacy_012_json_rule_array.*` | кейс: json-объект, `Keep` выше массива по файлу и ниже его частей по `num`, массив из трёх с элементом-строкой; мутация «N+1» роняет кейс |
| `core/backup/corpus_test.go:77`, `:568` | новый необязательный ключ ожидания `rules[].body` (deep-equal) |
| `core/backup/backup_test.go:385` | `TestImport10RuleBodyArrayEqualsExpandedRecords` — массив в `body` даёт то же состояние, что развёрнутые записи |

### 22.2 Ссылка без `folder_id` на член папки

| Адрес | Что |
|---|---|
| `core/state/sources_v7.go:87` | **`(*TagPolicy).FinalTag(raw)`** — prefix + сырой тег + postfix, nil → сырой; единственный дом формулы: её же зовут `core/config/tailscale_state_dir.go:262` (`canonicalStateDirTag`) и `ui/configurator/business/tailscale_state_dir.go:65`, `:192-193` (прежние копии конкатенации) |
| `core/backup/import10.go:499` | **`normalizeMemberLinks10(s, linked, rootNames)`** — приехавшие файлом `detour` (корневых узлов, членов папок, общий у контейнеров) и `hops[]` без `folder_id`: тег не в корне (`importKnownTags` `import.go:445` + `reservedTargetLiteral` `import.go:590`) и финальный тег ровно у одного члена папки/подписки результата → `{id контейнера здесь, сырой тег}`; несколько совпадений — как есть, без предупреждения (кода у импорта нет, BACKUP §4/§6) |
| `core/backup/import.go:397-407` | развилка по `decodedFile.Format` (`decoded.go:47`): 1.0 → `normalizeMemberLinks10` после `rewriteFolderLinks` (с §23 — `rewriteLinks`); 0.x → прежний `resolveImportedHops` (строковые хопы по сырым тегам). Для 1.0 `resolveImportedHops` больше не зовётся: сырой тег мимо финального переписал бы ссылку при неоднозначном финальном теге |
| `core/backup/purity_test.go:816` | подтест `tag-only links to folder members` в `TestImport10RewritesFolderLinksToLocalIDs`: detour корня, detour члена папки, хоп цепочки → адрес папки; неоднозначный тег, корневой узел, Направление — не тронуты; сборка (`GenerateOutboundsFromParserConfig`) эмитит узел и цепочку, `NodeLinkTargets.Resolve` находит цель, та же ссылка без адреса — висит. Мутация «без нормализации» роняет подтест с fail-closed обоих |

Документы: `contract/docs/BACKUP.md` §2 («Одно правило — одно тело»), §4, §6
(«Терпимость читателя…»), §11; `ONE_NAMESPACE.md` §1; `TASKS_LXBOX.md` §16.8;
`corpus/backup/README.md`; `DECISIONS.md` D-111; `docs/release_notes/1-6-0.md`,
`CHANGELOG.md` v1.6.0.

## 23. Целостность NodeLink: сборка, импорт, операции (NODE_LINK.md §9.3 п. 2, 5–8; D-113, D-114; релиз 1.6.0)

Ветка `fix/node-link-integrity`. Хранимая форма (state v8, файл 1.0) не
менялась ни на ключ. Каждый дефект сперва воспроизведён падающим тестом на
базе `d3f441ec`, затем закрыт.

### 23.1 Сборка: цепочки папки (§9.3 п. 2)

| Адрес | Что |
|---|---|
| `core/config/configtypes/types.go:172` | **`ProxySource.Chains []BuiltChain`** (`json:"-"`) вместо одного `Chain *SourceChain`: запись на каждый узел-цепочку источника |
| `core/config/configtypes/types.go:939-945` | **`BuiltChain{Tag, Chain}`** — тег узла-цепочки (сырой = финальный) и маршрут с разрешёнными позициями |
| `core/config/canonical_emit.go:275-332` | `ResolveCanonicalChainHops` копит `built` по цепочкам источника и кладёт `ps.Chains` только если есть что класть (сборочная форма, положенная вызывающим напрямую, не затирается — на этом держатся `chain_emit_test.go` и `contract_direction_test.go`) |
| `core/config/chain_nodes.go:39-48` | **`chainNodeTag(bc, sourceIndex, chainIndex)`** — вместо `chainSourceTag`/`canonicalChainTag`; запасное `chain-<N>`, у второй безымянной цепочки источника — `chain-<N>-<k>` |
| `core/config/chain_nodes.go:174-198`, `206-278` | цикл по `src.Chains` и **`buildChainNode`** — проверки одной цепочки (поддержка ядра, `ChainEmitError`, занятое имя, позиции, reality, вложенность); подпись источника называет цепочку только у корневой записи (`:220`), у папки — тег цепочки |

### 23.2 Импорт: перепись по адресам слияния (§9.3 п. 5, 6)

| Адрес | Что |
|---|---|
| `core/backup/merge.go:441-452` | `mergedInfo`: `folderIDs` — папки И подписки; `linked []linkedNode`; `landed landings`. Мёртвый `merge()` снят |
| `core/backup/merge.go:455-462` | **`linkedNode{at, fileContainer}`** — id контейнера-владельца в файле у члена папки 1.0: по нему член группы без `folder_id` идёт за переименованием, не меняя формы (§5.1 № 8) |
| `core/backup/merge.go:473-541` | **`landings`**: `members` ((id контейнера в файле, тег в файле) → тег здесь), `roots` (корневой узел: тег в файле → тег здесь), `legacyIDs` (0.x: id записи сервера/цепочки → адрес здесь), `fileFinals` (1.0: финальный тег члена в файле → адреса здесь), `legacyTags` (0.x: сырой тег члена папки → адреса здесь), `renamed` (адреса здесь у членов, добавленных под другим тегом); методы `root`, `member` |
| `core/backup/merge.go:569-585` | **`rewriteLinks`** (было `rewriteFolderLinks`): detour, `hops[]`, группа — у всех записей из файла |
| `core/backup/merge.go:593-612` | **`fileLinkHere`** — корень: `roots`; с `folder_id`: `legacyIDs` → `folderIDs` + `members` |
| `core/backup/merge.go:620-638` | **`rewriteGroup`** — члены и `default` вместе; член без `folder_id` в контейнере — по `members` с `fileContainer` |
| `core/backup/merge.go:804-839` | `mergeSubscriptionItem`: id подписки в файле (`:809`) → карта id (`:835-838`) и у совпавшей по URL, и у новой |
| `core/backup/merge.go:850-898`, `906-914` | `mergeServerItem` + **`landedRoot`**: корневой узел, узнанный по телу или уникализированный, → `roots`, у 0.x ещё `legacyIDs`; член папки 0.x → `legacyIDs`, `legacyTags`, `member` (`:885-897`) |
| `core/backup/merge.go:935-970` | `mergeFolderItem`: член → `member(fileFolderID, тег файла, финальный тег по политике ПАПКИ ФАЙЛА, адрес здесь, added)` |
| `core/backup/merge.go:1007-1026` | `addFolderMember` возвращает `(адрес здесь, добавлен)`; принимает `fileContainer` |
| `core/backup/merge.go:1038-1067` | `mergeChainItem`: 0.x id цепочки → `{tag}` (`:1050-1052`) |
| `core/backup/import.go:387-408` | `rewriteLinks` → `normalizeMemberLinks10(s, &merged, …)` / `resolveImportedHops(…, &merged)` |
| `core/backup/import10.go:510-586`, `590-601` | `normalizeMemberLinks10`: ярус файла (члены папок файла под именами из файла + узлы подписок, приехавших файлом — **`linkedSubscriptions`**) раньше яруса результата; из яруса результата исключены `renamed` |
| `core/backup/convert_v7.go:126-138` | `importNodeLinkRef` — без изменений кода; комментарий: id сервера в `folder_id` дописывает слияние |
| `core/backup/convert_v7.go:257-351` | `resolveImportedHops(sources, directions, merged)`: у цепочек из файла — ярус файла (`legacyTags` + узлы подписок файла), `renamed` вне общего индекса |

### 23.3 Операции UI: реестр узловых ссылок и корневые имена (§9.3 п. 7, D-113, D-114)

| Адрес | Что |
|---|---|
| `ui/configurator/business/node_move.go:485-491` | **`linkAddresses(link, space, target)`** — пространство ссылки без `folder_id`: корень у detour/позиции (№ 7), свой контейнер у члена группы (№ 8) |
| `ui/configurator/business/node_move.go:494-508` | `linkEdit` (`linkKeep`/`linkReplace`/`linkDrop`), `linkEditFunc` |
| `ui/configurator/business/node_move.go:516-553` | **`editNodeLinks(m, edit)`** — ЕДИНЫЙ обход: detour, позиции, группы корня и контейнеров; `(имена задетых источников, число ссылок)` |
| `ui/configurator/business/node_move.go:557-571`, `578-601` | `editDetourLink`, `editLinkList` (без `slices`, исходный массив не портится) |
| `ui/configurator/business/node_move.go:611-637` | **`editGroupLinks`** — ЕДИНАЯ точка правки состава группы: члены и `default` вместе, для переписи и гашения (замена `repointGroupLinks` и цикла `ClearContainerNodeLinks`; с §26 `default` — NodeLink и решается тем же `edit`) |
| `ui/configurator/business/node_move.go:439-450`, `465-476` | `clearNodeLinks(from)`, `repointNodeLinks(from, to)` поверх `editNodeLinks` |
| `ui/configurator/business/node_move.go:170-176` | `rootOnlyRefsToTag` — `editRootNameRefs(..., rootRefName)` (называет, не правит; теперь и `options.default`/`preferredDefault`) |
| `ui/configurator/business/root_name_refs.go:33-77` | **новый файл**: `rootRefAction` (`Miss`/`Rename`/`Clear`/`Name`), `rootRefDecide`, `rootRenames(map)` (один проход — `x` → `x-auto` не переписывается дважды), `rootNameIs` |
| `ui/configurator/business/root_name_refs.go:86-209` | **`editRootNameRefs(model, decide)`** — ЕДИНЫЙ обход ссылок на корневое имя: NodeLink корня (через `editNodeLinks`, члены групп контейнеров не трогает), цели правил, `route.final` + `SettingsVars`, Направления, переменные пресетов, detour DNS |
| `ui/configurator/business/root_name_refs.go:153-182` | Направления: у собственной записи `addOutbounds`, `options.default`, литеральный `preferredDefault`; у ЛЮБОЙ записи — USER-патч; патч пресета не трогается |
| `ui/configurator/business/root_name_refs.go:214-320` | `editNameList`, `editOptionsDefault`, `editDefaultLiteral`, **`editUserPatch`** (`addOutbounds` []interface{}/[]string, `options.default`, `preferredDefault`) |
| `ui/configurator/business/root_name_refs.go:337-357` | `directionDefaultLiteral` — только `X`/`!X`; регулярка `/…/` ссылкой не считается (§4.3) |
| `ui/configurator/business/root_name_refs.go:367-404` | `editDNSDetours` (было `renameDNSDetour`) |
| `ui/configurator/business/root_name_refs.go:418-427` | **`RenameRootNodeRefs(m, old, new)`** — D-113 |
| `ui/configurator/business/root_name_refs.go:437-447` | **`RootNodeTagTaken(m, tag, except)`** — гард переименования верхнего узла: владельцы корня без пары `-auto` (у узла двойника нет) |
| `ui/configurator/business/root_name_refs.go:457-465` | **`ClearRootNodeRefs(m, tag)`** — D-114: NodeLink гаснут, опция уходит, одиночные цели называются |
| `ui/configurator/business/root_name_refs.go:476-494` | `rootNameOwnedElsewhere` — имя после операции носит ещё кто-то (узел, Направление/твин, свёртка/двойник) → не трогать |
| `ui/configurator/business/root_name_refs.go:508-526` | **`RenameFoldRefs(m, before, after)`** — `-auto` только при both до и после |
| `ui/configurator/business/direction_rename.go:108-131` | `RenameDirection` — тег записи + `editRootNameRefs` с парой тег/`-auto` |
| `ui/configurator/business/detour_refs.go` | `ResetDetourNodeRefs` снят (с `detour_refs_test.go`); остался `SourceDisplayName` |
| `ui/configurator/tabs/source_edit_window.go:431-439` | `mergeEditedSourceIntoModel` → `RenameFoldRefs` после записи снимка |
| `ui/configurator/tabs/source_edit_window.go:2394-2429` | Save корневого узла: гард свободного имени (`RootNodeTagTaken`, текст — существующий ключ локали), перепись, `stale.NodesRenamed` |
| `ui/configurator/tabs/source_edit_window.go:2480-2506`, `2520-2532` | **`repointRefsAfterRootNodeRename`** (вместо `resetRefsAfterNodeRename`), **`showNodeRefsClearedDialog`** (вместо `showDetourRefsResetDialog`; ключи `Links to the deleted node`, `Node %q was deleted. …` — `bin/locale/ru.json`) |
| `ui/configurator/tabs/source_tab.go:1337-1352` | удаление верхнего узла (`server`/`chain`/`auto`): `ClearRootNodeRefs` ДО `applySourceMutation` (та сбрасывает осиротевшие цели правил), диалог |
| `ui/configurator/tabs/preview_node_ops.go:498-529` | удаление узла контейнера — `showNodeRefsClearedDialog`; перенос и переименование — `showNodeRefsRepointedDialog` (прежний текст «links cleared» врал) |

### 23.4 Заливка подписки в папку (§9.3 п. 8)

| Адрес | Что |
|---|---|
| `core/state/subscription_merge.go:42` | **`SubFetchMaterial.SourceID`** — id подписки материала |
| `core/state/subscription_merge.go:371-389` | **`repointFolderDetours(folder, subURL, subID, touched)`** — detour узла этой заливки на соседа по подписке → на его копию в папке; копии нет — как есть |
| `core/state/subscription_merge.go:393-401` | `fillCopyTags` — общий с `repointFolderAutoMembers` (`:311-351`) |
| `ui/configurator/business/folder_fill_subscription.go:113` | материал несёт `SourceID` |

### 23.5 Тесты (интеграционные, по одному на слой)

| Адрес | Что |
|---|---|
| `core/config/canonical_emit_test.go:379` | `TestEmitE3_TwoChainsInOneFolderAreTwoOutbounds` — две цепочки папки с префиксом: два outbound'а, позиции финальными тегами, без деградаций |
| `core/backup/merge_test.go:935` | `TestImportLinksFollowMergeAddresses` — подтесты `1.0` (`:1000`: подписка по URL, уникализация и узнавание по телу члена и корня, группа с `default`, ссылка финальным тегом, сборка результата без предупреждений) и `0.12` (`:1064`: id корневого и папочного сервера, строковые позиции, сборка) |
| `ui/configurator/business/detour_rename_e2e_test.go:148` | `TestRootNameRefs_RenameAndDeleteFollowEveryLink` — переименование верхнего узла (все виды, USER-патч, `options.default`, сборка: обе цепочки папки), Направление и свёртка в папках, тёзка, удаление |
| `ui/configurator/business/folder_fill_subscription_test.go:53` | расширен: detour на копию релея, релей без копии, идемпотентность |
| `core/backup/backup_test.go:896`, `convert_v7_test.go:174-183`, `purity_test.go:40` | висячая форма `{folder_id: id сервера}` заменена корневой `{tag}` |

Документы: `contract/docs/NODE_LINK.md` §5.2, §6, §7.2–§7.4, §8, §9.1, §9.3,
§10; `IDENTITY.md` §2.1; `TASKS_LXBOX.md` §17.3, §17.6, §17.7;
`corpus/backup/README.md`; `SPECS/features/directions.md` §9; `DECISIONS.md`
D-113, D-114; `docs/release_notes/1-6-0.md`; `CHANGELOG.md` v1.6.0.

## 24. Экспорт Направлений телом после слияния (релиз 1.6.0)

Ветка `fix/export-merged-directions`. Дефект с живого лаунчера (15.09.2026):
`GET /backup/export` писал ссылочные Направления (`ref: #TEMPLATE#` или id
пресета) одним `tag` — без `filter`, `include_direct`, `include`, `auto`.
Причина: `Export10` брал `s.Directions` как есть, а у ссылочной записи тело
срезано синхронизацией (`stripReferencedBody`,
`core/build/sync_outbounds.go:255`); UI отдавал `GlobalOutbounds` без
слияния (`ui/configurator/presentation/presenter_state.go:115`). Импорт и
слияние §9 не тронуты.

| Адрес | Что |
|---|---|
| `core/build/resolve_outbounds.go:141` | **`ResolveDirections(dirs, td, target)`** — слитый вид по записи (`resolveBaseBody` + `applyUpdatesToBase`, те же, что у `MergeOutboundUpdatesInPlace`), но запись с оборванной ссылкой НЕ выбрасывается; длина и порядок = вход |
| `core/template/direction_groups.go:123`, `:132` | `DefaultDirectionBlockTag` и **`(*TemplateData).DirectionBlockTag()`** — `magic_nodes.block` или `block-out`, nil-safe; один ответ на форму и экспорт |
| `core/backup/export.go:40`, `:45` | `ExportOptions.Directions` (слитый вид, считает вызывающий — core/backup шаблона не знает; nil = записи состояния как есть, верно только для прямых) и `ExportOptions.BlockTag` (пусто = `block-out`) |
| `core/backup/export10.go:69-82` | состав и порядок Направлений — из `s.Directions`, тело — из `opts.Directions` по тегу (`resolvedDirectionsByTag`, `directions.go:25`, первая запись побеждает) |
| `core/backup/directions.go:50`, `:75` | `exportDirection(d, blockTag)`: `include_block` по тегу шаблона (плюс литерал `block`); `importDirection` не тронут и пишет `block-out` |
| `core/debugapi/backup_endpoints.go:194-211` | `exportBackupBytes`: `LoadTemplate` (ошибка → 500 `export: load template: …`), `build.ResolveDirections(st.Directions, td, build.TargetSpecFromState(st))`, `td.DirectionBlockTag()`; зеркало `/remote/machines/{id}/backup/export` идёт тем же путём |
| `ui/configurator/tabs/settings_backup.go:148` | **`backupExportOptions(presenter, st)`** — то же из `model.TemplateData`; зовётся из `handleBackupExport` (`:125`) |
| `ui/configurator/outbounds_configurator/edit_dialog_helpers.go:197` | `directionBlockTag` формы — через `DirectionBlockTag()` |
| `core/debugapi/backup_endpoints_test.go:480` | **`TestBackupExportCarriesMergedDirections`** — шаблон с тегом блокировки не по умолчанию, `proxy-out` (шаблон + патч пресета), `vpn ②` (USER-патч), пресетное `ru VPN 🇷🇺`, прямое `local-net`; файл против литерала слитого вида (и `/state/outbounds/resolved`); импорт в тот же лаунчер — 0 применено, 4× `backup_direction_exists`, состояние байт в байт; импорт в чистый + `MigrateOutboundsToReferencedShape` + `SyncOutboundsWithTemplate` — без дублей, отбор и опции те же. Мутации «тело из состояния» и «без `BlockTag`» роняют тест |

Документы: `contract/docs/BACKUP.md` §10 («Цена канонизации Направлений»:
тело после слияния, перепривязка при загрузке, тег блокировки шаблона),
`docs/API.md`/`API.ru.md` (строка `/backup/export`, `500`),
`docs/release_notes/1-6-0.md`, `CHANGELOG.md` v1.6.0.

Не сделано: `importDirection` пишет `block-out` литералом — при шаблоне с
другим тегом блокировки круг «экспорт → импорт» превращает `include_block` в
ссылку на несуществующий `block-out` (правка входа вне этой задачи). **Сделано
в §26**: `ImportOptions.BlockTag`.

## 25. Хвосты импорта и загрузки на копии живых данных (релиз 1.6.0)

Ветка `fix/import-axis-ua-chain`. Офлайн-прогон установленной сборки против
develop на копии живого состояния (стенд `scratchpad/livecmp/harness_test.go.txt`)
нашёл три дефекта, четвёртый поймал LxBox на эмуляторе.

| Адрес | Что |
|---|---|
| `core/backup/import.go:503` | **`placeImportedAxis(rules, sectionRules)`** (вместо `renumberImportedAxis`) — номера файла сохраняются у корневых и узловых правил; неразмеченные корневые → хвост `max+1…`, не ниже `UserRuleNumStart`; ни одного размеченного корневого → остаются `nil` (MarkRuleOrder даст пресетам якоря шаблона); стабильная сортировка корня. Сплошная `1000+i` уводила `traffic-processing` (0) и якоря <1000 за `route.rules` шаблона (`core/build/preset_merge.go:373`), пресет из библиотеки (`PresetRuleNum`) вставал перед головой, `NextUserRuleNum` — за перехватчики. Норма — BACKUP.md §9 п. 7, NODE_SECTIONS.md §5, TASKS_LXBOX §16.2 (закрывает вопрос 3 спеки 438 LxBox: у них номера файла сохранялись с 8f538ce9) |
| `core/backup/import.go:649`, `:664`, `:679` | `importDNS`: наборы `haveServers`/`haveRules` строятся только из записей приёмника ДО импорта и по ходу не пополняются — одинаковые записи файла ввозятся все; ключи прежние (`kind`+`tag`+`ref`, правило `kind`+`ref`+тело). BACKUP.md §9 п. 5, TASKS_LXBOX §16.2. Route-правила дефекта не имеют (полная замена); источники дедупят файл сами с собой намеренно (`mergeSources`, `core/backup/merge.go:743`) |
| `core/state/disk_v8_flat_identity.go:37` | **`liftFlatSubscriptionIdentity(data, sources)`** — вызов `core/state/disk_v8.go:62` до `normalizeSourceShape`: плоские `user_agent`/`send_hwid`/`hwid`/`hash_device_model` подписки (v8 сборки bfd5fe15, до переноса 768ef591) → `identity` по ключу, если там пусто; заданное в `identity` главнее; `""`/`null`/чужой тип — отброс; не подписка — не читается. Файл на загрузке не пишется |
| `core/config/chain_nodes.go:288`, `:311` | **`sourceHasPendingChains(ps)`** (включённый chain-узел канона с позициями или `ps.Chains`) и **`chainSourceFailure(ps, i, broken)`** (причины `BrokenChains` по тегу цепочки, подпись — тег при пустой) |
| `core/config/outbound_generator.go:1272`, `:1375`, `:1414`, `:1477` | `chainOnlySources`: источник без узлов прохода 1, но с цепочками, не идёт в silent-empty; вердикт после `ResolveChainSources` — узел есть → `succeededSources`, нет → `source_parse_failed` с причиной цепочки; при раннем выходе «узлов нет вовсе» — пуст. Ложная пометка была видна: строка Sources «⚠ No nodes from this source» (`ui/configurator/tabs/source_tab.go:1021`), отчёт «Итога» (`final_report_model.go:133`) и тост обновления «partially refreshed … (1 failed)» (`core/config_service.go:124`) |
| `core/backup/import_axis_dns_test.go:73` | **`TestImportKeepsAxisZonesAndFileDNSDuplicates`** — раскладка живого состояния (0/945/950/955/1000/1001/1003/1120/1130 + неразмеченное): импорт в пустое и в непустое, DNS-пары файла и совпавшая с приёмником, `NextUserRuleNum`, `MergePresetsIntoRoute` с `route.rules` шаблона (sniff первым). Старый `import.go` роняет все проверки |
| `core/state/disk_v8_flat_identity_test.go:19` | **`TestLoadV8LiftsFlatSubscriptionIdentity`** — три подписки (всё плоско / identity главнее / пустые) и папка; Load → Save без плоских ключей → Load→Save байт в байт. Без вызова в `parseV8` падает |
| `core/backup/backup_test.go:515`, `:558`; `node_sections_roundtrip_test.go:400` | ожидания сплошной нумерации заменены на номера файла |

Стенд на копии живых данных (`scratchpad/livecmp/out_tails/{baseline,fixed}`):
`config.json` байт в байт прежний; отчёт сборки без записей (было
`source_parse_failed` у `chain-test`); круг импорта своего экспорта — состояние
и повторный экспорт без разницы (было 13 номеров → 1000…1012); Load→Save
переносит UA в `identity`, экспорт 1.0 его везёт. Фаза «файл с двумя
одинаковыми DNS-правилами → пустое / живое / повторно» — 2 / 2 / 2.

Не сделано: состояния, уже перенумерованные импортом 1.5.3–1.5.6 (голова на
1000+), этот фикс не лечит; лечение — ставить несортируемому пресету номер
шаблона в `NormalizeRuleOrder` (`core/state/rule_order.go:219`). Импорт в
пустое состояние через `POST /backup/import` выключает правила с целью
`direct-out` (`backup_unknown_outbound`: `knownOutboundsFor` на пустом
состоянии не видит системных тегов) — хвост «два списка известных целей».

## 26. NodeLink для групп и `default`; Направления без узлов (D-115, контракт 1.0.1, релиз 1.6.0)

Ветка `feat/nodelink-groups-directions`. Норма — `contract/docs/NODE_LINK.md`
§2.1, §5.2, §7.3, §8; решения владельца 15.09.2026, форма согласована с LxBox
(`TASKS_LXBOX.md` §17.8). Новой версии схемы state v8 и файла 1.0 нет —
dev-формы читаются терпимо.

### 26.1 Группа адресуется сырым тегом (W1)

| Адрес | Что |
|---|---|
| `core/config/canonical_emit.go:242-252` | `buildCanonicalAuto`: **`IdentityTag: cn.Tag`** у узла-группы — словарь целей берёт сырой тег (`canonicalRawTag`) |
| `core/config/nodelink_resolve.go:53-60`, `98-106`, `242-247` | `NodeLinkTargets.groupFinals` (папка → финальный тег группы → сырой) — только подсказка `emitLinkGroupFinalTagText` в `Resolve`; резолва по финальному тегу нет |
| `core/state/nodelink_normalize.go` | **новый файл**: `NodeLinkFinalTag(policy, raw)` (`norm(prefix+raw+postfix)`, переменные → нет кандидата), `NodeLinkFinalIndex(sources, skip)` (цепочка — свой тег), **`NormalizeNodeLinks(sources, directions)`** — S1 (член группы в контейнере без `folder_id`), S2 (`default` строкой), S3 (пара на финальный тег группы), S5′ (корневая ссылка через опцию Направления); одна строка `InfoLog` на подъём и на неоднозначные |
| `core/state/disk_v8.go:77`, `core/state/migration_v6_to_v7.go:113` | вызовы на чтении v8/v7 и в хвосте миграции v6→v7 (перезаписи файла нет — правило идемпотентно) |
| `core/backup/import.go:433-450` | 1.0 — до `normalizeMemberLinks10`; 0.x — после `resolveImportedHops` |
| `core/backup/import10.go:538-599` | `normalizeMemberLinks10` на общем индексе (`renamed` — через `skip`); `merge.go:968` — `fileFinal` той же формулой |
| `ui/configurator/business/source_record_paste.go:129-135` | вставка записи — после `repointRecordLinks` |
| `core/config/subscription/parse_body.go` | «сперва узлы, затем группы» (IDENTITY.md §1.3): `accept` тегов группам не даёт, `finish` раздаёт их тем же `st.idCounts` после всех узлов и резолвит состав по карте «узлы первыми»; тест — `core/subscription_fetch_test.go` `TestFetchGroupNamesakeDoesNotShiftNodeTag` (Xray-балансировщик перед узлом-тёзкой) |

### 26.2 `group.default` → NodeLink (W2)

| Адрес | Что |
|---|---|
| `core/state/sources_v7.go:102-177` | `AutoGroup.Default *NodeLink`; **`UnmarshalJSON`** через алиас с перекрывающим `Default json.RawMessage`; `decodeGroupDefault` — строка → `{tag}`, объект, иное → `UnmarshalTypeError` |
| `core/config/configtypes/types.go` | `CanonicalAutoGroup.Default`, `ParsedNode.CanonicalGroupDefault` — `*NodeLink`; json-теги у зеркала `NodeLink` |
| `core/state/adapter_source.go:194` | проекция `canonicalLink(n.Group.Default)` |
| `core/config/canonical_emit.go:255-263` | умолчание без `folder_id` в контейнере → свой контейнер (как члены) |
| `core/config/nodelink_resolve.go:508-528` | `resolveCanonicalGroup`: резолв по своему адресу; `canonicalGroupFolder` снят |
| `core/config/migrate_materialize.go:142-147` | fetch/миграция — сразу пара `{subID, raw}` |
| `core/state/subscription_merge.go:339-354` | заливка в папку: `default` → `{id папки, tag}` новым экземпляром или снимается |
| `ui/configurator/business/node_move.go:611-637` | `editGroupLinks`: `default` — тем же `edit`, что члены |
| `core/backup/merge.go:614-641` | `rewriteGroup`: одно правило на члены и `default` |
| `core/backup/file_keys_10.go:154` | сканер ключей: `group.default` как ссылка |
| клоны | `node_move.go` `cloneCanonicalNodeForMove`, `tabs/source_edit_window.go` `cloneCanonicalNode`, `core/backup/export10.go` `cloneNode` — копия указателя |

### 26.3 Направления не хранят узлы (вариант А)

| Адрес | Что |
|---|---|
| `core/config/nodelink_resolve.go:145-181` | **`allRootLinkTargets(pc, dirTags, opts)`** — без `AddOutbounds`; плюс `opts.BlockTag`, `DirectTag`, `SystemTags` |
| `core/template/direction_groups.go:149` | **`(*TemplateData).SystemOutboundTags()`** — теги `config.outbounds`/`endpoints` + `magic_nodes.direct/block`; `core/config_service.go:597` кладёт их в `DirectionBuildOptions.SystemTags` |
| `core/config/direction_options.go` | **новый файл**: `directionDeclaredTags` (все Направления, включая выключенные, и `-auto`), `directionOptionWarnings` — узел в опциях → «use a filter», неизвестное → «not found»; адресат `DirectionTag` |
| `core/config/outbound_generator.go:1203-1208`, `1483-1524` | снимок объявленных до `PrepareDirections`; узлы — до резолва ссылок (плюс `brokenChains`) |
| `ui/configurator/business/direction_options.go` | **новый файл**: `DeclaredRootNames(model)`, **`ValidateDirectionOptions(model, tag, hasAuto, options)`**, `modelNodeNames` (текст отказа) |
| `ui/configurator/outbounds_configurator/edit_dialog.go:588-598` | сохранение Raw — отказ до `renameRefs` |
| `ui/configurator/business/outbound.go:98-116` | `GetAvailableOutbounds`: из опций — только объявленные |
| `ui/configurator/business/tag_guard_model.go:117-140` | `KnownRuleTargetTags` знает строки опций как есть (сброс целей правил маршрут молча не меняет) |
| `core/backup/directions.go:60-109`, `118-156` | `exportDirection(d, blockTag, directionTags)` → `(Direction, localOnly)`; `importDirection(in, blockTag)` |
| `core/backup/export10.go:75-99` | `backup_local_only_dropped` на опции не-Направления |
| `core/backup/import.go` | `ImportOptions.BlockTag`, `SystemTags`; **`filterImportedDirectionOptions`** (`:490-555`) + `WarnBackupDirectionIncludeDropped`; вызов после `mergeSources` |
| `ui/configurator/tabs/settings_backup.go`, `core/debugapi/backup_endpoints.go` | импорт передаёт `BlockTag`/`SystemTags` шаблона; `warnText` — фразы для двух кодов |
| `core/backup/schema_test.go` | `backup_local_only_dropped` вышел из списка снятых |

### 26.4 Тесты

| Адрес | Что |
|---|---|
| `core/state/nodelink_normalize_test.go` | `TestNodeLinkDevFormsLiftedOnRead` — S1, S2 (подписка, копия в папке, корень двух видов), S3, политика с переменной, идемпотентность |
| `core/config/canonical_emit_test.go` | `TestEmitProviderGroupAddressedByRawTag` — позиция и detour на группу при двух префиксах; финальный тег — подсказка |
| `ui/configurator/business/node_move_test.go` | `TestMoveNodeToFolder_GroupDefaultOnOtherMemberSurvivesBuild` |
| `ui/configurator/business/direction_options_test.go` | `TestDirectionsDoNotHoldNodes` — Raw, сборка с тем же составом и предупреждениями, S5′, экспорт |
| `core/backup/corpus_test.go` | ключи `groups{}`, `directions[].include`; кейсы `v10_group_links`, `v10_dev_forms`, `v10_direction_include` |

### 26.5 Поля стороны LxBox (этап 5)

| Адрес | Что |
|---|---|
| `core/backup/file_keys_10.go` | **`lxboxSourceKeys10`**, `lxboxGroupKeys10`, `lxboxRuleKeys10`, `lxboxDNSServerKeys10` — объявленные поля LxBox по виду записи; **`arrayKinds`** / `withKeys` — известные ключи = ключи типа ∪ поля стороны для `kind` |
| `core/backup/backup10.go` | `Backup10.ruleGroups` (не сериализуется) — члены папок с группой по правилу без состава |
| `core/backup/file.go` | `Parse` 1.0 заполняет `ruleGroups` (`ruleOnlyGroups10`, `import10.go`) |
| `core/backup/import10.go` | `decode10Source(in, subIndex, ruleGroup)`: `tag_policy` у server/chain отбрасывается; группа по правилу без `members[]` не ввозится — `WarnBackupGroupDegraded` + `GroupDegradedMembersRule` (`import.go`) |
| `core/backup/lxbox_fields_test.go` | `TestLxBoxSideFieldsLeaveStateUnchanged` — файл с полями и без них даёт одно состояние; `tag_policy` сервера не хранится |

Документы: `contract/docs/NODE_LINK.md`, `BACKUP.md` §2, §4, §6, §10,
`TASKS_LXBOX.md` §16.7, §17.3, §17.7, §17.8, `schema/backup.schema.json`,
`direction.schema.json`, `source_chain.schema.json`,
`registry/backup_warnings.json`, `corpus/backup/README.md`, `contract/VERSION`
1.0.1, `contract/README.md`, `DECISIONS.md` D-115, `SPECS/features/sources.md`,
`directions.md`, `docs/release_notes/1-6-0.md`, `CHANGELOG.md` v1.6.0.

## 27. Импорт в пустое состояние и оси, сдвинутые прошлыми импортами (D-117, релиз 1.6.0)

Ветка `fix/axis-heal-known-targets`. Четыре дефекта с копии живых данных
(стенд `scratchpad/livecmp/harness_ab_test.go.txt`, проба UI-входа
`probe_ui_realflow_test.go.txt`, выход `livecmp/out_ab/{baseline,fixed}`).
Норма — `contract/docs/BACKUP.md` §3, §9 п. 5, §9 п. 7; D-117.

### 27.1 А — ось, сдвинутая импортом 1.5.3–1.5.6

| Адрес | Что |
|---|---|
| `core/state/rule_order.go:233` | **`PinRequiredRuleNums(rules, specs)`** — несортируемому пресету номер шаблона, даже если номер уже есть; сортируемые не трогаются. Выпущенный `renumberImportedRules` (тег v1.5.6, `core/backup/import.go:711`) уводил голову `traffic-processing` 0 → 1000+: сборка ставила её за `route.rules` шаблона (`core/build/preset_merge.go:373`, голова = `num < 1000`), включённый позже пресет с номером шаблона < 1000 вставал перед ней |
| `core/state/rule_order.go:263` | `NormalizeRuleOrder`: дедуп → seed → **pin** → разметка → сортировка. Потребители — сборка (`core/build/resolve_route.go:157`) и загрузка визарда (`ui/configurator/presentation/presenter_state_helpers.go:92`, оттуда вылеченный номер уходит в state.json первым Save). Срез нормализуется на месте, как и прежде (MarkRuleOrder сортирует in place) |
| не сделано | сортируемые пресеты-якоря и перехватчики, уехавшие тем импортом в 1000+ (`private-ips` 950 → 1001, `russian` 1120 → 1011): однозначного признака нет — перетаскивание даёт те же номера (`PlaceRuleAfter` = сосед + 1, ленивый сдвиг). Следствия остаются: такие якоря идут за `route.rules` шаблона, новое правило (`NextUserRuleNum`) встаёт за перехватчиком в зоне 1000..1100, пресет из библиотеки с номером < 1000 встаёт перед сдвинутыми якорями. Лечится удалением и повторным включением пресета (номер шаблона) |

### 27.2 Б — известные цели импорта одним списком

| Адрес | Что |
|---|---|
| `core/backup/import.go:542` | **`importRootNames(opts, dec, s)`** — объявленные корневые имена результата: `direct-out`, тег блокировки, `opts.SystemTags`, Направления файла и приёмника ± `-auto`, свёртки ± `-auto`. Один источник на `filterImportedDirectionOptions` и известные цели |
| `core/backup/import.go:600` | **`importKnownTags`** — `KnownOutbounds` + `KnownTagsFromFile` + корневые узлы + `importRootNames`; `nil` («проверять нечем») только когда ни приёмник, ни файл, ни слияние не назвали ни одного имени сверх двух умолчаний |
| `core/backup/import.go:435`, `:459` | `applyDecoded`: список считается ОДИН раз после `merged.rewriteLinks`; им идут `normalizeMemberLinks10`, **`checkImportedRuleTargets`** (`core/backup/import10.go:338`, выключает inline/srs с неизвестной целью, warning в порядке правил) и `route.final` |
| `core/backup/import10.go:319`, `legacy_read_0x.go` `importRule`/`importJSONRule` | декодеры больше не проверяют цели — только пресет (`backup_unknown_preset`). Прежний список декодера (`import10.go:63-79`, `legacy_read_0x.go:103-110`) не видел системных тегов: на пустом состоянии `knownOutboundsFor` (Debug API) пуст, Направления файла делали список непустым — `direct-out` выключался |
| `core/backup/directions.go:27` | `defaultDirectTag` |
| `ui/configurator/business/tag_guard_model.go:131` | `KnownRuleTargetTags` += `DeclaredRootNames(model)`: сброс осиротевших целей при загрузке (`presentation/rule_target_reset.go`) переводил правило на `block-out` (системный тег `config.outbounds`) на `direct-out` |
| `core/debugapi/backup_endpoints.go` | `knownOutboundsFor` не менялся (комментарий: системные теги едут в `ImportOptions.SystemTags`) |

### 27.3 В — маршрут DNS на восстановлении

| Адрес | Что |
|---|---|
| `core/backup/portable_vars.go:48`, `contract/registry/vars.json` | переносимы `dns_google_udp_outbound`, `dns_google_dot_outbound`, `dns_cloudflare_dot_outbound`, `dns_safe_dns_dot_outbound`, `dns_safe_dns_dot_dom_resolver` — переменные вложенных записей `dns_options.servers` (`template/dns_server_form.go`, имя `dns_<tag>_<var>`) со значением-корневым именем. У LxBox то же значение — `dns.servers[].vars` записи шаблонного сервера (поле стороны LxBox); свести формы — отдельное решение |
| `ui/configurator/presentation/presenter_backup_import.go:42` | **`ImportBackupFile(file, fresh)`** — UI-вход импорта (из `tabs/settings_backup.go` `applyBackup`); `fresh` → слияние в `state.New()` с целью модели (Target/Platform/Arch), иначе в `CreateStateFromModel`. `backupImportOptions` (`:75`) — `KnownOutbounds` модели, пресеты, `BlockTag`, `SystemTags`. Стадия ошибки — `BackupImportStage` (текст диалога) |
| `ui/configurator/tabs/settings_backup.go:189` | `fresh := !StateExists("") && !HasUnsavedChanges()`: визард новой машины — сид шаблона (`configurator.go` ветка без state.json: `LoadConfigFromFile`, `ApplyWizardDNSTemplate`), и «своё сильнее» оставляло DNS-серверы файла выключенными (финальный DNS → системный резолвер) и давало `backup_direction_exists` на Направления шаблона |
| `core/state/save.go:64` | `Save` создаёт каталог состояния: POST /backup/import на чистом `bin/` (каталога `wizard_states` нет, визард ни разу не сохранял) падал 500 |

### 27.4 Г — отсутствующая переменная берёт дефолт шаблона одним правилом

| Адрес | Что |
|---|---|
| `core/template/vars_resolve.go:378` | **`VarValuesFor(vars, state, raw, target)`** — сохранённые значения + скаляр `ResolveTemplateVarsFor` (то же, чем `ApplyTemplateWithVarsFor` собирает секции конфига) для объявленных переменных платформы; без значения и дефолта имя не добавляется; секреты не генерируются |
| `core/build/preset_merge.go:275` | **`PresetMergeContext.globalVarValues()`** = `VarValuesFor(TemplateVars, GlobalVars, nil, Target)`; им идут `ResolveRouteWithGlobals` (`:345`, `:623`) и `ResolveDNS` (`:431`). Раньше `GlobalVars` без `tun`/`enable_proxy_in` давали `#if` false → sniff/resolve `inbound: []`, `@resolve_strategy` без значения ронял resolve, а inbounds собирались по дефолтам |
| `ui/configurator/business/create_config.go:39` | **`PresetGlobalVars(model)`** — то же правило для UI: `tabs/dns_user_rules.go:432,494`, `dns_unified_rules.go:234`, `dns_preset_bundled.go:212` (`gatherTemplateVars`), `preset_ref_edit_dialog.go:118` (превью), `preset_ref_convert.go:34` (конвертация в свои правила — раньше вписывала `inbound: []` навсегда), `business/preset_bundled_dns.go:39` |
| проверено | реальный поток новой машины воспроизводит Г на обоих входах: визард до импорта кладёт в `SettingsVars` только секреты (`clash_secret`, `proxy_in_password`), `CreateStateFromModel` эмитит только тронутые переменные |

### 27.5 Тесты (по одному интеграционному на дефект, каждый падает на старом коде)

| Адрес | Что |
|---|---|
| `core/build/shifted_axis_heal_test.go` | **`TestShiftedAxisHeadPinnedToTemplate`** — ось после импорта 1.5.x + пресет 980 после импорта + `route.rules` шаблона: нормализация (голова 0, остальные номера на месте, идемпотентно, Save → Load), `MergePresetsIntoRoute` из сдвинутого состояния — sniff первым |
| `core/debugapi/backup_import_targets_test.go` | **`TestBackupImportIntoEmptyKeepsRuleTargets`** — экспорт через API → POST /backup/import без state.json: правила на `direct-out`, `block-out`, endpoint шаблона, `reject`, `-auto` Направления, srs на Направление файла включены, `ghost → vpn-9` выключено одним warning; `route.final` direct-out, detour DNS и переменные пресета на месте; файл 0.12 — те же цели |
| `ui/configurator/presentation/backup_restore_dns_route_test.go` | **`TestBackupRestoreKeepsDNSRouteOnNewMachine`** — шаблон репозитория; вход Debug API (`ImportFile` в `state.New()`) и UI-вход новой машины (`ImportBackupFile(fresh)`): `google_udp` включён и `detour: proxy-out` в `ResolveDNS`, без `backup_direction_exists`, правило на `block-out` переживает `LoadState`. Без `fresh` падает на «google_udp выключен» |
| `core/build/preset_var_defaults_test.go` | **`TestMissingVarTakesTemplateDefaultInPresetsAndInbounds`** — синтетический шаблон, `BuildConfig`: без переменных inbounds = sniff.inbound = resolve.inbound = `[tun-in]`, `resolve.strategy` дефолт; с переменными — сохранённые значения |

Документы: `contract/docs/BACKUP.md` (§2 `vars`, поля LxBox `dns.servers[].vars`,
§3, §9 п. 5, §9 п. 7), `contract/registry/vars.json`,
`contract/schema/backup.schema.json` (`rules[].num` — номера файла сохраняются,
D-116/D-117; `dnsServer.vars`), `contract/README.md` (строка 1.0.1, VERSION не
поднят), `DECISIONS.md` D-117, `docs/release_notes/1-6-0.md`, `CHANGELOG.md` v1.6.0.
