# Задачи: 041 — ATOMIC_FILE_WRITES

## Этап 1 — config.json

- [x] `core/config/updater.go`: stage `.tmp` + Rename.
- [x] `ui/wizard/business/saver.go`: stage `.swap` + Rename (после BackupFile).

## Этап 2 — settings.json

- [x] `internal/locale/settings.go`: stage `.tmp` + Rename.

## Этап 3 — gitignore

- [x] `*.tmp` — было.
- [x] `*.swap` — добавлено.

## Этап 4 — TODO

- [ ] `ui/wizard/business/state_store.go` — state.json атомарно.
- [ ] Pid-file на macOS privileged path.
- [ ] Fault-injection unit-тест.
- [ ] Добавить в `docs/ARCHITECTURE.md` раздел «Инвариант: атомарные записи пользовательских файлов».
