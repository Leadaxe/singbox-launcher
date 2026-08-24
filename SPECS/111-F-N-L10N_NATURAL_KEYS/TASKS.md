# TASKS 111 — Локализация на естественных ключах

Чеклист к [SPEC.md](SPEC.md) / [PLAN.md](PLAN.md).

**Предусловие:** ветка отводится от `develop` при чистом рабочем дереве.
Сейчас в нём незакоммиченные изменения по SPEC 110 (цепочки) — механический
дифф по 74 файлам с ними не смержится.

---

## Этап 1 — Движок

- [ ] `internal/locale/entry.go`: типы `Entry` / `Value`, `UnmarshalJSON`
      для трёх видов записи: голая строка (= `{"value": строка}`; так живут
      `_display_name` и старый формат), объект `value`, объект форм +
      `special`
- [ ] Битая запись не паникует: пропуск + `debuglog.WarnLog`
- [ ] `internal/locale/plural.go`: интерфейс `PluralResolver` с `Forms()`,
      `RuPluralResolver` (CLDR one/few/many/other), `EnPluralResolver`
- [ ] `internal/locale/locale.go`: убрать `//go:embed en.json` и `enJSON`
- [ ] `catalogs` → `map[string]map[string]Entry`, резолвер на язык
- [ ] Внутренний `render(form, key, plural, n, args…)`
- [ ] Публичные `T`, `Tf`, `TN`, `TfN`, `Plural`, `PluralN`
- [ ] Fallback: язык → форма 0 → ключ; аргументы подставляются и в fallback
- [ ] `LangDisplayName` читает `_display_name` как простую строку
- [ ] `entry_test.go`: разбор трёх видов записи + деградация битой
- [ ] `plural_test.go`: русский на 1, 2, 5, 11, 21, 22, 25, 101, 111
- [ ] `locale_test.go`: удалить `TestEmbeddedEnglish`, адаптировать остальные
- [ ] `go test ./internal/locale/...` зелёный

## Этап 2 — Миграция каталога

- [ ] `scripts/l10n_migrate.py --catalog`: группировка ключей по английскому
      значению
- [ ] Скрипт **падает** на группе с разными русскими переводами (13 известных)
- [ ] 13 коллизий разведены руками: форма 0 + `special["N"]`
- [ ] Соответствие `старый_ключ → индекс формы` записано в
      `scripts/l10n_collisions.json`
- [ ] 130 мёртвых ключей отброшены
- [ ] `_display_name` перенесён как простая строка
- [ ] `bin/locale/ru.json` в новом формате, ~1075 записей — валидный JSON

## Этап 3 — Call-sites

- [ ] `scripts/l10n_migrate.py --code`: замена 1224 литеральных вызовов
- [ ] Корректное экранирование Go-строк (кавычки, `\n`, слэши)
- [ ] Коллизии из `l10n_collisions.json` → `locale.TN(N, …)` / `TfN`
- [ ] Длинные (>120 символов) и многострочные значения — в `const` рядом с
      местом использования, не инлайн
- [ ] 7 динамических call-sites поправлены руками (константы-ключи, не
      видимые сканеру напрямую, помечены `// l10n-key`):
  - [ ] `ui/command_row.go:48,104`
  - [ ] `ui/connection_local_daemon_darwin.go:26`
  - [ ] `ui/traffic/by_client_view.go:200`
  - [ ] `ui/configurator/tabs/source_tab.go:523`
  - [ ] `ui/configurator/tabs/dns_tab.go:153`
  - [ ] `ui/configurator/tabs/source_edit_window.go:1202`
- [ ] `internal/locale/en.json` удалён
- [ ] `gofmt` по изменённым файлам
- [ ] `go build ./...` зелёный
- [ ] Старых точечных ключей не осталось (grep по строгой форме точечного
      ключа: только `[a-z0-9_.]` до закрывающей кавычки, минимум одна точка)
- [ ] Ручная проверка: запуск с `lang=ru` — интерфейс русский; с `lang=en` —
      английский; сырых ключей на экране нет
- [ ] Ручная проверка: битый/отсутствующий `ru.json` — старт есть, UI
      английский, в логе warn

> Этапы 1–3 вливаются **одним коммитом** — между ними проект собирается, но
> интерфейс временно английский.

## Этап 4 — Чекер `l10n_check`

