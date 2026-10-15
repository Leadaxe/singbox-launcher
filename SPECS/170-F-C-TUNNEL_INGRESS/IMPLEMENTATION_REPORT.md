# IMPLEMENTATION REPORT 170

**Итог:** пиры клиентских туннелей (WireGuard/AWG, OpenVPN, OpenConnect, Tailscale) больше не ходят через устройство без разрешения: штатный пресет `tunnel-ingress` (`num: 940`, поздний дефолт) отклоняет входящее из туннеля, кроме разрешённого тремя новыми полями записи узла `allow_local` / `allow_lan` / `allow_gateway`. `skip_presets` сужен до пресета `tailscale` и переименован в `skip_tailscale_presets` с терпимым чтением старого ключа. Контракт 1.1.126, решение D-134, LxBox — `TASKS_LXBOX.md` §123. Golden-конфиги байт в байт прежние.

## Что сделано

- **Контракт (этап 1).** `TEMPLATE_LANG.md`: §6.5 (пример, список кейсов), §6.5.1 (фильтр `tailscale` на новое имя, абзац о переименовании), **§6.5.2 — новый раздел с телом пресета `tunnel-ingress` целиком** и обоснованием каждого решения, §6.6 (таблица полей записи, терпимое чтение), §6.7.1 (пример `#do`), §6.7.5 (поздние дефолты), §9 (строка D-134). `BACKUP.md` (строка `skip_tailscale_presets` + три `allow_*`), `schema/backup.schema.json` (`$defs/sourceServer` и `$defs/node`: новые поля, `skip_presets` с `deprecated: true`), `registry/presets.json` (`tunnel-ingress`), `registry/protocols/tailscale.json` (note), `NODE_SECTIONS.md` §6. `VERSION` → 1.1.126, строка changelog в `contract/README.md`, `TASKS_LXBOX.md` §123 (самодостаточный: зачем, поля и тумблеры с английскими текстами, переименование и терпимое чтение, тело пресета, тонкости переноса, что сделать в Dart, корпус-критерий), `go generate ./contract/...` (index.md).
- **Корпус.** `for_each_node/*` (6) и `for_each_node_do/do_if_*` (9) — поле `nodes[].skip_tailscale_presets` и фильтры на новое имя, ожидания не менялись. Новые `for_each_node_do/tunnel_ingress_mixed`, `tunnel_ingress_no_nodes`. README корпуса — формат `nodes[]`.
- **Пайплайн (этап 2).** `configtypes.NodeRecordFlags{SkipTailscalePresets, AllowLocal, AllowLan, AllowGateway}` с `Any()` и `MergeTrue()`; один тип на весь путь: `state.Node` (встроен, JSON-ключи записи) → `CanonicalNode.Record` (у узла подписки — нули, `adapter_source.go`) → `ParsedNode.Record` (`canonical_emit.go`) → `OutboundGenerationResult.RecordFlags` → `ParsedCache.RecordFlags` (`rebuild_snapshot.go`) / `WizardModel.GeneratedRecordFlags` (превью) → `template.PresetNode.Record` (`CollectPresetNodes`) → `@node.<поле>` (`PresetNodeRecordFields` — закрытый перечень «имя → чтение поля», `NewNodeScope` кладёт четыре bool).
- **Терпимое чтение.** `state.Node.LegacySkipPresets` (`json:"skip_presets,omitempty"`, только чтение) и метод `LiftLegacyRecordFlags()`; состояние — `normalizeNodeRecordFlags` (`load_normalize.go`) из `parseV8`; бэкап — `Source10.LegacySkipPresets` + подъём у корневой записи и у каждого члена папки в `decode10Source`. После подъёма легаси-поле ложно и не пишется.
- **Бэкап.** `Source10` встраивает `NodeRecordFlags`; экспорт пишет только `true`; слияние совпавшего по телу узла (корень и член папки) — `MergeTrue` для всех четырёх полей.
- **Шаблон (этап 3).** `bin/wizard_template.json`: фильтр `tailscale` → `@node.skip_tailscale_presets`; пресет `tunnel-ingress` ровно с телом SPEC §2.3 сразу после `tailscale` (файл пересобран `json.dumps(indent=1)` — иного диффа форматирования нет). `state.LateDefaultPresetIDs = ["tailscale", "tunnel-ingress"]`.
- **UI.** `source_skip_presets.go`: «Skip Tailscale presets», только у тела `type: tailscale` (проба «пресет узел обслуживает» прежняя). Новый `source_tunnel_access.go`: блок «Incoming connections from this tunnel» с подсказкой и тремя `ttwidget.NewCheck` с тултипами (у `allow_local` — предупреждение про `127.0.0.1`), у Tailscale — строка про `advertise_routes`/`advertise_exit_node`; виден у своего сервера и члена папки с `body.type` ∈ {wireguard, openvpn-client, openconnect, tailscale}, правит рабочую копию узла. Подключён в `source_edit_window.go` после «Skip Tailscale presets». Ключи локали с переводом в `bin/locale/ru.json` (старые два ключа заменены).
- **Тесты.** Единственный новый — `TestLoadLegacySkipPresetsKey` (две функции с одним именем: `core/state/record_flags_legacy_test.go` — загрузка/сохранение `state.json`; `core/backup/skip_presets_test.go` — импорт бэкапа со старым ключом). Существующие тесты, ссылавшиеся на `SkipPresets`, переведены на новые имена без изменения сути (`TestSkipPresetsRoundTrip`, `TestTailscalePresetBuild_TwoNodes`, `TestPresetNodesForView_MatchesBuildSelection`, превью `for_each`), раннер корпуса `contract_node_loop_test.go` читает четыре поля.
- **Доки.** `docs/TEMPLATE_REFERENCE.md` + `.ru.md` §4.5.1 (таблица полей записи, пресет), `docs/ARCHITECTURE.md` (поток полей записи), `docs/release_notes/upcoming.md` (EN/RU), `RELEASE_NOTES.md` (черновик).

