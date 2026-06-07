# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **Windows 7: stale TUN adapters cleaned automatically.** On launcher startup (when sing-box is not already running), accumulated `singbox-tun*` WinTun ghosts from prior sessions are removed — no manual Device Manager cleanup after upgrade. Also runs after each VPN Stop/Restart (SPEC 065).

### Technical / Internal
- SPEC 065 follow-up: aggressive cleanup mode (prefix + Wintun only, no CM_PROB_PHANTOM gate) for taskkill stop path; startup hook `CleanupStaleTunAtStartUtil` in `main.go`.

## RU
### Основное
- **Windows 7: старые TUN-адаптеры чистятся сами.** При запуске лаунчера (если sing-box ещё не работает) снимаются накопившиеся ghost `singbox-tun*` с прошлых сессий — после обновления не нужен ручной Device Manager. Плюс очистка после каждого Stop/Restart VPN (SPEC 065).

### Техническое / Внутреннее
- SPEC 065: aggressive cleanup (только префикс + Wintun), startup-хук `CleanupStaleTunAtStartUtil` в `main.go`.
