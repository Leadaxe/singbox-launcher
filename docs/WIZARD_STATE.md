# Wizard state (state.json)

Формат файла состояния визарда конфигурации и логика загрузки/сохранения.

## Назначение

Файл `state.json` (и именованные состояния `<id>.json`) хранит полное состояние визарда: выбранные источники прокси, outbounds, правила маршрутизации (в т.ч. пользовательские), параметры конфигурации. При открытии визарда состояние загружается из текущего файла; при сохранении — записывается обратно.

## Резюме по блокам (чтение)

Ниже — **кто главный** при восстановлении модели. «Шаблон» = актуальный **`bin/wizard_template.json`** после **`LoadTemplateData`**. **State** = загруженный снимок (`state.json` или `<id>.json`). Порядок вызовов при **`LoadState`** — в разделе **«Поток чтения»**.

| Блок | Резюме при **`LoadState`** (есть state) | Резюме **без** state (первый запуск / Read → New) |
|------|----------------------------------------|---------------------------------------------------|
| **Шаблон целиком** | Всегда читается **до** state: каркас `config`, дефолты DNS/selectable, сырой `dns_options` шаблона, `DefaultFinal` и т.д. State **не** заменяет шаблон целиком — по полям правила разные (строки таблицы ниже). | Тот же шаблон; парсер может прийти из **`config.json`**, если там есть валидный `@ParserConfig`. |
| **`parser_config`** | **Только state.** Шаблонный парсер на этом шаге **не** подмешивается. | **`config.json`** (приоритет) или **шаблон**. |
| **`config_params`** (`route.final`, …) | **State**; если параметра нет — **`DefaultFinal`** из **шаблона**. Устаревший **`enable_tun_macos`** при загрузке мигрирует в **`vars.tun`** (см. **`vars`**). `route.default_domain_resolver` здесь не норма (одноразовая миграция → см. DNS). | Обычно нет файла state → final задаётся из шаблона / **`EnsureFinalSelected`** после инициализации **`custom_rules`**. |
| **`vars`** | **State**: переопределения переменных шаблона (вкладка **Settings**), пары **`name`** / **`value`**. Сироты (имена не из текущего шаблона) при **LoadState** не попадают в модель; при **Save** в файл уходят только имена из шаблона. Повтор **`name`** в массиве JSON — при загрузке побеждает **последняя** запись. После **`restoreConfigParams`** для плейсхолдера **`clash_secret`** вызывается **`MaterializeClashSecretIfNeeded`**: сгенерированный секрет один раз попадает в **`SettingsVars`**, чтобы превью/DNS не меняли значение на каждом обновлении (пока пользователь не нажмёт **Сброс** для этого поля). | Нет ключа → дефолты из **`wizard_template.json`** (`vars[].default_value` / `default_node`). |
| **`dns_options`** | Снимок из **state** в модель, затем **`ApplyWizardDNSTemplate`**: список серверов **сшивается** с **текущим** шаблоном; **пустые** поля модели добираются из шаблона (скелет `config.dns` + `dns_options` шаблона по правилам в коде). | Нет снимка → **`ApplyWizardDNSTemplate`** только из **шаблона** (если список DNS в модели ещё пуст). |
| **`selectable_rule_states`** | **Только формат до `rules_library_merged` (версия 2 без флага):** до **`LoadState`** миграция **`ApplyRulesLibraryMigration`** переносит записи в начало **`custom_rules`** и очищает selectable. В сохранённом файле **3** ключа обычно нет. | Не используется: первый запуск без state — **`InitializeTemplateState`** засевает **`custom_rules`** из пресетов шаблона с **`default: true`**. |
| **`custom_rules`** | **Единственный список правил маршрута** в модели после миграции: полные объекты, порядок = порядок в `route.rules` при генерации. | См. **`selectable_rule_states`** / засев из шаблона. |

