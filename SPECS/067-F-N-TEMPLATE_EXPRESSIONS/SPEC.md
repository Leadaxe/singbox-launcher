# SPEC 067-F-N — TEMPLATE_EXPRESSIONS

## Проблема

`bin/wizard_template.json` сейчас умеет условия только на **уровне целой entry**
в `params[]` через `if` / `if_or` (списки bool var'ов). Этого хватало пока
требовалось условно эмитить большой block (целый `inbounds`, целая группа
`route.rules`, preset target). Когда нужно условно включить **одно поле внутри
уже эмиченного объекта** — engine не позволяет.

Конкретный кейс v0.9.9: `mixed` proxy-in inbound поддерживает аутентификацию
через `users: [{username, password}]`. Поле `users` должно появляться рядом с
`type`/`tag`/`listen`/`listen_port` только когда юзер включил аутентификацию
(новый toggle `proxy_in_auth_enabled`):

```json
{
  "type": "mixed",
  "tag": "proxy-in",
  "listen": "@proxy_in_listen",
  "listen_port": "@proxy_in_listen_port",
  "set_system_proxy": "@proxy_in_set_system_proxy",
  "users": [{"username": "@proxy_in_username", "password": "@proxy_in_password"}]
             ← поле нужно условно
}
```

Обходные пути и почему они плохие:

* **Две взаимоисключающие entries** — нет `if_not` / negation. `if`/`if_or`
  обе принимают только "all bool true" / "any bool true".
* **Always-on `users:[{"":""}]`** — `mixed` inbound с пустыми creds ломает
  sing-box: либо 401 анонимам, либо требует пустой логин.
* **Go post-substitute hook** — работает, но прячет логику от шаблона. Через
  две недели понадобится «ещё одно условное поле где-то ещё» — снова Go-код,
  снова SPEC. Не масштабируется.

Нужен **declarative conditional field inclusion** прямо в template engine.

## Целевая модель

Вводим **template expressions** v1 — в одном релизе, без отложенных фаз:

1. Control-construct **`#if`** — meta-key в `substituteWalk`.
2. **Expression language** — predicates (`#in`, `#not`, …) в `and`/`or`.
3. **Runtime globals** **`@platform`** / **`@arch`** — сразу в predicates (без
   globals `#if` не закрывает platform-условия в payload).
4. **`@`-only** var-ref в outer `if`/`if_or`.

`#`-префикс зарезервирован для control-constructs (дисциплина: все `#*` ключи
НЕ попадают в output; неизвестные → warn + drop для forward-compat).

### Принцип naming — двухуровневая модель

Не все control-machinery ключи получают `#`-prefix. Правило:

> **`#` — только там, где без контекста не различить смысл.
> Bare — там, где контекст однозначен.**

Применение:

* **`#`-prefixed:**
  - **Construct gateway** (`#if`) — маркер «вход в control-construct world».
    Без `#` walker мог бы спутать `if`-как-control с `if`-как-валидное
    имя поля данных в user-defined object'е.
  - **Predicates** (`#in`, `#not`, `#notEmpty`, `#isEmpty`, `#notIn`,
    `#matches`) — живут в `and`/`or`-массивах вперемешку с bare
    `@var`-строками и data-литералами. Без `#` walker не отличит `"in"`
    (predicate-имя) от `"in"` (string literal как value сравнения).
  - **Будущие `#`-constructs** (`#for_each`, `#include`, …) — тот же gateway-
    принцип что у `#if`.

* **Bare (без префикса):**
  - **Inner keys тела `#if`** (`and`, `or`, `value`, `else`) — живут ТОЛЬКО
    внутри `#if`-body. Walker уже в known scope, маркер избыточен.
  - **Outer legacy keys** (`params[].if`, `params[].if_or`, `params[].value`,
    `params[].mode`, `params[].name`) — устоявшийся schema, миграция не
    оправдана сейчас (см. open question SPEC 068 как future).

* **`@`-prefixed:**
  - **Template var-ref** (outer `if`/`if_or`, placeholders в JSON) — **только**
    имена из `vars[]`; bare `"var"` → loader error.
  - **Runtime globals** (`@platform`, `@arch`) — только в `#if.and`/`or`
    predicates; inject'ятся walker'ом из `runtime.GOOS` / `runtime.GOARCH`;
    **не** в `vars[]`, **не** substitute из state.
  - Predicates и placeholders — см. таблицы ниже.

Аналогии в других DSL: HTML — `<a href="...">` (gateway `<a>` маркирован,
inner `href` bare). C preprocessor — `#if FOO` (gateway `#if` маркирован,
inner `FOO` bare). Принцип «маркер на scope-switch, bare внутри known scope».

### Форма

```json
"#if": {
  "and":   [<predicate>, <predicate>, ...],   // mutually exclusive with `or`
  "or":    [<predicate>, <predicate>, ...],   // mutually exclusive with `and`
  "value": <any JSON>,                         // then-branch
  "else":  <any JSON>                          // optional else-branch
}
```

**Поля:**

* **Ровно один** из `and` / `or` обязан присутствовать.
  - Нет ни одного → validation error на template load.
  - Оба → validation error. Composition через вложенный `#if`.
* `value` — обязателен. Подставляется когда condition satisfied.
* `else` — опционален. Подставляется когда condition НЕ satisfied. Если
  отсутствует и condition false:
  - **map-spread context** → ключ `#if` просто удаляется (родительский объект
    остаётся без добавлений);
  - **array-element context** → элемент удаляется из массива (массив сокращается
    на 1).

### Семантика condition

* `and: [p1, p2, ..., pN]` — все predicate'ы возвращают true. Short-circuit:
  на первом false stop, condition false.
* `or: [p1, p2, ..., pN]` — хотя бы один predicate true. Short-circuit на
  первом true.

### Expression language — predicates

Каждый элемент `and` / `or` — predicate. Восемь форм:

| Форма | Семантика |
|---|---|
| `"@var"` | bool template var → `scalar == "true"` |
| `{"@var": "literal"}` | equality: trim(scalar) == "literal" (literal НЕ начинается с `#`) |
| `{"@var": "#notEmpty"}` | text → `len(trim(scalar)) > 0`; text_list → `len(list) > 0`; bool → `scalar == "true"` |
| `{"@var": "#isEmpty"}` | инверсия `#notEmpty` (shorthand для `{"#not": {"@var": "#notEmpty"}}`) |
| `{"@var": {"#in":      ["a", "b", "c"]}}` | trim(scalar) присутствует в списке |
| `{"@var": {"#notIn":   ["a", "b", "c"]}}` | trim(scalar) отсутствует в списке (shorthand для `{"#not": {"@var": {"#in": [...]}}}`) |
| `{"@var": {"#matches": "^[a-z]+$"}}` | trim(scalar) match'ит Go-regexp |
| `{"#not": <predicate>}` | унарная негация: `NOT evaluate(predicate)`. Inner — любой predicate из этой таблицы (рекурсивно). |

`@var` в таблице — template var из `vars[]` **или** runtime global `@platform` /
`@arch` (см. § Runtime globals). Globals поддерживают только text-predicates
(equality, `#in`, …), не bare bool-form.

`#not` — higher-order operator. Делает `#isEmpty` / `#notIn` редундантными
(они — синтаксический сахар для `{"#not": {"@var": "#notEmpty"}}` / `{"#not":
{"@var": {"#in":[...]}}}`), но оставляем shorthand-ы как канонические идиомы
для частых паттернов.

Примеры комбинирования:

```json
"and": [
  "@flag_a",                                        // bool true
  {"#not": "@flag_b"},                              // bool NOT true (i.e. false)
  {"@platform": {"#in": ["darwin", "linux"]}},      // runtime GOOS
  {"@arch": "amd64"},                               // runtime GOARCH
  {"@protocol": {"#in": ["vless", "trojan"]}},      // в списке
  {"#not": {"@protocol": {"#in": ["http"]}}},       // НЕ в списке (альтернатива #notIn)
  {"#not": {"@hostname": {"#matches": "^test-"}}}   // НЕ начинается на "test-"
]
```

**Правила распознавания** (walker, при обходе элементов `and`/`or` массива):

* Голая строка (`"@varname"`) → bool form (только template bool var; **не**
  `@platform` / `@arch`).
* Single-key object с ключом `"@varname"`:
  - Если varname — `platform` или `arch` → runtime global (text scalar).
  - Иначе — lookup в `vars[]`.
  - Value тоже строка:
    - Совпадает с `#notEmpty` / `#isEmpty` → no-arg predicate.
    - Начинается с `#` но не входит в known no-arg → unknown predicate, validation error.
    - Иначе → literal equality.
  - Value — single-key object с ключом `"#in"`/`"#notIn"`/`"#matches"` → arg-taking predicate, value этого подключа = аргумент.
  - Любая другая форма → validation error.
* Single-key object с ключом `"#not"` → unary negation. Value — inner predicate
  (любой из форм этой таблицы, рекурсивно). Distinct от `{"@var": ...}` тем,
  что key начинается с `#` а не `@`.
* Любой другой shape элемента (multi-key object, array, scalar без `@`) →
  validation error на template load.

**Type-based validation** при load template'а:

| Predicate | Применим к |
|---|---|
| bare `"@var"` | bool template var |
| `{"@platform": …}` / `{"@arch": …}` | runtime global (always text) |
| `{"@var": "literal"}` | text template var |
| `#notEmpty` / `#isEmpty` | text, text_list, bool template var; `@platform` / `@arch` |
| `#in` / `#notIn` | text template var; `@platform` / `@arch` |
| `#matches` | text template var; `@platform` / `@arch` |
| `{"#not": <inner>}` | следует typing inner-predicate'а (negation не меняет тип) |

Mismatch → loader error. `vars[]` с `name` `platform` или `arch` → loader error.

**Substitution внутри предикатов:**

Аргументы predicate'ов (literal в equality, элементы `#in`/`#notIn`, regex в
`#matches`) могут содержать `@var` placeholder'ы — walker substitute'ит их
рекурсивно ДО оценки predicate'а. Это даёт композицию:

