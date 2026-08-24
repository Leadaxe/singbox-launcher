# TASKS 111 — Локализация на естественных ключах

Чеклист к [SPEC.md](SPEC.md) / [PLAN.md](PLAN.md).

**Режим работы:** коммиты прямо в `develop` (ветки не переключаем — рабочую
копию делят другие агенты), инкрементальный переход по PLAN §«Порядок»:
движок параллелен старому, оба стиля ключей работают одновременно, легаси
снимается финальным шагом.

---

## Этап 1 — Движок

- [x] `internal/locale/entry.go`: типы `Entry` / `Value`, `UnmarshalJSON`
      для трёх видов записи: голая строка (= `{"value": строка}`; так живут
      `_display_name` и старый формат), объект `value`, объект форм +
      `special`
- [x] Битая запись не паникует: пропуск + `debuglog.WarnLog`
- [x] `internal/locale/plural.go`: интерфейс `PluralResolver` с `Forms()`,
      `RuPluralResolver` (CLDR one/few/many/other), `EnPluralResolver`
- [x] `internal/locale/locale.go`: убрать `//go:embed en.json` и `enJSON`
      (финальный шаг миграции, не этап 1 — см. PLAN §«Порядок»)
- [x] `catalogs` → `map[string]map[string]Entry`, резолвер на язык
- [x] Внутренний `render(form, key, plural, n, args…)`
- [x] Публичные `T`, `Tf`, `TN`, `TfN`, `Plural`, `PluralN`
- [x] Fallback: язык → форма 0 → ключ; аргументы подставляются и в fallback
- [x] `LangDisplayName` читает `_display_name` как простую строку
- [x] `entry_test.go`: разбор трёх видов записи + деградация битой
- [x] `plural_test.go`: русский на 1, 2, 5, 11, 21, 22, 25, 101, 111
- [x] `locale_test.go`: адаптировать под `Entry` (`TestEmbeddedEnglish`
      живёт до снятия `en.json`; parity остался только en → ru)
- [x] `go test ./internal/locale/...` зелёный

## Этап 2 — Миграция каталога

- [x] `scripts/l10n_migrate.py --catalog`: группировка ключей по английскому
      значению
- [x] Скрипт **падает** на группе с разными русскими переводами (13 известных)
- [x] 13 коллизий разведены руками: форма 0 + `special["N"]`
- [x] Соответствие `старый_ключ → индекс формы` записано в
      `scripts/l10n_collisions.json`
- [x] 130 мёртвых ключей отброшены
- [x] `_display_name` перенесён как простая строка
- [x] `bin/locale/ru.json` в новом формате, ~1091 запись — валидный JSON

## Этап 3 — Call-sites

- [x] `scripts/l10n_migrate.py --code`: замена 1234 литеральных вызовов
- [x] Корректное экранирование Go-строк (кавычки, `\n`, слэши)
- [x] Коллизии из `l10n_collisions.json` → `locale.TN(N, …)` / `TfN`
- [x] Длинные (>120 символов) и многострочные значения — в `const` рядом с
      местом использования, не инлайн
- [x] 7 динамических call-sites поправлены руками (константы-ключи, не
      видимые сканеру напрямую, помечены `// l10n-key`):
  - [x] `ui/command_row.go:48,104`
  - [x] `ui/connection_local_daemon_darwin.go:26`
  - [x] `ui/traffic/by_client_view.go:200`
  - [x] `ui/configurator/tabs/source_tab.go:523`
  - [x] `ui/configurator/tabs/dns_tab.go:153`
  - [x] `ui/configurator/tabs/source_edit_window.go:1202`
- [x] `internal/locale/en.json` удалён
- [x] `gofmt` по изменённым файлам
- [x] `go build ./...` зелёный
- [x] Старых точечных ключей не осталось (grep по строгой форме точечного
      ключа: только `[a-z0-9_.]` до закрывающей кавычки, минимум одна точка)
- [ ] Ручная проверка: запуск с `lang=ru` — интерфейс русский; с `lang=en` —
      английский; сырых ключей на экране нет
- [ ] Ручная проверка: битый/отсутствующий `ru.json` — старт есть, UI
      английский, в логе warn

> Переход инкрементальный (PLAN §«Порядок»): каждая пачка замен — рабочий
> коммит; легаси-ключи и `en.json` снимаются финальным шагом этапа 3.

## Этап 4 — Чекер `l10n_check`

