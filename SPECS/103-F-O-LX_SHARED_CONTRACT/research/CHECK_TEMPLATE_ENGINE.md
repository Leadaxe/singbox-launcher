# Сравнение шаблонных движков: Go (singbox-launcher/core/template) vs Dart (LxBox/app/lib)

## 1. Предикаты #if

| Форма | Go (substitute.go) | Dart (if_engine.dart) | Расхождения |
|---|---|---|---|
| bare `"@boolvar"` | есть — :354-372; `EqualFold(TrimSpace(scalar), "true")` (:372); для `@runtime.*` bare-форма запрещена (:362-365) | есть — :262-266; `_scalar(resolve(name)) == 'true'` (:265) — строго, **без trim и без case-fold** | Go толерантен к `"True"`/`" true "`, Dart — нет |
| `{"@var": "literal"}` equality | есть — :414-416; RHS проходит `substituteSimpleString` (:415, :544-567) → **можно сравнивать var-с-var** (`{"@a": "@b"}`) | есть — :287 (`scalar.trim() == arg`); RHS всегда литерал | @var в RHS — только Go |
| `#notEmpty` / `#isEmpty` | есть — :404-408; семантика по типу (checkNotEmpty :469-480): text → trim-длина, text_list → len(list)>0, **bool → scalar=="true"** (:473-475) | есть — :284-285; чисто строковая trim-длина | **Для bool-var расходятся**: Dart `#notEmpty` на bool всегда true (bool коэрсится в 'true'/'false' — обе непустые), Go = значение var |
| `{"#in": [...]}` / `{"#notIn": ...}` | есть — :424-427; аргумент — массив **или строка `"@text_list_var"`** (checkInList :484-521); элементы массива проходят @var-подстановку (:515) | есть — :293-296; аргумент только List, прямой `contains`, без @var-подстановки | `@text_list_var`-форма и @var внутри списка — только Go |
| `{"#matches": "re"}` | есть — :428-429, checkMatches :525-538; паттерн проходит @var-подстановку (:531), компиляция на каждый вызов | есть — :297-298; кэш компиляции `_matchCache` (:56-59), без @var-подстановки | @var в паттерне — только Go; кэш — только Dart |
| `{"#not": pred}` | есть — :379-381; объект обязан иметь ровно 1 ключ (:374-377) | есть — :270-273; single-key проверяет только load-валидатор (:445-448) | эквивалент |
| `and` / `or` | :304-329; оба → warn+false (:307-309); **ни одного → warn+true** (:310-313); пустой and → true (:334), пустой or → false (:342-348) | :236-252; `and`-List имеет приоритет (or игнорируется); **ни одного/невалидно → false** (:251); load-валидатор требует ровно один непустой список (:392-402) | Defensive-пути противоположны (neither: Go=true, Dart=false); у Dart жёсткая load-валидация (TemplateIfError :339-344), у Go — только warn-и-продолжай |
| `@runtime.*` в предикатах | есть — :28-51, lookupVarScalar :443-455 (platform/arch/target из TargetSpec) | **нет** (заявлено :17) | только Go |
| Только в Dart | — | `evalIfScalar` (:224-229) — вычисление одиночного `{"#if":...}` до скаляра для `on_change.set` (§232) | только Dart |

## 2. Приведение типов

| Аспект | Go | Dart |
|---|---|---|
| int-каст | **Хардкод по ИМЕНИ var** — isIntCastVar (substitute.go:15-21): `tun_mtu`, `mixed_listen_port`, `proxy_in_listen_port`, `urltest_tolerance`. `strconv.Atoi`; не число → warn + **0** (:230-236); пустой скаляр int-var → 0 (:224-228) | **По объявленному `type: 'int'`** — coerceVarValue (if_engine.dart:37-51); `int.tryParse` + **clamp(0, 65535)** (§161, :29, :44-47); не число → **возвращается исходная строка** (advisory) |
| bool | По `type: "bool"`: `EqualFold(trim, "true")`, пустое → false (substitute.go:218-222) | По `type: 'bool'`: `raw == 'true'` строго (:40) |
| text_list | Тип есть: ResolvedVar.List → `[]interface{}`; пустой список → warn + `[]` (:205-215); плюс legacy-коллапс `["@text_list_var"]` → массив (:166-177); резолв из state построчно (vars_resolve.go:314-323) | **Типа text_list нет вообще** (список типов parser_config.dart:509: bool/int/text/enum/secret/outbound/dns_servers) |
| Остальные типы | строка как есть (:238); var без типа в varTypes → строка | text/secret/enum/outbound/dns_servers + неизвестный → дословная строка (:48-50); var без ноды → coerce как 'text' (makeResolver :68-78) |

