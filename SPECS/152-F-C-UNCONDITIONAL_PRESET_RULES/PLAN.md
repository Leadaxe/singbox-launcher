# PLAN 152

1. `core/template`: счётчик `canonCtx.emptyRefs` — `replacementCanon` (Dropped
   или нулевое значение), `evalTplCanon` (вставка без значения). Наружу —
   флаг `emptyRef` у `SubstituteVarsInJSONCanonScoped` (единственный вызов —
   сборка пресета); общий обход `substituteCanonCtx`, остальные обёртки без
   изменений сигнатур.
2. `core/build/preset_expand.go`: `substitutePresetRule` (флаг наружу,
   `substitutePresetBody` — обёртка); гейты в `expandPresetBody` и
   `expandOnePresetDNSRule`: висячий `rule_set` или (нет условий и
   `emptyRef`) → `template_fragment_dropped`; нет условий без сбоя → правило
   в конфиг + `template_rule_unconditional` (`WarnTemplateRuleUnconditional`,
   `ruleUnconditional`).
3. Контракт 1.1.100: `warnings.json` (новый код, уточнение
   `template_fragment_dropped`), note двух списков в `allowlists.json`,
   TEMPLATE_LANG §5.1, строка changelog в `contract/README.md`,
   `go generate` для `contract/docs/generated`, TASKS_LXBOX §97, 4 кейса
   `corpus/template/for_each/`.
4. Тест: `TestPresetRuleConditionGate` переписан под новую семантику, route
   и DNS.
5. `docs/release_notes/upcoming.md` — EN/RU Highlights и Technical.

Не трогается: `isRuleUnusable`/`isDNSRuleUnusable`,
`cleanDanglingRuleSetInRule`/`cleanDanglingDNSRule` (`preset_merge.go`).
