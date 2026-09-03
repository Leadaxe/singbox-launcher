# Recon: AutoStrategy фактическая (твины Направлений, свёртки, эмиссия групп)

Аудит существующего кода против SPECS/117-F-N-CLEAN_SOURCE_MODEL/DRAFT.md.
DRAFT говорит: «AutoStrategy — url, interval, tolerance, … — набор полей возьмём
из фактического кода групп». Ниже — этот фактический набор, все места, где он
живёт, и конфликты с моделью.

---

## 1. Каноническая структура: `configtypes.DirectionAuto`

`core/config/configtypes/types.go:337-365` — единственная структура параметров
авто-группы, используется НА ДВУХ уровнях (Направление и свёртка подписки —
`source_fold.go:43` это фиксирует намеренно: «одна и та же настройка на двух
уровнях»).

Полный набор полей:

| Поле | Тип | Семантика |
|---|---|---|
| `mode` | string | `""`/`least_test` (апстримный urltest, бит-в-бит) \| `round_robin` (SPEC 088, расширение lx-ядра) — types.go:327-330 |
| `url` | string | цель проверки |
| `interval` | string | период проверки |
| `idle_timeout` | string | таймаут простоя |
| `tolerance` | `*TemplateInt` | число ЛИБО ссылка на переменную шаблона `"@urltest_tolerance"` — types.go:343-348, 372-436 |
| `interrupt_exist_connections` | `*bool` | ТРЁХЗНАЧНОСТЬ: nil = «шаблон решает», false = «выключил явно» — types.go:350-355 |
| `pool` | int | только round_robin |
| `pool_tolerance` | `*TemplateInt` | только round_robin; указатель из-за ловушки omitempty — types.go:357-363 |
| `sticky_hash` | []string | только round_robin; пустой список ≠ выкл, сентинел `["none"]` — direction_twins.go:156-161 |

Контракт (`contract/schema/direction.schema.json`, `$defs.auto`) объявляет тот
же набор из 9 полей: mode, url, interval, tolerance (int), idle_timeout,
interrupt_exist_connections, pool, pool_tolerance, sticky_hash. Расхождение с
Go: в контракте tolerance — integer, в Go — TemplateInt (int | "@var").

## 2. Твины Направлений — `core/config/direction_twins.go`

- `twinSuffix = "-auto"` (:31) — формула тега зашита, зеркалит
  `group_templates.magic_nodes.auto.tpl` = `"{parent_tag}-auto"` (шаблон
  `bin/wizard_template.json`, magic_nodes).
- `PrepareDirections` (:45-60) — проход 0: выключенные Направления выпадают,
  твины разворачиваются. Твин НЕ хранится в состоянии.
- Коллизионный гард (:87-103): карта `used` — только по тегам самих
  Направлений; занятый `<tag>-auto` → твин не создаётся, warning.
- `buildTwin` (:126-187) — слияние: options шаблона
  (`group_templates.auto.options`) ← перекрываются полями Auto пользователя.
  Эмитятся: `url`, `interval`, `tolerance` (как есть — число или "@var"),
  `idle_timeout`, `interrupt_exist_connections`; при `round_robin` — ещё
  `mode` + `balancer{pool, sticky_hash (дефолт-сентинел ["none"]),
  pool_tolerance (0 если не задан)}` (:153-171).
- Состав твина = тот же ФИЛЬТР, что у родителя (`Filters: parent.Filters`,
  :177); из состава auto-групп выкидываются узлы-группы —
  `dropGroupNodes` (:280-289, SchemeGroup).
- Служебные поля сборки: `TwinOf`/`TwinTag` — `json:"-"` (types.go:315-316),
  живут только в рантайме сборки.
- Родитель получает твин первой опцией и умолчанием селектора, если
  preferredDefault не поймал (`outbound_generator.go:805-819`).
- `DirectionBuildOptions` (:208-219): TwinOptions + BlockTag/DirectTag из
  `group_templates.magic_nodes.{block,direct}`.

## 3. Свёртки подписок (fold) — `core/config/source_folds.go` + `configtypes/source_fold.go`

Прямой предшественник FolderReplace из DRAFT.

