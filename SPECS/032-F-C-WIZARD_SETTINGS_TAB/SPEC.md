# SPEC: WIZARD_SETTINGS_TAB — вкладка «Settings» в визарде

**Статус:** C (complete). **Тип:** F (feature).

**Объём:** **SPEC.md**, **PLAN.md** (в т.ч. **раздел 13 — решения**), **TASKS.md** (чеклист этапов), **IMPLEMENTATION_REPORT.md**.

**Итог реализации:** папка **SPECS/032-F-C-WIZARD_SETTINGS_TAB/**; отчёт — **IMPLEMENTATION_REPORT.md**.

---

## Проблема (исходная формулировка)

До реализации часть полей итогового `config.json` задавалась только `wizard_template.json`; TUN для macOS включался галочкой на **Rules** (`EnableTunForMacOS` / `config_params.enable_tun_macos`), хотя по смыслу это системная настройка. Сейчас это закрыто вкладкой **Settings** и **`vars`** (см. **Цель** и **IMPLEMENTATION_REPORT.md**).

## Цель

Вкладка **Settings** (EN *Settings*, RU по CONSTITUTION) — **предпоследняя** перед **Preview**: Sources → Outbounds → DNS → Rules → **Settings** → Preview. На ней — настройки уровня ядра / experimental / TUN, с записью в состояние визарда и участием в превью и итоговом `config.json`.

Состав и смысл переменных задаёт только **`wizard_template.json`**. В **`state.json`** массив **`state.vars`**: лишь переопределения пользователя (**`name`** + **`value`**), без отдельной схемы переменных. Сборка конфига по-прежнему через `config`, `params`, `GetEffectiveConfig` (macOS, TUN).

**Качество поставки:** реализация сразу **полноценная** — аккуратный UI, миграции state, краевые случаи и документация по SPEC/PLAN; намеренно «урезать на потом» без причины не нужно.

---

### Шаблон: `vars` и плейсхолдеры `@…`

Объявления — в корне шаблона, ключ **`vars`**. Строки **`@<имя>`** разрешены **только** в **`config`** и **`params`** (см. ниже); в остальных местах шаблона их не ждём.

#### Объявление переменной

- **`vars`** — массив объектов; один такой ключ на корень шаблона.
- **`name`** — единый идентификатор: объявление, **`state.vars`**, плейсхолдер **`@<name>`**, массив **`if`** у **`params`**. Лексика **`name`** и то, как маркер ищется в JSON, — в подразделе **«Имя переменной и маркер `@<name>`»**. Регистр фиксирован; в шаблоне имена уникальны (среди **переменных** с непустым **`name`**); в **state** — не больше одной записи на **`name`**.
- **Разделитель:** элемент **`{"separator": true}`** — не переменная: без **`name`**, без **`@…`**, не попадает в **`state.vars`** и не участвует в **`VarIndex`** / **`ResolveTemplateVars`**; на вкладке **Settings** — горизонтальная линия между строками. Ограничения полей и опциональные **`platforms`** / **`wizard_ui: hidden`** — в **docs/CREATE_WIZARD_TEMPLATE.md** (EN/RU).

Поля элемента **`vars`** (для обычной переменной; см. строку **`separator`**):

| Поле | Назначение |
|------|------------|
| **`separator`** | Если **`true`** — только оформление **Settings** (линия между строками). **Не** сочетать с **`name`**, **`type`**, **`default_value`**, **`if`** и т.д. (валидация шаблона). Опционально **`platforms`**, **`wizard_ui`**: только **`hidden`**. |
| **`name`** | Обязательно для переменной (не для **`separator`**). Пример: `tun_address` ↔ `@tun_address`. |
| **`default_node`** | Опционально. Путь от корня шаблона к литералу (точечная нотация), напр. `config.log.level`. Приведение bool/числа к строке, циклы `@…` в узле — PLAN. |
| **`default_value`** | Опционально. **Скаляр** (строка/число/bool в JSON) **или JSON-объект** с ключами платформы (**`linux`** / **`darwin`** / **`windows`**, псевдоним **`win7`** для **windows/386**, **`default`**) — разрешение **`VarDefaultValue.ForPlatform`** (**`ui/wizard/template/vars_default.go`**); семантика как у **`platforms`**, см. **docs/CREATE_WIZARD_TEMPLATE.md**. Иначе — литерал в объявлении по прежним правилам. Если нет записи в **`state.vars`**, порядок: **`default_value`**, затем **`default_node`**. Для **`type: text_list`** литерал / объект / узел — PLAN. |
| **`type`** | Обязательно. **`text`** — **одна** строка; однострочное поле на Settings; формат (CIDR, `host:port`, …) в **`comment`**; отдельных типов `cidr` / `host_port` нет. В шаблоне **`@name`** — одна строка или **один** элемент строкового массива (пример: `"address": ["@tun_address"]`). **`text_list`** — переменная — **массив строк** (`[]string`): на Settings — **список / пачка строк** (виджет — PLAN); в **`state.vars`** одна запись **`name`**, в **`value`** — закодированный список (формат — PLAN); в шаблоне **`@name`** стоит в узле, который после подстановки становится **JSON-массивом** строк (синтаксис плейсхолдера — PLAN). **`bool`**, **`enum`** — один скаляр (в конфиге как строка — PLAN). **`enum`** требует **`options`**: массив строк. **`custom`** — без автостроки Settings; UI в коде (PLAN). |
| **`wizard_ui`** | Опционально: **`edit`** \| **`view`** \| **`hidden`**. Default — PLAN (для настроечных имён разумно `edit`). |
| **`platforms`** | Опционально. На каких ОС показывать строку Settings. **Не** то же самое, что **`platforms`** у **`params`**. |
| **`comment`** | Подпись/tooltip и подсказка автору шаблона (CONSTITUTION / локали — PLAN). Для **`text_list`** — формат элементов списка (напр. одна CIDR на строку). |

После разрешения: **`text`**, **`bool`**, **`enum`**, **`custom`** → одна строка (bool как `"true"`/`"false"` — PLAN). **`text_list`** → **[]string**; парсинг **`value`** и дефолтов — PLAN.

**Пример фрагмента `vars`:**

```json
"vars": [
  {
    "name": "tun_address",
    "type": "text",
    "default_value": "172.16.0.1/30",
    "wizard_ui": "edit",
    "comment": "Формат: CIDR TUN; в params: \"address\": [\"@tun_address\"]"
  },
  {
    "name": "clash_api",
    "type": "text",
    "default_node": "config.experimental.clash_api.external_controller",
    "default_value": "127.0.0.1:9090",
    "wizard_ui": "edit",
    "comment": "Формат: host:port (Clash external_controller)."
  },
  {
    "name": "log_level",
    "type": "enum",
    "default_node": "config.log.level",
    "default_value": "warn",
    "options": ["trace", "debug", "info", "warn", "error", "fatal", "panic"],
    "wizard_ui": "edit",
    "comment": "sing-box log.level"
  },
  {
    "name": "tun",
    "type": "bool",
    "default_value": "true",
    "wizard_ui": "edit",
    "platforms": ["darwin"],
    "comment": "Галочка TUN на macOS; TUN inbounds: platforms darwin + \"if\": [\"tun\"]"
  },
  {
    "name": "internal_provider_marker",
    "type": "text",
    "default_value": "acme-corp",
    "wizard_ui": "hidden",
    "comment": "Метка для @… и if"
  },
  {
    "name": "feature_x_flag",
    "type": "bool",
    "default_value": "false",
    "wizard_ui": "hidden",
    "comment": "Без автостроки Settings"
  }
]
```

**`tun`:** в целевой модели заменяет **`EnableTunForMacOS`** и **`config_params.enable_tun_macos`**; **`@tun`** в JSON не подставляется. TUN inbound на macOS — **`params`** с **`"platforms": ["darwin"]`** и **`"if": ["tun"]`** (**`GetEffectiveConfig`**, PLAN, миграция). Платформа **`darwin-tun`** не используется.

**macOS (UI):** снятие галочки у переменной с **`name`**: **`tun`** на вкладке **Settings** — только если ядро **остановлено** (**`RunningState`** лаунчера); иначе сообщение и галка не меняется. После остановки при необходимости — одно привилегированное **`rm -rf`**: **`experimental.cache_file.path`** внутри **`bin/`** (если есть в шаблоне и файл существует) и логи ядра **`logs/sing-box.log`** / **`logs/sing-box.log.old`** под **`ExecDir`** при их наличии (см. **docs/CREATE_WIZARD_TEMPLATE.md** / **_RU.md**, **docs/WIZARD_STATE.md**, **`settings_tun_darwin.go`**). Имя **`tun`** в коде зафиксировано для этой ветки.

#### Где допускается `@…`

В **`wizard_template.json`** подстановка **`@<name>`** (после объявления в **`vars`**) только в:

1. **`config`** — узлы, где допустима строка или элемент строкового массива (белый список путей — PLAN).
2. **`params`** — значения, попадающие в итоговый JSON sing-box (те же ограничения — PLAN).

Вне **`config`** и **`params`** лаунчер плейсхолдеры **не** подставляет: **`parser_config`**, вложенные **`wizard`**, прочие секции. В **`vars`** плейсхолдеров для мержа нет — только литералы и метаданные.

В **`state.json`** синтаксис **`@…`** **не** используется. В **`state.vars`** поле **`value`** — строка; для переменных с **`type: text_list`** в ней хранится закодированный **список** строк (формат — PLAN).

В **`params[].if`** перечисляются **имена** переменных (`"if": ["foo"]`), не строки **`@foo`**.

#### Имя переменной и маркер `@<name>`

**Лексика `name`:** идентификатор в **`vars[].name`** и в маркере **`@<name>`** — один и тот же токен. Рекомендуемый синтаксис: **`[A-Za-z_][A-Za-z0-9_]*`** (латиница, цифры, подчёркивание; первая позиция — не цифра). Расширение набора символов или запрет коллизий с зарезервированными словами — валидация шаблона (PLAN).

**Подстановка по конкретным переменным, не по маске `@…`:** реализация **не** сканирует строки регексом вида **`@` + «что угодно похожее на имя»** — так легко задеть **`user@host`**, URL и прочие **`@`** вне шаблонных маркеров. Вместо этого для **каждого** объявленного в **`vars`** **`name`** ищется **только** литерал **`@`** + **это же** **`name`** целиком, в разрешённых местах **`config`** / **`params`** (например значение JSON-строки **ровно** `"@tun_address"`, либо позиции для **`text_list`** — PLAN). Маркер **`@something`**, для которого нет объявления в **`vars`**, в шаблоне не допускается: валидация при загрузке или **warn** и правило в PLAN.

**Префиксы имён:** если в шаблоне теоретически возможны **`tun`** и **`tun_address`**, сопоставление должно исключать подстановку короткого имени «внутри» длинного маркера (например обход **`name`** в порядке убывания длины или только сравнение целых строковых литералов — PLAN).

#### Сироты в **`state.vars`**

**State** не задаёт набор переменных: метаданные (**`type`**, …) всегда из шаблона. Запись, чей **`name`** не объявлен в текущем шаблоне, при загрузке отбрасывается; при сохранении в файл снова попадают только имена из шаблона — сироты исчезают без отдельного флага.

#### UI вкладки Settings

В **`state.json`** по-прежнему только **`{ "name", "value" }`**; режима «как в шаблоне» в файле нет.

Для строки с **`wizard_ui: edit`** и наличием дефолта (**`default_value`** и/или **`default_node`**) в памяти ведётся признак «ещё не переопределяли» (имя поля в модели — PLAN). На диске это то же самое, что **отсутствие** записи в **`state.vars`** с данным **`name`**. Отдельной галки «по умолчанию» пользователю не показываем.

Пока записи в **state** нет: в поле показывается значение из цепочки шаблона (см. ниже). Нужно ли блокировать ввод до фокуса — PLAN.

Как только пользователь меняет значение, признак сбрасывается, в **`state.vars`** появляется или обновляется запись, рядом — **«Сброс»** (локали — CONSTITUTION / PLAN). **Сброс** удаляет запись и возвращает отображение к шаблону.

Пока запись есть, **`value`** — источник итога, в том числе **`""`**. Вернуться к шаблону можно только **Сбросом**, не «пустым» значением в файле.

Без дефолта в объявлении (**ни** **`default_value`**, **ни** **`default_node`**): поведение кнопки «Сброс» и поля — PLAN. **`wizard_ui: hidden`** / **`view`** — PLAN.

#### Порядок разрешения значения

Для каждого объявления в **`vars`** по **`name`**:

1. Найти объявление в шаблоне (без него переменной нет).
2. Есть ли в **`state.vars`** запись с этим **`name`**?
   - **Да** — распарсить **`value`** по **`type`**: **`text`** / **`bool`** / **`enum`** / **`custom`** → одна строка; **`text_list`** → **[]string** (правила — PLAN). Цепочка заканчивается.
   - **Нет** — **`default_value`** / **`default_node`** по тем же правилам. Иначе пусто: для скалярных типов — `""`; для **`text_list`** — пустой массив (PLAN).

Если заданы и **`default_value`**, и **`default_node`**, при отсутствии записи в **state** сначала используется **`default_value`**, затем **`default_node`** — это уже следует из шагов выше.

**`state.vars`:** только ключи **`name`**, **`value`**. Дубликаты **`name`** запрещены. Метаданные переменной только в шаблоне. Лишние ключи в элементах **`vars`** шаблона или в **state** — отбросить или отклонить снимок (строгость — PLAN).

**Миграция (PLAN):** старый **`use_default`** в state; пустой **`value`** как «нет переопределения»; в шаблоне типы **`cidr`** / **`host_port`** / текстовый **`string`** → **`text`**; устаревший флаг **`is_array: true`** → **`type: text_list`** (и убрать **`is_array`**).

**`default_node`** на узлы внутри массива **`params`** — синтаксис пути (JSON Pointer и т.д.) — PLAN.

#### Пустой итог и подстановка `@…`

**`@tun_address`** в шаблоне — не значение sing-box, а метка замены. В итоговом JSON после подстановки не должно оставаться **незаменённых** литералов **`@…`**.

**Подстановка после мержа** (`applyParams` / `GetEffectiveConfig`): для **каждого объявленного** **`name`** находятся маркеры **`@`** + **`name`** (как выше — **не** общий поиск по `@`); подставляется разрешённое значение согласно **`type`**: **`text`** / **`bool`** / **`enum`** / **`custom`** — **одна строка** (в позицию строки в JSON или одного элемента массива — как в шаблоне); **`text_list`** — **массив строк** в узел под массив (синтаксис в шаблоне — PLAN).

Если после разрешения значение **пустое**: **warn** в лог (CONSTITUTION, **`internal/debuglog`**), **`name`** и контекст; для скалярных типов на место **`@<name>`** — `""`; для **`text_list`** — пустой массив или правило узла (PLAN). **Save** / **Preview** не блокируются. Сырых **`@…`** в выдаче нет.

Переменная без участия в **`@…`** (только **`if`** и т.д.) может иметь пустой итог без подстановки.

**Сводка:** массив строк задаётся типом **`text_list`**, не отдельным флагом. Запись в **state** = переопределение; для **`text`** значение **`""`** допустимо. Нет записи = дефолты шаблона.

#### Шаблоны без новых возможностей

Нет секции **`vars`** и нет **`@…`** — поведение как в текущей версии.

#### Расширение

Переменные для **`mixed`**, [Listen Fields](https://sing-box.sagernet.org/configuration/shared/listen/#structure) — PLAN.

#### Условные **`params`**: **`if`** (AND) и **`if_or`** (OR)

К записи **`params`** опционально добавляется **`if`**: массив имён из **`vars`**. Запись применяется, если совпала платформа **и** все перечисленные переменные истинны **на текущей ОС** (учёт **`vars[].platforms`** — PLAN). Опционально **`if_or`**: истинна **хотя бы одна** из перечисленных bool-переменных; **`if`** и **`if_or`** в одной записи не сочетаются.

**Уточнение (контракт):** для каждого имени в **`if`** / **`if_or`** сначала проверяется **`VarAppliesOnGOOS(vars[].platforms, runtime.GOOS)`** (пустой список **`platforms`** → переменная на всех ОС; иначе только совпадение с **`runtime.GOOS`**, без отдельной метки **`win7`** в шаблоне — legacy-сборка Win7 это **windows/386**). Если переменная **не** объявлена на текущей ОС, она для условия считается **ложной** (как «нет истинной переменной»), **независимо** от строки в **`state.vars`** и от **`ResolveTemplateVars`**. Так **`tun_builtin`** с **`platforms`: [`windows`, `linux`]** на **darwin** не даёт истины в **`if_or`**, а **`tun`** только под **darwin** на **linux** не проходит **`if`**. Код: **`ParamBoolVarTrue`** / **`ParamIfSatisfied`** / **`ParamIfOrSatisfied`** в **`ui/wizard/template/vars_resolve.go`**; тесты **`TestParamBoolVarTrue_respectsVarPlatforms`**, **`TestParamIfSatisfied_falseWhenVarNotOnGOOSEvenIfResolvedTrue`**, **`TestParamIfSatisfied_AND_falseWhenOneOperandNotOnGOOS`**.

Без **`if`** / **`if_or`** — фильтр только по **`platforms`**. Для macOS TUN: переменная **`tun`** (**`type`: `bool`**, **`platforms`: [`darwin`]**) и блок TUN-**`inbounds`** с **`"platforms": ["darwin"]`**, **`"if": ["tun"]`** (см. **`bin/wizard_template.json`**).

**Порядок сборки:** разрешить все переменные → отфильтровать **`params`** (платформа, **`if`** / **`if_or`**) → мерж → заменить **`@…`**.

**Пример (`if_or` для `route.rules`):**

```json
{
  "name": "route.rules",
  "platforms": ["windows", "linux", "darwin"],
  "if_or": ["tun_builtin", "tun"],
  "mode": "prepend",
  "value": [
    { "inbound": "tun-in", "action": "resolve", "strategy": "prefer_ipv4" },
    { "inbound": "tun-in", "action": "sniff", "timeout": "1s" }
  ]
}
```

#### Реализация (сводно)

Загрузка **`vars`**, `TemplateData`, подстановка **`@…`**, **`if`**, **`state.vars`** — PLAN/TASKS. DoD: **docs/WIZARD_STATE.md**, **docs/CREATE_WIZARD_TEMPLATE.md**.

---

## Целевой набор переменных **`vars`**

Имена в шаблоне — **`snake_case`**, латиница (см. лексику **`name`**). Ниже — **полный** набор переменных под Settings (первая поставка фичи, без урезания); соответствие рабочим названиям — в колонке «Смысл».

| **`vars[].name`** | Смысл (ваши имена) | **`type`** | **`platforms`** | Куда попадает в конфиг | **`@…`** | Дефолт / примечание |
|-------------------|-------------------|------------|-----------------|-------------------------|----------|---------------------|
| **`tun_address`** | tunAddress, CIDR TUN | `text` | не задано (все ОС, где есть строка TUN в UI) | **`params`**, TUN `address[]` | `@tun_address` | из шаблона / **`default_node`** — PLAN |
| **`tun_mtu`** | mtu | `text` | не задано | **`params`**, TUN `mtu` (число) | `@tun_mtu` | строка в **`value`**, в JSON — **число** (парсинг — PLAN); текущий шаблон: `1492` |
| **`tun`** | isTun под Mac | `bool` | **`["darwin"]`** | только **`if`** / **`if_or`** вместе с **`darwin`** **params** | — | **`@tun`** не подставляется |
| **`mixed_listen_port`** | proxy-port = `listen_port` (mixed, только macOS-ветка без TUN) | `text` | **`["darwin"]`** | **`params`**, mixed inbound `listen_port` | `@mixed_listen_port` | формат: порт (число), см. **`comment`**; узел — запись **`inbounds`** с **`platforms`: [`darwin`]** (см. **`bin/wizard_template.json`**) |
| **`log_level`** | log_level | `enum` | не задано | **`config.log.level`** | `@log_level` | уровни — по ядру (PLAN) |
| **`clash_api`** | Clash API = `external_controller` | `text` | не задано | **`config.experimental.clash_api.external_controller`** | `@clash_api` | формат **`host:port`** в **`comment`** |
| **`clash_secret`** | clash_key, секрет Clash API | `text` | не задано | **`config.experimental.clash_api.secret`** | `@clash_secret` | **Дефолт при первом применении шаблона / создании профиля** (PLAN): случайная строка **16** символов из **`[A-Za-z0-9]`** (буквы разного регистра и цифры); не писать в логи; показ в UI — осторожно (CONSTITUTION). Плейсхолдер вроде `CHANGE_THIS_…` в шаблоне заменяется на сгенерированный или на значение из **`state.vars`**. |

**Примеры в этом SPEC** с именем **`external_controller`** считать устаревшими для новых шаблонов: использовать **`clash_api`** / **`@clash_api`** (одно и то же поле, что `experimental.clash_api.external_controller`).

---

## Область данных (sing-box)

### 1. `experimental.clash_api.external_controller`

**UI:** **`type: text`**, **`name`**: **`clash_api`** (см. таблицу выше). Формат **`host:port`** в **`comment`**. **Шаблон:** **`@clash_api`**. Не ломать **`secret`** и остальные ключи **`clash_api`**.

### 1b. `experimental.clash_api.secret`

**UI:** **`type: text`**, **`name`**: **`clash_secret`**. **Шаблон:** **`@clash_secret`**. Дефолт — случайная строка 16 символов **`[A-Za-z0-9]`** при инициализации (PLAN); безопасность и отображение — CONSTITUTION / PLAN.

### 2. `log.level`

**UI:** **`enum`**, список уровней — по версии ядра (PLAN). **Шаблон:** `@log_level` или литерал. Остальные ключи **`log`** — round-trip.

### 3. TUN `address`

**UI:** один CIDR — **`type: text`**, **`name`**: **`tun_address`**; в шаблоне `"address": ["@tun_address"]`. Несколько CIDR — **`type: text_list`**, пачка строк на Settings; шаблон **`address`** и сериализация **`value`** — PLAN. **Шаблон:** TUN в **`params`** (при необходимости **`config`**).

### 3b. TUN `mtu`

**UI:** **`type: text`**, **`name`**: **`tun_mtu`**, в **`params`** у TUN-inbound значение **`mtu`** — из маркера **`"@tun_mtu"`** (в итоговом JSON — **число**, парсинг — PLAN). Одна переменная на все платформенные записи TUN в **`params`**.

### 4. TUN для macOS

**Текущий продукт:** в **`wizard_template.json`** объявлена переменная **`tun`** и блок TUN-**`inbounds`** с **`"platforms": ["darwin"]`**, **`"if": ["tun"]`**. Пользователь переключает TUN на вкладке **Settings**; в **`state.json`** — **`vars`**. Устаревший **`config_params.enable_tun_macos`** при **LoadState** мигрирует в **`vars.tun`**. Отдельного поля **`EnableTunForMacOS`** в модели нет. Значение **`darwin-tun`** в **`platforms`** не совпадает с **`runtime.GOOS`** на macOS (**`darwin`**), поэтому такие блоки не применяются.

---

### Сквозной пример: TUN

Фрагменты иллюстративные; фактический **`bin/wizard_template.json`** и поля TUN — по sing-box и PLAN. Правила размещения **`@…`** — в разделе **«Где допускается `@…`»** выше.

**1) `vars`**

Один CIDR на все TUN-inbound. **`tun`** в примере — шаблон + **`state.vars`** (см. §4). Ранее (до 032) эквивалент задавался **`config_params.enable_tun_macos`** и галочкой на **Rules**. Через **`@…`** имя **`tun`** **не** подставляется — оно участвует в **`if`** / **`if_or`**. Опционально отдельный bool (**`apply_tun_sniff_rules`**) для **`if`** у **`route.rules`** — в шаблоне по умолчанию не используется.

```json
"vars": [
  {
    "name": "tun_address",
    "type": "text",
    "default_value": "172.16.0.1/30",
    "wizard_ui": "edit",
    "comment": "CIDR TUN; в params: address: [\"@tun_address\"]"
  },
  {
    "name": "tun",
    "type": "bool",
    "default_value": "true",
    "wizard_ui": "edit",
    "platforms": ["darwin"],
    "comment": "Галочка TUN macOS; params darwin inbounds + if [tun]"
  },
  {
    "name": "apply_tun_sniff_rules",
    "type": "bool",
    "default_value": "true",
    "wizard_ui": "hidden",
    "comment": "Для if у route.rules"
  }
]
```

**Несколько CIDR:** **`type: text_list`**; на Settings — **пачка строк** (виджет — PLAN). Формат в **`comment`**, шаблон **`address`**, сериализация **`state.vars[].value`** — PLAN. Пример объявления:

```json
{
  "name": "tun_address_list",
  "type": "text_list",
  "default_value": "172.16.0.1/30",
  "wizard_ui": "edit",
  "comment": "Несколько CIDR — кодировка default_value и @… в PLAN"
}
```

**2) TUN в `params`**

Для **одного** CIDR (**`type: text`**, **`tun_address`**) во всех записях с **`type: "tun"`** (платформы **`windows`/`linux`**, **`darwin`** с **`if`**) используйте один плейсхолдер в **`address`**:

```json
{
  "name": "inbounds",
  "platforms": ["windows", "linux"],
  "value": [
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "singbox-tun0",
      "address": ["@tun_address"],
      "mtu": 1492,
      "auto_route": true,
      "strict_route": false,
      "stack": "system"
    }
  ]
}
```

Для **macOS TUN** — тот же приём с **`address": ["@tun_address"]`** (отличаются **`platforms`**, **`if`**, наличие **`interface_name`** — как в **`bin/wizard_template.json`**). Win7-сборка лаунчера (**windows/386**) использует тот же TUN-блок, что и **`windows`/`linux`**; незаданный **`tun_stack`** по умолчанию **`gvisor`** — через **`default_value`**-объект в шаблоне (**`VarDefaultValue`**). **`@…`** не ставить в **`parser_config`**.

**macOS, TUN-inbound:** запись **`"name": "inbounds"`**, **`"platforms": ["darwin"]`**, **`"if": ["tun"]`** (см. **`bin/wizard_template.json`**).

```json
{
  "name": "inbounds",
  "platforms": ["darwin"],
  "if": ["tun"],
  "value": [
    {
      "type": "tun",
      "tag": "tun-in",
      "address": ["@tun_address"],
      "mtu": 1492,
      "auto_route": true,
      "strict_route": false,
      "stack": "system"
    }
  ]
}
```

**3) `route.rules` с `if_or`**

```json
{
  "name": "route.rules",
  "platforms": ["windows", "linux", "darwin"],
  "if_or": ["tun_builtin", "tun"],
  "mode": "prepend",
  "value": [
    { "inbound": "tun-in", "action": "resolve", "strategy": "prefer_ipv4" },
    { "inbound": "tun-in", "action": "sniff", "timeout": "1s" }
  ]
}
```

**4) `tun`:** **`vars`** + **`state.vars`** и **`"if": ["tun"]`** у TUN-**`inbounds`** на **`darwin`** — в продукте (см. **`bin/wizard_template.json`**).

**5) Пример `state.json` (целевой формат с `vars`)**

```json
"vars": [
  { "name": "tun_address", "value": "10.0.0.1/24" },
  { "name": "tun", "value": "false" }
]
```

В старых снимках state без **`vars.tun`** значение может приходить из **`config_params.enable_tun_macos`** до однократной миграции при загрузке.

Если **`tun_address`** пользователь не трогал — записи в **`state.vars`** нет, адрес берётся из **`default_value`** / **`default_node`** в шаблоне.

**6) После сборки** (фрагмент **`inbounds`** на Windows):

```json
"inbounds": [
  {
    "type": "tun",
    "tag": "tun-in",
    "interface_name": "singbox-tun0",
    "address": ["10.0.0.1/24"],
    "mtu": 1492,
    "auto_route": true,
    "strict_route": false,
    "stack": "system"
  }
]
```

**7) `@log_level` в `config`**

```json
"config": {
  "log": {
    "level": "@log_level",
    "timestamp": true
  }
}
```

**8) macOS без TUN** — запись с **`mixed`**, без **`@…`**:

```json
{
  "name": "inbounds",
  "platforms": ["darwin"],
  "value": [
    {
      "type": "mixed",
      "tag": "proxy-in",
      "listen": "127.0.0.1",
      "listen_port": 7890,
      "set_system_proxy": true
    }
  ],
  "mode": "prepend"
}
```

**9) Таблица примера**

| Переменная | Объявление | `@…` | `if` | Иначе |
|------------|------------|------|------|--------|
| **`tun_address`** | **`vars`** | **`params`**, `address[]` | — | — |
| **`tun_mtu`** | **`vars`** | **`params`**, TUN `mtu` | — | — |
| **`tun`** | **`vars`** | — | **`params`**, `inbounds`, **`darwin` + if** | — |
| **`mixed_listen_port`** | **`vars`** | **`params`**, mixed `listen_port`, **`darwin`** | — | — |
| **`apply_tun_sniff_rules`** | **`vars`** | — | **`route.rules`** | — |
| **`log_level`** | **`vars`** | **`config.log.level`** | — | — |
| **`clash_api`** | **`vars`** | **`config.experimental.clash_api.external_controller`** | — | — |
| **`clash_secret`** | **`vars`** | **`config.experimental.clash_api.secret`** | — | — |

---

## Дополнительные настройки (кандидаты в PLAN)

| Настройка | Путь | Примечание |
|-----------|------|------------|
| `log.timestamp` | `log.timestamp` | bool |
| `interface_name`, `stack` (TUN) | inbounds / params | при необходимости отдельные **`vars`** — PLAN |
| `experimental.cache_file` | | |

---

## Поведение для пользователя

1. **Settings** перед **Preview**.
2. Clash API / секрет, log level, TUN address / MTU; на macOS — TUN, порт mixed, галочка TUN.
3. Preview / Save отражают изменения в JSON.

---

## Состояние и персистентность

- **`state.json` → `vars`:** только переопределения, поля **`name`** и **`value`**. Семантика переменных — из шаблона.
- Сироты отфильтрованы при загрузке (см. выше).
- Старый state **без** **`vars`**: при разрешении шаг **state** пропускается → дефолты шаблона.
- Миграция **`config_params.enable_tun_macos`** → **`state.vars`** с **`name`: `tun`** — PLAN.

Пример:

```json
"vars": [
  { "name": "log_level", "value": "info" },
  { "name": "tun", "value": "false" }
]
```

Здесь нет **`tun_address`** — значит адрес из шаблона; кнопка «Сброс» для этой строки — PLAN.

---

## Критерии приёмки

- Порядок вкладок: **Settings** перед **Preview**.
- Строки **Settings** строятся из **`vars`** (**`type`**, **`wizard_ui`**, **`platforms`**), в т.ч. **`text_list`**. **`type: custom`** — без автостроки, пока нет явного UI (PLAN).
- Режим шаблона не хранится в **state**: нет записи в **`state.vars`**; в памяти — признак дефолта; при вводе — запись и «Сброс» (где применимо); **`value: ""`** допустим как явное значение.
- **`clash_api`**, **`clash_secret`** (в т.ч. дефолт-случайная строка — PLAN), **`log_level`**, **`tun_address`**, **`tun_mtu`** влияют на конфиг; на macOS — **`tun`**, **`mixed_listen_port`**; пустое разрешение у **`@…`** → **warn** в лог (**`name`**, поле); для скалярных типов подстановка **`""`**, для **`text_list`** — пустой массив (PLAN); сырых **`@…`** в выдаче нет.
- TUN macOS на **Settings**, не на **Rules**.
- Регрессий нет: **`applyParams`**, TUN **`inbounds`** (в т.ч. **windows/386** и дефолт **`tun_stack`**), darwin без TUN.
- Локали — CONSTITUTION.
- **docs/release_notes/upcoming.md**; при необходимости **ARCHITECTURE.md** (IMPLEMENTATION_PROMPT).

---

## Зависимости и ссылки

- `bin/wizard_template.json`, `ui/wizard/template/loader.go`, `create_config.go`, `wizard_model.go`, `ui/wizard/tabs/settings_tab.go`, `rules_tab.go`.
- sing-box: [Configuration](https://sing-box.sagernet.org/configuration/), [TUN](https://sing-box.sagernet.org/configuration/inbound/tun/), [Mixed](https://sing-box.sagernet.org/configuration/inbound/mixed/), [Listen Fields](https://sing-box.sagernet.org/configuration/shared/listen/#structure).
- DNS через **`vars`:** перенос полей вкладки **DNS** (strategy, independent cache, default domain resolver, **final**) на **`vars` + `@…` в `config`** и персист только в **`state.vars`** — ТЗ в **[SUB_SPEC_DNS_TAB_VARS.md](./SUB_SPEC_DNS_TAB_VARS.md)** (после реализации обновить **docs/WIZARD_STATE.md**, **CREATE_WIZARD_TEMPLATE**.md / **_RU.md**).

---

## Вне scope

- Полное редактирование всех `inbounds` / произвольных `params` с нуля.
- DNS и маршруты (отдельные вкладки).
- Детали UX показа/копирования **`clash_secret`** и аудита без согласования с CONSTITUTION / security — PLAN.

