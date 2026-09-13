# CODEMAP — 120-F-N-NODE_SECTIONS («секции узла»)

Карта фактов по коду на ветке `develop` (снимок; коммит `b120c15`). Только
существующее положение дел — ни предложений, ни проектных решений.
Все якоря проверены `grep`/`sed` на момент снятия карты.

Легенда: `файл:строка` относительно корня репозитория.

---

## 1. Модель узла и JSON-источника

### 1.1 Два разных «узла»: канон на диске и сборочная форма

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `state.Node` — **канон v7, то, что лежит в state.json** | `core/state/sources_v7.go:116-166` | Юнион по `Kind`; поля ниже |
| `Node.Kind` | `core/state/sources_v7.go:117` | `server` / `chain` / `auto` / `folder` / `subscription` / `unsupported` |
| `Node.Tag` | `:121` | СЫРОЙ тег = идентичность в контейнере (SPEC 112) |
| `Node.Enabled` | `:122` | Выключение узла — здесь |
| `Node.Origin` | `:123` | Происхождение (URI / INI / JSON) |
| `Node.Body json.RawMessage` | `:135` | Готовое тело sing-box **без** `tag`/`detour` |
| `Node.Detour *NodeLink` | `:139` | server only; у `folder` — общий detour папки |
| `Node.Hops []NodeLink` | `:141` | chain only |
| `Node.Group *AutoGroup` | `:143` | auto only |
| `Node.Service bool` | `:160` | Служебный узел (релей BYPASS) |
| `Node.Reason string` | `:165` | unsupported only — текст причины (английский) |
| `state.Source` (встраивает `Node`) | `core/state/sources_v7.go:204-246+` | `ID` ULID `:215`, `Name` `:216`, `TagPolicy` `:217`, `Nodes []Node` `:218`, `Replace` `:219`, `URL` `:223`, `UserAgent` `:233`, `HWID` `:237` |
| `FolderReplace` | `core/state/sources_v7.go:187-194` | свёртка папки |
| `SchemaVersionV7 = 7` | `core/state/disk_v7.go:29` | версия формата state.json |
| `SchemaMajor` | `core/state/schema_gate.go:28` | гейт схемы |
| Ключевые схемы файла | `core/state/disk_v7.go:6` | `meta.version=7`, `meta.schema="sources_v7"` |

**Ключевой факт:** новых секций (dns/route) у `state.Node` **нет** —
единственное «тело» узла это `Body`, и оно по контракту есть `sing-box
outbound`, а не фрагмент конфига.

### 1.2 Сборочная форма (build-only проекция)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `configtypes.ProxySource` (тело-вход) | `core/config/configtypes/types.go:~100-183` | `ID` `:103`, `Label` `:113`, `ConfigJSON json.RawMessage` `:144`, `Canonical *CanonicalSource` `:157`, `Chain` `:169`, `LocalGroups` `:182` |
| `ConfigJSON` — ВХОД ПАРСЕРА, не поле состояния | `core/config/configtypes/types.go:139-144` | комментарий SPEC 118 W5 |
| `CanonicalSource` | `core/config/configtypes/types.go:186-219` | `FolderID`, `IsContainer`, `TagPrefix/TagPostfix`, `Nodes []CanonicalNode`, `FolderDetour`, `Replace`, `RelaysInDirections` |
| `CanonicalNode` | `core/config/configtypes/types.go:222-247` | `Kind`, `Tag`, `Enabled`, `Body`, `Detour`, `Hops`, `Group`, `Service` |
| `NodeLink` | `core/config/configtypes/types.go:257-262` | `{FolderID, Tag}`; `FolderID==""` → корневое пространство |
| `ParsedNode` — рабочая форма эмиссии | `core/config/configtypes/types.go:645-736` | см. ниже |
| `ParsedNode.Tag/Scheme/Server/Port/Outbound` | `:646-654` | `Outbound map[string]interface{}` `:654` |
| `ParsedNode.EmitRaw` | `:696` | тело эмитится как есть (ручной config_json) |
| `ParsedNode.EmitBody json.RawMessage` | `:709` | тело канона v7 — приоритетнее `EmitRaw` и per-scheme ветки |
| `ParsedNode.CanonicalDetour` | `:715` | |
| `ParsedNode.Service` | `:684` | |
| `ParsedNode.Warnings []string` + `AddWarning` | `:735`, `:739` | коды из `contract/registry/warnings.json` |
| Алиас `config.ParsedNode` | `core/config/models.go:12` | |

### 1.3 Как тело источника становится узлами

| Шаг | Файл:строка | Заметка |
|---|---|---|
| Классификация тела | `core/config/subscription/body_classify.go:90` | `ClassifySubscriptionBody(body) BodyKind` |
| `BodyKind` перечисление | `body_classify.go:14-42` | `BodyKindSingboxConfig` `:30`, `BodyKindSingboxOutbound` `:22`, `…OutboundArray` `:24`, `…ConfigArray` `:32` |
| `BodyKind.IsSingbox()` | `body_classify.go:66` | четыре sing-box формы |
| `classifyJSONObjectBody` | `body_classify.go:181` | `"type"` проверяется РАНЬШЕ `"outbounds"` (SPEC 094 A1) |
| Вход импортёра | `core/config/subscription/singbox_import.go:66` | `ParseSingboxBody(body, kind, skip)` |
| Нормализация четырёх форм к «массиву конфигов» | `singbox_import.go:73-122` | `normalizeSingboxBodyToConfigs` |
| Ядро импорта | `singbox_import.go:125` | `ParseNodesFromSingboxConfigs(configs, skip)` |

### 1.4 UI создания/правки узла

| Сущность | Файл:строка | Заметка |
|---|---|---|
| Окно источника (2555 строк) | `ui/configurator/tabs/source_edit_window.go` | вкладки Overview / JSON / Group / Chain / … |
| Буфер-копия узла (SPEC 117) | `source_edit_window.go:188-197` | `Body` копируется, форма владеет копией |
| Вкладка **JSON** — комментарий-закон | `source_edit_window.go:1840-1849` | server = редактируемый outbound; subscription = read-only |
| `jsonEntry` (MultiLineEntry) | `source_edit_window.go:1852-1853` | |
| read-only откат для подписки | `source_edit_window.go:1861-1867` | `OnChanged` возвращает `lastSetJSON` |
| Кнопка **Apply JSON** | `source_edit_window.go:1885` | |
| Проверка «объект с непустым `type`» | `source_edit_window.go:1893-1904` | **до** ветвления; отвергает всё, у чего нет `type` |
| Ветка цепочки (свой набор ключей, чужие молча не принимаются) | `source_edit_window.go:1906-1955` | `Outbounds/IdleTimeout/StripEvasion/Strip/Rewrite` |
| Ветка сервера — прямой редактор тела | `source_edit_window.go:1957-…` | `applyServerBodyJSON`, откат при ошибке |
| Отрисовка тела из `scratch.Body` | `source_edit_window.go:2056-2060` | `json.Indent` |
| Грязный-флаг вкладки JSON | `source_edit_window.go:2126` | `jsonEntry.Text != lastSetJSON` |
| Распаковка узлов источника в документ | `ui/configurator/tabs/source_edit_json.go:108` | `unpackNodesDoc(nodes, limit)` |
| Документ распаковки | `source_edit_json.go:111-114` | `{"outbounds":[…],"endpoints":[…]}` — **ровно две секции** |
| Точка эмиссии — та же, что у сборки | `source_edit_json.go:129` | `config.EmitNodeJSONs(node)` |
| `renderUnpackedNodes` (лимит показа) | `source_edit_json.go:155` | |
| `stripEmittedDecorations` / `withPendingDetour` / `emittedToEditableJSON` | `source_edit_json.go:21` / `:45` / `:74` | |

---

## 2. Импорт endpoints из sing-box JSON

### 2.1 Что импорт читает и что выбрасывает

| Сущность | Файл:строка | Заметка |
|---|---|---|
| **`singboxIgnoredSections`** | `core/config/subscription/singbox_import.go:27` | `[]string{"route", "dns", "inbounds", "experimental"}` — **именно здесь секции конфига теряются** |
| `SingboxImportResult` | `singbox_import.go:30-48` | `Nodes` `:33`, `IgnoredSections` `:36`, `UnsupportedTypes` `:38`, `Warnings` `:41`, `rejected` `:47` |
| Отметка присутствующей игнорируемой секции | `singbox_import.go:134-138` | `if _, present := cfg[section]; present { ignored[section] = … }` |
| Заполнение результата | `singbox_import.go:143-144` | `orderedSubset` + `sortedKeys` |
| Лог игнора (только DebugLog) | `singbox_import.go:146-149` | пользователь узнаёт из превью, не из UI-предупреждения |
| Лог неподдержанных типов (WarnLog) | `singbox_import.go:150-153` | |
| `parseSingboxConfig` | `singbox_import.go:157` | разбор одного конфига |
| Запись без `type` → rejected | `singbox_import.go:189-194` | |
| Служебные типы пропускаются | `singbox_import.go:195-197` | `IsSingboxServiceType` (A3) |
| Групповые типы — после узлов | `singbox_import.go:198-201` | `IsSingboxGroupType` (A5) |
| **`singboxAllEntries` — outbounds ++ endpoints одним списком** | `singbox_import.go:260-274` | цикл по `[]string{"outbounds","endpoints"}` `:261`. **Предиката «это endpoint» НЕТ** — различие теряется прямо здесь |
| `parseSingboxEntry` | `singbox_import.go:281` | |
| **Отказ по неизвестному типу** | `singbox_import.go:288-291` | `scheme, ok := singboxTypeToScheme(entryType); if !ok → "unsupported outbound type %q"` |
| **`singboxSchemeByType` — таблица типов** | `singbox_import.go:344-359` | 14 типов: vless, vmess, trojan, shadowsocks, hysteria, hysteria2, tuic, anytls, ssh, socks, http, naive, wireguard, masque. **`tailscale` отсутствует** |
| `SchemeFromSingboxType` (та же таблица наружу) | `singbox_import.go:366` | SPEC 118 W4 — второй таблицы заводить нельзя |
| **`singboxTypeIsAddressless`** | `singbox_import.go:371-373` | `return t == "wireguard"` — единственный тип без `server`/`server_port`; всё остальное обязано иметь адрес (`:299-307`) |
| Материализация отказа узлом | `singbox_import.go:333-341` | `result.rejected.add(...)` → `kind=unsupported` (SPEC 116 W11) |
| `NewUnsupportedNode` | `core/state/sources_v7.go:176` | как отказ становится узлом на своей позиции |

**Итог по `tailscale`:** запись типа `tailscale` не проходит `singboxTypeToScheme`
(`singbox_import.go:288`), попадает в `UnsupportedTypes`, лог WarnLog
(`:150-153`), и материализуется узлом `kind=unsupported` с `Reason`
(`:333-341`). Плюс — даже если бы схема была, её завалил бы обязательный
`server`/`server_port` (`:299-307`), т.к. `singboxTypeIsAddressless`
возвращает true только для `wireguard`.

### 2.2 Как endpoint попадает в `endpoints[]` при сборке

| Сущность | Файл:строка | Заметка |
|---|---|---|
| **Разделитель outbound/endpoint** | `core/config/outbound_generator.go:1081` | `EmitNodeJSONs(node) (outboundJSONs []string, endpointJSON string, err error)` |
| **Жёсткий гейт** | `core/config/outbound_generator.go:1086` | `if node.Scheme == "wireguard" { … GenerateEndpointJSON … }` — ЕДИНСТВЕННЫЙ признак endpoint'а |
| `GenerateEndpointJSON` | `outbound_generator.go:1040` | обёртка с комментарием |
| **`GenerateEndpointJSONBare`** | `outbound_generator.go:1054-1056` | `if node == nil \|\| node.Scheme != "wireguard" \|\| node.Outbound == nil { return "", err }` — второй такой же гейт |
| Разложение результата по спискам | `outbound_generator.go:1405-1417` | `if epJSON != "" { endpointsJSON = append(…) } else { selectorsJSON = append(…) }` |
| `OutboundGenerationResult.EndpointsJSON` | `outbound_generator.go:55` | комментарий: «WireGuard nodes go to EndpointsJSON» |
| `EndpointsCount` | `outbound_generator.go:57` | |
| Возврат | `outbound_generator.go:1430-1432` | |
| `masque` эмитится в **outbounds**, не endpoints | `outbound_generator.go:528` | ветка per-scheme внутри `GenerateNodeJSON` |
| Перенос в кэш сборки | `core/rebuild_snapshot.go:110` | `Endpoints: jsonStringsToRawMessages(result.EndpointsJSON)` |
| Пустой результат = ошибка | `core/config_service.go:285` | `len(OutboundsJSON)==0 && len(EndpointsJSON)==0` |
| `ParsedCache.Endpoints []json.RawMessage` | `core/build/parsed_cache.go:24` | комментарий: «WireGuard endpoints (если используются)» |
| Секция в сборке | `core/build/build.go:304-306` | `case "endpoints": genEP := cacheEndpointsAsStrings(ctx.Cache); return BuildEndpointsSection(...)` |
| `cacheEndpointsAsStrings` | `core/build/build.go:395-400` | pretty-printed (в отличие от outbounds — compact, `:385`) |
| `BuildEndpointsSection` | `core/build/sections.go:111` | маркеры `/** @ParserSTART_E */` / `@ParserEND_E`, `staticEndpoints` из шаблона `:117-118`, preview-обрезка `:129-131` |
| `BuildOutboundsSection` | `core/build/sections.go:52` | пара |

### 2.3 Endpoint-узлы в селекторах Направлений

| Сущность | Файл:строка | Заметка |
|---|---|---|
| Пул кандидатов Направлений | `core/config/outbound_filter.go:41` | `FilterDirectionCandidatePool(allNodes, proxies)` |
| Что исключается | `outbound_filter.go:52-60` | свёрнутая папка (`Replace != nil`) и `n.Service` без `RelaysInDirections` |
| **Endpoint'ы НЕ исключаются** | `outbound_filter.go:41-64` | схема узла в фильтре не участвует вовсе — wireguard-узел рядовой кандидат |
| Фильтр по полям Направления | `outbound_filter.go:66` | `filterNodesForSelector(allNodes, filter)` |
| Сбор состава селектора | `core/config/outbound_generator.go:1421-1426` | `buildOutboundsInfo` → `computeOutboundValidity` → `generateSelectorJSONs` |
| Endpoints в множестве финальных тегов | `core/build/preset_outbounds.go:387-390` | комментарий: «Endpoints тоже в set: sing-box резолвит group members / route outbounds / detour и через endpoint manager» |
| Пул для пикера UI | `ui/configurator/business/node_pool.go:167` | служебные узлы по умолчанию не предлагаются |

---

## 3. Конвейер пресетов

### 3.1 Структура `Preset` (что пресет умеет)

| Поле | json | Файл:строка |
|---|---|---|
| `ID` | `id` | `core/template/preset_types.go:27` |
| `Label` | `label` | `:30` |
| `Description` | `description` | `:33` |
| `DefaultEnabled` | `default_enabled` | `:38` |
| `Num *int` | `num` | `:46` |
| `Sortable *bool` | `sortable` | `:55` |
| `Locked bool` | `locked` | `:60` |
| `Platforms []string` | `platforms` | `:65` |
| `Vars []PresetVar` | `vars` | `:68` |
| `RuleSet []PresetRuleSet` | `rule_set` | `:72` |
| `DNSServers []PresetDNSServer` | `dns_servers` | `:76` |
| `Rules []map[string]interface{}` | `rules` | `:87` |
| `DNSRule map[string]interface{}` | `dns_rule` | `:90` |
| `DNSRules []map[string]interface{}` | `dns_rules` | `:97` |
| `Outbounds []PresetOutbound` | `outbounds` | `:109` |

Структура объявлена `preset_types.go:24`, закрыта `:110`.
**Поля `endpoints` в `Preset` НЕТ** — подтверждено: `grep -rn "endpoints" core/template/*.go`
(без тестов) даёт 0 совпадений. Единственные упоминания endpoints в
preset-конвейере — чтение `ctx.Cache.Endpoints` в `core/build/preset_outbounds.go:387,390`.

Сопутствующие типы: `presetGate` `:125` (+`EnableRaw()` `:130`),
`PresetOutbound` `:152-222`, `PresetVar` `:229-303`, `OptionEntry` `:306`,
`PresetRuleSet` `:318-339`, `PresetDNSServer` `:349-390`
(поля `tag`:351, `type`:354, `server`:357, `server_port`:361, `path`:364,
`tls`:367, `detour`:372, `inet4_range/inet6_range`:384-385).
Хелперы: `DisplayLabel()` `:115`, `DecodeOptions()` `:400`,
`canonicalVarType` `:448`, `UnmarshalJSON` `:455`.

