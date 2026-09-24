# SPEC 138 · План

Разведка (SPEC §2) показала: лаунчер ключи сна WG не пишет и не читает. Работа
сводится к фиксации решения, страховочному тесту и докам. Код сборщика, шаблон,
состояние и UI не меняются.

## Файлы

| Файл | Изменение |
|---|---|
| `SPECS/138-F-N-LX_WG_IDLE_KEYS/{SPEC,PLAN,TASKS}.md` | новые |
| `core/build/lx_root_block_test.go` | новый: `TestBuildConfigPassesRootLXBlock` |
| `docs/ARCHITECTURE.md`, `docs/ARCHITECTURE.ru.md` | пункт в «Key properties» у `BuildConfig`: сквозной путь корневых секций без обработчика, блок `lx`, сон WG не пишется |
| `docs/release_notes/upcoming.md` | строка EN/RU в «Technical / Internal» |
| `SPECS/README.md` | запись 138 |

## Тест

Один интеграционный тест сборщика на волну: `TestBuildConfigPassesRootLXBlock`.

- Шаблон с `vars` идёт через `template.ParseTemplateData` → `BuildConfig`. Это
  тот же путь `GetEffectiveConfigFor`, что у поставляемого шаблона.
- В `config` лежит `"lx": {"masque": {"idle_timeout": "@masque_idle"}}`.
- Проверяется: блок дошёл до config.json, `@var` подставлена значением из
  состояния, соседняя секция на месте, в `route` не появилось `lx_idle`.

Порядок секций тест не проверяет. `ParseTemplateData` уже отдаёт их по алфавиту,
ядру порядок безразличен.

## Не делается (обоснование — SPEC §3.1)

- Гейт по версии ядра.
- Вынос `parseCoreBuild` / `compareCoreBuilds` из `core/daemon_service_state_darwin.go`.
- Перенос `route` → `lx` в сборщике.
- Разбор running-config.

## Коммиты

1. `docs(spec138): …` — SPEC/PLAN/TASKS.
2. `test(build): …` — `lx_root_block_test.go`.
3. `docs(spec138): …` — ARCHITECTURE EN/RU, upcoming, `SPECS/README.md`, отметки в TASKS.

Не пушится.
