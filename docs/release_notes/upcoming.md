# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
-

### Fixes
- Xray JSON subscriptions: the `users[0].encryption` value of VLESS nodes (VLESS Encryption / ML-KEM, `mlkem768x25519plus…`) is now carried into the sing-box outbound as is; before, it was dropped and servers requiring VLESS Encryption refused the connection (`bad http protocol version`). Empty or `none` still emits no field, same as the share-link parser (#121).

### Technical / Internal
-

## RU
### Основное
-

### Исправления
- Подписки в формате Xray JSON: значение `users[0].encryption` у VLESS-узлов (VLESS Encryption / ML-KEM, `mlkem768x25519plus…`) теперь переносится в outbound sing-box как есть; раньше поле терялось, и серверы с обязательным VLESS Encryption рвали соединение (`bad http protocol version`). Пусто или `none` по-прежнему не даёт поля — как у парсера share-ссылок (#121).

### Техническое / Внутреннее
-
