# SPEC 122 · Tailscale: endpoint-узел, конструктор, гейт ядра

Статус: N (черновик; реализуется после приёмки SPEC 121). Ветка develop.
Зависит от SPEC 121 «Секции узла» — связка DNS + маршрут едет в `sections`
узла, эта задача её только порождает. Карта кода — `SPECS/121-F-N-NODE_SECTIONS/CODEMAP.md`
(§2 импорт endpoints, §2.3 селекторы, §5 DNS-форма, §8 гейт ядра).

## 1. Проблема

Пользователи просят Tailscale «как в sing-box» (issue). sing-box ≥ 1.12
умеет быть узлом tailnet сам: endpoint типа `tailscale` (tsnet в
user-space), DNS-сервер типа `tailscale` для MagicDNS и обычные правила
маршрута на `100.64.0.0/10`. Лаунчер сегодня:

- отвергает тип `tailscale` при импорте sing-box JSON (нет в
  `singboxSchemeByType`, `core/config/subscription/singbox_import.go:344-359`)
  и дополнительно требует `server`/`server_port` (`singboxTypeIsAddressless`
  `:371-373` знает только `wireguard`);
- считает endpoint'ом только `wireguard` (`core/config/outbound_generator.go:1086`,
  `:1054-1056`) — принятый `tailscale` уехал бы в `outbounds[]`;
- предложил бы такой узел в Направления как выход в интернет
  (`core/config/outbound_filter.go:41-64`), хотя без `exit_node` он выходом
  не является;
- не знает тип `tailscale` в форме DNS-сервера (`ui/configurator/tabs/dns_server_form.go:37-44`);
- ядро в бандле (lx.27) собрано без `with_tailscale`; форк с lx.31
  (2026-09-04) тег включает для desktop и роутеров. Конфиг с
  `tailscale` на старом ядре валит `sing-box check` целиком.

## 2. Что делаем

### 2.1 Импорт и эмиссия (core)

- `tailscale` → схема `tailscale` в `singboxSchemeByType` (единственная
  таблица, SPEC 118 W4). Безадресные типы: `{wireguard, tailscale}`.
- Признак endpoint'а — по реестру, а не по строке: `contract/registry/protocols/tailscale.json`
  (`kind: endpoint`, `singbox_type: tailscale`, `sources: [singbox]`,
  без URI-формы), и гейт в `EmitNodeJSONs`/`GenerateEndpointJSONBare`
  проверяет `kind == endpoint` по схеме через реестр (`wireguard` и
  `tailscale`; `masque` остаётся как есть — отдельное решение). Sync-тест
  реестра (`core/config/subscription/registry_sync_test.go`) знает новый файл.
- Эмиссия тела — `EmitBody` как есть (канон v7); per-scheme ветки не нужно.
  `state_directory`: если в теле нет — эмиттер подставляет путь по тому же
  правилу, что локальные `.srs` (`bin/…`), вида `tailscale/<финальный тег>`,
  чтобы два узла не сели в один каталог (дефолт ядра — общий `tailscale`).
- Warning-код `tailscale_core_unsupported` в `contract/registry/warnings.json`.

### 2.2 Гейт ядра

По образцу naive (CODEMAP §8): проба `CoreSupportsTailscale()` по строке
`Tags:` (`core/core_capabilities.go:84`, `hasTag("with_tailscale")`), хук
`config.TailscaleSupportProbe` ставится в `core/controller.go:255-256`,
на эмиссии узел `tailscale` без поддержки ядра **выбрасывается с warning**
(`outbound_generator.go:1175-1225` — та же ветка, что naive), вид отчёта
`BuildReportTailscaleDegraded`, текст: `sing-box core is built without
with_tailscale (need ≥ 1.14.0-lx.31)`. Политика «не деградировать по
догадке» (`core_capabilities.go:91`) сохраняется: неразборчивый вывод =
поддерживается. Секции узла (SPEC 121) при выброшенном узле не эмитятся
сами собой — узел не доходит до кэша.

### 2.3 Направления

