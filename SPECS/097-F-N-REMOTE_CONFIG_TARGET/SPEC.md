# SPEC 097 — Remote config target

**Тип:** Feature · **Статус:** Implemented · **Платформа:** все

Лаунчер учится готовить конфиги не только для себя. Новый шаг визарда,
предшествующий текущим, спрашивает: конфиг для **этой машины (local)** или для
**удалённой (remote)** — сервера, роутера, другого mac. Remote-конфиги живут в
собственном состоянии `bin/wizard_states/remote/`, генерируются под платформу
и роль целевой машины и **экспортируются файлом** (деплой под lxd), не трогая
рабочий `bin/config.json`.

**Depends on:** SPEC 067 (template expressions), SPEC 096 (lxd — единственный
движок на remote-машине).
**Не меняет:** вкладку Servers / gear-диалог Remote endpoint (SPEC 064) —
настройки *соединения* с удалённой машиной остаются там; визард знает только
то, что нужно для *генерации*.

---

## 1. Проблема

Генерация config.json неявно предполагает «конфиг для машины, на которой
запущен лаунчер»:

- `runtime.GOOS` / `runtime.GOARCH` зашиты в 5+ точках пайплайна
  (`core/template/loader.go:277,299,340`, `core/template/vars_resolve.go:289,310`,
  `core/build/sync_outbounds.go:93`) — конфиг для linux-роутера нельзя собрать
  с mac.
- В шаблоне локальные допущения — литералы: `experimental.clash_api`
  (на remote недоступен — управление только через lxd), `route.find_process:
  true` (бессмысленно для транзитного трафика), `set_system_proxy` (никогда
  на remote), `interface_name: "singbox-tun0"` (литерал в params, не var,
  только win/linux).
- Gateway-кейс (роутер: wifi-с-VPN + обычный wifi) требует полей, которых нет
  как понятия: `include_interface: ["br-vpn"]`, явное имя TUN-интерфейса на
  всех платформах.
- Некуда сохранить remote-состояние: `bin/wizard_states/` — плоский список
  local-снапшотов; запись собранного remote-конфига в `bin/config.json`
  убила бы рабочий VPN.

## 2. Целевая модель

### 2.1 Шаг 0 визарда

Перед текущими шагами — выбор таргета:

- **Local** — как сейчас, ничего не меняется.
- **Remote** — раскрываются дополнительные поля:
  - **платформа целевой машины** (linux / darwin / windows) — подменяет
    `runtime.GOOS` во всей генерации;
  - **gateway-режим** (bool-var) — включает поля `include_interface`
    (list-var, напр. `["br-vpn"]`), LAN-подсеть;
  - `tun_interface_name` (var, дефолт `lxd-tun0` для remote), `tun_address`,
    `tun_mtu`, `strict_route` — существующие vars.

Переключение local ⇄ remote: флаш текущего состояния → чтение целевого →
перерисовка вкладок. Молчаливой потери правок нет.

### 2.2 Хранение

```
bin/wizard_states/state.json          # local — без изменений (инвариант цел)
bin/wizard_states/<id>.json           # local-снапшоты — без изменений
bin/wizard_states/remote/state.json   # remote-состояние
bin/wizard_states/remote/<id>.json    # remote-снапшоты
```

- `StateStore` конструируется с `statesDir` ([state_store.go:44]) — для remote
  создаётся инстанс на подпапку. `ListWizardStates` со своим
  `if entry.IsDir() { continue }` даёт изоляцию списков даром.
- Новая `platform.GetWizardStatesDirFor(execDir, target)` рядом с
  существующей; `GetWizardStatePath` (local) не меняется — core-читатели
  (`varsubst`, `snapshot`, `config_service`) не трогаются.
- `srs_downloader.collectAllStageRuleSetTags` ([srs_downloader.go:246])
  обходит и `remote/`, иначе rebuild снесёт .srs, на которые ссылается только
  remote-state.

### 2.3 TargetSpec — развязка runtime.GOOS

```go
type TargetSpec struct {
    GOOS   string // целевая платформа (для local = runtime.GOOS)
    GOARCH string
    Target string // "local" | "remote"
}
```

Протаскивается через `LoadTemplateData` / `ApplyTemplateWithVars` /
`ResolveTemplateVars` / `SubstituteVarsInJSON` / `ExpandPresetOutbounds`.
`runtime.GOOS` остаётся дефолтом ровно в одном месте — конструктор
«таргет = я сам».

### 2.4 @runtime.target

Новый runtime-global (SPEC 067 namespace): `@runtime.target` = `"local"` |
`"remote"`. Строковый — встаёт в ряд к `platform`/`arch` без изменений
предикатной машинерии (одна строка в `runtimeGlobalFields` + dispatch в
`lookupVarScalar`); bare-форма остаётся запрещённой для всех globals, как
зафиксировано SPEC 067:

```json
"and": [{"@runtime.target": "remote"}]    // remote-only ветка
"and": [{"@runtime.target": "local"}]     // local-only ветка
```

