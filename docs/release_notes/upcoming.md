# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **Servers tab can connect to remote Clash API endpoints (SPEC 064).** Gear ⚙ button in the tab header opens a small dialog where you set host / port / secret and connect to any Clash-API-compatible server: sing-box on a home router (e.g. RouteRich on `192.168.10.1`), mihomo on a VPS through ssh-tunnel, another launcher instance on the same machine. Status badge shows `🏠 Local` or `🌐 host:port`. Tab stays accessible even when the local sing-box isn't running, as long as remote override is active. Ephemeral (RAM-only) — resets on launcher restart. HTTP only for MVP.
- **Debug API reference doc** — `docs/API.md` ships a complete endpoint reference + curl cookbook for the headless control plane (24 endpoints across health/info, state read/write, actions, traffic profiler, snapshot). Replaces ad-hoc lookups in SPEC 038. Linked from both READMEs.

### Technical / Internal
- SPEC 064 implementation: new `ui/clash_remote.go` (RemoteOverride storage + EffectiveClashAPIConfig resolver + generation counter for drop-stale in refresh goroutines + NormalizeHost helper). All 9 callsites in `ui/clash_api_tab.go` migrated from `ac.APIService.GetClashAPIConfig()` to `EffectiveClashAPIConfig(ac)`. Tab availability rule now `running || hasOverride`.
- README's "28 endpoints" claim corrected to **24** — actual count verified against `core/debugapi/` handler registrations.

## RU
### Основное
- **Вкладка Servers подключается к удалённым Clash API (SPEC 064).** Кнопка-шестерёнка ⚙ в шапке таба открывает диалог с полями host / port / secret — подключаемся к любому Clash-API-совместимому серверу: sing-box на роутере (например RouteRich на `192.168.10.1`), mihomo на VPS через ssh-tunnel, другой инстанс лаунчера на той же машине. Badge в шапке показывает `🏠 Local` или `🌐 host:port`. Tab остаётся доступным даже если локальный sing-box не запущен — пока активен remote override. Ephemeral (только в памяти) — сбрасывается при рестарте лаунчера. HTTP only для MVP.
- **Документация Debug API** — `docs/API.md`: полный референс endpoint'ов + curl cookbook для headless-слоя (24 endpoint'а в группах health/info, state read/write, actions, traffic profiler, snapshot). Заменяет разрозненный поиск по SPEC 038. Залинкована из обоих README.

### Техническое / Внутреннее
- SPEC 064 реализация: новый `ui/clash_remote.go` (RemoteOverride storage + EffectiveClashAPIConfig resolver + generation counter для drop-stale в refresh-goroutine'ах + NormalizeHost helper). Все 9 callsite'ов в `ui/clash_api_tab.go` мигрировали с `ac.APIService.GetClashAPIConfig()` на `EffectiveClashAPIConfig(ac)`. Tab availability rule теперь `running || hasOverride`.
- Заявление в README про «28 endpoints» исправлено на **24** — реальный счётчик сверен с handler-регистрациями в `core/debugapi/`.
