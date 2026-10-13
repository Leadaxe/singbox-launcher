# IMPLEMENTATION_REPORT — SPEC 166

Дата: 2026-10-13. Статус: реализованы три волны, ждёт CI и живой проверки (T16). Контракт 1.1.118 → **1.1.119**.

## Коммиты (ветка `spec166`)

| sha | Что |
|---|---|
| `a2703227` | норма: `TEMPLATE_LANG.md` §2.1, §4.5.1, §6.5–6.7, D-130..D-132, N16, `TASKS_LXBOX.md` §116 |
| `e7daa91f` | корпус: `for_each/` → `for_each_node/` (+ `load_reject_*`), новый `for_each_node_do/`; `backup_var_skipped` причина `internal` |
| `f25d3d1a` | движок: `for_each_node` / `for_each_node_do`, `#filter`, `#do`/`#append`, `#final`, `unique`, `wizard_ui: internal`; `for_each` — ошибка загрузки; пресет `tailscale` на `for_each_node` |
| `615b128e` | норма: пустой собранный список → `[]` (§2.2), правило без условий снимает гейт фрагментов |
| `f7401f2e` | волна 2: пресет `node-domains-fresh-dns`, UI формы пресета, Skip presets, доки |
| `2070b788` | волна 3: пресет снят; `route.default_domain_resolver` объектом, переменная `dns_resolver_no_stale`, галка на вкладке DNS, контракт §6.9, корпус, эталон |

## По задачам

- **T1–T12 (волна 1)** — норма, корпус, движок, состояние/бэкап, раннер `TestContractCorpusNodeLoops`. Карта конвейера — `CODEMAP.md`.
- **T13 шаблон.** `bin/wizard_template.json:1099` — пресет из SPEC §4 дословно (поля переменных — диалект лаунчера `default`, не `default_value`), стоит в `presets[]` сразу за `traffic-processing`, перед `tailscale`. `num: 10`, `sortable`/`locked` не заданы — как у `tailscale` (945): пресет можно выключить, удалить и двигать. `sortable: false` не взят сознательно: он делает пресет неотчуждаемым (пере-засев на каждой сборке, D-050), а это рекомендация, не инвариант. `10` — свободный якорь сразу за системной головой (0), вне пользовательской зоны 1000–1100, уникален (`TestAxisAnchorsAreUnique`, `TestUserZoneIsFree` зелёные). `LateDefaultPresetIDs` += `node-domains-fresh-dns` (`core/state/rule_order.go:419`) — по TEMPLATE_LANG §6.7.5 существующие профили получают пресет один раз, удалённый не возвращается.
- **T14 UI.**
  - Форма пресета (`preset_ref_edit_dialog.go`): `wizard_ui: internal` — строки нет (:165); `text_list` — многострочное поле, по значению на строку (:253); Save → `uniqueTextListVars` (:551): у `text_list` с `unique` строки обрезаются, пустые отбрасываются, дубли снимаются в порядке первого появления — до общей нормализации записи. `fix`/`hidden` у переменной пресета, как и раньше, ничего не значат.
  - Вкладка Settings (`settings_tab.go:37`): глобаль с `wizard_ui: internal` не рисуется.
  - Вкладка JSON (путь прочитан, GUI не запускался): `buildPresetJSONPreview` → `ExpandPresetForView` → `ExpandPresetForNodes` с узлами последней эмиссии; у `for_each_node_do` тело раскрыто один раз — `dns_rules` с одним правилом и собранным массивом; «No matching nodes.» остаётся только у `for_each_node`; пустой итог у `_do` — предупреждение `template_fragment_dropped` над `{}`.
  - Подпись строки в Rules и DNS (`PresetServedTags`) — узлы, прошедшие `#filter`, для обоих ключей (сделано в волне 1, комментарии переписаны).
  - «Skip presets» (`source_skip_presets.go`): переключатель виден, только если есть пресет с обходом, чей `#filter` пропускает узел без `skip_presets` и отсекает с ним; переменные пресета берутся из его правила. Иначе с новым пресетом переключатель появился бы у каждого узла с доменом и ничего бы не делал — фильтр пресета `skip_presets` не читает.
  - Комментарии `for_each` в `ui/**` переписаны. Новых ключей `locale.T` нет (строки пресетов шаблона не локализуются).
- **T15 доки.** `docs/release_notes/upcoming.md` (EN/RU: пункт в Highlights/Основное и в Technical/Техническое; поправлен невыпущенный пункт про вкладку Preset — `for_each` → `for_each_node`), `docs/ARCHITECTURE.md` (пункт «Node loops»), `docs/TEMPLATE_REFERENCE.md` / `.ru.md` (§4.5 `text_list`, `unique`, `internal`; новый §4.5.1 обход узлов; снята фраза про «потенциальный `#for_each`»).

## Волна 3 — резолвер по умолчанию объектом (T17)

