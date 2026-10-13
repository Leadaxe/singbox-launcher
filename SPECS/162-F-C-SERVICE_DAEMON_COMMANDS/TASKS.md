# TASKS 162

Один исполнитель, ветка `develop`, коммиты своими путями
(`git commit -m "…" -- <пути>`). Локально: `go build ./...` и только
именованные тесты волны (`-run`). Полный прогон — CI
`gh workflow run ci.yml --ref develop -f run_mode=tests` в конце.
Общий код без `min`/`max`, `slices`, `maps` (go1.20, Win7).

## A — клиент и core (без UI)
- [x] A1 `internal/lxdclient/client.go`: `InfoData` + goos/goarch/platform/init_system/service.
- [x] A2 `internal/lxdclient/commands.go`: `Command`, ошибки, `CommandsCtx`/`CommandCtx`/`RunCommandCtx`; тест `TestClientCommands` (httptest: 200 список, 404 список, 202, 409, 404 имя, `exit_code: null`).
- [x] A3 `core/core_build.go`: суффикс после `lx.N` только `rc`; дополнить `TestCompareCoreVersion`.
- [x] A4 `core/service_recipes.go`: `ServiceInitOpenRC`, `ParseInitSystem`, поля `Platform/ServiceKnown/Service` в `ServicePlatform`, `Unsupported` в рецептах для openrc, `ServiceStepEditDaemonJSON`, `CommandsRecipe` + `TestCommandsRecipe`.
- [x] A5 `core/core_downloader.go`, `core/core_download_target.go`: ассет по `platform`, fallback — таблица; `ui/service_core_download.go` передаёт platform.
- [x] A6 `core/services/lxd_remote_registry.go`: `RemotePassport` +5, `RemoteHealth` +5, `healthCtx` обновляет платформу и init из паспорта (лог), `SetInitSystem` принимает openrc/launchd/scm, `PassportPlatformKnown`, `Commands/Command/RunCommand/CommandsWithin`.
- [x] A7 `core/daemon_manager.go`: `DaemonAdminClient()` (тег darwin/windows!386).
- Проверка: `go build ./...`, `go test ./internal/lxdclient -run TestClientCommands -count=1`, `go test ./core -run 'TestCompareCoreVersion|TestCommandsRecipe' -count=1`.

## B — окно Service
- [x] B1 `ui/service_model.go`: `serviceCommander`, `Commander()` в `serviceSource`, поля снапшота (`CommandsState`, `Commands`, платформа), `serviceTabNone`, `serviceTabSuggested`, диагноз с текстом заголовков, `serviceSnapshotEqual`.
- [x] B2 `ui/service_source_remote.go`: init/платформа из паспорта (InitChoice=false), commander через реестр, `Reconnect` через `HealthWithin`; `ui/machine_heartbeat.go`: `commands[id]`, опрос только при открытом окне.
- [x] B3 `ui/service_source_local.go`: то же для local через `DaemonAdminClient`; на Windows `Commander()` nil → `not available`.
- [x] B4 `ui/service_window.go`: Accordion вместо AppTabs, раскрытие по диагнозу/явному запросу, сохранение раскрытой секции при перестройке, строка `Troubleshooting · …`, селектор init только при `InitChoice`, `service:false` в строке платформы.
- [x] B5 `ui/service_commands.go` (новый): `serviceCommandBar`, `watch`, `serviceUpdateRun`, подтверждение update, `check-update` исход из лога.
- [x] B6 `ui/service_tabs.go`: подсказка про `update` в Core, «Install the service» первым при `service:false`, openrc-заглушка, `buildSuggestedCommandsTab`, тексты переходов «Open …».
- [x] B7 `ui/machine_edit_window.go`, `ui/machine_host_window.go`: селекторы неактивны при паспорте; mismatch только без паспорта.
- Проверка: `go build ./...`; вёрстку смотрит владелец (стенд RouteRich: паспорт, `echo`; `update` — по слову владельца).

## C — закрытие
- [x] C1 `bin/locale/ru.json` по `go run ./tools/l10n/l10n_check --strict`; `go run ./tools/l10n/hardcoded_check --strict`; `go run ./tools/win7guard`.
- [x] C2 `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md`, `docs/TROUBLESHOOTING.md` (Daemon: commands, Update), `RELEASE_NOTES.md`.
- [x] C3 `IMPLEMENTATION_REPORT.md`, CI `run_mode=tests`, папка → `162-F-C-…`.