Узел `tailscale` без `exit_node` — не выход в интернет: исключается из
пула кандидатов Направлений в обеих точках (CODEMAP §10 п.25:
`core/config/outbound_filter.go:52-60` и `ui/configurator/business/node_pool.go:167`)
одним предикатом на модели (`CanonicalNode`/`ParsedNode`), например
`IsExitCapable()`: `tailscale` с непустым `exit_node` → кандидат, иначе нет.
Detour на такой узел не запрещается (пользователь может осознанно гнать
трафик через tailnet).

### 2.4 DNS-форма

`dns_server_form.go`: тип `tailscale` в списке (`:44`), строки формы —
`tag` и `endpoint` (Select по тегам узлов `tailscale` в пуле модели; при
пустом пуле — текстовое поле). `accept_default_resolvers` — чекбокс.
Остальные строки (server/port/path/detour/resolver) скрыты, как у `group`
(`:211-246`). `Load` перестаёт возвращать `false` для `tailscale`.

### 2.5 Конструктор «Tailscale» в «Add server»

`ui/configurator/dialogs/add_server_dialog.go`: в `proto`-Select
(`:261`) появляется пункт `Tailscale`. Форма:

| Поле | Тип | В тело |
|---|---|---|
| Tag | text (обязателен, дефолт `tailscale`) | тег узла |
| Auth key | password entry (обязателен, `tskey-…`) | `auth_key` |
| Control URL | text (пусто = официальный) | `control_url` |
| Hostname | text (пусто = имя машины) | `hostname` |
| Ephemeral | check | `ephemeral` |
| Accept routes | check | `accept_routes` |
| Exit node | text (пусто = нет) | `exit_node` |

Подсказка под ключом: «One-off keys are consumed at first login; the node
identity then lives in the state directory. Deleting the profile registers
a new device — use a reusable key for that.»

Форма отдаёт **документ** SPEC 121 §5.1 в `AddServerResult.ConfigJSON`:

```json
{ "endpoints": [ { "type": "tailscale", "tag": "<tag>", "auth_key": "…", … } ],
  "dns":   { "servers": [ { "type": "tailscale", "tag": "ts-dns", "endpoint": "@self" } ],
             "rules":   [ { "domain_suffix": [".ts.net"], "server": "ts-dns" } ] },
  "route": { "rules":   [ { "ip_cidr": ["100.64.0.0/10"], "outbound": "@self" } ] } }
```

Путь записи — тот же `AppendManualConfigJSON`/`carveSingboxJSON`, который
после SPEC 121 понимает документ; своего пути записи форма не заводит
(ловушка emitter-parser-pairing). Вкладка JSON диалога показывает
документ; ручная правка побеждает, как сегодня. Новых глифов и иконок
нет.

### 2.6 Секрет

`auth_key` — секрет в теле узла. По решению «секреты в state — by design»
маскируется только в UI (password entry, маска в подписи узла не нужна —
тег ключа не содержит); в бэкап уезжает открыто, как ключи WG. В логи тела
узлов не пишутся — политика действующая, новых мест логирования не
добавлять.

### 2.7 Каталог состояния: жизненный цикл

Дополнение к §2.1, согласовано с LxBox 14.09.2026. Реализация — `core/config/tailscale_state_dir.go`
(файловые операции и ожидаемый набор) + `ui/configurator/business/tailscale_state_dir.go`
(финальный тег по модели и точки вызова).

`state_directory` — не кэш: в нём `tailscaled.state` с ключом устройства.
Одноразовый ключ сгорает на первом логине, поэтому потеря каталога значит
«зарегистрируй устройство заново» вручную, а ЧУЖОЙ каталог под тем же именем
значит молча подхваченную чужую идентичность. Отсюда четыре нормы.

**1. Удаление узла → удаление каталога.** Удалили узел Tailscale (корневой или
члена папки; удалили папку вместе с составом) — каталог `<root>/<имя>`
(корень = `<execDir>/bin/tailscale`, `config.TailscaleStateDirRoot()`) сносится
`os.RemoveAll`, где `<имя>` = `sanitizeStateDirName(финальный тег на момент
удаления)`. Ошибка удаления — `WarnLog`, операция UI не блокируется:
узел в модели уже удалён, и откатывать это из-за занятого файла значило бы
показать «узел не удалился» там, где он удалился.

