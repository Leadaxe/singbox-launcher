# IMPLEMENTATION REPORT 171

**Итог:** DNS-слот пресета появляется по оси его маршрутного правила, а не в хвосте `dns.rules`. Одна функция места `state.PresetDNSSlotIndex`, три вызывающих: `SyncDNSOptionsWithActivePresets` (сохранение, Debug API, новая установка), `ReconcileDNSRuleOrder` визарда (загрузка после засева, включение из Library, превью, сохранение) и Pass 4 `ResolveDNS` (сборка на состоянии без записи — теперь обход синхронизированной копии, fallback «в конец» снят). У существующего пользователя с `fakeip` поздний дефолт `tunnel-ingress` (940) встаёт перед `fakeip` (1130) — DNS-reject закрытых пиров срабатывает. Контракт **1.1.127**, решение **D-135**, `TEMPLATE_LANG.md` §6.10, LxBox — `TASKS_LXBOX.md` §124. Golden-конфиги байт в байт прежние.

## Как упорядочивались DNS-слоты до 171

- `state.DNS.Rules[]` — порядок пользователя позицией в слайсе, номера нет (SPEC 062). Маршрутные `state.Rules[]` — ось `num` (SPEC 106/113-C).
- Новый слот пресета создавался в трёх местах и везде дописывался в конец: `SyncDNSOptionsWithActivePresets` (`core/state/sync_dns.go`), `ReconcileDNSRuleOrder` pass 2 (`ui/configurator/models/dns_rule_slot.go`), «defensive fallback» Pass 4 `ResolveDNS` (`core/build/resolve_dns.go`, обход `map` — порядок нескольких неучтённых пресетов был недетерминирован).
- Новая установка получала порядок оси только потому, что все слоты создавались одним проходом по уже отсортированным `state.Rules`.

## Норма (D-135)

Новая запись `dns.rules[] {kind: preset, ref}` встаёт перед первым уже стоящим слотом пресета, чей `num` маршрутного правила больше; иначе в конец. Номер — `num` правила `rules[]` с тем же `ref` (неразмеченное — 1000). Пользовательские DNS-правила не учитываются и не двигаются; уже стоящие слоты не переупорядочиваются; равные номера — после. Норма про место появления, не про инвариант: дальше слот живёт порядком DNS-вкладки.

## Что сделано

- **Контракт.** `TEMPLATE_LANG.md`: новый §6.10, ссылки из §6.5.2 (пункт про DNS-reject) и §6.7.5, строка D-135 в §9. `BACKUP.md`: строка `rules[].ref` таблицы `dns` (слот, которого файл не несёт, создаёт синхронизация приёмника по §6.10; §9 п. 8 без изменений). `VERSION` → 1.1.127, строка changelog в `contract/README.md`, `TASKS_LXBOX.md` §124 (норма, что сделать в `seedLateDefaultPresets` и при включении пресета, критерий) и ссылка из §123 вместо «известного ограничения». `go generate ./contract/...` (index.md). Реестр и корпус не менялись.
- **Код.** `core/state/sync_dns.go`: `PresetDNSSlotIndex(n, num, slotNum)`, `presetRuleNums`, `insertDNSRule`; шаг 4 `SyncDNSOptionsWithActivePresets` вставляет по оси. `ui/configurator/models/dns_rule_slot.go`: `ReconcileDNSRuleOrder` pass 2 — слот пресета по `state.PresetDNSSlotIndex` (номер — `PresetRefs[i].Num`, nil → `DefaultRuleNum`), пользовательские слоты по-прежнему в конец; `presetRefNum`. `core/build/resolve_dns.go` Pass 4: обход копии `state.DNS.Rules`, пропущенной через `SyncDNSOptionsWithActivePresets` (состояние не мутируется), цикл fallback снят. Комментарии в `rule_order.go` (`SeedLateDefaultRules`).
- **Тест.** Единственный новый — `core/build/late_preset_dns_order_test.go` `TestLatePresetDNSRuleTakesAxisPosition`: боевой шаблон, `fakeip` включён, своё DNS-правило первым, `late_presets_seeded: [tailscale]`; после `SeedLateDefaultRules` — (а) синхронизация даёт `[user, tunnel-ingress, fakeip]`; (б) `MergePresetsIntoDNS` на состоянии без записи `tunnel-ingress` с endpoint'ом `wg-home` — правило пользователя первым, `{"action":"reject","inbound":["wg-home"]}` раньше `query_type: [A, AAAA]`.
- **Доки.** `docs/release_notes/upcoming.md` (EN/RU, Highlights + Technical), `docs/ARCHITECTURE.md` (поток `MergePresetsIntoDNS → ResolveDNS`), отчёт SPEC 170 — отметка «закрыто SPEC 171».

## Решения по умолчанию

- **Миграции нет, отметки в `state.json` нет** (SPEC §2.2): `tunnel-ingress` ни в одном релизе не выходил — у пользователей релизов слот появится уже по норме; `tailscale` у существующих остаётся где стоит (перенос вслепую сломал бы ручной порядок). Состояние dev-сборки, где `tunnel-ingress` уже дописан в хвост (между d0e96ebd и этим коммитом), лечится перетаскиванием в DNS-вкладке или удалением записи `{kind: preset, ref: tunnel-ingress}` из `dns.rules` в `state.json` — синхронизация создаст её по оси.
- **Pass 4 `ResolveDNS` — синхронизированная копия, а не вставка в `out.Rules`:** один алгоритм на все три точки, детерминированный порядок, инвариант memory == disk сохранён (копия). Поведение для состояний с полным `dns.rules` не меняется: Sync на таком списке — no-op.
- **Номер слота — из маршрутного правила, не из шаблона:** `PresetLite` шаблон не нужен, к моменту появления слота правило размечено (`SeedLateDefaultRules`, `NormalizeRuleOrder`, Library ставит номер шаблона).
- **Пользовательские слоты в `ReconcileDNSRuleOrder` — по-прежнему в конец:** их создаёт `addDNSUserRule`, место задаёт пользователь.
- `RELEASE_NOTES.md` не трогался: изменение — исправление порядка, не новый UX; пункт есть в `upcoming.md`.

## Проверки

`go build ./...`; `gofmt -l` по изменённым файлам пусто. По имени: `go test ./core/build -run 'TestLatePresetDNSRuleTakesAxisPosition|TestResolveDNS_|TestTailscalePresetBuild_TwoNodes|TestWizardTemplateConfigUnchanged|TestGoldenScenarios|TestContractCorpusNodeLoops'`, `go test ./core/state -run 'TestSync_|TestSeedLateDefaultRules|TestPresetRefHelpers'`, `go test ./ui/configurator/models -run 'TestReconcileDNSRuleOrder_|TestRebuildDNSRuleOrder_'`, `go test ./ui/configurator/presentation -run 'TestSyncDNSByOrderToState_|TestDNSRuleOrderFromStateRules_'` — зелёные, golden без правки эталонов. В новом коде нет builtin `min`/`max`, `slices`, `maps` (Win7). Полный прогон — CI `run_mode=tests`: см. ниже.