## 3. Политика unresolved var

| Режим | Go | Dart |
|---|---|---|
| Обычный | `SubstituteVarsInJSON` (substitute.go:56-59): unresolved → **`""` + WarnLog** (replacementForPlaceholderCtx :196-202) | Имя **не объявлено** → **плейсхолдер `@name` остаётся в выводе как есть** (if_engine.dart:96-98, build_config-контракт) |
| Strict (пресеты) | `SubstituteVarsInJSONStrict` (:65-67): имена собираются в sink → `UnresolvedVarError` (:71-77, :99-101) → **preset пропускается целиком** (комментарий :62-64) | Имя объявлено, значение **null** (optional-var §033) → sentinel **`Dropped`** (:21-24, :99-101): ключ/элемент удаляется из родителя (:140-148, :179-186); preset_expand.substituteVars :573-580. Плюс fragment-гейты валидности: rule без outbound/action → drop (preset_expand.dart:296-301), dns_rule без server/serverless-action → drop (:234-241), required var пустая → весь preset в warnings (:121-144) |
| В #if-предикатах | unknown var → warn + false (:367-370, :396-399) | resolve→null → `_scalar`→`''` → предикат false (:265, :281, :312-316) |

Итог: гранулярность разная — Go strict роняет пресет целиком через ошибку, Dart деградирует по-фрагментно через Dropped + гейты.

## 4. Семантика #if: map-spread и array-element

| Граничный случай | Go | Dart | Совпадает? |
|---|---|---|---|
| map-spread merge | pre-pass ДО обхода полей (substitute.go:114-133); ветка substitute → merge в parent, ветка перетирает сиблингов (:247-268) | обычные ключи резолвятся ПЕРВЫМИ, #if-ветка мержится ПОСЛЕДНЕЙ, last-wins (:137-160) | Итог одинаков (ветка бьёт сиблинга), порядок обхода разный |
| else | true→value (нет value → warn+skip :288-292); false→else если есть (:295-298) | `picked = ok ? value : else`; null → no-merge (:191-201); array: false + нет else → Dropped (:205-214). Load-валидатор требует value (:403-405) | Да; defensive-путь «true без value»: Go skip, Dart в array-режиме добавил бы `null` в массив (:207) — но валидатор не пропустит |
| array-element распознавание | элемент = map ровно с 1 ключом `#if` (:141-147, :151-152) | то же (:169-172) | Да |
| Вложенные #if | lazy: substitute только выбранной ветки (:259, :278) | lazy: walk только клона выбранной ветки (:198-199, :320-330 — clone защищает источник от мутации) | Да; клонирование — только Dart (Go декодирует свежее дерево на каждый вызов) |
| Dropped | нет аналога вне #if; элемент выпадает только через take=false (:273-280) | Dropped распространяется и от **null-var** (не только от #if): ключ/элемент с optional-var выпадает | **Только Dart** |
| Неизвестные `#ключи` | warn + drop (:126-128) | silent drop (:121-133), валидатор пропускает как forward-compat (:368-371) | Семантика та же |
| Legacy `["@text_list_var"]` | коллапс одноэлементного массива в список (:166-177) | нет | только Go |

## 5. Пресеты

| Аспект | Go (preset_types.go) | Dart (preset_expand.dart / SelectableRule) |
|---|---|---|
| Неймспейс тегов | **`<preset_id>:<tag>` автопрефикс при build** — заявлено :7-8, RuleSet :47-48, DNSServers :50-52, PresetRuleSet :240-241, PresetDNSServer :269-271 | **Префиксовки НЕТ** — теги как есть; коллизии решает mergeFragments дедупом по tag: identical → skip, конфликт → first-wins + warning (preset_expand.dart:473-526) |
| Поля | Preset :22-86: ID, Label, Description, DefaultEnabled, **Platforms**, Vars[], RuleSet[], DNSServers[] (типизированные структуры :261-309), Rules[], DNSRule+DNSRules[], **Outbounds[]** (mode add/update, :98-167) | SelectableRule (parser_config.dart:637-795): presetId, label/description/defaultEnabled (через `ui{}` :761-772), **locked** (§264), **num + isSortable** (§370 :655-678), ruleSets/rules/dnsRules/dnsServers (сырые map), vars[] |
| Только в Go | Platforms-фильтр (loader.go:298-310), Outbounds[] add/update, PresetVar.Select local/global (:207-214), «все vars required, опциональность через if/if_or» (:172-174), If/IfOr на каждом фрагменте | — |
| Только в Dart | — | `required`-флаг на var, outbound-override через `varsValues['outbound']` (:328-337), reject→action backstop (:358-361), dangling-rule_set guards (:246-278, :372-412), remote→local .srs подмена (:195-215), `enabled:"@var"`-конвенция §045 (:172-180), DNS-группы §312/§354 (:427-443), ref-vars §265 (:110-117, :152-156), globalVars-fallback §264 (:165-167) |
| Типы preset-var | outbound, dns_server, enum, text, **number**, bool (:179-186) | те же WizardVar-типы: bool, int, text, enum, secret, outbound, **dns_servers** (имя типа отличается: dns_server vs dns_servers) |