```json
"and": [
  {"@selected_proto": {"#in": "@allowed_protocols_list"}}
]
```

Если `allowed_protocols_list` — text_list var с `["vless", "trojan"]`, predicate
проверит что `selected_proto` ∈ `["vless", "trojan"]`.

ИСКЛЮЧЕНИЕ: bare-string `"@var"` элементы и ключи `"@var"` в single-key объектах
walker НЕ substitute'ит (иначе потеряется var-reference). Это локальная семантика
ВНУТРИ `#if.and` / `#if.or` — снаружи `@var` substitute по обычным правилам.

### Runtime globals — `@platform`, `@arch` (v1, не откладывать)

Две **зарезервированные** pseudo-var, доступные **только** в predicates
`#if.and` / `#if.or`. **Ship вместе с walker'ом** — иначе platform-условия
внутри `value` снова требуют дублирования `params[]` / bool-kostyli.

| Global | Runtime source | Scalar (text) | Пример значений |
|---|---|---|---|
| `@platform` | `runtime.GOOS` | GOOS текущего процесса | `"darwin"`, `"windows"`, `"linux"` |
| `@arch` | `runtime.GOARCH` | GOARCH текущего процесса | `"amd64"`, `"arm64"`, `"386"` |

**Семантика:** те же predicate-формы, что у text-var — equality, `#in`,
`#notIn`, `#matches`, `#notEmpty` / `#isEmpty`. **Bare** `"@platform"` /
`"@arch"` в списке (bool-form) → validation error (не bool).

**Win7** (legacy `windows/386` binary): `{"@platform": "windows"}` +
`{"@arch": "386"}` в одном `and` — эквивалент «только win7-сборка». Совпадает
с дисциплиной `platforms: ["windows"]` + отличие по arch, не отдельная метка
`win7` в `@platform`.

**Case-sensitivity:** `@platform` / `@arch` lookup и сравнение значений —
**case-sensitive**, в lower-case (`"darwin"`, `"windows"`, `"linux"`,
`"amd64"`, `"arm64"`, `"386"`) — те же значения что выдаёт `runtime.GOOS` /
`runtime.GOARCH`. Reserved-name check (`vars[].name == "platform"|"arch"`)
тоже strict lower-case: `"Platform"` / `"PLATFORM"` — НЕ reserved (но
плохая практика, лучше избегать confusing camelCase). Это соответствует
case-sensitivity всех остальных var-имён в template.

