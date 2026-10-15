# SPEC 170 — Tunnel ingress: пир клиентского туннеля не шлюз; галки `allow_local` / `allow_lan` / `allow_gateway` у узла; `skip_presets` → `skip_tailscale_presets`

Статус: **C** (закрыта 15.10.2026). Тип: Feature. Контракт: бамп `contract/VERSION` → **1.1.126**, решение **D-134**, норма `contract/docs/TEMPLATE_LANG.md` §6.6, `contract/docs/BACKUP.md`, `contract/schema/backup.schema.json`, `contract/registry/presets.json`, LxBox — `contract/TASKS_LXBOX.md` §123. Зависит от SPEC 169 (закрыта: короткая форма `#if`, `#if` на операции `#do`).

Решения владельца приняты 15.10.2026 по итогам разбора с агентом ядра sing-box-lx.

---

## 1. Проблема

1. **Endpoint — двусторонний канал.** У клиентских endpoint'ов `wireguard` (и AWG), `openvpn-client`, `openconnect`, `tailscale` входящий поток из туннеля ядро отдаёт в роутер как обычное inbound-соединение с `metadata.Inbound = <тег endpoint'а>`. Адресат, равный нашему туннельному адресу, endpoint переписывает в loopback; любой другой идёт по `route.rules` и в `final`.
2. **gVisor-стек принимает пакеты на любой dst.** Сервер WG/AWG, OpenVPN или OpenConnect с той стороны может послать SYN на `192.168.1.1` или на внешний IP — и мы откроем соединение: при `final = direct` это доступ к нашему LAN и выход с нашего IP, при `final = proxy` сервер катается по нашей цепочке. Кернельный WG так себя не ведёт (нужен `ip_forward`), так что наш netstack заметно «открытее». Tailscale фильтрует адресатов control-plane'ом (наши адреса плюс анонсированное `advertise_routes`/`advertise_exit_node`), но пропущенное дальше тоже идёт по нашим правилам.
3. **Ядро менять не будем.** Это поведение upstream: endpoint задуман как «inbound + outbound», в том числе чтобы публиковать локальные сервисы в WG-сети. Глухой reject в ядре сломал бы законный сценарий и разъехался с upstream. Закрываем на уровне генератора конфига: правило `inbound` матчится по тегу endpoint'а (`rule_item_inbound.go`), хотя в документации поле описано как «Tags of Inbound».
4. **Автоматики не будет.** Ни `advertise_routes`, ни включённый `private-ips` сами по себе ничего пиру не открывают. Доступ открывают только явные галки у узла.
5. **`skip_presets` стал бы дырой.** Поле означает «пресеты с обходом узел не обслуживают» — оно сняло бы и защитный пресет, тихо открыв шлюз. Поле сужается до того, для чего и вводилось (LxBox §578): пресет `tailscale`.

## 2. Решение

### 2.1 Три поля записи узла (D-134)

| Поле записи | Тумблер в окне узла | Смысл для пира этого endpoint'а |
|---|---|---|
| `allow_local` | Allow access to this device | достаёт сервисы на нашем туннельном адресе (ядро уже переписало адресат в loopback). Tooltip предупреждает: сервис видит пира как `127.0.0.1`, доступно всё, что доверяет localhost |
| `allow_lan` | Expose local network | видит наши частные сети (`ip_is_private`) |
| `allow_gateway` | Allow as gateway | ходит через нас куда угодно; трафик уходит в outbound, выбранный в пресете (`gateway_out`), DNS не отбрасывается |

- Тип bool; **пишется только `true`** (`omitempty`), отсутствие = `false` — как у `skip_presets`.
- Носитель — свой сервер в корне и член папки (`kind: server`). У узла подписки записи нет — значение всегда ложь.
- Видны обходу как `@node.allow_local`, `@node.allow_lan`, `@node.allow_gateway` (закрытый перечень §6.6 расширяется контрактом).
- Тумблеры показываются только у узлов с `body.type` ∈ {`wireguard`, `openvpn-client`, `openconnect`, `tailscale`}. У Tailscale рядом подсказка: `advertise_routes`/`advertise_exit_node` анонсируют нас в tailnet, а пропускает пира галка здесь.
- Импорт бэкапа: узел, совпавший по телу с уже имеющимся, получает `true` из файла; отсутствие поля своё значение не сбрасывает (норма `skip_presets`, BACKUP.md).

### 2.2 `skip_presets` → `skip_tailscale_presets`

- JSON-ключ записи в состоянии и бэкапе — `skip_tailscale_presets`; Go-идентификаторы — `SkipTailscalePresets`; имя обхода — `@node.skip_tailscale_presets`; фильтр пресета `tailscale` читает новое имя.
- **Чтение терпимое:** старый ключ `skip_presets` в `state.json` и в импортируемом бэкапе читается как новое поле (`true` → `true`), пишется только новое. Сторона LxBox делает то же.
- Тумблер «Skip Tailscale presets» показывается только у узлов `tailscale` (сегодняшняя проверка «пресет узел обслуживает» остаётся, тип добавляется).
- Пресет `tunnel-ingress` это поле **не читает**.
- Корпус `for_each_node*`: поле `nodes[].skip_presets` → `skip_tailscale_presets`, фильтры кейсов — новое имя.

### 2.3 Пресет `tunnel-ingress`

Один пресет на `for_each_node_do`: пять списков, четыре правила, одно DNS-правило. Списки — переменные пресета: четыре видимых (ввод пользователя — база, собранное добавляется на сборку, `unique` снимает дубли; так тег можно вписать руками) и один служебный.

```json
{
  "id": "tunnel-ingress",
  "label": "Tunnel ingress",
  "description": "Connections arriving from a client tunnel (WireGuard, AWG, OpenVPN, OpenConnect, Tailscale) are rejected unless the node's own switches allow them: access to this device, the local network, or using this device as a gateway. The lists below are filled from those switches; add tags by hand to extend them.",
  "default_enabled": true,
  "num": 940,
  "vars": [
    { "name": "all_nodes", "type": "text_list", "unique": true, "title": "Tunnel endpoints",
      "tooltip": "Every client tunnel endpoint in the config. Anything from them not allowed below is rejected." },
    { "name": "allow_local_nodes", "type": "text_list", "unique": true, "title": "May reach this device",
      "tooltip": "Peers of these endpoints reach services on this device's tunnel address. The service sees them as 127.0.0.1." },
    { "name": "allow_lan_nodes", "type": "text_list", "unique": true, "title": "May reach the local network",
      "tooltip": "Peers of these endpoints reach private networks behind this device." },
    { "name": "allow_gateway_nodes", "type": "text_list", "unique": true, "title": "May use this device as a gateway",
      "tooltip": "Peers of these endpoints route everything else through the outbound chosen below." },
    { "name": "closed_nodes", "type": "text_list", "wizard_ui": "internal" },
    { "name": "gateway_out", "type": "outbound", "default": "proxy-out", "title": "Gateway traffic goes to",
      "tooltip": "Outbound for peers allowed to use this device as a gateway." }
  ],
  "for_each_node_do": {
    "#filter": { "@node.body.type": { "#in": ["wireguard", "openvpn-client", "openconnect", "tailscale"] } },
    "#do": {
      "@all_nodes":           { "#append": "@node" },
      "@allow_local_nodes":   { "#if": ["@node.allow_local",   { "#append": "@node" }] },
      "@allow_lan_nodes":     { "#if": ["@node.allow_lan",     { "#append": "@node" }] },
      "@allow_gateway_nodes": { "#if": ["@node.allow_gateway", { "#append": "@node" }] },
      "@closed_nodes":        { "#if": [{ "#not": "@node.allow_gateway" }, { "#append": "@node" }] }
    }
  },
  "rules": [
    { "#if": [{ "@allow_local_nodes": "#notEmpty" },   { "inbound": "@allow_local_nodes",   "ip_cidr": ["127.0.0.0/8", "::1/128"], "outbound": "direct-out" }] },
    { "#if": [{ "@allow_lan_nodes": "#notEmpty" },     { "inbound": "@allow_lan_nodes",     "ip_is_private": true, "outbound": "direct-out" }] },
    { "#if": [{ "@allow_gateway_nodes": "#notEmpty" }, { "inbound": "@allow_gateway_nodes", "outbound": "@gateway_out" }] },
    { "#if": [{ "@all_nodes": "#notEmpty" },           { "inbound": "@all_nodes",           "action": "reject" }] }
  ],
  "dns_rules": [
    { "#if": [{ "@closed_nodes": "#notEmpty" }, { "inbound": "@closed_nodes", "action": "reject" }] }
  ]
}
```

Почему так:

- **Порядок `num: 940`** — сразу после запертой головы `traffic-processing` (0) и раньше `private-ips` (950): иначе «private → direct» пропустит пира в LAN. `sniff` и `resolve` в голове не терминальные, терминальный там только `hijack-dns`, его закрывает `dns_rules`.
- **DNS-reject по `closed_nodes`, а не по `all_nodes`:** пир шлюза обязан резолвить имена, `hijack-dns` перехватит даже его запрос к внешнему DNS. Это единственное место, где нужен пятый список.
- **`#notEmpty` на каждом правиле обязателен:** пустой список без него даёт `template_fragment_dropped` и деградацию в отчёте у всех, у кого туннельных узлов нет.
- **Правило без `network`** отбрасывает и ICMP: пинг через нас в LAN закрывается вместе с TCP/UDP. Намеренно.
- **`gateway_out` по умолчанию `proxy-out`**, как `route.final`. Узел с галкой едет в выбранный outbound, минуя остальные правила.
- **Поздний дефолт:** пресет включён по умолчанию и появляется у существующих пользователей один раз (`LateDefaultPresetIDs`, TEMPLATE_LANG §6.7.5).
- `inbound` — в `route_rule_conditions` и `dns_rule_conditions` реестра, гейт «правило без условий» правила не трогает.

