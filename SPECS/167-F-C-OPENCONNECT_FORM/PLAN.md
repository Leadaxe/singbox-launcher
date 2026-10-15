# PLAN 167

Работа идёт в worktree `spec-167-openconnect-form` (ветка `worktree-spec-167-openconnect-form`, ff от develop), слияние в develop `--ff-only` по завершении. Исполнители читают `CODEMAP.md` и `REGISTRY_DESIGN.md`; код — точечно по якорям карты.

## Этап 1 — контракт 1.1.122 (данные)
- `contract/schema/registry_body.schema.json`: `wildcardField`, `fields` с `"*"`, удалить `fields_unchecked`, `formSection`/`fieldForm`, роль `variant`.
- `contract/registry/protocols/openconnect.json` — по `REGISTRY_DESIGN.md` §5; `openvpn-client.json` → `order: ["*"]`.
- `contract/registry/warnings.json` — четыре кода.
- Корпус: `contract/corpus/body/singbox/openconnect_*` (8), `contract/corpus/authored/hard_openconnect_token_without_secret`, `contract/corpus/form/openconnect_form.json` (+ README формата).
- `contract/VERSION` → 1.1.122; `contract/README.md` строка changelog; `contract/TASKS_LXBOX.md` §119; `docs/PARSING_PRINCIPLES.md` §3 (wildcard).

## Этап 2 — движок реестра и nodeflow (Go)
- `core/config/registry/registry.go`: `Wildcard` у секции/поля, `FieldForm`/`FormSection`, `Form(scheme)`, роль `variant`, пропуск `*` в `Field()`/`fieldsAtBodyPath`/resolveSection; удалить `FieldsUnchecked`.
- `core/config/nodeflow/sanitize.go`, `emit.go`, `gate.go`, `coerce.go`: wildcard по норме; удалить ветки `FieldsUnchecked`.
- `core/config/linkmap/*`, `contract/tools/gendocs/*`: обходы `Fields` пропускают `*`; генератор выводит `*`, `form`, `variant`.
- Тесты: `TestWildcardNestedStrict` (переименованный `fields_unchecked_test.go`), правки `registry_body_test.go`, новый раннер `contract/corpus/form` (`core/config/contract_form_test.go`), списки схем без изменений (openconnect уже есть).
- `go generate ./contract/...`.

## Этап 3 — подпись узла и форма (UI)
- `ui/configurator/business/config_node_labels.go`: ветка роли `variant` в `TransportLabel`.
- Общая модель `ui/configurator/business/openconnect_form_model.go`: раскладка и соответствие «поле ↔ путь тела» по `reg.Form("openconnect")` (блоки, видимость, выбор сертификата, обязательность, диапазоны, advisory, secret); пустое поле ключ не пишет.
- Конструктор в `ui/configurator/dialogs/add_server_dialog.go` (новая вкладка/ветка OpenConnect) и правка узла `ui/configurator/tabs/source_openconnect_edit.go` — оба на общей модели; `bin/locale/ru.json`.
- `core/config/node_materialize.go`: `server`/`server_port` дописываются только если описаны у схемы (этап 2).
- Подключение в меню Add server (`source_tab.go`, `folder_add_nodes.go`) и в Settings узла.
- Подсказка классического режима на узле.

## Этап 4 — второй фактор и SSO под демоном
- `core/backend_daemon_openconnect.go`: `superviseOpenConnect`, стрим статуса, `OpenConnectStatus(tag)`, `OpenConnectSubmitForm`, `OpenConnectSubmitBrowser`, `OpenConnectCancel`.
- `core/services/lxd_remote_openconnect.go`: то же для удалённых машин; общий интерфейс в `core/openconnect_status.go` (по образцу `tailscale_status.go`, цель = TailscaleTarget-аналог).
- `core/openconnect_sso_callback.go`: временный слушатель `127.0.0.1:<port>` на время диалога.
- UI: `ui/servers_openconnect_auth.go` — диалог формы и диалог браузера; статус в строке узла; показ по новому `challenge.id`; ru.json.

## Этап 5 — доки и закрытие
- `docs/release_notes/upcoming.md` (EN/RU), `docs/ARCHITECTURE.md` (поток челленджа), `docs/Protocols*.md` при необходимости, `IMPLEMENTATION_REPORT.md`, папка → `167-F-C-…`.
- Слияние `--ff-only` в develop, push, CI `run_mode=tests`.

## Проверки
`go build ./...` после каждого этапа; именованные тесты этапа 2 по имени; UI — показ владельцу; CI в конце.
