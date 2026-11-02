# TASKS 175

- [x] T1 Сборка: `taildrop_directory` в `applyTailscaleStateDirectory`, перенос в `daemonEndpointsTailscaleStateDir`
- [x] T2 Статус: `CanShareFiles`, `CanReceiveFiles`
- [x] T3 `core/services/taildrop.go`: типы и RPC
- [x] T4 `taildropClient` у демона и машины, `core/taildrop.go`
- [x] T5 UI: секция Files, Send file…, Save/Delete/Cancel
- [x] T6 Locale (`bin/locale/ru.json`), `docs/release_notes/upcoming.md`
- [x] T7 Контракт 1.1.147: note `tailscale.json`, TASKS_LXBOX §143
- [x] T9 Автосохранение в Downloads: галка узла, фоновая подписка (`core/taildrop_autosave.go`)
- [x] T10 «Allow Taildrop» у узла, по умолчанию ВЫКЛЮЧЕНО (решение владельца 02.11.2026): сборка эмитит опцию «выключено», кроме явного «включено» в теле; выключено — нет Files, Send file…, автосохранения и служебных файлов; контракт с нормой для обеих сторон; release notes о смене поведения. Сделано: `disable_taildrop`, контракт 1.1.150, гейт с пре-релизами
- [ ] T8 Обмен десктоп ↔ LxBox на стенде (узел стенда — ключ даёт владелец)
