# CODEMAP 166 — конвейер обхода узлов в пресете

Карта для волны UI: где живёт каждый шаг `for_each_node` / `for_each_node_do`
(контракт 1.1.119, TEMPLATE_LANG §2.1, §6.5–§6.7). Строки — на коммит движка
SPEC 166.

## 1. Типы и разбор (`core/template`)

| Что | Где |
|---|---|
| Имя узла `node`, ключи, слова движка | `core/template/node_loop.go:36` `NodeLoopVar`, :39 константы `NodeLoopKey`/`NodeLoopDoKey`/`#filter`/`#do`/`#final`/`#append`/`#unique`/`#sort`/`#limit` |
| `wizard_ui: "internal"` | `node_loop.go:58` `WizardUIInternal`; у переменной пресета `PresetVar.WizardUI` `preset_types.go:365`, `IsInternal()` :369 |
| `unique` | `PresetVar.Unique` `preset_types.go:359`; у глобали `TemplateVar.Unique` `vars_resolve.go` (поле + alias, `UnmarshalJSON` :131) |
| `for_each_node` | `node_loop.go:62` `NodeLoop{Filter}`, разбор `UnmarshalJSON` :105 (ошибка формы запоминается, не роняет JSON), `MarshalJSON` :127 |
| `for_each_node_do` | `node_loop.go:72` `NodeLoopDo{Filter, Do []NodeDoOp, Final map[string]NodeFinalOps}`, разбор :133, `#do` — `parseNodeDo` :235, правая часть — `validAppendValue` :264, `#final` — `parseNodeFinal` :281 |
| Поля пресета | `preset_types.go:114` `Preset.ForEachNode`, :120 `ForEachNodeDo`, :130 `ForEachDNSServers` (сырые dns_servers только у `for_each_node`), `legacyForEach` (снятый `for_each`), `UnmarshalJSON` :139 |
| Помощники пресета | `node_loop.go:415` `(*Preset).IsNodeLoop()` — любой из двух ключей; :420 `NodeFilter()` — `#filter` того ключа, что есть |

## 2. Загрузка (отказы — пресет отвергается целиком, `PresetWarning{Action: "skip"}` с id пресета)

| Что | Где |
|---|---|
| Все отказы обхода (§6.5–§6.7) | `node_loop.go:364` `ValidateNodeLoops`: снятый `for_each`; оба ключа; переменная `node`/`node.*`; `unique` не у text_list; ошибки формы (`node_type`, `as`, `filter`, `#do` в `for_each_node`, нет `#do`, ключ без `@`, не один глагол, неизвестный глагол, объект/массив в `#append`, `#final` без пары в `#do`, неверные значения `#unique`/`#sort`/`#limit`); `#do` в переменную не своего пресета / `ref` / не text_list |
| Вызов | `core/template/preset_loader.go:122` `validatePreset` → `ValidateNodeLoops` |
| Тип `text_list` у переменной пресета | `preset_loader.go:375` (раньше пресет с text_list отвергался) |
| Глобали | `core/template/template_validate.go:107` имя `node` зарезервировано, :110 `unique` только у text_list (ошибка загрузки ВСЕГО шаблона, как у прочих ошибок `vars`) |

## 3. Сборка (`core/build/preset_expand.go`)

