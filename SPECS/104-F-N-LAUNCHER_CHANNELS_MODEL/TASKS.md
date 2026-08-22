# TASKS 104 — Направления (Direction)

Нормативка: [`SPEC.md`](SPEC.md) (§-ссылки ниже) и [`DECISIONS_DIRECTION.md`](DECISIONS_DIRECTION.md).
Порядок фаз обязателен: сначала снос, потом модель, потом всё остальное —
чтобы ни одного коммита не жить с двумя моделями. Каждая фаза — зелёные
`go build/vet/test` + линт; коммит на фазу (или меньше).

## Фаза 0 — снос первой реализации (§8) — один коммит
- [ ] Удалить файлы §8 (8 шт.), вычистить ссылки по списку §8
- [ ] `bin/wizard_template.json`: убрать `default_channels`; `group_templates` оставить
- [ ] Локали: `wizard.channels.*` EN+RU удалить
- [ ] `docs/WIZARD_STATE{,.ru}.md §3.7`, `TEMPLATE_REFERENCE{,.ru}.md §4.6`, `ARCHITECTURE_PACKAGES{,.ru}.md`, `upcoming.md` — убрать текст о каналах (переписать на Направления можно в фазе 5)
- [ ] `go build ./... && go vet ./... && go test ./...` зелёные

## Фаза 1 — модель и хранение (§2) — коммит
- [ ] `OutboundConfig`→`Direction`, `OutboundUpdate`→`DirectionUpdate` по всему модулю; алиас `core/config/models.go:10` удалить (T3)
- [ ] Новые поля `Label`, `Disabled`, `Auto *DirectionAuto`; `DisplayName()` (T1)
- [ ] `configtypes/direction_filter.go`: `DirectionFilterBody/Pattern`, `DirectionDefaultBody/Pattern`, `NextDirectionTag`, `DefaultDirectionLabel` + тесты (эмодзи, кириллица, `/x/` без `i`, литерал, инверсия, чужие ключи в `filters`)
- [ ] `ConnectionsSection`: json `direction_outbounds` + `LegacyOutbounds` read-only; load-перенос, save-обнуление; всегда писать `[]` (§2.4, T15)
- [ ] Тесты: round-trip save→load; чтение старого `connections.outbounds`; старый `channels[]` игнорируется; `hasReferencedOutbounds` не триггерит лишний бэкап
- [ ] Слои пресетов (§5, T2): `PresetOutbound`, `applyOutboundUpdate`, `OutboundFieldDiff`, `stripReferencedBody`, `stripDirectBodyForReferenced` + тест «USER-патч label/auto/disabled переживает Save»
- [ ] Debug API `/state/full` отдаёт новый ключ (ничего не делать, проверить тестом/curl)

## Фаза 2 — материализация (§3) — коммит
- [ ] Проход 0 `expandDirectionTwins` (build-only, `TwinOf`), дефолты из `group_templates.auto.options`, round_robin → `mode`+`balancer` (sentinel sticky `["none"]`)
- [ ] Генератор: `TwinOf` исключает exposed-группы из кандидатов (`outbound_validity.go:82,112`, `outbound_generator.go:579`)
- [ ] Родитель: двойник первым в `addOutbounds`; `default=<tag>-auto` только при валидном двойнике и несовпавшем `preferredDefault` (T5)
- [ ] `Disabled` — выкинуть до прохода 1
- [ ] Пустое глобальное направление → `[block-out, direct-out]` default=block + warning (§3.3); теги из `magic_nodes`; per-source без изменений
- [ ] Битый regex ключа фильтра → ключ отброшен + WarnLog, `MatchesPattern` не трогать (§3.5, T7)
- [ ] Предупреждение только когда виноват фильтр; доставка `WarnLog` + Preview (T6)
- [ ] Тесты (перенести смысл из удалённого `channels_test.go`): без фильтра; фильтр; инверсия; битый regex; пустое → block/direct; нет предупреждения без узлов вовсе; предупреждение «direct» когда первым direct-out; двойник исключает группы; без двойника при `Auto==nil`; default-regex; default=auto; disabled пропущено; `@urltest_*` из шаблона доходят до конфига (аналог `TestChannelGroupsReachBuiltConfig`); Kahn: направление выше — опция ниже

