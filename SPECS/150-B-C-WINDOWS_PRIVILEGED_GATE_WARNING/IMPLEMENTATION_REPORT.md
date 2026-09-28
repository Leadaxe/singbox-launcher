# IMPLEMENTATION REPORT 150

## Сделано

- Нарушение защиты звена — `*platform.ProtectionError` (путь, причина,
  команда `icacls`, признак предка); причина называет права по именам.
  `errors.Is(err, platform.ErrProtectedPathMissing)` работает как раньше:
  отсутствие звена эта ошибка не представляет.
- `OpenPrivilegedCoreLog` проверяет `<ProgramData>\sing-box-lxd` и `logs\`
  без предков; случай issue #137 (запись «Все» в `C:\ProgramData`) больше
  не останавливает старт.
- Отказ гейта копии (missing, outdated, unsafe) и любая ошибка открытия
  `classic.log` показывают диалог с «Run anyway». Флаги сессии —
  `ProcessService.runAnywayCopy` / `runAnywayLog` (`atomic.Bool`).
- Любая ошибка `OpenPrivilegedCoreLog` при наличии UI идёт в диалог лога
  (раньше ошибки, кроме отсутствия `logs\`, шли в общий «Failed to start»):
  так «Run anyway» доступен при любой причине. Без UI — прежнее поведение.
- Тексты missing/unsafe копии больше не утверждают «only»; фраза «If the
  command below reports the same problem…» стала отдельным ключом и
  показывается только рядом с командой copy/install при нарушении прав.

## Проверки

- `go build ./...` (macOS) — успешно.
- `GOOS=windows GOARCH=amd64 go vet ./internal/platform/`,
  `GOOS=windows GOARCH=arm64 go build ./internal/platform/` — успешно.
- Пакет `core` под Windows с macOS не собирается (Fyne/go-gl требует cgo и
  mingw); типы Windows-файлов `core` проверены `gopls check` с
  `GOOS=windows` — без ошибок.
- `go run ./tools/l10n/l10n_check --strict`, `hardcoded_check --strict` — чисто.

## Не проверено на живой Windows

- Диалоги «Core copy …» и «Core log folder …» с кнопкой Run anyway.
- Команды `icacls` (`/setowner`, `/remove:g`, `/inheritance:d /remove:g`)
  на реальных DACL.
- Старт после Run anyway: ядро из `bin\`, вывод в лог лаунчера, WARN на
  повторных стартах сессии.