## 6. @runtime.* и params[]

**В Dart отсутствуют оба — подтверждено**: явный отказ в if_engine.dart:17 («НЕ берём: params[]-механику, @runtime.* globals»); grep по LxBox/app/lib не находит ни `@runtime`, ни params-механики в билдере.

Что есть только в Go:
- `@runtime.platform` / `@runtime.arch` / `@runtime.target` из TargetSpec (SPEC 067/097) — substitute.go:28-51, :443-455; bare-форма запрещена (:362-365).
- `params[]` (TemplateParam, loader.go:192-200): dot-notation пути (`route.rules`) с рекурсией applyParam :393-426; mode replace/prepend/append (applyValue :429-454, mergeArrays :457-472); фильтры platforms + if/if_or (applyParamsFiltered :364-388); применяются ДО var-substitute (ApplyTemplateWithVarsFor :339-352).
- if/if_or записи как JSON-предикаты того же языка #if (condEntryTrue, vars_resolve.go:525-546) + VarConditionIsTargetOnly «скрыть vs погасить» (:483-509).
- MaybeGenerateSecrets — автогенерация `type:"secret"` при пустом/CHANGE_THIS_* (vars_resolve.go:19-27, :288-309).
- default_node — фолбэк-значение из JSON-пути шаблона (vars_resolve.go:331-339, :352-356, getRawAtPath :361-379).
- per-platform default_value с #if-деревьями и псевдо-ключом `win7` (vars_default.go:15-21, :45-61, :95-107).

Что есть только в Dart (нет в Go): `on_change.set` side-effects через evalIfScalar (§232), ref-vars (§265), clamp int в uint16 (§161).

## 7. Поля объявления переменной

| Поле | Go TemplateVar (vars_resolve.go:30-56) | Dart WizardVar (parser_config.dart:489-596) |
|---|---|---|
| name / type / title / tooltip / wizard_ui / options | есть; options строка или {title,value}, объектная форма **форсит type→enum** (:146-151), OptionTitles параллельный | есть; WizardOption {value,title} (:462-479); type НЕ форсится |
| default_value | VarDefaultValue: скаляр ИЛИ per-platform объект с #if-деревьями, win7-ключ (vars_default.go:15-21, :95-107) | плоская String; per-platform объект **схлопывается** в ключ `default` или первое значение, платформа игнорируется (:570-576) |
| separator | есть (:32) | **нет** |
| default_node | есть (:36) | **нет** |
| platforms | есть (:47) | **нет** |
| if / if_or | есть (:53-55), та же грамматика предикатов (VarUISatisfiedFor :210-224) | **нет** |
| тип text_list | есть | **нет** |
| section / chapter | **нет** (секции — Separator-строки) | есть (:515-516, VarSection :601-616) |
| required | **нет** (в PresetVar «все required») | есть (§033, :521) |
| on_change | **нет** | есть (§232, :530) |
| ref (ссылка на глобальную var) | **нет** | есть (§265, :538-541) |
| тип int (объявленный) | нет — int-каст по хардкод-именам | есть, с clamp |
| типы outbound/dns_servers в глобальных vars | только в PresetVar | есть в общем WizardVar |

**Ключевые семантические расхождения одним списком**: (1) bool-сравнение — Go case-insensitive+trim, Dart строгое; (2) `#notEmpty` на bool — Go = значение, Dart = всегда true; (3) neither and/or — Go true, Dart false; (4) unresolved var — Go `""`, Dart оставляет плейсхолдер; (5) int — Go по имени → 0 при ошибке, Dart по типу → clamp/строка; (6) text_list и его #in-форма — только Go; (7) префикс `<preset_id>:<tag>` — только Go; (8) Dropped-каскад optional-var — только Dart; (9) @var-подстановка внутри RHS предикатов — только Go.