## Собранный config.json для узлов кейса `tunnel_ingress_mixed`

Штатный шаблон, все пресеты включены, endpoint'ы `wg-home` (без полей), `oc-work` (`allow_local` + `allow_lan`), `ts` (`allow_gateway`), ручной `manual-wg` в «Tunnel endpoints» (проверочная сборка, в репозиторий не вошла):

```json
"route.rules": [
  { "action": "sniff", "network": ["tcp", "udp"], "timeout": "1s" },
  { "action": "hijack-dns", "protocol": "dns" },
  { "action": "resolve", "network": ["tcp", "udp"], "strategy": "ipv4_only" },
  { "inbound": ["oc-work"], "ip_cidr": ["127.0.0.0/8", "::1/128"], "outbound": "direct-out" },
  { "inbound": ["oc-work"], "ip_is_private": true, "outbound": "direct-out" },
  { "inbound": ["ts"], "outbound": "proxy-out" },
  { "action": "reject", "inbound": ["manual-wg", "wg-home", "oc-work", "ts"] },
  { "action": "resolve", "preferred_by": ["ts"], "server": "ts-dns" },
  { "outbound": "ts", "preferred_by": ["ts"] },
  { "ip_is_private": true, "outbound": "direct-out" },
  …
]
"dns.rules": [
  { "preferred_by": ["ts-dns"], "server": "ts-dns" },
  { "action": "reject", "inbound": ["wg-home", "oc-work"] },
  …
]
```

## Golden

`TestWizardTemplateConfigUnchanged` и `TestGoldenScenarios` зелёные **без правки эталонов**: в кэше эталонов штатного шаблона нет endpoint'ов, все пять списков пусты, каждое правило снято своим `#notEmpty` молча (без `template_fragment_dropped`); golden-сценарии несут свои `template.json`.

## Решения по умолчанию

