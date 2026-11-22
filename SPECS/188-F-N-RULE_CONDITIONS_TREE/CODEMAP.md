# CODEMAP 188 — карта кода под дерево условий

Снято 23.11.2026 с `develop` (после cf619666). Адреса `файл:строка` — на этот
снимок; после правок первой волны сдвигаются, имена функций — нет. Адреса
`core/state`, `core/build` (карта кэша, выпуск правила), `core/backup` — волны 2–3
(версии) и Debug API обновлены после волны 2 (хранение).

## 1. Реестр: типы и функции

### Состояние (`core/state`)

| Адрес | Что делает |
|---|---|
| `rule_types.go:47` `RuleKind` | `preset` / `inline` / `srs` (у inline/srs вид выводится из дерева) |
| `rule_types.go:79` `Rule` | запись `rules[]`: `Kind, ID, Ref, Name, Enabled, Num, Refs (старая форма, только чтение), Vars, SrsMode, UpdateIntervalHours, DownloadDetour, Body (RawMessage), DNS` |
| `rule_types.go:143` `RuleDNS` | объект `dns` записи (camelCase): `enabled, serverTag, forceIpv4, blockHttps, fixedIps` |
| `rule_types.go:166` `(*RuleDNS).IsZero` | опций нет — объект не пишется |
| `rule_types.go:216` `InlineBody`, `:234` `SrsBody` | виды записи из `DecodeBody`; `SrsBody.Refs` — `RuleRefs` (все ссылки дерева) |
| `rule_types.go:268` `NewInlineRule` | писатель тела inline: match ∪ цель, ключи по алфавиту |
| `rule_types.go:275` `NewTreeRule` (волна 2) | тело-дерево ∪ цель, вид по дереву (`refs` есть — srs) |
| `rule_types.go:302` `NewSrsRule` | писатель srs: `{"refs":[…], цель}` — `refs` первым ключом тела (дедуп) |
| `rule_types.go:321` `(*Rule).SetOutbound` | замена цели в сыром теле с сохранением порядка ключей |
| `rule_types.go:390` `decodeObjectOrdered` | ключи объекта в порядке файла — для правок сырого JSON |
| `rule_types.go:448` `outboundFromBody`, `:469` `isTargetKey` | цель тела: `outbound` / `action: reject` (+`method`) |
| `rule_types.go:494` `(*Rule).DecodeBody` | единственный читатель; srs без ссылок (`RuleRefs`) → ошибка `requires refs` |
| `rule_types.go:554` `(*Rule).BodyMap` | тело картой как есть, **с `refs`** (сборка снимает `StripBodyRefs`) |
| `rule_tree.go:32` (волна 2) | константы ключей формы записи и логики: `refs`, `type`, `mode`, `rules`, `invert`; `RuleKeyRefs` |
| `rule_tree.go:47` `IsLogicalRule`, `:58` `WalkRuleTree` | узел `logical`; обход pre-order с путём индексов `rules[]` |
| `rule_tree.go:113` `BodyRefs`, `:126` `BodyRefsNested`, `:141` `StripBodyRefs` | ссылки дерева (pre-order, дедуп); есть ли ссылки ниже корня; снятие `refs` со всех узлов |
| `rule_tree.go:155` `RuleRefs` | **единственный читатель ссылок правила**: дерево, а без ссылок в дереве — `Refs` записи |
| `rule_tree.go:185` `NormalizeRuleRecord`, `:229` `NormalizeRuleRecords` | подъём `refs` записи в тело (простой корень — первым ключом; `logical` — обёртка `and`), дерево главнее, `kind` из дерева (srs без ссылок и без условий остаётся srs) |
| `rule_tree.go` `IsRuleTargetKey`, `IsRuleKeptKey`, `IsRuleInvertKey`, `RuleNodeInvert`, `RuleLogicalParts`, `NewRuleLogical`, `SetRuleNodeInvert` (волна 4) | служебные ключи для модели формы: цель корня, рукописный `rule_set`, `invert`, разбор и сборка узла `logical` (`ruleTreeFlatRefs` удалён) |
| `rule_identity.go:38` `StableRuleID` | identity: preset → `Ref`, inline/srs → `sanitize(Name)` (+ `-h8` имени при потере символов) |
| `rule_identity.go:64` `LegacyRuleID` | identity до хэша (до 22.11.2026) — для миграции ссылок |
| `rule_identity.go` `MigrateRuleSetTagRefsV9` (волна 3) | `user:<stable|legacy>` (+`:N`, N — позиция в `RuleRefs`) → тег §6.13 той же ссылки (`StateRuleTags`) в телах inline/srs и DNS-правил `user`; неоднозначный старый id и старый тег, равный новому, не трогает |
| `rule_identity.go:146` `renameRuleSetRefs` | замена тегов в `rule_set` карты и в `rules[]` под-правил |
| `rule_identity.go` `legacySrsRuleSetTag` | старая схема `user:<id>`, `user:<id>:N` — только для миграции |
| `rule_tags.go` (волна 3) `AllocateRuleTags`, `RuleTags.Ref/Inline`, `RefHash`, `RuleTagBase`, `RuleNodePath`, `NodeRefs`, `NodeHasSetKeys`, `StateRuleTags` | **одна раздача тегов** §6.13: базы всем inline|srs оси, затем производные всем (вкл. выключенные); тег inline-набора узла без `refs` резервируется; h8 / h16 по всем ссылкам дерева |
| `rule_identity.go:241` `CarryRuleMetadata` | провоз `Rule.ID` по `StableRuleID` при сохранении визарда |
| `rule_set_options.go:63` `SetRuleSetOptions` | у правила без ссылок (`RuleRefs` пуст, не preset) настройки наборов очищаются |
| `record_clone.go:14` `CloneRule` / `:43` `cloneRule` | копия записи (копирует `Refs` старой формы — до нормализации) |
| `rule_order.go:254` `NormalizeRuleOrder` | ось: сортировка по `Num` + засев неотчуждаемых пресетов; порядок для тегов брать отсюда |
| `disk_v8.go:35` `SchemaVersionV8 = 8`, `:42` `SchemaVersionV9 = 9` | мажоры файла (v9 — `refs` в теле); `diskStateV8`, `:66` `parseV8` (v8 и v9) |
| `disk_v8.go:122` | `NormalizeRuleRecords` при чтении (подъём `refs`, вид) |
| `disk_v8.go` (после `NormalizeRuleRecords`) | `MigrateRuleSetTagRefsV9` — рукописные ссылки на старые теги → теги §6.13 |
| `disk_v8.go:132` | `legacyCustomRulesFromV6` — построение UI-вида |
| `load_router.go:32` `Load`, `:132` `parseWithContext` | `meta >= 8` → `parseV8` (`:143`) |
| `schema_gate.go:29` `SchemaMajor = 9`, `CheckSchemaCompatible` | гейт копирования/PATCH состояния машин: мажор выше — отказ |
| `save.go:41`, `:136` | `Save`/`marshalDisk` пишут `SchemaVersionV9` (`meta.schema` прежний `sources_v8`) |
| `state.go:45` `SchemaVersion` | алиас текущего мажора (9) |
| `load_v6.go:214` `legacyCustomRulesFromV6` | запись → `CustomRule` для UI; srs — тело целиком (`refs` внутри, без заглушки `rule_set`, волна 4), `Type: srs`, `RuleSet` — производное по URL (кнопка `upd`); `Params` не восстанавливаются |
| `legacy_types.go:43` `CustomRule.Params` | UI-состояние типа; на диск v8 не попадает (см. §4.5) |
| `migration_v7_to_v8.go:160` `migrateV8Rule` | v7→v8: `refs` на запись (`:194-205`); в тело их поднимает `parseV8` |
| `dns_options.go:70-74` | `DNSRuleKindPreset`, `DNSRuleKindUser` — свои DNS-правила |

