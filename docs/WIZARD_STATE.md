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
| **`vars`** | **State**: переопределения переменных шаблона (вкладка **Settings**), пары **`name`** / **`value`**. Элементы шаблона **`{"separator": true}`** не имеют **`name`** и **не** сериализуются в **`state.vars`**. Сироты (имена не из текущего шаблона) при **LoadState** не попадают в модель; при **Save** в файл уходят только имена из объявленных переменных шаблона (без разделителей). Повтор **`name`** в массиве JSON — при загрузке побеждает **последняя** запись. После **`restoreConfigParams`** для плейсхолдера **`clash_secret`** вызывается **`MaterializeClashSecretIfNeeded`**: сгенерированный секрет один раз попадает в **`SettingsVars`**, чтобы превью/DNS не меняли значение на каждом обновлении (пока пользователь не нажмёт **Сброс** для этого поля). | Нет ключа → дефолты из **`wizard_template.json`** (`vars[].default_value` / `default_node`). |
| **`dns_options`** | Из **state** в модель попадают **только** **`servers`** и **`rules`**; скаляры вкладки DNS (**strategy**, **cache**, **final**, **default domain resolver**) — в **`state.vars`** как **`dns_*`** (см. **`SUB_SPEC_DNS_TAB_VARS`**). Затем **`ApplyWizardDNSTemplate`** + **`ApplyDNSVarsFromSettingsToModel`**. | Нет снимка → **`ApplyWizardDNSTemplate`** + **`ApplyDNSVarsFromSettingsToModel`** из **шаблона** (если список DNS в модели ещё пуст). |
| **`selectable_rule_states`** | **Только формат до `rules_library_merged` (версия 2 без флага):** до **`LoadState`** миграция **`ApplyRulesLibraryMigration`** переносит записи в начало **`custom_rules`** и очищает selectable. В сохранённом файле **3** ключа обычно нет. | Не используется: первый запуск без state — **`InitializeTemplateState`** засевает **`custom_rules`** из пресетов шаблона с **`default: true`**. |
| **`custom_rules`** | **Единственный список правил маршрута** в модели после миграции: полные объекты, порядок = порядок в `route.rules` при генерации. | См. **`selectable_rule_states`** / засев из шаблона. |

**Итог одной фразой:** для **парсера** и **`custom_rules`** при **`LoadState`** приоритет у **state** (после однократной миграции library selectable→custom); пресеты **`selectable_rules`** в шаблоне — только **библиотека** для кнопки «Add from library», не отдельный слой в модели; для **DNS** — **`dns_options.servers`/`rules` из state**, скаляры из **`vars`**, **сшивка с шаблоном** (**`ApplyWizardDNSTemplate`**) и выравнивание полей UI из **`dns_*`** (**`ApplyDNSVarsFromSettingsToModel`**).

### Механизм переменных шаблона (`vars` и `@…`)

Это **отдельный** контур от вкладки DNS (следующий подраздел).

**Принцип:** в **`wizard_template.json`** в корне объявляется массив **`vars`** (имя, тип, дефолты, `wizard_ui`, …). Пользовательские переопределения попадают в **`state.json` → `vars`** как массив объектов **`{ "name", "value" }`** (строка **`value`**). При сборке эффективного конфига строковые литералы **`"@<name>"`** в разрешённых местах **`config`** и **`params`** заменяются на разрешённое значение (**SPECS/032**, **`docs/CREATE_WIZARD_TEMPLATE.md`**).

**UI:** автогенерируемые строки на вкладке **Settings** (плюс особые случаи вроде **`clash_secret`**). Метаданные переменной (**тип**, подписи) всегда из шаблона; в файле state — только пары **`name` / `value`**.

**DNS и `vars`:** скаляры вкладки **DNS** (**strategy**, **independent_cache**, **final**, **default domain resolver**) объявляются в шаблоне как скрытые переменные **`dns_*`** с литералами **`@dns_*`** в **`config`**; пользовательские значения — в **`state.vars`**. Подробнее — **`SPECS/032-F-C-WIZARD_SETTINGS_TAB/SUB_SPEC_DNS_TAB_VARS.md`**.

#### `vars` и условия `params.if` / `params.if_or`

