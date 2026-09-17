# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
-

### Fixes
- vmess: channel cipher list now matches the core exactly — `aes-128-ctr` (which the core never supported and which killed the whole config) is gone, `aes-128-cfb` is accepted instead of silently falling back to `auto`.
- xhttp: `mode`, `seq_placement`, `session_placement`, `uplink_data_placement`, `x_padding_placement` and `x_padding_method` values outside the core's enum are dropped with an `xhttp_param_reset` warning instead of being passed through and aborting the whole config.

### Technical / Internal
-

## RU
### Основное
-

### Исправления
-

### Техническое / Внутреннее
-
