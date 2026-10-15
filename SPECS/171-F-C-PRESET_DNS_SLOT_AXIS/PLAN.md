# PLAN 171

Принцип: **одна функция места, три вызывающих.** Позицию нового слота считает один хелпер в `core/state`; все точки, где слот пресета создаётся, зовут его вместо `append`.

## Этап 1 — контракт (1.1.127)

1. `contract/docs/TEMPLATE_LANG.md`: §6.10 (норма D-135), правки §6.5.2 и §6.7.5, строка D-135 в §9.
2. `contract/docs/BACKUP.md`: строка `rules[].ref` таблицы `dns`.
3. `contract/VERSION` → 1.1.127; `contract/README.md` changelog; `contract/TASKS_LXBOX.md` §124 (+ ссылка из §123); `go generate ./contract/...`.

## Этап 2 — код

4. **Хелпер.** `core/state/sync_dns.go`: `PresetDNSSlotIndex(n, num int, slotNum func(i int) (int, bool)) int` — индекс первого слота `i < n`, у которого `slotNum(i)` даёт `ok && n_i > num`; нет — `n`. Пользовательский слот — `ok=false`, пропускается. Рядом `presetRuleNums(rules []Rule) map[string]int` — номер маршрутного правила по `ref` (`ruleNum`, неразмеченное = `DefaultRuleNum`).
5. `SyncDNSOptionsWithActivePresets`, шаг 4: отсутствующая запись вставляется в `keptRules` по `PresetDNSSlotIndex` (номера существующих слотов — из той же карты; слот пресета без маршрутного правила в `rules` сюда не попадает — его уже снял фильтр). Обход `rules` в порядке списка, как сегодня.
6. `ui/configurator/models/dns_rule_slot.go` `ReconcileDNSRuleOrder`, pass 2: слот пресета, которого нет в `kept`, вставляется по `state.PresetDNSSlotIndex`; номер слота — `PresetRefs[idx].Num` (nil → `state.DefaultRuleNum`). Пользовательские слоты, как и раньше, дописываются в конец (их создаёт `addDNSUserRule`, место задаёт пользователь).
7. `core/build/resolve_dns.go` Pass 4: вместо обхода `state.DNS.Rules` с «defensive fallback» в конце — обход **синхронизированной копии**: `dnsRules := state.DNS.Rules` → копия `DNSOptions{Rules: clone}` → `SyncDNSOptionsWithActivePresets(state.Rules, &copy, template.PresetLiteMap(td.Presets))` → обход `copy.Rules`. Состояние не мутируется (инвариант memory == disk сохранён), fallback-цикл с `map`-обходом снимается: все активные пресеты с DNS-правилом уже в списке, порядок детерминирован.
8. Комментарии в `sync_dns.go` (шапка «Семантика»), `dns_rule_slot.go` (`ReconcileDNSRuleOrder`), `resolve_dns.go` (Pass 4), `rule_order.go` (`SeedLateDefaultRules` — отсылка к норме).

## Этап 3 — тест, доки

9. `core/build/late_preset_dns_order_test.go` — `TestLatePresetDNSRuleTakesAxisPosition` по SPEC §4 п. 2 (образец — `tailscale_preset_build_test.go`: боевой шаблон через `template.ParseTemplateData`, `ParsedCache` с endpoint'ом, `CollectPresetNodes`, `MergePresetsIntoDNS`).
10. `docs/release_notes/upcoming.md` (EN/RU: Highlights + Technical), `docs/ARCHITECTURE.md` (строка потока `MergePresetsIntoDNS → ResolveDNS`), `SPECS/170-F-C-TUNNEL_INGRESS/IMPLEMENTATION_REPORT.md` (ссылка на 171), `IMPLEMENTATION_REPORT.md`.

## Проверки

- `go build ./...`; `gofmt -l` по изменённым файлам.
- По имени: `go test ./core/build -run 'TestLatePresetDNSRuleTakesAxisPosition|TestResolveDNS_|TestTailscalePresetBuild_TwoNodes|TestWizardTemplateConfigUnchanged|TestGoldenScenarios' -count=1`; `go test ./core/state -run 'TestSync_|TestSeedLateDefaultRules' -count=1`; `go test ./ui/configurator/models -run 'TestReconcileDNSRuleOrder_|TestRebuildDNSRuleOrder_' -count=1`; `go test ./ui/configurator/presentation -run 'TestSyncDNSByOrderToState_|TestDNSRuleOrderFromStateRules_' -count=1`.
- CI: `gh workflow run ci.yml --ref develop -f run_mode=tests`; упавшее — `gh run view <ID> --log-failed`.
- Win7: без builtin `min`/`max`, `slices`, `maps`.

## Границы

- Файлы: перечисленные выше и папка SPEC. UI-вёрстка не меняется.
- Не трогать: шаблон, реестр, корпус, `SeedLateDefaultRules` (логика засева), `DNSRuleOrderFromStateRules`, `RebuildDNSRuleOrder`, порядок уже записанных `dns.rules`.