При сборке эффективного конфига из шаблона для каждого имени в **`if`** или **`if_or`** проверяется, входит ли текущая ОС в **`vars[].platforms`** (пустой список — на всех ОС; иначе совпадение с **`runtime.GOOS`**, без отдельной метки **`win7`** — Win7-сборка лаунчера это **windows/386**). Если **нет**, переменная для этого условия считается **ложной**, **даже если** в **`state.vars`** сохранено **`"true"`** (например, профиль перенесён с другой ОС). Подробнее — **docs/CREATE_WIZARD_TEMPLATE.md** (раздел про **`vars`** и **`if`**), **`ui/wizard/template/vars_resolve.go`**.

### Вкладка DNS: `dns_options` (servers/rules) и скаляры в `vars` (`dns_*`)

Вкладка **DNS** управляет списком серверов и **`rules`** через **`dns_options`** в **`state.json`** и **шаблон**; **strategy**, **independent_cache**, **`dns.final`**, **`route.default_domain_resolver`** — через скрытые переменные **`dns_*`** в **`wizard_template.json` → `vars`** и переопределения в **`state.vars`** (литералы **`@dns_*`** в **`config`**). Флаг **«резолвер не задан»** остаётся в модели (**`DefaultDomainResolverUnset`**), не кодируется одной пустой строкой в **`vars`**.

**Принцип работы:** **`LoadPersistedWizardDNS`** копирует только **`servers`** и **`rules`**; устаревшие ключи в старом **`dns_options`** при **`LoadState`** однократно мигрируют в **`state.vars`** (**`MigrateDNSScalarsFromPersistedToSettingsVars`**). Далее **`ApplyWizardDNSTemplate`**, затем **`ApplyDNSVarsFromSettingsToModel`**. Перед сохранением и сборкой конфига **`SyncDNSModelToSettingsVars`** синхронизирует модель → **`SettingsVars`**. Итоговый **`config.json`**: **`MergeDNSSection`** / **`MergeRouteSection`**.

### macOS: снятие `tun` в визарде (не про формат JSON)

К **`state.vars`** это не добавляет полей: речь о UI на вкладке **Settings**. Пока лаунчер считает ядро **запущенным** (**`RunningState`**, как кнопки Start/Stop на вкладке Core), переменную **`tun`** (**`name`** в шаблоне) **нельзя** переключить в off — показывается сообщение, галка остаётся включённой. После **Stop**, при переходе TUN off, при необходимости одним привилегированным **`rm -rf`** удаляются **`experimental.cache_file.path`** внутри **`bin/`** (если настроено в шаблоне и файл есть) и логи ядра **`logs/sing-box.log`** / **`logs/sing-box.log.old`** под **`ExecDir`**, если они существуют (см. подраздел **macOS: выключение `tun`** в **docs/CREATE_WIZARD_TEMPLATE.md** / **_RU.md**).

## Версия формата

