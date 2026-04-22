# SPEC: Атомарные записи пользовательских файлов

Задача: защитить `config.json` и `settings.json` от обнуления на kill -9 / обесточивании / crash'е процесса посреди записи. Заменить прямые `os.WriteFile(target, ...)` на паттерн «stage → rename».

**Статус:** реализовано. Коммиты:
- `acfa0f5` — `config.json` (parser-update + wizard save).
- `f28d8db` (часть pack'а) — `settings.json`.
- `aea001a` — `*.swap` в `.gitignore`.

**Спека написана ретроспективно.**

---

## 1. Проблема

### 1.1 До изменений

`os.WriteFile` — не атомарен. Последовательность:

1. Open(O_TRUNC) → файл усекается в нуль.
2. Write(data).
3. Close.

Если процесс умирает между шагами 1 и 3 (kill -9 пользователем, reboot, power loss, OS OOM-killer, паника в любой горутине — вся программа валится) — на диске остаётся файл размером 0 или с обрезанным содержимым.

Страдают:

- **`config.json`** — sing-box при следующем старте не парсится, лаунчер выдаёт «Config Error», пользователь без ручной работы (восстановить из `.bak`, заново пройти wizard) не запустится. На macOS с TUN ещё требуется админ-пароль, чтобы стопнуть застрявший процесс.
- **`settings.json`** — лаунчер стартует, но теряет язык, ping-URL, Clash-group, все настройки.

### 1.2 Цель

Все writes user-data файлов — атомарны. Крах в момент записи оставляет **либо** старый файл (целым), **либо** новый (целым). Промежуточных состояний нет.

---

## 2. Требования

### 2.1 Паттерн

```go
tmp := target + ".tmp"  // или ".swap"
if err := os.WriteFile(tmp, data, mode); err != nil {
    return fmt.Errorf("write temp: %w", err)
}
if err := os.Rename(tmp, target); err != nil {
    _ = os.Remove(tmp)
    return fmt.Errorf("rename: %w", err)
}
```

- `os.Rename` атомарен:
  - POSIX (Linux, macOS) — да по стандарту.
  - Windows NTFS — да через `MoveFileEx` с `MOVEFILE_REPLACE_EXISTING`, Go 1.22+ использует это по умолчанию.
- На ошибку rename — удалить temp, чтобы не копить мусор.

### 2.2 Применение

**`core/config/updater.go`** — `WriteToConfig`:
- Суффикс `.tmp`.
- Вызывается из планового парсера при каждом Update.

**`ui/wizard/business/saver.go`** — `SaveConfigWithBackup`:
- Суффикс `.swap` (чтобы отличить от auto-update path при debug'е).
- **Сохраняется** вызов `services.BackupFile(configPath)` перед writes — даёт дополнительно `.bak` для ручного восстановления «я зря сохранил».
- Атомарный write поверх backup'а — двойная защита.

**`internal/locale/settings.go`** — `SaveSettings`:
- Суффикс `.tmp`.
- Вызывается на любой OnChanged-handler чекбоксов / селекторов в UI.

### 2.3 .gitignore

- `*.tmp` — уже было в `.gitignore`.
- `*.swap` — добавлено в `aea001a`.
- Оба — если процесс умер в момент rename, файл останется; `git add -A` не должен подхватить.

---

## 3. Инварианты

1. **Ни одна user-data запись не делает прямой `os.WriteFile(target, ...)`** кроме first-time creates в тестовых фикстурах.
2. **Temp-файл создаётся в той же директории что target** — необходимо для атомарности rename (cross-device move не атомарен).
3. **На ошибке rename — удалить temp.** Иначе накапливается мусор при репликации проблемы.
4. **Режим файла передаётся temp'у**, а не target'у. `os.Rename` наследует permissions от temp — поэтому `0o644` / `DefaultFileMode` задаём именно на WriteFile, не на rename.

---

## 4. Не покрыто (scope limits)

- **`state.json` в wizard'е** (`ui/wizard/business/state_store.go`) — не тронут этой спекой. Аналогичный паттерн надо добавить **TODO** (коммит написан отдельной веткой, чтобы не мешать).
- **Log-файлы** — запись append-only, атомарность не нужна.
- **Binary downloads** (`core_downloader.go`) — временный архив скачивается в .tmp-путь, затем распаковка в bin/ одной операцией. Там другой паттерн (extract-to-dir), отдельной атомарности не требует.
- **pid-files** на macOS privileged path — мало-байтовые, проблема минимальна, тоже можно TODO.

---

## 5. Тесты

- Unit-тесты на writer'ы — есть частично (для config updater'а через integration-тест wizard'а).
- **TODO:** целевой fault-injection тест — `os.Remove(tmp)` между WriteFile и Rename, проверить что target не изменился.

---

## 6. Совместимость

- Никаких breaking changes. Ни читатели, ни writers, ни external tools не видят разницы.
- Размер и permissions target'а — те же.

---

## 7. Открытые вопросы

- Нужен ли **`fsync`** между WriteFile и Rename для гарантий persistence на POSIX? В Go `os.WriteFile` закрывает fd, но не fsync'ит. Для истинной «crash-safe» записи надо:
  ```go
  f, _ := os.Create(tmp)
  f.Write(data)
  f.Sync()   // ← ensure dirty pages flushed
  f.Close()
  os.Rename(tmp, target)
  // опционально: fsync directory
  ```
  Для наших файлов (config / settings) пока hdd-timing'а не критичен, а SSD в основном у всех — отложим на момент реального bug report'а.
- **Windows-specific nuance**: `os.Rename` в Go 1.22+ использует `MoveFileEx` с `MOVEFILE_REPLACE_EXISTING`, но если target открыт в другом процессе (наше же sing-box держит read-lock?) — может упасть. На практике sing-box читает config при старте и не держит, поэтому OK. TODO: check на Windows.
