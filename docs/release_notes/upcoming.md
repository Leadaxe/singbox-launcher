# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
-

### Fixes
-

### Technical / Internal
- Linux builds now obtain Wayland header paths from `pkg-config` and fall back to X11 when the optional native Wayland/EGL development files are incomplete, fixing local builds on openSUSE.

## RU
### Основное
-

### Исправления
-

### Техническое / Внутреннее
- Linux-сборка теперь получает пути к заголовкам Wayland через `pkg-config` и использует X11 при неполном наборе опциональных Wayland/EGL-файлов разработки, исправляя локальную сборку в openSUSE.
