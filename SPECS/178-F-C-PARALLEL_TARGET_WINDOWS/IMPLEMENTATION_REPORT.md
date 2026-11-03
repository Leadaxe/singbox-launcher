# IMPLEMENTATION REPORT 178

**Итог:** Конфигуратор — по одному окну на цель (`local` / `machine:<id>`), окна Local и машин живут рядом; заголовки Конфигуратора и Профайлера называют цель; корень состояния tailnet машины больше не глобал и действует только в сборке своей цели. `go build ./...` зелёный; вживую окна не проверялись — показ владельцу.

## Что сделано

- **Реестр окон.** `core/uiservice/ui_service.go`: поле `WizardWindow` и `FocusOpenChildWindows` заменены реестром `wizardWindows map[string]WizardWindowEntry{Window, FocusChildWindows}` с методами `WizardWindowFor`, `SetWizardWindow`, `RemoveWizardWindow`, `AnyWizardWindow` (локальное приоритетно), `LocalWizardOpen`, `WizardWindowCount`. `ShowMainWindowOrFocusWizard` поднимает `AnyWizardWindow`.
- **Ключ.** `core/template/target.go`: метод `TargetSpec.WizardWindowKey()` (`local` | `machine:<id>`) — одна функция для поиска и регистрации. Методом, а не свободной функцией: рядом с `MachineIDOrEmpty`, на котором он и построен.
- **Конфигуратор.** `ui/configurator/configurator.go`: `showConfigWizardFor` ищет окно по ключу цели (`focusWizardWindow`), в том числе повторно в асинхронной ветке после скачивания шаблона; `buildWizardWindow` принимает `subject`, заголовок `locale.Tf("Config Wizard — %s", subject)` (`Local` / `machine.Name` из записи реестра), регистрирует окно по ключу, `SetOnClosed` снимает только свой ключ. `SetTailscaleRemoteStateDirRoot` из открытия и set/restore из `RebuildMachineConfig` убраны.
- **Потребители.** `ui/wizard_overlay.go` — overlay, пока `WizardWindowCount() > 0`; `ui/components/click_redirect.go` — `AnyWizardWindow()` + его `FocusChildWindows`; `core/elevation.go` — «Switch to proxy mode» блокируется только при `LocalWizardOpen()`. R5: `watchExternalStateChanges` уже фильтрует по `(Target, MachineID)` презентера — без изменений.
- **Профайлер.** `ui/traffic/window.go`: `WindowDeps.Subject`; начальный заголовок и тик берут `tprof.FormatRecordingTitle(subject, session)` (`internal/traffic/profiler.go`, единственный вызов — сигнатура изменена, строки через `locale.Tf`). `ui/traffic_bootstrap.go` → `locale.T("Local")`, `ui/machine_profiler.go` → `d.Name`.
- **R10.** Поиск родительского окна карточек (`parentWindowOf`, `perProcessView.parentWindow`) искал окно по префиксу «Traffic Profiler» — с локализованным заголовком он перестал бы находить окно, а с двумя профайлерами нашёл бы чужое. Теперь общий `isProfilerWindowOf(deps, w)`: заголовок своей цели без записи целиком или он же + пробел (хвост записи).
- **Locale.** В `bin/locale/ru.json`: `Config Wizard — %s`, `Traffic Profiler — %s`, `Traffic Profiler — %s ⚡ Recording · %02d:%02d`.

## Корень tailnet: вариант А (параметр)

Глобал `SetTailscaleRemoteStateDirRoot` / `TailscaleRemoteStateDirRoot` удалён целиком. Протяжка оказалась короткой:

- `configtypes.ParserConfig.TailscaleRemoteStateDirRoot string \`json:"-"\`` — поле сборочной проекции, не данных;
- `ParseAndPreview` (единственный производитель `model.GeneratedEndpoints`; через него идут заход на Rules/Preview, `ensureOutboundsParsed`/`parseOutboundsForSave`, `writeRemoteConfig`, `RebuildMachineConfig`) ставит его из `wizardbusiness.TailscaleRemoteRootOf(model)` — пусто у Local, `Target.TailscaleStateDir` у машины;
- `GenerateOutboundsFromParserConfig` передаёт его в новую `EmitNodeJSONsFor(node, root)` → `GenerateEndpointJSONFor` → `applyTailscaleStateDirectory`. `EmitNodeJSONs` / `GenerateEndpointJSON` остались с прежней сигнатурой и пустым корнем (тесты и прочие вызовы не тронуты);
- локальные сборки (`core/config_service.go`, `core/rebuild_snapshot.go`) поле не ставят — корень пуст всегда (R12);
- превью окна (вкладка JSON источника, Copy JSON папки, Add server) зовут `EmitNodeJSONsFor` с тем же `TailscaleRemoteRootOf(model)` — раньше они получали корень из того же глобала, теперь — из своего окна.

Вариант Б (scoped set/restore под мьютексом) не понадобился: правка уложилась в одно поле и одну обёртку без каскада сигнатур.

## Тесты

- `core/config/tailscale_state_dir_test.go` (`TestTailscaleStateDirRemoteRootWins`) ходил через удалённый глобал — переведён на прямую передачу корня; прогнан по имени — `ok`. Новых тестов нет (CONSTITUTION §8.1).

## Не проверено

- Вживую: два Конфигуратора рядом, overlay, «Switch to proxy mode», заголовки профайлеров, A4 (грепом `state_directory` в двух `config.json`) — за владельцем.
- CI `run_mode=tests` не запускался.