**Находка.** Пресет волны 2 цели не достигает. У исходящего с `domain_resolver` — а `route.default_domain_resolver` шаблон задаёт всегда — ядро резолвит адрес сервера узла прямо этим DNS-сервером и DNS-правила не смотрит (`sing-box-lx/dns/router.go:1302`, `if options.Transport != nil { r.client.Lookup(...) }`). Правило с доменами узлов на их резолв не влияло. Рычаг — объектная форма ключа резолвера (`sing-box-lx/option/outbound.go:114`, `DomainResolveOptions`).

- **Шаблон.** Пресет `node-domains-fresh-dns` снят из `presets[]` и из `LateDefaultPresetIDs` (`core/state/rule_order.go:419`); в `develop` он не попадал, переноса для состояний не нужно. Глобальная переменная `dns_resolver_no_stale` (`bin/wizard_template.json:595`; bool, `default_value: "true"`, `wizard_ui: fix`) рядом с `dns_default_domain_resolver`; `route.default_domain_resolver` (`:676`) — `{"server": "@dns_default_domain_resolver", "disable_optimistic_cache": "@dns_resolver_no_stale"}`.
- **Подстановка bool.** По TEMPLATE_LANG §2.2 `bool` → JSON-литерал, ключ не снимается: включено — `{"server": "direct_dns_resolver", "disable_optimistic_cache": true}`, выключено — `{"server": "direct_dns_resolver", "disable_optimistic_cache": false}`. Ядро принимает обе (`UnmarshalJSON` объекта требует только непустой `server`).
- **Читатели и подмена.** `MergeRouteSection` (`core/build/route_merge.go:156`) писал выбранный на вкладке DNS резолвер строкой поверх ключа — объект терялся; теперь меняет только `server` (`resolverTarget`/`setResolverTarget` из `dns_detour_sanitize.go`, которые обе формы уже понимали — замена выпавшего сервера при сборке). `extractDefaultDomainResolver` (`core/template/loader.go:716`) и `routeDefaultDomainResolver` (`ui/configurator/business/wizard_dns.go:204`) на объекте отдавали `fmt.Sprint(map)` — теперь `server`. Выпадающий список резолвера на вкладке DNS читает переменную `dns_default_domain_resolver` (строка) — не затронут.
- **UI.** Галка «Never serve stale answers for node hostnames» под строкой Final/Default domain resolver (`ui/configurator/tabs/dns_tab.go:356`), подсказка — tooltip; подпись и подсказка — литералы каталога, совпадающие с title/tooltip переменной (как у настроек кэша DNS, чтобы их видел l10n_check и был перевод); значение в `SettingsVars`, изменение → `MarkAsChanged`; обновление — цепочкой к `RefreshDNSCacheSettings`; без переменной в шаблоне строка скрыта. Перевод — `bin/locale/ru.json:4739`.
- **Бэкап и контракт.** Переменная переносимая: `contract/registry/vars.json:59`, `core/backup/portable_vars.go:17` (сверка — `TestPortableVarsMatchRegistry`). Контракт остаётся 1.1.119: TEMPLATE_LANG §6.7.4 переименован в «Пример `for_each_node_do` (в шаблон не входит)» с причиной, новый §6.9 (объект, таблица переменных, правило «читатель понимает обе формы, подмена меняет только `server`»); README строка 1.1.119 — пункт (7); TASKS_LXBOX §116 — повод уточнён, п. 10 и пункт в «Что сделать LxBox». Корпус: `subst/default_resolver_object_defaults`, `subst/default_resolver_object_off`. BACKUP.md переносимые переменные поимённо не перечисляет; кейсов корпуса бэкапа, перечисляющих их, нет.
- **Эталон.** `core/build/testdata/wizard_template_config/*.json` (32 файла) перегенерирован осознанно (`-update`): единственное изменение — `"default_domain_resolver": "direct_dns_resolver"` → объект с `disable_optimistic_cache: true`; сценарий `globals_off` получил `dns_resolver_no_stale: "false"` и фиксирует форму с `false`.
- **Доки.** `upcoming.md` — пункт про пресет в Highlights/Основное заменён пунктом про галку и объектную форму; Technical пресет не называл. `TEMPLATE_REFERENCE*.md` §4.5.1 — без ссылки на пресет, «DNS scalars» — объектная форма; `WIZARD_TEMPLATE*.md` — поля объекта.

## Решения сверх SPEC