### 3.2 Разворачивание пресета (`preset_expand.go`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| **`const TagSeparator = ":"`** | `core/build/preset_expand.go:36` | определён в пакете `build`, не в `template` (D-012) |
| `PresetFragments` | `preset_expand.go:39-63` | `RuleSets`:42, `RoutingRules`:52, `DNSRule`:55, `DNSRules`:59, `DNSServers`:62 — **и всё**; endpoints поля нет |
| `ExpandWarning` | `preset_expand.go:66` | `String()` `:71` |
| `ExpandPreset(...)` | `preset_expand.go:87` | обёртка над `…WithGlobals` |
| **`ExpandPresetWithGlobals(preset, userVars, globalVars, target)`** | `preset_expand.go:103` | основная точка входа |
| Сборка `varsMap`, приоритет | `preset_expand.go:117-152` | `v.Ref`→globals `:122` > userVars `:131` > `v.Default` `:135`; globals подмешиваются для необъявленных `:148` |
| Фильтр vars по `if`/`if_or` | `preset_expand.go:156` | `filterActiveVars` (опр. `:311`) |
| Шаг rule_set | `preset_expand.go:179-212` | гейт `:181`; **префикс тега** `preset.ID + TagSeparator + localTag` `:207`; `emittedTags[localTag]` `:210` (по ЛОКАЛЬНОМУ тегу) |
| Шаг routing rules | `preset_expand.go:216-265` | `extractGateFromMap` `:219`, `rewriteRuleSetRefs` `:243`, `outboundutil.ApplyOutboundToRule` `:246`, `isRuleEmpty` `:248` |
| Шаг dns_rule + dns_rules | `preset_expand.go:268-281` | singular `:268` эмитится ПЕРЕД плюралом `:274` |
| Шаг dns_servers | `preset_expand.go:288-309` | strip `if`/`if_or`/`title` `:297-299`; strip `detour=="direct-out"` `:301`; **префикс тега** `:303` |
| **Подстановка `@var`** | `preset_expand.go:387` | `substitutePresetBody(raw, presetVars, varsMap, target)` → `template.SubstituteVarsInJSONStrict` на `:404`; `ok=false` при неразрешённой `@var` |
| Движок подстановки | `core/template/substitute.go:88` | `SubstituteVarsInJSONStrict`; нестрогий `:79`; canon `:104` |
| `rewriteRuleSetRefs` | `preset_expand.go:509` | string-форма `:520`, массив `:527-541`; dangling выкидываются, пустой массив → ключ удаляется `:523` |
| `expandOnePresetDNSRule` | `preset_expand.go:597` | **префикс `server`-тега** `:619` |
| `presetVarsToTemplateVars` / `…WithExtras` / `varsMapToResolved` | `:422` / `:443` / `:471` | |
| Все места применения `TagSeparator` | `:207`, `:303`, `:520`, `:533`, `:619` | rule_set tag, dns server tag, два вида rule_set-ссылок, dns rule `server` |

### 3.3 Слияние в секции (`preset_merge.go`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `PresetMergeContext` | `core/build/preset_merge.go:155-199` | `ExecDir`:161, `TemplateDNSDefaults`:166, `Target`:170, `GlobalVars`:177, `TemplateVars`:184, `EmittedRuleSetTags`:198 |
| **`MergePresetsIntoRoute(routeRaw, ctx)`** | `preset_merge.go:219` | |
| ранний выход | `preset_merge.go:223` | только если `!hasAnyV6Rule && !hasNonSortablePreset` |
| резолв | `preset_merge.go:234` | `ResolveRouteWithGlobals`; временный `state.State{Rules,DNS}` + `template.TemplateData{Presets}` `:232-233` |
| emit rule_set с dedup | `preset_merge.go:246-255` | skip `Skipped \|\| !Enabled` |
| **порядок правил: голова вперёд** | `preset_merge.go:264-279` | `if r.OrderNum < state.UserRuleNumStart { head = append(head, r.Body) }` `:270`; `rules = append(head, rules...)` `:276` |
| **`MergePresetsIntoDNS(dnsRaw, ctx)`** | `preset_merge.go:312` | |
| резолв DNS | `preset_merge.go:320` | `ResolveDNS(st, &tdVal, ctx.GlobalVars, ctx.Target)` |
| dns_servers пресета → `dns.servers` | `preset_merge.go:344-356` | фильтр `Active && Enabled`, dedup по tag с шаблонными `:336-343` |
| dns_rules пресета → `dns.rules` | `preset_merge.go:369-383` | `DNSSourcePreset` → as-is `:375`; `DNSSourceUser` → `cleanDanglingDNSRule` `:377` |
| post-обработка | `preset_merge.go:396`, `:402` | `pruneDNSGroupMembers`, `repairDanglingDNSRefs` |
| `convertPresetRuleSetRemoteToLocal` | `preset_merge.go:43` | remote→local, content-addressed tag, проверка `bin/rule-sets/<tag>.srs`, drop `url`/`download_detour`/`update_interval` |
| `SRSTagFromURL` / `srsTagFromURLLocal` | `:150` / `:152` | |
| `hasNonSortablePreset` | `preset_merge.go:201` | |
| `CollectEmittedRouteRuleSetTags` | `preset_merge.go:472` | |
| `cleanDanglingDNSRule` | `preset_merge.go:537` | |
| `CollectSrsCachedPaths` | `preset_merge.go:609` | |
| `repairDanglingDNSRefs` / `pruneDNSGroupMembers` | `:660` / `:728` | |

### 3.4 Outbounds пресета (`preset_outbounds.go`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| Файловый docstring: биндинг живёт в state через `Ref`/`Updates` | `core/build/preset_outbounds.go:1-17` | runtime-путь — `sync_outbounds.go` + `resolve_outbounds.go::mergeOutboundUpdates` |
| `outboundSentinelLiterals` | `preset_outbounds.go:35` | `reject/block/drop/direct/dns-out` не dangling |
| `presetOutboundEntry` | `preset_outbounds.go:50` | внутренний |
| `ExpandPresetOutbounds(preset, userVars, target)` | `preset_outbounds.go:73` | varsMap `:79-86` — **без** globalVars (в отличие от `ExpandPreset`); `evalIf` `:103`; `mode==""→"add"` `:107`; `substitutePresetBody` `:131`; strip `mode`/`if`/`if_or` `:144-146` |
| `applyOutboundUpdate` | `preset_outbounds.go:196` | Tag/Type immutable; AddOutbounds — union; Options — per-key |
| `CleanDanglingOutboundsInRouteRules` | `preset_outbounds.go:278` | |
| **`collectAllFinalOutboundTags(ctx, cfg)`** | `preset_outbounds.go:373` | обходит `ctx.Cache.Outbounds` **и** `ctx.Cache.Endpoints` `:387-390` |
| Потребители `ExpandPresetOutbounds` | `core/build/sync_outbounds.go:112`, `:226`; `core/build/resolve_outbounds.go:77`; `core/build/migrate_outbounds_spec058.go:101` | |

### 3.5 Порядок вызовов в сборке

| Шаг | Файл:строка |
|---|---|
| `BuildConfig` | `core/build/build.go:171` (docstring шагов `:151-170`) |
| `effectiveConfig` | `core/build/build.go:212` |
| **`buildOrderedSections`** | `core/build/build.go:242` |
| `collectAllFinalOutboundTags` (только не-preview) | `core/build/build.go:246` |
| `sanitizeOutboundGraph` — ДО обхода секций | `core/build/build.go:253` |
| `CollectEmittedRouteRuleSetTags` — один раз до цикла, и в preview | `core/build/build.go:266` |
| цикл `order` → `buildSection` | `core/build/build.go:268-279` |
| `buildSection` (диспетчер) | `core/build/build.go:285` |
| `case "outbounds"` | `:287-303` (TLS-трансформы `:292-300`) |
| `case "endpoints"` | `:304-306` — **пресеты в эту ветку не заходят** |
| `case "dns"` → `MergeDNSSection` `:308` → **`MergePresetsIntoDNS`** `:312` → `SanitizeDNSDetours` `:322` (не в preview) |
| `case "route"` → `MergeRouteSection` `:327` → **`MergePresetsIntoRoute`** `:332` → `CleanDanglingOutboundsInRouteRules` `:342` (не в preview, fallback `extractRouteFinal` `:341`) |

### 3.6 Загрузка пресетов и preset-ref в состоянии

| Сущность | Файл:строка | Заметка |
|---|---|---|
| Путь шаблона `<execDir>/bin/wizard_template.json` | `internal/platform/platform_common.go:116-118` | `GetWizardTemplatePath` |
| `TemplateFileName` | `core/template/loader.go:42` | |
| `LoadTemplateData(execDir)` | `core/template/loader.go:247` | чтение `:253`, strip BOM `:258` |
| root-структура шаблона, `Presets json.RawMessage` | `core/template/loader.go:266` | рядом `parser_config`, `config`, `dns_options`, `params`, `vars` |
| `LoadPresets` (шаг 5) | `core/template/loader.go:342` | опр. `core/template/preset_loader.go:43` |
| `filterPresetsByPlatform` | `core/template/loader.go:346` (опр. `:543`) | |
| `TemplateData.Presets []Preset` / `PresetWarnings` | `core/template/loader.go:72` / `:74` | |
| `validatePreset` и спутники | `core/template/preset_loader.go:115`, `:185`, `:317`, `:357`, `:478`, `:540`, `:550`, `:615`, `:626` | `validateRuleSetRefs` `:550`, `findDuplicateDNSTag` `:626` |
| **`state.Rule` — preset-ref на диске** | `core/state/rule_types.go:46-72` | `Kind` `:48`, **`Ref`** `:51`, **`Enabled`** `:54`, **`OrderNum *int`** `:68`, `Body` `:70` |
| `RuleKindPreset/Inline/Srs` | `core/state/rule_types.go:15` / `:19` / `:23` | |
| `PresetBody{Vars}` — только diff от дефолтов | `core/state/rule_types.go:79-81` | |
| `InlineBody{Name, Match, Outbound}` | `core/state/rule_types.go:84-93` | |
| `SrsBody{Name, SrsURL, Outbound}` | `core/state/rule_types.go:96-100` | |
| `DecodeBody` | `core/state/rule_types.go:117` | preset требует непустой `Ref` `:119` |
| DNS preset-ref сервера: `Ref = "<preset_id>:<local_tag>"` | `core/state/dns_options.go:50` (коммент `:47-49`) | `DNSServerKindPreset` |
| DNS preset-ref правила: `Ref = "<preset_id>"` | `core/state/dns_options.go:62` | один dns_rule-ref на пресет |
| `DNSServer{Kind,Ref,Enabled,…}` | `core/state/dns_options.go:75-96` | сериализация `:143-155`, десер. `:183-194` |
| `DNSRule{Kind,Ref,Enabled,…}` | `core/state/dns_options.go:99-111` | emit `ref` `:212` |
| `DNSOptions` | `core/state/dns_options.go:128` | |

**Асимметрия:** у route-правил (`state.Rule`) есть `order_num`; у DNS-preset-ref'ов
(`dns_options.go:75`, `:99`) поля порядка **нет** — порядок задаётся позицией
в списке и порядком эмиссии в `MergePresetsIntoDNS`.

---

## 4. Ось порядка правил

### 4.1 Числовая ось

| Сущность | Файл:строка | Значение |
|---|---|---|
| Раскладка оси (комментарий-закон) | `core/state/rule_order.go:9-24` | `0` голова, `950..990` шаблонные якоря, `1000..1100` пользовательская зона, `1110..1150` широкие перехватчики; «ЗАКОН ОСИ (SPEC 113-C): массив правил ВСЕГДА отсортирован по оси» `:19-24` |
| `UserRuleNumStart = 1000` | `core/state/rule_order.go:32` | |
| `UserRuleNumEnd = 1100` | `:34` | |
| **`DefaultRuleNum`** | `core/state/rule_order.go:36` | `= UserRuleNumStart` |
| **`MinSortableRuleNum = 1`** | `core/state/rule_order.go:44` | единственная жёсткая граница (решение пользователя 28.08.2026) |
| `RuleOrderSpec.Num` | `core/state/rule_order.go:52` | |
| **`RuleOrderSpec.Sortable`** | `core/state/rule_order.go:55` | |
| `RuleOrderSpec.DefaultEnabled` | `:57` | |
| `Preset.Num` / `Preset.Sortable` / **`Preset.Locked`** | `core/template/preset_types.go:46` / `:55` / `:60` | `Locked` = запрет выключения и удаления в UI, роль ОТДЕЛЬНАЯ от `Sortable` |
| `Preset.OrderNum()` | `core/template/preset_lite.go:41-46` | nil → `state.DefaultRuleNum` |
| `Preset.IsSortable()` | `core/template/preset_lite.go:51-53` | `Sortable == nil \|\| *Sortable` |
| `RuleOrderSpecs(presets)` — мост шаблон→state | `core/template/preset_lite.go:58-70` | |
| `Rule.OrderNum *int` | `core/state/rule_types.go:68` | |
| **`PlaceRuleAfter(rules, movedIdx, targetIdx, sortable)`** | `core/state/rule_order.go:275` | |
| `PlaceRuleBefore` | `core/state/rule_order.go:313` | остаётся для вырожденного «списка выше нет» |
| `placeRuleAt` (общее тело, ленивый сдвиг) | `core/state/rule_order.go:323` | блок `:350-357`, сдвиг `:360-364` |
| `MarkRuleOrder` | `core/state/rule_order.go:77` | |
| `SortRulesByNum` (стабильная) | `core/state/rule_order.go:116` | |
| `SeedRequiredRules` | `core/state/rule_order.go:140` | сидит только `!spec.Sortable` `:152` |
| `DedupePresetRules` | `core/state/rule_order.go:183` | |
| `NormalizeRuleOrder` | `core/state/rule_order.go:210` | дедуп → seed → разметка → сортировка |
| `NextUserRuleNum` | `core/state/rule_order.go:222` | |
| `ruleOrderNum(r)` (nil → DefaultRuleNum) | `core/build/resolve_route.go:340-345` | |
| `ResolvedRule.OrderNum` | `core/build/resolve_route.go:84` | заполняется `:248` (preset) и `:276` (inline) |
| re-seed на каждой сборке | `core/build/resolve_route.go:150-154` | `NormalizeRuleOrder(state.Rules, template.RuleOrderSpecs(td.Presets))` |

Модель UI: `RuleState.OrderNum` `ui/configurator/models/rule_state.go:38-42`;
`PresetRefState.OrderNum` `ui/configurator/models/preset_ref_state.go:33-38`;
`applyAxisAfterMove` `ui/configurator/models/rule_order_axis.go:85`
(вызов `PlaceRuleBefore` `:104`, `PlaceRuleAfter` `:110`);
`isSortableAxisRule` `:199`; `NextRuleOrderNum` `:213`;
`MoveRuleSlot` `ui/configurator/models/rule_slot.go:114`.

### 4.2 Системная голова `traffic-processing`

Отдельной Go-константы с этим тегом **нет** — идентификатор живёт данными в
шаблоне, а код узнаёт голову только через `sortable:false` / `OrderNum < UserRuleNumStart`.

| Что | Файл:строка | Факт |
|---|---|---|
| Объявление пресета | `bin/wizard_template.json:980-985` | `"id":"traffic-processing"`, `"default_enabled":true`, `"num":0`, `"sortable":false`, `"locked":true` |
| Реестр контракта | `contract/registry/presets.json:25` | `"traffic-processing": {"in":"mobile", … locked, num=0, isSortable=false …}` |
| Клэмп при драге | `core/state/rule_order.go:280-282` | `if want < MinSortableRuleNum { want = MinSortableRuleNum }` (обоснование `:261-274`) |
| Несортируемые исключены из вытеснения | `core/state/rule_order.go:327-336` | `if !sortable(rules[movedIdx]) { return }` |
| Клэмп в слотах UI | `ui/configurator/models/rule_slot.go:128-132` | `isSystemSlot(moved)` → отказ; `to < firstSortableSlotIndex()` → отказ |
| `isSystemSlot` / `firstSortableSlotIndex` | `rule_slot.go:176-191` / `:196-203` | распознавание по `!Presets[i].IsSortable()` |
| Голова вперёд на сборке | `core/build/preset_merge.go:268-286` | |
| Гард раннего выхода | `core/build/preset_merge.go:201-203`, `:223` | `hasNonSortablePreset` |

### 4.3 Решения D-050…D-053a

Папки `SPECS/106-*` и `SPECS/113-*` **не содержат** `DECISIONS.md` — решения
живут в `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md`
(ссылка из `SPECS/106-F-C-PRESET_MODEL_UNIFICATION/TASKS.md:3`).

| Решение | Файл:строка | Кратко |
|---|---|---|
| D-049 | `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md:57` | Модель пресетов канонизируется по LxBox: один пресет = route-правила + DNS-серверы + DNS-правила + свои переменные |
| D-050 | `…/DECISIONS.md:58` | Базовые правила переезжают из зашитого `config.route.rules` в неотчуждаемый пресет `traffic-processing` (sniff + hijack-dns + resolve) с `locked`/`isSortable:false`; неотчуждаемость держится re-seed'ом на каждой сборке, а не флагом |
| D-051 | `…/DECISIONS.md:59` | Числовая ось: `orderNum` в состоянии, `ui.num` — стартовое значение в шаблоне; шаг 10 между якорями, зона пользователя 1000–1100, ленивый сдвиг при drag, `isSortable:false` исключает из сдвига |
| D-052 | `…/DECISIONS.md:60` | Логика сложных пресетов мигрирует из LxBox целиком: `ref`-переменные, глобальные vars как fallback, `required` с Dropped-каскадом, магические имена (`outbound`-override, `dns_enable`, `dns_server`-фильтр), гейты валидности, reject-backstop, нормализация detour у DNS-групп |
| D-053 (шапка) | `…/DECISIONS.md:61` | Инварианты оси, которые запрещено «упрощать» |
| **D-053а** | `…/DECISIONS.md:61` | каскад сдвига останавливается на **первой дырке**, а не двигает всех с `num >= want` |
| D-053б | `…/DECISIONS.md:61` | зоны **не перенумеровываются** — иначе `num` шаблона перестаёт быть якорем |
| D-053в | `…/DECISIONS.md:61` | дедуп по preset_id идёт **перед** seed'ом |
| D-053г | `…/DECISIONS.md:61` | несортируемые исключены и из перемещения, и из сдвига |
| D-059 / D-061 | `…/DECISIONS.md:67` / `:69` | глобальные vars видны телу пресета; `MergePresetsIntoRoute` не выходит рано |

Прочее по SPEC-файлам: `SPECS/106-…/SPEC.md:25-30` и `:32-37`;
`SPECS/113-F-C-AUDIT_FIXES/SPEC-C-RULE_ORDER_INVARIANT.md:8-14`, `:47-56`,
`:58-79` (решение 28.08.2026 отменяет клэмп к `UserRuleNumStart`), `:81-93`, `:99-103`.

### 4.4 UI: строка-якорь пресета

Рисование — `buildSinglePresetRefRow`, `ui/configurator/tabs/rules_unified_rows.go:72-349`;
диспетчер слотов — `buildUnifiedRuleRows`, `rules_unified_rows.go:39-62`
(`SlotKindPresetRef` → `:59`).

