# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

---

## EN

### Highlights
- **Processes rule — Match by path:** In the Add/Edit Rule dialog, for rule type "Processes" you can enable "Match by path" to match by process path (regex) instead of process name. Use the Simple mode with `*` as wildcard (e.g. `*/steam/*`) or the Regex mode for full regular expressions. Stored as `process_path_regex` in the config (sing-box 1.10+).
- **Restart button:** A Restart button (🔄) is available on the Core dashboard between Start and Stop. It kills the sing-box process so the watcher restarts it; the UI briefly shows "Restarting..." and button state feedback (Start on, Stop off) before returning to Running.
- **Wizard save:** Saving in the config wizard only writes files and runs Update (no sing-box restart). Config is validated with `sing-box check` against a temporary file (`config-check.json`) before writing to `config.json`; on validation failure the user sees an error and the existing config is not overwritten. Clash API config is reloaded from `config.json` only when sing-box is started.
- **Linux capabilities dialog (issue #34):** The "Linux capabilities required" / "Linux Capabilities" dialog now shows the setcap command in a selectable field and adds a "Copy" button to copy it to the clipboard.

---

## RU

### Основное
- **Правило Processes — Match by path:** В диалоге добавления/редактирования правила для типа «Processes» можно включить «Match by path» и задавать сопоставление по пути процесса (regex), а не по имени. Режим Simple: подстановка `*` как «любая последовательность» (например `*/steam/*`). Режим Regex: полные регулярные выражения. В конфиг записывается `process_path_regex` (sing-box 1.10+).
- **Кнопка перезапуска:** На дашборде Core между кнопками Start и Stop добавлена кнопка перезапуска (🔄). Она завершает процесс sing-box, после чего вотчер снова его поднимает; в интерфейсе кратко показывается «Restarting...» и смена состояния кнопок (Start активна, Stop неактивна), затем снова «Running».
- **Сохранение в визарде:** При сохранении в визарде выполняются только запись файлов и Update; перезапуск sing-box убран. Конфиг валидируется через `sing-box check` по временному файлу `config-check.json` до записи в `config.json`; при ошибке валидации пользователь видит ошибку и рабочий конфиг не перезаписывается. Настройки Clash API перечитываются из `config.json` только при запуске sing-box.
- **Диалог Linux capabilities (issue #34):** В диалоге «Linux capabilities required» / «Linux Capabilities» команда setcap выводится в выделяемом поле и добавлена кнопка «Copy» для копирования в буфер обмена.