- **version** (корень **`state.json`**): целое число. Чтение поддерживает **`2`**, **`3`** и **`4`**; **новые сохранения** пишут **`4`**. Версия **`3`** — rules library (единый **`custom_rules`**, **`rules_library_merged`**). Версия **`4`** — тот же каркас **плюс** закреплённая модель **переменных шаблона** (**SPECS/032**): в снимке — опциональный массив **`vars`**; в **`wizard_template.json`** — объявления **`vars`**, подстановки **`@<name>`** в **`config`**/**`params`**, условные **`if`** / **`if_or`**. Старые файлы **`3`** без ключа **`vars`** по-прежнему загружаются. **Не путать** с **`parser_config.version`** внутри того же файла (см. **`docs/ParserConfig.md`**).
- **`rules_library_merged`** (обычно `true` у актуальных снимков): маршрут собирается **только** из **`custom_rules`**. Ключ **`selectable_rule_states`** в новых файлах не используется (может отсутствовать). Пресеты шаблона **`selectable_rules`** остаются **библиотекой** в UI («Add from library»), а не отдельным слоем в state. При первом открытии файла версии **2** без флага выполняется однократная миграция: содержимое **`selectable_rule_states`** сливается в начало **`custom_rules`**, затем state перезаписывается на диск.

## Структура JSON

Корневой объект содержит:

| Поле | Тип | Описание |
|------|-----|----------|
| `version` | int | Версия формата **снимка визарда** (обязательное). Текущая запись: **`4`**. |
| `id` | string | Идентификатор состояния (для именованных состояний; опционально для state.json) |
| `comment` | string | Комментарий (опционально) |
| `created_at` | string | RFC3339 (обязательное) |
| `updated_at` | string | RFC3339 (обязательное) |
| `parser_config` | object | Конфигурация парсера (proxies, outbounds, parser); см. **`parser_config.version`** в **`docs/ParserConfig.md`** |
| `config_params` | array | Параметры без отдельной секции в state (в первую очередь **`route.final`**). Устаревший **`enable_tun_macos`** читается только для миграции в **`vars`**. Устаревший **`route.default_domain_resolver`** — одноразовая миграция в **`vars`** / модель (**`restoreDNS`**). |
| `vars` | array | Переопределения шаблонных переменных (**Settings** и скрытые **`dns_*`** с вкладки DNS): объекты **`{ "name": string, "value": string }`**. TUN на macOS — **`tun`**. Переменная **`dns_independent_cache`** в шаблоне объявлена как **`type: bool`** (как **`tun_builtin`**), в файле по-прежнему строки **`"true"`** / **`"false"`**. |
| `dns_options` | object | Состояние вкладки DNS визарда (опционально; см. ниже). Имя ключа совпадает с секцией шаблона `wizard_template.json`. |
| `selectable_rule_states` | array | Устарело при **`rules_library_merged`**: в актуальном формате не используется для route (миграция с версии **2**) |
| `rules_library_merged` | bool | **`true`** после миграции/нового формата: только **`custom_rules`** задают порядок правил в маршруте |
| `custom_rules` | array | Все правила маршрута (полная структура), порядок = порядок в `route.rules` |

Краткие резюме по ключам JSON (детали — в разделах ниже и в **«Резюме по блокам»**):

- **`parser_config`** — при `LoadState`: вся правда в этом объекте из файла.
- **`config_params`** — в т.ч. **`route.final`**; резолвер DNS сюда не кладём.
- **`vars`** — пользовательские значения для **`wizard_template.json`** → **`vars`** (Settings); TUN macOS — **`tun`**.
- **`dns_options`** — **servers**/**rules** вкладки DNS + сшивка с шаблоном; скаляры — **`vars`** (**`dns_*`**).
- **`selectable_rule_states`** — устаревший слой (v2); при отсутствии **`rules_library_merged`** сливается в **`custom_rules`** при загрузке.
- **`rules_library_merged`** — после **`true`** в файле и модели нет отдельного списка selectable-state; в **`custom_rules`** лежат все правила маршрута.
- **`custom_rules`** — полный список **пользовательских** правил маршрута; при генерации конфига **`MergeRouteSection`** дописывает включённые записи к **базовому** `route` из шаблона (статические `rules` / `rule_set` в шаблоне остаются первыми). Подробнее — **`docs/ARCHITECTURE.md`**, **`create_config.go`**.

## dns_options (объект в state.json)

> **Резюме (чтение):** в **новых** снимках в **`dns_options`** только **`servers`** и **`rules`**. Старые файлы могут содержать **`strategy`**, **`final`**, **`independent_cache`**, **`default_domain_resolver`**, **`default_domain_resolver_unset`** — они при **`LoadState`** мигрируют в **`state.vars`** (**`dns_*`**) и при следующем сохранении из **`dns_options`** исчезают. Далее **`ApplyWizardDNSTemplate`** + **`ApplyDNSVarsFromSettingsToModel`**.

Корневой ключ **`dns_options`** — снимок списка серверов и правил DNS визарда (то же имя, что у секции дефолтов в шаблоне). Правила — массив **`rules`**; в редакторе — построчный текст; при сохранении state текст **парсится** в **`rules`**. Ключ **`rules_text`** в старых `state.json` **не читается**.

| Поле | Тип | Описание |
|------|-----|----------|
| `servers` | array | Список объектов DNS-сервера (sing-box + **`description`**, **`enabled`** для визарда). |
| `rules` | array | Правила DNS (как `dns.rules` в sing-box). |
| *устаревшие* | | **`final`**, **`strategy`**, **`independent_cache`**, **`default_domain_resolver`**, **`default_domain_resolver_unset`** — читаются для миграции в **`vars`**, в новых сохранениях не пишутся. |

**`config_params`:** **`route.default_domain_resolver`** не используется как постоянное хранилище; старые файлы — одноразовый подхват в **`restoreDNS`**, если в **`vars`** ещё нет **`dns_default_domain_resolver`**.

Дефолты скаляров — из **`wizard_template.json`**: **`vars[].default_value`** для **`dns_*`** и литералы **`@dns_*`** в **`config`**. Секция шаблона **`dns_options`** содержит только **`servers`** и **`rules`** (плюс поля внутри объектов серверов).

