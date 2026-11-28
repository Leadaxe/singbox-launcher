# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Fixed: an `awg://<base64 .conf>` link with a `#` comment under `[Peer]` (Proton server names) and a `vmess://` link with `#` in the server name produced no node. The `[Peer]` comment now becomes the node name when the link has no `#name`.

### Technical / Internal
- Debug API: `POST /debug/notify` `{"title"?, "body"?}` shows a test system notification the way the launcher does — to check that notifications reach the notification center.

## RU
### Основное
- Исправлено: ссылка `awg://<base64 .conf>` с комментарием `#` под `[Peer]` (имена серверов Proton) и ссылка `vmess://` с `#` в имени сервера не давали узла. Если в ссылке нет `#имя`, комментарий под `[Peer]` становится именем узла.

### Техническое / Внутреннее
- Debug API: `POST /debug/notify` `{"title"?, "body"?}` показывает тестовое системное уведомление тем же путём, что и лаунчер, — проверить, доходят ли уведомления до центра уведомлений.
