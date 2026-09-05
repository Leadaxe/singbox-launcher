# TASKS 121 · Секции узла

Ссылки на строки — в `CODEMAP.md` и `SPEC.md`. Правила исполнителю:
ветки не переключать, `ui/traffic` и `bin/locale/ru.json` не трогать,
коммитов не делать, тесты — в конце волны, полный `go test ./...` — один
раз в конце волны. Свои правки отражать в `CODEMAP.md` (адреса, новые
функции). Отчёт — с адресами `файл:строка` того, что добавлено.

## Волна 1 — модель, сборка, бэкап, контракт (без UI)

- [ ] W1.1 `state.Node.Sections` + `NodeSections{DNSServers,DNSRules,Rules}` +
      `IsEmpty()`; нормализация пустого набора в nil при сохранении;
      `kind≠server` → поле обнуляется при чтении (SPEC §3.1).
- [ ] W1.2 `RuleKindNode = "node"` + `NodeRuleBody{FolderID,Tag}` в
      `core/state/rule_types.go`; `DecodeBody` знает новый вид;
      `NodeRuleDefaultNum = 945` в `rule_order.go`; `SeedNodeRules` и
      вызов из `NormalizeRuleOrder` (сигнатура расширяется списком
      `NodeLink` узлов с непустыми `Rules`; все вызывающие обновить).
- [ ] W1.3 Пронос `Sections` через `CanonicalNode` → `ParsedNode` →
      `OutboundGenerationResult` → `ParsedCache.NodeSections []NodeSectionSet`
      (SPEC §3.2). Заполнить во всех трёх производителях кэша (шапка
      `parsed_cache.go:1-15`). Узел, не дошедший до эмиссии, в кэш не входит.
- [ ] W1.4 `core/build/node_sections_expand.go`: развёртывание одного
      `NodeSectionSet` (SPEC §4 п.2): строгая подстановка `@self`, префиксы
      тегов, дефолт `outbound`, отсев `rule_set`, warnings.
- [ ] W1.5 `MergePresetsIntoDNS`: серверы и правила узлов после пресетных,
      до `pruneDNSGroupMembers`/`repairDanglingDNSRefs`; dedup по тегу.
      `PresetMergeContext` получает `NodeSections`.
- [ ] W1.6 Резолв route: ветка `RuleKindNode` в `resolve_route.go`
      (поиск `NodeSectionSet` по `Link`, отсутствие = пропуск без warning);
      ранний выход `MergePresetsIntoRoute:223` учитывает node-правила;
      `NormalizeRuleOrder` в `resolve_route.go:150-154` получает links из
      `state.Sources`.
- [ ] W1.7 `SanitizeDNSDetours`: ребро `endpoint` — висячее → сервер
      выбрасывается целиком + WarnLog, затем починка ссылок DNS-правил
      (SPEC §4 п.5, следить за порядком с `repairDanglingDNSRefs`).
- [ ] W1.8 Бэкап: `servers[].sections{dns_servers,dns_rules,rules,rule_num}`
      в `core/backup/types.go`; `serverKeys` в `scanUnknown`
      (`core/backup/file.go:359-386`); экспорт (`exportServerNode`/`exportServer`,
      `rule_num` из записи `kind=node`; `kind=node` в `rules[]` не пишется);
      импорт (`importServer`: секции файла замещают локальные при совпадении
      тела; после `s.Rules=nil`+слияния — пересев якорей с `rule_num`).
- [ ] W1.9 Контракт и документы: `contract/schema/backup.schema.json`
      (поле у `servers[]`, `additionalProperties` как у соседей);
      `contract/docs/BACKUP.md` (таблица `servers[]:83-94`, абзац в §9);
      `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md` — строка D-098
      (формат `DECISIONS.md:3`, дата 2026-09-05, «кто: Пользователь»,
      пометка «черновик до ответа LxBox; VERSION не поднят»);
      `contract/TASKS_LXBOX.md` — `## 9. servers[].sections — поле лаунчера
      (приоритет 3)`: что это, что LxBox игнорирует молча и провозит,
      предложение 0.13.0. `contract/VERSION` НЕ менять.
      `SPECS/features/sources.md` — абзацы по SPEC §9.
- [ ] W1.10 Тесты волны (в конце, комплексные): один табличный тест
      сборки в `core/build/` на сценарии SPEC §8 п.1–4 (узел с секциями
      в корне; в папке с префиксом TagPolicy; выключен; без секций =
      байт-в-байт с сегодняшним выводом — сравнить с `BuildConfig` на том
      же состоянии без `Sections`); один тест бэкапа round-trip
      (`core/backup/`) на §8 п.5; санитайзер §8 п.8 — в существующий
      `dns_detour_sanitize` тест как ещё один случай. Запуск:
      `go build ./... && go vet ./... && go test ./...`. Golden
      `real-v088` обязан остаться зелёным без пересчёта.

## Волна 2 — UI и импорт из тела

- [ ] W2.1 Вкладка JSON узла: приём документа (SPEC §5.1), проверки,
      перепись реального тега в `@self`, дефолт `outbound`, отрисовка
      документом при наличии секций. Ошибки — через существующий путь
      отката вкладки; тексты по-английски через `locale.T`.
- [ ] W2.2 Модель `NodeRefState`, слот `SlotKindNodeRef`, синхронизация с
      `state.Rule kind=node`, пересев при загрузке модели и после
      сохранения узла; строка якоря в `rules_unified_rows.go` (SPEC §5.2);
      `MoveRuleSlot`/`applyAxisAfterMove` знают новый слот.
- [ ] W2.3 Вкладка DNS: read-only серверы и правила узлов (SPEC §5.3).
- [ ] W2.4 Импорт из тела источника: `singbox_sections_extract.go`
      (SPEC §6), счётчик в `SingboxImportResult`, InfoLog.
- [ ] W2.5 Тесты волны: один тест извлечения из тела на конфиге из
      issue (endpoint заменить на `wireguard`, т.к. `tailscale` до SPEC 122
      unsupported) и на конфиге с двумя узлами; UI-тесты не писать.
      Полный прогон один раз; `build/test_*.sh` для fyne-пакетов, если
      затронуты.
- [ ] W2.6 `docs/release_notes/upcoming.md`; `CODEMAP.md`; статус папки
      `121-F-N` → `121-F-C` и `IMPLEMENTATION_REPORT.md` — после приёмки.