Примеры:

```json
"and": [
  {"@platform": {"#in": ["darwin", "linux"]}},
  "@proxy_in_auth_enabled"
]
```

```json
"and": [
  {"@platform": "windows"},
  {"@arch": "386"},
  {"@tun_stack": {"#in": ["gvisor", "system"]}}
]
```

**Real-world case — схлопывание TUN inbound.** В текущем шаблоне (v0.9.9) два
`params[].name="inbounds"` entry, различающиеся **только** наличием
`interface_name: "singbox-tun0"`:

```json
// БЫЛО (2 entries, 40 строк, общий блок дублируется)
{ "name": "inbounds", "platforms": ["windows", "linux"], "if": ["@tun"],
  "value": [{ "type": "tun", "tag": "tun-in", "interface_name": "singbox-tun0",
              "address": ["@tun_address"], "mtu": "@tun_mtu",
              "auto_route": true, "strict_route": "@strict_route",
              "stack": "@tun_stack" }] },
{ "name": "inbounds", "platforms": ["darwin"], "if": ["@tun"],
  "value": [{ "type": "tun", "tag": "tun-in",
              "address": ["@tun_address"], "mtu": "@tun_mtu",
              "auto_route": true, "strict_route": "@strict_route",
              "stack": "@tun_stack" }] }
```

```json
// СТАНЕТ (1 entry, общий блок — один раз, platform-conditional поле — изолировано)
{
  "name": "inbounds",
  "if": ["@tun"],
  "value": [{
    "type": "tun",
    "tag": "tun-in",
    "address": ["@tun_address"],
    "mtu": "@tun_mtu",
    "auto_route": true,
    "strict_route": "@strict_route",
    "stack": "@tun_stack",
    "#if": {
      "and": [{"@platform": {"#in": ["windows", "linux"]}}],
      "value": {"interface_name": "singbox-tun0"}
    }
  }]
}
```

Эффект: −1 entry, −20 строк, общие поля не задублированы (`tag`/`mtu`/
`strict_route`/etc) — добавление нового общего поля больше нельзя забыть в
одном из вариантов. Платформо-условное поле инкапсулировано в `#if`-spread.

Такие platform-split duplications есть и в других местах шаблона
(`presets[]` split-all-traffic mac+win vs linux, DNS preset bundled
вариации). **Schema cleanup отдельной mini-SPEC** после landing'а 067 —
не в scope этой SPEC (см. § Open questions).

**Reserved names:** в `vars[]` **запрещены** `name: "platform"` и
`name: "arch"` — loader error (collision с globals).

**Outer `if`/`if_or`:** `@platform` / `@arch` **не** допускаются — там только
bool template vars. Platform gate на уровне param — по-прежнему `platforms[]`
или `#if` внутри `value`.

**Не заменяют** `params[].platforms` / `vars[].platforms` / `presets.platforms`
на schema-envelope — globals только для expression language внутри payload.

**Implementation:** `evaluatePredicate` принимает `(goos, goarch string)`.
Lookup `@platform` / `@arch` — до `resolved` map. `SubstituteVarsInJSON` и все
`#if`-handlers — параметры `goos`, `goarch` (callers: `runtime.GOOS`,
`runtime.GOARCH`; тесты — fake).

### Два режима размещения `#if`

**Map-spread mode** — `#if` как ключ внутри объекта:

```json
{
  "type": "mixed",
  "tag": "proxy-in",
  "#if": {
    "and": ["@proxy_in_auth_enabled"],
    "value": {"users": [{"username": "@proxy_in_username", "password": "@proxy_in_password"}]}
  }
}
```

Поведение:
* condition true → walker substitute'ит placeholder'ы внутри `value`, затем
  мерджит **поля** `value` (ОБЯЗАН быть объектом) в родительский объект. Ключ
  `#if` удаляется.
* condition false:
  - есть `else` → substitute'ит `else`, мерджит его поля в parent. Ключ удаляется.
  - нет `else` → ключ `#if` просто удаляется.
* Collision (parent уже имеет ключ из `value`/`else`) → branch overrides
  (документировано; аналог `mode=replace`).

**Array-element mode** — `#if` как единственный ключ объекта-элемента:

```json
"options": [
  "always",
  {"#if": {"and": ["@dark_mode"], "value": "extra-dark", "else": "extra-light"}},
  "regular"
]
```

Поведение:
* condition true → walker substitute'ит `value`, элемент массива заменяется на
  него (любой тип: scalar/object/array).
* condition false:
  - есть `else` → walker substitute'ит `else`, элемент заменяется на него.
  - нет `else` → элемент **удаляется** из массива (длина -1).
* Detection rule: элемент — `#if` wrapper если это объект из РОВНО одного
  ключа `#if`. Иначе обычный элемент с возможным spread-mode `#if` внутри.

### Композиция

* **Вложение** — `#if` может быть внутри `value` или `else` другого `#if`,
  рекурсивно. Walker single-pass: внутренний `#if` обрабатывается после того
  как outer `#if` выбрал ветку.
* **`(A AND B) OR C`** — `#if.or` с predicate-list где один из элементов сам
  `#if`-объект... нет, на уровне expression language такого нет. Composition
  через вложенный `#if` в array-element mode:
  ```json
  {"#if": {
    "or": ["@flag_c"],
    "value": "branch_a",
    "else": {"#if": {"and": ["@flag_a", "@flag_b"], "value": "branch_a", "else": "branch_b"}}
  }}
  ```
* **Negation** — `else` покрывает: для "NOT X = Y" пиши `{"#if": {"and":["@x"], "else": Y}}`
  (value отсутствует → когда X=true ничего; X=false → Y).

### Стиль форматирования `#if` в шаблоне

Шаблон форматируется стандартным 2-space pretty-print'ом, но **`#if`-блоки —
гибрид**: opener на одной строке, payload разворачивается только при
необходимости. Цель — `#if` визуально читается как декоратор поля, не как
6-строчный обёрточный блок.

