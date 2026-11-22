# TASKS 186

- [x] T1 `core/template/loader.go`: `AllPresets`, `ForTarget`
- [x] T2 модель визарда: `SyncTemplateToTarget` + вызовы после установки цели; `buildContextFromState`
- [x] T3 `configtypes`: `BuildTarget`, поля проекции
- [x] T4 гейт узлов и цепочек от цели (`node_build_gate.go`, `outbound_generator.go`, `chain_*.go`, `endpoint_schemes.go`)
- [x] T5 `varsubst.go`: `BuildVarSubstituterFor`; `config_service.go`
- [x] T6 визард: `BuildTargetOf`, `parser.go`, `node_pool.go`
- [x] T7 тест `TestRemoteTargetBuildUsesTargetInputs`; `go build ./...`
- [x] T8 `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md`, IMPLEMENTATION_REPORT
- [ ] T9 показ владельцу; полный прогон в CI (`gh workflow run ci.yml --ref develop -f run_mode=tests`)
