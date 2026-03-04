# Задачи: Тип правила «SRS»

## Этап 1: Модель и константы

- [ ] Добавить константу `RuleTypeSRSURL = "SRS"` в `ui/wizard/dialogs/rule_dialog.go`
- [ ] Реализовать функцию генерации tag `GenerateCustomSRSTag(url string) string`: tag = `custom-` + имя файла из URL без расширения; при нескольких URL вызывать для каждого (при дубликатах имён — уникальный суффикс, чтобы теги не совпадали)
- [ ] В `ui/wizard/models/wizard_state_file.go`: добавить в `PersistedCustomRule` опциональное поле `RuleSets` для хранения определений rule_set
- [ ] В `ToPersistedCustomRule`: при наличии у RuleState.Rule.RuleSets записывать их в PersistedCustomRule.RuleSets
- [ ] В `ToRuleState` (восстановление из PersistedCustomRule): при наличии RuleSets восстанавливать Rule.RuleSets в TemplateSelectableRule
- [ ] В `DetermineRuleType`: учесть тип «SRS» (по полю rule_set и отсутствию domain/ip_cidr/process или по сохранённому type)

## Этап 2: Диалог добавления/редактирования правила

- [ ] В `add_rule_dialog.go`: двухуровневый выбор — уровень 1: категория Geosite / GeoIP (rule-set-geosite, rule-set-geoip); уровень 2: список rule-set'ов выбранной категории, **отсортированный по алфавиту**, с мультивыбором (чекбоксы)
- [ ] Добавить в диалог ссылку на README (https://github.com/runetfreedom/russia-v2ray-rules-dat) и при необходимости ссылки на rule-set-geosite / rule-set-geoip на GitHub
- [ ] Для элементов категории Geosite: показывать дополнительную ссылку на источник в v2fly (https://github.com/v2fly/domain-list-community/blob/master/data/&lt;name&gt;), формируя &lt;name&gt; из имени файла: без .srs; при префиксе geosite- — только часть после него (например geosite-anime.srs → data/anime)
- [ ] Добавить тип «SRS» в радио-группу типов правил (RuleTypeIP, RuleTypeDomain, RuleTypeProcess, RuleTypeCustom, RuleTypeSRSURL)
- [ ] При выборе типа «SRS» показывать: Rule name, двухуровневый каталог, ссылку на README, опционально поле SRS URLs (ручной ввод), Outbound (скрывать остальные поля)
- [ ] При открытии диалога редактирования для правила типа «SRS»: по URL из Rule.RuleSets восстановить категорию и отмеченные пункты в списке или подставить ручные URL в поле SRS URLs
- [ ] Валидация при Save: для типа «SRS» хотя бы один rule-set (из каталога и/или ручного ввода); каждый URL валидный (http/https); при ошибке показывать диалог и не сохранять
- [ ] При Save для типа «SRS»: из выбранных в каталоге + ручного ввода собрать URL; для каждого сгенерировать tag; Rule.RuleSets = [{ tag, type, format, url }, ...], Rule.Rule = { rule_set: [tag1, tag2, ...] }; создать/обновить RuleState и добавить в CustomRules или обновить по индексу

## Этап 3: Сборка конфига и сохранение состояния

- [ ] Убедиться, что `MergeRouteSection` в `create_config.go` добавляет rule_set из custom rules с `type: "remote"` и произвольным url без подстановки local (convertRuleSetToLocalIfNeeded только для raw.githubusercontent.com)
- [ ] Проверить сохранение и загрузку состояния: custom rule типа «SRS» (в т.ч. с несколькими URL) сохраняется с RuleSets и восстанавливается с Rule.RuleSets

## Этап 4: Тесты и документация

- [ ] Добавить юнит-тест генерации tag (например `.../geosite-anime.srs` → `custom-geosite-anime`; URL без .srs; пустой путь → запасной tag)
- [ ] При необходимости добавить интеграционный тест: custom rule SRS с одним и с несколькими URL → в конфиге есть соответствующие remote rule_set и правило с rule_set: [tag1, ...]
- [ ] После реализации заполнить IMPLEMENTATION_REPORT.md
