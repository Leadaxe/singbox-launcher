# SPEC 104 — Направления (Direction): одна модель «правила → направления → узлы» для лаунчера и LxBox

Статус: **ТЗ утверждено к реализации** (2026-08-22). Нормативный список решений —
[`DECISIONS_DIRECTION.md`](DECISIONS_DIRECTION.md) (18 пунктов, далее «D-N»).
Этот документ — техническое задание для исполнителя: что строим, где, какие
ловушки. Чеклист — [`TASKS.md`](TASKS.md).

Первая реализация 104 («каналы» как отдельная сущность `corestate.Channel`)
признана дублем существующего `OutboundConfig` и **сносится целиком** (§8).
Релизов с ней не было → миграции `channels[]` не нужны, ключ просто
игнорируется.

---

## 0. Термины

| Термин | Значение |
|---|---|
| **Direction / Направление** | Именованная точка выбора, на которую ссылаются правила. Материализуется в `selector` (+ парный `urltest`). Заменяет «глобальный outbound» лаунчера и «канал» LxBox. |
| **Тег** | Системный идентификатор направления (`vpn-1`, `proxy-out`, `ru VPN 🇷🇺`). Цель правил. Не меняется после создания. |
| **Имя (label)** | Отображаемое имя («VPN ①», «Моя Германия»). Правится свободно. |
| **Auto-двойник** | `<tag>-auto` — `urltest` по узлам направления. **Только опция внутри своего направления** (D-9, вариант А): не цель правил, не опция других направлений. |
| **Служебные группы подписки** | `AL:select`, `AL:auto` и прочие локальные группы из `sources[].outbounds[]`. Не направления (D-13). |
| **Слоёный пирог** | `шаблон → патчи пресетов → патч пользователя` (`Ref`/`Updates`, SPEC 057/058). Работает для направлений как сейчас для outbound'ов (D-10). |

Словарь UI: EN **Direction / Directions**, RU **Направление / Направления**.
Термины «Channel/Канал» и «Outbound» из пользовательских строк уходят
(в коде `outbound` остаётся там, где это термин sing-box).

---

## 1. Что делаем одной фразой

`configtypes.OutboundConfig` переименовывается в `configtypes.Direction`,
получает недостающие поля (имя, выключение, auto-двойник), его редактор
превращается в форму направления, вкладка Outbounds становится списком
направлений, общий JSON-редактор уходит. LxBox выравнивается (пресеты,
опции из других направлений, бэкап). Контракт получает схему и корпус.

---

## 2. Модель данных (лаунчер, Go)

### 2.1 Тип

`core/config/configtypes/types.go:171-202` — `OutboundConfig` → **`Direction`**
(полное переименование по модулю, без алиаса; `OutboundUpdate` → `DirectionUpdate`,
`RefTemplate`/`RefUser` без изменений).

```go
type Direction struct {
    // как сейчас
    Tag, Type string; Options, Filters, PreferredDefault map[string]interface{}
    AddOutbounds []string; Comment string; Required bool
    Ref string; Updates []DirectionUpdate

    // новое
    Label    string         `json:"label,omitempty"`    // имя; пусто → показываем Tag
    Disabled bool           `json:"disabled,omitempty"` // zero = включено (намеренно НЕ Enabled — см. ловушку T1)
    Auto     *DirectionAuto `json:"auto,omitempty"`     // nil = двойника нет
}

type DirectionAuto struct {
    Mode                      string   `json:"mode,omitempty"`           // "" | "least_test" | "round_robin"
    URL                       string   `json:"url,omitempty"`
    Interval                  string   `json:"interval,omitempty"`
    Tolerance                 int      `json:"tolerance,omitempty"`
    IdleTimeout               string   `json:"idle_timeout,omitempty"`
    InterruptExistConnections bool     `json:"interrupt_exist_connections,omitempty"`
    Pool                      int      `json:"pool,omitempty"`           // только round_robin
    PoolTolerance             int      `json:"pool_tolerance,omitempty"` // только round_robin
    StickyHash                []string `json:"sticky_hash,omitempty"`    // только round_robin; пусто = дефолт ядра
}
```

`Type` остаётся: `selector` (направление) | `urltest` (легаси-самостоятельная
auto-группа шаблона — `auto-proxy-out`; D-12, не мигрируем). У направления
`Type=="selector"` или пусто.

### 2.2 Состав и default — сахар поверх `filters`/`preferredDefault`

Хранилище фильтра **не меняется**: `filters.tag` в существующем языке
паттернов (`core/config/configtypes/matcher.go:28` — `/re/i`, `!/re/i`).
Форма показывает только **тело** регулярки и галку «инвертировать» (D-5, D-6):

