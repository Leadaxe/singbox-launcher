# План: Тип правила «SRS»

## 1. Архитектура

### 1.1 Компоненты

```
┌─────────────────────────────────────────────────────────────────┐
│  Add Rule Dialog (add_rule_dialog.go)                            │
│  ├── Rule type radio: IP | Domains/URLs | Processes | Custom JSON │ SRS
│  └── При типе «SRS»:                                       │
│        - Rule name (label)                                       │
│        - Уровень 1: категория Geosite | GeoIP                    │
│        - Уровень 2: список rule-set'ов (по алфавиту), мультивыбор│
│        - Ссылка на README / источник                            │
│        - SRS URLs (ручной ввод, опционально)                     │
│        - Outbound selector                                       │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  RuleState (custom rule)                                         │
│  Rule.RuleSets: [{ tag, type, format, url }, ...]  (по одному на URL) │
│  Rule.Rule: { rule_set: [tag1, tag2, ...] }                       │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  MergeRouteSection (create_config.go)                            │
│  - Уже добавляет rule_set из Rule.RuleSets и правило из Rule.Rule│
│  - Для custom SRS: remote остаётся remote (без подстановки local) │
└─────────────────────────────────────────────────────────────────┘
```

### 1.2 Поток данных

1. **Добавление правила** → пользователь выбирает тип «SRS», вводит название и URL → по Save генерируется tag, создаётся RuleState с RuleSets и Rule, правило добавляется в model.CustomRules.
2. **Сборка конфига** → MergeRouteSection обходит custom rules, для каждого добавляет RuleSets и rule; для «SRS» rule_set остаётся `type: "remote"`, url пользователя.
3. **Сохранение состояния** → в PersistedCustomRule для типа «SRS» сохраняются type, label, rule (с rule_set), при необходимости rule_sets (определения rule_set для восстановления).
4. **Загрузка состояния** → при восстановлении custom rule типа «SRS» восстанавливаются RuleSets из сохранённых данных и Rule.Rule.

---

## 2. Изменения по файлам

### 2.1 ui/wizard/dialogs/rule_dialog.go

- Добавить константу: `RuleTypeSRSURL = "SRS"`.
- В радио-группе типов в add_rule_dialog.go добавить этот тип.

### 2.2 ui/wizard/dialogs/add_rule_dialog.go

- **Двухуровневый выбор**: сначала выбор категории — Geosite (rule-set-geosite) или GeoIP (rule-set-geoip); затем список всех .srs в выбранной категории, **отсортированный по алфавиту** (имя без расширения), с чекбоксами/мультиселектом. Источник списка имён: захардкоженный список в коде (обновлять при релизах runetfreedom) или запрос GitHub API при открытии диалога — на выбор реализации. Ссылки в UI: на [rule-set-geosite](https://github.com/runetfreedom/russia-v2ray-rules-dat/tree/release/sing-box/rule-set-geosite), [rule-set-geoip](https://github.com/runetfreedom/russia-v2ray-rules-dat/tree/release/sing-box/rule-set-geoip), на README проекта. **Для категории Geosite** у каждого пункта списка дополнительно показывать ссылку на источник в v2fly: `https://github.com/v2fly/domain-list-community/blob/master/data/<name>`, где `<name>` = имя файла без `.srs`, при префиксе `geosite-` — только часть после него (например `geosite-anime.srs` → `data/anime`).
- Дополнительно поле **SRS URLs** (ручной ввод) для своих URL вне runetfreedom.
- В переключателе по типу правила: при выборе «SRS» показывать label, двухуровневый каталог, ссылку на README, опционально SRS URLs, outbound (скрывать IP, Domains, Processes, Custom JSON).
- Валидация при Save: для типа «SRS» хотя бы один rule-set (из каталога и/или из ручного ввода); каждый URL валидный.
- При сохранении: из выбранных в каталоге + ручного ввода собрать список URL; для каждого сгенерировать tag; собрать Rule: RuleSets и Rule = { rule_set: [tag1, ...] }; создать RuleState.
- При редактировании: по сохранённым URL определить категорию и отмеченные пункты в списке (или подставить ручные URL); при сохранении пересобрать RuleSets и Rule.

### 2.3 ui/wizard/models/wizard_state_file.go

- **PersistedCustomRule**: добавить опциональное поле для хранения определений rule_set (массив). При сохранении custom rule типа «SRS» записывать в него все элементы из Rule.RuleSets (один или несколько), как в шаблоне у «Russian blocked resources».
- **ToPersistedCustomRule**: для правил с Rule.RuleSets заполнять поле RuleSets.
- **ToRuleState** (из PersistedCustomRule): при наличии RuleSets восстанавливать Rule.RuleSets из них (конвертация в []json.RawMessage для TemplateSelectableRule).
- **DetermineRuleType**: для custom rules тип берётся из сохранённого поля `type`; при миграции старых state без type — по правилу: если в rule есть только `rule_set` (массив из одного элемента) и нет domain/ip_cidr/process_name — считать тип «SRS» или «Custom JSON» (лучше по явному type).

### 2.4 ui/wizard/business/create_config.go

- **MergeRouteSection**: уже добавляет в route rule_set из каждого RuleState.Rule.RuleSets и правило из Rule.Rule. Для custom SRS URL rule_set будет `type: "remote"`, url пользователя — подстановка local не выполняется (convertRuleSetToLocalIfNeeded срабатывает только на raw.githubusercontent.com). Изменений может не потребоваться; проверить, что remote rule_set с произвольным URL попадает в конфиг как есть.

### 2.5 Утилита генерации tag

- Функция `GenerateCustomSRSTag(url string) string`: из URL — последний сегмент пути, убрать `.srs` → префикс `custom-`. Для нескольких URL вызывается по одному разу на каждый; при дубликатах имён файлов можно добавить суффикс (например номер), чтобы теги оставались уникальными.

---

## 3. Тесты

- Юнит-тест генерации tag: одинаковые label+URL → один и тот же tag; разные пары → разные теги.
- Интеграционный тест (при наличии): добавить custom rule типа «SRS», сохранить конфиг, проверить наличие в route одного rule_set (remote) и одного правила с rule_set.

---

## 4. Чеклист изменений

| Файл | Действие |
|------|----------|
| ui/wizard/dialogs/rule_dialog.go | Константа RuleTypeSRSURL, при необходимости GenerateCustomSRSTag |
| ui/wizard/dialogs/add_rule_dialog.go | Двухуровневый выбор (категория Geosite/GeoIP → список по алфавиту), ссылки на источник и README, опционально SRS URLs, валидация, сборка Rule/RuleSets при Save для типа SRS |
| ui/wizard/models/wizard_state_file.go | PersistedCustomRule.RuleSets, ToPersistedCustomRule/ToRuleState с RuleSets, DetermineRuleType для SRS при миграции |
| ui/wizard/business/create_config.go | Проверить MergeRouteSection (remote без raw.githubusercontent.com не подменять на local) |