- `SourceFold {Mode: select|auto|select_auto, Auto: *DirectionAuto}` —
  source_fold.go:33-44. Режимы 1:1 с DRAFT `ReplaceMode {manual|auto|both}`.
- Теги ДЕРИВАТИВНЫЕ от префикса, не явные: `FoldAutoTag = <PFX>auto`,
  `FoldSelectTag = <PFX>select`, дефолтный префикс `"<индекс+1>:"` —
  source_fold.go:86-100. Индекс-зависимость: перестановка источников меняет
  теги при пустом tag_prefix.
- `PrepareSourceFolds` (source_folds.go:41-69): группы разворачиваются на
  каждой сборке, переиспользуя `buildTwin` (:95) — та же механика, что у
  твинов Направлений; TwinOf у группы подписки затирается (:100), т.к. это
  самостоятельная запись, а не производная.
- Экспозиция — через ДВА флага + маркеры в comment:
  `ExcludeFromGlobal` + `ExposeGroupTagsToGlobal` (:62-64), маркеры
  `WIZARD:auto` / `WIZARD:selector` (:29-31), причём при `select_auto`
  авто-группа маркера НЕ получает (:102-112) — предлагается только селектор.
- Селектор свёртки хардкодит `interrupt_exist_connections: true` и
  `default: <autoTag>` при наличии авто (:116-131).
- Fold — миграция четырёх легаси-флагов (`Local auto`, `Local select`,
  `Exclude from global`, `Expose tags`) — source_fold.go:8-11.

## 4. Эмиссия селекторов/urltest — `core/config/outbound_generator.go`

`GenerateSelectorWithFilteredAddOutbounds` (:673-941):

- Порядок полей: `tag`, `type`, `default` (до outbounds), `outbounds`,
  `interrupt_exist_connections`, остальные Options по сортировке ключей.
- `default` дважды валидируется на вхождение в состав (:836-849, :898-915) —
  ядро отвергает весь конфиг иначе; невходящий — снимается с warning.
- Options — ОТКРЫТЫЙ pass-through: любой ключ из Options уезжает в конфиг
  (:919-922), allowlist'а нет.
- `sanitizeBalancerOptions` (:951-975): пустой `balancer.sticky_hash`
  удаляется (сентинел-контракт lx-ядра SPEC 019/088).
- Подстановка `@urltest_*` — `SubstituteParserConfigPlaceholders`
  (:1090-1093), ПОСЛЕ PrepareDirections/PrepareSourceFolds; хардкод-фолбэки
  varsubst.go:95-106: url=`https://cp.cloudflare.com/generate_204`,
  interval=`5m`, tolerance=`100` (число).

## 5. Импортированные группы (SchemeGroup) — прообраз `Auto extends Node`

DRAFT: `Auto` — узел с `members` + `AutoStrategy`. Фактически сегодня:

- `singbox_groups.go:32-38` — ЗАКРЫТЫЙ allowlist опций импортированной
  группы: `url`, `interval`, `tolerance`, `idle_timeout`,
  `interrupt_exist_connections`; плюс `default` (валидируется на вхождение,
  :119-125) и `outbounds` (`GroupMembersKey = "outbounds"`, types.go:542).
  `mode`/`balancer` при импорте НЕ переносятся.
- Тип сохраняется как есть: selector остаётся selector'ом (в отличие от
  Dart, который конвертит обе формы в urltest — registry group.json note).
- `xray_balancer.go:39-86` — Xray-балансировщик → узел-группа urltest,
  поля только `url` (дефолт `https://www.gstatic.com/generate_204`, :24) и
  `interval` (дефолт `3m`, :27). Дефолты у Go и Dart РАЗНЫЕ (Dart:
  cp.cloudflare.com / 15m — registry group.json note).
- Эмиссия — `generateGroupNodeJSON` (outbound_generator.go:1374-1422):
  tag, type, outbounds, остальные ключи Outbound сортированно; пустая
  группа = ошибка (ядро не стартует).
- Хранение: ParsedNode.Outbound как рыхлый map, НЕ DirectionAuto.

## 6. Шаблон — `group_templates` (bin/wizard_template.json) и `core/template/direction_groups.go`