| форма | хранится |
|---|---|
| тело `🇩🇪\|🇳🇱`, инверсия выкл | `filters: {"tag": "/🇩🇪\|🇳🇱/i"}` |
| тело `🇷🇺`, инверсия вкл | `filters: {"tag": "!/🇷🇺/i"}` |
| пусто | ключ `tag` отсутствует |
| default-тело `🇳🇱` | `preferredDefault: {"tag": "/🇳🇱/i"}` |

Форма **всегда** пишет флаг `i` (регистр не учитывается, D-6). Чтение:
`/x/` без `i` или литерал → в форме показываем как тело с тем же текстом,
при сохранении перепишется в `/x/i` — by design. Если в `filters` есть другие
ключи (`host`, `scheme`…) — форма показывает `tag`, остальное остаётся только
в JSON-вкладке и сохраняется нетронутым.

Хелперы (новый файл `core/config/configtypes/direction_filter.go`, тесты
рядом): `DirectionFilterBody(filters) (body string, invert bool, ok bool)`,
`DirectionFilterPattern(body string, invert bool) string`,
`DirectionDefaultBody`/`DirectionDefaultPattern`. Эмодзи в теле — проверено,
Go `regexp` с UTF-8 работает; тест с 🇷🇺 обязателен.

Пресеты продолжают патчить `filters.tag` как сейчас (`bin/wizard_template.json:1129-1141`,
`russian`) — ничего не ломается.

### 2.3 Семантика полей направления

| поле | смысл | откуда в LxBox |
|---|---|---|
| `tag` | цель правил; задаётся при создании, дальше не правится | `tag` |
| `label` | имя | `label` |
| `disabled` | не материализуется, не предлагается целью правил; правила на него → dangling-cleanup как сейчас | `!enabled` |
| `filters.tag` | состав (§2.2) | `node_filter` + `node_filter_invert` |
| `preferredDefault.tag` | default = первый совпавший (§2.2) | `default_filter` |
| `addOutbounds` | опции: `direct-out`, `block-out`, **направления выше по списку** (D-8) и легаси-группы шаблона выше по списку. **Не** `<x>-auto` чужих направлений (D-9) | `include_direct`, `include_block`, новое `include[]` |
| `auto` | двойник `<tag>-auto` (§3.2) | `auto{}` |
| `options.interrupt_exist_connections` и прочие | как сейчас | `interrupt_exist_connections` |
| `comment`, `required`, `ref`, `updates` | как сейчас | `ref`/`updates` — переносятся в LxBox (§6) |

Отображаемое имя: `Label`, иначе `Tag` (функция `Direction.DisplayName()`).
`Comment` именем **не** считается (у шаблонных это длинные описания).

### 2.4 Хранение (state.json v6, совместимое расширение)

`core/state/connections.go:13-17`:

```go
type ConnectionsSection struct {
    Sources    []Source   `json:"sources"`
    Outbounds  []configtypes.Direction `json:"direction_outbounds"`        // канонический ключ (D-1)
    LegacyOutbounds []configtypes.Direction `json:"outbounds,omitempty"`  // ТОЛЬКО чтение
    Defaults   Defaults   `json:"defaults"`
}
```

- Имя Go-поля `Outbounds` сохраняем (≈40 ссылок `Connections.Outbounds` по
  коду), меняется только json-тег.
- Load (`core/state/load_v6.go:59`, после parse): если `Outbounds == nil &&
  LegacyOutbounds != nil` → перенести, `LegacyOutbounds = nil`. Старые v2–v4
  загрузчики (`load_v2_v3_v4.go:111`, `legacy_v4.go:29`) не трогаем — они
  пишут в свои структуры и потом в `Connections.Outbounds`.
- Save (`core/state/save.go:117,136`): `LegacyOutbounds` обнулить перед
  marshal; `direction_outbounds` всегда писать (пустой `[]`, не null — как
  сейчас для `outbounds`).
- `channels[]` (`disk_v6.go:66`, `load_v6.go:64`, `save.go:127`, `state.go:123`)
  — **удалить**. Неизвестный ключ в старом state.json игнорируется штатно.
- Per-source `sources[].outbounds[]` (`connections.go:53`) — ключ **не
  меняем** (служебные группы, D-13), тип становится `[]configtypes.Direction`
  чисто по факту переименования.
- Шаблон `parser_config.outbounds[]` (`bin/wizard_template.json:5`) — **не
  меняем** (D-12; это язык шаблона, общий с LxBox по духу).

