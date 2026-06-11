# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **Fixed: subscriptions from panels that route the response by User-Agent.** Some panels (Remnawave/Marzban-style) match a substring of the client User-Agent and served the launcher a full sing-box client-config JSON instead of the base64/URI subscription list — which the launcher couldn't ingest (add-subscription failed/crashed on older builds). The User-Agent is now `LxBox-desktop/<v> (sing-box/<core>; <os> <arch>)`: the `sing-box` token makes panels recognize a real sing-box client and return the proper subscription list (a bare `singbox` substring — the failure trigger — is gone).

### Technical / Internal
- `BuildSubscriptionUserAgent` (and the GitHub/core/SRS download UAs) now emit the `LxBox-desktop` brand token plus a `sing-box/<core>` recognition token; no bare `singbox` substring. Regression tests: `core/config/configtypes/useragent_test.go`, updated `TestFetchSubscription_UserAgentFormat`.

## RU
### Основное
- **Исправлено: подписки от панелей, которые роутят ответ по User-Agent.** Некоторые панели (Remnawave/Marzban-типа) матчат подстроку в User-Agent клиента и отдавали лаунчеру полный sing-box JSON-конфиг вместо base64/URI-списка подписки — а его лаунчер переварить не мог (добавление подписки падало/крашилось на старых сборках). User-Agent теперь — `LxBox-desktop/<v> (sing-box/<ядро>; <os> <arch>)`: токен `sing-box` заставляет панели распознать настоящий sing-box-клиент и отдать правильный список подписки (bare `singbox`, который и был триггером бага, убран).

### Техническое / Внутреннее
- `BuildSubscriptionUserAgent` (и UA для скачивания с GitHub/ядра/SRS) теперь выдают бренд-токен `LxBox-desktop` плюс распознавательный `sing-box/<ядро>`; подстроки `singbox` без дефиса нет. Регресс-тесты: `core/config/configtypes/useragent_test.go`, обновлённый `TestFetchSubscription_UserAgentFormat`.
