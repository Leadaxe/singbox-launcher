# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **Remote Linux machine: "TUN auto redirect" no longer blocks Save** (issue #161). A Windows or macOS launcher checked the config of a remote Linux machine with its own core, which rejects `auto_redirect` as Linux-only. The check now ignores this host-only key; the machine still gets it.
- **TUN: "LAN interfaces" apply only in gateway mode** (issue #160). The `include_interface` list of the TUN inbound was written into the config even with **Gateway mode** off — a value left in the greyed-out field (for example an IP address instead of an interface name) broke TUN on the machine. The list is now emitted only when gateway mode is on.

### Technical / Internal
-

## RU
### Основное
- **Удалённая Linux-машина: «TUN auto redirect» больше не ломает Save** (issue #161). Лаунчер на Windows и macOS проверял конфиг удалённой Linux-машины своим ядром, а оно отвергает `auto_redirect` как ключ только для Linux. Проверка теперь этот ключ хоста не учитывает; на машину он уходит как был.
- **TUN: «LAN interfaces» действуют только в режиме шлюза** (issue #160). Список `include_interface` у TUN-инбаунда попадал в конфиг и при выключенном **Gateway mode** — значение, оставшееся в погашенном поле (например IP-адрес вместо имени интерфейса), ломало TUN на машине. Теперь список пишется только при включённом режиме шлюза.

### Техническое / Внутреннее
-
