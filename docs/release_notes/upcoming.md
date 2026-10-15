# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Tailscale: removed the "Keep local network reachable while using the exit node" checkbox from Add server and from the node's Settings block. It had no effect: the core's Tailscale stack installs no LAN-bypass routes because sing-box does all routing. LAN traffic bypasses the exit node through the "Private IPs" rule preset. `exit_node_allow_lan_access` is still accepted from JSON.
- DNS rules: the "SRS (existing rule_set)" picker now lists the rule sets of your own SRS rules (`user:<name>`, plus `user:<name>:2`, … for rules with several URLs), not only those of enabled library presets ([#140](https://github.com/Leadaxe/singbox-launcher/issues/140)).
- DNS servers: the server form now covers DoQ (`quic`, port 853) and DoH3 (`h3`, port 443, with path) alongside UDP/TCP/DoT/DoH, same as LxBox. Before, these types could only be edited on the JSON tab.
- OpenConnect (AnyConnect, GlobalProtect, Fortinet, F5, Pulse) nodes added as sing-box JSON (`"type": "openconnect"`) now land in `endpoints[]`, where the core expects them. Before, the launcher wrote them to `outbounds[]` and the core rejected the whole config with `unknown outbound type: openconnect`. The body is passed through as written; on a core built without `with_openconnect` the node is dropped with a build-report warning instead of breaking the config. Contract 1.1.120.
- A node whose body the core cannot even decode (for example an old MASQUE body with `network`: `decode config: outbounds[12].network: json: unknown field "network"`) is now turned off automatically like any other node the core rejects, and the config is rebuilt. Before, this error named no tag, so the safety net stayed silent and the VPN did not start. Contract 1.1.121.
- Detour picker and rule/DNS target lists no longer offer template Direction stubs you deleted (`vpn ①`, `vpn ②`): they are not in the config, so picking one pointed to nothing. `block-out` is no longer offered as a detour either — chaining through a block makes no sense.

### Technical / Internal
- Fyne 2.8.1 → 2.8.2 ([#142](https://github.com/Leadaxe/singbox-launcher/pull/142)); GLFW 3.4 moves to the tagged `pre.2`.
- golangci-lint CI takes Go from `go.mod` (it was pinned to 1.25 and failed `go mod tidy` on every PR), golangci-lint 2.14.0, action v9; the 36 errcheck/unused/bodyclose findings it reported are fixed. The stale Dependabot ignore for `golang.org/x/sys` ≥ 0.48 is removed (the toolchain is already Go 1.26).

## RU
### Основное
- Tailscale: из «Add server» и блока узла в Settings убрана галка «Keep local network reachable while using the exit node». Она ничего не делала: стек Tailscale в ядре маршрутов LAN-защиты не ставит, маршрутизацию ведёт sing-box. Локальную сеть мимо exit node пускает пресет «Private IPs». Поле `exit_node_allow_lan_access` по-прежнему принимается из JSON.
- DNS-правила: в выборе «SRS (existing rule_set)» теперь есть наборы ваших собственных SRS-правил (`user:<имя>`, а у правил с несколькими URL ещё `user:<имя>:2`, …), а не только наборы включённых пресетов библиотеки ([#140](https://github.com/Leadaxe/singbox-launcher/issues/140)).
- DNS-серверы: форма сервера теперь умеет DoQ (`quic`, порт 853) и DoH3 (`h3`, порт 443, с путём) наряду с UDP/TCP/DoT/DoH — как в LxBox. Раньше эти типы правились только на вкладке JSON.
- Узлы OpenConnect (AnyConnect, GlobalProtect, Fortinet, F5, Pulse), добавленные как sing-box JSON (`"type": "openconnect"`), теперь попадают в `endpoints[]`, где их ждёт ядро. Раньше лаунчер писал их в `outbounds[]`, и ядро отвергало весь конфиг с `unknown outbound type: openconnect`. Тело уходит как написано; на ядре без `with_openconnect` узел снимается с предупреждением в отчёте сборки, а не ломает конфиг. Контракт 1.1.120.
- Узел, тело которого ядро не может даже декодировать (например, старое тело MASQUE с `network`: `decode config: outbounds[12].network: json: unknown field "network"`), теперь выключается автоматически, как любой узел, отвергнутый ядром, и конфиг пересобирается. Раньше в такой ошибке не было тега, страховка молчала, и VPN не поднимался. Контракт 1.1.121.
- Выбор detour и списки целей правил/DNS больше не предлагают удалённые вами заготовки Направлений шаблона (`vpn ①`, `vpn ②`): в конфиге их нет, и выбор указывал в пустоту. `block-out` из detour тоже убран — цепочки через блокировку не бывает.

### Техническое / Внутреннее
- Fyne 2.8.1 → 2.8.2 ([#142](https://github.com/Leadaxe/singbox-launcher/pull/142)); GLFW 3.4 переходит на тег `pre.2`.
- CI golangci-lint берёт Go из `go.mod` (был пин 1.25, и `go mod tidy` падал на каждом PR), golangci-lint 2.14.0, action v9; 36 его замечаний (errcheck/unused/bodyclose) исправлены. Снят устаревший запрет Dependabot на `golang.org/x/sys` ≥ 0.48 (тулчейн уже Go 1.26).