**Три формы (по сложности `value`):**

1. **Скалярный value** → полностью на одной строке:
   ```jsonc
   {"#if": {"and": ["@tun"], "value": "tun-in"}}
   ```

2. **Map value с одним коротким ключом** (`value: {key: scalar}` или
   `value: {key: [scalar]}`) → opener inline, payload expanded на следующую
   строку, `}}` на своей:
   ```jsonc
   "#if": {"and": [{"@platform": {"#in": ["windows", "linux"]}}], "value": {
     "interface_name": "singbox-tun0"
   }}
   ```

3. **Map value с массивом-объектов** короткой длины → массив тоже inline:
   ```jsonc
   "#if": {"and": ["@proxy_in_auth_enabled"], "value": {
     "users": [{"username": "@proxy_in_username", "password": "@proxy_in_password"}]
   }}
   ```

**Правило большого пальца:**

* `and`/`or` массив, `value`-обёртка, и (если есть) `else`-обёртка — **всегда
  на одной строке** в opener'е.
* Сложный payload (multi-key map, длинная строка) — на отдельных строках
  внутри `value: { ... }`.
* `}}` — на своей строке, на уровне отступа родительского ключа.

**Когда `#if` сидит в массиве** (array-element mode) вместе с другими полями
объекта — trailing fields после `]` пакуются на одну строку:

```jsonc
{
  "inbound": [
    {"#if": {"and": ["@tun"],             "value": "tun-in"}},
    {"#if": {"and": ["@enable_proxy_in"], "value": "proxy-in"}}
  ], "action": "resolve", "strategy": "@resolve_strategy"
}
```

Параллельные `#if` в `and` колонке выравниваются пробелами — читается как
маленькая таблица.

**Не пытаться сжать в одну строку:**

* `value` с 3+ ключами (станет нечитаемо).
* `and`/`or` с 3+ predicate'ами (одна строка перевалит за 120 символов).

В таких случаях разворачивать на несколько строк весь блок как обычный JSON
(стандартный 2-space pretty-print, как для прочих сложных entries в `params[]`).

### Forward compatibility

Неизвестный ключ начинающийся с `#` → walker логирует warn и удаляет (graceful
degradation). Дисциплина: всё `#*` — control-construct, удаляется если walker
не знает. Позволяет вводить новые constructs (`#for_each`, `#include`, …) без
breaking change.

### Var-ref `@`-форма в outer `if`/`if_or`

Legacy outer `params[].if`/`params[].if_or` (и `vars[].if`/`vars[].if_or`,
`presets[].if`/`presets[].if_or`, etc.) — оставляем как есть **по семантике и
по имени ключа**. Меняется **нотация var-ref'ов внутри списков**: только
`"@var"`, bare `"var"` **запрещён**.

```jsonc
// БЫЛО (invalid после этой SPEC)
{"name": "inbounds", "if": ["enable_proxy_in"], "value": [...], "mode": "prepend"}
{"name": "param",    "if_or": ["use_yandex_dns", "use_cloudflare_dns"], "value": [...]}

// КАНОН
{"name": "inbounds", "if": ["@enable_proxy_in"], "value": [...], "mode": "prepend"}
{"name": "param",    "if_or": ["@use_yandex_dns", "@use_cloudflare_dns"], "value": [...]}
```