> **ОТМЕНЕНО ПОЗЖЕ (контракт 0.9.0).** Отдельного отображаемого имени
> (`label`) у Направления больше нет — имя ровно одно, `tag`. Два имени у
> одной сущности означали, что в списке Направлений видно одно, а в
> выпадашке целей правил — другое, и связать их пользователь не мог. Ниже
> по тексту `Label` / `DefaultDirectionLabel` / поле формы «Name» —
> историческая запись исходного замысла, а не текущее поведение: поле
> снято из формы, из модели, из шаблона и из `direction.schema.json`
> (там же — причина и запрет возвращать). Переименование Направления =
> смена `tag` вместе со всеми ссылками на него.

### 2.5 Создание нового направления

Тег по умолчанию — первый свободный `vpn-N` среди тегов вида `^vpn-(\d+)$`
(логика «первая дыра, не max+1» — перенести из `core/state/channel_seed.go`
`NextChannelTag` в `configtypes` как `NextDirectionTag(tags []string) string`);
имя по умолчанию `VPN ⓝ` (`DefaultDirectionLabel(n)`: `"VPN " + U+2460+n-1`
для 1..10, иначе `VPN N` — зеркало `defaultChannelLabel` в
`LxBox/app/lib/models/channel.dart:22-25`). Потолка нет (D-4). Тег на форме
Add **редактируемый** (пользователь может назвать иначе), на Edit —
заблокирован.

---

## 3. Материализация (лаунчер, build)

Глобальные направления сегодня эмитятся через legacy-вид
`ParserConfig.ParserConfig.Outbounds` → `GenerateOutboundsFromParserConfig`
(`core/config/outbound_generator.go`, 3 прохода: info → validity/Kahn →
`GenerateSelectorWithFilteredAddOutbounds` :506). **Переиспользуем этот
конвейер**, а не пишем второй (первая реализация `core/build/channels.go`
сносится; её частности переезжают сюда).

### 3.1 Выключенные
`Disabled` направления выкидываются до прохода 1 (и из `GetAvailableOutbounds`).
Правила на них чистит существующий `CleanDanglingOutboundsInRouteRules`.

### 3.2 Auto-двойник
Новый проход 0 `expandDirectionTwins(globals []Direction) []Direction`
(build-only, **не персистится**): для каждого направления с `Auto != nil`
добавляет перед ним запись `<tag>-auto`:

- `Type: "urltest"`, `Filters` = те же (`filters.tag`), `AddOutbounds` —
  пусто, `Options` = дефолты из шаблона `group_templates.auto.options`
  (`bin/wizard_template.json:1803-1811`: `url/interval/tolerance` как
  `@urltest_*` — подстановку делает движок, класть как есть) **поверх** —
  непустые поля `Auto` (`url/interval/tolerance/idle_timeout/interrupt_exist_connections`);
  `Mode=="round_robin"` → `mode` + `balancer{pool,pool_tolerance,sticky_hash}`
  в том же виде, что SPEC 088 уже пишет для `type_loadbalance`
  (`edit_dialog.go:516-518` — там же посмотреть sentinel `["none"]` для
  пустого sticky_hash: пустой список ядро схлопывает в дефолт).
- Поле `TwinOf string \`json:"-"\`` = тег родителя. Генератор, видя
  `TwinOf != ""`, **не** добавляет в кандидаты exposed-группы подписок
  (`outbound_validity.go:82,112`, `outbound_generator.go:579` —
  `ExposeTagSyntheticNode`): urltest внутри urltest мерил бы группу, а не
  сервер (частность LxBox §322).
- Родитель: `AddOutbounds = ["<tag>-auto", ...остальные]` (двойник первым);
  `options.default = "<tag>-auto"` **только если** `preferredDefault` не
  совпал ни с одним узлом **и** двойник валиден (прошёл Kahn, isValid) —
  иначе sing-box падает на `default` вне `outbounds` (T5). Реализовать в
  `GenerateSelectorWithFilteredAddOutbounds` рядом с обработкой
  `preferredDefaultMap` (:607).
- Двойник без узлов → `isValid=false` → пропускается как сейчас; ссылка из
  `addOutbounds` родителя отфильтровывается существующей логикой
  («FilteredAddOutbounds»).
- `<tag>-auto` **не** попадает в `GetAvailableOutbounds` и в список опций
  других направлений (D-9А). Легаси `auto-proxy-out` (самостоятельная
  запись) — попадает, как сейчас.