Альтернатива `@runtime.isRemote` (bool) отклонена: потребовала бы типизации
globals и точечного снятия запрета bare-формы ради синтаксического сахара,
и закрывает ось двумя значениями навсегда.

### 2.5 List-vars — уже существуют

Тип `text_list` в шаблонном движке был реализован до SPEC 097
([substitute.go] `replacementForPlaceholderCtx`: `text_list` → JSON-массив,
строки разделяются переводом строки). `gateway_include_interface` объявлен
как `text_list` — нового кода не потребовалось.

Значения (`br-vpn`, LAN CIDR) — данные конкретной машины, живут в vars,
НЕ в шаблоне: шаблон один на все устройства.

### 2.6 Правки wizard_template.json (без Go-кода)

- `experimental.clash_api` → под `#if {"@runtime.target": "local"}`
  (map-spread, [substitute.go:119] уже умеет). На remote управление — только
  канал lxd (gRPC+REST, mTLS), clash_api выпиливается целиком.
- `route.find_process` → `false` при `@gateway_mode` (РОЛЬ, не таргет: у
  удалённого сервера/mac свои процессы есть, process_name-правила там
  работают; бессмыслен матчинг только для транзитного трафика шлюза).
- `proxy_in_listen` → `0.0.0.0` при `@gateway_mode` (клиенты из LAN), иначе
  `127.0.0.1` — в т.ч. на remote-СЕРВЕРЕ: mixed inbound без авторизации на
  `0.0.0.0` был бы открытым прокси в интернет.
- `proxy_in_set_system_proxy` → default `false` на remote (headless-демон
  не должен трогать сессию пользователя; включается руками).
- `tun_interface_name` → полноценный var на всех платформах (замена литерала
  `singbox-tun0` из params); default для remote — `lxd-tun0`.
- `include_interface` → эмитится в tun-inbound под `#if @is_gateway`
  (обычный bool-var remote-шага, не runtime-global).
- `experimental.cache_file.path` — на remote путём владеет lxd
  (daemon state-dir), относительный `cache.db` остаётся.

### 2.7 Вывод

- **Local** → `bin/config.json`, как сейчас.
- **Remote** → экспорт в файл по выбору пользователя (деплой под lxd).
  Рабочий `bin/config.json` при remote-таргете не пишется никогда.

## 3. Из scope

- Транспорт/деплой конфига на удалённую машину (канал lxd) — отдельная спека.
- Мониторинг удалённого ядра — вкладка Servers (SPEC 064) и/или lxd-канал,
  здесь не меняются.
- Per-source таргеты (несколько remote-машин со своими состояниями) —
  расширение `remote/` до именованных папок возможно позже, формат к этому
  готов.

## 4. Порядок реализации

1. `TargetSpec` — развязка `runtime.GOOS`/`GOARCH` (механика, ничего не
   ломает; local-путь байт-в-байт эквивалентен).
2. `@runtime.target` в `runtimeGlobalFields` + dispatch.
3. ~~List-cast для vars~~ — не нужен, `text_list` уже есть.
4. `GetWizardStatesDirFor` + `StateStore` на подпапку + walk подпапок в
   `collectAllStageRuleSetTags` И `collectAllStageSourceIDs` (у второго
   была та же дыра: он чистит bin/subscriptions/ и снёс бы raw-body
   подписки, которыми владеет только remote-state).
5. Шаг 0 визарда + флаш-и-свитч.
6. Экспорт для remote (вместо записи config.json).
7. Правки wizard_template.json + эмиссионные/golden-тесты на оба таргета.

## 5. Что реализовано

Go:
- `core/template/target.go` — `TargetSpec{GOOS,GOARCH,Target}`, `LocalTarget()`,
  `RemoteTarget()`, `Normalized()`.
- `@runtime.target` в `runtimeGlobalFields` + dispatch в `lookupVarScalar`;
  `(goos, goarch)` заменены на `TargetSpec` по всей цепочке подстановки.
- `ForPlatform` → `ForTarget`; `ResolveTemplateVarsFor`,
  `ApplyTemplateWithVarsFor`, `GetEffectiveConfigFor` — target-варианты
  рядом с legacy-обёртками (local-путь байт-в-байт прежний).
- `BuildContext.Target` + `build.TargetSpecFromState`; target протащен через
  `ExpandPreset` / `ExpandPresetOutbounds` / `ResolveDNS` / `ResolveRoute` /
  `SyncOutboundsWithActivePresets` / `MergeOutboundUpdates*` /
  `MigrateOutboundsToReferencedShape`.
- `state.meta.{target,target_platform,target_arch}` + round-trip Save/Load.
- `platform.GetWizardStatesDirFor` / `GetWizardStatePathFor`;
  `constants.ConfigTarget{Local,Remote}`; `StateStore.NewStateStoreFor`.
- `WizardModel.Target`; `presenter.SwitchConfigTarget` (flush → read → resync),
  `SetTargetPlatform`; `GetStateStore()` корневится на директории таргета.
- Вкладка Target первой в визарде; Settings рендерится по платформе таргета.