| Элемент | Файл:строка | Факт |
|---|---|---|
| **Признак «системная строка»** | `rules_unified_rows.go:107` | `systemRule := tplPreset != nil && !tplPreset.IsSortable()` — по `Sortable`, **не** по `Locked` (обоснование `:99-106`) |
| Lookup пресета по `pr.Ref` | `rules_unified_rows.go:86-94` | линейный поиск по `model.TemplateData.Presets[i].ID` |
| **Имя якоря** | `rules_unified_rows.go:96` → `presetTileLabel` `:426-438` | `tpl.Label`, пустой → `tpl.ID`; префикс `"🔗 "`; broken → `"🔗 ⚠ Broken preset: %s"` |
| Хвост-суммарий vars | `:434-437`, `summarizePresetVarsCompact` `:440-462` | |
| Tooltip | `rules_unified_rows.go:119-123` | `tplPreset.Description` |
| Тумблер enabled | `rules_unified_rows.go:194-232` | `enableCh.Checked = pr.Enabled` `:195` |
| **Запрет выключения** | `rules_unified_rows.go:196-199` | `if systemRule { enableCh.Disable() }` |
| Запрет при broken ref | `:234-236` | |
| **Шестерёнка (Edit)** | `rules_unified_rows.go:242-249` | остаётся и у системной строки (комментарий `:104-106`); дизейбл только при `brokenRef` `:247` |
| Диалог правки | `ui/configurator/tabs/preset_ref_edit_dialog.go:35` | `showEditPresetRefDialog` |
| Кнопка удаления | `rules_unified_rows.go:251-269` | + `CompactRuleOrderIndices` `:260-261` |
| **Запрет удаления у системной** | `rules_unified_rows.go:309-315` | `buildRowEditDelCluster(editBtn, nil)` — delBtn не попадает в строку |
| **Запрет перетаскивания** | `rules_unified_rows.go:271-276` | `if !systemRule { … NewDragHandle(...) }` — ручки нет вовсе |
| Inline-селектор outbound (только при ровно одной var типа `outbound`) | `rules_unified_rows.go:128-177` | `outCount != 1 → soloOutVar = nil` `:139-141`; запись в `pr.Vars[soloOutVar.Name]` `:174` |
| Клик по подписи = тоггл | `:303` | `newRowLabelToggleTap` |
| Хелперы ряда | `ui/configurator/tabs/row_scaffold.go:33`, `:45`, `:54`, `:80` | |
| Коммит драга | `rules_unified_rows.go:467-473` | → `wizardmodels.MoveRuleSlot(model, from, to)` |
| Строка обычного правила (контраст) | `ui/configurator/tabs/rules_tab.go:284-354` | drag-handle безусловно `:332`, delete всегда `:329-331` |
| Библиотека пресетов | `ui/configurator/tabs/library_rules_dialog.go:45` | источник `model.TemplateData.Presets` `:53` (**фильтра по Sortable/Locked нет**); имя `:83-86`; «already added» `:87-90`, `:116-119`; добавление `:181-203` |

### 4.5 Цель правила (`outbound`) и список целей

| Что | Файл:строка | Факт |
|---|---|---|
| inline-правило | `core/state/rule_types.go:91-92` | `Outbound string` — тег либо литерал `reject`/`drop` |
| srs-правило | `core/state/rule_types.go:99` | то же |
| preset-правило | `core/state/rule_types.go:80-82` | своего поля цели нет — цель приезжает через var типа `outbound` (магическое имя, D-052) |
| UI-модель custom | `ui/configurator/models/rule_state.go:36-37` | `SelectedOutbound` |
| UI-модель preset-ref | `ui/configurator/models/preset_ref_state.go:21-23` | `Vars map[string]string` |
| `GetEffectiveOutbound` | `ui/configurator/models/rule_state_utils.go:17-23` | |
| `EnsureDefaultOutbound` | `ui/configurator/models/rule_state_utils.go:26-33` | |
| Сохранение цели | `ui/configurator/models/preset_ref_sync.go:263` | |
| **`GetAvailableOutbounds(model)`** | `ui/configurator/business/outbound.go:69` | единая точка списка целей |
| базовый набор | `business/outbound.go:70-74` | `DefaultOutboundTag`, `RejectActionName`, `"drop"` |
| мемоизация по `model.Revision` | `:80-86` | |
| Направления | `:97-107` | пропуск `Disabled`; берётся `Tag` + все `AddOutbounds` |
| исключение `<tag>-auto` и тегов замен свёрнутых папок | `:91-95`, `:108-113` | D-9А, SPEC 108 S3 |
| теги пресетов (`preset.outbounds[]` mode=add) | `:115-123` | `collectActivePresetOutboundTags` |
| глобальные outbound'ы шаблона | `:125-140` | `model.TemplateData.GlobalOutbounds()` |
| `EnsureDefaultAvailableOutbounds` | `:259-264` | |
| Вызов из вкладки Rules | `ui/configurator/tabs/rules_tab.go:164`, `:179`, `:182` | |
| Селектор в строке custom | `rules_tab.go:398-421` | |
| Сброс осиротевших целей на загрузке | `business/outbound.go:112-113` (коммент) | `state.resetForeignRuleTargets` |

---

## 5. DNS

### 5.1 Формы и типы

| Сущность | Файл:строка | Заметка |
|---|---|---|
| Файловый docstring — «формы покрывают пять типов» | `ui/configurator/tabs/dns_server_form.go:1-15` | |
| **Комментарий про tailscale** | `ui/configurator/tabs/dns_server_form.go:8-11` | дословно: «Прочие (quic, h3, hosts, fakeip, dhcp, `tailscale`, legacy\*) правятся на вкладке JSON — она остаётся запасным путём, как у Направления, иначе типы без формы стали бы недоступны.» |
| **Типы с формой** | `dns_server_form.go:35-41` | `dnsTypeUDP="udp"` `:36`, `dnsTypeTCP="tcp"` `:37`, `dnsTypeTLS="tls"` `:38`, `dnsTypeHTTPS="https"` `:39`, `dnsTypeGroup="group"` `:40` |
| Порядок в выпадающем списке | `dns_server_form.go:44` | `dnsFormTypes` |
| Порт по умолчанию | `dns_server_form.go:49-58` | tls→853, https→443, иначе 53 |
| Единая точка записи (форма и ручной JSON) | `dns_server_form.go:13-15` | «Форма собирает map и отдаёт его в `applyDNSServerJSON` — тот же путь, что у ручного JSON. Второй путь записи означал бы вторую реализацию проверок» |
| **Чтение JSON → форма** | `dns_server_form.go:303-350` | `Load(obj) bool`; тип пустой → `udp` `:305-307`; глубокая копия тела в `f.base` `:322`; SNI из `obj["tls"]["server_name"]` `:333-337`; участники группы из `servers` `:341` |
| **Неизвестный тип → `return false`** | `dns_server_form.go:308-318` | форма НЕ заполняется, `f.base` не ставится |
| **Запись форма → JSON** | `dns_server_form.go:357-415` | старт с копии `f.base` `:363`; удаляются ТОЛЬКО управляемые ключи `type,tag,server,server_port,path,detour,domain_resolver,servers,mode,error_ttl,win_ttl` `:364-370`; в `tls` переписывается только `server_name`, `insecure`/`alpn` сохраняются `:375-391`; `detour` пишется только если ≠ `dnsNoDetour()` `:410-412` |
| `putIfSet` / `dnsDeepCopyMap` | `dns_server_form.go:558` / `:421` | пустые поля не пишутся |
| `syncRows` — у `group` скрыты detour/resolver/server/port | `dns_server_form.go:211-246` | комментарий `:236-239`: детур на группе ядро не применяет |
| Диалог сервера: `formOK = form.Load(body)` | `ui/configurator/tabs/dns_tab.go:895-897` | |
| При `!formOK` — вкладка **только JSON** | `dns_tab.go:955-964` | текст: «This server type has no form yet — edit it as JSON…» |
| При `!formOK` в JSON-поле — исходное `body` как есть | `dns_tab.go:919-924` | |
| Источник истины на Save | `dns_tab.go:989-996` | форма только если `formOK && tabs.Selected() == formTab`, иначе текст JSON |
| Единая точка записи сервера | `dns_tab.go:807` | `applyDNSServerJSON` |
| Вкладка DNS — конструктор | `ui/configurator/tabs/dns_tab.go:62` | `CreateDNSTab`; сборка `:487-492` |
| Контейнер серверов / перерисовка | `dns_tab.go:72` / `refreshList` `:74-215` | bundled-серверы пресетов дописываются в КОНЕЦ того же списка `:89-91`, `:211-213` |
| Контейнер правил / перерисовка | `dns_tab.go:352` / `rebuildUnified` `:370-386` | `ReconcileDNSRuleOrder(m)` `:374`; `buildUnifiedDNSRuleRows` `:379` |
| Переключатель «список ↔ raw JSON» | `dns_tab.go:354-366`, `:400-457` | |
| Legacy `DNSRulesEntry` (скрытый, только для sync на Save) | `dns_tab.go:295-301`, `:463-465` | |
| Прочие хелперы вкладки | `dns_tab.go:495`, `:514`, `:550`, `:583`, `:623`, `:649`, `:695`, `:750`, `:769`, `:807`, `:863`, `:883`, `:1023` | |
| **`DNSUserRule{Enabled, Body map[string]interface{}}`** | `ui/configurator/models/dns_user_rule.go:22-25` | |
| `WizardModel.DNSUserRules` | `ui/configurator/models/wizard_model.go:138` | |
| `DNSUserRulesFromText` / `…ToText` | `dns_user_rule.go:33-58` / `:63-79` | из текста срезаются `kind/ref/enabled`, всем `Enabled=true`; disabled правила ВКЛЮЧАЮТСЯ в вывод |
| `DNSRuleSlotKind`, `DNSSlotKindUser` / `DNSSlotKindPresetRef` | `ui/configurator/models/dns_rule_slot.go:26`, `:30`, `:33` | |
| `DNSRuleSlot{Kind, Index}` / `RebuildDNSRuleOrder` / `ReconcileDNSRuleOrder` | `dns_rule_slot.go:37-40` / `:50-62` / `:75-` | |
| `buildUnifiedDNSRuleRows` | `ui/configurator/tabs/dns_unified_rules.go:34-62` | обход `model.DNSRuleOrder`, dispatch по `slot.Kind` `:48`, `:54` |
| Строка пользовательского DNS-правила | `dns_unified_rules.go:65-138` | префикс `"✏️ "` `:85`; удаление + `CompactDNSRuleOrderIndices` `:116-121` |
| Строка preset-DNS-правила (read-only) | `dns_unified_rules.go:145-236` | префикс `"🔗 "` `:184`; тело резолвится `build.ExpandPresetWithGlobals` `:189-197`; только View JSON `:220-227`; чекбокс = `pr.IsDNSRuleEnabled() && pr.Enabled` `:211` |
| `moveDNSSlot` / `addDNSUserRule` / `syncDNSRulesTextToHiddenEntry` | `dns_unified_rules.go:238` / `:253` / `:273` | |
| Редактор пользовательского правила | `ui/configurator/tabs/dns_user_rules.go:103-316` | working-copy `:117`; SRS vs Inline по наличию `rule_set` `:127-131`; picker сервера `wizardbusiness.DNSEnabledTagOptions` `:176` |
| `dnsRuleSummary` / `updateFromForm` / `collectAllRuleSetTags` | `dns_user_rules.go:43` / `:318` / `:403` | |
| SPEC-обоснование выбора пяти типов | `SPECS/109-F-N-DNS_FULL_CONFIGURATOR/SPEC.md:63` | `tailscale`, `legacy*` формы не получают |

### 5.2 Валидация DNS-типов при сборке

| Факт | Якорь |
|---|---|
| Структура `allowlists.json` | `contract/registry/allowlists.json:1-3` — `{"v":1,"allowlists":{…}}` |
| Всего **7** ключей, типов DNS среди них нет | `utls_fingerprints` `:4`, `ss_methods` `:24`, `tuic_congestion` `:38`, `tuic_udp_relay_mode` `:46`, `hysteria2_obfs` `:53`, `packet_encoding` `:60`, `vless_flow` `:68`. Подстроки `dns` и `tailscale` в файле нет вовсе |
| В `core/build/*.go` (без тестов) `allowlist`/`Allowlist` — **0 совпадений** | проверено грепом |
| Тип DNS-сервера при сборке **не валидируется ничем** — ни аллоулистом, ни switch'ем | тело сервера проносится в конфиг as-is `core/build/resolve_dns.go:500-520` |
| Упоминания типов — только в комментариях | `core/template/preset_types.go:354` (`"udp" \| "https" \| "tls" \| "h3"`), `core/build/resolve_dns.go:517` (`type:"fakeip"`), `core/state/dns_options.go:15` |
| `core/template/template_validate.go` (665 строк) типы DNS-серверов **не проверяет** | единственное упоминание `dns` — `:566`, про типы **переменных** (`outbound / dns_server / interface / enum`) |
| Тип var `dns_server` валидируется отдельно | `core/template/preset_loader.go:362`, `:384-390`, `:454`; алиас `dns_servers → dns_server` — `core/template/preset_types.go:443-451` |
| Единственный «гейт» типа — выпадающий список формы | `ui/configurator/tabs/dns_server_form.go:44` |

**Следствие:** DNS-сервер произвольного типа (в т.ч. `tailscale`) проходит
сборку — единственным рубежом остаётся `sing-box check` / `run`.

**Все упоминания `tailscale` в репозитории:**
`ui/configurator/tabs/dns_server_form.go:10` (комментарий формы),
`core/core_capabilities_test.go:24` (build-tag `with_tailscale` в фикстуре),
`SPECS/109-F-N-DNS_FULL_CONFIGURATOR/SPEC.md:63`,
`SPECS/009-F-C-WIREGUARD_URI/SPEC.md:60`,
`internal/daemonpb/started_service_grpc.pb.go:39-43`, `:98-102`, `:431-…`
(сгенерированный gRPC-код демона).
**В `contract/` и в `core/build/` слова `tailscale` нет.**

### 5.3 `detour` DNS-сервера при сборке

| Сущность | Файл:строка | Заметка |
|---|---|---|
| Запись `detour` из пресетного описания | `core/build/resolve_dns.go:508-509` | `if ds.Detour != "" { body["detour"] = ds.Detour }` |
| Подстановка `@var` в `detour` | `core/build/resolve_dns.go:188` | комментарий с примером `"detour": "@dns_google_dot_outbound"` |
| Strip `detour: "direct-out"` — две зеркальные ветки | `core/build/resolve_dns.go:533-536`, `:571-575` | плюс на раскрытии пресета `core/build/preset_expand.go:17` (п. 8), реализация `preset_expand.go:301` |
| `SanitizeDNSDetours(dnsRaw, finalTags)` | `core/build/dns_detour_sanitize.go:48-94` | |
| Читается **только** `servers[].detour` | `dns_detour_sanitize.go:56-77` | `rawServers := dnsObj["servers"]` `:56`; `detour, ok := servers[i]["detour"].(string)` `:67`; проверка `finalTags[detour]` `:71` |
| Политика при висячем detour | `dns_detour_sanitize.go:71-78` | ключ удаляется + WarnLog («resolution will go direct») |
| Признание разрыва в шапке файла | `dns_detour_sanitize.go:6-12` | «Ребро "DNS-сервер → outbound" в этот обход не попадало вовсе» |
| Расхождение политик зафиксировано в коде | `dns_detour_sanitize.go:19-27`, `:77` vs `outbound_graph_sanitize.go:226-231` | у DNS-сервера снимается **ключ**, у outbound-узла выбрасывается **носитель** |
| Вызов из сборки (не в preview) | `core/build/build.go:322-324` | гейт `!ctx.ForPreview && len(finalOutboundTags) > 0` |
| Хранение в state | `core/state/dns_options.go:90` | в `Body` для `kind=user` |
| Миграция v6→v7 | `core/state/migration_v6_to_v7.go:816` | «4. detour DNS-серверов (kind=user, Body-карта)» |

---

## 6. Граф-санитайзер сборки

| Сущность | Файл:строка | Заметка |
|---|---|---|
| Файловый закон — «один проход по ГРАФУ вместо трёх частных проверок» | `core/build/outbound_graph_sanitize.go:1-42` | правила 1–5 перечислены `:19-38`; фикспойнт и мутация `finalTags` `:40-42` |
| `graphEntry` | `outbound_graph_sanitize.go:52-61` | `prefix`, `raw`, **`isEndpoint bool`** `:58` |
| `graphEntry.typ()` / `isGroup()` / `isChain()` | `:63` / `:68` / `:73` | `isChain` = `typ()=="chain"` |
| `members()` — ссылки из `m["outbounds"]` | `:76` | |
| `setMembers` | `:87` | |
| **`sanitizeOutboundGraph(cache, finalTags)`** | `outbound_graph_sanitize.go:104` | точка входа; возвращает `(*ParsedCache, []SourceExclusion)` |
| Ранний выход на пустом кэше | `:105` | `len(cache.Outbounds)==0 && len(cache.Endpoints)==0` |
| Разбор записей обоих видов | `:109-128` | `parse := func(raw, isEndpoint bool)` `:110`; endpoint-цикл `:128` |
| Сборка результата | `:212` | `out.Endpoints = rebuildEntries(entries, true, false)` |
| Внутренние замыкания входа | `:110-124` (`parse`), `:139-146` (`drop`), `:154-183` (`dropDetourCarrier`) | |
| Фикспойнт-цикл | `outbound_graph_sanitize.go:188-207` | верхняя граница `len(entries)*4+8` `:188` |
| Мутация множества тегов при дропе | `outbound_graph_sanitize.go:144` | `delete(finalTags, e.tag)` |
| `sanitizeEntryRefs` — правила 1–3 | `outbound_graph_sanitize.go:223` | detour `:227`; правила позиций цепочки `:234-249`; члены группы `:250-275` |
| `pruneChainLeavesUnderGroups` — правило 4 | `outbound_graph_sanitize.go:283` | |
| `breakDependencyCycle` — правило 5 | `outbound_graph_sanitize.go:338` | `edge{from, ref, kind}` `:346-350`; `tref{ref, kind}` `:353`; сбор рёбер `:362-368`; `kind` = `member`/`chain` `:357-361`, `detour` `:366-368` |
| `rebuildEntries(entries, endpoints, compact)` | `outbound_graph_sanitize.go:425-428` | разделение по `e.isEndpoint != endpoints`; вызовы `:211-212` — outbounds компактно, endpoints с отступом |
| **Экспортируемых идентификаторов в файле нет ни одного** | — | единственный вызов `sanitizeOutboundGraph` — `core/build/build.go:254` |
| `wireguard` / `masque` в санитайзере **не упоминаются** | — | endpoints трактуются обобщённо по флагу `isEndpoint`; `isGroup()` только `selector`/`urltest` `:70`, `isChain()` только `chain` `:73` → endpoint проходит **только** через ветку detour `:227-231` |
| `masque` во всём `core/build/` — один раз | `core/build/tls_transforms.go:108` | список типов, к которым TLS-трансформы НЕ применяются |
| **Endpoints входят в `finalOutboundTags`** | `core/build/preset_outbounds.go:387-390` | `for _, list := range [][]json.RawMessage{ctx.Cache.Outbounds, ctx.Cache.Endpoints}` |
| Проверка `route.rules[].outbound` | `core/build/preset_outbounds.go:278` + `:328` | `CleanDanglingOutboundsInRouteRules` / `cleanDanglingOutboundRefInRule`; вызов `core/build/build.go:342` |
| Проверка `dns.servers[].detour` | `core/build/dns_detour_sanitize.go:48` | вызов `core/build/build.go:322` |
| Литералы, не считающиеся dangling | `core/build/preset_outbounds.go:35` | `reject/block/drop/direct/dns-out` |

