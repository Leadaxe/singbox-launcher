# PLAN 178

## Архитектура

### Реестр окон Конфигуратора (`core/uiservice/ui_service.go`)

Поле `WizardWindow fyne.Window` + `FocusOpenChildWindows func()` заменяются на реестр:

```go
type WizardWindowEntry struct {
    Window            fyne.Window
    FocusChildWindows func() // дочерние окна этого Конфигуратора (View/Edit/Rule)
}
// ключи: "local" | "machine:<id>"
wizardWindows map[string]WizardWindowEntry   // доступ только из UI-потока

func (u *UIService) WizardWindowFor(key string) (WizardWindowEntry, bool)
func (u *UIService) SetWizardWindow(key string, e WizardWindowEntry)
func (u *UIService) RemoveWizardWindow(key string)
func (u *UIService) AnyWizardWindow() (WizardWindowEntry, bool) // локальное приоритетно
func (u *UIService) LocalWizardOpen() bool
func (u *UIService) WizardWindowCount() int
```

Ключ строится одной функцией `WizardWindowKey(target wizardtemplate.TargetSpec) string` рядом с `TargetSpec` (в `core/template`), чтобы `machineTargetSpec` и реестр не разошлись.

### Конфигуратор (`ui/configurator/configurator.go`)

- `showConfigWizardFor`: вместо `UIService.WizardWindow != nil` → `WizardWindowFor(key)`; убрать `SetTailscaleRemoteStateDirRoot`.
- `buildWizardWindow`: получает `subject string` (имя для заголовка), регистрирует окно по ключу, `SetOnClosed` снимает только свой ключ. `FocusOpenChildWindows` кладётся в entry.
- `ShowConfigWizardForMachine` передаёт `machine.Name`; `ShowConfigWizard` — `locale.T("Local")`.
- Асинхронная ветка (скачивание шаблона) повторно проверяет реестр по ключу перед `buildWizardWindow`.

### Потребители

- `ui/wizard_overlay.go`: `WizardWindowCount() > 0`.
- `ui/components/click_redirect.go`: `AnyWizardWindow()` → Show/RequestFocus + `FocusChildWindows`.
- `core/elevation.go`: `LocalWizardOpen()`.
- Остальные упоминания найти грепом `WizardWindow` по репо (включая `ui/app.go`, трей, тесты).

### Профайлер

- `ui/traffic/window.go`: `WindowDeps.Subject string`; `NewWindow(title)` и `refreshTitle` берут `profilerTitle(subject, session)`.
- `internal/traffic/profiler.go`: `FormatRecordingTitle(s)` → `FormatRecordingTitle(subject string, s *Session)` либо оставить старую и добавить новую — выбрать по числу вызовов (грепнуть, включая тесты).
- `ui/traffic_bootstrap.go` (локальный) и `ui/machine_profiler.go` (машина) заполняют `Subject`.
- Локализация: строки заголовка через `locale.T`/`locale.Tf`, ключи в `bin/locale/ru.json`.

### Корень tailnet

- Проследить путь от `applyTailscaleStateDirectory` (`core/config/outbound_generator.go:713`, вызов `GenerateEndpointJSON` в :761) вверх до точек входа парсера, которыми пользуется Мастер (`parseOutboundsForSave`, обновление превью/Rules) и `BuildRemoteConfig`/локальная сборка.
- Вариант А (предпочтительный): корень как поле опций разбора/сборки (`ParserConfig` или аргумент функции-входа), глобал `SetTailscaleRemoteStateDirRoot`/`TailscaleRemoteStateDirRoot` удалить вместе с его использованием в `RebuildMachineConfig`.
- Вариант Б (если А раздувает дифф): `config.WithTailscaleRemoteStateDirRoot(root, func() error)` — set под мьютексом, defer restore; звать из `writeRemoteConfig`, `parseOutboundsForSave`/парсера окна и `RebuildMachineConfig`. Локальные пути ничего не ставят (корень пуст по умолчанию и всегда восстанавливается).
- Решение зафиксировать в IMPLEMENTATION_REPORT.

## Файлы

- `core/uiservice/ui_service.go`
- `core/template/*.go` (функция ключа)
- `ui/configurator/configurator.go`
- `ui/wizard_overlay.go`, `ui/components/click_redirect.go`, `core/elevation.go`, прочие по грепу `WizardWindow`
- `ui/traffic/window.go`, `internal/traffic/profiler.go`, `ui/traffic_bootstrap.go`, `ui/machine_profiler.go`
- `core/config/endpoint_schemes.go`, `core/config/outbound_generator.go` и путь разбора (по варианту)
- `ui/configurator/presentation/presenter_save.go`
- `bin/locale/ru.json`
- `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md` (абзац про реестр окон)

## Проверки

- `go build ./...`.
- Тесты: если существующие тесты ссылаются на `WizardWindow`/`FormatRecordingTitle` — починить и прогнать только их по имени. Новых юнит-тестов на формат заголовка не писать (CONSTITUTION §8.1).
- Полный прогон — CI `gh workflow run ci.yml --ref develop -f run_mode=tests` после коммита.
