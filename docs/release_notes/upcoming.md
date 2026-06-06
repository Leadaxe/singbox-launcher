# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **Servers tab can connect to remote Clash API endpoints (SPEC 064).** Gear ⚙ button in the tab header opens a small dialog where you set host / port / secret and connect to any Clash-API-compatible server: sing-box on a home router (e.g. RouteRich on `192.168.10.1`), mihomo on a VPS through ssh-tunnel, another launcher instance on the same machine. Status badge shows `🏠 Local` or `🌐 host:port`. Tab stays accessible even when the local sing-box isn't running, as long as remote override is active. Ephemeral (RAM-only) — resets on launcher restart. HTTP only for MVP.
- **Settings is now a separate window** opened from the `⚙️` tab. The tab acts as a button — clicking it pops a standalone resizable window (520×640, scrollable) with Subscriptions / Language / Subscription identification / Debug API sections. Singleton — reopening just focuses the existing window. Tab order: `Core │ Servers │ 🔍 │ ⚙️ │ ❓`.
- **Custom subscription User-Agent.** Settings → Subscriptions has a `User-Agent:` field for providers that reject our default `singbox-launcher/<ver> (<os> <arch>)` UA. Type a custom one (e.g. `v2rayN/7.5.0`, `Hiddify/1.0.0`) — autosaves 500ms after you stop typing. ⟳ button resets to default. Default is shown as placeholder + hint text. Sent on every subscription fetch.
- **Debug API reference doc** — `docs/API.md` ships a complete endpoint reference + curl cookbook for the headless control plane. Replaces ad-hoc lookups in SPEC 038. Linked from both READMEs.
- **New Debug API endpoint** — `GET/PATCH /settings/user-agent`. GET returns `{user_agent, default, effective}`. PATCH with `{"user_agent":"..."}` writes; `{"user_agent":""}` resets to default. No sing-box restart needed — fetcher reads settings lazily on every fetch.

### Technical / Internal
- SPEC 064 implementation: new `ui/clash_remote.go` (RemoteOverride storage + EffectiveClashAPIConfig resolver + generation counter for drop-stale in refresh goroutines + NormalizeHost helper). All 9 callsites in `ui/clash_api_tab.go` migrated from `ac.APIService.GetClashAPIConfig()` to `EffectiveClashAPIConfig(ac)`. Tab availability rule now `running || hasOverride`.
- README's "28 endpoints" claim corrected to **24** — actual count verified against `core/debugapi/` handler registrations.
- New `Settings.SubscriptionUserAgent` field in `internal/locale/settings.go`. Plumbed through `subscription.SubscriptionRequestSettings.UserAgent` and `applySubscriptionRequestHeaders` — empty value falls back to `configtypes.BuildSubscriptionUserAgent()`. `UIService.SettingsWindow` singleton field added next to `WizardWindow`.
- New `core/debugapi/settings_endpoints.go` for launcher-level preference mutations (separate namespace from `/state/*`). First inhabitant: `/settings/user-agent`.

## RU
### Основное
- **Вкладка Servers подключается к удалённым Clash API (SPEC 064).** Кнопка-шестерёнка ⚙ в шапке таба открывает диалог с полями host / port / secret — подключаемся к любому Clash-API-совместимому серверу: sing-box на роутере (например RouteRich на `192.168.10.1`), mihomo на VPS через ssh-tunnel, другой инстанс лаунчера на той же машине. Badge в шапке показывает `🏠 Local` или `🌐 host:port`. Tab остаётся доступным даже если локальный sing-box не запущен — пока активен remote override. Ephemeral (только в памяти) — сбрасывается при рестарте лаунчера. HTTP only для MVP.
- **Настройки переехали в отдельное окно** — открывается кликом по табу `⚙️`. Таб теперь работает как кнопка: клик открывает standalone окно (520×640, со скроллом) с секциями Подписки / Язык / Идентификация устройства / Debug API. Singleton — повторный клик просто фокусирует уже открытое. Порядок табов: `Core │ Servers │ 🔍 │ ⚙️ │ ❓`.
- **Кастомный User-Agent для подписок.** В Settings → Подписки появилось поле `User-Agent:` — для провайдеров, которые режут наш default `singbox-launcher/<ver> (<os> <arch>)`. Впишите свой (например `v2rayN/7.5.0`, `Hiddify/1.0.0`) — автосохранение через 500ms после остановки печати. Кнопка ⟳ сбрасывает на стандартный. Default показан как placeholder + подсказка. Отправляется при каждом обновлении подписки.
- **Документация Debug API** — `docs/API.md`: полный референс endpoint'ов + curl cookbook для headless-слоя. Заменяет разрозненный поиск по SPEC 038. Залинкована из обоих README.
- **Новый Debug API endpoint** — `GET/PATCH /settings/user-agent`. GET возвращает `{user_agent, default, effective}`. PATCH с `{"user_agent":"..."}` пишет; `{"user_agent":""}` сбрасывает на default. Рестарт sing-box не нужен — fetcher читает настройки lazy на каждом fetch'е.

### Техническое / Внутреннее
- SPEC 064 реализация: новый `ui/clash_remote.go` (RemoteOverride storage + EffectiveClashAPIConfig resolver + generation counter для drop-stale в refresh-goroutine'ах + NormalizeHost helper). Все 9 callsite'ов в `ui/clash_api_tab.go` мигрировали с `ac.APIService.GetClashAPIConfig()` на `EffectiveClashAPIConfig(ac)`. Tab availability rule теперь `running || hasOverride`.
- Заявление в README про «28 endpoints» исправлено на **24** — реальный счётчик сверен с handler-регистрациями в `core/debugapi/`.
- Новое поле `Settings.SubscriptionUserAgent` в `internal/locale/settings.go`. Плумбится через `subscription.SubscriptionRequestSettings.UserAgent` и `applySubscriptionRequestHeaders` — пустое значение → fallback на `configtypes.BuildSubscriptionUserAgent()`. Поле `UIService.SettingsWindow` singleton добавлено рядом с `WizardWindow`.
- Новый файл `core/debugapi/settings_endpoints.go` для мутаций launcher-level preferences (отдельный namespace от `/state/*`). Первый житель: `/settings/user-agent`.
