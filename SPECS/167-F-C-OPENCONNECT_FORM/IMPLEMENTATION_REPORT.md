# IMPLEMENTATION_REPORT 167

Дата: 15.10.2026. Ветка worktree `worktree-spec-167-openconnect-form`, ff-слияние в `develop`. Контракт **1.1.122**.

## Коммиты

| SHA | Что |
|---|---|
| 1f184aa8, ed4617e9, b036771e, 3d1d9dfb | SPEC, PLAN, TASKS, REGISTRY_DESIGN, CODEMAP; поправки по карте кода (§119, общая модель формы, правило материализации, порт SSO без ключа узла) |
| b0a30684 | Контракт 1.1.122: схема (wildcard `*`, `form`, роль `variant`, `fields_unchecked` снят), записи `openconnect.json` (описанные поля формы + `*` в корне/tls/token/fortinet_host_check) и `openvpn-client.json` (→ `*`), четыре кода, корпус тела (9 кейсов), авторский кейс, корпус формы `corpus/form/`, README, TASKS_LXBOX §119, PARSING_PRINCIPLES |
| e8208f07 | Движок: `core/config/registry` (Wildcard, FieldForm/FormSection, `Form()`, `form.go`: FormState/FieldVisible/FormRequired/ActiveFormOption, RoleVariant), `nodeflow` (сквозной проход без unknown_key, слот `*` при эмиссии, ветки FieldsUnchecked сняты), `node_materialize.go` (`server`/`server_port` только описанным полям), gendocs, тесты `TestWildcardNestedStrict`, `registry_body_test.go`, раннер `TestContractCorpusForm`, `go generate` |
| 948ea838, 0e152f95 | Второй фактор и SSO под демоном: `core/services/openconnect_status.go` (типы, кеш, Submit/Cancel), `core/openconnect_status.go` (выбор локального/удалённого ядра, колбэки), `core/backend_daemon_openconnect.go` (супервизор стрима, стоп при Unimplemented), `core/services/lxd_remote_openconnect.go`, `core/openconnect_sso_callback.go` (loopback-слушатель на время диалога, порт из префикса или 8020), `ui/servers_openconnect_auth.go` (диалоги формы и браузера), статус в строке узла (`ui/proxy_list_panel.go`), `ru.json` |
| e553a874 | Форма: `ui/configurator/business/openconnect_form_model.go` (модель по `reg.Form`, FormFromBody/BodyFromForm/Verdict), `ui/configurator/dialogs/add_server_openconnect.go` + пункт OpenConnect в `add_server_dialog.go`, правка узла `ui/configurator/tabs/source_openconnect_edit.go` в Settings окна узла, роль `variant` в `TransportLabel`, `ru.json` |
| f813432e | `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md` §6.0 и §11.10 |

## Проверки

- `go build ./...` — ok.
- Именованные: `TestWildcardNestedStrict`, `TestEmitEveryScheme`, `TestContractCorpusBody`, `TestContractCorpusAuthored`, `TestContractCorpusEmitRoundTrip`, `TestContractCorpusForm`, `TestContractCorpusURI`, `TestContractCorpusDirection`, `TestRegistryBodyStructure`, `TestRegistryBodyCoversCoreSchema`, `TestRegistryMapperSection`, `TestRegistryMapperCodesDeclared`, `TestRegistryWarningsHaveCauseAndFix`, `TestCorpusBodiesPassSingboxCheck` (ядро 1.14.3-lx.17), `TestEngineVsXrayCorpus`, `TestEngineVsConfCorpus` — ok.
- Полный прогон — CI `run_mode=tests` после слияния.

## Отступления от SPEC/REGISTRY_DESIGN (осознанные)

- Кейс `openconnect_endpoint` получил info `tls_insecure` в ожидании: §5 проекта сам требует этот advisory у `tls.insecure`, тело кейса не менялось.
- `auth_group` скрыт при fortinet через `not_in` (HoldsOn не подставляет default).
- `mtu`: `min: 0` у ядра (uint32); `user_agent`: `\x00` вместо `\u0000` (RE2).
- Корпус формы несёт поле `missing` сверх §6 проекта.
- Форма: поле Name отдельно не рисуется (общее поле Tag диалога); отпечаток под радиогруппой; порядок строк внутри блока — по макету SPEC, принадлежность к блоку — из реестра. Поля `token.*` и клиентского сертификата описаны в реестре с `form`, но форма их не рисует (нет подписей) и в теле не трогает — задел на вторую волну.
- Удалённые машины: диалоги работают и при классике на этом компьютере (канал даёт демон машины), как в SPEC §2.5.

## Не сделано / хвосты

- Живая проверка: форма, диалог второго фактора и SAML-вход на реальном FortiGate не проверялись — показ владельцу.
- Сохранение нового пароля из формы челленджа в узел — только текстовая подсказка.
- SSO в cookie/header-режиме (AnyConnect) — вне SPEC; кнопка «Fetch fingerprint», импорт FortiClient, `token.*` в форме — вторая волна.
- Сверка «описанное ⊆ ядро» для openconnect/openvpn-client пуста: типов нет в `core_schema.draft.json`.
- Генератор доков у обязательных полей внутри `token` пишет «узел снимается» (старая формулировка).
- Ответ LxBox на §119.
