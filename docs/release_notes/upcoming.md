# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
-

### Fixes
- Subscriptions with a rare URLTest interval no longer break the core startup. A provider asking for `"interval": "3h"` produced a config the core rejected with `interval must be less or equal than idle_timeout`, because `idle_timeout` was missing and defaulted to 30m. The build now pairs such a group with a matching `idle_timeout`; the provider's interval itself is never shortened, so the server is not probed more often than it asked (#118). The same fix covers the launcher's own `1h` URLTest interval setting, which hit the identical failure without any subscription involved.

### Technical / Internal
-

## RU
### Основное
-

### Исправления
- Подписки с редким интервалом URLTest больше не ломают старт ядра. Провайдерский `"interval": "3h"` давал конфиг, который ядро отвергало с `interval must be less or equal than idle_timeout`: парного `idle_timeout` в конфиге не было, и ядро подставляло свои 30 минут. Теперь сборка достраивает такой группе `idle_timeout` под интервал, а сам интервал остаётся провайдерским — сервер не опрашивается чаще, чем он просил (#118). Тот же фикс закрывает и собственную настройку лаунчера `1h`, которая падала так же без всякой подписки.

### Техническое / Внутреннее
-