- **Флаги встроены в `state.Node` и `Source10`** (`configtypes.NodeRecordFlags` с json-тегами), а не четырьмя отдельными полями (PLAN п. 6–7): JSON-ключи записи те же, порядок ключей в файле — на месте прежнего `skip_presets`; `canonicalNodeProjection` и экспорт/импорт копируют структуру целиком, новое поле записи добавляется в одном месте. Хелпер слияния — метод `NodeRecordFlags.MergeTrue`, а не `mergeRecordFlags(dst, src *state.Node)`.
- **Подъём легаси-ключа — в `parseV8`, а не в `Load`** (PLAN п. 6): `parseV8` — общая точка `Load`, `Parse` и мигрированного v7 (ключ `skip_presets` появился в контракте 1.1.86, после схемы v8). Файл не перезаписывается на загрузке — новый ключ уйдёт при следующем `Save`. У узлов подписки ключ тоже поднимается (флаг сборка там не читает, но мусорный ключ в файле не остаётся).
- **`registry/presets.json`: `in: "desktop"`, а не `launcher`** (SPEC §3): словарь реестра — `both` / `desktop` / `mobile`; в `note` сказано, что после реализации у LxBox — `both`.
- **В норме новый раздел §6.5.2 с телом пресета** — SPEC §3 называл только §6.6, §6.5.1, §6.7.5; без тела в норме агент LxBox сверялся бы с нашим шаблоном, а владелец просил контракт в приоритете.
- **Корпус:** переменные пресета в форме ядра (`default_value: "proxy-out"` вместо `default`), без оболочки (`id`, `label`, `description`, `default_enabled`, `num`) — формат раздела. У узла `v1` (VLESS) в `tunnel_ingress_mixed` стоит `allow_lan: true`: кейс заодно проверяет, что поле у не-туннеля ничего не открывает.
- **Тест импорта бэкапа — в `core/backup` под тем же именем** `TestLoadLegacySkipPresetsKey` (дописан в `skip_presets_test.go`): `core/state` не может импортировать `core/backup` (цикл).
- **Тултипы** — `ttwidget.NewCheck` + `SetToolTip` (как у галки свёртки в том же окне); над переключателями — заголовок и строка-пояснение «без переключателей соединения отклоняются». Блок у члена папки виден, у узла подписки — нет (записи нет).
- **`normalizeNodeShape` не снимает `allow_*` у `chain`/`auto`** — норма говорит «server only», но поля у других видов сборка не читает (у них нет `body.type` туннеля), а отбрасывание с warning'ом в SPEC не заказано.
- **`PresetNodeRecordFields`** стал картой «имя → функция чтения поля» (был `map[string]bool`): перечень и чтение в одном месте, вне пакета не использовался.

## Известное ограничение (не исправлялось, вне границ SPEC) — закрыто SPEC 171

> Исправлено в **SPEC 171** (контракт 1.1.127, D-135): DNS-слот пресета встаёт по оси маршрутного правила (`state.PresetDNSSlotIndex`), `TASKS_LXBOX.md` §124. Текст ниже — состояние на момент закрытия 170.

**Порядок DNS-правила пресета у существующих пользователей.** `SyncDNSOptionsWithActivePresets` добавляет слот DNS-правила впервые включённого пресета в **хвост** `dns.rules` (так же у `tailscale` с 1.1.86). У пользователя с уже настроенными DNS-правилами DNS-reject `tunnel-ingress` встанет после них: если впереди есть «ловящее всё» правило (`query_type: A/AAAA → fakeip` пресета `fakeip`, выключенного по умолчанию), запрос закрытого пира на такие типы будет обслужен. Маршрутный `reject` от этого не зависит — соединения пира закрыты в любом случае; страдает только DNS-часть. Новая установка получает порядок по оси правил (940 — раньше остальных пресетов). Лечение — вставлять новый пресетный слот по оси правил, а не в хвост; это меняет общую семантику порядка DNS-правил всех пресетов, поэтому вынесено отдельной задачей (упомянуто в §123 для LxBox).

## Проверки

