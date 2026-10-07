# TASKS 156 · Xray strategy → round_robin

## Этап 1. Разбор

- [ ] Прочитать `xrayBalancerFromElement` и фикстуру
      `TestXrayBalancerBecomesGroupNode`: стратегия сегодня выбрасывается,
      `url` / `interval` уже переносятся
- [ ] Сверить таблицу SPEC §3 с LxBox §322. Отличие одно и оно названо:
      `sticky_hash` включает `source_ip`
- [ ] Не заводить свою пару `interval` / `idle_timeout`: это правило 6
      SPEC 128

## Этап 2. Парсер

- [ ] `xrayBalancerStrategy` — type и expected (`float64` и строка; мусор
      как «нет»)
- [ ] `applyXrayBalancerStrategy` — таблица SPEC §3
- [ ] При `round_robin` писать `sticky_hash` ровно
      `["process","domain","source_ip"]`. Не `[]`, не `["none"]`
- [ ] Не переносить `fallbackTag`, `costs`, `baselines`, `maxRTT`,
      `tolerance`. Не писать `pool_tolerance`. Warning не ставить
- [ ] Пустой состав по-прежнему `return nil` до маппинга

## Этап 3. Контракт

- [ ] Оба note в `contract/registry/protocols/group.json`: Go больше не
      «игнорирует strategy»; отличие `sticky_hash` от Dart названо
- [ ] Бамп `contract/VERSION` следующим патчем
- [ ] Перегенерировать доки контракта, если note в них попадает
- [ ] Строка в `TASKS_LXBOX.md`: Dart список не копирует, встречной
      реализации нет

## Этап 4. Тесты

- [ ] Liberty-фикстура: `pool == 7`, три компонента sticky в этом порядке,
      `url` / `interval` на месте, нет `tolerance` / `pool_tolerance`
- [ ] `expected: 1` и нет `expected` — голый `urltest`
- [ ] `leastPing` и неизвестный type — голый `urltest`
- [ ] `roundRobin` и нет `strategy` — `pool` равен числу членов, не 0 и не 3
- [ ] `"7"` считается, `"7.5"` — нет
- [ ] `generateGroupNodeJSON` пропускает `mode` и `sticky_hash` без правки
      эмиттера
- [ ] Пустой состав группы не создаёт

## Этап 5. Приёмка

- [ ] `go test` пакета `core/config/subscription` и эмита группы
- [ ] Существующие тесты балансировщика (владение, состав, порядок) зелёные
- [ ] Код Направлений и `singboxGroupOptionKeys` не менялись
