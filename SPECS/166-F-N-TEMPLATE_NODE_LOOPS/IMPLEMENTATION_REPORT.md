# IMPLEMENTATION_REPORT — SPEC 166

Дата: 2026-10-13. Статус: реализовано обе волны, ждёт CI и живой проверки (T16). Контракт 1.1.118 → **1.1.119**.

## Коммиты (ветка `spec166`)

| sha | Что |
|---|---|
| `a2703227` | норма: `TEMPLATE_LANG.md` §2.1, §4.5.1, §6.5–6.7, D-130..D-132, N16, `TASKS_LXBOX.md` §116 |
| `e7daa91f` | корпус: `for_each/` → `for_each_node/` (+ `load_reject_*`), новый `for_each_node_do/`; `backup_var_skipped` причина `internal` |
| `f25d3d1a` | движок: `for_each_node` / `for_each_node_do`, `#filter`, `#do`/`#append`, `#final`, `unique`, `wizard_ui: internal`; `for_each` — ошибка загрузки; пресет `tailscale` на `for_each_node` |
| `615b128e` | норма: пустой собранный список → `[]` (§2.2), правило без условий снимает гейт фрагментов |
| `f7401f2e` | волна 2: пресет `node-domains-fresh-dns`, UI формы пресета, Skip presets, доки |

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

- `go build ./...` — ок; `go test -c` пакетов `ui/configurator/{tabs,business,presentation}`, `core/state`, `core/template` — компилируются (не запускались).
- `go test ./core/build -run 'TestContractCorpusNodeLoops|TestTailscalePresetBuild_TwoNodes|TestAxisAnchorsAreUnique|TestUserZoneIsFree|TestTrafficProcessingIsReseeded|TestWizardTemplateConfigUnchanged|TestGoldenScenarios'` — ок.
- `go test ./core/template -run 'TestResolveTemplate|TestGateMigrationProducesIdenticalConfig|TestContractCorpus'`, `go test ./core/state -run TestSeedLateDefaultRules`, `go test ./core -run TestSpec129RecordVarsKeepConfigByteIdentical` — ок.
- Разовый тест (удалён, не закоммичен) на боевом шаблоне: узлы `a.example`, `1.2.3.4`, `a.example`, `2001:db8::1`, Tailscale + ввод `extra.example\nextra.example\na.example` → `{"domain":["extra.example","a.example"],"server":"direct_dns_resolver","disable_optimistic_cache":true}`; без ввода → `["a.example"]`; без узлов → правило снято; обслуживаемые узлы — `a`, `c`.

## Что осталось

- **CI** `gh workflow run ci.yml --ref develop -f run_mode=tests` — после слияния в `develop` (T16).
- **Живая проверка** на профиле с доменными узлами: в собранном `config.json` одно DNS-правило с массивом их доменов; форма пресета, вкладка JSON, подпись узлов в Rules/DNS — смотрит владелец.
- **Место правила в DNS.** `num` упорядочивает только ось `state.Rules`; порядок DNS-правил — отдельный `state.DNS.Rules`/`DNSRuleOrder`. На чистой установке пользовательских DNS-правил нет, слоты пресетов идут в порядке правил — пресет первым среди DNS-правил (до `tailscale`, `russian`, `fakeip`). У существующего профиля поздний засев добавляет слот **в конец** DNS-правил (`ReconcileDNSRuleOrder`, `SyncDNSOptionsWithActivePresets`, запасной путь `resolve_dns.go`) — после пользовательских правил и `fakeip`; пользователь может перетащить. Поставить новый слот пресета по `num` среди слотов — общая правка порядка DNS для всех пресетов, вне SPEC; решение владельца.
- **Предупреждение в отчёте сборки.** Пресет включён по умолчанию; у профиля без доменных узлов и без своих доменов каждая сборка кладёт в отчёт `template_fragment_dropped` (owner `node-domains-fresh-dns`, `reason: rule_set` — подпись причины унаследована от общего гейта и вводит в заблуждение). Норма это поведение принимает (§2.6); если шумно — решение владельца.
- `unique` у глобали в Settings применяется только при чтении сборкой (поле сохраняется на каждый ввод, дедуп по ходу набора ломал бы ввод); глобалей с `unique` в шаблоне нет.
- `RELEASE_NOTES.md` не трогался — выжимка делается при выпуске.
