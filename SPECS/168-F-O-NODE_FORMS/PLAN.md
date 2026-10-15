# PLAN 168

Работа в worktree `.claude/worktrees/spec-168-node-forms` (ветка `spec-168-node-forms` от develop), слияние в develop `--ff-only`. Два исполнителя последовательно: этап 1–2 (контракт и движок), затем этап 3 (UI) — UI читает новые записи реестра.

## Этап 1 — контракт 1.1.123 (данные)
- `contract/schema/registry_body.schema.json`: `form` у ref-полей схемы и у полей `tls.json`/вариантов `transports.json`; `form.fixed` у тела; `form.exclude_variants` у ref-поля; `form` у `array` с `items: object` (правка `[0]`).
- `contract/registry/protocols/{vless,hysteria2,trojan,wireguard,naive,masque,socks,http}.json`: `body.form` (blocks, choices, fixed) и `form` полей по SPEC §2.5.
- `contract/registry/tls.json`: `form` у `enabled`, `server_name`, `insecure`, `alpn`, `utls.fingerprint`, `reality.enabled`, `reality.public_key` (required при reality), `reality.short_id`.
- `contract/registry/transports.json`: `form` у полей вариантов (ws: path, headers.Host; grpc: service_name; http: host, path; httpupgrade: host, path; xhttp: host, path, mode); у ws/httpupgrade `headers` — поле `Host` + `"*"`.
- Корпус формы `contract/corpus/form/<scheme>_form.json` на 8 схем (видимые/обязательные пути, активный вариант, варианты транспорта, `fixed`, `peers.0.*`), README формата — дополнить.
- `contract/VERSION` 1.1.123, `contract/README.md` (changelog + строка нормы «Форма»), `contract/TASKS_LXBOX.md` §120, `go generate ./contract/...`.

## Этап 2 — движок формы (Go)
- `core/config/registry/registry.go`: перенос `Form` ref-поля в `resolveSection`/`resolveNamedRef`/вариантные ref; `FormSection.Fixed`, `FieldForm.ExcludeVariants`; наследование `block` от ref-поля.
- `core/config/registry/form.go`: `walkForm` — отбор по `forbidden_for` схемы, обход `Variants` по дискриминатору, массив-объект → сегмент `0`; `bodyValue` с числовым сегментом; `ActiveFormOption` — самое длинное совпадение; хелпер записи по пути с числовым сегментом (для модели).
- `core/config/registry_body_test.go` (`TestRegistryBodyStructure`): пути `fixed`/`choices`/`visible_when` описаны, блоки объявлены, `form` у поля общей секции допустим, `exclude_variants` ⊆ values.
- `core/config/contract_form_test.go` (`TestContractCorpusForm`): новые кейсы проходят.

## Этап 3 — UI
- `ui/configurator/business/openconnect_form_model.go` → `node_form_model.go` (`NodeFormModel(scheme)`), `node_form_labels.go` (подписи по пути + переопределения схемы).
- `ui/configurator/dialogs/add_server_openconnect.go` → `node_form_view.go`; `add_server_dialog.go`: `modeForm` + схема, список типов по SPEC §2.1, удаление ручных веток SOCKS/HTTP/WG и сборщиков URI (+ их тесты).
- `ui/configurator/tabs/source_openconnect_edit.go` → `source_form_edit.go`, подключение в `source_edit_window.go` для схем с формой; у wireguard блок `amnezia` в окне узла не рисуется (`source_awg_edit.go` остаётся).
- `bin/locale/ru.json` — все новые ключи; `l10n_check --strict`.

## Этап 4 — доки и закрытие
- `docs/release_notes/upcoming.md` (EN/RU), `RELEASE_NOTES.md` (UX), `docs/ARCHITECTURE.md` (модель формы), `IMPLEMENTATION_REPORT.md`.
- Слияние `--ff-only` в develop, CI `run_mode=tests`; папка → `168-F-C-…` после приёмки владельцем.
