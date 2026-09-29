# PLAN 153

1. `core/build/preset_merge.go`:
   - `cleanDanglingRuleSetInRule` и `cleanDanglingDNSRule` возвращают второй
     результат `lost`: все ссылки `rule_set` висячие → `nil, true`, правило
     выпадает, соседние поля не смотрятся. Частично уцелевший массив,
     отсутствие `rule_set`, пустой массив — прежняя логика.
   - `MergePresetsIntoDNS`: очистка для правил обоих источников; правило
     пресета без `rule_set` едет как есть (гейты SPEC 152 уже прошли). При
     `lost` — `template_fragment_dropped` в накопитель сборки
     (`noteTemplateWarnings`); `owner` — `dnsRuleOwner` (id пресета, `name`
     пользовательского правила или `dns_options`).
   - `CollectEmittedRouteRuleSetTags`: `PresetNodes` в резолв (как в
     `MergePresetsIntoRoute`) — наборы пресета `for_each` без них выпадали бы
     из множества, и очистка DNS-правил пресета сняла бы живую ссылку.
2. `core/build/resolve_route.go:resolvePresetRouteRule` — при `lost` у
   включённого пресета `template_fragment_dropped` (`route.rules`,
   `rule_set`) в `out.Warnings`.
3. `core/build/resolve_dns.go` — поле `ResolvedDNSRule.Name` (имя
   пользовательского правила из состояния) для `owner`.
4. Код предупреждения — `template_fragment_dropped`: тот же исход и тот же
   `reason`, что у гейта 1.1.82 при раскрытии; отдельный код различал бы
   только причину пропуска набора. В реестре дополнить `cause_*`, `fix_*`,
   `desc`, `go`.
5. Контракт 1.1.101: `VERSION`, `warnings.json`, `go generate` для
   `contract/docs/generated`, TEMPLATE_LANG §5.1, строка changelog в
   `contract/README.md`, TASKS_LXBOX §98. Корпус не меняется: кейсы шаблона
   раскрывают пресет без кэша `.srs`, пропуск по кэшу ими не выражается.
6. Тесты: новый `core/build/dangling_rule_set_test.go`
   (`TestDanglingRuleSetDropsRule`: штатный шаблон без кэша; пресет с
   remote-набором — route, DNS пресета, пользовательское DNS-правило,
   частично уцелевший массив); в `dns_ruleset_dangling_test.go` ожидание
   «висячее правило остаётся по server» заменено на «выпадает»; эталоны
   `wizard_template_config` перегенерированы (`-update`) со сверкой, что
   ушли только два правила на файл.
7. `docs/release_notes/upcoming.md` — EN/RU Highlights и Technical;
   `SPECS/README.md` — строка 153.
