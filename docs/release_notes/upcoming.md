# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Tailscale: removed the "Keep local network reachable while using the exit node" checkbox from Add server and from the node's Settings block. It had no effect: the core's Tailscale stack installs no LAN-bypass routes because sing-box does all routing. LAN traffic bypasses the exit node through the "Private IPs" rule preset. `exit_node_allow_lan_access` is still accepted from JSON.

### Technical / Internal
-

## RU
### Основное
- Tailscale: из «Add server» и блока узла в Settings убрана галка «Keep local network reachable while using the exit node». Она ничего не делала: стек Tailscale в ядре маршрутов LAN-защиты не ставит, маршрутизацию ведёт sing-box. Локальную сеть мимо exit node пускает пресет «Private IPs». Поле `exit_node_allow_lan_access` по-прежнему принимается из JSON.

### Техническое / Внутреннее
-
