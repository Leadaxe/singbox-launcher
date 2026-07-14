# 084.1 — UI-кнопка/визард WARP-генератора

**Статус:** реализовано (UI не прогнан через GUI — нужен smoke). **Дата:** 2026-07-12.

## Сделано
- `ui/configurator/dialogs/warp_dialog.go` — `ShowAddWarpDialog(presenter, onURI)`:
  диалог (транспорт WireGuard/AmneziaWG, random endpoint, WARP+ license) →
  `warp.Register` на горутине (fyne.Do для UI) → `wireguard://`-URI в onURI.
- `ui/configurator/tabs/source_tab.go` — кнопка «Add WARP» в header вкладки Sources,
  колбэк = `applyAddedSources` (тот же путь, что ручная вставка URI).
- Локали `wizard.warp.*` + `wizard.source.button_add_warp` в en.json + ru.json (паритет).

## Паттерн (в стиле проекта)
Мирроринг `ShowGetFreeVPNDialog`: сеть на горутине, UI через `fyne.Do`, ошибки — `dialog.ShowError`.
Регистрация не пишет state напрямую — только общий Add-путь. `warp.NewClient(nil)` уважает
HTTP_PROXY окружения (регистрация через активный туннель, если CF недоступен напрямую).

## Проверка
- `go build ./ui/...`, `go vet` — OK; `go test ./internal/locale/` (паритет) — OK.
- Backend `core/warp` уже device-verified (живая регистрация + sing-box check).
- **GUI-smoke (перед релизом):** открыть Sources → Add WARP → создать → узел появляется в списке.

---

## 084.2 — Полный конфигуратор (все AWG-поля + пресеты + MASQUE)

- `core/warp/obfuscation.go`: `QuicParams` расширен до всех AWG-полей (s1-s4, h1-h4, i1-i5),
  WARP-safe дефолты; `buildAWGFields` эмитит полный набор (masquerade id/ip/ib ИЛИ явные i1-i5).
- `core/warp/presets.go`: `ObfuscationPresets` (WARP default / masquerade dns/stun/sip / aggressive),
  `MasqueSNIPool`.
- `core/warp/masque.go`: `ToMasqueURI()` (masque:// share URI).
- `ui/configurator/dialogs/warp_dialog.go`: конфигуратор с выбором транспорта WireGuard/MASQUE,
  obfuscate + пресет + Advanced (все поля, masquerade-блок, endpoint/SNI/domain с 🎲), MASQUE-секция
  (h3/h2, sni, idle, keep-alive). Структура — порт warp_wizard_screen (LxBox).
- Локали wizard.warp.* (en+ru, паритет).

**Проверка:** build/vet/test OK; **E2E живьём**: WARP(ip=dns) → sing-box check rc.17 OK;
MASQUE(h3) → sing-box check lx.3 OK. GUI-smoke на пользователе.