### Сборка (`core/build`)

| Адрес | Что делает |
|---|---|
| `resolve_route.go:36` `ResolvedRouteRuleSet` | запись `rule_set`: `Tag, Body, Source, SrsID, Skipped, Enabled` |
| `resolve_route.go:71` `ResolvedRouteRule` | правило маршрута: `Body, Source, InlineID/SrsID, Name, DNS, Num, Enabled` |
| `resolve_route.go:160` `ResolveRouteChecked` | обход оси, `emittedTags` (first-wins), диспетчер по `Kind`; `srsCachedPaths map[ref]path` |
| `resolve_route.go:201` `resolvePresetRouteRule` | наборы пресета (`<preset_id>:<tag>`), висячие → `cleanDanglingRuleSetInRule` (`:287`) |
| `resolve_route.go` `ResolveRouteTagged` (волна 3) | `ResolveRouteChecked` + теги `route.rule_set` шаблона: `ReservedRuleSetTags` → `AllocateRuleTags` до цикла; `dropDanglingHandwrittenRuleSets` после (templateTags == nil — без проверки) |
| `resolve_route.go` `resolveUserRouteRule` (волна 3) | inline и srs одним путём через `emitUserRule`; наборы `Skipped` при невыпуске; `Mirror`, `MirrorDomains`, рукописные теги на `ResolvedRouteRule` |
| `rule_tree_emit.go` (волна 3) `emitUserRule`, `rewriteRuleTree`, `ruleTreeMirror`, `RuleDNSMirrorForBody`, `RuleDNSMirrorEmpty`, `ReservedRuleSetTags`, `TemplateRouteRuleSetTags` | выпуск дерева §6.13 (ссылки, inline-наборы по `set` таблицы, `rule_set` = рукописные + ссылки + inline, всё или ничего, `refs` на `logical` — невыпуск) и зеркало §6.11 по дереву |
| `resolve_route.go:447` `ruleNum` | номер оси, nil → `DefaultRuleNum` |
| `rule_set_policy.go:34` `ResolveRuleSetPolicy`, `:62` `RulePolicy` | local/remote/интервал/detour правила; `file://` → local (`:64` по `RuleRefs`) |
| `rule_set_policy.go:83` `remoteRuleSetBody` | `type: remote` запись набора |
| `rule_set_policy.go:106` `RuleSetFetch`, `:121` `RuleSetFetches` | что качает лаунчер; srs — по `RuleRefs` (`:155-158`); имя файла `srsTagFromURLLocal(url)` |
| `rule_set_policy.go:193` `DropUncachedSrsPaths` | снимает из карты кэша ссылку без файла (правило без хоть одной копии выпадает в сборке) |
| `preset_merge.go:45` `convertPresetRuleSetRemoteToLocal` | набор пресета → local путь |
| `preset_merge.go:101` `cleanDanglingRuleSetInRule` | висячие теги: часть снять, все висячие → правило выпало (SPEC 153) |
| `preset_merge.go:232` `srsTagFromURLLocal` | = `srstag.TagFromURL` |
| `preset_merge.go:235` `PresetMergeContext` | контекст сборки: `Presets, Rules, DNS, SrsCachedPaths (map[ref]path, :242), DataDir, Target, GlobalVars, TemplateVars, PresetNodes, FinalOutboundTags` |
| `preset_merge.go:342` `MergePresetsIntoRoute` | эмиссия `route.rule_set` / `route.rules`; теги шаблона из `routeRaw` (`:366-373`) |
| `preset_merge.go:436` `MergePresetsIntoDNS` | DNS; зеркала `ruleDNSMirrorsForMerge` (`:450`), вставка (`:526`) |
| `preset_merge.go:553` / `:720` `cleanDanglingDNSRule` | висячие `rule_set` в DNS-правилах (в т.ч. `user`) |
| `preset_merge.go:647` `CollectEmittedRouteRuleSetTags` | множество выпущенных тегов (третий вызов `ResolveRouteChecked`, `:688`) |
| `preset_merge.go:810` `CollectSrsCachedPaths` | **карта «ссылка → путь»** по `RuleRefs` всех srs-правил (волна 2); имя файла от URL |
| `preset_expand.go:37` `TagSeparator`, `:222` `ExpandPresetForNodes` | префикс `<preset_id>:` у тегов пресета |
| `preset_expand.go:944-945` | имена списков `route_rule_conditions` / `dns_rule_conditions` |
| `route_merge.go:173` `ResourceNameForSRS` | имя ресурса удалённой машины = `srstag.FileName(tag от URL)` |
| `rule_dns_mirror.go:29-31` | имена списков реестра зеркала |
| `rule_dns_mirror.go` `RuleDNSMirrorMatch`, `RuleDNSBlockHTTPSAvailable`, `RuleDNSMirrorNarrowed` | по дереву тела (через `RuleDNSMirrorForBody`); `ruleDNSMirrorBody` — обёртка `and` над `logical` и над простым `M` с `invert`; `collectRuleDNSMirrors` берёт `r.Mirror` |
| `rule_dns_mirror.go:87` `RuleDNSBlockHTTPSAvailable` | srs или доменный ключ |
| `rule_dns_mirror.go:98` `RuleDNSMirrorNarrowed` | есть ключ из `rule_dns_mirror_unseen` → подсказка |
| `rule_dns_mirror.go:121` `collectRuleDNSMirrors` | зеркала выпущенных правил: Block HTTPS, Fixed (A/AAAA), Force IPv4, server; `invert` **не переносится** |
| `rule_dns_mirror.go:172` `ruleDNSFixedBody` | тело локального ответа: матч + `query_type` + `answer` |
| `rule_dns_mirror.go:198` `RuleDNSMirrors` | то же для показа (Preview окна правила) |
| `rule_dns_mirror.go:223` `ruleDNSMirrorsForMerge` | свой `ResolveRouteChecked` (`:236`) |
| `rule_dns_mirror.go:255` `RuleDNSMirrorIndex`, `:274` `insertRuleDNSMirrors` | место зеркал на оси DNS |

