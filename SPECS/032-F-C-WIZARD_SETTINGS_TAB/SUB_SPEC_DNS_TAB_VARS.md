# SUB-SPEC: DNS вкладка через `vars` и `@…` (без дубля в `dns_options` state)

**Родительская спека:** [SPEC.md](./SPEC.md) — **`vars`**, **`state.vars`**, маркеры **`@<name>`**, **`params.if` / `if_or`**.

**Статус:** реализовано в коде и **`bin/wizard_template.json`**; см. **`docs/WIZARD_STATE.md`**, **`ui/wizard/business/dns_settings_vars.go`**.

---

## 1. Цель

- **Читаемость шаблона:** в **`wizard_template.json` → `config`** явно видны **`@…`** для полей, которыми управляет вкладка **DNS** (как для **`@log_level`**).
- **Один механизм:** объявления и дефолты — в корневом **`vars`**, переопределения пользователя — только в **`state.json` → `vars`** (**`name` / `value`**), без «псевдопеременных» и без сохранения этих скаляров в **`state.json` → `dns_options`**.
- **Секция `dns_options` в шаблоне и объект `dns_options` в `state.json`:** после реализации содержат **только** **`servers`** и **`rules`**. В элементах **`servers`** по-прежнему допускаются визард-поля (**`description`**, **`enabled`**, **`detour`**, …) и поля sing-box. **Не** хранить в **`dns_options`** корневые ключи **`strategy`**, **`independent_cache`**, **`final`**, **`default_domain_resolver`**, **`default_domain_resolver_unset`**, **`dns.final`**, **`route.default_domain_resolver`** — всё это задаётся **`config` + `vars`** и **`state.vars`**.
- **UI:** вкладка **DNS** без изменения сценариев; элементы **`vars`** с типами из таблицы (**enum/bool/text**) и **`wizard_ui: fix`** (без строк на **Settings**). В **`comment`** у переменной указать, что значение задаётся на вкладке **DNS** (язык комментариев — по **CONSTITUTION** для шаблона, обычно EN).

---

## 2. Список переменных и где стоят маркеры `@…`

Имена зафиксированы в **`snake_case`**, префикс **`dns_`**. Подстановка — по правилам **032**: только объявленные имена, литерал JSON-строки **ровно** `"@<name>"` (или позиция для bool/числа — см. реализацию и белый список путей в **`config`**).

| `vars[].name` | Смысл (вкладка DNS) | Где маркер **`@…`** в шаблоне | Примечание |
|---------------|---------------------|-------------------------------|------------|
| **`dns_strategy`** | **Strategy** (`dns.strategy`) | **`config.dns.strategy`** | Значение — строка, допустимые литералы как у sing-box и текущего селекта DNS. |
| **`dns_independent_cache`** | **Раздельный кеш** (`independent_cache`) | **`config.dns.independent_cache`** | **`type: bool`**, **`wizard_ui: fix`**: в **`state.vars`** — строки **`"true"`** / **`"false"`**; в итоговом JSON — **bool** (общая ветка подстановки для **`bool`**). |
| **`dns_default_domain_resolver`** | **Default domain resolver** (тег сервера) | **`config.route.default_domain_resolver`** | Строка — **тег** из списка серверов; динамический список в UI без изменений. |
| **`dns_final`** | **Final** (тег сервера для `dns.final`) | **`config.dns.final`** | Строка — **тег** сервера (как в селекте **Final** на вкладке DNS). |

**Четыре переменные** — обязательный набор первой поставки (единый контур с **strategy**, **cache**, **default domain resolver**).

**Не** объявлять маркеры внутри **`dns_options`** в шаблоне для этих полей: дефолты задаются через **`vars[].default_value`** / **`default_node`** и строки **`@…`** в **`config`**, как у остальных переменных.

---

## 3. Объявления в `vars` (шаблон)

Для **каждой** из четырёх строк таблицы:

- **`type`:** для **`dns_strategy`** — **`enum`**; для **`dns_independent_cache`** — **`bool`**; для **`dns_default_domain_resolver`** и **`dns_final`** — **`text`**.
- **`wizard_ui`:** **`fix`** (строка на вкладке **Settings** не показывается; правка только с вкладки **DNS**).
- **`default_node`** (предпочтительно): путь к узлу в **`config`** после загрузки шаблона, например **`config.dns.strategy`**, **`config.dns.independent_cache`**, **`config.route.default_domain_resolver`**, **`config.dns.final`** — чтобы дефолт совпадал со скелетом.
- **`default_value`:** при необходимости fallback, если **`default_node`** пуст (порядок разрешения — **SPEC 032**).
- **`comment`:** явно: переменная **DNS tab** / **Settings hidden** / кратко что в sing-box (для автора шаблона).

---

## 4. `state.json`

- **`vars`:** единственное место персиста пользовательских переопределений для этих имён (**`{ "name", "value" }`**).
- **`dns_options` в `state.json`:** только **`servers`** и **`rules`** (плюс поля внутри объектов серверов). Скаляры вкладки DNS (**strategy**, **cache**, **final**, **default resolver**, **unset**) — **только** в **`state.vars`** (**`dns_*`**). При **Save** не добавлять в корень **`dns_options`** других ключей.
- Миграция при **LoadState:** при наличии старых полей в **`dns_options`** — однократно перенести в **`model.SettingsVars`** / **`state.vars`** при отсутствии записи с тем же **`name`**; затем не писать обратно в **`dns_options`** (идемпотентно).

---

## 5. Особые случаи

- **`default_domain_resolver` + unset:** по-прежнему отдельный флаг в модели (**не** хранить как «пустая строка в **`@dns_default_domain_resolver`**» без правила). После разрешения **`vars`** при **unset** — удаление ключа **`route.default_domain_resolver`** в **`MergeRouteSection`** (как сейчас). Зафиксировать в **PLAN**.
- **Белый список путей** для **`@…`** в **`config`:** расширить под **`config.dns.strategy`**, **`config.dns.independent_cache`**, **`config.dns.final`**, **`config.route.default_domain_resolver`** (и не выходить за **032** без согласования).
- **Валидация шаблона:** каждый **`@dns_*`** имеет объявление в **`vars`**.

---

## 6. Зависимости для реализации

- **`bin/wizard_template.json`:** добавить **`vars[]`**, заменить в **`config.dns` / `config.route`** литералы на **`@dns_…`**; в **`dns_options`** оставить **только** **`servers`** и **`rules`** — удалить оттуда **`independent_cache`**, **`dns.final`**, **`route.default_domain_resolver`** и любые однокорневые аналоги **strategy/final/resolver** (дефолты — из **`config`** через **`vars`**).
- Код: **`ui/wizard/tabs/dns_tab.go`**, **`presenter_state.go`** / **`CreateStateFromModel`**, **`restoreDNS`**, **`wizard_dns.go`**, **`create_config.go`**, **`template` / подстановка**, **`loader` / валидация путей**.
- Документация: **`docs/WIZARD_STATE.md`**, **`docs/CREATE_WIZARD_TEMPLATE*.md`**, **`docs/ARCHITECTURE.md`**, **`docs/release_notes/upcoming.md`** — убрать формулировки «только **`dns_options`** для strategy/…» и описать связку **DNS UI ↔ `vars`**.

---

## 7. Историческая справка

Ранее в репозитории было зафиксировано решение **не** переносить DNS-скаляры в **`vars`**. Текущий документ **заменяет** то решение: перенос **принят**; старые формулировки в доках — до прохода реализации.



