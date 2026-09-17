# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
-

### Fixes
-

### Technical / Internal
- Core pinned to sing-box-lx 1.14.1-lx.4. Two core-side changes reach users: `tls.reality.key_share` in a node's JSON body is now passed through to the core (`hybrid` / `classical`; empty = whatever the uTLS fingerprint carries), and the template's TLS fragmentation flags (`tls_fragment` / `tls_record_fragment`) now actually apply to REALITY nodes — older cores silently ignored them there, so a REALITY node with fragmentation enabled behaves differently on the wire.

## RU
### Основное
-

### Исправления
-

### Техническое / Внутреннее
- Ядро запинено на sing-box-lx 1.14.1-lx.4. Две правки ядра видны пользователю: `tls.reality.key_share` из JSON-тела узла доезжает до ядра (`hybrid` / `classical`; пусто = как несёт uTLS-отпечаток), и флаги фрагментации TLS из шаблона (`tls_fragment` / `tls_record_fragment`) теперь реально применяются к REALITY-узлам — прежние ядра их там молча игнорировали, так что у REALITY-узла с включённой фрагментацией меняется поведение на проводе.