### 3.3 Пустое направление → `[block-out, direct-out]`, default=block
Сейчас глобальный selector без членов **пропускается** (`outbound_validity.go:327-329`)
и правила на него уходят в `route.final`. Перенимаем LxBox (D-17): для
**глобальных** записей типа selector при `isValid=false` эмитим
`{"outbounds":["block-out","direct-out"],"default":"block-out"}` + warning.
Теги block/direct — из `group_templates.magic_nodes.{block,direct}.tag`
(`bin/wizard_template.json:1786-1796`), фолбэк `block-out`/`direct-out`.
Per-source локальные группы — поведение не меняем (пропуск).

### 3.4 Предупреждение «фильтр не поймал ни одного узла»
Только когда виноват фильтр: `filters.tag` непуст, узлы в пуле есть, совпадений 0
(LxBox §200/§274, тест `TestNoWarningWhenNoNodesAtAll` из старого
`channels_test.go` переносится). Текст называет фактический исход («traffic is
blocked (default)» / «goes direct» если первым стоит direct-out). Канал
доставки — `debuglog.WarnLog` + статус Preview (первая реализация собирала
`Warnings`, но `build.go:283` их **терял** — T6).

### 3.5 Битая регулярка = пустой фильтр
`MatchesPattern` на невалидном regex возвращает false (`matcher.go:26-27`) —
направление становится пустым. LxBox: битый regex = все узлы. Делать **на
уровне ключа фильтра**, не паттерна: в `filterNodesForSelector`
(`core/config/outbound_filter.go:35`) пред-валидировать паттерны вида `/…/`,
невалидный ключ — отбросить (как отсутствующий) + WarnLog. **Не** трогать
`MatchesPattern`: он общий со skip-фильтрами подписок, и там «битый = всё
совпало» значило бы «выкинуть все узлы» (T7).

### 3.6 Порядок и ссылки вниз
Направления материализуются в порядке списка; опцией можно брать только
стоящие **выше** (D-8) — совпадает с нынешним инвариантом `collectRows`
(`configurator_helpers.go:52-53`) и с Kahn-сортировкой генератора.

### 3.7 Регистр и эмодзи
Форма всегда пишет `/…/i`; `compileFilterRegex(…, ci=true)` → `(?i)`. Тест
на `🇷🇺`, `🇩🇪|🇳🇱`, кириллицу.

---

## 4. UI лаунчера

### 4.1 Вкладка
`ui/configurator/tabs/source_tab.go:761-886` `CreateOutboundsAndParserConfigTab`:
- Название вкладки → «Directions» / «Направления» (где регистрируется —
  ключ `wizard.tab_outbounds` (`internal/locale/en.json:346`, ru аналог), вызов `ui/configurator/configurator.go:351`).
- **Убрать** `ParserConfigEntry` (строки 767-790, 797-803 docButton,
  parserContainer) — D-14. Ловушки при удалении: T8.
- Содержимое: список направлений (`outbounds_configurator.NewConfiguratorContent`)
  + Add + Restore missing (как сейчас). `buildChannelsSection` (:873-874) —
  удалить.
- Служебные группы подписок (per-source строки `collectRows`) **не
  смешивать** с направлениями: показывать ниже, в свёрнутой по умолчанию
  секции «Subscription groups (service)» / «Группы подписок (служебные)».
  Это единственное место их правки — убирать нельзя (моё решение, см. §10).
- Строка списка: `DisplayName()` + серым `(tag)`; бейджи preset/template/✏
  как сейчас; `Disabled` — строка приглушена + переключатель Enable/Disable.

### 4.2 Редактор `ShowEditDialog` (`ui/configurator/outbounds_configurator/edit_dialog.go`)
Дорабатываем, **не переписываем** (D-15):