- [ ] `tools/l10n/l10n_check/scan.go`: AST-скан вызовов `locale.*`, без `os`
- [ ] Резолв ключа-константы в пределах пакета
- [ ] Распознавание индекса special-формы (целочисленный литерал первым)
- [ ] Различение `T`-семейства и `Plural`-семейства
- [ ] Находки: `missing`, `orphan`, `orphan-special` (warn → fail при
      `--strict`)
- [ ] Находки: `usage-conflict`, `shape`, `arity` (fail всегда)
- [ ] Нормализация плейсхолдеров: `%s`, `%d`, `%[N]s`; `%%` не считается
- [ ] Для plural — сверка arity против **каждой** формы
- [ ] Полнота форм проверяется против `resolver.Forms()`, список не дублируется
- [ ] Динамические ключи считаются, не валидируются
- [ ] Маркер `// l10n-key` на константе = ключ использован (закрывает
      orphan-ложняки динамики)
- [ ] Оба чекера — только stdlib и синтаксис ≤ go1.20 (win7-джоба гоняет
      `go get ./...` по всем пакетам, включая `tools/`)
- [ ] Сводка вида `[l10n_check] keys: N, missing: N, orphan: N, dynamic: N`
- [ ] `scan_test.go`: self-тест на исходниках в памяти
- [ ] `go run ./tools/l10n/l10n_check --strict` — 0 находок

## Этап 5 — Ratchet `hardcoded_check`

- [ ] `tools/l10n/display_positions.json`: список конструкторов и сеттеров
      Fyne (SPEC §7)
- [ ] `tools/l10n/hardcoded_check/scan.go`: чистая логика скана
- [ ] Литерал внутри `locale.*` легален
- [ ] Рекурсия в ветки `if` / `switch` / тернарные внутри display-аргумента
- [ ] `// l10n-exempt: <причина>` — на строке или строкой выше; пустая причина
      = ошибка
- [ ] Baseline: счётчик файла не растёт, новый файл не появляется;
      `--write-baseline`
- [ ] Исключены `internal/debuglog`, `core/debugapi`, `tools/`
- [ ] Прогон по текущему `ui/`, найденный хардкод вычищен
- [ ] `tools/l10n/hardcoded_baseline.json` = `{}`
- [ ] `scan_test.go`: позитив, негатив, exempt, ветки
- [ ] `go run ./tools/l10n/hardcoded_check --strict` — 0 находок

## Этап 6 — Английский в `core/`

- [ ] `core/config/chain_generator.go:56,60,65,68,71,77,82`
- [ ] `core/config/chain_nodes.go:79,130`
- [ ] `core/core_chain_capability.go:108,110`
- [ ] `core/config_service.go:140`
- [ ] `core/config/direction_twins.go:179,268`
- [ ] `core/backup/file.go`, `import.go`, `export.go`
- [ ] `core/config/outbound_generator.go:701,767`, `detour_group_cycle.go:70`
- [ ] Строки переведены на английский; **не локализуются** (diagnostic
      passthrough, SPEC §8-§9) — записи в `ru.json` не заводятся
- [ ] В лог и в UI уходит один и тот же английский текст
- [ ] `internal/platform/glprobe_windows.go` — `// l10n-exempt: pre-init
      native dialog`, текст не трогаем

## Этап 7 — CI и документация

- [ ] Шаг «L10n checks» в `.github/workflows/ci.yml`, job `Test`, только
      Ubuntu, после «Run tests», оба чекера с `--strict`
- [ ] `tools/l10n/README.md`: команды и значение каждой находки
- [ ] `docs/ARCHITECTURE.md`: раздел «Локализация» переписан под новую модель
- [ ] `SPECS/CONSTITUTION.md` 8.3: английский текст в коде — источник истины
      и ключ перевода
- [ ] `docs/release_notes/upcoming.md`: смена механики ломает сторонние
      `bin/locale/*.json`
- [ ] `IMPLEMENTATION_REPORT.md` заполнен

## Закрытие

- [ ] `go build ./...`, `go test ./...`, `go vet ./...` — зелёные
- [ ] Легаси-джоба Win7 (тулчейн go 1.20.14, `-modfile=go.win7.mod`,
      `go get ./...` по всем пакетам) не сломана: чекеры stdlib-only, без
      go1.21+ синтаксиса
- [ ] Все критерии приёмки SPEC §«Критерии приёмки» выполнены
- [ ] Папка переименована в `111-F-C-L10N_NATURAL_KEYS`
