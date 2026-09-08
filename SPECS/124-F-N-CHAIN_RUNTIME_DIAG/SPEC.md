# SPEC 124 · Цепочка в рантайме: состояние звена, ошибки по позициям, флаг разрыва соединений

Статус: N (в работе). Ветка develop. Ядро: sing-box-lx `v1.14.0-lx.34` (пин);
тумблер позиций и `ChainCloneState` в `GetChains` есть с lx.28 (SPEC 075 ядра).

Спека ядра: `/Users/macbook/projects/sing-box-lx/SPECS/TASKS/075-*/SPEC.md`.
Опции цепочки: `/Users/macbook/projects/sing-box-lx/option/chain_lx.go:12-30`.
Документация: `/Users/macbook/projects/sing-box-lx/docs-lx/lx-config.ru.md` §chain.

## 1. Проблема

Тумблер позиции (SPEC 110/113, `ui/servers_node_info_chain.go`) работает
против живого ядра, но три вещи из SPEC 075 ядра до UI не доехали:

- **Состояние звена не показывается.** `GetChains` отдаёт у позиции
  `clone.state` (`starting | active | idle`) и `clone.last_error`; лаунчер
  кладёт их в `core.ChainPositionInfo.CloneState/LastError`
  (`core/chain_probe.go:52-58`, `core/services/lxd_remote_transport.go:357-364`),
  и на этом всё — UI поля не читает. Пользователь не видит, что выключенное
  звено ещё держит соединения (ядро не рвёт его принудительно, забирает
  idle-эвикшн), и не видит, что звено urltest-позиции ещё не родилось.
- **Ошибки сваливаются в одну строку.** В секции одна красная строка под
  списком (`errLabel`), в неё по очереди падают провал прогрева после
  тумблера и ошибка пробы. При двух проблемах видна одна; `last_error`
  звена не показывается вообще.
- **Флаг разрыва соединений недоступен.** У ядра в `ChainOutboundOptions`
  есть `interrupt_exist_connections` (SPEC 075: внутренние соединения при
  тумблере рвутся всегда, внешние/пользовательские — только при флаге). У
  лаунчера `configtypes.SourceChain` (`core/config/configtypes/types.go:887`)
  поля нет, эмиттеры (`chain_generator.go:100`, `configtypes/chain_body.go`)
  его не пишут, форма (`ui/configurator/tabs/source_chain_tab.go`) не
  предлагает. Комментарий у `StripEvasion` ссылается на «ту же
  трёхзначность, что у InterruptExistConnections», но самого поля не
  завели. Итог: тумблер всегда мягкий, живые потоки доживают старым путём,
  задать иное нельзя.

## 2. Решения (утверждены владельцем 2026-09-06)

### 2.1 Секция «Chain positions» в окне Info

Хвост состояния — только словами ядра, ничего своего не выдумывать:

```
Chain positions (3)
[x]  1. wg-fi                                       412 ms
[x]  2. 🇩🇪 Berlin-pool  ● de-3  · starting
     ⚠ handshake did not complete within 5s; retrying
[ ]  3. reality-nl  · off · active                  —
     ⚠ position 3 via 1→2: dial tcp 1.2.3.4:443: i/o timeout
                    ( ↻ Probe by position )
```

Правила хвоста строки позиции (`chainPositionText`):

| Disabled | CloneState | хвост |
|---|---|---|
| нет | пусто | ничего (вход, или звено ещё не создано — urltest-позиция родится лениво) |
| нет | `starting` / `active` / `idle` | `· <state>` |
| да | пусто | `· off` |
| да | непусто | `· off · <state>` — «выключена, но звено ещё держит соединения»; когда эвикшн его заберёт, останется `· off` |

Слово `retiring` и любые другие свои состояния — НЕ вводить.

Красная строка под КАЖДОЙ позицией (`posErr[i]`): скрыта, пока пустая,
`Wrapping = TextWrapWord` (иначе длинный текст ядра раздует окно —
см. память «Fyne label min-width trap»), `DangerImportance`. Общая строка
под списком остаётся для того, что к позиции не привязано.

Маршрутизация ошибок:

| источник | когда | куда |
|---|---|---|
| `warmupError` из ответа тумблера | сразу после клика | строка под этой позицией |
| `LastError` из `GetChains` | при каждом перечитывании состава: открытие окна, после тумблера, перед пробой | строка под этой позицией, только если она ПУСТА |
| ошибка пробы ядра | после замера | строка под этой позицией; в колонке задержки слово `error` |
| ошибка транспорта: старое ядро, RPC отвалился, `ChainFor` не ответил | после клика или замера | общая строка под списком, как сейчас |

Приоритет: свежее событие вытесняет старое. Клик по тумблеру чистит строку
СВОЕЙ позиции перед вызовом; проба чистит ВСЕ строки позиций перед
замером; `LastError` заполняет только пустые. Так ошибка прогрева не
потрётся старым `last_error` из состава, который ядро могло ещё не обновить.

