# Спецификация: Тип правила «SRS»

## 1. Проблема

### 1.1 Текущие типы пользовательских правил

В диалоге «Add Rule» пользователь может создать правило одного из типов:

- **IP Addresses (CIDR)** — маршрутизация по списку IP/подсетей
- **Domains/URLs** — по доменам/суффиксам/ключевым словам/регулярным выражениям
- **Processes** — по имени процесса
- **Custom JSON** — произвольный JSON правила sing-box

Нет возможности добавить правило, которое использует **уже готовый rule-set в формате SRS по своей ссылке** (например, свой или сторонний geosite/geoip .srs по URL), без ручного редактирования JSON.

### 1.2 Зачем нужен тип «SRS»

- Пользователь хочет подключить произвольный SRS по URL (другой репозиторий, свой хостинг, другой ветка runetfreedom и т.д.).
- Не нужно вручную править конфиг и знать структуру `rule_set` + `rules`.
- Единообразный UX с остальными типами правил: название, выбор outbound, включено/выключено.

---

## 2. Требования

### 2.1 Новый тип в диалоге добавления правила

- В диалоге «Add Rule» / «Edit Rule» добавить тип **«SRS»** наравне с IP, Domains/URLs, Processes, Custom JSON.
- При выборе этого типа отображать:
  - **Rule name** — название правила (как у остальных типов).
  - **Визуальный выбор rule-set'ов** — двухуровневый:
    1. **Уровень 1 — категория**: выбор одной из двух папок — [rule-set-geosite](https://github.com/runetfreedom/russia-v2ray-rules-dat/tree/release/sing-box/rule-set-geosite) или [rule-set-geoip](https://github.com/runetfreedom/russia-v2ray-rules-dat/tree/release/sing-box/rule-set-geoip) (например переключатель или два блока «Geosite» / «GeoIP»).
    2. **Уровень 2 — список rule-set'ов**: после выбора категории показывается список всех .srs в этой категории, **отсортированный по алфавиту** (по имени файла без расширения). Пользователь отмечает один или несколько пунктов (чекбоксы или мультиселект). Для каждого пункта при необходимости — ссылка на источник (страница файла на GitHub).
  - В диалоге — **ссылка на README проекта**: [russia-v2ray-rules-dat](https://github.com/runetfreedom/russia-v2ray-rules-dat) и/или [sing-box в release](https://github.com/runetfreedom/russia-v2ray-rules-dat/tree/release/sing-box).
  - **Дополнительно** — поле «SRS URLs» (ручной ввод): одно или несколько своих URL на .srs, если нужны rule-set'ы не из runetfreedom. Валидация: при выборе из списка или ручном вводе — хотя бы один rule-set; каждый URL — непустой и допустимого формата. По аналогии с шаблоном правило может использовать **несколько** rule_set.
  - **Outbound** — выбор outbound (как у остальных типов).

### 2.2 Каталог runetfreedom (двухуровневый выбор)

- **Уровень 1 — категория**: две опции, соответствующие папкам в репозитории:
  - [rule-set-geosite](https://github.com/runetfreedom/russia-v2ray-rules-dat/tree/release/sing-box/rule-set-geosite) (отображать как «Geosite»),
  - [rule-set-geoip](https://github.com/runetfreedom/russia-v2ray-rules-dat/tree/release/sing-box/rule-set-geoip) (отображать как «GeoIP»).
  Пользователь сначала выбирает категорию (переключатель или вкладки).
- **Уровень 2 — список rule-set'ов**: после выбора категории показывается список всех .srs в этой категории. Список **отсортирован по алфавиту** (по имени файла без расширения, например geosite-anime, geosite-google). Элементы — чекбоксы или мультиселект; можно выбрать один или несколько. Для каждого элемента — ссылка на файл в runetfreedom (например `https://github.com/runetfreedom/russia-v2ray-rules-dat/blob/release/sing-box/rule-set-geosite/geosite-anime.srs`). **Для категории Geosite** дополнительно показывать ссылку на источник в [v2fly/domain-list-community](https://github.com/v2fly/domain-list-community/blob/master/data/): URL формируется автоматически по имени файла — `https://github.com/v2fly/domain-list-community/blob/master/data/<name>`, где `<name>` = имя файла без `.srs`, при этом если имя начинается с префикса `geosite-`, брать часть после него (например `geosite-anime.srs` → `data/anime`, `geosite-google.srs` → `data/google`); иначе — полное имя без расширения.
- **Источник данных списка**: список имён .srs может быть зашит в приложение (JSON/константы по категориям, обновляется при релизах) или получаться по GitHub API при открытии диалога/смене категории; при недоступности API — запасной статический список или только ручной ввод URL.
- **Ссылка на README**: в диалоге — кликабельная ссылка на [README russia-v2ray-rules-dat](https://github.com/runetfreedom/russia-v2ray-rules-dat) и/или [sing-box в release](https://github.com/runetfreedom/russia-v2ray-rules-dat/tree/release/sing-box).
- **Ссылка на источник (v2fly) для Geosite**: для элементов категории Geosite показывать дополнительную ссылку «Source» на [domain-list-community/data](https://github.com/v2fly/domain-list-community/blob/master/data/): `https://github.com/v2fly/domain-list-community/blob/master/data/<name>`, где `<name>` из имени .srs: убрать расширение; если имя начинается с `geosite-` — взять только часть после префикса (например `geosite-anime` → `anime`), иначе — имя целиком.
- **URL для выбранных**: при выборе из каталога URL = `https://raw.githubusercontent.com/runetfreedom/russia-v2ray-rules-dat/release/sing-box/rule-set-geosite/<name>.srs` или `.../rule-set-geoip/<name>.srs` в зависимости от категории.

### 2.3 Генерация tag для rule_set

- Для **каждого** URL при сохранении генерируется **tag**: **`custom-` + имя файла из URL без расширения** (например `.../geosite-anime.srs` → `custom-geosite-anime`). Один URL — один элемент в массиве rule_set со своим tag. Если URL несколько — несколько rule_set и в правиле маршрутизации `rule_set: [tag1, tag2, ...]`, как у «Russian blocked resources» в шаблоне.
- Один и тот же URL даёт один и тот же tag; при смене списка URL теги пересчитываются.

### 2.4 Поведение в конфиге

- В секции `route`:
  - Добавляется **по одному rule_set на каждый URL**: каждый с `type: "remote"`, `format: "binary"`, `url`, `tag` (tag = `custom-` + имя файла без расширения). Может быть один или несколько — по аналогии с правилом «Russian blocked resources» в шаблоне.
  - Добавляется одно **правило** маршрутизации: `rule_set: ["<tag1>", "<tag2>", ...]` (все сгенерированные теги), `outbound` (или `action` для reject/drop).
- Правило попадает в конфиг только если оно **включено** (Enabled), как и остальные пользовательские правила.
- sing-box при запуске загружает SRS по указанному URL сам (remote rule-set). Локальное скачивание в `bin/rule-sets/` для этого типа **не предусматривается** в рамках данной задачи (при желании можно вынести в отдельную задачу).

### 2.5 Сохранение и загрузка состояния

- В состоянии визарда (state file) пользовательское правило типа «SRS» сохраняется так же, как остальные custom rules: label, type, enabled, selected_outbound, rule.
- В `rule` хранится всё необходимое для восстановления: ссылка на rule_set (tag) и определение самого rule_set (tag + url). Структура данных — см. раздел 3.

### 2.6 Критерии приёмки

1. В диалоге доступен тип «SRS» с полями: название, визуальный выбор rule-set'ов из каталога runetfreedom (и при необходимости ручной ввод URL), outbound; есть ссылка на источник (файл на GitHub) и на README проекта.
2. Валидация: хотя бы один URL; при некорректном URL показывать ошибку и не сохранять.
3. При сохранении создаётся пользовательское правило с корректными RuleSets (по одному на URL) и Rule; в сгенерированном конфиге — один или несколько remote rule_set и одно правило по ним (`rule_set: [tag1, ...]`) с выбранным outbound.
4. При загрузке состояния правило типа «SRS» восстанавливается (название, список URL, outbound, включено/выключено).
5. Редактирование и удаление такого правила работают так же, как для других пользовательских правил.

---

## 3. Структура данных

### 3.1 Представление в модели (RuleState, TemplateSelectableRule)

Для пользовательского правила типа «SRS» (по аналогии с правилами из шаблона, в т.ч. «Russian blocked resources»):

- **Rule.RuleSets** — массив: по одному элементу на каждый URL: `{ "tag": "<tag>", "type": "remote", "format": "binary", "url": "<URL>" }`. Тегов может быть один или несколько.
- **Rule.Rule** — объект правила: `{ "rule_set": ["<tag1>", "<tag2>", ...] }` (все теги правила). Outbound/action подставляется при сборке конфига (как для остальных правил).

### 3.2 Уникальный tag

- Формат: **`custom-` + имя файла из URL без расширения**. Примеры:
  - URL `https://example.com/path/geosite-anime.srs` → tag `custom-geosite-anime`
  - URL `https://raw.githubusercontent.com/foo/repo/branch/geoip-ru.srs` → tag `custom-geoip-ru`
- Имя файла берётся из последнего сегмента пути URL; расширение `.srs` (если есть) отбрасывается. Если путь заканчивается слэшем или имя пусто — запасной вариант (например `custom-srs-` + короткий хэш от URL). При нескольких URL с одинаковым именем файла — добавлять суффикс (например номер), чтобы теги оставались уникальными.
- При редактировании правила: при смене списка URL теги пересчитываются.

### 3.3 PersistedCustomRule (state file)

По аналогии с **selectable_rules** в `bin/wizard_template.json`: там у правила с SRS есть массив **rule_set** (определения `{ "tag", "type", "format", "url" }`) и объект **rule** (маршрутизация `{ "rule_set": "<tag>", "outbound" }`). То же храним для пользовательского правила типа «SRS»:

- **type**: `"SRS"` (новая константа в rule_dialog.go / wizard_state_file.go).
- **rule_set** (или **RuleSets**): массив из одного или нескольких элементов — определения rule_set `{ "tag", "type", "format", "url" }`, как в шаблоне (например у «Russian blocked resources» два элемента).
- **rule**: объект правила маршрутизации `{ "rule_set": ["<tag1>", "<tag2>", ...] }` (outbound подставляется при сборке). Совпадает с полем `rule` в элементах selectable_rules шаблона.

При сохранении: для типа «SRS» записываем в PersistedCustomRule поле rule_set (массив) из RuleState.Rule.RuleSets и rule из RuleState.Rule.Rule. При загрузке: восстанавливаем Rule.RuleSets и Rule.Rule из этих полей — получается та же структура, что у правил с SRS из шаблона.

### 3.4 DetermineRuleType

- В wizard_state_file.go в DetermineRuleType добавить распознавание типа «SRS»: по наличию в rule поля `rule_set` (массив тегов) и наличию в PersistedCustomRule поля rule_set (массив определений). Либо тип однозначно задаётся сохранённым полем `type` — тогда при загрузке тип берётся из state.

---

## 4. Ограничения и риски

- **Remote-only**: SRS загружается sing-box по URL при старте. Если URL недоступен (сеть, блокировки), правило может не работать до появления доступа. Локальное кэширование не входит в эту задачу.
- **Валидация URL**: проверять только непустоту и допустимый формат URL; не проверять доступность по HTTP.
- **Язык UI**: тексты в UI и сообщения об ошибках — только английский (constitution).

---

## 5. Связанные документы

- sing-box rule-set: https://sing-box.sagernet.org/configuration/rule-set/
- Каталог SRS (runetfreedom): [release/sing-box](https://github.com/runetfreedom/russia-v2ray-rules-dat/tree/release/sing-box), [README проекта](https://github.com/runetfreedom/russia-v2ray-rules-dat).
- Текущая реализация правил: `ui/wizard/dialogs/add_rule_dialog.go`, `ui/wizard/business/create_config.go` (MergeRouteSection), `ui/wizard/models/wizard_state_file.go` (PersistedCustomRule, DetermineRuleType).
- Локальное скачивание SRS (шаблонные правила): `todo/complete/SRS_LOCAL_DOWNLOAD/`.
