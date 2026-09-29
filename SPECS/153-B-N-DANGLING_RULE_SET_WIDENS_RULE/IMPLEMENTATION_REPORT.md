# IMPLEMENTATION REPORT 153

Дата: 29.09.2026. Статус: реализовано, не закоммичено, ждёт CI и приёмки.
Реализовано поверх незакоммиченной SPEC 152.

## Ответы на открытые вопросы

1. **DNS-правила пресетов со ссылкой на нескачанный набор.** Да, уходили в
   конфиг с неизвестным тегом: `rewriteRuleSetRefs` при раскрытии сверяет
   ссылки с наборами пресета (после `#if`), а не с кэшем `.srs`, и
   `MergePresetsIntoDNS` эмитил правила пресетов как есть; `repairDanglingDNSRefs`
   чинит только ссылки на серверы. В штатном шаблоне не воспроизводится
   (единственное такое правило, у `russian`, ссылается на inline-наборы), но
   любой шаблон с DNS-правилом на remote-набор ронял бы ядро до скачивания.
   Починено тем же правилом: очистка `cleanDanglingDNSRule` теперь и для
   правил пресетов.
2. **Гейт SPEC 152 и «набор не скачан».** Не доходит: `ruleSetLost` в
   `preset_expand.go` видит только наборы, объявленные пресетом и прошедшие
   `#if`/Dropped; пропуск по кэшу решается позже, в `resolve_route.go`
   (`convertPresetRuleSetRemoteToLocal`). Поэтому фикс — в очистках
   `preset_merge.go`.

## Что выяснилось по ходу (SPEC поправлена)

- `ai-services` и `messengers` в штатном шаблоне несут inline-наборы и не
  задеты; задеты `ru-blocked` и `games` (remote).
- Эталоны `wizard_template_config` собираются без DataDir, то есть без кэша
  `.srs`, и содержали баг: в каждом из 32 файлов было два правила
  `{"network": ["tcp", "udp"], "outbound": …}`. Эталоны перегенерированы
  (`-update`); сверка с копией до перегенерации — ушли ровно эти два правила
  на файл, других отличий нет (в эталонах есть и незакоммиченная правка #139 —
  она не тронута, перегенерация шла по текущему дереву).

## Изменения

- `core/build/preset_merge.go` — `cleanDanglingRuleSetInRule`,
  `cleanDanglingDNSRule` (признак `lost`, правило выпадает целиком);
  `MergePresetsIntoDNS` (очистка правил пресетов, `template_fragment_dropped`),
  `dnsRuleOwner`; `CollectEmittedRouteRuleSetTags` — `PresetNodes`.
- `core/build/resolve_route.go` — предупреждение при `lost`.
- `core/build/resolve_dns.go` — `ResolvedDNSRule.Name`.
- `core/build/dangling_rule_set_test.go` — новый `TestDanglingRuleSetDropsRule`.
- `core/build/dns_ruleset_dangling_test.go` — ожидание по висячему
  пользовательскому DNS-правилу: выпадает, а не остаётся по `server`.
- `core/build/testdata/wizard_template_config/*.json` — минус два правила.
- Контракт 1.1.101: `contract/VERSION`, `contract/registry/warnings.json`
  (`template_fragment_dropped`: `cause`, `fix`, `desc`, `go`),
  `contract/docs/generated/*`, `contract/docs/TEMPLATE_LANG.md` §5.1,
  `contract/README.md`, `contract/TASKS_LXBOX.md` §98. Корпус не менялся:
  кейсы шаблона не моделируют кэш `.srs`.
- `docs/release_notes/upcoming.md`, `SPECS/README.md`.

## Код предупреждения

`template_fragment_dropped` (`reason` `rule_set`): исход и смысл те же, что у
гейта 1.1.82 при раскрытии (все ссылки висячие → правило выпало), и LxBox
для пресетов уже шлёт этот код в случае «no cached file». Новый код различал
бы только причину пропуска набора. В реестре причина и совет дополнены
случаем «набор не скачан». `owner` пользовательского DNS-правила — `name`
правила или `dns_options` (так же зовётся владелец шаблонных DNS-серверов).

## Проверки

`go build ./...`; `TestDanglingRuleSetDropsRule`,
`TestWizardTemplateConfigUnchanged`, `TestPresetRuleConditionGate`,
`TestDNSRuleSetRefsSurviveWhenTagIsEmitted`,
`TestDisabledPresetDoesNotEmitRuleSets`, `TestMergePresets_*`,
`TestContractCorpusForEach` (core/build); `TestRegistryCodeRefsResolve`,
`TestRegistryWarningsHaveCauseAndFix`, `TestRegistryBodyStructure`
(core/config) — зелёные. Полный прогон — CI.

## LxBox

Правила пресетов — паритет уже есть (`preset_expand.dart` отбирает наборы по
кэшу до гейтов). Пользовательские DNS-правила (`DnsRuleInline`) по чтению
кода уходят как есть без очистки висячих ссылок — TASKS_LXBOX §98.
