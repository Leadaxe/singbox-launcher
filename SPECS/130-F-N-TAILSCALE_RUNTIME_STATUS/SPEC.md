# SPEC 130 · Статус tailnet у работающего ядра

Статус: **N — принята** 16.09.2026 владельцем; реализация одной волной в этой
же сессии от develop `e6aad5a2` (незакоммиченный хвост Tailscale-формы,
окна «Core runtime», remote `state_directory`). Связанные: SPEC 122 (узел
tailnet, §2.3 — узел без `exit_node` не выход), SPEC 121 (секции узла),
`core/chain_probe.go` + `ui/servers_node_info_chain.go` (образец секции по
gRPC), `internal/daemonpb` ревизии `d3b092d7` (три Tailscale-RPC).

---

## 1. Зачем

Узел `tailscale` с `advertise_exit_node` ни в одну группу не входит
(`IsExitCapable`, SPEC 122 §2.3) — для вкладки серверов, которая смотрит
только на группы, его нет. Окно «Core runtime» (вкладка Endpoints) сделало
его **видимым**, но в колонке статуса стоял «—»: пинг через узел без
`exit_node` ушёл бы в tailnet и вернул ошибку, а не задержку.

Настоящий статус — поднялся / ждёт входа / в tailnet, адрес, пиры — у ядра
есть, но наружу шёл только в лог. На роутере и лога нет.

## 2. Решение владельца (16.09.2026)

| Что | Норма |
|---|---|
| Колонка на вкладке Endpoints | одно слово по `BackendState`: `starting` / `running` / `stopped` / `needs login` / `no state`; без счётчика пиров |
| Подробности | **секцией «Tailscale» в окне Info узла**, не отдельной вкладкой — у окна одна форма, вкладки не появляются |
| Login | **кнопка** `Open login URL` при `NeedsLogin` и непустом `AuthURL`; строка с самим URL не показывается |
| Подписка | **постоянный стрим** на время коннекта, не на время окна: колонка актуальна без открытого окна |
| Exit-node из UI | только показать; `SetTailscaleExitNode` не зовём (§3) |

## 3. Границы

- **Только gRPC-бэкенды** — `DaemonBackend` (macOS) и `LxdRemoteTransport`
  (удалённая машина). Legacy на Windows/Linux ходит в Clash API; секция там
  не рисуется, колонка остаётся «—». Гейт — `ac.TailscaleAvailable()`, как
  `ChainsAvailable()`.
- **`StateText` ядра не используется**: locale в gRPC-метаданных лаунчер не
  шлёт (проверено: `metadata.*` нет ни в одном транспорте), текст пришёл бы на
  дефолте ядра. Слова — `locale.T` по сырому `BackendState`.
- **«Одобрен ли мой анонс exit-node» — показать нечем.** `ExitNodeOption`
  есть у `TailscalePeer`, то есть у *пиров*; про *свой* анонс статус молчит.
  Задача форку, не лаунчеру.
- **Переключение exit-node из UI не делаем.** RPC принимает `StableID` пира,
  конфиг — IP или hostname (`SetExitNodeIP`): две адресации одной сущности,
  рантайм не запишет в state то, что применил; плюс SPEC 122 §2.3 — пул
  Направлений о рантайм-смене не узнает. Переключатель, если понадобится,
  пишет `exit_node` в узел и делает apply.
- **Пинг пира** (`StartTailscalePing`) — следующая фаза, не в этой волне.

## 4. Устройство

```
ядро ──SubscribeTailscaleStatus──▶ кеш ──▶ ac.TailscaleStatus(tag) ──▶ UI
        (все endpoint'ы разом,      │
         первый кадр сразу)         └── один на процесс, живёт с коннектом
```

