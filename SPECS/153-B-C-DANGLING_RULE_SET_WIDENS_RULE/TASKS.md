# TASKS 153

- [x] Ответы на открытые вопросы SPEC (DNS-правила пресетов; гейт SPEC 152 и кэш `.srs`) — в SPEC.md
- [x] `cleanDanglingRuleSetInRule` / `cleanDanglingDNSRule`: все ссылки висячие → правило выпадает, признак `lost`
- [x] `resolvePresetRouteRule`: `template_fragment_dropped` при `lost`
- [x] `MergePresetsIntoDNS`: очистка правил пресетов, предупреждение, `dnsRuleOwner`; `ResolvedDNSRule.Name`
- [x] `CollectEmittedRouteRuleSetTags`: `PresetNodes`
- [x] Контракт 1.1.101: VERSION, warnings.json, generated docs, TEMPLATE_LANG §5.1, changelog, TASKS_LXBOX §98
- [x] `TestDanglingRuleSetDropsRule`; правка `TestDNSRuleSetRefsSurviveWhenTagIsEmitted`; эталоны `wizard_template_config`
- [x] upcoming.md (EN/RU), SPECS/README.md
- [x] Проверки: `go build ./...`, именованные тесты
- [ ] CI (`run_mode=tests`) и приёмка владельцем