**Итог одной фразой:** для **парсера** и **`custom_rules`** при **`LoadState`** приоритет у **state** (после однократной миграции library selectable→custom); пресеты **`selectable_rules`** в шаблоне — только **библиотека** для кнопки «Add from library», не отдельный слой в модели; для **DNS** — **снимок state + обязательная сшивка с шаблоном** и добор пустот из шаблона.

### `vars` и условия `params.if` / `params.if_or`

При сборке эффективного конфига из шаблона для каждого имени в **`if`** или **`if_or`** проверяется, входит ли текущая ОС в **`vars[].platforms`** (пустой список — на всех ОС; иначе совпадение с **`runtime.GOOS`**, без отдельной метки **`win7`** — Win7-сборка лаунчера это **windows/386**). Если **нет**, переменная для этого условия считается **ложной**, **даже если** в **`state.vars`** сохранено **`"true"`** (например, профиль перенесён с другой ОС). Подробнее — **docs/CREATE_WIZARD_TEMPLATE.md** (раздел про **`vars`** и **`if`**), **`ui/wizard/template/vars_resolve.go`**.

## Версия формата

- **version**: целое число. Поддерживается чтение **`2`** и **`3`**; новые сохранения пишут **`3`**.
- **`3` + rules library:** поле **`rules_library_merged`** (обычно `true`). Маршрут собирается **только** из **`custom_rules`** (единый список). Ключ **`selectable_rule_states`** в новых файлах не используется (может отсутствовать). Пресеты шаблона **`selectable_rules`** остаются **библиотекой** в UI («Add from library»), а не отдельным слоем в state. При первом открытии файла версии **2** без флага выполняется однократная миграция: содержимое **`selectable_rule_states`** сливается в начало **`custom_rules`**, затем state перезаписывается на диск.

## Структура JSON

Корневой объект содержит:

| Поле | Тип | Описание |
|------|-----|----------|
| `version` | int | Версия формата (обязательное) |
| `id` | string | Идентификатор состояния (для именованных состояний; опционально для state.json) |
| `comment` | string | Комментарий (опционально) |
| `created_at` | string | RFC3339 (обязательное) |
| `updated_at` | string | RFC3339 (обязательное) |
| `parser_config` | object | Конфигурация парсера (proxies, outbounds, parser) |
| `config_params` | array | Параметры без отдельной секции в state (в первую очередь **`route.final`**). Устаревший **`enable_tun_macos`** читается только для миграции в **`vars`**. **Не** используется для `route.default_domain_resolver` — см. **`dns_options`**. |
| `vars` | array | Переопределения шаблонных переменных: объекты **`{ "name": string, "value": string }`**. TUN на macOS — переменная **`tun`** (`"true"` / `"false"`). |
| `dns_options` | object | Состояние вкладки DNS визарда (опционально; см. ниже). Имя ключа совпадает с секцией шаблона `wizard_template.json`. |
| `selectable_rule_states` | array | Устарело при **`rules_library_merged`**: в формате **3** не используется для route (миграция с версии **2**) |
| `rules_library_merged` | bool | **`true`** после миграции/нового формата: только **`custom_rules`** задают порядок правил в маршруте |
| `custom_rules` | array | Все правила маршрута (полная структура), порядок = порядок в `route.rules` |

Краткие резюме по ключам JSON (детали — в разделах ниже и в **«Резюме по блокам»**):

- **`parser_config`** — при `LoadState`: вся правда в этом объекте из файла.
- **`config_params`** — в т.ч. **`route.final`**; резолвер DNS сюда не кладём.
- **`vars`** — пользовательские значения для **`wizard_template.json`** → **`vars`** (Settings); TUN macOS — **`tun`**.
- **`dns_options`** — снимок вкладки DNS + сшивка с шаблоном после загрузки.
- **`selectable_rule_states`** — устаревший слой (v2); при отсутствии **`rules_library_merged`** сливается в **`custom_rules`** при загрузке.
- **`rules_library_merged`** — после **`true`** в файле и модели нет отдельного списка selectable-state; в **`custom_rules`** лежат все правила маршрута.
- **`custom_rules`** — полный список **пользовательских** правил маршрута; при генерации конфига **`MergeRouteSection`** дописывает включённые записи к **базовому** `route` из шаблона (статические `rules` / `rule_set` в шаблоне остаются первыми). Подробнее — **`docs/ARCHITECTURE.md`**, **`create_config.go`**.

