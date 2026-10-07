# SPEC 156 — Xray-балансировщик становится round_robin

Статус: **N** (постановка; код не писался).
Тип: Feature.
Связь: SPEC 094 фаза C (элемент с `routing.balancers[0]` → узел-группа),
SPEC 088 (эмит `round_robin` / `balancer` уже есть), SPEC 128 (пара
`interval` / `idle_timeout` — не дублировать).
Ядро: [007-URLTEST_BALANCE](https://github.com/Leadaxe/sing-box-lx/blob/lx/SPECS/FEATURES/007-URLTEST_BALANCE/FEATURE.md),
SPEC 019. `balancer` законен только при `mode: round_robin`; иначе ядро
отказывается стартовать.

## 1. Проблема

`xrayBalancerFromElement` (`core/config/subscription/xray_balancer.go`)
всегда пишет `type: urltest` плюс `url` / `interval` и выбрасывает
`strategy`. Ядро читает голый `urltest` как `least_test`: весь трафик
уходит в один самый быстрый узел. Стратегия провайдера теряется.

Живой случай (02.10.2026). Одна ссылка Liberty ветвится по User-Agent.
`LxBox/…` и Happ получают Xray-массив; первый элемент
`🇪🇺 🚀Авто | Лучший сервер`:

- `strategy.type` = `leastLoad`
- `strategy.settings.expected` = `7`
- `burstObservatory.pingConfig.interval` = `2m`

Под UA `sing-box/…` тот же URL отдаёт уже sing-box-объект с `urltest`
«Best Latency» без `mode`. Это ветка провайдера, не дыра парсера.

Расхождение с LxBox §322 записано в `contract/registry/protocols/group.json`:
Go игнорирует `strategy`, Dart маппит `leastLoad` с `expected > 1` в
`round_robin`.

## 2. Решение

Маппинг живёт в `xrayBalancerFromElement`. Эмиттер не трогать:
`generateGroupNodeJSON` уже пишет в тело все ключи `Outbound`, кроме
`tag` / `type` / состава.

Для примера из §1 узел должен получить:

```json
{
  "type": "urltest",
  "mode": "round_robin",
  "url": "http://www.gstatic.com/generate_204",
  "interval": "2m",
  "balancer": {
    "pool": 7,
    "sticky_hash": ["process", "domain", "source_ip"]
  }
}
```

`pool` — это `expected`, не зашитая семёрка. Ядро само берёт
`min(pool, число членов)`.

`url` и `interval` остаются как сейчас (`xrayBalancerURL` /
`xrayBalancerInterval`): из `burstObservatory.pingConfig`, иначе
`https://www.gstatic.com/generate_204` и `3m`. В `15m` не переписывать.

## 3. Маппинг strategy

Стратегии Xray: `random` (дефолт, если `strategy` нет), `roundRobin`,
`leastPing`, `leastLoad`. `settings` есть только у `leastLoad`.

| Xray | Что пишем |
|------|-----------|
| `leastLoad`, `expected` > 1 | `mode: round_robin`, `pool` = `expected`, sticky из §4 |
| `leastLoad`, `expected` ≤ 1 или нет | голый `urltest`. Ни `mode`, ни `balancer` |
| `leastPing` | то же, голый `urltest` |
| `roundRobin`, `random`, стратегии нет | `round_robin`, `pool` = число уже отобранных членов |
| неизвестный `type` | голый `urltest`. Пул не выдумывать |

`expected` приходит числом JSON (`float64`), иногда строкой. Нецелое,
отрицательное и не число — как «нет».

Почему у `roundRobin` пул равен числу членов, а не `0`. У ядра `pool: 0`
и опущенный `pool` — дефолт **3**, не «весь набор».

## 4. sticky_hash

Каждый `round_robin` из этого маппинга пишет явно:

```json
"sticky_hash": ["process", "domain", "source_ip"]
```

Один список и для локального конфига, и для серверного (SPEC 097).
Конкретный IP в конфиг не подставлять: ядро берёт `metadata.Source.Addr`
в момент соединения.

- Локалка: источник часто один (`127.0.0.1` или адрес TUN). Клиентов
  различает `process`, сайты — `domain`.
- Сервер: процесса нет, ключ схлопывается в `domain` + IP клиента на
  inbound. За одним NAT несколько машин склеятся. Это не чинить.

Запрещено писать `[]`. Пустой список ядро считает опущенным полем и
подставляет дефолт `["process","domain"]` — без `source_ip`.
`["none"]` здесь не использовать: он выключает липкость.

`sanitizeBalancerOptions` это поле на эмите группы не дописывает. Три
компонента обязан положить парсер.

Это сознательное отличие от дефолта LxBox и ядра (`process`+`domain`).
Лаунчер собирает конфиг и для сервера, поэтому `source_ip` обязателен.
Встречную задачу LxBox на копирование списка не ставить.

## 5. Что не переносить

Молча, без warning. На каждом обновлении Liberty предупреждение было бы
шумом.

- `fallbackTag` — у urltest фолбэка нет, мёртвый слот занимает живой узел.
- `costs`, `baselines`, `maxRTT` — эквивалента нет. Это не `pool_tolerance`.
- `strategy.settings.tolerance` — не писать ни в `tolerance`, ни в
  `pool_tolerance`. Корневой `tolerance` при `round_robin` ядро игнорирует
  и варнит, если `pool_tolerance` не задан.
- `pool_tolerance` не писать. Опущенное поле — дефолт ядра `0`: держать
  пул живым, скорость не ранжировать.

Пара `interval` / `idle_timeout` уже поднимается правилом 6 графового
санитайзера (SPEC 128) для всякого `urltest`, включая узел из подписки.
В парсере вторую копию не заводить. У Liberty `2m`, правило не сработает;
длинный interval провайдера по-прежнему не роняет старт.

## 6. Чего не делать

- Не менять состав группы, selector-префикс и «берём только первый
  balancer». Пустой состав по-прежнему не создаёт группу.
- Не трогать двойники Направлений. У них свой `mode` / `sticky_hash`.
- Не чинить sing-box-ветку той же ссылки. Импорт чужого sing-box
  по-прежнему не копирует `mode` / `balancer` (`singboxGroupOptionKeys`) —
  отдельная дыра, не эта задача.
- Не мигрировать уже сохранённые узлы. Новый режим появляется на следующем
  обновлении подписки.
- Не менять подзаголовок в списке серверов, если он не врёт, что группа
  «самый быстрый».

## 7. Критерии приёмки

1. Фикстура как у Liberty: `leastLoad`, `expected: 7`, несколько
   `proxy-*`. В `Outbound` есть `mode: round_robin`, `balancer.pool == 7`,
   `sticky_hash` ровно `["process","domain","source_ip"]`. `url` и
   `interval` на месте. Нет `tolerance`, `pool_tolerance`, `fallback`.
2. Тот же элемент с `expected: 1` и без `expected` — голый `urltest`,
   ключей `mode` и `balancer` нет.
3. `leastPing` — голый `urltest`.
4. `roundRobin` на N членах — `pool` равен N, не 0 и не 3. Стратегии нет
   и `random` — то же.
5. Неизвестный `strategy.type` — голый `urltest`.
6. `expected` строкой `"7"` считается; `"7.5"`, `""`, мусор — как «нет».
7. Эмит группы содержит `"mode":"round_robin"` и тот же `sticky_hash`.
   Отдельный код эмиттера для этого не нужен.
8. Пустой состав по-прежнему не создаёт группу.
9. Запись в `group.json` больше не говорит, что Go игнорирует `strategy`.
   Отличие `sticky_hash` от Dart названо явно.

## 8. Контракт

Note в `contract/registry/protocols/group.json` (оба абзаца, где сказано
«Go игнорирует strategy» / «кладёт url/interval без фильтра») обновить:
Go маппит `strategy` по §3 и всегда дописывает `sticky_hash` из §4.
Дефолты `url` / `interval` у проектов по-прежнему разные — эту фразу
не затирать.

Бамп `contract/VERSION` (сейчас 1.1.108) — следующим патчем на момент
реализации. Сгенерированные доки — если note в них попадает.
Строка в `TASKS_LXBOX.md`: расхождение осознанное, Dart список не копирует.