**Рёбра, которых в графе нет:** `dns.servers[].endpoint` (поле tailscale-сервера
sing-box 1.12) не читается нигде — ни в `dns_detour_sanitize.go`, ни в
`outbound_graph_sanitize.go`. `route.rules[].outbound` покрыт
`CleanDanglingOutboundsInRouteRules`, но политика там — **замена на fallback**,
а не выброс носителя (в отличие от `detour`, правило 1, `:20-26`).

---

## 7. Бэкап / контракт / задачи

### 7.1 Версии

| Что | Якорь | Значение |
|---|---|---|
| Версия контракта | `contract/VERSION:1` | **0.12.8** |
| Шапка BACKUP.md отстала | `contract/docs/BACKUP.md:1` | «контракт **0.12.5**» |
| Шапка TASKS_LXBOX.md отстала | `contract/TASKS_LXBOX.md:1` | «контракт **0.12.7**» |

### 7.2 `contract/docs/BACKUP.md` §9 — слияние (`:334-467`)

Режима `replace` **нет** (D-095); полной замене подлежит только `rules[]`.

| Сущность | Ключ слияния | Строка |
|---|---|---|
| Подписки | `url` **байт в байт**, без нормализации; совпало → локальная запись остаётся (id, узлы, история), из файла берутся настройки; `identity` — слепком целиком; `disabled` — **объединение** | `BACKUP.md:348-373` |
| Серверы без `folder` | по **ТЕЛУ** исходника (`uri` без фрагмента `#`, `config_json` в канонической форме D-007, wg-quick — текстом); **тег в сравнении не участвует**; совпало → пропуск без warning, иначе тег + `-2` | `BACKUP.md:375-406` |
| Серверы с `folder` | папка по имени как есть, с регистром; члены по телу внутри папки, новые в конец | `BACKUP.md:408-415` |
| Цепочки и Направления | по **тегу**, точно с регистром; занятый тег → приехавшее НЕ применяется, `backup_chain_exists` / `backup_direction_exists` | `BACKUP.md:417-424` |
| DNS | серверы по `kind`+`tag`, dns-правила по `kind`+`ref`+телу; «своё сильнее»; `dns.final`/`dns.strategy` замещаются | `BACKUP.md:426-437` |
| `vars` / `route.final` | только portable-имена (`backup_var_skipped`); final замещает только при известной цели (`backup_final_dropped`) | `BACKUP.md:438-443` |
| `warp[]` | ключ `device_id`, иначе приватный ключ; дубль пропускается молча | `BACKUP.md:445-452` |
| **`rules[]`** | **единственная секция полной замены**; неизвестная цель → правило приезжает выключенным с `backup_unknown_outbound` | `BACKUP.md:454-457` |

### 7.3 Таблицы поддержки полей desktop/mobile (§2, `BACKUP.md:33-215`)

Колонки: **Поле | Тип | Поддержка | Смысл** (в последней таблице тип опущен,
`BACKUP.md:191`). Значения «Поддержка»: `обе` / `launcher` / `LxBox`
(легенда `BACKUP.md:35-39`). Разделы: Корень `:41`, `subscriptions[]` `:60`,
`servers[]` `:83`, `chains[]` `:96`, `subscriptions[].identity` `:107`,
Папки `:140`, `directions[]` `:162`, `rules[]/dns/vars/route/warp[]` `:189`.

Показательные строки:

| Строка | Запись | Заметка |
|---|---|---|
| `BACKUP.md:73` | `skip` \| array объектов \| **launcher** | фильтры отсева узлов |
| `BACKUP.md:76` | `exclude_from_global` \| bool \| launcher | класс упразднён (SPEC 118): поле читается, но НЕ применяется, warning `backup_source_flag_dropped` |
| `BACKUP.md:103` | `chains[].label` \| string \| **LxBox** | лаунчер игнорирует молча, на экспорте не пишет |
| `BACKUP.md:174-175` | `ping_url`, `ping_timeout_ms` \| **LxBox** (D-096) | лаунчер игнорирует молча и провозит |
| `BACKUP.md:196` | `rules[].dns`, `rules[].resolve` \| LxBox | лаунчер **отбрасывает с warning** |
| `BACKUP.md:206-209` | `warp[].sni`, `idle_timeout`, `keep_alive`, `endpoint`, `awg` \| **LxBox** | лаунчер не применяет, но провозит сырым JSON |

**Три разных судьбы чужого поля** уже существуют в контракте: провозится молча
(`chains[].label`, `warp[].*`), отбрасывается с warning (`rules[].dns`),
читается-но-не-применяется (`exclude_from_global`).

### 7.4 `contract/schema/`

Шесть файлов: `backup.schema.json` (31 КБ), `direction.schema.json`,
`node.schema.json`, `registry.schema.json`, `source_chain.schema.json`,
`source_fold.schema.json`.

| Сущность | Якорь | Заметка |
|---|---|---|
| **Узел** — «LX Canonical Node Envelope» | `contract/schema/node.schema.json:4` | корень `{v:1, nodes[], dropped[], meta}` `:11-30` |
| `$defs.node`, обязательные `kind\|scheme\|entry` | `node.schema.json:36`, `:37` | |
| `kind ∈ outbound\|endpoint\|group` | `node.schema.json:40` | **endpoint как класс в контракте уже есть** |
| `scheme` — имя из `registry/protocols/` | `:41` | |
| `entry` — sing-box outbound/endpoint map **БЕЗ tag**, `additionalProperties: true` | `:44-49` | |
| `chain[]` — ближний хоп первым, `maxItems: 8` | `:50-55` | |
| `warnings[]` — коды из `registry/warnings.json` | `:56-60` | |
| **`dns` / `route` в схеме узла отсутствуют вовсе** | `node.schema.json` | ни одного вхождения |
| `dns` и `route` — только на **корне бэкапа** | `backup.schema.json:289` (`rules[].dns`), `:300` (корневой `dns`, `dnsRef` `:307`, `:313`), `:331` (`route`), `:438` (`$defs.dnsRef`) | |
| Своей `source.schema.json` **нет** | подписка/сервер/цепочка описаны в `backup.schema.json` (`subscriptions[]`, `servers[]`, `chains[]`) | `$defs.direction` `:438` |
| Цепочка хопов (SPEC 110) | `contract/schema/source_chain.schema.json` | `hops[]` `minItems:2` `:12-20`, `idle_timeout` `:21`, `strip_evasion` `:25`, `strip` enum `tls.fragment\|multiplex.padding\|xhttp.padding\|tls.utls` `:29-40` |
| Свёртка подписки (SPEC 108) | `contract/schema/source_fold.schema.json` | `mode ∈ select\|auto\|select_auto` `:9-12`; `auto` через `$ref` на `direction.schema.json#/$defs/auto` `:14-17` |

### 7.5 Экспорт/импорт бэкапа (`core/backup/`)

| Сущность | Файл:строка |
|---|---|
| **`Export(s *state.State, opts ExportOptions) (*Backup, []Warning, error)`** | `core/backup/export.go:51` |
| `exportSubscription` / `exportServer` / `exportFolder` / `exportServerNode` | `export.go:283` / `:367` / `:384` / `:417` |
| `exportChain` / `exportDirections` / `exportSourceIdentity` / `exportWarp` / `exportRule` / `exportDNS` | `:242` / `:197` / `:321` / `:258` / `:443` / `:535` |
| **`Import(s *state.State, b *Backup, opts ImportOptions) (*ImportResult, error)`** | `core/backup/import.go:214` |
| `importSubscription` / `importServer` / `importChain` / `importDirections` | `import.go:407` / `:536` / `:592` / `:370` |
| `importSourceIdentity` / `importRule` / `importDNS` / `importWarp` | `:490` / `:615` / `:820` / `:896` |
| `s.Rules = nil` перед слиянием (единственная полная замена) | `import.go:235`; далее `mergeSubscriptions(...)` `:238` |
| Слияние | `core/backup/merge.go` |

**Механизм неизвестных ключей — НЕ passthrough, а «отбросить + назвать»,
три слоя:**

| Слой | Файл:строка | Механизм |
|---|---|---|
| 1. `Parse(data []byte)` | `core/backup/file.go:83` | комментарий-якорь `:70-82`: «Схема намеренно открыта (`additionalProperties: true`)… Но применять неизвестное молча нельзя (П3)»; возврат `append(typeWarns, scanUnknown(data)...)` `:91` |
| 2. **`scanUnknown(data) []Warning`** | `core/backup/file.go:351` | второй разбор сырого JSON в `map[string]json.RawMessage` `:352`; обход на всю глубину по спискам разрешённых ключей `rootKeys, subscriptionKeys, serverKeys, chainKeys, directionKeys, ruleKeys, warpKeys, dnsRefKeys` `:359-386`; путь пишется целиком `:348`; упразднённый `extensions` — один warning на файл `:338-342` |
| 3. `decodeTolerant(data)` — терпимость к чужому **типу** знакомого ключа | вызов `file.go:84`, потолок `maxTypeMismatchPasses` `:93` | комментарий `:77-82`: `subscriptions[].skip` boolean у LxBox против списка фильтров у launcher → поле отбрасывается с warning, импорт продолжается (П6) |

Точечный passthrough через `json.RawMessage` — только там, где раздел
переносится целиком: `Backup.Warp []json.RawMessage` `core/backup/types.go:53`;
`Server.ConfigJSON` `:361`; `Rule.Match/DNS/Resolve` `:404-406`;
`DNSRef.Value` `:425`.

Кастомный разбор `identity`: `SubscriptionIdentity.UnmarshalJSON`
`core/backup/types.go:189` — `presentKeys` `:157-167`, `identityKeyOrder` `:172`,
`identityAppliedKeys` `:180`, `UnappliedKeys()` `:230`. Якорь `:163-166`:
«Общий обход неизвестных ключей (`scanUnknown`) внутрь identity не спускается
намеренно, иначе одна потеря давала бы два предупреждения».
Тот же приём — объявить поле LxBox в типе, чтобы `scanUnknown` его не поймал:
`import.go:141`, `import.go:452`, `types.go:281`, `types.go:288`.

### 7.6 `contract/TASKS_LXBOX.md` — формат задачи

Заголовок: `## N. <тема> — <статус/приоритет>`, где статус — либо
`ЗАКРЫТО (D-NNN)`, либо `(приоритет N)`. Дословно первая задача
(`TASKS_LXBOX.md:301-317`):

```
## 1. `label` у `servers[]` / `chains[]` — ЗАКРЫТО (D-082 + D-094)

Развилка решена дважды и по-разному для двух секций:

- **`servers[]`** — вариант **Б** (D-082): поля в схеме нет, у узла одно имя,
  тег. Чтение — legacy-вход для файлов 0.11 и раньше: у сервера без
  `node_tag` подпись становится тегом, разошедшаяся отбрасывается с
  `backup_label_dropped`. LxBox `label` в этой секции не пишет и показывает
  `tag`;
- **`chains[]`** (и `directions[]`) — вариант **А** (D-094, контракт 0.12.4):
  поле объявлено в схеме, **Поддержка: LxBox**. Он подпись пишет и читает,
  лаунчер игнорирует её молча и не пишет; предупреждения нет ни у одной
  стороны. Ожидание корпуса side-specific:
  `corpus/backup/chains_roundtrip.expected.lxbox.json` — у `chains[0]` есть
  `label`, `warnings` пусты; в базовом ожидании ключа нет.

Осталось за LxBox: §405 (см. п. 12 выше) и зелёный `app/test/contract/` на
синхронизированном корпусе.
```

Пример с приоритетом — `TASKS_LXBOX.md:320`:
`## 2. Дериватив тега замены — сверка правила (приоритет 1)`.

**Последняя задача — № 8** (`TASKS_LXBOX.md:383`). Нумерация заголовков идёт
1…8: `:301, :320, :333, :346, :355, :368, :376, :383`. В шапке «Ответы и
статус» (`:3-…`) есть отдельный список пунктов до № 12 (ссылка «см. п. 12
выше» `:315`), но заголовков `##` там нет — не путать нумерации.

### 7.7 `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md` — формат D-NNN

Формат объявлен `DECISIONS.md:3`:
`` Формат: `D-NNN | дата | решение | кто | обоснование` ``.
Физически — markdown-таблица, шапка `:7-8`.
Правило `:4-5`: «Решения не переписываются — отменённое получает новую запись
со ссылкой на старую».

Два примера дословно:

`DECISIONS.md:9`
```
| D-001 | 2026-08-18 | Унификация контрактная, не кодовая: две реализации (Go/Dart), общими делаются форматы данных, блок-схемы, тесты | Пользователь | «Работать через Go-сущность на мобиле — неправильно»; gomobile/FFI-мост отвергнут |
```

`DECISIONS.md:11`
```
| D-003 | 2026-08-19 | Канонический узел — JSON-конверт с `kind: outbound|endpoint|group` | Пользователь + план | «Учти, что бывают endpoint»; sing-box ≥1.11 |
```

**Максимальный существующий номер — D-097**
(`SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md:105`, последняя строка файла,
2026-09-03, XHTTP `host`/`path`/`mode` из плоских параметров, контракт 0.12.7).
Грепом `D-[0-9]{3}` по всему `SPECS/` выше D-097 ничего нет; **все записи
D-NNN живут в этом одном файле** — следующий свободный номер **D-098**.

### 7.8 Прочее

| Сущность | Якорь | Заметка |
|---|---|---|
| Реестры | `contract/registry/` | `allowlists.json`, `warnings.json`, `presets.json`, `containers.json`, словари протоколов |
| Реестр пресетов (`traffic-processing`) | `contract/registry/presets.json:25` | |
| Реестр warning-кодов | `contract/registry/warnings.json` | напр. `:293` — AWG-заголовки |
| Sync-тест словарей | `core/config/subscription/registry_sync_test.go` | «Реестр без проверки — просто текст» |
| Общий корпус фикстур | `contract/corpus/` | гоняют оба приложения |

Правило контракта (`SPECS/CONSTITUTION.md:115-141`): всё, что видят **оба**
приложения, описывается в `contract/` **раньше** кода; версия — semver
(`contract/VERSION`), ломающее изменение формата — мажор, новые конструкции —
минор; синхронизация в LxBox через `tool/sync_contract.sh` + `contract.lock`,
копия в приложении не редактируется.

---

## 8. Возможности ядра (гейт `with_tailscale`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| Файловый docstring (модель гейта) | `core/core_capabilities.go:16-26` | «Вердикт консервативен: любая неопределённость → supported, чтобы не дропать узлы на догадках; `sing-box check` остаётся запасным рубежом» |
| Кэш вердикта по (mtime, size) бинаря | `core/core_capabilities.go:28-34` | `naiveSupportVerdict` — переустановка ядра в той же сессии перепроверяется |
| Публичный вход | `core/core_capabilities.go:39` | `(ac *AppController) CoreSupportsNaive() (bool, string)` |
| Отсутствие ядра → `true, ""` | `:47-50` | |
| Запуск пробы | `core/core_capabilities.go:72-82` | `exec.Command(singboxPath, "version")`; ошибка → `true, ""` `:76-79` |
| **Разбор `Tags:`** | `core/core_capabilities.go:84` | `var versionTagsRegex = regexp.MustCompile("(?m)^Tags:\\s*(\\S+)")` |
| Чистая часть (юнит-тестируемая) | `core/core_capabilities.go:88` | `naiveVerdictFromVersionOutput(versionOutput, libAvailable)` |
| Неизвестный формат вывода → не деградируем | `:90-92` | |
| `splitBuildTags` + замыкание `hasTag` | `:93-101` | |
| **Пример гейта по тегу** | `core/core_capabilities.go:102-104` | `if !hasTag("with_naive_outbound") { return false, "sing-box core is built without with_naive_outbound" }` |
| Второй гейт (purego + libcronet) | `:105-110` | |
| `cronetLibName` / `cronetLibAvailable` | `:114` / `:127` | |
| **`with_tailscale` уже встречается в фикстуре теста** | `core/core_capabilities_test.go:24` | строка `Tags: with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api,with_tailscale` |
| **Место деградации узла** | `core/config/outbound_generator.go:1219-1236` | `skippedNaiveHere++` `:1224`; агрегация `skippedNaive` `:1231`; источник с нулём узлов и ненулевым skip → отдельная диагностика `:1236` |
| Поля результата | `core/config/outbound_generator.go:66-73` | `SkippedNaiveNodes` `:72`, `SkippedNaiveReason` `:73` |
| Фатальный исход «все узлы срезаны» | `outbound_generator.go:1297-1298` | `no usable nodes: %d naive node(s) skipped` |
| Кэш «пресет с naive» | `core/build/parsed_cache.go:28` | комментарий про `with_naive_outbound` |
| `splitBuildTags` живёт **в другом файле** | `core/core_chain_capability.go:115` | вызовы: `core_capabilities.go:93`, `core_chain_capability.go:101` |
| Версия ядра из вывода | `core/core_chain_capability.go:119` (`coreVersionRegex`), `:123` (`coreVersionFromVersionOutput`) | |
| **Второй пример гейта — цепочки** | `core/core_chain_capability.go:96` (`chainVerdictFromVersionOutput`), цикл `:101-105`, тег `chainBuildTag = "with_lx_chain"` `:102`, отказ с версией ядра `:112-113` | |
| **`with_awg` тегом в Go НЕ гейтится** | грепом `with_awg` по `*.go` — только тесты (`core_capabilities_test.go:16,32`, `core_chain_capability_test.go:14,21`) и комментарии (`node_parser_wireguard.go:222,372`, `internal/constants/constants.go:109`) | пробы `CoreSupportsAWG` нет; AWG-деградации на уровне разбора: `WarnAWGHeaderInvalid` `core/config/subscription/parse_warnings.go:61`, `WarnAWGHeadersOverlap` `:63` |