- `auto.options` фактические: `url:"@urltest_url"`,
  `interval:"@urltest_interval"`, `tolerance:"@urltest_tolerance"`,
  `interrupt_exist_connections:true`. `idle_timeout` в шаблоне НЕТ.
- `channel.options`: `interrupt_exist_connections:true`.
- `magic_nodes`: auto (generate, `{parent_tag}-auto`), direct (`direct-out`),
  block (`block-out`).
- Переменные: `urltest_url` (дефолт cp.cloudflare.com/generate_204),
  `urltest_interval` (5m), `urltest_tolerance` (100).
- `DirectionGroupSpec.Options` — открытый `map[string]json.RawMessage`
  (direction_groups.go:64): автор шаблона может добавить ЛЮБОЙ ключ
  (напр. idle_timeout), и он доедет до конфига через pass-through эмиссии.
- Плюс `parser_config.outbounds[0]` (proxy-out) в шаблоне уже несёт
  `auto:{url:"@urltest_url",...}` — required-Направление с твином.

## 7. UI-форма — `ui/configurator/autogroupform/form.go`

Общая форма для вкладки «Автовыбор» Направления и вкладки «Группа» свёртки
(:1-11). Показывает: mode (fastest/pool), interval, tolerance, url, pool,
pool_tolerance, sticky_hash (5 ключей: process, domain, source_ip, dest_ip,
dest_port — :29). НЕ показывает `interrupt_exist_connections` и
`idle_timeout` — они проносятся через `keep` (:85-91), чтобы diff не принял
их пропажу за правку. Дефолты пула: size=3, tolerance=0 (:58-61).

## 8. Диффы и нормализация

- `core/build/outbound_diff.go:88-97` — Auto диффится ЦЕЛИКОМ (replace, не
  по-полевой); `auto:null` = «двойник выключен» — осознанное значение,
  спец-кейсы в `hasEmptyOverride` (:216-219) и
  `core/state/load_normalize.go:118` (isAllEmptyPatch).

---

## Конфликты и пробелы модели DRAFT

### К-1. Набор AutoStrategy шире, чем «url, interval, tolerance, …»
Фактический набор — 9 полей, включая весь round_robin-блок (mode, pool,
pool_tolerance, sticky_hash с сентинелом `["none"]`) — расширение lx-ядра,
которого нет у апстримного urltest. Плюс две нетривиальные семантики,
которые DRAFT не оговаривает:
- `tolerance`/`pool_tolerance` = TemplateInt (число ИЛИ `"@urltest_*"`-ссылка
  на переменную шаблона; подстановка на сборке, varsubst). Если AutoStrategy
  новой модели хранит голые числа — ломается «наследовать из Settings»
  (плейсхолдер-ссылки — первый вариант в UI-селектах, form.go:216-223).
- `interrupt_exist_connections` = трёхзначный *bool (nil = «шаблон решает»).
Резолв дефолтов трёхуровневый: поле пользователя → group_templates.auto.options
→ хардкод-фолбэк varsubst.go:95-106. Модель должна сказать, где это живёт.

### К-2. Auto (провайдерская группа) сегодня — НЕ DirectionAuto
Импортированные группы (SchemeGroup) хранят опции рыхлым map с ЗАКРЫТЫМ
allowlist'ом (singbox_groups.go:32-38): url/interval/tolerance/idle_timeout/
interrupt + `default` + сохранённый type (selector ИЛИ urltest). В AutoStrategy
DRAFT нет места для: (а) типа группы — импортированный selector остаётся
selector'ом (Go-поведение, зафиксировано в registry как расхождение с Dart);
(б) поля `default` селектора. Конверсия SchemeGroup → Auto{members, strategy}
теряет оба, либо AutoStrategy нужно расширить (type?, default?), либо
зафиксировать деградацию selector→urltest (как у Dart, с warning).