Ловушки:
- `applyChainRows` (`servers_node_info_chain.go:~252`) вызывается из
  UI-потока под флагом `applying`; туда же ложится заполнение `LastError`.
  Ходить в ядро из него нельзя (SPEC 113-E).
- `applyChainProbeResults` сейчас берёт `firstError` в общую строку —
  заменить на строку позиции; общая строка от пробы больше не пишется.
- `probeChainLayers` пропускает `Disabled`/`Transparent` (тег `chain#i` при
  выключенной позиции измерил бы чужой путь) — не трогать.

### 2.2 Редактор цепочки, раскрывашка Advanced

Одна галочка под idle timeout, по умолчанию снята, как у ядра:

```
[ Advanced ▾ ]
  Link idle timeout   [ 5m — core default, 0s — never drop ]
  [ ] Interrupt existing connections on position toggle
      Otherwise live connections finish on the old route;
      only new ones take the new path.
  ───────────────────────────────────────────
  [x] Strip DPI evasion from links
```

Поле `SourceChain.InterruptExistConnections bool json:"interrupt_exist_connections,omitempty"`
— обычный bool, НЕ указатель: у ядра дефолт false, «не задано» и
«выключено» неотличимы, трёхзначность не нужна. Комментарий у
`StripEvasion` про «ту же причину, что у InterruptExistConnections»
переписать: он ссылается на поле Направления (`DirectionAuto`), где
указатель нужен, у цепочки — нет.

Ключ пишется ТОЛЬКО при true, во всех четырёх местах, где живёт форма
цепочки (эмиттер и парсер ходят парой — память «emitter-parser-pairing»):

1. `core/config/chain_generator.go` `ChainOutboundObject` — конфиг ядра;
2. `core/config/configtypes/chain_body.go` `ChainBody`/`ChainFromBody` —
   тело v7; порядок ключей фиксирован для байт-в-байт Load→Save→Load:
   `type → idle_timeout → interrupt_exist_connections → strip_evasion → strip → rewrite`;
3. `ui/configurator/tabs/source_edit_window.go:~1920` — разбор вкладки JSON
   цепочки (структура `parsed`);
4. `core/backup/file.go:~309` `chainBodyKeys` — allowlist сканера бэкапа,
   иначе ключ уедет как `backup_unknown_field`.

Форма: `f.interrupt *widget.Check`, Load/Collect по образцу `idleEntry`
(снять `OnChanged` на время `SetChecked`, иначе загрузка формы
пометит источник изменённым; `OnChanged → f.changed()`, иначе правка
одной этой галочки молча теряется — тот же баг, что был у idle_timeout,
см. комментарий `:393`).

### 2.3 Контракт (LX Shared Contract)

- `contract/schema/source_chain.schema.json`: новое необязательное
  `interrupt_exist_connections: boolean`, описание — семантика SPEC 075
  ядра; «Поддержка: обе» (LxBox тоже эмитит `chain` outbound).
- `contract/README.md`: строка `0.12.9`.
- `contract/corpus/direction/chain_interrupt_connections.{direction,expected}.json`
  — новый кейс: флаг true доезжает до конфига; существующий
  `chain_strip_and_timeout` НЕ менять (его golden делит LxBox).
- `contract/TASKS_LXBOX.md`: §11 — зеркальная правка: поле в модели
  цепочки, эмиссия, бэкап.

### 2.4 Документация

`docs/ParserConfig.md:244`, `docs/ParserConfig.ru.md:244`,
`docs/WIZARD_STATE.ru.md:146` — добавить ключ в перечень полей `chain`.

### 2.5 Локализация

Новые ключи в `bin/locale/ru.json` (естественные ключи, SPEC 111):
- `Interrupt existing connections on position toggle`
- `Otherwise live connections finish on the old route; only new ones take the new path.`
- `starting`, `active`, `idle` — состояния звена (словарь ядра, в UI
  переводятся).

## 3. Вне объёма

- `GetChainCloneConfig` (вкладка эффективного конфига звена) — отдельная
  задача.
- Classic-режим: секции нет по-прежнему (Clash API этого не умеет).
- Вид фактического пути `ChainPath` — не рисуется, состояние «всё
  выключено» читается по `· off` на всех строках.

## 4. Проверка

- `go build ./... && go vet ./...`
- Прогон существующих: `go test ./core/config/... ./core/backup/... ./ui/configurator/tabs/...`
  (корпус direction подхватит новый кейс; goldens chain_body roundtrip).
- Руками в daemon-режиме: тумблер выключения на позиции с живым трафиком
  → строка `· off · active`, через idle_timeout → `· off`; включение
  позиции с недоступным сервером → красная строка под ней, галочка на месте.
