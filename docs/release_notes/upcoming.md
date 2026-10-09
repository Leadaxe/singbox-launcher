# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **Remote machines: is my config on the machine?** The machine row now shows it at a glance. An orange dot on **Deploy** (renamed from "Deploy config") means the config saved here differs from the one running on the machine (sha256 of the built `config.json` vs the daemon's active SHA, checked on every 5-second poll). A red dot on **Configure** means the last deploy did not start and the core rolled back to the last-good config. The ⓘ window also lists the local built-config sha256 next to the machine's active and last-good hashes.

### Technical / Internal
-

## RU
### Основное
- **Удалённые машины: доехал ли конфиг.** Строка машины теперь показывает это сразу. Оранжевая точка на **Отправить** (бывшее «Отправить конфиг»): сохранённый здесь конфиг не тот, что работает на машине (sha256 собранного `config.json` против active SHA демона, сверка на каждом опросе раз в 5 с). Красная точка на **Настроить**: последняя отправка не завелась, ядро откатилось на прежний удачный конфиг. В окне ⓘ рядом с хешами машины добавлен sha256 собранного здесь конфига.

### Техническое / Внутреннее
-