| элемент | сейчас | становится |
|---|---|---|
| заголовок | Edit/Add Outbound | Edit/Add Direction |
| Tag (:79) | текст | на Add предзаполнен `NextDirectionTag`, правится; на Edit — read-only |
| **Name** | — | новое поле сверху, `Label`, placeholder `DefaultDirectionLabel` |
| Type (:98-114) | manual/auto/loadbalance | оставить **только для записей с `Type=="urltest"`** (легаси-группы). Для направлений — скрыть; вместо него блок Auto (ниже) |
| Comment (:117) | есть | есть, ниже, вторичное |
| Filter (:255-294) | raw `/x/i` + flag-picker 🌐 | **тело** + галка «Invert (exclude matching)»; flag-picker (`flag_picker.go:172-174`) отдаёт тело+инверсию, а не `/…/i`; рядом кнопка «?» → `docs/DIRECTION_FILTERS.md` (новый, EN+RU: синтаксис Go RE2 без флагов, примеры с флагами-эмодзи, `^`, `\|`, что регистр игнорируется) |
| Preferred default (:296-313) | raw | тело |
| Add outbounds (:315-343) | direct-out, reject, все теги | `+direct` (`direct-out`), `+block` (`block-out`; `reject` — убрать из предложений, это action, а не outbound), чекбоксы **направлений выше по списку** и легаси-групп выше; без `*-auto` двойников |
| **Auto twin** | — | галка «Auto-select twin (`<tag>-auto`)»; при вкл: Mode (least_test / round_robin), URL / Interval / Tolerance / Idle timeout / Interrupt — переиспользовать виджеты `urltestBlock` (:654-684) и `balancerBlock` (:665+) |
| Scope (:208-237) | For all / For source | оставить; при открытии из списка направлений — «For all» и read-only; из служебной секции — как сейчас |
| Preview tab | есть | + строка «auto twin: N nodes» |
| JSON tab | есть | есть (D-14: JSON остаётся только здесь) |

Сборка `cfg` из формы (:422-556): добавить `Label`, `Auto`, `Disabled`;
`Filters`/`PreferredDefault` через `DirectionFilterPattern`.

### 4.3 Цели правил
`ui/configurator/business/outbound.go:GetAvailableOutbounds` (:71-176):
убрать блок `ChannelTags` (:164-169); исключить `Disabled`; `<tag>-auto`
двойники не добавляются (их в `AddOutbounds` нет — expand build-only, ✓
само собой). Опционально: в дропдауне показывать `tag — label`.

### 4.4 Локализация
- `wizard.channels.*` (32 ключа, `internal/locale/en.json:1120-1151`,
  `bin/locale/ru.json:1120+`) — удалить.
- `wizard.outbound.*` (57 ключей, `en.json:704-805`) — ключи **оставить**,
  тексты перевести на Direction/Направление; добавить ключи для Name, Invert,
  Auto twin, Mode, Disabled, секции служебных групп, подсказки «?».
- `wizard.outbounds.placeholder|button_docs|error_open_docs|label` — удалить
  вместе с JSON-редактором.
- Никаких Test* на строки UI (память `no-ui-format-tests`).

---

## 5. Пресеты (слоёный пирог) — уже работает, нужно не сломать

`russian` → `ru VPN 🇷🇺` через `mode=add` (`bin/wizard_template.json:1142+`)
синхронизируется `SyncOutboundsWithActivePresets` (`core/build/sync_outbounds.go:49`):
появляется **в конце** списка (D-11 ✓), строка `IsPreset` → Del скрыт, Edit →
USER patch (✓ уже так). Новые поля должны пройти через все слои (T2):

- `core/template/preset_types.go:144-193` `PresetOutbound` — добавить
  `Label`, `Disabled`, `Auto` (зеркало `Direction`).
- `core/build/preset_outbounds.go:194 applyOutboundUpdate` — `Label` (replace iff
  non-empty), `Auto` (replace целиком iff non-nil), `Disabled` (replace).
- `core/build/outbound_diff.go:34 OutboundFieldDiff` — diff по `label`,
  `auto`, `disabled`; иначе правка имени/двойника у шаблонной записи молча
  теряется на Save.
- `core/build/sync_outbounds.go:232 stripReferencedBody` и
  `ui/.../edit_dialog_helpers.go:182 stripDirectBodyForReferenced` — обнулять
  и новые поля (тело referenced-записи живёт в шаблоне/пресете).
- `resolve_outbounds.go:183 applyOutboundUpdatePatch`,
  `sync_outbounds.go:310 outboundConfigToPatchMap` — JSON round-trip, новые
  поля проходят сами; покрыть тестом.
- `core/build/migrate_outbounds_spec058.go` — проверить, что adopt-legacy
  сравнение тел не ломается от новых полей.

---

## 6. LxBox — паритет

Ориентиры: модель `app/lib/models/channel.dart`, материализация
`app/lib/services/builder/build_config.dart:644-800 _buildChannelGroups`,
хранение `app/lib/services/settings_storage/channels.dart`, мутации
`app/lib/services/channel_mutations.dart`, экран `app/lib/screens/channel_edit_screen.dart`,
Debug API `app/lib/services/debug/handlers/channels.dart`.

