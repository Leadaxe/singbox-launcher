# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
-

### Technical / Internal
- Shared contract 1.1.48: every dropped entry in the contract envelope now carries a machine `code` and the element `index`, including entries that could not be parsed at all (new code `form_unrecognized`); the group genus is declared as the sing-box body type; the “comment with `=` is not a node name” rule for `.conf` moved from engine code into the registry.

### Fixed
- Hysteria / Hysteria2: a port-hopping range outside 0–65535 or with leading zeros (`99999:99999`, `00443:00444`) is now dropped from the node with a warning instead of making the core reject the whole configuration.

## RU
### Основное
-

### Техническое / Внутреннее
- Общий контракт 1.1.48: каждая отбраковка в конверте контракта несёт машинный `code` и `index` элемента, включая записи, которые не удалось прочитать вовсе (новый код `form_unrecognized`); род группы объявлен типом тела sing-box; правило «комментарий с `=` — не имя узла» для `.conf` перенесено из кода движка в реестр.

### Исправлено
- Hysteria / Hysteria2: диапазон прыжков по портам вне 0–65535 или с ведущими нулями (`99999:99999`, `00443:00444`) теперь снимается с узла с предупреждением, а не заставляет ядро отвергнуть весь конфиг.
