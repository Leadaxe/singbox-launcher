# TASKS 185

## Этап 1
- [x] T1 `srs_downloader.go`: `RefreshSRS`, проверка содержимого, mtime
- [x] T2 `core/build/rule_set_policy.go`: политика, `RuleSetFetches`, `DropUncachedSrsPaths`
- [x] T3 `core/srs_refresh.go` + вызовы в `auto_update.go`
- [x] T4 `rebuild.go`: докачка при сборке, pre-start; `config_service_context.go`
- [x] T5 Deploy: `PreDeploy` + `RefreshMachineRuleSets`

## Этап 2
- [x] T6 state: три поля записи правила, клон, провоз через модель визарда
- [x] T7 сборка: эмиссия remote, `download_detour` в графе, переименование
- [x] T8 UI: блок «Rule-set updates», дата в подсказке srs, переводы
- [x] T9 шаблон: три переменные
- [x] T10 контракт: VERSION, changelog, BACKUP.md, schema, vars.json, корпус, TASKS_LXBOX; backup-ключи

## Закрытие
- [x] T11 тест `TestRefreshSRS`; `go build ./...`; раннеры контракта по имени
- [x] T12 `docs/ARCHITECTURE.md`, `docs/release_notes/upcoming.md`, `RELEASE_NOTES.md`, IMPLEMENTATION_REPORT
- [ ] T13 показ владельцу (UI, проверка fswatch на живом ядре)