**Полная цепочка деградации по тегу (образец для `with_tailscale`):**

| Звено | Файл:строка |
|---|---|
| Хук-переменная | `core/config/outbound_generator.go:224` — `var NaiveSupportProbe func() (supported bool, reason string)` |
| Установка хука | `core/controller.go:255` — `config.NaiveSupportProbe = ac.CoreSupportsNaive`; парный `ChainSupportProbe` `:256` |
| Вызов пробы один раз на прогон | `outbound_generator.go:1179-1182`; якорь-комментарий `:1175-1177` |
| Сброс узла с warning | `outbound_generator.go:1224-1225` — `skippedNaiveHere++` + `debuglog.WarnLog("… skipping naive node %q — %s", n.Tag, naiveReason)` |
| В отчёт сборки | `outbound_generator.go:1287-1288`, `:1438`, `:1446` |
| Фатальный случай | `outbound_generator.go:1297-1298` |
| Вид отчёта | `core/config/build_report.go:50` — `BuildReportNaiveDegraded = "naive_degraded"`; подача в фид `core/build_report_feed.go:117` |
| Аналог для цепочек | `core/config/chain_generator.go:32` (`ChainSupportProbe`), безопасное умолчание `chainSupported()` `:36-41` («nil → считаем, что знает: деградировать на догадке нельзя»), вердикт для UI `ChainSupportedByCore()` `:47` |

Политика «не деградировать по догадке» повторена дважды дословно:
`core/core_capabilities.go:91` и `core/core_chain_capability.go:99`.

---

## 9. Правила проекта (кратко)

1. **Spec-driven** — крупные фичи в `SPECS/NNN-T-S-NAME` со структурой
   `SPEC.md` / `PLAN.md` / `TASKS.md` / `IMPLEMENTATION_REPORT.md`
   (`SPECS/CONSTITUTION.md:104-107`).
2. **Контракт раньше кода** — всё, что видят лаунчер и LxBox, описывается в
   `contract/` до реализации; словарь без sync-теста — просто текст
   (`SPECS/CONSTITUTION.md:113-141`).
3. **Архитектурные инварианты** (`CONSTITUTION.md:34-40`): UI не ходит в
   `core/`/сеть напрямую (только через контроллер/сервисы); парсер
   детерминирован и без побочных эффектов (без сети, без записи, без
   глобального состояния); платформозависимый код — в `internal/platform`.
4. **Запреты** (`CONSTITUTION.md:195-201`): хардкод секретов; прямые вызовы UI
   из бизнес-логики; **игнорирование ошибок (`_ = err`)**; зависимость первого
   запуска от заблокированных доменов; деанонимизация / неявный сбор данных.
5. **Приватность** (`CONSTITUTION.md:94-99`): содержимое конфигов, домены, IP,
   идентификаторы устройств — в телеметрию нельзя; allowlist + opt-in.
6. **Обратная совместимость** (`CONSTITUTION.md:109-111`): миграции состояний
   с сохранением работоспособности старых версий, явная версионность форматов.
7. **Локализация** — язык UI английский; **английский текст в коде = источник
   истины И ключ перевода** (SPEC 111): `locale.T("English text")`
   (`SPECS/CONSTITUTION.md:167-169`). Реализация: `func T(key string) string`
   `internal/locale/locale.go:119`; `LoadExternalLocales(localeDir)` `:255`;
   код языка из имени файла `:267`; докачка `DownloadLocale` `:293`,
   URL-шаблон `:289`. Каталоги — внешние JSON в `bin/locale/`
   (`internal/locale/locale.go:5`); в репозитории фактически только
   `bin/locale/ru.json` (en — встроенный псевдокаталог `:254`).
   Гард-рейлы — `tools/l10n` в CI (`CONSTITUTION.md:169`).
8. Комментарии — на английском, длинные пояснения допустимы на русском
   (`CONSTITUTION.md:170`, `IMPLEMENTATION_PROMPT.md:38-42`); имена —
   английские (`CONSTITUTION.md:171`).
9. Логирование — только через `internal/debuglog`, не `log` напрямую; в новых
   участках обязательны точки start/success/error (`CONSTITUTION.md:88-93`).
   Ошибки пользователю — единой точкой `dialogs.ShowError` /
   `ShowErrorText`, тексты только по-английски (`CONSTITUTION.md:155-161`).
10. Производительность (`CONSTITUTION.md:187-191`): никаких блокирующих
    операций в UI-потоке (только горутины + `fyne.Do`); без синхронных
    сетевых вызовов без таймаута/контекста.
11. **Тесты и сборка**: `go build ./...`, `go test ./...`, `go vet ./...`
    (`SPECS/IMPLEMENTATION_PROMPT.md:60-62`, DoD `:269-273`; `AGENTS.md:81`).
    DoD дополнительно: линтер проекта и отсутствие новых warning'ов
    (`IMPLEMENTATION_PROMPT.md:272-273`); блоки «Качество кода» `:275-282`,
    «Требования реализации» `:284-292`, «Совместимость» `:294-300`.
12. **GUI-пакеты (fyne) исключены из `go test`** — требуют OpenGL, гоняются
    скриптами `build/test_*.sh` (`SPECS/CONSTITUTION.md:145-148`, `AGENTS.md:71`).
    Документы: `docs/BUILD_WINDOWS.md`, `docs/TEST_README.md` (`AGENTS.md:74`).
13. **Scope агента** (`AGENTS.md:9-15`): читать можно всё; менять/создавать —
    только в рамках TASKS.md/SPEC.md/PLAN.md текущей задачи. Выход за рамки
    (другие файлы, архитектура, перемещение файлов, public API) — спрашивать.
    Язык отчётов и ответов — русский (`AGENTS.md:36`). Не упоминать агентов и
    модели в документах, коммитах и коде (`AGENTS.md:47`).
14. Порядок работы (`IMPLEMENTATION_PROMPT.md:259-264`): изучить ТЗ → изучить
    код → минимальный дифф → реализовать → DoD. При неполном ТЗ —
    сформулировать Assumptions и **остановиться**, а не додумывать (`:255-257`).
15. Закрытие задачи (`AGENTS.md:56-64`, `:79-88`): обновить
    `docs/release_notes/upcoming.md`; при смене архитектуры —
    `docs/ARCHITECTURE.md`; при значимом UX — `RELEASE_NOTES.md`;
    релиз — по `docs/RELEASE_PROCESS.md`.
16. **Golden-тест сборки** — `core/build/golden_test.go:60 (докстринг с :31)`
    (`TestGoldenScenarios`, «strangler-fig регрессионная защита для
    BuildConfig»). Контракт сценария `:33-38`: каталог
    `core/build/testdata/golden/<scenario>/` с `template.json`, `state.json`,
    `cache.json`, `expected.config.json` (+ опц. `notes.md`); сравнение
    **побайтное**, при расхождении рядом пишется `actual.config.json`
    (`:40-42`); каталоги без всех четырёх файлов пропускаются (`:44-46`);
    префикс `real-` = сценарий с реальной установки, гоняется наравне,
    переменной `GOLDEN_RUN_REAL` больше нет (`:48-50`). Нормализация:
    `parserTimestampRegex` `:20`, `nodeCommentRegex` `:28`.
    Каталог сценариев сейчас — единственный `real-v088/`.
17. **Эталон миграции** — единственный файл `core/etalon_v6mig_capture_test.go`
    (шапка `:1-25`): гоняет v6-состояние с raw-кэшем через сегодняшний
    конвейер `buildSnapshotFromState` и сверяет байт-в-байт. Режимы (`:19-25`):
    по умолчанию **пропускается**; `ETALON_V6MIG=1` — сверка;
    `ETALON_V6MIG=capture` — перезапись эталона (использовано один раз в W2,
    повторно не делать).
18. **Эталонные JSON лежат не в `testdata`, а в SPEC** —
    `SPECS/118-F-N-STATE_V7/etalon/`: `README.md`,
    `real-v088.config.json` (полноформатный эталон BuildConfig, ссылка в
    `etalon_v6mig_capture_test.go:13-14`) и `v6mig/` с `state.json`,
    `01J00000000000000000000SUB.raw`, `outbounds.snapshot.json` (эталон),
    `outbounds.actual.json` (вывод при расхождении).
    Прочие `testdata` в core: `core/config/testdata`, `core/state/testdata`.
19. Правила порядка правил зафиксированы «законом оси»
    (`core/state/rule_order.go:19-24`) и тестами
    `core/state/rule_order_invariant_test.go`, `rule_order_test.go`.

---

## 10. Ловушки

1. **Номер SPEC 120 уже занят в комментариях кода.** `SPEC 120` в исходниках
   означает «служебные узлы / релеи BYPASS» (`Service`-флаг):
   `core/state/sources_v7.go:145`, `ui/configurator/tabs/preview_rows.go:40`,
   `ui/configurator/tabs/source_node_row.go:66`,
   `ui/configurator/tabs/source_edit_json.go:40`,
   `ui/configurator/business/node_pool.go:167`,
   `ui/configurator/outbounds_configurator/edit_dialog.go:170`.
   Папки `SPECS/120-*` до этой карты не существовало — номер разошёлся между
   кодом и каталогом; новые комментарии «SPEC 120» будут двусмысленны.

2. **Endpoint определяется одной строкой и только для wireguard.**
   `core/config/outbound_generator.go:1086` (`node.Scheme == "wireguard"`) и
   дублирующий гейт в `GenerateEndpointJSONBare` `:1054-1056`. Всё остальное
   уезжает в `outbounds[]`. Даже `masque` — который в sing-box ≥1.11 живёт в
   endpoints — эмитится в outbounds (`:528`). Тип, добавленный в
   `singboxSchemeByType`, но не в этот гейт, **молча окажется в outbounds**.

3. **Импорт стирает различие outbounds/endpoints на входе.**
   `singboxAllEntries` (`core/config/subscription/singbox_import.go:260-274`)
   склеивает обе секции в один список без пометки происхождения. Обратной
   информации «эта запись была endpoint'ом» дальше по конвейеру не существует —
   её восстанавливают по схеме, а не по факту импорта.

4. **`server`/`server_port` обязательны для всех, кроме wireguard.**
   `singboxTypeIsAddressless` (`singbox_import.go:371-373`) возвращает `true`
   только для `wireguard`. Любой безадресный тип (`tailscale` в их числе)
   отвергается на `:299-307` **даже если добавить его в таблицу схем**. Это
   вторая, независимая точка отказа.

5. **Игнорируемые секции сообщаются только в DebugLog.**
   `singboxIgnoredSections` (`singbox_import.go:27`) фиксирует факт в
   `IgnoredSections`, но лог — `DebugLog` (`:146-149`), а не WarnLog, в отличие
   от `UnsupportedTypes` (`:150-153`). Пользователь узнаёт о потере route/dns
   только из превью источника.

6. **У `Preset` нет секции `endpoints`, и `case "endpoints"` сборки пресетов не
   видит.** `core/build/build.go:304-306` берёт исключительно `ctx.Cache.Endpoints`.
   Любая новая «секция узла», развёрнутая как пресет, в `endpoints[]` не попадёт
   по построению.

7. **Префиксация тегов пресета применяется к пяти конкретным местам, не универсально.**
   `TagSeparator` (`core/build/preset_expand.go:36`) используется точечно:
   `:207` (rule_set tag), `:303` (dns server tag), `:520`/`:533`
   (rule_set-ссылки), `:619` (`server` в dns-правиле). Поля `outbound`,
   `detour`, `endpoint` **не префиксуются** — новое поле-ссылка не получит
   префикс автоматически.

8. **`emittedTags` в rule_set сравнивается по ЛОКАЛЬНОМУ тегу**
   (`preset_expand.go:210`), а сам тег пишется префиксованным (`:207`).
   Расхождение намеренное, но при добавлении новой сущности с тегами повторить
   эту пару легко неправильно.

9. **DNS-preset-ref'ы не имеют `order_num`.** `state.Rule` его имеет
   (`core/state/rule_types.go:68`), а `state.DNSServer` / `state.DNSRule`
   (`core/state/dns_options.go:75-96`, `:99-111`) — нет: порядок DNS-правил
   держится позицией в списке и порядком эмиссии в `MergePresetsIntoDNS`
   (`core/build/preset_merge.go:369-383`). Ось порядка для DNS не существует.

10. **`SanitizeDNSDetours` знает только ключ `detour`.**
    `core/build/dns_detour_sanitize.go:56-77` читает `dns.servers[].detour`
    и больше ничего. Поле `endpoint` у DNS-сервера (форма tailscale-сервера
    sing-box 1.12) в графе зависимостей отсутствует — висячая ссылка доедет до
    ядра.

11. **Две разные политики для висячих ссылок.** `detour` → **выброс носителя**
    (`outbound_graph_sanitize.go:20-26`, правило 1: снимать ключ запрещено,
    иначе «тихий direct»); `route.rules[].outbound` → **замена на fallback**
    (`preset_outbounds.go:278`, `core/build/build.go:341-342`);
    `dns.servers[].detour` → **удаление ключа** (`dns_detour_sanitize.go:71-78`).
    Три разных исхода для формально одного класса ошибки.

12. **Санитайзер графа не работает в preview.**
    `core/build/build.go:246-253` — `collectAllFinalOutboundTags` и
    `sanitizeOutboundGraph` вызываются только при `!ctx.ForPreview`. Превью
    показывает неочищенный конфиг; расхождение превью и реальной сборки
    заложено намеренно (`build.go` комментарий над `buildOrderedSections`).

13. **Системная строка распознаётся по `Sortable`, а не по `Locked`.**
    `ui/configurator/tabs/rules_unified_rows.go:107`. `Preset.Locked`
    (`core/template/preset_types.go:60`) задокументирован как «запрет выключения
    и удаления в UI», но UI на него не смотрит — читает `IsSortable()`.
    Якорь с `locked:true, sortable:true` получит и ручку драга, и активный
    тумблер, и кнопку удаления.

14. **Библиотека пресетов не фильтрует системные.**
    `ui/configurator/tabs/library_rules_dialog.go:53` берёт
    `model.TemplateData.Presets` целиком — ни `Sortable`, ни `Locked` не
    учитываются; защита держится только на пометке «already added» `:87-90`.

15. **Клэмп оси менялся решением пользователя.**
    `SPECS/113-…/SPEC-C-RULE_ORDER_INVARIANT.md:47-56` описывает клэмп к
    `UserRuleNumStart`, а `:58-79` его **отменяет**: действующая граница —
    `MinSortableRuleNum = 1` (`core/state/rule_order.go:44`). Верхняя часть
    файла осталась в тексте и читается как действующая.

16. **Типы DNS-серверов нигде не валидируются в Go.** Ни
    `contract/registry/allowlists.json` (там только utls/ss/tuic/hysteria2/
    packet_encoding/vless_flow), ни `core/template/template_validate.go`.
    Единственный гейт — выпадающий список формы
    (`ui/configurator/tabs/dns_server_form.go:44`), который вкладка JSON
    обходит по замыслу (`:8-11`). Ошибка типа обнаружится только на
    `sing-box check`/`run`.

17. **`sing-box check` не ловит часть ошибок старта** — задокументированный
    класс: вложенная цепочка под группой отвергается только на `run`
    (`core/build/outbound_graph_sanitize.go:32-35`). Проверка формой/санитайзером —
    единственный настоящий рубеж.

18. **`ProxySource.ConfigJSON` — вход парсера, а не поле состояния**
    (`core/config/configtypes/types.go:139-144`). Тело узла живёт в
    `state.Node.Body` (`core/state/sources_v7.go:135`). Запись «секции» в
    `ConfigJSON` не переживёт цикл сохранения.

19. **Вкладка JSON узла требует объект с непустым `type`.**
    `ui/configurator/tabs/source_edit_window.go:1893-1904` — проверка стоит
    **до** ветвления server/chain. Документ-обёртка с несколькими секциями
    через эту форму не пройдёт.

20. **Распаковка источника знает ровно две секции.**
    `ui/configurator/tabs/source_edit_json.go:111-114` — структура
    `unpackedDoc{Outbounds, Endpoints}`. Это же место — «единственная точка
    эмиссии» (SPEC 116 §O2 вариант А, `:100-104`), то есть предпросмотр состава
    источника новую секцию не покажет.

21. **Порядок ключей тела значим.** `state.Node.Body` хранится байт-в-байт
    как его написал эмиттер (`core/config/configtypes/types.go:703-707`), и
    `unpackNodesDoc` использует `json.RawMessage` именно ради сохранения
    порядка полей (`source_edit_json.go:110-111`). Перегон тела через
    `map[string]interface{}` порядок потеряет.

