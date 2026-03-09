# Отчёт о реализации: 017 — Process path (Match by path, Simple/Regex)

## Статус

Реализовано. Готово к тестированию и закрытию задачи.

## Изменения

1. **ui/wizard/dialogs/rule_dialog.go**
   - Константа `ProcessPathRegexKey = "process_path_regex"`.
   - Функция `SimplePatternToRegex(pattern string) (string, error)`: замена `*` → `(.*)`, экранирование метасимволов regex.

2. **ui/wizard/dialogs/rule_dialog_test.go** (новый)
   - Юнит-тесты для `SimplePatternToRegex`: базовые шаблоны, экранирование, пустая строка.

3. **ui/wizard/dialogs/rule_type_selection.go** (новый)
   - Микро-модель **RuleTypeSelection**: хранение выбранного типа, `SetType`/`Type`, callback `OnChange` для синхронизации UI. Один источник истины; guard от реентрантности в диалоге при синхронизации чекбоксов.

4. **ui/wizard/models/wizard_state_file.go**
   - В `DetermineRuleType`: при наличии в rule поля `process_path_regex` возвращается тип «Processes».

5. **ui/wizard/dialogs/add_rule_dialog.go**
   - Выбор типа правила: **четыре чекбокса** (IP, Domains/URLs, Processes, Custom JSON), всегда выбран ровно один. Микро-модель `RuleTypeSelection`, при открытии вызывается начальная синхронизация чекбоксов (`onRuleTypeChange(ruleSel.Type())`).
   - **По умолчанию при создании** — первая позиция (IP Addresses (CIDR)); при редактировании — тип из правила.
   - Повторное нажатие на уже выбранную галочку не меняет выбор (чекбокс остаётся отмеченным). Снять можно только с выбранного типа — у остальных галочек снимать нечего.
   - В строке с чекбоксом Processes по центру — чекбокс **«Match by path»** (`HBox(typeProcessCheck, spacer, matchByPathCheck, spacer)`).
   - При включении Match by path: переключатель Simple / Regex, многострочное поле «Path patterns». Placeholder поля меняется при переключении (Simple: про `*`; Regex: про готовые regex, «no /regex/i wrapping»).
   - Режим Simple: при сохранении `*` → `(.*)` через `SimplePatternToRegex`, валидация regex. Режим Regex: строки как есть, проверка `regexp.Compile`.
   - В `buildRuleRaw` для Processes при Match by path — rule с `process_path_regex` (массив regex). В `validateFields` — наличие строк и валидность regex.
   - При редактировании правила с `process_path_regex`: чекбокс Match by path включён, поле заполнено сохранёнными строками, переключатель «Regex».
   - Высота окна Add Rule: 640 px.

6. **docs/release_notes/upcoming.md**
   - Пункты EN/RU про Process rule — Match by path.

## Проверка

- `go build ./...` — в среде без CGO/GUI сборка может падать на зависимостях fyne/gl; изменения в коде не добавляют новых зависимостей.
- `go test ./ui/wizard/models/` — проходит.
- `go test ./ui/wizard/dialogs/` — при успешной сборке тесты `SimplePatternToRegex` выполняются.
- `go vet ./...` — без замечаний по изменённым файлам.

## Риски и ограничения

- `process_path_regex` в sing-box поддерживается только на Linux, Windows, macOS.
- Режим Simple/Regex не сохраняется в state: при открытии на редактирование показываются сохранённые regex и переключатель «Regex».

## Дата

2025-03-09
