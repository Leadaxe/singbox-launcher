# PLAN — SPEC 184

| Файл | Изменение |
|---|---|
| `core/config/registry/mapper.go` | `SourceKind.Container` (`scheme`, `omit`, `set`) |
| `core/config/linkmap/source.go` | распаковщик `container_link` в `ClassifySource`, `UnwrapContainerDocument` |
| `core/config/subscription/parse_body.go` | распаковка документа-объекта перед классификацией тела |
| `core/config/subscription/source_loader.go` | `SubscriptionURLOfLink` — адрес подписки из отказа с параметром `subscription_url` |
| `ui/configurator/business/parser.go` | `classifyInputLines`: строка-указатель → подписка |
| `ui/configurator/business/node_form_labels.go` | тип «TrustTunnel» в `NodeFormSchemes`, подписи `health_check`, `idle_timeout` |
| `bin/locale/ru.json` | четыре ключа |
| `contract/registry/source_kinds.json` | вид `trusttunnel_endpoint_json` |
| `contract/registry/protocols/trusttunnel.json` | `body.form`, `form` у полей |
| `contract/registry/warnings.json` | `tt_subscription_only`: параметр `subscription_url`, тексты |
| `contract/schema/registry_mapper.schema.json` | `sourceKind.container` |
| `contract/corpus/body/trusttunnel/`, `contract/corpus/form/trusttunnel_form.json` | кейсы |
| `contract/docs/MAPPER_ENGINE.md` §2.1, `contract/README.md`, `contract/VERSION`, `contract/TASKS_LXBOX.md` §178 | норма, версия 1.1.176 |
| `core/config/subscription/tt_subscription_link_test.go` | тест волны |
| `docs/release_notes/upcoming.md`, `docs/ParserConfig.md`, `docs/Protocols.md` | документация |
