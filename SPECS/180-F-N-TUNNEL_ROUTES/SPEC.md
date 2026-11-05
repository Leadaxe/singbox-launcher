# SPEC 180 — Tunnel routes: в клиентский туннель идут только его собственные сети; переключатель `own_routes` у узла; пресет `tunnel-routes`

Статус: **N** (реализована 05.11.2026, ждёт показа владельцу и живой проверки). Тип: Feature. Контракт: бамп `contract/VERSION` → **1.1.161**, решение **D-144**, норма `contract/docs/TEMPLATE_LANG.md` §6.5.3, §6.6, `contract/docs/BACKUP.md`, `contract/schema/backup.schema.json`, `contract/registry/presets.json`, LxBox — `contract/TASKS_LXBOX.md` §158.

Источник — запрос LxBox (их задача 718, вопрос пользователя 05.11.2026). Решения владельца приняты и не пересматриваются: пресет по образцу `tailscale`, обход `for_each_node`, поле на включение, пресет включён по умолчанию, место между `tunnel-ingress` и `tailscale`, язык шаблона не расширяется, узлы подписки вне задачи.

## 1. Проблема

Несколько одновременно работающих WireGuard и OpenVPN, через каждый доступны одна-две сети /24, остальное должно идти дальше по правилам. Правило маршрута выбирает узел, дальше узел решает сам: собственные сети туннеля (allowed_ips у WireGuard, маршруты от сервера у OpenVPN и OpenConnect) в `route.rules` не попадают. Адрес вне сетей туннеля, отправленный в него, рвётся и к следующему правилу не переходит. Чтобы пользоваться туннелем «только для своих сетей», сегодня префиксы переписываются в правило руками; у OpenVPN с маршрутами от сервера это невозможно.

## 2. Решение

### 2.1 Поле записи `own_routes` (D-144)

- Bool, по умолчанию `false`, пишется только `true` (`omitempty`).
- Носитель — свой сервер в корне и член папки. У узла подписки записи нет — ложь.
- Пятое поле `configtypes.NodeRecordFlags`: едет тем же путём, что `allow_*` (состояние → эмиссия → кэш сборки → `@node.own_routes`), входит в `Any()` и `MergeTrue()`; бэкап и Debug API несут его через встроенную структуру.

### 2.2 Пресет `tunnel-routes`

Тело — `contract/docs/TEMPLATE_LANG.md` §6.5.3, в шаблоне `bin/wizard_template.json`. `for_each_node` по узлам `wireguard` / `openvpn-client` / `openconnect` с поднятым полем:

- всем — правило маршрута `preferred_by: [узел] → узел`;
- `openvpn-client` и `openconnect`, пока включена переменная пресета `dns_enable`, — правило `resolve`, DNS-сервер типа `openvpn` / `openconnect` с `endpoint` узла, DNS-правило `preferred_by: [<тег>-dns]`.

`num: 943`, включён по умолчанию, сортируемый, поздний дефолт (`state.LateDefaultPresetIDs`). Без поднятого поля пресет не выдаёт ничего: у существующих установок конфиг не меняется.

### 2.3 Интерфейс

Вкладка Settings окна узла, блок «Routing through this tunnel» над блоком «Incoming connections from this tunnel»: переключатель **Only its own networks** с подсказкой. Виден у своего сервера и члена папки с телом `wireguard` (и AWG), `openvpn-client`, `openconnect`. У WireGuard, чьи `allowed_ips` содержат префикс `/0`, под включённым переключателем предупреждение «This tunnel announces the default route, so it takes all traffic.»; переключатель не блокируется. Строка пресета на Rules — стандартная, с настройкой DNS.

## 3. Что проверено по ядру (`v1.14.4-lx.30`)

- `preferred_by` в правиле маршрута: WireGuard — `allowed_ips` пиров, доменов нет; OpenVPN и OpenConnect — маршруты и домены, присланные сервером. Пока туннель не поднят — «нет».
- DNS-сервер типа `openvpn` / `openconnect` без присланного DNS: конфиг принимается, одна строка info на смену конфигурации туннеля, error — только при противоречивом push у OpenVPN (search domains или `DOMAIN-ROUTE` без DNS, `DNSSEC yes`).
- `preferred_by` в DNS-правиле смотрит только DNS-маршруты и search domains: сервер, приславший один адрес DNS без доменов, DNS-правилу не даёт ни одного совпадения.
- Один DNS-сервер на endpoint: второй с другим тегом — отказ старта ядра.

## 4. Чего задача не делает

- Узлы подписки и правила, прикреплённые к подписке.
- Условие `preferred_by` в редакторе правил.
- Защиту от утечки, пока туннель не поднят.
- Проверку «у узла уже есть свой DNS-сервер типа `openvpn` / `openconnect`»: в форме DNS-сервера таких типов нет, конфликт возможен только через сырой JSON.

## 5. Проверка

- Корпус `contract/corpus/template/for_each_node/tunnel_routes_*` (7 кейсов), раннер `TestContractCorpusNodeLoops`.
- `TestTunnelRoutesDNSRuleBeforeFakeIP` — боевой шаблон: DNS-сервер и DNS-правило узла с полем, место раньше `A/AAAA → fakeip`, узел без поля не обслуживается.
- Вживую: WireGuard с одной сетью /24 плюс обычный сервер; OpenVPN с маршрутами от сервера — владелец.