### Имена файлов наборов (`internal/srstag/srstag.go`)

| Адрес | Что делает |
|---|---|
| `:31` `TagFromURL` | `<basename>-<sha256(url)[:4] hex>[.json]` — **имя копии от ссылки, не от правила** |
| `:75` `FileName`, `:83` `TagFromFileName` | `<тег>.srs` / тег с `.json` как есть |
| `:92` `Format`, `:101` `FormatFromRef` | `source` для `.json`, иначе `binary` |
| `:104` `IsFileRef`, `:135` `FileRefFromPath`, `:150` `PathFromFileRef` | `file://` (SPEC 187) |

### Реестр контракта

| Адрес | Что делает |
|---|---|
| `contract/embed.go` | `//go:embed registry/*.json registry/protocols/*.json` — новый файл реестра подхватится без правки embed |
| `core/config/registry/registry.go:782` `Registry`, `:834` `Get`, `:850` `Load` | загрузка; `allowlists.json` читается в `:989`, таблица условий — `:995` (после волны 1) |
| `core/config/registry/registry.go:2089` `Allowlist(name)` | список по имени |
| `core/config/registry/rule_conditions.go` (волна 1; `Label`/`Placeholder` ключа и проверка подписей формы лаунчера — волна 4, контракт 1.1.181) | `RuleCondition`, `RuleConditionKey`, `loadRuleConditions` (сверка с `route_rule_conditions` / `rule_dns_mirror_match` / `rule_dns_block_https_domains`), `RuleConditions()`, `RuleConditionOfKey()`, `RuleSetKeys()`, `RuleDNSKeys()`, `RuleDomainKeys()`, `RuleCondition.ShownIn(form)` |
| `contract/registry/rule_conditions.json` (волна 1) | таблица условий |
| `contract/registry/allowlists.json:199` | `rule_dns_mirror_match`, далее `rule_dns_mirror_unseen`, `rule_dns_block_https_domains`; рядом `route_rule_conditions`, `dns_rule_conditions` |
| `contract/doc.go:11` | `//go:generate go run ./tools/gendocs` — генерирует только протоколы и предупреждения (`contract/tools/gendocs/raw.go:168`, `warnings.go`); таблица условий в generated сама не попадёт |

