# PLAN 172

Принцип: **поле — метаданные записи, зеркало — вывод сборки.** `dns` живёт в `state.Rule` рядом с `Body` и проходит все круги данных без потерь; в `dns.rules` состояния ничего не пишется — зеркала появляются только в `ResolveDNS`, одной функцией места (продолжение `PresetDNSSlotIndex`, SPEC 171).

Перед этапом 2 — ответы владельца на развилки SPEC §6 (по умолчанию — рекомендованные).

## Этап 1 — контракт

1. `contract/docs/TEMPLATE_LANG.md`: §6.11 (норма SPEC §2.2: поле, гейт, DNS-совместимые ключи, тела, место, сервер), ссылка из §6.10, строка D-137 в §9.
2. `contract/docs/ONE_NAMESPACE.md` §1 (строка «расширения приложения»), `contract/docs/BACKUP.md` (таблица `rules[]`, раздел «Поля стороны LxBox» — `rules[].dns` снять, `resolve` оставить с честным «игнорирует молча»).
3. `contract/schema/backup.schema.json` `$defs/rule/properties/dns` — типизированный объект; `contract/VERSION`, `contract/README.md`, `contract/TASKS_LXBOX.md` §127; `go generate ./contract/...`.

## Этап 2 — данные

4. **State.** `core/state/rule_types.go`: `type RuleDNS struct { Enabled bool \`json:"enabled"\`; ServerTag string \`json:"serverTag"\`; ForceIPv4 bool \`json:"forceIpv4,omitempty"\` }`, поле `Rule.DNS *RuleDNS \`json:"dns,omitempty"\``; комментарий — норма §6.11. Хелпер `RuleDNS.IsZero()` (ничего не включено и тег пуст → не писать). `core/state/record_clone.go` `cloneRule` — копия объекта.
5. **Бэкап.** `core/backup/file_keys_10.go` `lxboxRuleKeys10`: `dns` снять (поле канона), `resolve` оставить; импорт/экспорт `1.0` (`import10.go`, `export10.go`) идут через `state.Rule` — проверить, что объект доезжает; формат 0.12 (`core/backup/file.go` `ruleKeys`) — если у LxBox 0.12 та же форма, читать в `Rule.DNS`, иначе оставить игнор (решить по их `legacy_form_v0.dart`).
6. **Модель Конфигуратора.** `core/state/legacy_types.go` `CustomRule` — поле `DNS *RuleDNS \`json:"-"\``; `core/state/load_v6.go` `legacyCustomRulesFromV6` — перенос для `inline` и `srs`; `ui/configurator/models/rule_state.go` `RuleState.DNS`; место, где `CustomRule` становится `RuleState` (загрузка визарда) — перенос; `ui/configurator/models/preset_ref_sync.go` `customRuleStateToV6Rule` — `DNS` в обе ветки (`NewSrsRule`, `NewInlineRule`). Debug API `core/stateedit` (`/state/rules`) работает со `state.Rule` — поле едет само.

## Этап 3 — сборка

7. **Зеркала.** `core/build/rule_dns_mirror.go` (новый):
   - матчер и гейт — по спискам реестра `rule_dns_mirror_match` / `rule_dns_mirror_gate` над выпущенным маршрутным телом (у `srs` в нём уже `rule_set`); имён полей в коде нет. Пустой матчер → зеркала нет. Та же функция нужна окну правила (п. 11) — живёт в `core/build` экспортированной (`RuleDNSMirrorMatch`).
   - `ruleDNSMirrors(resolvedRoute *ResolvedRoute, rules []state.Rule) []ruleDNSMirror` — по выпущенным маршрутным правилам (`Active && Enabled`, не `Skipped`; источник `inline|srs`, номер — `ResolvedRouteRule.Num`), в порядке оси; у каждого — глушилка и/или зеркало (тело без `server`) и `ServerTag`.
   - `ResolvedRouteRule` получает ссылку на запись (`StableRuleID` уже есть в `InlineID`/`SrsID`) — по ней находится `Rule.DNS`.
8. **Место.** `core/build/resolve_dns.go` Pass 4 (обход синхронизированной копии `dns.rules`): перед каждым слотом пресета с номером `S` выпускаются ещё не выпущенные зеркала с номером `N < S` (это и есть «перед первым слотом с номером больше `N`»); оставшиеся — сразу после последнего слота пресета; слотов нет — все в конец. Номер слота — та же карта `presetRuleNums`, что у `PresetDNSSlotIndex` (`core/state/sync_dns.go`; экспортировать при необходимости). Источник правила — новый `DNSSourceRuleMirror` с именем правила для отчёта (`dnsRuleOwner`).
   - `ResolveDNS` получает разрешённый маршрут: `MergePresetsIntoDNS` строит `ResolveRouteWithGlobals` один раз (он уже нужен `CollectEmittedRouteRuleSetTags`, `build.go:326`) — переиспользовать, не звать второй раз.