## dns_options (объект в state.json)

> **Резюме (чтение):** из state в модель копируется **весь** объект `dns_options` (серверы, **`rules`**, `final`, `strategy`, резолвер, …). Затем **`ApplyWizardDNSTemplate`** сверяет список с **актуальным** шаблоном: порядок/закреплённые теги из `config.dns`, перекрытие тел строк по тегу и галочке «в конфиг», добор **пустых** полей из шаблона. **`strategy`** из шаблона, если в модели пусто: база `config.dns.strategy`, поверх — `dns_options.strategy` шаблона.

Корневой ключ **`dns_options`** — снимок настроек DNS визарда для последующей генерации `config.dns` (то же имя, что у секции дефолтов в шаблоне). Правила хранятся как массив **`rules`** (те же объекты, что в sing-box `dns.rules` и в шаблонном `dns_options.rules`). Во **внутреннем** редакторе визарда — построчный текст (один JSON-объект на строку, комментарии `#`); при сохранении state текст **парсится** в **`rules`**. Если парсинг **не удался**, ключ **`rules`** в файл не попадает; комментарии `#` и пустые строки при успешном сохранении в **`rules`** не восстанавливаются. Ключ **`rules_text`** в старых `state.json` **не читается** — правила в редакторе берутся только из **`rules`** (или заполняются из шаблона при пустом массиве).

| Поле | Тип | Описание |
|------|-----|----------|
| `servers` | array | Список объектов DNS-сервера: поля sing-box как в `dns.servers`, плюс опционально **`description`** (строка для подсказки в списке) и **`enabled`** (bool; если `false`, сервер не попадает в сгенерированный `config.dns`, но остаётся в state и во вкладке DNS с выключенной галочкой; отсутствие ключа = включён). Непустой **`detour`** в объекте сервера в **строке** списка вкладки DNS дописывается в конец в квадратных скобках (после `tag · type · server`); **всплывающая подсказка** показывает только **`description`**. |
| `rules` | array | Правила DNS как JSON-массив объектов (как `dns.rules` в sing-box). |
| `final` | string | Тег сервера для `dns.final`. |
| `strategy` | string | Опционально (`ipv4_only`, …). |
| `independent_cache` | bool | Опционально. |
| `default_domain_resolver` | string | Опционально. Тег DNS-сервера для `config.route.default_domain_resolver` — **единственное** место в `state.json`, где хранится выбранный резолвер (вместе с флагом сброса ниже). |
| `default_domain_resolver_unset` | bool | Если `true`, пользователь явно выбрал «не задано» для `route.default_domain_resolver`; ключ в сгенерированном `route` опускается. |

**`config_params`:** параметр **`route.default_domain_resolver`** туда **не записывается**. Старые файлы, где он ещё есть, при загрузке учитываются **только как миграция**: если после чтения **`dns_options`** в модели резолвер пустой и не режим «unset», значение один раз подставляется из **`config_params`** (см. **`restoreDNS`**). После следующего сохранения state дубль исчезнет.

Дефолт из шаблона: в **`wizard_template.json`** в секции **`dns_options`** — поле `default_domain_resolver` или строковый ключ **`route.default_domain_resolver`**, иначе `config.route.default_domain_resolver`. Стартовый список серверов и правила при первом запуске могут задаваться там же (`servers`, `rules`, **`dns.final`** / `final`, `strategy`, `independent_cache`); у серверов поля `description` и `enabled` только для визарда и не попадают в sing-box; если в шаблонном `dns_options.servers` нет `type: local`, локальный резолвер дописывается из `config.dns.servers`.