| # | что | детали |
|---|---|---|
| L1 | Термин | `Channel`→`Direction`, `ChannelAuto`→`DirectionAuto`, файлы `channel*.dart`→`direction*.dart`, `ChannelMutations`→`DirectionMutations`; UI-строки «Channel»→«Direction» (RU «Направление», ключи в `assets/l10n/ru/ui.json` — l10n strict!). **Не трогать**: platform channels, `cc_channel.dart`, log-«channel» — это другое слово. Debug API: команды `channels*` оставить как алиасы, добавить `directions*`. |
| L2 | Хранение | читать `channels[]` (бессрочно) и `direction_outbounds[]`; писать только `direction_outbounds[]` (D-2). Legacy-бэкап allowlist (`lx_backup.dart:26-31` — `channels`, `channels_migrated`) + новый ключ. |
| L3 | Опции из других направлений | новое поле `include: [tags]` (только направления **выше** по списку, D-8); в селектор после узлов, перед `direct/block`; UI — чекбоксы направлений выше. `<x>-auto` чужие — никогда (D-9А). |
| L4 | Пресеты поверх направлений (D-10) | шаблон: `selectable_rules[].outbounds[]` с `mode: add\|update` и полями в каноническом виде (§7.1); хранение: у направления `ref` (`<preset_id>`) и `updates[]` (`{ref, patch}`), как в лаунчере; sync при включении/выключении пресета (add → создать в **конце**, update → патч в стек, выключили → убрать add/патч, USER-патч оставить); направление от пресета: Del скрыт, Edit = USER-патч (D-11). Разложить в `preset_expand.dart` + новый `services/builder/direction_sync.dart`. |
| L5 | Default и место двойника | выровнять с §3.2: двойник **первым** в `outbounds`, `default = <tag>-auto` если `default_filter` не совпал. Сейчас двойник последним и без default (`build_config.dart:712-716`). |
| L6 | Бэкап v1.1 | §7.2 — экспорт `directions[]`, импорт создаёт отсутствующие. |
| L7 | Контракт | раннер `app/test/contract/direction_corpus_test.dart` по `corpus/direction/`. |
| L8 | Документы | `docs/spec/features/393 directions/spec.md` (номер проверить — сейчас максимум 392), `CHANGELOG.md`, `AGENTS.md` §Контракт. |

Фильтр-семантика (тело regex, `caseSensitive:false`, инверсия, битый = все
узлы, пустое → `[block, direct]` default=block, предупреждение только когда
виноват фильтр) — в LxBox уже так; **не менять**, это эталон для Go-стороны.

---

## 7. Контракт (`contract/`)

### 7.1 Каноническая форма направления — `contract/schema/direction.schema.json`
```json
{ "tag": "vpn-1", "label": "VPN ①", "enabled": true,
  "filter": "🇩🇪|🇳🇱", "invert": false, "default": "🇳🇱",
  "include_direct": false, "include_block": false, "include": ["vpn-1"],
  "interrupt_exist_connections": true,
  "auto": { "mode": "least_test", "url": "…", "interval": "15m", "tolerance": 50,
            "idle_timeout": "30m", "interrupt_exist_connections": false,
            "pool": 3, "pool_tolerance": 0, "sticky_hash": ["process","domain"] } }
```
Маппинг в `contract/docs/DIRECTION.md` (таблица §2.3 + правила §3.2–3.5
как нормативный текст; Go-сторона: `filter`↔тело `filters.tag`,
`include_direct/block/include[]`↔`addOutbounds`, `enabled`↔`!disabled`).

### 7.2 Корпус `contract/corpus/direction/`
`<case>.direction.json` = `{directions:[канон], node_tags:[], group_tags:[],
magic:{direct,block}}` → `<case>.expected.json` = `{groups:[{tag,type,outbounds,default?,…}], warnings:[коды]}`.
Обязательные кейсы: без фильтра; фильтр; инверсия; эмодзи; битый regex;
пустое → block/direct; default-regex; default=auto; auto без узлов (двойник
не эмитится, default не ставится); узел-группа не идёт в двойник; include
направления выше; disabled пропущено; round_robin balancer; предупреждение
только когда виноват фильтр. Раннеры: Go `core/config/contract_direction_test.go`
(синтетические `ParsedNode` из `node_tags` через `ExposeTagSyntheticNode`,
прогон через `GenerateOutboundsFromParserConfig`), Dart — L7.
`contract/VERSION` → `0.3.0`; `tool/sync_contract.sh`, `contract.lock`
(LxBox CI `check_contract_lock.dart`) перегенерировать.