`go build ./...`; `gofmt -l` по изменённым файлам пусто; по имени: `go test ./core/template -run 'TestContractCorpusTemplate'`, `go test ./core/build -run 'TestContractCorpusNodeLoops|TestWizardTemplateConfigUnchanged|TestGoldenScenarios'`, `go test ./core/state -run 'TestLoadLegacySkipPresetsKey'`, `go test ./core/backup -run 'TestLoadLegacySkipPresetsKey'` — зелёные. `go generate ./contract/...`. В новом коде нет builtin `min`/`max`, `slices`, `maps` (Win7). UI тестами не покрыт — показ владельцу. Полный прогон — CI `run_mode=tests`: run [38793036404](https://github.com/Leadaxe/singbox-launcher/actions/runs/38793036404) на коммите `d0e96ebd` — **success** (Test ubuntu / macOS / Windows).

## Дополнение 15.10.2026 — Tailscale выведен из пресета, форма — три настройки (контракт 1.1.129, D-136)

Решение владельца (SPEC §«Изменение 15.10.2026»): `#filter` пресета `tunnel-ingress` — три типа (`wireguard`, `openvpn-client`, `openconnect`), `description` — «(WireGuard, AWG, OpenVPN, OpenConnect)». Пиров tailnet ограничивают ACL Tailscale и фильтр control-plane (наши адреса и явно анонсированное `advertise_routes` / `advertise_exit_node`); reject по умолчанию отрезал бы устройства пользователя от этой машины.

- **Форма пресета (второе решение владельца того же дня).** Пять списков — `wizard_ui: internal` без `unique`, ручного ввода тегов нет; видимы ровно три `outbound`: `closed_out` (`reject`), `allowed_out` (`direct-out`), `gateway_out` (`proxy-out`). Правила loopback/LAN — в `@allowed_out`, остальное — `{"inbound": "@all_nodes", "outbound": "@closed_out"}`; сентинел `reject` превращает `preset_expand.go` (`outboundutil.ApplyOutboundToRule` после подстановки) — в конфиге и в корпусе `"action": "reject"`. DNS-правило под `#and` с `{"@closed_out": {"#in": ["reject", "drop"]}}`. `description` — о трёх настройках. Код форм не менялся: internal-переменные форма скрывает с SPEC 166.
- **Код и шаблон.** `bin/wizard_template.json` (фильтр, описание, переменные, правила); `ui/configurator/tabs/source_tunnel_access.go` — `tunnelIngressTypes` без `tailscale`, подсказка про `advertise_*` и её константа сняты, ключ снят из `bin/locale/ru.json` (`l10n_check --strict` чистый).
- **Контракт.** `VERSION` → 1.1.129, строка changelog в `contract/README.md`; `TEMPLATE_LANG.md` §6.5.2 (заголовок, «Задача», абзац «Почему без Tailscale», тело, итог кейса) и строка D-136 в §9; `BACKUP.md` и `schema/backup.schema.json` — описания `allow_*`; `registry/presets.json` — note; `TASKS_LXBOX.md` §123 по месту (зачем, тумблеры у трёх типов, тело, итог, корпус-критерий) и новый §126; `go generate ./contract/...` (index.md подтянул версию).
- **Корпус.** Тело всех трёх кейсов собрано из шаблона (`default` → `default_value`, без оболочки). `tunnel_ingress_mixed`: `ts` оставлен, добавлен `wg-exit` (wireguard, `allow_gateway`) между `ts` и `v1`, `vars.json` — `{}`; ожидание — шлюз `["wg-exit"]`, reject `["wg-home", "oc-work", "wg-exit"]` (`action: reject`), DNS-reject `["wg-home", "oc-work"]`. Новый `tunnel_ingress_closed_out_forwarded` (`{"closed_out": "proxy-out"}`: последнее правило `outbound: proxy-out`, `dns_rules` пуст). `tunnel_ingress_no_nodes` — новое тело, ожидание прежнее.
- **Доки.** `docs/TEMPLATE_REFERENCE.md` + `.ru.md` §4.5.1, `docs/release_notes/upcoming.md` (EN/RU), `RELEASE_NOTES.md` (черновик).

Решения по умолчанию:

- **`BACKUP.md` и `schema/backup.schema.json` поправлены тоже** (в списке задачи их не было, но описания `allow_*` перечисляли `tailscale` среди типов): формулировка «у `tailscale` поле хранится, но пресетом не читается».
- **`ts` в корпусе — между `oc-work` и `wg-exit`**, `wg-exit` перед `v1`: порядок `all_nodes` = порядок узлов конфига, `ts` в середине доказывает, что фильтр выкидывает узел, а не обрезает хвост.
- **Строка §9 TEMPLATE_LANG (D-136)** добавлена, хотя задача называла только пометку в §6.5.2: таблица решений — единый перечень D-NNN нормы.
- **§6.5.2 «`num: 940` … раньше `tailscale` (945)»** оставлено: `preferred_by` пресета `tailscale` по-прежнему пропустил бы пира WG/OpenVPN/OpenConnect в tailnet.
- **Строки `docs/ARCHITECTURE.md`** не менялись: там типы не перечислены.
- **`unique` у служебных списков снят** (бриф разрешал): ввода нет, `#append` по тегам узлов дублей не даёт.
- **`description`:** «The settings below choose what happens to the rest and where allowed and gateway traffic goes.» — одна фраза о трёх настройках вместо фразы о списках.
- **Строки шаблона не локализуются** (`title`/`tooltip` пресетов в `ru.json` не заводятся — как у остальных пресетов).
- **Сохранённый ввод старых видимых списков** у тех, кто собирал develop, игнорируется (internal, D-132) — миграции нет, пресет в релиз не выходил.
- **Миграции нет:** `allow_*` у Tailscale-узла остаются в записи, ничего не открывают и не закрывают; пресет `tunnel-ingress` в релизе ещё не выходил.

Проверки: `go build ./...`; `go test ./core/template -run TestContractCorpusTemplate`, `go test ./core/build -run 'TestContractCorpusNodeLoops|TestWizardTemplateConfigUnchanged|TestGoldenScenarios|TestLatePresetDNSRuleTakesAxisPosition'` — зелёные, golden без правки (в эталонах нет endpoint-узлов); `TestLatePresetDNSRuleTakesAxisPosition` на боевом шаблоне подтверждает DNS-reject при `closed_out` по умолчанию.

## Дополнение 15.10.2026 — компактное оформление JSON шаблона (решение владельца)

- **`internal/jsonfmt.FormatCompact`**: разбор `json.Decoder` (`UseNumber`, `Token`) в упорядоченное дерево; контейнер печатается в строку `{"a": 1, "b": [1, 2]}`, если она ≤ 100 символов (руны, без отступа и без ключа перед ней), иначе по элементу на строку с отступом 2 пробела, рекурсивно; пустые `{}`/`[]`; строки — `json.Encoder` с `SetEscapeHTML(false)`; один `\n` в конце; лишние данные после значения — ошибка. Идемпотентна.
- **`tools/tplfmt`**: `-w` на месте (файл не трогается, если уже в каноне), `-check` — код 1 с подсказкой команды; без флагов — stdout; `-w` и `-check` вместе — использование, код 2.
- **Вкладка Preset** (`presetSourceJSON`): `FormatCompact`, хвостовой `\n` срезан (как у прежнего `json.Indent`); при ошибке — debug-лог и прежний `json.Indent`.
- **`bin/wizard_template.json`**: 2250 → 1383 строки (до SPEC-правок дня — 2233); упорядоченное дерево совпадает с файлом до форматирования, 17 пресетов, id и порядок прежние, от 427bdd06 отличается только `tunnel-ingress`.
- **Тест** — единственный `TestWizardTemplateFormatted` (`internal/jsonfmt`): шаблон в каноне, идемпотентность, четыре табличных случая (100 символов и порядок ключей, перенос с повторной проверкой детей, литералы `1.0`/`1e3`/`-0.50`/длинное целое, не-ASCII и `<`/`&`), плюс отказ на невалидном JSON.

Решения по умолчанию:

- Длина считается в **рунах** (`utf8.RuneCountInString`): «символы» из брифа, у шаблона есть `①` и эмодзи.
- ` `/` ` и невалидный UTF-8 кодирует стандартный энкодер (экранирование / U+FFFD) — в шаблоне их нет; escape-последовательности исходника (`é`) раскрываются в символ, это та же строка.
- Корпус контракта (`*.preset.json`, отступ 2 через `json.dumps`) в `tplfmt` не переводился — бриф касался файла шаблона; правка кейсов `tunnel_ingress_*` сохранила их прежнее оформление.
- `-check` в CI не подключался: канон стережёт тест, а CI-workflow вне границ брифа.