**Порядок при `LoadState`:** миграция **`enable_tun_macos` → `vars.tun`** (если **`tun`** ещё нет), затем **`config_params`** (`route.final`, прочее — **без** резолвера DNS), затем **`restoreDNS`**: при наличии **`dns_options`** — **`LoadPersistedWizardDNS`** (в модель попадают **все** поля снимка из state, в т.ч. **`strategy`**, **`final`**, серверы и т.д.), при необходимости подхват старого **`route.default_domain_resolver`** из **`config_params`**, затем всегда **`ApplyWizardDNSTemplate`** — слияние списка серверов с шаблоном и подстановка **только пустых** полей из шаблона. Для **`strategy`** из шаблона (если в модели после state всё ещё пусто): база — **`config.dns.strategy`**, поверх — **`dns_options.strategy`** шаблона (второе перекрывает первое).

**`ApplyWizardDNSTemplate`** пересобирает список серверов в порядке `config.dns.servers` (закреплённые теги), затем **`dns_options.servers`** с остальными тегами, затем осиротевшие сохранённые теги. Для **одинакового `tag`** между **`config.dns.servers`** (скелет) и **`dns_options.servers`**: при включённой галочке «в конфиг» строка берётся из **`dns_options`**, при выключенной — форма остаётся как в скелете **`config.dns`**. Пустые / плейсхолдер **правил** (текст редактора после загрузки **`rules`**), пустые **`final`** / **`strategy`**, отсутствующий **`independent_cache`** и пустой **`default_domain_resolver`** (если не «не задан») добираются из шаблона; при необходимости в начало списка добавляется **`local`** из `config.dns`.

Если ключа **`dns_options`** в state нет, после **`ApplyWizardDNSTemplate`** всё берётся из шаблона.

### Поток DNS (шаблон → модель → state → config.json)

> **Резюме:** снимок **`dns_options`** из state (если есть) → модель → **`ApplyWizardDNSTemplate`** (сшивка с **текущим** шаблоном + добор пустых полей). Итоговый **`config.json`**: **`MergeDNSSection`** / **`MergeRouteSection`** при сохранении.

Единая «сшивка» шаблона с данными визарда и state — **`wizardbusiness.ApplyWizardDNSTemplate(model)`** (`ui/wizard/business/wizard_dns.go`). Её вызывают **`restoreDNS`** после **`LoadPersistedWizardDNS`** (если в файле есть **`dns_options`**) и **`initializeWizardContent`** в **`wizard.go`**, если список серверов в модели ещё пуст (нет state или первый запуск).

1. **Шаблон** (`LoadTemplateData`): в модель попадают эффективный **`config`**, сырой **`dns_options`** шаблона и агрегаты вроде **`DefaultDomainResolver`** для дефолтов.
2. **State** при загрузке: объект **`dns_options`** из файла целиком → **`LoadPersistedWizardDNS`** (в т.ч. **`strategy`**, **`final`**, **`rules`**, серверы); при пустом резолвере в модели — одноразовый подхват из устаревшего **`config_params.route.default_domain_resolver`**; затем **`ApplyWizardDNSTemplate`**. Непустые поля из state не затираются подстановкой из шаблона; пустой **`strategy`** добирается из шаблона: скелет **`config.dns`**, затем **`dns_options`** шаблона (перекрытие).
3. **Модель** держит текущее состояние вкладки DNS; **`DNSLockedTags`** задаётся при reconcile (теги из **`config.dns.servers`** шаблона).
4. **UI** (`dns_tab.go`): селекты **Final**, **default domain resolver** и **strategy** при выборе пользователя сразу обновляют **`WizardModel`** (и флаг превью), затем при необходимости срабатывает синхронизация виджетов из модели; после смены **enabled** у сервера — **`RefreshDNSDependentSelectsOnly`** (только селекты), после Add/Edit/Delete — **`RefreshDNSListAndSelects`**; полный проход модель→виджеты — **`SyncModelToGUI`** (через **`fyne.Do`**). У строк из скелета (**`config.dns`** шаблона, закреплённые теги) чекбокс «в конфиг» **заблокирован** (`Disable`): отображает текущее **`enabled`** из модели/state, переключение только вне этого списка (редактирование **`dns_options.servers`** в state при необходимости). Выпадающие **Final** и **резолвер** содержат только теги серверов с включённой галочкой «в конфиг»; закреплённая строка из скелета **без** галочки в выпадашке не показывается (в списке серверов строка остаётся; при включении галочки тело строки может перекрываться **`dns_options`** — см. **`mergeLockedRow`**). **`SyncGUIToModel`** при смене вкладки/Save содержит защиты от гонки с отложенным обновлением DNS-виджетов.
5. **Сохранение state:** только **`dns_options`** для резолвера (без дубля в **`config_params`**).
6. **Сборка `config.json`:** **`buildDNSSection`** / **`MergeDNSSection`** для **`dns`**; **`MergeRouteSection`** — для **`route.default_domain_resolver`** или его удаления при **unset**. При открытии визарда **без** предварительного `LoadState` (нет `state.json`) тот же **`ApplyWizardDNSTemplate`** вызывается из **`initializeWizardContent`**, если список серверов ещё пуст. Спецификация: **SPECS/024-F-C-WIZARD_DNS_SECTION/SPEC.md**.

