# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
-

### Fixes
-

### Technical / Internal
- REALITY links accept `key_share=hybrid|classical` on `vless://` and `anytls://`, next to `pbk`/`sid`. `classical` is what REALITY servers on Xray **older** than v26.9.8 need — they drop a hybrid ClientHello, and until now such a server could only be set up by hand-editing the node's JSON body. The value is read only on a real REALITY node (valid `pbk`); anything outside the two values drops the field, not the node.
- Core pinned to sing-box-lx 1.14.1-lx.4. Two core-side changes reach users: `tls.reality.key_share` in a node's JSON body is now passed through to the core (`hybrid` / `classical`; empty = whatever the uTLS fingerprint carries), and the template's TLS fragmentation flags (`tls_fragment` / `tls_record_fragment`) now actually apply to REALITY nodes — older cores silently ignored them there, so a REALITY node with fragmentation enabled behaves differently on the wire.

## RU
### Основное
-

### Исправления
-

### Техническое / Внутреннее
- В ссылках REALITY читается `key_share=hybrid|classical` — у `vless://` и `anytls://`, рядом с `pbk`/`sid`. `classical` нужен REALITY-серверам Xray **старше** v26.9.8: они рвут соединение на гибридном ClientHello, и раньше такой сервер заводился только правкой JSON-тела узла. Значение читается только у настоящего REALITY-узла (валидный `pbk`); всё, кроме этих двух значений, снимает поле, а не узел.
- Ядро запинено на sing-box-lx 1.14.1-lx.4. Две правки ядра видны пользователю: `tls.reality.key_share` из JSON-тела узла доезжает до ядра (`hybrid` / `classical`; пусто = как несёт uTLS-отпечаток), и флаги фрагментации TLS из шаблона (`tls_fragment` / `tls_record_fragment`) теперь реально применяются к REALITY-узлам — прежние ядра их там молча игнорировали, так что у REALITY-узла с включённой фрагментацией меняется поведение на проводе.