| Шаг | Где |
|---|---|
| Узлы конфига | `core/build/build.go:572` `CollectPresetNodes` → `ctx.Preset.PresetNodes` (:325) |
| Вход | `ExpandPresetForNodes` :222 — ветки: `ForEachNodeDo` → `collectNodeLoopDo` + тело один раз (scope nil, пространство тегов §6.2 действует); без обхода — как раньше; `ForEachNode` — тело на каждый узел (без пространства) |
| Переменные пресета поверх глобалей | `presetVarsMapWithGlobals` :371 — значение записи у `internal` не читается; `unique` к вводу и default (`uniqueTextList` :442) |
| Отбор узлов | `selectLoopNodes` :354 (`#filter` через `template.EvalNodeFilter` `node_loop.go:517`, scope `template.NewNodeScope(n)` :455) |
| `#do` / `#final` / `unique` | `collectNodeLoopDo` :275 — база = значение переменной (строки через `\n`), `#append` значения узла `template.NodeAppendValue` `node_loop.go:540` (правило вставки `#tpl`: скаляр текстом, пусто/объект/список — узел пропущен молча), `#final` `NodeFinalOps.Apply` :586 (unique→sort→limit), `unique` объявления `template.UniqueStrings` :602; результат — КОПИЯ varsMap |
| Узлы для UI | `PresetForEachNodes` :338 — для обоих ключей; пресет без обхода — nil |
| text_list пресета | `presetSubstitutionScope` :819 — сплит по `\n` (TEMPLATE_LANG §2.2), как у глобали; прежний сплит по запятой снят |
| Правило без условий | общий гейт тела `expandPresetBody` :452: маршрут :553 (`isRuleEmpty && emptyRef`), DNS `expandOnePresetDNSRule` :1146 → `template_fragment_dropped`; тело `for_each_node_do` идёт тем же путём, отдельного кода нет |
| Вызовы | `core/build/resolve_route.go:199`; `core/build/resolve_dns.go:315` (серверы только у `ForEachNode` — сырые, с `#tpl`-тегами), `:705` (`substitutePresetDNSRules`) |

## 4. Состояние и бэкап (`internal`)

| Что | Где |
|---|---|
| Объявление записи | `core/state/record_vars.go:52` `RecordVarDecl.Internal`, причина `RecordVarInternal = "internal"` :80; мост `core/template/record_var_decls.go:87` |
| Нормализация | `NormalizeRecordVarMap` `record_vars.go:422` — имя `internal` снимается: хранение (`ApplyRecordVars`) молча, экспорт (`core/backup/export10.go:158` на копии) не пишет, импорт `core/backup/import.go:507` → `recordVarWarnings` :1052 → `backup_var_skipped {reason: internal}` |
| Глобаль `internal` | `core/template/vars_resolve.go:370` (`ResolveTemplateVarsFor`) и :414 (`VarValuesFor`) — значение состояния не читается, константа `default_value` |

## 5. Корпус и шаблон

| Что | Где |
|---|---|
| Раннер | `core/build/contract_node_loop_test.go:48` `TestContractCorpusNodeLoops` — разделы `contract/corpus/template/for_each_node/`, `for_each_node_do/` |
| Шаблон | `bin/wizard_template.json:1103` пресет `tailscale` на `for_each_node` + `#filter` (тип и `skip_presets`). Пресет `node-domains-fresh-dns` — волна UI |

## 6. Потребители UI (SPEC §5) — правки этой волны только «чтобы собиралось»

| Где | Сейчас | Доделать в волне UI |
|---|---|---|
| `ui/configurator/tabs/source_skip_presets.go:56` | пресет обслуживает узел ⇔ `build.PresetForEachNodes(p, nil, globals, target, [узел без skip_presets])` непуст | ок; при желании — vars пресета из состояния вместо nil |
| `ui/configurator/tabs/preset_ref_edit_dialog.go:486` | «No matching nodes» только у `ForEachNode` | форма: `unique` — дубли при сохранении; `internal` — строки нет; text_list пока рендерится веткой `default` (:237 рядом) — нужен многострочный ввод |
| `ui/configurator/tabs/dns_preset_bundled.go:107` | без выключателя только у `ForEachNode` (у `_do` серверы с пространством пресета, как у обычного) | — |
| `ui/configurator/business/preset_nodes_view.go:42,51` | `IsNodeLoop()`: узлы передаются и для `_do`; подпись строки — узлы, прошедшие `#filter` | — |
| `ui/configurator/presentation/presenter_async.go:82` | `IsNodeLoop()` | — |
| Комментарии `for_each` в `ui/**` | не тронуты | переписать при правке файлов |