## `parser_config` и `config_params` (корень state.json)

> **Резюме (`parser_config`):** при **`LoadState`** в модель попадает **только** содержимое из файла state (**`restoreParserConfig`**). Шаблонный парсер на этом шаге **не** смешивается.

> **Резюме (`config_params`):** из state читаются **`route.final`** и остальные пары `name`/`value`; если **`route.final`** в state нет — **`DefaultFinal`** из шаблона. **`enable_tun_macos`** не используется как источник истины: до **`restoreConfigParams`** выполняется миграция в **`vars.tun`**. **`route.default_domain_resolver`** в `config_params` — устаревший дубль; подхватывается **один раз** в **`restoreDNS`**, если после **`dns_options`** резолвер в модели пуст и не режим unset.

Схема **`parser_config`** в JSON и миграции — **SPECS/002-F-C-WIZARD_STATE/WIZARD_STATE_JSON_SCHEMA.md**, **`WizardStateFile.UnmarshalJSON`**.

## `selectable_rule_states` (корень state.json)

> **Резюме (актуальный формат 3):** в норме **отсутствует**. Если файл ещё в старом виде (**`rules_library_merged`** ложь / отсутствует), **`ApplyRulesLibraryMigration`** (в **`LoadState`**, до **`restoreCustomRules`**) строит единый **`custom_rules`**: сначала правила из шаблона в порядке **`selectable_rules`** с учётом сохранённых **`enabled` / selected_outbound** по **`label`**, затем хвост прежних **`custom_rules`**; выставляет **`rules_library_merged`**, очищает **`selectable_rule_states`** в объекте, который уйдёт в **`restoreCustomRules`**.

> **Исторически (до миграции):** источник структуры — шаблон; в state были только **`label`**, **`enabled`**, **`selected_outbound`** по совпадению с **`TemplateData.SelectableRules`**.

## custom_rules (PersistedCustomRule)

> **Резюме (чтение):** при **`LoadState`** правила берутся **только** из массива `custom_rules` в файле state. Шаблон их не определяет и не накладывает. Миграции формата — при **`UnmarshalJSON`** (`MigrateCustomRules`, вывод `type` из `rule` при необходимости).

Каждый элемент — объект с полями:

| Поле | Тип | Описание |
|------|-----|----------|
| `label` | string | Название правила |
| `type` | string | Тип: только `ips`, `urls`, `processes`, `srs`, `raw` |
| `enabled` | bool | Включено ли правило |
| `selected_outbound` | string | Выбранный outbound |
| `description` | string | Описание (опционально) |
| `rule` | object | JSON объекта правила маршрутизации (ip_cidr, domain, rule_set и т.д.) |
| `default_outbound` | string | Outbound по умолчанию |
| `has_outbound` | bool | Есть ли outbound в правиле |
| `params` | object | Состояние UI по типу (опционально; в конфиг не попадает) |
| `rule_set` | array | Определения rule-set'ов для типа `srs` (опционально) |

