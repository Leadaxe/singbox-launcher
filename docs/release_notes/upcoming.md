# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **Remote Linux machine: "TUN auto redirect" no longer blocks Save** (issue #161). A Windows or macOS launcher checked the config of a remote Linux machine with its own core, which rejects `auto_redirect` as Linux-only. The check now ignores this host-only key; the machine still gets it.

### Technical / Internal
-

## RU
### Основное
- **Удалённая Linux-машина: «TUN auto redirect» больше не ломает Save** (issue #161). Лаунчер на Windows и macOS проверял конфиг удалённой Linux-машины своим ядром, а оно отвергает `auto_redirect` как ключ только для Linux. Проверка теперь этот ключ хоста не учитывает; на машину он уходит как был.

### Техническое / Внутреннее
-