### Бэкап (`core/backup`)

| Адрес | Что делает |
|---|---|
| `types.go:37` `FormatVersion = 1` | 0.x |
| `backup10.go:38` `FormatVersion10 = 2`, `:49` `FormatVersion11 = 3`, `Backup10` | формат 1.0 / 1.1 (`refs` в теле); `Rules []state.Rule` как в состоянии |
| `file.go:154` `Parse` | диспетчер по `lx_backup`: 1, 2 и 3 (`:175`, один разбор `decodeTolerant10`), **иначе отказ** `backup format v%d is newer than supported v3 — update the app` |
| `file_keys_10.go:81` `rule10Keys` | известные ключи записи — из тегов `state.Rule` (поле `Refs` должно остаться) |
| `export10.go:41` `Export10` (пишет `LxBackup: FormatVersion11`, `:53`), `:230` `export10Rules` | экспорт копией + `NormalizeRuleRecord` (`:237`) |
| `import10.go:33` `decode10`, `:316` `decode10Rule` | ввоз записи правила копией + `NormalizeRuleRecord` (`:323`); 0.x `kind: json` идёт сюда же |
| `import.go:343` `Import10` | отказ на `lx_backup` > 3 (`:350`) |
| `import10.go:335` `checkImportedRuleTargets`, `:370` `ruleTarget10` | цель через `DecodeBody` |
| `import10.go:431` `splitRuleBodies`, `:479` `ruleBodyPresent` | норма «одно правило — одно тело» |
| `legacy_read_0x.go:408` `importRule` | 0.x → `NewInlineRule` / `NewSrsRule` (`:443`, `:451`) |
| `import.go:314` `Import`; `:416-425`, `:516-518` | `rules[]` — полная замена |
| `corpus_test.go:373` `TestBackupCorpus` | раннер `contract/corpus/backup`; файл с `lx_backup` выше 3 — пропуск кейса; `refs` правила — `state.RuleRefs` |
| `schema_test.go:32` `schemaPath10` | писатель против `backup.schema.json` |

