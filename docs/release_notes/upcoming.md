# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **Fixed: subscriptions from panels that route the response by User-Agent.** Some panels (Remnawave/Marzban-style) match a substring of the client User-Agent and served the launcher a full sing-box client-config JSON instead of the base64/URI subscription list — which the launcher couldn't ingest (add-subscription failed/crashed on older builds). The User-Agent is now `LxBox/<v> (desktop; <os>)`: a bare `singbox` substring — the failure trigger that made panels serve JSON — is gone.

### Technical / Internal
- `BuildSubscriptionUserAgent` (and the GitHub/core/SRS download UAs) now emit the `LxBox` brand token with a `desktop` variant tag; no bare `singbox` substring. Regression tests: `core/config/configtypes/useragent_test.go`, updated `TestFetchSubscription_UserAgentFormat`.

## RU
### Основное
- **Исправлено: подписки от панелей, которые роутят ответ по User-Agent.** Некоторые панели (Remnawave/Marzban-типа) матчат подстроку в User-Agent клиента и отдавали лаунчеру полный sing-box JSON-конфиг вместо base64/URI-списка подписки — а его лаунчер переварить не мог (добавление подписки падало/крашилось на старых сборках). User-Agent теперь — `LxBox/<v> (desktop; <os>)`: bare `singbox`, который и был триггером (панель отдавала JSON), убран.

### Техническое / Внутреннее
- `BuildSubscriptionUserAgent` (и UA для скачивания с GitHub/ядра/SRS) теперь выдают бренд-токен `LxBox` с меткой варианта `desktop`; подстроки `singbox` без дефиса нет. Регресс-тесты: `core/config/configtypes/useragent_test.go`, обновлённый `TestFetchSubscription_UserAgentFormat`.
