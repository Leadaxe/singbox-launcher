# TASKS — SPEC 131 · Единый конвейер добавления узла

Нормы — `SPEC.md`; адреса — `CODEMAP.md` (на `d0e5aca3`); расхождения —
`DRIFT.md`; поля ядра — `CORE_SCHEMA.md` / `core_schema.draft.json`.
Каждая волна — отдельный набор коммитов в develop, старый код живёт до замены.

## W0 — ТЗ

- [x] SPEC.md (решения владельца, конвейер, реестр, state/контракт, UI, генерация, волны, приёмка)
- [x] CODEMAP.md — карта кода и 28 ловушек
- [x] CORE_SCHEMA.md + core_schema.draft.json — поля тел на теге `v1.14.1-lx.4`, lx_only от базы форка
- [x] DRIFT.md — расхождения Go/Dart с обоснованиями + прогоны на пине lx.4 (§9)
- [x] TASKS.md

## W1 — контракт (1.1.0, аддитивно)

Реестр:
- [x] Общие суб-схемы `body` в `registry/tls.json` (tls/utls/reality/ech), `registry/transports.json` (6 транспортов + xmux), новые `registry/multiplex.json`, `registry/dialer.json` — из `core_schema.draft.json`; атрибуты по SPEC §4 (`type`, `values`, `required`, `secret`, `tristate`, `all_or_nothing`, `lx_only`, `min_core`, `platform`, `desc_en`, `desc_ru`, `impl`)
- [x] Секция `body` в каждом `registry/protocols/<scheme>.json` (15 схем + chain, group как есть) с `order` в порядке структур ядра
- [x] `allowed_for`/`forbidden_for` у TLS-полей для naive (16 фатальных + 6 игнорируемых, `CORE_SCHEMA.md` §3.9) — код `tls_field_unsupported_naive`
- [x] `conflicts`/`requires`/`forbidden_when` из `CORE_SCHEMA.md` «конфликты» (27 пунктов) и `DRIFT.md`
- [x] `on_invalid` и коды для каждого поля с ограничением (enum/format/len) — по `DRIFT.md`, целевое = вариант LxBox с обоснованием; там, где обоснование против — пометка `decision_pending` и вопрос владельцу
- [x] `maps_to` у каждого `uri.query.<param>` и `userinfo`; правила значений переезжают из `uri.*` в `body.*`, в `uri.*` остаются синтаксис и алиасы имён
- [x] `note` → `impl` (перенос без потерь), `desc_en`/`desc_ru` — новые короткие
- [x] `warnings.json`: `title_en/title_ru/text_en/text_ru` для всех 61 + новые коды (`unknown_key`, `type_invalid`, `alias_shadowed`, `field_conflict`, `tls_field_unsupported_naive`, `flow_with_transport`, `partial_object_defaulted` — сверить с существующими, дублей не плодить)
- [x] `schema/registry_body.schema.json` — JSON-схема секции `body` (для линтера обеих сторон)
- [x] Линтер реестра (Go-тест): каждый `code` объявлен, каждый `ref` существует, `order` покрывает `fields`, каждое поле `core_schema` на теге пина отражено или помечено `skip`

Документы:
- [x] `docs/CANON.md` §6 (`warnings[] = [{code, path, value?, params?}]`, порядок = `body.order`), новый §8 «Конвейер» с инвариантом «fatal-значение в entry не попадает»
- [x] `schema/backup.schema.json`: `warnings` у узла опционально; импорт не доверяет
- [x] `corpus/**/expected.json`: `path` у существующих кодов; новые кейсы-пары ссылка↔JSON по `DRIFT.md` (сторона Go подтягивается к целевому поведению; изменения существующих ожиданий — со ссылкой на DRIFT/DECISIONS)
- [x] `VERSION` 1.1.0, `README.md` version log, `SPECS/103…/DECISIONS.md` D-122 (решения владельца §2 SPEC)
- [x] `TASKS_LXBOX.md` §24: норма, таблица изменённых правил с обоснованиями, вопросы (а) `warnings` в бэкап, (б) потребление `body` генерацией или руками, (в) согласие по пунктам DRIFT

## W2 — ядро конвейера в лаунчере (релиз patch)