### 7.3 LX Backup v1.1 (аддитивно внутри мажора 1, `BACKUP.md §6`)
Новый необязательный корневой ключ `directions[]` (канон §7.1, только
пользовательские и merged-тела preset/template-записей **без** `ref`).
Импорт: тег отсутствует → создать direct-запись в конце; тег есть → не
трогать + warning `backup_direction_exists`; `rules[].outbound`/`route.final`
проверяются по `KnownOutbounds` **после** создания → правила приходят
включёнными (D-16, снимает политику `BACKUP.md §3`). Обновить
`contract/schema/backup.schema.json`, `contract/docs/BACKUP.md`,
`core/backup/{types,export,import}.go` + Dart `lx_backup.dart`, фикстуры
`corpus/backup/` (новый кейс `directions_created_on_import`).

---

## 8. Снос первой реализации (один коммит, до всего остального)

Удалить:
`core/state/channel_types.go`, `core/state/channel_seed.go`,
`core/state/channel_test.go`, `core/template/channel_defaults.go`,
`core/build/channels.go`, `core/build/channels_test.go`,
`core/build/zz_seed_probe_test.go`, `ui/configurator/tabs/channels_section.go`.

Вычистить ссылки: `core/state/{disk_v6.go:66, load_v6.go:64, save.go:127, state.go:113-123}`,
`core/build/build.go:101-105,280-285`, `core/config_service_context.go:47`,
`ui/configurator/models/{wizard_state_file.go:42-43,197-201, wizard_model.go:71}`,
`ui/configurator/business/{create_config.go:81, outbound.go:164-169}`,
`ui/configurator/presentation/{presenter_state.go:97,311, presenter_state_helpers.go:244-275}`,
`ui/configurator/tabs/source_tab.go:871-875`.

Шаблон `bin/wizard_template.json`: `default_channels` (:1814-1825) — удалить
(лаунчер сидируется из `parser_config.outbounds`, D-12); `group_templates`
(:1780-1813) — **оставить**: даёт дефолты двойника (§3.2) и magic-теги (§3.3).
Ключ `group_templates.channel` не переименовываем — язык шаблона общий с LxBox
(`TEMPLATE_LANG.md §8.4`), в доке пометить «исторический ключ = шаблон направления».

Локали: `wizard.channels.*` EN/RU. Документы (§9) — переписать, не «дополнить».
Память: в `/Applications/...` лежит шаблон с `default_channels` — после
удаления ключа из репо он там безвреден (неизвестный ключ), трогать руками не
надо; бинарь ядра не трогать (`core-binary-hands-off`).

---

## 9. Документация

| файл | что |
|---|---|
| `docs/WIZARD_STATE{,.ru}.md` §3.7 | «channels[i]» → «connections.direction_outbounds[i] — Directions»: таблица §2.3, чтение `outbounds` legacy, §2.4 |
| `docs/TEMPLATE_REFERENCE{,.ru}.md` §4.6 | `group_templates` — что читает лаунчер; `default_channels` — mobile-only |
| `docs/ARCHITECTURE_PACKAGES{,.ru}.md` :78-80,120,219 | убрать channel-файлы, описать `direction_filter.go`, проход 0 в генераторе |
| `docs/ParserConfig.md` §«outbounds» | новые поля `label/disabled/auto`, примечание что общий JSON-редактор убран, ссылка на DIRECTION_FILTERS |
| `docs/DIRECTION_FILTERS{,.ru}.md` | новый — справка из кнопки «?» |
| `contract/docs/{DIRECTION,BACKUP,TEMPLATE_LANG §8.4}.md`, `contract/README.md`, `corpus/README.md` | §7 |
| `docs/release_notes/upcoming.md` :16,:45 | абзацы про «каналы» переписать на «Направления» (EN+RU); релиза с каналами не было — не описывать как изменение |
| `SPECS/CONSTITUTION.md` §7.3, `SPECS/README.md` | термин |
| LxBox: `docs/spec/features/393 directions/spec.md`, `CHANGELOG.md`, `AGENTS.md` | L8 |

---

## 10. Решения этого ТЗ сверх DECISIONS (пользователь может наложить вето)