### Правка состояния: Debug API

| Адрес | Что делает |
|---|---|
| `core/debugapi/state_endpoints.go:184` `stateRulesWith` | GET/PATCH `/state/rules`; `NormalizeRuleRecord` + проверка `DecodeBody` (`:213`), replace/append |
| `core/debugapi/backup_endpoints.go:150` | `reads`: 1, 2, 3 |
| `core/debugapi/state_crud_endpoints.go:10-11` | POST/DELETE `/state/rules` → `stateedit` |
| `core/debugapi/server.go:275`, `remote_endpoints.go:112` | регистрация локальной и машинной точек |
| `core/stateedit/rules.go:64` `AddRule` | `NormalizeRuleRecord` (`:67`), проверка тела и цели, номер оси |
| `core/stateedit/servers.go:654` `nameRefs` | кто ссылается на тег: цель корня тела (`outbound`), ссылок наборов не читает |

### Загрузка наборов и сборщик мусора

| Адрес | Что делает |
|---|---|
| `core/srs_refresh.go:104` | цикл обновления берёт `ctx.Preset.RuleSetFetches()` |
| `core/config_service.go:404` `collectAllStageRuleSetTags` | keep-лист файлов для GC; srs по `RuleRefs` (`:474-485`) |
| `core/config_service_context.go:85-92` | контекст сборки сервиса: `CollectSrsCachedPaths` + `DropUncachedSrsPaths` |
| `core/services/srs_downloader.go:341` `GetSRSEntries` | записи для кнопки `upd` из сырых `rule_set` |

### UI: модель (`ui/configurator/models`)

| Адрес | Что делает |
|---|---|
| `rule_state.go:31` `RuleState` | `Rule (TemplateSelectableRule), Enabled, SelectedOutbound, Num, DNS, SrsOptions` |
| `core/template/loader.go:179` `TemplateSelectableRule` | `Label, Rule (map), Rules, RuleSets ([]RawMessage), DefaultOutbound, HasOutbound, Params (:208)` |
| `wizard_state_file.go:119` `PersistedCustomRuleToRuleState`, `:152` `ToPersistedCustomRule` | legacy-вид ↔ модель |
| `wizard_state_file.go:190` `DetermineRuleType` | только legacy-тип вида CustomRule (state v6): `refs` в дереве → srs (волна 4) / ip / домены / процессы / `rule_set` / иначе raw; форма его не зовёт |
| `rule_conditions.go` (волна 4, без Fyne) `RuleCondTable` (`NewRuleCondTable`, `LauncherRuleCondTable`, `Cond`), `RuleCondStack`/`RuleCond`/`RuleGroup`, `StackFromBody`, `StackBody`, `ForForm`, `RuleCondBodyJSON`, `RuleCondValueEmpty`, `RuleSetsFromBody`, `RuleStateRuleSets` | модель формы: тело ⇄ стопка условий и групп (SPEC §3), снимок «не меняли»; наборы правила — производное от `refs`. Тест `TestRuleCondStackRoundTrip` — круг по `corpus/rule_tree` |
| `preset_ref_sync.go:31` `EmitStateRulesWithoutOrder`, `:65` `EmitStateRulesInAxisOrder` | модель → `[]state.Rule` |
| `preset_ref_sync.go:128` `RuleOrderFromAxis`, `:189` `buildCustomRuleQueue` | сопоставление слотов по `StableRuleID` |
| `preset_ref_sync.go:252` `customRuleStateToV6Rule` | **модель → запись**: `refs` в теле → `NewTreeRule` (вид по дереву, настройки наборов); legacy-вид без `refs` с remote `RuleSets` → `NewSrsRule`; `Params` не переносятся |

### UI: окно правила (`ui/configurator/dialogs`, волна 4)