**2. Смена финального тега → переименование каталога.** Переименование
`Node.Tag`, перенос в папку / из папки, смена `tag_policy` контейнера — каталог
переезжает со старого имени на новое (`os.Rename`), если старый существует и
нового нет. Идентичность устройства сохраняется. Если новое имя ЗАНЯТО — там
чужое состояние; оставляем оба и `WarnLog`, затирать чужую идентичность
переименованием нельзя.

Почему не «дождаться GC»: для GC переименование неотличимо от «старый узел
удалили» — каталог со старым именем стал бы сиротой и уехал в снос вместе с
ключом. Переезд обязан произойти В МОМЕНТ правки.

**3. GC на сборке.** После ПОЛНОЙ эмиссии (`GenerateOutboundsFromParserConfig`,
там же, где отработал `applyTailscaleStateDirectory`) каталоги под `<root>`,
чьё имя не входит в ожидаемый набор, удаляются — по одному
`InfoLog` на каталог.

Ожидаемый набор = имена по ВСЕМ ХРАНИМЫМ узлам Tailscale состояния, корневым и
членам папок, **включая выключенные и снятые гейтом ядра** (узел, выключенный
на месяц, идентичность терять не обязан):

- узел, ДОШЕДШИЙ до эмиссии — `sanitize(финальный тег ПОСЛЕ уникализации)`:
  суффикс (`ts-2`) знает только глобальный счётчик сборки;
- узел, НЕ дошедший (выключен / снят гейтом) — `sanitize(префикс контейнера +
  сырой тег + постфикс)` без суффикса уникализации и без раскрытия переменных
  тег-политики: порядка эмиссии у него нет. Перекос осознанный и в сторону
  СОХРАНЕНИЯ — лишнее имя в наборе оставляет каталог жить, недостающее сносит.

Набор не построился (состояние не читается, `parserConfig == nil`) — GC
**пропускается целиком** с `WarnLog`. Снести по пустому набору значит снести всё,
а «данных нет» и «узлов нет» — разные вещи (ловушка ленивого кэша).

Защита пути: удаление и переименование идут только по имени в ОДИН сегмент и
только после `filepath.Clean`, с проверкой, что итог лежит строго внутри корня.
Имя приходит из тега, то есть из чужой подписки, и `os.RemoveAll` по
неочищенному пути был бы удалением произвольного каталога машины по строке из
стороннего JSON.

**4. В бэкап каталог не едет.** Свойство держится тем, что `state_directory`
штампует ТОЛЬКО config-форма `GenerateEndpointJSON`; `GenerateEndpointJSONBare`
(канон, подпись содержимого, экспорт) его не подставляет — §2.1. Закреплено
тестом `core/backup/tailscale_state_dir_test.go`, проверяющим оба конца: голую
эмиссию и байты файлов 1.0 и 0.12.

**Каталог с явным `state_directory` в теле узла** (задал пользователь) не
участвует ни в одной из норм: он лежит вне `<root>`, лаунчер его не переносит,
не сносит и в ожидаемый набор не кладёт.

**Точки вызова.** Удаление: `ui/configurator/tabs/preview_node_ops.go`
(`applyDelete` — узел контейнера), `ui/configurator/tabs/source_tab.go`
(`showSourceRowDeleteDialog` — строка целиком; `showFolderDeleteDialog`,
кнопка «Delete with nodes»). Переименование: `preview_node_ops.go`
(`applyRename`), `ui/configurator/tabs/source_edit_window.go` (Save — узел
контейнера, корневой узел, и смена `tag_policy` в `mergeEditedSourceIntoModel`),
`ui/configurator/business/node_move.go` (`MoveNodeToFolder`, `MoveNodeToRoot`,
`ExtractFolderNodesToRoot`). GC: `core/config/outbound_generator.go`, сразу
после цикла `EmitNodeJSONs`.

**Что эти вызовы НЕ ловят:** импорт бэкапа, fetch подписки, ручную правку
`state.json`. Вешать вызов на каждый такой путь значило бы ловить их вечно; их
добирает GC — ценой перелогина, если состав изменился переименованием мимо UI.