Итог для узлов `wg-home` (без галок), `oc-work` (`allow_local` + `allow_lan`), `ts` (`allow_gateway`):

```json
"route.rules": [
  { "inbound": ["oc-work"], "ip_cidr": ["127.0.0.0/8", "::1/128"], "outbound": "direct-out" },
  { "inbound": ["oc-work"], "ip_is_private": true, "outbound": "direct-out" },
  { "inbound": ["ts"], "outbound": "proxy-out" },
  { "inbound": ["wg-home", "oc-work", "ts"], "action": "reject" }
]
"dns.rules": [ { "inbound": ["wg-home", "oc-work"], "action": "reject" } ]
```

### 2.4 Что НЕ вводится

- Правки ядра (`lx.inbound_policy` и подобное) — нет.
- Авто-правило по `advertise_routes` Tailscale — нет (отвергнуто: автоматика).
- Градация «только LAN» через два пресета — не нужна: это `allow_lan` без `allow_gateway`.
- WG с `system: true` и кернельный интерфейс этим не управляются (решает ОС), правило там просто не срабатывает.

## 3. Контракт

- `contract/VERSION` → 1.1.126; `contract/README.md` — строка changelog (D-134).
- `contract/docs/TEMPLATE_LANG.md` §6.6: перечень полей записи — `skip_tailscale_presets` (бывший `skip_presets`, терпимое чтение), `allow_local`, `allow_lan`, `allow_gateway`; §6.5.1 (пресет `tailscale`) — новое имя в фильтре; §6.7.5 — `tunnel-ingress` в примерах поздних дефолтов.
- `contract/docs/BACKUP.md`: строка таблицы `skip_presets` → `skip_tailscale_presets` (+ терпимое чтение старого ключа), три строки `allow_*`.
- `contract/schema/backup.schema.json`: обе записи узла — переименование + три поля; старый ключ в схеме оставить как deprecated с описанием «читается, не пишется».
- `contract/registry/presets.json`: `tunnel-ingress` (`in: launcher` до реализации у LxBox, `note` с §123).
- `contract/registry/protocols/tailscale.json`: текст `note` — новое имя поля.
- `contract/docs/NODE_SECTIONS.md`, `docs/TEMPLATE_REFERENCE.md` + `.ru.md`: упоминания поля.
- Корпус `contract/corpus/template/`: `for_each_node/*` и `for_each_node_do/do_if_*` — переименование поля в `nodes[]` и в фильтрах; README (формат `nodes[]`: `skip_tailscale_presets?`, `allow_local?`, `allow_lan?`, `allow_gateway?`); новые кейсы `for_each_node_do/tunnel_ingress_mixed` (узлы: `wg-home` wireguard без галок, `oc-work` openconnect с `allow_local`+`allow_lan`, `ts` tailscale с `allow_gateway`, `v1` vless; `vars.json` с ручным тегом `manual-wg` в `all_nodes`; expected — четыре правила и DNS-правило из §2.3 с `manual-wg` первым в `all_nodes`) и `for_each_node_do/tunnel_ingress_no_nodes` (узел vless, без ввода — пусто, warnings пусты).
- `contract/TASKS_LXBOX.md` §123: три поля с тумблерами, переименование с терпимым чтением, пресет целиком (тело из §2.3), поздний дефолт, кейсы корпуса как критерий.