22. **Golden-тесты сравниваются побайтно.** `core/build/golden_test.go:33-40`,
    `expected.config.json`. Любая новая секция или сдвиг порядка ключей в
    выходном конфиге ломает `TestGoldenScenarios` — эталон придётся пересчитать
    осознанно, а не «подогнать».

23. **`.(int)` / `.([]string)` на JSON-теле.** Тела, пришедшие из JSON,
    несут `float64` и `[]interface{}`; эмиттер с прямыми type-assert'ами
    молча теряет поля — пример живёт в коде: `mtu, ok := node.Outbound["mtu"].(int)`
    (`core/config/outbound_generator.go:543`). Для новых полей та же ловушка.

24. **`resetForeignRuleTargets` сбрасывает осиротевшие цели на загрузке**
    (`ui/configurator/business/outbound.go:112-113`). Правило, чья цель — тег,
    которого нет в `GetAvailableOutbounds` (`:69`), потеряет цель при загрузке
    состояния. Тег узла в этот список сам по себе не попадает: там Направления,
    теги пресетов и глобальные outbound'ы шаблона (`:97-140`).

25. **Пул кандидатов Направлений считается дважды** — пикером формы и сборкой
    по фильтрам; поэтому `RelaysInDirections` живёт в модели, а не в UI
    (`core/config/configtypes/types.go:206-219`). Любой новый признак
    «предлагать/не предлагать» обязан быть виден обоим.

26. **Ранний выход `MergePresetsIntoRoute`.** `core/build/preset_merge.go:223` —
    функция выходит, если нет ни одного v6-правила и ни одного несортируемого
    пресета. Уже был багом (D-061, `SPECS/103-…/DECISIONS.md:69`); новое условие
    материализации секций обязано попасть и в этот гард.

27. **`ExpandPresetOutbounds` не видит globalVars**, в отличие от
    `ExpandPresetWithGlobals`: `core/build/preset_outbounds.go:79-86` против
    `preset_expand.go:117-152`. Одна и та же `@var` в `outbounds` и в `rules`
    одного пресета разрешается по-разному.

28. **`scanUnknown` ловит новый ключ бэкапа и делает из него warning.**
    `core/backup/file.go:351` — обход по спискам разрешённых ключей
    (`rootKeys`, `serverKeys`, … `:359-386`). Новый ключ, **не** добавленный
    в эти списки, приедет пользователю предупреждением «неизвестное поле».
    Обратный приём (объявить поле в типе, чтобы `scanUnknown` его не поймал)
    уже применяется: `core/backup/import.go:141`, `:452`,
    `core/backup/types.go:281`, `:288`.

29. **Слияние серверов идёт по ТЕЛУ, а не по тегу** (`BACKUP.md:375-406`).
    Узел, у которого тело осталось прежним, а добавились секции, при импорте
    будет распознан как **тот же** узел — и приехавшие секции пропадут
    «пропуском без warning». Для цепочек и Направлений ключ, наоборот,
    тег (`BACKUP.md:417-424`).

30. **`Backup.Rules` — единственная секция полной замены.**
    `core/backup/import.go:235` (`s.Rules = nil`). Правила, порождённые
    секциями узла, при импорте бэкапа будут снесены целиком и пересозданы
    только тем, что приехало в файле.

31. **Шапки контрактных документов отстают от `contract/VERSION`.**
    `contract/VERSION:1` = 0.12.8, `contract/docs/BACKUP.md:1` = 0.12.5,
    `contract/TASKS_LXBOX.md:1` = 0.12.7. Ориентироваться на `VERSION`.

32. **Две несовместимые нумерации в `TASKS_LXBOX.md`.** Заголовки задач `##`
    идут 1…8 (`:301, :320, :333, :346, :355, :368, :376, :383`), а список в
    шапке «Ответы и статус» (`:3-…`) доходит до № 12, и на него ссылаются
    внутри задач («см. п. 12 выше», `:315`). Новая задача = `## 9`, не `## 13`.

33. **Максимальный номер решения — D-097** (`SPECS/103-…/DECISIONS.md:105`);
    все `D-NNN` живут в одном файле, следующий свободный — **D-098**.
    Правило `:4-5`: решения не переписываются — отменённое получает новую
    запись со ссылкой на старую.

34. **`kind: endpoint` в контракте узла уже объявлен**
    (`contract/schema/node.schema.json:40`), а в Go endpoint определяется
    исключительно `Scheme == "wireguard"`
    (`core/config/outbound_generator.go:1086`). Контракт и код в этой точке
    расходятся уже сейчас.

35. **`entry` в схеме узла — «БЕЗ tag»** (`node.schema.json:44-49`), и
    `state.Node.Body` тоже чист от `tag`/`detour`
    (`core/state/sources_v7.go:125-135`). Тег возвращается на место только на
    эмиссии (`core/config/outbound_generator.go:1063-1065`). Секция, ссылающаяся
    на «тег этого узла», внутри тела тега не найдёт.

36. **Форма DNS-сервера сохраняет незнакомые ключи, но только на своём пути.**
    `Collect()` (`ui/configurator/tabs/dns_server_form.go:357-415`) стартует с
    копии `f.base` и удаляет лишь 11 управляемых ключей (`:364-370`) — чужое
    переживает правку формы. Но `Load` при неизвестном **типе** возвращает
    `false` (`:308-318`), `f.base` не ставится, и весь сервер уходит в
    JSON-ветку (`ui/configurator/tabs/dns_tab.go:955-964`).

37. **`DNSUserRulesFromText` срезает `kind`/`ref`/`enabled` и всем ставит
    `Enabled=true`** (`ui/configurator/models/dns_user_rule.go:33-58`).
    Переключение вкладки DNS в raw-JSON и обратно
    (`ui/configurator/tabs/dns_tab.go:400-457`) поднимет все выключенные
    пользовательские DNS-правила.

38. **Bundled-серверы пресетов дописываются в КОНЕЦ общего списка серверов**
    (`ui/configurator/tabs/dns_tab.go:89-91`, `:211-213`) — визуальный порядок
    не совпадает с порядком эмиссии в `MergePresetsIntoDNS`
    (`core/build/preset_merge.go:344-356`).

39. **Detour на группе ядро не применяет** — форма его прячет
    (`ui/configurator/tabs/dns_server_form.go:211-246`, комментарий `:236-239`).
    Ссылка, поставленная на DNS-группу мимо формы, молча не сработает.

40. **`splitBuildTags` живёт в `core_chain_capability.go:115`, а не рядом с
    `versionTagsRegex`** (`core/core_capabilities.go:84`). Пробы возможностей
    ядра размазаны по двум файлам с одинаковой структурой и дословно
    повторённым комментарием про «не деградировать по догадке»
    (`core_capabilities.go:91`, `core_chain_capability.go:99`).

41. **Хук-пробы ставятся из контроллера, а не из пакета сборки.**
    `core/controller.go:255-256` присваивает `config.NaiveSupportProbe` и
    `ChainSupportProbe`. Проба, не установленная в этой точке, останется `nil`,
    и по действующей политике (`core/config/chain_generator.go:36-41`) фича
    будет считаться **поддержанной**.

---

## 11. Реализовано волной 1 (адреса)

> **УПРАЗДНЕНО волной 3 (SPEC §10) — раздел исторический.** Хранимая форма,
> развёртывание, якорь `kind=node` и всё, что из них следовало, заменены;
> действующие адреса — в §14. Пометки «упразднено» стоят у отдельных строк
> ниже; остальные строки уехали по номерам и точными адресами больше не
> являются.

Раздел ведётся исполнителем. Снимок после W1.1–W1.10; строки соответствуют
состоянию рабочей копии на момент завершения волны.

### 11.1 Состояние (`core/state/`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `Node.Sections *NodeSections` | `core/state/sources_v7.go:174` | `json:"sections,omitempty"`; только `kind=server` |
| `type NodeSections` | `sources_v7.go:184` | `DNSServers`/`DNSRules`/`Rules` — `[]json.RawMessage` |
| `NodeSections.IsEmpty()` | `sources_v7.go:196` | |
| `NodeSections.HasRules()` | `sources_v7.go:202` | признак «узлу положен якорь» |
| ~~`NodeSectionLinks`~~ | — | **УПРАЗДНЕНО** (волна 3) вместе с `SeedNodeRules` |
| `(*Node).NormalizeNodeSections()` | `sources_v7.go:252` | пустое → nil; не-server → nil |
| `normalizeSectionsOfSources` | `sources_v7.go:208` | обход дерева, зовётся из `marshalDisk` |
| Хук нормализации на ЧТЕНИИ | `sources_v7.go:586-592` (в `normalizeNodeShape`) | drop("sections") у чужого вида |
| Хук нормализации на ЗАПИСИ | `core/state/save.go:126-129` (в `marshalDisk`) | |
| ~~`RuleKindNode = "node"`~~ | — | **УПРАЗДНЕНО** (волна 3): вида `node` нет |
| ~~`NodeRuleBody`~~ | — | **УПРАЗДНЕНО** (волна 3) |
| `(*NodeRuleBody).Link()` | `rule_types.go:125` | |
| ~~`DecodeBody` ветка `node`~~ | — | **УПРАЗДНЕНО** (волна 3) |
| **`NodeRuleDefaultNum = 945`** | `core/state/rule_order.go:57` | перед `private-ips` (950) |
| ~~`SeedNodeRules`~~ | — | **УПРАЗДНЕНО** (волна 3): сеять нечего, правила живут у узла |
| `NormalizeRuleOrder(rules, specs)` | `core/state/rule_order.go:230` | **параметр `nodeLinks` снят** (волна 3) |
| Проекция секций в канон | `core/state/adapter_source.go:169-176` | в `canonicalNodeProjection` |

### 11.2 Сборочная проекция (`core/config/`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `CanonicalNode.Sections` | `core/config/configtypes/types.go:243` | |
| `configtypes.NodeSections` + `IsEmpty()` | `types.go:251`, `:258` | зеркало `state.NodeSections` |
| `ParsedNode.Sections` / `ParsedNode.SectionsLink` | `types.go:737`, `:742` | link = `{FolderID контейнера, СЫРОЙ тег}` |
| Заполнение на эмиссии | `core/config/canonical_emit.go:212-218` | в `buildCanonicalServer` |
| `OutboundGenerationResult.NodeSections` | `core/config/outbound_generator.go:80` | |
| `config.NodeSectionSet` | `outbound_generator.go:159` | |
| Сбор в цикле эмиссии | `outbound_generator.go:1426-1449` | только удачная ветка |
| Возврат | `outbound_generator.go:1479` | |

### 11.3 Кэш сборки и слияние (`core/build/`, `core/`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `ParsedCache.NodeSections` | `core/build/parsed_cache.go:43` | |
| `build.NodeSectionSet` | `parsed_cache.go:52` | `FinalTag`, `Link`, три списка сырых тел |
| `build.NodeLink` | `parsed_cache.go:68` | третье зеркало NodeLink (leaf-пакет) |
| Заполнение кэша | `core/rebuild_snapshot.go:108-114` | + `buildNodeSections` `:120` |
| ~~`ExpandNodeSections`~~ / ~~`node_sections_expand.go`~~ | — | **УПРАЗДНЕНО** (волна 3): файл удалён, разворачивания нет — есть инъекция (§14.2) |
| `NodeSectionFragments` | `node_sections_expand.go:54` | `DNSServers`/`DNSRules`/`RoutingRules` как `[]map[string]interface{}` |
| `expandNodeFragment` | `node_sections_expand.go:142` | строгая подстановка + `UseNumber` |
| `nodeSelfVar = "self"` | `node_sections_expand.go:47` | |
| `PresetMergeContext.NodeSections` | `core/build/preset_merge.go:205` | |
| `hasNodeRouteRules()` / `hasNodeDNSFragments()` | `preset_merge.go:210` / `:221` | гарды ранних выходов |
| Гард `MergePresetsIntoRoute` | `preset_merge.go:254-258` | третье условие |
| Гард `MergePresetsIntoDNS` | `preset_merge.go:357-361` | |
| Вставка DNS-серверов узлов | `preset_merge.go:400-425` | после пресетных, dedup по тегу |
| Вставка DNS-правил узлов | `preset_merge.go:457-461` | ДО пруна и `repairDanglingDNSRefs` |
| Снятие секций с кэша в контекст | `core/build/build.go:257-263` | **после** `sanitizeOutboundGraph` |
| ~~`RouteSourceNode`~~ | — | **УПРАЗДНЕНО** (волна 3) |
| `ResolvedRouteRule.NodeTag` | `resolve_route.go:96` | |
| **`ResolveRouteWithNodeSections(...)`** | `resolve_route.go:157` | `ResolveRouteWithGlobals` делегирует сюда `:138-147` |
| ~~`resolveNodeRouteRule`~~ / ~~`ResolveRouteWithNodeSections`~~ | — | **УПРАЗДНЕНО** (волна 3): узловой ветки в резолве нет |
| `keepSectionsOfPresentNodes` | `core/build/outbound_graph_sanitize.go:463` | вызов `:216` |
| Ребро `dns.servers[].endpoint` | `core/build/dns_detour_sanitize.go:74-85` | выброс СЕРВЕРА целиком |
| `repairAfterServerDrop` | `dns_detour_sanitize.go:135` | переиспользует `pruneDNSGroupMembers` + `repairDanglingDNSRefs` |

### 11.4 Бэкап (`core/backup/`) и контракт

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `Server.Sections *ServerSections` | `core/backup/types.go:381` | |
| `type ServerSections` | `types.go:394` | + `RuleNum *float64` |
| `serverKeys` += `sections` | `core/backup/file.go:269` | |
| `serverSectionsKeys` | `file.go:274` | |
| Спуск `scanUnknown` внутрь `sections` | `file.go:383-387` | |
| `nodeAnchorNums` / `collectNodeAnchorNums` | `core/backup/export.go:386` / `:393` | |
| `exportServer(src, anchors)` | `export.go:378` | **сигнатура изменена** |
| `exportFolder(src, anchors)` | `export.go:419` | **сигнатура изменена** |
| `exportServerNode(src, link, anchors)` | `export.go:452` | **сигнатура изменена**; запись секций `:457-469` |
| `kind=node` не пишется в `rules[]` | `export.go:176-180` | |
| Чтение секций в узел | `core/backup/import.go:570-579` | в `importServer` |
| `mergeServers(...) []nodeAnchorImport` | `core/backup/merge.go:199` | **сигнатура изменена** |
| `nodeAnchorImport` | `merge.go:304` | |
| `appendNodeAnchor` | `merge.go:312` | |
| `applyImportedSections` | `merge.go:330` | секции файла ЗАМЕЩАЮТ локальные |
| `folderNodeWithBody` (было `folderHasBody`) | `merge.go:349` | возвращает индекс |
| Пересев якорей на импорте | `core/backup/import.go:350-364` | до и после `renumberImportedRules` |
| `importedNodeAnchorRules` | `import.go:963` | |
| Схема поля | `contract/schema/backup.schema.json:234-274` | `servers[].sections` |
| Таблица `servers[]` | `contract/docs/BACKUP.md:94` | Поддержка: launcher |
| Абзац слияния §9 | `contract/docs/BACKUP.md:406-417` | «Исключение — `sections`» |
| D-098 (черновик) | `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md:106` | VERSION не поднят |
| Задача LxBox | `contract/TASKS_LXBOX.md:392` | `## 9` |
| Нормативный текст | `SPECS/features/sources.md:42-57`, `:78-84` | |

### 11.5 Тесты волны

| Тест | Файл:строка | Покрывает |
|---|---|---|
| `TestBuildWithNodeSections` | `core/build/node_sections_build_test.go:45` | SPEC §8 пп. 1–4 (таблица из четырёх сценариев) |
| `TestSanitizeDNSDetours_DanglingEndpointDropsServerAndRepairsRule` | `core/build/dns_detour_sanitize_test.go:83` | SPEC §8 п. 8 |
| `TestBackupNodeSectionsRoundTrip` | `core/backup/node_sections_roundtrip_test.go:119` | SPEC §8 п. 5 |

### 11.6 Что волна 1 НЕ трогала (хвосты для W2)

- `ui/configurator/business/create_config.go:248` (`inMemoryCacheFromModel`) —
  третий производитель `ParsedCache`; `NodeSections` там не заполняются,
  превью визарда узловых фрагментов не показывает.
- `ui/configurator/presentation/presenter_state_helpers.go:92` — тронут
  ТОЛЬКО как обязательная правка сигнатуры `NormalizeRuleOrder`
  (добавлен `corestate.NodeSectionLinks(state.Sources)`).
- `docs/release_notes/upcoming.md` — по TASKS относится к W2.6.

---

## 12. Реализовано волной 2 (адреса)

> **Частично УПРАЗДНЕНО волной 3.** Живы: разбор документа узла
> (`ParseNodeDocument`/`RenderNodeDocument` — переписаны на хранимую форму,
> §14.4), извлечение связки из целого конфига (§14.5), read-only показ DNS.
> Упразднены: `NodeRefState` и вся якорная модель (заменена `NodeRuleRef`,
> §14.6). Действующие адреса — в §14.

Раздел ведётся исполнителем. Снимок после W2.1–W2.6; строки соответствуют
состоянию рабочей копии на момент завершения волны.

### 12.1 Разбор документа узла (`core/config/node_document.go`)

