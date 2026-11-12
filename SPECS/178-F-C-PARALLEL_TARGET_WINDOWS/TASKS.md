# TASKS 178

## Этап 1 — реестр окон Конфигуратора
- [x] `core/uiservice/ui_service.go`: заменить `WizardWindow`/`FocusOpenChildWindows` реестром по ключу (см. PLAN).
- [x] `core/template`: `WizardWindowKey(TargetSpec)`.
- [x] `ui/configurator/configurator.go`: поиск/регистрация по ключу, subject в заголовке, снять `SetTailscaleRemoteStateDirRoot` из открытия.
- [x] Потребители: `ui/wizard_overlay.go`, `ui/components/click_redirect.go`, `core/elevation.go`, всё по грепу `WizardWindow`.

## Этап 2 — заголовок Профайлера
- [x] `ui/traffic/window.go`: `WindowDeps.Subject`, заголовок с целью и при записи.
- [x] `internal/traffic/profiler.go`: `FormatRecordingTitle` с subject.
- [x] `ui/traffic_bootstrap.go` → `Local`; `ui/machine_profiler.go` → `d.Name`.
- [x] Ключи locale + `bin/locale/ru.json`.

## Этап 3 — корень tailnet только в сборке
- [x] Проследить путь до `applyTailscaleStateDirectory`; выбрать вариант А/Б (PLAN).
- [x] `writeRemoteConfig`, разбор узлов окна машины, `RebuildMachineConfig` — корень из `model.Target.TailscaleStateDir` только на время операции.
- [x] Локальные сборки — с пустым корнем всегда.

## Этап 4 — закрытие
- [x] `go build ./...`; затронутые существующие тесты по имени.
- [x] `docs/release_notes/upcoming.md` (EN+RU), `docs/ARCHITECTURE.md` (реестр окон).
- [x] `IMPLEMENTATION_REPORT.md`.
- [x] Коммит явным списком путей в `develop`. CI `run_mode=tests` — не запускался (запускает владелец после волны).