Шаблон (`bin/wizard_template.json`):
- `tun_interface_name` — новый var на ВСЕХ платформах (был литерал
  `singbox-tun0` только для win/linux); remote-дефолт `lxd-tun0`.
- `gateway_mode` (bool) + `gateway_include_interface` (`text_list`) →
  `include_interface` в TUN-инбаунде.
- `experimental.clash_api` — под `#if {"@runtime.target": "local"}`.
- `route.find_process` — true только для local.
- `proxy_in_set_system_proxy` — никогда true на remote.
- `proxy_in_listen` — `0.0.0.0` на remote, `127.0.0.1` на local.

Тесты: `core/template/target_test.go` (движок + рендер боевого шаблона на
обоих таргетах), `internal/platform/target_paths_test.go`,
`core/state/target_meta_test.go`.

## 6. Не сделано / известные ограничения

- **Экспорт remote-конфига в файл** (§2.7): remote-таргет имеет своё
  состояние, превью и все секции рендерятся для таргета, но UI-действия
  «выгрузить в файл» ещё нет. Затирание рабочего конфига при этом
  НЕВОЗМОЖНО by construction: Save визарда пишет только state
  (target-aware store → remote/state.json), а `bin/config.json`
  пересобирается Update-путём строго из локального
  `bin/wizard_states/state.json` (SPEC 045). Remote-конфиг сейчас
  материализуется только в превью — экспорт остаётся следующим шагом.
- **Пресеты фильтруются по платформе ХОСТА** (`loader.go
  filterPresetsByPlatform(runtime.GOOS)`): пресет с `platforms`, не
  включающими ОС машины, где запущен лаунчер, не попадёт в TemplateData —
  и значит и в remote-конфиг для платформы, где он доступен. Сегодня в
  bundled-шаблоне это не проявляется (единственный платформенный preset
  `messengers` покрывает windows+darwin+linux), но при появлении
  linux-only пресета фильтрацию надо переносить с загрузки на
  использование (по TargetSpec). Прекеш `TemplateData.Config` тоже
  строится под ХОСТ — но он используется только как fallback при ошибке
  `GetEffectiveConfigFor` (с warning в Validation); основной путь всегда
  пересчитывает секции из RawConfig под таргет.
- **Дашборд** (`ui/core_dashboard_tab.go`) листает только local-снапшоты
  (плоский `wizard_states/`) — осознанно: remote-снапшоты управляются из
  визарда, изоляция списков — часть дизайна §2.2.
- Транспорт/деплой на удалённую машину (канал lxd) — отдельная спека.

## 7. Ревью-проходка (2026-08-11)

Дефекты, найденные самопроверкой после первичной реализации, все исправлены:

1. `BuildPreviewConfig` не заполнял `BuildContext.Target` и
   `PresetMergeContext.Target` → превью remote-модели рендерило local-конфиг.
   Fix + регрессионный тест `preview_target_test.go`.
2. `buildContextFromState` (Update-путь) — `Target` теперь из
   `TargetSpecFromState(s)`: сегодня всегда local (Update читает только
   local state.json), но конфиг гарантированно консистентен с meta файла.
3. `effectiveTemplate` (business/template_helpers.go) считал секции по
   `runtime.GOOS` → DNS-таб и `EffectiveConfigSection` видели local-версию
   при remote-модели. Переведён на `GetEffectiveConfigFor(model.Target)`.
4. `SwitchConfigTarget` логировал «remote → remote» (ConfigTarget()
   читался после присваивания).
5. `statePathForLog` / `saveStateOnly` собирали путь литералами → лог и
   диалог Save показывали local-путь при записи в remote/. Переведены на
   `platform.GetWizardStatePathFor`.

## 8. Ревью-проходка №2: remote ≠ gateway (2026-08-11)

Пользователь поймал ошибку классификации: `proxy_in_listen` дефолтился в
`0.0.0.0` по «удалённости», хотя критерий — РОЛЬ шлюза. На remote-СЕРВЕРЕ
это был бы открытый прокси (auth по умолчанию выключен). Проверены все
target-ветки шаблона, переклассифицированы две:

- `proxy_in_listen`: `0.0.0.0` ← `@gateway_mode` (было: target=remote);
- `route.find_process`: `false` ← `@gateway_mode` (было: target=remote) —
  у сервера/удалённого mac собственные процессы есть, матчинг осмыслен.

Остались по target (проверено, корректно): `clash_api` (канал управления
lxd), `tun_interface_name` (`lxd-tun0` — имя привязано к демону),
`set_system_proxy` (консервативный дефолт для headless).

Механика, которой это потребовало: `default_value.#if` теперь может
ссылаться на user-vars, объявленные ВЫШЕ по списку vars
(`VarDefaultValue.ForTargetIn`; резолв однопроходный, порядок объявления =
порядок вычисления). Валидатор шаблона обновлён зеркально: backward-ссылка
легальна, forward-ссылка — ошибка загрузки (до SPEC 097 user-vars в
default #if были запрещены целиком). `gateway_mode` перестал позиционироваться
как remote-only: local-gateway легален (mac + Internet Sharing), tooltip
поправлен.