| # | решение | почему |
|---|---|---|
| S1 | Фильтр хранится как сахар над `filters.tag` (§2.2), а не новыми полями | ноль изменений в генераторе, пресетах, превью, валидности; `russian` уже патчит именно `filters.tag` |
| S2 | `Disabled` (zero=вкл), а не `Enabled` | иначе нужен `UnmarshalJSON` с дефолтом true — ловушка, на которой первая реализация уже спотыкалась |
| S3 | Auto-двойник первым и default'ом, если default-regex не совпал; LxBox выравнивается (L5) | так устроен `proxy-out` в шаблоне лаунчера (`default: auto-proxy-out`); смысл двойника — чтобы им пользовались |
| S4 | Пустое направление → `[block-out, direct-out]` default=block вместо пропуска (§3.3) | безопаснее: сейчас трафик молча уходит в `route.final` |
| S5 | Служебные группы подписок — свёрнутая секция под списком направлений (§4.1) | иначе пропадает единственный UI их правки; в список направлений не смешиваются |
| S6 | Имя = `Label`, `Comment` остаётся отдельным | у шаблонных записей comment — описание абзацем |
| S7 | Тег правится только на Add | цель правил; LxBox тег immutable |
| S8 | `reject` убран из предложений опций состава | это action sing-box, не outbound (`outboundSentinelLiterals`) |
| S9 | `direction_outbounds` внутри `connections`, не в корне state | секция подключений остаётся цельной; имя поля в Go не меняется |

---

## 11. Ловушки (T)

| # | где | что |
|---|---|---|
| T1 | `Direction.Disabled` | не заводить `Enabled bool`: нулевое значение выключит всё при чтении старых записей |
| T2 | §5, 6 файлов | новое поле, не проведённое через `applyOutboundUpdate`/`OutboundFieldDiff`/`stripReferencedBody`/`PresetOutbound`, молча теряется при Save шаблонной/пресетной записи |
| T3 | `gofmt -r`/sed переименование | `OutboundConfig` встречается в строках комментариев docs и в `core/config/models.go:10` алиасе — алиас удалить, комментарии поправить; `internal/daemonpb` не трогать |
| T4 | Win7 `go1.20` | никаких `min/max/slices/maps/PathValue` в не-test коде (память `win7-build-go120`) |
| T5 | §3.2 default=twin | ставить только после validity двойника; sing-box падает на `default` вне `outbounds` |
| T6 | §3.4 | первая реализация собирала `Warnings`, `build.go:283` их выбрасывал — проверить, что новые предупреждения доходят хотя бы до `WarnLog` |
| T7 | §3.5 | «битый regex = все узлы» только для фильтра направления, не в `MatchesPattern` (общий со skip-фильтрами) |
| T8 | удаление `ParserConfigEntry` | `presenter_async.go:39` — `TriggerParseForPreview` возвращается, если `ParserConfigEntry == nil` → **Preview перестанет парсить**; убрать условие. Остальные обращения nil-гардированы (`presenter_sync.go:107,383,464,535`, `presenter_ui_updater.go:38`) — оставить как есть. `guiState.ParserConfigEntry/ParserConfigUpdating` (`gui_state.go:51,111`) — удалить вместе с гардами или оставить поля nil — на выбор, но проверить `go vet` |
| T9 | `configurator.go:137,267` `ShowEditDialog` callback | scope по-прежнему нужен для служебной секции; при открытии из направлений `scopeKind=="global"` |
| T10 | `GetAvailableOutbounds` memo (`outbound.go:87-95`) | кэш по `ParserConfigJSON` — `Disabled`/`Label` меняются через ту же JSON-строку, ok; но preset-теги в обход мемо (:141-145) — не сломать |
| T11 | `InvalidateTemplateIfStale` (`core/template_migration.go:42`) | dev-сборка `-dirty`/`unnamed-dev` пропускает стейл-чек; релизная метка удалит шаблон один раз — при ручной установке шаблон копировать **после** первого запуска новой версии или ставить маркер |
| T12 | Живой лаунчер | не перезапускать без согласования; `bin/sing-box` не трогать; интернет пользователя идёт через VPN лаунчера |
| T13 | LxBox l10n strict | новая строка без ключа в `assets/l10n/ru/ui.json` валит тест; `.s` vs `.plural` различать |
| T14 | `contract.lock` | изменение `contract/` без `tool/sync_contract.sh` валит CI LxBox |
| T15 | `hasReferencedOutbounds` (`save.go:44,159`) | делает бэкап state перед первой записью referenced-записей — не должен сработать повторно от смены json-ключа; проверить тестом round-trip save→load |

---

## 12. Definition of Done

- Лаунчер: `go build ./... && go vet ./... && go test ./...` зелёные; линт чист;
  `go.win7.mod`-сборка проходит (grep на запрещённые API).
- LxBox: `flutter test` (в т.ч. l10n/docs-проверки, contract lock) зелёные.
- Контракт: корпус `direction/` проходит с обеих сторон, `VERSION` 0.3.0, lock обновлён.
- Документы §9 обновлены; `upcoming.md` описывает Направления.
- Живая проверка за пользователем: сборка `./build/build_darwin.sh -i`, шаблон/локаль
  скопировать в бандл руками; перезапуск лаунчера — по его команде.