## 3. Что не входит

- Множественные узлы tailnet как особый режим: второй узел = ещё один
  `Add server → Tailscale` с другим тегом; каталоги состояния разведены
  по тегу (§2.1). Правило на `100.64.0.0/10` у второго узла пользователь
  снимает сам в JSON узла.
- `advertise_routes`/`advertise_exit_node`/`system_interface` — не в
  форме; доступны через JSON узла.
- Тег узла как цель чужих правил (`GetAvailableOutbounds`) — отдельно,
  если попросят.
- LxBox: libbox без tailscale (`// lx:no-tailscale`), в контракте только
  реестр протокола; в бэкапе узел едет как любой `config_json` + `sections`.

## 4. Приёмка

1. Вставка JSON из issue (целый конфиг, один endpoint `tailscale`) как
   источника на ядре ≥ lx.31 → узел `tailscale-node` с секциями; конфиг
   проходит `sing-box check`; узел не предлагается в Направления.
2. То же на ядре без тега → узел выброшен с warning `tailscale_core_unsupported`,
   остальной конфиг собран и проходит check; отчёт сборки называет причину.
3. «Add server → Tailscale» с тегом `ts` и ключом → узел с секциями,
   `state_directory` вида `…/tailscale/ts`; правка в JSON диалога побеждает.
4. Форма DNS-сервера открывает сервер типа `tailscale` формой, а не JSON.
5. Узел с `exit_node` появляется в пуле Направлений; без — нет.
6. Тесты (в конце, комплексные): импорт+эмиссия `tailscale` из JSON
   (endpoint, не outbound); гейт по фикстуре `sing-box version` с тегом и
   без (`core/core_capabilities_test.go` уже держит `with_tailscale` в
   фикстуре `:24`); пул Направлений с/без `exit_node`. UI не тестировать.
7. Жизненный цикл каталога (§2.7) — три теста на временном корне
   (`SetTailscaleStateDirRoot`), по одному на сторону:
   `core/config/tailscale_state_dir_test.go` (remove + rename + GC на составе
   «корневой / член папки с префиксом / выключенный», плюс невыход за корень),
   `ui/configurator/business/tailscale_state_dir_test.go` (удаление,
   переименование, перенос в папку и из папки, смена тег-политики),
   `core/backup/tailscale_state_dir_test.go` (норма 4).

## 5. Документы

`README.md`/`README.ru.md` — раздел про Tailscale (что нужно ядро ≥ lx.31);
`docs/release_notes/upcoming.md`; `contract/registry/protocols/tailscale.json`
+ `contract/registry/warnings.json`; `contract/TASKS_LXBOX.md` — строка в
`## 9` (реестр протокола появился, LxBox узел не применяет).

## 6. Что уже дал SPEC 121 (адреса для исполнителя)

- Разбор документа узла: `core/config/node_document.go` —
  `IsNodeDocument(raw)` `:87`, `ParseNodeDocument(raw) (body, *state.NodeSections, err)` `:112`,
  `NodeBodyGoesToEndpoints` `:505`, `RenderNodeDocument` `:520`.
  `AppendManualConfigJSON` (`ui/configurator/business/sources_json.go:130-164`)
  уже принимает документ — форма «Add server → Tailscale» отдаёт документ в
  `AddServerResult.ConfigJSON`, своего пути записи не заводит.
- Извлечение секций из целого конфига: `core/config/subscription/singbox_sections_extract.go`
  (`ExtractNodeSections` `:61`). После добавления схемы `tailscale` JSON из
  issue пройдёт этим путём без доработок.
- Карта кода: `SPECS/121-F-N-NODE_SECTIONS/CODEMAP.md` §2 (импорт и гейт
  endpoint'а), §2.3 (селекторы Направлений), §5.1 (форма DNS-сервера), §8
  (гейт ядра по образцу naive), §11–12 (что добавили волны 1–2).
- Ядро с тегом для ручной проверки `check`: локальная сборка форка lx.31
  лежит вне репозитория, путь сообщается исполнителю отдельно; `bin/sing-box`
  (lx.27, без тега) не заменять.
