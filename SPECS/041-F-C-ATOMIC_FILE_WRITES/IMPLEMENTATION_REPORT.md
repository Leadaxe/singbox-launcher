# Реализация: 041 — ATOMIC_FILE_WRITES

**Коммиты:** `acfa0f5` (config.json в updater + wizard), `f28d8db` часть 3 (settings.json), `aea001a` (gitignore `*.swap`). Ветка `night-work`, 2026-04-22.
**Спека написана ретроспективно.**

## Что сделано

### `acfa0f5` — config.json

- `core/config/updater.go WriteToConfig`: `os.WriteFile(configPath+".tmp", …)` → `os.Rename(tmp, configPath)`. Cleanup на ошибке rename.
- `ui/wizard/business/saver.go SaveConfigWithBackup`: после вызова `services.BackupFile(configPath)` — `os.WriteFile(swapPath, finalText, 0o644)` + `os.Rename(swapPath, configPath)`.

### `f28d8db` — settings.json (внутри pack'а)

- `internal/locale/settings.go SaveSettings`: stage `.tmp` + rename. Cleanup на ошибке.

### `aea001a` — gitignore

- `*.swap` добавлено в `.gitignore` рядом с существующим `*.tmp`. Предотвращает подхват stuck temp-файлов через `git add -A` если процесс умер между WriteFile и Rename.

## Что не сделано (TODO)

- `state.json` в визарде (`ui/wizard/business/state_store.go`) — всё ещё прямой `os.WriteFile`.
- pid-file на macOS privileged path (мало-байтовый, малая вероятность бага, но паттерн унифицировать надо).
- Fault-injection тест (tdd для следующих сайтов).
- Документация в `docs/ARCHITECTURE.md`.

## Проверка

- `go test ./...` — зелёные.
- Ручной тест: нет (fault-injection непросто воспроизвести на живом лаунчере).
