# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

---

## EN

### Highlights

- Subscriptions: **Xray/V2Ray JSON array** body (`[ { full config }, … ]`) — one logical node per element; **`dialerProxy`** (or **`dialer`**) to a **SOCKS** or **VLESS** outbound → sing-box **`detour`** (jump outbound emitted first). Non-empty **`remarks`** → **`Label`** (full text) and tags **`{slug}`** / **`{slug}_jump_server`** for main vs jump (else `xray-{i}` / `xray-{i}_jump_server`); slug keeps letters/digits and **Unicode regional indicators** (UTF flag emoji), then usual prefix/unique rules. Example: `docs/examples/xray_subscription_array_sample.json`. Share URI: outbounds with **`detour`** are not encodable (**`ErrShareURINotSupported`**).

### Technical / Internal

---

## RU

### Основное

- Подписки: **JSON-массив** полных конфигов **Xray** (`[ {...}, … ]`) — по одной логической ноде на элемент; **`dialerProxy`**/**`dialer`** → hop **SOCKS** или **VLESS**, затем основной outbound с **`detour`**. **`remarks`**: полный текст в **`Label`** и в комментарии к outbound в JSON; теги: основной **`{slug}`**, jump **`{slug}_jump_server`** (или **`xray-{i}`** / **`xray-{i}_jump_server`** без `remarks`); в slug сохраняются буквы/цифры и **символы UTF-флагов** (региональные индикаторы). Пример: **`docs/examples/xray_subscription_array_sample.json`**. «Копировать ссылку» для таких нод с цепочкой — не поддерживается (**`detour`**).

### Техническое / Внутреннее
