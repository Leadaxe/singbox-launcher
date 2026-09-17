# TASKS — SPEC 131 · Единый конвейер добавления узла

Нормы — `SPEC.md`; адреса — `CODEMAP.md` (на `d0e5aca3`); расхождения —
`DRIFT.md`; поля ядра — `CORE_SCHEMA.md` / `core_schema.draft.json`.
Каждая волна — отдельный набор коммитов в develop, старый код живёт до замены.

## W0 — ТЗ

- [x] SPEC.md (решения владельца, конвейер, реестр, state/контракт, UI, генерация, волны, приёмка)
- [x] CODEMAP.md — карта кода и 28 ловушек
- [x] CORE_SCHEMA.md + core_schema.draft.json — поля тел на теге `v1.14.1-lx.4`, lx_only от базы форка
- [ ] DRIFT.md — расхождения Go/Dart с обоснованиями (в работе)
- [x] TASKS.md

## W1 — контракт (1.1.0, аддитивно)

Реестр:
- [ ] Общие суб-схемы `body` в `registry/tls.json` (tls/utls/reality/ech), `registry/transports.json` (6 транспортов + xmux), новые `registry/multiplex.json`, `registry/dialer.json` — из `core_schema.draft.json`; атрибуты по SPEC §4 (`type`, `values`, `required`, `secret`, `tristate`, `all_or_nothing`, `lx_only`, `min_core`, `platform`, `desc_en`, `desc_ru`, `impl`)
- [ ] Секция `body` в каждом `registry/protocols/<scheme>.json` (15 схем + chain, group как есть) с `order` в порядке структур ядра
- [ ] `allowed_for`/`forbidden_for` у TLS-полей для naive (16 фатальных + 6 игнорируемых, `CORE_SCHEMA.md` §3.9) — код `tls_field_unsupported_naive`
- [ ] `conflicts`/`requires`/`forbidden_when` из `CORE_SCHEMA.md` «конфликты» (27 пунктов) и `DRIFT.md`
- [ ] `on_invalid` и коды для каждого поля с ограничением (enum/format/len) — по `DRIFT.md`, целевое = вариант LxBox с обоснованием; там, где обоснование против — пометка `decision_pending` и вопрос владельцу
- [ ] `maps_to` у каждого `uri.query.<param>` и `userinfo`; правила значений переезжают из `uri.*` в `body.*`, в `uri.*` остаются синтаксис и алиасы имён
- [ ] `note` → `impl` (перенос без потерь), `desc_en`/`desc_ru` — новые короткие
- [ ] `warnings.json`: `title_en/title_ru/text_en/text_ru` для всех 61 + новые коды (`unknown_key`, `type_invalid`, `alias_shadowed`, `field_conflict`, `tls_field_unsupported_naive`, `flow_with_transport`, `partial_object_defaulted` — сверить с существующими, дублей не плодить)
- [ ] `schema/registry_body.schema.json` — JSON-схема секции `body` (для линтера обеих сторон)
- [ ] Линтер реестра (Go-тест): каждый `code` объявлен, каждый `ref` существует, `order` покрывает `fields`, каждое поле `core_schema` на теге пина отражено или помечено `skip`

Документы:
- [ ] `docs/CANON.md` §6 (`warnings[] = [{code, path, value?, params?}]`, порядок = `body.order`), новый §8 «Конвейер» с инвариантом «fatal-значение в entry не попадает»
- [ ] `schema/backup.schema.json`: `warnings` у узла опционально; импорт не доверяет
- [ ] `corpus/**/expected.json`: `path` у существующих кодов; новые кейсы-пары ссылка↔JSON по `DRIFT.md` (сторона Go подтягивается к целевому поведению; изменения существующих ожиданий — со ссылкой на DRIFT/DECISIONS)
- [ ] `VERSION` 1.1.0, `README.md` version log, `SPECS/103…/DECISIONS.md` D-122 (решения владельца §2 SPEC)
- [ ] `TASKS_LXBOX.md` §24: норма, таблица изменённых правил с обоснованиями, вопросы (а) `warnings` в бэкап, (б) потребление `body` генерацией или руками, (в) согласие по пунктам DRIFT

## W2 — ядро конвейера в лаунчере (релиз patch)