### type — константы

В state и в коде используются только значения: `ips`, `urls`, `processes`, `srs`, `raw`. При загрузке, если `type` отсутствует или имеет старый формат (например `"Domains/URLs"`), тип выводится из содержимого `rule` функцией **DetermineRuleType(rule)**. При сохранении всегда записываются только эти константы.

### params

Объект для восстановления состояния интерфейса по типу правила:

- **processes:** `match_by_path` (bool), `path_mode` ("Simple"|"Regex") — переключатель «Match by path» и режим Simple/Regex.
- **urls:** `domain_regex` (bool) — состояние галочки «Regex».
- Типы `ips`, `srs`, `raw` могут не использовать params.

### rule_set (для типа srs)

Массив определений rule-set'ов в формате как в `bin/wizard_template.json`: элементы с полями `tag`, `type`, `format`, `url`. При загрузке восстанавливаются в `Rule.RuleSets`; при сохранении записываются из `Rule.RuleSets`.

## Поток чтения: `wizard_template.json`, текущий `state.json` и другой снимок

Ниже — как **собирается модель визарда** из шаблона и из файлов состояния. Код: `ui/wizard/wizard.go` (старт), `ui/wizard/presentation/presenter_state.go` (`LoadState`), `ui/wizard/business/loader.go` (`LoadConfigFromFile`), `ui/wizard/template/loader.go` (`LoadTemplateData`), `ui/wizard/business/state_store.go`, `ui/wizard/models/wizard_state_file.go` (`UnmarshalJSON`, миграции).

### 1. Шаблон всегда загружается первым

При открытии визарда **`LoadTemplateData(ExecDir)`** читает **`bin/wizard_template.json`** и заполняет **`model.TemplateData`**:

| Часть шаблона | Куда попадает | Примечание |
|---------------|----------------|------------|
| **`parser_config`** | `TemplateData.ParserConfig` (строка JSON с обёрткой `ParserConfig` для UI) | Используется, если нет state и нет валидного блока в `config.json` |
| **`config` + `params`** | После **`applyParams`** под текущий **GOOS** (и на darwin с учётом TUN — см. **`GetEffectiveConfig`**) → **`TemplateData.Config`** (секции по ключам), **`ConfigOrder`**, **`RawConfig`**, **`Params`** | Эффективный **`config.dns`** — скелет для DNS; **`route`** — для дефолтов и генерации |
| **`dns_options`** | **`TemplateData.DNSOptionsRaw`** (сырой JSON) | Дефолты вкладки DNS, не отдельный объект sing-box |
| **`selectable_rules`** | **`TemplateData.SelectableRules`** | После фильтра по **`platforms`** под текущую ОС |
| Агрегаты | **`DefaultFinal`**, **`DefaultDomainResolver`** | Извлекаются из `config.route` / `dns_options` шаблона в загрузчике |

Шаблон **не перезагружается** при смене снимка state: остаётся тот же файл в `ExecDir`. Имеет смысл держать шаблон актуальным; при несовпадении версии шаблона и старого state возможны пропуски правил (selectable без совпадения по `label`).

### 2. Старт визарда при **наличии** `state.json`

> **Резюме:** файл state → миграции при разборе JSON → **`LoadState`**: парсер и правила маршрута **из state** (после **`ApplyRulesLibraryMigration`** — только **`custom_rules`**); **config_params** из state (с fallback шаблона для final); DNS — **state + ApplyWizardDNSTemplate**.