- [x] `tools/l10n/l10n_check/scan.go`: AST-скан вызовов `locale.*`, без `os`
- [x] Резолв ключа-константы в пределах пакета
- [x] Распознавание индекса special-формы (целочисленный литерал первым)
- [x] Различение `T`-семейства и `Plural`-семейства
- [x] Находки: `missing`, `orphan`, `orphan-special` (warn → fail при
      `--strict`)
- [x] Находки: `usage-conflict`, `shape`, `arity` (fail всегда)
- [x] Нормализация плейсхолдеров: `%s`, `%d`, `%[N]s`; `%%` не считается
- [x] Для plural — сверка arity против **каждой** формы
- [x] Полнота форм проверяется против `resolver.Forms()`, список не дублируется
- [x] Динамические ключи считаются, не валидируются
- [x] Маркер `// l10n-key` на константе = ключ использован (закрывает
      orphan-ложняки динамики)
- [x] Оба чекера — только stdlib и синтаксис ≤ go1.20 (win7-джоба гоняет
      `go get ./...` по всем пакетам, включая `tools/`)
- [x] Сводка вида `[l10n_check] keys: N, missing: N, orphan: N, dynamic: N`
- [x] `scan_test.go`: self-тест на исходниках в памяти
- [x] `go run ./tools/l10n/l10n_check --strict` — 0 находок

## Этап 5 — Ratchet `hardcoded_check`

- [x] `tools/l10n/display_positions.json`: список конструкторов и сеттеров
      Fyne (SPEC §7)
- [x] `tools/l10n/hardcoded_check/scan.go`: чистая логика скана
- [x] Литерал внутри `locale.*` легален
- [x] ~~Рекурсия в ветки~~ — не нужна: в Go нет тернарного оператора,
      ветки кладут литерал в переменную; поток переменных — осознанный зазор
      best-effort ratchet (tools/l10n/README.md)
- [x] `// l10n-exempt: <причина>` — на строке или строкой выше; пустая причина
      = ошибка
- [x] Baseline: счётчик файла не растёт, новый файл не появляется;
      `--write-baseline`
- [x] Исключены `internal/debuglog`, `core/debugapi`, `tools/`
- [x] Прогон по текущему `ui/`, найденный хардкод вычищен
- [x] `tools/l10n/hardcoded_baseline.json` = `{}`
- [x] `scan_test.go`: позитив, негатив, exempt, ветки
- [x] `go run ./tools/l10n/hardcoded_check --strict` — 0 находок

## Этап 6 — Английский в `core/`

- [x] `core/config/chain_generator.go:56,60,65,68,71,77,82`
- [x] `core/config/chain_nodes.go:79,130`
- [x] `core/core_chain_capability.go:108,110`
- [x] `core/config_service.go:140`
- [x] `core/config/direction_twins.go:179,268`
- [x] `core/backup/file.go`, `import.go`, `export.go`
- [x] ~~`core/config/outbound_generator.go:701,767`, `detour_group_cycle.go:70`~~
      — это WarnLog-строки: логи не переводятся (SPEC §8), тотальная чистка
      русских логов вне scope 111
- [x] Строки переведены на английский; **не локализуются** (diagnostic
      passthrough, SPEC §8-§9) — записи в `ru.json` не заводятся
- [x] В лог и в UI уходит один и тот же английский текст
- [x] `internal/platform/glprobe_windows.go` — не трогаем; ratchet его
      нативные MessageBox-вызовы не сканирует, маркер не понадобился

## Этап 7 — CI и документация

- [x] Шаг «L10n checks» в `.github/workflows/ci.yml`, job `Test`, только
      Ubuntu, после «Run tests», оба чекера с `--strict`
- [x] `tools/l10n/README.md`: команды и значение каждой находки
- [x] `docs/ARCHITECTURE.md`: раздел «Локализация» переписан под новую модель
- [x] `SPECS/CONSTITUTION.md` 8.3: английский текст в коде — источник истины
      и ключ перевода
- [x] `docs/release_notes/upcoming.md`: смена механики ломает сторонние
      `bin/locale/*.json`
- [ ] `IMPLEMENTATION_REPORT.md` заполнен

## Закрытие

- [x] `go build ./...`, `go test ./...`, `go vet ./...` — зелёные
- [x] Легаси-джоба Win7 (тулчейн go 1.20.14, `-modfile=go.win7.mod`,
      `go get ./...` по всем пакетам) не сломана: чекеры stdlib-only, без
      go1.21+ синтаксиса
- [x] Все критерии приёмки SPEC §«Критерии приёмки» выполнены
- [ ] Папка переименована в `111-F-C-L10N_NATURAL_KEYS`
