# IMPLEMENTATION REPORT 186

Дата: 2026-11-22. Статус: реализовано, не закоммичено, ждёт показа владельцу и прогона CI.

## Вердикты по находкам

| № | Находка | Вердикт | Доказательство |
|---|---------|---------|----------------|
| 1 | Пресеты по ОС лаунчера | подтверждена, спящая (в шаблоне платформы только у `messengers`: windows/darwin/linux) | `core/template/loader.go` — `filterPresetsByPlatform(allPresets, runtime.GOOS)` |
| 1a | params по ОС лаунчера | ложная | `core/build/build.go` `effectiveConfig` пересчитывает от цели |
| 2 | Узлы по локальному ядру | подтверждена | `core/config/node_build_gate.go` `coreInfoForBuild`; пробы — `core/controller.go` |
| 3 | `@vars` из локального state | подтверждена | `core/config_service.go` `GenerateOutboundsFromParserConfig` → `core/config/varsubst.go` `loadStateSettingsVars` |
| 4 | `.srs` legacy-правил в `bin/rule-sets` | ложная: ветка для машины недостижима | `ui/configurator/business/create_config.go` — `ctx.Route.Rules = nil` при непустых v6-правилах |

## Что сделано

- `core/template/loader.go`: `TemplateData.AllPresets`, `ForTarget`.
- `ui/configurator/models/wizard_model.go`: `SyncTemplateToTarget`; вызовы в `configurator.go`, `presentation/presenter_state.go`, `presentation/presenter_target.go`.
- `core/config_service_context.go`: `buildContextFromState` берёт шаблон под цель состояния.
- `core/config/configtypes/types.go`, `core/config/models.go`: `BuildTarget`, поля `ParserConfig.BuildTarget`, `ParsedNode.BuildTarget`.
- `core/config/node_build_gate.go`, `outbound_generator.go`, `chain_generator.go`, `chain_nodes.go`, `endpoint_schemes.go`: гейт узлов и цепочек от цели.
- `core/config/varsubst.go`: `BuildVarSubstituterFor`; `core/config_service.go` — подстановщик от цели.
- `ui/configurator/business/parser.go`, `node_pool.go`: `BuildTargetOf(model)`.
- Тест `core/config/remote_target_build_test.go` — `TestRemoteTargetBuildUsesTargetInputs`; `core/config/corpus_singbox_check_test.go` — новая сигнатура гейта.
- Доки: `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md`.

## Проверки

- `go build ./...` — проходит.
- `go test ./core/config/ -run 'TestRemoteTargetBuildUsesTargetInputs' -count=1` — зелёный.
- Полный прогон — в CI, не запускался. Вживую на роутере не проверено.

## Изменение поведения, о котором надо знать

Для машины гейт по тегам сборки ядра и проба цепочек больше не применяются (раньше считались по ядру лаунчера). Узел naive/tailscale или цепочка, которых ядро лаунчера не умеет, раньше молча снимались из конфига машины с записью в отчёте; теперь остаются, и решает локальный `check` и ядро машины.

## Хвосты

- Паспорт демона не несёт теги сборки ядра (`/admin/info` форка) — с ними гейт по тегу для машины станет точным.
- Превью узла в окне машины (вкладка JSON источника, `TailscaleBuildAdditions`) показывает гейт локального ядра.
- `SetTargetPlatform` (`presentation/presenter_target.go`) собирает `TargetSpec` без `TailscaleStateDir` — после смены платформы на вкладке Target путь состояния tailnet машины теряется до переоткрытия окна. Не проверено вживую, найдено чтением.
