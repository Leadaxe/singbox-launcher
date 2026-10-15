# IMPLEMENTATION REPORT 172

**Итог:** собственное правило маршрута (`rules[]` вида `inline` | `srs`) несёт объект `dns` в форме LxBox — `{enabled, serverTag, forceIpv4}` — и сборка выводит из него DNS-зеркало: глушилку AAAA и/или DNS-правило на выбранный сервер, на месте по оси перед первым слотом пресета с бо́льшим номером. Поле больше не теряется: импорт бэкапа (1.0 и 0.x), `state.json`, Конфигуратор и экспорт проводят его без потерь. Контракт **1.1.130**, решение **D-137**, норма `TEMPLATE_LANG.md` §6.11, списки реестра `rule_dns_mirror_match` / `rule_dns_mirror_gate`, LxBox — `TASKS_LXBOX.md` §127. Повод — issue #140. Развилки SPEC §6 решены владельцем 15.10.2026 — все по умолчанию.

## Что было до 172

- Контракт объявлял `rules[].dns` полем LxBox и в таблице `rules[]` обещал «лаунчер не применяет, но провозит»; раздел «Поля стороны LxBox» и код говорили обратное: `state.Rule` поля не имел, импорт 1.0 отбрасывал его (`lxboxRuleKeys10`), импорт 0.x не читал (`types.go` держал сырой `DNS`, `importRule` его не переносил), сохранение Конфигуратора пересобирало правило из `RuleState`. DNS-опции LxBox, прошедшие через лаунчер, возвращались пустыми.
- Своё правило DNS не вело: пользователь заводил отдельное DNS-правило со ссылкой на сгенерированный тег набора (`user:<имя>`) — так и возник #140.

## Что сделано

- **Контракт** (`19a03c17`, `a426ab4d`). `TEMPLATE_LANG.md` §6.11 (новый, ссылка из §6.10), строка D-137 в §9; `ONE_NAMESPACE.md` §1 — `dns{}` поле обеих сторон, `resolve{}` LxBox; `BACKUP.md` — строка `dns` таблицы `rules[]` (форма, camelCase как исключение, импорт — запись целиком), `resolve` — «игнорирует молча и не пишет» вместо «провозит»; `schema/backup.schema.json` — `$defs/rule/properties/dns` типизирован; `registry/allowlists.json` — `rule_dns_mirror_match` (`rule_set`, `domain*`, `process_*`, `package_name*`, `user*`, `wifi_*`, `network_type`, `source_ip_*`, `inbound`) и `rule_dns_mirror_gate` (`port`, `port_range`, `protocol`, `network`); `VERSION` 1.1.130, changelog, `TASKS_LXBOX.md` §127; `go generate ./contract/...`. Корпус не менялся.
- **Данные** (`e8303b02`). `state.RuleDNS` (`IsZero`, `CloneRuleDNS`), `Rule.DNS` (`json:"dns,omitempty"`), `cloneRule`; `lxboxRuleKeys10` — без `dns`; импорт 0.x (`legacy_read_0x.go`) читает `dns` у `inline`/`srs`; `CustomRule.DNS` (`json:"-"`) ← `legacyCustomRulesFromV6`; `RuleState.DNS` ← `PersistedCustomRuleToRuleState`, → `ToPersistedCustomRule`, `customRuleStateToV6Rule` (обе ветки). Debug API (`core/stateedit`) работает со `state.Rule` — поле едет само.
- **Сборка** (`2ab9d670`). `core/build/rule_dns_mirror.go`: `RuleDNSMirrorMatch` (гейт и матчер по спискам реестра над выпущенным маршрутным телом — у `srs` в нём уже `rule_set`; имён полей и видов записи в коде нет), `collectRuleDNSMirrors` (только выпущенные правила: включены, наборы скачаны), `RuleDNSMirrorIndex` и `insertRuleDNSMirrors` (место по §6.11), `ruleDNSMirrorsForMerge` (маршрут разрешается, только если хоть одно правило несёт опцию). `ResolvedRouteRule` получил `Name` и `DNS`; новый источник `DNSSourceRule`; `state.PresetRuleNums` экспортирован. `MergePresetsIntoDNS`: сервер, нужный зеркалу, эмитится и выключенным; зеркало на отсутствующий сервер не выпускается, в отчёт — `template_fragment_dropped {owner: <имя правила>, kind: dns.rules, reason: server}`; зеркала вставляются до прохода SPEC 153 (их `rule_set` всегда выпущен). Превью Конфигуратора идёт тем же путём.
- **UI** (`466280de`). Окно правила: блок DNS под вкладками Form/JSON — «Send DNS to a dedicated server» + выбор сервера (`DNSEnabledTagOptions` + текущий тег), «Force IPv4 (drop AAAA)»; доступность — `build.RuleDNSMirrorMatch` по телу текущей вкладки, подсказки про гейт и про SRS; значения при недоступности не сбрасываются. Rules — метка «· DNS: tag · IPv4 only» у правила, чьё зеркало сборка выпустит. DNS — строки 📎 только для чтения на месте зеркала (`build.RuleDNSMirrorIndex` по слотам модели), вне перетаскивания; у выключенного сервера, нужного зеркалу, — 📎 и подсказка «Used by rule …». 9 ключей `locale.T` с переводом в `bin/locale/ru.json`.
- **Тест.** `core/build/rule_dns_mirror_test.go` `TestRuleDNSMirror`: (1) круг данных — бэкап 1.0 LxBox с `dns` у `inline` и `srs` → `ImportFile` → `state.Save` / `state.Load` (+ вид Конфигуратора `CustomRules`) → `Export10`: объекты те же байт в байт; (2) сборка на боевом шаблоне — правило пользователя первым, `corp` (`domain_suffix` + `ip_cidr`) → зеркало без `ip_cidr` на выключенный `cf_doh`, `lists` (srs) → глушилка раньше зеркала, оба перед слотом `fakeip` (1130); `ports` (гейт), `ip-only` (пустой матчер), `gone` (удалённый сервер) не выпущены; `cf_doh` в `dns.servers`; предупреждение о `gone`.
- **Доки.** `docs/release_notes/upcoming.md` (EN/RU, Highlights + Technical), `RELEASE_NOTES.md` (черновик, кратко), `docs/ARCHITECTURE.md` (поток DNS).