Чистая функция на три входа: вкладка JSON окна источника, `AppendManualConfigJSON`
и (позже) форма «Add server» SPEC 122.

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `NodeDocumentSelfVar = "@self"` | `core/config/node_document.go:56` | |
| `nodeDocTopKeys` | `:59` | `outbounds`/`endpoints`/`dns`/`route` |
| `nodeDocDNSKeys` / `nodeDocRouteKeys` | `:71-75` | внутри `dns` — только `servers`/`rules`; внутри `route` — только `rules` |
| **`IsNodeDocument(raw)`** | `:87` | признак: объект БЕЗ `type`, но с ключом документа (порядок проверок как у `classifyJSONObjectBody`) |
| **`ParseNodeDocument(raw) (body, *state.NodeSections, err)`** | `:112` | тело + секции; ошибка = полный отказ, вызывающий откатывает |
| `nodeDocEntries` | `:199` | `outbounds`++`endpoints` одним списком |
| `nodeDocSubsection` | `:217` | проверка ключей внутри `dns`/`route` с перечислением лишних |
| `nodeDocFragments` | `:264` | правила §5.1 к одному списку фрагментов |
| `nodeDocReplaceTag` | `:319` | реальный тег узла → `@self` |
| **`nodeDocRewriteStrings`** | `:334` | потоковая перезапись строковых ЗНАЧЕНИЙ с сохранением порядка ключей (ловушка §10 п. 21) |
| `nodeDocForeignVars` | `:431` | чужие `@var` → отказ |
| `nodeDocDefaultOutbound` | `:459` | дописывает `"outbound":"@self"` В КОНЕЦ текста объекта (без пересборки карты) |
| **`NodeBodyGoesToEndpoints(body)`** | `:505` | та же схема, что у `EmitNodeJSONs` (`canonicalSchemeFromType == "wireguard"`) |
| **`RenderNodeDocument(body, sections, isEndpoint)`** | `:520` | обратная операция для отрисовки вкладки |

### 12.2 Вкладка JSON узла (`ui/configurator/tabs/`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| Гейт «документ или тело» | `source_edit_window.go:1902` | `isDoc := !isChainSource && config.IsNodeDocument(...)`; общая проверка `type` пропускается только для документа |
| Своя ошибка документа | `source_edit_window.go:1973` | «Node document rejected: %s» вместо «Invalid JSON» |
| Отрисовка документом | `source_edit_window.go:2078-2087` | при непустых `scratch.Sections` |
| Приём обеих форм | `source_body_edit.go:47` (`applyServerBodyJSON`) | документ → `ParseNodeDocument`; прежняя форма секций НЕ трогает |
| `buildRowEditDelCluster(nil, nil)` | `row_scaffold.go:45-56` | пустой кластер (раньше падал на nil-объекте) |

### 12.3 Якорь правил узла (модель + строка)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| ~~`NodeRefState`~~ / ~~`node_ref_state.go`~~ | — | **УПРАЗДНЕНО** (волна 3): файл удалён, заменён `NodeRuleRef` (§14.6) |
| `(*NodeRefState).Link()` / `Clone()` | `:38` / `:46` | |
| **`SeedNodeRefsFromSources(m) bool`** | `:68` | зеркало `state.SeedNodeRules`; true = состав изменился |
| `SyncNodeRefsToStateRules` | `:122` | fallback-эмиссия |
| `SyncStateRulesToNodeRefs` | `:141` | state → UI |
| `nodeRefToStateRule` | `:172` | |
| `NodeRefRulesCount` / `NodeRefNodeEnabled` | `:192` / `:208` | для подписи и приглушения строки |
| `FindNodeByLink` | `:225` | корневой узел — по `NodeTagOrLabel()`, как у `NodeSectionLinks` |
| `WizardModel.NodeRefs` | `ui/configurator/models/wizard_model.go:122` | |
| **`SlotKindNodeRef`** | `ui/configurator/models/rule_slot.go:30` | |
| `RebuildRuleOrder` / `ReconcileRuleOrder` ветки | `rule_slot.go:59`, `:80`, `:97`, `:117` | |
| `slotOrderNum` / `setSlotOrderNum` / `axisProxyRules` ветки | `rule_order_axis.go:35`, `:54`, `:76` | якорь сортируем (`isSortableAxisRule` его не исключает) |
| `EmitStateRulesInAxisOrder` ветка `node` | `preset_ref_sync.go:108` | **сигнатура расширена** `nodeRefs []*NodeRefState` |
| `EmitStateRulesWithoutOrder` | `preset_ref_sync.go:31` | **сигнатура расширена** |
| `RuleOrderFromAxis` ветка `node` | `preset_ref_sync.go:147` (сигнатура), `:195-212` (ветка) | ссылка составная — карта по `NodeLink` |
| Пересев при загрузке | `presenter_state_helpers.go:100-105` | `SyncStateRulesToNodeRefs` + `SeedNodeRefsFromSources` |
| **Пересев при любой правке состава** | `ui/configurator/business/node_pool.go:168-175` | внутри `InvalidateNodePool` — единственная точка, которую уже зовут все 22 мутатора состава |
| Диспетчер строк | `ui/configurator/tabs/rules_unified_rows.go:60-64` | |
| **`buildSingleNodeRefRow`** | `rules_unified_rows.go:85` | 🔗 + тег, тумблер и ручка есть, edit/del нет, выключенный узел → приглушение + `node is disabled` |

Обновлённые вызывающие (расширенная сигнатура эмиссии):
`ui/configurator/business/parser.go:96`, `ui/configurator/business/create_config.go:124`,
`ui/configurator/presentation/presenter_sync.go:250`,
`ui/configurator/presentation/presenter_state.go:133`,
`ui/configurator/outbounds_configurator/configurator_helpers.go:269`.

### 12.4 Вкладка DNS (read-only фрагменты узлов)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| **`NodeSectionSetsFromModel(model)`** | `ui/configurator/business/node_sections.go:28` | финальные теги через `config.EmitCanonicalSource`; эмитятся ТОЛЬКО источники с секциями (пул целиком не строится) |
| `sourceCarriesNodeSections` | `:67` | |
| `NodeSectionDNSServer` / `NodeSectionDNSRule` | `:83` / `:94` | |
| **`NodeSectionDNSForModel`** | `:104` | развёртывание тем же `build.ExpandNodeSections`, что на сборке |
| `trimNodeTagPrefix` | `:129` | |
| `renderNodeSectionDNSRows` | `ui/configurator/tabs/dns_preset_bundled.go:114` | подпись `🔗 <финальный тег>:<локальный тег>`, только View JSON |
| `buildNodeSectionDNSServerRow` | `dns_preset_bundled.go:140` | |
| Вставка в список серверов | `ui/configurator/tabs/dns_tab.go:95` | за bundled-серверами пресетов |
| `buildNodeSectionDNSRuleRows` | `ui/configurator/tabs/dns_unified_rules.go:72` | вызов `:64` — в конце обхода `DNSRuleOrder` |
| «No DNS rules.» после рендера, а не до | `dns_tab.go:376-385` | пустой `DNSRuleOrder` ≠ «правил нет»: узловые слотов не имеют |

### 12.5 Извлечение секций из тела (`core/config/subscription/`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| **`ExtractNodeSections(cfg, nodeTag)`** | `singbox_sections_extract.go:61` | чистая функция; берёт только связанное по ссылке, ссылки → `@self` |
| **`SingleSectionCarrierTag(cfg)`** | `:125` | ровно один не-групповой и не-служебный узел, иначе "" |
| `marshalNodeSectionFragment` | `:150` | |
| `replaceJSONStringValue` | `:162` | |
| `jsonObjectList` | `:187` | |
| `sortedNodeSectionKinds` | `:203` | для InfoLog |
| `SingboxImportResult.SectionFragments` | `singbox_import.go:46` | счётчик извлечённых фрагментов |
| Вызов носителя | `singbox_import.go:178` | `sectionCarrier` считается ДО цикла разбора |
| Присвоение узлу + InfoLog | `singbox_import.go:243-251` | только удачно разобранному носителю |
| `singboxJSONNode.Sections` | `ui/configurator/business/sources_json.go:104-112` | |
| Пронос в `state.Node` | `ui/configurator/business/source_input.go:171` | через `corestate.NodeSectionsFromConfigTypes` |
| **`state.NodeSectionsFromConfigTypes`** | `core/state/adapter_source.go:289` | обратная проекция к `canonicalNodeProjection:169-176` |
| `AppendManualConfigJSON` принимает документ | `ui/configurator/business/sources_json.go:130-164` | |

**Осознанное ограничение:** извлечение работает на `map[string]interface{}`
(импорт разбирает конфиг в карту ещё до этой точки), поэтому порядок ключей
ВНУТРИ извлечённого фрагмента не сохраняется. Значим он только при байтовом
сравнении с эмиссией, а фрагменты секций ни с чем не сравниваются. Тело узла
порядок сохраняет — оно идёт своим путём (`ParseNodeDocument` → `json.Compact`).

**Секции у узлов подписки НЕ заводятся:** `canonicalNodeFromEntry`
(`core/config/migrate_materialize.go:125`) секции не переносит — оба его
вызывающих (`fetch_materialize.go:97`, `migrate_materialize.go:84`)
материализуют состав контейнера, а узлы подписки несвободны
(`features/sources.md` §Свобода, SPEC 121 §2).

### 12.6 Превью визарда

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `inMemoryCacheFromModel` заполняет `NodeSections` | `ui/configurator/business/create_config.go:270` | третий производитель `ParsedCache` (хвост волны 1 закрыт) |

### 12.7 Тесты волны

| Тест | Файл:строка | Покрывает |
|---|---|---|
| `TestParseNodeDocument` | `core/config/node_document_test.go:13` | SPEC §5.1 — таблица из четырёх входов |
| `TestParseSingboxBody_NodeSectionsFromWholeConfig` | `core/config/subscription/singbox_sections_extract_test.go:23` | SPEC §6 / §8 п. 7 — конфиг из issue |
| `TestParseSingboxBody_NoSectionsWithTwoNodes` | `singbox_sections_extract_test.go:86` | §8 п. 7, отрицательная половина |

---

## 13. Реализовано SPEC 122 «Tailscale» (адреса)

> **Действует.** Волна 3 SPEC 121 в этом разделе не тронула ничего, кроме
> формы, в которой конструктор отдаёт секции: документ он по-прежнему пишет
> sing-box-фрагментами (`dns`/`route`), а в записи их переводит общий
> конвертер (§14.3). Гейт ядра, `IsExitCapable`, форма DNS-сервера,
> `state_directory` и папочный путь — без изменений.

Раздел ведётся исполнителем SPEC 122. Строки соответствуют состоянию рабочей
копии на момент завершения задачи.

### 13.1 Признак endpoint'а и каталог состояния (`core/config/endpoint_schemes.go` — новый файл)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| **`endpointSchemes`** | `core/config/endpoint_schemes.go:29` | константная таблица `{wireguard, tailscale}`; ссылка на реестр в комментарии над каждой строкой. `masque` намеренно вне (эмитится в outbounds, `outbound_generator.go:528`) |
| **`IsEndpointScheme(scheme)`** | `endpoint_schemes.go:35` | ЕДИНСТВЕННАЯ точка истины; заменила строку `Scheme == "wireguard"` в четырёх местах (ловушка §10 п. 2) |
| `SchemeTailscale` | `endpoint_schemes.go:45` (алиас) → `core/config/configtypes/types.go:780` (объявление) | строка одна: `configtypes` — leaf-пакет и импортировать `config` не может, а предикат `IsExitCapable` живёт там |
| `SetTailscaleStateDirRoot` / `TailscaleStateDirRoot` | `:60` / `:71` | корень каталогов состояния; хук-переменная под RWMutex, ставится из контроллера |
| **`applyTailscaleStateDirectory`** | `:84` | подставляет `state_directory` ТОЛЬКО при отсутствии ключа в теле и при непустом корне |
| `sanitizeStateDirName` | `:103` | тег → имя каталога: всё, кроме `[A-Za-z0-9-_.]`, → `_`; точки по краям срезаются; пусто → `tailscale`. Склейка — `filepath.Join` (`:97`), не `"/"`: на Windows корень приходит с обратными слэшами |

### 13.2 Импорт и эмиссия (`core/config/`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `singboxSchemeByType` += `tailscale` | `core/config/subscription/singbox_import.go:383` | SPEC 118 W4 — таблица одна |
| `singboxTypeIsAddressless` знает tailscale | `singbox_import.go:404` | вторая, независимая точка отказа (ловушка §10 п. 4) закрыта |
| Гейт `EmitNodeJSONs` | `core/config/outbound_generator.go:1135` | было `node.Scheme == "wireguard"` (`:1086` до правки) |
| **`generateEndpointJSONBare(node, forConfig)`** | `outbound_generator.go:1099` | общая реализация; `forConfig=true` только из `GenerateEndpointJSON` |
| `GenerateEndpointJSON` (с state_directory) | `outbound_generator.go:1069` | **важно:** `GenerateEndpointJSONBare` путь машины НЕ пишет — его вызывающие сохраняют канонический BODY (`migrate_materialize.go:263`, `node_hash.go:112`), а путь этой машины в теле уехал бы в бэкап и на чужую машину |
| `NodeBodyGoesToEndpoints` через предикат | `core/config/node_document.go:512` | документ показывает узел в той же секции, в которой он окажется |
| Комментарии `OutboundGenerationResult` | `outbound_generator.go:52-57` | «WireGuard nodes» → «endpoint-scheme nodes» |

### 13.3 Гейт ядра (по образцу naive)

| Звено | Файл:строка |
|---|---|
| Проба (публичный вход + кэш по mtime/size) | `core/core_capabilities.go:131` — `CoreSupportsTailscale()` |
| Тип кэша / тег сборки | `core_capabilities.go:119` / `:127` (`tailscaleBuildTag = "with_tailscale"`) |
| Запуск `sing-box version` | `core_capabilities.go:165` — `probeTailscaleSupport` |
| **Чистая часть (юнит-тестируемая)** | `core_capabilities.go:177` — `tailscaleVerdictFromVersionOutput`; нет строки `Tags:` → `true, ""` (`:179-181`) |
| Текст причины | `core_capabilities.go:187` — `sing-box core is built without with_tailscale (need 1.14.0-lx.31 or newer)` |
| Поля кэша в контроллере | `core/controller.go:118-119` |
| **Установка хука** | `core/controller.go:263` — `config.TailscaleSupportProbe = ac.CoreSupportsTailscale` |
| Корень каталогов состояния | `core/controller.go:268` — `<execDir>/bin/tailscale` (тот же корень, что у локальных `.srs`) |
| Хук-переменная | `core/config/outbound_generator.go:258` — `TailscaleSupportProbe` |
| Вызов пробы один раз на прогон | `outbound_generator.go:1235-1238` |
| **Сброс узла с warning** | `outbound_generator.go:1293-1310`; код деградации в логе — `subscription.WarnTailscaleCoreUnsupported` (`:1303`) |
| Источник с нулём узлов после сброса не «пустой» | `outbound_generator.go:1314` |
| Поля результата | `outbound_generator.go:80-81` (`SkippedTailscaleNodes`/`SkippedTailscaleReason`); заполнение `:1367-1368`, `:1534-1535` |
| Фатальный случай «все узлы срезаны» | `outbound_generator.go:1380-1382` |
| **Вид отчёта** | `core/config/build_report.go:54` — `BuildReportTailscaleDegraded = "tailscale_degraded"` |
| Фид отчёта | `core/build_report_feed.go:127` |
| Строка и порядок в Итоге | `ui/configurator/tabs/final_report_model.go:100`, `:142` |
| Прочие места показа | `core/config_service.go:137-139`, `core/rebuild_snapshot.go:107-111` |
| Код деградации | `core/config/subscription/parse_warnings.go:63` + `contract/registry/warnings.json` (`tailscale_core_unsupported`, severity=warning) |

### 13.4 Направления

| Сущность | Файл:строка | Заметка |
|---|---|---|
| **`(*ParsedNode).IsExitCapable()`** | `core/config/configtypes/types.go:793` | не-tailscale → всегда true; tailscale → только с непустым `exit_node` |
| Точка 1 — сборка | `core/config/outbound_filter.go:53` | стоит ДО разбора источника: свойство узла, а не источника (узел с `SourceIndex` вне диапазона раньше проскакивал целиком) |
| Точка 2 — пикер формы | `ui/configurator/business/node_pool.go:240` | ловушка §10 п. 25 |

### 13.5 Форма DNS-сервера

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `dnsTypeTailscale` | `ui/configurator/tabs/dns_server_form.go:46` | |
| `dnsFormTypes` (шесть типов) | `dns_server_form.go:50` | |
| Виджеты `endpoint` + `accept_default_resolvers` | `dns_server_form.go:203-207` | Select по тегам узлов, при пустом пуле — Entry |
| `syncRows` — ветка tailscale | `dns_server_form.go:246`, `:265-271` | server/port/path/sni/detour/resolver скрыты, как у `group` |
| `Load` — ссылка и галка | `dns_server_form.go:414-437` | чужой тег дописывается в варианты Select, чтобы не потеряться; пустая ссылка → `ClearSelected()`, а не `SetSelected("")` (Fyne на значение вне вариантов ругается в лог) |
| `Collect` — управляемые ключи + ветка | `dns_server_form.go:455-460`, `:485-493` | `endpoint`/`accept_default_resolvers` СНИМАЮТСЯ при смене типа |
| `endpointValue` / `dnsSelectHasOption` | `dns_server_form.go:336` / `:347` | |
| `SetReadOnly` | `dns_server_form.go:320`, `:324-326` | |
| **Источник тегов узлов** | `ui/configurator/business/node_pool.go:190` — `TailscaleEndpointTags(model)` | финальные теги из пула (пул эмитит тем же `EmitCanonicalSource`, что и сборка) |
| Шапка файла (комментарий про tailscale) | `dns_server_form.go:7-11` | переписана: tailscale больше не «правится JSON'ом» |

### 13.6 Конструктор «Add server → Tailscale»