**Мотивация:**
* Нотация var-ref'ов унифицирована — всюду `@var`. Внутри `@name` substitute,
  внутри `#if.and/or`, внутри outer `if/if_or`. Снимается ambiguity «это
  string literal или var name?» (особенно важно когда имя var'а совпадает с
  literal'ом, например var `"tun"` vs string `"tun"`).
* Готовит почву к SPEC 068 (expression language в outer `if`/`if_or`, без
  переименования ключей schema).

**Validation (strict, без backward compat):**

Каждый элемент `if`/`if_or` **обязан** начинаться с `@`. Иначе — **loader
error** (template load fail):
```
template: entry `inbounds` has bare var-ref "tun" in if[]; use "@tun" form.
```

Lookup: strip `@`-prefix → имя в `varByName`. Unknown var после strip →
отдельная validation error.

Bare `"tun"` **не** принимается ни на load, ни в runtime. Кастомные шаблоны
без `@` — правка автором шаблона (breaking change для legacy bare-синтаксиса;
осознанно).

**Migration в `bin/wizard_template.json`:**

Bulk find/replace всех outer `if`/`if_or` arrays: каждый элемент → prefix `@`.
Делается одним commit'ом отдельной фазой (см. Phases ниже). Golden test
verifies output unchanged.

**Migration кастомных шаблонов (breaking changes для third-party):**

Пользователи с custom-template'ами должны до апгрейда проверить:

1. **Bare `if[]` / `if_or[]` элементы** — везде добавить `@`-prefix. Loader
   error на load иначе:
   ```
   template: entry `inbounds` has bare var-ref "tun" in if[]; use "@tun" form.
   ```
2. **Var с `name: "platform"` или `name: "arch"`** в `vars[]` — переименовать
   (collision с runtime globals). Loader error:
   ```
   template: vars[].name "platform" is reserved (runtime global); rename.
   ```
3. **Case-sensitivity:** strict lower-case для reserved (`platform`, `arch`),
   case-sensitive для var-имён везде (consistent с прежним поведением).
   `"Platform"` (capital P) технически НЕ reserved — но плохая практика,
   рекомендуется избегать confusing camelCase для template vars.

Эти три breaking changes документируются в release notes + триггерится
SPEC 046 invalidation: после bump'а `AppVersion` лаунчер на первом запуске
удалит старый `bin/wizard_template.json` → юзер скачает обновлённый из
`RequiredTemplateRef`. Для custom-шаблонов на диске — ручная правка автором.

### Инвалидация кэшированного шаблона при апгрейде (SPEC 046)

Локальный `bin/wizard_template.json` у существующих установок **несовместим**
с этим релизом (bare `if[]`, нет `#if`). Новый код **не** принимает старый
формат → wizard не загрузится, пока шаблон не заменён.

**Отдельный флаг не вводим.** Используем уже реализованный механизм SPEC 046:

| Механизм | Где | Поведение |
|---|---|---|
| `InvalidateTemplateIfStale(execDir)` | `main.go` (до UI) | Если `settings.last_template_launcher_version < AppVersion` и файл есть → **удалить** `bin/wizard_template.json` |
| `MarkTemplateInstalled(appVersion)` | после успешного Download Template | Пишет `last_template_launcher_version = AppVersion` |
| Dev skip | `isDevAppVersion` | `v-local-test`, `unnamed-dev`, `*-dirty` — **не** удаляют (локальная разработка) |

**Обязательно при закрытии этой SPEC (release checklist):**

1. **Релиз с bump `AppVersion`** (semver tag) — тогда у пользователей после
   апгрейда лаунчера на **первом запуске** старый шаблон удалится автоматически,
   UI покажет «Download Template» (один клик).
2. **`RequiredTemplateRef`** — bump по `docs/RELEASE_PROCESS.md` §5.2 вместе с
   релизом (CI ldflags + source-default в `constants.go`), чтобы Download Template
   тянул commit с `@`-only и `#if`.
3. **`docs/release_notes/upcoming.md`** — явный пункт: после апгрейда шаблон
   перекачается один раз (breaking template format).
4. **Ручная проверка:** симулировать upgrade — `last_template_launcher_version`
   меньше текущего `AppVersion`, файл `bin/wizard_template.json` есть → после
   старта файла нет (см. `core/template_migration_test.go`).

**Не делать:** тихо помечать `last_template_launcher_version` без удаления —
оставляет broken template на диске.

**Dev / `-i` install:** при локальной сборке invalidation пропускается; после
правок `wizard_template.json` в репо — удалить `bin/wizard_template.json`
вручную или пересобрать из bundled copy, иначе тестируешь stale файл.

**Уже в документации (SPEC 046, правки кода не нужны):**

| Документ | Что есть |
|---|---|
| [`docs/ARCHITECTURE.md`](../../docs/ARCHITECTURE.md) | `core/template_migration.go` + `InvalidateTemplateIfStale()` в дереве `core/`; в «Точки входа» — вызов из `main()` (SPEC 046) |
| [`docs/DATA_FLOW.md`](../../docs/DATA_FLOW.md) | Load flow: первый шаг `InvalidateTemplateIfStale` |
| [`docs/TEMPLATE_REFERENCE.md`](../../docs/TEMPLATE_REFERENCE.md) | § lifecycle на upgrade + таблица «Где лежит реализация» |
| [`docs/RELEASE_PROCESS.md`](../../docs/RELEASE_PROCESS.md) | §5.3 — поведение на апгрейде, `last_template_launcher_version` |

При закрытии 067 — **дополнить** эти файлы (см. § «Документация» ниже): cross-ref
на breaking format 067, исправить неточность в TEMPLATE_REFERENCE/DATA_FLOW
(сейчас там «compare RequiredTemplateRef vs marker» — фактически
`LastTemplateLauncherVersion` vs `AppVersion`).

## Документация (обновить при закрытии)

| Файл | Обязательно | Что добавить / исправить |
|---|---|---|
| [`docs/TEMPLATE_REFERENCE.md`](../../docs/TEMPLATE_REFERENCE.md) | **да** | Новый раздел **`#if` construct**: форма, map-spread / array-element, predicates, naming discipline (`#` vs bare vs `@`). Runtime globals `@platform` / `@arch`. `@`-only в outer `if`/`if_or`. Таблица «Где лежит реализация»: `substitute.go` — `#if` walker; `template_validate.go` — validate `#if` + outer refs. **Исправить** lifecycle-диаграмму: `LastTemplateLauncherVersion` vs `AppVersion`, не `RequiredTemplateRef`. Cross-ref SPEC 067. |
| [`docs/CREATE_WIZARD_TEMPLATE.md`](../../docs/CREATE_WIZARD_TEMPLATE.md) | **да** | Примеры `"if": ["@var"]`; параграф про `#if` для conditional fields; validation: bare `if[]` → error. |
| [`docs/CREATE_WIZARD_TEMPLATE_RU.md`](../../docs/CREATE_WIZARD_TEMPLATE_RU.md) | **да** | Зеркало EN (те же правила и примеры). |
| [`docs/ARCHITECTURE.md`](../../docs/ARCHITECTURE.md) | **да** | В блок **`core/template/`** (или рядом с `template_migration.go`): `substitute.go` — `#if` + predicates; `template_validate.go` — `#if` validation. У `InvalidateTemplateIfStale` — footnote: breaking template format (067) триггерится тем же bump `AppVersion`. Cross-ref SPEC 067. |
| [`docs/DATA_FLOW.md`](../../docs/DATA_FLOW.md) | **да** | Load flow §1: исправить описание `InvalidateTemplateIfStale`; после `ApplyParams` — шаг `#if` / substitute в `SubstituteVarsInJSON`. |
| [`docs/release_notes/upcoming.md`](../../docs/release_notes/upcoming.md) | **да** (релиз) | EN/RU: `#if`, `@` в `if[]`, proxy-in auth vars; one-time re-download шаблона после апгрейда. |
| [`RELEASE_NOTES.md`](../../RELEASE_NOTES.md) | по значимости UX | Краткий пункт для пользователей (conditional fields, template re-download) — если релиз user-visible. |
| [`docs/RELEASE_PROCESS.md`](../../docs/RELEASE_PROCESS.md) | опционально | §5.3: одна строка-пример — breaking template format → bump `AppVersion` (067). |
| [`docs/WIZARD_STATE.md`](../../docs/WIZARD_STATE.md) | нет | Out of scope (формат state не меняется). |

**Не трогать:** `docs/PRODUCT_ANALYSIS.md` (обзор, не спека формата).

## Что меняется по компонентам

### `core/template/substitute.go` (EDIT, ~+140 LOC)

`SubstituteVarsInJSON` приобретает параметры `goos`, `goarch` (для `#if`
predicates `@platform` / `@arch` и для `ParamIfSatisfied`, который проверяет
platform-conditional default vars). Все callers — передать `runtime.GOOS`,
`runtime.GOARCH` или fake в тестах.

`substituteWalk`:

**Map case** — pre-pass перед текущим циклом:

```go
case map[string]interface{}:
    // Control-constructs (keys starting with "#").
    for k, raw := range x {
        if !strings.HasPrefix(k, "#") {
            continue
        }
        switch k {
        case "#if":
            handleIfMapSpread(x, k, raw, varTypes, resolved, goos, goarch)
        default:
            debuglog.WarnLog("substitute: unknown control-construct %q — dropping", k)
            delete(x, k)
        }
    }
    // Normal field walk continues as before.
    for k, val := range x { substituteWalk(&val, ...); x[k] = val }
```

`handleIfMapSpread`:
1. Cast `raw` к `map[string]interface{}` (= "#if" body).
2. Extract `and`/`or`/`value`/`else`.
3. `evaluateIfBody(body, varTypes, resolved, goos, goarch)` → bool.
4. На true: pick `value`; substitute placeholders в нём; merge fields в `x`.
5. На false:
   - есть `else` → substitute, merge.
   - нет `else` → nothing.
6. Удалить ключ `"#if"` из `x` всегда.

**Array case** — pre-pass перед общим циклом:

```go
case []interface{}:
    out := make([]interface{}, 0, len(x))
    for _, elem := range x {
        if m, ok := elem.(map[string]interface{}); ok && len(m) == 1 {
            if body, ok := m["#if"].(map[string]interface{}); ok {
                // Array-element mode
                if branch, take := handleIfArrayElement(body, varTypes, resolved, goos, goarch); take {
                    out = append(out, branch)
                }
                continue
            }
        }
        substituteWalk(&elem, ...)
        out = append(out, elem)
    }
    *v = out
    // (single-element @var collapse при необходимости — отдельным условием)
```

`evaluateIfBody`:
1. Mutual exclusion check — должен быть РОВНО один из `and`/`or` непустой.
   (При validation time это error, в runtime — defensive: если обоих нет →
   true; если оба → false с warn-log.)
2. Substitute placeholders в args predicate'ов (через `substituteWalk`
   рекурсивно), НЕ substitute'я bare `"@var"` ключи.
