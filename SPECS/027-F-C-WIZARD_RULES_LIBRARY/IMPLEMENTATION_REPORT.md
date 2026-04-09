# IMPLEMENTATION_REPORT: Rules — custom + библиотека (027)

## Статус

**Закрыто (Complete):** 2026-03-24. Реализовано по **SPEC.md** / **PLAN.md** / **TASKS.md**. Папка задачи: **`SPECS/027-F-C-WIZARD_RULES_LIBRARY`** (статус **F-C** по Spec Kit).

## Сделано

### State, миграция, первый засев

- **`WizardStateVersion = 3`**; **`state_store`** принимает чтение **2..текущая**.
- Поля **`rules_library_merged`**, при сохранении **`selectable_rule_states = nil`**.
- **`ApplyRulesLibraryMigration`**: при отсутствии флага — слияние блока шаблона (порядок **`selectable_rules`**, enabled/outbound из **`selectable_rule_states`**) + хвост **`custom_rules`**; идемпотентность за счёт флага и немедленной записи state после первой миграции в **`LoadState`**.
- **`EnsureCustomRulesDefaultOutbounds`**: сразу после **`restoreCustomRules`** в **`LoadState`** — подстановка **`SelectedOutbound`** из ParserConfig (миграция клонировала пресеты с `nil` outbounds).
- **`InitializeTemplateState`**: **`SelectableRuleStates` всегда nil**; при **`!RulesLibraryMerged && len(CustomRules)==0`** — клоны пресетов с **`IsDefault`** в **`CustomRules`**, затем **`RulesLibraryMerged = true`**.

### Генерация и клонирование

- **`CloneTemplateSelectableToRuleState`** — глубокая копия пресета → **`RuleState`** (тип через **`DetermineRuleType`**).
- **`MergeRouteSection`** — скелет **`route`** из шаблона + включённые правила из **`custom_rules`** (без отдельного цикла по selectable-state).
- **`ClonePresetWithSRSGuard`**, **`disableRuleIfSRSPending`**, **`EnsureCustomRulesDefaultOutbounds`** — общая логика пресетов/SRS и outbound после **`LoadState`**.

### UI

- **`rules_tab.go`**: над скроллом одна строка — **Add Rule** / **Add from library** и подпись столбца **Outbound:**; в скролле строки с **`widget.Check`**, **↑↓** (тултипы), подпись (**Border**), SRS, Edit/Del, outbound **Select** без дублирования «Outbound:» в каждой строке.
- **`library_rules_dialog.go`**: модалка, чекбоксы, подсветка выбранных строк, **Add selected** — append клонов в **`CustomRules`**.

### Локализация и тесты

- Ключи в **`internal/locale/en.json`**; зеркальные строки в **`bin/locale/ru.json`** (требование **`TestAllKeysPresent`**).
- Тесты: **`rules_library_test.go`** (клон + миграция), правки интеграционных/генератора под новый **`MergeRouteSection`**.

### Документация

- **`docs/WIZARD_STATE.md`** — формат снимка **3** (rules library) / **4** (актуальная запись при **`state.vars`**), поток **`LoadState`** / **`InitializeTemplateState`**, миграция **v2→v3**.
- **`docs/ARCHITECTURE.md`** — вкладка Rules, **`library_rules_dialog`**, **`rules_library.go`**, **`MergeRouteSection`**, загрузка state.
- **`docs/release_notes/upcoming.md`**, **`docs/TEST_README.md`**, **`docs/CREATE_WIZARD_TEMPLATE.md`** / **`_RU.md`**, **`SPECS/002-.../WIZARD_STATE_JSON_SCHEMA.md`**, **`SPECS/024-.../SPEC.md`**, **`RELEASE_NOTES.md`**, **`SPECS/README.md`** (строка **027**).

## Ссылки

**SPEC.md**, **PLAN.md**, **TASKS.md**.
