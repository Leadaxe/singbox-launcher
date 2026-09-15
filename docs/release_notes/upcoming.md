# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
-

### Fixes
- **REALITY nodes keep the fingerprint the provider chose.** Since v1.5.6 the launcher replaced any REALITY fingerprint outside the Chrome family (for example `firefox`) with `chrome` when building the config. Some providers set `firefox` on purpose: older servers accept it, and Chrome's larger ClientHello gets cut by DPI in mobile networks, so those nodes stopped connecting. An explicit fingerprint now goes into the config as is; only an empty one (or the launcher's own implicit `random`) becomes `chrome`. Nodes with a non-Chrome fingerprint still show a hint: servers on Xray 26.9.8+ reject such a ClientHello, so if a node does not connect, try `chrome`.

### Technical / Internal
-

## RU
### Основное
-

### Исправления
- **REALITY-узлы сохраняют отпечаток, который выбрал провайдер.** С v1.5.6 лаунчер при сборке конфига заменял любой отпечаток REALITY вне chrome-семейства (например, `firefox`) на `chrome`. Некоторые провайдеры ставят `firefox` сознательно: старые серверы его принимают, а крупный ClientHello chrome режет DPI мобильных сетей, — и такие узлы переставали подключаться. Теперь явный отпечаток уходит в конфиг как есть, `chrome` ставится только вместо пустого (или собственного неявного `random` лаунчера). Узлы с отпечатком вне chrome-семейства по-прежнему показывают подсказку: серверы Xray 26.9.8+ такой ClientHello отвергают, и если узел не подключается — попробуйте `chrome`.

### Техническое / Внутреннее
-