| Файл | Что там |
|---|---|
| `add_rule_dialog.go` | `ShowAddRuleDialog`: имя, Direction, настройки наборов (видны при `refs` в теле), вкладки Form / DNS / JSON / Preview, снимок формы (неизменённое правило — исходным телом), `saveRule`; JSON ↔ форма через `RuleCondTable`; цель только у корня (`ruleHasNestedTarget`); `ruleArrayPastedError`, `conflictingRuleName` |
| `rule_cond_stack.go` | `condStackEditor`: рамки условий и групп, «AND», «+ Condition» (PopUpMenu: условия таблицы, которых нет в стопке; за чертой «Rule group») |
| `rule_cond_frame.go` | `ruleFormCtx`, `condFrame`: поля по виду `field` таблицы (`lines`, `cidr_lines`, `ports`, `bool`, `enum_multi` чипами, `processes`, `refs`), «any of», `one_of` переключателем, `conflicts` (поле недоступно с подсказкой), пометка `platforms` по ОС цели модели; `ruleCondRegistryTexts` — подписи реестра для l10n_check |
| `rule_cond_group.go` | `groupEditor`: «all of / any of», «Not», правила-стопки, «+ Rule», рекурсия |
| `rule_cond_refs.go` | поле наборов (URL или файл, «Add file…», «?», подсказка SPEC §6), `normalizeRuleSetRef`, `ruleSetFileError`, `ruleRefsHaveFile` |
| `rule_dialog_dns.go` | вкладка DNS: опции доступны всегда, ⚠ пустого зеркала (`RuleDNSMirrorEmpty`), `RuleDNSNoEffectMark` (строка правила на вкладках Rules и DNS) |
| `rule_dialog_preview.go` | `rulePreviewText`, `rulePreviewAxis`, `rulePreviewOwn` — ось модели целиком, теги шаблона |
| `rule_process_picker.go` | `processListField`, окно выбора процессов, `normalizeProcName`/`sortProcessStrings`/`dedupeProcessStrings` |
| `rule_dialog.go` | `ParseLines` |

Удалены: `rule_type_selection.go`, `rule_dialog_test.go`, `ExtractStringArray`, `SimplePatternToRegex`, `ProcessKey`/`ProcessPathRegexKey`, подписи типов, `srsTagFromURL`, `srsRuleSetsForRawBody`, `buildRuleRaw`, `Params` окна.
`rule_set_options.go:22` `RuleSetOptionsForm`, `:141` `SetModeLocked`; `srs_tag.go:10` `SRSTagFromURL`.

### UI: вкладки

| Адрес | Что делает |
|---|---|
| `tabs/rules_tab.go:95` `customRuleSRSEntries` | наборы правила — `RuleStateRuleSets` (`refs` в дереве, legacy `RuleSets`) |
| `tabs/rules_tab.go:396` `buildSingleCustomRuleRow` | **строка правила**: метка DNS `ruleDNSMirrorRowText` — опции или ⚠ пустого зеркала (тултип — текст ⚠) |
| `tabs/rules_tab.go:611` `createCustomRuleSRSButton` | кнопка `upd` |
| `tabs/rules_unified_rows.go:40` `buildUnifiedRuleRows` | диспетчер строк; пресеты `:73`; строка своего правила — в `rules_tab.go` |
| `tabs/dns_unified_rules.go:80` `ruleDNSMirrorRowsByPos` | строки зеркал на вкладке DNS |
| `tabs/dns_unified_rules.go:119` `ruleDNSMirrorText` | текст метки; пустое зеркало → `""`; `ruleDNSMirrorRowText` — то же или ⚠, `ruleDNSMirrorIsEmpty` |
| `tabs/dns_unified_rules.go:145` `ruleStateIsSRS` | `RuleStateRuleSets` не пуст |
| `tabs/dns_unified_rules.go:169` `buildRuleDNSMirrorRow` | строка «📎 имя · опции» или «📎 имя · ⚠ …» |
| `tabs/dns_user_rules.go:525` `collectAllRuleSetTags` | теги для формы своих DNS-правил: только пресеты; `user:` не предлагаются (`:567-574`) |
| `business/create_config.go:195` | `PresetMergeContext` визарда; `:303` `routeConfigFromModel` (legacy `RouteRule.RuleSets`) |
| `presentation/presenter_save.go:550` `modelHasRuleSetFiles` | нужен ли ресурс-стор машине — по `refs` в теле (`:563`) и `Rule.RuleSets` |

