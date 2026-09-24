# SPEC 137 · План реализации

Норма — SPEC.md. Ветка — `spec-137-classic-root-owned` (worktree от
`spec-136-daemon-root-owned`), не пушится до команды владельца.

## Архитектурные решения

| Решение | Почему |
|---|---|
| Гейт — чистая функция от `daemonServiceLayout` SPEC 136 и пути ядра лаунчера | та же раскладка и тот же тестовый каркас (`newTestServiceLayout`), что у классификатора службы; прод — `/Library/…`, uid 0 |
| Гейт переиспользует `checkRootOwnedChain`, `errDaemonCopyMissing`, кэш `daemonServiceHashes`, `daemonServiceCommand` | одна цепочка владения и один кэш sha на процесс — классификатор службы и гейт не читают ядро дважды |
| Тело root-шелла и сборка argv — в `internal/platform` (`privilegedStartBody`, `PrivilegedStartArgs`), канонический путь копии — в `core` | платформенный код — в platform (CONSTITUTION §1.5); путь копии остаётся константой SPEC 136, platform получает его аргументом |
| `/usr/bin/env -i PATH=…` перед `/bin/sh` | окружение лаунчера (PATH, `BASH_FUNC_*%%`) не доходит до root-шелла; `env` делает `exec`, PID для `Wait4` тот же |
| kill / pkill / rm — прямые вызовы утилит через AEWP, без шелла | нечему раскрываться, некого искать по `PATH` |
| pid-файл удаляет лаунчер сам | его и пишет лаунчер от имени пользователя |
| Отказ гейта — сентинел `errPrivilegedCopyNotReady`, `Start` не показывает «Failed to start» | у отказа свой диалог с командой и Retry |
| Диалог — `internal/dialogs.ShowCommandRetry` с колбэками терминала и Retry | core не зависит от `ui`; терминал открывает `OpenTerminalWithCommand` SPEC 136 |
| Время жизни авторизации — константа `privilegedAuthReuse` | вариант Б включается одной правкой (SPEC §6) |

## Файлы

| Файл | Что |
|---|---|
| `core/classic_privileged_darwin.go` (новый) | вердикты гейта, `checkPrivilegedCoreCopy`, `privilegedCopyCommandFor`, `(ac) privilegedCoreCopyGate`, диалог, `notifyPrivilegedCopyAfterCoreUpdate` |
| `core/classic_privileged_other.go` (новый, `!darwin`) | заглушки гейта и уведомления |
| `core/classic_privileged_test.go` (новый, darwin) | `TestPrivilegedCoreCopyGate` |
| `core/process_service.go` | гейт перед AEWP, старт копии через `platform.StartPrivilegedCore`, удаление старого скрипта, pkill через `platform.KillPrivilegedByPattern`, сентинел в `Start` |
| `core/core_downloader.go` | шаг 6.7 — `notifyPrivilegedCopyAfterCoreUpdate` |
| `internal/platform/privileged_darwin.go` | `privilegedAuthReuse`, тело и argv старта, `StartPrivilegedCore`, kill по PID через `/bin/kill`, `KillPrivilegedByPattern`, `RemoveWithPrivileges`; `WritePrivilegedStartScript` удалён |
| `internal/platform/privileged_stub.go` | заглушки новых функций |
| `internal/platform/privileged_darwin_test.go` (новый) | `TestPrivilegedStartCommand` — `sh -n` тела, прогон argv с поддельным ядром и отравленным окружением |
| `internal/dialogs/dialogs.go` | `ShowCommandRetry`, общая оценка высоты текста |
| `ui/diagnostics_tab.go` | Kill через `platform.KillPrivilegedByPattern` |
| `ui/configurator/tabs/settings_tun_darwin.go` | очистка через `platform.RemoveWithPrivileges` |
| `bin/locale/ru.json` | новые ключи |
| `SPECS/136…/SPEC.md`, `SPECS/README.md`, `docs/DAEMON_AND_REMOTE*.md`, `docs/ARCHITECTURE*.md`, `docs/ARCHITECTURE_PACKAGES*.md`, `docs/release_notes/upcoming.md`, `RELEASE_NOTES.md` | документация |

## Этапы (коммиты)

| # | Что | Проверка |
|---|---|---|
| 1 | SPEC/PLAN/TASKS | — |
| 2 | Гейт на функциях SPEC 136, выбор команды, тест гейта; старт — через гейт на копию | `go build ./...`, `go test ./core/ -run TestPrivilegedCoreCopyGate` |
| 3 | Путь исполнения под root: `env -i` + постоянное тело, kill/pkill/rm без шелла, флаг времени авторизации, тест тела | `go test ./internal/platform/ -run TestPrivilegedStartCommand` |
| 4 | Диалог с Copy / Run in Terminal / Retry, WARN после обновления ядра, locale | `go build ./...`, `l10n_check --strict`, показать владельцу |
| 5 | Доки и заметки | `paths_guard`, `win7guard` |