3. Для каждого predicate — `evaluatePredicate(predicate, resolved, goos, goarch)`.
4. AND/OR short-circuit.

`evaluatePredicate` — switch по формам predicates. Lookup scalar:
1. `@platform` → `goos`; `@arch` → `goarch`.
2. Иначе template var из `resolved`.
Реализует type-checked lookup (`#notEmpty` на bool → `scalar=="true"` etc).
Bare `"@platform"` / `"@arch"` → false + defensive (validator ловит раньше).

### `core/template/vars_resolve.go` (EDIT, ~+10 LOC)

`ParamBoolVarTrue(name, varByName, resolved, goos)` — **требовать** `@`-prefix:
если `!strings.HasPrefix(name, "@")` → return false (defensive; validator
должен отловить раньше). После проверки — `name = strings.TrimPrefix(name, "@")`
для lookup в `varByName`. Вся остальная логика `ParamIfSatisfied` /
`ParamIfOrSatisfied` без изменений семантики.

### `core/template/template_validate.go` (EDIT, ~+90 LOC)

Новые функции:

* `validateIfConstruct(rawValue json.RawMessage, varByName)` — рекурсивный walk
  любого `value`/`default`/etc, ищет `#if` объекты, валидирует:
  - Ровно один из `and`/`or` присутствует и непустой list.
  - `value` field present (не nil).
  - `vars[]` не содержит `name` `platform` / `arch`.
  - Каждый predicate в `and`/`or`:
    - Bare string — template bool var; `"@platform"` / `"@arch"` → error.
    - Single-key object — key `@varname`: `platform`/`arch` → text predicates only;
      иначе var exists, value form valid, type-predicate compat.
    - `#matches` regex компилируется (`regexp.Compile` без ошибок).
    - `#in` / `#notIn` — args это []string (либо может быть `@text_list_var`).
    - `#not` — inner predicate present, рекурсивно валидируется.
* Любой `#`-key, не равный `#if` → warning (НЕ error, forward-compat).
* `validateOuterIfRefs(ifList []string, varByName, context string)` — для outer
  `if`/`if_or` (и `vars[].if`, `presets[].if`, etc.): каждый элемент **должен**
  начинаться с `@`. Bare → **error** с указанием context (entry/var/preset id).
  После strip `@` — var exists, type=bool.

Вызывать из `loader.go` после parse каждой entry, по всем `value`s и `default`s.

### `bin/wizard_template.json` (EDIT)

1. Добавить 3 var'а в `vars[]` (после `proxy_in_set_system_proxy`):

```json
{
  "name": "proxy_in_auth_enabled",
  "type": "bool",
  "default_value": "false",
  "wizard_ui": "edit",
  "if": ["@enable_proxy_in"],
  "title": "Proxy-in require authentication",
  "tooltip": "When enabled, clients connecting to mixed inbound proxy-in must provide username and password."
},
{
  "name": "proxy_in_username",
  "type": "text",
  "default_value": "",
  "wizard_ui": "edit",
  "if": ["@enable_proxy_in", "@proxy_in_auth_enabled"],
  "title": "Proxy-in username",
  "tooltip": "Username for mixed inbound proxy-in authentication."
},
{
  "name": "proxy_in_password",
  "type": "text",
  "default_value": "",
  "wizard_ui": "edit",
  "if": ["@enable_proxy_in", "@proxy_in_auth_enabled"],
  "title": "Proxy-in password",
  "tooltip": "Password for mixed inbound proxy-in authentication."
}
```

2. Обернуть `users` поле в существующем `inbounds`-entry для proxy-in:

```json
{
  "name": "inbounds",
  "if": ["@enable_proxy_in"],
  "value": [{
    "type": "mixed",
    "tag": "proxy-in",
    "listen": "@proxy_in_listen",
    "listen_port": "@proxy_in_listen_port",
    "set_system_proxy": "@proxy_in_set_system_proxy",
    "#if": {
      "and": ["@proxy_in_auth_enabled"],
      "value": {
        "users": [{"username": "@proxy_in_username", "password": "@proxy_in_password"}]
      }
    }
  }],
  "mode": "prepend"
}
```

