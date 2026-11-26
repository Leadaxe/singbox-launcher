# PLAN 186

## 1. Пресеты по ОС цели

- `core/template/loader.go`: `TemplateData.AllPresets` — пресеты до платформенного фильтра; `Presets` остаётся отбором под ОС лаунчера (локальное поведение прежнее). Метод `(*TemplateData).ForTarget(TargetSpec)` — копия с `Presets`, отобранными по ОС цели.
- `ui/configurator/models/wizard_model.go`: `SyncTemplateToTarget()` — `TemplateData = TemplateData.ForTarget(Target)`.
- Вызов после каждой установки цели модели: `ui/configurator/configurator.go`, `presentation/presenter_state.go`, `presentation/presenter_target.go`.
- `core/config_service_context.go` (`buildContextFromState`): `td.ForTarget(TargetSpecFromState(s))` — план `.srs` машины (`srs_refresh.go`) видит её пресеты.

## 2. Гейт узлов по ядру цели

- `core/config/configtypes/types.go`: `BuildTarget{GOOS, CoreVersion, SettingsVars}`; поле сборочной проекции `ParserConfig.BuildTarget` и `ParsedNode.BuildTarget` (nil = своя машина), по образцу `TailscaleRemoteStateDirRoot`.
- `core/config/node_build_gate.go`: `coreInfoFor(target)` / `coreCapabilitiesFor(target)`; `gateBodyForCore` принимает цель.
- `core/config/outbound_generator.go`: узловой гейт от `parserConfig.BuildTarget`; перед эмиссией цель ставится на узел и его звенья.
- `core/config/chain_generator.go`, `chain_nodes.go`: `chainSupportedFor(target)` — для машины «поддерживает» (теги неизвестны).
- `core/config/endpoint_schemes.go`: вызов гейта с nil (превью, вне задачи).
- `ui/configurator/business/parser.go`: `BuildTargetOf(model)`; ставится в `ParseAndPreview`; `node_pool.go` — в пул узлов.

## 3. `@vars` цели

- `core/config/varsubst.go`: `BuildVarSubstituterFor(layout, overrides)`; `BuildVarSubstituterFromDisk` — обёртка с локальным состоянием.
- `core/config_service.go` (`GenerateOutboundsFromParserConfig`): при `parserConfig.BuildTarget != nil` — подстановщик от `BuildTarget.SettingsVars`.

## Тест

`core/config/remote_target_build_test.go` — `TestRemoteTargetBuildUsesTargetInputs`.