9. **Сервер.**
   - Принудительное включение (развилка 2): в `ResolveDNS` (проход пользовательских серверов, `resolve_dns.go` ~384-404) сервер, на который ссылается выпускаемое зеркало, эмитится и выключенным.
   - Удалённый сервер: `repairDanglingDNSRefs` (`preset_merge.go` ~830) уже снимает правило; для источника `DNSSourceRuleMirror` — предупреждение в отчёт (`fragmentDropped(owner, fragmentKindDNSRule, "server")` по образцу `rule_set`), не только лог.
   - `cleanDanglingDNSRule` (SPEC 153) зеркала не задевает (теги из выпущенного тела) — оставить проход как есть.
10. **Превью.** `ui/configurator/business/create_config.go` идёт тем же путём сборки — зеркала в превью без отдельной работы; проверить.

## Этап 4 — интерфейс

11. `ui/configurator/dialogs/add_rule_dialog.go`: блок DNS под «Direction» (`inputContainer` ~964-986) — галка, `Select` сервера (`DNSEnabledTagOptions` + текущий тег), галка Force IPv4; гейт и пустой матчер считает **тот же** `ruleDNSMatch` (вынести в `core/state` или `ui/configurator/business`, чтобы UI и сборка не разошлись); SRS-подсказка; `saveRule` (~721-835) пишет `RuleState.DNS`. Вкладка JSON окна — объект `dns` рядом с телом.
12. `ui/configurator/tabs/rules_unified_rows.go` — метка DNS на строке.
13. `ui/configurator/tabs/dns_unified_rules.go` — строки-зеркала только для чтения на их месте (функция места из п. 8, вызванная на модели визарда); клик — окно правила (развилка 3). Во вкладке серверов — подсказка «used by rule …» у выключенного сервера, на который ссылаются (развилка 2).
14. `bin/locale/ru.json` — переводы новых ключей.

## Этап 5 — тест, доки

15. `core/build/rule_dns_mirror_test.go` — `TestRuleDNSMirror` по SPEC §4 п. 2; круг данных п. 3 — подтест там же (импорт `1.0` → `legacyCustomRulesFromV6` → `RuleState` → `customRuleStateToV6Rule` → экспорт).
16. `docs/release_notes/upcoming.md` (EN/RU), `RELEASE_NOTES.md`, `docs/ARCHITECTURE.md` (поток DNS), `IMPLEMENTATION_REPORT.md`.

## Проверки

- `go build ./...`; `gofmt -l` по изменённым файлам.
- По имени: `go test ./core/build -run 'TestRuleDNSMirror|TestResolveDNS_|TestLatePresetDNSRuleTakesAxisPosition|TestSrsRuleKeepsAxisPosition|TestGoldenScenarios|TestWizardTemplateConfigUnchanged' -count=1`; `go test ./core/state -run 'TestSync_' -count=1`; тесты бэкапа `1.0` по имени (`core/backup`).
- CI: `gh workflow run ci.yml --ref develop -f run_mode=tests`; упавшее — `gh run view <ID> --log-failed`.
- Win7: без builtin `min`/`max`, `slices`, `maps`.
- UI — сборка `./ui/...` и показ владельцу, тестов на UI нет.

## Границы

- Файлы: перечисленные выше, папка SPEC, `contract/**` по этапу 1.
- Не трогать: шаблон, реестр, корпус шаблона, порядок уже записанных `dns.rules`, `SyncDNSOptionsWithActivePresets` (зеркала в состояние не пишутся), `resolve{}`.

## Риски

- **Семантика `rule_set` с IP в DNS-правиле.** Набор `.srs` только из `ip_cidr` в DNS-правиле ядро на этапе запроса не сопоставит — зеркало ничего не поймает (у LxBox так же, отсюда их подсказка для SRS). Проверить на ядре `sing-box check` + живой запрос на одном наборе доменов и одном наборе IP; подсказку в UI оставить.
- **Совпадение с LxBox.** Их inline-зеркало ссылается на headless `rule_set` из всех матчеров тела (включая `ip_cidr`), наше копирует DNS-совместимые ключи. Для доменов и приложений результат один; расхождение — только в IP-части, которую DNS-запрос не проверяет. Зафиксировать в §127 как норму «IP-матчеры в зеркале не участвуют».