- [x] `contract/registry` → `go:embed` в пакет `core/config/registry` (загрузка, индексы по схеме/пути, разрешение `ref`, `maps_to`)
- [x] `core/nodeflow`: `Sanitize(scheme, map) (clean, warns, drop)` по SPEC §3.2 — go1.20-совместимо (Л22), обход по `order`, tristate/all-or-nothing, приведение типов, `secret` маскирование
- [x] `core/nodeflow`: `Emit(scheme, clean) json.RawMessage` по `order`; Listable как пришло
- [x] Гейт сборки по `min_core`/`platform` из реестра; полевые `*SupportProbe` (`outbound_generator.go:284`) убраны; узловые гейты (`:1195-1213`) остаются
- [x] `state.Node.Warnings` (`sources_v7.go:199`, рядом с `Reason :252`), `NodeWarning{Code, Path, Value, Params}`; `ParsedNode.Warnings` расширяется `Path`/`Value`
- [x] Шов 1: `ServerNodeMaterial.Warnings` (`migrate_materialize.go:326-332`) и запись в `state.Node` во всех потребителях (Add, Regen `source_body_edit.go:147`, правка тела, миграция)
- [x] Шов 2: `canonicalNodeFromEntry` (`migrate_materialize.go:156-166`) переносит warnings; `SubUpdateStatus.Warnings` не трогается (Л15)
- [x] Ручной `config_json` (`migrate_materialize.go:175-191`) и `NodeFromManualConfigJSON` (`manual_config.go:34`) — через маппер JSON → санитайзер → эмиттер; `EmitRaw` снят
- [x] Релеи (`relay_materialize.go:129`), AWG-форма (`source_awg_edit.go:239`), отдельные парсеры мимо switch (Л27) — через конвейер
- [x] Вынос решений из парсеров схем (`CODEMAP.md` §2.2–§2.4): парсер = перевод по `maps_to`; правила-«убийцы» §2.3 → `required`/`drop_node` в реестре; дефолты §2.4 → `default_when` или снятие — **W2d**, отчёт `W2D_CHANGES.md` (21 правило снято, 6 перенесено в реестр, 14 осталось структурными)
- [x] `SanitizeSingboxOutboundMap` и `sanitizeSingbox*` заменены `nodeflow.Sanitize`; allowlist-эмиттеры (`outbound_jsonbuilder.go:84`, `outbound_tls_emit.go:41`, xhttp-списки) и цепочка `else if` (`outbound_generator.go:346-638`) заменены `nodeflow.Emit`
- [x] Разовая миграция при загрузке state: `warnings == nil` → санитайзер по телу; тело перезаписывается только при снятии/приведении, WARN на узел (Л3)
- [x] `max_uri_length` из `limits.json` (Л18)
- [x] `registry_sync_test.go:170` переписан на реестр (Л12): добавлен встречный `TestRegistryWarningCodesHaveAProducer` — у кода полей узла обязан быть производитель, константа Go **или** правило секции реестра
- [x] Корпус зелёный на новых `expected.json`; `sing-box check` на сводном конфиге из всех кейсов (ядро 1.14.0-lx.39)
- [x] Интеграционный тест конвейера: `TestPipelineAllInputsAgree` — ссылка / тело sing-box / Xray-JSON на одном мусоре дают равные `Body` и `Warnings`
- [x] `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md` (поток данных)

## W3 — UI (релиз patch)

- [x] ⚠ на строке узла: Servers, контейнер источника, Core runtime, список в Add — через `SubtitleWarn` (`source_node_row.go:61`) / `previewUnsupportedMark` (`preview_row_view.go:22`); tooltip = `title_<lang>` первого кода + «и ещё N»
- [x] Секция «Предупреждения» в окне Info узла: `text_<lang>` с подстановкой, кнопка «Подробнее» → `platform.OpenURL` через `urlsafe` (образец `source_support_link.go:27`), `Label` с `TextWrapWord` (Л19)
- [x] Превью Add: строка «⚠ снято N полей у M узлов» + раскрытие (узел · путь · код)
- [x] Итог fetch: счётчик узлов с ⚠
- [x] Тексты кодов — из embed-реестра (`title_*`/`text_*`), не из locale; обвязочные ключи locale + `bin/locale/ru.json`
- [x] Константа URL документации `…/contract/docs/generated/warnings.md#<code>`
- [x] Release notes

## W4 — генерация документации

- [x] `contract/tools/gendocs` (Go, go1.20-совместимо) → `contract/docs/generated/{index.md, warnings.md, protocols/<scheme>.md}` + страницы суб-схем `protocols/_{tls,transports,multiplex,dialer}.md`; `//go:generate` в `contract/doc.go`
- [x] CI-джоба `Contract` (`.github/workflows/contract.yml`, job `contract`): `go generate ./contract/...` + `git diff --exit-code contract/docs/generated` + `go test ./core/config/ -run Registry` (первая контрактная джоба в репо, Л13)
- [x] `docs/Protocols*.md` (`CODEMAP.md` §8.1) → абзац со ссылкой на `generated/index.md` и `warnings.md`; уникальные разделы (build-теги xhttp/AWG, JSON-массив Xray, share-URI, `.conf`/`vpn://`/Add-from-file) сохранены ниже ссылки; ссылки из README/README.ru/ParserConfig* перенаправлены
- [x] `TASKS_LXBOX.md`: путь к сгенерированным страницам, чтобы LxBox ссылался на те же якоря

## Закрытие

- [x] Все пункты `DRIFT.md` закрыты кейсом корпуса или записью в DECISIONS
- [~] Чеклист `SPECS/README.md` (сборка, vet, Win7-греп, доки, статус папки → C) — сборка/vet/Win7-греп/доки выполнены; статус папки → O (в работе до релиза), C — после релиза
- [x] IMPLEMENTATION_REPORT.md

## W2d — парсеры → мапперы

- [x] 21 правило значений снято из парсеров как дубликат реестра, 6 перенесено в реестр (`base64_32`, `hex_only`, условный `advisory`, `requires`+`equals`, `default_when`), 14 оставлены как перевод диалекта; алиасы имён — из реестра (`queryParam`); naive userinfo=password (7.3; дата синхронизации снята владельцем 18.09.2026); `W2D_CHANGES.md`
- [x] Пара ссылка↔JSON даёт равные тела и warnings (`nodeflow_pipeline_test.go`); `TestRegistryWarningCodesHaveAProducer` вместо регулярки по `parse_warnings.go`
- [ ] Порт вне 1–65535 в ссылке по-прежнему отбраковывает узел в `ParseNode` (форма `dropped[]`), а не код `port_invalid` на узле — отдельное решение (W2D_CHANGES §6)