| Слой | Файл | Что |
|---|---|---|
| Типы, конвертер, кеш | `core/services/tailscale_status.go` | `TailscaleStatus/Peer/UserGroup`, `TailscaleStatusesFromPB`, `TailscaleStatusCache` (Apply заменяет снимок целиком; MarkDead снимает живость, снимок оставляет) |
| Диспетчер | `core/tailscale_status.go` | `tailscaleSource`, `ac.TailscaleAvailable/TailscaleStatus/TailscaleLive` — схема `ChainsAvailable/ChainFor`: remote-override → бэкенд |
| Стрим, локальный демон | `core/backend_daemon_tailscale_darwin.go` | `superviseTailscale` — образец `superviseConnections`; стартует в `NewDaemonBackend` рядом с остальными |
| Стрим, удалённая машина | `core/services/lxd_remote_tailscale.go` | ленивый `ensureTailscaleStream` (`streamConn` + `runResilientStream`), гасится в `Close()` |
| Колонка | `ui/core_runtime_window.go` | `coreRuntimeNodeRowWithStatus`; у tailscale — `tailscaleStateLabel(BackendState)` вместо пинга |
| Секция Info | `ui/servers_node_info_tailscale.go` | `addTailscaleSection` — образец `addChainSection`; вставка в `servers_node_info.go` после chain |

Почему кеш и конвертер в `services`, а не в `core`: `core` импортирует
`services`, транспорт удалённой машины живёт в `services` — обратная
зависимость дала бы цикл. `core.TailscaleStatus` — алиас.

Почему стрим удалённой машины **ленивый** (первое чтение), а локального —
с конструктора: `registry.Transport(id)` зовётся и там, где статус не
нужен (проверка связи, деплой); открывать стрим к роутеру ради этого незачем.

## 5. Секция «Tailscale» в окне Info

```
Tailscale
State        running                      ← + «stale, 2m ago» при оборванном стриме
[Open login URL]                          ← только NeedsLogin && AuthURL
Auth         auth_key | interactive
Network      example.ts.net · MagicDNS tail1234.ts.net
This node    mac · 100.64.0.2             ← нет до входа (Self nil)
Exit node    router · 100.64.0.1 · online | none
Peers (1 online / 2)
  ● router   100.64.0.1 · linux · exit node · ↓12 KB ↑3 KB
  ○ phone    100.64.0.5 · android · last seen 2h ago
```

Пиры — по группам пользователя (заголовок группы только при >1 группе),
онлайновые первыми. Нет статуса по тегу — словами, не пустой секцией:
«стрим не подключён» отличается от «ядро не сообщило».

## 6. Ловушки

- **Nil-поля.** `Self` и `ExitNode` пусты в NoState/NeedsLogin — все строки
  nil-safe (`tailscalePeerFromPB` возвращает nil на nil, секция строку
  пропускает).
- **Ноль tailscale-узлов.** Стрим либо не поднимается, либо молчит; кеш
  пуст, `Get` → ok=false, колонка «—». Отдельно не гейтится.
- **`daemonpb` = `package daemon`** — импорт только с алиасом `daemonpb`,
  иначе «undefined: daemonpb».
- **Remote `Close()`** гасит стрим ДО закрытия conn, иначе
  `runResilientStream` переподписывается на мёртвом соединении до отмены.
- **Длинные строки** (AuthURL в ошибке, детали пира) — `Wrapping`, урок
  `newChainErrLabel`.
- **Ширина окна Info** не менялась (620×400): секция вертикальная, колонка
  ключей `nodeInfoKeyColumnWidth` общая.

## 7. Приёмка

1. Локальный lxd с tailscale-узлом: колонка Endpoints показывает слово
   состояния, ⓘ — секцию с адресом и пирами.
2. Узел без `auth_key`: `needs login` в колонке, кнопка в секции открывает
   браузер на AuthURL.
3. Remote (роутер): то же через `LxdRemoteTransport`; после Disconnect
   стрим погашен (нет DEBUG «tailscale: stream closed» по кругу).
4. Обрыв стрима: секция показывает «stale, N ago», снимок не пропадает.
5. Тест: `core/services/tailscale_status_test.go` — конвертер (nil/пустой
   тег отброшены, Self/ExitNode/пиры/LastSeen перенесены) и кеш (Apply
   заменяет, MarkDead не стирает, OnSnapshot зовётся). UI не тестируется.
