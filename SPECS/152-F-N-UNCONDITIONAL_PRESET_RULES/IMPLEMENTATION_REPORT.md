# IMPLEMENTATION REPORT 152

Дата: 29.09.2026. Статус: реализовано, не закоммичено, ждёт CI и приёмки.

## Изменения

- `core/template/substitute_canon.go`, `substitute.go`, `for_each.go` —
  счётчик `emptyRefs`, `substituteCanonCtx`, флаг `emptyRef` у
  `SubstituteVarsInJSONCanonScoped`.
- `core/build/preset_expand.go` — `substitutePresetRule`, гейты route/DNS,
  `WarnTemplateRuleUnconditional`.
- `core/build/rule_condition_gate_test.go` — тест под новую семантику.
- Контракт 1.1.100: `contract/VERSION`, `contract/README.md`,
  `contract/registry/warnings.json`, `contract/registry/allowlists.json`,
  `contract/docs/TEMPLATE_LANG.md`, `contract/docs/generated/*`,
  `contract/TASKS_LXBOX.md` §97, корпус
  `contract/corpus/template/for_each/rule_{unconditional_kept,unconditional_dns_kept,conditions_lost_var_dropped,conditions_lost_rule_set_dropped}.*`.
- `docs/release_notes/upcoming.md`, `SPECS/README.md`.

## Проверки

`go build ./...`; `TestPresetRuleConditionGate`, `TestContractCorpusForEach`,
`TestWizardTemplateConfigUnchanged` (core/build); `TestContractCorpusTemplate`,
`TestWalkerParityAgainstCorpus` (core/template); `TestRegistryCodeRefsResolve`,
`TestRegistryWarningsHaveCauseAndFix`, `TestRegistryBodyStructure`
(core/config) — зелёные. Эталоны `wizard_template_config` не изменились.

## Сверка с очисткой висячих ссылок (`preset_merge.go`, не менялось)

- `cleanDanglingRuleSetInRule` (применяется в `resolve_route.go` к правилам
  пресета после скачивания SRS): правило без `rule_set` возвращает как есть —
  с новой семантикой согласуется (голый sniff проходит). Но «условием» там
  считается любой ключ кроме `outbound`/`action`/`method`/`#…`: у пресетов
  `ai-services`, `messengers`, `games`, `ru-blocked` при нескачанном `.srs`
  ссылка снимается, а `network: [tcp, udp]` остаётся — правило совпадает со
  всем TCP/UDP (по чтению кода, не прогонялось). Это расхождение со случаем 2 (и с 1.1.82) было и до SPEC 152.
- `cleanDanglingDNSRule` (пользовательские DNS-правила): `server` считается
  совпадением, поэтому правило, у которого пропал единственный `rule_set`,
  остаётся и перехватывает все запросы. Тоже расхождение со случаем 2, было и
  раньше.

Оба места — отдельная задача по решению владельца.
