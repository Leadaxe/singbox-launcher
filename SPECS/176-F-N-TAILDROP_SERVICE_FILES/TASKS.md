# TASKS 176

- [x] T1 Ресинк daemonpb (lx.30), `TaildropFile.SenderID`
- [x] T2 Кодек служебных файлов `core/services/taildrop_service.go` + тест
- [x] T3 Наблюдатель входящих: служебные файлы своего ядра + автосохранение одной подпиской
- [x] T4 Разрешения (`settings.json`) и выполнение clipboard / open_url, уведомления
- [x] T5 UI: Accept clipboard / Accept links / Send text… в меню устройства
- [x] T6 Locale (`bin/locale/ru.json`), release notes, контракт (`current.go`)
- [x] T7 Прогон с LxBox на стенде — 03.11.2026, Mac-Lex-449 (ядро демона lx.25, ключ по senderName) ↔ lxbox-emu-a: A без разрешения — служебный удалён без выполнения, обычный остался; B с разрешением — буфер Мака и ссылка в браузере; C1/C2 без разрешения у LxBox — пропуск с удалением; C3/C4 — буфер эмулятора и уведомление «Link from mac-lex-449». Галки меню и окно «Send text…» вживую не нажимались (разрешение выдано правкой settings.json, снято после прогона), senderID на lx.30 не проверен.