1. Отказ загрузки обхода (`for_each`, `node_type`, `as`, оба ключа, `#do` не там, неизвестный глагол и т. п.) снимает **пресет целиком** (`PresetWarning{Action: "skip"}`), а не весь шаблон. Ошибки глобалей (`node` в имени, `unique` не у `text_list`) — ошибка загрузки всего шаблона, как прочие ошибки `vars`.
2. `text_list` переменной пресета делится по переводу строки (как у глобали, TEMPLATE_LANG §2.2); прежний сплит по запятой снят.
3. Правая часть `#append` — ссылка `"@…"`, `{"#tpl": …}` или **литеральная строка** (норма не запрещает; объект/массив — отказ).
4. Пустой собранный список подставляется как `[]` (ключ не снимается); правило, у которого не осталось условий, снимает гейт фрагментов с `template_fragment_dropped`.
5. `dns_servers` у `for_each_node_do` — типизированные, с пространством тегов пресета (`<preset_id>:<tag>`), как у обычного пресета; сырые серверы с `#tpl`-тегами — только у `for_each_node`.
6. Отсутствующий путь `@node.*` в условии — ложь **без** предупреждения в лог (`substitute.go:483` `warnUnknownPredicateVar`): иначе каждый узел без `server` (Tailscale, WG) писал WARN на каждой сборке.
7. Пустой `default` у `text_list` переменной пресета — не предупреждение загрузчика (`preset_loader.go:388`).
8. Форма пресета: дедуп `unique` заодно нормализует строки (trim, без пустых) — то же, что делает сборка при чтении.
9. «Skip presets» показывается только там, где он что-то меняет (см. T14).

## Проверки (только по имени)

- Волна 3: `go build ./...` — ок; `go test ./core/build -run '^(TestWizardTemplateConfigUnchanged|TestGoldenScenarios|TestContractCorpusNodeLoops|TestContractCorpusDNS|TestMergeRouteSection.*)$'`, `go test ./core/template -run '^(TestResolveTemplate|TestGateMigrationProducesIdenticalConfig|TestContractCorpusTemplate)$'`, `go test ./core/state -run '^TestSeedLateDefaultRules$'`, `go test ./core/backup -run '^(TestBackupCorpus|TestImportVarsPortableOnly|TestPortableVarsMatchRegistry)$'`, `go test ./core/config/subscription -run '^TestRegistryWarningCodesAreActuallySet$'` — ок.

- `go build ./...` — ок; `go test -c` пакетов `ui/configurator/{tabs,business,presentation}`, `core/state`, `core/template` — компилируются (не запускались).
- `go test ./core/build -run 'TestContractCorpusNodeLoops|TestTailscalePresetBuild_TwoNodes|TestAxisAnchorsAreUnique|TestUserZoneIsFree|TestTrafficProcessingIsReseeded|TestWizardTemplateConfigUnchanged|TestGoldenScenarios'` — ок.
- `go test ./core/template -run 'TestResolveTemplate|TestGateMigrationProducesIdenticalConfig|TestContractCorpus'`, `go test ./core/state -run TestSeedLateDefaultRules`, `go test ./core -run TestSpec129RecordVarsKeepConfigByteIdentical` — ок.
- Разовый тест (удалён, не закоммичен) на боевом шаблоне: узлы `a.example`, `1.2.3.4`, `a.example`, `2001:db8::1`, Tailscale + ввод `extra.example\nextra.example\na.example` → `{"domain":["extra.example","a.example"],"server":"direct_dns_resolver","disable_optimistic_cache":true}`; без ввода → `["a.example"]`; без узлов → правило снято; обслуживаемые узлы — `a`, `c`.

## Что осталось

- **CI** `gh workflow run ci.yml --ref develop -f run_mode=tests` — после слияния в `develop` (T16).
- **Живая проверка** (волна 3): галка на вкладке DNS, в собранном `config.json` — `route.default_domain_resolver` объектом; выбор другого резолвера сохраняет `disable_optimistic_cache`. Форма пресета, вкладка JSON, подпись узлов в Rules/DNS (волна 2, на `tailscale`) — смотрит владелец.
- ~~**Место правила в DNS**~~ и ~~**предупреждение в отчёте сборки**~~ — сняты вместе с пресетом (волна 3).
- (история волны 2) **Место правила в DNS.** `num` упорядочивает только ось `state.Rules`; порядок DNS-правил — отдельный `state.DNS.Rules`/`DNSRuleOrder`. На чистой установке пользовательских DNS-правил нет, слоты пресетов идут в порядке правил — пресет первым среди DNS-правил (до `tailscale`, `russian`, `fakeip`). У существующего профиля поздний засев добавляет слот **в конец** DNS-правил (`ReconcileDNSRuleOrder`, `SyncDNSOptionsWithActivePresets`, запасной путь `resolve_dns.go`) — после пользовательских правил и `fakeip`; пользователь может перетащить. Поставить новый слот пресета по `num` среди слотов — общая правка порядка DNS для всех пресетов, вне SPEC; решение владельца.
- **Предупреждение в отчёте сборки.** Пресет включён по умолчанию; у профиля без доменных узлов и без своих доменов каждая сборка кладёт в отчёт `template_fragment_dropped` (owner `node-domains-fresh-dns`, `reason: rule_set` — подпись причины унаследована от общего гейта и вводит в заблуждение). Норма это поведение принимает (§2.6); если шумно — решение владельца.
- `unique` у глобали в Settings применяется только при чтении сборкой (поле сохраняется на каждый ввод, дедуп по ходу набора ломал бы ввод); глобалей с `unique` в шаблоне нет.
- `RELEASE_NOTES.md` не трогался — выжимка делается при выпуске.