## 2. Кто кого зовёт

### Состояние → наборы → маршрут → DNS-зеркало

```
state.Load (load_router.go:32)
 └ parseWithContext → parseV8 (disk_v8.go:66)
    ├ NormalizeRuleRecords (rule_tree.go:229)          — refs записи → тело, kind
    ├ MigrateRuleSetTagRefsV9 (rule_identity.go) → StateRuleTags (rule_tags.go)
    └ legacyCustomRulesFromV6 (load_v6.go:214)            → UI-вид

ConfigService / визард
 ├ PresetMergeContext (config_service_context.go:85 | create_config.go:195)
 │   └ CollectSrsCachedPaths (preset_merge.go:811) → DropUncachedSrsPaths (rule_set_policy.go:193)
 ├ MergePresetsIntoRoute (preset_merge.go:342)
 │   └ ResolveRouteChecked (resolve_route.go:159)
 │       ├ NormalizeRuleOrder (rule_order.go:254)
 │       ├ resolvePresetRouteRule (:201) → ExpandPresetForNodes → cleanDanglingRuleSetInRule
 │       ├ ReservedRuleSetTags → AllocateRuleTags  — теги §6.13 один раз на ось
 │       ├ resolveUserRouteRule → emitUserRule (rule_tree_emit.go) — наборы, тело, Mirror
 │       └ dropDanglingHandwrittenRuleSets — висячие рукописные rule_set
 ├ CollectEmittedRouteRuleSetTags (preset_merge.go:647) → ResolveRouteChecked
 └ MergePresetsIntoDNS (preset_merge.go:436)
     ├ ruleDNSMirrorsForMerge (rule_dns_mirror.go:223) → ResolveRouteChecked → collectRuleDNSMirrors (:121)
     │   └ r.Mirror (дерево зеркала из выпуска)
     ├ cleanDanglingDNSRule (preset_merge.go:720) — свои DNS-правила
     └ insertRuleDNSMirrors (rule_dns_mirror.go:274)

Цикл обновления: srs_refresh.go:104 → RuleSetFetches (rule_set_policy.go:121) → srs_downloader
GC файлов: collectAllStageRuleSetTags (config_service.go:404)
```

Четыре вызова `ResolveRouteChecked` (`preset_merge.go:360`, `:688`,
`rule_dns_mirror.go:236`, `add_rule_dialog.go:1659`) обязаны раздавать
одинаковые теги — новая схема тегов зависит от всех правил оси и тегов
шаблона, а не от одного правила.

### Форма → состояние

```
ShowAddRuleDialog (add_rule_dialog.go)
 ├ открытие: RuleState.Rule.Rule → RuleCondTable.StackFromBody → ForForm → condStackEditor | вкладка JSON
 ├ saveRule: стопка (снимок не изменён — исходное тело) | JSON → RuleState{Rule.Rule (refs внутри), Rule.RuleSets (производное), DNS, SrsOptions}
 └ Preview: rulePreviewText → rulePreviewAxis (ось модели) → ResolveRouteTagged → rulePreviewOwn → RuleDNSMirrors

Сохранение визарда: EmitStateRulesInAxisOrder (preset_ref_sync.go:65)
 └ customRuleStateToV6Rule (:252) → NewTreeRule (legacy: NewSrsRule) → state.Rules → Save
```

### Бэкап

```
экспорт: Export10 (export10.go:41) → export10Rules (:228) — копии записей
импорт:  Parse (file.go:154) → decode10 (import10.go:33) → splitRuleBodies → decode10Rule (:316, NormalizeRuleRecord)
         → Import (import.go:314): rules[] замена → checkImportedRuleTargets (import10.go:335)
0.x:     importRule (legacy_read_0x.go:408) → NewInlineRule / NewSrsRule
Debug API: stateRulesWith (state_endpoints.go:184) / stateedit.AddRule (rules.go:64)
```

## 3. Корпус и раннеры

| Каталог | Раннер | Что сверяет |
|---|---|---|
| `contract/corpus/rule_dns/` (11 кейсов) | `core/build/contract_rule_dns_test.go:40` `TestContractCorpusRuleDNS` | запись → `NormalizeRuleRecord` → `ResolveRouteWithGlobals` → `RuleDNSMirrors`; **матчер снимается** (ключи `rule_dns_mirror_match`), сверяются действия. Кэш srs — по ссылке `srsCached[ref]` |
| `contract/corpus/backup/` | `core/backup/corpus_test.go:373` `TestBackupCorpus` | файл → `Parse` → `Import` → ожидания (`refs` правила — `corpusRuleExpectation.Refs` против `state.RuleRefs`) |
| `contract/corpus/template/` | `core/build/contract_*_test.go` | язык шаблона |

