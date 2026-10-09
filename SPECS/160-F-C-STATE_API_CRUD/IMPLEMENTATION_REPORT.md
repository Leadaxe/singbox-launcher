# IMPLEMENTATION_REPORT 160

**Дата:** 09.10.2026. **Статус:** реализовано, полный прогон тестов — CI (`ci.yml`, `run_mode=tests`).

## Что сделано

### A. Импорт бэкапа — ключ `rules` (контракт 1.1.116)
- `core/backup`: `Parse` запоминает присутствие ключа `rules` (`rootKeyPresent`) у обоих форматов; `decodedFile.RulesPresent`; `applyDecoded` замещает `rules[]` только при нём, снятые считает (`ImportResult.ReplacedRules`, `RulesReplaced`) и называет кодом `backup_rules_replaced {count}`.
- Контракт: `registry/backup_warnings.json`, `schema/backup.schema.json`, BACKUP.md §9 (вводная, таблица полей, п. 7), README 1.1.116, `VERSION`, TASKS_LXBOX §113, `go generate`.
- UI: фраза для кода в `settings_backup.go`, перевод в `ru.json`. API: `applied.rules_replaced`, `applied.replaced_rules`.
- Тест `TestImportWithoutRulesKeyKeepsLocalRules` (1.0 без ключа, 1.0 с `[]`, 0.x без ключа).

### B. CRUD состояния
- `core/state/tags.go`: `TakenRootTags` (перенос из `core/backup/merge.go`, плюс `-auto`-двойники Направлений), `UniqueTag`.
- Новый пакет `core/stateedit`: `AddServers`/`ListServers`/`DeleteServer`, `KnownRuleTargets`/`AddRule`/`DeleteRule`, `AddDNSServer`/`DeleteDNSServer`/`AddDNSRule`/`DeleteDNSRule`. Разбор ввода повторяет ветки `parseSourceInput` Конфигуратора поверх `config.MaterializeServerNode`/`MaterializeVPNLinkNodes`/`ConvertWGConfText`. Удаление узла снимает NodeLink-ссылки (detour, хопы, члены и default групп) и строки `include` Направлений; ссылки по имени (цели правил, `route_final`, detour DNS, vars, `options.default`) перечисляются в `dangling`.
- `core/debugapi/state_crud_endpoints.go`: `GET/POST/DELETE /state/servers`, `POST/DELETE /state/rules`, `POST/DELETE /state/dns/servers`, `POST/DELETE /state/dns/rules`; общий хвост `commitStateEdit` (нормы записи → save → у локального состояния пересборка с `config_rebuilt`/`config_rebuild_error`); зеркала `/remote/machines/{id}/state/*` без пересборки. Идентификаторы — query-параметры (Win7/go1.20).
- `PATCH /state/dns` требует оба ключа; `PATCH /state/dns/rules` получил `guardStateSchema` и `ApplyRecordVars`.
- Тесты: `TestStateEditIncidentScenario` (core/stateedit), `TestStateCRUDEndpoints` (core/debugapi), кейс «только servers» в `TestPatchStateDNS_EmptyBodyDoesNotClear`.

### C. Журнал
- `core/debugapi/mutation_log.go`: middleware снаружи auth — каждый POST/PATCH/PUT/DELETE пишет в `singbox-launcher.log` строку `debugapi: <METHOD> <path?query> → <status> <≤400 символов ответа>`; тело запроса не пишется.

### D. Документация
- `docs/API.md`, `docs/API.ru.md` (State write, Backup, Remote, General rules, Source), `docs/ARCHITECTURE.md` (L2: `core/stateedit`), `docs/release_notes/upcoming.md`, `SPECS/README.md`.

## Решения, принятые по ходу (отличия от SPEC)
1. `skipped.line` — номер строки, не текст: ответ попадает в журнал, ссылки несут секреты.
2. Пресет без `num` встаёт на якорь шаблона, а не в пользовательскую зону.
3. `POST /state/rules`: пресет, уже стоящий среди правил, несортируемая голова, недоступный шаблон — 422 `field: ref`; `num < 1` — 422 `field: num`.
4. Каталог Tailscale снимается только у локального состояния (корень каталогов общий для машины).
5. Корневой узел уникализируется против полного набора известных целей (включая системные теги шаблона), как в UI.
6. `DELETE /state/servers` без `folder` трогает только `kind=server`; цепочки и группы — вне волны.
7. Журнал стоит снаружи auth — отклонённые 401 тоже видны.

## Вне рамок (решение владельца)
- Открытое окно Конфигуратора не перечитывает `state.json` после правки через API и при своём Save перепишет её. Варианты: подписка на `StateChanged` с перечитыванием при отсутствии несохранённых правок, либо отказ API в 409, пока окно открыто.
- CRUD подписок, папок, цепочек, Направлений.

## Проверка
- `go build ./...` — OK; `gofmt`, `go vet ./core/stateedit ./core/debugapi` — чисто.
- Именованные тесты зелёные: `TestImportWithoutRulesKeyKeepsLocalRules`, `TestStateEditIncidentScenario`, `TestStateCRUDEndpoints`, `TestHelpMethodsMatchHandlers`, `TestPatchStateDNS_EmptyBodyDoesNotClear`, `TestManifest*`, `TestRoundTripLossless`, `TestMergeServerTagUniquifiedAgainstRootSpace`.