### К-3. Тег твина FolderReplace: коллизия формулы `<tag>-auto`
Сегодня твины Направлений (`<tag>-auto`) и группы свёрток (`<PFX>auto`,
`<PFX>select` — БЕЗ дефиса) живут в разных формулах — столкновение почти
исключено. DRAFT переводит FolderReplace(both) на ТУ ЖЕ формулу
`"<tag>-auto"`, что у твинов Направлений. Гард занятости тега существует
только внутри ExpandDirectionTwins (direction_twins.go:87-103) и видит только
directions; PrepareSourceFolds выполняется ПОСЛЕ (outbound_generator.go:
1082, 1087) и никакой перекрёстной проверки не делает. Направление `x` с auto
и FolderReplace с tag=`x` дадут два `x-auto`. Новой модели нужен ЕДИНЫЙ
неймспейс-гард дериватов (или запрет совпадения replace.tag с тегом
Направления — они и так в одном неймспейсе тегов конфига).

### К-4. Механики твинов и FolderReplace НЕ дублируются в сборке, но
дублируются в экспозиции
Сборка общая — source_folds.go:95 переиспользует buildTwin. А вот «как группа
попадает в пул Направлений» — два разных механизма: твин помечается TwinOf
(не предлагается никому, кроме родителя), fold — связкой
ExcludeFromGlobal + ExposeGroupTagsToGlobal + маркеры `WIZARD:auto`/
`WIZARD:selector` В КОММЕНТАРИИ (source_folds.go:29-31). DRAFT убивает
fold-флаг и excludeFromGlobal — вместе с ними должна умереть и
маркеры-в-comment механика (хрупкая: комментарий — носитель семантики).
Модель не говорит, чем заменяется expose-механика для replace-тегов
(«Резолв NodeLink обязан видеть replace-теги наравне с nodes[]» — сказано
про резолв, но не про пул кандидатов Направлений).

### К-5. Явный tag против деривативных `<PFX>select`/`<PFX>auto`
DRAFT: «tag ЯВНЫЙ, не из имени папки». Сегодня теги свёртки выводятся из
tag_prefix, а дефолтный префикс — из ИНДЕКСА источника (source_fold.go:86-91):
пустой tag_prefix → `1:auto`. Миграция fold → FolderReplace обязана
МАТЕРИАЛИЗОВАТЬ вычисленный на момент миграции тег (включая индексный),
иначе ссылки правил/цепочек на `1:select` порвутся. DRAFT миграцию «fold →
replace один в один» упоминает, но про фиксацию деривативного тега — нет.

### К-6. Селекторная половина FolderReplace без опций
При mode=manual strategy=null — но у селектора есть СВОИ поля:
`interrupt_exist_connections` (сегодня хардкод true, source_folds.go:122) и
`default` (сегодня — авто-твин при both). В DRAFT у FolderReplace нет места
для селекторных опций; либо наследовать `group_templates.channel.options`
(как Направления), либо зафиксировать хардкод.

### К-7. Пул FolderReplace «вся папка» и узлы-группы
Твины Направлений выкидывают из состава узлы-группы (dropGroupNodes,
direction_twins.go:280-289: urltest поверх группы мерил бы чужой выбор).
Пул FolderReplace = вся папка, а папка может содержать Auto-узлы (DRAFT сам
разрешает: `List<Node> nodes; // Server, Chain, Auto`). Для auto/both режима
нужно то же исключение групп из urltest-состава — в DRAFT не оговорено.

### К-8. Кросс-платформенные дефолты не сходятся
Registry (contract/registry/protocols/group.json, note): Go для
Xray-балансировщика — gstatic/3m; Dart — cp.cloudflare.com/15m; Dart маппит
strategy→mode, Go игнорирует. Раз AutoStrategy становится канонической
сущностью контракта — дефолты пора зафиксировать в одном месте.

### К-9. Контракт vs Go: tolerance
direction.schema.json `$defs.auto.tolerance` — integer;
Go — TemplateInt (int | "@var"-строка). Состояние с `"@urltest_tolerance"`
в tolerance формально не проходит контрактную схему уже сегодня.

### К-10. Мелочь: `idle_timeout` отсутствует в шаблонных options
group_templates.auto.options шаблона не задаёт idle_timeout, UI его не
показывает (form.go:85-91 проносит через keep) — поле фактически доступно
только импорту/ручной правке state. При проектировании AutoStrategy решить:
поле первого класса или выкинуть из формы навсегда.
