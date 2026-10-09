# TASKS 160

## A. Импорт: ключ `rules` (контракт 1.1.116) — сделано 09.10
- [x] Флаг присутствия `rules` у `Backup`/`Backup10`, выставляется в `Parse` (`rootKeyPresent`)
- [x] `decodedFile.RulesPresent`; applyDecoded замещает `rules[]` только при нём; `backup_rules_replaced {count}`
- [x] `ImportResult.RulesReplaced/ReplacedRules`; ответ `/backup/import` — `applied.rules_replaced`, `applied.replaced_rules`
- [x] Реестр `backup_warnings.json`, схема, BACKUP.md §9 п. 7 (+ таблица полей, вводная §9), README 1.1.116, VERSION, TASKS_LXBOX §113, `go generate ./contract/...`
- [x] Фраза UI (`settings_backup.go`) + перевод `ru.json`
- [x] Тест `TestImportWithoutRulesKeyKeepsLocalRules` (1.0 без ключа / `[]` / 0.x без ключа)

## B. CRUD состояния
- [x] `core/state`: `TakenRootTags`, `UniqueTag` (перенос из `core/backup/merge.go`, backup переключить)
- [x] `core/stateedit/servers.go`: разбор ввода (ссылки, `.conf`, `vpn://`, JSON sing-box; подписки → skipped), `AddServers` (корень/папка, уникализация тегов, дубли по Origin), `DeleteServer` (чистка NodeLink в источниках и `include` Направлений, отчёт висячих имён, каталог Tailscale)
- [x] `core/stateedit/rules.go`: `KnownRuleTargets`, `AddRule` (валидация, цель, num, сорт, синк пресета), `DeleteRule` (num/name/ref, неоднозначность, голова оси)
- [x] `core/stateedit/dns.go`: `AddDNSServer`, `DeleteDNSServer` (перенос `dns_final`, сброс resolver), `AddDNSRule`, `DeleteDNSRule`
- [x] `core/stateedit/stateedit_test.go` — интеграционный сценарий инцидента + удаление с чисткой ссылок
- [x] `core/debugapi/state_crud_endpoints.go`: `GET/POST/DELETE /state/servers`, `POST/DELETE /state/rules`, `POST/DELETE /state/dns/servers`, `POST/DELETE /state/dns/rules`; rebuild + `config_rebuilt`; зеркала `/remote/machines/{id}/state/*`
- [x] `PATCH /state/dns/rules`: `guardStateSchema` + `ApplyRecordVars`
- [x] `PATCH /state/dns`: оба ключа обязательны (сделано 09.10; тест `TestPatchStateDNS_EmptyBodyDoesNotClear` дополнен кейсом «только servers»)
- [x] `core/debugapi/state_crud_endpoints_test.go`; `TestHelpMethodsMatchHandlers` зелёный

## C. Журнал
- [x] `core/debugapi/mutation_log.go`: middleware для POST/PATCH/PUT/DELETE → `debuglog.InfoLog` (метод, путь, код, ≤400 символов ответа); подключить в `routes()`

## D. Документация и закрытие
- [x] `docs/API.md`, `docs/API.ru.md` — State write (новые точки, примеры v8), Backup (норма `rules`, новые поля, рецепт «добавить узел»), Source
- [x] `docs/ARCHITECTURE.md` — пакет `core/stateedit`
- [x] `docs/release_notes/upcoming.md` (EN + RU)
- [x] `SPECS/README.md` — строка 160
- [x] `go build ./...`; новые тесты по имени; CI `gh workflow run ci.yml --ref develop -f run_mode=tests`
- [x] IMPLEMENTATION_REPORT.md; папка → `160-F-C-…`
