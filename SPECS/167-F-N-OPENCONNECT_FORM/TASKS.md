# TASKS 167

## Этап 1 — контракт 1.1.122
- [x] T1 `contract/schema/registry_body.schema.json` — wildcard `*`, `form`/`formSection`, роль `variant`, снять `fields_unchecked`
- [x] T2 `contract/registry/protocols/openconnect.json` по `REGISTRY_DESIGN.md` §5; `openvpn-client.json` → `*`
- [x] T3 `contract/registry/warnings.json` — `openconnect_flavor_invalid`, `openconnect_fingerprint_invalid`, `openconnect_mtu_invalid`, `openconnect_insecure_crypto`
- [x] T4 корпус тела (8 кейсов), авторский кейс, корпус формы `contract/corpus/form/` с README
- [x] T5 `contract/VERSION` 1.1.122, `contract/README.md`, `contract/TASKS_LXBOX.md` §119, `contract/docs/PARSING_PRINCIPLES.md`

## Этап 2 — движок
- [x] T6 `core/config/registry/registry.go` — `Wildcard`, `FieldForm`/`FormSection`/`Form()`, роль `variant`, удалить `FieldsUnchecked`
- [x] T7 `core/config/nodeflow/sanitize.go`, `emit.go`, `gate.go`, `coerce.go` — норма wildcard, снять ветки
- [x] T8 `core/config/linkmap/*`, `contract/tools/gendocs/*` — пропуск `*`, вывод `form`/`variant`; `go generate ./contract/...`
- [x] T9 тесты: `TestWildcardNestedStrict`, `registry_body_test.go`, раннер `contract/corpus/form`

## Этап 3 — UI: подпись и форма
- [ ] T10 `ui/configurator/business/config_node_labels.go` — роль `variant`
- [ ] T11 `ui/configurator/business/openconnect_form_model.go` (общая модель по `reg.Form`), ветка OpenConnect в `ui/configurator/dialogs/add_server_dialog.go`, правка узла `ui/configurator/tabs/source_openconnect_edit.go`, `bin/locale/ru.json`
- [ ] T12 подключение в меню Add server и Settings узла; подсказка классического режима

## Этап 4 — второй фактор и SSO
- [x] T13 `core/backend_daemon_openconnect.go`, `core/services/lxd_remote_openconnect.go`, `core/openconnect_status.go` — стрим, submit, cancel
- [x] T14 `core/openconnect_sso_callback.go` — loopback-слушатель на время диалога
- [x] T15 `ui/servers_openconnect_auth.go` — диалоги формы и браузера, статус узла, ru.json

## Этап 5 — доки и закрытие
- [ ] T16 `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md`, `IMPLEMENTATION_REPORT.md`
- [ ] T17 `go build ./...`, именованные тесты, слияние в develop, push, CI `run_mode=tests`, папка → `167-F-C-…`
