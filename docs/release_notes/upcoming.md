# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **Debug API reference doc** — `docs/API.md` ships a complete endpoint reference + curl cookbook for the headless control plane (24 endpoints across health/info, state read/write, actions, traffic profiler, snapshot). Replaces ad-hoc lookups in SPEC 038. Linked from both READMEs.

### Technical / Internal
- README's "28 endpoints" claim corrected to **24** — actual count verified against `core/debugapi/` handler registrations.

## RU
### Основное
- **Документация Debug API** — `docs/API.md`: полный референс endpoint'ов + curl cookbook для headless-слоя (24 endpoint'а в группах health/info, state read/write, actions, traffic profiler, snapshot). Заменяет разрозненный поиск по SPEC 038. Залинкована из обоих README.

### Техническое / Внутреннее
- Заявление в README про «28 endpoints» исправлено на **24** — реальный счётчик сверен с handler-регистрациями в `core/debugapi/`.
