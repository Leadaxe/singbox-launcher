# SPEC 106-B — подключить ось порядка правил (drag&drop не сохраняется)

## Симптом (сообщение пользователя Roman Sh., 26.08.2026)

Перетаскивание правил на вкладке Rules не переживает перезаход в Config
Wizard: правила «автосортируются» обратно.

## Диагноз (проверено)

1. `core/state/rule_order.go` — ПОЛНЫЙ механизм оси порядка SPEC 106
   (MarkRuleOrder, NormalizeRuleOrder, PlaceRuleAfter с ленивым сдвигом
   D-053а, NextUserRuleNum, SeedRequiredRules, DedupePresetRules) —
   существует, документирован, покрыт тестами и **никем не вызывается**
   (грепом: вызовов вне файла и его тестов нет). Портирован из LxBox
   (`rule_order.dart`), но не подключён.
2. Живой state.json: у всех 13 правил `order_num = null` — ось никогда
   не проставлялась.
3. UI-перетаскивание (`MoveRuleSlot`, ui/configurator/models/rule_slot.go:113)
   двигает только слоты; Save (`SyncRulesByOrderToStateRulesV6`,
   ui/configurator/models/preset_ref_sync.go:54) эмитит state.Rule БЕЗ
   OrderNum.
4. Восстановление порядка при загрузке (`RuleOrderFromStateRulesV6`,
   preset_ref_sync.go:114) матчит custom-правила ПО LABEL (crByLabel,
   строка ~128): безымянные inline-правила и дубли label выпадают из
   порядка и доклеиваются в конец через ReconcileRuleOrder.
5. Эмит конфига сортирует по оси (core/build/resolve_route.go:150,
   ruleOrderNum: nil → DefaultRuleNum=1000; core/build/preset_merge.go:266) —
   т.е. потребитель оси живёт, поставщика нет.

## Задача

Подключить существующую ось, НЕ изобретая новой модели. Семантика SPEC 106
сохраняется: системные правила прибиты (не двигаются, поверх них не встать —
уже enforced в MoveRuleSlot), сортируемые пресеты и пользовательские правила
двигаются свободно, порядок переживает save→load→build.

### Изменения

1. **Загрузка в Wizard** (`presenter_state_helpers.go:~100` restore-путь):
   перед восстановлением порядка прогнать `state.NormalizeRuleOrder(rules,
   specs)` — specs собрать из template.Preset (RuleOrderSpec{Num, Sortable,
   DefaultEnabled}; поле num у пресетов шаблона — проверить имя в
   core/template/preset_types.go и bin/wizard_template.json). После
   normalize порядок state.Rules = порядок оси; восстановление слотов
   остаётся позиционным поверх него.
2. **Модель UI держит номер**: RuleState / PresetRefState получают поле
   номера оси (прокидывается при SyncStateRulesToPresetRefs /
   restoreCustomRules и обратно при Sync*ToStateRulesV6 — order_num обязан
   переживать round-trip). Слоты остаются механизмом отображения.
3. **Drag&drop** (`moveSlot` → `MoveRuleSlot`): после перестановки слота
   вычислить нового «соседа сверху» и применить семантику PlaceRuleAfter
   к номерам модели (или собрать []state.Rule, применить
   state.PlaceRuleAfter и раздать номера обратно — выбрать меньшую
   склейку). Ленивый сдвиг D-053а обязателен — НЕ пере-нумеровывать всю
   зону подряд (комментарий в rule_order.go объясняет почему).
4. **Добавление правила**: номер от state.NextUserRuleNum по текущему
   набору, не хардкод.
5. **Save** (`SyncRulesByOrderToStateRulesV6`): эмитить OrderNum из модели.
6. **Восстановление порядка** (`RuleOrderFromStateRulesV6`): сортировать по
   оси (стабильно), а не доверять только позиции; матчинг custom-правил
   перевести с label на identity SPEC 063 (`state.StableRuleID`, упомянут
   в комментарии preset_ref_sync.go:125) — безымянные правила не должны
   терять место.

### Ловушки

- `RefreshAfterPresetToggle` (presenter_sync.go:250) тоже гоняет
  Sync→build: номера обязаны быть стабильны на этом пути, иначе каждое
  переключение пресета перетасует ось.
- Бэкап-импорт уже штампует последовательные номера
  (core/backup/import.go:392,463) — совместимость сохранить (normalize
  после импорта их не портит — проверить тестом).
- DNS-правила имеют СВОЮ ось (DNSRuleOrder, SPEC 062) — в объём НЕ входит;
  если там тот же паттерн — зафиксировать в отчёте, не чинить.
- win7 = go1.20: без min/max/slices.
- Тестов на форматирование UI-строк не писать.

### Тесты (обязательные)

1. e2e: загрузка → MoveRuleSlot (custom выше сортируемого пресета) → Save →
   повторная загрузка → порядок слотов идентичен; state.json несёт
   order_num.
2. Безымянное inline-правило сохраняет позицию через round-trip.
3. Эмит: порядок route.rules в config.json следует оси; системное правило
   первым; несортируемый пресет на якоре из шаблона.
4. Ленивый сдвиг: перетаскивание вплотную к якорю не двигает якорь
   (расширить существующие тесты rule_order при необходимости).
5. Существующий state с order_num=null нормализуется при первой загрузке
   и сохраняется размеченным.

### Приёмка

`go build ./...`, `go test ./...` зелёные; ручной сценарий Романа
(перетащил → Save → закрыл/открыл Wizard → порядок на месте) воспроизведён
e2e-тестом.