Регенерация зеркала: `go test ./core/build -run TestContractCorpusRuleDNS -update-rule-dns`.

## 4. Выжимки по подсистемам

### 4.1 Теги наборов (после волны 3)

Схема §6.13: база — имя правила (края обрезаны, пустое — `unnamed`), развод
`B (2)`…; первая ссылка корня — `B'`, прочие — `B':<h8>`, inline-набор —
`B'` / `B':inline` / `B':inline:<путь>`. Считает `core/state/rule_tags.go`
(`AllocateRuleTags`) — сборка по оси `NormalizeRuleOrder` с занятыми тегами
шаблона и пресетов (`ReservedRuleSetTags`), загрузка состояния — `StateRuleTags`
(ось `SortRulesByNum`, занятые — префиксы `<preset_id>:`). Старые теги
`user:<id>`, `user:<id>:N` в телах правил и своих DNS-правил переписывает
`MigrateRuleSetTagRefsV9` при каждом чтении (`disk_v8.go`). Теги пресетов —
`<preset_id>:<tag>` (`preset_expand.go:37`).

### 4.2 Файлы копий

Имя файла — `srstag.TagFromURL(ref)` = `<basename>-<h8(url)>[.json]` + `.srs`,
от тега правила **не зависит** (issue #77, `preset_merge.go:786-842`). Смена
тегов правил перекачки не вызывает. Привязка «правило → файлы» после волны 2
— по ссылке: `srsCachedPaths[ref]`, позиции нет.

### 4.3 Рукописный `rule_set`

- inline: тело едет как есть, висячий тег не снимается — ядро падает на
  `rule-set not found`, если набор не выпущен (правило-владелец выключено,
  файл не скачан).
- srs: сборка затирает `rule_set` тела своим списком (`resolve_route.go:429`).
- свои DNS-правила: висячие теги снимает `cleanDanglingDNSRule`.

### 4.4 DNS-зеркало

Плоское: ключи тела из `rule_dns_mirror_match` (сейчас 18 ключей, среди них
`network_type`, `source_ip_is_private`; нет `network_is_expensive`).
`invert` в список не входит и не переносится. Тела Block HTTPS / Force IPv4 /
Fixed = копия матчера + `query_type` / `ip_version` / `answer`. Ядро
(`sing-box-lx/option/rule_dns.go:237` `RawLogicalDNSRule`) у `logical`
принимает только `mode`, `rules`, `invert` и действие — `query_type` и
`ip_version` у корня-`logical` недопустимы.

### 4.5 `Params`

`TemplateSelectableRule.Params` (`loader.go:208`) — UI-состояние типа
(`domain_mode`, `match_by_path`, `path_mode`). С волны 4 окно его не пишет
(обнуляет при сохранении); раньше `customRuleStateToV6Rule` его не переносит, а `legacyCustomRulesFromV6` не
восстанавливает: до диска v8 не доживает, живёт до закрытия Конфигуратора.

### 4.6 Версии форматов

- `state.json`: `meta.version` = 9 (после волны 2; 8 — `refs` на записи);
  читатель берёт `>= 8` одним разбором; `CheckSchemaCompatible` отказывает на
  мажоре выше 9 — только при переносе и PATCH состояния машин.
- бэкап: `lx_backup` (1 — 0.x, 2 — 1.0, 3 — 1.1, `refs` в теле); экспорт пишет
  3; значение выше 3 — отказ всего файла, BACKUP.md §8. Схема
  `backup.schema.json` — `lx_backup: enum [2, 3]` (волна 1).

### 4.7 Контракт: нумерация

`contract/VERSION` = 1.1.178; последний параграф `TASKS_LXBOX.md` — §181;
последнее решение `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md` — D-155;
changelog — таблица в `contract/README.md` (строка 1.1.178 — `:410`);
TEMPLATE_LANG §6.11 — `contract/docs/TEMPLATE_LANG.md:1610`; ONE_NAMESPACE
«Правило маршрута» — `contract/docs/ONE_NAMESPACE.md:94`; BACKUP `rules[]` —
`contract/docs/BACKUP.md:430`, §8 — `:740`.