| Сущность | Файл:строка | Заметка |
|---|---|---|
| **Файл варианта** | `ui/configurator/dialogs/add_server_tailscale.go` | |
| `addServerTailscaleNoteText` | `add_server_tailscale.go:32` | текст подсказки дословно из SPEC 122 §2.5 |
| `tailscaleDefaultTag` / `tailscaleDNSTag` / CGNAT / суффикс | `:37` / `:42` / `:45` / `:48` | |
| `buildTailscaleFields(onChange)` | `:63` | Auth key — `widget.NewPasswordEntry` (`:71`) |
| **`tailscaleDocument(tag, fields)`** | `:107` | документ SPEC 121 §5.1: endpoint + `dns.servers/rules` + `route.rules`, ссылки на себя — `@self`; пустой ключ → ошибка формы |
| `modeTailscale` | `ui/configurator/dialogs/add_server_dialog.go:164` | |
| Пункт в `proto`-Select | `add_server_dialog.go:210`, `:270`, `:278-279` | |
| Видимость блока | `add_server_dialog.go:340-344` | ни host, ни port |
| **Превью вкладки JSON** | `add_server_dialog.go:469-470`, реализация `:511` | `ParseNodeDocument` → `RenderNodeDocument`: показывается то, что ПОЛУЧИТСЯ, а не то, что форма написала |
| Результат | `add_server_dialog.go:597-606` | документ в `AddServerResult.ConfigJSON`; путь записи — общий `AppendManualConfigJSON` (`sources_json.go:130`) |
| Ручная правка документа побеждает | `add_server_dialog.go:643-655`, гард `manualDocCarriesSections` `:673` | гард обязателен: голый `{"outbounds":[…]}` формально документ тоже, но это давняя многоузловая форма и её по-прежнему разбирает общий путь Add (тест `TestManualJSONResult_MultiGoesAsText`) |

### 13.7 Контракт и документы

| Что | Где |
|---|---|
| Реестр протокола | `contract/registry/protocols/tailscale.json` (`kind: endpoint`, `uri: null`, `sources: ["singbox"]`) |
| Код деградации | `contract/registry/warnings.json` — `tailscale_core_unsupported` |
| Задача LxBox | `contract/TASKS_LXBOX.md` — подраздел «Реестр протокола `tailscale` (SPEC 122)» внутри `## 9` |
| README | `README.md:37`, `README.ru.md:43` |
| Release notes | `docs/release_notes/upcoming.md` — по пункту в EN/RU Highlights и в EN/RU Technical |

### 13.8 Тесты SPEC 122

| Тест | Файл:строка | Покрывает |
|---|---|---|
| `TestTailscaleEmittedAsEndpoint` | `core/config/tailscale_test.go:66` | §4 п. 1 / §2.1: документ → тело + секции, эмиссия в `endpoints[]`, `state_directory` по тегу, явное значение не перебивается, голая эмиссия пути НЕ несёт |
| `TestTailscaleCoreGate` | `tailscale_test.go:130` | §4 п. 2 / §2.2: три пробы (нет поддержки / есть / хука нет) |
| `TestTailscaleDirectionPool` | `tailscale_test.go:178` | §4 п. 5 / §2.3: пул с `exit_node` и без, включая узел без источника |
| `TestTailscaleConfigPassesSingboxCheck` | `tailscale_test.go:217` | §4 п. 1 «последняя миля»: реальный `sing-box check`. Пропускается без `SINGBOX_TAILSCALE_BIN` |
| `TestTailscaleVerdictFromVersionOutput` | `core/core_capabilities_test.go:71` | §4 п. 6: вердикт по строке `Tags:` (фикстуры с тегом и без) |

### 13.9 Что SPEC 122 НЕ трогала

- **Папка как адресат формы.** `folder_add_nodes.go:294-300` кладёт
  `AddServerResult.ConfigJSON` в текстовый путь `applyInput` — документ
  (в отличие от одиночного outbound) он не разберёт. Это положение дел от
  SPEC 121: документ и там был бы потерян. Вариант Tailscale в окне папки
  сегодня даст «ничего не добавлено».
- **`SanitizeDNSDetours` и поле `endpoint`** — ребро уже добавлено волной 1
  (`dns_detour_sanitize.go:74-85`), правок не требовалось.
- **`masque`** остаётся в `outbounds[]` (§2.1 SPEC: отдельное решение).
- **`contract/VERSION`** не поднят — по правилу обеих сторон.

**Дополнение к §13.9 (закрыто).** Папочный путь («Add server» в окне папки →
`applyInput` → `AppendNodesToFolder` → общий парсер) принимает документ узла:
`ExtractNodeSections` считает ссылкой на узел не только его тег, но и
плейсхолдер `@self` (`core/config/subscription/singbox_sections_extract.go`,
`refersToNode`). Проверено сквозным прогоном: документ конструктора Tailscale
в папке даёт узел `kind=server` с тремя фрагментами секций.

---

## 14. Реализовано волной 3 — пересмотр модели (адреса)

Раздел ведётся исполнителем. Снимок после W3.1–W3.7; строки соответствуют
состоянию рабочей копии на момент завершения волны. **Этот раздел действующий:**
там, где он расходится с §11–§13, прав он.

### 14.1 Хранимая форма и плейсхолдер (`core/state/`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `Node.Sections *NodeSections` | `core/state/sources_v7.go:174` | `json:"sections,omitempty"`; только `kind=server` |
| **`type NodeSections{Rules []Rule, DNS *NodeSectionsDNS}`** | `core/state/node_sections.go:57` | записи лаунчера, не фрагменты sing-box; своего `UnmarshalJSON` нет |
| `type NodeSectionsDNS{Servers []DNSServer, Rules []DNSRule}` | `node_sections.go:66` | |
| `IsEmpty()` / `HasRules()` | `node_sections.go:72` / `:88` | |
| `DNSServers()` / `DNSRules()` / `SetDNS(...)` | `node_sections.go:93` / `:100` / `:108` | доступ к DNS-половине без проверок на nil |
| `Clone()` | `node_sections.go:121` | редактор владеет своей копией (SPEC 117) |
| **`(*Node).NormalizeNodeSections()`** | `node_sections.go:180` | не-server → nil; чужие виды записей → WarnLog + отброс (`dropForeignKinds` `:197`) |
| `normalizeSectionsOfSources` | `node_sections.go:244` | зовётся из `marshalDisk` (`core/state/save.go:129`) и `normalizeNodeShape` (`sources_v7.go:508`) |
| **`ReadNodeSections(raw)`** | `node_sections.go:258` | текст, написанный человеком: вкладка JSON узла; виды проверяются с ошибкой, а не молча |
| `SelfPlaceholder` / `SelfPlaceholderBraced` | `node_sections.go:52` / `:53` | `@self` / `@{self}` |
| **`SubstituteSelf(raw, finalTag)`** | `core/state/selfvar.go:51` | ЕДИНСТВЕННАЯ точка подстановки; свой обход JSON-строк, порядок ключей сохраняется |
| `SubstituteSelfInString` | `selfvar.go:79` | `@{self}` заменяется первым; `@self` — только целой строкой |
| `SubstituteSelfInMap` | `selfvar.go:97` | для тел DNS-записей (они карты, не сырой JSON) |
| `rewriteJSONStringValues` | `selfvar.go:129` | потоковая перезапись строковых ЗНАЧЕНИЙ (ключи не трогаются) |

### 14.2 Старая форма волн 1–2 — **УПРАЗДНЕНА**

Конвертер старой формы снят 2026-09-05: релизов с ней не было (решение
владельца). Отдельного чтения нет — незнакомые ключи `sections` отбрасываются
штатной нормализацией (`NormalizeNodeSections` / `dropForeignKinds`).

### 14.3 Единый перевод sing-box ↔ хранимая форма

| Сущность | Файл:строка | Заметка |
|---|---|---|
| **`SingboxNodeFragments{NodeTag, DNSServers, DNSRules, RouteRules}`** | `core/state/node_sections_convert.go:36` | вход перевода |
| **`NodeSectionsFromSingbox(in)`** | `node_sections_convert.go:55` | ЕДИНСТВЕННАЯ реализация правил перевода; ошибка = документ не годится целиком |
| `nodeSectionRuleFromBody` | `node_sections_convert.go:119` | правило → `inline`: `match` = всё, кроме `outbound`/`action`; `name` из `@{self}` |
| `nodeSectionFragmentBody` | `node_sections_convert.go:157` | перепись реального тега в `@self`, отказ на `rule_set` и чужую `@var` |
| **`NodeSectionsToSingbox(sections)`** | `node_sections_convert.go:209` | обратный перевод (для `sing-box check` и показа) |
| `nodeSectionForeignVars` / `nodeSectionVarRe` | `node_sections_convert.go:281` / `:278` | |
| Потребитель 1 — вкладка JSON узла | `core/config/node_document.go:207` | внутри `ParseNodeDocument` |
| Потребитель 2 — целый конфиг как источник | `core/config/subscription/singbox_sections_extract.go:120` (отбор `:80-118`) | внутри `ExtractNodeSections` |
| Потребитель 3 — конструктор Tailscale | `ui/configurator/dialogs/add_server_tailscale.go:107` | `tailscaleDocument` пишет sing-box-документ, разбирает его тот же `ParseNodeDocument` |

### 14.4 Документ узла (`core/config/node_document.go`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| `nodeDocTopKeys` += `sections` | `core/config/node_document.go:71` | три входа §10.4 |
| **`ParseNodeDocument(raw)`** | `node_document.go:121` | тело + `*state.NodeSections` НОВОЙ формы |
| Вход 2 — `sections` в хранимой форме | `node_document.go:178` | `state.ReadNodeSections`; смешивать с `dns`/`route` в одном документе запрещено |
| Вход 3 — `dns`/`route` (sing-box) | `node_document.go:200-215` | переводится `NodeSectionsFromSingbox` |
| **`RenderNodeDocument(body, sections, isEndpoint)`** | `node_document.go:321` | рисует `{outbounds\|endpoints:[тело], sections:{хранимая форма}}` |
| `NodeBodyGoesToEndpoints` | `node_document.go:301` | предикат тот же, что у эмиссии (`IsEndpointScheme`) |
| Приём на вкладке | `ui/configurator/tabs/source_body_edit.go:47` | `applyServerBodyJSON`; прежняя форма (голое тело) секций не трогает |
| Отрисовка документом | `ui/configurator/tabs/source_edit_window.go:2080` | при непустых `scratch.Sections` |

### 14.5 Сборка = инъекция (`core/build/`)

| Сущность | Файл:строка | Заметка |
|---|---|---|
| **`NodeSectionSet{FinalTag, Link, Sections}`** | `core/build/parsed_cache.go:55` | записи в форме хранения, ДО подстановки |
| **`RulesWithSelf()`** | `parsed_cache.go:70` | подстановка по ТЕЛУ записи (порядок ключей `match` значим) |
| `DNSServersWithSelf()` / `DNSRulesWithSelf()` | `parsed_cache.go:91` / `:106` | |
| **`PresetMergeContext.rulesWithNodeSections()`** | `core/build/preset_merge.go:214` | конкатенация к `state.Rules` + `SortRulesByNum` |
| **`PresetMergeContext.dnsWithNodeSections()`** | `preset_merge.go:246` | узловые записи в КОНЕЦ списков (оси у DNS нет) |
| `hasNodeRouteRules` / `hasNodeDNSFragments` | `preset_merge.go:277` / `:288` | гарды ранних выходов; сами выходы — `:316` и `:421` |
| Инъекция в `MergePresetsIntoRoute` | `preset_merge.go:330` | временный `state.State` из инъецированных списков |
| Инъекция в `MergePresetsIntoDNS` | `preset_merge.go:412` | |
| Инъекция в `CollectEmittedRouteRuleSetTags` | `preset_merge.go:608` (функция `:571`) | |
| Резолв без узловых веток | `core/build/resolve_route.go:130` | `ResolveRouteWithGlobals` — три ветки: preset / inline / srs |
| Снятие секций с кэша в контекст | `core/build/build.go:257-263` | после `sanitizeOutboundGraph` |
| `keepSectionsOfPresentNodes` | `core/build/outbound_graph_sanitize.go:463` | узел снят санитайзером → его записи уходят с ним |
| Ребро `dns.servers[].endpoint` | `core/build/dns_detour_sanitize.go:74-85` | волна 1, без изменений |
| Проекция «канон → сборка» | `core/config/configtypes/types.go:253` | `NodeSections{Raw json.RawMessage}` — НЕПРОЗРАЧНЫЙ блок (leaf-пакет не знает `core/state`) |
| Кодирование / декодирование блока | `core/state/adapter_source.go:175` / `:292` | `canonicalNodeProjection` / `NodeSectionsFromConfigTypes` |
| `config.NodeSectionSet` | `core/config/outbound_generator.go:173` | несёт `*state.NodeSections` |
| Сбор на эмиссии | `outbound_generator.go:1550` | только дошедшие до конфига узлы |
| Перенос в кэш сборки | `core/rebuild_snapshot.go:130` | `buildNodeSections` |

### 14.6 UI: строки правил узлов и перерисовка

| Сущность | Файл:строка | Заметка |
|---|---|---|
| **`NodeRuleRef{Link, Index, Enabled, OrderNum, Name}`** | `ui/configurator/models/node_rule_ref.go:31` | обратный указатель в ПАМЯТИ модели |
| `WizardModel.NodeRuleRefs` | `ui/configurator/models/wizard_model.go:123` | |
| **`SeedNodeRuleRefs(m)`** | `node_rule_ref.go:80` | строка на каждую запись `sections.rules[]`; true = состав изменился |
| `nodeRuleDisplayName` | `node_rule_ref.go:141` | подпись с подставленным тегом узла |
| **`SyncNodeRuleRefsToSources(m)`** | `node_rule_ref.go:161` | Save: `order_num`/`enabled` обратно в запись узла |
| `NodeRuleRefNodeEnabled` / `FindNodeByLink` | `node_rule_ref.go:186` / `:198` | |
| Пересев на любой правке состава | `ui/configurator/business/node_pool.go:169` | внутри `InvalidateNodePool` |
| Пересев при загрузке | `ui/configurator/presentation/presenter_state_helpers.go:100` | ДО `RuleOrderFromAxis` |
| Слоты правил узлов дописываются | `ui/configurator/models/preset_ref_sync.go:186` | в `RuleOrderFromAxis`, дальше общая `SortRuleOrderByAxis` (`presenter_state_helpers.go:120`) |
| Правила узлов НЕ уезжают в `state.Rules` | `preset_ref_sync.go:105` | ветка `SlotKindNodeRef` в `EmitStateRulesInAxisOrder` — `continue` |
| Вызов синхронизации на Save | `ui/configurator/presentation/presenter_state.go:112` | ДО снятия копии `state.Sources` |
| `SlotKindNodeRef` | `ui/configurator/models/rule_slot.go:30` | |
| Ось: слот правила узла сортируем | `ui/configurator/models/rule_order_axis.go:76` | прокси вида `inline` |
| **`buildSingleNodeRuleRow`** | `ui/configurator/tabs/rules_unified_rows.go:86` | `🔗 <name> · <тег узла>`; тумблер и ручка есть, edit/del нет |
| **Перерисовка по ревизии** | `ui/configurator/configurator.go:630` (счётчики), `:704` (DNS), `:717` (Rules) | `tabs.OnSelected`: вкладка старше `model.Revision` перестраивается |
| DNS-записи узлов для показа | `ui/configurator/business/node_sections.go:108` | `NodeSectionDNSForModel` — та же подстановка, что на сборке |
| Строка DNS-сервера узла | `ui/configurator/tabs/dns_preset_bundled.go:140` | `🔗 <тег сервера> · <тег узла>`, только View JSON |
| Строки DNS-правил узлов | `ui/configurator/tabs/dns_unified_rules.go:72` | в конце обхода `DNSRuleOrder` |

### 14.7 Бэкап и контракт

| Сущность | Файл:строка | Заметка |
|---|---|---|
| **`ServerSections{Raw json.RawMessage}`** | `core/backup/types.go:396` | непрозрачный блок + свои `MarshalJSON`/`UnmarshalJSON` |
| **`decodeBackupSections(sec, nodeTag)`** | `core/backup/node_sections.go:22` | одна форма §10.1 |
| `serverSectionsKeys` | `core/backup/file.go:275` | `rules`/`dns` |
| Экспорт секций | `core/backup/export.go:429` | в `exportServerNode` |
| Импорт секций | `core/backup/import.go:561` | в `importServer` |
| Замещение при совпадении тела | `core/backup/merge.go:297` | `applyImportedSections` |
| Схема поля | `contract/schema/backup.schema.json:239-278` | `servers[].sections` |
| Таблица `servers[]` | `contract/docs/BACKUP.md:94` | Поддержка: launcher |
| Абзац слияния §9 | `contract/docs/BACKUP.md:410-419` | |
| D-099 (черновик) | `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md:107` | уточняет D-098; `contract/VERSION` не поднят |
| Задача LxBox | `contract/TASKS_LXBOX.md:392` | `## 9`, переписан под новую форму |
| Нормативный текст | `SPECS/features/sources.md:45-66`, `:100-107` | |
| Release notes | `docs/release_notes/upcoming.md` | пункт «A node can now carry…» / «Узел может нести…» |

### 14.8 Тесты волны

| Тест | Файл:строка | Покрывает |
|---|---|---|
| `TestBuildWithNodeSections` | `core/build/node_sections_build_test.go:45` | SPEC §8 пп. 1–4 в новой форме (корень / папка с TagPolicy / выключенный узел / без секций = байт-в-байт) |
| `TestSanitizeDNSDetours_DanglingEndpointDropsServerAndRepairsRule` | `core/build/dns_detour_sanitize_test.go:83` | §8 п. 8 (волна 1, без изменений) |
| `TestBackupNodeSectionsRoundTrip` | `core/backup/node_sections_roundtrip_test.go:118` | §8 п. 5 в новой форме |
| `TestParseNodeDocument` | `core/config/node_document_test.go:13` | §5.1 в новой форме |
| `TestParseSingboxBody_NodeSectionsFromWholeConfig` | `core/config/subscription/singbox_sections_extract_test.go:22` | §6 / §8 п. 7 |
| `TestParseSingboxBody_NoSectionsWithTwoNodes` | `singbox_sections_extract_test.go:96` | §8 п. 7, отрицательная половина |
| `TestTailscaleEmittedAsEndpoint` / `TestTailscaleCoreGate` / `TestTailscaleDirectionPool` / `TestTailscaleConfigPassesSingboxCheck` | `core/config/tailscale_test.go:66` / `:141` / `:189` / `:228` | SPEC 122 §4 — переписаны на новую форму, ни один сценарий не снят |
| **`TestSubstituteSelf`** | `core/state/node_sections_test.go:11` | §10.1: обе формы плейсхолдера, порядок ключей, чужой текст, пустой тег |
