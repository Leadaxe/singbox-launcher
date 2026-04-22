# План: 041 — ATOMIC_FILE_WRITES

## 1. Паттерн

```go
tmp := path + ".tmp"
os.WriteFile(tmp, data, mode)
os.Rename(tmp, path)
// cleanup tmp on rename error
```

## 2. Сайты применения (done)

| Файл                                      | Функция                  | Суффикс |
|-------------------------------------------|--------------------------|---------|
| `core/config/updater.go`                  | `WriteToConfig`          | `.tmp`  |
| `ui/wizard/business/saver.go`             | `SaveConfigWithBackup`   | `.swap` |
| `internal/locale/settings.go`             | `SaveSettings`           | `.tmp`  |

## 3. Gitignore (done)

- `*.tmp` — было.
- `*.swap` — добавлено (`aea001a`).

## 4. TODO (not done)

- `ui/wizard/business/state_store.go` — state.json атомарно.
- macOS `internal/platform/privileged_darwin.go` — pid-file.
- Fault-injection unit-тест (удалить tmp между WriteFile и Rename).
- Документация — в docs/ARCHITECTURE.md упомянуть паттерн как инвариант.