1. **`StateStore.LoadCurrentState()`** читает **`bin/wizard_states/state.json`**. Десериализация в **`WizardStateFile`**: кастомный **`UnmarshalJSON`** (миграции **`MigrateSelectableRuleStates`**, **`MigrateCustomRules`**, упрощённый **`parser_config`**).
2. **`presenter.LoadState(stateFile)`** (порядок шагов в коде):
   - **`restoreParserConfig`** — **`parser_config` целиком из state** перезаписывает модель (`ParserConfig`, `ParserConfigJSON`); шаблонный парсер здесь не используется.
   - **`SourceURLs = ""`** — поле ввода URL только для добавления; список источников из **`ParserConfig.Proxies`**.
   - **`restoreConfigParams`** — из **`config_params`**: `route.final` → **`SelectedFinalOutbound`**; если `route.final` нет — **`DefaultFinal`** из шаблона. Из **`vars`** в state — в **`model.SettingsVars`** (TUN macOS — ключ **`tun`**). Затем **`MaterializeClashSecretIfNeeded`** — стабилизация автогенерации **`clash_secret`** в модели (см. строку таблицы про **`vars`**). **`route.default_domain_resolver` в `config_params`** на этом шаге не читается (только миграция в **`restoreDNS`**).
   - **`restoreDNS`** — см. раздел **dns_options** и **Поток DNS** выше: **`LoadPersistedWizardDNS`** (если в state есть **`dns_options`**) копирует в модель **весь** снимок DNS из файла; при необходимости подхват старого резолвера из **`config_params`**; затем **`ApplyWizardDNSTemplate`** (слияние списка серверов с **текущим** шаблоном + подстановка **пустых** полей из шаблона).
   - **`ApplyRulesLibraryMigration(stateFile, TemplateData, ExecDir)`** — если миграция library ещё не выполнена: объединение selectable+template order и существующих **`custom_rules`** в один список в **`stateFile.CustomRules`**, **`RulesLibraryMerged = true`**, **`SelectableRuleStates = nil`**.
   - **`model.RulesLibraryMerged`**, **`model.SelectableRuleStates = nil`**, затем **`restoreCustomRules(stateFile.CustomRules)`** — единственный источник правил маршрута в модели.
   - **`PreviewNeedsParse = true`**, **`SyncModelToGUI`**, **`RefreshOutboundOptions`**. Если миграция только что записала флаг merged — **`SaveWizardState`** текущего файла (идемпотентность при повторном открытии) и **`MarkAsSaved`**; иначе **`MarkAsSaved`**.

Итог: при **LoadState** источники правды — **state** для парсера, **config_params** для final, **`vars`** для настроек шаблона (в т.ч. TUN), **dns_options + шаблон** для DNS (см. DNS-раздел), **`custom_rules` (после миграции)** для маршрута.

### 3. Старт визарда **без** `state.json`

> **Резюме:** парсер из **`config.json`** или шаблона; правила маршрута и DNS — из **шаблона** (`InitializeTemplateState`, при пустом списке DNS — `ApplyWizardDNSTemplate`). **`LoadState` не вызывается.**

1. **`LoadConfigFromFile`** — приоритет **`config.json`**: извлекается блок **`@ParserConfig`**; иначе парсер из **шаблона**. Опционально **`EnsureRequiredOutbounds`**. В модель: **`ParserConfigJSON`**, **`SourceURLs`** (строка из источников в конфиге).
2. **`initializeWizardContent`** → **`InitializeTemplateState`**: **`SelectableRuleStates` всегда сбрасывается**; если **`!RulesLibraryMerged`** и **`CustomRules` пуст** — в **`CustomRules`** добавляются клоны пресетов **`selectable_rules`** с **`IsDefault`** (и SRS-проверкой), затем **`RulesLibraryMerged = true`**; для каждой записи — **`EnsureDefaultOutbound`**; **`EnsureFinalSelected`** для **`SelectedFinalOutbound`**.
3. Если **`len(DNSServers) == 0`** — **`ApplyWizardDNSTemplate`** (только шаблон, без предварительного **`LoadPersistedWizardDNS`**).