## 4. Критерии приёмки

1. `go build ./...`; Win7-тулчейн go1.20 (без builtin `min`/`max`, `slices`, `maps`).
2. Раннеры корпуса зелёные: `TestContractCorpusTemplate`, `TestContractCorpusNodeLoops` (включая переименованные и новые кейсы).
3. `state.json` со старым `skip_presets: true` после загрузки даёт `SkipTailscalePresets = true` и пишется новым ключом; бэкап 1.0 со старым ключом импортируется так же (один именованный тест на загрузку/импорт — единственный новый юнит-тест волны).
4. Golden `TestWizardTemplateConfigUnchanged` / `TestGoldenScenarios`: конфиги без туннельных узлов **не изменились**; при наличии в golden-сценариях узлов-endpoint'ов — обновлённые golden с правилами `tunnel-ingress` в ревью отчёта.
5. Окно узла: три тумблера у четырёх типов endpoint'ов, «Skip Tailscale presets» только у `tailscale`; строки через `locale.T` с переводом в `bin/locale/ru.json`. UI без тестов (показывает владелец).
6. Контракт, корпус, README корпуса, `TASKS_LXBOX.md` §123, `docs/release_notes/upcoming.md` (EN/RU), `RELEASE_NOTES.md` (значимый UX), `docs/ARCHITECTURE.md` при изменении потоков обновлены.
7. CI `gh workflow run ci.yml --ref develop -f run_mode=tests` зелёный.

## 5. Связи

- SPEC 169 — короткая форма `#if`, `#if` на операции `#do` (контракт 1.1.125).
- SPEC 166 — обходы узлов, `for_each_node_do`, `unique`, `wizard_ui: internal`, поздние дефолты.
- LxBox §578 — `skip_presets` (контракт 1.1.86); SPEC 024 — loop guard на уровне приложения (тот же принцип: защиту даёт генератор конфига, не ядро).