## Фаза 3 — UI (§4) — коммит (можно два: вкладка / редактор)
- [ ] Вкладка: заголовок `wizard.tab_outbounds` → Directions/Направления; убрать `ParserConfigEntry` + docButton + `wizard.outbounds.*`; T8 (`presenter_async.go:39`)
- [ ] Список: только направления; служебные группы — свёрнутая секция ниже (S5); строка `DisplayName (tag)`, Disabled приглушён + переключатель
- [ ] Редактор по таблице §4.2: Name, tag на Add/Edit, Type только для `urltest`, фильтр тело+инверсия, flag-picker отдаёт тело+инверсию, default тело, опции `+direct/+block/направления выше` без `reject` и без `*-auto`, блок Auto twin (mode/url/interval/tolerance/idle/interrupt/pool/pool_tolerance/sticky), Preview «auto twin: N nodes», JSON-вкладка остаётся, Scope read-only из направлений (T9)
- [ ] Кнопка «?» → `docs/DIRECTION_FILTERS{,.ru}.md` (новый)
- [ ] `GetAvailableOutbounds`: без `ChannelTags`, без Disabled, без двойников; опционально `tag — label` (T10)
- [ ] Локали EN+RU: тексты `wizard.outbound.*` на Direction/Направление + новые ключи; без Test* на строки
- [ ] Ручная проверка сборки: `./build/build_darwin.sh -i`, шаблон+локаль в бандл руками; перезапуск лаунчера — только по команде пользователя (T11, T12)

## Фаза 4 — контракт и бэкап (§7) — коммит
- [ ] `contract/schema/direction.schema.json`, `contract/docs/DIRECTION.md`
- [ ] `contract/corpus/direction/` — кейсы по списку §7.2 (+ README корпуса)
- [ ] Go-раннер `core/config/contract_direction_test.go`
- [ ] Бэкап v1.1: `directions[]` экспорт/импорт (создание отсутствующих, warning `backup_direction_exists`, KnownOutbounds после создания), схема, `BACKUP.md §3/§6`, фикстура `directions_created_on_import`
- [ ] `contract/VERSION` 0.3.0, `tool/sync_contract.sh`, `contract.lock` (T14)

## Фаза 5 — документация лаунчера (§9) — коммит
- [ ] `WIZARD_STATE{,.ru}` §3.7 → `direction_outbounds`; `TEMPLATE_REFERENCE{,.ru}` §4.6; `ARCHITECTURE_PACKAGES{,.ru}`; `ParserConfig.md` §outbounds; `TEMPLATE_LANG.md §8.4`; `upcoming.md` EN+RU; `CONSTITUTION.md §7.3`, `SPECS/README.md`

## Фаза 6 — LxBox (§6) — отдельные коммиты в репо LxBox
- [ ] L1 термин: классы/файлы/строки; `ru/ui.json` (T13); Debug API алиасы
- [ ] L2 хранение: читать `channels[]`, писать `direction_outbounds[]`; allowlist legacy-бэкапа
- [ ] L3 `include[]` направлений выше (модель, билдер, экран)
- [ ] L4 пресеты поверх направлений: шаблон `selectable_rules[].outbounds[]` add/update, `ref`/`updates[]`, sync на toggle, Del скрыт / Edit = USER-патч
- [ ] L5 двойник первым + default=auto (как §3.2)
- [ ] L6 бэкап v1.1 (`lx_backup.dart`)
- [ ] L7 раннер `app/test/contract/direction_corpus_test.dart`
- [ ] L8 `docs/spec/features/393 directions/spec.md` (номер проверить), `CHANGELOG.md`, `AGENTS.md`
- [ ] `flutter test` зелёный, contract lock совпадает

## Осталось от первой редакции (за пользователем)
- [ ] Живая проверка на рабочем конфиге после фазы 3