## Найдено по пути

- **SRS-правило вставало выше всех в `route.rules`** (`7cc4bcc7`, вне SPEC, отдельным коммитом): `resolveSrsRouteRule` не ставил `Num`, номер 0 считался системным якорем, и правило уходило в голову списка — выше своих inline-правил и пресетов, стоящих раньше в списке Rules. Тест `TestSrsRuleKeepsAxisPosition`. Без исправления место зеркала (по номеру) и место маршрутного правила расходились бы.

## Отступления от SPEC/PLAN

- **Списки ключей — в реестре**, а не перечнем в норме/коде (правило репо: в коде движка нет имён полей). Добавлены в тот же 1.1.130 до пуша. `rule_set` вошёл в список матчера — поэтому `srs` и `inline` идут одним кодом. В список добавлен `package_name_regex` (есть в `dns_rule_conditions`).
- **JSON-вкладка окна правила `dns` не показывает:** там тело sing-box как есть, метаданные записи — вне `body` (ONE_NAMESPACE §1). Блок DNS стоит под вкладками и виден на обеих.
- **Строка-зеркало во вкладке DNS не открывает окно правила** (у вкладки DNS нет функции открытия окна правила) — подсказка говорит, где править.
- **Импорт 0.x тоже читает `dns`** — форма LxBox 0.x та же (`legacy_form_v0.dart`: `RuleDns.fromJson(j['dns'])`).

## Не проверено

- **Живое ядро:** `sing-box check` и реальные запросы на конфиге с глушилкой `{ip_version: 6, action: predefined, rcode: NOERROR}` и зеркалом по `rule_set` из одних IP (ожидается: зеркало ничего не ловит — подсказка в UI об этом). Риск из PLAN остаётся открытым до прогона на ядре.
- **UI** — смотрит владелец: блок DNS в окне правила (доступность, подсказки, сохранение), метка в Rules, строки 📎 во вкладке DNS, 📎 у выключенного сервера.
- **CI** `run_mode=tests` — после пуша.
- LxBox — §127 ждёт сверки (порядок при уплотнённом блоке, матчер inline против их headless `rule_set`, гейт по `network`).

## Проверки

`go build ./...`; `gofmt -l` по изменённым файлам пусто. По имени: `go test ./core/build -run 'TestRuleDNSMirror|TestResolveDNS_|TestLatePresetDNSRuleTakesAxisPosition|TestSrsRuleKeepsAxisPosition|TestGoldenScenarios|TestWizardTemplateConfigUnchanged|TestTailscalePresetBuild_TwoNodes'`, `go test ./core/state -run 'TestSync_'`, `go test ./core/backup -run 'TestLxboxFields|Corpus'`, `go test ./ui/configurator/models -run 'TestEmitStateRulesWithoutOrder_|TestReconcileDNSRuleOrder_|TestRebuildDNSRuleOrder_|TestSaveEmitsRulesSortedByAxis|…'`, `go test ./ui/configurator/presentation -run 'TestSyncDNSByOrderToState_|TestDNSRuleOrderFromStateRules_'` — зелёные, golden без правки эталонов. В новом коде нет builtin `min`/`max`, `slices`, `maps` (Win7).
