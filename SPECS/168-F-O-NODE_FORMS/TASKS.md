# TASKS 168

## Этап 1 — контракт 1.1.123
- [x] T1 `contract/schema/registry_body.schema.json` — form у ref-полей и общих секций, `fixed`, `exclude_variants`, form у массива объектов
- [x] T2 `contract/registry/protocols/{vless,hysteria2,trojan,wireguard,naive,masque,socks,http}.json` — `body.form` и `form` полей
- [x] T3 `contract/registry/tls.json`, `contract/registry/transports.json` — `form` полей, `headers.Host` + `*`
- [x] T4 корпус `contract/corpus/form/*_form.json` (8 схем), README корпуса формы
- [x] T5 `contract/VERSION` 1.1.123, `contract/README.md`, `contract/TASKS_LXBOX.md` §120, `go generate ./contract/...`

## Этап 2 — движок
- [x] T6 `core/config/registry/registry.go` — перенос form ref-поля, `Fixed`, `ExcludeVariants`, наследование block
- [x] T7 `core/config/registry/form.go` — forbidden_for, Variants, `[0]` массива, активный вариант по длине совпадения, запись по пути
- [x] T8 `core/config/registry_body_test.go`, `core/config/contract_form_test.go`

## Этап 3 — UI
- [x] T9 `ui/configurator/business/node_form_model.go`, `node_form_labels.go` (из openconnect_form_model.go)
- [x] T10 `ui/configurator/dialogs/node_form_view.go` (из add_server_openconnect.go), `add_server_dialog.go` — modeForm, список типов, удаление ручных веток и их тестов
- [x] T11 `ui/configurator/tabs/source_form_edit.go` (из source_openconnect_edit.go), `source_edit_window.go`
- [x] T12 `bin/locale/ru.json`, `l10n_check --strict`

## Этап 4 — доки и закрытие
- [x] T13 `docs/release_notes/upcoming.md`, `RELEASE_NOTES.md`, `docs/ARCHITECTURE.md`, `IMPLEMENTATION_REPORT.md`
- [ ] T14 `go build ./...`, именованные тесты, слияние в develop, CI `run_mode=tests`
