# Отчёт о реализации: 017 — Process path (Match by path, Simple/Regex)

## Статус

Реализовано. Готово к тестированию и закрытию задачи.

## Изменения

1. **ui/wizard/dialogs/rule_dialog.go**
   - Константа `ProcessPathRegexKey = "process_path_regex"`.
   - Функция `SimplePatternToRegex(pattern string) (string, error)`: замена `*` → `(.*)`, экранирование метасимволов regex.

2. **ui/wizard/dialogs/rule_dialog_test.go** (новый)
   - Юнит-тесты для `SimplePatternToRegex`: базовые шаблоны, экранирование, пустая строка.

3. **ui/wizard/models/wizard_state_file.go**
   - В `DetermineRuleType`: при наличии в rule поля `process_path_regex` возвращается тип «Processes».

4. **ui/wizard/dialogs/add_rule_dialog.go**
   - Чекбокс «Match by path» в блоке Processes.
   - При включении: переключатель Simple / Regex и многострочное поле «Path patterns».
   - Режим Simple: при сохранении `*` → `(.*)` через `SimplePatternToRegex`, валидация regex.
   - Режим Regex: строки как есть, проверка `regexp.Compile`.
   - В `buildRuleRaw` для Processes при Match by path формируется rule с `process_path_regex` (массив regex).
   - В `validateFields` для Processes при Match by path проверяется наличие строк и валидность regex.
   - При редактировании правила с `process_path_regex`: чекбокс включён, поле заполнено сохранёнными строками, переключатель «Regex».
   - Видимость: при Process показываются либо выбор по имени (кнопка + список), либо блок path (переключатель + поле) в зависимости от чекбокса.

5. **docs/release_notes/upcoming.md**
   - Пункты EN/RU про Process rule — Match by path.

## Проверка

- `go build ./...` — в среде без CGO/GUI сборка может падать на зависимостях fyne/gl; изменения в коде не добавляют новых зависимостей.
- `go test ./ui/wizard/models/` — проходит.
- `go test ./ui/wizard/dialogs/` — требует сборки пакета (CGO/display); при успешной сборке тесты `SimplePatternToRegex` выполняются.
- `go vet ./...` — без замечаний по изменённым файлам.

## Риски и ограничения

- `process_path_regex` в sing-box поддерживается только на Linux, Windows, macOS.
- Режим Simple/Regex не сохраняется в state: при открытии на редактирование всегда показываются сохранённые regex и переключатель «Regex».

## Дата

2025-03-09