- [ ] `contract/registry` → `go:embed` в пакет `core/config/registry` (загрузка, индексы по схеме/пути, разрешение `ref`, `maps_to`)
- [ ] `core/nodeflow`: `Sanitize(scheme, map) (clean, warns, drop)` по SPEC §3.2 — go1.20-совместимо (Л22), обход по `order`, tristate/all-or-nothing, приведение типов, `secret` маскирование
- [ ] `core/nodeflow`: `Emit(scheme, clean) json.RawMessage` по `order`; Listable как пришло
- [ ] Гейт сборки по `min_core`/`platform` из реестра; полевые `*SupportProbe` (`outbound_generator.go:284`) убраны; узловые гейты (`:1195-1213`) остаются
- [ ] `state.Node.Warnings` (`sources_v7.go:199`, рядом с `Reason :252`), `NodeWarning{Code, Path, Value, Params}`; `ParsedNode.Warnings` расширяется `Path`/`Value`
- [ ] Шов 1: `ServerNodeMaterial.Warnings` (`migrate_materialize.go:326-332`) и запись в `state.Node` во всех потребителях (Add, Regen `source_body_edit.go:147`, правка тела, миграция)
- [ ] Шов 2: `canonicalNodeFromEntry` (`migrate_materialize.go:156-166`) переносит warnings; `SubUpdateStatus.Warnings` не трогается (Л15)
- [ ] Ручной `config_json` (`migrate_materialize.go:175-191`) и `NodeFromManualConfigJSON` (`manual_config.go:34`) — через маппер JSON → санитайзер → эмиттер; `EmitRaw` снят
- [ ] Релеи (`relay_materialize.go:129`), AWG-форма (`source_awg_edit.go:239`), отдельные парсеры мимо switch (Л27) — через конвейер
- [ ] Вынос решений из парсеров схем (`CODEMAP.md` §2.2–§2.4): парсер = перевод по `maps_to`; правила-«убийцы» §2.3 → `required`/`drop_node` в реестре; дефолты §2.4 → `default_when` или снятие
- [ ] `SanitizeSingboxOutboundMap` и `sanitizeSingbox*` заменены `nodeflow.Sanitize`; allowlist-эмиттеры (`outbound_jsonbuilder.go:84`, `outbound_tls_emit.go:41`, xhttp-списки) и цепочка `else if` (`outbound_generator.go:346-638`) заменены `nodeflow.Emit`
- [ ] Разовая миграция при загрузке state: `warnings == nil` → санитайзер по телу; тело перезаписывается только при снятии/приведении, WARN на узел (Л3)
- [ ] `max_uri_length` из `limits.json` (Л18)
- [ ] `registry_sync_test.go:170` переписан на реестр (Л12) в том же коммите
- [ ] Корпус зелёный на новых `expected.json`; `sing-box check` на сводном конфиге из всех кейсов
- [ ] Интеграционный тест конвейера: все входы на одном наборе мусора → равные `Body` и `Warnings`
- [ ] `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md` (поток данных)

## W3 — UI (релиз patch)

- [ ] ⚠ на строке узла: Servers, контейнер источника, Core runtime, список в Add — через `SubtitleWarn` (`source_node_row.go:61`) / `previewUnsupportedMark` (`preview_row_view.go:22`); tooltip = `title_<lang>` первого кода + «и ещё N»
- [ ] Секция «Предупреждения» в окне Info узла: `text_<lang>` с подстановкой, кнопка «Подробнее» → `platform.OpenURL` через `urlsafe` (образец `source_support_link.go:27`), `Label` с `TextWrapWord` (Л19)
- [ ] Превью Add: строка «⚠ снято N полей у M узлов» + раскрытие (узел · путь · код)
- [ ] Итог fetch: счётчик узлов с ⚠
- [ ] Тексты кодов — из embed-реестра (`title_*`/`text_*`), не из locale; обвязочные ключи locale + `bin/locale/ru.json`
- [ ] Константа URL документации `…/contract/docs/generated/warnings.md#<code>`
- [ ] Release notes

## W4 — генерация документации

- [x] `contract/tools/gendocs` (Go, go1.20-совместимо) → `contract/docs/generated/{index.md, warnings.md, protocols/<scheme>.md}` + страницы суб-схем `protocols/_{tls,transports,multiplex,dialer}.md`; `//go:generate` в `contract/doc.go`
- [x] CI-джоба `Contract` (`.github/workflows/contract.yml`, job `contract`): `go generate ./contract/...` + `git diff --exit-code contract/docs/generated` + `go test ./core/config/ -run Registry` (первая контрактная джоба в репо, Л13)
- [x] `docs/Protocols*.md` (`CODEMAP.md` §8.1) → абзац со ссылкой на `generated/index.md` и `warnings.md`; уникальные разделы (build-теги xhttp/AWG, JSON-массив Xray, share-URI, `.conf`/`vpn://`/Add-from-file) сохранены ниже ссылки; ссылки из README/README.ru/ParserConfig* перенаправлены
- [ ] `TASKS_LXBOX.md`: путь к сгенерированным страницам, чтобы LxBox ссылался на те же якоря

## Закрытие

- [ ] Все пункты `DRIFT.md` закрыты кейсом корпуса или записью в DECISIONS
- [ ] Чеклист `SPECS/README.md` (сборка, vet, Win7-греп, доки, статус папки → C)
- [ ] IMPLEMENTATION_REPORT.md