### `core/template/substitute_test.go` (EDIT)

Минимум 11 тестов:

**Walker semantics:**

1. `TestIf_MapSpread_AndTrue_NoElse_Merges` — `and:[bool true]`, value есть, else
   нет → fields из value мерджатся в parent.
2. `TestIf_MapSpread_AndFalse_NoElse_RemovesKey` — `and:[bool false]`, value
   есть, else нет → ключ `#if` исчезает, parent unchanged.
3. `TestIf_MapSpread_AndFalse_WithElse_MergesElse` — `and:[bool false]`, есть
   else → fields из else мерджатся в parent.
4. `TestIf_ArrayElement_OrTrue_Replaces` — `or:[bool true]` → элемент заменяется
   на value.
5. `TestIf_ArrayElement_OrFalse_NoElse_Drops` — `or:[bool false]`, нет else →
   элемент удаляется (массив -1).
6. `TestIf_ArrayElement_OrFalse_WithElse_Replaces` — `or:[bool false]`, есть
   else → элемент заменяется на else.
7. `TestIf_Nested_OuterTrueInnerTrue` — вложенный `#if` внутри outer `value`,
   оба true → внутренний branch применён.

**Expression language:**

8. `TestIf_Predicate_Equality` — `{"@var": "literal"}` true когда scalar match,
   false когда не.
9. `TestIf_Predicate_NotEmpty_TextVar` — `#notEmpty` на text var с пустой и
   непустой строкой.
10. `TestIf_Predicate_In_TextList` — `#in: [...]` true когда scalar в списке,
    false когда нет.
11. `TestIf_Predicate_Matches_Regex` — `#matches: "^[a-z]+$"` true/false.
12. `TestIf_Predicate_Not_BoolVar` — `{"#not": "@bool_var}"` инвертирует bool.
13. `TestIf_Predicate_Not_Nested_In` — `{"#not": {"@var": {"#in": [...]}}}`
    эквивалентно `#notIn`.
14. `TestIf_Predicate_Not_DoubleNegation` — `{"#not": {"#not": "@var"}}` дважды
    отрицает, эквивалентно бare `"@var"`.

**Runtime globals (`@platform`, `@arch`):**

15. `TestIf_Predicate_Platform_In` — `{"@platform": {"#in": ["darwin","linux"]}}`
    true/false по fake goos.
16. `TestIf_Predicate_Arch_Equality` — `{"@arch": "386"}` на fake goarch.
17. `TestIf_Predicate_Platform_BareString_LoaderError` — bare `"@platform"` в
    `and[]` → validation error.

**Edge cases (рекомендуется):**

18. `TestIf_UnknownBangKey_Dropped` — `"#foo": anything` → warn + drop, остальное
    untouched.
19. `TestIf_ValueIsScalar_InMapSpread_LoaderError` — validator отказывается
    грузить (отдельный validate тест).
20. `TestIf_BothAndOr_LoaderError` — validation error.
21. `TestIf_NeitherAndOr_LoaderError` — validation error.
22. `TestIf_NotWithoutInner_LoaderError` — `{"#not": null}` или `{"#not": {}}` →
    validation error «`#not` requires inner predicate».
23. `TestIf_ReservedVarName_platform_LoaderError` — `vars[]` с `name: "platform"`.

**Outer `if`/`if_or` `@`-only:**

24. `TestOuterIf_AtPrefix_Works` — `"if": ["@tun"]` evaluate'ится корректно.
25. `TestOuterIf_BareVarRef_LoaderError` — bare `"tun"` в `if`/`if_or` →
    validation error при load.
26. `TestOuterIf_UnknownVar_LoaderError` — `"if": ["@nonexistent"]` → error.
27. `TestOuterIf_PlatformGlobal_LoaderError` — `"if": ["@platform"]` → error
    (globals только в `#if` predicates).

### `core/template/template_validate_test.go` (EDIT)

Покрыть:
* `#if` валидация type-checks (bare-bool на text-var → error и т.п.).
* Unknown `#*` ключ → warning, не error.
* `#matches` с invalid regex → error.
* Empty `and` или `or` → error.
* Bare var-ref в outer `if`/`if_or` → error.
* Reserved `vars[].name` `platform` / `arch` → error.

### `internal/locale/*.json` (нет изменений)

Template-driven UI уже читает `title`/`tooltip` из самого template — доп.
локали не нужны.

## Edge cases

* **`value` отсутствует, condition true** — error на validation (template
  load). `value` обязательный.
* **`value` это `null`** — legal JSON null. Behavior: в map-spread null нельзя
  merge'нуть (validation error: must be object). В array-element элемент
  заменяется на null (legal).
* **`else` это `null`** — то же что и `value: null`. В map-spread error, в
  array-element элемент = null.
* **Empty `and` / `or` list** (`"and": []`) — на validation: error «and must
  be non-empty». Если в runtime каким-то образом — defensive: пустой `and` →
  vacuously true; пустой `or` → vacuously false. Сразу error в validator.
* **Predicate references unknown var** — validation error: `predicate references
  unknown var @foo in entry inbounds`.
* **Циклическая зависимость** через `@var` в predicate args — substitute walker
  single-pass, `@var` в args резолвится из `resolved` map. Циклов быть не должно.
* **`@platform` / `@arch` в placeholder substitution** (вне `#if` predicates) —
  не поддерживаются; `@platform` в JSON string value substitute'ится как unknown
  var → unresolved warning (как любой необъявленный `@name`).

## Tests — acceptance criteria

### Unit

* Walker + predicate + **runtime globals** + validation тесты зелёные (см. §
  `substitute_test.go`).
* Существующие substitute / template_validate тесты НЕ ломаются.
* Golden config diff после template применения — diff показывает ТОЛЬКО:
  * 3 новых var'а в `vars[]`.
  * `inbounds[0]` для proxy-in идентичен legacy когда auth_enabled=false.
  * `inbounds[0]` имеет `users` поле когда auth_enabled=true.

### Manual matrix