**Порядок при `LoadState`:** **`restoreConfigParams`** → **`MigrateDNSScalarsFromPersistedToSettingsVars`** → при **`default_domain_resolver_unset`** в старом снимке выставляется флаг модели → **`LoadPersistedWizardDNS`** (только **servers**/**rules**) → при необходимости подхват резолвера из **`config_params`** → **`ApplyWizardDNSTemplate`** → **`ApplyDNSVarsFromSettingsToModel`**.

**`ApplyWizardDNSTemplate`** пересобирает список серверов (как раньше: скелет **`config.dns`**, шаблонный **`dns_options.servers`**, осиротевшие теги). Пустые **правила** и прочие поля, для которых в шаблоне **нет** объявлений **`dns_*`**, по-прежнему добираются из шаблона (**`fillDNSAuxiliaryIfEmpty`**).

### Поток DNS (шаблон → модель → state → config.json)

> **Резюме:** **`dns_options`** (**servers**/**rules**) + **`vars`** (**`dns_*`**) → модель → **`MergeDNSSection`** / **`MergeRouteSection`**.

1. **Шаблон** (`LoadTemplateData`): эффективный **`config`** (с подстановкой **`@dns_*`**), сырой **`dns_options`** (**servers**/**rules**).
2. **State:** миграция скаляров из старого **`dns_options`** в **`vars`**; загрузка **servers**/**rules**; **`ApplyWizardDNSTemplate`**; **`ApplyDNSVarsFromSettingsToModel`**.
3. **Модель** и **UI** — без смены сценариев вкладки DNS; **`SyncDNSModelToSettingsVars`** / **`SyncGUIToModel`** поддерживают **`state.vars`**.
4. **Сохранение state:** **`dns_options`** — только **servers** и **rules**; скаляры — в **`vars`**.
5. **Сборка `config.json`:** **`MergeDNSSection`** / **`MergeRouteSection`**. При первом запуске: **`initializeWizardContent`** вызывает **`ApplyWizardDNSTemplate`** и **`ApplyDNSVarsFromSettingsToModel`**. Спецификация: **024** + **SUB_SPEC_DNS_TAB_VARS**.

## `parser_config` и `config_params` (корень state.json)

> **Резюме (`parser_config`):** при **`LoadState`** в модель попадает **только** содержимое из файла state (**`restoreParserConfig`**). Шаблонный парсер на этом шаге **не** смешивается.

> **Резюме (`config_params`):** из state читаются **`route.final`** и остальные пары `name`/`value`; если **`route.final`** в state нет — **`DefaultFinal`** из шаблона. **`enable_tun_macos`** не используется как источник истины: до **`restoreConfigParams`** выполняется миграция в **`vars.tun`**. **`route.default_domain_resolver`** в `config_params` — устаревший дубль; подхватывается **один раз** в **`restoreDNS`**, если после **`dns_options`** резолвер в модели пуст и не режим unset.

Схема **`parser_config`** в JSON и миграции — **SPECS/002-F-C-WIZARD_STATE/WIZARD_STATE_JSON_SCHEMA.md**, **`WizardStateFile.UnmarshalJSON`**.

## `selectable_rule_states` (корень state.json)

> **Резюме (актуальный формат, `version` 3–4):** в норме **отсутствует**. Если файл ещё в старом виде (**`rules_library_merged`** ложь / отсутствует), **`ApplyRulesLibraryMigration`** (в **`LoadState`**, до **`restoreCustomRules`**) строит единый **`custom_rules`**: сначала правила из шаблона в порядке **`selectable_rules`** с учётом сохранённых **`enabled` / selected_outbound** по **`label`**, затем хвост прежних **`custom_rules`**; выставляет **`rules_library_merged`**, очищает **`selectable_rule_states`** в объекте, который уйдёт в **`restoreCustomRules`**.

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
- **Корневой `version` 3 → 4:** **`4`** соответствует продуктовой линии с **vars** / **`@…`** / **`if`**/**`if_or`** (**032**); **`3`** — снимки только с rules library (переменные шаблона в шаблоне/state могли ещё не использоваться). Обязательного переписывания всех **`3`→`4`** нет. Новые сохранения пишут **`4`** (см. **«Версия формата»** выше).

См. также: **docs/ARCHITECTURE.md** (раздел про загрузку state), **SPECS/002-F-C-WIZARD_STATE/WIZARD_STATE_JSON_SCHEMA.md**. Краткая сводка приоритетов — раздел **«Резюме по блокам (чтение)»** в начале этого файла.