**`LoadState` не вызывается.**

### 4. Кнопка **Read** — текущий или **другой** снимок

> **Резюме:** тот же **`LoadState`**, что при старте с `state.json`. Именованный снимок перед этим **копируется** в `state.json`. **New** в диалоге = сценарий без state (п.3).

- Выбор **`state.json`** → **`LoadCurrentState()`** → тот же **`LoadState`**, что в п.2.
- Выбор **именованного** `<id>.json` → **`LoadWizardState(id)`**; при успехе снимок **копируется** в **`state.json`** (**`SaveCurrentState`**), затем **`LoadState`**. Логика восстановления модели **та же**, что при старте с текущим файлом.
- **New** в диалоге: без **`LoadState`** — снова **`LoadConfigFromFile`** + **`InitializeTemplateState`** + **`SyncModelToGUI`** (как «чистый» сценарий без сохранённого state).

### 5. Сводная таблица: что откуда при **`LoadState`**

> **Резюме:** дублирует таблицу **«Резюме по блокам»** в виде трёх колонок для быстрого сопоставления с кодом.

| Область | Основной источник | Роль шаблона |
|---------|-------------------|--------------|
| Парсер, источники, outbounds в JSON | **`parser_config` в state** | Не подмешивается при LoadState |
| Поле URL на Sources | Пустое; список из **Proxies** | — |
| **`route.final`** | **`config_params` state** | Fallback **`DefaultFinal`** шаблона, если параметра нет |
| **Переменные шаблона / TUN macOS** | **`vars` state** (в т.ч. `tun`) | Дефолты из **`wizard_template.json`** (`vars`) |
| Вкладка DNS | **`dns_options` state** + **`ApplyWizardDNSTemplate`** | Скелет **`config.dns`**, сырой **`dns_options`**, блокировки тегов |
| Правила маршрута (`custom_rules`) | **`custom_rules` state** (после миграции — единственный список) | Первый запуск: засев из **`selectable_rules`** с **`default: true`**; шаблон **`selectable_rules`** — библиотека для UI |

### 6. Десериализация файла state (до `LoadState`)

> **Резюме:** сырой JSON → **`WizardStateFile.UnmarshalJSON`** (миграции selectable/custom, форма `parser_config`) → затем п.2.

1. Чтение байтов с диска.
2. **`json.Unmarshal` → `WizardStateFile`**: миграции **`selectable_rule_states`** и **`custom_rules`**, нормализация **`parser_config`**.
3. Далее — **`LoadState`** по п.2.

Подробнее о схеме полей и v1→v2: **SPECS/002-F-C-WIZARD_STATE/WIZARD_STATE_JSON_SCHEMA.md**.

## Где хранится state

- **Текущее состояние:** `bin/wizard_states/state.json` (относительно ExecDir).
- **Именованные состояния:** `bin/wizard_states/<id>.json`.

Чтение/запись выполняет слой бизнес-логики (state_store); презентер создаёт состояние из модели (CreateStateFromModel) и восстанавливает модель из загруженного файла (LoadState).

## Миграции

- **v1 → v2:** `selectable_rule_states` и `custom_rules` приводятся к новому формату (см. WIZARD_STATE_JSON_SCHEMA.md). Поле `type` в custom_rules при загрузке может быть в старом виде — тогда тип выводится из `rule`.
- **v2 → v3 (rules library):** при **`LoadState`**, если **`rules_library_merged`** ещё не установлен, **`ApplyRulesLibraryMigration`** переносит selectable-слой в **`custom_rules`**, выставляет флаг и очищает **`selectable_rule_states`** в памяти; при успешной записи **`state.json`** повторная миграция не дублирует правила.

См. также: **docs/ARCHITECTURE.md** (раздел про загрузку state), **SPECS/002-F-C-WIZARD_STATE/WIZARD_STATE_JSON_SCHEMA.md**. Краткая сводка приоритетов — раздел **«Резюме по блокам (чтение)»** в начале этого файла.