| `enable_proxy_in` | `auth_enabled` | username | password | inbound proxy-in |
|---|---|---|---|---|
| false | — | — | — | НЕ эмитится |
| true | false | — | — | без `users` (anonymous) |
| true | true | заполнен | заполнен | с `users:[{u,p}]` (auth required) |
| true | true | пусто | пусто | `users:[{"":""}]` — broken state; UI должен предотвращать (out of scope этой SPEC, отдельная UI задача) |

## Phases — implementation order

1. **Walker extension** — `substitute.go`: `#if` + predicates + **`@platform` /
   `@arch`** (`goos`, `goarch`); synthetic templates в тестах (в т.ч. platform/arch
   predicates). Без правок `wizard_template.json` на этом шаге.
2. **Validator** — `template_validate.go` + validation тесты (в т.ч. reserved
   `platform`/`arch`, bare `"@platform"` в `and[]`).
3. **Var-ref `@`-only в outer `if`/`if_or`** — `ParamBoolVarTrue` требует `@`;
   `validateOuterIfRefs` — error на bare. Доп. тесты: `@` работает, bare →
   loader error.
4. **Template применение** — `wizard_template.json`:
   - **4a:** добавить 3 var'а (`proxy_in_auth_enabled`/`username`/`password`).
   - **4b:** обернуть `users` в `#if`.
   - **4c:** bulk find/replace всех `if`/`if_or` элементов: prefix `@`.
     Golden test verify output unchanged.

   ⚠ **Phase 3 + 4c — атомарны (один commit, не разнести).** Phase 3 включает
   strict validation `@`-only в outer `if`/`if_or`. После phase 3 без 4c
   `bin/wizard_template.json` (с bare refs) не загружается → `go test ./...`
   падает на всех тестах что грузят template. Phase 4c — bulk find/replace —
   обязательно идёт в том же commit'е что phase 3, иначе бранч в красном
   между ними. Phase 4a/4b могут отдельным commit'ом (они не зависят от
   validation strict mode).
5. **Manual reinstall** — `./build/build_darwin.sh -i arm64` → ручная проверка
   матрицы.
6. **Документация** — по таблице § «Документация» (минимум:
   `TEMPLATE_REFERENCE.md`, `CREATE_WIZARD_TEMPLATE*.md`, `ARCHITECTURE.md`,
   `DATA_FLOW.md`).
7. **Release notes + commit** — pack изменения, обновить `upcoming.md`
   (пункт про one-time re-download шаблона после апгрейда). **Release checklist:**
   bump `AppVersion` + `RequiredTemplateRef` — без bump invalidation не сработает
   у пользователей на той же версии лаунчера.

8. **Phase 8 — preset substitute unification (post-Phase-7 amendment).**
   Replace `core/build/preset_expand.go::substituteAny` (and its callsites in
   `preset_outbounds.go`, `resolve_dns.go`) with `template.SubstituteVarsInJSON`,
   so `#if` and the expression language work in preset bodies too.
   `ExpandPreset` / `ExpandPresetOutbounds` / `PresetOutboundAddByTag`
   signatures gain `goos, goarch` (runtime path: `runtime.GOOS` /
   `runtime.GOARCH`; tests: fakes). New strict variant
   `template.SubstituteVarsInJSONStrict` returns `UnresolvedVarError` on
   unresolved `@var` — preserves the legacy "skip preset entirely" semantic
   that `substituteAny` had via its `ok=false` return. Enables collapse of
   platform-split presets (`split-all-traffic-mac-win` +
   `split-all-traffic-linux`, DNS bundled variants) — actual collapse is a
   follow-up template-cleanup commit, not in this phase.

## Open questions / future extensions

### Predicates v2 (когда понадобятся)

| Predicate | Семантика | Args |
|---|---|---|
| `#gt: N` | numeric `>` | scalar |
| `#lt: N` | numeric `<` | scalar |
| `#gte: N` / `#lte: N` | numeric `>=` / `<=` | scalar |
| `#startsWith: "p"` | string prefix | scalar |
| `#endsWith: "s"` | string suffix | scalar |

Сейчас не реализуем — YAGNI до конкретного use case. Negation покрыта `#not`
(уже в v1).

### `#if` в outer `params[].if` / `params[].if_or`

Сейчас outer `params[].if`/`params[].if_or` — список `@var` bool-ref'ов.
Возможно симметрии ради завести там тоже expression language (принимать
predicates). Не сейчас — separate SPEC. Ключи `if`/`if_or` и bare var names
без `@` — не поддерживаются (breaking vs pre-067 templates).

### `#if` outside `params[].value`

Например в `params[].vars[].default_value` или `params[].select`. Walker
формально его обработает (если вызывается через `SubstituteVarsInJSON`). Не
запрещаем, но и не cover'им тестами в v1.

### Schema cleanup: consolidate platform-split entries (follow-up SPEC)

После landing'а 067 в шаблоне останутся дубликаты `params[]` / `presets[]`,
различающиеся только по `platforms[]` фильтру (и одним-двумя полями внутри
`value`). Кандидаты для схлопывания через `#if` + `@platform`:

* `params[].name="inbounds"` для TUN (windows+linux vs darwin) — см. Real-
  world case в § Runtime globals.
* `presets[].id="split-all-traffic-mac-win"` + `split-all-traffic-linux` —
  один preset с `source_port_range` через `#if` на `@platform`.
* DNS preset bundled варианты (вероятно есть platform-split, проверить).

Не в scope этой SPEC — отдельная **mini-SPEC после 067** landing'а: один
sweep по шаблону, audit всех `platforms[]` entries, схлопнуть где
осмысленно. Чисто template-side cleanup, без changes в Go-коде.

### Mobile parity

LxBox (мобильный лаунчер) использует свой template parser, про `#if` и `@` в
`if[]` не знает. Если template шарится — mobile увидит ключ `#if` и/или
откажется от bare `if[]` (если mobile тоже ужесточит правила). **План:** в
`TEMPLATE_REFERENCE.md` пометить раздел про `#*` constructs как
desktop-only до подтяжки реализации в мобиле. Шаблон должен helmet'ить
платформы которых поддержка ещё нет. Кастомные шаблоны с bare `if[]` —
правка на `@` обязательна для desktop после этой SPEC.
