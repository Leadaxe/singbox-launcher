# TASKS 108 — Свёртка подписки в группу

Нормативка: [`SPEC.md`](SPEC.md). Порядок фаз обязателен: модель → правила →
материализация → UI → уборка → контракт. Каждая фаза заканчивается зелёной
сборкой; тесты — в конце по решению оператора.

## Фаза 0 — подтвердить открытый вопрос
- [ ] `Exclude from global` без локальных групп: вариант (а) — оставить поле
      без UI. Подтвердить у оператора **до** фазы 1

## Фаза 1 — модель и миграция (§Модель)
- [ ] `SourceFold{Mode, Auto}` в `core/state/connections.go`; `Mode` —
      `select` \| `auto` \| `select_auto`
- [ ] `Source.Fold *SourceFold`, json-ключ `fold`
- [ ] Чтение старых флагов: `ExcludeFromGlobal`, `ExposeGroupTagsToGlobal`
      остаются в структуре, но при загрузке разворачиваются в `Fold` по
      таблице §Миграция; обратно не пишутся
- [ ] Зеркало в `configtypes.ProxySource` (legacy-вид), чтобы генератор видел
      свёртку

## Фаза 2 — правила на не-Направления → direct (S5, T4)
- [ ] На загрузке (`core/state/load_v6.go`): цель правила вне множества
      Направлений и литералов → `direct` + `WarnLog` с именем правила и
      прежней целью
- [ ] То же для `route.final` (`config_params`) — T4
- [ ] Проверить, что литералы `direct`/`reject`/`drop` и тег блокировки из
      шаблона не сбрасываются

## Фаза 3 — материализация (§Материализация)
- [ ] Свёрнутая подписка: узлы исключаются из пула Направлений через
      существующий `FilterNodesExcludeFromGlobal`
- [ ] Тег группы — в кандидаты через существующий `collectExposeTagCandidates`
      (маркеры `WIZARD:*` в Comment оставить, T1)
- [ ] `mode = select_auto`: у записи `:select` ставится `Auto`, двойник
      разворачивается общим проходом 0 (`direction_twins.go`)
- [ ] `mode = auto`: эмитится только `:auto`
- [ ] Проверить `RenameWizardLocalOutboundTags` при смене префикса (T2)

## Фаза 4 — UI (§UI)
- [ ] Четыре галки → одна («свернуть подписку в группу»); ключи локали
      `wizard.source.{local_auto,local_select,exclude_global,expose_tags,expose_tags_tooltip}`
      удалить, добавить пару для новой
- [ ] Вкладка «Группа» в окне подписки: видна при включённой галке, три
      расклада + настройки автогруппы (виджеты с вкладки «Автовыбор»)
- [ ] Разметка как на «Автовыборе»: подпись фиксированной ширины (Border),
      подсказка с переносом, липкость в два ряда
- [ ] Обработчики после установки значений (ловушка рекурсии SPEC 104)

## Фаза 5 — уборка (S3, S4)
- [ ] `GetAvailableOutbounds`: ветка «Add local outbounds from all ProxySource»
      удаляется
- [ ] Список Направлений: секции и показ локальных строк убрать; ключи
      `wizard.outbound.section_service`, `section_directions` удалить
- [ ] `SyncExposeFlagWhenNoLocalGroups` — мёртвый, удалить (T3)
- [ ] Проверить, что `ProxyHasLocalAuto`/`RemoveWizard*` остались нужны

## Фаза 6 — контракт, бэкап, документация
- [ ] `fold` в схеме подписки контракта; `Fold.Auto` через `$ref` на
      `direction.schema.json#/$defs/auto` (T6)
- [ ] Бэкап: `fold` в `subscriptions[]` (T5)
- [ ] `docs/WIZARD_STATE{,.ru}.md` — секция про `fold`; `ParserConfig.md` —
      про свёртку; `upcoming.md` EN+RU

## Проверка
- [ ] `go build ./... && go vet ./...`, линт 0 issues
- [ ] Тесты модуля
- [ ] Живая проверка на рабочем конфиге (за оператором)
