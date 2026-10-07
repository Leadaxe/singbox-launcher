# PLAN 156 · Xray strategy → mode / balancer

## 1. Где ставится фикс и почему там

Стратегия видна только в момент разбора элемента. Дальше в состоянии
лежит уже `Outbound` группы, и эмиттер копирует его ключи как есть
(`generateGroupNodeJSON`, `core/config/outbound_generator.go`).

Чинить эмиттер нельзя: он не знает, откуда группа взялась, и затёр бы
собственный `mode` Направления. Чинить `singboxGroupToNode` нельзя:
чужой sing-box — другая дыра (SPEC §6).

Единственная точка — `xrayBalancerFromElement`
(`core/config/subscription/xray_balancer.go`). Сейчас она пишет пять
ключей и на этом кончается. Рядом уже есть `xrayFirstBalancer`,
`xrayBalancerURL`, `xrayBalancerInterval`. Стратегию читать из того же
`balancer`, не из корня элемента.

## 2. Что добавить

Одна функция, вызываемая из `xrayBalancerFromElement` после сборки
базового `outbound`. Пишет ключи только когда маппинг SPEC §3 даёт
`round_robin`. Иначе `outbound` остаётся как сегодня.

| Функция | Назначение |
|---------|------------|
| `xrayBalancerStrategy(balancer) (kind string, expected int, hasExpected bool)` | `strategy.type`; нет блока — `random`. `expected` из `settings`: `float64` (только целое), строка через `strconv`. Мусор — `hasExpected=false` |
| `applyXrayBalancerStrategy(outbound, balancer, memberCount)` | таблица SPEC §3. При `round_robin` кладёт `mode`, `balancer.pool`, `balancer.sticky_hash` |

`memberCount` — `len(memberTags)` уже после отбора селектором. Пустой
список до этой функции не доходит: ранний `return nil` остаётся.

Ключи, которые не переносим (SPEC §5), функция не читает. Warning не
пишет.

`idle_timeout` здесь не ставить. Пара уже поднимается правилом 6
(`sanitizeURLTestTimings`, SPEC 128) на сборке. Второй копии правила
в парсере не заводить.

## 3. Контракт

Два `note` в `contract/registry/protocols/group.json`:

- верхний: «Go игнорирует strategy (всегда urltest …)» заменить на
  маппинг SPEC §3 и явный `sticky_hash`. Фразу про разные дефолты
  `url` / `interval` оставить;
- нижний (`share`): «Xray-балансировщик кладёт url/interval без фильтра»
  дополнить: ещё `mode` и `balancer`, когда стратегия это требует.

Бамп `contract/VERSION` следующим патчем. `TASKS_LXBOX.md` — короткая
запись «Dart не копирует `source_ip`», без встречной реализации.

## 4. Что не трогать

- `xrayBalancerSelects`, владение сервером, дедуп, порядок членов.
- `generateGroupNodeJSON` и `sanitizeBalancerOptions`.
- `singboxGroupOptionKeys`.
- Направления и их `mode`.
- Уже сохранённые узлы. Обновление подписки пересобирает `Outbound`.

## 5. Тесты

Рядом с `TestXrayBalancerBecomesGroupNode`
(`core/config/subscription/xray_ownership_test.go`). Фикстура Liberty
уже содержит `leastLoad` / `expected: 7` — существующий тест не должен
сломаться от новых ключей, его дополнить проверкой `mode` / `pool` /
`sticky_hash`.

Отдельные случаи — маленькие массивы из одного элемента, без хука
подписи, чтобы состав не схлопывался:

- `expected: 1`, нет `expected`, `leastPing`, неизвестный type — нет
  `mode` и `balancer`;
- `roundRobin` и отсутствие `strategy` — `pool` равен числу членов;
- `expected` строкой `"7"` и мусором `"7.5"`.

Эмит: один вызов `generateGroupNodeJSON` на узле из разбора. В строке
есть `"mode":"round_robin"` и
`"sticky_hash":["process","domain","source_ip"]`. Это фиксирует, что
эмиттер уже пропускает ключи, а не что его надо менять.
